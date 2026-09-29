package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	messageclients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	"cosmicforge/logistics/shared/go/observability"
)

// Customer V4 §27.2: Notification gauges run against the migrated schema and
// report dead letters by channel with bounded labels only.
func TestNotificationGaugeQueriesRunAgainstMigratedSchema(t *testing.T) {
	dsn := os.Getenv("NOTIFICATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOT EXECUTED: NOTIFICATION_TEST_DATABASE_URL is required")
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
	message := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO notification_messages(id,idempotency_key,source_service,event_type,recipient_type,recipient_id,title,body)
		VALUES($1,$2,'driver-taxi-service','ride.assigned','customer','customer-1','t','b')`, message, message); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO notification_deliveries(id,message_id,channel,status,attempts) VALUES($1,$2,'websocket','dead_lettered',5)`,
		uuid.NewString(), message); err != nil {
		t.Fatal(err)
	}
	m := observability.NewMetrics()
	if err = observability.CollectGauges(ctx, observability.PoolQuerier(pool), m, notificationGaugeQueries); err != nil {
		t.Fatalf("gauge collection: %v", err)
	}
	m.SetGauge("karry_notification_websocket_sessions", nil, float64(messageclients.NewWebSocketHub().ActiveSessions()))
	out := m.Render()
	if !strings.Contains(out, `karry_notification_dead_letters{channel="websocket"}`) || !strings.Contains(out, "karry_notification_websocket_sessions 0") {
		t.Fatalf("notification gauges missing:\n%s", out)
	}
	if strings.Contains(out, "customer-1") {
		t.Fatal("a recipient reached a metric label")
	}
}
