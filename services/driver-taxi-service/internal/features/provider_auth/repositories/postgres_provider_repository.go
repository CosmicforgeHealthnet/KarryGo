package providerauthrepositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// providerColumns is the shared select/returning column list; scanProvider must
// match its order exactly. Kept as one constant so adding a column cannot leave
// one query out of step with the scanner.
const providerColumns = `id::text, COALESCE(phone,''), COALESCE(email,''),
	          first_name, last_name,
	          account_status, onboarding_status, identity_verification_status,
	          vehicle_operational_status, vehicle_verification_status, document_compliance_status,
	          profile_photo_url, photo_asset_id,
	          COALESCE(rating,5.00), total_trips, created_at, updated_at,
	          location_state, location_city, language, service_type, operation_mode,
	          driver_license_number, license_expiry_date,
	          gov_id_url, driver_license_url, vehicle_reg_url,
	          guarantor_name, guarantor_phone,
	          emergency_contact_name, emergency_contact_phone, emergency_contact_relationship,
	          accepts_parcels`

// ─── Provider Repository ──────────────────────────────────────────────────────

type ProviderRepository interface {
	// UpsertByPhone creates the driver on first login. Taxi driver auth is
	// phone-only; there is no email path.
	UpsertByPhone(ctx context.Context, phone string) (providerauthmodels.Provider, error)
	GetByID(ctx context.Context, id string) (providerauthmodels.Provider, error)
	UpdatePhone(ctx context.Context, id, phone string) (providerauthmodels.Provider, error)
	// ContactTaken reports whether the given email and/or phone already belong to
	// a driver other than excludeID. Empty inputs report false for that field.
	ContactTaken(ctx context.Context, excludeID, email, phone string) (emailTaken, phoneTaken bool, err error)
}

type PostgresProviderRepository struct {
	db *pgxpool.Pool
}

func NewPostgresProviderRepository(db *pgxpool.Pool) *PostgresProviderRepository {
	return &PostgresProviderRepository{db: db}
}

func (r *PostgresProviderRepository) UpsertByPhone(ctx context.Context, phone string) (providerauthmodels.Provider, error) {
	id := uuid.NewString()
	// New driver rows start at the "nothing done yet" value on every one of the
	// six status dimensions. Only onboarding_status and account_status are set
	// explicitly here because the other four already default correctly in the
	// schema (001_taxi_core.sql) -- naming them again would just be two places
	// to keep in sync instead of one.
	row := r.db.QueryRow(ctx, `
		INSERT INTO taxi_providers (id, phone, onboarding_status, account_status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (phone) DO UPDATE SET updated_at = now()
		RETURNING `+providerColumns+`
	`, id, phone, providerauthmodels.OnboardingProfileNeeded, providerauthmodels.AccountStatusActive)

	return scanProvider(row)
}

func (r *PostgresProviderRepository) GetByID(ctx context.Context, id string) (providerauthmodels.Provider, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+providerColumns+`
		FROM taxi_providers WHERE id = $1
	`, id)

	p, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerauthmodels.Provider{}, apperrors.NotFound("Driver could not be found.", err)
	}
	return p, err
}

func (r *PostgresProviderRepository) UpdatePhone(ctx context.Context, id, phone string) (providerauthmodels.Provider, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE taxi_providers SET phone = $2, updated_at = now()
		WHERE id = $1
		RETURNING `+providerColumns+`
	`, id, phone)

	p, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerauthmodels.Provider{}, apperrors.NotFound("Driver could not be found.", err)
	}
	return p, err
}

func (r *PostgresProviderRepository) ContactTaken(ctx context.Context, excludeID, email, phone string) (bool, bool, error) {
	var emailTaken, phoneTaken bool
	err := r.db.QueryRow(ctx, `
		SELECT
		  CASE WHEN $2 = '' THEN false ELSE EXISTS(
		    SELECT 1 FROM taxi_providers WHERE id <> $1 AND lower(email) = lower($2)
		  ) END,
		  CASE WHEN $3 = '' THEN false ELSE EXISTS(
		    SELECT 1 FROM taxi_providers WHERE id <> $1 AND phone = $3
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
		&p.FirstName, &p.LastName,
		&p.AccountStatus, &p.OnboardingStatus, &p.IdentityVerificationStatus,
		&p.VehicleOperationalStatus, &p.VehicleVerificationStatus, &p.DocumentComplianceStatus,
		&p.ProfilePhotoURL, &p.PhotoAssetID,
		&p.Rating, &p.TotalTrips, &p.CreatedAt, &p.UpdatedAt,
		&p.LocationState, &p.LocationCity, &p.Language, &p.ServiceType, &p.OperationMode,
		&p.DriverLicenseNumber, &p.LicenseExpiryDate,
		&p.GovIDURL, &p.DriverLicenseURL, &p.VehicleRegURL,
		&p.GuarantorName, &p.GuarantorPhone,
		&p.EmergencyContactName, &p.EmergencyContactPhone, &p.EmergencyContactRelationship,
		&p.AcceptsParcels,
	)
	return p, err
}
