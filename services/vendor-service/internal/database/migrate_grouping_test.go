package database

// Migration 000118 is reversible and repeatable.
//
// It is worth its own test rather than being folded into the generic replay
// because of what it touches: an append-only audit table whose event vocabulary
// it widens. Restoring the narrower CHECK while grouping evidence is still in
// the table would fail, and the immutability trigger would then refuse the
// delete that would have fixed it -- a rollback that cannot be rolled back.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func groupingMigrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set")
	}
	if !strings.Contains(strings.ToLower(dsn), "test") {
		t.Fatal("refusing a database whose name does not identify it as a test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestGroupingMigrationRollbackAndReapply(t *testing.T) {
	pool := groupingMigrationPool(t)
	ctx := context.Background()

	columnExists := func(column string) bool {
		t.Helper()
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM information_schema.columns
			 WHERE table_name='vendor_orchestration_pickups' AND column_name=$1)`,
			column).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		return exists
	}
	for _, column := range []string{"group_index", "split_reason", "weight_grams",
		"temperature_band", "secure_pickup", "pickup_latitude"} {
		if !columnExists(column) {
			t.Fatalf("%s missing after apply", column)
		}
	}

	// Seed the state the rollback actually has to survive: an orchestration
	// carrying grouping evidence in the append-only events table. A rollback
	// tested against an empty table proves nothing, because an empty table
	// satisfies any CHECK.
	// A fresh id per run. The evidence rows this seeds are append-only by
	// trigger and deliberately cannot be cleaned up afterwards, so reusing one
	// id would make the test pass once and then fail on its own leftovers.
	id := "orc_grouping_rollback_" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO vendor_fulfilment_orchestrations(
			id, fulfilment_group_ref, parent_order_id, plan_version, state, requirements_snapshot)
		VALUES($1,$2,$3,1,'PLANNED','{}'::jsonb)
		ON CONFLICT (id) DO NOTHING`,
		id, id+"_fg", id+"_po"); err != nil {
		t.Fatalf("seed orchestration: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO vendor_orchestration_events(id, orchestration_id, plan_version, event_type, reason_code)
		VALUES($1,$2,1,'GROUP_PLANNED','GROUP_0_SHARED_2'),
		      ($3,$2,1,'GROUP_SPLIT','PICKUP_LIMIT')`,
		id+"_e1", id, id+"_e2"); err != nil {
		t.Fatalf("seed grouping evidence: %v", err)
	}

	// Roll back until 000118 itself is undone, rather than assuming it is the
	// last migration. It was when it was written and it is not any more, and a
	// test that silently rolls back whatever happens to be on top would start
	// asserting things about somebody else's migration.
	//
	// The bound is there so a down file that fails to drop the column makes
	// this test fail rather than unwind the entire schema looking for it.
	rolledBack := 0
	for columnExists("group_index") {
		if rolledBack >= 8 {
			t.Fatal("rolled back 8 migrations and group_index is still present; " +
				"000118's down file is not dropping it")
		}
		if err := RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback: %v", err)
		}
		rolledBack++
	}

	// The narrowed vocabulary must actually be back in force, not merely
	// dropped. A rollback that leaves the constraint off would let the next
	// deployment write event types nobody has defined.
	var refused bool
	if _, err := pool.Exec(ctx, `
		INSERT INTO vendor_orchestration_events(id, orchestration_id, plan_version, event_type)
		VALUES($1,$2,1,'GROUP_PLANNED')`, id+"_e3", id); err != nil {
		refused = true
	}
	if !refused {
		t.Fatal("GROUP_PLANNED was accepted after the rollback; the CHECK was not restored")
	}

	if err := ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply 000118: %v", err)
	}
	for _, column := range []string{"group_index", "split_reason", "weight_grams"} {
		if !columnExists(column) {
			t.Fatalf("%s missing after reapply", column)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO vendor_orchestration_events(id, orchestration_id, plan_version, event_type, reason_code)
		VALUES($1,$2,1,'GROUP_PLANNED','GROUP_0_SOLO')`, id+"_e4", id); err != nil {
		t.Fatalf("GROUP_PLANNED refused after reapply: %v", err)
	}
}
