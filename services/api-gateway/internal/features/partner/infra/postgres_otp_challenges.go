package infra

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/api-gateway/internal/features/partneraccount"
)

// OTPChallengeRepo is the Postgres implementation of partneraccount.OTPChallengeRepository.
type OTPChallengeRepo struct {
	pool *pgxpool.Pool
}

func NewOTPChallengeRepo(pool *pgxpool.Pool) *OTPChallengeRepo { return &OTPChallengeRepo{pool: pool} }

func (r *OTPChallengeRepo) CreateOTPChallenge(ctx context.Context, params partneraccount.CreateOTPChallengeParams) (partneraccount.OTPChallenge, error) {
	const q = `INSERT INTO partner_otp_challenges (business_email, otp_code_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id::text, business_email, otp_code_hash, attempts, consumed_at, expires_at, created_at`
	var c partneraccount.OTPChallenge
	err := r.pool.QueryRow(ctx, q, params.BusinessEmail, params.OTPCodeHash, params.ExpiresAt).
		Scan(&c.ID, &c.BusinessEmail, &c.OTPCodeHash, &c.Attempts, &c.ConsumedAt, &c.ExpiresAt, &c.CreatedAt)
	return c, err
}

func (r *OTPChallengeRepo) FindLatestOTPChallenge(ctx context.Context, businessEmail string) (*partneraccount.OTPChallenge, error) {
	const q = `SELECT id::text, business_email, otp_code_hash, attempts, consumed_at, expires_at, created_at
		FROM partner_otp_challenges
		WHERE business_email = $1
		ORDER BY created_at DESC
		LIMIT 1`
	var c partneraccount.OTPChallenge
	err := r.pool.QueryRow(ctx, q, businessEmail).
		Scan(&c.ID, &c.BusinessEmail, &c.OTPCodeHash, &c.Attempts, &c.ConsumedAt, &c.ExpiresAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *OTPChallengeRepo) IncrementOTPAttempts(ctx context.Context, challengeID string) (int, error) {
	const q = `UPDATE partner_otp_challenges SET attempts = attempts + 1 WHERE id = $1 RETURNING attempts`
	var attempts int
	if err := r.pool.QueryRow(ctx, q, challengeID).Scan(&attempts); err != nil {
		return 0, err
	}
	return attempts, nil
}

func (r *OTPChallengeRepo) ConsumeOTPChallenge(ctx context.Context, challengeID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE partner_otp_challenges SET consumed_at = now() WHERE id = $1 AND consumed_at IS NULL`, challengeID)
	return err
}

func (r *OTPChallengeRepo) DeleteOTPChallenge(ctx context.Context, challengeID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM partner_otp_challenges WHERE id = $1`, challengeID)
	return err
}
