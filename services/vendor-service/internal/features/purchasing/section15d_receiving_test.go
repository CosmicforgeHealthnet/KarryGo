package purchasing

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Section 15D goods-receiving certification against real PostgreSQL.
//
// The single most important property in Section 15 is that Inventory receives
// exactly the accepted quantity, exactly once. Everything else in purchasing
// is paperwork; this is the part that changes what a vendor can sell.

// TestSection15D_CanonicalDiscrepancyExample is the worked example from the
// specification, run end to end against the database.
//
//	Ordered   100
//	Delivered  98
//	Accepted   90
//	Damaged     5
//	Rejected    3
//	Short       2
//
// Inventory must receive +90. Not 98, not 100.
func TestSection15D_CanonicalDiscrepancyExample(t *testing.T) {
	f := newS15Fixture(t)

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 100, nil)

	before := f.balanceOf(t, item)

	_, result := f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 98,
		AcceptedQuantity:  90,
		DamagedQuantity:   5,
		RejectedQuantity:  3,
		ShortQuantity:     2,
	})

	after := f.balanceOf(t, item)
	delta := after - before

	if delta != 90 {
		t.Fatalf("Inventory delta = %d, want exactly 90 "+
			"(delivered 98, accepted 90, damaged 5, rejected 3, short 2)", delta)
	}
	if delta == 98 {
		t.Fatal("damaged and rejected goods entered sellable stock")
	}
	if delta == 100 {
		t.Fatal("the ordered quantity was posted instead of the accepted quantity")
	}

	if result.Receipt.Status != "POSTED" {
		t.Fatalf("receipt status = %q, want POSTED", result.Receipt.Status)
	}

	// Every non-accepted outcome must leave evidence rather than vanish.
	counts := map[string]int{}
	rows, err := f.pool.Query(context.Background(),
		`SELECT discrepancy_type, count(*)
		   FROM vendor_supplier_discrepancies
		  WHERE business_id = $1 AND purchase_order_id = $2
		  GROUP BY discrepancy_type`,
		f.businessID, po.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		counts[kind] = n
	}
	rows.Close()

	for _, expected := range []string{"SHORT_DELIVERY", "DAMAGED", "REJECTED"} {
		if counts[expected] == 0 {
			t.Errorf("no %s discrepancy was raised; the loss has no evidence", expected)
		}
	}
}

// TestSection15D_PartialReceivingAcrossThreeReceipts walks the specification's
// partial-receiving example and checks the purchase order only reaches
// RECEIVED when the last unit arrives.
func TestSection15D_PartialReceivingAcrossThreeReceipts(t *testing.T) {
	f := newS15Fixture(t)

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 100, nil)

	before := f.balanceOf(t, item)

	for _, step := range []struct {
		quantity   int64
		cumulative int64
		wantStatus string
	}{
		{40, 40, "PARTIALLY_RECEIVED"},
		{50, 90, "PARTIALLY_RECEIVED"},
		{10, 100, "RECEIVED"},
	} {
		f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
			DeliveredQuantity: step.quantity,
			AcceptedQuantity:  step.quantity,
		})

		if got := f.balanceOf(t, item) - before; got != step.cumulative {
			t.Fatalf("after receiving %d, Inventory total = %d, want %d",
				step.quantity, got, step.cumulative)
		}

		reloaded, err := f.orders.GetPurchaseOrder(
			context.Background(), f.actor, po.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.Status != step.wantStatus {
			t.Fatalf("after receiving %d of 100, purchase order is %q, want %q",
				step.cumulative, reloaded.Status, step.wantStatus)
		}
		po = reloaded
	}
}

// TestSection15D_RejectedAndDamagedNeverEnterStock isolates the two outcomes
// that must never become sellable, with nothing accepted at all.
func TestSection15D_RejectedAndDamagedNeverEnterStock(t *testing.T) {
	f := newS15Fixture(t)

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 20, nil)

	before := f.balanceOf(t, item)

	f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 20,
		AcceptedQuantity:  0,
		DamagedQuantity:   12,
		RejectedQuantity:  8,
	})

	if delta := f.balanceOf(t, item) - before; delta != 0 {
		t.Fatalf("Inventory moved by %d on a delivery where nothing was accepted", delta)
	}
}

// TestSection15D_ExactPurchaseUnitConversion proves accepted purchase units are
// converted to inventory units exactly, with no rounding.
func TestSection15D_ExactPurchaseUnitConversion(t *testing.T) {
	f := newS15Fixture(t)

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")

	// One purchase unit is a case of 24.
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 24, 120000)
	po := f.orderedPO(t, supplier, product, 10, nil)

	before := f.balanceOf(t, item)

	f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 7,
		AcceptedQuantity:  7,
		ShortQuantity:     3,
	})

	if delta := f.balanceOf(t, item) - before; delta != 7*24 {
		t.Fatalf("Inventory delta = %d, want %d (7 cases of 24)", delta, 7*24)
	}
}

// TestSection15D_ConfirmIsExactlyOnceUnderConcurrency is the hard concurrency
// gate. Two goroutines confirm the same receipt at the same time; exactly one
// must post, and the database must show one posting, not two.
//
// Exactly-once here means the stock is right, not merely that one caller saw
// an error.
func TestSection15D_ConfirmIsExactlyOnceUnderConcurrency(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 50, nil)

	receipt, err := f.receiving.CreateGoodsReceipt(ctx, f.receivingActor(),
		CreateGoodsReceiptInput{PurchaseOrderID: po.ID, ReceivedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}

	inspected, err := f.receiving.InspectGoodsReceipt(ctx, f.receivingActor(),
		receipt.ID, receipt.Version,
		InspectGoodsReceiptInput{Lines: []InspectGoodsReceiptLineInput{{
			PurchaseOrderLineID: po.Lines[0].ID,
			DeliveredQuantity:   50,
			AcceptedQuantity:    50,
		}}})
	if err != nil {
		t.Fatal(err)
	}

	before := f.balanceOf(t, item)

	const racers = 6

	var wg sync.WaitGroup
	start := make(chan struct{})
	successes := make(chan bool, racers)

	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			// Each racer is a distinct command, so a shared idempotency key is
			// not what prevents the double post. The database is.
			actor := f.receivingActor()
			_, err := f.receiving.ConfirmGoodsReceipt(
				ctx, actor, inspected.ID, inspected.Version)
			successes <- err == nil
		}()
	}

	close(start)
	wg.Wait()
	close(successes)

	won := 0
	for ok := range successes {
		if ok {
			won++
		}
	}

	if delta := f.balanceOf(t, item) - before; delta != 50 {
		t.Fatalf("%d concurrent confirmations posted %d units; want exactly 50",
			racers, delta)
	}

	if won != 1 {
		t.Errorf("%d confirmations reported success; want exactly 1", won)
	}

	// One generic Inventory receipt, one movement per accepted line, one cost
	// observation. Anything more means the posting ran twice.
	var inventoryReceipts, movements, costRows int

	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM inventory_receipts
		  WHERE business_id = $1 AND source_ref LIKE 'PURCHASING:%' || $2`,
		f.businessID, inspected.ID).Scan(&inventoryReceipts); err != nil {
		t.Fatal(err)
	}
	if inventoryReceipts != 1 {
		t.Errorf("%d generic Inventory receipts exist for one goods receipt; want 1",
			inventoryReceipts)
	}

	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM inventory_movements
		  WHERE business_id = $1 AND inventory_item_id = $2
		    AND movement_type = 'RECEIVE'`,
		f.businessID, item).Scan(&movements); err != nil {
		t.Fatal(err)
	}
	if movements != 1 {
		t.Errorf("%d RECEIVE movements were appended; want exactly 1", movements)
	}

	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_purchase_cost_history
		  WHERE business_id = $1 AND goods_receipt_id = $2`,
		f.businessID, inspected.ID).Scan(&costRows); err != nil {
		t.Fatal(err)
	}
	if costRows != 1 {
		t.Errorf("%d purchase-cost observations were recorded; want exactly 1", costRows)
	}

	// Receiving progress must also advance exactly once.
	reloaded, err := f.orders.GetPurchaseOrder(ctx, f.actor, po.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != "RECEIVED" {
		t.Errorf("purchase order status = %q, want RECEIVED", reloaded.Status)
	}
}

// TestSection15D_LotAndExpiryPostThrough proves lot and expiry metadata on an
// accepted line reaches the Inventory lot, so FEFO can see it later.
func TestSection15D_LotAndExpiryPostThrough(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "EXPIRY")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 30, nil)

	expiry := time.Now().UTC().Add(90 * 24 * time.Hour)
	lot := "LOT-" + uuid.NewString()[:8]

	f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 30,
		AcceptedQuantity:  30,
		LotNumber:         lot,
		ExpiresAt:         &expiry,
	})

	var stored int64
	var storedExpiry *time.Time

	if err := f.pool.QueryRow(ctx,
		`SELECT on_hand_quantity, expires_at
		   FROM inventory_lots
		  WHERE business_id = $1 AND inventory_item_id = $2`,
		f.businessID, item).Scan(&stored, &storedExpiry); err != nil {
		t.Fatalf("no Inventory lot was created for an expiry-tracked receipt: %v", err)
	}

	if stored != 30 {
		t.Errorf("lot holds %d units, want 30", stored)
	}
	if storedExpiry == nil {
		t.Fatal("the lot has no expiry date, so FEFO cannot order it")
	}
	// Compare instants, not wall-clock representations: PostgreSQL returns the
	// value in the session time zone, which is the same moment in a different
	// location.
	if storedExpiry.UTC().Truncate(time.Hour) != expiry.UTC().Truncate(time.Hour) {
		t.Errorf("lot expiry = %v, want %v", storedExpiry.UTC(), expiry.UTC())
	}
}

// TestSection15D_CostVarianceLeavesTheApprovedOrderAlone is the commercial
// gate: an invoice above the approved price creates evidence, and does not
// silently rewrite what was approved.
func TestSection15D_CostVarianceLeavesTheApprovedOrderAlone(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")

	// Approved at 50,000 minor per unit.
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 1, nil)

	approvedTotal := po.TotalAmountMinor

	// Supplier invoices 54,000.
	actual := int64(54000)

	f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity:   1,
		AcceptedQuantity:    1,
		ActualUnitCostMinor: &actual,
	})

	reloaded, err := f.orders.GetPurchaseOrder(ctx, f.actor, po.ID)
	if err != nil {
		t.Fatal(err)
	}

	if reloaded.TotalAmountMinor != approvedTotal {
		t.Fatalf("the approved purchase order total changed from %d to %d; "+
			"an invoice must never rewrite an approved commitment",
			approvedTotal, reloaded.TotalAmountMinor)
	}

	var variances int
	var expected, actualRecorded, variance int64

	if err := f.pool.QueryRow(ctx,
		`SELECT count(*),
		        coalesce(max(expected_cost_minor),0),
		        coalesce(max(actual_cost_minor),0),
		        coalesce(max(variance_minor),0)
		   FROM vendor_supplier_discrepancies
		  WHERE business_id = $1
		    AND purchase_order_id = $2
		    AND discrepancy_type = 'COST_VARIANCE'`,
		f.businessID, po.ID).Scan(&variances, &expected, &actualRecorded, &variance); err != nil {
		t.Fatal(err)
	}

	if variances == 0 {
		t.Fatal("an invoice above the approved cost raised no COST_VARIANCE evidence")
	}
	if expected != 50000 || actualRecorded != 54000 {
		t.Errorf("variance evidence records expected=%d actual=%d, want 50000/54000",
			expected, actualRecorded)
	}
	if variance != 4000 {
		t.Errorf("variance amount = %d, want 4000", variance)
	}

	// The immutable cost record must carry what was actually paid.
	var observed int64
	if err := f.pool.QueryRow(ctx,
		`SELECT purchase_unit_cost_minor
		   FROM vendor_purchase_cost_history
		  WHERE business_id = $1 AND purchase_order_id = $2`,
		f.businessID, po.ID).Scan(&observed); err != nil {
		t.Fatal(err)
	}
	if observed != 54000 {
		t.Errorf("purchase cost evidence records %d, want the actual 54000", observed)
	}
}

// TestSection15D_PurchaseCostHistoryIsAppendOnly proves the commercial record
// cannot be rewritten, at the database level.
func TestSection15D_PurchaseCostHistoryIsAppendOnly(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 5, nil)

	f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 5,
		AcceptedQuantity:  5,
	})

	var costID string
	if err := f.pool.QueryRow(ctx,
		`SELECT id FROM vendor_purchase_cost_history
		  WHERE business_id = $1 AND purchase_order_id = $2 LIMIT 1`,
		f.businessID, po.ID).Scan(&costID); err != nil {
		t.Fatal(err)
	}

	if _, err := f.pool.Exec(ctx,
		`UPDATE vendor_purchase_cost_history
		    SET purchase_unit_cost_minor = 1
		  WHERE id = $1`, costID); err == nil {
		t.Error("purchase cost history accepted an UPDATE; it must be append-only")
	}

	if _, err := f.pool.Exec(ctx,
		`DELETE FROM vendor_purchase_cost_history WHERE id = $1`, costID); err == nil {
		t.Error("purchase cost history accepted a DELETE; it must be append-only")
	}

	var still int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_purchase_cost_history WHERE id = $1`,
		costID).Scan(&still); err != nil {
		t.Fatal(err)
	}
	if still != 1 {
		t.Fatal("the cost observation did not survive the tamper attempts")
	}
}
