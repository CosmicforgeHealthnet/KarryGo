// Package noncestore closes a gap shared/go/serviceauth.Verifier leaves
// open: that package signs a nonce as part of every internal HMAC request
// and requires one to be present, but never records that a nonce was
// already consumed, so a valid, freshness-window-signed request can be
// replayed verbatim any number of times. This store makes replay of an
// already-verified request within the signature's own freshness window
// impossible, without editing shared/go/serviceauth — that package is used
// by every other service in the platform, and hardening it the same way
// for all of them is a separate, platform-wide decision outside one
// service's scope.
package noncestore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

// ErrNonceReplayed means this exact (caller, nonce) pair was already
// consumed — the request is a replay, however validly it was signed.
var ErrNonceReplayed = errors.New("noncestore: nonce already consumed")

// Store records consumed HMAC nonces per calling service.
type Store struct {
	db database.Querier
}

// New wires the store over a pool or transaction.
func New(db database.Querier) *Store { return &Store{db: db} }

// Consume records one (callerService, nonce) pair as used, for ttl. Returns
// ErrNonceReplayed if that exact pair was already recorded and has not yet
// expired — a replay of a previously accepted request, not a new one.
//
// ttl should be set to at least the verifier's own signature freshness
// window (its maxSkew): a nonce cannot be replayed once its signature has
// expired on its own, so nothing is lost by letting the row expire and be
// reclaimed once the window it could possibly matter in has passed.
func (s *Store) Consume(ctx context.Context, callerService string, nonce string, ttl time.Duration) error {
	if callerService == "" || nonce == "" {
		return fmt.Errorf("noncestore: caller service and nonce are required")
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	tag, err := s.db.Exec(ctx, `
		INSERT INTO fleet_partner_service_nonces (caller_service, nonce, expires_at)
		VALUES ($1, $2, now() + $3::interval)
		ON CONFLICT (caller_service, nonce) DO UPDATE SET
			expires_at = EXCLUDED.expires_at
		WHERE fleet_partner_service_nonces.expires_at < now()`,
		callerService, nonce, fmt.Sprintf("%d seconds", int64(ttl.Seconds())))
	if err != nil {
		return fmt.Errorf("noncestore: consume: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either the row already existed and had not yet expired (a genuine
		// replay), or the ON CONFLICT UPDATE's WHERE excluded it for the same
		// reason — both land here identically.
		return ErrNonceReplayed
	}
	return nil
}

// ExpireDue deletes nonce records past their expiry, so the table does not
// grow without bound. Implements jobs.Expirer, so *Store can be registered
// directly alongside the OTP/session/invitation sweeps the existing
// maintenance worker already runs.
func (s *Store) ExpireDue(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 || limit > 10000 {
		limit = 5000
	}
	tag, err := s.db.Exec(ctx, `
		DELETE FROM fleet_partner_service_nonces
		WHERE (caller_service, nonce) IN (
			SELECT caller_service, nonce FROM fleet_partner_service_nonces
			WHERE expires_at < now()
			LIMIT $1
		)`, limit)
	if err != nil {
		return 0, fmt.Errorf("noncestore: expire due: %w", err)
	}
	return tag.RowsAffected(), nil
}
