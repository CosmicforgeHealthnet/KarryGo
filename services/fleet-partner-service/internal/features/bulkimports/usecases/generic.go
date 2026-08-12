// Section 25C: the generic (engine='GENERIC') import orchestration —
// dry-run/commit-separated validation, adapter-mediated commit, row
// retry/versioning, and the three worker passes.
//
// Nothing here writes a domain table. Every committed row goes through an
// Adapter, whose real implementation lives in internal/app (the composition
// root) and calls the owning domain's own usecase — CreateProfile,
// CreateAsset, Receipt — exactly as if a caller had used that domain's
// ordinary create endpoint. The owning domain's permissions, eligibility and
// safety rules are never bypassed, because they are never re-implemented
// here: they run inside the real call this package makes into them.
package usecases

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// GenericFlags gates the generic-engine surface, independent of the legacy
// Flags on Options.
type GenericFlags struct {
	Enabled          bool
	WritesEnabled    bool
	ValidationWorker bool
	CommitWorker     bool
	RetryWorker      bool
	KillSwitch       bool
}

// RowOutcome is one row's validated, normalized shape — or its rejection.
type RowOutcome struct {
	// ExternalReference is the stable business identity of the row, shared
	// across every corrected version of it.
	ExternalReference string
	// Data is the normalized payload the adapter's Commit method will
	// receive back unchanged.
	Data map[string]any
	// Valid is false when the row failed validation; ErrorCode/ErrorMessage
	// are then required and Data/ExternalReference may be partial.
	Valid        bool
	ErrorCode    string
	ErrorMessage string
}

// Adapter is one import type's contract with the generic engine. An adapter
// never touches a database itself for the domain it commits into — Commit
// calls the real owning-domain usecase, which owns its own persistence,
// audit and outbox behavior.
type Adapter interface {
	// Type is the import_type this adapter serves.
	Type() string
	// SchemaVersion is the row schema this adapter currently parses. A
	// caller-declared schema version that does not match is refused before
	// any row is read, per "schema-version rejection" in the mandated tests.
	SchemaVersion() int
	// RequiredColumns/OptionalColumns document the CSV header this adapter
	// expects; RequiredColumns absent from the file header is a fatal
	// (import-level) validation failure, not a per-row one.
	RequiredColumns() []string
	OptionalColumns() []string
	// ValidateRow parses and validates one raw CSV row (header name -> cell
	// value). The same method is used by both dry-run and commit-mode
	// validation — there is exactly one validation code path.
	ValidateRow(raw map[string]string) RowOutcome
	// Commit performs the real domain write for one validated row and
	// returns a safe subject reference (never a raw error to the caller —
	// the generic engine's commit loop maps any error to a safe code before
	// it ever reaches a stored row).
	Commit(ctx context.Context, t tenant.Context, correlationID string, data map[string]any) (subjectReference string, err error)
}

func (s *Service) genericGate(writes bool) error {
	if s.opts.GenericFlags.KillSwitch {
		return errorcodes.Unavailable("import_disabled", "Imports are temporarily unavailable.")
	}
	if !s.opts.GenericFlags.Enabled {
		return errorcodes.NotFound("import_not_found", "Import not found.")
	}
	if writes && !s.opts.GenericFlags.WritesEnabled {
		return errorcodes.Forbidden("import_writes_disabled", "Import writes are disabled.")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// CreateGenericInput is the validated shape of a generic import request.
type CreateGenericInput struct {
	BranchID      string
	ImportType    string
	MediaFileID   string
	FileFormat    string
	Mode          string
	SchemaVersion int
}

func genericFingerprint(orgID, branchID, importType, mediaFileID, format, mode string, schemaVersion int) string {
	return fingerprint(orgID, branchID, mediaFileID, format+"\x00"+importType+"\x00"+mode+"\x00"+itoa(schemaVersion))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// CreateGeneric registers a new generic import job. It does not read the
// file: validation is a separate, explicit step (POST .../validate).
func (s *Service) CreateGeneric(ctx context.Context, t tenant.Context, in CreateGenericInput) (models.Import, error) {
	if e := s.genericGate(true); e != nil {
		return models.Import{}, e
	}
	importType := strings.ToUpper(strings.TrimSpace(in.ImportType))
	format := strings.ToUpper(strings.TrimSpace(in.FileFormat))
	mode := strings.ToUpper(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = models.ModeCommit
	}
	schemaVersion := in.SchemaVersion
	if schemaVersion <= 0 {
		schemaVersion = 1
	}
	switch {
	case !models.ValidImportType(importType):
		return models.Import{}, errorcodes.BadRequest("import_type_invalid", "importType is not supported.")
	case !models.ValidMode(mode):
		return models.Import{}, errorcodes.BadRequest("import_mode_invalid", "mode must be DRY_RUN or COMMIT.")
	case format != "CSV":
		return models.Import{}, errorcodes.UnprocessableEntity("import_invalid_file", "Only CSV imports are supported.")
	case in.BranchID == "" || !t.BranchPermitted(in.BranchID) || in.MediaFileID == "":
		return models.Import{}, apperrors.BadRequest("branchId and mediaFileId are required.", nil)
	}
	adapter, registered := s.opts.Adapters[importType]
	if !registered {
		return models.Import{}, errorcodes.Unavailable("import_type_unavailable",
			"This import type is recognized but not yet available in this deployment.")
	}
	if adapter.SchemaVersion() != schemaVersion {
		return models.Import{}, errorcodes.UnprocessableEntity("import_schema_version_unsupported",
			"This adapter does not support the requested schema version.")
	}

	job := models.Import{
		OrganizationID: t.OrganizationID, BranchID: in.BranchID, MediaFileID: in.MediaFileID, FileFormat: format,
		Status: models.StatusUploaded, CreatedBy: t.ActorID, ImportType: importType, Mode: mode,
		SchemaVersion: schemaVersion,
		RequestFingerprint: genericFingerprint(t.OrganizationID, in.BranchID, importType, in.MediaFileID, format,
			mode, schemaVersion),
	}
	created, err := s.opts.Repository.CreateGeneric(ctx, nil, job)
	if err != nil {
		if errors.Is(err, repositories.ErrConflict) {
			return models.Import{}, errorcodes.Conflict("import_conflict", "This import already exists.")
		}
		return models.Import{}, err
	}
	return created, nil
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func (s *Service) GetGeneric(ctx context.Context, t tenant.Context, id string) (models.Import, []models.Row, error) {
	if e := s.genericGate(false); e != nil {
		return models.Import{}, nil, e
	}
	job, err := s.opts.Repository.GetGeneric(ctx, nil, t.OrganizationID, id)
	if errors.Is(err, repositories.ErrNotFound) || (err == nil && !t.BranchPermitted(job.BranchID)) {
		return models.Import{}, nil, errorcodes.NotFound("import_not_found", "Import not found.")
	}
	if err != nil {
		return job, nil, err
	}
	rows, err := s.opts.Repository.ListLatestGenericRows(ctx, nil, t.OrganizationID, id, "")
	return job, rows, err
}

func (s *Service) ListGeneric(ctx context.Context, t tenant.Context) ([]models.Import, error) {
	if e := s.genericGate(false); e != nil {
		return nil, e
	}
	return s.opts.Repository.ListGeneric(ctx, t.OrganizationID)
}

// ---------------------------------------------------------------------------
// Validate (dry-run entry point)
// ---------------------------------------------------------------------------

// Validate queues a job for validation. Both DRY_RUN and COMMIT mode jobs go
// through the identical validation path — the mode only governs whether
// Commit is later permitted.
func (s *Service) Validate(ctx context.Context, t tenant.Context, id string) (models.Import, error) {
	if e := s.genericGate(true); e != nil {
		return models.Import{}, e
	}
	job, err := s.opts.Repository.GetGeneric(ctx, nil, t.OrganizationID, id)
	if err != nil || !t.BranchPermitted(job.BranchID) {
		return models.Import{}, errorcodes.NotFound("import_not_found", "Import not found.")
	}
	if job.Status != models.StatusUploaded && job.Status != models.StatusValidationFailed && job.Status != models.StatusValidated {
		return models.Import{}, errorcodes.Conflict("import_conflict", "This import cannot be validated from its current state.")
	}
	if err = s.transitionToValidationPending(ctx, t.OrganizationID, id); err != nil {
		return models.Import{}, err
	}
	return s.opts.Repository.GetGeneric(ctx, nil, t.OrganizationID, id)
}

func (s *Service) transitionToValidationPending(ctx context.Context, organizationID, id string) error {
	_, err := s.opts.Pool.Exec(ctx, `
		UPDATE fleet_partner_bulk_imports SET status='VALIDATION_PENDING', updated_at=now()
		WHERE organization_id=$1 AND id=$2 AND engine='GENERIC'
		  AND status IN ('UPLOADED','VALIDATION_FAILED','VALIDATED')`, organizationID, id)
	return err
}

// ---------------------------------------------------------------------------
// Commit
// ---------------------------------------------------------------------------

// Commit queues a validated job for commit. Refused for a DRY_RUN-mode job —
// dry-run creates no target business records, and this is the one gate that
// guarantees it: there is no code path from DRY_RUN to COMMITTING.
func (s *Service) Commit(ctx context.Context, t tenant.Context, id string) (models.Import, error) {
	if e := s.genericGate(true); e != nil {
		return models.Import{}, e
	}
	job, err := s.opts.Repository.GetGeneric(ctx, nil, t.OrganizationID, id)
	if err != nil || !t.BranchPermitted(job.BranchID) {
		return models.Import{}, errorcodes.NotFound("import_not_found", "Import not found.")
	}
	if job.Mode == models.ModeDryRun {
		return models.Import{}, errorcodes.Forbidden("import_dry_run", "A dry-run import cannot be committed.")
	}
	if job.Status != models.StatusValidated {
		return models.Import{}, errorcodes.Conflict("import_not_validated",
			"This import requires a successful validation before it can be committed.")
	}
	if _, err = s.opts.Pool.Exec(ctx, `
		UPDATE fleet_partner_bulk_imports SET status='COMMIT_PENDING', updated_at=now()
		WHERE organization_id=$1 AND id=$2 AND engine='GENERIC' AND status='VALIDATED'`,
		t.OrganizationID, id); err != nil {
		return models.Import{}, err
	}
	return s.opts.Repository.GetGeneric(ctx, nil, t.OrganizationID, id)
}

// ---------------------------------------------------------------------------
// Retry
// ---------------------------------------------------------------------------

// RetryImport re-arms every FAILED row of a COMMITTED/PARTIALLY_COMMITTED
// job for another commit pass.
func (s *Service) RetryImport(ctx context.Context, t tenant.Context, id string) (int64, error) {
	if e := s.genericGate(true); e != nil {
		return 0, e
	}
	job, err := s.opts.Repository.GetGeneric(ctx, nil, t.OrganizationID, id)
	if err != nil || !t.BranchPermitted(job.BranchID) {
		return 0, errorcodes.NotFound("import_not_found", "Import not found.")
	}
	count, err := s.opts.Repository.RetryImport(ctx, nil, t.OrganizationID, id)
	if err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, errorcodes.Conflict("import_no_failed_rows", "This import has no failed rows to retry.")
	}
	if err = s.opts.Repository.RequeueImportForCommit(ctx, nil, t.OrganizationID, id); err != nil {
		return 0, err
	}
	return count, nil
}

// RetryRow re-arms one failed row for another commit attempt.
func (s *Service) RetryRow(ctx context.Context, t tenant.Context, importID, rowID string) error {
	if e := s.genericGate(true); e != nil {
		return e
	}
	job, err := s.opts.Repository.GetGeneric(ctx, nil, t.OrganizationID, importID)
	if err != nil || !t.BranchPermitted(job.BranchID) {
		return errorcodes.NotFound("import_not_found", "Import not found.")
	}
	if err = s.opts.Repository.RetryRow(ctx, nil, t.OrganizationID, importID, rowID); err != nil {
		if errors.Is(err, repositories.ErrConflict) {
			return errorcodes.Conflict("import_row_not_retryable", "This row is not in a retryable state.")
		}
		return err
	}
	return s.opts.Repository.RequeueImportForCommit(ctx, nil, t.OrganizationID, importID)
}

// ---------------------------------------------------------------------------
// Row correction
// ---------------------------------------------------------------------------

// SubmitCorrectionInput is the validated shape of a corrected-row submission.
type SubmitCorrectionInput struct {
	ImportID        string
	RowID           string
	ExpectedVersion int
	RowData         map[string]string
}

// SubmitCorrection creates a new, immutable row version replacing an INVALID
// or FAILED row's data. The original row is never edited — its history stays
// exactly as it was, and the corrected row is validated through the SAME
// registered adapter every other row in this job used, so a correction can
// never bypass the rules an ordinary row would have been held to.
//
// The corrected row lands VALID (eligible for the next commit pass) or
// INVALID (still not committable) based on that validation — there is no
// third outcome. Validation runs synchronously here rather than through the
// async validation worker: re-reading and re-parsing the whole source file
// to validate one corrected row would be wasteful and would risk the worker
// re-touching rows nothing asked it to touch. Once the row is VALID, the job
// is requeued through the existing commit worker exactly like RetryRow does
// — commit itself is never reimplemented here.
func (s *Service) SubmitCorrection(ctx context.Context, t tenant.Context, in SubmitCorrectionInput) (models.Row, error) {
	if e := s.genericGate(true); e != nil {
		return models.Row{}, e
	}
	job, err := s.opts.Repository.GetGeneric(ctx, nil, t.OrganizationID, in.ImportID)
	if err != nil || !t.BranchPermitted(job.BranchID) {
		return models.Row{}, errorcodes.NotFound("import_not_found", "Import not found.")
	}
	original, err := s.opts.Repository.GetGenericRow(ctx, nil, t.OrganizationID, in.ImportID, in.RowID)
	if err != nil {
		return models.Row{}, errorcodes.NotFound("import_row_not_found", "Row not found.")
	}
	if original.Status == models.RowStatusCommitted {
		return models.Row{}, errorcodes.Conflict("import_row_committed",
			"A committed row cannot be corrected.")
	}

	adapter, registered := s.opts.Adapters[job.ImportType]
	if !registered {
		return models.Row{}, errorcodes.Unavailable("import_type_unavailable",
			"This import type is recognized but not yet available in this deployment.")
	}
	outcome := adapter.ValidateRow(in.RowData)
	status := models.RowStatusValid
	fingerprintSource := outcome.Data
	if !outcome.Valid {
		status = models.RowStatusInvalid
		fingerprintSource = map[string]any{"error": outcome.ErrorCode}
	}

	corrected, err := s.opts.Repository.SubmitCorrectedRow(ctx, nil, original, in.ExpectedVersion,
		status, genericRowFingerprint(fingerprintSource), outcome.Data)
	if err != nil {
		if errors.Is(err, repositories.ErrConflict) {
			return models.Row{}, errorcodes.Conflict("import_row_correction_conflict",
				"The row has changed since you loaded it, or is not in a correctable state.")
		}
		return models.Row{}, err
	}

	if status == models.RowStatusValid {
		// A job that has already committed or partially committed other rows
		// (or whole-job failed) needs to be requeued for another commit pass
		// to pick this corrected row up; a job that never reached commit
		// (still mid-validation) needs nothing further here — the next
		// explicit commit call handles it. Errors from a job not currently
		// requeueable are expected and harmless: the row is still corrected
		// and will be picked up whenever the job does reach COMMIT_PENDING.
		_ = s.opts.Repository.RequeueImportForCommit(ctx, nil, t.OrganizationID, in.ImportID)
	}
	return corrected, nil
}

// ---------------------------------------------------------------------------
// Cancel / Replay
// ---------------------------------------------------------------------------

func (s *Service) CancelGeneric(ctx context.Context, t tenant.Context, id string, expectedVersion int64) (models.Import, error) {
	if e := s.genericGate(true); e != nil {
		return models.Import{}, e
	}
	if expectedVersion <= 0 {
		return models.Import{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	job, err := s.opts.Repository.GetGeneric(ctx, nil, t.OrganizationID, id)
	if err != nil || !t.BranchPermitted(job.BranchID) {
		return models.Import{}, errorcodes.NotFound("import_not_found", "Import not found.")
	}
	out, err := s.opts.Repository.CancelGeneric(ctx, nil, t.OrganizationID, id, expectedVersion)
	if err != nil {
		return out, errorcodes.Conflict("import_conflict", "Import can no longer be cancelled.")
	}
	return out, nil
}

func (s *Service) ReplayGeneric(ctx context.Context, t tenant.Context, id string) error {
	if e := s.genericGate(true); e != nil {
		return e
	}
	job, err := s.opts.Repository.GetGeneric(ctx, nil, t.OrganizationID, id)
	if err != nil || !t.BranchPermitted(job.BranchID) {
		return errorcodes.NotFound("import_not_found", "Import not found.")
	}
	if err = s.opts.Repository.ReplayGeneric(ctx, t.OrganizationID, id); err != nil {
		return errorcodes.Conflict("import_conflict", "Only a dead-lettered import may be replayed.")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Generic CSV parsing — header-driven, adapter-agnostic.
// ---------------------------------------------------------------------------

// parseGenericCSV reads a header row and returns each subsequent row as a
// header-name -> cell-value map, which is what every Adapter.ValidateRow
// receives. Unlike the legacy parse() (typed to OrderRow), this never
// assumes a fixed column set — the adapter alone knows its required and
// optional columns.
func parseGenericCSV(data []byte) ([]map[string]string, error) {
	r := csv.NewReader(bytes.NewReader(data))
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 1 {
		return nil, errors.New("empty file")
	}
	header := records[0]
	rows := make([]map[string]string, 0, len(records)-1)
	for _, record := range records[1:] {
		row := make(map[string]string, len(header))
		for i, name := range header {
			if i < len(record) {
				row[strings.TrimSpace(name)] = strings.TrimSpace(record[i])
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func missingColumns(header []string, required []string) []string {
	present := map[string]bool{}
	for _, h := range header {
		present[strings.TrimSpace(h)] = true
	}
	var missing []string
	for _, col := range required {
		if !present[col] {
			missing = append(missing, col)
		}
	}
	return missing
}

func readHeader(data []byte) ([]string, error) {
	r := csv.NewReader(bytes.NewReader(data))
	record, err := r.Read()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(record))
	for i, h := range record {
		out[i] = strings.TrimSpace(h)
	}
	return out, nil
}

func genericRowFingerprint(data map[string]any) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]any, len(data))
	for _, k := range keys {
		ordered[k] = data[k]
	}
	raw, _ := json.Marshal(ordered)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// tenantFor builds a system-actor tenant context for a worker acting on
// behalf of one organization's job.
func tenantFor(job models.Import) tenant.Context {
	t := tenant.Context{
		OrganizationID: job.OrganizationID, ActorType: tenant.ActorTypeSystem,
		ActorID: "import-worker", CorrelationID: job.ID,
	}
	if job.BranchID != "" {
		t.PermittedBranchIDs = []string{job.BranchID}
	}
	return t
}
