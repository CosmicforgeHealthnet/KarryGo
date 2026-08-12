package compliance

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
		{"GET", "/compliance/status", "listComplianceStatus", "compliance.read", "COMPLIANCE_ENABLED", "corporate", false, false},
		{"GET", "/compliance/requirements", "listComplianceRequirements", "compliance.read", "COMPLIANCE_ENABLED", "corporate", false, false},
		{"POST", "/compliance/requirements", "createComplianceRequirement", "compliance.manage", "COMPLIANCE_ENABLED", "corporate", true, false},
		{"GET", "/compliance/requirements/:requirementId", "getComplianceRequirement", "compliance.read", "COMPLIANCE_ENABLED", "corporate", false, false},
		{"PATCH", "/compliance/requirements/:requirementId", "updateComplianceRequirement", "compliance.manage", "COMPLIANCE_ENABLED", "corporate", false, true},
		{"POST", "/compliance/requirements/:requirementId/activate", "activateComplianceRequirement", "compliance.manage", "COMPLIANCE_ENABLED", "corporate", true, true},
		{"POST", "/compliance/requirements/:requirementId/deactivate", "deactivateComplianceRequirement", "compliance.manage", "COMPLIANCE_ENABLED", "corporate", true, true},
		{"POST", "/compliance/evaluations", "evaluateCompliance", "compliance.evaluate", "COMPLIANCE_ENABLED", "corporate", true, false},
		{"GET", "/providers/:membershipId/compliance", "getProviderCompliance", "compliance.read", "COMPLIANCE_ENABLED", "corporate", false, false},
		{"GET", "/assets/:assetId/compliance", "getAssetCompliance", "compliance.read", "COMPLIANCE_ENABLED", "corporate", false, false},
		{"POST", "/compliance/restrictions", "applyRestriction", "compliance.restrict", "RESTRICTION_PROPAGATION_ENABLED", "corporate", true, false},
		{"GET", "/compliance/restrictions/:restrictionId", "getRestriction", "compliance.read", "RESTRICTION_PROPAGATION_ENABLED", "corporate", false, false},
		{"POST", "/compliance/restrictions/:restrictionId/clear", "clearRestriction", "compliance.clear_restriction", "RESTRICTION_PROPAGATION_ENABLED", "corporate", true, true},
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
	if e := h.a.Require(c, t.OrganizationID, t.OrganizationUserID, rv, p, accesscontrol.Target{BranchID: c.Query("branchId")}); e != nil {
		respond.Error(c, e)
		return t, false
	}
	return t, true
}
func bindCompliance(c *gin.Context, v any) bool {
	if e := c.ShouldBindJSON(v); e != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", e))
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
func (h *Handler) command(c *gin.Context, t tenant.Context, r Route, run func() (any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if h.idem == nil {
		respond.Error(c, apperrors.Internal("Idempotency store is unavailable.", nil))
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
		c.Data(rec.ResponseStatus, "application/json; charset=utf-8", rec.ResponseBody)
		return
	}
	v, e := run()
	if e != nil && e.Error() == "bound" {
		return
	}
	status := http.StatusOK
	if strings.HasPrefix(r.OperationID, "create") || r.OperationID == "applyRestriction" {
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
	c.Data(status, "application/json; charset=utf-8", payload)
}

type versionBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	run := func() (any, error) {
		switch r.OperationID {
		case "listComplianceStatus":
			return h.s.ListStatuses(c, t, c.Query("subjectType"), c.Query("subjectReference"))
		case "listComplianceRequirements":
			return h.s.ListRequirements(c, t)
		case "getComplianceRequirement":
			return h.s.GetRequirement(c, t, c.Param("requirementId"))
		case "createComplianceRequirement":
			var b Requirement
			if !bindCompliance(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateRequirement(c, t, b)
		case "updateComplianceRequirement":
			var b Requirement
			if !bindCompliance(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateRequirement(c, t, c.Param("requirementId"), expected(c, b.Version), b, "")
		case "activateComplianceRequirement", "deactivateComplianceRequirement":
			var b versionBody
			if !bindCompliance(c, &b) {
				return nil, errors.New("bound")
			}
			a := "ACTIVATE"
			if r.OperationID == "deactivateComplianceRequirement" {
				a = "DEACTIVATE"
			}
			return h.s.UpdateRequirement(c, t, c.Param("requirementId"), expected(c, b.ExpectedVersion), Requirement{}, a)
		case "evaluateCompliance":
			var b EvaluateInput
			if !bindCompliance(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.Evaluate(c, t, b)
		case "getProviderCompliance":
			return h.s.ListStatuses(c, t, "PROVIDER", c.Param("membershipId"))
		case "getAssetCompliance":
			return h.s.ListStatuses(c, t, "ASSET", c.Param("assetId"))
		case "applyRestriction":
			var b RestrictInput
			if !bindCompliance(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.ApplyRestriction(c, t, b)
		case "getRestriction":
			return h.s.GetRestriction(c, t.OrganizationID, c.Param("restrictionId"))
		case "clearRestriction":
			var b versionBody
			if !bindCompliance(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.ClearRestriction(c, t, c.Param("restrictionId"), b.Reason, expected(c, b.ExpectedVersion))
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
