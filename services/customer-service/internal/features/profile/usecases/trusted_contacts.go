package profileusecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	profilerepositories "cosmicforge/logistics/services/customer-service/internal/features/profile/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

// TrustedContactService is the /trusted-contacts presentation of the
// emergency_contacts entity. Trip sharing is explicit consent only.
type TrustedContactService struct {
	repo    profilerepositories.TrustedContactRepository
	enabled bool
}

func NewTrustedContactService(repo profilerepositories.TrustedContactRepository, enabled bool) *TrustedContactService {
	return &TrustedContactService{repo: repo, enabled: enabled}
}

// TrustedContactInput carries client-editable fields. Consent flags are
// pointers so an omitted flag means false on create, never implied true.
type TrustedContactInput struct {
	Name                   string
	Phone                  string
	RelationshipLabel      *string
	EmergencyAlertsEnabled *bool
	TripSharingEnabled     *bool
}

func (s *TrustedContactService) guard() error {
	if s == nil || !s.enabled || s.repo == nil {
		return apperrors.Unavailable("Trusted contacts are not available yet.", nil)
	}
	return nil
}

func (s *TrustedContactService) List(ctx context.Context, customerID string) ([]profilemodels.TrustedContact, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	return s.repo.ListTrustedContacts(ctx, customerID)
}

func (s *TrustedContactService) Create(ctx context.Context, customerID string, input TrustedContactInput, requestKey string) (profilemodels.TrustedContact, bool, error) {
	if err := s.guard(); err != nil {
		return profilemodels.TrustedContact{}, false, err
	}
	requestKey = strings.TrimSpace(requestKey)
	if len(requestKey) > 128 {
		return profilemodels.TrustedContact{}, false, apperrors.BadRequest("Idempotency-Key is too long.", nil)
	}
	contact := input.model(customerID, "")
	if err := contact.Normalize(); err != nil {
		return profilemodels.TrustedContact{}, false, err
	}
	created, replayed, err := s.repo.CreateTrustedContact(ctx, contact, requestKey, trustedContactFingerprint(contact))
	if errors.Is(err, profilerepositories.ErrContactRequestKeyReused) {
		return created, false, apperrors.Conflict("This Idempotency-Key was already used for a different contact.", err)
	}
	return created, replayed, err
}

// Update replaces all editable fields. Omitted consent flags become false,
// so a client can never enable sharing by accident.
func (s *TrustedContactService) Update(ctx context.Context, customerID, id string, input TrustedContactInput, expectedVersion *int64) (profilemodels.TrustedContact, error) {
	if err := s.guard(); err != nil {
		return profilemodels.TrustedContact{}, err
	}
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return profilemodels.TrustedContact{}, apperrors.NotFound("Contact not found.", nil)
	}
	if expectedVersion == nil || *expectedVersion < 1 {
		return profilemodels.TrustedContact{}, apperrors.Validation("Check the contact's details.", []apperrors.FieldViolation{{Field: "expected_version", Message: "Expected version is required."}})
	}
	contact := input.model(customerID, strings.TrimSpace(id))
	if err := contact.Normalize(); err != nil {
		return profilemodels.TrustedContact{}, err
	}
	updated, err := s.repo.UpdateTrustedContact(ctx, contact, *expectedVersion)
	if errors.Is(err, profilerepositories.ErrContactVersionConflict) {
		return updated, apperrors.Conflict("This contact changed elsewhere. Reload and try again.", err)
	}
	return updated, err
}

func (s *TrustedContactService) Delete(ctx context.Context, customerID, id string) error {
	if err := s.guard(); err != nil {
		return err
	}
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return apperrors.NotFound("Contact not found.", nil)
	}
	return s.repo.DeactivateTrustedContact(ctx, customerID, strings.TrimSpace(id))
}

func (i TrustedContactInput) model(customerID, id string) profilemodels.TrustedContact {
	return profilemodels.TrustedContact{
		ID: id, CustomerID: customerID, Name: i.Name, Phone: i.Phone, RelationshipLabel: i.RelationshipLabel,
		EmergencyAlertsEnabled: i.EmergencyAlertsEnabled != nil && *i.EmergencyAlertsEnabled,
		TripSharingEnabled:     i.TripSharingEnabled != nil && *i.TripSharingEnabled,
	}
}

func trustedContactFingerprint(t profilemodels.TrustedContact) string {
	raw, _ := json.Marshal([]any{t.Name, t.Phone, t.RelationshipLabel, t.EmergencyAlertsEnabled, t.TripSharingEnabled})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
