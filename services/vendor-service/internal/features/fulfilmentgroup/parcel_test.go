package fulfilmentgroup

import (
	"testing"
	"time"
)

// An at-least-once pipeline redelivers events as a matter of course. The
// projection absorbs the repeat rather than walking the journey backwards.
func TestAdvanceParcelStatus_NeverRegresses(t *testing.T) {
	if AdvanceParcelStatus(ParcelDelivered, ParcelInTransit) {
		t.Fatal("a delayed IN_TRANSIT must not un-deliver a delivered parcel")
	}
	if AdvanceParcelStatus(ParcelDelivered, ParcelDelivered) {
		t.Fatal("a duplicate DELIVERED must not reapply")
	}
	if !AdvanceParcelStatus(ParcelReady, ParcelInTransit) {
		t.Fatal("a genuine forward step must apply")
	}
}

// A return is a LATER fact than the failed delivery that caused it, and must
// not be overwritten by a delayed report of that failure.
func TestAdvanceParcelStatus_ReturnOutranksFailure(t *testing.T) {
	if AdvanceParcelStatus(ParcelReturning, ParcelFailed) {
		t.Fatal("a late failure report must not reopen a parcel already coming back")
	}
	if !AdvanceParcelStatus(ParcelFailed, ParcelReturnRequired) {
		t.Fatal("a failed delivery may progress to a required return")
	}
}

func TestParcelStatus_InCustodySealsTheManifest(t *testing.T) {
	preCustody := []ParcelStatus{ParcelCreated, ParcelPacked, ParcelReady, ParcelCancelled}
	inCustody := []ParcelStatus{
		ParcelInTransit, ParcelDelivered, ParcelReturning, ParcelReturned,
		ParcelReturnRequired, ParcelFailed,
	}

	for _, status := range preCustody {
		if status.InCustody() {
			t.Fatalf("%s is still in the vendor's hands", status)
		}
		if (Parcel{Status: status}).Sealed() {
			t.Fatalf("%s must still be amendable", status)
		}
	}
	for _, status := range inCustody {
		if !status.InCustody() {
			t.Fatalf("%s has left the vendor's hands", status)
		}
		if !(Parcel{Status: status}).Sealed() {
			t.Fatalf("%s must be sealed: its manifest is now history", status)
		}
	}
}

func TestParcel_SealedByExplicitTimestamp(t *testing.T) {
	sealed := time.Now()
	parcel := Parcel{
		Status:   ParcelPacked,
		Manifest: Manifest{SealedAt: &sealed},
	}
	if !parcel.Sealed() {
		t.Fatal("an explicitly sealed manifest is history regardless of status")
	}
}

func TestParcelStatusFor_MapsCustodyEvents(t *testing.T) {
	cases := map[CustodyEventType]ParcelStatus{
		CustodyCreated:         ParcelCreated,
		CustodyPacked:          ParcelPacked,
		CustodyReady:           ParcelReady,
		CustodyHandedOver:      ParcelInTransit,
		CustodyTransferred:     ParcelInTransit,
		CustodyDelivered:       ParcelDelivered,
		CustodyFailed:          ParcelFailed,
		CustodyReturnInitiated: ParcelReturning,
		CustodyReturned:        ParcelReturned,
	}
	for event, want := range cases {
		if got := ParcelStatusFor(event); got != want {
			t.Fatalf("%s -> %s; want %s", event, got, want)
		}
	}
}

// A provider transfer keeps the parcel in transit under a new custodian. It is
// not a delivery followed by a brand-new parcel.
func TestCustodyTransferKeepsParcelInTransit(t *testing.T) {
	if ParcelStatusFor(CustodyTransferred) != ParcelInTransit {
		t.Fatal("a transfer must leave the parcel in transit")
	}
	if AdvanceParcelStatus(ParcelInTransit, ParcelInTransit) {
		t.Fatal("a transfer must not re-advance an already in-transit parcel")
	}
}

// Movement is executed by exactly one mission: neither leaves the event
// unattributable, both leaves it unclear who was carrying it.
func TestCustodyEvent_MovementNeedsExactlyOneMission(t *testing.T) {
	base := CustodyEvent{
		ParcelID:        "p1",
		EventType:       CustodyHandedOver,
		ToCustodianType: CustodianPlatform,
		Source:          SourceDispatch,
		SourceEventID:   "evt-1",
		SourceVersion:   1,
		OccurredAt:      time.Now(),
	}

	if err := base.Validate(); err == nil {
		t.Fatal("a movement event naming no mission must be refused")
	}

	both := base
	both.PlatformMissionRefID, both.VendorMissionID = "ref-1", "vm-1"
	if err := both.Validate(); err == nil {
		t.Fatal("a movement event naming two missions must be refused")
	}

	one := base
	one.PlatformMissionRefID = "ref-1"
	if err := one.Validate(); err != nil {
		t.Fatalf("a well-formed movement event was refused: %v", err)
	}
}

// Pre-custody events happen inside the vendor and name no mission at all.
func TestCustodyEvent_PreCustodyNeedsNoMission(t *testing.T) {
	event := CustodyEvent{
		ParcelID:        "p1",
		EventType:       CustodyCreated,
		ToCustodianType: CustodianVendor,
		Source:          SourceVendor,
		SourceEventID:   "parcel-created-p1",
		OccurredAt:      time.Now(),
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("pre-custody event refused: %v", err)
	}
}

func TestManifest_RoundTrips(t *testing.T) {
	manifest := Manifest{
		Lines: []ManifestLine{
			{OrderLineRef: "line-1", ItemRef: "item-1", Quantity: 2, UnitRef: "EA"},
			{OrderLineRef: "line-2", MeasuredQuantity: 750, UnitRef: "G"},
		},
		PackageCount: 3,
		Notes:        "handle upright",
	}

	encoded, err := manifest.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded := DecodeManifest(encoded)

	if len(decoded.Lines) != 2 || decoded.PackageCount != 3 {
		t.Fatalf("manifest did not round-trip: %+v", decoded)
	}
	if decoded.Lines[1].MeasuredQuantity != 750 {
		t.Fatal("variable-measure evidence must survive the round trip")
	}
}

// A read of a delivered parcel must not fail because of one malformed
// historical row.
func TestDecodeManifest_MalformedIsEmptyNotFatal(t *testing.T) {
	if lines := DecodeManifest([]byte("not json")).Lines; lines != nil {
		t.Fatal("a malformed manifest must decode as empty")
	}
}

func TestManifest_RejectsUnreferencedLine(t *testing.T) {
	manifest := Manifest{Lines: []ManifestLine{{Quantity: 1}}}
	if err := manifest.Validate(); err != ErrInvalid {
		t.Fatal("a manifest line with no order line reference must be refused")
	}
}

func TestParcelInput_RequiresIdempotencyKey(t *testing.T) {
	if err := (ParcelInput{}).Validate(); err != ErrInvalid {
		t.Fatal("packing without a stable operation key would create a second box on retry")
	}
	if err := (ParcelInput{IdempotencyKey: "pack-1"}).Validate(); err != nil {
		t.Fatalf("valid packing input refused: %v", err)
	}
}

func TestAdvanceReadiness_ForwardOnlyExceptCancel(t *testing.T) {
	if err := AdvanceReadiness(ReadinessReady, ReadinessPacking); err != ErrConflict {
		t.Fatal("readiness must not move backwards")
	}
	if err := AdvanceReadiness(ReadinessNotReady, ReadinessReady); err != nil {
		t.Fatalf("forward readiness refused: %v", err)
	}
	if err := AdvanceReadiness(ReadinessPacking, ReadinessCancelled); err != nil {
		t.Fatalf("cancelling a not-yet-collected obligation refused: %v", err)
	}
	// Cancelling after collection would contradict custody evidence the
	// logistics authority already recorded.
	if err := AdvanceReadiness(ReadinessCollected, ReadinessCancelled); err != ErrConflict {
		t.Fatal("a collected obligation must not be cancelled")
	}
}
