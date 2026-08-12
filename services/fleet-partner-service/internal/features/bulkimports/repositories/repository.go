// Package repositories persists bulk imports and immutable row identities.
package repositories

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

var (
	ErrNotFound = errors.New("bulk imports: not found")
	ErrConflict = errors.New("bulk imports: conflict")
)

const (
	PrefixImport  = "imp"
	PrefixRow     = "imr"
	PrefixAttempt = "ima"
)

type Repository struct{ db database.Querier }

func NewRepository(db database.Querier) *Repository { return &Repository{db} }
func (r *Repository) q(q database.Querier) database.Querier {
	if q != nil {
		return q
	}
	return r.db
}

const cols = `id,organization_id,branch_id,media_file_id,file_name,file_format,content_type,size_bytes,status,total_rows,valid_rows,invalid_rows,imported_rows,failed_rows,error_report_media_id,request_fingerprint,version,created_by,created_at,updated_at,completed_at,cancelled_at,attempts,import_type,engine,mode,schema_version,file_fingerprint,external_reference,committed_rows,validation_version`

func scan(row pgx.Row) (models.Import, error) {
	var i models.Import
	e := row.Scan(&i.ID, &i.OrganizationID, &i.BranchID, &i.MediaFileID, &i.FileName, &i.FileFormat, &i.ContentType, &i.SizeBytes, &i.Status, &i.TotalRows, &i.ValidRows, &i.InvalidRows, &i.ImportedRows, &i.FailedRows, &i.ErrorReportMediaID, &i.RequestFingerprint, &i.Version, &i.CreatedBy, &i.CreatedAt, &i.UpdatedAt, &i.CompletedAt, &i.CancelledAt, &i.Attempts, &i.ImportType, &i.Engine, &i.Mode, &i.SchemaVersion, &i.FileFingerprint, &i.ExternalReference, &i.CommittedRows, &i.ValidationVersion)
	if errors.Is(e, pgx.ErrNoRows) {
		return i, ErrNotFound
	}
	return i, e
}
func (r *Repository) Create(ctx context.Context, q database.Querier, i models.Import) (models.Import, error) {
	v, e := scan(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_bulk_imports(id,organization_id,branch_id,media_file_id,file_name,file_format,content_type,size_bytes,status,request_fingerprint,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)RETURNING `+cols, i.ID, i.OrganizationID, i.BranchID, i.MediaFileID, i.FileName, i.FileFormat, i.ContentType, i.SizeBytes, i.Status, i.RequestFingerprint, i.CreatedBy))
	if e != nil {
		var pe *pgconn.PgError
		if errors.As(e, &pe) && pe.Code == "23505" {
			return v, ErrConflict
		}
	}
	return v, e
}
func (r *Repository) Get(ctx context.Context, q database.Querier, org, id string) (models.Import, error) {
	return scan(r.q(q).QueryRow(ctx, `SELECT `+cols+` FROM fleet_partner_bulk_imports WHERE organization_id=$1 AND id=$2`, org, id))
}
func (r *Repository) List(ctx context.Context, org string) ([]models.Import, error) {
	rows, e := r.db.Query(ctx, `SELECT `+cols+` FROM fleet_partner_bulk_imports WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 100`, org)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.Import
	for rows.Next() {
		i, x := scan(rows)
		if x != nil {
			return nil, x
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (r *Repository) Cancel(ctx context.Context, q database.Querier, org, id string, version int64) (models.Import, error) {
	return scan(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_bulk_imports SET status='CANCELLED',cancelled_at=now(),lease_owner=NULL,lease_expires_at=NULL,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND status IN('UPLOADED','VALIDATING','READY','IMPORTING') RETURNING `+cols, org, id, version))
}

// Claim is the legacy (engine='LEGACY') worker's lease query, unchanged in
// every respect except the added engine filter: without it, a generic-engine
// import sitting at a status value the two vocabularies happen to share
// (e.g. 'VALIDATING') could be scooped up by this query and processed as if
// it were an order import, since this claim path — unlike every generic-flow
// entry point — never goes through a Service method that checks the engine
// first.
func (r *Repository) Claim(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.Import, error) {
	rows, e := r.db.Query(ctx, `WITH c AS(SELECT id FROM fleet_partner_bulk_imports WHERE engine='LEGACY' AND status IN('UPLOADED','VALIDATING','READY','IMPORTING')AND(lease_expires_at IS NULL OR lease_expires_at<now())ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $1)UPDATE fleet_partner_bulk_imports i SET lease_owner=$2,lease_expires_at=now()+$3::interval,attempts=attempts+1,status=CASE WHEN status='UPLOADED' THEN 'VALIDATING' WHEN status='READY' THEN 'IMPORTING' ELSE status END FROM c WHERE i.id=c.id RETURNING i.id,i.organization_id,i.branch_id,i.media_file_id,i.file_name,i.file_format,i.content_type,i.size_bytes,i.status,i.total_rows,i.valid_rows,i.invalid_rows,i.imported_rows,i.failed_rows,i.error_report_media_id,i.request_fingerprint,i.version,i.created_by,i.created_at,i.updated_at,i.completed_at,i.cancelled_at,i.attempts,i.import_type,i.engine,i.mode,i.schema_version,i.file_fingerprint,i.external_reference,i.committed_rows,i.validation_version`, limit, owner, lease.String())
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.Import
	for rows.Next() {
		i, x := scan(rows)
		if x != nil {
			return nil, x
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (r *Repository) InsertRow(ctx context.Context, q database.Querier, row models.Row) error {
	raw, err := json.Marshal(row.Data)
	if err != nil {
		return err
	}
	tag, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_bulk_import_rows(id,organization_id,import_id,row_number,row_reference,row_fingerprint,status,normalized_data,safe_error_code,safe_error_message)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)ON CONFLICT(import_id,row_reference)DO NOTHING`, row.ID, row.OrganizationID, row.ImportID, row.RowNumber, row.RowReference, row.RowFingerprint, row.Status, raw, row.SafeErrorCode, row.SafeErrorMessage)
	if e != nil || tag.RowsAffected() == 1 {
		return e
	}
	var existing string
	if e = r.q(q).QueryRow(ctx, `SELECT row_fingerprint FROM fleet_partner_bulk_import_rows
		WHERE import_id=$1 AND row_reference=$2`, row.ImportID, row.RowReference).Scan(&existing); e != nil {
		return e
	}
	if existing != row.RowFingerprint {
		return ErrConflict
	}
	return nil
}
func (r *Repository) ListRows(ctx context.Context, q database.Querier, org, id, status string) ([]models.Row, error) {
	rows, e := r.q(q).Query(ctx, `SELECT id,organization_id,import_id,row_number,row_reference,row_fingerprint,status,normalized_data,COALESCE(generated_order_id,''),safe_error_code,safe_error_message,created_at,updated_at FROM fleet_partner_bulk_import_rows WHERE organization_id=$1 AND import_id=$2 AND($3='' OR status=$3)ORDER BY row_number`, org, id, status)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.Row
	for rows.Next() {
		var x models.Row
		var raw []byte
		if e = rows.Scan(&x.ID, &x.OrganizationID, &x.ImportID, &x.RowNumber, &x.RowReference, &x.RowFingerprint, &x.Status, &raw, &x.GeneratedOrderID, &x.SafeErrorCode, &x.SafeErrorMessage, &x.CreatedAt, &x.UpdatedAt); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &x.Data); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) SetValidated(ctx context.Context, q database.Querier, id string, total, valid, invalid int, status string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_bulk_imports SET total_rows=$1,valid_rows=$2,invalid_rows=$3,status=$4,lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$5`, total, valid, invalid, status, id)
	return e
}
func (r *Repository) MarkRow(ctx context.Context, q database.Querier, id, status, order, code string) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_bulk_import_rows SET status=$1,generated_order_id=NULLIF($2,''),safe_error_code=$3,updated_at=now() WHERE id=$4 AND status='VALID'`, status, order, code, id)
	return e
}
func (r *Repository) Finalize(ctx context.Context, q database.Querier, id, status, report string, imported, failed int) error {
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_bulk_imports SET status=$1,error_report_media_id=$2,imported_rows=$3,failed_rows=$4,completed_at=now(),lease_owner=NULL,lease_expires_at=NULL,updated_at=now(),version=version+1 WHERE id=$5`, status, report, imported, failed, id)
	return e
}
func (r *Repository) Fail(ctx context.Context, owner, id, code string, max int) error {
	_, e := r.db.Exec(ctx, `UPDATE fleet_partner_bulk_imports SET status=CASE WHEN attempts>=$4 THEN 'FAILED' ELSE status END,last_error_code=$1,lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$2 AND lease_owner=$3`, code, id, owner, max)
	return e
}

func (r *Repository) Heartbeat(ctx context.Context, owner, id string, lease time.Duration) error {
	tag, err := r.db.Exec(ctx, `UPDATE fleet_partner_bulk_imports SET lease_expires_at=now()+$3::interval
		WHERE id=$2 AND lease_owner=$1 AND lease_expires_at>now()`, owner, id, lease.String())
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}

func (r *Repository) IsCancelled(ctx context.Context, org, id string) (bool, error) {
	var cancelled bool
	err := r.db.QueryRow(ctx, `SELECT status='CANCELLED' FROM fleet_partner_bulk_imports
		WHERE organization_id=$1 AND id=$2`, org, id).Scan(&cancelled)
	return cancelled, err
}

func (r *Repository) Replay(ctx context.Context, org, id string) error {
	tag, err := r.db.Exec(ctx, `UPDATE fleet_partner_bulk_imports SET status=CASE WHEN total_rows>0
		THEN 'READY' ELSE 'UPLOADED' END,attempts=0,last_error_code='',lease_owner=NULL,
		lease_expires_at=NULL,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status='FAILED'`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}
func (r *Repository) Attempt(ctx context.Context, org, id, outcome, code string, n int) error {
	_, e := r.db.Exec(ctx, `INSERT INTO fleet_partner_bulk_import_attempts(id,organization_id,import_id,attempt,outcome,safe_error_code)VALUES($1,$2,$3,$4,$5,$6)`, ids.New(PrefixAttempt), org, id, n, outcome, code)
	return e
}
