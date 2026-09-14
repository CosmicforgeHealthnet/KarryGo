package walletrepositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Section 12 WAL-017: finding managed jobs whose money never moved.
//
// A job marked ORGANIZATION_MANAGED is closed to the legacy direct-payout
// path. That is correct -- it stops the provider being paid directly for work
// the organization owes -- but it means that if Fleet never instructs a
// settlement, the money stays in escrow with nothing wrong anywhere: no error,
// no failed row, no alert. The first person to notice is the provider, asking
// where their money is.
//
// This is the sweep that notices first.

// StuckManagedJob is one managed job still holding a customer's money.
type StuckManagedJob struct {
	PaymentIntentID string
	SourceService   string
	SourceJobID     string
	Currency        string
	AmountMinor     int64
	HeldSince       time.Time
}

// ManagedReconciliation is the durable record of one such job.
type ManagedReconciliation struct {
	ID                 string
	SourceService      string
	SourceJobID        string
	PaymentIntentID    string
	Currency           string
	HeldAmountMinor    int64
	Status             string
	Resolution         string
	InstructionID      string
	InstructionVersion int64
	HeldSince          time.Time
	FirstObservedAt    time.Time
	LastObservedAt     time.Time
	Observations       int
}

// Reconciliation statuses and resolutions.
const (
	ReconciliationOpen     = "OPEN"
	ReconciliationResolved = "RESOLVED"

	// ReconciliationResolutionSettled means a settlement posted after all.
	ReconciliationResolutionSettled = "SETTLED"
)

// FindStuckManagedJobs lists managed jobs still held past a grace period with
// no posted settlement.
//
// The grace period matters: a job completing right now is held and unsettled
// for entirely normal reasons, and reporting it would make the signal useless.
// What is being looked for is a job that stayed that way.
func (r *PostgresWalletRepository) FindStuckManagedJobs(
	ctx context.Context, heldBefore time.Time, limit int,
) ([]StuckManagedJob, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT i.id, i.source_service, i.source_reference, i.currency, i.amount_kobo, i.updated_at
		FROM payment_intents i
		WHERE i.settlement_mode = 'ORGANIZATION_MANAGED'
		  AND i.status = 'held'
		  AND i.updated_at < $1
		  AND NOT EXISTS (
		      SELECT 1 FROM managed_settlements s
		      WHERE s.source_service = i.source_service
		        AND s.source_job_id = i.source_reference
		        AND s.status = 'POSTED')
		ORDER BY i.updated_at
		LIMIT $2`, heldBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("wallet: find stuck managed jobs: %w", err)
	}
	defer rows.Close()

	var stuck []StuckManagedJob
	for rows.Next() {
		var job StuckManagedJob
		if err := rows.Scan(&job.PaymentIntentID, &job.SourceService, &job.SourceJobID,
			&job.Currency, &job.AmountMinor, &job.HeldSince); err != nil {
			return nil, err
		}
		stuck = append(stuck, job)
	}
	return stuck, rows.Err()
}

// RecordManagedReconciliation opens or refreshes the reconciliation for one
// stuck job.
//
// WAL-017A. Idempotent by the job's own identity: running the worker again
// updates the existing row's observation count rather than opening a second
// reconciliation, so a job stuck for a week is one item to act on and not two
// thousand. The first_observed_at is never moved, because how long this has
// been true is the whole point of the record.
func (r *PostgresWalletRepository) RecordManagedReconciliation(
	ctx context.Context, job StuckManagedJob,
) (ManagedReconciliation, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO managed_settlement_reconciliations (
			id, source_service, source_job_id, payment_intent_id,
			currency, held_amount_minor, status, held_since
		) VALUES ($1,$2,$3,$4,$5,$6,'OPEN',$7)
		ON CONFLICT (source_service, source_job_id) DO UPDATE
		SET last_observed_at = now(),
		    observations = managed_settlement_reconciliations.observations + 1,
		    held_amount_minor = EXCLUDED.held_amount_minor,
		    updated_at = now()
		RETURNING id, source_service, source_job_id, payment_intent_id, currency,
		          held_amount_minor, status, resolution, instruction_id, instruction_version,
		          held_since, first_observed_at, last_observed_at, observations`,
		uuid.NewString(), job.SourceService, job.SourceJobID, job.PaymentIntentID,
		job.Currency, job.AmountMinor, job.HeldSince)
	return scanManagedReconciliation(row)
}

// ResolveManagedReconciliations closes every open reconciliation whose job has
// since settled.
//
// Resolved rows are kept rather than deleted: "this job was stuck for two days
// and then settled" is exactly the history worth having, and a table that only
// ever holds current problems cannot answer whether a fix worked.
func (r *PostgresWalletRepository) ResolveManagedReconciliations(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE managed_settlement_reconciliations rec
		SET status = 'RESOLVED', resolution = $1,
		    instruction_id = s.instruction_id, instruction_version = s.instruction_version,
		    resolved_at = now(), updated_at = now()
		FROM managed_settlements s
		WHERE rec.status = 'OPEN'
		  AND s.source_service = rec.source_service
		  AND s.source_job_id = rec.source_job_id
		  AND s.status = 'POSTED'`, ReconciliationResolutionSettled)
	if err != nil {
		return 0, fmt.Errorf("wallet: resolve managed reconciliations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListOpenManagedReconciliations returns the jobs still waiting on Fleet.
func (r *PostgresWalletRepository) ListOpenManagedReconciliations(
	ctx context.Context, limit int,
) ([]ManagedReconciliation, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, source_service, source_job_id, payment_intent_id, currency,
		       held_amount_minor, status, resolution, instruction_id, instruction_version,
		       held_since, first_observed_at, last_observed_at, observations
		FROM managed_settlement_reconciliations
		WHERE status = 'OPEN'
		ORDER BY held_since
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("wallet: list open managed reconciliations: %w", err)
	}
	defer rows.Close()

	var open []ManagedReconciliation
	for rows.Next() {
		reconciliation, err := scanManagedReconciliation(rows)
		if err != nil {
			return nil, err
		}
		open = append(open, reconciliation)
	}
	return open, rows.Err()
}

func scanManagedReconciliation(row rowScanner) (ManagedReconciliation, error) {
	var rec ManagedReconciliation
	err := row.Scan(&rec.ID, &rec.SourceService, &rec.SourceJobID, &rec.PaymentIntentID,
		&rec.Currency, &rec.HeldAmountMinor, &rec.Status, &rec.Resolution,
		&rec.InstructionID, &rec.InstructionVersion,
		&rec.HeldSince, &rec.FirstObservedAt, &rec.LastObservedAt, &rec.Observations)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ManagedReconciliation{}, err
		}
		return ManagedReconciliation{}, fmt.Errorf("wallet: scan managed reconciliation: %w", err)
	}
	return rec, nil
}
