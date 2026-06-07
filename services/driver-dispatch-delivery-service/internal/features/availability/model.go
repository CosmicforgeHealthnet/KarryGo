package availability

import (
	"context"
	"fmt"
	"time"
)

type AvailabilityStatus string

const (
	StatusOnline  AvailabilityStatus = "online"
	StatusOffline AvailabilityStatus = "offline"
	StatusBusy    AvailabilityStatus = "busy"
)

const (
	StatusTTL   = 90 * time.Second
	LocationTTL = 30 * time.Second

	OnlineProvidersGeoKey = "avail:geo:online"

	// LocationRateLimit* — GPS ping rate limit (Phase 5K).
	LocationRateLimitWindow    = 60 * time.Second
	LocationRateLimitMaxPerMin = 30

	// NearbyMaxRadius and NearbyMaxLimit cap the internal nearby endpoint (Phase 5I).
	NearbyMaxRadius = 50.0
	NearbyMaxLimit  = 50
)

func ProviderStatusKey(providerID string) string {
	return fmt.Sprintf("avail:status:%s", providerID)
}

func ProviderLocationKey(providerID string) string {
	return fmt.Sprintf("avail:location:%s", providerID)
}

func ProviderLocationChannel(providerID string) string {
	return fmt.Sprintf("avail:loc:chan:%s", providerID)
}

// ProviderLocationRateLimitKey is the Redis key used by the GPS ping rate limiter.
func ProviderLocationRateLimitKey(providerID string) string {
	return fmt.Sprintf("avail:ratelimit:location:%s", providerID)
}

type Availability struct {
	ID                 string             `json:"id"`
	ProviderID         string             `json:"provider_id"`
	Status             AvailabilityStatus `json:"status"`
	VerifiedToGoOnline bool               `json:"verified_to_go_online"`
	SessionStart       *time.Time         `json:"session_start,omitempty"`
	LastChangedAt      time.Time          `json:"last_changed_at"`
	CreatedAt          time.Time          `json:"created_at"`
}

type AvailabilitySession struct {
	ID              string     `json:"id"`
	ProviderID      string     `json:"provider_id"`
	WentOnlineAt    time.Time  `json:"went_online_at"`
	WentOfflineAt   *time.Time `json:"went_offline_at,omitempty"`
	DurationMinutes *int       `json:"duration_minutes,omitempty"`
	TripsInSession  int        `json:"trips_in_session"`
	ForcedOffline   bool       `json:"forced_offline"`
	CreatedAt       time.Time  `json:"created_at"`
}

type ProviderGateState struct {
	ProviderID             string
	IsActive               bool
	VerificationStatus     string
	VerifiedToGoOnline     bool
	HasVerifiedActiveBike  bool
	AvailabilityStatus     AvailabilityStatus
	AvailabilityRowCreated bool
}

type SetAvailabilityRequest struct {
	Status AvailabilityStatus `json:"status"`
}

type UpdateLocationRequest struct {
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Heading  float64 `json:"heading"`
	Speed    float64 `json:"speed"`
	Accuracy float64 `json:"accuracy"`
}

type NearbyProvidersRequest struct {
	Latitude  float64
	Longitude float64
	RadiusKM  float64
	Limit     int
}

type AvailabilityResponse struct {
	Status       AvailabilityStatus `json:"status"`
	SessionStart *time.Time         `json:"session_start"`
	Message      string             `json:"message"`
}

type AvailabilityStatusResponse struct {
	Status                 AvailabilityStatus `json:"status"`
	VerifiedToGoOnline     bool               `json:"verified_to_go_online"`
	SessionStart           *time.Time         `json:"session_start"`
	SessionDurationMinutes int                `json:"session_duration_minutes"`
	HoursOnlineToday       float64            `json:"hours_online_today"`
	TripsToday             int                `json:"trips_today"`
}

type TodayAvailabilityStats struct {
	MinutesOnline int
	Trips         int
}

type GateError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e GateError) Error() string {
	return e.Code + ": " + e.Message
}

const (
	GateNotVerified       = "not_verified"
	GateAccountSuspended  = "account_suspended"
	GateNoVerifiedVehicle = "no_verified_vehicle"
)

type Location struct {
	ProviderID string    `json:"provider_id"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
	Heading    float64   `json:"heading"`
	Speed      float64   `json:"speed"`
	Accuracy   float64   `json:"accuracy"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type NearbyProvider struct {
	ProviderID string    `json:"provider_id"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
	DistanceKM float64   `json:"distance_km"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type StatusResponse struct {
	ProviderID           string             `json:"provider_id"`
	Status               AvailabilityStatus `json:"status"`
	VerifiedToGoOnline   bool               `json:"verified_to_go_online"`
	CanGoOnline          bool               `json:"can_go_online"`
	BlockingReasons      []string           `json:"blocking_reasons"`
	SessionStart         *time.Time         `json:"session_start,omitempty"`
	LastChangedAt        time.Time          `json:"last_changed_at"`
	HasLiveLocation      bool               `json:"has_live_location"`
	LastLocation         *Location          `json:"last_location,omitempty"`
	RedisStatusKey       string             `json:"redis_status_key"`
	RedisLocationKey     string             `json:"redis_location_key"`
	RedisLocationChannel string             `json:"redis_location_channel"`
}

type CurrentSessionResponse struct {
	ProviderID string               `json:"provider_id"`
	Status     AvailabilityStatus   `json:"status"`
	Session    *AvailabilitySession `json:"session,omitempty"`
}

type LocationUpdateResponse struct {
	Updated bool `json:"updated"`
}

type LocationResponse struct {
	Location *Location `json:"location"`
}

type NearbyResponse struct {
	Providers []NearbyProvider `json:"providers"`
	Count     int              `json:"count"`
	RadiusKM  float64          `json:"radius_km"`
}

// WebSocket message types for the location stream.

// WSLocationUpdate is pushed to WebSocket clients on each GPS ping.
type WSLocationUpdate struct {
	Type       string    `json:"type"`
	ProviderID string    `json:"provider_id"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
	Heading    float64   `json:"heading"`
	Speed      float64   `json:"speed"`
	Accuracy   float64   `json:"accuracy"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// WSProviderOffline is pushed when the provider goes offline.
type WSProviderOffline struct {
	Type       string `json:"type"`
	ProviderID string `json:"provider_id"`
}

// WSLocationUnavailable is pushed immediately on connect when no cached
// location exists yet.
type WSLocationUnavailable struct {
	Type       string `json:"type"`
	ProviderID string `json:"provider_id"`
}

const (
	TopicProviderWentOnline      = "provider.went_online"
	TopicProviderWentOffline     = "provider.went_offline"
	TopicProviderLocationUpdated = "provider.location_updated"

	// Trip event topics — published by Phase 6 trip-service, consumed here.
	TopicTripStarted   = "trip.started"
	TopicTripCompleted = "trip.completed"
	TopicTripCancelled = "trip.cancelled"
)

type ProviderWentOnlineEvent struct {
	Event        string             `json:"event"`
	ProviderID   string             `json:"provider_id"`
	Status       AvailabilityStatus `json:"status"`
	SessionStart time.Time          `json:"session_start"`
	OccurredAt   time.Time          `json:"occurred_at"`
}

type ProviderWentOfflineEvent struct {
	Event         string             `json:"event"`
	ProviderID    string             `json:"provider_id"`
	Status        AvailabilityStatus `json:"status"`
	WentOfflineAt time.Time          `json:"went_offline_at"`
	ForcedOffline bool               `json:"forced_offline"`
	OccurredAt    time.Time          `json:"occurred_at"`
}

// ProviderLocationUpdatedEvent is published on every accepted GPS ping for
// analytics and admin map purposes.  It is separate from the Redis Pub/Sub
// WebSocket stream which uses WSLocationUpdate.
type ProviderLocationUpdatedEvent struct {
	Event      string    `json:"event"`
	ProviderID string    `json:"provider_id"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
	Heading    float64   `json:"heading"`
	Speed      float64   `json:"speed"`
	Accuracy   float64   `json:"accuracy"`
	UpdatedAt  time.Time `json:"updated_at"`
	OccurredAt time.Time `json:"occurred_at"`
}

// Trip event payloads — consumed from Phase 6 trip-service.

type TripStartedEvent struct {
	Event      string    `json:"event"`
	TripID     string    `json:"trip_id"`
	ProviderID string    `json:"provider_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

type TripCompletedEvent struct {
	Event      string    `json:"event"`
	TripID     string    `json:"trip_id"`
	ProviderID string    `json:"provider_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

type TripCancelledEvent struct {
	Event      string    `json:"event"`
	TripID     string    `json:"trip_id"`
	ProviderID string    `json:"provider_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

type EventPublisher interface {
	PublishProviderWentOnline(ctx context.Context, event ProviderWentOnlineEvent) error
	PublishProviderWentOffline(ctx context.Context, event ProviderWentOfflineEvent) error
	// PublishProviderLocationUpdated is a non-blocking publish; GPS endpoint
	// must not fail if this publish fails.
	PublishProviderLocationUpdated(ctx context.Context, event ProviderLocationUpdatedEvent) error
}

func IsValidStatus(status AvailabilityStatus) bool {
	switch status {
	case StatusOnline, StatusOffline, StatusBusy:
		return true
	default:
		return false
	}
}

func IsProviderSettableStatus(status AvailabilityStatus) bool {
	return status == StatusOnline || status == StatusOffline
}
