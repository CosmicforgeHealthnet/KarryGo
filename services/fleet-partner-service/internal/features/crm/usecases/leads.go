package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Sales lead events. Payloads carry identifiers and status projections only;
// the captured contact snapshot (names, emails, phones) never leaves through
// the outbox.
const (
	EventLeadCreated       = "organization.sales_lead.created.v1"
	EventLeadUpdated       = "organization.sales_lead.updated.v1"
	EventLeadAssigned      = "organization.sales_lead.assigned.v1"
	EventLeadStatusChanged = "organization.sales_lead.status_changed.v1"
	EventLeadConverted     = "organization.sales_lead.converted.v1"
)

func leadEventPayload(lead models.Lead) map[string]any {
	return map[string]any{
		"leadId":         lead.ID,
		"organizationId": lead.OrganizationID,
		"branchId":       lead.BranchID,
		"source":         lead.Source,
		"status":         lead.Status,
		"hasOwner":       lead.OwnerUserID != "",
		"version":        lead.Version,
		"occurredAt":     lead.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func leadAuditValues(lead models.Lead) map[string]any {
	// The contact snapshot is personal data; the audit row records that a
	// lead changed and how, not who the prospect's people are.
	return map[string]any{
		"companyName": lead.CompanyName,
		"source":      lead.Source,
		"status":      lead.Status,
		"branchId":    lead.BranchID,
		"ownerUserId": lead.OwnerUserID,
		"hasScore":    lead.Score != nil,
	}
}

func leadNotFound() error {
	return errorcodes.NotFound(errorcodes.LeadNotFound, "Lead was not found.")
}

func translateLead(err error) error {
	return translate(err, "Lead", errorcodes.LeadNotFound, errorcodes.LeadInvalidTransition)
}

// requireLead reads (or locks) a lead and refuses one outside the caller's
// branch scope exactly as it refuses a missing one.
func (s *Service) requireLead(ctx context.Context, tx pgx.Tx, t tenant.Context, leadID string,
	lock bool) (models.Lead, error) {
	var lead models.Lead
	var err error
	switch {
	case tx == nil:
		lead, err = s.opts.Repository.GetLead(ctx, nil, t.OrganizationID, leadID)
	case lock:
		lead, err = s.opts.Repository.LockLead(ctx, tx, t.OrganizationID, leadID)
	default:
		lead, err = s.opts.Repository.GetLead(ctx, tx, t.OrganizationID, leadID)
	}
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return lead, leadNotFound()
		}
		return lead, err
	}
	if !t.BranchPermitted(lead.BranchID) {
		return lead, leadNotFound()
	}
	return lead, nil
}

func (s *Service) appendLeadHistory(ctx context.Context, tx pgx.Tx, t tenant.Context, lead models.Lead,
	changeType, previous, reason, correlationID string) error {
	return s.opts.Repository.AppendLeadHistory(ctx, tx, models.LeadHistory{
		OrganizationID: t.OrganizationID, LeadID: lead.ID, ChangeType: changeType,
		PreviousStatus: previous, NewStatus: lead.Status, ReasonCode: reason,
		ActorType: string(t.ActorType), ActorID: t.ActorID, CorrelationID: correlationID,
	})
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// CreateLeadInput captures a new lead.
type CreateLeadInput struct {
	BranchID        string
	Source          string
	CompanyName     string
	ContactSnapshot map[string]any
	OwnerUserID     string
	Score           *int
	NextAction      string
	NextActionAt    *time.Time
	Notes           string
	IdempotencyKey  string
	CorrelationID   string
}

func leadCreateFingerprint(input CreateLeadInput) string {
	encoded, _ := json.Marshal(map[string]any{
		"branchId": input.BranchID, "source": input.Source, "companyName": input.CompanyName,
		"contact": input.ContactSnapshot, "ownerUserId": input.OwnerUserID, "score": input.Score,
		"nextAction": input.NextAction, "nextActionAt": input.NextActionAt, "notes": input.Notes,
	})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (s *Service) requireLeadOwner(ctx context.Context, q pgx.Tx, t tenant.Context, ownerUserID string) error {
	if ownerUserID == "" {
		return nil
	}
	active, err := s.opts.Repository.ActiveOrganizationUser(ctx, q, t.OrganizationID, ownerUserID)
	if err != nil {
		return err
	}
	if !active {
		// Another tenant's user and a nonexistent one are the same answer.
		return badRequest(errorcodes.LeadInvalidTransition,
			"ownerUserId is not an active member of this organization.")
	}
	return nil
}

// CreateLead captures a lead in NEW status.
//
// A retried create carrying the same Idempotency-Key returns the lead the
// first attempt made; the same key with a different body is refused, because
// the second caller believes something different was captured.
func (s *Service) CreateLead(ctx context.Context, t tenant.Context, input CreateLeadInput) (models.Lead, error) {
	if err := s.writeEnabled(s.opts.Flags.OpportunitiesEnabled); err != nil {
		return models.Lead{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Lead{}, err
	}
	input.Source = strings.ToUpper(strings.TrimSpace(input.Source))
	input.CompanyName = strings.TrimSpace(input.CompanyName)
	if input.Source == "" {
		input.Source = models.LeadSourceOther
	}
	if !models.ValidLeadSource(input.Source) {
		return models.Lead{}, badRequest(errorcodes.LeadInvalidTransition, "source is not supported.")
	}
	if input.CompanyName == "" {
		return models.Lead{}, badRequest(errorcodes.LeadInvalidTransition, "companyName is required.")
	}
	if err := models.ValidateLeadScore(input.Score); err != nil {
		return models.Lead{}, badRequest(errorcodes.LeadInvalidTransition, err.Error())
	}
	if input.BranchID != "" && !t.BranchPermitted(input.BranchID) {
		return models.Lead{}, badRequest(errorcodes.LeadInvalidTransition,
			"branchId is not within your authorized scope.")
	}
	fingerprint := leadCreateFingerprint(input)
	replay := func() (models.Lead, bool, error) {
		existing, err := s.opts.Repository.GetLeadByCreateKey(ctx, nil, t.OrganizationID, input.IdempotencyKey)
		if errors.Is(err, repositories.ErrNotFound) {
			return models.Lead{}, false, nil
		}
		if err != nil {
			return models.Lead{}, false, err
		}
		if existing.CreateFingerprint != fingerprint {
			return models.Lead{}, true, conflict(errorcodes.LeadInvalidTransition,
				"This Idempotency-Key was already used for a different lead.")
		}
		return existing, true, nil
	}
	if existing, found, err := replay(); found || err != nil {
		return existing, err
	}

	var created models.Lead
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		if txErr := s.requireLeadOwner(ctx, tx, t, input.OwnerUserID); txErr != nil {
			return txErr
		}
		var txErr error
		created, txErr = s.opts.Repository.CreateLead(ctx, tx, models.Lead{
			ID: ids.New(repositories.PrefixLead), OrganizationID: t.OrganizationID,
			BranchID: input.BranchID, Source: input.Source, CompanyName: input.CompanyName,
			ContactSnapshot: input.ContactSnapshot, Status: models.LeadNew,
			OwnerUserID: input.OwnerUserID, Score: input.Score, NextAction: input.NextAction,
			NextActionAt: input.NextActionAt, Notes: input.Notes,
			CreateIdempotencyKey: input.IdempotencyKey, CreateFingerprint: fingerprint,
			CreatedBy: t.OrganizationUserID,
		})
		if txErr != nil {
			return txErr
		}
		if txErr = s.appendLeadHistory(ctx, tx, t, created, models.LeadChangeCreated, "", "",
			input.CorrelationID); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.lead.created", "sales_lead", created.ID, nil,
			leadAuditValues(created), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventLeadCreated, "sales_lead", created.ID, created.Version,
			t.OrganizationID, input.CorrelationID, leadEventPayload(created))
	})
	if errors.Is(err, repositories.ErrDuplicate) {
		// Lost a race with a concurrent create under the same key.
		if existing, found, replayErr := replay(); found || replayErr != nil {
			return existing, replayErr
		}
	}
	if err != nil {
		return models.Lead{}, translateLead(err)
	}
	s.count("crm_leads_created_total", map[string]string{"source": created.Source})
	return created, nil
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// GetLead reads one lead.
func (s *Service) GetLead(ctx context.Context, t tenant.Context, leadID string) (models.Lead, error) {
	if err := s.readEnabled(); err != nil {
		return models.Lead{}, err
	}
	return s.requireLead(ctx, nil, t, leadID, false)
}

// ListLeads returns a tenant- and branch-scoped page.
func (s *Service) ListLeads(ctx context.Context, t tenant.Context,
	filter repositories.LeadFilter) ([]models.Lead, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if filter.Status != "" && !models.ValidLeadStatus(filter.Status) {
		return nil, badRequest(errorcodes.LeadInvalidTransition, "status filter is not supported.")
	}
	if filter.Source != "" && !models.ValidLeadSource(filter.Source) {
		return nil, badRequest(errorcodes.LeadInvalidTransition, "source filter is not supported.")
	}
	return s.opts.Repository.ListLeads(ctx, t.OrganizationID, t.PermittedBranchIDs, filter)
}

// LeadHistory returns a lead's immutable change log.
func (s *Service) LeadHistory(ctx context.Context, t tenant.Context, leadID string,
	page cursor.Cursor, hasCursor bool, limit int) ([]models.LeadHistory, error) {
	if err := s.readEnabled(); err != nil {
		return nil, err
	}
	if _, err := s.requireLead(ctx, nil, t, leadID, false); err != nil {
		return nil, err
	}
	return s.opts.Repository.ListLeadHistory(ctx, t.OrganizationID, leadID, page, hasCursor, limit)
}

// ---------------------------------------------------------------------------
// Update, assign, qualify
// ---------------------------------------------------------------------------

// UpdateLeadInput is a partial edit of an open lead. Ownership changes go
// through AssignLead so the assignee is always validated.
type UpdateLeadInput struct {
	CompanyName     *string
	Source          *string
	ContactSnapshot map[string]any
	Score           *int
	ClearScore      bool
	NextAction      *string
	NextActionAt    *time.Time
	Notes           *string
	ExpectedVersion int64
	CorrelationID   string
}

// UpdateLead edits an open (not disqualified, not converted) lead.
func (s *Service) UpdateLead(ctx context.Context, t tenant.Context, leadID string,
	input UpdateLeadInput) (models.Lead, error) {
	if err := s.writeEnabled(s.opts.Flags.OpportunitiesEnabled); err != nil {
		return models.Lead{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Lead{}, err
	}
	if input.CompanyName != nil {
		trimmed := strings.TrimSpace(*input.CompanyName)
		if trimmed == "" {
			return models.Lead{}, badRequest(errorcodes.LeadInvalidTransition, "companyName cannot be blank.")
		}
		input.CompanyName = &trimmed
	}
	if input.Source != nil {
		upper := strings.ToUpper(strings.TrimSpace(*input.Source))
		if !models.ValidLeadSource(upper) {
			return models.Lead{}, badRequest(errorcodes.LeadInvalidTransition, "source is not supported.")
		}
		input.Source = &upper
	}
	if err := models.ValidateLeadScore(input.Score); err != nil {
		return models.Lead{}, badRequest(errorcodes.LeadInvalidTransition, err.Error())
	}

	var updated models.Lead
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.requireLead(ctx, tx, t, leadID, true)
		if txErr != nil {
			return txErr
		}
		if models.TerminalLead(existing.Status) {
			return conflict(errorcodes.LeadInvalidTransition,
				"A disqualified or converted lead can no longer be edited.")
		}
		if existing.Version != input.ExpectedVersion {
			return repositories.ErrVersionConflict
		}
		updated, txErr = s.opts.Repository.UpdateLead(ctx, tx, t.OrganizationID, leadID,
			input.ExpectedVersion, repositories.LeadUpdate{
				CompanyName: input.CompanyName, Source: input.Source,
				ContactSnapshot: input.ContactSnapshot, Score: input.Score, ClearScore: input.ClearScore,
				NextAction: input.NextAction, NextActionAt: input.NextActionAt, Notes: input.Notes,
				UpdatedBy: t.OrganizationUserID,
			})
		if txErr != nil {
			return txErr
		}
		if txErr = s.appendLeadHistory(ctx, tx, t, updated, models.LeadChangeUpdated,
			existing.Status, "", input.CorrelationID); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.lead.updated", "sales_lead", updated.ID,
			leadAuditValues(existing), leadAuditValues(updated), ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventLeadUpdated, "sales_lead", updated.ID, updated.Version,
			t.OrganizationID, input.CorrelationID, leadEventPayload(updated))
	})
	if err != nil {
		return models.Lead{}, translateLead(err)
	}
	return updated, nil
}

// AssignLeadInput hands a lead to a staff member (or clears the owner).
type AssignLeadInput struct {
	OwnerUserID     string
	ExpectedVersion int64
	IdempotencyKey  string
	CorrelationID   string
}

// AssignLead sets the lead's owner. The assignee must be a live member of
// the same organization: a lead is never handed to another tenant's staff.
func (s *Service) AssignLead(ctx context.Context, t tenant.Context, leadID string,
	input AssignLeadInput) (models.Lead, error) {
	if err := s.writeEnabled(s.opts.Flags.OpportunitiesEnabled); err != nil {
		return models.Lead{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Lead{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Lead{}, err
	}
	owner := strings.TrimSpace(input.OwnerUserID)

	var updated models.Lead
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.requireLead(ctx, tx, t, leadID, true)
		if txErr != nil {
			return txErr
		}
		if models.TerminalLead(existing.Status) {
			return conflict(errorcodes.LeadInvalidTransition,
				"A disqualified or converted lead can no longer be reassigned.")
		}
		if existing.Version != input.ExpectedVersion {
			return repositories.ErrVersionConflict
		}
		if txErr = s.requireLeadOwner(ctx, tx, t, owner); txErr != nil {
			return txErr
		}
		updated, txErr = s.opts.Repository.UpdateLead(ctx, tx, t.OrganizationID, leadID,
			input.ExpectedVersion, repositories.LeadUpdate{OwnerUserID: &owner,
				UpdatedBy: t.OrganizationUserID})
		if txErr != nil {
			return txErr
		}
		if txErr = s.appendLeadHistory(ctx, tx, t, updated, models.LeadChangeAssigned,
			existing.Status, "", input.CorrelationID); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.lead.assigned", "sales_lead", updated.ID,
			map[string]any{"ownerUserId": existing.OwnerUserID},
			map[string]any{"ownerUserId": updated.OwnerUserID}, ""); txErr != nil {
			return txErr
		}
		return s.publish(ctx, tx, EventLeadAssigned, "sales_lead", updated.ID, updated.Version,
			t.OrganizationID, input.CorrelationID, leadEventPayload(updated))
	})
	if err != nil {
		return models.Lead{}, translateLead(err)
	}
	return updated, nil
}

// ChangeLeadStatusInput moves a lead through qualification.
type ChangeLeadStatusInput struct {
	Status          string
	ReasonCode      string
	ExpectedVersion int64
	IdempotencyKey  string
	CorrelationID   string
}

// ChangeLeadStatus applies a qualification transition. CONVERTED is not
// reachable here; it is reachable only through ConvertLead, which creates the
// customer and opportunity in the same transaction as the status change.
func (s *Service) ChangeLeadStatus(ctx context.Context, t tenant.Context, leadID string,
	input ChangeLeadStatusInput) (models.Lead, error) {
	if err := s.writeEnabled(s.opts.Flags.OpportunitiesEnabled); err != nil {
		return models.Lead{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return models.Lead{}, err
	}
	if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
		return models.Lead{}, err
	}
	target := strings.ToUpper(strings.TrimSpace(input.Status))
	reason := strings.TrimSpace(input.ReasonCode)
	if target == models.LeadConverted {
		return models.Lead{}, badRequest(errorcodes.LeadInvalidTransition,
			"Use the convert command to convert a lead.")
	}
	if !models.ValidLeadStatus(target) {
		return models.Lead{}, badRequest(errorcodes.LeadInvalidTransition, "status is not supported.")
	}
	if target == models.LeadDisqualified && reason == "" {
		return models.Lead{}, badRequest(errorcodes.LeadInvalidTransition,
			"reasonCode is required to disqualify a lead.")
	}

	var updated models.Lead
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		existing, txErr := s.requireLead(ctx, tx, t, leadID, true)
		if txErr != nil {
			return txErr
		}
		if existing.Version != input.ExpectedVersion {
			return repositories.ErrVersionConflict
		}
		if !models.CanTransitionLead(existing.Status, target) {
			return conflict(errorcodes.LeadInvalidTransition,
				"This lead cannot move from "+existing.Status+" to "+target+".")
		}
		disqualification := ""
		if target == models.LeadDisqualified {
			disqualification = reason
		}
		updated, txErr = s.opts.Repository.TransitionLead(ctx, tx, t.OrganizationID, leadID,
			input.ExpectedVersion, existing.Status, target, disqualification, t.OrganizationUserID)
		if txErr != nil {
			return txErr
		}
		if txErr = s.appendLeadHistory(ctx, tx, t, updated, models.LeadChangeStatusChanged,
			existing.Status, reason, input.CorrelationID); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.lead.status_changed", "sales_lead", updated.ID,
			map[string]any{"status": existing.Status}, map[string]any{"status": updated.Status},
			reason); txErr != nil {
			return txErr
		}
		payload := leadEventPayload(updated)
		payload["previousStatus"] = existing.Status
		payload["reasonCode"] = reason
		return s.publish(ctx, tx, EventLeadStatusChanged, "sales_lead", updated.ID, updated.Version,
			t.OrganizationID, input.CorrelationID, payload)
	})
	if err != nil {
		return models.Lead{}, translateLead(err)
	}
	return updated, nil
}

// ---------------------------------------------------------------------------
// Conversion
// ---------------------------------------------------------------------------

// ConvertLeadInput maps a lead onto a company CRM customer — an existing one
// (ExistingCustomerID) or a new one (NewCustomer) — and optionally opens a
// sales opportunity against it. Exactly one customer mapping is required.
type ConvertLeadInput struct {
	ExistingCustomerID string
	NewCustomer        *CreateCustomerInput
	Opportunity        *CreateOpportunityInput
	ExpectedVersion    int64
	IdempotencyKey     string
	CorrelationID      string
}

// LeadConversion is the outcome of converting a lead.
type LeadConversion struct {
	Lead            models.Lead
	Customer        models.Customer
	Opportunity     *models.Opportunity
	CustomerCreated bool
	// Replayed is true when this call returned the result of an earlier
	// conversion made under the same Idempotency-Key.
	Replayed bool
}

// ConvertLead converts a lead exactly once.
//
// The lead row is locked for the whole conversion, so two concurrent
// conversions serialize: the first creates the customer/opportunity and marks
// the lead CONVERTED; the second sees CONVERTED and either replays the first
// result (same Idempotency-Key) or is refused (different key). The customer,
// the opportunity, the status change, history, audit and outbox events all
// commit in one transaction or not at all.
//
// The customer is a tenant-private company CRM record. No marketplace
// customer identity is created, looked up or linked here.
func (s *Service) ConvertLead(ctx context.Context, t tenant.Context, leadID string,
	input ConvertLeadInput) (LeadConversion, error) {
	if err := s.writeEnabled(s.opts.Flags.OpportunitiesEnabled); err != nil {
		return LeadConversion{}, err
	}
	if err := requireIdempotencyKey(input.IdempotencyKey); err != nil {
		return LeadConversion{}, err
	}
	existingID := strings.TrimSpace(input.ExistingCustomerID)
	if (existingID == "") == (input.NewCustomer == nil) {
		return LeadConversion{}, badRequest(errorcodes.LeadInvalidTransition,
			"Provide exactly one of customerId (map to an existing customer) or newCustomer.")
	}
	if input.NewCustomer != nil {
		if err := s.writeEnabled(s.opts.Flags.CustomerWritesEnabled); err != nil {
			return LeadConversion{}, err
		}
	}

	var result LeadConversion
	err := s.withTx(ctx, func(tx pgx.Tx) error {
		lead, txErr := s.requireLead(ctx, tx, t, leadID, true)
		if txErr != nil {
			return txErr
		}
		if lead.Status == models.LeadConverted {
			if lead.ConversionIdempotencyKey != input.IdempotencyKey {
				return conflict(errorcodes.LeadAlreadyConverted, "This lead has already been converted.")
			}
			replayed, replayErr := s.loadConversion(ctx, tx, t, lead)
			if replayErr != nil {
				return replayErr
			}
			replayed.Replayed = true
			result = replayed
			return nil
		}
		if !models.LeadConvertible(lead.Status) {
			return conflict(errorcodes.LeadInvalidTransition, "A disqualified lead cannot be converted.")
		}
		if err := requireExpectedVersion(input.ExpectedVersion); err != nil {
			return err
		}
		if lead.Version != input.ExpectedVersion {
			return repositories.ErrVersionConflict
		}

		var customer models.Customer
		created := false
		if existingID != "" {
			customer, txErr = s.requireCustomer(ctx, tx, t, existingID)
			if txErr != nil {
				return txErr
			}
			if customer.IsArchived() {
				return conflict(errorcodes.CustomerArchived,
					"An archived customer cannot receive a converted lead.")
			}
		} else {
			details := *input.NewCustomer
			if strings.TrimSpace(details.LegalName) == "" {
				details.LegalName = lead.CompanyName
			}
			if details.BranchID == "" {
				details.BranchID = lead.BranchID
			}
			if details.AccountOwnerUserID == "" {
				details.AccountOwnerUserID = lead.OwnerUserID
			}
			// Deliberately never carried over from a lead: a converted company
			// customer is not a marketplace identity.
			details.MarketplaceCustomerID = ""
			details.CorrelationID = input.CorrelationID
			if txErr = s.validateCustomerInput(t, &details); txErr != nil {
				return txErr
			}
			customer, txErr = s.createCustomerTx(ctx, tx, t, details)
			if txErr != nil {
				return txErr
			}
			created = true
		}

		var opportunity *models.Opportunity
		if input.Opportunity != nil {
			details := *input.Opportunity
			details.CustomerID = customer.ID
			if details.BranchID == "" {
				details.BranchID = customer.BranchID
			}
			if details.AccountOwnerUserID == "" {
				details.AccountOwnerUserID = lead.OwnerUserID
			}
			details.CorrelationID = input.CorrelationID
			if txErr = validateOpportunityInput(t, &details); txErr != nil {
				return txErr
			}
			opened, openErr := s.createOpportunityTx(ctx, tx, t, customer, details)
			if openErr != nil {
				return openErr
			}
			opportunity = &opened
		}

		opportunityID := ""
		if opportunity != nil {
			opportunityID = opportunity.ID
		}
		converted, txErr := s.opts.Repository.MarkLeadConverted(ctx, tx, t.OrganizationID, leadID,
			input.ExpectedVersion, lead.Status, customer.ID, opportunityID, input.IdempotencyKey,
			t.OrganizationUserID)
		if txErr != nil {
			return txErr
		}
		if txErr = s.appendLeadHistory(ctx, tx, t, converted, models.LeadChangeConverted,
			lead.Status, "", input.CorrelationID); txErr != nil {
			return txErr
		}
		if txErr = s.record(ctx, tx, t, "crm.lead.converted", "sales_lead", converted.ID,
			map[string]any{"status": lead.Status},
			map[string]any{"status": converted.Status, "customerId": customer.ID,
				"opportunityId": opportunityID, "customerCreated": created}, ""); txErr != nil {
			return txErr
		}
		payload := leadEventPayload(converted)
		payload["previousStatus"] = lead.Status
		payload["customerId"] = customer.ID
		payload["customerCreated"] = created
		if opportunityID != "" {
			payload["opportunityId"] = opportunityID
		}
		if txErr = s.publish(ctx, tx, EventLeadConverted, "sales_lead", converted.ID,
			converted.Version, t.OrganizationID, input.CorrelationID, payload); txErr != nil {
			return txErr
		}
		result = LeadConversion{Lead: converted, Customer: customer, Opportunity: opportunity,
			CustomerCreated: created}
		return nil
	})
	if err != nil {
		return LeadConversion{}, translateLead(err)
	}
	if !result.Replayed {
		s.count("crm_leads_converted_total", map[string]string{
			"customerCreated": boolLabel(result.CustomerCreated),
			"opportunity":     boolLabel(result.Opportunity != nil)})
	}
	return result, nil
}

// loadConversion rebuilds a completed conversion's outcome from the lead's
// stored references, for an idempotent replay.
func (s *Service) loadConversion(ctx context.Context, tx pgx.Tx, t tenant.Context,
	lead models.Lead) (LeadConversion, error) {
	customer, err := s.opts.Repository.GetCustomer(ctx, tx, t.OrganizationID, lead.ConvertedCustomerID)
	if err != nil {
		return LeadConversion{}, translate(err, "Customer", errorcodes.CustomerNotFound,
			errorcodes.CustomerInvalidState)
	}
	out := LeadConversion{Lead: lead, Customer: customer}
	if lead.ConvertedOpportunityID != "" {
		opportunity, oppErr := s.opts.Repository.GetOpportunity(ctx, tx, t.OrganizationID,
			lead.ConvertedOpportunityID)
		if oppErr != nil {
			return LeadConversion{}, translate(oppErr, "Opportunity", errorcodes.OpportunityNotFound,
				errorcodes.OpportunityInvalidTransition)
		}
		out.Opportunity = &opportunity
	}
	return out, nil
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
