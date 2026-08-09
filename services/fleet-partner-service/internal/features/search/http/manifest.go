package http

// Route is the one mounted Section 25 search operation, following the
// established manifest convention.
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

// TenantRoutes is the corporate Section 25 search catalogue.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/search", OperationID: "search", Permission: PermissionSearchRead},
		// Operator repair: force a subject to be reindexed, or removed from
		// results, independently of whichever feature normally drives it.
		// Gated by the narrower search.repair permission, not search.read —
		// reading results and repairing the index are different capabilities.
		{Method: "POST", Path: "/search/reindex", OperationID: "reindexSearchSubject", Permission: PermissionSearchRepair},
		{Method: "POST", Path: "/search/remove", OperationID: "removeSearchSubject", Permission: PermissionSearchRepair},
		{Method: "POST", Path: "/search/index-jobs/:jobId/replay", OperationID: "replaySearchIndexJob", Permission: PermissionSearchReplay},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
