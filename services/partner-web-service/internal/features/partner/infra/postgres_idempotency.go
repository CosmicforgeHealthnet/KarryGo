package infra

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/delivery"
)

// IdempotencyStore is the Postgres implementation of delivery.IdempotencyStore.
type IdempotencyStore struct {
	pool *pgxpool.Pool
}

// NewIdempotencyStore constructs an IdempotencyStore.
func NewIdempotencyStore(pool *pgxpool.Pool) *IdempotencyStore { return &IdempotencyStore{pool: pool} }

func (s *IdempotencyStore) Get(ctx context.Context, key string) (*delivery.IdempotencyRecord, error) {
	const q = `SELECT key, partner_id, request_hash, response, status_code, expires_at
		FROM partner_idempotency_keys WHERE key = $1`
	var (
		rec         delivery.IdempotencyRecord
		requestHash *string
	)
	err := s.pool.QueryRow(ctx, q, key).Scan(
		&rec.Key, &rec.PartnerID, &requestHash, &rec.Response, &rec.StatusCode, &rec.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if requestHash != nil {
		rec.RequestHash = *requestHash
	}
	return &rec, nil
}

func (s *IdempotencyStore) Put(ctx context.Context, rec delivery.IdempotencyRecord) error {
	// Overwrite an expired/existing key with the same composite key.
	const q = `INSERT INTO partner_idempotency_keys (key, partner_id, request_hash, response, status_code, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, now(), $6)
		ON CONFLICT (key) DO UPDATE SET
			request_hash = EXCLUDED.request_hash,
			response = EXCLUDED.response,
			status_code = EXCLUDED.status_code,
			created_at = now(),
			expires_at = EXCLUDED.expires_at`
	_, err := s.pool.Exec(ctx, q, rec.Key, rec.PartnerID, rec.RequestHash, rec.Response, rec.StatusCode, expiresOrDefault(rec.ExpiresAt))
	return err
}

func expiresOrDefault(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().Add(24 * time.Hour)
	}
	return t
}
