// Package http exposes per-organization security settings.
package http

// Route is the OpenAPI-contract-parity manifest shape.
type Route struct {
	Method                  string
	Path                    string
	OperationID             string
	Permission              string
	ExpectedVersionRequired bool
}

// CorporatePrefix is the tenant-scoped route-class prefix.
const CorporatePrefix = "/fleet-partner/v1"

// TenantRoutes is the corporate security-settings surface.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/security-settings", OperationID: "getSecuritySettings", Permission: PermissionRead},
		{Method: "PUT", Path: "/security-settings", OperationID: "updateSecuritySettings",
			Permission: PermissionManage, ExpectedVersionRequired: true},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
