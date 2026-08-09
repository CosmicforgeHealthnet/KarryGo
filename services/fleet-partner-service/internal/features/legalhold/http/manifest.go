// Package http exposes the generic (non-document) legal-hold API.
package http

// Route is the OpenAPI-contract-parity manifest shape, following the
// convention documents, search, exports and audit already use.
type Route struct {
	Method                  string
	Path                    string
	OperationID             string
	Permission              string
	ExpectedVersionRequired bool
}

// CorporatePrefix is the tenant-scoped route-class prefix.
const CorporatePrefix = "/fleet-partner/v1"

// TenantRoutes is the corporate legal-hold surface.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "POST", Path: "/legal-holds", OperationID: "createLegalHold", Permission: PermissionManage},
		{Method: "GET", Path: "/legal-holds", OperationID: "listLegalHolds", Permission: PermissionRead},
		{Method: "GET", Path: "/legal-holds/:holdId", OperationID: "getLegalHold", Permission: PermissionRead},
		{Method: "POST", Path: "/legal-holds/:holdId/release", OperationID: "releaseLegalHold",
			Permission: PermissionRelease, ExpectedVersionRequired: true},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
