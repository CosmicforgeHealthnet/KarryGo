package repositories

import (
	"context"
	"cosmicforge/logistics/shared/go/commerceanalytics"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDispatchRouteIngestReplayPostgres(t *testing.T) {
	dsn := os.Getenv("ANALYTICS_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("ANALYTICS_TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("canonical Analytics test DB unavailable")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "dispatch_analytics_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, name := range []string{"001_commerce_analytics_reporting.sql", "002_dispatch_route_metrics.sql"} {
		raw, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", name))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	payload, _ := json.Marshal(commerceanalytics.DispatchRouteMetric{RouteVersion: 2, PolicyVersion: "dispatch_route_integrity_v1", State: "OPEN", AlertType: "STALE_LOCATION", Severity: "HIGH"})
	e := commerceanalytics.Envelope{EventID: uuid.NewString(), EventType: "dispatch.route_alert.opened.v1", SourceService: "driver-dispatch-delivery-service", SourceAggregate: "dispatch_route_metric", SourceVersion: 1, SchemaVersion: 1, OccurredAt: time.Now(), Payload: payload}
	e.SourceID = e.EventID
	repo := NewRepository(pool)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := repo.Ingest(ctx, e); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, ErrDuplicateEvent) {
			t.Fatal(err)
		}
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT event_count FROM analytics_dispatch_route_daily_metrics`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	e.Payload = json.RawMessage(`{"route_version":3,"policy_version":"dispatch_route_integrity_v1","state":"OPEN"}`)
	if _, err = repo.Ingest(ctx, e); err == nil || errors.Is(err, ErrDuplicateEvent) {
		t.Fatal("changed fingerprint accepted")
	}
}
