package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection12AMigrationReplayAndBoundary(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 12A replay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("fresh migration replay: %v", err)
	}

	var tip bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vendor_service_schema_migrations WHERE version='000043_vendor_shop_permissions')`).Scan(&tip); err != nil || !tip {
		t.Fatalf("Section 12A migration tip absent: exists=%v err=%v", tip, err)
	}

	var seeded, unsafe int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shop_measure_definitions`).Scan(&seeded); err != nil {
		t.Fatal(err)
	}
	// TestMigrationReplay's legacy TRUNCATE ... CASCADE cleanup can empty a
	// platform-seed table while leaving its migration marked applied. Repair
	// that test-only state by replaying the two Section 12A migrations.
	if seeded == 0 {
		// Newer Vendor migrations may exist above Section 12A.
		// Walk back to the Section 12A tip before replaying its pair.
		for {
			var latest string
			if err = pool.QueryRow(ctx, `SELECT version FROM vendor_service_schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&latest); err != nil {
				t.Fatalf("read migration tip for Section 12A seed repair: %v", err)
			}

			if latest <= "000043_vendor_shop_permissions" {
				break
			}

			if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
				t.Fatalf("rollback post-12A migration %s for seed repair: %v", latest, err)
			}
		}

		if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback 000043 for seed repair: %v", err)
		}

		if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback 000042 for seed repair: %v", err)
		}

		if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
			t.Fatal(err)
		}

		if err = pool.QueryRow(ctx, `SELECT count(*) FROM shop_measure_definitions`).Scan(&seeded); err != nil {
			t.Fatal(err)
		}
	}
	if seeded < 40 {
		t.Fatalf("controlled measurement seed incomplete: count=%d err=%v", seeded, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shop_measure_definitions WHERE code IN ('PAINT','DERICA','MUDU','CONGO','TIYA') AND base_quantity IS NOT NULL`).Scan(&unsafe); err != nil || unsafe != 0 {
		t.Fatalf("local measure was assigned a universal base quantity: count=%d err=%v", unsafe, err)
	}
	localCodes := []string{"PAINT", "PAINT_RUBBER", "CUSTARD_RUBBER", "DERICA", "MUDU", "CONGO", "TIYA", "KOBIOWU", "MILK_CUP", "CIGARETTE_CUP", "CUP", "BOWL", "BASIN", "BASKET", "RUBBER", "HEAP", "BUNCH", "BUNDLE", "STALK", "STICK", "TUBER", "JERRYCAN", "WHEELBARROW", "PICKUP_LOAD", "BAG", "SACK", "CRATE", "CARTON", "TRAY", "DRUM", "BOTTLE", "CAN", "PACK", "SACHET", "ROLL"}
	var localCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM shop_measure_definitions WHERE code=ANY($1)`, localCodes).Scan(&localCount); err != nil || localCount != len(localCodes) {
		t.Fatalf("Nigerian/local trade seed count=%d want=%d err=%v", localCount, len(localCodes), err)
	}

	for _, table := range []string{
		"shop_categories", "shop_products", "shop_variants", "shop_measure_definitions",
		"shop_measure_aliases", "shop_measure_calibrations", "shop_sell_units",
		"shop_package_relations", "shop_product_identifiers", "shop_branch_listings",
		"shop_quantity_breaks", "shop_fresh_attributes", "shop_price_versions",
		"shop_catalogue_revisions", "shop_branch_operating_models",
	} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Errorf("required table %s absent: exists=%v err=%v", table, exists, err)
		}
	}

	// Later Section 12B legitimately adds Vendor-owned inventory. Order
	// authority remains external throughout Section 12 and must never appear.
	for _, forbidden := range []string{"shop_orders", "shop_order_lines", "shop_carts", "shop_checkout_sessions"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, forbidden).Scan(&exists); err != nil || exists {
			t.Errorf("Part B/external authority table %s present: exists=%v err=%v", forbidden, exists, err)
		}
	}

	var ownerPermissions, kitchenPermissions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp JOIN vendor_permissions p ON p.id=rp.permission_id WHERE rp.role_id='role_owner' AND p.category='shop'`).Scan(&ownerPermissions); err != nil || ownerPermissions != 7 {
		t.Errorf("owner Shop permission count=%d want=7 err=%v", ownerPermissions, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp JOIN vendor_permissions p ON p.id=rp.permission_id WHERE rp.role_id='role_kitchen_pastry' AND p.code LIKE 'vendor.shop.%'`).Scan(&kitchenPermissions); err != nil || kitchenPermissions != 0 {
		t.Errorf("kitchen/pastry leaked Shop authority: count=%d err=%v", kitchenPermissions, err)
	}

	// Walk through later accepted migrations before proving both Section 12A
	// downs together. Boundary tests must not assume their own section remains
	// the repository tip forever.
	for {
		var latest string
		if err = pool.QueryRow(ctx, `SELECT version FROM vendor_service_schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&latest); err != nil {
			t.Fatalf("read migration tip: %v", err)
		}
		if latest <= "000043_vendor_shop_permissions" {
			break
		}
		if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback post-12A migration %s: %v", latest, err)
		}
	}
	if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("rollback 000043: %v", err)
	}
	if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("rollback 000042: %v", err)
	}
	var catalogueGone bool
	if err = pool.QueryRow(ctx, `SELECT to_regclass('public.shop_products') IS NULL`).Scan(&catalogueGone); err != nil || !catalogueGone {
		t.Fatalf("Section 12A catalogue down incomplete: gone=%v err=%v", catalogueGone, err)
	}
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply Section 12A after down: %v", err)
	}
}
