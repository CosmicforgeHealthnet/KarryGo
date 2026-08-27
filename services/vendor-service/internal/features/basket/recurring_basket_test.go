package basket

// Cart optimization, recurring templates, and the exactly-once occurrence
// identity.
//
// The claim these tests defend is that nothing in this feature happens to a
// customer without them asking. A cart is never rearranged by an optimizer
// that thought it knew better; a recurring basket never buys a different
// product without permission; and one scheduled occurrence never becomes two
// deliveries because a worker was restarted.
//
// The occurrence-key tests are the important ones. Exactly-once ultimately
// rests on a unique index, which needs Postgres - but a unique index only
// helps if two attempts at one occurrence present the SAME key, and that half
// is pure logic and is pinned here.

import (
	"context"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Optimize My Cart: proposal only
// ---------------------------------------------------------------------------

type fakeCarts struct {
	snapshot CartSnapshot

	applied int
}

func (f *fakeCarts) LoadForOptimization(
	_ context.Context,
	_ string,
) (CartSnapshot, error) {
	return f.snapshot, nil
}

func (f *fakeCarts) ApplyProposal(
	_ context.Context,
	_ string,
	_ int64,
	_ []ProposedLine,
) (CartSnapshot, error) {
	f.applied++

	f.snapshot.Version++

	return f.snapshot, nil
}

// THE MANDATORY TEST for Smart Basket.
//
// Running the optimizer over a cart must leave the cart exactly as it was. No
// new version, no rewritten lines, no silent substitution - only a proposal
// the customer may accept or ignore.
func TestOptimizeCart_NeverMutatesTheCart(t *testing.T) {
	carts := &fakeCarts{snapshot: CartSnapshot{
		CartID:   "cart_1",
		Version:  5,
		Currency: "NGN",
		Destination: Destination{
			AddressRef: "addr_1", Latitude: 6.43, Longitude: 3.45,
		},
		Lines: []CartLine{{
			LineID:         "line_1",
			ListingRef:     "l_expensive",
			ProductRef:     "p1",
			SellUnitRef:    "unit",
			BusinessID:     "shopA",
			BranchID:       "shopA_br",
			DisplayName:    "Rice",
			Quantity:       2,
			UnitPriceMinor: 90000,
			Currency:       "NGN",
		}},
	}}

	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"line_1": {
			listing("l_cheap", "p1", "shopB", 60000, 50),
			listing("l_expensive", "p1", "shopA", 90000, 50),
		},
	}}

	service, err := NewService(ServiceOptions{
		Catalogue: catalogue,
		Quoter:    &fakeQuoter{perVendorMinor: 10000},
		Carts:     carts,
	})
	if err != nil {
		t.Fatal(err)
	}

	before := carts.snapshot.Version

	result, err := service.OptimizeCart(
		context.Background(), "cust_1",
		AllowSameProductAnyVendor, "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if carts.applied != 0 {
		t.Fatal("optimization applied itself to the cart")
	}

	if carts.snapshot.Version != before {
		t.Fatalf("cart version moved from %d to %d without acceptance",
			before, carts.snapshot.Version)
	}

	if len(result.Variants) == 0 {
		t.Fatal("expected a cheaper proposal")
	}

	// The cheaper option really was found, and it is described as a change
	// from what the customer had rather than presented as a fresh choice.
	best := result.Variants[0]

	if best.Lines[0].ListingRef != "l_cheap" {
		t.Fatalf("proposed %s, expected the cheaper listing",
			best.Lines[0].ListingRef)
	}

	if len(best.Substitutions) != 1 {
		t.Fatalf("substitutions = %d, want 1", len(best.Substitutions))
	}

	substitution := best.Substitutions[0]

	if substitution.FromListingRef != "l_expensive" ||
		substitution.ToListingRef != "l_cheap" {
		t.Fatalf("substitution not described as a change: %+v", substitution)
	}

	if !substitution.VendorChanged {
		t.Fatal("a vendor change must be called out separately")
	}

	if substitution.Basis != BasisSameCatalogueProduct {
		t.Fatalf("basis = %s, want the catalogue product match",
			substitution.Basis)
	}

	if substitution.UnitPriceDeltaMinor != -30000 {
		t.Fatalf("price delta = %d, want -30000",
			substitution.UnitPriceDeltaMinor)
	}
}

func TestOptimizeCart_DoesNotProposeTheCartYouAlreadyHave(t *testing.T) {
	carts := &fakeCarts{snapshot: CartSnapshot{
		CartID:   "cart_1",
		Version:  3,
		Currency: "NGN",
		Destination: Destination{
			AddressRef: "addr_1", Latitude: 6.43, Longitude: 3.45,
		},
		Lines: []CartLine{{
			LineID:         "line_1",
			ListingRef:     "l_only",
			ProductRef:     "p1",
			SellUnitRef:    "unit",
			BusinessID:     "shopA",
			BranchID:       "shopA_br",
			Quantity:       1,
			UnitPriceMinor: 1000,
			Currency:       "NGN",
		}},
	}}

	// The only option is what the customer already has.
	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"line_1": {listing("l_only", "p1", "shopA", 1000, 50)},
	}}

	service, err := NewService(ServiceOptions{
		Catalogue: catalogue,
		Quoter:    &fakeQuoter{perVendorMinor: 10000},
		Carts:     carts,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.OptimizeCart(
		context.Background(), "cust_1",
		AllowSameProductAnyVendor, "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	// "Keep what you have" is not a suggestion. Offering it would invite a
	// customer to press Accept and be told nothing changed.
	if len(result.Variants) != 0 {
		t.Fatalf("proposed the current cart back: %+v", result.Variants)
	}
}

// ---------------------------------------------------------------------------
// Occurrence identity: the half of exactly-once that is pure logic
// ---------------------------------------------------------------------------

func testTemplate() Template {
	return Template{
		ID:         "rbt_1",
		CustomerID: "cust_1",
		Name:       "Weekly shop",
		Currency:   "NGN",
		AddressRef: "addr_1",
		Requirements: []Requirement{
			{Ref: "r1", CategoryRef: "node_rice", Quantity: 2},
		},
		Objective:      ObjectiveValue,
		ApprovalPolicy: ApprovalAlwaysAsk,
		Cadence:        CadenceWeekly,
		Timezone:       "Africa/Lagos",
		LocalHour:      9,
		LocalMinute:    0,
		Weekday:        2,
		Enabled:        true,
	}
}

// The key must depend ONLY on the schedule.
//
// Not on the current time, not on a worker, not on an attempt count. Anything
// volatile in it would make two attempts at one occurrence present two
// different keys, and the unique index they are supposed to collide on would
// happily accept both.
func TestOccurrenceKey_DependsOnlyOnTheSchedule(t *testing.T) {
	template := testTemplate()

	at := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

	first := template.OccurrenceKey(at)

	// Same moment, expressed differently, computed at a different "now".
	second := template.OccurrenceKey(at.In(time.FixedZone("X", 3600)))

	if first != second {
		t.Fatalf("key changed with the caller's timezone: %q vs %q",
			first, second)
	}

	third := template.OccurrenceKey(at.Add(time.Nanosecond))

	if first != third {
		t.Fatalf("key changed with sub-minute noise: %q vs %q", first, third)
	}

	// A DIFFERENT occurrence must get a different key, or a weekly basket
	// would generate once and never again.
	next := template.OccurrenceKey(at.Add(7 * 24 * time.Hour))

	if first == next {
		t.Fatal("two different occurrences share one key")
	}

	// And the key names the zone it was computed in, so an occurrence is
	// interpretable without re-deriving the template's settings.
	if !containsSubstring(first, template.Timezone) {
		t.Fatalf("key does not carry its timezone: %q", first)
	}
}

func TestNextOccurrence_IsComputedInTheCustomersTimezone(t *testing.T) {
	template := testTemplate()

	// A Monday.
	from := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

	next, err := template.NextOccurrence(from)
	if err != nil {
		t.Fatal(err)
	}

	location, err := time.LoadLocation(template.Timezone)
	if err != nil {
		t.Fatal(err)
	}

	local := next.In(location)

	if local.Weekday() != time.Tuesday {
		t.Fatalf("occurrence landed on %s, want Tuesday", local.Weekday())
	}

	if local.Hour() != 9 || local.Minute() != 0 {
		t.Fatalf("occurrence at %02d:%02d local, want 09:00",
			local.Hour(), local.Minute())
	}

	if !next.After(from) {
		t.Fatal("next occurrence must be in the future")
	}
}

func TestNextOccurrence_MonthlyNeverSkipsAMonth(t *testing.T) {
	template := testTemplate()
	template.Cadence = CadenceMonthly
	template.DayOfMonth = 28

	from := time.Date(2026, 1, 29, 12, 0, 0, 0, time.UTC)

	next, err := template.NextOccurrence(from)
	if err != nil {
		t.Fatal(err)
	}

	location, _ := time.LoadLocation(template.Timezone)

	local := next.In(location)

	// February. A day-of-month above 28 would have skipped it, which is why
	// the schema caps it there.
	if local.Month() != time.February {
		t.Fatalf("next monthly occurrence is in %s, want February",
			local.Month())
	}
}

// ---------------------------------------------------------------------------
// Templates store needs, not old SKUs
// ---------------------------------------------------------------------------

func TestTemplate_RequiresATimezone(t *testing.T) {
	template := testTemplate()
	template.Timezone = ""

	if _, err := template.Normalize(); err == nil {
		t.Fatal("a schedule with no timezone must be refused")
	}

	template.Timezone = "Not/AZone"

	if _, err := template.Normalize(); err == nil {
		t.Fatal("an unloadable timezone must be refused at write time")
	}
}

func TestTemplate_AutoApprovalRequiresACeiling(t *testing.T) {
	template := testTemplate()
	template.ApprovalPolicy = ApprovalAutoWithinCeiling
	template.CeilingMinor = 0

	// An unbounded standing purchase authority is not a preference this
	// feature offers. It is refused rather than silently downgraded, so a
	// customer who set the policy and forgot the ceiling is told.
	if _, err := template.Normalize(); err == nil {
		t.Fatal("auto-approval with no ceiling must be refused")
	}

	template.CeilingMinor = 500000

	if _, err := template.Normalize(); err != nil {
		t.Fatalf("a bounded preauthorization must be accepted: %v", err)
	}
}

func TestTemplate_CategoryRequirementIsNotBoundToASKU(t *testing.T) {
	template := testTemplate()

	normalized, err := template.Normalize()
	if err != nil {
		t.Fatal(err)
	}

	requirement := normalized.Requirements[0]

	// The whole design decision: a template stores a NEED. A category
	// requirement that stayed EXACT_ONLY would be unsatisfiable, since there
	// is no exact thing for it to be only.
	if requirement.Substitution != AllowEquivalentProduct {
		t.Fatalf("category requirement allowance = %s",
			requirement.Substitution)
	}

	if requirement.ListingRef != "" || requirement.VariantRef != "" {
		t.Fatal("a category requirement must not carry a listing binding")
	}
}

// ---------------------------------------------------------------------------
// Approval: material changes need permission
// ---------------------------------------------------------------------------

func TestApprovalState_MaterialChangesNeedTheCustomer(t *testing.T) {
	template := testTemplate()
	template.ApprovalPolicy = ApprovalAutoWithinCeiling
	template.CeilingMinor = 500000

	variant := Variant{FinalDeliveredTotalMinor: 100000}

	for _, tc := range []struct {
		name    string
		changes []string
		want    GenerationState
	}{
		{"nothing changed", []string{ChangeNothingMaterial},
			GenerationAutoApproved},

		// The one change this policy names, and only because the ARTICLE is
		// unchanged: the customer receives exactly what they chose, bought at
		// a different shop.
		{"vendor changed", []string{ChangeDifferentVendor},
			GenerationAutoApproved},

		{"product changed", []string{ChangeDifferentProduct},
			GenerationAwaitingApproval},
		{"brand changed", []string{ChangeDifferentBrand},
			GenerationAwaitingApproval},
		{"price rose materially", []string{ChangePriceIncrease},
			GenerationAwaitingApproval},
		{"quantity unmet", []string{ChangeQuantityUnmet},
			GenerationAwaitingApproval},
	} {
		got := approvalStateFor(
			template, PlanComparison{Changes: tc.changes}, variant,
		)

		if got != tc.want {
			t.Fatalf("%s: state = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestApprovalState_CeilingIsAHardBound(t *testing.T) {
	template := testTemplate()
	template.ApprovalPolicy = ApprovalAutoWithinCeiling
	template.CeilingMinor = 100000

	// One kobo over the ceiling the customer themselves set. Nothing is
	// auto-approved past it, however unchanged the basket is.
	over := Variant{FinalDeliveredTotalMinor: 100001}

	if approvalStateFor(
		template, PlanComparison{Changes: []string{ChangeNothingMaterial}},
		over,
	) != GenerationAwaitingApproval {
		t.Fatal("spending past the ceiling was auto-approved")
	}
}

func TestApprovalState_AlwaysAskNeverAutoApproves(t *testing.T) {
	template := testTemplate()

	state := approvalStateFor(
		template,
		PlanComparison{Changes: []string{ChangeNothingMaterial}},
		Variant{FinalDeliveredTotalMinor: 1},
	)

	if state != GenerationProposed {
		t.Fatalf("state = %s, want PROPOSED", state)
	}
}

// ---------------------------------------------------------------------------
// Material change detection
// ---------------------------------------------------------------------------

func TestMaterialChanges_IgnoreOperationalDifferences(t *testing.T) {
	previous := []ProposedLine{{
		RequirementRef: "r1",
		ProductRef:     "p1",
		VariantRef:     "p1_v1",
		Brand:          "BrandA",
		BusinessID:     "shopA",
		BranchID:       "shopA_br",
		Quantity:       2,
		LineTotalMinor: 100000,
	}}

	// The same product, the same brand, the same shop, the same quantity, at
	// a price two per cent higher. Prices move; demanding approval for every
	// kobo trains people to approve without reading.
	optimized := Variant{
		MerchandiseTotalMinor: 102000,
		Lines: []ProposedLine{{
			RequirementRef: "r1",
			ProductRef:     "p1",
			VariantRef:     "p1_v1",
			Brand:          "BrandA",
			BusinessID:     "shopA",
			BranchID:       "shopA_br",
			Quantity:       2,
			LineTotalMinor: 102000,
		}},
	}

	changes, vendorChanges := materialChanges(optimized, previous)

	if len(changes) != 1 || changes[0] != ChangeNothingMaterial {
		t.Fatalf("changes = %v, want no material change", changes)
	}

	if len(vendorChanges) != 0 {
		t.Fatalf("vendor changes = %v", vendorChanges)
	}
}

func TestMaterialChanges_NameEveryDifferenceThatMatters(t *testing.T) {
	previous := []ProposedLine{{
		RequirementRef: "r1",
		ProductRef:     "p1",
		VariantRef:     "p1_v1",
		Brand:          "BrandA",
		BusinessID:     "shopA",
		BranchID:       "shopA_br",
		Quantity:       2,
		LineTotalMinor: 100000,
	}}

	optimized := Variant{
		MerchandiseTotalMinor: 180000,
		Lines: []ProposedLine{{
			RequirementRef: "r1",
			ProductRef:     "p2",
			VariantRef:     "p2_v1",
			Brand:          "BrandB",
			BusinessID:     "shopB",
			BranchID:       "shopB_br",
			Quantity:       2,
			LineTotalMinor: 180000,
		}},
	}

	changes, vendorChanges := materialChanges(optimized, previous)

	for _, want := range []string{
		ChangeDifferentProduct,
		ChangeDifferentBrand,
		ChangeDifferentVendor,
		ChangePriceIncrease,
	} {
		if !contains(changes, want) {
			t.Fatalf("changes %v missing %s", changes, want)
		}
	}

	if len(vendorChanges) != 1 {
		t.Fatalf("vendor changes = %v, want one named move", vendorChanges)
	}
}

func TestMaterialChanges_FirstOccurrenceHasNothingToDifferFrom(t *testing.T) {
	changes, _ := materialChanges(Variant{}, nil)

	if len(changes) != 1 || changes[0] != ChangeNothingMaterial {
		t.Fatalf("changes = %v", changes)
	}
}

// ---------------------------------------------------------------------------
// Comparison
// ---------------------------------------------------------------------------

func TestCompareRecurringPlans_ReportsBothSidesAndTheDeltas(t *testing.T) {
	optimized := Variant{
		MerchandiseTotalMinor:    90000,
		DeliveryTotalMinor:       20000,
		FinalDeliveredTotalMinor: 110000,
		VendorCount:              1,
		PlanSummary:              PlanSummary{CompletionMinutes: 40},
	}

	sameVendor := Variant{
		MerchandiseTotalMinor:    85000,
		DeliveryTotalMinor:       60000,
		FinalDeliveredTotalMinor: 145000,
		VendorCount:              3,
		PlanSummary:              PlanSummary{CompletionMinutes: 35},
	}

	comparison := compareRecurringPlans(optimized, &sameVendor, nil)

	if !comparison.SameVendorAvailable {
		t.Fatal("same-vendor plan should be available")
	}

	// Dearer on goods, much cheaper delivered, fewer vendors, slightly
	// slower. A customer deciding whether to accept needs all four.
	if comparison.MerchandiseDeltaMinor != 5000 {
		t.Fatalf("merchandise delta = %d", comparison.MerchandiseDeltaMinor)
	}

	if comparison.DeliveryDeltaMinor != -40000 {
		t.Fatalf("delivery delta = %d", comparison.DeliveryDeltaMinor)
	}

	if comparison.TotalDeltaMinor != -35000 {
		t.Fatalf("total delta = %d", comparison.TotalDeltaMinor)
	}

	if comparison.VendorCountDelta != -2 {
		t.Fatalf("vendor delta = %d", comparison.VendorCountDelta)
	}

	if comparison.CompletionDeltaMinutes != 5 {
		t.Fatalf("completion delta = %d", comparison.CompletionDeltaMinutes)
	}
}

func TestCompareRecurringPlans_SaysWhenTheOldVendorsCannotSupply(t *testing.T) {
	previous := []ProposedLine{{RequirementRef: "r1", ListingRef: "gone"}}

	comparison := compareRecurringPlans(Variant{}, nil, previous)

	if comparison.SameVendorAvailable {
		t.Fatal("there is no same-vendor plan to compare against")
	}

	// Stated rather than silent. A missing comparison that says nothing is
	// indistinguishable from a comparison nobody ran.
	if comparison.SameVendorReason == "" {
		t.Fatal("an unavailable baseline must say why")
	}
}

func containsSubstring(haystack, needle string) bool {
	return len(haystack) >= len(needle) &&
		(haystack == needle ||
			len(needle) == 0 ||
			indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if haystack[index:index+len(needle)] == needle {
			return index
		}
	}

	return -1
}
