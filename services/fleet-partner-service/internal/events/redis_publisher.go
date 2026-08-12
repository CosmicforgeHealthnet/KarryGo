package events

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// DefaultStreamKey is the single shared Redis stream every produced corporate
// event is appended to. One stream rather than one per event type: this
// service produces hundreds of event types across two dozen domains, and
// provisioning a stream per type would mean every new event type is also a
// new piece of consumer infrastructure to stand up. A consumer subscribes to
// the one stream and dispatches on the event_type field instead.
const DefaultStreamKey = "fleet-partner:events"

// DefaultConsumerGroup is the consumer-group name typed consumers in this
// service register under, mirroring the shared/go/notifications.ConsumerGroup
// convention notification-service's own Redis queue already uses.
const DefaultConsumerGroup = "fleet-partner-consumers"

// streamMaxLen approximately bounds the stream so an unconsumed backlog
// cannot grow without limit. Approximate trimming (~) is what Redis
// recommends for MAXLEN under load — exact trimming is O(N) per XADD.
const streamMaxLen = 100000

// RedisStreamPublisher delivers claimed outbox records to a Redis stream.
//
// This is the "real transport" the outbox worker was missing: every prior
// deployment ran with events.DisabledPublisher{}, meaning nothing this
// service ever produced left the outbox table. Wiring this in is additive
// and safe — nothing today depends on delivery being absent, since nothing
// could previously receive anything.
type RedisStreamPublisher struct {
	client *redis.Client
	stream string
}

// NewRedisStreamPublisher builds the publisher. An empty stream defaults to
// DefaultStreamKey.
func NewRedisStreamPublisher(client *redis.Client, stream string) *RedisStreamPublisher {
	if stream == "" {
		stream = DefaultStreamKey
	}
	return &RedisStreamPublisher{client: client, stream: stream}
}

// streamValues builds the XADD field set for one outbox record.
//
// record.Payload already holds the entire marshalled Envelope (OutboxRepository.Append
// stores json.Marshal(envelope) in the payload column, not just the inner
// business payload — see outbox.go), so it is carried through verbatim as
// the envelope field rather than only a lookup ID: unlike
// notification-service's own delivery queue (whose consumer is the same
// service and can re-read its own database by ID), fleet-partner-service's
// stream consumers may be other services entirely, with no access to this
// service's database — the stream itself must carry enough to act on.
//
// Extracted as a pure function, independent of a live Redis connection,
// specifically so it stays unit-testable in an environment with no Redis
// reachable.
func streamValues(record OutboxRecord) map[string]any {
	return map[string]any{
		"event_id":        record.ID,
		"event_type":      record.EventType,
		"schema_version":  record.SchemaVersion,
		"aggregate_type":  record.AggregateType,
		"aggregate_id":    record.AggregateID,
		"organization_id": record.OrganizationID,
		"envelope":        string(record.Payload),
	}
}

// Publish implements the Publisher port.
func (p *RedisStreamPublisher) Publish(ctx context.Context, record OutboxRecord) error {
	if p.client == nil {
		return ErrPublisherNotConfigured
	}
	err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		MaxLen: streamMaxLen,
		Approx: true,
		Values: streamValues(record),
	}).Err()
	if err != nil {
		return fmt.Errorf("events: redis stream publish: %w", err)
	}
	return nil
}
