package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/enforcement"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// EventPublisher appends corporate domain events to the outbox.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string, aggregateVersion int64, organizationID string, correlationID string, payload any) error
}

// MediaValidator confirms decline evidence belongs to this organization and
// is safe to reference.
type MediaValidator interface {
	ValidateEvidence(ctx context.Context, meta platformclients.CallMeta, organizationID string, fileIDs []string) error
}

type PrivateAcceptanceReader interface {
	VerifyPrivateAcceptance(context.Context, string) (*ProviderAcceptanceView, error)
}

// PrivateExecutionEventProjector applies authoritative Dispatch and Hauling
// events that carry a Fleet private-execution operation reference. It runs
// inside the same transaction as the consumer inbox.
type PrivateExecutionEventProjector interface {
	ProjectPrivateExecutionEvent(context.Context, pgx.Tx, InboundEvent) (repositories.SourceEventOutcome, error)
}

// Flags gates the marketplace write paths so a service category can be rolled
// out — or switched off — without a deploy.
//
// Disabling a write flag must never hide provider Decline for an assignment
// that was already presented: the decline path deliberately does not consult
// any flag.
type Flags struct {
	ProjectionEnabled                bool
	OrganizationAcceptEnabled        bool
	ProviderDirectAcceptEnabled      bool
	AssignmentEnabled                bool
	ExecutionStartEnabled            bool
	ReassignmentEnabled              bool
	AutomaticReassignmentEnabled     bool
	AutomaticReassignmentMaxAttempts int
	// EnabledSourceServices lists the categories whose live execution
	// integration is switched on. Categories whose authoritative service has
	// not implemented the contract stay off.
	EnabledSourceServices map[string]bool
}

// SourceEnabled reports whether live execution calls are allowed for a
// service category.
func (f Flags) SourceEnabled(category string) bool {
	return f.EnabledSourceServices[category]
}

// Options configure the assignment service.
type Options struct {
	Pool        *pgxpool.Pool
	Projections *repositories.ProjectionRepository
	Offers      *repositories.OfferRepository
	Assignments *repositories.AssignmentRepository
	// ExecutionStarts holds durable execution-start work created atomically
	// with a provider's acceptance.
	ExecutionStarts *repositories.ExecutionStartRepository
	Trips           *repositories.TripRepository
	Commands        *repositories.CommandRepository
	Executions      platformclients.ExecutionSet
	Eligibility     *EligibilityChecker
	// DurableEligibility records and enforces the durable decision layer. The
	// in-feature EligibilityChecker above answers "may this membership take
	// this request right now"; this one records that answer as an auditable,
	// expiring decision citing the authorities it consulted. Both run: the
	// first is fast and marketplace-specific, the second is the accountable
	// record a gate can be audited against afterwards.
	DurableEligibility enforcement.Gate
	Media              MediaValidator
	PrivateAcceptance  PrivateAcceptanceReader
	PrivateEvents      PrivateExecutionEventProjector
	Events             EventPublisher
	Audit              *audit.Recorder
	Metrics            *observability.Metrics
	Flags              Flags
	// CandidateMemberships lists active provider memberships for the
	// automatic reassignment coordinator to try, in order. Optional: when
	// nil, AutoReassignStaleCase reports the capability as unavailable
	// rather than panicking, so a deployment that has not wired it keeps
	// the safe (human-reviewed) reassignment path.
	CandidateMemberships CandidateMembershipLister
	// MembershipHistory backs the provider-self "membership history" read
	// (Section 28/34). Optional: when nil, ListProviderMemberships reports
	// the capability as unavailable rather than panicking.
	MembershipHistory MembershipHistoryReader

	ResponseTimeoutDefault time.Duration
	ResponseTimeoutMin     time.Duration
	ResponseTimeoutMax     time.Duration
	Now                    func() time.Time
}

// Service implements marketplace acceptance, assignment and provider response.
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
	if opts.ResponseTimeoutDefault <= 0 {
		opts.ResponseTimeoutDefault = 90 * time.Second
	}
	if opts.ResponseTimeoutMin <= 0 {
		opts.ResponseTimeoutMin = 30 * time.Second
	}
	if opts.ResponseTimeoutMax <= 0 {
		opts.ResponseTimeoutMax = 15 * time.Minute
	}
	return &Service{opts: opts}
}

// ProviderActor is the verified identity of a responding provider.
//
// It is produced only by the provider authentication boundary (a signed
// assertion from the owning execution service). A membership ID from a
// request body is never trusted.
type ProviderActor struct {
	MembershipID        string
	ExecutingProviderID string
	SourceService       string
	OrganizationID      string
}

// AcceptAssignmentInput is a provider accepting work.
type AcceptAssignmentInput struct {
	Actor            ProviderActor
	AssignmentID     string
	ClientResponseID string
	DeviceTime       *time.Time
	ExpectedVersion  int64
	RequestID        string
	CorrelationID    string
}

// AssignmentResult is the response returned to a provider.
type AssignmentResult struct {
	Assignment           *models.Assignment
	AllowedActions       []string
	ExecutionStartStatus string
	Replayed             bool
}

// AcceptAssignment records a provider's acceptance.
//
// Sequence, and why it is this order:
//  1. everything is validated and the immutable response is committed
//     locally, so the provider's decision survives any downstream failure;
//  2. only then is canonical execution requested from the owning service.
//
// A timed-out execution call therefore never loses the acceptance and never
// produces a second provider response — it leaves a reconciliation marker and
// resolves by read-back.
func (s *Service) AcceptAssignment(ctx context.Context, input AcceptAssignmentInput) (*AssignmentResult, error) {
	// A retrying mobile client replays the original outcome instead of
	// colliding with its own earlier response.
	if replay, err := s.findReplay(ctx, input.Actor.MembershipID, input.ClientResponseID, input.AssignmentID); err != nil {
		return nil, err
	} else if replay != nil {
		return replay, nil
	}

	var result AssignmentResult
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		assignment, err := s.lockOwnedAssignment(ctx, tx, input.AssignmentID, input.Actor)
		if err != nil {
			return err
		}
		if err := s.assertRespondable(assignment, input.ExpectedVersion); err != nil {
			return err
		}

		// Re-check eligibility at RESPONSE time: assignment-time eligibility
		// does not prove the provider is still allowed to work now.
		projection, err := s.opts.Projections.LockByID(ctx, tx, assignment.OrganizationID, assignment.RequestProjectionID)
		if err != nil {
			return err
		}
		if err := s.assertRequestStillAssignable(projection); err != nil {
			return err
		}
		if err := s.recheckEligibility(ctx, assignment, projection); err != nil {
			return err
		}
		// Acceptance is the moment work becomes real, so it takes its own
		// durable decision rather than relying on the one taken at assignment
		// time — which may be minutes or hours old and was answering a
		// different question.
		if err := s.gateExecution(ctx, assignment, "PROVIDER",
			input.Actor.MembershipID, input.CorrelationID); err != nil {
			return err
		}

		response := &models.Response{
			OrganizationID:   assignment.OrganizationID,
			AssignmentID:     assignment.ID,
			MembershipID:     assignment.MembershipID,
			Response:         models.ResponseAccepted,
			ClientResponseID: input.ClientResponseID,
			DeviceRecordedAt: input.DeviceTime,
			RequestID:        input.RequestID,
			CorrelationID:    input.CorrelationID,
		}
		if err := s.opts.Assignments.CreateResponse(ctx, tx, response); err != nil {
			if errors.Is(err, repositories.ErrResponseExists) {
				return errorcodes.Conflict(errorcodes.AssignmentAlreadyResolved,
					"This assignment already has a response.")
			}
			return err
		}

		if err := s.opts.Assignments.Transition(ctx, tx, assignment, models.AssignmentAccepted,
			input.ExpectedVersion, models.StateHistoryEntry{
				ActorType: "PROVIDER", ActorID: assignment.MembershipID,
				RequestID: input.RequestID, CorrelationID: input.CorrelationID,
			}); err != nil {
			return mapTransitionError(err)
		}

		// The winning provider keeps its own hold; every other assignment's
		// holds were released when those assignments resolved.
		if err := s.opts.Projections.SetStatus(ctx, tx, assignment.OrganizationID,
			projection.ID, models.ProjectionProviderAccepted, projection.Version); err != nil {
			return err
		}
		if err := s.opts.Projections.SetExecutionContext(ctx, tx, assignment.OrganizationID,
			projection.ID, assignment.MembershipID, assignment.ExecutingProviderID,
			assignment.ID, models.AcceptanceDispatcher); err != nil {
			return err
		}

		entry := audit.Entry{
			OrganizationID: assignment.OrganizationID,
			ActorType:      "provider", ActorID: assignment.MembershipID,
			Action: "assignment.accepted", ResourceType: "assignment", ResourceID: assignment.ID,
			NewValues: map[string]any{"status": models.AssignmentAccepted},
			RequestID: input.RequestID, CorrelationID: input.CorrelationID,
		}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		if err := s.opts.Events.Publish(ctx, tx, "organization.assignment.accepted.v1", "assignment",
			assignment.ID, assignment.Version+1, assignment.OrganizationID, input.CorrelationID,
			map[string]any{
				"assignmentId":   assignment.ID,
				"organizationId": assignment.OrganizationID,
				"membershipId":   assignment.MembershipID,
				"sourceService":  assignment.SourceService,
			}); err != nil {
			return err
		}

		// Queue durable execution-start work in the SAME transaction as the
		// acceptance. If this were a follow-up write, a crash in between would
		// leave a provider who said yes with no work queued and no trip ever
		// created — the acceptance would be silently stranded.
		if s.opts.ExecutionStarts != nil {
			if err := s.opts.ExecutionStarts.Create(ctx, tx, &models.ExecutionStartOperation{
				OrganizationID:      assignment.OrganizationID,
				AssignmentID:        assignment.ID,
				ProviderResponseID:  response.ID,
				RequestProjectionID: assignment.RequestProjectionID,
				SourceService:       assignment.SourceService,
				// The key is derived from the assignment, so every retry and
				// read-back for this assignment reuses the SAME command.
				IdempotencyKey:     "exec-" + assignment.ID,
				RequestFingerprint: fingerprint(assignment.ID, response.ID, assignment.ExecutingProviderID),
			}); err != nil {
				return err
			}
		}

		assignment.Status = models.AssignmentAccepted
		assignment.Version = input.ExpectedVersion + 1
		result.Assignment = assignment
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_assignment_accepts_total",
		map[string]string{"source": result.Assignment.SourceService}, 1)

	// Canonical execution is requested outside the transaction: an external
	// call must never hold a database lock, and the acceptance is already
	// durable.
	result.ExecutionStartStatus = s.requestExecution(ctx, result.Assignment, input.CorrelationID)
	result.AllowedActions = result.Assignment.AllowedActions(s.opts.Now())
	return &result, nil
}

// DeclineAssignmentInput is a provider refusing work.
type DeclineAssignmentInput struct {
	Actor            ProviderActor
	AssignmentID     string
	ClientResponseID string
	ReasonCode       string
	Note             string
	EvidenceFileIDs  []string
	ExpectedVersion  int64
	RequestID        string
	CorrelationID    string
}

// DeclineAssignment records a provider's refusal.
//
// A decline is always available while an assignment is pending — no
// organization setting, role, operating mode or feature flag can remove it,
// and none is consulted here. The decline itself never triggers a penalty:
// it produces reassignment plus typed, durable follow-up actions only.
func (s *Service) DeclineAssignment(ctx context.Context, input DeclineAssignmentInput) (*AssignmentResult, error) {
	if input.ReasonCode == "" {
		return nil, errorcodes.BadRequest(errorcodes.DeclineReasonRequired,
			"A decline reason is required.")
	}
	if !models.ValidDeclineReason(input.ReasonCode) {
		return nil, errorcodes.BadRequest(errorcodes.DeclineReasonInvalid,
			"That decline reason is not recognised.")
	}
	if models.RequiresNote(input.ReasonCode) && input.Note == "" {
		return nil, errorcodes.BadRequest(errorcodes.DeclineNoteRequired,
			"Please explain the reason for declining.")
	}
	if len(input.Note) > models.MaxDeclineNoteLength {
		return nil, errorcodes.BadRequest(errorcodes.DeclineNoteRequired,
			"The note is too long.")
	}

	if replay, err := s.findReplay(ctx, input.Actor.MembershipID, input.ClientResponseID, input.AssignmentID); err != nil {
		return nil, err
	} else if replay != nil {
		return replay, nil
	}

	// Evidence ownership is validated before anything is committed.
	if len(input.EvidenceFileIDs) > 0 && s.opts.Media != nil {
		if err := s.opts.Media.ValidateEvidence(ctx,
			platformclients.CallMeta{RequestID: input.RequestID, CorrelationID: input.CorrelationID},
			input.Actor.OrganizationID, input.EvidenceFileIDs); err != nil {
			return nil, errorcodes.BadRequest(errorcodes.DeclineEvidenceInvalid,
				"The attached evidence could not be verified.")
		}
	}

	safety := models.SafetyProtected(input.ReasonCode)
	var result AssignmentResult

	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		assignment, err := s.lockOwnedAssignment(ctx, tx, input.AssignmentID, input.Actor)
		if err != nil {
			return err
		}
		if err := s.assertRespondable(assignment, input.ExpectedVersion); err != nil {
			return err
		}

		var notePtr *string
		if input.Note != "" {
			notePtr = &input.Note
		}
		reason := input.ReasonCode
		response := &models.Response{
			OrganizationID:   assignment.OrganizationID,
			AssignmentID:     assignment.ID,
			MembershipID:     assignment.MembershipID,
			Response:         models.ResponseDeclined,
			ReasonCode:       &reason,
			ReasonText:       notePtr,
			SafetyRelated:    safety,
			ClientResponseID: input.ClientResponseID,
			RequestID:        input.RequestID,
			CorrelationID:    input.CorrelationID,
			EvidenceFileIDs:  input.EvidenceFileIDs,
		}
		if err := s.opts.Assignments.CreateResponse(ctx, tx, response); err != nil {
			if errors.Is(err, repositories.ErrResponseExists) {
				return errorcodes.Conflict(errorcodes.AssignmentAlreadyResolved,
					"This assignment already has a response.")
			}
			return err
		}

		if err := s.opts.Assignments.Transition(ctx, tx, assignment, models.AssignmentDeclined,
			input.ExpectedVersion, models.StateHistoryEntry{
				ActorType: "PROVIDER", ActorID: assignment.MembershipID, ReasonCode: &reason,
				RequestID: input.RequestID, CorrelationID: input.CorrelationID,
			}); err != nil {
			return mapTransitionError(err)
		}

		// Free the provider and asset immediately: a declined provider must
		// not stay blocked from other work.
		if err := s.opts.Assignments.ReleaseReservations(ctx, tx, assignment.ID); err != nil {
			return err
		}

		// Typed, durable follow-ups. None of these is a penalty.
		for _, actionType := range models.FollowupsForReason(input.ReasonCode) {
			if err := s.opts.Assignments.CreateFollowup(ctx, tx, &models.FollowupAction{
				OrganizationID: assignment.OrganizationID,
				ResponseID:     response.ID,
				MembershipID:   assignment.MembershipID,
				ActionType:     actionType,
				ReasonCode:     input.ReasonCode,
				SafetyRelated:  safety,
				Payload: map[string]any{
					"assignmentId":  assignment.ID,
					"sourceService": assignment.SourceService,
				},
			}); err != nil {
				return err
			}
		}

		// The request needs another provider. Safety declines require a human
		// to look before any automatic reassignment runs.
		if err := s.opts.Assignments.CreateReassignmentCase(ctx, tx, assignment.OrganizationID,
			&models.ReassignmentCase{
				RequestProjectionID:   assignment.RequestProjectionID,
				OriginAssignmentID:    assignment.ID,
				Trigger:               models.TriggerDeclined,
				ReasonCode:            &reason,
				RequiresHumanReview:   safety,
				ExcludedMembershipIDs: []string{assignment.MembershipID},
			}); err != nil {
			return err
		}

		projection, err := s.opts.Projections.LockByID(ctx, tx, assignment.OrganizationID, assignment.RequestProjectionID)
		if err == nil {
			_ = s.opts.Projections.SetStatus(ctx, tx, assignment.OrganizationID, projection.ID,
				models.ProjectionOrganizationAccepted, projection.Version)
		}

		entry := audit.Entry{
			OrganizationID: assignment.OrganizationID,
			ActorType:      "provider", ActorID: assignment.MembershipID,
			Action: "assignment.declined", ResourceType: "assignment", ResourceID: assignment.ID,
			// The reason code is recorded; sensitive free text is not copied
			// into the audit summary.
			NewValues: map[string]any{"status": models.AssignmentDeclined, "reasonCode": input.ReasonCode,
				"safetyRelated": safety},
			Reason:    input.ReasonCode,
			RequestID: input.RequestID, CorrelationID: input.CorrelationID,
		}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}

		if err := s.opts.Events.Publish(ctx, tx, "organization.assignment.declined.v1", "assignment",
			assignment.ID, assignment.Version+1, assignment.OrganizationID, input.CorrelationID,
			map[string]any{
				"assignmentId":   assignment.ID,
				"organizationId": assignment.OrganizationID,
				"membershipId":   assignment.MembershipID,
				"reasonCode":     input.ReasonCode,
				"safetyRelated":  safety,
			}); err != nil {
			return err
		}
		if safety {
			if err := s.opts.Events.Publish(ctx, tx, "organization.assignment.safety_followup_requested.v1",
				"assignment", assignment.ID, assignment.Version+1, assignment.OrganizationID,
				input.CorrelationID, map[string]any{
					"assignmentId": assignment.ID,
					"reasonCode":   input.ReasonCode,
				}); err != nil {
				return err
			}
		}
		if err := s.opts.Events.Publish(ctx, tx, "organization.assignment.reassignment_required.v1",
			"assignment", assignment.ID, assignment.Version+1, assignment.OrganizationID,
			input.CorrelationID, map[string]any{
				"assignmentId":        assignment.ID,
				"requestProjectionId": assignment.RequestProjectionID,
				"trigger":             models.TriggerDeclined,
				"requiresHumanReview": safety,
			}); err != nil {
			return err
		}

		assignment.Status = models.AssignmentDeclined
		assignment.Version = input.ExpectedVersion + 1
		result.Assignment = assignment
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_assignment_declines_total",
		map[string]string{"category": declineCategory(input.ReasonCode)}, 1)
	if safety {
		s.opts.Metrics.IncCounter("fleet_partner_safety_declines_total", nil, 1)
	}

	result.AllowedActions = []string{}
	return &result, nil
}

// declineCategory keeps the decline metric low-cardinality and avoids
// exposing a per-provider decline tally as an operational signal.
func declineCategory(reasonCode string) string {
	if models.SafetyProtected(reasonCode) {
		return "safety_or_welfare"
	}
	switch reasonCode {
	case models.DeclineServiceMismatch, models.DeclineCapacityMismatch,
		models.DeclineLocationMismatch, models.DeclineExcessiveDistance:
		return "matching_or_data"
	case models.DeclineAlreadyEngaged, models.DeclineShiftEnding:
		return "availability"
	case models.DeclineDeviceOrNetworkIssue:
		return "technical"
	default:
		return "other"
	}
}

// lockOwnedAssignment loads the assignment FOR UPDATE and proves the caller
// owns it. Ownership is enforced in SQL, never taken from the request.
func (s *Service) lockOwnedAssignment(ctx context.Context, tx pgx.Tx, assignmentID string, actor ProviderActor) (*models.Assignment, error) {
	assignment, err := s.opts.Assignments.LockForProvider(ctx, tx, assignmentID, actor.MembershipID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			// Do not disclose that the assignment exists for someone else.
			return nil, errorcodes.New(404, errorcodes.AssignmentNotFound, "Assignment not found.", nil)
		}
		return nil, err
	}
	// The provider must belong to the service that owns the work.
	if actor.SourceService != "" && assignment.SourceService != actor.SourceService {
		return nil, errorcodes.Forbidden(errorcodes.AssignmentProviderMismatch,
			"This assignment belongs to a different service.")
	}
	if actor.ExecutingProviderID != "" && assignment.ExecutingProviderID != actor.ExecutingProviderID {
		return nil, errorcodes.Forbidden(errorcodes.AssignmentProviderMismatch,
			"This assignment belongs to a different provider.")
	}
	return assignment, nil
}

// assertRespondable checks the assignment is still awaiting this provider.
func (s *Service) assertRespondable(assignment *models.Assignment, expectedVersion int64) error {
	if assignment.Status != models.AssignmentPendingProviderResponse {
		return errorcodes.Conflict(errorcodes.AssignmentAlreadyResolved,
			"This assignment is no longer awaiting a response.")
	}
	if !s.opts.Now().Before(assignment.ResponseDeadline) {
		return errorcodes.Conflict(errorcodes.AssignmentResponseExpired,
			"The response window for this assignment has closed.")
	}
	if expectedVersion != 0 && expectedVersion != assignment.Version {
		return errorcodes.Conflict(errorcodes.ExpectedVersionMismatch,
			"This assignment changed. Reload and try again.")
	}
	return nil
}

// assertRequestStillAssignable refuses to proceed on a stale, gapped or
// resolved request.
func (s *Service) assertRequestStillAssignable(projection *models.RequestProjection) error {
	if models.BlocksStateChangingCommands(projection.FreshnessState) {
		return errorcodes.Conflict(errorcodes.MarketplaceProjectionGap,
			"This request's status is being re-synchronised. Try again shortly.")
	}
	switch projection.ProjectionStatus {
	case models.ProjectionCancelled, models.ProjectionExpired, models.ProjectionCompleted:
		return errorcodes.Conflict(errorcodes.RequestNotAssignable,
			"This request is no longer available.")
	}
	return nil
}

// recheckEligibility re-runs organization and source eligibility at response
// time.
func (s *Service) recheckEligibility(ctx context.Context, assignment *models.Assignment, projection *models.RequestProjection) error {
	if s.opts.Eligibility == nil {
		return nil
	}
	input := MembershipInput{
		OrganizationID:  assignment.OrganizationID,
		MembershipID:    assignment.MembershipID,
		ServiceCategory: assignment.SourceService,
		SourceRequestID: projection.PlatformRequestID,
		RequiredUntil:   s.opts.Now().Add(time.Hour),
	}
	if assignment.AssetReference != nil {
		input.AssetReference = *assignment.AssetReference
	}

	membership, decision, err := s.opts.Eligibility.CheckMembership(ctx, input)
	if err != nil {
		return err
	}
	if !decision.Eligible {
		return errorcodes.Conflict(apperrorsCode(decision.ReasonCode), "You are no longer eligible for this work.")
	}

	// Only consult the source service when its live integration is enabled.
	if s.opts.Flags.SourceEnabled(assignment.SourceService) && membership.ProviderID != nil {
		sourceDecision, err := s.opts.Eligibility.CheckSource(ctx,
			platformclients.CallMeta{}, input, *membership.ProviderID)
		if err != nil {
			return errorcodes.Unavailable(errorcodes.SourceServiceUnavailable,
				"We could not confirm eligibility right now. Please try again.")
		}
		if !sourceDecision.Eligible {
			return errorcodes.Conflict(apperrorsCode(sourceDecision.ReasonCode),
				"You are not currently eligible for this work.")
		}
	}
	return nil
}

// findReplay returns the original outcome when a client retries a response.
func (s *Service) findReplay(ctx context.Context, membershipID string, clientResponseID string, assignmentID string) (*AssignmentResult, error) {
	if clientResponseID == "" {
		return nil, nil
	}
	response, err := s.opts.Assignments.FindResponseByClientID(ctx, membershipID, clientResponseID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	// The same client response ID must not be reused for a different
	// assignment.
	if response.AssignmentID != assignmentID {
		return nil, errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
			"This response identifier was already used for another assignment.")
	}
	assignment, err := s.opts.Assignments.GetByID(ctx, response.OrganizationID, response.AssignmentID)
	if err != nil {
		return nil, err
	}
	return &AssignmentResult{
		Assignment:           assignment,
		AllowedActions:       assignment.AllowedActions(s.opts.Now()),
		ExecutionStartStatus: assignment.ExecutionStartStatus,
		Replayed:             true,
	}, nil
}

func mapTransitionError(err error) error {
	switch {
	case errors.Is(err, concurrency.ErrVersionMismatch):
		return errorcodes.Conflict(errorcodes.ExpectedVersionMismatch,
			"This assignment changed. Reload and try again.")
	case errors.Is(err, repositories.ErrInvalidTransition):
		return errorcodes.Conflict(errorcodes.AssignmentAlreadyResolved,
			"This assignment is no longer in a state that allows that action.")
	default:
		return err
	}
}

func apperrorsCode(reason string) apperrors.Code {
	if reason == "" {
		return errorcodes.ProviderNotEligible
	}
	return apperrors.Code(reason)
}

// Repositories exposes read paths to handlers.
func (s *Service) Repositories() (*repositories.ProjectionRepository, *repositories.OfferRepository, *repositories.AssignmentRepository, *repositories.TripRepository) {
	return s.opts.Projections, s.opts.Offers, s.opts.Assignments, s.opts.Trips
}

// Clock exposes the service clock for handlers computing allowedActions.
func (s *Service) Clock() time.Time { return s.opts.Now() }

var _ = tenant.Query{}
