package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

func (s *Service) Board(ctx context.Context, t tenant.Context, f PageFilter) (Board, error) {
	if e := s.gate(s.o.Flags.ControlTower, false, "control_tower_disabled"); e != nil {
		return Board{}, e
	}
	if f.BranchID != "" && !t.BranchPermitted(f.BranchID) {
		return Board{}, nf()
	}
	routes, e := s.o.Repository.ListRoutes(ctx, t.OrganizationID, f)
	if e != nil {
		return Board{}, e
	}
	tracking, e := s.o.Repository.ListTracking(ctx, t.OrganizationID, f)
	if e != nil {
		return Board{}, e
	}
	alerts, e := s.ListAlerts(ctx, t, f)
	if e != nil {
		return Board{}, e
	}
	unassigned, e := s.unassigned(ctx, t, f)
	if e != nil {
		return Board{}, e
	}
	pending, e := s.assignmentSummary(ctx, t, f, "PENDING_PROVIDER_RESPONSE")
	if e != nil {
		return Board{}, e
	}
	accepted, e := s.assignmentSummary(ctx, t, f, "ACCEPTED,READY_FOR_EXECUTION")
	if e != nil {
		return Board{}, e
	}
	summary := map[string]int{Fresh: 0, Aging: 0, Stale: 0, Unknown: 0, Gapped: 0}
	gaps := false
	for _, p := range tracking {
		state := Freshness(p.SourceTimestamp, s.o.Now(), s.o.TrackingFreshAfter, s.o.TrackingStaleAfter, p.GapStatus != "CURRENT")
		p.FreshnessStatus = state
		summary[state]++
		if state == Gapped {
			gaps = true
		}
	}
	active := []RoutePlan{}
	for _, r := range routes {
		if r.Status == RoutePublished || r.Status == RouteInProgress {
			active = append(active, r)
		}
	}
	providers, vehicles, projectionGaps, e := s.boardAuthoritySummaries(ctx, t, f)
	if e != nil {
		return Board{}, e
	}
	capacity := map[string]int{
		"unassignedWork":      len(unassigned),
		"pendingAssignments":  len(pending),
		"acceptedAssignments": len(accepted),
		"activeRoutes":        len(active),
		"activeTrips":         len(tracking),
	}
	return Board{Unassigned: unassigned, PendingAssignments: pending, AcceptedAssignments: accepted, ActiveRoutes: active, ActiveTrips: tracking, Alerts: alerts, TrackingSummary: summary, CapacitySummary: capacity, ProviderAvailability: providers, VehicleAvailability: vehicles, ProjectionGaps: projectionGaps, ProjectionIncomplete: summary[Stale]+summary[Unknown]+summary[Gapped] > 0, ReconciliationRequired: gaps}, nil
}

// boardAuthoritySummaries uses persisted candidate and tracking projections only;
// it never fans out to provider or vehicle authorities while rendering the board.
func (s *Service) boardAuthoritySummaries(ctx context.Context, t tenant.Context, f PageFilter) ([]map[string]any, []map[string]any, []map[string]any, error) {
	rows, e := s.o.Pool.Query(ctx, `SELECT provider_id,vehicle_or_truck_reference,eligibility_status,generated_at,expires_at,branch_id
FROM fleet_partner_assignment_candidates WHERE organization_id=$1 AND ($2='' OR branch_id=$2) AND expires_at>now()
ORDER BY generated_at DESC LIMIT 200`, t.OrganizationID, f.BranchID)
	if e != nil {
		return nil, nil, nil, e
	}
	providers, vehicles := []map[string]any{}, []map[string]any{}
	seenProviders, seenVehicles := map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var providerID, vehicleReference, eligibility, branchID string
		var generatedAt, expiresAt time.Time
		if e = rows.Scan(&providerID, &vehicleReference, &eligibility, &generatedAt, &expiresAt, &branchID); e != nil {
			rows.Close()
			return nil, nil, nil, e
		}
		if !t.BranchPermitted(branchID) {
			continue
		}
		if providerID != "" && !seenProviders[providerID] {
			providers = append(providers, map[string]any{"providerId": providerID, "availability": eligibility, "source": "candidate_projection", "generatedAt": generatedAt, "expiresAt": expiresAt})
			seenProviders[providerID] = true
		}
		if vehicleReference != "" && !seenVehicles[vehicleReference] {
			vehicles = append(vehicles, map[string]any{"vehicleOrTruckReference": vehicleReference, "availability": eligibility, "source": "candidate_projection", "generatedAt": generatedAt, "expiresAt": expiresAt})
			seenVehicles[vehicleReference] = true
		}
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return nil, nil, nil, e
	}
	rows.Close()

	gapRows, e := s.o.Pool.Query(ctx, `SELECT source_service,canonical_trip_reference,source_version,gap_status,received_at
FROM fleet_partner_tracking_projections WHERE organization_id=$1 AND gap_status<>'CURRENT' ORDER BY received_at DESC LIMIT 200`, t.OrganizationID)
	if e != nil {
		return nil, nil, nil, e
	}
	defer gapRows.Close()
	projectionGaps := []map[string]any{}
	for gapRows.Next() {
		var source, trip, status string
		var version int64
		var receivedAt time.Time
		if e = gapRows.Scan(&source, &trip, &version, &status, &receivedAt); e != nil {
			return nil, nil, nil, e
		}
		projectionGaps = append(projectionGaps, map[string]any{"sourceService": source, "canonicalTripReference": trip, "sourceVersion": version, "gapStatus": status, "receivedAt": receivedAt})
	}
	return providers, vehicles, projectionGaps, gapRows.Err()
}
func (s *Service) ControlTower(ctx context.Context, t tenant.Context, f PageFilter) (Board, error) {
	return s.Board(ctx, t, f)
}
func (s *Service) Unassigned(ctx context.Context, t tenant.Context, f PageFilter) ([]map[string]any, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, false, "dispatch_planning_disabled"); e != nil {
		return nil, e
	}
	return s.unassigned(ctx, t, f)
}
func (s *Service) unassigned(ctx context.Context, t tenant.Context, f PageFilter) ([]map[string]any, error) {
	args := []any{t.OrganizationID}
	branchSQL := ""
	if f.BranchID != "" {
		args = append(args, f.BranchID)
		branchSQL = " AND w.branch_id=$2"
	}
	rows, e := s.o.Pool.Query(ctx, `WITH work AS(
SELECT o.id,'ORDER' source_type,o.branch_id,o.status,CASE WHEN o.status='CONFIRMED' THEN 'CONFIRMED_WITHOUT_ROUTE' ELSE 'ORDER_REQUIRES_ACTION' END reason FROM fleet_partner_private_orders o WHERE o.organization_id=$1 AND o.status IN('CONFIRMED','PLANNED')
UNION ALL SELECT sh.id,'SHIPMENT',sh.branch_id,sh.status,'READY_WITHOUT_ACTIVE_ASSIGNMENT' FROM fleet_partner_shipments sh WHERE sh.organization_id=$1 AND sh.status='READY'
UNION ALL SELECT rp.id,'ROUTE',rp.branch_id,rp.status,'PROVIDER_REPLACEMENT_REQUIRED' FROM fleet_partner_route_plans rp WHERE rp.organization_id=$1 AND rp.status IN('FAILED','PUBLISHED') AND EXISTS(SELECT 1 FROM fleet_partner_private_assignments pa WHERE pa.organization_id=rp.organization_id AND pa.route_stop_id IN(SELECT rs.id FROM fleet_partner_route_stops rs WHERE rs.route_plan_id=rp.id) AND pa.status IN('DECLINED','EXPIRED','CANCELLED','REASSIGNMENT_REQUIRED')))
SELECT w.id,w.source_type,w.branch_id,w.status,w.reason FROM work w WHERE NOT EXISTS(SELECT 1 FROM fleet_partner_route_stops rs JOIN fleet_partner_route_plans rp ON rp.id=rs.route_plan_id WHERE rp.organization_id=$1 AND rp.status NOT IN('CANCELLED','COMPLETED') AND (rs.order_id=w.id OR rs.shipment_id=w.id))`+branchSQL+` ORDER BY source_type,id LIMIT 200`, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, kind, b, status, reason string
		if e = rows.Scan(&id, &kind, &b, &status, &reason); e != nil {
			return nil, e
		}
		if !t.BranchPermitted(b) {
			continue
		}
		out = append(out, map[string]any{"id": id, "sourceType": kind, "branchId": b, "status": status, "actionReason": reason})
	}
	return out, rows.Err()
}
func (s *Service) assignmentSummary(ctx context.Context, t tenant.Context, f PageFilter, statuses string) ([]map[string]any, error) {
	rows, e := s.o.Pool.Query(ctx, `SELECT id,branch_id,work_origin,status,provider_id,vehicle_or_truck_reference,execution_service,response_deadline FROM fleet_partner_private_assignments WHERE organization_id=$1 AND status=ANY(string_to_array($2,',')) ORDER BY created_at DESC LIMIT 200`, t.OrganizationID, statuses)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, b, origin, status, provider, vehicle, service string
		var deadline time.Time
		if e = rows.Scan(&id, &b, &origin, &status, &provider, &vehicle, &service, &deadline); e != nil {
			return nil, e
		}
		if !t.BranchPermitted(b) || f.BranchID != "" && f.BranchID != b || f.Service != "" && f.Service != service || f.ProviderID != "" && f.ProviderID != provider || f.VehicleReference != "" && f.VehicleReference != vehicle {
			continue
		}
		out = append(out, map[string]any{"id": id, "branchId": b, "workOrigin": origin, "status": status, "providerId": provider, "vehicleOrTruckReference": vehicle, "executionService": service, "responseDeadline": deadline})
	}
	return out, rows.Err()
}

func (s *Service) IngestTracking(ctx context.Context, e TrackingEvent) (TrackingProjection, string, error) {
	if x := s.gate(s.o.Flags.TrackingProjections, true, "tracking_projections_disabled"); x != nil {
		return TrackingProjection{}, "", x
	}
	if !s.o.Flags.TrackingSourceConfigured {
		return TrackingProjection{}, "", errors.New("authoritative tracking source is not configured")
	}
	if e.OrganizationID == "" || e.CanonicalTripReference == "" || e.EventID == "" || e.SourceVersion <= 0 || e.SourceTimestamp.IsZero() {
		return TrackingProjection{}, "", apperrors.BadRequest("tracking event identity, version and sourceTimestamp are required.", nil)
	}
	if e.SourceService != "driver-dispatch-delivery-service" && e.SourceService != "driver-hauling-service" {
		return TrackingProjection{}, "", apperrors.BadRequest("sourceService is not authoritative.", nil)
	}
	if (e.Latitude == nil) != (e.Longitude == nil) {
		return TrackingProjection{}, "", apperrors.BadRequest("latitude and longitude must be supplied together.", nil)
	}
	raw, _ := json.Marshal(e)
	sum := sha256sum(raw)
	now := s.o.Now().UTC()
	var out TrackingProjection
	disposition := "APPLIED"
	err := s.tx(ctx, func(tx pgx.Tx) error {
		tag, x := tx.Exec(ctx, `INSERT INTO fleet_partner_tracking_event_inbox(event_id,organization_id,source_service,canonical_trip_reference,source_version,payload_hash) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(event_id) DO NOTHING`, e.EventID, e.OrganizationID, e.SourceService, e.CanonicalTripReference, e.SourceVersion, sum)
		if x != nil {
			return x
		}
		if tag.RowsAffected() == 0 {
			disposition = "DUPLICATE"
			return nil
		}
		var oldVersion int64
		var oldHash, oldGapStatus string
		x = tx.QueryRow(ctx, `SELECT source_version,payload_hash,gap_status FROM fleet_partner_tracking_projections WHERE source_service=$1 AND canonical_trip_reference=$2 FOR UPDATE`, e.SourceService, e.CanonicalTripReference).Scan(&oldVersion, &oldHash, &oldGapStatus)
		if x != nil && !errors.Is(x, pgx.ErrNoRows) {
			return x
		}
		if x == nil && e.SourceVersion < oldVersion {
			disposition = "LOWER_VERSION_IGNORED"
			return nil
		}
		if x == nil && e.SourceVersion == oldVersion {
			if oldHash == sum {
				disposition = "DUPLICATE"
				return nil
			}
			disposition = "CONFLICT"
			_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_tracking_projection_conflicts(id,organization_id,source_service,canonical_trip_reference,source_version,existing_payload_hash,received_payload_hash) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, ids.New("tpc"), e.OrganizationID, e.SourceService, e.CanonicalTripReference, e.SourceVersion, oldHash, sum)
			return x
		}
		gap := x == nil && (oldGapStatus != "CURRENT" || e.SourceVersion > oldVersion+1)
		reconciled := gap && e.ReconcilesThroughVersion >= e.SourceVersion
		if reconciled {
			gap = false
			disposition = "RECONCILED"
		}
		fresh := Freshness(e.SourceTimestamp, now, s.o.TrackingFreshAfter, s.o.TrackingStaleAfter, gap)
		gapState := "CURRENT"
		if gap {
			gapState = "GAP_DETECTED"
			disposition = "GAP_DETECTED"
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_tracking_projections(organization_id,source_service,canonical_trip_reference,assignment_id,route_plan_id,provider_id,vehicle_or_truck_reference,latitude,longitude,heading,speed,accuracy,operational_status,source_timestamp,received_at,source_version,payload_hash,freshness_status,gap_status,last_reconciled_at,retention_expires_at) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21) ON CONFLICT(source_service,canonical_trip_reference) DO UPDATE SET organization_id=EXCLUDED.organization_id,assignment_id=EXCLUDED.assignment_id,route_plan_id=EXCLUDED.route_plan_id,provider_id=EXCLUDED.provider_id,vehicle_or_truck_reference=EXCLUDED.vehicle_or_truck_reference,latitude=EXCLUDED.latitude,longitude=EXCLUDED.longitude,heading=EXCLUDED.heading,speed=EXCLUDED.speed,accuracy=EXCLUDED.accuracy,operational_status=EXCLUDED.operational_status,source_timestamp=EXCLUDED.source_timestamp,received_at=EXCLUDED.received_at,source_version=EXCLUDED.source_version,payload_hash=EXCLUDED.payload_hash,freshness_status=EXCLUDED.freshness_status,gap_status=EXCLUDED.gap_status,last_reconciled_at=COALESCE(EXCLUDED.last_reconciled_at,fleet_partner_tracking_projections.last_reconciled_at),retention_expires_at=EXCLUDED.retention_expires_at`, e.OrganizationID, e.SourceService, e.CanonicalTripReference, e.AssignmentID, e.RoutePlanID, e.ProviderID, e.VehicleOrTruckReference, e.Latitude, e.Longitude, e.Heading, e.Speed, e.Accuracy, e.OperationalStatus, e.SourceTimestamp, now, e.SourceVersion, sum, fresh, gapState, nullableReconciledAt(reconciled, now), now.Add(s.o.TrackingRetention))
		if x != nil {
			return x
		}
		out = TrackingProjection{OrganizationID: e.OrganizationID, SourceService: e.SourceService, CanonicalTripReference: e.CanonicalTripReference, AssignmentID: e.AssignmentID, RoutePlanID: e.RoutePlanID, ProviderID: e.ProviderID, VehicleOrTruckReference: e.VehicleOrTruckReference, Latitude: e.Latitude, Longitude: e.Longitude, Heading: e.Heading, Speed: e.Speed, Accuracy: e.Accuracy, OperationalStatus: e.OperationalStatus, SourceTimestamp: e.SourceTimestamp, ReceivedAt: now, SourceVersion: e.SourceVersion, FreshnessStatus: fresh, GapStatus: gapState, LastReconciledAt: nullableReconciledAt(reconciled, now)}
		if gap {
			_, x = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: e.OrganizationID, JobType: JobTrackingReconcile, DedupeKey: e.SourceService + ":" + e.CanonicalTripReference, Payload: map[string]any{"organizationId": e.OrganizationID, "sourceService": e.SourceService, "canonicalTripReference": e.CanonicalTripReference}, MaxAttempts: 8})
			if x != nil {
				return x
			}
			if s.o.Flags.OperationalAlerts {
				_, x = s.createAlert(ctx, tx, tenant.Context{OrganizationID: e.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "tracking-projection"}, "TRACKING_GAP", "HIGH", "CANONICAL_TRIP", e.CanonicalTripReference, "", e.RoutePlanID, e.AssignmentID, e.CanonicalTripReference, "tracking-gap:"+e.SourceService+":"+e.CanonicalTripReference)
				if x != nil {
					return x
				}
			}
		}
		if s.o.Flags.Geofences {
			_, x = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: e.OrganizationID, JobType: JobGeofenceEvaluate, DedupeKey: e.SourceService + ":" + e.CanonicalTripReference + ":" + fmt.Sprint(e.SourceVersion), Payload: map[string]any{"organizationId": e.OrganizationID, "sourceService": e.SourceService, "canonicalTripReference": e.CanonicalTripReference, "routePlanId": e.RoutePlanID}, MaxAttempts: 8})
			if x != nil {
				return x
			}
		}
		if e.RoutePlanID != "" {
			_, x = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: e.OrganizationID, JobType: JobRouteProgress, DedupeKey: e.RoutePlanID + ":" + fmt.Sprint(e.SourceVersion), Payload: map[string]any{"organizationId": e.OrganizationID, "routePlanId": e.RoutePlanID}, MaxAttempts: 8})
			if x != nil {
				return x
			}
		}
		if s.o.Events != nil {
			eventType := "tracking_projection.updated.v1"
			if gap {
				eventType = "tracking_projection.gap_detected.v1"
			} else if reconciled {
				eventType = "tracking_projection.reconciled.v1"
			}
			if x = s.o.Events.Publish(ctx, tx, eventType, "tracking_projection", e.CanonicalTripReference, e.SourceVersion, e.OrganizationID, "", map[string]any{"canonicalTripReference": e.CanonicalTripReference, "sourceService": e.SourceService, "sourceVersion": e.SourceVersion, "freshnessStatus": fresh, "gapStatus": gapState}); x != nil {
				return x
			}
		}
		return nil
	})
	return out, disposition, err
}
func sha256sum(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func nullableReconciledAt(reconciled bool, at time.Time) *time.Time {
	if !reconciled {
		return nil
	}
	return &at
}
func (s *Service) GetTracking(ctx context.Context, t tenant.Context, trip string) (TrackingProjection, error) {
	if x := s.gate(s.o.Flags.TrackingProjections, false, "tracking_projections_disabled"); x != nil {
		return TrackingProjection{}, x
	}
	v, e := s.o.Repository.Tracking(ctx, t.OrganizationID, trip)
	if e != nil {
		return TrackingProjection{}, nf()
	}
	if v.RoutePlanID != "" {
		route, x := s.o.Repository.GetRoute(ctx, nil, t.OrganizationID, v.RoutePlanID, false)
		if x != nil || !t.BranchPermitted(route.BranchID) {
			return TrackingProjection{}, nf()
		}
	}
	v.FreshnessStatus = Freshness(v.SourceTimestamp, s.o.Now(), s.o.TrackingFreshAfter, s.o.TrackingStaleAfter, v.GapStatus != "CURRENT")
	return v, nil
}
func (s *Service) ListTracking(ctx context.Context, t tenant.Context, f PageFilter) ([]TrackingProjection, error) {
	if x := s.gate(s.o.Flags.TrackingProjections, false, "tracking_projections_disabled"); x != nil {
		return nil, x
	}
	items, e := s.o.Repository.ListTracking(ctx, t.OrganizationID, f)
	out := items[:0]
	for i := range items {
		items[i].FreshnessStatus = Freshness(items[i].SourceTimestamp, s.o.Now(), s.o.TrackingFreshAfter, s.o.TrackingStaleAfter, items[i].GapStatus != "CURRENT")
		if items[i].RoutePlanID != "" {
			route, x := s.o.Repository.GetRoute(ctx, nil, t.OrganizationID, items[i].RoutePlanID, false)
			if x != nil || !t.BranchPermitted(route.BranchID) {
				continue
			}
		}
		out = append(out, items[i])
	}
	return out, e
}

type CreateGeofenceInput struct {
	BranchID, Name, GeometryType, GeometryReference string
	Geometry                                        Geometry
	EventRules                                      []string
	IdempotencyKey                                  string
}

func (s *Service) CreateGeofence(ctx context.Context, t tenant.Context, in CreateGeofenceInput) (Geofence, error) {
	if x := s.gate(s.o.Flags.Geofences, true, "geofences_disabled"); x != nil {
		return Geofence{}, x
	}
	if in.BranchID != "" && !t.BranchPermitted(in.BranchID) {
		return Geofence{}, nf()
	}
	if strings.TrimSpace(in.Name) == "" || in.IdempotencyKey == "" {
		return Geofence{}, apperrors.BadRequest("name and Idempotency-Key are required.", nil)
	}
	if x := ValidateGeometry(in.GeometryType, in.Geometry); x != nil {
		return Geofence{}, apperrors.BadRequest(x.Error(), x)
	}
	in.EventRules = normalizeRules(in.EventRules)
	for _, r := range in.EventRules {
		if !map[string]bool{"ENTER": true, "EXIT": true, "DWELL": true, "ROUTE_DEVIATION": true, "RESTRICTED_AREA": true}[r] {
			return Geofence{}, apperrors.BadRequest("eventRules contains an invalid rule.", nil)
		}
	}
	raw, _ := json.Marshal(in.Geometry)
	rules, _ := json.Marshal(in.EventRules)
	var out Geofence
	x := s.tx(ctx, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_geofences(id,organization_id,branch_id,name,geometry_type,geometry,geometry_reference,event_rules,active_status,created_by,idempotency_key,request_fingerprint) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,'INACTIVE',$9,$10,$11) ON CONFLICT(organization_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE fleet_partner_geofences.request_fingerprint=EXCLUDED.request_fingerprint RETURNING id,organization_id,COALESCE(branch_id,''),name,geometry_type,geometry,geometry_reference,event_rules,active_status,version,created_by,created_at,updated_at`, ids.New("geo"), t.OrganizationID, in.BranchID, in.Name, strings.ToUpper(in.GeometryType), raw, in.GeometryReference, rules, t.ActorID, in.IdempotencyKey, fingerprint(in)).Scan(&out.ID, &out.OrganizationID, &out.BranchID, &out.Name, &out.GeometryType, &raw, &out.GeometryReference, &rules, &out.ActiveStatus, &out.Version, &out.CreatedBy, &out.CreatedAt, &out.UpdatedAt); e != nil {
			return e
		}
		return s.record(ctx, tx, t, "geofence.created.v1", "geofence", out.ID, out.Version, out)
	})
	if x == nil {
		_ = json.Unmarshal(raw, &out.Geometry)
		_ = json.Unmarshal(rules, &out.EventRules)
	}
	return out, x
}
func scanGeofence(row pgx.Row) (Geofence, error) {
	var g Geofence
	var geom, rules []byte
	e := row.Scan(&g.ID, &g.OrganizationID, &g.BranchID, &g.Name, &g.GeometryType, &geom, &g.GeometryReference, &rules, &g.ActiveStatus, &g.Version, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	_ = json.Unmarshal(geom, &g.Geometry)
	_ = json.Unmarshal(rules, &g.EventRules)
	return g, e
}
func (s *Service) GetGeofence(ctx context.Context, t tenant.Context, id string) (Geofence, error) {
	if x := s.gate(s.o.Flags.Geofences, false, "geofences_disabled"); x != nil {
		return Geofence{}, x
	}
	g, e := scanGeofence(s.o.Pool.QueryRow(ctx, `SELECT id,organization_id,COALESCE(branch_id,''),name,geometry_type,geometry,geometry_reference,event_rules,active_status,version,created_by,created_at,updated_at FROM fleet_partner_geofences WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil || !t.BranchPermitted(g.BranchID) {
		return Geofence{}, nf()
	}
	return g, nil
}
func (s *Service) ListGeofences(ctx context.Context, t tenant.Context, f PageFilter) ([]Geofence, error) {
	if x := s.gate(s.o.Flags.Geofences, false, "geofences_disabled"); x != nil {
		return nil, x
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,COALESCE(branch_id,''),name,geometry_type,geometry,geometry_reference,event_rules,active_status,version,created_by,created_at,updated_at FROM fleet_partner_geofences WHERE organization_id=$1 AND ($2='' OR branch_id=$2) ORDER BY created_at DESC LIMIT $3 OFFSET $4`, t.OrganizationID, f.BranchID, bounded(f.Limit), f.Offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Geofence{}
	for rows.Next() {
		g, e := scanGeofence(rows)
		if e != nil {
			return nil, e
		}
		if t.BranchPermitted(g.BranchID) {
			out = append(out, g)
		}
	}
	return out, rows.Err()
}
func (s *Service) UpdateGeofence(ctx context.Context, t tenant.Context, id string, in CreateGeofenceInput, expected int64) (Geofence, error) {
	if x := s.gate(s.o.Flags.Geofences, true, "geofences_disabled"); x != nil {
		return Geofence{}, x
	}
	if x := requireVersion(expected); x != nil {
		return Geofence{}, x
	}
	if x := ValidateGeometry(in.GeometryType, in.Geometry); x != nil {
		return Geofence{}, apperrors.BadRequest(x.Error(), x)
	}
	raw, _ := json.Marshal(in.Geometry)
	rules, _ := json.Marshal(normalizeRules(in.EventRules))
	var g Geofence
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		g, x = scanGeofence(tx.QueryRow(ctx, `UPDATE fleet_partner_geofences SET name=COALESCE(NULLIF($4,''),name),geometry_type=$5,geometry=$6,geometry_reference=$7,event_rules=$8,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING id,organization_id,COALESCE(branch_id,''),name,geometry_type,geometry,geometry_reference,event_rules,active_status,version,created_by,created_at,updated_at`, t.OrganizationID, id, expected, in.Name, strings.ToUpper(in.GeometryType), raw, in.GeometryReference, rules))
		if x == ErrNotFound {
			return ErrConflict
		}
		if x != nil {
			return x
		}
		if !t.BranchPermitted(g.BranchID) {
			return nf()
		}
		return s.record(ctx, tx, t, "geofence.updated.v1", "geofence", g.ID, g.Version, g)
	})
	if e == ErrNotFound {
		return g, ErrConflict
	}
	if !t.BranchPermitted(g.BranchID) {
		return Geofence{}, nf()
	}
	return g, e
}
func (s *Service) SetGeofenceActive(ctx context.Context, t tenant.Context, id, status string, expected int64) (Geofence, error) {
	if x := s.gate(s.o.Flags.Geofences, true, "geofences_disabled"); x != nil {
		return Geofence{}, x
	}
	if x := requireVersion(expected); x != nil {
		return Geofence{}, x
	}
	var g Geofence
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		g, x = scanGeofence(tx.QueryRow(ctx, `UPDATE fleet_partner_geofences SET active_status=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING id,organization_id,COALESCE(branch_id,''),name,geometry_type,geometry,geometry_reference,event_rules,active_status,version,created_by,created_at,updated_at`, t.OrganizationID, id, expected, status))
		if x == ErrNotFound {
			return ErrConflict
		}
		if x != nil {
			return x
		}
		if !t.BranchPermitted(g.BranchID) {
			return nf()
		}
		event := "geofence.deactivated.v1"
		if status == "ACTIVE" {
			event = "geofence.activated.v1"
		}
		return s.record(ctx, tx, t, event, "geofence", g.ID, g.Version, g)
	})
	if e == ErrNotFound {
		return g, ErrConflict
	}
	if !t.BranchPermitted(g.BranchID) {
		return Geofence{}, nf()
	}
	return g, e
}

func scanAlert(row pgx.Row) (OperationalAlert, error) {
	var a OperationalAlert
	e := row.Scan(&a.ID, &a.OrganizationID, &a.BranchID, &a.AlertType, &a.Severity, &a.SourceType, &a.SourceID, &a.RoutePlanID, &a.AssignmentID, &a.ShipmentID, &a.CanonicalTripReference, &a.OwnerUserID, &a.Status, &a.SLADueAt, &a.AcknowledgedAt, &a.ResolvedAt, &a.ResolutionCode, &a.ResolutionReason, &a.Version, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return a, e
}

const alertColumns = `id,organization_id,COALESCE(branch_id,''),alert_type,severity,source_type,source_id,COALESCE(route_plan_id,''),assignment_id,shipment_id,canonical_trip_reference,owner_user_id,status,sla_due_at,acknowledged_at,resolved_at,resolution_code,resolution_reason,version,created_at,updated_at`

func (s *Service) createAlert(ctx context.Context, tx pgx.Tx, t tenant.Context, kind, severity, sourceType, sourceID, branchID, routeID, assignmentID, trip, condition string) (OperationalAlert, error) {
	var a OperationalAlert
	var inserted bool
	e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_operational_alerts(id,organization_id,branch_id,alert_type,severity,source_type,source_id,route_plan_id,assignment_id,canonical_trip_reference,condition_fingerprint,sla_due_at) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12) ON CONFLICT(organization_id,condition_fingerprint) WHERE status IN('OPEN','ACKNOWLEDGED','IN_PROGRESS') DO UPDATE SET updated_at=fleet_partner_operational_alerts.updated_at RETURNING `+alertColumns+`,(xmax=0)`, ids.New("alt"), t.OrganizationID, branchID, kind, severity, sourceType, sourceID, routeID, assignmentID, trip, condition, s.o.Now().Add(s.o.AlertSLA)).Scan(&a.ID, &a.OrganizationID, &a.BranchID, &a.AlertType, &a.Severity, &a.SourceType, &a.SourceID, &a.RoutePlanID, &a.AssignmentID, &a.ShipmentID, &a.CanonicalTripReference, &a.OwnerUserID, &a.Status, &a.SLADueAt, &a.AcknowledgedAt, &a.ResolvedAt, &a.ResolutionCode, &a.ResolutionReason, &a.Version, &a.CreatedAt, &a.UpdatedAt, &inserted)
	if e == nil && inserted {
		e = s.record(ctx, tx, t, "operational_alert.created.v1", "operational_alert", a.ID, a.Version, map[string]any{"alert": a, "notificationIntent": true})
		if e == nil && s.o.Flags.Workers {
			_, e = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: t.OrganizationID, JobType: JobAlertEscalate, DedupeKey: "alert-sla:" + a.ID, Payload: map[string]any{"organizationId": t.OrganizationID, "alertId": a.ID}, ScheduledAt: a.SLADueAt, MaxAttempts: 8})
		}
	}
	return a, e
}

// CreateSystemAlert is the narrow cross-feature port used by asset safety.
// It preserves the Section 16 alert dedupe, audit, outbox and escalation rules.
func (s *Service) CreateSystemAlert(ctx context.Context, organizationID, kind, sourceID, reference, severity, message string) error {
	if x := s.gate(s.o.Flags.OperationalAlerts, true, "operational_alerts_disabled"); x != nil {
		return x
	}
	t := tenant.Context{OrganizationID: organizationID, ActorType: tenant.ActorTypeSystem, ActorID: "asset-safety", CorrelationID: reference}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, err := s.createAlert(ctx, tx, t, kind, severity, "ASSET", sourceID, "", "", reference, "", "asset-safety:"+kind+":"+sourceID+":"+reference)
		return err
	})
}
func (s *Service) GetAlert(ctx context.Context, t tenant.Context, id string) (OperationalAlert, error) {
	if x := s.gate(s.o.Flags.OperationalAlerts, false, "operational_alerts_disabled"); x != nil {
		return OperationalAlert{}, x
	}
	a, e := scanAlert(s.o.Pool.QueryRow(ctx, `SELECT `+alertColumns+` FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil || !t.BranchPermitted(a.BranchID) {
		return OperationalAlert{}, nf()
	}
	return a, nil
}
func (s *Service) ListAlerts(ctx context.Context, t tenant.Context, f PageFilter) ([]OperationalAlert, error) {
	if x := s.gate(s.o.Flags.OperationalAlerts, false, "operational_alerts_disabled"); x != nil {
		return nil, x
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+alertColumns+` FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND ($2='' OR branch_id=$2) AND ($3='' OR status=$3) AND ($4='' OR alert_type=$4) ORDER BY created_at DESC LIMIT $5 OFFSET $6`, t.OrganizationID, f.BranchID, f.Status, f.AlertType, bounded(f.Limit), f.Offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []OperationalAlert{}
	for rows.Next() {
		a, e := scanAlert(rows)
		if e != nil {
			return nil, e
		}
		if t.BranchPermitted(a.BranchID) {
			out = append(out, a)
		}
	}
	return out, rows.Err()
}
func (s *Service) AcknowledgeAlert(ctx context.Context, t tenant.Context, id string, expected int64) (OperationalAlert, error) {
	return s.updateAlert(ctx, t, id, expected, AlertAcknowledged, "", "")
}
func (s *Service) ResolveAlert(ctx context.Context, t tenant.Context, id string, expected int64, code, reason string) (OperationalAlert, error) {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(reason) == "" {
		return OperationalAlert{}, apperrors.BadRequest("resolutionCode and resolutionReason are required.", nil)
	}
	return s.updateAlert(ctx, t, id, expected, AlertResolved, code, reason)
}
func (s *Service) updateAlert(ctx context.Context, t tenant.Context, id string, expected int64, to, code, reason string) (OperationalAlert, error) {
	if x := requireVersion(expected); x != nil {
		return OperationalAlert{}, x
	}
	var out OperationalAlert
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := scanAlert(tx.QueryRow(ctx, `SELECT `+alertColumns+` FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id))
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return nf()
		}
		if !ValidAlertTransition(old.Status, to) {
			return errors.New("invalid alert transition")
		}
		x = tx.QueryRow(ctx, `UPDATE fleet_partner_operational_alerts SET status=$4,owner_user_id=CASE WHEN $4='ACKNOWLEDGED' THEN $5 ELSE owner_user_id END,acknowledged_at=CASE WHEN $4='ACKNOWLEDGED' THEN now() ELSE acknowledged_at END,resolved_at=CASE WHEN $4='RESOLVED' THEN now() ELSE resolved_at END,resolution_code=$6,resolution_reason=$7,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+alertColumns, t.OrganizationID, id, expected, to, t.OrganizationUserID, code, reason).Scan(&out.ID, &out.OrganizationID, &out.BranchID, &out.AlertType, &out.Severity, &out.SourceType, &out.SourceID, &out.RoutePlanID, &out.AssignmentID, &out.ShipmentID, &out.CanonicalTripReference, &out.OwnerUserID, &out.Status, &out.SLADueAt, &out.AcknowledgedAt, &out.ResolvedAt, &out.ResolutionCode, &out.ResolutionReason, &out.Version, &out.CreatedAt, &out.UpdatedAt)
		if x != nil {
			return ErrConflict
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_operational_alert_history(id,organization_id,alert_id,previous_status,new_status,actor_id,reason,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("alh"), t.OrganizationID, id, old.Status, to, t.ActorID, reason, out.Version)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "operational_alert."+strings.ToLower(to)+".v1", "operational_alert", id, out.Version, out)
	})
	return out, e
}
