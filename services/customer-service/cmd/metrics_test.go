package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/observability"
)

// Customer V4 §27.2: every durable gauge query runs against the real migrated
// schema and publishes only bounded labels.
func TestCustomerGaugeQueriesRunAgainstMigratedSchema(t *testing.T) {
	dsn := os.Getenv("CUSTOMER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOT EXECUTED: CUSTOMER_TEST_DATABASE_URL (isolated disposable PostgreSQL) is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	entries, _ := os.ReadDir("../migrations")
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, _ := os.ReadFile(filepath.Join("../migrations", name))
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	m := observability.NewMetrics()
	if err = observability.CollectGauges(ctx, observability.PoolQuerier(pool), m, customerGaugeQueries); err != nil {
		t.Fatalf("gauge collection: %v", err)
	}
	if out := m.Render(); !strings.Contains(out, "karry_journeyshare_notification_backlog") || !strings.Contains(out, "karry_movement_rejections_24h") {
		t.Fatalf("durable gauges missing:\n%s", out)
	}
}
