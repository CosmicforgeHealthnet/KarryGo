package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	dispatchmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func (s *Service) Optimize(ctx context.Context, t tenant.Context, id string, expected int64, key string) (models.OptimizationRequest, error) {
	if e := s.gate(s.o.Flags.RouteOptimization, true, "route_optimization_disabled"); e != nil {
		return models.OptimizationRequest{}, e
	}
	if !s.o.Flags.OptimizationProviderConfigured {
		return models.OptimizationRequest{}, unavailable("optimization_provider_unavailable", "Optimization provider is not configured.")
	}
	if e := dispatchmodels.RequireVersion(expected); e != nil {
		return models.OptimizationRequest{}, e
	}
	var out models.OptimizationRequest
	e := s.tx(ctx, func(tx pgx.Tx) error {
		route, x := s.repo.GetRoute(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(route.BranchID) {
			return dispatchmodels.NotFound()
		}
		if route.Version != expected {
			return dispatchmodels.ErrConflict
		}
		if route.Status != models.RouteDraft && route.Status != models.RouteOptimized && route.Status != models.RouteReady {
			return conflict("route_not_optimizable", "Route cannot be optimized in its current state.")
		}
		stops, x := s.repo.ListStops(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return x
		}
		input := models.OptimizationInput{RouteID: id, RouteVersion: route.Version, Stops: stops, Constraints: map[string]any{}}
		raw, _ := json.Marshal(input)
		inputFingerprint := models.OptimizationFingerprint(input)
		if existing, findErr := s.repo.FindOptimization(ctx, tx, t.OrganizationID, id, route.Version, inputFingerprint); findErr == nil {
			out = existing
			return nil
		} else if !errors.Is(findErr, dispatchmodels.ErrNotFound) {
			return findErr
		}
		out = models.OptimizationRequest{ID: newID("opt"), OrganizationID: t.OrganizationID, RoutePlanID: id, RouteVersion: route.Version, OptimizationVersion: route.OptimizationVersion + 1, Status: "PENDING", InputFingerprint: inputFingerprint, InputSnapshot: raw, IdempotencyKey: key, RequestFingerprint: dispatchmodels.Fingerprint(input)}
		if x = s.repo.CreateOptimization(ctx, tx, out); x != nil {
			return x
		}
		_, x = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: t.OrganizationID, JobType: models.JobOptimizationSubmit, DedupeKey: id + ":" + itoa(route.Version) + ":" + out.InputFingerprint, Payload: map[string]any{"organizationId": t.OrganizationID, "optimizationRequestId": out.ID}, MaxAttempts: 8})
		if x != nil {
			return x
		}
		_, x = s.repo.UpdateRoute(ctx, tx, t.OrganizationID, id, expected, models.RouteOptimizationPending, "", "", "", nil, out.ID, out.OptimizationVersion, -1, -1, "")
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_plan.optimization_requested.v1", "route_plan", id, expected+1, map[string]any{"optimizationRequestId": out.ID, "inputFingerprint": out.InputFingerprint})
	})
	return out, e
}

// RegisterWorkers binds the route jobs to the durable worker.
func (s *Service) RegisterWorkers(w *jobs.Worker) {
	w.Register(models.JobOptimizationSubmit, s.handleOptimizationSubmit)
	w.Register(models.JobOptimizationReadBack, s.handleOptimizationReadBack)
	w.Register(models.JobRouteProgress, s.handleRouteProgress)
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
	request, e := s.repo.GetOptimization(ctx, nil, p.OrganizationID, p.OptimizationRequestID)
	if e != nil {
		return e
	}
	if request.Status == "SUCCEEDED" {
		return nil
	}
	var input models.OptimizationInput
	if e = json.Unmarshal(request.InputSnapshot, &input); e != nil {
		return e
	}
	ref, unknown, e := s.o.Optimizer.Submit(ctx, input)
	if e != nil {
		return e
	}
	status := "RUNNING"
	next := models.JobOptimizationReadBack
	if unknown {
		status = "RECONCILIATION_REQUIRED"
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if e := s.repo.MarkOptimizationSubmitted(ctx, tx, p.OrganizationID, p.OptimizationRequestID, status, ref); e != nil {
			return e
		}
		_, e := s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: p.OrganizationID, JobType: next, DedupeKey: p.OptimizationRequestID + ":" + ref, Payload: p, MaxAttempts: 8, ScheduledAt: s.o.Now().Add(time.Second)})
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
	request, e := s.repo.GetOptimization(ctx, nil, p.OrganizationID, p.OptimizationRequestID)
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
		route, e := s.repo.GetRoute(ctx, tx, p.OrganizationID, request.RoutePlanID, true)
		if e != nil {
			return e
		}
		if route.Version != request.RouteVersion+1 || route.OptimizationVersion != request.OptimizationVersion {
			return s.repo.CompleteOptimization(ctx, tx, p.OrganizationID, request.ID, result, "RECONCILIATION_REQUIRED")
		}
		if e = s.repo.CompleteOptimization(ctx, tx, p.OrganizationID, request.ID, result, "SUCCEEDED"); e != nil {
			return e
		}
		_, e = s.repo.UpdateRoute(ctx, tx, p.OrganizationID, route.ID, route.Version, models.RouteOptimized, "", "", "", nil, request.ID, request.OptimizationVersion, result.EstimatedDistance, result.EstimatedDurationSeconds, "")
		if e == nil && s.o.Events != nil {
			e = s.o.Events.Publish(ctx, tx, "route_plan.optimized.v1", "route_plan", route.ID, route.Version+1, p.OrganizationID, route.BranchID, map[string]any{"routePlanId": route.ID, "optimizationRequestId": request.ID, "optimizationVersion": request.OptimizationVersion, "inputFingerprint": request.InputFingerprint})
		}
		return e
	})
}

var progressTargets = map[string]string{"PENDING_PROVIDER_RESPONSE": models.StopAssigned, "ACCEPTED": models.StopAssigned, "READY_FOR_EXECUTION": models.StopAssigned, "IN_PROGRESS": models.StopEnRoute, "COMPLETED": models.StopCompleted, "DECLINED": models.StopFailed, "EXPIRED": models.StopFailed, "REASSIGNMENT_REQUIRED": models.StopFailed, "CANCELLED": models.StopCancelled}

func (s *Service) handleRouteProgress(ctx context.Context, job jobs.Job) error {
	if e := s.workerGate(); e != nil {
		return e
	}
	p, e := decodeJob(job)
	if e != nil {
		return e
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		route, e := s.repo.GetRoute(ctx, tx, p.OrganizationID, p.RoutePlanID, true)
		if e != nil {
			return e
		}
		stops, e := s.repo.ListStops(ctx, tx, p.OrganizationID, p.RoutePlanID)
		if e != nil {
			return e
		}
		systemTenant := tenant.Context{OrganizationID: p.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "route-progress-worker", CorrelationID: job.ID}
		for i := range stops {
			st := stops[i]
			if models.IsDepot(st.StopType) {
				continue
			}
			assignmentStatus, x := s.repo.LatestStopAssignmentStatus(ctx, tx, p.OrganizationID, st.ID)
			if errors.Is(x, dispatchmodels.ErrNotFound) {
				continue
			}
			if x != nil {
				return x
			}
			target := progressTargets[assignmentStatus]
			if target == "" || target == st.Status || !models.ValidStopTransition(st.Status, target) {
				continue
			}
			version, x := s.repo.SetStopStatus(ctx, tx, p.OrganizationID, p.RoutePlanID, st.ID, st.Version, target, "")
			if x != nil {
				return x
			}
			if x = s.repo.History(ctx, tx, p.OrganizationID, "ROUTE_STOP", st.ID, st.Status, target, "AUTHORITATIVE_ASSIGNMENT_PROJECTION", systemTenant.ActorID, version, map[string]any{"assignmentStatus": assignmentStatus}); x != nil {
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
			if models.IsDepot(st.StopType) {
				continue
			}
			operationalStops++
			if st.Status == models.StopCompleted {
				completed++
			}
			if st.Status == models.StopFailed || st.Status == models.StopSkipped || st.Status == models.StopCancelled {
				failed++
			}
		}
		to := route.Status
		if operationalStops > 0 && completed == operationalStops {
			to = models.RouteCompleted
		} else if completed > 0 && completed+failed == operationalStops {
			to = models.RoutePartiallyCompleted
		} else if failed > 0 && completed == 0 && completed+failed == operationalStops {
			to = models.RouteFailed
		} else if completed > 0 && route.Status == models.RoutePublished {
			to = models.RouteInProgress
		}
		if to == route.Status {
			return nil
		}
		if !models.ValidRouteTransition(route.Status, to) {
			return nil
		}
		updated, e := s.repo.UpdateRoute(ctx, tx, p.OrganizationID, p.RoutePlanID, route.Version, to, "", "", "", nil, "", 0, -1, -1, "")
		if e != nil {
			return e
		}
		if e = s.repo.History(ctx, tx, p.OrganizationID, "ROUTE_PLAN", route.ID, route.Status, to, "STOP_STATUS_AGGREGATION", systemTenant.ActorID, updated.Version, map[string]any{"completedStops": completed, "failedStops": failed, "operationalStops": operationalStops}); e != nil {
			return e
		}
		return s.record(ctx, tx, systemTenant, "route_plan.updated.v1", "route_plan", route.ID, updated.Version, map[string]any{"previousStatus": route.Status, "status": to, "source": "route_progress_worker"})
	})
}

// EnqueueDueProgress schedules one progress job per active route whose stop
// assignments changed, inside the caller's maintenance transaction.
func (s *Service) EnqueueDueProgress(ctx context.Context, tx pgx.Tx) error {
	due, e := s.repo.DueProgressRoutes(ctx, tx)
	if e != nil {
		return e
	}
	for _, v := range due {
		if _, e = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: v.OrganizationID, JobType: models.JobRouteProgress, DedupeKey: v.RoutePlanID + ":assignment:" + itoa(v.AssignmentVersion), Payload: map[string]any{"organizationId": v.OrganizationID, "routePlanId": v.RoutePlanID}, MaxAttempts: 8}); e != nil {
			return e
		}
	}
	return nil
}
