package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// GenericWorkerConfig tunes one worker pass, following the same shape as the
// legacy WorkerConfig (owner identity, lease, bounded batch, retry ceiling).
type GenericWorkerConfig struct {
	Owner                  string
	Lease                  time.Duration
	Backoff                time.Duration
	BatchSize, MaxAttempts int
}

// GenericWorkerResult reports one pass's outcome.
type GenericWorkerResult struct {
	Claimed, Validated, Committed, Failed, DeadLettered int
}

func withGenericDefaults(cfg GenericWorkerConfig) GenericWorkerConfig {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 8
	}
	if cfg.Lease <= 0 {
		cfg.Lease = time.Minute
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = 30 * time.Second
	}
	if strings.TrimSpace(cfg.Owner) == "" {
		cfg.Owner = "import-worker"
	}
	return cfg
}

// RunValidationPass claims jobs queued for validation (or whose validation
// lease expired) and validates each one.
func (s *Service) RunValidationPass(ctx context.Context, cfg GenericWorkerConfig) (GenericWorkerResult, error) {
	var result GenericWorkerResult
	if !s.opts.GenericFlags.ValidationWorker || s.opts.GenericFlags.KillSwitch {
		return result, nil
	}
	cfg = withGenericDefaults(cfg)
	jobs, err := s.opts.Repository.ClaimForValidation(ctx, cfg.Owner, cfg.Lease, cfg.BatchSize)
	if err != nil {
		return result, err
	}
	result.Claimed = len(jobs)
	for _, job := range jobs {
		s.processClaimed(ctx, cfg, job, &result)
	}
	return result, nil
}

// RunCommitPass claims jobs queued for commit (or whose commit lease
// expired) and commits each one's valid rows.
func (s *Service) RunCommitPass(ctx context.Context, cfg GenericWorkerConfig) (GenericWorkerResult, error) {
	var result GenericWorkerResult
	if !s.opts.GenericFlags.CommitWorker || s.opts.GenericFlags.KillSwitch {
		return result, nil
	}
	cfg = withGenericDefaults(cfg)
	jobs, err := s.opts.Repository.ClaimForCommit(ctx, cfg.Owner, cfg.Lease, cfg.BatchSize)
	if err != nil {
		return result, err
	}
	result.Claimed = len(jobs)
	for _, job := range jobs {
		s.processClaimed(ctx, cfg, job, &result)
	}
	return result, nil
}

// RunRetryPass claims whole-job FAILED imports whose backoff window has
// elapsed and re-attempts whichever phase they were last running —
// distinct from RetryImport/RetryRow, which re-arm individual committed-row
// failures for the next ordinary commit pass.
func (s *Service) RunRetryPass(ctx context.Context, cfg GenericWorkerConfig) (GenericWorkerResult, error) {
	var result GenericWorkerResult
	if !s.opts.GenericFlags.RetryWorker || s.opts.GenericFlags.KillSwitch {
		return result, nil
	}
	cfg = withGenericDefaults(cfg)
	jobs, err := s.opts.Repository.ClaimForRetry(ctx, cfg.Owner, cfg.Lease, cfg.Backoff, cfg.BatchSize)
	if err != nil {
		return result, err
	}
	result.Claimed = len(jobs)
	for _, job := range jobs {
		s.processClaimed(ctx, cfg, job, &result)
	}
	return result, nil
}

// processClaimed dispatches a freshly-claimed job by the status the claim
// query left it in: VALIDATING jobs (from ClaimForValidation, or from
// ClaimForRetry re-entering an unfinished validation) run the validation
// path; COMMITTING jobs run the commit path. One shared switch means all
// three worker passes above are thin wrappers over the same processing
// logic, differing only in which query claims the batch.
func (s *Service) processClaimed(ctx context.Context, cfg GenericWorkerConfig, job models.Import, result *GenericWorkerResult) {
	switch job.Status {
	case models.StatusValidating:
		s.runValidation(ctx, cfg, job, result)
	case models.StatusCommitting:
		s.runCommit(ctx, cfg, job, result)
	}
}

func (s *Service) startGenericHeartbeat(ctx context.Context, owner, id string, lease time.Duration) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	interval := lease / 3
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.opts.Repository.Heartbeat(ctx, owner, id, lease)
			}
		}
	}()
	return func() { close(stop); <-done }
}

func (s *Service) failGenericAttempt(ctx context.Context, cfg GenericWorkerConfig, job models.Import, code string,
	result *GenericWorkerResult) {
	_ = s.opts.Repository.Attempt(ctx, job.OrganizationID, job.ID, "FAILED", code, job.Attempts)
	_ = s.opts.Repository.FailGeneric(ctx, cfg.Owner, job.ID, code, cfg.MaxAttempts)
	if job.Attempts >= cfg.MaxAttempts {
		result.DeadLettered++
	} else {
		result.Failed++
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func (s *Service) runValidation(ctx context.Context, cfg GenericWorkerConfig, job models.Import, result *GenericWorkerResult) {
	adapter, registered := s.opts.Adapters[job.ImportType]
	if !registered {
		s.failGenericAttempt(ctx, cfg, job, "adapter_not_registered", result)
		return
	}
	if s.opts.Media == nil {
		s.failGenericAttempt(ctx, cfg, job, "media_unavailable", result)
		return
	}

	t := tenantFor(job)
	stop := s.startGenericHeartbeat(ctx, cfg.Owner, job.ID, cfg.Lease)
	file, err := s.opts.Media.LoadImport(ctx, t, job.MediaFileID, s.opts.MaxBytes)
	stop()
	if err != nil {
		s.failGenericAttempt(ctx, cfg, job, "media_unavailable", result)
		return
	}
	if file.SizeBytes > s.opts.MaxBytes || strings.ToUpper(file.Format) != "CSV" {
		_ = s.opts.Repository.SetValidationOutcome(ctx, nil, job.ID, 0, 0, 0, models.StatusValidationFailed)
		_ = s.opts.Repository.Attempt(ctx, job.OrganizationID, job.ID, "FAILED", "import_invalid_file", job.Attempts)
		result.Failed++
		return
	}

	sum := sha256.Sum256(file.Content)
	contentFingerprint := hex.EncodeToString(sum[:])
	if job.FileFingerprint != "" && job.FileFingerprint != contentFingerprint {
		s.failGenericAttempt(ctx, cfg, job, "file_fingerprint_changed", result)
		return
	}
	if job.FileFingerprint == "" {
		_, _ = s.opts.Pool.Exec(ctx, `UPDATE fleet_partner_bulk_imports SET file_fingerprint=$1 WHERE id=$2`,
			contentFingerprint, job.ID)
	}

	header, err := readHeader(file.Content)
	if err != nil {
		_ = s.opts.Repository.SetValidationOutcome(ctx, nil, job.ID, 0, 0, 0, models.StatusValidationFailed)
		_ = s.opts.Repository.Attempt(ctx, job.OrganizationID, job.ID, "FAILED", "import_unreadable", job.Attempts)
		result.Failed++
		return
	}
	if missing := missingColumns(header, adapter.RequiredColumns()); len(missing) > 0 {
		_ = s.opts.Repository.SetValidationOutcome(ctx, nil, job.ID, 0, 0, 0, models.StatusValidationFailed)
		_ = s.opts.Repository.Attempt(ctx, job.OrganizationID, job.ID, "FAILED", "import_missing_columns", job.Attempts)
		result.Failed++
		return
	}

	rawRows, err := parseGenericCSV(file.Content)
	if err != nil || len(rawRows) > s.opts.MaxRows {
		_ = s.opts.Repository.SetValidationOutcome(ctx, nil, job.ID, len(rawRows), 0, len(rawRows), models.StatusValidationFailed)
		_ = s.opts.Repository.Attempt(ctx, job.OrganizationID, job.ID, "FAILED", "import_row_limit_exceeded", job.Attempts)
		result.Failed++
		return
	}

	valid, invalid := 0, 0
	for number, raw := range rawRows {
		outcome := adapter.ValidateRow(raw)
		status := models.RowStatusValid
		fingerprintSource := outcome.Data
		if !outcome.Valid {
			status = models.RowStatusInvalid
			invalid++
			fingerprintSource = map[string]any{"error": outcome.ErrorCode, "row": number}
		} else {
			valid++
		}
		externalRef := outcome.ExternalReference
		if externalRef == "" {
			externalRef = job.ID + "-row-" + itoa(number+1)
		}
		row := models.Row{
			OrganizationID: job.OrganizationID, ImportID: job.ID, RowNumber: number + 1,
			RowFingerprint: genericRowFingerprint(fingerprintSource), Status: status,
			SafeErrorCode: outcome.ErrorCode, SafeErrorMessage: outcome.ErrorMessage,
			ExternalReference: externalRef, SchemaVersion: job.SchemaVersion, RowVersion: 1,
			GenericData: outcome.Data,
		}
		if err = s.opts.Repository.InsertGenericRow(ctx, nil, row); err != nil {
			s.failGenericAttempt(ctx, cfg, job, "row_persist_failed", result)
			return
		}
	}

	status := models.StatusValidated
	if valid == 0 {
		status = models.StatusValidationFailed
	}
	if err = s.opts.Repository.SetValidationOutcome(ctx, nil, job.ID, len(rawRows), valid, invalid, status); err != nil {
		s.failGenericAttempt(ctx, cfg, job, "validation_persist_failed", result)
		return
	}
	_ = s.opts.Repository.Attempt(ctx, job.OrganizationID, job.ID, "VALIDATED", "", job.Attempts)
	result.Validated++
}

// ---------------------------------------------------------------------------
// Commit
// ---------------------------------------------------------------------------

func (s *Service) runCommit(ctx context.Context, cfg GenericWorkerConfig, job models.Import, result *GenericWorkerResult) {
	adapter, registered := s.opts.Adapters[job.ImportType]
	if !registered {
		s.failGenericAttempt(ctx, cfg, job, "adapter_not_registered", result)
		return
	}

	rows, err := s.opts.Repository.ListLatestGenericRows(ctx, nil, job.OrganizationID, job.ID, models.RowStatusValid)
	if err != nil {
		s.failGenericAttempt(ctx, cfg, job, "rows_unavailable", result)
		return
	}

	t := tenantFor(job)
	committed, failed := 0, 0
	var errorRows []map[string]any
	cancelled := false
	for _, row := range rows {
		if cancelled, err = s.opts.Repository.IsCancelled(ctx, job.OrganizationID, job.ID); err != nil {
			s.failGenericAttempt(ctx, cfg, job, "cancellation_check_failed", result)
			return
		}
		if cancelled {
			break
		}

		stop := s.startGenericHeartbeat(ctx, cfg.Owner, job.ID, cfg.Lease)
		subjectReference, commitErr := adapter.Commit(ctx, t, row.RowReference, row.GenericData)
		stop()

		if commitErr != nil {
			safe := apperrors.From(commitErr)
			_ = s.opts.Repository.MarkRowFailed(ctx, nil, row.ID, string(safe.Code), safe.Message)
			errorRows = append(errorRows, map[string]any{"externalReference": row.ExternalReference, "code": safe.Code})
			failed++
			continue
		}
		_ = s.opts.Repository.MarkRowCommitted(ctx, nil, row.ID, subjectReference)
		committed++
	}
	if cancelled {
		return
	}

	status := models.StatusCommitted
	switch {
	case committed == 0 && failed > 0:
		status = models.StatusFailed
	case failed > 0:
		status = models.StatusPartiallyCommitted
	}

	reportID := ""
	if len(errorRows) > 0 && s.opts.Media != nil {
		raw, _ := json.Marshal(map[string]any{"importId": job.ID, "errors": errorRows})
		reportID, _ = s.opts.Media.WriteErrorReport(ctx, t, job.ID, raw)
	}
	if err = s.opts.Repository.FinalizeCommit(ctx, nil, job.ID, status, reportID, committed, failed); err != nil {
		s.failGenericAttempt(ctx, cfg, job, "commit_finalize_failed", result)
		return
	}
	outcome := "COMMITTED"
	if status == models.StatusFailed {
		outcome = "FAILED"
	}
	_ = s.opts.Repository.Attempt(ctx, job.OrganizationID, job.ID, outcome, "", job.Attempts)
	if status == models.StatusFailed {
		result.Failed++
		return
	}
	result.Committed++
}
