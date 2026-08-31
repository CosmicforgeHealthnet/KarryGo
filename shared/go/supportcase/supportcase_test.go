package supportcase

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestControlledCategoriesAndFingerprint(t *testing.T) {
	for _, category := range []Category{WrongItemOrMeal, MissingItem, UnauthorizedSubstitution, DamagedOrSpoiltProduct, QualityIssue, VendorDelay, DeliveryDelay, PickupDispute, ReceiverHandoverDispute, RefundProblem, VendorOwnedDeliveryIssue} {
		if !category.Valid() {
			t.Errorf("controlled category rejected: %s", category)
		}
	}
	r := CreateRequest{SchemaVersion: SchemaVersion, OperationRef: "op-1", Category: MissingItem, Subject: "Missing item", Description: "One sealed line was missing", CustomerRef: "customer-1", Context: Context{CommerceOrderRef: "order-1", VendorOrderRef: "vendor-order-1", VendorRef: "vendor-1"}}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	a, _ := Fingerprint(r)
	r.Category = QualityIssue
	b, _ := Fingerprint(r)
	if a == b {
		t.Fatal("materially different requests must not share a fingerprint")
	}
	r.Category = Category("FREE_TEXT")
	if r.Validate() == nil {
		t.Fatal("uncontrolled category accepted")
	}
}

func TestContextIsOpaquePIIMinimizedAndCredentialSafe(t *testing.T) {
	r := CreateRequest{SchemaVersion: 1, OperationRef: "op", Category: PickupDispute, Subject: "Pickup dispute", Description: SanitizeDescription("The handover code was 123456"), CustomerRef: "customer-ref", Context: Context{CommerceOrderRef: "order-ref", VendorOrderRef: "vendor-order-ref", VendorRef: "vendor-ref", EvidenceRefs: []string{"evidence-ref"}, MediaRefs: []string{"media-object-ref"}}}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(raw))
	for _, forbidden := range []string{"123456", "phone", "email", "address", "payment_token", "otp", "qr_secret", "media_bytes"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("typed case leaked forbidden field/material %q: %s", forbidden, text)
		}
	}
}
