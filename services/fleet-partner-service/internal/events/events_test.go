package events

import (
	"context"
	"errors"
	"testing"
)

func TestNewEnvelopeRequiresOrganizationForOwnedEvents(t *testing.T) {
	_, err := NewEnvelope("corporate.organization.created", 1, "organization", "org_1", 1, "", "cor_1", nil, false)
	if err == nil {
		t.Fatal("organization-owned event without organization id must fail")
	}

	envelope, err := NewEnvelope("corporate.organization.created", 1, "organization", "org_1", 1, "org_1", "cor_1", map[string]string{"k": "v"}, false)
	if err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}
	if envelope.EventID == "" || envelope.Producer != "fleet-partner-service" || envelope.OccurredAt.IsZero() {
		t.Fatalf("envelope incomplete: %+v", envelope)
	}
}

func TestPlatformScopedEventAllowed(t *testing.T) {
	envelope, err := NewEnvelope("corporate.platform.migrated", 1, "service", "fleet-partner-service", 1, "", "cor_1", nil, true)
	if err != nil {
		t.Fatalf("platform-scoped envelope rejected: %v", err)
	}
	if envelope.OrganizationID != "" {
		t.Fatalf("platform-scoped event should have no organization: %+v", envelope)
	}
}

func TestNewEnvelopeValidation(t *testing.T) {
	if _, err := NewEnvelope("", 1, "organization", "org_1", 1, "org_1", "c", nil, false); err == nil {
		t.Fatal("missing event type must fail")
	}
	if _, err := NewEnvelope("x", 0, "organization", "org_1", 1, "org_1", "c", nil, false); err == nil {
		t.Fatal("schema version 0 must fail")
	}
}

func TestDisabledPublisherFailsExplicitly(t *testing.T) {
	err := DisabledPublisher{}.Publish(context.Background(), OutboxRecord{})
	if !errors.Is(err, ErrPublisherNotConfigured) {
		t.Fatalf("expected ErrPublisherNotConfigured, got %v", err)
	}
}

func TestHashPayloadStable(t *testing.T) {
	a := HashPayload([]byte(`{"x":1}`))
	b := HashPayload([]byte(`{"x":1}`))
	c := HashPayload([]byte(`{"x":2}`))
	if a != b || a == c {
		t.Fatalf("hashing not stable/distinct: %s %s %s", a, b, c)
	}
}
