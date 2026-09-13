package http

import (
	"github.com/gin-gonic/gin"
)

// Route defines a single HTTP route.
type Route struct {
	Method     string
	Path       string
	Handler    string
	Permission string
}

// TenantRoutes returns all dashboard routes.
func TenantRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/dashboard", Handler: "GetDashboard", Permission: "dashboard.read"},
	}
}

// RegisterTenantRoutes mounts the dashboard routes.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.GET("/dashboard", h.GetDashboard)
}