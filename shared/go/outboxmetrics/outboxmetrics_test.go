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

// OBS-H001. Every label this package emits must be a bounded breakdown. An
// identifier here would create one series per entity and grow the registry
// without bound for as long as the service runs; the first symptom is usually
// the monitoring system falling over rather than anything pointing back here.
func TestEmittedLabelKeysAreLowCardinality(t *testing.T) {
	config := Config{Service: "hauling-service", Target: "fleet-completed-work",
		Table: "hauling_domain_outbox", TargetColumn: "target",
		CreatedColumn: "occurred_at", DeliveredColumn: "delivered_at",
		AttemptsColumn: "attempts", ReconciliationColumn: "reconciliation_state"}

	// Rendered rather than introspected: the assertion is about what actually
	// reaches the registry, not about a helper that might not be the one
	// Publish uses.
	metrics := observability.NewMetrics()
	Publish(metrics, config, Snapshot{
		Pending: 1, OldestPendingAge: time.Minute,
		ReconciliationByState:   map[string]int64{"RECONCILIATION_REQUIRED": 1},
		ReconciliationOldestAge: time.Minute,
	})

	for _, line := range strings.Split(metrics.Render(), "\n") {
		open := strings.IndexByte(line, '{')
		close := strings.LastIndexByte(line, '}')
		if open < 0 || close < open {
			continue
		}
		for _, pair := range strings.Split(line[open+1:close], ",") {
			key := strings.TrimSpace(strings.SplitN(pair, "=", 2)[0])
			if !observability.LabelKeyIsLowCardinality(key) {
				t.Fatalf("high-cardinality label key %q emitted in: %s", key, line)
			}
		}
	}
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

// collectPool connects to a database this package may create tables in.
//
// It deliberately does NOT reach for a service DSN. shared/go owns no schema,
// so borrowing another service's database made these tests pass locally
// against a populated one and fail in CI against an empty one. The
// "does the compiled configuration match the real table" assertion now lives
// in each owning service, where the migrated schema genuinely exists; what is
// left here is the query builder, which can prove itself against a table it
// creates.
func collectPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; outbox metric collection is infrastructure-gated")
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

// fixtureOutbox creates a table shaped like the outboxes this package reads,
// scoped to one test and dropped afterwards.
func fixtureOutbox(t *testing.T, pool *pgxpool.Pool, table string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+table+` (
		id                   BIGSERIAL PRIMARY KEY,
		target               TEXT        NOT NULL,
		occurred_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
		delivered_at         TIMESTAMPTZ,
		dead_lettered_at     TIMESTAMPTZ,
		attempts             INTEGER     NOT NULL DEFAULT 0,
		reconciliation_state TEXT        NOT NULL DEFAULT 'PENDING'
	)`)
	if err != nil {
		t.Fatalf("create fixture outbox: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+table) })
}

// The aggregate must run against a real PostgreSQL table, not a hand-rolled
// row struct: the FILTER clauses, the EXTRACT and the NULL handling are where
// this query can be wrong, and none of that is exercised by compiling it.
func TestCollectReadsARealTable(t *testing.T) {
	pool := collectPool(t)
	ctx := context.Background()
	const table = "outboxmetrics_fixture_outbox"
	fixtureOutbox(t, pool, table)

	config := Config{
		Service: "fixture", Table: table,
		Target: "fleet-completed-work", TargetColumn: "target",
		CreatedColumn: "occurred_at", DeliveredColumn: "delivered_at",
		DeadLetterColumn: "dead_lettered_at", AttemptsColumn: "attempts",
		ReconciliationColumn: "reconciliation_state",
	}

	// An empty table must read as drained, not as an error and not as a
	// non-zero age: a service that has never enqueued anything is healthy.
	snapshot, err := Collect(ctx, pool, config)
	if err != nil {
		t.Fatalf("collect on an empty table: %v", err)
	}
	if snapshot.Pending != 0 || snapshot.DeadLettered != 0 || snapshot.OldestPendingAge != 0 {
		t.Fatalf("an empty outbox did not read as drained: %+v", snapshot)
	}

	_, err = pool.Exec(ctx, `INSERT INTO `+table+`
		(target, occurred_at, delivered_at, dead_lettered_at, attempts, reconciliation_state) VALUES
		($1, now() - interval '10 minutes', NULL, NULL, 0, 'PENDING'),
		($1, now() - interval '5 minutes',  NULL, NULL, 3, 'RETRY_SCHEDULED'),
		($1, now() - interval '1 minute',   NULL, now(), 7, 'FAILED_TERMINAL'),
		($1, now() - interval '2 hours',    now(), NULL, 1, 'DELIVERED'),
		($2, now() - interval '9 hours',    NULL, NULL, 0, 'PENDING')`,
		config.Target, "some-other-target")
	if err != nil {
		t.Fatalf("seed fixture rows: %v", err)
	}

	snapshot, err = Collect(ctx, pool, config)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	// Two pending, and the row for another target must not be counted: a
	// Fleet backlog and a Vendor backlog are different incidents.
	if snapshot.Pending != 2 {
		t.Fatalf("pending = %d, want 2 (the other target must not be counted)", snapshot.Pending)
	}
	// A dead-lettered row is not pending. It needs an operator, not patience.
	if snapshot.DeadLettered != 1 {
		t.Fatalf("dead lettered = %d, want 1", snapshot.DeadLettered)
	}
	if snapshot.Retrying != 1 {
		t.Fatalf("retrying = %d, want 1", snapshot.Retrying)
	}
	if snapshot.PendingAttempts != 3 {
		t.Fatalf("pending attempts = %d, want 3", snapshot.PendingAttempts)
	}
	// The oldest PENDING row is 10 minutes old. The 9-hour row belongs to
	// another target and the 2-hour row is delivered; neither may set the age.
	if snapshot.OldestPendingAge < 9*time.Minute || snapshot.OldestPendingAge > 30*time.Minute {
		t.Fatalf("oldest pending age = %s, want about 10 minutes", snapshot.OldestPendingAge)
	}

	counts, oldest, err := CollectStates(ctx, pool, config,
		[]string{"PENDING", "RETRY_SCHEDULED", "RECONCILIATION_REQUIRED"})
	if err != nil {
		t.Fatalf("collect states: %v", err)
	}
	for state := range counts {
		if strings.TrimSpace(state) == "" {
			t.Fatal("a blank reconciliation state was reported as a label")
		}
	}
	if counts["DELIVERED"] != 1 {
		t.Fatalf("a resolved state must still be counted and labelled: %v", counts)
	}
	// DELIVERED is two hours old but is not declared unresolved, so it must
	// not set the oldest-case age -- otherwise the metric is permanently red
	// and therefore permanently ignored.
	if oldest < 9*time.Minute || oldest > 30*time.Minute {
		t.Fatalf("oldest unresolved age = %s; a resolved state set the age", oldest)
	}
}
