// Package http serves the Section 23 communication routes: threads, messages
// and notification rules.
package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenantcmd"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Route describes one tenant route.
type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

// TenantRoutes lists the communication tenant routes.
func TenantRoutes() []Route {
	return append(coreRoutes(), preferenceRoutes()...)
}

func coreRoutes() []Route {
	return []Route{
		{"GET", "/communications/threads", "listThreads", "communications.read", "COMMUNICATIONS_ENABLED", "corporate", false, false}, {"POST", "/communications/threads", "createThread", "communications.manage", "COMMUNICATIONS_ENABLED", "corporate", true, false}, {"GET", "/communications/threads/:threadId", "getThread", "communications.read", "COMMUNICATIONS_ENABLED", "corporate", false, false}, {"PATCH", "/communications/threads/:threadId", "updateThread", "communications.manage", "COMMUNICATIONS_ENABLED", "corporate", true, true}, {"GET", "/communications/threads/:threadId/messages", "listMessages", "communications.read", "COMMUNICATIONS_ENABLED", "corporate", false, false}, {"POST", "/communications/threads/:threadId/messages", "sendMessage", "communications.send", "COMMUNICATIONS_ENABLED", "corporate", true, false}, {"POST", "/communications/threads/:threadId/close", "closeThread", "communications.manage", "COMMUNICATIONS_ENABLED", "corporate", true, true},
		{"GET", "/notification-rules", "listNotificationRules", "notification_rules.read", "NOTIFICATION_RULES_ENABLED", "corporate", false, false}, {"PUT", "/notification-rules", "saveNotificationRule", "notification_rules.manage", "NOTIFICATION_RULES_ENABLED", "corporate", true, true},
	}
}

// FullPath is the mounted path.
func (r Route) FullPath() string { return "/fleet-partner/v1" + r.Path }

// Handler serves the communication tenant routes.
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

type versionBody struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := tenantcmd.Gate(c, h.a, r.Permission)
	if !ok {
		return
	}
	run := func(key string) (any, error) {
		if v, err, handled := h.preferenceOps(c, t, r); handled {
			return v, err
		}
		switch r.OperationID {
		case "listThreads":
			return h.s.ListThreads(c, t)
		case "createThread":
			var in models.ThreadInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreateThread(c, t, in)
		case "getThread":
			return h.s.GetThread(c, t, c.Param("threadId"))
		case "updateThread":
			var in models.ThreadInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateThread(c, t, c.Param("threadId"), in)
		case "listMessages":
			return h.s.ListMessages(c, t, c.Param("threadId"))
		case "sendMessage":
			var in models.MessageInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.SendMessage(c, t, c.Param("threadId"), key, in)
		case "closeThread":
			var b versionBody
			if !tenantcmd.Bind(c, &b) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CloseThread(c, t, c.Param("threadId"), tenantcmd.ExpectedVersion(c, b.ExpectedVersion))
		case "listNotificationRules":
			return h.s.ListNotificationRules(c, t)
		case "saveNotificationRule":
			var in models.NotificationRuleInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			return h.s.SaveNotificationRule(c, t, in)
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
