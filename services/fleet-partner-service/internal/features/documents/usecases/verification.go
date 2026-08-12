package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// The verification workflow.
//
// One rule governs everything here: only a SAFE document can become VERIFIED.
// PENDING, UNKNOWN, UNSAFE and QUARANTINED all refuse, because each means the
// platform does not actually know the content is clean — and a verification
// recorded against a file nobody vouched for is exactly the evidence an
// auditor would later find worthless.
//
// Verification never alters media's authority. It records that a human at this
// organization looked at a file media had already confirmed, and decided.

// VerificationInput is one verification decision.
type VerificationInput struct {
	Actor           Actor
	DocumentID      string
	ExpectedVersion int64
	// ReasonCode is mandatory on a rejection and must be a controlled value.
	ReasonCode    string
	ReasonNote    string
	CorrelationID string
	CausationID   string
	RequestID     string
}

// RequestVerification moves a document into the verification queue.
func (s *Service) RequestVerification(ctx context.Context, input VerificationInput) (*models.Document, error) {
	if err := s.writesAllowed(); err != nil {
		return nil, err
	}
	return s.transition(ctx, input.Actor, input.DocumentID, input.ExpectedVersion,
		input.CorrelationID, input.CausationID, input.RequestID,
		func(document *models.Document) error {
			if document.Terminal() {
				return errorcodes.Conflict(errorcodes.DocumentNotVerifiable,
					"This document is "+strings.ToLower(document.LifecycleState)+".")
			}
			if document.VerificationState == models.VerificationVerified {
				return errorcodes.Conflict(errorcodes.DocumentAlreadyVerified,
					"This document has already been verified.")
			}
			document.VerificationState = models.VerificationRequested
			document.LifecycleState = models.StateVerificationPending
			return nil
		},
		transitionEffects{
			action:       models.ActionVerificationRequest,
			auditAction:  "document.verification.requested",
			decision:     models.DecisionRequested,
			eventType:    EventDocumentStateChanged,
			metricReason: "requested",
		})
}

// Verify records a positive verification decision.
//
// The safety check is deliberately re-read from the row inside the
// transaction rather than trusted from the caller's earlier read: a
// media.unsafe_detected event that arrived in between must win.
func (s *Service) Verify(ctx context.Context, input VerificationInput) (*models.Document, error) {
	if err := s.writesAllowed(); err != nil {
		return nil, err
	}
	now := s.opts.Now().UTC()

	return s.transition(ctx, input.Actor, input.DocumentID, input.ExpectedVersion,
		input.CorrelationID, input.CausationID, input.RequestID,
		func(document *models.Document) error {
			if document.VerificationState == models.VerificationVerified &&
				document.LifecycleState == models.StateVerified {
				return errorcodes.Conflict(errorcodes.DocumentAlreadyVerified,
					"This document has already been verified.")
			}
			if verifiable, why := document.Verifiable(now); !verifiable {
				return errorcodes.Conflict(errorcodes.DocumentNotVerifiable,
					"This document cannot be verified: "+why+".")
			}
			// Separation of duties. The value of a verification step is that
			// somebody other than the submitter looked; a deployment may waive
			// it, but never by accident.
			if !s.opts.SelfVerificationAllowed &&
				document.CreatedByActorID != "" &&
				document.CreatedByActorID == input.Actor.ActorID {
				return errorcodes.Forbidden(errorcodes.DocumentSelfVerification,
					"You cannot verify a document you submitted.")
			}
			document.VerificationState = models.VerificationVerified
			document.LifecycleState = models.StateVerified
			document.RejectedReason = ""
			return nil
		},
		transitionEffects{
			action:       models.ActionVerified,
			auditAction:  "document.verified",
			decision:     models.DecisionVerified,
			eventType:    EventDocumentVerified,
			metricReason: "verified",
			// A newly verified document can only widen what is eligible, but
			// invalidating is still correct: the previous decision was taken
			// while this evidence was missing.
			invalidateReason: models.ReasonDocumentNotVerified,
		})
}

// Reject records a negative verification decision with a controlled reason.
func (s *Service) Reject(ctx context.Context, input VerificationInput) (*models.Document, error) {
	if err := s.writesAllowed(); err != nil {
		return nil, err
	}
	if !models.ValidRejectionReason(input.ReasonCode) {
		return nil, errorcodes.BadRequest(errorcodes.DocumentInvalid,
			"A controlled rejection reason is required.")
	}
	return s.transition(ctx, input.Actor, input.DocumentID, input.ExpectedVersion,
		input.CorrelationID, input.CausationID, input.RequestID,
		func(document *models.Document) error {
			if document.Terminal() {
				return errorcodes.Conflict(errorcodes.DocumentNotVerifiable,
					"This document is "+strings.ToLower(document.LifecycleState)+".")
			}
			document.VerificationState = models.VerificationRejected
			document.LifecycleState = models.StateRejected
			document.RejectedReason = input.ReasonCode
			return nil
		},
		transitionEffects{
			action:           models.ActionRejected,
			auditAction:      "document.rejected",
			decision:         models.DecisionRejected,
			reasonCode:       input.ReasonCode,
			reasonNote:       input.ReasonNote,
			eventType:        EventDocumentRejected,
			metricReason:     "rejected",
			invalidateReason: models.ReasonDocumentNotVerified,
		})
}

// InvalidateVerification voids a standing verification.
//
// Used when the evidence changed underneath it: media reported a new source
// version, the file became unsafe, or the object was deleted. It is not a
// rejection — nobody decided the document was wrong — so it is recorded as a
// distinct decision that a later reader can tell apart.
func (s *Service) InvalidateVerification(ctx context.Context, q database.Querier,
	document *models.Document, reason string, correlationID string) error {
	if document.VerificationState != models.VerificationVerified {
		return nil
	}
	document.VerificationState = models.VerificationInvalidated
	return s.opts.Documents.AppendVerification(ctx, q, &models.VerificationDecision{
		OrganizationID:      document.OrganizationID,
		DocumentID:          document.ID,
		Decision:            models.DecisionInvalidated,
		ReasonCode:          reason,
		MediaSourceVersion:  document.MediaSourceVersion,
		MediaChecksumSHA256: document.MediaChecksumSHA256,
		DecidedByActorType:  models.ActorSystem,
		DecidedByActorID:    "fleet-partner-service",
		DocumentVersion:     document.Version,
		CorrelationID:       correlationID,
	})
}

// transitionEffects describes the side effects of one state-changing
// operation, so the shared transition path does not need a branch per verb.
type transitionEffects struct {
	action      string
	auditAction string
	// decision, when set, appends an immutable verification record.
	decision   string
	reasonCode string
	reasonNote string
	eventType  string
	// invalidateReason, when set, supersedes eligibility decisions that rested
	// on this document.
	invalidateReason string
	metricReason     string
	extraPayload     map[string]any
}

// transition is the shared state-changing path.
//
// Every state-changing operation in this domain requires the same things:
// tenant authorization, owner validation, an expected version, history, audit,
// a transactional outbox event and an atomic commit. Implementing that once
// means a new verb cannot ship having forgotten one of them.
func (s *Service) transition(ctx context.Context, actor Actor, documentID string,
	expectedVersion int64, correlationID string, causationID string, requestID string,
	apply func(*models.Document) error, effects transitionEffects) (*models.Document, error) {
	if actor.OrganizationID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized, "Organization scope is required.")
	}

	var result *models.Document
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		document, err := s.opts.Documents.LockByID(ctx, tx, actor.OrganizationID, documentID)
		if errors.Is(err, repositories.ErrNotFound) {
			return errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
		}
		if err != nil {
			return err
		}
		// An expected version of zero means the caller did not supply one.
		// Requiring it on every call would break internal callers that legitimately
		// act on the current row; supplying a wrong one is always refused.
		if expectedVersion > 0 && document.Version != expectedVersion {
			return errorcodes.Conflict(errorcodes.ExpectedVersionMismatch,
				"This document changed since you loaded it. Reload and try again.")
		}

		fromState := document.LifecycleState
		if applyErr := apply(document); applyErr != nil {
			return applyErr
		}

		currentVersion := document.Version
		if updateErr := s.opts.Documents.Update(ctx, tx, document, currentVersion); updateErr != nil {
			if errors.Is(updateErr, repositories.ErrVersionMismatch) {
				return errorcodes.Conflict(errorcodes.ExpectedVersionMismatch,
					"This document changed since you loaded it. Reload and try again.")
			}
			return updateErr
		}

		if effects.decision != "" {
			if decisionErr := s.opts.Documents.AppendVerification(ctx, tx, &models.VerificationDecision{
				OrganizationID:      document.OrganizationID,
				DocumentID:          document.ID,
				Decision:            effects.decision,
				ReasonCode:          effects.reasonCode,
				ReasonNote:          effects.reasonNote,
				MediaSourceVersion:  document.MediaSourceVersion,
				MediaChecksumSHA256: document.MediaChecksumSHA256,
				DecidedByActorType:  actor.ActorType,
				DecidedByActorID:    actor.ActorID,
				DocumentVersion:     document.Version,
				CorrelationID:       correlationID,
				CausationID:         causationID,
			}); decisionErr != nil {
				return decisionErr
			}
		}

		metadata := map[string]any{}
		for key, value := range effects.extraPayload {
			metadata[key] = value
		}
		if effects.reasonCode != "" {
			metadata["reasonCode"] = effects.reasonCode
		}
		if histErr := s.opts.Documents.AppendHistory(ctx, tx, &models.HistoryEntry{
			OrganizationID: document.OrganizationID,
			DocumentID:     document.ID,
			Action:         effects.action,
			FromState:      fromState,
			ToState:        document.LifecycleState,
			ActorType:      actor.ActorType,
			ActorID:        actor.ActorID,
			Metadata:       metadata,
			CorrelationID:  correlationID,
			CausationID:    causationID,
		}); histErr != nil {
			return histErr
		}

		if auditErr := s.opts.Audit.Record(ctx, tx, audit.Entry{
			OrganizationID: document.OrganizationID,
			ActorType:      strings.ToLower(actor.ActorType),
			ActorID:        actor.ActorID,
			Action:         effects.auditAction,
			ResourceType:   "document",
			ResourceID:     document.ID,
			PreviousValues: map[string]any{"lifecycleState": fromState},
			NewValues: map[string]any{
				"lifecycleState":    document.LifecycleState,
				"verificationState": document.VerificationState,
			},
			Reason:        effects.reasonCode,
			RequestID:     requestID,
			CorrelationID: correlationID,
		}); auditErr != nil {
			return auditErr
		}

		if effects.invalidateReason != "" {
			if invalidateErr := s.invalidateDecisions(ctx, tx, document,
				effects.invalidateReason, correlationID); invalidateErr != nil {
				return invalidateErr
			}
		}

		if effects.eventType != "" {
			if publishErr := s.publish(ctx, tx, effects.eventType, document,
				correlationID, effects.extraPayload); publishErr != nil {
				return publishErr
			}
		}
		result = document
		return nil
	})
	if err != nil {
		return nil, err
	}

	if effects.metricReason != "" {
		s.opts.Metrics.IncCounter("fleet_partner_document_transitions_total",
			map[string]string{"action": effects.metricReason}, 1)
	}
	return result, nil
}

// markExpired is the expiry worker's transition. Kept here beside the
// verification rules because expiry is what invalidates a verification without
// anybody deciding anything.
func (s *Service) markExpired(ctx context.Context, tx pgx.Tx, document *models.Document,
	now time.Time) error {
	fromState := document.LifecycleState
	currentVersion := document.Version

	if err := s.InvalidateVerification(ctx, tx, document, models.RejectExpired, document.CorrelationID); err != nil {
		return err
	}
	document.LifecycleState = models.StateExpired

	if err := s.opts.Documents.Update(ctx, tx, document, currentVersion); err != nil {
		return err
	}
	if err := s.opts.Documents.AppendHistory(ctx, tx, &models.HistoryEntry{
		OrganizationID: document.OrganizationID,
		DocumentID:     document.ID,
		Action:         models.ActionExpired,
		FromState:      fromState,
		ToState:        document.LifecycleState,
		ActorType:      models.ActorSystem,
		ActorID:        "fleet-partner-service",
		Metadata:       map[string]any{"expiredAt": now.Format(time.RFC3339)},
	}); err != nil {
		return err
	}
	if err := s.opts.Audit.Record(ctx, tx, audit.Entry{
		OrganizationID: document.OrganizationID,
		ActorType:      "system",
		ActorID:        "fleet-partner-service",
		Action:         "document.expired",
		ResourceType:   "document",
		ResourceID:     document.ID,
		PreviousValues: map[string]any{"lifecycleState": fromState},
		NewValues:      map[string]any{"lifecycleState": document.LifecycleState},
	}); err != nil {
		return err
	}
	// An expired mandatory document must stop satisfying anything, which means
	// the decisions that relied on it can no longer be reused.
	if err := s.invalidateDecisions(ctx, tx, document, models.ReasonDocumentExpired, ""); err != nil {
		return err
	}
	return s.publish(ctx, tx, EventDocumentExpired, document, document.CorrelationID, nil)
}
