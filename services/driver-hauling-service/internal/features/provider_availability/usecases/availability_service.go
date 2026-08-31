package availabilityusecases

import (
	"context"
	"time"

	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	providerprofilerepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	// IdleHeartbeatSeconds is how often an online-but-unassigned provider
	// reports. It only has to keep the online TTL from lapsing.
	IdleHeartbeatSeconds = 30
	// OnTripHeartbeatSeconds is how often a provider on a job reports. This
	// is what the customer's moving vehicle marker interpolates between, so
	// it is deliberately much tighter than the idle cadence.
	OnTripHeartbeatSeconds = 4
)

type AvailabilityService struct {
	store        availabilityrepositories.AvailabilityStore
	trucks       providerprofilerepositories.TruckRepository
	onlineTTL    time.Duration
	authority    availabilityrepositories.AuthorityStore
	restrictions interface {
		Restricted(context.Context, string, string, string) (bool, error)
	}
}

func (s *AvailabilityService) WithRestrictionGate(gate interface {
	Restricted(context.Context, string, string, string) (bool, error)
}) *AvailabilityService {
	s.restrictions = gate
	return s
}

func NewAvailabilityServiceWithAuthority(store availabilityrepositories.AvailabilityStore, trucks providerprofilerepositories.TruckRepository, onlineTTL time.Duration, authority availabilityrepositories.AuthorityStore) *AvailabilityService {
	return &AvailabilityService{store: store, trucks: trucks, onlineTTL: onlineTTL, authority: authority}
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
	Status     string // "online" | "offline"
	TruckID    string
	Lat        float64
	Lng        float64
}

type AvailabilityResult struct {
	Status string `json:"status"`
}

func (s *AvailabilityService) SetAvailability(ctx context.Context, input SetAvailabilityInput) (AvailabilityResult, error) {
	if input.Status == "offline" {
		if s.authority != nil {
			if err := s.authority.SetAuthorityState(ctx, input.ProviderID, "UNAVAILABLE", false, "PROVIDER_OFFLINE"); err != nil {
				return AvailabilityResult{}, err
			}
		}
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
	if s.restrictions != nil {
		restricted, err := s.restrictions.Restricted(ctx, "", "PROVIDER", input.ProviderID)
		if err != nil {
			return AvailabilityResult{}, apperrors.Unavailable("Restriction authority is unavailable.", err)
		}
		if restricted {
			return AvailabilityResult{}, apperrors.Forbidden("You are temporarily restricted from operations.", nil)
		}
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

	if s.authority != nil {
		if err := s.authority.SetAuthorityState(ctx, input.ProviderID, "AVAILABLE", true, "PROVIDER_ONLINE"); err != nil {
			return AvailabilityResult{}, err
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
		if s.authority != nil {
			_ = s.authority.SetAuthorityState(ctx, input.ProviderID, "UNAVAILABLE", false, "CACHE_WRITE_FAILED")
		}
		return AvailabilityResult{}, err
	}
	return AvailabilityResult{Status: "online"}, nil
}

type HeartbeatInput struct {
	ProviderID string
	Lat        float64
	Lng        float64
	// Heading (degrees), Speed (m/s) and Accuracy (metres) are optional.
	// Devices that report them let the customer's map rotate the vehicle and
	// dead-reckon between fixes; devices that do not still stay online.
	Heading  float64
	Speed    float64
	Accuracy float64
}

func (s *AvailabilityService) Heartbeat(ctx context.Context, input HeartbeatInput) error {
	err := s.store.Heartbeat(ctx, input.ProviderID, availabilityrepositories.Telemetry{
		Lat:      input.Lat,
		Lng:      input.Lng,
		Heading:  input.Heading,
		Speed:    input.Speed,
		Accuracy: input.Accuracy,
	}, s.onlineTTL)
	if err != nil {
		return err
	}
	if s.authority != nil {
		return s.authority.SetAuthorityState(ctx, input.ProviderID, "AVAILABLE", true, "HEARTBEAT")
	}
	return nil
}

// HeartbeatIntervalSeconds tells the provider app how often to report. Idle
// providers only need to hold their online TTL open; a provider on a job is
// driving the customer's live map, so they report far more often.
//
// Returned by the availability endpoints so the cadence can be tuned server
// side without shipping a new app build.
func (s *AvailabilityService) HeartbeatIntervalSeconds(onTrip bool) int {
	if onTrip {
		return OnTripHeartbeatSeconds
	}
	return IdleHeartbeatSeconds
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
