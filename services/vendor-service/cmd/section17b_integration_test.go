package main

import (
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
)

// TestSection17BValidatesAndHoldsRealStock is the happy path, end to end
// through every real authority: the cart is revalidated, the candidate is
// grouped, and the Section 14 available balance actually drops.
func TestSection17BValidatesAndHoldsRealStock(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Rice",
		Quantity:   10,
		PriceMinor: 250000,
	})

	customer := env.customer("happy")

	current := env.seedShopCart(
		customer,
		s17bCartLine{Item: item, Quantity: 3},
	)

	before := env.availableStock(env.businessA, env.branchA1, item.InventoryItemID)

	if before != 10 {
		t.Fatalf("seeded available=%d want=10", before)
	}

	session, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatalf("StartCheckout: %v", err)
	}

	if session.Status != checkout.StatusValidated {
		t.Fatal(describeSession(session))
	}

	if session.ReservationCoordinationState != checkout.CoordinationComplete {
		t.Fatalf("coordination=%s", session.ReservationCoordinationState)
	}

	if len(session.Groups) != 1 {
		t.Fatalf("groups=%d want=1", len(session.Groups))
	}

	group := session.Groups[0]

	if group.ReservationStatus != checkout.ReservationReserved ||
		group.InventoryReservationRef == "" {
		t.Fatalf("group=%+v", group)
	}

	if group.QuotedTotalMinor != 750000 {
		t.Fatalf("quoted=%d want=750000", group.QuotedTotalMinor)
	}

	// A fixed line carries no authorized headroom.
	if group.MaxAuthorizedTotalMinor != group.QuotedTotalMinor {
		t.Fatalf("maximum=%d quoted=%d",
			group.MaxAuthorizedTotalMinor, group.QuotedTotalMinor)
	}

	if session.QuotedItemsTotalMinor != 750000 ||
		session.MaxAuthorizedItemsTotalMinor != 750000 ||
		session.Currency != "NGN" {
		t.Fatalf("session totals=%+v", session)
	}

	if session.AddressRef == "" {
		t.Fatal("the session did not record which delivery address it validated")
	}

	if session.ExpiresAt.IsZero() ||
		session.ReservationTTLSeconds != int((10*time.Minute).Seconds()) {
		t.Fatalf("ttl=%d expires=%s", session.ReservationTTLSeconds, session.ExpiresAt)
	}

	// The whole point: real stock left the available pool.
	after := env.availableStock(env.businessA, env.branchA1, item.InventoryItemID)

	if after != 7 {
		t.Fatalf("available after hold=%d want=7", after)
	}

	// The cart is frozen while the hold exists, so it cannot drift away from
	// what was validated and reserved.
	frozen := env.reloadCart(customer, current.ID)

	if frozen.Status != cart.StatusCheckoutInProgress {
		t.Fatalf("cart status=%s", frozen.Status)
	}

	if _, err = env.carts.AddItem(
		env.ctx(),
		customer,
		current.ID,
		frozen.Version,
		cart.AddItemInput{
			ItemRef:     item.ProductID,
			VariantRef:  item.VariantID,
			ListingRef:  item.ListingID,
			SellUnitRef: item.SellUnitID,
			Quantity:    1,
		},
	); !errors.Is(err, cart.ErrCartNotMutable) {
		t.Fatalf("cart accepted a mutation during checkout: %v", err)
	}
}

// TestSection17BCheckoutIsIdempotentForTheSameCart pins the "same customer
// runs checkout twice" gate: a second request must return the same session
// and must not hold the stock twice.
func TestSection17BCheckoutIsIdempotentForTheSameCart(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Beans",
		Quantity:   6,
		PriceMinor: 100000,
	})

	customer := env.customer("idem")

	current := env.seedShopCart(
		customer,
		s17bCartLine{Item: item, Quantity: 2},
	)

	first, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	second, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatalf("second StartCheckout: %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("second session=%s first=%s: a duplicate request created a second checkout",
			second.ID, first.ID)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 4 {
		t.Fatalf("available=%d want=4: the stock was held twice", available)
	}
}

// TestSection17BTwoCustomersCannotBothTakeTheLastStock is the concurrency
// gate that matters most. Both requests are released from one barrier and
// race the same final units on real PostgreSQL.
func TestSection17BTwoCustomersCannotBothTakeTheLastStock(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Garri",
		Quantity:   3,
		PriceMinor: 50000,
	})

	first := env.customer("race_a")
	second := env.customer("race_b")

	cartA := env.seedShopCart(first, s17bCartLine{Item: item, Quantity: 3})
	cartB := env.seedShopCart(second, s17bCartLine{Item: item, Quantity: 3})

	type outcome struct {
		session checkout.Session
		err     error
	}

	start := make(chan struct{})
	results := make(chan outcome, 2)

	run := func(customerID string, current cart.Cart) {
		<-start

		session, err := env.checkout.StartCheckout(
			env.ctx(),
			customerID,
			current.ID,
			current.Version,
		)

		results <- outcome{session: session, err: err}
	}

	go run(first, cartA)
	go run(second, cartB)

	close(start)

	validated := 0
	refused := 0

	for i := 0; i < 2; i++ {
		result := <-results

		if result.err != nil {
			t.Fatalf("checkout returned an error rather than a decision: %v", result.err)
		}

		switch result.session.Status {
		case checkout.StatusValidated:
			validated++

		case checkout.StatusFailed:
			refused++

			if result.session.FailureCode != checkout.FailureStockUnavailable {
				t.Fatalf("failure=%s want=%s",
					result.session.FailureCode, checkout.FailureStockUnavailable)
			}

		default:
			t.Fatalf("unexpected status %s", result.session.Status)
		}
	}

	if validated != 1 || refused != 1 {
		t.Fatalf("validated=%d refused=%d want exactly one of each", validated, refused)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 0 {
		t.Fatalf("available=%d want=0", available)
	}

	env.assertNoNegativeStock(item.InventoryItemID)
}

// TestSection17BMultiVendorPartialFailureLeavesNoHold is the multi-vendor
// atomicity gate: group A succeeds, group B cannot, and A must not stay
// reserved.
func TestSection17BMultiVendorPartialFailureLeavesNoHold(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	// Two different vendors, so the two reservations are genuinely two
	// separate Section 14 transactions rather than one.
	plenty := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Plenty",
		Quantity:   20,
		PriceMinor: 10000,
	})

	scarce := env.seedShopItem(s17bStockOptions{
		Business:   env.businessB,
		Staff:      env.staffB,
		Branch:     env.branchB1,
		Name:       "Scarce",
		Quantity:   1,
		PriceMinor: 10000,
	})

	customer := env.customer("multivendor")

	current := env.seedShopCart(
		customer,
		s17bCartLine{Item: plenty, Quantity: 2},
		s17bCartLine{Item: scarce, Quantity: 5},
	)

	session, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	if session.Status != checkout.StatusFailed ||
		session.FailureCode != checkout.FailureStockUnavailable {
		t.Fatalf("status=%s failure=%s", session.Status, session.FailureCode)
	}

	if session.ReservationCoordinationState != checkout.CoordinationCompensated {
		t.Fatalf("coordination=%s want=COMPENSATED",
			session.ReservationCoordinationState)
	}

	// A refused candidate must still add up: the session total is the sum of
	// its groups, which is the sum of their lines. Zeroing one level and not
	// the others would leave a record that contradicts itself.
	groupTotal := int64(0)

	for _, group := range session.Groups {
		lineTotal := int64(0)

		for _, line := range group.Lines {
			lineTotal += line.QuotedTotalMinor
		}

		if group.QuotedTotalMinor != lineTotal {
			t.Fatalf("group %s total=%d but its lines sum to %d",
				group.ID, group.QuotedTotalMinor, lineTotal)
		}

		groupTotal += group.QuotedTotalMinor
	}

	if session.QuotedItemsTotalMinor != groupTotal {
		t.Fatalf("session total=%d but its groups sum to %d",
			session.QuotedItemsTotalMinor, groupTotal)
	}

	// The group that succeeded must have been compensated, not left holding.
	for _, group := range session.Groups {
		if group.ReservationStatus == checkout.ReservationReserved {
			t.Fatalf("group %s/%s is still holding stock after a partial failure",
				group.BusinessID, group.BranchID)
		}
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		plenty.InventoryItemID,
	); available != 20 {
		t.Fatalf("vendor A available=%d want=20: a compensated hold was not released",
			available)
	}

	if available := env.availableStock(
		env.businessB,
		env.branchB1,
		scarce.InventoryItemID,
	); available != 1 {
		t.Fatalf("vendor B available=%d want=1", available)
	}

	// A failed candidate must hand the cart straight back.
	released := env.reloadCart(customer, current.ID)

	if released.Status != cart.StatusActive {
		t.Fatalf("cart status=%s want ACTIVE", released.Status)
	}
}

// TestSection17BMultiVendorSuccessReservesEveryGroup is the other half of the
// atomicity rule: all required groups RESERVED, or none.
func TestSection17BMultiVendorSuccessReservesEveryGroup(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	first := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "First",
		Quantity:   5,
		PriceMinor: 10000,
	})

	second := env.seedShopItem(s17bStockOptions{
		Business:   env.businessB,
		Staff:      env.staffB,
		Branch:     env.branchB1,
		Name:       "Second",
		Quantity:   5,
		PriceMinor: 20000,
	})

	customer := env.customer("multiok")

	current := env.seedShopCart(
		customer,
		s17bCartLine{Item: first, Quantity: 1},
		s17bCartLine{Item: second, Quantity: 2},
	)

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

	if len(session.Groups) != 2 {
		t.Fatalf("groups=%d want=2", len(session.Groups))
	}

	for _, group := range session.Groups {
		if group.ReservationStatus != checkout.ReservationReserved {
			t.Fatalf("group %s/%s reservation=%s",
				group.BusinessID, group.BranchID, group.ReservationStatus)
		}
	}

	if session.QuotedItemsTotalMinor != 50000 {
		t.Fatalf("session quoted=%d want=50000", session.QuotedItemsTotalMinor)
	}
}

// TestSection17BCancellationReleasesTheHoldAndTheCart proves the customer can
// always get out, and that getting out costs the vendor nothing.
func TestSection17BCancellationReleasesTheHoldAndTheCart(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Yam",
		Quantity:   8,
		PriceMinor: 30000,
	})

	customer := env.customer("cancel")

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

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 4 {
		t.Fatalf("available while held=%d want=4", available)
	}

	cancelled, err := env.checkout.CancelSession(env.ctx(), customer, session.ID)

	if err != nil {
		t.Fatal(err)
	}

	if cancelled.Status != checkout.StatusCancelled {
		t.Fatalf("status=%s", cancelled.Status)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 8 {
		t.Fatalf("available after cancel=%d want=8", available)
	}

	if released := env.reloadCart(customer, current.ID); released.Status != cart.StatusActive {
		t.Fatalf("cart status=%s want ACTIVE", released.Status)
	}

	// Cancelling twice is a conflict, not a second release.
	if _, err = env.checkout.CancelSession(
		env.ctx(),
		customer,
		session.ID,
	); !errors.Is(err, checkout.ErrSessionNotLive) {
		t.Fatalf("second cancel err=%v", err)
	}
}

// TestSection17BExpiredReservationBecomesReusableStock is the TTL gate.
//
// Expiry is reached by moving the checkout clock, never by sleeping, so the
// test asserts the rule rather than the wall clock.
func TestSection17BExpiredReservationBecomesReusableStock(t *testing.T) {
	env := newSection17BEnv(t, 2*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Onion",
		Quantity:   5,
		PriceMinor: 20000,
	})

	customer := env.customer("ttl")

	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 5})

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
		t.Fatalf("status=%s", session.Status)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 0 {
		t.Fatalf("available while held=%d want=0", available)
	}

	env.clock.advance(3 * time.Minute)

	// A read must never report an out-of-date hold as valid.
	expired, err := env.checkout.GetSession(env.ctx(), customer, session.ID)

	if err != nil {
		t.Fatal(err)
	}

	if expired.Status != checkout.StatusExpired {
		t.Fatalf("status=%s want EXPIRED", expired.Status)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 5 {
		t.Fatalf("available after expiry=%d want=5", available)
	}

	if released := env.reloadCart(customer, current.ID); released.Status != cart.StatusActive {
		t.Fatalf("cart status=%s want ACTIVE", released.Status)
	}

	// And the customer can immediately check out again.
	retry := env.reloadCart(customer, current.ID)

	again, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		retry.ID,
		retry.Version,
	)
	if err != nil {
		t.Fatalf("retry after expiry: %v", err)
	}

	if again.Status != checkout.StatusValidated {
		t.Fatal("retry: " + describeSession(again))
	}

	if again.ID == session.ID {
		t.Fatal("the expired session was reused rather than a new one created")
	}
}

// TestSection17BSweepReleasesLapsedHoldsWithoutAnyRequest is the worker gate:
// nobody reads the session, and the stock still comes back.
func TestSection17BSweepReleasesLapsedHoldsWithoutAnyRequest(t *testing.T) {
	env := newSection17BEnv(t, 2*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Pepper",
		Quantity:   4,
		PriceMinor: 15000,
	})

	customer := env.customer("sweep")

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

	env.clock.advance(3 * time.Minute)

	settled, err := env.checkout.SweepDueSessions(env.ctx(), 50)

	if err != nil {
		t.Fatal(err)
	}

	if settled == 0 {
		t.Fatal("the sweep settled nothing")
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 4 {
		t.Fatalf("available after sweep=%d want=4", available)
	}

	after, err := env.checkout.GetSession(env.ctx(), customer, session.ID)

	if err != nil {
		t.Fatal(err)
	}

	if after.Status != checkout.StatusExpired {
		t.Fatalf("status=%s want EXPIRED", after.Status)
	}

	// The sweep is idempotent: running it again changes nothing.
	if _, err = env.checkout.SweepDueSessions(env.ctx(), 50); err != nil {
		t.Fatal(err)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 4 {
		t.Fatalf("available after a second sweep=%d want=4", available)
	}
}

// TestSection17BCartMutationDuringValidationIsAConflict pins the ordering
// rule: the authoritative reads happen without the cart lock, and the lock
// path refuses anything that moved underneath them.
func TestSection17BCartMutationDuringValidationIsAConflict(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Tomato",
		Quantity:   10,
		PriceMinor: 40000,
	})

	customer := env.customer("mutation")

	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 1})

	// The client's view of the cart, captured before it edits it.
	stale := current.Version

	moved, err := env.carts.AddItem(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
		cart.AddItemInput{
			ItemRef:     item.ProductID,
			VariantRef:  item.VariantID,
			ListingRef:  item.ListingID,
			SellUnitRef: item.SellUnitID,
			Quantity:    2,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		stale,
	); !errors.Is(err, cart.ErrVersionConflict) {
		t.Fatalf("stale checkout err=%v want a version conflict", err)
	}

	// Nothing may be held for a checkout that was refused.
	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 10 {
		t.Fatalf("available=%d want=10", available)
	}

	// The current version still works.
	session, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		moved.ID,
		moved.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	if session.Status != checkout.StatusValidated {
		t.Fatal(describeSession(session))
	}
}

// TestSection17BStockRemovedBeforeCheckoutIsRefusedCleanly is the
// "stock adjustment vs checkout reserve" gate.
func TestSection17BStockRemovedBeforeCheckoutIsRefusedCleanly(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Salt",
		Quantity:   4,
		PriceMinor: 5000,
	})

	customer := env.customer("adjust")

	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 4})

	scope, err := inventory.ExactBranchScope(env.businessA, env.branchA1)

	if err != nil {
		t.Fatal(err)
	}

	// A vendor writes stock down through Section 14's own authority, exactly
	// as an operator would, between the cart and the checkout.
	if _, err = env.stock.Adjust(
		env.ctx(),
		inventory.Actor{
			StaffID:    env.staffA,
			BusinessID: env.businessA,
		},
		scope,
		inventory.QuantityMutationInput{
			BranchID:        env.branchA1,
			InventoryItemID: item.InventoryItemID,
			SourceType:      "MANUAL",
			SourceRef:       "s17b-writedown-" + env.suffix,
			ReasonCode:      "STOCK_WRITE_DOWN",
		},
		-3,
	); err != nil {
		t.Fatalf("adjust down: %v", err)
	}

	session, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	if session.Status != checkout.StatusFailed ||
		session.FailureCode != checkout.FailureStockUnavailable {
		t.Fatalf("status=%s failure=%s", session.Status, session.FailureCode)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 1 {
		t.Fatalf("available=%d want=1", available)
	}

	env.assertNoNegativeStock(item.InventoryItemID)
}

// TestSection17BCustomerCannotReachAnotherCustomersSession is the isolation
// gate. There is no customer identifier anywhere in the request surface, so
// the only way to try is to guess a session id.
func TestSection17BCustomerCannotReachAnotherCustomersSession(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Sugar",
		Quantity:   5,
		PriceMinor: 10000,
	})

	owner := env.customer("owner")
	intruder := env.customer("intruder")

	current := env.seedShopCart(owner, s17bCartLine{Item: item, Quantity: 1})

	session, err := env.checkout.StartCheckout(
		env.ctx(),
		owner,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = env.checkout.GetSession(
		env.ctx(),
		intruder,
		session.ID,
	); !errors.Is(err, checkout.ErrSessionNotFound) {
		t.Fatalf("cross-customer read err=%v", err)
	}

	if _, err = env.checkout.CancelSession(
		env.ctx(),
		intruder,
		session.ID,
	); !errors.Is(err, checkout.ErrSessionNotFound) {
		t.Fatalf("cross-customer cancel err=%v", err)
	}

	// And the owner's hold is untouched by the attempt.
	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 4 {
		t.Fatalf("available=%d want=4", available)
	}
}

// TestSection17BVariableWeightPreservesTheAuthorizedMaximum is the
// variable-weight gate: expected measure, tolerance and reservation basis all
// survive into the record Section 17D will quote from.
func TestSection17BVariableWeightPreservesTheAuthorizedMaximum(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	// A gram-based variable-fulfilment basis with 4000 grams on hand.
	item := env.seedShopItem(s17bStockOptions{
		Business:       env.businessA,
		Staff:          env.staffA,
		Branch:         env.branchA1,
		Name:           "Fresh Fish",
		Quantity:       4000,
		PriceMinor:     150000,
		VariableWeight: true,
		OverfillBps:    500,
		UnderfillBps:   300,
	})

	customer := env.customer("variable")

	current := env.seedShopCart(
		customer,
		s17bCartLine{
			Item:                    item,
			Quantity:                2,
			ExpectedMeasureQuantity: 1000,
		},
	)

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

	if len(session.Groups) != 1 || len(session.Groups[0].Lines) != 1 {
		t.Fatalf("groups=%+v", session.Groups)
	}

	line := session.Groups[0].Lines[0]

	if line.PricingBasis != cart.PricingBasisVariableMeasure {
		t.Fatalf("pricing basis=%s", line.PricingBasis)
	}

	if line.ExpectedMeasureQuantity != 1000 ||
		line.RequestedMeasureRef != item.MeasureID {
		t.Fatalf("measure=%s expected=%d",
			line.RequestedMeasureRef, line.ExpectedMeasureQuantity)
	}

	if line.MeasureOverfillBps != 500 || line.MeasureUnderfillBps != 300 {
		t.Fatalf("tolerance over=%d under=%d",
			line.MeasureOverfillBps, line.MeasureUnderfillBps)
	}

	// 150000 x 2 quoted, plus 5% authorized headroom for the overfill the
	// vendor may legitimately deliver.
	if line.QuotedTotalMinor != 300000 {
		t.Fatalf("quoted=%d want=300000", line.QuotedTotalMinor)
	}

	if line.MaxAuthorizedTotalMinor != 315000 {
		t.Fatalf("maximum=%d want=315000", line.MaxAuthorizedTotalMinor)
	}

	// A variable-fulfilment line reserves in the stock measure, not in the
	// ordered count: 2 x 1000 grams.
	if line.ReservationBasis != checkout.BasisStockMeasure {
		t.Fatalf("reservation basis=%s", line.ReservationBasis)
	}

	if line.ReservationQuantity != 2000 {
		t.Fatalf("reservation quantity=%d want=2000", line.ReservationQuantity)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 2000 {
		t.Fatalf("available=%d want=2000", available)
	}
}

// TestSection17BFractionalSellUnitFailsClosed states the boundary honestly: a
// half-unit sell unit has no conversion authority anywhere in this platform,
// so checkout refuses rather than reserving a number it guessed.
func TestSection17BFractionalSellUnitFailsClosed(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:            env.businessA,
		Staff:               env.staffA,
		Branch:              env.branchA1,
		Name:                "Half Measure",
		Quantity:            10,
		PriceMinor:          10000,
		FractionDenominator: 2,
		MeasureID:           "smd_paint",
	})

	customer := env.customer("fraction")

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

	if session.Status != checkout.StatusFailed ||
		session.FailureCode != checkout.FailureLineNotSellable {
		t.Fatalf("status=%s failure=%s", session.Status, session.FailureCode)
	}

	found := false

	for _, group := range session.Groups {
		for _, line := range group.Lines {
			if line.ReasonCode == checkout.ReasonReservationBasisUnknown {
				found = true
			}
		}
	}

	if !found {
		t.Fatalf("no line reported %s: %+v",
			checkout.ReasonReservationBasisUnknown, session.Groups)
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 10 {
		t.Fatalf("available=%d want=10: a refused line still took stock", available)
	}
}

// TestSection17BCheckoutRefusesACartWithNoDeliveryAddress states the
// precondition explicitly rather than producing a candidate that cannot be
// grouped or fulfilled.
func TestSection17BCheckoutRefusesACartWithNoDeliveryAddress(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Milk",
		Quantity:   5,
		PriceMinor: 10000,
	})

	customer := env.customer("noaddress")

	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 1})

	current, err := env.carts.RemoveDeliveryAddress(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)

	var precondition checkout.CartPreconditionError

	if !errors.As(err, &precondition) ||
		precondition.Code != checkout.PreconditionDeliveryAddress {
		t.Fatalf("err=%v want a delivery-address precondition", err)
	}
}

func (e *s17bEnv) assertNoNegativeStock(inventoryItemID string) {
	e.t.Helper()

	var (
		onHand      int64
		reserved    int64
		quarantined int64
	)

	if err := e.pool.QueryRow(
		e.ctx(),
		`
        SELECT on_hand_quantity, reserved_quantity, quarantined_quantity
        FROM inventory_balances
        WHERE inventory_item_id=$1
        `,
		inventoryItemID,
	).Scan(&onHand, &reserved, &quarantined); err != nil {
		e.t.Fatal(err)
	}

	if onHand < 0 || reserved < 0 || quarantined < 0 {
		e.t.Fatalf("negative stock: on_hand=%d reserved=%d quarantined=%d",
			onHand, reserved, quarantined)
	}

	if reserved+quarantined > onHand {
		e.t.Fatalf("reserved+quarantined=%d exceeds on_hand=%d",
			reserved+quarantined, onHand)
	}
}
