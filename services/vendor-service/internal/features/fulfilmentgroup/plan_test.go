package fulfilmentgroup

import "testing"

func platformMission(slot string, sequence int, unitIDs ...string) PlanMission {
	mission := PlanMission{
		SlotRef:   slot,
		Sequence:  sequence,
		Owner:     OwnerPlatform,
		Authority: AuthorityDispatch,
		Currency:  "NGN",
	}
	for index, unitID := range unitIDs {
		mission.Stops = append(mission.Stops, Stop{
			Sequence: index + 1,
			Type:     StopPickup,
			UnitID:   unitID,
			BranchID: "br-" + unitID,
		})
	}
	mission.Stops = append(mission.Stops, Stop{
		Sequence: len(unitIDs) + 1,
		Type:     StopDropoff,
		Address:  "12 Customer Road",
	})
	return mission
}

func TestPlanMission_ValidMultiPickup(t *testing.T) {
	if err := platformMission("slot-1", 1, "u1", "u2").Validate(); err != nil {
		t.Fatalf("valid multi-pickup mission refused: %v", err)
	}
}

// Exactly one receiver destination. Batching several customers onto one
// mission is a different capability that this aggregate does not own.
func TestPlanMission_RejectsTwoDestinations(t *testing.T) {
	mission := platformMission("slot-1", 1, "u1")
	mission.Stops = append(mission.Stops, Stop{
		Sequence: 3, Type: StopDropoff, Address: "9 Other Street",
	})

	if err := mission.Validate(); err == nil {
		t.Fatal("a mission with two receiver destinations must be refused")
	}
}

func TestPlanMission_RejectsMissingDestination(t *testing.T) {
	mission := PlanMission{
		SlotRef: "slot-1", Sequence: 1, Owner: OwnerPlatform,
		Authority: AuthorityDispatch,
		Stops: []Stop{
			{Sequence: 1, Type: StopPickup, UnitID: "u1"},
		},
	}
	if err := mission.Validate(); err == nil {
		t.Fatal("a mission with no destination must be refused")
	}
}

func TestPlanMission_RejectsDuplicateStopSequence(t *testing.T) {
	mission := PlanMission{
		SlotRef: "slot-1", Sequence: 1, Owner: OwnerPlatform,
		Authority: AuthorityDispatch,
		Stops: []Stop{
			{Sequence: 1, Type: StopPickup, UnitID: "u1"},
			{Sequence: 1, Type: StopDropoff},
		},
	}
	if err := mission.Validate(); err == nil {
		t.Fatal("two stops claiming the same position is not a route")
	}
}

// A vendor-owned mission executed by a platform authority would make
// responsibility unanswerable.
func TestPlanMission_RejectsOwnerAuthorityMismatch(t *testing.T) {
	mission := platformMission("slot-1", 1, "u1")
	mission.Owner = OwnerVendor

	if err := mission.Validate(); err == nil {
		t.Fatal("owner/authority mismatch must be refused")
	}
}

func TestPlanInput_FirstVersionMustBeInitialPlan(t *testing.T) {
	in := PlanInput{
		ExpectedVersion: 0,
		Reason:          ReasonCapacityOverflow,
		DeliveryMode:    ModePlatform,
		IdempotencyKey:  "key-1",
		Missions:        []PlanMission{platformMission("slot-1", 1, "u1")},
	}
	if err := in.Validate(); err == nil {
		t.Fatal("version 1 must be the initial plan, not a replan reason")
	}

	in.Reason = ReasonInitialPlan
	if err := in.Validate(); err != nil {
		t.Fatalf("initial plan refused: %v", err)
	}
}

// Every version after the first has to name the operational condition that
// forced it, or the history cannot explain itself.
func TestPlanInput_ReplanMustNameAReason(t *testing.T) {
	in := PlanInput{
		ExpectedVersion: 3,
		Reason:          ReasonInitialPlan,
		DeliveryMode:    ModeHybrid,
		IdempotencyKey:  "key-2",
		Missions:        []PlanMission{platformMission("slot-1", 1, "u1")},
	}
	if err := in.Validate(); err == nil {
		t.Fatal("a replan must not claim to be the initial plan")
	}

	in.Reason = ReasonVehicleOverflow
	if err := in.Validate(); err != nil {
		t.Fatalf("vehicle-overflow replan refused: %v", err)
	}
}

// Taxi is modelled in the vocabulary but is not an active commerce authority.
// Planning against it is refused rather than attempted.
func TestPlanInput_RejectsInactiveAuthority(t *testing.T) {
	mission := platformMission("slot-1", 1, "u1")
	mission.Authority = AuthorityTaxi

	in := PlanInput{
		ExpectedVersion: 0,
		Reason:          ReasonInitialPlan,
		DeliveryMode:    ModePlatform,
		IdempotencyKey:  "key-3",
		Missions:        []PlanMission{mission},
	}
	if err := in.Validate(); err != ErrConflict {
		t.Fatalf("err = %v; want ErrConflict for an inactive authority", err)
	}
}

func TestAuthority_TaxiIsModelledButNotActive(t *testing.T) {
	if !AuthorityTaxi.Valid() {
		t.Fatal("taxi must be a known authority in the vocabulary")
	}
	if !AuthorityTaxi.Platform() {
		t.Fatal("taxi is a platform authority")
	}
	if AuthorityTaxi.ActiveForCommerce() {
		t.Fatal("taxi has no commerce mission ingress and must not be selectable")
	}
	if !AuthorityDispatch.ActiveForCommerce() || !AuthorityHauling.ActiveForCommerce() {
		t.Fatal("dispatch and hauling are active commerce authorities")
	}
}

func TestPlanInput_RejectsDuplicateSlots(t *testing.T) {
	in := PlanInput{
		ExpectedVersion: 0,
		Reason:          ReasonInitialPlan,
		DeliveryMode:    ModePlatform,
		IdempotencyKey:  "key-4",
		Missions: []PlanMission{
			platformMission("slot-1", 1, "u1"),
			platformMission("slot-1", 2, "u2"),
		},
	}
	if err := in.Validate(); err == nil {
		t.Fatal("two missions sharing a slot reference must be refused")
	}
}

// The fingerprint covers planning SEMANTICS, so the same decision digests the
// same way regardless of the order it was assembled in — which is what makes it
// usable for replan idempotency rather than just a checksum of a row.
func TestFingerprint_StableAcrossMissionOrder(t *testing.T) {
	first := []PlanMission{
		platformMission("slot-a", 1, "u1"),
		platformMission("slot-b", 2, "u2"),
	}
	second := []PlanMission{
		platformMission("slot-b", 2, "u2"),
		platformMission("slot-a", 1, "u1"),
	}

	if Fingerprint(ModePlatform, first) != Fingerprint(ModePlatform, second) {
		t.Fatal("fingerprint must not depend on mission assembly order")
	}
}

func TestFingerprint_ChangesWithAuthority(t *testing.T) {
	base := []PlanMission{platformMission("slot-a", 1, "u1")}

	moved := []PlanMission{platformMission("slot-a", 1, "u1")}
	moved[0].Authority = AuthorityHauling

	if Fingerprint(ModePlatform, base) == Fingerprint(ModePlatform, moved) {
		t.Fatal("moving a mission to another authority must change the fingerprint")
	}
}

func TestFingerprint_ChangesWithParcelAssignment(t *testing.T) {
	base := []PlanMission{platformMission("slot-a", 1, "u1")}

	loaded := []PlanMission{platformMission("slot-a", 1, "u1")}
	loaded[0].ParcelIDs = []string{"parcel-1"}

	if Fingerprint(ModePlatform, base) == Fingerprint(ModePlatform, loaded) {
		t.Fatal("changing which parcels ride a mission must change the fingerprint")
	}
}
