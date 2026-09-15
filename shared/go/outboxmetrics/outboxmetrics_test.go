package outboxmetrics

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/observability"
)

// A table or column name cannot be bound as a query parameter, so it is
// interpolated. Today those names are compiled into each service; the
// validator is what keeps that safe if they ever become configurable.
func TestIdentifiersMustBePlain(t *testing.T) {
	base := Config{
		Service: "taxi-service", Table: "taxi_outbox",
		CreatedColumn: "occurred_at", DeliveredColumn: "delivered_at",
		AttemptsColumn: "attempts",
	}
	if err := base.Valid(); err != nil {
		t.Fatalf("a plain configuration was refused: %v", err)
	}

	for name, mutate := range map[string]func(Config) Config{
		"table":          func(c Config) Config { c.Table = "taxi_outbox; DROP TABLE trips"; return c },
		"created column": func(c Config) Config { c.CreatedColumn = "occurred_at)--"; return c },
		"quoted column":  func(c Config) Config { c.AttemptsColumn = `"attempts"`; return c },
		"uppercase":      func(c Config) Config { c.Table = "TaxiOutbox"; return c },
		"empty service":  func(c Config) Config { c.Service = ""; return c },
	} {
		if err := mutate(base).Valid(); err == nil {
			t.Fatalf("%s was accepted as an identifier", name)
		}
	}
}

// Every service must report the same metric names, because the point of the
// shared collector is one alert rule and one dashboard panel for the
// platform. A service-specific prefix would defeat that silently.
func TestMetricNamesAreServiceNeutral(t *testing.T) {
	for _, name := range []string{
		MetricPending, MetricOldestPendingAge, MetricRetrying,
		MetricDeadLetter, MetricAttempts,
		MetricReconciliationBacklog, MetricReconciliationOldestAge,
	} {
		for _, service := range []string{"taxi", "hauling", "dispatch", "wallet", "fleet", "vendor"} {
			if strings.Contains(name, service) {
				t.Fatalf("metric %q names a service; it must be labelled, not prefixed", name)
			}
		}
	}
}

func TestPublishUsesLowCardinalityLabels(t *testing.T) {
	metrics := observability.NewMetrics()
	config := Config{Service: "hauling-service", Target: "fleet-partner-service",
		Table: "hauling_domain_outbox", CreatedColumn: "occurred_at",
		DeliveredColumn: "delivered_at", AttemptsColumn: "attempts",
		TargetColumn: "target"}

	Publish(metrics, config, Snapshot{
		Pending: 3, OldestPendingAge: 90 * time.Second, Retrying: 1,
		DeadLettered: 2, PendingAttempts: 7,
		ReconciliationByState:   map[string]int64{"PENDING": 3, "RECONCILIATION_REQUIRED": 1},
		ReconciliationOldestAge: 600 * time.Second,
	})

	rendered := metrics.Render()
	for _, want := range []string{
		`outbox_pending_events{service="hauling-service",target="fleet-partner-service"} 3`,
		`outbox_oldest_pending_age_seconds{service="hauling-service",target="fleet-partner-service"} 90`,
		`outbox_dead_letter_events{service="hauling-service",target="fleet-partner-service"} 2`,
		`reconciliation_backlog_operations{service="hauling-service",state="RECONCILIATION_REQUIRED",target="fleet-partner-service"} 1`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in:\n%s", want, rendered)
		}
	}

	// A nil registry must be a no-op rather than a panic: a service that has
	// not wired metrics yet still calls the collector.
	Publish(nil, config, Snapshot{Pending: 1})
}

// A reconciliation state that empties must stop being reported. A gauge that
// keeps its last value forever turns a resolved backlog into a permanent
// alert, and an operator who has seen that once stops believing the next one.
func TestEmptiedReconciliationStateDisappears(t *testing.T) {
	metrics := observability.NewMetrics()
	config := Config{Service: "wallet", Table: "finance_outbox",
		CreatedColumn: "created_at", DeliveredColumn: "published_at",
		AttemptsColumn: "attempts"}

	Publish(metrics, config, Snapshot{
		ReconciliationByState: map[string]int64{"PENDING": 5, "FAILED_TERMINAL": 2}})
	if !strings.Contains(metrics.Render(), `state="FAILED_TERMINAL"`) {
		t.Fatal("the initial backlog was not reported")
	}

	Publish(metrics, config, Snapshot{
		ReconciliationByState: map[string]int64{"PENDING": 1}})
	rendered := metrics.Render()
	if strings.Contains(rendered, `state="FAILED_TERMINAL"`) {
		t.Fatalf("a resolved state is still reported:\n%s", rendered)
	}
	if !strings.Contains(rendered, `reconciliation_backlog_operations{service="wallet",state="PENDING"} 1`) {
		t.Fatalf("the remaining state was lost:\n%s", rendered)
	}
}

// collectPool connects to whichever service database the caller names.
func collectPool(t *testing.T, env string) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(env)
	if dsn == "" {
		t.Skipf("%s not set; outbox metric collection is infrastructure-gated", env)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err = pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

// The aggregate must run against the real Hauling outbox: a query that only
// compiles in a unit test proves nothing about the column names a live
// service actually has.
func TestCollectReadsARealOutbox(t *testing.T) {
	pool := collectPool(t, "HAULING_TEST_DATABASE_URL")
	ctx := context.Background()

	config := Config{
		Service: "driver-hauling-service", Table: "hauling_domain_outbox",
		Target: "fleet-partner-service", TargetColumn: "target",
		CreatedColumn: "occurred_at", DeliveredColumn: "delivered_at",
		DeadLetterColumn: "dead_lettered_at", AttemptsColumn: "attempts",
		ReconciliationColumn: "reconciliation_state",
	}

	snapshot, err := Collect(ctx, pool, config)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if snapshot.Pending < 0 || snapshot.DeadLettered < 0 {
		t.Fatalf("negative counts: %+v", snapshot)
	}

	counts, oldest, err := CollectStates(ctx, pool, config,
		[]string{"PENDING", "RETRY_SCHEDULED", "RECONCILIATION_REQUIRED"})
	if err != nil {
		t.Fatalf("collect states: %v", err)
	}
	if oldest < 0 {
		t.Fatalf("negative oldest age: %s", oldest)
	}
	for state := range counts {
		if strings.TrimSpace(state) == "" {
			t.Fatal("a blank reconciliation state was reported as a label")
		}
	}
}
