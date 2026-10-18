package infra

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/api-gateway/internal/features/partneraccount"
)

// SessionRepo is the Postgres implementation of partneraccount.SessionRepository.
// Only SHA-256 credential digests are stored, never raw access/refresh tokens.
type SessionRepo struct {
	pool interface {
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
		QueryRow(context.Context, string, ...any) pgx.Row
	}
}

// NewSessionRepo constructs a SessionRepo.
func NewSessionRepo(pool *pgxpool.Pool) *SessionRepo { return &SessionRepo{pool: pool} }

func (r *SessionRepo) CreateSession(ctx context.Context, params partneraccount.CreateSessionParams) error {
	const q = `INSERT INTO partner_sessions (partner_id, token_hash, ip_address, user_agent, expires_at, refresh_token_hash, id)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6,''), COALESCE(NULLIF($7,'')::uuid,gen_random_uuid()))`
	_, err := r.pool.Exec(ctx, q, params.PartnerID, params.TokenHash,
		nilIfEmpty(params.IPAddress), nilIfEmpty(params.UserAgent), params.ExpiresAt, params.RefreshHash, params.ID)
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

func (r *SessionRepo) FindByRefreshHash(ctx context.Context, hash string) (*partneraccount.Session, error) {
	var s partneraccount.Session
	err := r.pool.QueryRow(ctx, `SELECT id,partner_id,token_hash,refresh_token_hash,expires_at,revoked_at,created_at FROM partner_sessions WHERE refresh_token_hash=$1`, hash).Scan(&s.ID, &s.PartnerID, &s.TokenHash, &s.RefreshHash, &s.ExpiresAt, &s.RevokedAt, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}
func (r *SessionRepo) Rotate(ctx context.Context, old string, next partneraccount.CreateSessionParams, now time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE partner_sessions SET token_hash=$2,refresh_token_hash=$3,expires_at=$4 WHERE refresh_token_hash=$1 AND revoked_at IS NULL AND expires_at>$5 AND partner_id=$6 AND id=$7`, old, next.TokenHash, next.RefreshHash, next.ExpiresAt, now, next.PartnerID, next.ID)
	return tag.RowsAffected() == 1, err
}
func (r *SessionRepo) RevokeSession(ctx context.Context, sessionID string) (bool, error) {
	// Both this update and rotation serialize on the SAME row, independent of
	// the rotating digest. Repeated logout preserves the original revocation.
	tag, err := r.pool.Exec(ctx, `UPDATE partner_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1`, sessionID)
	return tag.RowsAffected() == 1, err
}
