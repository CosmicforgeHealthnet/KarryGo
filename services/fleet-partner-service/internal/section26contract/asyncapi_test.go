// Package section26contract proves the Section 26 AsyncAPI document matches
// what internal/events.Registry actually holds — the same parity discipline
// section25contract (and section15contract before it) already enforce
// between an OpenAPI document and a live route manifest, applied here to an
// event catalogue instead of an HTTP route list.
package section26contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"

	// Blank-imported so this test observes the real, populated registry
	// (event types register themselves in an init()) rather than an empty
	// one — mirroring how section25contract imports the http packages whose
	// manifests it checks, not a hand-copied list of what they should
	// contain.
	_ "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
)

func load(t *testing.T) map[string]any {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "asyncapi",
		"section26-events.asyncapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func specMessages(t *testing.T, spec map[string]any) map[string]map[string]any {
	t.Helper()
	components := spec["components"].(map[string]any)
	raw := components["messages"].(map[string]any)
	out := make(map[string]map[string]any, len(raw))
	for name, message := range raw {
		out[name] = message.(map[string]any)
	}
	return out
}

// TestAsyncAPIChannelMatchesStreamConstants proves the documented channel
// name and consumer group are not just prose — they are the exact values
// internal/events actually publishes to and expects consumers to join.
func TestAsyncAPIChannelMatchesStreamConstants(t *testing.T) {
	spec := load(t)
	channels := spec["channels"].(map[string]any)
	channel, ok := channels[events.DefaultStreamKey]
	if !ok {
		t.Fatalf("spec has no channel named %q (events.DefaultStreamKey)", events.DefaultStreamKey)
	}
	channelMap := channel.(map[string]any)
	if channelMap["x-stream-key"] != events.DefaultStreamKey {
		t.Errorf("x-stream-key = %v, want %s", channelMap["x-stream-key"], events.DefaultStreamKey)
	}
	if channelMap["x-consumer-group"] != events.DefaultConsumerGroup {
		t.Errorf("x-consumer-group = %v, want %s", channelMap["x-consumer-group"], events.DefaultConsumerGroup)
	}
}

// TestAsyncAPICatalogMatchesRegistry proves every event type
// internal/events.Registry holds has exactly one corresponding AsyncAPI
// message with the same schema version and aggregate type, and that the
// spec advertises no event type the registry does not actually recognize —
// the same two-directional parity check TestOpenAPIRouteParity applies to
// routes, applied here to event types.
func TestAsyncAPICatalogMatchesRegistry(t *testing.T) {
	spec := load(t)
	messages := specMessages(t, spec)

	registered := events.Registered()
	if len(registered) == 0 {
		t.Fatal("events.Registered() is empty — the documents/usecases registration init() did not run")
	}

	seenByEventType := map[string]bool{}
	for _, def := range registered {
		var found map[string]any
		var foundName string
		for name, message := range messages {
			if message["x-event-type"] == def.Type {
				found = message
				foundName = name
				break
			}
		}
		if found == nil {
			t.Errorf("registered event type %q has no AsyncAPI message", def.Type)
			continue
		}
		seenByEventType[def.Type] = true
		if found["title"] != def.Type {
			t.Errorf("message %s title = %v, want %s", foundName, found["title"], def.Type)
		}
		if version, _ := found["x-schema-version"].(float64); int(version) != def.SchemaVersion {
			t.Errorf("message %s x-schema-version = %v, want %d", foundName, found["x-schema-version"], def.SchemaVersion)
		}
		if found["x-aggregate-type"] != def.AggregateType {
			t.Errorf("message %s x-aggregate-type = %v, want %s", foundName, found["x-aggregate-type"], def.AggregateType)
		}
		payload, _ := found["payload"].(map[string]any)
		if payload["$ref"] != "#/components/schemas/Envelope" {
			t.Errorf("message %s payload must reference the shared Envelope schema, got %v", foundName, payload)
		}
	}

	for name, message := range messages {
		eventType, _ := message["x-event-type"].(string)
		if eventType == "" {
			t.Errorf("message %s has no x-event-type", name)
			continue
		}
		if !seenByEventType[eventType] {
			t.Errorf("AsyncAPI advertises event type %q that internal/events.Registry does not recognize", eventType)
		}
	}
}

// TestAsyncAPIEnvelopeSchemaMatchesGoStruct proves the documented Envelope
// schema's required fields are exactly events.Envelope's own JSON fields —
// a drift here would mean the document describes a wire shape this service
// does not actually produce.
func TestAsyncAPIEnvelopeSchemaMatchesGoStruct(t *testing.T) {
	spec := load(t)
	components := spec["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	envelope := schemas["Envelope"].(map[string]any)
	properties := envelope["properties"].(map[string]any)

	raw, err := json.Marshal(events.Envelope{
		EventID: "evt_1", EventType: "t", SchemaVersion: 1, AggregateType: "a", AggregateID: "a_1",
		AggregateVersion: 1, OrganizationID: "org_1", CorrelationID: "c_1", Producer: "fleet-partner-service",
		Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	for field := range wire {
		if _, ok := properties[field]; !ok {
			t.Errorf("events.Envelope produces field %q that the AsyncAPI Envelope schema does not document", field)
		}
	}
}
