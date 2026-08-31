package fulfilmentgroup

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type vendorDeliveryRuntimeFixture struct {
	store         *Store
	businessID    string
	branchID      string
	ownerStaffID  string
	riderStaffIDs []string
	mission       VendorMission
	parcel        Parcel
	plan          Plan
	planMission   PlanMission
}

func seedVendorDeliveryRuntime(t *testing.T, riderCount int) vendorDeliveryRuntimeFixture {
	t.Helper()
	pool := integrationPool(t)
	store := NewStore(pool, nil)
	ctx := context.Background()
	now := time.Now().UTC()
	suffix := uuid.NewString()
	businessID := "vendor_runtime_" + suffix
	branchID := "branch_runtime_" + suffix
	ownerStaffID := "staff_owner_" + suffix
	ownerUserID := "user_owner_" + suffix

	if _, err := pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES($1,$2)`, ownerUserID, ownerUserID+"@example.test"); err != nil {
		t.Fatalf("seed owner user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES($1,$2,$3,'active')`, ownerStaffID, businessID, ownerUserID); err != nil {
		t.Fatalf("seed owner staff: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone) VALUES($1,'Runtime Test Vendor','ACTIVE',$2,'NGN','Africa/Lagos')`, businessID, ownerStaffID); err != nil {
		t.Fatalf("seed business: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status) VALUES($1,$2,'Runtime Branch','12 Runtime Road','Lagos','Lagos','NG',6.5244,3.3792,'Africa/Lagos','ACTIVE')`, branchID, businessID); err != nil {
		t.Fatalf("seed branch: %v", err)
	}

	riders := make([]string, 0, riderCount)
	for index := 0; index < riderCount; index++ {
		riderID := "staff_rider_" + uuid.NewString()
		userID := "user_rider_" + uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES($1,$2)`, userID, userID+"@example.test"); err != nil {
			t.Fatalf("seed Rider user: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES($1,$2,$3,'active')`, riderID, businessID, userID); err != nil {
			t.Fatalf("seed Rider staff: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id,branch_id,assigned_by) VALUES($1,$2,'role_vendor_rider',$3,$4)`, "assignment_"+uuid.NewString(), riderID, branchID, ownerStaffID); err != nil {
			t.Fatalf("seed Rider role: %v", err)
		}
		if _, err := store.SetRiderAvailability(ctx, businessID, riderID, branchID, RiderAvailable, 0, "availability_"+uuid.NewString(), now); err != nil {
			t.Fatalf("seed Rider availability: %v", err)
		}
		riders = append(riders, riderID)
	}

	parentOrderID := "parent_runtime_" + suffix
	vendorOrderID := "vendor_order_runtime_" + suffix
	customerID := "customer_runtime_" + suffix
	if _, err := pool.Exec(ctx, `INSERT INTO commerce_parent_orders(id,customer_id,currency,state,created_at,updated_at) VALUES($1,$2,'NGN','CREATED',$3,$3)`, parentOrderID, customerID, now); err != nil {
		t.Fatalf("seed parent order: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO commerce_vendor_orders(id,parent_order_id,customer_id,business_id,branch_id,domain,currency,status,delivery_responsibility,delivery_fee_beneficiary,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'FOOD','NGN','PAID','VENDOR_OWN_DELIVERY','VENDOR',$6,$6)`, vendorOrderID, parentOrderID, customerID, businessID, branchID, now); err != nil {
		t.Fatalf("seed VendorOrder: %v", err)
	}
	group, err := store.EnsureGroup(ctx, parentOrderID, customerID, now)
	if err != nil {
		t.Fatalf("seed group: %v", err)
	}
	unit, err := store.EnsureUnit(ctx, group, UnitInput{VendorOrderID: vendorOrderID}, now)
	if err != nil {
		t.Fatalf("seed unit: %v", err)
	}
	parcel, err := store.CreateParcel(ctx, businessID, unit.ID, ParcelInput{IdempotencyKey: "parcel_" + suffix, WeightGrams: 1_000}, now)
	if err != nil {
		t.Fatalf("seed parcel: %v", err)
	}
	plan, err := store.ActivatePlan(ctx, group.ID, PlanInput{
		ExpectedVersion: 0, Reason: ReasonInitialPlan, DeliveryMode: ModeVendor,
		PlannedCostMinor: 1_000, Currency: "NGN", IdempotencyKey: "plan_" + suffix,
		Missions: []PlanMission{{SlotRef: "vendor_slot_" + suffix, Sequence: 1, Owner: OwnerVendor, Authority: AuthorityVendor, PlannedCostMinor: 1_000, Currency: "NGN", ParcelIDs: []string{parcel.ID}, Stops: []Stop{{Sequence: 1, Type: StopPickup, UnitID: unit.ID}, {Sequence: 2, Type: StopDropoff, Address: "44 Receiver Road"}}}},
	}, now)
	if err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	mission, err := store.EnsureVendorMission(ctx, group.ID, plan.ID, plan.Missions[0].ID, now)
	if err != nil {
		t.Fatalf("seed VendorMission: %v", err)
	}
	return vendorDeliveryRuntimeFixture{store: store, businessID: businessID, branchID: branchID, ownerStaffID: ownerStaffID, riderStaffIDs: riders, mission: mission, parcel: parcel, plan: plan, planMission: plan.Missions[0]}
}

func assignFixtureRider(t *testing.T, fixture vendorDeliveryRuntimeFixture, riderIndex int) VendorMission {
	t.Helper()
	mission, err := fixture.store.AssignEligibleVendorRider(context.Background(), fixture.businessID, fixture.mission.ID, fixture.riderStaffIDs[riderIndex], fixture.ownerStaffID, "assign_"+uuid.NewString(), fixture.mission.Version, time.Now().UTC())
	if err != nil {
		t.Fatalf("assign Rider: %v", err)
	}
	return mission
}

func applyRuntimeAction(t *testing.T, fixture vendorDeliveryRuntimeFixture, mission VendorMission, action RiderAction) VendorMissionRuntime {
	t.Helper()
	request := RiderActionRequest{Action: action, OperationRef: "action_" + uuid.NewString(), ExpectedVersion: mission.Version}
	if action == ActionCompletePickup || action == ActionCompleteDelivery || action == ActionReturn {
		request.verifiedRef = "handover_" + request.OperationRef
		request.verifiedProof = "proof_" + request.OperationRef
	}
	value, err := fixture.store.ApplyRiderAction(context.Background(), fixture.businessID, mission.AssignedStaffID, mission.ID, request, time.Now().UTC())
	if err != nil {
		t.Fatalf("apply %s: %v", action, err)
	}
	return value
}

func advanceFixtureToPickupArrival(t *testing.T, fixture vendorDeliveryRuntimeFixture, mission VendorMission) VendorMissionRuntime {
	t.Helper()
	for _, action := range []RiderAction{ActionAcknowledge, ActionStartPickup, ActionArrivePickup} {
		value := applyRuntimeAction(t, fixture, mission, action)
		mission = value.Mission
	}
	value, err := fixture.store.GetVendorMissionRuntime(context.Background(), fixture.businessID, mission.ID)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestIntegration_VendorRiderAssignmentRaceHasOneWinner(t *testing.T) {
	fixture := seedVendorDeliveryRuntime(t, 2)
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range errs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, errs[index] = fixture.store.AssignEligibleVendorRider(context.Background(), fixture.businessID, fixture.mission.ID, fixture.riderStaffIDs[index], fixture.ownerStaffID, "assign_race_"+uuid.NewString(), fixture.mission.Version, time.Now().UTC())
		}(index)
	}
	wait.Wait()
	winners := 0
	for _, err := range errs {
		if err == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("assignment winners=%d errors=%v", winners, errs)
	}
	var active int
	if err := fixture.store.db.QueryRow(context.Background(), `SELECT count(*) FROM commerce_vendor_mission_assignments WHERE vendor_mission_id=$1 AND state='ACTIVE'`, fixture.mission.ID).Scan(&active); err != nil || active != 1 {
		t.Fatalf("active assignments=%d err=%v", active, err)
	}
}

func TestIntegration_PickupAndFallbackSerializeAtCustodyBoundary(t *testing.T) {
	fixture := seedVendorDeliveryRuntime(t, 1)
	mission := assignFixtureRider(t, fixture, 0)
	runtime := advanceFixtureToPickupArrival(t, fixture, mission)
	pickup := RiderActionRequest{Action: ActionCompletePickup, OperationRef: "pickup_race_" + uuid.NewString(), ExpectedVersion: runtime.Mission.Version, verifiedRef: "pickup_handover", verifiedProof: "pickup_proof"}
	fallback := HybridFallbackRequest{OperationRef: "fallback_race_" + uuid.NewString(), ExpectedVersion: runtime.Mission.Version, ReasonCode: ReasonCapacityOverflow}

	errs := make([]error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		_, errs[0] = fixture.store.ApplyRiderAction(context.Background(), fixture.businessID, runtime.Mission.AssignedStaffID, runtime.Mission.ID, pickup, time.Now().UTC())
	}()
	go func() {
		defer wait.Done()
		_, errs[1] = fixture.store.EnsureHybridFallback(context.Background(), fixture.businessID, fixture.ownerStaffID, runtime.Mission.ID, fallback, time.Now().UTC())
	}()
	wait.Wait()
	winners := 0
	for _, err := range errs {
		if err == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("pickup/fallback winners=%d errors=%v", winners, errs)
	}
	var custody, fallbacks int
	_ = fixture.store.db.QueryRow(context.Background(), `SELECT count(*) FROM commerce_parcel_custody_events WHERE vendor_mission_id=$1 AND event_type='HANDED_OVER'`, runtime.Mission.ID).Scan(&custody)
	_ = fixture.store.db.QueryRow(context.Background(), `SELECT count(*) FROM commerce_vendor_hybrid_fallbacks WHERE vendor_mission_id=$1`, runtime.Mission.ID).Scan(&fallbacks)
	if (custody == 1) == (fallbacks == 1) {
		t.Fatalf("custody=%d fallbacks=%d; exactly one boundary outcome must exist", custody, fallbacks)
	}
}

func TestIntegration_ConcurrentSameKeyFallbackCreatesOneOperation(t *testing.T) {
	fixture := seedVendorDeliveryRuntime(t, 1)
	mission := assignFixtureRider(t, fixture, 0)
	request := HybridFallbackRequest{OperationRef: "fallback_once_" + uuid.NewString(), ExpectedVersion: mission.Version, ReasonCode: ReasonCapacityOverflow}
	results := make([]HybridFallback, 2)
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index], errs[index] = fixture.store.EnsureHybridFallback(context.Background(), fixture.businessID, fixture.ownerStaffID, mission.ID, request, time.Now().UTC())
		}(index)
	}
	wait.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("same-key fallback retry: %v", err)
		}
	}
	if results[0].ID == "" || results[0].ID != results[1].ID {
		t.Fatalf("fallback retry results differ: %q/%q", results[0].ID, results[1].ID)
	}
	var operations int
	if err := fixture.store.db.QueryRow(context.Background(), `SELECT count(*) FROM commerce_vendor_hybrid_fallbacks WHERE operation_ref=$1`, request.OperationRef).Scan(&operations); err != nil || operations != 1 {
		t.Fatalf("fallback operations=%d err=%v", operations, err)
	}
}

func TestIntegration_DuplicatePickupAndCompletionCreateOneCustodyPath(t *testing.T) {
	fixture := seedVendorDeliveryRuntime(t, 1)
	mission := assignFixtureRider(t, fixture, 0)
	runtime := advanceFixtureToPickupArrival(t, fixture, mission)
	pickup := RiderActionRequest{Action: ActionCompletePickup, OperationRef: "pickup_once_" + uuid.NewString(), ExpectedVersion: runtime.Mission.Version, verifiedRef: "pickup_handover", verifiedProof: "pickup_proof"}

	results := make([]VendorMissionRuntime, 2)
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index], errs[index] = fixture.store.ApplyRiderAction(context.Background(), fixture.businessID, runtime.Mission.AssignedStaffID, runtime.Mission.ID, pickup, time.Now().UTC())
		}(index)
	}
	wait.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("same-key pickup retry: %v", err)
		}
	}
	if results[0].Mission.Version != results[1].Mission.Version {
		t.Fatalf("pickup retry versions differ: %d/%d", results[0].Mission.Version, results[1].Mission.Version)
	}
	var pickupEvents int
	if err := fixture.store.db.QueryRow(context.Background(), `SELECT count(*) FROM commerce_parcel_custody_events WHERE vendor_mission_id=$1 AND event_type='HANDED_OVER'`, runtime.Mission.ID).Scan(&pickupEvents); err != nil || pickupEvents != 1 {
		t.Fatalf("pickup custody events=%d err=%v", pickupEvents, err)
	}

	mission = results[0].Mission
	for _, action := range []RiderAction{ActionStartReceiver, ActionArriveReceiver} {
		value := applyRuntimeAction(t, fixture, mission, action)
		mission = value.Mission
	}
	completion := RiderActionRequest{Action: ActionCompleteDelivery, OperationRef: "complete_once_" + uuid.NewString(), ExpectedVersion: mission.Version, verifiedRef: "receiver_handover", verifiedProof: "receiver_proof"}
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index], errs[index] = fixture.store.ApplyRiderAction(context.Background(), fixture.businessID, mission.AssignedStaffID, mission.ID, completion, time.Now().UTC())
		}(index)
	}
	wait.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("same-key completion retry: %v", err)
		}
	}
	var deliveredEvents int
	if err := fixture.store.db.QueryRow(context.Background(), `SELECT count(*) FROM commerce_parcel_custody_events WHERE vendor_mission_id=$1 AND event_type='DELIVERED'`, mission.ID).Scan(&deliveredEvents); err != nil || deliveredEvents != 1 {
		t.Fatalf("delivery custody events=%d err=%v", deliveredEvents, err)
	}
}
