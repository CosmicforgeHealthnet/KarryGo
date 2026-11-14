// Package repositories is the PostgreSQL persistence of Section 18
// inspections: checklist templates, inspections and their item results, and
// the safety-decline inbox. Methods take the Querier to run on (nil means the
// pool); a missing row is pgx.ErrNoRows.
package repositories

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/inspections/models"
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

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

const templateCols = `id,organization_id,name,asset_types,service_types,checklist_items,criticality_rules,status,version,created_at,updated_at`

func scanTemplate(row pgx.Row) (models.Template, error) {
	var v models.Template
	var a, b []byte
	e := row.Scan(&v.ID, &v.OrganizationID, &v.Name, &a, &b, &v.ChecklistItems, &v.CriticalityRules, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	_ = json.Unmarshal(a, &v.AssetTypes)
	_ = json.Unmarshal(b, &v.ServiceTypes)
	return v, e
}

func (r *Repository) InsertTemplate(ctx context.Context, v *models.Template) error {
	a, _ := json.Marshal(v.AssetTypes)
	b, _ := json.Marshal(v.ServiceTypes)
	return r.db.QueryRow(ctx, `INSERT INTO fleet_partner_inspection_templates(id,organization_id,name,asset_types,service_types,checklist_items,criticality_rules)VALUES($1,$2,$3,$4,$5,$6,$7)RETURNING status,version,created_at,updated_at`, v.ID, v.OrganizationID, v.Name, a, b, v.ChecklistItems, v.CriticalityRules).Scan(&v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
}

func (r *Repository) UpdateTemplate(ctx context.Context, org, id string, expected int64, v *models.Template) error {
	return r.db.QueryRow(ctx, `UPDATE fleet_partner_inspection_templates SET name=COALESCE(NULLIF($4,''),name),checklist_items=COALESCE($5,checklist_items),criticality_rules=COALESCE($6,criticality_rules),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING organization_id,status,version,created_at,updated_at`, org, id, expected, v.Name, v.ChecklistItems, v.CriticalityRules).Scan(&v.OrganizationID, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
}

func (r *Repository) GetTemplate(ctx context.Context, org, id string) (models.Template, error) {
	return scanTemplate(r.db.QueryRow(ctx, `SELECT `+templateCols+` FROM fleet_partner_inspection_templates WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) ListTemplates(ctx context.Context, org string) ([]models.Template, error) {
	rows, e := r.db.Query(ctx, `SELECT `+templateCols+` FROM fleet_partner_inspection_templates WHERE organization_id=$1 ORDER BY created_at DESC`, org)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanTemplate)
}

// ---------------------------------------------------------------------------
// Inspections
// ---------------------------------------------------------------------------

const inspectionCols = `id,organization_id,asset_id,template_id,inspector_membership_id,assignment_id,trip_reference,status,COALESCE(overall_result,''),defect_summary,evidence_media_ids,started_at,completed_at,version,created_at,updated_at`

func scanInspection(row pgx.Row) (models.Inspection, error) {
	var v models.Inspection
	e := row.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.TemplateID, &v.InspectorMembershipID, &v.AssignmentID, &v.TripReference, &v.Status, &v.OverallResult, &v.DefectSummary, &v.EvidenceMediaIDs, &v.StartedAt, &v.CompletedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) InsertInspection(ctx context.Context, v *models.Inspection) error {
	return r.db.QueryRow(ctx, `INSERT INTO fleet_partner_inspections(id,organization_id,asset_id,template_id,inspector_membership_id,assignment_id,trip_reference,status,started_at)VALUES($1,$2,$3,$4,$5,$6,$7,'IN_PROGRESS',$8)RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, v.AssetID, v.TemplateID, v.InspectorMembershipID, v.AssignmentID, v.TripReference, v.StartedAt).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
}

func (r *Repository) GetInspection(ctx context.Context, org, id string) (models.Inspection, error) {
	return scanInspection(r.db.QueryRow(ctx, `SELECT `+inspectionCols+` FROM fleet_partner_inspections WHERE organization_id=$1 AND id=$2`, org, id))
}

// ListInspections returns up to limit inspections newest first, keyset on
// (created_at, id) after the given position (nil starts at the newest).
func (r *Repository) ListInspections(ctx context.Context, org, status string, limit int, after *time.Time, afterID string) ([]models.Inspection, error) {
	rows, e := r.db.Query(ctx, `SELECT `+inspectionCols+` FROM fleet_partner_inspections WHERE organization_id=$1 AND($2=''OR status=$2)AND($4::timestamptz IS NULL OR(created_at,id)<($4::timestamptz,$5))ORDER BY created_at DESC,id DESC LIMIT $3`, org, status, limit, after, afterID)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanInspection)
}

func (r *Repository) CompleteInspection(ctx context.Context, q database.Querier, org, id string, expected int64, result string, defectSummary json.RawMessage) (models.Inspection, error) {
	return scanInspection(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_inspections SET status='COMPLETED',overall_result=$4,defect_summary=$5,completed_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND status IN('DRAFT','IN_PROGRESS') RETURNING `+inspectionCols, org, id, expected, result, defectSummary))
}

func (r *Repository) InsertResult(ctx context.Context, q database.Querier, org, inspectionID string, a models.Answer, result string, critical bool) error {
	raw, _ := json.Marshal(a)
	ev, _ := json.Marshal(a.EvidenceMediaIDs)
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_inspection_results(id,organization_id,inspection_id,item_code,answer,evidence_media_ids,result,critical)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("inr"), org, inspectionID, a.ItemCode, raw, ev, result, critical)
	return e
}

// CriticalFollowUpUnresolved reports a critical inspection of the asset
// completed after the work order.
func (r *Repository) CriticalFollowUpUnresolved(ctx context.Context, q database.Querier, org, assetID, workOrderID string) (bool, error) {
	var unresolved bool
	e := r.q(q).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_inspections WHERE organization_id=$1 AND asset_id=$2 AND overall_result='CRITICAL_FAIL' AND completed_at>(SELECT completed_at FROM fleet_partner_work_orders WHERE id=$3))`, org, assetID, workOrderID).Scan(&unresolved)
	return unresolved, e
}

// SourceInspectionCleared reports whether the source inspection was a
// critical failure and whether a passing follow-up inspection of the asset
// was completed after the work order.
func (r *Repository) SourceInspectionCleared(ctx context.Context, q database.Querier, org, inspectionID, assetID, workOrderID string) (bool, bool, error) {
	var sourceCritical, followupPassed bool
	e := r.q(q).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_inspections WHERE organization_id=$1 AND id=$2 AND overall_result='CRITICAL_FAIL'),EXISTS(SELECT 1 FROM fleet_partner_inspections WHERE organization_id=$1 AND asset_id=$3 AND id<>$2 AND status='COMPLETED' AND overall_result IN('PASS','PASS_WITH_ADVISORY') AND completed_at>(SELECT completed_at FROM fleet_partner_work_orders WHERE id=$4))`, org, inspectionID, assetID, workOrderID).Scan(&sourceCritical, &followupPassed)
	return sourceCritical, followupPassed, e
}

// ---------------------------------------------------------------------------
// Safety declines
// ---------------------------------------------------------------------------

// PendingSafetyDeclines reads unsafe-vehicle declines from the durable
// Section 15 assignment projections that the inbox has not yet seen.
func (r *Repository) PendingSafetyDeclines(ctx context.Context) ([]models.SafetyDecline, error) {
	rows, e := r.db.Query(ctx, `
		SELECT organization_id, id, vehicle_or_truck_reference, provider_membership_id, decline_reason, decline_note, 'PRIVATE_ASSIGNMENT', declined_at
		FROM fleet_partner_private_assignments a
		WHERE status='DECLINED' AND decline_reason IN('UNSAFE_VEHICLE','VEHICLE_DEFECT') AND vehicle_or_truck_reference<>''
		  AND NOT EXISTS(SELECT 1 FROM fleet_partner_safety_decline_inbox i WHERE i.organization_id=a.organization_id AND i.assignment_id=a.id AND i.reason_code=a.decline_reason)
		UNION ALL
		SELECT r.organization_id, r.assignment_id, a.asset_reference, r.membership_id, r.reason_code, '', 'MARKETPLACE_ASSIGNMENT', r.occurred_at
		FROM fleet_partner_assignment_responses r
		JOIN fleet_partner_provider_assignments a ON a.id = r.assignment_id AND a.organization_id = r.organization_id
		WHERE r.response='DECLINED' AND r.reason_code IN('UNSAFE_VEHICLE','VEHICLE_DEFECT') AND COALESCE(a.asset_reference,'')<>''
		  AND NOT EXISTS(SELECT 1 FROM fleet_partner_safety_decline_inbox i WHERE i.organization_id=r.organization_id AND i.assignment_id=r.assignment_id AND i.reason_code=r.reason_code)
		ORDER BY 8 LIMIT 100`)
	if e != nil {
		return nil, e
	}
	return collect(rows, func(row pgx.Row) (models.SafetyDecline, error) {
		var v models.SafetyDecline
		var declinedAt any
		return v, row.Scan(&v.OrganizationID, &v.AssignmentID, &v.AssetReference, &v.MembershipID, &v.ReasonCode, &v.Note, &v.Source, &declinedAt)
	})
}

// InsertDeclineInbox records a decline as handled; inserted is false for a
// decline already in the inbox.
func (r *Repository) InsertDeclineInbox(ctx context.Context, q database.Querier, v models.SafetyDecline, payloadHash string) (bool, error) {
	var inserted string
	e := r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_safety_decline_inbox(event_id,organization_id,assignment_id,asset_reference,reason_code,payload_hash)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(organization_id,assignment_id,reason_code)DO NOTHING RETURNING event_id`, ids.New("sdi"), v.OrganizationID, v.AssignmentID, v.AssetReference, v.ReasonCode, payloadHash).Scan(&inserted)
	if e == pgx.ErrNoRows {
		return false, nil
	}
	return e == nil, e
}

// LockSystemTemplate serializes creation of the organization-owned system
// template; different safety declines may be reconciled concurrently.
func (r *Repository) LockSystemTemplate(ctx context.Context, q database.Querier, org string) error {
	_, e := r.q(q).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, org+":SYSTEM_UNSAFE_VEHICLE")
	return e
}

func (r *Repository) SystemTemplateID(ctx context.Context, q database.Querier, org string) (string, error) {
	var id string
	e := r.q(q).QueryRow(ctx, `SELECT id FROM fleet_partner_inspection_templates WHERE organization_id=$1 AND name='SYSTEM_UNSAFE_VEHICLE' AND status='ACTIVE' ORDER BY created_at LIMIT 1`, org).Scan(&id)
	return id, e
}

func (r *Repository) InsertSystemTemplate(ctx context.Context, q database.Querier, id, org string, items, rules []byte) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_inspection_templates(id,organization_id,name,asset_types,service_types,checklist_items,criticality_rules)VALUES($1,$2,'SYSTEM_UNSAFE_VEHICLE','[]','[]',$3,$4)`, id, org, items, rules)
	return e
}

// InsertDeclineInspection opens the DRAFT inspection a safety decline starts.
func (r *Repository) InsertDeclineInspection(ctx context.Context, q database.Querier, id, org, assetID, templateID, membershipID, assignmentID string, defectSummary []byte) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_inspections(id,organization_id,asset_id,template_id,inspector_membership_id,assignment_id,status,defect_summary)VALUES($1,$2,$3,$4,$5,$6,'DRAFT',$7)`, id, org, assetID, templateID, membershipID, assignmentID, defectSummary)
	return e
}
