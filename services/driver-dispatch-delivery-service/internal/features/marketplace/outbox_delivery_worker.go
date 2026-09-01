package marketplace

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"cosmicforge/logistics/shared/go/logging"
	"github.com/jackc/pgx/v5"
)

// OutboxWorkerConfig tunes durable event delivery.
type OutboxWorkerConfig struct {
	Owner         string
	SourceService string
	Target        string
	Lease         time.Duration
	BatchSize     int
	MaxAttempts   int
	BaseBackoff   time.Duration
	MaxBackoff    time.Duration
}

func (c OutboxWorkerConfig) withDefaults() OutboxWorkerConfig {
	if c.Owner == "" {
		c.Owner = "marketplace-outbox-worker"
	}
	if c.SourceService == "" {
		c.SourceService = "driver-dispatch-delivery-service"
	}
	if c.Target == "" {
		c.Target = "fleet-partner-service"
	}
	if c.Lease <= 0 {
		c.Lease = 2 * time.Minute
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 25
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 10
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 5 * time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 15 * time.Minute
	}
	return c
}

// backoffFor computes bounded exponential backoff.
func (c OutboxWorkerConfig) backoffFor(attempt int) time.Duration {
	delay := c.BaseBackoff
	for i := 0; i < attempt && delay < c.MaxBackoff; i++ {
		delay *= 2
	}
	if delay > c.MaxBackoff {
		delay = c.MaxBackoff
	}
	return delay
}

// OutboxResult summarises one worker pass.
type OutboxResult struct {
	Claimed    int
	Published  int
	Duplicate  int
	Rejected   int
	Unknown    int
	Retried    int
	Recovered  int64
	DeadLetter int
}

// OutboxWorker drains the marketplace outbox to a consumer.
type OutboxWorker struct {
	repo      *Repository
	deliverer EventDeliverer
	config    OutboxWorkerConfig
}

// NewOutboxWorker builds the worker. A nil deliverer fails closed.
func NewOutboxWorker(repo *Repository, deliverer EventDeliverer,
	config OutboxWorkerConfig) *OutboxWorker {
	if deliverer == nil {
		deliverer = UnconfiguredDeliverer{}
	}
	return &OutboxWorker{repo: repo, deliverer: deliverer, config: config.withDefaults()}
}

// RunOnce processes one batch.
//
// Same shape as the relay worker, and for the same reason: the claim commits
// before any HTTP call, so no database transaction is open while waiting on
// another service. A crash mid-delivery releases the lease by timeout rather
// than stranding the event.
func (w *OutboxWorker) RunOnce(ctx context.Context) (returnedResult OutboxResult, returnedErr error) {
	started := time.Now()
	defer func() { observability.Worker("outbox", started, returnedErr) }()
	var result OutboxResult

	recovered, err := w.repo.RecoverExpiredLeases(ctx, w.repo.db)
	if err != nil {
		return result, err
	}
	result.Recovered = recovered

	var claimed []OutboxRow
	if err := w.repo.WithinTx(ctx, func(tx pgx.Tx) error {
		var claimErr error
		claimed, claimErr = w.repo.ClaimOutboxForTarget(ctx, tx, w.config.Owner,
			w.config.Target, w.config.Lease, w.config.BatchSize)
		return claimErr
	}); err != nil {
		return result, err
	}
	result.Claimed = len(claimed)

	for index := range claimed {
		row := claimed[index]
		// Respect cancellation between events so shutdown is graceful: the
		// remaining rows stay leased and are recovered by timeout.
		if ctx.Err() != nil {
			return result, nil
		}
		outcome, err := w.deliverOne(ctx, row)
		if err != nil {
			return result, err
		}
		switch outcome {
		case DeliveryAccepted:
			result.Published++
		case DeliveryDuplicate:
			result.Duplicate++
			result.Published++
		case DeliveryRejected:
			result.Rejected++
			result.DeadLetter++
		case DeliveryUnknown:
			result.Unknown++
		default:
			result.Retried++
		}
	}
	return result, nil
}

// deliverOne sends a single event and records the outcome.
func (w *OutboxWorker) deliverOne(ctx context.Context, row OutboxRow) (result DeliveryOutcome, returnedErr error) {
	defer func() {
		observability.OutboxOutcome(w.config.Target, string(result))
		logging.Structured("info", "dispatch_event_delivery", map[string]any{"event_id": row.ID, "attempt": row.AttemptCount, "outcome": observability.Bounded(string(result), "ACCEPTED", "DUPLICATE", "REJECTED", "UNKNOWN", "RETRYABLE")})
	}()
	if !w.deliverer.Configured() {
		// Not the event's fault — leave it queued rather than burning attempts
		// against a target that does not exist yet.
		return DeliveryRetryable, w.repo.MarkDeliveryFailed(ctx, w.repo.db, row.ID,
			"delivery_target_not_configured", w.config.backoffFor(row.AttemptCount),
			w.config.MaxAttempts, row.LeaseOwner)
	}

	envelope, err := w.envelopeFor(row)
	if err != nil {
		// A row we cannot even decode will never become deliverable. Send it
		// straight to the dead-letter queue instead of retrying ten times.
		return DeliveryRejected, w.repo.MarkDeliveryFailed(ctx, w.repo.db, row.ID,
			"envelope_build_failed", 0, 1, row.LeaseOwner)
	}

	started := time.Now()
	outcome, deliverErr := w.deliverer.Deliver(ctx, envelope)
	elapsed := time.Since(started)
	_ = deliverErr // classification already carries what the caller needs

	if err := w.repo.RecordDeliveryAttempt(ctx, w.repo.db, row.ID, w.config.Target,
		row.AttemptCount+1, 0, outcome.Code, elapsed); err != nil {
		return "", err
	}

	switch outcome.Outcome {
	case DeliveryAccepted, DeliveryDuplicate:
		// DUPLICATE is a success: it is precisely what a redelivery after a
		// lost response returns, and it proves the consumer holds the event.
		return outcome.Outcome, w.repo.MarkPublished(ctx, w.repo.db, row.ID, row.LeaseOwner)

	case DeliveryRejected:
		// Permanent. Dead-letter immediately so an operator sees it rather
		// than letting it churn through the full attempt budget first.
		return DeliveryRejected, w.repo.MarkDeliveryFailed(ctx, w.repo.db, row.ID,
			outcome.Code, 0, 1, row.LeaseOwner)

	case DeliveryUnknown:
		// We never learned whether the consumer committed, so this is queued
		// for retry exactly like a plain failure. That is safe only because
		// MarkDeliveryFailed leaves the event id and payload hash untouched,
		// making the retry a REDELIVERY rather than a new event, and the
		// consumer deduplicates on that pair. Minting a fresh event id here
		// would turn an ambiguous delivery into a duplicate applied change.
		return DeliveryUnknown, w.repo.MarkDeliveryFailed(ctx, w.repo.db, row.ID,
			outcome.Code, w.config.backoffFor(row.AttemptCount), w.config.MaxAttempts, row.LeaseOwner)

	default:
		return DeliveryRetryable, w.repo.MarkDeliveryFailed(ctx, w.repo.db, row.ID,
			outcome.Code, w.config.backoffFor(row.AttemptCount), w.config.MaxAttempts, row.LeaseOwner)
	}
}

// envelopeFor rebuilds the wire envelope from a stored row.
//
// The payload hash is the one stored at publish time, not a fresh hash of the
// decoded map: the consumer verifies against what dispatch committed, so
// recomputing here would hide any corruption in between.
func (w *OutboxWorker) envelopeFor(row OutboxRow) (Envelope, error) {
	var payload map[string]any
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		return Envelope{}, err
	}
	return Envelope{
		EventID:          row.ID,
		EventType:        row.EventType,
		SchemaVersion:    row.SchemaVersion,
		SourceService:    w.config.SourceService,
		AggregateType:    row.AggregateType,
		AggregateID:      row.AggregateID,
		AggregateVersion: row.AggregateVersion,
		OccurredAt:       row.CreatedAt,
		CorrelationID:    row.CorrelationID,
		CausationID:      row.CausationID,
		PayloadHash:      row.PayloadHash,
		Payload:          payload,
	}, nil
}

// Run drains the outbox on an interval until the context ends.
func (w *OutboxWorker) Run(ctx context.Context, interval time.Duration) error {
	defer observability.WorkerRunning("outbox")()
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Durable gauges are collected once by the central periodic collector,
			// not by every target worker on every delivery interval.
			if _, err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
				log.Printf("dispatch outbox maintenance failed target=%s", w.config.Target)
				// One bad pass must not kill the worker; the next tick retries.
				continue
			}
		}
	}
}
