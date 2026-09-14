package fulfilmentgroup

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/custodychain"
)

// custodySeed packs one parcel on an active platform plan and returns it.
func custodySeed(t *testing.T, store *Store) (Parcel, string) {
	t.Helper()
	ctx := context.Background()
	group, unit := seedGroup(t, store)
	plan, err := store.ActivatePlan(ctx, group.ID, PlanInput{
		ExpectedVersion: 0, Reason: ReasonInitialPlan, DeliveryMode: ModePlatform,
		IdempotencyKey: "custody-activation",
		Missions:       []PlanMission{slotFor(unit.ID, "slot-1", 1)},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("activate plan: %v", err)
	}
	ref, _, err := store.EnsurePlatformMissionRef(
		ctx, group.ID, plan.ID, plan.Missions[0].ID, AuthorityDispatch, time.Now().UTC())
	if err != nil {
		t.Fatalf("reserve mission: %v", err)
	}
	parcel, err := store.CreateParcel(ctx, unit.BusinessID, unit.ID, ParcelInput{
		IdempotencyKey: "custody-pack",
		Manifest:       Manifest{PackageCount: 1},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("pack parcel: %v", err)
	}
	return parcel, ref.ID
}

// The chain must refuse a delivery reported for a parcel no provider ever
// collected. Before this gate the row was written and the parcel advanced to
// DELIVERED while it was still on the vendor's counter.
func TestIntegration_DeliveryWithoutPickupIsRefusedAndLeavesNoTrace(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	parcel, missionRef := custodySeed(t, store)
	ctx := context.Background()

	_, err := store.RecordCustody(ctx, CustodyEvent{
		ParcelID:             parcel.ID,
		GroupID:              parcel.GroupID,
		EventType:            CustodyDelivered,
		ToCustodianType:      CustodianCustomer,
		Source:               SourceDispatch,
		SourceEventID:        "evt-impossible-delivery",
		SourceVersion:        1,
		PlatformMissionRefID: missionRef,
		OccurredAt:           time.Now().UTC(),
	}, time.Now().UTC())
	if err == nil {
		t.Fatal("a delivery that skipped the provider was accepted")
	}
	var refused *CustodyRefusedError
	if !errors.As(err, &refused) || refused.Refusal != custodychain.RefusalSkipsProvider {
		t.Fatalf("err = %v; want %s", err, custodychain.RefusalSkipsProvider)
	}

	// Nothing was written: the parcel is still where it was, and the chain
	// does not carry an impossible link.
	current, err := store.GetParcel(ctx, parcel.BusinessID, parcel.ID)
	if err != nil {
		t.Fatalf("read parcel: %v", err)
	}
	if current.Status == ParcelDelivered || current.CustodianType == CustodianCustomer {
		t.Fatalf("the parcel moved anyway: status=%s custodian=%s",
			current.Status, current.CustodianType)
	}
	events, err := store.ListCustodyEvents(ctx, parcel.ID)
	if err != nil {
		t.Fatalf("list custody: %v", err)
	}
	for _, event := range events {
		if event.SourceEventID == "evt-impossible-delivery" {
			t.Fatal("the refused transition was recorded in the chain")
		}
	}
}

// The legal sequence still works end to end, and the refusal above is about
// the missing pickup rather than deliveries in general.
func TestIntegration_PickupThenDeliveryIsRecorded(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	parcel, missionRef := custodySeed(t, store)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, step := range []struct {
		event  CustodyEventType
		to     CustodianType
		source string
	}{
		{CustodyHandedOver, CustodianPlatform, "evt-pickup"},
		{CustodyDelivered, CustodianCustomer, "evt-delivery"},
	} {
		if _, err := store.RecordCustody(ctx, CustodyEvent{
			ParcelID:             parcel.ID,
			GroupID:              parcel.GroupID,
			EventType:            step.event,
			ToCustodianType:      step.to,
			Source:               SourceDispatch,
			SourceEventID:        step.source,
			SourceVersion:        1,
			PlatformMissionRefID: missionRef,
			OccurredAt:           now,
		}, now); err != nil {
			t.Fatalf("%s: %v", step.event, err)
		}
	}

	delivered, err := store.GetParcel(ctx, parcel.BusinessID, parcel.ID)
	if err != nil {
		t.Fatalf("read parcel: %v", err)
	}
	if delivered.Status != ParcelDelivered || delivered.CustodianType != CustodianCustomer {
		t.Fatalf("status=%s custodian=%s; want DELIVERED with the customer",
			delivered.Status, delivered.CustodianType)
	}
}

// A duplicate or late report must stay a silent no-op rather than becoming an
// error: an at-least-once pipeline redelivers, and the gate only judges
// events that would actually move the parcel forward.
func TestIntegration_LateHandoverAfterDeliveryStaysANoOp(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	parcel, missionRef := custodySeed(t, store)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, step := range []struct {
		event  CustodyEventType
		to     CustodianType
		source string
	}{
		{CustodyHandedOver, CustodianPlatform, "evt-pickup"},
		{CustodyDelivered, CustodianCustomer, "evt-delivery"},
	} {
		if _, err := store.RecordCustody(ctx, CustodyEvent{
			ParcelID: parcel.ID, GroupID: parcel.GroupID, EventType: step.event,
			ToCustodianType: step.to, Source: SourceDispatch, SourceEventID: step.source,
			SourceVersion: 1, PlatformMissionRefID: missionRef, OccurredAt: now,
		}, now); err != nil {
			t.Fatalf("%s: %v", step.event, err)
		}
	}

	// The same pickup arriving again under a new event id would move
	// RECEIVER -> PROVIDER if it were applied. It advances nothing, so it is
	// absorbed rather than refused.
	if _, err := store.RecordCustody(ctx, CustodyEvent{
		ParcelID: parcel.ID, GroupID: parcel.GroupID, EventType: CustodyHandedOver,
		ToCustodianType: CustodianPlatform, Source: SourceDispatch,
		SourceEventID: "evt-pickup-redelivered", SourceVersion: 1,
		PlatformMissionRefID: missionRef, OccurredAt: now,
	}, now); err != nil {
		t.Fatalf("a late redelivery was rejected instead of absorbed: %v", err)
	}

	after, err := store.GetParcel(ctx, parcel.BusinessID, parcel.ID)
	if err != nil {
		t.Fatalf("read parcel: %v", err)
	}
	if after.Status != ParcelDelivered || after.CustodianType != CustodianCustomer {
		t.Fatalf("a late event regressed the parcel: status=%s custodian=%s",
			after.Status, after.CustodianType)
	}
}
