package main

// Customer service-level (fulfilment-mode) change evidence.
//
// The address-change path is covered by section17f_change_test.go. This adds
// the other reason a customer re-quotes after paying: they want the order
// FASTER and are willing to pay for it.
//
// It matters separately because a service-level change is the case where a
// post-payment price INCREASE is the expected outcome rather than an anomaly.
// Everything protecting the customer therefore has to hold at once — the paid
// price stays put until they say yes, the yes has to be theirs, and saying it
// twice must not charge them twice.
//
// Nothing here is a new capability. The commercial-change chain is generic
// over "a new checkout session or fulfilment plan", and a mode change is
// simply a different fulfilment plan. This test exists because that
// genericity was previously assumed rather than demonstrated: SelectedMode
// appeared in the codebase only as a struct field, an adapter copy and a
// fixture literal, with no test exercising a mode transition end to end.

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// serviceLevelUpgradeDelta is what the customer is being asked to pay to move
// from BALANCED to FASTEST. It is deliberately large enough that a silent
// charge would be unmistakable in the assertions below.
const serviceLevelUpgradeDelta int64 = 40_000

// switchToFasterService sets up the precondition this test is about: an order
// that was paid for at BALANCED, and a replacement quote that is FASTEST.
//
// It writes both directly, because the mode is frozen evidence on the paid
// order and on the replacement quote rather than something the change chain
// derives. The rows belong to this test's own isolated fixture.
func switchToFasterService(
	t *testing.T,
	fixture section17EFixture,
	order orders.ParentOrder,
	replacementQuoteID string,
) {
	t.Helper()

	ctx := context.Background()

	fixture.env.exec(ctx,
		`UPDATE commerce_parent_orders SET selected_fulfilment_mode='BALANCED' WHERE id=$1`,
		order.ID)

	fixture.env.exec(ctx,
		`UPDATE commerce_quotes SET selected_fulfilment_mode='BALANCED' WHERE id=$1`,
		order.QuoteID)

	fixture.env.exec(ctx,
		`UPDATE commerce_quotes SET selected_fulfilment_mode='FASTEST' WHERE id=$1`,
		replacementQuoteID)
}

// fulfilmentMode reads a quote's frozen service level back from the database,
// so every assertion below is about stored evidence rather than about the
// value a scripted authority happened to return.
func fulfilmentMode(t *testing.T, fixture section17EFixture, quoteID string) string {
	t.Helper()

	var mode string
	if err := fixture.env.pool.QueryRow(context.Background(),
		`SELECT selected_fulfilment_mode FROM commerce_quotes WHERE id=$1`,
		quoteID).Scan(&mode); err != nil {
		t.Fatalf("read fulfilment mode for %s: %v", quoteID, err)
	}

	return mode
}

// orderFulfilmentMode reads the paid order's own frozen service level.
func orderFulfilmentMode(t *testing.T, fixture section17EFixture, orderID string) string {
	t.Helper()

	var mode string
	if err := fixture.env.pool.QueryRow(context.Background(),
		`SELECT selected_fulfilment_mode FROM commerce_parent_orders WHERE id=$1`,
		orderID).Scan(&mode); err != nil {
		t.Fatalf("read order fulfilment mode: %v", err)
	}

	return mode
}

// adjustmentCount counts the financial adjustments filed against one change.
func adjustmentCount(t *testing.T, fixture section17EFixture, changeID string) int {
	t.Helper()

	var count int
	if err := fixture.env.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM commerce_order_adjustments WHERE change_id=$1`,
		changeID).Scan(&count); err != nil {
		t.Fatalf("count adjustments: %v", err)
	}

	return count
}

// A customer upgrading BALANCED to FASTEST after paying.
//
// Five things have to be true at once, and they are asserted in the order the
// customer experiences them.
func TestSection17FChange_ServiceLevelUpgradeRequiresApprovalAndChargesOnce(t *testing.T) {
	fixture, order, quotes, authority := changeFixture(t, serviceLevelUpgradeDelta)
	ctx := context.Background()

	paidTotal := order.CustomerApprovedTotalMinor
	switchToFasterService(t, fixture, order, quotes.quoteID)

	// ---------------------------------------------------------------------
	// 1. The re-quote happens, and it is a genuine service-level change.
	// ---------------------------------------------------------------------

	change := requestChange(t, fixture, order)

	if quotes.created != 1 {
		t.Fatalf("replacement quotes created=%d, want exactly 1", quotes.created)
	}

	if change.State != orders.ChangeAwaitingApproval {
		t.Fatalf("state=%s, want AWAITING_CUSTOMER_APPROVAL", change.State)
	}

	if got := fulfilmentMode(t, fixture, change.ReplacementQuote.ID); got != "FASTEST" {
		t.Fatalf("replacement quote service level=%s, want FASTEST", got)
	}

	if got := fulfilmentMode(t, fixture, order.QuoteID); got != "BALANCED" {
		t.Fatalf("the original quote's service level changed to %s; a replacement must never rewrite the agreement it replaces",
			got)
	}

	// The delta is the platform's own arithmetic over stored evidence. The
	// request body carries no amount — ChangeRequest has two reference fields
	// and nothing else — so a customer cannot name the price of their own
	// upgrade.
	if change.DeltaMinor != serviceLevelUpgradeDelta {
		t.Fatalf("delta=%d, want %d derived from the replacement quote total",
			change.DeltaMinor, serviceLevelUpgradeDelta)
	}

	// ---------------------------------------------------------------------
	// 2. Before approval, the customer still owes exactly what they agreed.
	// ---------------------------------------------------------------------

	pending, err := fixture.service.GetCommercialChange(ctx, fixture.customerID, change.ID)
	if err != nil {
		t.Fatalf("GetCommercialChange: %v", err)
	}

	if pending.WalletStatus != orders.WalletNone {
		t.Fatalf("wallet status=%s before approval, want NONE - asking for a faster delivery is not agreeing to pay for one",
			pending.WalletStatus)
	}

	if calls := authority.callCount(); calls != 0 {
		t.Fatalf("the payment authority was called %d times before approval", calls)
	}

	if adjustments := adjustmentCount(t, fixture, change.ID); adjustments != 0 {
		t.Fatalf("adjustments=%d before approval, want 0", adjustments)
	}

	var storedPaidTotal int64
	if err = fixture.env.pool.QueryRow(ctx,
		`SELECT customer_approved_total_minor FROM commerce_parent_orders WHERE id=$1`,
		order.ID).Scan(&storedPaidTotal); err != nil {
		t.Fatalf("read paid total: %v", err)
	}

	if storedPaidTotal != paidTotal {
		t.Fatalf("the paid total moved from %d to %d without approval",
			paidTotal, storedPaidTotal)
	}

	if got := orderFulfilmentMode(t, fixture, order.ID); got != "BALANCED" {
		t.Fatalf("the order was upgraded to %s before the customer approved it", got)
	}

	// ---------------------------------------------------------------------
	// 3. The approval has to be the customer's, and it has to be this quote.
	// ---------------------------------------------------------------------

	if _, err = fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, "not-the-fingerprint",
	); err == nil {
		t.Fatal("a wrong fingerprint approved a paid price increase")
	}

	if calls := authority.callCount(); calls != 0 {
		t.Fatalf("a rejected approval still called the payment authority %d times", calls)
	}

	// ---------------------------------------------------------------------
	// 4. With the real approval, exactly one adjustment is filed.
	// ---------------------------------------------------------------------

	approved, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, change.ReplacementQuote.Fingerprint,
	)
	if err != nil {
		t.Fatalf("ApproveCommercialChange: %v", err)
	}

	if approved.State != orders.ChangeApplied {
		t.Fatalf("state=%s after approval, want APPLIED", approved.State)
	}

	if approved.WalletStatus != orders.WalletHeld {
		t.Fatalf("wallet status=%s, want HELD for a positive delta",
			approved.WalletStatus)
	}

	if adjustments := adjustmentCount(t, fixture, change.ID); adjustments != 1 {
		t.Fatalf("adjustments=%d after approval, want exactly 1", adjustments)
	}

	var (
		adjustmentType  string
		differenceMinor int64
	)

	if err = fixture.env.pool.QueryRow(ctx,
		`SELECT adjustment_type,difference_minor FROM commerce_order_adjustments WHERE change_id=$1`,
		change.ID).Scan(&adjustmentType, &differenceMinor); err != nil {
		t.Fatalf("read adjustment: %v", err)
	}

	if adjustmentType != "POSITIVE_DELTA" {
		t.Fatalf("adjustment type=%s, want POSITIVE_DELTA", adjustmentType)
	}

	// The customer is charged the upgrade and nothing more. Charging the new
	// total again, rather than the difference, is the mistake this pins.
	if differenceMinor != serviceLevelUpgradeDelta {
		t.Fatalf("charged %d, want the %d difference and not the whole new total",
			differenceMinor, serviceLevelUpgradeDelta)
	}

	callsAfterApproval := authority.callCount()
	if callsAfterApproval == 0 {
		t.Fatal("the approved upgrade never reached the payment authority")
	}

	// ---------------------------------------------------------------------
	// 5. Approving again is idempotent, not a second charge.
	// ---------------------------------------------------------------------

	repeat, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, change.ReplacementQuote.Fingerprint,
	)
	if err != nil {
		t.Fatalf("repeat approval returned an error instead of the applied change: %v", err)
	}

	if repeat.ID != change.ID || repeat.State != orders.ChangeApplied {
		t.Fatalf("repeat approval produced a different outcome: id=%s state=%s",
			repeat.ID, repeat.State)
	}

	if calls := authority.callCount(); calls != callsAfterApproval {
		t.Fatalf("repeat approval charged again: %d -> %d",
			callsAfterApproval, calls)
	}

	if adjustments := adjustmentCount(t, fixture, change.ID); adjustments != 1 {
		t.Fatalf("adjustments=%d after a duplicate approval, want exactly 1",
			adjustments)
	}
}

// A service-level upgrade the customer declines leaves them on what they paid
// for. Without this, "approval is required" could be satisfied by a flow that
// applies the change anyway once the customer has looked at it.
func TestSection17FChange_DeclinedServiceLevelUpgradeChargesNothing(t *testing.T) {
	fixture, order, quotes, authority := changeFixture(t, serviceLevelUpgradeDelta)
	ctx := context.Background()

	switchToFasterService(t, fixture, order, quotes.quoteID)

	change := requestChange(t, fixture, order)

	if _, err := fixture.service.RejectCommercialChange(
		ctx, fixture.customerID, change.ID,
	); err != nil {
		t.Fatalf("RejectCommercialChange: %v", err)
	}

	if calls := authority.callCount(); calls != 0 {
		t.Fatalf("a declined upgrade called the payment authority %d times", calls)
	}

	if adjustments := adjustmentCount(t, fixture, change.ID); adjustments != 0 {
		t.Fatalf("adjustments=%d after a decline, want 0", adjustments)
	}

	if got := orderFulfilmentMode(t, fixture, order.ID); got != "BALANCED" {
		t.Fatalf("a declined upgrade still moved the order to %s", got)
	}

	// And the declined change cannot be resurrected by approving it anyway.
	if _, err := fixture.service.ApproveCommercialChange(
		ctx, fixture.customerID, change.ID, change.ReplacementQuote.Fingerprint,
	); !errors.Is(err, orders.ErrCommercialChangeConflict) {
		t.Fatalf("approving a rejected change returned %v, want ErrCommercialChangeConflict", err)
	}

	if calls := authority.callCount(); calls != 0 {
		t.Fatalf("approving a rejected change charged the customer: %d calls", calls)
	}
}
