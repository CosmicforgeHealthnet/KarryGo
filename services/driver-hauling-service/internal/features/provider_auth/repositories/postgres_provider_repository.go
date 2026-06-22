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

// ─── Provider Repository ──────────────────────────────────────────────────────

type ProviderRepository interface {
	UpsertByPhone(ctx context.Context, phone string) (providerauthmodels.Provider, error)
	UpsertByEmail(ctx context.Context, email string) (providerauthmodels.Provider, error)
	GetByID(ctx context.Context, id string) (providerauthmodels.Provider, error)
}

type PostgresProviderRepository struct {
	db *pgxpool.Pool
}

func NewPostgresProviderRepository(db *pgxpool.Pool) *PostgresProviderRepository {
	return &PostgresProviderRepository{db: db}
}

func (r *PostgresProviderRepository) UpsertByPhone(ctx context.Context, phone string) (providerauthmodels.Provider, error) {
	id := uuid.NewString()
	row := r.db.QueryRow(ctx, `
		INSERT INTO truck_providers (id, phone, onboarding_status, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (phone) DO UPDATE SET updated_at = now()
		RETURNING id::text, COALESCE(phone,''), COALESCE(email,''),
		          first_name, last_name, onboarding_status, status,
		          profile_photo_url, photo_asset_id,
		          COALESCE(rating,5.00), total_trips, created_at, updated_at
	`, id, phone, providerauthmodels.OnboardingProfileNeeded, providerauthmodels.ProviderStatusActive)

	return scanProvider(row)
}

func (r *PostgresProviderRepository) UpsertByEmail(ctx context.Context, email string) (providerauthmodels.Provider, error) {
	id := uuid.NewString()
	row := r.db.QueryRow(ctx, `
		INSERT INTO truck_providers (id, email, onboarding_status, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE SET updated_at = now()
		RETURNING id::text, COALESCE(phone,''), COALESCE(email,''),
		          first_name, last_name, onboarding_status, status,
		          profile_photo_url, photo_asset_id,
		          COALESCE(rating,5.00), total_trips, created_at, updated_at
	`, id, email, providerauthmodels.OnboardingProfileNeeded, providerauthmodels.ProviderStatusActive)

	return scanProvider(row)
}

func (r *PostgresProviderRepository) GetByID(ctx context.Context, id string) (providerauthmodels.Provider, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, COALESCE(phone,''), COALESCE(email,''),
		       first_name, last_name, onboarding_status, status,
		       profile_photo_url, photo_asset_id,
		       COALESCE(rating,5.00), total_trips, created_at, updated_at
		FROM truck_providers WHERE id = $1
	`, id)

	p, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerauthmodels.Provider{}, apperrors.NotFound("Provider could not be found.", err)
	}
	return p, err
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
	)
	return p, err
}
