// Package idempotency implements the reusable Idempotency-Key foundation for
// future state-changing commands (financial commands, provisioning, imports,
// assignments).
//
// Keys are scoped per organization and endpoint. The stored request
// fingerprint detects key reuse with a different request (conflict). Original
// success responses are replayed; authentication failures and server errors
// are never cached.
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

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
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

// Store persists idempotency records in the service database.
type Store struct {
	db     database.Querier
	expiry time.Duration
}

// NewStore wires the store; expiry <= 0 uses DefaultExpiry.
func NewStore(db database.Querier, expiry time.Duration) *Store {
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
func (s *Store) Begin(ctx context.Context, organizationID string, endpoint string, key string, fingerprint string) (*Record, error) {
	if key == "" {
		return nil, apperrors.BadRequest("Idempotency-Key header is required.", nil)
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO fleet_partner_idempotency_keys
			(organization_id, endpoint, idempotency_key, request_fingerprint, status, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, now(), now() + $6::interval)`,
		organizationID, endpoint, key, fingerprint, StatusInProgress,
		fmt.Sprintf("%d seconds", int64(s.expiry.Seconds())),
	)
	if err == nil {
		return nil, nil // claimed; caller executes the command
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return nil, fmt.Errorf("idempotency: claim key: %w", err)
	}

	// Key exists: load it and decide.
	var record Record
	var storedFingerprint string
	var expiresAt time.Time
	err = s.db.QueryRow(ctx, `
		SELECT request_fingerprint, status, COALESCE(response_status, 0), COALESCE(response_body, ''), expires_at
		FROM fleet_partner_idempotency_keys
		WHERE organization_id = $1 AND endpoint = $2 AND idempotency_key = $3`,
		organizationID, endpoint, key,
	).Scan(&storedFingerprint, &record.Status, &record.ResponseStatus, &record.ResponseBody, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInProgress // raced with cleanup; treat as in progress
	}
	if err != nil {
		return nil, fmt.Errorf("idempotency: load key: %w", err)
	}

	if time.Now().After(expiresAt) {
		// Expired: replace the claim and let the caller execute.
		_, err = s.db.Exec(ctx, `
			UPDATE fleet_partner_idempotency_keys SET
				request_fingerprint = $1, status = $2, response_status = NULL,
				response_body = NULL, created_at = now(), expires_at = now() + $3::interval
			WHERE organization_id = $4 AND endpoint = $5 AND idempotency_key = $6`,
			fingerprint, StatusInProgress,
			fmt.Sprintf("%d seconds", int64(s.expiry.Seconds())),
			organizationID, endpoint, key,
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

// Complete stores the response for replay. Only cacheable outcomes are
// stored: 2xx/3xx and deterministic client validation failures (4xx except
// 401/403/429). Authentication failures, rate limits and 5xx release the key
// instead so the command can be retried.
func (s *Store) Complete(ctx context.Context, organizationID string, endpoint string, key string, responseStatus int, responseBody []byte) error {
	if !cacheable(responseStatus) {
		return s.Release(ctx, organizationID, endpoint, key)
	}
	_, err := s.db.Exec(ctx, `
		UPDATE fleet_partner_idempotency_keys SET
			status = $1, response_status = $2, response_body = $3
		WHERE organization_id = $4 AND endpoint = $5 AND idempotency_key = $6`,
		StatusCompleted, responseStatus, responseBody, organizationID, endpoint, key,
	)
	if err != nil {
		return fmt.Errorf("idempotency: complete: %w", err)
	}
	return nil
}

// Release drops the claim so a later retry can execute again.
func (s *Store) Release(ctx context.Context, organizationID string, endpoint string, key string) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM fleet_partner_idempotency_keys
		WHERE organization_id = $1 AND endpoint = $2 AND idempotency_key = $3 AND status = $4`,
		organizationID, endpoint, key, StatusInProgress,
	)
	if err != nil {
		return fmt.Errorf("idempotency: release: %w", err)
	}
	return nil
}

// AsAppError maps package errors onto public API errors.
func AsAppError(err error) error {
	switch {
	case errors.Is(err, ErrInProgress):
		return apperrors.Conflict("A request with this idempotency key is already in progress.", err)
	case errors.Is(err, ErrFingerprintMismatch):
		return apperrors.Conflict("This idempotency key was already used with a different request.", err)
	default:
		return err
	}
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
