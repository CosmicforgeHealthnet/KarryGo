package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// OperationsRepository is the persistence of the Section 51 inbound and
// outbound workflows. repositories.Repository satisfies it.
type OperationsRepository interface {
	LocationRef(ctx context.Context, q database.Querier, org, id string) (models.LocationRef, error)
	YardVisitWarehouse(ctx context.Context, q database.Querier, org, id string) (string, string, error)
	ReceiptByNumber(ctx context.Context, org, number string) (models.WarehouseReceipt, error)
	InsertReceipt(ctx context.Context, q database.Querier, v models.WarehouseReceipt, actor string) (models.WarehouseReceipt, error)
	InsertReceiptLine(ctx context.Context, q database.Querier, org string, l models.ReceiptLine) (models.ReceiptLine, error)
	GetReceipt(ctx context.Context, org, id string) (models.WarehouseReceipt, error)
	LockReceipt(ctx context.Context, q database.Querier, org, id string) (models.WarehouseReceipt, error)
	ListReceipts(ctx context.Context, org string, branches []string) ([]models.WarehouseReceipt, error)
	ReceiptLines(ctx context.Context, q database.Querier, org, receiptID string, lock bool) ([]models.ReceiptLine, error)
	RecordLineReceipt(ctx context.Context, q database.Querier, org, lineID string, in models.ReceiveLineInput, accepted float64, discrepancy, itemID, quarantineItemID, actor string, at time.Time) error
	ResolveLineDiscrepancy(ctx context.Context, q database.Querier, org, lineID, resolution, note string) (int64, error)
	TransitionReceipt(ctx context.Context, q database.Querier, org, id string, expected int64, next, yardVisitID string) (models.WarehouseReceipt, error)

	InsertPutawayTask(ctx context.Context, q database.Querier, v models.PutawayTask) error
	GetPutawayTask(ctx context.Context, org, id string) (models.PutawayTask, error)
	LockPutawayTask(ctx context.Context, q database.Querier, org, id string) (models.PutawayTask, error)
	ListPutawayTasks(ctx context.Context, org string, branches []string) ([]models.PutawayTask, error)
	ClaimPutawayTask(ctx context.Context, q database.Querier, org, id, owner, claimID string, until time.Time) error
	CompletePutawayTask(ctx context.Context, q database.Querier, org, id, to, destItem, actor string) error
	OpenPutawayCount(ctx context.Context, q database.Querier, org, receiptID string) (int, error)

	LockStockLine(ctx context.Context, q database.Querier, src models.Item, location string) (models.Item, error)
	InsertItemWithStatus(ctx context.Context, q database.Querier, item models.Item, quantity float64, status string) (models.Item, error)
	AddItemQuantityKeepStatus(ctx context.Context, q database.Querier, org, id string, quantity float64) (models.Item, error)
	SubtractItemQuantity(ctx context.Context, q database.Querier, org, id string, quantity float64) (models.Item, error)

	InsertWave(ctx context.Context, q database.Querier, v models.PickWave, actor string) (models.PickWave, error)
	WaveByNumber(ctx context.Context, org, number string) (models.PickWave, error)
	GetWave(ctx context.Context, org, id string) (models.PickWave, error)
	ListWaves(ctx context.Context, org string, branches []string) ([]models.PickWave, error)
	FulfilmentLinesForWave(ctx context.Context, q database.Querier, org, orderID string) ([]models.PickTask, error)
	InsertPickTask(ctx context.Context, q database.Querier, v models.PickTask) error
	WaveTasks(ctx context.Context, org, waveID string) ([]models.PickTask, error)
	GetPickTask(ctx context.Context, org, id string) (models.PickTask, error)
	LockPickTask(ctx context.Context, q database.Querier, org, id string) (models.PickTask, error)
	ListPickTasks(ctx context.Context, org string, branches []string) ([]models.PickTask, error)
	ClaimPickTask(ctx context.Context, q database.Querier, org, id, owner, claimID string, until time.Time) error
	ClaimIdentity(ctx context.Context, q database.Querier, table, org, id string) (string, string, error)
	CompletePickTask(ctx context.Context, q database.Querier, org, id, status, actor string, picked float64) error
	ActiveReservation(ctx context.Context, q database.Querier, org, itemID, orderID string) (float64, error)
	SetLinePickedQuantity(ctx context.Context, q database.Querier, lineID string, picked float64) error
	InsertPickTaskMovement(ctx context.Context, q database.Querier, org, taskID, itemID, orderID, actor string, quantity float64) error
	OrderPickProgress(ctx context.Context, q database.Querier, org, orderID string) (int, int, error)
	ReleaseOrderReservations(ctx context.Context, q database.Querier, org, itemID, orderID string) error
	InsertPickReleaseMovement(ctx context.Context, q database.Querier, org, itemID, orderID, actor string, quantity float64) error
	WaveOpenCount(ctx context.Context, q database.Querier, org, waveID string) (int, error)
	CompleteWave(ctx context.Context, q database.Querier, org, waveID string) (int64, error)
	SetOrderStatusLocked(ctx context.Context, q database.Querier, org, orderID, status string) (int64, error)
	LockOrderForWave(ctx context.Context, q database.Querier, org, orderID string) (string, string, error)

	InsertPackingUnit(ctx context.Context, q database.Querier, v models.PackingUnit, actor string) (models.PackingUnit, error)
	PackingByCode(ctx context.Context, org, code string) (models.PackingUnit, error)
	GetPackingUnit(ctx context.Context, org, id string) (models.PackingUnit, error)
	ListPackingUnits(ctx context.Context, org string, branches []string) ([]models.PackingUnit, error)
	PackedQuantities(ctx context.Context, q database.Querier, org, orderID string) (map[string]float64, error)
	PickedQuantities(ctx context.Context, q database.Querier, org, orderID string) (map[string]float64, error)

	LockYardVisitRow(ctx context.Context, q database.Querier, org, id string) (string, error)
	GateEvents(ctx context.Context, q database.Querier, org, visitID string) ([]models.GateEvent, error)
	InsertGateEvent(ctx context.Context, q database.Querier, v models.GateEvent) (models.GateEvent, error)
	ReleaseExpiredTaskLeases(ctx context.Context, q database.Querier, org string) error
}

// DefaultTaskLease is how long a claimed put-away or pick task stays with one
// worker before the sweep returns it to OPEN.
const DefaultTaskLease = 5 * time.Minute

var (
	errVersion = apperrors.New(http.StatusConflict, "version_conflict", "The record changed; refresh and retry.", nil)
	errState   = func(m string) error { return apperrors.New(http.StatusConflict, "state_transition_invalid", m, nil) }
	errScan    = func(m string) error { return apperrors.New(http.StatusUnprocessableEntity, "validation_error", m, nil) }
	errStock   = func(m string) error { return apperrors.New(http.StatusConflict, "inventory_insufficient", m, nil) }
)

func (s *Service) ops() OperationsRepository {
	if r, ok := s.repo.(OperationsRepository); ok {
		return r
	}
	return nil
}

func (s *Service) taskLease() time.Duration {
	if s.o.TaskLease > 0 {
		return s.o.TaskLease
	}
	return DefaultTaskLease
}

func notFound(e error, what string) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return apperrors.NotFound(what+" not found.", nil)
	}
	return e
}

// activeLocation checks a location exists in the warehouse and is usable.
func (s *Service) activeLocation(ctx context.Context, q database.Querier, org, warehouseID, id string) (models.LocationRef, error) {
	loc, e := s.ops().LocationRef(ctx, q, org, id)
	if e != nil || loc.WarehouseID != warehouseID {
		return loc, apperrors.NotFound("Storage location not found in warehouse.", nil)
	}
	if loc.Status != "ACTIVE" {
		return loc, errState("Storage location is not active.")
	}
	return loc, nil
}

// ---------------------------------------------------------------------------
// Receipts
// ---------------------------------------------------------------------------

// CreateWarehouseReceipt records an expected inbound receipt with its lines.
// The receipt number is the natural idempotency key.
func (s *Service) CreateWarehouseReceipt(ctx context.Context, t tenant.Context, in models.ReceiptCreateInput) (models.WarehouseReceipt, error) {
	if e := s.gate(s.o.Flags.Inventory, true); e != nil {
		return models.WarehouseReceipt{}, e
	}
	in.ReceiptNumber = strings.TrimSpace(in.ReceiptNumber)
	in.SourceType = strings.ToUpper(strings.TrimSpace(in.SourceType))
	if in.WarehouseID == "" || in.ReceiptNumber == "" || in.ReceivingLocationID == "" || len(in.Lines) == 0 {
		return models.WarehouseReceipt{}, apperrors.Validation("Warehouse, receipt number, receiving location and at least one line are required.", nil)
	}
	switch in.SourceType {
	case "SUPPLIER", "CUSTOMER", "TRANSFER", "RETURN", "OTHER":
	default:
		return models.WarehouseReceipt{}, apperrors.Validation("sourceType must be SUPPLIER, CUSTOMER, TRANSFER, RETURN or OTHER.", nil)
	}
	if existing, e := s.ops().ReceiptByNumber(ctx, t.OrganizationID, in.ReceiptNumber); e == nil {
		if existing.WarehouseID != in.WarehouseID {
			return existing, apperrors.Conflict("Receipt number is already used.", nil)
		}
		return s.GetWarehouseReceipt(ctx, t, existing.ID)
	}
	w, e := s.GetWarehouse(ctx, t, in.WarehouseID)
	if e != nil {
		return models.WarehouseReceipt{}, e
	}
	if w.Status != "ACTIVE" {
		return models.WarehouseReceipt{}, errState("Warehouse is not active.")
	}
	if e = s.requireUnrestricted(ctx, t, "WAREHOUSE", in.WarehouseID); e != nil {
		return models.WarehouseReceipt{}, e
	}
	v := models.WarehouseReceipt{ID: ids.New("whr"), OrganizationID: t.OrganizationID, WarehouseID: in.WarehouseID, ReceiptNumber: in.ReceiptNumber, SourceType: in.SourceType, SourceReference: in.SourceReference, ReceivingLocationID: in.ReceivingLocationID, QuarantineLocationID: in.QuarantineLocationID, ExpectedAt: in.ExpectedAt}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if _, x := s.activeLocation(ctx, tx, t.OrganizationID, in.WarehouseID, in.ReceivingLocationID); x != nil {
			return x
		}
		if in.QuarantineLocationID != "" {
			loc, x := s.activeLocation(ctx, tx, t.OrganizationID, in.WarehouseID, in.QuarantineLocationID)
			if x != nil {
				return x
			}
			if loc.Type != "QUARANTINE" {
				return apperrors.Validation("quarantineLocationId must be a QUARANTINE location.", nil)
			}
		}
		var x error
		if v, x = s.ops().InsertReceipt(ctx, tx, v, t.ActorID); x != nil {
			return x
		}
		for i, l := range in.Lines {
			if strings.TrimSpace(l.SKU) == "" || l.OwnerType == "" || l.OwnerReference == "" || l.Unit == "" || l.ExpectedQuantity <= 0 {
				return apperrors.Validation("Every line needs sku, owner, unit and a positive expected quantity.", nil)
			}
			if l.PutawayLocationID != "" {
				if _, x = s.activeLocation(ctx, tx, t.OrganizationID, in.WarehouseID, l.PutawayLocationID); x != nil {
					return x
				}
			}
			line, x := s.ops().InsertReceiptLine(ctx, tx, t.OrganizationID, models.ReceiptLine{ID: ids.New("whrl"), ReceiptID: v.ID, LineNumber: i + 1, SKU: strings.TrimSpace(l.SKU), OwnerType: l.OwnerType, OwnerReference: l.OwnerReference, LotReference: l.LotReference, SerialReference: l.SerialReference, Unit: l.Unit, ExpectedQuantity: l.ExpectedQuantity, PutawayLocationID: l.PutawayLocationID})
			if x != nil {
				return x
			}
			v.Lines = append(v.Lines, line)
		}
		return s.record(ctx, tx, t, "warehouse_receipt.expected.v1", "warehouse_receipt", v.ID, v.Version, map[string]any{"receiptId": v.ID, "warehouseId": v.WarehouseID, "status": v.Status, "lineCount": len(v.Lines)})
	})
	return v, e
}

// GetWarehouseReceipt reads a receipt and its lines in the caller's scope.
func (s *Service) GetWarehouseReceipt(ctx context.Context, t tenant.Context, id string) (models.WarehouseReceipt, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return models.WarehouseReceipt{}, e
	}
	v, e := s.ops().GetReceipt(ctx, t.OrganizationID, id)
	if e != nil {
		return v, notFound(e, "Warehouse receipt")
	}
	if _, e = s.GetWarehouse(ctx, t, v.WarehouseID); e != nil {
		return models.WarehouseReceipt{}, apperrors.NotFound("Warehouse receipt not found.", nil)
	}
	v.Lines, e = s.ops().ReceiptLines(ctx, nil, t.OrganizationID, id, false)
	return v, e
}

// ListWarehouseReceipts lists receipts in the caller's permitted branches.
func (s *Service) ListWarehouseReceipts(ctx context.Context, t tenant.Context) ([]models.WarehouseReceipt, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return nil, e
	}
	return s.ops().ListReceipts(ctx, t.OrganizationID, t.PermittedBranchIDs)
}

// ReceiptCommand runs ARRIVE, RECEIVE, RESOLVE, COMPLETE or CANCEL.
func (s *Service) ReceiptCommand(ctx context.Context, t tenant.Context, id, command string, in models.ReceiptCommandInput) (models.WarehouseReceipt, error) {
	current, e := s.GetWarehouseReceipt(ctx, t, id)
	if e != nil {
		return current, e
	}
	if s.o.Flags.KillSwitch {
		return current, apperrors.Unavailable("Fleet integrations are disabled.", nil)
	}
	if in.ExpectedVersion <= 0 {
		return current, apperrors.Validation("expectedVersion is required.", nil)
	}
	org := t.OrganizationID
	var out models.WarehouseReceipt
	e = s.tx(ctx, func(tx pgx.Tx) error {
		r, x := s.ops().LockReceipt(ctx, tx, org, id)
		if x != nil {
			return notFound(x, "Warehouse receipt")
		}
		if r.Version != in.ExpectedVersion {
			return errVersion
		}
		lines, x := s.ops().ReceiptLines(ctx, tx, org, id, true)
		if x != nil {
			return x
		}
		next := ""
		event := ""
		switch command {
		case "ARRIVE":
			if r.Status != "EXPECTED" {
				return errState("Only an expected receipt can arrive.")
			}
			if in.YardVisitID != "" {
				wh, _, x := s.ops().YardVisitWarehouse(ctx, tx, org, in.YardVisitID)
				if x != nil || wh != r.WarehouseID {
					return apperrors.NotFound("Yard visit not found at this warehouse.", nil)
				}
			}
			next, event = "ARRIVED", "warehouse_receipt.arrived.v1"
		case "RECEIVE":
			if r.Status != "ARRIVED" && r.Status != "RECEIVING" && r.Status != "EXCEPTION" {
				return errState("The receipt is not open for receiving.")
			}
			if len(in.Lines) == 0 {
				return apperrors.Validation("At least one counted line is required.", nil)
			}
			byID := map[string]*models.ReceiptLine{}
			for i := range lines {
				byID[lines[i].ID] = &lines[i]
			}
			for _, c := range in.Lines {
				l := byID[c.LineID]
				if l == nil {
					return apperrors.NotFound("Receipt line not found.", nil)
				}
				if x = s.receiveLine(ctx, tx, t, r, l, c); x != nil {
					return x
				}
			}
			next, event = receiptStatus(lines), "warehouse_receipt.receiving.v1"
			if next == "EXCEPTION" {
				event = "warehouse_receipt.exception.v1"
			} else if next == "RECEIVED" {
				event = "warehouse_receipt.received.v1"
			}
		case "RESOLVE":
			if r.Status != "EXCEPTION" && r.Status != "RECEIVING" {
				return errState("Only a receipt with open discrepancies can be resolved.")
			}
			switch in.Resolution {
			case "ACCEPT_VARIANCE", "SUPPLIER_CLAIM", "RETURN_TO_SENDER", "WRITE_OFF", "RECOUNTED":
			default:
				return apperrors.Validation("resolution must be ACCEPT_VARIANCE, SUPPLIER_CLAIM, RETURN_TO_SENDER, WRITE_OFF or RECOUNTED.", nil)
			}
			if strings.TrimSpace(in.Note) == "" {
				return apperrors.Validation("A resolution note is required.", nil)
			}
			n, x := s.ops().ResolveLineDiscrepancy(ctx, tx, org, in.LineID, in.Resolution, in.Note)
			if x != nil {
				return x
			}
			if n == 0 {
				return errState("The line has no open discrepancy.")
			}
			for i := range lines {
				if lines[i].ID == in.LineID {
					lines[i].DiscrepancyStatus = "RESOLVED"
				}
			}
			next, event = receiptStatus(lines), "warehouse_receipt.discrepancy_resolved.v1"
			if next == "RECEIVED" {
				event = "warehouse_receipt.received.v1"
			}
		case "COMPLETE":
			if r.Status != "RECEIVED" {
				return errState("All lines must be received and every discrepancy resolved first.")
			}
			created := 0
			for _, l := range lines {
				if l.AcceptedQuantity <= 0 || l.InventoryItemID == "" {
					continue
				}
				task := models.PutawayTask{ID: ids.New("pwt"), OrganizationID: org, WarehouseID: r.WarehouseID, ReceiptID: r.ID, ReceiptLineID: l.ID, InventoryItemID: l.InventoryItemID, FromLocationID: r.ReceivingLocationID, SuggestedLocationID: l.PutawayLocationID, Quantity: l.AcceptedQuantity}
				if x = s.ops().InsertPutawayTask(ctx, tx, task); x != nil {
					return x
				}
				created++
			}
			next, event = "PUTAWAY_IN_PROGRESS", "warehouse_receipt.putaway_started.v1"
			if created == 0 {
				next, event = "COMPLETED", "warehouse_receipt.completed.v1"
			}
		case "CANCEL":
			if r.Status != "DRAFT" && r.Status != "EXPECTED" {
				return errState("Only a receipt with no receiving activity can be cancelled.")
			}
			if strings.TrimSpace(in.Reason) == "" {
				return apperrors.Validation("A cancellation reason is required.", nil)
			}
			next, event = "CANCELLED", "warehouse_receipt.cancelled.v1"
		default:
			return apperrors.BadRequest("Unsupported receipt command.", nil)
		}
		if out, x = s.ops().TransitionReceipt(ctx, tx, org, id, in.ExpectedVersion, next, in.YardVisitID); x != nil {
			if errors.Is(x, pgx.ErrNoRows) {
				return errVersion
			}
			return x
		}
		return s.record(ctx, tx, t, event, "warehouse_receipt", id, out.Version, map[string]any{"receiptId": id, "warehouseId": out.WarehouseID, "status": out.Status})
	})
	if e != nil {
		return out, e
	}
	out.Lines, e = s.ops().ReceiptLines(ctx, nil, org, id, false)
	return out, e
}

// receiptStatus derives the receiving status from the line states.
func receiptStatus(lines []models.ReceiptLine) string {
	pending, open := false, false
	for _, l := range lines {
		if l.Status == "PENDING" {
			pending = true
		}
		if l.DiscrepancyStatus == "OPEN" {
			open = true
		}
	}
	switch {
	case open:
		return "EXCEPTION"
	case pending:
		return "RECEIVING"
	}
	return "RECEIVED"
}

// receiveLine counts one line: accepted stock goes to the receiving location
// as AVAILABLE; quarantined and damaged stock goes to the quarantine
// location as QUARANTINED when one is configured, and is never available.
func (s *Service) receiveLine(ctx context.Context, tx pgx.Tx, t tenant.Context, r models.WarehouseReceipt, l *models.ReceiptLine, c models.ReceiveLineInput) error {
	if l.Status != "PENDING" {
		return errState("Receipt line " + l.SKU + " is already received.")
	}
	if strings.TrimSpace(c.ScannedSKU) != l.SKU {
		return errScan("Scanned SKU does not match the receipt line.")
	}
	if c.ReceivedQuantity < 0 || c.DamagedQuantity < 0 || c.QuarantinedQuantity < 0 || c.DamagedQuantity+c.QuarantinedQuantity > c.ReceivedQuantity {
		return apperrors.Validation("Damaged and quarantined quantities must be non-negative parts of the received quantity.", nil)
	}
	accepted := c.ReceivedQuantity - c.DamagedQuantity - c.QuarantinedQuantity
	discrepancy := "NONE"
	if c.ReceivedQuantity != l.ExpectedQuantity || c.DamagedQuantity > 0 || c.QuarantinedQuantity > 0 {
		discrepancy = "OPEN"
	}
	key := models.Item{OrganizationID: t.OrganizationID, WarehouseID: r.WarehouseID, OwnerType: l.OwnerType, OwnerReference: l.OwnerReference, SKU: l.SKU, LotReference: l.LotReference, SerialReference: l.SerialReference, Unit: l.Unit, Condition: "GOOD"}
	itemID := ""
	if accepted > 0 {
		item, x := s.stockIn(ctx, tx, t, key, r.ReceivingLocationID, accepted, "AVAILABLE", "receipt:"+l.ID, "WAREHOUSE_RECEIPT")
		if x != nil {
			return x
		}
		itemID = item.ID
	}
	quarantineID := ""
	if held := c.DamagedQuantity + c.QuarantinedQuantity; held > 0 && r.QuarantineLocationID != "" {
		key.Condition = "DAMAGED"
		item, x := s.stockIn(ctx, tx, t, key, r.QuarantineLocationID, held, "QUARANTINED", "receipt-quarantine:"+l.ID, "RECEIPT_QUARANTINE")
		if x != nil {
			return x
		}
		quarantineID = item.ID
	}
	if x := s.ops().RecordLineReceipt(ctx, tx, t.OrganizationID, l.ID, c, accepted, discrepancy, itemID, quarantineID, t.ActorID, s.o.Now()); x != nil {
		return x
	}
	l.Status, l.DiscrepancyStatus, l.AcceptedQuantity, l.InventoryItemID = "RECEIVED", discrepancy, accepted, itemID
	return nil
}

// stockIn adds quantity to the stock line at a location, creating it when
// needed, and appends the RECEIPT movement.
func (s *Service) stockIn(ctx context.Context, tx pgx.Tx, t tenant.Context, key models.Item, location string, qty float64, status, correlation, reason string) (models.Item, error) {
	if x := s.capacity(ctx, tx, t.OrganizationID, location, qty); x != nil {
		return models.Item{}, x
	}
	item, x := s.ops().LockStockLine(ctx, tx, key, location)
	if errors.Is(x, pgx.ErrNoRows) {
		key.ID, key.LocationID = ids.New("inv"), location
		item, x = s.ops().InsertItemWithStatus(ctx, tx, key, qty, status)
	} else if x == nil {
		if item.Status == "QUARANTINED" && status != "QUARANTINED" || item.Status != "QUARANTINED" && status == "QUARANTINED" {
			return item, errState("Quarantined and available stock cannot share a stock line.")
		}
		item, x = s.ops().AddItemQuantityKeepStatus(ctx, tx, t.OrganizationID, item.ID, qty)
	}
	if x != nil {
		return item, x
	}
	m := models.Movement{ID: ids.New("imv"), OrganizationID: t.OrganizationID, InventoryItemID: item.ID, MovementType: "RECEIPT", ToLocationID: location, Quantity: qty, ReasonCode: reason, ActorReference: t.ActorID, CorrelationID: correlation, ProjectionBefore: item.QuantityProjection - qty, ProjectionAfter: item.QuantityProjection}
	if x = s.repo.InsertMovement(ctx, tx, &m); x != nil {
		return item, x
	}
	return item, s.record(ctx, tx, t, "inventory.received.v1", "inventory_item", item.ID, item.Version, map[string]any{"inventoryItemId": item.ID, "movementId": m.ID, "quantity": qty, "status": status})
}

// ---------------------------------------------------------------------------
// Leased tasks
// ---------------------------------------------------------------------------

// claim enforces the lease rules shared by put-away and pick tasks: an OPEN
// task or one whose lease lapsed can be claimed; a live lease held by
// another worker cannot; the same worker retrying with the same claim id
// gets its existing lease back.
func (s *Service) claim(ctx context.Context, tx pgx.Tx, kind, org, id, status string, leaseOwner string, leaseExpires *time.Time, actor, claimID string) (bool, error) {
	if strings.TrimSpace(claimID) == "" {
		return false, apperrors.Validation("clientClaimId is required.", nil)
	}
	now := s.o.Now()
	switch status {
	case "OPEN":
	case "CLAIMED":
		live := leaseExpires != nil && leaseExpires.After(now)
		if live && leaseOwner != actor {
			return false, apperrors.New(http.StatusConflict, "task_already_claimed", "The task is claimed by another worker.", nil)
		}
		if live {
			_, existing, x := s.ops().ClaimIdentity(ctx, tx, kind, org, id)
			if x != nil {
				return false, x
			}
			if existing == claimID {
				return true, nil
			}
		}
	default:
		return false, errState("The task is no longer claimable.")
	}
	return false, nil
}

func (s *Service) requireLease(status, owner string, expires *time.Time, actor string) error {
	if status != "CLAIMED" {
		return errState("The task must be claimed before it is completed.")
	}
	if owner != actor {
		return apperrors.New(http.StatusConflict, "task_already_claimed", "The task is claimed by another worker.", nil)
	}
	if expires == nil || !expires.After(s.o.Now()) {
		return apperrors.New(http.StatusConflict, "task_lease_expired", "The task lease expired; claim it again.", nil)
	}
	return nil
}

func (s *Service) taskWarehouseInScope(ctx context.Context, t tenant.Context, warehouseID string) error {
	if _, e := s.GetWarehouse(ctx, t, warehouseID); e != nil {
		return apperrors.NotFound("Task not found.", nil)
	}
	return nil
}

func (s *Service) ListPutawayTasks(ctx context.Context, t tenant.Context) ([]models.PutawayTask, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return nil, e
	}
	return s.ops().ListPutawayTasks(ctx, t.OrganizationID, t.PermittedBranchIDs)
}

func (s *Service) GetPutawayTask(ctx context.Context, t tenant.Context, id string) (models.PutawayTask, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return models.PutawayTask{}, e
	}
	v, e := s.ops().GetPutawayTask(ctx, t.OrganizationID, id)
	if e != nil {
		return v, notFound(e, "Put-away task")
	}
	return v, s.taskWarehouseInScope(ctx, t, v.WarehouseID)
}

// ClaimPutawayTask leases a put-away task to the calling worker.
func (s *Service) ClaimPutawayTask(ctx context.Context, t tenant.Context, id string, in models.TaskClaimInput) (models.PutawayTask, error) {
	if _, e := s.GetPutawayTask(ctx, t, id); e != nil {
		return models.PutawayTask{}, e
	}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		v, x := s.ops().LockPutawayTask(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return notFound(x, "Put-away task")
		}
		same, x := s.claim(ctx, tx, "putaway", t.OrganizationID, id, v.Status, v.LeaseOwner, v.LeaseExpiresAt, t.ActorID, in.ClientClaimID)
		if x != nil || same {
			return x
		}
		if x = s.ops().ClaimPutawayTask(ctx, tx, t.OrganizationID, id, t.ActorID, in.ClientClaimID, s.o.Now().Add(s.taskLease())); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "putaway_task.claimed.v1", "putaway_task", id, v.Version+1, map[string]any{"taskId": id})
	})
	if e != nil {
		return models.PutawayTask{}, e
	}
	return s.GetPutawayTask(ctx, t, id)
}

// CompletePutawayTask confirms the destination scan and moves the accepted
// stock from the receiving location into the destination bin.
func (s *Service) CompletePutawayTask(ctx context.Context, t tenant.Context, id string, in models.PutawayCompleteInput) (models.PutawayTask, error) {
	current, e := s.GetPutawayTask(ctx, t, id)
	if e != nil {
		return current, e
	}
	if current.Status == "COMPLETED" && current.LeaseOwner == "" {
		return current, nil
	}
	org := t.OrganizationID
	e = s.tx(ctx, func(tx pgx.Tx) error {
		v, x := s.ops().LockPutawayTask(ctx, tx, org, id)
		if x != nil {
			return notFound(x, "Put-away task")
		}
		if x = s.requireLease(v.Status, v.LeaseOwner, v.LeaseExpiresAt, t.ActorID); x != nil {
			return x
		}
		to := in.ToLocationID
		if to == "" {
			to = v.SuggestedLocationID
		}
		if to == "" {
			return apperrors.Validation("toLocationId is required.", nil)
		}
		dest, x := s.activeLocation(ctx, tx, org, v.WarehouseID, to)
		if x != nil {
			return x
		}
		if dest.Type == "QUARANTINE" || dest.Type == "DOCK" || dest.Type == "YARD_BAY" {
			return errScan("Put-away destination must be a storage location.")
		}
		if strings.TrimSpace(in.ScannedLocationCode) != dest.Code {
			return errScan("Scanned location does not match the put-away destination.")
		}
		src, x := s.repo.LockItem(ctx, tx, org, v.InventoryItemID)
		if x != nil {
			return notFound(x, "Inventory item")
		}
		if strings.TrimSpace(in.ScannedSKU) != src.SKU {
			return errScan("Scanned SKU does not match the put-away task.")
		}
		if in.Quantity != v.Quantity {
			return apperrors.Validation("Put-away quantity must equal the task quantity.", nil)
		}
		if src.QuantityProjection-src.ReservedQuantity < v.Quantity {
			return errStock("Receiving stock is no longer available for put-away.")
		}
		after, x := s.ops().SubtractItemQuantity(ctx, tx, org, src.ID, v.Quantity)
		if x != nil {
			return x
		}
		srcMove := models.Movement{ID: ids.New("imv"), OrganizationID: org, InventoryItemID: src.ID, MovementType: "TRANSFER", FromLocationID: src.LocationID, ToLocationID: to, Quantity: v.Quantity, ReasonCode: "PUT_AWAY_OUT", ActorReference: t.ActorID, CorrelationID: "putaway:" + id + ":out", ProjectionBefore: src.QuantityProjection, ProjectionAfter: after.QuantityProjection}
		if x = s.repo.InsertMovement(ctx, tx, &srcMove); x != nil {
			return x
		}
		if x = s.capacity(ctx, tx, org, to, v.Quantity); x != nil {
			return x
		}
		key := src
		destItem, x := s.ops().LockStockLine(ctx, tx, key, to)
		if errors.Is(x, pgx.ErrNoRows) {
			key.ID, key.LocationID = ids.New("inv"), to
			destItem, x = s.ops().InsertItemWithStatus(ctx, tx, key, v.Quantity, "AVAILABLE")
		} else if x == nil {
			destItem, x = s.ops().AddItemQuantityKeepStatus(ctx, tx, org, destItem.ID, v.Quantity)
		}
		if x != nil {
			return x
		}
		dstMove := models.Movement{ID: ids.New("imv"), OrganizationID: org, InventoryItemID: destItem.ID, MovementType: "PUT_AWAY", FromLocationID: src.LocationID, ToLocationID: to, Quantity: v.Quantity, ReasonCode: "PUT_AWAY_IN", ActorReference: t.ActorID, CorrelationID: "putaway:" + id + ":in", ProjectionBefore: destItem.QuantityProjection - v.Quantity, ProjectionAfter: destItem.QuantityProjection}
		if x = s.repo.InsertMovement(ctx, tx, &dstMove); x != nil {
			return x
		}
		if x = s.ops().CompletePutawayTask(ctx, tx, org, id, to, destItem.ID, t.ActorID); x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, "putaway_task.completed.v1", "putaway_task", id, v.Version+1, map[string]any{"taskId": id, "toLocationId": to, "quantity": v.Quantity}); x != nil {
			return x
		}
		open, x := s.ops().OpenPutawayCount(ctx, tx, org, v.ReceiptID)
		if x != nil || open > 0 {
			return x
		}
		r, x := s.ops().LockReceipt(ctx, tx, org, v.ReceiptID)
		if x != nil {
			return x
		}
		if r.Status != "PUTAWAY_IN_PROGRESS" {
			return nil
		}
		done, x := s.ops().TransitionReceipt(ctx, tx, org, r.ID, r.Version, "COMPLETED", "")
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "warehouse_receipt.completed.v1", "warehouse_receipt", r.ID, done.Version, map[string]any{"receiptId": r.ID, "status": "COMPLETED"})
	})
	if e != nil {
		return models.PutawayTask{}, e
	}
	return s.GetPutawayTask(ctx, t, id)
}

// ---------------------------------------------------------------------------
// Pick waves and tasks
// ---------------------------------------------------------------------------

// CreatePickWave groups RELEASED fulfilment orders (whose stock is already
// reserved under row lock) and creates one pick task per line. The wave
// number is the natural idempotency key.
func (s *Service) CreatePickWave(ctx context.Context, t tenant.Context, in models.PickWaveInput) (models.PickWave, error) {
	if e := s.gate(s.o.Flags.Fulfilment, true); e != nil {
		return models.PickWave{}, e
	}
	in.WaveNumber = strings.TrimSpace(in.WaveNumber)
	if in.WarehouseID == "" || in.WaveNumber == "" || len(in.FulfilmentOrderIDs) == 0 {
		return models.PickWave{}, apperrors.Validation("Warehouse, wave number and fulfilment orders are required.", nil)
	}
	if existing, e := s.ops().WaveByNumber(ctx, t.OrganizationID, in.WaveNumber); e == nil {
		return s.GetPickWave(ctx, t, existing.ID)
	}
	w, e := s.GetWarehouse(ctx, t, in.WarehouseID)
	if e != nil {
		return models.PickWave{}, e
	}
	if w.Status != "ACTIVE" {
		return models.PickWave{}, errState("Warehouse is not active.")
	}
	org := t.OrganizationID
	wave := models.PickWave{ID: ids.New("pwv"), OrganizationID: org, WarehouseID: in.WarehouseID, WaveNumber: in.WaveNumber}
	seen := map[string]bool{}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		if wave, x = s.ops().InsertWave(ctx, tx, wave, t.ActorID); x != nil {
			return x
		}
		for _, orderID := range in.FulfilmentOrderIDs {
			if seen[orderID] {
				continue
			}
			seen[orderID] = true
			wh, status, x := s.ops().LockOrderForWave(ctx, tx, org, orderID)
			if x != nil || wh != in.WarehouseID {
				return apperrors.NotFound("Fulfilment order not found in warehouse.", nil)
			}
			if status != "RELEASED" {
				return errState("Only released fulfilment orders can be waved.")
			}
			lines, x := s.ops().FulfilmentLinesForWave(ctx, tx, org, orderID)
			if x != nil {
				return x
			}
			for _, l := range lines {
				l.ID, l.OrganizationID, l.PickWaveID = ids.New("pkt"), org, wave.ID
				if x = s.ops().InsertPickTask(ctx, tx, l); x != nil {
					return x
				}
			}
			if _, x = s.ops().SetOrderStatusLocked(ctx, tx, org, orderID, "PICKING"); x != nil {
				return x
			}
		}
		return s.record(ctx, tx, t, "pick_wave.released.v1", "pick_wave", wave.ID, wave.Version, map[string]any{"pickWaveId": wave.ID, "warehouseId": wave.WarehouseID, "orderCount": len(seen)})
	})
	if e != nil {
		return models.PickWave{}, e
	}
	return s.GetPickWave(ctx, t, wave.ID)
}

func (s *Service) GetPickWave(ctx context.Context, t tenant.Context, id string) (models.PickWave, error) {
	if e := s.gate(s.o.Flags.Fulfilment, false); e != nil {
		return models.PickWave{}, e
	}
	v, e := s.ops().GetWave(ctx, t.OrganizationID, id)
	if e != nil {
		return v, notFound(e, "Pick wave")
	}
	if e = s.taskWarehouseInScope(ctx, t, v.WarehouseID); e != nil {
		return models.PickWave{}, apperrors.NotFound("Pick wave not found.", nil)
	}
	v.Tasks, e = s.ops().WaveTasks(ctx, t.OrganizationID, id)
	return v, e
}

func (s *Service) ListPickWaves(ctx context.Context, t tenant.Context) ([]models.PickWave, error) {
	if e := s.gate(s.o.Flags.Fulfilment, false); e != nil {
		return nil, e
	}
	return s.ops().ListWaves(ctx, t.OrganizationID, t.PermittedBranchIDs)
}

func (s *Service) ListPickTasks(ctx context.Context, t tenant.Context) ([]models.PickTask, error) {
	if e := s.gate(s.o.Flags.Fulfilment, false); e != nil {
		return nil, e
	}
	return s.ops().ListPickTasks(ctx, t.OrganizationID, t.PermittedBranchIDs)
}

func (s *Service) GetPickTask(ctx context.Context, t tenant.Context, id string) (models.PickTask, error) {
	if e := s.gate(s.o.Flags.Fulfilment, false); e != nil {
		return models.PickTask{}, e
	}
	v, e := s.ops().GetPickTask(ctx, t.OrganizationID, id)
	if e != nil {
		return v, notFound(e, "Pick task")
	}
	if _, e = s.GetPickWave(ctx, t, v.PickWaveID); e != nil {
		return models.PickTask{}, apperrors.NotFound("Pick task not found.", nil)
	}
	return v, nil
}

// ClaimPickTask leases a pick task to the calling worker.
func (s *Service) ClaimPickTask(ctx context.Context, t tenant.Context, id string, in models.TaskClaimInput) (models.PickTask, error) {
	if _, e := s.GetPickTask(ctx, t, id); e != nil {
		return models.PickTask{}, e
	}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		v, x := s.ops().LockPickTask(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return notFound(x, "Pick task")
		}
		same, x := s.claim(ctx, tx, "pick", t.OrganizationID, id, v.Status, v.LeaseOwner, v.LeaseExpiresAt, t.ActorID, in.ClientClaimID)
		if x != nil || same {
			return x
		}
		if x = s.ops().ClaimPickTask(ctx, tx, t.OrganizationID, id, t.ActorID, in.ClientClaimID, s.o.Now().Add(s.taskLease())); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "pick_task.claimed.v1", "pick_task", id, v.Version+1, map[string]any{"taskId": id})
	})
	if e != nil {
		return models.PickTask{}, e
	}
	return s.GetPickTask(ctx, t, id)
}

// cancelPickingOrder ends a wave order that can no longer complete, for
// example after a short pick. The caller holds the order and its lines
// locked, which also serializes it against pick-task completion (that path
// locks the order first). Every pick task must be finished: an open or
// claimed task still holds stock an operator may be carrying. Each line
// still holds exactly its picked quantity (a wave creates one task per line
// and a short pick already released the unpicked remainder), so that amount
// is released and a RELEASE movement balances the PICK in the history.
func (s *Service) cancelPickingOrder(ctx context.Context, tx pgx.Tx, t tenant.Context, orderID string, lines []models.FulfilmentLineState) error {
	ops := s.ops()
	if ops == nil {
		return apperrors.Unavailable("Warehouse operations are unavailable.", nil)
	}
	org := t.OrganizationID
	open, _, x := ops.OrderPickProgress(ctx, tx, org, orderID)
	if x != nil {
		return x
	}
	if open > 0 {
		return errState("Finish or release the order's open pick tasks before cancelling it.")
	}
	for _, l := range lines {
		if _, x = s.repo.LockItem(ctx, tx, org, l.InventoryItemID); x != nil {
			return x
		}
		if l.Picked > 0 {
			if x = s.repo.ReleaseItemReservation(ctx, tx, org, l.InventoryItemID, l.Picked); x != nil {
				return x
			}
			if x = ops.InsertPickReleaseMovement(ctx, tx, org, l.InventoryItemID, orderID, t.ActorID, l.Picked); x != nil {
				return x
			}
		}
		if x = ops.ReleaseOrderReservations(ctx, tx, org, l.InventoryItemID, orderID); x != nil {
			return x
		}
	}
	return nil
}

// CompletePickTask confirms the source scan against the reserved stock. A
// short pick releases the unpicked reservation and leaves the order in
// PICKING, from which an operator cancels it once no task is open (see
// cancelPickingOrder); a full pick of every task moves the order to PICKED.
func (s *Service) CompletePickTask(ctx context.Context, t tenant.Context, id string, in models.PickCompleteInput) (models.PickTask, error) {
	current, e := s.GetPickTask(ctx, t, id)
	if e != nil {
		return current, e
	}
	if (current.Status == "COMPLETED" || current.Status == "SHORT") && current.PickedQuantity == in.PickedQuantity {
		return current, nil
	}
	org := t.OrganizationID
	e = s.tx(ctx, func(tx pgx.Tx) error {
		// Lock order before task so put-away/pick/dispatch on the same
		// order serialize in one direction.
		if _, _, x := s.ops().LockOrderForWave(ctx, tx, org, current.FulfilmentOrderID); x != nil {
			return x
		}
		v, x := s.ops().LockPickTask(ctx, tx, org, id)
		if x != nil {
			return notFound(x, "Pick task")
		}
		if x = s.requireLease(v.Status, v.LeaseOwner, v.LeaseExpiresAt, t.ActorID); x != nil {
			return x
		}
		if in.PickedQuantity <= 0 || in.PickedQuantity > v.Quantity {
			return apperrors.Validation("pickedQuantity must be positive and no more than the task quantity.", nil)
		}
		loc, x := s.ops().LocationRef(ctx, tx, org, v.FromLocationID)
		if x != nil {
			return x
		}
		if strings.TrimSpace(in.ScannedLocationCode) != loc.Code {
			return errScan("Scanned location does not match the pick location.")
		}
		item, x := s.repo.LockItem(ctx, tx, org, v.InventoryItemID)
		if x != nil {
			return x
		}
		if item.LocationID != v.FromLocationID {
			return errStock("Reserved stock moved after the wave was created.")
		}
		if strings.TrimSpace(in.ScannedSKU) != item.SKU {
			return errScan("Scanned SKU does not match the pick task.")
		}
		reserved, x := s.ops().ActiveReservation(ctx, tx, org, v.InventoryItemID, v.FulfilmentOrderID)
		if x != nil {
			return x
		}
		if reserved < v.Quantity {
			return errStock("The stock reservation for this pick has expired or been released.")
		}
		status := "COMPLETED"
		if in.PickedQuantity < v.Quantity {
			status = "SHORT"
			if x = s.repo.ReleaseItemReservation(ctx, tx, org, v.InventoryItemID, v.Quantity-in.PickedQuantity); x != nil {
				return x
			}
		}
		if x = s.repo.SetItemStatus(ctx, tx, v.InventoryItemID, "PICKING"); x != nil {
			return x
		}
		if x = s.ops().SetLinePickedQuantity(ctx, tx, v.FulfilmentLineID, in.PickedQuantity); x != nil {
			return x
		}
		if x = s.repo.SetReservationsStatus(ctx, tx, org, v.InventoryItemID, v.FulfilmentOrderID, "PICKED"); x != nil {
			return x
		}
		if x = s.ops().InsertPickTaskMovement(ctx, tx, org, id, v.InventoryItemID, v.FulfilmentOrderID, t.ActorID, in.PickedQuantity); x != nil {
			return x
		}
		if x = s.ops().CompletePickTask(ctx, tx, org, id, status, t.ActorID, in.PickedQuantity); x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, "pick_task."+strings.ToLower(status)+".v1", "pick_task", id, v.Version+1, map[string]any{"taskId": id, "pickedQuantity": in.PickedQuantity, "status": status}); x != nil {
			return x
		}
		open, short, x := s.ops().OrderPickProgress(ctx, tx, org, v.FulfilmentOrderID)
		if x != nil {
			return x
		}
		if open == 0 && short == 0 {
			ver, x := s.ops().SetOrderStatusLocked(ctx, tx, org, v.FulfilmentOrderID, "PICKED")
			if x != nil {
				return x
			}
			if x = s.record(ctx, tx, t, "fulfilment_order.picked.v1", "fulfilment_order", v.FulfilmentOrderID, ver, map[string]any{"fulfilmentOrderId": v.FulfilmentOrderID, "status": "PICKED"}); x != nil {
				return x
			}
		}
		waveOpen, x := s.ops().WaveOpenCount(ctx, tx, org, v.PickWaveID)
		if x != nil || waveOpen > 0 {
			return x
		}
		waveVersion, x := s.ops().CompleteWave(ctx, tx, org, v.PickWaveID)
		if x != nil || waveVersion == 0 {
			return x
		}
		return s.record(ctx, tx, t, "pick_wave.completed.v1", "pick_wave", v.PickWaveID, waveVersion, map[string]any{"pickWaveId": v.PickWaveID})
	})
	if e != nil {
		return models.PickTask{}, e
	}
	return s.GetPickTask(ctx, t, id)
}

// ---------------------------------------------------------------------------
// Packing units
// ---------------------------------------------------------------------------

// CreatePackingUnit packs picked stock of one fulfilment order. Packed
// quantities across all units can never exceed what was picked.
func (s *Service) CreatePackingUnit(ctx context.Context, t tenant.Context, in models.PackingUnit) (models.PackingUnit, error) {
	if e := s.gate(s.o.Flags.Fulfilment, true); e != nil {
		return models.PackingUnit{}, e
	}
	in.PackingCode = strings.TrimSpace(in.PackingCode)
	if in.FulfilmentOrderID == "" || in.PackingCode == "" || len(in.Contents) == 0 {
		return models.PackingUnit{}, apperrors.Validation("Fulfilment order, packing code and contents are required.", nil)
	}
	if existing, e := s.ops().PackingByCode(ctx, t.OrganizationID, in.PackingCode); e == nil {
		if existing.FulfilmentOrderID != in.FulfilmentOrderID {
			return existing, apperrors.Conflict("Packing code is already used.", nil)
		}
		return existing, nil
	}
	if _, e := s.GetFulfilment(ctx, t, in.FulfilmentOrderID); e != nil {
		return models.PackingUnit{}, e
	}
	org := t.OrganizationID
	var out models.PackingUnit
	e := s.tx(ctx, func(tx pgx.Tx) error {
		_, status, x := s.ops().LockOrderForWave(ctx, tx, org, in.FulfilmentOrderID)
		if x != nil {
			return x
		}
		if status != "PICKED" && status != "PACKING" {
			return errState("Only a picked fulfilment order can be packed.")
		}
		picked, x := s.ops().PickedQuantities(ctx, tx, org, in.FulfilmentOrderID)
		if x != nil {
			return x
		}
		packed, x := s.ops().PackedQuantities(ctx, tx, org, in.FulfilmentOrderID)
		if x != nil {
			return x
		}
		for _, c := range in.Contents {
			if c.Quantity <= 0 {
				return apperrors.Validation("Packed quantities must be positive.", nil)
			}
			if _, ok := picked[c.InventoryItemID]; !ok {
				return apperrors.Validation("Packed item is not on the fulfilment order.", nil)
			}
			packed[c.InventoryItemID] += c.Quantity
			if packed[c.InventoryItemID] > picked[c.InventoryItemID] {
				return errStock("Packed quantity exceeds the picked quantity.")
			}
		}
		in.ID, in.OrganizationID = ids.New("pku"), org
		in.Status = "PACKED"
		if strings.TrimSpace(in.SealNumber) != "" {
			in.Status = "SEALED"
		}
		if in.DimensionsMm != nil && !json.Valid(in.DimensionsMm) {
			return apperrors.Validation("dimensionsMm must be a JSON object.", nil)
		}
		if out, x = s.ops().InsertPackingUnit(ctx, tx, in, t.ActorID); x != nil {
			return x
		}
		if status == "PICKED" {
			if _, x = s.ops().SetOrderStatusLocked(ctx, tx, org, in.FulfilmentOrderID, "PACKING"); x != nil {
				return x
			}
		}
		return s.record(ctx, tx, t, "packing_unit.created.v1", "packing_unit", out.ID, out.Version, map[string]any{"packingUnitId": out.ID, "fulfilmentOrderId": out.FulfilmentOrderID, "status": out.Status})
	})
	return out, e
}

func (s *Service) GetPackingUnit(ctx context.Context, t tenant.Context, id string) (models.PackingUnit, error) {
	if e := s.gate(s.o.Flags.Fulfilment, false); e != nil {
		return models.PackingUnit{}, e
	}
	v, e := s.ops().GetPackingUnit(ctx, t.OrganizationID, id)
	if e != nil {
		return v, notFound(e, "Packing unit")
	}
	if _, e = s.GetFulfilment(ctx, t, v.FulfilmentOrderID); e != nil {
		return models.PackingUnit{}, apperrors.NotFound("Packing unit not found.", nil)
	}
	return v, nil
}

func (s *Service) ListPackingUnits(ctx context.Context, t tenant.Context) ([]models.PackingUnit, error) {
	if e := s.gate(s.o.Flags.Fulfilment, false); e != nil {
		return nil, e
	}
	return s.ops().ListPackingUnits(ctx, t.OrganizationID, t.PermittedBranchIDs)
}

// ---------------------------------------------------------------------------
// Gate events
// ---------------------------------------------------------------------------

// RecordGateEvent appends a gate event in strict yard order: ENTRY first,
// nothing after EXIT.
func (s *Service) RecordGateEvent(ctx context.Context, t tenant.Context, visitID string, in models.GateEventInput) (models.GateEvent, error) {
	visit, e := s.GetYard(ctx, t, visitID)
	if e != nil {
		return models.GateEvent{}, e
	}
	if s.o.Flags.KillSwitch {
		return models.GateEvent{}, apperrors.Unavailable("Fleet integrations are disabled.", nil)
	}
	in.EventType = strings.ToUpper(strings.TrimSpace(in.EventType))
	switch in.EventType {
	case "ENTRY", "SECURITY_CHECK", "SEAL_CHECK", "PARKING", "EXIT":
	default:
		return models.GateEvent{}, apperrors.Validation("eventType must be ENTRY, SECURITY_CHECK, SEAL_CHECK, PARKING or EXIT.", nil)
	}
	if strings.TrimSpace(in.GateCode) == "" {
		return models.GateEvent{}, apperrors.Validation("gateCode is required.", nil)
	}
	if len(in.Notes) > 1000 {
		return models.GateEvent{}, apperrors.Validation("notes must be at most 1000 characters.", nil)
	}
	evidence, _ := json.Marshal(in.EvidenceFileIDs)
	if in.EvidenceFileIDs == nil {
		evidence = []byte("[]")
	}
	var out models.GateEvent
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if _, x := s.ops().LockYardVisitRow(ctx, tx, t.OrganizationID, visit.ID); x != nil {
			return notFound(x, "Yard visit")
		}
		prior, x := s.ops().GateEvents(ctx, tx, t.OrganizationID, visit.ID)
		if x != nil {
			return x
		}
		entered, exited := false, false
		for _, p := range prior {
			entered = entered || p.EventType == "ENTRY"
			exited = exited || p.EventType == "EXIT"
		}
		switch {
		case exited:
			return errState("The vehicle has already exited through the gate.")
		case in.EventType == "ENTRY" && entered:
			return errState("The vehicle has already entered.")
		case in.EventType != "ENTRY" && !entered:
			return errState("An ENTRY gate event must be recorded first.")
		}
		v := models.GateEvent{ID: ids.New("gte"), OrganizationID: t.OrganizationID, YardVisitID: visit.ID, SequenceNo: len(prior) + 1, EventType: in.EventType, GateCode: in.GateCode, ActorReference: t.ActorID, EvidenceFileIDs: evidence, Notes: in.Notes}
		if out, x = s.ops().InsertGateEvent(ctx, tx, v); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "yard.gate_event_recorded.v1", "yard_visit", visit.ID, int64(out.SequenceNo), map[string]any{"yardVisitId": visit.ID, "eventType": out.EventType, "gateCode": out.GateCode, "sequenceNo": out.SequenceNo})
	})
	return out, e
}

func (s *Service) ListGateEvents(ctx context.Context, t tenant.Context, visitID string) ([]models.GateEvent, error) {
	visit, e := s.GetYard(ctx, t, visitID)
	if e != nil {
		return nil, e
	}
	return s.ops().GateEvents(ctx, nil, t.OrganizationID, visit.ID)
}
