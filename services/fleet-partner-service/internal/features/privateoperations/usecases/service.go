// Package usecases implements Section 15 private assignment decisions.
package usecases

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

type WorkInfo struct {
	PrivateOrderID, ShipmentID, BranchID, ContractReference, QuotationReference string
	Ready                                                                       bool
	ExecutionPayload                                                            models.ExecutionPayload
}
type WorkPort interface {
	Resolve(context.Context, database.Querier, string, string, string) (WorkInfo, error)
}
type EligibilityPort interface {
	Validate(context.Context, string, string, string, string, string) (bool, error)
}
type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type Flags struct{ Enabled, WritesEnabled, WorkerEnabled, DispatchEnabled, TaxiEnabled, HaulingEnabled, KillSwitch bool }
type Options struct {
	Pool        *pgxpool.Pool
	Repository  *repositories.Repository
	Work        WorkPort
	Eligibility EligibilityPort
	Events      EventPublisher
	Audit       *audit.Recorder
	Flags       Flags
	ResponseTTL time.Duration
	Now         func() time.Time
}
type Service struct{ opts Options }

func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.ResponseTTL <= 0 {
		o.ResponseTTL = 30 * time.Minute
	}
	return &Service{o}
}
func (s *Service) gate(w bool) error {
	if s.opts.Flags.KillSwitch {
		return errorcodes.Unavailable("private_execution_disabled", "Private operations are unavailable.")
	}
	if !s.opts.Flags.Enabled {
		return errorcodes.NotFound("private_assignment_not_found", "Private assignment not found.")
	}
	if w && !s.opts.Flags.WritesEnabled {
		return errorcodes.Forbidden("private_execution_disabled", "Private operation writes are disabled.")
	}
	return nil
}
func (s *Service) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.opts.Pool, fn)
}
func nf() error {
	return errorcodes.NotFound("private_assignment_not_found", "Private assignment not found.")
}
func enabledService(f Flags, service string) bool {
	switch service {
	case "DISPATCH":
		return f.DispatchEnabled
	case "HAULING":
		return f.HaulingEnabled
	case "TAXI":
		return f.TaxiEnabled
	}
	return false
}

type CreateInput struct {
	BranchID, WorkOrigin, WorkID, ProviderMembershipID, ProviderID, VehicleOrTruckReference, ExecutionService string
	ResponseDeadline                                                                                          *time.Time
}

// ReassignInput names the replacement provider. The original decision is
// immutable; reassigning creates a new aggregate and links the old one to it.
type ReassignInput struct {
	ProviderMembershipID    string
	ProviderID              string
	VehicleOrTruckReference string
	ExecutionService        string
	ExpectedVersion         int64
	ResponseDeadline        *time.Time
}

func (s *Service) Create(ctx context.Context, t tenant.Context, in CreateInput) (models.Assignment, error) {
	if e := s.gate(true); e != nil {
		return models.Assignment{}, e
	}
	origin := strings.ToUpper(strings.TrimSpace(in.WorkOrigin))
	service := strings.ToUpper(strings.TrimSpace(in.ExecutionService))
	if !models.ValidOrigin(origin) || !models.ValidExecutionService(service) {
		return models.Assignment{}, apperrors.BadRequest("workOrigin or executionService is invalid.", nil)
	}
	if service == "TAXI" || !enabledService(s.opts.Flags, service) {
		code := apperrors.Code("private_execution_disabled")
		if service == "TAXI" {
			code = apperrors.Code("taxi_execution_disabled")
		}
		return models.Assignment{}, errorcodes.Forbidden(code, "The selected execution service is disabled.")
	}
	if in.ProviderMembershipID == "" || in.ProviderID == "" {
		return models.Assignment{}, apperrors.BadRequest("providerMembershipId and providerId are required.", nil)
	}
	var out models.Assignment
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		out, x = s.createAssignment(ctx, tx, t, in, origin, service)
		return x
	})
	return out, e
}

// CreateInTransaction lets an owning Fleet orchestration (for example route
// publication) create Section 15 assignments atomically with its own
// publication snapshot. All ordinary gates, authority checks, history, audit
// and outbox writes are identical to Create.
func (s *Service) CreateInTransaction(ctx context.Context, tx pgx.Tx, t tenant.Context, in CreateInput) (models.Assignment, error) {
	if e := s.gate(true); e != nil {
		return models.Assignment{}, e
	}
	origin := strings.ToUpper(strings.TrimSpace(in.WorkOrigin))
	service := strings.ToUpper(strings.TrimSpace(in.ExecutionService))
	if !models.ValidOrigin(origin) || !models.ValidExecutionService(service) {
		return models.Assignment{}, apperrors.BadRequest("workOrigin or executionService is invalid.", nil)
	}
	if service == "TAXI" || !enabledService(s.opts.Flags, service) {
		return models.Assignment{}, errorcodes.Forbidden("private_execution_disabled", "The selected execution service is disabled.")
	}
	if in.ProviderMembershipID == "" || in.ProviderID == "" {
		return models.Assignment{}, apperrors.BadRequest("providerMembershipId and providerId are required.", nil)
	}
	return s.createAssignment(ctx, tx, t, in, origin, service)
}

func (s *Service) createAssignment(ctx context.Context, tx pgx.Tx, t tenant.Context, in CreateInput, origin, service string) (models.Assignment, error) {
	work, x := s.opts.Work.Resolve(ctx, tx, t.OrganizationID, origin, in.WorkID)
	if x != nil || !work.Ready || work.BranchID != in.BranchID || !t.BranchPermitted(work.BranchID) {
		return models.Assignment{}, errorcodes.Conflict("private_assignment_not_eligible", "Private work is not ready for assignment.")
	}
	if s.opts.Eligibility == nil {
		return models.Assignment{}, errorcodes.Unavailable("private_assignment_not_eligible", "Provider eligibility is unavailable.")
	}
	ok, x := s.opts.Eligibility.Validate(ctx, t.OrganizationID, in.ProviderMembershipID, in.ProviderID, service, in.VehicleOrTruckReference)
	if x != nil || !ok {
		return models.Assignment{}, errorcodes.Conflict("private_assignment_not_eligible", "Provider or vehicle is not eligible.")
	}
	deadline := s.opts.Now().Add(s.opts.ResponseTTL)
	if in.ResponseDeadline != nil && in.ResponseDeadline.After(s.opts.Now()) {
		deadline = *in.ResponseDeadline
	}
	a := models.Assignment{ID: ids.New(repositories.PrefixAssignment), OrganizationID: t.OrganizationID, BranchID: work.BranchID, WorkOrigin: origin, ProviderMembershipID: in.ProviderMembershipID, ProviderID: in.ProviderID, VehicleOrTruckReference: in.VehicleOrTruckReference, ExecutionService: service, Status: models.StatusPending, ResponseDeadline: deadline, CreatedBy: t.ActorID}
	switch origin {
	case "PRIVATE_ORDER":
		a.PrivateOrderID = in.WorkID
	case "PRIVATE_SHIPMENT":
		a.ShipmentID = in.WorkID
	case "ROUTE_STOP":
		a.RouteStopID = in.WorkID
	}
	out, x := s.opts.Repository.CreateAssignment(ctx, tx, a)
	if errors.Is(x, repositories.ErrConflict) {
		return models.Assignment{}, errorcodes.Conflict("private_assignment_already_resolved", "Active assignment already exists for this work.")
	}
	if x != nil {
		return models.Assignment{}, x
	}
	if x = s.opts.Repository.History(ctx, tx, t.OrganizationID, out.ID, "", out.Status, "CREATED", string(t.ActorType), t.ActorID, t.CorrelationID); x != nil {
		return models.Assignment{}, x
	}
	if x = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "private_assignments.created", "private_assignment", out.ID)); x != nil {
		return models.Assignment{}, x
	}
	if s.opts.Events != nil {
		if x = s.opts.Events.Publish(ctx, tx, "private_assignment.created.v1", "private_assignment", out.ID, out.Version, t.OrganizationID, t.CorrelationID, map[string]any{"assignmentId": out.ID, "workOrigin": origin, "executionService": service, "providerId": in.ProviderID}); x != nil {
			return models.Assignment{}, x
		}
	}
	return out, nil
}
func (s *Service) Get(ctx context.Context, t tenant.Context, id string) (models.Assignment, error) {
	if e := s.gate(false); e != nil {
		return models.Assignment{}, e
	}
	a, e := s.opts.Repository.GetAssignment(ctx, nil, t.OrganizationID, id)
	if errors.Is(e, repositories.ErrNotFound) || e == nil && !t.BranchPermitted(a.BranchID) {
		return models.Assignment{}, nf()
	}
	return a, e
}
func (s *Service) List(ctx context.Context, t tenant.Context, status string) ([]models.Assignment, error) {
	if e := s.gate(false); e != nil {
		return nil, e
	}
	return s.opts.Repository.ListAssignments(ctx, t.OrganizationID, status)
}

func (s *Service) GetExecution(ctx context.Context, t tenant.Context, id string) (models.ExecutionOperation, error) {
	if e := s.gate(false); e != nil {
		return models.ExecutionOperation{}, e
	}
	o, e := s.opts.Repository.GetExecution(ctx, nil, t.OrganizationID, id)
	if errors.Is(e, repositories.ErrNotFound) {
		return models.ExecutionOperation{}, errorcodes.NotFound("private_execution_not_found", "Private execution operation not found.")
	}
	return o, e
}

func (s *Service) Reassign(ctx context.Context, t tenant.Context, id string, in ReassignInput) (models.Assignment, error) {
	if e := s.gate(true); e != nil {
		return models.Assignment{}, e
	}
	service := strings.ToUpper(strings.TrimSpace(in.ExecutionService))
	if !models.ValidExecutionService(service) || service == "TAXI" || !enabledService(s.opts.Flags, service) {
		return models.Assignment{}, errorcodes.Forbidden("private_execution_disabled", "The selected execution service is disabled.")
	}
	if in.ProviderMembershipID == "" || in.ProviderID == "" {
		return models.Assignment{}, apperrors.BadRequest("providerMembershipId and providerId are required.", nil)
	}
	var replacement models.Assignment
	err := s.tx(ctx, func(tx pgx.Tx) error {
		original, err := s.opts.Repository.LockAssignment(ctx, tx, t.OrganizationID, id)
		if err != nil || !t.BranchPermitted(original.BranchID) {
			return nf()
		}
		if original.Status == models.StatusReassignment && original.ReplacementAssignmentID != "" {
			replacement, err = s.opts.Repository.GetAssignment(ctx, tx, t.OrganizationID, original.ReplacementAssignmentID)
			return err
		}
		if original.Status != models.StatusDeclined && original.Status != models.StatusExpired {
			return errorcodes.Conflict("private_assignment_not_reassignable", "Only declined or expired assignments may be reassigned.")
		}
		if in.ExpectedVersion <= 0 || original.Version != in.ExpectedVersion {
			return errorcodes.Conflict("version_conflict", "Assignment version changed.")
		}
		if s.opts.Eligibility == nil {
			return errorcodes.Unavailable("private_assignment_not_eligible", "Provider eligibility is unavailable.")
		}
		ok, validateErr := s.opts.Eligibility.Validate(ctx, t.OrganizationID, in.ProviderMembershipID, in.ProviderID, service, in.VehicleOrTruckReference)
		if validateErr != nil || !ok {
			return errorcodes.Conflict("private_assignment_not_eligible", "Provider or vehicle is not eligible.")
		}
		deadline := s.opts.Now().Add(s.opts.ResponseTTL)
		if in.ResponseDeadline != nil && in.ResponseDeadline.After(s.opts.Now()) {
			deadline = *in.ResponseDeadline
		}
		replacement = models.Assignment{
			ID: ids.New(repositories.PrefixAssignment), OrganizationID: original.OrganizationID,
			BranchID: original.BranchID, WorkOrigin: original.WorkOrigin,
			PrivateOrderID: original.PrivateOrderID, ShipmentID: original.ShipmentID,
			RouteStopID: original.RouteStopID, ProviderMembershipID: in.ProviderMembershipID,
			ProviderID: in.ProviderID, VehicleOrTruckReference: in.VehicleOrTruckReference,
			ExecutionService: service, Status: models.StatusPending,
			ResponseDeadline: deadline, CreatedBy: t.ActorID,
		}
		replacement, err = s.opts.Repository.CreateAssignment(ctx, tx, replacement)
		if err != nil {
			return err
		}
		updated, err := s.opts.Repository.SetAssignmentStatus(ctx, tx, original.OrganizationID, original.ID, original.Version, models.StatusReassignment, original.DeclineReason, original.DeclineNote, replacement.ID)
		if err != nil {
			return err
		}
		if err = s.opts.Repository.History(ctx, tx, original.OrganizationID, original.ID, original.Status, updated.Status, "REPLACEMENT_CREATED", string(t.ActorType), t.ActorID, t.CorrelationID); err != nil {
			return err
		}
		if err = s.opts.Repository.History(ctx, tx, replacement.OrganizationID, replacement.ID, "CREATED", replacement.Status, "REASSIGNED", string(t.ActorType), t.ActorID, t.CorrelationID); err != nil {
			return err
		}
		if err = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "private_assignments.reassigned", "private_assignment", original.ID)); err != nil {
			return err
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "private_assignment.reassigned.v1", "private_assignment", original.ID, updated.Version, original.OrganizationID, t.CorrelationID, map[string]any{"assignmentId": original.ID, "replacementAssignmentId": replacement.ID, "executionService": service})
		}
		return nil
	})
	return replacement, err
}

type RespondInput struct {
	ProviderID, Decision, ReasonCode, SafeNote, IdempotencyKey, RequestFingerprint string
	ExpectedVersion                                                                int64
}

func (s *Service) Respond(ctx context.Context, org, id string, in RespondInput) (models.Assignment, error) {
	if in.IdempotencyKey == "" || len(in.IdempotencyKey) > 128 || in.IdempotencyKey != strings.TrimSpace(in.IdempotencyKey) {
		return models.Assignment{}, apperrors.BadRequest("A bounded original idempotency key is required.", nil)
	}
	decision := strings.ToUpper(strings.TrimSpace(in.Decision))
	if decision != "ACCEPT" && decision != "DECLINE" {
		return models.Assignment{}, apperrors.BadRequest("decision must be ACCEPT or DECLINE.", nil)
	}
	// Decline is a safety boundary, not an execution write. A provider must be
	// able to refuse an issued assignment while execution is disabled.
	if decision == "ACCEPT" {
		if e := s.gate(false); e != nil {
			return models.Assignment{}, e
		}
	}
	if decision == "DECLINE" && (!models.ValidDecline(in.ReasonCode) || models.RequiresNote(in.ReasonCode) && strings.TrimSpace(in.SafeNote) == "") {
		return models.Assignment{}, apperrors.BadRequest("A valid decline reason and required note are required.", nil)
	}
	if prior, e := s.opts.Repository.GetDecision(ctx, nil, org, id); e == nil {
		if prior.ProviderID == in.ProviderID && prior.Decision == decision && prior.IdempotencyKey == in.IdempotencyKey && prior.RequestFingerprint == in.RequestFingerprint {
			a, _ := s.opts.Repository.GetAssignment(ctx, nil, org, id)
			return a, nil
		}
		return models.Assignment{}, errorcodes.Conflict("private_assignment_already_resolved", "Assignment already has a provider decision.")
	} else if !errors.Is(e, repositories.ErrNotFound) {
		return models.Assignment{}, e
	}
	var out models.Assignment
	e := s.tx(ctx, func(tx pgx.Tx) error {
		a, x := s.opts.Repository.LockAssignment(ctx, tx, org, id)
		if x != nil {
			return nf()
		}
		if a.ProviderID != in.ProviderID {
			return errorcodes.Forbidden("private_assignment_provider_mismatch", "Only the selected provider may respond.")
		}
		if a.Status != models.StatusPending {
			return errorcodes.Conflict("private_assignment_already_resolved", "Assignment is already resolved.")
		}
		if !s.opts.Now().Before(a.ResponseDeadline) {
			return errorcodes.Conflict("private_assignment_response_expired", "Assignment response deadline has passed.")
		}
		if in.ExpectedVersion > 0 && a.Version != in.ExpectedVersion {
			return errorcodes.Conflict("version_conflict", "Assignment version changed.")
		}
		if decision == "ACCEPT" {
			if a.ExecutionService == "TAXI" || !enabledService(s.opts.Flags, a.ExecutionService) {
				return errorcodes.Forbidden("private_execution_disabled", "Private execution is disabled for this service.")
			}
			if s.opts.Eligibility == nil {
				return errorcodes.Unavailable("private_assignment_not_eligible", "Eligibility is unavailable.")
			}
			ok, x := s.opts.Eligibility.Validate(ctx, org, a.ProviderMembershipID, a.ProviderID, a.ExecutionService, a.VehicleOrTruckReference)
			if x != nil || !ok {
				return errorcodes.Conflict("private_assignment_not_eligible", "Provider or vehicle is no longer eligible.")
			}
		}
		d := models.Decision{OrganizationID: org, AssignmentID: id, ProviderID: in.ProviderID, Decision: decision, ReasonCode: in.ReasonCode, SafeNote: strings.TrimSpace(in.SafeNote), SafetyProtected: models.SafetyProtected(in.ReasonCode), IdempotencyKey: in.IdempotencyKey, RequestFingerprint: in.RequestFingerprint}
		createdDecision, decisionErr := s.opts.Repository.CreateDecision(ctx, tx, d)
		x = decisionErr
		if x != nil {
			return errorcodes.Conflict("private_assignment_already_resolved", "Assignment is already resolved.")
		}
		status := models.StatusAccepted
		event := "private_assignment.accepted.v1"
		if decision == "DECLINE" {
			status = models.StatusDeclined
			event = "private_assignment.declined.v1"
		}
		out, x = s.opts.Repository.SetAssignmentStatus(ctx, tx, org, id, a.Version, status, in.ReasonCode, in.SafeNote, "")
		if x != nil {
			return x
		}
		if x = s.opts.Repository.History(ctx, tx, org, id, a.Status, status, in.ReasonCode, "provider", in.ProviderID, id); x != nil {
			return x
		}
		auditTenant := tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypePlatformService, ActorID: in.ProviderID, CorrelationID: id}
		if x = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(auditTenant, "private_assignments."+strings.ToLower(decision), "private_assignment", id)); x != nil {
			return x
		}
		if decision == "ACCEPT" {
			work, x := s.opts.Work.Resolve(ctx, tx, org, a.WorkOrigin, workID(a))
			if x != nil {
				return x
			}
			fp := hash(org, id, a.ProviderID, a.ExecutionService)
			execution, createErr := s.opts.Repository.CreateExecution(ctx, tx, models.ExecutionOperation{ID: ids.New(repositories.PrefixExecution), OrganizationID: org, SettlementOrganizationID: org, ExecutingProviderID: a.ProviderID, OrganizationMembershipID: a.ProviderMembershipID, PrivateOrderID: work.PrivateOrderID, ShipmentID: work.ShipmentID, AssignmentID: id, ProviderResponseID: createdDecision.ID, VehicleOrTruckReference: a.VehicleOrTruckReference, ContractReference: work.ContractReference, QuotationReference: work.QuotationReference, ExecutionService: a.ExecutionService, Status: models.ExecutionPending, IdempotencyKey: "private-execution-" + id, RequestFingerprint: fp, CorrelationID: id, CausationID: createdDecision.ID, Payload: work.ExecutionPayload})
			x = createErr
			if x != nil {
				return x
			}
			if s.opts.Events != nil {
				if x = s.opts.Events.Publish(ctx, tx, "private_execution.requested.v1", "private_execution", execution.ID, execution.Version, org, id, map[string]any{"executionId": execution.ID, "assignmentId": id, "executionService": a.ExecutionService}); x != nil {
					return x
				}
			}
		}
		if s.opts.Events != nil {
			if x = s.opts.Events.Publish(ctx, tx, event, "private_assignment", id, out.Version, org, id, map[string]any{"assignmentId": id, "providerId": a.ProviderID, "reasonCode": in.ReasonCode, "safetyProtected": models.SafetyProtected(in.ReasonCode)}); x != nil {
				return x
			}
			if decision == "DECLINE" {
				return s.opts.Events.Publish(ctx, tx, "private_assignment.reassignment_required.v1", "private_assignment", id, out.Version, org, id, map[string]any{"assignmentId": id, "automaticProviderSelection": false})
			}
		}
		return nil
	})
	return out, e
}
func workID(a models.Assignment) string {
	if a.PrivateOrderID != "" {
		return a.PrivateOrderID
	}
	if a.ShipmentID != "" {
		return a.ShipmentID
	}
	return a.RouteStopID
}

// GetProviderAssignment resolves an opaque assignment id without trusting an
// organization supplied by the provider app. The authenticated provider
// service and selected provider are checked against the stored assignment.
func (s *Service) GetProviderAssignment(ctx context.Context, id, providerID,
	executionService string) (models.Assignment, error) {
	assignment, err := s.opts.Repository.GetAssignmentByID(ctx, nil, id)
	if err != nil || assignment.ProviderID != providerID || assignment.ExecutionService != executionService {
		return models.Assignment{}, nf()
	}
	return assignment, nil
}

func (s *Service) ReplayExecution(ctx context.Context, t tenant.Context, id string) error {
	if err := s.gate(true); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := s.opts.Repository.ReplayExecution(ctx, tx, t.OrganizationID, id); err != nil {
			return errorcodes.Conflict("private_execution_not_replayable", "Execution is not safely replayable; reconcile unknown outcomes first.")
		}
		return s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t,
			"private_executions.replayed", "private_execution", id))
	})
}

func (s *Service) ReplayReconciliation(ctx context.Context, t tenant.Context, id string) error {
	if err := s.gate(true); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := s.opts.Repository.ReplayReconciliation(ctx, tx, t.OrganizationID, id); err != nil {
			return errorcodes.Conflict("private_reconciliation_not_replayable", "Reconciliation case is not replayable.")
		}
		return s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t,
			"private_execution_reconciliations.replayed", "private_execution_reconciliation", id))
	})
}
func hash(v ...string) string {
	x := sha256.Sum256([]byte(strings.Join(v, "\x00")))
	return hex.EncodeToString(x[:])
}
func (s *Service) Cancel(ctx context.Context, t tenant.Context, id string, version int64) (models.Assignment, error) {
	if e := s.gate(true); e != nil {
		return models.Assignment{}, e
	}
	var out models.Assignment
	e := s.tx(ctx, func(tx pgx.Tx) error {
		a, x := s.opts.Repository.LockAssignment(ctx, tx, t.OrganizationID, id)
		if x != nil || !t.BranchPermitted(a.BranchID) {
			return nf()
		}
		if a.Status != models.StatusPending && a.Status != models.StatusAccepted && a.Status != models.StatusReady {
			return errorcodes.Conflict("private_assignment_already_resolved", "Only pending, accepted, or execution-ready assignments can be cancelled.")
		}
		if version <= 0 {
			return errorcodes.Conflict("version_conflict", "expectedVersion is required.")
		}
		if a.Status == models.StatusAccepted || a.Status == models.StatusReady {
			if x = s.opts.Repository.RequestExecutionCancellation(ctx, tx, t.OrganizationID, id); x != nil {
				return errorcodes.Conflict("private_execution_cancellation_required", "Execution cancellation could not be scheduled safely.")
			}
		}
		out, x = s.opts.Repository.SetAssignmentStatus(ctx, tx, t.OrganizationID, id, version, models.StatusCancelled, "COMPANY_CANCELLED", "", "")
		if x != nil {
			return x
		}
		if x = s.opts.Repository.History(ctx, tx, t.OrganizationID, id, a.Status, models.StatusCancelled, "COMPANY_CANCELLED", string(t.ActorType), t.ActorID, t.CorrelationID); x != nil {
			return x
		}
		if x = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "private_assignments.cancelled", "private_assignment", id)); x != nil {
			return x
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "private_assignment.cancelled.v1", "private_assignment", id, out.Version, t.OrganizationID, t.CorrelationID, map[string]any{"assignmentId": id, "executionCancellationRequested": a.Status == models.StatusAccepted || a.Status == models.StatusReady})
		}
		return nil
	})
	return out, e
}
func (s *Service) ExpirePass(ctx context.Context, limit int) (int, error) {
	if !s.opts.Flags.WorkerEnabled || s.opts.Flags.KillSwitch {
		return 0, nil
	}
	count := 0
	e := s.tx(ctx, func(tx pgx.Tx) error {
		items, x := s.opts.Repository.ClaimExpired(ctx, tx, limit)
		if x != nil {
			return x
		}
		for _, a := range items {
			updated, x := s.opts.Repository.SetAssignmentStatus(ctx, tx, a.OrganizationID, a.ID, a.Version, models.StatusExpired, "RESPONSE_DEADLINE", "", "")
			if x != nil {
				return x
			}
			if x = s.opts.Repository.History(ctx, tx, a.OrganizationID, a.ID, a.Status, models.StatusExpired, "RESPONSE_DEADLINE", "system", "assignment-expiry-worker", a.ID); x != nil {
				return x
			}
			systemTenant := tenant.Context{OrganizationID: a.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "assignment-expiry-worker", CorrelationID: a.ID}
			if x = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(systemTenant, "private_assignments.expired", "private_assignment", a.ID)); x != nil {
				return x
			}
			if s.opts.Events != nil {
				if x = s.opts.Events.Publish(ctx, tx, "private_assignment.expired.v1", "private_assignment", a.ID, updated.Version, a.OrganizationID, a.ID, map[string]any{"assignmentId": a.ID}); x != nil {
					return x
				}
				if x = s.opts.Events.Publish(ctx, tx, "private_assignment.reassignment_required.v1", "private_assignment", a.ID, updated.Version, a.OrganizationID, a.ID, map[string]any{"assignmentId": a.ID, "automaticProviderSelection": false}); x != nil {
					return x
				}
			}
			count++
		}
		return nil
	})
	return count, e
}
