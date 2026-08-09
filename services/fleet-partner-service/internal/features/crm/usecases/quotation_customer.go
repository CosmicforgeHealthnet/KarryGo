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

// CustomerQuotationView is everything a customer holding a secure link may see.
//
// The type exists so the customer surface is built by construction rather than
// by pruning. Nothing in it can accidentally acquire an internal note, an
// approval record, a risk status, a credit limit, a wallet balance, a provider
// reference, a margin or a staff identity, because there is no field for any of
// them and the only constructor is the one below.
type CustomerQuotationView struct {
	QuotationNumber  string                  `json:"quotationNumber"`
	Status           string                  `json:"status"`
	Currency         string                  `json:"currency"`
	TotalAmountMinor int64                   `json:"totalAmountMinor"`
	ValidFrom        string                  `json:"validFrom"`
	ValidUntil       string                  `json:"validUntil"`
	IssuedAt         string                  `json:"issuedAt"`
	CustomerName     string                  `json:"customerName"`
	LineItems        []CustomerQuotationLine `json:"lineItems"`
	ServiceScope     map[string]any          `json:"serviceScope,omitempty"`
	Terms            map[string]any          `json:"terms,omitempty"`
	CanRespond       bool                    `json:"canRespond"`
	Expired          bool                    `json:"expired"`
}

// CustomerQuotationLine is one line as the customer sees it.
type CustomerQuotationLine struct {
	Description     string `json:"description"`
	Quantity        string `json:"quantity"`
	UnitType        string `json:"unitType,omitempty"`
	UnitAmountMinor int64  `json:"unitAmountMinor"`
	LineAmountMinor int64  `json:"lineAmountMinor"`
	Currency        string `json:"currency"`
}

// tokenContext is a validated secure link.
type tokenContext struct {
	token     models.QuotationAccessToken
	quotation models.Quotation
	version   models.QuotationVersion
}

// resolveToken validates a presented raw token.
//
// The lookup is by keyed digest alone. The quotation ID from the request is
// compared against what the token says afterwards and is never used to find the
// row: a token that names a different quotation must fail, and it cannot fail
// if the query trusted the request to locate it in the first place.
//
// Every rejection returns the same generic answer. A caller must not be able to
// tell "wrong quotation" from "expired" from "no such token", because that
// difference is an oracle for probing which quotations exist.
func (s *Service) resolveToken(ctx context.Context, rawToken, quotationID string,
	allowResolved bool) (tokenContext, error) {
	if len(s.opts.TokenSecret) == 0 || strings.TrimSpace(rawToken) == "" {
		return tokenContext{}, errorcodes.Unauthorized(errorcodes.QuotationTokenInvalid,
			"This link is not valid.")
	}
	token, err := s.opts.Repository.FindAccessTokenByHash(ctx, nil,
		hashWithSecret(s.opts.TokenSecret, rawToken))
	if err != nil {
		return tokenContext{}, errorcodes.Unauthorized(errorcodes.QuotationTokenInvalid,
			"This link is not valid.")
	}
	if quotationID != "" && token.QuotationID != quotationID {
		return tokenContext{}, errorcodes.Unauthorized(errorcodes.QuotationTokenInvalid,
			"This link is not valid.")
	}
	now := s.now()
	if !now.Before(token.ExpiresAt) {
		return tokenContext{}, errorcodes.Unauthorized(errorcodes.QuotationTokenExpired,
			"This link has expired.")
	}

	quotation, err := s.opts.Repository.GetQuotation(ctx, nil, token.OrganizationID,
		token.QuotationID)
	if err != nil {
		return tokenContext{}, errorcodes.Unauthorized(errorcodes.QuotationTokenInvalid,
			"This link is not valid.")
	}
	if token.Status != models.TokenStatusActive {
		// Accepting or rejecting revokes the link. A customer whose response
		// was lost in transit must still be able to retry and be told what
		// happened — otherwise a dropped connection reads to them as a broken
		// link, and they try to accept again through some other route.
		//
		// Permitting this is safe because it only reaches an idempotent replay:
		// the quotation is already terminal, so every write path downstream
		// refuses, and a retry carrying different content still conflicts.
		if !allowResolved || !models.TerminalQuotation(quotation.Status) {
			return tokenContext{}, errorcodes.Unauthorized(errorcodes.QuotationTokenInvalid,
				"This link is no longer valid.")
		}
	}
	version, err := s.opts.Repository.GetQuotationVersion(ctx, nil, token.OrganizationID,
		token.QuotationID, token.QuotationVersionID)
	if err != nil {
		return tokenContext{}, errorcodes.Unauthorized(errorcodes.QuotationTokenInvalid,
			"This link is not valid.")
	}
	return tokenContext{token: token, quotation: quotation, version: version}, nil
}

// ViewQuotationAsCustomer returns the customer-facing quotation and records the
// view.
//
// The ISSUED to VIEWED transition is idempotent: a customer refreshing the page
// does not produce a second transition, a second history row or a second event.
func (s *Service) ViewQuotationAsCustomer(ctx context.Context, rawToken, quotationID,
	correlationID string) (CustomerQuotationView, error) {
	if err := s.readEnabled(); err != nil {
		return CustomerQuotationView{}, err
	}
	resolved, err := s.resolveToken(ctx, rawToken, quotationID, false)
	if err != nil {
		return CustomerQuotationView{}, err
	}

	now := s.now()
	var latest = resolved.quotation
	if resolved.quotation.Status == models.QuotationIssued {
		err = s.withTx(ctx, func(tx pgx.Tx) error {
			quotation, txErr := s.opts.Repository.LockQuotation(ctx, tx,
				resolved.token.OrganizationID, resolved.token.QuotationID)
			if txErr != nil {
				return txErr
			}
			if quotation.Status != models.QuotationIssued {
				// Somebody else moved it — accepted, expired, cancelled. The
				// view still renders; it just does not re-transition.
				latest = quotation
				return nil
			}
			latest, txErr = s.opts.Repository.TransitionQuotation(ctx, tx,
				quotation.OrganizationID, quotation.ID, quotation.Version,
				repositories.QuotationTransition{
					FromStatus: models.QuotationIssued, ToStatus: models.QuotationViewed,
					Actor: models.ActorCustomer,
				})
			if txErr != nil {
				return txErr
			}
			if txErr = s.opts.Repository.AppendQuotationHistory(ctx, tx,
				models.QuotationHistory{
					OrganizationID: quotation.OrganizationID, QuotationID: quotation.ID,
					PreviousStatus: quotation.Status, NewStatus: latest.Status,
					QuotationVersion: latest.CurrentVersion,
					ActorType:        models.ActorCustomer,
					// The token record ID, never the raw token and never an IP.
					ActorID:       resolved.token.ID,
					CorrelationID: correlationID,
				}); txErr != nil {
				return txErr
			}
			return s.publish(ctx, tx, EventQuotationViewed, "quotation", quotation.ID,
				latest.Version, quotation.OrganizationID, correlationID,
				quotationViewedEventPayload(latest, now))
		})
		if err != nil {
			return CustomerQuotationView{}, err
		}
		s.count("quotations_viewed_total", nil)
	}

	// Recorded outside the transition so a repeat view still updates the
	// counter, and so a single-use token is consumed exactly when it is used.
	_ = s.opts.Repository.RecordTokenView(ctx, nil, resolved.token.ID)

	return buildCustomerView(latest, resolved.version, resolved.token, now), nil
}

// buildCustomerView constructs the customer-facing projection field by field.
//
// It reads from the immutable issued version, not the live quotation: what the
// customer sees is what they were offered, even if a draft revision has moved
// on since.
func buildCustomerView(quotation models.Quotation, version models.QuotationVersion,
	token models.QuotationAccessToken, now time.Time) CustomerQuotationView {
	lines := make([]CustomerQuotationLine, 0, len(version.LineItemsSnapshot))
	for _, item := range version.LineItemsSnapshot {
		lines = append(lines, CustomerQuotationLine{
			Description:     item.Description,
			Quantity:        item.Quantity,
			UnitType:        item.UnitType,
			UnitAmountMinor: item.UnitAmountMinor,
			LineAmountMinor: item.LineAmountMinor,
			Currency:        item.Currency,
		})
	}
	customerName, _ := version.CustomerSnapshot["legalName"].(string)
	expired := !now.Before(version.ValidUntil)
	return CustomerQuotationView{
		QuotationNumber:  quotation.QuotationNumber,
		Status:           quotation.Status,
		Currency:         version.Currency,
		TotalAmountMinor: version.TotalAmountMinor,
		ValidFrom:        version.ValidFrom.UTC().Format(time.RFC3339),
		ValidUntil:       version.ValidUntil.UTC().Format(time.RFC3339),
		IssuedAt:         version.IssuedAt.UTC().Format(time.RFC3339),
		CustomerName:     customerName,
		LineItems:        lines,
		ServiceScope:     version.ServiceScopeSnapshot,
		Terms:            version.TermsSnapshot,
		CanRespond: token.Scope == models.TokenScopeViewAndRespond && !expired &&
			models.DeliveredQuotation(quotation.Status),
		Expired: expired,
	}
}

// AcceptQuotationInput records that a customer took the offer.
type AcceptQuotationInput struct {
	// AcceptedVersionID must name the exact issued version being accepted. A
	// mismatch is refused rather than resolved in the customer's favour: an
	// acceptance of "the current version" is an acceptance of something nobody
	// has agreed is the same thing.
	AcceptedVersionID         string
	AcceptanceMethod          string
	ActorType                 string
	ActorReference            string
	EvidenceDocumentReference string
	RemoteAddress             string
	UserAgent                 string
	ExpectedVersion           int64
	IdempotencyKey            string
	CorrelationID             string
	CausationID               string
}

// AcceptQuotationResult is the acceptance and the conversion intent it created.
type AcceptQuotationResult struct {
	Quotation        models.Quotation
	Acceptance       models.Acceptance
	ConversionIntent models.ConversionIntent
	Replayed         bool
}

// AcceptQuotationByToken records a customer acceptance made through a secure
// link.
func (s *Service) AcceptQuotationByToken(ctx context.Context, rawToken, quotationID string,
	input AcceptQuotationInput) (AcceptQuotationResult, error) {
	resolved, err := s.resolveToken(ctx, rawToken, quotationID, true)
	if err != nil {
		return AcceptQuotationResult{}, err
	}
	if resolved.token.Scope != models.TokenScopeViewAndRespond {
		return AcceptQuotationResult{}, errorcodes.Forbidden(errorcodes.QuotationTokenInvalid,
			"This link does not permit responding to the quotation.")
	}
	input.AcceptanceMethod = models.AcceptSecureToken
	input.ActorType = models.ActorCustomer
	// The token record ID, not the raw token: an acceptance record is evidence,
	// and evidence containing a working credential is a liability.
	input.ActorReference = resolved.token.ID
	if input.AcceptedVersionID == "" {
		input.AcceptedVersionID = resolved.token.QuotationVersionID
	}
	return s.acceptQuotation(ctx, resolved.token.OrganizationID, resolved.token.QuotationID,
		input)
}

// AcceptQuotationAsStaff records an acceptance a member of staff observed.
//
// An offline or integration acceptance is a claim about something that happened
// outside this system, so it needs evidence: a document reference or an
// explicit actor reference. Recording "the customer said yes on the phone" with
// nothing attached is not evidence, it is an assertion.
func (s *Service) AcceptQuotationAsStaff(ctx context.Context, t tenant.Context,
	quotationID string, input AcceptQuotationInput) (AcceptQuotationResult, error) {
	if !models.ValidAcceptanceMethod(input.AcceptanceMethod) {
		return AcceptQuotationResult{}, badRequest(errorcodes.QuotationAcceptanceConflict,
			"acceptanceMethod is not supported.")
	}
	if input.AcceptanceMethod == models.AcceptSecureToken {
		return AcceptQuotationResult{}, badRequest(errorcodes.QuotationAcceptanceConflict,
			"A secure-token acceptance must be made through the customer link.")
	}
	if input.AcceptanceMethod == models.AcceptOfflineDocumented &&
		strings.TrimSpace(input.EvidenceDocumentReference) == "" &&
		strings.TrimSpace(input.ActorReference) == "" {
		return AcceptQuotationResult{}, badRequest(errorcodes.QuotationAcceptanceConflict,
			"A documented offline acceptance requires an evidence document reference or an actor reference.")
	}
	if _, err := s.requireQuotation(ctx, nil, t, quotationID); err != nil {
		return AcceptQuotationResult{}, err
	}
	switch input.AcceptanceMethod {
	case models.AcceptIntegration:
		input.ActorType = models.ActorIntegration
	default:
		input.ActorType = models.ActorOrganization
		if input.ActorReference == "" {
			input.ActorReference = t.OrganizationUserID
		}
	}
	return s.acceptQuotation(ctx, t.OrganizationID, quotationID, input)
}

// acceptQuotation is the single acceptance path.
//
// Both the customer link and the staff record converge here so there is exactly
// one place that decides what an acceptance means, one place that creates the
// conversion intent, and no second implementation to drift.
func (s *Service) acceptQuotation(ctx context.Context, organizationID, quotationID string,
	input AcceptQuotationInput) (AcceptQuotationResult, error) {
	if err := s.writeEnabled(s.opts.Flags.QuotationAcceptEnabled); err != nil {
		return AcceptQuotationResult{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return AcceptQuotationResult{}, err
	}

	requestFingerprint := fingerprint(quotationID, input.AcceptedVersionID,
		input.AcceptanceMethod, input.ActorType, input.ActorReference,
		input.EvidenceDocumentReference)

	// A replay is answered from the stored acceptance before anything else
	// happens. A network timeout on the customer's side must not turn into a
	// second acceptance or a confusing conflict.
	if existing, err := s.opts.Repository.FindAcceptanceByKey(ctx, nil, organizationID,
		input.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != requestFingerprint {
			return AcceptQuotationResult{}, conflict(errorcodes.QuotationAcceptanceConflict,
				"This idempotency key was already used for a different acceptance.")
		}
		quotation, quotationErr := s.opts.Repository.GetQuotation(ctx, nil, organizationID,
			existing.QuotationID)
		if quotationErr != nil {
			return AcceptQuotationResult{}, translate(quotationErr, "Quotation",
				errorcodes.QuotationNotFound, errorcodes.QuotationAcceptanceConflict)
		}
		intent, intentErr := s.opts.Repository.GetConversionIntent(ctx, nil, organizationID,
			existing.AcceptedVersionID)
		if intentErr != nil {
			return AcceptQuotationResult{}, intentErr
		}
		return AcceptQuotationResult{Quotation: quotation, Acceptance: existing,
			ConversionIntent: intent, Replayed: true}, nil
	}

	var result AcceptQuotationResult
	var contactReference, channel string
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		quotation, txErr := s.opts.Repository.LockQuotation(ctx, tx, organizationID,
			quotationID)
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationAcceptanceConflict)
		}
		if !models.DeliveredQuotation(quotation.Status) {
			if models.TerminalQuotation(quotation.Status) {
				return conflict(errorcodes.QuotationAlreadyResolved,
					"This quotation has already been resolved and cannot be accepted.")
			}
			return conflict(errorcodes.QuotationInvalidState,
				"Only an issued quotation can be accepted.")
		}
		// Server time decides. A client clock is the one thing a party with an
		// interest in a late acceptance controls.
		if quotation.Expired(s.now()) {
			return conflict(errorcodes.QuotationExpired,
				"This quotation's validity period has ended.")
		}
		if quotation.IssuedVersionID == "" ||
			input.AcceptedVersionID != quotation.IssuedVersionID {
			return conflict(errorcodes.QuotationVersionConflict,
				"The accepted version does not match the version that was issued.")
		}

		customer, txErr := s.opts.Repository.GetCustomer(ctx, tx, organizationID,
			quotation.CustomerID)
		if txErr != nil {
			return translate(txErr, "Customer", errorcodes.CustomerNotFound,
				errorcodes.CustomerInvalidState)
		}
		if customer.IsArchived() {
			return conflict(errorcodes.CustomerArchived,
				"This customer is archived and cannot accept a quotation.")
		}
		// A staff-recorded acceptance is a new commercial commitment being
		// entered on the organization's behalf, so a credit hold blocks it. A
		// customer clicking accept on a link they were already sent is not
		// blocked: the offer was already made, and revoking it after the fact is
		// a supersession, not a silent refusal.
		if input.AcceptanceMethod != models.AcceptSecureToken {
			if txErr = s.requireNoBlockingCreditHold(ctx, tx, organizationID,
				quotation.CustomerID); txErr != nil {
				return txErr
			}
		}

		acceptance := models.Acceptance{
			ID:                ids.New(repositories.PrefixAcceptance),
			OrganizationID:    organizationID,
			QuotationID:       quotationID,
			AcceptedVersionID: input.AcceptedVersionID,
			ActorType:         input.ActorType,
			ActorReference:    input.ActorReference,
			AcceptanceMethod:  input.AcceptanceMethod,
			// Keyed digests, never the values. An acceptance record is evidence
			// that somebody accepted, not a log of where they were sitting.
			IPHash:                    hashWithSecret(s.opts.ContactSecret, input.RemoteAddress),
			UserAgentHash:             hashWithSecret(s.opts.ContactSecret, input.UserAgent),
			EvidenceDocumentReference: input.EvidenceDocumentReference,
			IdempotencyKey:            input.IdempotencyKey,
			RequestFingerprint:        requestFingerprint,
			CorrelationID:             input.CorrelationID,
			CausationID:               input.CausationID,
		}
		stored, txErr := s.opts.Repository.CreateAcceptance(ctx, tx, acceptance)
		if txErr != nil {
			if errors.Is(txErr, repositories.ErrDuplicate) {
				return conflict(errorcodes.QuotationAcceptanceConflict,
					"This quotation has already been accepted.")
			}
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationAcceptanceConflict)
		}
		result.Acceptance = stored

		// The entire Section 15 surface of an acceptance. No order, no shipment,
		// no package, no assignment, no reservation, no trip, no wallet
		// movement — not as omissions, but because none of those things exist
		// yet and inventing a placeholder for them would be a lie downstream
		// consumers would act on.
		intent, txErr := s.opts.Repository.CreateConversionIntent(ctx, tx,
			models.ConversionIntent{
				ID:                ids.New(repositories.PrefixConversionIntent),
				OrganizationID:    organizationID,
				QuotationID:       quotationID,
				AcceptedVersionID: input.AcceptedVersionID,
				AcceptanceID:      stored.ID,
				IdempotencyKey:    input.IdempotencyKey,
				Fingerprint:       requestFingerprint,
				CorrelationID:     input.CorrelationID,
				CausationID:       input.CausationID,
			})
		if txErr != nil {
			if errors.Is(txErr, repositories.ErrDuplicate) {
				return conflict(errorcodes.QuotationAcceptanceConflict,
					"This quotation version has already been converted.")
			}
			return txErr
		}
		result.ConversionIntent = intent

		updated, txErr := s.opts.Repository.TransitionQuotation(ctx, tx, organizationID,
			quotationID, quotation.Version, repositories.QuotationTransition{
				FromStatus: quotation.Status, ToStatus: models.QuotationAccepted,
				AcceptedVersionID: input.AcceptedVersionID, MarkResolved: true,
				Actor: input.ActorReference,
			})
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationAcceptanceConflict)
		}
		result.Quotation = updated

		if txErr = s.opts.Repository.RevokeQuotationTokens(ctx, tx, organizationID,
			quotationID, "accepted"); txErr != nil {
			return txErr
		}
		if txErr = s.opts.Repository.AppendQuotationHistory(ctx, tx, models.QuotationHistory{
			OrganizationID: organizationID, QuotationID: quotationID,
			PreviousStatus: quotation.Status, NewStatus: updated.Status,
			QuotationVersion: updated.CurrentVersion, ActorType: input.ActorType,
			ActorID: input.ActorReference, CorrelationID: input.CorrelationID,
			CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.audit.Record(ctx, tx, acceptanceAuditEntry(organizationID, quotationID,
			stored, intent, input.CorrelationID)); txErr != nil {
			return txErr
		}

		primary := s.primaryContactFor(ctx, tx, organizationID, quotation.CustomerID,
			models.PurposeSales)
		contactReference, channel = primary.ID, notificationChannelFor(primary)
		if txErr = s.enqueueNotification(ctx, tx, organizationID, "quotation", quotationID,
			models.IntentQuotationAccepted, contactReference, channel, models.JSONMap{
				"quotationNumber": updated.QuotationNumber,
			}, "quotation_accepted:"+stored.ID, input.CorrelationID,
			input.CausationID); txErr != nil {
			return txErr
		}

		return s.publish(ctx, tx, EventQuotationAccepted, "quotation", quotationID,
			updated.Version, organizationID, input.CorrelationID,
			quotationAcceptedEventPayload(updated, stored, intent))
	})
	if err != nil {
		return AcceptQuotationResult{}, err
	}
	s.count("quotations_accepted_total",
		map[string]string{"method": input.AcceptanceMethod})
	s.count("quotation_conversion_intents_total", nil)
	return result, nil
}

// acceptanceAuditEntry builds the acceptance audit record.
//
// The actor reference is a token record ID or a staff member ID. There is no IP
// address and no user-agent string here or anywhere else that is readable: only
// the keyed digests on the acceptance row itself.
func acceptanceAuditEntry(organizationID, quotationID string, acceptance models.Acceptance,
	intent models.ConversionIntent, correlationID string) auditEntry {
	return auditEntry{
		OrganizationID: organizationID,
		ActorType:      acceptance.ActorType,
		ActorID:        acceptance.ActorReference,
		Action:         "crm.quotation.accepted",
		ResourceType:   "quotation",
		ResourceID:     quotationID,
		NewValues: map[string]any{
			"acceptanceId":       acceptance.ID,
			"acceptedVersionId":  acceptance.AcceptedVersionID,
			"acceptanceMethod":   acceptance.AcceptanceMethod,
			"conversionIntentId": intent.ID,
			"conversionStatus":   intent.Status,
		},
		CorrelationID: correlationID,
	}
}

// RejectQuotationInput records that a customer declined the offer.
type RejectQuotationInput struct {
	ReasonCode                string
	Note                      string
	ActorType                 string
	ActorReference            string
	EvidenceDocumentReference string
	ExpectedVersion           int64
	IdempotencyKey            string
	CorrelationID             string
	CausationID               string
}

// RejectQuotationByToken records a rejection made through a secure link.
func (s *Service) RejectQuotationByToken(ctx context.Context, rawToken, quotationID string,
	input RejectQuotationInput) (models.Quotation, error) {
	resolved, err := s.resolveToken(ctx, rawToken, quotationID, true)
	if err != nil {
		return models.Quotation{}, err
	}
	if resolved.token.Scope != models.TokenScopeViewAndRespond {
		return models.Quotation{}, errorcodes.Forbidden(errorcodes.QuotationTokenInvalid,
			"This link does not permit responding to the quotation.")
	}
	input.ActorType = models.ActorCustomer
	input.ActorReference = resolved.token.ID
	// A customer declining does not get to leave a note in the operator's
	// internal record: the note field on this path is discarded.
	input.Note = ""
	return s.rejectQuotation(ctx, resolved.token.OrganizationID, resolved.token.QuotationID,
		input)
}

// RejectQuotationAsStaff records a rejection a member of staff observed.
func (s *Service) RejectQuotationAsStaff(ctx context.Context, t tenant.Context,
	quotationID string, input RejectQuotationInput) (models.Quotation, error) {
	if _, err := s.requireQuotation(ctx, nil, t, quotationID); err != nil {
		return models.Quotation{}, err
	}
	input.ActorType = models.ActorOrganization
	if input.ActorReference == "" {
		input.ActorReference = t.OrganizationUserID
	}
	return s.rejectQuotation(ctx, t.OrganizationID, quotationID, input)
}

func (s *Service) rejectQuotation(ctx context.Context, organizationID, quotationID string,
	input RejectQuotationInput) (models.Quotation, error) {
	if err := s.writeEnabled(s.opts.Flags.QuotationAcceptEnabled); err != nil {
		return models.Quotation{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Quotation{}, err
	}
	if strings.TrimSpace(input.ReasonCode) == "" {
		return models.Quotation{}, badRequest(errorcodes.QuotationInvalidState,
			"reasonCode is required to reject a quotation.")
	}

	var rejected models.Quotation
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		quotation, txErr := s.opts.Repository.LockQuotation(ctx, tx, organizationID,
			quotationID)
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationVersionConflict)
		}
		if quotation.Status == models.QuotationRejected {
			rejected = quotation
			return nil
		}
		if models.TerminalQuotation(quotation.Status) {
			// An accepted quotation cannot later be rejected, and a rejected one
			// cannot later be accepted. Whichever resolution landed first is the
			// one the customer was told about.
			return conflict(errorcodes.QuotationAlreadyResolved,
				"This quotation has already been resolved.")
		}
		if !models.DeliveredQuotation(quotation.Status) {
			return conflict(errorcodes.QuotationInvalidState,
				"Only an issued quotation can be rejected.")
		}

		rejected, txErr = s.opts.Repository.TransitionQuotation(ctx, tx, organizationID,
			quotationID, quotation.Version, repositories.QuotationTransition{
				FromStatus: quotation.Status, ToStatus: models.QuotationRejected,
				MarkResolved: true, Actor: input.ActorReference,
			})
		if txErr != nil {
			return translate(txErr, "Quotation", errorcodes.QuotationNotFound,
				errorcodes.QuotationVersionConflict)
		}
		if txErr = s.opts.Repository.RevokeQuotationTokens(ctx, tx, organizationID,
			quotationID, "rejected"); txErr != nil {
			return txErr
		}
		if txErr = s.opts.Repository.AppendQuotationHistory(ctx, tx, models.QuotationHistory{
			OrganizationID: organizationID, QuotationID: quotationID,
			PreviousStatus: quotation.Status, NewStatus: rejected.Status,
			QuotationVersion: rejected.CurrentVersion, ReasonCode: input.ReasonCode,
			ActorType: input.ActorType, ActorID: input.ActorReference,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID,
		}); txErr != nil {
			return txErr
		}
		if txErr = s.audit.Record(ctx, tx, auditEntry{
			OrganizationID: organizationID, ActorType: input.ActorType,
			ActorID: input.ActorReference, Action: "crm.quotation.rejected",
			ResourceType: "quotation", ResourceID: quotationID,
			PreviousValues: map[string]any{"status": quotation.Status},
			NewValues:      map[string]any{"status": rejected.Status},
			Reason:         input.ReasonCode, CorrelationID: input.CorrelationID,
		}); txErr != nil {
			return txErr
		}

		primary := s.primaryContactFor(ctx, tx, organizationID, quotation.CustomerID,
			models.PurposeSales)
		if txErr = s.enqueueNotification(ctx, tx, organizationID, "quotation", quotationID,
			models.IntentQuotationRejected, primary.ID, notificationChannelFor(primary),
			models.JSONMap{"quotationNumber": rejected.QuotationNumber},
			"quotation_rejected:"+quotationID, input.CorrelationID,
			input.CausationID); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventQuotationRejected, "quotation", quotationID,
			rejected.Version, organizationID, input.CorrelationID,
			quotationStatusEventPayload(rejected, quotation.Status, input.ReasonCode))
	})
	if err != nil {
		return models.Quotation{}, err
	}
	s.count("quotations_rejected_total", nil)
	return rejected, nil
}
