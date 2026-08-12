package engagement

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func TenantRoutes() []Route {
	return []Route{
		{"GET", "/communications/threads", "listThreads", "communications.read", "COMMUNICATIONS_ENABLED", "corporate", false, false}, {"POST", "/communications/threads", "createThread", "communications.manage", "COMMUNICATIONS_ENABLED", "corporate", true, false}, {"GET", "/communications/threads/:threadId", "getThread", "communications.read", "COMMUNICATIONS_ENABLED", "corporate", false, false}, {"PATCH", "/communications/threads/:threadId", "updateThread", "communications.manage", "COMMUNICATIONS_ENABLED", "corporate", true, true}, {"GET", "/communications/threads/:threadId/messages", "listMessages", "communications.read", "COMMUNICATIONS_ENABLED", "corporate", false, false}, {"POST", "/communications/threads/:threadId/messages", "sendMessage", "communications.send", "COMMUNICATIONS_ENABLED", "corporate", true, false}, {"POST", "/communications/threads/:threadId/close", "closeThread", "communications.manage", "COMMUNICATIONS_ENABLED", "corporate", true, true},
		{"GET", "/notification-rules", "listNotificationRules", "notification_rules.read", "NOTIFICATION_RULES_ENABLED", "corporate", false, false}, {"PUT", "/notification-rules", "saveNotificationRule", "notification_rules.manage", "NOTIFICATION_RULES_ENABLED", "corporate", true, true},
		{"GET", "/support/cases", "listSupportCases", "support.read", "SUPPORT_PROJECTIONS_ENABLED", "corporate", false, false}, {"POST", "/support/cases", "createSupportCase", "support.create", "SUPPORT_PROJECTIONS_ENABLED", "corporate", true, false}, {"GET", "/support/cases/:caseId", "getSupportCase", "support.read", "SUPPORT_PROJECTIONS_ENABLED", "corporate", false, false},
		{"GET", "/quality/cases", "listQualityCases", "quality.read", "QUALITY_CASES_ENABLED", "corporate", false, false}, {"POST", "/quality/cases", "createQualityCase", "quality.manage", "QUALITY_CASES_ENABLED", "corporate", true, false}, {"GET", "/quality/cases/:qualityCaseId", "getQualityCase", "quality.read", "QUALITY_CASES_ENABLED", "corporate", false, false}, {"PATCH", "/quality/cases/:qualityCaseId", "updateQualityCase", "quality.manage", "QUALITY_CASES_ENABLED", "corporate", true, true}, {"POST", "/quality/cases/:qualityCaseId/resolve", "resolveQualityCase", "quality.resolve", "QUALITY_CASES_ENABLED", "corporate", true, true},
		{"GET", "/sla/measurements", "listMeasurements", "sla.read", "SLA_MEASUREMENTS_ENABLED", "corporate", false, false}, {"GET", "/sla/measurements/:measurementId", "getMeasurement", "sla.read", "SLA_MEASUREMENTS_ENABLED", "corporate", false, false},
		{"GET", "/escalations", "listEscalations", "escalations.read", "ESCALATIONS_ENABLED", "corporate", false, false}, {"GET", "/escalations/:escalationId", "getEscalation", "escalations.read", "ESCALATIONS_ENABLED", "corporate", false, false}, {"POST", "/escalations/:escalationId/acknowledge", "acknowledgeEscalation", "escalations.acknowledge", "ESCALATIONS_ENABLED", "corporate", true, true}, {"POST", "/escalations/:escalationId/resolve", "resolveEscalation", "escalations.resolve", "ESCALATIONS_ENABLED", "corporate", true, true},
	}
}
func (r Route) FullPath() string { return "/fleet-partner/v1" + r.Path }

type Handler struct {
	s    *Service
	a    *accesscontrol.Authorizer
	idem *idempotency.Store
}

func NewHandler(s *Service, a *accesscontrol.Authorizer, i *idempotency.Store) *Handler {
	return &Handler{s: s, a: a, idem: i}
}
func (h *Handler) TenantRoutes() []Route { return TenantRoutes() }
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
	if err := h.a.Require(c, t.OrganizationID, t.OrganizationUserID, rv, p, accesscontrol.Target{BranchID: c.Query("branchId")}); err != nil {
		respond.Error(c, err)
		return t, false
	}
	return t, true
}
func bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", err))
		return false
	}
	return true
}
func expected(c *gin.Context, v int64) int64 {
	if v > 0 {
		return v
	}
	n, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return n
}

type versionBody struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

var errBound = errors.New("request already rendered")

func (h *Handler) command(c *gin.Context, t tenant.Context, r Route, run func(string) (any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if h.idem == nil {
		respond.Error(c, apperrors.Internal("Idempotency store is unavailable.", nil))
		return
	}
	raw, err := c.GetRawData()
	if err != nil || (!json.Valid(raw) && len(bytes.TrimSpace(raw)) > 0) {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", err))
		return
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte(`{}`)
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	ep := r.Method + " " + r.Path
	rec, err := h.idem.Begin(c, t.OrganizationID, ep, key, idempotency.Fingerprint(r.Method, ep, raw))
	if err != nil {
		respond.Error(c, idempotency.AsAppError(err))
		return
	}
	if rec != nil {
		c.Data(rec.ResponseStatus, "application/json; charset=utf-8", rec.ResponseBody)
		return
	}
	value, runErr := run(key)
	if errors.Is(runErr, errBound) {
		return
	}
	status := http.StatusOK
	if strings.HasPrefix(r.OperationID, "create") {
		status = http.StatusCreated
	}
	var payload []byte
	if runErr != nil {
		ae := apperrors.From(runErr)
		status = ae.Status
		payload, _ = json.Marshal(gin.H{"error": respond.ErrorBody{Code: ae.Code, Message: ae.Message, Details: []apperrors.FieldViolation{}, RequestID: httpx.GetRequestID(c)}})
	} else {
		payload, _ = json.Marshal(gin.H{"data": value, "meta": gin.H{"requestId": httpx.GetRequestID(c)}})
	}
	if err := h.idem.Complete(c, t.OrganizationID, ep, key, status, payload); err != nil {
		respond.Error(c, err)
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}
func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	run := func(key string) (any, error) {
		switch r.OperationID {
		case "listThreads":
			return h.s.ListThreads(c, t)
		case "createThread":
			var in ThreadInput
			if !bind(c, &in) {
				return nil, errBound
			}
			return h.s.CreateThread(c, t, in)
		case "getThread":
			return h.s.GetThread(c, t, c.Param("threadId"))
		case "updateThread":
			var in ThreadInput
			if !bind(c, &in) {
				return nil, errBound
			}
			in.ExpectedVersion = expected(c, in.ExpectedVersion)
			return h.s.UpdateThread(c, t, c.Param("threadId"), in)
		case "listMessages":
			return h.s.ListMessages(c, t, c.Param("threadId"))
		case "sendMessage":
			var in MessageInput
			if !bind(c, &in) {
				return nil, errBound
			}
			return h.s.SendMessage(c, t, c.Param("threadId"), key, in)
		case "closeThread":
			var b versionBody
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.CloseThread(c, t, c.Param("threadId"), expected(c, b.ExpectedVersion))
		case "listNotificationRules":
			return h.s.ListNotificationRules(c, t)
		case "saveNotificationRule":
			var in NotificationRuleInput
			if !bind(c, &in) {
				return nil, errBound
			}
			in.ExpectedVersion = expected(c, in.ExpectedVersion)
			return h.s.SaveNotificationRule(c, t, in)
		case "listSupportCases":
			return h.s.ListSupportCases(c, t)
		case "createSupportCase":
			var in SupportCreateInput
			if !bind(c, &in) {
				return nil, errBound
			}
			return h.s.CreateSupportCase(c, t, key, in)
		case "getSupportCase":
			return h.s.GetSupportCase(c, t, c.Param("caseId"))
		case "listQualityCases":
			return h.s.ListQualityCases(c, t)
		case "createQualityCase":
			var in QualityInput
			if !bind(c, &in) {
				return nil, errBound
			}
			return h.s.CreateQualityCase(c, t, in)
		case "getQualityCase":
			return h.s.GetQualityCase(c, t, c.Param("qualityCaseId"))
		case "updateQualityCase", "resolveQualityCase":
			var in QualityInput
			if !bind(c, &in) {
				return nil, errBound
			}
			in.ExpectedVersion = expected(c, in.ExpectedVersion)
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
			if !bind(c, &b) {
				return nil, errBound
			}
			cmd := "acknowledge"
			if r.OperationID == "resolveEscalation" {
				cmd = "resolve"
			}
			return h.s.EscalationCommand(c, t, c.Param("escalationId"), cmd, expected(c, b.ExpectedVersion))
		}
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	if r.Method == http.MethodGet {
		v, err := run("")
		if err != nil {
			respond.Error(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": v, "meta": gin.H{"requestId": httpx.GetRequestID(c)}})
		return
	}
	h.command(c, t, r, run)
}

type InternalHandler struct{ s *Service }

func NewInternalHandler(s *Service) *InternalHandler { return &InternalHandler{s: s} }
func (h *InternalHandler) RegisterInternalRoutes(g *gin.RouterGroup) {
	g.POST("/support/events", h.supportEvent)
	g.POST("/sla/measurements", h.measurement)
}
func (h *InternalHandler) supportEvent(c *gin.Context) {
	var in SupportEvent
	if !bind(c, &in) {
		return
	}
	in.OrganizationID = c.GetHeader("X-Organization-ID")
	if err := h.s.ApplySupportEvent(c, in); err != nil {
		respond.Error(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"accepted": true}})
}
func (h *InternalHandler) measurement(c *gin.Context) {
	var in MeasurementInput
	if !bind(c, &in) {
		return
	}
	in.OrganizationID = c.GetHeader("X-Organization-ID")
	v, err := h.s.RecordMeasurement(c, in)
	if err != nil {
		respond.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": v})
}
