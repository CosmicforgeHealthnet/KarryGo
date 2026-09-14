package custodychain

import "testing"

// The single most important property in this package: nobody approves
// themselves. A descriptor whose holder and verifier are the same role means
// one party generates a credential and then accepts it, which is not a
// handover at all. Provider transfer is the one exception, and it is an
// exception only because the two parties are two different providers.
func TestNoPurposeLetsOnePartyApproveItself(t *testing.T) {
	for _, purpose := range Purposes() {
		descriptor, ok := DescriptorFor(purpose)
		if !ok {
			t.Fatalf("%s is listed by Purposes but has no descriptor", purpose)
		}
		if descriptor.SameRoleBothSides() && purpose != PurposeProviderTransfer {
			t.Fatalf("%s has holder==verifier==%s: that party approves itself",
				purpose, descriptor.Holder)
		}
	}
}

// Commerce and direct pickup are mirror images, and swapping them is a real
// security failure rather than a naming slip: the Commerce direction on
// direct work lets a driver approve their own arrival, and the direct
// direction on Commerce work lets any provider who turns up collect a
// merchant's goods.
func TestCommerceAndDirectPickupPointOppositeWays(t *testing.T) {
	commerce, _ := DescriptorFor(PurposeCommercePickup)
	direct, _ := DescriptorFor(PurposeDirectPickup)

	if !commerce.HeldBy(RoleProvider) || !commerce.VerifiedBy(RoleVendorSender) {
		t.Fatalf("commerce pickup must be provider-held and vendor-verified, got holder=%s verifier=%s",
			commerce.Holder, commerce.Verifier)
	}
	if !direct.HeldBy(RoleCustomerSender) || !direct.VerifiedBy(RoleProvider) {
		t.Fatalf("direct pickup must be sender-held and provider-verified, got holder=%s verifier=%s",
			direct.Holder, direct.Verifier)
	}
	if commerce.Holder == direct.Holder || commerce.Verifier == direct.Verifier {
		t.Fatal("commerce and direct pickup have collapsed into the same direction")
	}
}

func TestDeliveryCredentialIsReceiverHeld(t *testing.T) {
	delivery, ok := DescriptorFor(PurposeDelivery)
	if !ok {
		t.Fatal("delivery has no descriptor")
	}
	if !delivery.HeldBy(RoleReceiver) {
		t.Fatalf("the receiver must hold the delivery credential, got %s", delivery.Holder)
	}
	if !delivery.VerifiedBy(RoleProvider) {
		t.Fatalf("the provider must verify the delivery credential, got %s", delivery.Verifier)
	}
}

// A vendor must never be able to verify a direct-taxi pickup, and a customer
// must never be able to verify a Commerce one. Stated as a table so a new
// purpose cannot be added without deciding this question for it.
func TestVerifierIsExclusivePerPurpose(t *testing.T) {
	forbidden := map[CredentialPurpose][]PartyRole{
		PurposeCommercePickup: {RoleProvider, RoleCustomerSender, RoleReceiver},
		PurposeDirectPickup:   {RoleVendorSender, RoleCustomerSender, RoleReceiver},
		// A customer session must never be able to verify a merchant's
		// mission, which is why this is a separate purpose from Commerce
		// pickup rather than a reuse of it.
		PurposeSenderVerifiedPickup: {RoleVendorSender, RoleProvider, RoleReceiver},
		PurposeDelivery:             {RoleVendorSender, RoleCustomerSender, RoleReceiver},
		PurposeReturn:               {RoleProvider, RoleCustomerSender, RoleReceiver},
		PurposeProviderTransfer:     {RoleVendorSender, RoleCustomerSender, RoleReceiver},
	}
	for purpose, roles := range forbidden {
		descriptor, ok := DescriptorFor(purpose)
		if !ok {
			t.Fatalf("%s has no descriptor", purpose)
		}
		for _, role := range roles {
			if descriptor.VerifiedBy(role) {
				t.Fatalf("%s must not be verifiable by %s", purpose, role)
			}
		}
	}
	if len(forbidden) != len(Purposes()) {
		t.Fatalf("a purpose was added without deciding who may not verify it: %d rules for %d purposes",
			len(forbidden), len(Purposes()))
	}
}

func TestEveryPurposeConsumesItsCredential(t *testing.T) {
	for _, purpose := range Purposes() {
		descriptor, _ := DescriptorFor(purpose)
		if !descriptor.SingleUse {
			t.Fatalf("%s is reusable; a replayed credential would move custody twice", purpose)
		}
	}
}

func TestUnknownPurposeIsRefusedRatherThanZeroValued(t *testing.T) {
	if _, ok := DescriptorFor("PICKUP"); ok {
		t.Fatal("a purpose this platform does not define was resolved")
	}
	if _, ok := ParsePurpose("  commerce_pickup "); !ok {
		t.Fatal("a known purpose in the wrong case was refused")
	}
	if _, ok := ParsePurpose("handover"); ok {
		t.Fatal("an unknown purpose parsed")
	}
}

func TestPurposeForSeparatesCommerceFromDirect(t *testing.T) {
	commerce, ok := PurposeFor(OperationPickup, true)
	if !ok || commerce != PurposeCommercePickup {
		t.Fatalf("a vendor-sender pickup resolved to %q", commerce)
	}
	direct, ok := PurposeFor(OperationPickup, false)
	if !ok || direct != PurposeDirectPickup {
		t.Fatalf("an individual-sender pickup resolved to %q", direct)
	}
	// Delivery has one direction regardless of who sent the goods: the
	// receiver always holds the credential.
	vendorDelivery, _ := PurposeFor(OperationDelivery, true)
	directDelivery, _ := PurposeFor(OperationDelivery, false)
	if vendorDelivery != directDelivery || vendorDelivery != PurposeDelivery {
		t.Fatalf("delivery direction depends on the sender: %q vs %q",
			vendorDelivery, directDelivery)
	}
	if _, ok := PurposeFor(OperationPreparation, true); ok {
		t.Fatal("preparation resolved to a credential purpose; nothing changes hands")
	}
}

// This platform runs three pickup directions, not the two the section names.
// Commerce is provider-held and vendor-verified; Taxi direct is customer-held
// and provider-verified; Dispatch and Hauling direct parcel work is
// provider-held and customer-verified. All three are asserted together so a
// future change cannot quietly merge two of them.
func TestThreePickupDirectionsStayDistinct(t *testing.T) {
	commerce, _ := DescriptorFor(PurposeCommercePickup)
	taxiDirect, _ := DescriptorFor(PurposeDirectPickup)
	senderVerified, _ := DescriptorFor(PurposeSenderVerifiedPickup)

	if !senderVerified.HeldBy(RoleProvider) || !senderVerified.VerifiedBy(RoleCustomerSender) {
		t.Fatalf("sender-verified pickup must be provider-held and customer-verified, got holder=%s verifier=%s",
			senderVerified.Holder, senderVerified.Verifier)
	}
	// It shares the holder with Commerce but not the verifier: the whole
	// reason it is a separate purpose is that the verifying party
	// authenticates differently.
	if senderVerified.Holder != commerce.Holder {
		t.Fatal("both provider-presented pickups must be provider-held")
	}
	if senderVerified.Verifier == commerce.Verifier {
		t.Fatal("a customer session can now verify a merchant mission")
	}
	// And it is the exact reverse of the Taxi direct contract.
	if senderVerified.Holder != taxiDirect.Verifier || senderVerified.Verifier != taxiDirect.Holder {
		t.Fatalf("the two direct pickups are no longer mirror images: %+v vs %+v",
			senderVerified, taxiDirect)
	}
}
