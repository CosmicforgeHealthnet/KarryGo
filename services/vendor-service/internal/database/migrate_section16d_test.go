package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection16DMigrationApplyRollbackReapply(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("no test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("apply to latest: %v", err)
	}

	tables := []string{"vendor_cost_policies", "vendor_cost_snapshots", "vendor_cost_snapshot_components"}
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("%s exists=%t err=%v", table, exists, err)
		}
	}

	var perms int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions
		WHERE code IN ('vendor.cost.read','vendor.cost.policy.manage','vendor.production.cost.read')`).Scan(&perms); err != nil {
		t.Fatal(err)
	}
	if perms != 3 {
		t.Fatalf("permissions seeded=%d want 3", perms)
	}

	// Kitchen/pastry must hold none of them.
	var kitchen int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE rp.role_id='role_kitchen_pastry' AND p.code LIKE '%cost%'`).Scan(&kitchen); err != nil {
		t.Fatal(err)
	}
	if kitchen != 0 {
		t.Fatalf("kitchen_pastry holds %d cost permissions; want 0", kitchen)
	}

	// Roll back until Section 16D is gone rather than assuming it is the tip.
	// It was the tip when this was written and is not any more - later sections
	// stack on top - and a test that assumed a fixed position would fail as a
	// confusing error in unrelated code every time the repository grew.
	rollbackUntilRemoved(t, ctx, pool, "000074_vendor_operational_cost")
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("after rollback %s exists=%t err=%v", table, exists, err)
		}
	}
	var afterPerms int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code LIKE 'vendor.cost%'`).Scan(&afterPerms); err != nil {
		t.Fatal(err)
	}
	if afterPerms != 0 {
		t.Fatalf("rollback left %d cost permissions", afterPerms)
	}

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply: %v", err)
	}
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("after reapply %s exists=%t err=%v", table, exists, err)
		}
	}
	t.Log("apply -> verify -> rollback -> verify dropped -> reapply -> verify: all pass")
}
