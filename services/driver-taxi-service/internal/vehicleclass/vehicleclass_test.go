package vehicleclass

import "testing"

// Pricing has moved to internal/pricing entirely (addendum §1/§37): this
// package no longer carries a Multiplier or any other rate field, so
// "cheapest first" is now asserted as a fixed name order rather than derived
// from a numeric field on Spec. The passenger app renders the fare tiles
// straight from this order, and internal/pricing's rate config is expected to
// follow the same convention by agreement between the two packages, not by a
// shared type.
func TestAllClassesAreOrderedEconomyComfortXL(t *testing.T) {
	all := All()
	if len(all) != 3 {
		t.Fatalf("expected 3 classes, got %d", len(all))
	}
	want := []string{Economy, Comfort, XL}
	for i, spec := range all {
		if spec.Name != want[i] {
			t.Fatalf("position %d: got %q, want %q", i, spec.Name, want[i])
		}
	}
}

func TestNormalizeAcceptsMessyInput(t *testing.T) {
	for _, in := range []string{"Economy", "  economy  ", "ECONOMY"} {
		if got := Normalize(in); got != Economy {
			t.Fatalf("Normalize(%q) = %q, want %q", in, got, Economy)
		}
	}
}

func TestValidRejectsUnknownClass(t *testing.T) {
	if Valid("helicopter") {
		t.Fatal("an unknown class must not validate")
	}
	if Valid("") {
		t.Fatal("an empty class must not validate")
	}
}

func TestXLSeatsMoreThanEconomy(t *testing.T) {
	economy, _ := Get(Economy)
	xl, _ := Get(XL)
	if xl.MaxPassengers <= economy.MaxPassengers {
		t.Fatalf("XL should carry more passengers than economy (%d vs %d)", xl.MaxPassengers, economy.MaxPassengers)
	}
}

func TestMaxPassengersLeavesRoomForTheDriver(t *testing.T) {
	for _, s := range All() {
		if s.MaxPassengers >= s.DefaultSeats {
			t.Fatalf("%s: MaxPassengers (%d) must be below DefaultSeats (%d) — the driver occupies one",
				s.Name, s.MaxPassengers, s.DefaultSeats)
		}
	}
}
