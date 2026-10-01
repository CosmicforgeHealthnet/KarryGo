// Package jobs is pricing-service's background-worker convention: a plain
// ticker-driven goroutine per recurring job, registered through
// serviceapp.Options.Workers.
//
// Every worker's state lives in Postgres (due-at columns claimed with FOR UPDATE
// SKIP LOCKED), so a restart loses nothing and several replicas can run the same
// job safely. That matters more here than in most services: a reconciliation
// that is owed to a completed trip must survive a deployment, because settlement
// is held until it lands.
package jobs

import (
	"context"
	"time"
)

// Job is one recurring background task. Pass is called on every tick; a returned
// error is logged by the caller and does not stop the ticker, because a
// transient failure must not permanently kill a recovery worker.
type Job struct {
	Name     string
	Interval time.Duration
	Pass     func(context.Context) error
}

// Run ticks Pass until ctx is cancelled. Pass runs immediately on start, so a
// restart catches up on overdue work without waiting a full interval.
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
