package warehouse

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
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
	Events       Events
	Audit        *audit.Recorder
	Queue        *jobs.Queue
	Proof        ProofGate
	Restrictions RestrictionGate
	Flags        Flags
	Now          func() time.Time
}
type Service struct{ o Options }

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
	return &Service{o: o}
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

const warehouseCols = `id,organization_id,branch_id,code,name,site_reference,address_reference,timezone,operating_hours,status,version,created_at,updated_at,archived_at`

func scanWarehouse(r pgx.Row) (Warehouse, error) {
	var v Warehouse
	e := r.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.Code, &v.Name, &v.SiteReference, &v.AddressReference, &v.Timezone, &v.OperatingHours, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt)
	return v, e
}
func (s *Service) CreateWarehouse(ctx context.Context, t tenant.Context, v Warehouse) (Warehouse, error) {
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
		v, x = scanWarehouse(tx.QueryRow(ctx, `INSERT INTO fleet_partner_warehouses(id,organization_id,branch_id,code,name,site_reference,address_reference,timezone,operating_hours)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+warehouseCols, v.ID, v.OrganizationID, v.BranchID, v.Code, v.Name, v.SiteReference, v.AddressReference, v.Timezone, v.OperatingHours))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "warehouse.created.v1", "warehouse", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetWarehouse(ctx context.Context, t tenant.Context, id string) (Warehouse, error) {
	if e := s.gate(s.o.Flags.Warehouses, false); e != nil {
		return Warehouse{}, e
	}
	v, e := scanWarehouse(s.o.Pool.QueryRow(ctx, `SELECT `+warehouseCols+` FROM fleet_partner_warehouses WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !t.BranchPermitted(v.BranchID) {
		e = apperrors.NotFound("Warehouse not found.", nil)
	}
	return v, e
}
func (s *Service) ListWarehouses(ctx context.Context, t tenant.Context) ([]Warehouse, error) {
	if e := s.gate(s.o.Flags.Warehouses, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+warehouseCols+` FROM fleet_partner_warehouses WHERE organization_id=$1 AND(COALESCE(cardinality($2::text[]),0)=0 OR branch_id=ANY($2::text[])) ORDER BY created_at DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Warehouse{}
	for rows.Next() {
		v, e := scanWarehouse(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) UpdateWarehouse(ctx context.Context, t tenant.Context, id string, expected int64, v Warehouse, archive bool) (Warehouse, error) {
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
		var used bool
		_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_inventory_items WHERE warehouse_id=$1)`, id).Scan(&used)
		if used {
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
		v, x = scanWarehouse(tx.QueryRow(ctx, `UPDATE fleet_partner_warehouses SET name=COALESCE(NULLIF($4,''),name),site_reference=COALESCE(NULLIF($5,''),site_reference),address_reference=COALESCE(NULLIF($6,''),address_reference),timezone=COALESCE(NULLIF($7,''),timezone),operating_hours=CASE WHEN $8::jsonb='{}' THEN operating_hours ELSE $8 END,status=$9,archived_at=CASE WHEN $9='ARCHIVED' THEN now() ELSE archived_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+warehouseCols, t.OrganizationID, id, expected, v.Name, v.SiteReference, v.AddressReference, v.Timezone, raw(v.OperatingHours, "{}"), status))
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

const locationCols = `id,organization_id,warehouse_id,COALESCE(parent_location_id,''),code,name,location_type,capacity,capacity_unit,restrictions,status,version,created_at,updated_at`

func scanLocation(r pgx.Row) (Location, error) {
	var v Location
	e := r.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.ParentLocationID, &v.Code, &v.Name, &v.LocationType, &v.Capacity, &v.CapacityUnit, &v.Restrictions, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) CreateLocation(ctx context.Context, t tenant.Context, warehouseID string, v Location) (Location, error) {
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
		p, e := scanLocation(s.o.Pool.QueryRow(ctx, `SELECT `+locationCols+` FROM fleet_partner_storage_locations WHERE organization_id=$1 AND warehouse_id=$2 AND id=$3`, t.OrganizationID, warehouseID, v.ParentLocationID))
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
	v.Restrictions = raw(v.Restrictions, "{}")
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanLocation(tx.QueryRow(ctx, `INSERT INTO fleet_partner_storage_locations(id,organization_id,warehouse_id,parent_location_id,code,name,location_type,capacity,capacity_unit,restrictions)VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10) RETURNING `+locationCols, v.ID, v.OrganizationID, v.WarehouseID, v.ParentLocationID, v.Code, v.Name, v.LocationType, v.Capacity, v.CapacityUnit, v.Restrictions))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "warehouse.location_created.v1", "storage_location", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetLocation(ctx context.Context, t tenant.Context, warehouseID, id string) (Location, error) {
	if _, e := s.GetWarehouse(ctx, t, warehouseID); e != nil {
		return Location{}, e
	}
	v, e := scanLocation(s.o.Pool.QueryRow(ctx, `SELECT `+locationCols+` FROM fleet_partner_storage_locations WHERE organization_id=$1 AND warehouse_id=$2 AND id=$3`, t.OrganizationID, warehouseID, id))
	if errors.Is(e, pgx.ErrNoRows) {
		e = apperrors.NotFound("Storage location not found.", nil)
	}
	return v, e
}
func (s *Service) ListLocations(ctx context.Context, t tenant.Context, warehouseID string) ([]Location, error) {
	if _, e := s.GetWarehouse(ctx, t, warehouseID); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+locationCols+` FROM fleet_partner_storage_locations WHERE organization_id=$1 AND warehouse_id=$2 ORDER BY code`, t.OrganizationID, warehouseID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Location{}
	for rows.Next() {
		v, e := scanLocation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) UpdateLocation(ctx context.Context, t tenant.Context, warehouseID, id string, expected int64, v Location) (Location, error) {
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
		var cycle bool
		e = s.o.Pool.QueryRow(ctx, `WITH RECURSIVE descendants AS(SELECT id,parent_location_id FROM fleet_partner_storage_locations WHERE id=$1 UNION ALL SELECT l.id,l.parent_location_id FROM fleet_partner_storage_locations l JOIN descendants d ON l.parent_location_id=d.id)SELECT EXISTS(SELECT 1 FROM descendants WHERE id=$2)`, id, v.ParentLocationID).Scan(&cycle)
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
		v, x = scanLocation(tx.QueryRow(ctx, `UPDATE fleet_partner_storage_locations SET parent_location_id=NULLIF($4,''),name=COALESCE(NULLIF($5,''),name),capacity=COALESCE($6,capacity),capacity_unit=COALESCE(NULLIF($7,''),capacity_unit),restrictions=CASE WHEN $8::jsonb='{}' THEN restrictions ELSE $8 END,status=$9,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+locationCols, t.OrganizationID, id, expected, v.ParentLocationID, v.Name, v.Capacity, v.CapacityUnit, raw(v.Restrictions, "{}"), status))
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

const itemCols = `id,organization_id,warehouse_id,location_id,owner_type,owner_reference,sku,batch_reference,lot_reference,serial_reference,quantity_projection,reserved_quantity,unit,status,expiry_at,condition,version,created_at,updated_at`

func scanItem(r pgx.Row) (Item, error) {
	var v Item
	e := r.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.LocationID, &v.OwnerType, &v.OwnerReference, &v.SKU, &v.BatchReference, &v.LotReference, &v.SerialReference, &v.QuantityProjection, &v.ReservedQuantity, &v.Unit, &v.Status, &v.ExpiryAt, &v.Condition, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

const movementCols = `id,organization_id,inventory_item_id,movement_type,COALESCE(from_location_id,''),COALESCE(to_location_id,''),quantity,reason_code,actor_reference,COALESCE(fulfilment_order_id,''),COALESCE(terminal_transfer_id,''),COALESCE(return_id,''),correlation_id,projection_before,projection_after,created_at`

func scanMovement(r pgx.Row) (Movement, error) {
	var v Movement
	e := r.Scan(&v.ID, &v.OrganizationID, &v.InventoryItemID, &v.MovementType, &v.FromLocationID, &v.ToLocationID, &v.Quantity, &v.ReasonCode, &v.ActorReference, &v.FulfilmentOrderID, &v.TerminalTransferID, &v.ReturnID, &v.CorrelationID, &v.ProjectionBefore, &v.ProjectionAfter, &v.CreatedAt)
	return v, e
}
func (s *Service) capacity(ctx context.Context, q pgx.Tx, org, location string, additional float64) error {
	var cap *float64
	e := q.QueryRow(ctx, `SELECT capacity FROM fleet_partner_storage_locations WHERE organization_id=$1 AND id=$2 AND status='ACTIVE' FOR UPDATE`, org, location).Scan(&cap)
	if e != nil {
		return apperrors.NotFound("Active storage location not found.", nil)
	}
	if cap == nil {
		return nil
	}
	var used float64
	if e = q.QueryRow(ctx, `SELECT COALESCE(sum(quantity_projection),0) FROM fleet_partner_inventory_items WHERE organization_id=$1 AND location_id=$2`, org, location).Scan(&used); e != nil {
		return e
	}
	if used+additional > *cap {
		return apperrors.Conflict("Storage location capacity would be exceeded.", nil)
	}
	return nil
}
func (s *Service) recordCustodyTx(ctx context.Context, tx pgx.Tx, t tenant.Context, in ReceiptInput) error {
	if in.PackageID == "" {
		return nil
	}
	var shipment string
	e := tx.QueryRow(ctx, `SELECT shipment_id FROM fleet_partner_packages WHERE organization_id=$1 AND id=$2`, t.OrganizationID, in.PackageID).Scan(&shipment)
	if e != nil {
		return apperrors.NotFound("Package not found.", nil)
	}
	if in.ShipmentID != "" && shipment != in.ShipmentID {
		return apperrors.Conflict("Package does not belong to shipment.", nil)
	}
	_, e = tx.Exec(ctx, `INSERT INTO fleet_partner_custody_events(id,organization_id,package_id,shipment_id,from_actor_type,from_actor_reference,to_actor_type,to_actor_reference,from_location_reference,to_location_reference,condition,evidence_media_id,occurred_at,source,correlation_id)VALUES($1,$2,$3,$4,$5,$6,'WAREHOUSE',$7,'',$8,$9,$10,$11,'WAREHOUSE_RECEIPT',$12)`, ids.New("cst"), t.OrganizationID, in.PackageID, shipment, in.FromActorType, in.FromActorReference, in.WarehouseID, in.LocationID, in.Condition, in.EvidenceMediaID, s.o.Now(), in.CorrelationID)
	return e
}
func (s *Service) Receipt(ctx context.Context, t tenant.Context, in ReceiptInput) (Item, error) {
	if e := s.gate(s.o.Flags.Inventory, true); e != nil {
		return Item{}, e
	}
	if in.CorrelationID == "" || in.Quantity <= 0 || in.WarehouseID == "" || in.LocationID == "" || in.OwnerType == "" || in.OwnerReference == "" || in.SKU == "" {
		return Item{}, apperrors.BadRequest("Warehouse, location, owner, SKU, positive quantity and correlationId are required.", nil)
	}
	if e := s.requireUnrestricted(ctx, t, "WAREHOUSE", in.WarehouseID); e != nil {
		return Item{}, e
	}
	if e := s.requireUnrestricted(ctx, t, "LOCATION", in.LocationID); e != nil {
		return Item{}, e
	}
	var existingID string
	e := s.o.Pool.QueryRow(ctx, `SELECT inventory_item_id FROM fleet_partner_inventory_movements WHERE organization_id=$1 AND correlation_id=$2`, t.OrganizationID, in.CorrelationID).Scan(&existingID)
	if e == nil {
		return s.GetItem(ctx, t, existingID)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return Item{}, e
	}
	w, e := s.GetWarehouse(ctx, t, in.WarehouseID)
	if e != nil {
		return Item{}, e
	}
	if w.Status != "ACTIVE" {
		return Item{}, apperrors.Conflict("Warehouse is not active.", nil)
	}
	var item Item
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if x := s.capacity(ctx, tx, t.OrganizationID, in.LocationID, in.Quantity); x != nil {
			return x
		}
		var x error
		item, x = scanItem(tx.QueryRow(ctx, `SELECT `+itemCols+` FROM fleet_partner_inventory_items WHERE organization_id=$1 AND warehouse_id=$2 AND location_id=$3 AND owner_type=$4 AND owner_reference=$5 AND sku=$6 AND batch_reference=$7 AND lot_reference=$8 AND serial_reference=$9 FOR UPDATE`, t.OrganizationID, in.WarehouseID, in.LocationID, in.OwnerType, in.OwnerReference, in.SKU, in.BatchReference, in.LotReference, in.SerialReference))
		if errors.Is(x, pgx.ErrNoRows) {
			item = Item{ID: ids.New("inv"), OrganizationID: t.OrganizationID, WarehouseID: in.WarehouseID, LocationID: in.LocationID, OwnerType: in.OwnerType, OwnerReference: in.OwnerReference, SKU: in.SKU, BatchReference: in.BatchReference, LotReference: in.LotReference, SerialReference: in.SerialReference, Unit: in.Unit, Status: "AVAILABLE", ExpiryAt: in.ExpiryAt, Condition: in.Condition}
			item, x = scanItem(tx.QueryRow(ctx, `INSERT INTO fleet_partner_inventory_items(id,organization_id,warehouse_id,location_id,owner_type,owner_reference,sku,batch_reference,lot_reference,serial_reference,quantity_projection,unit,status,expiry_at,condition)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'AVAILABLE',$13,$14) RETURNING `+itemCols, item.ID, item.OrganizationID, item.WarehouseID, item.LocationID, item.OwnerType, item.OwnerReference, item.SKU, item.BatchReference, item.LotReference, item.SerialReference, in.Quantity, item.Unit, item.ExpiryAt, item.Condition))
		} else if x == nil {
			before := item.QuantityProjection
			item, x = scanItem(tx.QueryRow(ctx, `UPDATE fleet_partner_inventory_items SET quantity_projection=quantity_projection+$3,status='AVAILABLE',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING `+itemCols, t.OrganizationID, item.ID, in.Quantity))
			_ = before
		}
		if x != nil {
			return x
		}
		before := item.QuantityProjection - in.Quantity
		movement := Movement{ID: ids.New("imv"), OrganizationID: t.OrganizationID, InventoryItemID: item.ID, MovementType: "RECEIPT", ToLocationID: in.LocationID, Quantity: in.Quantity, ReasonCode: "WAREHOUSE_RECEIPT", ActorReference: t.ActorID, CorrelationID: in.CorrelationID, ProjectionBefore: before, ProjectionAfter: item.QuantityProjection}
		if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,to_location_id,quantity,reason_code,actor_reference,correlation_id,projection_before,projection_after)VALUES($1,$2,$3,'RECEIPT',$4,$5,$6,$7,$8,$9,$10)`, movement.ID, movement.OrganizationID, movement.InventoryItemID, movement.ToLocationID, movement.Quantity, movement.ReasonCode, movement.ActorReference, movement.CorrelationID, movement.ProjectionBefore, movement.ProjectionAfter); x != nil {
			return x
		}
		if x = s.recordCustodyTx(ctx, tx, t, in); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "inventory.received.v1", "inventory_item", item.ID, item.Version, map[string]any{"inventoryItemId": item.ID, "movementId": movement.ID, "quantity": movement.Quantity, "unit": item.Unit})
	})
	return item, e
}
func (s *Service) GetItem(ctx context.Context, t tenant.Context, id string) (Item, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return Item{}, e
	}
	v, e := scanItem(s.o.Pool.QueryRow(ctx, `SELECT i.`+strings.ReplaceAll(itemCols, ",", ",i.")+` FROM fleet_partner_inventory_items i JOIN fleet_partner_warehouses w ON w.id=i.warehouse_id WHERE i.organization_id=$1 AND i.id=$2`, t.OrganizationID, id))
	if errors.Is(e, pgx.ErrNoRows) || e == nil && !t.BranchPermitted(warehouseBranch(ctx, s.o.Pool, v.WarehouseID)) {
		e = apperrors.NotFound("Inventory item not found.", nil)
	}
	return v, e
}
func warehouseBranch(ctx context.Context, p *pgxpool.Pool, id string) string {
	var b string
	_ = p.QueryRow(ctx, `SELECT branch_id FROM fleet_partner_warehouses WHERE id=$1`, id).Scan(&b)
	return b
}
func (s *Service) ListItems(ctx context.Context, t tenant.Context) ([]Item, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+itemCols+` FROM fleet_partner_inventory_items i WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=i.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC`, t.OrganizationID, t.PermittedBranchIDs)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		v, e := scanItem(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) Movements(ctx context.Context, t tenant.Context, id string) ([]Movement, error) {
	if _, e := s.GetItem(ctx, t, id); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+movementCols+` FROM fleet_partner_inventory_movements WHERE organization_id=$1 AND inventory_item_id=$2 ORDER BY created_at,id`, t.OrganizationID, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Movement{}
	for rows.Next() {
		v, e := scanMovement(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) Move(ctx context.Context, t tenant.Context, id string, in MovementInput) (Movement, error) {
	if e := s.gate(s.o.Flags.Inventory, true); e != nil {
		return Movement{}, e
	}
	in.MovementType = strings.ToUpper(in.MovementType)
	if in.CorrelationID == "" || in.Quantity <= 0 || in.ReasonCode == "" || in.ExpectedVersion <= 0 {
		return Movement{}, apperrors.BadRequest("Movement type, positive quantity, reason, correlationId and expectedVersion are required.", nil)
	}
	v, e := scanMovement(s.o.Pool.QueryRow(ctx, `SELECT `+movementCols+` FROM fleet_partner_inventory_movements WHERE organization_id=$1 AND correlation_id=$2`, t.OrganizationID, in.CorrelationID))
	if e == nil {
		return v, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return v, e
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		item, x := scanItem(tx.QueryRow(ctx, `SELECT `+itemCols+` FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id))
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
		_, x = tx.Exec(ctx, `UPDATE fleet_partner_inventory_items SET location_id=$3,quantity_projection=$4,reserved_quantity=LEAST(reserved_quantity,$4),status=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id, location, after, status)
		if x != nil {
			return x
		}
		v = Movement{ID: ids.New("imv"), OrganizationID: t.OrganizationID, InventoryItemID: id, MovementType: in.MovementType, FromLocationID: item.LocationID, ToLocationID: in.ToLocationID, Quantity: in.Quantity, ReasonCode: in.ReasonCode, ActorReference: t.ActorID, FulfilmentOrderID: in.FulfilmentOrderID, TerminalTransferID: in.TerminalTransferID, ReturnID: in.ReturnID, CorrelationID: in.CorrelationID, ProjectionBefore: before, ProjectionAfter: after}
		x = tx.QueryRow(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,to_location_id,quantity,reason_code,actor_reference,fulfilment_order_id,terminal_transfer_id,return_id,correlation_id,projection_before,projection_after)VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),$13,$14,$15) RETURNING created_at`, v.ID, v.OrganizationID, v.InventoryItemID, v.MovementType, v.FromLocationID, v.ToLocationID, v.Quantity, v.ReasonCode, v.ActorReference, v.FulfilmentOrderID, v.TerminalTransferID, v.ReturnID, v.CorrelationID, v.ProjectionBefore, v.ProjectionAfter).Scan(&v.CreatedAt)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "inventory.moved.v1", "inventory_item", id, item.Version+1, map[string]any{"inventoryItemId": id, "movementId": v.ID, "movementType": v.MovementType, "quantity": v.Quantity})
	})
	return v, e
}
func raw(v json.RawMessage, d string) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage(d)
	}
	return v
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
