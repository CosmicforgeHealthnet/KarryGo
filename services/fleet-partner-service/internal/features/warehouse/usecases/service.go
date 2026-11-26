// Package usecases implements the Section 21 warehouse workflows: warehouses
// and storage locations, customer stock receipts and movements, fulfilment,
// dock, yard, terminal transfers, returns and the operations sweep.
// Maintenance parts remain owned by Section 18 and canonical trips remain
// external.
package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

type Events interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type ProofGate interface {
	MandatorySatisfied(context.Context, string, string, string, string, string, string) (bool, error)
}
type RestrictionGate interface {
	Restricted(context.Context, string, string, string) (bool, error)
}
type Options struct {
	Pool         *pgxpool.Pool
	Repository   Repository
	Events       Events
	Audit        *audit.Recorder
	Queue        *jobs.Queue
	Proof        ProofGate
	Restrictions RestrictionGate
	Flags        models.Flags
	Now          func() time.Time
	// TaskLease is how long a claimed put-away or pick task is held;
	// zero means DefaultTaskLease.
	TaskLease time.Duration
}
type Service struct {
	o    Options
	repo Repository
}

func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Queue == nil {
		o.Queue = jobs.NewQueue(o.Pool)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{o: o, repo: o.Repository}
}
func (s *Service) gate(on, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet operations are disabled.", nil)
	}
	if !on {
		if write {
			return apperrors.Forbidden("Capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}
func (s *Service) tx(ctx context.Context, f func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, f)
}
func (s *Service) requireUnrestricted(ctx context.Context, t tenant.Context, subjectType, subjectReference string) error {
	if s.o.Restrictions == nil {
		return apperrors.Unavailable("Compliance restriction validation is unavailable.", nil)
	}
	restricted, err := s.o.Restrictions.Restricted(ctx, t.OrganizationID, subjectType, subjectReference)
	if err != nil {
		return apperrors.Unavailable("Compliance restriction validation is unavailable.", err)
	}
	if restricted {
		return apperrors.Conflict("Operation is blocked by an active compliance restriction.", nil)
	}
	return nil
}
func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, v int64, p any) error {
	if e := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); e != nil {
		return e
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, v, t.OrganizationID, t.CorrelationID, p)
	}
	return nil
}

func (s *Service) CreateWarehouse(ctx context.Context, t tenant.Context, v models.Warehouse) (models.Warehouse, error) {
	if e := s.gate(s.o.Flags.Warehouses, true); e != nil {
		return v, e
	}
	if !t.BranchPermitted(v.BranchID) {
		return v, apperrors.NotFound("Branch not found.", nil)
	}
	if v.Code == "" || v.Name == "" || v.AddressReference == "" || v.Timezone == "" {
		return v, apperrors.BadRequest("Branch, code, name, address and timezone are required.", nil)
	}
	if _, e := time.LoadLocation(v.Timezone); e != nil {
		return v, apperrors.BadRequest("Timezone must be a valid IANA timezone.", nil)
	}
	if len(v.OperatingHours) == 0 {
		v.OperatingHours = json.RawMessage(`{}`)
	}
	v.ID = ids.New("whs")
	v.OrganizationID = t.OrganizationID
	v.Code = strings.ToUpper(v.Code)
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.InsertWarehouse(ctx, tx, v)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "warehouse.created.v1", "warehouse", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetWarehouse(ctx context.Context, t tenant.Context, id string) (models.Warehouse, error) {
	if e := s.gate(s.o.Flags.Warehouses, false); e != nil {
		return models.Warehouse{}, e
	}
	v, e := s.repo.GetWarehouse(ctx, t.OrganizationID, id)
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !t.BranchPermitted(v.BranchID) {
		e = apperrors.NotFound("Warehouse not found.", nil)
	}
	return v, e
}
func (s *Service) ListWarehouses(ctx context.Context, t tenant.Context) ([]models.Warehouse, error) {
	if e := s.gate(s.o.Flags.Warehouses, false); e != nil {
		return nil, e
	}
	return s.repo.ListWarehouses(ctx, t.OrganizationID, t.PermittedBranchIDs)
}
func (s *Service) UpdateWarehouse(ctx context.Context, t tenant.Context, id string, expected int64, v models.Warehouse, archive bool) (models.Warehouse, error) {
	old, e := s.GetWarehouse(ctx, t, id)
	if e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	status := v.Status
	if status == "" {
		status = old.Status
	}
	event := "warehouse.updated.v1"
	if archive {
		if s.repo.WarehouseHasStock(ctx, id) {
			return v, apperrors.Conflict("Warehouse with operational stock cannot be archived.", nil)
		}
		status = "ARCHIVED"
		event = "warehouse.archived.v1"
	}
	if v.Timezone != "" {
		if _, e = time.LoadLocation(v.Timezone); e != nil {
			return v, apperrors.BadRequest("Timezone must be a valid IANA timezone.", nil)
		}
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.UpdateWarehouse(ctx, tx, t.OrganizationID, id, expected, v, status)
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, event, "warehouse", id, v.Version, v)
	})
	return v, e
}

func (s *Service) CreateLocation(ctx context.Context, t tenant.Context, warehouseID string, v models.Location) (models.Location, error) {
	if e := s.gate(s.o.Flags.Warehouses, true); e != nil {
		return v, e
	}
	w, e := s.GetWarehouse(ctx, t, warehouseID)
	if e != nil {
		return v, e
	}
	if w.Status == "ARCHIVED" {
		return v, apperrors.Conflict("Archived warehouse cannot receive locations.", nil)
	}
	if v.ParentLocationID != "" {
		p, e := s.repo.GetLocation(ctx, t.OrganizationID, warehouseID, v.ParentLocationID)
		if e != nil || p.ID == "" {
			return v, apperrors.NotFound("Parent location not found.", nil)
		}
	}
	if v.Code == "" || v.Name == "" || v.LocationType == "" {
		return v, apperrors.BadRequest("Code, name and location type are required.", nil)
	}
	v.ID = ids.New("loc")
	v.OrganizationID = t.OrganizationID
	v.WarehouseID = warehouseID
	v.Code = strings.ToUpper(v.Code)
	v.LocationType = strings.ToUpper(v.LocationType)
	v.Restrictions = models.Raw(v.Restrictions, "{}")
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.InsertLocation(ctx, tx, v)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "warehouse.location_created.v1", "storage_location", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetLocation(ctx context.Context, t tenant.Context, warehouseID, id string) (models.Location, error) {
	if _, e := s.GetWarehouse(ctx, t, warehouseID); e != nil {
		return models.Location{}, e
	}
	v, e := s.repo.GetLocation(ctx, t.OrganizationID, warehouseID, id)
	if errors.Is(e, pgx.ErrNoRows) {
		e = apperrors.NotFound("Storage location not found.", nil)
	}
	return v, e
}
func (s *Service) ListLocations(ctx context.Context, t tenant.Context, warehouseID string) ([]models.Location, error) {
	if _, e := s.GetWarehouse(ctx, t, warehouseID); e != nil {
		return nil, e
	}
	return s.repo.ListLocations(ctx, t.OrganizationID, warehouseID)
}
func (s *Service) UpdateLocation(ctx context.Context, t tenant.Context, warehouseID, id string, expected int64, v models.Location) (models.Location, error) {
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	old, e := s.GetLocation(ctx, t, warehouseID, id)
	if e != nil {
		return v, e
	}
	if v.ParentLocationID == id {
		return v, apperrors.Conflict("Location hierarchy cycle detected.", nil)
	}
	if v.ParentLocationID != "" {
		cycle, e := s.repo.LocationParentCycle(ctx, id, v.ParentLocationID)
		if e != nil {
			return v, e
		}
		if cycle {
			return v, apperrors.Conflict("Location hierarchy cycle detected.", nil)
		}
	} else {
		v.ParentLocationID = old.ParentLocationID
	}
	status := v.Status
	if status == "" {
		status = old.Status
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.UpdateLocation(ctx, tx, t.OrganizationID, id, expected, v, status)
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "warehouse.location_updated.v1", "storage_location", id, v.Version, v)
	})
	return v, e
}

func (s *Service) capacity(ctx context.Context, tx pgx.Tx, org, location string, additional float64) error {
	capacity, e := s.repo.LockLocationCapacity(ctx, tx, org, location)
	if e != nil {
		return apperrors.NotFound("Active storage location not found.", nil)
	}
	if capacity == nil {
		return nil
	}
	used, e := s.repo.LocationUsedQuantity(ctx, tx, org, location)
	if e != nil {
		return e
	}
	if used+additional > *capacity {
		return apperrors.Conflict("Storage location capacity would be exceeded.", nil)
	}
	return nil
}
func (s *Service) recordCustodyTx(ctx context.Context, tx pgx.Tx, t tenant.Context, in models.ReceiptInput) error {
	if in.PackageID == "" {
		return nil
	}
	shipment, e := s.repo.PackageShipment(ctx, tx, t.OrganizationID, in.PackageID)
	if e != nil {
		return apperrors.NotFound("Package not found.", nil)
	}
	if in.ShipmentID != "" && shipment != in.ShipmentID {
		return apperrors.Conflict("Package does not belong to shipment.", nil)
	}
	return s.repo.InsertReceiptCustody(ctx, tx, t.OrganizationID, in, shipment, s.o.Now())
}
func (s *Service) Receipt(ctx context.Context, t tenant.Context, in models.ReceiptInput) (models.Item, error) {
	if e := s.gate(s.o.Flags.Inventory, true); e != nil {
		return models.Item{}, e
	}
	if in.CorrelationID == "" || in.Quantity <= 0 || in.WarehouseID == "" || in.LocationID == "" || in.OwnerType == "" || in.OwnerReference == "" || in.SKU == "" {
		return models.Item{}, apperrors.BadRequest("Warehouse, location, owner, SKU, positive quantity and correlationId are required.", nil)
	}
	if e := s.requireUnrestricted(ctx, t, "WAREHOUSE", in.WarehouseID); e != nil {
		return models.Item{}, e
	}
	if e := s.requireUnrestricted(ctx, t, "LOCATION", in.LocationID); e != nil {
		return models.Item{}, e
	}
	existingID, e := s.repo.MovementItemByCorrelation(ctx, t.OrganizationID, in.CorrelationID)
	if e == nil {
		return s.GetItem(ctx, t, existingID)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return models.Item{}, e
	}
	w, e := s.GetWarehouse(ctx, t, in.WarehouseID)
	if e != nil {
		return models.Item{}, e
	}
	if w.Status != "ACTIVE" {
		return models.Item{}, apperrors.Conflict("Warehouse is not active.", nil)
	}
	var item models.Item
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if x := s.capacity(ctx, tx, t.OrganizationID, in.LocationID, in.Quantity); x != nil {
			return x
		}
		var x error
		item, x = s.repo.LockReceiptItem(ctx, tx, t.OrganizationID, in)
		if errors.Is(x, pgx.ErrNoRows) {
			item = models.Item{ID: ids.New("inv"), OrganizationID: t.OrganizationID, WarehouseID: in.WarehouseID, LocationID: in.LocationID, OwnerType: in.OwnerType, OwnerReference: in.OwnerReference, SKU: in.SKU, BatchReference: in.BatchReference, LotReference: in.LotReference, SerialReference: in.SerialReference, Unit: in.Unit, Status: "AVAILABLE", ExpiryAt: in.ExpiryAt, Condition: in.Condition}
			item, x = s.repo.InsertItem(ctx, tx, item, in.Quantity)
		} else if x == nil {
			item, x = s.repo.AddItemQuantity(ctx, tx, t.OrganizationID, item.ID, in.Quantity)
		}
		if x != nil {
			return x
		}
		before := item.QuantityProjection - in.Quantity
		movement := models.Movement{ID: ids.New("imv"), OrganizationID: t.OrganizationID, InventoryItemID: item.ID, MovementType: "RECEIPT", ToLocationID: in.LocationID, Quantity: in.Quantity, ReasonCode: "WAREHOUSE_RECEIPT", ActorReference: t.ActorID, CorrelationID: in.CorrelationID, ProjectionBefore: before, ProjectionAfter: item.QuantityProjection}
		if x = s.repo.InsertReceiptMovement(ctx, tx, movement); x != nil {
			return x
		}
		if x = s.recordCustodyTx(ctx, tx, t, in); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "inventory.received.v1", "inventory_item", item.ID, item.Version, map[string]any{"inventoryItemId": item.ID, "movementId": movement.ID, "quantity": movement.Quantity, "unit": item.Unit})
	})
	return item, e
}
func (s *Service) GetItem(ctx context.Context, t tenant.Context, id string) (models.Item, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return models.Item{}, e
	}
	v, e := s.repo.GetItem(ctx, t.OrganizationID, id)
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !t.BranchPermitted(s.repo.WarehouseBranch(ctx, v.WarehouseID)) {
		e = apperrors.NotFound("Inventory item not found.", nil)
	}
	return v, e
}
func (s *Service) ListItems(ctx context.Context, t tenant.Context) ([]models.Item, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return nil, e
	}
	return s.repo.ListItems(ctx, t.OrganizationID, t.PermittedBranchIDs)
}
func (s *Service) Movements(ctx context.Context, t tenant.Context, id string) ([]models.Movement, error) {
	if _, e := s.GetItem(ctx, t, id); e != nil {
		return nil, e
	}
	return s.repo.ListMovements(ctx, t.OrganizationID, id)
}
func (s *Service) Move(ctx context.Context, t tenant.Context, id string, in models.MovementInput) (models.Movement, error) {
	if e := s.gate(s.o.Flags.Inventory, true); e != nil {
		return models.Movement{}, e
	}
	in.MovementType = strings.ToUpper(in.MovementType)
	if in.CorrelationID == "" || in.Quantity <= 0 || in.ReasonCode == "" || in.ExpectedVersion <= 0 {
		return models.Movement{}, apperrors.BadRequest("Movement type, positive quantity, reason, correlationId and expectedVersion are required.", nil)
	}
	v, e := s.repo.GetMovementByCorrelation(ctx, t.OrganizationID, in.CorrelationID)
	if e == nil {
		return v, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return v, e
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		item, x := s.repo.LockItem(ctx, tx, t.OrganizationID, id)
		if x != nil {
			return apperrors.NotFound("Inventory item not found.", x)
		}
		if item.Version != in.ExpectedVersion {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if item.Status == "QUARANTINED" && in.MovementType != "RELEASE" {
			return apperrors.Conflict("Quarantined stock cannot be moved.", nil)
		}
		if item.ExpiryAt != nil && !item.ExpiryAt.After(s.o.Now()) && in.MovementType != "EXPIRY" {
			return apperrors.Conflict("Expired stock cannot be moved.", nil)
		}
		before := item.QuantityProjection
		after := before
		status := item.Status
		location := item.LocationID
		switch in.MovementType {
		case "ADJUSTMENT":
			if in.Increase {
				after += in.Quantity
			} else {
				after -= in.Quantity
			}
		case "DISPATCH", "DAMAGE", "EXPIRY":
			after -= in.Quantity
		case "TRANSFER", "PUT_AWAY":
			if in.ToLocationID == "" || in.Quantity != before {
				return apperrors.Conflict("Location transfer must move the full inventory item.", nil)
			}
			if x = s.capacity(ctx, tx, t.OrganizationID, in.ToLocationID, before); x != nil {
				return x
			}
			location = in.ToLocationID
		case "QUARANTINE":
			status = "QUARANTINED"
		case "RELEASE":
			status = "AVAILABLE"
		case "PICK":
			status = "PICKING"
		case "PACK":
			status = "PACKED"
		case "RETURN":
			status = "RETURNED"
		default:
			return apperrors.BadRequest("Unsupported inventory movement type.", nil)
		}
		if after < 0 {
			return apperrors.Conflict("Movement would create negative stock.", nil)
		}
		if in.FulfilmentOrderID == "" && after < item.ReservedQuantity {
			return apperrors.Conflict("Movement would consume reserved stock.", nil)
		}
		if after == 0 && in.MovementType == "DISPATCH" {
			status = "DISPATCHED"
		}
		if x = s.repo.ApplyItemMove(ctx, tx, t.OrganizationID, id, location, after, status); x != nil {
			return x
		}
		v = models.Movement{ID: ids.New("imv"), OrganizationID: t.OrganizationID, InventoryItemID: id, MovementType: in.MovementType, FromLocationID: item.LocationID, ToLocationID: in.ToLocationID, Quantity: in.Quantity, ReasonCode: in.ReasonCode, ActorReference: t.ActorID, FulfilmentOrderID: in.FulfilmentOrderID, TerminalTransferID: in.TerminalTransferID, ReturnID: in.ReturnID, CorrelationID: in.CorrelationID, ProjectionBefore: before, ProjectionAfter: after}
		if x = s.repo.InsertMovement(ctx, tx, &v); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "inventory.moved.v1", "inventory_item", id, item.Version+1, map[string]any{"inventoryItemId": id, "movementId": v.ID, "movementType": v.MovementType, "quantity": v.Quantity})
	})
	return v, e
}
