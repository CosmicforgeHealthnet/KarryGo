package events

import (
	"context"
	"errors"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// OutboxWorker polls the outbox and publishes claimed records.
type OutboxWorker struct {
	Repository   *OutboxRepository
	Publisher    Publisher
	Metrics      *observability.Metrics
	Owner        string
	PollInterval time.Duration
	Lease        time.Duration
	BatchSize    int
	MaxAttempts  int
}

// Run polls until ctx is cancelled. A not-configured publisher is observed
// and backed off without consuming attempts, so no event is lost while the
// transport is missing.
func (w *OutboxWorker) Run(ctx context.Context) {
	if w.BatchSize <= 0 {
		w.BatchSize = 20
	}
	if w.MaxAttempts <= 0 {
		w.MaxAttempts = DefaultOutboxMaxAttempts
	}
	ticker := time.NewTicker(w.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *OutboxWorker) tick(ctx context.Context) {
	if backlog, err := w.Repository.PendingCount(ctx); err == nil {
		w.Metrics.SetGauge("fleet_partner_outbox_backlog", nil, float64(backlog))
	}

	records, err := w.Repository.ClaimBatch(ctx, w.Owner, w.Lease, w.BatchSize)
	if err != nil {
		w.Metrics.IncCounter("fleet_partner_outbox_claim_errors_total", nil, 1)
		return
	}

	for _, record := range records {
		if err := w.Publisher.Publish(ctx, record); err != nil {
			if errors.Is(err, ErrPublisherNotConfigured) {
				// Transport missing is a configuration state, not an event
				// failure: release the lease without consuming an attempt.
				w.Metrics.IncCounter("fleet_partner_outbox_publisher_unconfigured_total", nil, 1)
				continue
			}
			w.Metrics.IncCounter("fleet_partner_outbox_publish_failures_total", nil, 1)
			_ = w.Repository.MarkFailed(ctx, w.Owner, record.ID, err, w.MaxAttempts)
			continue
		}
		w.Metrics.IncCounter("fleet_partner_outbox_published_total", nil, 1)
		_ = w.Repository.MarkPublished(ctx, w.Owner, record.ID)
	}
}
