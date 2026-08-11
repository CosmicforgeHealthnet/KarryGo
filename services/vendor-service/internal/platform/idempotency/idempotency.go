// Package idempotency implements the Idempotency-Key foundation for
// vendor-service's state-changing commands (commerce checkout, vendor order
// mutations, fulfilment plan creation, etc.).
//
// This is a service-local copy adapted from fleet-partner-service's proven
// idempotency store, not an import from a shared package: as of this
// architecture phase no other service in the platform implements idempotency
// through a shared/go package either (see
// docs/architecture/00_REPOSITORY_AUDIT_BASELINE.md, "Idempotency"). Every
// service that has real command idempotency today reimplements it locally.
// Extracting a shared/go/idempotency package is a reasonable future step once
// a second service wants this exact shape, but is out of scope for this
// service's own architecture foundation and would touch every module in
// go.work's dependency graph. Do not fork this again inside vendor-service
// itself — one Store, reused by every command handler.
//
// Keys are scoped per (scopeID, endpoint, idempotencyKey). scopeID is
// caller-defined: typically a vendor business id for vendor-management
// commands, or a customer id for commerce checkout commands. The stored
// request fingerprint detects key reuse with a different request body
// (conflict). Only cacheable outcomes (2xx/3xx, deterministic 4xx) are
// replayed; auth failures, rate limits and 5xx release the key so a retry can
// execute again.
package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Statuses of an idempotency record.
const (
	StatusInProgress = "IN_PROGRESS"
	StatusCompleted  = "COMPLETED"
)

// DefaultExpiry bounds how long completed responses are replayable.
const DefaultExpiry = 24 * time.Hour

// ErrInProgress means a request with the same key is currently executing.
var ErrInProgress = errors.New("idempotency: request with this key is in progress")

// ErrFingerprintMismatch means the key was reused with a different request.
var ErrFingerprintMismatch = errors.New("idempotency: key reused with different request")

// Record is the stored idempotency state.
type Record struct {
	Status         string
	ResponseStatus int
	ResponseBody   []byte
}

// Fingerprint produces the normalized request hash: method, path and body.
func Fingerprint(method string, path string, body []byte) string {
	sum := sha256.Sum256(append([]byte(method+"\n"+path+"\n"), body...))
	return hex.EncodeToString(sum[:])
}

// querier is satisfied by *pgxpool.Pool and pgx.Tx, so a caller can run
// Begin/Complete inside the same transaction as the command it guards.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store persists idempotency records in vendor-service's own database.
type Store struct {
	db     querier
	expiry time.Duration
}

// NewStore wires the store over a pool or transaction; expiry <= 0 uses
// DefaultExpiry.
func NewStore(db *pgxpool.Pool, expiry time.Duration) *Store {
	if expiry <= 0 {
		expiry = DefaultExpiry
	}
	return &Store{db: db, expiry: expiry}
}

// Begin claims the key for execution.
//   - First use: inserts IN_PROGRESS and returns (nil, nil) — caller executes.
//   - Completed with same fingerprint: returns the stored record for replay.
//   - Completed/in-progress with different fingerprint: ErrFingerprintMismatch.
//   - In progress with same fingerprint: ErrInProgress.
//
// Concurrency is settled by the primary-key insert: exactly one concurrent
// caller wins the claim.
func (s *Store) Begin(ctx context.Context, scopeID, endpoint, key, fingerprint string) (*Record, error) {
	if key == "" {
		return nil, errors.New("idempotency: Idempotency-Key is required")
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO vendor_idempotency_keys
			(scope_id, endpoint, idempotency_key, request_fingerprint, status, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, now(), now() + $6::interval)`,
		scopeID, endpoint, key, fingerprint, StatusInProgress,
		fmt.Sprintf("%d seconds", int64(s.expiry.Seconds())),
	)
	if err == nil {
		return nil, nil // claimed; caller executes the command
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return nil, fmt.Errorf("idempotency: claim key: %w", err)
	}

	var record Record
	var storedFingerprint string
	var expiresAt time.Time
	err = s.db.QueryRow(ctx, `
		SELECT request_fingerprint, status, COALESCE(response_status, 0), COALESCE(response_body, ''), expires_at
		FROM vendor_idempotency_keys
		WHERE scope_id = $1 AND endpoint = $2 AND idempotency_key = $3`,
		scopeID, endpoint, key,
	).Scan(&storedFingerprint, &record.Status, &record.ResponseStatus, &record.ResponseBody, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInProgress // raced with cleanup; treat as in progress
	}
	if err != nil {
		return nil, fmt.Errorf("idempotency: load key: %w", err)
	}

	if time.Now().After(expiresAt) {
		_, err = s.db.Exec(ctx, `
			UPDATE vendor_idempotency_keys SET
				request_fingerprint = $1, status = $2, response_status = NULL,
				response_body = NULL, created_at = now(), expires_at = now() + $3::interval
			WHERE scope_id = $4 AND endpoint = $5 AND idempotency_key = $6`,
			fingerprint, StatusInProgress,
			fmt.Sprintf("%d seconds", int64(s.expiry.Seconds())),
			scopeID, endpoint, key,
		)
		if err != nil {
			return nil, fmt.Errorf("idempotency: reclaim expired key: %w", err)
		}
		return nil, nil
	}

	if storedFingerprint != fingerprint {
		return nil, ErrFingerprintMismatch
	}
	if record.Status == StatusInProgress {
		return nil, ErrInProgress
	}
	return &record, nil
}

// Complete stores the response for replay, or releases the key if the
// outcome is not safely cacheable.
func (s *Store) Complete(ctx context.Context, scopeID, endpoint, key string, responseStatus int, responseBody []byte) error {
	if !cacheable(responseStatus) {
		return s.Release(ctx, scopeID, endpoint, key)
	}
	_, err := s.db.Exec(ctx, `
		UPDATE vendor_idempotency_keys SET
			status = $1, response_status = $2, response_body = $3
		WHERE scope_id = $4 AND endpoint = $5 AND idempotency_key = $6`,
		StatusCompleted, responseStatus, responseBody, scopeID, endpoint, key,
	)
	if err != nil {
		return fmt.Errorf("idempotency: complete: %w", err)
	}
	return nil
}

// Release drops the claim so a later retry can execute again.
func (s *Store) Release(ctx context.Context, scopeID, endpoint, key string) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM vendor_idempotency_keys
		WHERE scope_id = $1 AND endpoint = $2 AND idempotency_key = $3 AND status = $4`,
		scopeID, endpoint, key, StatusInProgress,
	)
	if err != nil {
		return fmt.Errorf("idempotency: release: %w", err)
	}
	return nil
}

func cacheable(status int) bool {
	if status >= 200 && status < 400 {
		return true
	}
	if status == 401 || status == 403 || status == 429 {
		return false
	}
	return status >= 400 && status < 500
}
