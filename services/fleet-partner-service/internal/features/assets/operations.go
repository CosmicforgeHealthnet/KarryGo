package assets

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

func scanSchedule(r pgx.Row) (Schedule, error) {
	var v Schedule
	e := r.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.TriggerType, &v.IntervalValue, &v.NextDueDate, &v.NextDueOdometer, &v.NextDueHours, &v.Critical, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

const scheduleCols = `id,organization_id,asset_id,trigger_type,interval_value,next_due_date,next_due_odometer,next_due_hours,critical,status,version,created_at,updated_at`

func (s *Service) SaveSchedule(ctx context.Context, t tenant.Context, id string, expected int64, v Schedule) (Schedule, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return v, e
	}
	if id == "" {
		if _, e := s.GetAsset(ctx, t, v.AssetID); e != nil {
			return v, e
		}
		v.ID = ids.New("msc")
		v.OrganizationID = t.OrganizationID
		var e error
		v, e = scanSchedule(s.o.Pool.QueryRow(ctx, `INSERT INTO fleet_partner_maintenance_schedules(id,organization_id,asset_id,trigger_type,interval_value,next_due_date,next_due_odometer,next_due_hours,critical)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)RETURNING `+scheduleCols, v.ID, v.OrganizationID, v.AssetID, v.TriggerType, v.IntervalValue, v.NextDueDate, v.NextDueOdometer, v.NextDueHours, v.Critical))
		return v, e
	}
	if _, e := s.GetSchedule(ctx, t, id); e != nil {
		return v, e
	}
	var e error
	v, e = scanSchedule(s.o.Pool.QueryRow(ctx, `UPDATE fleet_partner_maintenance_schedules SET next_due_date=COALESCE($4,next_due_date),next_due_odometer=COALESCE($5,next_due_odometer),next_due_hours=COALESCE($6,next_due_hours),status=COALESCE(NULLIF($7,''),status),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+scheduleCols, t.OrganizationID, id, expected, v.NextDueDate, v.NextDueOdometer, v.NextDueHours, v.Status))
	return v, e
}
func (s *Service) GetSchedule(ctx context.Context, t tenant.Context, id string) (Schedule, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return Schedule{}, e
	}
	v, e := scanSchedule(s.o.Pool.QueryRow(ctx, `SELECT `+scheduleCols+` FROM fleet_partner_maintenance_schedules WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	if _, e = s.GetAsset(ctx, t, v.AssetID); e != nil {
		return v, e
	}
	return v, nil
}
func (s *Service) ListSchedules(ctx context.Context, t tenant.Context) ([]Schedule, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+scheduleCols+` FROM fleet_partner_maintenance_schedules WHERE organization_id=$1 ORDER BY created_at DESC`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Schedule{}
	for rows.Next() {
		v, e := scanSchedule(rows)
		if e != nil {
			return nil, e
		}
		if _, e = s.GetAsset(ctx, t, v.AssetID); e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const workCols = `id,organization_id,asset_id,source_type,source_reference,priority,diagnosis,required_work,status,approved_by,approved_at,started_at,completed_at,downtime_start,downtime_end,evidence_media_ids,return_to_service_status,version,created_at,updated_at`

func scanWork(r pgx.Row) (WorkOrder, error) {
	var v WorkOrder
	e := r.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.SourceType, &v.SourceReference, &v.Priority, &v.Diagnosis, &v.RequiredWork, &v.Status, &v.ApprovedBy, &v.ApprovedAt, &v.StartedAt, &v.CompletedAt, &v.DowntimeStart, &v.DowntimeEnd, &v.EvidenceMediaIDs, &v.ReturnToServiceStatus, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) CreateWorkOrder(ctx context.Context, t tenant.Context, v WorkOrder) (WorkOrder, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return v, e
	}
	if _, e := s.GetAsset(ctx, t, v.AssetID); e != nil {
		return v, e
	}
	v.ID = ids.New("wor")
	v.OrganizationID = t.OrganizationID
	if v.Priority == "" {
		v.Priority = "MEDIUM"
	}
	var e error
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanWork(tx.QueryRow(ctx, `INSERT INTO fleet_partner_work_orders(id,organization_id,asset_id,source_type,source_reference,priority,diagnosis,required_work,status)VALUES($1,$2,$3,$4,$5,$6,$7,$8,'DRAFT')RETURNING `+workCols, v.ID, v.OrganizationID, v.AssetID, v.SourceType, v.SourceReference, v.Priority, v.Diagnosis, v.RequiredWork))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "work_order.created.v1", "work_order", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetWorkOrder(ctx context.Context, t tenant.Context, id string) (WorkOrder, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return WorkOrder{}, e
	}
	v, e := scanWork(s.o.Pool.QueryRow(ctx, `SELECT `+workCols+` FROM fleet_partner_work_orders WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	if _, e = s.GetAsset(ctx, t, v.AssetID); e != nil {
		return v, e
	}
	return v, nil
}
func (s *Service) ListWorkOrders(ctx context.Context, t tenant.Context) ([]WorkOrder, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+workCols+` FROM fleet_partner_work_orders WHERE organization_id=$1 ORDER BY created_at DESC`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []WorkOrder{}
	for rows.Next() {
		v, e := scanWork(rows)
		if e != nil {
			return nil, e
		}
		if _, e = s.GetAsset(ctx, t, v.AssetID); e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type WorkAction struct {
	ExpectedVersion                         int64 `json:"expectedVersion"`
	Diagnosis, RequiredWork, Reason, Status string
	EvidenceMediaIDs                        []string `json:"evidenceMediaIds"`
}

func (s *Service) UpdateWorkOrder(ctx context.Context, t tenant.Context, id, action string, in WorkAction) (WorkOrder, error) {
	old, e := s.GetWorkOrder(ctx, t, id)
	if e != nil {
		return old, e
	}
	if in.ExpectedVersion <= 0 {
		return old, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	to := old.Status
	ev := "work_order.updated.v1"
	switch action {
	case "":
		if in.Status != "" {
			allowed := map[string]map[string]bool{"DRAFT": {"PENDING_APPROVAL": true, "CANCELLED": true}, "PENDING_APPROVAL": {"CANCELLED": true}, "APPROVED": {"IN_PROGRESS": true, "CANCELLED": true}, "IN_PROGRESS": {"CANCELLED": true}}
			if !allowed[old.Status][in.Status] {
				return old, apperrors.Conflict("Invalid work-order transition.", nil)
			}
			to = in.Status
		}
	case "APPROVE":
		if old.Status != "DRAFT" && old.Status != "PENDING_APPROVAL" {
			return old, apperrors.Conflict("Work order is not approvable.", nil)
		}
		to = "APPROVED"
		ev = "work_order.approved.v1"
	case "COMPLETE":
		if old.Status != "APPROVED" && old.Status != "IN_PROGRESS" {
			return old, apperrors.Conflict("Work order is not completable.", nil)
		}
		if in.RequiredWork == "" && old.RequiredWork == "" {
			return old, apperrors.BadRequest("Work summary is required.", nil)
		}
		for _, id := range in.EvidenceMediaIDs {
			if e = s.media(ctx, t, id); e != nil {
				return old, e
			}
		}
		to = "COMPLETED"
		ev = "work_order.completed.v1"
	}
	raw, _ := json.Marshal(in.EvidenceMediaIDs)
	var v WorkOrder
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanWork(tx.QueryRow(ctx, `UPDATE fleet_partner_work_orders SET diagnosis=COALESCE(NULLIF($4,''),diagnosis),required_work=COALESCE(NULLIF($5,''),required_work),status=$6,approved_by=CASE WHEN $6='APPROVED' THEN $7 ELSE approved_by END,approved_at=CASE WHEN $6='APPROVED' THEN now() ELSE approved_at END,completed_at=CASE WHEN $6='COMPLETED' THEN now() ELSE completed_at END,evidence_media_ids=CASE WHEN $8::jsonb<>'[]' THEN $8 ELSE evidence_media_ids END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+workCols, t.OrganizationID, id, in.ExpectedVersion, in.Diagnosis, in.RequiredWork, to, t.OrganizationUserID, raw))
		if x != nil {
			return apperrors.Conflict("Work order version conflict.", x)
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_work_order_history(id,organization_id,work_order_id,previous_status,new_status,actor_id,reason,version)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("woh"), t.OrganizationID, id, old.Status, to, t.ActorID, in.Reason, v.Version)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, ev, "work_order", id, v.Version, v)
	})
	return v, e
}
func (s *Service) ReturnToService(ctx context.Context, t tenant.Context, id string, in WorkAction) (WorkOrder, error) {
	old, e := s.GetWorkOrder(ctx, t, id)
	if e != nil {
		return old, e
	}
	if old.Status != "COMPLETED" || in.ExpectedVersion <= 0 {
		return old, apperrors.Conflict("Completed work order and expected version are required.", nil)
	}
	if s.o.Restrictions != nil {
		restricted, gateErr := s.o.Restrictions.Restricted(ctx, t.OrganizationID, "ASSET", old.AssetID)
		if gateErr != nil {
			return old, apperrors.Unavailable("Restriction authority is unavailable.", gateErr)
		}
		if restricted {
			return old, apperrors.Conflict("Asset has an active compliance restriction.", nil)
		}
	}
	if len(in.EvidenceMediaIDs) == 0 && len(old.EvidenceMediaIDs) <= 2 {
		return old, apperrors.BadRequest("Return-to-service evidence is required.", nil)
	}
	for _, mid := range in.EvidenceMediaIDs {
		if e = s.media(ctx, t, mid); e != nil {
			return old, e
		}
	}
	raw, _ := json.Marshal(in.EvidenceMediaIDs)
	var v WorkOrder
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var unresolved bool
		if x := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_inspections WHERE organization_id=$1 AND asset_id=$2 AND overall_result='CRITICAL_FAIL' AND completed_at>(SELECT completed_at FROM fleet_partner_work_orders WHERE id=$3))`, t.OrganizationID, old.AssetID, id).Scan(&unresolved); x != nil {
			return x
		}
		if unresolved {
			return apperrors.Conflict("A follow-up critical inspection remains unresolved.", nil)
		}
		if old.SourceType == "INSPECTION" {
			var sourceCritical, followupPassed bool
			if x := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_inspections WHERE organization_id=$1 AND id=$2 AND overall_result='CRITICAL_FAIL'),EXISTS(SELECT 1 FROM fleet_partner_inspections WHERE organization_id=$1 AND asset_id=$3 AND id<>$2 AND status='COMPLETED' AND overall_result IN('PASS','PASS_WITH_ADVISORY') AND completed_at>(SELECT completed_at FROM fleet_partner_work_orders WHERE id=$4))`, t.OrganizationID, old.SourceReference, old.AssetID, id).Scan(&sourceCritical, &followupPassed); x != nil {
				return x
			}
			if sourceCritical && !followupPassed {
				return apperrors.Conflict("A passing follow-up inspection is required before return to service.", nil)
			}
		}
		var x error
		v, x = scanWork(tx.QueryRow(ctx, `UPDATE fleet_partner_work_orders SET return_to_service_status='APPROVED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+workCols, t.OrganizationID, id, in.ExpectedVersion))
		if x != nil {
			return apperrors.Conflict("Return-to-service conflict.", x)
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_return_to_service_decisions(id,organization_id,work_order_id,asset_id,decision,reason,evidence_media_ids,decided_by)VALUES($1,$2,$3,$4,'APPROVED',$5,$6,$7)`, ids.New("rts"), t.OrganizationID, id, old.AssetID, in.Reason, raw, t.ActorID)
		if x != nil {
			return x
		}
		var assetVersion int64
		x = tx.QueryRow(ctx, `UPDATE fleet_partner_assets SET operational_status=CASE WHEN EXISTS(SELECT 1 FROM fleet_partner_asset_assignments aa WHERE aa.organization_id=$1 AND aa.asset_id=$2 AND aa.status='ACTIVE')THEN'ASSIGNED'ELSE'AVAILABLE'END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND operational_status='BLOCKED' RETURNING version`, t.OrganizationID, old.AssetID).Scan(&assetVersion)
		if x != nil {
			return x
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_asset_safety_blocks(id,organization_id,asset_id,source_type,source_reference,status,reason,created_by,cleared_by,cleared_at,clear_reason)VALUES($1,$2,$3,'RETURN_TO_SERVICE',$4,'CLEARED','Authorized return to service',$5,$5,now(),$6)`, ids.New("asb"), t.OrganizationID, old.AssetID, id, t.ActorID, in.Reason)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "asset.returned_to_service.v1", "asset", old.AssetID, assetVersion, map[string]any{"assetId": old.AssetID, "workOrderId": id, "returnToServiceStatus": "APPROVED"})
	})
	return v, e
}

func (s *Service) SaveBreakdown(ctx context.Context, t tenant.Context, id string, expected int64, v Breakdown) (Breakdown, error) {
	if e := s.gate(s.o.Flags.Breakdowns, true); e != nil {
		return v, e
	}
	if id == "" {
		if _, e := s.GetAsset(ctx, t, v.AssetID); e != nil {
			return v, e
		}
		v.ID = ids.New("brk")
		v.OrganizationID = t.OrganizationID
		if v.OccurredAt.IsZero() {
			v.OccurredAt = s.o.Now()
		}
		e := s.tx(ctx, func(tx pgx.Tx) error {
			if e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_breakdowns(id,organization_id,asset_id,provider_membership_id,trip_reference,location_reference,occurred_at,severity,trip_impact,rescue_status,towing_reference,replacement_asset_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,COALESCE(NULLIF($10,''),'REQUESTED'),$11,NULLIF($12,''))RETURNING status,version,created_at,updated_at`, v.ID, v.OrganizationID, v.AssetID, v.ProviderMembershipID, v.TripReference, v.LocationReference, v.OccurredAt, v.Severity, v.TripImpact, v.RescueStatus, v.TowingReference, v.ReplacementAssetID).Scan(&v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt); e != nil {
				return e
			}
			priority := "HIGH"
			if v.Severity == "CRITICAL" {
				priority = "CRITICAL"
			}
			if _, e := tx.Exec(ctx, `INSERT INTO fleet_partner_work_orders(id,organization_id,asset_id,source_type,source_reference,priority,required_work)VALUES($1,$2,$3,'BREAKDOWN',$4,$5,'Diagnose and resolve breakdown')ON CONFLICT(organization_id,source_type,source_reference)DO NOTHING`, ids.New("wor"), t.OrganizationID, v.AssetID, v.ID, priority); e != nil {
				return e
			}
			if v.Severity == "HIGH" || v.Severity == "CRITICAL" {
				if _, e := tx.Exec(ctx, `UPDATE fleet_partner_assets SET operational_status='OUT_OF_SERVICE',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, t.OrganizationID, v.AssetID); e != nil {
					return e
				}
			}
			return s.record(ctx, tx, t, "breakdown.created.v1", "breakdown", v.ID, v.Version, map[string]any{"breakdown": v, "automaticProviderPenalty": false, "canonicalTripMutation": false})
		})
		if e == nil && s.o.Alerts != nil {
			_ = s.o.Alerts.CreateSystemAlert(ctx, t.OrganizationID, "ASSET_BREAKDOWN", v.AssetID, v.ID, v.Severity, "Asset breakdown requires rescue, towing or replacement review")
		}
		return v, e
	}
	old, e := s.GetBreakdown(ctx, t, id)
	if e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `UPDATE fleet_partner_breakdowns SET rescue_status=COALESCE(NULLIF($4,''),rescue_status),towing_reference=COALESCE(NULLIF($5,''),towing_reference),replacement_asset_id=COALESCE(NULLIF($6,''),replacement_asset_id),status=COALESCE(NULLIF($7,''),status),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING asset_id,provider_membership_id,trip_reference,location_reference,occurred_at,severity,trip_impact,organization_id,version,created_at,updated_at`, t.OrganizationID, id, expected, v.RescueStatus, v.TowingReference, v.ReplacementAssetID, v.Status).Scan(&v.AssetID, &v.ProviderMembershipID, &v.TripReference, &v.LocationReference, &v.OccurredAt, &v.Severity, &v.TripImpact, &v.OrganizationID, &v.Version, &v.CreatedAt, &v.UpdatedAt); e != nil {
			return apperrors.Conflict("Breakdown version conflict.", e)
		}
		v.ID = id
		ev := "breakdown.updated.v1"
		if v.Status == "RESOLVED" && old.Status != "RESOLVED" {
			ev = "breakdown.resolved.v1"
		}
		return s.record(ctx, tx, t, ev, "breakdown", id, v.Version, map[string]any{"breakdown": v, "automaticProviderPenalty": false, "canonicalTripMutation": false})
	})
	return v, e
}
func (s *Service) GetBreakdown(ctx context.Context, t tenant.Context, id string) (Breakdown, error) {
	if e := s.gate(s.o.Flags.Breakdowns, false); e != nil {
		return Breakdown{}, e
	}
	var v Breakdown
	e := s.o.Pool.QueryRow(ctx, `SELECT id,organization_id,asset_id,provider_membership_id,trip_reference,location_reference,occurred_at,severity,trip_impact,rescue_status,towing_reference,COALESCE(replacement_asset_id,''),status,version,created_at,updated_at FROM fleet_partner_breakdowns WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id).Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.ProviderMembershipID, &v.TripReference, &v.LocationReference, &v.OccurredAt, &v.Severity, &v.TripImpact, &v.RescueStatus, &v.TowingReference, &v.ReplacementAssetID, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if e == nil {
		_, e = s.GetAsset(ctx, t, v.AssetID)
	}
	return v, e
}
func (s *Service) ListBreakdowns(ctx context.Context, t tenant.Context) ([]Breakdown, error) {
	if e := s.gate(s.o.Flags.Breakdowns, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id FROM fleet_partner_breakdowns WHERE organization_id=$1 ORDER BY created_at DESC`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Breakdown{}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		v, e := s.GetBreakdown(ctx, t, id)
		if e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) SavePart(ctx context.Context, t tenant.Context, id string, expected int64, v Part) (Part, error) {
	if e := s.gate(s.o.Flags.Inventory, true); e != nil {
		return v, e
	}
	if id == "" {
		if !t.BranchPermitted(v.BranchID) {
			return v, apperrors.NotFound("Resource not found.", nil)
		}
		v.ID = ids.New("prt")
		v.OrganizationID = t.OrganizationID
		e := s.o.Pool.QueryRow(ctx, `INSERT INTO fleet_partner_parts(id,organization_id,branch_id,sku,name,stock_unit,reorder_level,warranty_information)VALUES($1,$2,$3,$4,$5,$6,$7,$8)RETURNING current_quantity_projection,status,version,created_at,updated_at`, v.ID, v.OrganizationID, v.BranchID, v.SKU, v.Name, v.StockUnit, v.ReorderLevel, v.WarrantyInformation).Scan(&v.CurrentQuantityProjection, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
		return v, e
	}
	if _, e := s.GetPart(ctx, t, id); e != nil {
		return v, e
	}
	e := s.o.Pool.QueryRow(ctx, `UPDATE fleet_partner_parts SET name=COALESCE(NULLIF($4,''),name),reorder_level=CASE WHEN $5>=0 THEN $5 ELSE reorder_level END,status=COALESCE(NULLIF($6,''),status),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING branch_id,sku,stock_unit,current_quantity_projection,warranty_information,organization_id,version,created_at,updated_at`, t.OrganizationID, id, expected, v.Name, v.ReorderLevel, v.Status).Scan(&v.BranchID, &v.SKU, &v.StockUnit, &v.CurrentQuantityProjection, &v.WarrantyInformation, &v.OrganizationID, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	v.ID = id
	return v, e
}
func (s *Service) GetPart(ctx context.Context, t tenant.Context, id string) (Part, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return Part{}, e
	}
	var v Part
	e := s.o.Pool.QueryRow(ctx, `SELECT id,organization_id,branch_id,sku,name,stock_unit,current_quantity_projection,reorder_level,warranty_information,status,version,created_at,updated_at FROM fleet_partner_parts WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id).Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.SKU, &v.Name, &v.StockUnit, &v.CurrentQuantityProjection, &v.ReorderLevel, &v.WarrantyInformation, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if e == nil && !t.BranchPermitted(v.BranchID) {
		return v, apperrors.NotFound("Resource not found.", nil)
	}
	return v, e
}
func (s *Service) ListParts(ctx context.Context, t tenant.Context) ([]Part, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id FROM fleet_partner_parts WHERE organization_id=$1 ORDER BY sku`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Part{}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		v, e := s.GetPart(ctx, t, id)
		if e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) CreateMovement(ctx context.Context, t tenant.Context, v StockMovement) (StockMovement, error) {
	if e := s.gate(s.o.Flags.Inventory, true); e != nil {
		return v, e
	}
	if v.Quantity <= 0 || v.Reason == "" {
		return v, apperrors.BadRequest("Positive quantity and reason are required.", nil)
	}
	if _, e := s.GetPart(ctx, t, v.PartID); e != nil {
		return v, e
	}
	if v.WorkOrderID != "" {
		if _, e := s.GetWorkOrder(ctx, t, v.WorkOrderID); e != nil {
			return v, e
		}
	}
	v.ID = ids.New("stm")
	v.OrganizationID = t.OrganizationID
	v.CreatedBy = t.ActorID
	if v.CorrelationID == "" {
		v.CorrelationID = t.CorrelationID
	}
	sign := 1.0
	if v.MovementType == "ISSUE" || v.MovementType == "TRANSFER_OUT" {
		sign = -1
	}
	reorderRequired := false
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var q, reorder float64
		if x := tx.QueryRow(ctx, `SELECT current_quantity_projection,reorder_level FROM fleet_partner_parts WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, v.PartID).Scan(&q, &reorder); x != nil {
			return x
		}
		projected := q + sign*v.Quantity
		if projected < 0 {
			return apperrors.Conflict("Stock cannot become negative.", nil)
		}
		var partVersion int64
		if x := tx.QueryRow(ctx, `UPDATE fleet_partner_parts SET current_quantity_projection=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, v.PartID, projected).Scan(&partVersion); x != nil {
			return x
		}
		if x := tx.QueryRow(ctx, `INSERT INTO fleet_partner_stock_movements(id,organization_id,part_id,location_reference,movement_type,quantity,reason,work_order_id,correlation_id,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10)RETURNING created_at`, v.ID, v.OrganizationID, v.PartID, v.LocationReference, v.MovementType, v.Quantity, v.Reason, v.WorkOrderID, v.CorrelationID, v.CreatedBy).Scan(&v.CreatedAt); x != nil {
			return x
		}
		if x := s.record(ctx, tx, t, "stock_movement.recorded.v1", "stock_movement", v.ID, 1, v); x != nil {
			return x
		}
		reorderRequired = q > reorder && projected <= reorder
		if reorderRequired {
			return s.record(ctx, tx, t, "part.reorder_required.v1", "part", v.PartID, partVersion, map[string]any{"partId": v.PartID, "stockMovementId": v.ID, "projectedQuantity": projected, "reorderLevel": reorder})
		}
		return nil
	})
	if e == nil && reorderRequired && s.o.Alerts != nil {
		_ = s.o.Alerts.CreateSystemAlert(ctx, t.OrganizationID, "PART_REORDER_REQUIRED", v.PartID, v.ID, "MEDIUM", "Part stock is at or below reorder level")
	}
	return v, e
}
func (s *Service) ListMovements(ctx context.Context, t tenant.Context) ([]StockMovement, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,part_id,location_reference,movement_type,quantity,reason,COALESCE(work_order_id,''),correlation_id,created_by,created_at FROM fleet_partner_stock_movements WHERE organization_id=$1 ORDER BY created_at DESC`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []StockMovement{}
	for rows.Next() {
		var v StockMovement
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.PartID, &v.LocationReference, &v.MovementType, &v.Quantity, &v.Reason, &v.WorkOrderID, &v.CorrelationID, &v.CreatedBy, &v.CreatedAt); e != nil {
			return nil, e
		}
		if _, e = s.GetPart(ctx, t, v.PartID); e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) CreateFuel(ctx context.Context, t tenant.Context, v FuelRecord) (FuelRecord, error) {
	if e := s.gate(s.o.Flags.Fuel, true); e != nil {
		return v, e
	}
	valid := map[string]map[string]bool{"PETROL": {"L": true, "LITRE": true}, "DIESEL": {"L": true, "LITRE": true}, "ELECTRIC": {"KWH": true}, "GAS": {"KG": true, "L": true}, "OTHER": {"UNIT": true}}
	v.FuelOrEnergyType = strings.ToUpper(v.FuelOrEnergyType)
	v.Unit = strings.ToUpper(v.Unit)
	if v.Quantity <= 0 || !valid[v.FuelOrEnergyType][v.Unit] {
		return v, apperrors.BadRequest("Fuel quantity, type or unit is invalid.", nil)
	}
	a, e := s.GetAsset(ctx, t, v.AssetID)
	if e != nil {
		return v, e
	}
	if v.Odometer != nil && *v.Odometer < a.Odometer || v.OperatingHours != nil && *v.OperatingHours < a.OperatingHours {
		return v, apperrors.Conflict("Odometer or operating hours cannot regress.", nil)
	}
	if v.ReceiptMediaID != "" {
		if e = s.media(ctx, t, v.ReceiptMediaID); e != nil {
			return v, e
		}
	}
	v.ID = ids.New("fur")
	v.OrganizationID = t.OrganizationID
	if v.RecordedAt.IsZero() {
		v.RecordedAt = s.o.Now()
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_fuel_records(id,organization_id,asset_id,provider_membership_id,fuel_or_energy_type,quantity,unit,odometer,operating_hours,receipt_media_id,supplier_reference,recorded_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)RETURNING approval_status,version,created_at,updated_at`, v.ID, v.OrganizationID, v.AssetID, v.ProviderMembershipID, v.FuelOrEnergyType, v.Quantity, v.Unit, v.Odometer, v.OperatingHours, v.ReceiptMediaID, v.SupplierReference, v.RecordedAt).Scan(&v.ApprovalStatus, &v.Version, &v.CreatedAt, &v.UpdatedAt); e != nil {
			return e
		}
		return s.record(ctx, tx, t, "fuel_record.created.v1", "fuel_record", v.ID, v.Version, map[string]any{"fuelRecord": v, "walletMutation": false, "ledgerMutation": false, "automaticProviderPenalty": false})
	})
	return v, e
}
func (s *Service) GetFuel(ctx context.Context, t tenant.Context, id string) (FuelRecord, error) {
	if e := s.gate(s.o.Flags.Fuel, false); e != nil {
		return FuelRecord{}, e
	}
	var v FuelRecord
	e := s.o.Pool.QueryRow(ctx, `SELECT id,organization_id,asset_id,provider_membership_id,fuel_or_energy_type,quantity,unit,odometer,operating_hours,receipt_media_id,supplier_reference,recorded_at,approval_status,approved_by,version,created_at,updated_at FROM fleet_partner_fuel_records WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id).Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.ProviderMembershipID, &v.FuelOrEnergyType, &v.Quantity, &v.Unit, &v.Odometer, &v.OperatingHours, &v.ReceiptMediaID, &v.SupplierReference, &v.RecordedAt, &v.ApprovalStatus, &v.ApprovedBy, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if e == nil {
		_, e = s.GetAsset(ctx, t, v.AssetID)
	}
	return v, e
}
func (s *Service) ListFuel(ctx context.Context, t tenant.Context) ([]FuelRecord, error) {
	if e := s.gate(s.o.Flags.Fuel, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id FROM fleet_partner_fuel_records WHERE organization_id=$1 ORDER BY recorded_at DESC`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []FuelRecord{}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		v, e := s.GetFuel(ctx, t, id)
		if e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) FuelAction(ctx context.Context, t tenant.Context, id, to, reason string, expected int64) (FuelRecord, error) {
	old, e := s.GetFuel(ctx, t, id)
	if e != nil {
		return old, e
	}
	if old.ApprovalStatus != "PENDING" || expected <= 0 {
		return old, apperrors.Conflict("Fuel record is already decided or version is missing.", nil)
	}
	var v FuelRecord
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.scanFuelTx(ctx, tx, t.OrganizationID, id, expected, to, t.OrganizationUserID)
		if x != nil {
			return apperrors.Conflict("Fuel approval conflict.", x)
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_fuel_approval_history(id,organization_id,fuel_record_id,previous_status,new_status,actor_id,reason,version)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("fah"), t.OrganizationID, id, old.ApprovalStatus, to, t.ActorID, reason, v.Version)
		if x != nil {
			return x
		}
		ev := "fuel_record." + strings.ToLower(to) + ".v1"
		return s.record(ctx, tx, t, ev, "fuel_record", id, v.Version, v)
	})
	return v, e
}
func (s *Service) scanFuelTx(ctx context.Context, tx pgx.Tx, org, id string, expected int64, to, actor string) (FuelRecord, error) {
	var v FuelRecord
	e := tx.QueryRow(ctx, `UPDATE fleet_partner_fuel_records SET approval_status=$4,approved_by=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND approval_status='PENDING' RETURNING id,organization_id,asset_id,provider_membership_id,fuel_or_energy_type,quantity,unit,odometer,operating_hours,receipt_media_id,supplier_reference,recorded_at,approval_status,approved_by,version,created_at,updated_at`, org, id, expected, to, actor).Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.ProviderMembershipID, &v.FuelOrEnergyType, &v.Quantity, &v.Unit, &v.Odometer, &v.OperatingHours, &v.ReceiptMediaID, &v.SupplierReference, &v.RecordedAt, &v.ApprovalStatus, &v.ApprovedBy, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (s *Service) RunMaintenance(ctx context.Context, organizationID string) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	if _, e := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_asset_documents SET verification_status='EXPIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND verification_status IN('PENDING','VERIFIED') AND expires_at<=now()`, organizationID); e != nil {
		return e
	}
	if _, e := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_asset_assignments SET status='COMPLETED',version=version+1,updated_at=now() WHERE organization_id=$1 AND status IN('PLANNED','ACTIVE') AND end_at<=now()`, organizationID); e != nil {
		return e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT s.id,s.asset_id,s.critical FROM fleet_partner_maintenance_schedules s JOIN fleet_partner_assets a ON a.id=s.asset_id WHERE s.organization_id=$1 AND s.status='ACTIVE' AND((s.next_due_date IS NOT NULL AND s.next_due_date<=CURRENT_DATE)OR(s.next_due_odometer IS NOT NULL AND s.next_due_odometer<=a.odometer)OR(s.next_due_hours IS NOT NULL AND s.next_due_hours<=a.operating_hours))`, organizationID)
	if e != nil {
		return e
	}
	type due struct {
		id, asset string
		critical  bool
	}
	items := []due{}
	for rows.Next() {
		var v due
		if e = rows.Scan(&v.id, &v.asset, &v.critical); e != nil {
			rows.Close()
			return e
		}
		items = append(items, v)
	}
	rows.Close()
	for _, v := range items {
		scope := tenant.Context{OrganizationID: organizationID, ActorType: tenant.ActorTypeSystem, ActorID: "section18-maintenance-worker", CorrelationID: v.id}
		if e = s.tx(ctx, func(tx pgx.Tx) error {
			workID := ids.New("wor")
			priority := "MEDIUM"
			if v.critical {
				priority = "CRITICAL"
			}
			var version int64
			x := tx.QueryRow(ctx, `INSERT INTO fleet_partner_work_orders(id,organization_id,asset_id,source_type,source_reference,priority,required_work)VALUES($1,$2,$3,'MAINTENANCE_SCHEDULE',$4,$5,'Perform due preventive maintenance')ON CONFLICT(organization_id,source_type,source_reference)DO NOTHING RETURNING version`, workID, organizationID, v.asset, v.id, priority).Scan(&version)
			if x != nil && x != pgx.ErrNoRows {
				return x
			}
			if x == nil {
				if x = s.record(ctx, tx, scope, "maintenance_schedule.due.v1", "maintenance_schedule", v.id, 1, map[string]any{"scheduleId": v.id, "assetId": v.asset}); x != nil {
					return x
				}
				if x = s.record(ctx, tx, scope, "work_order.created.v1", "work_order", workID, version, map[string]any{"workOrderId": workID, "scheduleId": v.id, "assetId": v.asset}); x != nil {
					return x
				}
			}
			if v.critical {
				var assetVersion int64
				x = tx.QueryRow(ctx, `UPDATE fleet_partner_assets SET operational_status='BLOCKED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND lifecycle_status='ACTIVE' AND operational_status<>'BLOCKED' RETURNING version`, organizationID, v.asset).Scan(&assetVersion)
				if x != nil && x != pgx.ErrNoRows {
					return x
				}
				if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_asset_safety_blocks(id,organization_id,asset_id,source_type,source_reference,status,reason,created_by)VALUES($1,$2,$3,'MAINTENANCE_SCHEDULE',$4,'ACTIVE','Critical maintenance overdue','section18-maintenance-worker')ON CONFLICT(organization_id,source_type,source_reference)DO NOTHING`, ids.New("asb"), organizationID, v.asset, v.id); x != nil {
					return x
				}
				if assetVersion > 0 {
					if x = s.record(ctx, tx, scope, "asset.blocked.v1", "asset", v.asset, assetVersion, map[string]any{"assetId": v.asset, "scheduleId": v.id, "operationalStatus": "BLOCKED"}); x != nil {
						return x
					}
				}
			}
			return nil
		}); e != nil {
			return e
		}
		if v.critical && s.o.Alerts != nil {
			_ = s.o.Alerts.CreateSystemAlert(ctx, organizationID, "CRITICAL_MAINTENANCE_OVERDUE", v.asset, v.id, "HIGH", "Critical preventive maintenance is overdue")
		}
	}
	if s.o.Alerts != nil {
		parts, e := s.o.Pool.Query(ctx, `SELECT id FROM fleet_partner_parts WHERE organization_id=$1 AND status='ACTIVE' AND current_quantity_projection<=reorder_level`, organizationID)
		if e != nil {
			return e
		}
		for parts.Next() {
			var id string
			if e = parts.Scan(&id); e != nil {
				parts.Close()
				return e
			}
			_ = s.o.Alerts.CreateSystemAlert(ctx, organizationID, "PART_REORDER_REQUIRED", id, id, "MEDIUM", "Part stock is at or below reorder level")
		}
		parts.Close()
	}
	return nil
}

var _ = time.Second
