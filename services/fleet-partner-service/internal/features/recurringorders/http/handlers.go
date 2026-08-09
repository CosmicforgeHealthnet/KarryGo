// Package http exposes recurring-order template routes.
package http

import (
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	PermissionRead   = "recurring_orders.read"
	PermissionManage = "recurring_orders.manage"
)

type Handler struct {
	s *usecases.Service
	a *accesscontrol.Authorizer
	i *idempotency.Store
}

func NewHandler(s *usecases.Service, a *accesscontrol.Authorizer, i *idempotency.Store) *Handler {
	return &Handler{s, a, i}
}
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	if h.s == nil {
		return
	}
	m := map[string]gin.HandlerFunc{"listRecurringOrders": h.list, "createRecurringOrder": h.create, "getRecurringOrder": h.get, "activateRecurringOrder": h.activate, "deactivateRecurringOrder": h.deactivate, "replayRecurringOrder": h.replay}
	for _, r := range h.TenantRoutes() {
		g.Handle(r.Method, r.Path, m[r.OperationID])
	}
}
func ten(c *gin.Context) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
	}
	return t, ok
}
func rv(c *gin.Context) int64 {
	if x, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if n, ok := x.(int64); ok {
			return n
		}
	}
	return 1
}
func (h *Handler) auth(c *gin.Context, t tenant.Context, p string) bool {
	if h.a == nil {
		respond.Error(c, apperrors.Forbidden("Permission denied.", nil))
		return false
	}
	if e := h.a.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID, rv(c), p, accesscontrol.Target{BranchID: c.Query("branchId")}); e != nil {
		respond.Error(c, e)
		return false
	}
	return true
}
func bind(c *gin.Context, v any) bool {
	if c.ShouldBindJSON(v) != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return false
	}
	return true
}
func ver(c *gin.Context, n int64) int64 {
	if n > 0 {
		return n
	}
	v, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return v
}

type smeta struct {
	RequestID string `json:"requestId"`
}
type sok struct {
	Data any   `json:"data"`
	Meta smeta `json:"meta"`
}
type serr struct {
	Error respond.ErrorBody `json:"error"`
}

func (h *Handler) run(c *gin.Context, endpoint string, t tenant.Context, body any, fn func() (int, any, error)) {
	if h.i == nil {
		respond.Error(c, apperrors.Internal("Idempotency unavailable.", nil))
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	raw, _ := json.Marshal(body)
	fp := idempotency.Fingerprint(c.Request.Method, endpoint, raw)
	rec, e := h.i.Begin(c.Request.Context(), t.OrganizationID, endpoint, key, fp)
	if e != nil {
		respond.Error(c, idempotency.AsAppError(e))
		return
	}
	if rec != nil {
		c.Data(rec.ResponseStatus, "application/json; charset=utf-8", rec.ResponseBody)
		return
	}
	status, data, x := fn()
	rid := httpx.GetRequestID(c)
	var payload []byte
	if x != nil {
		ae := apperrors.From(x)
		d := ae.Fields
		if d == nil {
			d = []apperrors.FieldViolation{}
		}
		payload, _ = json.Marshal(serr{respond.ErrorBody{Code: ae.Code, Message: ae.Message, Details: d, RequestID: rid}})
		status = ae.Status
	} else {
		payload, _ = json.Marshal(sok{data, smeta{rid}})
	}
	if e = h.i.Complete(c.Request.Context(), t.OrganizationID, endpoint, key, status, payload); e != nil {
		respond.Error(c, apperrors.Internal("Failed to persist idempotent response.", e))
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

type createReq struct {
	BranchID                   string              `json:"branchId"`
	CustomerID                 string              `json:"customerId"`
	ServiceCatalogItemID       string              `json:"serviceCatalogItemId"`
	ContractID                 string              `json:"contractId"`
	RecurrenceType             string              `json:"recurrenceType"`
	RRule                      string              `json:"rrule"`
	Timezone                   string              `json:"timezone"`
	StartDate                  time.Time           `json:"startDate"`
	EndDate                    *time.Time          `json:"endDate"`
	MissedRunPolicy            string              `json:"missedRunPolicy"`
	GeneratedOrderInitialState string              `json:"generatedOrderInitialState"`
	TemplateData               models.TemplateData `json:"templateData"`
}

func (h *Handler) create(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionManage) {
		return
	}
	var b createReq
	if !bind(c, &b) {
		return
	}
	h.run(c, "POST /recurring-orders", t, b, func() (int, any, error) {
		v, e := h.s.Create(c.Request.Context(), t, usecases.CreateInput{BranchID: b.BranchID, CustomerID: b.CustomerID, ServiceCatalogItemID: b.ServiceCatalogItemID, ContractID: b.ContractID, RecurrenceType: b.RecurrenceType, RRule: b.RRule, Timezone: b.Timezone, StartDate: b.StartDate, EndDate: b.EndDate, MissedRunPolicy: b.MissedRunPolicy, GeneratedOrderInitialState: b.GeneratedOrderInitialState, Data: b.TemplateData})
		return http.StatusCreated, v, e
	})
}
func (h *Handler) list(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionRead) {
		return
	}
	v, e := h.s.List(c.Request.Context(), t, c.Query("status"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, v)
}
func (h *Handler) get(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionRead) {
		return
	}
	v, e := h.s.Get(c.Request.Context(), t, c.Param("templateId"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, v)
}

type statusReq struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func (h *Handler) status(c *gin.Context, active bool) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionManage) {
		return
	}
	var b statusReq
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = ver(c, b.ExpectedVersion)
	h.run(c, "POST "+c.FullPath(), t, b, func() (int, any, error) {
		v, e := h.s.SetActive(c.Request.Context(), t, c.Param("templateId"), b.ExpectedVersion, active)
		return http.StatusOK, v, e
	})
}
func (h *Handler) activate(c *gin.Context)   { h.status(c, true) }
func (h *Handler) deactivate(c *gin.Context) { h.status(c, false) }

func (h *Handler) replay(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionManage) {
		return
	}
	h.run(c, "POST /recurring-orders/:templateId/replay", t, struct{}{}, func() (int, any, error) {
		e := h.s.Replay(c.Request.Context(), t, c.Param("templateId"))
		return http.StatusAccepted, gin.H{"status": "REQUEUED"}, e
	})
}
