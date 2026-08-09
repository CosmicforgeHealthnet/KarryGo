package events

import (
	"context"
	"errors"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tracing"
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
	// StaleAfter bounds how long a PENDING record may go unpublished before
	// it counts toward the stale gauge. Defaults to 15 minutes.
	StaleAfter time.Duration
	// Environment labels every structured log line (cfg.AppEnv). Empty is
	// valid, matching every other optional worker field in this service.
	Environment string
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
	if w.StaleAfter <= 0 {
		w.StaleAfter = 15 * time.Minute
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
	if stale, err := w.Repository.StaleCount(ctx, w.StaleAfter); err == nil {
		w.Metrics.SetGauge("fleet_partner_outbox_stale_backlog", nil, float64(stale))
	}
	if age, err := w.Repository.OldestPendingAge(ctx); err == nil {
		w.Metrics.SetGauge("fleet_partner_outbox_oldest_pending_age_seconds", nil, age.Seconds())
	}

	records, err := w.Repository.ClaimBatch(ctx, w.Owner, w.Lease, w.BatchSize)
	if err != nil {
		w.Metrics.IncCounter("fleet_partner_outbox_claim_errors_total", nil, 1)
		return
	}

	for _, record := range records {
		w.publishOne(ctx, record)
	}
}

// publishOne publishes exactly one claimed record under its own span, linked
// to the originating request/domain-mutation trace via the record's own
// correlationId (see internal/platform/tracing's doc comment on async
// linking). The event payload is never logged — only identity fields.
func (w *OutboxWorker) publishOne(ctx context.Context, record OutboxRecord) {
	spanCtx, endSpan := tracing.StartSpan(ctx, w.Metrics, config.ServiceName, w.Environment,
		"outbox.publish", map[string]any{
			"eventId": record.ID, "eventType": record.EventType,
			"aggregateType": record.AggregateType, "aggregateId": record.AggregateID,
		})
	logFields := map[string]any{
		"eventId": record.ID, "eventType": record.EventType,
		"aggregateType": record.AggregateType, "aggregateId": record.AggregateID,
		"attempt": record.Attempts + 1,
	}
	if record.OrganizationID != "" {
		logFields["organizationId"] = record.OrganizationID
	}

	if err := w.Publisher.Publish(spanCtx, record); err != nil {
		if errors.Is(err, ErrPublisherNotConfigured) {
			// Transport missing is a configuration state, not an event
			// failure: release the lease without consuming an attempt.
			w.Metrics.IncCounter("fleet_partner_outbox_publisher_unconfigured_total", nil, 1)
			observability.Log(config.ServiceName, w.Environment, "warn", "outbox_publisher_unconfigured", logFields)
			endSpan(nil)
			return
		}
		w.Metrics.IncCounter("fleet_partner_outbox_publish_failures_total", nil, 1)
		logFields["safeErrorClass"] = "publish_failed"
		observability.Log(config.ServiceName, w.Environment, "error", "outbox_publish_failed", logFields)
		_ = w.Repository.MarkFailed(ctx, w.Owner, record.ID, err, w.MaxAttempts)
		endSpan(err)
		return
	}
	w.Metrics.IncCounter("fleet_partner_outbox_published_total", nil, 1)
	observability.Log(config.ServiceName, w.Environment, "info", "outbox_published", logFields)
	_ = w.Repository.MarkPublished(ctx, w.Owner, record.ID)
	endSpan(nil)
}
