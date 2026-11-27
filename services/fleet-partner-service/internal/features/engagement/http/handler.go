// Package http serves the Section 23 engagement routes: support case
// projections, quality cases, SLA measurements and escalations, plus the
// internal support-event and SLA-measurement ingress.
package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
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

// TenantRoutes lists the engagement tenant routes.
func TenantRoutes() []Route {
	return append(coreRoutes(), exceptionRoutes()...)
}

func coreRoutes() []Route {
	return []Route{
		{"GET", "/support/cases", "listSupportCases", "support.read", "SUPPORT_PROJECTIONS_ENABLED", "corporate", false, false}, {"POST", "/support/cases", "createSupportCase", "support.create", "SUPPORT_PROJECTIONS_ENABLED", "corporate", true, false}, {"GET", "/support/cases/:caseId", "getSupportCase", "support.read", "SUPPORT_PROJECTIONS_ENABLED", "corporate", false, false},
		// Spec 59.10 POST /support-cases/{id}/messages, under the existing
		// /support/cases family.
		{"POST", "/support/cases/:caseId/messages", "createSupportMessage", "support.manage", "SUPPORT_PROJECTIONS_ENABLED", "corporate", true, false},
		{"GET", "/quality/cases", "listQualityCases", "quality.read", "QUALITY_CASES_ENABLED", "corporate", false, false}, {"POST", "/quality/cases", "createQualityCase", "quality.manage", "QUALITY_CASES_ENABLED", "corporate", true, false}, {"GET", "/quality/cases/:qualityCaseId", "getQualityCase", "quality.read", "QUALITY_CASES_ENABLED", "corporate", false, false}, {"PATCH", "/quality/cases/:qualityCaseId", "updateQualityCase", "quality.manage", "QUALITY_CASES_ENABLED", "corporate", true, true}, {"POST", "/quality/cases/:qualityCaseId/resolve", "resolveQualityCase", "quality.resolve", "QUALITY_CASES_ENABLED", "corporate", true, true},
		{"GET", "/sla/measurements", "listMeasurements", "sla.read", "SLA_MEASUREMENTS_ENABLED", "corporate", false, false}, {"GET", "/sla/measurements/:measurementId", "getMeasurement", "sla.read", "SLA_MEASUREMENTS_ENABLED", "corporate", false, false},
		{"GET", "/escalations", "listEscalations", "escalations.read", "ESCALATIONS_ENABLED", "corporate", false, false}, {"GET", "/escalations/:escalationId", "getEscalation", "escalations.read", "ESCALATIONS_ENABLED", "corporate", false, false}, {"POST", "/escalations/:escalationId/acknowledge", "acknowledgeEscalation", "escalations.acknowledge", "ESCALATIONS_ENABLED", "corporate", true, true}, {"POST", "/escalations/:escalationId/resolve", "resolveEscalation", "escalations.resolve", "ESCALATIONS_ENABLED", "corporate", true, true},
	}
}

// FullPath is the mounted path.
func (r Route) FullPath() string { return "/fleet-partner/v1" + r.Path }

// Handler serves the engagement tenant routes.
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
		if v, err, handled := h.exceptionOps(c, t, r); handled {
			return v, err
		}
		switch r.OperationID {
		case "listSupportCases":
			return h.s.ListSupportCases(c, t)
		case "createSupportCase":
			var in models.SupportCreateInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreateSupportCase(c, t, key, in)
		case "getSupportCase":
			return h.s.GetSupportCase(c, t, c.Param("caseId"))
		case "createSupportMessage":
			var in models.SupportMessageInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.PostSupportMessage(c, t, c.Param("caseId"), in)
		case "listQualityCases":
			return h.s.ListQualityCases(c, t)
		case "createQualityCase":
			var in models.QualityInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreateQualityCase(c, t, in)
		case "getQualityCase":
			return h.s.GetQualityCase(c, t, c.Param("qualityCaseId"))
		case "updateQualityCase", "resolveQualityCase":
			var in models.QualityInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateQualityCase(c, t, c.Param("qualityCaseId"), in, r.OperationID == "resolveQualityCase")
		case "listMeasurements":
			return h.s.ListMeasurements(c, t)
		case "getMeasurement":
			return h.s.GetMeasurement(c, t, c.Param("measurementId"))
		case "listEscalations":
			return h.s.ListEscalations(c, t)
		case "getEscalation":
			return h.s.GetEscalation(c, t, c.Param("escalationId"))
		case "acknowledgeEscalation", "resolveEscalation":
			var b versionBody
			if !tenantcmd.Bind(c, &b) {
				return nil, tenantcmd.ErrBound
			}
			cmd := "acknowledge"
			if r.OperationID == "resolveEscalation" {
				cmd = "resolve"
			}
			return h.s.EscalationCommand(c, t, c.Param("escalationId"), cmd, tenantcmd.ExpectedVersion(c, b.ExpectedVersion))
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

// InternalHandler serves the internal support-event and SLA ingress.
type InternalHandler struct{ s IngressService }

// NewInternalHandler builds the internal handler.
func NewInternalHandler(s IngressService) *InternalHandler { return &InternalHandler{s: s} }

// RegisterInternalRoutes mounts the internal routes.
func (h *InternalHandler) RegisterInternalRoutes(g *gin.RouterGroup) {
	g.POST("/support/events", h.supportEvent)
	g.POST("/sla/measurements", h.measurement)
}

func (h *InternalHandler) supportEvent(c *gin.Context) {
	var in models.SupportEvent
	if !tenantcmd.Bind(c, &in) {
		return
	}
	in.OrganizationID = c.GetHeader("X-Organization-ID")
	if err := h.s.ApplySupportEvent(c, in); err != nil {
		respond.Error(c, err)
		return
	}
	respond.JSON(c, http.StatusAccepted, gin.H{"accepted": true})
}

func (h *InternalHandler) measurement(c *gin.Context) {
	var in models.MeasurementInput
	if !tenantcmd.Bind(c, &in) {
		return
	}
	in.OrganizationID = c.GetHeader("X-Organization-ID")
	v, err := h.s.RecordMeasurement(c, in)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, v)
}
