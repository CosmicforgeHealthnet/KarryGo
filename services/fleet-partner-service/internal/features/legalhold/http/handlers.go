package http

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the routes. Release is deliberately separate from
// manage (create): an organization may want different people authorized to
// place a hold than to lift one.
const (
	PermissionRead    = "privacy.legal_hold.read"
	PermissionManage  = "privacy.legal_hold.manage"
	PermissionRelease = "privacy.legal_hold.release"
)

// Handler serves the corporate legal-hold routes.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler. service may be nil in deployments where the
// pool is unavailable; routes then answer unavailable rather than panicking.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts the legal-hold surface.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.POST("/legal-holds", h.create)
	group.GET("/legal-holds", h.list)
	group.GET("/legal-holds/:holdId", h.get)
	group.POST("/legal-holds/:holdId/release", h.release)
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

func actorFrom(t tenant.Context) usecases.Actor {
	return usecases.Actor{OrganizationID: t.OrganizationID, ActorType: "organization_user", ActorID: t.OrganizationUserID}
}

// sessionIDFrom resolves the caller's session ID from the verified access
// token claims — the same claims ResolveTenant already consumed to build the
// tenant context, re-read here because tenant.Context itself does not carry
// SessionID. Needed so a step-up grant obtained for one session cannot be
// presented by a request authenticated with a different one.
func sessionIDFrom(c *gin.Context) string {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		return ""
	}
	return claims.SessionID
}

// HoldView is the allowlisted response shape.
type HoldView struct {
	ID                string `json:"id"`
	ScopeType         string `json:"scopeType"`
	ScopeReference    string `json:"scopeReference"`
	Reason            string `json:"reason"`
	Status            string `json:"status"`
	CreatedByActorID  string `json:"createdByActorId"`
	ApprovedByActorID string `json:"approvedByActorId,omitempty"`
	StepUpReference   string `json:"stepUpReference,omitempty"`
	ReleasedByActorID string `json:"releasedByActorId,omitempty"`
	ReleaseReason     string `json:"releaseReason,omitempty"`
	CreatedAt         string `json:"createdAt"`
	ReleasedAt        string `json:"releasedAt,omitempty"`
	Version           int64  `json:"version"`
}

func newHoldView(h models.Hold) HoldView {
	view := HoldView{
		ID: h.ID, ScopeType: h.ScopeType, ScopeReference: h.ScopeReference, Reason: h.Reason,
		Status: h.Status, CreatedByActorID: h.CreatedByActorID, ApprovedByActorID: h.ApprovedByActorID,
		StepUpReference: h.StepUpReference, ReleasedByActorID: h.ReleasedByActorID,
		ReleaseReason: h.ReleaseReason, CreatedAt: h.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		Version: h.Version,
	}
	if h.ReleasedAt != nil {
		view.ReleasedAt = h.ReleasedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	return view
}

type createHoldRequest struct {
	ScopeType       string `json:"scopeType"`
	ScopeReference  string `json:"scopeReference"`
	Reason          string `json:"reason"`
	ApprovedByID    string `json:"approvedByActorId"`
	StepUpReference string `json:"stepUpReference"`
}

func (h *Handler) create(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionManage) {
		return
	}
	if h.service == nil {
		respond.Error(c, apperrors.Unavailable("Legal hold management is not available.", nil))
		return
	}
	var body createHoldRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	hold, err := h.service.Create(c.Request.Context(), usecases.CreateInput{
		Actor: actorFrom(t), IdentityID: t.ActorID, SessionID: sessionIDFrom(c),
		ScopeType: body.ScopeType, ScopeReference: body.ScopeReference,
		Reason: body.Reason, ApprovedByID: body.ApprovedByID, StepUpReference: body.StepUpReference,
		RequestID: t.RequestID, CorrelationID: t.CorrelationID,
	})
	if err != nil {
		respond.Error(c, mapCreateError(err))
		return
	}
	respond.Created(c, newHoldView(hold))
}

func mapCreateError(err error) error {
	switch {
	case errors.Is(err, usecases.ErrScopeTypeInvalid):
		return apperrors.Validation("scopeType is not supported.", []apperrors.FieldViolation{
			{Field: "scopeType", Message: "must be one of the supported scope types"},
		})
	case errors.Is(err, usecases.ErrReasonRequired):
		return apperrors.Validation("reason is required.", []apperrors.FieldViolation{
			{Field: "reason", Message: "is required"},
		})
	default:
		return err
	}
}

func (h *Handler) list(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionRead) {
		return
	}
	if h.service == nil {
		respond.Error(c, apperrors.Unavailable("Legal hold management is not available.", nil))
		return
	}
	filter := repositories.ListFilter{
		OrganizationID: t.OrganizationID,
		ScopeType:      models.NormalizeScopeType(c.Query("scopeType")),
		ScopeReference: strings.TrimSpace(c.Query("scopeReference")),
		Status:         strings.ToUpper(strings.TrimSpace(c.Query("status"))),
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			filter.Limit = parsed
		}
	}
	holds, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]HoldView, 0, len(holds))
	for _, hold := range holds {
		views = append(views, newHoldView(hold))
	}
	respond.OK(c, views)
}

func (h *Handler) get(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionRead) {
		return
	}
	if h.service == nil {
		respond.Error(c, apperrors.Unavailable("Legal hold management is not available.", nil))
		return
	}
	hold, err := h.service.Get(c.Request.Context(), t.OrganizationID, c.Param("holdId"))
	if err != nil {
		respond.Error(c, apperrors.NotFound("Legal hold not found.", err))
		return
	}
	respond.OK(c, newHoldView(hold))
}

type releaseHoldRequest struct {
	ExpectedVersion *int64 `json:"expectedVersion"`
	ReleaseReason   string `json:"releaseReason"`
	StepUpReference string `json:"stepUpReference"`
}

func (h *Handler) release(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionRelease) {
		return
	}
	if h.service == nil {
		respond.Error(c, apperrors.Unavailable("Legal hold management is not available.", nil))
		return
	}
	var body releaseHoldRequest
	_ = c.ShouldBindJSON(&body)
	version, err := concurrency.ExpectedVersion(c.GetHeader("If-Match"), body.ExpectedVersion)
	if err != nil {
		respond.Error(c, err)
		return
	}
	hold, err := h.service.Release(c.Request.Context(), usecases.ReleaseInput{
		Actor: actorFrom(t), IdentityID: t.ActorID, SessionID: sessionIDFrom(c),
		HoldID: c.Param("holdId"), ExpectedVersion: version,
		ReleaseReason: body.ReleaseReason, StepUpReference: body.StepUpReference,
		RequestID: t.RequestID, CorrelationID: t.CorrelationID,
	})
	if err != nil {
		respond.Error(c, mapReleaseError(err))
		return
	}
	respond.OK(c, newHoldView(hold))
}

func mapReleaseError(err error) error {
	switch {
	case errors.Is(err, repositories.ErrVersionMismatch):
		return apperrors.Conflict("The legal hold was modified by someone else. Reload and retry.", err)
	case errors.Is(err, repositories.ErrAlreadyReleased):
		return apperrors.Conflict("This legal hold has already been released.", err)
	case errors.Is(err, repositories.ErrNotFound):
		return apperrors.NotFound("Legal hold not found.", err)
	default:
		return err
	}
}
