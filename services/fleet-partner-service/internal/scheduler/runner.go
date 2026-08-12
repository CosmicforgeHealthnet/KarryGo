package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// Runner is the scheduler's own claim-and-decide loop: it never executes
// domain work itself. For every due scheduled task it decides -- based on
// concurrency policy and misfire policy -- whether to enqueue a durable job
// occurrence (via jobs.Queue, so the existing lease/heartbeat/retry/
// dead-letter machinery owns everything from there on), skip it, or replace
// a not-yet-started prior occurrence, then advances the task to its next
// due time. Multiple Runner instances (multiple scheduler processes) are
// safe to run concurrently: ClaimDue uses FOR UPDATE SKIP LOCKED, and
// Advance is a compare-and-set gated on still holding the lease.
type Runner struct {
	Repo    *Repository
	Jobs    *jobs.Queue
	DB      database.Querier
	Owner   string
	Logger  *slog.Logger
	Metrics *observability.Metrics

	// LeaseFor bounds how long a claimed-but-unprocessed task holds its
	// scheduler lease before another instance may reclaim it (crash
	// recovery). Defaults to 60s.
	LeaseFor time.Duration
	// BatchLimit bounds how many due tasks one Tick claims. Defaults to 50.
	BatchLimit int
	// MisfireGrace is how far behind next_run_at "now" must be before an
	// occurrence counts as missed. Defaults to misfireGrace (30s).
	MisfireGrace time.Duration
	// PollInterval is how often Run ticks. Defaults to 5s.
	PollInterval time.Duration
	// Now is the clock, overridable for tests.
	Now func() time.Time

	mu    sync.RWMutex
	ready bool
}

func (r *Runner) leaseFor() time.Duration {
	if r.LeaseFor > 0 {
		return r.LeaseFor
	}
	return 60 * time.Second
}

func (r *Runner) batchLimit() int {
	if r.BatchLimit > 0 {
		return r.BatchLimit
	}
	return 50
}

func (r *Runner) misfireGrace() time.Duration {
	if r.MisfireGrace > 0 {
		return r.MisfireGrace
	}
	return misfireGrace
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) logger() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}

func (r *Runner) metrics() *observability.Metrics {
	if r.Metrics != nil {
		return r.Metrics
	}
	return observability.NewMetrics()
}

func (r *Runner) pollInterval() time.Duration {
	if r.PollInterval > 0 {
		return r.PollInterval
	}
	return 5 * time.Second
}

// Ready reports whether the scheduler loop is polling (for readiness checks).
func (r *Runner) Ready() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready
}

// Run polls until ctx is cancelled. Shutdown is simple compared to the job
// worker's: a Tick only claims and immediately decides/enqueues/advances --
// it never holds a task lease across a long-running external effect -- so
// stopping the poll loop is sufficient. A task claimed mid-Tick when ctx is
// cancelled is recovered by the next scheduler instance once its lease
// expires, exactly like a crashed instance (proven by
// TestScheduler_LeaseRecoveryAfterCrash), so no separate release-on-shutdown
// path is needed here.
func (r *Runner) Run(ctx context.Context) {
	r.mu.Lock()
	r.ready = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.ready = false
		r.mu.Unlock()
	}()

	ticker := time.NewTicker(r.pollInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := r.Tick(ctx)
			if err != nil {
				r.metrics().IncCounter("fleet_partner_scheduler_tick_errors_total", nil, 1)
				continue
			}
			r.metrics().IncCounter("scheduled_tasks_due_total", nil, float64(result.Claimed))
			r.metrics().IncCounter("scheduled_task_misfires_total", nil, float64(result.SkippedMisfire))
			r.metrics().IncCounter("scheduler_enqueue_failures_total", nil, float64(result.Failed))
		}
	}
}

// TickResult summarizes one claim-and-process pass, for metrics/tests.
type TickResult struct {
	Claimed        int
	Enqueued       int
	SkippedForbid  int
	SkippedMisfire int
	Failed         int
}

// Tick claims due tasks and processes each to completion. It never blocks
// on domain work -- enqueueing a job is a single durable insert, not
// executing the job's handler.
func (r *Runner) Tick(ctx context.Context) (TickResult, error) {
	var result TickResult
	due, err := r.Repo.ClaimDue(ctx, r.Owner, r.leaseFor(), r.batchLimit())
	if err != nil {
		return result, fmt.Errorf("scheduler: tick claim: %w", err)
	}
	result.Claimed = len(due)
	for _, task := range due {
		outcome, err := r.processOne(ctx, task)
		if err != nil {
			result.Failed++
			r.logger().Error("scheduler: process task failed", "scheduled_task_id", task.ID, "error", err)
			continue
		}
		switch outcome {
		case OutcomeEnqueued, OutcomeReplacedPrior:
			result.Enqueued++
		case OutcomeSkippedForbid:
			result.SkippedForbid++
		case OutcomeSkippedMisfire:
			result.SkippedMisfire++
		case OutcomeEnqueueFailed:
			result.Failed++
		}
	}
	return result, nil
}

// processOne decides and applies the occurrence outcome for a single
// claimed task, then advances it to its next due time.
func (r *Runner) processOne(ctx context.Context, task Task) (string, error) {
	now := r.now().UTC()
	if task.NextRunAt == nil {
		// A ONCE task with no further occurrence should never have been
		// claimed (next_run_at would be NULL and therefore not due), but
		// guard defensively: disable it and release the lease.
		return "", r.Repo.Advance(ctx, r.Owner, task.ID, nil, "no further occurrences", false)
	}
	scheduledFor := *task.NextRunAt

	// Misfire handling: how far behind is this occurrence?
	behind := now.Sub(scheduledFor)
	occurrencesToRun := []time.Time{scheduledFor}
	if behind > r.misfireGrace() {
		switch task.MisfirePolicy {
		case MisfireRunOnce:
			occurrencesToRun = []time.Time{scheduledFor}
		case MisfireCatchUpLimited:
			occurrencesToRun, err := r.collectCatchUp(task, scheduledFor, now)
			if err != nil {
				return "", err
			}
			return r.runOccurrences(ctx, task, occurrencesToRun, now, true)
		case MisfireSkip:
			fallthrough
		default:
			if err := r.Repo.RecordRun(ctx, task.ID, task.OrganizationID, scheduledFor, OutcomeSkippedMisfire, "", ""); err != nil {
				return "", err
			}
			next, err := ComputeNextRun(task, now)
			if err != nil {
				return "", err
			}
			if err := r.Repo.Advance(ctx, r.Owner, task.ID, next, "skipped misfire", false); err != nil {
				return "", err
			}
			return OutcomeSkippedMisfire, nil
		}
	}
	return r.runOccurrences(ctx, task, occurrencesToRun, now, false)
}

// collectCatchUp computes the bounded set of missed occurrences to run
// under CATCH_UP_LIMITED, walking forward from scheduledFor up to
// task.MaxCatchUp occurrences that are <= now.
func (r *Runner) collectCatchUp(task Task, scheduledFor time.Time, now time.Time) ([]time.Time, error) {
	occurrences := []time.Time{scheduledFor}
	cursor := scheduledFor
	for len(occurrences) < task.MaxCatchUp {
		next, err := ComputeNextRun(task, cursor)
		if err != nil {
			return nil, err
		}
		if next == nil || next.After(now) {
			break
		}
		occurrences = append(occurrences, *next)
		cursor = *next
	}
	return occurrences, nil
}

// runOccurrences applies concurrency policy and enqueues (or skips) each
// occurrence in order, then advances the task past the last one processed.
func (r *Runner) runOccurrences(ctx context.Context, task Task, occurrences []time.Time, now time.Time, misfireCatchUp bool) (string, error) {
	lastOutcome := OutcomeSkippedForbid
	for _, occurrence := range occurrences {
		outcome, err := r.applyConcurrencyAndEnqueue(ctx, task, occurrence)
		if err != nil {
			return "", err
		}
		lastOutcome = outcome
	}
	last := occurrences[len(occurrences)-1]
	next, err := ComputeNextRun(task, last)
	if err != nil {
		return "", err
	}
	// A misfire catch-up run must not leave next_run_at behind "now" again
	// immediately -- if the freshly computed next occurrence is itself
	// already due, that is legitimate (a very frequent schedule) and the
	// following Tick will pick it up; no special-casing needed since
	// ClaimDue only re-selects once the lease we're about to clear expires
	// or is released by Advance.
	_ = misfireCatchUp
	// An enqueue failure still advances the schedule (recorded, with
	// failure_count incremented for operator visibility) rather than
	// leaving the task's scheduler lease held until it times out --
	// "record failure without losing schedule."
	failed := lastOutcome == OutcomeEnqueueFailed
	if err := r.Repo.Advance(ctx, r.Owner, task.ID, next, lastOutcome, failed); err != nil {
		return "", err
	}
	return lastOutcome, nil
}

// applyConcurrencyAndEnqueue enforces ALLOW/FORBID/REPLACE against any
// still-in-flight job from a prior occurrence, then enqueues (idempotently,
// via scheduled_task_id + scheduled_for) the durable job for this
// occurrence.
func (r *Runner) applyConcurrencyAndEnqueue(ctx context.Context, task Task, occurrence time.Time) (string, error) {
	if task.ConcurrencyPolicy == ConcurrencyForbid || task.ConcurrencyPolicy == ConcurrencyReplace {
		active, found, err := r.Jobs.ActiveJobForScheduledTask(ctx, task.ID)
		if err != nil {
			return "", err
		}
		if found {
			if task.ConcurrencyPolicy == ConcurrencyForbid {
				if err := r.Repo.RecordRun(ctx, task.ID, task.OrganizationID, occurrence, OutcomeSkippedForbid, active.ID, ""); err != nil {
					return "", err
				}
				return OutcomeSkippedForbid, nil
			}
			// REPLACE: only a not-yet-started occurrence (PENDING or
			// RETRY_SCHEDULED) can be safely superseded -- a CLAIMED/RUNNING
			// job may already have caused an external effect and is never
			// force-cancelled here.
			if active.Status == jobs.StatusPending || active.Status == jobs.StatusRetryScheduled {
				if err := r.Jobs.Cancel(ctx, task.OrganizationID, active.ID); err != nil {
					return "", fmt.Errorf("scheduler: replace prior occurrence: %w", err)
				}
			} else {
				if err := r.Repo.RecordRun(ctx, task.ID, task.OrganizationID, occurrence, OutcomeSkippedForbid, active.ID, ""); err != nil {
					return "", err
				}
				return OutcomeSkippedForbid, nil
			}
		}
	}

	scheduledFor := occurrence
	jobID, err := r.Jobs.Enqueue(ctx, r.DB, jobs.EnqueueRequest{
		OrganizationID:  task.OrganizationID,
		BranchID:        task.BranchID,
		JobType:         task.JobType,
		Payload:         task.Payload,
		PayloadVersion:  task.PayloadVersion,
		Priority:        task.Priority,
		ScheduledAt:     occurrence,
		CorrelationID:   task.ID,
		ScheduledTaskID: task.ID,
		ScheduledFor:    &scheduledFor,
		MaxAttempts:     task.MaxAttempts,
	})
	if err != nil {
		r.logger().Error("scheduler: enqueue occurrence failed", "scheduled_task_id", task.ID, "error", err)
		if recordErr := r.Repo.RecordRun(ctx, task.ID, task.OrganizationID, occurrence, OutcomeEnqueueFailed, "", "enqueue_failed"); recordErr != nil {
			return "", fmt.Errorf("scheduler: record enqueue failure: %w", recordErr)
		}
		return OutcomeEnqueueFailed, nil
	}
	outcome := OutcomeEnqueued
	if task.ConcurrencyPolicy == ConcurrencyReplace {
		outcome = OutcomeReplacedPrior
	}
	if err := r.Repo.RecordRun(ctx, task.ID, task.OrganizationID, occurrence, outcome, jobID, ""); err != nil {
		return "", err
	}
	return outcome, nil
}
