// Package idempotency implements the Idempotency-Key foundation for
// state-changing financial commands.
//
// Keys are scoped per financial scope (organization, or PLATFORM) and
// endpoint. The stored request fingerprint detects key reuse with a different
// request, which is a conflict rather than a replay. Original success
// responses are replayed; authentication failures, rate limits and server
// errors release the key so the command can be retried.
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

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
)

// Statuses of an idempotency record.
const (
	StatusInProgress = "IN_PROGRESS"
	StatusCompleted  = "COMPLETED"
)

// PlatformScope is the scope used for commands that are not organization-owned.
const PlatformScope = "PLATFORM"

// DefaultExpiry bounds how long completed responses are replayable.
const DefaultExpiry = 24 * time.Hour

// ErrInProgress means a request with the same key is currently executing.
var ErrInProgress = errors.New("idempotency: request with this key is in progress")

// ErrFingerprintMismatch means the key was reused with a different request.
var ErrFingerprintMismatch = errors.New("idempotency: key reused with different request")

// ErrKeyRequired means the caller omitted the key on a command that must be
// deduplicable.
var ErrKeyRequired = errors.New("idempotency: an idempotency key is required")

// Record is the stored idempotency state.
type Record struct {
	Status         string
	ResponseStatus int
	ResponseBody   []byte
}

// Fingerprint produces the normalized request hash over method, path and body.
func Fingerprint(method string, path string, body []byte) string {
	sum := sha256.Sum256(append([]byte(method+"\n"+path+"\n"), body...))
	return hex.EncodeToString(sum[:])
}

// FingerprintFields hashes an ordered list of semantic request fields. Used by
// service-level commands whose fingerprint must not depend on JSON key order
// or transport encoding.
func FingerprintFields(fields ...string) string {
	hasher := sha256.New()
	for _, field := range fields {
		_, _ = hasher.Write([]byte(field))
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
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
//   - Any state with a different fingerprint: ErrFingerprintMismatch.
//   - In progress with same fingerprint: ErrInProgress.
//
// Concurrency is settled by the primary-key insert: exactly one concurrent
// caller wins the claim.
func (s *Store) Begin(ctx context.Context, scopeID string, endpoint string, key string, fingerprint string) (*Record, error) {
	if key == "" {
		return nil, ErrKeyRequired
	}
	if scopeID == "" {
		scopeID = PlatformScope
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO finance_idempotency_keys
			(scope_id, endpoint, idempotency_key, request_fingerprint, status, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, now(), now() + make_interval(secs => $6))`,
		scopeID, endpoint, key, fingerprint, StatusInProgress, s.expiry.Seconds(),
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
		SELECT request_fingerprint, status, COALESCE(response_status, 0),
		       COALESCE(response_body, ''::bytea), expires_at
		FROM finance_idempotency_keys
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
			UPDATE finance_idempotency_keys SET
				request_fingerprint = $1, status = $2, response_status = NULL,
				response_body = NULL, created_at = now(),
				expires_at = now() + make_interval(secs => $3)
			WHERE scope_id = $4 AND endpoint = $5 AND idempotency_key = $6`,
			fingerprint, StatusInProgress, s.expiry.Seconds(), scopeID, endpoint, key,
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

// Complete stores the response for replay. Only cacheable outcomes are stored;
// 401/403/429 and 5xx release the key so a retry can execute again.
func (s *Store) Complete(ctx context.Context, scopeID string, endpoint string, key string, responseStatus int, responseBody []byte) error {
	if scopeID == "" {
		scopeID = PlatformScope
	}
	if !cacheable(responseStatus) {
		return s.Release(ctx, scopeID, endpoint, key)
	}
	_, err := s.db.Exec(ctx, `
		UPDATE finance_idempotency_keys SET
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
func (s *Store) Release(ctx context.Context, scopeID string, endpoint string, key string) error {
	if scopeID == "" {
		scopeID = PlatformScope
	}
	_, err := s.db.Exec(ctx, `
		DELETE FROM finance_idempotency_keys
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
