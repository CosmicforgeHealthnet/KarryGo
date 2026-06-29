package profile

import (
	"context"
	"errors"

	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	EnsureProvider(ctx context.Context, providerID string, phone string) (Provider, error)
	GetProviderByID(ctx context.Context, providerID string) (Provider, bool, error)
	UpdateOnboarding(ctx context.Context, providerID string, input OnboardingInput) (Provider, error)
	PatchProvider(ctx context.Context, providerID string, input UpdateProviderInput) (Provider, error)
	UpsertEmergencyContact(ctx context.Context, providerID string, input EmergencyContactInput) (EmergencyContact, error)
	GetEmergencyContact(ctx context.Context, providerID string) (EmergencyContact, bool, error)
	UpsertGuarantor(ctx context.Context, providerID string, input GuarantorInput) (Guarantor, error)
	GetGuarantor(ctx context.Context, providerID string) (Guarantor, bool, error)
	RecalculateOnboardingComplete(ctx context.Context, providerID string) (Provider, error)
	CountRatings(ctx context.Context, providerID string) (int, error)
	UpdateVerificationStatus(ctx context.Context, providerID string, status VerificationStatus) error
	IncrementTotalTrips(ctx context.Context, providerID string) error
	InsertRatingAndRecalculate(ctx context.Context, input RatingInput) (bool, error)
	GetOrCreateSettings(ctx context.Context, providerID string) (ProviderSettings, error)
	UpdateSettings(ctx context.Context, providerID string, input UpdateSettingsInput) (ProviderSettings, error)
	UpdateProfilePhotoURL(ctx context.Context, providerID string, url string) error
	DeactivateProvider(ctx context.Context, providerID string) error
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) EnsureProvider(ctx context.Context, providerID string, phone string) (Provider, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO providers (id, phone, verification_status)
		VALUES ($1, $2, 'unverified')
		ON CONFLICT (id) DO UPDATE
		SET phone = COALESCE(NULLIF(providers.phone, ''), EXCLUDED.phone)
		RETURNING id, phone, full_name, email, state, city, country, profile_photo_url,
			operation_type, verification_status, avg_rating, total_trips, is_active,
			onboarding_complete, created_at, updated_at
	`, providerID, phone)
	return scanProvider(row)
}

func (r *PostgresRepository) GetProviderByID(ctx context.Context, providerID string) (Provider, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, phone, full_name, email, state, city, country, profile_photo_url,
			operation_type, verification_status, avg_rating, total_trips, is_active,
			onboarding_complete, created_at, updated_at
		FROM providers
		WHERE id = $1
	`, providerID)
	provider, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Provider{}, false, nil
	}
	return provider, err == nil, err
}

func (r *PostgresRepository) UpdateOnboarding(ctx context.Context, providerID string, input OnboardingInput) (Provider, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE providers
		SET full_name = $2,
			email = $3,
			state = $4,
			city = $5,
			operation_type = $6,
			updated_at = now()
		WHERE id = $1
		RETURNING id, phone, full_name, email, state, city, country, profile_photo_url,
			operation_type, verification_status, avg_rating, total_trips, is_active,
			onboarding_complete, created_at, updated_at
	`, providerID, input.FullName, input.Email, input.State, input.City, input.OperationType)
	return scanProvider(row)
}

func (r *PostgresRepository) PatchProvider(ctx context.Context, providerID string, input UpdateProviderInput) (Provider, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE providers
		SET full_name = COALESCE($2, full_name),
			email = COALESCE($3, email),
			state = COALESCE($4, state),
			city = COALESCE($5, city),
			profile_photo_url = COALESCE($6, profile_photo_url),
			updated_at = now()
		WHERE id = $1
		RETURNING id, phone, full_name, email, state, city, country, profile_photo_url,
			operation_type, verification_status, avg_rating, total_trips, is_active,
			onboarding_complete, created_at, updated_at
	`, providerID, input.FullName, input.Email, input.State, input.City, input.ProfilePhotoURL)
	return scanProvider(row)
}

func (r *PostgresRepository) UpsertEmergencyContact(ctx context.Context, providerID string, input EmergencyContactInput) (EmergencyContact, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO emergency_contacts (provider_id, full_name, phone, relationship)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (provider_id) DO UPDATE
		SET full_name = EXCLUDED.full_name,
			phone = EXCLUDED.phone,
			relationship = EXCLUDED.relationship,
			updated_at = now()
		RETURNING id, provider_id, full_name, phone, relationship, created_at, updated_at
	`, providerID, input.FullName, input.Phone, input.Relationship)
	return scanEmergencyContact(row)
}

func (r *PostgresRepository) GetEmergencyContact(ctx context.Context, providerID string) (EmergencyContact, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, provider_id, full_name, phone, relationship, created_at, updated_at
		FROM emergency_contacts
		WHERE provider_id = $1
	`, providerID)
	contact, err := scanEmergencyContact(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return EmergencyContact{}, false, nil
	}
	return contact, err == nil, err
}

func (r *PostgresRepository) UpsertGuarantor(ctx context.Context, providerID string, input GuarantorInput) (Guarantor, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO guarantors (provider_id, full_name, phone)
		VALUES ($1, $2, $3)
		ON CONFLICT (provider_id) DO UPDATE
		SET full_name = EXCLUDED.full_name,
			phone = EXCLUDED.phone,
			updated_at = now()
		RETURNING id, provider_id, full_name, phone, created_at, updated_at
	`, providerID, input.FullName, input.Phone)
	return scanGuarantor(row)
}

func (r *PostgresRepository) GetGuarantor(ctx context.Context, providerID string) (Guarantor, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, provider_id, full_name, phone, created_at, updated_at
		FROM guarantors
		WHERE provider_id = $1
	`, providerID)
	guarantor, err := scanGuarantor(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Guarantor{}, false, nil
	}
	return guarantor, err == nil, err
}

func (r *PostgresRepository) RecalculateOnboardingComplete(ctx context.Context, providerID string) (Provider, error) {
	row := r.db.QueryRow(ctx, `
		WITH checked AS (
			SELECT p.id,
				(
					COALESCE(NULLIF(TRIM(p.full_name), ''), '') <> ''
					AND COALESCE(NULLIF(TRIM(p.state), ''), '') <> ''
					AND COALESCE(NULLIF(TRIM(p.city), ''), '') <> ''
					AND p.operation_type IN ('individual', 'fleet')
					AND EXISTS (SELECT 1 FROM emergency_contacts ec WHERE ec.provider_id = p.id)
					AND EXISTS (SELECT 1 FROM guarantors g WHERE g.provider_id = p.id)
				) AS complete
			FROM providers p
			WHERE p.id = $1
		),
		updated AS (
			UPDATE providers p
			SET onboarding_complete = true,
				updated_at = now()
			FROM checked
			WHERE p.id = checked.id
				AND p.onboarding_complete = false
				AND checked.complete = true
			RETURNING p.id, p.phone, p.full_name, p.email, p.state, p.city, p.country, p.profile_photo_url,
				p.operation_type, p.verification_status, p.avg_rating, p.total_trips, p.is_active,
				p.onboarding_complete, p.created_at, p.updated_at
		)
		SELECT id, phone, full_name, email, state, city, country, profile_photo_url,
			operation_type, verification_status, avg_rating, total_trips, is_active,
			onboarding_complete, created_at, updated_at
		FROM updated
		UNION ALL
		SELECT id, phone, full_name, email, state, city, country, profile_photo_url,
			operation_type, verification_status, avg_rating, total_trips, is_active,
			onboarding_complete, created_at, updated_at
		FROM providers
		WHERE id = $1
			AND NOT EXISTS (SELECT 1 FROM updated)
	`, providerID)
	return scanProvider(row)
}

func (r *PostgresRepository) CountRatings(ctx context.Context, providerID string) (int, error) {
	row := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM ratings WHERE provider_id = $1
	`, providerID)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PostgresRepository) UpdateVerificationStatus(ctx context.Context, providerID string, status VerificationStatus) error {
	_, err := r.db.Exec(ctx, `
		UPDATE providers
		SET verification_status = $1,
			is_active = CASE WHEN $1 = 'suspended' THEN false ELSE is_active END,
			updated_at = now()
		WHERE id = $2
	`, status, providerID)
	return err
}

func (r *PostgresRepository) IncrementTotalTrips(ctx context.Context, providerID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE providers
		SET total_trips = total_trips + 1,
			updated_at = now()
		WHERE id = $1
	`, providerID)
	return err
}

func (r *PostgresRepository) InsertRatingAndRecalculate(ctx context.Context, input RatingInput) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		INSERT INTO ratings (provider_id, booking_id, rated_by_customer_id, score, comment)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (booking_id) DO NOTHING
	`, input.ProviderID, input.BookingID, input.RatedByCustomerID, input.Score, input.Comment)
	if err != nil {
		return false, err
	}

	inserted := tag.RowsAffected() == 1
	if inserted {
		if _, err := tx.Exec(ctx, `
			UPDATE providers
			SET avg_rating = COALESCE((
					SELECT ROUND(AVG(score)::numeric, 2)
					FROM ratings
					WHERE provider_id = $1
				), 0.00),
				updated_at = now()
			WHERE id = $1
		`, input.ProviderID); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return inserted, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProvider(row scanner) (Provider, error) {
	var provider Provider
	err := row.Scan(
		&provider.ID,
		&provider.Phone,
		&provider.FullName,
		&provider.Email,
		&provider.State,
		&provider.City,
		&provider.Country,
		&provider.ProfilePhotoURL,
		&provider.OperationType,
		&provider.VerificationStatus,
		&provider.AvgRating,
		&provider.TotalTrips,
		&provider.IsActive,
		&provider.OnboardingComplete,
		&provider.CreatedAt,
		&provider.UpdatedAt,
	)
	return provider, err
}

func scanEmergencyContact(row scanner) (EmergencyContact, error) {
	var contact EmergencyContact
	err := row.Scan(
		&contact.ID,
		&contact.ProviderID,
		&contact.FullName,
		&contact.Phone,
		&contact.Relationship,
		&contact.CreatedAt,
		&contact.UpdatedAt,
	)
	return contact, err
}

func (r *PostgresRepository) GetOrCreateSettings(ctx context.Context, providerID string) (ProviderSettings, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO provider_settings (provider_id)
		VALUES ($1)
		ON CONFLICT (provider_id) DO UPDATE
		SET updated_at = provider_settings.updated_at
		RETURNING provider_id, push_enabled, sms_enabled, language, dark_mode_enabled, updated_at
	`, providerID)
	return scanProviderSettings(row)
}

func (r *PostgresRepository) UpdateSettings(ctx context.Context, providerID string, input UpdateSettingsInput) (ProviderSettings, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE provider_settings
		SET push_enabled     = COALESCE($2, push_enabled),
			sms_enabled      = COALESCE($3, sms_enabled),
			language         = COALESCE($4, language),
			dark_mode_enabled = COALESCE($5, dark_mode_enabled),
			updated_at       = now()
		WHERE provider_id = $1
		RETURNING provider_id, push_enabled, sms_enabled, language, dark_mode_enabled, updated_at
	`, providerID, input.PushEnabled, input.SMSEnabled, input.Language, input.DarkModeEnabled)
	return scanProviderSettings(row)
}

func (r *PostgresRepository) UpdateProfilePhotoURL(ctx context.Context, providerID string, url string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE providers
		SET profile_photo_url = $2,
			updated_at        = now()
		WHERE id = $1
	`, providerID, url)
	return err
}

func (r *PostgresRepository) DeactivateProvider(ctx context.Context, providerID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE providers
		SET is_active   = false,
			updated_at  = now()
		WHERE id = $1
	`, providerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("Provider was not found.", nil)
	}
	return nil
}

func scanProviderSettings(row scanner) (ProviderSettings, error) {
	var s ProviderSettings
	err := row.Scan(&s.ProviderID, &s.PushEnabled, &s.SMSEnabled, &s.Language, &s.DarkModeEnabled, &s.UpdatedAt)
	return s, err
}

func scanGuarantor(row scanner) (Guarantor, error) {
	var guarantor Guarantor
	err := row.Scan(
		&guarantor.ID,
		&guarantor.ProviderID,
		&guarantor.FullName,
		&guarantor.Phone,
		&guarantor.CreatedAt,
		&guarantor.UpdatedAt,
	)
	return guarantor, err
}
