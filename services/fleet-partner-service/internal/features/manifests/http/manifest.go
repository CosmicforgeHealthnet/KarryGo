package http

type Route struct {
	Method, Path, OperationID, Permission        string
	IdempotencyRequired, ExpectedVersionRequired bool
}

func (h *Handler) TenantRoutes() []Route {
	return []Route{{"GET", "/manifests", "listManifests", PermissionRead, false, false}, {"POST", "/manifests", "createManifest", PermissionManage, true, false}, {"GET", "/manifests/:manifestId", "getManifest", PermissionRead, false, false}, {"PATCH", "/manifests/:manifestId", "updateManifest", PermissionManage, false, true}, {"POST", "/manifests/:manifestId/items", "addManifestItem", PermissionManage, true, false}, {"POST", "/manifests/:manifestId/close", "closeManifest", PermissionManage, true, true}, {"POST", "/manifests/:manifestId/seal-amendments", "amendManifestSeal", PermissionSealAmend, true, true}, {"POST", "/manifests/:manifestId/cancel", "cancelManifest", PermissionManage, true, true}}
}
