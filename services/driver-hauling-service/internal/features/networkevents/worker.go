// Package networkevents delivers Haulage's network facts to analytics
// (Section 13).
//
// Recovery, guarantee and capability-mismatch facts have been written to
// hauling_domain_outbox addressed to analytics-service since Section 9, but no
// worker ever claimed that target, so every one sat undelivered. This worker
// claims exactly that target and nothing else.
package networkevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	sharedevents "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/logging"
)

// Target is the outbox target this worker owns.
const Target = "analytics-service"

// Deliverer posts one envelope.
type Deliverer interface {
	Deliver(ctx context.Context, envelope sharedevents.Envelope) error
}

// Worker drains analytics-addressed rows.
type Worker struct {
	DB          *pgxpool.Pool
	Deliverer   Deliverer
	BatchSize   int
	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

func (w *Worker) defaults() {
	if w.BatchSize <= 0 {
		w.BatchSize = 25
	}
	if w.MaxAttempts <= 0 {
		w.MaxAttempts = 12
	}
	if w.BaseBackoff <= 0 {
		w.BaseBackoff = 10 * time.Second
	}
	if w.MaxBackoff <= 0 {
		w.MaxBackoff = 15 * time.Minute
	}
}

// Run drains on an interval until the context ends.
func (w *Worker) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := w.RunOnce(ctx); err != nil {
			logging.Error("hauling-analytics", "analytics delivery: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type row struct {
	ID, AggregateType, AggregateID, EventType, CorrelationID string
	Version                                                  int64
	Payload                                                  []byte
	OccurredAt                                               time.Time
	Attempts                                                 int
}

// RunOnce claims and delivers one batch.
func (w *Worker) RunOnce(ctx context.Context) error {
	if w == nil || w.DB == nil || w.Deliverer == nil {
		return nil
	}
	w.defaults()
	rows, err := w.DB.Query(ctx, `
		UPDATE hauling_domain_outbox
		   SET lease_owner='hauling-analytics', lease_until=now()+interval '1 minute', attempts=attempts+1
		 WHERE id IN (
			SELECT id FROM hauling_domain_outbox
			 WHERE target=$1 AND delivered_at IS NULL AND dead_lettered_at IS NULL
			   AND available_at<=now() AND (lease_until IS NULL OR lease_until<now())
			 ORDER BY occurred_at
			 FOR UPDATE SKIP LOCKED
			 LIMIT $2)
		RETURNING id::text, aggregate_type, aggregate_id::text, event_type, event_version,
		          payload, correlation_id, occurred_at, attempts`, Target, w.BatchSize)
	if err != nil {
		return err
	}
	var batch []row
	for rows.Next() {
		var r row
		if err = rows.Scan(&r.ID, &r.AggregateType, &r.AggregateID, &r.EventType, &r.Version,
			&r.Payload, &r.CorrelationID, &r.OccurredAt, &r.Attempts); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	var failures []error
	for _, r := range batch {
		if ctx.Err() != nil {
			return nil
		}
		if deliverErr := w.Deliverer.Deliver(ctx, envelopeOf(r)); deliverErr != nil {
			failures = append(failures, w.retry(ctx, r, deliverErr))
			continue
		}
		if _, err = w.DB.Exec(ctx, `UPDATE hauling_domain_outbox SET delivered_at=now(), lease_owner=NULL,
			lease_until=NULL, last_error=NULL WHERE id=$1::uuid`, r.ID); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// retry schedules the next attempt, or dead-letters once the budget is spent.
// Analytics answers every stored outcome -- duplicate and stale included --
// with success, so a failure here is either transport or a malformed row.
func (w *Worker) retry(ctx context.Context, r row, cause error) error {
	delay := w.BaseBackoff
	for i := 1; i < r.Attempts && delay < w.MaxBackoff; i++ {
		delay *= 2
	}
	if delay > w.MaxBackoff {
		delay = w.MaxBackoff
	}
	message := cause.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := w.DB.Exec(ctx, `
		UPDATE hauling_domain_outbox
		   SET lease_owner=NULL, lease_until=NULL, last_error=$2,
		       available_at=now()+$3::interval,
		       dead_lettered_at=CASE WHEN attempts >= $4 THEN now() ELSE NULL END
		 WHERE id=$1::uuid`, r.ID, message, fmt.Sprintf("%f seconds", delay.Seconds()), w.MaxAttempts)
	if err != nil {
		return err
	}
	return fmt.Errorf("deliver %s: %w", r.EventType, cause)
}

// envelopeOf rebuilds the canonical envelope entirely from the stored row, so a
// redelivery days later reproduces the same fact with the same identity.
//
// The event id the producer stamped into the payload wins over the row id:
// it is what the fact was published as.
func envelopeOf(r row) sharedevents.Envelope {
	var stamped struct {
		EventID       string `json:"event_id"`
		SchemaVersion int    `json:"schema_version"`
	}
	_ = json.Unmarshal(r.Payload, &stamped)
	eventID := stamped.EventID
	if eventID == "" {
		eventID = r.ID
	}
	schema := stamped.SchemaVersion
	if schema < 1 {
		schema = 1
	}
	version := r.Version
	if version < 1 {
		version = 1
	}
	return sharedevents.Envelope{
		EventID: eventID, EventName: r.EventType, SchemaVersion: schema,
		SourceService: "driver-hauling-service", SourceEntityType: r.AggregateType,
		SourceEntityID: r.AggregateID, SourceVersion: version, OccurredAt: r.OccurredAt.UTC(),
		CorrelationID: r.CorrelationID, Payload: r.Payload,
	}
}
