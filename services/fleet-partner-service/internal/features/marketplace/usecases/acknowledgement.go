package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// EventAssignmentAcknowledged is the corporate event emitted when the assigned
// party confirms receipt.
//
// Named "acknowledged" rather than any word in the acceptance family so no
// downstream consumer can mistake it for a commitment to perform the work.
const EventAssignmentAcknowledged = "organization.assignment.acknowledged.v1"

// AcknowledgeAssignmentInput is a confirmation that work was received.
//
// It carries no decision field. Acknowledgement records that an assignment
// reached the assigned party; whether they will do it is answered separately,
// and later, by Accept or Decline.
type AcknowledgeAssignmentInput struct {
	Actor           ProviderActor
	AssignmentID    string
	IdempotencyKey  string
	ExpectedVersion int64
	RequestID       string
	CorrelationID   string
}

// AcknowledgementResult is what the caller learns.
type AcknowledgementResult struct {
	Assignment *models.Assignment
	// AllowedActions is the server's answer to "what may this provider do
	// now?". Acknowledging never removes an entry from it: a provider who has
	// confirmed receipt still has both Accept and Decline.
	AllowedActions       []string
	AcknowledgementState string
	Replayed             bool
}

// AcknowledgeAssignmentForProvider records receipt relayed by the owning
// execution service.
//
// Mirrors AcceptAssignmentForProvider: the asserted provider is bound to the
// assignment before anything is written, so a valid service signature can
// never be used to acknowledge on another provider's behalf.
func (s *Service) AcknowledgeAssignmentForProvider(ctx context.Context,
	input AcknowledgeAssignmentInput) (*AcknowledgementResult, error) {
	resolved, err := s.bindProviderToAssignment(ctx, input.AssignmentID, input.Actor)
	if err != nil {
		return nil, err
	}
	input.Actor = resolved
	return s.AcknowledgeAssignment(ctx, input)
}

// AcknowledgeAssignment records that the assigned party has seen the work.
//
// What this deliberately does NOT do:
//   - it never writes assignment.status, so acknowledgement cannot become
//     acceptance by accident or by a later edit to the UPDATE statement;
//   - it never consults an acceptance feature flag, because confirming receipt
//     of work already shown to a provider is not a privilege to switch off;
//   - it never narrows AllowedActions, so a provider who acknowledges keeps
//     both Accept and Decline, including a safety decline.
//
// Everything that must be all-or-nothing — the immutable acknowledgement, the
// assignment flag, the state history row, the audit record and the outbox
// event — commits in one transaction.
func (s *Service) AcknowledgeAssignment(ctx context.Context,
	input AcknowledgeAssignmentInput) (*AcknowledgementResult, error) {
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An idempotency key is required.")
	}
	if strings.TrimSpace(input.AssignmentID) == "" {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An assignment id is required.")
	}

	requestFingerprint := acknowledgementFingerprint(input)

	// A retry replays the original outcome; a reused key carrying different
	// inputs is rejected rather than silently answered with someone else's
	// result.
	if replay, err := s.findAcknowledgementReplay(ctx, input, requestFingerprint); err != nil {
		return nil, err
	} else if replay != nil {
		return replay, nil
	}

	if err := s.assertActorMayAcknowledge(ctx, input.Actor); err != nil {
		return nil, err
	}

	var result AcknowledgementResult
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		assignment, err := s.lockOwnedAssignment(ctx, tx, input.AssignmentID, input.Actor)
		if err != nil {
			return err
		}
		if !models.AcknowledgeableStatus(assignment.Status) {
			return errorcodes.Conflict(errorcodes.AssignmentNotAcknowledgeable,
				"This assignment is no longer in a state that can be acknowledged.")
		}
		if assignment.Acknowledged() {
			return errorcodes.Conflict(errorcodes.AssignmentAlreadyAcknowledged,
				"This assignment has already been acknowledged.")
		}
		// A caller that names a version must be acting on the version it saw.
		// Zero means "no opinion", which is how a client with no cached copy
		// acknowledges without guessing.
		if input.ExpectedVersion != 0 && input.ExpectedVersion != assignment.Version {
			return errorcodes.Conflict(errorcodes.ExpectedVersionMismatch,
				"This assignment changed. Reload and try again.")
		}

		record := &models.Acknowledgement{
			OrganizationID:      assignment.OrganizationID,
			AssignmentID:        assignment.ID,
			MembershipID:        assignment.MembershipID,
			ExecutingProviderID: assignment.ExecutingProviderID,
			SourceService:       assignment.SourceService,
			ActorType:           models.AcknowledgementActorProvider,
			ActorID:             assignment.MembershipID,
			ExpectedVersion:     input.ExpectedVersion,
			IdempotencyKey:      key,
			RequestFingerprint:  requestFingerprint,
			RequestID:           input.RequestID,
			CorrelationID:       input.CorrelationID,
		}
		if err := s.opts.Assignments.CreateAcknowledgement(ctx, tx, assignment,
			assignment.Version, record); err != nil {
			switch {
			case errors.Is(err, repositories.ErrAcknowledgementExists):
				return errorcodes.Conflict(errorcodes.AssignmentAlreadyAcknowledged,
					"This assignment has already been acknowledged.")
			case errors.Is(err, concurrency.ErrVersionMismatch):
				return errorcodes.Conflict(errorcodes.ExpectedVersionMismatch,
					"This assignment changed. Reload and try again.")
			default:
				return err
			}
		}

		// History records the acknowledgement against the status the assignment
		// was in. From and to are equal because nothing moved — which is the
		// point, and an operator reading the history needs to see it.
		if err := s.opts.Assignments.AppendAcknowledgementHistory(ctx, tx, assignment,
			models.StateHistoryEntry{
				ActorType: models.AcknowledgementActorProvider,
				ActorID:   assignment.MembershipID,
				Metadata: map[string]any{
					"acknowledgementId": record.ID,
					"sourceService":     assignment.SourceService,
				},
				RequestID: input.RequestID, CorrelationID: input.CorrelationID,
			}); err != nil {
			return err
		}

		if err := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: assignment.OrganizationID,
			ActorType:      "provider", ActorID: assignment.MembershipID,
			Action: "assignment.acknowledged", ResourceType: "assignment",
			ResourceID: assignment.ID,
			NewValues: map[string]any{
				"acknowledgementState": models.AcknowledgementAcknowledged,
				// The status is recorded UNCHANGED so an auditor can see for
				// themselves that acknowledgement moved nothing.
				"status": assignment.Status,
			},
			RequestID: input.RequestID, CorrelationID: input.CorrelationID,
		}); err != nil {
			return err
		}

		if err := s.opts.Events.Publish(ctx, tx, EventAssignmentAcknowledged, "assignment",
			assignment.ID, assignment.Version+1, assignment.OrganizationID, input.CorrelationID,
			map[string]any{
				"assignmentId":   assignment.ID,
				"organizationId": assignment.OrganizationID,
				"membershipId":   assignment.MembershipID,
				"sourceService":  assignment.SourceService,
				"status":         assignment.Status,
				"acknowledged":   true,
				// Stated explicitly so a consumer never has to infer it from
				// the absence of a field.
				"impliesAcceptance": false,
			}); err != nil {
			return err
		}

		assignment.AcknowledgementState = models.AcknowledgementAcknowledged
		assignment.Version = assignment.Version + 1
		result.Assignment = assignment
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_assignment_acknowledgements_total",
		map[string]string{"source": result.Assignment.SourceService}, 1)

	result.AcknowledgementState = models.AcknowledgementAcknowledged
	result.AllowedActions = result.Assignment.AllowedActions(s.opts.Now())

	// An organization may require acknowledgement before departure. When it
	// does, an acceptance that arrived first was parked rather than dispatched,
	// and this is the moment it becomes allowed to leave. Done after commit
	// because it makes an external call.
	s.resumeBlockedDeparture(ctx, result.Assignment, input.CorrelationID)
	return &result, nil
}

// acknowledgementFingerprint hashes the inputs that define this command.
//
// expectedVersion is included on purpose: a client that changes the version it
// claims to be acting on has issued a DIFFERENT command, and answering it with
// the earlier command's result would hide a real conflict behind a replay.
func acknowledgementFingerprint(input AcknowledgeAssignmentInput) string {
	return fingerprint(input.AssignmentID, input.Actor.ExecutingProviderID,
		input.Actor.SourceService, versionToken(input.ExpectedVersion))
}

// findAcknowledgementReplay resolves a retried acknowledgement.
//
// Three outcomes, and the middle one is the one that matters: the same key
// against a different assignment, or with different inputs, is a conflict.
// Returning the stored result in that case would let a client believe it had
// acknowledged something it had not.
func (s *Service) findAcknowledgementReplay(ctx context.Context,
	input AcknowledgeAssignmentInput, requestFingerprint string) (*AcknowledgementResult, error) {
	if input.Actor.MembershipID == "" {
		return nil, nil
	}
	record, err := s.opts.Assignments.FindAcknowledgementByIdempotencyKey(ctx,
		input.Actor.MembershipID, strings.TrimSpace(input.IdempotencyKey))
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if record.AssignmentID != input.AssignmentID {
		return nil, errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
			"This idempotency key was already used for another assignment.")
	}
	if record.RequestFingerprint != requestFingerprint {
		return nil, errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
			"This idempotency key was already used for a different request.")
	}

	assignment, err := s.opts.Assignments.GetByID(ctx, record.OrganizationID, record.AssignmentID)
	if err != nil {
		return nil, err
	}
	return &AcknowledgementResult{
		Assignment:           assignment,
		AllowedActions:       assignment.AllowedActions(s.opts.Now()),
		AcknowledgementState: assignment.AcknowledgementState,
		Replayed:             true,
	}, nil
}

// assertActorMayAcknowledge refuses suspended and revoked memberships.
//
// Narrower than the acceptance recheck by design. Acknowledgement is a factual
// record that work was seen, so a provider whose shift has lapsed or whose
// source-service eligibility has changed may still record it. What must be
// refused is an actor whose relationship with the organization has been
// suspended or ended: their signal is no longer theirs to give.
//
// A nil eligibility port means this deployment has no membership reader wired,
// which is the same posture every other membership check in this service
// takes: skip rather than fabricate a verdict.
func (s *Service) assertActorMayAcknowledge(ctx context.Context, actor ProviderActor) error {
	if s.opts.Eligibility == nil || s.opts.Eligibility.Providers == nil {
		return nil
	}
	if actor.OrganizationID == "" || actor.MembershipID == "" {
		return nil
	}
	membership, err := s.opts.Eligibility.Providers.GetMembership(ctx,
		actor.OrganizationID, actor.MembershipID)
	if err != nil {
		if errors.Is(err, ErrMembershipNotFound) {
			return errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
				"This provider may not acknowledge work for this organization.")
		}
		return err
	}
	switch membership.Status {
	case providermodels.StatusSuspended, providermodels.StatusRemoved,
		providermodels.StatusCancelled, providermodels.StatusDeclined,
		providermodels.StatusExpired:
		return errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"This provider may not acknowledge work right now.")
	}
	return nil
}

// requiresAcknowledgementBeforeDeparture reads the organization's policy.
//
// Errors resolve to "not required". The policy exists to add a gate an
// organization opted into; a transient read failure must not invent one for an
// organization that never asked for it, and the acceptance itself is already
// durable either way.
func (s *Service) requiresAcknowledgementBeforeDeparture(ctx context.Context, organizationID string) bool {
	required, err := s.opts.Assignments.RequiresAcknowledgement(ctx, nil, organizationID)
	if err != nil {
		return false
	}
	return required
}

// departureBlocked reports whether policy forbids starting execution now.
func (s *Service) departureBlocked(ctx context.Context, assignment *models.Assignment) bool {
	if assignment == nil || assignment.Acknowledged() {
		return false
	}
	return s.requiresAcknowledgementBeforeDeparture(ctx, assignment.OrganizationID)
}

// resumeBlockedDeparture starts execution for an acceptance that policy parked.
//
// The ordering problem this solves: a provider may accept before they
// acknowledge. Under a policy that requires acknowledgement before departure,
// that acceptance is committed but its execution start is withheld. Without
// this call the acceptance would sit forever — durable, correct, and never
// acted on.
func (s *Service) resumeBlockedDeparture(ctx context.Context, assignment *models.Assignment,
	correlationID string) {
	if assignment == nil || assignment.Status != models.AssignmentAccepted {
		return
	}
	if assignment.ExecutionStartStatus != models.ExecutionNotRequested {
		return
	}
	if !s.requiresAcknowledgementBeforeDeparture(ctx, assignment.OrganizationID) {
		return
	}
	s.requestExecution(ctx, assignment, correlationID)
}
