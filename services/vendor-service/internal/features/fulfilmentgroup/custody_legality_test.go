package fulfilmentgroup

import (
	"errors"
	"testing"

	"cosmicforge/logistics/shared/go/custodychain"
)

func refusalFor(t *testing.T, event CustodyEvent, from CustodianType) custodychain.Refusal {
	t.Helper()
	err := checkCustodyTransition(event, from)
	if err == nil {
		return custodychain.RefusalNone
	}
	var refused *CustodyRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("unexpected error type %T: %v", err, err)
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatal("a refused custody transition must read as an invalid request")
	}
	return refused.Refusal
}

// The defect this gate exists for: a logistics authority reporting DELIVERED
// for a parcel still sitting with the vendor used to be recorded as
// VENDOR -> CUSTOMER and advanced the parcel to DELIVERED, with no provider
// ever having collected it.
func TestPlatformDeliveryCannotSkipTheProvider(t *testing.T) {
	event := CustodyEvent{
		EventType:            CustodyDelivered,
		ToCustodianType:      CustodianCustomer,
		PlatformMissionRefID: "pmr_1",
	}
	if got := refusalFor(t, event, CustodianVendor); got != custodychain.RefusalSkipsProvider {
		t.Fatalf("expected %s, got %s", custodychain.RefusalSkipsProvider, got)
	}
}

func TestPlatformDeliveryFromTheProviderIsAllowed(t *testing.T) {
	event := CustodyEvent{
		EventType:            CustodyDelivered,
		ToCustodianType:      CustodianCustomer,
		PlatformMissionRefID: "pmr_1",
	}
	if got := refusalFor(t, event, CustodianPlatform); got != custodychain.RefusalNone {
		t.Fatalf("a normal delivery was refused: %s", got)
	}
}

// Vendor-owned delivery legitimately goes straight from the vendor to the
// customer: the vendor's own staff carried the goods and no platform provider
// was ever involved. It is recognised by the event naming a vendor mission.
func TestVendorOwnedDeliveryIsAllowedStraightToTheCustomer(t *testing.T) {
	event := CustodyEvent{
		EventType:       CustodyDelivered,
		ToCustodianType: CustodianCustomer,
		VendorMissionID: "vm_1",
	}
	if got := refusalFor(t, event, CustodianVendor); got != custodychain.RefusalNone {
		t.Fatalf("vendor-owned delivery was refused: %s", got)
	}
}

// A vendor-owned handover is to the vendor's own rider, so custody stays with
// the vendor. That must not be read as a pickup that never happened.
func TestVendorOwnedHandoverKeepsCustodyWithTheVendor(t *testing.T) {
	event := CustodyEvent{
		EventType:       CustodyHandedOver,
		ToCustodianType: CustodianVendor,
		VendorMissionID: "vm_1",
	}
	if got := refusalFor(t, event, CustodianVendor); got != custodychain.RefusalNone {
		t.Fatalf("a vendor-owned handover was refused: %s", got)
	}
}

func TestPlatformPickupMovesVendorToProvider(t *testing.T) {
	event := CustodyEvent{
		EventType:            CustodyHandedOver,
		ToCustodianType:      CustodianPlatform,
		PlatformMissionRefID: "pmr_1",
	}
	if got := refusalFor(t, event, CustodianVendor); got != custodychain.RefusalNone {
		t.Fatalf("a normal pickup was refused: %s", got)
	}
}

// A provider swap is legal only under an explicit transfer. Reporting it as a
// pickup is the silent reassignment CFC-019 forbids.
func TestProviderSwapNeedsAnExplicitTransfer(t *testing.T) {
	transfer := CustodyEvent{
		EventType:            CustodyTransferred,
		ToCustodianType:      CustodianPlatform,
		PlatformMissionRefID: "pmr_1",
	}
	if got := refusalFor(t, transfer, CustodianPlatform); got != custodychain.RefusalNone {
		t.Fatalf("an explicit provider transfer was refused: %s", got)
	}

	asPickup := CustodyEvent{
		EventType:            CustodyHandedOver,
		ToCustodianType:      CustodianPlatform,
		PlatformMissionRefID: "pmr_1",
	}
	if got := refusalFor(t, asPickup, CustodianPlatform); got != custodychain.RefusalWrongOperation {
		t.Fatalf("expected %s, got %s", custodychain.RefusalWrongOperation, got)
	}
}

func TestCustodyCannotComeBackOutOfTheCustomer(t *testing.T) {
	event := CustodyEvent{
		EventType:            CustodyHandedOver,
		ToCustodianType:      CustodianPlatform,
		PlatformMissionRefID: "pmr_1",
	}
	got := refusalFor(t, event, CustodianCustomer)
	if got != custodychain.RefusalWrongOperation && got != custodychain.RefusalReversesTerminal {
		t.Fatalf("a parcel was taken back from the customer: %s", got)
	}
}

func TestFailedAttemptLeavesTheProviderHoldingTheParcel(t *testing.T) {
	event := CustodyEvent{
		EventType:            CustodyFailed,
		ToCustodianType:      CustodianPlatform,
		PlatformMissionRefID: "pmr_1",
	}
	if got := refusalFor(t, event, CustodianPlatform); got != custodychain.RefusalNone {
		t.Fatalf("a failed delivery attempt was refused: %s", got)
	}
}

func TestReturnTravelsBackToTheVendor(t *testing.T) {
	initiated := CustodyEvent{
		EventType:            CustodyReturnInitiated,
		ToCustodianType:      CustodianPlatform,
		PlatformMissionRefID: "pmr_1",
	}
	if got := refusalFor(t, initiated, CustodianPlatform); got != custodychain.RefusalNone {
		t.Fatalf("a return could not start: %s", got)
	}
	completed := CustodyEvent{
		EventType:            CustodyReturned,
		ToCustodianType:      CustodianVendor,
		PlatformMissionRefID: "pmr_1",
	}
	if got := refusalFor(t, completed, CustodianPlatform); got != custodychain.RefusalNone {
		t.Fatalf("a return could not complete: %s", got)
	}
}

func TestParcelCreationEntersTheChain(t *testing.T) {
	event := CustodyEvent{EventType: CustodyCreated, ToCustodianType: CustodianVendor}
	if got := refusalFor(t, event, CustodianNone); got != custodychain.RefusalNone {
		t.Fatalf("a parcel could not be created: %s", got)
	}
}

func TestUnknownCustodianIsRefused(t *testing.T) {
	event := CustodyEvent{EventType: CustodyDelivered, ToCustodianType: CustodianType("WAREHOUSE")}
	if got := refusalFor(t, event, CustodianPlatform); got != custodychain.RefusalUnknownParty {
		t.Fatalf("expected %s, got %s", custodychain.RefusalUnknownParty, got)
	}
}
