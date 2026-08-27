package promotions

import (
	"testing"
	"time"
)

func baseCampaign(kind Type) Campaign {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	return Campaign{ID: "promo_1", BusinessID: "biz_1", Name: "Offer", Type: kind, Status: Active,
		Currency: "NGN", StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Sponsor: Vendor,
		VendorFundingBPS: 10000, Combinability: Exclusive, Priority: 10, Version: 3}
}

func baseInput() EvaluationInput {
	return EvaluationInput{CustomerID: "customer_1", Currency: "NGN", ProductSubtotalMinor: 10000,
		DeliveryCostMinor: 2000, At: time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		CustomerSegments: map[string]bool{"seg_vip": true}, Capabilities: map[string]bool{"SHOP_GROCERY": true},
		CategoryRefs: map[string]bool{"cat_1": true}, Lines: []Line{{BusinessID: "biz_1", BranchID: "branch_1",
			Capability: "SHOP_GROCERY", CategoryRef: "cat_1", ItemRef: "item_1", VariantRef: "variant_1", Quantity: 5, AmountMinor: 10000}}}
}

func TestPercentageAndFixedDiscountsAreIntegerAndCapped(t *testing.T) {
	p := baseCampaign(PercentageDiscount)
	p.PercentageBPS = 1250
	capMinor := int64(1000)
	p.MaxDiscountMinor = &capMinor
	got, ok := EvaluateCampaign(p, baseInput())
	if !ok || got.DiscountMinor != 1000 || got.VendorFundedMinor != 1000 {
		t.Fatalf("got %+v", got)
	}
	f := baseCampaign(FixedDiscount)
	f.FixedMinor = 20000
	got, ok = EvaluateCampaign(f, baseInput())
	if !ok || got.DiscountMinor != 10000 {
		t.Fatalf("fixed discount was not floored: %+v", got)
	}
}

func TestCouponAndTimeBoundariesFailClosed(t *testing.T) {
	c := baseCampaign(Coupon)
	c.FixedMinor, c.CouponRequired, c.CouponDigest = 500, true, "digest"
	in := baseInput()
	if _, ok := EvaluateCampaign(c, in); ok {
		t.Fatal("coupon-free request applied")
	}
	in.CouponDigest = "digest"
	if got, ok := EvaluateCampaign(c, in); !ok || got.CouponDigest != "digest" {
		t.Fatalf("got %+v", got)
	}
	in.At = c.EndsAt
	if _, ok := EvaluateCampaign(c, in); ok {
		t.Fatal("end boundary must be exclusive")
	}
	c.Status = Paused
	in.At = c.StartsAt
	if _, ok := EvaluateCampaign(c, in); ok {
		t.Fatal("paused campaign applied")
	}
}

func TestBundleQuantityAndEligibilityScopes(t *testing.T) {
	b := baseCampaign(Bundle)
	b.FixedMinor = 700
	b.BundleRequirements = []BundleRequirement{{ItemRef: "item_1", MinimumQuantity: 5}}
	b.Scopes = []Scope{{Type: "BRANCH", Ref: "branch_1"}, {Type: "CAPABILITY", Ref: "SHOP_GROCERY"},
		{Type: "CATEGORY", Ref: "cat_1"}, {Type: "ITEM", Ref: "item_1"}, {Type: "CUSTOMER_SEGMENT", Ref: "seg_vip"}}
	if got, ok := EvaluateCampaign(b, baseInput()); !ok || got.DiscountMinor != 700 {
		t.Fatalf("got %+v", got)
	}

	q := baseCampaign(QuantityBreak)
	max4, max9 := int64(4), int64(9)
	q.QuantityTiers = []QuantityTier{{MinimumQuantity: 1, MaximumQuantity: &max4, FixedMinor: 100},
		{MinimumQuantity: 5, MaximumQuantity: &max9, PercentageBPS: 1000}, {MinimumQuantity: 10, FixedMinor: 2000}}
	if got, ok := EvaluateCampaign(q, baseInput()); !ok || got.DiscountMinor != 1000 {
		t.Fatalf("quantity boundary: %+v", got)
	}
}

func TestDeliveryPromotionsPreserveUnderlyingCostAndFunding(t *testing.T) {
	free := baseCampaign(FreeDelivery)
	free.Sponsor, free.VendorFundingBPS = Combined, 6000
	free.PlatformApprovalRef = "platform_approval_1"
	got, ok := EvaluateCampaign(free, baseInput())
	if !ok || got.CalculationBaseMinor != 2000 || got.DiscountMinor != 2000 || got.VendorFundedMinor != 1200 || got.PlatformFundedMinor != 800 {
		t.Fatalf("free-delivery funding collapsed: %+v", got)
	}
	percentage := baseCampaign(DeliveryPercentage)
	percentage.PercentageBPS, percentage.Sponsor, percentage.VendorFundingBPS = 5000, Platform, 0
	percentage.PlatformApprovalRef = "platform_approval_2"
	got, ok = EvaluateCampaign(percentage, baseInput())
	if !ok || got.DiscountMinor != 1000 || got.PlatformFundedMinor != 1000 {
		t.Fatalf("got %+v", got)
	}
	flat := baseCampaign(DeliveryFlat)
	flat.FixedMinor = 500
	got, ok = EvaluateCampaign(flat, baseInput())
	if !ok || got.DiscountMinor != 500 {
		t.Fatalf("got %+v", got)
	}
}

func TestStackingOrderIsDeterministicAndExclusiveStops(t *testing.T) {
	a, b := baseCampaign(FixedDiscount), baseCampaign(FixedDiscount)
	a.ID, a.FixedMinor, a.Priority, a.Combinability = "promo_a", 100, 20, Stackable
	b.ID, b.FixedMinor, b.Priority, b.Combinability = "promo_b", 200, 10, Exclusive
	got := Evaluate([]Campaign{a, b}, baseInput())
	if len(got) != 1 || got[0].CampaignID != "promo_b" {
		t.Fatalf("got %+v", got)
	}
}

func TestExclusiveCampaignDoesNotCombineWithEarlierStackable(t *testing.T) {
	stackable, exclusive := baseCampaign(FixedDiscount), baseCampaign(FixedDiscount)
	stackable.ID, stackable.FixedMinor, stackable.Priority, stackable.Combinability = "promo_stack", 100, 5, Stackable
	exclusive.ID, exclusive.FixedMinor, exclusive.Priority, exclusive.Combinability = "promo_exclusive", 200, 10, Exclusive
	got := Evaluate([]Campaign{stackable, exclusive}, baseInput())
	if len(got) != 1 || got[0].CampaignID != exclusive.ID {
		t.Fatalf("exclusive campaign combined: %+v", got)
	}
}

func TestCampaignTransitionsAndTierOverlap(t *testing.T) {
	if !CanTransition(Draft, Active) || CanTransition(Archived, Active) {
		t.Fatal("invalid lifecycle")
	}
	q := baseCampaign(QuantityBreak)
	max5 := int64(5)
	q.QuantityTiers = []QuantityTier{{MinimumQuantity: 1, MaximumQuantity: &max5, FixedMinor: 1}, {MinimumQuantity: 5, FixedMinor: 2}}
	if q.Validate() == nil {
		t.Fatal("overlapping quantity tiers accepted")
	}
}

func TestScopeRefsAreORedWithinTypeAndScopedLinesDefineBase(t *testing.T) {
	campaign := baseCampaign(PercentageDiscount)
	campaign.PercentageBPS = 1000
	campaign.Scopes = []Scope{{Type: "BRANCH", Ref: "branch_other"}, {Type: "BRANCH", Ref: "branch_1"},
		{Type: "ITEM", Ref: "item_1"}}
	input := baseInput()
	input.ProductSubtotalMinor = 15000
	input.Lines = append(input.Lines, Line{BusinessID: "biz_1", BranchID: "branch_2", ItemRef: "item_2", Quantity: 1, AmountMinor: 5000})
	got, ok := EvaluateCampaign(campaign, input)
	if !ok || got.CalculationBaseMinor != 10000 || got.DiscountMinor != 1000 {
		t.Fatalf("scope OR/filter semantics incorrect: %+v", got)
	}
}

func TestScheduledWindowAndBranchBoundCapability(t *testing.T) {
	campaign := baseCampaign(FixedDiscount)
	campaign.FixedMinor = 100
	campaign.Status = Scheduled
	campaign.Scopes = []Scope{{Type: "BRANCH", Ref: "branch_1"}, {Type: "CAPABILITY", Ref: "SHOP_GROCERY"}}
	input := baseInput()
	input.Lines[0].Capability = ""
	input.Capabilities = map[string]bool{capabilityEligibilityKey("biz_1", "branch_2", "SHOP_GROCERY"): true}
	if _, ok := EvaluateCampaign(campaign, input); ok {
		t.Fatal("capability from a foreign branch qualified the campaign")
	}
	input.Capabilities[capabilityEligibilityKey("biz_1", "branch_1", "SHOP_GROCERY")] = true
	if _, ok := EvaluateCampaign(campaign, input); !ok {
		t.Fatal("scheduled campaign inside its authoritative window did not apply")
	}
}

func TestFrozenCalculationDoesNotChangeWithCampaignRevision(t *testing.T) {
	campaign := baseCampaign(PercentageDiscount)
	campaign.PercentageBPS = 2000
	frozen, ok := EvaluateCampaign(campaign, baseInput())
	if !ok || frozen.DiscountMinor != 2000 || frozen.CampaignVersion != 3 {
		t.Fatalf("initial snapshot: %+v", frozen)
	}
	campaign.PercentageBPS = 1000
	campaign.Version = 4
	current, ok := EvaluateCampaign(campaign, baseInput())
	if !ok || current.DiscountMinor != 1000 || frozen.DiscountMinor != 2000 || frozen.CampaignVersion != 3 {
		t.Fatalf("historical snapshot was recomputed: frozen=%+v current=%+v", frozen, current)
	}
}
