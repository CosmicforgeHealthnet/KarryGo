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
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Replacement, revocation, expiry and retention.
//
// The shape shared by all four: the old record is never edited into the new
// one. A replaced document stays exactly as it was and gains a pointer to its
// successor; a revoked one keeps everything it said and gains a reason and a
// timestamp. What changes is whether it may still authorize anything.

// ReplaceInput supersedes an existing document with a new one.
type ReplaceInput struct {
	Actor           Actor
	DocumentID      string
	ExpectedVersion int64
	// The replacement's own details. It is a new document in every sense: it
	// gets its own media object, and it must pass validation and verification
	// independently.
	Category         string
	DocumentType     string
	IssueDate        *time.Time
	ExpiryDate       *time.Time
	AllowedMIMETypes []string
	MaxSizeBytes     int64
	RetentionClass   string
	IdempotencyKey   string
	CorrelationID    string
	CausationID      string
	RequestID        string
}

// Replace creates a successor document and marks the original REPLACED.
//
// The replacement inherits nothing but its owner: not the verification, not
// the safety projection, not the media object. Inheriting a verification would
// mean a new file is trusted because an old one was, which is precisely the
// hole a replacement flow invites.
func (s *Service) Replace(ctx context.Context, input ReplaceInput) (*UploadTicket, error) {
	if err := s.writesAllowed(); err != nil {
		return nil, err
	}
	if input.Actor.OrganizationID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized, "Organization scope is required.")
	}

	original, err := s.Get(ctx, input.Actor.OrganizationID, input.DocumentID)
	if err != nil {
		return nil, err
	}
	if input.ExpectedVersion > 0 && original.Version != input.ExpectedVersion {
		return nil, errorcodes.Conflict(errorcodes.ExpectedVersionMismatch,
			"This document changed since you loaded it. Reload and try again.")
	}
	if original.ReplacementDocumentID != nil {
		return nil, errorcodes.Conflict(errorcodes.DocumentReplaced,
			"That document has already been replaced.")
	}
	if original.LifecycleState == models.StateDeleted {
		return nil, errorcodes.Conflict(errorcodes.DocumentDeleted,
			"A deleted document cannot be replaced.")
	}

	category := input.Category
	if category == "" {
		category = original.Category
	}
	documentType := input.DocumentType
	if documentType == "" {
		documentType = original.DocumentType
	}
	retention := input.RetentionClass
	if retention == "" {
		retention = original.RetentionClass
	}
	branchID := ""
	if original.BranchID != nil {
		branchID = *original.BranchID
	}

	ticket, err := s.RequestUpload(ctx, RequestUploadInput{
		Actor:              input.Actor,
		OwnerType:          original.OwnerType,
		OwnerID:            original.OwnerID,
		BranchID:           branchID,
		Category:           category,
		DocumentType:       documentType,
		IssueDate:          input.IssueDate,
		ExpiryDate:         input.ExpiryDate,
		AllowedMIMETypes:   input.AllowedMIMETypes,
		MaxSizeBytes:       input.MaxSizeBytes,
		RetentionClass:     retention,
		ReplacesDocumentID: original.ID,
		IdempotencyKey:     input.IdempotencyKey,
		CorrelationID:      input.CorrelationID,
		CausationID:        input.CausationID,
		RequestID:          input.RequestID,
	})
	if err != nil {
		return nil, err
	}
	if ticket.Replayed {
		return ticket, nil
	}

	replacementID := ticket.Document.ID
	_, err = s.transition(ctx, input.Actor, original.ID, 0,
		input.CorrelationID, input.CausationID, input.RequestID,
		func(document *models.Document) error {
			document.ReplacementDocumentID = &replacementID
			document.LifecycleState = models.StateReplaced
			// The superseded document's verification no longer stands for the
			// requirement it satisfied — the replacement has to earn that.
			if document.VerificationState == models.VerificationVerified {
				document.VerificationState = models.VerificationInvalidated
			}
			return nil
		},
		transitionEffects{
			action:           models.ActionReplaced,
			auditAction:      "document.replaced",
			eventType:        EventDocumentReplaced,
			metricReason:     "replaced",
			invalidateReason: models.ReasonDocumentReplaced,
			extraPayload:     map[string]any{"replacementDocumentId": replacementID},
		})
	if err != nil {
		return nil, err
	}
	return ticket, nil
}

// RevokeInput withdraws a document.
type RevokeInput struct {
	Actor           Actor
	DocumentID      string
	ExpectedVersion int64
	Reason          string
	CorrelationID   string
	CausationID     string
	RequestID       string
}

// Revoke withdraws a document immediately.
//
// History is preserved: the document keeps everything it said and gains a
// reason and a timestamp. What it loses is the ability to satisfy an
// eligibility requirement or to be released for download, and both of those
// take effect at once rather than at the next sweep.
func (s *Service) Revoke(ctx context.Context, input RevokeInput) (*models.Document, error) {
	if err := s.writesAllowed(); err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		// A revocation with no reason is an unanswerable question three months
		// later, so it is refused rather than stored empty.
		return nil, errorcodes.BadRequest(errorcodes.DocumentInvalid,
			"A revocation reason is required.")
	}
	if len(reason) > 500 {
		return nil, errorcodes.BadRequest(errorcodes.DocumentInvalid, "The revocation reason is too long.")
	}
	now := s.opts.Now().UTC()

	return s.transition(ctx, input.Actor, input.DocumentID, input.ExpectedVersion,
		input.CorrelationID, input.CausationID, input.RequestID,
		func(document *models.Document) error {
			if document.LifecycleState == models.StateRevoked {
				return errorcodes.Conflict(errorcodes.DocumentRevoked,
					"This document has already been revoked.")
			}
			if document.LifecycleState == models.StateDeleted {
				return errorcodes.Conflict(errorcodes.DocumentDeleted,
					"A deleted document cannot be revoked.")
			}
			document.LifecycleState = models.StateRevoked
			document.RevokedAt = &now
			document.RevokedReason = reason
			if document.VerificationState == models.VerificationVerified {
				document.VerificationState = models.VerificationInvalidated
			}
			return nil
		},
		transitionEffects{
			action:           models.ActionRevoked,
			auditAction:      "document.revoked",
			reasonCode:       reason,
			eventType:        EventDocumentRevoked,
			metricReason:     "revoked",
			invalidateReason: models.ReasonDocumentRevoked,
		})
}

// ArchiveInput archives a document.
type ArchiveInput struct {
	Actor           Actor
	DocumentID      string
	ExpectedVersion int64
	CorrelationID   string
	CausationID     string
	RequestID       string
}

// Archive marks a document archived.
//
// Archival is orthogonal to the lifecycle state machine: it does not revoke,
// delete or otherwise change what the document says, it only takes it out of
// default listings.
func (s *Service) Archive(ctx context.Context, input ArchiveInput) (*models.Document, error) {
	if err := s.writesAllowed(); err != nil {
		return nil, err
	}
	now := s.opts.Now().UTC()

	return s.transition(ctx, input.Actor, input.DocumentID, input.ExpectedVersion,
		input.CorrelationID, input.CausationID, input.RequestID,
		func(document *models.Document) error {
			if document.Archived() {
				return errorcodes.Conflict(errorcodes.DocumentConflict,
					"This document has already been archived.")
			}
			document.ArchivedAt = &now
			document.ArchivedByActorID = input.Actor.ActorID
			return nil
		},
		transitionEffects{
			action:       models.ActionArchived,
			auditAction:  "document.archived",
			eventType:    EventDocumentArchived,
			metricReason: "archived",
		})
}

// RetentionInput updates retention policy and legal hold.
type RetentionInput struct {
	Actor           Actor
	DocumentID      string
	ExpectedVersion int64
	RetentionClass  string
	RetentionUntil  *time.Time
	LegalHold       *bool
	Reason          string
	CorrelationID   string
	CausationID     string
	RequestID       string
}

// SetRetention updates fleet's retention policy and propagates it to media.
//
// Fleet's copy governs fleet's deletion scheduling; media's copy is what
// actually refuses to destroy the bytes. Both are set, because a legal hold
// recorded only on the fleet side would be one direct media call away from
// being ignored.
func (s *Service) SetRetention(ctx context.Context, input RetentionInput) (*models.Document, error) {
	if err := s.writesAllowed(); err != nil {
		return nil, err
	}

	retentionClass := strings.ToUpper(strings.TrimSpace(input.RetentionClass))
	switch retentionClass {
	case "", models.RetentionStandard, models.RetentionExtended, models.RetentionLegal:
	default:
		return nil, errorcodes.BadRequest(errorcodes.DocumentInvalid, "retentionClass is not supported.")
	}
	if input.LegalHold != nil && *input.LegalHold && strings.TrimSpace(input.Reason) == "" {
		return nil, errorcodes.BadRequest(errorcodes.DocumentInvalid,
			"A legal hold requires a reason.")
	}

	action := models.ActionRetentionUpdated
	if input.LegalHold != nil {
		if *input.LegalHold {
			action = models.ActionLegalHoldApplied
		} else {
			action = models.ActionLegalHoldReleased
		}
	}

	document, err := s.transition(ctx, input.Actor, input.DocumentID, input.ExpectedVersion,
		input.CorrelationID, input.CausationID, input.RequestID,
		func(document *models.Document) error {
			if retentionClass != "" {
				document.RetentionClass = retentionClass
			}
			if input.RetentionUntil != nil {
				until := input.RetentionUntil.UTC()
				document.RetentionUntil = &until
			}
			if input.LegalHold != nil {
				document.LegalHold = *input.LegalHold
				if *input.LegalHold {
					document.LegalHoldReason = input.Reason
				} else {
					document.LegalHoldReason = ""
				}
			}
			return nil
		},
		transitionEffects{
			action:       action,
			auditAction:  "document.retention.updated",
			eventType:    EventDocumentStateChanged,
			metricReason: "retention",
		})
	if err != nil {
		return nil, err
	}

	// Propagate to the only service that can actually refuse to destroy bytes.
	// A failure here is reported: a legal hold the operator believes is in
	// force but that media never received is worse than a visible error.
	if s.opts.Media != nil && s.opts.Media.Configured() {
		if _, mediaErr := s.opts.Media.SetRetention(ctx, platformclients.CallMeta{
			OrganizationID: input.Actor.OrganizationID,
			RequestID:      input.RequestID,
			CorrelationID:  input.CorrelationID,
			CausationID:    input.CausationID,
		}, document.MediaObjectID, document.RetentionClass, document.RetentionUntil,
			input.LegalHold); mediaErr != nil {
			return nil, s.mediaError(mediaErr)
		}
	}
	return document, nil
}

// RunExpiryPass marks documents whose stated expiry has passed.
//
// Recoverable by construction: it claims a bounded batch with SKIP LOCKED, and
// each document is expired in its own transaction, so a crash halfway through
// leaves the ones already committed expired and the rest to be picked up next
// pass.
func (s *Service) RunExpiryPass(ctx context.Context, batch int) (int, error) {
	if s.opts.Flags.KillSwitch {
		return 0, nil
	}
	now := s.opts.Now().UTC()

	var documents []models.Document
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		claimed, claimErr := s.opts.Documents.ClaimExpiredDocuments(ctx, tx, batch)
		if claimErr != nil {
			return claimErr
		}
		documents = claimed
		for index := range documents {
			if expireErr := s.markExpired(ctx, tx, &documents[index], now); expireErr != nil {
				return expireErr
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if len(documents) > 0 {
		s.opts.Metrics.IncCounter("fleet_partner_documents_expired_total", nil, float64(len(documents)))
	}
	return len(documents), nil
}

// RunExpiryWorker sweeps expiries until ctx ends.
//
// Returns when ctx is cancelled, which is how the composition root stops it;
// an in-flight pass finishes rather than being killed mid-transaction.
func (s *Service) RunExpiryWorker(ctx context.Context, interval time.Duration, batch int) error {
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := s.RunExpiryPass(ctx, batch); err != nil {
			s.opts.Metrics.IncCounter("fleet_partner_document_worker_failures_total",
				map[string]string{"worker": "expiry"}, 1)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// ExpireDocument marks one document expired, for callers that already know it
// lapsed. Exposed for tests and operator tooling.
func (s *Service) ExpireDocument(ctx context.Context, organizationID string, documentID string) error {
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		document, err := s.opts.Documents.LockByID(ctx, tx, organizationID, documentID)
		if errors.Is(err, repositories.ErrNotFound) {
			return errorcodes.NotFound(errorcodes.DocumentNotFound, "Document not found.")
		}
		if err != nil {
			return err
		}
		return s.markExpired(ctx, tx, document, s.opts.Now().UTC())
	})
}
