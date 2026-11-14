package repositories

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ---------------------------------------------------------------------------
// Dock appointments
// ---------------------------------------------------------------------------

const dockCols = `id,organization_id,warehouse_id,dock_location_id,carrier_reference,provider_reference,vehicle_or_truck_reference,appointment_type,window_start,window_end,status,checked_in_at,loading_started_at,loading_completed_at,checked_out_at,version,created_at,updated_at`

func scanDock(row pgx.Row) (models.DockAppointment, error) {
	var v models.DockAppointment
	e := row.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.DockLocationID, &v.CarrierReference, &v.ProviderReference, &v.VehicleOrTruckReference, &v.AppointmentType, &v.WindowStart, &v.WindowEnd, &v.Status, &v.CheckedInAt, &v.LoadingStartedAt, &v.LoadingCompletedAt, &v.CheckedOutAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// LockDock serializes appointment booking on one dock for the transaction.
func (r *Repository) LockDock(ctx context.Context, q database.Querier, org, dockLocationID string) {
	_, _ = r.q(q).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, org+":"+dockLocationID)
}

func (r *Repository) DockConflict(ctx context.Context, q database.Querier, org, dockLocationID string, start, end time.Time) bool {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_dock_appointments WHERE organization_id=$1 AND dock_location_id=$2 AND status NOT IN('COMPLETED','NO_SHOW','CANCELLED') AND window_start<$4 AND window_end>$3)`, org, dockLocationID, start, end)
}

func (r *Repository) InsertDock(ctx context.Context, q database.Querier, v models.DockAppointment) (models.DockAppointment, error) {
	return scanDock(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_dock_appointments(id,organization_id,warehouse_id,dock_location_id,carrier_reference,provider_reference,vehicle_or_truck_reference,appointment_type,window_start,window_end)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+dockCols, v.ID, v.OrganizationID, v.WarehouseID, v.DockLocationID, v.CarrierReference, v.ProviderReference, v.VehicleOrTruckReference, v.AppointmentType, v.WindowStart, v.WindowEnd))
}

func (r *Repository) GetDock(ctx context.Context, org, id string) (models.DockAppointment, error) {
	return scanDock(r.db.QueryRow(ctx, `SELECT `+dockCols+` FROM fleet_partner_dock_appointments WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) ListDocks(ctx context.Context, org string, branches []string) ([]models.DockAppointment, error) {
	rows, e := r.db.Query(ctx, `SELECT `+dockCols+` FROM fleet_partner_dock_appointments d WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=d.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY window_start`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanDock)
}

// dockCommandStamps are the timestamps each dock command sets.
var dockCommandStamps = map[string]string{
	"CHECK_IN": "checked_in_at=now(),",
	"COMPLETE": "loading_completed_at=COALESCE(loading_completed_at,now()),checked_out_at=now(),",
}

func (r *Repository) UpdateDock(ctx context.Context, q database.Querier, org, id string, expected int64, command, status string) (models.DockAppointment, error) {
	return scanDock(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_dock_appointments SET `+dockCommandStamps[command]+`status=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+dockCols, org, id, expected, status))
}

// ---------------------------------------------------------------------------
// Yard visits
// ---------------------------------------------------------------------------

const yardCols = `id,organization_id,warehouse_id,provider_reference,vehicle_or_truck_reference,gate_reference,bay_reference,seal_reference,security_status,status,checked_in_at,entered_yard_at,exited_yard_at,checked_out_at,version,created_at,updated_at`

func scanYard(row pgx.Row) (models.YardVisit, error) {
	var v models.YardVisit
	e := row.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.ProviderReference, &v.VehicleOrTruckReference, &v.GateReference, &v.BayReference, &v.SealReference, &v.SecurityStatus, &v.Status, &v.CheckedInAt, &v.EnteredYardAt, &v.ExitedYardAt, &v.CheckedOutAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) InsertYard(ctx context.Context, q database.Querier, v models.YardVisit) (models.YardVisit, error) {
	return scanYard(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_yard_visits(id,organization_id,warehouse_id,provider_reference,vehicle_or_truck_reference,gate_reference,bay_reference,seal_reference)VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+yardCols, v.ID, v.OrganizationID, v.WarehouseID, v.ProviderReference, v.VehicleOrTruckReference, v.GateReference, v.BayReference, v.SealReference))
}

func (r *Repository) GetYard(ctx context.Context, org, id string) (models.YardVisit, error) {
	return scanYard(r.db.QueryRow(ctx, `SELECT `+yardCols+` FROM fleet_partner_yard_visits WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) ListYards(ctx context.Context, org string, branches []string) ([]models.YardVisit, error) {
	rows, e := r.db.Query(ctx, `SELECT `+yardCols+` FROM fleet_partner_yard_visits y WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=y.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanYard)
}

func (r *Repository) UpdateYard(ctx context.Context, q database.Querier, org, id string, expected int64, status, security, newSeal string) (models.YardVisit, error) {
	return scanYard(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_yard_visits SET status=$4,security_status=$5,seal_reference=CASE WHEN $6<>'' THEN $6 ELSE seal_reference END,checked_in_at=CASE WHEN $4='GATE_CHECK_IN' THEN now() ELSE checked_in_at END,exited_yard_at=CASE WHEN $4='RELEASED' THEN now() ELSE exited_yard_at END,checked_out_at=CASE WHEN $4='CHECKED_OUT' THEN now() ELSE checked_out_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+yardCols, org, id, expected, status, security, newSeal))
}

func (r *Repository) InsertYardHistory(ctx context.Context, q database.Querier, org, id string, old, next models.YardVisit, reason, actor string) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_yard_visit_history(id,organization_id,yard_visit_id,previous_status,new_status,previous_seal,new_seal,reason,actor_reference,version)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("yrh"), org, id, old.Status, next.Status, old.SealReference, next.SealReference, reason, actor, next.Version)
	return e
}

// ---------------------------------------------------------------------------
// Terminal transfers
// ---------------------------------------------------------------------------

const transferCols = `id,organization_id,origin_warehouse_id,destination_warehouse_id,manifest_id,route_reference,status,departed_at,arrived_at,origin_snapshot_reference,destination_snapshot_reference,reconciliation_status,version,created_at,updated_at`

func scanTransfer(row pgx.Row) (models.TerminalTransfer, error) {
	var v models.TerminalTransfer
	e := row.Scan(&v.ID, &v.OrganizationID, &v.OriginWarehouseID, &v.DestinationWarehouseID, &v.ManifestID, &v.RouteReference, &v.Status, &v.DepartedAt, &v.ArrivedAt, &v.OriginSnapshotReference, &v.DestinationSnapshotReference, &v.ReconciliationStatus, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) InsertTransfer(ctx context.Context, q database.Querier, v models.TerminalTransfer) (models.TerminalTransfer, error) {
	return scanTransfer(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_terminal_transfers(id,organization_id,origin_warehouse_id,destination_warehouse_id,manifest_id,route_reference)VALUES($1,$2,$3,$4,$5,$6) RETURNING `+transferCols, v.ID, v.OrganizationID, v.OriginWarehouseID, v.DestinationWarehouseID, v.ManifestID, v.RouteReference))
}

func (r *Repository) GetTransfer(ctx context.Context, org, id string) (models.TerminalTransfer, error) {
	return scanTransfer(r.db.QueryRow(ctx, `SELECT `+transferCols+` FROM fleet_partner_terminal_transfers WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) ListTransfers(ctx context.Context, org string, branches []string) ([]models.TerminalTransfer, error) {
	rows, e := r.db.Query(ctx, `SELECT `+transferCols+` FROM fleet_partner_terminal_transfers tr WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=tr.origin_warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=tr.destination_warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanTransfer)
}

// TransferSnapshot reads the ORIGIN or DESTINATION package snapshot.
func (r *Repository) TransferSnapshot(ctx context.Context, q database.Querier, transferID, snapshotType string) ([]byte, error) {
	var snap []byte
	e := r.q(q).QueryRow(ctx, `SELECT package_snapshot FROM fleet_partner_terminal_transfer_snapshots WHERE transfer_id=$1 AND snapshot_type=$2`, transferID, snapshotType).Scan(&snap)
	return snap, e
}

// InsertDiscrepancy records a snapshot mismatch; a failure is ignored.
func (r *Repository) InsertDiscrepancy(ctx context.Context, q database.Querier, org, transferID string, expected, actual []byte) {
	_, _ = r.q(q).Exec(ctx, `INSERT INTO fleet_partner_terminal_discrepancies(id,organization_id,transfer_id,discrepancy_type,expected_snapshot,actual_snapshot)VALUES($1,$2,$3,'PACKAGE_SNAPSHOT',$4,$5)`, ids.New("tds"), org, transferID, expected, actual)
}

func (r *Repository) ManifestPackageSnapshot(ctx context.Context, q database.Querier, manifestID string) (json.RawMessage, error) {
	var pkg json.RawMessage
	e := r.q(q).QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('packageId',p.id,'status',p.status) ORDER BY p.id),'[]') FROM fleet_partner_manifest_items mi JOIN fleet_partner_packages p ON p.id=mi.item_id WHERE mi.manifest_id=$1 AND mi.item_type='PACKAGE'`, manifestID).Scan(&pkg)
	return pkg, e
}

func (r *Repository) InsertTransferSnapshot(ctx context.Context, q database.Querier, id, org, transferID, snapshotType string, pkg json.RawMessage, actor string) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_terminal_transfer_snapshots(id,organization_id,transfer_id,snapshot_type,package_snapshot,inventory_snapshot,custody_snapshot,captured_by)VALUES($1,$2,$3,$4,$5,'[]','[]',$6)`, id, org, transferID, snapshotType, pkg, actor)
	return e
}

func (r *Repository) UpdateTransfer(ctx context.Context, q database.Querier, org, id string, expected int64, next, snapshotType, snapRef string) (models.TerminalTransfer, error) {
	return scanTransfer(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_terminal_transfers SET status=$4,departed_at=CASE WHEN $4='DEPARTED' THEN now() ELSE departed_at END,arrived_at=CASE WHEN $4='ARRIVED' THEN now() ELSE arrived_at END,origin_snapshot_reference=CASE WHEN $5='ORIGIN' THEN $6 ELSE origin_snapshot_reference END,destination_snapshot_reference=CASE WHEN $5='DESTINATION' THEN $6 ELSE destination_snapshot_reference END,reconciliation_status=CASE WHEN $4='COMPLETED' THEN 'MATCHED' WHEN $4='DISCREPANCY' THEN 'DISCREPANCY' ELSE reconciliation_status END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+transferCols, org, id, expected, next, snapshotType, snapRef))
}

// ---------------------------------------------------------------------------
// Returns
// ---------------------------------------------------------------------------

const returnCols = `id,organization_id,warehouse_id,source_order_id,source_shipment_id,COALESCE(package_id,''),reason_code,condition,disposition,financial_reference,status,COALESCE(inventory_item_id,''),version,created_at,updated_at,completed_at`

func scanReturn(row pgx.Row) (models.Return, error) {
	var v models.Return
	e := row.Scan(&v.ID, &v.OrganizationID, &v.WarehouseID, &v.SourceOrderID, &v.SourceShipmentID, &v.PackageID, &v.ReasonCode, &v.Condition, &v.Disposition, &v.FinancialReference, &v.Status, &v.InventoryItemID, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.CompletedAt)
	return v, e
}

func (r *Repository) InsertReturn(ctx context.Context, q database.Querier, v models.Return) (models.Return, error) {
	return scanReturn(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_returns(id,organization_id,warehouse_id,source_order_id,source_shipment_id,package_id,reason_code,condition,financial_reference,inventory_item_id)VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,NULLIF($10,'')) RETURNING `+returnCols, v.ID, v.OrganizationID, v.WarehouseID, v.SourceOrderID, v.SourceShipmentID, v.PackageID, v.ReasonCode, v.Condition, v.FinancialReference, v.InventoryItemID))
}

func (r *Repository) GetReturn(ctx context.Context, org, id string) (models.Return, error) {
	return scanReturn(r.db.QueryRow(ctx, `SELECT `+returnCols+` FROM fleet_partner_returns WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) ListReturns(ctx context.Context, org string, branches []string) ([]models.Return, error) {
	rows, e := r.db.Query(ctx, `SELECT `+returnCols+` FROM fleet_partner_returns r WHERE organization_id=$1 AND EXISTS(SELECT 1 FROM fleet_partner_warehouses w WHERE w.id=r.warehouse_id AND(COALESCE(cardinality($2::text[]),0)=0 OR w.branch_id=ANY($2::text[]))) ORDER BY created_at DESC`, org, branches)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanReturn)
}

// LockItemForReturn locks a returned item and returns its quantity and
// location.
func (r *Repository) LockItemForReturn(ctx context.Context, q database.Querier, org, itemID string) (float64, string, error) {
	var qty float64
	var loc string
	e := r.q(q).QueryRow(ctx, `SELECT quantity_projection,location_id FROM fleet_partner_inventory_items WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, itemID).Scan(&qty, &loc)
	return qty, loc, e
}

func (r *Repository) InsertReturnMovement(ctx context.Context, q database.Querier, org string, v models.Return, location, actor string, before, after float64) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_inventory_movements(id,organization_id,inventory_item_id,movement_type,from_location_id,to_location_id,quantity,reason_code,actor_reference,return_id,correlation_id,projection_before,projection_after)VALUES($1,$2,$3,'RETURN',$4,$4,1,$5,$6,$7,$8,$9,$10)`, ids.New("imv"), org, v.InventoryItemID, location, v.ReasonCode, actor, v.ID, v.ID+":receive", before, after)
	return e
}

func (r *Repository) UpdateReturn(ctx context.Context, q database.Querier, org, id string, expected int64, next, disposition string) (models.Return, error) {
	return scanReturn(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_returns SET status=$4,disposition=CASE WHEN $5<>'' THEN $5 ELSE disposition END,completed_at=CASE WHEN $4='COMPLETED' THEN now() ELSE completed_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+returnCols, org, id, expected, next, disposition))
}
