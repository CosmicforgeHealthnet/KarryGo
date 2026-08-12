package trip

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
	"cosmicforge/logistics/shared/go/handover"
)

// emissionFixture is a migrated schema with an emitting repository attached.
type emissionFixture struct {
	pool       *pgxpool.Pool
	repo       *PostgresRepository
	providerID string
	bookingID  string
}

// newEmissionFixture builds an isolated schema wired to the REAL marketplace
// emitter rather than a stub.
//
// Using the production emitter is the point of this fixture. A fake would
// prove that emitTripEvent calls something; only the real one proves the
// payload survives marketplace's forbidden-field validation and lands in the
// outbox table with the columns the delivery worker reads back.
func newEmissionFixture(t *testing.T, ctx context.Context) *emissionFixture {
	t.Helper()

	databaseURL := os.Getenv("DISPATCH_TRIP_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("set DISPATCH_TRIP_TEST_DATABASE_URL or TEST_DATABASE_URL to run trip event emission tests")
	}

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	schema := "trip_emission_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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

	providerID, bookingID, customerID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO providers (id,phone) VALUES ($1,$2)`,
		providerID, "+2348000072001"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO request_broadcasts (booking_id,broadcast_radius_km,expires_at,booking_payload)
		VALUES ($1,5,now()+interval '30 seconds',jsonb_build_object('customer_id',$2::text))
	`, bookingID, customerID); err != nil {
		t.Fatal(err)
	}

	emitter := marketplace.NewLegacyEventEmitter(marketplace.NewRepository(pool), true)
	return &emissionFixture{
		pool:       pool,
		repo:       NewPostgresRepository(pool).WithEventEmitter(emitter),
		providerID: providerID,
		bookingID:  bookingID,
	}
}

// outboxEvent is one row as a consumer would receive it.
type outboxEvent struct {
	EventType        string
	AggregateID      string
	AggregateVersion int64
	Payload          map[string]any
}

// events reads the outbox in publication order.
func (f *emissionFixture) events(t *testing.T, ctx context.Context) []outboxEvent {
	t.Helper()
	rows, err := f.pool.Query(ctx, `
		SELECT event_type, aggregate_id::text, aggregate_version, payload
		FROM dispatch_marketplace_outbox
		ORDER BY aggregate_version, created_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var events []outboxEvent
	for rows.Next() {
		var event outboxEvent
		var raw []byte
		if err := rows.Scan(&event.EventType, &event.AggregateID,
			&event.AggregateVersion, &raw); err != nil {
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

func (f *emissionFixture) createTrip(t *testing.T, ctx context.Context) *Trip {
	t.Helper()
	trip, err := f.repo.CreateTripFromAcceptedRequest(ctx, CreateTripInput{
		BookingID: f.bookingID, ProviderID: f.providerID,
		PickupAddress: "Pickup", PickupLat: 6.5, PickupLng: 3.4,
		DropoffAddress: "Dropoff", DropoffLat: 6.6, DropoffLng: 3.5,
		FareAmount: 150000, Currency: "NGN", ReceiverName: "Receiver",
		ReceiverPhone: "+2348011223344",
	})
	if err != nil {
		t.Fatal(err)
	}
	return trip
}

// TestTripLifecycleEmitsOrderedCanonicalEvents walks a trip from assignment to
// completion and asserts the consumer-visible event stream.
func TestTripLifecycleEmitsOrderedCanonicalEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)

	trip := fixture.createTrip(t, ctx)
	now := time.Now().UTC()

	pickup, err := handover.NewCodeRecord(fghHandoverSecret, handover.BuildChallengeID("dispatch", handover.StepPickup, trip.ID), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repo.MarkArrived(ctx, trip.ID, fixture.providerID, StatusAssigned, now, pickup); err != nil {
		t.Fatal(err)
	}
	delivery, err := handover.NewCodeRecord(fghHandoverSecret, handover.BuildChallengeID("dispatch", handover.StepDelivery, trip.ID), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repo.MarkTripStarted(ctx, trip.ID, fixture.providerID, now, delivery); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repo.SubmitProofTx(ctx, SubmitProofDBInput{
		TripID: trip.ID, ProviderID: fixture.providerID,
		PhotoRef: "media://photo", SignatureRef: "media://signature",
		ReceiverName: "Receiver", ReceiverPhone: "+2348011223344", Now: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.repo.CompleteTripTx(ctx, CompleteTripInput{
		TripID: trip.ID, ProviderID: fixture.providerID, Now: now,
	}); err != nil {
		t.Fatal(err)
	}

	events := fixture.events(t, ctx)
	wantTypes := []string{
		eventTripStatusChanged, // arrived_pickup
		eventTripStatusChanged, // in_progress
		eventProofCompleted,
		eventTripCompleted,
	}
	if len(events) != len(wantTypes) {
		t.Fatalf("emitted %d events, want %d: %+v", len(events), len(wantTypes), events)
	}
	for index, want := range wantTypes {
		if events[index].EventType != want {
			t.Fatalf("event %d is %s, want %s", index, events[index].EventType, want)
		}
		if events[index].AggregateID != trip.ID {
			t.Fatalf("event %d aggregate %s, want trip %s", index,
				events[index].AggregateID, trip.ID)
		}
	}

	// The trip row is created at source_version 1, so the first emitted event
	// is 2. Consumers gap-detect on consecutive versions, so any hole here
	// would make fleet mark the projection GAP_DETECTED and stop applying.
	for index, event := range events {
		if want := int64(index + 2); event.AggregateVersion != want {
			t.Fatalf("event %d (%s) has version %d, want %d — consumers gap-detect on this",
				index, event.EventType, event.AggregateVersion, want)
		}
		version, ok := event.Payload["sourceVersion"].(float64)
		if !ok || int64(version) != event.AggregateVersion {
			t.Fatalf("event %d payload sourceVersion=%v disagrees with column %d",
				index, event.Payload["sourceVersion"], event.AggregateVersion)
		}
	}

	var stored int64
	if err := fixture.pool.QueryRow(ctx,
		`SELECT source_version FROM trips WHERE id=$1`, trip.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if want := int64(len(events) + 1); stored != want {
		t.Fatalf("trips.source_version=%d, want %d", stored, want)
	}
}

// TestTripEventsCarryNoCustomerCommercialData guards the boundary that makes
// this stream safe to hand to a managing organization.
//
// The trip row holds the customer's fare, the currency and the receiver's name
// and phone number. A managing organization is entitled to none of them. This
// asserts on the emitted payloads rather than on the builder so that a future
// field added to tripEventPayload is caught here.
func TestTripEventsCarryNoCustomerCommercialData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)

	trip := fixture.createTrip(t, ctx)
	now := time.Now().UTC()
	pickup, err := handover.NewCodeRecord(fghHandoverSecret, handover.BuildChallengeID("dispatch", handover.StepPickup, trip.ID), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repo.MarkArrived(ctx, trip.ID, fixture.providerID, StatusAssigned, now, pickup); err != nil {
		t.Fatal(err)
	}
	delivery, err := handover.NewCodeRecord(fghHandoverSecret, handover.BuildChallengeID("dispatch", handover.StepDelivery, trip.ID), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repo.MarkTripStarted(ctx, trip.ID, fixture.providerID, now, delivery); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repo.SubmitProofTx(ctx, SubmitProofDBInput{
		TripID: trip.ID, ProviderID: fixture.providerID,
		PhotoRef: "media://photo", SignatureRef: "media://signature",
		ReceiverName: "Receiver", ReceiverPhone: "+2348011223344", Now: now,
	}); err != nil {
		t.Fatal(err)
	}

	forbidden := []string{
		"fare", "fareAmount", "amount", "currency", "price",
		"receiverName", "receiverPhone", "customerId", "customerPhone",
		"photoRef", "signatureRef",
	}
	events := fixture.events(t, ctx)
	if len(events) == 0 {
		t.Fatal("no events emitted; the rest of this test would vacuously pass")
	}
	for _, event := range events {
		for _, key := range forbidden {
			if _, present := event.Payload[key]; present {
				t.Fatalf("%s payload exposes %q to the managing organization: %+v",
					event.EventType, key, event.Payload)
			}
		}
		// Values leak as readily as keys: the receiver's phone number under
		// any name at all is the same disclosure.
		encoded, err := json.Marshal(event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{"+2348011223344", "Receiver", "150000"} {
			if strings.Contains(string(encoded), value) {
				t.Fatalf("%s payload contains restricted value %q: %s",
					event.EventType, value, encoded)
			}
		}
	}
}

// TestRejectedTransitionEmitsNothing proves the event cannot outlive a change
// that did not happen.
func TestRejectedTransitionEmitsNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)

	trip := fixture.createTrip(t, ctx)
	now := time.Now().UTC()

	// The trip is 'assigned'; starting requires 'arrived_pickup'. The UPDATE
	// matches no row, so the repository reports the race and must publish
	// nothing — an event here would tell fleet a trip began that never did.
	delivery, err := handover.NewCodeRecord(fghHandoverSecret, handover.BuildChallengeID("dispatch", handover.StepDelivery, trip.ID), now)
	if err != nil {
		t.Fatal(err)
	}
	started, err := fixture.repo.MarkTripStarted(ctx, trip.ID, fixture.providerID, now, delivery)
	if err != nil {
		t.Fatal(err)
	}
	if started != nil {
		t.Fatalf("expected no transition from assigned, got %+v", started)
	}
	if events := fixture.events(t, ctx); len(events) != 0 {
		t.Fatalf("a rejected transition emitted %d events: %+v", len(events), events)
	}

	// A transition for a provider who does not own the trip is likewise a
	// no-op and must stay silent.
	pickup, err := handover.NewCodeRecord(fghHandoverSecret, handover.BuildChallengeID("dispatch", handover.StepPickup, trip.ID), now)
	if err != nil {
		t.Fatal(err)
	}
	arrived, err := fixture.repo.MarkArrived(ctx, trip.ID, uuid.NewString(), StatusAssigned, now, pickup)
	if err != nil {
		t.Fatal(err)
	}
	if arrived != nil {
		t.Fatalf("expected no transition for a foreign provider, got %+v", arrived)
	}
	if events := fixture.events(t, ctx); len(events) != 0 {
		t.Fatalf("a foreign-provider transition emitted %d events: %+v", len(events), events)
	}
}

// TestRepositoryWithoutEmitterPublishesNothing pins the disabled path.
//
// Every existing caller constructs the repository without an emitter, so this
// is what the rest of the suite is exercising; if it ever started writing
// events, the outbox would fill in deployments that never enabled it.
func TestRepositoryWithoutEmitterPublishesNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)

	silent := NewPostgresRepository(fixture.pool)
	trip, err := silent.CreateTripFromAcceptedRequest(ctx, CreateTripInput{
		BookingID: fixture.bookingID, ProviderID: fixture.providerID,
		PickupAddress: "Pickup", PickupLat: 6.5, PickupLng: 3.4,
		DropoffAddress: "Dropoff", DropoffLat: 6.6, DropoffLng: 3.5,
		FareAmount: 150000, Currency: "NGN", ReceiverName: "Receiver",
		ReceiverPhone: "+2348011223344",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	pickup, err := handover.NewCodeRecord(fghHandoverSecret, handover.BuildChallengeID("dispatch", handover.StepPickup, trip.ID), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := silent.MarkArrived(ctx, trip.ID, fixture.providerID,
		StatusAssigned, now, pickup); err != nil {
		t.Fatal(err)
	}
	if events := fixture.events(t, ctx); len(events) != 0 {
		t.Fatalf("a repository with no emitter published %d events: %+v", len(events), events)
	}
	var version int64
	if err := fixture.pool.QueryRow(ctx,
		`SELECT source_version FROM trips WHERE id=$1`, trip.ID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("source_version advanced to %d without an emitter", version)
	}
}
