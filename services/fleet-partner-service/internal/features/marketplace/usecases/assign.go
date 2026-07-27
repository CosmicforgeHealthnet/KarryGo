package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// AssignInput is a dispatcher assigning work to a managed provider.
type AssignInput struct {
	Tenant                 tenant.Context
	ProjectionID           string
	MembershipID           string
	AssetReference         string
	ResponseTimeoutSeconds int
	ExpectedRequestVersion int64
	IdempotencyKey         string
}

// Assign creates a PENDING_PROVIDER_RESPONSE assignment.
//
// Creating an assignment is not accepting work: the provider still chooses.
// The assignment is created with both actions available and a deadline, and
// provider plus asset capacity is reserved so two dispatchers cannot commit
// the same provider twice.
func (s *Service) Assign(ctx context.Context, input AssignInput) (*models.Assignment, error) {
	if !s.opts.Flags.AssignmentEnabled {
		return nil, errorcodes.Unavailable(errorcodes.SourceServiceUnavailable,
			"Assignment creation is not enabled.")
	}
	if input.IdempotencyKey == "" {
		return nil, apperrors.BadRequest("Idempotency-Key header is required.", nil)
	}

	timeout, err := s.resolveTimeout(input.ResponseTimeoutSeconds)
	if err != nil {
		return nil, err
	}

	projection, err := s.opts.Projections.GetByID(ctx, input.Tenant.OrganizationID, input.ProjectionID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, apperrors.NotFound("Request not found.", nil)
		}
		return nil, err
	}
	if err := s.assertFreshEnough(projection); err != nil {
		return nil, err
	}
	// The organization must already hold the request.
	if projection.ProjectionStatus != models.ProjectionOrganizationAccepted {
		return nil, errorcodes.Conflict(errorcodes.RequestNotAssignable,
			"This request is not ready for assignment.")
	}

	deadline := s.opts.Now().Add(timeout)
	eligibilityInput := MembershipInput{
		OrganizationID:  input.Tenant.OrganizationID,
		MembershipID:    input.MembershipID,
		ServiceCategory: projection.SourceService,
		SourceRequestID: projection.PlatformRequestID,
		AssetReference:  input.AssetReference,
		RequiredUntil:   deadline.Add(time.Hour),
	}
	membership, decision, err := s.opts.Eligibility.CheckMembership(ctx, eligibilityInput)
	if err != nil {
		return nil, err
	}
	if !decision.Eligible {
		return nil, errorcodes.Conflict(apperrorsCode(decision.ReasonCode),
			"That provider cannot take this work right now.")
	}
	if s.opts.Flags.SourceEnabled(projection.SourceService) && membership.ProviderID != nil {
		sourceDecision, err := s.opts.Eligibility.CheckSource(ctx, callMeta(input.Tenant), eligibilityInput, *membership.ProviderID)
		if err != nil {
			return nil, errorcodes.Unavailable(errorcodes.SourceServiceUnavailable,
				"We could not confirm provider eligibility right now.")
		}
		if !sourceDecision.Eligible {
			return nil, errorcodes.Conflict(apperrorsCode(sourceDecision.ReasonCode),
				"That provider cannot take this work right now.")
		}
	}

	assignment := &models.Assignment{
		RequestProjectionID: projection.ID,
		MembershipID:        input.MembershipID,
		ExecutingProviderID: derefString(membership.ProviderID),
		SourceService:       projection.SourceService,
		AssignedByType:      "ORGANIZATION_USER",
		AssignedByID:        input.Tenant.OrganizationUserID,
		Status:              models.AssignmentCreated,
		ResponseDeadline:    deadline,
	}
	if input.AssetReference != "" {
		assignment.AssetReference = &input.AssetReference
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		locked, err := s.opts.Projections.LockByID(ctx, tx, input.Tenant.OrganizationID, projection.ID)
		if err != nil {
			return err
		}
		if input.ExpectedRequestVersion != 0 && input.ExpectedRequestVersion != locked.Version {
			return errorcodes.Conflict(errorcodes.ExpectedVersionMismatch,
				"This request changed. Reload and try again.")
		}

		offer, offerErr := s.opts.Offers.GetLiveByProjection(ctx, tx, input.Tenant.OrganizationID, projection.ID)
		if offerErr == nil {
			assignment.OfferID = &offer.ID
		}

		if err := s.opts.Assignments.Create(ctx, tx, input.Tenant.OrganizationID, assignment); err != nil {
			if errors.Is(err, repositories.ErrAssignmentConflict) {
				return errorcodes.Conflict(errorcodes.AssignmentConflict,
					"This request already has a live assignment.")
			}
			return err
		}

		// Reserve capacity. A second dispatcher assigning the same provider
		// loses on the partial unique index rather than double-booking them.
		if err := s.opts.Assignments.HoldReservation(ctx, tx, input.Tenant.OrganizationID,
			assignment.ID, models.ResourceProvider, input.MembershipID); err != nil {
			if errors.Is(err, repositories.ErrAssignmentConflict) {
				return errorcodes.Conflict(errorcodes.ProviderHasConflictingWork,
					"That provider is already committed to other work.")
			}
			return err
		}
		if input.AssetReference != "" {
			if err := s.opts.Assignments.HoldReservation(ctx, tx, input.Tenant.OrganizationID,
				assignment.ID, models.ResourceAsset, input.AssetReference); err != nil {
				if errors.Is(err, repositories.ErrAssignmentConflict) {
					return errorcodes.Conflict(errorcodes.VehicleConflict,
						"That vehicle is already committed to other work.")
				}
				return err
			}
		}

		if err := s.opts.Assignments.Transition(ctx, tx, assignment,
			models.AssignmentPendingProviderResponse, assignment.Version, models.StateHistoryEntry{
				ActorType: "ORGANIZATION_USER", ActorID: input.Tenant.OrganizationUserID,
				RequestID: input.Tenant.RequestID, CorrelationID: input.Tenant.CorrelationID,
			}); err != nil {
			return err
		}
		if err := s.opts.Projections.SetStatus(ctx, tx, input.Tenant.OrganizationID, projection.ID,
			models.ProjectionAssignmentPending, locked.Version); err != nil {
			return err
		}

		entry := audit.EntryFromTenant(input.Tenant, "assignment.created", "assignment", assignment.ID)
		entry.NewValues = map[string]any{"membershipId": input.MembershipID, "deadline": deadline}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.assignment.created.v1", "assignment",
			assignment.ID, 1, input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{
				"assignmentId":        assignment.ID,
				"requestProjectionId": projection.ID,
				"membershipId":        input.MembershipID,
				"sourceService":       projection.SourceService,
				"responseDeadline":    deadline,
			})
	})
	if err != nil {
		return nil, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_assignments_created_total",
		map[string]string{"source": projection.SourceService}, 1)

	// Notification delivery is deliberately NOT required to commit: the
	// provider can fetch pending work after reconnecting until the deadline.
	assignment.Status = models.AssignmentPendingProviderResponse
	assignment.Version = 2
	return assignment, nil
}

func (s *Service) resolveTimeout(seconds int) (time.Duration, error) {
	if seconds <= 0 {
		return s.opts.ResponseTimeoutDefault, nil
	}
	timeout := time.Duration(seconds) * time.Second
	if timeout < s.opts.ResponseTimeoutMin || timeout > s.opts.ResponseTimeoutMax {
		return 0, apperrors.Validation("The response timeout is outside the allowed range.",
			[]apperrors.FieldViolation{{Field: "responseTimeoutSeconds", Message: "Out of range."}})
	}
	return timeout, nil
}

// ExpireAssignments expires overdue pending assignments.
//
// Expiry NEVER auto-accepts. It releases the provider and asset, opens a
// reassignment case and notifies dispatchers, so the work goes back to a
// human rather than being silently committed to an unresponsive provider.
func (s *Service) ExpireAssignments(ctx context.Context, limit int) (int, error) {
	expired := 0
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		claimed, err := s.opts.Assignments.ClaimExpired(ctx, tx, limit)
		if err != nil {
			return err
		}
		for index := range claimed {
			assignment := claimed[index]
			// Re-check the deadline under lock: a provider may have answered
			// between the scan and the update.
			if s.opts.Now().Before(assignment.ResponseDeadline) {
				continue
			}
			if assignment.Status != models.AssignmentPendingProviderResponse {
				continue
			}

			if err := s.opts.Assignments.Transition(ctx, tx, &assignment, models.AssignmentExpired,
				assignment.Version, models.StateHistoryEntry{
					ActorType: "SYSTEM", ActorID: "assignment-expiry",
				}); err != nil {
				if errors.Is(err, repositories.ErrInvalidTransition) {
					continue
				}
				return err
			}
			if err := s.opts.Assignments.ReleaseReservations(ctx, tx, assignment.ID); err != nil {
				return err
			}
			if err := s.opts.Assignments.CreateReassignmentCase(ctx, tx, assignment.OrganizationID,
				&models.ReassignmentCase{
					RequestProjectionID:   assignment.RequestProjectionID,
					OriginAssignmentID:    assignment.ID,
					Trigger:               models.TriggerExpired,
					ExcludedMembershipIDs: []string{assignment.MembershipID},
				}); err != nil {
				return err
			}
			if err := s.opts.Events.Publish(ctx, tx, "organization.assignment.expired.v1", "assignment",
				assignment.ID, assignment.Version+1, assignment.OrganizationID, "",
				map[string]any{
					"assignmentId":        assignment.ID,
					"requestProjectionId": assignment.RequestProjectionID,
					"membershipId":        assignment.MembershipID,
				}); err != nil {
				return err
			}
			expired++
		}
		return nil
	})
	if err != nil {
		return expired, err
	}
	if expired > 0 {
		s.opts.Metrics.IncCounter("fleet_partner_assignment_expiries_total", nil, float64(expired))
	}
	return expired, nil
}

// ReassignInput replaces an unresolved or resolved assignment.
type ReassignInput struct {
	Tenant                 tenant.Context
	AssignmentID           string
	MembershipID           string
	AssetReference         string
	ResponseTimeoutSeconds int
	Reason                 string
	IdempotencyKey         string
}

// Reassign creates a replacement assignment for another provider.
//
// The original assignment is never rewritten: its resolved state and its
// provider's response stay immutable. The replacement links back through
// rootAssignmentId / replacesAssignmentId, and the new provider gets a fresh
// Accept/Decline choice.
func (s *Service) Reassign(ctx context.Context, input ReassignInput) (*models.Assignment, error) {
	if !s.opts.Flags.ReassignmentEnabled {
		return nil, errorcodes.Unavailable(errorcodes.SourceServiceUnavailable,
			"Reassignment is not enabled.")
	}
	timeout, err := s.resolveTimeout(input.ResponseTimeoutSeconds)
	if err != nil {
		return nil, err
	}

	var replacement *models.Assignment
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		current, err := s.opts.Assignments.LockByID(ctx, tx, input.AssignmentID)
		if err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				return apperrors.NotFound("Assignment not found.", nil)
			}
			return err
		}
		if current.OrganizationID != input.Tenant.OrganizationID {
			return apperrors.NotFound("Assignment not found.", nil)
		}

		// An assignment already accepted or executing cannot simply be
		// replaced: that would strip a provider of committed work.
		switch current.Status {
		case models.AssignmentDeclined, models.AssignmentExpired, models.AssignmentCancelled:
			// Already resolved: a replacement is exactly what is needed.
		case models.AssignmentPendingProviderResponse:
			// Cancel the outstanding offer first so the provider does not
			// answer an assignment that no longer exists.
			if err := s.opts.Assignments.Transition(ctx, tx, current, models.AssignmentCancelled,
				current.Version, models.StateHistoryEntry{
					ActorType: "ORGANIZATION_USER", ActorID: input.Tenant.OrganizationUserID,
					ReasonCode: &input.Reason,
				}); err != nil {
				return mapTransitionError(err)
			}
			if err := s.opts.Assignments.ReleaseReservations(ctx, tx, current.ID); err != nil {
				return err
			}
		default:
			return errorcodes.Conflict(errorcodes.AssignmentCannotReassign,
				"This assignment can no longer be reassigned.")
		}

		projection, err := s.opts.Projections.LockByID(ctx, tx, input.Tenant.OrganizationID, current.RequestProjectionID)
		if err != nil {
			return err
		}

		membership, decision, err := s.opts.Eligibility.CheckMembership(ctx, MembershipInput{
			OrganizationID:  input.Tenant.OrganizationID,
			MembershipID:    input.MembershipID,
			ServiceCategory: current.SourceService,
			SourceRequestID: projection.PlatformRequestID,
			AssetReference:  input.AssetReference,
			RequiredUntil:   s.opts.Now().Add(timeout).Add(time.Hour),
		})
		if err != nil {
			return err
		}
		if !decision.Eligible {
			return errorcodes.Conflict(apperrorsCode(decision.ReasonCode),
				"That provider cannot take this work right now.")
		}

		root := current.RootAssignmentID
		if root == nil {
			root = &current.ID
		}
		replacement = &models.Assignment{
			RequestProjectionID:  current.RequestProjectionID,
			OfferID:              current.OfferID,
			MembershipID:         input.MembershipID,
			ExecutingProviderID:  derefString(membership.ProviderID),
			SourceService:        current.SourceService,
			AssignedByType:       "ORGANIZATION_USER",
			AssignedByID:         input.Tenant.OrganizationUserID,
			Status:               models.AssignmentCreated,
			ResponseDeadline:     s.opts.Now().Add(timeout),
			RootAssignmentID:     root,
			ReplacesAssignmentID: &current.ID,
			ReassignmentAttempt:  current.ReassignmentAttempt + 1,
			ReassignmentReason:   &input.Reason,
		}
		if input.AssetReference != "" {
			replacement.AssetReference = &input.AssetReference
		}

		if err := s.opts.Assignments.Create(ctx, tx, input.Tenant.OrganizationID, replacement); err != nil {
			if errors.Is(err, repositories.ErrAssignmentConflict) {
				return errorcodes.Conflict(errorcodes.AssignmentConflict,
					"This request already has a live assignment.")
			}
			return err
		}
		if err := s.opts.Assignments.HoldReservation(ctx, tx, input.Tenant.OrganizationID,
			replacement.ID, models.ResourceProvider, input.MembershipID); err != nil {
			return errorcodes.Conflict(errorcodes.ProviderHasConflictingWork,
				"That provider is already committed to other work.")
		}
		if err := s.opts.Assignments.Transition(ctx, tx, replacement,
			models.AssignmentPendingProviderResponse, replacement.Version, models.StateHistoryEntry{
				ActorType: "ORGANIZATION_USER", ActorID: input.Tenant.OrganizationUserID,
				Metadata: map[string]any{"replacesAssignmentId": current.ID},
			}); err != nil {
			return err
		}
		if err := s.opts.Projections.SetStatus(ctx, tx, input.Tenant.OrganizationID,
			projection.ID, models.ProjectionAssignmentPending, projection.Version); err != nil {
			return err
		}

		if openCase, caseErr := s.opts.Assignments.GetOpenReassignmentCase(ctx,
			input.Tenant.OrganizationID, projection.ID); caseErr == nil {
			_ = s.opts.Assignments.ResolveReassignmentCase(ctx, tx, openCase.ID, replacement.ID, openCase.Version)
		}

		entry := audit.EntryFromTenant(input.Tenant, "assignment.reassigned", "assignment", replacement.ID)
		entry.PreviousValues = map[string]any{"assignmentId": current.ID, "membershipId": current.MembershipID}
		entry.NewValues = map[string]any{"assignmentId": replacement.ID, "membershipId": input.MembershipID}
		entry.Reason = input.Reason
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.assignment.reassigned.v1", "assignment",
			replacement.ID, 1, input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{
				"assignmentId":         replacement.ID,
				"replacesAssignmentId": current.ID,
				"membershipId":         input.MembershipID,
				"attempt":              replacement.ReassignmentAttempt,
			})
	})
	if err != nil {
		return nil, err
	}
	s.opts.Metrics.IncCounter("fleet_partner_assignment_reassignments_total", nil, 1)
	replacement.Status = models.AssignmentPendingProviderResponse
	replacement.Version = 2
	return replacement, nil
}

// CancelInput cancels an assignment before execution.
type CancelInput struct {
	Tenant          tenant.Context
	AssignmentID    string
	Reason          string
	ExpectedVersion int64
}

// Cancel cancels an assignment.
//
// If canonical execution has already started, cancellation must go through
// the source service's trip-cancellation contract — a live trip is never
// cancelled locally without source confirmation.
func (s *Service) Cancel(ctx context.Context, input CancelInput) error {
	if input.Reason == "" {
		return apperrors.Validation("A cancellation reason is required.",
			[]apperrors.FieldViolation{{Field: "reason", Message: "Required."}})
	}

	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		assignment, err := s.opts.Assignments.LockByID(ctx, tx, input.AssignmentID)
		if err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				return apperrors.NotFound("Assignment not found.", nil)
			}
			return err
		}
		if assignment.OrganizationID != input.Tenant.OrganizationID {
			return apperrors.NotFound("Assignment not found.", nil)
		}
		if assignment.Status == models.AssignmentInProgress || assignment.SourceExecutionID != nil {
			return errorcodes.Conflict(errorcodes.AssignmentCannotCancel,
				"This work has already started. Cancel it through the trip instead.")
		}
		// EXECUTION_START_PENDING is the commit point for canonical execution:
		// the execution-start worker has moved the assignment into this state
		// and is calling the source service right now.
		//
		// This is what closes the acceptance/cancellation race. Dispatch cannot
		// take a lock in this service, so a cancellation landing between
		// dispatch verifying acceptance and dispatch committing its trip would
		// otherwise produce a live canonical trip for a cancelled assignment.
		// Refusing here means a cancel either lands BEFORE the worker claims
		// the assignment — in which case dispatch's verification sees
		// CANCELLED and creates nothing — or it waits until the outcome is
		// known and is then handled through trip cancellation. It is never
		// silently interleaved.
		if assignment.Status == models.AssignmentExecutionStartPending {
			return errorcodes.Conflict(errorcodes.AssignmentCannotCancel,
				"This assignment is starting execution. Try again shortly, or cancel the trip once it exists.")
		}
		if !models.CanTransition(assignment.Status, models.AssignmentCancelled) {
			return errorcodes.Conflict(errorcodes.AssignmentCannotCancel,
				"This assignment can no longer be cancelled.")
		}

		expected := input.ExpectedVersion
		if expected == 0 {
			expected = assignment.Version
		}
		if err := s.opts.Assignments.Transition(ctx, tx, assignment, models.AssignmentCancelled,
			expected, models.StateHistoryEntry{
				ActorType: "ORGANIZATION_USER", ActorID: input.Tenant.OrganizationUserID,
				ReasonCode: &input.Reason,
				RequestID:  input.Tenant.RequestID, CorrelationID: input.Tenant.CorrelationID,
			}); err != nil {
			return mapTransitionError(err)
		}
		if err := s.opts.Assignments.ReleaseReservations(ctx, tx, assignment.ID); err != nil {
			return err
		}

		entry := audit.EntryFromTenant(input.Tenant, "assignment.cancelled", "assignment", assignment.ID)
		entry.Reason = input.Reason
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.assignment.cancelled.v1", "assignment",
			assignment.ID, assignment.Version+1, assignment.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{"assignmentId": assignment.ID, "reason": input.Reason})
	})
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func callMeta(t tenant.Context) platformclients.CallMeta {
	return platformclients.CallMeta{RequestID: t.RequestID, CorrelationID: t.CorrelationID}
}
