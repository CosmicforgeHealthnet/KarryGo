package providerprofilerepositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	providerauthmodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/models"
	providerauthrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/repositories"
	providerprofilemodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ─── Profile repo (extends auth repo with write methods) ─────────────────────

type UpdateProfileParams struct {
	ProviderID                   string
	FirstName                    string
	LastName                     string
	Email                        string
	LocationState                string
	LocationCity                 string
	OperationMode                string
	ServiceType                  string
	GovIDURL                     string
	DriverLicenseURL             string
	VehicleRegURL                string
	GuarantorName                string
	GuarantorPhone               string
	EmergencyContactName         string
	EmergencyContactPhone        string
	EmergencyContactRelationship string
	ProfilePhotoURL              string
	PhotoAssetID                 string
	SubmitForVerification        bool
}

type ProfileRepository interface {
	providerauthrepositories.ProviderRepository
	UpdateProfile(ctx context.Context, params UpdateProfileParams) (providerauthmodels.Provider, error)
	UpdatePhoto(ctx context.Context, id, assetID, photoURL string) (providerauthmodels.Provider, error)
}

type PostgresProfileRepository struct {
	db *pgxpool.Pool
}

func NewPostgresProfileRepository(db *pgxpool.Pool) *PostgresProfileRepository {
	return &PostgresProfileRepository{db: db}
}

func (r *PostgresProfileRepository) UpsertByPhone(ctx context.Context, phone string) (providerauthmodels.Provider, error) {
	return providerauthrepositories.NewPostgresProviderRepository(r.db).UpsertByPhone(ctx, phone)
}

func (r *PostgresProfileRepository) UpsertByEmail(ctx context.Context, email string) (providerauthmodels.Provider, error) {
	return providerauthrepositories.NewPostgresProviderRepository(r.db).UpsertByEmail(ctx, email)
}

func (r *PostgresProfileRepository) GetByID(ctx context.Context, id string) (providerauthmodels.Provider, error) {
	return providerauthrepositories.NewPostgresProviderRepository(r.db).GetByID(ctx, id)
}

func (r *PostgresProfileRepository) UpdateProfile(ctx context.Context, params UpdateProfileParams) (providerauthmodels.Provider, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE truck_providers
		SET first_name                    = $2,
		    last_name                     = $3,
		    email                         = CASE WHEN $4 != '' THEN $4 ELSE email END,
		    location_state                = $5,
		    location_city                 = $6,
		    operation_mode                = $7,
		    service_type                  = $8,
		    gov_id_url                    = CASE WHEN $9  != '' THEN $9  ELSE gov_id_url         END,
		    driver_license_url            = CASE WHEN $10 != '' THEN $10 ELSE driver_license_url  END,
		    vehicle_reg_url               = CASE WHEN $11 != '' THEN $11 ELSE vehicle_reg_url     END,
		    guarantor_name                = $12,
		    guarantor_phone               = $13,
		    emergency_contact_name        = $14,
		    emergency_contact_phone       = $15,
		    emergency_contact_relationship = $16,
		    profile_photo_url             = CASE WHEN $20 != '' THEN $20 ELSE profile_photo_url END,
		    photo_asset_id                = CASE WHEN $21 != '' THEN $21 ELSE photo_asset_id    END,
		    onboarding_status             = CASE
		        WHEN $17 AND onboarding_status = $18 THEN $19
		        ELSE onboarding_status
		    END,
		    updated_at = now()
		WHERE id = $1
		RETURNING id::text, COALESCE(phone,''), COALESCE(email,''),
		          first_name, last_name, onboarding_status, status,
		          profile_photo_url, photo_asset_id,
		          COALESCE(rating,5.00), total_trips, created_at, updated_at
	`,
		params.ProviderID,
		params.FirstName, params.LastName, params.Email,
		params.LocationState, params.LocationCity, params.OperationMode, params.ServiceType,
		params.GovIDURL, params.DriverLicenseURL, params.VehicleRegURL,
		params.GuarantorName, params.GuarantorPhone,
		params.EmergencyContactName, params.EmergencyContactPhone, params.EmergencyContactRelationship,
		params.SubmitForVerification,
		providerauthmodels.OnboardingProfileNeeded,
		providerauthmodels.OnboardingPendingVerification,
		params.ProfilePhotoURL,
		params.PhotoAssetID,
	)

	p, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerauthmodels.Provider{}, apperrors.NotFound("Provider could not be found.", err)
	}
	return p, err
}

func (r *PostgresProfileRepository) UpdatePhoto(ctx context.Context, id, assetID, photoURL string) (providerauthmodels.Provider, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE truck_providers
		SET profile_photo_url = $2, photo_asset_id = $3, updated_at = now()
		WHERE id = $1
		RETURNING id::text, COALESCE(phone,''), COALESCE(email,''),
		          first_name, last_name, onboarding_status, status,
		          profile_photo_url, photo_asset_id,
		          COALESCE(rating,5.00), total_trips, created_at, updated_at
	`, id, photoURL, assetID)

	p, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerauthmodels.Provider{}, apperrors.NotFound("Provider could not be found.", err)
	}
	return p, err
}

// ─── Truck repo ───────────────────────────────────────────────────────────────

type TruckRepository interface {
	Create(ctx context.Context, truck providerprofilemodels.Truck) (providerprofilemodels.Truck, error)
	ListByProvider(ctx context.Context, providerID string) ([]providerprofilemodels.Truck, error)
	GetByID(ctx context.Context, id, providerID string) (providerprofilemodels.Truck, error)
	GetByIDAnywhere(ctx context.Context, id string) (providerprofilemodels.Truck, error)
	Update(ctx context.Context, truck providerprofilemodels.Truck) (providerprofilemodels.Truck, error)
	CountActiveByProvider(ctx context.Context, providerID string) (int, error)
}

type PostgresTruckRepository struct {
	db *pgxpool.Pool
}

func NewPostgresTruckRepository(db *pgxpool.Pool) *PostgresTruckRepository {
	return &PostgresTruckRepository{db: db}
}

func (r *PostgresTruckRepository) Create(ctx context.Context, t providerprofilemodels.Truck) (providerprofilemodels.Truck, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO trucks (id, provider_id, truck_type, capacity_kg, plate_number, year, make, model, color, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active')
		RETURNING id::text, provider_id::text, truck_type, capacity_kg, plate_number,
		          year, make, model, color, status, created_at, updated_at
	`, t.ID, t.ProviderID, t.TruckType, t.CapacityKg, t.PlateNumber, t.Year, t.Make, t.Model, t.Color)

	return scanTruck(row)
}

func (r *PostgresTruckRepository) ListByProvider(ctx context.Context, providerID string) ([]providerprofilemodels.Truck, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, provider_id::text, truck_type, capacity_kg, plate_number,
		       year, make, model, color, status, created_at, updated_at
		FROM trucks WHERE provider_id = $1 ORDER BY created_at ASC
	`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trucks []providerprofilemodels.Truck
	for rows.Next() {
		t, err := scanTruck(rows)
		if err != nil {
			return nil, err
		}
		trucks = append(trucks, t)
	}
	return trucks, rows.Err()
}

func (r *PostgresTruckRepository) GetByID(ctx context.Context, id, providerID string) (providerprofilemodels.Truck, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, provider_id::text, truck_type, capacity_kg, plate_number,
		       year, make, model, color, status, created_at, updated_at
		FROM trucks WHERE id = $1 AND provider_id = $2
	`, id, providerID)

	t, err := scanTruck(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerprofilemodels.Truck{}, apperrors.NotFound("Truck could not be found.", err)
	}
	return t, err
}

func (r *PostgresTruckRepository) Update(ctx context.Context, t providerprofilemodels.Truck) (providerprofilemodels.Truck, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE trucks
		SET truck_type = $3, capacity_kg = $4, plate_number = $5,
		    year = $6, make = $7, model = $8, color = $9, status = $10, updated_at = now()
		WHERE id = $1 AND provider_id = $2
		RETURNING id::text, provider_id::text, truck_type, capacity_kg, plate_number,
		          year, make, model, color, status, created_at, updated_at
	`, t.ID, t.ProviderID, t.TruckType, t.CapacityKg, t.PlateNumber,
		t.Year, t.Make, t.Model, t.Color, t.Status)

	updated, err := scanTruck(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerprofilemodels.Truck{}, apperrors.NotFound("Truck could not be found.", err)
	}
	return updated, err
}

func (r *PostgresTruckRepository) CountActiveByProvider(ctx context.Context, providerID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM trucks WHERE provider_id = $1 AND status = 'active'`, providerID).Scan(&count)
	return count, err
}

func (r *PostgresTruckRepository) GetByIDAnywhere(ctx context.Context, id string) (providerprofilemodels.Truck, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, provider_id::text, truck_type, capacity_kg, plate_number,
		       year, make, model, color, status, created_at, updated_at
		FROM trucks WHERE id = $1
	`, id)
	t, err := scanTruck(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerprofilemodels.Truck{}, apperrors.NotFound("Truck could not be found.", err)
	}
	return t, err
}

// ─── scanner helpers ──────────────────────────────────────────────────────────

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

func scanTruck(row scannable) (providerprofilemodels.Truck, error) {
	var t providerprofilemodels.Truck
	err := row.Scan(
		&t.ID, &t.ProviderID, &t.TruckType, &t.CapacityKg, &t.PlateNumber,
		&t.Year, &t.Make, &t.Model, &t.Color, &t.Status, &t.CreatedAt, &t.UpdatedAt,
	)
	return t, err
}
