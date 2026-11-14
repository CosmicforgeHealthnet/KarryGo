// Package repositories is the PostgreSQL persistence of Section 21 warehouse
// operations. Methods take the Querier to run on (nil means the pool) so a
// use case composes them into one transaction. A missing row is
// pgx.ErrNoRows.
package repositories

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

type Repository struct{ db database.Querier }

func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.db
	}
	return q
}

func (r *Repository) exists(ctx context.Context, q database.Querier, sql string, args ...any) bool {
	var ok bool
	_ = r.q(q).QueryRow(ctx, sql, args...).Scan(&ok)
	return ok
}

// ---------------------------------------------------------------------------
// Warehouses and storage locations
// ---------------------------------------------------------------------------

const warehouseCols = `id,organization_id,branch_id,code,name,site_reference,address_reference,timezone,operating_hours,status,version,created_at,updated_at,archived_at`

func scanWarehouse(row pgx.Row) (models.Warehouse, error) {
	var v models.Warehouse
	e := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.Code, &v.Name, &v.SiteReference, &v.AddressReference, &v.Timezone, &v.OperatingHours, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt)
	return v, e
}

func (r *Repository) InsertWarehouse(ctx context.Context, q database.Querier, v models.Warehouse) (models.Warehouse, error) {
	return scanWarehouse(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_warehouses(id,organization_id,branch_id,code,name,site_reference,address_reference,timezone,operating_hours)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+warehouseCols, v.ID, v.OrganizationID, v.BranchID, v.Code, v.Name, v.SiteReference, v.AddressReference, v.Timezone, v.OperatingHours))
}

func (r *Repository) GetWarehouse(ctx context.Context, org, id string) (models.Warehouse, error) {
	return scanWarehouse(r.db.QueryRow(ctx, `SELECT `+warehouseCols+` FROM fleet_partner_warehouses WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) ListWarehouses(ctx context.Context, org string, branches []string) ([]models.Warehouse, error) {
	rows, e := r.db.Query(ctx, `SELECT `+warehouseCols+` FROM fleet_partner_warehouses WHERE organization_id=$1 AND(COALESCE(cardinality($2::text[]),0)=0 OR branch_id=ANY($2::text[])) ORDER BY created_at DESC`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanWarehouse)
}

func (r *Repository) WarehouseHasStock(ctx context.Context, id string) bool {
	return r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_inventory_items WHERE warehouse_id=$1)`, id)
}

func (r *Repository) UpdateWarehouse(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Warehouse, status string) (models.Warehouse, error) {
	return scanWarehouse(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_warehouses SET name=COALESCE(NULLIF($4,''),name),site_reference=COALESCE(NULLIF($5,''),site_reference),address_reference=COALESCE(NULLIF($6,''),address_reference),timezone=COALESCE(NULLIF($7,''),timezone),operating_hours=CASE WHEN $8::jsonb='{}' THEN operating_hours ELSE $8 END,status=$9,archived_at=CASE WHEN $9='ARCHIVED' THEN now() ELSE archived_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+warehouseCols, org, id, expected, v.Name, v.SiteReference, v.AddressReference, v.Timezone, models.Raw(v.OperatingHours, "{}"), status))
}

// WarehouseBranch is the branch of a warehouse, or "" when unknown.
func (r *Repository) WarehouseBranch(ctx context.Context, id string) string {
	var b string
	_ = r.db.QueryRow(ctx, `SELECT branch_id FROM fleet_partner_warehouses WHERE id=$1`, id).Scan(&b)
	return b
}

func (r *Repository) OrganizationsWithActiveWarehouses(ctx context.Context) ([]string, error) {
	rows, e := r.db.Query(ctx, `SELECT organization_id FROM fleet_partner_warehouses WHERE status='ACTIVE' GROUP BY organization_id`)
	if e != nil {
		return nil, e
	}
	return collect(rows, func(row pgx.Row) (string, error) {
		var org string
		return org, row.Scan(&org)
	})
}

const locationCols = `id,organization_id,warehouse_id,COALESCE(parent_location_id,''),code,name,location_type,capacity,capacity_unit,restrictions,status,version,created_at,updated_at`

func scanLocation(row pgx.Row) (models.Location, error) {
	var v models.Location
	e := row.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.ParentLocationID, &v.Code, &v.Name, &v.LocationType, &v.Capacity, &v.CapacityUnit, &v.Restrictions, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) GetLocation(ctx context.Context, org, warehouseID, id string) (models.Location, error) {
	return scanLocation(r.db.QueryRow(ctx, `SELECT `+locationCols+` FROM fleet_partner_storage_locations WHERE organization_id=$1 AND warehouse_id=$2 AND id=$3`, org, warehouseID, id))
}

func (r *Repository) InsertLocation(ctx context.Context, q database.Querier, v models.Location) (models.Location, error) {
	return scanLocation(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_storage_locations(id,organization_id,warehouse_id,parent_location_id,code,name,location_type,capacity,capacity_unit,restrictions)VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10) RETURNING `+locationCols, v.ID, v.OrganizationID, v.WarehouseID, v.ParentLocationID, v.Code, v.Name, v.LocationType, v.Capacity, v.CapacityUnit, v.Restrictions))
}

func (r *Repository) ListLocations(ctx context.Context, org, warehouseID string) ([]models.Location, error) {
	rows, e := r.db.Query(ctx, `SELECT `+locationCols+` FROM fleet_partner_storage_locations WHERE organization_id=$1 AND warehouse_id=$2 ORDER BY code`, org, warehouseID)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanLocation)
}

// LocationParentCycle reports whether parent is id or one of its descendants.
func (r *Repository) LocationParentCycle(ctx context.Context, id, parent string) (bool, error) {
	var cycle bool
	e := r.db.QueryRow(ctx, `WITH RECURSIVE descendants AS(SELECT id,parent_location_id FROM fleet_partner_storage_locations WHERE id=$1 UNION ALL SELECT l.id,l.parent_location_id FROM fleet_partner_storage_locations l JOIN descendants d ON l.parent_location_id=d.id)SELECT EXISTS(SELECT 1 FROM descendants WHERE id=$2)`, id, parent).Scan(&cycle)
	return cycle, e
}

func (r *Repository) UpdateLocation(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Location, status string) (models.Location, error) {
	return scanLocation(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_storage_locations SET parent_location_id=NULLIF($4,''),name=COALESCE(NULLIF($5,''),name),capacity=COALESCE($6,capacity),capacity_unit=COALESCE(NULLIF($7,''),capacity_unit),restrictions=CASE WHEN $8::jsonb='{}' THEN restrictions ELSE $8 END,status=$9,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+locationCols, org, id, expected, v.ParentLocationID, v.Name, v.Capacity, v.CapacityUnit, models.Raw(v.Restrictions, "{}"), status))
}

// LockLocationCapacity locks an active location and returns its capacity
// (nil means unbounded).
func (r *Repository) LockLocationCapacity(ctx context.Context, q database.Querier, org, location string) (*float64, error) {
	var capacity *float64
	e := r.q(q).QueryRow(ctx, `SELECT capacity FROM fleet_partner_storage_locations WHERE organization_id=$1 AND id=$2 AND status='ACTIVE' FOR UPDATE`, org, location).Scan(&capacity)
	return capacity, e
}

func (r *Repository) LocationUsedQuantity(ctx context.Context, q database.Querier, org, location string) (float64, error) {
	var used float64
	e := r.q(q).QueryRow(ctx, `SELECT COALESCE(sum(quantity_projection),0) FROM fleet_partner_inventory_items WHERE organization_id=$1 AND location_id=$2`, org, location).Scan(&used)
	return used, e
}

// ---------------------------------------------------------------------------
// Packages, manifests and custody (read and append only)
// ---------------------------------------------------------------------------

func (r *Repository) PackageShipment(ctx context.Context, q database.Querier, org, packageID string) (string, error) {
	var shipment string
	e := r.q(q).QueryRow(ctx, `SELECT shipment_id FROM fleet_partner_packages WHERE organization_id=$1 AND id=$2`, org, packageID).Scan(&shipment)
	return shipment, e
}

func (r *Repository) PackageExists(ctx context.Context, q database.Querier, org, packageID string) bool {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_packages WHERE organization_id=$1 AND id=$2)`, org, packageID)
}

func (r *Repository) ManifestExists(ctx context.Context, org, id string) bool {
	return r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_manifests WHERE organization_id=$1 AND id=$2)`, org, id)
}

func (r *Repository) InsertReceiptCustody(ctx context.Context, q database.Querier, org string, in models.ReceiptInput, shipment string, at time.Time) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_custody_events(id,organization_id,package_id,shipment_id,from_actor_type,from_actor_reference,to_actor_type,to_actor_reference,from_location_reference,to_location_reference,condition,evidence_media_id,occurred_at,source,correlation_id)VALUES($1,$2,$3,$4,$5,$6,'WAREHOUSE',$7,'',$8,$9,$10,$11,'WAREHOUSE_RECEIPT',$12)`, ids.New("cst"), org, in.PackageID, shipment, in.FromActorType, in.FromActorReference, in.WarehouseID, in.LocationID, in.Condition, in.EvidenceMediaID, at, in.CorrelationID)
	return e
}

func (r *Repository) InsertReturnCustody(ctx context.Context, q database.Querier, org string, v models.Return, shipment string) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_custody_events(id,organization_id,package_id,shipment_id,from_actor_type,from_actor_reference,to_actor_type,to_actor_reference,from_location_reference,to_location_reference,condition,occurred_at,source,correlation_id)VALUES($1,$2,$3,$4,'RETURN_SOURCE',$5,'WAREHOUSE',$6,'',$6,$7,now(),'WAREHOUSE_RETURN',$8)`, ids.New("cst"), org, v.PackageID, shipment, v.SourceOrderID, v.WarehouseID, v.Condition, v.ID+":receive")
	return e
}

func collect[T any](rows pgx.Rows, scan func(pgx.Row) (T, error)) ([]T, error) {
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Inventory items and movements
// ---------------------------------------------------------------------------

const itemCols = `id,organization_id,warehouse_id,location_id,owner_type,owner_reference,sku,batch_reference,lot_reference,serial_reference,quantity_projection,reserved_quantity,unit,status,expiry_at,condition,version,created_at,updated_at`

func scanItem(row pgx.Row) (models.Item, error) {
	var v models.Item
	e := row.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.LocationID, &v.OwnerType, &v.OwnerReference, &v.SKU, &v.BatchReference, &v.LotReference, &v.SerialReference, &v.QuantityProjection, &v.ReservedQuantity, &v.Unit, &v.Status, &v.ExpiryAt, &v.Condition, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

const movementCols = `id,organization_id,inventory_item_id,movement_type,COALESCE(from_location_id,''),COALESCE(to_location_id,''),quantity,reason_code,actor_reference,COALESCE(fulfilment_order_id,''),COALESCE(terminal_transfer_id,''),COALESCE(return_id,''),correlation_id,projection_before,projection_after,created_at`

func scanMovement(row pgx.Row) (models.Movement, error) {
	var v models.Movement
	e := row.Scan(&v.ID, &v.OrganizationID, &v.InventoryItemID, &v.MovementType, &v.FromLocationID, &v.ToLocationID, &v.Quantity, &v.ReasonCode, &v.ActorReference, &v.FulfilmentOrderID, &v.TerminalTransferID, &v.ReturnID, &v.CorrelationID, &v.ProjectionBefore, &v.ProjectionAfter, &v.CreatedAt)
	return v, e
}

// MovementItemByCorrelation returns the item a correlated movement touched.
func (r *Repository) MovementItemByCorrelation(ctx context.Context, org, correlation string) (string, error) {
	var id string
	e := r.db.QueryRow(ctx, `SELECT inventory_item_id FROM fleet_partner_inventory_movements WHERE organization_id=$1 AND correlation_id=$2`, org, correlation).Scan(&id)
	return id, e
}

func (r *Repository) GetMovementByCorrelation(ctx context.Context, org, correlation string) (models.Movement, error) {
	return scanMovement(r.db.QueryRow(ctx, `SELECT `+movementCols+` FROM fleet_partner_inventory_movements WHERE organization_id=$1 AND correlation_id=$2`, org, correlation))
}

// LockReceiptItem locks the stock line a receipt adds to.
func (r *Repository) LockReceiptItem(ctx context.Context, q database.Querier, org string, in models.ReceiptInput) (models.Item, error) {
	return scanItem(r.q(q).QueryRow(ctx, `SELECT `+itemCols+` FROM fleet_partner_inventory_items WHERE organization_id=$1 AND warehouse_id=$2 AND location_id=$3 AND owner_type=$4 AND owner_reference=$5 AND sku=$6 AND batch_reference=$7 AND lot_reference=$8 AND serial_reference=$9 FOR UPDATE`, org, in.WarehouseID, in.LocationID, in.OwnerType, in.OwnerReference, in.SKU, in.BatchReference, in.LotReference, in.SerialReference))
}

func (r *Repository) InsertItem(ctx context.Context, q database.Querier, item models.Item, quantity float64) (models.Item, error) {
	return scanItem(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_inventory_items(id,organization_id,warehouse_id,location_id,owner_type,owner_reference,sku,batch_reference,lot_reference,serial_reference,quantity_projection,unit,status,expiry_at,condition)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'AVAILABLE',$13,$14) RETURNING `+itemCols, item.ID, item.OrganizationID, item.WarehouseID, item.LocationID, item.OwnerType, item.OwnerReference, item.SKU, item.BatchReference, item.LotReference, item.SerialReference, quantity, item.Unit, item.ExpiryAt, item.Condition))
}

func (r *Repository) AddItemQuantity(ctx context.Context, q database.Querier, org, id string, quantity float64) (models.Item, error) {
	return scanItem(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_inventory_items SET quantity_projection=quantity_projection+$3,status='AVAILABLE',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING `+itemCols, org, id, quantity))
}

func (r *Repository) InsertReceiptMovement(ctx context.Context, q database.Querier, m models.Movement) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,to_location_id,quantity,reason_code,actor_reference,correlation_id,projection_before,projection_after)VALUES($1,$2,$3,'RECEIPT',$4,$5,$6,$7,$8,$9,$10)`, m.ID, m.OrganizationID, m.InventoryItemID, m.ToLocationID, m.Quantity, m.ReasonCode, m.ActorReference, m.CorrelationID, m.ProjectionBefore, m.ProjectionAfter)
	return e
}

func (r *Repository) GetItem(ctx context.Context, org, id string) (models.Item, error) {
	return scanItem(r.db.QueryRow(ctx, `SELECT i.`+strings.ReplaceAll(itemCols, ",", ",i.")+` FROM fleet_partner_inventory_items i JOIN fleet_partner_warehouses w ON w.id=i.warehouse_id WHERE i.organization_id=$1 AND i.id=$2`, org, id))
}

func (r *Repository) ListItems(ctx context.Context, org string, branches []string) ([]models.Item, error) {
	rows, e := r.db.Query(ctx, `SELECT `+itemCols+` FROM fleet_partner_inventory_items i WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=i.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanItem)
}

func (r *Repository) ListMovements(ctx context.Context, org, itemID string) ([]models.Movement, error) {
	rows, e := r.db.Query(ctx, `SELECT `+movementCols+` FROM fleet_partner_inventory_movements WHERE organization_id=$1 AND inventory_item_id=$2 ORDER BY created_at,id`, org, itemID)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanMovement)
}

func (r *Repository) LockItem(ctx context.Context, q database.Querier, org, id string) (models.Item, error) {
	return scanItem(r.q(q).QueryRow(ctx, `SELECT `+itemCols+` FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id))
}

func (r *Repository) ApplyItemMove(ctx context.Context, q database.Querier, org, id, location string, after float64, status string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_inventory_items SET location_id=$3,quantity_projection=$4,reserved_quantity=LEAST(reserved_quantity,$4),status=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, id, location, after, status)
	return e
}

func (r *Repository) InsertMovement(ctx context.Context, q database.Querier, v *models.Movement) error {
	return r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,to_location_id,quantity,reason_code,actor_reference,fulfilment_order_id,terminal_transfer_id,return_id,correlation_id,projection_before,projection_after)VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),$13,$14,$15) RETURNING created_at`, v.ID, v.OrganizationID, v.InventoryItemID, v.MovementType, v.FromLocationID, v.ToLocationID, v.Quantity, v.ReasonCode, v.ActorReference, v.FulfilmentOrderID, v.TerminalTransferID, v.ReturnID, v.CorrelationID, v.ProjectionBefore, v.ProjectionAfter).Scan(&v.CreatedAt)
}

// ItemWarehouse is the warehouse holding an inventory item.
func (r *Repository) ItemWarehouse(ctx context.Context, q database.Querier, org, itemID string) (string, error) {
	var wh string
	e := r.q(q).QueryRow(ctx, `SELECT warehouse_id FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2`, org, itemID).Scan(&wh)
	return wh, e
}

// LockItemStock locks an item and returns quantity, reserved quantity, status
// and expiry.
func (r *Repository) LockItemStock(ctx context.Context, q database.Querier, org, itemID string) (qty, reserved float64, status string, expiry *time.Time, e error) {
	e = r.q(q).QueryRow(ctx, `SELECT quantity_projection,reserved_quantity,status,expiry_at FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, itemID).Scan(&qty, &reserved, &status, &expiry)
	return
}

func (r *Repository) LockItemQuantity(ctx context.Context, q database.Querier, org, itemID string) (float64, error) {
	var qty float64
	e := r.q(q).QueryRow(ctx, `SELECT quantity_projection FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, itemID).Scan(&qty)
	return qty, e
}

// LockItemQuantityAndLocation locks an item and returns its quantity,
// reserved quantity and location.
func (r *Repository) LockItemQuantityAndLocation(ctx context.Context, q database.Querier, org, itemID string) (qty, reserved float64, location string, e error) {
	e = r.q(q).QueryRow(ctx, `SELECT quantity_projection,reserved_quantity,location_id FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, itemID).Scan(&qty, &reserved, &location)
	return
}

// SetItemStatus sets an item's workflow status (PICKING, PACKED).
func (r *Repository) SetItemStatus(ctx context.Context, q database.Querier, itemID, status string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_inventory_items SET status=$2,version=version+1,updated_at=now() WHERE id=$1`, itemID, status)
	return e
}

func (r *Repository) SetItemQuantityAndStatus(ctx context.Context, q database.Querier, org, itemID string, quantity float64, status string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_inventory_items SET quantity_projection=$3,status=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, itemID, quantity, status)
	return e
}

// ---------------------------------------------------------------------------
// Fulfilment
// ---------------------------------------------------------------------------

const orderCols = `id,organization_id,warehouse_id,customer_reference,source_order_id,priority,status,requested_dispatch_at,assigned_team_reference,packing_requirements,proof_policy_reference,shipment_reference,manifest_reference,version,created_at,updated_at,completed_at`

func scanOrder(row pgx.Row) (models.FulfilmentOrder, error) {
	var v models.FulfilmentOrder
	e := row.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.CustomerReference, &v.SourceOrderID, &v.Priority, &v.Status, &v.RequestedDispatchAt, &v.AssignedTeamReference, &v.PackingRequirements, &v.ProofPolicyReference, &v.ShipmentReference, &v.ManifestReference, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.CompletedAt)
	return v, e
}

func (r *Repository) LoadLines(ctx context.Context, order *models.FulfilmentOrder) error {
	rows, e := r.db.Query(ctx, `SELECT id,organization_id,fulfilment_order_id,inventory_item_id,requested_quantity,picked_quantity,package_reference,version,created_at,updated_at FROM fleet_partner_fulfilment_lines WHERE organization_id=$1 AND fulfilment_order_id=$2 ORDER BY id`, order.OrganizationID, order.ID)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var v models.FulfilmentLine
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.FulfilmentOrderID, &v.InventoryItemID, &v.RequestedQuantity, &v.PickedQuantity, &v.PackageReference, &v.Version, &v.CreatedAt, &v.UpdatedAt); e != nil {
			return e
		}
		order.Lines = append(order.Lines, v)
	}
	return rows.Err()
}

func (r *Repository) InsertFulfilmentOrder(ctx context.Context, q database.Querier, v models.FulfilmentOrder) (models.FulfilmentOrder, error) {
	return scanOrder(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_fulfilment_orders(id,organization_id,warehouse_id,customer_reference,source_order_id,priority,requested_dispatch_at,assigned_team_reference,packing_requirements,proof_policy_reference)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+orderCols, v.ID, v.OrganizationID, v.WarehouseID, v.CustomerReference, v.SourceOrderID, v.Priority, v.RequestedDispatchAt, v.AssignedTeamReference, v.PackingRequirements, v.ProofPolicyReference))
}

func (r *Repository) InsertFulfilmentLine(ctx context.Context, q database.Querier, l *models.FulfilmentLine) error {
	return r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_fulfilment_lines(id,organization_id,fulfilment_order_id,inventory_item_id,requested_quantity)VALUES($1,$2,$3,$4,$5) RETURNING version,created_at,updated_at`, l.ID, l.OrganizationID, l.FulfilmentOrderID, l.InventoryItemID, l.RequestedQuantity).Scan(&l.Version, &l.CreatedAt, &l.UpdatedAt)
}

func (r *Repository) GetFulfilmentOrder(ctx context.Context, org, id string) (models.FulfilmentOrder, error) {
	return scanOrder(r.db.QueryRow(ctx, `SELECT `+orderCols+` FROM fleet_partner_fulfilment_orders WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) ListFulfilmentOrders(ctx context.Context, org string, branches []string) ([]models.FulfilmentOrder, error) {
	rows, e := r.db.Query(ctx, `SELECT `+orderCols+` FROM fleet_partner_fulfilment_orders f WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=f.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanOrder)
}

func (r *Repository) UpdateFulfilmentOrder(ctx context.Context, q database.Querier, org, id string, expected int64, v models.FulfilmentOrder) (models.FulfilmentOrder, error) {
	return scanOrder(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_fulfilment_orders SET priority=COALESCE(NULLIF($4,''),priority),requested_dispatch_at=COALESCE($5,requested_dispatch_at),assigned_team_reference=COALESCE(NULLIF($6,''),assigned_team_reference),packing_requirements=CASE WHEN $7::jsonb='{}' THEN packing_requirements ELSE $7 END,proof_policy_reference=COALESCE(NULLIF($8,''),proof_policy_reference),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+orderCols, org, id, expected, v.Priority, v.RequestedDispatchAt, v.AssignedTeamReference, models.Raw(v.PackingRequirements, "{}"), v.ProofPolicyReference))
}

func (r *Repository) LockFulfilmentOrder(ctx context.Context, q database.Querier, org, id string) (models.FulfilmentOrderState, error) {
	var v models.FulfilmentOrderState
	e := r.q(q).QueryRow(ctx, `SELECT status,version,warehouse_id,proof_policy_reference,shipment_reference FROM fleet_partner_fulfilment_orders WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id).Scan(&v.Status, &v.Version, &v.WarehouseID, &v.ProofPolicyReference, &v.ShipmentReference)
	return v, e
}

func (r *Repository) LockFulfilmentLines(ctx context.Context, q database.Querier, org, id string) ([]models.FulfilmentLineState, error) {
	rows, e := r.q(q).Query(ctx, `SELECT id,inventory_item_id,requested_quantity,picked_quantity,package_reference FROM fleet_partner_fulfilment_lines WHERE organization_id=$1 AND fulfilment_order_id=$2 ORDER BY id FOR UPDATE`, org, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.FulfilmentLineState
	for rows.Next() {
		var l models.FulfilmentLineState
		if e = rows.Scan(&l.ID, &l.InventoryItemID, &l.Requested, &l.Picked, &l.PackageReference); e != nil {
			return nil, e
		}
		out = append(out, l)
	}
	return out, nil
}

func (r *Repository) ReserveItem(ctx context.Context, q database.Querier, org, itemID string, quantity float64) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_inventory_items SET reserved_quantity=reserved_quantity+$3,status='RESERVED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, itemID, quantity)
	return e
}

func (r *Repository) InsertReservation(ctx context.Context, q database.Querier, org, itemID, orderID string, quantity float64, expiresAt *time.Time) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_inventory_reservations(id,organization_id,inventory_item_id,fulfilment_order_id,quantity,expires_at)VALUES($1,$2,$3,$4,$5,COALESCE($6,now()+interval '24 hours'))`, ids.New("res"), org, itemID, orderID, quantity, expiresAt)
	return e
}

func (r *Repository) MarkLinePicked(ctx context.Context, q database.Querier, lineID string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_fulfilment_lines SET picked_quantity=requested_quantity,version=version+1,updated_at=now() WHERE id=$1`, lineID)
	return e
}

// SetReservationsStatus moves an order's active reservations of one item to
// PICKED or RELEASED.
func (r *Repository) SetReservationsStatus(ctx context.Context, q database.Querier, org, itemID, orderID, status string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_inventory_reservations SET status=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND inventory_item_id=$2 AND fulfilment_order_id=$3 AND status='ACTIVE'`, org, itemID, orderID, status)
	return e
}

func (r *Repository) InsertPickMovement(ctx context.Context, q database.Querier, org, itemID, orderID, actor string, quantity float64) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,quantity,reason_code,actor_reference,fulfilment_order_id,correlation_id,projection_before,projection_after)SELECT $1,$2,id,'PICK',location_id,$3,'FULFILMENT_PICK',$4,$5,$6,quantity_projection,quantity_projection FROM fleet_partner_inventory_items WHERE id=$7`, ids.New("imv"), org, quantity, actor, orderID, orderID+":pick:"+itemID, itemID)
	return e
}

func (r *Repository) SetLinePackage(ctx context.Context, q database.Querier, lineID, packageRef string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_fulfilment_lines SET package_reference=$2,version=version+1,updated_at=now() WHERE id=$1`, lineID, packageRef)
	return e
}

func (r *Repository) DispatchItem(ctx context.Context, q database.Querier, org, itemID string, after, requested float64, status string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_inventory_items SET quantity_projection=$3,reserved_quantity=reserved_quantity-$4,status=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, itemID, after, requested, status)
	return e
}

func (r *Repository) InsertDispatchMovement(ctx context.Context, q database.Querier, org, itemID, location, orderID, actor string, quantity, before, after float64) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,quantity,reason_code,actor_reference,fulfilment_order_id,correlation_id,projection_before,projection_after)VALUES($1,$2,$3,'DISPATCH',$4,$5,'FULFILMENT_DISPATCH',$6,$7,$8,$9,$10)`, ids.New("imv"), org, itemID, location, quantity, actor, orderID, orderID+":dispatch:"+itemID, before, after)
	return e
}

func (r *Repository) ReleaseItemReservation(ctx context.Context, q database.Querier, org, itemID string, quantity float64) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_inventory_items SET reserved_quantity=GREATEST(0,reserved_quantity-$3),status=CASE WHEN reserved_quantity-$3<=0 THEN 'AVAILABLE' ELSE status END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, itemID, quantity)
	return e
}

func (r *Repository) TransitionFulfilmentOrder(ctx context.Context, q database.Querier, org, id string, expected int64, next, shipment, manifest string) (models.FulfilmentOrder, error) {
	return scanOrder(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_fulfilment_orders SET status=$4,shipment_reference=COALESCE(NULLIF($5,''),shipment_reference),manifest_reference=COALESCE(NULLIF($6,''),manifest_reference),version=version+1,updated_at=now(),completed_at=CASE WHEN $4 IN('DISPATCHED','COMPLETED','CANCELLED') THEN now() ELSE completed_at END WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+orderCols, org, id, expected, next, shipment, manifest))
}

// ---------------------------------------------------------------------------
// Sweep
// ---------------------------------------------------------------------------

func (r *Repository) LockExpiredReservations(ctx context.Context, q database.Querier, org string) ([]models.ExpiredReservation, error) {
	rows, e := r.q(q).Query(ctx, `SELECT id,inventory_item_id,quantity FROM fleet_partner_inventory_reservations WHERE organization_id=$1 AND status='ACTIVE' AND expires_at<=now() FOR UPDATE SKIP LOCKED`, org)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.ExpiredReservation
	for rows.Next() {
		var v models.ExpiredReservation
		if e = rows.Scan(&v.ID, &v.InventoryItemID, &v.Quantity); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}

func (r *Repository) ExpireReservation(ctx context.Context, q database.Querier, v models.ExpiredReservation) error {
	if _, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_inventory_reservations SET status='EXPIRED',version=version+1,updated_at=now() WHERE id=$1`, v.ID); e != nil {
		return e
	}
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_inventory_items SET reserved_quantity=GREATEST(0,reserved_quantity-$2),status=CASE WHEN reserved_quantity-$2<=0 AND status='RESERVED' THEN 'AVAILABLE' ELSE status END,version=version+1,updated_at=now() WHERE id=$1`, v.InventoryItemID, v.Quantity)
	return e
}

// SweepStaleOperations applies the time-based state transitions of one
// organization: expired stock, dock no-shows, stale yard visits, arrived
// transfers awaiting reconciliation and received returns awaiting inspection.
func (r *Repository) SweepStaleOperations(ctx context.Context, q database.Querier, org string) error {
	for _, sql := range []string{
		`UPDATE fleet_partner_inventory_items SET status='EXPIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND expiry_at<=now() AND status NOT IN('EXPIRED','DISPATCHED')`,
		`UPDATE fleet_partner_dock_appointments SET status='NO_SHOW',version=version+1,updated_at=now() WHERE organization_id=$1 AND status IN('REQUESTED','CONFIRMED') AND window_end<now()`,
		`UPDATE fleet_partner_yard_visits SET status='SECURITY_HOLD',security_status='HOLD',version=version+1,updated_at=now() WHERE organization_id=$1 AND status IN('GATE_CHECK_IN','IN_YARD','AT_BAY') AND updated_at<now()-interval '12 hours'`,
		`UPDATE fleet_partner_terminal_transfers SET status='RECONCILING',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='ARRIVED' AND arrived_at<now()-interval '1 hour'`,
		`UPDATE fleet_partner_returns SET status='INSPECTING',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='RECEIVED' AND updated_at<now()-interval '24 hours'`,
	} {
		if _, e := r.q(q).Exec(ctx, sql, org); e != nil {
			return e
		}
	}
	return nil
}

// InsertReservationExpiryMovement appends the audit movement of an expired
// reservation; a duplicate is ignored.
func (r *Repository) InsertReservationExpiryMovement(ctx context.Context, q database.Querier, org string, v models.ExpiredReservation) {
	_, _ = r.q(q).Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,quantity,reason_code,actor_reference,correlation_id,projection_before,projection_after)SELECT $1,$2,id,'ADJUSTMENT',location_id,$3,'RESERVATION_EXPIRED','SYSTEM',$4,quantity_projection,quantity_projection FROM fleet_partner_inventory_items WHERE id=$5 ON CONFLICT DO NOTHING`, ids.New("imv"), org, v.Quantity, "reservation-expired:"+v.ID, v.InventoryItemID)
}
