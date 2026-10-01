package providerauthrepositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	providerauthmodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// providerColumns is the shared select/returning column list; scanProvider must
// match its order exactly.
const providerColumns = `id::text, COALESCE(phone,''), COALESCE(email,''),
	          first_name, last_name, onboarding_status, status,
	          profile_photo_url, photo_asset_id,
	          COALESCE(rating,5.00), total_trips, created_at, updated_at,
	          location_state, location_city, language, service_type, operation_mode,
	          driver_license_number, license_expiry_year, license_expiry_date,
	          gov_id_url, driver_license_url, vehicle_reg_url,
	          guarantor_name, guarantor_phone,
	          emergency_contact_name, emergency_contact_phone, emergency_contact_relationship`

// ─── Provider Repository ──────────────────────────────────────────────────────

type ProviderRepository interface {
	// UpsertByPhone and UpsertByEmail return the provider for an identifier,
	// creating it on first sight. signup is announced to the referral
	// programme only when the account is actually created.
	UpsertByPhone(ctx context.Context, phone string, signup providerauthmodels.Signup) (providerauthmodels.Provider, error)
	UpsertByEmail(ctx context.Context, email string, signup providerauthmodels.Signup) (providerauthmodels.Provider, error)
	GetByID(ctx context.Context, id string) (providerauthmodels.Provider, error)
	UpdatePhone(ctx context.Context, id, phone string) (providerauthmodels.Provider, error)
	// ContactTaken reports whether the given email and/or phone already belong to a
	// provider other than excludeID. Empty inputs report false for that field.
	ContactTaken(ctx context.Context, excludeID, email, phone string) (emailTaken, phoneTaken bool, err error)
}

// SignupAnnouncer records a newly created provider for the referral
// programme inside the account-creation transaction, so the account and its
// announcement commit together or not at all.
type SignupAnnouncer interface {
	AnnounceProvider(ctx context.Context, tx pgx.Tx, provider providerauthmodels.Provider, signup providerauthmodels.Signup) error
}

type PostgresProviderRepository struct {
	db        *pgxpool.Pool
	announcer SignupAnnouncer
}

func NewPostgresProviderRepository(db *pgxpool.Pool) *PostgresProviderRepository {
	return &PostgresProviderRepository{db: db}
}

// SetSignupAnnouncer switches the referral announcement on (non-nil) or off.
func (r *PostgresProviderRepository) SetSignupAnnouncer(announcer SignupAnnouncer) {
	r.announcer = announcer
}

func (r *PostgresProviderRepository) UpsertByPhone(ctx context.Context, phone string, signup providerauthmodels.Signup) (providerauthmodels.Provider, error) {
	return r.upsert(ctx, "phone", phone, signup)
}

func (r *PostgresProviderRepository) UpsertByEmail(ctx context.Context, email string, signup providerauthmodels.Signup) (providerauthmodels.Provider, error) {
	return r.upsert(ctx, "email", email, signup)
}

// upsert creates or returns the provider for one identifier column. The
// returned id equals the id generated here only when this call inserted the
// row, which is how a new sign-up is told apart from a returning login.
func (r *PostgresProviderRepository) upsert(ctx context.Context, column string, value string, signup providerauthmodels.Signup) (providerauthmodels.Provider, error) {
	if column != "phone" && column != "email" {
		return providerauthmodels.Provider{}, errors.New("unsupported identifier column")
	}
	id := uuid.NewString()
	var provider providerauthmodels.Provider
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO truck_providers (id, `+column+`, onboarding_status, status)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (`+column+`) DO UPDATE SET updated_at = now()
			RETURNING `+providerColumns+`
		`, id, value, providerauthmodels.OnboardingProfileNeeded, providerauthmodels.ProviderStatusActive)
		var err error
		if provider, err = scanProvider(row); err != nil {
			return err
		}
		if provider.ID == id && r.announcer != nil {
			return r.announcer.AnnounceProvider(ctx, tx, provider, signup)
		}
		return nil
	})
	return provider, err
}

func (r *PostgresProviderRepository) GetByID(ctx context.Context, id string) (providerauthmodels.Provider, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+providerColumns+`
		FROM truck_providers WHERE id = $1
	`, id)

	p, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerauthmodels.Provider{}, apperrors.NotFound("Provider could not be found.", err)
	}
	return p, err
}

func (r *PostgresProviderRepository) UpdatePhone(ctx context.Context, id, phone string) (providerauthmodels.Provider, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE truck_providers SET phone = $2, updated_at = now()
		WHERE id = $1
		RETURNING `+providerColumns+`
	`, id, phone)

	p, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerauthmodels.Provider{}, apperrors.NotFound("Provider could not be found.", err)
	}
	return p, err
}

func (r *PostgresProviderRepository) ContactTaken(ctx context.Context, excludeID, email, phone string) (bool, bool, error) {
	var emailTaken, phoneTaken bool
	err := r.db.QueryRow(ctx, `
		SELECT
		  CASE WHEN $2 = '' THEN false ELSE EXISTS(
		    SELECT 1 FROM truck_providers WHERE id <> $1 AND lower(email) = lower($2)
		  ) END,
		  CASE WHEN $3 = '' THEN false ELSE EXISTS(
		    SELECT 1 FROM truck_providers WHERE id <> $1 AND phone = $3
		  ) END
	`, excludeID, email, phone).Scan(&emailTaken, &phoneTaken)
	return emailTaken, phoneTaken, err
}

// ─── Session Repository ───────────────────────────────────────────────────────

type RefreshSessionRepository interface {
	Create(ctx context.Context, session providerauthmodels.RefreshSession) error
	GetByID(ctx context.Context, id string) (providerauthmodels.RefreshSession, error)
	Revoke(ctx context.Context, id string) error
}

type PostgresRefreshSessionRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRefreshSessionRepository(db *pgxpool.Pool) *PostgresRefreshSessionRepository {
	return &PostgresRefreshSessionRepository{db: db}
}

func (r *PostgresRefreshSessionRepository) Create(ctx context.Context, session providerauthmodels.RefreshSession) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO provider_sessions
		  (id, provider_id, refresh_token_hash, device_id, user_agent, ip_address, expires_at)
		VALUES ($1, $2, $3, COALESCE($4, ''), $5, $6, $7)
	`, session.ID, session.ProviderID, session.RefreshTokenHash, session.DeviceID, session.UserAgent, session.IPAddress, session.ExpiresAt)
	return err
}

func (r *PostgresRefreshSessionRepository) GetByID(ctx context.Context, id string) (providerauthmodels.RefreshSession, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, provider_id::text, refresh_token_hash, device_id,
		       user_agent, ip_address, expires_at, revoked_at, created_at
		FROM provider_sessions WHERE id = $1
	`, id)

	var s providerauthmodels.RefreshSession
	err := row.Scan(&s.ID, &s.ProviderID, &s.RefreshTokenHash, &s.DeviceID,
		&s.UserAgent, &s.IPAddress, &s.ExpiresAt, &s.RevokedAt, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerauthmodels.RefreshSession{}, apperrors.Unauthorized("Your session has expired. Please sign in again.", err)
	}
	return s, err
}

func (r *PostgresRefreshSessionRepository) Revoke(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE provider_sessions SET revoked_at = COALESCE(revoked_at, now()) WHERE id = $1
	`, id)
	return err
}

// ─── helpers ──────────────────────────────────────────────────────────────────

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanProvider(row scannable) (providerauthmodels.Provider, error) {
	var p providerauthmodels.Provider
	err := row.Scan(
		&p.ID, &p.Phone, &p.Email,
		&p.FirstName, &p.LastName, &p.OnboardingStatus, &p.Status,
		&p.ProfilePhotoURL, &p.PhotoAssetID,
		&p.Rating, &p.TotalTrips, &p.CreatedAt, &p.UpdatedAt,
		&p.LocationState, &p.LocationCity, &p.Language, &p.ServiceType, &p.OperationMode,
		&p.DriverLicenseNumber, &p.LicenseExpiryYear, &p.LicenseExpiryDate,
		&p.GovIDURL, &p.DriverLicenseURL, &p.VehicleRegURL,
		&p.GuarantorName, &p.GuarantorPhone,
		&p.EmergencyContactName, &p.EmergencyContactPhone, &p.EmergencyContactRelationship,
	)
	return p, err
}
