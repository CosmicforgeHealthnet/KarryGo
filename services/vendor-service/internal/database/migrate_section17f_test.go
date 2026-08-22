package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection17FMigrationApplyRollbackReapply(t *testing.T) {
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
		"commerce_payment_operations", "commerce_payment_legs", "commerce_payment_events",
		"commerce_order_changes", "commerce_order_adjustments", "commerce_order_change_events",
	}
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("%s exists=%t err=%v", table, exists, err)
		}
	}

	// The order carries a mirrored payment state, so no reader has to join to
	// learn whether an order may be worked.
	var hasPaymentState bool
	if err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			 WHERE table_name='commerce_parent_orders' AND column_name='payment_state')
		`).Scan(&hasPaymentState); err != nil || !hasPaymentState {
		t.Fatalf("commerce_parent_orders has no payment_state: %v", err)
	}

	// Exactly-once payment per order and per quote are database constraints,
	// not conventions.
	for _, column := range []string{"parent_order_id", "quote_id"} {
		var unique bool
		if err = pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM pg_constraint c
				 WHERE c.conrelid='commerce_payment_operations'::regclass
				   AND c.contype='u'
				   AND (SELECT array_agg(attname::text ORDER BY attname)
				          FROM pg_attribute
				         WHERE attrelid=c.conrelid AND attnum=ANY(c.conkey)) = ARRAY[$1])
			`, column).Scan(&unique); err != nil || !unique {
			t.Fatalf("commerce_payment_operations has no UNIQUE(%s): %v", column, err)
		}
	}

	// A leg's idempotency key is unique, so a retried start can never produce
	// a second financial effect.
	var uniqueKey bool
	if err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_constraint c
			 WHERE c.conrelid='commerce_payment_legs'::regclass
			   AND c.contype='u'
			   AND (SELECT array_agg(attname::text ORDER BY attname)
			          FROM pg_attribute
			         WHERE attrelid=c.conrelid AND attnum=ANY(c.conkey)) = ARRAY['idempotency_key'])
		`).Scan(&uniqueKey); err != nil || !uniqueKey {
		t.Fatalf("commerce_payment_legs has no UNIQUE(idempotency_key): %v", err)
	}

	// Payment evidence is frozen; the operation and leg rows are not, because
	// their status is what an authority answer changes.
	var evidenceTriggers, mutableTriggers int
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_trigger
		  WHERE NOT tgisinternal AND tgname='commerce_payment_events_immutable'`).
		Scan(&evidenceTriggers); err != nil || evidenceTriggers != 1 {
		t.Fatalf("payment evidence trigger=%d err=%v", evidenceTriggers, err)
	}
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_trigger
		  WHERE NOT tgisinternal
		    AND tgrelid IN ('commerce_payment_operations'::regclass,
		                    'commerce_payment_legs'::regclass)
		    AND tgname LIKE '%immutable%'`).Scan(&mutableTriggers); err != nil ||
		mutableTriggers != 0 {
		t.Fatalf("payment aggregates were frozen: triggers=%d err=%v", mutableTriggers, err)
	}

	rollbackUntilRemoved(t, ctx, pool, "000081_vendor_commerce_payment_binding")
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("after rollback %s exists=%t err=%v", table, exists, err)
		}
	}
	// The rollback must also remove the column it added to Section 17E's table.
	if err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			 WHERE table_name='commerce_parent_orders' AND column_name='payment_state')
		`).Scan(&hasPaymentState); err != nil || hasPaymentState {
		t.Fatalf("rollback left payment_state behind: %v", err)
	}

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply: %v", err)
	}
}

// Section 17F binds payment; it does not become a ledger. Vendor Commerce is
// not a money authority and its own schema must not start looking like one.
func TestSection17FMigrationCreatesNoLedger(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/000081_vendor_commerce_payment_binding.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))

	for _, forbidden := range []string{
		"create table commerce_ledger",
		"create table commerce_ledger_entries",
		"create table commerce_wallet",
		"create table commerce_accounts",
		"create table commerce_balances",
		"balance_minor",
		"available_minor",
	} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("Section 17F started building a competing ledger: %s", forbidden)
		}
	}

	// The authority's own answer must be storable verbatim, so a Vendor-side
	// reading can always be checked against it.
	if !strings.Contains(lower, "authority_status") {
		t.Error("payment legs do not record the authority's raw status")
	}
}
