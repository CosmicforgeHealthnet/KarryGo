package request

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
)

type broadcastEmissionFixture struct {
	pool *pgxpool.Pool
	repo *PostgresRepository
}

// newBroadcastEmissionFixture builds an isolated schema wired to the real
// marketplace emitter, so payloads pass production's forbidden-field
// validation rather than a stub's.
func newBroadcastEmissionFixture(t *testing.T, ctx context.Context) *broadcastEmissionFixture {
	t.Helper()

	databaseURL := os.Getenv("DISPATCH_REQUEST_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("set DISPATCH_REQUEST_TEST_DATABASE_URL or TEST_DATABASE_URL to run request event emission tests")
	}

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	schema := "request_emission_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE") //nolint:errcheck
	})

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	entries, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	for _, path := range entries {
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(path), err)
		}
	}

	emitter := marketplace.NewLegacyEventEmitter(marketplace.NewRepository(pool), true)
	return &broadcastEmissionFixture{
		pool: pool,
		repo: NewPostgresRepository(pool).WithEventEmitter(emitter),
	}
}

type broadcastEvent struct {
	EventType        string
	AggregateVersion int64
	Payload          map[string]any
}

func (f *broadcastEmissionFixture) events(t *testing.T, ctx context.Context) []broadcastEvent {
	t.Helper()
	rows, err := f.pool.Query(ctx, `
		SELECT event_type, aggregate_version, payload
		FROM dispatch_marketplace_outbox
		ORDER BY aggregate_version, created_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var events []broadcastEvent
	for rows.Next() {
		var event broadcastEvent
		var raw []byte
		if err := rows.Scan(&event.EventType, &event.AggregateVersion, &raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &event.Payload); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

// bookingPayload is a realistic customer booking, carrying exactly the data
// that must not reach a managing organization.
func bookingPayload(t *testing.T) []byte {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"customer_id":    uuid.NewString(),
		"customer_phone": "+2348099887766",
		"fare_amount":    250000,
		"currency":       "NGN",
		"receiver_name":  "Confidential Receiver",
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func (f *broadcastEmissionFixture) createBroadcast(t *testing.T, ctx context.Context) RequestBroadcast {
	t.Helper()
	broadcast, err := f.repo.CreateBroadcast(ctx, CreateBroadcastInput{
		BookingID: uuid.NewString(), ServiceType: "dispatch", RadiusKM: 5,
		Attempt: 1, ProvidersNotified: 3,
		BroadcastAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(30 * time.Second),
		BookingPayload: bookingPayload(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	return broadcast
}

// TestBroadcastLifecycleEmitsOrderedCanonicalEvents covers offer, re-broadcast
// and cancellation.
func TestBroadcastLifecycleEmitsOrderedCanonicalEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newBroadcastEmissionFixture(t, ctx)

	broadcast := fixture.createBroadcast(t, ctx)
	if err := fixture.repo.UpdateBroadcastAttempt(ctx, broadcast.ID, 2, 8, 4,
		time.Now().UTC(), time.Now().UTC().Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repo.CancelBroadcast(ctx, broadcast.ID); err != nil {
		t.Fatal(err)
	}

	events := fixture.events(t, ctx)
	wantTypes := []string{eventRequestOffered, eventRequestUpdated, eventRequestCancelled}
	if len(events) != len(wantTypes) {
		t.Fatalf("emitted %d events, want %d: %+v", len(events), len(wantTypes), events)
	}
	for index, want := range wantTypes {
		if events[index].EventType != want {
			t.Fatalf("event %d is %s, want %s", index, events[index].EventType, want)
		}
		if got := events[index].AggregateVersion; got != int64(index+2) {
			t.Fatalf("event %d (%s) has version %d, want %d — consumers gap-detect on this",
				index, want, got, index+2)
		}
	}
}

// TestTerminalBroadcastTransitionsEmitExactlyOnce is the property that makes
// these safe to drive from a scheduler.
//
// Expiry and cancellation both arrive from timers that can fire more than
// once. The status guard in the UPDATE means a repeat call changes no row, and
// this asserts the event stream agrees — a consumer must not watch one request
// expire twice.
func TestTerminalBroadcastTransitionsEmitExactlyOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newBroadcastEmissionFixture(t, ctx)

	broadcast := fixture.createBroadcast(t, ctx)
	for attempt := 0; attempt < 3; attempt++ {
		if err := fixture.repo.MarkBroadcastExpired(ctx, broadcast.ID); err != nil {
			t.Fatalf("expiry attempt %d: %v", attempt, err)
		}
	}

	var expired int
	for _, event := range fixture.events(t, ctx) {
		if event.EventType == eventRequestExpired {
			expired++
		}
	}
	if expired != 1 {
		t.Fatalf("three expiry calls produced %d expired events, want exactly 1", expired)
	}

	// Cancellation after expiry is a real transition — the guard admits
	// 'expired' — so it emits, and it must not reuse the expiry's version.
	if err := fixture.repo.CancelBroadcast(ctx, broadcast.ID); err != nil {
		t.Fatal(err)
	}
	events := fixture.events(t, ctx)
	seen := map[int64]bool{}
	for _, event := range events {
		if seen[event.AggregateVersion] {
			t.Fatalf("version %d reused across events: %+v", event.AggregateVersion, events)
		}
		seen[event.AggregateVersion] = true
	}
	if events[len(events)-1].EventType != eventRequestCancelled {
		t.Fatalf("last event is %s, want %s", events[len(events)-1].EventType, eventRequestCancelled)
	}
}

// TestNoProviderFoundEmitsExpiry pins the mapping decision.
func TestNoProviderFoundEmitsExpiry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newBroadcastEmissionFixture(t, ctx)

	broadcast := fixture.createBroadcast(t, ctx)
	if err := fixture.repo.MarkBroadcastNoProviderFound(ctx, broadcast.ID); err != nil {
		t.Fatal(err)
	}

	events := fixture.events(t, ctx)
	last := events[len(events)-1]
	if last.EventType != eventRequestExpired {
		t.Fatalf("no-provider-found emitted %s, want %s", last.EventType, eventRequestExpired)
	}
	if last.Payload["reason"] != "no_provider_found" {
		t.Fatalf("reason=%v, want no_provider_found — without it the consumer "+
			"cannot tell this from a timeout", last.Payload["reason"])
	}
}

// TestBroadcastEventsCarryNoBookingPayload is the important one.
//
// request_broadcasts.booking_payload holds the customer's booking exactly as
// submitted: contact details, fare, receiver. Copying that column onto a
// consumer-visible event is the single most likely way customer commercial
// data escapes this service, so it is asserted on the emitted bytes.
func TestBroadcastEventsCarryNoBookingPayload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newBroadcastEmissionFixture(t, ctx)

	broadcast := fixture.createBroadcast(t, ctx)
	if err := fixture.repo.UpdateBroadcastAttempt(ctx, broadcast.ID, 2, 8, 4,
		time.Now().UTC(), time.Now().UTC().Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repo.MarkBroadcastExpired(ctx, broadcast.ID); err != nil {
		t.Fatal(err)
	}

	events := fixture.events(t, ctx)
	if len(events) == 0 {
		t.Fatal("no events emitted; the rest of this test would vacuously pass")
	}
	for _, event := range events {
		for _, key := range []string{"bookingPayload", "booking_payload", "customerId",
			"customerPhone", "fareAmount", "receiverName"} {
			if _, present := event.Payload[key]; present {
				t.Fatalf("%s payload exposes %q: %+v", event.EventType, key, event.Payload)
			}
		}
		encoded, err := json.Marshal(event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{"+2348099887766", "Confidential Receiver", "250000"} {
			if strings.Contains(string(encoded), value) {
				t.Fatalf("%s payload leaks booking_payload value %q: %s",
					event.EventType, value, encoded)
			}
		}
	}
}

// TestBroadcastRepositoryWithoutEmitterPublishesNothing pins the disabled path
// that every existing caller uses.
func TestBroadcastRepositoryWithoutEmitterPublishesNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newBroadcastEmissionFixture(t, ctx)

	silent := NewPostgresRepository(fixture.pool)
	broadcast, err := silent.CreateBroadcast(ctx, CreateBroadcastInput{
		BookingID: uuid.NewString(), ServiceType: "dispatch", RadiusKM: 5,
		Attempt: 1, ProvidersNotified: 3,
		BroadcastAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(30 * time.Second),
		BookingPayload: bookingPayload(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := silent.MarkBroadcastExpired(ctx, broadcast.ID); err != nil {
		t.Fatal(err)
	}
	if events := fixture.events(t, ctx); len(events) != 0 {
		t.Fatalf("a repository with no emitter published %d events: %+v", len(events), events)
	}
}
