package retentionexec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ErrNotFound means the run does not exist within the caller's organization
// scope.
var ErrNotFound = errors.New("retentionexec: run not found")

// Store persists fleet_partner_retention_runs and
// fleet_partner_retention_run_records.
type Store struct {
	db database.Querier
}

// NewStore wires the store over a pool or transaction.
func NewStore(db database.Querier) *Store { return &Store{db: db} }

const runColumns = `id, organization_id, retention_class, mode, status, COALESCE(job_id, ''),
	initiated_by, idempotency_key, records_considered, records_archived, records_anonymized,
	records_skipped, records_failed, COALESCE(error_message, ''), started_at, completed_at,
	created_at, updated_at`

// CreateRun inserts a new run row, PENDING, all counters zero.
func (s *Store) CreateRun(ctx context.Context, q database.Querier, run Run) (Run, error) {
	if run.ID == "" {
		run.ID = ids.New("rret")
	}
	if run.Status == "" {
		run.Status = RunStatusPending
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_retention_runs
			(id, organization_id, retention_class, mode, status, initiated_by, idempotency_key,
			 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())`,
		run.ID, run.OrganizationID, run.RetentionClass, run.Mode, run.Status, run.InitiatedBy, run.IdempotencyKey,
	)
	if err != nil {
		return Run{}, fmt.Errorf("retentionexec: create run: %w", err)
	}
	return s.GetRun(ctx, run.OrganizationID, run.ID)
}

// GetByIdempotencyKey returns the existing run for (organizationID, key), or
// ErrNotFound if none -- Administrator.Initiate uses this to make
// initiation idempotent (a retried initiate request with the same key
// returns the same run rather than starting a second one).
func (s *Store) GetByIdempotencyKey(ctx context.Context, organizationID, key string) (Run, error) {
	row := s.db.QueryRow(ctx, `SELECT `+runColumns+`
		FROM fleet_partner_retention_runs WHERE organization_id = $1 AND idempotency_key = $2`,
		organizationID, key)
	return scanRunRow(row)
}

// GetRun loads one run scoped to organizationID.
func (s *Store) GetRun(ctx context.Context, organizationID, id string) (Run, error) {
	row := s.db.QueryRow(ctx, `SELECT `+runColumns+`
		FROM fleet_partner_retention_runs WHERE organization_id = $1 AND id = $2`, organizationID, id)
	return scanRunRow(row)
}

// GetRunByID loads one run by ID alone -- used by the job handler, which
// only has the run ID from the job payload (organization scoping was
// already enforced at Initiate time; the job itself runs with platform
// authority, not a caller-supplied organization).
func (s *Store) GetRunByID(ctx context.Context, id string) (Run, error) {
	row := s.db.QueryRow(ctx, `SELECT `+runColumns+`
		FROM fleet_partner_retention_runs WHERE id = $1`, id)
	return scanRunRow(row)
}

// SetJobID records which durable job is executing this run.
func (s *Store) SetJobID(ctx context.Context, q database.Querier, runID, jobID string) error {
	_, err := q.Exec(ctx, `UPDATE fleet_partner_retention_runs SET job_id = $2, updated_at = now() WHERE id = $1`,
		runID, jobID)
	if err != nil {
		return fmt.Errorf("retentionexec: set job id: %w", err)
	}
	return nil
}

// MarkRunning transitions PENDING -> RUNNING and stamps started_at (only on
// the first transition -- a retried job resuming an already-RUNNING run
// must not reset its start time).
func (s *Store) MarkRunning(ctx context.Context, q database.Querier, runID string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_retention_runs
		SET status = $2, started_at = COALESCE(started_at, now()), updated_at = now()
		WHERE id = $1 AND status IN ('PENDING', 'RUNNING')`, runID, RunStatusRunning)
	if err != nil {
		return fmt.Errorf("retentionexec: mark running: %w", err)
	}
	return nil
}

// CompleteRun finalizes a run's terminal status, counters and (for
// failures) error message.
// AddCounters ADDS one pass's tally to the run's running totals, without
// touching status -- called after EVERY RunOnce pass, exhausted or not.
// A run that takes more than one pass to exhaust (batchSize smaller than
// the true candidate count) previously had no mechanism at all to persist
// an intermediate, non-exhausted pass's counters anywhere: only the
// single, final CompleteRun call ever wrote to these columns, so a
// multi-pass run's stored totals reflected only its LAST pass, silently
// discarding every earlier one. Every RunCounters value passed in is
// already scoped to genuinely NEW work for its own pass (Runner.decide's
// HasOutcome check excludes anything already decided earlier in the same
// run), so addition here is exactly correct and never double-counts.
// Caught by TestSupportMetadataExecutor_BatchLimitEnforced: a batchSize=1
// run over 2 candidates completed with RecordsConsidered=0.
func (s *Store) AddCounters(ctx context.Context, q database.Querier, runID string, counters RunCounters) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_retention_runs SET
			records_considered = records_considered + $2, records_archived = records_archived + $3,
			records_anonymized = records_anonymized + $4, records_skipped = records_skipped + $5,
			records_failed = records_failed + $6, updated_at = now()
		WHERE id = $1`,
		runID, counters.Considered, counters.Archived, counters.Anonymized,
		counters.Skipped, counters.Failed,
	)
	if err != nil {
		return fmt.Errorf("retentionexec: add counters: %w", err)
	}
	return nil
}

// CompleteRun finalizes a run's status/error/completed_at. It does NOT
// touch the counter columns -- every pass's contribution is already
// persisted via AddCounters as it happens, so re-adding the final pass's
// counters here would double-count them.
func (s *Store) CompleteRun(ctx context.Context, q database.Querier, runID, status string, errMessage string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_retention_runs SET
			status = $2, error_message = NULLIF($3, ''), completed_at = now(), updated_at = now()
		WHERE id = $1`,
		runID, status, errMessage,
	)
	if err != nil {
		return fmt.Errorf("retentionexec: complete run: %w", err)
	}
	return nil
}

// RunCounters summarizes one run's outcome tally.
type RunCounters struct {
	Considered, Archived, Anonymized, Skipped, Failed int
}

// RecordOutcome inserts one immutable per-candidate result. Idempotent by
// construction: (run_id, record_type, record_id) is unique, so replaying
// the same run (a durable job resumed after lease recovery) safely no-ops
// on a candidate this run already decided, via ON CONFLICT DO NOTHING --
// the caller distinguishes "already decided" from "genuinely just
// recorded" via HasOutcome, called first.
func (s *Store) RecordOutcome(ctx context.Context, q database.Querier, record RunRecord) error {
	if record.ID == "" {
		record.ID = ids.New("rrec")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_retention_run_records
			(id, run_id, organization_id, record_type, record_id, outcome, detail, processed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (run_id, record_type, record_id) DO NOTHING`,
		record.ID, record.RunID, record.OrganizationID, record.RecordType, record.RecordID,
		record.Outcome, record.Detail,
	)
	if err != nil {
		return fmt.Errorf("retentionexec: record outcome: %w", err)
	}
	return nil
}

// HasOutcome reports whether (runID, recordType, recordID) already has a
// recorded outcome -- the idempotent-replay guard RunOnce checks before
// re-deciding a candidate.
func (s *Store) HasOutcome(ctx context.Context, q database.Querier, runID, recordType, recordID string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM fleet_partner_retention_run_records
			WHERE run_id = $1 AND record_type = $2 AND record_id = $3)`,
		runID, recordType, recordID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("retentionexec: check outcome: %w", err)
	}
	return exists, nil
}

// ListRunRecords returns every immutable outcome for a run -- backs a
// detail view; callers that must expose nothing sensitive should use
// counts from the Run itself (records_archived etc.), not this, in a
// public-facing preview response. Detail strings are operator-facing
// (record type/id and a short human reason), never raw column values.
func (s *Store) ListRunRecords(ctx context.Context, runID string) ([]RunRecord, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, run_id, organization_id, record_type, record_id, outcome, detail, processed_at
		FROM fleet_partner_retention_run_records WHERE run_id = $1 ORDER BY processed_at ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("retentionexec: list run records: %w", err)
	}
	defer rows.Close()
	var records []RunRecord
	for rows.Next() {
		var r RunRecord
		if err := rows.Scan(&r.ID, &r.RunID, &r.OrganizationID, &r.RecordType, &r.RecordID,
			&r.Outcome, &r.Detail, &r.ProcessedAt); err != nil {
			return nil, fmt.Errorf("retentionexec: scan run record: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRunRow(row pgx.Row) (Run, error) {
	r, err := scanRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return r, err
}

func scanRun(row scannable) (Run, error) {
	var r Run
	var jobID, errMessage string
	var startedAt, completedAt *time.Time
	err := row.Scan(
		&r.ID, &r.OrganizationID, &r.RetentionClass, &r.Mode, &r.Status, &jobID,
		&r.InitiatedBy, &r.IdempotencyKey, &r.RecordsConsidered, &r.RecordsArchived, &r.RecordsAnonymized,
		&r.RecordsSkipped, &r.RecordsFailed, &errMessage, &startedAt, &completedAt,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return Run{}, fmt.Errorf("retentionexec: scan run: %w", err)
	}
	r.JobID, r.ErrorMessage, r.StartedAt, r.CompletedAt = jobID, errMessage, startedAt, completedAt
	return r, nil
}
