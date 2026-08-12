package observability

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestObservePoolStats_RecordsBoundedGauges(t *testing.T) {
	rawURL := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL not set; skipping DB pool metrics test")
	}
	pool, err := pgxpool.New(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	metrics := NewMetrics()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { ObservePoolStats(ctx, metrics, pool, time.Millisecond); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	rendered := metrics.Render()
	for _, name := range []string{
		"fleet_partner_db_pool_total_conns",
		"fleet_partner_db_pool_acquired_conns",
		"fleet_partner_db_pool_idle_conns",
		"fleet_partner_db_pool_max_conns",
	} {
		if !strings.Contains(rendered, name) {
			t.Fatalf("expected metric %s to be recorded, got:\n%s", name, rendered)
		}
	}
}

func TestObservePoolStats_StopsOnContextCancellation(t *testing.T) {
	rawURL := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL not set; skipping DB pool metrics test")
	}
	pool, err := pgxpool.New(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { ObservePoolStats(ctx, NewMetrics(), pool, time.Second); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ObservePoolStats did not return promptly after context cancellation")
	}
}
