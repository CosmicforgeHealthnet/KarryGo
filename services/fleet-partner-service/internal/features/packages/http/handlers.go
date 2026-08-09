// Package http exposes tenant-scoped Section 15 package, scan and custody routes.
package http

import (
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/usecases"
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
	PermissionPackagesRead   = "packages.read"
	PermissionPackagesManage = "packages.manage"
	PermissionPackagesScan   = "packages.scan"
	PermissionCustodyRead    = "custody.read"
	PermissionCustodyRecord  = "custody.record"
)

type Handler struct {
	service     *usecases.Service
	authorizer  *accesscontrol.Authorizer
	idempotency *idempotency.Store
}

func NewHandler(s *usecases.Service, a *accesscontrol.Authorizer, i *idempotency.Store) *Handler {
	return &Handler{s, a, i}
}
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	if h.service == nil {
		return
	}
	m := map[string]gin.HandlerFunc{"createPackage": h.create, "listPackages": h.list, "getPackage": h.get, "getPackageHistory": h.history, "transitionPackage": h.transition, "scanPackage": h.scan, "listCustody": h.custody, "recordCustody": h.recordCustody}
	for _, r := range h.TenantRoutes() {
		g.Handle(r.Method, r.Path, m[r.OperationID])
	}
}
func requireTenant(c *gin.Context) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
	}
	return t, ok
}
func roleVersion(c *gin.Context) int64 {
	if v, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if n, ok := v.(int64); ok {
			return n
		}
	}
	return 1
}
func (h *Handler) auth(c *gin.Context, t tenant.Context, p string) bool {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("Permission denied.", nil))
		return false
	}
	if e := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID, roleVersion(c), p, accesscontrol.Target{BranchID: c.Query("branchId")}); e != nil {
		respond.Error(c, e)
		return false
	}
	return true
}
func bind(c *gin.Context, v any) bool {
	if e := c.ShouldBindJSON(v); e != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return false
	}
	return true
}
func expected(c *gin.Context, n int64) int64 {
	if n > 0 {
		return n
	}
	raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`)
	v, _ := strconv.ParseInt(raw, 10, 64)
	return v
}

type meta struct {
	RequestID string `json:"requestId"`
}
type success struct {
	Data any  `json:"data"`
	Meta meta `json:"meta"`
}
type failure struct {
	Error respond.ErrorBody `json:"error"`
}

func (h *Handler) runIdempotent(c *gin.Context, endpoint string, t tenant.Context, body any, fn func() (int, any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if h.idempotency == nil {
		respond.Error(c, apperrors.Internal("Idempotent commands are unavailable.", nil))
		return
	}
	b, _ := json.Marshal(body)
	fp := idempotency.Fingerprint(c.Request.Method, endpoint, b)
	rec, e := h.idempotency.Begin(c.Request.Context(), t.OrganizationID, endpoint, key, fp)
	if e != nil {
		respond.Error(c, idempotency.AsAppError(e))
		return
	}
	if rec != nil {
		c.Data(rec.ResponseStatus, "application/json; charset=utf-8", rec.ResponseBody)
		return
	}
	status, data, svcErr := fn()
	requestID := httpx.GetRequestID(c)
	var payload []byte
	if svcErr != nil {
		ae := apperrors.From(svcErr)
		details := ae.Fields
		if details == nil {
			details = []apperrors.FieldViolation{}
		}
		payload, _ = json.Marshal(failure{respond.ErrorBody{Code: ae.Code, Message: ae.Message, Details: details, RequestID: requestID}})
		status = ae.Status
	} else {
		payload, _ = json.Marshal(success{data, meta{requestID}})
	}
	if e = h.idempotency.Complete(c.Request.Context(), t.OrganizationID, endpoint, key, status, payload); e != nil {
		respond.Error(c, apperrors.Internal("Failed to persist idempotent response.", e))
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

type PackageView struct {
	ID                       string    `json:"id"`
	ShipmentID               string    `json:"shipmentId"`
	BranchID                 string    `json:"branchId,omitempty"`
	PackageCode              string    `json:"packageCode"`
	BarcodeValue             string    `json:"barcodeValue"`
	QRValue                  string    `json:"qrValue"`
	PackageType              string    `json:"packageType"`
	Length                   string    `json:"length"`
	Width                    string    `json:"width"`
	Height                   string    `json:"height"`
	DimensionUnit            string    `json:"dimensionUnit"`
	Weight                   string    `json:"weight"`
	WeightUnit               string    `json:"weightUnit"`
	DeclaredValueAmountMinor int64     `json:"declaredValueAmountMinor"`
	DeclaredValueCurrency    string    `json:"declaredValueCurrency"`
	HandlingInstructions     string    `json:"handlingInstructions,omitempty"`
	Condition                string    `json:"condition"`
	Status                   string    `json:"status"`
	Version                  int64     `json:"version"`
	CreatedAt                time.Time `json:"createdAt"`
	UpdatedAt                time.Time `json:"updatedAt"`
}

func view(p models.Package) PackageView {
	return PackageView{p.ID, p.ShipmentID, p.BranchID, p.PackageCode, p.BarcodeValue, p.QRValue, p.PackageType, p.Length, p.Width, p.Height, p.DimensionUnit, p.Weight, p.WeightUnit, p.DeclaredValueAmountMinor, p.DeclaredValueCurrency, p.HandlingInstructions, p.Condition, p.Status, p.Version, p.CreatedAt, p.UpdatedAt}
}

type createRequest struct {
	PackageType              string `json:"packageType"`
	Length                   string `json:"length"`
	Width                    string `json:"width"`
	Height                   string `json:"height"`
	DimensionUnit            string `json:"dimensionUnit"`
	Weight                   string `json:"weight"`
	WeightUnit               string `json:"weightUnit"`
	DeclaredValueAmountMinor int64  `json:"declaredValueAmountMinor"`
	DeclaredValueCurrency    string `json:"declaredValueCurrency"`
	HandlingInstructions     string `json:"handlingInstructions"`
}

func (h *Handler) create(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.auth(c, t, PermissionPackagesManage) {
		return
	}
	var b createRequest
	if !bind(c, &b) {
		return
	}
	h.runIdempotent(c, "POST /shipments/:shipmentId/packages", t, b, func() (int, any, error) {
		p, e := h.service.Create(c.Request.Context(), t, usecases.CreateInput{ShipmentID: c.Param("shipmentId"), PackageType: b.PackageType, Length: b.Length, Width: b.Width, Height: b.Height, DimensionUnit: b.DimensionUnit, Weight: b.Weight, WeightUnit: b.WeightUnit, DeclaredValueAmountMinor: b.DeclaredValueAmountMinor, DeclaredValueCurrency: b.DeclaredValueCurrency, HandlingInstructions: b.HandlingInstructions})
		return http.StatusCreated, view(p), e
	})
}
func (h *Handler) list(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.auth(c, t, PermissionPackagesRead) {
		return
	}
	items, e := h.service.List(c.Request.Context(), t, c.Param("shipmentId"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	out := make([]PackageView, 0, len(items))
	for _, p := range items {
		out = append(out, view(p))
	}
	respond.OK(c, out)
}
func (h *Handler) get(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.auth(c, t, PermissionPackagesRead) {
		return
	}
	p, e := h.service.Get(c.Request.Context(), t, c.Param("packageId"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, view(p))
}
func (h *Handler) history(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.auth(c, t, PermissionPackagesRead) {
		return
	}
	v, e := h.service.History(c.Request.Context(), t, c.Param("packageId"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, v)
}

type transitionRequest struct {
	NewStatus       string `json:"newStatus"`
	ReasonCode      string `json:"reasonCode"`
	SafeNote        string `json:"safeNote"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) transition(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.auth(c, t, PermissionPackagesManage) {
		return
	}
	var b transitionRequest
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = expected(c, b.ExpectedVersion)
	h.runIdempotent(c, "POST /packages/:packageId/transition", t, b, func() (int, any, error) {
		p, e := h.service.Transition(c.Request.Context(), t, c.Param("packageId"), usecases.TransitionInput{NewStatus: b.NewStatus, ReasonCode: b.ReasonCode, SafeNote: b.SafeNote, Source: "CORPORATE_API", ExpectedVersion: b.ExpectedVersion})
		return http.StatusOK, view(p), e
	})
}

type scanRequest struct {
	ScanType          string    `json:"scanType"`
	Barcode           string    `json:"barcode"`
	QR                string    `json:"qr"`
	LocationReference string    `json:"locationReference"`
	DeviceReference   string    `json:"deviceReference"`
	OccurredAt        time.Time `json:"occurredAt"`
	ClientScanID      string    `json:"clientScanId"`
	ReasonCode        string    `json:"reasonCode"`
	ExpectedVersion   int64     `json:"expectedVersion"`
}

func (h *Handler) scan(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.auth(c, t, PermissionPackagesScan) {
		return
	}
	var b scanRequest
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = expected(c, b.ExpectedVersion)
	h.runIdempotent(c, "POST /packages/:packageId/scan", t, b, func() (int, any, error) {
		s, e := h.service.Scan(c.Request.Context(), t, c.Param("packageId"), usecases.ScanInput{ScanType: b.ScanType, Barcode: b.Barcode, QR: b.QR, LocationReference: b.LocationReference, DeviceReference: b.DeviceReference, OccurredAt: b.OccurredAt, ClientScanID: b.ClientScanID, ReasonCode: b.ReasonCode, ExpectedVersion: b.ExpectedVersion})
		return http.StatusOK, s, e
	})
}
func (h *Handler) custody(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.auth(c, t, PermissionCustodyRead) {
		return
	}
	v, e := h.service.Custody(c.Request.Context(), t, c.Param("packageId"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, v)
}

type custodyRequest struct {
	FromActorType         string    `json:"fromActorType"`
	FromActorReference    string    `json:"fromActorReference"`
	ToActorType           string    `json:"toActorType"`
	ToActorReference      string    `json:"toActorReference"`
	FromLocationReference string    `json:"fromLocationReference"`
	ToLocationReference   string    `json:"toLocationReference"`
	Condition             string    `json:"condition"`
	EvidenceMediaID       string    `json:"evidenceMediaId"`
	CorrectionOfEventID   string    `json:"correctionOfEventId"`
	OccurredAt            time.Time `json:"occurredAt"`
}

func (h *Handler) recordCustody(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.auth(c, t, PermissionCustodyRecord) {
		return
	}
	var b custodyRequest
	if !bind(c, &b) {
		return
	}
	h.runIdempotent(c, "POST /packages/:packageId/custody", t, b, func() (int, any, error) {
		e, x := h.service.RecordCustody(c.Request.Context(), t, c.Param("packageId"), usecases.CustodyInput{FromActorType: b.FromActorType, FromActorReference: b.FromActorReference, ToActorType: b.ToActorType, ToActorReference: b.ToActorReference, FromLocationReference: b.FromLocationReference, ToLocationReference: b.ToLocationReference, Condition: b.Condition, EvidenceMediaID: b.EvidenceMediaID, CorrectionOfEventID: b.CorrectionOfEventID, OccurredAt: b.OccurredAt, Source: "CORPORATE_API"})
		return http.StatusCreated, e, x
	})
}
