// Package repositories owns every Section 16 dispatch SQL statement:
// dispatch plans, assignment candidates, control-tower summaries,
// geofences and their events, operational alerts and rescue cases. Methods
// that take a database.Querier join the caller's transaction; a nil
// Querier reads from the pool.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
)

// Repository is the PostgreSQL dispatch store.
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
		return models.ErrNotFound
	}
	return err
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

const planColumns = `id,organization_id,branch_id,plan_date,work_window_start,work_window_end,timezone,status,planner_user_id,COALESCE(approved_by,''),approved_at,published_at,version,created_at,updated_at`

func scanPlan(row pgx.Row) (models.DispatchPlan, error) {
	var v models.DispatchPlan
	err := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.PlanDate, &v.WorkWindowStart, &v.WorkWindowEnd, &v.Timezone, &v.Status, &v.PlannerUserID, &v.ApprovedBy, &v.ApprovedAt, &v.PublishedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, noRows(err)
}

// CreatePlan inserts a dispatch plan, replaying the stored row for a
// matching idempotency key and fingerprint.
func (r *Repository) CreatePlan(ctx context.Context, q database.Querier, p models.DispatchPlan, key, fp string) (models.DispatchPlan, error) {
	return scanPlan(q.QueryRow(ctx, `INSERT INTO fleet_partner_dispatch_plans(id,organization_id,branch_id,plan_date,work_window_start,work_window_end,timezone,status,planner_user_id,idempotency_key,request_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(organization_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE fleet_partner_dispatch_plans.request_fingerprint=EXCLUDED.request_fingerprint RETURNING `+planColumns, p.ID, p.OrganizationID, p.BranchID, p.PlanDate, p.WorkWindowStart, p.WorkWindowEnd, p.Timezone, p.Status, p.PlannerUserID, key, fp))
}

// GetPlan reads (and optionally locks) one dispatch plan.
func (r *Repository) GetPlan(ctx context.Context, q database.Querier, org, id string, lock bool) (models.DispatchPlan, error) {
	s := `SELECT ` + planColumns + ` FROM fleet_partner_dispatch_plans WHERE organization_id=$1 AND id=$2`
	if lock {
		s += ` FOR UPDATE`
	}
	return scanPlan(r.q(q).QueryRow(ctx, s, org, id))
}

// ListPlans reads up to limit plans, latest plan date first with id
// ascending as the tie-breaker. It pages by f.Offset, or by the keyset
// position f.After (a plan date)/f.AfterID when set.
func (r *Repository) ListPlans(ctx context.Context, org string, f models.PageFilter, limit int) ([]models.DispatchPlan, error) {
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
	offset := pagination.ClampOffset(f.Offset)
	if f.After != nil {
		args = append(args, f.After.UTC().Format("2006-01-02"), f.AfterID)
		where += fmt.Sprintf(` AND (plan_date<$%d::date OR (plan_date=$%d::date AND id>$%d))`, len(args)-1, len(args)-1, len(args))
		offset = 0
	}
	args = append(args, limit, offset)
	rows, e := r.pool.Query(ctx, `SELECT `+planColumns+` FROM fleet_partner_dispatch_plans WHERE`+where+fmt.Sprintf(` ORDER BY plan_date DESC,id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.DispatchPlan{}
	for rows.Next() {
		v, e := scanPlan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdatePlan applies a versioned plan change. A stale version is
// models.ErrConflict.
func (r *Repository) UpdatePlan(ctx context.Context, q database.Querier, org, id string, expected int64, status, timezone string, start, end time.Time, approver string) (models.DispatchPlan, error) {
	v, e := scanPlan(q.QueryRow(ctx, `UPDATE fleet_partner_dispatch_plans SET status=COALESCE(NULLIF($4,''),status),timezone=COALESCE(NULLIF($5,''),timezone),work_window_start=CASE WHEN $6::timestamptz IS NULL THEN work_window_start ELSE $6 END,work_window_end=CASE WHEN $7::timestamptz IS NULL THEN work_window_end ELSE $7 END,approved_by=CASE WHEN $4='APPROVED' THEN $8 ELSE approved_by END,approved_at=CASE WHEN $4='APPROVED' THEN now() ELSE approved_at END,published_at=CASE WHEN $4='PUBLISHED' THEN now() ELSE published_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+planColumns, org, id, expected, status, timezone, nullableTime(start), nullableTime(end), approver))
	if e == models.ErrNotFound {
		return v, models.ErrConflict
	}
	return v, e
}

// PutCandidate persists one scored candidate once per input fingerprint.
func (r *Repository) PutCandidate(ctx context.Context, q database.Querier, c models.Candidate) error {
	components, _ := json.Marshal(c.ScoreComponents)
	reasons, _ := json.Marshal(c.EligibilityReasons)
	warnings, _ := json.Marshal(c.Warnings)
	blocks, _ := json.Marshal(c.HardBlocks)
	versions, _ := json.Marshal(c.SourceVersions)
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_assignment_candidates(id,organization_id,branch_id,source_type,source_id,provider_id,provider_membership_id,vehicle_or_truck_reference,execution_service,eligibility_status,total_score,score_components,eligibility_reasons,warnings,hard_blocks,data_freshness,generated_at,expires_at,source_versions,input_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) ON CONFLICT(organization_id,source_type,source_id,provider_membership_id,vehicle_or_truck_reference,input_fingerprint) DO NOTHING`, c.ID, c.OrganizationID, c.BranchID, c.SourceType, c.SourceID, c.ProviderID, c.ProviderMembershipID, c.VehicleOrTruckReference, c.ExecutionService, c.EligibilityStatus, c.TotalScore, components, reasons, warnings, blocks, c.DataFreshness, c.GeneratedAt, c.ExpiresAt, versions, c.InputFingerprint)
	return e
}

// ActiveCandidates reads up to 200 unexpired candidate projections, newest
// first, optionally for one branch.
func (r *Repository) ActiveCandidates(ctx context.Context, org, branch string) ([]models.CandidateAvailability, error) {
	rows, e := r.pool.Query(ctx, `SELECT provider_id,vehicle_or_truck_reference,eligibility_status,generated_at,expires_at,branch_id
FROM fleet_partner_assignment_candidates WHERE organization_id=$1 AND ($2='' OR branch_id=$2) AND expires_at>now()
ORDER BY generated_at DESC LIMIT 200`, org, branch)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.CandidateAvailability{}
	for rows.Next() {
		var v models.CandidateAvailability
		if e = rows.Scan(&v.ProviderID, &v.VehicleReference, &v.Eligibility, &v.GeneratedAt, &v.ExpiresAt, &v.BranchID); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UnassignedWork reads up to 200 orders, shipments and routes that need a
// dispatcher, optionally for one branch.
func (r *Repository) UnassignedWork(ctx context.Context, org, branch string) ([]map[string]any, error) {
	args := []any{org}
	branchSQL := ""
	if branch != "" {
		args = append(args, branch)
		branchSQL = " AND w.branch_id=$2"
	}
	rows, e := r.pool.Query(ctx, `WITH work AS(
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
		out = append(out, map[string]any{"id": id, "sourceType": kind, "branchId": b, "status": status, "actionReason": reason})
	}
	return out, rows.Err()
}

// AssignmentsByStatus reads up to 200 private assignments in the given
// comma-separated statuses, newest first.
func (r *Repository) AssignmentsByStatus(ctx context.Context, org, statuses string) ([]models.AssignmentSummary, error) {
	rows, e := r.pool.Query(ctx, `SELECT id,branch_id,work_origin,status,provider_id,vehicle_or_truck_reference,execution_service,response_deadline FROM fleet_partner_private_assignments WHERE organization_id=$1 AND status=ANY(string_to_array($2,',')) ORDER BY created_at DESC LIMIT 200`, org, statuses)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.AssignmentSummary{}
	for rows.Next() {
		var v models.AssignmentSummary
		if e = rows.Scan(&v.ID, &v.BranchID, &v.WorkOrigin, &v.Status, &v.ProviderID, &v.VehicleReference, &v.ExecutionService, &v.ResponseDeadline); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// RouteStopAssignmentConditions reads up to 200 route-stop assignments a
// provider declined or let expire on an active route.
func (r *Repository) RouteStopAssignmentConditions(ctx context.Context, q database.Querier) ([]models.AssignmentCondition, error) {
	rows, e := q.Query(ctx, `SELECT pa.organization_id,rp.branch_id,rp.id,pa.id,pa.status,pa.version
FROM fleet_partner_private_assignments pa JOIN fleet_partner_route_stops rs ON rs.id=pa.route_stop_id JOIN fleet_partner_route_plans rp ON rp.id=rs.route_plan_id AND rp.organization_id=pa.organization_id
WHERE pa.work_origin='ROUTE_STOP' AND pa.status IN('DECLINED','EXPIRED','REASSIGNMENT_REQUIRED') AND rp.status IN('PUBLISHED','IN_PROGRESS','FAILED') ORDER BY pa.updated_at DESC LIMIT 200`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	conditions := []models.AssignmentCondition{}
	for rows.Next() {
		var v models.AssignmentCondition
		if e = rows.Scan(&v.OrganizationID, &v.BranchID, &v.RoutePlanID, &v.AssignmentID, &v.Status, &v.Version); e != nil {
			return nil, e
		}
		conditions = append(conditions, v)
	}
	return conditions, rows.Err()
}

const geofenceColumns = `id,organization_id,COALESCE(branch_id,''),name,geometry_type,geometry,geometry_reference,event_rules,active_status,version,created_by,created_at,updated_at`

func scanGeofence(row pgx.Row) (models.Geofence, error) {
	var g models.Geofence
	var geom, rules []byte
	e := row.Scan(&g.ID, &g.OrganizationID, &g.BranchID, &g.Name, &g.GeometryType, &geom, &g.GeometryReference, &rules, &g.ActiveStatus, &g.Version, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt)
	e = noRows(e)
	_ = json.Unmarshal(geom, &g.Geometry)
	_ = json.Unmarshal(rules, &g.EventRules)
	return g, e
}

// CreateGeofence inserts an INACTIVE geofence, replaying the stored row for
// a matching idempotency key and fingerprint.
func (r *Repository) CreateGeofence(ctx context.Context, q database.Querier, org string, in models.CreateGeofenceInput, actor, fp string) (models.Geofence, error) {
	raw, _ := json.Marshal(in.Geometry)
	rules, _ := json.Marshal(in.EventRules)
	return scanGeofence(q.QueryRow(ctx, `INSERT INTO fleet_partner_geofences(id,organization_id,branch_id,name,geometry_type,geometry,geometry_reference,event_rules,active_status,created_by,idempotency_key,request_fingerprint) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,'INACTIVE',$9,$10,$11) ON CONFLICT(organization_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE fleet_partner_geofences.request_fingerprint=EXCLUDED.request_fingerprint RETURNING `+geofenceColumns, ids.New("geo"), org, in.BranchID, in.Name, in.GeometryType, raw, in.GeometryReference, rules, actor, in.IdempotencyKey, fp))
}

// GetGeofence reads (and optionally locks) one geofence.
func (r *Repository) GetGeofence(ctx context.Context, q database.Querier, org, id string, lock bool) (models.Geofence, error) {
	s := `SELECT ` + geofenceColumns + ` FROM fleet_partner_geofences WHERE organization_id=$1 AND id=$2`
	if lock {
		s += ` FOR UPDATE`
	}
	return scanGeofence(r.q(q).QueryRow(ctx, s, org, id))
}

func collectGeofences(rows pgx.Rows, err error) ([]models.Geofence, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Geofence{}
	for rows.Next() {
		g, e := scanGeofence(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ListGeofences reads up to limit geofences, newest first with id ascending
// as the tie-breaker. It pages by f.Offset, or by the keyset position
// f.After/f.AfterID when set.
func (r *Repository) ListGeofences(ctx context.Context, org string, f models.PageFilter, limit int) ([]models.Geofence, error) {
	if f.After != nil {
		return collectGeofences(r.pool.Query(ctx, `SELECT `+geofenceColumns+` FROM fleet_partner_geofences WHERE organization_id=$1 AND ($2='' OR branch_id=$2) AND (created_at<$4 OR (created_at=$4 AND id>$5)) ORDER BY created_at DESC,id LIMIT $3`, org, f.BranchID, limit, *f.After, f.AfterID))
	}
	return collectGeofences(r.pool.Query(ctx, `SELECT `+geofenceColumns+` FROM fleet_partner_geofences WHERE organization_id=$1 AND ($2='' OR branch_id=$2) ORDER BY created_at DESC,id LIMIT $3 OFFSET $4`, org, f.BranchID, limit, pagination.ClampOffset(f.Offset)))
}

// ActiveGeofences reads every ACTIVE geofence of the organization.
func (r *Repository) ActiveGeofences(ctx context.Context, org string) ([]models.Geofence, error) {
	return collectGeofences(r.pool.Query(ctx, `SELECT `+geofenceColumns+` FROM fleet_partner_geofences WHERE organization_id=$1 AND active_status='ACTIVE'`, org))
}

// UpdateGeofence edits a geofence at the expected version. A stale version
// is models.ErrNotFound.
func (r *Repository) UpdateGeofence(ctx context.Context, q database.Querier, org, id string, expected int64, in models.CreateGeofenceInput) (models.Geofence, error) {
	raw, _ := json.Marshal(in.Geometry)
	rules, _ := json.Marshal(in.EventRules)
	return scanGeofence(q.QueryRow(ctx, `UPDATE fleet_partner_geofences SET name=COALESCE(NULLIF($4,''),name),geometry_type=$5,geometry=$6,geometry_reference=$7,event_rules=$8,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+geofenceColumns, org, id, expected, in.Name, in.GeometryType, raw, in.GeometryReference, rules))
}

// SetGeofenceStatus switches a geofence at the expected version. A stale
// version is models.ErrNotFound.
func (r *Repository) SetGeofenceStatus(ctx context.Context, q database.Querier, org, id string, expected int64, status string) (models.Geofence, error) {
	return scanGeofence(q.QueryRow(ctx, `UPDATE fleet_partner_geofences SET active_status=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+geofenceColumns, org, id, expected, status))
}

// GeofenceTransitionState reads the latest recorded event per rule for one
// geofence and trip.
func (r *Repository) GeofenceTransitionState(ctx context.Context, q database.Querier, org, geofenceID, trip string) (models.GeofenceTransitionState, error) {
	var s models.GeofenceTransitionState
	e := q.QueryRow(ctx, `SELECT
COALESCE(max(source_version) FILTER(WHERE event_rule='ENTER'),0),
COALESCE(max(source_version) FILTER(WHERE event_rule='EXIT'),0),
COALESCE(max(source_version) FILTER(WHERE event_rule='DWELL'),0),
COALESCE(max(source_version) FILTER(WHERE event_rule='ROUTE_DEVIATION'),0),
COALESCE(max(source_version) FILTER(WHERE event_rule='RESTRICTED_AREA'),0),
max(occurred_at) FILTER(WHERE event_rule='ENTER')
FROM fleet_partner_geofence_events WHERE organization_id=$1 AND geofence_id=$2 AND canonical_trip_reference=$3`, org, geofenceID, trip).Scan(&s.LastEnter, &s.LastExit, &s.LastDwell, &s.LastDeviation, &s.LastRestricted, &s.EnteredAt)
	return s, e
}

// InsertGeofenceEvent records one transition once; inserted reports whether
// it was new.
func (r *Repository) InsertGeofenceEvent(ctx context.Context, q database.Querier, org, geofenceID, trip, rule string, sourceVersion int64, occurredAt time.Time) (bool, error) {
	tag, e := q.Exec(ctx, `INSERT INTO fleet_partner_geofence_events(id,organization_id,geofence_id,canonical_trip_reference,event_rule,source_version,confidence,occurred_at) VALUES($1,$2,$3,$4,$5,$6,'HIGH',$7) ON CONFLICT DO NOTHING`, ids.New("gfe"), org, geofenceID, trip, rule, sourceVersion, occurredAt)
	if e != nil {
		return false, e
	}
	return tag.RowsAffected() > 0, nil
}

// ListGeofenceEvents reads up to limit geofence events, latest first with
// id ascending as the tie-breaker. It pages by f.Offset, or by the keyset
// position f.After/f.AfterID when set.
func (r *Repository) ListGeofenceEvents(ctx context.Context, org string, f models.GeofenceEventFilter, limit int) ([]models.GeofenceEvent, error) {
	offset := pagination.ClampOffset(f.Offset)
	var after any
	if f.After != nil {
		after, offset = *f.After, 0
	}
	rows, e := r.pool.Query(ctx, `SELECT e.id,e.organization_id,e.geofence_id,g.name,COALESCE(g.branch_id,''),e.canonical_trip_reference,e.event_rule,e.source_version,e.confidence,e.occurred_at,e.created_at
FROM fleet_partner_geofence_events e JOIN fleet_partner_geofences g ON g.organization_id=e.organization_id AND g.id=e.geofence_id
WHERE e.organization_id=$1 AND ($2='' OR e.geofence_id=$2) AND ($3='' OR e.canonical_trip_reference=$3) AND ($4='' OR e.event_rule=$4) AND ($5='' OR g.branch_id=$5)
AND ($6::timestamptz IS NULL OR e.occurred_at>=$6) AND ($7::timestamptz IS NULL OR e.occurred_at<$7)
AND ($10::timestamptz IS NULL OR e.occurred_at<$10 OR (e.occurred_at=$10 AND e.id>$11))
ORDER BY e.occurred_at DESC,e.id LIMIT $8 OFFSET $9`, org, f.GeofenceID, f.CanonicalTripReference, f.EventRule, f.BranchID, f.From, f.To, limit, offset, after, f.AfterID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.GeofenceEvent{}
	for rows.Next() {
		var v models.GeofenceEvent
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.GeofenceID, &v.GeofenceName, &v.BranchID, &v.CanonicalTripReference, &v.EventRule, &v.SourceVersion, &v.Confidence, &v.OccurredAt, &v.RecordedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const alertColumns = `id,organization_id,COALESCE(branch_id,''),alert_type,severity,source_type,source_id,COALESCE(route_plan_id,''),assignment_id,shipment_id,canonical_trip_reference,owner_user_id,status,sla_due_at,acknowledged_at,resolved_at,resolution_code,resolution_reason,version,created_at,updated_at`

func alertTargets(a *models.OperationalAlert) []any {
	return []any{&a.ID, &a.OrganizationID, &a.BranchID, &a.AlertType, &a.Severity, &a.SourceType, &a.SourceID, &a.RoutePlanID, &a.AssignmentID, &a.ShipmentID, &a.CanonicalTripReference, &a.OwnerUserID, &a.Status, &a.SLADueAt, &a.AcknowledgedAt, &a.ResolvedAt, &a.ResolutionCode, &a.ResolutionReason, &a.Version, &a.CreatedAt, &a.UpdatedAt}
}

func scanAlert(row pgx.Row) (models.OperationalAlert, error) {
	var a models.OperationalAlert
	return a, noRows(row.Scan(alertTargets(&a)...))
}

// UpsertAlert raises an alert for a condition. While an alert for the same
// condition is open it is returned unchanged; inserted reports whether a new
// alert was created.
func (r *Repository) UpsertAlert(ctx context.Context, q database.Querier, org string, req models.AlertRequest) (models.OperationalAlert, bool, error) {
	var a models.OperationalAlert
	var inserted bool
	e := q.QueryRow(ctx, `INSERT INTO fleet_partner_operational_alerts(id,organization_id,branch_id,alert_type,severity,source_type,source_id,route_plan_id,assignment_id,canonical_trip_reference,condition_fingerprint,sla_due_at) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12) ON CONFLICT(organization_id,condition_fingerprint) WHERE status IN('OPEN','ACKNOWLEDGED','IN_PROGRESS') DO UPDATE SET updated_at=fleet_partner_operational_alerts.updated_at RETURNING `+alertColumns+`,(xmax=0)`, ids.New("alt"), org, req.BranchID, req.Kind, req.Severity, req.SourceType, req.SourceID, req.RoutePlanID, req.AssignmentID, req.CanonicalTripReference, req.Condition, req.SLADueAt).Scan(append(alertTargets(&a), &inserted)...)
	return a, inserted, e
}

// AlertConditionRaised reports whether an alert was ever raised for the
// condition, open or closed.
func (r *Repository) AlertConditionRaised(ctx context.Context, q database.Querier, org, condition string) (bool, error) {
	var exists bool
	e := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND condition_fingerprint=$2)`, org, condition).Scan(&exists)
	return exists, e
}

// GetAlert reads (and optionally locks) one alert.
func (r *Repository) GetAlert(ctx context.Context, q database.Querier, org, id string, lock bool) (models.OperationalAlert, error) {
	s := `SELECT ` + alertColumns + ` FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND id=$2`
	if lock {
		s += ` FOR UPDATE`
	}
	return scanAlert(r.q(q).QueryRow(ctx, s, org, id))
}

// ListAlerts reads up to limit alerts, newest first with id ascending as the
// tie-breaker. It pages by f.Offset, or by the keyset position
// f.After/f.AfterID when set.
func (r *Repository) ListAlerts(ctx context.Context, org string, f models.PageFilter, limit int) ([]models.OperationalAlert, error) {
	offset := pagination.ClampOffset(f.Offset)
	var after any
	if f.After != nil {
		after, offset = *f.After, 0
	}
	rows, e := r.pool.Query(ctx, `SELECT `+alertColumns+` FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND ($2='' OR branch_id=$2) AND ($3='' OR status=$3) AND ($4='' OR alert_type=$4) AND ($7::timestamptz IS NULL OR created_at<$7 OR (created_at=$7 AND id>$8)) ORDER BY created_at DESC,id LIMIT $5 OFFSET $6`, org, f.BranchID, f.Status, f.AlertType, limit, offset, after, f.AfterID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.OperationalAlert{}
	for rows.Next() {
		a, e := scanAlert(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// TransitionAlert moves an alert at the expected version.
func (r *Repository) TransitionAlert(ctx context.Context, q database.Querier, org, id string, expected int64, to, owner, code, reason string) (models.OperationalAlert, error) {
	return scanAlert(q.QueryRow(ctx, `UPDATE fleet_partner_operational_alerts SET status=$4,owner_user_id=CASE WHEN $4='ACKNOWLEDGED' THEN $5 ELSE owner_user_id END,acknowledged_at=CASE WHEN $4='ACKNOWLEDGED' THEN now() ELSE acknowledged_at END,resolved_at=CASE WHEN $4='RESOLVED' THEN now() ELSE resolved_at END,resolution_code=$6,resolution_reason=$7,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+alertColumns, org, id, expected, to, owner, code, reason))
}

// LockAlertStatus locks an alert and returns its status.
func (r *Repository) LockAlertStatus(ctx context.Context, q database.Querier, org, id string) (string, error) {
	var status string
	e := q.QueryRow(ctx, `SELECT status FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id).Scan(&status)
	return status, noRows(e)
}

// CloseAlert closes an alert whose source no longer exists and returns its
// new version.
func (r *Repository) CloseAlert(ctx context.Context, q database.Querier, org, id, to, code, reason string) (int64, error) {
	var version int64
	e := q.QueryRow(ctx, `UPDATE fleet_partner_operational_alerts SET status=$3,resolved_at=CASE WHEN $3='RESOLVED' THEN now() ELSE resolved_at END,resolution_code=$4,resolution_reason=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, org, id, to, code, reason).Scan(&version)
	return version, e
}

// LockAlertSeverity locks an alert and returns its version, status and
// severity.
func (r *Repository) LockAlertSeverity(ctx context.Context, q database.Querier, org, id string) (int64, string, string, error) {
	var version int64
	var status, severity string
	e := q.QueryRow(ctx, `SELECT version,status,severity FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id).Scan(&version, &status, &severity)
	return version, status, severity, e
}

// SetAlertSeverity escalates an alert's severity.
func (r *Repository) SetAlertSeverity(ctx context.Context, q database.Querier, org, id, severity string) error {
	_, e := q.Exec(ctx, `UPDATE fleet_partner_operational_alerts SET severity=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, id, severity)
	return e
}

// AlertHistory appends one alert transition.
func (r *Repository) AlertHistory(ctx context.Context, q database.Querier, org, id, from, to, actor, reason string, version int64) error {
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_operational_alert_history(id,organization_id,alert_id,previous_status,new_status,actor_id,reason,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("alh"), org, id, from, to, actor, reason, version)
	return e
}

// ScheduleAlertEscalation enqueues the SLA escalation of an alert unless one
// is already pending or running.
func (r *Repository) ScheduleAlertEscalation(ctx context.Context, org, alertID string, payload []byte) error {
	_, e := r.pool.Exec(ctx, `INSERT INTO fleet_partner_jobs(id,organization_id,job_type,dedupe_key,payload,status,priority,scheduled_at,max_attempts,created_at) SELECT $1,$2,$3,$4,$5,'PENDING',0,now(),8,now() WHERE NOT EXISTS(SELECT 1 FROM fleet_partner_jobs WHERE organization_id=$2 AND job_type=$3 AND dedupe_key=$4 AND status IN('PENDING','RETRY_SCHEDULED','CLAIMED','RUNNING'))`, ids.New("job"), org, models.JobAlertEscalate, "alert-sla:"+alertID, payload)
	return e
}

const rescueColumns = `rc.id,rc.organization_id,rc.branch_id,rc.trigger_type,rc.severity,rc.status,COALESCE(rc.private_assignment_id,''),rc.canonical_trip_reference,COALESCE(rc.asset_id,''),COALESCE(rc.breakdown_id,''),rc.reason,rc.assigned_to_user_id,COALESCE(rc.current_assignment_id,''),COALESCE(rc.replacement_assignment_id,''),COALESCE(ra.status,''),rc.replacement_membership_id,rc.replacement_asset_reference,rc.replacement_attempts,rc.alert_id,rc.resolution_code,rc.resolution,rc.resolved_by,rc.resolved_at,rc.cancelled_at,rc.created_by,rc.version,rc.created_at,rc.updated_at`
const rescueFrom = ` FROM fleet_partner_rescue_cases rc LEFT JOIN fleet_partner_private_assignments ra ON ra.organization_id=rc.organization_id AND ra.id=rc.replacement_assignment_id`

func scanRescue(row pgx.Row) (models.RescueCase, error) {
	var v models.RescueCase
	var resolution []byte
	e := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.TriggerType, &v.Severity, &v.Status, &v.PrivateAssignmentID, &v.CanonicalTripReference, &v.AssetID, &v.BreakdownID, &v.Reason, &v.AssignedToUserID, &v.CurrentAssignmentID, &v.ReplacementAssignmentID, &v.ReplacementAssignmentStatus, &v.ReplacementMembershipID, &v.ReplacementAssetReference, &v.ReplacementAttempts, &v.AlertID, &v.ResolutionCode, &resolution, &v.ResolvedBy, &v.ResolvedAt, &v.CancelledAt, &v.CreatedBy, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, models.ErrNotFound
	}
	v.Resolution = json.RawMessage(resolution)
	if len(v.Resolution) == 0 {
		v.Resolution = json.RawMessage(`{}`)
	}
	return v, e
}

// GetRescue reads (and optionally locks) one rescue case with the status of
// its replacement assignment.
func (r *Repository) GetRescue(ctx context.Context, q database.Querier, org, id string, lock bool) (models.RescueCase, error) {
	s := `SELECT ` + rescueColumns + rescueFrom + ` WHERE rc.organization_id=$1 AND rc.id=$2`
	if lock {
		s += ` FOR UPDATE OF rc`
	}
	return scanRescue(r.q(q).QueryRow(ctx, s, org, id))
}

// ListRescues reads up to limit rescue cases, newest first with id ascending
// as the tie-breaker. It pages by f.Offset, or by the keyset position
// f.After/f.AfterID when set.
func (r *Repository) ListRescues(ctx context.Context, org string, f models.RescueFilter, limit int) ([]models.RescueCase, error) {
	offset := pagination.ClampOffset(f.Offset)
	var after any
	if f.After != nil {
		after, offset = *f.After, 0
	}
	rows, e := r.pool.Query(ctx, `SELECT `+rescueColumns+rescueFrom+` WHERE rc.organization_id=$1 AND ($2='' OR rc.branch_id=$2) AND ($3='' OR rc.status=$3) AND ($4='' OR rc.trigger_type=$4) AND ($7::timestamptz IS NULL OR rc.created_at<$7 OR (rc.created_at=$7 AND rc.id>$8)) ORDER BY rc.created_at DESC,rc.id LIMIT $5 OFFSET $6`, org, f.BranchID, f.Status, f.TriggerType, limit, offset, after, f.AfterID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.RescueCase{}
	for rows.Next() {
		v, e := scanRescue(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// AssignmentBranch resolves the branch of a private assignment.
func (r *Repository) AssignmentBranch(ctx context.Context, q database.Querier, org, id string) (string, error) {
	var branch string
	e := q.QueryRow(ctx, `SELECT branch_id FROM fleet_partner_private_assignments WHERE organization_id=$1 AND id=$2`, org, id).Scan(&branch)
	return branch, e
}

// BreakdownAsset resolves the asset and asset branch of a breakdown.
func (r *Repository) BreakdownAsset(ctx context.Context, q database.Querier, org, id string) (string, string, error) {
	var asset, branch string
	e := q.QueryRow(ctx, `SELECT b.asset_id,a.branch_id FROM fleet_partner_breakdowns b JOIN fleet_partner_assets a ON a.organization_id=b.organization_id AND a.id=b.asset_id WHERE b.organization_id=$1 AND b.id=$2`, org, id).Scan(&asset, &branch)
	return asset, branch, e
}

// AssetBranch resolves the branch of an asset.
func (r *Repository) AssetBranch(ctx context.Context, q database.Querier, org, id string) (string, error) {
	var branch string
	e := q.QueryRow(ctx, `SELECT branch_id FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2`, org, id).Scan(&branch)
	return branch, e
}

// BranchExists reports whether the branch belongs to the organization.
func (r *Repository) BranchExists(ctx context.Context, q database.Querier, org, id string) (bool, error) {
	var exists bool
	e := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_branches WHERE organization_id=$1 AND id=$2)`, org, id).Scan(&exists)
	return exists, e
}

// InsertRescue opens a rescue case, replaying the stored case for a matching
// idempotency key and fingerprint. inserted reports whether it is new; a
// reused key with another fingerprint is models.ErrConflict.
func (r *Repository) InsertRescue(ctx context.Context, q database.Querier, org, branch string, in models.OpenRescueCaseInput, subject, actor, fp string) (string, bool, error) {
	var inserted bool
	var id string
	x := q.QueryRow(ctx, `INSERT INTO fleet_partner_rescue_cases(id,organization_id,branch_id,trigger_type,severity,subject_key,private_assignment_id,canonical_trip_reference,asset_id,breakdown_id,reason,assigned_to_user_id,current_assignment_id,created_by,idempotency_key,request_fingerprint)
VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,NULLIF($9,''),NULLIF($10,''),$11,$12,NULLIF($7,''),$13,$14,$15)
ON CONFLICT(organization_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE fleet_partner_rescue_cases.request_fingerprint=EXCLUDED.request_fingerprint
RETURNING id,(xmax=0)`, ids.New("rsc"), org, branch, in.TriggerType, in.Severity, subject, in.PrivateAssignmentID, in.CanonicalTripReference, in.AssetID, in.BreakdownID, in.Reason, in.AssignedToUserID, actor, in.IdempotencyKey, fp).Scan(&id, &inserted)
	if errors.Is(x, pgx.ErrNoRows) {
		return "", false, models.ErrConflict
	}
	return id, inserted, x
}

// SetRescueAlert links a rescue case to the alert raised for it.
func (r *Repository) SetRescueAlert(ctx context.Context, q database.Querier, org, id, alertID string) error {
	_, e := q.Exec(ctx, `UPDATE fleet_partner_rescue_cases SET alert_id=$3 WHERE organization_id=$1 AND id=$2`, org, id, alertID)
	return e
}

// RescueHistory appends one rescue case transition.
func (r *Repository) RescueHistory(ctx context.Context, q database.Querier, org string, v models.RescueCase, action, from, to, actor, reason string) error {
	snapshot, _ := json.Marshal(map[string]any{"currentAssignmentId": v.CurrentAssignmentID, "replacementAssignmentId": v.ReplacementAssignmentID, "replacementAttempts": v.ReplacementAttempts, "resolutionCode": v.ResolutionCode})
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_rescue_case_history(id,organization_id,rescue_case_id,action,previous_status,new_status,actor_id,reason,version,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("rch"), org, v.ID, action, from, to, actor, reason, v.Version, snapshot)
	return e
}

// SetRescueStatus moves a rescue case at the expected version and returns
// its new version and update time. A stale version is models.ErrConflict.
func (r *Repository) SetRescueStatus(ctx context.Context, q database.Querier, org, id string, expected int64, status string) (int64, time.Time, error) {
	var version int64
	var updated time.Time
	e := q.QueryRow(ctx, `UPDATE fleet_partner_rescue_cases SET status=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version,updated_at`, org, id, expected, status).Scan(&version, &updated)
	if errors.Is(e, pgx.ErrNoRows) {
		return 0, updated, models.ErrConflict
	}
	return version, updated, e
}

// LockRescueAssignment locks the private assignment a rescue case holds.
func (r *Repository) LockRescueAssignment(ctx context.Context, q database.Querier, org, id string) (models.RescueAssignment, error) {
	var a models.RescueAssignment
	e := q.QueryRow(ctx, `SELECT status,work_origin,COALESCE(private_order_id,shipment_id,route_stop_id,''),provider_membership_id,execution_service,branch_id,version FROM fleet_partner_private_assignments WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id).Scan(&a.Status, &a.WorkOrigin, &a.WorkID, &a.ProviderMembershipID, &a.ExecutionService, &a.BranchID, &a.Version)
	return a, e
}

// OfferRescueReplacement records a replacement offer on a rescue case.
func (r *Repository) OfferRescueReplacement(ctx context.Context, q database.Querier, org, id string, expected int64, assignmentID, membershipID, assetReference string) error {
	var version int64
	return q.QueryRow(ctx, `UPDATE fleet_partner_rescue_cases SET status='REPLACEMENT_PENDING',current_assignment_id=$4,replacement_assignment_id=$4,replacement_membership_id=$5,replacement_asset_reference=$6,replacement_attempts=replacement_attempts+1,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version`, org, id, expected, assignmentID, membershipID, assetReference).Scan(&version)
}

// ResolveRescue resolves a rescue case at the expected version.
func (r *Repository) ResolveRescue(ctx context.Context, q database.Querier, org, id string, expected int64, code string, resolution []byte, actor string) error {
	var version int64
	return q.QueryRow(ctx, `UPDATE fleet_partner_rescue_cases SET status='RESOLVED',resolution_code=$4,resolution=$5,resolved_by=$6,resolved_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version`, org, id, expected, code, resolution, actor).Scan(&version)
}

// CancelRescue cancels a rescue case at the expected version.
func (r *Repository) CancelRescue(ctx context.Context, q database.Querier, org, id string, expected int64, resolution []byte) error {
	var version int64
	return q.QueryRow(ctx, `UPDATE fleet_partner_rescue_cases SET status='CANCELLED',cancelled_at=now(),resolution=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version`, org, id, expected, resolution).Scan(&version)
}

// RescuesAwaitingSync reads up to limit rescue cases whose replacement
// provider has responded.
func (r *Repository) RescuesAwaitingSync(ctx context.Context, q database.Querier, limit int) ([]models.RescueOrganizationRef, error) {
	rows, e := q.Query(ctx, `SELECT rc.organization_id,rc.id FROM fleet_partner_rescue_cases rc JOIN fleet_partner_private_assignments ra ON ra.organization_id=rc.organization_id AND ra.id=rc.replacement_assignment_id
WHERE rc.status='REPLACEMENT_PENDING' AND ra.status<>'PENDING_PROVIDER_RESPONSE' ORDER BY rc.updated_at LIMIT $1`, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	due := []models.RescueOrganizationRef{}
	for rows.Next() {
		var v models.RescueOrganizationRef
		if e = rows.Scan(&v.OrganizationID, &v.ID); e != nil {
			return nil, e
		}
		due = append(due, v)
	}
	return due, rows.Err()
}
