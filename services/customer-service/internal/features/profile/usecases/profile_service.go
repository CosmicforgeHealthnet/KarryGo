package profileusecases

import (
	"context"
	"io"
	"strings"

	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	profilerepositories "cosmicforge/logistics/services/customer-service/internal/features/profile/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/mediaclient"
)

type ProfileService struct {
	customers  profilerepositories.CustomerRepository
	mediaClient *mediaclient.Client
}

type Options struct {
	Customers   profilerepositories.CustomerRepository
	MediaClient *mediaclient.Client
}

func NewProfileService(opts Options) *ProfileService {
	return &ProfileService{
		customers:   opts.Customers,
		mediaClient: opts.MediaClient,
	}
}

type UploadPhotoInput struct {
	CustomerID  string
	Filename    string
	ContentType string
	Body        io.Reader
}

type UpdateProfileInput struct {
	CustomerID string
	FirstName  string
	LastName   string
}

func (s *ProfileService) GetProfile(ctx context.Context, customerID string) (profilemodels.Customer, error) {
	return s.customers.GetByID(ctx, customerID)
}

func (s *ProfileService) UpdateProfile(ctx context.Context, input UpdateProfileInput) (profilemodels.Customer, error) {
	return s.customers.UpdateProfile(ctx, input.CustomerID, input.FirstName, input.LastName)
}

type SavePhotoURLInput struct {
	CustomerID string
	PhotoURL   string
	AssetID    string
}

func (s *ProfileService) SaveProfilePhotoURL(ctx context.Context, input SavePhotoURLInput) (profilemodels.Customer, error) {
	return s.customers.UpdateProfilePhoto(ctx, input.CustomerID, input.AssetID, input.PhotoURL)
}

// ─── Emergency contacts ───────────────────────────────────────────────────────

type AddEmergencyContactInput struct {
	CustomerID   string
	Name         string
	Phone        string
	Relationship string
}

func (s *ProfileService) GetEmergencyContacts(ctx context.Context, customerID string) ([]profilemodels.EmergencyContact, error) {
	return s.customers.GetEmergencyContacts(ctx, customerID)
}

func (s *ProfileService) AddEmergencyContact(ctx context.Context, input AddEmergencyContactInput) (profilemodels.EmergencyContact, error) {
	name := strings.TrimSpace(input.Name)
	phone := strings.TrimSpace(input.Phone)
	relationship := strings.TrimSpace(input.Relationship)

	var fields []apperrors.FieldViolation
	if name == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "name", Message: "Name is required."})
	}
	if phone == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "phone", Message: "Phone is required."})
	}
	if relationship == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "relationship", Message: "Relationship is required."})
	}
	if len(fields) > 0 {
		return profilemodels.EmergencyContact{}, apperrors.Validation("Please check your details.", fields)
	}

	return s.customers.AddEmergencyContact(ctx, input.CustomerID, name, phone, relationship)
}

func (s *ProfileService) DeleteEmergencyContact(ctx context.Context, id, customerID string) error {
	return s.customers.DeleteEmergencyContact(ctx, id, customerID)
}

func (s *ProfileService) UploadProfilePhoto(ctx context.Context, input UploadPhotoInput) (profilemodels.Customer, error) {
	var assetID, photoURL string

	if s.mediaClient != nil {
		asset, err := s.mediaClient.Upload(ctx, mediaclient.UploadRequest{
			OwnerID:     input.CustomerID,
			Purpose:     "profile_photo",
			Filename:    input.Filename,
			ContentType: input.ContentType,
			Body:        input.Body,
		})
		if err != nil {
			return profilemodels.Customer{}, err
		}
		assetID = asset.ID
		photoURL = asset.PublicURL
	}

	return s.customers.UpdateProfilePhoto(ctx, input.CustomerID, assetID, photoURL)
}
