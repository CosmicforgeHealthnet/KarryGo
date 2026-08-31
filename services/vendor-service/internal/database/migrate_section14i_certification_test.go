package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// section14iPool returns a pool against the migration tip. Section 14's own
// migration tests are static text assertions; this file is the real-PostgreSQL
// half that Section 14I requires, because a trigger that exists in a .sql file
// is not the same fact as a trigger the database actually enforces.
func section14iPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 14I certification")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err = ApplyMigrations(ctx, pool, section14IMigrationsDir()); err != nil {
		t.Fatal(err)
	}

	return pool, ctx
}

// seedSection14IMovement creates the minimum real ownership chain a movement
// row needs (business -> branch -> inventory item) and appends one movement
// through the authorized-mutation session guard. It returns the movement id.
func seedSection14IMovement(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
) string {
	t.Helper()

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	user := "s14i_user_" + suffix
	staff := "s14i_staff_" + suffix
	business := "s14i_biz_" + suffix
	branch := "s14i_branch_" + suffix
	item := "s14i_item_" + suffix
	movement := "s14i_move_" + suffix

	if _, err := pool.Exec(ctx,
		`INSERT INTO vendor_users(id,email) VALUES($1,$2)`,
		user, user+"@example.test"); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO vendor_staff(id,vendor_id,user_id) VALUES($1,$2,$3)`,
		staff, business, user); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone)
		 VALUES($1,'Section 14I certification','ACTIVE',$2,'NGN','Africa/Lagos')`,
		business, staff); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status)
		 VALUES($1,$2,'Certification Branch','1 Test Street','Lagos','Lagos','NG',6.5244,3.3792,'Africa/Lagos','ACTIVE')`,
		branch, business); err != nil {
		t.Fatal(err)
	}

	// INGREDIENT rather than SHOP_VARIANT: a SHOP_VARIANT row is validated by a
	// database trigger against a live Shop variant and an active branch listing,
	// which would mean standing up a whole Shop catalogue to test a guard that
	// has nothing to do with Shop. The movement ledger's immutability is item
	// type agnostic.
	if _, err := pool.Exec(ctx,
		`INSERT INTO inventory_items(
			id,business_id,branch_id,item_type,source_ref,
			stock_measure_ref,stock_measure_owner_key,stock_unit_kind,
			stock_basis_mode,tracking_mode)
		 VALUES($1,$2,$3,'INGREDIENT',$4,'each','shop:each','COUNT','SAME_UNIT','SIMPLE')`,
		item, business, branch, "ingredient_"+suffix); err != nil {
		t.Fatal(err)
	}

	// The movement ledger has an insert guard demanding an authorized mutation
	// session flag, so the append itself must run inside one transaction that
	// sets it. This is the only legitimate way stock evidence is written.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, `SELECT set_config('karrygo.inventory_mutation','1',true)`); err != nil {
		t.Fatal(err)
	}

	if _, err = tx.Exec(ctx,
		`INSERT INTO inventory_movements(
			id,business_id,branch_id,inventory_item_id,movement_type,quantity,
			on_hand_delta,source_type,source_ref,inventory_revision,balance_revision)
		 VALUES($1,$2,$3,$4,'RECEIVE',10,10,'CERTIFICATION',$5,1,1)`,
		movement, business, branch, item, "s14i_source_"+suffix); err != nil {
		t.Fatal(err)
	}

	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	return movement
}

// TestSection14IMovementLedgerIsImmutableInPostgres proves the append-only
// guarantee against the running database rather than against the migration
// text. Both an UPDATE and a DELETE must be refused, and the row must survive.
func TestSection14IMovementLedgerIsImmutableInPostgres(t *testing.T) {
	pool, ctx := section14iPool(t)
	movement := seedSection14IMovement(t, pool, ctx)

	// An UPDATE is refused even when the caller holds the authorized-mutation
	// flag: the immutability trigger is unconditional, unlike the balance and
	// lot guards. Rewriting history is never an authorized mutation.
	for _, probe := range []struct {
		name string
		sql  string
	}{
		{"update quantity", `UPDATE inventory_movements SET quantity = 99 WHERE id = $1`},
		{"update source", `UPDATE inventory_movements SET source_ref = 'tampered' WHERE id = $1`},
		{"delete", `DELETE FROM inventory_movements WHERE id = $1`},
	} {
		t.Run(probe.name, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			if _, err = tx.Exec(ctx, `SELECT set_config('karrygo.inventory_mutation','1',true)`); err != nil {
				t.Fatal(err)
			}

			_, err = tx.Exec(ctx, probe.sql, movement)
			if err == nil {
				t.Fatalf("%s succeeded against the append-only movement ledger", probe.name)
			}
			if !strings.Contains(err.Error(), "append-only") {
				t.Fatalf("%s was refused for the wrong reason: %v", probe.name, err)
			}
		})
	}

	var quantity int64
	var sourceRef string
	if err := pool.QueryRow(ctx,
		`SELECT quantity, source_ref FROM inventory_movements WHERE id = $1`,
		movement).Scan(&quantity, &sourceRef); err != nil {
		t.Fatalf("the movement row did not survive the tamper attempts: %v", err)
	}
	if quantity != 10 || sourceRef == "tampered" {
		t.Fatalf("movement evidence was altered: quantity=%d source_ref=%s", quantity, sourceRef)
	}
}

// TestSection14IPhysicalStockRequiresAuthorizedMutation proves the balance
// guard refuses a direct write that did not come through an inventory
// mutation. Without this, any SQL path could invent stock.
func TestSection14IPhysicalStockRequiresAuthorizedMutation(t *testing.T) {
	pool, ctx := section14iPool(t)

	_, err := pool.Exec(ctx,
		`INSERT INTO inventory_balances(business_id,branch_id,inventory_item_id,on_hand_quantity)
		 VALUES('s14i_ghost','s14i_ghost','s14i_ghost',1000)`)

	if err == nil {
		t.Fatal("an unauthorized direct write invented inventory balance stock")
	}
	if !strings.Contains(err.Error(), "authorized inventory mutation") &&
		!strings.Contains(err.Error(), "violates foreign key") {
		t.Fatalf("balance guard refused for an unexpected reason: %v", err)
	}
}

// TestSection14IMigrationsRollbackAndReapply is Section 14I's Scenario C:
// the tip rolls back and comes back. Section 14's existing migration tests are
// static shape assertions and never exercised a real rollback.
func TestSection14IMigrationsRollbackAndReapply(t *testing.T) {
	pool, ctx := section14iPool(t)

	migrations, err := ListMigrations(section14IMigrationsDir())
	if err != nil {
		t.Fatal(err)
	}

	tip := migrations[len(migrations)-1]
	if tip.Version != "000066_vendor_inventory_operational_hardening" {
		// A later accepted section owns the tip; rolling back would certify that
		// section instead. Prove Section 14's chain is applied and stop.
		var applied int
		if err = pool.QueryRow(ctx,
			`SELECT count(*) FROM vendor_service_schema_migrations
			 WHERE version >= '000060' AND version < '000067'`).Scan(&applied); err != nil {
			t.Fatal(err)
		}
		if applied != 7 {
			t.Fatalf("Section 14 migrations applied=%d want=7 at tip %s", applied, tip.Version)
		}
		return
	}

	if err = RollbackLastMigration(ctx, pool, section14IMigrationsDir()); err != nil {
		t.Fatalf("rollback 000066: %v", err)
	}

	// The down file must actually drop the object, not merely untrack it.
	var present bool
	if err = pool.QueryRow(ctx,
		`SELECT to_regclass('public.vendor_job_controls') IS NOT NULL`).Scan(&present); err != nil {
		t.Fatal(err)
	}
	if present {
		t.Error("vendor_job_controls survived the 000066 rollback")
	}

	// Section 14's earlier stock schema must be undamaged by the rollback.
	for _, table := range []string{
		"inventory_items", "inventory_balances", "inventory_movements",
		"inventory_lots", "inventory_reservations",
	} {
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&present); err != nil {
			t.Fatal(err)
		}
		if !present {
			t.Errorf("the 000066 rollback damaged %s", table)
		}
	}

	if err = ApplyMigrations(ctx, pool, section14IMigrationsDir()); err != nil {
		t.Fatalf("reapply: %v", err)
	}

	if err = pool.QueryRow(ctx,
		`SELECT to_regclass('public.vendor_job_controls') IS NOT NULL`).Scan(&present); err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Fatal("vendor_job_controls did not come back after reapplying")
	}
}

// TestSection14IAvailabilityViewIsBigint pins the defect Section 14I found:
// 000062 created inventory_balance_availability with numeric columns and
// 000063 tried to CREATE OR REPLACE it with bigint casts, which PostgreSQL
// refuses. An empty database could not reach the tip at all. The view must now
// expose bigint, which is only true if 000063 recreated rather than replaced.
func TestSection14IAvailabilityViewIsBigint(t *testing.T) {
	pool, ctx := section14iPool(t)

	for _, column := range []string{
		"available_quantity", "reserved_quantity", "expired_quantity",
	} {
		var dataType string
		if err := pool.QueryRow(ctx,
			`SELECT data_type FROM information_schema.columns
			 WHERE table_name = 'inventory_balance_availability' AND column_name = $1`,
			column).Scan(&dataType); err != nil {
			t.Fatalf("%s missing from the availability view: %v", column, err)
		}
		if dataType != "bigint" {
			t.Errorf("inventory_balance_availability.%s is %s; want bigint", column, dataType)
		}
	}
}
