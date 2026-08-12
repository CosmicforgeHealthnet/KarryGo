package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

var ErrNotFound = errors.New("section16: not found")
var ErrConflict = errors.New("section16: conflict")

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
func (r *Repository) Pool() *pgxpool.Pool          { return r.pool }

const planColumns = `id,organization_id,branch_id,plan_date,work_window_start,work_window_end,timezone,status,planner_user_id,COALESCE(approved_by,''),approved_at,published_at,version,created_at,updated_at`

func scanPlan(row pgx.Row) (DispatchPlan, error) {
	var v DispatchPlan
	err := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.PlanDate, &v.WorkWindowStart, &v.WorkWindowEnd, &v.Timezone, &v.Status, &v.PlannerUserID, &v.ApprovedBy, &v.ApprovedAt, &v.PublishedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return v, err
}
func (r *Repository) CreatePlan(ctx context.Context, q database.Querier, p DispatchPlan, key, fp string) (DispatchPlan, error) {
	return scanPlan(q.QueryRow(ctx, `INSERT INTO fleet_partner_dispatch_plans(id,organization_id,branch_id,plan_date,work_window_start,work_window_end,timezone,status,planner_user_id,idempotency_key,request_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(organization_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE fleet_partner_dispatch_plans.request_fingerprint=EXCLUDED.request_fingerprint RETURNING `+planColumns, p.ID, p.OrganizationID, p.BranchID, p.PlanDate, p.WorkWindowStart, p.WorkWindowEnd, p.Timezone, p.Status, p.PlannerUserID, key, fp))
}
func (r *Repository) GetPlan(ctx context.Context, q database.Querier, org, id string, lock bool) (DispatchPlan, error) {
	if q == nil {
		q = r.pool
	}
	s := `SELECT ` + planColumns + ` FROM fleet_partner_dispatch_plans WHERE organization_id=$1 AND id=$2`
	if lock {
		s += ` FOR UPDATE`
	}
	return scanPlan(q.QueryRow(ctx, s, org, id))
}
func (r *Repository) ListPlans(ctx context.Context, org string, f PageFilter) ([]DispatchPlan, error) {
	args := []any{org}
	where := ` organization_id=$1`
	if f.BranchID != "" {
		args = append(args, f.BranchID)
		where += fmt.Sprintf(` AND branch_id=$%d`, len(args))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where += fmt.Sprintf(` AND status=$%d`, len(args))
	}
	limit := bounded(f.Limit)
	args = append(args, limit, f.Offset)
	rows, e := r.pool.Query(ctx, `SELECT `+planColumns+` FROM fleet_partner_dispatch_plans WHERE`+where+fmt.Sprintf(` ORDER BY plan_date DESC,id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []DispatchPlan{}
	for rows.Next() {
		v, e := scanPlan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) UpdatePlan(ctx context.Context, q database.Querier, org, id string, expected int64, status, timezone string, start, end time.Time, approver string) (DispatchPlan, error) {
	tag, row := ``, pgx.Row(nil)
	_ = tag
	row = q.QueryRow(ctx, `UPDATE fleet_partner_dispatch_plans SET status=COALESCE(NULLIF($4,''),status),timezone=COALESCE(NULLIF($5,''),timezone),work_window_start=CASE WHEN $6::timestamptz IS NULL THEN work_window_start ELSE $6 END,work_window_end=CASE WHEN $7::timestamptz IS NULL THEN work_window_end ELSE $7 END,approved_by=CASE WHEN $4='APPROVED' THEN $8 ELSE approved_by END,approved_at=CASE WHEN $4='APPROVED' THEN now() ELSE approved_at END,published_at=CASE WHEN $4='PUBLISHED' THEN now() ELSE published_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+planColumns, org, id, expected, status, timezone, nullableTime(start), nullableTime(end), approver)
	v, e := scanPlan(row)
	if e == ErrNotFound {
		return v, ErrConflict
	}
	return v, e
}

const routeColumns = `id,organization_id,branch_id,COALESCE(dispatch_plan_id,''),COALESCE(replacement_for_route_id,''),route_reference,route_type,provider_membership_id,provider_id,vehicle_or_truck_reference,execution_service,status,planned_distance,distance_unit,planned_duration_seconds,optimization_reference,optimization_version,constraint_snapshot,COALESCE(approved_by,''),approved_at,published_at,version,created_at,updated_at`

func scanRoute(row pgx.Row) (RoutePlan, error) {
	var v RoutePlan
	var constraints []byte
	err := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.DispatchPlanID, &v.ReplacementForRouteID, &v.RouteReference, &v.RouteType, &v.ProviderMembershipID, &v.ProviderID, &v.VehicleOrTruckReference, &v.ExecutionService, &v.Status, &v.PlannedDistance, &v.DistanceUnit, &v.PlannedDurationSeconds, &v.OptimizationReference, &v.OptimizationVersion, &constraints, &v.ApprovedBy, &v.ApprovedAt, &v.PublishedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	v.ConstraintSnapshot = constraints
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return v, err
}
func (r *Repository) CreateRoute(ctx context.Context, q database.Querier, v RoutePlan, key, fp string) (RoutePlan, error) {
	return scanRoute(q.QueryRow(ctx, `INSERT INTO fleet_partner_route_plans(id,organization_id,branch_id,dispatch_plan_id,replacement_for_route_id,route_reference,route_type,provider_membership_id,provider_id,vehicle_or_truck_reference,execution_service,status,constraint_snapshot,idempotency_key,request_fingerprint) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT(organization_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE fleet_partner_route_plans.request_fingerprint=EXCLUDED.request_fingerprint RETURNING `+routeColumns, v.ID, v.OrganizationID, v.BranchID, v.DispatchPlanID, v.ReplacementForRouteID, v.RouteReference, v.RouteType, v.ProviderMembershipID, v.ProviderID, v.VehicleOrTruckReference, v.ExecutionService, v.Status, v.ConstraintSnapshot, key, fp))
}
func (r *Repository) GetRoute(ctx context.Context, q database.Querier, org, id string, lock bool) (RoutePlan, error) {
	if q == nil {
		q = r.pool
	}
	s := `SELECT ` + routeColumns + ` FROM fleet_partner_route_plans WHERE organization_id=$1 AND id=$2`
	if lock {
		s += ` FOR UPDATE`
	}
	return scanRoute(q.QueryRow(ctx, s, org, id))
}
func (r *Repository) ListRoutes(ctx context.Context, org string, f PageFilter) ([]RoutePlan, error) {
	args := []any{org}
	where := ` organization_id=$1`
	for _, x := range []struct{ v, c string }{{f.BranchID, "branch_id"}, {f.Status, "status"}, {f.Service, "execution_service"}, {f.ProviderID, "provider_id"}, {f.VehicleReference, "vehicle_or_truck_reference"}} {
		if x.v != "" {
			args = append(args, x.v)
			where += fmt.Sprintf(` AND %s=$%d`, x.c, len(args))
		}
	}
	args = append(args, bounded(f.Limit), f.Offset)
	rows, e := r.pool.Query(ctx, `SELECT `+routeColumns+` FROM fleet_partner_route_plans WHERE`+where+fmt.Sprintf(` ORDER BY created_at DESC,id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []RoutePlan{}
	for rows.Next() {
		v, e := scanRoute(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) UpdateRoute(ctx context.Context, q database.Querier, org, id string, expected int64, status, providerMembership, provider, vehicle string, constraints json.RawMessage, optimizationRef string, optimizationVersion int64, distance float64, duration int, actor string) (RoutePlan, error) {
	if len(constraints) == 0 {
		constraints = nil
	}
	v, e := scanRoute(q.QueryRow(ctx, `UPDATE fleet_partner_route_plans SET status=COALESCE(NULLIF($4,''),status),provider_membership_id=COALESCE(NULLIF($5,''),provider_membership_id),provider_id=COALESCE(NULLIF($6,''),provider_id),vehicle_or_truck_reference=COALESCE(NULLIF($7,''),vehicle_or_truck_reference),constraint_snapshot=COALESCE($8,constraint_snapshot),optimization_reference=COALESCE(NULLIF($9,''),optimization_reference),optimization_version=CASE WHEN $10>0 THEN $10 ELSE optimization_version END,planned_distance=CASE WHEN $11>=0 THEN $11 ELSE planned_distance END,planned_duration_seconds=CASE WHEN $12>=0 THEN $12 ELSE planned_duration_seconds END,approved_by=CASE WHEN $4='APPROVED' THEN $13 ELSE approved_by END,approved_at=CASE WHEN $4='APPROVED' THEN now() ELSE approved_at END,published_at=CASE WHEN $4='PUBLISHED' THEN now() ELSE published_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+routeColumns, org, id, expected, status, providerMembership, provider, vehicle, constraints, optimizationRef, optimizationVersion, distance, duration, actor))
	if e == ErrNotFound {
		return v, ErrConflict
	}
	return v, e
}

const stopColumns = `id,organization_id,route_plan_id,sequence,stop_type,COALESCE(order_id,''),COALESCE(shipment_id,''),COALESCE(package_id,''),COALESCE(manifest_id,''),COALESCE(source_order_stop_id,''),location_reference,latitude,longitude,time_window_start,time_window_end,service_duration_seconds,planned_arrival_at,planned_departure_at,actual_arrival_at,actual_departure_at,status,version,created_at,updated_at`

func scanStop(row pgx.Row) (RouteStop, error) {
	var v RouteStop
	err := row.Scan(&v.ID, &v.OrganizationID, &v.RoutePlanID, &v.Sequence, &v.StopType, &v.OrderID, &v.ShipmentID, &v.PackageID, &v.ManifestID, &v.SourceOrderStopID, &v.LocationReference, &v.Latitude, &v.Longitude, &v.TimeWindowStart, &v.TimeWindowEnd, &v.ServiceDurationSeconds, &v.PlannedArrivalAt, &v.PlannedDepartureAt, &v.ActualArrivalAt, &v.ActualDepartureAt, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return v, err
}
func (r *Repository) CreateStop(ctx context.Context, q database.Querier, v RouteStop) (RouteStop, error) {
	return scanStop(q.QueryRow(ctx, `INSERT INTO fleet_partner_route_stops(id,organization_id,route_plan_id,sequence,stop_type,order_id,shipment_id,package_id,manifest_id,source_order_stop_id,location_reference,latitude,longitude,time_window_start,time_window_end,service_duration_seconds,planned_arrival_at,planned_departure_at,status) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING `+stopColumns, v.ID, v.OrganizationID, v.RoutePlanID, v.Sequence, v.StopType, v.OrderID, v.ShipmentID, v.PackageID, v.ManifestID, v.SourceOrderStopID, v.LocationReference, v.Latitude, v.Longitude, v.TimeWindowStart, v.TimeWindowEnd, v.ServiceDurationSeconds, v.PlannedArrivalAt, v.PlannedDepartureAt, v.Status))
}
func (r *Repository) ListStops(ctx context.Context, q database.Querier, org, route string) ([]RouteStop, error) {
	if q == nil {
		q = r.pool
	}
	rows, e := q.Query(ctx, `SELECT `+stopColumns+` FROM fleet_partner_route_stops WHERE organization_id=$1 AND route_plan_id=$2 ORDER BY sequence`, org, route)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []RouteStop{}
	for rows.Next() {
		v, e := scanStop(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *Repository) UpdateStop(ctx context.Context, q database.Querier, org, route, id string, expected int64, v RouteStop) (RouteStop, error) {
	out, e := scanStop(q.QueryRow(ctx, `UPDATE fleet_partner_route_stops SET stop_type=COALESCE(NULLIF($6,''),stop_type),location_reference=COALESCE(NULLIF($7,''),location_reference),time_window_start=COALESCE($8,time_window_start),time_window_end=COALESCE($9,time_window_end),service_duration_seconds=CASE WHEN $10>=0 THEN $10 ELSE service_duration_seconds END,planned_arrival_at=COALESCE($11,planned_arrival_at),planned_departure_at=COALESCE($12,planned_departure_at),version=version+1,updated_at=now() WHERE organization_id=$1 AND route_plan_id=$2 AND id=$3 AND version=$4 AND status=$5 RETURNING `+stopColumns, org, route, id, expected, StopPlanned, v.StopType, v.LocationReference, v.TimeWindowStart, v.TimeWindowEnd, v.ServiceDurationSeconds, v.PlannedArrivalAt, v.PlannedDepartureAt))
	if e == ErrNotFound {
		return out, ErrConflict
	}
	return out, e
}
func (r *Repository) DeleteStop(ctx context.Context, q database.Querier, org, route, id string) error {
	tag, e := q.Exec(ctx, `DELETE FROM fleet_partner_route_stops WHERE organization_id=$1 AND route_plan_id=$2 AND id=$3 AND status=$4`, org, route, id, StopPlanned)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return e
}
func (r *Repository) NormalizeStops(ctx context.Context, q database.Querier, org, route string) error {
	_, e := q.Exec(ctx, `WITH ranked AS(SELECT id,row_number() OVER(ORDER BY sequence,id) AS n FROM fleet_partner_route_stops WHERE organization_id=$1 AND route_plan_id=$2) UPDATE fleet_partner_route_stops s SET sequence=100000+r.n FROM ranked r WHERE s.id=r.id`, org, route)
	if e != nil {
		return e
	}
	_, e = q.Exec(ctx, `UPDATE fleet_partner_route_stops SET sequence=sequence-100000 WHERE organization_id=$1 AND route_plan_id=$2`, org, route)
	return e
}

func (r *Repository) PutCandidate(ctx context.Context, q database.Querier, c Candidate) error {
	components, _ := json.Marshal(c.ScoreComponents)
	reasons, _ := json.Marshal(c.EligibilityReasons)
	warnings, _ := json.Marshal(c.Warnings)
	blocks, _ := json.Marshal(c.HardBlocks)
	versions, _ := json.Marshal(c.SourceVersions)
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_assignment_candidates(id,organization_id,branch_id,source_type,source_id,provider_id,provider_membership_id,vehicle_or_truck_reference,execution_service,eligibility_status,total_score,score_components,eligibility_reasons,warnings,hard_blocks,data_freshness,generated_at,expires_at,source_versions,input_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) ON CONFLICT(organization_id,source_type,source_id,provider_membership_id,vehicle_or_truck_reference,input_fingerprint) DO NOTHING`, c.ID, c.OrganizationID, c.BranchID, c.SourceType, c.SourceID, c.ProviderID, c.ProviderMembershipID, c.VehicleOrTruckReference, c.ExecutionService, c.EligibilityStatus, c.TotalScore, components, reasons, warnings, blocks, c.DataFreshness, c.GeneratedAt, c.ExpiresAt, versions, c.InputFingerprint)
	return e
}

func (r *Repository) History(ctx context.Context, q database.Querier, org, kind, id, from, to, reason, actor string, version int64, snapshot any) error {
	raw, _ := json.Marshal(snapshot)
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_dispatch_route_history(id,organization_id,aggregate_type,aggregate_id,previous_status,new_status,reason_code,actor_id,version,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("drh"), org, kind, id, from, to, reason, actor, version, raw)
	return e
}

type OptimizationRequest struct {
	ID, OrganizationID, RoutePlanID, Status, ProviderReference, ProviderJobReference, InputFingerprint, IdempotencyKey, RequestFingerprint string
	RouteVersion, OptimizationVersion, ResultVersion                                                                                       int64
	InputSnapshot, ResultSnapshot                                                                                                          json.RawMessage
	EstimatedDistance                                                                                                                      float64
	EstimatedDurationSeconds                                                                                                               int
}

func (r *Repository) CreateOptimization(ctx context.Context, q database.Querier, o OptimizationRequest) error {
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_route_optimization_requests(id,organization_id,route_plan_id,route_version,optimization_version,status,input_fingerprint,input_snapshot,idempotency_key,request_fingerprint,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(organization_id,idempotency_key) DO NOTHING`, o.ID, o.OrganizationID, o.RoutePlanID, o.RouteVersion, o.OptimizationVersion, o.Status, o.InputFingerprint, o.InputSnapshot, o.IdempotencyKey, o.RequestFingerprint, "system")
	return e
}
func (r *Repository) FindOptimization(ctx context.Context, q database.Querier, org, route string, version int64, fingerprint string) (OptimizationRequest, error) {
	if q == nil {
		q = r.pool
	}
	var o OptimizationRequest
	err := q.QueryRow(ctx, `SELECT id,organization_id,route_plan_id,route_version,optimization_version,status,provider_reference,provider_job_reference,input_fingerprint,input_snapshot,COALESCE(result_snapshot,'null'::jsonb),estimated_distance,estimated_duration_seconds,result_version,idempotency_key,request_fingerprint FROM fleet_partner_route_optimization_requests WHERE organization_id=$1 AND route_plan_id=$2 AND route_version=$3 AND input_fingerprint=$4`, org, route, version, fingerprint).Scan(&o.ID, &o.OrganizationID, &o.RoutePlanID, &o.RouteVersion, &o.OptimizationVersion, &o.Status, &o.ProviderReference, &o.ProviderJobReference, &o.InputFingerprint, &o.InputSnapshot, &o.ResultSnapshot, &o.EstimatedDistance, &o.EstimatedDurationSeconds, &o.ResultVersion, &o.IdempotencyKey, &o.RequestFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return o, err
}
func (r *Repository) GetOptimization(ctx context.Context, q database.Querier, org, id string) (OptimizationRequest, error) {
	if q == nil {
		q = r.pool
	}
	var o OptimizationRequest
	err := q.QueryRow(ctx, `SELECT id,organization_id,route_plan_id,route_version,optimization_version,status,provider_reference,provider_job_reference,input_fingerprint,input_snapshot,COALESCE(result_snapshot,'null'::jsonb),estimated_distance,estimated_duration_seconds,result_version,idempotency_key,request_fingerprint FROM fleet_partner_route_optimization_requests WHERE organization_id=$1 AND id=$2`, org, id).Scan(&o.ID, &o.OrganizationID, &o.RoutePlanID, &o.RouteVersion, &o.OptimizationVersion, &o.Status, &o.ProviderReference, &o.ProviderJobReference, &o.InputFingerprint, &o.InputSnapshot, &o.ResultSnapshot, &o.EstimatedDistance, &o.EstimatedDurationSeconds, &o.ResultVersion, &o.IdempotencyKey, &o.RequestFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return o, err
}
func (r *Repository) CompleteOptimization(ctx context.Context, q database.Querier, org, id string, result OptimizationResult, status string) error {
	raw, _ := json.Marshal(result)
	violations, _ := json.Marshal(result.ConstraintViolations)
	unserved, _ := json.Marshal(result.UnservedStops)
	warnings, _ := json.Marshal(result.Warnings)
	tag, e := q.Exec(ctx, `UPDATE fleet_partner_route_optimization_requests SET status=$3,provider_reference=$4,provider_job_reference=$5,result_snapshot=$6,constraint_violations=$7,unserved_stops=$8,warnings=$9,estimated_distance=$10,estimated_duration_seconds=$11,result_version=result_version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, id, status, result.ProviderReference, result.ProviderJobReference, raw, violations, unserved, warnings, result.EstimatedDistance, result.EstimatedDurationSeconds)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return e
}

func (r *Repository) Tracking(ctx context.Context, org, trip string) (TrackingProjection, error) {
	var v TrackingProjection
	err := r.pool.QueryRow(ctx, `SELECT organization_id,source_service,canonical_trip_reference,assignment_id,COALESCE(route_plan_id,''),provider_id,vehicle_or_truck_reference,latitude,longitude,heading,speed,accuracy,operational_status,source_timestamp,received_at,source_version,freshness_status,gap_status,last_reconciled_at FROM fleet_partner_tracking_projections WHERE organization_id=$1 AND canonical_trip_reference=$2`, org, trip).Scan(&v.OrganizationID, &v.SourceService, &v.CanonicalTripReference, &v.AssignmentID, &v.RoutePlanID, &v.ProviderID, &v.VehicleOrTruckReference, &v.Latitude, &v.Longitude, &v.Heading, &v.Speed, &v.Accuracy, &v.OperationalStatus, &v.SourceTimestamp, &v.ReceivedAt, &v.SourceVersion, &v.FreshnessStatus, &v.GapStatus, &v.LastReconciledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return v, err
}
func (r *Repository) ListTracking(ctx context.Context, org string, f PageFilter) ([]TrackingProjection, error) {
	args := []any{org}
	where := `organization_id=$1`
	if f.ProviderID != "" {
		args = append(args, f.ProviderID)
		where += fmt.Sprintf(` AND provider_id=$%d`, len(args))
	}
	if f.VehicleReference != "" {
		args = append(args, f.VehicleReference)
		where += fmt.Sprintf(` AND vehicle_or_truck_reference=$%d`, len(args))
	}
	args = append(args, bounded(f.Limit), f.Offset)
	rows, e := r.pool.Query(ctx, `SELECT organization_id,source_service,canonical_trip_reference,assignment_id,COALESCE(route_plan_id,''),provider_id,vehicle_or_truck_reference,latitude,longitude,heading,speed,accuracy,operational_status,source_timestamp,received_at,source_version,freshness_status,gap_status,last_reconciled_at FROM fleet_partner_tracking_projections WHERE `+where+fmt.Sprintf(` ORDER BY received_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []TrackingProjection{}
	for rows.Next() {
		var v TrackingProjection
		if e = rows.Scan(&v.OrganizationID, &v.SourceService, &v.CanonicalTripReference, &v.AssignmentID, &v.RoutePlanID, &v.ProviderID, &v.VehicleOrTruckReference, &v.Latitude, &v.Longitude, &v.Heading, &v.Speed, &v.Accuracy, &v.OperationalStatus, &v.SourceTimestamp, &v.ReceivedAt, &v.SourceVersion, &v.FreshnessStatus, &v.GapStatus, &v.LastReconciledAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func bounded(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
