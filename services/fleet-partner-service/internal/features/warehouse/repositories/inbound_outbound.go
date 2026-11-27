package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ---------------------------------------------------------------------------
// Section 51 receipts with lines
// ---------------------------------------------------------------------------

const receiptCols = `id,organization_id,warehouse_id,receipt_number,source_type,source_reference,receiving_location_id,COALESCE(quarantine_location_id,''),COALESCE(yard_visit_id,''),status,expected_at,arrived_at,received_at,completed_at,version,created_at,updated_at`

func scanReceipt(row pgx.Row) (models.WarehouseReceipt, error) {
	var v models.WarehouseReceipt
	e := row.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.ReceiptNumber, &v.SourceType, &v.SourceReference, &v.ReceivingLocationID, &v.QuarantineLocationID, &v.YardVisitID, &v.Status, &v.ExpectedAt, &v.ArrivedAt, &v.ReceivedAt, &v.CompletedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	v.Lines = []models.ReceiptLine{}
	return v, e
}

const receiptLineCols = `id,receipt_id,line_number,sku,owner_type,owner_reference,lot_reference,serial_reference,unit,expected_quantity,received_quantity,damaged_quantity,quarantined_quantity,accepted_quantity,COALESCE(inventory_item_id,''),COALESCE(quarantine_item_id,''),COALESCE(putaway_location_id,''),status,discrepancy_status,discrepancy_resolution,discrepancy_note,received_at,version`

func scanReceiptLine(row pgx.Row) (models.ReceiptLine, error) {
	var v models.ReceiptLine
	e := row.Scan(&v.ID, &v.ReceiptID, &v.LineNumber, &v.SKU, &v.OwnerType, &v.OwnerReference, &v.LotReference, &v.SerialReference, &v.Unit, &v.ExpectedQuantity, &v.ReceivedQuantity, &v.DamagedQuantity, &v.QuarantinedQuantity, &v.AcceptedQuantity, &v.InventoryItemID, &v.QuarantineItemID, &v.PutawayLocationID, &v.Status, &v.DiscrepancyStatus, &v.DiscrepancyResolution, &v.DiscrepancyNote, &v.ReceivedAt, &v.Version)
	return v, e
}

// LocationRef reads a storage location's identity within an organization.
func (r *Repository) LocationRef(ctx context.Context, q database.Querier, org, id string) (models.LocationRef, error) {
	var v models.LocationRef
	e := r.q(q).QueryRow(ctx, `SELECT id,warehouse_id,code,location_type,status FROM fleet_partner_storage_locations WHERE organization_id=$1 AND id=$2`, org, id).Scan(&v.ID, &v.WarehouseID, &v.Code, &v.Type, &v.Status)
	return v, e
}

// YardVisitWarehouse returns the warehouse and status of a yard visit.
func (r *Repository) YardVisitWarehouse(ctx context.Context, q database.Querier, org, id string) (warehouse, status string, e error) {
	e = r.q(q).QueryRow(ctx, `SELECT warehouse_id,status FROM fleet_partner_yard_visits WHERE organization_id=$1 AND id=$2`, org, id).Scan(&warehouse, &status)
	return
}

// ReceiptByNumber finds a receipt by its organization-unique number.
func (r *Repository) ReceiptByNumber(ctx context.Context, org, number string) (models.WarehouseReceipt, error) {
	return scanReceipt(r.db.QueryRow(ctx, `SELECT `+receiptCols+` FROM fleet_partner_warehouse_receipts WHERE organization_id=$1 AND receipt_number=$2`, org, number))
}

func (r *Repository) InsertReceipt(ctx context.Context, q database.Querier, v models.WarehouseReceipt, actor string) (models.WarehouseReceipt, error) {
	return scanReceipt(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_warehouse_receipts(id,organization_id,warehouse_id,receipt_number,source_type,source_reference,receiving_location_id,quarantine_location_id,status,expected_at,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),'EXPECTED',$9,$10) RETURNING `+receiptCols,
		v.ID, v.OrganizationID, v.WarehouseID, v.ReceiptNumber, v.SourceType, v.SourceReference, v.ReceivingLocationID, v.QuarantineLocationID, v.ExpectedAt, actor))
}

func (r *Repository) InsertReceiptLine(ctx context.Context, q database.Querier, org string, l models.ReceiptLine) (models.ReceiptLine, error) {
	return scanReceiptLine(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_warehouse_receipt_lines(id,organization_id,receipt_id,line_number,sku,owner_type,owner_reference,lot_reference,serial_reference,unit,expected_quantity,putaway_location_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,'')) RETURNING `+receiptLineCols,
		l.ID, org, l.ReceiptID, l.LineNumber, l.SKU, l.OwnerType, l.OwnerReference, l.LotReference, l.SerialReference, l.Unit, l.ExpectedQuantity, l.PutawayLocationID))
}

func (r *Repository) GetReceipt(ctx context.Context, org, id string) (models.WarehouseReceipt, error) {
	return scanReceipt(r.db.QueryRow(ctx, `SELECT `+receiptCols+` FROM fleet_partner_warehouse_receipts WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) LockReceipt(ctx context.Context, q database.Querier, org, id string) (models.WarehouseReceipt, error) {
	return scanReceipt(r.q(q).QueryRow(ctx, `SELECT `+receiptCols+` FROM fleet_partner_warehouse_receipts WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id))
}

// ListReceipts lists receipts in warehouses of the permitted branches.
func (r *Repository) ListReceipts(ctx context.Context, org string, branches []string) ([]models.WarehouseReceipt, error) {
	rows, e := r.db.Query(ctx, `SELECT `+receiptCols+` FROM fleet_partner_warehouse_receipts x WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=x.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC,id LIMIT 500`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanReceipt)
}

func (r *Repository) ReceiptLines(ctx context.Context, q database.Querier, org, receiptID string, lock bool) ([]models.ReceiptLine, error) {
	sql := `SELECT ` + receiptLineCols + ` FROM fleet_partner_warehouse_receipt_lines WHERE organization_id=$1 AND receipt_id=$2 ORDER BY line_number`
	if lock {
		sql += ` FOR UPDATE`
	}
	rows, e := r.q(q).Query(ctx, sql, org, receiptID)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanReceiptLine)
}

// RecordLineReceipt stores the physical count of one line.
func (r *Repository) RecordLineReceipt(ctx context.Context, q database.Querier, org, lineID string, in models.ReceiveLineInput, accepted float64, discrepancy, itemID, quarantineItemID, actor string, at time.Time) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_warehouse_receipt_lines SET received_quantity=$3,damaged_quantity=$4,quarantined_quantity=$5,accepted_quantity=$6,discrepancy_status=$7,inventory_item_id=NULLIF($8,''),quarantine_item_id=NULLIF($9,''),status='RECEIVED',received_by=$10,received_at=$11,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status='PENDING'`,
		org, lineID, in.ReceivedQuantity, in.DamagedQuantity, in.QuarantinedQuantity, accepted, discrepancy, itemID, quarantineItemID, actor, at)
	return e
}

func (r *Repository) ResolveLineDiscrepancy(ctx context.Context, q database.Querier, org, lineID, resolution, note string) (int64, error) {
	tag, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_warehouse_receipt_lines SET discrepancy_status='RESOLVED',discrepancy_resolution=$3,discrepancy_note=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND discrepancy_status='OPEN'`, org, lineID, resolution, note)
	return tag.RowsAffected(), e
}

// TransitionReceipt moves a receipt under optimistic concurrency.
func (r *Repository) TransitionReceipt(ctx context.Context, q database.Querier, org, id string, expected int64, next, yardVisitID string) (models.WarehouseReceipt, error) {
	return scanReceipt(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_warehouse_receipts SET status=$4,yard_visit_id=COALESCE(NULLIF($5,''),yard_visit_id),
 arrived_at=CASE WHEN $4='ARRIVED' THEN now() ELSE arrived_at END,
 received_at=CASE WHEN $4='RECEIVED' AND received_at IS NULL THEN now() ELSE received_at END,
 completed_at=CASE WHEN $4 IN('COMPLETED','CANCELLED') THEN now() ELSE completed_at END,
 version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+receiptCols, org, id, expected, next, yardVisitID))
}

// ---------------------------------------------------------------------------
// Put-away tasks
// ---------------------------------------------------------------------------

const putawayCols = `id,organization_id,warehouse_id,receipt_id,receipt_line_id,inventory_item_id,from_location_id,COALESCE(suggested_location_id,''),COALESCE(to_location_id,''),COALESCE(destination_item_id,''),quantity,status,lease_owner,lease_expires_at,completed_at,version,created_at,updated_at`

func scanPutaway(row pgx.Row) (models.PutawayTask, error) {
	var v models.PutawayTask
	e := row.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.ReceiptID, &v.ReceiptLineID, &v.InventoryItemID, &v.FromLocationID, &v.SuggestedLocationID, &v.ToLocationID, &v.DestinationItemID, &v.Quantity, &v.Status, &v.LeaseOwner, &v.LeaseExpiresAt, &v.CompletedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) InsertPutawayTask(ctx context.Context, q database.Querier, v models.PutawayTask) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_putaway_tasks(id,organization_id,warehouse_id,receipt_id,receipt_line_id,inventory_item_id,from_location_id,suggested_location_id,quantity)VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9)`,
		v.ID, v.OrganizationID, v.WarehouseID, v.ReceiptID, v.ReceiptLineID, v.InventoryItemID, v.FromLocationID, v.SuggestedLocationID, v.Quantity)
	return e
}

func (r *Repository) GetPutawayTask(ctx context.Context, org, id string) (models.PutawayTask, error) {
	return scanPutaway(r.db.QueryRow(ctx, `SELECT `+putawayCols+` FROM fleet_partner_putaway_tasks WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) LockPutawayTask(ctx context.Context, q database.Querier, org, id string) (models.PutawayTask, error) {
	return scanPutaway(r.q(q).QueryRow(ctx, `SELECT `+putawayCols+` FROM fleet_partner_putaway_tasks WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id))
}

func (r *Repository) ListPutawayTasks(ctx context.Context, org string, branches []string) ([]models.PutawayTask, error) {
	rows, e := r.db.Query(ctx, `SELECT `+putawayCols+` FROM fleet_partner_putaway_tasks x WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=x.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at,id LIMIT 500`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanPutaway)
}

func (r *Repository) ClaimPutawayTask(ctx context.Context, q database.Querier, org, id, owner, claimID string, until time.Time) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_putaway_tasks SET status='CLAIMED',lease_owner=$3,client_claim_id=$4,lease_expires_at=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, id, owner, claimID, until)
	return e
}

func (r *Repository) CompletePutawayTask(ctx context.Context, q database.Querier, org, id, to, destItem, actor string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_putaway_tasks SET status='COMPLETED',to_location_id=$3,destination_item_id=$4,completed_by=$5,completed_at=now(),lease_owner='',lease_expires_at=NULL,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status='CLAIMED'`, org, id, to, destItem, actor)
	return e
}

func (r *Repository) OpenPutawayCount(ctx context.Context, q database.Querier, org, receiptID string) (int, error) {
	var n int
	e := r.q(q).QueryRow(ctx, `SELECT count(*) FROM fleet_partner_putaway_tasks WHERE organization_id=$1 AND receipt_id=$2 AND status IN('OPEN','CLAIMED')`, org, receiptID).Scan(&n)
	return n, e
}

// ---------------------------------------------------------------------------
// Stock helpers shared by put-away and pick
// ---------------------------------------------------------------------------

// LockStockLine locks the inventory item identified by its full stock key at
// a location.
func (r *Repository) LockStockLine(ctx context.Context, q database.Querier, src models.Item, location string) (models.Item, error) {
	return scanItem(r.q(q).QueryRow(ctx, `SELECT `+itemCols+` FROM fleet_partner_inventory_items WHERE organization_id=$1 AND warehouse_id=$2 AND location_id=$3 AND owner_type=$4 AND owner_reference=$5 AND sku=$6 AND batch_reference=$7 AND lot_reference=$8 AND serial_reference=$9 FOR UPDATE`,
		src.OrganizationID, src.WarehouseID, location, src.OwnerType, src.OwnerReference, src.SKU, src.BatchReference, src.LotReference, src.SerialReference))
}

// InsertItemWithStatus inserts a stock line with an explicit status.
func (r *Repository) InsertItemWithStatus(ctx context.Context, q database.Querier, item models.Item, quantity float64, status string) (models.Item, error) {
	return scanItem(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_inventory_items(id,organization_id,warehouse_id,location_id,owner_type,owner_reference,sku,batch_reference,lot_reference,serial_reference,quantity_projection,unit,status,expiry_at,condition)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING `+itemCols,
		item.ID, item.OrganizationID, item.WarehouseID, item.LocationID, item.OwnerType, item.OwnerReference, item.SKU, item.BatchReference, item.LotReference, item.SerialReference, quantity, item.Unit, status, item.ExpiryAt, item.Condition))
}

// AddItemQuantityKeepStatus adds quantity to a stock line without changing
// its status (quarantined stock stays quarantined).
func (r *Repository) AddItemQuantityKeepStatus(ctx context.Context, q database.Querier, org, id string, quantity float64) (models.Item, error) {
	return scanItem(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_inventory_items SET quantity_projection=quantity_projection+$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING `+itemCols, org, id, quantity))
}

// SubtractItemQuantity removes unreserved quantity from a stock line. The
// table's CHECK constraints (quantity >= 0, reserved <= quantity) are the
// last line of defence against negative or over-reserved stock.
func (r *Repository) SubtractItemQuantity(ctx context.Context, q database.Querier, org, id string, quantity float64) (models.Item, error) {
	return scanItem(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_inventory_items SET quantity_projection=quantity_projection-$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING `+itemCols, org, id, quantity))
}

// ---------------------------------------------------------------------------
// Pick waves and pick tasks
// ---------------------------------------------------------------------------

const waveCols = `id,organization_id,warehouse_id,wave_number,status,completed_at,version,created_at,updated_at`

func scanWave(row pgx.Row) (models.PickWave, error) {
	var v models.PickWave
	e := row.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.WaveNumber, &v.Status, &v.CompletedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	v.Tasks = []models.PickTask{}
	return v, e
}

const pickCols = `id,organization_id,pick_wave_id,fulfilment_order_id,fulfilment_line_id,inventory_item_id,from_location_id,quantity,picked_quantity,status,lease_owner,lease_expires_at,completed_at,version,created_at,updated_at`

func scanPick(row pgx.Row) (models.PickTask, error) {
	var v models.PickTask
	e := row.Scan(&v.ID, &v.OrganizationID, &v.PickWaveID, &v.FulfilmentOrderID, &v.FulfilmentLineID, &v.InventoryItemID, &v.FromLocationID, &v.Quantity, &v.PickedQuantity, &v.Status, &v.LeaseOwner, &v.LeaseExpiresAt, &v.CompletedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) InsertWave(ctx context.Context, q database.Querier, v models.PickWave, actor string) (models.PickWave, error) {
	return scanWave(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_pick_waves(id,organization_id,warehouse_id,wave_number,created_by)VALUES($1,$2,$3,$4,$5) RETURNING `+waveCols, v.ID, v.OrganizationID, v.WarehouseID, v.WaveNumber, actor))
}

func (r *Repository) WaveByNumber(ctx context.Context, org, number string) (models.PickWave, error) {
	return scanWave(r.db.QueryRow(ctx, `SELECT `+waveCols+` FROM fleet_partner_pick_waves WHERE organization_id=$1 AND wave_number=$2`, org, number))
}

func (r *Repository) GetWave(ctx context.Context, org, id string) (models.PickWave, error) {
	return scanWave(r.db.QueryRow(ctx, `SELECT `+waveCols+` FROM fleet_partner_pick_waves WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) ListWaves(ctx context.Context, org string, branches []string) ([]models.PickWave, error) {
	rows, e := r.db.Query(ctx, `SELECT `+waveCols+` FROM fleet_partner_pick_waves x WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=x.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC,id LIMIT 500`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanWave)
}

// FulfilmentLinesForWave locks the lines of a fulfilment order with the
// location of their reserved stock.
func (r *Repository) FulfilmentLinesForWave(ctx context.Context, q database.Querier, org, orderID string) ([]models.PickTask, error) {
	rows, e := r.q(q).Query(ctx, `SELECT l.id,l.inventory_item_id,i.location_id,l.requested_quantity FROM fleet_partner_fulfilment_lines l JOIN fleet_partner_inventory_items i ON i.id=l.inventory_item_id WHERE l.organization_id=$1 AND l.fulfilment_order_id=$2 ORDER BY l.created_at,l.id FOR UPDATE OF l`, org, orderID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.PickTask{}
	for rows.Next() {
		var v models.PickTask
		if e = rows.Scan(&v.FulfilmentLineID, &v.InventoryItemID, &v.FromLocationID, &v.Quantity); e != nil {
			return nil, e
		}
		v.FulfilmentOrderID = orderID
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) InsertPickTask(ctx context.Context, q database.Querier, v models.PickTask) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_pick_tasks(id,organization_id,pick_wave_id,fulfilment_order_id,fulfilment_line_id,inventory_item_id,from_location_id,quantity)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		v.ID, v.OrganizationID, v.PickWaveID, v.FulfilmentOrderID, v.FulfilmentLineID, v.InventoryItemID, v.FromLocationID, v.Quantity)
	return e
}

func (r *Repository) WaveTasks(ctx context.Context, org, waveID string) ([]models.PickTask, error) {
	rows, e := r.db.Query(ctx, `SELECT `+pickCols+` FROM fleet_partner_pick_tasks WHERE organization_id=$1 AND pick_wave_id=$2 ORDER BY created_at,id`, org, waveID)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanPick)
}

func (r *Repository) GetPickTask(ctx context.Context, org, id string) (models.PickTask, error) {
	return scanPick(r.db.QueryRow(ctx, `SELECT `+pickCols+` FROM fleet_partner_pick_tasks WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) LockPickTask(ctx context.Context, q database.Querier, org, id string) (models.PickTask, error) {
	return scanPick(r.q(q).QueryRow(ctx, `SELECT `+pickCols+` FROM fleet_partner_pick_tasks WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id))
}

func (r *Repository) ListPickTasks(ctx context.Context, org string, branches []string) ([]models.PickTask, error) {
	rows, e := r.db.Query(ctx, `SELECT `+pickCols+` FROM fleet_partner_pick_tasks x WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_pick_waves pw JOIN fleet_partner_warehouses w ON w.id=pw.warehouse_id WHERE pw.id=x.pick_wave_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at,id LIMIT 500`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanPick)
}

func (r *Repository) ClaimPickTask(ctx context.Context, q database.Querier, org, id, owner, claimID string, until time.Time) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_pick_tasks SET status='CLAIMED',lease_owner=$3,client_claim_id=$4,lease_expires_at=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, id, owner, claimID, until)
	return e
}

func (r *Repository) ClaimIdentity(ctx context.Context, q database.Querier, table, org, id string) (owner, claimID string, e error) {
	sql := map[string]string{
		"putaway": `SELECT lease_owner,client_claim_id FROM fleet_partner_putaway_tasks WHERE organization_id=$1 AND id=$2`,
		"pick":    `SELECT lease_owner,client_claim_id FROM fleet_partner_pick_tasks WHERE organization_id=$1 AND id=$2`,
	}[table]
	e = r.q(q).QueryRow(ctx, sql, org, id).Scan(&owner, &claimID)
	return
}

func (r *Repository) CompletePickTask(ctx context.Context, q database.Querier, org, id, status, actor string, picked float64) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_pick_tasks SET status=$3,picked_quantity=$4,completed_by=$5,completed_at=now(),lease_owner='',lease_expires_at=NULL,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status='CLAIMED'`, org, id, status, picked, actor)
	return e
}

// ActiveReservation returns the reserved quantity of an item still held for
// a fulfilment order.
func (r *Repository) ActiveReservation(ctx context.Context, q database.Querier, org, itemID, orderID string) (float64, error) {
	var qty float64
	e := r.q(q).QueryRow(ctx, `SELECT COALESCE(sum(quantity),0) FROM fleet_partner_inventory_reservations WHERE organization_id=$1 AND inventory_item_id=$2 AND fulfilment_order_id=$3 AND status='ACTIVE'`, org, itemID, orderID).Scan(&qty)
	return qty, e
}

func (r *Repository) SetLinePickedQuantity(ctx context.Context, q database.Querier, lineID string, picked float64) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_fulfilment_lines SET picked_quantity=$2,version=version+1,updated_at=now() WHERE id=$1`, lineID, picked)
	return e
}

// InsertPickTaskMovement appends the PICK movement of a completed task.
func (r *Repository) InsertPickTaskMovement(ctx context.Context, q database.Querier, org, taskID, itemID, orderID, actor string, quantity float64) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,quantity,reason_code,actor_reference,fulfilment_order_id,correlation_id,projection_before,projection_after)SELECT $1,$2,id,'PICK',location_id,$3,'PICK_TASK',$4,$5,$6,quantity_projection,quantity_projection FROM fleet_partner_inventory_items WHERE id=$7`,
		ids.New("imv"), org, quantity, actor, orderID, "pick-task:"+taskID, itemID)
	return e
}

// ReleaseOrderReservations ends every reservation an order still holds on
// an item, including the PICKED rows a completed or short pick task left.
func (r *Repository) ReleaseOrderReservations(ctx context.Context, q database.Querier, org, itemID, orderID string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_inventory_reservations SET status='RELEASED',version=version+1,updated_at=now() WHERE organization_id=$1 AND inventory_item_id=$2 AND fulfilment_order_id=$3 AND status IN('ACTIVE','PICKED')`, org, itemID, orderID)
	return e
}

// InsertPickReleaseMovement appends the RELEASE movement that returns
// picked-but-unshipped units of a cancelled order to available stock. Like
// PICK it leaves the quantity projection unchanged; it balances the PICK in
// the append-only movement history.
func (r *Repository) InsertPickReleaseMovement(ctx context.Context, q database.Querier, org, itemID, orderID, actor string, quantity float64) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,quantity,reason_code,actor_reference,fulfilment_order_id,correlation_id,projection_before,projection_after)SELECT $1,$2,id,'RELEASE',location_id,$3,'PICK_CANCELLED',$4,$5,$6,quantity_projection,quantity_projection FROM fleet_partner_inventory_items WHERE id=$7`,
		ids.New("imv"), org, quantity, actor, orderID, orderID+":pick-cancelled:"+itemID, itemID)
	return e
}

// OrderPickProgress counts the live tasks of an order still being picked
// and how many ended short.
func (r *Repository) OrderPickProgress(ctx context.Context, q database.Querier, org, orderID string) (open, short int, e error) {
	e = r.q(q).QueryRow(ctx, `SELECT count(*) FILTER(WHERE status IN('OPEN','CLAIMED')),count(*) FILTER(WHERE status='SHORT') FROM fleet_partner_pick_tasks WHERE organization_id=$1 AND fulfilment_order_id=$2 AND status<>'CANCELLED'`, org, orderID).Scan(&open, &short)
	return
}

func (r *Repository) WaveOpenCount(ctx context.Context, q database.Querier, org, waveID string) (int, error) {
	var n int
	e := r.q(q).QueryRow(ctx, `SELECT count(*) FROM fleet_partner_pick_tasks WHERE organization_id=$1 AND pick_wave_id=$2 AND status IN('OPEN','CLAIMED')`, org, waveID).Scan(&n)
	return n, e
}

func (r *Repository) CompleteWave(ctx context.Context, q database.Querier, org, waveID string) (int64, error) {
	var v int64
	e := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_pick_waves SET status='COMPLETED',completed_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status='RELEASED' RETURNING version`, org, waveID).Scan(&v)
	if errors.Is(e, pgx.ErrNoRows) {
		return 0, nil
	}
	return v, e
}

// SetOrderStatusLocked moves a fulfilment order whose row the caller holds
// locked; it returns the new version.
func (r *Repository) SetOrderStatusLocked(ctx context.Context, q database.Querier, org, orderID, status string) (int64, error) {
	var v int64
	e := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_fulfilment_orders SET status=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, org, orderID, status).Scan(&v)
	return v, e
}

// LockOrderForWave locks a fulfilment order header, returning warehouse and
// status.
func (r *Repository) LockOrderForWave(ctx context.Context, q database.Querier, org, orderID string) (warehouse, status string, e error) {
	e = r.q(q).QueryRow(ctx, `SELECT warehouse_id,status FROM fleet_partner_fulfilment_orders WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, orderID).Scan(&warehouse, &status)
	return
}

// ---------------------------------------------------------------------------
// Packing units
// ---------------------------------------------------------------------------

const packingCols = `id,organization_id,fulfilment_order_id,packing_code,status,weight_grams,dimensions_mm,seal_number,contents,packed_at,version,created_at`

func scanPacking(row pgx.Row) (models.PackingUnit, error) {
	var v models.PackingUnit
	var contents []byte
	e := row.Scan(&v.ID, &v.OrganizationID, &v.FulfilmentOrderID, &v.PackingCode, &v.Status, &v.WeightGrams, &v.DimensionsMm, &v.SealNumber, &contents, &v.PackedAt, &v.Version, &v.CreatedAt)
	if e == nil {
		v.Contents = []models.PackedContent{}
		e = json.Unmarshal(contents, &v.Contents)
	}
	return v, e
}

func (r *Repository) InsertPackingUnit(ctx context.Context, q database.Querier, v models.PackingUnit, actor string) (models.PackingUnit, error) {
	contents, _ := json.Marshal(v.Contents)
	return scanPacking(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_packing_units(id,organization_id,fulfilment_order_id,packing_code,status,weight_grams,dimensions_mm,seal_number,contents,packed_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+packingCols,
		v.ID, v.OrganizationID, v.FulfilmentOrderID, v.PackingCode, v.Status, v.WeightGrams, models.Raw(v.DimensionsMm, "{}"), v.SealNumber, contents, actor))
}

func (r *Repository) PackingByCode(ctx context.Context, org, code string) (models.PackingUnit, error) {
	return scanPacking(r.db.QueryRow(ctx, `SELECT `+packingCols+` FROM fleet_partner_packing_units WHERE organization_id=$1 AND packing_code=$2`, org, code))
}

func (r *Repository) GetPackingUnit(ctx context.Context, org, id string) (models.PackingUnit, error) {
	return scanPacking(r.db.QueryRow(ctx, `SELECT `+packingCols+` FROM fleet_partner_packing_units WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) ListPackingUnits(ctx context.Context, org string, branches []string) ([]models.PackingUnit, error) {
	rows, e := r.db.Query(ctx, `SELECT `+packingCols+` FROM fleet_partner_packing_units x WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_fulfilment_orders o JOIN fleet_partner_warehouses w ON w.id=o.warehouse_id WHERE o.id=x.fulfilment_order_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC,id LIMIT 500`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanPacking)
}

// PackedQuantities sums the quantities already packed per item for an order.
func (r *Repository) PackedQuantities(ctx context.Context, q database.Querier, org, orderID string) (map[string]float64, error) {
	rows, e := r.q(q).Query(ctx, `SELECT c->>'inventoryItemId',sum((c->>'quantity')::numeric)::float8 FROM fleet_partner_packing_units p,jsonb_array_elements(p.contents) c WHERE p.organization_id=$1 AND p.fulfilment_order_id=$2 GROUP BY 1`, org, orderID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var k string
		var v float64
		if e = rows.Scan(&k, &v); e != nil {
			return nil, e
		}
		out[k] = v
	}
	return out, rows.Err()
}

// PickedQuantities sums the picked quantity per item for an order.
func (r *Repository) PickedQuantities(ctx context.Context, q database.Querier, org, orderID string) (map[string]float64, error) {
	rows, e := r.q(q).Query(ctx, `SELECT inventory_item_id,sum(picked_quantity)::float8 FROM fleet_partner_fulfilment_lines WHERE organization_id=$1 AND fulfilment_order_id=$2 GROUP BY 1`, org, orderID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var k string
		var v float64
		if e = rows.Scan(&k, &v); e != nil {
			return nil, e
		}
		out[k] = v
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Gate events
// ---------------------------------------------------------------------------

const gateCols = `id,organization_id,yard_visit_id,sequence_no,event_type,gate_code,actor_reference,evidence_file_ids,notes,occurred_at`

func scanGate(row pgx.Row) (models.GateEvent, error) {
	var v models.GateEvent
	e := row.Scan(&v.ID, &v.OrganizationID, &v.YardVisitID, &v.SequenceNo, &v.EventType, &v.GateCode, &v.ActorReference, &v.EvidenceFileIDs, &v.Notes, &v.OccurredAt)
	return v, e
}

// LockYardVisitRow serializes gate events of one visit.
func (r *Repository) LockYardVisitRow(ctx context.Context, q database.Querier, org, id string) (string, error) {
	var status string
	e := r.q(q).QueryRow(ctx, `SELECT status FROM fleet_partner_yard_visits WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id).Scan(&status)
	return status, e
}

func (r *Repository) GateEvents(ctx context.Context, q database.Querier, org, visitID string) ([]models.GateEvent, error) {
	rows, e := r.q(q).Query(ctx, `SELECT `+gateCols+` FROM fleet_partner_gate_events WHERE organization_id=$1 AND yard_visit_id=$2 ORDER BY sequence_no`, org, visitID)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanGate)
}

func (r *Repository) InsertGateEvent(ctx context.Context, q database.Querier, v models.GateEvent) (models.GateEvent, error) {
	return scanGate(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_gate_events(id,organization_id,yard_visit_id,sequence_no,event_type,gate_code,actor_reference,evidence_file_ids,notes)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+gateCols,
		v.ID, v.OrganizationID, v.YardVisitID, v.SequenceNo, v.EventType, v.GateCode, v.ActorReference, models.Raw(v.EvidenceFileIDs, "[]"), v.Notes))
}

// ReleaseExpiredTaskLeases returns claimed put-away and pick tasks whose
// lease lapsed to OPEN so another worker can claim them.
func (r *Repository) ReleaseExpiredTaskLeases(ctx context.Context, q database.Querier, org string) error {
	for _, sql := range []string{
		`UPDATE fleet_partner_putaway_tasks SET status='OPEN',lease_owner='',client_claim_id='',lease_expires_at=NULL,version=version+1,updated_at=now() WHERE organization_id=$1 AND status='CLAIMED' AND lease_expires_at<now()`,
		`UPDATE fleet_partner_pick_tasks SET status='OPEN',lease_owner='',client_claim_id='',lease_expires_at=NULL,version=version+1,updated_at=now() WHERE organization_id=$1 AND status='CLAIMED' AND lease_expires_at<now()`,
	} {
		if _, e := r.q(q).Exec(ctx, sql, org); e != nil {
			return e
		}
	}
	return nil
}
