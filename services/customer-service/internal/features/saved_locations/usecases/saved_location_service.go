package savedlocationusecases

import (
	"context"
	"strings"

	savedlocationmodels "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/models"
	savedlocationrepositories "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

type SavedLocationService struct {
	locations savedlocationrepositories.SavedLocationRepository
}

func NewSavedLocationService(locations savedlocationrepositories.SavedLocationRepository) *SavedLocationService {
	return &SavedLocationService{locations: locations}
}

type SaveLocationInput struct {
	CustomerID  string
	Title       string
	Address     string
	Latitude    float64
	Longitude   float64
	Description string
}

func (s *SavedLocationService) ListLocations(ctx context.Context, customerID string) ([]savedlocationmodels.SavedLocation, error) {
	return s.locations.List(ctx, customerID)
}

func (s *SavedLocationService) AddLocation(ctx context.Context, input SaveLocationInput) (savedlocationmodels.SavedLocation, error) {
	location, err := validatedLocation(input)
	if err != nil {
		return savedlocationmodels.SavedLocation{}, err
	}
	return s.locations.Create(ctx, location)
}

func (s *SavedLocationService) UpdateLocation(ctx context.Context, id string, input SaveLocationInput) (savedlocationmodels.SavedLocation, error) {
	location, err := validatedLocation(input)
	if err != nil {
		return savedlocationmodels.SavedLocation{}, err
	}
	location.ID = id
	return s.locations.Update(ctx, location)
}

func (s *SavedLocationService) DeleteLocation(ctx context.Context, id, customerID string) error {
	return s.locations.Delete(ctx, id, customerID)
}

func validatedLocation(input SaveLocationInput) (savedlocationmodels.SavedLocation, error) {
	title := strings.TrimSpace(input.Title)
	address := strings.TrimSpace(input.Address)

	var fields []apperrors.FieldViolation
	if title == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "title", Message: "Title is required."})
	}
	if address == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "address", Message: "Address is required."})
	}
	if input.Latitude == 0 && input.Longitude == 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "latitude", Message: "A map location is required."})
	}
	if len(fields) > 0 {
		return savedlocationmodels.SavedLocation{}, apperrors.Validation("Please check your details.", fields)
	}

	return savedlocationmodels.SavedLocation{
		CustomerID:  input.CustomerID,
		Title:       title,
		Address:     address,
		Latitude:    input.Latitude,
		Longitude:   input.Longitude,
		Description: strings.TrimSpace(input.Description),
	}, nil
}
