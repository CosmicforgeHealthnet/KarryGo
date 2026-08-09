package retention

import (
	"context"
	"log/slog"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// JobScanAndReport is the platform-scoped (not tenant-scoped) Section 27
// data retention job type: it scans Section 27's own operational tables,
// which are not organization-owned data.
const JobScanAndReport = "DATA_RETENTION_SCAN_AND_REPORT"

// Worker drives the retention scan on an interval and via the durable job
// queue. It never deletes or mutates rows -- see the package doc comment
// for the scope decision.
type Worker struct {
	Service    *Service
	Queue      *jobs.Queue
	DB         database.Querier
	Metrics    *observability.Metrics
	Logger     *slog.Logger
	CutoffDays int
	BatchLimit int
}

// RegisterWorkers binds the scan-and-report handler to the shared durable
// queue.
func (w *Worker) RegisterWorkers(jw *jobs.Worker) {
	if jw == nil {
		return
	}
	jw.Register(JobScanAndReport, w.handleScanAndReport)
}

func (w *Worker) handleScanAndReport(ctx context.Context, _ jobs.Job) error {
	report, err := w.Service.ScanAndReport(ctx, w.CutoffDays, w.BatchLimit)
	if err != nil {
		return err
	}
	w.emit(report)
	return nil
}

func (w *Worker) emit(report Report) {
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}
	metrics := w.Metrics
	if metrics == nil {
		metrics = observability.NewMetrics()
	}
	for _, table := range report.Tables {
		metrics.SetGauge("fleet_partner_retention_eligible_rows", map[string]string{"table": table.Table}, float64(table.EligibleCount))
		logger.Info("data retention scan", "table", table.Table, "eligible_count", table.EligibleCount, "cutoff_days", table.CutoffDays)
	}
}

// RunPeriodic enqueues a scan-and-report job on interval until ctx ends.
// The job itself is idempotent (a pure read + metrics emission), so no
// dedupe key beyond the minute bucket is needed to avoid pile-up under a
// slow consumer.
func (w *Worker) RunPeriodic(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	run := func() {
		if _, err := w.Queue.Enqueue(ctx, w.DB, jobs.EnqueueRequest{
			JobType:   JobScanAndReport,
			DedupeKey: "retention-scan:" + time.Now().UTC().Format("2006010215"),
			Payload:   map[string]string{},
		}); err != nil {
			logger := w.Logger
			if logger == nil {
				logger = slog.Default()
			}
			logger.Error("retention: enqueue scan-and-report failed", "error", err)
		}
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
