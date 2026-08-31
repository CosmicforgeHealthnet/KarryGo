package routeintegrity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/commerceanalytics"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) TripContext(ctx context.Context, tripID, providerID string) (TripRouteContext, error) {
	query := `SELECT t.id::text,t.booking_id::text,t.provider_id::text,t.customer_id::text,t.status,t.source_version,
		t.pickup_lat::float8,t.pickup_lng::float8,t.dropoff_lat::float8,t.dropoff_lng::float8,COALESCE(t.service_tier,'standard')
		FROM trips t WHERE t.id=$1::uuid`
	args := []any{tripID}
	if providerID != "" {
		query += ` AND t.provider_id=$2::uuid`
		args = append(args, providerID)
	}
	var item TripRouteContext
	err := r.db.QueryRow(ctx, query, args...).Scan(&item.TripID, &item.BookingID, &item.ProviderID, &item.CustomerID, &item.Status, &item.SourceVersion,
		&item.Pickup.Latitude, &item.Pickup.Longitude, &item.Dropoff.Latitude, &item.Dropoff.Longitude, &item.ServiceTier)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, apperrors.NotFound("Trip not found.", nil)
	}
	item.Pickup.Ref = "pickup"
	item.Dropoff.Ref = "dropoff"
	return item, err
}

func (r *Repository) CreateRoute(ctx context.Context, trip TripRouteContext, plan RoutePlan, policy Policy, reason string, now time.Time) (ExpectedRoute, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ExpectedRoute{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var status string
	var sourceVersion int64
	if err = tx.QueryRow(ctx, `SELECT status,source_version FROM trips WHERE id=$1::uuid AND provider_id=$2::uuid FOR UPDATE`, trip.TripID, trip.ProviderID).Scan(&status, &sourceVersion); err != nil {
		return ExpectedRoute{}, err
	}
	if status == "completed" || status == "cancelled" || status == "failed" {
		return ExpectedRoute{}, apperrors.Conflict("A terminal trip cannot receive a route.", nil)
	}
	var version int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM dispatch_expected_routes WHERE trip_id=$1::uuid`, trip.TripID).Scan(&version); err != nil {
		return ExpectedRoute{}, err
	}
	var oldID string
	_ = tx.QueryRow(ctx, `SELECT id::text FROM dispatch_expected_routes WHERE trip_id=$1::uuid AND status='CURRENT' FOR UPDATE`, trip.TripID).Scan(&oldID)
	item := ExpectedRoute{ID: uuid.NewString(), TripID: trip.TripID, ProviderID: trip.ProviderID, Version: version, Status: "CURRENT", RouteReference: plan.Reference,
		RouteProvider: plan.Provider, RouteEvidenceVersion: plan.EvidenceVersion, OrderedPoints: plan.OrderedPoints, DistanceMeters: plan.DistanceMeters,
		DurationSeconds: plan.DurationSeconds, CorridorMeters: policy.CorridorMeters, WrongWayToleranceDegrees: policy.WrongWayToleranceDegrees,
		StationarySeconds: int(policy.StationaryThreshold.Seconds()), StaleSeconds: int(policy.StaleThreshold.Seconds()), HighRisk: trip.ServiceTier == "express",
		PolicyVersion: PolicyVersion, RerouteReason: reason, CreatedAt: now}
	if item.HighRisk {
		item.CorridorMeters = policy.HighRiskCorridorMeters
		item.StaleSeconds = int(policy.HighRiskStaleThreshold.Seconds())
	}
	raw, _ := json.Marshal(item.OrderedPoints)
	if oldID != "" {
		if _, err = tx.Exec(ctx, `UPDATE dispatch_expected_routes SET status='SUPERSEDED' WHERE id=$1::uuid`, oldID); err != nil {
			return ExpectedRoute{}, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_expected_routes(id,trip_id,provider_id,version,status,route_reference,route_provider,route_evidence_version,
		ordered_points,distance_meters,duration_seconds,corridor_meters,wrong_way_tolerance_degrees,stationary_seconds,stale_seconds,high_risk,policy_version,reroute_reason,created_at)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4,'CURRENT',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, item.ID, item.TripID, item.ProviderID, item.Version,
		item.RouteReference, item.RouteProvider, item.RouteEvidenceVersion, raw, item.DistanceMeters, item.DurationSeconds, item.CorridorMeters, item.WrongWayToleranceDegrees,
		item.StationarySeconds, item.StaleSeconds, item.HighRisk, item.PolicyVersion, item.RerouteReason, now)
	if err != nil {
		return ExpectedRoute{}, err
	}
	if oldID != "" {
		if _, err = tx.Exec(ctx, `UPDATE dispatch_expected_routes SET status='SUPERSEDED',superseded_by=$2::uuid WHERE id=$1::uuid`, oldID, item.ID); err != nil {
			return ExpectedRoute{}, err
		}
	}
	if err = emitRouteEvent(ctx, tx, "dispatch.route.versioned.v1", item.TripID, item.Version, item); err != nil {
		return ExpectedRoute{}, err
	}
	eventType := "dispatch.route.created.v1"
	if oldID != "" {
		eventType = "dispatch.route.rerouted.v1"
	}
	if err = emitRouteEvent(ctx, tx, eventType, item.TripID, item.Version, item); err != nil {
		return ExpectedRoute{}, err
	}
	return item, tx.Commit(ctx)
}

func (r *Repository) CurrentRouteForProvider(ctx context.Context, providerID string) (ExpectedRoute, bool, error) {
	var item ExpectedRoute
	var raw []byte
	err := r.db.QueryRow(ctx, `SELECT er.id::text,er.trip_id::text,er.provider_id::text,er.version,er.status,er.route_reference,er.route_provider,
		er.route_evidence_version,er.ordered_points,er.distance_meters,er.duration_seconds,er.corridor_meters,er.wrong_way_tolerance_degrees,
		er.stationary_seconds,er.stale_seconds,er.high_risk,er.policy_version,er.reroute_reason,er.created_at
		FROM dispatch_expected_routes er JOIN trips t ON t.id=er.trip_id WHERE er.provider_id=$1::uuid AND er.status='CURRENT'
		AND t.status NOT IN ('completed','cancelled','failed') ORDER BY er.created_at DESC LIMIT 1`, providerID).Scan(&item.ID, &item.TripID, &item.ProviderID, &item.Version, &item.Status,
		&item.RouteReference, &item.RouteProvider, &item.RouteEvidenceVersion, &raw, &item.DistanceMeters, &item.DurationSeconds, &item.CorridorMeters, &item.WrongWayToleranceDegrees,
		&item.StationarySeconds, &item.StaleSeconds, &item.HighRisk, &item.PolicyVersion, &item.RerouteReason, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, false, nil
	}
	if err != nil {
		return item, false, err
	}
	err = json.Unmarshal(raw, &item.OrderedPoints)
	return item, err == nil, err
}

func (r *Repository) LastFix(ctx context.Context, tripID string) (LocationFix, bool, error) {
	var item LocationFix
	err := r.db.QueryRow(ctx, `SELECT provider_id::text,latitude,longitude,heading,speed,accuracy,client_recorded_at,received_at
		FROM dispatch_route_location_fixes WHERE trip_id=$1::uuid ORDER BY received_at DESC,id DESC LIMIT 1`, tripID).Scan(&item.ProviderID, &item.Latitude, &item.Longitude, &item.Heading, &item.Speed, &item.Accuracy, &item.ClientRecordedAt, &item.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, false, nil
	}
	return item, err == nil, err
}

func (r *Repository) StationarySince(ctx context.Context, tripID string) (time.Time, bool, error) {
	var since *time.Time
	err := r.db.QueryRow(ctx, `SELECT CASE WHEN count(*)=0 THEN NULL ELSE COALESCE(max(received_at) FILTER(WHERE speed>=1),min(received_at)) END
		FROM dispatch_route_location_fixes WHERE trip_id=$1::uuid`, tripID).Scan(&since)
	if err != nil || since == nil {
		return time.Time{}, false, err
	}
	return *since, true, nil
}

func (r *Repository) RecordFixAndAlerts(ctx context.Context, route ExpectedRoute, fix LocationFix, alerts []Alert) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var current bool
	if err = tx.QueryRow(ctx, `SELECT status='CURRENT' FROM dispatch_expected_routes WHERE id=$1::uuid FOR SHARE`, route.ID).Scan(&current); err != nil {
		return err
	}
	if !current {
		return nil
	}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_route_location_fixes(trip_id,route_id,provider_id,latitude,longitude,heading,speed,accuracy,client_recorded_at,received_at)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10)`, route.TripID, route.ID, fix.ProviderID, fix.Latitude, fix.Longitude, fix.Heading, fix.Speed, fix.Accuracy, fix.ClientRecordedAt, fix.ReceivedAt)
	if err != nil {
		return err
	}
	for _, alert := range alerts {
		raw, _ := json.Marshal(alert.Evidence)
		tag, insertErr := tx.Exec(ctx, `INSERT INTO dispatch_route_alerts(id,trip_id,route_id,route_version,alert_type,state,severity,policy_version,observed_at,safe_evidence,dedupe_key)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,'OPEN',$6,$7,$8,$9,$10) ON CONFLICT(dedupe_key) DO NOTHING`, alert.ID, route.TripID, route.ID, route.Version, alert.Type, alert.Severity, PolicyVersion, alert.ObservedAt, raw, alert.DedupeKey)
		if insertErr != nil {
			return insertErr
		}
		if tag.RowsAffected() > 0 {
			if err = emitRouteEvent(ctx, tx, "dispatch.route_alert.opened.v1", route.TripID, route.Version, alert); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) CreateStaleAlerts(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	tag, err := r.db.Exec(ctx, `INSERT INTO dispatch_route_alerts(id,trip_id,route_id,route_version,alert_type,state,severity,policy_version,observed_at,safe_evidence,dedupe_key)
		SELECT gen_random_uuid(),r.trip_id,r.id,r.version,'STALE_LOCATION','OPEN',CASE WHEN r.high_risk THEN 'HIGH' ELSE 'MEDIUM' END,r.policy_version,$1,
		jsonb_build_object('last_received_at',max(f.received_at),'threshold_seconds',r.stale_seconds),
		'stale:'||r.id::text||':'||(extract(epoch FROM $1)::bigint/GREATEST(r.stale_seconds,1))::text
		FROM dispatch_expected_routes r JOIN trips t ON t.id=r.trip_id LEFT JOIN dispatch_route_location_fixes f ON f.route_id=r.id
		WHERE r.status='CURRENT' AND t.status NOT IN ('completed','cancelled','failed') GROUP BY r.id,t.id
		HAVING COALESCE(max(f.received_at),r.created_at) < $1-make_interval(secs=>r.stale_seconds)
		ON CONFLICT(dedupe_key) DO NOTHING`, now)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (r *Repository) CustomerStatus(ctx context.Context, bookingID, customerID string, now time.Time) (CustomerRouteStatus, error) {
	var result CustomerRouteStatus
	var staleSeconds int
	var last *time.Time
	err := r.db.QueryRow(ctx, `SELECT t.id::text,r.version,r.stale_seconds,max(f.received_at),count(DISTINCT a.id) FILTER(WHERE a.state='OPEN')
		FROM trips t JOIN dispatch_expected_routes r ON r.trip_id=t.id AND r.status='CURRENT'
		LEFT JOIN dispatch_route_location_fixes f ON f.route_id=r.id LEFT JOIN dispatch_route_alerts a ON a.route_id=r.id
		WHERE t.booking_id=$1::uuid AND t.customer_id=$2::uuid GROUP BY t.id,r.version,r.stale_seconds`, bookingID, customerID).
		Scan(&result.TripID, &result.ActiveRouteVersion, &staleSeconds, &last, &result.OpenRouteAlerts)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, apperrors.NotFound("Active route was not found.", nil)
	}
	if err != nil {
		return result, err
	}
	result.LastUpdatedAt = last
	result.IsStale = last == nil || now.Sub(*last) > time.Duration(staleSeconds)*time.Second
	if result.IsStale {
		result.RouteStatus = "STALE"
	} else {
		result.RouteStatus = "ACTIVE"
	}
	if last != nil {
		if fix, ok, e := r.LastFix(ctx, result.TripID); e == nil && ok {
			result.LastLocation = &CustomerLocation{Latitude: fix.Latitude, Longitude: fix.Longitude, RecordedAt: fix.ReceivedAt}
		}
	}
	return result, nil
}

func (r *Repository) RecordDeviceCommand(ctx context.Context, providerID string, command DeviceCommand, result DeviceCommandResult, fp string, now time.Time) (DeviceCommandResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var existing DeviceCommandResult
	var stored string
	var snapshot []byte
	err = tx.QueryRow(ctx, `SELECT id::text,state,command_type,result_snapshot,request_fingerprint FROM dispatch_device_commands
		WHERE provider_id=$1::uuid AND (idempotency_key=$2 OR (device_id=$3 AND device_command_id=$4)) FOR UPDATE`, providerID, command.IdempotencyKey, command.DeviceID, command.DeviceCommandID).
		Scan(&existing.ID, &existing.State, &existing.CommandType, &snapshot, &stored)
	if err == nil {
		if stored != fp {
			return result, apperrors.Conflict("Offline command identity was reused with different semantics.", nil)
		}
		_ = json.Unmarshal(snapshot, &existing)
		existing.Replayed = true
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	payload, _ := json.Marshal(command.Payload)
	snapshot, _ = json.Marshal(result)
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_device_commands(id,provider_id,trip_id,device_id,device_command_id,command_type,command_payload,idempotency_key,request_fingerprint,
		client_timestamp,last_known_server_version,state,result_snapshot,created_at,updated_at)
		VALUES($1::uuid,$2::uuid,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14)`, result.ID, providerID, command.TripID, command.DeviceID,
		command.DeviceCommandID, command.CommandType, payload, command.IdempotencyKey, fp, command.ClientTimestamp, command.LastKnownServerVersion, result.State, snapshot, now)
	if err != nil {
		_ = tx.Rollback(ctx)
		existing, stored, found, readErr := r.readDeviceCommand(ctx, providerID, command)
		if readErr == nil && found {
			if stored != fp {
				return result, apperrors.Conflict("Offline command identity was reused with different semantics.", nil)
			}
			existing.Replayed = true
			return existing, nil
		}
		return result, apperrors.Conflict("Offline command conflicts with an existing operation.", err)
	}
	return result, tx.Commit(ctx)
}

func (r *Repository) readDeviceCommand(ctx context.Context, providerID string, command DeviceCommand) (DeviceCommandResult, string, bool, error) {
	var result DeviceCommandResult
	var snapshot []byte
	var fp string
	err := r.db.QueryRow(ctx, `SELECT result_snapshot,request_fingerprint FROM dispatch_device_commands WHERE provider_id=$1::uuid AND (idempotency_key=$2 OR (device_id=$3 AND device_command_id=$4))`, providerID, command.IdempotencyKey, command.DeviceID, command.DeviceCommandID).Scan(&snapshot, &fp)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, "", false, nil
	}
	if err != nil {
		return result, "", false, err
	}
	if err = json.Unmarshal(snapshot, &result); err != nil {
		return result, "", false, err
	}
	return result, fp, true, nil
}

func emitRouteEvent(ctx context.Context, tx pgx.Tx, eventType, tripID string, version int64, payload any) error {
	// The alert-row trigger includes both immediate and stale-worker alerts and
	// uses alert identity rather than colliding at trip/route-version granularity.
	if eventType == "dispatch.route_alert.opened.v1" {
		return nil
	}
	if route, ok := payload.(ExpectedRoute); ok {
		payload = commerceanalytics.DispatchRouteMetric{RouteVersion: route.Version, State: route.Status, PolicyVersion: route.PolicyVersion, DistanceBucketKM: (route.DistanceMeters + 4999) / 5000 * 5, DurationBucketMinutes: (route.DurationSeconds + 299) / 300 * 5}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service)
	VALUES($1,'trip',$2,$3,$4,encode(digest($4::text,'sha256'),'hex'),'analytics-service') ON CONFLICT DO NOTHING`, eventType, tripID, version, raw)
	return err
}

func (r *Repository) String() string { return fmt.Sprintf("routeintegrity.Repository(%p)", r) }
