package purchasing

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Section 15D scale and query-plan certification.
//
// A plan over a hundred fixture rows proves nothing, so this builds a
// realistic purchasing dataset first and only then asks PostgreSQL how it
// intends to answer the operational hot paths. The gate is that no hot path
// falls back to a full scan of a large purchasing table.

const (
	scaleSuppliers        = 400
	scaleProductsPerSup   = 5
	scaleOrders           = 4000
	scaleLinesPerOrder    = 3
	scaleReceipts         = 3000
	scaleDiscrepancies    = 2000
	scaleCostObservations = 9000
)

// buildPurchasingScaleData generates the dataset with set-based SQL. The
// subject under test is the planner, not the write path, which the behavioural
// gates already cover.
func buildPurchasingScaleData(t *testing.T, f s15Fixture) string {
	t.Helper()
	ctx := context.Background()

	run := strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	business := f.businessID
	branch := f.branchID

	// Suppliers.
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO vendor_suppliers(id,business_id,code,name,legal_name,status)
		 SELECT 'sup_'||$1||'_'||g, $2, upper('SC'||$1||g), 'Scale supplier '||g,
		        'Scale supplier '||g||' Ltd', 'ACTIVE'
		   FROM generate_series(1,$3) AS g`,
		run, business, scaleSuppliers); err != nil {
		t.Fatal(err)
	}

	// Inventory items and supplier products.
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO inventory_items(
			id,business_id,branch_id,item_type,source_ref,
			stock_measure_ref,stock_measure_owner_key,stock_unit_kind,
			stock_basis_mode,tracking_mode)
		 SELECT 'sitm_'||$1||'_'||g, $2, $3, 'INGREDIENT', 'ssrc_'||$1||'_'||g,
		        'each','shop:each','COUNT','SAME_UNIT','SIMPLE'
		   FROM generate_series(1,$4) AS g`,
		run, business, branch, scaleSuppliers*scaleProductsPerSup); err != nil {
		t.Fatal(err)
	}

	if _, err := f.pool.Exec(ctx,
		`INSERT INTO vendor_supplier_products(
			id,business_id,branch_id,supplier_id,inventory_item_id,
			supplier_sku,purchase_unit_code,purchase_unit_name,
			conversion_mode,inventory_units_per_purchase_unit,
			minimum_order_quantity,purchase_multiple,currency_code,
			inventory_measure_ref,inventory_measure_owner_key,
			inventory_unit_kind)
		 SELECT 'sp_'||$1||'_'||g, $2, $3,
		        'sup_'||$1||'_'||((g - 1) / $5 + 1),
		        'sitm_'||$1||'_'||g,
		        upper('SKU'||$1||g),'CASE','Case','SAME_AS_INVENTORY',1,1,1,'NGN',
		        'each','shop:each','COUNT'
		   FROM generate_series(1,$4) AS g`,
		run, business, branch,
		scaleSuppliers*scaleProductsPerSup, scaleProductsPerSup); err != nil {
		t.Fatal(err)
	}

	// Purchase orders across every status, with a spread of expected dates so
	// the overdue and upcoming scans have real work.
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO vendor_purchase_orders(
			id,business_id,branch_id,supplier_id,purchase_order_number,
			supplier_code_snapshot,supplier_name_snapshot,
			supplier_legal_name_snapshot,currency_code,
			subtotal_minor,total_amount_minor,status,
			expected_delivery_date,created_by_staff_id,created_at)
		 SELECT 'spo_'||$1||'_'||g, $2, $3,
		        'sup_'||$1||'_'||(g % $5 + 1),
		        'PO-'||$1||'-'||g,
		        'SC','Scale supplier','Scale supplier Ltd','NGN',
		        50000, 50000,
		        (ARRAY['DRAFT','SUBMITTED','APPROVED','ORDERED',
		               'PARTIALLY_RECEIVED','RECEIVED','CLOSED',
		               'REJECTED','CANCELLED'])[1 + (g % 9)],
		        current_date + ((g % 60) - 30),
		        $4,
		        now() - (g % 180) * interval '1 day'
		   FROM generate_series(1,$6) AS g`,
		run, business, branch, f.staffID, scaleSuppliers, scaleOrders); err != nil {
		t.Fatal(err)
	}

	if _, err := f.pool.Exec(ctx,
		`INSERT INTO vendor_purchase_order_lines(
			id,business_id,purchase_order_id,supplier_product_id,
			supplier_product_version,inventory_item_id,supplier_sku_snapshot,
			purchase_unit_code_snapshot,purchase_unit_name_snapshot,
			conversion_mode_snapshot,inventory_units_per_purchase_unit_snapshot,
			inventory_measure_ref_snapshot,inventory_measure_owner_key_snapshot,
			inventory_unit_kind_snapshot,purchase_quantity,
			converted_inventory_quantity,unit_cost_minor,line_total_minor,
			currency_code_snapshot)
		 SELECT 'spol_'||$1||'_'||g||'_'||s, $2, 'spo_'||$1||'_'||g,
		        'sp_'||$1||'_'||((g + s) % $4 + 1), 1,
		        'sitm_'||$1||'_'||((g + s) % $4 + 1),
		        'SKU','CASE','Case','SAME_AS_INVENTORY',1,
		        'each','shop:each','COUNT',
		        10, 10, 5000, 50000, 'NGN'
		   FROM generate_series(1,$3) AS g, generate_series(1,$5) AS s`,
		run, business, scaleOrders,
		scaleSuppliers*scaleProductsPerSup, scaleLinesPerOrder); err != nil {
		t.Fatal(err)
	}

	// Goods receipts.
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO vendor_goods_receipts(
			id,business_id,purchase_order_id,supplier_id,branch_id,
			currency,status,received_at,received_by_staff_id)
		 SELECT 'sgr_'||$1||'_'||g, $2, 'spo_'||$1||'_'||g,
		        'sup_'||$1||'_'||(g % $4 + 1), $3,
		        'NGN',
		        (ARRAY['DRAFT','INSPECTED','POSTED','POSTED','REVERSED'])[1 + (g % 5)],
		        now() - (g % 120) * interval '1 day',
		        $5
		   FROM generate_series(1,$6) AS g`,
		run, business, branch, scaleSuppliers, f.staffID, scaleReceipts); err != nil {
		t.Fatal(err)
	}

	// Receipt lines. Cost evidence hangs off these, one observation per line,
	// which is the uniqueness the schema already enforces.
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO vendor_goods_receipt_lines(
			id,business_id,goods_receipt_id,purchase_order_id,
			purchase_order_line_id,supplier_product_id,inventory_item_id,
			ordered_quantity_snapshot,received_before_snapshot,
			expected_unit_cost_minor,
			purchase_unit_code,inventory_units_per_purchase_unit,
			delivered_quantity,accepted_quantity,currency)
		 SELECT 'sgrl_'||$1||'_'||((g - 1) * $4 + s), $2,
		        'sgr_'||$1||'_'||g, 'spo_'||$1||'_'||g,
		        'spol_'||$1||'_'||g||'_'||s,
		        'sp_'||$1||'_'||(g % $5 + 1),
		        'sitm_'||$1||'_'||(g % $5 + 1),
		        10, 0, 5000,
		        'CASE',1,10,10,'NGN'
		   FROM generate_series(1,$3) AS g, generate_series(1,$4) AS s`,
		run, business, scaleReceipts, scaleCostObservations/scaleReceipts,
		scaleSuppliers*scaleProductsPerSup); err != nil {
		t.Fatal(err)
	}

	// Discrepancies across every type and status.
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO vendor_supplier_discrepancies(
			id,business_id,supplier_id,purchase_order_id,goods_receipt_id,
			branch_id,discrepancy_type,status,quantity,currency,
			opened_by_staff_id,created_at)
		 SELECT 'sdi_'||$1||'_'||g, $2,
		        'sup_'||$1||'_'||(g % $4 + 1),
		        'spo_'||$1||'_'||g,
		        'sgr_'||$1||'_'||g,
		        $3,
		        (ARRAY['SHORT_DELIVERY','OVER_DELIVERY','DAMAGED','REJECTED',
		               'WRONG_ITEM','QUALITY_ISSUE','COST_VARIANCE'])[1 + (g % 7)],
		        (ARRAY['OPEN','APPROVAL_REQUIRED','APPROVED','RESOLVED'])[1 + (g % 4)],
		        5, 'NGN', $5,
		        now() - (g % 90) * interval '1 day'
		   FROM generate_series(1,$6) AS g`,
		run, business, branch, scaleSuppliers, f.staffID, scaleDiscrepancies); err != nil {
		t.Fatal(err)
	}

	// Immutable purchase cost observations.
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO vendor_purchase_cost_history(
			id,business_id,branch_id,supplier_id,supplier_product_id,
			inventory_item_id,purchase_order_id,purchase_order_line_id,
			goods_receipt_id,goods_receipt_line_id,evidence_type,
			purchase_unit_code,inventory_units_per_purchase_unit,
			purchase_quantity,inventory_quantity,purchase_unit_cost_minor,
			total_cost_minor,currency,observed_at,recorded_by_staff_id)
		 SELECT 'sch_'||$1||'_'||g, $2, $3,
		        'sup_'||$1||'_'||(g % $4 + 1),
		        'sp_'||$1||'_'||(g % $5 + 1),
		        'sitm_'||$1||'_'||(g % $5 + 1),
		        'spo_'||$1||'_'||(g % $6 + 1),
		        'spol_'||$1||'_'||g,
		        'sgr_'||$1||'_'||((g - 1) / ($8 / $7) + 1),
		        'sgrl_'||$1||'_'||g,
		        'OBSERVED','CASE',1,10,10,5000,50000,'NGN',
		        now() - (g % 200) * interval '1 day', $9
		   FROM generate_series(1,$8) AS g`,
		run, business, branch, scaleSuppliers,
		scaleSuppliers*scaleProductsPerSup, scaleOrders, scaleReceipts,
		scaleCostObservations, f.staffID); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{
		"vendor_suppliers", "vendor_supplier_products",
		"vendor_purchase_orders", "vendor_purchase_order_lines",
		"vendor_goods_receipts", "vendor_supplier_discrepancies",
		"vendor_purchase_cost_history",
	} {
		if _, err := f.pool.Exec(ctx, "ANALYZE "+table); err != nil {
			t.Fatal(err)
		}
	}

	return run
}

// TestSection15D_ScaleDatasetIsRealistic records the certified dataset size.
func TestSection15D_ScaleDatasetIsRealistic(t *testing.T) {
	f := newS15Fixture(t)
	buildPurchasingScaleData(t, f)
	ctx := context.Background()

	counts := map[string]int{}
	for _, table := range []string{
		"vendor_suppliers", "vendor_supplier_products",
		"vendor_purchase_orders", "vendor_purchase_order_lines",
		"vendor_goods_receipts", "vendor_supplier_discrepancies",
		"vendor_purchase_cost_history",
	} {
		var n int
		if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		counts[table] = n
		t.Logf("SCALE %-32s %7d rows", table, n)
	}

	if counts["vendor_purchase_orders"] < scaleOrders {
		t.Fatalf("purchase orders = %d, want at least %d",
			counts["vendor_purchase_orders"], scaleOrders)
	}
	if counts["vendor_purchase_cost_history"] < scaleCostObservations {
		t.Fatalf("cost observations = %d, want at least %d",
			counts["vendor_purchase_cost_history"], scaleCostObservations)
	}
}

// TestSection15D_HotPathQueryPlans asks PostgreSQL how it answers each
// operational hot path at scale and refuses a full scan of a large table.
func TestSection15D_HotPathQueryPlans(t *testing.T) {
	f := newS15Fixture(t)
	run := buildPurchasingScaleData(t, f)
	ctx := context.Background()

	large := []string{
		"vendor_purchase_orders", "vendor_purchase_order_lines",
		"vendor_goods_receipts", "vendor_supplier_discrepancies",
		"vendor_purchase_cost_history",
	}

	supplier := "sup_" + run + "_1"
	item := "sitm_" + run + "_1"
	order := "spo_" + run + "_1"

	probes := []struct {
		name string
		sql  string
		args []any
	}{
		{"supplier listing", `SELECT * FROM vendor_suppliers
		  WHERE business_id=$1 AND status='ACTIVE'
		  ORDER BY name LIMIT 50`, []any{f.businessID}},

		{"supplier product by supplier", `SELECT * FROM vendor_supplier_products
		  WHERE business_id=$1 AND supplier_id=$2 LIMIT 50`,
			[]any{f.businessID, supplier}},

		{"supplier product by inventory item", `SELECT * FROM vendor_supplier_products
		  WHERE business_id=$1 AND inventory_item_id=$2`,
			[]any{f.businessID, item}},

		{"purchase orders by branch and status", `SELECT * FROM vendor_purchase_orders
		  WHERE business_id=$1 AND status='ORDERED' AND branch_id=$2
		  LIMIT 50`, []any{f.businessID, f.branchID}},

		{"overdue purchase orders", `SELECT * FROM vendor_purchase_orders
		  WHERE business_id=$1
		    AND status IN ('ORDERED','PARTIALLY_RECEIVED')
		    AND expected_delivery_date IS NOT NULL
		    AND expected_delivery_date < current_date
		  ORDER BY expected_delivery_date LIMIT 50`, []any{f.businessID}},

		{"upcoming deliveries", `SELECT * FROM vendor_purchase_orders
		  WHERE business_id=$1
		    AND status IN ('ORDERED','PARTIALLY_RECEIVED')
		    AND expected_delivery_date BETWEEN current_date AND current_date + 3
		  ORDER BY expected_delivery_date LIMIT 50`, []any{f.businessID}},

		{"approval queue", `SELECT * FROM vendor_purchase_orders
		  WHERE business_id=$1 AND status='SUBMITTED'
		  ORDER BY created_at DESC LIMIT 50`, []any{f.businessID}},

		{"supplier order history", `SELECT * FROM vendor_purchase_orders
		  WHERE business_id=$1 AND supplier_id=$2
		  ORDER BY created_at DESC LIMIT 50`, []any{f.businessID, supplier}},

		{"purchase order lines", `SELECT * FROM vendor_purchase_order_lines
		  WHERE business_id=$1 AND purchase_order_id=$2`,
			[]any{f.businessID, order}},

		{"receipt listing by status", `SELECT * FROM vendor_goods_receipts
		  WHERE business_id=$1 AND status='POSTED'
		  ORDER BY received_at DESC LIMIT 50`, []any{f.businessID}},

		{"receipts by supplier", `SELECT * FROM vendor_goods_receipts
		  WHERE business_id=$1 AND supplier_id=$2
		  ORDER BY received_at DESC LIMIT 50`, []any{f.businessID, supplier}},

		{"open discrepancy listing", `SELECT * FROM vendor_supplier_discrepancies
		  WHERE business_id=$1 AND status IN ('OPEN','APPROVAL_REQUIRED')
		  ORDER BY created_at LIMIT 50`, []any{f.businessID}},

		{"discrepancies by supplier", `SELECT * FROM vendor_supplier_discrepancies
		  WHERE business_id=$1 AND supplier_id=$2 AND status='OPEN'`,
			[]any{f.businessID, supplier}},

		{"cost history by inventory item", `SELECT * FROM vendor_purchase_cost_history
		  WHERE business_id=$1 AND inventory_item_id=$2
		  ORDER BY observed_at DESC LIMIT 50`, []any{f.businessID, item}},

		{"cost history by supplier", `SELECT * FROM vendor_purchase_cost_history
		  WHERE business_id=$1 AND supplier_id=$2
		  ORDER BY observed_at DESC LIMIT 50`, []any{f.businessID, supplier}},
	}

	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			rows, err := f.pool.Query(ctx,
				"EXPLAIN (ANALYZE, BUFFERS) "+probe.sql, probe.args...)
			if err != nil {
				t.Fatal(err)
			}

			var plan strings.Builder
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				plan.WriteString(line)
				plan.WriteString("\n")
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}

			text := plan.String()
			t.Logf("PLAN %s\n%s", probe.name, text)

			for _, table := range large {
				if strings.Contains(text, "Seq Scan on "+table) {
					t.Errorf("hot path %q sequentially scans %s at purchasing scale",
						probe.name, table)
				}
			}
		})
	}
}

// TestSection15D_DashboardIsBoundedAtScale proves the operational read model
// stays a bounded set of aggregate queries rather than degrading into a
// per-supplier fan-out.
func TestSection15D_DashboardIsBoundedAtScale(t *testing.T) {
	f := newS15Fixture(t)
	buildPurchasingScaleData(t, f)
	ctx := context.Background()

	board, err := f.receiving.PurchasingDashboard(ctx, f.actor, f.scope, nil)
	if err != nil {
		t.Fatal(err)
	}

	total := board.PurchaseOrders.Draft + board.PurchaseOrders.Submitted +
		board.PurchaseOrders.Approved + board.PurchaseOrders.Ordered +
		board.PurchaseOrders.PartiallyReceived + board.PurchaseOrders.Received +
		board.PurchaseOrders.Closed + board.PurchaseOrders.Rejected +
		board.PurchaseOrders.Cancelled

	if total < scaleOrders {
		t.Fatalf("dashboard counted %d purchase orders, want at least %d",
			total, scaleOrders)
	}

	if board.Discrepancies.Unresolved == 0 {
		t.Error("dashboard reports no unresolved discrepancies in a dataset that has them")
	}
	if board.Suppliers.Active == 0 {
		t.Error("dashboard reports no active suppliers at scale")
	}

	t.Logf("DASHBOARD orders=%d overdue=%d unresolved=%d suppliers=%d",
		total, board.Deliveries.Overdue,
		board.Discrepancies.Unresolved, board.Suppliers.Active)
}
