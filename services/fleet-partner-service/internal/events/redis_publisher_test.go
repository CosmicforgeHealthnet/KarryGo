package events

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestStreamValuesCarriesFullEnvelope proves the pure field-mapping function
// carries every field a stream consumer needs to identify and dispatch an
// event without touching this service's database, and that the envelope
// field is exactly the outbox record's stored payload (the full marshalled
// Envelope, not just its inner business payload).
func TestStreamValuesCarriesFullEnvelope(t *testing.T) {
	record := OutboxRecord{
		ID: "evt_1", EventType: "organization.document.registered.v1", SchemaVersion: 1,
		AggregateType: "document", AggregateID: "doc_1", OrganizationID: "org_1",
		Payload: json.RawMessage(`{"eventId":"evt_1","eventType":"organization.document.registered.v1"}`),
	}
	values := streamValues(record)
	want := map[string]any{
		"event_id": "evt_1", "event_type": "organization.document.registered.v1",
		"schema_version": 1, "aggregate_type": "document", "aggregate_id": "doc_1",
		"organization_id": "org_1", "envelope": string(record.Payload),
	}
	for key, wantValue := range want {
		if values[key] != wantValue {
			t.Errorf("streamValues[%s] = %v, want %v", key, values[key], wantValue)
		}
	}
	if len(values) != len(want) {
		t.Errorf("streamValues has %d fields, want %d: %+v", len(values), len(want), values)
	}
}

// TestRedisStreamPublisherWithoutClientFailsClosed proves an unconfigured
// publisher answers the same sentinel DisabledPublisher does, so the outbox
// worker's "transport missing, do not consume an attempt" branch still
// applies when a RedisStreamPublisher is constructed with a nil client.
func TestRedisStreamPublisherWithoutClientFailsClosed(t *testing.T) {
	publisher := NewRedisStreamPublisher(nil, "")
	err := publisher.Publish(context.Background(), OutboxRecord{ID: "evt_1"})
	if !errors.Is(err, ErrPublisherNotConfigured) {
		t.Fatalf("err = %v, want ErrPublisherNotConfigured", err)
	}
}

// TestRedisStreamPublisherRoundTrip proves a real XADD lands a real,
// consumer-readable message on the stream, with every field intact.
//
// Env-gated on FLEET_PARTNER_TEST_REDIS_ADDR, skipping when unset — matching
// the FLEET_PARTNER_TEST_DATABASE_URL convention every Postgres-backed
// integration test in this service already uses for an external dependency
// this test process does not manage the lifecycle of.
func TestRedisStreamPublisherRoundTrip(t *testing.T) {
	addr := os.Getenv("FLEET_PARTNER_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("FLEET_PARTNER_TEST_REDIS_ADDR not set; skipping real Redis round-trip")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis not reachable at %s: %v", addr, err)
	}

	stream := "fleet-partner:events:test:" + time.Now().Format("20060102150405.000000000")
	t.Cleanup(func() { _ = client.Del(context.Background(), stream).Err() })
	publisher := NewRedisStreamPublisher(client, stream)

	record := OutboxRecord{
		ID: "evt_roundtrip", EventType: "organization.document.registered.v1", SchemaVersion: 1,
		AggregateType: "document", AggregateID: "doc_1", OrganizationID: "org_1",
		Payload: json.RawMessage(`{"eventId":"evt_roundtrip"}`),
	}
	if err := publisher.Publish(ctx, record); err != nil {
		t.Fatalf("publish: %v", err)
	}

	messages, err := client.XRange(ctx, stream, "-", "+").Result()
	if err != nil {
		t.Fatalf("xrange: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("stream has %d messages, want 1", len(messages))
	}
	if messages[0].Values["event_id"] != "evt_roundtrip" {
		t.Fatalf("stream message = %+v, want event_id evt_roundtrip", messages[0].Values)
	}
	if messages[0].Values["envelope"] != string(record.Payload) {
		t.Fatalf("stream message envelope = %v, want %s", messages[0].Values["envelope"], record.Payload)
	}
}

func TestRedisConsumerGroupRestartAckRetryAndAtLeastOnce(t *testing.T) {
	addr := os.Getenv("FLEET_PARTNER_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("FLEET_PARTNER_TEST_REDIS_ADDR not set; real Redis is required in CI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	stream := "fleet-partner:events:group:" + time.Now().Format("20060102150405.000000000")
	group := "section26-real"
	defer client.Del(context.Background(), stream)
	if err := client.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil {
		t.Fatal(err)
	}
	record := OutboxRecord{ID: "evt_delivery", EventType: "provider.availability.changed.v1", SchemaVersion: 1, AggregateType: "provider", AggregateID: "provider_1", OrganizationID: "org_1", Payload: json.RawMessage(`{"eventId":"evt_delivery"}`)}
	if err := NewRedisStreamPublisher(client, stream).Publish(ctx, record); err != nil {
		t.Fatal(err)
	}
	first, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: group, Consumer: "consumer-a", Streams: []string{stream, ">"}, Count: 1}).Result()
	if err != nil || len(first) != 1 || len(first[0].Messages) != 1 {
		t.Fatalf("xreadgroup delivery: %v %v", first, err)
	}
	id := first[0].Messages[0].ID
	// No acknowledgement simulates a crash. A restarted consumer claims the
	// same entry, proving at-least-once rather than at-most-once delivery.
	claimed, _, err := client.XAutoClaim(ctx, &redis.XAutoClaimArgs{Stream: stream, Group: group, Consumer: "consumer-b", MinIdle: 0, Start: "0-0", Count: 10}).Result()
	if err != nil || len(claimed) != 1 || claimed[0].ID != id {
		t.Fatalf("restart claim: %v %v", claimed, err)
	}
	if n, err := client.XAck(ctx, stream, group, id).Result(); err != nil || n != 1 {
		t.Fatalf("ack: n=%d err=%v", n, err)
	}
	// A retry uses the identical event id and may create another stream entry;
	// the transactional inbox, not Redis, is the deduplication authority.
	if err := NewRedisStreamPublisher(client, stream).Publish(ctx, record); err != nil {
		t.Fatal(err)
	}
	if count, err := client.XLen(ctx, stream).Result(); err != nil || count != 2 {
		t.Fatalf("at-least-once retry count=%d err=%v", count, err)
	}
	unavailable := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, ReadTimeout: 100 * time.Millisecond, WriteTimeout: 100 * time.Millisecond})
	defer unavailable.Close()
	if err := NewRedisStreamPublisher(unavailable, stream).Publish(ctx, record); err == nil {
		t.Fatal("unavailable Redis must return a retryable publish failure")
	}
}
