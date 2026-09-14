package commercemission

// Hauling's declared capability (HLG-031, VND-008).
//
// The specification hypothesised that Hauling might be single-pickup and said
// to declare max=1 if so -- but also to audit first and not hardcode false if
// staging already implements more. It does: the commerce quote routes across
// stops and the mission persists them, so multi-pickup is declared truthfully.
// Declaring false here would have silently removed a capability that works.

import (
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/logisticscapability"
)

func TestHaulingDeclaresTheMultiPickupItActuallyImplements(t *testing.T) {
	capability := CapabilityProvider{Options: CapabilityOptions{
		Enabled: true, CorporateProviderSupported: true,
		Now: func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	}}.DeclaredCapability()

	if capability.Authority != logisticscapability.AuthorityHauling {
		t.Fatalf("authority=%q", capability.Authority)
	}
	if !capability.MultiPickupSupported || capability.MaxPickups != MaxPickupStops {
		t.Fatalf("Hauling routes across stops and must declare it, got multi=%v max=%d",
			capability.MultiPickupSupported, capability.MaxPickups)
	}
	if capability.MultiStopSupported || capability.MaxDropoffs != 1 {
		t.Fatalf("a commerce haul delivers to one place; declaring otherwise would invite a mission it cannot run: %+v", capability)
	}
	if !capability.SecurePickupSupported || !capability.SecureDeliverySupported || !capability.CustodyEvidenceSupported {
		t.Fatal("Hauling implements protected pickup, protected delivery and custody evidence, and must declare all three")
	}
	if len(capability.VehicleClasses) == 0 {
		t.Fatal("Hauling owns van and truck and must name them")
	}
	if capability.ObservedAt.IsZero() {
		t.Fatal("a capability document without an observation time cannot be reasoned about as fresh or stale")
	}
}

func TestHaulingCapabilityReflectsTheCorporateProviderSwitch(t *testing.T) {
	off := CapabilityProvider{Options: CapabilityOptions{Enabled: true}}.DeclaredCapability()
	if off.CorporateProviderSupported {
		t.Fatal("corporate provider support must be reported from configuration, not assumed")
	}
	on := CapabilityProvider{Options: CapabilityOptions{
		Enabled: true, CorporateProviderSupported: true,
	}}.DeclaredCapability()
	if !on.CorporateProviderSupported {
		t.Fatal("an enabled Fleet provider-work deployment must declare corporate provider support")
	}
}

func TestDisabledHaulingCommerceDeclaresItselfUnavailable(t *testing.T) {
	capability := CapabilityProvider{Options: CapabilityOptions{Enabled: false}}.DeclaredCapability()
	if capability.Available {
		t.Fatal("a disabled commerce integration must not declare itself available")
	}
	answer := logisticscapability.Evaluate(capability, logisticscapability.Requirement{
		PickupCount: 1, DropoffCount: 1,
	})
	if answer.Serviceable {
		t.Fatal("an unavailable authority is never serviceable")
	}
}

func TestHaulingRefusesMorePickupsThanItDeclares(t *testing.T) {
	capability := CapabilityProvider{Options: CapabilityOptions{Enabled: true}}.DeclaredCapability()

	beyond := logisticscapability.Evaluate(capability, logisticscapability.Requirement{
		PickupCount: MaxPickupStops + 1, DropoffCount: 1,
	})
	if beyond.Serviceable || beyond.ReasonCode != logisticscapability.ReasonTooManyPickups {
		t.Fatalf("more pickups than declared must be refused: %+v", beyond)
	}
	// A mission at the declared routing limit is serviceable. Protected
	// PICKUP is deliberately not requested here: Hauling issues one pickup
	// credential per mission, and the bound for that is asserted separately
	// by TestHaulingDeclaresOneProtectedPickup. Asking for both together
	// used to pass only because the protected bound was inferred from the
	// routing bound, which over-promised it.
	within := logisticscapability.Evaluate(capability, logisticscapability.Requirement{
		PickupCount: MaxPickupStops, DropoffCount: 1,
		SecureDelivery: true,
	})
	if !within.Serviceable {
		t.Fatalf("a mission at the declared limit must be serviceable: %+v", within)
	}
}

// The protected-pickup bound is a different fact from the routing bound, and
// Hauling's is one: hauling_handover_challenges is keyed (booking_id, stage)
// with a partial unique index allowing a single ACTIVE row per stage, so a
// mission can hold exactly one live pickup credential however many places it
// collects from. Declaring the routing bound here would let Vendor send a
// protected three-pickup mission whose second handover could never be issued.
func TestHaulingDeclaresOneProtectedPickup(t *testing.T) {
	capability := CapabilityProvider{Options: CapabilityOptions{Enabled: true}}.DeclaredCapability()

	if capability.MaxSecurePickups != 1 {
		t.Fatalf("MaxSecurePickups=%d; Hauling can issue one pickup credential per mission",
			capability.MaxSecurePickups)
	}
	if capability.SecurePickupLimit() != 1 {
		t.Fatalf("SecurePickupLimit()=%d; want 1", capability.SecurePickupLimit())
	}

	// The routing bound is untouched: an unprotected consolidated haul is
	// still serviceable up to the declared maximum.
	if ok, reason := logisticscapability.Match(capability, logisticscapability.Requirement{
		PickupCount: MaxPickupStops, DropoffCount: 1,
		VehicleClass: "TRUCK",
	}); !ok {
		t.Fatalf("an unprotected consolidated haul was refused: %s", reason)
	}
	if ok, reason := logisticscapability.Match(capability, logisticscapability.Requirement{
		PickupCount: 2, DropoffCount: 1, VehicleClass: "TRUCK", SecurePickup: true,
	}); ok || reason != logisticscapability.ReasonSecurePickupLimit {
		t.Fatalf("a protected two-pickup haul was accepted: ok=%v reason=%s", ok, reason)
	}
}
