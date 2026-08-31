package main

// Section 17F cheaper-fulfilment benefit test evidence.
//
// ApplyCheaperFulfilmentBenefit compares what the CUSTOMER was committed to for
// delivery against the eligible final customer amount. Internal provider
// operating cost is deliberately not an input, which is what keeps an internal
// replan from ever reaching the customer's charge.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

const cheaperPolicy = "CUSTOMER_BENEFIT_ON_CHEAPER_DELIVERY"

// cheaperFixture is a paid, activated multi-vendor order.
func cheaperFixture(
	t *testing.T,
) (section17EFixture, orders.ParentOrder, *fakeAuthority) {
	t.Helper()

	authority := newFakeAuthority()
	fixture := withPayment(buildSection17EFixture(t, true, false), authority)
	stock, missions := &fakeInventory{}, newFakeMissions()
	fixture.service.SetFulfilmentAuthorities(stock, missions)

	order, _, err := fixture.service.StartPayment(
		context.Background(), fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}
	return fixture, order, authority
}

// committedDelivery is what the customer was actually committed to pay for
// delivery across the whole purchase.
func committedDelivery(order orders.ParentOrder) int64 {
	var total int64
	for _, vendorOrder := range order.VendorOrders {
		total += vendorOrder.DeliveryCustomerMinor
	}
	return total
}

// -----------------------------------------------------------------------------
// A / B — the benefit is refunded, and the original quote is untouched
// -----------------------------------------------------------------------------

func TestSection17FCheaper_RefundsExactlyTheDifferenceThroughWallet(t *testing.T) {
	fixture, order, authority := cheaperFixture(t)
	ctx := context.Background()

	committed := committedDelivery(order)
	if committed <= 0 {
		t.Fatalf("fixture committed no customer delivery: %d", committed)
	}
	const benefit = 100
	eligible := committed - benefit

	adjustment, err := fixture.service.ApplyCheaperFulfilmentBenefit(
		ctx, order.ID, eligible, cheaperPolicy,
	)
	if err != nil {
		t.Fatalf("ApplyCheaperFulfilmentBenefit: %v", err)
	}

	if adjustment.DifferenceMinor != benefit {
		t.Fatalf("difference=%d, want %d", adjustment.DifferenceMinor, benefit)
	}
	if adjustment.OriginalCommitmentMinor != committed {
		t.Fatalf("recorded commitment=%d, want %d",
			adjustment.OriginalCommitmentMinor, committed)
	}
	if adjustment.WalletStatus != orders.WalletRefunded {
		t.Fatalf("wallet status=%s, want REFUNDED", adjustment.WalletStatus)
	}

	// Exactly one Wallet movement, for exactly the difference. Never both a
	// release and a refund.
	refunds := authority.refundedFor()
	if len(refunds) != 1 {
		t.Fatalf("wallet refunds=%d, want exactly 1", len(refunds))
	}
	if refunds[0].AmountKobo != benefit {
		t.Fatalf("refunded %d, want the difference %d", refunds[0].AmountKobo, benefit)
	}

	// B — the original quote still says what it always said.
	var quotedTotal, deliverySubtotal int64
	if err = fixture.env.pool.QueryRow(ctx,
		`SELECT quoted_total_minor,delivery_subtotal_minor FROM commerce_quotes WHERE id=$1`,
		order.QuoteID).Scan(&quotedTotal, &deliverySubtotal); err != nil {
		t.Fatal(err)
	}
	if quotedTotal != order.CustomerApprovedTotalMinor {
		t.Fatalf("the original quote was rewritten to %d", quotedTotal)
	}

	// And the order's own approved total is equally untouched: the benefit is
	// separate durable evidence, not an edit.
	reread, err := fixture.service.GetForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.CustomerApprovedTotalMinor != order.CustomerApprovedTotalMinor {
		t.Fatalf("approved total changed to %d", reread.CustomerApprovedTotalMinor)
	}

	var adjustments int
	if err = fixture.env.pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_order_adjustments WHERE parent_order_id=$1`,
		order.ID).Scan(&adjustments); err != nil {
		t.Fatal(err)
	}
	if adjustments != 1 {
		t.Fatalf("adjustment rows=%d, want 1", adjustments)
	}
}

// -----------------------------------------------------------------------------
// C — exactly once
// -----------------------------------------------------------------------------

func TestSection17FCheaper_IsExactlyOnceAcrossRepeats(t *testing.T) {
	fixture, order, authority := cheaperFixture(t)
	ctx := context.Background()
	eligible := committedDelivery(order) - 100

	for attempt := 0; attempt < 4; attempt++ {
		if _, err := fixture.service.ApplyCheaperFulfilmentBenefit(
			ctx, order.ID, eligible, cheaperPolicy,
		); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}

	if refunds := authority.refundedFor(); len(refunds) != 1 {
		t.Fatalf("wallet refunds=%d after 4 attempts, want 1", len(refunds))
	}

	var adjustments int
	if err := fixture.env.pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_order_adjustments WHERE parent_order_id=$1`,
		order.ID).Scan(&adjustments); err != nil {
		t.Fatal(err)
	}
	if adjustments != 1 {
		t.Fatalf("adjustment rows=%d after 4 attempts, want 1", adjustments)
	}
}

func TestSection17FCheaper_ConcurrentApplicationsRefundOnce(t *testing.T) {
	fixture, order, authority := cheaperFixture(t)
	eligible := committedDelivery(order) - 100

	var wait sync.WaitGroup
	for i := 0; i < 6; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _ = fixture.service.ApplyCheaperFulfilmentBenefit(
				context.Background(), order.ID, eligible, cheaperPolicy,
			)
		}()
	}
	wait.Wait()

	var adjustments int
	if err := fixture.env.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM commerce_order_adjustments WHERE parent_order_id=$1`,
		order.ID).Scan(&adjustments); err != nil {
		t.Fatal(err)
	}
	if adjustments != 1 {
		t.Fatalf("concurrent applications produced %d adjustments, want 1", adjustments)
	}
	if refunds := authority.refundedFor(); len(refunds) > 1 {
		t.Fatalf("concurrent applications refunded %d times", len(refunds))
	}
}

// -----------------------------------------------------------------------------
// D / E — unknown outcome and crash recovery
// -----------------------------------------------------------------------------

// A Wallet timeout may still have moved money, so the local state stays
// UNKNOWN and recovery reconciles it. It must never move money twice.
func TestSection17FCheaper_WalletTimeoutStaysUnknownThenRecovers(t *testing.T) {
	fixture, order, authority := cheaperFixture(t)
	ctx := context.Background()
	eligible := committedDelivery(order) - 100

	authority.refundFails = true
	if _, err := fixture.service.ApplyCheaperFulfilmentBenefit(
		ctx, order.ID, eligible, cheaperPolicy,
	); !errors.Is(err, orders.ErrPaymentAuthority) {
		t.Fatalf("err=%v, want ErrPaymentAuthority", err)
	}

	var status string
	if err := fixture.env.pool.QueryRow(ctx,
		`SELECT wallet_status FROM commerce_order_adjustments WHERE parent_order_id=$1`,
		order.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(orders.WalletUnknown) {
		t.Fatalf("wallet status=%s after a timeout, want UNKNOWN", status)
	}

	// E — the process restarts and recovery converges it.
	authority.refundFails = false
	if _, err := fixture.service.RecoverPayments(ctx, orders.RecoveryLimits{
		Grace: time.Nanosecond, Deadline: time.Hour, Batch: 50,
	}); err != nil {
		t.Fatalf("RecoverPayments: %v", err)
	}

	if err := fixture.env.pool.QueryRow(ctx,
		`SELECT wallet_status FROM commerce_order_adjustments WHERE parent_order_id=$1`,
		order.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(orders.WalletRefunded) {
		t.Fatalf("wallet status=%s after recovery, want REFUNDED", status)
	}

	// Exactly one movement across the whole episode.
	if refunds := authority.refundedFor(); len(refunds) != 1 {
		t.Fatalf("wallet refunds=%d across timeout and recovery, want 1", len(refunds))
	}

	var adjustments int
	if err := fixture.env.pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_order_adjustments WHERE parent_order_id=$1`,
		order.ID).Scan(&adjustments); err != nil {
		t.Fatal(err)
	}
	if adjustments != 1 {
		t.Fatalf("adjustment rows=%d, want 1", adjustments)
	}
}

// -----------------------------------------------------------------------------
// F — a more expensive internal replan never reaches the customer
// -----------------------------------------------------------------------------

func TestSection17FCheaper_MoreExpensiveInternalReplanCreatesNoCustomerCharge(t *testing.T) {
	fixture, order, authority := cheaperFixture(t)
	ctx := context.Background()

	committed := committedDelivery(order)
	callsBefore := authority.callCount()

	// The internal outcome cost MORE than the customer committed to. There is
	// deliberately no path that turns that into a customer charge: the policy
	// only ever compares against the customer commitment, and an eligible
	// amount at or above it is simply not a benefit.
	for name, eligible := range map[string]int64{
		"exactly the commitment": committed,
		"above the commitment":   committed + 50_000,
	} {
		if _, err := fixture.service.ApplyCheaperFulfilmentBenefit(
			ctx, order.ID, eligible, cheaperPolicy,
		); !errors.Is(err, orders.ErrAdjustmentNotAvailable) {
			t.Errorf("%s: err=%v, want ErrAdjustmentNotAvailable", name, err)
		}
	}

	if authority.callCount() != callsBefore {
		t.Fatal("an expensive internal replan authorized additional money")
	}
	if len(authority.refundedFor()) != 0 {
		t.Fatal("an expensive internal replan moved money")
	}

	// The customer's approved total is exactly what it was.
	reread, err := fixture.service.GetForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.CustomerApprovedTotalMinor != order.CustomerApprovedTotalMinor {
		t.Fatalf("customer total moved from %d to %d",
			order.CustomerApprovedTotalMinor, reread.CustomerApprovedTotalMinor)
	}

	var adjustments int
	if err = fixture.env.pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_order_adjustments WHERE parent_order_id=$1`,
		order.ID).Scan(&adjustments); err != nil {
		t.Fatal(err)
	}
	if adjustments != 0 {
		t.Fatalf("an expensive replan recorded %d adjustments", adjustments)
	}
}

// A benefit larger than the leg it would be refunded from is refused rather
// than over-refunded.
func TestSection17FCheaper_BenefitCannotExceedTheRefundableLeg(t *testing.T) {
	fixture, order, authority := cheaperFixture(t)

	if _, err := fixture.service.ApplyCheaperFulfilmentBenefit(
		context.Background(), order.ID, -1, cheaperPolicy,
	); !errors.Is(err, orders.ErrAdjustmentNotAvailable) {
		t.Fatalf("err=%v, want ErrAdjustmentNotAvailable for a negative eligible amount", err)
	}
	if len(authority.refundedFor()) != 0 {
		t.Fatal("a negative eligible amount moved money")
	}
}

// A policy must be named. An unnamed benefit has no authority behind it.
func TestSection17FCheaper_RequiresAnExplicitPolicy(t *testing.T) {
	fixture, order, authority := cheaperFixture(t)
	eligible := committedDelivery(order) - 100

	if _, err := fixture.service.ApplyCheaperFulfilmentBenefit(
		context.Background(), order.ID, eligible, "   ",
	); !errors.Is(err, orders.ErrAdjustmentNotAvailable) {
		t.Fatalf("err=%v, want ErrAdjustmentNotAvailable", err)
	}
	if len(authority.refundedFor()) != 0 {
		t.Fatal("a policy-less benefit moved money")
	}
}

// -----------------------------------------------------------------------------
// G — multi-vendor isolation
// -----------------------------------------------------------------------------

func TestSection17FCheaper_AdjustmentDoesNotDisturbSiblingAllocations(t *testing.T) {
	fixture, order, authority := cheaperFixture(t)
	ctx := context.Background()

	before := map[string]orders.VendorOrder{}
	for _, vendorOrder := range order.VendorOrders {
		before[vendorOrder.ID] = vendorOrder
	}

	eligible := committedDelivery(order) - 100
	adjustment, err := fixture.service.ApplyCheaperFulfilmentBenefit(
		ctx, order.ID, eligible, cheaperPolicy,
	)
	if err != nil {
		t.Fatalf("ApplyCheaperFulfilmentBenefit: %v", err)
	}

	// The refund names exactly one vendor allocation's leg.
	refunds := authority.refundedFor()
	if len(refunds) != 1 {
		t.Fatalf("refunds=%d, want 1", len(refunds))
	}
	touched := refunds[0].SourceReference

	reread, err := fixture.service.GetForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, vendorOrder := range reread.VendorOrders {
		original := before[vendorOrder.ID]

		// No allocation's own money changed: the benefit is separate evidence.
		if vendorOrder.CustomerPayableMinor != original.CustomerPayableMinor ||
			vendorOrder.VendorPayableMinor != original.VendorPayableMinor ||
			vendorOrder.DeliveryCustomerMinor != original.DeliveryCustomerMinor {
			t.Errorf("%s allocation money changed: %+v -> %+v",
				vendorOrder.BusinessID, original, vendorOrder)
		}
		if vendorOrder.Status != original.Status {
			t.Errorf("%s status changed from %s to %s",
				vendorOrder.BusinessID, original.Status, vendorOrder.Status)
		}
	}

	// Only the named leg carries a refund; its siblings carry none.
	rows, err := fixture.env.pool.Query(ctx, `
		SELECT COALESCE(vendor_order_id,''),refunded_minor
		  FROM commerce_payment_legs WHERE parent_order_id=$1`, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	refundedLegs := 0
	for rows.Next() {
		var vendorOrderID string
		var refunded int64
		if err = rows.Scan(&vendorOrderID, &refunded); err != nil {
			t.Fatal(err)
		}
		if refunded > 0 {
			refundedLegs++
			if vendorOrderID != touched {
				t.Errorf("leg for %s was refunded, but the adjustment named %s",
					vendorOrderID, touched)
			}
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if refundedLegs != 1 {
		t.Fatalf("%d legs were refunded, want exactly 1", refundedLegs)
	}

	if adjustment.VendorOrderID != "" && adjustment.VendorOrderID != touched {
		t.Fatalf("adjustment names %s but the wallet refunded %s",
			adjustment.VendorOrderID, touched)
	}
}

// -----------------------------------------------------------------------------
// Regression: refunds are recorded on the leg, so the schema's own over-refund
// guard can see them.
//
// The defect this pins: a benefit refunded through Wallet updated only the
// adjustment row, leaving commerce_payment_legs.refunded_minor at zero. The
// schema enforces refunded + settled <= amount, but a guard cannot protect
// money it cannot see: two refund paths against the same leg — a cheaper
// replan and a commercial-change benefit — could each pass the check and
// together take the leg past its own authorized amount.
// -----------------------------------------------------------------------------

// A completed benefit must appear on the leg it was taken from, in the exact
// amount, or the next refund against that leg is unguarded.
func TestSection17FCheaper_RefundIsRecordedAgainstTheLegItCameFrom(t *testing.T) {
	fixture, order, authority := cheaperFixture(t)
	ctx := context.Background()

	const benefit = int64(100)

	if _, err := fixture.service.ApplyCheaperFulfilmentBenefit(
		ctx, order.ID, committedDelivery(order)-benefit, cheaperPolicy,
	); err != nil {
		t.Fatalf("ApplyCheaperFulfilmentBenefit: %v", err)
	}

	refunds := authority.refundedFor()
	if len(refunds) != 1 {
		t.Fatalf("the authority saw %d refunds, want 1", len(refunds))
	}
	if refunds[0].AmountKobo != benefit {
		t.Fatalf("Wallet was asked to refund %d kobo, want %d",
			refunds[0].AmountKobo, benefit)
	}

	// What Wallet did must also be what the database records, or the leg's own
	// over-refund guard is protecting a number that is not the truth.
	var recorded, legAmount int64
	if err := fixture.env.pool.QueryRow(ctx, `
		SELECT COALESCE(sum(refunded_minor),0), COALESCE(sum(amount_minor),0)
		  FROM commerce_payment_legs
		 WHERE parent_order_id = $1 AND refunded_minor > 0`,
		order.ID).Scan(&recorded, &legAmount); err != nil {
		t.Fatal(err)
	}

	if recorded != benefit {
		t.Fatalf("the leg records %d kobo refunded, want %d: a refund Wallet "+
			"performed is invisible to the database's own over-refund guard",
			recorded, benefit)
	}
	if recorded > legAmount {
		t.Fatalf("recorded refund %d exceeds the leg's own amount %d",
			recorded, legAmount)
	}
}

// The guard actually bites. A second refund that would take the leg past its
// authorized amount is refused by the database, not merely by Go.
func TestSection17FCheaper_ASecondRefundCannotTakeTheLegPastItsAmount(t *testing.T) {
	fixture, order, _ := cheaperFixture(t)
	ctx := context.Background()

	var legID, amount, refunded int64
	var id string
	if err := fixture.env.pool.QueryRow(ctx, `
		SELECT l.id, l.amount_minor, l.refunded_minor
		  FROM commerce_payment_legs l
		  JOIN commerce_payment_operations o ON o.id = l.payment_operation_id
		 WHERE o.parent_order_id = $1
		 ORDER BY l.amount_minor DESC LIMIT 1`,
		order.ID).Scan(&id, &amount, &refunded); err != nil {
		t.Fatal(err)
	}
	_ = legID

	// Refunding the whole leg is allowed exactly once.
	if _, err := fixture.env.pool.Exec(ctx, `
		UPDATE commerce_payment_legs
		   SET refunded_minor = refunded_minor + $2
		 WHERE id = $1 AND refunded_minor + $2 + settled_minor <= amount_minor`,
		id, amount-refunded); err != nil {
		t.Fatalf("refunding the leg in full was refused: %v", err)
	}

	// A second kobo has nowhere to come from. The CHECK constraint is the
	// last line of defence, and it must hold even against a direct write.
	_, err := fixture.env.pool.Exec(ctx, `
		UPDATE commerce_payment_legs SET refunded_minor = refunded_minor + 1
		 WHERE id = $1`, id)
	if err == nil {
		t.Fatal("the database allowed a leg to be refunded past its own amount")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "amounts_chk") &&
		!strings.Contains(strings.ToLower(err.Error()), "constraint") {
		t.Fatalf("the over-refund failed for the wrong reason: %v", err)
	}
}
