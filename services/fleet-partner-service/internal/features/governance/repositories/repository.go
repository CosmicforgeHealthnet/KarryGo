// Package repositories persists governance records. Every statement is
// tenant-scoped by organization ID.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Repository is the PostgreSQL governance store.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository builds the store.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.pool
	}
	return q
}

// WithinTx runs f in one transaction.
func (r *Repository) WithinTx(ctx context.Context, f func(pgx.Tx) error) error {
	return database.WithinTx(ctx, r.pool, f)
}

func list(rows pgx.Rows, err error) ([]map[string]any, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SecurityEvents lists the organization's authentication security events.
// Identity material is hashed at write time; only hashes and safe detail
// are ever read back.
func (r *Repository) SecurityEvents(ctx context.Context, organizationID string, since time.Time) ([]map[string]any, error) {
	return list(r.pool.Query(ctx, `SELECT row_to_json(x) FROM (SELECT id,event_type AS "eventType",severity,identity_id AS "identityId",ip_hash AS "ipHash",user_agent_hash AS "userAgentHash",detail,request_id AS "requestId",correlation_id AS "correlationId",occurred_at AS "occurredAt" FROM fleet_partner_auth_security_events WHERE organization_id=$1 AND occurred_at>=$2 ORDER BY occurred_at DESC,id LIMIT 500) x`, organizationID, since))
}

func (r *Repository) InsertPrivacyRequest(ctx context.Context, q database.Querier, id, organizationID string, in models.PrivacyRequestInput, actor string, due time.Time) (bool, error) {
	tag, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_privacy_requests(id,organization_id,request_type,subject_type,subject_reference,requested_by,reason,due_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, id, organizationID, in.RequestType, in.SubjectType, in.SubjectReference, actor, in.Reason, due)
	return tag.RowsAffected() == 1, err
}

func (r *Repository) OpenPrivacyRequest(ctx context.Context, q database.Querier, organizationID string, in models.PrivacyRequestInput) (string, error) {
	var id string
	err := r.q(q).QueryRow(ctx, `SELECT id FROM fleet_partner_privacy_requests WHERE organization_id=$1 AND request_type=$2 AND subject_type=$3 AND subject_reference=$4 AND status IN('SUBMITTED','APPROVED')`, organizationID, in.RequestType, in.SubjectType, in.SubjectReference).Scan(&id)
	return id, err
}

func (r *Repository) GetPrivacyRequest(ctx context.Context, organizationID, id string) (map[string]any, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT row_to_json(x) FROM fleet_partner_privacy_requests x WHERE organization_id=$1 AND id=$2`, organizationID, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	if err != nil {
		return nil, err
	}
	var v map[string]any
	return v, json.Unmarshal(raw, &v)
}

func (r *Repository) ListPrivacyRequests(ctx context.Context, organizationID string) ([]map[string]any, error) {
	return list(r.pool.Query(ctx, `SELECT row_to_json(x) FROM fleet_partner_privacy_requests x WHERE organization_id=$1 ORDER BY created_at DESC,id LIMIT 500`, organizationID))
}

func (r *Repository) LockPrivacyRequest(ctx context.Context, q database.Querier, organizationID, id string) (models.PrivacyState, error) {
	var v models.PrivacyState
	err := r.q(q).QueryRow(ctx, `SELECT request_type,subject_type,subject_reference,status,requested_by,version FROM fleet_partner_privacy_requests WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&v.RequestType, &v.SubjectType, &v.SubjectReference, &v.Status, &v.RequestedBy, &v.Version)
	return v, err
}

// ActiveHolds lists the active legal holds covering a subject or its whole
// organization.
func (r *Repository) ActiveHolds(ctx context.Context, q database.Querier, organizationID, scopeType, reference string) ([]string, error) {
	rows, err := r.q(q).Query(ctx, `SELECT id FROM fleet_partner_legal_holds WHERE organization_id=$1 AND status='ACTIVE' AND ((scope_type=$2 AND scope_reference=$3) OR scope_type='ORGANIZATION') ORDER BY id`, organizationID, scopeType, reference)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repository) UpdatePrivacyRequest(ctx context.Context, q database.Querier, organizationID, id, status, actor, reason string, holds []string, report json.RawMessage) (int64, error) {
	rawHolds, _ := json.Marshal(holds)
	if holds == nil {
		rawHolds = []byte("[]")
	}
	var v int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_privacy_requests SET status=$3,
 decided_by=CASE WHEN $3 IN('APPROVED','REJECTED','BLOCKED_BY_LEGAL_HOLD') THEN $4 ELSE decided_by END,
 decided_at=CASE WHEN $3 IN('APPROVED','REJECTED','BLOCKED_BY_LEGAL_HOLD') THEN now() ELSE decided_at END,
 decision_reason=CASE WHEN $3 IN('APPROVED','REJECTED','BLOCKED_BY_LEGAL_HOLD') THEN $5 ELSE decision_reason END,
 blocking_hold_ids=$6,completion_report=COALESCE($7,completion_report),
 completed_by=CASE WHEN $3='COMPLETED' THEN $4 ELSE completed_by END,completed_at=CASE WHEN $3='COMPLETED' THEN now() ELSE completed_at END,
 version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, status, actor, reason, rawHolds, nullable(report)).Scan(&v)
	return v, err
}

func nullable(v json.RawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return []byte(v)
}

func (r *Repository) InsertPrivacyHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next, actor, note string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_privacy_request_history(id,organization_id,privacy_request_id,previous_status,new_status,actor_reference,note)VALUES($1,$2,$3,$4,$5,$6,$7)`, ids.New("prh"), organizationID, id, previous, next, actor, note)
	return err
}
