package observability

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ObservePoolStats periodically records pgxpool's own connection-pool
// counters as bounded gauges (no labels beyond the metric name itself — a
// single dedicated pool per process, so there is nothing to key by). Runs
// until ctx is cancelled; safe to start unconditionally (a nil pool or
// metrics registry is never passed by either binary's composition).
func ObservePoolStats(ctx context.Context, metrics *Metrics, pool *pgxpool.Pool, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	record := func() {
		stat := pool.Stat()
		metrics.SetGauge("fleet_partner_db_pool_total_conns", nil, float64(stat.TotalConns()))
		metrics.SetGauge("fleet_partner_db_pool_acquired_conns", nil, float64(stat.AcquiredConns()))
		metrics.SetGauge("fleet_partner_db_pool_idle_conns", nil, float64(stat.IdleConns()))
		metrics.SetGauge("fleet_partner_db_pool_max_conns", nil, float64(stat.MaxConns()))
		metrics.SetGauge("fleet_partner_db_pool_new_conns_total", nil, float64(stat.NewConnsCount()))
		metrics.SetGauge("fleet_partner_db_pool_acquire_count_total", nil, float64(stat.AcquireCount()))
		metrics.SetGauge("fleet_partner_db_pool_acquire_duration_seconds", nil, stat.AcquireDuration().Seconds())
		metrics.SetGauge("fleet_partner_db_pool_canceled_acquire_count_total", nil, float64(stat.CanceledAcquireCount()))
		metrics.SetGauge("fleet_partner_db_pool_empty_acquire_count_total", nil, float64(stat.EmptyAcquireCount()))
	}
	record()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			record()
		}
	}
}
