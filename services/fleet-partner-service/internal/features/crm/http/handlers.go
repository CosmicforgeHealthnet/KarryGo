package http

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the Section 14 routes. These are the exact codes seeded
// by migration 000023; nothing here invents a semantically duplicate grant.
const (
	PermissionCustomersRead      = "crm.customers.read"
	PermissionCustomersManage    = "crm.customers.manage"
	PermissionContactsRead       = "crm.contacts.read"
	PermissionContactsManage     = "crm.contacts.manage"
	PermissionLocationsRead      = "crm.locations.read"
	PermissionLocationsManage    = "crm.locations.manage"
	PermissionOpportunitiesRead  = "crm.opportunities.read"
	PermissionOpportunitiesWrite = "crm.opportunities.manage"
	PermissionQuotationsRead     = "crm.quotations.read"
	PermissionQuotationsManage   = "crm.quotations.manage"
	PermissionQuotationsSubmit   = "crm.quotations.submit"
	PermissionQuotationsIssue    = "crm.quotations.issue"
	PermissionQuotationsAccept   = "crm.quotations.accept"
	PermissionContractsRead      = "crm.contracts.read"
	PermissionContractsManage    = "crm.contracts.manage"
	PermissionContractsActivate  = "crm.contracts.activate"
	PermissionContractsTerminate = "crm.contracts.terminate"
	PermissionSLARead            = "crm.sla.read"
	PermissionSLAManage          = "crm.sla.manage"
	PermissionSLAActivate        = "crm.sla.activate"
	PermissionCreditRead         = "crm.credit.read"
	PermissionCreditManage       = "crm.credit.manage"
	PermissionSensitiveRead      = "crm.sensitive.read"
)

// Handler serves the Section 14 routes.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts the corporate CRM routes.
//
// The routes come from TenantRoutes(), the shared Section 14 catalogue, so the
// router and the OpenAPI contract test read the same list. A manifest entry
// with no handler is a programming error rather than a silently missing route,
// so it panics at registration instead of at three in the morning.
//
// The routes are mounted whenever the service exists. With the flags off they
// answer not-found rather than disappearing from the surface, so a disabled
// capability is observable instead of indistinguishable from a deployment that
// never had it.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	if h.service == nil {
		return
	}
	mount(group, h.TenantRoutes(), map[string]gin.HandlerFunc{
		"listCustomers":            h.listCustomers,
		"createCustomer":           h.createCustomer,
		"getCustomer":              h.getCustomer,
		"updateCustomer":           h.updateCustomer,
		"archiveCustomer":          h.archiveCustomer,
		"changeCustomerStatus":     h.changeCustomerStatus,
		"changeCustomerRiskStatus": h.changeCustomerRisk,
		"listCustomerHistory":      h.customerHistory,

		"listContacts":       h.listContacts,
		"createContact":      h.createContact,
		"getContact":         h.getContact,
		"updateContact":      h.updateContact,
		"archiveContact":     h.archiveContact,
		"makeContactPrimary": h.makeContactPrimary,

		"listLocations":   h.listLocations,
		"createLocation":  h.createLocation,
		"getLocation":     h.getLocation,
		"updateLocation":  h.updateLocation,
		"archiveLocation": h.archiveLocation,

		"getCreditProfile":  h.getCreditProfile,
		"setCreditProfile":  h.setCreditProfile,
		"applyCreditHold":   h.applyCreditHold,
		"releaseCreditHold": h.releaseCreditHold,
		"listCreditHistory": h.creditHistory,

		"listOpportunities":      h.listOpportunities,
		"createOpportunity":      h.createOpportunity,
		"getOpportunity":         h.getOpportunity,
		"updateOpportunity":      h.updateOpportunity,
		"markOpportunityWon":     h.markOpportunityWon,
		"markOpportunityLost":    h.markOpportunityLost,
		"holdOpportunity":        h.holdOpportunity,
		"resumeOpportunity":      h.resumeOpportunity,
		"listOpportunityHistory": h.opportunityHistory,

		"listQuotations":          h.listQuotations,
		"createQuotation":         h.createQuotation,
		"getQuotation":            h.getQuotation,
		"updateQuotation":         h.updateQuotation,
		"listQuotationHistory":    h.quotationHistory,
		"listQuotationVersions":   h.quotationVersions,
		"getQuotationVersion":     h.quotationVersion,
		"listQuotationDocuments":  h.quotationDocuments,
		"attachQuotationDocument": h.attachQuotationDocument,
		"submitQuotation":         h.submitQuotation,
		"issueQuotation":          h.issueQuotation,
		"acceptQuotationAsStaff":  h.acceptQuotation,
		"rejectQuotationAsStaff":  h.rejectQuotation,
		"cancelQuotation":         h.cancelQuotation,
		"supersedeQuotation":      h.supersedeQuotation,

		"listContracts":           h.listContracts,
		"createContract":          h.createContract,
		"getContract":             h.getContract,
		"updateContract":          h.updateContract,
		"listContractHistory":     h.contractHistory,
		"listContractVersions":    h.contractVersions,
		"getContractVersion":      h.contractVersion,
		"listContractDocuments":   h.contractDocuments,
		"attachContractDocument":  h.attachContractDocument,
		"replaceContractDocument": h.replaceContractDocument,
		"removeContractDocument":  h.removeContractDocument,
		"submitContract":          h.submitContract,
		"activateContract":        h.activateContract,
		"suspendContract":         h.suspendContract,
		"resumeContract":          h.resumeContract,
		"terminateContract":       h.terminateContract,
		"renewContract":           h.renewContract,

		"listServiceLevelAgreements":       h.listSLAs,
		"createServiceLevelAgreement":      h.createSLA,
		"getServiceLevelAgreement":         h.getSLA,
		"updateServiceLevelAgreement":      h.updateSLA,
		"activateServiceLevelAgreement":    h.activateSLA,
		"suspendServiceLevelAgreement":     h.suspendSLA,
		"terminateServiceLevelAgreement":   h.terminateSLA,
		"listServiceLevelObservations":     h.slaObservations,
		"listServiceLevelAgreementHistory": h.slaHistory,

		"listNotificationOperations":  h.listNotificationOperations,
		"replayNotificationOperation": h.replayNotification,
	})
}

// mount registers a catalogue onto a route group.
//
// It panics on a manifest entry with no handler. That is deliberate: the
// alternative is a route the specification promises and the router never
// serves, which is exactly the drift this catalogue exists to prevent, and it
// fails at process start rather than on a customer request.
func mount(group *gin.RouterGroup, routes []Route, handlers map[string]gin.HandlerFunc) {
	for _, route := range routes {
		handler, ok := handlers[route.OperationID]
		if !ok || handler == nil {
			panic("crm: no handler registered for operation " + route.OperationID)
		}
		group.Handle(route.Method, route.Path, handler)
	}
}

// ---------------------------------------------------------------------------
// Request plumbing
// ---------------------------------------------------------------------------

func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

// authorize is default-deny: a nil authorizer refuses rather than allows.
func (h *Handler) authorize(c *gin.Context, t tenant.Context, permission string) bool {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden(
			"You do not have permission to perform this action.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID,
		roleVersionFrom(c), permission,
		accesscontrol.Target{BranchID: c.Query("branchId")}); err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

// mayReadSensitive reports whether the caller holds the separate sensitive-data
// grant.
//
// It never writes an error: a caller without the grant gets the masked view,
// not a refusal. Refusing would tell them the record exists and that it has
// something worth hiding.
func (h *Handler) mayReadSensitive(c *gin.Context, t tenant.Context) bool {
	if h.authorizer == nil {
		return false
	}
	decision, err := h.authorizer.Authorize(c.Request.Context(), t.OrganizationID,
		t.OrganizationUserID, roleVersionFrom(c), PermissionSensitiveRead,
		accesscontrol.Target{BranchID: c.Query("branchId")})
	return err == nil && decision.Allowed
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}

func idempotencyKey(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("Idempotency-Key"))
}

func correlationID(c *gin.Context) string { return middleware.GetCorrelationID(c) }

// expectedVersion reads either the body field or If-Match.
func expectedVersion(c *gin.Context, bodyValue int64) int64 {
	if bodyValue != 0 {
		return bodyValue
	}
	raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`)
	if raw == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func pageParams(c *gin.Context) (cursor.Cursor, bool, int, error) {
	decoded, ok, err := cursor.Decode(strings.TrimSpace(c.Query("cursor")))
	if err != nil {
		return decoded, false, 0, apperrors.BadRequest("The cursor is not valid.", nil)
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			return decoded, false, 0, apperrors.BadRequest("limit must be an integer.", nil)
		}
		limit = parsed
	}
	return decoded, ok, cursor.ClampLimit(limit), nil
}

// allowedQuery rejects any filter the endpoint does not recognise.
//
// Silently ignoring an unknown filter is the failure mode where a caller
// believes they narrowed a list and got everything instead. An explicit refusal
// is the only honest answer.
func allowedQuery(c *gin.Context, allowed ...string) error {
	permitted := map[string]bool{"cursor": true, "limit": true, "sort": true}
	for _, name := range allowed {
		permitted[name] = true
	}
	for name := range c.Request.URL.Query() {
		if !permitted[name] {
			return apperrors.BadRequest("Unknown query parameter: "+name+".", nil)
		}
	}
	return nil
}

func parseTimeQuery(c *gin.Context, name string) (*time.Time, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, apperrors.BadRequest(name+" must be an RFC 3339 timestamp.", nil)
	}
	return &parsed, nil
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return false
	}
	return true
}

// nextCursor builds the paging token from the last row of a page.
func nextCursor(size, limit int, sortedAt time.Time, lastID string) string {
	if size < limit || size == 0 {
		return ""
	}
	return cursor.EncodeTime(sortedAt, lastID)
}

// ---------------------------------------------------------------------------
// Customers
// ---------------------------------------------------------------------------

type createCustomerRequest struct {
	BranchID              string `json:"branchId"`
	CustomerType          string `json:"customerType"`
	LegalName             string `json:"legalName"`
	DisplayName           string `json:"displayName"`
	RegistrationNumber    string `json:"registrationNumber"`
	TaxReference          string `json:"taxReference"`
	MarketplaceCustomerID string `json:"marketplaceCustomerId"`
	AccountOwnerUserID    string `json:"accountOwnerUserId"`
	DefaultCurrency       string `json:"defaultCurrency"`
	PaymentTermsReference string `json:"paymentTermsReference"`
}

func (h *Handler) createCustomer(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCustomersManage) {
		return
	}
	var body createCustomerRequest
	if !bindJSON(c, &body) {
		return
	}
	customer, err := h.service.CreateCustomer(c.Request.Context(), t,
		usecases.CreateCustomerInput{
			BranchID: body.BranchID, CustomerType: body.CustomerType,
			LegalName: body.LegalName, DisplayName: body.DisplayName,
			RegistrationNumber: body.RegistrationNumber, TaxReference: body.TaxReference,
			MarketplaceCustomerID: body.MarketplaceCustomerID,
			AccountOwnerUserID:    body.AccountOwnerUserID,
			DefaultCurrency:       body.DefaultCurrency,
			PaymentTermsReference: body.PaymentTermsReference,
			IdempotencyKey:        idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewCustomerView(customer))
}

func (h *Handler) listCustomers(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCustomersRead) {
		return
	}
	if err := allowedQuery(c, "status", "riskStatus", "customerType", "branchId",
		"accountOwnerUserId", "createdAfter", "createdBefore", "search"); err != nil {
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
	customers, err := h.service.ListCustomers(c.Request.Context(), t,
		usecases.ListCustomersInput{
			Status: c.Query("status"), RiskStatus: c.Query("riskStatus"),
			CustomerType: c.Query("customerType"), BranchID: c.Query("branchId"),
			AccountOwnerUserID: c.Query("accountOwnerUserId"),
			CreatedAfter:       createdAfter, CreatedBefore: createdBefore,
			Search: c.Query("search"), Cursor: page, HasCursor: hasCursor, Limit: limit,
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]CustomerView, 0, len(customers))
	for _, customer := range customers {
		views = append(views, NewCustomerView(customer))
	}
	token := ""
	if len(customers) > 0 {
		last := customers[len(customers)-1]
		token = nextCursor(len(customers), limit, last.CreatedAt, last.ID)
	}
	respond.List(c, views, token)
}

func (h *Handler) getCustomer(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCustomersRead) {
		return
	}
	customer, err := h.service.GetCustomer(c.Request.Context(), t, c.Param("customerId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	if h.mayReadSensitive(c, t) {
		respond.OK(c, NewSensitiveCustomerView(customer))
		return
	}
	respond.OK(c, NewCustomerView(customer))
}

type updateCustomerRequest struct {
	BranchID              *string `json:"branchId"`
	LegalName             *string `json:"legalName"`
	DisplayName           *string `json:"displayName"`
	RegistrationNumber    *string `json:"registrationNumber"`
	TaxReference          *string `json:"taxReference"`
	MarketplaceCustomerID *string `json:"marketplaceCustomerId"`
	AccountOwnerUserID    *string `json:"accountOwnerUserId"`
	DefaultCurrency       *string `json:"defaultCurrency"`
	PaymentTermsReference *string `json:"paymentTermsReference"`
	ExpectedVersion       int64   `json:"expectedVersion"`
}

func (h *Handler) updateCustomer(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCustomersManage) {
		return
	}
	var body updateCustomerRequest
	if !bindJSON(c, &body) {
		return
	}
	customer, err := h.service.UpdateCustomer(c.Request.Context(), t, c.Param("customerId"),
		usecases.UpdateCustomerInput{
			BranchID: body.BranchID, LegalName: body.LegalName,
			DisplayName: body.DisplayName, RegistrationNumber: body.RegistrationNumber,
			TaxReference:          body.TaxReference,
			MarketplaceCustomerID: body.MarketplaceCustomerID,
			AccountOwnerUserID:    body.AccountOwnerUserID,
			DefaultCurrency:       body.DefaultCurrency,
			PaymentTermsReference: body.PaymentTermsReference,
			ExpectedVersion:       expectedVersion(c, body.ExpectedVersion),
			CorrelationID:         correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewCustomerView(customer))
}

type statusChangeRequest struct {
	Status          string `json:"status"`
	ReasonCode      string `json:"reasonCode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) changeCustomerStatus(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCustomersManage) {
		return
	}
	var body statusChangeRequest
	if !bindJSON(c, &body) {
		return
	}
	customer, err := h.service.ChangeCustomerStatus(c.Request.Context(), t,
		c.Param("customerId"), usecases.ChangeCustomerStatusInput{
			Status: body.Status, ReasonCode: body.ReasonCode,
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewCustomerView(customer))
}

type reasonedRequest struct {
	ReasonCode      string `json:"reasonCode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) archiveCustomer(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCustomersManage) {
		return
	}
	var body reasonedRequest
	if !bindJSON(c, &body) {
		return
	}
	customer, err := h.service.ArchiveCustomer(c.Request.Context(), t, c.Param("customerId"),
		expectedVersion(c, body.ExpectedVersion), body.ReasonCode, idempotencyKey(c),
		correlationID(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewCustomerView(customer))
}

type riskChangeRequest struct {
	RiskStatus      string `json:"riskStatus"`
	ReasonCode      string `json:"reasonCode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) changeCustomerRisk(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCustomersManage) {
		return
	}
	var body riskChangeRequest
	if !bindJSON(c, &body) {
		return
	}
	customer, err := h.service.ChangeCustomerRisk(c.Request.Context(), t,
		c.Param("customerId"), usecases.ChangeCustomerRiskInput{
			RiskStatus: body.RiskStatus, ReasonCode: body.ReasonCode,
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewCustomerView(customer))
}

func (h *Handler) customerHistory(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCustomersRead) {
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	entries, err := h.service.CustomerHistory(c.Request.Context(), t, c.Param("customerId"),
		page, hasCursor, limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]CustomerHistoryView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, NewCustomerHistoryView(entry))
	}
	token := ""
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		token = nextCursor(len(entries), limit, last.OccurredAt, last.ID)
	}
	respond.List(c, views, token)
}

// ---------------------------------------------------------------------------
// Contacts
// ---------------------------------------------------------------------------

type createContactRequest struct {
	FullName                string         `json:"fullName"`
	RoleTitle               string         `json:"roleTitle"`
	Email                   string         `json:"email"`
	Phone                   string         `json:"phone"`
	PreferredChannel        string         `json:"preferredChannel"`
	ContactPurpose          string         `json:"contactPurpose"`
	IsPrimary               bool           `json:"isPrimary"`
	NotificationPreferences models.JSONMap `json:"notificationPreferences"`
	ConsentStatus           string         `json:"consentStatus"`
	ConsentSource           string         `json:"consentSource"`
}

func (h *Handler) createContact(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContactsManage) {
		return
	}
	var body createContactRequest
	if !bindJSON(c, &body) {
		return
	}
	contact, err := h.service.CreateContact(c.Request.Context(), t, c.Param("customerId"),
		usecases.CreateContactInput{
			FullName: body.FullName, RoleTitle: body.RoleTitle, Email: body.Email,
			Phone: body.Phone, PreferredChannel: body.PreferredChannel,
			ContactPurpose: body.ContactPurpose, IsPrimary: body.IsPrimary,
			NotificationPreferences: body.NotificationPreferences,
			ConsentStatus:           body.ConsentStatus, ConsentSource: body.ConsentSource,
			IdempotencyKey: idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewContactView(contact))
}

func (h *Handler) listContacts(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContactsRead) {
		return
	}
	if err := allowedQuery(c, "status", "contactPurpose"); err != nil {
		respond.Error(c, err)
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	contacts, err := h.service.ListContacts(c.Request.Context(), t, c.Param("customerId"),
		repositories.ContactFilter{Status: c.Query("status"),
			ContactPurpose: c.Query("contactPurpose"), Cursor: page, HasCursor: hasCursor,
			Limit: limit})
	if err != nil {
		respond.Error(c, err)
		return
	}
	sensitive := h.mayReadSensitive(c, t)
	if sensitive {
		views := make([]SensitiveContactView, 0, len(contacts))
		for _, contact := range contacts {
			views = append(views, NewSensitiveContactView(contact))
		}
		respond.List(c, views, contactCursor(contacts, limit))
		return
	}
	views := make([]ContactView, 0, len(contacts))
	for _, contact := range contacts {
		views = append(views, NewContactView(contact))
	}
	respond.List(c, views, contactCursor(contacts, limit))
}

func contactCursor(contacts []models.Contact, limit int) string {
	if len(contacts) == 0 {
		return ""
	}
	last := contacts[len(contacts)-1]
	return nextCursor(len(contacts), limit, last.CreatedAt, last.ID)
}

func (h *Handler) getContact(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContactsRead) {
		return
	}
	contact, err := h.service.GetContact(c.Request.Context(), t, c.Param("customerId"),
		c.Param("contactId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	if h.mayReadSensitive(c, t) {
		respond.OK(c, NewSensitiveContactView(contact))
		return
	}
	respond.OK(c, NewContactView(contact))
}

type updateContactRequest struct {
	FullName                *string        `json:"fullName"`
	RoleTitle               *string        `json:"roleTitle"`
	Email                   *string        `json:"email"`
	Phone                   *string        `json:"phone"`
	PreferredChannel        *string        `json:"preferredChannel"`
	ContactPurpose          *string        `json:"contactPurpose"`
	NotificationPreferences models.JSONMap `json:"notificationPreferences"`
	ConsentStatus           *string        `json:"consentStatus"`
	ConsentSource           *string        `json:"consentSource"`
	Status                  *string        `json:"status"`
	ExpectedVersion         int64          `json:"expectedVersion"`
}

func (h *Handler) updateContact(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContactsManage) {
		return
	}
	var body updateContactRequest
	if !bindJSON(c, &body) {
		return
	}
	contact, err := h.service.UpdateContact(c.Request.Context(), t, c.Param("customerId"),
		c.Param("contactId"), usecases.UpdateContactInput{
			FullName: body.FullName, RoleTitle: body.RoleTitle, Email: body.Email,
			Phone: body.Phone, PreferredChannel: body.PreferredChannel,
			ContactPurpose:          body.ContactPurpose,
			NotificationPreferences: body.NotificationPreferences,
			ConsentStatus:           body.ConsentStatus, ConsentSource: body.ConsentSource,
			Status:          body.Status,
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
			CorrelationID:   correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewContactView(contact))
}

func (h *Handler) archiveContact(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContactsManage) {
		return
	}
	var body reasonedRequest
	if !bindJSON(c, &body) {
		return
	}
	contact, err := h.service.ArchiveContact(c.Request.Context(), t, c.Param("customerId"),
		c.Param("contactId"), expectedVersion(c, body.ExpectedVersion), body.ReasonCode,
		idempotencyKey(c), correlationID(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewContactView(contact))
}

func (h *Handler) makeContactPrimary(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionContactsManage) {
		return
	}
	var body reasonedRequest
	if !bindJSON(c, &body) {
		return
	}
	contact, err := h.service.MakeContactPrimary(c.Request.Context(), t,
		c.Param("customerId"), c.Param("contactId"),
		expectedVersion(c, body.ExpectedVersion), idempotencyKey(c), correlationID(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewContactView(contact))
}

// ---------------------------------------------------------------------------
// Locations
// ---------------------------------------------------------------------------

type addressRequest struct {
	Line1         string `json:"line1"`
	Line2         string `json:"line2"`
	City          string `json:"city"`
	StateOrRegion string `json:"stateOrRegion"`
	PostalCode    string `json:"postalCode"`
	CountryCode   string `json:"countryCode"`
}

func (a addressRequest) toModel() models.Address {
	return models.Address{Line1: a.Line1, Line2: a.Line2, City: a.City,
		StateOrRegion: a.StateOrRegion, PostalCode: a.PostalCode,
		CountryCode: strings.ToUpper(strings.TrimSpace(a.CountryCode))}
}

type createLocationRequest struct {
	LocationType       string         `json:"locationType"`
	Label              string         `json:"label"`
	Address            addressRequest `json:"address"`
	Latitude           *float64       `json:"latitude"`
	Longitude          *float64       `json:"longitude"`
	GeofenceReference  string         `json:"geofenceReference"`
	OperatingHours     models.JSONMap `json:"operatingHours"`
	AccessInstructions string         `json:"accessInstructions"`
	SafetyInstructions string         `json:"safetyInstructions"`
	ContactReference   string         `json:"contactReference"`
	Timezone           string         `json:"timezone"`
}

func (h *Handler) createLocation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionLocationsManage) {
		return
	}
	var body createLocationRequest
	if !bindJSON(c, &body) {
		return
	}
	location, err := h.service.CreateLocation(c.Request.Context(), t, c.Param("customerId"),
		usecases.CreateLocationInput{
			LocationType: body.LocationType, Label: body.Label,
			Address: body.Address.toModel(), Latitude: body.Latitude,
			Longitude: body.Longitude, GeofenceReference: body.GeofenceReference,
			OperatingHours:     body.OperatingHours,
			AccessInstructions: body.AccessInstructions,
			SafetyInstructions: body.SafetyInstructions,
			ContactReference:   body.ContactReference, Timezone: body.Timezone,
			IdempotencyKey: idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewLocationView(location))
}

func (h *Handler) listLocations(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionLocationsRead) {
		return
	}
	if err := allowedQuery(c, "locationType", "status"); err != nil {
		respond.Error(c, err)
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	locations, err := h.service.ListLocations(c.Request.Context(), t, c.Param("customerId"),
		repositories.LocationFilter{LocationType: c.Query("locationType"),
			Status: c.Query("status"), Cursor: page, HasCursor: hasCursor, Limit: limit})
	if err != nil {
		respond.Error(c, err)
		return
	}
	token := ""
	if len(locations) > 0 {
		last := locations[len(locations)-1]
		token = nextCursor(len(locations), limit, last.CreatedAt, last.ID)
	}
	if h.mayReadSensitive(c, t) {
		views := make([]SensitiveLocationView, 0, len(locations))
		for _, location := range locations {
			views = append(views, NewSensitiveLocationView(location))
		}
		respond.List(c, views, token)
		return
	}
	views := make([]LocationView, 0, len(locations))
	for _, location := range locations {
		views = append(views, NewLocationView(location))
	}
	respond.List(c, views, token)
}

func (h *Handler) getLocation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionLocationsRead) {
		return
	}
	location, err := h.service.GetLocation(c.Request.Context(), t, c.Param("customerId"),
		c.Param("locationId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	if h.mayReadSensitive(c, t) {
		respond.OK(c, NewSensitiveLocationView(location))
		return
	}
	respond.OK(c, NewLocationView(location))
}

type updateLocationRequest struct {
	LocationType       *string         `json:"locationType"`
	Label              *string         `json:"label"`
	Address            *addressRequest `json:"address"`
	Latitude           *float64        `json:"latitude"`
	Longitude          *float64        `json:"longitude"`
	ClearCoordinates   bool            `json:"clearCoordinates"`
	GeofenceReference  *string         `json:"geofenceReference"`
	OperatingHours     models.JSONMap  `json:"operatingHours"`
	AccessInstructions *string         `json:"accessInstructions"`
	SafetyInstructions *string         `json:"safetyInstructions"`
	ContactReference   *string         `json:"contactReference"`
	Timezone           *string         `json:"timezone"`
	Status             *string         `json:"status"`
	ExpectedVersion    int64           `json:"expectedVersion"`
}

func (h *Handler) updateLocation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionLocationsManage) {
		return
	}
	var body updateLocationRequest
	if !bindJSON(c, &body) {
		return
	}
	input := usecases.UpdateLocationInput{
		LocationType: body.LocationType, Label: body.Label, Latitude: body.Latitude,
		Longitude: body.Longitude, ClearCoordinates: body.ClearCoordinates,
		GeofenceReference: body.GeofenceReference, OperatingHours: body.OperatingHours,
		AccessInstructions: body.AccessInstructions,
		SafetyInstructions: body.SafetyInstructions,
		ContactReference:   body.ContactReference, Timezone: body.Timezone,
		Status:          body.Status,
		ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
		CorrelationID:   correlationID(c),
	}
	if body.Address != nil {
		address := body.Address.toModel()
		input.Address = &address
	}
	location, err := h.service.UpdateLocation(c.Request.Context(), t, c.Param("customerId"),
		c.Param("locationId"), input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewLocationView(location))
}

func (h *Handler) archiveLocation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionLocationsManage) {
		return
	}
	var body reasonedRequest
	if !bindJSON(c, &body) {
		return
	}
	location, err := h.service.ArchiveLocation(c.Request.Context(), t, c.Param("customerId"),
		c.Param("locationId"), expectedVersion(c, body.ExpectedVersion), body.ReasonCode,
		idempotencyKey(c), correlationID(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewLocationView(location))
}

// ---------------------------------------------------------------------------
// Opportunities
// ---------------------------------------------------------------------------

type createOpportunityRequest struct {
	BranchID               string     `json:"branchId"`
	CustomerID             string     `json:"customerId"`
	Title                  string     `json:"title"`
	Description            string     `json:"description"`
	ExpectedValue          *MoneyView `json:"expectedValue"`
	ProbabilityBasisPoints int        `json:"probabilityBasisPoints"`
	ExpectedCloseDate      string     `json:"expectedCloseDate"`
	ServiceCapabilities    []string   `json:"serviceCapabilities"`
	AccountOwnerUserID     string     `json:"accountOwnerUserId"`
	NextAction             string     `json:"nextAction"`
	NextActionAt           string     `json:"nextActionAt"`
}

func (h *Handler) createOpportunity(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOpportunitiesWrite) {
		return
	}
	var body createOpportunityRequest
	if !bindJSON(c, &body) {
		return
	}
	input := usecases.CreateOpportunityInput{
		BranchID: body.BranchID, CustomerID: body.CustomerID, Title: body.Title,
		Description:            body.Description,
		ProbabilityBasisPoints: body.ProbabilityBasisPoints,
		ServiceCapabilities:    body.ServiceCapabilities,
		AccountOwnerUserID:     body.AccountOwnerUserID, NextAction: body.NextAction,
		IdempotencyKey: idempotencyKey(c), CorrelationID: correlationID(c),
	}
	if body.ExpectedValue != nil {
		amount := body.ExpectedValue.AmountMinor
		input.ExpectedValueAmountMinor = &amount
		input.ExpectedValueCurrency = body.ExpectedValue.Currency
	}
	if body.ExpectedCloseDate != "" {
		parsed, err := time.Parse("2006-01-02", body.ExpectedCloseDate)
		if err != nil {
			respond.Error(c, apperrors.BadRequest(
				"expectedCloseDate must be a YYYY-MM-DD date.", nil))
			return
		}
		input.ExpectedCloseDate = &parsed
	}
	if body.NextActionAt != "" {
		parsed, err := time.Parse(time.RFC3339, body.NextActionAt)
		if err != nil {
			respond.Error(c, apperrors.BadRequest(
				"nextActionAt must be an RFC 3339 timestamp.", nil))
			return
		}
		input.NextActionAt = &parsed
	}
	opportunity, err := h.service.CreateOpportunity(c.Request.Context(), t, input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewOpportunityView(opportunity))
}

func (h *Handler) listOpportunities(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOpportunitiesRead) {
		return
	}
	if err := allowedQuery(c, "stage", "customerId", "branchId", "accountOwnerUserId",
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
	opportunities, err := h.service.ListOpportunities(c.Request.Context(), t,
		repositories.OpportunityFilter{Stage: c.Query("stage"),
			CustomerID: c.Query("customerId"), BranchID: c.Query("branchId"),
			AccountOwnerUserID: c.Query("accountOwnerUserId"),
			CreatedAfter:       createdAfter, CreatedBefore: createdBefore,
			Cursor: page, HasCursor: hasCursor, Limit: limit})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]OpportunityView, 0, len(opportunities))
	for _, opportunity := range opportunities {
		views = append(views, NewOpportunityView(opportunity))
	}
	token := ""
	if len(opportunities) > 0 {
		last := opportunities[len(opportunities)-1]
		token = nextCursor(len(opportunities), limit, last.CreatedAt, last.ID)
	}
	respond.List(c, views, token)
}

func (h *Handler) getOpportunity(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOpportunitiesRead) {
		return
	}
	opportunity, err := h.service.GetOpportunity(c.Request.Context(), t,
		c.Param("opportunityId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewOpportunityView(opportunity))
}

type updateOpportunityRequest struct {
	Title                  *string    `json:"title"`
	Description            *string    `json:"description"`
	ExpectedValue          *MoneyView `json:"expectedValue"`
	ClearExpectedValue     bool       `json:"clearExpectedValue"`
	ProbabilityBasisPoints *int       `json:"probabilityBasisPoints"`
	ServiceCapabilities    []string   `json:"serviceCapabilities"`
	AccountOwnerUserID     *string    `json:"accountOwnerUserId"`
	NextAction             *string    `json:"nextAction"`
	ExpectedVersion        int64      `json:"expectedVersion"`
}

func (h *Handler) updateOpportunity(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOpportunitiesWrite) {
		return
	}
	var body updateOpportunityRequest
	if !bindJSON(c, &body) {
		return
	}
	input := usecases.UpdateOpportunityInput{
		Title: body.Title, Description: body.Description,
		ClearExpectedValue:     body.ClearExpectedValue,
		ProbabilityBasisPoints: body.ProbabilityBasisPoints,
		ServiceCapabilities:    body.ServiceCapabilities,
		AccountOwnerUserID:     body.AccountOwnerUserID, NextAction: body.NextAction,
		ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
		CorrelationID:   correlationID(c),
	}
	if body.ExpectedValue != nil {
		amount := body.ExpectedValue.AmountMinor
		currency := body.ExpectedValue.Currency
		input.ExpectedValueAmountMinor = &amount
		input.ExpectedValueCurrency = &currency
	}
	opportunity, err := h.service.UpdateOpportunity(c.Request.Context(), t,
		c.Param("opportunityId"), input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewOpportunityView(opportunity))
}

func (h *Handler) markOpportunityWon(c *gin.Context) {
	h.transitionOpportunity(c, models.StageWon)
}

func (h *Handler) markOpportunityLost(c *gin.Context) {
	h.transitionOpportunity(c, models.StageLost)
}

func (h *Handler) holdOpportunity(c *gin.Context) {
	h.transitionOpportunity(c, models.StageOnHold)
}

type resumeOpportunityRequest struct {
	Stage           string `json:"stage"`
	ReasonCode      string `json:"reasonCode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

// resumeOpportunity returns a held opportunity to a stage it previously
// occupied. The target stage is explicit rather than inferred, and the service
// verifies it against the immutable history.
func (h *Handler) resumeOpportunity(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOpportunitiesWrite) {
		return
	}
	var body resumeOpportunityRequest
	if !bindJSON(c, &body) {
		return
	}
	opportunity, err := h.service.TransitionOpportunity(c.Request.Context(), t,
		c.Param("opportunityId"), usecases.TransitionOpportunityInput{
			Stage: body.Stage, ReasonCode: body.ReasonCode,
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewOpportunityView(opportunity))
}

func (h *Handler) transitionOpportunity(c *gin.Context, stage string) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOpportunitiesWrite) {
		return
	}
	var body reasonedRequest
	if !bindJSON(c, &body) {
		return
	}
	opportunity, err := h.service.TransitionOpportunity(c.Request.Context(), t,
		c.Param("opportunityId"), usecases.TransitionOpportunityInput{
			Stage: stage, ReasonCode: body.ReasonCode,
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewOpportunityView(opportunity))
}

func (h *Handler) opportunityHistory(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOpportunitiesRead) {
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	entries, err := h.service.OpportunityHistory(c.Request.Context(), t,
		c.Param("opportunityId"), page, hasCursor, limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]OpportunityHistoryView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, NewOpportunityHistoryView(entry))
	}
	token := ""
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		token = nextCursor(len(entries), limit, last.OccurredAt, last.ID)
	}
	respond.List(c, views, token)
}

// ---------------------------------------------------------------------------
// Credit policy
// ---------------------------------------------------------------------------

type setCreditProfileRequest struct {
	CreditLimit      *MoneyView `json:"creditLimit"`
	Currency         string     `json:"currency"`
	PaymentTermsDays int        `json:"paymentTermsDays"`
	ReasonCode       string     `json:"reasonCode"`
	ExpectedVersion  int64      `json:"expectedVersion"`
}

func (h *Handler) setCreditProfile(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCreditManage) {
		return
	}
	var body setCreditProfileRequest
	if !bindJSON(c, &body) {
		return
	}
	input := usecases.SetCreditProfileInput{
		Currency: body.Currency, PaymentTermsDays: body.PaymentTermsDays,
		ReasonCode:      body.ReasonCode,
		ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
		IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c),
	}
	if body.CreditLimit != nil {
		amount := body.CreditLimit.AmountMinor
		input.CreditLimitAmountMinor = &amount
		if input.Currency == "" {
			input.Currency = body.CreditLimit.Currency
		}
	}
	profile, err := h.service.SetCreditProfile(c.Request.Context(), t, c.Param("customerId"),
		input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewSensitiveCreditProfileView(profile))
}

func (h *Handler) getCreditProfile(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCreditRead) {
		return
	}
	profile, err := h.service.GetCreditProfile(c.Request.Context(), t, c.Param("customerId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	// `crm.credit.read` is what unlocks the limit itself; the masked view exists
	// for callers who reach a credit profile through a broader CRM grant.
	respond.OK(c, NewSensitiveCreditProfileView(profile))
}

type creditHoldRequest struct {
	HoldStatus      string `json:"holdStatus"`
	ReasonCode      string `json:"reasonCode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) applyCreditHold(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCreditManage) {
		return
	}
	var body creditHoldRequest
	if !bindJSON(c, &body) {
		return
	}
	profile, err := h.service.ApplyCreditHold(c.Request.Context(), t, c.Param("customerId"),
		usecases.CreditHoldInput{HoldStatus: body.HoldStatus, ReasonCode: body.ReasonCode,
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c)})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewSensitiveCreditProfileView(profile))
}

func (h *Handler) releaseCreditHold(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCreditManage) {
		return
	}
	var body creditHoldRequest
	if !bindJSON(c, &body) {
		return
	}
	profile, err := h.service.ReleaseCreditHold(c.Request.Context(), t, c.Param("customerId"),
		usecases.CreditHoldInput{ReasonCode: body.ReasonCode,
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion),
			IdempotencyKey:  idempotencyKey(c), CorrelationID: correlationID(c)})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewSensitiveCreditProfileView(profile))
}

func (h *Handler) creditHistory(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCreditRead) {
		return
	}
	page, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	profile, err := h.service.GetCreditProfile(c.Request.Context(), t, c.Param("customerId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	entries, err := h.service.CreditHistory(c.Request.Context(), t, c.Param("customerId"),
		page, hasCursor, limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]CreditHistoryView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, NewCreditHistoryView(entry, profile.Currency))
	}
	token := ""
	if len(entries) > 0 {
		last := entries[len(entries)-1]
		token = nextCursor(len(entries), limit, last.OccurredAt, last.ID)
	}
	respond.List(c, views, token)
}

// ---------------------------------------------------------------------------
// Notification operations
// ---------------------------------------------------------------------------

func (h *Handler) listNotificationOperations(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsRead) {
		return
	}
	if err := allowedQuery(c, "status", "subjectId"); err != nil {
		respond.Error(c, err)
		return
	}
	_, _, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	operations, err := h.service.ListNotifications(c.Request.Context(), t, c.Query("status"),
		c.Query("subjectId"), limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]NotificationOperationView, 0, len(operations))
	for _, operation := range operations {
		views = append(views, NewNotificationOperationView(operation))
	}
	respond.List(c, views, "")
}

func (h *Handler) replayNotification(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionQuotationsManage) {
		return
	}
	operation, err := h.service.ReplayNotification(c.Request.Context(), t,
		c.Param("operationId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.JSON(c, http.StatusAccepted, NewNotificationOperationView(operation))
}
