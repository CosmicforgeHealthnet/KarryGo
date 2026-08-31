package purchasing

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
)

// Section 15D operational certification: supplier performance honesty,
// overdue detection, alert idempotency, discrepancy follow-up and reversal.

// TestSection15D_InsufficientHistoryIsNotPerfectPerformance is the honesty
// gate. A supplier nobody has received from must not report a perfect record.
func TestSection15D_InsufficientHistoryIsNotPerfectPerformance(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, _ := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)

	perf, err := f.receiving.SupplierPerformance(
		ctx, f.actor, f.scope, supplier.ID, SupplierPerformanceQuery{})
	if err != nil {
		t.Fatal(err)
	}

	for _, metric := range []struct {
		name  string
		ratio Ratio
	}{
		{"on_time_rate", perf.OnTimeRate},
		{"short_delivery_rate", perf.ShortDeliveryRate},
		{"damage_rate", perf.DamageRate},
		{"rejection_rate", perf.RejectionRate},
		{"acceptance_rate", perf.AcceptanceRate},
		{"cost_variance_rate", perf.CostVarianceRate},
	} {
		if metric.ratio.Status != MetricInsufficientHistory {
			t.Errorf("%s reports status %q with no history; want %s",
				metric.name, metric.ratio.Status, MetricInsufficientHistory)
		}
		if metric.ratio.Value != nil {
			t.Errorf("%s reports a value of %v with no history; "+
				"absence of evidence must not read as a score",
				metric.name, *metric.ratio.Value)
		}
	}

	if perf.AverageActualLeadTime.Value != nil {
		t.Error("average lead time has a value before any delivery")
	}
	if perf.LatestObservedCost != nil {
		t.Error("a latest observed cost exists before any purchase was received")
	}
	if perf.AverageCostVariance.Status != MetricInsufficientHistory {
		t.Error("average cost variance claims a value with no observations")
	}
}

// TestSection15D_PerformanceReflectsRealEvidence proves the same metrics do
// appear once there is a real denominator, and that they carry their sample.
func TestSection15D_PerformanceReflectsRealEvidence(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)

	// Four receipts, one of them short, gives every rate a real denominator.
	for i, accepted := range []int64{10, 10, 10, 6} {
		po := f.orderedPO(t, supplier, product, 10, nil)

		line := InspectGoodsReceiptLineInput{
			DeliveredQuantity: accepted,
			AcceptedQuantity:  accepted,
		}
		if accepted < 10 {
			line.ShortQuantity = 10 - accepted
		}

		f.receiveOnce(t, po, line)
		_ = i
	}

	perf, err := f.receiving.SupplierPerformance(
		ctx, f.actor, f.scope, supplier.ID, SupplierPerformanceQuery{})
	if err != nil {
		t.Fatal(err)
	}

	if perf.TotalPostedGoodsReceipt != 4 {
		t.Fatalf("posted receipts = %d, want 4", perf.TotalPostedGoodsReceipt)
	}
	if perf.TotalAcceptedQuantity != 36 {
		t.Errorf("accepted quantity = %d, want 36", perf.TotalAcceptedQuantity)
	}

	if perf.ShortDeliveryRate.Status != MetricOK {
		t.Fatalf("short delivery rate is %q with four receipts; want OK",
			perf.ShortDeliveryRate.Status)
	}
	if perf.ShortDeliveryRate.Value == nil || *perf.ShortDeliveryRate.Value != 0.25 {
		t.Errorf("short delivery rate = %v, want 0.25 (one of four lines short)",
			perf.ShortDeliveryRate.Value)
	}
	if perf.ShortDeliveryRate.SampleCount != 4 {
		t.Errorf("short delivery sample = %d, want 4", perf.ShortDeliveryRate.SampleCount)
	}

	if perf.ObservedFrom == nil || perf.ObservedTo == nil {
		t.Error("a supplier with history reports no observation window")
	}
	if perf.LatestObservedCost == nil || *perf.LatestObservedCost != 50000 {
		t.Errorf("latest observed cost = %v, want 50000", perf.LatestObservedCost)
	}
}

// TestSection15D_OverdueOnlyAppliesToOutstandingDelivery is the correctness
// gate for overdue: a delivered or cancelled order is never overdue, however
// old its expected date.
func TestSection15D_OverdueOnlyAppliesToOutstandingDelivery(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	past := time.Now().UTC().AddDate(0, 0, -10)

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)

	// One order still outstanding, one fully received. Both are long past due.
	outstanding := f.orderedPO(t, supplier, product, 10, &past)
	delivered := f.orderedPO(t, supplier, product, 5, &past)

	f.receiveOnce(t, delivered, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 5,
		AcceptedQuantity:  5,
	})

	overdue, err := f.receiving.ListOutstandingDeliveries(
		ctx, f.actor, f.scope, DeliveryOverdue, nil, 50)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, delivery := range overdue {
		seen[delivery.PurchaseOrderID] = true
	}

	if !seen[outstanding.ID] {
		t.Error("an order still owing delivery is missing from the overdue view")
	}
	if seen[delivered.ID] {
		t.Error("a fully received order is still reported as an overdue delivery")
	}

	board, err := f.receiving.PurchasingDashboard(ctx, f.actor, f.scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	if board.Deliveries.Overdue != 1 {
		t.Errorf("dashboard overdue count = %d, want 1", board.Deliveries.Overdue)
	}
	if board.Deliveries.MateriallyOverdue != 1 {
		t.Errorf("dashboard materially-overdue count = %d, want 1 (10 days late)",
			board.Deliveries.MateriallyOverdue)
	}
}

// TestSection15D_DeliveryAlertsAreIdempotent proves the sweep can run
// repeatedly without spamming, and that alerts close when the order moves on.
func TestSection15D_DeliveryAlertsAreIdempotent(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	past := time.Now().UTC().AddDate(0, 0, -3)

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 10, &past)

	first, err := f.receiving.SweepDeliveryAlerts(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if first == 0 {
		t.Fatal("the first sweep raised no alert for an overdue order")
	}

	// Repeated sweeps in the same window must raise nothing further.
	for pass := 0; pass < 3; pass++ {
		again, err := f.receiving.SweepDeliveryAlerts(ctx, 100)
		if err != nil {
			t.Fatal(err)
		}
		if again != 0 {
			t.Fatalf("sweep pass %d raised %d duplicate alerts", pass+2, again)
		}
	}

	alerts, err := f.receiving.ListPurchasingAlerts(
		ctx, f.actor, f.scope, AlertStatusOpen, nil, 50)
	if err != nil {
		t.Fatal(err)
	}

	found := 0
	for _, alert := range alerts {
		if alert.PurchaseOrderID == po.ID {
			found++
			if alert.AlertType != AlertPOOverdue {
				t.Errorf("alert type = %q, want %s", alert.AlertType, AlertPOOverdue)
			}
		}
	}
	if found != 1 {
		t.Fatalf("%d open alerts exist for one overdue order; want exactly 1", found)
	}

	// Once the order is fully received, the alert must stop being open.
	f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 10,
		AcceptedQuantity:  10,
	})

	if _, err := f.receiving.SweepDeliveryAlerts(ctx, 100); err != nil {
		t.Fatal(err)
	}

	open, err := f.receiving.ListPurchasingAlerts(
		ctx, f.actor, f.scope, AlertStatusOpen, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, alert := range open {
		if alert.PurchaseOrderID == po.ID {
			t.Fatal("a delivered order still has an open overdue alert")
		}
	}
}

// TestSection15D_ConcurrentSweepsRaiseOneAlert proves alert idempotency holds
// under a real race, not just under sequential passes.
func TestSection15D_ConcurrentSweepsRaiseOneAlert(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	past := time.Now().UTC().AddDate(0, 0, -2)

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 10, &past)

	const racers = 5

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _ = f.receiving.SweepDeliveryAlerts(ctx, 100)
		}()
	}

	close(start)
	wg.Wait()

	var alerts int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_purchasing_alerts
		  WHERE business_id = $1 AND purchase_order_id = $2`,
		f.businessID, po.ID).Scan(&alerts); err != nil {
		t.Fatal(err)
	}

	if alerts != 1 {
		t.Fatalf("%d concurrent sweeps produced %d alerts; want exactly 1",
			racers, alerts)
	}
}

// TestSection15D_DiscrepancyEscalationDoesNotMutateEvidence proves ageing
// raises an operational signal and leaves the commercial record alone.
func TestSection15D_DiscrepancyEscalationDoesNotMutateEvidence(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 10, nil)

	f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 7,
		AcceptedQuantity:  7,
		ShortQuantity:     3,
	})

	var discrepancyID, statusBefore string
	var versionBefore int64
	if err := f.pool.QueryRow(ctx,
		`SELECT id, status, version FROM vendor_supplier_discrepancies
		  WHERE business_id = $1 AND purchase_order_id = $2 LIMIT 1`,
		f.businessID, po.ID).Scan(&discrepancyID, &statusBefore, &versionBefore); err != nil {
		t.Fatal(err)
	}

	// Age it past the escalation threshold.
	if _, err := f.pool.Exec(ctx,
		`UPDATE vendor_supplier_discrepancies
		    SET created_at = now() - interval '10 days'
		  WHERE id = $1`, discrepancyID); err != nil {
		t.Fatal(err)
	}

	raised, err := f.receiving.SweepDiscrepancyEscalations(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if raised == 0 {
		t.Fatal("a ten-day-old unresolved discrepancy raised no escalation")
	}

	// The discrepancy itself must be untouched.
	var statusAfter string
	var versionAfter int64
	if err := f.pool.QueryRow(ctx,
		`SELECT status, version FROM vendor_supplier_discrepancies WHERE id = $1`,
		discrepancyID).Scan(&statusAfter, &versionAfter); err != nil {
		t.Fatal(err)
	}
	if statusAfter != statusBefore || versionAfter != versionBefore {
		t.Fatalf("escalation mutated the discrepancy: status %s->%s version %d->%d",
			statusBefore, statusAfter, versionBefore, versionAfter)
	}

	// Repeat sweeps must not duplicate the escalation.
	if again, err := f.receiving.SweepDiscrepancyEscalations(ctx, 100); err != nil {
		t.Fatal(err)
	} else if again != 0 {
		t.Fatalf("a repeated escalation sweep raised %d duplicate alerts", again)
	}

	var escalations int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_purchasing_alerts
		  WHERE business_id = $1 AND discrepancy_id = $2
		    AND alert_type = $3`,
		f.businessID, discrepancyID, AlertDiscrepancyEscalate).Scan(&escalations); err != nil {
		t.Fatal(err)
	}
	if escalations != 1 {
		t.Fatalf("%d escalation alerts exist for one discrepancy; want 1", escalations)
	}
}

// TestSection15D_ReversalRecordsEvidenceRatherThanDeleting proves a safe
// reversal writes inverse evidence and never erases history.
func TestSection15D_ReversalRecordsEvidenceRatherThanDeleting(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 10, nil)

	before := f.balanceOf(t, item)

	receipt, _ := f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 10,
		AcceptedQuantity:  10,
	})

	if delta := f.balanceOf(t, item) - before; delta != 10 {
		t.Fatalf("posted %d units, want 10", delta)
	}

	var movementsBefore, costBefore int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM inventory_movements
		  WHERE business_id = $1 AND inventory_item_id = $2`,
		f.businessID, item).Scan(&movementsBefore); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_purchase_cost_history
		  WHERE business_id = $1 AND goods_receipt_id = $2`,
		f.businessID, receipt.ID).Scan(&costBefore); err != nil {
		t.Fatal(err)
	}

	posted, err := f.receiving.GetGoodsReceipt(ctx, f.receivingActor(), receipt.ID)
	if err != nil {
		t.Fatal(err)
	}

	reversed, err := f.receiving.ReverseGoodsReceipt(
		ctx, f.receivingActor(), posted.ID, posted.Version, "supplier recalled the delivery")
	if err != nil {
		t.Fatalf("a safe reversal was refused: %v", err)
	}
	if reversed.Status != "REVERSED" {
		t.Fatalf("receipt status = %q, want REVERSED", reversed.Status)
	}

	if after := f.balanceOf(t, item) - before; after != 0 {
		t.Errorf("stock after reversal = %d, want 0", after)
	}

	// The original movement must still exist; reversal appends, never deletes.
	var movementsAfter, costAfter int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM inventory_movements
		  WHERE business_id = $1 AND inventory_item_id = $2`,
		f.businessID, item).Scan(&movementsAfter); err != nil {
		t.Fatal(err)
	}
	if movementsAfter <= movementsBefore {
		t.Errorf("movements went from %d to %d; a reversal must append inverse evidence",
			movementsBefore, movementsAfter)
	}

	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_purchase_cost_history
		  WHERE business_id = $1 AND goods_receipt_id = $2`,
		f.businessID, receipt.ID).Scan(&costAfter); err != nil {
		t.Fatal(err)
	}
	if costAfter < costBefore {
		t.Errorf("purchase cost evidence shrank from %d to %d rows on reversal",
			costBefore, costAfter)
	}
}

// TestSection15D_ReversalIsRefusedOnceStockMoved proves a receipt whose stock
// has been consumed cannot be reversed, because the attribution is no longer
// safe.
func TestSection15D_ReversalIsRefusedOnceStockMoved(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 10, nil)

	receipt, _ := f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 10,
		AcceptedQuantity:  10,
	})

	// Consume most of the received stock through generic Inventory.
	invScope, err := inventoryBranchScope(f.businessID, f.branchID)
	if err != nil {
		t.Fatal(err)
	}

	// Reserve the received stock. This is one of the documented unsafe cases:
	// the quantity is now committed to something else, so reversing the receipt
	// would either drive stock negative or strand the reservation.
	if _, err := f.stock.CreateReservation(ctx,
		inventoryActorFor(f.staffID, f.businessID), invScope,
		inventory.CreateReservationInput{
			BranchID:   f.branchID,
			SourceType: "ORDER",
			SourceRef:  "s15d-hold-" + uuid.NewString(),
			Lines: []inventory.ReservationLineInput{
				{InventoryItemID: item, Quantity: 10},
			},
		}); err != nil {
		t.Fatalf("could not reserve received stock: %v", err)
	}

	posted, err := f.receiving.GetGoodsReceipt(ctx, f.receivingActor(), receipt.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.receiving.ReverseGoodsReceipt(
		ctx, f.receivingActor(), posted.ID, posted.Version, "late reversal"); err == nil {
		t.Fatal("a receipt was reversed after its stock had been consumed; " +
			"the reversal would have driven stock negative or misattributed it")
	}
}
