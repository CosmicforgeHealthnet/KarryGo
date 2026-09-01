package providerprofileusecases

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	providerprofilemodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/models"
	providerprofilerepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/repositories"
	"cosmicforge/logistics/services/taxi-service/internal/vehicleclass"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/phonenumber"
)

type ProfileService struct {
	profiles providerprofilerepositories.ProfileRepository
	vehicles providerprofilerepositories.VehicleRepository
}

func NewProfileService(profiles providerprofilerepositories.ProfileRepository, vehicles providerprofilerepositories.VehicleRepository) *ProfileService {
	return &ProfileService{profiles: profiles, vehicles: vehicles}
}

// ─── Profile ──────────────────────────────────────────────────────────────────

type UpdateProfileInput struct {
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

func (s *ProfileService) GetProfile(ctx context.Context, providerID string) (providerauthmodels.PublicProvider, error) {
	p, err := s.profiles.GetByID(ctx, providerID)
	if err != nil {
		return providerauthmodels.PublicProvider{}, err
	}
	return p.Public(), nil
}

// SetAcceptsParcels toggles the driver's opt-in to carrying packages.
//
// Deliberately NOT part of UpdateProfile: that path preserves fields left
// empty, which cannot express "false" for a boolean, so a profile save from
// a screen without this toggle would silently opt the driver out.
func (s *ProfileService) SetAcceptsParcels(ctx context.Context, providerID string, accepts bool) (providerauthmodels.PublicProvider, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return providerauthmodels.PublicProvider{}, apperrors.Unauthorized("Authentication is required.", nil)
	}
	p, err := s.profiles.SetAcceptsParcels(ctx, providerID, accepts)
	if err != nil {
		return providerauthmodels.PublicProvider{}, err
	}
	return p.Public(), nil
}

func (s *ProfileService) UpdateProfile(ctx context.Context, input UpdateProfileInput) (providerauthmodels.PublicProvider, error) {
	firstName := strings.TrimSpace(input.FirstName)

	var fields []apperrors.FieldViolation
	if firstName == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "first_name", Message: "First name is required."})
	}
	if len(fields) > 0 {
		return providerauthmodels.PublicProvider{}, apperrors.Validation("Please check your details.", fields)
	}

	// Normalize the login phone to +234 so it matches what sign-in stores and the
	// unique constraint is meaningful.
	phone := strings.TrimSpace(input.Phone)
	if phone != "" {
		normalized, err := phonenumber.NormalizeNigerianPhoneNumber(phone)
		if err != nil {
			return providerauthmodels.PublicProvider{}, err
		}
		phone = normalized
	}

	// Guarantor and emergency-contact numbers are only trimmed, never normalized:
	// they are next-of-kin contacts that may legitimately be non-Nigerian, and
	// rejecting them would block onboarding. The app sends the final string.
	p, err := s.profiles.UpdateProfile(ctx, providerprofilerepositories.UpdateProfileParams{
		ProviderID:                   input.ProviderID,
		FirstName:                    firstName,
		LastName:                     strings.TrimSpace(input.LastName),
		Email:                        strings.ToLower(strings.TrimSpace(input.Email)),
		Phone:                        phone,
		LocationState:                strings.TrimSpace(input.LocationState),
		LocationCity:                 strings.TrimSpace(input.LocationCity),
		OperationMode:                strings.ToLower(strings.TrimSpace(input.OperationMode)),
		ServiceType:                  strings.ToLower(strings.TrimSpace(input.ServiceType)),
		Language:                     strings.TrimSpace(input.Language),
		DriverLicenseNumber:          strings.TrimSpace(input.DriverLicenseNumber),
		LicenseExpiryDate:            strings.TrimSpace(input.LicenseExpiryDate),
		GovIDURL:                     strings.TrimSpace(input.GovIDURL),
		DriverLicenseURL:             strings.TrimSpace(input.DriverLicenseURL),
		VehicleRegURL:                strings.TrimSpace(input.VehicleRegURL),
		GuarantorName:                strings.TrimSpace(input.GuarantorName),
		GuarantorPhone:               strings.TrimSpace(input.GuarantorPhone),
		EmergencyContactName:         strings.TrimSpace(input.EmergencyContactName),
		EmergencyContactPhone:        strings.TrimSpace(input.EmergencyContactPhone),
		EmergencyContactRelationship: strings.TrimSpace(input.EmergencyContactRelationship),
		ProfilePhotoURL:              strings.TrimSpace(input.ProfilePhotoURL),
		PhotoAssetID:                 strings.TrimSpace(input.PhotoAssetID),
	})
	if err != nil {
		// A unique violation means the email or phone belongs to another driver.
		// Surface a field-specific validation error rather than a 500.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			field, msg := "email", "This email address is already in use."
			if strings.Contains(pgErr.ConstraintName, "phone") {
				field, msg = "phone", "This phone number is already in use."
			}
			return providerauthmodels.PublicProvider{}, apperrors.Validation(msg, []apperrors.FieldViolation{
				{Field: field, Message: msg},
			})
		}
		return providerauthmodels.PublicProvider{}, err
	}
	return p.Public(), nil
}

// CheckContactAvailability lets onboarding flag a taken identifier before the
// driver advances, rather than failing at submit.
func (s *ProfileService) CheckContactAvailability(ctx context.Context, providerID, email, phone string) (emailTaken, phoneTaken bool, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	phone = strings.TrimSpace(phone)
	if phone != "" {
		normalized, perr := phonenumber.NormalizeNigerianPhoneNumber(phone)
		if perr != nil {
			return false, false, perr
		}
		phone = normalized
	}
	return s.profiles.ContactTaken(ctx, providerID, email, phone)
}

// ─── Vehicles ─────────────────────────────────────────────────────────────────

type CreateVehicleInput struct {
	ProviderID   string
	VehicleClass string
	PlateNumber  string
	Seats        int
	Year         *int
	Make         *string
	Model        *string
	Color        *string
}

func (s *ProfileService) CreateVehicle(ctx context.Context, input CreateVehicleInput) (providerprofilemodels.PublicVehicle, error) {
	class := vehicleclass.Normalize(input.VehicleClass)
	spec, classOK := vehicleclass.Get(class)

	var fields []apperrors.FieldViolation
	if !classOK {
		fields = append(fields, apperrors.FieldViolation{
			Field:   "vehicle_class",
			Message: "Vehicle class must be one of: " + strings.Join(vehicleclass.Names(), ", ") + ".",
		})
	}
	plate := strings.TrimSpace(strings.ToUpper(input.PlateNumber))
	if plate == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "plate_number", Message: "Plate number is required."})
	}
	if input.Seats < 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "seats", Message: "Seats cannot be negative."})
	}
	if len(fields) > 0 {
		return providerprofilemodels.PublicVehicle{}, apperrors.Validation("Please check your vehicle details.", fields)
	}

	// An unstated seat count falls back to the class default, so the matcher
	// always has a real number to compare a booking's passenger count against.
	seats := input.Seats
	if seats == 0 {
		seats = spec.DefaultSeats
	}

	vehicle := providerprofilemodels.Vehicle{
		ID:           uuid.NewString(),
		ProviderID:   input.ProviderID,
		VehicleClass: class,
		PlateNumber:  plate,
		Seats:        seats,
		Year:         input.Year,
		Make:         input.Make,
		Model:        input.Model,
		Color:        input.Color,
	}

	created, err := s.vehicles.Create(ctx, vehicle)
	if err != nil {
		// A plate unique-violation means the plate is already registered. If it
		// belongs to this same driver, treat the create as idempotent — onboarding
		// retries re-submit the same vehicle. Otherwise it is a real conflict.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if existing, lookupErr := s.vehicles.GetByPlate(ctx, plate); lookupErr == nil && existing.ProviderID == input.ProviderID {
				s.syncVehicleOperationalStatus(ctx, input.ProviderID)
				return existing.Public(), nil
			}
			return providerprofilemodels.PublicVehicle{}, apperrors.Validation("This plate number is already in use.", []apperrors.FieldViolation{
				{Field: "plate_number", Message: "This plate number is already in use."},
			})
		}
		return providerprofilemodels.PublicVehicle{}, apperrors.Internal("Vehicle could not be registered.", err)
	}
	// A new vehicle is always created active (see s.vehicles.Create's INSERT),
	// so the provider now has at least one -- recompute the display-only
	// dimension. Never gates go-online itself; see CanGoAvailable's comment.
	s.syncVehicleOperationalStatus(ctx, input.ProviderID)
	return created.Public(), nil
}

// syncVehicleOperationalStatus recomputes taxi_providers.vehicle_operational_status
// from a live count, rather than trusting the single row a CreateVehicle/
// UpdateVehicle call just touched -- a driver can own several vehicles, and
// only a live re-count is correct when one of several flips inactive while
// another stays active. Best-effort: a failure here only means the display-only
// field is stale until the next vehicle write, never that go-online is wrongly
// gated -- that check is always live (see resolveVehicle in the availability
// usecase), never sourced from this cached column.
func (s *ProfileService) syncVehicleOperationalStatus(ctx context.Context, providerID string) {
	count, err := s.vehicles.CountActiveByProvider(ctx, providerID)
	if err != nil {
		return
	}
	status := providerauthmodels.VehicleOperationalNone
	if count > 0 {
		status = providerauthmodels.VehicleOperationalActive
	}
	_ = s.profiles.SetVehicleOperationalStatus(ctx, providerID, status)
}

func (s *ProfileService) ListVehicles(ctx context.Context, providerID string) ([]providerprofilemodels.PublicVehicle, error) {
	vehicles, err := s.vehicles.ListByProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	result := make([]providerprofilemodels.PublicVehicle, len(vehicles))
	for i, v := range vehicles {
		result[i] = v.Public()
	}
	return result, nil
}

func (s *ProfileService) GetVehicle(ctx context.Context, id, providerID string) (providerprofilemodels.PublicVehicle, error) {
	v, err := s.vehicles.GetByID(ctx, id, providerID)
	if err != nil {
		return providerprofilemodels.PublicVehicle{}, err
	}
	return v.Public(), nil
}

// GetPublicVehicle exposes a vehicle without an ownership check, for the
// passenger looking at the car that is coming to collect them.
func (s *ProfileService) GetPublicVehicle(ctx context.Context, id string) (providerprofilemodels.PublicVehicle, error) {
	v, err := s.vehicles.GetByIDAnywhere(ctx, id)
	if err != nil {
		return providerprofilemodels.PublicVehicle{}, err
	}
	return v.Public(), nil
}

type UpdateVehicleInput struct {
	ID           string
	ProviderID   string
	VehicleClass string
	PlateNumber  string
	Seats        int
	Year         *int
	Make         *string
	Model        *string
	Color        *string
	Status       string
}

func (s *ProfileService) UpdateVehicle(ctx context.Context, input UpdateVehicleInput) (providerprofilemodels.PublicVehicle, error) {
	class := vehicleclass.Normalize(input.VehicleClass)
	spec, classOK := vehicleclass.Get(class)

	var fields []apperrors.FieldViolation
	if !classOK {
		fields = append(fields, apperrors.FieldViolation{
			Field:   "vehicle_class",
			Message: "Vehicle class must be one of: " + strings.Join(vehicleclass.Names(), ", ") + ".",
		})
	}
	plate := strings.TrimSpace(strings.ToUpper(input.PlateNumber))
	if plate == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "plate_number", Message: "Plate number is required."})
	}
	if !providerprofilemodels.ValidVehicleStatuses[input.Status] {
		fields = append(fields, apperrors.FieldViolation{Field: "status", Message: "Status must be 'active' or 'inactive'."})
	}
	if input.Seats < 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "seats", Message: "Seats cannot be negative."})
	}
	if len(fields) > 0 {
		return providerprofilemodels.PublicVehicle{}, apperrors.Validation("Please check your vehicle details.", fields)
	}

	seats := input.Seats
	if seats == 0 {
		seats = spec.DefaultSeats
	}

	vehicle := providerprofilemodels.Vehicle{
		ID:           input.ID,
		ProviderID:   input.ProviderID,
		VehicleClass: class,
		PlateNumber:  plate,
		Seats:        seats,
		Year:         input.Year,
		Make:         input.Make,
		Model:        input.Model,
		Color:        input.Color,
		Status:       input.Status,
	}
	updated, err := s.vehicles.Update(ctx, vehicle)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return providerprofilemodels.PublicVehicle{}, apperrors.Validation("This plate number is already in use.", []apperrors.FieldViolation{
				{Field: "plate_number", Message: "This plate number is already in use."},
			})
		}
		return providerprofilemodels.PublicVehicle{}, err
	}
	// Status may have flipped active<->inactive; recompute from a live count
	// rather than assuming this one row's new status settles the provider-level
	// summary (the driver may have other active vehicles).
	s.syncVehicleOperationalStatus(ctx, input.ProviderID)
	return updated.Public(), nil
}
