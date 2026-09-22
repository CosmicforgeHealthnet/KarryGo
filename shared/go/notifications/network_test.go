package notifications

import (
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
)

func intent() NetworkIntent {
	return NetworkIntent{EventType: EventNetworkCustomerProviderReplaced, RecipientType: RecipientCustomer,
		RecipientID: "c1", SourceEventID: "case-1:v2", SourceAggregateType: "recovery_case",
		SourceAggregateID: "case-1", SourceVersion: 2, Data: map[string]interface{}{"booking_id": "b1"}}
}

func TestNetworkRequestDedupesOnFactRecipientAndTemplate(t *testing.T) {
	a := NetworkRequest("driver-hauling-service", intent())
	b := NetworkRequest("driver-hauling-service", intent())
	if a.IDempotencyKey != b.IDempotencyKey || a.TemplateKey != EventNetworkCustomerProviderReplaced ||
		a.SourceEventID != "case-1:v2" {
		t.Fatalf("request %+v", a)
	}
	other := intent()
	other.RecipientID = "c2"
	if NetworkRequest("driver-hauling-service", other).IDempotencyKey == a.IDempotencyKey {
		t.Fatal("two recipients of one fact must be two notifications")
	}
	fleet := intent()
	fleet.RecipientType, fleet.RecipientID = RecipientFleetOrganization, "c1"
	if NetworkRequest("driver-hauling-service", fleet).IDempotencyKey == a.IDempotencyKey {
		t.Fatal("the same id in two audiences must be two notifications")
	}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkNotificationsCarryReferencesOnly(t *testing.T) {
	leaky := intent()
	leaky.Data = map[string]interface{}{"booking_id": "b1", "pickup_lat": 6.5, "customerPhone": "+234",
		"fare_kobo": 100, "delivery_code_hash": "x"}
	err := NetworkRequest("driver-hauling-service", leaky).Validate()
	if err == nil {
		t.Fatal("a network notification carrying location, contact and money was accepted")
	}
	for _, key := range []string{"pickup_lat", "customerPhone", "fare_kobo"} {
		if !strings.Contains(err.Error()+errorFields(err), key) {
			t.Errorf("refusal does not name %s", key)
		}
	}
	// Existing non-network notifications are untouched by this rule.
	legacy := Request{IDempotencyKey: "k", SourceService: "s", EventType: "booking.delivery_code_ready",
		Recipient: Recipient{Type: RecipientCustomer, ID: "c"}, TemplateKey: "t",
		Data: map[string]interface{}{"code": "123456"}}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy notification refused: %v", err)
	}
}

func TestEveryNetworkIntentIsNamespaced(t *testing.T) {
	for _, event := range NetworkEventTypes() {
		if !strings.HasPrefix(event, "network.") {
			t.Errorf("%s escapes the network privacy rule", event)
		}
	}
}

func errorFields(err error) string {
	var out []string
	for _, field := range apperrors.From(err).Fields {
		out = append(out, field.Field+": "+field.Message)
	}
	return strings.Join(out, "; ")
}
