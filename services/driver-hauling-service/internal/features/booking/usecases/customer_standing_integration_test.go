package bookingusecases

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/database"
	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	"cosmicforge/logistics/shared/go/customerstanding"
	"cosmicforge/logistics/shared/go/customerstanding/standingtest"
)

func standingTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required for customer standing PostgreSQL tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, sourceFile, _, _ := runtime.Caller(0)
	serviceRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", ".."))
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(serviceRoot); err != nil {
		t.Fatal(err)
	}
	migrateErr := database.ApplyMigrations(context.Background(), pool)
	if err = os.Chdir(previous); err != nil {
		t.Fatal(err)
	}
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}
	return pool
}

// TestPostgresCustomerStanding_HoldBlocksNewBookingsOnly pins, against the
// real haulage_bookings schema, that refused attempts write no booking or
// booking event, that a pre-hold booking continues, and that a lifted hold
// takes effect on the very next booking.
func TestPostgresCustomerStanding_HoldBlocksNewBookingsOnly(t *testing.T) {
	pool := standingTestPool(t)
	ctx := context.Background()
	customer := uuid.NewString()
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM booking_events WHERE booking_id IN (SELECT id FROM haulage_bookings WHERE customer_id = $1)`, customer)
		_, _ = pool.Exec(c, `DELETE FROM haulage_bookings WHERE customer_id = $1`, customer)
	})
	srv := standingtest.New(t, "driver-hauling-service")
	store := newCountingStore()
	store.setProviders(onlineStore().providers)
	svc := NewBookingService(Options{
		Bookings: bookingrepo.NewPostgresBookingRepository(pool), Availability: store, Trucks: allTrucks{capacity: 10000},
		Notifier:            bookingclients.NewBookingNotifierWith(&recordingNotifier{}),
		MatchTimeoutSeconds: 1, SearchWindowSeconds: 1, MaxRadiusKm: 25,
		CustomerStanding: srv.Gate(500 * time.Millisecond),
	})
	count := func() (bookings, events int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM haulage_bookings WHERE customer_id = $1`, customer).Scan(&bookings); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM booking_events WHERE booking_id IN (SELECT id FROM haulage_bookings WHERE customer_id = $1)`, customer).Scan(&events); err != nil {
			t.Fatal(err)
		}
		return bookings, events
	}

	existing, err := svc.CreateBooking(ctx, standingInput(customer))
	if err != nil {
		t.Fatalf("eligible create: %v", err)
	}
	srv.Hold(customer, "BOOKING_HOLD", 1)
	baseBookings, baseEvents := count()

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.CreateBooking(ctx, standingInput(customer))
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, customerstanding.ErrBookingHold) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	srv.FailWith(500)
	if _, err := svc.CreateBooking(ctx, standingInput(customer)); !errors.Is(err, customerstanding.ErrUnavailable) {
		t.Fatalf("outage: %v", err)
	}
	srv.FailWith(0)
	if b, e := count(); b != baseBookings || e != baseEvents {
		t.Fatalf("refused attempts wrote rows: bookings %d->%d events %d->%d", baseBookings, b, baseEvents, e)
	}

	// The pre-hold booking still runs its customer lifecycle.
	calls := srv.Calls()
	if _, err := svc.CancelBooking(ctx, existing.ID, customer, "changed my mind"); err != nil {
		t.Fatalf("pre-hold booking cancel blocked: %v", err)
	}
	if srv.Calls() != calls {
		t.Fatal("existing booking lifecycle consulted standing")
	}

	srv.Lift(customer, 2)
	if _, err := svc.CreateBooking(ctx, standingInput(customer)); err != nil {
		t.Fatalf("lifted hold still refused: %v", err)
	}
	if b, _ := count(); b != baseBookings+1 {
		t.Fatalf("bookings after lift = %d, want %d", b, baseBookings+1)
	}
}
