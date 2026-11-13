package assets

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Asset defects (spec 48.2, 48.3). Lifecycle OPEN -> DEFERRED|REPAIRING ->
// RESOLVED. A CRITICAL defect blocks the asset in the same transaction, can
// never be deferred, can only be resolved once its repair work order is
// complete, and never unblocks the asset by itself: only an authorized
// return-to-service decision does.
const (
	DefectOpen      = "OPEN"
	DefectDeferred  = "DEFERRED"
	DefectRepairing = "REPAIRING"
	DefectResolved  = "RESOLVED"
)

type Defect struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organizationId"`
	BranchID        string     `json:"branchId"`
	AssetID         string     `json:"assetId"`
	SourceType      string     `json:"sourceType"`
	SourceReference string     `json:"sourceReference,omitempty"`
	ItemCode        string     `json:"itemCode,omitempty"`
	Description     string     `json:"description"`
	Severity        string     `json:"severity"`
	Critical        bool       `json:"critical"`
	Status          string     `json:"status"`
	WorkOrderID     string     `json:"workOrderId,omitempty"`
	DeferredUntil   *time.Time `json:"deferredUntil,omitempty"`
	DeferralReason  string     `json:"deferralReason,omitempty"`
	ResolutionNotes string     `json:"resolutionNotes,omitempty"`
	ResolvedBy      string     `json:"resolvedBy,omitempty"`
	ResolvedAt      *time.Time `json:"resolvedAt,omitempty"`
	ReportedBy      string     `json:"reportedBy"`
	Version         int64      `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type ReportDefectInput struct {
	AssetID         string `json:"assetId"`
	SourceType      string `json:"sourceType"`
	SourceReference string `json:"sourceReference"`
	ItemCode        string `json:"itemCode"`
	Description     string `json:"description"`
	Severity        string `json:"severity"`
}

type DefectAction struct {
	ExpectedVersion int64      `json:"expectedVersion"`
	Reason          string     `json:"reason"`
	Notes           string     `json:"notes"`
	DeferredUntil   *time.Time `json:"deferredUntil"`
	WorkOrderID     string     `json:"workOrderId"`
}

type DefectFilter struct{ AssetID, Status, Severity string }

const defectCols = `id,organization_id,branch_id,asset_id,source_type,source_reference,item_code,description,severity,critical,status,COALESCE(work_order_id,''),deferred_until,deferral_reason,resolution_notes,resolved_by,resolved_at,reported_by,version,created_at,updated_at`

func scanDefect(r pgx.Row) (Defect, error) {
	var v Defect
	e := r.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.AssetID, &v.SourceType, &v.SourceReference, &v.ItemCode, &v.Description, &v.Severity, &v.Critical, &v.Status, &v.WorkOrderID, &v.DeferredUntil, &v.DeferralReason, &v.ResolutionNotes, &v.ResolvedBy, &v.ResolvedAt, &v.ReportedBy, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

var defectSeverities = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}

// insertDefectTx records a defect inside the caller's transaction. System
// sources (inspection items, safety declines, tyre readings) are deduplicated
// per source and item; inserted reports false for a duplicate.
func (s *Service) insertDefectTx(ctx context.Context, tx pgx.Tx, t tenant.Context, branchID string, in ReportDefectInput, workOrderID string) (Defect, bool, error) {
	v, e := scanDefect(tx.QueryRow(ctx, `INSERT INTO fleet_partner_asset_defects(id,organization_id,branch_id,asset_id,source_type,source_reference,item_code,description,severity,work_order_id,reported_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11)ON CONFLICT DO NOTHING RETURNING `+defectCols, ids.New("dfc"), t.OrganizationID, branchID, in.AssetID, in.SourceType, in.SourceReference, in.ItemCode, in.Description, in.Severity, workOrderID, t.ActorID))
	if errors.Is(e, pgx.ErrNoRows) {
		return Defect{}, false, nil
	}
	if e != nil {
		return Defect{}, false, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO fleet_partner_asset_defect_history(id,organization_id,defect_id,previous_status,new_status,actor_id,reason,version)VALUES($1,$2,$3,'',$4,$5,'REPORTED',$6)`, ids.New("dfh"), t.OrganizationID, v.ID, v.Status, t.ActorID, v.Version); e != nil {
		return v, true, e
	}
	return v, true, s.record(ctx, tx, t, "defect.reported.v1", "defect", v.ID, v.Version, map[string]any{"defectId": v.ID, "assetId": v.AssetID, "severity": v.Severity, "critical": v.Critical, "sourceType": v.SourceType, "sourceReference": v.SourceReference})
}

// blockAssetForDefectTx applies the operational restriction a critical defect
// requires: a repair work order, a safety block and BLOCKED status, all in the
// caller's transaction. Returns the repair work-order id.
func (s *Service) blockAssetForDefectTx(ctx context.Context, tx pgx.Tx, t tenant.Context, d *Defect) error {
	workID := ids.New("wor")
	var workVersion int64
	if x := tx.QueryRow(ctx, `INSERT INTO fleet_partner_work_orders(id,organization_id,asset_id,source_type,source_reference,priority,required_work)VALUES($1,$2,$3,'DEFECT',$4,'CRITICAL',$5)RETURNING version`, workID, t.OrganizationID, d.AssetID, d.ID, "Repair critical defect: "+d.Description).Scan(&workVersion); x != nil {
		return x
	}
	if x := tx.QueryRow(ctx, `UPDATE fleet_partner_asset_defects SET work_order_id=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version,updated_at`, t.OrganizationID, d.ID, workID).Scan(&d.Version, &d.UpdatedAt); x != nil {
		return x
	}
	d.WorkOrderID = workID
	if x := s.record(ctx, tx, t, "work_order.created.v1", "work_order", workID, workVersion, map[string]any{"workOrderId": workID, "assetId": d.AssetID, "defectId": d.ID}); x != nil {
		return x
	}
	if _, x := tx.Exec(ctx, `INSERT INTO fleet_partner_asset_safety_blocks(id,organization_id,asset_id,source_type,source_reference,status,reason,created_by)VALUES($1,$2,$3,'DEFECT',$4,'ACTIVE','Critical defect reported',$5)`, ids.New("asb"), t.OrganizationID, d.AssetID, d.ID, t.ActorID); x != nil {
		return x
	}
	var previous, lifecycle string
	if x := tx.QueryRow(ctx, `SELECT operational_status,lifecycle_status FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2`, t.OrganizationID, d.AssetID).Scan(&previous, &lifecycle); x != nil {
		return x
	}
	if lifecycle != "ACTIVE" || previous == "BLOCKED" {
		return nil
	}
	var assetVersion int64
	if x := tx.QueryRow(ctx, `UPDATE fleet_partner_assets SET operational_status='BLOCKED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, d.AssetID).Scan(&assetVersion); x != nil {
		return x
	}
	if x := s.assetHistory(ctx, tx, t, d.AssetID, previous, "BLOCKED", lifecycle, "CRITICAL_DEFECT", assetVersion, map[string]any{"defectId": d.ID}); x != nil {
		return x
	}
	return s.record(ctx, tx, t, "asset.blocked.v1", "asset", d.AssetID, assetVersion, map[string]any{"assetId": d.AssetID, "defectId": d.ID, "operationalStatus": "BLOCKED"})
}

// ReportDefect records a manually or provider-reported defect. Inspection,
// safety-decline and tyre-reading defects are created only by those flows.
func (s *Service) ReportDefect(ctx context.Context, t tenant.Context, in ReportDefectInput) (Defect, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return Defect{}, e
	}
	in.SourceType = strings.ToUpper(strings.TrimSpace(in.SourceType))
	if in.SourceType == "" {
		in.SourceType = "MANUAL_REPORT"
	}
	in.Severity = strings.ToUpper(strings.TrimSpace(in.Severity))
	in.Description = strings.TrimSpace(in.Description)
	if (in.SourceType != "MANUAL_REPORT" && in.SourceType != "PROVIDER_REPORT") || !defectSeverities[in.Severity] || in.Description == "" || len(in.Description) > 2000 {
		return Defect{}, apperrors.BadRequest("sourceType, severity or description is invalid.", nil)
	}
	a, e := s.GetAsset(ctx, t, in.AssetID)
	if e != nil {
		return Defect{}, e
	}
	var out Defect
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if _, x := tx.Exec(ctx, `SELECT id FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, in.AssetID); x != nil {
			return x
		}
		var x error
		out, _, x = s.insertDefectTx(ctx, tx, t, a.BranchID, in, "")
		if x != nil {
			return x
		}
		if out.Critical {
			return s.blockAssetForDefectTx(ctx, tx, t, &out)
		}
		return nil
	})
	if e == nil && out.Critical && s.o.Alerts != nil {
		_ = s.o.Alerts.CreateSystemAlert(ctx, t.OrganizationID, "CRITICAL_DEFECT", in.AssetID, out.ID, "CRITICAL", "Critical defect reported; asset blocked")
	}
	return out, e
}

func (s *Service) GetDefect(ctx context.Context, t tenant.Context, id string) (Defect, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return Defect{}, e
	}
	v, e := scanDefect(s.o.Pool.QueryRow(ctx, `SELECT `+defectCols+` FROM fleet_partner_asset_defects WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return Defect{}, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}

func (s *Service) ListDefects(ctx context.Context, t tenant.Context, f DefectFilter) ([]Defect, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+defectCols+` FROM fleet_partner_asset_defects WHERE organization_id=$1 AND($2=''OR asset_id=$2)AND($3=''OR status=$3)AND($4=''OR severity=$4) ORDER BY created_at DESC LIMIT 200`, t.OrganizationID, f.AssetID, strings.ToUpper(f.Status), strings.ToUpper(f.Severity))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Defect{}
	for rows.Next() {
		v, e := scanDefect(rows)
		if e != nil {
			return nil, e
		}
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}

// DefectTransition moves a defect through DEFER, START_REPAIR or RESOLVE.
func (s *Service) DefectTransition(ctx context.Context, t tenant.Context, id, action string, in DefectAction) (Defect, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return Defect{}, e
	}
	if in.ExpectedVersion <= 0 {
		return Defect{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	old, e := s.GetDefect(ctx, t, id)
	if e != nil {
		return Defect{}, e
	}
	var out Defect
	e = s.tx(ctx, func(tx pgx.Tx) error {
		cur, x := scanDefect(tx.QueryRow(ctx, `SELECT `+defectCols+` FROM fleet_partner_asset_defects WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id))
		if x != nil {
			return x
		}
		if cur.Version != in.ExpectedVersion {
			return apperrors.Conflict("Defect version conflict.", nil)
		}
		to, event, reason := "", "", ""
		workOrder := cur.WorkOrderID
		switch action {
		case "DEFER":
			if cur.Critical {
				return apperrors.Conflict("A critical defect cannot be deferred.", nil)
			}
			if cur.Status != DefectOpen {
				return apperrors.Conflict("Only an open defect can be deferred.", nil)
			}
			reason = strings.TrimSpace(in.Reason)
			if reason == "" || in.DeferredUntil == nil || !in.DeferredUntil.After(s.o.Now()) {
				return apperrors.BadRequest("reason and a future deferredUntil are required.", nil)
			}
			to, event = DefectDeferred, "defect.deferred.v1"
		case "START_REPAIR":
			if cur.Status != DefectOpen && cur.Status != DefectDeferred {
				return apperrors.Conflict("Only an open or deferred defect can enter repair.", nil)
			}
			if in.WorkOrderID != "" {
				var ok bool
				if x = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_work_orders WHERE organization_id=$1 AND id=$2 AND asset_id=$3 AND status NOT IN('CANCELLED'))`, t.OrganizationID, in.WorkOrderID, cur.AssetID).Scan(&ok); x != nil {
					return x
				}
				if !ok {
					return apperrors.NotFound("Work order not found for this asset.", nil)
				}
				workOrder = in.WorkOrderID
			}
			if workOrder == "" {
				return apperrors.BadRequest("A repair work order is required.", nil)
			}
			reason = strings.TrimSpace(in.Reason)
			to, event = DefectRepairing, "defect.repair_started.v1"
		case "RESOLVE":
			if cur.Status == DefectResolved {
				return apperrors.Conflict("Defect is already resolved.", nil)
			}
			reason = strings.TrimSpace(in.Notes)
			if reason == "" {
				return apperrors.BadRequest("Resolution notes are required.", nil)
			}
			if cur.Critical {
				var completed bool
				if workOrder != "" {
					if x = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_work_orders WHERE organization_id=$1 AND id=$2 AND status='COMPLETED')`, t.OrganizationID, workOrder).Scan(&completed); x != nil {
						return x
					}
				}
				if !completed {
					return apperrors.Conflict("A critical defect can only be resolved after its repair work order is completed.", nil)
				}
			}
			to, event = DefectResolved, "defect.resolved.v1"
		default:
			return apperrors.NotFound("Operation not found.", nil)
		}
		out, x = scanDefect(tx.QueryRow(ctx, `UPDATE fleet_partner_asset_defects SET status=$4,work_order_id=NULLIF($5,''),deferred_until=CASE WHEN $4='DEFERRED' THEN $6 ELSE deferred_until END,deferral_reason=CASE WHEN $4='DEFERRED' THEN $7 ELSE deferral_reason END,resolution_notes=CASE WHEN $4='RESOLVED' THEN $7 ELSE resolution_notes END,resolved_by=CASE WHEN $4='RESOLVED' THEN $8 ELSE resolved_by END,resolved_at=CASE WHEN $4='RESOLVED' THEN now() ELSE resolved_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+defectCols, t.OrganizationID, id, cur.Version, to, workOrder, in.DeferredUntil, reason, t.ActorID))
		if x != nil {
			return apperrors.Conflict("Defect version conflict.", x)
		}
		if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_asset_defect_history(id,organization_id,defect_id,previous_status,new_status,actor_id,reason,version)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("dfh"), t.OrganizationID, id, cur.Status, to, t.ActorID, reason, out.Version); x != nil {
			return x
		}
		return s.record(ctx, tx, t, event, "defect", id, out.Version, map[string]any{"defectId": id, "assetId": out.AssetID, "status": to, "critical": out.Critical, "assetReleased": false})
	})
	_ = old
	return out, e
}

// resolveRepairedCriticalDefectsTx is the return-to-service side of the
// defect lifecycle: it refuses while any critical defect lacks a completed
// repair, then resolves the critical defects the authorized decision covers.
func (s *Service) resolveRepairedCriticalDefectsTx(ctx context.Context, tx pgx.Tx, t tenant.Context, assetID, workOrderID string) error {
	var unrepaired int
	if x := tx.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_asset_defects d WHERE d.organization_id=$1 AND d.asset_id=$2 AND d.critical AND d.status<>'RESOLVED' AND NOT EXISTS(SELECT 1 FROM fleet_partner_work_orders w WHERE w.organization_id=d.organization_id AND w.id=d.work_order_id AND w.status='COMPLETED')`, t.OrganizationID, assetID).Scan(&unrepaired); x != nil {
		return x
	}
	if unrepaired > 0 {
		return apperrors.Conflict("Every critical defect on the asset requires a completed repair work order before return to service.", nil)
	}
	rows, x := tx.Query(ctx, `UPDATE fleet_partner_asset_defects SET status='RESOLVED',resolution_notes=$3,resolved_by=$4,resolved_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND asset_id=$2 AND critical AND status<>'RESOLVED' RETURNING id,status,version`, t.OrganizationID, assetID, "Released by authorized return to service of work order "+workOrderID, t.ActorID)
	if x != nil {
		return x
	}
	type resolved struct {
		id      string
		version int64
	}
	items := []resolved{}
	for rows.Next() {
		var r resolved
		var status string
		if x = rows.Scan(&r.id, &status, &r.version); x != nil {
			rows.Close()
			return x
		}
		items = append(items, r)
	}
	rows.Close()
	if x = rows.Err(); x != nil {
		return x
	}
	for _, r := range items {
		if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_asset_defect_history(id,organization_id,defect_id,previous_status,new_status,actor_id,reason,version)VALUES($1,$2,$3,'UNRESOLVED','RESOLVED',$4,'RETURN_TO_SERVICE',$5)`, ids.New("dfh"), t.OrganizationID, r.id, t.ActorID, r.version); x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, "defect.resolved.v1", "defect", r.id, r.version, map[string]any{"defectId": r.id, "assetId": assetID, "workOrderId": workOrderID, "returnToService": true}); x != nil {
			return x
		}
	}
	return nil
}

// activeSafetyBlocksCoveredTx enforces that every safety block currently in
// force on the asset (created after the last authorized clearance) has its
// own repair work order completed. Blocks and their work orders share the
// same (source_type, source_reference), so an unrelated work order can never
// be used to release an asset.
func (s *Service) activeSafetyBlocksCoveredTx(ctx context.Context, tx pgx.Tx, t tenant.Context, assetID string) error {
	var uncovered int
	if x := tx.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_asset_safety_blocks b
WHERE b.organization_id=$1 AND b.asset_id=$2 AND b.status='ACTIVE'
AND b.created_at>COALESCE((SELECT max(c.created_at) FROM fleet_partner_asset_safety_blocks c WHERE c.organization_id=$1 AND c.asset_id=$2 AND c.status='CLEARED'),'-infinity'::timestamptz)
AND NOT EXISTS(SELECT 1 FROM fleet_partner_work_orders w WHERE w.organization_id=b.organization_id AND w.asset_id=b.asset_id AND w.source_type=b.source_type AND w.source_reference=b.source_reference AND w.status='COMPLETED')`, t.OrganizationID, assetID).Scan(&uncovered); x != nil {
		return x
	}
	if uncovered > 0 {
		return apperrors.Conflict("Every active safety block requires its own completed work order before return to service.", nil)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tyres (spec 48.3 GET/POST /tyres)
// ---------------------------------------------------------------------------

type Tyre struct {
	ID               string     `json:"id"`
	OrganizationID   string     `json:"organizationId"`
	BranchID         string     `json:"branchId"`
	SerialNumber     string     `json:"serialNumber"`
	Brand            string     `json:"brand,omitempty"`
	Size             string     `json:"size,omitempty"`
	Status           string     `json:"status"`
	AssetID          string     `json:"assetId,omitempty"`
	Position         string     `json:"position,omitempty"`
	InstalledAt      *time.Time `json:"installedAt,omitempty"`
	LastTreadDepthMM *float64   `json:"lastTreadDepthMm,omitempty"`
	MinTreadDepthMM  float64    `json:"minTreadDepthMm"`
	Version          int64      `json:"version"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	// LowTreadDefectID is set on a tread reading that opened a defect.
	LowTreadDefectID string `json:"lowTreadDefectId,omitempty"`
}

type RegisterTyreInput struct {
	BranchID        string   `json:"branchId"`
	SerialNumber    string   `json:"serialNumber"`
	Brand           string   `json:"brand"`
	Size            string   `json:"size"`
	MinTreadDepthMM *float64 `json:"minTreadDepthMm"`
	TreadDepthMM    *float64 `json:"treadDepthMm"`
}

type TyreAction struct {
	ExpectedVersion int64    `json:"expectedVersion"`
	AssetID         string   `json:"assetId"`
	Position        string   `json:"position"`
	ToPosition      string   `json:"toPosition"`
	Reason          string   `json:"reason"`
	Scrap           bool     `json:"scrap"`
	TreadDepthMM    *float64 `json:"treadDepthMm"`
	Odometer        *float64 `json:"odometer"`
}

const tyreCols = `id,organization_id,branch_id,serial_number,brand,size,status,COALESCE(asset_id,''),position,installed_at,last_tread_depth_mm,min_tread_depth_mm,version,created_at,updated_at`

func scanTyre(r pgx.Row) (Tyre, error) {
	var v Tyre
	e := r.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.SerialNumber, &v.Brand, &v.Size, &v.Status, &v.AssetID, &v.Position, &v.InstalledAt, &v.LastTreadDepthMM, &v.MinTreadDepthMM, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (s *Service) tyreEvent(ctx context.Context, tx pgx.Tx, t tenant.Context, tyreID, kind, assetID, from, to, reason string, depth, odometer *float64) (string, error) {
	id := ids.New("tye")
	_, e := tx.Exec(ctx, `INSERT INTO fleet_partner_tyre_events(id,organization_id,tyre_id,event_type,asset_id,from_position,to_position,tread_depth_mm,odometer,reason,actor_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, t.OrganizationID, tyreID, kind, assetID, from, to, depth, odometer, reason, t.ActorID)
	return id, e
}

func (s *Service) RegisterTyre(ctx context.Context, t tenant.Context, in RegisterTyreInput) (Tyre, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return Tyre{}, e
	}
	in.SerialNumber = strings.TrimSpace(in.SerialNumber)
	if in.SerialNumber == "" || len(in.SerialNumber) > 120 || in.BranchID == "" {
		return Tyre{}, apperrors.BadRequest("branchId and serialNumber are required.", nil)
	}
	if !t.BranchPermitted(in.BranchID) {
		return Tyre{}, apperrors.NotFound("Resource not found.", nil)
	}
	min := 1.6
	if in.MinTreadDepthMM != nil {
		min = *in.MinTreadDepthMM
	}
	if min < 0 || in.TreadDepthMM != nil && *in.TreadDepthMM < 0 {
		return Tyre{}, apperrors.BadRequest("Tread depths cannot be negative.", nil)
	}
	var out Tyre
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var exists bool
		if x := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_branches WHERE organization_id=$1 AND id=$2)`, t.OrganizationID, in.BranchID).Scan(&exists); x != nil || !exists {
			return apperrors.NotFound("Resource not found.", x)
		}
		var x error
		out, x = scanTyre(tx.QueryRow(ctx, `INSERT INTO fleet_partner_tyres(id,organization_id,branch_id,serial_number,brand,size,last_tread_depth_mm,min_tread_depth_mm)VALUES($1,$2,$3,$4,$5,$6,$7,$8)RETURNING `+tyreCols, ids.New("tyr"), t.OrganizationID, in.BranchID, in.SerialNumber, strings.TrimSpace(in.Brand), strings.TrimSpace(in.Size), in.TreadDepthMM, min))
		if uniqueViolation(x) {
			return apperrors.Conflict("A tyre with this serial number already exists.", nil)
		}
		if x != nil {
			return x
		}
		if _, x = s.tyreEvent(ctx, tx, t, out.ID, "REGISTERED", "", "", "", "", in.TreadDepthMM, nil); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "tyre.registered.v1", "tyre", out.ID, out.Version, out)
	})
	return out, e
}

func (s *Service) GetTyre(ctx context.Context, t tenant.Context, id string) (Tyre, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return Tyre{}, e
	}
	v, e := scanTyre(s.o.Pool.QueryRow(ctx, `SELECT `+tyreCols+` FROM fleet_partner_tyres WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return Tyre{}, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}

func (s *Service) ListTyres(ctx context.Context, t tenant.Context, assetID, status string) ([]Tyre, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+tyreCols+` FROM fleet_partner_tyres WHERE organization_id=$1 AND($2=''OR asset_id=$2)AND($3=''OR status=$3) ORDER BY serial_number LIMIT 500`, t.OrganizationID, assetID, strings.ToUpper(status))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Tyre{}
	for rows.Next() {
		v, e := scanTyre(rows)
		if e != nil {
			return nil, e
		}
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}

// TyreTransition applies INSTALL, REMOVE, ROTATE or TREAD to a tyre. At most
// one installed tyre may occupy an asset position; a tread reading under the
// tyre's minimum on an installed tyre opens a HIGH defect for review.
func (s *Service) TyreTransition(ctx context.Context, t tenant.Context, id, action string, in TyreAction) (Tyre, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return Tyre{}, e
	}
	if in.ExpectedVersion <= 0 {
		return Tyre{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if _, e := s.GetTyre(ctx, t, id); e != nil {
		return Tyre{}, e
	}
	var out Tyre
	var lowTread bool
	e := s.tx(ctx, func(tx pgx.Tx) error {
		cur, x := scanTyre(tx.QueryRow(ctx, `SELECT `+tyreCols+` FROM fleet_partner_tyres WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id))
		if x != nil {
			return x
		}
		if cur.Version != in.ExpectedVersion {
			return apperrors.Conflict("Tyre version conflict.", nil)
		}
		status, assetID, position, event, kind := cur.Status, cur.AssetID, cur.Position, "", ""
		depth := cur.LastTreadDepthMM
		switch action {
		case "INSTALL":
			if cur.Status != "IN_STOCK" && cur.Status != "REMOVED" {
				return apperrors.Conflict("Only a stocked or removed tyre can be installed.", nil)
			}
			in.Position = strings.ToUpper(strings.TrimSpace(in.Position))
			if in.AssetID == "" || in.Position == "" {
				return apperrors.BadRequest("assetId and position are required.", nil)
			}
			var lifecycle, assetBranch string
			if x = tx.QueryRow(ctx, `SELECT lifecycle_status,branch_id FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2`, t.OrganizationID, in.AssetID).Scan(&lifecycle, &assetBranch); x != nil || !t.BranchPermitted(assetBranch) {
				return apperrors.NotFound("Asset not found.", nil)
			}
			if lifecycle != "ACTIVE" {
				return apperrors.Conflict("Tyres cannot be installed on an archived or retired asset.", nil)
			}
			status, assetID, position, event, kind = "INSTALLED", in.AssetID, in.Position, "tyre.installed.v1", "INSTALLED"
		case "REMOVE":
			if cur.Status != "INSTALLED" {
				return apperrors.Conflict("Only an installed tyre can be removed.", nil)
			}
			if strings.TrimSpace(in.Reason) == "" {
				return apperrors.BadRequest("reason is required.", nil)
			}
			status, assetID, position, event, kind = "REMOVED", "", "", "tyre.removed.v1", "REMOVED"
			if in.Scrap {
				status, event, kind = "SCRAPPED", "tyre.scrapped.v1", "SCRAPPED"
			}
		case "ROTATE":
			if cur.Status != "INSTALLED" {
				return apperrors.Conflict("Only an installed tyre can be rotated.", nil)
			}
			in.ToPosition = strings.ToUpper(strings.TrimSpace(in.ToPosition))
			if in.ToPosition == "" || in.ToPosition == cur.Position {
				return apperrors.BadRequest("A different toPosition is required.", nil)
			}
			position, event, kind = in.ToPosition, "tyre.rotated.v1", "ROTATED"
		case "TREAD":
			if cur.Status == "SCRAPPED" {
				return apperrors.Conflict("A scrapped tyre cannot be measured.", nil)
			}
			if in.TreadDepthMM == nil || *in.TreadDepthMM < 0 {
				return apperrors.BadRequest("treadDepthMm is required and cannot be negative.", nil)
			}
			depth, event, kind = in.TreadDepthMM, "tyre.tread_recorded.v1", "TREAD_READING"
		default:
			return apperrors.NotFound("Operation not found.", nil)
		}
		out, x = scanTyre(tx.QueryRow(ctx, `UPDATE fleet_partner_tyres SET status=$4,asset_id=NULLIF($5,''),position=$6,installed_at=CASE WHEN $4='INSTALLED' AND status<>'INSTALLED' THEN now() WHEN $4='INSTALLED' THEN installed_at ELSE NULL END,last_tread_depth_mm=$7,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+tyreCols, t.OrganizationID, id, cur.Version, status, assetID, position, depth))
		if uniqueViolation(x) {
			return apperrors.Conflict("Another tyre is already installed at that position.", nil)
		}
		if x != nil {
			return apperrors.Conflict("Tyre version conflict.", x)
		}
		eventAsset := assetID
		if eventAsset == "" {
			eventAsset = cur.AssetID
		}
		var tyreEventID string
		if tyreEventID, x = s.tyreEvent(ctx, tx, t, id, kind, eventAsset, cur.Position, position, strings.TrimSpace(in.Reason), in.TreadDepthMM, in.Odometer); x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, event, "tyre", id, out.Version, map[string]any{"tyre": out, "fromPosition": cur.Position}); x != nil {
			return x
		}
		if action == "TREAD" && out.Status == "INSTALLED" && *in.TreadDepthMM < out.MinTreadDepthMM {
			var open bool
			if x = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_defects WHERE organization_id=$1 AND source_type='TYRE_READING' AND item_code=$2 AND status<>'RESOLVED')`, t.OrganizationID, "TYRE:"+id).Scan(&open); x != nil {
				return x
			}
			if !open {
				d, _, x := s.insertDefectTx(ctx, tx, t, out.BranchID, ReportDefectInput{AssetID: out.AssetID, SourceType: "TYRE_READING", SourceReference: tyreEventID, ItemCode: "TYRE:" + id, Description: "Tyre " + out.SerialNumber + " at " + out.Position + " is below minimum tread depth", Severity: "HIGH"}, "")
				if x != nil {
					return x
				}
				out.LowTreadDefectID = d.ID
				lowTread = true
			}
		}
		return nil
	})
	_ = lowTread
	return out, e
}
