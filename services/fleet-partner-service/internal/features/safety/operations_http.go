package safety

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

type OperationsRoute struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func OperationsTenantRoutes() []OperationsRoute {
	return []OperationsRoute{
		{"GET", "/safety/reports", "listSafetyReports", "safety.read", "SAFETY_REPORTS_ENABLED", "corporate", false, false}, {"POST", "/safety/reports", "createSafetyReport", "safety.report", "SAFETY_REPORTS_ENABLED", "corporate", true, false}, {"GET", "/safety/reports/:reportId", "getSafetyReport", "safety.read", "SAFETY_REPORTS_ENABLED", "corporate", false, false}, {"PATCH", "/safety/reports/:reportId", "updateSafetyReport", "safety.manage", "SAFETY_REPORTS_ENABLED", "corporate", false, true}, {"POST", "/safety/reports/:reportId/resolve", "resolveSafetyReport", "safety.manage", "SAFETY_REPORTS_ENABLED", "corporate", true, true},
		{"GET", "/incidents", "listIncidents", "incidents.read", "INCIDENTS_ENABLED", "corporate", false, false}, {"POST", "/incidents", "createIncident", "incidents.manage", "INCIDENTS_ENABLED", "corporate", true, false}, {"GET", "/incidents/:incidentId", "getIncident", "incidents.read", "INCIDENTS_ENABLED", "corporate", false, false}, {"PATCH", "/incidents/:incidentId", "updateIncident", "incidents.manage", "INCIDENTS_ENABLED", "corporate", false, true}, {"POST", "/incidents/:incidentId/evidence", "addIncidentEvidence", "incidents.attach_evidence", "INCIDENTS_ENABLED", "corporate", true, false}, {"POST", "/incidents/:incidentId/corrective-actions", "createCorrectiveAction", "incidents.manage", "INCIDENTS_ENABLED", "corporate", true, false},
		{"POST", "/corrective-actions/:actionId/complete", "completeCorrectiveAction", "incidents.manage", "INCIDENTS_ENABLED", "corporate", true, true}, {"POST", "/corrective-actions/:actionId/verify", "verifyCorrectiveAction", "incidents.verify_actions", "INCIDENTS_ENABLED", "corporate", true, true}}
}
func (r OperationsRoute) FullPath() string { return "/fleet-partner/v1" + r.Path }

type OperationsHandler struct {
	s    *OperationsService
	a    *accesscontrol.Authorizer
	idem *idempotency.Store
}

func NewOperationsHandler(s *OperationsService, a *accesscontrol.Authorizer, i *idempotency.Store) *OperationsHandler {
	return &OperationsHandler{s: s, a: a, idem: i}
}
func (h *OperationsHandler) TenantRoutes() []OperationsRoute { return OperationsTenantRoutes() }
func (h *OperationsHandler) RegisterTenantRoutes(g *gin.RouterGroup) {
	for _, r := range OperationsTenantRoutes() {
		r := r
		g.Handle(r.Method, r.Path, func(c *gin.Context) { h.handle(c, r) })
	}
}
func (h *OperationsHandler) gate(c *gin.Context, p string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok || h.a == nil {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return t, false
	}
	rv := int64(1)
	if x, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		rv, _ = x.(int64)
	}
	if e := h.a.Require(c, t.OrganizationID, t.OrganizationUserID, rv, p, accesscontrol.Target{BranchID: c.Query("branchId")}); e != nil {
		respond.Error(c, e)
		return t, false
	}
	return t, true
}
func bindSafety(c *gin.Context, v any) bool {
	if e := c.ShouldBindJSON(v); e != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", e))
		return false
	}
	return true
}
func safetyVersion(c *gin.Context, v int64) int64 {
	if v > 0 {
		return v
	}
	n, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return n
}
func (h *OperationsHandler) command(c *gin.Context, t tenant.Context, r OperationsRoute, run func() (any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	raw, e := c.GetRawData()
	if e != nil || !json.Valid(raw) {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", e))
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	ep := r.Method + " " + r.Path
	rec, e := h.idem.Begin(c, t.OrganizationID, ep, key, idempotency.Fingerprint(r.Method, ep, raw))
	if e != nil {
		respond.Error(c, idempotency.AsAppError(e))
		return
	}
	if rec != nil {
		c.Data(rec.ResponseStatus, "application/json", rec.ResponseBody)
		return
	}
	v, e := run()
	if e != nil && e.Error() == "bound" {
		return
	}
	status := http.StatusOK
	if strings.HasPrefix(r.OperationID, "create") || strings.HasPrefix(r.OperationID, "add") {
		status = http.StatusCreated
	}
	var payload []byte
	if e != nil {
		ae := apperrors.From(e)
		status = ae.Status
		payload, _ = json.Marshal(gin.H{"error": respond.ErrorBody{Code: ae.Code, Message: ae.Message, Details: []apperrors.FieldViolation{}, RequestID: httpx.GetRequestID(c)}})
	} else {
		payload, _ = json.Marshal(gin.H{"data": v, "meta": gin.H{"requestId": httpx.GetRequestID(c)}})
	}
	if e = h.idem.Complete(c, t.OrganizationID, ep, key, status, payload); e != nil {
		respond.Error(c, e)
		return
	}
	c.Data(status, "application/json", payload)
}

type actionBody struct {
	ExpectedVersion  int64  `json:"expectedVersion"`
	Notes            string `json:"notes"`
	ResolutionAction string `json:"resolutionAction"`
	ResolutionReason string `json:"resolutionReason"`
}

func publicReport(v Report) Report {
	if v.Anonymous {
		v.ReporterReference = ""
	}
	return v
}
func (h *OperationsHandler) handle(c *gin.Context, r OperationsRoute) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	run := func() (any, error) {
		switch r.OperationID {
		case "listSafetyReports":
			v, e := h.s.ListReports(c, t)
			for i := range v {
				v[i] = publicReport(v[i])
			}
			return v, e
		case "createSafetyReport":
			var b Report
			if !bindSafety(c, &b) {
				return nil, errors.New("bound")
			}
			v, e := h.s.CreateReport(c, t, b)
			return publicReport(v), e
		case "getSafetyReport":
			v, e := h.s.GetReport(c, t, c.Param("reportId"))
			return publicReport(v), e
		case "updateSafetyReport":
			var b Report
			if !bindSafety(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateReport(c, t, c.Param("reportId"), safetyVersion(c, b.Version), b, false)
		case "resolveSafetyReport":
			var b actionBody
			if !bindSafety(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateReport(c, t, c.Param("reportId"), safetyVersion(c, b.ExpectedVersion), Report{ResolutionAction: b.ResolutionAction, ResolutionReason: b.ResolutionReason}, true)
		case "listIncidents":
			return h.s.ListIncidents(c, t)
		case "createIncident":
			var b Incident
			if !bindSafety(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateIncident(c, t, b)
		case "getIncident":
			return h.s.GetIncident(c, t, c.Param("incidentId"))
		case "updateIncident":
			var b Incident
			if !bindSafety(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateIncident(c, t, c.Param("incidentId"), safetyVersion(c, b.Version), b)
		case "addIncidentEvidence":
			var b Evidence
			if !bindSafety(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.AddEvidence(c, t, c.Param("incidentId"), b)
		case "createCorrectiveAction":
			var b CorrectiveAction
			if !bindSafety(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateCorrectiveAction(c, t, c.Param("incidentId"), b)
		case "completeCorrectiveAction", "verifyCorrectiveAction":
			var b actionBody
			if !bindSafety(c, &b) {
				return nil, errors.New("bound")
			}
			cmd := "COMPLETE"
			if r.OperationID == "verifyCorrectiveAction" {
				cmd = "VERIFY"
			}
			return h.s.Action(c, t, c.Param("actionId"), cmd, b.Notes, safetyVersion(c, b.ExpectedVersion))
		}
		return nil, apperrors.NotFound("Operation not found.", nil)
	}
	if r.IdempotencyRequired {
		h.command(c, t, r, run)
		return
	}
	v, e := run()
	if e != nil && e.Error() == "bound" {
		return
	}
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, v)
}
