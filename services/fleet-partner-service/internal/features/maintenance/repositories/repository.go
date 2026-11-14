// Package repositories is the PostgreSQL persistence of Section 18
// maintenance: preventive schedules, work orders, their history and
// return-to-service decisions. Methods take the Querier to run on (nil means
// the pool); a missing row is pgx.ErrNoRows.
package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/maintenance/models"
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

const scheduleCols = `id,organization_id,asset_id,trigger_type,interval_value,next_due_date,next_due_odometer,next_due_hours,critical,status,version,created_at,updated_at`

func scanSchedule(row pgx.Row) (models.Schedule, error) {
	var v models.Schedule
	e := row.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.TriggerType, &v.IntervalValue, &v.NextDueDate, &v.NextDueOdometer, &v.NextDueHours, &v.Critical, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) InsertSchedule(ctx context.Context, q database.Querier, v models.Schedule) (models.Schedule, error) {
	return scanSchedule(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_maintenance_schedules(id,organization_id,asset_id,trigger_type,interval_value,next_due_date,next_due_odometer,next_due_hours,critical)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)RETURNING `+scheduleCols, v.ID, v.OrganizationID, v.AssetID, v.TriggerType, v.IntervalValue, v.NextDueDate, v.NextDueOdometer, v.NextDueHours, v.Critical))
}

func (r *Repository) UpdateSchedule(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Schedule) (models.Schedule, error) {
	return scanSchedule(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_maintenance_schedules SET next_due_date=COALESCE($4,next_due_date),next_due_odometer=COALESCE($5,next_due_odometer),next_due_hours=COALESCE($6,next_due_hours),status=COALESCE(NULLIF($7,''),status),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+scheduleCols, org, id, expected, v.NextDueDate, v.NextDueOdometer, v.NextDueHours, v.Status))
}

func (r *Repository) GetSchedule(ctx context.Context, org, id string) (models.Schedule, error) {
	return scanSchedule(r.db.QueryRow(ctx, `SELECT `+scheduleCols+` FROM fleet_partner_maintenance_schedules WHERE organization_id=$1 AND id=$2`, org, id))
}

// ListSchedules returns up to limit schedules newest first, keyset on
// (created_at, id) after the given position (nil starts at the newest).
func (r *Repository) ListSchedules(ctx context.Context, org string, limit int, after *time.Time, afterID string) ([]models.Schedule, error) {
	rows, e := r.db.Query(ctx, `SELECT `+scheduleCols+` FROM fleet_partner_maintenance_schedules WHERE organization_id=$1 AND($3::timestamptz IS NULL OR(created_at,id)<($3::timestamptz,$4)) ORDER BY created_at DESC,id DESC LIMIT $2`, org, limit, after, afterID)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanSchedule)
}

// ListDueSchedules lists the active schedules of the organization that are
// due by date, odometer or operating hours.
func (r *Repository) ListDueSchedules(ctx context.Context, org string) ([]models.DueSchedule, error) {
	rows, e := r.db.Query(ctx, `SELECT s.id,s.asset_id,s.critical FROM fleet_partner_maintenance_schedules s JOIN fleet_partner_assets a ON a.id=s.asset_id WHERE s.organization_id=$1 AND s.status='ACTIVE' AND((s.next_due_date IS NOT NULL AND s.next_due_date<=CURRENT_DATE)OR(s.next_due_odometer IS NOT NULL AND s.next_due_odometer<=a.odometer)OR(s.next_due_hours IS NOT NULL AND s.next_due_hours<=a.operating_hours))`, org)
	if e != nil {
		return nil, e
	}
	return collect(rows, func(row pgx.Row) (models.DueSchedule, error) {
		var v models.DueSchedule
		return v, row.Scan(&v.ID, &v.AssetID, &v.Critical)
	})
}

const workCols = `id,organization_id,asset_id,source_type,source_reference,priority,diagnosis,required_work,status,approved_by,approved_at,started_at,completed_at,downtime_start,downtime_end,evidence_media_ids,return_to_service_status,version,created_at,updated_at`

func scanWork(row pgx.Row) (models.WorkOrder, error) {
	var v models.WorkOrder
	e := row.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.SourceType, &v.SourceReference, &v.Priority, &v.Diagnosis, &v.RequiredWork, &v.Status, &v.ApprovedBy, &v.ApprovedAt, &v.StartedAt, &v.CompletedAt, &v.DowntimeStart, &v.DowntimeEnd, &v.EvidenceMediaIDs, &v.ReturnToServiceStatus, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) InsertWorkOrder(ctx context.Context, q database.Querier, v models.WorkOrder) (models.WorkOrder, error) {
	return scanWork(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_work_orders(id,organization_id,asset_id,source_type,source_reference,priority,diagnosis,required_work,status)VALUES($1,$2,$3,$4,$5,$6,$7,$8,'DRAFT')RETURNING `+workCols, v.ID, v.OrganizationID, v.AssetID, v.SourceType, v.SourceReference, v.Priority, v.Diagnosis, v.RequiredWork))
}

// InsertSystemWorkOrder raises a work order on behalf of a safety source
// (inspection, defect) and returns its version.
func (r *Repository) InsertSystemWorkOrder(ctx context.Context, q database.Querier, id, org, assetID, sourceType, sourceRef, priority, requiredWork string) (int64, error) {
	var version int64
	e := r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_work_orders(id,organization_id,asset_id,source_type,source_reference,priority,required_work)VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING version`, id, org, assetID, sourceType, sourceRef, priority, requiredWork).Scan(&version)
	return version, e
}

// EnsureSystemWorkOrder raises a work order once per source; pgx.ErrNoRows
// means the source already has one.
func (r *Repository) EnsureSystemWorkOrder(ctx context.Context, q database.Querier, id, org, assetID, sourceType, sourceRef, priority, requiredWork string) (int64, error) {
	var version int64
	e := r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_work_orders(id,organization_id,asset_id,source_type,source_reference,priority,required_work)VALUES($1,$2,$3,$4,$5,$6,$7)ON CONFLICT(organization_id,source_type,source_reference)DO NOTHING RETURNING version`, id, org, assetID, sourceType, sourceRef, priority, requiredWork).Scan(&version)
	return version, e
}

// SystemWorkOrderID is the work order raised for one source.
func (r *Repository) SystemWorkOrderID(ctx context.Context, q database.Querier, org, sourceType, sourceRef string) (string, error) {
	var id string
	e := r.q(q).QueryRow(ctx, `SELECT id FROM fleet_partner_work_orders WHERE organization_id=$1 AND source_type=$2 AND source_reference=$3`, org, sourceType, sourceRef).Scan(&id)
	return id, e
}

func (r *Repository) GetWorkOrder(ctx context.Context, org, id string) (models.WorkOrder, error) {
	return scanWork(r.db.QueryRow(ctx, `SELECT `+workCols+` FROM fleet_partner_work_orders WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) ListWorkOrders(ctx context.Context, org string) ([]models.WorkOrder, error) {
	rows, e := r.db.Query(ctx, `SELECT `+workCols+` FROM fleet_partner_work_orders WHERE organization_id=$1 ORDER BY created_at DESC`, org)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanWork)
}

func (r *Repository) UpdateWorkOrder(ctx context.Context, q database.Querier, org, id string, expected int64, in models.WorkAction, to, approver string, evidence []byte) (models.WorkOrder, error) {
	return scanWork(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_work_orders SET diagnosis=COALESCE(NULLIF($4,''),diagnosis),required_work=COALESCE(NULLIF($5,''),required_work),status=$6,approved_by=CASE WHEN $6='APPROVED' THEN $7 ELSE approved_by END,approved_at=CASE WHEN $6='APPROVED' THEN now() ELSE approved_at END,completed_at=CASE WHEN $6='COMPLETED' THEN now() ELSE completed_at END,evidence_media_ids=CASE WHEN $8::jsonb<>'[]' THEN $8 ELSE evidence_media_ids END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+workCols, org, id, expected, in.Diagnosis, in.RequiredWork, to, approver, evidence))
}

func (r *Repository) InsertWorkOrderHistory(ctx context.Context, q database.Querier, org, id, previous, next, actor, reason string, version int64) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_work_order_history(id,organization_id,work_order_id,previous_status,new_status,actor_id,reason,version)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("woh"), org, id, previous, next, actor, reason, version)
	return e
}

func (r *Repository) ApproveReturnToService(ctx context.Context, q database.Querier, org, id string, expected int64) (models.WorkOrder, error) {
	return scanWork(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_work_orders SET return_to_service_status='APPROVED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+workCols, org, id, expected))
}

func (r *Repository) InsertReturnToServiceDecision(ctx context.Context, q database.Querier, org, workOrderID, assetID, reason string, evidence []byte, actor string) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_return_to_service_decisions(id,organization_id,work_order_id,asset_id,decision,reason,evidence_media_ids,decided_by)VALUES($1,$2,$3,$4,'APPROVED',$5,$6,$7)`, ids.New("rts"), org, workOrderID, assetID, reason, evidence, actor)
	return e
}
