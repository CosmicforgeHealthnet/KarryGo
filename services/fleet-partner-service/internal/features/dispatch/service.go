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
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	JobOptimizationSubmit   = "section16.route_optimization.submit"
	JobOptimizationReadBack = "section16.route_optimization.readback"
	JobTrackingReconcile    = "section16.tracking.reconcile"
	JobGeofenceEvaluate     = "section16.geofence.evaluate"
	JobAlertGenerate        = "section16.alert.generate"
	JobAlertEscalate        = "section16.alert.escalate"
	JobRouteProgress        = "section16.route.progress"
)

type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type CandidateSource interface {
	Facts(context.Context, string, string, string, string) ([]CandidateFact, error)
}
type PrivateAssignmentCreator interface {
	CreateInTransaction(context.Context, pgx.Tx, tenant.Context, privateusecases.CreateInput) (privatemodels.Assignment, error)
}
type Options struct {
	Pool                                                                              *pgxpool.Pool
	Repository                                                                        *Repository
	PrivateAssignments                                                                PrivateAssignmentCreator
	Candidates                                                                        CandidateSource
	Optimizer                                                                         OptimizationPort
	Events                                                                            EventPublisher
	Audit                                                                             *audit.Recorder
	Queue                                                                             *jobs.Queue
	Flags                                                                             Flags
	CandidateTTL, TrackingFreshAfter, TrackingStaleAfter, TrackingRetention, AlertSLA time.Duration
	Now                                                                               func() time.Time
}
type Service struct{ o Options }

func NewService(o Options) *Service {
	if o.Repository == nil {
		o.Repository = NewRepository(o.Pool)
	}
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Queue == nil {
		o.Queue = jobs.NewQueue(o.Pool)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.CandidateTTL <= 0 {
		o.CandidateTTL = 5 * time.Minute
	}
	if o.TrackingFreshAfter <= 0 {
		o.TrackingFreshAfter = 2 * time.Minute
	}
	if o.TrackingStaleAfter <= 0 {
		o.TrackingStaleAfter = 10 * time.Minute
	}
	if o.TrackingRetention <= 0 {
		o.TrackingRetention = 24 * time.Hour
	}
	if o.AlertSLA <= 0 {
		o.AlertSLA = 30 * time.Minute
	}
	return &Service{o: o}
}
func (s *Service) gate(enabled, write bool, code string) error {
	if s.o.Flags.KillSwitch {
		return errorcodes.Unavailable("fleet_operations_disabled", "Fleet operations are disabled.")
	}
	if !enabled {
		if write {
			return errorcodes.Forbidden(apperrors.Code(code), "This capability is disabled.")
		}
		return errorcodes.NotFound(apperrors.Code(code), "Resource not found.")
	}
	return nil
}
func (s *Service) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, fn)
}
func fingerprint(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func requireVersion(v int64) error {
	if v <= 0 {
		return apperrors.BadRequest("expectedVersion or If-Match is required.", nil)
	}
	return nil
}
func nf() error { return errorcodes.NotFound("section16_not_found", "Resource not found.") }
func branch(t tenant.Context, b string) error {
	if b == "" || !t.BranchPermitted(b) {
		return nf()
	}
	return nil
}

type CreatePlanInput struct {
	BranchID        string    `json:"branchId"`
	PlanDate        time.Time `json:"planDate"`
	WorkWindowStart time.Time `json:"workWindowStart"`
	WorkWindowEnd   time.Time `json:"workWindowEnd"`
	Timezone        string    `json:"timezone"`
	IdempotencyKey  string    `json:"-"`
}

func (s *Service) CreatePlan(ctx context.Context, t tenant.Context, in CreatePlanInput) (DispatchPlan, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, true, "dispatch_planning_disabled"); e != nil {
		return DispatchPlan{}, e
	}
	if e := branch(t, in.BranchID); e != nil {
		return DispatchPlan{}, e
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" || in.WorkWindowStart.IsZero() || !in.WorkWindowEnd.After(in.WorkWindowStart) || strings.TrimSpace(in.Timezone) == "" {
		return DispatchPlan{}, apperrors.BadRequest("branchId, planDate, valid work window, timezone and Idempotency-Key are required.", nil)
	}
	if _, e := time.LoadLocation(in.Timezone); e != nil {
		return DispatchPlan{}, apperrors.BadRequest("timezone is invalid.", e)
	}
	var out DispatchPlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		out, x = s.o.Repository.CreatePlan(ctx, tx, DispatchPlan{ID: ids.New("dsp"), OrganizationID: t.OrganizationID, BranchID: in.BranchID, PlanDate: in.PlanDate, WorkWindowStart: in.WorkWindowStart, WorkWindowEnd: in.WorkWindowEnd, Timezone: in.Timezone, Status: PlanDraft, PlannerUserID: t.OrganizationUserID}, in.IdempotencyKey, fingerprint(in))
		if x != nil {
			return x
		}
		if x = s.o.Repository.History(ctx, tx, t.OrganizationID, "DISPATCH_PLAN", out.ID, "", out.Status, "CREATED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "dispatch_plan.created.v1", "dispatch_plan", out.ID, out.Version, out)
	})
	return out, e
}
func (s *Service) GetPlan(ctx context.Context, t tenant.Context, id string) (DispatchPlan, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, false, "dispatch_planning_disabled"); e != nil {
		return DispatchPlan{}, e
	}
	v, e := s.o.Repository.GetPlan(ctx, nil, t.OrganizationID, id, false)
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return DispatchPlan{}, nf()
	}
	return v, nil
}
func (s *Service) ListPlans(ctx context.Context, t tenant.Context, f PageFilter) ([]DispatchPlan, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, false, "dispatch_planning_disabled"); e != nil {
		return nil, e
	}
	if f.BranchID != "" && !t.BranchPermitted(f.BranchID) {
		return nil, nf()
	}
	items, e := s.o.Repository.ListPlans(ctx, t.OrganizationID, f)
	if e != nil {
		return nil, e
	}
	out := items[:0]
	for _, v := range items {
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, nil
}

type EditPlanInput struct {
	Timezone        string    `json:"timezone"`
	WorkWindowStart time.Time `json:"workWindowStart"`
	WorkWindowEnd   time.Time `json:"workWindowEnd"`
	ExpectedVersion int64     `json:"expectedVersion"`
}

func (s *Service) EditPlan(ctx context.Context, t tenant.Context, id string, in EditPlanInput) (DispatchPlan, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, true, "dispatch_planning_disabled"); e != nil {
		return DispatchPlan{}, e
	}
	if e := requireVersion(in.ExpectedVersion); e != nil {
		return DispatchPlan{}, e
	}
	var out DispatchPlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.o.Repository.GetPlan(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return nf()
		}
		if old.Status != PlanDraft && old.Status != PlanReady {
			return errorcodes.Conflict("invalid_dispatch_plan_state", "Only draft or ready plans are editable.")
		}
		out, x = s.o.Repository.UpdatePlan(ctx, tx, t.OrganizationID, id, in.ExpectedVersion, "", in.Timezone, in.WorkWindowStart, in.WorkWindowEnd, "")
		if x != nil {
			return x
		}
		if x = s.o.Repository.History(ctx, tx, t.OrganizationID, "DISPATCH_PLAN", id, old.Status, out.Status, "EDITED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "dispatch_plan.updated.v1", "dispatch_plan", id, out.Version, out)
	})
	return out, e
}
func (s *Service) transitionPlan(ctx context.Context, t tenant.Context, id, to string, expected int64, key string) (DispatchPlan, error) {
	if e := requireVersion(expected); e != nil {
		return DispatchPlan{}, e
	}
	var out DispatchPlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.o.Repository.GetPlan(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return nf()
		}
		if !ValidPlanTransition(old.Status, to) {
			return errorcodes.Conflict("invalid_dispatch_plan_state", "Dispatch plan transition is not allowed.")
		}
		if to == PlanApproved && old.PlannerUserID == t.OrganizationUserID {
			return errorcodes.Forbidden("separation_of_duties", "Planner cannot approve their own plan.")
		}
		out, x = s.o.Repository.UpdatePlan(ctx, tx, t.OrganizationID, id, expected, to, "", time.Time{}, time.Time{}, t.OrganizationUserID)
		if x != nil {
			return x
		}
		if x = s.o.Repository.History(ctx, tx, t.OrganizationID, "DISPATCH_PLAN", id, old.Status, to, to, t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "dispatch_plan."+strings.ToLower(to)+".v1", "dispatch_plan", id, out.Version, out)
	})
	return out, e
}
func (s *Service) ApprovePlan(ctx context.Context, t tenant.Context, id string, v int64, key string) (DispatchPlan, error) {
	return s.transitionPlan(ctx, t, id, PlanApproved, v, key)
}
func (s *Service) PublishPlan(ctx context.Context, t tenant.Context, id string, v int64, key string) (DispatchPlan, error) {
	return s.transitionPlan(ctx, t, id, PlanPublished, v, key)
}
func (s *Service) CancelPlan(ctx context.Context, t tenant.Context, id string, v int64, key string) (DispatchPlan, error) {
	return s.transitionPlan(ctx, t, id, PlanCancelled, v, key)
}

type CreateRouteInput struct {
	BranchID, DispatchPlanID, RouteReference, RouteType, ProviderMembershipID, ProviderID, VehicleOrTruckReference, ExecutionService string
	ConstraintSnapshot                                                                                                               json.RawMessage
	IdempotencyKey                                                                                                                   string
}

func (s *Service) CreateRoute(ctx context.Context, t tenant.Context, in CreateRouteInput) (RoutePlan, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, true, "route_planning_disabled"); e != nil {
		return RoutePlan{}, e
	}
	if e := branch(t, in.BranchID); e != nil {
		return RoutePlan{}, e
	}
	in.ExecutionService = strings.ToUpper(in.ExecutionService)
	in.RouteType = strings.ToUpper(in.RouteType)
	if in.ExecutionService == "TAXI" || in.RouteType == "PASSENGER" {
		return RoutePlan{}, errorcodes.Forbidden("taxi_execution_disabled", "Passenger route publication is disabled.")
	}
	if in.ExecutionService != "DISPATCH" && in.ExecutionService != "HAULING" || in.RouteReference == "" || in.IdempotencyKey == "" {
		return RoutePlan{}, apperrors.BadRequest("routeReference, executionService and Idempotency-Key are required.", nil)
	}
	if len(in.ConstraintSnapshot) == 0 {
		in.ConstraintSnapshot = json.RawMessage(`{}`)
	}
	var out RoutePlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		if in.DispatchPlanID != "" {
			p, x := s.o.Repository.GetPlan(ctx, tx, t.OrganizationID, in.DispatchPlanID, false)
			if x != nil || p.BranchID != in.BranchID {
				return nf()
			}
		}
		var x error
		out, x = s.o.Repository.CreateRoute(ctx, tx, RoutePlan{ID: ids.New("rte"), OrganizationID: t.OrganizationID, BranchID: in.BranchID, DispatchPlanID: in.DispatchPlanID, RouteReference: in.RouteReference, RouteType: in.RouteType, ProviderMembershipID: in.ProviderMembershipID, ProviderID: in.ProviderID, VehicleOrTruckReference: in.VehicleOrTruckReference, ExecutionService: in.ExecutionService, Status: RouteDraft, ConstraintSnapshot: in.ConstraintSnapshot}, in.IdempotencyKey, fingerprint(in))
		if x != nil {
			return x
		}
		if x = s.o.Repository.History(ctx, tx, t.OrganizationID, "ROUTE_PLAN", out.ID, "", out.Status, "CREATED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_plan.created.v1", "route_plan", out.ID, out.Version, out)
	})
	return out, e
}
func (s *Service) GetRoute(ctx context.Context, t tenant.Context, id string) (RoutePlan, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, false, "route_planning_disabled"); e != nil {
		return RoutePlan{}, e
	}
	v, e := s.o.Repository.GetRoute(ctx, nil, t.OrganizationID, id, false)
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return RoutePlan{}, nf()
	}
	return v, nil
}
func (s *Service) ListRoutes(ctx context.Context, t tenant.Context, f PageFilter) ([]RoutePlan, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, false, "route_planning_disabled"); e != nil {
		return nil, e
	}
	if f.BranchID != "" && !t.BranchPermitted(f.BranchID) {
		return nil, nf()
	}
	items, e := s.o.Repository.ListRoutes(ctx, t.OrganizationID, f)
	if e != nil {
		return nil, e
	}
	out := items[:0]
	for _, v := range items {
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, nil
}

type EditRouteInput struct {
	ProviderMembershipID, ProviderID, VehicleOrTruckReference string
	ConstraintSnapshot                                        json.RawMessage
	ExpectedVersion                                           int64
}

func (s *Service) EditRoute(ctx context.Context, t tenant.Context, id string, in EditRouteInput) (RoutePlan, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, true, "route_planning_disabled"); e != nil {
		return RoutePlan{}, e
	}
	if e := requireVersion(in.ExpectedVersion); e != nil {
		return RoutePlan{}, e
	}
	var out RoutePlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.o.Repository.GetRoute(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return nf()
		}
		if old.Status != RouteDraft && old.Status != RouteOptimized && old.Status != RouteReady {
			return errorcodes.Conflict("route_immutable", "Published or approved routes require replan.")
		}
		out, x = s.o.Repository.UpdateRoute(ctx, tx, t.OrganizationID, id, in.ExpectedVersion, "", in.ProviderMembershipID, in.ProviderID, in.VehicleOrTruckReference, in.ConstraintSnapshot, "", 0, -1, -1, "")
		if x != nil {
			return x
		}
		if x = s.o.Repository.History(ctx, tx, t.OrganizationID, "ROUTE_PLAN", id, old.Status, out.Status, "EDITED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_plan.updated.v1", "route_plan", id, out.Version, out)
	})
	return out, e
}
func validateStop(v RouteStop) error {
	allowed := map[string]bool{"PICKUP": true, "DELIVERY": true, "WAREHOUSE_TRANSFER": true, "RETURN": true, "DEPOT_START": true, "DEPOT_END": true}
	if !allowed[strings.ToUpper(v.StopType)] {
		if strings.HasPrefix(strings.ToUpper(v.StopType), "PASSENGER_") {
			return errorcodes.Forbidden("taxi_execution_disabled", "Passenger stops are disabled.")
		}
		return apperrors.BadRequest("stopType is invalid.", nil)
	}
	if v.TimeWindowStart != nil && v.TimeWindowEnd != nil && !v.TimeWindowEnd.After(*v.TimeWindowStart) {
		return apperrors.BadRequest("time window end must be after start.", nil)
	}
	if (v.Latitude == nil) != (v.Longitude == nil) {
		return apperrors.BadRequest("latitude and longitude must be supplied together.", nil)
	}
	return nil
}
func (s *Service) AddStop(ctx context.Context, t tenant.Context, routeID string, v RouteStop) (RouteStop, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, true, "route_planning_disabled"); e != nil {
		return RouteStop{}, e
	}
	if e := validateStop(v); e != nil {
		return RouteStop{}, e
	}
	var out RouteStop
	e := s.tx(ctx, func(tx pgx.Tx) error {
		route, x := s.o.Repository.GetRoute(ctx, tx, t.OrganizationID, routeID, true)
		if x != nil || !t.BranchPermitted(route.BranchID) {
			return nf()
		}
		if route.Status != RouteDraft {
			return errorcodes.Conflict("route_immutable", "Stops may only be added to draft routes.")
		}
		stops, x := s.o.Repository.ListStops(ctx, tx, t.OrganizationID, routeID)
		if x != nil {
			return x
		}
		v.ID = ids.New("rst")
		v.OrganizationID = t.OrganizationID
		v.RoutePlanID = routeID
		v.Sequence = len(stops) + 1
		v.Status = StopPlanned
		out, x = s.o.Repository.CreateStop(ctx, tx, v)
		if x != nil {
			return x
		}
		if x = s.o.Repository.History(ctx, tx, t.OrganizationID, "ROUTE_STOP", out.ID, "", out.Status, "CREATED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_stop.created.v1", "route_stop", out.ID, out.Version, out)
	})
	return out, e
}
func (s *Service) ListStops(ctx context.Context, t tenant.Context, routeID string) ([]RouteStop, error) {
	r, e := s.GetRoute(ctx, t, routeID)
	if e != nil {
		return nil, e
	}
	if !t.BranchPermitted(r.BranchID) {
		return nil, nf()
	}
	return s.o.Repository.ListStops(ctx, nil, t.OrganizationID, routeID)
}
func (s *Service) EditStop(ctx context.Context, t tenant.Context, routeID, id string, v RouteStop) (RouteStop, error) {
	if e := requireVersion(v.Version); e != nil {
		return RouteStop{}, e
	}
	if e := validateStop(v); e != nil {
		return RouteStop{}, e
	}
	var out RouteStop
	e := s.tx(ctx, func(tx pgx.Tx) error {
		route, x := s.o.Repository.GetRoute(ctx, tx, t.OrganizationID, routeID, true)
		if x != nil || !t.BranchPermitted(route.BranchID) {
			return nf()
		}
		if route.Status != RouteDraft {
			return errorcodes.Conflict("route_immutable", "Stops may only be edited on draft routes.")
		}
		out, x = s.o.Repository.UpdateStop(ctx, tx, t.OrganizationID, routeID, id, v.Version, v)
		if x != nil {
			return x
		}
		if x = s.o.Repository.History(ctx, tx, t.OrganizationID, "ROUTE_STOP", out.ID, StopPlanned, out.Status, "UPDATED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_stop.updated.v1", "route_stop", out.ID, out.Version, out)
	})
	return out, e
}
func (s *Service) DeleteStop(ctx context.Context, t tenant.Context, routeID, id string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		route, x := s.o.Repository.GetRoute(ctx, tx, t.OrganizationID, routeID, true)
		if x != nil || !t.BranchPermitted(route.BranchID) {
			return nf()
		}
		if route.Status != RouteDraft {
			return errorcodes.Conflict("route_immutable", "Published route history cannot be deleted.")
		}
		if x = s.o.Repository.DeleteStop(ctx, tx, t.OrganizationID, routeID, id); x != nil {
			return x
		}
		return s.o.Repository.NormalizeStops(ctx, tx, t.OrganizationID, routeID)
	})
}

func (s *Service) transitionRoute(ctx context.Context, t tenant.Context, id, to string, expected int64) (RoutePlan, error) {
	if e := requireVersion(expected); e != nil {
		return RoutePlan{}, e
	}
	var out RoutePlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.o.Repository.GetRoute(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return nf()
		}
		if !ValidRouteTransition(old.Status, to) {
			return errorcodes.Conflict("invalid_route_state", "Route transition is not allowed.")
		}
		out, x = s.o.Repository.UpdateRoute(ctx, tx, t.OrganizationID, id, expected, to, "", "", "", nil, "", 0, -1, -1, t.OrganizationUserID)
		if x != nil {
			return x
		}
		if x = s.o.Repository.History(ctx, tx, t.OrganizationID, "ROUTE_PLAN", id, old.Status, to, to, t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_plan."+strings.ToLower(to)+".v1", "route_plan", id, out.Version, out)
	})
	return out, e
}
func (s *Service) ApproveRoute(ctx context.Context, t tenant.Context, id string, v int64) (RoutePlan, error) {
	return s.transitionRoute(ctx, t, id, RouteApproved, v)
}
func (s *Service) CancelRoute(ctx context.Context, t tenant.Context, id string, v int64) (RoutePlan, error) {
	return s.transitionRoute(ctx, t, id, RouteCancelled, v)
}

type PublishRouteInput struct {
	ExpectedVersion                int64
	OverrideReason, IdempotencyKey string
}

func (s *Service) PublishRoute(ctx context.Context, t tenant.Context, id string, in PublishRouteInput) (RoutePlan, []string, error) {
	if e := s.gate(s.o.Flags.RoutePublication, true, "route_publication_disabled"); e != nil {
		return RoutePlan{}, nil, e
	}
	if e := requireVersion(in.ExpectedVersion); e != nil {
		return RoutePlan{}, nil, e
	}
	var out RoutePlan
	assignmentIDs := []string{}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		route, x := s.o.Repository.GetRoute(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(route.BranchID) {
			return nf()
		}
		if route.Status != RouteApproved {
			return errorcodes.Conflict("invalid_route_state", "Only approved routes may be published.")
		}
		if route.ExecutionService == "TAXI" || route.RouteType == "PASSENGER" {
			return errorcodes.Forbidden("taxi_execution_disabled", "Passenger route publication is disabled.")
		}
		if route.ProviderID == "" || route.ProviderMembershipID == "" || route.VehicleOrTruckReference == "" {
			return errorcodes.Conflict("route_assignment_incomplete", "Provider and vehicle/truck are required.")
		}
		stops, x := s.o.Repository.ListStops(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return x
		}
		if len(stops) == 0 {
			return errorcodes.Conflict("route_stops_required", "Route must contain stops.")
		}
		for i, stop := range stops {
			if stop.Sequence != i+1 {
				return errorcodes.Conflict("route_sequence_invalid", "Route stop sequence must be contiguous.")
			}
			if x = validateStop(stop); x != nil {
				return x
			}
		}
		if s.o.Candidates == nil {
			return errorcodes.Unavailable("candidate_authority_unavailable", "Provider eligibility is unavailable.")
		}
		facts, x := s.o.Candidates.Facts(ctx, t.OrganizationID, route.BranchID, "ROUTE", route.ID)
		if x != nil {
			return errorcodes.Unavailable("candidate_authority_unavailable", "Provider eligibility is unavailable.")
		}
		matched := false
		for _, f := range facts {
			if f.ProviderMembershipID == route.ProviderMembershipID && f.ProviderID == route.ProviderID && f.VehicleReference == route.VehicleOrTruckReference {
				c := ScoreCandidate(t.OrganizationID, route.BranchID, "ROUTE", route.ID, f, s.o.Now(), s.o.CandidateTTL)
				if len(c.HardBlocks) > 0 {
					return errorcodes.Conflict("route_hard_block", "Provider or vehicle has a hard eligibility block.")
				}
				if len(c.Warnings) > 0 && strings.TrimSpace(in.OverrideReason) == "" {
					return errorcodes.Conflict("route_warning_override_required", "An explicit override reason is required.")
				}
				matched = true
				break
			}
		}
		if !matched {
			return errorcodes.Conflict("route_provider_not_eligible", "Selected provider is not eligible.")
		}
		if s.o.PrivateAssignments == nil {
			return errorcodes.Unavailable("private_assignment_unavailable", "Private assignment service is unavailable.")
		}
		for _, stop := range stops {
			if stop.StopType == "DEPOT_START" || stop.StopType == "DEPOT_END" {
				continue
			}
			a, x := s.o.PrivateAssignments.CreateInTransaction(ctx, tx, t, privateusecases.CreateInput{BranchID: route.BranchID, WorkOrigin: "ROUTE_STOP", WorkID: stop.ID, ProviderMembershipID: route.ProviderMembershipID, ProviderID: route.ProviderID, VehicleOrTruckReference: route.VehicleOrTruckReference, ExecutionService: route.ExecutionService})
			if x != nil {
				return x
			}
			assignmentIDs = append(assignmentIDs, a.ID)
			var stopVersion int64
			x = tx.QueryRow(ctx, `UPDATE fleet_partner_route_stops SET status=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND route_plan_id=$2 AND id=$3 AND version=$4 AND status=$6 RETURNING version`, t.OrganizationID, id, stop.ID, stop.Version, StopAssigned, StopPlanned).Scan(&stopVersion)
			if x != nil {
				return ErrConflict
			}
			if x = s.record(ctx, tx, t, "route_stop.status_changed.v1", "route_stop", stop.ID, stopVersion, map[string]any{"routePlanId": id, "previousStatus": StopPlanned, "status": StopAssigned, "assignmentId": a.ID}); x != nil {
				return x
			}
		}
		out, x = s.o.Repository.UpdateRoute(ctx, tx, t.OrganizationID, id, in.ExpectedVersion, RoutePublished, "", "", "", nil, "", 0, -1, -1, t.OrganizationUserID)
		if x != nil {
			return x
		}
		publishedStops, x := s.o.Repository.ListStops(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return x
		}
		snapshot, _ := json.Marshal(map[string]any{"route": out, "stops": publishedStops})
		assignments, _ := json.Marshal(assignmentIDs)
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_route_publication_snapshots(id,organization_id,route_plan_id,route_version,assignment_ids,override_reason,snapshot,published_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(route_plan_id,route_version) DO NOTHING`, ids.New("rps"), t.OrganizationID, id, out.Version, assignments, in.OverrideReason, snapshot, t.ActorID)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_plan.published.v1", "route_plan", id, out.Version, map[string]any{"routeId": id, "assignmentIds": assignmentIDs})
	})
	return out, assignmentIDs, e
}

func (s *Service) Replan(ctx context.Context, t tenant.Context, id string, expected int64, key, reason string) (RoutePlan, error) {
	if e := s.gate(s.o.Flags.RoutePlanning, true, "route_planning_disabled"); e != nil {
		return RoutePlan{}, e
	}
	if e := requireVersion(expected); e != nil {
		return RoutePlan{}, e
	}
	var out RoutePlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.o.Repository.GetRoute(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return nf()
		}
		if old.Status != RoutePublished && old.Status != RouteInProgress && old.Status != RouteFailed {
			return errorcodes.Conflict("route_not_replannable", "Only published, active or failed routes may be replanned.")
		}
		out, x = s.o.Repository.CreateRoute(ctx, tx, RoutePlan{ID: ids.New("rte"), OrganizationID: old.OrganizationID, BranchID: old.BranchID, DispatchPlanID: old.DispatchPlanID, ReplacementForRouteID: old.ID, RouteReference: old.RouteReference + "-R" + fmt.Sprint(old.Version), RouteType: old.RouteType, ExecutionService: old.ExecutionService, Status: RouteDraft, ConstraintSnapshot: old.ConstraintSnapshot}, key, fingerprint(map[string]any{"route": id, "version": expected, "reason": reason}))
		if x != nil {
			return x
		}
		stops, x := s.o.Repository.ListStops(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return x
		}
		seq := 0
		for _, st := range stops {
			if st.Status == StopCompleted {
				continue
			}
			seq++
			st.ID = ids.New("rst")
			st.RoutePlanID = out.ID
			st.Sequence = seq
			st.Status = StopPlanned
			st.Version = 0
			if _, x = s.o.Repository.CreateStop(ctx, tx, st); x != nil {
				return x
			}
		}
		return s.record(ctx, tx, t, "route_plan.replanned.v1", "route_plan", out.ID, out.Version, map[string]any{"replacementForRouteId": id, "reason": reason})
	})
	return out, e
}

func (s *Service) Optimize(ctx context.Context, t tenant.Context, id string, expected int64, key string) (OptimizationRequest, error) {
	if e := s.gate(s.o.Flags.RouteOptimization, true, "route_optimization_disabled"); e != nil {
		return OptimizationRequest{}, e
	}
	if !s.o.Flags.OptimizationProviderConfigured {
		return OptimizationRequest{}, errorcodes.Unavailable("optimization_provider_unavailable", "Optimization provider is not configured.")
	}
	if e := requireVersion(expected); e != nil {
		return OptimizationRequest{}, e
	}
	var out OptimizationRequest
	e := s.tx(ctx, func(tx pgx.Tx) error {
		route, x := s.o.Repository.GetRoute(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(route.BranchID) {
			return nf()
		}
		if route.Version != expected {
			return ErrConflict
		}
		if route.Status != RouteDraft && route.Status != RouteOptimized && route.Status != RouteReady {
			return errorcodes.Conflict("route_not_optimizable", "Route cannot be optimized in its current state.")
		}
		stops, x := s.o.Repository.ListStops(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return x
		}
		input := OptimizationInput{RouteID: id, RouteVersion: route.Version, Stops: stops, Constraints: map[string]any{}}
		raw, _ := json.Marshal(input)
		inputFingerprint := OptimizationFingerprint(input)
		if existing, findErr := s.o.Repository.FindOptimization(ctx, tx, t.OrganizationID, id, route.Version, inputFingerprint); findErr == nil {
			out = existing
			return nil
		} else if !errors.Is(findErr, ErrNotFound) {
			return findErr
		}
		out = OptimizationRequest{ID: ids.New("opt"), OrganizationID: t.OrganizationID, RoutePlanID: id, RouteVersion: route.Version, OptimizationVersion: route.OptimizationVersion + 1, Status: "PENDING", InputFingerprint: inputFingerprint, InputSnapshot: raw, IdempotencyKey: key, RequestFingerprint: fingerprint(input)}
		if x = s.o.Repository.CreateOptimization(ctx, tx, out); x != nil {
			return x
		}
		_, x = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: t.OrganizationID, JobType: JobOptimizationSubmit, DedupeKey: id + ":" + fmt.Sprint(route.Version) + ":" + out.InputFingerprint, Payload: map[string]any{"organizationId": t.OrganizationID, "optimizationRequestId": out.ID}, MaxAttempts: 8})
		if x != nil {
			return x
		}
		_, x = s.o.Repository.UpdateRoute(ctx, tx, t.OrganizationID, id, expected, RouteOptimizationPending, "", "", "", nil, out.ID, out.OptimizationVersion, -1, -1, "")
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "route_plan.optimization_requested.v1", "route_plan", id, expected+1, map[string]any{"optimizationRequestId": out.ID, "inputFingerprint": out.InputFingerprint})
	})
	return out, e
}

func (s *Service) GenerateCandidates(ctx context.Context, t tenant.Context, branchID, sourceType, sourceID string) ([]Candidate, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, false, "dispatch_planning_disabled"); e != nil {
		return nil, e
	}
	if e := branch(t, branchID); e != nil {
		return nil, e
	}
	if s.o.Candidates == nil {
		return nil, errorcodes.Unavailable("candidate_authority_unavailable", "Candidate authority is unavailable.")
	}
	facts, e := s.o.Candidates.Facts(ctx, t.OrganizationID, branchID, sourceType, sourceID)
	if e != nil {
		return nil, e
	}
	out := make([]Candidate, 0, len(facts))
	e = s.tx(ctx, func(tx pgx.Tx) error {
		for _, fact := range facts {
			c := ScoreCandidate(t.OrganizationID, branchID, strings.ToUpper(sourceType), sourceID, fact, s.o.Now(), s.o.CandidateTTL)
			c.ID = ids.New("can")
			if x := s.o.Repository.PutCandidate(ctx, tx, c); x != nil {
				return x
			}
			out = append(out, c)
		}
		if len(out) == 0 && s.o.Flags.OperationalAlerts {
			_, _ = s.createAlert(ctx, tx, t, "NO_ELIGIBLE_PROVIDER", "HIGH", strings.ToUpper(sourceType), sourceID, branchID, "", "", "", fingerprint(map[string]string{"source": sourceID, "condition": "none"}))
		}
		return s.record(ctx, tx, t, "assignment_candidates.generated.v1", "assignment_candidates", sourceID, 1, map[string]any{"sourceType": sourceType, "sourceId": sourceID, "count": len(out)})
	})
	return out, e
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error {
	entry := audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)
	if e := s.o.Audit.Record(ctx, tx, entry); e != nil {
		return e
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, version, t.OrganizationID, t.CorrelationID, payload)
	}
	return nil
}
