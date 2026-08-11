package vendorauthrepositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	vendorauthmodels "cosmicforge/logistics/services/vendor-service/internal/features/auth/models"
)

type OTPChallengeRepository struct {
	db *pgxpool.Pool
}

func NewOTPChallengeRepository(db *pgxpool.Pool) *OTPChallengeRepository {
	return &OTPChallengeRepository{db: db}
}

const challengeColumns = `id, purpose, identifier_type, identifier_value, otp_hash, status, attempts,
	resend_count, generation, session_id, staff_id, vendor_id, step_up_purpose,
	expires_at, consumed_at, locked_at, created_at, updated_at`

func (r *OTPChallengeRepository) Create(ctx context.Context, c vendorauthmodels.OTPChallenge) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO vendor_otp_challenges
			(id, purpose, identifier_type, identifier_value, identifier_key, otp_hash, status,
			 attempts, resend_count, generation, session_id, staff_id, vendor_id, step_up_purpose, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,0,0,1,$8,$9,$10,$11,$12)`,
		c.ID, c.Purpose, c.IdentifierType, c.IdentifierValue, c.IdentifierKey(), c.OTPHash, vendorauthmodels.ChallengeStatusActive,
		c.SessionID, c.StaffID, c.VendorID, c.StepUpPurpose, c.ExpiresAt,
	)
	return err
}

// GetActiveByIdentifier loads the single currently-active challenge for a
// given identifier+purpose, if any. There can be at most one active row per
// (identifier_key, purpose) at a time — a resend updates the same row rather
// than creating a second.
func (r *OTPChallengeRepository) GetActiveByIdentifier(ctx context.Context, identifierKey, purpose string) (vendorauthmodels.OTPChallenge, bool, error) {
	row := r.db.QueryRow(ctx, `SELECT `+challengeColumns+`
		FROM vendor_otp_challenges WHERE identifier_key = $1 AND purpose = $2 AND status = $3`,
		identifierKey, purpose, vendorauthmodels.ChallengeStatusActive)
	c, err := scanChallenge(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return vendorauthmodels.OTPChallenge{}, false, nil
	}
	return c, err == nil, err
}

func (r *OTPChallengeRepository) GetByID(ctx context.Context, id string) (vendorauthmodels.OTPChallenge, error) {
	row := r.db.QueryRow(ctx, `SELECT `+challengeColumns+` FROM vendor_otp_challenges WHERE id = $1`, id)
	c, err := scanChallenge(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return vendorauthmodels.OTPChallenge{}, fmt.Errorf("otp challenge %s: %w", id, ErrNotFound)
	}
	return c, err
}

// Resend atomically bumps generation, replaces the hash, resets expiry, and
// increments resend_count — the previous code's hash is gone the instant this
// commits, so it becomes unusable immediately, not eventually.
func (r *OTPChallengeRepository) Resend(ctx context.Context, id string, newHash string, newExpiresAt time.Time) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE vendor_otp_challenges SET
			otp_hash = $1, generation = generation + 1, resend_count = resend_count + 1,
			attempts = 0, expires_at = $2, updated_at = now()
		WHERE id = $3 AND status = $4`,
		newHash, newExpiresAt, id, vendorauthmodels.ChallengeStatusActive)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("otp challenge %s is not active: %w", id, ErrNotFound)
	}
	return nil
}

// RecordFailedAttempt atomically increments attempts and locks the challenge
// once maxAttempts is reached, in one statement — no read-then-write race
// window between two concurrent wrong guesses.
func (r *OTPChallengeRepository) RecordFailedAttempt(ctx context.Context, id string, maxAttempts int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE vendor_otp_challenges SET
			attempts = attempts + 1,
			status = CASE WHEN attempts + 1 >= $2 THEN $3 ELSE status END,
			locked_at = CASE WHEN attempts + 1 >= $2 THEN now() ELSE locked_at END,
			updated_at = now()
		WHERE id = $1 AND status = $4`,
		id, maxAttempts, vendorauthmodels.ChallengeStatusLocked, vendorauthmodels.ChallengeStatusActive)
	return err
}

// Consume atomically transitions an active challenge to consumed, guarded so
// exactly one concurrent caller wins: RowsAffected()==0 means someone else
// (or an expiry/lock) already consumed/invalidated it first.
func (r *OTPChallengeRepository) Consume(ctx context.Context, id string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE vendor_otp_challenges SET status = $1, consumed_at = now(), updated_at = now()
		WHERE id = $2 AND status = $3`,
		vendorauthmodels.ChallengeStatusConsumed, id, vendorauthmodels.ChallengeStatusActive)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func scanChallenge(row pgx.Row) (vendorauthmodels.OTPChallenge, error) {
	var c vendorauthmodels.OTPChallenge
	err := row.Scan(&c.ID, &c.Purpose, &c.IdentifierType, &c.IdentifierValue, &c.OTPHash, &c.Status, &c.Attempts,
		&c.ResendCount, &c.Generation, &c.SessionID, &c.StaffID, &c.VendorID, &c.StepUpPurpose,
		&c.ExpiresAt, &c.ConsumedAt, &c.LockedAt, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}
