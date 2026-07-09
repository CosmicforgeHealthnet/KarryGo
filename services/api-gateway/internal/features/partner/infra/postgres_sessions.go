package infra

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/api-gateway/internal/features/partneraccount"
)

// SessionRepo is the Postgres implementation of partneraccount.SessionRepository.
// Only the SHA-256 hash of the session JWT is stored, never the raw token.
type SessionRepo struct {
	pool *pgxpool.Pool
}

// NewSessionRepo constructs a SessionRepo.
func NewSessionRepo(pool *pgxpool.Pool) *SessionRepo { return &SessionRepo{pool: pool} }

func (r *SessionRepo) CreateSession(ctx context.Context, params partneraccount.CreateSessionParams) error {
	const q = `INSERT INTO partner_sessions (partner_id, token_hash, ip_address, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.pool.Exec(ctx, q, params.PartnerID, params.TokenHash,
		nilIfEmpty(params.IPAddress), nilIfEmpty(params.UserAgent), params.ExpiresAt)
	return err
}

func (r *SessionRepo) FindByTokenHash(ctx context.Context, tokenHash string) (*partneraccount.Session, error) {
	const q = `SELECT id, partner_id, token_hash, expires_at, revoked_at, created_at
		FROM partner_sessions WHERE token_hash = $1`
	var s partneraccount.Session
	err := r.pool.QueryRow(ctx, q, tokenHash).
		Scan(&s.ID, &s.PartnerID, &s.TokenHash, &s.ExpiresAt, &s.RevokedAt, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SessionRepo) Revoke(ctx context.Context, tokenHash string) error {
	const q = `UPDATE partner_sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`
	_, err := r.pool.Exec(ctx, q, tokenHash)
	return err
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
