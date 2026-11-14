// Package repositories is the PostgreSQL persistence of Section 18 assets:
// assets and their history, assignments, documents, meter readings,
// operating costs, safety blocks, breakdowns, parts and stock movements,
// defects and tyres. Methods take the Querier to run on (nil means the pool)
// so a use case composes them into one transaction. A missing row is
// pgx.ErrNoRows.
package repositories

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
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

func (r *Repository) exists(ctx context.Context, q database.Querier, sql string, args ...any) (bool, error) {
	var ok bool
	e := r.q(q).QueryRow(ctx, sql, args...).Scan(&ok)
	return ok, e
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// ---------------------------------------------------------------------------
// Assets
// ---------------------------------------------------------------------------

const assetCols = `id,organization_id,branch_id,asset_code,asset_type,platform_vehicle_id,ownership_type,registration_reference,manufacturer,model,year,capacity,capacity_unit,odometer,operating_hours,operational_status,lifecycle_status,version,created_at,updated_at,archived_at,retired_at`

func scanAsset(row pgx.Row) (models.Asset, error) {
	var v models.Asset
	e := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.AssetCode, &v.AssetType, &v.PlatformVehicleID, &v.OwnershipType, &v.RegistrationReference, &v.Manufacturer, &v.Model, &v.Year, &v.Capacity, &v.CapacityUnit, &v.Odometer, &v.OperatingHours, &v.OperationalStatus, &v.LifecycleStatus, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.RetiredAt)
	return v, e
}

func (r *Repository) InsertAsset(ctx context.Context, q database.Querier, org string, v models.Asset) (models.Asset, error) {
	return scanAsset(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_assets(id,organization_id,branch_id,asset_code,asset_type,platform_vehicle_id,ownership_type,registration_reference,manufacturer,model,year,capacity,capacity_unit,odometer,operating_hours)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)RETURNING `+assetCols, v.ID, org, v.BranchID, v.AssetCode, strings.ToUpper(v.AssetType), v.PlatformVehicleID, v.OwnershipType, v.RegistrationReference, v.Manufacturer, v.Model, v.Year, v.Capacity, v.CapacityUnit, v.Odometer, v.OperatingHours))
}

func (r *Repository) GetAsset(ctx context.Context, org, id string) (models.Asset, error) {
	return scanAsset(r.db.QueryRow(ctx, `SELECT `+assetCols+` FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2`, org, id))
}

// FindAssetByReference resolves an asset by its id or platform vehicle id.
func (r *Repository) FindAssetByReference(ctx context.Context, org, reference string) (models.Asset, error) {
	return scanAsset(r.db.QueryRow(ctx, `SELECT `+assetCols+` FROM fleet_partner_assets WHERE organization_id=$1 AND(id=$2 OR platform_vehicle_id=$2)`, org, reference))
}

// ListAssets returns one page newest first, keyset on (created_at, id). The
// caller's permitted branches are applied in SQL, before LIMIT, so a
// branch-scoped member gets full pages of their own assets.
func (r *Repository) ListAssets(ctx context.Context, org string, f models.Page, permitted []string, limit, offset int) ([]models.Asset, error) {
	if permitted == nil {
		permitted = []string{}
	}
	rows, e := r.db.Query(ctx, `SELECT `+assetCols+` FROM fleet_partner_assets WHERE organization_id=$1 AND($2=''OR branch_id=$2)AND($3=''OR operational_status=$3 OR lifecycle_status=$3)AND(COALESCE(cardinality($6::text[]),0)=0 OR branch_id=ANY($6::text[]))AND($7::timestamptz IS NULL OR(created_at,id)<($7::timestamptz,$8))ORDER BY created_at DESC,id DESC LIMIT $4 OFFSET $5`, org, f.BranchID, f.Status, limit, offset, permitted, f.After, f.AfterID)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanAsset)
}

func (r *Repository) ActiveAssignmentExists(ctx context.Context, q database.Querier, org, assetID string) (bool, error) {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_assignments WHERE organization_id=$1 AND asset_id=$2 AND status='ACTIVE')`, org, assetID)
}

// LockAsset locks the asset row and returns its safety state.
func (r *Repository) LockAsset(ctx context.Context, q database.Querier, org, id string) (models.AssetLock, error) {
	var v models.AssetLock
	e := r.q(q).QueryRow(ctx, `SELECT branch_id,operational_status,lifecycle_status,version,odometer,operating_hours FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id).Scan(&v.BranchID, &v.OperationalStatus, &v.LifecycleStatus, &v.Version, &v.Odometer, &v.OperatingHours)
	return v, e
}

// AssetStatus reads the asset's statuses without locking.
func (r *Repository) AssetStatus(ctx context.Context, q database.Querier, org, id string) (operational, lifecycle string, e error) {
	e = r.q(q).QueryRow(ctx, `SELECT operational_status,lifecycle_status FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2`, org, id).Scan(&operational, &lifecycle)
	return
}

func (r *Repository) AssetLifecycleAndBranch(ctx context.Context, q database.Querier, org, id string) (lifecycle, branch string, e error) {
	e = r.q(q).QueryRow(ctx, `SELECT lifecycle_status,branch_id FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2`, org, id).Scan(&lifecycle, &branch)
	return
}

func (r *Repository) UpdateAsset(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Asset, lifecycle string) (models.Asset, error) {
	return scanAsset(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_assets SET manufacturer=COALESCE(NULLIF($4,''),manufacturer),model=COALESCE(NULLIF($5,''),model),capacity=CASE WHEN $6>=0 THEN $6 ELSE capacity END,odometer=CASE WHEN $7>=odometer THEN $7 ELSE odometer END,operating_hours=CASE WHEN $8>=operating_hours THEN $8 ELSE operating_hours END,lifecycle_status=$9,archived_at=CASE WHEN $9='ARCHIVED' THEN now() ELSE archived_at END,retired_at=CASE WHEN $9='RETIRED' THEN now() ELSE retired_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+assetCols, org, id, expected, v.Manufacturer, v.Model, v.Capacity, v.Odometer, v.OperatingHours, lifecycle))
}

func (r *Repository) InsertAssetHistory(ctx context.Context, q database.Querier, org, assetID, prevOp, newOp, prevLife, newLife, reason, actor string, version int64, snapshot any) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_asset_history(id,organization_id,asset_id,previous_operational_status,new_operational_status,previous_lifecycle_status,new_lifecycle_status,reason,actor_id,version,snapshot)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, ids.New("ash"), org, assetID, prevOp, newOp, prevLife, newLife, reason, actor, version, mustJSON(snapshot))
	return e
}

func (r *Repository) ListAssetHistory(ctx context.Context, org, assetID string) ([]models.AssetHistoryEntry, error) {
	rows, e := r.db.Query(ctx, `SELECT id,previous_operational_status,new_operational_status,previous_lifecycle_status,new_lifecycle_status,reason,actor_id,version,snapshot,occurred_at FROM fleet_partner_asset_history WHERE organization_id=$1 AND asset_id=$2 ORDER BY occurred_at DESC,version DESC LIMIT 200`, org, assetID)
	if e != nil {
		return nil, e
	}
	return collect(rows, func(row pgx.Row) (models.AssetHistoryEntry, error) {
		var v models.AssetHistoryEntry
		var snap []byte
		e := row.Scan(&v.ID, &v.PreviousOperationalStatus, &v.NewOperationalStatus, &v.PreviousLifecycleStatus, &v.NewLifecycleStatus, &v.Reason, &v.ActorID, &v.Version, &snap, &v.OccurredAt)
		v.Snapshot = json.RawMessage(snap)
		return v, e
	})
}

// SetOperationalStatus sets an asset's operational status and returns its
// new version.
func (r *Repository) SetOperationalStatus(ctx context.Context, q database.Querier, org, id, status string) (int64, error) {
	var version int64
	e := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_assets SET operational_status=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, org, id, status).Scan(&version)
	return version, e
}

// MarkAssigned moves an AVAILABLE asset to ASSIGNED.
func (r *Repository) MarkAssigned(ctx context.Context, q database.Querier, assetID string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_assets SET operational_status='ASSIGNED',version=version+1,updated_at=now() WHERE id=$1 AND operational_status='AVAILABLE'`, assetID)
	return e
}

// MarkOutOfService moves an asset OUT_OF_SERVICE after a severe breakdown.
func (r *Repository) MarkOutOfService(ctx context.Context, q database.Querier, org, id string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_assets SET operational_status='OUT_OF_SERVICE',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, id)
	return e
}

func (r *Repository) ActiveMembership(ctx context.Context, org, id string) bool {
	ok, _ := r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND id=$2 AND status='ACTIVE')`, org, id)
	return ok
}

func (r *Repository) BranchExists(ctx context.Context, q database.Querier, org, id string) (bool, error) {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_branches WHERE organization_id=$1 AND id=$2)`, org, id)
}

// ---------------------------------------------------------------------------
// Eligibility reads
// ---------------------------------------------------------------------------

func (r *Repository) AssignmentConflict(ctx context.Context, org, assetID, provider string) bool {
	ok, _ := r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_assignments WHERE organization_id=$1 AND asset_id=$2 AND status='ACTIVE' AND($3=''OR provider_membership_id<>$3))`, org, assetID, provider)
	return ok
}

func (r *Repository) ProviderAuthorized(ctx context.Context, org, assetID, provider string, at time.Time) bool {
	ok, _ := r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_assignments WHERE organization_id=$1 AND asset_id=$2 AND provider_membership_id=$3 AND status IN('PLANNED','ACTIVE') AND start_at<=$4 AND(end_at IS NULL OR end_at>$4))`, org, assetID, provider, at)
	return ok
}

func (r *Repository) DocumentExpired(ctx context.Context, org, assetID string) bool {
	ok, _ := r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_documents WHERE organization_id=$1 AND asset_id=$2 AND(verification_status IN('EXPIRED','REVOKED','REJECTED') OR expires_at<=now()))`, org, assetID)
	return ok
}

func (r *Repository) MaintenanceOverdue(ctx context.Context, org, assetID string, odometer, hours float64) bool {
	ok, _ := r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_maintenance_schedules WHERE organization_id=$1 AND asset_id=$2 AND status='ACTIVE' AND((next_due_date IS NOT NULL AND next_due_date<=CURRENT_DATE)OR(next_due_odometer IS NOT NULL AND next_due_odometer<=$3)OR(next_due_hours IS NOT NULL AND next_due_hours<=$4)))`, org, assetID, odometer, hours)
	return ok
}

// ---------------------------------------------------------------------------
// Assignments and documents
// ---------------------------------------------------------------------------

func (r *Repository) InsertAssignment(ctx context.Context, q database.Querier, v *models.Assignment) error {
	return r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_asset_assignments(id,organization_id,asset_id,provider_membership_id,shift_id,assignment_type,start_at,end_at,status)VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,'ACTIVE')RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, v.AssetID, v.ProviderMembershipID, v.ShiftID, v.AssignmentType, v.StartAt, v.EndAt).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
}

// LockCurrentAssignment locks the asset's current (ACTIVE first) or planned
// assignment, or the named one.
func (r *Repository) LockCurrentAssignment(ctx context.Context, q database.Querier, org, assetID, assignmentID string) (models.Assignment, error) {
	var out models.Assignment
	e := r.q(q).QueryRow(ctx, `SELECT id,organization_id,asset_id,provider_membership_id,COALESCE(shift_id,''),assignment_type,start_at,end_at,status,version,created_at,updated_at FROM fleet_partner_asset_assignments WHERE organization_id=$1 AND asset_id=$2 AND status IN('PLANNED','ACTIVE') AND($3=''OR id=$3) ORDER BY(status='ACTIVE')DESC,start_at LIMIT 1 FOR UPDATE`, org, assetID, assignmentID).Scan(&out.ID, &out.OrganizationID, &out.AssetID, &out.ProviderMembershipID, &out.ShiftID, &out.AssignmentType, &out.StartAt, &out.EndAt, &out.Status, &out.Version, &out.CreatedAt, &out.UpdatedAt)
	return out, e
}

// ProviderHasActiveWork reports company work of the provider on the asset.
func (r *Repository) ProviderHasActiveWork(ctx context.Context, q database.Querier, org, membershipID, assetID, platformVehicleID string) (bool, error) {
	return r.exists(ctx, q, `SELECT EXISTS(SELECT 1 FROM fleet_partner_private_assignments WHERE organization_id=$1 AND provider_membership_id=$2 AND vehicle_or_truck_reference IN($3,NULLIF($4,'')) AND status IN('ACCEPTED','READY_FOR_EXECUTION','IN_PROGRESS'))
OR EXISTS(SELECT 1 FROM fleet_partner_provider_assignments WHERE organization_id=$1 AND membership_id=$2 AND asset_reference IN($3,NULLIF($4,'')) AND status IN('ACCEPTED','READY_FOR_EXECUTION','IN_PROGRESS','EXECUTION_START_PENDING'))`, org, membershipID, assetID, platformVehicleID)
}

// EndAssignment completes a started assignment or cancels a future one.
func (r *Repository) EndAssignment(ctx context.Context, q database.Querier, org, actor, reason string, out *models.Assignment) error {
	return r.q(q).QueryRow(ctx, `UPDATE fleet_partner_asset_assignments SET status=CASE WHEN start_at<now() THEN 'COMPLETED' ELSE 'CANCELLED' END,end_at=CASE WHEN start_at<now() AND(end_at IS NULL OR end_at>now()) THEN now() ELSE end_at END,ended_by=$3,end_reason=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING status,end_at,version,updated_at`, org, out.ID, actor, reason).Scan(&out.Status, &out.EndAt, &out.Version, &out.UpdatedAt)
}

func (r *Repository) InsertDocument(ctx context.Context, q database.Querier, v *models.Document) error {
	return r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_asset_documents(id,organization_id,asset_id,media_file_id,document_type,issued_at,expires_at)VALUES($1,$2,$3,$4,$5,$6,$7)RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, v.AssetID, v.MediaFileID, v.DocumentType, v.IssuedAt, v.ExpiresAt).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
}

func (r *Repository) ListDocuments(ctx context.Context, org, assetID string) ([]models.Document, error) {
	rows, e := r.db.Query(ctx, `SELECT id,organization_id,asset_id,media_file_id,document_type,issued_at,expires_at,verification_reference,verification_status,version,created_at,updated_at FROM fleet_partner_asset_documents WHERE organization_id=$1 AND asset_id=$2`, org, assetID)
	if e != nil {
		return nil, e
	}
	return collect(rows, func(row pgx.Row) (models.Document, error) {
		var v models.Document
		e := row.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.MediaFileID, &v.DocumentType, &v.IssuedAt, &v.ExpiresAt, &v.VerificationReference, &v.VerificationStatus, &v.Version, &v.CreatedAt, &v.UpdatedAt)
		return v, e
	})
}

// ExpireDocumentsAndAssignments is the asset part of the Section 18 sweep.
func (r *Repository) ExpireDocumentsAndAssignments(ctx context.Context, org string) error {
	if _, e := r.db.Exec(ctx, `UPDATE fleet_partner_asset_documents SET verification_status='EXPIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND verification_status IN('PENDING','VERIFIED') AND expires_at<=now()`, org); e != nil {
		return e
	}
	_, e := r.db.Exec(ctx, `UPDATE fleet_partner_asset_assignments SET status='COMPLETED',version=version+1,updated_at=now() WHERE organization_id=$1 AND status IN('PLANNED','ACTIVE') AND end_at<=now()`, org)
	return e
}

func (r *Repository) ActiveOrganizationIDs(ctx context.Context) ([]string, error) {
	rows, e := r.db.Query(ctx, `SELECT id FROM fleet_partner_organizations WHERE status='ACTIVE'`)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanString)
}

func scanString(row pgx.Row) (string, error) {
	var s string
	return s, row.Scan(&s)
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
// Meter readings
// ---------------------------------------------------------------------------

func (r *Repository) LastMeterReadingAt(ctx context.Context, q database.Querier, org, assetID, meterType string) (*time.Time, error) {
	var lastAt *time.Time
	e := r.q(q).QueryRow(ctx, `SELECT max(recorded_at) FROM fleet_partner_meter_readings WHERE organization_id=$1 AND asset_id=$2 AND meter_type=$3`, org, assetID, meterType).Scan(&lastAt)
	return lastAt, e
}

func (r *Repository) InsertMeterReading(ctx context.Context, q database.Querier, v *models.MeterReading) error {
	return r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_meter_readings(id,organization_id,asset_id,meter_type,reading,previous_reading,source,source_reference,recorded_at,anomaly,anomaly_reasons,recorded_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)RETURNING created_at`, v.ID, v.OrganizationID, v.AssetID, v.MeterType, v.Reading, v.PreviousReading, v.Source, v.SourceReference, v.RecordedAt, v.Anomaly, mustJSON(v.AnomalyReasons), v.RecordedBy).Scan(&v.CreatedAt)
}

// SetMeter advances the asset's odometer or engine-hours meter.
func (r *Repository) SetMeter(ctx context.Context, q database.Querier, org, id, meterType string, reading float64) (int64, error) {
	column := "odometer"
	if meterType == "ENGINE_HOURS" {
		column = "operating_hours"
	}
	var version int64
	e := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_assets SET `+column+`=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, org, id, reading).Scan(&version)
	return version, e
}

// SchedulesCrossed lists the meter-triggered schedules a reading moved past.
func (r *Repository) SchedulesCrossed(ctx context.Context, q database.Querier, org, assetID, meterType string, reading, previous float64) ([]string, error) {
	thresholdColumn := "next_due_odometer"
	if meterType == "ENGINE_HOURS" {
		thresholdColumn = "next_due_hours"
	}
	rows, e := r.q(q).Query(ctx, `SELECT id FROM fleet_partner_maintenance_schedules WHERE organization_id=$1 AND asset_id=$2 AND status='ACTIVE' AND trigger_type IN('MILEAGE','OPERATING_HOURS','COMBINED') AND `+thresholdColumn+` IS NOT NULL AND `+thresholdColumn+`<=$3 AND `+thresholdColumn+`>$4 ORDER BY id`, org, assetID, reading, previous)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repository) ListMeterReadings(ctx context.Context, org, assetID, meterType string) ([]models.MeterReading, error) {
	rows, e := r.db.Query(ctx, `SELECT id,organization_id,asset_id,meter_type,reading,previous_reading,source,source_reference,recorded_at,anomaly,anomaly_reasons,recorded_by,created_at FROM fleet_partner_meter_readings WHERE organization_id=$1 AND asset_id=$2 AND($3=''OR meter_type=$3) ORDER BY recorded_at DESC,created_at DESC LIMIT 200`, org, assetID, strings.ToUpper(meterType))
	if e != nil {
		return nil, e
	}
	return collect(rows, func(row pgx.Row) (models.MeterReading, error) {
		var v models.MeterReading
		var reasons []byte
		e := row.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.MeterType, &v.Reading, &v.PreviousReading, &v.Source, &v.SourceReference, &v.RecordedAt, &v.Anomaly, &reasons, &v.RecordedBy, &v.CreatedAt)
		_ = json.Unmarshal(reasons, &v.AnomalyReasons)
		return v, e
	})
}

// ---------------------------------------------------------------------------
// Operating costs
// ---------------------------------------------------------------------------

// CostSourceLinked reports whether a sourced cost references a record of the
// same asset in the same organization.
func (r *Repository) CostSourceLinked(ctx context.Context, org, sourceType, reference, assetID string) (bool, error) {
	switch sourceType {
	case "WORK_ORDER":
		return r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_work_orders WHERE organization_id=$1 AND id=$2 AND asset_id=$3)`, org, reference, assetID)
	case "FUEL_RECORD":
		return r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_fuel_records WHERE organization_id=$1 AND id=$2 AND asset_id=$3)`, org, reference, assetID)
	case "STOCK_MOVEMENT":
		return r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_stock_movements m JOIN fleet_partner_work_orders w ON w.organization_id=m.organization_id AND w.id=m.work_order_id WHERE m.organization_id=$1 AND m.id=$2 AND w.asset_id=$3)`, org, reference, assetID)
	case "TYRE":
		return r.exists(ctx, nil, `SELECT EXISTS(SELECT 1 FROM fleet_partner_tyres WHERE organization_id=$1 AND id=$2 AND asset_id=$3) OR EXISTS(SELECT 1 FROM fleet_partner_tyre_events WHERE organization_id=$1 AND tyre_id=$2 AND asset_id=$3)`, org, reference, assetID)
	}
	return true, nil
}

func (r *Repository) InsertCost(ctx context.Context, q database.Querier, v *models.CostEntry) error {
	return r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_asset_cost_entries(id,organization_id,asset_id,cost_type,source_type,source_reference,amount_minor,currency,incurred_at,description,recorded_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)RETURNING created_at`, v.ID, v.OrganizationID, v.AssetID, v.CostType, v.SourceType, v.SourceReference, v.AmountMinor, v.Currency, v.IncurredAt, v.Description, v.RecordedBy).Scan(&v.CreatedAt)
}

func (r *Repository) ListCostEntries(ctx context.Context, org, assetID string) ([]models.CostEntry, error) {
	rows, e := r.db.Query(ctx, `SELECT id,organization_id,asset_id,cost_type,source_type,source_reference,amount_minor,currency,incurred_at,description,recorded_by,created_at FROM fleet_partner_asset_cost_entries WHERE organization_id=$1 AND asset_id=$2 ORDER BY incurred_at DESC,created_at DESC LIMIT 500`, org, assetID)
	if e != nil {
		return nil, e
	}
	return collect(rows, func(row pgx.Row) (models.CostEntry, error) {
		var v models.CostEntry
		e := row.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.CostType, &v.SourceType, &v.SourceReference, &v.AmountMinor, &v.Currency, &v.IncurredAt, &v.Description, &v.RecordedBy, &v.CreatedAt)
		return v, e
	})
}

func (r *Repository) CostTotals(ctx context.Context, org, assetID string) ([]models.CostTotal, error) {
	rows, e := r.db.Query(ctx, `SELECT cost_type,currency,sum(amount_minor)::bigint FROM fleet_partner_asset_cost_entries WHERE organization_id=$1 AND asset_id=$2 GROUP BY cost_type,currency ORDER BY cost_type,currency`, org, assetID)
	if e != nil {
		return nil, e
	}
	return collect(rows, func(row pgx.Row) (models.CostTotal, error) {
		var v models.CostTotal
		return v, row.Scan(&v.CostType, &v.Currency, &v.AmountMinor)
	})
}
