package synchronization

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	PermissionRead      = "synchronization.read"
	PermissionReconcile = "synchronization.reconcile"
	PermissionReplay    = "synchronization.replay"
)

type Route struct{ Method, Path, OperationID, Permission string }

func TenantRoutes() []Route {
	return []Route{{"GET", "/synchronization/projections", "listSynchronizationProjections", PermissionRead}, {"GET", "/synchronization/reconciliation-cases", "listSynchronizationCases", PermissionRead}, {"GET", "/synchronization/reconciliation-cases/:caseId", "getSynchronizationCase", PermissionRead}, {"POST", "/synchronization/reconciliation-cases/:caseId/reconcile", "reconcileSynchronizationCase", PermissionReconcile}, {"GET", "/synchronization/dead-letters", "listSynchronizationDeadLetters", PermissionRead}, {"GET", "/synchronization/dead-letters/:deadLetterId", "getSynchronizationDeadLetter", PermissionRead}, {"POST", "/synchronization/dead-letters/:deadLetterId/replay", "replaySynchronizationDeadLetter", PermissionReplay}}
}

type Handler struct {
	s *Service
	a *accesscontrol.Authorizer
}

func NewHandler(s *Service, a *accesscontrol.Authorizer) *Handler { return &Handler{s: s, a: a} }
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	for _, r := range TenantRoutes() {
		r := r
		g.Handle(r.Method, r.Path, func(c *gin.Context) { h.handle(c, r) })
	}
}
func (h *Handler) gate(c *gin.Context, p string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok || h.a == nil {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return t, false
	}
	rv := int64(1)
	if x, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		rv, _ = x.(int64)
	}
	if err := h.a.Require(c, t.OrganizationID, t.OrganizationUserID, rv, p, accesscontrol.Target{}); err != nil {
		respond.Error(c, err)
		return t, false
	}
	return t, true
}
func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	var value any
	var err error
	switch r.OperationID {
	case "listSynchronizationProjections":
		value, err = h.s.ListProjections(c, t.OrganizationID)
	case "listSynchronizationCases":
		value, err = h.s.ListCases(c, t.OrganizationID)
	case "getSynchronizationCase":
		value, err = h.s.GetCase(c, t.OrganizationID, c.Param("caseId"))
	case "reconcileSynchronizationCase":
		err = h.s.Reconcile(c, t.OrganizationID, c.Param("caseId"))
		value = gin.H{"status": "REQUEUED"}
	case "listSynchronizationDeadLetters":
		value, err = h.s.ListDeadLetters(c, t.OrganizationID)
	case "getSynchronizationDeadLetter":
		value, err = h.s.GetDeadLetter(c, t.OrganizationID, c.Param("deadLetterId"))
	case "replaySynchronizationDeadLetter":
		err = h.s.Replay(c, t.OrganizationID, c.Param("deadLetterId"))
		value = gin.H{"status": "REPLAYED"}
	}
	if err != nil {
		respond.Error(c, apperrors.NotFound("Synchronization record not found.", err))
		return
	}
	if r.Method == http.MethodPost {
		respond.JSON(c, http.StatusAccepted, value)
		return
	}
	respond.OK(c, value)
}
