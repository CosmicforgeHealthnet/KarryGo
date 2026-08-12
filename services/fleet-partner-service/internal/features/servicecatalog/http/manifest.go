package http

// Route is one mounted Section 15 service catalogue operation, following the
// Section 14 CRM manifest convention: a single declarative catalogue the
// router builds from, so the specification and the thing that actually
// registers routes cannot drift apart.
type Route struct {
	Method                  string
	Path                    string
	OperationID             string
	Permission              string
	IdempotencyRequired     bool
	ExpectedVersionRequired bool
}

// CorporatePrefix is the tenant-scoped route-class prefix, matching the
// composition root's mounted group.
const CorporatePrefix = "/fleet-partner/v1"

// TenantRoutes is the corporate Section 15 service catalogue catalogue.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/service-catalog", OperationID: "listServiceCatalogItems",
			Permission: PermissionCatalogRead},
		{Method: "POST", Path: "/service-catalog", OperationID: "createServiceCatalogItem",
			Permission: PermissionCatalogManage, IdempotencyRequired: true},
		{Method: "GET", Path: "/service-catalog/:serviceId", OperationID: "getServiceCatalogItem",
			Permission: PermissionCatalogRead},
		{Method: "PATCH", Path: "/service-catalog/:serviceId", OperationID: "updateServiceCatalogItem",
			Permission: PermissionCatalogManage, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/service-catalog/:serviceId/activate", OperationID: "activateServiceCatalogItem",
			Permission: PermissionCatalogActivate, IdempotencyRequired: true, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/service-catalog/:serviceId/deactivate", OperationID: "deactivateServiceCatalogItem",
			Permission: PermissionCatalogDeactivate, IdempotencyRequired: true, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/service-catalog/:serviceId/archive", OperationID: "archiveServiceCatalogItem",
			Permission: PermissionCatalogArchive, IdempotencyRequired: true, ExpectedVersionRequired: true},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
