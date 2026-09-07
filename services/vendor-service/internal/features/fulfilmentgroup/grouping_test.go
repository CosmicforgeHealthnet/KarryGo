package fulfilmentgroup

import "testing"

func motorcycleCapability() Capability {
	return Capability{
		Authority:       AuthorityDispatch,
		MultiStopPickup: true,
		MaxPickupStops:  3,
		MaxWeightGrams:  25_000,
		MaxPackageCount: 10,
	}
}

func candidate(unitID, business, branch, destination string, weight int64) Candidate {
	return Candidate{
		Unit: Unit{
			ID:                   unitID,
			BusinessID:           business,
			BranchID:             branch,
			VehicleClass:         "MOTORCYCLE",
			ServiceClass:         "FOOD",
			RequiredWeightGrams:  weight,
			RequiredPackageCount: 1,
		},
		Owner:          OwnerPlatform,
		Authority:      AuthorityDispatch,
		DestinationRef: destination,
		WeightKnown:    true,
		ParcelIDs:      []string{"p-" + unitID},
	}
}

// Two compatible pickups to one receiver are the case multi-stop exists for.
func TestCompatible_TwoVendorsOneDestination(t *testing.T) {
	a := candidate("u1", "biz-a", "br-a", "dest-1", 4_000)
	b := candidate("u2", "biz-b", "br-b", "dest-1", 5_000)

	if refusal, ok := Compatible(a, b, motorcycleCapability()); !ok {
		t.Fatalf("expected compatible pickups, refused with %q", refusal)
	}
}

// A shared destination is necessary but never sufficient. This is the check
// that stops truck cargo being planned onto a motorcycle.
func TestCompatible_SharedDestinationIsNotEnough(t *testing.T) {
	a := candidate("u1", "biz-a", "br-a", "dest-1", 4_000)
	b := candidate("u2", "biz-b", "br-b", "dest-1", 4_000)
	b.Unit.VehicleClass = "TRUCK"

	refusal, ok := Compatible(a, b, motorcycleCapability())
	if ok {
		t.Fatal("a motorcycle pickup and truck cargo must not share a mission")
	}
	if refusal != RefuseDifferentVehicle {
		t.Fatalf("refusal = %q; want %q", refusal, RefuseDifferentVehicle)
	}
}

func TestCompatible_DifferentDestinationsAreNeverGrouped(t *testing.T) {
	a := candidate("u1", "biz-a", "br-a", "dest-1", 1_000)
	b := candidate("u2", "biz-b", "br-b", "dest-2", 1_000)

	refusal, ok := Compatible(a, b, motorcycleCapability())
	if ok {
		t.Fatal("different receiver destinations must not be grouped")
	}
	if refusal != RefuseDifferentDestination {
		t.Fatalf("refusal = %q; want %q", refusal, RefuseDifferentDestination)
	}
}

func TestCompatible_DifferentServiceClassSplits(t *testing.T) {
	a := candidate("u1", "biz-a", "br-a", "dest-1", 1_000)
	b := candidate("u2", "biz-b", "br-b", "dest-1", 1_000)
	b.Unit.ServiceClass = "EXPRESS"

	if refusal, ok := Compatible(a, b, motorcycleCapability()); ok || refusal != RefuseDifferentService {
		t.Fatalf("expected service-class refusal, got %q ok=%v", refusal, ok)
	}
}

// An authority with no multi-stop capability must cause a SPLIT, not a mission
// it would have to fail once the customer is already waiting.
func TestCompatible_AuthorityWithoutMultiStopSplits(t *testing.T) {
	capability := motorcycleCapability()
	capability.MultiStopPickup = false

	a := candidate("u1", "biz-a", "br-a", "dest-1", 1_000)
	b := candidate("u2", "biz-b", "br-b", "dest-1", 1_000)

	refusal, ok := Compatible(a, b, capability)
	if ok {
		t.Fatal("an authority without multi-stop must not be given two pickups")
	}
	if refusal != RefuseAuthorityNoMultiStop {
		t.Fatalf("refusal = %q; want %q", refusal, RefuseAuthorityNoMultiStop)
	}
}

// Unstated weight is unknown, never zero. Grouping on a guessed weight is what
// fails at the roadside.
func TestCompatible_UnknownWeightRefusesGrouping(t *testing.T) {
	a := candidate("u1", "biz-a", "br-a", "dest-1", 1_000)
	b := candidate("u2", "biz-b", "br-b", "dest-1", 0)
	b.WeightKnown = false

	refusal, ok := Compatible(a, b, motorcycleCapability())
	if ok {
		t.Fatal("unknown weight must refuse grouping rather than assume zero")
	}
	if refusal != RefuseUnknownConstraint {
		t.Fatalf("refusal = %q; want %q", refusal, RefuseUnknownConstraint)
	}
}

func TestCompatible_VendorOwnedIsNeverConsolidated(t *testing.T) {
	a := candidate("u1", "biz-a", "br-a", "dest-1", 1_000)
	b := candidate("u2", "biz-a", "br-b", "dest-1", 1_000)
	b.Owner = OwnerVendor
	b.Authority = AuthorityVendor

	if refusal, ok := Compatible(a, b, motorcycleCapability()); ok || refusal != RefuseVendorOwned {
		t.Fatalf("expected vendor-own refusal, got %q ok=%v", refusal, ok)
	}
}

func TestCompatible_IncompatibleHandlingSplits(t *testing.T) {
	a := candidate("u1", "biz-a", "br-a", "dest-1", 1_000)
	a.Unit.HandlingRequirements = []string{"CHILLED"}
	b := candidate("u2", "biz-b", "br-b", "dest-1", 1_000)

	if refusal, ok := Compatible(a, b, motorcycleCapability()); ok || refusal != RefuseHandlingConflict {
		t.Fatalf("expected handling refusal, got %q ok=%v", refusal, ok)
	}
}

// Cumulative capacity is what no pairwise check can see: three pickups that are
// each individually fine must not build one overloaded mission.
func TestGroupCandidates_CumulativeWeightSplits(t *testing.T) {
	capability := motorcycleCapability()
	capability.MaxWeightGrams = 10_000

	groups := GroupCandidates([]Candidate{
		candidate("u1", "biz-a", "br-a", "dest-1", 6_000),
		candidate("u2", "biz-b", "br-b", "dest-1", 3_000),
		candidate("u3", "biz-c", "br-c", "dest-1", 4_000),
	}, map[Authority]Capability{AuthorityDispatch: capability})

	if len(groups) != 2 {
		t.Fatalf("groups = %d; want 2 (cumulative load must split)", len(groups))
	}
	for _, group := range groups {
		if group.TotalWeightGrams() > capability.MaxWeightGrams {
			t.Fatalf("group weight %d exceeds capacity %d",
				group.TotalWeightGrams(), capability.MaxWeightGrams)
		}
	}
}

func TestGroupCandidates_StopLimitSplits(t *testing.T) {
	capability := motorcycleCapability()
	capability.MaxPickupStops = 2

	groups := GroupCandidates([]Candidate{
		candidate("u1", "biz-a", "br-a", "dest-1", 100),
		candidate("u2", "biz-b", "br-b", "dest-1", 100),
		candidate("u3", "biz-c", "br-c", "dest-1", 100),
	}, map[Authority]Capability{AuthorityDispatch: capability})

	if len(groups) != 2 {
		t.Fatalf("groups = %d; want 2", len(groups))
	}
	if len(groups[0].Candidates) != 2 {
		t.Fatalf("first group has %d stops; want 2", len(groups[0].Candidates))
	}
}

// A replan must produce the same missions from the same inputs, or replan
// idempotency cannot be asserted at all.
func TestGroupCandidates_IsDeterministic(t *testing.T) {
	capabilities := map[Authority]Capability{AuthorityDispatch: motorcycleCapability()}
	input := []Candidate{
		candidate("u3", "biz-c", "br-c", "dest-1", 100),
		candidate("u1", "biz-a", "br-a", "dest-1", 100),
		candidate("u2", "biz-b", "br-b", "dest-1", 100),
	}

	first := GroupCandidates(input, capabilities)
	second := GroupCandidates([]Candidate{input[1], input[2], input[0]}, capabilities)

	if len(first) != len(second) {
		t.Fatalf("group counts differ: %d vs %d", len(first), len(second))
	}
	for index := range first {
		if len(first[index].Candidates) != len(second[index].Candidates) {
			t.Fatalf("group %d differs in size", index)
		}
		for member := range first[index].Candidates {
			if first[index].Candidates[member].Unit.ID !=
				second[index].Candidates[member].Unit.ID {
				t.Fatalf("group %d member %d differs: %s vs %s", index, member,
					first[index].Candidates[member].Unit.ID,
					second[index].Candidates[member].Unit.ID)
			}
		}
	}
}

// Each vendor-owned candidate becomes its own mission: one vendor's goods never
// travel in another vendor's van.
func TestGroupCandidates_VendorOwnedNeverMerges(t *testing.T) {
	vendorA := candidate("u1", "biz-a", "br-a", "dest-1", 100)
	vendorA.Owner, vendorA.Authority = OwnerVendor, AuthorityVendor
	vendorB := candidate("u2", "biz-b", "br-b", "dest-1", 100)
	vendorB.Owner, vendorB.Authority = OwnerVendor, AuthorityVendor

	groups := GroupCandidates([]Candidate{vendorA, vendorB},
		map[Authority]Capability{})

	if len(groups) != 2 {
		t.Fatalf("groups = %d; want 2 vendor-owned missions", len(groups))
	}
}

// Parcel identity is preserved through grouping: separate sources stay separate
// parcels even when they share a vehicle.
func TestBuildStops_MultiPickupOneDestination(t *testing.T) {
	group := MissionGroup{
		Authority: AuthorityDispatch,
		Owner:     OwnerPlatform,
		Candidates: []Candidate{
			candidate("u1", "biz-a", "br-a", "dest-1", 100),
			candidate("u2", "biz-b", "br-b", "dest-1", 100),
		},
	}

	stops := BuildStops(group, Stop{Address: "12 Customer Road"})

	if len(stops) != 3 {
		t.Fatalf("stops = %d; want 2 pickups + 1 dropoff", len(stops))
	}
	for index, stop := range stops {
		if stop.Sequence != index+1 {
			t.Fatalf("stop %d has sequence %d; route order must be persisted",
				index, stop.Sequence)
		}
	}
	if stops[2].Type != StopDropoff || stops[2].UnitID != "" {
		t.Fatal("final stop must be the single receiver destination")
	}

	parcels := CollectParcelIDs(group)
	if len(parcels) != 2 {
		t.Fatalf("parcels = %d; want 2 distinct parcels", len(parcels))
	}
	if parcels[0] == parcels[1] {
		t.Fatal("parcels from different vendors must not be merged into one")
	}
}

func TestSameBranchObligation_DistinguishesBranches(t *testing.T) {
	a := Unit{BusinessID: "biz-a", BranchID: "br-1"}
	b := Unit{BusinessID: "biz-a", BranchID: "br-2"}

	if SameBranchObligation(a, b) {
		t.Fatal("two branches of one business are two pickup obligations")
	}
	if !SameBranchObligation(a, Unit{BusinessID: "biz-a", BranchID: "br-1"}) {
		t.Fatal("the same branch must be the same obligation")
	}
}

// A replan's slot references must be derived from what is being planned, so
// replanning the same units twice produces the same slot and the executing
// authority sees one idempotency key rather than two.
func TestBuildPlanMissions_SlotRefsAreStableAndDistinct(t *testing.T) {
	capabilities := map[Authority]Capability{AuthorityDispatch: motorcycleCapability()}
	candidates := []Candidate{
		candidate("u1", "biz-a", "br-a", "dest-1", 100),
		candidate("u2", "biz-b", "br-b", "dest-1", 100),
	}

	first := BuildPlanMissions(candidates, capabilities, Stop{Address: "12 Customer Road"}, "NGN")
	second := BuildPlanMissions(candidates, capabilities, Stop{Address: "12 Customer Road"}, "NGN")

	if len(first) != 1 {
		t.Fatalf("missions = %d; two compatible pickups share one vehicle", len(first))
	}
	if first[0].SlotRef != second[0].SlotRef {
		t.Fatalf("slot ref is unstable: %q then %q", first[0].SlotRef, second[0].SlotRef)
	}
	if err := first[0].Validate(); err != nil {
		t.Fatalf("built mission invalid: %v", err)
	}
	// Cost is not invented here: what a replanned delivery costs is the
	// executing authority's own answer.
	if first[0].PlannedCostMinor != 0 {
		t.Fatalf("planned cost = %d; this package must not price a platform delivery",
			first[0].PlannedCostMinor)
	}
}

// When the authority cannot serve several stops, the replan produces several
// missions with distinct slot references — never one the authority would fail.
func TestBuildPlanMissions_SplitsWhenAuthorityCannotMultiStop(t *testing.T) {
	capability := motorcycleCapability()
	capability.MultiStopPickup = false

	missions := BuildPlanMissions([]Candidate{
		candidate("u1", "biz-a", "br-a", "dest-1", 100),
		candidate("u2", "biz-b", "br-b", "dest-1", 100),
	}, map[Authority]Capability{AuthorityDispatch: capability},
		Stop{Address: "12 Customer Road"}, "NGN")

	if len(missions) != 2 {
		t.Fatalf("missions = %d; a single-pickup authority gets one mission per pickup", len(missions))
	}

	slots := map[string]bool{}
	for index, mission := range missions {
		if err := mission.Validate(); err != nil {
			t.Fatalf("split mission invalid: %v", err)
		}
		if mission.Sequence != index+1 {
			t.Fatalf("mission %d has sequence %d; sequences must be contiguous from 1",
				index, mission.Sequence)
		}
		if slots[mission.SlotRef] {
			t.Fatalf("two missions share slot %q; they would collide at the authority",
				mission.SlotRef)
		}
		slots[mission.SlotRef] = true
	}
}

func TestBuildPlanMissions_PlansTaxiGoodsWithDeclaredCapability(t *testing.T) {
	taxi := candidate("u1", "biz-a", "br-a", "dest-1", 100)
	taxi.Authority = AuthorityTaxi

	missions := BuildPlanMissions([]Candidate{taxi}, map[Authority]Capability{
		AuthorityTaxi: {Authority: AuthorityTaxi, MaxPickupStops: 1},
	}, Stop{Address: "12 Customer Road"}, "NGN")

	if len(missions) != 1 || missions[0].Authority != AuthorityTaxi {
		t.Fatalf("Taxi Goods missions = %+v; want one TAXI mission", missions)
	}
}

// Vendor-owned candidates each become their own mission and keep the vendor as
// both owner and authority.
func TestBuildPlanMissions_VendorOwnedGetsItsOwnMission(t *testing.T) {
	own := candidate("u1", "biz-a", "br-a", "dest-1", 100)
	own.Owner, own.Authority = OwnerVendor, AuthorityVendor

	missions := BuildPlanMissions([]Candidate{own},
		map[Authority]Capability{}, Stop{Address: "12 Customer Road"}, "NGN")

	if len(missions) != 1 {
		t.Fatalf("missions = %d; want 1", len(missions))
	}
	if missions[0].Owner != OwnerVendor || missions[0].Authority != AuthorityVendor {
		t.Fatalf("owner/authority = %s/%s; want VENDOR/VENDOR",
			missions[0].Owner, missions[0].Authority)
	}
	if err := missions[0].Validate(); err != nil {
		t.Fatalf("vendor mission invalid: %v", err)
	}
}
