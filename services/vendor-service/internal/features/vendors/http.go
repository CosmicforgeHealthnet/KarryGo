package vendors

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	service            *Service
	idempotency        *idempotency.Store
	writesEnabled      bool
	commerceKillSwitch bool
}

func NewHandler(service *Service, store *idempotency.Store, writesEnabled, commerceKillSwitch bool) *Handler {
	return &Handler{service: service, idempotency: store, writesEnabled: writesEnabled, commerceKillSwitch: commerceKillSwitch}
}

func actor(c *gin.Context) (Actor, error) {
	a, ok := authctx.FromContext(c)
	if !ok {
		return Actor{}, apperrors.Unauthorized("Authentication is required.", nil)
	}
	return Actor{StaffID: a.StaffID, VendorID: a.VendorID, SessionID: a.SessionID, CorrelationID: httpx.GetRequestID(c), CausationID: c.GetHeader("X-Causation-ID")}, nil
}
func strict(raw []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return apperrors.BadRequest("Check your details.", err)
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apperrors.BadRequest("Request body must contain one JSON value.", err)
	}
	return nil
}
func ok(c *gin.Context, data any)                 { c.JSON(http.StatusOK, gin.H{"success": true, "data": data}) }
func (h *Handler) fail(c *gin.Context, err error) { httpx.RespondError(c, err) }

func listOptions(c *gin.Context, allowedSort map[string]bool) (ListOptions, error) {
	o := ListOptions{Limit: 50, Status: c.Query("status"), CapabilityType: c.Query("capability_type"), BrandID: c.Query("brand_id"), Search: strings.TrimSpace(c.Query("search")), Sort: c.Query("sort"), Direction: strings.ToLower(c.Query("direction"))}
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return ListOptions{}, validation(apperrors.FieldViolation{Field: "limit", Message: "Limit must be from 1 to 100."})
		}
		o.Limit = n
	}
	if raw := c.Query("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return ListOptions{}, validation(apperrors.FieldViolation{Field: "offset", Message: "Offset must be zero or greater."})
		}
		o.Offset = n
	}
	if len(o.Search) > 100 {
		return ListOptions{}, validation(apperrors.FieldViolation{Field: "search", Message: "Search cannot exceed 100 characters."})
	}
	if o.CapabilityType != "" && !IsSupportedCapability(CapabilityType(o.CapabilityType)) {
		return ListOptions{}, validation(apperrors.FieldViolation{Field: "capability_type", Message: "Unsupported capability type."})
	}
	if o.Sort != "" && !allowedSort[o.Sort] {
		return ListOptions{}, validation(apperrors.FieldViolation{Field: "sort", Message: "Unsupported sort field."})
	}
	if o.Direction != "" && o.Direction != "asc" && o.Direction != "desc" {
		return ListOptions{}, validation(apperrors.FieldViolation{Field: "direction", Message: "Direction must be asc or desc."})
	}
	return o, nil
}

func pageData[T any](items []T, o ListOptions) gin.H {
	hasMore := len(items) > o.Limit
	if hasMore {
		items = items[:o.Limit]
	}
	return gin.H{"items": items, "pagination": gin.H{"limit": o.Limit, "offset": o.Offset, "has_more": hasMore}}
}

func statusFilter(value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return validation(apperrors.FieldViolation{Field: "status", Message: "Unsupported status filter."})
}
func (h *Handler) readActor(c *gin.Context) (Actor, bool) {
	a, err := actor(c)
	if err != nil {
		h.fail(c, err)
		return Actor{}, false
	}
	return a, true
}

func (h *Handler) mutate(c *gin.Context, status int, fn func(Actor, []byte) (any, error)) {
	if !h.writesEnabled || h.commerceKillSwitch {
		h.fail(c, apperrors.Unavailable("Vendor onboarding writes are temporarily disabled.", nil))
		return
	}
	a, yes := h.readActor(c)
	if !yes {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		h.fail(c, apperrors.BadRequest("Could not read request body.", err))
		return
	}
	key := c.GetHeader("Idempotency-Key")
	endpoint := c.Request.Method + " " + c.FullPath()
	fingerprint := idempotency.Fingerprint(c.Request.Method, c.Request.URL.Path, raw)
	record, err := h.idempotency.Begin(c, a.VendorID, endpoint, key, fingerprint)
	if err != nil {
		if errors.Is(err, idempotency.ErrFingerprintMismatch) || errors.Is(err, idempotency.ErrInProgress) {
			h.fail(c, apperrors.Conflict("This idempotency key is already in use for another request.", err))
		} else {
			h.fail(c, apperrors.BadRequest("Idempotency-Key is required.", err))
		}
		return
	}
	if record != nil {
		c.Data(record.ResponseStatus, "application/json", record.ResponseBody)
		return
	}
	data, err := fn(a, raw)
	if err != nil {
		_ = h.idempotency.Release(c, a.VendorID, endpoint, key)
		h.fail(c, err)
		return
	}
	body, err := json.Marshal(gin.H{"success": true, "data": data})
	if err != nil {
		_ = h.idempotency.Release(c, a.VendorID, endpoint, key)
		h.fail(c, err)
		return
	}
	if err = h.idempotency.Complete(c, a.VendorID, endpoint, key, status, body); err != nil {
		h.fail(c, err)
		return
	}
	c.Data(status, "application/json", body)
}

type createBusinessRequest struct {
	LegalName    string `json:"legal_name"`
	TradingName  string `json:"trading_name"`
	SupportEmail string `json:"support_email"`
	SupportPhone string `json:"support_phone"`
	Currency     string `json:"currency"`
	Timezone     string `json:"timezone"`
}
type updateBusinessRequest struct {
	createBusinessRequest
	ExpectedVersion int64 `json:"expected_version"`
}
type commandRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}
type capabilityRequest struct {
	CapabilityType CapabilityType `json:"capability_type"`
}
type brandRequest struct {
	DisplayName string           `json:"display_name"`
	Description string           `json:"description"`
	MediaRefs   []MediaReference `json:"media_refs"`
}
type updateBrandRequest struct {
	brandRequest
	ExpectedVersion int64 `json:"expected_version"`
}
type branchRequest struct {
	BrandID     string  `json:"brand_id"`
	DisplayName string  `json:"display_name"`
	Address     Address `json:"address"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Timezone    string  `json:"timezone"`
}
type updateBranchRequest struct {
	branchRequest
	ExpectedVersion int64 `json:"expected_version"`
}
type hoursRequest struct {
	Hours           []OperatingWindow `json:"hours"`
	ExpectedVersion int64             `json:"expected_version"`
}
type serviceAreaRequest struct {
	Mode            string  `json:"mode"`
	CenterLatitude  float64 `json:"center_latitude"`
	CenterLongitude float64 `json:"center_longitude"`
	RadiusMeters    int     `json:"radius_meters"`
	ExpectedVersion int64   `json:"expected_version"`
}
type settingsRequest struct {
	SchemaVersion      int   `json:"schema_version"`
	AcceptingNewOrders bool  `json:"accepting_new_orders"`
	PickupEnabled      bool  `json:"pickup_enabled"`
	ExpectedVersion    int64 `json:"expected_version"`
}
type branchCapabilityRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

func (h *Handler) CreateBusiness(c *gin.Context) {
	h.mutate(c, http.StatusCreated, func(a Actor, raw []byte) (any, error) {
		var r createBusinessRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.CreateBusiness(c, a, Business{LegalName: r.LegalName, TradingName: r.TradingName, SupportEmail: r.SupportEmail, SupportPhone: r.SupportPhone, Currency: r.Currency, Timezone: r.Timezone})
	})
}
func (h *Handler) ListBusinesses(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.ListBusinesses(c, a)
	if e != nil {
		h.fail(c, e)
		return
	}
	o, e := listOptions(c, map[string]bool{})
	if e != nil {
		h.fail(c, e)
		return
	}
	if e = statusFilter(o.Status, "DRAFT", "PENDING_VERIFICATION", "PENDING_REVIEW", "ACTIVE", "PAUSED", "SUSPENDED", "CLOSED"); e != nil {
		h.fail(c, e)
		return
	}
	filtered := v[:0]
	for _, b := range v {
		if o.Status != "" && string(b.Status) != o.Status {
			continue
		}
		if o.Search != "" && !strings.Contains(strings.ToLower(b.LegalName+" "+b.TradingName), strings.ToLower(o.Search)) {
			continue
		}
		filtered = append(filtered, b)
	}
	if o.Offset >= len(filtered) {
		filtered = []Business{}
	} else if o.Offset > 0 {
		filtered = filtered[o.Offset:]
	}
	ok(c, pageData(filtered, o))
}
func (h *Handler) GetBusiness(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.GetBusiness(c, a, c.Param("businessId"))
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) UpdateBusiness(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r updateBusinessRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.UpdateBusiness(c, a, c.Param("businessId"), UpdateBusinessInput{LegalName: r.LegalName, TradingName: r.TradingName, SupportEmail: r.SupportEmail, SupportPhone: r.SupportPhone, Currency: r.Currency, Timezone: r.Timezone, ExpectedVersion: r.ExpectedVersion})
	})
}
func (h *Handler) businessStatus(c *gin.Context, to BusinessStatus) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r commandRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.ChangeBusinessStatus(c, a, c.Param("businessId"), to, r.ExpectedVersion)
	})
}

func (h *Handler) CapabilityTypes(c *gin.Context) {
	out := make([]gin.H, 0, len(SupportedCapabilityTypes))
	for _, t := range SupportedCapabilityTypes {
		out = append(out, gin.H{"type": t, "family": CapabilityFamily(t)})
	}
	ok(c, out)
}
func (h *Handler) ListCapabilities(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	o, e := listOptions(c, map[string]bool{})
	if e != nil {
		h.fail(c, e)
		return
	}
	if e = statusFilter(o.Status, "DRAFT", "PENDING_REVIEW", "ACTIVE", "PAUSED", "SUSPENDED", "REJECTED"); e != nil {
		h.fail(c, e)
		return
	}
	v, e := h.service.ListCapabilities(c, a, c.Param("businessId"), o)
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, pageData(v, o))
}
func (h *Handler) CreateCapability(c *gin.Context) {
	h.mutate(c, http.StatusCreated, func(a Actor, raw []byte) (any, error) {
		var r capabilityRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.CreateCapability(c, a, c.Param("businessId"), r.CapabilityType)
	})
}
func (h *Handler) GetCapability(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.GetCapability(c, a, c.Param("businessId"), c.Param("capabilityId"))
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) capabilityStatus(c *gin.Context, to CapabilityStatus) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r commandRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.ChangeCapabilityStatus(c, a, c.Param("businessId"), c.Param("capabilityId"), to, r.ExpectedVersion)
	})
}

func (h *Handler) ListBrands(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	o, e := listOptions(c, map[string]bool{"display_name": true, "verification_state": true, "created_at": true})
	if e != nil {
		h.fail(c, e)
		return
	}
	if e = statusFilter(o.Status, "NOT_STARTED", "IN_PROGRESS", "PENDING_REVIEW", "VERIFIED", "REJECTED", "EXPIRED", "SUSPENDED"); e != nil {
		h.fail(c, e)
		return
	}
	v, e := h.service.ListBrands(c, a, c.Param("businessId"), o)
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, pageData(v, o))
}
func (h *Handler) CreateBrand(c *gin.Context) {
	h.mutate(c, http.StatusCreated, func(a Actor, raw []byte) (any, error) {
		var r brandRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.CreateBrand(c, a, c.Param("businessId"), Brand{DisplayName: r.DisplayName, Description: r.Description, MediaRefs: r.MediaRefs})
	})
}
func (h *Handler) GetBrand(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.GetBrand(c, a, c.Param("businessId"), c.Param("brandId"))
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) UpdateBrand(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r updateBrandRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.UpdateBrand(c, a, c.Param("businessId"), c.Param("brandId"), Brand{DisplayName: r.DisplayName, Description: r.Description, MediaRefs: r.MediaRefs}, r.ExpectedVersion)
	})
}

func (h *Handler) ListBranches(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	o, e := listOptions(c, map[string]bool{"display_name": true, "status": true, "created_at": true})
	if e != nil {
		h.fail(c, e)
		return
	}
	if e = statusFilter(o.Status, "DRAFT", "ACTIVE", "PAUSED", "SUSPENDED", "CLOSED"); e != nil {
		h.fail(c, e)
		return
	}
	v, e := h.service.ListBranches(c, a, c.Param("businessId"), o)
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, pageData(v, o))
}
func (h *Handler) CreateBranch(c *gin.Context) {
	h.mutate(c, http.StatusCreated, func(a Actor, raw []byte) (any, error) {
		var r branchRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.CreateBranch(c, a, c.Param("businessId"), Branch{BrandID: r.BrandID, DisplayName: r.DisplayName, Address: r.Address, Latitude: r.Latitude, Longitude: r.Longitude, Timezone: r.Timezone})
	})
}
func (h *Handler) GetBranch(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.GetBranch(c, a, c.Param("businessId"), c.Param("branchId"))
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) UpdateBranch(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r updateBranchRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.UpdateBranch(c, a, c.Param("businessId"), c.Param("branchId"), Branch{BrandID: r.BrandID, DisplayName: r.DisplayName, Address: r.Address, Latitude: r.Latitude, Longitude: r.Longitude, Timezone: r.Timezone}, r.ExpectedVersion)
	})
}
func (h *Handler) branchStatus(c *gin.Context, to BranchStatus) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r commandRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.ChangeBranchStatus(c, a, c.Param("businessId"), c.Param("branchId"), to, r.ExpectedVersion)
	})
}
func (h *Handler) GetHours(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.GetHours(c, a, c.Param("businessId"), c.Param("branchId"))
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) PutHours(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r hoursRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.PutHours(c, a, c.Param("businessId"), c.Param("branchId"), r.Hours, r.ExpectedVersion)
	})
}
func (h *Handler) GetServiceArea(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.GetServiceArea(c, a, c.Param("businessId"), c.Param("branchId"))
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) PutServiceArea(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r serviceAreaRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.PutServiceArea(c, a, c.Param("businessId"), c.Param("branchId"), ServiceArea{Mode: r.Mode, CenterLatitude: r.CenterLatitude, CenterLongitude: r.CenterLongitude, RadiusMeters: r.RadiusMeters}, r.ExpectedVersion)
	})
}
func (h *Handler) GetSettings(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.GetSettings(c, a, c.Param("businessId"), c.Param("branchId"))
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) PutSettings(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r settingsRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.PutSettings(c, a, c.Param("businessId"), c.Param("branchId"), OperationsSettings{SchemaVersion: r.SchemaVersion, AcceptingNewOrders: r.AcceptingNewOrders, PickupEnabled: r.PickupEnabled}, r.ExpectedVersion)
	})
}
func (h *Handler) ListBranchCapabilities(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.ListBranchCapabilities(c, a, c.Param("businessId"), c.Param("branchId"))
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) PutBranchCapability(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r branchCapabilityRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.PutBranchCapability(c, a, c.Param("businessId"), c.Param("branchId"), c.Param("capabilityId"), r.ExpectedVersion)
	})
}
func (h *Handler) branchCapabilityStatus(c *gin.Context, to BranchCapabilityStatus) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r commandRequest
		if err := strict(raw, &r); err != nil {
			return nil, err
		}
		return h.service.ChangeBranchCapabilityStatus(c, a, c.Param("businessId"), c.Param("branchId"), c.Param("capabilityId"), to, r.ExpectedVersion)
	})
}

func RegisterRoutes(group *gin.RouterGroup, h *Handler) {
	group.GET("/capability-types", h.CapabilityTypes)
	group.POST("/businesses", h.CreateBusiness)
	group.GET("/businesses", h.ListBusinesses)
	group.GET("/businesses/:businessId", h.GetBusiness)
	group.PATCH("/businesses/:businessId", h.UpdateBusiness)
	group.POST("/businesses/:businessId/pause", func(c *gin.Context) { h.businessStatus(c, BusinessPaused) })
	group.POST("/businesses/:businessId/resume", func(c *gin.Context) { h.businessStatus(c, BusinessActive) })
	group.GET("/businesses/:businessId/capabilities", h.ListCapabilities)
	group.POST("/businesses/:businessId/capabilities", h.CreateCapability)
	group.GET("/businesses/:businessId/capabilities/:capabilityId", h.GetCapability)
	group.POST("/businesses/:businessId/capabilities/:capabilityId/pause", func(c *gin.Context) { h.capabilityStatus(c, CapabilityPaused) })
	group.POST("/businesses/:businessId/capabilities/:capabilityId/resume", func(c *gin.Context) { h.capabilityStatus(c, CapabilityActive) })
	group.GET("/businesses/:businessId/brands", h.ListBrands)
	group.POST("/businesses/:businessId/brands", h.CreateBrand)
	group.GET("/businesses/:businessId/brands/:brandId", h.GetBrand)
	group.PATCH("/businesses/:businessId/brands/:brandId", h.UpdateBrand)
	group.GET("/businesses/:businessId/branches", h.ListBranches)
	group.POST("/businesses/:businessId/branches", h.CreateBranch)
	group.GET("/businesses/:businessId/branches/:branchId", h.GetBranch)
	group.PATCH("/businesses/:businessId/branches/:branchId", h.UpdateBranch)
	group.POST("/businesses/:businessId/branches/:branchId/pause", func(c *gin.Context) { h.branchStatus(c, BranchPaused) })
	group.POST("/businesses/:businessId/branches/:branchId/resume", func(c *gin.Context) { h.branchStatus(c, BranchActive) })
	group.GET("/businesses/:businessId/branches/:branchId/hours", h.GetHours)
	group.PUT("/businesses/:businessId/branches/:branchId/hours", h.PutHours)
	group.GET("/businesses/:businessId/branches/:branchId/service-area", h.GetServiceArea)
	group.PUT("/businesses/:businessId/branches/:branchId/service-area", h.PutServiceArea)
	group.GET("/businesses/:businessId/branches/:branchId/operations-settings", h.GetSettings)
	group.PUT("/businesses/:businessId/branches/:branchId/operations-settings", h.PutSettings)
	group.GET("/businesses/:businessId/branches/:branchId/capabilities", h.ListBranchCapabilities)
	group.PUT("/businesses/:businessId/branches/:branchId/capabilities/:capabilityId", h.PutBranchCapability)
	group.POST("/businesses/:businessId/branches/:branchId/capabilities/:capabilityId/pause", func(c *gin.Context) { h.branchCapabilityStatus(c, BranchCapabilityPaused) })
	group.POST("/businesses/:businessId/branches/:branchId/capabilities/:capabilityId/resume", func(c *gin.Context) { h.branchCapabilityStatus(c, BranchCapabilityActive) })
}
