package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tracing"
)

// StreamHandler processes one decoded envelope read from the stream. An
// error means "retry later" — the message is left unacknowledged and will
// be redelivered (to this consumer, via XCLAIM-style pending-entry recovery,
// or to another consumer in the group) rather than lost.
type StreamHandler func(ctx context.Context, envelope Envelope) error
type TransactionalStreamHandler func(ctx context.Context, tx pgx.Tx, envelope Envelope) error

// StreamConsumer reads DefaultStreamKey through a named Redis consumer
// group, deduplicates via the same InboxRepository every HTTP-delivered
// event already uses, and dispatches to a registered StreamHandler by event
// type.
//
// Before this, nothing in this service consumed from the stream
// RedisStreamPublisher (WP9) started actually delivering to — the outbox had
// a real transport and nothing on the receiving end. This is the other half
// of that pipe.
type StreamConsumer struct {
	client       *redis.Client
	inbox        *InboxRepository
	pool         database.Querier
	stream       string
	group        string
	consumerName string
	metrics      *observability.Metrics

	handlers   map[string]StreamHandler
	txHandlers map[string]TransactionalStreamHandler
	txPool     *pgxpool.Pool
	// environment labels every structured log line (cfg.AppEnv). Empty is
	// valid and is what every existing NewStreamConsumer call site gets
	// until it opts in via WithEnvironment.
	environment string
}

// WithEnvironment sets the deployment environment label for structured
// consumer logs. Optional — see the environment field's own doc comment.
func (c *StreamConsumer) WithEnvironment(env string) *StreamConsumer {
	c.environment = env
	return c
}

// NewStreamConsumer builds a consumer. stream/group default to
// DefaultStreamKey/DefaultConsumerGroup when empty.
func NewStreamConsumer(client *redis.Client, pool database.Querier, consumerName string,
	stream string, group string, metrics *observability.Metrics) *StreamConsumer {
	if stream == "" {
		stream = DefaultStreamKey
	}
	if group == "" {
		group = DefaultConsumerGroup
	}
	if metrics == nil {
		metrics = observability.NewMetrics()
	}
	return &StreamConsumer{
		client: client, inbox: NewInboxRepository(pool), pool: pool,
		stream: stream, group: group, consumerName: consumerName, metrics: metrics,
		handlers: map[string]StreamHandler{}, txHandlers: map[string]TransactionalStreamHandler{},
	}
}

// WithTransactionPool enables atomic inbox + projection updates. It is kept
// separate from the legacy Querier constructor so existing non-stream tests
// and HTTP-owned transactional consumers remain source compatible.
func (c *StreamConsumer) WithTransactionPool(pool *pgxpool.Pool) *StreamConsumer {
	c.txPool = pool
	return c
}

// Handle registers the handler for one event type. Registering the same
// type twice panics — the same collision-is-a-bug posture Register already
// takes in the event-type registry, for the same reason: two handlers
// silently splitting one event type between them is never intended.
func (c *StreamConsumer) Handle(eventType string, handler StreamHandler) {
	if _, exists := c.handlers[eventType]; exists || c.txHandlers[eventType] != nil {
		panic(fmt.Sprintf("events: a handler is already registered for %q", eventType))
	}
	c.handlers[eventType] = handler
}

func (c *StreamConsumer) HandleTransactional(eventType string, handler TransactionalStreamHandler) {
	if c.handlers[eventType] != nil || c.txHandlers[eventType] != nil {
		panic(fmt.Sprintf("events: a handler is already registered for %q", eventType))
	}
	def, ok := Lookup(eventType)
	if !ok || def.Direction != Consumed {
		panic(fmt.Sprintf("events: consumed contract is not registered for %q", eventType))
	}
	c.txHandlers[eventType] = handler
}

// decodeMessage extracts the envelope and event type from one stream
// message's field values. Pure and independent of a live Redis connection,
// so it is unit-testable without one — the same reasoning streamValues was
// built this way for.
func decodeMessage(values map[string]any) (Envelope, string, error) {
	eventType, _ := values["event_type"].(string)
	raw, _ := values["envelope"].(string)
	if eventType == "" || raw == "" {
		return Envelope{}, "", fmt.Errorf("events: stream message missing event_type or envelope field")
	}
	var envelope Envelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return Envelope{}, "", fmt.Errorf("events: decode stream envelope: %w", err)
	}
	return envelope, eventType, nil
}

// Run polls until ctx is cancelled, dispatching each delivered message to
// its registered handler (if any) and acknowledging on success.
//
// A message whose event type has no registered handler is acknowledged
// without dispatch — the same "ignored, not retried forever" posture
// IngestMediaEvent already takes for an event about an object this service
// does not hold: this stream carries every event type this service
// produces, and a consumer naturally only cares about a subset.
func (c *StreamConsumer) Run(ctx context.Context, block time.Duration) {
	if c.client == nil {
		return
	}
	if err := c.client.XGroupCreateMkStream(ctx, c.stream, c.group, "0").Err(); err != nil &&
		!errors.Is(err, redis.Nil) && !strings.Contains(err.Error(), "BUSYGROUP") {
		// BUSYGROUP (group already exists) is the expected steady-state case;
		// go-redis surfaces it as a plain error, not redis.Nil, so it is not
		// distinguished from a real failure here — the next XReadGroup call
		// below will fail loudly and visibly (via the poll error counter) if
		// the group genuinely could not be created for another reason.
		_ = err
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// Recover entries abandoned by a crashed consumer before reading new
		// ones. XAUTOCLAIM provides restart-safe at-least-once delivery.
		pending, _, claimErr := c.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{Stream: c.stream, Group: c.group, Consumer: c.consumerName, MinIdle: 30 * time.Second, Start: "0-0", Count: 20}).Result()
		if claimErr == nil {
			for _, message := range pending {
				c.process(ctx, message)
			}
		}
		streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: c.group, Consumer: c.consumerName,
			Streams: []string{c.stream, ">"}, Count: 20, Block: block,
		}).Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				c.metrics.IncCounter("fleet_partner_stream_consumer_poll_errors_total",
					map[string]string{"consumer": c.consumerName}, 1)
			}
			continue
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				c.process(ctx, message)
			}
		}
	}
}

func (c *StreamConsumer) process(ctx context.Context, message redis.XMessage) {
	envelope, eventType, err := decodeMessage(message.Values)
	if err != nil {
		// A message this consumer cannot even parse is acknowledged rather
		// than retried forever — retrying will never make it parseable.
		c.metrics.IncCounter("fleet_partner_stream_consumer_decode_errors_total", nil, 1)
		observability.Log(config.ServiceName, c.environment, "error", "stream_consumer_decode_error", map[string]any{
			"consumer": c.consumerName, "safeErrorClass": "decode_error",
		})
		_ = c.client.XAck(ctx, c.stream, c.group, message.ID).Err()
		return
	}

	// Async spans link back to the trace that produced this event via
	// envelope.CorrelationID (see internal/platform/tracing's own doc
	// comment on why an async boundary links by correlationId rather than a
	// persisted trace-id column).
	spanCtx, endSpan := tracing.StartSpan(ctx, c.metrics, config.ServiceName, c.environment,
		"stream.consume."+eventType, map[string]any{
			"eventId": envelope.EventID, "eventType": eventType,
			"aggregateType": envelope.AggregateType, "aggregateId": envelope.AggregateID,
			"correlationId": envelope.CorrelationID, "consumer": c.consumerName,
		})
	ctx = spanCtx
	logFields := map[string]any{
		"eventId": envelope.EventID, "eventType": eventType,
		"aggregateType": envelope.AggregateType, "aggregateId": envelope.AggregateID,
		"correlationId": envelope.CorrelationID, "consumer": c.consumerName,
	}
	var finalErr error
	defer func() { endSpan(finalErr) }()
	logOutcome := func(event, level string, extra map[string]any) {
		fields := make(map[string]any, len(logFields)+len(extra))
		for k, v := range logFields {
			fields[k] = v
		}
		for k, v := range extra {
			fields[k] = v
		}
		observability.Log(config.ServiceName, c.environment, level, event, fields)
	}

	if envelope.EventType != eventType || ValidateEnvelope(envelope) != nil {
		c.metrics.IncCounter("fleet_partner_stream_consumer_contract_errors_total", nil, 1)
		finalErr = fmt.Errorf("contract violation")
		logOutcome("stream_consumer_contract_error", "error", map[string]any{"safeErrorClass": "contract_violation"})
		_ = c.client.XAck(ctx, c.stream, c.group, message.ID).Err()
		return
	}
	if handler := c.txHandlers[eventType]; handler != nil {
		if c.txPool == nil {
			return
		}
		consumerKey := envelope.Producer + ":" + c.consumerName
		err = database.WithinTx(ctx, c.txPool, func(tx pgx.Tx) error {
			if beginErr := c.inbox.Begin(ctx, tx, consumerKey, envelope.EventID, envelope.EventType, envelope.SchemaVersion, envelope.Payload); beginErr != nil {
				return beginErr
			}
			if handlerErr := handler(ctx, tx, envelope); handlerErr != nil {
				return handlerErr
			}
			return c.inbox.MarkProcessed(ctx, tx, consumerKey, envelope.EventID)
		})
		if errors.Is(err, ErrDuplicateEvent) {
			logOutcome("stream_consumer_duplicate", "info", nil)
			_ = c.client.XAck(ctx, c.stream, c.group, message.ID).Err()
			return
		}
		if err != nil {
			c.metrics.IncCounter("fleet_partner_stream_consumer_handler_errors_total", map[string]string{"eventType": eventType}, 1)
			finalErr = err
			logOutcome("stream_consumer_handler_error", "error", map[string]any{"safeErrorClass": "handler_error"})
			return
		}
		logOutcome("stream_consumer_processed", "info", nil)
		c.metrics.IncCounter("fleet_partner_stream_consumer_processed_total", map[string]string{"eventType": eventType}, 1)
		_ = c.client.XAck(ctx, c.stream, c.group, message.ID).Err()
		return
	}

	handler, registered := c.handlers[eventType]
	if !registered {
		_ = c.client.XAck(ctx, c.stream, c.group, message.ID).Err()
		return
	}

	if beginErr := c.inbox.Begin(ctx, c.pool, c.consumerName, envelope.EventID, envelope.EventType,
		envelope.SchemaVersion, envelope.Payload); beginErr != nil {
		if errors.Is(beginErr, ErrDuplicateEvent) {
			logOutcome("stream_consumer_duplicate", "info", nil)
			_ = c.client.XAck(ctx, c.stream, c.group, message.ID).Err()
			return
		}
		if errors.Is(beginErr, ErrPayloadMismatch) {
			c.metrics.IncCounter("fleet_partner_stream_consumer_payload_conflicts_total", nil, 1)
			finalErr = beginErr
			logOutcome("stream_consumer_payload_conflict", "error", map[string]any{"safeErrorClass": "payload_conflict"})
			_ = c.client.XAck(ctx, c.stream, c.group, message.ID).Err()
			return
		}
		c.metrics.IncCounter("fleet_partner_stream_consumer_inbox_errors_total", nil, 1)
		finalErr = beginErr
		logOutcome("stream_consumer_inbox_error", "error", map[string]any{"safeErrorClass": "inbox_error"})
		return // leave unacknowledged; retried on next read of the pending entry
	}

	if handlerErr := handler(ctx, envelope); handlerErr != nil {
		_ = c.inbox.MarkFailed(ctx, c.consumerName, envelope.EventID, redactError(handlerErr))
		c.metrics.IncCounter("fleet_partner_stream_consumer_handler_errors_total",
			map[string]string{"eventType": eventType}, 1)
		finalErr = handlerErr
		logOutcome("stream_consumer_handler_error", "error", map[string]any{"safeErrorClass": "handler_error"})
		return // leave unacknowledged; redelivered and retried
	}

	if markErr := c.inbox.MarkProcessed(ctx, c.pool, c.consumerName, envelope.EventID); markErr != nil {
		c.metrics.IncCounter("fleet_partner_stream_consumer_inbox_errors_total", nil, 1)
		finalErr = markErr
		logOutcome("stream_consumer_inbox_error", "error", map[string]any{"safeErrorClass": "inbox_error"})
		return
	}
	c.metrics.IncCounter("fleet_partner_stream_consumer_processed_total",
		map[string]string{"eventType": eventType}, 1)
	logOutcome("stream_consumer_processed", "info", nil)
	_ = c.client.XAck(ctx, c.stream, c.group, message.ID).Err()
}
