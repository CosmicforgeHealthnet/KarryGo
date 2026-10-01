package integration_test

// Section 12 WAL-017 / WAL-017A: the sweep that finds managed jobs whose money
// never moved.
//
// Database-backed because the thing being certified is a query against real
// rows: a job is stuck precisely when an intent is held, marked managed, and
// has no POSTED settlement. Every one of those is a database fact.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
)

// markManaged closes a job to the legacy direct-payout path, which is what an
// execution service does the moment it assigns a managed executor.
func markManaged(t *testing.T, pool *pgxpool.Pool, sourceService, jobID string, heldFor time.Duration) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		UPDATE payment_intents
		SET settlement_mode = 'ORGANIZATION_MANAGED', updated_at = now() - $3::interval
		WHERE source_service = $1 AND source_reference = $2`,
		sourceService, jobID, heldFor.String(),
	); err != nil {
		t.Fatalf("mark job managed: %v", err)
	}
}

func reconciliationPool(t *testing.T) (*pgxpool.Pool, *walletrepositories.PostgresWalletRepository) {
	t.Helper()
	pool := walletLedgerPool(t)
	if _, err := pool.Exec(context.Background(),
		`TRUNCATE managed_settlement_reconciliations CASCADE`); err != nil {
		t.Fatalf("clean reconciliations: %v", err)
	}
	return pool, walletrepositories.NewPostgresWalletRepository(pool)
}

func sweepFor(repo *walletrepositories.PostgresWalletRepository, grace time.Duration) *walletusecases.ManagedReconciliationWorker {
	return &walletusecases.ManagedReconciliationWorker{
		Store:  repo,
		Config: walletusecases.ManagedReconciliationConfig{Grace: grace, BatchSize: 50},
	}
}

// The failure this worker exists for: money held for a corporate job that
// Fleet never instructed. Nothing is corrupt and no error was raised, which is
// exactly why nobody would otherwise notice.
func TestAManagedJobHeldWithNoSettlementIsSurfaced(t *testing.T) {
	pool, repo := reconciliationPool(t)
	jobID := "job_stuck"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	markManaged(t, pool, "taxi", jobID, 2*time.Hour)

	sweep, err := sweepFor(repo, 30*time.Minute).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sweep.Opened != 1 {
		t.Fatalf("sweep opened %d reconciliations, want 1", sweep.Opened)
	}

	open, err := repo.ListOpenManagedReconciliations(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 {
		t.Fatalf("%d open reconciliations, want 1", len(open))
	}
	if open[0].SourceJobID != jobID || open[0].HeldAmountMinor != 10000 {
		t.Fatalf("reconciliation = %+v", open[0])
	}
}

// A job completing right now is held and unsettled for entirely normal
// reasons. Reporting those would bury the real ones.
func TestARecentlyCompletedJobIsNotYetStuck(t *testing.T) {
	pool, repo := reconciliationPool(t)
	jobID := "job_recent"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	markManaged(t, pool, "taxi", jobID, 2*time.Minute)

	sweep, err := sweepFor(repo, 30*time.Minute).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sweep.Opened != 0 {
		t.Fatalf("a job held for two minutes was reported as stuck")
	}
}

// An independent job holds money the same way and is settled by an entirely
// different path. Reporting it here would be a false alarm on every ride.
func TestAnIndependentJobIsNotAManagedReconciliation(t *testing.T) {
	pool, repo := reconciliationPool(t)
	heldJob(t, pool, "taxi", "job_independent", 10000, 1000)

	sweep, err := sweepFor(repo, time.Minute).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sweep.Opened != 0 {
		t.Fatal("an independent job was reported as a stuck managed settlement")
	}
}

func TestASettledManagedJobIsNotStuck(t *testing.T) {
	pool, repo := reconciliationPool(t)
	jobID := "job_settled"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	markManaged(t, pool, "taxi", jobID, 2*time.Hour)
	if _, err := repo.PostManagedSettlement(context.Background(), commissionInstruction(jobID)); err != nil {
		t.Fatal(err)
	}

	sweep, err := sweepFor(repo, 30*time.Minute).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sweep.Opened != 0 {
		t.Fatal("a settled job was reported as stuck")
	}
}

// WAL-017A. The worker runs every few minutes forever. If each run opened a
// new reconciliation, a job stuck for a week would be two thousand items to
// act on instead of one.
func TestRepeatedSweepsKeepOneReconciliationPerJob(t *testing.T) {
	pool, repo := reconciliationPool(t)
	jobID := "job_repeat"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	markManaged(t, pool, "taxi", jobID, 2*time.Hour)
	worker := sweepFor(repo, 30*time.Minute)
	ctx := context.Background()

	for run := 0; run < 5; run++ {
		if _, err := worker.RunOnce(ctx); err != nil {
			t.Fatalf("sweep %d: %v", run, err)
		}
	}

	open, err := repo.ListOpenManagedReconciliations(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 {
		t.Fatalf("five sweeps produced %d reconciliations, want 1", len(open))
	}
	// How many times it has been seen is the thing that distinguishes a job
	// noticed once from one nobody has acted on for a week.
	if open[0].Observations != 5 {
		t.Fatalf("observations = %d, want 5", open[0].Observations)
	}
	// And how long it has been true must not be reset by looking again.
	if !open[0].FirstObservedAt.Before(open[0].LastObservedAt) &&
		!open[0].FirstObservedAt.Equal(open[0].LastObservedAt) {
		t.Fatalf("first observation moved forward: %+v", open[0])
	}
}

// A job that settles after being reported must be closed out, or the queue of
// things to act on only ever grows.
func TestAStuckJobThatLaterSettlesIsResolved(t *testing.T) {
	pool, repo := reconciliationPool(t)
	jobID := "job_late_settle"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	markManaged(t, pool, "taxi", jobID, 2*time.Hour)
	worker := sweepFor(repo, 30*time.Minute)
	ctx := context.Background()

	if _, err := worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PostManagedSettlement(ctx, commissionInstruction(jobID)); err != nil {
		t.Fatal(err)
	}

	sweep, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sweep.Resolved != 1 {
		t.Fatalf("resolved %d reconciliations, want 1", sweep.Resolved)
	}
	// Resolution runs before the scan, so the job is not reported as stuck one
	// last time on its way out.
	if sweep.Opened != 0 {
		t.Fatalf("a job that settled was still reported as stuck")
	}

	open, err := repo.ListOpenManagedReconciliations(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Fatalf("%d reconciliations still open after settlement", len(open))
	}

	// The history is kept: "stuck for two hours, then settled" is exactly what
	// is worth being able to look up later.
	var status, resolution string
	if err := pool.QueryRow(ctx,
		`SELECT status, resolution FROM managed_settlement_reconciliations WHERE source_job_id = $1`, jobID,
	).Scan(&status, &resolution); err != nil {
		t.Fatal(err)
	}
	if status != walletrepositories.ReconciliationResolved ||
		resolution != walletrepositories.ReconciliationResolutionSettled {
		t.Fatalf("resolved row = %s/%s", status, resolution)
	}
}

// The worker must never move money. It has no authority to decide an
// entitlement and no way to post one.
func TestTheSweepMovesNoMoney(t *testing.T) {
	pool, repo := reconciliationPool(t)
	jobID := "job_no_money_moved"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	markManaged(t, pool, "taxi", jobID, 2*time.Hour)
	ctx := context.Background()

	if _, err := sweepFor(repo, 30*time.Minute).RunOnce(ctx); err != nil {
		t.Fatal(err)
	}

	var journals int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ledger_transactions WHERE source_reference = $1`, jobID).Scan(&journals); err != nil {
		t.Fatal(err)
	}
	if journals != 0 {
		t.Fatalf("the reconciliation sweep posted %d journals", journals)
	}
	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM payment_intents WHERE source_reference = $1`, jobID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "held" {
		t.Fatalf("the sweep changed the payment status to %q", status)
	}
}

// A worker with nothing wired must do nothing rather than panic: a
// misconfigured deployment should degrade to no sweep, not to a crash loop.
func TestASweepWithNoStoreIsANoOp(t *testing.T) {
	worker := &walletusecases.ManagedReconciliationWorker{}
	sweep, err := worker.RunOnce(context.Background())
	if err != nil || sweep.Opened != 0 || sweep.Resolved != 0 {
		t.Fatalf("sweep=%+v err=%v", sweep, err)
	}
}
