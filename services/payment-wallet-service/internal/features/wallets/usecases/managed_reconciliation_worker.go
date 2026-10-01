package walletusecases

import (
	"context"
	"log"
	"time"

	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
)

// Section 12 WAL-017: the Wallet-side reconciliation worker.
//
// What it is looking for is the failure nobody notices. A job marked
// ORGANIZATION_MANAGED is closed to the legacy direct-payout path, so if Fleet
// never instructs a settlement the customer's money simply stays in escrow.
// Nothing is corrupt, no error is raised and no row is marked failed -- which
// is precisely why it can sit there indefinitely. The provider finds out by
// asking where their money is.
//
// The worker does not move money and has no authority to. Entitlement is
// Fleet's and posting is the settlement path's; all this does is make the
// state visible and durable so someone can act on it.

// ManagedReconciliationStore is the narrow slice of the repository this worker
// needs. Narrow on purpose: a worker that could post would eventually be asked
// to.
type ManagedReconciliationStore interface {
	FindStuckManagedJobs(ctx context.Context, heldBefore time.Time, limit int) ([]walletrepositories.StuckManagedJob, error)
	RecordManagedReconciliation(ctx context.Context, job walletrepositories.StuckManagedJob) (walletrepositories.ManagedReconciliation, error)
	ResolveManagedReconciliations(ctx context.Context) (int64, error)
}

// ManagedReconciliationConfig tunes the sweep.
type ManagedReconciliationConfig struct {
	// Grace is how long a managed job may stay held before it counts as stuck.
	// A job completing right now is held and unsettled for entirely normal
	// reasons; reporting those would bury the real ones.
	Grace time.Duration

	// Interval is how often the sweep runs.
	Interval time.Duration

	// BatchSize bounds one sweep, so a large backlog is worked through over
	// several ticks rather than in one unbounded query.
	BatchSize int
}

// Defaults chosen to surface a stuck job within the hour without reporting
// every in-flight completion.
const (
	DefaultManagedReconciliationGrace    = 30 * time.Minute
	DefaultManagedReconciliationInterval = 10 * time.Minute
	DefaultManagedReconciliationBatch    = 200
)

func (c ManagedReconciliationConfig) normalized() ManagedReconciliationConfig {
	if c.Grace <= 0 {
		c.Grace = DefaultManagedReconciliationGrace
	}
	if c.Interval <= 0 {
		c.Interval = DefaultManagedReconciliationInterval
	}
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultManagedReconciliationBatch
	}
	return c
}

// ManagedReconciliationWorker sweeps for managed jobs whose money never moved.
type ManagedReconciliationWorker struct {
	Store  ManagedReconciliationStore
	Config ManagedReconciliationConfig
	Now    func() time.Time
}

// ReconciliationSweep reports what one sweep found.
type ReconciliationSweep struct {
	// Resolved is how many previously-stuck jobs have since settled.
	Resolved int64

	// Opened is how many jobs are recorded as still stuck. A job already
	// recorded counts here too: it is still stuck, and the record's
	// observation count is what distinguishes "noticed once" from "noticed
	// two hundred times".
	Opened int
}

// RunOnce performs a single sweep.
//
// Resolution runs first, deliberately: a job that settled between two sweeps
// must be closed out before the scan looks for stuck jobs, or it would be
// reported as stuck one last time on its way out.
func (w *ManagedReconciliationWorker) RunOnce(ctx context.Context) (ReconciliationSweep, error) {
	if w == nil || w.Store == nil {
		return ReconciliationSweep{}, nil
	}
	config := w.Config.normalized()
	now := time.Now
	if w.Now != nil {
		now = w.Now
	}

	var sweep ReconciliationSweep
	resolved, err := w.Store.ResolveManagedReconciliations(ctx)
	if err != nil {
		return sweep, err
	}
	sweep.Resolved = resolved

	stuck, err := w.Store.FindStuckManagedJobs(ctx, now().UTC().Add(-config.Grace), config.BatchSize)
	if err != nil {
		return sweep, err
	}
	for _, job := range stuck {
		if ctx.Err() != nil {
			return sweep, ctx.Err()
		}
		record, err := w.Store.RecordManagedReconciliation(ctx, job)
		if err != nil {
			// One unrecordable job must not stop the sweep: the rest are
			// still stuck and still need surfacing.
			log.Printf("managed reconciliation not recorded source=%s job=%s: %v",
				job.SourceService, job.SourceJobID, err)
			continue
		}
		sweep.Opened++
		if record.Observations == 1 {
			log.Printf("managed job held with no settlement source=%s job=%s amount=%d %s held_since=%s",
				job.SourceService, job.SourceJobID, job.AmountMinor, job.Currency,
				job.HeldSince.UTC().Format(time.RFC3339))
		}
	}
	return sweep, nil
}

// Run sweeps on a ticker until the context ends.
//
// It sweeps once immediately: a restart is one of the times a job is most
// likely to have been left behind, so waiting a full interval to look is
// exactly the wrong behaviour.
func (w *ManagedReconciliationWorker) Run(ctx context.Context) {
	if w == nil || w.Store == nil {
		return
	}
	config := w.Config.normalized()
	if _, err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
		log.Printf("managed reconciliation sweep failed: %v", err)
	}
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
				log.Printf("managed reconciliation sweep failed: %v", err)
			}
		}
	}
}
