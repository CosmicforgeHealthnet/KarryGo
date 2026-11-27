package repositories

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
)

const supportMessageColumns = `id,organization_id,projection_id,support_service_case_id,client_message_id::text,request_hash,visibility,file_ids,status,support_message_id,attempts,last_error_code,created_by,version,created_at,updated_at,confirmed_at`

func scanSupportMessage(row pgx.Row) (models.SupportMessageLedger, error) {
	var l models.SupportMessageLedger
	var files []byte
	err := row.Scan(&l.ID, &l.OrganizationID, &l.ProjectionID, &l.SupportCaseID, &l.ClientMessageID, &l.RequestHash, &l.Visibility, &files,
		&l.Status, &l.SupportMessageID, &l.Attempts, &l.LastErrorCode, &l.CreatedBy, &l.Version, &l.CreatedAt, &l.UpdatedAt, &l.ConfirmedAt)
	if err != nil {
		return l, err
	}
	l.FileIDs = []string{}
	if len(files) > 0 {
		err = json.Unmarshal(files, &l.FileIDs)
	}
	return l, err
}

// SupportCaseRefs resolves a projection by local or upstream ID to its local
// projection ID and support-dispute-service case ID.
func (r *Repository) SupportCaseRefs(ctx context.Context, organizationID, caseID string) (projectionID, supportCaseID string, err error) {
	err = r.pool.QueryRow(ctx, `SELECT id,support_service_case_id FROM fleet_partner_support_case_projections WHERE organization_id=$1 AND (id=$2 OR support_service_case_id=$2)`, organizationID, caseID).Scan(&projectionID, &supportCaseID)
	return projectionID, supportCaseID, err
}

// ClaimSupportMessage records a submission once per (case, client message id)
// and returns the ledger row locked FOR UPDATE in the caller's transaction.
func (r *Repository) ClaimSupportMessage(ctx context.Context, tx pgx.Tx, l models.SupportMessageLedger) (models.SupportMessageLedger, error) {
	files, err := json.Marshal(l.FileIDs)
	if err != nil {
		return l, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_support_case_messages(id,organization_id,projection_id,support_service_case_id,client_message_id,request_hash,visibility,file_ids,created_by)
VALUES($1,$2,$3,$4,$5::uuid,$6,$7,$8::jsonb,$9) ON CONFLICT (organization_id,support_service_case_id,client_message_id) DO NOTHING`,
		l.ID, l.OrganizationID, l.ProjectionID, l.SupportCaseID, l.ClientMessageID, l.RequestHash, l.Visibility, string(files), l.CreatedBy); err != nil {
		return l, err
	}
	return scanSupportMessage(tx.QueryRow(ctx, `SELECT `+supportMessageColumns+` FROM fleet_partner_support_case_messages WHERE organization_id=$1 AND support_service_case_id=$2 AND client_message_id=$3::uuid FOR UPDATE`,
		l.OrganizationID, l.SupportCaseID, l.ClientMessageID))
}

// RecordSupportMessageAttempt moves a not-yet-confirmed submission to status.
// It reports false when the row is already CONFIRMED, so a confirmation and
// its event happen exactly once however many attempts race.
func (r *Repository) RecordSupportMessageAttempt(ctx context.Context, q database.Querier, organizationID, id, status, supportMessageID, code string) (bool, error) {
	tag, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_support_case_messages SET status=$3,support_message_id=CASE WHEN $3='CONFIRMED' THEN $4 ELSE support_message_id END,
confirmed_at=CASE WHEN $3='CONFIRMED' THEN now() ELSE NULL END,attempts=attempts+1,last_error_code=$5,version=version+1,updated_at=now()
WHERE organization_id=$1 AND id=$2 AND status<>'CONFIRMED'`, organizationID, id, status, supportMessageID, code)
	return tag.RowsAffected() == 1, err
}

// GetSupportMessage reads one ledger row.
func (r *Repository) GetSupportMessage(ctx context.Context, organizationID, id string) (models.SupportMessageLedger, error) {
	return scanSupportMessage(r.pool.QueryRow(ctx, `SELECT `+supportMessageColumns+` FROM fleet_partner_support_case_messages WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// SupportMessagesNeedingReadBack lists ambiguous submissions last touched
// before the cutoff, oldest first.
func (r *Repository) SupportMessagesNeedingReadBack(ctx context.Context, organizationID string, before time.Time) ([]models.SupportMessageLedger, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+supportMessageColumns+` FROM fleet_partner_support_case_messages WHERE organization_id=$1 AND status='OUTCOME_UNKNOWN' AND updated_at<$2 ORDER BY updated_at LIMIT 50`, organizationID, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SupportMessageLedger
	for rows.Next() {
		l, err := scanSupportMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
