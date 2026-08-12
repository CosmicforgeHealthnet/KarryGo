// Package http exposes Section 15 corporate manifest routes.
package http

import (
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/usecases"
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
)

const (
	PermissionRead      = "manifests.read"
	PermissionManage    = "manifests.manage"
	PermissionSealAmend = "manifests.seal_amend"
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
	m := map[string]gin.HandlerFunc{"listManifests": h.list, "createManifest": h.create, "getManifest": h.get, "updateManifest": h.update, "addManifestItem": h.addItem, "closeManifest": h.close, "amendManifestSeal": h.amendSeal, "cancelManifest": h.cancel}
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
	BranchID             string `json:"branchId"`
	RouteReference       string `json:"routeReference"`
	VehicleReference     string `json:"vehicleReference"`
	ProviderMembershipID string `json:"providerMembershipId"`
	SealNumber           string `json:"sealNumber"`
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
	h.run(c, "POST /manifests", t, b, func() (int, any, error) {
		m, e := h.s.Create(c.Request.Context(), t, usecases.CreateInput{BranchID: b.BranchID, RouteReference: b.RouteReference, VehicleReference: b.VehicleReference, ProviderMembershipID: b.ProviderMembershipID, SealNumber: b.SealNumber})
		return http.StatusCreated, m, e
	})
}
func (h *Handler) list(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionRead) {
		return
	}
	v, e := h.s.List(c.Request.Context(), t, c.Query("status"), c.Query("branchId"))
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
	m, i, e := h.s.Get(c.Request.Context(), t, c.Param("manifestId"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, gin.H{"manifest": m, "items": i})
}

type updateReq struct {
	RouteReference       string `json:"routeReference"`
	VehicleReference     string `json:"vehicleReference"`
	ProviderMembershipID string `json:"providerMembershipId"`
	SealNumber           string `json:"sealNumber"`
	ExpectedVersion      int64  `json:"expectedVersion"`
}

func (h *Handler) update(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionManage) {
		return
	}
	var b updateReq
	if !bind(c, &b) {
		return
	}
	m, e := h.s.Update(c.Request.Context(), t, c.Param("manifestId"), usecases.UpdateInput{RouteReference: b.RouteReference, VehicleReference: b.VehicleReference, ProviderMembershipID: b.ProviderMembershipID, SealNumber: b.SealNumber, ExpectedVersion: ver(c, b.ExpectedVersion)})
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, m)
}

type itemReq struct {
	ItemType string `json:"itemType"`
	ItemID   string `json:"itemId"`
}

func (h *Handler) addItem(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionManage) {
		return
	}
	var b itemReq
	if !bind(c, &b) {
		return
	}
	h.run(c, "POST /manifests/:manifestId/items", t, b, func() (int, any, error) {
		i, e := h.s.AddItem(c.Request.Context(), t, c.Param("manifestId"), usecases.AddItemInput{ItemType: b.ItemType, ItemID: b.ItemID})
		return http.StatusCreated, i, e
	})
}

type closeReq struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func (h *Handler) close(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionManage) {
		return
	}
	var b closeReq
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = ver(c, b.ExpectedVersion)
	h.run(c, "POST /manifests/:manifestId/close", t, b, func() (int, any, error) {
		m, s, e := h.s.Close(c.Request.Context(), t, c.Param("manifestId"), usecases.CloseInput{ExpectedVersion: b.ExpectedVersion, Fingerprint: "http-idempotency"})
		return http.StatusOK, gin.H{"manifest": m, "snapshot": s}, e
	})
}

type sealAmendReq struct {
	SealNumber      string `json:"sealNumber"`
	ReasonCode      string `json:"reasonCode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) amendSeal(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionSealAmend) {
		return
	}
	var b sealAmendReq
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = ver(c, b.ExpectedVersion)
	h.run(c, "POST /manifests/:manifestId/seal-amendments", t, b, func() (int, any, error) {
		m, e := h.s.AmendSeal(c.Request.Context(), t, c.Param("manifestId"), usecases.SealAmendmentInput{
			SealNumber: b.SealNumber, ReasonCode: b.ReasonCode, ExpectedVersion: b.ExpectedVersion})
		return http.StatusOK, m, e
	})
}

type cancelReq struct {
	ReasonCode      string `json:"reasonCode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) cancel(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionManage) {
		return
	}
	var b cancelReq
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = ver(c, b.ExpectedVersion)
	h.run(c, "POST /manifests/:manifestId/cancel", t, b, func() (int, any, error) {
		m, e := h.s.Cancel(c.Request.Context(), t, c.Param("manifestId"), usecases.CancelInput{
			ReasonCode: b.ReasonCode, ExpectedVersion: b.ExpectedVersion})
		return http.StatusOK, m, e
	})
}
