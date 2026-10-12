package staffhttp

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts /staff, /roles, /permissions, /assignments onto
// group, which the caller (cmd/main.go, the composition root) must already
// have protected with the auth feature's RequireAuth middleware before
// calling this — staffhttp never imports internal/features/auth/http
// directly, which the cross-feature import guardrail
// (internal/architecture_test.go) forbids. This mirrors how every other
// service wires cross-cutting middleware from its own cmd/main.go rather
// than one feature importing another feature's HTTP package.
func RegisterRoutes(group *gin.RouterGroup, handler *Handler) {
	staff := group.Group("/staff")
	staff.GET("/me/access", handler.CurrentAccess)
	staff.GET("", handler.ListStaff)
	staff.GET("/:id", handler.GetStaff)
	staff.POST("", handler.InviteStaff)
	staff.PUT("/:id/suspend", handler.SuspendStaff)
	staff.PUT("/:id/reactivate", handler.ReactivateStaff)
	staff.GET("/:id/roles", handler.ListAssignments)

	group.GET("/permissions", handler.ListPermissions)

	roles := group.Group("/roles")
	roles.GET("", handler.ListRoles)
	roles.GET("/:id", handler.GetRole)
	roles.POST("", handler.CreateRole)
	roles.PUT("/:id/permissions", handler.SetRolePermissions)
	roles.POST("/:id/assign", handler.AssignRole)

	group.DELETE("/assignments/:id", handler.UnassignRole)
}
