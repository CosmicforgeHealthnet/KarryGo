// Package usecases implements the approval workflow for controlled
// operations.
//
// The framework's job is to make one sentence true at execution time: "a
// sufficient number of eligible people, none of whom was the requester when
// policy forbids it, approved EXACTLY this operation against EXACTLY this
// version of the subject, and it has not been spent yet."
//
// Every rule here exists to keep some part of that sentence honest. Drop the
// fingerprint check and "exactly this operation" becomes "something like it".
// Drop the subject version and it becomes "against some earlier world". Drop
// the execution reference and "not been spent yet" becomes unenforceable.
package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Corporate events emitted by the approval workflow.
const (
	EventApprovalRequested   = "organization.approval.requested.v1"
	EventApprovalDecided     = "organization.approval.decided.v1"
	EventApprovalInvalidated = "organization.approval.invalidated.v1"
	EventApprovalExecuted    = "organization.approval.executed.v1"
)

// EventPublisher appends corporate domain events to the outbox.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string,
		aggregateID string, aggregateVersion int64, organizationID string,
		correlationID string, payload any) error
}

// RoleReader exposes a member's live role bindings.
type RoleReader interface {
	ListBindings(ctx context.Context, organizationID string, memberID string) ([]staffmodels.RoleBinding, error)
}

// SubjectVersionReader reports the current version of a controlled subject.
//
// The approval framework cannot know how every subject stores its version, so
// it asks. A resource type with no reader registered cannot be version-bound,
// and a policy that requires version binding for such a type refuses rather
// than silently skipping the check.
type SubjectVersionReader interface {
	SubjectVersion(ctx context.Context, q database.Querier, organizationID string,
		resourceType string, resourceID string) (int64, error)
}

// Options configure the approval service.
type Options struct {
	Pool        *pgxpool.Pool
	Approvals   *repositories.Repository
	Eligibility *eligibilityusecases.Service
	Roles       RoleReader
	Subjects    SubjectVersionReader
	Events      EventPublisher
	Audit       *audit.Recorder
	Metrics     *observability.Metrics
	Now         func() time.Time
}

// Service runs the approval workflow.
type Service struct {
	opts Options
}

// NewService builds the service.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}
	if opts.Audit == nil {
		opts.Audit = audit.NewRecorder()
	}
	return &Service{opts: opts}
}

// Actor is the authenticated caller.
type Actor struct {
	OrganizationID     string
	OrganizationUserID string
	BranchID           string
	ActorType          string
	ActorID            string
}

// RequestInput asks for approval of a controlled operation.
type RequestInput struct {
	Actor        Actor
	ResourceType string
	ResourceID   string
	Action       string
	// OperationType names the eligibility policy governing this operation.
	OperationType string
	// Payload is the exact controlled action. It is hashed into the
	// fingerprint execution must later present.
	Payload map[string]any
	// ExpectedSubjectVersion is the version the requester believes the subject
	// is at. Zero means unstated.
	ExpectedSubjectVersion int64
	// EligibilityDecisionID binds the request to the decision that authorized
	// raising it.
	EligibilityDecisionID string

	IdempotencyKey string
	CorrelationID  string
	CausationID    string
}

// RequestResult is what a caller learns.
type RequestResult struct {
	Request  *models.Request
	Replayed bool
}

// Request raises an approval request.
//
// A retried identical request replays. A reused idempotency key with different
// content is a conflict, not a silent overwrite — the second caller believes
// something different about what is being approved and must be told.
func (s *Service) Request(ctx context.Context, input RequestInput) (*RequestResult, error) {
	if err := s.validateRequest(&input); err != nil {
		return nil, err
	}

	fingerprint, err := PayloadFingerprint(input.Actor.OrganizationID, input.ResourceType,
		input.ResourceID, input.Action, input.Payload)
	if err != nil {
		return nil, err
	}

	if replay, err := s.findRequestReplay(ctx, input, fingerprint); err != nil {
		return nil, err
	} else if replay != nil {
		return replay, nil
	}

	policy, err := s.opts.Eligibility.Policy(ctx, input.Actor.OrganizationID, input.OperationType)
	if err != nil {
		return nil, err
	}
	if !policy.ApprovalRequired {
		return nil, errorcodes.UnprocessableEntity(errorcodes.ApprovalPolicyMissing,
			"This operation does not require approval.")
	}

	now := s.opts.Now().UTC()
	request := &models.Request{
		OrganizationID:         input.Actor.OrganizationID,
		ResourceType:           input.ResourceType,
		ResourceID:             input.ResourceID,
		Action:                 input.Action,
		RequesterID:            input.Actor.OrganizationUserID,
		RequestFingerprint:     fingerprint,
		PayloadVersion:         payloadVersion,
		PayloadSnapshot:        input.Payload,
		ExpectedSubjectVersion: input.ExpectedSubjectVersion,
		MinimumApprovals:       policy.MinimumApprovals,
		EligibleRoleIDs:        policy.EligibleRoleIDs,
		SelfApprovalAllowed:    policy.SelfApprovalAllowed,
		SeparationOfDuties:     policy.SeparationOfDuties,
		PolicyVersion:          policy.Version,
		ExpiresAt:              now.Add(policy.ApprovalTTL()),
		IdempotencyKey:         input.IdempotencyKey,
		CorrelationID:          input.CorrelationID,
		CausationID:            input.CausationID,
	}
	if input.Actor.BranchID != "" {
		branchID := input.Actor.BranchID
		request.BranchID = &branchID
	}
	if input.EligibilityDecisionID != "" {
		decisionID := input.EligibilityDecisionID
		request.EligibilityDecisionID = &decisionID
	}

	// Record the subject version now if the requester did not state one, so
	// the approval is bound to a world even when the client is careless.
	if request.ExpectedSubjectVersion == 0 && s.opts.Subjects != nil {
		observed, versionErr := s.opts.Subjects.SubjectVersion(ctx, nil,
			input.Actor.OrganizationID, input.ResourceType, input.ResourceID)
		if versionErr == nil {
			request.ExpectedSubjectVersion = observed
		}
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		policyID, policyVersion, policyErr := s.opts.Approvals.EnsurePolicy(ctx, tx,
			input.Actor.OrganizationID, input.ResourceType, input.Action,
			repositories.PolicyTemplate{
				Name:                       input.OperationType,
				Quorum:                     policy.MinimumApprovals,
				EligibleRoleIDs:            policy.EligibleRoleIDs,
				RequesterMayApprove:        policy.SelfApprovalAllowed,
				ExpirySeconds:              policy.ApprovalTTLSeconds,
				SeparationOfDuties:         policy.SeparationOfDuties,
				EligibilityPolicyKey:       policy.PolicyKey,
				RequiresCurrentEligibility: true,
				Version:                    policy.Version,
			})
		if policyErr != nil {
			return policyErr
		}
		request.PolicyID = policyID
		if policyVersion > 0 {
			request.PolicyVersion = policyVersion
		}

		// An open request for the same resource and action whose payload has
		// changed is invalidated rather than left to race this one. Two live
		// approvals for the same operation would let a caller execute the one
		// that suits it.
		if existing, findErr := s.opts.Approvals.FindOpenForResource(ctx, tx,
			input.Actor.OrganizationID, input.ResourceType, input.ResourceID,
			input.Action); findErr == nil {
			if existing.RequestFingerprint == fingerprint {
				return errorcodes.Conflict(errorcodes.ApprovalConflictingDecision,
					"An approval request for this operation is already open.")
			}
			if invalidateErr := s.invalidate(ctx, tx, existing,
				models.InvalidationSupersededByRequest, input.Actor); invalidateErr != nil {
				return invalidateErr
			}
		} else if !errors.Is(findErr, repositories.ErrNotFound) {
			return findErr
		}

		if createErr := s.opts.Approvals.CreateRequest(ctx, tx,
			input.Actor.OrganizationID, request); createErr != nil {
			if errors.Is(createErr, repositories.ErrConflict) {
				return errorcodes.Conflict(errorcodes.ApprovalConflictingDecision,
					"An approval request for this operation is already open.")
			}
			return createErr
		}

		if histErr := s.opts.Approvals.AppendHistory(ctx, tx, input.Actor.OrganizationID,
			request.ID, "", models.StatusPending, strings.ToLower(input.Actor.ActorType),
			input.Actor.ActorID, "requested", input.CorrelationID); histErr != nil {
			return histErr
		}

		if auditErr := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: request.OrganizationID,
			BranchID:       input.Actor.BranchID,
			ActorType:      strings.ToLower(input.Actor.ActorType),
			ActorID:        input.Actor.ActorID,
			Action:         "approval.requested",
			ResourceType:   request.ResourceType,
			ResourceID:     request.ResourceID,
			NewValues: map[string]any{
				"approvalRequestId":      request.ID,
				"action":                 request.Action,
				"requestFingerprint":     request.RequestFingerprint,
				"expectedSubjectVersion": request.ExpectedSubjectVersion,
				"minimumApprovals":       request.MinimumApprovals,
				"policyVersion":          request.PolicyVersion,
			},
			RequestID:     input.CorrelationID,
			CorrelationID: input.CorrelationID,
		}); auditErr != nil {
			return auditErr
		}

		if s.opts.Events == nil {
			return nil
		}
		return s.opts.Events.Publish(ctx, tx, EventApprovalRequested, "approval_request",
			request.ID, request.Version, request.OrganizationID, input.CorrelationID,
			s.eventPayload(request))
	})
	if err != nil {
		return nil, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_approval_requests_total",
		map[string]string{"resource": request.ResourceType, "action": request.Action}, 1)
	return &RequestResult{Request: request}, nil
}

// DecideInput records one approver's decision.
type DecideInput struct {
	Actor             Actor
	ApprovalRequestID string
	Decision          string
	Reason            string
	// ClientDecisionID makes a retried decision idempotent.
	ClientDecisionID string
	// ExpectedFingerprint, when set, is checked against what is actually being
	// approved so an approver cannot be shown one payload and sign another.
	ExpectedFingerprint string
	CorrelationID       string
}

// Decide records an approval or a rejection.
//
// The request row is locked for the duration. Everything that decides the
// outcome — how many approvals exist, whether this approver already decided,
// whether the subject moved — is read inside that lock, because each of those
// is a race that produces a wrongly-approved operation if lost.
func (s *Service) Decide(ctx context.Context, input DecideInput) (*models.Request, error) {
	decision := strings.ToUpper(strings.TrimSpace(input.Decision))
	if decision != models.DecisionApproved && decision != models.DecisionRejected {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed,
			"decision must be APPROVED or REJECTED.")
	}
	if input.Actor.OrganizationUserID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"An organization user is required to decide an approval.")
	}

	var result *models.Request
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		request, err := s.opts.Approvals.LockRequest(ctx, tx,
			input.Actor.OrganizationID, input.ApprovalRequestID)
		if errors.Is(err, repositories.ErrNotFound) {
			// A request in another tenant answers identically to one that does
			// not exist.
			return errorcodes.NotFound(errorcodes.ApprovalNotFound, "Approval request not found.")
		}
		if err != nil {
			return err
		}

		// A retried decision replays rather than being refused or double
		// counted.
		if existing, findErr := s.opts.Approvals.FindDecisionByClientID(ctx, tx,
			request.ID, input.ClientDecisionID); findErr == nil {
			if existing.Decision != decision {
				return errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
					"This decision identifier was already used for a different decision.")
			}
			result = request
			return nil
		} else if !errors.Is(findErr, repositories.ErrNotFound) {
			return findErr
		}

		now := s.opts.Now().UTC()
		if err := s.assertDecidable(request, now); err != nil {
			return err
		}
		if input.ExpectedFingerprint != "" &&
			input.ExpectedFingerprint != request.RequestFingerprint {
			return errorcodes.Conflict(errorcodes.ApprovalFingerprintMismatch,
				"The operation changed since it was presented for approval.")
		}
		if err := s.assertMayDecide(ctx, tx, request, input.Actor); err != nil {
			return err
		}

		// The subject moving on invalidates the request outright. Recording a
		// decision against a subject that already changed would produce an
		// approval nobody could honestly execute.
		if changed, observed, checkErr := s.subjectChanged(ctx, tx, request); checkErr != nil {
			return checkErr
		} else if changed {
			if invalidateErr := s.invalidate(ctx, tx, request,
				models.InvalidationSubjectVersionChanged, input.Actor); invalidateErr != nil {
				return invalidateErr
			}
			_ = observed
			return errorcodes.Conflict(errorcodes.ApprovalSubjectVersionStale,
				"The subject changed since approval was requested.")
		}

		step, err := s.opts.Approvals.CurrentStep(ctx, tx, request.OrganizationID,
			request.ID, request.CurrentStep)
		if err != nil {
			return err
		}

		record := &models.Decision{
			ApprovalRequestID:   request.ID,
			ApprovalStepID:      step.ID,
			ApproverID:          input.Actor.OrganizationUserID,
			Decision:            decision,
			Reason:              input.Reason,
			RequestVersion:      request.Version,
			ApprovedFingerprint: request.RequestFingerprint,
			CorrelationID:       input.CorrelationID,
		}
		if input.ClientDecisionID != "" {
			clientID := input.ClientDecisionID
			record.ClientDecisionID = &clientID
		}
		if err := s.opts.Approvals.RecordDecision(ctx, tx,
			request.OrganizationID, record); err != nil {
			if errors.Is(err, repositories.ErrConflict) {
				return errorcodes.Conflict(errorcodes.ApprovalAlreadyResolved,
					"You have already decided this approval request.")
			}
			return err
		}

		nextStatus := models.StatusPending
		if decision == models.DecisionRejected {
			// One rejection ends it. A quorum of approvals cannot outvote a
			// documented objection: an approval framework that lets it is a
			// voting system, and this is a control.
			nextStatus = models.StatusRejected
			if err := s.opts.Approvals.CompleteStep(ctx, tx, step.ID, "REJECTED"); err != nil {
				return err
			}
		} else {
			approvals, countErr := s.opts.Approvals.CountApprovals(ctx, tx, request.ID)
			if countErr != nil {
				return countErr
			}
			if approvals >= request.MinimumApprovals {
				nextStatus = models.StatusApproved
				if err := s.opts.Approvals.CompleteStep(ctx, tx, step.ID, "COMPLETED"); err != nil {
					return err
				}
			}
		}

		if nextStatus != request.Status {
			if err := s.opts.Approvals.UpdateStatus(ctx, tx, request.OrganizationID,
				request.ID, request.Version, nextStatus, ""); err != nil {
				if errors.Is(err, repositories.ErrConflict) {
					return errorcodes.Conflict(errorcodes.ApprovalAlreadyResolved,
						"This approval request changed while you were deciding it.")
				}
				return err
			}
			if err := s.opts.Approvals.AppendHistory(ctx, tx, request.OrganizationID,
				request.ID, request.Status, nextStatus,
				strings.ToLower(input.Actor.ActorType), input.Actor.ActorID,
				strings.ToLower(decision), input.CorrelationID); err != nil {
				return err
			}
			request.Status = nextStatus
			request.Version++
		}

		if err := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: request.OrganizationID,
			ActorType:      strings.ToLower(input.Actor.ActorType),
			ActorID:        input.Actor.ActorID,
			Action:         "approval.decided",
			ResourceType:   request.ResourceType,
			ResourceID:     request.ResourceID,
			NewValues: map[string]any{
				"approvalRequestId": request.ID,
				"decision":          decision,
				"status":            request.Status,
				"policyVersion":     request.PolicyVersion,
			},
			Reason:        input.Reason,
			CorrelationID: input.CorrelationID,
		}); err != nil {
			return err
		}

		result = request
		if s.opts.Events == nil {
			return nil
		}
		return s.opts.Events.Publish(ctx, tx, EventApprovalDecided, "approval_request",
			request.ID, request.Version, request.OrganizationID, input.CorrelationID,
			s.eventPayload(request))
	})
	if err != nil {
		return nil, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_approval_decisions_total",
		map[string]string{"decision": decision}, 1)
	return result, nil
}

// CancelInput withdraws a request.
type CancelInput struct {
	Actor             Actor
	ApprovalRequestID string
	Reason            string
	CorrelationID     string
}

// Cancel withdraws a pending request.
//
// Only the requester or someone holding the cancel permission may withdraw,
// and only while the request is still open — cancelling an approved request
// after the fact would let a requester erase evidence that approval was given.
func (s *Service) Cancel(ctx context.Context, input CancelInput) (*models.Request, error) {
	var result *models.Request
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		request, err := s.opts.Approvals.LockRequest(ctx, tx,
			input.Actor.OrganizationID, input.ApprovalRequestID)
		if errors.Is(err, repositories.ErrNotFound) {
			return errorcodes.NotFound(errorcodes.ApprovalNotFound, "Approval request not found.")
		}
		if err != nil {
			return err
		}
		if request.Status != models.StatusPending {
			return errorcodes.Conflict(errorcodes.ApprovalAlreadyResolved,
				"This approval request is no longer open.")
		}

		if err := s.opts.Approvals.UpdateStatus(ctx, tx, request.OrganizationID,
			request.ID, request.Version, models.StatusCancelled, ""); err != nil {
			return err
		}
		if err := s.opts.Approvals.AppendHistory(ctx, tx, request.OrganizationID,
			request.ID, models.StatusPending, models.StatusCancelled,
			strings.ToLower(input.Actor.ActorType), input.Actor.ActorID,
			"cancelled", input.CorrelationID); err != nil {
			return err
		}
		request.Status = models.StatusCancelled
		request.Version++
		result = request

		return s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: request.OrganizationID,
			ActorType:      strings.ToLower(input.Actor.ActorType),
			ActorID:        input.Actor.ActorID,
			Action:         "approval.cancelled",
			ResourceType:   request.ResourceType,
			ResourceID:     request.ResourceID,
			NewValues:      map[string]any{"approvalRequestId": request.ID},
			Reason:         input.Reason,
			CorrelationID:  input.CorrelationID,
		})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ExecuteInput spends an approval on the protected operation.
type ExecuteInput struct {
	Actor             Actor
	ApprovalRequestID string
	// Payload is what the caller is about to execute. Its fingerprint must
	// equal the approved one.
	Payload map[string]any
	// ExecutionReference identifies the operation being performed. It is
	// unique per organization, which is what makes execution exactly-once.
	ExecutionReference string
	IdempotencyKey     string
	CorrelationID      string
	CausationID        string
}

// ExecuteResult carries the outcome.
type ExecuteResult struct {
	Request  *models.Request
	Replayed bool
}

// Execute performs the protected operation atomically with spending the
// approval.
//
// The protected operation runs inside the same transaction as the approval
// state change, so there is no window where the operation happened and the
// approval still looks unspent, or the reverse. Where an operation genuinely
// cannot join this transaction — a call to another service — the caller passes
// a run function that records a durable intent instead, and the outbox carries
// it.
func (s *Service) Execute(ctx context.Context, input ExecuteInput,
	run func(tx pgx.Tx, request *models.Request) error) (*ExecuteResult, error) {
	if strings.TrimSpace(input.ExecutionReference) == "" {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An execution reference is required.")
	}

	// A retried execution resolves to the recorded outcome rather than running
	// the operation a second time.
	if existing, err := s.opts.Approvals.FindExecutionByIdempotencyKey(ctx, nil,
		input.Actor.OrganizationID, input.IdempotencyKey); err == nil {
		request, getErr := s.opts.Approvals.GetRequest(ctx, nil,
			input.Actor.OrganizationID, existing.ApprovalRequestID)
		if getErr != nil {
			return nil, getErr
		}
		if existing.Outcome != models.OutcomeExecuted {
			return nil, errorcodes.Conflict(errorcodes.ApprovalNotExecutable,
				"This execution was already attempted and did not succeed.")
		}
		return &ExecuteResult{Request: request, Replayed: true}, nil
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}

	// A refusal still has to leave a record, and a record written inside the
	// transaction that refuses would be rolled back with it. The verdict is
	// therefore reached in one transaction and, when it is a refusal, the
	// accountability write happens in a second one that commits.
	var result *ExecuteResult
	var refusal *executionRefusal
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		request, err := s.opts.Approvals.LockRequest(ctx, tx,
			input.Actor.OrganizationID, input.ApprovalRequestID)
		if errors.Is(err, repositories.ErrNotFound) {
			return errorcodes.NotFound(errorcodes.ApprovalNotFound, "Approval request not found.")
		}
		if err != nil {
			return err
		}

		presented, err := PayloadFingerprint(request.OrganizationID, request.ResourceType,
			request.ResourceID, request.Action, input.Payload)
		if err != nil {
			return err
		}

		observed, _, versionErr := s.observedSubjectVersion(ctx, tx, request)
		if versionErr != nil {
			return versionErr
		}

		now := s.opts.Now().UTC()
		refuse := func(reasonCode string, invalidationReason string, cause error) error {
			refusal = &executionRefusal{
				request: request, presented: presented, observed: observed,
				reasonCode: reasonCode, invalidationReason: invalidationReason,
				cause: cause,
			}
			// Roll this transaction back; nothing it did should survive.
			return errExecutionRefused
		}

		if !request.Executable(now) {
			return refuse("not_executable", "", s.notExecutableError(request, now))
		}

		// The exact-payload rule. A different fingerprint means the approvers
		// signed off on something else, and the approval is invalidated rather
		// than merely refused - otherwise the caller can keep trying payloads
		// until one matches.
		if presented != request.RequestFingerprint {
			return refuse("fingerprint_mismatch", models.InvalidationPayloadChanged,
				errorcodes.Conflict(errorcodes.ApprovalFingerprintMismatch,
					"The operation being executed is not the one that was approved."))
		}

		if request.ExpectedSubjectVersion != 0 && observed != 0 &&
			observed != request.ExpectedSubjectVersion {
			return refuse("subject_version_changed", models.InvalidationSubjectVersionChanged,
				errorcodes.Conflict(errorcodes.ApprovalSubjectVersionStale,
					"The subject changed since it was approved."))
		}

		// The protected operation and the approval spend commit together.
		if run != nil {
			if runErr := run(tx, request); runErr != nil {
				return runErr
			}
		}

		if err := s.opts.Approvals.MarkExecuted(ctx, tx, request.OrganizationID,
			request.ID, request.Version, input.ExecutionReference); err != nil {
			if errors.Is(err, repositories.ErrConflict) {
				return errorcodes.Conflict(errorcodes.ApprovalNotExecutable,
					"This approval was already executed.")
			}
			return err
		}
		if err := s.opts.Approvals.RecordExecution(ctx, tx, request.OrganizationID,
			s.executionRecord(input, request, presented, observed,
				models.OutcomeExecuted, "")); err != nil {
			if errors.Is(err, repositories.ErrConflict) {
				return errorcodes.Conflict(errorcodes.ApprovalNotExecutable,
					"This execution reference was already used.")
			}
			return err
		}
		if err := s.opts.Approvals.AppendHistory(ctx, tx, request.OrganizationID,
			request.ID, models.StatusApproved, models.StatusExecuted,
			strings.ToLower(input.Actor.ActorType), input.Actor.ActorID,
			"executed", input.CorrelationID); err != nil {
			return err
		}

		executedAt := now
		request.Status = models.StatusExecuted
		request.ExecutedAt = &executedAt
		reference := input.ExecutionReference
		request.ExecutionReference = &reference
		request.Version++

		if err := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: request.OrganizationID,
			ActorType:      strings.ToLower(input.Actor.ActorType),
			ActorID:        input.Actor.ActorID,
			Action:         "approval.executed",
			ResourceType:   request.ResourceType,
			ResourceID:     request.ResourceID,
			NewValues: map[string]any{
				"approvalRequestId":  request.ID,
				"executionReference": input.ExecutionReference,
				"fingerprint":        request.RequestFingerprint,
			},
			CorrelationID: input.CorrelationID,
		}); err != nil {
			return err
		}

		result = &ExecuteResult{Request: request}
		if s.opts.Events == nil {
			return nil
		}
		return s.opts.Events.Publish(ctx, tx, EventApprovalExecuted, "approval_request",
			request.ID, request.Version, request.OrganizationID, input.CorrelationID,
			s.eventPayload(request))
	})
	if refusal != nil {
		return nil, s.recordRefusal(ctx, input, refusal)
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// errExecutionRefused rolls the verdict transaction back. It never reaches a
// caller: the refusal it stands for is returned by recordRefusal.
var errExecutionRefused = errors.New("approvals: execution refused")

// executionRefusal is a refused attempt awaiting its durable record.
type executionRefusal struct {
	request            *models.Request
	presented          string
	observed           int64
	reasonCode         string
	invalidationReason string
	cause              error
}

// recordRefusal commits the attempt record and any invalidation, then returns
// the refusal.
//
// A failure to WRITE the record does not replace the refusal with a different
// error: the caller is refused either way, and a write failure is not
// something they can act on. It is counted so an operator sees the gap.
func (s *Service) recordRefusal(ctx context.Context, input ExecuteInput,
	refusal *executionRefusal) error {
	writeErr := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Approvals.RecordExecution(ctx, tx,
			refusal.request.OrganizationID,
			s.executionRecord(input, refusal.request, refusal.presented, refusal.observed,
				models.OutcomeRejected, refusal.reasonCode)); err != nil {
			if !errors.Is(err, repositories.ErrConflict) {
				return err
			}
		}
		if refusal.invalidationReason == "" {
			return nil
		}
		return s.invalidate(ctx, tx, refusal.request, refusal.invalidationReason, input.Actor)
	})
	if writeErr != nil {
		s.opts.Metrics.IncCounter("fleet_partner_approval_refusal_write_failures_total",
			map[string]string{"reason": refusal.reasonCode}, 1)
	}
	return refusal.cause
}

// executionRecord builds the durable record of one execution attempt.
func (s *Service) executionRecord(input ExecuteInput, request *models.Request,
	presented string, observed int64, outcome string, reasonCode string) *models.Execution {
	return &models.Execution{
		ApprovalRequestID:      request.ID,
		PresentedFingerprint:   presented,
		ApprovedFingerprint:    request.RequestFingerprint,
		ExpectedSubjectVersion: request.ExpectedSubjectVersion,
		ObservedSubjectVersion: observed,
		ExecutionReference:     input.ExecutionReference,
		Outcome:                outcome,
		SafeReasonCode:         reasonCode,
		ActorType:              input.Actor.ActorType,
		ActorID:                input.Actor.ActorID,
		IdempotencyKey:         input.IdempotencyKey,
		CorrelationID:          input.CorrelationID,
		CausationID:            input.CausationID,
	}
}

// Get returns one request inside the organization scope.
func (s *Service) Get(ctx context.Context, organizationID string,
	requestID string) (*models.Request, error) {
	request, err := s.opts.Approvals.GetRequest(ctx, nil, organizationID, requestID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, errorcodes.NotFound(errorcodes.ApprovalNotFound, "Approval request not found.")
	}
	return request, err
}

// List returns requests inside the tenant and branch scope.
func (s *Service) List(ctx context.Context, query tenant.Query,
	filter repositories.ListFilter) ([]models.Request, error) {
	if query.OrganizationID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"Organization scope is required.")
	}
	return s.opts.Approvals.List(ctx, query, filter)
}

// Decisions returns the immutable decisions on one request.
func (s *Service) Decisions(ctx context.Context, organizationID string,
	requestID string) ([]models.Decision, error) {
	if _, err := s.Get(ctx, organizationID, requestID); err != nil {
		return nil, err
	}
	return s.opts.Approvals.ListDecisions(ctx, nil, requestID)
}

// History returns the append-only history of one request.
func (s *Service) History(ctx context.Context, organizationID string,
	requestID string) ([]repositories.HistoryEntry, error) {
	if _, err := s.Get(ctx, organizationID, requestID); err != nil {
		return nil, err
	}
	return s.opts.Approvals.ListHistory(ctx, organizationID, requestID)
}

// ExpireOverdue sweeps requests whose window has closed.
func (s *Service) ExpireOverdue(ctx context.Context, limit int) (int64, error) {
	return s.opts.Approvals.ExpireOverdue(ctx, nil, limit)
}

// RequireApproved is the enforcement helper a protected use case calls when it
// needs an approval to already exist.
func (s *Service) RequireApproved(ctx context.Context, organizationID string,
	requestID string, payload map[string]any) (*models.Request, error) {
	request, err := s.Get(ctx, organizationID, requestID)
	if err != nil {
		return nil, err
	}
	now := s.opts.Now().UTC()
	if !request.Executable(now) {
		return nil, s.notExecutableError(request, now)
	}
	presented, err := PayloadFingerprint(organizationID, request.ResourceType,
		request.ResourceID, request.Action, payload)
	if err != nil {
		return nil, err
	}
	if presented != request.RequestFingerprint {
		return nil, errorcodes.Conflict(errorcodes.ApprovalFingerprintMismatch,
			"The operation being executed is not the one that was approved.")
	}
	return request, nil
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

// payloadVersion identifies the normalization that produces a fingerprint.
// Changing the normalization requires bumping this, so old approvals cannot be
// silently revalidated under new hashing rules.
const payloadVersion = 1

// PayloadFingerprint hashes the normalized controlled action.
//
// Normalization is canonical JSON with sorted keys, produced by encoding/json's
// map ordering, prefixed by the tenant, resource and action. Two callers
// describing the same operation therefore produce the same hash regardless of
// field order, and any change of value produces a different one.
func PayloadFingerprint(organizationID string, resourceType string, resourceID string,
	action string, payload map[string]any) (string, error) {
	canonical, err := json.Marshal(normalize(payload))
	if err != nil {
		return "", errorcodes.BadRequest(errorcodes.ValidationFailed,
			"The operation payload could not be encoded.")
	}
	hasher := sha256.New()
	for _, part := range []string{organizationID, resourceType, resourceID, action} {
		_, _ = hasher.Write([]byte(part))
		_, _ = hasher.Write([]byte{0})
	}
	_, _ = hasher.Write(canonical)
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// normalize produces a stable representation of a payload.
//
// encoding/json already sorts map keys, so the work here is recursing into
// nested maps and slices so a nested reordering cannot change the hash either.
func normalize(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			normalized[key] = normalize(typed[key])
		}
		return normalized
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, element := range typed {
			normalized = append(normalized, normalize(element))
		}
		return normalized
	default:
		return value
	}
}

func (s *Service) validateRequest(input *RequestInput) error {
	input.ResourceType = strings.TrimSpace(input.ResourceType)
	input.ResourceID = strings.TrimSpace(input.ResourceID)
	input.Action = strings.TrimSpace(input.Action)
	input.OperationType = strings.ToUpper(strings.TrimSpace(input.OperationType))
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Actor.ActorType == "" {
		input.Actor.ActorType = models.ActorOrganizationUser
	}

	switch {
	case input.Actor.OrganizationID == "":
		return errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"Organization scope is required.")
	case input.Actor.OrganizationUserID == "":
		return errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"An organization user is required to request approval.")
	case input.ResourceType == "" || input.ResourceID == "" || input.Action == "":
		return errorcodes.BadRequest(errorcodes.ValidationFailed,
			"resourceType, resourceId and action are required.")
	case input.OperationType == "":
		return errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An operation type is required.")
	case input.IdempotencyKey == "":
		return errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An idempotency key is required.")
	}
	return nil
}

// findRequestReplay resolves a retried creation.
func (s *Service) findRequestReplay(ctx context.Context, input RequestInput,
	fingerprint string) (*RequestResult, error) {
	existing, err := s.opts.Approvals.FindByIdempotencyKey(ctx, nil,
		input.Actor.OrganizationID, input.IdempotencyKey)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if existing.RequestFingerprint != fingerprint {
		return nil, errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
			"This idempotency key was already used for a different request.")
	}
	return &RequestResult{Request: existing, Replayed: true}, nil
}

// assertDecidable refuses a decision on a request that cannot take one.
func (s *Service) assertDecidable(request *models.Request, now time.Time) error {
	switch {
	case request.Status == models.StatusInvalidated:
		return errorcodes.Conflict(errorcodes.ApprovalInvalidated,
			"This approval request no longer describes the operation and must be raised again.")
	case request.Terminal():
		return errorcodes.Conflict(errorcodes.ApprovalAlreadyResolved,
			"This approval request is no longer open.")
	case !request.ExpiresAt.After(now):
		return errorcodes.Conflict(errorcodes.ApprovalExpired,
			"This approval request has expired.")
	default:
		return nil
	}
}

// assertMayDecide enforces self-approval policy, separation of duties and the
// eligible-role list.
func (s *Service) assertMayDecide(ctx context.Context, q pgx.Tx, request *models.Request,
	actor Actor) error {
	if !request.SelfApprovalAllowed && actor.OrganizationUserID == request.RequesterID {
		return errorcodes.Forbidden(errorcodes.ApprovalSelfDecisionForbidden,
			"You cannot approve a request you raised.")
	}
	// Separation of duties goes further than self-approval: even where the
	// requester is allowed to approve, someone who already acted on this
	// request in another capacity cannot also be the one who completes quorum.
	if request.SeparationOfDuties {
		if existing, err := s.opts.Approvals.FindDecisionByApprover(ctx, q,
			request.ID, actor.OrganizationUserID); err == nil {
			_ = existing
			return errorcodes.Forbidden(errorcodes.ApprovalSeparationOfDuties,
				"You have already acted on this approval request.")
		} else if !errors.Is(err, repositories.ErrNotFound) {
			return err
		}
	}

	if len(request.EligibleRoleIDs) == 0 {
		return nil
	}
	if s.opts.Roles == nil {
		// Eligible roles were specified and there is no way to check them.
		// Denying is the only safe reading of a missing gate.
		return errorcodes.Forbidden(errorcodes.ApprovalActorNotEligible,
			"Your eligibility to decide this request could not be established.")
	}
	bindings, err := s.opts.Roles.ListBindings(ctx, request.OrganizationID,
		actor.OrganizationUserID)
	if err != nil {
		return err
	}
	now := s.opts.Now()
	for _, binding := range bindings {
		if !binding.Active(now) {
			continue
		}
		for _, eligible := range request.EligibleRoleIDs {
			if binding.RoleID == eligible {
				return nil
			}
		}
	}
	return errorcodes.Forbidden(errorcodes.ApprovalActorNotEligible,
		"You do not hold a role that may decide this request.")
}

// subjectChanged reports whether the subject moved since approval was asked
// for.
func (s *Service) subjectChanged(ctx context.Context, q pgx.Tx,
	request *models.Request) (bool, int64, error) {
	observed, known, err := s.observedSubjectVersionTx(ctx, q, request)
	if err != nil {
		return false, 0, err
	}
	if !known || request.ExpectedSubjectVersion == 0 {
		return false, observed, nil
	}
	return observed != request.ExpectedSubjectVersion, observed, nil
}

func (s *Service) observedSubjectVersion(ctx context.Context, q pgx.Tx,
	request *models.Request) (int64, bool, error) {
	return s.observedSubjectVersionTx(ctx, q, request)
}

func (s *Service) observedSubjectVersionTx(ctx context.Context, q pgx.Tx,
	request *models.Request) (int64, bool, error) {
	if s.opts.Subjects == nil {
		return 0, false, nil
	}
	observed, err := s.opts.Subjects.SubjectVersion(ctx, q, request.OrganizationID,
		request.ResourceType, request.ResourceID)
	if err != nil {
		if errors.Is(err, repositories.ErrSubjectVersionUnsupported) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return observed, true, nil
}

// invalidate kills an approval and records why.
func (s *Service) invalidate(ctx context.Context, q pgx.Tx, request *models.Request,
	reason string, actor Actor) error {
	if err := s.opts.Approvals.UpdateStatus(ctx, q, request.OrganizationID, request.ID,
		request.Version, models.StatusInvalidated, reason); err != nil {
		return err
	}
	if err := s.opts.Approvals.AppendHistory(ctx, q, request.OrganizationID, request.ID,
		request.Status, models.StatusInvalidated, strings.ToLower(actor.ActorType),
		actor.ActorID, reason, request.CorrelationID); err != nil {
		return err
	}
	previousStatus := request.Status
	request.Status = models.StatusInvalidated
	request.InvalidationReason = reason
	request.Version++

	if err := s.opts.Audit.Record(ctx, q, audit.Entry{
		OrganizationID: request.OrganizationID,
		ActorType:      strings.ToLower(actor.ActorType),
		ActorID:        actor.ActorID,
		Action:         "approval.invalidated",
		ResourceType:   request.ResourceType,
		ResourceID:     request.ResourceID,
		PreviousValues: map[string]any{"status": previousStatus},
		NewValues: map[string]any{
			"approvalRequestId":  request.ID,
			"status":             models.StatusInvalidated,
			"invalidationReason": reason,
		},
	}); err != nil {
		return err
	}

	if s.opts.Events == nil {
		return nil
	}
	return s.opts.Events.Publish(ctx, q, EventApprovalInvalidated, "approval_request",
		request.ID, request.Version, request.OrganizationID, request.CorrelationID,
		s.eventPayload(request))
}

// notExecutableError explains precisely why an approval cannot be spent.
func (s *Service) notExecutableError(request *models.Request, now time.Time) error {
	switch {
	case request.Status == models.StatusInvalidated:
		return errorcodes.Conflict(errorcodes.ApprovalInvalidated,
			"This approval no longer describes the operation and must be raised again.")
	case request.Status == models.StatusRejected:
		return errorcodes.Conflict(errorcodes.ApprovalAlreadyResolved,
			"This operation was rejected.")
	case request.Status == models.StatusExecuted || request.ExecutedAt != nil:
		return errorcodes.Conflict(errorcodes.ApprovalNotExecutable,
			"This approval was already executed.")
	case request.Status == models.StatusPending:
		return errorcodes.UnprocessableEntity(errorcodes.ApprovalQuorumNotMet,
			"This operation has not been approved yet.")
	case !request.ExpiresAt.After(now):
		return errorcodes.Conflict(errorcodes.ApprovalExpired,
			"This approval has expired.")
	default:
		return errorcodes.Conflict(errorcodes.ApprovalNotExecutable,
			"This approval cannot be executed.")
	}
}

// eventPayload is the allowlisted event body.
//
// Built field by field and deliberately without the payload snapshot: the
// snapshot can contain operational detail, and an event is the widest audience
// any of this reaches.
func (s *Service) eventPayload(request *models.Request) map[string]any {
	return map[string]any{
		"approvalRequestId":      request.ID,
		"organizationId":         request.OrganizationID,
		"resourceType":           request.ResourceType,
		"resourceId":             request.ResourceID,
		"action":                 request.Action,
		"status":                 request.Status,
		"requestFingerprint":     request.RequestFingerprint,
		"payloadVersion":         request.PayloadVersion,
		"expectedSubjectVersion": request.ExpectedSubjectVersion,
		"minimumApprovals":       request.MinimumApprovals,
		"policyVersion":          request.PolicyVersion,
		"invalidationReason":     request.InvalidationReason,
		"expiresAt":              request.ExpiresAt,
	}
}

// EligibilityOperationFor maps a resource action onto the eligibility policy
// that governs it. Unmapped actions have no policy, and an operation with no
// policy cannot be approved — which is the safe default for a control.
func EligibilityOperationFor(action string) string {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "ACTIVATE", "COMPANY_ACTIVATION":
		return eligibilitymodels.OperationCompanyActivation
	case "LINK_PROVIDER", "PROVIDER_LINKING":
		return eligibilitymodels.OperationProviderLinking
	case "LINK_VEHICLE", "VEHICLE_LINKING":
		return eligibilitymodels.OperationVehicleLinking
	case "SETTLE", "RELEASE_HOLD", "FINANCIAL_RELEASE":
		return eligibilitymodels.OperationFinancialRelease
	case "REVERSE", "REFUND", "FINANCIAL_REVERSAL":
		return eligibilitymodels.OperationFinancialReversal
	default:
		return ""
	}
}
