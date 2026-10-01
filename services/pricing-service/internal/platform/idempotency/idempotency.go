// Package idempotency makes a repeated administrative command safe.
//
// Quote issuance, binding and reconciliation do NOT use this store: they are
// made idempotent by natural keys and unique constraints on the records
// themselves, which is stronger because the guarantee survives this table being
// pruned. This store covers the administrative commands, where the request is
// the only key available.
//
// A retry of the same command returns the first outcome. A DIFFERENT command
// sent under a key that has already been used is refused rather than executed,
// because the caller evidently believes it is retrying something else.
//
// It is a deliberate local copy of the store in vendor-service and
// fleet-partner-service: there is no shared/go/idempotency package yet, and
// inventing one as part of this service would couple three services' schemas.
package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/pricing-service/internal/database"
)

// Command lifecycle states.
const (
	statusInProgress = "IN_PROGRESS"
	statusCompleted  = "COMPLETED"
)

// DefaultExpiry is how long a key is remembered.
const DefaultExpiry = 24 * time.Hour

var (
	// ErrInProgress means an identical command is still running. The caller
	// should retry, not duplicate.
	ErrInProgress = errors.New("idempotency: an identical request is in progress")
	// ErrFingerprintMismatch means the key was reused for a different request.
	ErrFingerprintMismatch = errors.New("idempotency: key reused with a different request")
)

// Fingerprint identifies the exact request a key was first used for.
func Fingerprint(method, path string, body []byte) string {
	sum := sha256.New()
	sum.Write([]byte(strings.ToUpper(method)))
	sum.Write([]byte("\n"))
	sum.Write([]byte(path))
	sum.Write([]byte("\n"))
	sum.Write(body)
	return hex.EncodeToString(sum.Sum(nil))
}

// Store records in-flight and completed commands.
type Store struct {
	db     database.Querier
	expiry time.Duration
}

// NewStore returns a store over db.
func NewStore(db database.Querier, expiry time.Duration) *Store {
	if expiry <= 0 {
		expiry = DefaultExpiry
	}
	return &Store{db: db, expiry: expiry}
}

// Outcome is what Begin found.
type Outcome struct {
	// Proceed is true when this caller owns the claim and should run the
	// command.
	Proceed bool
	// Response is the stored response of a command that already completed.
	Response []byte
}

// Begin claims key for this request.
//
// It returns Proceed when the caller owns the claim, a stored Response when the
// command already completed, ErrInProgress when an identical command is running
// elsewhere, or ErrFingerprintMismatch when the key belongs to a different
// request.
func (s *Store) Begin(ctx context.Context, caller, endpoint, key, fingerprint string) (Outcome, error) {
	if strings.TrimSpace(key) == "" {
		return Outcome{}, errors.New("idempotency: key is required")
	}

	// Try to claim. DO NOTHING means someone else already holds the key, which
	// is decided by the database rather than by inspecting timestamps.
	tag, err := s.db.Exec(ctx, `
		INSERT INTO pricing_idempotency_keys
			(caller, endpoint, idempotency_key, request_fingerprint, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, now() + $6::interval)
		ON CONFLICT (caller, endpoint, idempotency_key) DO NOTHING`,
		caller, endpoint, key, fingerprint, statusInProgress, s.expiry.String())
	if err != nil {
		return Outcome{}, fmt.Errorf("idempotency: claim %s: %w", key, err)
	}
	if tag.RowsAffected() == 1 {
		return Outcome{Proceed: true}, nil
	}

	var (
		storedFingerprint string
		status            string
		response          []byte
	)
	if err := s.db.QueryRow(ctx, `
		SELECT request_fingerprint, status, response_body
		FROM pricing_idempotency_keys
		WHERE caller = $1 AND endpoint = $2 AND idempotency_key = $3`,
		caller, endpoint, key,
	).Scan(&storedFingerprint, &status, &response); err != nil {
		return Outcome{}, fmt.Errorf("idempotency: inspect %s: %w", key, err)
	}

	if storedFingerprint != fingerprint {
		return Outcome{}, fmt.Errorf("%w: %s", ErrFingerprintMismatch, key)
	}
	if status == statusCompleted {
		return Outcome{Response: response}, nil
	}
	return Outcome{}, fmt.Errorf("%w: %s", ErrInProgress, key)
}

// Complete stores the outcome so a retry returns the same answer. Pass the
// transaction that performed the command so the record and its response commit
// together.
func (s *Store) Complete(ctx context.Context, q database.Querier, caller, endpoint, key string, response []byte) error {
	if q == nil {
		q = s.db
	}
	_, err := q.Exec(ctx, `
		UPDATE pricing_idempotency_keys
		SET status = $4, response_body = $5, completed_at = now()
		WHERE caller = $1 AND endpoint = $2 AND idempotency_key = $3`,
		caller, endpoint, key, statusCompleted, response)
	if err != nil {
		return fmt.Errorf("idempotency: complete %s: %w", key, err)
	}
	return nil
}

// Release abandons a claim so the caller can genuinely retry. Use it only when
// the command failed leaving nothing behind.
func (s *Store) Release(ctx context.Context, caller, endpoint, key string) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM pricing_idempotency_keys
		WHERE caller = $1 AND endpoint = $2 AND idempotency_key = $3 AND status = $4`,
		caller, endpoint, key, statusInProgress)
	if err != nil {
		return fmt.Errorf("idempotency: release %s: %w", key, err)
	}
	return nil
}

// Prune deletes expired keys.
func (s *Store) Prune(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `DELETE FROM pricing_idempotency_keys WHERE expires_at < now()`)
	return err
}
