package dispatchcommerce

import (
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/custodychain"
)

// A Commerce pickup credential is provider-held and vendor-verified, and the
// payload has to say so. Before CFC-008 the only way to know was which route
// the credential arrived on, which stops being true the moment a route is
// re-mounted or proxied.
func TestPickupCredentialDescribesItsOwnDirection(t *testing.T) {
	expires := time.Now().Add(15 * time.Minute).UTC()
	credential := PickupCredential{
		MissionRef: "mis_1", StopRef: "stp_1", Code: "123456",
		ExpiresAt: expires, AttemptsRemaining: 5,
	}.WithDirection()

	if credential.Direction.Purpose != custodychain.PurposeCommercePickup {
		t.Fatalf("purpose = %q", credential.Direction.Purpose)
	}
	if credential.Direction.Holder != custodychain.RoleProvider {
		t.Fatalf("holder = %q; the rider carries the Commerce pickup credential",
			credential.Direction.Holder)
	}
	if credential.Direction.Verifier != custodychain.RoleVendorSender {
		t.Fatalf("verifier = %q; the merchant verifies it before releasing the goods",
			credential.Direction.Verifier)
	}
	if credential.Direction.Authority != "DISPATCH" {
		t.Fatalf("authority = %q", credential.Direction.Authority)
	}
	if credential.Direction.MissionID != "mis_1" {
		t.Fatalf("missionId = %q; an unscoped credential is not scoped to anything",
			credential.Direction.MissionID)
	}
	if !credential.Direction.Consistent() {
		t.Fatal("the stamped direction disagrees with the canonical matrix")
	}
	if credential.Direction.ExpiresAt == nil || !credential.Direction.ExpiresAt.Equal(expires) {
		t.Fatal("the expiry did not travel with the description")
	}
	if credential.Direction.Expired(expires.Add(-time.Minute)) {
		t.Fatal("a live credential reported itself expired")
	}
	if !credential.Direction.Expired(expires.Add(time.Minute)) {
		t.Fatal("a lapsed credential did not report itself expired")
	}
}
