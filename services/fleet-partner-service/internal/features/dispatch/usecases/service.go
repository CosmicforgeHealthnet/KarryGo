// Package usecases owns the Section 16 dispatch workflows: dispatch plans,
// explainable assignment candidates, the control-tower board, geofences and
// their events, operational alerts, rescue cases, job replay and the
// maintenance sweep. Route plans and tracking projections belong to the
// routes and tracking features and are reached through the ports declared
// here.
package usecases

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	routemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/models"
	trackingmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Repository is the dispatch persistence port. Methods taking a
// database.Querier join the caller's transaction; nil reads the pool.
type Repository interface {
	CreatePlan(ctx context.Context, q database.Querier, p models.DispatchPlan, key, fp string) (models.DispatchPlan, error)
	GetPlan(ctx context.Context, q database.Querier, org, id string, lock bool) (models.DispatchPlan, error)
	ListPlans(ctx context.Context, org string, f models.PageFilter, limit int) ([]models.DispatchPlan, error)
	UpdatePlan(ctx context.Context, q database.Querier, org, id string, expected int64, status, timezone string, start, end time.Time, approver string) (models.DispatchPlan, error)
	PutCandidate(ctx context.Context, q database.Querier, c models.Candidate) error
	ActiveCandidates(ctx context.Context, org, branch string) ([]models.CandidateAvailability, error)
	UnassignedWork(ctx context.Context, org, branch string) ([]map[string]any, error)
	AssignmentsByStatus(ctx context.Context, org, statuses string) ([]models.AssignmentSummary, error)
	RouteStopAssignmentConditions(ctx context.Context, q database.Querier) ([]models.AssignmentCondition, error)
	CreateGeofence(ctx context.Context, q database.Querier, org string, in models.CreateGeofenceInput, actor, fp string) (models.Geofence, error)
	GetGeofence(ctx context.Context, q database.Querier, org, id string, lock bool) (models.Geofence, error)
	ListGeofences(ctx context.Context, org string, f models.PageFilter, limit int) ([]models.Geofence, error)
	ActiveGeofences(ctx context.Context, org string) ([]models.Geofence, error)
	UpdateGeofence(ctx context.Context, q database.Querier, org, id string, expected int64, in models.CreateGeofenceInput) (models.Geofence, error)
	SetGeofenceStatus(ctx context.Context, q database.Querier, org, id string, expected int64, status string) (models.Geofence, error)
	GeofenceTransitionState(ctx context.Context, q database.Querier, org, geofenceID, trip string) (models.GeofenceTransitionState, error)
	InsertGeofenceEvent(ctx context.Context, q database.Querier, org, geofenceID, trip, rule string, sourceVersion int64, occurredAt time.Time) (bool, error)
	ListGeofenceEvents(ctx context.Context, org string, f models.GeofenceEventFilter, limit int) ([]models.GeofenceEvent, error)
	UpsertAlert(ctx context.Context, q database.Querier, org string, req models.AlertRequest) (models.OperationalAlert, bool, error)
	AlertConditionRaised(ctx context.Context, q database.Querier, org, condition string) (bool, error)
	GetAlert(ctx context.Context, q database.Querier, org, id string, lock bool) (models.OperationalAlert, error)
	ListAlerts(ctx context.Context, org string, f models.PageFilter, limit int) ([]models.OperationalAlert, error)
	TransitionAlert(ctx context.Context, q database.Querier, org, id string, expected int64, to, owner, code, reason string) (models.OperationalAlert, error)
	LockAlertStatus(ctx context.Context, q database.Querier, org, id string) (string, error)
	CloseAlert(ctx context.Context, q database.Querier, org, id, to, code, reason string) (int64, error)
	LockAlertSeverity(ctx context.Context, q database.Querier, org, id string) (int64, string, string, error)
	SetAlertSeverity(ctx context.Context, q database.Querier, org, id, severity string) error
	AlertHistory(ctx context.Context, q database.Querier, org, id, from, to, actor, reason string, version int64) error
	ScheduleAlertEscalation(ctx context.Context, org, alertID string, payload []byte) error
	GetRescue(ctx context.Context, q database.Querier, org, id string, lock bool) (models.RescueCase, error)
	ListRescues(ctx context.Context, org string, f models.RescueFilter, limit int) ([]models.RescueCase, error)
	AssignmentBranch(ctx context.Context, q database.Querier, org, id string) (string, error)
	BreakdownAsset(ctx context.Context, q database.Querier, org, id string) (string, string, error)
	AssetBranch(ctx context.Context, q database.Querier, org, id string) (string, error)
	BranchExists(ctx context.Context, q database.Querier, org, id string) (bool, error)
	InsertRescue(ctx context.Context, q database.Querier, org, branch string, in models.OpenRescueCaseInput, subject, actor, fp string) (string, bool, error)
	SetRescueAlert(ctx context.Context, q database.Querier, org, id, alertID string) error
	RescueHistory(ctx context.Context, q database.Querier, org string, v models.RescueCase, action, from, to, actor, reason string) error
	SetRescueStatus(ctx context.Context, q database.Querier, org, id string, expected int64, status string) (int64, time.Time, error)
	LockRescueAssignment(ctx context.Context, q database.Querier, org, id string) (models.RescueAssignment, error)
	OfferRescueReplacement(ctx context.Context, q database.Querier, org, id string, expected int64, assignmentID, membershipID, assetReference string) error
	ResolveRescue(ctx context.Context, q database.Querier, org, id string, expected int64, code string, resolution []byte, actor string) error
	CancelRescue(ctx context.Context, q database.Querier, org, id string, expected int64, resolution []byte) error
	RescuesAwaitingSync(ctx context.Context, q database.Querier, limit int) ([]models.RescueOrganizationRef, error)
}

// EventPublisher appends domain events to the outbox inside a transaction.
type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

// CandidateSource returns the authoritative eligibility facts of every
// provider for a unit of work.
type CandidateSource interface {
	Facts(context.Context, string, string, string, string) ([]models.CandidateFact, error)
}

// RescueAssignmentPort is the narrow Section 15 surface rescue uses to offer
// replacement work. Both calls join the caller's transaction and keep every
// private-assignment gate (eligibility, history, audit, outbox).
type RescueAssignmentPort interface {
	CreateInTransaction(context.Context, pgx.Tx, tenant.Context, privateusecases.CreateInput) (privatemodels.Assignment, error)
	ReassignInTransaction(context.Context, pgx.Tx, tenant.Context, string, privateusecases.ReassignInput) (privatemodels.Assignment, error)
}

// RoutePort is the route-planning surface dispatch uses: board reads, the
// shared dispatch/route history and route progress scheduling.
type RoutePort interface {
	BoardRoutes(ctx context.Context, org string, f models.PageFilter) ([]routemodels.RoutePlan, error)
	RecordHistory(ctx context.Context, tx pgx.Tx, org, kind, id, from, to, reason, actor string, version int64, snapshot any) error
	EnqueueDueProgress(ctx context.Context, tx pgx.Tx) error
}

// TrackingPort is the tracking-projection surface dispatch uses: board
// reads, the geofence worker's position read and the maintenance sweep.
type TrackingPort interface {
	BoardProjections(ctx context.Context, org string, f models.PageFilter) ([]trackingmodels.TrackingProjection, error)
	ProjectionGaps(ctx context.Context, org string) ([]map[string]any, error)
	Projection(ctx context.Context, org, trip string) (trackingmodels.TrackingProjection, error)
	SweepProjections(ctx context.Context, tx pgx.Tx, staleNeeded bool) ([]trackingmodels.StaleProjection, error)
}

// Options wires the service. Repository is required; Routes and Tracking
// are required for the board, the geofence worker and maintenance.
type Options struct {
	Pool                                                           *pgxpool.Pool
	Repository                                                     Repository
	Routes                                                         RoutePort
	Tracking                                                       TrackingPort
	Rescue                                                         RescueAssignmentPort
	Candidates                                                     CandidateSource
	Events                                                         EventPublisher
	Audit                                                          *audit.Recorder
	Queue                                                          *jobs.Queue
	Flags                                                          models.Flags
	CandidateTTL, TrackingFreshAfter, TrackingStaleAfter, AlertSLA time.Duration
	Now                                                            func() time.Time
}

// Service implements the dispatch workflows.
type Service struct {
	o    Options
	repo Repository
}

// NewService builds the service.
func NewService(o Options) *Service {
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
	if o.AlertSLA <= 0 {
		o.AlertSLA = 30 * time.Minute
	}
	return &Service{o: o, repo: o.Repository}
}

// SetRoutes and SetTracking complete the wiring after the routes and
// tracking services exist; they depend on this service for alerts.
func (s *Service) SetRoutes(p RoutePort)      { s.o.Routes = p }
func (s *Service) SetTracking(p TrackingPort) { s.o.Tracking = p }

func (s *Service) gate(enabled, write bool, code string) error {
	return s.o.Flags.Gate(enabled, write, code)
}
func (s *Service) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, fn)
}
func nf() error { return models.NotFound() }

// Record writes the audit entry and outbox event of one Section 16 change
// inside the caller's transaction. Routes record through it.
func (s *Service) Record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error {
	entry := audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)
	if e := s.o.Audit.Record(ctx, tx, entry); e != nil {
		return e
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, version, t.OrganizationID, t.CorrelationID, payload)
	}
	return nil
}
func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error {
	return s.Record(ctx, tx, t, event, kind, id, version, payload)
}
func (s *Service) history(ctx context.Context, tx pgx.Tx, org, kind, id, from, to, reason, actor string, version int64, snapshot any) error {
	if s.o.Routes == nil {
		return errorcodes.Unavailable("route_history_unavailable", "Route history is unavailable.")
	}
	return s.o.Routes.RecordHistory(ctx, tx, org, kind, id, from, to, reason, actor, version, snapshot)
}

// PlanBranch resolves a dispatch plan's branch inside the caller's
// transaction, for route creation.
func (s *Service) PlanBranch(ctx context.Context, q database.Querier, org, planID string) (string, error) {
	p, e := s.repo.GetPlan(ctx, q, org, planID, false)
	if e != nil {
		return "", e
	}
	return p.BranchID, nil
}

func (s *Service) CreatePlan(ctx context.Context, t tenant.Context, in models.CreatePlanInput) (models.DispatchPlan, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, true, "dispatch_planning_disabled"); e != nil {
		return models.DispatchPlan{}, e
	}
	if e := models.RequireBranch(t, in.BranchID); e != nil {
		return models.DispatchPlan{}, e
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" || in.WorkWindowStart.IsZero() || !in.WorkWindowEnd.After(in.WorkWindowStart) || strings.TrimSpace(in.Timezone) == "" {
		return models.DispatchPlan{}, apperrors.BadRequest("branchId, planDate, valid work window, timezone and Idempotency-Key are required.", nil)
	}
	if _, e := time.LoadLocation(in.Timezone); e != nil {
		return models.DispatchPlan{}, apperrors.BadRequest("timezone is invalid.", e)
	}
	var out models.DispatchPlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		out, x = s.repo.CreatePlan(ctx, tx, models.DispatchPlan{ID: ids.New("dsp"), OrganizationID: t.OrganizationID, BranchID: in.BranchID, PlanDate: in.PlanDate, WorkWindowStart: in.WorkWindowStart, WorkWindowEnd: in.WorkWindowEnd, Timezone: in.Timezone, Status: models.PlanDraft, PlannerUserID: t.OrganizationUserID}, in.IdempotencyKey, models.Fingerprint(in))
		if x != nil {
			return x
		}
		if x = s.history(ctx, tx, t.OrganizationID, "DISPATCH_PLAN", out.ID, "", out.Status, "CREATED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "dispatch_plan.created.v1", "dispatch_plan", out.ID, out.Version, out)
	})
	return out, e
}
func (s *Service) GetPlan(ctx context.Context, t tenant.Context, id string) (models.DispatchPlan, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, false, "dispatch_planning_disabled"); e != nil {
		return models.DispatchPlan{}, e
	}
	v, e := s.repo.GetPlan(ctx, nil, t.OrganizationID, id, false)
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return models.DispatchPlan{}, nf()
	}
	return v, nil
}

// ListPlans returns one page of dispatch plans in the caller's branch scope
// and the cursor of the next page ("" on the last page).
func (s *Service) ListPlans(ctx context.Context, t tenant.Context, f models.PageFilter) ([]models.DispatchPlan, string, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, false, "dispatch_planning_disabled"); e != nil {
		return nil, "", e
	}
	if f.BranchID != "" && !t.BranchPermitted(f.BranchID) {
		return nil, "", nf()
	}
	size := models.BoundedLimit(f.Limit)
	items, e := s.repo.ListPlans(ctx, t.OrganizationID, f, size+1)
	if e != nil {
		return nil, "", e
	}
	items, next := models.TrimPage(items, size, func(v models.DispatchPlan) (time.Time, string) { return v.PlanDate, v.ID })
	out := items[:0]
	for _, v := range items {
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, next, nil
}

func (s *Service) EditPlan(ctx context.Context, t tenant.Context, id string, in models.EditPlanInput) (models.DispatchPlan, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, true, "dispatch_planning_disabled"); e != nil {
		return models.DispatchPlan{}, e
	}
	if e := models.RequireVersion(in.ExpectedVersion); e != nil {
		return models.DispatchPlan{}, e
	}
	var out models.DispatchPlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.repo.GetPlan(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return nf()
		}
		if old.Status != models.PlanDraft && old.Status != models.PlanReady {
			return errorcodes.Conflict("invalid_dispatch_plan_state", "Only draft or ready plans are editable.")
		}
		out, x = s.repo.UpdatePlan(ctx, tx, t.OrganizationID, id, in.ExpectedVersion, "", in.Timezone, in.WorkWindowStart, in.WorkWindowEnd, "")
		if x != nil {
			return x
		}
		if x = s.history(ctx, tx, t.OrganizationID, "DISPATCH_PLAN", id, old.Status, out.Status, "EDITED", t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "dispatch_plan.updated.v1", "dispatch_plan", id, out.Version, out)
	})
	return out, e
}
func (s *Service) transitionPlan(ctx context.Context, t tenant.Context, id, to string, expected int64) (models.DispatchPlan, error) {
	if e := models.RequireVersion(expected); e != nil {
		return models.DispatchPlan{}, e
	}
	var out models.DispatchPlan
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.repo.GetPlan(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return nf()
		}
		if !models.ValidPlanTransition(old.Status, to) {
			return errorcodes.Conflict("invalid_dispatch_plan_state", "Dispatch plan transition is not allowed.")
		}
		if to == models.PlanApproved && old.PlannerUserID == t.OrganizationUserID {
			return errorcodes.Forbidden("separation_of_duties", "Planner cannot approve their own plan.")
		}
		out, x = s.repo.UpdatePlan(ctx, tx, t.OrganizationID, id, expected, to, "", time.Time{}, time.Time{}, t.OrganizationUserID)
		if x != nil {
			return x
		}
		if x = s.history(ctx, tx, t.OrganizationID, "DISPATCH_PLAN", id, old.Status, to, to, t.ActorID, out.Version, out); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "dispatch_plan."+strings.ToLower(to)+".v1", "dispatch_plan", id, out.Version, out)
	})
	return out, e
}
func (s *Service) ApprovePlan(ctx context.Context, t tenant.Context, id string, v int64, key string) (models.DispatchPlan, error) {
	return s.transitionPlan(ctx, t, id, models.PlanApproved, v)
}
func (s *Service) PublishPlan(ctx context.Context, t tenant.Context, id string, v int64, key string) (models.DispatchPlan, error) {
	return s.transitionPlan(ctx, t, id, models.PlanPublished, v)
}
func (s *Service) CancelPlan(ctx context.Context, t tenant.Context, id string, v int64, key string) (models.DispatchPlan, error) {
	return s.transitionPlan(ctx, t, id, models.PlanCancelled, v)
}

func (s *Service) GenerateCandidates(ctx context.Context, t tenant.Context, branchID, sourceType, sourceID string) ([]models.Candidate, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, false, "dispatch_planning_disabled"); e != nil {
		return nil, e
	}
	if e := models.RequireBranch(t, branchID); e != nil {
		return nil, e
	}
	if s.o.Candidates == nil {
		return nil, errorcodes.Unavailable("candidate_authority_unavailable", "Candidate authority is unavailable.")
	}
	facts, e := s.o.Candidates.Facts(ctx, t.OrganizationID, branchID, sourceType, sourceID)
	if e != nil {
		return nil, e
	}
	out := make([]models.Candidate, 0, len(facts))
	e = s.tx(ctx, func(tx pgx.Tx) error {
		for _, fact := range facts {
			c := models.ScoreCandidate(t.OrganizationID, branchID, strings.ToUpper(sourceType), sourceID, fact, s.o.Now(), s.o.CandidateTTL)
			c.ID = ids.New("can")
			if x := s.repo.PutCandidate(ctx, tx, c); x != nil {
				return x
			}
			out = append(out, c)
		}
		if len(out) == 0 && s.o.Flags.OperationalAlerts {
			_, _ = s.createAlert(ctx, tx, t, models.AlertRequest{Kind: "NO_ELIGIBLE_PROVIDER", Severity: "HIGH", SourceType: strings.ToUpper(sourceType), SourceID: sourceID, BranchID: branchID, Condition: models.Fingerprint(map[string]string{"source": sourceID, "condition": "none"})})
		}
		return s.record(ctx, tx, t, "assignment_candidates.generated.v1", "assignment_candidates", sourceID, 1, map[string]any{"sourceType": sourceType, "sourceId": sourceID, "count": len(out)})
	})
	return out, e
}
