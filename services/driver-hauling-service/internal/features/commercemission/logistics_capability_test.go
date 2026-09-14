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
	within := logisticscapability.Evaluate(capability, logisticscapability.Requirement{
		PickupCount: MaxPickupStops, DropoffCount: 1,
		SecurePickup: true, SecureDelivery: true,
	})
	if !within.Serviceable {
		t.Fatalf("a mission at the declared limit must be serviceable: %+v", within)
	}
}
