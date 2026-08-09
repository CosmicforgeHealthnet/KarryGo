// Package http exposes the corporate append-only audit trail as read-only
// routes. Section 28 requires audit access itself be tenant-isolated,
// field-redacted and permission-gated in two tiers — this package is the
// entire surface for that requirement; the write path lives in
// internal/platform/audit and is intentionally not reachable from here.
package http

// Route is the OpenAPI-contract-parity manifest shape, following the
// convention documents, search, exports and backgroundops already use.
type Route struct {
	Method      string
	Path        string
	OperationID string
	Permission  string
}

// CorporatePrefix is the tenant-scoped route-class prefix.
const CorporatePrefix = "/fleet-partner/v1"

// TenantRoutes is the audit-log read surface.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/audit-logs", OperationID: "listAuditLogs", Permission: PermissionAuditRead},
		{Method: "GET", Path: "/audit-logs/:auditId", OperationID: "getAuditLog", Permission: PermissionAuditRead},
		{Method: "POST", Path: "/audit/verify-chain", OperationID: "verifyAuditChain", Permission: PermissionAuditVerifyChain},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
