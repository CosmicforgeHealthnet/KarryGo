package providerprofileusecases

import (
	"context"
	"strings"

	"github.com/google/uuid"

	providerauthmodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/models"
	providerprofilemodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/models"
	providerprofilerepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
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

	p, err := s.profiles.UpdateProfile(ctx, providerprofilerepositories.UpdateProfileParams{
		ProviderID:                   input.ProviderID,
		FirstName:                    firstName,
		LastName:                     strings.TrimSpace(input.LastName),
		Email:                        strings.ToLower(strings.TrimSpace(input.Email)),
		LocationState:                strings.TrimSpace(input.LocationState),
		LocationCity:                 strings.TrimSpace(input.LocationCity),
		OperationMode:                strings.ToLower(strings.TrimSpace(input.OperationMode)),
		ServiceType:                  strings.ToLower(strings.TrimSpace(input.ServiceType)),
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
		return providerauthmodels.PublicProvider{}, err
	}
	return p.Public(), nil
}

// ─── Trucks ───────────────────────────────────────────────────────────────────

type CreateTruckInput struct {
	ProviderID  string
	TruckType   string
	CapacityKg  int
	PlateNumber string
	Year        *int
	Make        *string
	Model       *string
	Color       *string
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
		ID:          uuid.NewString(),
		ProviderID:  input.ProviderID,
		TruckType:   strings.ToLower(input.TruckType),
		CapacityKg:  input.CapacityKg,
		PlateNumber: plate,
		Year:        input.Year,
		Make:        input.Make,
		Model:       input.Model,
		Color:       input.Color,
	}

	created, err := s.trucks.Create(ctx, truck)
	if err != nil {
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
	ID          string
	ProviderID  string
	TruckType   string
	CapacityKg  int
	PlateNumber string
	Year        *int
	Make        *string
	Model       *string
	Color       *string
	Status      string
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
		ID:          input.ID,
		ProviderID:  input.ProviderID,
		TruckType:   strings.ToLower(input.TruckType),
		CapacityKg:  input.CapacityKg,
		PlateNumber: plate,
		Year:        input.Year,
		Make:        input.Make,
		Model:       input.Model,
		Color:       input.Color,
		Status:      input.Status,
	}
	updated, err := s.trucks.Update(ctx, truck)
	if err != nil {
		return providerprofilemodels.PublicTruck{}, err
	}
	return updated.Public(), nil
}
