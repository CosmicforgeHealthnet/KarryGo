package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/media-file-service/internal/database"
	"cosmicforge/logistics/shared/go/observability"
)

// Customer V4 §27.2: every durable Media gauge runs against the migrated schema.
func TestMediaGaugeQueriesRunAgainstMigratedSchema(t *testing.T) {
	dsn := os.Getenv("MEDIA_FILE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOT EXECUTED: MEDIA_FILE_TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// Like the integration suite, bring the disposable database to the
	// migration tip itself rather than depending on package order.
	if err = database.ApplyMigrations(context.Background(), pool, "../migrations"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	m := observability.NewMetrics()
	if err = observability.CollectGauges(context.Background(), observability.PoolQuerier(pool), m, mediaGaugeQueries); err != nil {
		t.Fatalf("gauge collection: %v", err)
	}
	if strings.Contains(m.Render(), "karry_metrics_collection_errors_total") {
		t.Fatal("a Media gauge family failed")
	}
}
