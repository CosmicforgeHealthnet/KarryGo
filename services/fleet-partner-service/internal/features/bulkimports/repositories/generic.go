// Section 25C: the generic (engine='GENERIC') import repository surface.
//
// Every method here is additive and lives in its own file precisely so
// repository.go — the legacy (engine='LEGACY') order-import surface — never
// has to be re-read to confirm nothing changed. The two surfaces share the
// same two tables but every generic query filters on engine='GENERIC'
// explicitly; nothing here is ever claimed, listed or mutated by the legacy
// methods, and nothing legacy is ever touched by these.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// CreateGeneric inserts a new generic-engine import job.
func (r *Repository) CreateGeneric(ctx context.Context, q database.Querier, i models.Import) (models.Import, error) {
	if i.ID == "" {
		i.ID = ids.New(PrefixImport)
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_bulk_imports
			(id, organization_id, branch_id, media_file_id, file_name, file_format, content_type, size_bytes,
			 status, request_fingerprint, created_by, import_type, engine, mode, schema_version,
			 file_fingerprint, external_reference)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'GENERIC',$13,$14,$15,$16)
		RETURNING `+cols,
		i.ID, i.OrganizationID, i.BranchID, i.MediaFileID, i.FileName, i.FileFormat, i.ContentType, i.SizeBytes,
		i.Status, i.RequestFingerprint, i.CreatedBy, i.ImportType, i.Mode, i.SchemaVersion,
		i.FileFingerprint, i.ExternalReference)
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

// FindGenericByFingerprint resolves a retried creation for the HTTP-layer
// idempotency store to compare against. It is not itself the idempotency
// mechanism — internal/platform/idempotency.Store owns that at the HTTP
// layer, replaying the original response bytes — this exists only so a
// caller can inspect what an existing fingerprint maps to.
func (r *Repository) FindGenericByFingerprint(ctx context.Context, q database.Querier,
	organizationID, fingerprint string) (models.Import, error) {
	return scan(r.q(q).QueryRow(ctx, `SELECT `+cols+`
		FROM fleet_partner_bulk_imports
		WHERE organization_id=$1 AND engine='GENERIC' AND request_fingerprint=$2
		ORDER BY created_at DESC LIMIT 1`, organizationID, fingerprint))
}

// GetGeneric reads one generic-engine import inside the tenant scope,
// answering ErrNotFound for a legacy-engine row so a caller can never reach
// across engines by guessing an id.
func (r *Repository) GetGeneric(ctx context.Context, q database.Querier, organizationID, id string) (models.Import, error) {
	return scan(r.q(q).QueryRow(ctx, `SELECT `+cols+`
		FROM fleet_partner_bulk_imports WHERE organization_id=$1 AND id=$2 AND engine='GENERIC'`, organizationID, id))
}

// ListGeneric returns the organization's generic-engine imports, newest first.
func (r *Repository) ListGeneric(ctx context.Context, organizationID string) ([]models.Import, error) {
	rows, err := r.db.Query(ctx, `SELECT `+cols+`
		FROM fleet_partner_bulk_imports WHERE organization_id=$1 AND engine='GENERIC'
		ORDER BY created_at DESC LIMIT 100`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Import
	for rows.Next() {
		i, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// ClaimForValidation leases a bounded batch of jobs queued for validation.
func (r *Repository) ClaimForValidation(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.Import, error) {
	return r.claimGeneric(ctx, owner, lease, limit, "VALIDATION_PENDING", "VALIDATING")
}

// ClaimForCommit leases a bounded batch of jobs queued for commit.
func (r *Repository) ClaimForCommit(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.Import, error) {
	return r.claimGeneric(ctx, owner, lease, limit, "COMMIT_PENDING", "COMMITTING")
}

// ClaimForRetry leases a bounded batch of FAILED jobs whose retry backoff has
// elapsed, re-entering whichever phase they were last attempting.
func (r *Repository) ClaimForRetry(ctx context.Context, owner string, lease, backoff time.Duration, limit int) ([]models.Import, error) {
	if backoff <= 0 {
		backoff = 30 * time.Second
	}
	rows, err := r.db.Query(ctx, `
		WITH candidates AS (
			SELECT id FROM fleet_partner_bulk_imports
			WHERE engine='GENERIC' AND status='FAILED'
			  AND updated_at < now() - make_interval(secs => $4 * GREATEST(attempts, 1))
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE fleet_partner_bulk_imports b SET
			status = CASE WHEN b.validation_version > 0 THEN 'COMMITTING' ELSE 'VALIDATING' END,
			lease_owner=$2, lease_expires_at=now()+$3::interval, attempts=attempts+1, updated_at=now()
		FROM candidates c WHERE b.id=c.id
		RETURNING `+genericClaimReturning(),
		limit, owner, lease.String(), backoff.Seconds())
	if err != nil {
		return nil, err
	}
	return scanGenericRows(rows)
}

func (r *Repository) claimGeneric(ctx context.Context, owner string, lease time.Duration, limit int,
	fromStatus, toStatus string) ([]models.Import, error) {
	rows, err := r.db.Query(ctx, `
		WITH candidates AS (
			SELECT id FROM fleet_partner_bulk_imports
			WHERE engine='GENERIC' AND (
				status = $4
				OR (status = $5 AND (lease_expires_at IS NULL OR lease_expires_at < now()))
			)
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE fleet_partner_bulk_imports b SET
			status=$5, lease_owner=$2, lease_expires_at=now()+$3::interval, attempts=attempts+1, updated_at=now()
		FROM candidates c WHERE b.id=c.id
		RETURNING `+genericClaimReturning(),
		limit, owner, lease.String(), fromStatus, toStatus)
	if err != nil {
		return nil, err
	}
	return scanGenericRows(rows)
}

func genericClaimReturning() string {
	return `b.id,b.organization_id,b.branch_id,b.media_file_id,b.file_name,b.file_format,b.content_type,
		b.size_bytes,b.status,b.total_rows,b.valid_rows,b.invalid_rows,b.imported_rows,b.failed_rows,
		b.error_report_media_id,b.request_fingerprint,b.version,b.created_by,b.created_at,b.updated_at,
		b.completed_at,b.cancelled_at,b.attempts,b.import_type,b.engine,b.mode,b.schema_version,
		b.file_fingerprint,b.external_reference,b.committed_rows,b.validation_version`
}

func scanGenericRows(rows pgx.Rows) ([]models.Import, error) {
	defer rows.Close()
	var out []models.Import
	for rows.Next() {
		i, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// SetValidationOutcome persists a completed validation pass: row counts,
// a bumped validation_version (commit requires a validation_version that
// still reflects the current file_fingerprint/rows — enforced by the
// usecase, not here), and the resulting status.
func (r *Repository) SetValidationOutcome(ctx context.Context, q database.Querier, id string,
	total, valid, invalid int, status string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_bulk_imports SET
			total_rows=$1, valid_rows=$2, invalid_rows=$3, status=$4,
			validation_version=validation_version+1, lease_owner=NULL, lease_expires_at=NULL, updated_at=now()
		WHERE id=$5`, total, valid, invalid, status, id)
	return err
}

// FinalizeCommit records the terminal outcome of a commit pass.
func (r *Repository) FinalizeCommit(ctx context.Context, q database.Querier, id, status, report string,
	committed, failed int) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_bulk_imports SET
			status=$1, error_report_media_id=$2, committed_rows=$3, failed_rows=$4,
			completed_at=now(), lease_owner=NULL, lease_expires_at=NULL, updated_at=now(), version=version+1
		WHERE id=$5`, status, report, committed, failed, id)
	return err
}

// FailGeneric records a failed attempt at whichever phase was running,
// moving to DEAD_LETTER once the attempt ceiling is reached.
func (r *Repository) FailGeneric(ctx context.Context, owner, id, code string, maxAttempts int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_bulk_imports SET
			status = CASE WHEN attempts >= $4 THEN 'DEAD_LETTER' ELSE 'FAILED' END,
			last_error_code=$1, lease_owner=NULL, lease_expires_at=NULL, updated_at=now()
		WHERE id=$2 AND lease_owner=$3`, code, id, owner, maxAttempts)
	return err
}

// ReplayGeneric moves a DEAD_LETTER job back to the pending status for
// whichever phase it last attempted, with a clean attempt count.
func (r *Repository) ReplayGeneric(ctx context.Context, organizationID, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_bulk_imports SET
			status = CASE WHEN validation_version > 0 THEN 'COMMIT_PENDING' ELSE 'VALIDATION_PENDING' END,
			attempts=0, last_error_code='', lease_owner=NULL, lease_expires_at=NULL, updated_at=now()
		WHERE organization_id=$1 AND id=$2 AND engine='GENERIC' AND status='DEAD_LETTER'`, organizationID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}

// RetryImport re-queues every FAILED (row-level) row of a VALIDATED or
// PARTIALLY_COMMITTED import for another commit pass, without touching rows
// that already committed.
func (r *Repository) RetryImport(ctx context.Context, q database.Querier, organizationID, importID string) (int64, error) {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_bulk_import_rows SET status='VALID', commit_status='', updated_at=now()
		WHERE organization_id=$1 AND import_id=$2 AND status='FAILED'`, organizationID, importID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// RetryRow re-queues a single failed row for another commit attempt.
func (r *Repository) RetryRow(ctx context.Context, q database.Querier, organizationID, importID, rowID string) error {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_bulk_import_rows SET status='VALID', commit_status='', updated_at=now()
		WHERE organization_id=$1 AND import_id=$2 AND id=$3 AND status='FAILED'`, organizationID, importID, rowID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// RequeueImportForCommit moves a job with at least one row re-armed by
// RetryImport/RetryRow/SubmitCorrection back to COMMIT_PENDING so the commit
// worker picks it up again.
//
//   - FAILED is included alongside VALIDATED/PARTIALLY_COMMITTED because a
//     commit pass in which every row failed (0 committed) lands the whole
//     job in FAILED — see runCommit's status rule — and that is exactly the
//     case a row retry exists to recover from, not a dead end distinct from
//     PARTIALLY_COMMITTED.
//   - VALIDATION_FAILED is included because a job whose only rows were
//     INVALID lands there (see runValidation's status rule: valid==0 means
//     VALIDATION_FAILED, not VALIDATED) — correcting that one row to VALID
//     must be able to move the job straight to COMMIT_PENDING without a
//     redundant re-validation pass, since SubmitCorrection already validated
//     the corrected row synchronously through the same adapter.
func (r *Repository) RequeueImportForCommit(ctx context.Context, q database.Querier, organizationID, id string) error {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_bulk_imports SET status='COMMIT_PENDING', updated_at=now()
		WHERE organization_id=$1 AND id=$2 AND engine='GENERIC'
		  AND status IN ('VALIDATED','PARTIALLY_COMMITTED','FAILED','VALIDATION_FAILED')`,
		organizationID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// CancelGeneric guards the transition to CANCELLED from any non-terminal
// generic-engine status.
func (r *Repository) CancelGeneric(ctx context.Context, q database.Querier,
	organizationID, id string, expectedVersion int64) (models.Import, error) {
	job, err := scan(r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_bulk_imports SET
			status='CANCELLED', cancelled_at=now(), lease_owner=NULL, lease_expires_at=NULL,
			version=version+1, updated_at=now()
		WHERE organization_id=$1 AND id=$2 AND engine='GENERIC' AND version=$3
		  AND status IN ('UPLOADED','VALIDATION_PENDING','VALIDATING','VALIDATED',
		                 'COMMIT_PENDING','COMMITTING','FAILED')
		RETURNING `+cols, organizationID, id, expectedVersion))
	if errors.Is(err, ErrNotFound) {
		return job, ErrConflict
	}
	return job, err
}

// ---------------------------------------------------------------------------
// Rows
// ---------------------------------------------------------------------------

const genericRowCols = `id,organization_id,import_id,row_number,row_reference,row_fingerprint,status,
	normalized_data,COALESCE(generated_order_id,''),safe_error_code,safe_error_message,created_at,updated_at,
	external_reference,schema_version,commit_status,generated_subject_reference,row_version,
	COALESCE(replaces_row_id,'')`

func scanGenericRow(row pgx.Row) (models.Row, error) {
	var x models.Row
	var raw []byte
	err := row.Scan(&x.ID, &x.OrganizationID, &x.ImportID, &x.RowNumber, &x.RowReference, &x.RowFingerprint,
		&x.Status, &raw, &x.GeneratedOrderID, &x.SafeErrorCode, &x.SafeErrorMessage, &x.CreatedAt, &x.UpdatedAt,
		&x.ExternalReference, &x.SchemaVersion, &x.CommitStatus, &x.GeneratedSubjectReference, &x.RowVersion,
		&x.ReplacesRowID)
	if err != nil {
		return x, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &x.GenericData); err != nil {
			return x, err
		}
	}
	return x, nil
}

// InsertGenericRow persists one row version. A retried insert of an
// identical row (same import_id+row_reference, i.e. the same external
// reference and version) is harmless; one whose fingerprint changed under
// that same key is refused, exactly like the legacy InsertRow.
func (r *Repository) InsertGenericRow(ctx context.Context, q database.Querier, row models.Row) error {
	if row.ID == "" {
		row.ID = ids.New(PrefixRow)
	}
	raw, err := json.Marshal(row.GenericData)
	if err != nil {
		return err
	}
	if row.RowReference == "" {
		row.RowReference = row.ExternalReference + "#v" + strconv.Itoa(row.RowVersion)
	}
	var replaces any
	if row.ReplacesRowID != "" {
		replaces = row.ReplacesRowID
	}
	tag, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_bulk_import_rows
			(id, organization_id, import_id, row_number, row_reference, row_fingerprint, status,
			 normalized_data, safe_error_code, safe_error_message, external_reference, schema_version,
			 commit_status, row_version, replaces_row_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (import_id, row_reference) DO NOTHING`,
		row.ID, row.OrganizationID, row.ImportID, row.RowNumber, row.RowReference, row.RowFingerprint,
		row.Status, raw, row.SafeErrorCode, row.SafeErrorMessage, row.ExternalReference, row.SchemaVersion,
		row.CommitStatus, row.RowVersion, replaces)
	if err != nil || tag.RowsAffected() == 1 {
		return err
	}
	var existing string
	if err = r.q(q).QueryRow(ctx, `SELECT row_fingerprint FROM fleet_partner_bulk_import_rows
		WHERE import_id=$1 AND row_reference=$2`, row.ImportID, row.RowReference).Scan(&existing); err != nil {
		return err
	}
	if existing != row.RowFingerprint {
		return ErrConflict
	}
	return nil
}

// ListGenericRows returns every row version for an import, newest version
// last within each row_number.
func (r *Repository) ListGenericRows(ctx context.Context, q database.Querier,
	organizationID, importID, status string) ([]models.Row, error) {
	rows, err := r.q(q).Query(ctx, `SELECT `+genericRowCols+`
		FROM fleet_partner_bulk_import_rows
		WHERE organization_id=$1 AND import_id=$2 AND ($3='' OR status=$3)
		ORDER BY row_number, row_version`, organizationID, importID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Row
	for rows.Next() {
		x, err := scanGenericRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// ListLatestGenericRows returns only the newest version of each row,
// keyed by external_reference — this is the set a commit pass acts on.
func (r *Repository) ListLatestGenericRows(ctx context.Context, q database.Querier,
	organizationID, importID, status string) ([]models.Row, error) {
	rows, err := r.q(q).Query(ctx, `SELECT `+genericRowCols+`
		FROM fleet_partner_bulk_import_rows
		WHERE organization_id=$1 AND import_id=$2 AND external_reference<>'' AND ($3='' OR status=$3)
		ORDER BY external_reference, row_version DESC`, organizationID, importID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var out []models.Row
	for rows.Next() {
		x, err := scanGenericRow(rows)
		if err != nil {
			return nil, err
		}
		if seen[x.ExternalReference] {
			continue
		}
		seen[x.ExternalReference] = true
		out = append(out, x)
	}
	return out, rows.Err()
}

// MarkRowCommitted records a successful commit for one row.
func (r *Repository) MarkRowCommitted(ctx context.Context, q database.Querier, rowID, subjectReference string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_bulk_import_rows SET
			status='COMMITTED', commit_status='COMMITTED', generated_subject_reference=$1, updated_at=now()
		WHERE id=$2`, subjectReference, rowID)
	return err
}

// MarkRowFailed records a failed commit attempt for one row with a safe
// error code and message — never the raw underlying error, which may carry
// constraint names or other internal detail.
func (r *Repository) MarkRowFailed(ctx context.Context, q database.Querier, rowID, code, message string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_bulk_import_rows SET
			status='FAILED', commit_status='FAILED', safe_error_code=$1, safe_error_message=$2, updated_at=now()
		WHERE id=$3`, code, message, rowID)
	return err
}

// GetGenericRow reads one row inside the tenant and import scope.
func (r *Repository) GetGenericRow(ctx context.Context, q database.Querier,
	organizationID, importID, rowID string) (models.Row, error) {
	return scanGenericRow(r.q(q).QueryRow(ctx, `SELECT `+genericRowCols+`
		FROM fleet_partner_bulk_import_rows WHERE organization_id=$1 AND import_id=$2 AND id=$3`,
		organizationID, importID, rowID))
}

// SubmitCorrectedRow inserts a new, immutable row version replacing an
// INVALID or FAILED row, guarded by the row's own version (the caller must
// have read the exact row version they are correcting). The original row is
// never touched — this only ever inserts. Returns ErrConflict if the row is
// not in a correctable state or the expected version is stale.
func (r *Repository) SubmitCorrectedRow(ctx context.Context, q database.Querier, original models.Row,
	expectedRowVersion int, status, rowFingerprint string, data any) (models.Row, error) {
	if original.RowVersion != expectedRowVersion {
		return models.Row{}, ErrConflict
	}
	if original.Status != models.RowStatusInvalid && original.Status != models.RowStatusFailed {
		return models.Row{}, ErrConflict
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return models.Row{}, err
	}
	corrected := models.Row{
		ID:                ids.New(PrefixRow),
		OrganizationID:    original.OrganizationID,
		ImportID:          original.ImportID,
		RowNumber:         original.RowNumber,
		Status:            status,
		RowFingerprint:    rowFingerprint,
		ExternalReference: original.ExternalReference,
		SchemaVersion:     original.SchemaVersion,
		RowVersion:        original.RowVersion + 1,
		ReplacesRowID:     original.ID,
	}
	corrected.RowReference = corrected.ExternalReference + "#v" + strconv.Itoa(corrected.RowVersion)
	tag, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_bulk_import_rows
			(id, organization_id, import_id, row_number, row_reference, row_fingerprint, status,
			 normalized_data, external_reference, schema_version, commit_status, row_version, replaces_row_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'',$11,$12)
		ON CONFLICT (import_id, row_reference) DO NOTHING`,
		corrected.ID, corrected.OrganizationID, corrected.ImportID, corrected.RowNumber, corrected.RowReference,
		corrected.RowFingerprint, corrected.Status, raw, corrected.ExternalReference, corrected.SchemaVersion,
		corrected.RowVersion, corrected.ReplacesRowID)
	if err != nil {
		return models.Row{}, err
	}
	if tag.RowsAffected() == 0 {
		// Another request already created this exact version (same row,
		// same target version) — a genuine concurrent correction, not this
		// caller's problem to resolve silently.
		return models.Row{}, ErrConflict
	}
	return r.GetGenericRow(ctx, q, corrected.OrganizationID, corrected.ImportID, corrected.ID)
}
