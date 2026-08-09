package events

import (
	"context"
	"testing"
)

func TestDecodeMessageRoundTripsAWrittenEnvelope(t *testing.T) {
	record := OutboxRecord{
		ID: "evt_1", EventType: "organization.document.registered.v1", SchemaVersion: 1,
		AggregateType: "document", AggregateID: "doc_1", OrganizationID: "org_1",
		Payload: []byte(`{"eventId":"evt_1","eventType":"organization.document.registered.v1","aggregateId":"doc_1","aggregateVersion":1,"schemaVersion":1}`),
	}
	envelope, eventType, err := decodeMessage(streamValues(record))
	if err != nil {
		t.Fatalf("decodeMessage: %v", err)
	}
	if eventType != "organization.document.registered.v1" {
		t.Fatalf("eventType = %s", eventType)
	}
	if envelope.EventID != "evt_1" || envelope.AggregateID != "doc_1" {
		t.Fatalf("envelope decoded incorrectly: %+v", envelope)
	}
}

func TestDecodeMessageRejectsMissingFields(t *testing.T) {
	cases := []map[string]any{
		{"envelope": `{"eventId":"evt_1"}`},
		{"event_type": "x"},
		{"event_type": "x", "envelope": "not json"},
	}
	for _, values := range cases {
		if _, _, err := decodeMessage(values); err == nil {
			t.Errorf("expected an error for %+v", values)
		}
	}
}

func TestStreamConsumerHandleDuplicatePanics(t *testing.T) {
	consumer := NewStreamConsumer(nil, nil, "test-consumer", "", "", nil)
	consumer.Handle("test.stream_consumer.dup.v1", func(context.Context, Envelope) error { return nil })
	defer func() {
		if recover() == nil {
			t.Fatal("expected a duplicate handler registration to panic")
		}
	}()
	consumer.Handle("test.stream_consumer.dup.v1", func(context.Context, Envelope) error { return nil })
}

// TestStreamConsumerRunWithNilClientReturnsImmediately proves an
// unconfigured consumer (no Redis client) does not block or error, matching
// the same "unconfigured degrades safely" posture as the rest of this
// service's optional dependencies.
func TestStreamConsumerRunWithNilClientReturnsImmediately(t *testing.T) {
	consumer := NewStreamConsumer(nil, nil, "test-consumer", "", "", nil)
	consumer.Run(context.Background(), 0) // must return immediately, not block
}
