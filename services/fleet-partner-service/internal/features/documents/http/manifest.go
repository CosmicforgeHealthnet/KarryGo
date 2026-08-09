package http

// Route is the OpenAPI-contract-parity manifest shape, following the
// convention search, exports, orders and bulk imports already use.
//
// Documents itself does not mount routes from this list (RegisterTenantRoutes
// predates the manifest convention and registers them directly); this method
// is purely descriptive, added additively so the Section 25 contract test can
// enumerate the same routes RegisterTenantRoutes already serves without a
// second, drifting definition of what "correct" looks like.
type Route struct {
	Method                  string
	Path                    string
	OperationID             string
	Permission              string
	IdempotencyRequired     bool
	ExpectedVersionRequired bool
}

// CorporatePrefix is the tenant-scoped route-class prefix.
const CorporatePrefix = "/fleet-partner/v1"

// TenantRoutes is the corporate document catalogue, in the same order
// RegisterTenantRoutes mounts them.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "POST", Path: "/documents/upload-requests", OperationID: "requestDocumentUpload",
			Permission: PermissionManage, IdempotencyRequired: true},
		{Method: "GET", Path: "/documents", OperationID: "listDocuments", Permission: PermissionRead},
		{Method: "GET", Path: "/documents/:documentId", OperationID: "getDocument", Permission: PermissionRead},
		{Method: "GET", Path: "/documents/:documentId/history", OperationID: "documentHistory", Permission: PermissionRead},
		{Method: "GET", Path: "/documents/:documentId/verifications", OperationID: "documentVerifications", Permission: PermissionRead},
		{Method: "POST", Path: "/documents/:documentId/download-requests", OperationID: "requestDocumentDownload",
			Permission: PermissionDownload},
		{Method: "POST", Path: "/documents/:documentId/verification-requests", OperationID: "requestDocumentVerification",
			Permission: PermissionManage, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/documents/:documentId/verify", OperationID: "verifyDocument",
			Permission: PermissionVerify, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/documents/:documentId/reject", OperationID: "rejectDocument",
			Permission: PermissionVerify, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/documents/:documentId/replace", OperationID: "replaceDocument",
			Permission: PermissionManage, IdempotencyRequired: true, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/documents/:documentId/revoke", OperationID: "revokeDocument",
			Permission: PermissionManage, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/documents/:documentId/archive", OperationID: "archiveDocument",
			Permission: PermissionManage, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/documents/:documentId/retention", OperationID: "setDocumentRetention",
			Permission: PermissionDelete, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/documents/:documentId/deletion-requests", OperationID: "scheduleDocumentDeletion",
			Permission: PermissionDelete, IdempotencyRequired: true, ExpectedVersionRequired: true},
		{Method: "GET", Path: "/documents/:documentId/deletion", OperationID: "readDocumentDeletion",
			Permission: PermissionRead},
		{Method: "GET", Path: "/documents/reconciliation-cases", OperationID: "listDocumentReconciliationCases",
			Permission: PermissionRead},
		{Method: "POST", Path: "/documents/reconciliation-cases/:caseId/replay", OperationID: "replayDocumentReconciliationCase",
			Permission: PermissionDelete},
		{Method: "POST", Path: "/documents/deletion-requests/:requestId/replay", OperationID: "replayDocumentDeletion",
			Permission: PermissionDelete},
	}
}

// InternalRoutes is the HMAC-authenticated service-to-service document
// surface: media event ingestion (documenthttp.RegisterInternalRoutes) and
// the provider-facing/customer-facing external access surface
// (documenthttp.RegisterExternalRoutes). Both are mounted under
// /internal/fleet-partner/v1 by the composition root.
func InternalRoutes() []Route {
	return []Route{
		{Method: "POST", Path: "/documents/media-events", OperationID: "ingestDocumentMediaEvent"},
		{Method: "GET", Path: "/providers/:providerId/documents", OperationID: "listProviderDocuments"},
		{Method: "GET", Path: "/providers/:providerId/documents/:documentId", OperationID: "getProviderDocument"},
		{Method: "POST", Path: "/providers/:providerId/documents/:documentId/download-requests",
			OperationID: "requestProviderDocumentDownload"},
		{Method: "GET", Path: "/customers/:customerId/orders/:orderId/documents", OperationID: "listCustomerDocuments"},
		{Method: "GET", Path: "/customers/:customerId/orders/:orderId/documents/:documentId",
			OperationID: "getCustomerDocument"},
		{Method: "POST", Path: "/customers/:customerId/orders/:orderId/documents/:documentId/download-requests",
			OperationID: "requestCustomerDocumentDownload"},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
