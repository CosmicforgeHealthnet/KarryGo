package bookingusecases

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/customerstanding"
	"cosmicforge/logistics/shared/go/customerstanding/standingtest"
)

// seedStandingZone inserts a real market/zone and returns a markets lookup
// resolving to it, so CreateBooking writes real rows.
func seedStandingZone(t *testing.T, db *pgxpool.Pool) *fakeMarkets {
	t.Helper()
	marketID, zoneID, code := uuid.NewString(), uuid.NewString(), "STD-"+uuid.NewString()[:8]
	mustExec(t, db, `INSERT INTO markets (id, country, name, code) VALUES ($1, 'NG', 'Standing Market', $2)`, marketID, code)
	mustExec(t, db, `INSERT INTO operating_zones (id, market_id, name, code, center_lat, center_lng, match_radius_km, supported_classes)
		VALUES ($1, $2, 'Standing Zone', $3, 1.0, -10.0, 15, '{economy,comfort}')`, zoneID, marketID, code)
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.Exec(ctx, `DELETE FROM taxi_outbox WHERE aggregate_id IN (SELECT id FROM taxi_bookings WHERE market_id = $1)`, marketID)
		_, _ = db.Exec(ctx, `DELETE FROM booking_events WHERE booking_id IN (SELECT id FROM taxi_bookings WHERE market_id = $1)`, marketID)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_booking_stops WHERE booking_id IN (SELECT id FROM taxi_bookings WHERE market_id = $1)`, marketID)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_bookings WHERE market_id = $1`, marketID)
		_, _ = db.Exec(ctx, `DELETE FROM operating_zones WHERE id = $1`, zoneID)
		_, _ = db.Exec(ctx, `DELETE FROM markets WHERE id = $1`, marketID)
	})
	markets := newFakeMarkets()
	markets.zone.ID, markets.zone.MarketID = zoneID, marketID
	return markets
}

func countCustomerRows(t *testing.T, db *pgxpool.Pool, customerID string) (bookings, outbox int) {
	t.Helper()
	ctx := context.Background()
	if err := db.QueryRow(ctx, `SELECT count(*) FROM taxi_bookings WHERE customer_id = $1`, customerID).Scan(&bookings); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM taxi_outbox WHERE aggregate_id IN (SELECT id FROM taxi_bookings WHERE customer_id = $1)`, customerID).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	return bookings, outbox
}

func TestPostgresCustomerStanding_HoldBlocksNewBookingsOnly(t *testing.T) {
	db := lifecycleTestDB(t)
	markets := seedStandingZone(t, db)
	srv := standingtest.New(t, "driver-taxi-service")
	svc := NewBookingService(Options{
		Bookings: bookingrepositories.NewPostgresBookingRepository(db), Markets: markets, Pricing: newFakePricing(),
		OperationalState: availabilityrepositories.NewPostgresOperationalStateRepository(db),
		CustomerStanding: srv.Gate(500 * time.Millisecond),
	})
	ctx := context.Background()
	customer := uuid.NewString()

	// Eligible: a real booking is created.
	existing, err := svc.CreateBooking(ctx, immediateInput(customer))
	if err != nil {
		t.Fatalf("eligible create: %v", err)
	}

	// Hold applied after the booking exists.
	srv.Hold(customer, "BOOKING_HOLD", 1)
	callsBefore := srv.Calls()

	// The pre-hold booking continues its lifecycle without asking standing.
	if _, err := svc.GetByIDForCustomer(ctx, existing.ID, customer); err != nil {
		t.Fatalf("read existing booking: %v", err)
	}
	cancelled, err := svc.CancelByCustomer(ctx, existing.ID, customer, "plans changed")
	if err != nil || cancelled.Status != bookingmodels.StatusCancelled {
		t.Fatalf("existing booking lifecycle blocked by hold: %v %s", err, cancelled.Status)
	}
	if srv.Calls() != callsBefore {
		t.Fatal("lifecycle of an existing booking consulted the standing")
	}

	// N concurrent new bookings by the held customer: all refused, no rows.
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.CreateBooking(ctx, immediateInput(customer))
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, customerstanding.ErrBookingHold) {
			t.Fatalf("attempt %d not refused: %v", i, err)
		}
	}
	if bookings, outbox := countCustomerRows(t, db, customer); bookings != 1 || outbox == 0 {
		t.Fatalf("rows after refused attempts: bookings=%d outbox=%d (want only the pre-hold booking)", bookings, outbox)
	}
	outboxBefore := 0
	_, outboxBefore = countCustomerRows(t, db, customer)

	// Outage: fail closed, still nothing written.
	srv.FailWith(503)
	if _, err := svc.CreateBooking(ctx, immediateInput(customer)); !errors.Is(err, customerstanding.ErrUnavailable) {
		t.Fatalf("outage not fail-closed: %v", err)
	}
	srv.FailWith(0)
	srv.StaleBy(time.Minute)
	if _, err := svc.CreateBooking(ctx, immediateInput(customer)); !errors.Is(err, customerstanding.ErrUnavailable) {
		t.Fatalf("stale standing accepted: %v", err)
	}
	srv.StaleBy(0)
	if bookings, outbox := countCustomerRows(t, db, customer); bookings != 1 || outbox != outboxBefore {
		t.Fatalf("side effects on refusal: bookings=%d outbox=%d", bookings, outbox)
	}

	// Hold lifted: the very next booking succeeds.
	srv.Lift(customer, 2)
	if _, err := svc.CreateBooking(ctx, immediateInput(customer)); err != nil {
		t.Fatalf("lifted hold still refused: %v", err)
	}
	if bookings, _ := countCustomerRows(t, db, customer); bookings != 2 {
		t.Fatalf("bookings after lift = %d, want 2", bookings)
	}
}
