package bookingusecases

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/shared/go/apperrors"
)

// This file proves the property fakes cannot: that a booking status
// transition and its corresponding provider operational-state transition
// genuinely commit or roll back TOGETHER, in one real Postgres transaction
// (the addendum's explicit requirement -- see lifecycle.go's
// withOperationalState doc comment). Every other lifecycle test lives here,
// not just the atomicity ones, since setting up a booking already assigned
// to a provider requires real rows (there is no B2 matcher yet to produce
// one through the API).

func lifecycleTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for booking lifecycle integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func mustExec(t *testing.T, db *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// seedLifecycleFixture inserts a market/zone/provider, one booking already
// assigned to that provider in the given status, and (when opState != "") a
// starting provider_operational_state row at version 1. Registers a single
// cleanup covering every table touched, in FK-safe order.
func seedLifecycleFixture(t *testing.T, db *pgxpool.Pool, status bookingmodels.Status, opState availabilitymodels.OperationalState) (bookingID, providerID string) {
	t.Helper()
	marketID := uuid.NewString()
	zoneID := uuid.NewString()
	providerID = uuid.NewString()
	bookingID = uuid.NewString()
	code := "TEST-" + uuid.NewString()[:8]

	mustExec(t, db, `INSERT INTO markets (id, country, name, code) VALUES ($1, 'NG', 'Test Market', $2)`, marketID, code)
	mustExec(t, db, `
		INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, match_radius_km, supported_classes)
		VALUES ($1, $2, 'Test Zone', $3, 1.0, -10.0, 10, '{economy}')
	`, zoneID, marketID, code)
	mustExec(t, db, `INSERT INTO taxi_providers (id, phone) VALUES ($1, $2)`, providerID, "+234"+providerID[:10])
	mustExec(t, db, `
		INSERT INTO taxi_bookings (id, customer_id, provider_id, market_id, operating_zone_id, vehicle_class, status, pricing_quote_reference)
		VALUES ($1, $2, $3, $4, $5, 'economy', $6, 'quote-ref-test')
	`, bookingID, uuid.NewString(), providerID, marketID, zoneID, string(status))
	if opState != "" {
		mustExec(t, db, `INSERT INTO provider_operational_state (provider_id, state, version) VALUES ($1, $2, 1)`, providerID, string(opState))
	}

	t.Cleanup(func() {
		cctx := context.Background()
		_, _ = db.Exec(cctx, `DELETE FROM provider_operational_state_history WHERE provider_id = $1`, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM provider_operational_state WHERE provider_id = $1`, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_outbox WHERE aggregate_id IN (SELECT id FROM taxi_bookings WHERE market_id = $1 OR provider_id = $2)`, marketID, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_bookings WHERE market_id = $1 OR provider_id = $2`, marketID, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM taxi_providers WHERE id = $1`, providerID)
		_, _ = db.Exec(cctx, `DELETE FROM markets WHERE id = $1`, marketID)
	})
	return bookingID, providerID
}

func newLifecycleService(db *pgxpool.Pool) *BookingService {
	bookings := bookingrepositories.NewPostgresBookingRepository(db)
	opState := availabilityrepositories.NewPostgresOperationalStateRepository(db)
	return NewBookingService(Options{
		Bookings: bookings, Markets: newFakeMarkets(), Pricing: newFakePricing(), OperationalState: opState,
	})
}

func operationalStateOf(t *testing.T, db *pgxpool.Pool, providerID string) (state string, version int64) {
	t.Helper()
	err := db.QueryRow(context.Background(),
		`SELECT state, version FROM provider_operational_state WHERE provider_id = $1`, providerID).Scan(&state, &version)
	if err != nil {
		t.Fatalf("read operational state: %v", err)
	}
	return state, version
}

func TestWithTransition_RecordsBookingEventInTheSameTransaction(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusAccepted, availabilitymodels.StateOffered)

	if _, err := svc.MarkEnRoutePickup(context.Background(), bookingID, providerID); err != nil {
		t.Fatalf("MarkEnRoutePickup: %v", err)
	}

	events, err := svc.bookings.ListEvents(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].EventType != outbox.EventBookingEnRoutePickup || events[0].ActorType != "provider" || events[0].ActorID != providerID {
		t.Errorf("event = %+v, want type=%s actor=(provider,%s)", events[0], outbox.EventBookingEnRoutePickup, providerID)
	}
}

func TestMarkEnRoutePickup_TransitionsBookingAndOperationalStateTogether(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusAccepted, availabilitymodels.StateOffered)

	booking, err := svc.MarkEnRoutePickup(context.Background(), bookingID, providerID)
	if err != nil {
		t.Fatalf("MarkEnRoutePickup: %v", err)
	}
	if booking.Status != bookingmodels.StatusEnRoutePickup {
		t.Errorf("booking status = %q, want en_route_pickup", booking.Status)
	}
	state, version := operationalStateOf(t, db, providerID)
	if state != string(availabilitymodels.StateEnRoutePickup) {
		t.Errorf("operational state = %q, want EN_ROUTE_PICKUP", state)
	}
	if version != 2 {
		t.Errorf("operational state version = %d, want 2 (bumped from the seeded 1)", version)
	}

	// B3's lifecycle transitions did not write outbox events until this
	// session retrofitted withTransition to always write one -- assert it
	// actually landed, in the same transaction as everything above.
	var eventType string
	if err := db.QueryRow(context.Background(),
		`SELECT event_type FROM taxi_outbox WHERE aggregate_id = $1 AND aggregate_type = 'booking'`, bookingID).Scan(&eventType); err != nil {
		t.Fatalf("expected an outbox row for this transition: %v", err)
	}
	if eventType != "taxi.booking.en_route_pickup" {
		t.Errorf("event_type = %q, want taxi.booking.en_route_pickup", eventType)
	}
}

// TestAcceptBooking_DoesNotCorruptOperationalState is a regression test for
// a real bug this session's live verification caught: withTransition's skip
// condition originally gated only on providerID != "", but AcceptBooking
// passes a REAL providerID (needed for MarkAccepted's own SQL guard) with
// an EMPTY newState (accept deliberately does not move operational state --
// see AcceptBooking's own doc comment for why OFFERED already covers this
// window). That let the Transition call through anyway and wrote a literal
// empty string into provider_operational_state.state, corrupting it. A
// fake-based unit test could not have caught this: the fake Transition
// accepts any state value unconditionally. Only a real Postgres write
// surfaces a corrupted column value.
func TestAcceptBooking_DoesNotCorruptOperationalState(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusAwaitingAccept, availabilitymodels.StateOffered)

	booking, err := svc.AcceptBooking(context.Background(), bookingID, providerID)
	if err != nil {
		t.Fatalf("AcceptBooking: %v", err)
	}
	if booking.Status != bookingmodels.StatusAccepted {
		t.Errorf("booking status = %q, want accepted", booking.Status)
	}

	state, version := operationalStateOf(t, db, providerID)
	if state != string(availabilitymodels.StateOffered) {
		t.Errorf("operational state = %q, want OFFERED unchanged (accept must not touch it)", state)
	}
	if version != 1 {
		t.Errorf("operational state version = %d, want 1 (unchanged from the seed -- Transition must never have been called)", version)
	}
}

func TestMarkArrivedAtPickup_WrongProviderRejectedAndNothingChanges(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusEnRoutePickup, availabilitymodels.StateEnRoutePickup)
	imposterID := uuid.NewString()

	_, err := svc.MarkArrivedAtPickup(context.Background(), bookingID, imposterID)
	if err == nil {
		t.Fatal("expected an error when the wrong provider attempts the transition")
	}

	// The whole point of sharing one transaction: a rejected booking
	// transition must never have partially applied an operational-state
	// change for the imposter (there should be no row for them at all), and
	// the REAL provider's own booking/state must be completely untouched.
	var imposterRows int
	if err := db.QueryRow(context.Background(),
		`SELECT count(*) FROM provider_operational_state WHERE provider_id = $1`, imposterID).Scan(&imposterRows); err != nil {
		t.Fatalf("count imposter state rows: %v", err)
	}
	if imposterRows != 0 {
		t.Errorf("imposter operational_state rows = %d, want 0 (rejected transition must not have committed anything)", imposterRows)
	}

	var status string
	if err := db.QueryRow(context.Background(), `SELECT status FROM taxi_bookings WHERE id = $1`, bookingID).Scan(&status); err != nil {
		t.Fatalf("read booking status: %v", err)
	}
	if status != string(bookingmodels.StatusEnRoutePickup) {
		t.Errorf("booking status = %q, want unchanged en_route_pickup", status)
	}
	state, version := operationalStateOf(t, db, providerID)
	if state != string(availabilitymodels.StateEnRoutePickup) || version != 1 {
		t.Errorf("real provider's operational state changed: state=%q version=%d, want unchanged EN_ROUTE_PICKUP/1", state, version)
	}
}

// markPickupVerifiedDirectly bypasses the real verify flow to set up a
// precondition for tests that are about MarkInProgress, not about
// verification itself -- see verification_integration_test.go for the real
// generate/verify round trip.
func markPickupVerifiedDirectly(t *testing.T, db *pgxpool.Pool, bookingID string) {
	t.Helper()
	mustExec(t, db, `UPDATE taxi_bookings SET pickup_verification_status = 'verified' WHERE id = $1`, bookingID)
}

func TestMarkInProgress_FromArrivedAtPickupSucceedsOnceVerified(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusArrivedAtPickup, availabilitymodels.StateAtPickup)
	markPickupVerifiedDirectly(t, db, bookingID)

	booking, err := svc.MarkInProgress(context.Background(), bookingID, providerID)
	if err != nil {
		t.Fatalf("MarkInProgress: %v", err)
	}
	if booking.Status != bookingmodels.StatusInProgress {
		t.Errorf("booking status = %q, want in_progress", booking.Status)
	}
	state, _ := operationalStateOf(t, db, providerID)
	if state != string(availabilitymodels.StateOnTrip) {
		t.Errorf("operational state = %q, want ON_TRIP", state)
	}
}

// TestMarkInProgress_BlockedWithoutVerification is B5's real gate, un-stubbed
// from B3's "always true" placeholder: pickup_verification_status defaults
// to 'not_required' at booking creation and is flipped to 'pending' by
// MarkArrivedAtPickup itself (never 'verified' without a real verify call),
// so a booking that just arrived and has NOT been verified must refuse to
// start.
func TestMarkInProgress_BlockedWithoutVerification(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusArrivedAtPickup, availabilitymodels.StateAtPickup)
	// Left at the schema default (not_required) deliberately -- proves the
	// gate blocks even the "never touched" case, not just an explicit
	// 'pending'.

	_, err := svc.MarkInProgress(context.Background(), bookingID, providerID)
	if err == nil {
		t.Fatal("expected MarkInProgress to be refused without pickup verification")
	}

	final, ferr := svc.GetByIDForProvider(context.Background(), bookingID, providerID)
	if ferr != nil {
		t.Fatalf("GetByIDForProvider: %v", ferr)
	}
	if final.Status != bookingmodels.StatusArrivedAtPickup {
		t.Errorf("status = %q, want unchanged arrived_at_pickup", final.Status)
	}
}

func TestMarkCompleted_ReconcilesFareAndReturnsProviderToAvailable(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, availabilitymodels.StateOnTrip)

	booking, err := svc.MarkCompleted(context.Background(), CompleteBookingInput{
		BookingID: bookingID, ProviderID: providerID, ActualDistanceKm: 12.5, ActualDurationMin: 30,
	})
	if err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	if booking.Status != bookingmodels.StatusCompleted {
		t.Errorf("booking status = %q, want completed", booking.Status)
	}
	if booking.ActualDistanceKm == nil || *booking.ActualDistanceKm != 12.5 {
		t.Errorf("actual_distance_km = %v, want 12.5", booking.ActualDistanceKm)
	}
	// fakePricing.Reconcile returns ActualDistanceKm*10000 -- proves the
	// reconciled fare came from PricingClient, not a locally recomputed value.
	if booking.FareFinalKobo == nil || *booking.FareFinalKobo != 125000 {
		t.Errorf("fare_final_kobo = %v, want 125000", booking.FareFinalKobo)
	}
	state, _ := operationalStateOf(t, db, providerID)
	if state != string(availabilitymodels.StateAvailable) {
		t.Errorf("operational state = %q, want AVAILABLE (driver released back to the pool)", state)
	}
}

func TestMarkCompleted_RejectsNegativeDistance(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, availabilitymodels.StateOnTrip)

	_, err := svc.MarkCompleted(context.Background(), CompleteBookingInput{
		BookingID: bookingID, ProviderID: providerID, ActualDistanceKm: -1, ActualDurationMin: 30,
	})
	if errCode(t, err) != apperrors.CodeValidationFailed {
		t.Fatalf("code = %v, want validation_failed", errCode(t, err))
	}
}

func TestCancelByCustomer_BeforeProviderAssignedSkipsOperationalState(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusPendingMatch, "")

	// A booking that never had a provider assigned: clear provider_id so this
	// exercises the true "no provider yet" path rather than relying on the
	// fixture helper's default assignment.
	mustExec(t, db, `UPDATE taxi_bookings SET provider_id = NULL WHERE id = $1`, bookingID)

	customerID := customerIDOf(t, db, bookingID)
	booking, err := svc.CancelByCustomer(context.Background(), bookingID, customerID, "changed my mind")
	if err != nil {
		t.Fatalf("CancelByCustomer: %v", err)
	}
	if booking.Status != bookingmodels.StatusCancelled {
		t.Errorf("booking status = %q, want cancelled", booking.Status)
	}
	if booking.CancelReason == nil || *booking.CancelReason != "changed my mind" {
		t.Errorf("cancel_reason = %v, want 'changed my mind'", booking.CancelReason)
	}
	var rows int
	if err := db.QueryRow(context.Background(),
		`SELECT count(*) FROM provider_operational_state WHERE provider_id = $1`, providerID).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 0 {
		t.Errorf("operational_state rows = %d, want 0 (no provider was ever assigned)", rows)
	}
}

func TestCancelByCustomer_WithProviderAssignedReleasesOperationalState(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusAccepted, availabilitymodels.StateOffered)
	customerID := customerIDOf(t, db, bookingID)

	booking, err := svc.CancelByCustomer(context.Background(), bookingID, customerID, "found another ride")
	if err != nil {
		t.Fatalf("CancelByCustomer: %v", err)
	}
	if booking.Status != bookingmodels.StatusCancelled {
		t.Errorf("booking status = %q, want cancelled", booking.Status)
	}
	state, _ := operationalStateOf(t, db, providerID)
	if state != string(availabilitymodels.StateAvailable) {
		t.Errorf("operational state = %q, want AVAILABLE (driver released back to the pool)", state)
	}
}

func TestCancelByCustomer_InProgressIsRefused(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, availabilitymodels.StateOnTrip)
	customerID := customerIDOf(t, db, bookingID)
	_ = providerID

	_, err := svc.CancelByCustomer(context.Background(), bookingID, customerID, "changed my mind")
	if err == nil {
		t.Fatal("expected an error: a ride already in progress must not be self-service cancellable")
	}
}

func TestCancelByProvider_ReleasesOperationalStateToAvailable(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusEnRoutePickup, availabilitymodels.StateEnRoutePickup)

	booking, err := svc.CancelByProvider(context.Background(), bookingID, providerID, "vehicle breakdown")
	if err != nil {
		t.Fatalf("CancelByProvider: %v", err)
	}
	if booking.Status != bookingmodels.StatusCancelled {
		t.Errorf("booking status = %q, want cancelled", booking.Status)
	}
	state, _ := operationalStateOf(t, db, providerID)
	if state != string(availabilitymodels.StateAvailable) {
		t.Errorf("operational state = %q, want AVAILABLE", state)
	}
}

func customerIDOf(t *testing.T, db *pgxpool.Pool, bookingID string) string {
	t.Helper()
	var customerID string
	if err := db.QueryRow(context.Background(), `SELECT customer_id::text FROM taxi_bookings WHERE id = $1`, bookingID).Scan(&customerID); err != nil {
		t.Fatalf("read customer_id: %v", err)
	}
	return customerID
}
