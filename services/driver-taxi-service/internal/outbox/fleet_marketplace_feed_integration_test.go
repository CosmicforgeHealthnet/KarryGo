package outbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/taxi-service/internal/database"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Sections 11-12: Taxi publishes its open ride requests and the lifecycle of
// organization-run trips to Fleet on the existing outbox (migration 029
// triggers + FleetMarketplaceEventDestination). Real PostgreSQL.

func feedDB(t *testing.T, feedOn bool) *pgxpool.Pool {
	t.Helper()
	migrated := testDB(t) // applies migrations once
	_ = migrated
	var options []database.PoolOption
	if feedOn {
		options = append(options, database.WithSessionSetting(database.FleetMarketplaceFeedSetting, "on"))
	}
	pool, err := database.NewPool(context.Background(), os.Getenv("TAXI_TEST_DATABASE_URL"), options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type feedFixture struct {
	db           *pgxpool.Pool
	market, zone string
	provider     string
}

func newFeedFixture(t *testing.T, db *pgxpool.Pool) feedFixture {
	t.Helper()
	ctx := context.Background()
	f := feedFixture{db: db, market: uuid.NewString(), zone: uuid.NewString(), provider: uuid.NewString()}
	code := "FMF-" + uuid.NewString()[:8]
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO markets(id,country,name,code) VALUES($1,'NG','Feed',$2)`, []any{f.market, code}},
		{`INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng,match_radius_km,supported_classes) VALUES($1,$2,'Feed',$3,6.5,3.3,5,'{economy}')`, []any{f.zone, f.market, code}},
		{`INSERT INTO taxi_providers(id,phone) VALUES($1,$2)`, []any{f.provider, "+234" + f.provider[:10]}},
	} {
		if _, err := db.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return f
}

func (f feedFixture) booking(t *testing.T) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := f.db.Exec(context.Background(), `INSERT INTO taxi_bookings
		(id,customer_id,market_id,operating_zone_id,vehicle_class,status,fare_estimate_kobo,ride_notes)
		VALUES($1,gen_random_uuid(),$2,$3,'economy','pending_match',845000,'call me on +2348000000000')`,
		id, f.market, f.zone); err != nil {
		t.Fatalf("insert booking: %v", err)
	}
	return id
}

type feedEvent struct {
	ID, AggregateType, EventType, Payload string
}

func feedEvents(t *testing.T, db *pgxpool.Pool, bookingID string) []feedEvent {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT id::text, aggregate_type, event_type, payload::text
		FROM taxi_outbox WHERE target = 'fleet-marketplace' AND aggregate_id = $1
		ORDER BY (payload->>'sourceVersion')::bigint, occurred_at`, bookingID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []feedEvent
	for rows.Next() {
		var event feedEvent
		if err := rows.Scan(&event.ID, &event.AggregateType, &event.EventType, &event.Payload); err != nil {
			t.Fatal(err)
		}
		out = append(out, event)
	}
	return out
}

func TestFleetMarketplaceFeedPublishesRequestsAndOrganizationTrips(t *testing.T) {
	db := feedDB(t, true)
	ctx := context.Background()
	f := newFeedFixture(t, db)

	// An independent ride: offered, then taken by an independent driver.
	open := f.booking(t)
	if _, err := db.Exec(ctx, `UPDATE taxi_bookings SET status='awaiting_acceptance' WHERE id=$1`, open); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE taxi_bookings SET status='accepted', provider_id=$2 WHERE id=$1`, open, f.provider); err != nil {
		t.Fatal(err)
	}
	events := feedEvents(t, db, open)
	if len(events) != 2 || events[0].EventType != "platform.request.offered.v1" ||
		events[1].EventType != "platform.request.updated.v1" || events[0].AggregateType != "REQUEST" {
		t.Fatalf("independent ride events = %+v", events)
	}
	if !strings.Contains(events[0].Payload, `"sourceVersion": 1`) || !strings.Contains(events[1].Payload, `"sourceVersion": 2`) {
		t.Fatalf("versions not contiguous: %+v", events)
	}
	for _, event := range events {
		for _, forbidden := range []string{"845000", "fare", "+2348000000000", "ride_notes", "customer"} {
			if strings.Contains(event.Payload, forbidden) {
				t.Fatalf("feed payload leaks %q: %s", forbidden, event.Payload)
			}
		}
		if strings.Contains(event.Payload, f.provider) {
			t.Fatalf("open-request event names the independent driver: %s", event.Payload)
		}
	}

	// An organization-run ride: offered, bound to a Fleet assignment (the
	// managed-execution statement advances the version itself), then driven
	// through its lifecycle to completion.
	managed := f.booking(t)
	// A different driver: one driver holds one active booking at a time.
	f.provider = uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO taxi_providers(id,phone) VALUES($1,$2)`, f.provider, "+234"+f.provider[:10]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE taxi_bookings SET is_corporate_managed=TRUE, organization_id='org-feed',
		membership_id='mem-feed', fleet_assignment_id=$2, source_context='FLEET_ASSIGNMENT',
		settlement_mode='ORGANIZATION_MANAGED', provider_id=$3, source_version=source_version+1 WHERE id=$1`,
		managed, "asg-"+managed, f.provider); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"accepted", "en_route_pickup", "in_progress"} {
		if _, err := db.Exec(ctx, `UPDATE taxi_bookings SET status=$2 WHERE id=$1`, managed, status); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `UPDATE taxi_bookings SET status='completed', completed_at=now() WHERE id=$1`, managed); err != nil {
		t.Fatal(err)
	}
	events = feedEvents(t, db, managed)
	want := []string{"platform.request.offered.v1", "trip.created.v1", "trip.status_changed.v1",
		"trip.status_changed.v1", "trip.status_changed.v1", "trip.completed.v1"}
	if len(events) != len(want) {
		t.Fatalf("managed ride events = %+v", events)
	}
	for index, eventType := range want {
		if events[index].EventType != eventType {
			t.Fatalf("event %d = %s, want %s", index, events[index].EventType, eventType)
		}
		var payload map[string]any
		_ = json.Unmarshal([]byte(events[index].Payload), &payload)
		if payload["sourceVersion"] != float64(index+1) {
			t.Fatalf("event %d version = %v, want %d (contiguous, never double-advanced)", index, payload["sourceVersion"], index+1)
		}
	}
	created := events[1].Payload
	for _, required := range []string{`"tripId": "` + managed, `"fleetAssignmentId": "asg-` + managed,
		`"executingProviderId": "` + f.provider, `"settlementOrganizationId": "org-feed"`} {
		if !strings.Contains(created, required) {
			t.Fatalf("trip.created payload missing %s: %s", required, created)
		}
	}
	if !strings.Contains(events[5].Payload, `"completedAt"`) {
		t.Fatalf("trip.completed carries no completedAt: %s", events[5].Payload)
	}

	// Delivery: the destination posts Fleet's envelope, signed, with a hash
	// Fleet recomputes identically.
	var received map[string]any
	fleet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != FleetEventsPath || r.Header.Get(serviceauth.HeaderServiceName) != "driver-taxi-service" ||
			r.Header.Get(serviceauth.HeaderSignature) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer fleet.Close()
	destination := FleetMarketplaceEventDestination{BaseURL: fleet.URL, ServiceName: "driver-taxi-service",
		Secret: []byte("feed-secret"), HTTPClient: &http.Client{Timeout: 5 * time.Second}, Enabled: true}
	var raw []byte
	if err := db.QueryRow(ctx, `SELECT payload FROM taxi_outbox WHERE id=$1`, events[1].ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	err := destination.Deliver(ctx, Record{ID: events[1].ID, AggregateType: events[1].AggregateType,
		AggregateID: managed, EventType: events[1].EventType, Payload: raw, OccurredAt: time.Now(),
		Target: TargetFleetMarketplace})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if received["eventType"] != "trip.created.v1" || received["aggregateType"] != "trip" ||
		received["aggregateVersion"] != float64(2) || received["sourceService"] != "driver-taxi-service" {
		t.Fatalf("delivered envelope = %v", received)
	}
	canonical, _ := json.Marshal(received["payload"])
	sum := sha256.Sum256(canonical)
	if received["payloadHash"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("payloadHash %v != Fleet recomputation %s", received["payloadHash"], hex.EncodeToString(sum[:]))
	}

	// A permanent Fleet rejection dead-letters; anything else retries.
	reject := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":"REJECTED","code":"unknown_event_type"}`))
	}))
	defer reject.Close()
	destination.BaseURL = reject.URL
	err = destination.Deliver(ctx, Record{ID: events[1].ID, AggregateType: "TRIP", AggregateID: managed,
		EventType: events[1].EventType, Payload: raw, OccurredAt: time.Now()})
	var terminal *TerminalDeliveryError
	if !errors.As(err, &terminal) || terminal.Code != "unknown_event_type" {
		t.Fatalf("rejection err = %v, want terminal unknown_event_type", err)
	}
}

func TestFleetMarketplaceFeedSilentWhenDisabled(t *testing.T) {
	db := feedDB(t, false)
	f := newFeedFixture(t, db)
	id := f.booking(t)
	if _, err := db.Exec(context.Background(), `UPDATE taxi_bookings SET status='cancelled' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if events := feedEvents(t, db, id); len(events) != 0 {
		t.Fatalf("disabled feed published %+v", events)
	}
	var version int64
	if err := db.QueryRow(context.Background(), `SELECT source_version FROM taxi_bookings WHERE id=$1`, id).Scan(&version); err != nil || version != 1 {
		t.Fatalf("disabled feed moved source_version to %d (%v)", version, err)
	}
}
