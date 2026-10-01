package mobilityusecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	mobilitymodels "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/models"
	mobilityrepositories "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

type Repository interface {
	GetPreferences(ctx context.Context, customerID string) (mobilitymodels.MobilityPreferences, bool, error)
	SavePreferences(ctx context.Context, p mobilitymodels.MobilityPreferences, expectedVersion int64) (mobilitymodels.MobilityPreferences, error)
	ListGuests(ctx context.Context, customerID string) ([]mobilitymodels.GuestTraveller, error)
	GetGuest(ctx context.Context, customerID, id string) (mobilitymodels.GuestTraveller, error)
	CreateGuest(ctx context.Context, g mobilitymodels.GuestTraveller, requestKey, fingerprint string) (mobilitymodels.GuestTraveller, bool, error)
	UpdateGuest(ctx context.Context, g mobilitymodels.GuestTraveller, expectedVersion int64) (mobilitymodels.GuestTraveller, error)
	DeactivateGuest(ctx context.Context, customerID, id string) error
}

// Flags gate each capability independently so they can roll out separately.
type Flags struct {
	MobilityPreferences bool
	GuestTravellers     bool
}

type Service struct {
	repo  Repository
	flags Flags
}

func NewService(repo Repository, flags Flags) *Service {
	return &Service{repo: repo, flags: flags}
}

const maxRequestKeyLength = 128

func disabled(name string) error {
	return apperrors.Unavailable(name+" is not available yet.", nil)
}

// GetPreferences returns defaults (version 0) when nothing is stored.
func (s *Service) GetPreferences(ctx context.Context, customerID string) (mobilitymodels.MobilityPreferences, error) {
	if !s.flags.MobilityPreferences {
		return mobilitymodels.MobilityPreferences{}, disabled("Mobility preferences")
	}
	stored, found, err := s.repo.GetPreferences(ctx, customerID)
	if err != nil {
		return stored, err
	}
	if !found {
		return mobilitymodels.DefaultPreferences(customerID), nil
	}
	return stored, nil
}

// PutPreferences requires expected_version: 0 creates, N updates version N.
func (s *Service) PutPreferences(ctx context.Context, customerID string, input mobilitymodels.MobilityPreferences, expectedVersion *int64) (mobilitymodels.MobilityPreferences, error) {
	if !s.flags.MobilityPreferences {
		return mobilitymodels.MobilityPreferences{}, disabled("Mobility preferences")
	}
	if expectedVersion == nil || *expectedVersion < 0 {
		return mobilitymodels.MobilityPreferences{}, apperrors.Validation("Check your mobility preferences.", []apperrors.FieldViolation{{Field: "expected_version", Message: "Expected version is required."}})
	}
	input.CustomerID = customerID
	if err := input.Normalize(); err != nil {
		return mobilitymodels.MobilityPreferences{}, err
	}
	saved, err := s.repo.SavePreferences(ctx, input, *expectedVersion)
	if errors.Is(err, mobilityrepositories.ErrVersionConflict) {
		return saved, apperrors.Conflict("Your preferences changed elsewhere. Reload and try again.", err)
	}
	return saved, err
}

func (s *Service) ListGuests(ctx context.Context, customerID string) ([]mobilitymodels.GuestTraveller, error) {
	if !s.flags.GuestTravellers {
		return nil, disabled("Guest travellers")
	}
	return s.repo.ListGuests(ctx, customerID)
}

func (s *Service) GetGuest(ctx context.Context, customerID, id string) (mobilitymodels.GuestTraveller, error) {
	if !s.flags.GuestTravellers {
		return mobilitymodels.GuestTraveller{}, disabled("Guest travellers")
	}
	if !validID(id) {
		return mobilitymodels.GuestTraveller{}, apperrors.NotFound("Traveller not found.", nil)
	}
	return s.repo.GetGuest(ctx, customerID, id)
}

// CreateGuest honours an optional Idempotency-Key. The returned bool is true
// when an earlier identical request was replayed.
func (s *Service) CreateGuest(ctx context.Context, customerID string, input mobilitymodels.GuestTraveller, requestKey string) (mobilitymodels.GuestTraveller, bool, error) {
	if !s.flags.GuestTravellers {
		return mobilitymodels.GuestTraveller{}, false, disabled("Guest travellers")
	}
	requestKey = strings.TrimSpace(requestKey)
	if len(requestKey) > maxRequestKeyLength {
		return mobilitymodels.GuestTraveller{}, false, apperrors.BadRequest("Idempotency-Key is too long.", nil)
	}
	input.ID, input.CustomerID = "", customerID
	if err := input.Normalize(); err != nil {
		return mobilitymodels.GuestTraveller{}, false, err
	}
	created, replayed, err := s.repo.CreateGuest(ctx, input, requestKey, Fingerprint(input))
	if errors.Is(err, mobilityrepositories.ErrRequestKeyReused) {
		return created, false, apperrors.Conflict("This Idempotency-Key was already used for a different traveller.", err)
	}
	return created, replayed, err
}

func (s *Service) UpdateGuest(ctx context.Context, customerID, id string, input mobilitymodels.GuestTraveller, expectedVersion *int64) (mobilitymodels.GuestTraveller, error) {
	if !s.flags.GuestTravellers {
		return mobilitymodels.GuestTraveller{}, disabled("Guest travellers")
	}
	if !validID(id) {
		return mobilitymodels.GuestTraveller{}, apperrors.NotFound("Traveller not found.", nil)
	}
	if expectedVersion == nil || *expectedVersion < 1 {
		return mobilitymodels.GuestTraveller{}, apperrors.Validation("Check the traveller's details.", []apperrors.FieldViolation{{Field: "expected_version", Message: "Expected version is required."}})
	}
	input.ID, input.CustomerID = id, customerID
	if err := input.Normalize(); err != nil {
		return mobilitymodels.GuestTraveller{}, err
	}
	updated, err := s.repo.UpdateGuest(ctx, input, *expectedVersion)
	if errors.Is(err, mobilityrepositories.ErrVersionConflict) {
		return updated, apperrors.Conflict("This traveller changed elsewhere. Reload and try again.", err)
	}
	return updated, err
}

func (s *Service) DeleteGuest(ctx context.Context, customerID, id string) error {
	if !s.flags.GuestTravellers {
		return disabled("Guest travellers")
	}
	if !validID(id) {
		return apperrors.NotFound("Traveller not found.", nil)
	}
	return s.repo.DeactivateGuest(ctx, customerID, id)
}

// Fingerprint hashes the normalized, customer-visible fields of a create.
func Fingerprint(g mobilitymodels.GuestTraveller) string {
	raw, _ := json.Marshal([]any{g.Name, g.Phone, g.RelationshipLabel, g.AccessibilityNotes, g.MobilityRequirementsJSON})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validID(id string) bool {
	_, err := uuid.Parse(strings.TrimSpace(id))
	return err == nil
}
