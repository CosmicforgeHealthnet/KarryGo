package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection17EMigrationApplyRollbackReapply(t *testing.T) {
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

	tables := []string{
		"commerce_parent_orders", "commerce_vendor_orders",
		"commerce_vendor_order_lines", "commerce_order_allocations",
		"commerce_order_events",
	}
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("%s exists=%t err=%v", table, exists, err)
		}
	}

	// Snapshots, allocations and lifecycle evidence are frozen at the
	// database. The two aggregate rows are deliberately not, because their
	// status is exactly what a lifecycle transition changes.
	var triggers int
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_trigger
		  WHERE NOT tgisinternal AND tgname IN (
		      'commerce_vendor_order_lines_immutable',
		      'commerce_order_allocations_immutable',
		      'commerce_order_events_immutable')`).Scan(&triggers); err != nil || triggers != 3 {
		t.Fatalf("immutable triggers=%d want 3 err=%v", triggers, err)
	}

	var aggregateTriggers int
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_trigger
		  WHERE NOT tgisinternal
		    AND tgrelid IN ('commerce_parent_orders'::regclass,'commerce_vendor_orders'::regclass)
		    AND tgname LIKE '%immutable%'`).Scan(&aggregateTriggers); err != nil ||
		aggregateTriggers != 0 {
		t.Fatalf("aggregate rows were frozen: triggers=%d err=%v", aggregateTriggers, err)
	}

	// One approved quote can only ever produce one ParentOrder. The
	// idempotency guarantee is a database constraint, not a convention.
	var uniqueQuote bool
	if err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_constraint c
			 WHERE c.conrelid='commerce_parent_orders'::regclass
			   AND c.contype='u'
			   AND (SELECT array_agg(attname::text ORDER BY attname)
			          FROM pg_attribute
			         WHERE attrelid=c.conrelid AND attnum=ANY(c.conkey)) = ARRAY['quote_id'])
		`).Scan(&uniqueQuote); err != nil || !uniqueQuote {
		t.Fatalf("commerce_parent_orders has no UNIQUE(quote_id): %v", err)
	}

	rollbackUntilRemoved(t, ctx, pool, "000080_vendor_commerce_orders")
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("after rollback %s exists=%t err=%v", table, exists, err)
		}
	}
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply: %v", err)
	}
}

// Section 17E creates orders. It does not finish payment execution, which is a
// later section's work, so its own migration must not quietly introduce a
// payment or settlement ledger.
func TestSection17EMigrationCreatesNoPaymentLedger(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/000080_vendor_commerce_orders.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))

	for _, forbidden := range []string{
		"create table commerce_payments",
		"create table commerce_payment_intents",
		"create table commerce_settlements",
		"create table commerce_refunds",
		"create table commerce_ledger",
		"wallet_authorization",
	} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("Section 17E migration crossed into payment execution: %s", forbidden)
		}
	}

	// The payment reference column may exist, because a later section binds an
	// operation to an order that already exists. It must not be populated here.
	if !strings.Contains(lower, "payment_operation_ref text not null default ''") {
		t.Error("payment_operation_ref must default to empty, never to a fabricated reference")
	}
}
