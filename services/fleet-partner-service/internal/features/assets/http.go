package assets

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
		{"GET", "/assets", "listAssets", "assets.read", "ASSETS_ENABLED", "corporate", false, false}, {"POST", "/assets", "createAsset", "assets.manage", "ASSETS_ENABLED", "corporate", true, false}, {"GET", "/assets/:assetId", "getAsset", "assets.read", "ASSETS_ENABLED", "corporate", false, false}, {"PATCH", "/assets/:assetId", "updateAsset", "assets.manage", "ASSETS_ENABLED", "corporate", false, true}, {"POST", "/assets/:assetId/archive", "archiveAsset", "assets.archive", "ASSETS_ENABLED", "corporate", true, true}, {"POST", "/assets/:assetId/retire", "retireAsset", "assets.retire", "ASSETS_ENABLED", "corporate", true, true}, {"POST", "/assets/:assetId/assignments", "assignAsset", "assets.assign", "ASSET_ASSIGNMENTS_ENABLED", "corporate", true, false},
		{"GET", "/assets/:assetId/documents", "listAssetDocuments", "assets.read", "ASSETS_ENABLED", "corporate", false, false}, {"POST", "/assets/:assetId/documents", "addAssetDocument", "assets.manage", "ASSETS_ENABLED", "corporate", true, false},
		{"GET", "/inspection-templates", "listInspectionTemplates", "inspections.read", "INSPECTIONS_ENABLED", "corporate", false, false}, {"POST", "/inspection-templates", "createInspectionTemplate", "inspections.manage", "INSPECTIONS_ENABLED", "corporate", true, false}, {"GET", "/inspection-templates/:templateId", "getInspectionTemplate", "inspections.read", "INSPECTIONS_ENABLED", "corporate", false, false}, {"PATCH", "/inspection-templates/:templateId", "updateInspectionTemplate", "inspections.manage", "INSPECTIONS_ENABLED", "corporate", false, true},
		{"GET", "/inspections", "listInspections", "inspections.read", "INSPECTIONS_ENABLED", "corporate", false, false}, {"POST", "/inspections", "createInspection", "inspections.manage", "INSPECTIONS_ENABLED", "corporate", true, false}, {"GET", "/inspections/:inspectionId", "getInspection", "inspections.read", "INSPECTIONS_ENABLED", "corporate", false, false}, {"POST", "/inspections/:inspectionId/complete", "completeInspection", "inspections.complete", "INSPECTIONS_ENABLED", "corporate", true, true},
		{"GET", "/maintenance/schedules", "listMaintenanceSchedules", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"POST", "/maintenance/schedules", "createMaintenanceSchedule", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", true, false}, {"GET", "/maintenance/schedules/:scheduleId", "getMaintenanceSchedule", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"PATCH", "/maintenance/schedules/:scheduleId", "updateMaintenanceSchedule", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", false, true},
		{"GET", "/maintenance/work-orders", "listWorkOrders", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"POST", "/maintenance/work-orders", "createWorkOrder", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", true, false}, {"GET", "/maintenance/work-orders/:workOrderId", "getWorkOrder", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"PATCH", "/maintenance/work-orders/:workOrderId", "updateWorkOrder", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", false, true}, {"POST", "/maintenance/work-orders/:workOrderId/approve", "approveWorkOrder", "maintenance.approve", "MAINTENANCE_ENABLED", "corporate", true, true}, {"POST", "/maintenance/work-orders/:workOrderId/complete", "completeWorkOrder", "maintenance.complete", "MAINTENANCE_ENABLED", "corporate", true, true}, {"POST", "/maintenance/work-orders/:workOrderId/return-to-service", "returnToService", "maintenance.return_to_service", "MAINTENANCE_ENABLED", "corporate", true, true},
		{"GET", "/breakdowns", "listBreakdowns", "breakdowns.read", "BREAKDOWNS_ENABLED", "corporate", false, false}, {"POST", "/breakdowns", "createBreakdown", "breakdowns.manage", "BREAKDOWNS_ENABLED", "corporate", true, false}, {"GET", "/breakdowns/:breakdownId", "getBreakdown", "breakdowns.read", "BREAKDOWNS_ENABLED", "corporate", false, false}, {"PATCH", "/breakdowns/:breakdownId", "updateBreakdown", "breakdowns.manage", "BREAKDOWNS_ENABLED", "corporate", false, true},
		{"GET", "/inventory/parts", "listParts", "inventory.read", "PARTS_INVENTORY_ENABLED", "corporate", false, false}, {"POST", "/inventory/parts", "createPart", "inventory.manage", "PARTS_INVENTORY_ENABLED", "corporate", true, false}, {"GET", "/inventory/parts/:partId", "getPart", "inventory.read", "PARTS_INVENTORY_ENABLED", "corporate", false, false}, {"PATCH", "/inventory/parts/:partId", "updatePart", "inventory.manage", "PARTS_INVENTORY_ENABLED", "corporate", false, true}, {"GET", "/inventory/stock-movements", "listStockMovements", "inventory.read", "PARTS_INVENTORY_ENABLED", "corporate", false, false}, {"POST", "/inventory/stock-movements", "createStockMovement", "inventory.adjust", "PARTS_INVENTORY_ENABLED", "corporate", true, false},
		{"GET", "/fuel-records", "listFuelRecords", "fuel.read", "FUEL_RECORDS_ENABLED", "corporate", false, false}, {"POST", "/fuel-records", "createFuelRecord", "fuel.manage", "FUEL_RECORDS_ENABLED", "corporate", true, false}, {"GET", "/fuel-records/:fuelRecordId", "getFuelRecord", "fuel.read", "FUEL_RECORDS_ENABLED", "corporate", false, false}, {"POST", "/fuel-records/:fuelRecordId/approve", "approveFuelRecord", "fuel.approve", "FUEL_RECORDS_ENABLED", "corporate", true, true}, {"POST", "/fuel-records/:fuelRecordId/reject", "rejectFuelRecord", "fuel.approve", "FUEL_RECORDS_ENABLED", "corporate", true, true}}
}

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
func bind(c *gin.Context, v any) bool {
	if e := c.ShouldBindJSON(v); e != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", e))
		return false
	}
	return true
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
	if e != nil {
		respond.Error(c, apperrors.BadRequest("Request body could not be read.", e))
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if len(raw) == 0 || !json.Valid(raw) {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
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
	if strings.HasPrefix(r.OperationID, "create") || strings.HasPrefix(r.OperationID, "add") || strings.HasPrefix(r.OperationID, "assign") {
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
func ver(c *gin.Context, v int64) int64 {
	if v > 0 {
		return v
	}
	n, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return n
}

type vb struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	id := ""
	for _, p := range []string{"assetId", "templateId", "inspectionId", "scheduleId", "workOrderId", "breakdownId", "partId", "fuelRecordId"} {
		if c.Param(p) != "" {
			id = c.Param(p)
		}
	}
	run := func() (any, error) {
		switch r.OperationID {
		case "listAssets":
			l, _ := strconv.Atoi(c.Query("limit"))
			o, _ := strconv.Atoi(c.Query("offset"))
			return h.s.ListAssets(c, t, Page{BranchID: c.Query("branchId"), Status: strings.ToUpper(c.Query("status")), Limit: l, Offset: o})
		case "getAsset":
			return h.s.GetAsset(c, t, id)
		case "createAsset":
			var b Asset
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateAsset(c, t, b)
		case "updateAsset":
			var b Asset
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateAsset(c, t, id, ver(c, b.Version), b, "")
		case "archiveAsset", "retireAsset":
			var b vb
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			a := "ARCHIVE"
			if r.OperationID == "retireAsset" {
				a = "RETIRE"
			}
			return h.s.UpdateAsset(c, t, id, ver(c, b.ExpectedVersion), Asset{Capacity: -1, Odometer: -1, OperatingHours: -1}, a)
		case "assignAsset":
			var b Assignment
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.Assign(c, t, id, b)
		case "listAssetDocuments":
			return h.s.ListDocuments(c, t, id)
		case "addAssetDocument":
			var b Document
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.AddDocument(c, t, id, b)
		case "listInspectionTemplates":
			return h.s.ListTemplates(c, t)
		case "getInspectionTemplate":
			return h.s.GetTemplate(c, t, id)
		case "createInspectionTemplate":
			var b Template
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.SaveTemplate(c, t, "", 0, b)
		case "updateInspectionTemplate":
			var b Template
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.SaveTemplate(c, t, id, ver(c, b.Version), b)
		case "listInspections":
			return h.s.ListInspections(c, t, Page{Status: strings.ToUpper(c.Query("status"))})
		case "getInspection":
			return h.s.GetInspection(c, t, id)
		case "createInspection":
			var b Inspection
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateInspection(c, t, b)
		case "completeInspection":
			var b CompleteInspectionInput
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			b.ExpectedVersion = ver(c, b.ExpectedVersion)
			return h.s.CompleteInspection(c, t, id, b)
		case "listMaintenanceSchedules":
			return h.s.ListSchedules(c, t)
		case "getMaintenanceSchedule":
			return h.s.GetSchedule(c, t, id)
		case "createMaintenanceSchedule":
			var b Schedule
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.SaveSchedule(c, t, "", 0, b)
		case "updateMaintenanceSchedule":
			var b Schedule
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.SaveSchedule(c, t, id, ver(c, b.Version), b)
		case "listWorkOrders":
			return h.s.ListWorkOrders(c, t)
		case "getWorkOrder":
			return h.s.GetWorkOrder(c, t, id)
		case "createWorkOrder":
			var b WorkOrder
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateWorkOrder(c, t, b)
		case "updateWorkOrder", "approveWorkOrder", "completeWorkOrder", "returnToService":
			var b WorkAction
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			b.ExpectedVersion = ver(c, b.ExpectedVersion)
			if r.OperationID == "returnToService" {
				return h.s.ReturnToService(c, t, id, b)
			}
			a := ""
			if r.OperationID == "approveWorkOrder" {
				a = "APPROVE"
			}
			if r.OperationID == "completeWorkOrder" {
				a = "COMPLETE"
			}
			return h.s.UpdateWorkOrder(c, t, id, a, b)
		case "listBreakdowns":
			return h.s.ListBreakdowns(c, t)
		case "getBreakdown":
			return h.s.GetBreakdown(c, t, id)
		case "createBreakdown":
			var b Breakdown
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.SaveBreakdown(c, t, "", 0, b)
		case "updateBreakdown":
			var b Breakdown
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.SaveBreakdown(c, t, id, ver(c, b.Version), b)
		case "listParts":
			return h.s.ListParts(c, t)
		case "getPart":
			return h.s.GetPart(c, t, id)
		case "createPart":
			var b Part
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.SavePart(c, t, "", 0, b)
		case "updatePart":
			var b Part
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.SavePart(c, t, id, ver(c, b.Version), b)
		case "listStockMovements":
			return h.s.ListMovements(c, t)
		case "createStockMovement":
			var b StockMovement
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateMovement(c, t, b)
		case "listFuelRecords":
			return h.s.ListFuel(c, t)
		case "getFuelRecord":
			return h.s.GetFuel(c, t, id)
		case "createFuelRecord":
			var b FuelRecord
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateFuel(c, t, b)
		case "approveFuelRecord", "rejectFuelRecord":
			var b vb
			if !bind(c, &b) {
				return nil, errors.New("bound")
			}
			to := "APPROVED"
			if r.OperationID == "rejectFuelRecord" {
				to = "REJECTED"
			}
			return h.s.FuelAction(c, t, id, to, b.Reason, ver(c, b.ExpectedVersion))
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
