package main

// Composition-root tests for the orders -> fulfilment aggregate translation.
//
// These exercise the pure translation and vocabulary mapping, which is where a
// boundary mistake would actually be made: a vendor-owned allocation quietly
// consolidated onto a platform mission, a logistics event mapped onto the wrong
// custody link, or a delivery mode that claims one executor for a mission that
// really has another. Persistence is covered by the aggregate's own tests and
// the database constraints.

import (
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

func vendorOrderFor(id, business, branch string, responsibility orders.DeliveryResponsibility) orders.VendorOrder {
	return orders.VendorOrder{
		ID:                     id,
		BusinessID:             business,
		BranchID:               branch,
		Domain:                 "FOOD",
		Currency:               "NGN",
		DeliveryResponsibility: responsibility,
		Status:                 orders.VendorStatusPaid,
	}
}

func projection() *fulfilmentOperationsProjection {
	return &fulfilmentOperationsProjection{}
}

func unitsFor(vendorOrders ...orders.VendorOrder) map[string]fulfilmentgroup.Unit {
	units := map[string]fulfilmentgroup.Unit{}
	for _, vendorOrder := range vendorOrders {
		units[vendorOrder.ID] = fulfilmentgroup.Unit{
			ID:         "unit-" + vendorOrder.ID,
			BusinessID: vendorOrder.BusinessID,
			BranchID:   vendorOrder.BranchID,
		}
	}
	return units
}

// One consolidated platform mission collecting two vendors and delivering to
// one receiver: several pickup stops, exactly one dropoff, deterministic order.
func TestPlanMissions_ConsolidatedPlatformMission(t *testing.T) {
	vendorA := vendorOrderFor("vo-a", "biz-a", "br-a", orders.DeliveryPlatform)
	vendorB := vendorOrderFor("vo-b", "biz-b", "br-b", orders.DeliveryPlatform)

	order := orders.ParentOrder{
		ID: "po-1", CustomerID: "cust-1", Currency: "NGN",
		VendorOrders: []orders.VendorOrder{vendorA, vendorB},
	}
	plans := []orders.MissionPlan{{
		PlanMissionRef:         "plan-mission-1",
		Service:                orders.LogisticsDispatch,
		VendorOrderIDs:         []string{"vo-a", "vo-b"},
		AgreedFeeMinor:         150_000,
		LoadingDurationSeconds: 30 * 60,
	}}

	missions := projection().planMissions(order, plans, unitsFor(vendorA, vendorB))

	if len(missions) != 1 {
		t.Fatalf("missions = %d; want 1 consolidated mission", len(missions))
	}
	mission := missions[0]
	if err := mission.Validate(); err != nil {
		t.Fatalf("translated mission is invalid: %v", err)
	}
	if got := len(mission.PickupStops()); got != 2 {
		t.Fatalf("pickup stops = %d; want 2", got)
	}
	if _, found := mission.Dropoff(); !found {
		t.Fatal("a mission must have a receiver destination")
	}
	if mission.Owner != fulfilmentgroup.OwnerPlatform ||
		mission.Authority != fulfilmentgroup.AuthorityDispatch {
		t.Fatalf("owner/authority = %s/%s; want PLATFORM/DISPATCH",
			mission.Owner, mission.Authority)
	}
	// The slot reference is the Section 17C plan mission, which is what an
	// inbound logistics event names. Correlation must stay an equality check.
	if mission.SlotRef != "plan-mission-1" {
		t.Fatalf("slot ref = %q; want the plan mission reference", mission.SlotRef)
	}
	if mission.LoadingDurationSeconds != 30*60 {
		t.Fatalf("loading duration = %d; want %d", mission.LoadingDurationSeconds, 30*60)
	}
}

// Vendor-owned delivery is the vendor's own vehicle. Two vendors delivering
// themselves are two missions, never one.
func TestPlanMissions_VendorOwnedNeverConsolidated(t *testing.T) {
	vendorA := vendorOrderFor("vo-a", "biz-a", "br-a", orders.DeliveryVendorOwn)
	vendorB := vendorOrderFor("vo-b", "biz-b", "br-b", orders.DeliveryVendorOwn)

	order := orders.ParentOrder{
		ID: "po-1", CustomerID: "cust-1", Currency: "NGN",
		VendorOrders: []orders.VendorOrder{vendorA, vendorB},
	}

	missions := projection().planMissions(order, nil, unitsFor(vendorA, vendorB))

	if len(missions) != 2 {
		t.Fatalf("missions = %d; want one per vendor-owned delivery", len(missions))
	}
	for _, mission := range missions {
		if mission.Owner != fulfilmentgroup.OwnerVendor ||
			mission.Authority != fulfilmentgroup.AuthorityVendor {
			t.Fatalf("vendor-owned mission has owner/authority %s/%s",
				mission.Owner, mission.Authority)
		}
		if err := mission.Validate(); err != nil {
			t.Fatalf("vendor mission invalid: %v", err)
		}
		if len(mission.PickupStops()) != 1 {
			t.Fatal("a vendor-owned mission collects from its own branch only")
		}
	}
}

// One group, several missions, mixed executors — the capacity-overflow and
// vehicle-overflow shape. Every mission still has exactly one owner.
func TestPlanMissions_MultiMissionMixedOwnership(t *testing.T) {
	platform := vendorOrderFor("vo-a", "biz-a", "br-a", orders.DeliveryPlatform)
	hauling := vendorOrderFor("vo-b", "biz-b", "br-b", orders.DeliveryPlatform)
	own := vendorOrderFor("vo-c", "biz-c", "br-c", orders.DeliveryVendorOwn)

	order := orders.ParentOrder{
		ID: "po-1", CustomerID: "cust-1", Currency: "NGN",
		VendorOrders: []orders.VendorOrder{platform, hauling, own},
	}
	plans := []orders.MissionPlan{
		{PlanMissionRef: "pm-1", Service: orders.LogisticsDispatch, VendorOrderIDs: []string{"vo-a"}},
		{PlanMissionRef: "pm-2", Service: orders.LogisticsHauling, VendorOrderIDs: []string{"vo-b"}},
	}

	missions := projection().planMissions(order, plans, unitsFor(platform, hauling, own))

	if len(missions) != 3 {
		t.Fatalf("missions = %d; want 3 in one group", len(missions))
	}

	byAuthority := map[fulfilmentgroup.Authority]int{}
	sequences := map[int]bool{}
	for _, mission := range missions {
		byAuthority[mission.Authority]++
		if sequences[mission.Sequence] {
			t.Fatalf("duplicate mission sequence %d", mission.Sequence)
		}
		sequences[mission.Sequence] = true
		if !mission.Owner.Valid() {
			t.Fatal("every mission must have exactly one valid executor")
		}
	}
	if byAuthority[fulfilmentgroup.AuthorityDispatch] != 1 ||
		byAuthority[fulfilmentgroup.AuthorityHauling] != 1 ||
		byAuthority[fulfilmentgroup.AuthorityVendor] != 1 {
		t.Fatalf("authority mix = %v; want one each", byAuthority)
	}
}

// A terminal allocation is not fulfilled. Planning one would dispatch a vehicle
// for goods that were cancelled or refunded.
func TestPlanMissions_SkipsTerminalAllocations(t *testing.T) {
	cancelled := vendorOrderFor("vo-a", "biz-a", "br-a", orders.DeliveryVendorOwn)
	cancelled.Status = orders.VendorStatusCancelled

	order := orders.ParentOrder{
		ID: "po-1", CustomerID: "cust-1", Currency: "NGN",
		VendorOrders: []orders.VendorOrder{cancelled},
	}

	if missions := projection().planMissions(order, nil, unitsFor(cancelled)); len(missions) != 0 {
		t.Fatalf("missions = %d; a terminal allocation must not be planned", len(missions))
	}
}

// HYBRID describes the plan, not a mission. It appears only when both executors
// are genuinely in use for the order.
func TestDeliveryModeFor(t *testing.T) {
	platform := vendorOrderFor("vo-a", "biz-a", "br-a", orders.DeliveryPlatform)
	own := vendorOrderFor("vo-b", "biz-b", "br-b", orders.DeliveryVendorOwn)

	cases := []struct {
		name  string
		order orders.ParentOrder
		want  fulfilmentgroup.DeliveryMode
	}{
		{"platform only", orders.ParentOrder{VendorOrders: []orders.VendorOrder{platform}},
			fulfilmentgroup.ModePlatform},
		{"vendor only", orders.ParentOrder{VendorOrders: []orders.VendorOrder{own}},
			fulfilmentgroup.ModeVendor},
		{"both", orders.ParentOrder{VendorOrders: []orders.VendorOrder{platform, own}},
			fulfilmentgroup.ModeHybrid},
	}
	for _, testCase := range cases {
		if got := deliveryModeFor(testCase.order); got != testCase.want {
			t.Errorf("%s: mode = %s; want %s", testCase.name, got, testCase.want)
		}
	}
}

func TestAuthorityFor_MapsLogisticsService(t *testing.T) {
	if authorityFor(orders.LogisticsHauling) != fulfilmentgroup.AuthorityHauling {
		t.Error("hauling must map to the hauling authority")
	}
	if authorityFor(orders.LogisticsDispatch) != fulfilmentgroup.AuthorityDispatch {
		t.Error("dispatch must map to the dispatch authority")
	}
}

// Only events that actually move the goods produce a custody link. A timing or
// ETA update is real information but it is not a change of hands.
func TestCustodyEventFor_OnlyMovementCounts(t *testing.T) {
	moving := map[string]fulfilmentgroup.CustodyEventType{
		"PICKED_UP":       fulfilmentgroup.CustodyHandedOver,
		"DELIVERED":       fulfilmentgroup.CustodyDelivered,
		"DELIVERY_FAILED": fulfilmentgroup.CustodyFailed,
	}
	for eventType, want := range moving {
		got, moves := custodyEventFor(eventType)
		if !moves || got != want {
			t.Errorf("%s -> %s (moves=%v); want %s", eventType, got, moves, want)
		}
	}

	for _, eventType := range []string{
		"DELIVERING", "TIMING_UPDATED", "ETA_UPDATED", "PICKUP_TIMING_UPDATED",
	} {
		if _, moves := custodyEventFor(eventType); moves {
			t.Errorf("%s is not a change of hands and must not write a custody link", eventType)
		}
	}
}

func TestCustodianFor_DeliveredHandsToCustomer(t *testing.T) {
	if custodianFor(fulfilmentgroup.CustodyDelivered) != fulfilmentgroup.CustodianCustomer {
		t.Error("a delivered parcel is with the customer")
	}
	if custodianFor(fulfilmentgroup.CustodyHandedOver) != fulfilmentgroup.CustodianPlatform {
		t.Error("a handed-over parcel is with the platform provider")
	}
}

// An authority that declined without a reason is recorded as unspecified, never
// as success: a refusal with no code cannot be acted on, only retried blindly.
func TestRefusalCodeFor_NeverSilentlySucceeds(t *testing.T) {
	if code := refusalCodeFor(orders.MissionResult{Status: orders.MissionAccepted}); code != "" {
		t.Errorf("an accepted mission has no refusal code, got %q", code)
	}
	if code := refusalCodeFor(orders.MissionResult{Status: orders.MissionRefused}); code == "" {
		t.Error("a refusal with no code must still record one")
	}
	if code := refusalCodeFor(orders.MissionResult{
		Status: orders.MissionRefused, RefusalCode: "NO_RIDER",
	}); code != "NO_RIDER" {
		t.Errorf("refusal code = %q; want the authority's own", code)
	}
}

// The planned cost is evidence, in integer minor units. It creates no financial
// obligation; payment-wallet-service remains the authority for what was charged.
func TestPlannedCostFor_SumsAgreedFees(t *testing.T) {
	total := plannedCostFor([]orders.MissionPlan{
		{AgreedFeeMinor: 150_000}, {AgreedFeeMinor: 90_000},
	})
	if total != 240_000 {
		t.Fatalf("planned cost = %d; want 240000", total)
	}
}

func TestPlanReasonFor_FirstVersionIsInitial(t *testing.T) {
	if planReasonFor(0) != fulfilmentgroup.ReasonInitialPlan {
		t.Error("the first version is the initial plan")
	}
	if reason := planReasonFor(3); !reason.Replan() {
		t.Errorf("a later version must name a replan reason, got %s", reason)
	}
}

// The projection is optional. A deployment without the aggregate activates
// orders exactly as before rather than failing.
func TestProjection_NilWhenDisabled(t *testing.T) {
	if newFulfilmentOperationsProjection(nil) != nil {
		t.Fatal("a nil service must not produce a projection")
	}

	disabled := fulfilmentgroup.NewService(fulfilmentgroup.Options{Enabled: false})
	if newFulfilmentOperationsProjection(disabled) != nil {
		t.Fatal("a disabled aggregate must not produce a projection")
	}
}

// Every method tolerates being called on a nil projection, because activation
// and recovery both run through this path more than once.
func TestProjection_NilIsSafe(t *testing.T) {
	var nilProjection *fulfilmentOperationsProjection

	if err := nilProjection.OpenFulfilment(nil, orders.ParentOrder{}, nil); err != nil {
		t.Fatalf("OpenFulfilment on nil: %v", err)
	}
	if err := nilProjection.LinkMission(nil, orders.ParentOrder{},
		orders.MissionPlan{}, orders.MissionResult{}); err != nil {
		t.Fatalf("LinkMission on nil: %v", err)
	}
	if err := nilProjection.ProjectLogisticsEvent(nil, orders.LogisticsOrderEvent{}); err != nil {
		t.Fatalf("ProjectLogisticsEvent on nil: %v", err)
	}
}

// A checkout-time cluster stays one mission. It was quoted once, its pickups
// are co-located by the rule that produced it, and re-deciding that here would
// split a delivery that can genuinely be collected in one stop.
func TestPlanMissions_ConsolidatedClusterIsPreserved(t *testing.T) {
	first := vendorOrderFor("vo-a", "biz-a", "br-a", orders.DeliveryPlatform)
	second := vendorOrderFor("vo-b", "biz-b", "br-b", orders.DeliveryPlatform)

	order := orders.ParentOrder{
		ID: "po-1", CustomerID: "cust-1", Currency: "NGN",
		VendorOrders: []orders.VendorOrder{first, second},
	}
	plans := []orders.MissionPlan{{
		PlanMissionRef: "plan-mission-1",
		Service:        orders.LogisticsDispatch,
		VendorOrderIDs: []string{"vo-a", "vo-b"},
		AgreedFeeMinor: 150_000,
	}}

	missions := projection().planMissions(order, plans, unitsFor(first, second))

	if len(missions) != 1 {
		t.Fatalf("missions = %d; a cluster quoted once stays one mission", len(missions))
	}
	// One fee was agreed. The plan must record it once, not once per pickup.
	if missions[0].PlannedCostMinor != 150_000 {
		t.Fatalf("planned cost = %d; the customer agreed 150000 once",
			missions[0].PlannedCostMinor)
	}
	// The new traceability: each allocation is recorded as its own pickup stop.
	if got := len(missions[0].PickupStops()); got != 2 {
		t.Fatalf("pickup stops = %d; each allocation must be traceable", got)
	}
}

// The declared capabilities must match each authority's current contract.
// Dispatch V1 is route-bound and multi-pickup; Hauling remains on the legacy
// single-pickup commerce contract.
func TestAuthorityCapabilities_MatchTheExecutingContract(t *testing.T) {
	capabilities := authorityCapabilities()
	dispatch := capabilities[fulfilmentgroup.AuthorityDispatch]
	if !dispatch.MultiStopPickup || dispatch.MaxPickupStops < 2 {
		t.Fatalf("Dispatch V1 multi-pickup capability not declared: %+v", dispatch)
	}
	hauling := capabilities[fulfilmentgroup.AuthorityHauling]
	if hauling.MultiStopPickup || hauling.MaxPickupStops != 1 {
		t.Fatalf("legacy Hauling capability overstated: %+v", hauling)
	}

	taxi, found := capabilities[fulfilmentgroup.AuthorityTaxi]
	if !found || taxi.MultiStopPickup || taxi.MaxPickupStops != 1 {
		t.Fatalf("Taxi Goods single-pickup capability not declared: %+v", taxi)
	}
}
