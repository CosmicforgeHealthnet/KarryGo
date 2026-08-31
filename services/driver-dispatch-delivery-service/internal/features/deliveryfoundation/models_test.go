package deliveryfoundation

import "testing"

func TestSourceAndArchitectureFoundation(t *testing.T) {
	source := Source{Type: SourceCustomer, Ref: "customer-order-1", Version: 1, BookingID: "booking-1"}
	group := DeliveryGroup{ID: "group-1", CustomerID: "customer-1", Source: source, State: GroupDraft, Version: 1}
	if err := group.Validate(); err != nil {
		t.Fatal(err)
	}
	mission := DeliveryMission{ID: "mission-1", DeliveryGroupID: group.ID, Source: source, State: MissionPlanned, Version: 1}
	if err := mission.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStopSequence([]MissionStop{{ID: "pickup", MissionID: mission.ID, Sequence: 1, Type: StopPickup}, {ID: "delivery", MissionID: mission.ID, Sequence: 2, Type: StopDelivery}}); err != nil {
		t.Fatal(err)
	}
	parcel := Parcel{ID: "parcel-1", DeliveryGroupID: group.ID, State: ParcelDeclared, Version: 1}
	if err := parcel.Validate(); err != nil {
		t.Fatal(err)
	}
	manifest := ParcelManifest{ID: "manifest-1", MissionID: mission.ID, Version: 1, ParcelRefs: []string{parcel.ID}}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestArchitectureFoundationRejectsInvalidInput(t *testing.T) {
	if (Source{Type: "CALLER_CHOSEN", Ref: "x", Version: 1}).Validate() == nil {
		t.Fatal("unknown source accepted")
	}
	if ValidateStopSequence([]MissionStop{{ID: "a", MissionID: "m", Sequence: 2, Type: StopPickup}}) == nil {
		t.Fatal("non-contiguous sequence accepted")
	}
	if (ParcelManifest{ID: "m", MissionID: "mission", Version: 1, ParcelRefs: []string{"p", "p"}}).Validate() == nil {
		t.Fatal("duplicate parcel accepted")
	}
}

func TestStateTransitionsAndCompletionInvariant(t *testing.T) {
	if !GroupDraft.CanTransition(GroupServiceability) || GroupDraft.CanTransition(GroupCompleted) {
		t.Fatal("group transition policy incorrect")
	}
	if !MissionPlanned.CanTransition(MissionQueued) || MissionCompleted.CanTransition(MissionQueued) {
		t.Fatal("mission transition policy incorrect")
	}
	if !ParcelDeclared.CanTransition(ParcelReadyForPickup) || ParcelDelivered.CanTransition(ParcelInTransit) {
		t.Fatal("parcel transition policy incorrect")
	}
	if !ChallengeActive.CanTransition(ChallengeConsumed) || ChallengeConsumed.CanTransition(ChallengeActive) {
		t.Fatal("challenge transition policy incorrect")
	}
	if ValidateCompletion(GroupSettlementPending, true, true) != nil {
		t.Fatal("valid completion rejected")
	}
	if ValidateCompletion(GroupSettlementPending, true, false) == nil {
		t.Fatal("completion without financial acknowledgement accepted")
	}
	if GroupState("UNKNOWN").Valid() || MissionState("UNKNOWN").Valid() || ParcelState("UNKNOWN").Valid() {
		t.Fatal("unknown state accepted")
	}
}
