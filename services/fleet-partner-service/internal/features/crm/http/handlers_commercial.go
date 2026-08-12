package http

import (
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
)

// ---------------------------------------------------------------------------
// Quotations
// ---------------------------------------------------------------------------

// lineItemRequest is one manually priced line.
//
// lineAmountMinor is optional. Where it is supplied the service checks it
// against the exact decimal product of unit amount and quantity and refuses a
// mismatch rather than silently correcting it: a quotation whose lines do not
// add up is a dispute, and the caller needs to know which number is wrong.
type lineItemRequest struct {
	Description              string `json:"description"`
	Quantity                 string `json:"quantity"`
	UnitType                 string `json:"unitType"`
	UnitAmountMinor          int64  `json:"unitAmountMinor"`
	LineAmountMinor          *int64 `json:"lineAmountMinor"`
	ExternalPricingReference string `json:"externalPricingReference"`
	ManualReason             string `json:"manualReason"`
}

func toLineItemInputs(items []lineItemRequest) []usecases.LineItemInput {
	out := make([]usecases.LineItemInput, 0, len(items))
	for _, item := range items {
		out = append(out, usecases.LineItemInput{
			Description: item.Description, Quantity: item.Quantity, UnitType: item.UnitType,
			UnitAmountMinor: item.UnitAmountMinor, LineAmountMinor: item.LineAmountMinor,
			ExternalPricingReference: item.ExternalPricingReference,
			ManualReason:             item.ManualReason,
		})
	}
	return out
}

type createQuotationRequest struct {
	BranchID      string            `json:"branchId"`
	CustomerID    string            `json:"customerId"`
	OpportunityID string            `json:"opportunityId"`
	Currency      string            `json:"currency"`
	ValidFrom     string            `json:"validFrom"`
	ValidUntil    string            `json:"validUntil"`
	ServiceScope  models.JSONMap    `json:"serviceScope"`
	Terms         models.JSONMap    `json:"terms"`
	InternalNotes string            `json:"internalNotes"`
	LineItems     []lineItemRequest `json:"lineItems"`
}

func (h *Handler) createQuotation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsManage) {
		return
	}
	var body createQuotationRequest
	if !bindJSON(c, &body) {
		return
	}
	validFrom, err := time.Parse(time.RFC3339, body.ValidFrom)
	if err != nil {
		respond.Error(c, apperrors.BadRequest("validFrom must be an RFC 3339 timestamp.", nil))
		return
	}
	validUntil, err := time.Parse(time.RFC3339, body.ValidUntil)
	if err != nil {
		respond.Error(c, apperrors.BadRequest("validUntil must be an RFC 3339 timestamp.", nil))
		return
	}
	detail, err := h.service.CreateQuotation(c.Request.Context(), t,
		usecases.CreateQuotationInput{
			BranchID: body.BranchID, CustomerID: body.CustomerID,
			OpportunityID: body.OpportunityID, Currency: body.Currency,
			ValidFrom: validFrom, ValidUntil: validUntil, ServiceScope: body.ServiceScope,
			Terms: body.Terms, InternalNotes: body.InternalNotes,
			LineItems:      toLineItemInputs(body.LineItems),
			IdempotencyKey: idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewQuotationView(detail.Quotation, detail.LineItems))
}

func (h *Handler) listQuotations(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsRead) {
		return
	}
	if err := allowedQuery(c, "status", "customerId", "opportunityId", "branchId",
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
	quotations, err := h.service.ListQuotations(c.Request.Context(), t,
		repositories.QuotationFilter{Status: c.Query("status"),
			CustomerID: c.Query("customerId"), OpportunityID: c.Query("opportunityId"),
			BranchID: c.Query("branchId"), CreatedAfter: createdAfter,
			CreatedBefore: createdBefore, Cursor: page, HasCursor: hasCursor, Limit: limit})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]QuotationView, 0, len(quotations))
	for _, quotation := range quotations {
		// The list view omits line items: a page of quotations is a summary,
		// and shipping every line of every offer widens the blast radius of a
		// single over-broad read.
		views = append(views, NewQuotationView(quotation, nil))
	}
	token := ""
	if len(quotations) > 0 {
		last := quotations[len(quotations)-1]
		token = nextCursor(len(quotations), limit, last.CreatedAt, last.ID)
	}
	respond.List(c, views, token)
}

func (h *Handler) getQuotation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsRead) {
		return
	}
	detail, err := h.service.GetQuotation(c.Request.Context(), t, c.Param("quotationId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	if h.mayReadSensitive(c, t) {
		respond.OK(c, NewSensitiveQuotationView(detail.Quotation, detail.LineItems))
		return
	}
	respond.OK(c, NewQuotationView(detail.Quotation, detail.LineItems))
}

type updateQuotationRequest struct {
	ValidFrom        string            `json:"validFrom"`
	ValidUntil       string            `json:"validUntil"`
	ServiceScope     models.JSONMap    `json:"serviceScope"`
	Terms            models.JSONMap    `json:"terms"`
	InternalNotes    *string           `json:"internalNotes"`
	OpportunityID    *string           `json:"opportunityId"`
	LineItems        []lineItemRequest `json:"lineItems"`
	ReplaceLineItems bool              `json:"replaceLineItems"`
	ExpectedVersion  int64             `json:"expectedVersion"`
}

func (h *Handler) updateQuotation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsManage) {
		return
	}
	var body updateQuotationRequest
	if !bindJSON(c, &body) {
		return
	}
	input := usecases.UpdateQuotationInput{
		ServiceScope: body.ServiceScope, Terms: body.Terms,
		InternalNotes: body.InternalNotes, OpportunityID: body.OpportunityID,
		LineItems: toLineItemInputs(body.LineItems), ReplaceLineItems: body.ReplaceLineItems,
		ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
		CorrelationID:   correlationID(c),
	}
	if body.ValidFrom != "" {
		parsed, err := time.Parse(time.RFC3339, body.ValidFrom)
		if err != nil {
			respond.Error(c, apperrors.BadRequest(
				"validFrom must be an RFC 3339 timestamp.", nil))
			return
		}
		input.ValidFrom = &parsed
	}
	if body.ValidUntil != "" {
		parsed, err := time.Parse(time.RFC3339, body.ValidUntil)
		if err != nil {
			respond.Error(c, apperrors.BadRequest(
				"validUntil must be an RFC 3339 timestamp.", nil))
			return
		}
		input.ValidUntil = &parsed
	}
	detail, err := h.service.UpdateQuotation(c.Request.Context(), t, c.Param("quotationId"),
		input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewQuotationView(detail.Quotation, detail.LineItems))
}

func (h *Handler) submitQuotation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsSubmit) {
		return
	}
	var body reasonedRequest
	if !bindJSON(c, &body) {
		return
	}
	detail, err := h.service.SubmitQuotation(c.Request.Context(), t, c.Param("quotationId"),
		usecases.SubmitQuotationInput{
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewQuotationView(detail.Quotation, detail.LineItems))
}

type issueQuotationRequest struct {
	MintCustomerToken bool  `json:"mintCustomerToken"`
	SingleUseToken    bool  `json:"singleUseToken"`
	ExpectedVersion   int64 `json:"expectedVersion"`
}

// issueQuotationResponse carries the raw customer token exactly once.
//
// It is returned here and nowhere else: the database holds only a keyed digest,
// and there is no endpoint that can produce the raw value again. A caller that
// loses it must supersede the quotation and issue a new one.
type issueQuotationResponse struct {
	Quotation     QuotationView        `json:"quotation"`
	Version       QuotationVersionView `json:"issuedVersion"`
	CustomerToken string               `json:"customerAccessToken,omitempty"`
	TokenExpiry   string               `json:"customerAccessTokenExpiresAt,omitempty"`
}

func (h *Handler) issueQuotation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsIssue) {
		return
	}
	var body issueQuotationRequest
	if !bindJSON(c, &body) {
		return
	}
	result, err := h.service.IssueQuotation(c.Request.Context(), t, c.Param("quotationId"),
		usecases.IssueQuotationInput{
			MintCustomerToken: body.MintCustomerToken, SingleUseToken: body.SingleUseToken,
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	response := issueQuotationResponse{
		Quotation:     NewQuotationView(result.Quotation, nil),
		Version:       NewQuotationVersionView(result.Version),
		CustomerToken: result.CustomerToken,
	}
	if result.TokenExpiry != nil {
		response.TokenExpiry = formatTime(*result.TokenExpiry)
	}
	respond.OK(c, response)
}

type acceptQuotationRequest struct {
	AcceptedVersionID         string `json:"acceptedVersionId"`
	AcceptanceMethod          string `json:"acceptanceMethod"`
	ActorReference            string `json:"actorReference"`
	EvidenceDocumentReference string `json:"evidenceDocumentReference"`
	ExpectedVersion           int64  `json:"expectedVersion"`
}

type acceptQuotationResponse struct {
	Quotation        QuotationView        `json:"quotation"`
	Acceptance       AcceptanceView       `json:"acceptance"`
	ConversionIntent ConversionIntentView `json:"conversionIntent"`
	Replayed         bool                 `json:"replayed"`
}

func (h *Handler) acceptQuotation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsAccept) {
		return
	}
	var body acceptQuotationRequest
	if !bindJSON(c, &body) {
		return
	}
	result, err := h.service.AcceptQuotationAsStaff(c.Request.Context(), t,
		c.Param("quotationId"), usecases.AcceptQuotationInput{
			AcceptedVersionID:         body.AcceptedVersionID,
			AcceptanceMethod:          body.AcceptanceMethod,
			ActorReference:            body.ActorReference,
			EvidenceDocumentReference: body.EvidenceDocumentReference,
			ExpectedVersion:           expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:            idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, acceptQuotationResponse{
		Quotation:        NewQuotationView(result.Quotation, nil),
		Acceptance:       NewAcceptanceView(result.Acceptance),
		ConversionIntent: NewConversionIntentView(result.ConversionIntent),
		Replayed:         result.Replayed,
	})
}

type rejectQuotationRequest struct {
	ReasonCode                string `json:"reasonCode"`
	Note                      string `json:"note"`
	ActorReference            string `json:"actorReference"`
	EvidenceDocumentReference string `json:"evidenceDocumentReference"`
	ExpectedVersion           int64  `json:"expectedVersion"`
}

func (h *Handler) rejectQuotation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsAccept) {
		return
	}
	var body rejectQuotationRequest
	if !bindJSON(c, &body) {
		return
	}
	quotation, err := h.service.RejectQuotationAsStaff(c.Request.Context(), t,
		c.Param("quotationId"), usecases.RejectQuotationInput{
			ReasonCode: body.ReasonCode, Note: body.Note,
			ActorReference:            body.ActorReference,
			EvidenceDocumentReference: body.EvidenceDocumentReference,
			ExpectedVersion:           expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:            idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewQuotationView(quotation, nil))
}

func (h *Handler) cancelQuotation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsManage) {
		return
	}
	var body reasonedRequest
	if !bindJSON(c, &body) {
		return
	}
	quotation, err := h.service.CancelQuotation(c.Request.Context(), t,
		c.Param("quotationId"), usecases.CancelQuotationInput{
			ReasonCode:      body.ReasonCode,
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewQuotationView(quotation, nil))
}

type supersedeQuotationRequest struct {
	ReplacementQuotationID string `json:"replacementQuotationId"`
	ReasonCode             string `json:"reasonCode"`
	ExpectedVersion        int64  `json:"expectedVersion"`
}

func (h *Handler) supersedeQuotation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsManage) {
		return
	}
	var body supersedeQuotationRequest
	if !bindJSON(c, &body) {
		return
	}
	quotation, err := h.service.SupersedeQuotation(c.Request.Context(), t,
		c.Param("quotationId"), usecases.SupersedeQuotationInput{
			ReplacementQuotationID: body.ReplacementQuotationID,
			ReasonCode:             body.ReasonCode,
			ExpectedVersion:        expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:         idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewQuotationView(quotation, nil))
}

func (h *Handler) quotationVersions(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsRead) {
		return
	}
	_, _, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	versions, err := h.service.QuotationVersions(c.Request.Context(), t,
		c.Param("quotationId"), limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]QuotationVersionView, 0, len(versions))
	for _, version := range versions {
		views = append(views, NewQuotationVersionView(version))
	}
	respond.List(c, views, "")
}

func (h *Handler) quotationVersion(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsRead) {
		return
	}
	version, err := h.service.QuotationVersion(c.Request.Context(), t,
		c.Param("quotationId"), c.Param("versionId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewQuotationVersionView(version))
}

func (h *Handler) quotationHistory(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsRead) {
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	entries, err := h.service.QuotationHistory(c.Request.Context(), t, c.Param("quotationId"),
		page, hasCursor, limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]QuotationHistoryView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, NewQuotationHistoryView(entry))
	}
	token := ""
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		token = nextCursor(len(entries), limit, last.OccurredAt, last.ID)
	}
	respond.List(c, views, token)
}

type attachDocumentRequest struct {
	MediaObjectID   string `json:"mediaObjectId"`
	DocumentPurpose string `json:"documentPurpose"`
	IsMandatory     bool   `json:"isMandatory"`
}

func (h *Handler) attachQuotationDocument(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsManage) {
		return
	}
	var body attachDocumentRequest
	if !bindJSON(c, &body) {
		return
	}
	document, err := h.service.AttachQuotationDocument(c.Request.Context(), t,
		c.Param("quotationId"), usecases.AttachQuotationDocumentInput{
			MediaObjectID: body.MediaObjectID, DocumentPurpose: body.DocumentPurpose,
			IsMandatory: body.IsMandatory, CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewDocumentView(document))
}

func (h *Handler) quotationDocuments(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsRead) {
		return
	}
	documents, err := h.service.QuotationDocuments(c.Request.Context(), t,
		c.Param("quotationId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]DocumentView, 0, len(documents))
	for _, document := range documents {
		views = append(views, NewDocumentView(document))
	}
	respond.List(c, views, "")
}

// ---------------------------------------------------------------------------
// Contracts
// ---------------------------------------------------------------------------

type createContractRequest struct {
	BranchID           string         `json:"branchId"`
	CustomerID         string         `json:"customerId"`
	QuotationID        string         `json:"quotationId"`
	EffectiveFrom      string         `json:"effectiveFrom"`
	EffectiveTo        string         `json:"effectiveTo"`
	RenewalTerms       models.JSONMap `json:"renewalTerms"`
	ServiceScope       models.JSONMap `json:"serviceScope"`
	CommercialSnapshot models.JSONMap `json:"commercialSnapshot"`
	TerminationTerms   models.JSONMap `json:"terminationTerms"`
	SupersedesID       string         `json:"supersedesId"`
}

func (h *Handler) createContract(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsManage) {
		return
	}
	var body createContractRequest
	if !bindJSON(c, &body) {
		return
	}
	effectiveFrom, err := time.Parse(time.RFC3339, body.EffectiveFrom)
	if err != nil {
		respond.Error(c, apperrors.BadRequest(
			"effectiveFrom must be an RFC 3339 timestamp.", nil))
		return
	}
	input := usecases.CreateContractInput{
		BranchID: body.BranchID, CustomerID: body.CustomerID, QuotationID: body.QuotationID,
		EffectiveFrom: effectiveFrom, RenewalTerms: body.RenewalTerms,
		ServiceScope: body.ServiceScope, CommercialSnapshot: body.CommercialSnapshot,
		TerminationTerms: body.TerminationTerms, SupersedesID: body.SupersedesID,
		IdempotencyKey: idempotencyKey(c), CorrelationID: correlationID(c),
	}
	if body.EffectiveTo != "" {
		parsed, parseErr := time.Parse(time.RFC3339, body.EffectiveTo)
		if parseErr != nil {
			respond.Error(c, apperrors.BadRequest(
				"effectiveTo must be an RFC 3339 timestamp.", nil))
			return
		}
		input.EffectiveTo = &parsed
	}
	contract, err := h.service.CreateContract(c.Request.Context(), t, input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewContractView(contract))
}

func (h *Handler) listContracts(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsRead) {
		return
	}
	if err := allowedQuery(c, "status", "customerId", "branchId", "createdAfter",
		"createdBefore"); err != nil {
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
	contracts, err := h.service.ListContracts(c.Request.Context(), t,
		repositories.ContractFilter{Status: c.Query("status"),
			CustomerID: c.Query("customerId"), BranchID: c.Query("branchId"),
			CreatedAfter: createdAfter, CreatedBefore: createdBefore,
			Cursor: page, HasCursor: hasCursor, Limit: limit})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]ContractView, 0, len(contracts))
	for _, contract := range contracts {
		views = append(views, NewContractView(contract))
	}
	token := ""
	if len(contracts) > 0 {
		last := contracts[len(contracts)-1]
		token = nextCursor(len(contracts), limit, last.CreatedAt, last.ID)
	}
	respond.List(c, views, token)
}

func (h *Handler) getContract(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsRead) {
		return
	}
	contract, err := h.service.GetContract(c.Request.Context(), t, c.Param("contractId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	if h.mayReadSensitive(c, t) {
		respond.OK(c, NewSensitiveContractView(contract))
		return
	}
	respond.OK(c, NewContractView(contract))
}

type updateContractRequest struct {
	EffectiveFrom      string         `json:"effectiveFrom"`
	EffectiveTo        string         `json:"effectiveTo"`
	RenewalTerms       models.JSONMap `json:"renewalTerms"`
	ServiceScope       models.JSONMap `json:"serviceScope"`
	CommercialSnapshot models.JSONMap `json:"commercialSnapshot"`
	TerminationTerms   models.JSONMap `json:"terminationTerms"`
	QuotationID        *string        `json:"quotationId"`
	ExpectedVersion    int64          `json:"expectedVersion"`
}

func (h *Handler) updateContract(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsManage) {
		return
	}
	var body updateContractRequest
	if !bindJSON(c, &body) {
		return
	}
	input := usecases.UpdateContractInput{
		RenewalTerms: body.RenewalTerms, ServiceScope: body.ServiceScope,
		CommercialSnapshot: body.CommercialSnapshot,
		TerminationTerms:   body.TerminationTerms, QuotationID: body.QuotationID,
		ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
		CorrelationID:   correlationID(c),
	}
	if body.EffectiveFrom != "" {
		parsed, err := time.Parse(time.RFC3339, body.EffectiveFrom)
		if err != nil {
			respond.Error(c, apperrors.BadRequest(
				"effectiveFrom must be an RFC 3339 timestamp.", nil))
			return
		}
		input.EffectiveFrom = &parsed
	}
	if body.EffectiveTo != "" {
		parsed, err := time.Parse(time.RFC3339, body.EffectiveTo)
		if err != nil {
			respond.Error(c, apperrors.BadRequest(
				"effectiveTo must be an RFC 3339 timestamp.", nil))
			return
		}
		input.EffectiveTo = &parsed
	}
	contract, err := h.service.UpdateContract(c.Request.Context(), t, c.Param("contractId"),
		input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewContractView(contract))
}

func (h *Handler) submitContract(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsManage) {
		return
	}
	var body reasonedRequest
	if !bindJSON(c, &body) {
		return
	}
	contract, err := h.service.SubmitContract(c.Request.Context(), t, c.Param("contractId"),
		expectedVersion(c, body.ExpectedVersion), idempotencyKey(c), correlationID(c), "")
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewContractView(contract))
}

type activateContractRequest struct {
	SignatoryReferences []string `json:"signatoryReferences"`
	ExpectedVersion     int64    `json:"expectedVersion"`
}

type activateContractResponse struct {
	Contract      ContractView        `json:"contract"`
	ActiveVersion ContractVersionView `json:"activeVersion"`
}

func (h *Handler) activateContract(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsActivate) {
		return
	}
	var body activateContractRequest
	if !bindJSON(c, &body) {
		return
	}
	contract, version, err := h.service.ActivateContract(c.Request.Context(), t,
		c.Param("contractId"), usecases.ActivateContractInput{
			SignatoryReferences: body.SignatoryReferences,
			ExpectedVersion:     expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:      idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, activateContractResponse{
		Contract:      NewContractView(contract),
		ActiveVersion: NewContractVersionView(version),
	})
}

type contractLifecycleRequest struct {
	ReasonCode                string `json:"reasonCode"`
	EffectiveAt               string `json:"effectiveAt"`
	EvidenceDocumentReference string `json:"evidenceDocumentReference"`
	ExpectedVersion           int64  `json:"expectedVersion"`
}

func (h *Handler) contractLifecycleInput(c *gin.Context,
	body contractLifecycleRequest) (usecases.ContractLifecycleInput, bool) {
	input := usecases.ContractLifecycleInput{
		ReasonCode:                body.ReasonCode,
		EvidenceDocumentReference: body.EvidenceDocumentReference,
		ExpectedVersion:           expectedVersion(c, body.ExpectedVersion),
		IdempotencyKey:            idempotencyKey(c), CorrelationID: correlationID(c),
	}
	if body.EffectiveAt != "" {
		parsed, err := time.Parse(time.RFC3339, body.EffectiveAt)
		if err != nil {
			respond.Error(c, apperrors.BadRequest(
				"effectiveAt must be an RFC 3339 timestamp.", nil))
			return input, false
		}
		input.EffectiveAt = &parsed
	}
	return input, true
}

func (h *Handler) suspendContract(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsManage) {
		return
	}
	var body contractLifecycleRequest
	if !bindJSON(c, &body) {
		return
	}
	input, ok := h.contractLifecycleInput(c, body)
	if !ok {
		return
	}
	contract, err := h.service.SuspendContract(c.Request.Context(), t, c.Param("contractId"),
		input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewContractView(contract))
}

func (h *Handler) resumeContract(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsActivate) {
		return
	}
	var body contractLifecycleRequest
	if !bindJSON(c, &body) {
		return
	}
	input, ok := h.contractLifecycleInput(c, body)
	if !ok {
		return
	}
	contract, err := h.service.ResumeContract(c.Request.Context(), t, c.Param("contractId"),
		input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewContractView(contract))
}

func (h *Handler) terminateContract(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsTerminate) {
		return
	}
	var body contractLifecycleRequest
	if !bindJSON(c, &body) {
		return
	}
	input, ok := h.contractLifecycleInput(c, body)
	if !ok {
		return
	}
	contract, err := h.service.TerminateContract(c.Request.Context(), t,
		c.Param("contractId"), input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewContractView(contract))
}

type renewContractRequest struct {
	EffectiveFrom      string         `json:"effectiveFrom"`
	EffectiveTo        string         `json:"effectiveTo"`
	RenewalTerms       models.JSONMap `json:"renewalTerms"`
	CommercialSnapshot models.JSONMap `json:"commercialSnapshot"`
	ExpectedVersion    int64          `json:"expectedVersion"`
}

func (h *Handler) renewContract(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsManage) {
		return
	}
	var body renewContractRequest
	if !bindJSON(c, &body) {
		return
	}
	effectiveFrom, err := time.Parse(time.RFC3339, body.EffectiveFrom)
	if err != nil {
		respond.Error(c, apperrors.BadRequest(
			"effectiveFrom must be an RFC 3339 timestamp.", nil))
		return
	}
	input := usecases.RenewContractInput{
		EffectiveFrom: effectiveFrom, RenewalTerms: body.RenewalTerms,
		CommercialSnapshot: body.CommercialSnapshot,
		ExpectedVersion:    expectedVersion(c, body.ExpectedVersion),
		IdempotencyKey:     idempotencyKey(c), CorrelationID: correlationID(c),
	}
	if body.EffectiveTo != "" {
		parsed, parseErr := time.Parse(time.RFC3339, body.EffectiveTo)
		if parseErr != nil {
			respond.Error(c, apperrors.BadRequest(
				"effectiveTo must be an RFC 3339 timestamp.", nil))
			return
		}
		input.EffectiveTo = &parsed
	}
	renewal, err := h.service.RenewContract(c.Request.Context(), t, c.Param("contractId"),
		input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewContractView(renewal))
}

func (h *Handler) contractVersions(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsRead) {
		return
	}
	_, _, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	versions, err := h.service.ContractVersions(c.Request.Context(), t, c.Param("contractId"),
		limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]ContractVersionView, 0, len(versions))
	for _, version := range versions {
		views = append(views, NewContractVersionView(version))
	}
	respond.List(c, views, "")
}

func (h *Handler) contractVersion(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsRead) {
		return
	}
	version, err := h.service.ContractVersion(c.Request.Context(), t, c.Param("contractId"),
		c.Param("versionId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewContractVersionView(version))
}

func (h *Handler) contractHistory(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsRead) {
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	entries, err := h.service.ContractHistory(c.Request.Context(), t, c.Param("contractId"),
		page, hasCursor, limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]ContractHistoryView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, NewContractHistoryView(entry))
	}
	token := ""
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		token = nextCursor(len(entries), limit, last.OccurredAt, last.ID)
	}
	respond.List(c, views, token)
}

func (h *Handler) attachContractDocument(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsManage) {
		return
	}
	var body attachDocumentRequest
	if !bindJSON(c, &body) {
		return
	}
	document, err := h.service.AttachContractDocument(c.Request.Context(), t,
		c.Param("contractId"), usecases.AttachContractDocumentInput{
			MediaObjectID: body.MediaObjectID, DocumentPurpose: body.DocumentPurpose,
			IsMandatory: body.IsMandatory, CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewDocumentView(document))
}

func (h *Handler) contractDocuments(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsRead) {
		return
	}
	documents, err := h.service.ContractDocuments(c.Request.Context(), t,
		c.Param("contractId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]DocumentView, 0, len(documents))
	for _, document := range documents {
		views = append(views, NewDocumentView(document))
	}
	respond.List(c, views, "")
}

func (h *Handler) replaceContractDocument(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsManage) {
		return
	}
	var body attachDocumentRequest
	if !bindJSON(c, &body) {
		return
	}
	document, err := h.service.ReplaceContractDocument(c.Request.Context(), t,
		c.Param("contractId"), c.Param("documentId"),
		usecases.AttachContractDocumentInput{
			MediaObjectID: body.MediaObjectID, DocumentPurpose: body.DocumentPurpose,
			IsMandatory: body.IsMandatory, CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewDocumentView(document))
}

func (h *Handler) removeContractDocument(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContractsManage) {
		return
	}
	var body reasonedRequest
	if !bindJSON(c, &body) {
		return
	}
	if err := h.service.RemoveContractDocument(c.Request.Context(), t, c.Param("contractId"),
		c.Param("documentId"), body.ReasonCode); err != nil {
		respond.Error(c, err)
		return
	}
	// The Media object itself is untouched. Physical deletion belongs to Media's
	// retention workflow, which is the only place that knows whether a file may
	// lawfully be destroyed.
	respond.OK(c, gin.H{"removed": true, "mediaObjectRetained": true})
}

// ---------------------------------------------------------------------------
// Service level agreements
// ---------------------------------------------------------------------------

type createSLARequest struct {
	CustomerID                 string         `json:"customerId"`
	ContractID                 string         `json:"contractId"`
	FutureServiceCatalogItemID string         `json:"futureServiceCatalogItemId"`
	MetricCode                 string         `json:"metricCode"`
	TargetValue                string         `json:"targetValue"`
	MeasurementUnit            string         `json:"measurementUnit"`
	MeasurementWindow          string         `json:"measurementWindow"`
	EscalationPolicy           models.JSONMap `json:"escalationPolicy"`
	EffectiveFrom              string         `json:"effectiveFrom"`
	EffectiveTo                string         `json:"effectiveTo"`
}

func (h *Handler) createSLA(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSLAManage) {
		return
	}
	var body createSLARequest
	if !bindJSON(c, &body) {
		return
	}
	effectiveFrom, err := time.Parse(time.RFC3339, body.EffectiveFrom)
	if err != nil {
		respond.Error(c, apperrors.BadRequest(
			"effectiveFrom must be an RFC 3339 timestamp.", nil))
		return
	}
	input := usecases.CreateSLAInput{
		CustomerID: body.CustomerID, ContractID: body.ContractID,
		FutureServiceCatalogItemID: body.FutureServiceCatalogItemID,
		MetricCode:                 body.MetricCode, TargetValue: body.TargetValue,
		MeasurementUnit: body.MeasurementUnit, MeasurementWindow: body.MeasurementWindow,
		EscalationPolicy: body.EscalationPolicy, EffectiveFrom: effectiveFrom,
		IdempotencyKey: idempotencyKey(c), CorrelationID: correlationID(c),
	}
	if body.EffectiveTo != "" {
		parsed, parseErr := time.Parse(time.RFC3339, body.EffectiveTo)
		if parseErr != nil {
			respond.Error(c, apperrors.BadRequest(
				"effectiveTo must be an RFC 3339 timestamp.", nil))
			return
		}
		input.EffectiveTo = &parsed
	}
	sla, err := h.service.CreateSLA(c.Request.Context(), t, input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewSLAView(sla))
}

func (h *Handler) listSLAs(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSLARead) {
		return
	}
	if err := allowedQuery(c, "status", "customerId", "contractId", "metricCode"); err != nil {
		respond.Error(c, err)
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	slas, err := h.service.ListSLAs(c.Request.Context(), t, repositories.SLAFilter{
		Status: c.Query("status"), CustomerID: c.Query("customerId"),
		ContractID: c.Query("contractId"), MetricCode: c.Query("metricCode"),
		Cursor: page, HasCursor: hasCursor, Limit: limit})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]SLAView, 0, len(slas))
	for _, sla := range slas {
		views = append(views, NewSLAView(sla))
	}
	token := ""
	if len(slas) > 0 {
		last := slas[len(slas)-1]
		token = nextCursor(len(slas), limit, last.CreatedAt, last.ID)
	}
	respond.List(c, views, token)
}

func (h *Handler) getSLA(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSLARead) {
		return
	}
	sla, err := h.service.GetSLA(c.Request.Context(), t, c.Param("slaId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewSLAView(sla))
}

type updateSLARequest struct {
	TargetValue       *string        `json:"targetValue"`
	MeasurementUnit   *string        `json:"measurementUnit"`
	MeasurementWindow *string        `json:"measurementWindow"`
	EscalationPolicy  models.JSONMap `json:"escalationPolicy"`
	EffectiveFrom     string         `json:"effectiveFrom"`
	EffectiveTo       string         `json:"effectiveTo"`
	ExpectedVersion   int64          `json:"expectedVersion"`
}

func (h *Handler) updateSLA(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSLAManage) {
		return
	}
	var body updateSLARequest
	if !bindJSON(c, &body) {
		return
	}
	input := usecases.UpdateSLAInput{
		TargetValue: body.TargetValue, MeasurementUnit: body.MeasurementUnit,
		MeasurementWindow: body.MeasurementWindow, EscalationPolicy: body.EscalationPolicy,
		ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
		CorrelationID:   correlationID(c),
	}
	if body.EffectiveFrom != "" {
		parsed, err := time.Parse(time.RFC3339, body.EffectiveFrom)
		if err != nil {
			respond.Error(c, apperrors.BadRequest(
				"effectiveFrom must be an RFC 3339 timestamp.", nil))
			return
		}
		input.EffectiveFrom = &parsed
	}
	if body.EffectiveTo != "" {
		parsed, err := time.Parse(time.RFC3339, body.EffectiveTo)
		if err != nil {
			respond.Error(c, apperrors.BadRequest(
				"effectiveTo must be an RFC 3339 timestamp.", nil))
			return
		}
		input.EffectiveTo = &parsed
	}
	sla, err := h.service.UpdateSLA(c.Request.Context(), t, c.Param("slaId"), input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewSLAView(sla))
}

func (h *Handler) activateSLA(c *gin.Context) {
	h.transitionSLA(c, PermissionSLAActivate, "activate")
}

func (h *Handler) suspendSLA(c *gin.Context) {
	h.transitionSLA(c, PermissionSLAActivate, "suspend")
}

func (h *Handler) terminateSLA(c *gin.Context) {
	h.transitionSLA(c, PermissionSLAManage, "terminate")
}

func (h *Handler) transitionSLA(c *gin.Context, permission, action string) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, permission) {
		return
	}
	var body reasonedRequest
	if !bindJSON(c, &body) {
		return
	}
	input := usecases.SLALifecycleInput{
		ReasonCode:      body.ReasonCode,
		ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
		IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c),
	}
	var sla models.SLA
	var err error
	switch action {
	case "activate":
		sla, err = h.service.ActivateSLA(c.Request.Context(), t, c.Param("slaId"), input)
	case "suspend":
		sla, err = h.service.SuspendSLA(c.Request.Context(), t, c.Param("slaId"), input)
	default:
		sla, err = h.service.TerminateSLA(c.Request.Context(), t, c.Param("slaId"), input)
	}
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewSLAView(sla))
}

func (h *Handler) slaObservations(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSLARead) {
		return
	}
	if err := allowedQuery(c, "outcome"); err != nil {
		respond.Error(c, err)
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	observations, err := h.service.SLAObservations(c.Request.Context(), t, c.Param("slaId"),
		c.Query("outcome"), page, hasCursor, limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]ObservationView, 0, len(observations))
	for _, observation := range observations {
		views = append(views, NewObservationView(observation))
	}
	token := ""
	if len(observations) > 0 {
		last := observations[len(observations)-1]
		token = nextCursor(len(observations), limit, last.MeasuredAt, last.ID)
	}
	respond.List(c, views, token)
}

func (h *Handler) slaHistory(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSLARead) {
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	entries, err := h.service.SLAHistory(c.Request.Context(), t, c.Param("slaId"), page,
		hasCursor, limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]SLAHistoryView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, NewSLAHistoryView(entry))
	}
	token := ""
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		token = nextCursor(len(entries), limit, last.OccurredAt, last.ID)
	}
	respond.List(c, views, token)
}
