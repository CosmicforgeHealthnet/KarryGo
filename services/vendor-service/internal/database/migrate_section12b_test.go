package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection12BMigrationReplayAndBoundary(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 12B replay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("fresh replay: %v", err)
	}
	var tip bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_service_schema_migrations WHERE version='000046_vendor_shop_inventory_permissions')`).Scan(&tip); err != nil || !tip {
		t.Fatalf("Section 12B tip missing: %v %v", tip, err)
	}
	for _, table := range []string{"shop_inventory_stock_bases", "shop_inventory_balances", "shop_inventory_movements", "shop_inventory_reservations", "shop_inventory_reservation_lines", "shop_inventory_reservation_allocations", "shop_inventory_lots", "shop_inventory_receipts", "shop_inventory_adjustments", "shop_inventory_quarantines", "shop_inventory_transfers", "shop_inventory_stocktakes", "shop_inventory_policies", "shop_inventory_reorder_rules", "shop_fresh_fulfilment_measurements", "shop_bulk_operation_jobs"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Errorf("required table %s missing: %v", table, err)
		}
	}
	for _, table := range []string{"shop_orders", "shop_order_lines", "shop_carts", "shop_wallet_transactions", "shop_dark_store_inventory"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Errorf("external authority table %s leaked", table)
		}
	}
	var owner, kitchen int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp JOIN vendor_permissions p ON p.id=rp.permission_id WHERE rp.role_id='role_owner' AND p.category='shop_inventory'`).Scan(&owner); err != nil || owner != 9 {
		t.Errorf("owner inventory permissions=%d want=9 err=%v", owner, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp JOIN vendor_permissions p ON p.id=rp.permission_id WHERE rp.role_id='role_kitchen_pastry' AND p.category='shop_inventory'`).Scan(&kitchen); err != nil || kitchen != 0 {
		t.Errorf("kitchen inventory authority=%d err=%v", kitchen, err)
	}
	// Newer Vendor migrations may exist above Section 12B.
	// Walk back to the Section 12B tip before proving its down sequence.
	for {
		var latest string
		if err = pool.QueryRow(ctx, `SELECT version FROM vendor_service_schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&latest); err != nil {
			t.Fatalf("read migration tip before Section 12B rollback: %v", err)
		}

		if latest <= "000046_vendor_shop_inventory_permissions" {
			break
		}

		if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback post-12B migration %s: %v", latest, err)
		}
	}
	for _, version := range []string{"000046", "000045", "000044"} {
		if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback %s: %v", version, err)
		}
	}
	var gone bool
	if err = pool.QueryRow(ctx, `SELECT to_regclass('public.shop_inventory_balances') IS NULL`).Scan(&gone); err != nil || !gone {
		t.Fatalf("Section 12B down incomplete: gone=%v err=%v", gone, err)
	}
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply Section 12B: %v", err)
	}
}
