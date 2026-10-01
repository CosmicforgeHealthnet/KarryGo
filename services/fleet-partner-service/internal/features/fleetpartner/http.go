package fleetpartner

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

const (
	PermissionRead   = "fleet_partner_foundation.read"
	PermissionManage = "fleet_partner_foundation.manage"
)

type Handler struct {
	service    *Service
	authorizer *accesscontrol.Authorizer
	allowed    map[string]bool
}

func NewHandler(service *Service, authorizer *accesscontrol.Authorizer, allowedCallers []string) *Handler {
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		caller = strings.TrimSpace(caller)
		if caller != "" {
			allowed[caller] = true
		}
	}
	return &Handler{service: service, authorizer: authorizer, allowed: allowed}
}

func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.GET("/fleet-partner/operating-mode", h.getOperatingMode)
	group.PUT("/fleet-partner/operating-mode", h.updateOperatingMode)
	group.POST("/fleet-partner/compensation-plans", h.createCompensationPlan)
	group.POST("/fleet-partner/memberships/:membershipId/compensation-assignments", h.assignCompensationPlan)
}

func (h *Handler) RegisterInternalRoutes(group *gin.RouterGroup) {
	group.POST("/completed-work", h.recordCompletedWork)
	group.GET("/providers/:providerId/corporate-context", h.getCorporateContext)
}

func (h *Handler) authorize(c *gin.Context, permission string) (tenant.Context, bool) {
	t, err := tenant.Require(c.Request.Context())
	if err != nil {
		respond.Error(c, err)
		return tenant.Context{}, false
	}
	if h.authorizer != nil {
		if err := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID, roleVersionFrom(c), permission, accesscontrol.Target{}); err != nil {
			respond.Error(c, err)
			return tenant.Context{}, false
		}
	}
	return t, true
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 0
}

func (h *Handler) getOperatingMode(c *gin.Context) {
	t, ok := h.authorize(c, PermissionRead)
	if !ok {
		return
	}
	mode, version, err := h.service.ReadOperatingMode(c.Request.Context(), t.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"organizationId": t.OrganizationID, "operatingMode": mode, "version": version})
}

type updateOperatingModeRequest struct {
	OperatingMode   string `json:"operatingMode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) updateOperatingMode(c *gin.Context) {
	t, ok := h.authorize(c, PermissionManage)
	if !ok {
		return
	}
	var body updateOperatingModeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}
	version, err := h.service.UpdateOperatingMode(c.Request.Context(), t, body.OperatingMode, body.ExpectedVersion)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"organizationId": t.OrganizationID, "operatingMode": body.OperatingMode, "version": version})
}

type createPlanRequest struct {
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	Currency      string             `json:"currency"`
	Status        string             `json:"status"`
	EffectiveFrom time.Time          `json:"effectiveFrom"`
	EffectiveTo   *time.Time         `json:"effectiveTo"`
	Rules         []CompensationRule `json:"rules"`
}

func (h *Handler) createCompensationPlan(c *gin.Context) {
	t, ok := h.authorize(c, PermissionManage)
	if !ok {
		return
	}
	var body createPlanRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}
	plan, err := h.service.CreateCompensationPlan(c.Request.Context(), t, CompensationPlan{
		Name: body.Name, Description: body.Description, Currency: body.Currency,
		Status: body.Status, EffectiveFrom: body.EffectiveFrom.UTC(), EffectiveTo: body.EffectiveTo,
	}, body.Rules)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, plan)
}

type assignPlanRequest struct {
	CompensationPlanID      string     `json:"compensationPlanId"`
	CompensationPlanVersion int64      `json:"compensationPlanVersion"`
	ServiceContext          string     `json:"serviceContext"`
	Currency                string     `json:"currency"`
	EffectiveFrom           time.Time  `json:"effectiveFrom"`
	EffectiveTo             *time.Time `json:"effectiveTo"`
	Status                  string     `json:"status"`
	ApprovedBy              string     `json:"approvedBy"`
}

func (h *Handler) assignCompensationPlan(c *gin.Context) {
	t, ok := h.authorize(c, PermissionManage)
	if !ok {
		return
	}
	var body assignPlanRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}
	out, err := h.service.AssignCompensationPlan(c.Request.Context(), t, MembershipCompensationAssignment{
		MembershipID: c.Param("membershipId"), CompensationPlanID: body.CompensationPlanID,
		CompensationPlanVersion: body.CompensationPlanVersion, ServiceContext: body.ServiceContext,
		Currency: body.Currency, EffectiveFrom: body.EffectiveFrom.UTC(), EffectiveTo: body.EffectiveTo,
		Status: body.Status, ApprovedBy: body.ApprovedBy,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, out)
}

type completedWorkRequest struct {
	SourceService      string    `json:"sourceService"`
	SourceJobID        string    `json:"sourceJobId"`
	SourceAssignmentID string    `json:"sourceAssignmentId"`
	ProviderID         string    `json:"providerId"`
	MembershipID       string    `json:"membershipId"`
	OrganizationID     string    `json:"organizationId"`
	CompletedAt        time.Time `json:"completedAt"`
	Currency           string    `json:"currency"`
	EligibleBasisMinor int64     `json:"eligibleBasisMinor"`
	SourceVersion      int64     `json:"sourceVersion"`
	IdempotencyKey     string    `json:"idempotencyKey"`
	CorrelationID      string    `json:"correlationId"`
}

func (h *Handler) recordCompletedWork(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	var body completedWorkRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}
	record, resolution, instruction, err := h.service.RecordCompletedWork(c.Request.Context(), c.GetString(middleware.ServiceCallerKey), CompletedWorkInput{
		SourceService: body.SourceService, SourceJobID: body.SourceJobID, SourceAssignmentID: body.SourceAssignmentID,
		ProviderID: body.ProviderID, MembershipID: body.MembershipID, OrganizationID: body.OrganizationID,
		CompletedAt: body.CompletedAt.UTC(), Currency: body.Currency, EligibleBasisMinor: body.EligibleBasisMinor,
		SourceVersion: body.SourceVersion, IdempotencyKey: body.IdempotencyKey, CorrelationID: body.CorrelationID,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.JSON(c, http.StatusCreated, gin.H{"completedWork": record, "compensationResolution": resolution, "settlementInstruction": instruction})
}

func (h *Handler) getCorporateContext(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	service := strings.TrimSpace(c.Query("service"))
	if service == "" {
		respond.Error(c, apperrors.BadRequest("service query parameter is required.", nil))
		return
	}
	ctx, err := h.service.CorporateContext(c.Request.Context(), c.Param("providerId"), service)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, ctx)
}

func (h *Handler) requireAllowedCaller(c *gin.Context) bool {
	caller := c.GetString(middleware.ServiceCallerKey)
	if !h.allowed[caller] {
		respond.Error(c, apperrors.Forbidden("Service is not allowed to call this endpoint.", nil))
		return false
	}
	return true
}
