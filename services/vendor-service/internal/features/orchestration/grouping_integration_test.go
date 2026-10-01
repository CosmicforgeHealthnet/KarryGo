package orchestration

// The multi-vendor planner against a real PostgreSQL.
//
// The pure grouping algorithm is unit-tested elsewhere. What these tests are
// for is the part that only exists once the rows do: that grouping actually
// RUNS on the evaluation path, that its policy comes from the selected
// authority's own declaration rather than a constant, that its decision is
// persisted per pickup, and that the reason two vendors were separated survives
// as evidence somebody can read later.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	readiness "cosmicforge/logistics/services/vendor-service/internal/features/fulfilment_readiness"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
)

// groupingHarness is the commit harness with a configurable declared
// capability, because the declared pickup limit is the input the whole
// grouping decision turns on.
func groupingHarness(t *testing.T, capability logistics.Capability) *orchestrationHarness {
	t.Helper()
	committer := &recordingCommitter{
		outcome: MissionOutcome{Definite: true, Accepted: true, MissionRef: "mission-group"}}
	h := newOrchestrationHarness(t, committer)
	readyAt := h.now.Add(-time.Minute)
	h.service = NewService(Options{
		Repository: NewRepository(h.pool),
		Gate: staticGate{
			usable: map[logistics.Authority]bool{logistics.Dispatch: true},
			answers: map[logistics.Authority]logistics.Serviceability{
				logistics.Dispatch: {Authority: logistics.Dispatch, Serviceable: true, Version: 3},
			},
			capabilities: map[logistics.Authority]logistics.Capability{
				logistics.Dispatch: capability,
			},
		},
		Preview: staticPreview{},
		// A committer is supplied because these orchestrations are already
		// ready, so an ordinary evaluation walks all the way through to the
		// commit. Grouping happens earlier, but a missing committer would stop
		// the evaluation before the assertion could see anything.
		Committer: committer,
		Readiness: staticReadiness{state: readiness.Readiness{
			VendorType: readiness.VendorRestaurant, State: readiness.StateReady,
			PredictedReadyStart: &readyAt, PredictedReadyEnd: &readyAt,
			ConfidenceBand: readiness.ConfidenceHigh, UpdatedAt: h.now,
		}},
		JITEnabled: true, AutoSelectionEnabled: true,
		Now: func() time.Time { return h.now },
	})
	return h
}

// openWithPickups opens an orchestration carrying several vendor orders.
func (h *orchestrationHarness) openWithPickups(t *testing.T, pickups []Pickup) Orchestration {
	t.Helper()
	suffix := uuid.NewString()
	for i := range pickups {
		pickups[i].Sequence = i + 1
		pickups[i].VendorOrderID = pickups[i].VendorOrderID + "_" + suffix
	}
	opened, err := h.service.Open(context.Background(), OpenRequest{
		FulfilmentGroupRef: "fg_" + suffix,
		ParentOrderID:      "po_" + suffix,
		Requirement: Requirement{
			PickupCount: int64(len(pickups)), DropoffCount: 1, WeightGrams: 2000,
			PickupLatitude: 6.43, PickupLongitude: 3.42,
			DropoffLatitude: 6.45, DropoffLongitude: 3.40,
			DesiredPickupStart: h.now, DesiredPickupEnd: h.now.Add(10 * time.Minute),
		},
		Pickups: pickups,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return opened
}

// nearbyPickups are two vendors a few hundred metres apart, ready at the same
// time, carrying compatible ambient goods. Nothing about them forces a split,
// so whether they share a trip is decided purely by the declared limit.
func nearbyPickups() []Pickup {
	return []Pickup{
		{VendorOrderID: "vo_a", BusinessID: "biz-1", BranchID: "br-1",
			PickupLatitude: 6.4300, PickupLongitude: 3.4200,
			WeightGrams: 1000, VolumeMl: 500, PackageCount: 1, TemperatureBand: "AMBIENT"},
		{VendorOrderID: "vo_b", BusinessID: "biz-2", BranchID: "br-2",
			PickupLatitude: 6.4310, PickupLongitude: 3.4210,
			WeightGrams: 1000, VolumeMl: 500, PackageCount: 1, TemperatureBand: "AMBIENT"},
	}
}

func (h *orchestrationHarness) pickupRows(t *testing.T, id string) []Pickup {
	t.Helper()
	pickups, err := NewRepository(h.pool).Pickups(context.Background(), id)
	if err != nil {
		t.Fatalf("read pickups: %v", err)
	}
	return pickups
}

func (h *orchestrationHarness) reasonsByEventType(t *testing.T, id string) map[string][]string {
	t.Helper()
	events, err := NewRepository(h.pool).Decisions(context.Background(), id, 0)
	if err != nil {
		t.Fatalf("read decisions: %v", err)
	}
	byType := map[string][]string{}
	for _, event := range events {
		byType[event.EventType] = append(byType[event.EventType], event.ReasonCode)
	}
	return byType
}

// TestGroupingRunsOnTheEvaluationPathAndIsPersisted is the test that would have
// caught the planner being written and never called: it asserts the decision
// exists in the database after an ordinary evaluation, with no direct call to
// the planner anywhere in the test.
func TestGroupingRunsOnTheEvaluationPathAndIsPersisted(t *testing.T) {
	h := groupingHarness(t, logistics.Capability{MaxPickups: 4})
	opened := h.openWithPickups(t, nearbyPickups())

	if _, err := h.service.Evaluate(context.Background(), opened.ID); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	pickups := h.pickupRows(t, opened.ID)
	if len(pickups) != 2 {
		t.Fatalf("pickups = %d, want 2", len(pickups))
	}
	for _, pickup := range pickups {
		if pickup.GroupIndex == nil {
			t.Fatalf("%s was never assigned a group: the planner did not run",
				pickup.VendorOrderID)
		}
	}
	if *pickups[0].GroupIndex != *pickups[1].GroupIndex {
		t.Fatalf("two compatible nearby vendors were split into groups %d and %d",
			*pickups[0].GroupIndex, *pickups[1].GroupIndex)
	}
	if len(h.reasonsByEventType(t, opened.ID)[EventGroupPlanned]) == 0 {
		t.Fatal("no GROUP_PLANNED evidence was recorded")
	}
}

// TestDeclaredPickupLimitSplitsTheGroup proves the policy comes from the
// authority rather than from a constant. Same vendors, same distance, same
// goods -- only the declared limit changes, and the plan changes with it.
func TestDeclaredPickupLimitSplitsTheGroup(t *testing.T) {
	h := groupingHarness(t, logistics.Capability{MaxPickups: 1})
	opened := h.openWithPickups(t, nearbyPickups())

	if _, err := h.service.Evaluate(context.Background(), opened.ID); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	pickups := h.pickupRows(t, opened.ID)
	if pickups[0].GroupIndex == nil || pickups[1].GroupIndex == nil {
		t.Fatal("grouping did not run")
	}
	if *pickups[0].GroupIndex == *pickups[1].GroupIndex {
		t.Fatalf("an authority declaring one pickup was given a two-pickup group %d",
			*pickups[0].GroupIndex)
	}

	// The split reason is the point. An operator asking why two riders were
	// sent must get the limit, not a shrug.
	split := ""
	for _, pickup := range pickups {
		if pickup.SplitReason != "" {
			split = pickup.SplitReason
		}
	}
	if split != string(SplitPickupLimit) {
		t.Fatalf("split reason = %q, want %s", split, SplitPickupLimit)
	}
	if len(h.reasonsByEventType(t, opened.ID)[EventGroupSplit]) == 0 {
		t.Fatal("a split was persisted on the pickup but left no GROUP_SPLIT evidence")
	}
}

// TestUndeclaredCapabilityGroupsConservatively pins the rule that an authority
// which did not answer is not treated as unlimited. A capability lookup that
// says nothing must cost efficiency, never correctness.
func TestUndeclaredCapabilityGroupsConservatively(t *testing.T) {
	h := groupingHarness(t, logistics.Capability{})
	opened := h.openWithPickups(t, nearbyPickups())

	if _, err := h.service.Evaluate(context.Background(), opened.ID); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	pickups := h.pickupRows(t, opened.ID)
	if pickups[0].GroupIndex == nil || pickups[1].GroupIndex == nil {
		t.Fatal("grouping did not run")
	}
	if *pickups[0].GroupIndex == *pickups[1].GroupIndex {
		t.Fatal("an authority that declared no pickup limit was treated as multi-pickup")
	}
}

// TestPickupFactsAreSnapshottedNotRecomputed proves the per-pickup facts
// grouping reads are stored with the pickup. If they were re-read from the
// vendor order at planning time, last week's split would be re-derived from
// today's numbers and could no longer be explained.
func TestPickupFactsAreSnapshottedNotRecomputed(t *testing.T) {
	h := groupingHarness(t, logistics.Capability{MaxPickups: 4})
	opened := h.openWithPickups(t, []Pickup{{
		VendorOrderID: "vo_snapshot", BusinessID: "biz-1", BranchID: "br-1",
		PickupLatitude: 6.4321, PickupLongitude: 3.4123,
		WeightGrams: 7777, VolumeMl: 8888, PackageCount: 9,
		Handling: []string{"FRAGILE"}, TemperatureBand: "CHILLED", SecurePickup: true,
	}})

	stored := h.pickupRows(t, opened.ID)
	if len(stored) != 1 {
		t.Fatalf("pickups = %d, want 1", len(stored))
	}
	got := stored[0]
	if got.WeightGrams != 7777 || got.VolumeMl != 8888 || got.PackageCount != 9 {
		t.Fatalf("load not snapshotted: %+v", got)
	}
	if got.TemperatureBand != "CHILLED" || !got.SecurePickup {
		t.Fatalf("handling constraints not snapshotted: %+v", got)
	}
	if len(got.Handling) != 1 || got.Handling[0] != "FRAGILE" {
		t.Fatalf("handling codes not snapshotted: %+v", got.Handling)
	}
	if got.PickupLatitude == 0 || got.PickupLongitude == 0 {
		t.Fatalf("pickup location not snapshotted: %+v", got)
	}
	// Not yet planned, and that must be distinguishable from group zero.
	if got.GroupIndex != nil {
		t.Fatalf("group assigned before any evaluation: %d", *got.GroupIndex)
	}
}

// TestGroupingCarriesNoPricing is the standing guard for this section: the
// planner decides who travels together, never what anyone is paid. A fare,
// rate or amount arriving on a pickup would make the grouping decision a
// commercial one.
func TestGroupingCarriesNoPricing(t *testing.T) {
	h := groupingHarness(t, logistics.Capability{MaxPickups: 4})
	opened := h.openWithPickups(t, nearbyPickups())
	if _, err := h.service.Evaluate(context.Background(), opened.ID); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	rows, err := h.pool.Query(context.Background(), `
		SELECT column_name FROM information_schema.columns
		 WHERE table_name='vendor_orchestration_pickups'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	banned := []string{"price", "fare", "amount", "kobo", "cost", "rate",
		"fee", "commission", "payout", "surge", "tariff"}
	for rows.Next() {
		var column string
		if err = rows.Scan(&column); err != nil {
			t.Fatal(err)
		}
		for _, word := range banned {
			if strings.Contains(column, word) {
				t.Fatalf("pickup column %q looks like pricing; grouping must not carry money", column)
			}
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
}
