package http

import (
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Sales leads (Sections 14, 43.3, 59.6)
// ---------------------------------------------------------------------------
//
// Leads use the customer permissions, as Section 59.6 specifies
// (customers.read/manage). Converting a lead into an opportunity additionally
// requires the opportunity grant, checked only when an opportunity is
// actually requested.

// LeadView is the staff projection of a lead. The captured contact snapshot
// is personal data and appears only in SensitiveLeadView.
type LeadView struct {
	ID                     string `json:"id"`
	BranchID               string `json:"branchId,omitempty"`
	Source                 string `json:"source"`
	CompanyName            string `json:"companyName"`
	Status                 string `json:"status"`
	OwnerUserID            string `json:"ownerUserId,omitempty"`
	Score                  *int   `json:"score,omitempty"`
	NextAction             string `json:"nextAction,omitempty"`
	NextActionAt           string `json:"nextActionAt,omitempty"`
	DisqualificationReason string `json:"disqualificationReason,omitempty"`
	ConvertedCustomerID    string `json:"convertedCustomerId,omitempty"`
	ConvertedOpportunityID string `json:"convertedOpportunityId,omitempty"`
	ConvertedAt            string `json:"convertedAt,omitempty"`
	Version                int64  `json:"version"`
	CreatedAt              string `json:"createdAt"`
	UpdatedAt              string `json:"updatedAt"`
}

// SensitiveLeadView adds the captured contact details and notes.
type SensitiveLeadView struct {
	LeadView
	ContactSnapshot map[string]any `json:"contactSnapshot"`
	Notes           string         `json:"notes,omitempty"`
}

// NewLeadView builds the staff projection.
func NewLeadView(lead models.Lead) LeadView {
	return LeadView{
		ID: lead.ID, BranchID: lead.BranchID, Source: lead.Source, CompanyName: lead.CompanyName,
		Status: lead.Status, OwnerUserID: lead.OwnerUserID, Score: lead.Score,
		NextAction: lead.NextAction, NextActionAt: formatTimePtr(lead.NextActionAt),
		DisqualificationReason: lead.DisqualificationReason,
		ConvertedCustomerID:    lead.ConvertedCustomerID,
		ConvertedOpportunityID: lead.ConvertedOpportunityID,
		ConvertedAt:            formatTimePtr(lead.ConvertedAt),
		Version:                lead.Version, CreatedAt: formatTime(lead.CreatedAt),
		UpdatedAt: formatTime(lead.UpdatedAt),
	}
}

// NewSensitiveLeadView builds the projection for callers holding the
// sensitive-data grant.
func NewSensitiveLeadView(lead models.Lead) SensitiveLeadView {
	contact := lead.ContactSnapshot
	if contact == nil {
		contact = map[string]any{}
	}
	return SensitiveLeadView{LeadView: NewLeadView(lead), ContactSnapshot: contact, Notes: lead.Notes}
}

// LeadHistoryView is one immutable lead change.
type LeadHistoryView struct {
	ID             string `json:"id"`
	ChangeType     string `json:"changeType"`
	PreviousStatus string `json:"previousStatus,omitempty"`
	NewStatus      string `json:"newStatus"`
	ReasonCode     string `json:"reasonCode,omitempty"`
	ActorType      string `json:"actorType"`
	OccurredAt     string `json:"occurredAt"`
}

// LeadConversionView is the outcome of a lead conversion.
type LeadConversionView struct {
	Lead            LeadView         `json:"lead"`
	Customer        CustomerView     `json:"customer"`
	CustomerCreated bool             `json:"customerCreated"`
	Opportunity     *OpportunityView `json:"opportunity,omitempty"`
	Replayed        bool             `json:"replayed"`
}

func (h *Handler) leadView(c *gin.Context, t tenant.Context, lead models.Lead) any {
	if h.mayReadSensitive(c, t) {
		return NewSensitiveLeadView(lead)
	}
	return NewLeadView(lead)
}

func parseOptionalRFC3339(name, raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, apperrors.BadRequest(name+" must be an RFC 3339 timestamp.", nil)
	}
	return &parsed, nil
}

type createLeadRequest struct {
	BranchID        string         `json:"branchId"`
	Source          string         `json:"source"`
	CompanyName     string         `json:"companyName"`
	ContactSnapshot map[string]any `json:"contactSnapshot"`
	OwnerUserID     string         `json:"ownerUserId"`
	Score           *int           `json:"score"`
	NextAction      string         `json:"nextAction"`
	NextActionAt    string         `json:"nextActionAt"`
	Notes           string         `json:"notes"`
}

func (h *Handler) createLead(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, &t, PermissionCustomersManage) {
		return
	}
	var body createLeadRequest
	if !bindJSON(c, &body) {
		return
	}
	nextActionAt, err := parseOptionalRFC3339("nextActionAt", body.NextActionAt)
	if err != nil {
		respond.Error(c, err)
		return
	}
	lead, err := h.service.CreateLead(c.Request.Context(), t, usecases.CreateLeadInput{
		BranchID: body.BranchID, Source: body.Source, CompanyName: body.CompanyName,
		ContactSnapshot: body.ContactSnapshot, OwnerUserID: body.OwnerUserID, Score: body.Score,
		NextAction: body.NextAction, NextActionAt: nextActionAt, Notes: body.Notes,
		IdempotencyKey: idempotencyKey(c), CorrelationID: correlationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, h.leadView(c, t, lead))
}

func (h *Handler) listLeads(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, &t, PermissionCustomersRead) {
		return
	}
	if err := allowedQuery(c, "status", "source", "ownerUserId", "branchId",
		"createdAfter", "createdBefore"); err != nil {
		respond.Error(c, err)
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	createdAfter, err := parseTimeQuery(c, "createdAfter")
	if err != nil {
		respond.Error(c, err)
		return
	}
	createdBefore, err := parseTimeQuery(c, "createdBefore")
	if err != nil {
		respond.Error(c, err)
		return
	}
	leads, err := h.service.ListLeads(c.Request.Context(), t, repositories.LeadFilter{
		Status: c.Query("status"), Source: c.Query("source"), OwnerUserID: c.Query("ownerUserId"),
		BranchID: c.Query("branchId"), CreatedAfter: createdAfter, CreatedBefore: createdBefore,
		Cursor: page, HasCursor: hasCursor, Limit: limit,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	sensitive := h.mayReadSensitive(c, t)
	views := make([]any, 0, len(leads))
	for _, lead := range leads {
		if sensitive {
			views = append(views, NewSensitiveLeadView(lead))
		} else {
			views = append(views, NewLeadView(lead))
		}
	}
	token := ""
	if len(leads) > 0 {
		last := leads[len(leads)-1]
		token = nextCursor(len(leads), limit, last.CreatedAt, last.ID)
	}
	respond.List(c, views, token)
}

func (h *Handler) getLead(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, &t, PermissionCustomersRead) {
		return
	}
	lead, err := h.service.GetLead(c.Request.Context(), t, c.Param("leadId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, h.leadView(c, t, lead))
}

type updateLeadRequest struct {
	CompanyName     *string        `json:"companyName"`
	Source          *string        `json:"source"`
	ContactSnapshot map[string]any `json:"contactSnapshot"`
	Score           *int           `json:"score"`
	ClearScore      bool           `json:"clearScore"`
	NextAction      *string        `json:"nextAction"`
	NextActionAt    string         `json:"nextActionAt"`
	Notes           *string        `json:"notes"`
	ExpectedVersion int64          `json:"expectedVersion"`
}

func (h *Handler) updateLead(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, &t, PermissionCustomersManage) {
		return
	}
	var body updateLeadRequest
	if !bindJSON(c, &body) {
		return
	}
	nextActionAt, err := parseOptionalRFC3339("nextActionAt", body.NextActionAt)
	if err != nil {
		respond.Error(c, err)
		return
	}
	lead, err := h.service.UpdateLead(c.Request.Context(), t, c.Param("leadId"), usecases.UpdateLeadInput{
		CompanyName: body.CompanyName, Source: body.Source, ContactSnapshot: body.ContactSnapshot,
		Score: body.Score, ClearScore: body.ClearScore, NextAction: body.NextAction,
		NextActionAt: nextActionAt, Notes: body.Notes,
		ExpectedVersion: expectedVersion(c, body.ExpectedVersion), CorrelationID: correlationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, h.leadView(c, t, lead))
}

type assignLeadRequest struct {
	OwnerUserID     string `json:"ownerUserId"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) assignLead(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, &t, PermissionCustomersManage) {
		return
	}
	var body assignLeadRequest
	if !bindJSON(c, &body) {
		return
	}
	lead, err := h.service.AssignLead(c.Request.Context(), t, c.Param("leadId"), usecases.AssignLeadInput{
		OwnerUserID: body.OwnerUserID, ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
		IdempotencyKey: idempotencyKey(c), CorrelationID: correlationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, h.leadView(c, t, lead))
}

type changeLeadStatusRequest struct {
	Status          string `json:"status"`
	ReasonCode      string `json:"reasonCode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) changeLeadStatus(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, &t, PermissionCustomersManage) {
		return
	}
	var body changeLeadStatusRequest
	if !bindJSON(c, &body) {
		return
	}
	lead, err := h.service.ChangeLeadStatus(c.Request.Context(), t, c.Param("leadId"),
		usecases.ChangeLeadStatusInput{
			Status: body.Status, ReasonCode: body.ReasonCode,
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, h.leadView(c, t, lead))
}

type convertLeadOpportunityRequest struct {
	Title                  string     `json:"title"`
	Description            string     `json:"description"`
	ExpectedValue          *MoneyView `json:"expectedValue"`
	ProbabilityBasisPoints int        `json:"probabilityBasisPoints"`
	ExpectedCloseDate      string     `json:"expectedCloseDate"`
	ServiceCapabilities    []string   `json:"serviceCapabilities"`
	NextAction             string     `json:"nextAction"`
}

type convertLeadRequest struct {
	// CustomerID maps the lead onto an existing company CRM customer.
	CustomerID string `json:"customerId"`
	// NewCustomer creates a company CRM customer from the lead. legalName,
	// branchId and accountOwnerUserId default from the lead when omitted.
	NewCustomer     *createCustomerRequest         `json:"newCustomer"`
	Opportunity     *convertLeadOpportunityRequest `json:"opportunity"`
	ExpectedVersion int64                          `json:"expectedVersion"`
}

func (h *Handler) convertLead(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, &t, PermissionCustomersManage) {
		return
	}
	var body convertLeadRequest
	if !bindJSON(c, &body) {
		return
	}
	input := usecases.ConvertLeadInput{
		ExistingCustomerID: body.CustomerID,
		ExpectedVersion:    expectedVersion(c, body.ExpectedVersion),
		IdempotencyKey:     idempotencyKey(c), CorrelationID: correlationID(c),
	}
	if body.NewCustomer != nil {
		// marketplaceCustomerId is accepted by the customer DTO but never
		// carried into a conversion: the usecase clears it.
		input.NewCustomer = &usecases.CreateCustomerInput{
			BranchID: body.NewCustomer.BranchID, CustomerType: body.NewCustomer.CustomerType,
			LegalName: body.NewCustomer.LegalName, DisplayName: body.NewCustomer.DisplayName,
			RegistrationNumber:    body.NewCustomer.RegistrationNumber,
			TaxReference:          body.NewCustomer.TaxReference,
			AccountOwnerUserID:    body.NewCustomer.AccountOwnerUserID,
			DefaultCurrency:       body.NewCustomer.DefaultCurrency,
			PaymentTermsReference: body.NewCustomer.PaymentTermsReference,
			IdempotencyKey:        idempotencyKey(c), CorrelationID: correlationID(c),
		}
	}
	if body.Opportunity != nil {
		// Opening an opportunity is a separate grant from managing customers.
		if !h.authorize(c, &t, PermissionOpportunitiesWrite) {
			return
		}
		opportunity := &usecases.CreateOpportunityInput{
			Title: body.Opportunity.Title, Description: body.Opportunity.Description,
			ProbabilityBasisPoints: body.Opportunity.ProbabilityBasisPoints,
			ServiceCapabilities:    body.Opportunity.ServiceCapabilities,
			NextAction:             body.Opportunity.NextAction,
			IdempotencyKey:         idempotencyKey(c), CorrelationID: correlationID(c),
		}
		if body.Opportunity.ExpectedValue != nil {
			amount := body.Opportunity.ExpectedValue.AmountMinor
			opportunity.ExpectedValueAmountMinor = &amount
			opportunity.ExpectedValueCurrency = body.Opportunity.ExpectedValue.Currency
		}
		if body.Opportunity.ExpectedCloseDate != "" {
			parsed, err := time.Parse("2006-01-02", body.Opportunity.ExpectedCloseDate)
			if err != nil {
				respond.Error(c, apperrors.BadRequest("opportunity.expectedCloseDate must be a YYYY-MM-DD date.", nil))
				return
			}
			opportunity.ExpectedCloseDate = &parsed
		}
		input.Opportunity = opportunity
	}
	result, err := h.service.ConvertLead(c.Request.Context(), t, c.Param("leadId"), input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	view := LeadConversionView{
		Lead: NewLeadView(result.Lead), Customer: NewCustomerView(result.Customer),
		CustomerCreated: result.CustomerCreated, Replayed: result.Replayed,
	}
	if result.Opportunity != nil {
		opportunity := NewOpportunityView(*result.Opportunity)
		view.Opportunity = &opportunity
	}
	if result.Replayed {
		respond.OK(c, view)
		return
	}
	respond.Created(c, view)
}

func (h *Handler) leadHistory(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, &t, PermissionCustomersRead) {
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	entries, err := h.service.LeadHistory(c.Request.Context(), t, c.Param("leadId"), page, hasCursor, limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]LeadHistoryView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, LeadHistoryView{
			ID: entry.ID, ChangeType: entry.ChangeType, PreviousStatus: entry.PreviousStatus,
			NewStatus: entry.NewStatus, ReasonCode: entry.ReasonCode, ActorType: entry.ActorType,
			OccurredAt: formatTime(entry.OccurredAt),
		})
	}
	token := ""
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		token = nextCursor(len(entries), limit, last.OccurredAt, last.ID)
	}
	respond.List(c, views, token)
}
