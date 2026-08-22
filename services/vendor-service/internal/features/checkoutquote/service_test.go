package checkoutquote

import (
	"testing"
	"time"
)

func sourceFixture(now time.Time) SourceSnapshot {
	return SourceSnapshot{
		CheckoutSessionID: "checkout-1", CheckoutValidationRevision: 3,
		CheckoutExpiresAt: now.Add(10 * time.Minute), CartID: "cart-1", CartVersion: 7,
		CustomerID: "customer-1", Currency: "NGN",
		Address:               AddressSnapshot{SourceAddressRef: "address-1", Address: "1 Test Street", Latitude: 6.5, Longitude: 3.4, SnapshottedAt: now.Add(-time.Minute)},
		QuotedItemsTotalMinor: 1000, MaxAuthorizedItemsTotalMinor: 1000,
		FulfilmentPlanID: "plan-1", FulfilmentPlanRevision: 2, FulfilmentPlanVersion: 4,
		FulfilmentPlanExpiresAt: now.Add(8 * time.Minute), SelectedFulfilmentMode: "SINGLE",
		DeliveryFunding: DeliveryFunding{GrossMinor: 500, CustomerMinor: 200, VendorFundedMinor: 300},
		Groups:          []GroupSnapshot{{CheckoutGroupID: "group-1", BusinessID: "business-1", BranchID: "branch-1", ProductSubtotalMinor: 1000, MaxProductSubtotalMinor: 1000, InventoryReservationRef: "reservation-1", LogisticsPlanGroupRef: "plan-group-1", LogisticsMissionRef: "mission-1", DeliveryFunding: DeliveryFunding{GrossMinor: 500, CustomerMinor: 200, VendorFundedMinor: 300}}},
		Lines:           []LineSnapshot{{CheckoutLineID: "line-1", CheckoutGroupID: "group-1", CartItemID: "item-1", Domain: "SHOP", BusinessID: "business-1", BranchID: "branch-1", ItemRef: "product-1", VariantRef: "variant-1", ListingRef: "listing-1", SellUnitRef: "unit-1", Quantity: 1, VendorOnlineUnitPriceMinor: 1000, PriceReference: "price-v3", CatalogueRevision: 9, PricingBasis: "FIXED_UNIT", QuotedTotalMinor: 1000, MaxAuthorizedTotalMinor: 1000, InventoryItemRef: "stock-1", InventoryReservationRef: "reservation-1"}},
		Missions:        []MissionSnapshot{{MissionID: "mission-1", Authority: "VENDOR_OWN_DELIVERY", Service: "VENDOR", GroupIDs: []string{"group-1"}, Funding: DeliveryFunding{GrossMinor: 500, CustomerMinor: 200, VendorFundedMinor: 300}}},
	}
}

func TestBuildQuoteExactTotalAndDeliveryFunding(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	quote, err := buildQuote(sourceFixture(now), nil, now, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if quote.ProductSubtotalMinor != 1000 || quote.DeliverySubtotalMinor != 500 ||
		quote.DiscountTotalMinor != 300 || quote.SubsidyTotalMinor != 300 ||
		quote.FinalPayableMinor != 1200 || quote.QuotedTotalMinor != 1200 ||
		quote.MaxAuthorizedTotalMinor != 1200 {
		t.Fatalf("unexpected exact quote: %+v", quote)
	}
	if quote.FundingBreakdown.DeliveryCustomerMinor != 200 ||
		quote.FundingBreakdown.DeliveryVendorFundedMinor != 300 ||
		quote.FundingBreakdown.DeliveryPlatformFundedMinor != 0 {
		t.Fatalf("delivery subsidy was misattributed: %+v", quote.FundingBreakdown)
	}
	if quote.PriceDisclosure.PriceContext != "VENDOR_ONLINE_PRICE" ||
		quote.PriceDisclosure.StorePriceParityClaimed || quote.Lines[0].StorePriceParityClaimed {
		t.Fatalf("online-price disclosure missing: %+v", quote.PriceDisclosure)
	}
}

func TestBuildQuoteVariableMaximumIsBounded(t *testing.T) {
	now := time.Now().UTC()
	source := sourceFixture(now)
	source.MaxAuthorizedItemsTotalMinor = 1150
	source.Groups[0].MaxProductSubtotalMinor = 1150
	source.Lines[0].PricingBasis = "VARIABLE_MEASURE"
	source.Lines[0].MaxAuthorizedTotalMinor = 1150
	source.Lines[0].MeasureOverfillBps = 1500
	quote, err := buildQuote(source, nil, now, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if quote.QuotedTotalMinor != 1200 || quote.MaxAuthorizedTotalMinor != 1350 {
		t.Fatalf("variable authorization not bounded by source policy: %+v", quote)
	}
}

func TestBuildQuoteAttributesDiscountFeeAndTaxFunding(t *testing.T) {
	now := time.Now().UTC()
	adjustments := []Adjustment{
		{ID: "vendor-promo", Type: AdjustmentDiscount, Scope: ScopeProduct, FundingSource: FundingVendor, SourceRef: "promo-v1", AmountMinor: 100},
		{ID: "platform-promo", Type: AdjustmentDiscount, Scope: ScopeOrder, FundingSource: FundingPlatform, SourceRef: "promo-p1", AmountMinor: 50},
		{ID: "service-fee", Type: AdjustmentFee, Scope: ScopeOrder, FundingSource: FundingCustomer, SourceRef: "fee-v1", AmountMinor: 20},
		{ID: "tax", Type: AdjustmentTax, Scope: ScopeOrder, FundingSource: FundingCustomer, SourceRef: "tax-v1", AmountMinor: 10},
	}
	quote, err := buildQuote(sourceFixture(now), adjustments, now, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if quote.DiscountTotalMinor != 450 || quote.FeeSubtotalMinor != 20 || quote.TaxSubtotalMinor != 10 || quote.FinalPayableMinor != 1080 {
		t.Fatalf("unexpected commercial totals: %+v", quote.PriceDisclosure)
	}
	if quote.FundingBreakdown.ProductVendorFundedMinor != 100 || quote.FundingBreakdown.OrderPlatformFundedMinor != 50 {
		t.Fatalf("product benefit funding lost: %+v", quote.FundingBreakdown)
	}
}

func TestBuildQuoteAttributesDeliveryPromotionWithoutChargingCustomer(t *testing.T) {
	now := time.Now().UTC()
	quote, err := buildQuote(sourceFixture(now), []Adjustment{{
		ID: "free-delivery", Type: AdjustmentDiscount, Scope: ScopeDelivery,
		FundingSource: FundingDeliverySubsidy, SourceRef: "subsidy-program", AmountMinor: 100,
	}}, now, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if quote.FundingBreakdown.DeliveryCustomerMinor != 100 ||
		quote.FundingBreakdown.DeliveryOtherFundedMinor != 100 ||
		quote.FinalPayableMinor != 1100 {
		t.Fatalf("delivery promotion was misattributed: quote=%+v funding=%+v", quote.PriceDisclosure, quote.FundingBreakdown)
	}
}

func TestFingerprintDeterministicallyBindsMaterialAndVersion(t *testing.T) {
	now := time.Now().UTC()
	quote, err := buildQuote(sourceFixture(now), nil, now, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	quote.QuoteVersion = 1
	first, err := QuoteFingerprint(quote)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := QuoteFingerprint(quote)
	if first != second {
		t.Fatal("same material state did not have stable fingerprint semantics")
	}
	quote.Lines[0].Quantity++
	changed, _ := QuoteFingerprint(quote)
	if changed == first {
		t.Fatal("material line change did not change fingerprint")
	}
	quote.Lines[0].Quantity--
	quote.QuoteVersion = 2
	versioned, _ := QuoteFingerprint(quote)
	if versioned == first {
		t.Fatal("quote version was not bound into fingerprint")
	}
	quote.QuoteVersion = 1
	quote.ExpiresAt = quote.ExpiresAt.Add(time.Second)
	rebounded, _ := QuoteFingerprint(quote)
	if rebounded == first {
		t.Fatal("quote expiry was not bound into fingerprint")
	}
}

func TestBuildQuoteRefusesExpiredSource(t *testing.T) {
	now := time.Now().UTC()
	source := sourceFixture(now)
	source.FulfilmentPlanExpiresAt = now
	if _, err := buildQuote(source, nil, now, 5*time.Minute); err != ErrQuoteExpired {
		t.Fatalf("expired plan produced quote: %v", err)
	}
}

func TestAssessReplanProtectsApprovedTotalWithoutMovingMoney(t *testing.T) {
	now := time.Now().UTC()
	current := Quote{ID: "q1", CustomerID: "c1", CheckoutSessionID: "s1", Currency: "NGN", QuotedTotalMinor: 1200, ApprovedAt: &now}
	cheaper := current
	cheaper.ID = "q2"
	cheaper.QuotedTotalMinor = 1000
	assessment, err := AssessReplan(current, cheaper)
	if err != nil || assessment.Disposition != ReplanReleaseOrRefundDifference || assessment.DifferenceMinor != 200 {
		t.Fatalf("cheaper assessment=%+v err=%v", assessment, err)
	}
	moreExpensive := cheaper
	moreExpensive.QuotedTotalMinor = 1400
	assessment, err = AssessReplan(current, moreExpensive)
	if err != nil || assessment.Disposition != ReplanCustomerReapproval || assessment.DifferenceMinor != 200 {
		t.Fatalf("expensive assessment=%+v err=%v", assessment, err)
	}
}
