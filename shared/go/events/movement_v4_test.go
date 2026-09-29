package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Customer V4 §22.1: the shared movement envelope carries the logical V4
// fields, a bounded stage vocabulary, and nothing outside the summary.
func TestMovementEnvelopeMapsTheV4LogicalEnvelope(t *testing.T) {
	summary := MovementSummary{CustomerID: "c1", Authority: MovementAuthorityDispatch, SourceRef: "trip-1",
		ProductCode: "STANDARD", MovementType: MovementTypeDispatch, State: MovementStateInProgress,
		Stage: MovementStageArrivedDropoff, ParentContextType: "COMMERCE_ORDER", ParentContextRef: "po-1"}
	envelope, err := NewMovementEnvelope("evt-1", EventNameFor(summary.Authority, summary.State),
		"driver-dispatch-delivery-service", "dispatch_booking", 7, time.Now(), "", "", summary)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.EventID != "evt-1" || envelope.SourceService == "" || envelope.SourceVersion != 7 ||
		envelope.SourceEntityID != "trip-1" || envelope.OccurredAt.Location() != time.UTC || envelope.EventName != "dispatch.delivery.started" {
		t.Fatalf("envelope does not carry the logical fields: %+v", envelope)
	}
	var decoded map[string]any
	if err = json.Unmarshal(envelope.Payload, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"customer_id", "authority", "source_ref", "product_code", "state", "stage", "parent_context_ref"} {
		if _, ok := decoded[field]; !ok {
			t.Errorf("payload lacks %s", field)
		}
	}
	// The payload is the typed summary only: no free-form data can ride along.
	for key := range decoded {
		for _, secret := range []string{"otp", "qr", "token", "secret", "credential", "password", "payment"} {
			if strings.Contains(strings.ToLower(key), secret) {
				t.Errorf("payload field %q could carry a secret", key)
			}
		}
	}
}

func TestMovementStageIsBoundedAndOptional(t *testing.T) {
	base := MovementSummary{CustomerID: "c1", Authority: MovementAuthorityTaxi, SourceRef: "r", MovementType: MovementTypePassenger, State: MovementStateAssigned}
	if err := base.Validate(); err != nil {
		t.Fatalf("a stageless summary must stay valid: %v", err)
	}
	base.Stage = MovementStageEnRoutePickup
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	base.Stage = "TELEPORTING"
	if err := base.Validate(); err == nil {
		t.Fatal("an unbounded stage was accepted")
	}
}

func TestVendorCommercePurchaseRowRules(t *testing.T) {
	row := MovementSummary{CustomerID: "c1", Authority: MovementAuthorityVendorCommerce, SourceRef: "po-1",
		ProductCode: "SHOP", MovementType: MovementTypeCommerceOrder, State: MovementStateRequested}
	if err := row.Validate(); err != nil {
		t.Fatal(err)
	}
	if EventNameFor(MovementAuthorityVendorCommerce, MovementStateCancelled) != "commerce.customer_order.cancelled" {
		t.Fatal("purchase row event name must not collide with Vendor's commerce.order.* domain events")
	}
	for name, mutate := range map[string]func(*MovementSummary){
		"non-commerce product":          func(m *MovementSummary) { m.ProductCode = "RIDE" },
		"missing product":               func(m *MovementSummary) { m.ProductCode = "" },
		"commerce authority, goods":     func(m *MovementSummary) { m.MovementType = MovementTypeGoods },
		"logistics authority, order":    func(m *MovementSummary) { m.Authority = MovementAuthorityHauling },
		"refund stage is still bounded": func(m *MovementSummary) { m.Stage = "REFUND_MAYBE" },
	} {
		candidate := row
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
