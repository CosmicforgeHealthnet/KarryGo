package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
)

// ChallengeRepository stores durable OTP challenges. Challenge status lives
// in PostgreSQL, never in Redis.
type ChallengeRepository struct {
	db database.Querier
}

// NewChallengeRepository wires the repository.
func NewChallengeRepository(db database.Querier) *ChallengeRepository {
	return &ChallengeRepository{db: db}
}

const challengeColumns = `id, identity_id, email_normalized, email_hash, phone_e164, phone_hash,
	purpose, status, code_digest, expires_at, resend_available_at, failed_attempts,
	maximum_attempts, delivery_attempts, delivery_status, notification_reference,
	session_id, organization_id, action_class, invitation_id, requested_ip_hash,
	user_agent_hash, request_id, correlation_id, verified_at, consumed_at, created_at, updated_at`

// Create inserts a challenge and supersedes any earlier usable challenge for
// the same contact pair and purpose, so only one code is ever live.
func (r *ChallengeRepository) Create(ctx context.Context, q database.Querier, challenge *models.Challenge) error {
	if _, err := q.Exec(ctx, `
		UPDATE fleet_partner_auth_otp_challenges SET status = 'SUPERSEDED', updated_at = now()
		WHERE email_hash = $1 AND phone_hash = $2 AND purpose = $3 AND status = 'PENDING'`,
		challenge.EmailHash, challenge.PhoneHash, challenge.Purpose,
	); err != nil {
		return fmt.Errorf("auth: supersede challenges: %w", err)
	}

	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_auth_otp_challenges
			(id, identity_id, email_normalized, email_hash, phone_e164, phone_hash, purpose, status,
			 code_digest, expires_at, resend_available_at, failed_attempts, maximum_attempts,
			 delivery_attempts, delivery_status, session_id, organization_id, action_class,
			 invitation_id, requested_ip_hash, user_agent_hash, request_id, correlation_id,
			 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'PENDING', $8, $9, $10, 0, $11, 0, 'PENDING',
			$12, $13, $14, $15, $16, $17, $18, $19, now(), now())`,
		challenge.ID, challenge.IdentityID, challenge.EmailNormalized, challenge.EmailHash,
		challenge.PhoneE164, challenge.PhoneHash, challenge.Purpose, challenge.CodeDigest,
		challenge.ExpiresAt, challenge.ResendAvailableAt, challenge.MaximumAttempts,
		challenge.SessionID, challenge.OrganizationID, challenge.ActionClass, challenge.InvitationID,
		challenge.RequestedIPHash, challenge.UserAgentHash, challenge.RequestID, challenge.CorrelationID,
	)
	if err != nil {
		return fmt.Errorf("auth: create challenge: %w", err)
	}
	return nil
}

// GetByID loads one challenge without locking.
func (r *ChallengeRepository) GetByID(ctx context.Context, challengeID string) (*models.Challenge, error) {
	row := r.db.QueryRow(ctx, `SELECT `+challengeColumns+`
		FROM fleet_partner_auth_otp_challenges WHERE id = $1`, challengeID)
	return scanChallenge(row)
}

// LockForVerification loads the challenge FOR UPDATE inside the caller's
// transaction. Concurrent verifications of the same challenge serialize here,
// so exactly one can consume it.
func (r *ChallengeRepository) LockForVerification(ctx context.Context, q database.Querier, challengeID string) (*models.Challenge, error) {
	row := q.QueryRow(ctx, `SELECT `+challengeColumns+`
		FROM fleet_partner_auth_otp_challenges WHERE id = $1 FOR UPDATE`, challengeID)
	return scanChallenge(row)
}

// RecordFailedAttempt increments the attempt counter and locks the challenge
// once the maximum is reached.
func (r *ChallengeRepository) RecordFailedAttempt(ctx context.Context, q database.Querier, challengeID string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_auth_otp_challenges SET
			failed_attempts = failed_attempts + 1,
			status = CASE WHEN failed_attempts + 1 >= maximum_attempts THEN 'LOCKED' ELSE status END,
			updated_at = now()
		WHERE id = $1`, challengeID)
	if err != nil {
		return fmt.Errorf("auth: record failed attempt: %w", err)
	}
	return nil
}

// Consume marks the challenge verified and consumed in one step. It is
// conditional on the challenge still being PENDING so a replay cannot consume
// it twice even if two transactions interleave.
func (r *ChallengeRepository) Consume(ctx context.Context, q database.Querier, challengeID string) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_auth_otp_challenges SET
			status = 'CONSUMED', verified_at = now(), consumed_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'PENDING'`, challengeID)
	if err != nil {
		return fmt.Errorf("auth: consume challenge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrChallengeNotUsable
	}
	return nil
}

// ErrChallengeNotUsable means the challenge was already consumed, expired,
// superseded or locked.
var ErrChallengeNotUsable = errors.New("auth: challenge is not usable")

// RecordDelivery stores the outcome of an OTP delivery attempt.
func (r *ChallengeRepository) RecordDelivery(ctx context.Context, challengeID string, status string, reference string) error {
	var ref *string
	if reference != "" {
		ref = &reference
	}
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_auth_otp_challenges SET
			delivery_attempts = delivery_attempts + 1,
			delivery_status = $2,
			notification_reference = COALESCE($3, notification_reference),
			updated_at = now()
		WHERE id = $1`, challengeID, status, ref)
	if err != nil {
		return fmt.Errorf("auth: record delivery: %w", err)
	}
	return nil
}

// Rotate replaces the live code of an existing challenge (resend). The
// destination contact pair and purpose are never changed here, so a resend
// cannot redirect a code to a different address.
func (r *ChallengeRepository) Rotate(ctx context.Context, q database.Querier, challengeID string, codeDigest string, expiresAt time.Time, resendAvailableAt time.Time) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_auth_otp_challenges SET
			code_digest = $2, expires_at = $3, resend_available_at = $4,
			failed_attempts = 0, updated_at = now()
		WHERE id = $1 AND status = 'PENDING'`,
		challengeID, codeDigest, expiresAt, resendAvailableAt)
	if err != nil {
		return fmt.Errorf("auth: rotate challenge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrChallengeNotUsable
	}
	return nil
}

// ExpireDue marks overdue pending challenges as expired; used by the worker.
func (r *ChallengeRepository) ExpireDue(ctx context.Context, limit int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_auth_otp_challenges SET status = 'EXPIRED', updated_at = now()
		WHERE id IN (
			SELECT id FROM fleet_partner_auth_otp_challenges
			WHERE status = 'PENDING' AND expires_at < now()
			LIMIT $1
		)`, limit)
	if err != nil {
		return 0, fmt.Errorf("auth: expire challenges: %w", err)
	}
	return tag.RowsAffected(), nil
}

func scanChallenge(row pgx.Row) (*models.Challenge, error) {
	var challenge models.Challenge
	err := row.Scan(
		&challenge.ID, &challenge.IdentityID, &challenge.EmailNormalized, &challenge.EmailHash,
		&challenge.PhoneE164, &challenge.PhoneHash, &challenge.Purpose, &challenge.Status,
		&challenge.CodeDigest, &challenge.ExpiresAt, &challenge.ResendAvailableAt,
		&challenge.FailedAttempts, &challenge.MaximumAttempts, &challenge.DeliveryAttempts,
		&challenge.DeliveryStatus, &challenge.NotificationReference, &challenge.SessionID,
		&challenge.OrganizationID, &challenge.ActionClass, &challenge.InvitationID,
		&challenge.RequestedIPHash, &challenge.UserAgentHash, &challenge.RequestID,
		&challenge.CorrelationID, &challenge.VerifiedAt, &challenge.ConsumedAt,
		&challenge.CreatedAt, &challenge.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth: scan challenge: %w", err)
	}
	return &challenge, nil
}
