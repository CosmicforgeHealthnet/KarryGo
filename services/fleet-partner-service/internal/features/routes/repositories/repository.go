// Package repositories owns every Section 16 route-planning SQL statement:
// route plans, stops, optimization requests, publication snapshots and the
// route history. Methods that take a database.Querier join the caller's
// transaction; a nil Querier reads from the pool.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	dispatchmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
)

// Repository is the PostgreSQL route-planning store.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.pool
	}
	return q
}

func noRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return dispatchmodels.ErrNotFound
	}
	return err
}

const routeColumns = `id,organization_id,branch_id,COALESCE(dispatch_plan_id,''),COALESCE(replacement_for_route_id,''),route_reference,route_type,provider_membership_id,provider_id,vehicle_or_truck_reference,execution_service,status,planned_distance,distance_unit,planned_duration_seconds,optimization_reference,optimization_version,constraint_snapshot,COALESCE(approved_by,''),approved_at,published_at,version,created_at,updated_at`

func scanRoute(row pgx.Row) (models.RoutePlan, error) {
	var v models.RoutePlan
	var constraints []byte
	err := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.DispatchPlanID, &v.ReplacementForRouteID, &v.RouteReference, &v.RouteType, &v.ProviderMembershipID, &v.ProviderID, &v.VehicleOrTruckReference, &v.ExecutionService, &v.Status, &v.PlannedDistance, &v.DistanceUnit, &v.PlannedDurationSeconds, &v.OptimizationReference, &v.OptimizationVersion, &constraints, &v.ApprovedBy, &v.ApprovedAt, &v.PublishedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	v.ConstraintSnapshot = constraints
	return v, noRows(err)
}

// CreateRoute inserts a route plan, replaying the stored row for a matching
// idempotency key and fingerprint.
func (r *Repository) CreateRoute(ctx context.Context, q database.Querier, v models.RoutePlan, key, fp string) (models.RoutePlan, error) {
	return scanRoute(q.QueryRow(ctx, `INSERT INTO fleet_partner_route_plans(id,organization_id,branch_id,dispatch_plan_id,replacement_for_route_id,route_reference,route_type,provider_membership_id,provider_id,vehicle_or_truck_reference,execution_service,status,constraint_snapshot,idempotency_key,request_fingerprint) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT(organization_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE fleet_partner_route_plans.request_fingerprint=EXCLUDED.request_fingerprint RETURNING `+routeColumns, v.ID, v.OrganizationID, v.BranchID, v.DispatchPlanID, v.ReplacementForRouteID, v.RouteReference, v.RouteType, v.ProviderMembershipID, v.ProviderID, v.VehicleOrTruckReference, v.ExecutionService, v.Status, v.ConstraintSnapshot, key, fp))
}

// GetRoute reads (and optionally locks) one route plan.
func (r *Repository) GetRoute(ctx context.Context, q database.Querier, org, id string, lock bool) (models.RoutePlan, error) {
	s := `SELECT ` + routeColumns + ` FROM fleet_partner_route_plans WHERE organization_id=$1 AND id=$2`
	if lock {
		s += ` FOR UPDATE`
	}
	return scanRoute(r.q(q).QueryRow(ctx, s, org, id))
}

// ListRoutes reads up to limit route plans, newest first with id ascending as
// the tie-breaker. It pages by f.Offset, or by the keyset position
// f.After/f.AfterID when set.
func (r *Repository) ListRoutes(ctx context.Context, org string, f dispatchmodels.PageFilter, limit int) ([]models.RoutePlan, error) {
	args := []any{org}
	where := ` organization_id=$1`
	for _, x := range []struct{ v, c string }{{f.BranchID, "branch_id"}, {f.Status, "status"}, {f.Service, "execution_service"}, {f.ProviderID, "provider_id"}, {f.VehicleReference, "vehicle_or_truck_reference"}} {
		if x.v != "" {
			args = append(args, x.v)
			where += fmt.Sprintf(` AND %s=$%d`, x.c, len(args))
		}
	}
	offset := pagination.ClampOffset(f.Offset)
	if f.After != nil {
		args = append(args, *f.After, f.AfterID)
		where += fmt.Sprintf(` AND (created_at<$%d OR (created_at=$%d AND id>$%d))`, len(args)-1, len(args)-1, len(args))
		offset = 0
	}
	args = append(args, limit, offset)
	rows, e := r.pool.Query(ctx, `SELECT `+routeColumns+` FROM fleet_partner_route_plans WHERE`+where+fmt.Sprintf(` ORDER BY created_at DESC,id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.RoutePlan{}
	for rows.Next() {
		v, e := scanRoute(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateRoute applies a versioned route change. Empty strings, a nil
// snapshot and negative numbers keep the stored value. A stale version is
// dispatchmodels.ErrConflict.
func (r *Repository) UpdateRoute(ctx context.Context, q database.Querier, org, id string, expected int64, status, providerMembership, provider, vehicle string, constraints json.RawMessage, optimizationRef string, optimizationVersion int64, distance float64, duration int, actor string) (models.RoutePlan, error) {
	if len(constraints) == 0 {
		constraints = nil
	}
	v, e := scanRoute(q.QueryRow(ctx, `UPDATE fleet_partner_route_plans SET status=COALESCE(NULLIF($4,''),status),provider_membership_id=COALESCE(NULLIF($5,''),provider_membership_id),provider_id=COALESCE(NULLIF($6,''),provider_id),vehicle_or_truck_reference=COALESCE(NULLIF($7,''),vehicle_or_truck_reference),constraint_snapshot=COALESCE($8,constraint_snapshot),optimization_reference=COALESCE(NULLIF($9,''),optimization_reference),optimization_version=CASE WHEN $10>0 THEN $10 ELSE optimization_version END,planned_distance=CASE WHEN $11>=0 THEN $11 ELSE planned_distance END,planned_duration_seconds=CASE WHEN $12>=0 THEN $12 ELSE planned_duration_seconds END,approved_by=CASE WHEN $4='APPROVED' THEN $13 ELSE approved_by END,approved_at=CASE WHEN $4='APPROVED' THEN now() ELSE approved_at END,published_at=CASE WHEN $4='PUBLISHED' THEN now() ELSE published_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+routeColumns, org, id, expected, status, providerMembership, provider, vehicle, constraints, optimizationRef, optimizationVersion, distance, duration, actor))
	if e == dispatchmodels.ErrNotFound {
		return v, dispatchmodels.ErrConflict
	}
	return v, e
}

const stopColumns = `id,organization_id,route_plan_id,sequence,stop_type,COALESCE(order_id,''),COALESCE(shipment_id,''),COALESCE(package_id,''),COALESCE(manifest_id,''),COALESCE(source_order_stop_id,''),location_reference,latitude,longitude,time_window_start,time_window_end,service_duration_seconds,planned_arrival_at,planned_departure_at,actual_arrival_at,actual_departure_at,status,version,created_at,updated_at`

func scanStop(row pgx.Row) (models.RouteStop, error) {
	var v models.RouteStop
	err := row.Scan(&v.ID, &v.OrganizationID, &v.RoutePlanID, &v.Sequence, &v.StopType, &v.OrderID, &v.ShipmentID, &v.PackageID, &v.ManifestID, &v.SourceOrderStopID, &v.LocationReference, &v.Latitude, &v.Longitude, &v.TimeWindowStart, &v.TimeWindowEnd, &v.ServiceDurationSeconds, &v.PlannedArrivalAt, &v.PlannedDepartureAt, &v.ActualArrivalAt, &v.ActualDepartureAt, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, noRows(err)
}

// CreateStop inserts one stop.
func (r *Repository) CreateStop(ctx context.Context, q database.Querier, v models.RouteStop) (models.RouteStop, error) {
	return scanStop(q.QueryRow(ctx, `INSERT INTO fleet_partner_route_stops(id,organization_id,route_plan_id,sequence,stop_type,order_id,shipment_id,package_id,manifest_id,source_order_stop_id,location_reference,latitude,longitude,time_window_start,time_window_end,service_duration_seconds,planned_arrival_at,planned_departure_at,status) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING `+stopColumns, v.ID, v.OrganizationID, v.RoutePlanID, v.Sequence, v.StopType, v.OrderID, v.ShipmentID, v.PackageID, v.ManifestID, v.SourceOrderStopID, v.LocationReference, v.Latitude, v.Longitude, v.TimeWindowStart, v.TimeWindowEnd, v.ServiceDurationSeconds, v.PlannedArrivalAt, v.PlannedDepartureAt, v.Status))
}

// ListStops reads a route's stops in sequence.
func (r *Repository) ListStops(ctx context.Context, q database.Querier, org, route string) ([]models.RouteStop, error) {
	rows, e := r.q(q).Query(ctx, `SELECT `+stopColumns+` FROM fleet_partner_route_stops WHERE organization_id=$1 AND route_plan_id=$2 ORDER BY sequence`, org, route)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.RouteStop{}
	for rows.Next() {
		v, e := scanStop(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateStop edits a PLANNED stop at the expected version. A stale version
// is dispatchmodels.ErrConflict.
func (r *Repository) UpdateStop(ctx context.Context, q database.Querier, org, route, id string, expected int64, v models.RouteStop) (models.RouteStop, error) {
	out, e := scanStop(q.QueryRow(ctx, `UPDATE fleet_partner_route_stops SET stop_type=COALESCE(NULLIF($6,''),stop_type),location_reference=COALESCE(NULLIF($7,''),location_reference),time_window_start=COALESCE($8,time_window_start),time_window_end=COALESCE($9,time_window_end),service_duration_seconds=CASE WHEN $10>=0 THEN $10 ELSE service_duration_seconds END,planned_arrival_at=COALESCE($11,planned_arrival_at),planned_departure_at=COALESCE($12,planned_departure_at),version=version+1,updated_at=now() WHERE organization_id=$1 AND route_plan_id=$2 AND id=$3 AND version=$4 AND status=$5 RETURNING `+stopColumns, org, route, id, expected, models.StopPlanned, v.StopType, v.LocationReference, v.TimeWindowStart, v.TimeWindowEnd, v.ServiceDurationSeconds, v.PlannedArrivalAt, v.PlannedDepartureAt))
	if e == dispatchmodels.ErrNotFound {
		return out, dispatchmodels.ErrConflict
	}
	return out, e
}

// DeleteStop removes a PLANNED stop.
func (r *Repository) DeleteStop(ctx context.Context, q database.Querier, org, route, id string) error {
	tag, e := q.Exec(ctx, `DELETE FROM fleet_partner_route_stops WHERE organization_id=$1 AND route_plan_id=$2 AND id=$3 AND status=$4`, org, route, id, models.StopPlanned)
	if e == nil && tag.RowsAffected() == 0 {
		return dispatchmodels.ErrNotFound
	}
	return e
}

// NormalizeStops renumbers a route's stops 1..n in their current order.
func (r *Repository) NormalizeStops(ctx context.Context, q database.Querier, org, route string) error {
	_, e := q.Exec(ctx, `WITH ranked AS(SELECT id,row_number() OVER(ORDER BY sequence,id) AS n FROM fleet_partner_route_stops WHERE organization_id=$1 AND route_plan_id=$2) UPDATE fleet_partner_route_stops s SET sequence=100000+r.n FROM ranked r WHERE s.id=r.id`, org, route)
	if e != nil {
		return e
	}
	_, e = q.Exec(ctx, `UPDATE fleet_partner_route_stops SET sequence=sequence-100000 WHERE organization_id=$1 AND route_plan_id=$2`, org, route)
	return e
}

// SetStopStatus moves one stop to status at the expected version, optionally
// only from requiredStatus, and returns its new version.
func (r *Repository) SetStopStatus(ctx context.Context, q database.Querier, org, route, id string, expected int64, status, requiredStatus string) (int64, error) {
	var version int64
	var e error
	if requiredStatus == "" {
		e = q.QueryRow(ctx, `UPDATE fleet_partner_route_stops SET status=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND route_plan_id=$2 AND id=$3 AND version=$4 RETURNING version`, org, route, id, expected, status).Scan(&version)
	} else {
		e = q.QueryRow(ctx, `UPDATE fleet_partner_route_stops SET status=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND route_plan_id=$2 AND id=$3 AND version=$4 AND status=$6 RETURNING version`, org, route, id, expected, status, requiredStatus).Scan(&version)
	}
	return version, e
}

// History appends one route, stop or plan transition.
func (r *Repository) History(ctx context.Context, q database.Querier, org, kind, id, from, to, reason, actor string, version int64, snapshot any) error {
	raw, _ := json.Marshal(snapshot)
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_dispatch_route_history(id,organization_id,aggregate_type,aggregate_id,previous_status,new_status,reason_code,actor_id,version,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("drh"), org, kind, id, from, to, reason, actor, version, raw)
	return e
}

// InsertPublicationSnapshot stores the immutable record of a published
// route version.
func (r *Repository) InsertPublicationSnapshot(ctx context.Context, q database.Querier, org, route string, version int64, assignmentIDs []string, overrideReason string, snapshot any, actor string) error {
	raw, _ := json.Marshal(snapshot)
	assignments, _ := json.Marshal(assignmentIDs)
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_route_publication_snapshots(id,organization_id,route_plan_id,route_version,assignment_ids,override_reason,snapshot,published_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(route_plan_id,route_version) DO NOTHING`, ids.New("rps"), org, route, version, assignments, overrideReason, raw, actor)
	return e
}

// LatestStopAssignmentStatus reads the status of the newest private
// assignment for a route stop.
func (r *Repository) LatestStopAssignmentStatus(ctx context.Context, q database.Querier, org, stopID string) (string, error) {
	var status string
	x := q.QueryRow(ctx, `SELECT status FROM fleet_partner_private_assignments WHERE organization_id=$1 AND route_stop_id=$2 ORDER BY version DESC,updated_at DESC LIMIT 1`, org, stopID).Scan(&status)
	return status, noRows(x)
}

// DueProgressRoutes reads up to 200 active routes with assignment state and
// the highest assignment version, so each change schedules one progress job.
func (r *Repository) DueProgressRoutes(ctx context.Context, q database.Querier) ([]models.RouteProgress, error) {
	rows, e := q.Query(ctx, `SELECT rp.organization_id,rp.id,max(pa.version)
FROM fleet_partner_route_plans rp JOIN fleet_partner_route_stops rs ON rs.route_plan_id=rp.id JOIN fleet_partner_private_assignments pa ON pa.organization_id=rp.organization_id AND pa.route_stop_id=rs.id
WHERE rp.status IN('PUBLISHED','IN_PROGRESS') GROUP BY rp.organization_id,rp.id LIMIT 200`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	due := []models.RouteProgress{}
	for rows.Next() {
		var v models.RouteProgress
		if e = rows.Scan(&v.OrganizationID, &v.RoutePlanID, &v.AssignmentVersion); e != nil {
			return nil, e
		}
		due = append(due, v)
	}
	return due, rows.Err()
}

const optimizationColumns = `id,organization_id,route_plan_id,route_version,optimization_version,status,provider_reference,provider_job_reference,input_fingerprint,input_snapshot,COALESCE(result_snapshot,'null'::jsonb),estimated_distance,estimated_duration_seconds,result_version,idempotency_key,request_fingerprint`

func scanOptimization(row pgx.Row) (models.OptimizationRequest, error) {
	var o models.OptimizationRequest
	err := row.Scan(&o.ID, &o.OrganizationID, &o.RoutePlanID, &o.RouteVersion, &o.OptimizationVersion, &o.Status, &o.ProviderReference, &o.ProviderJobReference, &o.InputFingerprint, &o.InputSnapshot, &o.ResultSnapshot, &o.EstimatedDistance, &o.EstimatedDurationSeconds, &o.ResultVersion, &o.IdempotencyKey, &o.RequestFingerprint)
	return o, noRows(err)
}

// CreateOptimization records an optimization request once per key.
func (r *Repository) CreateOptimization(ctx context.Context, q database.Querier, o models.OptimizationRequest) error {
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_route_optimization_requests(id,organization_id,route_plan_id,route_version,optimization_version,status,input_fingerprint,input_snapshot,idempotency_key,request_fingerprint,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(organization_id,idempotency_key) DO NOTHING`, o.ID, o.OrganizationID, o.RoutePlanID, o.RouteVersion, o.OptimizationVersion, o.Status, o.InputFingerprint, o.InputSnapshot, o.IdempotencyKey, o.RequestFingerprint, "system")
	return e
}

// FindOptimization reads the request for a route version and input.
func (r *Repository) FindOptimization(ctx context.Context, q database.Querier, org, route string, version int64, fingerprint string) (models.OptimizationRequest, error) {
	return scanOptimization(r.q(q).QueryRow(ctx, `SELECT `+optimizationColumns+` FROM fleet_partner_route_optimization_requests WHERE organization_id=$1 AND route_plan_id=$2 AND route_version=$3 AND input_fingerprint=$4`, org, route, version, fingerprint))
}

// GetOptimization reads one optimization request.
func (r *Repository) GetOptimization(ctx context.Context, q database.Querier, org, id string) (models.OptimizationRequest, error) {
	return scanOptimization(r.q(q).QueryRow(ctx, `SELECT `+optimizationColumns+` FROM fleet_partner_route_optimization_requests WHERE organization_id=$1 AND id=$2`, org, id))
}

// MarkOptimizationSubmitted records the provider job of a submitted request.
func (r *Repository) MarkOptimizationSubmitted(ctx context.Context, q database.Querier, org, id, status, providerJobReference string) error {
	_, e := q.Exec(ctx, `UPDATE fleet_partner_route_optimization_requests SET status=$3,provider_job_reference=$4,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status IN('PENDING','RUNNING','RECONCILIATION_REQUIRED')`, org, id, status, providerJobReference)
	return e
}

// CompleteOptimization stores the provider result of a request.
func (r *Repository) CompleteOptimization(ctx context.Context, q database.Querier, org, id string, result models.OptimizationResult, status string) error {
	raw, _ := json.Marshal(result)
	violations, _ := json.Marshal(result.ConstraintViolations)
	unserved, _ := json.Marshal(result.UnservedStops)
	warnings, _ := json.Marshal(result.Warnings)
	tag, e := q.Exec(ctx, `UPDATE fleet_partner_route_optimization_requests SET status=$3,provider_reference=$4,provider_job_reference=$5,result_snapshot=$6,constraint_violations=$7,unserved_stops=$8,warnings=$9,estimated_distance=$10,estimated_duration_seconds=$11,result_version=result_version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, id, status, result.ProviderReference, result.ProviderJobReference, raw, violations, unserved, warnings, result.EstimatedDistance, result.EstimatedDurationSeconds)
	if e == nil && tag.RowsAffected() == 0 {
		return dispatchmodels.ErrNotFound
	}
	return e
}
