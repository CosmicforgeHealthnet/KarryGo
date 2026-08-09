package http

// The Section 14 route manifest.
//
// This is the single declarative catalogue of every mounted CRM route. The
// registrars below build the router from it, and the OpenAPI contract tests
// read the same values, so the specification is checked against the thing that
// actually registers routes rather than against a second hand-maintained list
// that can drift.
//
// The permission recorded here is *not* trusted by the parity test. It is
// verified behaviourally: each route is driven through the real handler with
// only the documented permission granted, and separately with nothing granted,
// so a manifest entry that disagrees with the handler fails rather than
// documenting a comfortable fiction. See openapi_contract_test.go.

// SecurityClass distinguishes the three Section 14 route classes, which differ
// in who may call them and how they prove it.
type SecurityClass string

// Route classes.
const (
	// SecurityCorporate: bearer access token plus resolved organization scope
	// plus a permission.
	SecurityCorporate SecurityClass = "corporate"
	// SecurityCustomerToken: a hashed, expiring, single-quotation link. No
	// session, no tenant scope, no corporate permission.
	SecurityCustomerToken SecurityClass = "customerToken"
	// SecurityInternalHMAC: signature v2 plus a caller allowlist. Never
	// reachable with a corporate, provider or customer token.
	SecurityInternalHMAC SecurityClass = "internalHmac"
)

// Route is one mounted Section 14 operation.
type Route struct {
	// Method is the HTTP method as gin registers it.
	Method string
	// Path is the gin-style template relative to the route class prefix, for
	// example "/customers/:customerId".
	Path string
	// OperationID matches the OpenAPI operationId exactly.
	OperationID string
	// Handler is the production handler symbol, for traceability from the
	// specification back to the code.
	Handler string
	// Security is the route class.
	Security SecurityClass
	// Permission is the corporate permission the handler requires. Empty for
	// customer-token and internal routes, which have no corporate permission.
	Permission string
	// FeatureFlag is the environment switch that gates the operation. An
	// operation whose flag is off answers not-found rather than vanishing.
	FeatureFlag string
	// IdempotencyRequired reports whether the handler refuses a request with no
	// Idempotency-Key header.
	IdempotencyRequired bool
	// ExpectedVersionRequired reports whether the handler refuses a request
	// that states no expectedVersion (body field or If-Match).
	ExpectedVersionRequired bool
}

// Route class prefixes, as mounted by the composition root.
const (
	CorporatePrefix     = "/fleet-partner/v1"
	CustomerTokenPrefix = "/fleet-partner/public/v1"
	InternalPrefix      = "/internal/fleet-partner/v1"
)

// Feature flag names, matching internal/config exactly.
const (
	FlagCRMEnabled          = "CRM_ENABLED"
	FlagCustomerWrites      = "CRM_CUSTOMER_WRITES_ENABLED"
	FlagContactWrites       = "CRM_CONTACT_WRITES_ENABLED"
	FlagLocationWrites      = "CRM_LOCATION_WRITES_ENABLED"
	FlagOpportunities       = "CRM_OPPORTUNITIES_ENABLED"
	FlagQuotations          = "QUOTATIONS_ENABLED"
	FlagQuotationIssue      = "QUOTATION_ISSUE_ENABLED"
	FlagQuotationAcceptance = "QUOTATION_ACCEPTANCE_ENABLED"
	FlagContracts           = "CONTRACTS_ENABLED"
	FlagContractActivation  = "CONTRACT_ACTIVATION_ENABLED"
	FlagSLA                 = "SLA_ENABLED"
	FlagCustomerCredit      = "CUSTOMER_CREDIT_ENABLED"
)

// TenantRoutes is the corporate Section 14 catalogue.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		// Customers.
		{Method: "GET", Path: "/customers", OperationID: "listCustomers",
			Handler: "Handler.listCustomers", Security: SecurityCorporate,
			Permission: PermissionCustomersRead, FeatureFlag: FlagCRMEnabled},
		{Method: "POST", Path: "/customers", OperationID: "createCustomer",
			Handler: "Handler.createCustomer", Security: SecurityCorporate,
			Permission: PermissionCustomersManage, FeatureFlag: FlagCustomerWrites,
			IdempotencyRequired: true},
		{Method: "GET", Path: "/customers/:customerId", OperationID: "getCustomer",
			Handler: "Handler.getCustomer", Security: SecurityCorporate,
			Permission: PermissionCustomersRead, FeatureFlag: FlagCRMEnabled},
		{Method: "PATCH", Path: "/customers/:customerId", OperationID: "updateCustomer",
			Handler: "Handler.updateCustomer", Security: SecurityCorporate,
			Permission: PermissionCustomersManage, FeatureFlag: FlagCustomerWrites,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/customers/:customerId/archive", OperationID: "archiveCustomer",
			Handler: "Handler.archiveCustomer", Security: SecurityCorporate,
			Permission: PermissionCustomersManage, FeatureFlag: FlagCustomerWrites,
			IdempotencyRequired: true, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/customers/:customerId/status",
			OperationID: "changeCustomerStatus", Handler: "Handler.changeCustomerStatus",
			Security: SecurityCorporate, Permission: PermissionCustomersManage,
			FeatureFlag: FlagCustomerWrites, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/customers/:customerId/risk-status",
			OperationID: "changeCustomerRiskStatus", Handler: "Handler.changeCustomerRisk",
			Security: SecurityCorporate, Permission: PermissionCustomersManage,
			FeatureFlag: FlagCustomerWrites, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "GET", Path: "/customers/:customerId/history",
			OperationID: "listCustomerHistory", Handler: "Handler.customerHistory",
			Security: SecurityCorporate, Permission: PermissionCustomersRead,
			FeatureFlag: FlagCRMEnabled},

		// Contacts.
		{Method: "GET", Path: "/customers/:customerId/contacts", OperationID: "listContacts",
			Handler: "Handler.listContacts", Security: SecurityCorporate,
			Permission: PermissionContactsRead, FeatureFlag: FlagCRMEnabled},
		{Method: "POST", Path: "/customers/:customerId/contacts", OperationID: "createContact",
			Handler: "Handler.createContact", Security: SecurityCorporate,
			Permission: PermissionContactsManage, FeatureFlag: FlagContactWrites,
			IdempotencyRequired: true},
		{Method: "GET", Path: "/customers/:customerId/contacts/:contactId",
			OperationID: "getContact", Handler: "Handler.getContact",
			Security: SecurityCorporate, Permission: PermissionContactsRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "PATCH", Path: "/customers/:customerId/contacts/:contactId",
			OperationID: "updateContact", Handler: "Handler.updateContact",
			Security: SecurityCorporate, Permission: PermissionContactsManage,
			FeatureFlag: FlagContactWrites, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/customers/:customerId/contacts/:contactId/archive",
			OperationID: "archiveContact", Handler: "Handler.archiveContact",
			Security: SecurityCorporate, Permission: PermissionContactsManage,
			FeatureFlag: FlagContactWrites, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/customers/:customerId/contacts/:contactId/make-primary",
			OperationID: "makeContactPrimary", Handler: "Handler.makeContactPrimary",
			Security: SecurityCorporate, Permission: PermissionContactsManage,
			FeatureFlag: FlagContactWrites, IdempotencyRequired: true,
			ExpectedVersionRequired: true},

		// Locations.
		{Method: "GET", Path: "/customers/:customerId/locations", OperationID: "listLocations",
			Handler: "Handler.listLocations", Security: SecurityCorporate,
			Permission: PermissionLocationsRead, FeatureFlag: FlagCRMEnabled},
		{Method: "POST", Path: "/customers/:customerId/locations",
			OperationID: "createLocation", Handler: "Handler.createLocation",
			Security: SecurityCorporate, Permission: PermissionLocationsManage,
			FeatureFlag: FlagLocationWrites, IdempotencyRequired: true},
		{Method: "GET", Path: "/customers/:customerId/locations/:locationId",
			OperationID: "getLocation", Handler: "Handler.getLocation",
			Security: SecurityCorporate, Permission: PermissionLocationsRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "PATCH", Path: "/customers/:customerId/locations/:locationId",
			OperationID: "updateLocation", Handler: "Handler.updateLocation",
			Security: SecurityCorporate, Permission: PermissionLocationsManage,
			FeatureFlag: FlagLocationWrites, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/customers/:customerId/locations/:locationId/archive",
			OperationID: "archiveLocation", Handler: "Handler.archiveLocation",
			Security: SecurityCorporate, Permission: PermissionLocationsManage,
			FeatureFlag: FlagLocationWrites, IdempotencyRequired: true,
			ExpectedVersionRequired: true},

		// Credit policy.
		{Method: "GET", Path: "/customers/:customerId/credit-profile",
			OperationID: "getCreditProfile", Handler: "Handler.getCreditProfile",
			Security: SecurityCorporate, Permission: PermissionCreditRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "PUT", Path: "/customers/:customerId/credit-profile",
			OperationID: "setCreditProfile", Handler: "Handler.setCreditProfile",
			Security: SecurityCorporate, Permission: PermissionCreditManage,
			FeatureFlag: FlagCustomerCredit, IdempotencyRequired: true},
		{Method: "POST", Path: "/customers/:customerId/credit-profile/hold",
			OperationID: "applyCreditHold", Handler: "Handler.applyCreditHold",
			Security: SecurityCorporate, Permission: PermissionCreditManage,
			FeatureFlag: FlagCustomerCredit, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/customers/:customerId/credit-profile/release",
			OperationID: "releaseCreditHold", Handler: "Handler.releaseCreditHold",
			Security: SecurityCorporate, Permission: PermissionCreditManage,
			FeatureFlag: FlagCustomerCredit, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "GET", Path: "/customers/:customerId/credit-profile/history",
			OperationID: "listCreditHistory", Handler: "Handler.creditHistory",
			Security: SecurityCorporate, Permission: PermissionCreditRead,
			FeatureFlag: FlagCRMEnabled},

		// Opportunities.
		{Method: "GET", Path: "/sales/opportunities", OperationID: "listOpportunities",
			Handler: "Handler.listOpportunities", Security: SecurityCorporate,
			Permission: PermissionOpportunitiesRead, FeatureFlag: FlagCRMEnabled},
		{Method: "POST", Path: "/sales/opportunities", OperationID: "createOpportunity",
			Handler: "Handler.createOpportunity", Security: SecurityCorporate,
			Permission: PermissionOpportunitiesWrite, FeatureFlag: FlagOpportunities,
			IdempotencyRequired: true},
		{Method: "GET", Path: "/sales/opportunities/:opportunityId",
			OperationID: "getOpportunity", Handler: "Handler.getOpportunity",
			Security: SecurityCorporate, Permission: PermissionOpportunitiesRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "PATCH", Path: "/sales/opportunities/:opportunityId",
			OperationID: "updateOpportunity", Handler: "Handler.updateOpportunity",
			Security: SecurityCorporate, Permission: PermissionOpportunitiesWrite,
			FeatureFlag: FlagOpportunities, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/sales/opportunities/:opportunityId/mark-won",
			OperationID: "markOpportunityWon", Handler: "Handler.markOpportunityWon",
			Security: SecurityCorporate, Permission: PermissionOpportunitiesWrite,
			FeatureFlag: FlagOpportunities, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/sales/opportunities/:opportunityId/mark-lost",
			OperationID: "markOpportunityLost", Handler: "Handler.markOpportunityLost",
			Security: SecurityCorporate, Permission: PermissionOpportunitiesWrite,
			FeatureFlag: FlagOpportunities, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/sales/opportunities/:opportunityId/hold",
			OperationID: "holdOpportunity", Handler: "Handler.holdOpportunity",
			Security: SecurityCorporate, Permission: PermissionOpportunitiesWrite,
			FeatureFlag: FlagOpportunities, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/sales/opportunities/:opportunityId/resume",
			OperationID: "resumeOpportunity", Handler: "Handler.resumeOpportunity",
			Security: SecurityCorporate, Permission: PermissionOpportunitiesWrite,
			FeatureFlag: FlagOpportunities, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "GET", Path: "/sales/opportunities/:opportunityId/history",
			OperationID: "listOpportunityHistory", Handler: "Handler.opportunityHistory",
			Security: SecurityCorporate, Permission: PermissionOpportunitiesRead,
			FeatureFlag: FlagCRMEnabled},

		// Quotations.
		{Method: "GET", Path: "/quotations", OperationID: "listQuotations",
			Handler: "Handler.listQuotations", Security: SecurityCorporate,
			Permission: PermissionQuotationsRead, FeatureFlag: FlagCRMEnabled},
		{Method: "POST", Path: "/quotations", OperationID: "createQuotation",
			Handler: "Handler.createQuotation", Security: SecurityCorporate,
			Permission: PermissionQuotationsManage, FeatureFlag: FlagQuotations,
			IdempotencyRequired: true},
		{Method: "GET", Path: "/quotations/:quotationId", OperationID: "getQuotation",
			Handler: "Handler.getQuotation", Security: SecurityCorporate,
			Permission: PermissionQuotationsRead, FeatureFlag: FlagCRMEnabled},
		{Method: "PATCH", Path: "/quotations/:quotationId", OperationID: "updateQuotation",
			Handler: "Handler.updateQuotation", Security: SecurityCorporate,
			Permission: PermissionQuotationsManage, FeatureFlag: FlagQuotations,
			ExpectedVersionRequired: true},
		{Method: "GET", Path: "/quotations/:quotationId/history",
			OperationID: "listQuotationHistory", Handler: "Handler.quotationHistory",
			Security: SecurityCorporate, Permission: PermissionQuotationsRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "GET", Path: "/quotations/:quotationId/versions",
			OperationID: "listQuotationVersions", Handler: "Handler.quotationVersions",
			Security: SecurityCorporate, Permission: PermissionQuotationsRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "GET", Path: "/quotations/:quotationId/versions/:versionId",
			OperationID: "getQuotationVersion", Handler: "Handler.quotationVersion",
			Security: SecurityCorporate, Permission: PermissionQuotationsRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "GET", Path: "/quotations/:quotationId/documents",
			OperationID: "listQuotationDocuments", Handler: "Handler.quotationDocuments",
			Security: SecurityCorporate, Permission: PermissionQuotationsRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "POST", Path: "/quotations/:quotationId/documents",
			OperationID: "attachQuotationDocument",
			Handler:     "Handler.attachQuotationDocument", Security: SecurityCorporate,
			Permission: PermissionQuotationsManage, FeatureFlag: FlagQuotations},
		{Method: "POST", Path: "/quotations/:quotationId/submit",
			OperationID: "submitQuotation", Handler: "Handler.submitQuotation",
			Security: SecurityCorporate, Permission: PermissionQuotationsSubmit,
			FeatureFlag: FlagQuotations, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/quotations/:quotationId/issue",
			OperationID: "issueQuotation", Handler: "Handler.issueQuotation",
			Security: SecurityCorporate, Permission: PermissionQuotationsIssue,
			FeatureFlag: FlagQuotationIssue, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/quotations/:quotationId/accept",
			OperationID: "acceptQuotationAsStaff", Handler: "Handler.acceptQuotation",
			Security: SecurityCorporate, Permission: PermissionQuotationsAccept,
			FeatureFlag: FlagQuotationAcceptance, IdempotencyRequired: true},
		{Method: "POST", Path: "/quotations/:quotationId/reject",
			OperationID: "rejectQuotationAsStaff", Handler: "Handler.rejectQuotation",
			Security: SecurityCorporate, Permission: PermissionQuotationsAccept,
			FeatureFlag: FlagQuotationAcceptance, IdempotencyRequired: true},
		{Method: "POST", Path: "/quotations/:quotationId/cancel",
			OperationID: "cancelQuotation", Handler: "Handler.cancelQuotation",
			Security: SecurityCorporate, Permission: PermissionQuotationsManage,
			FeatureFlag: FlagQuotations, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/quotations/:quotationId/supersede",
			OperationID: "supersedeQuotation", Handler: "Handler.supersedeQuotation",
			Security: SecurityCorporate, Permission: PermissionQuotationsManage,
			FeatureFlag: FlagQuotations, IdempotencyRequired: true,
			ExpectedVersionRequired: true},

		// Contracts.
		{Method: "GET", Path: "/contracts", OperationID: "listContracts",
			Handler: "Handler.listContracts", Security: SecurityCorporate,
			Permission: PermissionContractsRead, FeatureFlag: FlagCRMEnabled},
		{Method: "POST", Path: "/contracts", OperationID: "createContract",
			Handler: "Handler.createContract", Security: SecurityCorporate,
			Permission: PermissionContractsManage, FeatureFlag: FlagContracts,
			IdempotencyRequired: true},
		{Method: "GET", Path: "/contracts/:contractId", OperationID: "getContract",
			Handler: "Handler.getContract", Security: SecurityCorporate,
			Permission: PermissionContractsRead, FeatureFlag: FlagCRMEnabled},
		{Method: "PATCH", Path: "/contracts/:contractId", OperationID: "updateContract",
			Handler: "Handler.updateContract", Security: SecurityCorporate,
			Permission: PermissionContractsManage, FeatureFlag: FlagContracts,
			ExpectedVersionRequired: true},
		{Method: "GET", Path: "/contracts/:contractId/history",
			OperationID: "listContractHistory", Handler: "Handler.contractHistory",
			Security: SecurityCorporate, Permission: PermissionContractsRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "GET", Path: "/contracts/:contractId/versions",
			OperationID: "listContractVersions", Handler: "Handler.contractVersions",
			Security: SecurityCorporate, Permission: PermissionContractsRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "GET", Path: "/contracts/:contractId/versions/:versionId",
			OperationID: "getContractVersion", Handler: "Handler.contractVersion",
			Security: SecurityCorporate, Permission: PermissionContractsRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "GET", Path: "/contracts/:contractId/documents",
			OperationID: "listContractDocuments", Handler: "Handler.contractDocuments",
			Security: SecurityCorporate, Permission: PermissionContractsRead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "POST", Path: "/contracts/:contractId/documents",
			OperationID: "attachContractDocument", Handler: "Handler.attachContractDocument",
			Security: SecurityCorporate, Permission: PermissionContractsManage,
			FeatureFlag: FlagContracts},
		{Method: "POST", Path: "/contracts/:contractId/documents/:documentId/replace",
			OperationID: "replaceContractDocument",
			Handler:     "Handler.replaceContractDocument", Security: SecurityCorporate,
			Permission: PermissionContractsManage, FeatureFlag: FlagContracts},
		{Method: "POST", Path: "/contracts/:contractId/documents/:documentId/remove",
			OperationID: "removeContractDocument", Handler: "Handler.removeContractDocument",
			Security: SecurityCorporate, Permission: PermissionContractsManage,
			FeatureFlag: FlagContracts},
		{Method: "POST", Path: "/contracts/:contractId/submit",
			OperationID: "submitContract", Handler: "Handler.submitContract",
			Security: SecurityCorporate, Permission: PermissionContractsManage,
			FeatureFlag: FlagContracts, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/contracts/:contractId/activate",
			OperationID: "activateContract", Handler: "Handler.activateContract",
			Security: SecurityCorporate, Permission: PermissionContractsActivate,
			FeatureFlag: FlagContractActivation, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/contracts/:contractId/suspend",
			OperationID: "suspendContract", Handler: "Handler.suspendContract",
			Security: SecurityCorporate, Permission: PermissionContractsManage,
			FeatureFlag: FlagContracts, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/contracts/:contractId/resume",
			OperationID: "resumeContract", Handler: "Handler.resumeContract",
			Security: SecurityCorporate, Permission: PermissionContractsActivate,
			FeatureFlag: FlagContractActivation, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/contracts/:contractId/terminate",
			OperationID: "terminateContract", Handler: "Handler.terminateContract",
			Security: SecurityCorporate, Permission: PermissionContractsTerminate,
			FeatureFlag: FlagContracts, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/contracts/:contractId/renew",
			OperationID: "renewContract", Handler: "Handler.renewContract",
			Security: SecurityCorporate, Permission: PermissionContractsManage,
			FeatureFlag: FlagContracts, IdempotencyRequired: true,
			ExpectedVersionRequired: true},

		// Service level agreements.
		{Method: "GET", Path: "/service-level-agreements", OperationID: "listServiceLevelAgreements",
			Handler: "Handler.listSLAs", Security: SecurityCorporate,
			Permission: PermissionSLARead, FeatureFlag: FlagCRMEnabled},
		{Method: "POST", Path: "/service-level-agreements",
			OperationID: "createServiceLevelAgreement", Handler: "Handler.createSLA",
			Security: SecurityCorporate, Permission: PermissionSLAManage,
			FeatureFlag: FlagSLA, IdempotencyRequired: true},
		{Method: "GET", Path: "/service-level-agreements/:slaId",
			OperationID: "getServiceLevelAgreement", Handler: "Handler.getSLA",
			Security: SecurityCorporate, Permission: PermissionSLARead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "PATCH", Path: "/service-level-agreements/:slaId",
			OperationID: "updateServiceLevelAgreement", Handler: "Handler.updateSLA",
			Security: SecurityCorporate, Permission: PermissionSLAManage,
			FeatureFlag: FlagSLA, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/service-level-agreements/:slaId/activate",
			OperationID: "activateServiceLevelAgreement", Handler: "Handler.activateSLA",
			Security: SecurityCorporate, Permission: PermissionSLAActivate,
			FeatureFlag: FlagSLA, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/service-level-agreements/:slaId/suspend",
			OperationID: "suspendServiceLevelAgreement", Handler: "Handler.suspendSLA",
			Security: SecurityCorporate, Permission: PermissionSLAActivate,
			FeatureFlag: FlagSLA, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "POST", Path: "/service-level-agreements/:slaId/terminate",
			OperationID: "terminateServiceLevelAgreement", Handler: "Handler.terminateSLA",
			Security: SecurityCorporate, Permission: PermissionSLAManage,
			FeatureFlag: FlagSLA, IdempotencyRequired: true,
			ExpectedVersionRequired: true},
		{Method: "GET", Path: "/service-level-agreements/:slaId/observations",
			OperationID: "listServiceLevelObservations", Handler: "Handler.slaObservations",
			Security: SecurityCorporate, Permission: PermissionSLARead,
			FeatureFlag: FlagCRMEnabled},
		{Method: "GET", Path: "/service-level-agreements/:slaId/history",
			OperationID: "listServiceLevelAgreementHistory", Handler: "Handler.slaHistory",
			Security: SecurityCorporate, Permission: PermissionSLARead,
			FeatureFlag: FlagCRMEnabled},

		// Notification operations — the operator dead-letter surface.
		{Method: "GET", Path: "/crm/notification-operations",
			OperationID: "listNotificationOperations",
			Handler:     "Handler.listNotificationOperations", Security: SecurityCorporate,
			Permission: PermissionQuotationsRead, FeatureFlag: FlagCRMEnabled},
		{Method: "POST", Path: "/crm/notification-operations/:operationId/replay",
			OperationID: "replayNotificationOperation",
			Handler:     "Handler.replayNotification", Security: SecurityCorporate,
			Permission: PermissionQuotationsManage, FeatureFlag: FlagCRMEnabled},
	}
}

// CustomerTokenRoutes is the secure-link catalogue.
//
// These carry no corporate permission by construction: the token is the whole
// authorization decision, so a Permission value here would be a claim the
// handler never checks.
func (h *PublicHandler) CustomerTokenRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/quotations/:quotationId/view",
			OperationID: "viewQuotationAsCustomer", Handler: "PublicHandler.viewQuotation",
			Security: SecurityCustomerToken, FeatureFlag: FlagCRMEnabled},
		{Method: "POST", Path: "/quotations/:quotationId/accept",
			OperationID: "acceptQuotationAsCustomer",
			Handler:     "PublicHandler.acceptQuotation", Security: SecurityCustomerToken,
			FeatureFlag: FlagQuotationAcceptance, IdempotencyRequired: true},
		{Method: "POST", Path: "/quotations/:quotationId/reject",
			OperationID: "rejectQuotationAsCustomer",
			Handler:     "PublicHandler.rejectQuotation", Security: SecurityCustomerToken,
			FeatureFlag: FlagQuotationAcceptance, IdempotencyRequired: true},
	}
}

// InternalRoutes is the HMAC-authenticated catalogue.
func (h *InternalHandler) InternalRoutes() []Route {
	return []Route{
		{Method: "POST", Path: "/crm/sla-observations",
			OperationID: "ingestServiceLevelObservation",
			Handler:     "InternalHandler.ingestObservation", Security: SecurityInternalHMAC,
			FeatureFlag: FlagSLA},
	}
}

// AllRoutes returns every Section 14 route across the three classes.
//
// It takes the handlers so the catalogue can never be read without the objects
// that mount it, which is what keeps the two from drifting apart.
func AllRoutes(corporate *Handler, customer *PublicHandler, internal *InternalHandler) []Route {
	routes := make([]Route, 0, 80)
	if corporate != nil {
		routes = append(routes, corporate.TenantRoutes()...)
	}
	if customer != nil {
		routes = append(routes, customer.CustomerTokenRoutes()...)
	}
	if internal != nil {
		routes = append(routes, internal.InternalRoutes()...)
	}
	return routes
}

// Prefix returns the route-class prefix a route is mounted under.
func (r Route) Prefix() string {
	switch r.Security {
	case SecurityCustomerToken:
		return CustomerTokenPrefix
	case SecurityInternalHMAC:
		return InternalPrefix
	default:
		return CorporatePrefix
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return r.Prefix() + r.Path }
