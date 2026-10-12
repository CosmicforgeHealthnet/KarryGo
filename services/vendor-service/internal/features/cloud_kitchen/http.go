package cloudkitchen

import (
	"cosmicforge/logistics/services/vendor-service/internal/features/crm"
	"encoding/json"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
)

type Handler struct {
	crm         *crm.Service
	service     *Service
	idempotency *idempotency.Store
}

func productionOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func NewHandler(s *Service) *Handler { return &Handler{service: s} }
func (h *Handler) UseIdempotency(store *idempotency.Store) *Handler {
	h.idempotency = store
	return h
}
func (h *Handler) auth(c *gin.Context, manage bool) (production.Actor, production.Scope, bool) {
	st, ok := authctx.FromContext(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return production.Actor{}, production.Scope{}, false
	}
	a := production.Actor{StaffID: st.StaffID, VendorID: st.VendorID, SessionID: st.SessionID, CorrelationID: httpx.GetRequestID(c), CausationID: c.GetHeader("X-Causation-ID"), CommandRef: c.GetHeader("Idempotency-Key")}
	perm := PermRead
	if manage {
		perm = PermManage
	}
	scope, err := h.service.Production().Authorize(c, a, perm)
	if err != nil {
		httpx.RespondError(c, err)
		return a, scope, false
	}
	return a, scope, true
}
func (h *Handler) settings(c *gin.Context) {
	a, scope, ok := h.auth(c, false)
	if !ok {
		return
	}
	branch := c.Param("branchId")
	if !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.Forbidden("Forbidden.", nil))
		return
	}
	v, err := h.service.GetSettings(c, a.VendorID, branch)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": v})
}
func (h *Handler) putSettings(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	branch := c.Param("branchId")
	if !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.Forbidden("Forbidden.", nil))
		return
	}
	var v Settings
	if c.ShouldBindJSON(&v) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	v.BranchID = branch
	out, err := h.service.PutSettings(c, a.VendorID, v)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) listPackages(c *gin.Context) {
	a, scope, ok := h.auth(c, false)
	if !ok {
		return
	}
	out, err := h.service.ListPackages(c, a.VendorID, scope)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) createPackage(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var p Package
	if c.ShouldBindJSON(&p) != nil || !scope.Covers(p.BranchID) {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	id, err := h.service.CreatePackage(c, a.VendorID, p)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id}})
}
func (h *Handler) bands(c *gin.Context) {
	a, scope, ok := h.auth(c, false)
	if !ok {
		return
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("packageId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Package not found.", nil))
		return
	}
	rows, err := h.service.DB().Query(c, `SELECT b.id,min_quantity,max_quantity,unit_price_minor,package_price_minor,lead_time_minutes,capacity_units_per_item,b.version,b.status,b.archived_at FROM cloud_kitchen_quantity_bands b JOIN cloud_kitchen_packages p ON p.id=b.package_id WHERE p.business_id=$1 AND b.package_id=$2 ORDER BY min_quantity`, a.VendorID, c.Param("packageId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id string
		var min int
		var max *int
		var unit, pack int64
		var lead *int
		var weight int
		var ver int64
		var status string
		var archivedAt *time.Time
		if rows.Scan(&id, &min, &max, &unit, &pack, &lead, &weight, &ver, &status, &archivedAt) == nil {
			out = append(out, gin.H{"id": id, "min_quantity": min, "max_quantity": max, "unit_price_minor": unit, "package_price_minor": pack, "lead_time_minutes": lead, "capacity_units_per_item": weight, "version": ver, "status": status, "archived_at": archivedAt})
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

type bandMutationRequest struct {
	MinQuantity            int   `json:"min_quantity"`
	MaxQuantity            *int  `json:"max_quantity"`
	UnitPriceMinor         int64 `json:"unit_price_minor"`
	PackagePriceMinor      int64 `json:"package_price_minor"`
	LeadTimeMinutes        *int  `json:"lead_time_minutes"`
	CapacityUnitsPerItem   int   `json:"capacity_units_per_item"`
	ExpectedPackageVersion int64 `json:"expected_package_version"`
	ExpectedBandVersion    int64 `json:"expected_band_version"`
}

func (h *Handler) updateBand(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("packageId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Package not found.", nil))
		return
	}
	var in bandMutationRequest
	if c.ShouldBindJSON(&in) != nil || in.ExpectedPackageVersion <= 0 || in.ExpectedBandVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("Positive expected package and band versions are required.", nil))
		return
	}
	if err := h.service.UpdateBand(c, a.VendorID, c.Param("packageId"), c.Param("bandId"), in.MinQuantity, in.MaxQuantity, in.UnitPriceMinor, in.PackagePriceMinor, in.LeadTimeMinutes, in.CapacityUnitsPerItem, in.ExpectedPackageVersion, in.ExpectedBandVersion); err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, gin.H{"id": c.Param("bandId"), "version": in.ExpectedBandVersion + 1, "package_version": in.ExpectedPackageVersion + 1})
}

func (h *Handler) archiveBand(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("packageId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Package not found.", nil))
		return
	}
	var in struct {
		ExpectedPackageVersion int64 `json:"expected_package_version"`
		ExpectedBandVersion    int64 `json:"expected_band_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedPackageVersion <= 0 || in.ExpectedBandVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("Positive expected package and band versions are required.", nil))
		return
	}
	if err := h.service.ArchiveBand(c, a.VendorID, c.Param("packageId"), c.Param("bandId"), in.ExpectedPackageVersion, in.ExpectedBandVersion); err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, gin.H{"archived": true, "package_version": in.ExpectedPackageVersion + 1})
}
func (h *Handler) addBand(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var in struct {
		MinQuantity          int   `json:"min_quantity"`
		MaxQuantity          *int  `json:"max_quantity"`
		UnitPriceMinor       int64 `json:"unit_price_minor"`
		PackagePriceMinor    int64 `json:"package_price_minor"`
		LeadTimeMinutes      *int  `json:"lead_time_minutes"`
		CapacityUnitsPerItem int   `json:"capacity_units_per_item"`
		ExpectedVersion      int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("packageId")).Scan(&branch) != nil {
		httpx.RespondError(c, apperrors.NotFound("Package not found.", nil))
		return
	}
	if !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.Forbidden("Forbidden.", nil))
		return
	}
	_, scopeErr := h.service.Production().Authorize(c, a, PermManage)
	if scopeErr != nil {
		httpx.RespondError(c, scopeErr)
		return
	}
	if in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	id, err := h.service.AddBand(c, c.Param("packageId"), in.MinQuantity, in.MaxQuantity, in.UnitPriceMinor, in.PackagePriceMinor, in.LeadTimeMinutes, in.CapacityUnitsPerItem, in.ExpectedVersion)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id}})
}
func (h *Handler) preview(c *gin.Context) {
	a, scope, ok := h.auth(c, false)
	if !ok {
		return
	}
	var owner, branch string
	if h.service.DB().QueryRow(c, `SELECT business_id,branch_id FROM cloud_kitchen_packages WHERE id=$1`, c.Param("packageId")).Scan(&owner, &branch) != nil || owner != a.VendorID || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Package not found.", nil))
		return
	}
	q, _ := strconv.Atoi(c.Query("quantity"))
	var start, end time.Time
	var err error
	if raw := c.Query("start_at"); raw != "" {
		start, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			httpx.RespondError(c, apperrors.BadRequest("start_at must be RFC3339.", nil))
			return
		}
	}
	if raw := c.Query("end_at"); raw != "" {
		end, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			httpx.RespondError(c, apperrors.BadRequest("end_at must be RFC3339.", nil))
			return
		}
	}
	out, err := h.service.Preview(c, a, scope, BulkPreviewInput{PackageID: c.Param("packageId"), Quantity: q, WindowStart: start, WindowEnd: end})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) plans(c *gin.Context) {
	a, scope, ok := h.auth(c, false)
	if !ok {
		return
	}
	out, err := h.service.ListPlans(c, a.VendorID, scope)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
func (h *Handler) createPlan(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var p Plan
	if c.ShouldBindJSON(&p) != nil || !scope.Covers(p.BranchID) {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	if !h.validateCorporate(c, a, p.BranchID, p.CorporateProfileRef) {
		return
	}
	id, err := h.service.CreatePlan(c, a.VendorID, p)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id}})
}
func (h *Handler) status(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	status := c.Param("status")
	if status != "ACTIVE" && status != "SUSPENDED" && status != "ARCHIVED" {
		httpx.RespondError(c, apperrors.BadRequest("Invalid status.", nil))
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	if err := h.service.SetPlanStatus(c, a.VendorID, c.Param("planId"), status, in.ExpectedVersion, scope); err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
func (h *Handler) exception(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var in struct {
		OccurrenceDate  string  `json:"occurrence_date"`
		Action          string  `json:"action"`
		ReplacementDate *string `json:"replacement_date"`
		Quantity        *int    `json:"quantity_override"`
		Package         *string `json:"package_override"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", nil))
		return
	}
	var business, branch string
	if h.service.DB().QueryRow(c, `SELECT business_id,branch_id FROM cloud_kitchen_corporate_plans WHERE id=$1`, c.Param("planId")).Scan(&business, &branch) != nil || business != a.VendorID || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Corporate plan not found.", nil))
		return
	}
	d, e := time.Parse("2006-01-02", in.OccurrenceDate)
	if e != nil {
		httpx.RespondError(c, apperrors.BadRequest("Invalid date.", nil))
		return
	}
	var r *time.Time
	if in.ReplacementDate != nil {
		x, e := time.Parse("2006-01-02", *in.ReplacementDate)
		if e != nil {
			httpx.RespondError(c, apperrors.BadRequest("Invalid date.", nil))
			return
		}
		r = &x
	}
	if err := h.service.AddException(c, c.Param("planId"), d, in.Action, r, in.Quantity, in.Package); err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true})
}

func (h *Handler) getPackage(c *gin.Context) {
	a, scope, ok := h.auth(c, false)
	if !ok {
		return
	}
	var p Package
	var raw []byte
	err := h.service.DB().QueryRow(c, `SELECT id,branch_id,name,description,status,minimum_quantity,maximum_quantity,capacity_pool_id,production_class,lead_time_minutes,setup_buffer_minutes,cleanup_buffer_minutes,logistics,version FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("packageId")).Scan(&p.ID, &p.BranchID, &p.Name, &p.Description, &p.Status, &p.MinimumQuantity, &p.MaximumQuantity, &p.CapacityPoolID, &p.ProductionClass, &p.LeadTimeMinutes, &p.SetupBufferMinutes, &p.CleanupBufferMinutes, &raw, &p.Version)
	if err != nil || !scope.Covers(p.BranchID) {
		httpx.RespondError(c, apperrors.NotFound("Package not found.", nil))
		return
	}
	_ = json.Unmarshal(raw, &p.Logistics)
	productionOK(c, p)
}

func (h *Handler) updatePackage(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var in Package
	if c.ShouldBindJSON(&in) != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check the package details.", nil))
		return
	}
	var branch string
	var version int64
	if h.service.DB().QueryRow(c, `SELECT branch_id,version FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("packageId")).Scan(&branch, &version) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Package not found.", nil))
		return
	}
	if in.Version <= 0 || in.Version != version {
		httpx.RespondError(c, apperrors.Conflict("The package changed; reload and retry.", nil))
		return
	}
	err := h.service.UpdatePackage(c, a.VendorID, c.Param("packageId"), in, version)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, gin.H{"updated": true, "version": version + 1})
}

func (h *Handler) archivePackage(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("packageId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Package not found.", nil))
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	e := h.service.ArchivePackage(c, a.VendorID, c.Param("packageId"), in.ExpectedVersion)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	productionOK(c, gin.H{"archived": true})
}

func (h *Handler) components(c *gin.Context) {
	a, scope, ok := h.auth(c, false)
	if !ok {
		return
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("packageId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Package not found.", nil))
		return
	}
	rows, e := h.service.DB().Query(c, `SELECT id,item_ref,default_quantity,variant_ref,position FROM cloud_kitchen_package_components WHERE package_id=$1 ORDER BY position,id`, c.Param("packageId"))
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, item string
		var qty, pos int
		var variant *string
		if rows.Scan(&id, &item, &qty, &variant, &pos) == nil {
			out = append(out, gin.H{"id": id, "item_ref": item, "default_quantity": qty, "variant_ref": variant, "position": pos})
		}
	}
	productionOK(c, out)
}

func (h *Handler) addComponent(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("packageId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Package not found.", nil))
		return
	}
	var in struct {
		ItemRef         string  `json:"item_ref"`
		DefaultQuantity int     `json:"default_quantity"`
		VariantRef      *string `json:"variant_ref"`
		Position        int     `json:"position"`
		ExpectedVersion int64   `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.DefaultQuantity <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("Check the component details.", nil))
		return
	}
	if in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	id, e := h.service.AddComponent(c, a.VendorID, c.Param("packageId"), in.ItemRef, in.DefaultQuantity, in.VariantRef, in.Position, in.ExpectedVersion)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id}})
}

func (h *Handler) removeComponent(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("packageId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Package not found.", nil))
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	if err := h.service.RemoveComponent(c, a.VendorID, c.Param("packageId"), c.Param("componentId"), in.ExpectedVersion); err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, gin.H{"removed": true, "package_version": in.ExpectedVersion + 1})
}

func (h *Handler) getPlan(c *gin.Context) {
	a, scope, ok := h.auth(c, false)
	if !ok {
		return
	}
	var p Plan
	if e := h.service.DB().QueryRow(c, `SELECT id,branch_id,corporate_profile_ref,name,package_id,recurrence_type,quantity,weekdays,timezone,to_char(local_time,'HH24:MI'),effective_from,effective_until,production_minutes,lead_time_minutes,reservation_horizon_days,status,version FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("planId")).Scan(&p.ID, &p.BranchID, &p.CorporateProfileRef, &p.Name, &p.PackageID, &p.RecurrenceType, &p.Quantity, &p.Weekdays, &p.Timezone, &p.LocalTime, &p.EffectiveFrom, &p.EffectiveUntil, &p.ProductionMinutes, &p.LeadTimeMinutes, &p.ReservationHorizonDays, &p.Status, &p.Version); e != nil || !scope.Covers(p.BranchID) {
		httpx.RespondError(c, apperrors.NotFound("Corporate plan not found.", nil))
		return
	}
	productionOK(c, p)
}
func (h *Handler) transitionPlan(c *gin.Context, status string) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var in struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	if e := h.service.SetPlanStatus(c, a.VendorID, c.Param("planId"), status, in.ExpectedVersion, scope); e != nil {
		httpx.RespondError(c, e)
		return
	}
	productionOK(c, gin.H{"status": status})
}

func (h *Handler) updatePlan(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var in Plan
	if c.ShouldBindJSON(&in) != nil || in.Version <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("Check the corporate plan details and version.", nil))
		return
	}
	var branch, savedReference string
	if err := h.service.DB().QueryRow(c, `SELECT branch_id,corporate_profile_ref FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("planId")).Scan(&branch, &savedReference); err != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Corporate plan not found.", nil))
		return
	}
	if in.CorporateProfileRef != savedReference && !h.validateCorporate(c, a, branch, in.CorporateProfileRef) {
		return
	}
	version, err := h.service.UpdatePlan(c, a.VendorID, c.Param("planId"), in, scope)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, gin.H{"id": c.Param("planId"), "version": version})
}
func (h *Handler) activatePlan(c *gin.Context) { h.transitionPlan(c, "ACTIVE") }
func (h *Handler) suspendPlan(c *gin.Context)  { h.transitionPlan(c, "SUSPENDED") }
func (h *Handler) resumePlan(c *gin.Context)   { h.transitionPlan(c, "ACTIVE") }
func (h *Handler) archivePlan(c *gin.Context)  { h.transitionPlan(c, "ARCHIVED") }
func (h *Handler) occurrences(c *gin.Context) {
	a, scope, ok := h.auth(c, false)
	if !ok {
		return
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("planId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Corporate plan not found.", nil))
		return
	}
	rows, e := h.service.DB().Query(c, `SELECT id,occurrence_key,occurrence_date,starts_at,ends_at,quantity,package_id,state,reservation_id,production_job_id,conflict_reason,last_error,attempt_count,replay_count,lease_owner,lease_expires_at,dead_lettered_at,version FROM cloud_kitchen_occurrences WHERE plan_id=$1 ORDER BY occurrence_date`, c.Param("planId"))
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, key, pkg, state string
		var date, start, end time.Time
		var qty int
		var reservation, productionJob, reason, leaseOwner *string
		var lastError string
		var attemptCount, replayCount int
		var version int64
		var leaseExpiresAt, deadLetteredAt *time.Time
		if rows.Scan(&id, &key, &date, &start, &end, &qty, &pkg, &state, &reservation, &productionJob, &reason, &lastError, &attemptCount, &replayCount, &leaseOwner, &leaseExpiresAt, &deadLetteredAt, &version) == nil {
			out = append(out, gin.H{"id": id, "occurrence_key": key, "occurrence_date": date, "starts_at": start, "ends_at": end, "quantity": qty, "package_id": pkg, "state": state, "reservation_id": reservation, "production_job_id": productionJob, "conflict_reason": reason, "last_error": lastError, "attempt_count": attemptCount, "replay_count": replayCount, "lease_owner": leaseOwner, "lease_expires_at": leaseExpiresAt, "dead_lettered_at": deadLetteredAt, "version": version})
		}
	}
	productionOK(c, out)
}

func (h *Handler) retryOccurrence(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var in struct {
		Reason          string `json:"reason"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	if in.Reason == "" {
		in.Reason = "manual retry"
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("planId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Corporate occurrence not found.", nil))
		return
	}
	if e := h.service.RetryOccurrenceForBusiness(c, a.VendorID, c.Param("planId"), c.Param("occurrenceId"), in.Reason, in.ExpectedVersion); e != nil {
		httpx.RespondError(c, e)
		return
	}
	productionOK(c, gin.H{"state": "PLANNED", "version": in.ExpectedVersion + 1})
}

func (h *Handler) replayOccurrence(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var in struct {
		Reason          string `json:"reason"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A positive expected_version is required.", nil))
		return
	}
	if in.Reason == "" {
		in.Reason = "manual dead-letter replay"
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("planId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Corporate occurrence not found.", nil))
		return
	}
	if e := h.service.ReplayDeadLetterForBusiness(c, a.VendorID, c.Param("planId"), c.Param("occurrenceId"), in.Reason, in.ExpectedVersion); e != nil {
		httpx.RespondError(c, e)
		return
	}
	productionOK(c, gin.H{"state": "PLANNED", "replayed": true, "version": in.ExpectedVersion + 1})
}

func (h *Handler) rescheduleOccurrence(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var in struct {
		StartsAt        time.Time `json:"starts_at"`
		EndsAt          time.Time `json:"ends_at"`
		ExpectedVersion int64     `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("Check occurrence window.", nil))
		return
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("planId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Corporate occurrence not found.", nil))
		return
	}
	if e := h.service.RescheduleOccurrence(c, a.VendorID, c.Param("planId"), c.Param("occurrenceId"), in.StartsAt, in.EndsAt, in.ExpectedVersion); e != nil {
		httpx.RespondError(c, e)
		return
	}
	productionOK(c, gin.H{"state": "PLANNED", "starts_at": in.StartsAt, "ends_at": in.EndsAt, "version": in.ExpectedVersion + 1})
}

func (h *Handler) cancelOccurrence(c *gin.Context) {
	a, scope, ok := h.auth(c, true)
	if !ok {
		return
	}
	var branch string
	if h.service.DB().QueryRow(c, `SELECT branch_id FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND id=$2`, a.VendorID, c.Param("planId")).Scan(&branch) != nil || !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.NotFound("Corporate occurrence not found.", nil))
		return
	}
	var in struct {
		Reason          string `json:"reason"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ExpectedVersion <= 0 {
		httpx.RespondError(c, apperrors.BadRequest("A cancellation reason is required.", nil))
		return
	}
	if err := h.service.CancelOccurrence(c, a, scope, c.Param("planId"), c.Param("occurrenceId"), in.Reason, in.ExpectedVersion); err != nil {
		httpx.RespondError(c, err)
		return
	}
	productionOK(c, gin.H{"state": "CANCELLED", "version": in.ExpectedVersion + 1})
}

func RegisterRoutes(group *gin.RouterGroup, h *Handler, flags production.Flags) {
	if !flags.ProductionEnabled {
		return
	}
	g := group.Group("/cloud-kitchen")
	g.Use(idempotency.CommandMiddleware(h.idempotency))
	g.GET("/context/:branchId", h.context)
	g.GET("/corporate-references", h.corporateReferences)
	g.GET("/settings/:branchId", h.settings)
	g.PUT("/settings/:branchId", h.putSettings)
	g.GET("/bulk-packages", h.listPackages)
	g.POST("/bulk-packages", h.createPackage)
	g.GET("/bulk-packages/:packageId", h.getPackage)
	g.PATCH("/bulk-packages/:packageId", h.updatePackage)
	g.POST("/bulk-packages/:packageId/archive", h.archivePackage)
	g.GET("/bulk-packages/:packageId/components", h.components)
	g.POST("/bulk-packages/:packageId/components", h.addComponent)
	g.DELETE("/bulk-packages/:packageId/components/:componentId", h.removeComponent)
	g.GET("/bulk-packages/:packageId/bands", h.bands)
	g.POST("/bulk-packages/:packageId/bands", h.addBand)
	g.PATCH("/bulk-packages/:packageId/bands/:bandId", h.updateBand)
	g.POST("/bulk-packages/:packageId/bands/:bandId/archive", h.archiveBand)
	g.GET("/bulk-packages/:packageId/capacity-preview", h.preview)
	g.GET("/corporate-plans", h.plans)
	g.POST("/corporate-plans", h.createPlan)
	g.GET("/corporate-plans/:planId", h.getPlan)
	g.PATCH("/corporate-plans/:planId", h.updatePlan)
	g.POST("/corporate-plans/:planId/activate", h.activatePlan)
	g.POST("/corporate-plans/:planId/suspend", h.suspendPlan)
	g.POST("/corporate-plans/:planId/resume", h.resumePlan)
	g.POST("/corporate-plans/:planId/archive", h.archivePlan)
	g.POST("/corporate-plans/:planId/:status", h.status)
	g.POST("/corporate-plans/:planId/exceptions", h.exception)
	g.GET("/corporate-plans/:planId/occurrences", h.occurrences)
	g.POST("/corporate-plans/:planId/occurrences/:occurrenceId/retry", h.retryOccurrence)
	g.POST("/corporate-plans/:planId/occurrences/:occurrenceId/replay", h.replayOccurrence)
	g.POST("/corporate-plans/:planId/occurrences/:occurrenceId/reschedule", h.rescheduleOccurrence)
	g.POST("/corporate-plans/:planId/occurrences/:occurrenceId/cancel", h.cancelOccurrence)
}
