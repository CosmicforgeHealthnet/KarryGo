package http

type Route struct {
	Method, Path, OperationID, Permission        string
	IdempotencyRequired, ExpectedVersionRequired bool
}

// TenantRoutes is the legacy (engine=LEGACY) order-import compatibility
// surface — unchanged. GenericTenantRoutes below is the additive Section 25C
// surface; both are registered by RegisterTenantRoutes.
func (h *Handler) TenantRoutes() []Route {
	return []Route{{"GET", "/orders/imports", "listBulkImports", PermissionRead, false, false}, {"POST", "/orders/imports", "createBulkImport", PermissionManage, true, false}, {"GET", "/orders/imports/:importId", "getBulkImport", PermissionRead, false, false}, {"POST", "/orders/imports/:importId/cancel", "cancelBulkImport", PermissionManage, true, true}, {"POST", "/orders/imports/:importId/replay", "replayBulkImport", PermissionManage, true, false}}
}

// GenericTenantRoutes is the Section 25C generic import catalogue.
func (h *Handler) GenericTenantRoutes() []Route {
	return []Route{
		{"GET", "/imports", "listImports", PermissionRead, false, false},
		{"POST", "/imports", "createImport", PermissionManage, true, false},
		{"GET", "/imports/:importId", "getImport", PermissionRead, false, false},
		{"GET", "/imports/:importId/rows", "listImportRows", PermissionRead, false, false},
		{"POST", "/imports/:importId/validate", "validateImport", PermissionValidate, true, false},
		{"POST", "/imports/:importId/commit", "commitImport", PermissionCommit, true, false},
		{"POST", "/imports/:importId/retry", "retryImport", PermissionRetry, true, false},
		{"POST", "/imports/:importId/rows/:rowId/retry", "retryImportRow", PermissionRetry, true, false},
		{"POST", "/imports/:importId/rows/:rowId/corrections", "correctImportRow", PermissionRetry, true, true},
		{"POST", "/imports/:importId/cancel", "cancelImport", PermissionManage, true, true},
		{"POST", "/imports/:importId/replay", "replayImport", PermissionRetry, true, false},
	}
}
