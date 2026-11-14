// Package usecases owns the compliance workflows: requirements, evaluations,
// checks, restriction application and propagation, and the Section 19
// workers. Persistence and the execution authorities are reached through the
// ports declared here.
package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
)

// EventPublisher appends domain events to the outbox inside a transaction.
type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

// Repository is the compliance persistence port. Methods taking a
// database.Querier join the caller's transaction.
type Repository interface {
	CreateRequirement(context.Context, database.Querier, models.Requirement) (models.Requirement, error)
	GetRequirement(ctx context.Context, org, id string) (models.Requirement, error)
	ListRequirements(ctx context.Context, org string) ([]models.Requirement, error)
	ListActiveRequirements(ctx context.Context, q database.Querier, org, subjectType string) ([]models.Requirement, error)
	UpdateRequirement(ctx context.Context, q database.Querier, org, id string, expected int64, in models.Requirement, validityRules json.RawMessage, severity string, blocksOperation bool, status string) (models.Requirement, error)
	QualificationEvidence(ctx context.Context, org, membership, qualification string, now time.Time) (*time.Time, bool)
	TrainingEvidence(ctx context.Context, org, membership, course string, now time.Time) (*time.Time, bool)
	AssetDocumentEvidence(ctx context.Context, org, asset, documentType string, now time.Time) (*time.Time, bool)
	OrganizationActive(ctx context.Context, org string) bool
	LockStatus(ctx context.Context, q database.Querier, org, subjectType, subjectReference, requirementID string) (string, int64, error)
	InsertStatus(context.Context, database.Querier, models.Status) (models.Status, error)
	UpdateStatus(context.Context, database.Querier, models.Status) (models.Status, error)
	SetStatusRestriction(ctx context.Context, q database.Querier, statusID, restrictionID string) error
	InsertStatusHistory(ctx context.Context, q database.Querier, previousState string, v models.Status, actor string) error
	ListStatuses(ctx context.Context, org, subjectType, subjectReference string) ([]models.Status, error)
	SweepStatuses(ctx context.Context, org string) error
	UpsertRestriction(context.Context, database.Querier, models.Restriction) (models.Restriction, error)
	InsertRestrictionHistory(ctx context.Context, q database.Querier, org, restrictionID, from, to, reasonCode, actor string, version int64, snapshot any) error
	InsertRestrictionOperation(ctx context.Context, q database.Querier, org, restrictionID, authority, action, key string, version int64) error
	GetRestriction(ctx context.Context, org, id string) (models.Restriction, error)
	RequestClear(ctx context.Context, q database.Querier, org, id string, expected int64, actor, reason string) (models.Restriction, error)
	MarkCleared(ctx context.Context, q database.Querier, org, id string) (models.Restriction, error)
	CountActiveRestrictions(ctx context.Context, org, subjectType, subjectReference string) (int, error)
	ListRestrictions(ctx context.Context, org, subjectType, subjectReference, state string) ([]models.Restriction, error)
	PendingOperations(ctx context.Context, org, restrictionID string) ([]models.RestrictionOperation, error)
	RecordOperationAttempt(ctx context.Context, q database.Querier, org string, op models.RestrictionOperation, attempt int, outcome, status, code string, authorityVersion int64, started, completed time.Time) error
	CountUnconfirmedOperations(ctx context.Context, restrictionID string) int
	ListPropagation(ctx context.Context, org, restrictionID string) ([]models.Propagation, error)
	DueRestrictionIDs(ctx context.Context, org string) ([]string, error)
	ActiveOrganizations(ctx context.Context) ([]string, error)
	SubjectBranch(ctx context.Context, q database.Querier, org, subjectType, ref string) (string, bool, error)
	InsertCheck(context.Context, database.Querier, models.Check) (time.Time, error)
	GetCheck(ctx context.Context, org, id string) (models.Check, error)
	ListChecks(ctx context.Context, org, subjectType, subjectReference string) ([]models.Check, error)
}

// Options wires the service. Repository is required.
type Options struct {
	Pool       *pgxpool.Pool
	Repository Repository
	Events     EventPublisher
	Audit      *audit.Recorder
	Queue      *jobs.Queue
	Dispatch   platformclients.RestrictionAuthority
	Hauling    platformclients.RestrictionAuthority
	// Taxi is driver-taxi-service's restriction authority. Unlike Dispatch
	// and Hauling, which are required targets in every deployment, Taxi
	// becomes a propagation target only when this deployment wires it: a
	// deployment with no Taxi integration must not leave every restriction
	// permanently short of FullyPropagated. Once Taxi is a target its
	// operations are processed exactly like the others, including the
	// retryable AUTHORITY_NOT_CONFIGURED outcome of an unconfigured client.
	Taxi  platformclients.RestrictionAuthority
	Flags models.Flags
	Now   func() time.Time
}

// Service implements the compliance workflows.
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
	return &Service{o: o, repo: o.Repository}
}

func (s *Service) gate(write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet operations are disabled.", nil)
	}
	if !s.o.Flags.Compliance {
		if write {
			return apperrors.Forbidden("Compliance capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}

func (s *Service) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, fn)
}
func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error {
	if err := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); err != nil {
		return err
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, version, t.OrganizationID, t.CorrelationID, payload)
	}
	return nil
}

func (s *Service) CreateRequirement(ctx context.Context, t tenant.Context, v models.Requirement) (models.Requirement, error) {
	if err := s.gate(true); err != nil {
		return v, err
	}
	models.NormalizeRequirement(&v)
	if v.Code == "" || v.Name == "" || !models.ValidSubject(v.SubjectType) || v.RequirementType == "" || v.Severity == "" {
		return v, apperrors.BadRequest("Code, name, subject type, requirement type and severity are required.", nil)
	}
	v.ID = ids.New("cmpreq")
	v.OrganizationID = t.OrganizationID
	v.CreatedBy = t.ActorID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var e error
		v, e = s.repo.CreateRequirement(ctx, tx, v)
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "compliance_requirement.created.v1", "compliance_requirement", v.ID, v.Version, v)
	})
	return v, err
}
func (s *Service) GetRequirement(ctx context.Context, t tenant.Context, id string) (models.Requirement, error) {
	if err := s.gate(false); err != nil {
		return models.Requirement{}, err
	}
	v, e := s.repo.GetRequirement(ctx, t.OrganizationID, id)
	if errors.Is(e, models.ErrNotFound) {
		e = apperrors.NotFound("Requirement not found.", nil)
	}
	return v, e
}
func (s *Service) ListRequirements(ctx context.Context, t tenant.Context) ([]models.Requirement, error) {
	if err := s.gate(false); err != nil {
		return nil, err
	}
	return s.repo.ListRequirements(ctx, t.OrganizationID)
}
func (s *Service) UpdateRequirement(ctx context.Context, t tenant.Context, id string, expected int64, in models.Requirement, action string) (models.Requirement, error) {
	if err := s.gate(true); err != nil {
		return in, err
	}
	if expected <= 0 {
		return in, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	old, e := s.GetRequirement(ctx, t, id)
	if e != nil {
		return in, e
	}
	status := old.Status
	blocksOperation := in.BlocksOperation
	event := "compliance_requirement.updated.v1"
	switch action {
	case "ACTIVATE":
		status = "ACTIVE"
		blocksOperation = old.BlocksOperation
		event = "compliance_requirement.activated.v1"
	case "DEACTIVATE":
		status = "INACTIVE"
		blocksOperation = old.BlocksOperation
		event = "compliance_requirement.deactivated.v1"
	}
	if old.Status == "ARCHIVED" {
		return in, apperrors.Conflict("Archived requirements cannot be changed.", nil)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		in, x = s.repo.UpdateRequirement(ctx, tx, t.OrganizationID, id, expected, in, orJSON(in.ValidityRules), strings.ToUpper(in.Severity), blocksOperation, status)
		if errors.Is(x, models.ErrNotFound) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, event, "compliance_requirement", id, in.Version, in)
	})
	return in, e
}

func (s *Service) evidence(ctx context.Context, org string, r models.Requirement, subject string) (string, []string, *time.Time) {
	now := s.o.Now().UTC()
	var ok bool
	var expires *time.Time
	switch {
	case r.RequiredQualificationID != "" && r.SubjectType == "PROVIDER":
		expires, ok = s.repo.QualificationEvidence(ctx, org, subject, r.RequiredQualificationID, now)
	case r.RequiredTrainingCourseID != "" && r.SubjectType == "PROVIDER":
		expires, ok = s.repo.TrainingEvidence(ctx, org, subject, r.RequiredTrainingCourseID, now)
	case r.RequiredDocumentType != "" && r.SubjectType == "ASSET":
		expires, ok = s.repo.AssetDocumentEvidence(ctx, org, subject, r.RequiredDocumentType, now)
	case r.SubjectType == "ORGANIZATION":
		ok = s.repo.OrganizationActive(ctx, org)
	default:
		return models.StateUnknown, []string{"AUTHORITATIVE_EVIDENCE_UNAVAILABLE"}, nil
	}
	if !ok {
		return models.StateNonCompliant, []string{"REQUIRED_EVIDENCE_MISSING"}, expires
	}
	if expires != nil && expires.Before(now.Add(30*24*time.Hour)) {
		return models.StateDueSoon, []string{"EVIDENCE_DUE_SOON"}, expires
	}
	return models.StateCompliant, []string{"REQUIREMENT_SATISFIED"}, expires
}

func (s *Service) Evaluate(ctx context.Context, t tenant.Context, in models.EvaluateInput) (models.Status, error) {
	if err := s.gate(true); err != nil {
		return models.Status{}, err
	}
	if in.SubjectReference == "" || in.RequirementID == "" || in.SourceType == "" || in.SourceVersion < 0 {
		return models.Status{}, apperrors.BadRequest("Subject, requirement, source and sourceVersion are required.", nil)
	}
	r, e := s.GetRequirement(ctx, t, in.RequirementID)
	if e != nil {
		return models.Status{}, e
	}
	if !r.ActiveAt(s.o.Now()) {
		return models.Status{}, apperrors.Conflict("Requirement is not active.", nil)
	}
	if strings.ToUpper(in.SubjectType) != r.SubjectType {
		return models.Status{}, apperrors.BadRequest("Subject type does not match requirement.", nil)
	}
	var out models.Status
	var restrictionID string
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, restrictionID, err = s.evaluateTx(ctx, tx, t, r, in)
		return err
	})
	if e == nil && restrictionID != "" {
		_ = s.ProcessRestriction(ctx, t.OrganizationID, restrictionID)
	}
	return out, e
}

// evaluateTx evaluates one active requirement for one subject inside the
// caller's transaction, applying a restriction when a blocking requirement is
// not met. It returns the status and the ID of any restriction it applied;
// the caller propagates that restriction after commit.
func (s *Service) evaluateTx(ctx context.Context, tx pgx.Tx, t tenant.Context, r models.Requirement, in models.EvaluateInput) (models.Status, string, error) {
	state, reasons, expires := s.evidence(ctx, t.OrganizationID, r, in.SubjectReference)
	if state == models.StateNonCompliant && r.BlocksOperation {
		state = models.StateRestricted
	}
	reasonJSON, _ := json.Marshal(reasons)
	out := models.Status{ID: ids.New("cmpst"), OrganizationID: t.OrganizationID, SubjectType: r.SubjectType, SubjectReference: in.SubjectReference, RequirementID: r.ID, State: state, ExpiresAt: expires, SourceType: strings.ToUpper(in.SourceType), SourceReference: in.SourceReference, SourceVersion: in.SourceVersion, EvaluatedAt: s.o.Now().UTC(), ReasonCodes: reasonJSON}
	var restrictionID string
	oldState, oldSource, err := s.repo.LockStatus(ctx, tx, t.OrganizationID, out.SubjectType, out.SubjectReference, out.RequirementID)
	if err == nil && in.SourceVersion < oldSource {
		return out, "", apperrors.Conflict("A newer evidence version has already been evaluated.", nil)
	}
	if err != nil && !errors.Is(err, models.ErrNotFound) {
		return out, "", err
	}
	if errors.Is(err, models.ErrNotFound) {
		out, err = s.repo.InsertStatus(ctx, tx, out)
	} else {
		out.ID = ""
		out, err = s.repo.UpdateStatus(ctx, tx, out)
	}
	if err != nil {
		return out, "", err
	}
	if out.State == models.StateRestricted {
		rr, er := s.createRestrictionTx(ctx, tx, t, models.RestrictInput{SubjectType: out.SubjectType, SubjectReference: out.SubjectReference, ReasonCode: "COMPLIANCE_" + r.Code, SourceType: "COMPLIANCE_STATUS", SourceReference: out.ID, Severity: r.Severity})
		if er != nil {
			return out, "", er
		}
		restrictionID = rr.ID
		out.RestrictionReference = rr.ID
		if er = s.repo.SetStatusRestriction(ctx, tx, out.ID, rr.ID); er != nil {
			return out, "", er
		}
	}
	if err = s.repo.InsertStatusHistory(ctx, tx, oldState, out, t.ActorID); err != nil {
		return out, "", err
	}
	return out, restrictionID, s.record(ctx, tx, t, "compliance_status.evaluated.v1", "compliance_status", out.ID, out.Version, out)
}

func (s *Service) createRestrictionTx(ctx context.Context, tx pgx.Tx, t tenant.Context, in models.RestrictInput) (models.Restriction, error) {
	auth := s.requiredAuthorities(strings.ToUpper(in.SubjectType))
	raw, _ := json.Marshal(auth)
	v := models.Restriction{ID: ids.New("rst"), OrganizationID: t.OrganizationID, SubjectType: strings.ToUpper(in.SubjectType), SubjectReference: in.SubjectReference, ReasonCode: strings.ToUpper(in.ReasonCode), SourceType: in.SourceType, SourceReference: in.SourceReference, Severity: strings.ToUpper(in.Severity), State: "APPLIED", RequiredAuthorities: raw, CreatedBy: t.ActorID}
	var e error
	v, e = s.repo.UpsertRestriction(ctx, tx, v)
	if e != nil {
		return v, e
	}
	if e = s.repo.InsertRestrictionHistory(ctx, tx, v.OrganizationID, v.ID, "", "APPLIED", v.ReasonCode, t.ActorID, v.Version, v); e != nil {
		return v, e
	}
	for _, a := range auth {
		if e = s.repo.InsertRestrictionOperation(ctx, tx, v.OrganizationID, v.ID, a, "APPLY", v.ID+":apply", v.Version); e != nil {
			return v, e
		}
	}
	if e = s.record(ctx, tx, t, "restriction.applied.v1", "restriction", v.ID, v.Version, v); e != nil {
		return v, e
	}
	if len(auth) > 0 && s.o.Events != nil {
		if e = s.o.Events.Publish(ctx, tx, "restriction.propagation_required.v1", "restriction", v.ID, v.Version, v.OrganizationID, t.CorrelationID, map[string]any{"restrictionId": v.ID, "action": "APPLY", "authorities": auth}); e != nil {
			return v, e
		}
	}
	return v, nil
}
func (s *Service) ApplyRestriction(ctx context.Context, t tenant.Context, in models.RestrictInput) (models.Restriction, error) {
	if !s.o.Flags.Restrictions {
		return models.Restriction{}, apperrors.Forbidden("Restriction propagation is disabled.", nil)
	}
	if in.SubjectReference == "" || in.ReasonCode == "" || in.SourceReference == "" {
		return models.Restriction{}, apperrors.BadRequest("Subject, reason and source are required.", nil)
	}
	var v models.Restriction
	e := s.tx(ctx, func(tx pgx.Tx) error { var x error; v, x = s.createRestrictionTx(ctx, tx, t, in); return x })
	if e != nil {
		return v, e
	}
	// The restriction is committed, enforced locally and its propagation is
	// durable (operations + outbox). A propagation failure is reconciled by
	// the worker, so it does not turn a recorded restriction into an error.
	_ = s.ProcessRestriction(ctx, t.OrganizationID, v.ID)
	return v, nil
}
func (s *Service) GetRestriction(ctx context.Context, org, id string) (models.Restriction, error) {
	v, e := s.repo.GetRestriction(ctx, org, id)
	if errors.Is(e, models.ErrNotFound) {
		e = apperrors.NotFound("Restriction not found.", nil)
	}
	return v, e
}
func (s *Service) ClearRestriction(ctx context.Context, t tenant.Context, id, reason string, expected int64) (models.Restriction, error) {
	if !s.o.Flags.Restrictions {
		return models.Restriction{}, apperrors.Forbidden("Restriction propagation is disabled.", nil)
	}
	if expected <= 0 || strings.TrimSpace(reason) == "" {
		return models.Restriction{}, apperrors.BadRequest("expectedVersion and clearance reason are required.", nil)
	}
	var v models.Restriction
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.RequestClear(ctx, tx, t.OrganizationID, id, expected, t.ActorID, reason)
		if errors.Is(x, models.ErrNotFound) {
			return apperrors.Conflict("Version conflict or restriction is not active.", nil)
		}
		if x != nil {
			return x
		}
		var auth []string
		_ = json.Unmarshal(v.RequiredAuthorities, &auth)
		for _, a := range auth {
			if x = s.repo.InsertRestrictionOperation(ctx, tx, t.OrganizationID, id, a, "CLEAR", id+":clear", v.Version); x != nil {
				return x
			}
		}
		if x = s.repo.InsertRestrictionHistory(ctx, tx, t.OrganizationID, id, "APPLIED", "CLEAR_PENDING", reason, t.ActorID, v.Version, v); x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, "restriction.clear_requested.v1", "restriction", id, v.Version, v); x != nil {
			return x
		}
		if len(auth) > 0 && s.o.Events != nil {
			return s.o.Events.Publish(ctx, tx, "restriction.propagation_required.v1", "restriction", id, v.Version, t.OrganizationID, t.CorrelationID, map[string]any{"restrictionId": id, "action": "CLEAR", "authorities": auth})
		}
		return nil
	})
	if e != nil {
		return v, e
	}
	_ = s.ProcessRestriction(ctx, t.OrganizationID, id)
	if current, x := s.GetRestriction(ctx, t.OrganizationID, id); x == nil {
		v = current
	}
	return v, nil
}
func (s *Service) CheckSubject(ctx context.Context, org, typ, ref string) (models.Eligibility, error) {
	count, err := s.repo.CountActiveRestrictions(ctx, org, strings.ToUpper(typ), ref)
	if err != nil {
		return models.Eligibility{}, err
	}
	v := models.Eligibility{Eligible: count == 0}
	if count > 0 {
		v.HardBlocks = []string{"COMPLIANCE_RESTRICTION_ACTIVE"}
	}
	return v, nil
}
func (s *Service) ListStatuses(ctx context.Context, t tenant.Context, typ, ref string) ([]models.Status, error) {
	return s.repo.ListStatuses(ctx, t.OrganizationID, strings.ToUpper(typ), ref)
}
func orJSON(v json.RawMessage) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage(`{}`)
	}
	return v
}

func clientOutcome(err error) platformclients.Outcome {
	var pe *platformclients.Error
	if errors.As(err, &pe) {
		return pe.Outcome
	}
	return platformclients.OutcomePermanent
}
func (s *Service) authority(name string) platformclients.RestrictionAuthority {
	if name == "DISPATCH" {
		return s.o.Dispatch
	}
	if name == "HAULING" {
		return s.o.Hauling
	}
	if name == models.AuthorityTaxi {
		return s.o.Taxi
	}
	return nil
}

// requiredAuthorities is RequiredAuthorities plus Taxi when this deployment
// wires a Taxi restriction authority (see Options.Taxi).
func (s *Service) requiredAuthorities(subject string) []string {
	auth := models.RequiredAuthorities(subject)
	if len(auth) > 0 && s.o.Taxi != nil {
		auth = append(auth, models.AuthorityTaxi)
	}
	return auth
}
func (s *Service) ProcessRestriction(ctx context.Context, org, id string) error {
	r, e := s.GetRestriction(ctx, org, id)
	if e != nil {
		return e
	}
	ops, e := s.repo.PendingOperations(ctx, org, id)
	if e != nil {
		return e
	}
	for _, o := range ops {
		c := s.authority(o.Authority)
		meta := platformclients.CallMeta{OrganizationID: org, IdempotencyKey: o.IdempotencyKey}
		cmd := platformclients.RestrictionCommand{RestrictionID: r.ID, OrganizationID: org, SubjectType: r.SubjectType, SubjectReference: r.SubjectReference, ReasonCode: r.ReasonCode, SourceReference: r.SourceReference, ExpectedVersion: o.ExpectedVersion}
		started := s.o.Now()
		var out platformclients.RestrictionOutcome
		var callErr error
		unconfigured := c == nil || !c.Configured()
		if unconfigured {
			// The restriction is already durable and enforced locally
			// (CheckSubject). An authority that is not reachable from this
			// deployment is a retryable propagation failure, recorded and
			// reconciled like any other -- never a reason to stop
			// propagating to the remaining authorities.
			callErr = &platformclients.Error{Service: strings.ToLower(o.Authority), Outcome: platformclients.OutcomeTransient}
		} else if o.Status == "OUTCOME_UNKNOWN" {
			if o.Action == "APPLY" {
				out, callErr = c.Read(ctx, meta, o.IdempotencyKey)
			} else {
				out, callErr = c.ReadClear(ctx, meta, o.IdempotencyKey)
			}
			if callErr == nil && ((o.Action == "APPLY" && out.State != "APPLIED") || (o.Action == "CLEAR" && out.State != "CLEARED")) {
				callErr = errors.New("restriction read-back state mismatch")
			}
		}
		// An ambiguous mutation is never resubmitted merely because its read-back
		// failed. Only an authoritative NOT_FOUND proves the original operation
		// did not land and makes an idempotent resubmission safe.
		shouldSubmit := o.Status != "OUTCOME_UNKNOWN" && !unconfigured
		if !unconfigured && o.Status == "OUTCOME_UNKNOWN" && clientOutcome(callErr) == platformclients.OutcomeNotFound {
			shouldSubmit = true
		}
		if shouldSubmit {
			if o.Action == "APPLY" {
				out, callErr = c.Apply(ctx, meta, cmd)
			} else {
				out, callErr = c.Clear(ctx, meta, cmd)
			}
		}
		outcome := "CONFIRMED"
		status := "CONFIRMED"
		code := ""
		if callErr != nil {
			oc := clientOutcome(callErr)
			outcome = string(oc)
			status = "RETRY_SCHEDULED"
			if oc == platformclients.OutcomeUnknown {
				status = "OUTCOME_UNKNOWN"
			}
			code = string(oc)
			if unconfigured {
				outcome, code = "NOT_CONFIGURED", "AUTHORITY_NOT_CONFIGURED"
			}
		}
		attempt := o.Attempts + 1
		e = s.tx(ctx, func(tx pgx.Tx) error {
			if x := s.repo.RecordOperationAttempt(ctx, tx, org, o, attempt, outcome, status, code, out.SourceVersion, started, s.o.Now()); x != nil {
				return x
			}
			if callErr == nil && s.o.Events != nil {
				return s.o.Events.Publish(ctx, tx, "restriction.propagation_confirmed.v1", "restriction", r.ID, r.Version, org, "", map[string]any{"restrictionId": r.ID, "authority": o.Authority, "action": o.Action, "authorityVersion": out.SourceVersion})
			}
			return nil
		})
		if e != nil {
			return e
		}
		if callErr != nil {
			_, _ = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: models.JobRestrictionReconcile, DedupeKey: o.ID, Payload: map[string]string{"organizationId": org, "restrictionId": id}, MaxAttempts: 8})
			continue
		}
	}
	pending := s.repo.CountUnconfirmedOperations(ctx, id)
	if pending == 0 && r.State == "CLEAR_PENDING" {
		e = s.tx(ctx, func(tx pgx.Tx) error {
			cleared, x := s.repo.MarkCleared(ctx, tx, org, id)
			if errors.Is(x, models.ErrNotFound) {
				return nil
			}
			if x != nil {
				return x
			}
			actor := cleared.ClearedBy
			if actor == "" {
				actor = "restriction-reconciliation-worker"
			}
			if x = s.repo.InsertRestrictionHistory(ctx, tx, org, id, "CLEAR_PENDING", "CLEARED", cleared.ClearanceReason, actor, cleared.Version, cleared); x != nil {
				return x
			}
			return s.record(ctx, tx, tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeSystem, ActorID: actor}, "restriction.cleared.v1", "restriction", id, cleared.Version, cleared)
		})
	}
	return e
}
