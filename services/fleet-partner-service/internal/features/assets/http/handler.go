// Package http serves the Section 18 asset routes: the asset register,
// assignments, documents, history, costs and meter readings, breakdowns,
// parts inventory, defects and tyres.
package http

import (
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/money"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

func TenantRoutes() []Route {
	return []Route{
		{"GET", "/assets", "listAssets", "assets.read", "ASSETS_ENABLED", "corporate", false, false}, {"POST", "/assets", "createAsset", "assets.manage", "ASSETS_ENABLED", "corporate", true, false}, {"GET", "/assets/:assetId", "getAsset", "assets.read", "ASSETS_ENABLED", "corporate", false, false}, {"PATCH", "/assets/:assetId", "updateAsset", "assets.manage", "ASSETS_ENABLED", "corporate", false, true}, {"POST", "/assets/:assetId/archive", "archiveAsset", "assets.archive", "ASSETS_ENABLED", "corporate", true, true}, {"POST", "/assets/:assetId/retire", "retireAsset", "assets.retire", "ASSETS_ENABLED", "corporate", true, true}, {"POST", "/assets/:assetId/assignments", "assignAsset", "assets.assign", "ASSET_ASSIGNMENTS_ENABLED", "corporate", true, false},
		{"GET", "/assets/:assetId/documents", "listAssetDocuments", "assets.read", "ASSETS_ENABLED", "corporate", false, false}, {"POST", "/assets/:assetId/documents", "addAssetDocument", "assets.manage", "ASSETS_ENABLED", "corporate", true, false},
		{"GET", "/breakdowns", "listBreakdowns", "breakdowns.read", "BREAKDOWNS_ENABLED", "corporate", false, false}, {"POST", "/breakdowns", "createBreakdown", "breakdowns.manage", "BREAKDOWNS_ENABLED", "corporate", true, false}, {"GET", "/breakdowns/:breakdownId", "getBreakdown", "breakdowns.read", "BREAKDOWNS_ENABLED", "corporate", false, false}, {"PATCH", "/breakdowns/:breakdownId", "updateBreakdown", "breakdowns.manage", "BREAKDOWNS_ENABLED", "corporate", false, true},
		{"GET", "/inventory/parts", "listParts", "inventory.read", "PARTS_INVENTORY_ENABLED", "corporate", false, false}, {"POST", "/inventory/parts", "createPart", "inventory.manage", "PARTS_INVENTORY_ENABLED", "corporate", true, false}, {"GET", "/inventory/parts/:partId", "getPart", "inventory.read", "PARTS_INVENTORY_ENABLED", "corporate", false, false}, {"PATCH", "/inventory/parts/:partId", "updatePart", "inventory.manage", "PARTS_INVENTORY_ENABLED", "corporate", false, true}, {"GET", "/inventory/stock-movements", "listStockMovements", "inventory.read", "PARTS_INVENTORY_ENABLED", "corporate", false, false}, {"POST", "/inventory/stock-movements", "createStockMovement", "inventory.adjust", "PARTS_INVENTORY_ENABLED", "corporate", true, false},
		// Section 18.12 lifecycle gaps (spec 47.3, 48.3).
		{"POST", "/assets/:assetId/unassign-provider", "unassignAssetProvider", "assets.assign", "ASSET_ASSIGNMENTS_ENABLED", "corporate", true, true},
		{"GET", "/assets/:assetId/history", "getAssetHistory", "assets.read", "ASSETS_ENABLED", "corporate", false, false},
		{"GET", "/assets/:assetId/costs", "listAssetCosts", "assets.costs.read", "ASSETS_ENABLED", "corporate", false, false}, {"POST", "/assets/:assetId/costs", "recordAssetCost", "assets.costs.manage", "ASSETS_ENABLED", "corporate", true, false},
		{"GET", "/assets/:assetId/meter-readings", "listMeterReadings", "assets.read", "ASSETS_ENABLED", "corporate", false, false}, {"POST", "/meter-readings", "recordMeterReading", "assets.manage", "ASSETS_ENABLED", "corporate", true, false},
		{"GET", "/defects", "listDefects", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"POST", "/defects", "reportDefect", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", true, false}, {"GET", "/defects/:defectId", "getDefect", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"POST", "/defects/:defectId/defer", "deferDefect", "maintenance.approve", "MAINTENANCE_ENABLED", "corporate", true, true}, {"POST", "/defects/:defectId/start-repair", "startDefectRepair", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", true, true}, {"POST", "/defects/:defectId/resolve", "resolveDefect", "maintenance.complete", "MAINTENANCE_ENABLED", "corporate", true, true},
		{"GET", "/tyres", "listTyres", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"POST", "/tyres", "registerTyre", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", true, false}, {"GET", "/tyres/:tyreId", "getTyre", "maintenance.read", "MAINTENANCE_ENABLED", "corporate", false, false}, {"POST", "/tyres/:tyreId/install", "installTyre", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", true, true}, {"POST", "/tyres/:tyreId/remove", "removeTyre", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", true, true}, {"POST", "/tyres/:tyreId/rotate", "rotateTyre", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", true, true}, {"POST", "/tyres/:tyreId/tread-readings", "recordTyreTread", "maintenance.manage", "MAINTENANCE_ENABLED", "corporate", true, true}}
}

// costEntryView is a cost entry with its amount also as a Money object
// (spec 6.1); the flat amountMinor and currency stay for compatibility.
type costEntryView struct {
	models.CostEntry
	Amount money.Money `json:"amount"`
}
type costTotalView struct {
	models.CostTotal
	Amount money.Money `json:"amount"`
}
type costHistoryView struct {
	AssetID string          `json:"assetId"`
	Entries []costEntryView `json:"entries"`
	Totals  []costTotalView `json:"totals"`
}

func costEntry(v models.CostEntry) costEntryView { return costEntryView{v, v.Amount()} }
func costHistory(v models.CostHistory) costHistoryView {
	out := costHistoryView{AssetID: v.AssetID, Entries: []costEntryView{}, Totals: []costTotalView{}}
	for _, e := range v.Entries {
		out.Entries = append(out.Entries, costEntry(e))
	}
	for _, t := range v.Totals {
		out.Totals = append(out.Totals, costTotalView{t, t.Amount()})
	}
	return out
}

func (h *Handler) listAssets(c *gin.Context, t tenant.Context) (any, error) {
	p, e := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if e != nil {
		return nil, e
	}
	after, afterID, e := keyset(c)
	if e != nil {
		return nil, e
	}
	items, more, e := h.s.ListAssetsPage(c, t, models.Page{BranchID: c.Query("branchId"), Status: strings.ToUpper(c.Query("status")), Limit: p.Limit, Offset: p.Offset, After: after, AfterID: afterID})
	if e != nil {
		return nil, e
	}
	next := ""
	if more && len(items) > 0 {
		last := items[len(items)-1]
		next = cursor.EncodeTime(last.CreatedAt, last.ID)
	}
	return listPage{data: items, next: next}, nil
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	id := ""
	for _, p := range []string{"assetId", "breakdownId", "partId", "defectId", "tyreId"} {
		if c.Param(p) != "" {
			id = c.Param(p)
		}
	}
	h.serve(c, t, r, func() (any, error) {
		switch r.OperationID {
		case "listAssets":
			return h.listAssets(c, t)
		case "getAsset":
			return h.s.GetAsset(c, t, id)
		case "createAsset":
			var b models.Asset
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.CreateAsset(c, t, b)
		case "updateAsset":
			var b models.Asset
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.UpdateAsset(c, t, id, ver(c, b.Version), b, "")
		case "archiveAsset", "retireAsset":
			var b vb
			if !bind(c, &b) {
				return nil, errBound
			}
			a := "ARCHIVE"
			if r.OperationID == "retireAsset" {
				a = "RETIRE"
			}
			return h.s.UpdateAsset(c, t, id, ver(c, b.ExpectedVersion), models.Asset{Capacity: -1, Odometer: -1, OperatingHours: -1}, a)
		case "assignAsset":
			var b models.Assignment
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.Assign(c, t, id, b)
		case "listAssetDocuments":
			return h.s.ListDocuments(c, t, id)
		case "addAssetDocument":
			var b models.Document
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.AddDocument(c, t, id, b)
		case "listBreakdowns":
			return h.s.ListBreakdowns(c, t)
		case "getBreakdown":
			return h.s.GetBreakdown(c, t, id)
		case "createBreakdown":
			var b models.Breakdown
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.SaveBreakdown(c, t, "", 0, b)
		case "updateBreakdown":
			var b models.Breakdown
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.SaveBreakdown(c, t, id, ver(c, b.Version), b)
		case "listParts":
			return h.s.ListParts(c, t)
		case "getPart":
			return h.s.GetPart(c, t, id)
		case "createPart":
			var b models.Part
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.SavePart(c, t, "", 0, b)
		case "updatePart":
			var b models.Part
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.SavePart(c, t, id, ver(c, b.Version), b)
		case "listStockMovements":
			return h.s.ListMovements(c, t)
		case "createStockMovement":
			var b models.StockMovement
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.CreateMovement(c, t, b)
		case "unassignAssetProvider":
			var b models.UnassignInput
			if !bind(c, &b) {
				return nil, errBound
			}
			b.ExpectedVersion = ver(c, b.ExpectedVersion)
			return h.s.UnassignProvider(c, t, id, b)
		case "getAssetHistory":
			return h.s.AssetHistory(c, t, id)
		case "listAssetCosts":
			v, e := h.s.ListCosts(c, t, id)
			if e != nil {
				return nil, e
			}
			return costHistory(v), nil
		case "recordAssetCost":
			var b models.CostInput
			if !bind(c, &b) {
				return nil, errBound
			}
			v, e := h.s.RecordCost(c, t, id, b)
			if e != nil {
				return nil, e
			}
			return costEntry(v), nil
		case "listMeterReadings":
			return h.s.ListMeterReadings(c, t, id, c.Query("meterType"))
		case "recordMeterReading":
			var b models.MeterReadingInput
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.RecordMeterReading(c, t, b)
		case "listDefects":
			return h.s.ListDefects(c, t, models.DefectFilter{AssetID: c.Query("assetId"), Status: c.Query("status"), Severity: c.Query("severity")})
		case "getDefect":
			return h.s.GetDefect(c, t, id)
		case "reportDefect":
			var b models.ReportDefectInput
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.ReportDefect(c, t, b)
		case "deferDefect", "startDefectRepair", "resolveDefect":
			var b models.DefectAction
			if !bind(c, &b) {
				return nil, errBound
			}
			b.ExpectedVersion = ver(c, b.ExpectedVersion)
			action := map[string]string{"deferDefect": "DEFER", "startDefectRepair": "START_REPAIR", "resolveDefect": "RESOLVE"}[r.OperationID]
			return h.s.DefectTransition(c, t, id, action, b)
		case "listTyres":
			return h.s.ListTyres(c, t, c.Query("assetId"), c.Query("status"))
		case "getTyre":
			return h.s.GetTyre(c, t, id)
		case "registerTyre":
			var b models.RegisterTyreInput
			if !bind(c, &b) {
				return nil, errBound
			}
			return h.s.RegisterTyre(c, t, b)
		case "installTyre", "removeTyre", "rotateTyre", "recordTyreTread":
			var b models.TyreAction
			if !bind(c, &b) {
				return nil, errBound
			}
			b.ExpectedVersion = ver(c, b.ExpectedVersion)
			action := map[string]string{"installTyre": "INSTALL", "removeTyre": "REMOVE", "rotateTyre": "ROTATE", "recordTyreTread": "TREAD"}[r.OperationID]
			return h.s.TyreTransition(c, t, id, action, b)
		}
		return nil, apperrors.NotFound("Operation not found.", nil)
	})
}
