package http

// Route is the OpenAPI-contract-parity manifest shape, following the
// convention documents, search, exports, backgroundops and audit already
// use.
type Route struct {
	Method      string
	Path        string
	OperationID string
	Permission  string
}

// CorporatePrefix is the tenant-scoped route-class prefix.
const CorporatePrefix = "/fleet-partner/v1"

// TenantRoutes is the retention-execution surface.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "POST", Path: "/retention-runs", OperationID: "initiateRetentionRun", Permission: PermissionRetentionExecute},
		{Method: "GET", Path: "/retention-runs/:runId", OperationID: "getRetentionRun", Permission: PermissionRetentionExecute},
		{Method: "GET", Path: "/retention-runs/:runId/records", OperationID: "listRetentionRunRecords", Permission: PermissionRetentionExecute},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
