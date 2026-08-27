package fulfilment

// Candidate construction, the mandatory splits, and the three modes.
//
// Every test here asks one of the questions Support actually receives, and
// asserts that the plan can answer it: why these vendors were combined, why
// the order was split, why the faster option cost more, why the cheaper
// option could not combine these pickups.
//
// The mandatory-split tests all share one shape on purpose. Each makes a
// consolidation that is attractive on price, and each asserts that the
// optimizer refuses it as INELIGIBLE rather than merely ranking it lower. The
// distinction is the whole point: a rejected candidate is one the optimizer
// weighed, and a safety limit must never be something it weighs.

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/commercequote"
)

// ---------------------------------------------------------------------------
// Two co-located vendors, quoted per route
// ---------------------------------------------------------------------------

// twoVendorHarness places a second vendor at a chosen distance from the first
// and lets each quoted route be priced and timed independently.
//
// Independent pricing is what makes the mode tests real. A fake that answered
// the same fee for every route could not distinguish "SAVER chose the cheaper
// plan" from "SAVER chose the only plan", and both would pass.
type routeQuote struct {
	feeMinor int64
	minutes  int
}

func twoVendorHarness(
	t *testing.T,
	secondLat, secondLng float64,
	routes map[string]routeQuote,
) *harness {
	t.Helper()

	h := newHarness()

	h.branches.branches["bizB|brB"] = activeBranch(secondLat, secondLng)
	h.logistics.profiles["l2"] = LineProfile{
		Known: true, WeightGrams: ptr64(1500),
	}

	// Both authorities answer from the same route table. A load heavy enough,
	// cold enough or fragile enough to need a van is quoted by hauling rather
	// than dispatch, and a table wired to only one of them would make those
	// scenarios silently fall back to a fixed fee.
	resolve := func(request commercequote.Request) commercequote.Quote {
		route, ok := routes[request.QuoteReference]

		if !ok {
			t.Fatalf("no route configured for %q", request.QuoteReference)
		}

		quote := serviceableQuote("q_"+request.QuoteReference, route.feeMinor)
		quote.EstimatedMinutes = route.minutes

		return quote
	}

	h.dispatch.resolve = resolve
	h.hauling.resolve = resolve

	return h
}

// vanLoad is a line profile heavy enough that no motorcycle may carry it.
//
// The mandatory-split tests need both vendors on the SAME vehicle class,
// because two groups needing different classes are refused for that reason
// alone and the rule under test is never reached. Pinning both to a van is
// how each test ends up actually exercising temperature, fragility or
// capacity rather than a class mismatch.
func vanLoad(grams int64) LineProfile {
	return LineProfile{Known: true, WeightGrams: ptr64(grams)}
}

// Reference keys the planner derives for the three routes in a two-vendor
// checkout. Stating them here rather than recomputing keeps the fakes honest:
// if the reference scheme changes, these tests fail loudly instead of
// silently quoting the wrong route.
const (
	routeG1           = "vfq_cs_1_g1"
	routeG2           = "vfq_cs_1_g2"
	routeConsolidated = "vfq_cs_1_g1_g2"
)

func groupB() GroupInput {
	return GroupInput{
		CheckoutGroupID: "g2",
		BusinessID:      "bizB",
		BranchID:        "brB",
		OrderValueMinor: 300000,
		Currency:        "NGN",
		Lines: []LineLogistics{{
			CheckoutLineID: "l2", Domain: "SHOP",
			BusinessID: "bizB", BranchID: "brB", Quantity: 1,
		}},
	}
}

// coLocated is ~50 m from branchLat/branchLng: genuinely one collection point.
const (
	coLocatedLat = 6.45505
	coLocatedLng = 3.41030
)

// separatePickup is ~1.1 km away: close enough to be worth evaluating as one
// mission, far enough that it is unmistakably two stops.
const (
	separateLat = 6.4650
	separateLng = 3.4100
)

func planWithMode(
	t *testing.T,
	h *harness,
	mode Mode,
	groups ...GroupInput,
) Plan {
	t.Helper()

	req := request(groups...)
	req.Mode = mode

	plan, _, err := h.planner(t).Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	return plan
}

func candidateOfKind(
	t *testing.T,
	plan Plan,
	kind CandidateKind,
) PlanCandidate {
	t.Helper()

	for _, candidate := range plan.Candidates {
		if candidate.Kind == kind {
			return candidate
		}
	}

	t.Fatalf("no %s candidate in plan; candidates=%+v", kind, plan.Candidates)

	return PlanCandidate{}
}

func assertSelected(t *testing.T, plan Plan, kind CandidateKind) {
	t.Helper()

	selected, ok := plan.SelectedCandidate()

	if !ok {
		t.Fatalf("no candidate selected; candidates=%+v", plan.Candidates)
	}

	if selected.Kind != kind {
		t.Fatalf("selected %s, want %s (reason %s)",
			selected.Kind, kind, selected.ReasonCode)
	}
}

func assertIneligible(
	t *testing.T,
	plan Plan,
	kind CandidateKind,
	reason string,
) {
	t.Helper()

	candidate := candidateOfKind(t, plan, kind)

	if candidate.Status != StatusCandidateIneligible {
		t.Fatalf("%s status = %s (reason %s), want INELIGIBLE",
			kind, candidate.Status, candidate.ReasonCode)
	}

	if candidate.ReasonCode != reason {
		t.Fatalf("%s reason = %s, want %s",
			kind, candidate.ReasonCode, reason)
	}
}

// ---------------------------------------------------------------------------
// The mode carries through
// ---------------------------------------------------------------------------

func TestPlan_DefaultModeIsBalanced(t *testing.T) {
	h := newHarness()

	plan, _, err := h.planner(t).Build(context.Background(), request(groupA()))
	if err != nil {
		t.Fatal(err)
	}

	if plan.Mode != ModeBalanced {
		t.Fatalf("mode = %s, want BALANCED", plan.Mode)
	}

	// The version is what makes a stored explanation stay an explanation
	// after the thresholds move.
	if plan.OptimizationPolicyVersion != DefaultOptimizationPolicyVersion {
		t.Fatalf("policy version = %d, want %d",
			plan.OptimizationPolicyVersion, DefaultOptimizationPolicyVersion)
	}
}

func TestParseMode(t *testing.T) {
	for _, tc := range []struct {
		in    string
		want  Mode
		valid bool
	}{
		{"", ModeBalanced, true},
		{"fastest", ModeFastest, true},
		{"  SAVER ", ModeSaver, true},
		{"BALANCED", ModeBalanced, true},

		// SINGLE is a historical internal value, not a customer choice.
		{"SINGLE", "", false},

		// An unknown mode is refused rather than downgraded. Silently
		// planning BALANCED for a customer who asked for something else
		// would hide a client bug behind a plausible plan.
		{"CHEAPEST", "", false},
	} {
		got, valid := ParseMode(tc.in)

		if valid != tc.valid || got != tc.want {
			t.Fatalf("ParseMode(%q) = (%q,%v), want (%q,%v)",
				tc.in, got, valid, tc.want, tc.valid)
		}
	}
}

func TestPlan_ModeFlowsIntoThePlanThatIsStored(t *testing.T) {
	h := newHarness()

	plan := planWithMode(t, h, ModeSaver, groupA())

	if plan.Mode != ModeSaver {
		t.Fatalf("mode = %s, want SAVER", plan.Mode)
	}

	// commerce_quotes.selected_fulfilment_mode is read straight off this
	// column, so a plan quoted as SAVER cannot later be executed as FASTEST
	// against a charge the customer never approved.
	for _, candidate := range plan.Candidates {
		if candidate.Mode != ModeSaver {
			t.Fatalf("candidate mode = %s, want SAVER", candidate.Mode)
		}
	}
}

// ---------------------------------------------------------------------------
// Alternatives are kept, with reasons
// ---------------------------------------------------------------------------

func TestPlan_BothAlternativesArePersistedWithReasons(t *testing.T) {
	h := twoVendorHarness(t, coLocatedLat, coLocatedLng, map[string]routeQuote{
		routeG1:           {feeMinor: 150000, minutes: 26},
		routeG2:           {feeMinor: 140000, minutes: 24},
		routeConsolidated: {feeMinor: 180000, minutes: 28},
	})

	plan := planWithMode(t, h, ModeBalanced, groupA(), groupB())

	if len(plan.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(plan.Candidates))
	}

	for _, candidate := range plan.Candidates {
		if !KnownCandidateReason(candidate.ReasonCode) {
			t.Fatalf("candidate carries a reason outside the controlled "+
				"vocabulary: %q", candidate.ReasonCode)
		}

		// The invariant the database also enforces: a hard refusal can only
		// appear on an INELIGIBLE row. Anything else would record a safety
		// limit as a price decision.
		if HardCandidateReason(candidate.ReasonCode) &&
			candidate.Status != StatusCandidateIneligible {
			t.Fatalf("hard reason %q on %s candidate",
				candidate.ReasonCode, candidate.Status)
		}

		if candidate.Fingerprint == "" {
			t.Fatal("candidate has no fingerprint")
		}

		if candidate.PolicyVersion != DefaultOptimizationPolicyVersion {
			t.Fatalf("candidate policy version = %d",
				candidate.PolicyVersion)
		}
	}

	// The split alternative is the baseline, so it never records a detour
	// against itself.
	split := candidateOfKind(t, plan, CandidateSplitParallel)

	if split.DetourMinutes != 0 {
		t.Fatalf("split detour = %d, want 0", split.DetourMinutes)
	}

	consolidated := candidateOfKind(t, plan, CandidateConsolidated)

	// 28 minutes for the combined route against a 26-minute slowest split leg.
	if consolidated.DetourMinutes != 2 {
		t.Fatalf("consolidated detour = %d, want 2",
			consolidated.DetourMinutes)
	}
}

func TestPlan_IdenticalInputsProduceIdenticalCandidates(t *testing.T) {
	routes := map[string]routeQuote{
		routeG1:           {feeMinor: 150000, minutes: 26},
		routeG2:           {feeMinor: 140000, minutes: 24},
		routeConsolidated: {feeMinor: 180000, minutes: 28},
	}

	first := planWithMode(t,
		twoVendorHarness(t, coLocatedLat, coLocatedLng, routes),
		ModeBalanced, groupA(), groupB(),
	)

	second := planWithMode(t,
		twoVendorHarness(t, coLocatedLat, coLocatedLng, routes),
		ModeBalanced, groupA(), groupB(),
	)

	if len(first.Candidates) != len(second.Candidates) {
		t.Fatalf("candidate counts differ: %d vs %d",
			len(first.Candidates), len(second.Candidates))
	}

	for index := range first.Candidates {
		a, b := first.Candidates[index], second.Candidates[index]

		if a.Fingerprint != b.Fingerprint ||
			a.Status != b.Status ||
			a.ReasonCode != b.ReasonCode {
			t.Fatalf("non-deterministic candidate at %d: %+v vs %+v",
				index, a, b)
		}
	}
}

// ---------------------------------------------------------------------------
// The three modes choose differently on the same facts
// ---------------------------------------------------------------------------

// The scenario is one set of physical facts. Only the customer's stated
// preference differs between the next three tests, and the plans differ
// because of it - which is the entire claim the three modes make.
func modeScenarioRoutes() map[string]routeQuote {
	return map[string]routeQuote{
		// Two separate missions, run in parallel: the last one lands at 24
		// minutes, and they cost 290,000 minor between them.
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 20},

		// One combined mission: 120,000 minor cheaper, two minutes slower.
		//
		// Two minutes is inside every mode detour tolerance, so all three
		// modes get to make a real choice here rather than one of them being
		// forced by a constraint. That is what makes the next three tests a
		// comparison of preferences instead of a comparison of gates.
		routeConsolidated: {feeMinor: 170000, minutes: 26},
	}
}

func TestPlan_FastestChoosesParallelEvenWhenConsolidationIsCheaper(t *testing.T) {
	h := twoVendorHarness(t,
		coLocatedLat, coLocatedLng, modeScenarioRoutes(),
	)

	plan := planWithMode(t, h, ModeFastest, groupA(), groupB())

	assertSelected(t, plan, CandidateSplitParallel)

	if len(plan.Missions) != 2 {
		t.Fatalf("missions = %d, want 2 parallel", len(plan.Missions))
	}

	// The consolidated alternative did not lose on arithmetic. It lost
	// because the customer chose the faster parallel service under a quote
	// that covers it, and saying "costlier" would misattribute their decision
	// to ours.
	consolidated := candidateOfKind(t, plan, CandidateConsolidated)

	if consolidated.ReasonCode != ReasonCustomerFastestParallel {
		t.Fatalf("consolidated reason = %s, want %s",
			consolidated.ReasonCode, ReasonCustomerFastestParallel)
	}
}

func TestPlan_SaverChoosesConsolidationOnTheSameFacts(t *testing.T) {
	h := twoVendorHarness(t,
		coLocatedLat, coLocatedLng, modeScenarioRoutes(),
	)

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertSelected(t, plan, CandidateConsolidated)

	if len(plan.Missions) != 1 {
		t.Fatalf("missions = %d, want 1 consolidated", len(plan.Missions))
	}

	split := candidateOfKind(t, plan, CandidateSplitParallel)

	if split.ReasonCode != ReasonCandidateCostlier {
		t.Fatalf("split reason = %s, want %s",
			split.ReasonCode, ReasonCandidateCostlier)
	}
}

func TestPlan_BalancedConsolidatesWhenTheTimeCostIsImmaterial(t *testing.T) {
	h := twoVendorHarness(t, coLocatedLat, coLocatedLng, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 20},

		// Meaningfully cheaper, and only three minutes slower - inside
		// BALANCED's materiality threshold, so the time is not a real cost.
		routeConsolidated: {feeMinor: 200000, minutes: 27},
	})

	plan := planWithMode(t, h, ModeBalanced, groupA(), groupB())

	assertSelected(t, plan, CandidateConsolidated)
}

func TestPlan_BalancedSplitsWhenConsolidationCostsRealTime(t *testing.T) {
	h := twoVendorHarness(t, coLocatedLat, coLocatedLng, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 20},

		// Free delivery, and sixteen minutes slower than the split
		// alternative. BALANCED refuses the detour OUTRIGHT rather than
		// ranking it lower on price: past the tolerance it is not a cheaper
		// option, it is an option the customer preference excludes.
		routeConsolidated: {feeMinor: 0, minutes: 40},
	})

	plan := planWithMode(t, h, ModeBalanced, groupA(), groupB())

	assertIneligible(t, plan,
		CandidateConsolidated, ReasonCandidateETADetourExceeded,
	)

	assertSelected(t, plan, CandidateSplitParallel)
}

func TestPlan_SaverAcceptsADetourBalancedRefuses(t *testing.T) {
	routes := map[string]routeQuote{
		routeG1:           {feeMinor: 150000, minutes: 24},
		routeG2:           {feeMinor: 140000, minutes: 20},
		routeConsolidated: {feeMinor: 100000, minutes: 45},
	}

	// Twenty-one minutes of detour. BALANCED tolerates twelve and refuses;
	// SAVER tolerates thirty-five and accepts. Identical physical facts,
	// different accepted patience - which is the only thing a mode is
	// permitted to change.
	balanced := planWithMode(t,
		twoVendorHarness(t, coLocatedLat, coLocatedLng, routes),
		ModeBalanced, groupA(), groupB(),
	)

	assertIneligible(t, balanced,
		CandidateConsolidated, ReasonCandidateETADetourExceeded,
	)

	saver := planWithMode(t,
		twoVendorHarness(t, coLocatedLat, coLocatedLng, routes),
		ModeSaver, groupA(), groupB(),
	)

	assertSelected(t, saver, CandidateConsolidated)
}

// ---------------------------------------------------------------------------
// Mandatory splits
// ---------------------------------------------------------------------------

func TestPlan_CapacityExceededForcesASplit(t *testing.T) {
	h := twoVendorHarness(t, coLocatedLat, coLocatedLng, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 24},
	})

	// Thirteen kilograms each. Either fits a motorcycle on its own; together
	// they are twenty-six and the platform motorcycle envelope is
	// twenty-five. Consolidating them is exactly how a mission gets quoted
	// and then cannot be filled.
	h.logistics.profiles["l1"] = vanLoad(13000)
	h.logistics.profiles["l2"] = vanLoad(13000)

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertIneligible(t, plan,
		CandidateConsolidated, ReasonCandidateCapacityExceeded,
	)

	assertSelected(t, plan, CandidateSplitParallel)
}

func TestPlan_TemperatureConflictForcesASplit(t *testing.T) {
	h := twoVendorHarness(t, coLocatedLat, coLocatedLng, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 24},
	})

	// Chilled beside frozen: two actively controlled bands that cannot share
	// one compartment.
	//
	// Both loads are heavy enough to need a van, so they agree on class and
	// the temperature rule is what actually decides. A motorcycle-sized load
	// beside a van-sized one would be refused for the class mismatch first,
	// and this test would prove nothing.
	//
	// Hot beside frozen is refused too, but for URGENCY first - prepared hot
	// food is immediate and a frozen consignment is not, so the two never
	// reach the temperature rule. Both are mandatory splits; this test pins
	// the temperature one, and the next pins the urgency one.
	h.logistics.profiles["l1"] = LineProfile{
		Known: true, WeightGrams: ptr64(40000), TemperatureClass: "CHILLED",
	}
	h.logistics.profiles["l2"] = LineProfile{
		Known: true, WeightGrams: ptr64(40000), TemperatureClass: "FROZEN",
	}

	// SAVER is the mode most motivated to combine these, which is exactly why
	// the test uses it: a preference must not be able to buy its way past a
	// temperature rule.
	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertIneligible(t, plan,
		CandidateConsolidated, ReasonCandidateTemperatureConflict,
	)

	assertSelected(t, plan, CandidateSplitParallel)
}

func TestPlan_HotFoodIsNotMadeToTravelWithAStandardLoad(t *testing.T) {
	h := twoVendorHarness(t, coLocatedLat, coLocatedLng, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 24},
	})

	// Prepared hot food degrades while it waits, which is what makes it
	// IMMEDIATE. Consolidating it with a standard consignment would either
	// make the meal wait or drag the consignment into express pricing, and
	// neither is something a cheaper fee may buy.
	h.logistics.profiles["l1"] = LineProfile{
		Known: true, WeightGrams: ptr64(40000), TemperatureClass: "HOT",
	}
	h.logistics.profiles["l2"] = vanLoad(40000)

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertIneligible(t, plan,
		CandidateConsolidated, ReasonCandidateUrgencyConflict,
	)

	assertSelected(t, plan, CandidateSplitParallel)
}

func TestPlan_FragileUnderHeavyLoadForcesASplit(t *testing.T) {
	h := twoVendorHarness(t, coLocatedLat, coLocatedLng, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 24},
	})

	// A fragile consignment beside thirty kilograms of something else. Each
	// is ordinary work alone; consolidation is what puts one under the other.
	h.logistics.profiles["l1"] = LineProfile{
		Known: true, WeightGrams: ptr64(500), Fragile: true,
	}
	h.logistics.profiles["l2"] = vanLoad(30000)

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertIneligible(t, plan,
		CandidateConsolidated, ReasonCandidateFragileUnsafe,
	)
}

func TestPlan_FragileWithLightCoLoadStillConsolidates(t *testing.T) {
	h := twoVendorHarness(t, coLocatedLat, coLocatedLng, map[string]routeQuote{
		routeG1:           {feeMinor: 150000, minutes: 24},
		routeG2:           {feeMinor: 140000, minutes: 24},
		routeConsolidated: {feeMinor: 170000, minutes: 25},
	})

	// Fragility on its own is not a refusal. A single crate of eggs beside a
	// light parcel is ordinary work, and a rule that refused it would refuse
	// most food deliveries.
	//
	// Both are marked as large loads so they need the same van class as the
	// fragile carry does; the weight beside the fragile part is what the rule
	// is about, and here there is barely any.
	h.logistics.profiles["l1"] = LineProfile{
		Known: true, WeightGrams: ptr64(500), Fragile: true,
	}
	h.logistics.profiles["l2"] = LineProfile{
		Known: true, WeightGrams: ptr64(800), LargeLoad: true,
	}

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertSelected(t, plan, CandidateConsolidated)
}

func TestPlan_ReadyWindowToleranceIsModeSpecific(t *testing.T) {
	ready := time.Now().UTC().Add(30 * time.Minute)
	late := ready.Add(20 * time.Minute)

	routes := map[string]routeQuote{
		routeG1:           {feeMinor: 150000, minutes: 24},
		routeG2:           {feeMinor: 140000, minutes: 24},
		routeConsolidated: {feeMinor: 170000, minutes: 25},
	}

	withTiming := func(group GroupInput, from time.Time) GroupInput {
		group.Timing = TimingProfile{ReadyFrom: &from}

		return group
	}

	// FASTEST will not make one vendor wait twenty minutes for another.
	fast := planWithMode(t,
		twoVendorHarness(t, coLocatedLat, coLocatedLng, routes),
		ModeFastest,
		withTiming(groupA(), ready), withTiming(groupB(), late),
	)

	assertIneligible(t, fast,
		CandidateConsolidated, ReasonCandidateReadyWindowConflict,
	)

	// SAVER will, because the customer accepted a wider window. The physical
	// facts are identical; only the accepted patience differs, which is the
	// one thing a mode is allowed to change.
	saver := planWithMode(t,
		twoVendorHarness(t, coLocatedLat, coLocatedLng, routes),
		ModeSaver,
		withTiming(groupA(), ready), withTiming(groupB(), late),
	)

	assertSelected(t, saver, CandidateConsolidated)
}

func TestPlan_ClosedHandoverWindowForcesASplitInEveryMode(t *testing.T) {
	ready := time.Now().UTC().Add(30 * time.Minute)
	closes := ready.Add(5 * time.Minute)
	late := ready.Add(20 * time.Minute)

	h := twoVendorHarness(t, coLocatedLat, coLocatedLng, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 24},
	})

	first := groupA()
	first.Timing = TimingProfile{ReadyFrom: &ready, ReadyUntil: &closes}

	second := groupB()
	second.Timing = TimingProfile{ReadyFrom: &late}

	// The mission cannot depart until the later vendor is ready, by which
	// time the earlier vendor's handover window has closed. That is not slow,
	// it is impossible, so no tolerance widens it - not even SAVER's.
	plan := planWithMode(t, h, ModeSaver, first, second)

	assertIneligible(t, plan,
		CandidateConsolidated, ReasonCandidateReadyWindowConflict,
	)
}

func TestPlan_ConsolidationBreakingAPromisedETAIsRefused(t *testing.T) {
	ready := time.Now().UTC().Add(10 * time.Minute)
	promised := ready.Add(30 * time.Minute)

	h := twoVendorHarness(t, coLocatedLat, coLocatedLng, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 25},
		routeG2: {feeMinor: 140000, minutes: 25},

		// Twenty-eight minutes is inside SAVER's detour tolerance and still
		// arrives after the promise. A promise the customer was given is not
		// a threshold a mode may trade away, which is why the ETA slack is
		// zero in every mode.
		routeConsolidated: {feeMinor: 100000, minutes: 45},
	})

	first := groupA()
	first.Timing = TimingProfile{ReadyFrom: &ready, PromisedBy: &promised}

	second := groupB()
	second.Timing = TimingProfile{ReadyFrom: &ready}

	plan := planWithMode(t, h, ModeSaver, first, second)

	assertIneligible(t, plan,
		CandidateConsolidated, ReasonCandidateETADetourExceeded,
	)

	assertSelected(t, plan, CandidateSplitParallel)
}

// ---------------------------------------------------------------------------
// THE MULTI-STOP GATE
//
// This is the live limitation and the most consequential rule in the section.
// Neither dispatch nor hauling models a multi-pickup mission. A consolidated
// candidate spanning two separate collection points is therefore not
// executable, and submitting one would produce a fee for a route nobody can
// drive.
//
// The two tests below are a pair, and the pair is the point: the same
// consolidation is refused when it needs two stops and permitted when it
// needs one. Getting either half wrong is silent - one destroys a real
// optimization, the other prices an undrivable route.
// ---------------------------------------------------------------------------

func TestPlan_SeparatePickupsAreRefusedWhileMultiStopIsUnsupported(t *testing.T) {
	h := twoVendorHarness(t, separateLat, separateLng, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 24},
	})

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertIneligible(t, plan,
		CandidateConsolidated, ReasonCandidateMultiStopUnsupported,
	)

	// The executable split alternative is produced and selected. A missing
	// runtime capability must cost the customer a consolidation, never a
	// delivery.
	assertSelected(t, plan, CandidateSplitParallel)

	if len(plan.Missions) != 2 {
		t.Fatalf("missions = %d, want 2", len(plan.Missions))
	}

	if plan.Status != StatusPlanned {
		t.Fatalf("status = %s, want PLANNED", plan.Status)
	}

	consolidated := candidateOfKind(t, plan, CandidateConsolidated)

	if !consolidated.MultiStopRequired {
		t.Fatal("multi-stop refusal recorded without multi_stop_required")
	}

	// The capability evidence has to survive with the decision. Without it,
	// reading MULTI_STOP_UNSUPPORTED back in a year is an assertion with
	// nothing behind it.
	if consolidated.ServiceCapabilities[ServiceDispatch] {
		t.Fatal("dispatch recorded as multi-pickup capable")
	}

	// And no fee was ever requested for the undrivable route. Asking an
	// authority to price a route it cannot drive spends a real call to get a
	// number that must be thrown away.
	for _, quoted := range h.dispatch.requests {
		if quoted.QuoteReference == routeConsolidated {
			t.Fatal("quoted a multi-stop route the runtime cannot drive")
		}
	}
}

func TestPlan_CoLocatedPickupsAreOneStopAndStillConsolidate(t *testing.T) {
	h := twoVendorHarness(t, coLocatedLat, coLocatedLng, map[string]routeQuote{
		routeG1:           {feeMinor: 150000, minutes: 24},
		routeG2:           {feeMinor: 140000, minutes: 24},
		routeConsolidated: {feeMinor: 170000, minutes: 25},
	})

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertSelected(t, plan, CandidateConsolidated)

	consolidated := candidateOfKind(t, plan, CandidateConsolidated)

	if consolidated.MultiStopRequired {
		t.Fatal("two vendors 50 m apart classified as a multi-stop mission")
	}

	if len(consolidated.Legs) != 1 ||
		consolidated.Legs[0].PickupStopCount != 1 {
		t.Fatalf("expected one leg with one stop, got %+v", consolidated.Legs)
	}
}

func TestPlan_ConsolidationOpensWhenTheRuntimeGainsMultiPickup(t *testing.T) {
	h := twoVendorHarness(t, separateLat, separateLng, map[string]routeQuote{
		routeG1:           {feeMinor: 150000, minutes: 24},
		routeG2:           {feeMinor: 140000, minutes: 24},
		routeConsolidated: {feeMinor: 170000, minutes: 26},
	})

	h.multiPickup = map[Service]bool{ServiceDispatch: true}

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	// The candidate the previous test refuses becomes executable with no
	// other change: the gate is read from capability data rather than
	// hardcoded, which is what makes the limitation temporary rather than
	// structural.
	assertSelected(t, plan, CandidateConsolidated)

	consolidated := candidateOfKind(t, plan, CandidateConsolidated)

	if !consolidated.MultiStopRequired {
		t.Fatal("two separate pickups should still be a multi-stop mission")
	}

	if consolidated.Legs[0].PickupStopCount != 2 {
		t.Fatalf("pickup stops = %d, want 2",
			consolidated.Legs[0].PickupStopCount)
	}
}

func TestPlan_DistantPickupsAreNotEvenEvaluatedTogether(t *testing.T) {
	// Twelve kilometres apart: beyond the proximity bound, so no consolidated
	// mission is constructed at all. The refusal is still recorded, because
	// "these pickups are nowhere near each other" is a better answer than
	// silence.
	h := twoVendorHarness(t, 6.5600, 3.4100, map[string]routeQuote{
		routeG1: {feeMinor: 150000, minutes: 24},
		routeG2: {feeMinor: 140000, minutes: 24},
	})

	plan := planWithMode(t, h, ModeSaver, groupA(), groupB())

	assertIneligible(t, plan,
		CandidateConsolidated, ReasonCandidateProximityExceeded,
	)

	assertSelected(t, plan, CandidateSplitParallel)
}

// ---------------------------------------------------------------------------
// Single-vendor checkouts
// ---------------------------------------------------------------------------

func TestPlan_SingleVendorOffersNoSecondAlternative(t *testing.T) {
	h := newHarness()

	plan := planWithMode(t, h, ModeBalanced, groupA())

	// One vendor cannot be consolidated with anything. Offering a
	// "consolidated" candidate identical to the split one would be the same
	// plan under two labels.
	if len(plan.Candidates) != 1 {
		t.Fatalf("candidates = %d, want 1; got %+v",
			len(plan.Candidates), plan.Candidates)
	}

	assertSelected(t, plan, CandidateSplitParallel)
}
