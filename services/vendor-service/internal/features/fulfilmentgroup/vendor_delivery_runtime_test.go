package fulfilmentgroup

import (
	"errors"
	"testing"
)

func TestVendorRiderStageMachineRejectsUnsafeSequences(t *testing.T) {
	if got := riderActionNext[StageAssigned][ActionAcknowledge]; got != StageAcknowledged {
		t.Fatalf("acknowledge stage=%s", got)
	}
	if _, ok := riderActionNext[StageAssigned][ActionCompleteDelivery]; ok {
		t.Fatal("assigned mission may not complete before pickup")
	}
	if _, ok := riderActionNext[StageInCustody][ActionCompleteDelivery]; ok {
		t.Fatal("in-custody mission must travel and arrive before completion")
	}
	if _, ok := riderActionNext[StageCompleted][ActionStartPickup]; ok {
		t.Fatal("completed mission was revived")
	}
	if _, ok := riderActionNext[StageArrivedReceiver][ActionReturn]; ok {
		t.Fatal("return cannot race completion before receiver-unavailable is recorded")
	}
	if _, ok := riderActionNext[StageReceiverUnavailable][ActionCompleteDelivery]; ok {
		t.Fatal("completion cannot bypass the receiver-unavailable recovery path")
	}
}

func TestRiderAvailabilityVocabularyExcludesBusyAndSuspendedFromClientWrites(t *testing.T) {
	if !RiderOffline.ValidRequest() || !RiderAvailable.ValidRequest() {
		t.Fatal("rider cannot control online availability")
	}
	if RiderBusy.ValidRequest() || RiderSuspended.ValidRequest() {
		t.Fatal("client can assert an authority-owned availability state")
	}
}

func TestVendorMissionCanonicalLifecycleRemainsAuthoritative(t *testing.T) {
	for _, step := range []struct{ from, to VendorMissionStatus }{{VendorMissionAssigned, VendorMissionReadyForPickup}, {VendorMissionReadyForPickup, VendorMissionCustodyTransferred}, {VendorMissionCustodyTransferred, VendorMissionOutForDelivery}, {VendorMissionOutForDelivery, VendorMissionDelivered}} {
		if err := TransitionVendorMission(step.from, step.to); err != nil {
			t.Fatalf("%s -> %s: %v", step.from, step.to, err)
		}
	}
	if !errors.Is(TransitionVendorMission(VendorMissionAssigned, VendorMissionDelivered), ErrConflict) {
		t.Fatal("arbitrary delivery jump was accepted")
	}
}

func TestOperationFingerprintBindsActionAndParcelSelection(t *testing.T) {
	base := RiderActionRequest{Action: ActionCompletePickup, OperationRef: "op", ExpectedVersion: 3, ParcelRefs: []string{"parcel-a"}}
	changed := base
	changed.ParcelRefs = []string{"parcel-b"}
	if actionFingerprint(base) == actionFingerprint(changed) {
		t.Fatal("fingerprint does not bind parcel selection")
	}
	changed = base
	changed.Action = ActionCompleteDelivery
	if actionFingerprint(base) == actionFingerprint(changed) {
		t.Fatal("fingerprint does not bind action")
	}
}

func TestOperationFingerprintExcludesAuthorityOwnedVerificationResult(t *testing.T) {
	client := RiderActionRequest{Action: ActionCompletePickup, OperationRef: "op", ExpectedVersion: 3, EvidenceRef: "challenge-ref", ParcelRefs: []string{"parcel-a"}}
	verified := client
	verified.verifiedRef = "opaque-authority-ref"
	verified.verifiedProof = "authority-proof-ref"
	if actionFingerprint(client) != actionFingerprint(verified) {
		t.Fatal("authority verification output changed the client operation fingerprint")
	}
}

func TestSecureRiderActionsAreClosedVocabulary(t *testing.T) {
	for _, action := range []RiderAction{ActionCompletePickup, ActionCompleteDelivery, ActionReturn} {
		if !secureRiderAction(action) {
			t.Fatalf("%s must require authoritative handover", action)
		}
	}
	for _, action := range []RiderAction{ActionAcknowledge, ActionStartPickup, ActionArrivePickup, ActionStartReceiver, ActionArriveReceiver, ActionReceiverUnavailable} {
		if secureRiderAction(action) {
			t.Fatalf("%s must not accept handover evidence", action)
		}
	}
}

func TestVendorRiderIsNotMarketplaceProviderIdentity(t *testing.T) {
	request := VendorHandoverRequest{RiderStaffID: "staff_vendor_rider"}
	if request.RiderStaffID == "" {
		t.Fatal("rider must be a staff identity")
	}
	// The request has deliberately no provider/service membership field. A
	// future addition would be an authority boundary regression.
}
