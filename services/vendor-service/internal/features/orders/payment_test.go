package orders

import (
	"errors"
	"testing"
)

func paidOrder() ParentOrder {
	return ParentOrder{
		ID:                         "cpo_1",
		CustomerID:                 "customer_1",
		QuoteID:                    "cq_1",
		QuoteVersion:               1,
		QuoteFingerprint:           "abc",
		Currency:                   "NGN",
		CustomerApprovedTotalMinor: 5000000,
		MaxAuthorizedTotalMinor:    5000000,
		PlatformFeeTaxMinor:        0,
		VendorOrders: []VendorOrder{
			{
				ID: "cvo_a", BusinessID: "biz_a", BranchID: "br_a", QuoteGroupRef: "g_a",
				CustomerPayableMinor: 2000000, VendorPayableMinor: 2000000,
			},
			{
				ID: "cvo_b", BusinessID: "biz_b", BranchID: "br_b", QuoteGroupRef: "g_b",
				CustomerPayableMinor: 1500000, VendorPayableMinor: 1500000,
			},
			{
				ID: "cvo_c", BusinessID: "biz_c", BranchID: "br_c", QuoteGroupRef: "g_c",
				CustomerPayableMinor: 1500000, VendorPayableMinor: 1500000,
			},
		},
	}
}

func TestBindingFor_BindsEveryApprovedCommercialInput(t *testing.T) {
	order := paidOrder()

	binding, err := BindingFor(order)
	if err != nil {
		t.Fatalf("BindingFor: %v", err)
	}
	if binding.QuoteID != "cq_1" || binding.QuoteVersion != 1 ||
		binding.QuoteFingerprint != "abc" || binding.Currency != "NGN" ||
		binding.CustomerID != "customer_1" {
		t.Fatalf("binding lost approved evidence: %+v", binding)
	}
	if binding.QuotedTotalMinor != 5000000 || binding.MaxAuthorizedTotalMinor != 5000000 {
		t.Fatalf("binding totals=%d/%d",
			binding.QuotedTotalMinor, binding.MaxAuthorizedTotalMinor)
	}
	if len(binding.AllocationFingerprint) != 64 {
		t.Fatalf("allocation fingerprint=%q", binding.AllocationFingerprint)
	}
}

// A re-allocated order is a different agreement even when its total is
// unchanged, so moving one vendor's share to another cannot ride an existing
// payment operation.
func TestAllocationFingerprint_ChangesWhenSharesMoveBetweenVendors(t *testing.T) {
	order := paidOrder()
	before, err := AllocationFingerprint(order)
	if err != nil {
		t.Fatal(err)
	}

	// Same customer total, different split.
	order.VendorOrders[0].CustomerPayableMinor = 2500000
	order.VendorOrders[1].CustomerPayableMinor = 1000000

	after, err := AllocationFingerprint(order)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("moving a share between vendors did not change the allocation binding")
	}
}

func TestAllocationFingerprint_IsIndependentOfRowOrder(t *testing.T) {
	order := paidOrder()
	forward, err := AllocationFingerprint(order)
	if err != nil {
		t.Fatal(err)
	}

	reversed := paidOrder()
	reversed.VendorOrders[0], reversed.VendorOrders[2] =
		reversed.VendorOrders[2], reversed.VendorOrders[0]

	backward, err := AllocationFingerprint(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if forward != backward {
		t.Fatal("allocation binding depends on query ordering")
	}
}

// The specification's worked example: one purchase, three vendors, each leg
// carrying only its own vendor's share.
func TestPlanPaymentLegs_OneLegPerVendorOrder(t *testing.T) {
	legs, err := PlanPaymentLegs(paidOrder())
	if err != nil {
		t.Fatalf("PlanPaymentLegs: %v", err)
	}
	if len(legs) != 3 {
		t.Fatalf("legs=%d, want 3", len(legs))
	}

	want := map[string]int64{"cvo_a": 2000000, "cvo_b": 1500000, "cvo_c": 1500000}
	seen := map[string]bool{}
	for _, leg := range legs {
		if leg.Scope != ScopeVendorLeg {
			t.Fatalf("unexpected scope %q", leg.Scope)
		}
		if leg.AmountMinor != want[leg.VendorOrderID] {
			t.Errorf("%s amount=%d, want %d",
				leg.VendorOrderID, leg.AmountMinor, want[leg.VendorOrderID])
		}
		if seen[leg.IdempotencyKey] {
			t.Errorf("duplicate idempotency key %q", leg.IdempotencyKey)
		}
		seen[leg.IdempotencyKey] = true
	}
}

// A retried start must reuse the same keys, so the authority returns the same
// intents rather than a second set.
func TestPlanPaymentLegs_IdempotencyKeysAreDeterministic(t *testing.T) {
	first, err := PlanPaymentLegs(paidOrder())
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanPaymentLegs(paidOrder())
	if err != nil {
		t.Fatal(err)
	}
	for i := range first {
		if first[i].IdempotencyKey != second[i].IdempotencyKey {
			t.Fatalf("key drift: %q vs %q",
				first[i].IdempotencyKey, second[i].IdempotencyKey)
		}
	}
}

func TestPlanPaymentLegs_PlatformFeeGetsItsOwnLeg(t *testing.T) {
	order := paidOrder()
	order.PlatformFeeTaxMinor = 7500
	order.CustomerApprovedTotalMinor += 7500

	legs, err := PlanPaymentLegs(order)
	if err != nil {
		t.Fatalf("PlanPaymentLegs: %v", err)
	}
	if len(legs) != 4 {
		t.Fatalf("legs=%d, want 4", len(legs))
	}

	var platform int
	for _, leg := range legs {
		if leg.Scope != ScopePlatformLeg {
			continue
		}
		platform++
		if leg.AmountMinor != 7500 {
			t.Errorf("platform leg=%d, want 7500", leg.AmountMinor)
		}
		// The platform leg belongs to no vendor, so it must name none.
		if leg.VendorOrderID != "" || leg.BusinessID != "" {
			t.Errorf("platform leg names a vendor: %+v", leg)
		}
	}
	if platform != 1 {
		t.Fatalf("platform legs=%d, want 1", platform)
	}
}

// The price guarantee at its most literal: if the legs do not sum to exactly
// what the customer approved, no payment is started at all.
func TestPlanPaymentLegs_RefusesWhenLegsDoNotSumToTheApprovedTotal(t *testing.T) {
	order := paidOrder()
	order.CustomerApprovedTotalMinor += 1

	if _, err := PlanPaymentLegs(order); !errors.Is(err, ErrAllocationImbalance) {
		t.Fatalf("err=%v, want ErrAllocationImbalance", err)
	}
}

func TestPlanPaymentLegs_ZeroValueAllocationNeedsNoLeg(t *testing.T) {
	order := paidOrder()
	order.CustomerApprovedTotalMinor -= order.VendorOrders[2].CustomerPayableMinor
	order.VendorOrders[2].CustomerPayableMinor = 0

	legs, err := PlanPaymentLegs(order)
	if err != nil {
		t.Fatalf("PlanPaymentLegs: %v", err)
	}
	if len(legs) != 2 {
		t.Fatalf("legs=%d, want 2", len(legs))
	}
	for _, leg := range legs {
		if leg.VendorOrderID == "cvo_c" {
			t.Fatal("a zero-value allocation was given a payment leg")
		}
	}
}

func TestDerivePaymentState(t *testing.T) {
	cases := []struct {
		name string
		legs []PaymentLegRecord
		want PaymentState
	}{
		{"no legs", nil, PaymentPending},
		{
			"all pending",
			[]PaymentLegRecord{{Status: LegPending}, {Status: LegPending}},
			PaymentAuthorizing,
		},
		{
			"partially held is never reported as held",
			[]PaymentLegRecord{{Status: LegHeld}, {Status: LegPending}},
			PaymentAuthorizing,
		},
		{
			"all held",
			[]PaymentLegRecord{{Status: LegHeld}, {Status: LegHeld}},
			PaymentHeld,
		},
		{
			"settled and refunded legs were held first",
			[]PaymentLegRecord{{Status: LegSettled}, {Status: LegRefunded}},
			PaymentHeld,
		},
		{
			"any failed leg unwinds the whole operation",
			[]PaymentLegRecord{{Status: LegHeld}, {Status: LegFailed}},
			PaymentCompensating,
		},
	}

	for _, testCase := range cases {
		if got := DerivePaymentState(testCase.legs); got != testCase.want {
			t.Errorf("%s: state=%q, want %q", testCase.name, got, testCase.want)
		}
	}
}

// Vendor Commerce never treats an unrecognised authority answer as success.
func TestStatusFor_UnknownAuthorityAnswerStaysPending(t *testing.T) {
	cases := map[string]LegStatus{
		"held":          LegHeld,
		"HELD":          LegHeld,
		"completed":     LegSettled,
		"refunded":      LegRefunded,
		"part_refunded": LegRefunded,
		"failed":        LegFailed,
		"cancelled":     LegFailed,
		"":              LegPending,
		"processing":    LegPending,
		"success":       LegPending,
		"ok":            LegPending,
	}

	for status, want := range cases {
		if got := StatusFor(LegResult{Status: status}); got != want {
			t.Errorf("StatusFor(%q)=%q, want %q", status, got, want)
		}
	}
}

func TestPaymentBinding_MatchesIsExact(t *testing.T) {
	base, err := BindingFor(paidOrder())
	if err != nil {
		t.Fatal(err)
	}

	if !base.Matches(base) {
		t.Fatal("a binding does not match itself")
	}

	// Every field is material. A change in any one is a different agreement.
	for name, mutate := range map[string]func(*PaymentBinding){
		"quote":       func(b *PaymentBinding) { b.QuoteID = "cq_other" },
		"version":     func(b *PaymentBinding) { b.QuoteVersion = 2 },
		"fingerprint": func(b *PaymentBinding) { b.QuoteFingerprint = "def" },
		"total":       func(b *PaymentBinding) { b.QuotedTotalMinor = 1 },
		"max":         func(b *PaymentBinding) { b.MaxAuthorizedTotalMinor = 1 },
		"currency":    func(b *PaymentBinding) { b.Currency = "USD" },
		"customer":    func(b *PaymentBinding) { b.CustomerID = "customer_2" },
		"allocation":  func(b *PaymentBinding) { b.AllocationFingerprint = "0" },
	} {
		altered := base
		mutate(&altered)
		if base.Matches(altered) {
			t.Errorf("a changed %s still matched the original binding", name)
		}
	}
}
