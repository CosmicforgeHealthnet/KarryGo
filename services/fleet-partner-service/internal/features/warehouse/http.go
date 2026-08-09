package warehouse

import (
	"bytes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func TenantRoutes() []Route {
	return []Route{
		{"GET", "/warehouses", "listWarehouses", "warehouses.read", "WAREHOUSES_ENABLED", "corporate", false, false}, {"POST", "/warehouses", "createWarehouse", "warehouses.manage", "WAREHOUSES_ENABLED", "corporate", true, false}, {"GET", "/warehouses/:warehouseId", "getWarehouse", "warehouses.read", "WAREHOUSES_ENABLED", "corporate", false, false}, {"PATCH", "/warehouses/:warehouseId", "updateWarehouse", "warehouses.manage", "WAREHOUSES_ENABLED", "corporate", false, true}, {"POST", "/warehouses/:warehouseId/archive", "archiveWarehouse", "warehouses.manage", "WAREHOUSES_ENABLED", "corporate", true, true},
		{"GET", "/warehouses/:warehouseId/locations", "listStorageLocations", "warehouse_locations.read", "WAREHOUSES_ENABLED", "corporate", false, false}, {"POST", "/warehouses/:warehouseId/locations", "createStorageLocation", "warehouse_locations.manage", "WAREHOUSES_ENABLED", "corporate", true, false}, {"GET", "/warehouses/:warehouseId/locations/:locationId", "getStorageLocation", "warehouse_locations.read", "WAREHOUSES_ENABLED", "corporate", false, false}, {"PATCH", "/warehouses/:warehouseId/locations/:locationId", "updateStorageLocation", "warehouse_locations.manage", "WAREHOUSES_ENABLED", "corporate", false, true},
		{"GET", "/inventory", "listWarehouseInventory", "warehouse_inventory.read", "WAREHOUSE_INVENTORY_ENABLED", "corporate", false, false}, {"POST", "/inventory/receipts", "receiveWarehouseInventory", "warehouse_inventory.receive", "WAREHOUSE_INVENTORY_ENABLED", "corporate", true, false}, {"POST", "/inventory/movements", "moveWarehouseInventory", "warehouse_inventory.move", "WAREHOUSE_INVENTORY_ENABLED", "corporate", true, true}, {"GET", "/inventory/:inventoryItemId", "getWarehouseInventory", "warehouse_inventory.read", "WAREHOUSE_INVENTORY_ENABLED", "corporate", false, false}, {"GET", "/inventory/:inventoryItemId/movements", "listWarehouseMovements", "warehouse_inventory.read", "WAREHOUSE_INVENTORY_ENABLED", "corporate", false, false},
		{"GET", "/fulfilment-orders", "listFulfilmentOrders", "fulfilment.read", "FULFILMENT_ENABLED", "corporate", false, false}, {"POST", "/fulfilment-orders", "createFulfilmentOrder", "fulfilment.manage", "FULFILMENT_ENABLED", "corporate", true, false}, {"GET", "/fulfilment-orders/:fulfilmentOrderId", "getFulfilmentOrder", "fulfilment.read", "FULFILMENT_ENABLED", "corporate", false, false}, {"PATCH", "/fulfilment-orders/:fulfilmentOrderId", "updateFulfilmentOrder", "fulfilment.manage", "FULFILMENT_ENABLED", "corporate", false, true}, {"POST", "/fulfilment-orders/:fulfilmentOrderId/release", "releaseFulfilmentOrder", "fulfilment.release", "FULFILMENT_ENABLED", "corporate", true, true}, {"POST", "/fulfilment-orders/:fulfilmentOrderId/pick", "pickFulfilmentOrder", "fulfilment.pick", "FULFILMENT_ENABLED", "corporate", true, true}, {"POST", "/fulfilment-orders/:fulfilmentOrderId/pack", "packFulfilmentOrder", "fulfilment.pack", "FULFILMENT_ENABLED", "corporate", true, true}, {"POST", "/fulfilment-orders/:fulfilmentOrderId/dispatch", "dispatchFulfilmentOrder", "fulfilment.dispatch", "FULFILMENT_ENABLED", "corporate", true, true}, {"POST", "/fulfilment-orders/:fulfilmentOrderId/cancel", "cancelFulfilmentOrder", "fulfilment.manage", "FULFILMENT_ENABLED", "corporate", true, true},
		{"GET", "/dock-appointments", "listDockAppointments", "dock.read", "DOCK_OPERATIONS_ENABLED", "corporate", false, false}, {"POST", "/dock-appointments", "createDockAppointment", "dock.manage", "DOCK_OPERATIONS_ENABLED", "corporate", true, false}, {"GET", "/dock-appointments/:appointmentId", "getDockAppointment", "dock.read", "DOCK_OPERATIONS_ENABLED", "corporate", false, false}, {"PATCH", "/dock-appointments/:appointmentId", "updateDockAppointment", "dock.manage", "DOCK_OPERATIONS_ENABLED", "corporate", false, true}, {"POST", "/dock-appointments/:appointmentId/check-in", "checkInDockAppointment", "dock.manage", "DOCK_OPERATIONS_ENABLED", "corporate", true, true}, {"POST", "/dock-appointments/:appointmentId/complete", "completeDockAppointment", "dock.manage", "DOCK_OPERATIONS_ENABLED", "corporate", true, true},
		{"GET", "/yard-visits", "listYardVisits", "yard.read", "YARD_OPERATIONS_ENABLED", "corporate", false, false}, {"POST", "/yard-visits", "createYardVisit", "yard.manage", "YARD_OPERATIONS_ENABLED", "corporate", true, false}, {"GET", "/yard-visits/:yardVisitId", "getYardVisit", "yard.read", "YARD_OPERATIONS_ENABLED", "corporate", false, false}, {"POST", "/yard-visits/:yardVisitId/check-in", "checkInYardVisit", "yard.manage", "YARD_OPERATIONS_ENABLED", "corporate", true, true}, {"POST", "/yard-visits/:yardVisitId/release", "releaseYardVisit", "yard.manage", "YARD_OPERATIONS_ENABLED", "corporate", true, true}, {"POST", "/yard-visits/:yardVisitId/check-out", "checkOutYardVisit", "yard.manage", "YARD_OPERATIONS_ENABLED", "corporate", true, true},
		{"GET", "/terminal-transfers", "listTerminalTransfers", "terminal.read", "TERMINAL_TRANSFERS_ENABLED", "corporate", false, false}, {"POST", "/terminal-transfers", "createTerminalTransfer", "terminal.manage", "TERMINAL_TRANSFERS_ENABLED", "corporate", true, false}, {"GET", "/terminal-transfers/:transferId", "getTerminalTransfer", "terminal.read", "TERMINAL_TRANSFERS_ENABLED", "corporate", false, false}, {"POST", "/terminal-transfers/:transferId/depart", "departTerminalTransfer", "terminal.manage", "TERMINAL_TRANSFERS_ENABLED", "corporate", true, true}, {"POST", "/terminal-transfers/:transferId/arrive", "arriveTerminalTransfer", "terminal.manage", "TERMINAL_TRANSFERS_ENABLED", "corporate", true, true}, {"POST", "/terminal-transfers/:transferId/reconcile", "reconcileTerminalTransfer", "terminal.reconcile", "TERMINAL_TRANSFERS_ENABLED", "corporate", true, true},
		{"GET", "/returns", "listReturns", "returns.read", "RETURNS_ENABLED", "corporate", false, false}, {"POST", "/returns", "createReturn", "returns.manage", "RETURNS_ENABLED", "corporate", true, false}, {"GET", "/returns/:returnId", "getReturn", "returns.read", "RETURNS_ENABLED", "corporate", false, false}, {"POST", "/returns/:returnId/receive", "receiveReturn", "returns.manage", "RETURNS_ENABLED", "corporate", true, true}, {"POST", "/returns/:returnId/decide", "decideReturn", "returns.decide", "RETURNS_ENABLED", "corporate", true, true}, {"POST", "/returns/:returnId/complete", "completeReturn", "returns.manage", "RETURNS_ENABLED", "corporate", true, true}}
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
func whVer(c *gin.Context, v int64) int64 {
	if v > 0 {
		return v
	}
	n, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return n
}
func bindWH(c *gin.Context, v any) bool {
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
	if strings.HasPrefix(r.OperationID, "create") || strings.HasPrefix(r.OperationID, "receiveWarehouse") {
		status = http.StatusCreated
	}
	var payload []byte
	if e != nil {
		ae := apperrors.From(e)
		status = ae.Status
		payload, _ = json.Marshal(gin.H{"error": respond.ErrorBody{Code: ae.Code, Message: ae.Message, Details: []apperrors.FieldViolation{}, RequestID: httpx.GetRequestID(c)}})
	} else {
		payload, _ = json.Marshal(gin.H{"data": v})
	}
	if e = h.idem.Complete(c, t.OrganizationID, ep, key, status, payload); e != nil {
		respond.Error(c, e)
		return
	}
	c.Data(status, "application/json", payload)
}

type cmdBody struct {
	ExpectedVersion   int64            `json:"expectedVersion"`
	Reason            string           `json:"reason"`
	Disposition       string           `json:"disposition"`
	NewSeal           string           `json:"newSeal"`
	Snapshot          json.RawMessage  `json:"snapshot"`
	ShipmentReference string           `json:"shipmentReference"`
	ManifestReference string           `json:"manifestReference"`
	Lines             []FulfilmentLine `json:"lines"`
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	run := func() (any, error) {
		switch r.OperationID {
		case "listWarehouses":
			return h.s.ListWarehouses(c, t)
		case "createWarehouse":
			var b Warehouse
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateWarehouse(c, t, b)
		case "getWarehouse":
			return h.s.GetWarehouse(c, t, c.Param("warehouseId"))
		case "updateWarehouse":
			var b Warehouse
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateWarehouse(c, t, c.Param("warehouseId"), whVer(c, b.Version), b, false)
		case "archiveWarehouse":
			var b cmdBody
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateWarehouse(c, t, c.Param("warehouseId"), whVer(c, b.ExpectedVersion), Warehouse{}, true)
		case "listStorageLocations":
			return h.s.ListLocations(c, t, c.Param("warehouseId"))
		case "createStorageLocation":
			var b Location
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateLocation(c, t, c.Param("warehouseId"), b)
		case "getStorageLocation":
			return h.s.GetLocation(c, t, c.Param("warehouseId"), c.Param("locationId"))
		case "updateStorageLocation":
			var b Location
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateLocation(c, t, c.Param("warehouseId"), c.Param("locationId"), whVer(c, b.Version), b)
		case "listWarehouseInventory":
			return h.s.ListItems(c, t)
		case "receiveWarehouseInventory":
			var b ReceiptInput
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			b.CorrelationID = c.GetHeader("Idempotency-Key")
			return h.s.Receipt(c, t, b)
		case "moveWarehouseInventory":
			var b MovementInput
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			b.CorrelationID = c.GetHeader("Idempotency-Key")
			b.ExpectedVersion = whVer(c, b.ExpectedVersion)
			return h.s.Move(c, t, b.InventoryItemID, b)
		case "getWarehouseInventory":
			return h.s.GetItem(c, t, c.Param("inventoryItemId"))
		case "listWarehouseMovements":
			return h.s.Movements(c, t, c.Param("inventoryItemId"))
		case "listFulfilmentOrders":
			return h.s.ListFulfilment(c, t)
		case "createFulfilmentOrder":
			var b FulfilmentOrder
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateFulfilment(c, t, b)
		case "getFulfilmentOrder":
			return h.s.GetFulfilment(c, t, c.Param("fulfilmentOrderId"))
		case "updateFulfilmentOrder":
			var b FulfilmentOrder
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateFulfilment(c, t, c.Param("fulfilmentOrderId"), whVer(c, b.Version), b)
		case "releaseFulfilmentOrder", "pickFulfilmentOrder", "packFulfilmentOrder", "dispatchFulfilmentOrder", "cancelFulfilmentOrder":
			var b cmdBody
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			cmd := map[string]string{"releaseFulfilmentOrder": "RELEASE", "pickFulfilmentOrder": "PICK", "packFulfilmentOrder": "PACK", "dispatchFulfilmentOrder": "DISPATCH", "cancelFulfilmentOrder": "CANCEL"}[r.OperationID]
			return h.s.FulfilmentCommand(c, t, c.Param("fulfilmentOrderId"), cmd, whVer(c, b.ExpectedVersion), FulfilmentOrder{Lines: b.Lines, ShipmentReference: b.ShipmentReference, ManifestReference: b.ManifestReference})
		case "listDockAppointments":
			return h.s.ListDocks(c, t)
		case "createDockAppointment":
			var b DockAppointment
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateDock(c, t, b)
		case "getDockAppointment":
			return h.s.GetDock(c, t, c.Param("appointmentId"))
		case "updateDockAppointment":
			var b DockAppointment
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.DockCommand(c, t, c.Param("appointmentId"), "UPDATE", whVer(c, b.Version), b)
		case "checkInDockAppointment", "completeDockAppointment":
			var b cmdBody
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			cmd := "CHECK_IN"
			if r.OperationID == "completeDockAppointment" {
				cmd = "COMPLETE"
			}
			return h.s.DockCommand(c, t, c.Param("appointmentId"), cmd, whVer(c, b.ExpectedVersion), DockAppointment{})
		case "listYardVisits":
			return h.s.ListYards(c, t)
		case "createYardVisit":
			var b YardVisit
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateYard(c, t, b)
		case "getYardVisit":
			return h.s.GetYard(c, t, c.Param("yardVisitId"))
		case "checkInYardVisit", "releaseYardVisit", "checkOutYardVisit":
			var b cmdBody
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			cmd := map[string]string{"checkInYardVisit": "CHECK_IN", "releaseYardVisit": "RELEASE", "checkOutYardVisit": "CHECK_OUT"}[r.OperationID]
			return h.s.YardCommand(c, t, c.Param("yardVisitId"), cmd, b.Reason, b.NewSeal, whVer(c, b.ExpectedVersion))
		case "listTerminalTransfers":
			return h.s.ListTransfers(c, t)
		case "createTerminalTransfer":
			var b TerminalTransfer
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateTransfer(c, t, b)
		case "getTerminalTransfer":
			return h.s.GetTransfer(c, t, c.Param("transferId"))
		case "departTerminalTransfer", "arriveTerminalTransfer", "reconcileTerminalTransfer":
			var b cmdBody
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			cmd := map[string]string{"departTerminalTransfer": "DEPART", "arriveTerminalTransfer": "ARRIVE", "reconcileTerminalTransfer": "RECONCILE"}[r.OperationID]
			return h.s.TransferCommand(c, t, c.Param("transferId"), cmd, whVer(c, b.ExpectedVersion), b.Snapshot)
		case "listReturns":
			return h.s.ListReturns(c, t)
		case "createReturn":
			var b Return
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateReturn(c, t, b)
		case "getReturn":
			return h.s.GetReturn(c, t, c.Param("returnId"))
		case "receiveReturn", "decideReturn", "completeReturn":
			var b cmdBody
			if !bindWH(c, &b) {
				return nil, errors.New("bound")
			}
			cmd := map[string]string{"receiveReturn": "RECEIVE", "decideReturn": "DECIDE", "completeReturn": "COMPLETE"}[r.OperationID]
			return h.s.ReturnCommand(c, t, c.Param("returnId"), cmd, b.Disposition, b.Reason, whVer(c, b.ExpectedVersion))
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
