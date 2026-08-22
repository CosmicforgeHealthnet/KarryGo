package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection17DMigrationApplyRollbackReapply(t *testing.T) {
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
		"commerce_quotes", "commerce_quote_groups", "commerce_quote_lines",
		"commerce_quote_missions", "commerce_quote_adjustments",
		"commerce_quote_approvals", "commerce_quote_events",
	}
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("%s exists=%t err=%v", table, exists, err)
		}
	}
	var triggers int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname LIKE 'commerce_quote%_immutable'`).Scan(&triggers); err != nil || triggers != 7 {
		t.Fatalf("immutable triggers=%d want 7 err=%v", triggers, err)
	}

	rollbackUntilRemoved(t, ctx, pool, "000079_vendor_immutable_checkout_quote")
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("after rollback %s exists=%t err=%v", table, exists, err)
		}
	}
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply: %v", err)
	}
}

func TestSection17DMigrationContainsNoPaymentOrOrderCreation(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/000079_vendor_immutable_checkout_quote.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"create table commerce_payments", "create table parent_orders", "create table vendor_orders", "wallet_authorization"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("Section 17D migration crossed into 17E: %s", forbidden)
		}
	}
}
