package repositories

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ---------------------------------------------------------------------------
// Safety blocks
// ---------------------------------------------------------------------------

// BlockAsset sets an asset BLOCKED unconditionally and returns its version.
func (r *Repository) BlockAsset(ctx context.Context, q database.Querier, org, id string) (int64, error) {
	var version int64
	e := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_assets SET operational_status='BLOCKED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, org, id).Scan(&version)
	return version, e
}

// BlockActiveAsset blocks an active, not yet blocked asset; pgx.ErrNoRows
// means nothing changed.
func (r *Repository) BlockActiveAsset(ctx context.Context, q database.Querier, org, id string) (int64, error) {
	var version int64
	e := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_assets SET operational_status='BLOCKED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND lifecycle_status='ACTIVE' AND operational_status<>'BLOCKED' RETURNING version`, org, id).Scan(&version)
	return version, e
}

// BlockAssetForDecline blocks an asset reported unsafe unless it is already
// blocked or out of use.
func (r *Repository) BlockAssetForDecline(ctx context.Context, q database.Querier, org, id string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_assets SET operational_status='BLOCKED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND operational_status NOT IN('BLOCKED','RETIRED','ARCHIVED')`, org, id)
	return e
}

// InsertSafetyBlock records an ACTIVE safety block; dedupe ignores a repeat of
// the same source.
func (r *Repository) InsertSafetyBlock(ctx context.Context, q database.Querier, org, assetID, sourceType, sourceRef, reason, createdBy string, dedupe bool) error {
	sql := `INSERT INTO fleet_partner_asset_safety_blocks(id,organization_id,asset_id,source_type,source_reference,status,reason,created_by)VALUES($1,$2,$3,$4,$5,'ACTIVE',$6,$7)`
	if dedupe {
		sql += `ON CONFLICT(organization_id,source_type,source_reference)DO NOTHING`
	}
	_, e := r.q(q).Exec(ctx, sql, ids.New("asb"), org, assetID, sourceType, sourceRef, reason, createdBy)
	return e
}

// UncoveredSafetyBlocks counts the safety blocks in force (since the last
// authorized clearance) whose own repair work order is not completed.
func (r *Repository) UncoveredSafetyBlocks(ctx context.Context, q database.Querier, org, assetID string) (int, error) {
	var uncovered int
	e := r.q(q).QueryRow(ctx, `SELECT count(*) FROM fleet_partner_asset_safety_blocks b
WHERE b.organization_id=$1 AND b.asset_id=$2 AND b.status='ACTIVE'
AND b.created_at>COALESCE((SELECT max(c.created_at) FROM fleet_partner_asset_safety_blocks c WHERE c.organization_id=$1 AND c.asset_id=$2 AND c.status='CLEARED'),'-infinity'::timestamptz)
AND NOT EXISTS(SELECT 1 FROM fleet_partner_work_orders w WHERE w.organization_id=b.organization_id AND w.asset_id=b.asset_id AND w.source_type=b.source_type AND w.source_reference=b.source_reference AND w.status='COMPLETED')`, org, assetID).Scan(&uncovered)
	return uncovered, e
}

// ReleaseBlock returns a BLOCKED asset to ASSIGNED or AVAILABLE and records
// the clearance.
func (r *Repository) ReleaseBlock(ctx context.Context, q database.Querier, org, assetID, workOrderID, actor, reason string) (int64, error) {
	var version int64
	if e := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_assets SET operational_status=CASE WHEN EXISTS(SELECT 1 FROM fleet_partner_asset_assignments aa WHERE aa.organization_id=$1 AND aa.asset_id=$2 AND aa.status='ACTIVE')THEN'ASSIGNED'ELSE'AVAILABLE'END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND operational_status='BLOCKED' RETURNING version`, org, assetID).Scan(&version); e != nil {
		return 0, e
	}
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_asset_safety_blocks(id,organization_id,asset_id,source_type,source_reference,status,reason,created_by,cleared_by,cleared_at,clear_reason)VALUES($1,$2,$3,'RETURN_TO_SERVICE',$4,'CLEARED','Authorized return to service',$5,$5,now(),$6)`, ids.New("asb"), org, assetID, workOrderID, actor, reason)
	return version, e
}

// ResolveAssetID resolves an asset by id or platform vehicle id and returns
// its id and branch.
func (r *Repository) ResolveAssetID(ctx context.Context, q database.Querier, org, reference string) (string, string, error) {
	var id, branch string
	e := r.q(q).QueryRow(ctx, `SELECT id,branch_id FROM fleet_partner_assets WHERE organization_id=$1 AND(id=$2 OR platform_vehicle_id=$2)`, org, reference).Scan(&id, &branch)
	return id, branch, e
}

// ---------------------------------------------------------------------------
// Breakdowns
// ---------------------------------------------------------------------------

func (r *Repository) InsertBreakdown(ctx context.Context, q database.Querier, v *models.Breakdown) error {
	return r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_breakdowns(id,organization_id,asset_id,provider_membership_id,trip_reference,location_reference,occurred_at,severity,trip_impact,rescue_status,towing_reference,replacement_asset_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,COALESCE(NULLIF($10,''),'REQUESTED'),$11,NULLIF($12,''))RETURNING rescue_status,status,version,created_at,updated_at`, v.ID, v.OrganizationID, v.AssetID, v.ProviderMembershipID, v.TripReference, v.LocationReference, v.OccurredAt, v.Severity, v.TripImpact, v.RescueStatus, v.TowingReference, v.ReplacementAssetID).Scan(&v.RescueStatus, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
}

func (r *Repository) UpdateBreakdown(ctx context.Context, q database.Querier, org, id string, expected int64, v *models.Breakdown) error {
	return r.q(q).QueryRow(ctx, `UPDATE fleet_partner_breakdowns SET rescue_status=COALESCE(NULLIF($4,''),rescue_status),towing_reference=COALESCE(NULLIF($5,''),towing_reference),replacement_asset_id=COALESCE(NULLIF($6,''),replacement_asset_id),status=COALESCE(NULLIF($7,''),status),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING asset_id,provider_membership_id,trip_reference,location_reference,occurred_at,severity,trip_impact,rescue_status,towing_reference,COALESCE(replacement_asset_id,''),status,organization_id,version,created_at,updated_at`, org, id, expected, v.RescueStatus, v.TowingReference, v.ReplacementAssetID, v.Status).Scan(&v.AssetID, &v.ProviderMembershipID, &v.TripReference, &v.LocationReference, &v.OccurredAt, &v.Severity, &v.TripImpact, &v.RescueStatus, &v.TowingReference, &v.ReplacementAssetID, &v.Status, &v.OrganizationID, &v.Version, &v.CreatedAt, &v.UpdatedAt)
}

func (r *Repository) GetBreakdown(ctx context.Context, org, id string) (models.Breakdown, error) {
	var v models.Breakdown
	e := r.db.QueryRow(ctx, `SELECT id,organization_id,asset_id,provider_membership_id,trip_reference,location_reference,occurred_at,severity,trip_impact,rescue_status,towing_reference,COALESCE(replacement_asset_id,''),status,version,created_at,updated_at FROM fleet_partner_breakdowns WHERE organization_id=$1 AND id=$2`, org, id).Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.ProviderMembershipID, &v.TripReference, &v.LocationReference, &v.OccurredAt, &v.Severity, &v.TripImpact, &v.RescueStatus, &v.TowingReference, &v.ReplacementAssetID, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) ListBreakdownIDs(ctx context.Context, org string) ([]string, error) {
	rows, e := r.db.Query(ctx, `SELECT id FROM fleet_partner_breakdowns WHERE organization_id=$1 ORDER BY created_at DESC`, org)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanString)
}

// ---------------------------------------------------------------------------
// Parts and stock movements
// ---------------------------------------------------------------------------

func (r *Repository) InsertPart(ctx context.Context, q database.Querier, v *models.Part) error {
	return r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_parts(id,organization_id,branch_id,sku,name,stock_unit,reorder_level,warranty_information)VALUES($1,$2,$3,$4,$5,$6,$7,$8)RETURNING current_quantity_projection,status,version,created_at,updated_at`, v.ID, v.OrganizationID, v.BranchID, v.SKU, v.Name, v.StockUnit, v.ReorderLevel, v.WarrantyInformation).Scan(&v.CurrentQuantityProjection, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
}

func (r *Repository) UpdatePart(ctx context.Context, q database.Querier, org, id string, expected int64, v *models.Part) error {
	return r.q(q).QueryRow(ctx, `UPDATE fleet_partner_parts SET name=COALESCE(NULLIF($4,''),name),reorder_level=CASE WHEN $5>=0 THEN $5 ELSE reorder_level END,status=COALESCE(NULLIF($6,''),status),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING branch_id,sku,name,stock_unit,current_quantity_projection,reorder_level,warranty_information,status,organization_id,version,created_at,updated_at`, org, id, expected, v.Name, v.ReorderLevel, v.Status).Scan(&v.BranchID, &v.SKU, &v.Name, &v.StockUnit, &v.CurrentQuantityProjection, &v.ReorderLevel, &v.WarrantyInformation, &v.Status, &v.OrganizationID, &v.Version, &v.CreatedAt, &v.UpdatedAt)
}

func (r *Repository) GetPart(ctx context.Context, org, id string) (models.Part, error) {
	var v models.Part
	e := r.db.QueryRow(ctx, `SELECT id,organization_id,branch_id,sku,name,stock_unit,current_quantity_projection,reorder_level,warranty_information,status,version,created_at,updated_at FROM fleet_partner_parts WHERE organization_id=$1 AND id=$2`, org, id).Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.SKU, &v.Name, &v.StockUnit, &v.CurrentQuantityProjection, &v.ReorderLevel, &v.WarrantyInformation, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) ListPartIDs(ctx context.Context, org string) ([]string, error) {
	rows, e := r.db.Query(ctx, `SELECT id FROM fleet_partner_parts WHERE organization_id=$1 ORDER BY sku`, org)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanString)
}

// PartsAtReorderLevel lists active parts at or below their reorder level.
func (r *Repository) PartsAtReorderLevel(ctx context.Context, org string) ([]string, error) {
	rows, e := r.db.Query(ctx, `SELECT id FROM fleet_partner_parts WHERE organization_id=$1 AND status='ACTIVE' AND current_quantity_projection<=reorder_level`, org)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanString)
}

// LockPartStock locks a part and returns its projected quantity and reorder
// level.
func (r *Repository) LockPartStock(ctx context.Context, q database.Querier, org, id string) (float64, float64, error) {
	var qty, reorder float64
	e := r.q(q).QueryRow(ctx, `SELECT current_quantity_projection,reorder_level FROM fleet_partner_parts WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id).Scan(&qty, &reorder)
	return qty, reorder, e
}

func (r *Repository) SetPartProjection(ctx context.Context, q database.Querier, org, id string, projected float64) (int64, error) {
	var version int64
	e := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_parts SET current_quantity_projection=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, org, id, projected).Scan(&version)
	return version, e
}

func (r *Repository) InsertStockMovement(ctx context.Context, q database.Querier, v *models.StockMovement) error {
	return r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_stock_movements(id,organization_id,part_id,location_reference,movement_type,quantity,reason,work_order_id,correlation_id,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10)RETURNING created_at`, v.ID, v.OrganizationID, v.PartID, v.LocationReference, v.MovementType, v.Quantity, v.Reason, v.WorkOrderID, v.CorrelationID, v.CreatedBy).Scan(&v.CreatedAt)
}

func (r *Repository) ListStockMovements(ctx context.Context, org string) ([]models.StockMovement, error) {
	rows, e := r.db.Query(ctx, `SELECT id,organization_id,part_id,location_reference,movement_type,quantity,reason,COALESCE(work_order_id,''),correlation_id,created_by,created_at FROM fleet_partner_stock_movements WHERE organization_id=$1 ORDER BY created_at DESC`, org)
	if e != nil {
		return nil, e
	}
	return collect(rows, func(row pgx.Row) (models.StockMovement, error) {
		var v models.StockMovement
		e := row.Scan(&v.ID, &v.OrganizationID, &v.PartID, &v.LocationReference, &v.MovementType, &v.Quantity, &v.Reason, &v.WorkOrderID, &v.CorrelationID, &v.CreatedBy, &v.CreatedAt)
		return v, e
	})
}

// ---------------------------------------------------------------------------
// Defects
// ---------------------------------------------------------------------------

const defectCols = `id,organization_id,branch_id,asset_id,source_type,source_reference,item_code,description,severity,critical,status,COALESCE(work_order_id,''),deferred_until,deferral_reason,resolution_notes,resolved_by,resolved_at,reported_by,version,created_at,updated_at`

func scanDefect(row pgx.Row) (models.Defect, error) {
	var v models.Defect
	e := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.AssetID, &v.SourceType, &v.SourceReference, &v.ItemCode, &v.Description, &v.Severity, &v.Critical, &v.Status, &v.WorkOrderID, &v.DeferredUntil, &v.DeferralReason, &v.ResolutionNotes, &v.ResolvedBy, &v.ResolvedAt, &v.ReportedBy, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// InsertDefect records a defect; a duplicate system source returns
// pgx.ErrNoRows.
func (r *Repository) InsertDefect(ctx context.Context, q database.Querier, org, branchID string, in models.ReportDefectInput, workOrderID, actor string) (models.Defect, error) {
	return scanDefect(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_asset_defects(id,organization_id,branch_id,asset_id,source_type,source_reference,item_code,description,severity,work_order_id,reported_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11)ON CONFLICT DO NOTHING RETURNING `+defectCols, ids.New("dfc"), org, branchID, in.AssetID, in.SourceType, in.SourceReference, in.ItemCode, in.Description, in.Severity, workOrderID, actor))
}

func (r *Repository) InsertDefectHistory(ctx context.Context, q database.Querier, org, defectID, previous, next, actor, reason string, version int64) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_asset_defect_history(id,organization_id,defect_id,previous_status,new_status,actor_id,reason,version)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("dfh"), org, defectID, previous, next, actor, reason, version)
	return e
}

// LinkDefectWorkOrder links a defect to its repair work order.
func (r *Repository) LinkDefectWorkOrder(ctx context.Context, q database.Querier, org string, d *models.Defect, workOrderID string) error {
	return r.q(q).QueryRow(ctx, `UPDATE fleet_partner_asset_defects SET work_order_id=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version,updated_at`, org, d.ID, workOrderID).Scan(&d.Version, &d.UpdatedAt)
}

// LockAssetRow takes the asset row lock without reading it.
func (r *Repository) LockAssetRow(ctx context.Context, q database.Querier, org, id string) error {
	_, e := r.q(q).Exec(ctx, `SELECT id FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id)
	return e
}

// GetDefect reads a defect; lock takes FOR UPDATE.
func (r *Repository) GetDefect(ctx context.Context, q database.Querier, org, id string, lock bool) (models.Defect, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	return scanDefect(r.q(q).QueryRow(ctx, `SELECT `+defectCols+` FROM fleet_partner_asset_defects WHERE organization_id=$1 AND id=$2`+suffix, org, id))
}

func (r *Repository) ListDefects(ctx context.Context, org string, f models.DefectFilter) ([]models.Defect, error) {
	rows, e := r.db.Query(ctx, `SELECT `+defectCols+` FROM fleet_partner_asset_defects WHERE organization_id=$1 AND($2=''OR asset_id=$2)AND($3=''OR status=$3)AND($4=''OR severity=$4) ORDER BY created_at DESC LIMIT 200`, org, f.AssetID, strings.ToUpper(f.Status), strings.ToUpper(f.Severity))
	if e != nil {
		return nil, e
	}
	return collect(rows, scanDefect)
}

// WorkOrderUsable reports a non-cancelled work order of the asset.
func (r *Repository) WorkOrderUsable(ctx context.Context, q database.Querier, org, workOrderID, assetID string) (bool, error) {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_work_orders WHERE organization_id=$1 AND id=$2 AND asset_id=$3 AND status NOT IN('CANCELLED'))`, org, workOrderID, assetID)
}

func (r *Repository) WorkOrderCompleted(ctx context.Context, q database.Querier, org, workOrderID string) (bool, error) {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_work_orders WHERE organization_id=$1 AND id=$2 AND status='COMPLETED')`, org, workOrderID)
}

func (r *Repository) TransitionDefect(ctx context.Context, q database.Querier, org, id string, version int64, to, workOrderID string, d models.DefectAction, reason, actor string) (models.Defect, error) {
	return scanDefect(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_asset_defects SET status=$4,work_order_id=NULLIF($5,''),deferred_until=CASE WHEN $4='DEFERRED' THEN $6 ELSE deferred_until END,deferral_reason=CASE WHEN $4='DEFERRED' THEN $7 ELSE deferral_reason END,resolution_notes=CASE WHEN $4='RESOLVED' THEN $7 ELSE resolution_notes END,resolved_by=CASE WHEN $4='RESOLVED' THEN $8 ELSE resolved_by END,resolved_at=CASE WHEN $4='RESOLVED' THEN now() ELSE resolved_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+defectCols, org, id, version, to, workOrderID, d.DeferredUntil, reason, actor))
}

// UnrepairedCriticalDefects counts unresolved critical defects whose repair
// work order is not completed.
func (r *Repository) UnrepairedCriticalDefects(ctx context.Context, q database.Querier, org, assetID string) (int, error) {
	var n int
	e := r.q(q).QueryRow(ctx, `SELECT count(*) FROM fleet_partner_asset_defects d WHERE d.organization_id=$1 AND d.asset_id=$2 AND d.critical AND d.status<>'RESOLVED' AND NOT EXISTS(SELECT 1 FROM fleet_partner_work_orders w WHERE w.organization_id=d.organization_id AND w.id=d.work_order_id AND w.status='COMPLETED')`, org, assetID).Scan(&n)
	return n, e
}

// ResolveCriticalDefects resolves every unresolved critical defect of the
// asset and returns their ids and versions.
func (r *Repository) ResolveCriticalDefects(ctx context.Context, q database.Querier, org, assetID, notes, actor string) ([]models.ResolvedDefect, error) {
	rows, e := r.q(q).Query(ctx, `UPDATE fleet_partner_asset_defects SET status='RESOLVED',resolution_notes=$3,resolved_by=$4,resolved_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND asset_id=$2 AND critical AND status<>'RESOLVED' RETURNING id,status,version`, org, assetID, notes, actor)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.ResolvedDefect{}
	for rows.Next() {
		var v models.ResolvedDefect
		var status string
		if e = rows.Scan(&v.ID, &status, &v.Version); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) OpenTyreDefectExists(ctx context.Context, q database.Querier, org, itemCode string) (bool, error) {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_defects WHERE organization_id=$1 AND source_type='TYRE_READING' AND item_code=$2 AND status<>'RESOLVED')`, org, itemCode)
}

// ---------------------------------------------------------------------------
// Tyres
// ---------------------------------------------------------------------------

const tyreCols = `id,organization_id,branch_id,serial_number,brand,size,status,COALESCE(asset_id,''),position,installed_at,last_tread_depth_mm,min_tread_depth_mm,version,created_at,updated_at`

func scanTyre(row pgx.Row) (models.Tyre, error) {
	var v models.Tyre
	e := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.SerialNumber, &v.Brand, &v.Size, &v.Status, &v.AssetID, &v.Position, &v.InstalledAt, &v.LastTreadDepthMM, &v.MinTreadDepthMM, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) InsertTyreEvent(ctx context.Context, q database.Querier, org, tyreID, kind, assetID, from, to, reason, actor string, depth, odometer *float64) (string, error) {
	id := ids.New("tye")
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_tyre_events(id,organization_id,tyre_id,event_type,asset_id,from_position,to_position,tread_depth_mm,odometer,reason,actor_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, org, tyreID, kind, assetID, from, to, depth, odometer, reason, actor)
	return id, e
}

func (r *Repository) InsertTyre(ctx context.Context, q database.Querier, org string, in models.RegisterTyreInput, minTread float64) (models.Tyre, error) {
	return scanTyre(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_tyres(id,organization_id,branch_id,serial_number,brand,size,last_tread_depth_mm,min_tread_depth_mm)VALUES($1,$2,$3,$4,$5,$6,$7,$8)RETURNING `+tyreCols, ids.New("tyr"), org, in.BranchID, in.SerialNumber, strings.TrimSpace(in.Brand), strings.TrimSpace(in.Size), in.TreadDepthMM, minTread))
}

// GetTyre reads a tyre; lock takes FOR UPDATE.
func (r *Repository) GetTyre(ctx context.Context, q database.Querier, org, id string, lock bool) (models.Tyre, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	return scanTyre(r.q(q).QueryRow(ctx, `SELECT `+tyreCols+` FROM fleet_partner_tyres WHERE organization_id=$1 AND id=$2`+suffix, org, id))
}

func (r *Repository) ListTyres(ctx context.Context, org, assetID, status string) ([]models.Tyre, error) {
	rows, e := r.db.Query(ctx, `SELECT `+tyreCols+` FROM fleet_partner_tyres WHERE organization_id=$1 AND($2=''OR asset_id=$2)AND($3=''OR status=$3) ORDER BY serial_number LIMIT 500`, org, assetID, strings.ToUpper(status))
	if e != nil {
		return nil, e
	}
	return collect(rows, scanTyre)
}

func (r *Repository) UpdateTyre(ctx context.Context, q database.Querier, org, id string, version int64, status, assetID, position string, depth *float64) (models.Tyre, error) {
	return scanTyre(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_tyres SET status=$4,asset_id=NULLIF($5,''),position=$6,installed_at=CASE WHEN $4='INSTALLED' AND status<>'INSTALLED' THEN now() WHEN $4='INSTALLED' THEN installed_at ELSE NULL END,last_tread_depth_mm=$7,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+tyreCols, org, id, version, status, assetID, position, depth))
}
