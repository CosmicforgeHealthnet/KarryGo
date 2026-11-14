// Package http serves the Section 19 compliance routes. Handlers parse the
// request and call the use cases through the Service port.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance/models"
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
		{"GET", "/compliance/restrictions", "listRestrictions", "compliance.read", "RESTRICTION_PROPAGATION_ENABLED", "corporate", false, false},
		{"GET", "/compliance/checks", "listComplianceChecks", "compliance.read", "COMPLIANCE_ENABLED", "corporate", false, false},
		{"POST", "/compliance/checks", "createComplianceCheck", "compliance.evaluate", "COMPLIANCE_ENABLED", "corporate", true, false},
		{"GET", "/compliance/checks/:checkId", "getComplianceCheck", "compliance.read", "COMPLIANCE_ENABLED", "corporate", false, false},
	}
}
func (r Route) FullPath() string { return "/fleet-partner/v1" + r.Path }

// Service is the use-case surface the compliance handlers call.
type Service interface {
	ListStatuses(ctx context.Context, t tenant.Context, typ, ref string) ([]models.Status, error)
	ListRequirements(ctx context.Context, t tenant.Context) ([]models.Requirement, error)
	GetRequirement(ctx context.Context, t tenant.Context, id string) (models.Requirement, error)
	CreateRequirement(ctx context.Context, t tenant.Context, v models.Requirement) (models.Requirement, error)
	UpdateRequirement(ctx context.Context, t tenant.Context, id string, expected int64, in models.Requirement, action string) (models.Requirement, error)
	Evaluate(ctx context.Context, t tenant.Context, in models.EvaluateInput) (models.Status, error)
	ApplyRestriction(ctx context.Context, t tenant.Context, in models.RestrictInput) (models.Restriction, error)
	RestrictionDetail(ctx context.Context, t tenant.Context, id string) (models.RestrictionView, error)
	ListRestrictions(ctx context.Context, t tenant.Context, typ, ref, state string) ([]models.Restriction, error)
	ListChecks(ctx context.Context, t tenant.Context, typ, ref string) ([]models.Check, error)
	GetCheck(ctx context.Context, t tenant.Context, id string) (models.Check, error)
	RunCheck(ctx context.Context, t tenant.Context, in models.CheckInput) (models.Check, error)
	ClearRestriction(ctx context.Context, t tenant.Context, id, reason string, expected int64) (models.Restriction, error)
}

type Handler struct {
	s    Service
	a    *accesscontrol.Authorizer
	idem *idempotency.Store
}

func NewHandler(s Service, a *accesscontrol.Authorizer, i *idempotency.Store) *Handler {
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
	// Narrowed to the branches where the permission is granted; a
	// caller-chosen ?branchId never widens it.
	scoped, e := h.a.RequireScoped(c, t, rv, p)
	if e != nil {
		respond.Error(c, e)
		return t, false
	}
	middleware.SetTenant(c, scoped)
	return scoped, true
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
			var b models.Requirement
			if !bindCompliance(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateRequirement(c, t, b)
		case "updateComplianceRequirement":
			var b models.Requirement
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
			return h.s.UpdateRequirement(c, t, c.Param("requirementId"), expected(c, b.ExpectedVersion), models.Requirement{}, a)
		case "evaluateCompliance":
			var b models.EvaluateInput
			if !bindCompliance(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.Evaluate(c, t, b)
		case "getProviderCompliance":
			return h.s.ListStatuses(c, t, "PROVIDER", c.Param("membershipId"))
		case "getAssetCompliance":
			return h.s.ListStatuses(c, t, "ASSET", c.Param("assetId"))
		case "applyRestriction":
			var b models.RestrictInput
			if !bindCompliance(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.ApplyRestriction(c, t, b)
		case "getRestriction":
			return h.s.RestrictionDetail(c, t, c.Param("restrictionId"))
		case "listRestrictions":
			return h.s.ListRestrictions(c, t, c.Query("subjectType"), c.Query("subjectReference"), c.Query("state"))
		case "listComplianceChecks":
			return h.s.ListChecks(c, t, c.Query("subjectType"), c.Query("subjectReference"))
		case "getComplianceCheck":
			return h.s.GetCheck(c, t, c.Param("checkId"))
		case "createComplianceCheck":
			var b models.CheckInput
			if !bindCompliance(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.RunCheck(c, t, b)
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
