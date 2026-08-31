package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Section 14I scale and query-plan certification.
//
// A sequential scan over a hundred fixture rows proves nothing, so this file
// builds a realistic generic Inventory dataset first and only then asks
// PostgreSQL how it intends to answer the hot-path queries. The gate is that
// no hot path falls back to a full scan of a large Inventory table.
//
// Fixtures are generated with set-based SQL rather than per-row Go calls: the
// subject under test is the query planner at scale, not the write path, which
// the behavioural suites already cover.

const (
	section14IScaleBusinesses     = 4
	section14IScaleBranchesPerBiz = 3
	section14IScaleItemsPerBranch = 1750

	// 4 * 3 * 1750 = 21,000 generic inventory items.
	section14IScaleExpectedItems = section14IScaleBusinesses *
		section14IScaleBranchesPerBiz *
		section14IScaleItemsPerBranch
)

type section14IScaleFixture struct {
	pool *pgxpool.Pool

	// A representative business/branch/item the hot-path probes target.
	businessID string
	branchID   string
	itemID     string
}

// buildSection14IScaleFixture creates the dataset once per test run.
func buildSection14IScaleFixture(t *testing.T) section14IScaleFixture {
	t.Helper()

	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping Section 14I scale certification")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err = ApplyMigrations(ctx, pool, section14IMigrationsDir()); err != nil {
		t.Fatal(err)
	}

	run := uuid.NewString()[:8]

	var firstBusiness, firstBranch string

	for b := 0; b < section14IScaleBusinesses; b++ {
		business := fmt.Sprintf("s14i_scale_biz_%s_%d", run, b)
		user := fmt.Sprintf("s14i_scale_user_%s_%d", run, b)
		staff := fmt.Sprintf("s14i_scale_staff_%s_%d", run, b)

		if _, err = pool.Exec(ctx,
			`INSERT INTO vendor_users(id,email) VALUES($1,$2)`,
			user, user+"@example.test"); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx,
			`INSERT INTO vendor_staff(id,vendor_id,user_id) VALUES($1,$2,$3)`,
			staff, business, user); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx,
			`INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone)
			 VALUES($1,'Section 14I scale','ACTIVE',$2,'NGN','Africa/Lagos')`,
			business, staff); err != nil {
			t.Fatal(err)
		}

		for br := 0; br < section14IScaleBranchesPerBiz; br++ {
			branch := fmt.Sprintf("s14i_scale_branch_%s_%d_%d", run, b, br)

			if _, err = pool.Exec(ctx,
				`INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status)
				 VALUES($1,$2,'Scale Branch','1 Test Street','Lagos','Lagos','NG',6.5244,3.3792,'Africa/Lagos','ACTIVE')`,
				branch, business); err != nil {
				t.Fatal(err)
			}

			if firstBusiness == "" {
				firstBusiness, firstBranch = business, branch
			}

			seedSection14IScaleBranch(t, pool, ctx, run, business, branch)
		}
	}

	// The planner needs current statistics; without ANALYZE the gate would be
	// measuring stale estimates rather than the shape of the data.
	for _, table := range []string{
		"inventory_items", "inventory_balances", "inventory_lots",
		"inventory_movements", "inventory_reservations",
		"inventory_reservation_allocations", "inventory_reorder_rules",
		"inventory_alerts",
	} {
		if _, err = pool.Exec(ctx, "ANALYZE "+table); err != nil {
			t.Fatal(err)
		}
	}

	var itemID string
	if err = pool.QueryRow(ctx,
		`SELECT id FROM inventory_items WHERE business_id=$1 AND branch_id=$2 LIMIT 1`,
		firstBusiness, firstBranch).Scan(&itemID); err != nil {
		t.Fatal(err)
	}

	return section14IScaleFixture{
		pool:       pool,
		businessID: firstBusiness,
		branchID:   firstBranch,
		itemID:     itemID,
	}
}

// seedSection14IScaleBranch bulk-generates one branch's inventory in a single
// authorized-mutation transaction.
func seedSection14IScaleBranch(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	run string,
	business string,
	branch string,
) {
	t.Helper()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Physical stock tables refuse writes that did not arrive through an
	// authorized inventory mutation. Fixture generation uses the same door.
	if _, err = tx.Exec(ctx,
		`SELECT set_config('karrygo.inventory_mutation','1',true)`); err != nil {
		t.Fatal(err)
	}

	prefix := run + "_" + branch

	// INGREDIENT avoids the SHOP_VARIANT trigger, which would require a whole
	// Shop catalogue per row. Query plans depend on row counts and indexes,
	// not on which source type produced the row.
	if _, err = tx.Exec(ctx,
		`INSERT INTO inventory_items(
			id,business_id,branch_id,item_type,source_ref,
			stock_measure_ref,stock_measure_owner_key,stock_unit_kind,
			stock_basis_mode,tracking_mode,lot_tracking_enabled,lot_policy)
		 SELECT
			'itm_'||$1||'_'||g, $2, $3, 'INGREDIENT', 'src_'||$1||'_'||g,
			'each','shop:each','COUNT','SAME_UNIT',
			CASE WHEN g % 3 = 0 THEN 'EXPIRY' ELSE 'SIMPLE' END,
			(g % 3 = 0),
			CASE WHEN g % 3 = 0 THEN 'FEFO' ELSE 'FIFO' END
		   FROM generate_series(1,$4) AS g`,
		prefix, business, branch, section14IScaleItemsPerBranch); err != nil {
		t.Fatal(err)
	}

	if _, err = tx.Exec(ctx,
		`INSERT INTO inventory_balances(
			id,business_id,branch_id,inventory_item_id,
			on_hand_quantity,reserved_quantity,quarantined_quantity)
		 SELECT 'bal_'||$1||'_'||g, $2, $3, 'itm_'||$1||'_'||g,
			100 + (g % 50), (g % 7), (g % 5)
		   FROM generate_series(1,$4) AS g`,
		prefix, business, branch, section14IScaleItemsPerBranch); err != nil {
		t.Fatal(err)
	}

	// Lots exist for the expiry-tracked third, with a spread of expiry dates so
	// FEFO ordering and the expiry sweep have real work to do.
	if _, err = tx.Exec(ctx,
		`INSERT INTO inventory_lots(
			id,business_id,branch_id,inventory_item_id,
			source_type,source_ref,received_at,expires_at,
			on_hand_quantity,reserved_quantity,quarantined_quantity,status)
		 SELECT 'lot_'||$1||'_'||g, $2, $3, 'itm_'||$1||'_'||g,
			'INTERNAL','seed_'||$1||'_'||g,
			now() - (g % 90) * interval '1 day',
			-- Shelf life runs from receipt, which the schema requires.
			-- Old receipts with short shelf lives therefore land in the
			-- past, giving the expiry sweep genuinely expired lots.
			(now() - (g % 90) * interval '1 day')
				+ ((g % 100) + 5) * interval '1 day',
			50 + (g % 25), 0, 0, 'ACTIVE'
		   FROM generate_series(3,$4,3) AS g`,
		prefix, business, branch, section14IScaleItemsPerBranch); err != nil {
		t.Fatal(err)
	}

	// Movement history: the ledger is the largest Inventory table in production,
	// so it gets several rows per item.
	if _, err = tx.Exec(ctx,
		`INSERT INTO inventory_movements(
			id,business_id,branch_id,inventory_item_id,movement_type,quantity,
			on_hand_delta,source_type,source_ref,occurred_at,
			inventory_revision,balance_revision)
		 SELECT 'mov_'||$1||'_'||g||'_'||s, $2, $3, 'itm_'||$1||'_'||g,
			'RECEIVE', 10, 10, 'INTERNAL', 'seedmov_'||$1||'_'||g||'_'||s,
			now() - (g % 60) * interval '1 hour', g, g
		   FROM generate_series(1,$4) AS g,
		        generate_series(1,3) AS s`,
		prefix, business, branch, section14IScaleItemsPerBranch); err != nil {
		t.Fatal(err)
	}

	if _, err = tx.Exec(ctx,
		`INSERT INTO inventory_reservations(
			id,business_id,branch_id,source_type,source_ref,
			request_fingerprint,status,expires_at)
		 SELECT 'rsv_'||$1||'_'||g, $2, $3, 'ORDER','ord_'||$1||'_'||g,
			-- The fingerprint column is a SHA-256 hex digest and is length
			-- checked at exactly 64 characters. Two md5 digests give a
			-- deterministic 64-character value without pgcrypto.
			md5('fp_'||$1||'_'||g) || md5($1||'_'||g||'_fp'),
			'HELD', now() + (g % 45) * interval '1 minute'
		   FROM generate_series(1,250) AS g`,
		prefix, business, branch); err != nil {
		t.Fatal(err)
	}

	if _, err = tx.Exec(ctx,
		`INSERT INTO inventory_reorder_rules(
			id,business_id,branch_id,inventory_item_id,
			minimum_quantity,target_quantity,status)
		 SELECT 'ror_'||$1||'_'||g, $2, $3, 'itm_'||$1||'_'||g,
			20, 100, 'ACTIVE'
		   FROM generate_series(1,250) AS g`,
		prefix, business, branch); err != nil {
		t.Fatal(err)
	}

	if _, err = tx.Exec(ctx,
		`INSERT INTO inventory_alerts(
			id,business_id,branch_id,inventory_item_id,
			alert_type,status,dedupe_key)
		 SELECT 'alt_'||$1||'_'||g, $2, $3, 'itm_'||$1||'_'||g,
			'LOW_STOCK','OPEN','dedupe_'||$1||'_'||g
		   FROM generate_series(1,150) AS g`,
		prefix, business, branch); err != nil {
		t.Fatal(err)
	}

	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestSection14IScaleDatasetIsRealisticallyLarge records the certified dataset
// size. If this shrinks, the query-plan gate below stops meaning anything.
func TestSection14IScaleDatasetIsRealisticallyLarge(t *testing.T) {
	f := buildSection14IScaleFixture(t)
	ctx := context.Background()

	counts := map[string]int{}

	for _, table := range []string{
		"inventory_items", "inventory_balances", "inventory_lots",
		"inventory_movements", "inventory_reservations",
		"inventory_reorder_rules", "inventory_alerts",
	} {
		var n int
		if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		counts[table] = n
		t.Logf("SCALE %-34s %8d rows", table, n)
	}

	if counts["inventory_items"] < 20000 {
		t.Fatalf("generic Inventory scale dataset has %d items; Section 14I requires at least 20000",
			counts["inventory_items"])
	}
}

// TestSection14IHotPathQueryPlans asks PostgreSQL how it answers each hot path
// at scale and refuses a full scan of a large Inventory table.
func TestSection14IHotPathQueryPlans(t *testing.T) {
	f := buildSection14IScaleFixture(t)
	ctx := context.Background()

	// Tables large enough that a sequential scan on a selective lookup is a
	// production problem rather than a planner preference.
	large := []string{
		"inventory_items", "inventory_balances", "inventory_lots",
		"inventory_movements", "inventory_reservations",
	}

	probes := []struct {
		name string
		sql  string
		args []any
	}{
		{
			"balances by business and branch",
			`SELECT * FROM inventory_balances
			  WHERE business_id=$1 AND branch_id=$2
			  ORDER BY inventory_item_id LIMIT 50`,
			[]any{f.businessID, f.branchID},
		},
		{
			"item availability by item",
			`SELECT * FROM inventory_balances
			  WHERE inventory_item_id=$1 AND business_id=$2`,
			[]any{f.itemID, f.businessID},
		},
		{
			"item branch lookup",
			`SELECT branch_id FROM inventory_items
			  WHERE id=$1 AND business_id=$2`,
			[]any{f.itemID, f.businessID},
		},
		{
			"reservation by source reference",
			`SELECT * FROM inventory_reservations
			  WHERE business_id=$1 AND source_type='ORDER' AND source_ref=$2`,
			[]any{f.businessID, "ord_x"},
		},
		{
			"FEFO lot allocation",
			`SELECT * FROM inventory_lots
			  WHERE business_id=$1 AND inventory_item_id=$2
			    AND status='ACTIVE' AND on_hand_quantity > 0
			  ORDER BY expires_at, received_at, id LIMIT 20`,
			[]any{f.businessID, f.itemID},
		},
		{
			"movement history by branch",
			`SELECT * FROM inventory_movements
			  WHERE business_id=$1 AND branch_id=$2
			  ORDER BY occurred_at DESC, id LIMIT 50`,
			[]any{f.businessID, f.branchID},
		},
		{
			"movement history by item",
			`SELECT * FROM inventory_movements
			  WHERE business_id=$1 AND inventory_item_id=$2
			  ORDER BY occurred_at DESC, id LIMIT 50`,
			[]any{f.businessID, f.itemID},
		},
		{
			"lot expiry sweep",
			`SELECT * FROM inventory_lots
			  WHERE business_id=$1 AND branch_id=$2 AND expires_at <= now()
			  LIMIT 100`,
			[]any{f.businessID, f.branchID},
		},
		{
			"reservation expiry sweep",
			`SELECT id FROM inventory_reservations
			  WHERE status IN ('HELD','CONFIRMED') AND expires_at IS NOT NULL
			    AND expires_at <= now()
			  ORDER BY expires_at, id LIMIT 100`,
			nil,
		},
		{
			"reorder rule scan",
			`SELECT * FROM inventory_reorder_rules
			  WHERE business_id=$1 AND branch_id=$2 AND status='ACTIVE' LIMIT 100`,
			[]any{f.businessID, f.branchID},
		},
		{
			"open alert scan",
			`SELECT * FROM inventory_alerts
			  WHERE business_id=$1 AND branch_id=$2 AND status='OPEN'
			  ORDER BY created_at DESC LIMIT 50`,
			[]any{f.businessID, f.branchID},
		},
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
					t.Errorf("hot path %q sequentially scans %s at %d-item scale",
						probe.name, table, section14IScaleExpectedItems)
				}
			}
		})
	}
}
