package availabilityusecases

import (
	"context"
	"strings"
	"time"

	marketmodels "cosmicforge/logistics/services/taxi-service/internal/features/markets/models"
	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	providerprofilemodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/models"
	"cosmicforge/logistics/services/taxi-service/internal/vehicleclass"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
)

// Heartbeat cadence is server-driven so it can be retuned without an app
// release. A driver on a ride reports far more often because the passenger is
// watching the car move on their map.
const (
	IdleHeartbeatSeconds   = 30
	OnTripHeartbeatSeconds = 4
)

// Distinct reasons per gate failure, so the driver app can render the exact
// problem rather than one generic "cannot go online".
//
// These travel in the error envelope's `details.reason` rather than its `code`:
// `code` is tied to the HTTP status (all of these are 403 forbidden), while
// `details` is the free-form slot for machine-readable context.
//
// The first five values are re-exported from providerauthmodels.CanGoAvailable
// so this package has one literal copy of each string, not two that could
// drift. ReasonNoActiveVehicle has no equivalent there because it is the one
// dimension CanGoAvailable deliberately excludes -- see that method's comment.
const (
	ReasonAccountSuspended            = providerauthmodels.ReasonAccountSuspended
	ReasonProfileIncomplete           = providerauthmodels.ReasonOnboardingIncomplete
	ReasonIdentityVerificationPending = providerauthmodels.ReasonIdentityVerificationRequired
	ReasonVehicleVerificationPending  = providerauthmodels.ReasonVehicleVerificationRequired
	ReasonDocumentComplianceRequired  = providerauthmodels.ReasonDocumentComplianceRequired
	ReasonNoActiveVehicle             = "no_active_vehicle"
	// ReasonOutsideServiceArea fires when a driver's or passenger's coordinate
	// does not fall inside any active operating zone's coverage circle.
	ReasonOutsideServiceArea = "outside_service_area"
)

// gateMessages gives each CanGoAvailable failure reason its passenger-facing
// wording. Kept as a lookup table (not a switch inline in SetAvailability) so
// adding a seventh dimension later is a one-line addition here, not a new
// branch scattered through the gate logic.
var gateMessages = map[string]string{
	ReasonAccountSuspended:            "Your account is suspended. Contact support.",
	ReasonProfileIncomplete:           "Complete your profile before going online.",
	ReasonIdentityVerificationPending: "Your identity verification is not complete yet.",
	ReasonVehicleVerificationPending:  "Your vehicle verification is not complete yet.",
	ReasonDocumentComplianceRequired:  "One of your required documents is missing or has expired.",
}

// gateError builds the 403 a failed online-gate check returns, tagging it with
// the specific reason the driver app switches on.
func gateError(message, reason string) *apperrors.Error {
	err := apperrors.Forbidden(message, nil)
	err.Details = map[string]interface{}{"reason": reason}
	return err
}

// ProviderLookup is the slice of the driver record the gate needs.
type ProviderLookup interface {
	GetByID(ctx context.Context, id string) (providerauthmodels.Provider, error)
}

// VehicleLookup is the slice of the vehicle repo the gate needs.
type VehicleLookup interface {
	CountActiveByProvider(ctx context.Context, providerID string) (int, error)
	GetByID(ctx context.Context, id, providerID string) (providerprofilemodels.Vehicle, error)
	FirstActiveByProvider(ctx context.Context, providerID string) (providerprofilemodels.Vehicle, error)
}

// MarketLookup resolves which operating zone (and therefore which market and
// geo-index partition) a coordinate belongs to.
type MarketLookup interface {
	ResolveZone(ctx context.Context, point geo.Point) (marketmodels.Zone, error)
}

type AvailabilityService struct {
	store            availabilityrepositories.AvailabilityStore
	operationalState availabilityrepositories.OperationalStateRepository
	providers        ProviderLookup
	vehicles         VehicleLookup
	markets          MarketLookup
	onlineTTL        time.Duration
}

func NewAvailabilityService(
	store availabilityrepositories.AvailabilityStore,
	operationalState availabilityrepositories.OperationalStateRepository,
	providers ProviderLookup,
	vehicles VehicleLookup,
	markets MarketLookup,
	onlineTTL time.Duration,
) *AvailabilityService {
	return &AvailabilityService{
		store: store, operationalState: operationalState,
		providers: providers, vehicles: vehicles, markets: markets, onlineTTL: onlineTTL,
	}
}

type SetAvailabilityInput struct {
	ProviderID string
	Status     string
	VehicleID  string
	Lat        float64
	Lng        float64
	Heading    float64
	Speed      float64
	Accuracy   float64
}

type AvailabilityState struct {
	Online           bool    `json:"online"`
	VehicleID        string  `json:"vehicle_id,omitempty"`
	Lat              float64 `json:"lat,omitempty"`
	Lng              float64 `json:"lng,omitempty"`
	UpdatedAt        int64   `json:"updated_at,omitempty"`
	HeartbeatSeconds int     `json:"heartbeat_seconds"`
}

// resolveZoneOrError wraps MarketLookup.ResolveZone, translating a NotFound
// into the same gateError shape every other online-gate failure uses, so the
// app's error handling does not need a special case for this one dimension.
func (s *AvailabilityService) resolveZoneOrError(ctx context.Context, lat, lng float64) (marketmodels.Zone, error) {
	zone, err := s.markets.ResolveZone(ctx, geo.Point{Lat: lat, Lng: lng})
	if err != nil {
		appErr := apperrors.From(err)
		if appErr != nil && appErr.Code == apperrors.CodeNotFound {
			return marketmodels.Zone{}, gateError("We do not operate in this area yet.", ReasonOutsideServiceArea)
		}
		return marketmodels.Zone{}, apperrors.Unavailable("Your location could not be resolved.", err)
	}
	return zone, nil
}

// SetAvailability puts a driver online or offline.
//
// Going online is gated on the driver being active, having finished onboarding,
// owning at least one active vehicle, and being inside a mapped operating
// zone. Each failure carries its own error code so the app can tell the
// driver what to fix.
func (s *AvailabilityService) SetAvailability(ctx context.Context, input SetAvailabilityInput) (AvailabilityState, error) {
	status := strings.ToLower(strings.TrimSpace(input.Status))
	switch status {
	case "offline":
		// Postgres is the durable authority (addendum architecture principle 1):
		// the operational-state transition is attempted first, and only once it
		// succeeds does the Redis fast-path get updated. current.Version is read
		// fresh immediately before the write so this is a guarded CAS against
		// whatever the state actually is right now, not a value cached from
		// earlier in the request.
		current, err := s.operationalState.Get(ctx, input.ProviderID)
		if err != nil {
			return AvailabilityState{}, apperrors.Unavailable("Availability could not be updated.", err)
		}
		if _, err := s.operationalState.Transition(ctx, nil, input.ProviderID,
			availabilitymodels.StateOffline, "went_offline", nil, current.Version); err != nil {
			return AvailabilityState{}, err
		}
		if err := s.store.SetOffline(ctx, input.ProviderID); err != nil {
			return AvailabilityState{}, apperrors.Unavailable("Availability could not be updated.", err)
		}
		return AvailabilityState{Online: false, HeartbeatSeconds: IdleHeartbeatSeconds}, nil
	case "online":
		// handled below
	default:
		return AvailabilityState{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "status", Message: "Status must be 'online' or 'offline'."},
		})
	}

	provider, err := s.providers.GetByID(ctx, input.ProviderID)
	if err != nil {
		return AvailabilityState{}, err
	}
	// Five of the six compliance dimensions in one call; the sixth (vehicle
	// readiness) is checked live below, via resolveVehicle -- see
	// CanGoAvailable's own comment for why it is not folded in here too.
	if ok, reason := provider.CanGoAvailable(); !ok {
		message := gateMessages[reason]
		if message == "" {
			message = "You cannot go online right now."
		}
		return AvailabilityState{}, gateError(message, reason)
	}

	// Resolve which vehicle the driver is working with. The matcher filters on
	// its class and seats, so a driver can never be online without one.
	vehicle, err := s.resolveVehicle(ctx, input.ProviderID, input.VehicleID)
	if err != nil {
		return AvailabilityState{}, err
	}

	// Resolve which operating zone (and therefore market + geo partition) this
	// position belongs to. A driver outside every mapped zone cannot be
	// indexed anywhere meaningful, so they cannot go online from there.
	zone, err := s.resolveZoneOrError(ctx, input.Lat, input.Lng)
	if err != nil {
		return AvailabilityState{}, err
	}

	current, err := s.operationalState.Get(ctx, input.ProviderID)
	if err != nil {
		return AvailabilityState{}, apperrors.Unavailable("Availability could not be updated.", err)
	}
	if _, err := s.operationalState.Transition(ctx, nil, input.ProviderID,
		availabilitymodels.StateAvailable, "went_online", nil, current.Version); err != nil {
		return AvailabilityState{}, err
	}

	if err := s.store.SetOnline(ctx, availabilityrepositories.ProviderStatus{
		ProviderID:   input.ProviderID,
		VehicleID:    vehicle.ID,
		MarketID:     zone.MarketID,
		VehicleClass: vehicle.VehicleClass,
		Lat:          input.Lat,
		Lng:          input.Lng,
		Heading:      input.Heading,
		Speed:        input.Speed,
		Accuracy:     input.Accuracy,
	}); err != nil {
		return AvailabilityState{}, apperrors.Unavailable("Availability could not be updated.", err)
	}

	return AvailabilityState{
		Online:           true,
		VehicleID:        vehicle.ID,
		Lat:              input.Lat,
		Lng:              input.Lng,
		UpdatedAt:        time.Now().Unix(),
		HeartbeatSeconds: IdleHeartbeatSeconds,
	}, nil
}

func (s *AvailabilityService) resolveVehicle(ctx context.Context, providerID, vehicleID string) (providerprofilemodels.Vehicle, error) {
	if strings.TrimSpace(vehicleID) != "" {
		// An explicitly chosen vehicle must belong to this driver and be active.
		vehicle, err := s.vehicles.GetByID(ctx, vehicleID, providerID)
		if err != nil {
			return providerprofilemodels.Vehicle{}, err
		}
		if vehicle.Status != providerprofilemodels.VehicleStatusActive {
			return providerprofilemodels.Vehicle{}, gateError("Select an active vehicle before going online.", ReasonNoActiveVehicle)
		}
		return vehicle, nil
	}

	count, err := s.vehicles.CountActiveByProvider(ctx, providerID)
	if err != nil {
		return providerprofilemodels.Vehicle{}, err
	}
	if count == 0 {
		return providerprofilemodels.Vehicle{}, gateError("Register an active vehicle before going online.", ReasonNoActiveVehicle)
	}
	return s.vehicles.FirstActiveByProvider(ctx, providerID)
}

// Heartbeat refreshes a driver's position and extends their online TTL.
//
// It requires an existing status key: a heartbeat cannot bring a driver online,
// otherwise a device that missed the gate could re-enter the matching pool.
//
// Deliberately Redis-only, no provider_operational_state write: a heartbeat
// does not change WHICH state the driver is in (still AVAILABLE, still
// ON_TRIP, whatever it already was), only that they are still alive in it.
// Addendum §32's own table assigns this exact split -- taxi:provider:status
// is "fresh location and operational state TTL" -- and bumping the Postgres
// version on every 30-second ping would make an in-flight guarded transition
// elsewhere (a matcher offering this driver a ride, built in phase B) race
// against heartbeats far more than the real state changes it should be
// guarding against.
//
// The geo-index partition is re-resolved from the new coordinate on every
// call, so a driver who drives from one zone into a neighboring one is
// re-indexed automatically. A resolution failure (a GPS blip just outside
// every zone's boundary) does not kick the driver offline mid-shift -- it
// keeps them indexed under whichever zone they last resolved successfully.
func (s *AvailabilityService) Heartbeat(ctx context.Context, input SetAvailabilityInput, onTrip bool) (AvailabilityState, error) {
	existing, ok, err := s.store.GetStatus(ctx, input.ProviderID)
	if err != nil {
		return AvailabilityState{}, apperrors.Unavailable("Availability could not be read.", err)
	}
	if !ok {
		return AvailabilityState{}, apperrors.BadRequest("You are not currently online.", nil)
	}

	vehicleID := existing.VehicleID
	if strings.TrimSpace(input.VehicleID) != "" {
		vehicleID = input.VehicleID
	}

	marketID, vehicleClass := existing.MarketID, existing.VehicleClass
	if zone, err := s.markets.ResolveZone(ctx, geo.Point{Lat: input.Lat, Lng: input.Lng}); err == nil {
		marketID = zone.MarketID
	}

	if err := s.store.Heartbeat(ctx, availabilityrepositories.ProviderStatus{
		ProviderID:   input.ProviderID,
		VehicleID:    vehicleID,
		MarketID:     marketID,
		VehicleClass: vehicleClass,
		Lat:          input.Lat,
		Lng:          input.Lng,
		Heading:      input.Heading,
		Speed:        input.Speed,
		Accuracy:     input.Accuracy,
	}); err != nil {
		return AvailabilityState{}, apperrors.Unavailable("Availability could not be updated.", err)
	}

	return AvailabilityState{
		Online:           true,
		VehicleID:        vehicleID,
		Lat:              input.Lat,
		Lng:              input.Lng,
		UpdatedAt:        time.Now().Unix(),
		HeartbeatSeconds: HeartbeatIntervalSeconds(onTrip),
	}, nil
}

func (s *AvailabilityService) GetAvailability(ctx context.Context, providerID string) (AvailabilityState, error) {
	status, ok, err := s.store.GetStatus(ctx, providerID)
	if err != nil {
		return AvailabilityState{}, apperrors.Unavailable("Availability could not be read.", err)
	}
	if !ok {
		return AvailabilityState{Online: false, HeartbeatSeconds: IdleHeartbeatSeconds}, nil
	}
	return AvailabilityState{
		Online:           true,
		VehicleID:        status.VehicleID,
		Lat:              status.Lat,
		Lng:              status.Lng,
		UpdatedAt:        status.UpdatedAt,
		HeartbeatSeconds: IdleHeartbeatSeconds,
	}, nil
}

// CustomerAvailability answers "can I book a ride from here right now".
type CustomerAvailability struct {
	Available bool  `json:"available"`
	Count     int64 `json:"count"`
}

// CheckAvailability counts drivers the passenger could actually be matched
// with. When a pickup point is supplied it resolves the zone it falls in and
// counts drivers of any bookable class within that zone's own radius;
// otherwise it falls back to everyone online anywhere.
//
// A pickup point outside every mapped zone is reported as simply unavailable,
// not as an error -- ResolveZone's NotFound is a legitimate "we do not serve
// this area" signal here, not a fault, and this is a lightweight
// polling-friendly endpoint that should not surface internal zone-resolution
// failures as HTTP errors.
func (s *AvailabilityService) CheckAvailability(ctx context.Context, center geo.Point, fallbackRadiusKm float64) (CustomerAvailability, error) {
	if center.HasFix() {
		zone, err := s.markets.ResolveZone(ctx, center)
		if err != nil {
			return CustomerAvailability{Available: false, Count: 0}, nil
		}

		var total int64
		for _, class := range vehicleclass.Names() {
			nearby, err := s.store.NearbyProviders(ctx, zone.MarketID, class, center, float64(zone.RadiusKm), 50)
			if err != nil {
				return CustomerAvailability{}, apperrors.Unavailable("Availability could not be read.", err)
			}
			total += int64(len(nearby))
		}
		if total > 0 {
			return CustomerAvailability{Available: true, Count: total}, nil
		}

		// An empty index is ambiguous: it also happens in the window after a
		// deploy, before drivers have heartbeated into the new index. Fall back
		// to scanning the authoritative online set — but keep applying the
		// zone's own radius, because this answer gates a booking from THIS
		// pickup point. Returning the global online count here would tell a
		// passenger in Abuja that a ride is available when the only driver is
		// in Lagos.
		online, err := s.store.GetOnlineProviders(ctx)
		if err != nil {
			return CustomerAvailability{}, apperrors.Unavailable("Availability could not be read.", err)
		}
		var withinRadius int64
		for _, p := range online {
			point := geo.Point{Lat: p.Lat, Lng: p.Lng}
			if !point.HasFix() {
				continue
			}
			if geo.HaversineKm(center, point) <= float64(zone.RadiusKm) {
				withinRadius++
			}
		}
		return CustomerAvailability{Available: withinRadius > 0, Count: withinRadius}, nil
	}

	// No pickup point supplied: the caller is asking whether the service has any
	// drivers at all, not whether one can reach a specific place.
	online, err := s.store.GetOnlineProviders(ctx)
	if err != nil {
		return CustomerAvailability{}, apperrors.Unavailable("Availability could not be read.", err)
	}
	return CustomerAvailability{Available: len(online) > 0, Count: int64(len(online))}, nil
}

// HeartbeatIntervalSeconds is the cadence the app should use next.
func HeartbeatIntervalSeconds(onTrip bool) int {
	if onTrip {
		return OnTripHeartbeatSeconds
	}
	return IdleHeartbeatSeconds
}
