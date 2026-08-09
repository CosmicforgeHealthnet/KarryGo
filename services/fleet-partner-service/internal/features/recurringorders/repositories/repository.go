// Package repositories persists recurring templates and leased occurrences.
package repositories

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

var (
	ErrNotFound = errors.New("recurring orders: not found")
	ErrConflict = errors.New("recurring orders: conflict")
)

const (
	PrefixTemplate   = "rot"
	PrefixOccurrence = "roc"
	PrefixAttempt    = "roa"
)

type Repository struct{ db database.Querier }

func NewRepository(db database.Querier) *Repository { return &Repository{db} }
func (r *Repository) q(q database.Querier) database.Querier {
	if q != nil {
		return q
	}
	return r.db
}

const cols = `id,organization_id,branch_id,customer_id,service_catalog_item_id,contract_id,template_data,recurrence_type,normalized_rrule,timezone,start_date,end_date,next_run_at,missed_run_policy,generated_order_initial_state,status,version,created_by,updated_by,created_at,updated_at,activated_at,deactivated_at,attempts,last_error_code`

func scan(row pgx.Row) (models.Template, error) {
	var t models.Template
	var raw []byte
	e := row.Scan(&t.ID, &t.OrganizationID, &t.BranchID, &t.CustomerID, &t.ServiceCatalogItemID, &t.ContractID, &raw, &t.RecurrenceType, &t.NormalizedRRule, &t.Timezone, &t.StartDate, &t.EndDate, &t.NextRunAt, &t.MissedRunPolicy, &t.GeneratedOrderInitialState, &t.Status, &t.Version, &t.CreatedBy, &t.UpdatedBy, &t.CreatedAt, &t.UpdatedAt, &t.ActivatedAt, &t.DeactivatedAt, &t.Attempts, &t.LastErrorCode)
	if errors.Is(e, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	if e == nil {
		e = json.Unmarshal(raw, &t.Data)
	}
	return t, e
}
func (r *Repository) Create(ctx context.Context, q database.Querier, t models.Template) (models.Template, error) {
	raw, _ := json.Marshal(t.Data)
	return scan(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_recurring_order_templates(id,organization_id,branch_id,customer_id,service_catalog_item_id,contract_id,template_data,recurrence_type,normalized_rrule,timezone,start_date,end_date,next_run_at,missed_run_policy,generated_order_initial_state,status,created_by,updated_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)RETURNING `+cols, t.ID, t.OrganizationID, t.BranchID, t.CustomerID, t.ServiceCatalogItemID, t.ContractID, raw, t.RecurrenceType, t.NormalizedRRule, t.Timezone, t.StartDate, t.EndDate, t.NextRunAt, t.MissedRunPolicy, t.GeneratedOrderInitialState, t.Status, t.CreatedBy))
}
func (r *Repository) Get(ctx context.Context, q database.Querier, org, id string) (models.Template, error) {
	return scan(r.q(q).QueryRow(ctx, `SELECT `+cols+` FROM fleet_partner_recurring_order_templates WHERE organization_id=$1 AND id=$2`, org, id))
}
func (r *Repository) List(ctx context.Context, org, status string) ([]models.Template, error) {
	rows, e := r.db.Query(ctx, `SELECT `+cols+` FROM fleet_partner_recurring_order_templates WHERE organization_id=$1 AND($2='' OR status=$2)ORDER BY created_at DESC LIMIT 100`, org, status)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.Template
	for rows.Next() {
		t, x := scan(rows)
		if x != nil {
			return nil, x
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (r *Repository) SetStatus(ctx context.Context, q database.Querier, org, id string, version int64, status, actor string) (models.Template, error) {
	t, e := scan(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_recurring_order_templates SET status=$1,updated_by=$2,version=version+1,updated_at=now(),activated_at=CASE WHEN $1='ACTIVE' THEN now() ELSE activated_at END,deactivated_at=CASE WHEN $1='INACTIVE' THEN now() ELSE deactivated_at END WHERE organization_id=$3 AND id=$4 AND version=$5 RETURNING `+cols, status, actor, org, id, version))
	if errors.Is(e, ErrNotFound) {
		return t, ErrConflict
	}
	return t, e
}
func (r *Repository) ClaimDue(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.Template, error) {
	rows, e := r.db.Query(ctx, `WITH c AS(SELECT id FROM fleet_partner_recurring_order_templates WHERE status='ACTIVE' AND next_run_at<=now() AND(lease_expires_at IS NULL OR lease_expires_at<now())ORDER BY next_run_at FOR UPDATE SKIP LOCKED LIMIT $1)UPDATE fleet_partner_recurring_order_templates t SET lease_owner=$2,lease_expires_at=now()+$3::interval,attempts=attempts+1 FROM c WHERE t.id=c.id RETURNING t.id,t.organization_id,t.branch_id,t.customer_id,t.service_catalog_item_id,t.contract_id,t.template_data,t.recurrence_type,t.normalized_rrule,t.timezone,t.start_date,t.end_date,t.next_run_at,t.missed_run_policy,t.generated_order_initial_state,t.status,t.version,t.created_by,t.updated_by,t.created_at,t.updated_at,t.activated_at,t.deactivated_at,t.attempts,t.last_error_code`, limit, owner, lease.String())
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.Template
	for rows.Next() {
		t, x := scan(rows)
		if x != nil {
			return nil, x
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (r *Repository) Heartbeat(ctx context.Context, owner, id string, lease time.Duration) error {
	tag, e := r.db.Exec(ctx, `UPDATE fleet_partner_recurring_order_templates SET lease_expires_at=now()+$3::interval WHERE id=$2 AND lease_owner=$1 AND lease_expires_at>now()`, owner, id, lease.String())
	if e == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return e
}
func (r *Repository) CreateOccurrence(ctx context.Context, q database.Querier, o models.Occurrence) (models.Occurrence, bool, error) {
	o.ID = ids.New(PrefixOccurrence)
	cmd, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_recurring_occurrences(id,organization_id,template_id,occurrence_key,scheduled_for,status,lease_owner)VALUES($1,$2,$3,$4,$5,'CLAIMED',$6)ON CONFLICT(template_id,occurrence_key)DO NOTHING`, o.ID, o.OrganizationID, o.TemplateID, o.OccurrenceKey, o.ScheduledFor, o.LeaseOwner)
	created := e == nil && cmd.RowsAffected() == 1
	if e != nil || created {
		return o, created, e
	}
	e = r.q(q).QueryRow(ctx, `SELECT id,organization_id,template_id,occurrence_key,scheduled_for,status,
		COALESCE(generated_order_id,''),attempts,last_error_code,COALESCE(lease_owner,''),lease_expires_at,
		created_at,updated_at FROM fleet_partner_recurring_occurrences WHERE template_id=$1 AND occurrence_key=$2`,
		o.TemplateID, o.OccurrenceKey).Scan(&o.ID, &o.OrganizationID, &o.TemplateID, &o.OccurrenceKey,
		&o.ScheduledFor, &o.Status, &o.GeneratedOrderID, &o.Attempts, &o.LastErrorCode, &o.LeaseOwner,
		&o.LeaseExpiresAt, &o.CreatedAt, &o.UpdatedAt)
	return o, false, e
}
func (r *Repository) CompleteOccurrence(ctx context.Context, q database.Querier, o models.Occurrence, orderID string, next time.Time) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_recurring_occurrences SET status='GENERATED',generated_order_id=$1,updated_at=now(),lease_owner=NULL,lease_expires_at=NULL WHERE id=$2`, orderID, o.ID)
	if e != nil {
		return e
	}
	_, e = r.q(q).Exec(ctx, `UPDATE fleet_partner_recurring_order_templates SET next_run_at=$1,lease_owner=NULL,lease_expires_at=NULL,last_error_code='',updated_at=now() WHERE id=$2`, next, o.TemplateID)
	return e
}
func (r *Repository) Fail(ctx context.Context, owner, id, code string, max int) error {
	_, e := r.db.Exec(ctx, `UPDATE fleet_partner_recurring_order_templates SET last_error_code=$1,status=CASE WHEN attempts>=$4 THEN 'INACTIVE' ELSE status END,lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$2 AND lease_owner=$3`, code, id, owner, max)
	return e
}

func (r *Repository) FailOccurrence(ctx context.Context, owner string, template models.Template,
	occurrence models.Occurrence, code string, max int) error {
	pool, ok := r.db.(*pgxpool.Pool)
	if !ok {
		return errors.New("recurring orders: transaction pool unavailable")
	}
	return database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		status := "FAILED"
		if template.Attempts >= max {
			status = "DEAD_LETTER"
		}
		if _, err := tx.Exec(ctx, `UPDATE fleet_partner_recurring_occurrences SET status=$1,
			attempts=attempts+1,last_error_code=$2,lease_owner=NULL,lease_expires_at=NULL,updated_at=now()
			WHERE id=$3`, status, code, occurrence.ID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE fleet_partner_recurring_order_templates SET last_error_code=$1,
			status=CASE WHEN $4 THEN 'INACTIVE' ELSE status END,lease_owner=NULL,lease_expires_at=NULL,
			updated_at=now() WHERE id=$2 AND lease_owner=$3`, code, template.ID, owner,
			template.Attempts >= max)
		return err
	})
}

func (r *Repository) Replay(ctx context.Context, org, id string) error {
	tag, err := r.db.Exec(ctx, `WITH replayed AS(
		UPDATE fleet_partner_recurring_occurrences SET status='CLAIMED',attempts=0,last_error_code='',
		lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=(SELECT id FROM
		fleet_partner_recurring_occurrences WHERE organization_id=$1 AND template_id=$2 AND
		status='DEAD_LETTER' ORDER BY scheduled_for DESC LIMIT 1) RETURNING template_id)
		UPDATE fleet_partner_recurring_order_templates t SET status='ACTIVE',attempts=0,last_error_code='',
		next_run_at=LEAST(next_run_at,now()),lease_owner=NULL,lease_expires_at=NULL,updated_at=now()
		FROM replayed WHERE t.organization_id=$1 AND t.id=replayed.template_id`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}
func (r *Repository) Attempt(ctx context.Context, q database.Querier, org, occ string, n int, outcome, code string) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_recurring_attempts(id,organization_id,occurrence_id,attempt,outcome,safe_error_code)VALUES($1,$2,$3,$4,$5,$6)`, ids.New(PrefixAttempt), org, occ, n, outcome, code)
	if e != nil {
		return fmt.Errorf("recurring attempt: %w", e)
	}
	return nil
}
