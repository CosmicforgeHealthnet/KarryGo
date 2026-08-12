package http

// Route is one mounted Section 15 private order operation, following the
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

// TenantRoutes is the corporate Section 15 private order catalogue.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/orders", OperationID: "listOrders", Permission: PermissionOrdersRead},
		{Method: "POST", Path: "/orders", OperationID: "createOrder",
			Permission: PermissionOrdersManage, IdempotencyRequired: true},
		{Method: "GET", Path: "/orders/:orderId", OperationID: "getOrder", Permission: PermissionOrdersRead},
		{Method: "POST", Path: "/orders/:orderId/confirm", OperationID: "confirmOrder",
			Permission: PermissionOrdersConfirm, IdempotencyRequired: true, ExpectedVersionRequired: true},
		{Method: "POST", Path: "/orders/:orderId/cancel", OperationID: "cancelOrder",
			Permission: PermissionOrdersCancel, IdempotencyRequired: true, ExpectedVersionRequired: true},
		{Method: "GET", Path: "/orders/:orderId/history", OperationID: "listOrderHistory",
			Permission: PermissionOrdersRead},
		{Method: "GET", Path: "/orders/:orderId/stops", OperationID: "listOrderStops",
			Permission: PermissionOrdersRead},
		{Method: "POST", Path: "/orders/:orderId/stops", OperationID: "addOrderStop",
			Permission: PermissionOrderStopsManage, IdempotencyRequired: true},
		// The Section 14 handoff: converts an accepted quotation's conversion
		// intent into a private order. Idempotent by construction (see
		// convertConversionIntent's doc comment), so it carries no
		// Idempotency-Key requirement of its own.
		{Method: "POST", Path: "/orders/from-quotation/:acceptedVersionId", OperationID: "convertQuotationToOrder",
			Permission: PermissionOrdersManage},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
