package crm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 8H — the export generator.
//
// The rule the whole worker is built around: NOTHING BECOMES READABLE UNTIL IT
// IS COMPLETE AND STILL LAWFUL. Generation, privacy re-certification, upload
// and promotion happen in that order, and the promotion is a guarded update
// that a concurrent cancellation wins. A worker that dies at any point leaves
// either a QUEUED job or a RUNNING one whose lease will lapse — never a
// half-written file somebody can download.

// RunExportGenerator claims and runs up to limit export jobs.
func (s *Service) RunExportGenerator(ctx context.Context, workerID string, limit int) (int, error) {
	if !s.flags.CRMEnabled || !s.flags.ExportsEnabled || s.flags.CommerceKillSwitch {
		return 0, nil
	}
	if limit <= 0 {
		limit = 1
	}
	processed := 0
	for i := 0; i < limit; i++ {
		export, claimed, err := s.claimExport(ctx, workerID)
		if err != nil {
			return processed, err
		}
		if !claimed {
			return processed, nil
		}
		processed++
		if err = s.generateExport(ctx, export); err != nil {
			if failErr := s.failExport(ctx, export, err); failErr != nil {
				return processed, failErr
			}
		}
	}
	return processed, nil
}

func (s *Service) claimExport(ctx context.Context, workerID string) (Export, bool, error) {
	lease := fmt.Sprintf("%d milliseconds", s.exports.LeaseTTL.Milliseconds())
	export, err := scanExport(s.db.QueryRow(ctx, `UPDATE crm_exports SET
			status='RUNNING', lease_owner=$1, lease_expires_at=now() + $2::interval,
			started_at=COALESCE(started_at, now()), worker_attempt_count=worker_attempt_count+1,
			next_attempt_at=now() + $2::interval, updated_at=now(), version=version+1
		WHERE id = (
			SELECT id FROM crm_exports
			WHERE status IN ('QUEUED','RUNNING')
			  AND next_attempt_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at <= now())
			ORDER BY next_attempt_at ASC, created_at ASC
			LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING `+exportColumns, workerID, lease))
	if errors.Is(err, pgx.ErrNoRows) {
		return Export{}, false, nil
	}
	if err != nil {
		return Export{}, false, err
	}
	return export, true, nil
}

func (s *Service) failExport(ctx context.Context, export Export, cause error) error {
	code := "GENERATION_ERROR"
	var appErr *apperrors.Error
	if errors.As(cause, &appErr) {
		if appErr.Details != nil {
			if value, ok := appErr.Details["code"].(string); ok {
				code = value
			}
		}
	}
	if len(code) > 60 {
		code = code[:60]
	}
	status := ExportQueued
	if export.Attempts >= s.exports.MaxAttempts {
		status = ExportFailed
	}
	backoff := fmt.Sprintf("%d milliseconds", s.exports.Backoff.Milliseconds()*int64(export.Attempts))
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	updated, err := scanExport(tx.QueryRow(ctx, `UPDATE crm_exports SET status=$2, error_code=$3,
		failed_at=CASE WHEN $2='FAILED' THEN now() ELSE failed_at END,
		lease_owner=NULL, lease_expires_at=NULL, next_attempt_at=now() + $4::interval,
		updated_at=now(), version=version+1
		WHERE id=$1 AND status='RUNNING' RETURNING `+exportColumns, export.ID, status, code, backoff))
	if errors.Is(err, pgx.ErrNoRows) {
		// The job was cancelled while it ran. That outcome wins.
		return nil
	}
	if err != nil {
		return err
	}
	if status == ExportFailed {
		if err = s.auditExport(ctx, Actor{StaffID: "export-generator", VendorID: export.BusinessID},
			updated, "crm.export.failed", map[string]any{"error_code": code}); err != nil {
			return err
		}
		if err = s.publishExportEvent(ctx, tx, updated, "vendor.crm.export.failed",
			map[string]any{"error_code": code}); err != nil {
			return err
		}
		s.metrics.Inc("crm_export_failed_total", "export_type", export.ExportType)
	}
	return tx.Commit(ctx)
}

// generateExport produces one artifact and promotes the job.
func (s *Service) generateExport(ctx context.Context, export Export) error {
	if s.artifacts == nil {
		return errExportStorageUnavailable()
	}
	dataset, ok := ExportDatasetFor(export.ExportType)
	if !ok {
		return crmError(apperrors.Conflict("Unsupported export type.", nil), CodeExportInvalidType)
	}
	started := time.Now()

	// The instant the generating SELECT begins, read from the database clock.
	// Privacy re-certification later asks whether any relationship in this
	// tenant was redacted after it, which is exactly the window in which a row
	// could have been read while still eligible and become ineligible before
	// publish. It must come from the same clock as the updated_at values it
	// will be compared against — a service clock even slightly behind the
	// database would make every run look like it raced a redaction.
	var generationStartedAt time.Time
	if err := s.db.QueryRow(ctx, `SELECT now()`).Scan(&generationStartedAt); err != nil {
		return err
	}

	if err := s.auditExport(ctx, Actor{StaffID: "export-generator", VendorID: export.BusinessID},
		export, "crm.export.started", nil); err != nil {
		return err
	}

	content, rowCount, err := s.renderExport(ctx, dataset, export)
	if err != nil {
		return err
	}

	certified, err := s.certifyExportPrivacy(ctx, export, generationStartedAt)
	if err != nil {
		return err
	}
	if !certified {
		// A relationship stopped being exportable while the file was being
		// written. The bytes are discarded, not filtered: rebuilding from a
		// fresh fence is the only way to be sure nothing partial or stale
		// survives, and the dataset queries already exclude redacted rows.
		return s.requeueExportForPrivacy(ctx, export)
	}

	checksum := sha256.Sum256(content)
	mediaType := "text/csv"
	extension := "csv"
	if export.Format == FormatJSONL {
		mediaType = "application/x-ndjson"
		extension = "jsonl"
	}
	stored, err := s.artifacts.Put(ctx, PutArtifactInput{
		BusinessID: export.BusinessID, ExportID: export.ID,
		Filename:  fmt.Sprintf("%s-%s.%s", strings.ToLower(export.ExportType), export.ID, extension),
		MediaType: mediaType, Content: content,
		// Keyed on the job, so a retried generation reuses one artifact instead
		// of orphaning the first.
		IdempotencyKey: "crm-export-" + export.ID,
		RetainUntil:    s.now().Add(s.exports.ArtifactTTL),
	})
	if err != nil {
		return errExportStorageUnavailable()
	}
	if stored.Checksum != "" && stored.Checksum != hex.EncodeToString(checksum[:]) {
		// The store disagrees about what it holds. Publishing anyway would mean
		// releasing bytes nobody has verified.
		_ = s.artifacts.Delete(ctx, export.BusinessID, stored.Ref, "checksum_mismatch")
		return crmError(apperrors.Conflict("Stored export did not match its checksum.", nil),
			CodeExportStorageUnavailable)
	}

	if err = s.publishExport(ctx, export, stored, rowCount, hex.EncodeToString(checksum[:])); err != nil {
		return err
	}
	s.metrics.Inc("crm_export_completed_total", "export_type", export.ExportType)
	s.metrics.Add("crm_export_rows_total", float64(rowCount), "export_type", export.ExportType)
	s.metrics.Add("crm_export_generation_duration_seconds", time.Since(started).Seconds())
	return nil
}

// renderExport streams the dataset and encodes it.
//
// Rows are streamed from PostgreSQL rather than collected: the result set is
// never materialised as Go structs, and the encoder writes each row as it
// arrives. What is held in memory is the encoded artifact, bounded by both the
// job's row limit and MaxBytes, because the private store accepts a completed
// object rather than a stream. Exceeding either bound fails the job — a
// truncated export is a wrong answer that looks like a right one.
func (s *Service) renderExport(ctx context.Context, dataset exportDataset, export Export) ([]byte, int64, error) {
	query, args := buildExportQuery(dataset, export)
	// One extra row so an export that would exceed its limit is detected rather
	// than silently truncated.
	args = append(args, export.RowLimit+1)
	query += " LIMIT $" + itoa(len(args))

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	buffer := &bytes.Buffer{}
	names := make([]string, 0, len(dataset.Fields))
	for _, field := range dataset.Fields {
		names = append(names, field.Name)
	}
	var writer *csv.Writer
	var encoder *json.Encoder
	if export.Format == FormatCSV {
		writer = csv.NewWriter(buffer)
		if err = writer.Write(names); err != nil {
			return nil, 0, err
		}
	} else {
		encoder = json.NewEncoder(buffer)
	}

	var rowCount int64
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, 0, err
		}
		rowCount++
		if rowCount > int64(export.RowLimit) {
			return nil, 0, errExportTooLarge()
		}
		if export.Format == FormatCSV {
			record := make([]string, len(values))
			for index, value := range values {
				record[index] = csvValue(value)
			}
			if err = writer.Write(record); err != nil {
				return nil, 0, err
			}
			writer.Flush()
		} else {
			object := make(map[string]any, len(names))
			for index, name := range names {
				object[name] = jsonValue(values[index])
			}
			if err = encoder.Encode(object); err != nil {
				return nil, 0, err
			}
		}
		if int64(buffer.Len()) > s.exports.MaxBytes {
			return nil, 0, errExportTooLarge()
		}
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	if writer != nil {
		writer.Flush()
		if err = writer.Error(); err != nil {
			return nil, 0, err
		}
	}
	return buffer.Bytes(), rowCount, nil
}

// csvValue renders one value. A NULL becomes an empty cell rather than the
// string "null", and nothing is quoted into a formula: a leading =, +, - or @
// is prefixed so a spreadsheet treats it as text.
func csvValue(value any) string {
	if value == nil {
		return ""
	}
	rendered := ""
	switch typed := value.(type) {
	case string:
		rendered = typed
	case time.Time:
		rendered = typed.UTC().Format(time.RFC3339)
	case []byte:
		rendered = string(typed)
	default:
		rendered = fmt.Sprintf("%v", typed)
	}
	if rendered != "" && strings.ContainsRune("=+-@\t\r", rune(rendered[0])) {
		return "'" + rendered
	}
	return rendered
}

func jsonValue(value any) any {
	if typed, ok := value.(time.Time); ok {
		return typed.UTC().Format(time.RFC3339)
	}
	if typed, ok := value.([]byte); ok {
		return string(typed)
	}
	return value
}

// certifyExportPrivacy asks whether anything stopped being exportable while
// the file was being written.
//
// It is deliberately conservative: any redaction, merge or archival in the
// tenant after the generating SELECT began fails certification, even if that
// relationship was not in the file. Being wrong in this direction costs one
// regeneration; being wrong in the other direction publishes a customer who
// asked to be forgotten.
func (s *Service) certifyExportPrivacy(ctx context.Context, export Export, since time.Time) (bool, error) {
	if exportProfileColumn(export.ExportType) == "" {
		// The dataset is not about individual customers, so there is nothing
		// for a privacy action to invalidate.
		return true, nil
	}
	var affected int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM vendor_customer_profiles
		WHERE business_id=$1 AND updated_at > $2
		  AND (privacy_state='REDACTED' OR merge_state='MERGED_SOURCE' OR status='ARCHIVED')`,
		export.BusinessID, since).Scan(&affected); err != nil {
		return false, err
	}
	return affected == 0, nil
}

// requeueExportForPrivacy sends a job back to the queue with a fresh fence.
func (s *Service) requeueExportForPrivacy(ctx context.Context, export Export) error {
	status := ExportQueued
	if export.Attempts >= s.exports.MaxAttempts {
		status = ExportFailed
	}
	_, err := s.db.Exec(ctx, `UPDATE crm_exports SET status=$2,
		privacy_fence_at=now(), error_code='PRIVACY_RECERTIFICATION_REQUIRED',
		failed_at=CASE WHEN $2='FAILED' THEN now() ELSE failed_at END,
		lease_owner=NULL, lease_expires_at=NULL, next_attempt_at=now(),
		updated_at=now(), version=version+1
		WHERE id=$1 AND status='RUNNING'`, export.ID, status)
	return err
}

// publishExport promotes a completed artifact to READY.
//
// The WHERE clause is the whole concurrency story. Promotion requires the job
// to still be RUNNING, so a cancellation that committed while the file was
// being written wins and the artifact is deleted instead of released. The
// database CHECK on READY does the rest: the row cannot reach that state
// without an artifact reference, a checksum, a size, a completion instant and
// an expiry, so there is no ordering of failures that leaves a partial export
// downloadable.
func (s *Service) publishExport(ctx context.Context, export Export, stored StoredArtifact,
	rowCount int64, checksum string) error {
	// The expiry is computed from the database clock, in the same statement
	// that stamps completed_at. Deriving it from the service clock instead
	// would let two slightly different clocks produce a row whose expiry
	// precedes its completion — which the CHECK correctly refuses.
	retention := fmt.Sprintf("%d milliseconds", s.exports.ArtifactTTL.Milliseconds())
	bytesStored := stored.Bytes

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	updated, err := scanExport(tx.QueryRow(ctx, `UPDATE crm_exports SET status='READY',
		row_count=$2, artifact_ref=$3, artifact_bytes=$4, checksum_sha256=$5,
		completed_at=now(), expires_at=now() + $6::interval, error_code=NULL,
		lease_owner=NULL, lease_expires_at=NULL, updated_at=now(), version=version+1
		WHERE id=$1 AND status='RUNNING' RETURNING `+exportColumns,
		export.ID, rowCount, stored.Ref, bytesStored, checksum, retention))
	if errors.Is(err, pgx.ErrNoRows) {
		// Cancelled mid-flight. The bytes must not survive the decision.
		_ = s.artifacts.Delete(ctx, export.BusinessID, stored.Ref, "cancelled_during_generation")
		return nil
	}
	if err != nil {
		return err
	}
	if err = s.auditExport(ctx, Actor{StaffID: "export-generator", VendorID: export.BusinessID},
		updated, "crm.export.completed", nil); err != nil {
		return err
	}
	if err = s.publishExportEvent(ctx, tx, updated, "vendor.crm.export.completed", nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
