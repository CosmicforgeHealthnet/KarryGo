package http

// Route is one mounted Section 15 shipment/cargo operation, following the
// established manifest convention: a single declarative catalogue the
// router builds from.
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

// TenantRoutes is the corporate Section 15 shipment and cargo item catalogue.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/shipments", OperationID: "listShipments", Permission: PermissionShipmentsRead},
		{Method: "POST", Path: "/shipments", OperationID: "createShipment",
			Permission: PermissionShipmentsManage, IdempotencyRequired: true},
		{Method: "GET", Path: "/shipments/:shipmentId", OperationID: "getShipment",
			Permission: PermissionShipmentsRead},
		{Method: "PATCH", Path: "/shipments/:shipmentId", OperationID: "updateShipment",
			Permission: PermissionShipmentsManage, ExpectedVersionRequired: true},
		{Method: "GET", Path: "/shipments/:shipmentId/cargo-items", OperationID: "listCargoItems",
			Permission: PermissionShipmentsRead},
		{Method: "POST", Path: "/shipments/:shipmentId/cargo-items", OperationID: "addCargoItem",
			Permission: PermissionShipmentsManage, IdempotencyRequired: true},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
