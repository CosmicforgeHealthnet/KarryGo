package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	trackingmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// RegisterWorkers binds the dispatch jobs (geofence evaluation and alert
// escalation) to the existing durable worker. The generic worker provides
// SKIP LOCKED claims, owner-only heartbeat, expired-lease recovery, bounded
// retry, dead-letter and graceful shutdown.
func (s *Service) RegisterWorkers(w *jobs.Worker) {
	w.Register(models.JobGeofenceEvaluate, s.handleGeofenceEvaluate)
	w.Register(models.JobAlertGenerate, s.handleAlertGenerate)
	w.Register(models.JobAlertEscalate, s.handleAlertEscalate)
}

type jobPayload struct {
	OrganizationID         string `json:"organizationId"`
	CanonicalTripReference string `json:"canonicalTripReference"`
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
func (s *Service) handleGeofenceEvaluate(ctx context.Context, job jobs.Job) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	if !s.o.Flags.Geofences || !s.o.Flags.TrackingProjections {
		return errors.New("geofence evaluation disabled")
	}
	if s.o.Tracking == nil {
		return errors.New("tracking projections unavailable")
	}
	p, e := decodeJob(job)
	if e != nil {
		return e
	}
	tracking, e := s.o.Tracking.Projection(ctx, p.OrganizationID, p.CanonicalTripReference)
	if e != nil {
		return e
	}
	if tracking.Latitude == nil || tracking.Longitude == nil || trackingmodels.Freshness(tracking.SourceTimestamp, s.o.Now(), s.o.TrackingFreshAfter, s.o.TrackingStaleAfter, tracking.GapStatus != "CURRENT") != trackingmodels.Fresh {
		return nil
	}
	geos, e := s.repo.ActiveGeofences(ctx, p.OrganizationID)
	if e != nil {
		return e
	}
	for _, g := range geos {
		inside := models.Contains(g, models.Point{Latitude: *tracking.Latitude, Longitude: *tracking.Longitude})
		e = s.tx(ctx, func(tx pgx.Tx) error {
			// Serialize each fence/trip transition so two tracking jobs cannot both
			// observe the same previous state and create an alert storm.
			if _, e := s.repo.GetGeofence(ctx, tx, p.OrganizationID, g.ID, true); e != nil {
				return e
			}
			state, e := s.repo.GeofenceTransitionState(ctx, tx, p.OrganizationID, g.ID, p.CanonicalTripReference)
			if e != nil {
				return e
			}
			for _, rule := range models.GeofenceTransitions(g, inside, state, tracking.SourceTimestamp, tracking.RoutePlanID != "") {
				inserted, e := s.repo.InsertGeofenceEvent(ctx, tx, p.OrganizationID, g.ID, p.CanonicalTripReference, rule, tracking.SourceVersion, tracking.SourceTimestamp)
				if e != nil {
					return e
				}
				if !inserted {
					continue
				}
				alertType := map[string]string{"ENTER": "GEOFENCE_ENTRY", "EXIT": "GEOFENCE_EXIT", "ROUTE_DEVIATION": "ROUTE_DEVIATION"}[rule]
				if alertType != "" && s.o.Flags.OperationalAlerts {
					_, e = s.createAlert(ctx, tx, tenant.Context{OrganizationID: p.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "geofence-worker"}, models.AlertRequest{Kind: alertType, Severity: "MEDIUM", SourceType: "GEOFENCE", SourceID: g.ID, BranchID: g.BranchID, RoutePlanID: tracking.RoutePlanID, AssignmentID: tracking.AssignmentID, CanonicalTripReference: p.CanonicalTripReference, Condition: fmt.Sprintf("geofence:%s:%s:%s:%d", g.ID, p.CanonicalTripReference, rule, tracking.SourceVersion)})
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

func (s *Service) handleAlertGenerate(ctx context.Context, job jobs.Job) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	p, e := decodeJob(job)
	if e != nil {
		return e
	}
	return s.repo.ScheduleAlertEscalation(ctx, p.OrganizationID, p.AlertID, job.Payload)
}
func (s *Service) handleAlertEscalate(ctx context.Context, job jobs.Job) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	p, e := decodeJob(job)
	if e != nil {
		return e
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		version, status, severity, e := s.repo.LockAlertSeverity(ctx, tx, p.OrganizationID, p.AlertID)
		if e != nil {
			return e
		}
		if status == models.AlertResolved || status == models.AlertDismissed {
			return nil
		}
		next := models.EscalatedSeverity(severity)
		if e = s.repo.SetAlertSeverity(ctx, tx, p.OrganizationID, p.AlertID, next); e != nil {
			return e
		}
		if s.o.Events != nil {
			return s.o.Events.Publish(ctx, tx, "operational_alert.escalated.v1", "operational_alert", p.AlertID, version+1, p.OrganizationID, "", map[string]any{"alertId": p.AlertID, "severity": next, "notificationIntent": true})
		}
		return nil
	})
}
func (s *Service) ReplayJob(ctx context.Context, t tenant.Context, id string) error {
	if e := s.gate(s.o.Flags.Workers, true, "section16_workers_disabled"); e != nil {
		return e
	}
	// The alerts.replay permission covers Section 16 work only. Another
	// organization's job, an unknown job and another feature's job are the
	// same safe not-found; a job that is not dead-lettered is a conflict.
	job, e := s.o.Queue.Get(ctx, t.OrganizationID, id)
	if e != nil || !strings.HasPrefix(job.JobType, "section16.") {
		return nf()
	}
	if job.Status != jobs.StatusDeadLettered {
		return errorcodes.Conflict("job_not_dead_lettered", "Only dead-lettered work can be replayed.")
	}
	if e = s.o.Queue.Replay(ctx, t.OrganizationID, id); e != nil {
		return errorcodes.Conflict("job_not_dead_lettered", "Only dead-lettered work can be replayed.")
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return s.record(ctx, tx, t, "section16_job.replayed.v1", "job", id, job.Version+1, map[string]any{"jobId": id, "jobType": job.JobType})
	})
}

// RunMaintenance sweeps tracking freshness and retention, raises stale and
// assignment alerts, projects rescue replacements and schedules route
// progress, in one transaction.
func (s *Service) RunMaintenance(ctx context.Context) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	if s.o.Tracking == nil || s.o.Routes == nil {
		return errors.New("section16 maintenance requires routes and tracking")
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		stale, e := s.o.Tracking.SweepProjections(ctx, tx, s.o.Flags.OperationalAlerts)
		if e != nil {
			return e
		}
		if s.o.Flags.OperationalAlerts {
			for _, v := range stale {
				t := tenant.Context{OrganizationID: v.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "section16-maintenance"}
				if _, e = s.createAlert(ctx, tx, t, models.AlertRequest{Kind: "TRACKING_STALE", Severity: "MEDIUM", SourceType: "CANONICAL_TRIP", SourceID: v.CanonicalTripReference, RoutePlanID: v.RoutePlanID, AssignmentID: v.AssignmentID, CanonicalTripReference: v.CanonicalTripReference, Condition: "tracking-stale:" + v.SourceService + ":" + v.CanonicalTripReference}); e != nil {
					return e
				}
			}
			conditions, e := s.repo.RouteStopAssignmentConditions(ctx, tx)
			if e != nil {
				return e
			}
			for _, v := range conditions {
				alertType := "PROVIDER_DECLINED"
				if v.Status == "EXPIRED" {
					alertType = "ASSIGNMENT_EXPIRED"
				}
				t := tenant.Context{OrganizationID: v.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "section16-maintenance"}
				if _, e = s.createAlert(ctx, tx, t, models.AlertRequest{Kind: alertType, Severity: "HIGH", SourceType: "PRIVATE_ASSIGNMENT", SourceID: v.AssignmentID, BranchID: v.BranchID, RoutePlanID: v.RoutePlanID, AssignmentID: v.AssignmentID, Condition: "assignment-condition:" + v.AssignmentID + ":" + v.Status}); e != nil {
					return e
				}
			}
		}
		// Project replacement-provider responses onto rescue cases (spec 16.14)
		// even when no dispatcher touches the case.
		if e := s.SyncRescueReplacements(ctx, tx, 200); e != nil {
			return e
		}
		return s.o.Routes.EnqueueDueProgress(ctx, tx)
	})
}
