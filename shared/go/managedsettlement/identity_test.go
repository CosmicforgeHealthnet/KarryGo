package managedsettlement_test

import (
	"testing"

	"cosmicforge/logistics/shared/go/managedsettlement"
)

func TestCanonicalSourceAcceptsEverySpellingOfAService(t *testing.T) {
	cases := map[string]string{
		"TAXI": "TAXI", "taxi": "TAXI", " Taxi ": "TAXI",
		"taxi-service": "TAXI", "driver-taxi-service": "TAXI",
		"DISPATCH": "DISPATCH", "dispatch": "DISPATCH",
		"dispatch-delivery-service": "DISPATCH", "driver-dispatch-delivery-service": "DISPATCH",
		"HAULING": "HAULING", "hauling": "HAULING",
		"driver-hauling-service": "HAULING", "hauling-service": "HAULING",
	}
	for name, want := range cases {
		got, ok := managedsettlement.CanonicalSource(name)
		if !ok || got != want {
			t.Errorf("CanonicalSource(%q) = %q,%t want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"", "FLEET", "vendor-service", "taxi-services", "fleet-partner-service"} {
		if got, ok := managedsettlement.CanonicalSource(name); ok {
			t.Errorf("CanonicalSource(%q) = %q, want unknown", name, got)
		}
	}
}

// No spelling may belong to two services: a payment that could resolve to two
// canonical services would let one service's settlement release another's
// money.
func TestPaymentSourceSpellingsAreDisjoint(t *testing.T) {
	seen := map[string]string{}
	for _, canonical := range []string{managedsettlement.SourceTaxi, managedsettlement.SourceDispatch, managedsettlement.SourceHauling} {
		spellings := managedsettlement.PaymentSourceServices(canonical)
		if len(spellings) == 0 {
			t.Fatalf("%s has no payment spellings", canonical)
		}
		for _, spelling := range spellings {
			if other, dup := seen[spelling]; dup {
				t.Fatalf("spelling %q belongs to both %s and %s", spelling, other, canonical)
			}
			seen[spelling] = canonical
		}
	}
	if managedsettlement.PaymentSourceServices("FLEET") != nil {
		t.Fatal("FLEET must have no payment spellings")
	}
	// The returned slice is a copy; mutating it cannot widen the table.
	spellings := managedsettlement.PaymentSourceServices("TAXI")
	spellings[0] = "vendor-service"
	if _, ok := managedsettlement.CanonicalSource("vendor-service"); ok {
		t.Fatal("PaymentSourceServices leaked its backing array")
	}
}
