package basket

// Optimizer tests.
//
// Two themes run through all of them.
//
// The first is that a wrong answer must be impossible, not merely unlikely: a
// required brand is never substituted, a quality ranking is never invented, a
// basket is never reported as fitting a budget it does not fit, and a
// requirement is never quietly satisfied with fewer items.
//
// The second is that the optimizer's side effects are counted rather than
// assumed. The fake quoter records every call, and the tests assert that
// evaluating many baskets produces many QUOTES and nothing else - no plan, no
// mission, no reservation, no payment.

import (
	"context"
	"strings"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakeCatalogue struct {
	// byRequirement is the shortlist each requirement resolves to, keyed by
	// requirement ref.
	byRequirement map[string][]Listing

	byListing map[string]Listing

	calls int
}

func (f *fakeCatalogue) FindCandidates(
	_ context.Context,
	query CandidateQuery,
) ([]Listing, error) {
	f.calls++

	return f.byRequirement[query.Requirement.Ref], nil
}

func (f *fakeCatalogue) ResolveListings(
	_ context.Context,
	refs []string,
) (map[string]Listing, error) {
	out := map[string]Listing{}

	for _, ref := range refs {
		if listing, ok := f.byListing[ref]; ok {
			out[ref] = listing
		}
	}

	return out, nil
}

// fakeQuoter prices delivery per VENDOR COUNT, which is the shape that
// actually matters: consolidating onto fewer shops is cheaper to deliver, and
// that is the trade the optimizer exists to find.
//
// It also records every request, which is how the side-effect tests count
// what the optimizer did.
type fakeQuoter struct {
	perVendorMinor int64
	minutes        int

	unserviceable bool

	requests []fulfilment.QuoteOnlyRequest
}

func (f *fakeQuoter) QuotePlan(
	_ context.Context,
	request fulfilment.QuoteOnlyRequest,
) (fulfilment.QuoteOnlyResult, error) {
	f.requests = append(f.requests, request)

	if f.unserviceable {
		return fulfilment.QuoteOnlyResult{
			Serviceable: false,
			ReasonCode:  "GROUP_UNSERVICEABLE",
		}, nil
	}

	vendors := len(request.Groups)

	fee := f.perVendorMinor * int64(vendors)

	minutes := f.minutes

	if minutes == 0 {
		minutes = 30
	}

	return fulfilment.QuoteOnlyResult{
		Mode:                  request.Mode,
		Serviceable:           true,
		Currency:              request.Currency,
		DeliveryGrossMinor:    fee,
		DeliveryCustomerMinor: fee,
		MissionCount:          vendors,
		VendorGroupCount:      vendors,
		CompletionMinutes:     minutes,
	}, nil
}

func listing(
	ref, product, business string,
	priceMinor int64,
	stock int64,
) Listing {
	return Listing{
		ListingRef:        ref,
		ProductRef:        product,
		VariantRef:        product + "_v1",
		BusinessID:        business,
		BranchID:          business + "_br",
		SellUnitRef:       "unit",
		SellUnitName:      "each",
		DisplayName:       product,
		Brand:             "BrandA",
		CanonicalNodeRef:  "node_" + product,
		UnitPriceMinor:    priceMinor,
		Currency:          "NGN",
		PriceReference:    ref + ":unit",
		AvailableQuantity: stock,
	}
}

func newTestService(
	t *testing.T,
	catalogue *fakeCatalogue,
	quoter *fakeQuoter,
) *Service {
	t.Helper()

	service, err := NewService(ServiceOptions{
		Catalogue: catalogue,
		Quoter:    quoter,
	})
	if err != nil {
		t.Fatal(err)
	}

	return service
}

func baseRequest(requirements ...Requirement) Request {
	return Request{
		CustomerID: "cust_1",
		Currency:   "NGN",
		Destination: Destination{
			AddressRef: "addr_1",
			Latitude:   6.43,
			Longitude:  3.45,
		},
		Requirements: requirements,
	}
}

func need(ref, product string, quantity int64) Requirement {
	return Requirement{
		Ref:          ref,
		ProductRef:   product,
		Quantity:     quantity,
		Substitution: AllowSameProductAnyVendor,
	}
}

// ---------------------------------------------------------------------------
// Side effects: there are none
// ---------------------------------------------------------------------------

// THE MANDATORY TEST.
//
// Running the optimizer over many candidate baskets must create zero
// logistics missions and zero financial records. It is asserted structurally
// as well as by counting: the service is constructed with no wallet, no order
// service and no mission authority, so there is nothing it COULD call - and
// the quote count proves it did the work rather than skipping it.
func TestOptimize_ManyBasketsCreateNoMissionsAndNoMoney(t *testing.T) {
	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{}}

	// Six requirements, each stocked by four different shops: enough
	// candidate vendors that the optimizer genuinely enumerates baskets.
	for index := 0; index < 6; index++ {
		ref := "r" + string(rune('1'+index))

		for vendor := 0; vendor < 4; vendor++ {
			shop := "shop" + string(rune('A'+vendor))

			catalogue.byRequirement[ref] = append(
				catalogue.byRequirement[ref],
				listing(ref+"_"+shop, "p"+ref, shop,
					int64(1000+vendor*50), 100),
			)
		}
	}

	quoter := &fakeQuoter{perVendorMinor: 50000}

	service := newTestService(t, catalogue, quoter)

	var requirements []Requirement

	for index := 0; index < 6; index++ {
		requirements = append(
			requirements, need("r"+string(rune('1'+index)), "", 1),
		)
	}

	// Category-style requirements: the fake catalogue answers by ref.
	for index := range requirements {
		requirements[index].ProductRef = ""
		requirements[index].CategoryRef = "cat"
		requirements[index].Substitution = AllowEquivalentProduct
	}

	// The fake catalogue keys on ref, so equivalence must be satisfied by the
	// canonical node the listings carry.
	for index := range requirements {
		ref := requirements[index].Ref
		requirements[index].CategoryRef =
			catalogue.byRequirement[ref][0].CanonicalNodeRef
	}

	result, err := service.Optimize(
		context.Background(), baseRequest(requirements...),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Variants) == 0 {
		t.Fatal("expected at least one variant")
	}

	if len(quoter.requests) < 2 {
		t.Fatalf("expected several baskets to be quoted, got %d",
			len(quoter.requests))
	}

	// The whole point: many quotes, and nothing else. A mission, a
	// reservation or a payment intent would have to have been created by a
	// dependency this service does not hold.
	if service.carts != nil || service.store != nil {
		t.Fatal("optimizer was constructed with mutation dependencies")
	}
}

// ---------------------------------------------------------------------------
// Budget is the DELIVERED total
// ---------------------------------------------------------------------------

func TestOptimize_BudgetIsEvaluatedOnTheDeliveredTotal(t *testing.T) {
	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {listing("l1", "p1", "shopA", 80000, 10)},
	}}

	// Merchandise 80,000 fits a budget of 100,000. Delivery is 50,000, so the
	// delivered total is 130,000 and does not.
	quoter := &fakeQuoter{perVendorMinor: 50000}

	service := newTestService(t, catalogue, quoter)

	request := baseRequest(need("r1", "p1", 1))
	request.BudgetMinor = 100000

	result, err := service.Optimize(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	if result.FitsBudget {
		t.Fatal("merchandise fit the budget but the delivered total did not")
	}

	if result.MinimumFeasibleDeliveredTotalMinor != 130000 {
		t.Fatalf("minimum feasible = %d, want 130000",
			result.MinimumFeasibleDeliveredTotalMinor)
	}

	if result.BudgetShortfallMinor != 30000 {
		t.Fatalf("shortfall = %d, want 30000", result.BudgetShortfallMinor)
	}
}

func TestOptimize_BudgetFitReportsTheWholeBasket(t *testing.T) {
	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {listing("l1", "p1", "shopA", 30000, 10)},
	}}

	quoter := &fakeQuoter{perVendorMinor: 20000, minutes: 40}

	service := newTestService(t, catalogue, quoter)

	request := baseRequest(need("r1", "p1", 2))
	request.BudgetMinor = 100000

	result, err := service.Optimize(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	if !result.FitsBudget {
		t.Fatal("basket should fit the budget")
	}

	variant := result.Variants[0]

	if variant.MerchandiseTotalMinor != 60000 {
		t.Fatalf("merchandise = %d, want 60000",
			variant.MerchandiseTotalMinor)
	}

	if variant.DeliveryTotalMinor != 20000 {
		t.Fatalf("delivery = %d, want 20000", variant.DeliveryTotalMinor)
	}

	if variant.FinalDeliveredTotalMinor != 80000 {
		t.Fatalf("delivered total = %d, want 80000",
			variant.FinalDeliveredTotalMinor)
	}

	if variant.VendorCount != 1 {
		t.Fatalf("vendor count = %d, want 1", variant.VendorCount)
	}

	// An expected delivery range built from the planner's own quoted transit,
	// not from an average.
	if variant.DeliveryWindow.LatestMinutes <= 0 ||
		variant.DeliveryWindow.EarliestMinutes >=
			variant.DeliveryWindow.LatestMinutes {
		t.Fatalf("delivery range is not usable: %+v", variant.DeliveryWindow)
	}

	// The claim is stated precisely, and it is the bounded one.
	if result.OptimalityNote != OptimalityUnderBoundedSearch {
		t.Fatalf("optimality note = %q", result.OptimalityNote)
	}
}

// ---------------------------------------------------------------------------
// No-solution answers are truthful
// ---------------------------------------------------------------------------

func TestOptimize_NoCompleteBasketIsNotABudgetProblem(t *testing.T) {
	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {listing("l1", "p1", "shopA", 1000, 10)},
		// r2 has nothing listed at all.
	}}

	service := newTestService(t, catalogue, &fakeQuoter{perVendorMinor: 1000})

	request := baseRequest(need("r1", "p1", 1), need("r2", "p2", 1))
	request.BudgetMinor = 10000000

	result, err := service.Optimize(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	if !result.NoCompleteBasket {
		t.Fatal("a requirement nothing can satisfy must be reported as such")
	}

	if len(result.UnmetRequirements) != 1 ||
		result.UnmetRequirements[0].RequirementRef != "r2" {
		t.Fatalf("unmet requirements = %+v", result.UnmetRequirements)
	}

	// No minimum feasible total is quoted, because there is no complete
	// basket to quote one for. Reporting one would be pricing something that
	// cannot be bought.
	if result.MinimumFeasibleDeliveredTotalMinor != 0 {
		t.Fatalf("quoted a minimum for an impossible basket: %d",
			result.MinimumFeasibleDeliveredTotalMinor)
	}

	if len(result.Variants) != 0 {
		t.Fatal("fabricated a basket for an unsatisfiable requirement")
	}
}

func TestOptimize_UnmetRequirementNamesTheBindingConstraint(t *testing.T) {
	// Everything listed is the wrong brand. "Nothing is available" would send
	// the customer looking for a product that is right in front of them.
	wrongBrand := listing("l1", "p1", "shopA", 1000, 10)
	wrongBrand.Brand = "BrandZ"

	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {wrongBrand},
	}}

	service := newTestService(t, catalogue, &fakeQuoter{perVendorMinor: 1000})

	requirement := need("r1", "p1", 1)
	requirement.RequiredBrand = "BrandA"

	result, err := service.Optimize(
		context.Background(), baseRequest(requirement),
	)
	if err != nil {
		t.Fatal(err)
	}

	if !result.NoCompleteBasket {
		t.Fatal("expected no complete basket")
	}

	if len(result.UnmetRequirements) == 0 ||
		result.UnmetRequirements[0].Constraints[0] != FailureBrandMismatch {
		t.Fatalf("binding constraint not reported: %+v",
			result.UnmetRequirements)
	}
}

// ---------------------------------------------------------------------------
// Hard constraints are never traded away
// ---------------------------------------------------------------------------

func TestOptimize_RequiredBrandIsNeverSubstituted(t *testing.T) {
	cheapWrongBrand := listing("l_cheap", "p1", "shopB", 100, 50)
	cheapWrongBrand.Brand = "BrandZ"

	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {
			cheapWrongBrand,
			listing("l_right", "p1", "shopA", 90000, 50),
		},
	}}

	service := newTestService(t, catalogue, &fakeQuoter{perVendorMinor: 1000})

	requirement := need("r1", "p1", 1)
	requirement.RequiredBrand = "BrandA"

	// A budget that only the wrong-brand option could meet. The optimizer
	// must return the expensive correct basket and report the shortfall, not
	// the cheap wrong one.
	request := baseRequest(requirement)
	request.BudgetMinor = 5000

	result, err := service.Optimize(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Variants) != 1 {
		t.Fatalf("variants = %d, want 1", len(result.Variants))
	}

	if result.Variants[0].Lines[0].ListingRef != "l_right" {
		t.Fatalf("substituted a required brand to fit a budget: %s",
			result.Variants[0].Lines[0].ListingRef)
	}

	if result.FitsBudget {
		t.Fatal("reported a budget fit that required breaking a constraint")
	}
}

func TestOptimize_ExactOnlyRefusesAnotherVendorsSameProduct(t *testing.T) {
	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {
			listing("l_other", "p1", "shopB", 100, 50),
			listing("l_named", "p1", "shopA", 90000, 50),
		},
	}}

	service := newTestService(t, catalogue, &fakeQuoter{perVendorMinor: 1000})

	requirement := Requirement{
		Ref: "r1", ListingRef: "l_named", ProductRef: "p1",
		Quantity: 1, Substitution: AllowExactOnly,
	}

	result, err := service.Optimize(
		context.Background(), baseRequest(requirement),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Variants) != 1 ||
		result.Variants[0].Lines[0].ListingRef != "l_named" {
		t.Fatalf("EXACT_ONLY was widened: %+v", result.Variants)
	}
}

func TestOptimize_NoAuthoritativeEquivalenceMeansNoSubstitution(t *testing.T) {
	// A different product with no shared identity: different product ref,
	// different canonical node. Cheaper, similar-sounding, and not offered -
	// because nothing authoritative says the two are interchangeable.
	unrelated := listing("l_unrelated", "p_other", "shopB", 100, 50)
	unrelated.CanonicalNodeRef = "node_something_else"

	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {unrelated, listing("l_real", "p1", "shopA", 90000, 50)},
	}}

	service := newTestService(t, catalogue, &fakeQuoter{perVendorMinor: 1000})

	requirement := need("r1", "p1", 1)
	requirement.Substitution = AllowEquivalentProduct

	result, err := service.Optimize(
		context.Background(), baseRequest(requirement),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Variants) != 1 ||
		result.Variants[0].Lines[0].ListingRef != "l_real" {
		t.Fatalf("substituted an unrelated product: %+v", result.Variants)
	}
}

func TestOptimize_QuantityIsSatisfiedExactlyOrNotAtAll(t *testing.T) {
	// Ten needed, three in stock. A cheaper basket is never produced by
	// quietly delivering three.
	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {listing("l1", "p1", "shopA", 1000, 3)},
	}}

	service := newTestService(t, catalogue, &fakeQuoter{perVendorMinor: 1000})

	result, err := service.Optimize(
		context.Background(), baseRequest(need("r1", "p1", 10)),
	)
	if err != nil {
		t.Fatal(err)
	}

	if !result.NoCompleteBasket {
		t.Fatal("insufficient stock must not be silently under-delivered")
	}

	if result.UnmetRequirements[0].Constraints[0] !=
		FailureInsufficientStock {
		t.Fatalf("constraint = %v", result.UnmetRequirements[0].Constraints)
	}
}

// ---------------------------------------------------------------------------
// Delivery decides the comparison
// ---------------------------------------------------------------------------

// The trade the whole feature exists to find: slightly dearer goods from one
// shop beat slightly cheaper goods from three, because delivery is charged
// per vendor and dwarfs the difference.
func TestOptimize_FewerVendorsWinWhenDeliveryDominates(t *testing.T) {
	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {
			listing("l1_cheap", "p1", "shopA", 1000, 50),
			listing("l1_hub", "p1", "hub", 1100, 50),
		},
		"r2": {
			listing("l2_cheap", "p2", "shopB", 1000, 50),
			listing("l2_hub", "p2", "hub", 1100, 50),
		},
		"r3": {
			listing("l3_cheap", "p3", "shopC", 1000, 50),
			listing("l3_hub", "p3", "hub", 1100, 50),
		},
	}}

	// 30,000 per vendor. Three shops cost 90,000 to deliver against 30,000
	// for one, and the hub is only 300 dearer on goods.
	quoter := &fakeQuoter{perVendorMinor: 30000}

	service := newTestService(t, catalogue, quoter)

	result, err := service.Optimize(context.Background(), baseRequest(
		need("r1", "p1", 1), need("r2", "p2", 1), need("r3", "p3", 1),
	))
	if err != nil {
		t.Fatal(err)
	}

	best := result.Variants[0]

	if best.VendorCount != 1 {
		t.Fatalf("vendor count = %d, want 1; delivery should have decided it",
			best.VendorCount)
	}

	for _, line := range best.Lines {
		if line.BusinessID != "hub" {
			t.Fatalf("expected the single-vendor basket, got %s from %s",
				line.ListingRef, line.BusinessID)
		}
	}

	// And the per-vendor comparison was real: the cheapest-goods basket was
	// evaluated too, and lost on delivered total.
	if len(quoter.requests) < 2 {
		t.Fatalf("only %d basket(s) were priced", len(quoter.requests))
	}
}

func TestOptimize_UndeliverableBasketNeverWinsOnPrice(t *testing.T) {
	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {listing("l1", "p1", "shopA", 1000, 50)},
	}}

	// Nothing can be delivered. The basket must not be reported with a zero
	// delivery cost and a very attractive total.
	service := newTestService(t, catalogue, &fakeQuoter{unserviceable: true})

	result, err := service.Optimize(
		context.Background(), baseRequest(need("r1", "p1", 1)),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Variants) != 0 {
		t.Fatalf("offered an undeliverable basket: %+v", result.Variants)
	}

	if !result.NoCompleteBasket {
		t.Fatal("expected the answer to say no basket could be delivered")
	}

	if !contains(result.BindingConstraints, "NO_DELIVERABLE_BASKET") {
		t.Fatalf("constraints = %v", result.BindingConstraints)
	}
}

// ---------------------------------------------------------------------------
// Quality is not invented
// ---------------------------------------------------------------------------

func TestOptimize_QualityVariantIsNotFabricatedWithoutData(t *testing.T) {
	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {listing("l1", "p1", "shopA", 1000, 50)},
	}}

	service := newTestService(t, catalogue, &fakeQuoter{perVendorMinor: 1000})

	request := baseRequest(need("r1", "p1", 1))
	request.Objectives = []Objective{ObjectiveQualityBalanced}

	result, err := service.Optimize(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	for _, variant := range result.Variants {
		if variant.Objective == ObjectiveQualityBalanced {
			t.Fatal("produced a quality-ranked variant with no quality data")
		}
	}

	if result.QualityVariantUnavailableReason == "" {
		t.Fatal("an absent quality variant must say why it is absent")
	}

	if !strings.HasPrefix(
		result.QualityVariantUnavailableReason,
		"NO_AUTHORITATIVE_QUALITY_SIGNAL",
	) {
		t.Fatalf("reason = %q", result.QualityVariantUnavailableReason)
	}
}

func TestOptimize_QualityConstraintIsHonouredWhereDataExists(t *testing.T) {
	// A quality GRADE the catalogue states is checkable, even though a
	// quality RANKING is not. The constraint is supported; the variant is
	// not, and the two are kept apart deliberately.
	graded := listing("l_graded", "p1", "shopA", 90000, 50)
	graded.QualityGrades = []string{"GRADE_A", "GRADE_B"}

	ungraded := listing("l_plain", "p1", "shopB", 100, 50)

	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
		"r1": {ungraded, graded},
	}}

	service := newTestService(t, catalogue, &fakeQuoter{perVendorMinor: 1000})

	requirement := need("r1", "p1", 1)
	requirement.MinimumQualityGrade = "GRADE_A"

	result, err := service.Optimize(
		context.Background(), baseRequest(requirement),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Variants) != 1 ||
		result.Variants[0].Lines[0].ListingRef != "l_graded" {
		t.Fatalf("quality floor was not honoured: %+v", result.Variants)
	}
}

// ---------------------------------------------------------------------------
// Determinism and bounds
// ---------------------------------------------------------------------------

func TestOptimize_IsDeterministic(t *testing.T) {
	build := func() (*Service, Request) {
		catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{
			"r1": {
				listing("l1_a", "p1", "shopA", 1000, 50),
				listing("l1_b", "p1", "shopB", 1000, 50),
			},
			"r2": {
				listing("l2_a", "p2", "shopA", 1200, 50),
				listing("l2_b", "p2", "shopB", 1200, 50),
			},
		}}

		service := newTestService(
			t, catalogue, &fakeQuoter{perVendorMinor: 20000},
		)

		return service, baseRequest(need("r1", "p1", 1), need("r2", "p2", 1))
	}

	var previous []string

	// Identical prices across two vendors is exactly the case where map
	// iteration order would leak into the answer if anything ranged over one.
	for attempt := 0; attempt < 8; attempt++ {
		service, request := build()

		result, err := service.Optimize(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}

		var fingerprints []string

		for _, variant := range result.Variants {
			fingerprints = append(fingerprints, variant.Fingerprint)
		}

		if previous == nil {
			previous = fingerprints

			continue
		}

		if len(previous) != len(fingerprints) {
			t.Fatalf("variant count changed between runs: %d vs %d",
				len(previous), len(fingerprints))
		}

		for index := range previous {
			if previous[index] != fingerprints[index] {
				t.Fatalf("non-deterministic result at %d", index)
			}
		}
	}
}

func TestOptimize_SearchIsBounded(t *testing.T) {
	catalogue := &fakeCatalogue{byRequirement: map[string][]Listing{}}

	// Twenty vendors stocking every one of eight requirements. Exhaustive
	// enumeration would be 20^8 baskets; the bound is what stands between one
	// HTTP request and a server.
	for index := 0; index < 8; index++ {
		ref := "r" + itoa(index)

		for vendor := 0; vendor < 20; vendor++ {
			shop := "shop" + itoa(vendor)

			catalogue.byRequirement[ref] = append(
				catalogue.byRequirement[ref],
				listing(ref+"_"+shop, "p"+ref, shop,
					int64(1000+vendor), 100),
			)
		}
	}

	quoter := &fakeQuoter{perVendorMinor: 10000}

	service := newTestService(t, catalogue, quoter)

	var requirements []Requirement

	for index := 0; index < 8; index++ {
		requirements = append(
			requirements, need("r"+itoa(index), "p"+"r"+itoa(index), 1),
		)
	}

	result, err := service.Optimize(
		context.Background(), baseRequest(requirements...),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(quoter.requests) > MaxEvaluatedBaskets {
		t.Fatalf("evaluated %d baskets, bound is %d",
			len(quoter.requests), MaxEvaluatedBaskets)
	}

	if result.EvaluatedBaskets > MaxEvaluatedBaskets {
		t.Fatalf("reported %d evaluated baskets", result.EvaluatedBaskets)
	}

	// Truncation is disclosed, not hidden. An optimizer that silently stops
	// early and presents its answer as the best available is claiming a
	// thoroughness it does not have.
	if !result.SearchTruncated {
		t.Fatal("a truncated search must say so")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
