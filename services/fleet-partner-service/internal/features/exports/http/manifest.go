package http

// Route follows the manifest convention already used by search, orders and
// bulk imports.
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

// TenantRoutes is the Section 25D generic export catalogue.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/exports", OperationID: "listExports", Permission: PermissionRead},
		{Method: "POST", Path: "/exports", OperationID: "createExport", Permission: PermissionManage, IdempotencyRequired: true},
		{Method: "GET", Path: "/exports/:exportId", OperationID: "getExport", Permission: PermissionRead},
		{Method: "POST", Path: "/exports/:exportId/download-url", OperationID: "exportDownloadURL", Permission: PermissionDownload},
		{Method: "POST", Path: "/exports/:exportId/cancel", OperationID: "cancelExport", Permission: PermissionManage, IdempotencyRequired: true, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/exports/:exportId/replay", OperationID: "replayExport", Permission: PermissionManage, IdempotencyRequired: true},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
