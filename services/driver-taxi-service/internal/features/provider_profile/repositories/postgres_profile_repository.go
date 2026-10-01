package providerprofilerepositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	providerauthrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/repositories"
	providerprofilemodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ─── Profile repo (extends the auth repo with write methods) ─────────────────

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
}

type ProfileRepository interface {
	providerauthrepositories.ProviderRepository
	UpdateProfile(ctx context.Context, params UpdateProfileParams) (providerauthmodels.Provider, error)
	UpdatePhoto(ctx context.Context, id, assetID, photoURL string) (providerauthmodels.Provider, error)
	// SetAcceptsParcels is its own method rather than a field on
	// UpdateProfileParams: that UPDATE uses preserve-on-empty
	// (`CASE WHEN $n != '' ...`) semantics, which has no meaning for a
	// boolean -- a partial profile PUT from a screen that does not render
	// this toggle would silently opt the driver out of parcels.
	SetAcceptsParcels(ctx context.Context, providerID string, accepts bool) (providerauthmodels.Provider, error)
	// SetVehicleOperationalStatus keeps the display-only vehicle_operational_status
	// dimension in sync as a best-effort side effect of vehicle CRUD. It is never
	// read by the go-online gate (see providerauthmodels.Provider.CanGoAvailable's
	// comment) -- the gate queries active vehicles live instead, so this column
	// existing out of sync for a moment cannot strand or wrongly admit a driver.
	SetVehicleOperationalStatus(ctx context.Context, providerID, status string) error
}

type PostgresProfileRepository struct {
	db *pgxpool.Pool
}

func NewPostgresProfileRepository(db *pgxpool.Pool) *PostgresProfileRepository {
	return &PostgresProfileRepository{db: db}
}

// providerColumns must stay in step with scanProvider below. This is a
// separate copy from provider_auth/repositories' own providerColumns -- see
// that file's comment; both must be updated together whenever a column is
// added or removed.
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

func (r *PostgresProfileRepository) UpsertByPhone(ctx context.Context, phone string, signup providerauthmodels.Signup) (providerauthmodels.Provider, error) {
	return providerauthrepositories.NewPostgresProviderRepository(r.db).UpsertByPhone(ctx, phone, signup)
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

// UpdateProfile uses preserve-on-empty semantics for every optional column:
// `CASE WHEN $n != ” THEN $n ELSE col END`. Several onboarding and profile
// screens PUT disjoint subsets of these fields, so a plain assignment would
// blank whatever the current screen does not render.
//
// first_name is the one exception: it is assigned outright because the usecase
// validates it non-empty on every call. last_name is NOT — only first_name is
// required, so a screen submitting just a first name plus documents would
// otherwise blank the surname.
//
// onboarding_status flips profile_required -> complete on the first successful
// update. There is no intermediate pending_verification state: this service
// owns no verification workflow.
func (r *PostgresProfileRepository) UpdateProfile(ctx context.Context, params UpdateProfileParams) (providerauthmodels.Provider, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE taxi_providers
		SET first_name                     = $2,
		    last_name                      = CASE WHEN $3  != '' THEN $3  ELSE last_name END,
		    email                          = CASE WHEN $4  != '' THEN $4  ELSE email END,
		    phone                          = CASE WHEN $5  != '' THEN $5  ELSE phone END,
		    location_state                 = CASE WHEN $6  != '' THEN $6  ELSE location_state END,
		    location_city                  = CASE WHEN $7  != '' THEN $7  ELSE location_city END,
		    operation_mode                 = CASE WHEN $8  != '' THEN $8  ELSE operation_mode END,
		    service_type                   = CASE WHEN $9  != '' THEN $9  ELSE service_type END,
		    language                       = CASE WHEN $10 != '' THEN $10 ELSE language END,
		    driver_license_number          = CASE WHEN $11 != '' THEN $11 ELSE driver_license_number END,
		    license_expiry_date            = CASE WHEN $12 != '' THEN $12 ELSE license_expiry_date END,
		    gov_id_url                     = CASE WHEN $13 != '' THEN $13 ELSE gov_id_url END,
		    driver_license_url             = CASE WHEN $14 != '' THEN $14 ELSE driver_license_url END,
		    vehicle_reg_url                = CASE WHEN $15 != '' THEN $15 ELSE vehicle_reg_url END,
		    guarantor_name                 = CASE WHEN $16 != '' THEN $16 ELSE guarantor_name END,
		    guarantor_phone                = CASE WHEN $17 != '' THEN $17 ELSE guarantor_phone END,
		    emergency_contact_name         = CASE WHEN $18 != '' THEN $18 ELSE emergency_contact_name END,
		    emergency_contact_phone        = CASE WHEN $19 != '' THEN $19 ELSE emergency_contact_phone END,
		    emergency_contact_relationship = CASE WHEN $20 != '' THEN $20 ELSE emergency_contact_relationship END,
		    profile_photo_url              = CASE WHEN $21 != '' THEN $21 ELSE profile_photo_url END,
		    photo_asset_id                 = CASE WHEN $22 != '' THEN $22 ELSE photo_asset_id END,
		    onboarding_status              = CASE
		        WHEN onboarding_status = $23 THEN $24
		        ELSE onboarding_status
		    END,
		    updated_at = now()
		WHERE id = $1
		RETURNING `+providerColumns+`
	`,
		params.ProviderID,
		params.FirstName, params.LastName, params.Email, params.Phone,
		params.LocationState, params.LocationCity, params.OperationMode, params.ServiceType,
		params.Language, params.DriverLicenseNumber, params.LicenseExpiryDate,
		params.GovIDURL, params.DriverLicenseURL, params.VehicleRegURL,
		params.GuarantorName, params.GuarantorPhone,
		params.EmergencyContactName, params.EmergencyContactPhone, params.EmergencyContactRelationship,
		params.ProfilePhotoURL, params.PhotoAssetID,
		providerauthmodels.OnboardingProfileNeeded,
		providerauthmodels.OnboardingComplete,
	)

	p, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerauthmodels.Provider{}, apperrors.NotFound("Driver could not be found.", err)
	}
	return p, err
}

func (r *PostgresProfileRepository) UpdatePhoto(ctx context.Context, id, assetID, photoURL string) (providerauthmodels.Provider, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE taxi_providers
		SET profile_photo_url = $2, photo_asset_id = $3, updated_at = now()
		WHERE id = $1
		RETURNING `+providerColumns+`
	`, id, photoURL, assetID)

	p, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerauthmodels.Provider{}, apperrors.NotFound("Driver could not be found.", err)
	}
	return p, err
}

func (r *PostgresProfileRepository) SetAcceptsParcels(ctx context.Context, providerID string, accepts bool) (providerauthmodels.Provider, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE taxi_providers
		SET accepts_parcels = $2, updated_at = now()
		WHERE id = $1
		RETURNING `+providerColumns+`
	`, providerID, accepts)

	p, err := scanProvider(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerauthmodels.Provider{}, apperrors.NotFound("Driver could not be found.", err)
	}
	return p, err
}

func (r *PostgresProfileRepository) SetVehicleOperationalStatus(ctx context.Context, providerID, status string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE taxi_providers SET vehicle_operational_status = $2, updated_at = now()
		WHERE id = $1
	`, providerID, status)
	return err
}

// ─── Vehicle repo ─────────────────────────────────────────────────────────────

type VehicleRepository interface {
	Create(ctx context.Context, vehicle providerprofilemodels.Vehicle) (providerprofilemodels.Vehicle, error)
	ListByProvider(ctx context.Context, providerID string) ([]providerprofilemodels.Vehicle, error)
	GetByID(ctx context.Context, id, providerID string) (providerprofilemodels.Vehicle, error)
	// GetByIDAnywhere ignores ownership. The matcher and the passenger-facing
	// vehicle lookup both need a vehicle they do not own.
	GetByIDAnywhere(ctx context.Context, id string) (providerprofilemodels.Vehicle, error)
	GetByPlate(ctx context.Context, plate string) (providerprofilemodels.Vehicle, error)
	Update(ctx context.Context, vehicle providerprofilemodels.Vehicle) (providerprofilemodels.Vehicle, error)
	CountActiveByProvider(ctx context.Context, providerID string) (int, error)
	// FirstActiveByProvider backs the availability gate and the matcher, which
	// need the vehicle a driver is currently working with.
	FirstActiveByProvider(ctx context.Context, providerID string) (providerprofilemodels.Vehicle, error)
}

type PostgresVehicleRepository struct {
	db *pgxpool.Pool
}

func NewPostgresVehicleRepository(db *pgxpool.Pool) *PostgresVehicleRepository {
	return &PostgresVehicleRepository{db: db}
}

const vehicleColumns = `id::text, provider_id::text, vehicle_class, plate_number, seats,
		          year, make, model, color, status, created_at, updated_at`

func (r *PostgresVehicleRepository) Create(ctx context.Context, v providerprofilemodels.Vehicle) (providerprofilemodels.Vehicle, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO vehicles (id, provider_id, vehicle_class, plate_number, seats, year, make, model, color, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active')
		RETURNING `+vehicleColumns+`
	`, v.ID, v.ProviderID, v.VehicleClass, v.PlateNumber, v.Seats, v.Year, v.Make, v.Model, v.Color)

	return scanVehicle(row)
}

func (r *PostgresVehicleRepository) ListByProvider(ctx context.Context, providerID string) ([]providerprofilemodels.Vehicle, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+vehicleColumns+`
		FROM vehicles WHERE provider_id = $1 ORDER BY created_at ASC
	`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vehicles []providerprofilemodels.Vehicle
	for rows.Next() {
		v, err := scanVehicle(rows)
		if err != nil {
			return nil, err
		}
		vehicles = append(vehicles, v)
	}
	return vehicles, rows.Err()
}

func (r *PostgresVehicleRepository) GetByID(ctx context.Context, id, providerID string) (providerprofilemodels.Vehicle, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+vehicleColumns+`
		FROM vehicles WHERE id = $1 AND provider_id = $2
	`, id, providerID)

	v, err := scanVehicle(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", err)
	}
	return v, err
}

func (r *PostgresVehicleRepository) GetByIDAnywhere(ctx context.Context, id string) (providerprofilemodels.Vehicle, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+vehicleColumns+`
		FROM vehicles WHERE id = $1
	`, id)

	v, err := scanVehicle(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", err)
	}
	return v, err
}

// GetByPlate looks up a vehicle by its globally unique plate regardless of
// owner, so a plate-number unique violation can be resolved into either an
// idempotent success (same driver re-submitting) or a friendly validation error.
func (r *PostgresVehicleRepository) GetByPlate(ctx context.Context, plate string) (providerprofilemodels.Vehicle, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+vehicleColumns+`
		FROM vehicles WHERE plate_number = $1
	`, plate)

	v, err := scanVehicle(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", err)
	}
	return v, err
}

func (r *PostgresVehicleRepository) Update(ctx context.Context, v providerprofilemodels.Vehicle) (providerprofilemodels.Vehicle, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE vehicles
		SET vehicle_class = $3, plate_number = $4, seats = $5,
		    year = $6, make = $7, model = $8, color = $9,
		    status = $10, updated_at = now()
		WHERE id = $1 AND provider_id = $2
		RETURNING `+vehicleColumns+`
	`, v.ID, v.ProviderID, v.VehicleClass, v.PlateNumber, v.Seats,
		v.Year, v.Make, v.Model, v.Color, v.Status)

	updated, err := scanVehicle(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", err)
	}
	return updated, err
}

func (r *PostgresVehicleRepository) CountActiveByProvider(ctx context.Context, providerID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM vehicles WHERE provider_id = $1 AND status = 'active'`, providerID).Scan(&count)
	return count, err
}

func (r *PostgresVehicleRepository) FirstActiveByProvider(ctx context.Context, providerID string) (providerprofilemodels.Vehicle, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+vehicleColumns+`
		FROM vehicles WHERE provider_id = $1 AND status = 'active'
		ORDER BY created_at ASC LIMIT 1
	`, providerID)

	v, err := scanVehicle(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return providerprofilemodels.Vehicle{}, apperrors.NotFound("Vehicle could not be found.", err)
	}
	return v, err
}

// ─── scanner helpers ──────────────────────────────────────────────────────────

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

func scanVehicle(row scannable) (providerprofilemodels.Vehicle, error) {
	var v providerprofilemodels.Vehicle
	err := row.Scan(
		&v.ID, &v.ProviderID, &v.VehicleClass, &v.PlateNumber, &v.Seats,
		&v.Year, &v.Make, &v.Model, &v.Color,
		&v.Status, &v.CreatedAt, &v.UpdatedAt,
	)
	return v, err
}
