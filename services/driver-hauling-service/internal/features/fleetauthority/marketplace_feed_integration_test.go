package fleetauthority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/database"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Sections 11-12: Hauling announces its open requests to Fleet as marketplace
// opportunities on the existing Fleet event outbox. Proven against real
// PostgreSQL (migration 034 triggers) and the real delivery worker.

func feedPool(t *testing.T, feedOn bool) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required for the Fleet marketplace feed tests")
	}
	var options []database.PoolOption
	if feedOn {
		options = append(options, database.WithSessionSetting(database.FleetMarketplaceFeedSetting, "on"))
	}
	pool, err := database.NewPool(context.Background(), dsn, options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, sourceFile, _, _ := runtime.Caller(0)
	serviceRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(serviceRoot); err != nil {
		t.Fatal(err)
	}
	if err = database.ApplyMigrations(context.Background(), pool); err != nil {
		_ = os.Chdir(previous)
		t.Fatalf("apply migrations: %v", err)
	}
	if err = os.Chdir(previous); err != nil {
		t.Fatal(err)
	}
	return pool
}

func insertBooking(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `INSERT INTO haulage_bookings
		(id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,
		 cargo_type,cargo_weight_kg,helper_count,fare_estimate_kobo,status)
		VALUES($1,gen_random_uuid(),'12 Private Close',6.52,3.37,'9 Secret Way',6.43,3.42,'general',
		 750,2,1250000,'pending_match')`, id); err != nil {
		t.Fatalf("insert booking: %v", err)
	}
	return id
}

type feedRow struct {
	EventType string
	Version   int64
	Payload   string
	Target    string
}

func feedRows(t *testing.T, pool *pgxpool.Pool, bookingID string) []feedRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT event_type,event_version,payload::text,target
		FROM hauling_domain_outbox WHERE aggregate_type='REQUEST' AND aggregate_id=$1
		ORDER BY event_version`, bookingID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []feedRow
	for rows.Next() {
		var row feedRow
		if err := rows.Scan(&row.EventType, &row.Version, &row.Payload, &row.Target); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	return out
}

func TestFleetMarketplaceFeedPublishesOpenRequestLifecycle(t *testing.T) {
	pool := feedPool(t, true)
	ctx := context.Background()

	// Created open -> offered at the booking's current version.
	open := insertBooking(t, pool)
	rows := feedRows(t, pool, open)
	if len(rows) != 1 || rows[0].EventType != "platform.request.offered.v1" || rows[0].Version != 1 ||
		rows[0].Target != "fleet-events" {
		t.Fatalf("offered rows = %+v", rows)
	}
	// Allowlisted payload: no address, contact, fare or payment field.
	for _, forbidden := range []string{"Private Close", "Secret Way", "fare", "1250000", "payment", "customer"} {
		if strings.Contains(rows[0].Payload, forbidden) {
			t.Fatalf("offered payload leaks %q: %s", forbidden, rows[0].Payload)
		}
	}
	for _, want := range []string{`"requestId": "` + open + `"`, `"serviceType": "hauling"`, `"weightKg": 750`} {
		if !strings.Contains(rows[0].Payload, want) {
			t.Fatalf("offered payload missing %s: %s", want, rows[0].Payload)
		}
	}

	// Internal matching moves (pending_match <-> awaiting_acceptance) are not
	// marketplace facts and publish nothing.
	if _, err := pool.Exec(ctx, `UPDATE haulage_bookings SET status='awaiting_acceptance' WHERE id=$1`, open); err != nil {
		t.Fatal(err)
	}
	if got := len(feedRows(t, pool, open)); got != 1 {
		t.Fatalf("matching move published %d rows", got)
	}

	// Taken by an independent provider -> updated(accepted) at version 2.
	if _, err := pool.Exec(ctx, `UPDATE haulage_bookings SET status='accepted' WHERE id=$1`, open); err != nil {
		t.Fatal(err)
	}
	rows = feedRows(t, pool, open)
	if len(rows) != 2 || rows[1].EventType != "platform.request.updated.v1" || rows[1].Version != 2 ||
		!strings.Contains(rows[1].Payload, `"status": "accepted"`) {
		t.Fatalf("accepted rows = %+v", rows)
	}
	var version int64
	if err := pool.QueryRow(ctx, `SELECT source_version FROM haulage_bookings WHERE id=$1`, open).Scan(&version); err != nil || version != 2 {
		t.Fatalf("booking source_version = %d, %v; want 2", version, err)
	}

	// An organization-won booking moving to accepted is NOT an independent
	// claim: Fleet already tracks it through reservation and trip events.
	won := insertBooking(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE haulage_bookings SET winner_organization_id=gen_random_uuid(),
		source_version=source_version+1 WHERE id=$1`, won); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE haulage_bookings SET status='accepted' WHERE id=$1`, won); err != nil {
		t.Fatal(err)
	}
	if got := len(feedRows(t, pool, won)); got != 1 {
		t.Fatalf("organization-won acceptance published %d rows, want only the offer", got)
	}

	// Cancelled and unmatched close the request; a statement that already
	// advanced the version is not advanced twice.
	cancelled := insertBooking(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE haulage_bookings SET status='cancelled',source_version=source_version+1 WHERE id=$1`, cancelled); err != nil {
		t.Fatal(err)
	}
	rows = feedRows(t, pool, cancelled)
	if len(rows) != 2 || rows[1].EventType != "platform.request.cancelled.v1" || rows[1].Version != 2 {
		t.Fatalf("cancelled rows = %+v", rows)
	}
	unmatched := insertBooking(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE haulage_bookings SET status='unmatched' WHERE id=$1`, unmatched); err != nil {
		t.Fatal(err)
	}
	if rows = feedRows(t, pool, unmatched); len(rows) != 2 || rows[1].EventType != "platform.request.expired.v1" {
		t.Fatalf("unmatched rows = %+v", rows)
	}

	// The existing worker delivers the rows to Fleet's event endpoint as the
	// signed envelope Fleet ingests.
	var mu sync.Mutex
	var delivered []map[string]any
	fleet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get(serviceauth.HeaderServiceName) != "driver-hauling-service" ||
			r.Header.Get(serviceauth.HeaderSignature) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var envelope map[string]any
		_ = json.Unmarshal(body, &envelope)
		mu.Lock()
		delivered = append(delivered, envelope)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer fleet.Close()
	worker := &OutboxWorker{DB: pool, URL: fleet.URL + "/internal/fleet-partner/v1/events",
		Secret: []byte("feed-secret"), Client: &http.Client{Timeout: 5 * time.Second}}
	for range 3 {
		worker.RunBatchForTest(ctx)
	}
	mu.Lock()
	defer mu.Unlock()
	var offered map[string]any
	for _, envelope := range delivered {
		if envelope["eventType"] == "platform.request.offered.v1" && envelope["aggregateId"] == open {
			offered = envelope
		}
	}
	if offered == nil {
		t.Fatalf("offered event for %s not delivered; delivered=%d", open, len(delivered))
	}
	if offered["sourceService"] != "driver-hauling-service" || offered["aggregateType"] != "request" ||
		offered["aggregateVersion"] != float64(1) {
		t.Fatalf("offered envelope = %v", offered)
	}
	// Fleet recomputes the hash from the payload object it decoded
	// (json.Marshal of the map: sorted keys, compact). The declared hash must
	// match exactly or Fleet rejects the event as payload_hash_mismatch.
	encoded, _ := json.Marshal(offered["payload"])
	sum := sha256.Sum256(encoded)
	if offered["payloadHash"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("payloadHash %v does not match Fleet's recomputation %s", offered["payloadHash"],
			hex.EncodeToString(sum[:]))
	}
}

func TestFleetMarketplaceFeedSilentWhenDisabled(t *testing.T) {
	pool := feedPool(t, false)
	ctx := context.Background()
	id := insertBooking(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE haulage_bookings SET status='cancelled' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if rows := feedRows(t, pool, id); len(rows) != 0 {
		t.Fatalf("disabled feed published %+v", rows)
	}
	var version int64
	if err := pool.QueryRow(ctx, `SELECT source_version FROM haulage_bookings WHERE id=$1`, id).Scan(&version); err != nil || version != 1 {
		t.Fatalf("disabled feed moved source_version to %d (%v)", version, err)
	}
}
