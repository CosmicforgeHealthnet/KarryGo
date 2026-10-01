package commercequote

// Dispatch's declared capability, and the matcher's use of it.
//
// Dispatch is the authority Vendor falls back on for consolidated orders, so
// its multi-pickup declaration is the one that decides whether a three-pickup
// commerce order can be fulfilled at all. Declaring it from the same constants
// the motorcycle capability already uses -- rather than from a second set --
// is what stops the two drifting apart.

import (
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/logisticscapability"
)

func TestDispatchDeclaresItsRealMultiPickupCapability(t *testing.T) {
	capability := DeclaredLogisticsCapability(LogisticsCapabilityOptions{
		Enabled: true, CorporateProviderSupported: true,
		Now: func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	})

	if capability.Authority != logisticscapability.AuthorityDispatch {
		t.Fatalf("authority=%q", capability.Authority)
	}
	if !capability.MultiPickupSupported || capability.MaxPickups < 2 {
		t.Fatalf("Dispatch consolidates collections and must declare it, got multi=%v max=%d",
			capability.MultiPickupSupported, capability.MaxPickups)
	}
	if capability.MaxDropoffs != 1 {
		t.Fatalf("a commerce mission has one delivery, got %d", capability.MaxDropoffs)
	}
	if capability.MaxWeightGrams <= 0 || capability.MaxVolumeMl <= 0 {
		t.Fatal("a motorcycle has real physical limits; leaving them undeclared reads as unknown and would let anything through")
	}
	if !capability.SecurePickupSupported || !capability.SecureDeliverySupported {
		t.Fatal("Dispatch implements protected handover and must say so")
	}
}

func TestDisabledDispatchIntegrationDeclaresItselfUnavailable(t *testing.T) {
	capability := DeclaredLogisticsCapability(LogisticsCapabilityOptions{Enabled: false})
	if capability.Available {
		t.Fatal("a disabled integration must not declare itself available")
	}
	if capability.UnavailableReason == "" {
		t.Fatal("an unavailable authority must say why, or an operator cannot tell an outage from a switch")
	}
	answer := logisticscapability.Evaluate(capability, logisticscapability.Requirement{
		PickupCount: 1, DropoffCount: 1,
	})
	if answer.Serviceable {
		t.Fatal("an unavailable authority is never serviceable")
	}
}

func TestDispatchRefusesLoadsBeyondItsDeclaredEnvelope(t *testing.T) {
	capability := DeclaredLogisticsCapability(LogisticsCapabilityOptions{Enabled: true})

	tooHeavy := logisticscapability.Evaluate(capability, logisticscapability.Requirement{
		PickupCount: 1, DropoffCount: 1, WeightGrams: capability.MaxWeightGrams + 1,
	})
	if tooHeavy.Serviceable || tooHeavy.ReasonCode != logisticscapability.ReasonWeightExceeded {
		t.Fatalf("an overweight load must be refused with the weight reason: %+v", tooHeavy)
	}

	tooManyStops := logisticscapability.Evaluate(capability, logisticscapability.Requirement{
		PickupCount: capability.MaxPickups + 1, DropoffCount: 1,
	})
	if tooManyStops.Serviceable || tooManyStops.ReasonCode != logisticscapability.ReasonTooManyPickups {
		t.Fatalf("more pickups than declared must be refused: %+v", tooManyStops)
	}

	withinEnvelope := logisticscapability.Evaluate(capability, logisticscapability.Requirement{
		PickupCount: 3, DropoffCount: 1, WeightGrams: 5000, PackageCount: 4,
		SecurePickup: true, SecureDelivery: true,
	})
	if !withinEnvelope.Serviceable {
		t.Fatalf("a consolidated order inside every declared limit must be serviceable: %+v", withinEnvelope)
	}
}

// Dispatch protects every pickup it routes, because its handover challenges
// are keyed per mission AND stop (dispatch_handover_challenges has a partial
// unique index on mission_ref, stop_ref, stage). This is the capability that
// makes a consolidated protected run possible at all, so it is asserted
// rather than left implied.
func TestDispatchProtectsEveryPickupItRoutes(t *testing.T) {
	capability := DeclaredLogisticsCapability(LogisticsCapabilityOptions{Enabled: true})

	if capability.MaxSecurePickups != capability.MaxPickups {
		t.Fatalf("MaxSecurePickups=%d MaxPickups=%d; Dispatch issues a credential per stop",
			capability.MaxSecurePickups, capability.MaxPickups)
	}
	if ok, reason := logisticscapability.Match(capability, logisticscapability.Requirement{
		PickupCount: capability.MaxPickups, DropoffCount: 1,
		VehicleClass: "MOTORCYCLE", SecurePickup: true, SecureDelivery: true,
	}); !ok {
		t.Fatalf("a fully protected consolidated run was refused: %s", reason)
	}
}
