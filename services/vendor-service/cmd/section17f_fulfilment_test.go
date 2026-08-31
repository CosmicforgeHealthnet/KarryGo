package main

// Section 17F fulfilment-activation integration tests.
//
// The stock and logistics authorities are scriptable fakes, because what is
// under test is the ORCHESTRATION: when stock is confirmed, when a mission is
// created, what is never created, and what happens to each when one allocation
// cancels while its siblings keep running.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

type inventoryCall struct {
	action     string
	businessID string
	branchID   string
	reference  string
}

type fakeInventory struct {
	mu    sync.Mutex
	calls []inventoryCall
	fail  bool
}

func (f *fakeInventory) record(action, businessID, branchID, reference string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("inventory unavailable")
	}
	f.calls = append(f.calls, inventoryCall{action, businessID, branchID, reference})
	return nil
}

func (f *fakeInventory) ConfirmReservation(_ context.Context, b, br, ref string) error {
	return f.record("confirm", b, br, ref)
}

func (f *fakeInventory) CommitReservation(_ context.Context, b, br, ref string) error {
	return f.record("commit", b, br, ref)
}

func (f *fakeInventory) ReleaseReservation(_ context.Context, b, br, ref string) error {
	return f.record("release", b, br, ref)
}

func (f *fakeInventory) actionsFor(reference string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var actions []string
	for _, call := range f.calls {
		if call.reference == reference {
			actions = append(actions, call.action)
		}
	}
	return actions
}

func (f *fakeInventory) count(action string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, call := range f.calls {
		if call.action == action {
			total++
		}
	}
	return total
}

type fakeMissions struct {
	mu       sync.Mutex
	requests []orders.MissionRequest
	refuse   bool
	fail     bool
	// reprice makes the authority answer with a different fee, which the
	// contract must refuse.
	reprice bool
	offline map[orders.LogisticsService]bool
}

func newFakeMissions() *fakeMissions {
	return &fakeMissions{offline: map[orders.LogisticsService]bool{}}
}

func (f *fakeMissions) Available(service orders.LogisticsService) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.offline[service]
}

func (f *fakeMissions) CreateMission(
	_ context.Context, request orders.MissionRequest,
) (orders.MissionResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return orders.MissionResult{}, errors.New("logistics authority unreachable")
	}
	f.requests = append(f.requests, request)
	if f.refuse {
		return orders.MissionResult{
			Status:      orders.MissionRefused,
			RefusalCode: "NO_RIDER_AVAILABLE",
		}, nil
	}
	fee := request.AgreedFeeMinor
	if f.reprice {
		fee += 1000
	}
	return orders.MissionResult{
		ExternalRef:      "trip_" + request.MissionRef,
		Status:           orders.MissionAccepted,
		AcceptedFeeMinor: fee,
	}, nil
}

func (f *fakeMissions) sent() []orders.MissionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]orders.MissionRequest(nil), f.requests...)
}

// paidFixture builds a multi-vendor order whose payment is actually held.
func paidFixture(
	t *testing.T, vendorOwnForA bool,
) (section17EFixture, orders.ParentOrder, *fakeInventory, *fakeMissions) {
	t.Helper()

	authority := newFakeAuthority()
	fixture := withPayment(buildSection17EFixture(t, true, vendorOwnForA), authority)

	stock, missions := &fakeInventory{}, newFakeMissions()
	fixture.service.SetFulfilmentAuthorities(stock, missions)

	order, _, err := fixture.service.StartPayment(
		context.Background(), fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}
	if order.PaymentState != string(orders.PaymentHeld) {
		t.Fatalf("payment state=%q, want HELD", order.PaymentState)
	}
	return fixture, order, stock, missions
}

// Nothing is activated for an order nobody has paid for.
func TestSection17FFulfilment_UnpaidOrderIsNeverActivated(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, true), authority)
	stock, missions := &fakeInventory{}, newFakeMissions()
	fixture.service.SetFulfilmentAuthorities(stock, missions)
	ctx := context.Background()

	order, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = fixture.service.ActivateFulfilment(ctx, order.ID); !errors.Is(
		err, orders.ErrPaymentConflict,
	) {
		t.Fatalf("err=%v, want ErrPaymentConflict", err)
	}
	if len(stock.calls) != 0 {
		t.Fatal("stock was confirmed for an unpaid order")
	}
	if len(missions.sent()) != 0 {
		t.Fatal("a mission was created for an unpaid order")
	}
}

func TestSection17FFulfilment_ConfirmsStockAndCreatesMissionsOncePaid(t *testing.T) {
	fixture, order, stock, missions := paidFixture(t, false)
	ctx := context.Background()

	outcome, err := fixture.service.ActivateFulfilment(ctx, order.ID)
	if err != nil {
		t.Fatalf("ActivateFulfilment: %v", err)
	}
	if outcome.InventoryConfirmed != 2 {
		t.Fatalf("stock confirmed=%d, want one per allocation", outcome.InventoryConfirmed)
	}
	if outcome.MissionsCreated == 0 {
		t.Fatal("no logistics mission was created for a platform-executed order")
	}

	// Every mission carries the fee the customer already paid.
	for _, request := range missions.sent() {
		if request.AgreedFeeMinor <= 0 {
			t.Errorf("mission sent with no agreed fee: %+v", request)
		}
		if request.Service == orders.LogisticsService("TAXI") {
			t.Error("a taxi mission was created")
		}
	}

	// Each allocation's own reservation was confirmed, and no other.
	reread, err := fixture.service.GetForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, vendorOrder := range reread.VendorOrders {
		if orders.InventoryState(vendorOrder.InventoryState) != orders.InventoryConfirmed {
			t.Errorf("%s inventory=%s, want CONFIRMED",
				vendorOrder.BusinessID, vendorOrder.InventoryState)
		}
		actions := stock.actionsFor(vendorOrder.ReservationRef)
		if len(actions) != 1 || actions[0] != "confirm" {
			t.Errorf("%s stock actions=%v, want exactly one confirm",
				vendorOrder.BusinessID, actions)
		}
	}
}

// Activation must be safe to run repeatedly: stock is never double-committed
// and no second vehicle is dispatched.
func TestSection17FFulfilment_ActivationIsIdempotent(t *testing.T) {
	fixture, order, stock, missions := paidFixture(t, false)
	ctx := context.Background()

	if _, err := fixture.service.ActivateFulfilment(ctx, order.ID); err != nil {
		t.Fatal(err)
	}
	confirmsAfterFirst := stock.count("confirm")
	missionsAfterFirst := len(missions.sent())

	if _, err := fixture.service.ActivateFulfilment(ctx, order.ID); err != nil {
		t.Fatalf("second activation: %v", err)
	}

	if stock.count("confirm") != confirmsAfterFirst {
		t.Fatalf("stock confirmed again: %d -> %d",
			confirmsAfterFirst, stock.count("confirm"))
	}
	if len(missions.sent()) != missionsAfterFirst {
		t.Fatalf("a second mission was created: %d -> %d",
			missionsAfterFirst, len(missions.sent()))
	}

	var rows int
	if err := fixture.env.pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_order_missions WHERE parent_order_id=$1`,
		order.ID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != missionsAfterFirst {
		t.Fatalf("mission rows=%d, want %d", rows, missionsAfterFirst)
	}
}

// Vendor-owned delivery gets NO platform mission: the vendor is the executor.
func TestSection17FFulfilment_VendorOwnDeliveryGetsNoPlatformMission(t *testing.T) {
	fixture, order, _, missions := paidFixture(t, true)
	ctx := context.Background()

	outcome, err := fixture.service.ActivateFulfilment(ctx, order.ID)
	if err != nil {
		t.Fatalf("ActivateFulfilment: %v", err)
	}
	if outcome.VendorOwnDelivery != 1 {
		t.Fatalf("vendor-own allocations=%d, want 1", outcome.VendorOwnDelivery)
	}

	vendorOwn := fixture.vendorOrderFor(t, order, fixture.env.businessA)
	for _, request := range missions.sent() {
		for _, id := range request.VendorOrderIDs {
			if id == vendorOwn.ID {
				t.Fatal("a platform mission was created for a vendor-owned delivery")
			}
		}
	}

	// Its own delivery workflow is what tracks that work.
	reread, err := fixture.service.GetForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, vendorOrder := range reread.VendorOrders {
		if vendorOrder.ID != vendorOwn.ID {
			continue
		}
		if vendorOrder.VendorOwnDeliveryState == orders.VendorOwnNotApplicable {
			t.Fatal("a vendor-owned allocation has no vendor delivery state")
		}
	}
}

// The price guarantee at the execution boundary: the authority is told the
// agreed fee, and an internal split cannot raise what the customer pays.
func TestSection17FFulfilment_MissionCarriesTheAgreedFeeOnly(t *testing.T) {
	fixture, order, _, missions := paidFixture(t, false)
	ctx := context.Background()

	if _, err := fixture.service.ActivateFulfilment(ctx, order.ID); err != nil {
		t.Fatal(err)
	}

	byPlanMission := map[string]int64{}
	for _, vendorOrder := range order.VendorOrders {
		if vendorOrder.DeliveryResponsibility != orders.DeliveryPlatform {
			continue
		}
		byPlanMission[vendorOrder.LogisticsMissionRef] = vendorOrder.DeliveryGrossMinor
	}

	var sentTotal int64
	for _, request := range missions.sent() {
		sentTotal += request.AgreedFeeMinor
	}
	var quotedTotal int64
	for _, fee := range byPlanMission {
		quotedTotal += fee
	}
	if sentTotal != quotedTotal {
		t.Fatalf("missions were sent %d in fees, the quote committed %d",
			sentTotal, quotedTotal)
	}

	// And the customer's approved total is untouched by activation.
	reread, err := fixture.service.GetForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.CustomerApprovedTotalMinor != order.CustomerApprovedTotalMinor {
		t.Fatalf("activation changed the customer total: %d -> %d",
			order.CustomerApprovedTotalMinor, reread.CustomerApprovedTotalMinor)
	}
}

// A refused mission is recorded as a refusal, not retried forever, and the
// stock it would have carried is not silently committed.
func TestSection17FFulfilment_RefusedMissionIsRecordedWithItsReason(t *testing.T) {
	fixture, order, _, missions := paidFixture(t, false)
	missions.refuse = true
	ctx := context.Background()

	outcome, err := fixture.service.ActivateFulfilment(ctx, order.ID)
	if err != nil {
		t.Fatalf("ActivateFulfilment: %v", err)
	}
	if outcome.MissionsRefused == 0 {
		t.Fatal("a refusal was not recorded")
	}

	var refused int
	if err = fixture.env.pool.QueryRow(ctx, `
		SELECT count(*) FROM commerce_order_missions
		 WHERE parent_order_id=$1 AND status='REFUSED' AND refusal_code <> ''`,
		order.ID).Scan(&refused); err != nil {
		t.Fatal(err)
	}
	if refused == 0 {
		t.Fatal("a refused mission stored no reason")
	}
}

// An unreachable authority leaves the mission PENDING for recovery rather than
// being recorded as a refusal it never made.
func TestSection17FFulfilment_UnreachableAuthorityLeavesTheMissionPending(t *testing.T) {
	fixture, order, _, missions := paidFixture(t, false)
	missions.fail = true
	ctx := context.Background()

	if _, err := fixture.service.ActivateFulfilment(ctx, order.ID); err != nil {
		t.Fatalf("ActivateFulfilment: %v", err)
	}

	var pending int
	if err := fixture.env.pool.QueryRow(ctx, `
		SELECT count(*) FROM commerce_order_missions
		 WHERE parent_order_id=$1 AND status='PENDING' AND failure_reason <> ''`,
		order.ID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending == 0 {
		t.Fatal("an unreachable authority did not leave a pending mission")
	}

	// Recovery resumes it once the authority is back.
	missions.fail = false
	outcome, err := fixture.service.RecoverPayments(ctx, orders.RecoveryLimits{
		Grace: time.Nanosecond, Deadline: time.Hour, Batch: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Activated == 0 {
		t.Fatal("recovery did not resume a stalled activation")
	}

	var accepted int
	if err = fixture.env.pool.QueryRow(ctx, `
		SELECT count(*) FROM commerce_order_missions
		 WHERE parent_order_id=$1 AND status='ACCEPTED'`, order.ID).Scan(&accepted); err != nil {
		t.Fatal(err)
	}
	if accepted == 0 {
		t.Fatal("recovery left every mission unexecuted")
	}
}

// Stock finalization timing, end to end: CONFIRMED on payment, COMMITTED on
// completion, and one allocation's cancellation releases ONLY its own stock.
func TestSection17FFulfilment_StockIsFinalizedPerAllocationOnly(t *testing.T) {
	fixture, order, stock, _ := paidFixture(t, true)
	ctx := context.Background()

	if _, err := fixture.service.ActivateFulfilment(ctx, order.ID); err != nil {
		t.Fatal(err)
	}

	// Activation confirmed stock, which bumped each allocation's version. A
	// caller holding the pre-activation version is correctly refused, so the
	// current state is re-read before transitioning.
	activated, err := fixture.service.GetForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.vendorOrderFor(t, activated, fixture.env.businessA)
	second := fixture.vendorOrderFor(t, activated, fixture.env.businessB)

	// Vendor B cancels. Only vendor B's reservation is released.
	if _, _, err := fixture.service.Advance(
		ctx, fixture.actor(fixture.staffB, fixture.env.businessB), second.ID,
		orders.TransitionCancel, "OUT_OF_STOCK", second.Version,
	); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	released := stock.actionsFor(second.ReservationRef)
	if len(released) < 2 || released[len(released)-1] != "release" {
		t.Fatalf("vendor B stock actions=%v, want a release", released)
	}
	siblingActions := stock.actionsFor(first.ReservationRef)
	for _, action := range siblingActions {
		if action == "release" {
			t.Fatal("cancelling one vendor released another vendor's stock")
		}
	}

	// Vendor A runs to completion. Only then is its stock committed.
	current := first
	actorA := fixture.actor(fixture.staffA, fixture.env.businessA)
	for _, step := range []orders.Transition{
		orders.TransitionAccept, orders.TransitionPreparing, orders.TransitionReady,
		orders.TransitionHandover, orders.TransitionDeliver, orders.TransitionComplete,
	} {
		current, _, err = fixture.service.Advance(ctx, actorA, current.ID, step, "", current.Version)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if step != orders.TransitionComplete {
			for _, action := range stock.actionsFor(first.ReservationRef) {
				if action == "commit" {
					t.Fatalf("stock was committed at %s, before completion", step)
				}
			}
		}
	}

	committed := stock.actionsFor(first.ReservationRef)
	if committed[len(committed)-1] != "commit" {
		t.Fatalf("vendor A stock actions=%v, want a commit on completion", committed)
	}

	// And it is committed exactly once.
	commits := 0
	for _, action := range committed {
		if action == "commit" {
			commits++
		}
	}
	if commits != 1 {
		t.Fatalf("stock was committed %d times", commits)
	}
}

// Section 17F never creates a Taxi mission.
func TestSection17FFulfilment_TaxiIsNeverAnExecutor(t *testing.T) {
	fixture, order, _, missions := paidFixture(t, false)
	ctx := context.Background()

	if _, err := fixture.service.ActivateFulfilment(ctx, order.ID); err != nil {
		t.Fatal(err)
	}
	for _, request := range missions.sent() {
		if request.Service != orders.LogisticsDispatch &&
			request.Service != orders.LogisticsHauling {
			t.Fatalf("mission routed to %q, want dispatch or hauling only", request.Service)
		}
	}

	var rows int
	if err := fixture.env.pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_order_missions
		  WHERE parent_order_id=$1 AND service NOT IN ('DISPATCH','HAULING')`,
		order.ID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("%d missions were routed outside dispatch/hauling", rows)
	}
}
