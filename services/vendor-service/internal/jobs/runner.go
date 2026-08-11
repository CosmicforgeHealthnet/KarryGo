// Package jobs is vendor-service's background-worker convention: a plain
// ticker-driven goroutine per recurring job, registered as a
// context-cancellable func(context.Context) error, matching the platform's
// dominant pattern (hauling's match-recovery worker, dispatch's
// broadcast-expiry sweep) rather than introducing a task-queue dependency.
//
// Only driver-dispatch-delivery-service and api-gateway use a real task-queue
// library (hibiken/asynq) today, specifically because they need delayed,
// independently-retried single tasks (e.g. "re-broadcast this booking in 30s"
// per booking). See docs/architecture/10_WORKER_AND_RECOVERY_MODEL.md for
// when a future commerce-fulfilment section might justify the same choice —
// not decided here, and not needed for the architecture foundation.
package jobs

import (
	"context"
	"time"
)

// Job is one recurring background task. Pass is called on every tick; a
// returned error is logged by the caller and does not stop the ticker
// (transient failures should not permanently kill a recovery worker).
type Job struct {
	Name     string
	Interval time.Duration
	Pass     func(context.Context) error
}

// Run ticks Pass until ctx is cancelled. Callers start this as
// `go job.Run(ctx, onError)` before serviceapp.Run, or register it through
// serviceapp.Options.Workers.
func (j Job) Run(ctx context.Context, onError func(job string, err error)) {
	if j.Pass == nil {
		return
	}
	interval := j.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := j.Pass(ctx); err != nil && onError != nil {
			onError(j.Name, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
