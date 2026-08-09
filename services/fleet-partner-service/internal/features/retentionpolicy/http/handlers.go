package http

import (
	"errors"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the routes.
const (
	PermissionRead   = "privacy.retention.read"
	PermissionManage = "privacy.retention.manage"
)

// Handler serves the corporate retention-policy routes.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts the retention-policy surface.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.GET("/retention-policies", h.list)
	group.GET("/retention-policies/:class", h.get)
	group.PUT("/retention-policies/:class", h.update)
}

func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

func (h *Handler) authorize(c *gin.Context, t tenant.Context, permission string) bool {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID,
		roleVersionFrom(c), permission, accesscontrol.Target{}); err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}

// PolicyView is the wire response shape.
type PolicyView struct {
	RetentionClass      string `json:"retentionClass"`
	RetentionDays       int    `json:"retentionDays"`
	PlatformMinimumDays int    `json:"platformMinimumDays"`
	Version             int64  `json:"version"`
}

func newPolicyView(p models.Policy) PolicyView {
	return PolicyView{
		RetentionClass: p.RetentionClass, RetentionDays: p.RetentionDays,
		PlatformMinimumDays: models.PlatformMinimumDays[p.RetentionClass], Version: p.Version,
	}
}

func (h *Handler) list(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionRead) {
		return
	}
	if h.service == nil {
		respond.Error(c, apperrors.Unavailable("Retention policy is not available.", nil))
		return
	}
	policies, err := h.service.List(c.Request.Context(), t.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]PolicyView, 0, len(policies))
	for _, p := range policies {
		views = append(views, newPolicyView(p))
	}
	respond.OK(c, views)
}

func (h *Handler) get(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionRead) {
		return
	}
	if h.service == nil {
		respond.Error(c, apperrors.Unavailable("Retention policy is not available.", nil))
		return
	}
	policy, err := h.service.Get(c.Request.Context(), t.OrganizationID, c.Param("class"))
	if err != nil {
		respond.Error(c, mapError(err))
		return
	}
	respond.OK(c, newPolicyView(policy))
}

type updatePolicyRequest struct {
	ExpectedVersion *int64 `json:"expectedVersion"`
	RetentionDays   int    `json:"retentionDays"`
}

func (h *Handler) update(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionManage) {
		return
	}
	if h.service == nil {
		respond.Error(c, apperrors.Unavailable("Retention policy is not available.", nil))
		return
	}
	var body updatePolicyRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	version, err := concurrency.ExpectedVersion(c.GetHeader("If-Match"), body.ExpectedVersion)
	if err != nil {
		respond.Error(c, err)
		return
	}
	policy, err := h.service.Update(c.Request.Context(), usecases.UpdateInput{
		Actor:           usecases.Actor{OrganizationID: t.OrganizationID, ActorType: "organization_user", ActorID: t.OrganizationUserID},
		RetentionClass:  c.Param("class"),
		RetentionDays:   body.RetentionDays,
		ExpectedVersion: version,
		RequestID:       t.RequestID,
		CorrelationID:   t.CorrelationID,
	})
	if err != nil {
		respond.Error(c, mapError(err))
		return
	}
	respond.OK(c, newPolicyView(policy))
}

func mapError(err error) error {
	switch {
	case errors.Is(err, usecases.ErrClassInvalid):
		return apperrors.Validation("retentionClass is not supported.", []apperrors.FieldViolation{
			{Field: "retentionClass", Message: "must be a recognized retention class"},
		})
	case errors.Is(err, usecases.ErrBelowPlatformMinimum):
		return apperrors.Validation("retentionDays is below the platform minimum for this class.", []apperrors.FieldViolation{
			{Field: "retentionDays", Message: "must be at least the platform minimum for this class"},
		})
	case errors.Is(err, repositories.ErrVersionMismatch):
		return apperrors.Conflict("The retention policy was modified by someone else, or already exists. Reload and retry.", err)
	default:
		return err
	}
}
