// Package repositories implements PostgreSQL persistence for corporate
// authentication. No query here reads or writes another service's database.
package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ErrNotFound is returned when a lookup matches nothing.
var ErrNotFound = errors.New("auth: not found")

// ErrIdentityPairConflict means the email and phone resolve to two different
// identities. It is never auto-resolved: one field must never silently take
// over another identity.
var ErrIdentityPairConflict = errors.New("auth: email and phone resolve to different identities")

// IdentityRepository stores corporate identities.
type IdentityRepository struct {
	db database.Querier
}

// NewIdentityRepository wires the repository.
func NewIdentityRepository(db database.Querier) *IdentityRepository {
	return &IdentityRepository{db: db}
}

const identityColumns = `id, email_normalized, email_display, phone_e164, phone_display, status,
	email_verified_at, phone_bound_at, last_authenticated_at, failed_authentication_count,
	locked_until, version, created_at, updated_at, archived_at`

// ResolvePair looks the identity pair up by email and phone.
//
// Outcomes:
//   - both match the same identity  -> that identity, nil
//   - neither matches              -> nil, ErrNotFound
//   - they match different rows, or only one matches an existing identity
//     -> nil, ErrIdentityPairConflict
//
// The half-match case is a conflict rather than a match because allowing it
// would let someone claim another person's identity by supplying a known
// email with their own phone (or vice versa).
func (r *IdentityRepository) ResolvePair(ctx context.Context, q database.Querier, emailNormalized string, phoneE164 string) (*models.Identity, error) {
	byEmail, err := r.findBy(ctx, q, "email_normalized", emailNormalized)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	byPhone, err := r.findBy(ctx, q, "phone_e164", phoneE164)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	switch {
	case byEmail == nil && byPhone == nil:
		return nil, ErrNotFound
	case byEmail != nil && byPhone != nil && byEmail.ID == byPhone.ID:
		return byEmail, nil
	default:
		return nil, ErrIdentityPairConflict
	}
}

func (r *IdentityRepository) findBy(ctx context.Context, q database.Querier, column string, value string) (*models.Identity, error) {
	if q == nil {
		q = r.db
	}
	// column is a fixed internal literal, never client input.
	row := q.QueryRow(ctx, `
		SELECT `+identityColumns+`
		FROM fleet_partner_corporate_identities
		WHERE `+column+` = $1 AND status <> 'ARCHIVED'`, value)
	return scanIdentity(row)
}

// GetByID loads one identity.
func (r *IdentityRepository) GetByID(ctx context.Context, identityID string) (*models.Identity, error) {
	row := r.db.QueryRow(ctx, `SELECT `+identityColumns+`
		FROM fleet_partner_corporate_identities WHERE id = $1`, identityID)
	return scanIdentity(row)
}

// Create inserts a new pending identity.
func (r *IdentityRepository) Create(ctx context.Context, q database.Querier, identity *models.Identity) error {
	if identity.ID == "" {
		identity.ID = ids.New("cid")
	}
	if identity.Status == "" {
		identity.Status = models.IdentityStatusPending
	}
	identity.Version = 1
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_corporate_identities
			(id, email_normalized, email_display, phone_e164, phone_display, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 1, now(), now())`,
		identity.ID, identity.EmailNormalized, identity.EmailDisplay,
		identity.PhoneE164, identity.PhoneDisplay, identity.Status,
	)
	if err != nil {
		return fmt.Errorf("auth: create identity: %w", err)
	}
	return nil
}

// MarkContactVerified activates the identity and records that the emailed OTP
// was verified and the phone bound. CONTACT_VERIFIED means exactly this.
func (r *IdentityRepository) MarkContactVerified(ctx context.Context, q database.Querier, identityID string, expectedVersion int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_corporate_identities SET
			status = CASE WHEN status = 'PENDING' THEN 'ACTIVE' ELSE status END,
			email_verified_at = COALESCE(email_verified_at, now()),
			phone_bound_at = COALESCE(phone_bound_at, now()),
			last_authenticated_at = now(),
			failed_authentication_count = 0,
			locked_until = NULL,
			version = version + 1,
			updated_at = now()
		WHERE id = $1 AND version = $2`,
		identityID, expectedVersion,
	)
	if err != nil {
		return fmt.Errorf("auth: mark contact verified: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// RecordFailedAuthentication increments the failure counter and locks the
// identity once the threshold is reached.
func (r *IdentityRepository) RecordFailedAuthentication(ctx context.Context, identityID string, threshold int, lockFor time.Duration) error {
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_corporate_identities SET
			failed_authentication_count = failed_authentication_count + 1,
			locked_until = CASE
				WHEN failed_authentication_count + 1 >= $2 THEN now() + make_interval(secs => $3::double precision)
				ELSE locked_until END,
			updated_at = now()
		WHERE id = $1`,
		identityID, threshold, lockFor.Seconds(),
	)
	if err != nil {
		return fmt.Errorf("auth: record failed authentication: %w", err)
	}
	return nil
}

// UpdateStatus performs a compare-and-swap identity status change.
func (r *IdentityRepository) UpdateStatus(ctx context.Context, q database.Querier, identityID string, status string, expectedVersion int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_corporate_identities SET
			status = $1, version = version + 1, updated_at = now(),
			archived_at = CASE WHEN $1 = 'ARCHIVED' THEN now() ELSE archived_at END
		WHERE id = $2 AND version = $3`,
		status, identityID, expectedVersion,
	)
	if err != nil {
		return fmt.Errorf("auth: update identity status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

func scanIdentity(row pgx.Row) (*models.Identity, error) {
	var identity models.Identity
	err := row.Scan(
		&identity.ID, &identity.EmailNormalized, &identity.EmailDisplay,
		&identity.PhoneE164, &identity.PhoneDisplay, &identity.Status,
		&identity.EmailVerifiedAt, &identity.PhoneBoundAt, &identity.LastAuthenticatedAt,
		&identity.FailedAuthenticationCount, &identity.LockedUntil, &identity.Version,
		&identity.CreatedAt, &identity.UpdatedAt, &identity.ArchivedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth: scan identity: %w", err)
	}
	return &identity, nil
}
