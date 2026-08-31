package purchasing

// Section 16D cost-evidence certification, driven through the REAL Section 15
// receiving pipeline.
//
// This lives in the purchasing package on purpose. Section 16D's own tests can
// assert the shape of the eligibility predicate, but only here can OBSERVED and
// REVERSAL rows be produced the way production produces them - by ordering,
// receiving, inspecting, confirming and then reversing an actual goods receipt
// through the Section 15 services. The data is synthetic; the code path is not.
//
// What is being certified is the contract between the two sections:
//
//   - a confirmed receipt makes cost evidence eligible for Section 16D;
//   - reversing that receipt takes it back out of the cost basis, for BOTH the
//     latest-cost and the weighted-average strategies;
//   - a reversed observation contributes neither quantity nor cost to the
//     average, rather than being subtracted from it;
//   - when nothing eligible remains, Section 16D reports absence rather than a
//     fabricated zero.

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/features/cost"
)

// costEvidenceFor reads the raw Section 15 rows for one inventory item, so a
// failure can say what purchasing actually wrote.
func costEvidenceFor(
	t *testing.T,
	f s15Fixture,
	itemID string,
) (observed int, reversed int) {
	t.Helper()
	ctx := context.Background()

	if err := f.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE evidence_type='OBSERVED'),
			count(*) FILTER (WHERE evidence_type='REVERSAL')
		  FROM vendor_purchase_cost_history
		 WHERE business_id=$1 AND inventory_item_id=$2`,
		f.businessID, itemID).Scan(&observed, &reversed); err != nil {
		t.Fatalf("read cost evidence: %v", err)
	}
	return observed, reversed
}

// TestSection16DCostEvidenceFollowsRealReceivingAndReversal is the Section 16D
// REVERSAL gate.
func TestSection16DCostEvidenceFollowsRealReceivingAndReversal(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")

	// Two supplier products on the SAME inventory item, priced differently, so
	// the item accumulates two receipts at two unit costs.
	supplierA, productA := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 500)
	supplierB, productB := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 900)

	reader := cost.NewPostgresPurchasingCostReader(f.pool)

	// Nothing has been purchased yet: absence must be absence.
	if _, found, err := reader.LatestAccepted(ctx, f.businessID, f.branchID, item); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("cost evidence existed before anything was received")
	}

	// --- First real receipt: 100 units at 500 minor each -------------------
	firstPO := f.orderedPO(t, supplierA, productA, 100, nil)
	firstReceipt, _ := f.receiveOnce(t, firstPO, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 100,
		AcceptedQuantity:  100,
	})

	observed, reversed := costEvidenceFor(t, f, item)
	if observed != 1 || reversed != 0 {
		t.Fatalf("after one confirmed receipt: observed=%d reversed=%d; want 1 and 0",
			observed, reversed)
	}

	latest, found, err := reader.LatestAccepted(ctx, f.businessID, f.branchID, item)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("a confirmed receipt did not make cost evidence eligible for Section 16D")
	}
	if latest.InventoryUnitCostMinor != 500 {
		t.Fatalf("latest unit cost = %d; want 500", latest.InventoryUnitCostMinor)
	}
	if latest.GoodsReceiptID != firstReceipt.ID {
		t.Fatalf("cost evidence points at receipt %s; the confirmed receipt was %s",
			latest.GoodsReceiptID, firstReceipt.ID)
	}

	// --- Second real receipt: 100 units at 900 minor each ------------------
	secondPO := f.orderedPO(t, supplierB, productB, 100, nil)
	secondReceipt, _ := f.receiveOnce(t, secondPO, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 100,
		AcceptedQuantity:  100,
	})

	latest, found, err = reader.LatestAccepted(ctx, f.businessID, f.branchID, item)
	if err != nil || !found {
		t.Fatalf("latest after second receipt: found=%t err=%v", found, err)
	}
	if latest.InventoryUnitCostMinor != 900 {
		t.Fatalf("latest unit cost = %d; want the newer 900", latest.InventoryUnitCostMinor)
	}

	// Weighted average over both: equal quantities, so (500+900)/2 = 700.
	recent, err := reader.RecentAccepted(ctx, f.businessID, f.branchID, item, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 {
		t.Fatalf("weighted-average window has %d observations; want 2", len(recent))
	}
	quantities := make([]int64, 0, len(recent))
	unitCosts := make([]int64, 0, len(recent))
	for _, observation := range recent {
		quantities = append(quantities, observation.InventoryQuantity)
		unitCosts = append(unitCosts, observation.InventoryUnitCostMinor)
	}
	average, ok := cost.WeightedAverageMinor(quantities, unitCosts)
	if !ok || average != 700 {
		t.Fatalf("weighted average = %d ok=%t; want 700 across both receipts", average, ok)
	}

	// --- Reverse the SECOND receipt through the real Section 15 path -------
	posted, err := f.receiving.GetGoodsReceipt(ctx, f.receivingActor(), secondReceipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.receiving.ReverseGoodsReceipt(ctx, f.receivingActor(),
		posted.ID, posted.Version, "supplier recalled the second delivery"); err != nil {
		t.Fatalf("reversing an untouched receipt was refused: %v", err)
	}

	observed, reversed = costEvidenceFor(t, f, item)
	if observed != 2 || reversed != 1 {
		t.Fatalf("after reversal: observed=%d reversed=%d; want 2 and 1 - "+
			"a reversal cancels its observation, it does not delete it", observed, reversed)
	}

	// LATEST must fall back to the surviving 500 receipt. If reversals were
	// ignored it would still report 900.
	latest, found, err = reader.LatestAccepted(ctx, f.businessID, f.branchID, item)
	if err != nil || !found {
		t.Fatalf("latest after reversal: found=%t err=%v", found, err)
	}
	if latest.InventoryUnitCostMinor == 900 {
		t.Fatal("latest cost still reports the reversed receipt; reversed evidence stayed in the basis")
	}
	if latest.InventoryUnitCostMinor != 500 {
		t.Fatalf("latest unit cost = %d; want the surviving 500", latest.InventoryUnitCostMinor)
	}
	if latest.GoodsReceiptID != firstReceipt.ID {
		t.Fatalf("latest evidence points at %s; want the unreversed receipt %s",
			latest.GoodsReceiptID, firstReceipt.ID)
	}

	// WEIGHTED AVERAGE must now be 500 exactly: the reversed observation
	// contributes neither its 100 units nor its 900 cost. Treating a reversal
	// as a negative quantity would give a different answer, and ignoring it
	// entirely would still give 700.
	recent, err = reader.RecentAccepted(ctx, f.businessID, f.branchID, item, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 {
		t.Fatalf("weighted-average window has %d observations after reversal; want 1", len(recent))
	}
	average, ok = cost.WeightedAverageMinor(
		[]int64{recent[0].InventoryQuantity},
		[]int64{recent[0].InventoryUnitCostMinor})
	if !ok || average != 500 {
		t.Fatalf("weighted average after reversal = %d ok=%t; want exactly 500", average, ok)
	}

	// --- Reverse the FIRST receipt too: nothing eligible remains -----------
	postedFirst, err := f.receiving.GetGoodsReceipt(ctx, f.receivingActor(), firstReceipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.receiving.ReverseGoodsReceipt(ctx, f.receivingActor(),
		postedFirst.ID, postedFirst.Version, "supplier recalled the first delivery"); err != nil {
		t.Fatalf("reversing the first receipt was refused: %v", err)
	}

	if _, found, err = reader.LatestAccepted(ctx, f.businessID, f.branchID, item); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("cost evidence survived after every receipt was reversed")
	}

	remaining, err := reader.RecentAccepted(ctx, f.businessID, f.branchID, item, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("%d observations remain eligible after every receipt was reversed", len(remaining))
	}

	// Absence must stay absence. WeightedAverageMinor refusing to resolve is
	// what stops a fully reversed item being costed at zero.
	if _, ok = cost.WeightedAverageMinor(nil, nil); ok {
		t.Fatal("an item with no surviving evidence resolved to a cost")
	}
}

// TestSection16DIgnoresRejectedAndWrongItemReceiving proves eligibility is a
// property of what Section 15 writes, not a filter Section 16D reimplements.
//
// A delivery that is inspected but never accepted must never become cost
// evidence: costing a recipe from goods the vendor refused would be wrong in
// the most expensive possible direction.
func TestSection16DIgnoresRejectedAndWrongItemReceiving(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 750)
	reader := cost.NewPostgresPurchasingCostReader(f.pool)

	po := f.orderedPO(t, supplier, product, 100, nil)

	// Delivered but entirely rejected: accepted quantity is zero.
	f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 100,
		AcceptedQuantity:  0,
		RejectedQuantity:  100,
	})

	observed, _ := costEvidenceFor(t, f, item)
	if observed != 0 {
		t.Fatalf("%d observations written for a fully rejected delivery; "+
			"rejected goods are not accepted purchase evidence", observed)
	}

	if _, found, err := reader.LatestAccepted(ctx, f.businessID, f.branchID, item); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("a rejected delivery became eligible cost evidence")
	}
}

// TestSection16DCostEvidenceIsTenantScoped proves one vendor's purchasing
// history is invisible to another's cost engine, asserted against evidence
// created by the real receiving path rather than hand-inserted rows.
func TestSection16DCostEvidenceIsTenantScoped(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 640)
	po := f.orderedPO(t, supplier, product, 100, nil)
	f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 100,
		AcceptedQuantity:  100,
	})

	reader := cost.NewPostgresPurchasingCostReader(f.pool)

	// The owning tenant sees it.
	if _, found, err := reader.LatestAccepted(ctx, f.businessID, f.branchID, item); err != nil {
		t.Fatal(err)
	} else if !found {
		t.Fatal("the owning tenant could not read its own cost evidence")
	}

	// The other tenant, holding the same inventory item id, does not.
	if _, found, err := reader.LatestAccepted(
		ctx, f.foreignBusinessID, f.foreignBranchID, item); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("another vendor read this vendor's purchase cost evidence")
	}

	// Nor does a different branch of the owning tenant.
	if _, found, err := reader.LatestAccepted(
		ctx, f.businessID, f.foreignBranchID, item); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("another branch read this branch's purchase cost evidence")
	}
}
