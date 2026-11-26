package http

import (
	"context"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// OperationsUseCases is the Section 51 inbound/outbound slice of the
// warehouse use cases.
type OperationsUseCases interface {
	CreateWarehouseReceipt(ctx context.Context, t tenant.Context, in models.ReceiptCreateInput) (models.WarehouseReceipt, error)
	GetWarehouseReceipt(ctx context.Context, t tenant.Context, id string) (models.WarehouseReceipt, error)
	ListWarehouseReceipts(ctx context.Context, t tenant.Context) ([]models.WarehouseReceipt, error)
	ReceiptCommand(ctx context.Context, t tenant.Context, id, command string, in models.ReceiptCommandInput) (models.WarehouseReceipt, error)
	ListPutawayTasks(ctx context.Context, t tenant.Context) ([]models.PutawayTask, error)
	GetPutawayTask(ctx context.Context, t tenant.Context, id string) (models.PutawayTask, error)
	ClaimPutawayTask(ctx context.Context, t tenant.Context, id string, in models.TaskClaimInput) (models.PutawayTask, error)
	CompletePutawayTask(ctx context.Context, t tenant.Context, id string, in models.PutawayCompleteInput) (models.PutawayTask, error)
	CreatePickWave(ctx context.Context, t tenant.Context, in models.PickWaveInput) (models.PickWave, error)
	GetPickWave(ctx context.Context, t tenant.Context, id string) (models.PickWave, error)
	ListPickWaves(ctx context.Context, t tenant.Context) ([]models.PickWave, error)
	ListPickTasks(ctx context.Context, t tenant.Context) ([]models.PickTask, error)
	GetPickTask(ctx context.Context, t tenant.Context, id string) (models.PickTask, error)
	ClaimPickTask(ctx context.Context, t tenant.Context, id string, in models.TaskClaimInput) (models.PickTask, error)
	CompletePickTask(ctx context.Context, t tenant.Context, id string, in models.PickCompleteInput) (models.PickTask, error)
	CreatePackingUnit(ctx context.Context, t tenant.Context, in models.PackingUnit) (models.PackingUnit, error)
	GetPackingUnit(ctx context.Context, t tenant.Context, id string) (models.PackingUnit, error)
	ListPackingUnits(ctx context.Context, t tenant.Context) ([]models.PackingUnit, error)
	RecordGateEvent(ctx context.Context, t tenant.Context, visitID string, in models.GateEventInput) (models.GateEvent, error)
	ListGateEvents(ctx context.Context, t tenant.Context, visitID string) ([]models.GateEvent, error)
}

// operationsRoutes are the Section 51 receipt, task, wave, packing and gate
// routes. Commands are idempotent through the Idempotency-Key header;
// receipt commands also require the receipt's expectedVersion.
func operationsRoutes() []Route {
	const inv, ful, yard = "WAREHOUSE_INVENTORY_ENABLED", "FULFILMENT_ENABLED", "YARD_OPERATIONS_ENABLED"
	return []Route{
		{"GET", "/warehouse-receipts", "listWarehouseReceipts", "warehouse_receipts.read", inv, "corporate", false, false},
		{"POST", "/warehouse-receipts", "createWarehouseReceipt", "warehouse_receipts.manage", inv, "corporate", true, false},
		{"GET", "/warehouse-receipts/:receiptId", "getWarehouseReceipt", "warehouse_receipts.read", inv, "corporate", false, false},
		{"POST", "/warehouse-receipts/:receiptId/arrive", "arriveWarehouseReceipt", "warehouse_receipts.manage", inv, "corporate", true, true},
		{"POST", "/warehouse-receipts/:receiptId/receive", "receiveWarehouseReceipt", "warehouse_receipts.manage", inv, "corporate", true, true},
		{"POST", "/warehouse-receipts/:receiptId/resolve-discrepancy", "resolveWarehouseReceiptDiscrepancy", "warehouse_receipts.manage", inv, "corporate", true, true},
		{"POST", "/warehouse-receipts/:receiptId/complete", "completeWarehouseReceipt", "warehouse_receipts.manage", inv, "corporate", true, true},
		{"POST", "/warehouse-receipts/:receiptId/cancel", "cancelWarehouseReceipt", "warehouse_receipts.manage", inv, "corporate", true, true},
		{"GET", "/putaway-tasks", "listPutawayTasks", "warehouse_tasks.read", inv, "corporate", false, false},
		{"GET", "/putaway-tasks/:taskId", "getPutawayTask", "warehouse_tasks.read", inv, "corporate", false, false},
		{"POST", "/putaway-tasks/:taskId/claim", "claimPutawayTask", "warehouse_tasks.execute", inv, "corporate", true, false},
		{"POST", "/putaway-tasks/:taskId/complete", "completePutawayTask", "warehouse_tasks.execute", inv, "corporate", true, false},
		{"GET", "/pick-waves", "listPickWaves", "warehouse_tasks.read", ful, "corporate", false, false},
		{"POST", "/pick-waves", "createPickWave", "pick_waves.manage", ful, "corporate", true, false},
		{"GET", "/pick-waves/:waveId", "getPickWave", "warehouse_tasks.read", ful, "corporate", false, false},
		{"GET", "/pick-tasks", "listPickTasks", "warehouse_tasks.read", ful, "corporate", false, false},
		{"GET", "/pick-tasks/:taskId", "getPickTask", "warehouse_tasks.read", ful, "corporate", false, false},
		{"POST", "/pick-tasks/:taskId/claim", "claimPickTask", "warehouse_tasks.execute", ful, "corporate", true, false},
		{"POST", "/pick-tasks/:taskId/complete", "completePickTask", "warehouse_tasks.execute", ful, "corporate", true, false},
		{"GET", "/packing-units", "listPackingUnits", "warehouse_tasks.read", ful, "corporate", false, false},
		{"POST", "/packing-units", "createPackingUnit", "packing.manage", ful, "corporate", true, false},
		{"GET", "/packing-units/:packingUnitId", "getPackingUnit", "warehouse_tasks.read", ful, "corporate", false, false},
		{"GET", "/yard-visits/:yardVisitId/gate-events", "listGateEvents", "yard.read", yard, "corporate", false, false},
		{"POST", "/yard-visits/:yardVisitId/gate-events", "createGateEvent", "yard.gate", yard, "corporate", true, false},
	}
}

var receiptCommands = map[string]string{
	"arriveWarehouseReceipt": "ARRIVE", "receiveWarehouseReceipt": "RECEIVE", "resolveWarehouseReceiptDiscrepancy": "RESOLVE",
	"completeWarehouseReceipt": "COMPLETE", "cancelWarehouseReceipt": "CANCEL",
}

// operations runs a Section 51 operation. handled is false for every other
// operation ID so the main switch keeps serving them.
func (h *Handler) operations(c *gin.Context, t tenant.Context, r Route) (out any, err error, handled bool) {
	s, ok := h.s.(OperationsUseCases)
	if !ok {
		for _, x := range operationsRoutes() {
			if x.OperationID == r.OperationID {
				return nil, apperrors.Unavailable("Warehouse operations are unavailable.", nil), true
			}
		}
		return nil, nil, false
	}
	bound := func(v any) bool { return bindWH(c, v) }
	errBound := errBoundSentinel
	switch r.OperationID {
	case "listWarehouseReceipts":
		out, err = s.ListWarehouseReceipts(c, t)
	case "createWarehouseReceipt":
		var b models.ReceiptCreateInput
		if !bound(&b) {
			return nil, errBound, true
		}
		out, err = s.CreateWarehouseReceipt(c, t, b)
	case "getWarehouseReceipt":
		out, err = s.GetWarehouseReceipt(c, t, c.Param("receiptId"))
	case "arriveWarehouseReceipt", "receiveWarehouseReceipt", "resolveWarehouseReceiptDiscrepancy", "completeWarehouseReceipt", "cancelWarehouseReceipt":
		var b models.ReceiptCommandInput
		if !bound(&b) {
			return nil, errBound, true
		}
		b.ExpectedVersion = whVer(c, b.ExpectedVersion)
		out, err = s.ReceiptCommand(c, t, c.Param("receiptId"), receiptCommands[r.OperationID], b)
	case "listPutawayTasks":
		out, err = s.ListPutawayTasks(c, t)
	case "getPutawayTask":
		out, err = s.GetPutawayTask(c, t, c.Param("taskId"))
	case "claimPutawayTask":
		var b models.TaskClaimInput
		if !bound(&b) {
			return nil, errBound, true
		}
		out, err = s.ClaimPutawayTask(c, t, c.Param("taskId"), b)
	case "completePutawayTask":
		var b models.PutawayCompleteInput
		if !bound(&b) {
			return nil, errBound, true
		}
		out, err = s.CompletePutawayTask(c, t, c.Param("taskId"), b)
	case "listPickWaves":
		out, err = s.ListPickWaves(c, t)
	case "createPickWave":
		var b models.PickWaveInput
		if !bound(&b) {
			return nil, errBound, true
		}
		out, err = s.CreatePickWave(c, t, b)
	case "getPickWave":
		out, err = s.GetPickWave(c, t, c.Param("waveId"))
	case "listPickTasks":
		out, err = s.ListPickTasks(c, t)
	case "getPickTask":
		out, err = s.GetPickTask(c, t, c.Param("taskId"))
	case "claimPickTask":
		var b models.TaskClaimInput
		if !bound(&b) {
			return nil, errBound, true
		}
		out, err = s.ClaimPickTask(c, t, c.Param("taskId"), b)
	case "completePickTask":
		var b models.PickCompleteInput
		if !bound(&b) {
			return nil, errBound, true
		}
		out, err = s.CompletePickTask(c, t, c.Param("taskId"), b)
	case "listPackingUnits":
		out, err = s.ListPackingUnits(c, t)
	case "createPackingUnit":
		var b models.PackingUnit
		if !bound(&b) {
			return nil, errBound, true
		}
		out, err = s.CreatePackingUnit(c, t, b)
	case "getPackingUnit":
		out, err = s.GetPackingUnit(c, t, c.Param("packingUnitId"))
	case "listGateEvents":
		out, err = s.ListGateEvents(c, t, c.Param("yardVisitId"))
	case "createGateEvent":
		var b models.GateEventInput
		if !bound(&b) {
			return nil, errBound, true
		}
		out, err = s.RecordGateEvent(c, t, c.Param("yardVisitId"), b)
	default:
		return nil, nil, false
	}
	return out, err, true
}
