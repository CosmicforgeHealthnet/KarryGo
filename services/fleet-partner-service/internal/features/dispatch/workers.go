package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// RegisterWorkers binds every Section 16 job to the existing durable worker.
// The generic worker provides SKIP LOCKED claims, owner-only heartbeat,
// expired-lease recovery, bounded retry, dead-letter and graceful shutdown.
func (s *Service) RegisterWorkers(w *jobs.Worker) {
	w.Register(JobOptimizationSubmit, s.handleOptimizationSubmit)
	w.Register(JobOptimizationReadBack, s.handleOptimizationReadBack)
	w.Register(JobTrackingReconcile, s.handleTrackingReconcile)
	w.Register(JobGeofenceEvaluate, s.handleGeofenceEvaluate)
	w.Register(JobAlertGenerate, s.handleAlertGenerate)
	w.Register(JobAlertEscalate, s.handleAlertEscalate)
	w.Register(JobRouteProgress, s.handleRouteProgress)
}

type jobPayload struct {
	OrganizationID         string `json:"organizationId"`
	OptimizationRequestID  string `json:"optimizationRequestId"`
	SourceService          string `json:"sourceService"`
	CanonicalTripReference string `json:"canonicalTripReference"`
	RoutePlanID            string `json:"routePlanId"`
	AlertID                string `json:"alertId"`
}

func decodeJob(job jobs.Job) (jobPayload, error) {
	var p jobPayload
	e := json.Unmarshal(job.Payload, &p)
	if e != nil {
		return p, e
	}
	if p.OrganizationID != job.OrganizationID {
		return p, jobs.ErrTenantMismatch
	}
	return p, nil
}
func (s *Service) workerGate() error {
	if s.o.Flags.KillSwitch || !s.o.Flags.Workers {
		return errors.New("section16 workers disabled")
	}
	return nil
}
func (s *Service) handleOptimizationSubmit(ctx context.Context, job jobs.Job) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	if !s.o.Flags.RouteOptimization || !s.o.Flags.OptimizationProviderConfigured || s.o.Optimizer == nil {
		return errors.New("optimization provider disabled or unavailable")
	}
	p, e := decodeJob(job)
	if e != nil {
		return e
	}
	request, e := s.o.Repository.GetOptimization(ctx, nil, p.OrganizationID, p.OptimizationRequestID)
	if e != nil {
		return e
	}
	if request.Status == "SUCCEEDED" {
		return nil
	}
	var input OptimizationInput
	if e = json.Unmarshal(request.InputSnapshot, &input); e != nil {
		return e
	}
	ref, unknown, e := s.o.Optimizer.Submit(ctx, input)
	if e != nil {
		return e
	}
	status := "RUNNING"
	next := JobOptimizationReadBack
	if unknown {
		status = "RECONCILIATION_REQUIRED"
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE fleet_partner_route_optimization_requests SET status=$3,provider_job_reference=$4,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status IN('PENDING','RUNNING','RECONCILIATION_REQUIRED')`, p.OrganizationID, p.OptimizationRequestID, status, ref)
		if e != nil {
			return e
		}
		_, e = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: p.OrganizationID, JobType: next, DedupeKey: p.OptimizationRequestID + ":" + ref, Payload: p, MaxAttempts: 8, ScheduledAt: s.o.Now().Add(time.Second)})
		return e
	})
}
func (s *Service) handleOptimizationReadBack(ctx context.Context, job jobs.Job) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	if s.o.Optimizer == nil {
		return errors.New("optimization provider unavailable")
	}
	p, e := decodeJob(job)
	if e != nil {
		return e
	}
	request, e := s.o.Repository.GetOptimization(ctx, nil, p.OrganizationID, p.OptimizationRequestID)
	if e != nil {
		return e
	}
	if request.Status == "SUCCEEDED" {
		return nil
	}
	result, found, e := s.o.Optimizer.ReadBack(ctx, request.ProviderJobReference)
	if e != nil {
		return e
	}
	if !found {
		return errors.New("optimization outcome remains unknown")
	}
	if result.InputFingerprint != "" && result.InputFingerprint != request.InputFingerprint {
		return errors.New("optimization fingerprint mismatch")
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		route, e := s.o.Repository.GetRoute(ctx, tx, p.OrganizationID, request.RoutePlanID, true)
		if e != nil {
			return e
		}
		if route.Version != request.RouteVersion+1 || route.OptimizationVersion != request.OptimizationVersion {
			return s.o.Repository.CompleteOptimization(ctx, tx, p.OrganizationID, request.ID, result, "RECONCILIATION_REQUIRED")
		}
		if e = s.o.Repository.CompleteOptimization(ctx, tx, p.OrganizationID, request.ID, result, "SUCCEEDED"); e != nil {
			return e
		}
		_, e = s.o.Repository.UpdateRoute(ctx, tx, p.OrganizationID, route.ID, route.Version, RouteOptimized, "", "", "", nil, request.ID, request.OptimizationVersion, result.EstimatedDistance, result.EstimatedDurationSeconds, "")
		if e == nil && s.o.Events != nil {
			e = s.o.Events.Publish(ctx, tx, "route_plan.optimized.v1", "route_plan", route.ID, route.Version+1, p.OrganizationID, route.BranchID, map[string]any{"routePlanId": route.ID, "optimizationRequestId": request.ID, "optimizationVersion": request.OptimizationVersion, "inputFingerprint": request.InputFingerprint})
		}
		return e
	})
}
func (s *Service) handleTrackingReconcile(ctx context.Context, job jobs.Job) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	p, e := decodeJob(job)
	if e != nil {
		return e
	}
	if !s.o.Flags.TrackingSourceConfigured {
		return errors.New("authoritative tracking read-back unavailable")
	}
	var gapStatus string
	if e = s.o.Pool.QueryRow(ctx, `SELECT gap_status FROM fleet_partner_tracking_projections WHERE organization_id=$1 AND source_service=$2 AND canonical_trip_reference=$3`, p.OrganizationID, p.SourceService, p.CanonicalTripReference).Scan(&gapStatus); e != nil {
		return e
	}
	if gapStatus == "CURRENT" {
		_, e = s.o.Pool.Exec(ctx, `UPDATE fleet_partner_tracking_projection_conflicts SET status='RESOLVED',resolved_at=now(),lease_owner=NULL,lease_expires_at=NULL WHERE organization_id=$1 AND source_service=$2 AND canonical_trip_reference=$3 AND status IN('OPEN','RECONCILING')`, p.OrganizationID, p.SourceService, p.CanonicalTripReference)
		return e
	}
	tag, e := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_tracking_projection_conflicts SET status='RECONCILING',lease_heartbeat_at=now() WHERE organization_id=$1 AND source_service=$2 AND canonical_trip_reference=$3 AND status='OPEN'`, p.OrganizationID, p.SourceService, p.CanonicalTripReference)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return errors.New("authoritative read-back required; no safe reconciliation result available")
	}
	return errors.New("authoritative read-back required; outcome not fabricated")
}
func (s *Service) handleGeofenceEvaluate(ctx context.Context, job jobs.Job) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	if !s.o.Flags.Geofences || !s.o.Flags.TrackingProjections {
		return errors.New("geofence evaluation disabled")
	}
	p, e := decodeJob(job)
	if e != nil {
		return e
	}
	tracking, e := s.o.Repository.Tracking(ctx, p.OrganizationID, p.CanonicalTripReference)
	if e != nil {
		return e
	}
	if tracking.Latitude == nil || tracking.Longitude == nil || Freshness(tracking.SourceTimestamp, s.o.Now(), s.o.TrackingFreshAfter, s.o.TrackingStaleAfter, tracking.GapStatus != "CURRENT") != Fresh {
		return nil
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,COALESCE(branch_id,''),name,geometry_type,geometry,geometry_reference,event_rules,active_status,version,created_by,created_at,updated_at FROM fleet_partner_geofences WHERE organization_id=$1 AND active_status='ACTIVE'`, p.OrganizationID)
	if e != nil {
		return e
	}
	defer rows.Close()
	geos := []Geofence{}
	for rows.Next() {
		g, e := scanGeofence(rows)
		if e != nil {
			return e
		}
		geos = append(geos, g)
	}
	for _, g := range geos {
		inside := Contains(g, Point{*tracking.Latitude, *tracking.Longitude})
		e = s.tx(ctx, func(tx pgx.Tx) error {
			// Serialize each fence/trip transition so two tracking jobs cannot both
			// observe the same previous state and create an alert storm.
			var lockedID string
			if e := tx.QueryRow(ctx, `SELECT id FROM fleet_partner_geofences WHERE organization_id=$1 AND id=$2 FOR UPDATE`, p.OrganizationID, g.ID).Scan(&lockedID); e != nil {
				return e
			}
			var lastEnter, lastExit, lastDwell, lastDeviation, lastRestricted int64
			var enteredAt *time.Time
			e := tx.QueryRow(ctx, `SELECT
COALESCE(max(source_version) FILTER(WHERE event_rule='ENTER'),0),
COALESCE(max(source_version) FILTER(WHERE event_rule='EXIT'),0),
COALESCE(max(source_version) FILTER(WHERE event_rule='DWELL'),0),
COALESCE(max(source_version) FILTER(WHERE event_rule='ROUTE_DEVIATION'),0),
COALESCE(max(source_version) FILTER(WHERE event_rule='RESTRICTED_AREA'),0),
max(occurred_at) FILTER(WHERE event_rule='ENTER')
FROM fleet_partner_geofence_events WHERE organization_id=$1 AND geofence_id=$2 AND canonical_trip_reference=$3`, p.OrganizationID, g.ID, p.CanonicalTripReference).Scan(&lastEnter, &lastExit, &lastDwell, &lastDeviation, &lastRestricted, &enteredAt)
			if e != nil {
				return e
			}
			rules := []string{}
			if inside {
				if lastEnter <= lastExit && geofenceRuleAllowed(g, "ENTER") {
					rules = append(rules, "ENTER")
				}
				if lastEnter > lastExit && lastDwell < lastEnter && enteredAt != nil && tracking.SourceTimestamp.Sub(*enteredAt) >= 5*time.Minute && geofenceRuleAllowed(g, "DWELL") {
					rules = append(rules, "DWELL")
				}
				if lastRestricted <= lastExit && geofenceRuleAllowed(g, "RESTRICTED_AREA") {
					rules = append(rules, "RESTRICTED_AREA")
				}
			} else if lastEnter > lastExit {
				if geofenceRuleAllowed(g, "EXIT") {
					rules = append(rules, "EXIT")
				}
				if tracking.RoutePlanID != "" && lastDeviation < lastEnter && geofenceRuleAllowed(g, "ROUTE_DEVIATION") {
					rules = append(rules, "ROUTE_DEVIATION")
				}
			}
			for _, rule := range rules {
				tag, e := tx.Exec(ctx, `INSERT INTO fleet_partner_geofence_events(id,organization_id,geofence_id,canonical_trip_reference,event_rule,source_version,confidence,occurred_at) VALUES($1,$2,$3,$4,$5,$6,'HIGH',$7) ON CONFLICT DO NOTHING`, ids.New("gfe"), p.OrganizationID, g.ID, p.CanonicalTripReference, rule, tracking.SourceVersion, tracking.SourceTimestamp)
				if e != nil || tag.RowsAffected() == 0 {
					if e != nil {
						return e
					}
					continue
				}
				alertType := map[string]string{"ENTER": "GEOFENCE_ENTRY", "EXIT": "GEOFENCE_EXIT", "ROUTE_DEVIATION": "ROUTE_DEVIATION"}[rule]
				if alertType != "" && s.o.Flags.OperationalAlerts {
					_, e = s.createAlert(ctx, tx, tenant.Context{OrganizationID: p.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "geofence-worker"}, alertType, "MEDIUM", "GEOFENCE", g.ID, g.BranchID, tracking.RoutePlanID, tracking.AssignmentID, p.CanonicalTripReference, fmt.Sprintf("geofence:%s:%s:%s:%d", g.ID, p.CanonicalTripReference, rule, tracking.SourceVersion))
					if e != nil {
						return e
					}
				}
				if s.o.Events != nil {
					e = s.o.Events.Publish(ctx, tx, "geofence.triggered.v1", "geofence", g.ID, g.Version, p.OrganizationID, "", map[string]any{"geofenceId": g.ID, "canonicalTripReference": p.CanonicalTripReference, "eventRule": rule, "sourceVersion": tracking.SourceVersion})
					if e != nil {
						return e
					}
				}
			}
			return nil
		})
		if e != nil {
			return e
		}
	}
	return nil
}

func geofenceRuleAllowed(g Geofence, rule string) bool {
	for _, configured := range g.EventRules {
		if configured == rule {
			return true
		}
	}
	return false
}
func (s *Service) handleAlertGenerate(ctx context.Context, job jobs.Job) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	p, e := decodeJob(job)
	if e != nil {
		return e
	}
	_, e = s.o.Pool.Exec(ctx, `INSERT INTO fleet_partner_jobs(id,organization_id,job_type,dedupe_key,payload,status,priority,scheduled_at,max_attempts,created_at) SELECT $1,$2,$3,$4,$5,'PENDING',0,now(),8,now() WHERE NOT EXISTS(SELECT 1 FROM fleet_partner_jobs WHERE organization_id=$2 AND job_type=$3 AND dedupe_key=$4 AND status IN('PENDING','RETRY_SCHEDULED','CLAIMED','RUNNING'))`, ids.New("job"), p.OrganizationID, JobAlertEscalate, "alert-sla:"+p.AlertID, job.Payload)
	return e
}
func (s *Service) handleAlertEscalate(ctx context.Context, job jobs.Job) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	p, e := decodeJob(job)
	if e != nil {
		return e
	}
	return database.WithinTx(ctx, s.o.Pool, func(tx pgx.Tx) error {
		var version int64
		var status, severity string
		e := tx.QueryRow(ctx, `SELECT version,status,severity FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND id=$2 FOR UPDATE`, p.OrganizationID, p.AlertID).Scan(&version, &status, &severity)
		if e != nil {
			return e
		}
		if status == AlertResolved || status == AlertDismissed {
			return nil
		}
		next := map[string]string{"INFO": "LOW", "LOW": "MEDIUM", "MEDIUM": "HIGH", "HIGH": "CRITICAL", "CRITICAL": "CRITICAL"}[severity]
		_, e = tx.Exec(ctx, `UPDATE fleet_partner_operational_alerts SET severity=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, p.OrganizationID, p.AlertID, next)
		if e != nil {
			return e
		}
		if s.o.Events != nil {
			return s.o.Events.Publish(ctx, tx, "operational_alert.escalated.v1", "operational_alert", p.AlertID, version+1, p.OrganizationID, "", map[string]any{"alertId": p.AlertID, "severity": next, "notificationIntent": true})
		}
		return nil
	})
}
func (s *Service) handleRouteProgress(ctx context.Context, job jobs.Job) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	p, e := decodeJob(job)
	if e != nil {
		return e
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		route, e := s.o.Repository.GetRoute(ctx, tx, p.OrganizationID, p.RoutePlanID, true)
		if e != nil {
			return e
		}
		stops, e := s.o.Repository.ListStops(ctx, tx, p.OrganizationID, p.RoutePlanID)
		if e != nil {
			return e
		}
		systemTenant := tenant.Context{OrganizationID: p.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "route-progress-worker", CorrelationID: job.ID}
		for i := range stops {
			st := stops[i]
			if st.StopType == "DEPOT_START" || st.StopType == "DEPOT_END" {
				continue
			}
			var assignmentStatus string
			x := tx.QueryRow(ctx, `SELECT status FROM fleet_partner_private_assignments WHERE organization_id=$1 AND route_stop_id=$2 ORDER BY version DESC,updated_at DESC LIMIT 1`, p.OrganizationID, st.ID).Scan(&assignmentStatus)
			if errors.Is(x, pgx.ErrNoRows) {
				continue
			}
			if x != nil {
				return x
			}
			target := map[string]string{"PENDING_PROVIDER_RESPONSE": StopAssigned, "ACCEPTED": StopAssigned, "READY_FOR_EXECUTION": StopAssigned, "IN_PROGRESS": StopEnRoute, "COMPLETED": StopCompleted, "DECLINED": StopFailed, "EXPIRED": StopFailed, "REASSIGNMENT_REQUIRED": StopFailed, "CANCELLED": StopCancelled}[assignmentStatus]
			if target == "" || target == st.Status || !ValidStopTransition(st.Status, target) {
				continue
			}
			var version int64
			x = tx.QueryRow(ctx, `UPDATE fleet_partner_route_stops SET status=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND route_plan_id=$2 AND id=$3 AND version=$4 RETURNING version`, p.OrganizationID, p.RoutePlanID, st.ID, st.Version, target).Scan(&version)
			if x != nil {
				return x
			}
			_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_dispatch_route_history(id,organization_id,aggregate_type,aggregate_id,previous_status,new_status,reason_code,actor_id,version,snapshot) VALUES($1,$2,'ROUTE_STOP',$3,$4,$5,'AUTHORITATIVE_ASSIGNMENT_PROJECTION',$6,$7,$8)`, ids.New("drh"), p.OrganizationID, st.ID, st.Status, target, systemTenant.ActorID, version, map[string]any{"assignmentStatus": assignmentStatus})
			if x != nil {
				return x
			}
			if x = s.record(ctx, tx, systemTenant, "route_stop.status_changed.v1", "route_stop", st.ID, version, map[string]any{"routePlanId": route.ID, "previousStatus": st.Status, "status": target, "assignmentStatus": assignmentStatus}); x != nil {
				return x
			}
			stops[i].Status = target
			stops[i].Version = version
		}
		completed, failed, operationalStops := 0, 0, 0
		for _, st := range stops {
			if st.StopType == "DEPOT_START" || st.StopType == "DEPOT_END" {
				continue
			}
			operationalStops++
			if st.Status == StopCompleted {
				completed++
			}
			if st.Status == StopFailed || st.Status == StopSkipped || st.Status == StopCancelled {
				failed++
			}
		}
		to := route.Status
		if operationalStops > 0 && completed == operationalStops {
			to = RouteCompleted
		} else if completed > 0 && completed+failed == operationalStops {
			to = RoutePartiallyCompleted
		} else if failed > 0 && completed == 0 && completed+failed == operationalStops {
			to = RouteFailed
		} else if completed > 0 && route.Status == RoutePublished {
			to = RouteInProgress
		}
		if to == route.Status {
			return nil
		}
		if !ValidRouteTransition(route.Status, to) {
			return nil
		}
		updated, e := s.o.Repository.UpdateRoute(ctx, tx, p.OrganizationID, p.RoutePlanID, route.Version, to, "", "", "", nil, "", 0, -1, -1, "")
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO fleet_partner_dispatch_route_history(id,organization_id,aggregate_type,aggregate_id,previous_status,new_status,reason_code,actor_id,version,snapshot) VALUES($1,$2,'ROUTE_PLAN',$3,$4,$5,'STOP_STATUS_AGGREGATION',$6,$7,$8)`, ids.New("drh"), p.OrganizationID, route.ID, route.Status, to, systemTenant.ActorID, updated.Version, map[string]any{"completedStops": completed, "failedStops": failed, "operationalStops": operationalStops})
		if e != nil {
			return e
		}
		return s.record(ctx, tx, systemTenant, "route_plan.updated.v1", "route_plan", route.ID, updated.Version, map[string]any{"previousStatus": route.Status, "status": to, "source": "route_progress_worker"})
	})
}
func (s *Service) ReplayJob(ctx context.Context, t tenant.Context, id string) error {
	if e := s.gate(s.o.Flags.Workers, true, "section16_workers_disabled"); e != nil {
		return e
	}
	return s.o.Queue.Replay(ctx, t.OrganizationID, id)
}
func (s *Service) RunMaintenance(ctx context.Context) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `DELETE FROM fleet_partner_tracking_projections WHERE retention_expires_at<now()`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE fleet_partner_tracking_projections SET freshness_status=CASE WHEN gap_status<>'CURRENT' THEN 'GAPPED' WHEN source_timestamp<now()-$1::interval THEN 'STALE' WHEN source_timestamp<now()-$2::interval THEN 'AGING' ELSE 'FRESH' END`, durationInterval(s.o.TrackingStaleAfter), durationInterval(s.o.TrackingFreshAfter)); e != nil {
			return e
		}
		if s.o.Flags.OperationalAlerts {
			rows, e := tx.Query(ctx, `SELECT organization_id,source_service,canonical_trip_reference,COALESCE(route_plan_id,''),assignment_id FROM fleet_partner_tracking_projections WHERE freshness_status='STALE' LIMIT 200`)
			if e != nil {
				return e
			}
			type staleProjection struct{ org, source, trip, route, assignment string }
			stale := []staleProjection{}
			for rows.Next() {
				var v staleProjection
				if e = rows.Scan(&v.org, &v.source, &v.trip, &v.route, &v.assignment); e != nil {
					rows.Close()
					return e
				}
				stale = append(stale, v)
			}
			if e = rows.Err(); e != nil {
				rows.Close()
				return e
			}
			rows.Close()
			for _, v := range stale {
				t := tenant.Context{OrganizationID: v.org, ActorType: tenant.ActorTypeSystem, ActorID: "section16-maintenance"}
				if _, e = s.createAlert(ctx, tx, t, "TRACKING_STALE", "MEDIUM", "CANONICAL_TRIP", v.trip, "", v.route, v.assignment, v.trip, "tracking-stale:"+v.source+":"+v.trip); e != nil {
					return e
				}
			}

			assignmentRows, e := tx.Query(ctx, `SELECT pa.organization_id,rp.branch_id,rp.id,pa.id,pa.status,pa.version
FROM fleet_partner_private_assignments pa JOIN fleet_partner_route_stops rs ON rs.id=pa.route_stop_id JOIN fleet_partner_route_plans rp ON rp.id=rs.route_plan_id AND rp.organization_id=pa.organization_id
WHERE pa.work_origin='ROUTE_STOP' AND pa.status IN('DECLINED','EXPIRED','REASSIGNMENT_REQUIRED') AND rp.status IN('PUBLISHED','IN_PROGRESS','FAILED') ORDER BY pa.updated_at DESC LIMIT 200`)
			if e != nil {
				return e
			}
			type assignmentCondition struct {
				org, branch, route, assignment, status string
				version                                int64
			}
			conditions := []assignmentCondition{}
			for assignmentRows.Next() {
				var v assignmentCondition
				if e = assignmentRows.Scan(&v.org, &v.branch, &v.route, &v.assignment, &v.status, &v.version); e != nil {
					assignmentRows.Close()
					return e
				}
				conditions = append(conditions, v)
			}
			if e = assignmentRows.Err(); e != nil {
				assignmentRows.Close()
				return e
			}
			assignmentRows.Close()
			for _, v := range conditions {
				alertType := "PROVIDER_DECLINED"
				if v.status == "EXPIRED" {
					alertType = "ASSIGNMENT_EXPIRED"
				}
				t := tenant.Context{OrganizationID: v.org, ActorType: tenant.ActorTypeSystem, ActorID: "section16-maintenance"}
				if _, e = s.createAlert(ctx, tx, t, alertType, "HIGH", "PRIVATE_ASSIGNMENT", v.assignment, v.branch, v.route, v.assignment, "", "assignment-condition:"+v.assignment+":"+v.status); e != nil {
					return e
				}
			}
		}
		routeRows, e := tx.Query(ctx, `SELECT rp.organization_id,rp.id,max(pa.version)
FROM fleet_partner_route_plans rp JOIN fleet_partner_route_stops rs ON rs.route_plan_id=rp.id JOIN fleet_partner_private_assignments pa ON pa.organization_id=rp.organization_id AND pa.route_stop_id=rs.id
WHERE rp.status IN('PUBLISHED','IN_PROGRESS') GROUP BY rp.organization_id,rp.id LIMIT 200`)
		if e != nil {
			return e
		}
		type routeProgress struct {
			org, route string
			version    int64
		}
		dueRoutes := []routeProgress{}
		for routeRows.Next() {
			var v routeProgress
			if e = routeRows.Scan(&v.org, &v.route, &v.version); e != nil {
				routeRows.Close()
				return e
			}
			dueRoutes = append(dueRoutes, v)
		}
		if e = routeRows.Err(); e != nil {
			routeRows.Close()
			return e
		}
		routeRows.Close()
		for _, v := range dueRoutes {
			if _, e = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: v.org, JobType: JobRouteProgress, DedupeKey: v.route + ":assignment:" + fmt.Sprint(v.version), Payload: map[string]any{"organizationId": v.org, "routePlanId": v.route}, MaxAttempts: 8}); e != nil {
				return e
			}
		}
		return nil
	})
}
func durationInterval(d time.Duration) string { return fmt.Sprintf("%d seconds", int64(d.Seconds())) }

// Compile-time guard against accidentally treating source state as owned.
var _ = strings.Builder{}
