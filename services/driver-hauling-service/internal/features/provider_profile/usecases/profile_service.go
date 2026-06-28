package providerprofileusecases

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	providerauthmodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/models"
	providerprofilemodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/models"
	providerprofilerepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/phonenumber"
)

type ProfileService struct {
	profiles providerprofilerepositories.ProfileRepository
	trucks   providerprofilerepositories.TruckRepository
}

func NewProfileService(profiles providerprofilerepositories.ProfileRepository, trucks providerprofilerepositories.TruckRepository) *ProfileService {
	return &ProfileService{profiles: profiles, trucks: trucks}
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

func (s *ProfileService) GetProfile(ctx context.Context, providerID string) (providerauthmodels.PublicProvider, error) {
	p, err := s.profiles.GetByID(ctx, providerID)
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

	// Normalize the phone (when provided) to Nigerian +234 format before persisting,
	// so it matches the format used at sign-in and the unique constraint is meaningful.
	phone := strings.TrimSpace(input.Phone)
	if phone != "" {
		normalized, err := phonenumber.NormalizeNigerianPhoneNumber(phone)
		if err != nil {
			return providerauthmodels.PublicProvider{}, err
		}
		phone = normalized
	}

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
		LicenseExpiryYear:            strings.TrimSpace(input.LicenseExpiryYear),
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
		SubmitForVerification:        input.SubmitForVerification,
	})
	if err != nil {
		// A unique-constraint violation means the email or phone is already taken by
		// another provider. Surface a friendly validation error instead of a 500.
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

// CheckContactAvailability reports whether the given email and/or phone already
// belong to a provider other than providerID. Lets the onboarding UI flag a taken
// identifier before the user advances. The phone is normalized to match stored
// values; a malformed phone returns a validation error.
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

// ─── Trucks ───────────────────────────────────────────────────────────────────

type CreateTruckInput struct {
	ProviderID        string
	TruckType         string
	CapacityKg        int
	PlateNumber       string
	Year              *int
	Make              *string
	Model             *string
	Color             *string
	LicenseType       string
	NumberOfAxles     string
	YearsOfExperience string
	GoodsTypes        []string
	HasInsurance      bool
}

func (s *ProfileService) CreateTruck(ctx context.Context, input CreateTruckInput) (providerprofilemodels.PublicTruck, error) {
	var fields []apperrors.FieldViolation
	if !providerprofilemodels.ValidTruckTypes[strings.ToLower(input.TruckType)] {
		fields = append(fields, apperrors.FieldViolation{Field: "truck_type", Message: "Truck type must be one of: flatbed, container, tipper, van, refrigerated."})
	}
	if input.CapacityKg <= 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "capacity_kg", Message: "Capacity must be greater than zero."})
	}
	plate := strings.TrimSpace(strings.ToUpper(input.PlateNumber))
	if plate == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "plate_number", Message: "Plate number is required."})
	}
	if len(fields) > 0 {
		return providerprofilemodels.PublicTruck{}, apperrors.Validation("Please check your truck details.", fields)
	}

	truck := providerprofilemodels.Truck{
		ID:                uuid.NewString(),
		ProviderID:        input.ProviderID,
		TruckType:         strings.ToLower(input.TruckType),
		CapacityKg:        input.CapacityKg,
		PlateNumber:       plate,
		Year:              input.Year,
		Make:              input.Make,
		Model:             input.Model,
		Color:             input.Color,
		LicenseType:       strings.TrimSpace(input.LicenseType),
		NumberOfAxles:     strings.TrimSpace(input.NumberOfAxles),
		YearsOfExperience: strings.TrimSpace(input.YearsOfExperience),
		GoodsTypes:        normalizeGoods(input.GoodsTypes),
		HasInsurance:      input.HasInsurance,
	}

	created, err := s.trucks.Create(ctx, truck)
	if err != nil {
		// A plate-number unique-violation means the plate is already registered.
		// If it belongs to this same provider, treat the create as idempotent
		// (e.g. an onboarding retry re-submitting the same truck); otherwise
		// surface a friendly validation error instead of a 500.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if existing, lookupErr := s.trucks.GetByPlate(ctx, plate); lookupErr == nil && existing.ProviderID == input.ProviderID {
				return existing.Public(), nil
			}
			return providerprofilemodels.PublicTruck{}, apperrors.Validation("This plate number is already in use.", []apperrors.FieldViolation{
				{Field: "plate_number", Message: "This plate number is already in use."},
			})
		}
		return providerprofilemodels.PublicTruck{}, apperrors.Internal("Truck could not be registered.", err)
	}
	return created.Public(), nil
}

func (s *ProfileService) ListTrucks(ctx context.Context, providerID string) ([]providerprofilemodels.PublicTruck, error) {
	trucks, err := s.trucks.ListByProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	result := make([]providerprofilemodels.PublicTruck, len(trucks))
	for i, t := range trucks {
		result[i] = t.Public()
	}
	return result, nil
}

func (s *ProfileService) GetTruck(ctx context.Context, id, providerID string) (providerprofilemodels.PublicTruck, error) {
	t, err := s.trucks.GetByID(ctx, id, providerID)
	if err != nil {
		return providerprofilemodels.PublicTruck{}, err
	}
	return t.Public(), nil
}

type UpdateTruckInput struct {
	ID                string
	ProviderID        string
	TruckType         string
	CapacityKg        int
	PlateNumber       string
	Year              *int
	Make              *string
	Model             *string
	Color             *string
	LicenseType       string
	NumberOfAxles     string
	YearsOfExperience string
	GoodsTypes        []string
	HasInsurance      bool
	Status            string
}

func (s *ProfileService) UpdateTruck(ctx context.Context, input UpdateTruckInput) (providerprofilemodels.PublicTruck, error) {
	var fields []apperrors.FieldViolation
	if !providerprofilemodels.ValidTruckTypes[strings.ToLower(input.TruckType)] {
		fields = append(fields, apperrors.FieldViolation{Field: "truck_type", Message: "Truck type must be one of: flatbed, container, tipper, van, refrigerated."})
	}
	if input.CapacityKg <= 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "capacity_kg", Message: "Capacity must be greater than zero."})
	}
	plate := strings.TrimSpace(strings.ToUpper(input.PlateNumber))
	if plate == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "plate_number", Message: "Plate number is required."})
	}
	if !providerprofilemodels.ValidTruckStatuses[input.Status] {
		fields = append(fields, apperrors.FieldViolation{Field: "status", Message: "Status must be 'active' or 'inactive'."})
	}
	if len(fields) > 0 {
		return providerprofilemodels.PublicTruck{}, apperrors.Validation("Please check your truck details.", fields)
	}

	truck := providerprofilemodels.Truck{
		ID:                input.ID,
		ProviderID:        input.ProviderID,
		TruckType:         strings.ToLower(input.TruckType),
		CapacityKg:        input.CapacityKg,
		PlateNumber:       plate,
		Year:              input.Year,
		Make:              input.Make,
		Model:             input.Model,
		Color:             input.Color,
		LicenseType:       strings.TrimSpace(input.LicenseType),
		NumberOfAxles:     strings.TrimSpace(input.NumberOfAxles),
		YearsOfExperience: strings.TrimSpace(input.YearsOfExperience),
		GoodsTypes:        normalizeGoods(input.GoodsTypes),
		HasInsurance:      input.HasInsurance,
		Status:            input.Status,
	}
	updated, err := s.trucks.Update(ctx, truck)
	if err != nil {
		return providerprofilemodels.PublicTruck{}, err
	}
	return updated.Public(), nil
}

// normalizeGoods trims and drops empty entries, always returning a non-nil slice
// so it stores as an empty array rather than NULL.
func normalizeGoods(goods []string) []string {
	out := make([]string, 0, len(goods))
	for _, g := range goods {
		if g = strings.TrimSpace(g); g != "" {
			out = append(out, g)
		}
	}
	return out
}
