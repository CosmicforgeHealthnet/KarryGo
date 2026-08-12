// Package repositories persists export jobs and their immutable attempt
// history, mirroring the bulk-imports persistence shape this feature is a
// sibling of.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

var (
	ErrNotFound = errors.New("exports: not found")
	ErrConflict = errors.New("exports: conflict")
)

const (
	PrefixExport  = "exp"
	PrefixAttempt = "exa"
)

type Repository struct{ db database.Querier }

func NewRepository(db database.Querier) *Repository { return &Repository{db} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q != nil {
		return q
	}
	return r.db
}

const cols = `id,organization_id,COALESCE(branch_id,''),export_type,format,filter_snapshot,requested_columns,
	status,request_fingerprint,idempotency_key,source_cutoff_at,media_file_id,row_count,expires_at,
	created_by,version,created_at,updated_at,completed_at,cancelled_at,attempts,last_error_code`

func scan(row pgx.Row) (models.Job, error) {
	var j models.Job
	var filterRaw, columnsRaw []byte
	err := row.Scan(&j.ID, &j.OrganizationID, &j.BranchID, &j.ExportType, &j.Format, &filterRaw, &columnsRaw,
		&j.Status, &j.RequestFingerprint, &j.IdempotencyKey, &j.SourceCutoffAt, &j.MediaFileID, &j.RowCount,
		&j.ExpiresAt, &j.CreatedBy, &j.Version, &j.CreatedAt, &j.UpdatedAt, &j.CompletedAt, &j.CancelledAt,
		&j.Attempts, &j.LastErrorCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return j, ErrNotFound
	}
	if err != nil {
		return j, err
	}
	if len(filterRaw) > 0 {
		if err := json.Unmarshal(filterRaw, &j.FilterSnapshot); err != nil {
			return j, err
		}
	}
	if len(columnsRaw) > 0 {
		if err := json.Unmarshal(columnsRaw, &j.RequestedColumns); err != nil {
			return j, err
		}
	}
	return j, nil
}

// Create inserts a new export job in PENDING.
func (r *Repository) Create(ctx context.Context, q database.Querier, j models.Job) (models.Job, error) {
	if j.ID == "" {
		j.ID = ids.New(PrefixExport)
	}
	filterRaw, err := json.Marshal(j.FilterSnapshot.Clone())
	if err != nil {
		return models.Job{}, err
	}
	columns := j.RequestedColumns
	if columns == nil {
		columns = []string{}
	}
	columnsRaw, err := json.Marshal(columns)
	if err != nil {
		return models.Job{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_export_jobs
			(id,organization_id,branch_id,export_type,format,filter_snapshot,requested_columns,
			 status,request_fingerprint,idempotency_key,created_by)
		VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING `+cols,
		j.ID, j.OrganizationID, j.BranchID, j.ExportType, j.Format, filterRaw, columnsRaw,
		j.Status, j.RequestFingerprint, j.IdempotencyKey, j.CreatedBy)
	created, err := scan(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return created, ErrConflict
		}
		return created, err
	}
	return created, nil
}

// FindByIdempotencyKey resolves a retried request.
func (r *Repository) FindByIdempotencyKey(ctx context.Context, q database.Querier,
	organizationID, key string) (models.Job, error) {
	return scan(r.q(q).QueryRow(ctx, `SELECT `+cols+`
		FROM fleet_partner_export_jobs WHERE organization_id=$1 AND idempotency_key=$2 AND idempotency_key<>''`,
		organizationID, key))
}

// Get reads one job inside the tenant scope.
func (r *Repository) Get(ctx context.Context, q database.Querier, organizationID, id string) (models.Job, error) {
	return scan(r.q(q).QueryRow(ctx, `SELECT `+cols+`
		FROM fleet_partner_export_jobs WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// List returns the organization's export jobs, newest first.
func (r *Repository) List(ctx context.Context, organizationID string, limit int) ([]models.Job, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `SELECT `+cols+`
		FROM fleet_partner_export_jobs WHERE organization_id=$1 ORDER BY created_at DESC LIMIT $2`,
		organizationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Job
	for rows.Next() {
		j, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// Cancel guards the transition to CANCELLED from a non-terminal status.
func (r *Repository) Cancel(ctx context.Context, q database.Querier,
	organizationID, id string, expectedVersion int64) (models.Job, error) {
	job, err := scan(r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_export_jobs SET
			status='CANCELLED', cancelled_at=now(), lease_owner=NULL, lease_expires_at=NULL,
			version=version+1, updated_at=now()
		WHERE organization_id=$1 AND id=$2 AND version=$3 AND status IN ('PENDING','RUNNING')
		RETURNING `+cols, organizationID, id, expectedVersion))
	if errors.Is(err, ErrNotFound) {
		return job, ErrConflict
	}
	return job, err
}

// Claim leases a bounded batch of runnable jobs: fresh PENDING jobs, jobs
// whose lease has expired (a worker that died mid-run), and FAILED jobs whose
// retry backoff has elapsed. DEAD_LETTER jobs are never claimed automatically
// — only an explicit Replay moves one back to PENDING.
func (r *Repository) Claim(ctx context.Context, owner string, lease time.Duration,
	backoff time.Duration, limit int) ([]models.Job, error) {
	if backoff <= 0 {
		backoff = 30 * time.Second
	}
	backoffSeconds := backoff.Seconds()
	rows, err := r.db.Query(ctx, `
		WITH candidates AS (
			SELECT id FROM fleet_partner_export_jobs
			WHERE (status='PENDING')
			   OR (status='RUNNING' AND (lease_expires_at IS NULL OR lease_expires_at < now()))
			   OR (status='FAILED'
			       AND updated_at < now() - make_interval(secs => $3 * GREATEST(attempts, 1)))
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE fleet_partner_export_jobs e SET
			status='RUNNING', lease_owner=$2, lease_expires_at=now()+$4::interval,
			attempts=attempts+1, updated_at=now()
		FROM candidates c WHERE e.id=c.id
		RETURNING `+qualified(),
		limit, owner, backoffSeconds, lease.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Job
	for rows.Next() {
		j, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// qualified prefixes every column in cols with the e. alias Claim's UPDATE
// statement requires, without hand-duplicating the column list.
func qualified() string {
	return `e.id,e.organization_id,COALESCE(e.branch_id,''),e.export_type,e.format,e.filter_snapshot,
		e.requested_columns,e.status,e.request_fingerprint,e.idempotency_key,e.source_cutoff_at,
		e.media_file_id,e.row_count,e.expires_at,e.created_by,e.version,e.created_at,e.updated_at,
		e.completed_at,e.cancelled_at,e.attempts,e.last_error_code`
}

// Heartbeat extends a held lease. Called periodically during a long-running
// generation pass so a live worker's job is never reclaimed out from under it.
func (r *Repository) Heartbeat(ctx context.Context, owner, id string, lease time.Duration) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_export_jobs SET lease_expires_at=now()+$3::interval
		WHERE id=$2 AND lease_owner=$1 AND lease_expires_at > now()`,
		owner, id, lease.String())
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}

// Complete marks a job READY with its produced file.
func (r *Repository) Complete(ctx context.Context, q database.Querier, id, mediaFileID string,
	rowCount int, expiresAt *time.Time) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_export_jobs SET
			status='READY', media_file_id=$1, row_count=$2, expires_at=$3,
			completed_at=now(), lease_owner=NULL, lease_expires_at=NULL, updated_at=now(), version=version+1
		WHERE id=$4`, mediaFileID, rowCount, expiresAt, id)
	return err
}

// Fail records a failed attempt. Below the attempt ceiling the job goes back
// to FAILED (retryable by the next claim pass once its backoff elapses);
// at or above the ceiling it moves to DEAD_LETTER, where only an operator
// replay can revive it.
func (r *Repository) Fail(ctx context.Context, owner, id, code string, maxAttempts int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_export_jobs SET
			status = CASE WHEN attempts >= $4 THEN 'DEAD_LETTER' ELSE 'FAILED' END,
			last_error_code=$1, lease_owner=NULL, lease_expires_at=NULL, updated_at=now()
		WHERE id=$2 AND lease_owner=$3`, code, id, owner, maxAttempts)
	return err
}

// Replay moves a DEAD_LETTER job back to PENDING with a clean attempt count,
// for an operator who has addressed whatever was failing it.
func (r *Repository) Replay(ctx context.Context, organizationID, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_export_jobs SET
			status='PENDING', attempts=0, last_error_code='', lease_owner=NULL, lease_expires_at=NULL,
			updated_at=now(), version=version+1
		WHERE organization_id=$1 AND id=$2 AND status='DEAD_LETTER'`, organizationID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}

// ExpireReady marks READY jobs whose expiry has passed. Returns the count
// affected so the worker pass can report it.
func (r *Repository) ExpireReady(ctx context.Context, batch int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_export_jobs SET status='EXPIRED', updated_at=now(), version=version+1
		WHERE id IN (
			SELECT id FROM fleet_partner_export_jobs
			WHERE status='READY' AND expires_at IS NOT NULL AND expires_at < now()
			ORDER BY expires_at LIMIT $1
		)`, batch)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Attempt appends one immutable attempt record.
func (r *Repository) Attempt(ctx context.Context, organizationID, exportID, outcome, code string, n int) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO fleet_partner_export_attempts (id,organization_id,export_id,attempt,outcome,safe_error_code)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		ids.New(PrefixAttempt), organizationID, exportID, n, outcome, code)
	return err
}

// ListAttempts returns one job's immutable attempt history, newest first.
func (r *Repository) ListAttempts(ctx context.Context, organizationID, exportID string) ([]models.Attempt, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id,organization_id,export_id,attempt,outcome,safe_error_code,occurred_at
		FROM fleet_partner_export_attempts WHERE organization_id=$1 AND export_id=$2
		ORDER BY occurred_at DESC`, organizationID, exportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Attempt
	for rows.Next() {
		var a models.Attempt
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.ExportID, &a.Attempt, &a.Outcome,
			&a.SafeErrorCode, &a.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
