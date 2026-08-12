package http

type Route struct {
	Method, Path, OperationID, Permission        string
	IdempotencyRequired, ExpectedVersionRequired bool
}

const CorporatePrefix = "/fleet-partner/v1"

func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{"POST", "/shipments/:shipmentId/packages", "createPackage", PermissionPackagesManage, true, false},
		{"GET", "/shipments/:shipmentId/packages", "listPackages", PermissionPackagesRead, false, false},
		{"GET", "/packages/:packageId", "getPackage", PermissionPackagesRead, false, false},
		{"GET", "/packages/:packageId/history", "getPackageHistory", PermissionPackagesRead, false, false},
		{"POST", "/packages/:packageId/transition", "transitionPackage", PermissionPackagesManage, true, true},
		{"POST", "/packages/:packageId/scan", "scanPackage", PermissionPackagesScan, true, true},
		{"GET", "/packages/:packageId/custody", "listCustody", PermissionCustodyRead, false, false},
		{"POST", "/packages/:packageId/custody", "recordCustody", PermissionCustodyRecord, true, false},
	}
}
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
