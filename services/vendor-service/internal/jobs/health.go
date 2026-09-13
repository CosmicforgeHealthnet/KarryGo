package jobs

import (
	"context"
	"errors"
	"time"
)

const (
	WorkerReady      = "READY"
	WorkerDegraded   = "DEGRADED"
	WorkerBlocked    = "BLOCKED"
	WorkerDisabled   = "DISABLED"
	WorkerBacklogged = "BACKLOGGED"
)

// WorkerHealth is a low-cardinality operational snapshot. It deliberately
// contains no job, customer, business, order or event identifiers.
type WorkerHealth struct {
	State                string  `json:"state"`
	PendingCount         int64   `json:"pendingCount"`
	RetryCount           int64   `json:"retryCount"`
	DeadLetterCount      int64   `json:"deadLetterCount"`
	RunningCount         int64   `json:"runningCount"`
	LeaseStarvationCount int64   `json:"leaseStarvationCount"`
	OldestPendingSeconds float64 `json:"oldestPendingSeconds"`
	IntegrationAvailable bool    `json:"integrationAvailable"`
}

// Metrics returns the bounded names used by the existing metrics/logging
// adapters. Values are gauges; callers choose their existing exporter.
func (h WorkerHealth) Metrics() map[string]float64 {
	blocked := float64(0)
	if h.State == WorkerBlocked {
		blocked = 1
	}
	return map[string]float64{
		"vendor_worker_pending_total":          float64(h.PendingCount),
		"vendor_worker_retry_total":            float64(h.RetryCount),
		"vendor_worker_dead_letter_total":      float64(h.DeadLetterCount),
		"vendor_worker_lease_starvation_total": float64(h.LeaseStarvationCount),
		"vendor_worker_blocked_total":          blocked,
		"vendor_worker_oldest_pending_seconds": h.OldestPendingSeconds,
	}
}

// Health reports retained backlog even when a kill switch is disabled. A
// caller must supply dependency availability rather than allowing this package
// to guess the health of a foreign authority.
func (c *DurableCoordinator) Health(ctx context.Context, enabled, integrationAvailable bool, heartbeatStaleAfter time.Duration) (WorkerHealth, error) {
	if c == nil || c.db == nil {
		return WorkerHealth{}, errors.New("jobs: durable coordinator database unavailable")
	}
	if heartbeatStaleAfter <= 0 {
		heartbeatStaleAfter = 2 * time.Minute
	}
	health := WorkerHealth{IntegrationAvailable: integrationAvailable}
	err := c.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status IN ('READY','RETRY')),
			count(*) FILTER (WHERE status='RETRY'),
			count(*) FILTER (WHERE status='DEAD_LETTER'),
			count(*) FILTER (WHERE status='RUNNING'),
			count(*) FILTER (WHERE status='RUNNING' AND
				(lease_expires_at < now() OR heartbeat_at < now() - $1::interval)),
			COALESCE(EXTRACT(EPOCH FROM (now() - min(next_run_at)
				FILTER (WHERE status IN ('READY','RETRY') AND next_run_at < now()))), 0)
		FROM vendor_job_controls`, heartbeatStaleAfter.String()).Scan(
		&health.PendingCount,
		&health.RetryCount,
		&health.DeadLetterCount,
		&health.RunningCount,
		&health.LeaseStarvationCount,
		&health.OldestPendingSeconds,
	)
	if err != nil {
		return WorkerHealth{}, err
	}
	health.State = classifyWorkerHealth(enabled, health)
	return health, nil
}

func classifyWorkerHealth(enabled bool, health WorkerHealth) string {
	if !enabled {
		return WorkerDisabled
	}
	if health.DeadLetterCount > 0 || (!health.IntegrationAvailable && health.PendingCount > 0) {
		return WorkerBlocked
	}
	if !health.IntegrationAvailable || health.LeaseStarvationCount > 0 || health.RetryCount > 0 {
		return WorkerDegraded
	}
	if health.PendingCount > 0 {
		return WorkerBacklogged
	}
	return WorkerReady
}
