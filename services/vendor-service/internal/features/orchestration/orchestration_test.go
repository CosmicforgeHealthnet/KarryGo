package orchestration

import (
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	readiness "cosmicforge/logistics/services/vendor-service/internal/features/fulfilment_readiness"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/shared/go/supplypreview"
)

func at(hour, minute int) time.Time {
	return time.Date(2026, 9, 21, hour, minute, 0, 0, time.UTC)
}

// ── Vendor never selects a provider (mandatory tests 1-7) ───────────────────

// providerSelectionFields are the field names an orchestration type must never
// carry.
//
// Naming them here rather than only in the walker means a reviewer adding a
// "preferred rider" field sees the rule beside the test that will reject it.
func providerSelectionFields() []string {
	return []string{
		"provider", "providerid", "provider_id", "providerref",
		"driver", "driverid", "driver_id",
		"rider", "riderid", "rider_id",
		"bike", "bikeid", "bike_id",
		"vehicle", "vehicleid", "vehicle_id",
		"truck", "truckid", "truck_id",
		"fleetmember", "fleet_member", "fleetmemberid", "fleet_member_id",
		"assignee", "assigneeid", "assignee_id",
	}
}

// TestOrchestrationNamesNoProvider is mandatory tests 1 through 7 in one
// structural assertion.
//
// Vendor chooses an AUTHORITY and a PRODUCT. Which driver, rider or vehicle
// executes is resolved by the execution service after Vendor's work is done, so
// there must be nowhere in these types to put one. Walking the structs catches
// a field added next year, which a hand-written list of seven checks would not.
func TestOrchestrationNamesNoProvider(t *testing.T) {
	subjects := map[string]any{
		"Orchestration":  Orchestration{},
		"Requirement":    Requirement{},
		"Pickup":         Pickup{},
		"Candidate":      Candidate{},
		"DecisionEvent":  DecisionEvent{},
		"GroupCandidate": GroupCandidate{},
		"Group":          Group{},
		"CommitInput":    CommitInput{},
		"CommitDecision": CommitDecision{},
	}
	for name, subject := range subjects {
		for _, field := range walkFieldNames(reflect.TypeOf(subject)) {
			lowered := strings.ToLower(field)
			for _, banned := range providerSelectionFields() {
				if lowered == banned {
					t.Errorf("%s carries %q: Vendor must never name a provider", name, field)
				}
			}
		}
	}
}

// TestTheProviderGuardWouldActuallyCatchOne proves the walker works, so a
// passing guard means "no such field" rather than "the walker found nothing".
func TestTheProviderGuardWouldActuallyCatchOne(t *testing.T) {
	type Offending struct {
		FulfilmentGroupRef string
		RiderID            string
	}
	found := false
	for _, field := range walkFieldNames(reflect.TypeOf(Offending{})) {
		if strings.EqualFold(field, "RiderID") {
			found = true
		}
	}
	if !found {
		t.Fatal("the provider-field walker does not detect a planted RiderID")
	}
}

func walkFieldNames(subject reflect.Type) []string {
	if subject == nil || subject.Kind() != reflect.Struct {
		return nil
	}
	names := []string{}
	for i := 0; i < subject.NumField(); i++ {
		field := subject.Field(i)
		names = append(names, field.Name)
		if tag := field.Tag.Get("json"); tag != "" {
			names = append(names, strings.Split(tag, ",")[0])
		}
		if field.Type.Kind() == reflect.Struct {
			names = append(names, walkFieldNames(field.Type)...)
		}
	}
	return names
}

// TestOrchestrationImportsNoPricing is mandatory test 38.
//
// Which authority carries an order is decided by suitability, serviceability,
// supply and policy. A pricing import here would make it possible to decide by
// cost, and the first time that happened it would look like an optimisation.
func TestOrchestrationImportsNoPricing(t *testing.T) {
	fileSet := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return walkErr
		}
		parsed, parseErr := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, spec := range parsed.Imports {
			importPath := strings.ToLower(strings.Trim(spec.Path.Value, `"`))
			for _, banned := range []string{"pricing", "commercequote", "dispatchpricing", "walletclient"} {
				if strings.Contains(importPath, banned) {
					t.Errorf("%s imports %s: authority selection must not read money", path, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// And no field that looks like money.
	for name, subject := range map[string]any{
		"Orchestration": Orchestration{}, "Candidate": Candidate{}, "Requirement": Requirement{},
	} {
		for _, field := range walkFieldNames(reflect.TypeOf(subject)) {
			lowered := strings.ToLower(field)
			for _, banned := range []string{"price", "fare", "minor", "currency", "fee", "tariff", "cost"} {
				if strings.Contains(lowered, banned) {
					t.Errorf("%s.%s looks like a pricing field", name, field)
				}
			}
		}
	}
}

// ── Authority selection ─────────────────────────────────────────────────────

func serviceable(authority logistics.Authority, product string, supply supplypreview.SupplyStatus) Candidate {
	return Candidate{Authority: authority, ProductCode: product,
		Outcome: OutcomeServiceable, SupplyStatus: supply}
}

// TestSmallestAdequateAuthorityWins is mandatory test 25.
//
// The ordering is physical, not commercial: a bike is smaller than a car, a car
// smaller than a truck, and the smallest vehicle that can actually do the job
// is the right one. No candidate is preferred for being cheaper, because
// nothing here can see a price.
func TestSmallestAdequateAuthorityWins(t *testing.T) {
	selected, err := SelectAuthority([]Candidate{
		serviceable(logistics.Hauling, "HAUL_VAN", supplypreview.StatusAvailable),
		serviceable(logistics.Dispatch, "DISPATCH_STANDARD", supplypreview.StatusAvailable),
		serviceable(logistics.Taxi, "TAXI_GOODS_STANDARD", supplypreview.StatusAvailable),
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Authority != logistics.Dispatch {
		t.Fatalf("selected %s, want the smallest adequate authority", selected.Authority)
	}

	// With Dispatch refused on physical grounds, Taxi Goods is next.
	selected, err = SelectAuthority([]Candidate{
		{Authority: logistics.Dispatch, Outcome: OutcomeCapabilityMismatch, ReasonCode: "WEIGHT_EXCEEDED"},
		serviceable(logistics.Taxi, "TAXI_GOODS_LARGE", supplypreview.StatusAvailable),
		serviceable(logistics.Hauling, "HAUL_VAN", supplypreview.StatusAvailable),
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Authority != logistics.Taxi {
		t.Fatalf("selected %s, want Taxi once Dispatch cannot carry it", selected.Authority)
	}

	// With both smaller families refused, Haulage carries it.
	selected, err = SelectAuthority([]Candidate{
		{Authority: logistics.Dispatch, Outcome: OutcomeCapabilityMismatch, ReasonCode: "WEIGHT_EXCEEDED"},
		{Authority: logistics.Taxi, Outcome: OutcomeCapabilityMismatch, ReasonCode: "VOLUME_EXCEEDED"},
		serviceable(logistics.Hauling, "HAUL_TRUCK", supplypreview.StatusAvailable),
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Authority != logistics.Hauling {
		t.Fatalf("selected %s, want Haulage", selected.Authority)
	}
}

// TestDependencyOutageIsNotACapabilityMismatch is mandatory test 23, and the
// distinction the whole selection rests on.
//
// An authority that never answered has said nothing about the goods. Treating
// that as a refusal would let one timeout become a standing belief that a city
// is unserved, and nobody would ever ask again.
func TestDependencyOutageIsNotACapabilityMismatch(t *testing.T) {
	if OutcomeDependencyUnavailable.PositiveIncompatibility() {
		t.Fatal("an unreachable authority must not count as a refusal")
	}
	if !OutcomeCapabilityMismatch.PositiveIncompatibility() {
		t.Fatal("a capability mismatch is a refusal")
	}

	// Nothing usable, and at least one authority unreachable: the caller must
	// learn it could not ask, not that the goods cannot be carried.
	_, err := SelectAuthority([]Candidate{
		{Authority: logistics.Dispatch, Outcome: OutcomeCapabilityMismatch},
		{Authority: logistics.Taxi, Outcome: OutcomeDependencyUnavailable},
	})
	if !errors.Is(err, ErrAuthoritiesUnreachable) {
		t.Fatalf("selection error = %v, want ErrAuthoritiesUnreachable", err)
	}

	// Every authority answered, and all refused: this really is final.
	_, err = SelectAuthority([]Candidate{
		{Authority: logistics.Dispatch, Outcome: OutcomeCapabilityMismatch},
		{Authority: logistics.Taxi, Outcome: OutcomeCapabilityMismatch},
		{Authority: logistics.Hauling, Outcome: OutcomeNotServiceable},
	})
	if !errors.Is(err, ErrNoEligibleAuthority) {
		t.Fatalf("selection error = %v, want ErrNoEligibleAuthority", err)
	}
}

// TestClassifyFailureKeepsTheTwoKindsApart pins the mapping from the logistics
// integration's own categories.
func TestClassifyFailureKeepsTheTwoKindsApart(t *testing.T) {
	cases := map[logistics.FailureCategory]OutcomeClass{
		logistics.FailureCapabilityMismatch:    OutcomeCapabilityMismatch,
		logistics.FailureCredential:            OutcomeCredential,
		logistics.FailureNotServiceable:        OutcomeNotServiceable,
		logistics.FailureDependencyUnavailable: OutcomeDependencyUnavailable,
	}
	for category, expected := range cases {
		if got := ClassifyFailure(category, ""); got != expected {
			t.Errorf("%s classified as %s, want %s", category, got, expected)
		}
	}
	// A disabled authority is a deployment state, not an authority's opinion.
	if got := ClassifyFailure(logistics.FailureDependencyUnavailable, "AUTHORITY_DISABLED"); got != OutcomeDisabled {
		t.Fatalf("a disabled authority classified as %s, want DISABLED", got)
	}
}

// TestDisabledAuthorityIsNeverSelected is mandatory test 24.
func TestDisabledAuthorityIsNeverSelected(t *testing.T) {
	_, err := SelectAuthority([]Candidate{
		{Authority: logistics.Dispatch, Outcome: OutcomeDisabled, ReasonCode: "AUTHORITY_DISABLED"},
	})
	if err == nil {
		t.Fatal("a disabled authority was selected")
	}
	// A disabled authority is not a refusal about the goods, so the caller is
	// told it could not ask rather than that nothing can carry this.
	if !errors.Is(err, ErrAuthoritiesUnreachable) {
		t.Fatalf("error = %v, want the unreachable classification", err)
	}
}

// TestConfidentSupplyWinsAmongEqualFamilies keeps the tie-break honest: it is
// about the likelihood a commit matches, never about cost.
func TestConfidentSupplyWinsAmongEqualFamilies(t *testing.T) {
	selected, err := SelectAuthority([]Candidate{
		{Authority: logistics.Dispatch, ProductCode: "THIN", Outcome: OutcomeServiceable,
			SupplyStatus: supplypreview.StatusLimited},
		{Authority: logistics.Dispatch, ProductCode: "AMPLE", Outcome: OutcomeServiceable,
			SupplyStatus: supplypreview.StatusAvailable},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.ProductCode != "AMPLE" {
		t.Fatalf("selected %q, want the product with confident supply", selected.ProductCode)
	}
}

// ── Warm Match creates nothing ──────────────────────────────────────────────

// TestSupplyPreviewLeaksNoProviderIdentity is mandatory test 26.
func TestSupplyPreviewLeaksNoProviderIdentity(t *testing.T) {
	requirement := Requirement{
		PickupCount: 1, DropoffCount: 1, WeightGrams: 4000,
		PickupLatitude: 6.43, PickupLongitude: 3.42,
		DropoffLatitude: 6.45, DropoffLongitude: 3.40,
	}
	raw, err := json.Marshal(requirement.Preview("DISPATCH_STANDARD"))
	if err != nil {
		t.Fatal(err)
	}
	lowered := strings.ToLower(string(raw))
	for _, banned := range providerSelectionFields() {
		if strings.Contains(lowered, `"`+banned+`"`) {
			t.Fatalf("the preview request carries %q", banned)
		}
	}
	// And the response type has nowhere to put one either.
	for _, field := range walkFieldNames(reflect.TypeOf(supplypreview.Response{})) {
		for _, banned := range providerSelectionFields() {
			if strings.EqualFold(field, banned) {
				t.Fatalf("the preview response carries %q", field)
			}
		}
	}
}

// TestWarmMatchHappensBeforeCommitment is mandatory tests 27 and 28's timing
// half: the preview runs in a state where no mission exists.
func TestWarmMatchHappensBeforeCommitment(t *testing.T) {
	if StateWarmMatch.Committed() {
		t.Fatal("WARM_MATCH reports as committed; nothing has been created yet")
	}
	for _, state := range []State{StatePlanned, StateWaitingReadiness, StateWarmMatch, StateCommitDue} {
		if state.Committed() {
			t.Fatalf("%s reports as committed before a mission exists", state)
		}
	}
	for _, state := range []State{StateCommitted, StateInExecution, StateCompleted} {
		if !state.Committed() {
			t.Fatalf("%s should report as committed", state)
		}
	}
	// A warm match that creates a mission would have to pass through
	// COMMITTING, which WARM_MATCH cannot reach directly.
	if CanTransition(StateWarmMatch, StateCommitting) {
		t.Fatal("WARM_MATCH can reach COMMITTING directly; a preview could create a mission")
	}
	if !CanTransition(StateCommitDue, StateCommitting) {
		t.Fatal("COMMIT_DUE must be able to commit")
	}
}

// TestAfterCommitmentTheAuthorityOwnsRecovery is mandatory test 36.
func TestAfterCommitmentTheAuthorityOwnsRecovery(t *testing.T) {
	// A committed orchestration cannot go back to planning or re-select.
	for _, target := range []State{StatePlanned, StateWaitingReadiness, StateWarmMatch, StateCommitDue} {
		if CanTransition(StateCommitted, target) {
			t.Errorf("COMMITTED -> %s is allowed; Vendor would be re-planning work an authority owns", target)
		}
		if CanTransition(StateInExecution, target) {
			t.Errorf("IN_EXECUTION -> %s is allowed", target)
		}
	}
	// What it CAN do is observe the authority's own recovery.
	if !CanTransition(StateInExecution, StateRecovering) {
		t.Fatal("Vendor must be able to project an authority's recovery")
	}
	if !CanTransition(StateRecovering, StateInExecution) {
		t.Fatal("a recovered mission must be able to resume")
	}
}

// ── Just-in-time timing ─────────────────────────────────────────────────────

// TestEarlyReadyAcceleratesCommit is mandatory test 19.
func TestEarlyReadyAcceleratesCommit(t *testing.T) {
	now := at(12, 0)
	base := CommitInput{
		Now: now, PickupETA: 6 * time.Minute, ConfidenceBand: readiness.ConfidenceHigh,
		PredictedReadyStart: now.Add(30 * time.Minute),
		PredictedReadyEnd:   now.Add(34 * time.Minute),
	}
	later := PlanCommit(base)
	if later.Due {
		t.Fatal("committed immediately for an order thirty minutes away")
	}

	// The kitchen finishes far sooner than planned.
	base.PredictedReadyStart = now.Add(4 * time.Minute)
	base.PredictedReadyEnd = now.Add(5 * time.Minute)
	earlier := PlanCommit(base)
	if !earlier.CommitNotBefore.Before(later.CommitNotBefore) {
		t.Fatalf("an early-ready order did not accelerate: early=%v late=%v",
			earlier.CommitNotBefore, later.CommitNotBefore)
	}
	if !earlier.Due {
		t.Fatal("an order ready in four minutes with a six-minute pickup ETA must commit now")
	}
}

// TestDelayPostponesCommitment is mandatory test 20, and the reason nothing is
// created during Warm Match: a slip before commitment costs nothing because no
// provider was ever reserved.
func TestDelayPostponesCommitment(t *testing.T) {
	now := at(12, 0)
	base := CommitInput{
		Now: now, PickupETA: 5 * time.Minute, ConfidenceBand: readiness.ConfidenceHigh,
		PredictedReadyStart: now.Add(8 * time.Minute),
		PredictedReadyEnd:   now.Add(9 * time.Minute),
	}
	before := PlanCommit(base)

	// The vendor reports a twenty-minute delay.
	base.PredictedReadyStart = now.Add(28 * time.Minute)
	base.PredictedReadyEnd = now.Add(30 * time.Minute)
	after := PlanCommit(base)

	if !after.CommitNotBefore.After(before.CommitNotBefore) {
		t.Fatalf("a delay did not postpone commitment: before=%v after=%v",
			before.CommitNotBefore, after.CommitNotBefore)
	}
	if after.Due {
		t.Fatal("a delayed order committed immediately")
	}
}

// TestLowConfidenceBuysABiggerBuffer connects the predictor's honesty to the
// planner's behaviour: an estimate nobody can vouch for gets more slack.
func TestLowConfidenceBuysABiggerBuffer(t *testing.T) {
	now := at(12, 0)
	base := CommitInput{
		Now: now, PickupETA: 5 * time.Minute,
		PredictedReadyStart: now.Add(40 * time.Minute),
		PredictedReadyEnd:   now.Add(42 * time.Minute),
	}
	base.ConfidenceBand = readiness.ConfidenceHigh
	confident := PlanCommit(base)
	base.ConfidenceBand = readiness.ConfidenceLow
	guessed := PlanCommit(base)

	if !(guessed.Buffer > confident.Buffer) {
		t.Fatalf("low confidence did not widen the buffer: high=%v low=%v",
			confident.Buffer, guessed.Buffer)
	}
	if !guessed.CommitNotBefore.Before(confident.CommitNotBefore) {
		t.Fatal("a wider buffer should commit earlier, not later")
	}
}

// TestStaleReadinessWidensTheBuffer is mandatory test 21's planner half.
func TestStaleReadinessWidensTheBuffer(t *testing.T) {
	now := at(12, 0)
	base := CommitInput{
		Now: now, PickupETA: 5 * time.Minute, ConfidenceBand: readiness.ConfidenceMedium,
		PredictedReadyStart: now.Add(40 * time.Minute),
		PredictedReadyEnd:   now.Add(43 * time.Minute),
	}
	fresh := PlanCommit(base)
	base.ReadinessStale = true
	stale := PlanCommit(base)
	if !(stale.Buffer > fresh.Buffer) {
		t.Fatalf("a stale signal did not widen the buffer: fresh=%v stale=%v",
			fresh.Buffer, stale.Buffer)
	}
}

// TestAnUnmeetableDeadlineIsSurfacedNotAbsorbed keeps a late delivery a thing
// somebody is told about rather than discovers.
func TestAnUnmeetableDeadlineIsSurfacedNotAbsorbed(t *testing.T) {
	now := at(12, 0)
	decision := PlanCommit(CommitInput{
		Now: now, PickupETA: 20 * time.Minute, ConfidenceBand: readiness.ConfidenceMedium,
		PredictedReadyStart: now.Add(40 * time.Minute),
		PredictedReadyEnd:   now.Add(45 * time.Minute),
		HandoverDuration:    3 * time.Minute,
		Deadline:            now.Add(30 * time.Minute),
	})
	if !decision.AtRisk {
		t.Fatal("an unmeetable deadline was absorbed silently")
	}
	if decision.ReasonCode != ReasonDeadlineAtRisk {
		t.Fatalf("reason = %q, want %q", decision.ReasonCode, ReasonDeadlineAtRisk)
	}
	if !decision.Due {
		t.Fatal("an at-risk order should commit as early as it legitimately can")
	}
}

// TestEveryDecisionSchedulesAnotherLook stops an orchestration going quiet.
func TestEveryDecisionSchedulesAnotherLook(t *testing.T) {
	now := at(12, 0)
	for _, input := range []CommitInput{
		{Now: now},
		{Now: now, PredictedReadyStart: now.Add(2 * time.Hour), PickupETA: 5 * time.Minute},
		{Now: now, PredictedReadyStart: now.Add(time.Minute), PickupETA: 5 * time.Minute},
	} {
		decision := PlanCommit(input)
		if decision.NextEvaluationAt.IsZero() {
			t.Fatal("a decision scheduled no further evaluation; the order would never be looked at again")
		}
		if !decision.NextEvaluationAt.After(now) {
			t.Fatalf("next evaluation %v is not in the future", decision.NextEvaluationAt)
		}
	}
}

// TestWarmMatchStartsBeforeCommitment pins the preview lead.
func TestWarmMatchStartsBeforeCommitment(t *testing.T) {
	now := at(12, 0)
	if ShouldWarmMatch(now, now.Add(time.Hour)) {
		t.Fatal("warm match started an hour early; the answer would be stale by commitment")
	}
	if !ShouldWarmMatch(now, now.Add(WarmMatchLead-time.Minute)) {
		t.Fatal("warm match did not start inside its own lead window")
	}
	if ShouldWarmMatch(now, time.Time{}) {
		t.Fatal("warm match ran with no commit time to lead")
	}
}
