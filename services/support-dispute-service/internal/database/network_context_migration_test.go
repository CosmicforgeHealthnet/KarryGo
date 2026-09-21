package database

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestNetworkContextMigrationReplayAndRollback certifies migration 008.
//
// The runner re-applies every file on every start, so 008 must be a no-op the
// second time. The down script lives in migrations/down/ (the runner skips
// directories) and must remove exactly what 008 added, after which 008 must
// apply cleanly again.
func TestNetworkContextMigrationReplayAndRollback(t *testing.T) {
	dsn := os.Getenv("SUPPORT_DISPUTE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SUPPORT_DISPUTE_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	// Cleanups run last-registered-first: the reapply below runs while still
	// in the service root, then this restores the directory.
	t.Cleanup(func() { _ = os.Chdir(old) })

	// Whatever happens below, leave the schema complete for the next package.
	t.Cleanup(func() {
		if err := ApplyMigrations(ctx, pool); err != nil {
			t.Errorf("reapply in cleanup: %v", err)
		}
	})

	columnExists := func(table, column string) bool {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns
			WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`, table, column).
			Scan(&exists); err != nil {
			t.Fatal(err)
		}
		return exists
	}
	tableExists := func(table string) bool {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).
			Scan(&exists); err != nil {
			t.Fatal(err)
		}
		return exists
	}
	assertApplied := func(stage string) {
		t.Helper()
		for _, column := range []string{"context_type", "order_ref", "fulfilment_ref", "mission_ref",
			"provider_ref", "asset_ref", "guarantee_breach_ref", "recovery_case_ref"} {
			if !columnExists("complaints", column) {
				t.Fatalf("%s: complaints.%s missing", stage, column)
			}
		}
		for _, table := range []string{"complaint_context_snapshots", "support_action_requests"} {
			if !tableExists(table) {
				t.Fatalf("%s: %s missing", stage, table)
			}
		}
	}

	// Apply twice: the second pass is what every service restart does.
	for pass := 1; pass <= 2; pass++ {
		if err = ApplyMigrations(ctx, pool); err != nil {
			t.Fatalf("apply pass %d: %v", pass, err)
		}
	}
	assertApplied("after replay")

	down, err := os.ReadFile(filepath.Join("migrations", "down", "008_network_case_context.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	for _, column := range []string{"context_type", "mission_ref", "recovery_case_ref"} {
		if columnExists("complaints", column) {
			t.Fatalf("down left complaints.%s behind", column)
		}
	}
	for _, table := range []string{"complaint_context_snapshots", "support_action_requests"} {
		if tableExists(table) {
			t.Fatalf("down left %s behind", table)
		}
	}
	// Columns 008 did not add must survive its rollback.
	for _, column := range []string{"commerce_context", "organization_reference", "category"} {
		if !columnExists("complaints", column) {
			t.Fatalf("down removed complaints.%s, which 008 does not own", column)
		}
	}

	if err = ApplyMigrations(ctx, pool); err != nil {
		t.Fatalf("reapply: %v", err)
	}
	assertApplied("after reapply")
}
