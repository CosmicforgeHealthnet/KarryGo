package http

type Route struct {
	Method, Path, OperationID, Permission        string
	IdempotencyRequired, ExpectedVersionRequired bool
}

func (h *Handler) TenantRoutes() []Route {
	return []Route{{"GET", "/recurring-orders", "listRecurringOrders", PermissionRead, false, false}, {"POST", "/recurring-orders", "createRecurringOrder", PermissionManage, true, false}, {"GET", "/recurring-orders/:templateId", "getRecurringOrder", PermissionRead, false, false}, {"POST", "/recurring-orders/:templateId/activate", "activateRecurringOrder", PermissionManage, true, true}, {"POST", "/recurring-orders/:templateId/deactivate", "deactivateRecurringOrder", PermissionManage, true, true}, {"POST", "/recurring-orders/:templateId/replay", "replayRecurringOrder", PermissionManage, true, false}}
}
