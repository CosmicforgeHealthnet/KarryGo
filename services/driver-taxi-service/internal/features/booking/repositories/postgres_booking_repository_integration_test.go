package bookingrepositories

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
)

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for PostgreSQL booking repository tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// seedFixture inserts a throwaway market, zone and provider so booking rows
// have valid FK targets, and registers ONE cleanup that removes them in the
// correct dependency order (bookings, then provider, then market -- the
// market/zone cascade is exercised by operating_zones' own ON DELETE CASCADE,
// but taxi_bookings' references to markets/providers stay RESTRICT by design,
// so bookings must go first). Geographic coordinates are irrelevant here:
// these tests never resolve a zone by coordinate, only by ID.
func seedFixture(t *testing.T, db *pgxpool.Pool) (marketID, zoneID, providerID string) {
	t.Helper()
	ctx := context.Background()
	marketID = uuid.NewString()
	zoneID = uuid.NewString()
	providerID = uuid.NewString()
	code := "TEST-" + uuid.NewString()[:8]

	if _, err := db.Exec(ctx, `INSERT INTO markets (id, country, name, code) VALUES ($1, 'NG', 'Test Market', $2)`,
		marketID, code); err != nil {
		t.Fatalf("seed market: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, match_radius_km, supported_classes)
		VALUES ($1, $2, 'Test Zone', $3, 1.0, -10.0, 10, '{economy,comfort,xl}')
	`, zoneID, marketID, code); err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO taxi_providers (id, phone) VALUES ($1, $2)`,
		providerID, "+234"+providerID[:10]); err != nil {
		t.Fatalf("seed provider: %v", err)
	}

	t.Cleanup(func() {
		cctx := context.Background()
		// Captured via subquery before the bookings themselves are deleted --
		// taxi_outbox carries no FK back to taxi_bookings (an outbox row must
		// survive even if its aggregate is later archived), so nothing cascades
		// this for us.
		_, _ = db.Exec(cctx, `DELETE FROM taxi_outbox WHERE aggregate_id IN (SELECT id FROM taxi_bookings WHERE market_id = $1 OR provider_id = $2)`, marketID, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_bookings WHERE market_id = $1 OR provider_id = $2`, marketID, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_providers WHERE id = $1`, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM markets WHERE id = $1`, marketID) // cascades to operating_zones
	})
	return marketID, zoneID, providerID
}

func sampleBooking(marketID, zoneID string, status bookingmodels.Status) bookingmodels.Booking {
	fare := int64(150000)
	distance := 8.4
	duration := 21
	return bookingmodels.Booking{
		CustomerID:            uuid.NewString(),
		MarketID:              marketID,
		OperatingZoneID:       zoneID,
		BookingMode:           bookingmodels.BookingModeStandard,
		VehicleClass:          "economy",
		PassengerCount:        1,
		FareEstimateKobo:      &fare,
		PricingQuoteReference: "quote-ref-1",
		PricingVersion:        "test-v1",
		DistanceKm:            &distance,
		DurationMinutes:       &duration,
		PaymentMethod:         "wallet",
		PaymentStatus:         "unpaid",
		Status:                status,
	}
}

func sampleStops() []bookingmodels.Stop {
	return []bookingmodels.Stop{
		{Sequence: 1, Type: bookingmodels.StopTypePickup, Address: "Pickup", Lat: 1.0005, Lng: -9.9995, Status: bookingmodels.StopStatusPending},
		{Sequence: 2, Type: bookingmodels.StopTypeDropoff, Address: "Dropoff", Lat: 1.02, Lng: -9.98, Status: bookingmodels.StopStatusPending},
	}
}

func TestCreateBooking_InsertsBookingAndStopsAtomically(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresBookingRepository(db)
	marketID, zoneID, _ := seedFixture(t, db)

	created, err := repo.CreateBooking(context.Background(),
		sampleBooking(marketID, zoneID, bookingmodels.StatusPendingMatch), sampleStops())
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected a generated ID")
	}
	if created.Status != bookingmodels.StatusPendingMatch {
		t.Errorf("status = %q, want pending_match", created.Status)
	}
	if len(created.Stops) != 2 {
		t.Fatalf("stops = %d, want 2", len(created.Stops))
	}
	if created.Stops[0].Type != bookingmodels.StopTypePickup || created.Stops[1].Type != bookingmodels.StopTypeDropoff {
		t.Errorf("unexpected stop types: %v, %v", created.Stops[0].Type, created.Stops[1].Type)
	}
	if created.FareEstimateKobo == nil || *created.FareEstimateKobo != 150000 {
		t.Errorf("fare estimate = %v, want 150000", created.FareEstimateKobo)
	}
}

// TestCreateBooking_WritesOutboxEventInSameTransaction is B7's write-side
// guarantee: the taxi.booking.created event must land atomically with the
// booking it describes, so a crash between commit and delivery can never
// lose it (see internal/outbox's own package doc comment).
func TestCreateBooking_WritesOutboxEventInSameTransaction(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresBookingRepository(db)
	marketID, zoneID, _ := seedFixture(t, db)

	created, err := repo.CreateBooking(context.Background(),
		sampleBooking(marketID, zoneID, bookingmodels.StatusPendingMatch), sampleStops())
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}

	var eventType string
	var payload []byte
	err = db.QueryRow(context.Background(),
		`SELECT event_type, payload FROM taxi_outbox WHERE aggregate_id = $1 AND aggregate_type = 'booking'`,
		created.ID).Scan(&eventType, &payload)
	if err != nil {
		t.Fatalf("expected an outbox row for the created booking: %v", err)
	}
	if eventType != "taxi.booking.created" {
		t.Errorf("event_type = %q, want taxi.booking.created", eventType)
	}
	if !bytes.Contains(payload, []byte(created.ID)) {
		t.Errorf("payload does not reference the booking id: %s", payload)
	}
}

func TestCreateBooking_StopsPersistedInSequenceOrder(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresBookingRepository(db)
	marketID, zoneID, _ := seedFixture(t, db)

	stops := []bookingmodels.Stop{
		{Sequence: 1, Type: bookingmodels.StopTypePickup, Address: "A", Lat: 1.0005, Lng: -9.9995, Status: bookingmodels.StopStatusPending},
		{Sequence: 2, Type: bookingmodels.StopTypeWaypoint, Address: "B", Lat: 1.01, Lng: -9.99, Status: bookingmodels.StopStatusPending},
		{Sequence: 3, Type: bookingmodels.StopTypeDropoff, Address: "C", Lat: 1.02, Lng: -9.98, Status: bookingmodels.StopStatusPending},
	}
	created, err := repo.CreateBooking(context.Background(),
		sampleBooking(marketID, zoneID, bookingmodels.StatusPendingMatch), stops)
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(fetched.Stops) != 3 {
		t.Fatalf("stops = %d, want 3", len(fetched.Stops))
	}
	wantAddr := []string{"A", "B", "C"}
	for i, want := range wantAddr {
		if fetched.Stops[i].Address != want {
			t.Errorf("stops[%d].Address = %q, want %q", i, fetched.Stops[i].Address, want)
		}
	}
}

func TestGetByID_NotFoundForUnknownID(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresBookingRepository(db)

	_, err := repo.GetByID(context.Background(), uuid.NewString())
	if err == nil {
		t.Fatal("expected an error for an unknown booking id")
	}
}

func TestGetByIDForCustomer_NotFoundForWrongCustomer(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresBookingRepository(db)
	marketID, zoneID, _ := seedFixture(t, db)

	created, err := repo.CreateBooking(context.Background(),
		sampleBooking(marketID, zoneID, bookingmodels.StatusPendingMatch), sampleStops())
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}

	_, err = repo.GetByIDForCustomer(context.Background(), created.ID, uuid.NewString())
	if err == nil {
		t.Fatal("expected an error when the booking does not belong to the requesting customer")
	}
}

func TestListByCustomer_OrdersNewestFirstAndRespectsLimit(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresBookingRepository(db)
	marketID, zoneID, _ := seedFixture(t, db)

	customerID := uuid.NewString()
	var lastID string
	for i := 0; i < 3; i++ {
		b := sampleBooking(marketID, zoneID, bookingmodels.StatusCompleted)
		b.CustomerID = customerID
		created, err := repo.CreateBooking(context.Background(), b, sampleStops())
		if err != nil {
			t.Fatalf("CreateBooking[%d]: %v", i, err)
		}
		lastID = created.ID
	}

	list, err := repo.ListByCustomer(context.Background(), customerID, 1)
	if err != nil {
		t.Fatalf("ListByCustomer: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list = %d, want 1 (limit respected)", len(list))
	}
	if list[0].ID != lastID {
		t.Errorf("newest-first ordering not respected: got %s, want %s", list[0].ID, lastID)
	}
}

func TestHasActiveBooking(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresBookingRepository(db)
	marketID, zoneID, _ := seedFixture(t, db)

	cases := []struct {
		name       string
		status     bookingmodels.Status
		wantActive bool
	}{
		{"pending_match is active", bookingmodels.StatusPendingMatch, true},
		{"accepted is active", bookingmodels.StatusAccepted, true},
		{"in_progress is active", bookingmodels.StatusInProgress, true},
		{"completed is not active", bookingmodels.StatusCompleted, false},
		{"cancelled is not active", bookingmodels.StatusCancelled, false},
		// Deliberate: a ride booked for later does not block booking one now
		// (see bookingmodels.ActiveStatuses' own doc comment).
		{"scheduled is not active", bookingmodels.StatusScheduled, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			customerID := uuid.NewString()
			before, err := repo.HasActiveBooking(context.Background(), customerID)
			if err != nil {
				t.Fatalf("HasActiveBooking (before): %v", err)
			}
			if before {
				t.Fatal("a fresh random customer id should never already have an active booking")
			}

			b := sampleBooking(marketID, zoneID, tc.status)
			b.CustomerID = customerID
			if _, err := repo.CreateBooking(context.Background(), b, sampleStops()); err != nil {
				t.Fatalf("CreateBooking: %v", err)
			}

			got, err := repo.HasActiveBooking(context.Background(), customerID)
			if err != nil {
				t.Fatalf("HasActiveBooking (after): %v", err)
			}
			if got != tc.wantActive {
				t.Errorf("HasActiveBooking = %v, want %v for status %q", got, tc.wantActive, tc.status)
			}
		})
	}
}

// TestOneActiveBookingPerProviderConstraint exercises the migration's DB-
// enforced addendum §5.1 guarantee directly: no repository method assigns a
// provider yet (that is phase B2/B3), so this reaches the schema with raw SQL
// -- the same guarantee the future matcher will rely on being enforced by
// Postgres itself, not by application-level care alone.
func TestOneActiveBookingPerProviderConstraint(t *testing.T) {
	db := testDB(t)
	marketID, zoneID, providerID := seedFixture(t, db)
	ctx := context.Background()

	id1 := uuid.NewString()
	_, err := db.Exec(ctx, `
		INSERT INTO taxi_bookings (id, customer_id, provider_id, market_id, operating_zone_id, status)
		VALUES ($1, $2, $3, $4, $5, 'accepted')
	`, id1, uuid.NewString(), providerID, marketID, zoneID)
	if err != nil {
		t.Fatalf("first active booking insert: %v", err)
	}

	id2 := uuid.NewString()
	_, err = db.Exec(ctx, `
		INSERT INTO taxi_bookings (id, customer_id, provider_id, market_id, operating_zone_id, status)
		VALUES ($1, $2, $3, $4, $5, 'en_route_pickup')
	`, id2, uuid.NewString(), providerID, marketID, zoneID)
	if err == nil {
		t.Fatal("expected the partial unique index to reject a second concurrent active booking for the same provider")
	}

	// A terminal-status booking for the same provider must NOT be blocked --
	// only the active-status set participates in the partial index.
	id3 := uuid.NewString()
	_, err = db.Exec(ctx, `
		INSERT INTO taxi_bookings (id, customer_id, provider_id, market_id, operating_zone_id, status)
		VALUES ($1, $2, $3, $4, $5, 'completed')
	`, id3, uuid.NewString(), providerID, marketID, zoneID)
	if err != nil {
		t.Fatalf("a terminal-status booking for the same provider should be allowed: %v", err)
	}
}

// seedAwaitingAcceptance inserts a booking already assigned to providerID in
// StatusAwaitingAccept -- the state a real MarkMatched call would have left
// it in, without needing the whole matcher to produce one.
func seedAwaitingAcceptance(t *testing.T, db *pgxpool.Pool, marketID, zoneID, providerID string) string {
	t.Helper()
	ctx := context.Background()
	bookingID := uuid.NewString()
	if _, err := db.Exec(ctx, `
		INSERT INTO taxi_bookings (id, customer_id, provider_id, market_id, operating_zone_id, vehicle_class, status, matched_at)
		VALUES ($1, $2, $3, $4, $5, 'economy', 'awaiting_acceptance', now())
	`, bookingID, uuid.NewString(), providerID, marketID, zoneID); err != nil {
		t.Fatalf("seed awaiting_acceptance booking: %v", err)
	}
	return bookingID
}

// TestAcceptAtTimeoutBoundary_LateAcceptNeverOverwritten is the exact
// defect class pinned in driver-hauling-service/booking_repository.go,
// ported to taxi's own ResetToMatching/MarkAccepted pair: a driver's accept
// landing an instant before the matcher's timeout-triggered reset fires
// must survive, never be silently clobbered back to pending_match.
//
// Tested deterministically (no goroutines, no sleeps) by calling the two
// guarded statements in the exact order a real race would produce: accept
// commits first, then the (now-stale) reset attempt runs against the
// already-changed row. Any real concurrent execution reduces to one of two
// serial orderings under Postgres's own row locking, so this is not a
// weaker test than a live race -- it is the same guarantee, without the
// flakiness a real goroutine race would add.
func TestAcceptAtTimeoutBoundary_LateAcceptNeverOverwritten(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresBookingRepository(db)
	marketID, zoneID, providerID := seedFixture(t, db)
	bookingID := seedAwaitingAcceptance(t, db, marketID, zoneID, providerID)

	// The driver's accept lands first.
	accepted, err := repo.MarkAccepted(context.Background(), nil, bookingID, providerID)
	if err != nil {
		t.Fatalf("MarkAccepted: %v", err)
	}
	if accepted.Status != bookingmodels.StatusAccepted {
		t.Fatalf("status after MarkAccepted = %q, want accepted", accepted.Status)
	}

	// The matcher's timeout goroutine, which read the booking as
	// awaiting_acceptance a moment before the accept above landed, now
	// (too late) tries to reset it.
	_, err = repo.ResetToMatching(context.Background(), nil, bookingID, providerID)
	if err == nil {
		t.Fatal("expected the late reset to be refused -- it would silently clobber a real accept")
	}

	// The accept's effect must have survived completely intact.
	final, err := repo.GetByID(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if final.Status != bookingmodels.StatusAccepted {
		t.Errorf("final status = %q, want accepted (the reset must not have overwritten it)", final.Status)
	}
	if final.ProviderID == nil || *final.ProviderID != providerID {
		t.Errorf("final provider_id = %v, want %s (must not have been cleared)", final.ProviderID, providerID)
	}
}

// TestAcceptAtTimeoutBoundary_LateAcceptAfterGenuineTimeoutIsRefused is the
// symmetric case: the timeout genuinely wins the race (reset lands first),
// and a driver's accept arriving a moment later -- for the offer they no
// longer hold -- must be refused, not silently re-commit them to a booking
// that has already moved on to a new candidate.
func TestAcceptAtTimeoutBoundary_LateAcceptAfterGenuineTimeoutIsRefused(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresBookingRepository(db)
	marketID, zoneID, providerID := seedFixture(t, db)
	bookingID := seedAwaitingAcceptance(t, db, marketID, zoneID, providerID)

	// The timeout fires first: reset back to pending_match.
	reset, err := repo.ResetToMatching(context.Background(), nil, bookingID, providerID)
	if err != nil {
		t.Fatalf("ResetToMatching: %v", err)
	}
	if reset.Status != bookingmodels.StatusPendingMatch || reset.ProviderID != nil {
		t.Fatalf("after reset: status=%q provider_id=%v, want pending_match/nil", reset.Status, reset.ProviderID)
	}

	// The driver's accept, for an offer that no longer stands, arrives late.
	_, err = repo.MarkAccepted(context.Background(), nil, bookingID, providerID)
	if err == nil {
		t.Fatal("expected a late accept for an already-reset offer to be refused")
	}

	final, err := repo.GetByID(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if final.Status != bookingmodels.StatusPendingMatch {
		t.Errorf("final status = %q, want pending_match (the late accept must not have re-applied)", final.Status)
	}
}

func TestCreateBooking_PersistsImmutableRequirementSnapshot(t *testing.T) {
	db := testDB(t)
	repo := NewPostgresBookingRepository(db)
	marketID, zoneID, _ := seedFixture(t, db)
	ctx := context.Background()

	booking := sampleBooking(marketID, zoneID, bookingmodels.StatusPendingMatch)
	code, version, policy, guarantee := "TAXI_COMFORT", int64(2), "p2", "NONE"
	booking.ServiceProductCode, booking.ServiceProductVersion = &code, &version
	booking.EligibilityPolicyVersion, booking.GuaranteeProfileVersion = &policy, &guarantee
	booking.RequirementSnapshot = []byte(`{"product_code":"TAXI_COMFORT","passenger_count":1,"wheelchair_required":true,"preferences":{"quiet_ride":true}}`)
	created, err := repo.CreateBooking(ctx, booking, sampleStops())
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}
	read, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.ServiceProductCode == nil || *read.ServiceProductCode != code || *read.ServiceProductVersion != version || *read.EligibilityPolicyVersion != policy {
		t.Fatalf("product columns = %+v", read)
	}
	if public := read.Public(); public.Requirements == nil || !public.Requirements.WheelchairRequired {
		t.Fatalf("public requirements = %+v", public.Requirements)
	}
	if _, err = db.Exec(ctx, `UPDATE taxi_bookings SET requirement_snapshot_json='{"wheelchair_required":false}' WHERE id=$1`, created.ID); err == nil {
		t.Fatal("requirement snapshot must be immutable after creation")
	}
	if _, err = db.Exec(ctx, `UPDATE taxi_bookings SET service_product_code='TAXI_XL' WHERE id=$1`, created.ID); err == nil {
		t.Fatal("service product must be immutable after creation")
	}
	if _, err = db.Exec(ctx, `UPDATE taxi_bookings SET ride_notes='still editable' WHERE id=$1`, created.ID); err != nil {
		t.Fatalf("unrelated columns must stay writable: %v", err)
	}

	legacy, err := repo.CreateBooking(ctx, sampleBooking(marketID, zoneID, bookingmodels.StatusPendingMatch), sampleStops())
	if err != nil || legacy.ServiceProductCode != nil || legacy.RequirementSnapshot != nil {
		t.Fatalf("legacy booking = %+v / %v", legacy, err)
	}
	half := sampleBooking(marketID, zoneID, bookingmodels.StatusPendingMatch)
	half.ServiceProductCode = &code
	if _, err = repo.CreateBooking(ctx, half, sampleStops()); err == nil {
		t.Fatal("a product without its full snapshot must be refused")
	}
}
