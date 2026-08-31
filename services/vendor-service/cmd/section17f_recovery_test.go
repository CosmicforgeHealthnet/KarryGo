package main

// Section 17F crash-recovery integration tests.
//
// A restart is simulated the only honest way: by leaving the durable rows in
// exactly the state a crash would leave them, then running the recovery pass
// that a restarted process would run. Nothing here relies on an in-memory
// goroutine having finished, because that is precisely what a crash destroys.

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// crashDuringAuthorization leaves an operation exactly as a process that died
// mid-call would: legs durable, outcome unknown.
func crashDuringAuthorization(
	t *testing.T, fixture section17EFixture, authority *fakeAuthority,
) orders.ParentOrder {
	t.Helper()
	ctx := context.Background()

	order, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Every leg refuses, so the run ends with the outcome unknown locally.
	authority.failFor = "*"
	for _, vendorOrder := range order.VendorOrders {
		authority.failFor = "vendor-order-payment-" + vendorOrder.ID
		break
	}

	if _, _, err = fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	); err == nil {
		t.Fatal("expected the staged failure to surface")
	}
	return order
}

// The money authority is the only source of truth about what happened. If it
// says the leg was actually held, recovery adopts that answer.
func TestSection17FRecovery_AdoptsTheAuthorityTruthAfterACrash(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, false), authority)
	ctx := context.Background()

	order := crashDuringAuthorization(t, fixture, authority)

	// A restarted process knows only what the rows say.
	before, err := fixture.service.GetPaymentForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.State == orders.PaymentHeld {
		t.Fatal("a crashed authorization already reported HELD")
	}

	// The authority will now answer, so the leg gets a reference and resolves.
	authority.failFor = ""

	outcome, err := fixture.service.RecoverPayments(ctx, orders.RecoveryLimits{
		Grace: time.Nanosecond, Batch: 50,
	})
	if err != nil {
		t.Fatalf("RecoverPayments: %v", err)
	}
	if outcome.LegsReconciled == 0 {
		t.Fatal("recovery found nothing to reconcile")
	}

	after, err := fixture.service.GetPaymentForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != orders.PaymentHeld {
		t.Fatalf("state after recovery=%s, want HELD", after.State)
	}
	for _, leg := range after.Legs {
		if leg.Status != orders.LegHeld || leg.IntentReference == "" {
			t.Errorf("leg unresolved after recovery: %+v", leg)
		}
	}

	// And the order's mirrored state followed.
	reread, err := fixture.service.GetForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.PaymentState != string(orders.PaymentHeld) {
		t.Fatalf("order payment state=%q after recovery", reread.PaymentState)
	}
}

// A leg that already carries an authority reference is READ, never re-paid:
// the authority created something and asking is the only safe way to learn what.
func TestSection17FRecovery_AlreadyReferencedLegIsReadNotRepaid(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, false), authority)
	ctx := context.Background()

	order, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	// The authority answered with an unrecognised status, so the leg keeps its
	// reference but stays unresolved.
	authority.answer["vendor-order-payment-"+order.VendorOrders[0].ID] = "processing"
	if _, _, err = fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	); err != nil {
		t.Fatal(err)
	}
	holdsBefore := authority.callCount()

	if _, err = fixture.service.RecoverPayments(ctx, orders.RecoveryLimits{
		Grace: time.Nanosecond, Batch: 50,
	}); err != nil {
		t.Fatal(err)
	}

	// It was read, not held again.
	if len(authority.reads) == 0 {
		t.Fatal("recovery did not read the authority for a referenced leg")
	}
	if authority.callCount() != holdsBefore {
		t.Fatalf("recovery re-paid a referenced leg: %d -> %d",
			holdsBefore, authority.callCount())
	}
}

// Recovery must be safe to run repeatedly. Running it twice produces the same
// state and no second financial effect.
func TestSection17FRecovery_IsSafeToRunRepeatedly(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, true), authority)
	ctx := context.Background()

	order := crashDuringAuthorization(t, fixture, authority)
	authority.failFor = ""

	limits := orders.RecoveryLimits{Grace: time.Nanosecond, Batch: 50}
	if _, err := fixture.service.RecoverPayments(ctx, limits); err != nil {
		t.Fatal(err)
	}
	first, err := fixture.service.GetPaymentForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = fixture.service.RecoverPayments(ctx, limits); err != nil {
		t.Fatal(err)
	}
	second, err := fixture.service.GetPaymentForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}

	if first.State != second.State {
		t.Fatalf("a second recovery changed the state: %s -> %s", first.State, second.State)
	}
	if len(authority.refundedFor()) != 0 {
		t.Fatal("a healthy operation was refunded by recovery")
	}
}

// A leg the authority will never resolve must not hold stock forever. Past the
// deadline it is treated as failed and the operation unwinds.
func TestSection17FRecovery_DeadlineUnwindsAnUnanswerableLeg(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, false), authority)
	ctx := context.Background()

	order := crashDuringAuthorization(t, fixture, authority)
	// The authority stays unreachable.
	authority.readFails = true

	// Before the deadline, recovery waits rather than deciding.
	if _, err := fixture.service.RecoverPayments(ctx, orders.RecoveryLimits{
		Grace: time.Nanosecond, Deadline: time.Hour, Batch: 50,
	}); err != nil {
		t.Fatal(err)
	}
	waiting, err := fixture.service.GetPaymentForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.State.Terminal() {
		t.Fatalf("recovery gave up before the deadline: %s", waiting.State)
	}

	// Past the deadline it stops waiting.
	if _, err = fixture.service.RecoverPayments(ctx, orders.RecoveryLimits{
		Grace: time.Nanosecond, Deadline: time.Nanosecond, Batch: 50,
	}); err != nil {
		t.Fatal(err)
	}
	unwound, err := fixture.service.GetPaymentForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unwound.State == orders.PaymentHeld {
		t.Fatal("an unanswerable payment was reported as held")
	}
}

// The dangerous case in full: one leg held, another permanently failed. The
// held leg must be refunded through the Wallet authority, and the operation
// only becomes terminal once nothing is left held.
func TestSection17FRecovery_CompensatesAHeldLegWhenTheOperationCannotComplete(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, true), authority)
	ctx := context.Background()

	order, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.vendorOrderFor(t, order, fixture.env.businessA)
	second := fixture.vendorOrderFor(t, order, fixture.env.businessB)

	// The second leg is refused by the authority for good.
	authority.failFor = "vendor-order-payment-" + second.ID
	if _, _, err = fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	); err == nil {
		t.Fatal("expected the refused leg to surface")
	}
	authority.readFails = true

	// Past the deadline, the refused leg becomes FAILED, which puts the whole
	// operation into COMPENSATING.
	if _, err = fixture.service.RecoverPayments(ctx, orders.RecoveryLimits{
		Grace: time.Nanosecond, Deadline: time.Nanosecond, Batch: 50,
	}); err != nil {
		t.Fatal(err)
	}

	// The next pass unwinds it: the held leg is refunded.
	authority.readFails = false
	if _, err = fixture.service.RecoverPayments(ctx, orders.RecoveryLimits{
		Grace: time.Nanosecond, Deadline: time.Nanosecond, Batch: 50,
	}); err != nil {
		t.Fatal(err)
	}

	operation, err := fixture.service.GetPaymentForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if operation.State != orders.PaymentCompensated {
		t.Fatalf("state=%s, want COMPENSATED", operation.State)
	}

	refunds := authority.refundedFor()
	if len(refunds) != 1 {
		t.Fatalf("refunds=%d, want exactly the one held leg", len(refunds))
	}
	// The refund names the vendor allocation it belongs to, and only that one.
	if refunds[0].SourceReference != first.ID {
		t.Fatalf("refund source=%q, want vendor order %s",
			refunds[0].SourceReference, first.ID)
	}
	if refunds[0].AmountKobo != first.CustomerPayableMinor {
		t.Fatalf("refund amount=%d, want %d",
			refunds[0].AmountKobo, first.CustomerPayableMinor)
	}

	// Nothing may still be held once the operation is terminal.
	for _, leg := range operation.Legs {
		if leg.Status == orders.LegHeld {
			t.Errorf("a terminal operation still holds customer money: %+v", leg)
		}
	}
}

// An operation cannot be declared unwound while the authority has not actually
// returned the money.
func TestSection17FRecovery_WillNotCloseOutWhileMoneyIsStillHeld(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, true), authority)
	ctx := context.Background()

	order, _, err := fixture.service.CreateFromQuote(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	second := fixture.vendorOrderFor(t, order, fixture.env.businessB)
	authority.failFor = "vendor-order-payment-" + second.ID
	if _, _, err = fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	); err == nil {
		t.Fatal("expected the refused leg to surface")
	}

	// The refund fails from the outset, so the pass that decides the operation
	// cannot complete is also the pass whose unwind is refused.
	authority.readFails = true
	authority.refundFails = true
	if _, err = fixture.service.RecoverPayments(ctx, orders.RecoveryLimits{
		Grace: time.Nanosecond, Deadline: time.Nanosecond, Batch: 50,
	}); err != nil {
		t.Fatal(err)
	}

	operation, err := fixture.service.GetPaymentForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if operation.State == orders.PaymentCompensated {
		t.Fatal("an operation was closed out while the customer's money was still held")
	}

	// Once the refund succeeds, the unwind completes on a later pass.
	authority.refundFails = false
	if _, err = fixture.service.RecoverPayments(ctx, orders.RecoveryLimits{
		Grace: time.Nanosecond, Deadline: time.Nanosecond, Batch: 50,
	}); err != nil {
		t.Fatal(err)
	}
	final, err := fixture.service.GetPaymentForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != orders.PaymentCompensated {
		t.Fatalf("state=%s, want COMPENSATED once the refund succeeded", final.State)
	}
}

// Recovery must never touch an operation that is doing fine.
func TestSection17FRecovery_LeavesAHealthyPaymentAlone(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(newSection17EFixture(t, true), authority)
	ctx := context.Background()

	_, operation, err := fixture.service.StartPayment(
		ctx, fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatal(err)
	}
	if operation.State != orders.PaymentHeld {
		t.Fatalf("setup state=%s", operation.State)
	}
	callsBefore := authority.callCount()

	outcome, err := fixture.service.RecoverPayments(ctx, orders.RecoveryLimits{
		Grace: time.Nanosecond, Deadline: time.Nanosecond, Batch: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.LegsReconciled != 0 {
		t.Fatalf("recovery touched %d legs of a healthy payment", outcome.LegsReconciled)
	}
	if authority.callCount() != callsBefore || len(authority.refundedFor()) != 0 {
		t.Fatal("recovery called the money authority about a healthy payment")
	}
}

// With no money authority wired there is nothing to recover, and the sweep must
// be a harmless no-op rather than an error loop.
func TestSection17FRecovery_NoAuthorityIsAHarmlessNoOp(t *testing.T) {
	fixture := newSection17EFixture(t, false)

	outcome, err := fixture.service.RecoverPayments(
		context.Background(), orders.RecoveryLimits{},
	)
	if err != nil {
		t.Fatalf("recovery without an authority errored: %v", err)
	}
	if outcome.LegsReconciled != 0 || outcome.Compensated != 0 {
		t.Fatalf("recovery acted without an authority: %+v", outcome)
	}
}
