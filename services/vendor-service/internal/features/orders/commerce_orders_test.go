package orders

import "testing"

func TestCommerceParticipantSecretsAreOpaqueAndScoped(t *testing.T) {
	rawA, hashA, err := newSecret()
	if err != nil {
		t.Fatal(err)
	}
	rawB, hashB, err := newSecret()
	if err != nil {
		t.Fatal(err)
	}
	if rawA == rawB || hashA == hashB {
		t.Fatal("participant secrets must be unguessable and unique")
	}
	if hashA != tokenHash(rawA) || hashB != tokenHash(rawB) {
		t.Fatal("stored token hash must be derived from the raw token")
	}
	if hashA == rawA {
		t.Fatal("raw participant token must not be stored as its hash")
	}
}

func TestCommerceBudgetArithmeticRejectsOverflow(t *testing.T) {
	if got, ok := safeMultiply(3, 7); !ok || got != 21 {
		t.Fatalf("safe multiplication = %d,%v", got, ok)
	}
	if _, ok := safeMultiply(1<<62, 8); ok {
		t.Fatal("overflowing participant total was accepted")
	}
}

func TestSection21DisabledFailsClosed(t *testing.T) {
	s := NewSection21Service(Section21Options{})
	if err := s.available(); err != ErrSection21Disabled {
		t.Fatalf("available()=%v", err)
	}
}

func TestScheduledAcceptanceCapacityStateIsCoherent(t *testing.T) {
	if !capacityStateCoherent("RESERVED", "cres_1") {
		t.Fatal("reserved acceptance must carry a reservation reference")
	}
	if capacityStateCoherent("RESERVED", "") {
		t.Fatal("reserved acceptance without a reservation must fail closed")
	}
	if capacityStateCoherent("CONFLICT", "") {
		t.Fatal("capacity conflict must never be accepted")
	}
	if !capacityStateCoherent("NOT_REQUIRED", "") {
		t.Fatal("non-capacity order may be accepted without a reservation")
	}
}

func TestRFQCheckoutBindingDigestBindsImmutableInputs(t *testing.T) {
	base := rfqCheckoutBindingDigest("rfq_1", "rfq_q_1", 2, "cq_1", "fp_a", "vendor_1", "branch_1", "EUR", 4200, "cres_1")
	if base == rfqCheckoutBindingDigest("rfq_1", "rfq_q_1", 2, "cq_1", "fp_b", "vendor_1", "branch_1", "EUR", 4200, "cres_1") {
		t.Fatal("checkout fingerprint must be bound")
	}
	if base == rfqCheckoutBindingDigest("rfq_1", "rfq_q_2", 2, "cq_1", "fp_a", "vendor_1", "branch_1", "EUR", 4200, "cres_1") {
		t.Fatal("accepted quote identity must be bound")
	}
	if base == rfqCheckoutBindingDigest("rfq_1", "rfq_q_1", 2, "cq_1", "fp_a", "vendor_1", "branch_2", "EUR", 4200, "cres_1") {
		t.Fatal("branch must be bound")
	}
}
