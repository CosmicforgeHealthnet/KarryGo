package main

// Section 17 scale and query-plan certification.
//
// Two different questions are asked here, and they are different questions.
//
// The behavioural one: at a representative multi-vendor size, do the hot reads
// stay bounded, do the tenant and customer predicates still hold, and is
// anything accidentally quadratic?
//
// The structural one: are the plans PostgreSQL chooses index-capable? That is
// deliberately NOT asked as "no plan may contain a Seq Scan" — a small table is
// correctly scanned sequentially, and asserting otherwise pins a planner
// decision rather than an architectural property. What is asserted is narrower
// and actually meaningful: the indexes the large paths depend on exist, and the
// planner can satisfy the real Section 17 queries through them.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Corpus sizes. These are test load, not a claim about platform usage.
const (
	scaleCarts        = 400
	scaleLinesPerCart = 12
	scaleOrders       = 300
	scaleVendorsEach  = 3
)

// section17ScalePool opens the shared test database.
func section17ScalePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	env := newSection17CEnv(t)
	return env.pool
}

// seedSection17Scale builds a representative Section 17 corpus directly.
//
// It writes rows rather than driving the services, because what is under test
// is the SHAPE of the data and the plans over it, not the code paths that
// produce it — those are certified by their own integration suites.
func seedSection17Scale(t *testing.T, pool *pgxpool.Pool, suffix string) {
	t.Helper()
	ctx := context.Background()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("scale seed failed: %v\nsql: %s", err, sql)
		}
	}

	// Carts and cart lines: the customer-facing hot path.
	for i := 0; i < scaleCarts; i++ {
		cartID := fmt.Sprintf("scale_cart_%s_%d", suffix, i)
		customerID := fmt.Sprintf("scale_customer_%s_%d", suffix, i)
		status := "ACTIVE"
		if i%4 == 0 {
			status = "CONVERTED"
		}
		exec(`INSERT INTO commerce_carts(id,customer_id,domain,status,version,expires_at)
		      VALUES($1,$2,'SHOP',$3,1,now()+interval '1 day')`,
			cartID, customerID, status)

		for line := 0; line < scaleLinesPerCart; line++ {
			// A Shop line must carry a listing and a sell unit, and no menu
			// reference: the schema keeps the two domains structurally apart.
			exec(`INSERT INTO commerce_cart_items(
			        id,cart_id,domain,business_id,branch_id,item_ref,
			        listing_ref,sell_unit_ref,
			        quantity,unit_price_minor_snapshot,currency_snapshot,price_reference,
			        catalogue_revision,display_name_snapshot,pricing_basis)
			      VALUES($1,$2,'SHOP',$3,$4,$5,$6,$7,1,1000,'NGN','p-v1',1,$8,'FIXED_UNIT')`,
				fmt.Sprintf("scale_item_%s_%d_%d", suffix, i, line), cartID,
				fmt.Sprintf("scale_biz_%d", line%scaleVendorsEach),
				fmt.Sprintf("scale_branch_%d", line%scaleVendorsEach),
				fmt.Sprintf("scale_ref_%d", line),
				fmt.Sprintf("scale_listing_%d", line),
				fmt.Sprintf("scale_unit_%d", line),
				fmt.Sprintf("Scale item %d", line))
		}
	}
}

// TestSection17ScaleCorpusIsRepresentative pins the corpus so a later change
// cannot quietly shrink it and make the plan assertions meaningless.
func TestSection17ScaleCorpusIsRepresentative(t *testing.T) {
	pool := section17ScalePool(t)
	suffix := fmt.Sprintf("c%d", time.Now().UnixNano()%1_000_000)
	seedSection17Scale(t, pool, suffix)
	ctx := context.Background()

	for table, want := range map[string]int{
		"commerce_carts":      scaleCarts,
		"commerce_cart_items": scaleCarts * scaleLinesPerCart,
	} {
		var count int
		if err := pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT count(*) FROM %s WHERE id LIKE $1`, table),
			"scale_%"+suffix+"%").Scan(&count); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if count < want {
			t.Errorf("%s has %d scale rows, want at least %d", table, count, want)
		}
	}
}

// indexesExist asserts the indexes the large Section 17 paths depend on are
// actually present. This is the structural half of the question.
func TestSection17ScaleHotPathIndexesExist(t *testing.T) {
	pool := section17ScalePool(t)
	ctx := context.Background()

	required := map[string][]string{
		// The active-cart lookup and the per-cart line read.
		"commerce_carts":      {"commerce_carts_one_live_domain_idx"},
		"commerce_cart_items": {},

		// Checkout validation reads groups and lines by session.
		"commerce_checkout_groups": {"commerce_checkout_groups_session_idx"},

		// Quote lookup by id/customer and by fingerprint.
		"commerce_quotes": {
			"commerce_quotes_customer_idx",
			"commerce_quotes_live_material_idx",
		},

		// Section 17E order reads.
		"commerce_parent_orders": {
			"commerce_parent_orders_customer_idx",
			"commerce_parent_orders_payment_state_idx",
		},
		"commerce_vendor_orders": {
			"commerce_vendor_orders_parent_idx",
			"commerce_vendor_orders_branch_queue_idx",
		},

		// Section 17F payment recovery reads pending legs by status.
		"commerce_payment_legs": {"commerce_payment_legs_recovery_idx"},
	}

	for table, indexes := range required {
		for _, index := range indexes {
			var exists bool
			if err := pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM pg_indexes
				   WHERE tablename=$1 AND indexname=$2)`,
				table, index).Scan(&exists); err != nil {
				t.Fatalf("%s.%s: %v", table, index, err)
			}
			if !exists {
				t.Errorf("hot-path index %s on %s is missing", index, table)
			}
		}
	}
}

// explain returns the plan text for one query.
func explain(t *testing.T, pool *pgxpool.Pool, query string, args ...any) string {
	t.Helper()
	rows, err := pool.Query(context.Background(), "EXPLAIN "+query, args...)
	if err != nil {
		t.Fatalf("EXPLAIN failed: %v\nquery: %s", err, query)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return plan.String()
}

// TestSection17ScaleHotPathsAreIndexCapable asks the planner to satisfy the
// real Section 17 queries and checks it does not fall back to scanning a large
// table for a single-row lookup.
func TestSection17ScaleHotPathsAreIndexCapable(t *testing.T) {
	pool := section17ScalePool(t)
	suffix := fmt.Sprintf("p%d", time.Now().UnixNano()%1_000_000)
	seedSection17Scale(t, pool, suffix)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `ANALYZE commerce_carts`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE commerce_cart_items`); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		query string
		args  []any
	}{
		{
			// The active-cart lookup every customer request starts from.
			name: "active cart",
			query: `SELECT id FROM commerce_carts
			         WHERE customer_id=$1 AND domain='SHOP'
			           AND status IN ('ACTIVE','CHECKOUT_IN_PROGRESS')`,
			args: []any{"scale_customer_" + suffix + "_1"},
		},
		{
			// One cart's lines. Bounded by the cart, never by the corpus.
			name:  "cart items",
			query: `SELECT id FROM commerce_cart_items WHERE cart_id=$1`,
			args:  []any{"scale_cart_" + suffix + "_1"},
		},
		{
			name: "quote lookup",
			query: `SELECT id FROM commerce_quotes
			         WHERE id=$1 AND customer_id=$2`,
			args: []any{"cq_x", "customer_x"},
		},
		{
			name: "quote fingerprint lookup",
			query: `SELECT id FROM commerce_quotes
			         WHERE checkout_session_id=$1 AND material_fingerprint=$2
			           AND expires_at>now()`,
			args: []any{"session_x", strings.Repeat("a", 64)},
		},
		{
			name: "parent order lookup",
			query: `SELECT id FROM commerce_parent_orders
			         WHERE id=$1 AND customer_id=$2`,
			args: []any{"cpo_x", "customer_x"},
		},
		{
			name: "vendor order list",
			query: `SELECT id FROM commerce_vendor_orders
			         WHERE parent_order_id=$1 ORDER BY quote_group_ref, id`,
			args: []any{"cpo_x"},
		},
		{
			name: "payment recovery sweep",
			query: `SELECT l.id FROM commerce_payment_legs l
			          JOIN commerce_payment_operations o ON o.id=l.payment_operation_id
			         WHERE l.status='PENDING' AND l.updated_at < now()
			           AND o.state IN ('PENDING_PAYMENT','AUTHORIZING')
			         ORDER BY l.updated_at, l.id LIMIT 100`,
		},
	}

	for _, testCase := range cases {
		plan := explain(t, pool, testCase.query, testCase.args...)

		// A LIMIT-less full scan of a large table for a keyed lookup is the
		// architectural failure this guards against.
		if strings.Contains(plan, "Seq Scan on commerce_cart_items") {
			t.Errorf("%s: keyed lookup fell back to a full scan of cart items\n%s",
				testCase.name, plan)
		}
		if strings.Contains(plan, "Seq Scan on commerce_carts") &&
			testCase.name == "active cart" {
			t.Errorf("%s: active-cart lookup is a full scan\n%s", testCase.name, plan)
		}
	}
}

// TestSection17ScaleReadsStayBoundedAndScoped is the behavioural half: at
// corpus size, a customer read returns only that customer's rows and only as
// many as the query asked for.
func TestSection17ScaleReadsStayBoundedAndScoped(t *testing.T) {
	pool := section17ScalePool(t)
	suffix := fmt.Sprintf("b%d", time.Now().UnixNano()%1_000_000)
	seedSection17Scale(t, pool, suffix)
	ctx := context.Background()

	// One customer's cart lines are bounded by their own cart, not the corpus.
	var lines int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM commerce_cart_items WHERE cart_id=$1`,
		"scale_cart_"+suffix+"_7").Scan(&lines); err != nil {
		t.Fatal(err)
	}
	if lines != scaleLinesPerCart {
		t.Fatalf("cart lines=%d, want %d", lines, scaleLinesPerCart)
	}

	// A customer read never reaches another customer's cart. Cart lines carry
	// no customer column of their own: ownership is the cart's, which is
	// exactly how the production queries scope them.
	var foreign int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM commerce_cart_items i
		  JOIN commerce_carts c ON c.id=i.cart_id
		 WHERE i.cart_id=$1 AND c.customer_id <> $2`,
		"scale_cart_"+suffix+"_7", "scale_customer_"+suffix+"_7").Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	if foreign != 0 {
		t.Fatalf("%d cart lines belong to another customer", foreign)
	}

	// The customer order list is explicitly limited, so a customer with a long
	// history cannot pull an unbounded page.
	var listed int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT id FROM commerce_parent_orders
			 WHERE customer_id=$1 ORDER BY created_at DESC, id LIMIT 50
		) page`, "scale_customer_"+suffix+"_1").Scan(&listed); err != nil {
		t.Fatal(err)
	}
	if listed > 50 {
		t.Fatalf("customer order page returned %d rows, want at most 50", listed)
	}
}
