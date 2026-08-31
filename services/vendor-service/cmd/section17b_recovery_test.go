package main

import (
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
)

// Section 17B failure recovery.
//
// The reservation coordinator is an in-process loop, so a process that dies
// between "I am about to reserve" and "the reservation succeeded" is a real,
// ordinary event. What must never happen is that it costs a vendor stock
// nobody can buy, or leaves a customer with a cart they cannot edit.

// crashSession rewrites a validated session into the exact shape a crash
// leaves behind: still VALIDATING, coordination IN_PROGRESS, and a group whose
// reservation exists in Section 14 but whose identity the checkout row never
// got to record.
//
// Clearing inventory_reservation_ref is the important half. Recovery must then
// find the hold from the durable source reference alone - the reference that
// was deliberately committed before the Inventory call was made.
func (e *s17bEnv) crashSession(sessionID string, staleBy time.Duration) {
	e.t.Helper()

	ctx := e.ctx()

	if _, err := e.pool.Exec(
		ctx,
		`
        UPDATE commerce_checkout_groups
        SET reservation_status='PENDING',
            inventory_reservation_ref=''
        WHERE session_id=$1
          AND requires_stock_reservation
        `,
		sessionID,
	); err != nil {
		e.t.Fatal(err)
	}

	if _, err := e.pool.Exec(
		ctx,
		`
        UPDATE commerce_checkout_sessions
        SET status='VALIDATING',
            validated_at=NULL,
            reservation_coordination_state='IN_PROGRESS',
            updated_at=now() - $2::interval
        WHERE id=$1
        `,
		sessionID,
		staleBy.String(),
	); err != nil {
		e.t.Fatal(err)
	}
}

func (e *s17bEnv) sessionRow(sessionID string) (string, string) {
	e.t.Helper()

	var (
		status       string
		coordination string
	)

	if err := e.pool.QueryRow(
		e.ctx(),
		`
        SELECT status, reservation_coordination_state
        FROM commerce_checkout_sessions
        WHERE id=$1
        `,
		sessionID,
	).Scan(&status, &coordination); err != nil {
		e.t.Fatal(err)
	}

	return status, coordination
}

// TestSection17BRecoversAHoldACrashNeverAcknowledged is the failure-recovery
// gate. The reservation is real, the checkout row does not know its identity,
// and the sweep still has to release it.
func TestSection17BRecoversAHoldACrashNeverAcknowledged(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Crash Stock",
		Quantity:   9,
		PriceMinor: 12000,
	})

	customer := env.customer("crash")

	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 4})

	session, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	if session.Status != checkout.StatusValidated {
		t.Fatal(describeSession(session))
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 5 {
		t.Fatalf("available while held=%d want=5", available)
	}

	// The crash: coordination is in flight, the hold exists, and nothing
	// recorded which reservation it was.
	env.crashSession(session.ID, 2*time.Minute)

	status, coordination := env.sessionRow(session.ID)

	if status != string(checkout.StatusValidating) ||
		coordination != string(checkout.CoordinationInProgress) {
		t.Fatalf("crash shape not set up: status=%s coordination=%s",
			status, coordination)
	}

	settled, err := env.checkout.SweepDueSessions(env.ctx(), 50)

	if err != nil {
		t.Fatal(err)
	}

	if settled != 1 {
		t.Fatalf("sweep settled %d sessions, want 1", settled)
	}

	recovered, err := env.checkout.GetSession(env.ctx(), customer, session.ID)

	if err != nil {
		t.Fatal(err)
	}

	if recovered.Status != checkout.StatusFailed ||
		recovered.FailureCode != checkout.FailureCoordinationOrphaned {
		t.Fatal(describeSession(recovered))
	}

	if recovered.ReservationCoordinationState != checkout.CoordinationCompensated {
		t.Fatalf("coordination=%s want COMPENSATED",
			recovered.ReservationCoordinationState)
	}

	for _, group := range recovered.Groups {
		if group.ReservationStatus == checkout.ReservationReserved {
			t.Fatalf("group %s is still holding stock after recovery", group.ID)
		}

		// Recovery had to find the reservation from the source reference
		// alone, and it must write down what it found.
		if group.RequiresStockReservation &&
			group.InventoryReservationRef == "" {
			t.Fatalf("recovery released a hold without recording which one: %+v", group)
		}
	}

	// The leaked hold is gone.
	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 9 {
		t.Fatalf("available after recovery=%d want=9", available)
	}

	// And the customer has their cart back.
	if released := env.reloadCart(customer, current.ID); released.Status != cart.StatusActive {
		t.Fatalf("cart status=%s want ACTIVE", released.Status)
	}

	env.assertNoNegativeStock(item.InventoryItemID)
}

// TestSection17BRecoveryLeavesAHealthyCheckoutAlone is the other half of the
// recovery rule: a session that is simply still working must not be swept out
// from under itself.
func TestSection17BRecoveryLeavesAHealthyCheckoutAlone(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Healthy Stock",
		Quantity:   6,
		PriceMinor: 8000,
	})

	customer := env.customer("healthy")

	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 2})

	session, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	// A coordination that started one second ago is not abandoned.
	env.crashSession(session.ID, time.Second)

	if _, err = env.checkout.SweepDueSessions(env.ctx(), 50); err != nil {
		t.Fatal(err)
	}

	status, coordination := env.sessionRow(session.ID)

	if status != string(checkout.StatusValidating) ||
		coordination != string(checkout.CoordinationInProgress) {
		t.Fatalf("an in-flight coordination was swept: status=%s coordination=%s",
			status, coordination)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 4 {
		t.Fatalf("available=%d want=4: an in-flight hold was released", available)
	}
}

// TestSection17BReservationRetryIsIdempotent pins the property the whole
// recovery design rests on: the durable source reference is committed before
// the Inventory call, so replaying that call cannot produce a second hold.
func TestSection17BReservationRetryIsIdempotent(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Retry Stock",
		Quantity:   10,
		PriceMinor: 5000,
	})

	stock := newSection17BStockAuthority(env.stock)

	request := checkout.ReserveRequest{
		BusinessID:    env.businessA,
		BranchID:      env.branchA1,
		SourceRef:     "chkg_retry_" + env.suffix,
		ExpiresAt:     time.Now().UTC().Add(10 * time.Minute),
		CorrelationID: "s17b-retry",
		Lines: []checkout.ReserveLine{
			{InventoryItemID: item.InventoryItemID, Quantity: 3},
		},
	}

	first, err := stock.Reserve(env.ctx(), request)

	if err != nil {
		t.Fatal(err)
	}

	second, err := stock.Reserve(env.ctx(), request)

	if err != nil {
		t.Fatalf("replaying the same reservation failed: %v", err)
	}

	if second.ReservationRef != first.ReservationRef {
		t.Fatalf("replay produced a second reservation: %s vs %s",
			second.ReservationRef, first.ReservationRef)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 7 {
		t.Fatalf("available=%d want=7: the replay held the stock twice", available)
	}

	// The same reference resolves back to the same hold, which is how
	// recovery finds a reservation it never recorded.
	found, err := stock.FindBySource(env.ctx(), env.businessA, request.SourceRef)

	if err != nil {
		t.Fatal(err)
	}

	if found != first.ReservationRef {
		t.Fatalf("FindBySource=%s want=%s", found, first.ReservationRef)
	}

	// Releasing is idempotent too: compensation, cancellation, the TTL sweep
	// and recovery can all reach the same reservation.
	for attempt := 0; attempt < 2; attempt++ {
		if err = stock.Release(
			env.ctx(),
			env.businessA,
			env.branchA1,
			first.ReservationRef,
		); err != nil {
			t.Fatalf("release attempt %d: %v", attempt, err)
		}
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 10 {
		t.Fatalf("available after release=%d want=10", available)
	}

	// A released reservation is no longer offered to recovery.
	if found, err = stock.FindBySource(
		env.ctx(),
		env.businessA,
		request.SourceRef,
	); err != nil || found != "" {
		t.Fatalf("terminal reservation still offered for release: %q err=%v", found, err)
	}
}

// TestSection17BLapsedHoldSelfHealsThroughTheInventoryWorker states the last
// line of defence honestly: even if every Section 17B path failed, Section
// 14's own reservation-expiry sweep still returns the stock.
func TestSection17BLapsedHoldSelfHealsThroughTheInventoryWorker(t *testing.T) {
	env := newSection17BEnv(t, time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Lapsed Stock",
		Quantity:   7,
		PriceMinor: 9000,
	})

	customer := env.customer("lapsed")

	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 7})

	if _, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	); err != nil {
		t.Fatal(err)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 0 {
		t.Fatalf("available while held=%d want=0", available)
	}

	// The reservation's own expiry is real wall-clock state in Section 14, so
	// it is moved directly rather than through the checkout clock.
	if _, err := env.pool.Exec(
		env.ctx(),
		`
        UPDATE inventory_reservations
        SET expires_at = now() - interval '1 minute'
        WHERE business_id=$1
          AND source_type='CHECKOUT'
        `,
		env.businessA,
	); err != nil {
		t.Fatal(err)
	}

	// Section 17B is not involved in this pass at all.
	expired, err := env.stock.ExpireDueReservations(env.ctx(), 100)

	if err != nil {
		t.Fatal(err)
	}

	if expired == 0 {
		t.Fatal("the Inventory expiry worker released nothing")
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 7 {
		t.Fatalf("available after the Inventory sweep=%d want=7", available)
	}

	env.assertNoNegativeStock(item.InventoryItemID)
}

// TestSection17BReservationSourceTypeIsCheckout keeps the Section 14 contract
// visible: a checkout hold is recorded as a CHECKOUT reservation, not an
// ORDER, and is not confusable with one.
func TestSection17BReservationSourceTypeIsCheckout(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Source Stock",
		Quantity:   3,
		PriceMinor: 4000,
	})

	customer := env.customer("source")

	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 1})

	session, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	group := session.Groups[0]

	var (
		sourceType string
		sourceRef  string
		status     string
	)

	if err = env.pool.QueryRow(
		env.ctx(),
		`
        SELECT source_type, source_ref, status
        FROM inventory_reservations
        WHERE id=$1
        `,
		group.InventoryReservationRef,
	).Scan(&sourceType, &sourceRef, &status); err != nil {
		t.Fatal(err)
	}

	if sourceType != inventory.ReservationSourceCheckout {
		t.Fatalf("source type=%s want=%s",
			sourceType, inventory.ReservationSourceCheckout)
	}

	// The source reference is the durable checkout group identity, which is
	// what makes the hold findable after a crash.
	if sourceRef != group.ReservationSourceRef || sourceRef != group.ID {
		t.Fatalf("source ref=%s group=%s source_ref_column=%s",
			sourceRef, group.ID, group.ReservationSourceRef)
	}

	if status != inventory.ReservationHeld {
		t.Fatalf("status=%s want HELD", status)
	}
}
