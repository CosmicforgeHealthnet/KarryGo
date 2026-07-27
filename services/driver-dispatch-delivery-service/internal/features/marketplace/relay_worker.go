package marketplace

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// FleetResponder delivers a provider decision to fleet-partner-service.
//
// The relay worker owns durability and retry policy; this port owns the wire
// call. Keeping them apart is what lets the worker be tested against real
// PostgreSQL without a live fleet, and lets the transport be swapped without
// touching the retry semantics that protect a provider's decision.
type FleetResponder interface {
	// Deliver sends one provider decision. It must reuse the operation's
	// original idempotency key so a redelivery is recognised by fleet as the
	// same decision rather than a new one.
	Deliver(ctx context.Context, meta CallMeta, op RelayOperation) (RelayDeliveryResult, error)
	Configured() bool
}

// RelayDeliveryResult is fleet's answer to a relayed decision.
type RelayDeliveryResult struct {
	// Accepted means fleet durably recorded the decision.
	Accepted bool
	// Permanent means fleet refused in a way that will never succeed on retry
	// (for example the assignment was already resolved by someone else).
	Permanent bool
	// ErrorCode is fleet's stable code, used for operator diagnosis.
	ErrorCode string
	Snapshot  any
}

// UnconfiguredResponder is the fail-closed default.
//
// It never reports success. An unconfigured deployment therefore leaves
// provider decisions durably queued rather than silently discarding them.
type UnconfiguredResponder struct{}

// Configured reports false.
func (UnconfiguredResponder) Configured() bool { return false }

// Deliver always reports the fleet service as unavailable.
func (UnconfiguredResponder) Deliver(context.Context, CallMeta, RelayOperation) (RelayDeliveryResult, error) {
	return RelayDeliveryResult{}, ErrFleetUnavailable
}

// RelayWorkerConfig tunes the relay worker.
type RelayWorkerConfig struct {
	Owner       string
	Lease       time.Duration
	BatchSize   int
	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

func (c RelayWorkerConfig) withDefaults() RelayWorkerConfig {
	if c.Owner == "" {
		c.Owner = "fleet-relay-worker"
	}
	if c.Lease <= 0 {
		c.Lease = 2 * time.Minute
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 20
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

func (c RelayWorkerConfig) backoffFor(attempt int) time.Duration {
	delay := c.BaseBackoff
	for i := 0; i < attempt && delay < c.MaxBackoff; i++ {
		delay *= 2
	}
	if delay > c.MaxBackoff {
		delay = c.MaxBackoff
	}
	return delay
}

// RelayResult summarises one worker pass.
type RelayResult struct {
	Claimed   int
	Confirmed int
	Rejected  int
	Unknown   int
	Retried   int
	Recovered int64
}

// RelayWorker delivers durable provider decisions to fleet.
type RelayWorker struct {
	repo      *Repository
	responder FleetResponder
	config    RelayWorkerConfig
}

// NewRelayWorker builds the worker. A nil responder fails closed.
func NewRelayWorker(repo *Repository, responder FleetResponder, config RelayWorkerConfig) *RelayWorker {
	if responder == nil {
		responder = UnconfiguredResponder{}
	}
	return &RelayWorker{repo: repo, responder: responder, config: config.withDefaults()}
}

// RunOnce processes one batch.
//
// Ordering matters: the claim is committed before any HTTP call, and no
// database transaction is open while talking to fleet. Holding a row lock
// across a network call would tie a dispatch transaction to another service's
// availability.
func (w *RelayWorker) RunOnce(ctx context.Context) (RelayResult, error) {
	var result RelayResult

	recovered, err := w.repo.RecoverExpiredRelayLeases(ctx, nil)
	if err != nil {
		return result, err
	}
	result.Recovered = recovered

	var claimed []RelayOperation
	if err := w.repo.WithinTx(ctx, func(tx pgx.Tx) error {
		var claimErr error
		claimed, claimErr = w.repo.ClaimRelays(ctx, tx, w.config.Owner, w.config.Lease, w.config.BatchSize)
		return claimErr
	}); err != nil {
		return result, err
	}
	result.Claimed = len(claimed)

	for index := range claimed {
		operation := claimed[index]
		// Respect cancellation between items so shutdown is graceful: the
		// remaining rows stay leased and are recovered by timeout.
		if ctx.Err() != nil {
			return result, nil
		}
		outcome, err := w.deliver(ctx, operation)
		if err != nil {
			return result, err
		}
		switch outcome {
		case RelayConfirmed:
			result.Confirmed++
		case RelayRejected:
			result.Rejected++
		case RelayUnknownOutcome:
			result.Unknown++
		default:
			result.Retried++
		}
	}
	return result, nil
}

// deliver sends one decision and records the outcome.
func (w *RelayWorker) deliver(ctx context.Context, operation RelayOperation) (string, error) {
	if !w.responder.Configured() {
		// Not a failure of the provider's decision — leave it queued.
		return RelayRetryScheduled, w.repo.FailRelay(ctx, nil, operation.ID,
			"fleet_service_unavailable", w.config.backoffFor(operation.AttemptCount),
			w.config.MaxAttempts, false)
	}

	meta := CallMeta{RequestID: operation.ID, CorrelationID: operation.IdempotencyKey}
	result, err := w.responder.Deliver(ctx, meta, operation)
	if err != nil {
		if errors.Is(err, ErrFleetUnavailable) {
			// Ambiguous: fleet may or may not have recorded it. The next pass
			// reuses the same idempotency key, so redelivery is recognised as
			// the same decision instead of creating a second one.
			return RelayUnknownOutcome, w.repo.MarkRelayUnknown(ctx, nil, operation.ID,
				"fleet_service_unavailable", w.config.backoffFor(operation.AttemptCount),
				w.config.MaxAttempts)
		}
		return RelayRetryScheduled, w.repo.FailRelay(ctx, nil, operation.ID, "relay_failed",
			w.config.backoffFor(operation.AttemptCount), w.config.MaxAttempts, false)
	}

	switch {
	case result.Accepted:
		return RelayConfirmed, w.repo.ConfirmRelay(ctx, nil, operation.ID, result.Snapshot)
	case result.Permanent:
		// Fleet gave a definite refusal. Retrying would burn the budget on an
		// answer that will never change.
		return RelayRejected, w.repo.FailRelay(ctx, nil, operation.ID, result.ErrorCode,
			0, w.config.MaxAttempts, true)
	default:
		return RelayRetryScheduled, w.repo.FailRelay(ctx, nil, operation.ID, result.ErrorCode,
			w.config.backoffFor(operation.AttemptCount), w.config.MaxAttempts, false)
	}
}

// Run loops until the context is cancelled, then returns for graceful shutdown.
func (w *RelayWorker) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := w.RunOnce(ctx); err != nil && ctx.Err() == nil {
				// A pass failure must not kill the worker: the rows are
				// durable and the next tick retries them.
				continue
			}
		}
	}
}
