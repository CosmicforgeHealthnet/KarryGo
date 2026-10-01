package goods

import (
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/custodychain"
)

// Taxi Goods runs both directions on one credential type, distinguished only
// by Stage. Stamping the direction means a client no longer has to know that
// mapping — and cannot get it wrong.
func TestTaxiGoodsCredentialDescribesBothDirections(t *testing.T) {
	expires := time.Now().Add(15 * time.Minute).UTC()

	pickup := Credential{ChallengeRef: "ch_1", Stage: StagePickup, ExpiresAt: expires}.
		WithDirection("mis_1")
	if pickup.Direction.Purpose != custodychain.PurposeCommercePickup ||
		pickup.Direction.Holder != custodychain.RoleProvider ||
		pickup.Direction.Verifier != custodychain.RoleVendorSender {
		t.Fatalf("pickup direction = %+v; want provider-held, vendor-verified", pickup.Direction)
	}

	delivery := Credential{ChallengeRef: "ch_2", Stage: StageDelivery, ExpiresAt: expires}.
		WithDirection("mis_1")
	if delivery.Direction.Purpose != custodychain.PurposeDelivery ||
		delivery.Direction.Holder != custodychain.RoleReceiver ||
		delivery.Direction.Verifier != custodychain.RoleProvider {
		t.Fatalf("delivery direction = %+v; want receiver-held, provider-verified", delivery.Direction)
	}

	for _, credential := range []Credential{pickup, delivery} {
		if credential.Direction.Authority != "TAXI" {
			t.Fatalf("authority = %q", credential.Direction.Authority)
		}
		if !credential.Direction.Consistent() {
			t.Fatalf("%s direction disagrees with the canonical matrix", credential.Stage)
		}
	}
}

// Taxi Goods is Commerce work by definition — Vendor Service creates every
// mission through the internal commerce-mission contract. Direct Taxi pickup,
// where the passenger holds the credential and the driver verifies it, lives
// on the passenger booking surface and must not resolve through this package.
func TestTaxiGoodsDoesNotResolveDirectTaxiPickup(t *testing.T) {
	if purpose, ok := PurposeForStage("RIDE"); ok {
		t.Fatalf("an unknown stage resolved to %q", purpose)
	}
	purpose, ok := PurposeForStage(StagePickup)
	if !ok || purpose == custodychain.PurposeDirectPickup {
		t.Fatalf("the Taxi Goods pickup resolved to the direct-taxi contract: %q", purpose)
	}
}
