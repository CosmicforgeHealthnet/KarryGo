// Package http exposes per-organization retention policy overrides.
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

// TenantRoutes is the corporate retention-policy surface.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/retention-policies", OperationID: "listRetentionPolicies", Permission: PermissionRead},
		{Method: "GET", Path: "/retention-policies/:class", OperationID: "getRetentionPolicy", Permission: PermissionRead},
		{Method: "PUT", Path: "/retention-policies/:class", OperationID: "updateRetentionPolicy",
			Permission: PermissionManage, ExpectedVersionRequired: true},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
