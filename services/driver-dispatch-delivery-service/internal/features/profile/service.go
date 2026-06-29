package profile

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

type AuthContext struct {
	ProviderID    string
	PhoneNumber   string
	CorrelationID string
}

type OnboardingInput struct {
	FullName      string        `json:"full_name"`
	Email         *string       `json:"email,omitempty"`
	State         string        `json:"state"`
	City          string        `json:"city"`
	OperationType OperationType `json:"operation_type"`
}

type UpdateProviderInput struct {
	FullName        *string `json:"full_name,omitempty"`
	Email           *string `json:"email,omitempty"`
	State           *string `json:"state,omitempty"`
	City            *string `json:"city,omitempty"`
	ProfilePhotoURL *string `json:"profile_photo_url,omitempty"`
}

type EmergencyContactInput struct {
	FullName     string `json:"full_name"`
	Phone        string `json:"phone"`
	Relationship string `json:"relationship"`
}

type GuarantorInput struct {
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
}

type RatingInput struct {
	ProviderID        string
	BookingID         string
	RatedByCustomerID string
	Score             int
	Comment           *string
}

// SessionRevoker allows the profile service to revoke all auth sessions for a
// provider on account deletion without taking a hard dependency on the auth
// package.
type SessionRevoker interface {
	RevokeAllByProviderID(ctx context.Context, providerID string) (int64, error)
}

type ProviderService interface {
	Onboarding(ctx context.Context, auth AuthContext, input OnboardingInput) (MeResponse, error)
	GetMe(ctx context.Context, auth AuthContext) (MeResponse, error)
	UpdateMe(ctx context.Context, auth AuthContext, input UpdateProviderInput) (MeResponse, error)
	SetEmergencyContact(ctx context.Context, auth AuthContext, input EmergencyContactInput) (EmergencyContact, error)
	GetEmergencyContact(ctx context.Context, auth AuthContext) (EmergencyContact, error)
	SetGuarantor(ctx context.Context, auth AuthContext, input GuarantorInput) (Guarantor, error)
	GetGuarantor(ctx context.Context, auth AuthContext) (Guarantor, error)
	GetStats(ctx context.Context, auth AuthContext) (Stats, error)
	GetPublicProfile(ctx context.Context, providerID string) (PublicProfile, error)
	GetSettings(ctx context.Context, auth AuthContext) (ProviderSettings, error)
	UpdateSettings(ctx context.Context, auth AuthContext, input UpdateSettingsInput) (ProviderSettings, error)
	UploadAvatar(ctx context.Context, auth AuthContext, uploader AvatarUploader, file AvatarFile, filename string, size int64, contentType string) (MeResponse, error)
	DeleteMe(ctx context.Context, auth AuthContext) error
}

type Service struct {
	repository     Repository
	events         ProfileEventPublisher
	sessionRevoker SessionRevoker
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func NewServiceWithEvents(repository Repository, events ProfileEventPublisher) *Service {
	return &Service{repository: repository, events: events}
}

func NewServiceWithSessionRevoker(repository Repository, events ProfileEventPublisher, revoker SessionRevoker) *Service {
	return &Service{repository: repository, events: events, sessionRevoker: revoker}
}

func (s *Service) Onboarding(ctx context.Context, auth AuthContext, input OnboardingInput) (MeResponse, error) {
	if err := validateAuth(auth); err != nil {
		return MeResponse{}, err
	}
	input, err := validateOnboarding(input)
	if err != nil {
		return MeResponse{}, err
	}
	provider, ok, dbErr := s.repository.GetProviderByID(ctx, auth.ProviderID)
	if dbErr != nil {
		return MeResponse{}, dbErr
	}
	if !ok {
		return MeResponse{}, apperrors.NotFound("Provider was not found.", nil)
	}
	if provider.OnboardingComplete {
		return MeResponse{}, apperrors.Conflict("Onboarding has already been completed.", nil)
	}
	if _, err := s.repository.UpdateOnboarding(ctx, auth.ProviderID, input); err != nil {
		return MeResponse{}, err
	}
	updated, err := s.checkOnboardingComplete(ctx, auth)
	if err != nil {
		return MeResponse{}, err
	}
	return s.buildMeResponse(ctx, updated)
}

func (s *Service) GetMe(ctx context.Context, auth AuthContext) (MeResponse, error) {
	if err := validateAuth(auth); err != nil {
		return MeResponse{}, err
	}
	provider, ok, err := s.repository.GetProviderByID(ctx, auth.ProviderID)
	if err != nil {
		return MeResponse{}, err
	}
	if !ok {
		return MeResponse{}, apperrors.NotFound("Provider was not found.", nil)
	}
	return s.buildMeResponse(ctx, provider)
}

func (s *Service) UpdateMe(ctx context.Context, auth AuthContext, input UpdateProviderInput) (MeResponse, error) {
	if err := validateAuth(auth); err != nil {
		return MeResponse{}, err
	}
	if !hasUpdateFields(input) {
		return MeResponse{}, validationBadRequest("No fields to update.", []apperrors.FieldViolation{
			{Field: "body", Message: "No fields to update."},
		})
	}
	input, err := validateUpdate(input)
	if err != nil {
		return MeResponse{}, err
	}
	_, ok, dbErr := s.repository.GetProviderByID(ctx, auth.ProviderID)
	if dbErr != nil {
		return MeResponse{}, dbErr
	}
	if !ok {
		return MeResponse{}, apperrors.NotFound("Provider was not found.", nil)
	}
	if _, err := s.repository.PatchProvider(ctx, auth.ProviderID, input); err != nil {
		return MeResponse{}, err
	}
	updated, err := s.checkOnboardingComplete(ctx, auth)
	if err != nil {
		return MeResponse{}, err
	}
	if s.events != nil {
		_ = s.events.PublishProfileUpdated(ctx, ProfileUpdatedEvent{
			Event:         TopicProfileUpdated,
			CorrelationID: auth.CorrelationID,
			ProviderID:    auth.ProviderID,
			ChangedFields: changedFields(input),
			CreatedAt:     time.Now().UTC(),
		})
	}
	return s.buildMeResponse(ctx, updated)
}

func (s *Service) SetEmergencyContact(ctx context.Context, auth AuthContext, input EmergencyContactInput) (EmergencyContact, error) {
	if err := validateAuth(auth); err != nil {
		return EmergencyContact{}, err
	}
	input, err := validateEmergencyContact(input)
	if err != nil {
		return EmergencyContact{}, err
	}
	if _, err := s.repository.EnsureProvider(ctx, auth.ProviderID, auth.PhoneNumber); err != nil {
		return EmergencyContact{}, err
	}

	contact, err := s.repository.UpsertEmergencyContact(ctx, auth.ProviderID, input)
	if err != nil {
		return EmergencyContact{}, err
	}
	if _, err := s.checkOnboardingComplete(ctx, auth); err != nil {
		return EmergencyContact{}, err
	}
	return contact, nil
}

func (s *Service) GetEmergencyContact(ctx context.Context, auth AuthContext) (EmergencyContact, error) {
	if err := validateAuth(auth); err != nil {
		return EmergencyContact{}, err
	}
	if _, err := s.repository.EnsureProvider(ctx, auth.ProviderID, auth.PhoneNumber); err != nil {
		return EmergencyContact{}, err
	}
	contact, ok, err := s.repository.GetEmergencyContact(ctx, auth.ProviderID)
	if err != nil {
		return EmergencyContact{}, err
	}
	if !ok {
		return EmergencyContact{}, apperrors.NotFound("Emergency contact was not found.", nil)
	}
	return contact, nil
}

func (s *Service) SetGuarantor(ctx context.Context, auth AuthContext, input GuarantorInput) (Guarantor, error) {
	if err := validateAuth(auth); err != nil {
		return Guarantor{}, err
	}
	input, err := validateGuarantor(input)
	if err != nil {
		return Guarantor{}, err
	}
	if _, err := s.repository.EnsureProvider(ctx, auth.ProviderID, auth.PhoneNumber); err != nil {
		return Guarantor{}, err
	}

	guarantor, err := s.repository.UpsertGuarantor(ctx, auth.ProviderID, input)
	if err != nil {
		return Guarantor{}, err
	}
	if _, err := s.checkOnboardingComplete(ctx, auth); err != nil {
		return Guarantor{}, err
	}
	return guarantor, nil
}

func (s *Service) GetGuarantor(ctx context.Context, auth AuthContext) (Guarantor, error) {
	if err := validateAuth(auth); err != nil {
		return Guarantor{}, err
	}
	if _, err := s.repository.EnsureProvider(ctx, auth.ProviderID, auth.PhoneNumber); err != nil {
		return Guarantor{}, err
	}
	guarantor, ok, err := s.repository.GetGuarantor(ctx, auth.ProviderID)
	if err != nil {
		return Guarantor{}, err
	}
	if !ok {
		return Guarantor{}, apperrors.NotFound("Guarantor was not found.", nil)
	}
	return guarantor, nil
}

func (s *Service) GetStats(ctx context.Context, auth AuthContext) (Stats, error) {
	if err := validateAuth(auth); err != nil {
		return Stats{}, err
	}
	provider, ok, err := s.repository.GetProviderByID(ctx, auth.ProviderID)
	if err != nil {
		return Stats{}, err
	}
	if !ok {
		return Stats{}, apperrors.NotFound("Provider was not found.", nil)
	}
	ratingsCount, err := s.repository.CountRatings(ctx, auth.ProviderID)
	if err != nil {
		return Stats{}, err
	}
	completionRate := 0.0
	if provider.TotalTrips > 0 {
		completionRate = 1.0
	}
	return Stats{
		TotalTrips:         provider.TotalTrips,
		AvgRating:          provider.AvgRating,
		RatingsCount:       ratingsCount,
		CompletionRate:     completionRate,
		IsActive:           provider.IsActive,
		VerificationStatus: provider.VerificationStatus,
	}, nil
}

func (s *Service) GetSettings(ctx context.Context, auth AuthContext) (ProviderSettings, error) {
	if err := validateAuth(auth); err != nil {
		return ProviderSettings{}, err
	}
	return s.repository.GetOrCreateSettings(ctx, auth.ProviderID)
}

var validLanguages = map[string]struct{}{
	"en": {}, "fr": {}, "es": {}, "it": {}, "pt": {}, "yo": {}, "ig": {}, "ha": {},
}

func (s *Service) UpdateSettings(ctx context.Context, auth AuthContext, input UpdateSettingsInput) (ProviderSettings, error) {
	if err := validateAuth(auth); err != nil {
		return ProviderSettings{}, err
	}
	if input.PushEnabled == nil && input.SMSEnabled == nil && input.Language == nil && input.DarkModeEnabled == nil {
		return ProviderSettings{}, validationBadRequest("No fields to update.", []apperrors.FieldViolation{
			{Field: "body", Message: "No fields to update."},
		})
	}
	if input.Language != nil {
		lang := strings.TrimSpace(*input.Language)
		if _, ok := validLanguages[lang]; !ok {
			return ProviderSettings{}, validationBadRequest("Check your details.", []apperrors.FieldViolation{
				{Field: "language", Message: "Language must be one of: en, fr, es, it, pt, yo, ig, ha."},
			})
		}
		input.Language = &lang
	}
	// Ensure row exists before patching.
	if _, err := s.repository.GetOrCreateSettings(ctx, auth.ProviderID); err != nil {
		return ProviderSettings{}, err
	}
	return s.repository.UpdateSettings(ctx, auth.ProviderID, input)
}

// AvatarFile is an io.ReadSeeker — enough for magic-bytes check + upload.
type AvatarFile interface {
	io.ReadSeeker
}

const MaxAvatarSize int64 = 3 << 20 // 3 MB

var (
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	pngMagic  = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
)

func (s *Service) UploadAvatar(ctx context.Context, auth AuthContext, uploader AvatarUploader, file AvatarFile, filename string, size int64, contentType string) (MeResponse, error) {
	if err := validateAuth(auth); err != nil {
		return MeResponse{}, err
	}
	if size > MaxAvatarSize {
		return MeResponse{}, validationBadRequest("Check your details.", []apperrors.FieldViolation{
			{Field: "avatar", Message: "Avatar must be smaller than 3 MB."},
		})
	}

	// Read magic bytes to verify actual MIME type.
	header := make([]byte, 8)
	if _, err := file.Read(header); err != nil {
		return MeResponse{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "avatar", Message: "Unable to read file."},
		})
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return MeResponse{}, apperrors.Internal("File seek failed.", err)
	}

	isJPEG := len(header) >= 3 && header[0] == jpegMagic[0] && header[1] == jpegMagic[1] && header[2] == jpegMagic[2]
	isPNG := len(header) >= 8 &&
		header[0] == pngMagic[0] && header[1] == pngMagic[1] &&
		header[2] == pngMagic[2] && header[3] == pngMagic[3] &&
		header[4] == pngMagic[4] && header[5] == pngMagic[5] &&
		header[6] == pngMagic[6] && header[7] == pngMagic[7]
	if !isJPEG && !isPNG {
		return MeResponse{}, validationBadRequest("Check your details.", []apperrors.FieldViolation{
			{Field: "avatar", Message: "Avatar must be a JPEG or PNG image."},
		})
	}
	declaredType := strings.ToLower(strings.TrimSpace(contentType))
	if declaredType != "image/jpeg" && declaredType != "image/png" {
		return MeResponse{}, validationBadRequest("Check your details.", []apperrors.FieldViolation{
			{Field: "avatar", Message: "Avatar must be declared as image/jpeg or image/png."},
		})
	}
	if (declaredType == "image/jpeg" && !isJPEG) || (declaredType == "image/png" && !isPNG) {
		return MeResponse{}, validationBadRequest("Check your details.", []apperrors.FieldViolation{
			{Field: "avatar", Message: "Avatar content does not match its declared MIME type."},
		})
	}

	objectPath := fmt.Sprintf("profiles/%s/avatar_%s%s", auth.ProviderID, uuid.NewString(), avatarExtension(declaredType))
	url, err := uploader.Upload(ctx, objectPath, file)
	if err != nil {
		return MeResponse{}, apperrors.Internal("Avatar upload failed.", err)
	}

	if err := s.repository.UpdateProfilePhotoURL(ctx, auth.ProviderID, url); err != nil {
		return MeResponse{}, err
	}

	if s.events != nil {
		_ = s.events.PublishProfileUpdated(ctx, ProfileUpdatedEvent{
			Event:         TopicProfileUpdated,
			CorrelationID: auth.CorrelationID,
			ProviderID:    auth.ProviderID,
			ChangedFields: []string{"profile_photo_url"},
			CreatedAt:     time.Now().UTC(),
		})
	}

	provider, ok, err := s.repository.GetProviderByID(ctx, auth.ProviderID)
	if err != nil {
		return MeResponse{}, err
	}
	if !ok {
		return MeResponse{}, apperrors.NotFound("Provider was not found.", nil)
	}
	return s.buildMeResponse(ctx, provider)
}

func (s *Service) DeleteMe(ctx context.Context, auth AuthContext) error {
	if err := validateAuth(auth); err != nil {
		return err
	}
	_, ok, err := s.repository.GetProviderByID(ctx, auth.ProviderID)
	if err != nil {
		return err
	}
	if !ok {
		return apperrors.NotFound("Provider was not found.", nil)
	}
	if err := s.repository.DeactivateProvider(ctx, auth.ProviderID); err != nil {
		return err
	}
	// Revoke all active sessions so existing JWTs can no longer be used after
	// the access-token TTL expires and a refresh is attempted.
	if s.sessionRevoker != nil {
		if _, err := s.sessionRevoker.RevokeAllByProviderID(ctx, auth.ProviderID); err != nil {
			log.Printf("profile: DeleteMe: session revocation failed for provider_id=%s: %v", auth.ProviderID, err)
		}
	}
	return nil
}

func avatarExtension(contentType string) string {
	if contentType == "image/png" {
		return ".png"
	}
	return ".jpg"
}

func (s *Service) GetPublicProfile(ctx context.Context, providerID string) (PublicProfile, error) {
	id := strings.TrimSpace(providerID)
	if _, err := uuid.Parse(id); err != nil {
		return PublicProfile{}, validationBadRequest("Check your details.", []apperrors.FieldViolation{
			{Field: "id", Message: "Provider ID must be a valid UUID."},
		})
	}
	provider, ok, err := s.repository.GetProviderByID(ctx, id)
	if err != nil {
		return PublicProfile{}, err
	}
	if !ok || !provider.IsActive || provider.VerificationStatus == StatusSuspended {
		return PublicProfile{}, apperrors.NotFound("Provider was not found.", nil)
	}
	return PublicProfile{
		ProviderID:         provider.ID,
		FullName:           provider.FullName,
		ProfilePhotoURL:    provider.ProfilePhotoURL,
		VerificationStatus: provider.VerificationStatus,
		AvgRating:          provider.AvgRating,
		TotalTrips:         provider.TotalTrips,
	}, nil
}

func (s *Service) checkOnboardingComplete(ctx context.Context, auth AuthContext) (Provider, error) {
	current, ok, err := s.repository.GetProviderByID(ctx, auth.ProviderID)
	if err != nil {
		return Provider{}, err
	}
	if !ok {
		return Provider{}, apperrors.NotFound("Provider was not found.", nil)
	}
	if current.OnboardingComplete {
		log.Printf("profile onboarding already complete provider_id=%s; %s will not replay", auth.ProviderID, TopicOnboardingCompleted)
		return current, nil
	}

	updated, err := s.repository.RecalculateOnboardingComplete(ctx, auth.ProviderID)
	if err != nil {
		return Provider{}, err
	}
	if !updated.OnboardingComplete {
		return updated, nil
	}

	if s.events != nil {
		operationType := ""
		if updated.OperationType != nil {
			operationType = string(*updated.OperationType)
		}
		event := OnboardingCompletedEvent{
			Event:         TopicOnboardingCompleted,
			CorrelationID: auth.CorrelationID,
			ProviderID:    auth.ProviderID,
			Phone:         updated.Phone,
			OperationType: operationType,
			CreatedAt:     time.Now().UTC(),
		}
		log.Printf("profile onboarding completed provider_id=%s publishing %s", auth.ProviderID, TopicOnboardingCompleted)
		if err := s.events.PublishOnboardingCompleted(ctx, event); err != nil {
			return Provider{}, fmt.Errorf("publish onboarding completed: %w", err)
		}
	} else {
		log.Printf("profile onboarding completed provider_id=%s but publisher is nil; %s not published", auth.ProviderID, TopicOnboardingCompleted)
	}
	return updated, nil
}

// computeSupportID derives a human-readable support identifier from a provider UUID.
// Format: KG-XXXXXXXX where XXXXXXXX is the first 8 hex chars of the UUID, uppercase.
// No DB storage — computed on every read.
func computeSupportID(id string) string {
	if idx := strings.IndexByte(id, '-'); idx > 0 {
		return "KG-" + strings.ToUpper(id[:idx])
	}
	if len(id) >= 8 {
		return "KG-" + strings.ToUpper(id[:8])
	}
	return "KG-" + strings.ToUpper(id)
}

func (s *Service) buildMeResponse(ctx context.Context, provider Provider) (MeResponse, error) {
	_, hasContact, err := s.repository.GetEmergencyContact(ctx, provider.ID)
	if err != nil {
		return MeResponse{}, err
	}
	_, hasGuarantor, err := s.repository.GetGuarantor(ctx, provider.ID)
	if err != nil {
		return MeResponse{}, err
	}
	return MeResponse{
		ProviderID:          provider.ID,
		SupportID:           computeSupportID(provider.ID),
		Phone:               provider.Phone,
		FullName:            provider.FullName,
		Email:               provider.Email,
		State:               provider.State,
		City:                provider.City,
		Country:             provider.Country,
		ProfilePhotoURL:     provider.ProfilePhotoURL,
		OperationType:       provider.OperationType,
		VerificationStatus:  provider.VerificationStatus,
		AvgRating:           provider.AvgRating,
		TotalTrips:          provider.TotalTrips,
		IsActive:            provider.IsActive,
		OnboardingComplete:  provider.OnboardingComplete,
		HasEmergencyContact: hasContact,
		HasGuarantor:        hasGuarantor,
		CreatedAt:           provider.CreatedAt,
	}, nil
}

func hasUpdateFields(input UpdateProviderInput) bool {
	return input.FullName != nil || input.Email != nil || input.State != nil ||
		input.City != nil || input.ProfilePhotoURL != nil
}

func changedFields(input UpdateProviderInput) []string {
	var fields []string
	if input.FullName != nil {
		fields = append(fields, "full_name")
	}
	if input.Email != nil {
		fields = append(fields, "email")
	}
	if input.State != nil {
		fields = append(fields, "state")
	}
	if input.City != nil {
		fields = append(fields, "city")
	}
	if input.ProfilePhotoURL != nil {
		fields = append(fields, "profile_photo_url")
	}
	return fields
}

func validateAuth(auth AuthContext) error {
	if strings.TrimSpace(auth.ProviderID) == "" || strings.TrimSpace(auth.PhoneNumber) == "" {
		return apperrors.Unauthorized("Access token is invalid.", nil)
	}
	return nil
}

func validateOnboarding(input OnboardingInput) (OnboardingInput, error) {
	var fields []apperrors.FieldViolation
	fullName, err := validateName("full_name", input.FullName, true, true)
	if err != nil {
		fields = append(fields, *err)
	}
	state, err := validateText("state", input.State, true, 2, 100)
	if err != nil {
		fields = append(fields, *err)
	}
	city, err := validateText("city", input.City, true, 2, 100)
	if err != nil {
		fields = append(fields, *err)
	}
	if input.OperationType != OperationIndividual && input.OperationType != OperationFleet {
		fields = append(fields, apperrors.FieldViolation{Field: "operation_type", Message: "Operation type must be individual or fleet."})
	}
	email, err := normalizeOptionalEmail(input.Email)
	if err != nil {
		fields = append(fields, *err)
	}
	if len(fields) > 0 {
		return OnboardingInput{}, validationBadRequest("Check your details.", fields)
	}
	input.FullName = fullName
	input.State = state
	input.City = city
	input.Email = email
	return input, nil
}

func validateUpdate(input UpdateProviderInput) (UpdateProviderInput, error) {
	var fields []apperrors.FieldViolation
	if input.FullName != nil {
		fullName, err := validateName("full_name", *input.FullName, true, true)
		if err != nil {
			fields = append(fields, *err)
		} else {
			input.FullName = &fullName
		}
	}
	if input.State != nil {
		state, err := validateText("state", *input.State, true, 2, 100)
		if err != nil {
			fields = append(fields, *err)
		} else {
			input.State = &state
		}
	}
	if input.City != nil {
		city, err := validateText("city", *input.City, true, 2, 100)
		if err != nil {
			fields = append(fields, *err)
		} else {
			input.City = &city
		}
	}
	email, err := normalizeOptionalEmail(input.Email)
	if err != nil {
		fields = append(fields, *err)
	} else {
		input.Email = email
	}
	if input.ProfilePhotoURL != nil {
		photoURL, err := validateHTTPSURL(*input.ProfilePhotoURL)
		if err != nil {
			fields = append(fields, *err)
		} else {
			input.ProfilePhotoURL = &photoURL
		}
	}
	if len(fields) > 0 {
		return UpdateProviderInput{}, validationBadRequest("Check your details.", fields)
	}
	return input, nil
}

func validateEmergencyContact(input EmergencyContactInput) (EmergencyContactInput, error) {
	var fields []apperrors.FieldViolation
	fullName, err := validateText("full_name", input.FullName, true, 2, 100)
	if err != nil {
		fields = append(fields, *err)
	}
	phone, phoneErr := normalizeRequiredPhone(input.Phone)
	if phoneErr != nil {
		fields = append(fields, *phoneErr)
	}
	relationship, err := validateText("relationship", input.Relationship, true, 2, 50)
	if err != nil {
		fields = append(fields, *err)
	}
	if len(fields) > 0 {
		return EmergencyContactInput{}, validationBadRequest("Check your details.", fields)
	}
	input.FullName = fullName
	input.Phone = phone
	input.Relationship = relationship
	return input, nil
}

func validateGuarantor(input GuarantorInput) (GuarantorInput, error) {
	var fields []apperrors.FieldViolation
	fullName, err := validateText("full_name", input.FullName, true, 2, 100)
	if err != nil {
		fields = append(fields, *err)
	}
	phone, phoneErr := normalizeRequiredPhone(input.Phone)
	if phoneErr != nil {
		fields = append(fields, *phoneErr)
	}
	if len(fields) > 0 {
		return GuarantorInput{}, validationBadRequest("Check your details.", fields)
	}
	input.FullName = fullName
	input.Phone = phone
	return input, nil
}

func validateName(field string, value string, required bool, requireFullName bool) (string, *apperrors.FieldViolation) {
	trimmed, err := validateText(field, value, required, 2, 100)
	if err != nil {
		return "", err
	}
	if requireFullName && len(strings.Fields(trimmed)) < 2 {
		return "", &apperrors.FieldViolation{Field: field, Message: "Full name must include first and last name."}
	}
	return trimmed, nil
}

func validateText(field string, value string, required bool, minLength int, maxLength int) (string, *apperrors.FieldViolation) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if required {
			return "", &apperrors.FieldViolation{Field: field, Message: "This field is required."}
		}
		return "", nil
	}
	if len(trimmed) < minLength || len(trimmed) > maxLength {
		return "", &apperrors.FieldViolation{Field: field, Message: "This field length is invalid."}
	}
	return trimmed, nil
}

func normalizeOptionalEmail(value *string) (*string, *apperrors.FieldViolation) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed.Address != trimmed {
		return nil, &apperrors.FieldViolation{Field: "email", Message: "Email address is invalid."}
	}
	lower := strings.ToLower(trimmed)
	return &lower, nil
}

func validateHTTPSURL(value string) (string, *apperrors.FieldViolation) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", &apperrors.FieldViolation{Field: "profile_photo_url", Message: "Profile photo URL must be a valid HTTPS URL."}
	}
	return trimmed, nil
}

func normalizeRequiredPhone(value string) (string, *apperrors.FieldViolation) {
	phone, err := authusecases.NormalizePhoneNumber(value)
	if err != nil {
		return "", &apperrors.FieldViolation{Field: "phone", Message: "Phone number must be in E.164 format."}
	}
	return phone, nil
}

func validationBadRequest(message string, fields []apperrors.FieldViolation) *apperrors.Error {
	err := apperrors.New(http.StatusBadRequest, apperrors.CodeValidationFailed, message, nil)
	err.Fields = fields
	return err
}
