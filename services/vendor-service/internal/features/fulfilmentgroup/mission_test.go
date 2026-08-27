package fulfilmentgroup

import (
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
)

// The vendor mission lifecycle IS the delivery feature's ExecutionState. This
// test is what keeps the two from drifting: the vocabulary is restated in this
// package for typing reasons, and a silent divergence would split one journey
// across two spellings.
func TestVendorMissionVocabularyMatchesDeliveryExecutionState(t *testing.T) {
	pairs := []struct {
		mission   VendorMissionStatus
		execution delivery.ExecutionState
	}{
		{VendorMissionPlanned, delivery.ExecutionPlanned},
		{VendorMissionAssigned, delivery.ExecutionAssigned},
		{VendorMissionReadyForPickup, delivery.ExecutionReadyForPickup},
		{VendorMissionCustodyTransferred, delivery.ExecutionCustodyTransferred},
		{VendorMissionOutForDelivery, delivery.ExecutionOutForDelivery},
		{VendorMissionDelivered, delivery.ExecutionDelivered},
		{VendorMissionFailed, delivery.ExecutionFailed},
		{VendorMissionReturnRequired, delivery.ExecutionReturnRequired},
		{VendorMissionReturned, delivery.ExecutionReturned},
	}

	for _, pair := range pairs {
		if string(pair.mission) != string(pair.execution) {
			t.Fatalf("vendor mission status %q does not match delivery execution state %q",
				pair.mission, pair.execution)
		}
	}

	// And the transition tables must agree, not merely the value names.
	all := []VendorMissionStatus{
		VendorMissionPlanned, VendorMissionAssigned, VendorMissionReadyForPickup,
		VendorMissionCustodyTransferred, VendorMissionOutForDelivery,
		VendorMissionDelivered, VendorMissionFailed,
		VendorMissionReturnRequired, VendorMissionReturned,
	}
	for _, from := range all {
		for _, to := range all {
			mine := TransitionVendorMission(from, to) == nil
			theirs := delivery.Transition(
				delivery.ExecutionState(from), delivery.ExecutionState(to)) == nil
			if mine != theirs {
				t.Fatalf("transition %s -> %s: fulfilment allows=%v, delivery allows=%v",
					from, to, mine, theirs)
			}
		}
	}
}

// A status field any caller can set to "delivered" is not delivery evidence.
func TestTransitionVendorMission_RejectsJumpToDelivered(t *testing.T) {
	if err := TransitionVendorMission(VendorMissionPlanned, VendorMissionDelivered); err == nil {
		t.Fatal("a planned mission must not jump straight to delivered")
	}
	if err := TransitionVendorMission(VendorMissionAssigned, VendorMissionDelivered); err == nil {
		t.Fatal("an assigned mission must not jump straight to delivered")
	}
}

func TestTransitionVendorMission_HappyPath(t *testing.T) {
	sequence := []VendorMissionStatus{
		VendorMissionPlanned, VendorMissionAssigned, VendorMissionReadyForPickup,
		VendorMissionCustodyTransferred, VendorMissionOutForDelivery,
		VendorMissionDelivered,
	}
	for index := 0; index+1 < len(sequence); index++ {
		if err := TransitionVendorMission(sequence[index], sequence[index+1]); err != nil {
			t.Fatalf("%s -> %s refused: %v", sequence[index], sequence[index+1], err)
		}
	}
}

// A mission nobody was assigned to cannot report a delivery: there is no rider
// to have made it.
func TestVendorMissionTransition_RequiresAssignedRider(t *testing.T) {
	mission := VendorMission{Status: VendorMissionPlanned}

	err := VendorMissionTransition{To: VendorMissionAssigned}.Validate(mission)
	if err != ErrConflict {
		t.Fatalf("err = %v; want ErrConflict for an unassigned mission", err)
	}

	assigned := time.Now()
	mission.AssignedStaffID = "staff-1"
	mission.AssignedAt = &assigned
	if err = (VendorMissionTransition{To: VendorMissionAssigned}).Validate(mission); err != nil {
		t.Fatalf("assigned mission refused: %v", err)
	}
}

func TestVendorMissionTransition_FailureNeedsAReason(t *testing.T) {
	assigned := time.Now()
	mission := VendorMission{
		Status:          VendorMissionAssigned,
		AssignedStaffID: "staff-1",
		AssignedAt:      &assigned,
	}

	if err := (VendorMissionTransition{To: VendorMissionFailed}).Validate(mission); err != ErrInvalid {
		t.Fatal("a failure with no stated reason must be refused")
	}
	if err := (VendorMissionTransition{
		To: VendorMissionFailed, FailureReason: "rider unavailable",
	}).Validate(mission); err != nil {
		t.Fatalf("failure with a reason refused: %v", err)
	}
}

// The projection must never regress. A delayed IN_TRANSIT cannot un-deliver a
// mission the authority already reported delivered.
func TestAcceptStatusProjection_RefusesStaleAndDuplicate(t *testing.T) {
	ref := PlatformMissionRef{StatusProjection: "DELIVERED", StatusSourceVersion: 7}

	if ref.AcceptStatusProjection(6) {
		t.Fatal("an older source version must not be applied")
	}
	if ref.AcceptStatusProjection(7) {
		t.Fatal("a redelivered event carries the same version and must not reapply")
	}
	if !ref.AcceptStatusProjection(8) {
		t.Fatal("a strictly newer source version must be applied")
	}
	if ref.AcceptStatusProjection(0) {
		t.Fatal("an unversioned event must not be applied")
	}
}

func TestMissionLink_RefusalNeedsACode(t *testing.T) {
	if err := (MissionLink{Accepted: false}).Validate(); err == nil {
		t.Fatal("a refusal with no reason cannot be acted on and must be refused")
	}
	if err := (MissionLink{Accepted: false, RefusalCode: "NO_RIDER"}).Validate(); err != nil {
		t.Fatalf("coded refusal rejected: %v", err)
	}
}

func TestMissionLink_AcceptanceNeedsAnExternalRef(t *testing.T) {
	if err := (MissionLink{Accepted: true}).Validate(); err == nil {
		t.Fatal("an accepted mission with no canonical reference cannot be followed")
	}
	if err := (MissionLink{Accepted: true, ExternalRef: "bk_123"}).Validate(); err != nil {
		t.Fatalf("accepted mission with a reference rejected: %v", err)
	}
}

func TestLinkState_OnlyLiveStatesOccupyTheSlot(t *testing.T) {
	live := []LinkState{LinkRequested, LinkLinked}
	history := []LinkState{LinkRefused, LinkFailed, LinkSuperseded, LinkReplaced}

	for _, state := range live {
		if !state.Occupies() {
			t.Fatalf("%s must occupy its plan mission slot", state)
		}
	}
	for _, state := range history {
		if state.Occupies() {
			t.Fatalf("%s is history and must not occupy the slot", state)
		}
	}
}

func TestOwnerFor_VendorAuthorityImpliesVendorOwner(t *testing.T) {
	if OwnerFor(AuthorityVendor) != OwnerVendor {
		t.Fatal("the vendor authority is executed by the vendor")
	}
	for _, authority := range []Authority{AuthorityDispatch, AuthorityHauling, AuthorityTaxi} {
		if OwnerFor(authority) != OwnerPlatform {
			t.Fatalf("%s must be platform-executed", authority)
		}
	}
}
