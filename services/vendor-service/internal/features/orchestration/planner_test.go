package orchestration

import (
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
)

// Multi-vendor grouping. The rule under test throughout is that grouping is
// conservative: a shared trip is an optimisation, and every hard
// incompatibility outranks it.

func candidate(id string, readyOffset time.Duration) GroupCandidate {
	base := at(12, 0)
	return GroupCandidate{
		VendorOrderID:       id,
		BusinessID:          "biz-" + id,
		BranchID:            "br-" + id,
		PredictedReadyStart: base.Add(readyOffset),
		PredictedReadyEnd:   base.Add(readyOffset + 2*time.Minute),
		Latitude:            6.43,
		Longitude:           3.42,
		PackageCount:        1,
		WeightGrams:         2000,
	}
}

func multiPickupPolicy() GroupingPolicy {
	policy := DefaultGroupingPolicy()
	policy.MaxPickups = 4
	policy.MaxReadinessSpread = 10 * time.Minute
	policy.MaxDetourKm = 5
	return policy
}

// TestCompatibleVendorsAreGrouped is mandatory test 29.
func TestCompatibleVendorsAreGrouped(t *testing.T) {
	plan := GroupCandidates([]GroupCandidate{
		candidate("a", 0),
		candidate("b", 4*time.Minute),
	}, multiPickupPolicy())

	if len(plan.Groups) != 1 {
		t.Fatalf("groups = %d, want 1: two compatible vendors should share a trip", len(plan.Groups))
	}
	if len(plan.Groups[0].Members) != 2 {
		t.Fatalf("group members = %d, want 2", len(plan.Groups[0].Members))
	}
	// The group cannot leave before its slowest member is ready.
	latest := at(12, 4)
	if !plan.Groups[0].PickupWindowStart.Equal(latest) {
		t.Fatalf("pickup window starts %v, want the latest member's ready time %v",
			plan.Groups[0].PickupWindowStart, latest)
	}
}

// TestAMateriallyLaterVendorIsSplitOut is mandatory test 30.
//
// The failure it prevents is the whole reason grouping is conservative: one
// vendor's goods sitting in a vehicle while another finishes.
func TestAMateriallyLaterVendorIsSplitOut(t *testing.T) {
	plan := GroupCandidates([]GroupCandidate{
		candidate("early", 0),
		candidate("also-early", 3*time.Minute),
		candidate("late", 45*time.Minute),
	}, multiPickupPolicy())

	if len(plan.Groups) != 2 {
		t.Fatalf("groups = %d, want 2: the late vendor travels separately", len(plan.Groups))
	}
	for _, group := range plan.Groups {
		if len(group.Members) == 1 && group.Members[0].VendorOrderID != "late" {
			t.Fatalf("the solo group is %q, want the late vendor", group.Members[0].VendorOrderID)
		}
	}
	var splitReason SplitReason
	for _, split := range plan.Splits {
		if split.Candidate.VendorOrderID == "late" {
			splitReason = split.Reason
		}
	}
	if splitReason != SplitReadinessSpread {
		t.Fatalf("split reason = %q, want %q", splitReason, SplitReadinessSpread)
	}
}

// TestTemperatureIncompatibleVendorsAreSplit is mandatory test 31.
//
// "Close enough" is not a temperature band. Deciding chilled and frozen can
// share a compartment is how a frozen order thaws.
func TestTemperatureIncompatibleVendorsAreSplit(t *testing.T) {
	chilled := candidate("chilled", 0)
	chilled.TemperatureBand = "CHILLED"
	frozen := candidate("frozen", 2*time.Minute)
	frozen.TemperatureBand = "FROZEN"

	plan := GroupCandidates([]GroupCandidate{chilled, frozen}, multiPickupPolicy())
	if len(plan.Groups) != 2 {
		t.Fatalf("groups = %d, want 2: chilled and frozen cannot share a compartment", len(plan.Groups))
	}
	found := false
	for _, split := range plan.Splits {
		if split.Reason == SplitTemperature {
			found = true
		}
	}
	if !found {
		t.Fatalf("splits = %+v, want a temperature split", plan.Splits)
	}

	// Two orders in the same band still group, and an unset band means ambient
	// rather than "compatible with anything".
	ambientA, ambientB := candidate("a", 0), candidate("b", time.Minute)
	if plan := GroupCandidates([]GroupCandidate{ambientA, ambientB}, multiPickupPolicy()); len(plan.Groups) != 1 {
		t.Fatalf("two ambient orders produced %d groups, want 1", len(plan.Groups))
	}
	explicitAmbient := candidate("c", time.Minute)
	explicitAmbient.TemperatureBand = "AMBIENT"
	if plan := GroupCandidates([]GroupCandidate{ambientA, explicitAmbient}, multiPickupPolicy()); len(plan.Groups) != 1 {
		t.Fatal("an unset band and an explicit AMBIENT band should be compatible")
	}
}

// TestThePickupLimitIsTheAuthoritysOwn is mandatory test 32.
//
// The limit comes from the authority's capability document, never a constant
// here. A service that has quietly become single-pickup must stop receiving
// groups the moment it says so.
func TestThePickupLimitIsTheAuthoritysOwn(t *testing.T) {
	candidates := []GroupCandidate{
		candidate("a", 0), candidate("b", time.Minute), candidate("c", 2*time.Minute),
	}

	twoStops := DefaultGroupingPolicy().FromCapability(logistics.Capability{MaxPickups: 2})
	twoStops.MaxReadinessSpread = 10 * time.Minute
	twoStops.MaxDetourKm = 5
	plan := GroupCandidates(candidates, twoStops)
	for _, group := range plan.Groups {
		if len(group.Members) > 2 {
			t.Fatalf("group of %d exceeds the authority's declared 2-pickup limit", len(group.Members))
		}
	}

	// An authority that declares NO pickup limit is treated as single pickup.
	// Grouping against an unknown limit is how a three-stop mission reaches a
	// service that only ever accepted one.
	undeclared := DefaultGroupingPolicy().FromCapability(logistics.Capability{})
	if undeclared.MaxPickups != 1 {
		t.Fatalf("an undeclared pickup limit became %d, want 1", undeclared.MaxPickups)
	}
	undeclared.MaxReadinessSpread = 10 * time.Minute
	solo := GroupCandidates(candidates, undeclared)
	for _, group := range solo.Groups {
		if len(group.Members) != 1 {
			t.Fatalf("group of %d against an undeclared limit", len(group.Members))
		}
	}
}

// TestUndeclaredCapacityIsUnknownNotZero keeps an authority that said nothing
// about weight from being treated as able to carry nothing.
func TestUndeclaredCapacityIsUnknownNotZero(t *testing.T) {
	policy := DefaultGroupingPolicy().FromCapability(logistics.Capability{MaxPickups: 3})
	policy.MaxReadinessSpread = 10 * time.Minute
	policy.MaxDetourKm = 5
	if policy.MaxWeightGrams != 0 {
		t.Fatalf("undeclared weight bound = %d, want 0 meaning unknown", policy.MaxWeightGrams)
	}
	plan := GroupCandidates([]GroupCandidate{candidate("a", 0), candidate("b", time.Minute)}, policy)
	if len(plan.Groups) != 1 {
		t.Fatal("an undeclared weight bound blocked grouping; unknown was treated as zero capacity")
	}
}

// TestDeclaredCapacityIsEnforced is the other half: a bound the authority DID
// declare really does bind.
func TestDeclaredCapacityIsEnforced(t *testing.T) {
	policy := DefaultGroupingPolicy().FromCapability(logistics.Capability{
		MaxPickups: 4, MaxWeightGrams: 3000})
	policy.MaxReadinessSpread = 10 * time.Minute
	policy.MaxDetourKm = 5

	plan := GroupCandidates([]GroupCandidate{
		candidate("a", 0),           // 2000g
		candidate("b", time.Minute), // 2000g, would total 4000g
	}, policy)
	if len(plan.Groups) != 2 {
		t.Fatalf("groups = %d, want 2: the pair exceeds the declared 3kg bound", len(plan.Groups))
	}
	found := false
	for _, split := range plan.Splits {
		if split.Reason == SplitCapacity {
			found = true
		}
	}
	if !found {
		t.Fatalf("splits = %+v, want a capacity split", plan.Splits)
	}
}

// TestDetourLimitSplitsDistantVendors keeps a shared trip from becoming a tour.
func TestDetourLimitSplitsDistantVendors(t *testing.T) {
	near := candidate("near", 0)
	far := candidate("far", time.Minute)
	far.Latitude, far.Longitude = 7.40, 3.90 // roughly 120km away

	policy := multiPickupPolicy()
	plan := GroupCandidates([]GroupCandidate{near, far}, policy)
	if len(plan.Groups) != 2 {
		t.Fatalf("groups = %d, want 2: a 120km detour is not a shared trip", len(plan.Groups))
	}
	found := false
	for _, split := range plan.Splits {
		if split.Reason == SplitDetour {
			found = true
		}
	}
	if !found {
		t.Fatalf("splits = %+v, want a detour split", plan.Splits)
	}
}

// TestProtectedAndUnprotectedPickupsDoNotMix keeps a verified collection from
// carrying an unverified one along inside it.
func TestProtectedAndUnprotectedPickupsDoNotMix(t *testing.T) {
	protected := candidate("protected", 0)
	protected.SecurePickup = true
	ordinary := candidate("ordinary", time.Minute)

	plan := GroupCandidates([]GroupCandidate{protected, ordinary}, multiPickupPolicy())
	if len(plan.Groups) != 2 {
		t.Fatalf("groups = %d, want 2: a protected handover must not be mixed", len(plan.Groups))
	}

	// Two protected pickups still group with each other.
	secondProtected := candidate("also-protected", time.Minute)
	secondProtected.SecurePickup = true
	if plan := GroupCandidates([]GroupCandidate{protected, secondProtected},
		multiPickupPolicy()); len(plan.Groups) != 1 {
		t.Fatal("two protected pickups should be able to share a trip")
	}
}

// TestADeadlineOutranksAConvenientGroup keeps an order that would be made late
// out of a shared trip.
func TestADeadlineOutranksAConvenientGroup(t *testing.T) {
	urgent := candidate("urgent", 0)
	urgent.Deadline = at(12, 5)
	slower := candidate("slower", 8*time.Minute)

	plan := GroupCandidates([]GroupCandidate{urgent, slower}, multiPickupPolicy())
	if len(plan.Groups) != 2 {
		t.Fatalf("groups = %d, want 2: waiting for the slower vendor would miss the deadline",
			len(plan.Groups))
	}
}

// TestCustodyStartPreventsReshuffle is mandatory test 33.
//
// Once goods have been collected, parcel identity, manifest history and custody
// ownership are a chain of evidence. Re-cutting the group afterwards would
// rewrite a record somebody may need to prove who held what and when.
func TestCustodyStartPreventsReshuffle(t *testing.T) {
	for _, state := range []State{StateCommitted, StateInExecution, StateRecovering, StateCompleted} {
		if !CustodyStarted(state) {
			t.Errorf("%s should count as custody started", state)
		}
	}
	for _, state := range []State{StatePlanned, StateWaitingReadiness, StateWarmMatch, StateCommitDue} {
		if CustodyStarted(state) {
			t.Errorf("%s should still be replannable", state)
		}
	}
}

// TestGroupingIsDeterministic keeps the planner explainable: the same inputs
// must produce the same trips, or an operator cannot reason about why two
// orders travelled together yesterday and not today.
func TestGroupingIsDeterministic(t *testing.T) {
	candidates := []GroupCandidate{
		candidate("a", 0), candidate("b", 2*time.Minute),
		candidate("c", 30*time.Minute), candidate("d", time.Minute),
	}
	first := GroupCandidates(candidates, multiPickupPolicy())
	for i := 0; i < 10; i++ {
		again := GroupCandidates(candidates, multiPickupPolicy())
		if len(again.Groups) != len(first.Groups) {
			t.Fatalf("run %d produced %d groups, first produced %d",
				i, len(again.Groups), len(first.Groups))
		}
		for g := range again.Groups {
			if len(again.Groups[g].Members) != len(first.Groups[g].Members) {
				t.Fatalf("run %d group %d differs in size", i, g)
			}
			for m := range again.Groups[g].Members {
				if again.Groups[g].Members[m].VendorOrderID != first.Groups[g].Members[m].VendorOrderID {
					t.Fatalf("run %d group %d member %d differs", i, g, m)
				}
			}
		}
	}
}

// TestEveryCandidateIsPlacedExactlyOnce guards the partition itself: an order
// dropped by the planner is an order nobody collects.
func TestEveryCandidateIsPlacedExactlyOnce(t *testing.T) {
	candidates := []GroupCandidate{
		candidate("a", 0), candidate("b", 2*time.Minute),
		candidate("c", 40*time.Minute), candidate("d", time.Minute),
		candidate("e", 80*time.Minute),
	}
	plan := GroupCandidates(candidates, multiPickupPolicy())

	seen := map[string]int{}
	for _, group := range plan.Groups {
		for _, member := range group.Members {
			seen[member.VendorOrderID]++
		}
	}
	for _, input := range candidates {
		switch seen[input.VendorOrderID] {
		case 1:
		case 0:
			t.Errorf("%s was never placed in a group; nobody would collect it", input.VendorOrderID)
		default:
			t.Errorf("%s appears in %d groups; two providers would be sent",
				input.VendorOrderID, seen[input.VendorOrderID])
		}
	}
}
