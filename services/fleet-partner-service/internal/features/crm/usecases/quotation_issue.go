package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// IssueQuotationInput issues an approved quotation to a customer.
type IssueQuotationInput struct {
	// MintCustomerToken asks for a secure customer link. An operator delivering
	// the quotation by hand can decline one; a customer-facing issue needs it.
	MintCustomerToken bool
	SingleUseToken    bool
	ExpectedVersion   int64
	IdempotencyKey    string
	CorrelationID     string
	CausationID       string
}

// IssueQuotationResult is what issue produces.
//
// CustomerToken is the raw secure token, present exactly once, in this
// response. It is never persisted, never logged and never returned again: what
// the database holds is a keyed digest of it.
type IssueQuotationResult struct {
	Quotation     models.Quotation
	Version       models.QuotationVersion
	CustomerToken string
	TokenExpiry   *time.Time
}

// IssueQuotation freezes an approved quotation into an immutable version and
// delivers it.
//
// The order of operations is the point:
//
//  1. Media is re-read before the transaction opens, so a document revoked
//     since approval blocks the issue.
//  2. The transaction locks the quotation, re-checks the approval against the
//     exact approved payload, re-checks the customer's status, risk and credit
//     hold, writes the immutable version, mints the token digest, moves the
//     status, and records history, audit and outbox atomically.
//  3. Notification is not in that transaction. A notification outage must not
//     roll back a commitment already made; the durable intent survives and the
//     worker keeps trying.
func (s *Service) IssueQuotation(ctx context.Context, t tenant.Context, quotationID string,
	input IssueQuotationInput) (IssueQuotationResult, error) {
	if err := s.writeEnabled(s.opts.Flags.QuotationIssueEnabled); err != nil {
		return IssueQuotationResult{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return IssueQuotationResult{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return IssueQuotationResult{}, err
	}

	preflight, err := s.requireQuotation(ctx, nil, t, quotationID)
	if err != nil {
		return IssueQuotationResult{}, err
	}
	if models.DeliveredQuotation(preflight.Status) {
		// Already issued. A repeat is a replay of a delivery that happened, and
		// re-issuing would mint a second version of the same offer.
		version, versionErr := s.opts.Repository.GetQuotationVersion(ctx, nil, t.OrganizationID,
			quotationID, preflight.IssuedVersionID)
		if versionErr != nil {
			return IssueQuotationResult{}, translate(versionErr, "Quotation version",
				errorcodes.QuotationNotFound, errorcodes.QuotationVersionConflict)
		}
		return IssueQuotationResult{Quotation: preflight, Version: version}, nil
	}
	if preflight.Status != models.QuotationApproved {
		return IssueQuotationResult{}, conflict(errorcodes.QuotationApprovalRequired,
			"Only an approved quotation can be issued.")
	}
	// Outside the transaction on purpose: an HTTP call to Media inside an open
	// transaction holds a database connection hostage to somebody else's outage.
	if err := s.refreshQuotationDocuments(ctx, t, quotationID); err != nil {
		return IssueQuotationResult{}, err
	}

	var result IssueQuotationResult
	var rawToken string
	var contactReference, channel string
	err = s.withTx(ctx, func(tx pgx.Tx) error {
		quotation, txErr := s.opts.Repository.LockQuotation(ctx, tx, t.OrganizationID,
			quotationID)
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationVersionConflict)
		}
		if !t.BranchPermitted(quotation.BranchID) {
			return notFound("Quotation")
		}
		if quotation.Status != models.QuotationApproved {
			return conflict(errorcodes.QuotationApprovalRequired,
				"Only an approved quotation can be issued.")
		}
		if !quotation.ValidUntil.After(s.now()) {
			return conflict(errorcodes.QuotationExpired,
				"The quotation's validity period has already ended.")
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
		if txErr = s.opts.Repository.RequireMandatoryDocumentsReady(ctx, tx, t.OrganizationID,
			quotationID); txErr != nil {
			if errors.Is(txErr, repositories.ErrDocumentNotReady) {
				return conflict(errorcodes.QuotationDocumentNotReady,
					"A required quotation document is not confirmed safe and ready.")
			}
			return txErr
		}

		items, txErr := s.opts.Repository.ListLineItems(ctx, tx, t.OrganizationID, quotationID)
		if txErr != nil {
			return txErr
		}
		if len(items) == 0 {
			return conflict(errorcodes.QuotationInvalidState,
				"A quotation with no line items cannot be issued.")
		}
		total, txErr := models.QuotationTotal(quotation.Currency, items)
		if txErr != nil {
			return badRequest(errorcodes.QuotationInvalidState, txErr.Error())
		}
		contentFingerprint := quotationFingerprint(quotation, items, total)

		// The approval is re-checked against the exact payload that was
		// approved. If the content moved since the reviewer said yes, the
		// framework refuses, and what was approved is what is issued.
		if quotation.ApprovalRequestID != "" && s.opts.Approvals != nil {
			if txErr = s.opts.Approvals.RequireApproved(ctx, t.OrganizationID,
				quotation.ApprovalRequestID, map[string]any{
					"quotationId":        quotationID,
					"quotationNumber":    quotation.QuotationNumber,
					"customerId":         quotation.CustomerID,
					"currency":           quotation.Currency,
					"totalAmountMinor":   total,
					"lineItemCount":      len(items),
					"contentFingerprint": contentFingerprint,
				}); txErr != nil {
				return txErr
			}
		}

		primary := s.primaryContactFor(ctx, tx, t.OrganizationID, quotation.CustomerID,
			models.PurposeSales)
		documents, txErr := s.opts.Repository.ListQuotationDocuments(ctx, tx, t.OrganizationID,
			quotationID)
		if txErr != nil {
			return txErr
		}
		documentIDs := make([]string, 0, len(documents))
		for _, document := range documents {
			documentIDs = append(documentIDs, document.ID)
		}

		version := models.QuotationVersion{
			ID:                   ids.New(repositories.PrefixQuotationVersion),
			OrganizationID:       t.OrganizationID,
			QuotationID:          quotationID,
			VersionNumber:        quotation.CurrentVersion,
			CustomerSnapshot:     customerSnapshot(customer),
			ContactSnapshot:      contactSnapshot(primary),
			Currency:             quotation.Currency,
			LineItemsSnapshot:    items,
			ServiceScopeSnapshot: quotation.ServiceScope,
			TermsSnapshot:        quotation.Terms,
			TotalAmountMinor:     total,
			ValidFrom:            quotation.ValidFrom,
			ValidUntil:           quotation.ValidUntil,
			DocumentReferences:   documentIDs,
			ApprovalRequestID:    quotation.ApprovalRequestID,
			IssuedBy:             t.OrganizationUserID,
			ContentFingerprint:   contentFingerprint,
		}
		issuedVersion, txErr := s.opts.Repository.CreateQuotationVersion(ctx, tx, version)
		if txErr != nil {
			return translate(txErr, "Quotation version", errorcodes.QuotationNotFound,
				errorcodes.QuotationVersionConflict)
		}
		if txErr = s.opts.Repository.BindQuotationDocumentsToVersion(ctx, tx, t.OrganizationID,
			quotationID, issuedVersion.ID); txErr != nil {
			return txErr
		}

		if input.MintCustomerToken {
			raw, hashed, tokenErr := s.mintToken()
			if tokenErr != nil {
				return tokenErr
			}
			expiry := s.now().Add(s.opts.TokenTTL)
			if expiry.After(quotation.ValidUntil) {
				// A link that outlives the offer it points at is a link that
				// invites an acceptance nobody meant to allow.
				expiry = quotation.ValidUntil
			}
			token := models.QuotationAccessToken{
				ID:                 ids.New(repositories.PrefixQuotationToken),
				OrganizationID:     t.OrganizationID,
				QuotationID:        quotationID,
				QuotationVersionID: issuedVersion.ID,
				TokenHash:          hashed,
				Scope:              models.TokenScopeViewAndRespond,
				SingleUse:          input.SingleUseToken,
				IssuedToContactID:  primary.ID,
				ExpiresAt:          expiry,
				CreatedBy:          t.OrganizationUserID,
			}
			if _, txErr = s.opts.Repository.CreateAccessToken(ctx, tx, token); txErr != nil {
				return translate(txErr, "Quotation", errorcodes.QuotationTokenInvalid,
					errorcodes.QuotationTokenInvalid)
			}
			rawToken = raw
			result.TokenExpiry = &expiry
		}

		updated, txErr := s.opts.Repository.TransitionQuotation(ctx, tx, t.OrganizationID,
			quotationID, input.ExpectedVersion, repositories.QuotationTransition{
				FromStatus: models.QuotationApproved, ToStatus: models.QuotationIssued,
				IssuedVersionID: issuedVersion.ID, TotalAmountMinor: &total,
				MarkIssued: true, Actor: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationVersionConflict)
		}
		result.Quotation = updated
		result.Version = issuedVersion

		if txErr = s.opts.Repository.AppendQuotationHistory(ctx, tx, models.QuotationHistory{
			OrganizationID: t.OrganizationID, QuotationID: quotationID,
			PreviousStatus: quotation.Status, NewStatus: updated.Status,
			QuotationVersion:  issuedVersion.VersionNumber,
			ApprovalRequestID: quotation.ApprovalRequestID, ActorType: string(t.ActorType),
			ActorID: t.ActorID, CorrelationID: input.CorrelationID,
			CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.quotation.issued", "quotation", quotationID,
			map[string]any{"status": quotation.Status},
			map[string]any{"status": updated.Status, "issuedVersionId": issuedVersion.ID},
			""); txErr != nil {
			return txErr
		}

		contactReference = primary.ID
		channel = notificationChannelFor(primary)
		if txErr = s.enqueueNotification(ctx, tx, t.OrganizationID, "quotation", quotationID,
			models.IntentQuotationIssued, contactReference, channel, models.JSONMap{
				"quotationNumber":  updated.QuotationNumber,
				"currency":         updated.Currency,
				"totalAmountMinor": total,
				"validUntil":       updated.ValidUntil.UTC().Format(time.RFC3339),
			}, "quotation_issued:"+issuedVersion.ID, input.CorrelationID,
			input.CausationID); txErr != nil {
			return txErr
		}

		return s.publish(ctx, tx, EventQuotationIssued, "quotation", quotationID,
			updated.Version, t.OrganizationID, input.CorrelationID,
			quotationStatusEventPayload(updated, quotation.Status, ""))
	})
	if err != nil {
		return IssueQuotationResult{}, err
	}
	result.CustomerToken = rawToken
	s.count("quotations_issued_total", nil)
	return result, nil
}

// customerSnapshot is what an issued quotation version records about the
// customer.
//
// The legal name is there because a quotation is a document addressed to a
// named party. The tax and registration references are not: an immutable
// version is written once and never redacted, so anything put in it is there
// forever, and a versioned copy of somebody's identity documents is a liability
// nothing in this flow needs.
func customerSnapshot(customer models.Customer) models.JSONMap {
	return models.JSONMap{
		"customerId":   customer.ID,
		"legalName":    customer.LegalName,
		"displayName":  customer.DisplayName,
		"customerType": customer.CustomerType,
		"currency":     customer.DefaultCurrency,
	}
}

// contactSnapshot is what an issued version records about the recipient.
//
// The contact ID and role, not the address. Delivery resolves the address from
// the live contact record under that contact's current consent, so a customer
// who withdraws consent is not still reachable through a frozen snapshot.
func contactSnapshot(contact models.Contact) models.JSONMap {
	if contact.ID == "" {
		return models.JSONMap{}
	}
	return models.JSONMap{
		"contactId":      contact.ID,
		"roleTitle":      contact.RoleTitle,
		"contactPurpose": contact.ContactPurpose,
	}
}

// CancelQuotationInput withdraws a quotation.
type CancelQuotationInput struct {
	ReasonCode      string
	ExpectedVersion int64
	IdempotencyKey  string
	CorrelationID   string
	CausationID     string
}

// CancelQuotation withdraws a quotation that has not been resolved.
func (s *Service) CancelQuotation(ctx context.Context, t tenant.Context, quotationID string,
	input CancelQuotationInput) (models.Quotation, error) {
	if err := s.writeEnabled(s.opts.Flags.QuotationsEnabled); err != nil {
		return models.Quotation{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Quotation{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Quotation{}, err
	}
	if strings.TrimSpace(input.ReasonCode) == "" {
		return models.Quotation{}, badRequest(errorcodes.QuotationInvalidState,
			"reasonCode is required to cancel a quotation.")
	}
	return s.resolveQuotation(ctx, t, quotationID, models.QuotationCancelled,
		EventQuotationCancelled, "crm.quotation.cancelled", input.ReasonCode,
		input.ExpectedVersion, input.CorrelationID, input.CausationID, "")
}

// SupersedeQuotationInput replaces a quotation with another.
type SupersedeQuotationInput struct {
	ReplacementQuotationID string
	ReasonCode             string
	ExpectedVersion        int64
	IdempotencyKey         string
	CorrelationID          string
	CausationID            string
}

// SupersedeQuotation marks a quotation as replaced by another.
//
// The original is preserved in full — its issued version, its history and its
// documents all stay exactly as they were. What changes is that its customer
// access tokens are revoked: a customer holding a link to a withdrawn offer
// must not be able to accept it, and revoking the link is the only thing that
// actually stops them.
func (s *Service) SupersedeQuotation(ctx context.Context, t tenant.Context, quotationID string,
	input SupersedeQuotationInput) (models.Quotation, error) {
	if err := s.writeEnabled(s.opts.Flags.QuotationsEnabled); err != nil {
		return models.Quotation{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Quotation{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Quotation{}, err
	}
	if strings.TrimSpace(input.ReplacementQuotationID) == "" {
		return models.Quotation{}, badRequest(errorcodes.QuotationInvalidState,
			"replacementQuotationId is required.")
	}
	return s.resolveQuotation(ctx, t, quotationID, models.QuotationSuperseded,
		EventQuotationSuperseded, "crm.quotation.superseded", input.ReasonCode,
		input.ExpectedVersion, input.CorrelationID, input.CausationID,
		input.ReplacementQuotationID)
}

// resolveQuotation performs a terminal quotation transition.
func (s *Service) resolveQuotation(ctx context.Context, t tenant.Context, quotationID,
	toStatus, eventType, auditAction, reason string, expectedVersion int64,
	correlationID, causationID, replacementID string) (models.Quotation, error) {
	var resolved models.Quotation
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
		if quotation.Status == toStatus {
			resolved = quotation
			return nil
		}
		if models.TerminalQuotation(quotation.Status) {
			return conflict(errorcodes.QuotationAlreadyResolved,
				"This quotation has already been resolved and cannot be changed.")
		}
		if !models.CanTransitionQuotation(quotation.Status, toStatus) {
			return conflict(errorcodes.QuotationInvalidState,
				"That change is not permitted from the quotation's current status.")
		}

		if replacementID != "" {
			replacement, replErr := s.opts.Repository.GetQuotation(ctx, tx, t.OrganizationID,
				replacementID)
			if replErr != nil || replacement.CustomerID != quotation.CustomerID {
				return errorcodes.NotFound(errorcodes.QuotationNotFound,
					"The replacement quotation was not found.")
			}
		}

		resolved, txErr = s.opts.Repository.TransitionQuotation(ctx, tx, t.OrganizationID,
			quotationID, expectedVersion, repositories.QuotationTransition{
				FromStatus: quotation.Status, ToStatus: toStatus,
				SupersededByID: replacementID, MarkResolved: true,
				Actor: t.OrganizationUserID,
			})
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationVersionConflict)
		}
		if txErr = s.opts.Repository.RevokeQuotationTokens(ctx, tx, t.OrganizationID,
			quotationID, toStatus); txErr != nil {
			return txErr
		}
		if txErr = s.opts.Repository.AppendQuotationHistory(ctx, tx, models.QuotationHistory{
			OrganizationID: t.OrganizationID, QuotationID: quotationID,
			PreviousStatus: quotation.Status, NewStatus: resolved.Status,
			QuotationVersion: resolved.CurrentVersion, ReasonCode: reason,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			CorrelationID: correlationID, CausationID: causationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, auditAction, "quotation", quotationID,
			map[string]any{"status": quotation.Status},
			map[string]any{"status": resolved.Status}, reason); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, eventType, "quotation", quotationID, resolved.Version,
			t.OrganizationID, correlationID,
			quotationStatusEventPayload(resolved, quotation.Status, reason))
	})
	if err != nil {
		return models.Quotation{}, err
	}
	return resolved, nil
}
