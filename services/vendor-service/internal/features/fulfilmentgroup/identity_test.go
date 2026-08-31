package fulfilmentgroup

// Identity preservation.
//
// These are the tests for the property the whole aggregate exists to protect:
// a parcel, a unit and a group keep their identity through every operational
// change that happens to them. Each case below is something a human is already
// tracking when the change occurs, so a new identity is not a refactor — it is
// the customer, the vendor and support each looking at a different thing.

import (
	"testing"
	"time"
)

// A parcel is created once and is still the same parcel after everything that
// can happen to it: replan, reassignment, mission replacement, transfer,
// return, recovery, retry.
func TestParcelIdentitySurvivesTheWholeJourney(t *testing.T) {
	const parcelID = "cfp_fixed"
	const parcelRef = "PKG-UNIT01-1"

	parcel := Parcel{
		ID:            parcelID,
		ParcelRef:     parcelRef,
		UnitID:        "cfu_1",
		VendorOrderID: "vo-1",
		BusinessID:    "biz-a",
		BranchID:      "br-a",
		Status:        ParcelCreated,
		CustodianType: CustodianVendor,
	}

	// The journey, in order. Each step is a status projection derived from
	// custody evidence; none of them mints a new identity.
	journey := []struct {
		what  string
		event CustodyEventType
	}{
		{"packed", CustodyPacked},
		{"ready for collection", CustodyReady},
		{"handed to the first provider", CustodyHandedOver},
		{"transferred to a second provider", CustodyTransferred},
		{"delivery failed", CustodyFailed},
		{"return initiated", CustodyReturnInitiated},
		{"returned to the vendor", CustodyReturned},
	}

	for _, step := range journey {
		next := ParcelStatusFor(step.event)
		if AdvanceParcelStatus(parcel.Status, next) {
			parcel.Status = next
		}
		if parcel.ID != parcelID {
			t.Fatalf("after %s the parcel id changed to %q", step.what, parcel.ID)
		}
		if parcel.ParcelRef != parcelRef {
			t.Fatalf("after %s the parcel reference changed to %q", step.what, parcel.ParcelRef)
		}
		if parcel.VendorOrderID != "vo-1" {
			t.Fatalf("after %s the parcel lost its vendor order", step.what)
		}
		if parcel.BusinessID != "biz-a" || parcel.BranchID != "br-a" {
			t.Fatalf("after %s the parcel changed source vendor/branch", step.what)
		}
	}

	if parcel.Status != ParcelReturned {
		t.Fatalf("final status = %s; want RETURNED", parcel.Status)
	}
}

// A replan moves work to a different authority. The unit, its vendor order and
// its parcels are untouched: which vehicle collects the goods is not a fact
// about what was purchased.
func TestReplanChangesTheMissionNotTheIdentities(t *testing.T) {
	unit := Unit{ID: "cfu_1", VendorOrderID: "vo-1", BusinessID: "biz-a", BranchID: "br-a"}

	v1 := PlanMission{
		SlotRef: "slot-1", Sequence: 1,
		Owner: OwnerPlatform, Authority: AuthorityDispatch,
		Stops: []Stop{
			{Sequence: 1, Type: StopPickup, UnitID: unit.ID, BranchID: unit.BranchID},
			{Sequence: 2, Type: StopDropoff},
		},
		ParcelIDs: []string{"cfp_1"},
	}

	// The vehicle class overflowed: the same goods now need a truck.
	v2 := v1
	v2.Authority = AuthorityHauling
	v2.VehicleClass = "TRUCK"

	if err := v2.Validate(); err != nil {
		t.Fatalf("replanned mission invalid: %v", err)
	}
	if got := v2.UnitIDs(); len(got) != 1 || got[0] != unit.ID {
		t.Fatalf("replan changed the unit: %v", got)
	}
	if len(v2.ParcelIDs) != 1 || v2.ParcelIDs[0] != "cfp_1" {
		t.Fatalf("replan changed the parcel: %v", v2.ParcelIDs)
	}

	// The plan really did change, and the fingerprint says so — which is what
	// makes a replan distinguishable from a retry of the same decision.
	if Fingerprint(ModePlatform, []PlanMission{v1}) ==
		Fingerprint(ModePlatform, []PlanMission{v2}) {
		t.Fatal("a replan onto another authority must change the plan fingerprint")
	}
}

// A provider change INSIDE one canonical mission is the logistics service's own
// business. Nothing in this aggregate reacts to it: same mission reference,
// same parcel, same everything.
func TestProviderReassignmentInsideOneMissionChangesNothingHere(t *testing.T) {
	linked := time.Now()
	ref := PlatformMissionRef{
		ID:                  "cpmr_1",
		MissionRef:          "cm-abc123",
		ExternalRef:         "bk_999",
		Authority:           AuthorityDispatch,
		LinkState:           LinkLinked,
		StatusProjection:    "IN_TRANSIT",
		StatusSourceVersion: 4,
		LinkedAt:            &linked,
	}

	before := ref

	// The authority reports progress under a different rider. It is a newer
	// version of the SAME mission, so only the projection moves.
	if !ref.AcceptStatusProjection(5) {
		t.Fatal("a newer report from the same mission must be applied")
	}
	ref.StatusProjection, ref.StatusSourceVersion = "IN_TRANSIT", 5

	if ref.ID != before.ID || ref.MissionRef != before.MissionRef ||
		ref.ExternalRef != before.ExternalRef {
		t.Fatal("a provider change inside one mission must not change the mission reference")
	}
	if !ref.Live() {
		t.Fatal("the mission is still the one executing the slot")
	}
}

// A mission REPLACEMENT is different: the old reference stays readable, because
// the mission it names really was requested and possibly executed.
func TestMissionReplacementKeepsTheOldReferenceReadable(t *testing.T) {
	old := PlatformMissionRef{
		ID: "cpmr_1", MissionRef: "cm-old", ExternalRef: "bk_1",
		Authority: AuthorityDispatch, LinkState: LinkLinked,
	}

	replaced := old
	replaced.LinkState = LinkReplaced
	replaced.ReplacementReason = "authority cancelled the booking"

	replacement := PlatformMissionRef{
		ID: "cpmr_2", MissionRef: "cm-new", Authority: AuthorityDispatch,
		LinkState: LinkRequested, ReplacesID: old.ID,
	}

	if replaced.ExternalRef != "bk_1" {
		t.Fatal("the replaced mission must keep the canonical reference it really had")
	}
	if replaced.Live() {
		t.Fatal("a replaced mission no longer executes the slot")
	}
	if !replacement.Live() {
		t.Fatal("the replacement executes the slot now")
	}
	if replacement.ReplacesID != old.ID {
		t.Fatal("the replacement must name what it replaced, or the chain breaks")
	}
}

// A return does not create a replacement parcel. It creates a return RECORD
// that names the same parcel, and may name a brand-new logistics mission that
// carries it back.
func TestReturnNamesTheSameParcelAndMayNameANewMission(t *testing.T) {
	parcelReturn := ParcelReturn{
		ID:                         "cpr_1",
		ParcelID:                   "cfp_1",
		ReasonCode:                 "RECIPIENT_UNAVAILABLE",
		OriginPlatformMissionRefID: "cpmr_1",
		State:                      ReturnInitiated,
	}

	// The return leg is a different mission. That is expected and correct.
	parcelReturn.ReturnPlatformMissionRefID = "cpmr_2"
	parcelReturn.State = ReturnInProcess

	if parcelReturn.ParcelID != "cfp_1" {
		t.Fatal("a return must name the parcel that went out, not a copy of it")
	}
	if parcelReturn.OriginPlatformMissionRefID == parcelReturn.ReturnPlatformMissionRefID {
		t.Fatal("the outbound and return legs are different missions")
	}
	if parcelReturn.OriginPlatformMissionRefID == "" {
		t.Fatal("a return must remember which mission failed to deliver")
	}
}

// One commerce order has one group, forever. Replanning appends a version under
// the same group rather than starting a new one.
func TestGroupSurvivesReplanning(t *testing.T) {
	group := Group{
		ID: "cfg_1", ParentOrderID: "po-1",
		CurrentPlanID: "cfpv_1", CurrentPlanVersion: 1, Version: 1,
	}
	if !group.Planned() {
		t.Fatal("a group with an active version has been planned")
	}

	// Two replans later.
	group.CurrentPlanID, group.CurrentPlanVersion = "cfpv_3", 3
	group.Version = 3

	if group.ID != "cfg_1" || group.ParentOrderID != "po-1" {
		t.Fatal("replanning must not change the group identity")
	}
	if group.CurrentPlanVersion != 3 {
		t.Fatalf("current version = %d; want 3", group.CurrentPlanVersion)
	}
}

// An unplanned group is honestly unplanned: version zero is not version one
// that happens to be superseded.
func TestGroupBeforePlanningIsNotPlanned(t *testing.T) {
	if (Group{ID: "cfg_1", ParentOrderID: "po-1"}).Planned() {
		t.Fatal("a group with no plan version has not been planned")
	}
	if (Group{ID: "cfg_1", CurrentPlanVersion: 1}).Planned() {
		t.Fatal("a version with no plan id is an inconsistent pointer, not a plan")
	}
}

// Cross-vendor parcels on one mission stay separate. Only the vehicle is
// shared; the sources are not merged.
func TestCrossVendorParcelsKeepSeparateSources(t *testing.T) {
	fromA := Parcel{ID: "cfp_a", BusinessID: "biz-a", BranchID: "br-a", VendorOrderID: "vo-a"}
	fromB := Parcel{ID: "cfp_b", BusinessID: "biz-b", BranchID: "br-b", VendorOrderID: "vo-b"}

	mission := PlanMission{
		SlotRef: "slot-1", Sequence: 1, Owner: OwnerPlatform, Authority: AuthorityDispatch,
		Stops: []Stop{
			{Sequence: 1, Type: StopPickup, UnitID: "cfu_a", BusinessID: "biz-a"},
			{Sequence: 2, Type: StopPickup, UnitID: "cfu_b", BusinessID: "biz-b"},
			{Sequence: 3, Type: StopDropoff},
		},
		ParcelIDs: []string{fromA.ID, fromB.ID},
	}

	if err := mission.Validate(); err != nil {
		t.Fatalf("consolidated mission invalid: %v", err)
	}
	if fromA.BusinessID == fromB.BusinessID {
		t.Fatal("sharing a vehicle must not merge two vendors into one source")
	}
	if len(mission.ParcelIDs) != 2 {
		t.Fatal("two vendors' goods are two parcels, never one consolidated parcel")
	}
}
