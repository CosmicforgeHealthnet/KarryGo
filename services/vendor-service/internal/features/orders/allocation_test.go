package orders

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// buildSource assembles an approved quote whose totals already satisfy Section
// 17D's own arithmetic, so a test failure means Section 17E's allocation is
// wrong rather than the fixture.
func buildSource(groups []GroupSource, lines []LineSource, adjustments []AdjustmentSource,
	fee, tax int64) QuoteSource {
	var product, maxProduct, delivery, deliveryVendor, deliveryPlatform int64
	for _, group := range groups {
		product += group.ProductSubtotalMinor
		maxProduct += group.MaxProductSubtotalMinor
		delivery += group.DeliveryGrossMinor
		deliveryVendor += group.DeliveryVendorFundedMinor
		deliveryPlatform += group.DeliveryPlatformFundedMinor
	}

	subsidy := deliveryVendor + deliveryPlatform
	discount := subsidy
	for _, adjustment := range adjustments {
		if adjustment.Type == AdjustmentDiscount {
			discount += adjustment.AmountMinor
		}
	}

	gross := product + delivery + fee + tax
	maxGross := maxProduct + delivery + fee + tax

	return QuoteSource{
		QuoteID:                    "cq_test",
		QuoteVersion:               1,
		QuoteFingerprint:           "f",
		ApprovedAt:                 time.Unix(0, 0).UTC(),
		ExpiresAt:                  time.Unix(3600, 0).UTC(),
		CustomerID:                 "customer",
		CheckoutSessionID:          "session",
		CheckoutValidationRevision: 1,
		CartID:                     "cart",
		CartVersion:                1,
		FulfilmentPlanID:           "plan",
		FulfilmentPlanRevision:     1,
		FulfilmentPlanVersion:      1,
		Domain:                     "SHOP",
		Currency:                   "NGN",
		SelectedFulfilmentMode:     "SINGLE",
		ProductSubtotalMinor:       product,
		MaxProductSubtotalMinor:    maxProduct,
		DeliverySubtotalMinor:      delivery,
		FeeSubtotalMinor:           fee,
		TaxSubtotalMinor:           tax,
		DiscountTotalMinor:         discount,
		SubsidyTotalMinor:          subsidy,
		FinalPayableMinor:          gross - discount,
		MaxAuthorizedTotalMinor:    maxGross - discount,
		FundingBreakdown:           json.RawMessage(`{}`),
		PriceDisclosureSnapshot:    json.RawMessage(`{}`),
		Groups:                     groups,
		Lines:                      lines,
		Adjustments:                adjustments,
	}
}

func shopGroup(ref, business, branch string, product, deliveryGross, deliveryCustomer,
	deliveryVendor, deliveryPlatform int64, responsibility DeliveryResponsibility) GroupSource {
	return GroupSource{
		QuoteGroupRef:               ref,
		BusinessID:                  business,
		BranchID:                    branch,
		Domain:                      "SHOP",
		ProductSubtotalMinor:        product,
		MaxProductSubtotalMinor:     product,
		ReservationRequired:         true,
		ReservationRef:              "res_" + ref,
		ReservationSourceRef:        "src_" + ref,
		LogisticsPlanGroupRef:       "pg_" + ref,
		LogisticsMissionRef:         "mission_" + ref,
		DeliveryResponsibility:      responsibility,
		DeliveryGrossMinor:          deliveryGross,
		DeliveryCustomerMinor:       deliveryCustomer,
		DeliveryVendorFundedMinor:   deliveryVendor,
		DeliveryPlatformFundedMinor: deliveryPlatform,
	}
}

func shopLine(ref, groupRef, cartItem string, total int64) LineSource {
	return LineSource{
		QuoteLineRef:            ref,
		QuoteGroupRef:           groupRef,
		CartItemID:              cartItem,
		ItemName:                "Item " + cartItem,
		ItemRef:                 "item_" + cartItem,
		Domain:                  "SHOP",
		Quantity:                1,
		UnitPriceMinor:          total,
		QuotedTotalMinor:        total,
		MaxAuthorizedTotalMinor: total,
		PricingBasis:            "FIXED_UNIT",
		PriceReference:          "online-price-v1",
		CatalogueRevision:       1,
		OnlinePriceDisclosure:   "VENDOR_ONLINE_PRICE",
	}
}

// The specification's own worked example: one ₦50,000 payment split across
// three vendors, each receiving only its own allocation.
func TestComputeAllocation_MultiVendorIsolation(t *testing.T) {
	groups := []GroupSource{
		shopGroup("g_a", "biz_a", "branch_a", 2000000, 0, 0, 0, 0, DeliveryNone),
		shopGroup("g_b", "biz_b", "branch_b", 1500000, 0, 0, 0, 0, DeliveryNone),
		shopGroup("g_c", "biz_c", "branch_c", 1000000, 0, 0, 0, 0, DeliveryNone),
	}
	lines := []LineSource{
		shopLine("l_a", "g_a", "ci_a", 2000000),
		shopLine("l_b", "g_b", "ci_b", 1500000),
		shopLine("l_c", "g_c", "ci_c", 1000000),
	}
	source := buildSource(groups, lines, nil, 0, 0)

	plan, err := ComputeAllocation(source)
	if err != nil {
		t.Fatalf("ComputeAllocation: %v", err)
	}
	if len(plan.Vendors) != 3 {
		t.Fatalf("vendors=%d, want 3", len(plan.Vendors))
	}

	want := map[string]int64{"biz_a": 2000000, "biz_b": 1500000, "biz_c": 1000000}
	for _, vendor := range plan.Vendors {
		if vendor.CustomerPayableMinor != want[vendor.BusinessID] {
			t.Errorf("%s customer payable=%d, want %d",
				vendor.BusinessID, vendor.CustomerPayableMinor, want[vendor.BusinessID])
		}
		if vendor.VendorPayableMinor != want[vendor.BusinessID] {
			t.Errorf("%s vendor payable=%d, want %d",
				vendor.BusinessID, vendor.VendorPayableMinor, want[vendor.BusinessID])
		}
		// No vendor allocation may name another vendor as its beneficiary.
		for _, allocation := range vendor.Allocations {
			if allocation.BeneficiaryType == BeneficiaryTypeVendor &&
				allocation.BeneficiaryRef != vendor.BusinessID {
				t.Errorf("%s allocation names foreign vendor %s",
					vendor.BusinessID, allocation.BeneficiaryRef)
			}
		}
	}
}

func TestComputeAllocation_DeliveryFundingStaysAttributed(t *testing.T) {
	groups := []GroupSource{
		// Platform-executed: the provider is owed the gross fee and the vendor
		// is owed nothing of it, even though the vendor subsidised part.
		shopGroup("g_a", "biz_a", "branch_a", 100000, 50000, 20000, 30000, 0, DeliveryPlatform),
		// Vendor-executed: the same vendor performs the delivery, so the fee
		// lands on the vendor payable additively.
		shopGroup("g_b", "biz_b", "branch_b", 200000, 70000, 50000, 0, 20000, DeliveryVendorOwn),
	}
	lines := []LineSource{
		shopLine("l_a", "g_a", "ci_a", 100000),
		shopLine("l_b", "g_b", "ci_b", 200000),
	}

	plan, err := ComputeAllocation(buildSource(groups, lines, nil, 0, 0))
	if err != nil {
		t.Fatalf("ComputeAllocation: %v", err)
	}

	byBusiness := map[string]VendorAllocation{}
	for _, vendor := range plan.Vendors {
		byBusiness[vendor.BusinessID] = vendor
	}

	platform := byBusiness["biz_a"]
	if platform.DeliveryFeeBeneficiary != BeneficiaryPlatformProvider {
		t.Errorf("platform beneficiary=%q", platform.DeliveryFeeBeneficiary)
	}
	// Product 100000 minus this vendor's own 30000 delivery subsidy.
	if platform.VendorPayableMinor != 70000 {
		t.Errorf("platform-delivered vendor payable=%d, want 70000", platform.VendorPayableMinor)
	}
	if platform.CustomerPayableMinor != 120000 {
		t.Errorf("platform-delivered customer payable=%d, want 120000", platform.CustomerPayableMinor)
	}

	vendorOwn := byBusiness["biz_b"]
	if vendorOwn.DeliveryFeeBeneficiary != BeneficiaryVendor {
		t.Errorf("vendor-own beneficiary=%q", vendorOwn.DeliveryFeeBeneficiary)
	}
	// Product 200000 plus the full 70000 delivery it performed itself.
	if vendorOwn.VendorPayableMinor != 270000 {
		t.Errorf("vendor-own payable=%d, want 270000", vendorOwn.VendorPayableMinor)
	}
	if vendorOwn.CustomerPayableMinor != 250000 {
		t.Errorf("vendor-own customer payable=%d, want 250000", vendorOwn.CustomerPayableMinor)
	}

	// Every delivery allocation must point at the party that executed it.
	for _, vendor := range plan.Vendors {
		for _, allocation := range vendor.Allocations {
			if allocation.AllocationType != AllocationDelivery {
				continue
			}
			if vendor.BusinessID == "biz_a" &&
				allocation.BeneficiaryType != BeneficiaryTypePlatformProvide {
				t.Errorf("platform delivery paid to %q", allocation.BeneficiaryType)
			}
			if vendor.BusinessID == "biz_b" && allocation.BeneficiaryRef != "biz_b" {
				t.Errorf("vendor-own delivery paid to %q", allocation.BeneficiaryRef)
			}
		}
	}
}

func TestComputeAllocation_VendorFundedDiscountIsNotSpreadAcrossVendors(t *testing.T) {
	groups := []GroupSource{
		shopGroup("g_a", "biz_a", "branch_a", 100000, 0, 0, 0, 0, DeliveryNone),
		shopGroup("g_b", "biz_b", "branch_b", 100000, 0, 0, 0, 0, DeliveryNone),
	}
	lines := []LineSource{
		shopLine("l_a", "g_a", "ci_a", 100000),
		shopLine("l_b", "g_b", "ci_b", 100000),
	}
	adjustments := []AdjustmentSource{{
		AdjustmentID:  "adj_1",
		Type:          AdjustmentDiscount,
		Scope:         ScopeProduct,
		FundingSource: FundingVendor,
		SourceRef:     "promo_1",
		AmountMinor:   10000,
	}}

	// Splitting it would make vendor B pay for vendor A's promotion, so the
	// whole order is refused instead.
	_, err := ComputeAllocation(buildSource(groups, lines, adjustments, 0, 0))
	if !errors.Is(err, ErrNotAttributable) {
		t.Fatalf("err=%v, want ErrNotAttributable", err)
	}
}

func TestComputeAllocation_SingleVendorFundedDiscountAttributes(t *testing.T) {
	groups := []GroupSource{
		shopGroup("g_a", "biz_a", "branch_a", 100000, 0, 0, 0, 0, DeliveryNone),
	}
	lines := []LineSource{shopLine("l_a", "g_a", "ci_a", 100000)}
	adjustments := []AdjustmentSource{{
		AdjustmentID:  "adj_1",
		Type:          AdjustmentDiscount,
		Scope:         ScopeProduct,
		FundingSource: FundingVendor,
		SourceRef:     "promo_1",
		AmountMinor:   10000,
	}}

	plan, err := ComputeAllocation(buildSource(groups, lines, adjustments, 0, 0))
	if err != nil {
		t.Fatalf("ComputeAllocation: %v", err)
	}
	vendor := plan.Vendors[0]
	if vendor.ProductVendorFundedMinor != 10000 {
		t.Errorf("vendor funded=%d, want 10000", vendor.ProductVendorFundedMinor)
	}
	// The vendor funded its own promotion, so its payable drops by exactly it.
	if vendor.VendorPayableMinor != 90000 {
		t.Errorf("vendor payable=%d, want 90000", vendor.VendorPayableMinor)
	}
	if vendor.CustomerPayableMinor != 90000 {
		t.Errorf("customer payable=%d, want 90000", vendor.CustomerPayableMinor)
	}
	if vendor.LineDiscounts["ci_a"] != 10000 {
		t.Errorf("line discount=%d, want 10000", vendor.LineDiscounts["ci_a"])
	}
}

func TestComputeAllocation_PlatformFundedDiscountCostsNoVendor(t *testing.T) {
	groups := []GroupSource{
		shopGroup("g_a", "biz_a", "branch_a", 100000, 0, 0, 0, 0, DeliveryNone),
		shopGroup("g_b", "biz_b", "branch_b", 300000, 0, 0, 0, 0, DeliveryNone),
	}
	lines := []LineSource{
		shopLine("l_a", "g_a", "ci_a", 100000),
		shopLine("l_b", "g_b", "ci_b", 300000),
	}
	adjustments := []AdjustmentSource{{
		AdjustmentID:  "adj_1",
		Type:          AdjustmentDiscount,
		Scope:         ScopeOrder,
		FundingSource: FundingPlatform,
		SourceRef:     "promo_platform",
		AmountMinor:   40000,
	}}

	plan, err := ComputeAllocation(buildSource(groups, lines, adjustments, 0, 0))
	if err != nil {
		t.Fatalf("ComputeAllocation: %v", err)
	}

	for _, vendor := range plan.Vendors {
		// The platform absorbed the benefit: no vendor's payable moved.
		if vendor.VendorPayableMinor != vendor.ProductSubtotalMinor {
			t.Errorf("%s payable=%d, want %d untouched",
				vendor.BusinessID, vendor.VendorPayableMinor, vendor.ProductSubtotalMinor)
		}
	}
	// Weighted 1:3, so 10000 and 30000.
	byBusiness := map[string]int64{}
	for _, vendor := range plan.Vendors {
		byBusiness[vendor.BusinessID] = vendor.OrderPlatformFundedMinor
	}
	if byBusiness["biz_a"] != 10000 || byBusiness["biz_b"] != 30000 {
		t.Errorf("platform split=%v, want a=10000 b=30000", byBusiness)
	}
}

func TestComputeAllocation_FeeAndTaxStayParentScope(t *testing.T) {
	groups := []GroupSource{
		shopGroup("g_a", "biz_a", "branch_a", 100000, 0, 0, 0, 0, DeliveryNone),
	}
	lines := []LineSource{shopLine("l_a", "g_a", "ci_a", 100000)}

	plan, err := ComputeAllocation(buildSource(groups, lines, nil, 5000, 2500))
	if err != nil {
		t.Fatalf("ComputeAllocation: %v", err)
	}
	if plan.PlatformFeeTaxMinor != 7500 {
		t.Errorf("platform fee+tax=%d, want 7500", plan.PlatformFeeTaxMinor)
	}
	if len(plan.ParentAllocations) != 2 {
		t.Fatalf("parent allocations=%d, want 2", len(plan.ParentAllocations))
	}
	for _, allocation := range plan.ParentAllocations {
		if allocation.BeneficiaryType != BeneficiaryTypePlatform {
			t.Errorf("fee/tax beneficiary=%q, want PLATFORM", allocation.BeneficiaryType)
		}
	}
	// The vendor is never charged the platform's own fee.
	if plan.Vendors[0].VendorPayableMinor != 100000 {
		t.Errorf("vendor payable=%d, want 100000", plan.Vendors[0].VendorPayableMinor)
	}
}

// A remainder that does not divide evenly must still reconcile exactly.
func TestDistribute_LargestRemainderIsExactAndDeterministic(t *testing.T) {
	shares, err := distribute(100, []int64{1, 1, 1})
	if err != nil {
		t.Fatalf("distribute: %v", err)
	}
	var total int64
	for _, share := range shares {
		total += share
	}
	if total != 100 {
		t.Fatalf("total=%d, want 100", total)
	}
	if shares[0] != 34 || shares[1] != 33 || shares[2] != 33 {
		t.Errorf("shares=%v, want [34 33 33]", shares)
	}

	repeat, err := distribute(100, []int64{1, 1, 1})
	if err != nil {
		t.Fatalf("distribute: %v", err)
	}
	for i := range shares {
		if shares[i] != repeat[i] {
			t.Fatalf("distribution is not deterministic: %v vs %v", shares, repeat)
		}
	}
}

func TestDistribute_ZeroWeightsSplitEqually(t *testing.T) {
	shares, err := distribute(7, []int64{0, 0})
	if err != nil {
		t.Fatalf("distribute: %v", err)
	}
	if shares[0]+shares[1] != 7 {
		t.Fatalf("shares=%v do not sum to 7", shares)
	}
}

// A quote whose parts do not add back up to the approved total must never
// become an order at all.
func TestComputeAllocation_ImbalanceIsRefused(t *testing.T) {
	groups := []GroupSource{
		shopGroup("g_a", "biz_a", "branch_a", 100000, 0, 0, 0, 0, DeliveryNone),
	}
	lines := []LineSource{shopLine("l_a", "g_a", "ci_a", 100000)}
	source := buildSource(groups, lines, nil, 0, 0)

	// Corrupt the approved total the way a mis-wired caller would.
	source.FinalPayableMinor += 1

	if _, err := ComputeAllocation(source); !errors.Is(err, ErrAllocationImbalance) {
		t.Fatalf("err=%v, want ErrAllocationImbalance", err)
	}
}

func TestComputeAllocation_LineOutsideItsGroupIsRefused(t *testing.T) {
	groups := []GroupSource{
		shopGroup("g_a", "biz_a", "branch_a", 100000, 0, 0, 0, 0, DeliveryNone),
	}
	lines := []LineSource{shopLine("l_x", "g_missing", "ci_x", 100000)}

	if _, err := ComputeAllocation(buildSource(groups, lines, nil, 0, 0)); !errors.Is(err, ErrQuoteNotOrderable) {
		t.Fatalf("err=%v, want ErrQuoteNotOrderable", err)
	}
}

func TestComputeAllocation_DeliveryDiscountMovesCostToItsFunder(t *testing.T) {
	groups := []GroupSource{
		shopGroup("g_a", "biz_a", "branch_a", 100000, 50000, 50000, 0, 0, DeliveryPlatform),
	}
	lines := []LineSource{shopLine("l_a", "g_a", "ci_a", 100000)}
	adjustments := []AdjustmentSource{{
		AdjustmentID:  "adj_free_delivery",
		Type:          AdjustmentDiscount,
		Scope:         ScopeDelivery,
		FundingSource: FundingPlatform,
		SourceRef:     "promo_free_delivery",
		AmountMinor:   50000,
	}}

	plan, err := ComputeAllocation(buildSource(groups, lines, adjustments, 0, 0))
	if err != nil {
		t.Fatalf("ComputeAllocation: %v", err)
	}
	vendor := plan.Vendors[0]
	if vendor.DeliveryCustomerMinor != 0 {
		t.Errorf("customer delivery=%d, want 0", vendor.DeliveryCustomerMinor)
	}
	if vendor.DeliveryPlatformFundedMinor != 50000 {
		t.Errorf("platform-funded delivery=%d, want 50000", vendor.DeliveryPlatformFundedMinor)
	}
	// The gross fee is unchanged: the provider is still owed it in full.
	if vendor.DeliveryGrossMinor != 50000 {
		t.Errorf("gross delivery=%d, want 50000", vendor.DeliveryGrossMinor)
	}
	if vendor.CustomerPayableMinor != 100000 {
		t.Errorf("customer payable=%d, want 100000", vendor.CustomerPayableMinor)
	}
}

// A delivery subsidy funded by an approved source that is neither the vendor
// nor the platform must keep its own identity. Folding it into the platform
// column would misreport who paid for the delivery.
func TestComputeAllocation_DeliverySubsidyIsNotReportedAsPlatformMoney(t *testing.T) {
	groups := []GroupSource{
		shopGroup("g_a", "biz_a", "branch_a", 100000, 50000, 50000, 0, 0, DeliveryPlatform),
	}
	lines := []LineSource{shopLine("l_a", "g_a", "ci_a", 100000)}
	adjustments := []AdjustmentSource{{
		AdjustmentID:  "adj_subsidy",
		Type:          AdjustmentDiscount,
		Scope:         ScopeDelivery,
		FundingSource: FundingDeliverySubsidy,
		SourceRef:     "grant_2026",
		AmountMinor:   20000,
	}}

	plan, err := ComputeAllocation(buildSource(groups, lines, adjustments, 0, 0))
	if err != nil {
		t.Fatalf("ComputeAllocation: %v", err)
	}
	vendor := plan.Vendors[0]

	if vendor.DeliveryOtherFundedMinor != 20000 {
		t.Errorf("other-funded delivery=%d, want 20000", vendor.DeliveryOtherFundedMinor)
	}
	if vendor.DeliveryPlatformFundedMinor != 0 {
		t.Errorf("subsidy was reported as platform money: %d",
			vendor.DeliveryPlatformFundedMinor)
	}
	if vendor.DeliveryCustomerMinor != 30000 {
		t.Errorf("customer delivery=%d, want 30000", vendor.DeliveryCustomerMinor)
	}
	// The vendor funded none of it, so its payable is untouched.
	if vendor.VendorPayableMinor != 100000 {
		t.Errorf("vendor payable=%d, want 100000", vendor.VendorPayableMinor)
	}

	var found bool
	for _, allocation := range vendor.Allocations {
		if allocation.AllocationType == AllocationDelivery &&
			allocation.FundingSource == FundingDeliverySubsidy {
			found = true
			if allocation.AmountMinor != 20000 {
				t.Errorf("subsidy allocation=%d, want 20000", allocation.AmountMinor)
			}
		}
	}
	if !found {
		t.Error("no allocation row names the delivery subsidy as its funder")
	}
}
