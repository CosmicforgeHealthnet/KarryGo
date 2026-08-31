package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection17FFulfilmentMigrationApplyRollbackReapply(t *testing.T) {
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
		t.Fatal(err)
	}

	tables := []string{"commerce_order_missions", "commerce_order_mission_allocations"}
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("%s exists=%t err=%v", table, exists, err)
		}
	}

	// Stock finalization is explicit, never inferred.
	var hasInventoryState bool
	if err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			 WHERE table_name='commerce_vendor_orders' AND column_name='inventory_state')
		`).Scan(&hasInventoryState); err != nil || !hasInventoryState {
		t.Fatalf("commerce_vendor_orders has no inventory_state: %v", err)
	}

	// A retried activation can never dispatch a second vehicle for the same
	// delivery: both keys are database constraints.
	for table, columns := range map[string][]string{
		"commerce_order_missions":            {"mission_ref"},
		"commerce_order_mission_allocations": {"vendor_order_id"},
	} {
		var unique bool
		if err = pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM pg_constraint c
				 WHERE c.conrelid=$1::regclass
				   AND c.contype='u'
				   AND (SELECT array_agg(attname::text ORDER BY attname)
				          FROM pg_attribute
				         WHERE attrelid=c.conrelid AND attnum=ANY(c.conkey)) = $2)
			`, table, columns).Scan(&unique); err != nil || !unique {
			t.Fatalf("%s has no UNIQUE%v: %v", table, columns, err)
		}
	}

	// Only dispatch and hauling may execute a commerce mission. Taxi is not a
	// value this column can hold.
	var taxiRejected bool
	if err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_constraint
			 WHERE conrelid='commerce_order_missions'::regclass
			   AND contype='c'
			   AND pg_get_constraintdef(oid) LIKE '%DISPATCH%'
			   AND pg_get_constraintdef(oid) LIKE '%HAULING%'
			   AND pg_get_constraintdef(oid) NOT LIKE '%TAXI%')
		`).Scan(&taxiRejected); err != nil || !taxiRejected {
		t.Fatalf("commerce_order_missions does not restrict the executor to dispatch/hauling: %v", err)
	}

	rollbackUntilRemoved(t, ctx, pool, "000082_vendor_commerce_fulfilment_activation")
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("after rollback %s exists=%t err=%v", table, exists, err)
		}
	}
	if err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			 WHERE table_name='commerce_vendor_orders' AND column_name='inventory_state')
		`).Scan(&hasInventoryState); err != nil || hasInventoryState {
		t.Fatalf("rollback left inventory_state behind: %v", err)
	}

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply: %v", err)
	}
}

// Section 17F executes deliveries through the logistics authorities. It must
// not start keeping its own rider, vehicle or trip state.
func TestSection17FFulfilmentMigrationBuildsNoLogisticsState(t *testing.T) {
	raw, err := os.ReadFile(
		"../../migrations/000082_vendor_commerce_fulfilment_activation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))

	for _, forbidden := range []string{
		"create table commerce_riders",
		"create table commerce_vehicles",
		"create table commerce_trips",
		"create table commerce_provider",
		"rider_id",
		"driver_id",
	} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("Section 17F started keeping logistics execution state: %s", forbidden)
		}
	}

	// Taxi must not be a VALUE the schema accepts. Naming it in a comment is
	// the point - the exclusion is deliberate and documented - so only quoted
	// SQL literals are checked here.
	for _, literal := range []string{"'taxi'", `"taxi"`} {
		if strings.Contains(lower, literal) {
			t.Errorf("Taxi is an accepted value in the commerce mission schema: %s", literal)
		}
	}
}
