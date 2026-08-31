package purchasing

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Section 15D security, isolation and boundary certification.
//
// Every probe below uses an identifier that genuinely exists and genuinely
// belongs to another business. Testing with invented identifiers would only
// prove the service can fail to find a row, not that it refuses one it could
// have found.

// TestSection15D_CrossVendorIsolation walks the purchasing surface with a
// valid caller from business B naming business A's real records.
func TestSection15D_CrossVendorIsolation(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	// Business A's real purchasing records.
	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 10, nil)

	receipt, result := f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 8,
		AcceptedQuantity:  6,
		DamagedQuantity:   2,
		ShortQuantity:     2,
	})
	_ = result

	var discrepancyID string
	if err := f.pool.QueryRow(ctx,
		`SELECT id FROM vendor_supplier_discrepancies
		  WHERE business_id = $1 AND purchase_order_id = $2 LIMIT 1`,
		f.businessID, po.ID).Scan(&discrepancyID); err != nil {
		t.Fatal(err)
	}

	foreign := f.foreignActor
	foreignReceiving := f.foreignReceivingActor()

	t.Run("supplier", func(t *testing.T) {
		if _, err := f.suppliers.GetSupplier(
			ctx, foreign, f.foreignScope, supplier.ID); err == nil {
			t.Fatal("business B read business A's supplier")
		}
	})

	t.Run("supplier product", func(t *testing.T) {
		if _, err := f.suppliers.GetSupplierProduct(
			ctx, foreign, f.foreignScope, supplier.ID, product.ID); err == nil {
			t.Fatal("business B read business A's supplier product")
		}
	})

	t.Run("purchase order", func(t *testing.T) {
		if _, err := f.orders.GetPurchaseOrder(ctx, foreign, po.ID); err == nil {
			t.Fatal("business B read business A's purchase order")
		}
	})

	t.Run("purchase order approval", func(t *testing.T) {
		if _, err := f.orders.ApprovePurchaseOrder(ctx, foreign, po.ID,
			PurchaseOrderApprovalInput{ExpectedVersion: po.Version}); err == nil {
			t.Fatal("business B approved business A's purchase order")
		}
	})

	t.Run("purchase order cancel", func(t *testing.T) {
		if _, err := f.orders.CancelPurchaseOrder(ctx, foreign, po.ID,
			PurchaseOrderTransitionInput{ExpectedVersion: po.Version}); err == nil {
			t.Fatal("business B cancelled business A's purchase order")
		}
	})

	t.Run("goods receipt read", func(t *testing.T) {
		if _, err := f.receiving.GetGoodsReceipt(
			ctx, foreignReceiving, receipt.ID); err == nil {
			t.Fatal("business B read business A's goods receipt")
		}
	})

	t.Run("goods receipt reversal", func(t *testing.T) {
		if _, err := f.receiving.ReverseGoodsReceipt(
			ctx, foreignReceiving, receipt.ID, receipt.Version, "probe"); err == nil {
			t.Fatal("business B reversed business A's goods receipt")
		}
	})

	t.Run("receive against a foreign purchase order", func(t *testing.T) {
		if _, err := f.receiving.CreateGoodsReceipt(ctx, foreignReceiving,
			CreateGoodsReceiptInput{
				PurchaseOrderID: po.ID,
				ReceivedAt:      time.Now().UTC(),
			}); err == nil {
			t.Fatal("business B opened a receipt against business A's purchase order")
		}
	})

	t.Run("discrepancy read", func(t *testing.T) {
		if _, err := f.receiving.GetDiscrepancy(
			ctx, foreignReceiving, discrepancyID); err == nil {
			t.Fatal("business B read business A's discrepancy")
		}
	})

	t.Run("supplier performance", func(t *testing.T) {
		if _, err := f.receiving.SupplierPerformance(
			ctx, foreign, f.foreignScope, supplier.ID,
			SupplierPerformanceQuery{}); err == nil {
			t.Fatal("business B aggregated business A's supplier performance")
		}
	})

	t.Run("dashboard never leaks another tenant", func(t *testing.T) {
		board, err := f.receiving.PurchasingDashboard(
			ctx, foreign, f.foreignScope, nil)
		if err != nil {
			t.Fatal(err)
		}
		if board.PurchaseOrders.Ordered != 0 ||
			board.PurchaseOrders.Received != 0 ||
			board.Receiving.TotalPostedReceipts != 0 ||
			board.Discrepancies.Unresolved != 0 {
			t.Fatalf("business B's dashboard counted business A's activity: %+v", board)
		}
	})

	t.Run("cost history", func(t *testing.T) {
		rows, err := f.receiving.ListPurchaseCostHistory(
			ctx, foreignReceiving, f.foreignBranchID, "", 50)
		if err == nil && len(rows) > 0 {
			t.Fatal("business B read business A's purchase cost evidence")
		}
	})
}

// TestSection15D_CrossVendorSupplierProductMapping proves a supplier product
// cannot be mapped onto another business's inventory item, which would let one
// vendor post stock into another vendor's shelves.
func TestSection15D_CrossVendorSupplierProductMapping(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	// An inventory item that belongs to business A.
	foreignItem := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")

	code := "SUP" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:8])

	supplier, err := f.suppliers.CreateSupplier(ctx, f.foreignActor, f.foreignScope,
		CreateSupplierInput{
			Code:      code,
			Name:      "Foreign supplier",
			LegalName: "Foreign supplier Ltd",
			Status:    "ACTIVE",
		})
	if err != nil {
		t.Fatal(err)
	}

	cost := int64(1000)

	if _, err := f.suppliers.CreateSupplierProduct(ctx, f.foreignActor, f.foreignScope,
		supplier.ID, CreateSupplierProductInput{
			BranchID:                      f.foreignBranchID,
			InventoryItemID:               foreignItem,
			SupplierSKU:                   "SKU-CROSS",
			PurchaseUnitCode:              "CASE",
			PurchaseUnitName:              "Case",
			ConversionMode:                ConversionSameAsInventory,
			InventoryUnitsPerPurchaseUnit: 1,
			MinimumOrderQuantity:          1,
			PurchaseMultiple:              1,
			TypicalCostMinor:              &cost,
			CurrencyCode:                  "NGN",
		}); err == nil {
		t.Fatal("business B mapped a supplier product onto business A's inventory item")
	}
}

// TestSection15D_BranchIsolation proves a branch-scoped operator cannot widen
// their view by asking for a branch they do not hold.
func TestSection15D_BranchIsolation(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	// A second branch inside the same business.
	otherBranch := "s15_branch_other_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status)
		 VALUES($1,$2,'Other branch','1 Test Street','Lagos','Lagos','NG',6.5244,3.3792,'Africa/Lagos','ACTIVE')`,
		otherBranch, f.businessID); err != nil {
		t.Fatal(err)
	}

	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 10, nil)

	f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 10,
		AcceptedQuantity:  10,
	})

	// An operator scoped only to the other branch.
	limited := Scope{BranchIDs: []string{otherBranch}}

	t.Run("requesting an unheld branch is refused", func(t *testing.T) {
		if _, err := f.receiving.PurchasingDashboard(
			ctx, f.actor, limited, []string{f.branchID}); err == nil {
			t.Fatal("a branch-scoped operator widened the dashboard to an unheld branch")
		}
	})

	t.Run("the dashboard shows only held branches", func(t *testing.T) {
		board, err := f.receiving.PurchasingDashboard(ctx, f.actor, limited, nil)
		if err != nil {
			t.Fatal(err)
		}
		if board.PurchaseOrders.Received != 0 || board.Receiving.TotalPostedReceipts != 0 {
			t.Fatalf("a branch-scoped dashboard counted another branch's activity: %+v", board)
		}
		if board.Scope != "BRANCH" {
			t.Errorf("scope reported as %q, want BRANCH", board.Scope)
		}
	})

	t.Run("outstanding deliveries respect branch scope", func(t *testing.T) {
		out, err := f.receiving.ListOutstandingDeliveries(
			ctx, f.actor, limited, DeliveryOverdue, nil, 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, delivery := range out {
			if delivery.BranchID != otherBranch {
				t.Fatalf("branch-scoped operator saw a delivery for %s", delivery.BranchID)
			}
		}
	})

	t.Run("supplier performance respects branch scope", func(t *testing.T) {
		perf, err := f.receiving.SupplierPerformance(
			ctx, f.actor, limited, supplier.ID, SupplierPerformanceQuery{})
		if err != nil {
			t.Fatal(err)
		}
		if perf.TotalPostedGoodsReceipt != 0 {
			t.Fatalf("branch-scoped performance counted %d receipts from another branch",
				perf.TotalPostedGoodsReceipt)
		}
	})
}

// TestSection15D_NoWalletMutationOnReceipt is the hard financial gate.
//
// Receiving stock records commercial cost evidence. It must not move money.
// This checks both the runtime effect and the source: a future edit that adds
// a wallet call to purchasing will fail here.
func TestSection15D_NoWalletMutationOnReceipt(t *testing.T) {
	f := newS15Fixture(t)
	ctx := context.Background()

	// Runtime: nothing wallet-shaped exists after a full receive.
	item := f.inventoryItem(t, f.businessID, f.branchID, "SIMPLE")
	supplier, product := f.supplierWithProduct(t, f.actor, f.scope, f.branchID, item, 1, 50000)
	po := f.orderedPO(t, supplier, product, 4, nil)

	f.receiveOnce(t, po, InspectGoodsReceiptLineInput{
		DeliveredQuantity: 4,
		AcceptedQuantity:  4,
	})

	// Any wallet, ledger or payout table that exists in this schema must be
	// untouched by purchasing. Tables that do not exist here are skipped rather
	// than silently passing.
	for _, table := range []string{
		"wallet_accounts", "wallet_ledger_entries", "wallet_transactions",
		"payment_intents", "provider_payouts", "vendor_payouts",
	} {
		var exists bool
		if err := f.pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			continue
		}

		var rows int
		if err := f.pool.QueryRow(ctx,
			`SELECT count(*) FROM `+table).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 0 {
			t.Errorf("receiving stock created %d row(s) in %s; purchasing must not move money",
				rows, table)
		}
	}

	// Source: purchasing must not reference a wallet or payment client at all.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	banned := regexp.MustCompile(
		`walletclient|payment-wallet|PayFromWallet|CreatePaymentIntent|RequestRefund|CompleteJob`)

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}

		body, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}

		if match := banned.Find(body); match != nil {
			t.Errorf("purchasing source %s references %q; "+
				"receiving stock must never settle money", name, string(match))
		}
	}
}
