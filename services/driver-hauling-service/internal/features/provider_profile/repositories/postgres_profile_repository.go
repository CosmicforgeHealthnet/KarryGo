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
	Phone                        string
	LocationState                string
	LocationCity                 string
	OperationMode                string
	ServiceType                  string
	Language                     string
	DriverLicenseNumber          string
	LicenseExpiryYear            string
	LicenseExpiryDate            string
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

// providerColumns is the shared column list returned for every provider read/write
// so scanProvider can populate the full model.
const providerColumns = `id::text, COALESCE(phone,''), COALESCE(email,''),
		          first_name, last_name, onboarding_status, status,
		          profile_photo_url, photo_asset_id,
		          COALESCE(rating,5.00), total_trips, created_at, updated_at,
		          location_state, location_city, language, service_type, operation_mode,
		          driver_license_number, license_expiry_year, license_expiry_date,
		          gov_id_url, driver_license_url, vehicle_reg_url`

func (r *PostgresProfileRepository) UpsertByPhone(ctx context.Context, phone string) (providerauthmodels.Provider, error) {
	return providerauthrepositories.NewPostgresProviderRepository(r.db).UpsertByPhone(ctx, phone)
}

func (r *PostgresProfileRepository) UpsertByEmail(ctx context.Context, email string) (providerauthmodels.Provider, error) {
	return providerauthrepositories.NewPostgresProviderRepository(r.db).UpsertByEmail(ctx, email)
}

func (r *PostgresProfileRepository) GetByID(ctx context.Context, id string) (providerauthmodels.Provider, error) {
	return providerauthrepositories.NewPostgresProviderRepository(r.db).GetByID(ctx, id)
}

func (r *PostgresProfileRepository) UpdatePhone(ctx context.Context, id, phone string) (providerauthmodels.Provider, error) {
	return providerauthrepositories.NewPostgresProviderRepository(r.db).UpdatePhone(ctx, id, phone)
}

func (r *PostgresProfileRepository) ContactTaken(ctx context.Context, excludeID, email, phone string) (bool, bool, error) {
	return providerauthrepositories.NewPostgresProviderRepository(r.db).ContactTaken(ctx, excludeID, email, phone)
}

func (r *PostgresProfileRepository) UpdateProfile(ctx context.Context, params UpdateProfileParams) (providerauthmodels.Provider, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE truck_providers
		SET first_name                    = $2,
		    last_name                     = $3,
		    email                         = CASE WHEN $4 != '' THEN $4 ELSE email END,
		    phone                         = CASE WHEN $26 != '' THEN $26 ELSE phone END,
		    location_state                = CASE WHEN $5 != '' THEN $5 ELSE location_state END,
		    location_city                 = CASE WHEN $6 != '' THEN $6 ELSE location_city END,
		    operation_mode                = CASE WHEN $7 != '' THEN $7 ELSE operation_mode END,
		    service_type                  = CASE WHEN $8 != '' THEN $8 ELSE service_type END,
		    gov_id_url                    = CASE WHEN $9  != '' THEN $9  ELSE gov_id_url         END,
		    driver_license_url            = CASE WHEN $10 != '' THEN $10 ELSE driver_license_url  END,
		    vehicle_reg_url               = CASE WHEN $11 != '' THEN $11 ELSE vehicle_reg_url     END,
		    guarantor_name                = CASE WHEN $12 != '' THEN $12 ELSE guarantor_name END,
		    guarantor_phone               = CASE WHEN $13 != '' THEN $13 ELSE guarantor_phone END,
		    emergency_contact_name        = CASE WHEN $14 != '' THEN $14 ELSE emergency_contact_name END,
		    emergency_contact_phone       = CASE WHEN $15 != '' THEN $15 ELSE emergency_contact_phone END,
		    emergency_contact_relationship = CASE WHEN $16 != '' THEN $16 ELSE emergency_contact_relationship END,
		    profile_photo_url             = CASE WHEN $20 != '' THEN $20 ELSE profile_photo_url END,
		    photo_asset_id                = CASE WHEN $21 != '' THEN $21 ELSE photo_asset_id    END,
		    language                      = CASE WHEN $22 != '' THEN $22 ELSE language END,
		    driver_license_number         = CASE WHEN $23 != '' THEN $23 ELSE driver_license_number END,
		    license_expiry_year           = CASE WHEN $24 != '' THEN $24 ELSE license_expiry_year END,
		    license_expiry_date           = CASE WHEN $25 != '' THEN $25 ELSE license_expiry_date END,
		    onboarding_status             = CASE
		        WHEN $17 AND onboarding_status = $18 THEN $19
		        ELSE onboarding_status
		    END,
		    updated_at = now()
		WHERE id = $1
		RETURNING `+providerColumns+`
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
		params.Language,
		params.DriverLicenseNumber,
		params.LicenseExpiryYear,
		params.LicenseExpiryDate,
		params.Phone,
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
		RETURNING `+providerColumns+`
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
	GetByPlate(ctx context.Context, plate string) (providerprofilemodels.Truck, error)
	Update(ctx context.Context, truck providerprofilemodels.Truck) (providerprofilemodels.Truck, error)
	CountActiveByProvider(ctx context.Context, providerID string) (int, error)
}

type PostgresTruckRepository struct {
	db *pgxpool.Pool
}

func NewPostgresTruckRepository(db *pgxpool.Pool) *PostgresTruckRepository {
	return &PostgresTruckRepository{db: db}
}

// truckColumns is the shared column list for every truck read/write.
const truckColumns = `id::text, provider_id::text, truck_type, capacity_kg, plate_number,
		          year, make, model, color,
		          license_type, number_of_axles, years_of_experience, goods_types, has_insurance,
		          status, created_at, updated_at`

func (r *PostgresTruckRepository) Create(ctx context.Context, t providerprofilemodels.Truck) (providerprofilemodels.Truck, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO trucks (id, provider_id, truck_type, capacity_kg, plate_number, year, make, model, color,
		                    license_type, number_of_axles, years_of_experience, goods_types, has_insurance, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'active')
		RETURNING `+truckColumns+`
	`, t.ID, t.ProviderID, t.TruckType, t.CapacityKg, t.PlateNumber, t.Year, t.Make, t.Model, t.Color,
		t.LicenseType, t.NumberOfAxles, t.YearsOfExperience, t.GoodsTypes, t.HasInsurance)

	return scanTruck(row)
}

func (r *PostgresTruckRepository) ListByProvider(ctx context.Context, providerID string) ([]providerprofilemodels.Truck, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+truckColumns+`
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
		SELECT `+truckColumns+`
		FROM trucks WHERE id = $1 AND provider_id = $2
	`, id, providerID)

	t, err := scanTruck(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerprofilemodels.Truck{}, apperrors.NotFound("Truck could not be found.", err)
	}
	return t, err
}

// GetByPlate looks up a truck by its (globally unique) plate number, regardless
// of owner. Used to resolve a plate-number unique-violation into either an
// idempotent result (same provider) or a friendly validation error.
func (r *PostgresTruckRepository) GetByPlate(ctx context.Context, plate string) (providerprofilemodels.Truck, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+truckColumns+`
		FROM trucks WHERE plate_number = $1
	`, plate)

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
		    year = $6, make = $7, model = $8, color = $9,
		    license_type = $11, number_of_axles = $12, years_of_experience = $13,
		    goods_types = $14, has_insurance = $15,
		    status = $10, updated_at = now()
		WHERE id = $1 AND provider_id = $2
		RETURNING `+truckColumns+`
	`, t.ID, t.ProviderID, t.TruckType, t.CapacityKg, t.PlateNumber,
		t.Year, t.Make, t.Model, t.Color, t.Status,
		t.LicenseType, t.NumberOfAxles, t.YearsOfExperience, t.GoodsTypes, t.HasInsurance)

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
		SELECT `+truckColumns+`
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
		&p.LocationState, &p.LocationCity, &p.Language, &p.ServiceType, &p.OperationMode,
		&p.DriverLicenseNumber, &p.LicenseExpiryYear, &p.LicenseExpiryDate,
		&p.GovIDURL, &p.DriverLicenseURL, &p.VehicleRegURL,
	)
	return p, err
}

func scanTruck(row scannable) (providerprofilemodels.Truck, error) {
	var t providerprofilemodels.Truck
	err := row.Scan(
		&t.ID, &t.ProviderID, &t.TruckType, &t.CapacityKg, &t.PlateNumber,
		&t.Year, &t.Make, &t.Model, &t.Color,
		&t.LicenseType, &t.NumberOfAxles, &t.YearsOfExperience, &t.GoodsTypes, &t.HasInsurance,
		&t.Status, &t.CreatedAt, &t.UpdatedAt,
	)
	return t, err
}
