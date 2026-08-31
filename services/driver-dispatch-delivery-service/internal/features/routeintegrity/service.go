package routeintegrity

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/google/uuid"
)

type RouteAuthority interface {
	Plan(context.Context, []Point) (RoutePlan, error)
}
type Service struct {
	repo   *Repository
	routes RouteAuthority
	policy Policy
	now    func() time.Time
}

func New(repo *Repository, routes RouteAuthority, policy Policy) *Service {
	return &Service{repo: repo, routes: routes, policy: policy.normalized(), now: func() time.Time { return time.Now().UTC() }}
}
func (s *Service) EnsureRoute(ctx context.Context, tripID, providerID, reason string) (ExpectedRoute, error) {
	if s == nil || !s.policy.Enabled {
		return ExpectedRoute{}, apperrors.Unavailable("Route integrity is not enabled.", nil)
	}
	if s.routes == nil {
		return ExpectedRoute{}, apperrors.Unavailable("Route authority is unavailable.", nil)
	}
	trip, err := s.repo.TripContext(ctx, strings.TrimSpace(tripID), strings.TrimSpace(providerID))
	if err != nil {
		return ExpectedRoute{}, err
	}
	plan, err := s.routes.Plan(ctx, []Point{trip.Pickup, trip.Dropoff})
	if err != nil {
		return ExpectedRoute{}, apperrors.Unavailable("Expected route could not be established.", err)
	}
	return s.repo.CreateRoute(ctx, trip, plan, s.policy, strings.TrimSpace(reason), s.now())
}
func (s *Service) IngestLocation(ctx context.Context, fix LocationFix) error {
	if s == nil || !s.policy.Enabled {
		return nil
	}
	route, ok, err := s.repo.CurrentRouteForProvider(ctx, fix.ProviderID)
	if err != nil || !ok {
		return err
	}
	now := s.now()
	if fix.ReceivedAt.IsZero() {
		fix.ReceivedAt = now
	}
	if fix.ClientRecordedAt.IsZero() {
		fix.ClientRecordedAt = fix.ReceivedAt
	}
	alerts := s.evaluate(ctx, route, fix)
	return s.repo.RecordFixAndAlerts(ctx, route, fix, alerts)
}
func (s *Service) evaluate(ctx context.Context, route ExpectedRoute, fix LocationFix) []Alert {
	now := fix.ReceivedAt
	point := Point{Latitude: fix.Latitude, Longitude: fix.Longitude}
	alerts := []Alert{}
	add := func(kind, severity string, evidence map[string]any) {
		bucket := now.Unix() / 300
		alerts = append(alerts, Alert{ID: uuid.NewString(), DedupeKey: fmt.Sprintf("%s:%s:%d", route.ID, kind, bucket), TripID: route.TripID, RouteID: route.ID, RouteVersion: route.Version, Type: kind, State: "OPEN", Severity: severity, ObservedAt: now, PolicyVersion: PolicyVersion, Evidence: evidence})
	}
	distance := distanceToRouteMeters(point, route.OrderedPoints)
	if distance > float64(route.CorridorMeters) {
		add("ROUTE_DEVIATION", "MEDIUM", map[string]any{"distance_meters": math.Round(distance), "corridor_meters": route.CorridorMeters})
	}
	if fix.Accuracy > s.policy.MaximumAccuracyMeters {
		add("SUSPICIOUS_LOCATION", "MEDIUM", map[string]any{"accuracy_meters": fix.Accuracy, "maximum_accuracy_meters": s.policy.MaximumAccuracyMeters})
	}
	if fix.ClientRecordedAt.After(now.Add(s.policy.MaximumClientClockSkew)) || fix.ClientRecordedAt.Before(now.Add(-s.policy.MaximumClientClockSkew)) {
		add("POSSIBLE_ROUTE_TAMPERING", "HIGH", map[string]any{"client_clock_delta_seconds": int(math.Abs(now.Sub(fix.ClientRecordedAt).Seconds()))})
	}
	if len(route.OrderedPoints) > 0 && fix.Speed > 2 {
		target := route.OrderedPoints[len(route.OrderedPoints)-1]
		expected := bearingDegrees(point, target)
		if delta := angleDelta(fix.Heading, expected); delta > float64(route.WrongWayToleranceDegrees) {
			add("WRONG_WAY_MOVEMENT", "MEDIUM", map[string]any{"heading_delta_degrees": math.Round(delta)})
		}
	}
	if since, ok, err := s.repo.StationarySince(ctx, route.TripID); err == nil && ok && fix.Speed < 1 && now.Sub(since) >= time.Duration(route.StationarySeconds)*time.Second {
		add("EXCESSIVE_STATIONARY", "LOW", map[string]any{"stationary_seconds": int(now.Sub(since).Seconds())})
	}
	return alerts
}
func (s *Service) CustomerStatus(ctx context.Context, bookingID, customerID string) (CustomerRouteStatus, error) {
	if s == nil || !s.policy.Enabled {
		return CustomerRouteStatus{}, apperrors.Unavailable("Route integrity is not enabled.", nil)
	}
	return s.repo.CustomerStatus(ctx, bookingID, customerID, s.now())
}
func (s *Service) RunStaleWorker(ctx context.Context, interval time.Duration) {
	defer observability.WorkerRunning("route")()
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		started := time.Now()
		_, err := s.repo.CreateStaleAlerts(ctx, s.now(), 100)
		observability.Worker("route", started, err)
		if err != nil && ctx.Err() == nil {
			log.Printf("route integrity stale-alert worker failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) ReplayCommand(ctx context.Context, providerID string, command DeviceCommand) (DeviceCommandResult, error) {
	if s == nil || !s.policy.OfflineReplayEnabled {
		return DeviceCommandResult{}, apperrors.Unavailable("Offline command replay is not enabled.", nil)
	}
	providerID = strings.TrimSpace(providerID)
	command.DeviceID = strings.TrimSpace(command.DeviceID)
	command.DeviceCommandID = strings.TrimSpace(command.DeviceCommandID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CommandType = canonicalCommandType(command.CommandType)
	if providerID == "" || command.DeviceID == "" || command.DeviceCommandID == "" || command.IdempotencyKey == "" || command.ClientTimestamp.IsZero() {
		return DeviceCommandResult{}, apperrors.BadRequest("Offline command envelope is incomplete.", nil)
	}
	now := s.now()
	if command.ClientTimestamp.After(now.Add(s.policy.MaximumClientClockSkew)) || command.ClientTimestamp.Before(now.Add(-24*time.Hour)) {
		return DeviceCommandResult{}, apperrors.New(409, "CLIENT_TIMESTAMP_INVALID", "Offline command timestamp is outside the accepted window.", nil)
	}
	result := DeviceCommandResult{ID: uuid.NewString(), State: "REJECTED", CommandType: command.CommandType, ServerVersion: command.LastKnownServerVersion}
	if command.TripID != "" {
		trip, err := s.repo.TripContext(ctx, command.TripID, providerID)
		if err != nil {
			return result, err
		}
		result.ServerVersion = trip.SourceVersion
		if command.LastKnownServerVersion != trip.SourceVersion {
			result.ReasonCode = "STALE_SERVER_VERSION"
			return s.repo.RecordDeviceCommand(ctx, providerID, command, result, commandFingerprint(providerID, command), now)
		}
	}
	state, reason, ok := classifyOfflineCommand(command.CommandType)
	if !ok {
		return result, apperrors.BadRequest("Offline command type is not supported.", nil)
	}
	result.State, result.ReasonCode = state, reason
	stored, err := s.repo.RecordDeviceCommand(ctx, providerID, command, result, commandFingerprint(providerID, command), now)
	if err != nil {
		return stored, err
	}
	if command.CommandType == "LOCATION_FIX" && stored.State == "ACCEPTED" && !stored.Replayed {
		fix, parseErr := locationFromPayload(providerID, command.Payload, command.ClientTimestamp, now)
		if parseErr != nil {
			return stored, parseErr
		}
		if err = s.IngestLocation(ctx, fix); err != nil {
			return stored, err
		}
	}
	return stored, nil
}
func classifyOfflineCommand(kind string) (state, reason string, ok bool) {
	switch canonicalCommandType(kind) {
	case "LOCATION_FIX":
		return "ACCEPTED", "", true
	case "EVIDENCE_REFERENCE":
		return "PENDING_EVIDENCE_SYNC", "MEDIA_AUTHORIZATION_REQUIRED", true
	case "PARCEL_SCAN":
		return "REJECTED", "ONLINE_MANIFEST_VALIDATION_REQUIRED", true
	case "PICKUP_COMPLETE", "DELIVERY_COMPLETE", "PROVIDER_TRANSFER":
		return "REJECTED", "ONLINE_HANDOVER_VERIFICATION_REQUIRED", true
	default:
		return "", "", false
	}
}
func locationFromPayload(provider string, p map[string]any, client, received time.Time) (LocationFix, error) {
	num := func(key string) (float64, bool) {
		v, ok := p[key]
		if !ok {
			return 0, false
		}
		n, ok := v.(float64)
		return n, ok
	}
	lat, a := num("latitude")
	lng, b := num("longitude")
	heading, c := num("heading")
	speed, d := num("speed")
	accuracy, e := num("accuracy")
	if !a || !b || !c || !d || !e || lat < -90 || lat > 90 || lng < -180 || lng > 180 || heading < 0 || heading > 360 || speed < 0 || accuracy < 0 {
		return LocationFix{}, apperrors.BadRequest("Offline location payload is invalid.", nil)
	}
	return LocationFix{ProviderID: provider, Latitude: lat, Longitude: lng, Heading: heading, Speed: speed, Accuracy: accuracy, ClientRecordedAt: client, ReceivedAt: received}, nil
}
