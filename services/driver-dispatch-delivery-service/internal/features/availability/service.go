package availability

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	providerVerifiedStatus  = "verified"
	providerSuspendedStatus = "suspended"
)

type LiveStore interface {
	SetStatus(ctx context.Context, providerID string, status AvailabilityStatus) error
	GetStatus(ctx context.Context, providerID string) (AvailabilityStatus, bool, error)
	ClearProvider(ctx context.Context, providerID string) error
	RemoveFromGeo(ctx context.Context, providerID string) error
	RestoreGeoFromLocation(ctx context.Context, providerID string) (bool, error)
	SetLocation(ctx context.Context, providerID string, location Location, discoverable bool) error
	GetLocation(ctx context.Context, providerID string) (Location, bool, error)
	GetNearby(ctx context.Context, request NearbyProvidersRequest) ([]NearbyProvider, error)
}

type RedisLiveStore struct {
	client *redis.Client
}

func NewRedisLiveStore(client *redis.Client) *RedisLiveStore {
	return &RedisLiveStore{client: client}
}

func (s *RedisLiveStore) SetStatus(ctx context.Context, providerID string, status AvailabilityStatus) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Set(ctx, ProviderStatusKey(providerID), string(status), StatusTTL).Err()
}

func (s *RedisLiveStore) GetStatus(ctx context.Context, providerID string) (AvailabilityStatus, bool, error) {
	if s == nil || s.client == nil {
		return "", false, nil
	}
	value, err := s.client.Get(ctx, ProviderStatusKey(providerID)).Result()
	if err != nil {
		if err == redis.Nil {
			return "", false, nil
		}
		return "", false, err
	}
	status := AvailabilityStatus(value)
	if !IsValidStatus(status) {
		return "", false, nil
	}
	return status, true, nil
}

func (s *RedisLiveStore) ClearProvider(ctx context.Context, providerID string) error {
	if s == nil || s.client == nil {
		return nil
	}
	pipe := s.client.Pipeline()
	pipe.Set(ctx, ProviderStatusKey(providerID), string(StatusOffline), StatusTTL)
	pipe.Del(ctx, ProviderLocationKey(providerID))
	pipe.ZRem(ctx, OnlineProvidersGeoKey, providerID)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	// Notify WebSocket subscribers that this provider went offline.
	offlineMsg, _ := json.Marshal(WSProviderOffline{
		Type:       "provider_offline",
		ProviderID: providerID,
	})
	_ = s.client.Publish(ctx, ProviderLocationChannel(providerID), offlineMsg)
	return nil
}

func (s *RedisLiveStore) RemoveFromGeo(ctx context.Context, providerID string) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.ZRem(ctx, OnlineProvidersGeoKey, providerID).Err()
}

func (s *RedisLiveStore) RestoreGeoFromLocation(ctx context.Context, providerID string) (bool, error) {
	location, ok, err := s.GetLocation(ctx, providerID)
	if err != nil || !ok {
		return false, err
	}
	if s == nil || s.client == nil {
		return false, nil
	}
	if err := s.client.GeoAdd(ctx, OnlineProvidersGeoKey, &redis.GeoLocation{
		Name:      providerID,
		Longitude: location.Lng,
		Latitude:  location.Lat,
	}).Err(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *RedisLiveStore) SetLocation(ctx context.Context, providerID string, location Location, discoverable bool) error {
	if s == nil || s.client == nil {
		return nil
	}
	// Store canonical Location JSON in the Redis key (used by GetLocation).
	locPayload, err := json.Marshal(location)
	if err != nil {
		return fmt.Errorf("marshal provider location: %w", err)
	}
	if err := s.client.Set(ctx, ProviderLocationKey(providerID), locPayload, LocationTTL).Err(); err != nil {
		return err
	}
	if discoverable {
		// Redis GEOADD takes longitude first, then latitude.
		if err := s.client.GeoAdd(ctx, OnlineProvidersGeoKey, &redis.GeoLocation{
			Name:      providerID,
			Longitude: location.Lng,
			Latitude:  location.Lat,
		}).Err(); err != nil {
			return err
		}
	} else if err := s.client.ZRem(ctx, OnlineProvidersGeoKey, providerID).Err(); err != nil {
		return err
	}
	// Publish WebSocket-ready message so StreamLocation subscribers receive it.
	wsMsg := WSLocationUpdate{
		Type:       "location_update",
		ProviderID: providerID,
		Lat:        location.Lat,
		Lng:        location.Lng,
		Heading:    location.Heading,
		Speed:      location.Speed,
		Accuracy:   location.Accuracy,
		UpdatedAt:  location.UpdatedAt,
	}
	wsPayload, err := json.Marshal(wsMsg)
	if err != nil {
		return fmt.Errorf("marshal ws location update: %w", err)
	}
	cmd := s.client.Publish(ctx, ProviderLocationChannel(providerID), wsPayload)
	if err := cmd.Err(); err != nil {
		return err
	}
	log.Printf("availability location published provider_id=%s channel=%s subscribers=%d", providerID, ProviderLocationChannel(providerID), cmd.Val())
	return nil
}

func (s *RedisLiveStore) GetLocation(ctx context.Context, providerID string) (Location, bool, error) {
	if s == nil || s.client == nil {
		return Location{}, false, nil
	}
	payload, err := s.client.Get(ctx, ProviderLocationKey(providerID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return Location{}, false, nil
		}
		return Location{}, false, err
	}
	var location Location
	if err := json.Unmarshal(payload, &location); err != nil {
		return Location{}, false, err
	}
	return location, true, nil
}

func (s *RedisLiveStore) GetNearby(ctx context.Context, request NearbyProvidersRequest) ([]NearbyProvider, error) {
	if s == nil || s.client == nil {
		return []NearbyProvider{}, nil
	}
	normalizeNearbyRequest(&request)
	results, err := s.client.GeoSearchLocation(ctx, OnlineProvidersGeoKey, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  request.Longitude,
			Latitude:   request.Latitude,
			Radius:     request.RadiusKM,
			RadiusUnit: "km",
			Sort:       "ASC",
			Count:      request.Limit,
		},
		WithCoord: true,
		WithDist:  true,
	}).Result()
	if err != nil {
		return nil, err
	}

	nearby := make([]NearbyProvider, 0, len(results))
	for _, result := range results {
		providerID := result.Name
		status, ok, err := s.GetStatus(ctx, providerID)
		if err != nil {
			return nil, err
		}
		if !ok || status != StatusOnline {
			_ = s.client.ZRem(ctx, OnlineProvidersGeoKey, providerID).Err()
			continue
		}
		location, ok, err := s.GetLocation(ctx, providerID)
		if err != nil {
			return nil, err
		}
		if !ok {
			_ = s.client.ZRem(ctx, OnlineProvidersGeoKey, providerID).Err()
			continue
		}
		nearby = append(nearby, NearbyProvider{
			ProviderID: providerID,
			Lat:        location.Lat,
			Lng:        location.Lng,
			DistanceKM: result.Dist,
			UpdatedAt:  location.UpdatedAt,
		})
	}
	return nearby, nil
}

type Service struct {
	repository Repository
	live       LiveStore
	events     EventPublisher
	now        func() time.Time
}

type ServiceOption func(*Service)

func WithClock(now func() time.Time) ServiceOption {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithEventPublisher(events EventPublisher) ServiceOption {
	return func(s *Service) {
		s.events = events
	}
}

func NewService(repository Repository, live LiveStore, options ...ServiceOption) *Service {
	service := &Service{
		repository: repository,
		live:       live,
		now:        func() time.Time { return time.Now().UTC() },
	}
	if service.live == nil {
		service.live = noopLiveStore{}
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) SetStatus(ctx context.Context, providerID string, request SetAvailabilityRequest) (AvailabilityResponse, error) {
	if err := validateProviderID(providerID); err != nil {
		return AvailabilityResponse{}, err
	}
	status := AvailabilityStatus(strings.ToLower(strings.TrimSpace(string(request.Status))))
	if !IsProviderSettableStatus(status) {
		return AvailabilityResponse{}, validationErrors([]apperrors.FieldViolation{
			{Field: "status", Message: "Status must be online or offline."},
		})
	}

	switch status {
	case StatusOnline:
		if err := s.checkOnlineGate(ctx, providerID); err != nil {
			return AvailabilityResponse{}, err
		}
		return s.SetOnline(ctx, providerID)
	case StatusOffline:
		return s.SetOffline(ctx, providerID, false)
	default:
		return AvailabilityResponse{}, validationErrors([]apperrors.FieldViolation{
			{Field: "status", Message: "Status must be online or offline."},
		})
	}
}

func (s *Service) SetOnline(ctx context.Context, providerID string) (AvailabilityResponse, error) {
	if err := validateProviderID(providerID); err != nil {
		return AvailabilityResponse{}, err
	}
	availability, err := s.repository.EnsureAvailability(ctx, providerID)
	if err != nil {
		return AvailabilityResponse{}, err
	}
	openSession, hasOpenSession, err := s.repository.GetOpenSession(ctx, providerID)
	if err != nil {
		return AvailabilityResponse{}, err
	}

	liveStatus, liveExists, err := s.live.GetStatus(ctx, providerID)
	if err != nil {
		return AvailabilityResponse{}, err
	}
	alreadyLiveOnline := liveExists && liveStatus == StatusOnline
	dbOnlineWithOpenSession := !liveExists && availability.Status == StatusOnline && hasOpenSession
	if alreadyLiveOnline || dbOnlineWithOpenSession {
		if _, err := s.repository.SetOnline(ctx, providerID, s.now()); err != nil {
			return AvailabilityResponse{}, err
		}
		if err := s.live.SetStatus(ctx, providerID, StatusOnline); err != nil {
			return AvailabilityResponse{}, err
		}
		if _, err := s.live.RestoreGeoFromLocation(ctx, providerID); err != nil {
			return AvailabilityResponse{}, err
		}
		sessionStart := availability.SessionStart
		if sessionStart == nil && hasOpenSession {
			start := openSession.WentOnlineAt
			sessionStart = &start
		}
		return AvailabilityResponse{
			Status:       StatusOnline,
			SessionStart: sessionStart,
			Message:      "You are already online.",
		}, nil
	}

	now := s.now()
	if err := s.live.SetStatus(ctx, providerID, StatusOnline); err != nil {
		return AvailabilityResponse{}, err
	}
	if _, err := s.live.RestoreGeoFromLocation(ctx, providerID); err != nil {
		return AvailabilityResponse{}, err
	}
	updated, err := s.repository.SetOnline(ctx, providerID, now)
	if err != nil {
		return AvailabilityResponse{}, err
	}
	if _, _, err := s.repository.CreateSessionIfNoneOpen(ctx, providerID, now); err != nil {
		return AvailabilityResponse{}, err
	}
	if err := s.publishWentOnline(ctx, providerID, derefTime(updated.SessionStart, now), now); err != nil {
		return AvailabilityResponse{}, err
	}
	return AvailabilityResponse{
		Status:       StatusOnline,
		SessionStart: updated.SessionStart,
		Message:      "You are now online and visible to customers.",
	}, nil
}

func (s *Service) SetOffline(ctx context.Context, providerID string, forced bool) (AvailabilityResponse, error) {
	if err := validateProviderID(providerID); err != nil {
		return AvailabilityResponse{}, err
	}
	availability, err := s.repository.EnsureAvailability(ctx, providerID)
	if err != nil {
		return AvailabilityResponse{}, err
	}
	openSession, hasOpenSession, err := s.repository.GetOpenSession(ctx, providerID)
	if err != nil {
		return AvailabilityResponse{}, err
	}
	liveStatus, liveExists, err := s.live.GetStatus(ctx, providerID)
	if err != nil {
		return AvailabilityResponse{}, err
	}
	wasOnline := hasOpenSession ||
		availability.Status == StatusOnline ||
		availability.Status == StatusBusy ||
		(liveExists && (liveStatus == StatusOnline || liveStatus == StatusBusy))

	now := s.now()
	if err := s.live.ClearProvider(ctx, providerID); err != nil {
		return AvailabilityResponse{}, err
	}
	updated, err := s.repository.SetOffline(ctx, providerID, now)
	if err != nil {
		return AvailabilityResponse{}, err
	}
	ended, endedOpen, err := s.repository.EndOpenSession(ctx, providerID, now, forced)
	if err != nil {
		return AvailabilityResponse{}, err
	}
	if wasOnline {
		if err := s.publishWentOffline(ctx, providerID, now, forced); err != nil {
			return AvailabilityResponse{}, err
		}
	}
	if !wasOnline && !hasOpenSession {
		return AvailabilityResponse{
			Status:       StatusOffline,
			SessionStart: nil,
			Message:      "You are already offline.",
		}, nil
	}
	_ = updated
	_ = openSession
	_ = ended
	_ = endedOpen
	return AvailabilityResponse{
		Status:       StatusOffline,
		SessionStart: nil,
		Message:      "You are now offline.",
	}, nil
}

func (s *Service) GetStatus(ctx context.Context, providerID string) (AvailabilityStatusResponse, error) {
	if err := validateProviderID(providerID); err != nil {
		return AvailabilityStatusResponse{}, err
	}
	availability, err := s.repository.EnsureAvailability(ctx, providerID)
	if err != nil {
		return AvailabilityStatusResponse{}, err
	}

	now := s.now()
	liveStatus, liveExists, err := s.live.GetStatus(ctx, providerID)
	if err != nil {
		return AvailabilityStatusResponse{}, err
	}
	status := StatusOffline
	if liveExists && (liveStatus == StatusOnline || liveStatus == StatusBusy) {
		status = liveStatus
	} else if !liveExists {
		if err := s.live.RemoveFromGeo(ctx, providerID); err != nil {
			return AvailabilityStatusResponse{}, err
		}
	}

	sessionStart := availability.SessionStart
	if status == StatusOffline {
		sessionStart = nil
	} else if sessionStart == nil {
		openSession, ok, err := s.repository.GetOpenSession(ctx, providerID)
		if err != nil {
			return AvailabilityStatusResponse{}, err
		}
		if ok {
			start := openSession.WentOnlineAt
			sessionStart = &start
		}
	}

	stats, err := s.repository.GetTodayAvailabilityStats(ctx, providerID, todayStartUTC(now), now)
	if err != nil {
		return AvailabilityStatusResponse{}, err
	}
	return AvailabilityStatusResponse{
		Status:                 status,
		VerifiedToGoOnline:     availability.VerifiedToGoOnline,
		SessionStart:           sessionStart,
		SessionDurationMinutes: sessionDurationMinutes(now, status, sessionStart),
		HoursOnlineToday:       float64(stats.MinutesOnline) / 60.0,
		TripsToday:             stats.Trips,
	}, nil
}

func (s *Service) GetCurrentSession(ctx context.Context, providerID string) (CurrentSessionResponse, error) {
	if err := validateProviderID(providerID); err != nil {
		return CurrentSessionResponse{}, err
	}
	availability, err := s.repository.EnsureAvailability(ctx, providerID)
	if err != nil {
		return CurrentSessionResponse{}, err
	}
	session, ok, err := s.repository.GetOpenSession(ctx, providerID)
	if err != nil {
		return CurrentSessionResponse{}, err
	}
	var sessionPtr *AvailabilitySession
	if ok {
		sessionPtr = &session
	}
	return CurrentSessionResponse{
		ProviderID: providerID,
		Status:     availability.Status,
		Session:    sessionPtr,
	}, nil
}

func (s *Service) UpdateLocation(ctx context.Context, providerID string, request UpdateLocationRequest) (LocationUpdateResponse, error) {
	if err := validateProviderID(providerID); err != nil {
		return LocationUpdateResponse{}, err
	}
	if err := validateLocationInput(request); err != nil {
		return LocationUpdateResponse{}, err
	}

	// Phase 5F: check Redis live status first — DB is not authoritative for GPS pings.
	liveStatus, liveExists, err := s.live.GetStatus(ctx, providerID)
	if err != nil {
		return LocationUpdateResponse{}, err
	}
	if !liveExists || liveStatus == StatusOffline {
		return LocationUpdateResponse{}, apperrors.BadRequest("Provider must be online before updating location.", nil)
	}
	if liveStatus != StatusOnline && liveStatus != StatusBusy {
		return LocationUpdateResponse{}, apperrors.BadRequest("Provider must be online before updating location.", nil)
	}

	location := Location{
		ProviderID: providerID,
		Lat:        request.Lat,
		Lng:        request.Lng,
		Heading:    request.Heading,
		Speed:      request.Speed,
		Accuracy:   request.Accuracy,
		UpdatedAt:  s.now(),
	}
	discoverable := liveStatus == StatusOnline
	if err := s.live.SetLocation(ctx, providerID, location, discoverable); err != nil {
		return LocationUpdateResponse{}, err
	}
	// Refresh status TTL to 90 s on each accepted ping.
	if err := s.live.SetStatus(ctx, providerID, liveStatus); err != nil {
		return LocationUpdateResponse{}, err
	}

	// Publish provider.location_updated for analytics/admin map (non-blocking).
	if s.events != nil {
		now := s.now()
		_ = s.events.PublishProviderLocationUpdated(ctx, ProviderLocationUpdatedEvent{
			Event:      TopicProviderLocationUpdated,
			ProviderID: providerID,
			Lat:        request.Lat,
			Lng:        request.Lng,
			Heading:    request.Heading,
			Speed:      request.Speed,
			Accuracy:   request.Accuracy,
			UpdatedAt:  location.UpdatedAt,
			OccurredAt: now,
		})
	}

	return LocationUpdateResponse{Updated: true}, nil
}

func (s *Service) GetLocation(ctx context.Context, providerID string) (LocationResponse, error) {
	if err := validateProviderID(providerID); err != nil {
		return LocationResponse{}, err
	}
	location, ok, err := s.live.GetLocation(ctx, providerID)
	if err != nil {
		return LocationResponse{}, err
	}
	if !ok {
		return LocationResponse{}, apperrors.NotFound("Location not found.", nil)
	}
	return LocationResponse{Location: &location}, nil
}

func (s *Service) GetNearbyProviders(ctx context.Context, request NearbyProvidersRequest) (NearbyResponse, error) {
	if err := validateNearbyInput(request); err != nil {
		return NearbyResponse{}, err
	}
	normalizeNearbyRequest(&request)
	providers, err := s.live.GetNearby(ctx, request)
	if err != nil {
		return NearbyResponse{}, err
	}
	if providers == nil {
		providers = []NearbyProvider{}
	}
	return NearbyResponse{
		Providers: providers,
		Count:     len(providers),
		RadiusKM:  request.RadiusKM,
	}, nil
}

// SetBusy marks the provider as busy when a trip starts.
// Called by the trip.started subscriber (Phase 6 trip-service).
func (s *Service) SetBusy(ctx context.Context, providerID string) error {
	if err := validateProviderID(providerID); err != nil {
		return err
	}
	if err := s.live.SetStatus(ctx, providerID, StatusBusy); err != nil {
		return err
	}
	if err := s.live.RemoveFromGeo(ctx, providerID); err != nil {
		return err
	}
	if _, err := s.repository.SetBusy(ctx, providerID, s.now()); err != nil {
		return err
	}
	return nil
}

// ReturnFromTrip brings a busy provider back online after a trip completes or
// is cancelled. incrementTrips should be true for trip.completed, false for
// trip.cancelled (cancelled trips do not count toward trips_in_session).
func (s *Service) ReturnFromTrip(ctx context.Context, providerID string, incrementTrips bool) error {
	if err := validateProviderID(providerID); err != nil {
		return err
	}
	if err := s.live.SetStatus(ctx, providerID, StatusOnline); err != nil {
		return err
	}
	if _, err := s.live.RestoreGeoFromLocation(ctx, providerID); err != nil {
		return err
	}
	if _, err := s.repository.SetOnline(ctx, providerID, s.now()); err != nil {
		return err
	}
	if incrementTrips {
		if err := s.repository.IncrementOpenSessionTrips(ctx, providerID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) publishWentOnline(ctx context.Context, providerID string, sessionStart time.Time, occurredAt time.Time) error {
	if s.events == nil {
		return nil
	}
	return s.events.PublishProviderWentOnline(ctx, ProviderWentOnlineEvent{
		Event:        TopicProviderWentOnline,
		ProviderID:   providerID,
		Status:       StatusOnline,
		SessionStart: sessionStart,
		OccurredAt:   occurredAt,
	})
}

func (s *Service) publishWentOffline(ctx context.Context, providerID string, wentOfflineAt time.Time, forced bool) error {
	if s.events == nil {
		return nil
	}
	return s.events.PublishProviderWentOffline(ctx, ProviderWentOfflineEvent{
		Event:         TopicProviderWentOffline,
		ProviderID:    providerID,
		Status:        StatusOffline,
		WentOfflineAt: wentOfflineAt,
		ForcedOffline: forced,
		OccurredAt:    wentOfflineAt,
	})
}

func (s *Service) UnlockVerifiedToGoOnline(ctx context.Context, providerID string) error {
	if err := validateProviderID(providerID); err != nil {
		return err
	}
	_, err := s.repository.SetVerifiedToGoOnline(ctx, providerID, true)
	return err
}

func (s *Service) ForceOffline(ctx context.Context, providerID string, clearEligibility bool, forced bool) error {
	if err := validateProviderID(providerID); err != nil {
		return err
	}
	if _, err := s.repository.EnsureAvailability(ctx, providerID); err != nil {
		return err
	}
	if clearEligibility {
		if _, err := s.repository.SetVerifiedToGoOnline(ctx, providerID, false); err != nil {
			return err
		}
	}
	_, err := s.SetOffline(ctx, providerID, forced)
	return err
}

func (s *Service) ForceOfflineIfNoVerifiedActiveBike(ctx context.Context, providerID string) error {
	if err := validateProviderID(providerID); err != nil {
		return err
	}
	hasBike, err := s.repository.HasVerifiedActiveBike(ctx, providerID)
	if err != nil {
		return err
	}
	if hasBike {
		return nil
	}
	return s.ForceOffline(ctx, providerID, false, true)
}

func (s *Service) checkOnlineGate(ctx context.Context, providerID string) error {
	availability, err := s.repository.EnsureAvailability(ctx, providerID)
	if err != nil {
		return err
	}
	if !availability.VerifiedToGoOnline {
		return gateForbidden(GateError{
			Code:    GateNotVerified,
			Message: "Complete all verification steps before going online.",
		})
	}

	state, ok, err := s.repository.GetProviderGateState(ctx, providerID)
	if err != nil {
		return err
	}
	if !ok || !state.IsActive || state.VerificationStatus == providerSuspendedStatus {
		return gateForbidden(GateError{
			Code:    GateAccountSuspended,
			Message: "Your account has been suspended. Contact support.",
		})
	}
	hasBike, err := s.repository.HasVerifiedActiveBike(ctx, providerID)
	if err != nil {
		return err
	}
	if !hasBike {
		return gateForbidden(GateError{
			Code:    GateNoVerifiedVehicle,
			Message: "Register and get your bike approved before going online.",
		})
	}

	// Block if licence expired more than 30 days ago (grace period exceeded).
	licenceExpiry, err := s.repository.GetLicenceExpiryDate(ctx, providerID)
	if err != nil {
		return err
	}
	if licenceExpiry != nil {
		gracePeriodEnd := licenceExpiry.AddDate(0, 0, 30)
		if time.Now().After(gracePeriodEnd) {
			return gateForbidden(GateError{
				Code:    GateLicenceExpired,
				Message: "Your driver's licence has expired. Renew it to go online.",
			})
		}
	}

	return nil
}

func gateForbidden(reason GateError) *apperrors.Error {
	err := apperrors.Forbidden(reason.Message, nil)
	err.Details = map[string]interface{}{"gate": reason}
	return err
}

func todayStartUTC(now time.Time) time.Time {
	utc := now.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func sessionDurationMinutes(now time.Time, status AvailabilityStatus, sessionStart *time.Time) int {
	if status != StatusOnline && status != StatusBusy {
		return 0
	}
	if sessionStart == nil {
		return 0
	}
	minutes := int(now.Sub(*sessionStart).Minutes())
	if minutes < 0 {
		return 0
	}
	return minutes
}

func derefTime(value *time.Time, fallback time.Time) time.Time {
	if value == nil {
		return fallback
	}
	return *value
}

type RedisEventPublisher struct {
	client *redis.Client
}

func NewRedisEventPublisher(client *redis.Client) *RedisEventPublisher {
	return &RedisEventPublisher{client: client}
}

func (p *RedisEventPublisher) PublishProviderWentOnline(ctx context.Context, event ProviderWentOnlineEvent) error {
	return p.publish(ctx, TopicProviderWentOnline, event)
}

func (p *RedisEventPublisher) PublishProviderWentOffline(ctx context.Context, event ProviderWentOfflineEvent) error {
	return p.publish(ctx, TopicProviderWentOffline, event)
}

func (p *RedisEventPublisher) PublishProviderLocationUpdated(ctx context.Context, event ProviderLocationUpdatedEvent) error {
	return p.publish(ctx, TopicProviderLocationUpdated, event)
}

func (p *RedisEventPublisher) publish(ctx context.Context, topic string, event any) error {
	if p == nil || p.client == nil {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", topic, err)
	}
	cmd := p.client.Publish(ctx, topic, payload)
	if err := cmd.Err(); err != nil {
		log.Printf("availability publisher %s publish failed error=%v", topic, err)
		return err
	}
	log.Printf("availability publisher %s published subscribers=%d", topic, cmd.Val())
	return nil
}

func validateProviderID(providerID string) error {
	if _, err := uuid.Parse(strings.TrimSpace(providerID)); err != nil {
		return validationErrors([]apperrors.FieldViolation{
			{Field: "provider_id", Message: "Provider ID must be a valid UUID."},
		})
	}
	return nil
}

func validateLocationInput(input UpdateLocationRequest) error {
	var fields []apperrors.FieldViolation
	if input.Lat < -90 || input.Lat > 90 || math.IsNaN(input.Lat) || math.IsInf(input.Lat, 0) {
		fields = append(fields, apperrors.FieldViolation{Field: "lat", Message: "Latitude must be between -90 and 90."})
	}
	if input.Lng < -180 || input.Lng > 180 || math.IsNaN(input.Lng) || math.IsInf(input.Lng, 0) {
		fields = append(fields, apperrors.FieldViolation{Field: "lng", Message: "Longitude must be between -180 and 180."})
	}
	if input.Heading < 0 || input.Heading > 360 || math.IsNaN(input.Heading) || math.IsInf(input.Heading, 0) {
		fields = append(fields, apperrors.FieldViolation{Field: "heading", Message: "Heading must be between 0 and 360."})
	}
	if input.Speed < 0 || math.IsNaN(input.Speed) || math.IsInf(input.Speed, 0) {
		fields = append(fields, apperrors.FieldViolation{Field: "speed", Message: "Speed must be zero or greater."})
	}
	if input.Accuracy < 0 || math.IsNaN(input.Accuracy) || math.IsInf(input.Accuracy, 0) {
		fields = append(fields, apperrors.FieldViolation{Field: "accuracy", Message: "Accuracy must be zero or greater."})
	}
	if len(fields) > 0 {
		return validationErrors(fields)
	}
	return nil
}

func validateNearbyInput(input NearbyProvidersRequest) error {
	var fields []apperrors.FieldViolation
	if input.Latitude < -90 || input.Latitude > 90 || math.IsNaN(input.Latitude) || math.IsInf(input.Latitude, 0) {
		fields = append(fields, apperrors.FieldViolation{Field: "lat", Message: "Latitude must be between -90 and 90."})
	}
	if input.Longitude < -180 || input.Longitude > 180 || math.IsNaN(input.Longitude) || math.IsInf(input.Longitude, 0) {
		fields = append(fields, apperrors.FieldViolation{Field: "lng", Message: "Longitude must be between -180 and 180."})
	}
	if input.RadiusKM < 0 || math.IsNaN(input.RadiusKM) || math.IsInf(input.RadiusKM, 0) {
		fields = append(fields, apperrors.FieldViolation{Field: "radius", Message: "Radius must be zero or greater."})
	}
	if input.RadiusKM > NearbyMaxRadius {
		fields = append(fields, apperrors.FieldViolation{Field: "radius", Message: "Radius cannot exceed 50 km."})
	}
	if input.Limit < 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "limit", Message: "Limit must be zero or greater."})
	}
	if input.Limit > NearbyMaxLimit {
		fields = append(fields, apperrors.FieldViolation{Field: "limit", Message: "Limit cannot exceed 50."})
	}
	if len(fields) > 0 {
		return validationErrors(fields)
	}
	return nil
}

func normalizeNearbyRequest(input *NearbyProvidersRequest) {
	if input.RadiusKM == 0 {
		input.RadiusKM = 5
	}
	if input.RadiusKM > NearbyMaxRadius {
		input.RadiusKM = NearbyMaxRadius
	}
	if input.Limit == 0 {
		input.Limit = 20
	}
	if input.Limit > NearbyMaxLimit {
		input.Limit = NearbyMaxLimit
	}
}

func validationErrors(fields []apperrors.FieldViolation) *apperrors.Error {
	err := apperrors.New(http.StatusBadRequest, apperrors.CodeValidationFailed, "Check your details.", nil)
	err.Fields = fields
	return err
}

type noopLiveStore struct{}

func (noopLiveStore) SetStatus(context.Context, string, AvailabilityStatus) error {
	return nil
}

func (noopLiveStore) GetStatus(context.Context, string) (AvailabilityStatus, bool, error) {
	return "", false, nil
}

func (noopLiveStore) ClearProvider(context.Context, string) error {
	return nil
}

func (noopLiveStore) RemoveFromGeo(context.Context, string) error {
	return nil
}

func (noopLiveStore) RestoreGeoFromLocation(context.Context, string) (bool, error) {
	return false, nil
}

func (noopLiveStore) SetLocation(context.Context, string, Location, bool) error {
	return nil
}

func (noopLiveStore) GetLocation(context.Context, string) (Location, bool, error) {
	return Location{}, false, nil
}

func (noopLiveStore) GetNearby(context.Context, NearbyProvidersRequest) ([]NearbyProvider, error) {
	return []NearbyProvider{}, nil
}
