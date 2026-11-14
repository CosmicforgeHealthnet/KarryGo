// Package usecases owns the Section 16 route-planning workflows: route plans
// and stops, optimization through a provider-neutral optimizer, approval,
// publication into Section 15 private assignments, replanning and the route
// progress projection. Persistence, the optimizer, candidate eligibility,
// private assignments and dispatch plans are reached through the ports
// declared here.
package usecases

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	dispatchmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Repository is the route-planning persistence port. Methods taking a
// database.Querier join the caller's transaction; nil reads the pool.
type Repository interface {
	CreateRoute(ctx context.Context, q database.Querier, v models.RoutePlan, key, fp string) (models.RoutePlan, error)
	GetRoute(ctx context.Context, q database.Querier, org, id string, lock bool) (models.RoutePlan, error)
	ListRoutes(ctx context.Context, org string, f dispatchmodels.PageFilter, limit int) ([]models.RoutePlan, error)
	UpdateRoute(ctx context.Context, q database.Querier, org, id string, expected int64, status, providerMembership, provider, vehicle string, constraints json.RawMessage, optimizationRef string, optimizationVersion int64, distance float64, duration int, actor string) (models.RoutePlan, error)
	CreateStop(ctx context.Context, q database.Querier, v models.RouteStop) (models.RouteStop, error)
	ListStops(ctx context.Context, q database.Querier, org, route string) ([]models.RouteStop, error)
	UpdateStop(ctx context.Context, q database.Querier, org, route, id string, expected int64, v models.RouteStop) (models.RouteStop, error)
	DeleteStop(ctx context.Context, q database.Querier, org, route, id string) error
	NormalizeStops(ctx context.Context, q database.Querier, org, route string) error
	SetStopStatus(ctx context.Context, q database.Querier, org, route, id string, expected int64, status, requiredStatus string) (int64, error)
	History(ctx context.Context, q database.Querier, org, kind, id, from, to, reason, actor string, version int64, snapshot any) error
	InsertPublicationSnapshot(ctx context.Context, q database.Querier, org, route string, version int64, assignmentIDs []string, overrideReason string, snapshot any, actor string) error
	LatestStopAssignmentStatus(ctx context.Context, q database.Querier, org, stopID string) (string, error)
	DueProgressRoutes(ctx context.Context, q database.Querier) ([]models.RouteProgress, error)
	CreateOptimization(ctx context.Context, q database.Querier, o models.OptimizationRequest) error
	FindOptimization(ctx context.Context, q database.Querier, org, route string, version int64, fingerprint string) (models.OptimizationRequest, error)
	GetOptimization(ctx context.Context, q database.Querier, org, id string) (models.OptimizationRequest, error)
	MarkOptimizationSubmitted(ctx context.Context, q database.Querier, org, id, status, providerJobReference string) error
	CompleteOptimization(ctx context.Context, q database.Querier, org, id string, result models.OptimizationResult, status string) error
}

// EventPublisher appends domain events to the outbox inside a transaction.
type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

// Recorder writes the audit entry and outbox event of one change inside the
// caller's transaction.
type Recorder interface {
	Record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error
}

// CandidateSource returns the authoritative eligibility facts of every
// provider for a unit of work.
type CandidateSource interface {
	Facts(context.Context, string, string, string, string) ([]dispatchmodels.CandidateFact, error)
}

// PrivateAssignmentCreator offers route-stop work through a Section 15
// private assignment inside the caller's transaction.
type PrivateAssignmentCreator interface {
	CreateInTransaction(context.Context, pgx.Tx, tenant.Context, privateusecases.CreateInput) (privatemodels.Assignment, error)
}

// PlanLookup resolves a dispatch plan's branch inside the caller's
// transaction.
type PlanLookup interface {
	PlanBranch(ctx context.Context, q database.Querier, org, planID string) (string, error)
}

// OptimizationPort is the provider-neutral route optimizer contract.
type OptimizationPort interface {
	Submit(ctx context.Context, input models.OptimizationInput) (providerJobReference string, unknown bool, err error)
	ReadBack(ctx context.Context, providerJobReference string) (models.OptimizationResult, bool, error)
}

// Options wires the service. Repository and Recorder are required.
type Options struct {
	Pool               *pgxpool.Pool
	Repository         Repository
	Recorder           Recorder
	Events             EventPublisher
	PrivateAssignments PrivateAssignmentCreator
	Candidates         CandidateSource
	Plans              PlanLookup
	Optimizer          OptimizationPort
	Queue              *jobs.Queue
	Flags              dispatchmodels.Flags
	CandidateTTL       time.Duration
	Now                func() time.Time
}

// Service implements the route-planning workflows.
type Service struct {
	o    Options
	repo Repository
}

// NewService builds the service.
func NewService(o Options) *Service {
	if o.Queue == nil {
		o.Queue = jobs.NewQueue(o.Pool)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.CandidateTTL <= 0 {
		o.CandidateTTL = 5 * time.Minute
	}
	return &Service{o: o, repo: o.Repository}
}

func (s *Service) gate(enabled, write bool, code string) error {
	return s.o.Flags.Gate(enabled, write, code)
}
func (s *Service) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, fn)
}
func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error {
	return s.o.Recorder.Record(ctx, tx, t, event, kind, id, version, payload)
}

// RecordHistory appends one transition to the shared dispatch/route history
// inside the caller's transaction. Dispatch plans record through it.
func (s *Service) RecordHistory(ctx context.Context, tx pgx.Tx, org, kind, id, from, to, reason, actor string, version int64, snapshot any) error {
	return s.repo.History(ctx, tx, org, kind, id, from, to, reason, actor, version, snapshot)
}

// RouteBranch resolves the branch of a route plan, for callers that scope a
// route-linked object by its route.
func (s *Service) RouteBranch(ctx context.Context, org, routeID string) (string, error) {
	r, e := s.repo.GetRoute(ctx, nil, org, routeID, false)
	if e != nil {
		return "", e
	}
	return r.BranchID, nil
}

// BoardRoutes reads route plans for the control-tower board without the
// capability gate or branch filtering the board applies itself.
func (s *Service) BoardRoutes(ctx context.Context, org string, f dispatchmodels.PageFilter) ([]models.RoutePlan, error) {
	return s.repo.ListRoutes(ctx, org, f, dispatchmodels.BoundedLimit(f.Limit))
}

func (s *Service) CreateRoute(ctx context.Context, t tenant.Context, in models.CreateRouteInput) (models.RoutePlan, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, true, "route_planning_disabled"); e != nil {
		return models.RoutePlan{}, e
	}
	if e := dispatchmodels.RequireBranch(t, in.BranchID); e != nil {
		return models.RoutePlan{}, e
	}
	in.ExecutionService = strings.ToUpper(in.ExecutionService)
	in.RouteType = strings.ToUpper(in.RouteType)
	if in.ExecutionService == "TAXI" || in.RouteType == "PASSENGER" {
		return models.RoutePlan{}, forbiddenTaxi()
	}
	if in.ExecutionService != "DISPATCH" && in.ExecutionService != "HAULING" || in.RouteReference == "" || in.IdempotencyKey == "" {
		return models.RoutePlan{}, badRequest("routeReference, executionService and Idempotency-Key are required.")
	}
	if len(in.ConstraintSnapshot) == 0 {
		in.ConstraintSnapshot = json.RawMessage(`{}`)
	}
	var out models.RoutePlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		if in.DispatchPlanID != "" {
			if s.o.Plans == nil {
				return dispatchmodels.NotFound()
			}
			branch, x := s.o.Plans.PlanBranch(ctx, tx, t.OrganizationID, in.DispatchPlanID)
			if x != nil || branch != in.BranchID {
				return dispatchmodels.NotFound()
			}
		}
		var x error
		out, x = s.repo.CreateRoute(ctx, tx, models.RoutePlan{ID: newID("rte"), OrganizationID: t.OrganizationID, BranchID: in.BranchID, DispatchPlanID: in.DispatchPlanID, RouteReference: in.RouteReference, RouteType: in.RouteType, ProviderMembershipID: in.ProviderMembershipID, ProviderID: in.ProviderID, VehicleOrTruckReference: in.VehicleOrTruckReference, ExecutionService: in.ExecutionService, Status: models.RouteDraft, ConstraintSnapshot: in.ConstraintSnapshot}, in.IdempotencyKey, dispatchmodels.Fingerprint(in))
		if x != nil {
			return x
		}
		if x = s.repo.History(ctx, tx, t.OrganizationID, "ROUTE_PLAN", out.ID, "", out.Status, "CREATED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_plan.created.v1", "route_plan", out.ID, out.Version, out)
	})
	return out, e
}
func (s *Service) GetRoute(ctx context.Context, t tenant.Context, id string) (models.RoutePlan, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, false, "route_planning_disabled"); e != nil {
		return models.RoutePlan{}, e
	}
	v, e := s.repo.GetRoute(ctx, nil, t.OrganizationID, id, false)
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return models.RoutePlan{}, dispatchmodels.NotFound()
	}
	return v, nil
}

// ListRoutes returns one page of route plans in the caller's branch scope
// and the cursor of the next page ("" on the last page).
func (s *Service) ListRoutes(ctx context.Context, t tenant.Context, f dispatchmodels.PageFilter) ([]models.RoutePlan, string, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, false, "route_planning_disabled"); e != nil {
		return nil, "", e
	}
	if f.BranchID != "" && !t.BranchPermitted(f.BranchID) {
		return nil, "", dispatchmodels.NotFound()
	}
	size := dispatchmodels.BoundedLimit(f.Limit)
	items, e := s.repo.ListRoutes(ctx, t.OrganizationID, f, size+1)
	if e != nil {
		return nil, "", e
	}
	items, next := dispatchmodels.TrimPage(items, size, func(v models.RoutePlan) (time.Time, string) { return v.CreatedAt, v.ID })
	out := items[:0]
	for _, v := range items {
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, next, nil
}

func (s *Service) EditRoute(ctx context.Context, t tenant.Context, id string, in models.EditRouteInput) (models.RoutePlan, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, true, "route_planning_disabled"); e != nil {
		return models.RoutePlan{}, e
	}
	if e := dispatchmodels.RequireVersion(in.ExpectedVersion); e != nil {
		return models.RoutePlan{}, e
	}
	var out models.RoutePlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.repo.GetRoute(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return dispatchmodels.NotFound()
		}
		if old.Status != models.RouteDraft && old.Status != models.RouteOptimized && old.Status != models.RouteReady {
			return conflict("route_immutable", "Published or approved routes require replan.")
		}
		out, x = s.repo.UpdateRoute(ctx, tx, t.OrganizationID, id, in.ExpectedVersion, "", in.ProviderMembershipID, in.ProviderID, in.VehicleOrTruckReference, in.ConstraintSnapshot, "", 0, -1, -1, "")
		if x != nil {
			return x
		}
		if x = s.repo.History(ctx, tx, t.OrganizationID, "ROUTE_PLAN", id, old.Status, out.Status, "EDITED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_plan.updated.v1", "route_plan", id, out.Version, out)
	})
	return out, e
}
func (s *Service) AddStop(ctx context.Context, t tenant.Context, routeID string, v models.RouteStop) (models.RouteStop, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, true, "route_planning_disabled"); e != nil {
		return models.RouteStop{}, e
	}
	if e := models.ValidateStop(v); e != nil {
		return models.RouteStop{}, e
	}
	var out models.RouteStop
	e := s.tx(ctx, func(tx pgx.Tx) error {
		route, x := s.repo.GetRoute(ctx, tx, t.OrganizationID, routeID, true)
		if x != nil || !t.BranchPermitted(route.BranchID) {
			return dispatchmodels.NotFound()
		}
		if route.Status != models.RouteDraft {
			return conflict("route_immutable", "Stops may only be added to draft routes.")
		}
		stops, x := s.repo.ListStops(ctx, tx, t.OrganizationID, routeID)
		if x != nil {
			return x
		}
		v.ID = newID("rst")
		v.OrganizationID = t.OrganizationID
		v.RoutePlanID = routeID
		v.Sequence = len(stops) + 1
		v.Status = models.StopPlanned
		out, x = s.repo.CreateStop(ctx, tx, v)
		if x != nil {
			return x
		}
		if x = s.repo.History(ctx, tx, t.OrganizationID, "ROUTE_STOP", out.ID, "", out.Status, "CREATED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_stop.created.v1", "route_stop", out.ID, out.Version, out)
	})
	return out, e
}
func (s *Service) ListStops(ctx context.Context, t tenant.Context, routeID string) ([]models.RouteStop, error) {
	r, e := s.GetRoute(ctx, t, routeID)
	if e != nil {
		return nil, e
	}
	if !t.BranchPermitted(r.BranchID) {
		return nil, dispatchmodels.NotFound()
	}
	return s.repo.ListStops(ctx, nil, t.OrganizationID, routeID)
}
func (s *Service) EditStop(ctx context.Context, t tenant.Context, routeID, id string, v models.RouteStop) (models.RouteStop, error) {
	if e := dispatchmodels.RequireVersion(v.Version); e != nil {
		return models.RouteStop{}, e
	}
	if e := models.ValidateStop(v); e != nil {
		return models.RouteStop{}, e
	}
	var out models.RouteStop
	e := s.tx(ctx, func(tx pgx.Tx) error {
		route, x := s.repo.GetRoute(ctx, tx, t.OrganizationID, routeID, true)
		if x != nil || !t.BranchPermitted(route.BranchID) {
			return dispatchmodels.NotFound()
		}
		if route.Status != models.RouteDraft {
			return conflict("route_immutable", "Stops may only be edited on draft routes.")
		}
		out, x = s.repo.UpdateStop(ctx, tx, t.OrganizationID, routeID, id, v.Version, v)
		if x != nil {
			return x
		}
		if x = s.repo.History(ctx, tx, t.OrganizationID, "ROUTE_STOP", out.ID, models.StopPlanned, out.Status, "UPDATED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_stop.updated.v1", "route_stop", out.ID, out.Version, out)
	})
	return out, e
}
func (s *Service) DeleteStop(ctx context.Context, t tenant.Context, routeID, id string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		route, x := s.repo.GetRoute(ctx, tx, t.OrganizationID, routeID, true)
		if x != nil || !t.BranchPermitted(route.BranchID) {
			return dispatchmodels.NotFound()
		}
		if route.Status != models.RouteDraft {
			return conflict("route_immutable", "Published route history cannot be deleted.")
		}
		if x = s.repo.DeleteStop(ctx, tx, t.OrganizationID, routeID, id); x != nil {
			return x
		}
		return s.repo.NormalizeStops(ctx, tx, t.OrganizationID, routeID)
	})
}

func (s *Service) transitionRoute(ctx context.Context, t tenant.Context, id, to string, expected int64) (models.RoutePlan, error) {
	if e := dispatchmodels.RequireVersion(expected); e != nil {
		return models.RoutePlan{}, e
	}
	var out models.RoutePlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.repo.GetRoute(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return dispatchmodels.NotFound()
		}
		if !models.ValidRouteTransition(old.Status, to) {
			return conflict("invalid_route_state", "Route transition is not allowed.")
		}
		out, x = s.repo.UpdateRoute(ctx, tx, t.OrganizationID, id, expected, to, "", "", "", nil, "", 0, -1, -1, t.OrganizationUserID)
		if x != nil {
			return x
		}
		if x = s.repo.History(ctx, tx, t.OrganizationID, "ROUTE_PLAN", id, old.Status, to, to, t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_plan."+strings.ToLower(to)+".v1", "route_plan", id, out.Version, out)
	})
	return out, e
}
func (s *Service) ApproveRoute(ctx context.Context, t tenant.Context, id string, v int64) (models.RoutePlan, error) {
	return s.transitionRoute(ctx, t, id, models.RouteApproved, v)
}
func (s *Service) CancelRoute(ctx context.Context, t tenant.Context, id string, v int64) (models.RoutePlan, error) {
	return s.transitionRoute(ctx, t, id, models.RouteCancelled, v)
}

func (s *Service) PublishRoute(ctx context.Context, t tenant.Context, id string, in models.PublishRouteInput) (models.RoutePlan, []string, error) {
	if e := s.gate(s.o.Flags.RoutePublication, true, "route_publication_disabled"); e != nil {
		return models.RoutePlan{}, nil, e
	}
	if e := dispatchmodels.RequireVersion(in.ExpectedVersion); e != nil {
		return models.RoutePlan{}, nil, e
	}
	var out models.RoutePlan
	assignmentIDs := []string{}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		route, x := s.repo.GetRoute(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(route.BranchID) {
			return dispatchmodels.NotFound()
		}
		if route.Status != models.RouteApproved {
			return conflict("invalid_route_state", "Only approved routes may be published.")
		}
		if route.ExecutionService == "TAXI" || route.RouteType == "PASSENGER" {
			return forbiddenTaxi()
		}
		if route.ProviderID == "" || route.ProviderMembershipID == "" || route.VehicleOrTruckReference == "" {
			return conflict("route_assignment_incomplete", "Provider and vehicle/truck are required.")
		}
		stops, x := s.repo.ListStops(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return x
		}
		if len(stops) == 0 {
			return conflict("route_stops_required", "Route must contain stops.")
		}
		for i, stop := range stops {
			if stop.Sequence != i+1 {
				return conflict("route_sequence_invalid", "Route stop sequence must be contiguous.")
			}
			if x = models.ValidateStop(stop); x != nil {
				return x
			}
		}
		if s.o.Candidates == nil {
			return unavailable("candidate_authority_unavailable", "Provider eligibility is unavailable.")
		}
		facts, x := s.o.Candidates.Facts(ctx, t.OrganizationID, route.BranchID, "ROUTE", route.ID)
		if x != nil {
			return unavailable("candidate_authority_unavailable", "Provider eligibility is unavailable.")
		}
		matched := false
		for _, f := range facts {
			if f.ProviderMembershipID == route.ProviderMembershipID && f.ProviderID == route.ProviderID && f.VehicleReference == route.VehicleOrTruckReference {
				c := dispatchmodels.ScoreCandidate(t.OrganizationID, route.BranchID, "ROUTE", route.ID, f, s.o.Now(), s.o.CandidateTTL)
				if len(c.HardBlocks) > 0 {
					return conflict("route_hard_block", "Provider or vehicle has a hard eligibility block.")
				}
				if len(c.Warnings) > 0 && strings.TrimSpace(in.OverrideReason) == "" {
					return conflict("route_warning_override_required", "An explicit override reason is required.")
				}
				matched = true
				break
			}
		}
		if !matched {
			return conflict("route_provider_not_eligible", "Selected provider is not eligible.")
		}
		if s.o.PrivateAssignments == nil {
			return unavailable("private_assignment_unavailable", "Private assignment service is unavailable.")
		}
		for _, stop := range stops {
			if models.IsDepot(stop.StopType) {
				continue
			}
			a, x := s.o.PrivateAssignments.CreateInTransaction(ctx, tx, t, privateusecases.CreateInput{BranchID: route.BranchID, WorkOrigin: "ROUTE_STOP", WorkID: stop.ID, ProviderMembershipID: route.ProviderMembershipID, ProviderID: route.ProviderID, VehicleOrTruckReference: route.VehicleOrTruckReference, ExecutionService: route.ExecutionService})
			if x != nil {
				return x
			}
			assignmentIDs = append(assignmentIDs, a.ID)
			stopVersion, x := s.repo.SetStopStatus(ctx, tx, t.OrganizationID, id, stop.ID, stop.Version, models.StopAssigned, models.StopPlanned)
			if x != nil {
				return dispatchmodels.ErrConflict
			}
			if x = s.record(ctx, tx, t, "route_stop.status_changed.v1", "route_stop", stop.ID, stopVersion, map[string]any{"routePlanId": id, "previousStatus": models.StopPlanned, "status": models.StopAssigned, "assignmentId": a.ID}); x != nil {
				return x
			}
		}
		out, x = s.repo.UpdateRoute(ctx, tx, t.OrganizationID, id, in.ExpectedVersion, models.RoutePublished, "", "", "", nil, "", 0, -1, -1, t.OrganizationUserID)
		if x != nil {
			return x
		}
		publishedStops, x := s.repo.ListStops(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return x
		}
		if x = s.repo.InsertPublicationSnapshot(ctx, tx, t.OrganizationID, id, out.Version, assignmentIDs, in.OverrideReason, map[string]any{"route": out, "stops": publishedStops}, t.ActorID); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_plan.published.v1", "route_plan", id, out.Version, map[string]any{"routeId": id, "assignmentIds": assignmentIDs})
	})
	return out, assignmentIDs, e
}

func (s *Service) Replan(ctx context.Context, t tenant.Context, id string, expected int64, key, reason string) (models.RoutePlan, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, true, "route_planning_disabled"); e != nil {
		return models.RoutePlan{}, e
	}
	if e := dispatchmodels.RequireVersion(expected); e != nil {
		return models.RoutePlan{}, e
	}
	var out models.RoutePlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.repo.GetRoute(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return dispatchmodels.NotFound()
		}
		if old.Status != models.RoutePublished && old.Status != models.RouteInProgress && old.Status != models.RouteFailed {
			return conflict("route_not_replannable", "Only published, active or failed routes may be replanned.")
		}
		out, x = s.repo.CreateRoute(ctx, tx, models.RoutePlan{ID: newID("rte"), OrganizationID: old.OrganizationID, BranchID: old.BranchID, DispatchPlanID: old.DispatchPlanID, ReplacementForRouteID: old.ID, RouteReference: old.RouteReference + "-R" + itoa(old.Version), RouteType: old.RouteType, ExecutionService: old.ExecutionService, Status: models.RouteDraft, ConstraintSnapshot: old.ConstraintSnapshot}, key, dispatchmodels.Fingerprint(map[string]any{"route": id, "version": expected, "reason": reason}))
		if x != nil {
			return x
		}
		stops, x := s.repo.ListStops(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return x
		}
		seq := 0
		for _, st := range stops {
			if st.Status == models.StopCompleted {
				continue
			}
			seq++
			st.ID = newID("rst")
			st.RoutePlanID = out.ID
			st.Sequence = seq
			st.Status = models.StopPlanned
			st.Version = 0
			if _, x = s.repo.CreateStop(ctx, tx, st); x != nil {
				return x
			}
		}
		return s.record(ctx, tx, t, "route_plan.replanned.v1", "route_plan", out.ID, out.Version, map[string]any{"replacementForRouteId": id, "reason": reason})
	})
	return out, e
}
