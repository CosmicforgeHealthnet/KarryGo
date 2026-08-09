package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRegisterAndLookup(t *testing.T) {
	Register(EventDefinition{
		Type: "test.registry.example.v1", SchemaVersion: 1, AggregateType: "example",
		Description: "used only by TestRegisterAndLookup",
	})
	def, ok := Lookup("test.registry.example.v1")
	if !ok {
		t.Fatal("expected the registered definition to be found")
	}
	if def.AggregateType != "example" {
		t.Fatalf("aggregateType = %s, want example", def.AggregateType)
	}
	if _, ok := Lookup("test.registry.does_not_exist.v1"); ok {
		t.Fatal("expected an unregistered type to be absent")
	}
}

func TestSection26RegistryIsCompleteAndStrict(t *testing.T) {
	required := []string{"organization.created.v1", "organization.status_changed.v1", "organization.provider.provisioned.v1", "organization.provider.membership_changed.v1", "organization.request.accepted.v1", "organization.assignment.created.v1", "organization.assignment.accepted.v1", "organization.assignment.declined.v1", "organization.assignment.expired.v1", "organization.asset.blocked.v1", "corporate.order.confirmed.v1", "corporate.shipment.status_changed.v1", "corporate.invoice.issued.v1", "platform.request.offered.v1", "platform.request.cancelled.v1", "provider.availability.changed.v1", "provider.verification.changed.v1", "trip.created.v1", "trip.status_changed.v1", "trip.completed.v1", "vehicle.verification.changed.v1", "wallet.entry.posted.v1", "settlement.created.v1", "settlement.completed.v1", "support.case.changed.v1", "media.asset.status_changed.v1"}
	for _, eventType := range required {
		def, ok := Lookup(eventType)
		if !ok {
			t.Errorf("missing %s", eventType)
			continue
		}
		if len(def.Producers) == 0 || def.PayloadFactory == nil || def.MaxPayloadBytes <= 0 || !def.TenantRequired || len(def.ForbiddenFields) == 0 {
			t.Errorf("incomplete contract for %s: %+v", eventType, def)
		}
	}
}

func TestSection55VisibilitySeparatesPublicBusinessEventsFromLifecycleEvents(t *testing.T) {
	for _, eventType := range []string{
		"organization.status_changed.v1", "organization.provider.membership_changed.v1",
		"organization.assignment.created.v1", "organization.assignment.accepted.v1",
		"organization.assignment.declined.v1", "organization.assignment.expired.v1",
		"corporate.order.confirmed.v1", "corporate.shipment.status_changed.v1", "corporate.invoice.issued.v1",
	} {
		if !IsPublicIntegration(eventType) {
			t.Errorf("%s must be public-integration visible", eventType)
		}
	}
	for _, eventType := range []string{
		"api_client.created.v1", "api_client.rotated.v1", "api_client.revoked.v1",
		"webhook.subscription_created.v1", "webhook.secret_rotated.v1", "webhook.delivery_failed.v1",
	} {
		def, ok := Lookup(eventType)
		if !ok || def.Visibility != VisibilityInternalService || IsPublicIntegration(eventType) {
			t.Errorf("lifecycle event visibility is unsafe for %s: %+v", eventType, def)
		}
	}
	if IsPublicIntegration("organization.status_changed.v1.extra") {
		t.Fatal("public integration visibility accepted a substring event type")
	}
}

func TestSection55LifecyclePayloadRejectsCredentials(t *testing.T) {
	payload, _ := json.Marshal(IntegrationContractPayload{SubjectReference: "apc_1", SourceVersion: 1, Status: "ACTIVE"})
	envelope := Envelope{EventID: "evt_s55", EventType: "api_client.created.v1", SchemaVersion: 1,
		AggregateType: "organization_api_client", AggregateID: "apc_1", AggregateVersion: 1,
		OrganizationID: "org_1", CorrelationID: "cor_1", OccurredAt: time.Now(),
		Producer: "fleet-partner-service", Payload: payload}
	if err := ValidateEnvelope(envelope); err != nil {
		t.Fatalf("valid Section 55 lifecycle payload rejected: %v", err)
	}
	envelope.Payload = []byte(`{"subjectReference":"apc_1","sourceVersion":1,"secret":"leak"}`)
	if err := ValidateEnvelope(envelope); err == nil {
		t.Fatal("credential-bearing Section 55 event payload accepted")
	}
}

func TestValidateEnvelopeUsesExactProducerAndMinimizedTypedPayload(t *testing.T) {
	payload, _ := json.Marshal(ContractPayload{SubjectReference: "provider_1", SourceVersion: 2, Status: "AVAILABLE"})
	envelope := Envelope{EventID: "evt_1", EventType: "provider.availability.changed.v1", SchemaVersion: 1, AggregateType: "provider", AggregateID: "provider_1", AggregateVersion: 2, OrganizationID: "org_1", CorrelationID: "cor_1", OccurredAt: time.Now(), Producer: "driver-dispatch-delivery-service", Payload: payload}
	if err := ValidateEnvelope(envelope); err != nil {
		t.Fatalf("valid contract: %v", err)
	}
	envelope.Producer = "driver-dispatch-delivery-service-evil"
	if err := ValidateEnvelope(envelope); err == nil {
		t.Fatal("substring producer must be rejected")
	}
	envelope.Producer = "driver-dispatch-delivery-service"
	envelope.Payload = []byte(`{"subjectReference":"provider_1","sourceVersion":2,"token":"secret"}`)
	if err := ValidateEnvelope(envelope); err == nil {
		t.Fatal("credential-bearing payload must be rejected")
	}
}

func TestRegisterPanicsOnDuplicate(t *testing.T) {
	Register(EventDefinition{Type: "test.registry.duplicate.v1", SchemaVersion: 1, AggregateType: "example"})
	defer func() {
		if recover() == nil {
			t.Fatal("expected a duplicate registration to panic")
		}
	}()
	Register(EventDefinition{Type: "test.registry.duplicate.v1", SchemaVersion: 1, AggregateType: "example"})
}

func TestRegisterRejectsIncompleteDefinitions(t *testing.T) {
	cases := []EventDefinition{
		{Type: "", SchemaVersion: 1, AggregateType: "example"},
		{Type: "test.registry.no_version.v1", SchemaVersion: 0, AggregateType: "example"},
		{Type: "test.registry.no_aggregate.v1", SchemaVersion: 1, AggregateType: ""},
	}
	for _, def := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected %+v to panic", def)
				}
			}()
			Register(def)
		}()
	}
}
