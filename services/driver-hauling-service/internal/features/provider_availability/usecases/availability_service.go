package availabilityusecases

import (
	"context"
	"time"

	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	providerprofilerepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

type AvailabilityService struct {
	store  availabilityrepositories.AvailabilityStore
	trucks providerprofilerepositories.TruckRepository
	onlineTTL time.Duration
}

func NewAvailabilityService(
	store availabilityrepositories.AvailabilityStore,
	trucks providerprofilerepositories.TruckRepository,
	onlineTTL time.Duration,
) *AvailabilityService {
	return &AvailabilityService{store: store, trucks: trucks, onlineTTL: onlineTTL}
}

// ─── Provider-facing ──────────────────────────────────────────────────────────

type SetAvailabilityInput struct {
	ProviderID string
	Status     string  // "online" | "offline"
	TruckID    string
	Lat        float64
	Lng        float64
}

type AvailabilityResult struct {
	Status string `json:"status"`
}

func (s *AvailabilityService) SetAvailability(ctx context.Context, input SetAvailabilityInput) (AvailabilityResult, error) {
	if input.Status == "offline" {
		if err := s.store.SetOffline(ctx, input.ProviderID); err != nil {
			return AvailabilityResult{}, err
		}
		return AvailabilityResult{Status: "offline"}, nil
	}

	if input.Status != "online" {
		return AvailabilityResult{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "status", Message: "Status must be 'online' or 'offline'."},
		})
	}

	// Gate: provider must have at least one active truck
	count, err := s.trucks.CountActiveByProvider(ctx, input.ProviderID)
	if err != nil {
		return AvailabilityResult{}, err
	}
	if count == 0 {
		return AvailabilityResult{}, apperrors.Forbidden("You must register at least one active truck before going online.", nil)
	}

	// Validate truck belongs to provider if specified
	if input.TruckID != "" {
		if _, err := s.trucks.GetByID(ctx, input.TruckID, input.ProviderID); err != nil {
			return AvailabilityResult{}, apperrors.NotFound("The specified truck was not found.", nil)
		}
	}

	err = s.store.SetOnline(ctx, availabilityrepositories.ProviderStatus{
		ProviderID: input.ProviderID,
		TruckID:    input.TruckID,
		Lat:        input.Lat,
		Lng:        input.Lng,
		UpdatedAt:  nowUnix(),
	}, s.onlineTTL)
	if err != nil {
		return AvailabilityResult{}, err
	}
	return AvailabilityResult{Status: "online"}, nil
}

type HeartbeatInput struct {
	ProviderID string
	Lat        float64
	Lng        float64
}

func (s *AvailabilityService) Heartbeat(ctx context.Context, input HeartbeatInput) error {
	return s.store.Heartbeat(ctx, input.ProviderID, input.Lat, input.Lng, s.onlineTTL)
}

func (s *AvailabilityService) GetStatus(ctx context.Context, providerID string) (AvailabilityResult, error) {
	_, ok, err := s.store.GetProviderStatus(ctx, providerID)
	if err != nil {
		return AvailabilityResult{}, err
	}
	if !ok {
		return AvailabilityResult{Status: "offline"}, nil
	}
	return AvailabilityResult{Status: "online"}, nil
}

// ─── Customer-facing ──────────────────────────────────────────────────────────

type CustomerAvailabilityResult struct {
	Available bool  `json:"available"`
	Count     int64 `json:"count"`
}

func (s *AvailabilityService) CheckAvailability(ctx context.Context) (CustomerAvailabilityResult, error) {
	// Use the live provider list (which prunes stale set entries) rather than a
	// raw SCARD, so the gate doesn't report "available" off expired keys and then
	// immediately unmatch the booking.
	providers, err := s.store.GetOnlineProviders(ctx)
	if err != nil {
		return CustomerAvailabilityResult{}, err
	}
	count := int64(len(providers))
	return CustomerAvailabilityResult{
		Available: count > 0,
		Count:     count,
	}, nil
}

func nowUnix() int64 {
	return time.Now().Unix()
}
