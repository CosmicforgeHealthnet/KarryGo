// Package http serves the Section 28/56 governance routes.
package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenantcmd"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"
)

// Route describes one tenant route.
type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

// FullPath is the mounted path.
func (r Route) FullPath() string { return "/fleet-partner/v1" + r.Path }

// TenantRoutes lists the governance tenant routes.
func TenantRoutes() []Route {
	const p = "PRIVACY_REQUESTS_ENABLED"
	return []Route{
		{"GET", "/security-events", "listSecurityEvents", "security_events.read", "SECURITY_EVENTS_ENABLED", "corporate", false, false},
		{"GET", "/security/sessions", "listOrganizationSessions", "security.manage", "SECURITY_SESSIONS_ENABLED", "corporate", false, false},
		{"POST", "/security/sessions/:sessionId/revoke", "revokeOrganizationSession", "security.manage", "SECURITY_SESSIONS_ENABLED", "corporate", true, false},
		{"GET", "/privacy-requests", "listPrivacyRequests", "privacy.manage", p, "corporate", false, false},
		{"POST", "/privacy-requests", "createPrivacyRequest", "privacy.manage", p, "corporate", true, false},
		{"GET", "/privacy-requests/:requestId", "getPrivacyRequest", "privacy.manage", p, "corporate", false, false},
		{"POST", "/privacy-requests/:requestId/decisions", "decidePrivacyRequest", "privacy.decide", p, "corporate", true, true},
		{"POST", "/privacy-requests/:requestId/complete", "completePrivacyRequest", "privacy.manage", p, "corporate", true, true},
	}
}

// Service is the governance use-case slice the handler calls.
type Service interface {
	SecurityEvents(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	ListOrganizationSessions(ctx context.Context, t tenant.Context, currentSessionID string) ([]models.OrganizationSession, error)
	RevokeOrganizationSession(ctx context.Context, t tenant.Context, sessionID string, in models.SessionRevokeInput) (models.OrganizationSession, error)
	SubmitPrivacyRequest(ctx context.Context, t tenant.Context, in models.PrivacyRequestInput) (map[string]any, error)
	GetPrivacyRequest(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	ListPrivacyRequests(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	DecidePrivacyRequest(ctx context.Context, t tenant.Context, id string, in models.PrivacyDecisionInput) (map[string]any, error)
	CompletePrivacyRequest(ctx context.Context, t tenant.Context, id string, in models.PrivacyDecisionInput) (map[string]any, error)
}

// Handler serves the governance routes.
type Handler struct {
	s    Service
	a    tenantcmd.ScopedAuthorizer
	idem *idempotency.Store
}

// NewHandler builds the handler.
func NewHandler(s Service, a tenantcmd.ScopedAuthorizer, i *idempotency.Store) *Handler {
	return &Handler{s: s, a: a, idem: i}
}

// TenantRoutes lists the routes this handler serves.
func (h *Handler) TenantRoutes() []Route { return TenantRoutes() }

// RegisterTenantRoutes mounts the routes.
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	for _, r := range TenantRoutes() {
		r := r
		g.Handle(r.Method, r.Path, func(c *gin.Context) { h.handle(c, r) })
	}
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := tenantcmd.Gate(c, h.a, r.Permission)
	if !ok {
		return
	}
	run := func(key string) (any, error) {
		switch r.OperationID {
		case "listSecurityEvents":
			return h.s.SecurityEvents(c, t)
		case "listOrganizationSessions":
			current := ""
			if claims, ok := auth.ClaimsFromContext(c); ok {
				current = claims.SessionID
			}
			return h.s.ListOrganizationSessions(c, t, current)
		case "revokeOrganizationSession":
			var in models.SessionRevokeInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.RevokeOrganizationSession(c, t, c.Param("sessionId"), in)
		case "listPrivacyRequests":
			return h.s.ListPrivacyRequests(c, t)
		case "createPrivacyRequest":
			var in models.PrivacyRequestInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.SubmitPrivacyRequest(c, t, in)
		case "getPrivacyRequest":
			return h.s.GetPrivacyRequest(c, t, c.Param("requestId"))
		case "decidePrivacyRequest", "completePrivacyRequest":
			var in models.PrivacyDecisionInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			if r.OperationID == "decidePrivacyRequest" {
				return h.s.DecidePrivacyRequest(c, t, c.Param("requestId"), in)
			}
			return h.s.CompletePrivacyRequest(c, t, c.Param("requestId"), in)
		}
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	if r.Method == http.MethodGet {
		v, err := run("")
		if err != nil {
			respond.Error(c, err)
			return
		}
		respond.JSON(c, http.StatusOK, v)
		return
	}
	tenantcmd.Command(c, h.idem, t.OrganizationID, r.Method, r.Path, strings.HasPrefix(r.OperationID, "create"), run)
}
