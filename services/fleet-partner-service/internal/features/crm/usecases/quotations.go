package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// LineItemInput is one manually priced quotation line.
//
// Every number here was typed in by somebody accountable for it. There is no
// rate card, no distance or weight formula, no marketplace fare, no surge, no
// commission and no tax rate anywhere in this flow: an operator's commercial
// terms are theirs, and a hidden calculation would silently change what a
// customer was quoted.
type LineItemInput struct {
	Description              string
	Quantity                 string
	UnitType                 string
	UnitAmountMinor          int64
	LineAmountMinor          *int64
	ExternalPricingReference string
	ManualReason             string
}

// CreateQuotationInput opens a draft quotation.
type CreateQuotationInput struct {
	BranchID       string
	CustomerID     string
	OpportunityID  string
	Currency       string
	ValidFrom      time.Time
	ValidUntil     time.Time
	ServiceScope   models.JSONMap
	Terms          models.JSONMap
	InternalNotes  string
	LineItems      []LineItemInput
	IdempotencyKey string
	CorrelationID  string
	CausationID    string
}

// QuotationDetail is a quotation with its current lines.
type QuotationDetail struct {
	Quotation models.Quotation
	LineItems []models.LineItem
}

// CreateQuotation opens a draft quotation.
func (s *Service) CreateQuotation(ctx context.Context, t tenant.Context,
	input CreateQuotationInput) (QuotationDetail, error) {
	if err := s.writeEnabled(s.opts.Flags.QuotationsEnabled); err != nil {
		return QuotationDetail{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return QuotationDetail{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if err := models.ValidateCurrency(currency); err != nil {
		return QuotationDetail{}, badRequest(errorcodes.QuotationInvalidState,
			"currency must be a three-letter ISO 4217 code.")
	}
	if !input.ValidUntil.After(input.ValidFrom) {
		return QuotationDetail{}, badRequest(errorcodes.QuotationInvalidState,
			"validUntil must be after validFrom.")
	}
	if input.BranchID != "" && !t.BranchPermitted(input.BranchID) {
		return QuotationDetail{}, badRequest(errorcodes.QuotationInvalidState,
			"branchId is not within your authorized scope.")
	}

	var detail QuotationDetail
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		customer, txErr := s.requireCustomer(ctx, tx, t, input.CustomerID)
		if txErr != nil {
			return txErr
		}
		if customer.IsArchived() {
			return conflict(errorcodes.CustomerArchived,
				"An archived customer cannot receive new quotations.")
		}
		if input.OpportunityID != "" {
			opportunity, oppErr := s.opts.Repository.GetOpportunity(ctx, tx, t.OrganizationID,
				input.OpportunityID)
			if oppErr != nil || opportunity.CustomerID != input.CustomerID {
				// Either it does not exist, belongs to another tenant, or
				// belongs to a different customer. All three are the same
				// answer: the caller may not link them.
				return errorcodes.NotFound(errorcodes.OpportunityNotFound,
					"Opportunity was not found.")
			}
		}

		number, txErr := s.opts.Repository.NextQuotationNumber(ctx, tx, t.OrganizationID,
			s.now())
		if txErr != nil {
			return txErr
		}
		quotation := models.Quotation{
			ID:              ids.New(repositories.PrefixQuotation),
			OrganizationID:  t.OrganizationID,
			BranchID:        input.BranchID,
			CustomerID:      input.CustomerID,
			OpportunityID:   input.OpportunityID,
			QuotationNumber: number,
			Currency:        currency,
			ValidFrom:       input.ValidFrom,
			ValidUntil:      input.ValidUntil,
			ServiceScope:    input.ServiceScope,
			Terms:           input.Terms,
			InternalNotes:   input.InternalNotes,
			Status:          models.QuotationDraft,
			CreatedBy:       t.OrganizationUserID,
		}
		created, txErr := s.opts.Repository.CreateQuotation(ctx, tx, quotation)
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationVersionConflict)
		}

		items, txErr := s.persistLineItems(ctx, tx, t.OrganizationID, created.ID, currency,
			input.LineItems)
		if txErr != nil {
			return txErr
		}
		total, txErr := models.QuotationTotal(currency, items)
		if txErr != nil {
			return badRequest(errorcodes.QuotationInvalidState, txErr.Error())
		}
		if total > 0 {
			created, txErr = s.opts.Repository.TransitionQuotation(ctx, tx, t.OrganizationID,
				created.ID, created.Version, repositories.QuotationTransition{
					FromStatus: models.QuotationDraft, ToStatus: models.QuotationDraft,
					TotalAmountMinor: &total, Actor: t.OrganizationUserID,
				})
			if txErr != nil {
				return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
					errorcodes.QuotationVersionConflict)
			}
		}

		detail = QuotationDetail{Quotation: created, LineItems: items}
		if txErr = s.opts.Repository.AppendQuotationHistory(ctx, tx, models.QuotationHistory{
			OrganizationID: t.OrganizationID, QuotationID: created.ID,
			PreviousStatus: "", NewStatus: models.QuotationDraft,
			QuotationVersion: created.CurrentVersion, ActorType: string(t.ActorType),
			ActorID: t.ActorID, CorrelationID: input.CorrelationID,
			CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.quotation.created", "quotation", created.ID,
			nil, quotationAuditValues(created), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventQuotationCreated, "quotation", created.ID,
			created.Version, t.OrganizationID, input.CorrelationID,
			quotationEventPayload(created))
	})
	if err != nil {
		return QuotationDetail{}, err
	}
	s.count("quotations_created_total", nil)
	return detail, nil
}

// persistLineItems validates and stores a draft quotation's lines.
//
// A caller may supply the line amount or leave it out. Where it is supplied it
// must equal the exact decimal product of unit amount and quantity — a mismatch
// is refused rather than quietly corrected, because a quotation whose lines do
// not add up is a dispute waiting to happen and the caller needs to know which
// number they got wrong.
func (s *Service) persistLineItems(ctx context.Context, q database.Querier,
	organizationID, quotationID, currency string,
	inputs []LineItemInput) ([]models.LineItem, error) {
	items := make([]models.LineItem, 0, len(inputs))
	for index, input := range inputs {
		if strings.TrimSpace(input.Description) == "" {
			return nil, badRequest(errorcodes.QuotationInvalidState,
				fmt.Sprintf("lineItems[%d].description is required.", index))
		}
		quantity, err := models.NormalizeQuantity(input.Quantity)
		if err != nil {
			return nil, badRequest(errorcodes.QuotationInvalidState,
				fmt.Sprintf("lineItems[%d].quantity must be a positive decimal.", index))
		}
		if input.UnitAmountMinor < 0 {
			return nil, badRequest(errorcodes.QuotationInvalidState,
				fmt.Sprintf("lineItems[%d].unitAmountMinor must not be negative.", index))
		}
		computed, err := models.LineAmountFor(input.UnitAmountMinor, quantity)
		if err != nil {
			return nil, badRequest(errorcodes.QuotationInvalidState,
				fmt.Sprintf("lineItems[%d] amount could not be computed.", index))
		}
		if input.LineAmountMinor != nil && *input.LineAmountMinor != computed {
			return nil, badRequest(errorcodes.QuotationInvalidState,
				fmt.Sprintf("lineItems[%d].lineAmountMinor does not match unitAmountMinor × quantity.",
					index))
		}
		items = append(items, models.LineItem{
			OrganizationID:           organizationID,
			QuotationID:              quotationID,
			Description:              strings.TrimSpace(input.Description),
			Quantity:                 quantity,
			UnitType:                 strings.TrimSpace(input.UnitType),
			UnitAmountMinor:          input.UnitAmountMinor,
			LineAmountMinor:          computed,
			Currency:                 currency,
			ExternalPricingReference: input.ExternalPricingReference,
			ManualReason:             input.ManualReason,
			Sequence:                 index,
		})
	}
	stored, err := s.opts.Repository.ReplaceLineItems(ctx, q, organizationID, quotationID, items)
	if err != nil {
		return nil, translate(err, "Quotation", errorcodes.QuotationNotFound,
			errorcodes.QuotationVersionConflict)
	}
	return stored, nil
}

// GetQuotation reads one quotation with its current lines.
func (s *Service) GetQuotation(ctx context.Context, t tenant.Context,
	quotationID string) (QuotationDetail, error) {
	if err := s.readEnabled(); err != nil {
		return QuotationDetail{}, err
	}
	quotation, err := s.requireQuotation(ctx, nil, t, quotationID)
	if err != nil {
		return QuotationDetail{}, err
	}
	items, err := s.opts.Repository.ListLineItems(ctx, nil, t.OrganizationID, quotationID)
	if err != nil {
		return QuotationDetail{}, err
	}
	return QuotationDetail{Quotation: quotation, LineItems: items}, nil
}

func (s *Service) requireQuotation(ctx context.Context, q database.Querier, t tenant.Context,
	quotationID string) (models.Quotation, error) {
	quotation, err := s.opts.Repository.GetQuotation(ctx, q, t.OrganizationID, quotationID)
	if err != nil {
		return quotation, translate(err, "Quotation", errorcodes.QuotationNotFound,
			errorcodes.QuotationVersionConflict)
	}
	if !t.BranchPermitted(quotation.BranchID) {
		return quotation, notFound("Quotation")
	}
	return quotation, nil
}

// ListQuotations returns a tenant-scoped page.
func (s *Service) ListQuotations(ctx context.Context, t tenant.Context,
	filter repositories.QuotationFilter) ([]models.Quotation, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListQuotations(ctx, t.OrganizationID, t.PermittedBranchIDs, filter)
}

// UpdateQuotationInput carries a partial draft quotation update.
type UpdateQuotationInput struct {
	ValidFrom        *time.Time
	ValidUntil       *time.Time
	ServiceScope     models.JSONMap
	Terms            models.JSONMap
	InternalNotes    *string
	OpportunityID    *string
	LineItems        []LineItemInput
	ReplaceLineItems bool
	ExpectedVersion  int64
	CorrelationID    string
	CausationID      string
}

// UpdateQuotation edits a draft quotation.
//
// Only a DRAFT is editable, and the repository enforces that in SQL as well.
// An issued quotation is a promise already made; correcting it produces a new
// version or a superseding quotation, never a rewrite of what the customer was
// shown.
func (s *Service) UpdateQuotation(ctx context.Context, t tenant.Context, quotationID string,
	input UpdateQuotationInput) (QuotationDetail, error) {
	if err := s.writeEnabled(s.opts.Flags.QuotationsEnabled); err != nil {
		return QuotationDetail{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return QuotationDetail{}, err
	}

	var detail QuotationDetail
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.opts.Repository.LockQuotation(ctx, tx, t.OrganizationID,
			quotationID)
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationVersionConflict)
		}
		if !t.BranchPermitted(existing.BranchID) {
			return notFound("Quotation")
		}
		if existing.Status != models.QuotationDraft {
			return conflict(errorcodes.QuotationInvalidState,
				"Only a draft quotation can be edited.")
		}
		validFrom, validUntil := existing.ValidFrom, existing.ValidUntil
		if input.ValidFrom != nil {
			validFrom = *input.ValidFrom
		}
		if input.ValidUntil != nil {
			validUntil = *input.ValidUntil
		}
		if !validUntil.After(validFrom) {
			return badRequest(errorcodes.QuotationInvalidState,
				"validUntil must be after validFrom.")
		}
		if input.OpportunityID != nil && *input.OpportunityID != "" {
			opportunity, oppErr := s.opts.Repository.GetOpportunity(ctx, tx, t.OrganizationID,
				*input.OpportunityID)
			if oppErr != nil || opportunity.CustomerID != existing.CustomerID {
				return errorcodes.NotFound(errorcodes.OpportunityNotFound,
					"Opportunity was not found.")
			}
		}

		updated, txErr := s.opts.Repository.UpdateQuotation(ctx, tx, t.OrganizationID,
			quotationID, input.ExpectedVersion, repositories.QuotationUpdate{
				ValidFrom: input.ValidFrom, ValidUntil: input.ValidUntil,
				ServiceScope: input.ServiceScope, Terms: input.Terms,
				InternalNotes: input.InternalNotes, OpportunityID: input.OpportunityID,
				UpdatedBy: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationVersionConflict)
		}

		items, txErr := s.opts.Repository.ListLineItems(ctx, tx, t.OrganizationID, quotationID)
		if txErr != nil {
			return txErr
		}
		if input.ReplaceLineItems {
			items, txErr = s.persistLineItems(ctx, tx, t.OrganizationID, quotationID,
				updated.Currency, input.LineItems)
			if txErr != nil {
				return txErr
			}
			total, totalErr := models.QuotationTotal(updated.Currency, items)
			if totalErr != nil {
				return badRequest(errorcodes.QuotationInvalidState, totalErr.Error())
			}
			updated, txErr = s.opts.Repository.TransitionQuotation(ctx, tx, t.OrganizationID,
				quotationID, updated.Version, repositories.QuotationTransition{
					FromStatus: models.QuotationDraft, ToStatus: models.QuotationDraft,
					TotalAmountMinor: &total, Actor: t.OrganizationUserID,
				})
			if txErr != nil {
				return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
					errorcodes.QuotationVersionConflict)
			}
		}

		detail = QuotationDetail{Quotation: updated, LineItems: items}
		if txErr = s.record(ctx, tx, t, "crm.quotation.updated", "quotation", quotationID,
			quotationAuditValues(existing), quotationAuditValues(updated), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventQuotationCreated, "quotation", quotationID,
			updated.Version, t.OrganizationID, input.CorrelationID,
			quotationEventPayload(updated))
	})
	if err != nil {
		return QuotationDetail{}, err
	}
	return detail, nil
}

// SubmitQuotationInput submits a draft quotation for approval.
type SubmitQuotationInput struct {
	ExpectedVersion int64
	IdempotencyKey  string
	CorrelationID   string
	CausationID     string
}

// SubmitQuotation validates a draft and routes it through the WP9 approval
// policy.
//
// The approval is bound to the quotation ID, the quotation's row version, the
// content fingerprint and the approval framework's own policy version. That
// binding is what makes an approval an approval of a specific offer rather than
// of a quotation number: any later edit changes the fingerprint, and the
// framework invalidates the request rather than letting a reviewed cheap
// version become an unreviewed expensive one.
func (s *Service) SubmitQuotation(ctx context.Context, t tenant.Context, quotationID string,
	input SubmitQuotationInput) (QuotationDetail, error) {
	if err := s.writeEnabled(s.opts.Flags.QuotationsEnabled); err != nil {
		return QuotationDetail{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return QuotationDetail{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return QuotationDetail{}, err
	}

	var detail QuotationDetail
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		quotation, txErr := s.opts.Repository.LockQuotation(ctx, tx, t.OrganizationID,
			quotationID)
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationVersionConflict)
		}
		if !t.BranchPermitted(quotation.BranchID) {
			return notFound("Quotation")
		}
		if quotation.Status == models.QuotationAwaitingApproval ||
			quotation.Status == models.QuotationApproved {
			// Already submitted. A repeat is a replay, not a second submission.
			items, listErr := s.opts.Repository.ListLineItems(ctx, tx, t.OrganizationID,
				quotationID)
			if listErr != nil {
				return listErr
			}
			detail = QuotationDetail{Quotation: quotation, LineItems: items}
			return nil
		}
		if quotation.Status != models.QuotationDraft {
			return conflict(errorcodes.QuotationInvalidState,
				"Only a draft quotation can be submitted for approval.")
		}

		customer, txErr := s.requireCustomer(ctx, tx, t, quotation.CustomerID)
		if txErr != nil {
			return txErr
		}
		if txErr = requireCommittableCustomer(customer); txErr != nil {
			return txErr
		}
		if txErr = s.requireNoBlockingCreditHold(ctx, tx, t.OrganizationID,
			quotation.CustomerID); txErr != nil {
			return txErr
		}

		items, txErr := s.opts.Repository.ListLineItems(ctx, tx, t.OrganizationID, quotationID)
		if txErr != nil {
			return txErr
		}
		if len(items) == 0 {
			return badRequest(errorcodes.QuotationInvalidState,
				"A quotation must have at least one line item before it is submitted.")
		}
		total, txErr := models.QuotationTotal(quotation.Currency, items)
		if txErr != nil {
			return badRequest(errorcodes.QuotationInvalidState, txErr.Error())
		}
		if !quotation.ValidUntil.After(s.now()) {
			return conflict(errorcodes.QuotationExpired,
				"The quotation's validity period has already ended.")
		}

		contentFingerprint := quotationFingerprint(quotation, items, total)
		nextStatus := models.QuotationApproved
		approvalID := ""
		if s.opts.Approvals != nil {
			approvalID, txErr = s.opts.Approvals.Request(ctx, ApprovalRequestInput{
				OrganizationID: t.OrganizationID, OrganizationUserID: t.OrganizationUserID,
				BranchID: quotation.BranchID, ResourceType: "quotation",
				ResourceID: quotationID, Action: "issue",
				OperationType: "CRM_QUOTATION_ISSUE",
				Payload: map[string]any{
					"quotationId":        quotationID,
					"quotationNumber":    quotation.QuotationNumber,
					"customerId":         quotation.CustomerID,
					"currency":           quotation.Currency,
					"totalAmountMinor":   total,
					"lineItemCount":      len(items),
					"contentFingerprint": contentFingerprint,
				},
				ExpectedSubjectVersion: quotation.Version,
				IdempotencyKey:         input.IdempotencyKey,
				CorrelationID:          input.CorrelationID, CausationID: input.CausationID,
			})
			if txErr != nil {
				return txErr
			}
			if approvalID != "" {
				// The framework raised a request, so a human still has to say
				// yes. Only when it declines to raise one — because policy does
				// not require approval at this value — does the quotation go
				// straight to APPROVED.
				nextStatus = models.QuotationAwaitingApproval
			}
		}

		updated, txErr := s.opts.Repository.TransitionQuotation(ctx, tx, t.OrganizationID,
			quotationID, input.ExpectedVersion, repositories.QuotationTransition{
				FromStatus: models.QuotationDraft, ToStatus: nextStatus,
				ApprovalRequestID: approvalID, TotalAmountMinor: &total,
				Actor: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationVersionConflict)
		}
		detail = QuotationDetail{Quotation: updated, LineItems: items}

		if txErr = s.opts.Repository.AppendQuotationHistory(ctx, tx, models.QuotationHistory{
			OrganizationID: t.OrganizationID, QuotationID: quotationID,
			PreviousStatus: quotation.Status, NewStatus: updated.Status,
			QuotationVersion: updated.CurrentVersion, ApprovalRequestID: approvalID,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.quotation.submitted", "quotation", quotationID,
			map[string]any{"status": quotation.Status},
			map[string]any{"status": updated.Status, "approvalRequestId": approvalID},
			""); txErr != nil {
			return txErr
		}
		eventType := EventQuotationSubmitted
		if updated.Status == models.QuotationApproved {
			eventType = EventQuotationApproved
		}
		return s.publish(ctx, tx, eventType, "quotation", quotationID, updated.Version,
			t.OrganizationID, input.CorrelationID,
			quotationStatusEventPayload(updated, quotation.Status, ""))
	})
	if err != nil {
		return QuotationDetail{}, err
	}
	s.count("quotations_submitted_total", nil)
	return detail, nil
}

// quotationFingerprint hashes everything that defines the offer.
//
// The line items are folded in field by field and in stable sequence order, so
// reordering, repricing or adding a line all produce a different fingerprint
// and therefore invalidate an approval bound to the old one.
func quotationFingerprint(quotation models.Quotation, items []models.LineItem,
	total int64) string {
	parts := []string{
		quotation.ID, quotation.CustomerID, quotation.Currency,
		quotation.ValidFrom.UTC().Format(time.RFC3339Nano),
		quotation.ValidUntil.UTC().Format(time.RFC3339Nano),
		fmt.Sprintf("%d", total),
	}
	if scope, err := models.Fingerprint(quotation.ServiceScope); err == nil {
		parts = append(parts, scope)
	}
	if terms, err := models.Fingerprint(quotation.Terms); err == nil {
		parts = append(parts, terms)
	}
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%d|%s|%s|%s|%d|%d", item.Sequence, item.Description,
			item.Quantity, item.UnitType, item.UnitAmountMinor, item.LineAmountMinor))
	}
	return fingerprint(parts...)
}

// QuotationVersions returns every immutable version of a quotation.
func (s *Service) QuotationVersions(ctx context.Context, t tenant.Context, quotationID string,
	limit int) ([]models.QuotationVersion, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.requireQuotation(ctx, nil, t, quotationID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListQuotationVersions(ctx, t.OrganizationID, quotationID, limit)
}

// QuotationVersion returns one immutable version.
func (s *Service) QuotationVersion(ctx context.Context, t tenant.Context,
	quotationID, versionID string) (models.QuotationVersion, error) {
	if err := s.readEnabled(); err != nil {
		return models.QuotationVersion{}, err
	}
	if _, err := s.requireQuotation(ctx, nil, t, quotationID); err != nil {
		return models.QuotationVersion{}, err
	}
	version, err := s.opts.Repository.GetQuotationVersion(ctx, nil, t.OrganizationID,
		quotationID, versionID)
	return version, translate(err, "Quotation version", errorcodes.QuotationNotFound,
		errorcodes.QuotationVersionConflict)
}

// QuotationHistory returns a page of a quotation's status changes.
func (s *Service) QuotationHistory(ctx context.Context, t tenant.Context, quotationID string,
	page cursor.Cursor, hasCursor bool, limit int) ([]models.QuotationHistory, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.requireQuotation(ctx, nil, t, quotationID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListQuotationHistory(ctx, t.OrganizationID, quotationID, page,
		hasCursor, limit)
}

// AttachQuotationDocumentInput links a Media object to a quotation.
type AttachQuotationDocumentInput struct {
	MediaObjectID   string
	DocumentPurpose string
	IsMandatory     bool
	CorrelationID   string
}

// AttachQuotationDocument records a Media reference against a quotation.
//
// Fleet asks Media for the object's state and stores a projection of it. It
// stores no bytes, no URL, no storage path, no checksum and no MIME type: those
// belong to Media, and a second copy here would be a second answer that can go
// stale exactly when it matters.
func (s *Service) AttachQuotationDocument(ctx context.Context, t tenant.Context,
	quotationID string, input AttachQuotationDocumentInput) (models.DocumentReference, error) {
	if err := s.writeEnabled(s.opts.Flags.QuotationsEnabled); err != nil {
		return models.DocumentReference{}, err
	}
	if strings.TrimSpace(input.MediaObjectID) == "" {
		return models.DocumentReference{}, badRequest(errorcodes.QuotationDocumentNotReady,
			"mediaObjectId is required.")
	}
	switch input.DocumentPurpose {
	case models.DocPurposeQuotation, models.DocPurposeSupporting,
		models.DocPurposeAcceptanceEvidence, models.DocPurposeRejectionEvidence:
	default:
		return models.DocumentReference{}, badRequest(errorcodes.QuotationDocumentNotReady,
			"documentPurpose is not supported for a quotation.")
	}

	quotation, err := s.requireQuotation(ctx, nil, t, quotationID)
	if err != nil {
		return models.DocumentReference{}, err
	}
	if models.TerminalQuotation(quotation.Status) {
		return models.DocumentReference{}, conflict(errorcodes.QuotationAlreadyResolved,
			"A resolved quotation cannot receive new documents.")
	}

	// The Media read happens outside the transaction: an external call inside an
	// open transaction holds a database connection for the duration of somebody
	// else's outage.
	statusProjection, readiness, sourceVersion := s.readMediaState(ctx, t, input.MediaObjectID)

	var attached models.DocumentReference
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		var txErr error
		attached, txErr = s.opts.Repository.AttachQuotationDocument(ctx, tx,
			models.DocumentReference{
				ID:               ids.New(repositories.PrefixQuotationDocument),
				OrganizationID:   t.OrganizationID,
				SubjectID:        quotationID,
				MediaObjectID:    input.MediaObjectID,
				DocumentPurpose:  input.DocumentPurpose,
				IsMandatory:      input.IsMandatory,
				StatusProjection: statusProjection,
				ReadinessState:   readiness,
				SourceVersion:    sourceVersion,
				CreatedBy:        t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Quotation document", errorcodes.QuotationDocumentNotReady,
				errorcodes.QuotationDocumentNotReady)
		}
		return s.record(ctx, tx, t, "crm.quotation.document_attached", "quotation",
			quotationID, nil, map[string]any{
				"documentPurpose":  input.DocumentPurpose,
				"statusProjection": statusProjection,
				"readinessState":   readiness,
			}, "")
	})
	if err != nil {
		return models.DocumentReference{}, err
	}
	return attached, nil
}

// QuotationDocuments returns the document references for a quotation.
func (s *Service) QuotationDocuments(ctx context.Context, t tenant.Context,
	quotationID string) ([]models.DocumentReference, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.requireQuotation(ctx, nil, t, quotationID); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListQuotationDocuments(ctx, nil, t.OrganizationID, quotationID)
}

// refreshQuotationDocuments re-reads every mandatory document from Media.
//
// It runs immediately before issue, outside the transaction. A projection
// written an hour ago is a memory of what Media said, and a file revoked since
// then must not be able to back a promise to a customer.
func (s *Service) refreshQuotationDocuments(ctx context.Context, t tenant.Context,
	quotationID string) error {
	documents, err := s.opts.Repository.ListQuotationDocuments(ctx, nil, t.OrganizationID,
		quotationID)
	if err != nil {
		return err
	}
	for _, document := range documents {
		if !document.IsMandatory {
			continue
		}
		statusProjection, readiness, sourceVersion := s.readMediaState(ctx, t,
			document.MediaObjectID)
		if _, err := s.opts.Repository.AttachQuotationDocument(ctx, nil,
			models.DocumentReference{
				ID: document.ID, OrganizationID: t.OrganizationID, SubjectID: quotationID,
				MediaObjectID:   document.MediaObjectID,
				DocumentPurpose: document.DocumentPurpose, IsMandatory: document.IsMandatory,
				StatusProjection: statusProjection, ReadinessState: readiness,
				SourceVersion: sourceVersion, CreatedBy: document.CreatedBy,
			}); err != nil {
			return err
		}
	}
	return nil
}

// quotationAuditValues builds the audit projection.
//
// No internal notes and no line items: the audit trail records that a quotation
// moved and what it was worth, not the commercial detail of the offer.
func quotationAuditValues(quotation models.Quotation) map[string]any {
	return map[string]any{
		"quotationNumber":  quotation.QuotationNumber,
		"customerId":       quotation.CustomerID,
		"status":           quotation.Status,
		"currency":         quotation.Currency,
		"totalAmountMinor": quotation.TotalAmountMinor,
		"currentVersion":   quotation.CurrentVersion,
		"validUntil":       quotation.ValidUntil.UTC().Format(time.RFC3339Nano),
	}
}

var errQuotationNotApproved = errors.New("crm: quotation is not approved")
