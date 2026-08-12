package http

type Route struct {
	Method, Path, OperationID, Permission string
	IdempotencyRequired                   bool
	ExpectedVersionRequired               bool
}

func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{"POST", "/private-assignments", "createPrivateAssignment", PermissionManage, true, false},
		{"GET", "/private-assignments", "listPrivateAssignments", PermissionRead, false, false},
		{"GET", "/private-assignments/:assignmentId", "getPrivateAssignment", PermissionRead, false, false},
		{"POST", "/private-assignments/:assignmentId/cancel", "cancelPrivateAssignment", PermissionManage, true, true},
		{"POST", "/private-assignments/:assignmentId/reassign", "reassignPrivateAssignment", PermissionManage, true, true},
		{"GET", "/private-executions/:executionId", "getPrivateExecution", PermissionExecutionRead, false, false},
		{"POST", "/private-executions/:executionId/replay", "replayPrivateExecution", PermissionExecutionReplay, true, false},
		{"POST", "/private-execution-reconciliations/:caseId/replay", "replayPrivateReconciliation", PermissionExecutionReplay, true, false},
	}
}

func (h *Handler) InternalRoutes() []Route {
	return []Route{
		{"GET", "/private-assignments/:assignmentId", "getPrivateAssignmentForProvider", PermissionRespond, false, false},
		{"POST", "/private-assignments/:assignmentId/accept", "acceptPrivateAssignment", PermissionRespond, true, true},
		{"POST", "/private-assignments/:assignmentId/decline", "declinePrivateAssignment", PermissionRespond, true, true},
	}
}
