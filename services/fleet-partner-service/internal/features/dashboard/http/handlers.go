package http

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/dashboard/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Handler serves the dashboard HTTP endpoints.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler creates a new dashboard handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{
		service:    service,
		authorizer: authorizer,
	}
}

// GetDashboard returns the aggregated dashboard data.
// GET /dashboard
func (h *Handler) GetDashboard(c *gin.Context) {
	// Get tenant context
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return
	}

	// Authorize: user must have dashboard.read permission
	err := h.authorizer.Require(c, tenantCtx.OrganizationID, tenantCtx.OrganizationUserID,
		roleVersion(c), "dashboard.read", accesscontrol.Target{})
	if err != nil {
		respond.Error(c, err)
		return
	}

	// Get dashboard data
	response, err := h.service.GetDashboard(c, tenantCtx)
	if err != nil {
		respond.Error(c, err)
		return
	}

	respond.OK(c, response)
}

// roleVersion extracts the role version from the context.
func roleVersion(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}
