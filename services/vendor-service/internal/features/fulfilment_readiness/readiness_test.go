package fulfilment_readiness

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Section 11's readiness rules. Everything here is a pure decision, so these
// need no database: the state machine, the per-category substates, the
// permission model and the predictor are all answerable from their inputs.

func at(hour, minute int) time.Time {
	return time.Date(2026, 9, 21, hour, minute, 0, 0, time.UTC)
}

// ── State machine ───────────────────────────────────────────────────────────

// TestReadinessTransitionMatrix is mandatory test 8, per vendor type.
//
// The matrix is asserted rather than sampled because the interesting entries
// are the ones nobody thinks to try: resuming from BLOCKED into the middle of
// the machine, and the fact that CANCELLED leads nowhere.
func TestReadinessTransitionMatrix(t *testing.T) {
	allowed := []struct{ from, to State }{
		{StateNotStarted, StateAccepted},
		{StateAccepted, StatePreparing},
		{StatePreparing, StateReadySoon},
		{StatePreparing, StateReady},
		{StateReadySoon, StateReady},
		{StateReady, StateDelayed},
		{StateReady, StateBlocked},
		// A blocked order resumes where it was rather than restarting.
		{StateBlocked, StatePreparing},
		{StateBlocked, StateReadySoon},
		{StateBlocked, StateReady},
		{StateDelayed, StatePreparing},
		{StateDelayed, StateReady},
	}
	for _, move := range allowed {
		if !CanTransition(move.from, move.to) {
			t.Errorf("%s -> %s should be allowed", move.from, move.to)
		}
	}

	refused := []struct{ from, to State }{
		// Preparation is not skippable: accepting the order is the moment the
		// branch takes responsibility for it.
		{StateNotStarted, StatePreparing},
		{StateNotStarted, StateReady},
		{StateAccepted, StateReady},
		// Readiness does not run backwards on its own.
		{StateReadySoon, StatePreparing},
		{StateReady, StatePreparing},
		// Cancellation is the end.
		{StateCancelled, StateAccepted},
		{StateCancelled, StateReady},
	}
	for _, move := range refused {
		if CanTransition(move.from, move.to) {
			t.Errorf("%s -> %s should be refused", move.from, move.to)
		}
	}
}

// TestCorrectingAReadyOrderNeedsAReasonAndAPerson is the rule that keeps a
// correction honest.
//
// An order can leave READY, because a vendor who spots a missing item must be
// able to say so rather than stay quiet and let a rider collect an incomplete
// order. What it cannot do is leave READY silently, or on an automatic signal:
// a stray production event must not un-ready an order a person declared ready.
func TestCorrectingAReadyOrderNeedsAReasonAndAPerson(t *testing.T) {
	silent := Move{To: StateDelayed, Source: SourceVendorStaff}
	if err := ValidateMove(StateReady, "", VendorRestaurant, silent); !errors.Is(err, ErrValidation) {
		t.Fatalf("correcting READY with no reason = %v, want a refusal", err)
	}

	automatic := Move{To: StateDelayed, Source: SourceSystem,
		ReasonCode: "ITEM_MISSING", OriginEventRef: "evt-1"}
	if err := ValidateMove(StateReady, "", VendorRestaurant, automatic); !errors.Is(err, ErrForbidden) {
		t.Fatalf("automatic un-ready = %v, want ErrForbidden", err)
	}

	explained := Move{To: StateDelayed, Source: SourceVendorStaff, ReasonCode: "ITEM_MISSING"}
	if err := ValidateMove(StateReady, "", VendorRestaurant, explained); err != nil {
		t.Fatalf("an explained correction by staff was refused: %v", err)
	}
}

// TestSystemTransitionsMustNameTheirOrigin is mandatory test 15's structural
// half: an automatic state change that cannot be traced to a Vendor-owned event
// is one nobody can audit.
func TestSystemTransitionsMustNameTheirOrigin(t *testing.T) {
	anonymous := Move{To: StatePreparing, Source: SourceSystem}
	if err := ValidateMove(StateAccepted, "", VendorRestaurant, anonymous); !errors.Is(err, ErrValidation) {
		t.Fatalf("an unattributed system transition = %v, want a refusal", err)
	}
	attributed := Move{To: StatePreparing, Source: SourceSystem, OriginEventRef: "prod-evt-9"}
	if err := ValidateMove(StateAccepted, "", VendorRestaurant, attributed); err != nil {
		t.Fatalf("an attributed system transition was refused: %v", err)
	}
}

// TestBlockAndDelayRequireAReason keeps an operator surface actionable.
func TestBlockAndDelayRequireAReason(t *testing.T) {
	for _, state := range []State{StateBlocked, StateDelayed} {
		bare := Move{To: state, Source: SourceVendorStaff}
		if err := ValidateMove(StatePreparing, "", VendorRestaurant, bare); !errors.Is(err, ErrValidation) {
			t.Fatalf("%s with no reason = %v, want a refusal", state, err)
		}
	}
}

// ── Vendor-type substates ───────────────────────────────────────────────────

// TestEveryPlatformVendorTypeHasSubstates is mandatory requirement 4.2's real
// content: readiness must work for ALL configured vendor types, not restaurants.
func TestEveryPlatformVendorTypeHasSubstates(t *testing.T) {
	for _, vendorType := range PlatformVendorTypes() {
		substates := SubstatesFor(vendorType)
		if len(substates) == 0 {
			t.Errorf("%s has no substates: readiness would be a single opaque step", vendorType)
		}
		if !vendorType.ConfigurableOnPlatform() {
			t.Errorf("%s is listed as a platform type but reports otherwise", vendorType)
		}
	}
}

// TestSpecifiedButUnconfigurableTypesAreDocumentedNotMissing pins the honest
// gap.
//
// Section 11 names catering and manufacturing substates. The platform's
// capability vocabulary (vendor_capabilities.capability_type) has no value for
// either, so an order can never reach them today. Defining them makes the day
// that vocabulary grows a one-line change; asserting they are NOT configurable
// stops the definition being mistaken for working support.
// Catering and Manufacturer are configurable since migration 000121. This is
// the test that used to pin the gap; it now pins its closure.
func TestCateringAndManufacturerAreConfigurable(t *testing.T) {
	for _, vendorType := range []VendorType{VendorCatering, VendorManufacturer} {
		if len(SubstatesFor(vendorType)) == 0 {
			t.Errorf("%s has no substates defined", vendorType)
		}
		if !vendorType.ConfigurableOnPlatform() {
			t.Errorf("%s is still unreachable: no capability_type admits it", vendorType)
		}
	}
	if len(PlatformVendorTypes()) != 9 {
		t.Fatalf("platform vendor types = %d, want the 9 capability_type values",
			len(PlatformVendorTypes()))
	}
	// Every vendor type is now a platform type; nothing is modelled-but-unreachable.
	if len(AllVendorTypes()) != len(PlatformVendorTypes()) {
		t.Fatalf("all=%d platform=%d: a type is modelled but unreachable",
			len(AllVendorTypes()), len(PlatformVendorTypes()))
	}
}

// TestSubstatesAreRefusedAcrossCategories is mandatory test 9.
//
// The failure it prevents is quiet: a grocery order reported as COOKING is
// accepted by a looser model, shown to staff who do not cook, and folded into a
// baseline it has nothing to do with.
func TestSubstatesAreRefusedAcrossCategories(t *testing.T) {
	if err := ValidateSubstate(VendorShopGrocery, StatePreparing, SubCooking); !errors.Is(err, ErrValidation) {
		t.Fatalf("a grocery order reported as COOKING = %v, want a refusal", err)
	}
	if err := ValidateSubstate(VendorPastryBakery, StatePreparing, SubWeighing); !errors.Is(err, ErrValidation) {
		t.Fatalf("a bakery order reported as WEIGHING = %v, want a refusal", err)
	}
	if err := ValidateSubstate(VendorShopGrocery, StatePreparing, SubPicking); err != nil {
		t.Fatalf("PICKING is a grocery substate: %v", err)
	}
	if err := ValidateSubstate(VendorMarket, StatePreparing, SubWeighing); err != nil {
		t.Fatalf("WEIGHING is a market substate: %v", err)
	}
}

// TestSubstatesOnlyDescribeWorkInProgress refuses a substate on a state where
// no work is happening.
func TestSubstatesOnlyDescribeWorkInProgress(t *testing.T) {
	for _, state := range []State{StateNotStarted, StateAccepted, StateReady, StateCancelled} {
		if err := ValidateSubstate(VendorRestaurant, state, SubCooking); !errors.Is(err, ErrValidation) {
			t.Errorf("%s accepted a substate = %v, want a refusal", state, err)
		}
	}
}

// TestWaitingSubstatesDoNotCountAsBranchProgress keeps the branch's own
// baseline honest.
//
// Time spent waiting for a customer to approve a substitution is not time the
// branch was working. Counting it would make every substitution look like a
// slow picker.
func TestWaitingSubstatesDoNotCountAsBranchProgress(t *testing.T) {
	picking := SubstateProgress(VendorShopGrocery, SubPicking)
	waiting := SubstateProgress(VendorShopGrocery, SubCustomerDecisionWait)
	packing := SubstateProgress(VendorShopGrocery, SubPacking)

	if !(picking < packing) {
		t.Fatalf("progress should increase through the sequence: picking=%v packing=%v", picking, packing)
	}
	if waiting > packing {
		t.Fatalf("a waiting substate reported more progress (%v) than packing (%v)", waiting, packing)
	}
	if !SubCustomerDecisionWait.Waiting() || !SubSubstitutionRequired.Waiting() ||
		!SubCustomerApprovalWait.Waiting() {
		t.Fatal("the three customer-wait substates must all report as waiting")
	}
	if SubPicking.Waiting() || SubCooking.Waiting() {
		t.Fatal("active work must not report as waiting")
	}
}

// ── Permission ──────────────────────────────────────────────────────────────

// TestOnlyVendorStaffAdminOrSystemMayMoveReadiness is mandatory tests 13 and 14.
//
// Neither a customer nor a provider appears in the Source vocabulary at all,
// which is the strongest form of the rule: there is no value they could send.
// What this test pins is the next layer -- that staff are scoped to their own
// business and branches.
func TestOnlyVendorStaffAdminOrSystemMayMoveReadiness(t *testing.T) {
	staff := Actor{Kind: SourceVendorStaff, BusinessID: "biz-1", PermittedBranches: []string{"br-1"}}
	if !staff.MayAct("biz-1", "br-1") {
		t.Fatal("staff should act for their own permitted branch")
	}
	if staff.MayAct("biz-1", "br-2") {
		t.Fatal("staff acted for a branch they do not cover")
	}
	if staff.MayAct("biz-2", "br-1") {
		t.Fatal("staff acted for another business's branch")
	}

	wide := Actor{Kind: SourceVendorStaff, BusinessID: "biz-1", AllBranches: true}
	if !wide.MayAct("biz-1", "br-9") {
		t.Fatal("business-wide staff should act for any of their own branches")
	}
	if wide.MayAct("biz-2", "br-9") {
		t.Fatal("business-wide staff acted for another business")
	}

	system := Actor{Kind: SourceSystem, BusinessID: "biz-1", OriginEventRef: "evt-1"}
	if !system.MayAct("biz-1", "br-1") || system.MayAct("biz-2", "br-1") {
		t.Fatal("a system adapter must stay inside the business that owns the order")
	}

	if (Actor{Kind: Source("CUSTOMER")}).MayAct("biz-1", "br-1") {
		t.Fatal("an unrecognised source may not move readiness")
	}
	if (Actor{Kind: Source("PROVIDER")}).MayAct("biz-1", "br-1") {
		t.Fatal("a provider may not move readiness")
	}
}

// TestSourceVocabularyHasNoCustomerOrProvider is the structural half of the
// same rule.
func TestSourceVocabularyHasNoCustomerOrProvider(t *testing.T) {
	for _, forbidden := range []Source{"CUSTOMER", "PROVIDER", "RIDER", "DRIVER"} {
		if forbidden.Valid() {
			t.Fatalf("%q is a recognised readiness source", forbidden)
		}
	}
	for _, permitted := range []Source{SourceVendorStaff, SourceSystem, SourceAdmin} {
		if !permitted.Valid() {
			t.Fatalf("%q should be a recognised source", permitted)
		}
	}
}

// ── Prediction ──────────────────────────────────────────────────────────────

// TestColdStartPredictionUsesTheCategoryDefault is mandatory test 16.
//
// A brand new branch with no history must still be plannable. The alternative
// is that its first orders cannot be orchestrated at all, which is the worst
// possible day for that to happen.
func TestColdStartPredictionUsesTheCategoryDefault(t *testing.T) {
	now := at(12, 0)
	prediction := Predict(PredictionInput{
		Now: now, VendorType: VendorRestaurant, Rules: DefaultRules(VendorRestaurant),
		State: StatePreparing, PrepStartedAt: &now,
	})
	if prediction.Basis != BasisTypeDefault {
		t.Fatalf("basis = %q, want %q", prediction.Basis, BasisTypeDefault)
	}
	if !prediction.Start.After(now) {
		t.Fatal("a cold-start prediction must still name a future ready time")
	}
	if !prediction.End.After(prediction.Start) {
		t.Fatal("a prediction must be a window, not an instant")
	}
	if prediction.Band != ConfidenceLow {
		t.Fatalf("cold-start confidence = %q, want LOW", prediction.Band)
	}
}

// TestBranchProfileBeatsTheDefaultWhenItIsFreshAndFounded is mandatory test 17.
func TestBranchProfileBeatsTheDefaultWhenItIsFreshAndFounded(t *testing.T) {
	now := at(12, 0)
	profile := BranchProfile{
		BaselineP50: 600, BaselineP80: 900, BaselineP90: 1200,
		Confidence: 0.8, SampleCount: 120, DataAsOf: now.Add(-24 * time.Hour),
	}
	input := PredictionInput{
		Now: now, VendorType: VendorRestaurant, Rules: DefaultRules(VendorRestaurant),
		State: StatePreparing, PrepStartedAt: &now,
		Profile: profile, ProfileFresh: profile.Fresh(now, ProfileMaxAge),
	}
	prediction := Predict(input)
	if prediction.Basis != BasisBranchProfile {
		t.Fatalf("basis = %q, want %q", prediction.Basis, BasisBranchProfile)
	}
	// The profile says ten minutes; the category default says twenty-five.
	if got := prediction.Start.Sub(now); got != 10*time.Minute {
		t.Fatalf("predicted prep = %v, want the branch's own 10m", got)
	}
	// The window comes from the branch's own P80-P50 spread.
	if got := prediction.End.Sub(prediction.Start); got != 5*time.Minute {
		t.Fatalf("window = %v, want the branch's observed 5m spread", got)
	}
}

// TestStaleOrUnfoundedProfilesFallBackSafely is mandatory test 18.
//
// Two ways a profile is not usable, and both must degrade rather than fail. A
// profile from six months ago is specific enough to be trusted and old enough
// to be wrong; one built on three observations moves with a single unusual
// lunchtime.
func TestStaleOrUnfoundedProfilesFallBackSafely(t *testing.T) {
	now := at(12, 0)
	cases := map[string]BranchProfile{
		"too old": {BaselineP50: 600, BaselineP80: 900, SampleCount: 500,
			DataAsOf: now.Add(-60 * 24 * time.Hour)},
		"too few samples": {BaselineP50: 600, BaselineP80: 900, SampleCount: 3,
			DataAsOf: now.Add(-time.Hour)},
	}
	for name, profile := range cases {
		if profile.Fresh(now, ProfileMaxAge) {
			t.Fatalf("%s profile reported fresh", name)
		}
		prediction := Predict(PredictionInput{
			Now: now, VendorType: VendorRestaurant, Rules: DefaultRules(VendorRestaurant),
			State: StatePreparing, PrepStartedAt: &now,
			Profile: profile, ProfileFresh: profile.Fresh(now, ProfileMaxAge),
		})
		if prediction.Basis != BasisTypeDefault {
			t.Fatalf("%s profile basis = %q, want the safe default", name, prediction.Basis)
		}
		if prediction.Start.IsZero() {
			t.Fatalf("%s profile produced no prediction at all", name)
		}
	}
}

// TestVendorEstimateWinsButIsBounded is the vendor-estimate half of the
// prediction ladder, plus the clamp that keeps one typo from moving a branch.
func TestVendorEstimateWinsButIsBounded(t *testing.T) {
	now := at(12, 0)
	rules := DefaultRules(VendorRestaurant)

	reasonable := now.Add(15 * time.Minute)
	prediction := Predict(PredictionInput{
		Now: now, VendorType: VendorRestaurant, Rules: rules,
		State: StatePreparing, PrepStartedAt: &now, VendorEstimatedReadyAt: &reasonable,
	})
	if prediction.Basis != BasisVendorEstimate {
		t.Fatalf("basis = %q, want the vendor's own estimate", prediction.Basis)
	}
	if got := prediction.Start.Sub(now); got != 15*time.Minute {
		t.Fatalf("predicted prep = %v, want the vendor's 15m", got)
	}
	if prediction.Band != ConfidenceHigh {
		t.Fatalf("a vendor estimate should be trusted: band = %q", prediction.Band)
	}

	// Four days is not an estimate.
	absurd := now.Add(96 * time.Hour)
	clamped, wasClamped := rules.ClampVendorEstimate(now, absurd)
	if !wasClamped {
		t.Fatal("an out-of-range vendor estimate was accepted unchanged")
	}
	if clamped.Sub(now) != rules.MaxVendorEstimate {
		t.Fatalf("clamped to %v, want the policy maximum %v", clamped.Sub(now), rules.MaxVendorEstimate)
	}
	tooSoon, wasClamped := rules.ClampVendorEstimate(now, now.Add(time.Second))
	if !wasClamped || tooSoon.Sub(now) != rules.MinVendorEstimate {
		t.Fatalf("an impossibly soon estimate was not raised to the minimum: %v", tooSoon.Sub(now))
	}
}

// TestStaleSignalLowersConfidenceWithoutMovingTheEstimate is mandatory test 21.
//
// A quiet order is probably fine and nobody has said so. Widening the window is
// the honest response; moving the estimate would be inventing information, and
// cancelling would throw away an order that is very likely cooking.
func TestStaleSignalLowersConfidenceWithoutMovingTheEstimate(t *testing.T) {
	now := at(12, 0)
	rules := DefaultRules(VendorRestaurant)
	prepStart := now.Add(-5 * time.Minute)

	fresh := now.Add(-time.Minute)
	quiet := now.Add(-rules.StaleSignal - 20*time.Minute)

	base := PredictionInput{
		Now: now, VendorType: VendorRestaurant, Rules: rules,
		State: StatePreparing, PrepStartedAt: &prepStart,
	}
	base.LastSignalAt = &fresh
	freshPrediction := Predict(base)
	base.LastSignalAt = &quiet
	stalePrediction := Predict(base)

	if !stalePrediction.Start.Equal(freshPrediction.Start) {
		t.Fatalf("a stale signal moved the estimate: fresh=%v stale=%v",
			freshPrediction.Start, stalePrediction.Start)
	}
	if !(stalePrediction.Score < freshPrediction.Score) {
		t.Fatalf("a stale signal did not lower confidence: fresh=%v stale=%v",
			freshPrediction.Score, stalePrediction.Score)
	}
	staleWindow := stalePrediction.End.Sub(stalePrediction.Start)
	freshWindow := freshPrediction.End.Sub(freshPrediction.Start)
	if !(staleWindow > freshWindow) {
		t.Fatalf("a stale signal did not widen the window: fresh=%v stale=%v", freshWindow, staleWindow)
	}
}

// TestPredictionIsDeterministic is what makes the whole approach defensible:
// the same inputs always give the same answer, so a wrong prediction can be
// explained rather than shrugged at.
func TestPredictionIsDeterministic(t *testing.T) {
	now := at(9, 30)
	input := PredictionInput{
		Now: now, VendorType: VendorPastryBakery, Rules: DefaultRules(VendorPastryBakery),
		State: StatePreparing, Substate: SubDecorating, PrepStartedAt: &now, QueueDepth: 3,
	}
	first := Predict(input)
	for i := 0; i < 20; i++ {
		if again := Predict(input); again != first {
			t.Fatalf("prediction %d differed: %+v vs %+v", i, again, first)
		}
	}
}

// TestItemLeadTimeIsAFloorNotAnAverage keeps a quiet branch from promising a
// four-hour cake in twenty minutes.
func TestItemLeadTimeIsAFloorNotAnAverage(t *testing.T) {
	now := at(8, 0)
	prediction := Predict(PredictionInput{
		Now: now, VendorType: VendorPastryBakery, Rules: DefaultRules(VendorPastryBakery),
		State: StatePreparing, PrepStartedAt: &now, ItemLeadTime: 4 * time.Hour,
	})
	if got := prediction.Start.Sub(now); got < 4*time.Hour {
		t.Fatalf("predicted %v for a 4h lead-time item", got)
	}
}

// ── Rules selection ─────────────────────────────────────────────────────────

// TestNarrowestApplicableRuleWins lets a branch be tuned without forking the
// category's defaults.
func TestNarrowestApplicableRuleWins(t *testing.T) {
	typeRule := Rules{VendorType: VendorRestaurant, DefaultPrep: 20 * time.Minute, Version: 1}
	businessRule := Rules{VendorType: VendorRestaurant, BusinessID: "biz-1",
		DefaultPrep: 15 * time.Minute, Version: 1}
	branchRule := Rules{VendorType: VendorRestaurant, BusinessID: "biz-1", BranchID: "br-1",
		DefaultPrep: 9 * time.Minute, Version: 1}

	selected := SelectRules([]Rules{typeRule, businessRule, branchRule},
		VendorRestaurant, "biz-1", "br-1")
	if selected.DefaultPrep != 9*time.Minute {
		t.Fatalf("selected %v, want the branch rule", selected.DefaultPrep)
	}

	// A different branch of the same business falls back to the business rule.
	selected = SelectRules([]Rules{typeRule, businessRule, branchRule},
		VendorRestaurant, "biz-1", "br-2")
	if selected.DefaultPrep != 15*time.Minute {
		t.Fatalf("selected %v, want the business rule", selected.DefaultPrep)
	}

	// A branch with nothing configured is still plannable.
	selected = SelectRules(nil, VendorMarket, "biz-9", "br-9")
	if selected.DefaultPrep <= 0 {
		t.Fatal("an unconfigured scope produced no usable rule")
	}
	if selected.VendorType != VendorMarket {
		t.Fatalf("fallback rule is for %s, want MARKET", selected.VendorType)
	}
}

// TestHigherVersionWinsAtEqualSpecificity pins how a re-issued rule supersedes
// its predecessor.
func TestHigherVersionWinsAtEqualSpecificity(t *testing.T) {
	older := Rules{VendorType: VendorFastFood, BusinessID: "biz-1",
		DefaultPrep: 12 * time.Minute, Version: 1}
	newer := Rules{VendorType: VendorFastFood, BusinessID: "biz-1",
		DefaultPrep: 7 * time.Minute, Version: 2}
	if got := SelectRules([]Rules{older, newer}, VendorFastFood, "biz-1", "br-1"); got.Version != 2 {
		t.Fatalf("selected version %d, want the newer rule", got.Version)
	}
}

// ── No pricing ──────────────────────────────────────────────────────────────

// TestReadinessCarriesNoPricing is mandatory test 38 for this half of the
// section.
//
// Readiness is about time. A fee or a currency here would make a kitchen's prep
// estimate a commercial term, and the first consumer to read it would start
// making money decisions on a number this feature has no authority over.
func TestReadinessCarriesNoPricing(t *testing.T) {
	forbidden := []string{
		"price", "fare", "amount", "minor", "currency", "fee", "tariff",
		"commission", "discount", "surge", "cost", "payable",
	}
	subjects := map[string]any{
		"Readiness":     Readiness{},
		"Prediction":    Prediction{},
		"BranchProfile": BranchProfile{},
		"Rules":         Rules{},
		"HistoryEntry":  HistoryEntry{},
	}
	for name, subject := range subjects {
		for _, field := range fieldNames(subject) {
			lowered := strings.ToLower(field)
			for _, banned := range forbidden {
				if strings.Contains(lowered, banned) {
					t.Errorf("%s.%s looks like a pricing field", name, field)
				}
			}
		}
	}
}

// fieldNames lists a struct's exported field names by reflection, so the
// pricing guard above catches a field added later rather than only the ones
// present when it was written.
func fieldNames(subject any) []string {
	value := reflect.ValueOf(subject)
	if value.Kind() != reflect.Struct {
		return nil
	}
	names := make([]string, 0, value.NumField())
	for i := 0; i < value.NumField(); i++ {
		names = append(names, value.Type().Field(i).Name)
	}
	return names
}
