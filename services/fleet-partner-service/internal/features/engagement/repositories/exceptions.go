package repositories

import (
	"context"
	"encoding/json"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// LiveExceptionFor returns the live exception for a source and type, if any.
func (r *Repository) LiveExceptionFor(ctx context.Context, q database.Querier, organizationID, sourceType, sourceReference, exceptionType string) (string, error) {
	var id string
	err := r.q(q).QueryRow(ctx, `SELECT id FROM fleet_partner_operational_exceptions WHERE organization_id=$1 AND source_type=$2 AND source_reference=$3 AND exception_type=$4 AND status IN('OPEN','ACKNOWLEDGED','ESCALATED')`, organizationID, sourceType, sourceReference, exceptionType).Scan(&id)
	return id, err
}

// InsertException raises an exception; a concurrent duplicate returns false.
func (r *Repository) InsertException(ctx context.Context, q database.Querier, id, organizationID string, in models.ExceptionInput, due time.Time, actor string) (bool, error) {
	evidence, _ := json.Marshal(in.EvidenceFileIDs)
	if in.EvidenceFileIDs == nil {
		evidence = []byte("[]")
	}
	tag, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_operational_exceptions(id,organization_id,branch_id,exception_type,severity,source_type,source_reference,summary,owner_user_id,evidence_file_ids,sla_due_at,created_by)
 VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`, id, organizationID, in.BranchID, in.ExceptionType, in.Severity, in.SourceType, in.SourceReference, in.Summary, in.OwnerUserID, evidence, due, actor)
	return tag.RowsAffected() == 1, err
}

// GetException reads one exception.
func (r *Repository) GetException(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(e) FROM fleet_partner_operational_exceptions e WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// ListExceptions lists exceptions in the permitted branches, most urgent
// first.
func (r *Repository) ListExceptions(ctx context.Context, organizationID string, branchIDs []string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(e) FROM fleet_partner_operational_exceptions e WHERE organization_id=$1 AND (COALESCE(cardinality($2::text[]),0)=0 OR branch_id IS NULL OR branch_id=ANY($2::text[]))
 ORDER BY (status IN('OPEN','ACKNOWLEDGED','ESCALATED')) DESC, CASE severity WHEN 'CRITICAL' THEN 0 WHEN 'HIGH' THEN 1 WHEN 'MEDIUM' THEN 2 ELSE 3 END, sla_due_at, id LIMIT 500`, organizationID, branchIDs)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// LockException locks an exception for a transition.
func (r *Repository) LockException(ctx context.Context, q database.Querier, organizationID, id string) (models.ExceptionState, error) {
	var v models.ExceptionState
	err := r.q(q).QueryRow(ctx, `SELECT status,severity,COALESCE(branch_id,''),owner_user_id,escalation_level,version,sla_due_at FROM fleet_partner_operational_exceptions WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&v.Status, &v.Severity, &v.BranchID, &v.OwnerUserID, &v.EscalationLevel, &v.Version, &v.SLADueAt)
	return v, err
}

// UpdateException applies a transition and returns the new version.
func (r *Repository) UpdateException(ctx context.Context, q database.Querier, organizationID, id, status, severity, owner string, level int, due time.Time, rootCause, resolution string) (int64, error) {
	var v int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_operational_exceptions SET status=$3,severity=$4,owner_user_id=$5,escalation_level=$6,sla_due_at=$7,
 root_cause=CASE WHEN $8<>'' THEN $8 ELSE root_cause END,resolution=CASE WHEN $9<>'' THEN $9 ELSE resolution END,
 acknowledged_at=CASE WHEN $3='ACKNOWLEDGED' AND acknowledged_at IS NULL THEN now() ELSE acknowledged_at END,
 resolved_at=CASE WHEN $3 IN('RESOLVED','CLOSED') THEN now() ELSE resolved_at END,
 version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, status, severity, owner, level, due, rootCause, resolution).Scan(&v)
	return v, err
}

// InsertExceptionHistory appends one transition.
func (r *Repository) InsertExceptionHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next, actor, note string, version int64) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_operational_exception_history(id,organization_id,exception_id,previous_status,new_status,actor_reference,note,version)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("oxh"), organizationID, id, previous, next, actor, note, version)
	return err
}

// ExceptionHistory reads an exception's transitions.
func (r *Repository) ExceptionHistory(ctx context.Context, organizationID, id string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(h) FROM fleet_partner_operational_exception_history h WHERE organization_id=$1 AND exception_id=$2 ORDER BY occurred_at,id`, organizationID, id)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// OverdueExceptions lists live exceptions past their SLA that have not yet
// been escalated past their current level.
func (r *Repository) OverdueExceptions(ctx context.Context, organizationID string, now time.Time) ([]models.OverdueException, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,escalation_level FROM fleet_partner_operational_exceptions WHERE organization_id=$1 AND status IN('OPEN','ACKNOWLEDGED','ESCALATED') AND sla_due_at<$2 ORDER BY sla_due_at LIMIT 200`, organizationID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.OverdueException
	for rows.Next() {
		var v models.OverdueException
		if err = rows.Scan(&v.ID, &v.EscalationLevel); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListSLABreaches lists breached and at-risk SLA measurements, newest first.
func (r *Repository) ListSLABreaches(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(m) FROM fleet_partner_sla_measurements m WHERE organization_id=$1 AND breach_status IN('BREACHED','AT_RISK') ORDER BY period_end DESC,id LIMIT 500`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}
