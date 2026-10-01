package handover

import (
	"testing"

	"cosmicforge/logistics/shared/go/custodychain"
)

func observationFor(authority, stage string, sent custodychain.Direction) (Observation, error) {
	binding := Binding{Authority: authority, MissionRef: "mis_1", FulfilmentRef: "ful_1"}
	return validateObservation(Observation{
		Binding: binding, HandoverRef: "hnd_1", Stage: stage,
		Status: "ACTIVE", SourceVersion: 1, Direction: sent,
	}, binding, stage)
}

// Vendor derives the direction from the frozen binding rather than believing
// what the authority sent. An authority claiming that a Commerce pickup is
// provider-verified would be telling Vendor it is not the verifier of its own
// merchant's goods — which is the one claim Vendor must never accept.
func TestVendorDerivesTheDirectionRatherThanTrustingIt(t *testing.T) {
	inverted := custodychain.Direction{
		Purpose:  custodychain.PurposeCommercePickup,
		Holder:   custodychain.RoleVendorSender,
		Verifier: custodychain.RoleProvider,
	}
	observed, err := observationFor("DISPATCH", "PICKUP", inverted)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if observed.Direction.Holder != custodychain.RoleProvider ||
		observed.Direction.Verifier != custodychain.RoleVendorSender {
		t.Fatalf("Vendor adopted the authority's direction: %+v", observed.Direction)
	}
	if !observed.Direction.Consistent() {
		t.Fatal("the derived direction disagrees with the canonical matrix")
	}
}

func TestEveryPlatformAuthorityDescribesTheSameDirections(t *testing.T) {
	for _, authority := range []string{"DISPATCH", "TAXI", "HAULING"} {
		pickup, err := observationFor(authority, "PICKUP", custodychain.Direction{})
		if err != nil {
			t.Fatalf("%s pickup: %v", authority, err)
		}
		if pickup.Direction.Purpose != custodychain.PurposeCommercePickup ||
			pickup.Direction.Authority != authority {
			t.Fatalf("%s pickup direction = %+v", authority, pickup.Direction)
		}

		delivery, err := observationFor(authority, "DELIVERY", custodychain.Direction{})
		if err != nil {
			t.Fatalf("%s delivery: %v", authority, err)
		}
		if delivery.Direction.Purpose != custodychain.PurposeDelivery ||
			delivery.Direction.Holder != custodychain.RoleReceiver ||
			delivery.Direction.Verifier != custodychain.RoleProvider {
			t.Fatalf("%s delivery direction = %+v", authority, delivery.Direction)
		}
	}
}

// Vendor-owned delivery has no platform credential: the vendor's own staff
// carry the goods, so describing a holder and a verifier would be inventing
// parties that do not exist in that flow.
func TestVendorOwnDeliveryHasNoCredentialDirection(t *testing.T) {
	observed, err := observationFor("VENDOR_OWN_DELIVERY", "PICKUP", custodychain.Direction{})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if observed.Direction.Purpose != "" || observed.Direction.Holder != "" {
		t.Fatalf("a vendor-owned handover claimed a platform credential: %+v", observed.Direction)
	}
}
