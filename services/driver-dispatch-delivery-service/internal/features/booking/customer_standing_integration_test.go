package booking

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/customerstanding"
	"cosmicforge/logistics/shared/go/customerstanding/standingtest"
)

// TestPostgresCustomerStandingWritesNothingWhenRefused pins the "no side
// effects" guarantee against the real dispatch_bookings schema.
func TestPostgresCustomerStandingWritesNothingWhenRefused(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("DISPATCH_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	customer := uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM dispatch_bookings WHERE customer_id = $1`, customer)
	})
	srv := standingtest.New(t, "driver-dispatch-delivery-service")
	payments := &fakePayments{}
	broadcast := &fakeBroadcast{}
	service := NewService(NewPostgresRepository(db), broadcast, &fakeTrips{}, &fakeLive{}, payments, &noopEvents{}, 10).
		WithCustomerStanding(srv.Gate(500 * time.Millisecond))
	rows := func() int {
		var n int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM dispatch_bookings WHERE customer_id = $1`, customer).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	srv.Hold(customer, "BOOKING_HOLD", 1)
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = service.CreateBookingWithKey(ctx, customer, uuid.NewString(), validCreateInput())
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, customerstanding.ErrBookingHold) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	srv.FailWith(503)
	if _, err := service.CreateBookingWithKey(ctx, customer, uuid.NewString(), validCreateInput()); !errors.Is(err, customerstanding.ErrUnavailable) {
		t.Fatalf("outage: %v", err)
	}
	if rows() != 0 || len(payments.holds) != 0 || len(broadcast.started) != 0 {
		t.Fatalf("refused attempts left side effects: rows=%d holds=%d broadcasts=%d", rows(), len(payments.holds), len(broadcast.started))
	}

	srv.FailWith(0)
	srv.Lift(customer, 2)
	view, err := service.CreateBookingWithKey(ctx, customer, uuid.NewString(), validCreateInput())
	if err != nil {
		t.Fatalf("lifted hold still refused: %v", err)
	}
	var paymentStatus string
	if err := db.QueryRow(ctx, `SELECT payment_status FROM dispatch_bookings WHERE id = $1`, view.ID).Scan(&paymentStatus); err != nil {
		t.Fatal(err)
	}
	if rows() != 1 || paymentStatus != PaymentHeld || len(broadcast.started) != 1 {
		t.Fatalf("eligible booking: rows=%d payment=%s broadcasts=%d", rows(), paymentStatus, len(broadcast.started))
	}
}

// A retry of a create that already produced a booking before the hold replays
// that booking; a genuinely new booking under a new key is still refused.
func TestPostgresCustomerStandingReplaysPreHoldBookingForItsKey(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("DISPATCH_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	customer := uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM dispatch_bookings WHERE customer_id = $1`, customer)
	})
	srv := standingtest.New(t, "driver-dispatch-delivery-service")
	payments := &fakePayments{}
	broadcast := &fakeBroadcast{}
	service := NewService(NewPostgresRepository(db), broadcast, &fakeTrips{}, &fakeLive{}, payments, &noopEvents{}, 10).
		WithCustomerStanding(srv.Gate(500 * time.Millisecond))

	key := uuid.NewString()
	first, err := service.CreateBookingWithKey(ctx, customer, key, validCreateInput())
	if err != nil {
		t.Fatalf("eligible create: %v", err)
	}
	srv.Hold(customer, "BOOKING_HOLD", 1)
	callsBefore := srv.Calls()
	replay, err := service.CreateBookingWithKey(ctx, customer, key, validCreateInput())
	if err != nil {
		t.Fatalf("replay of the pre-hold booking was refused: %v", err)
	}
	if replay.ID != first.ID {
		t.Fatalf("replay returned %s, want the existing booking %s", replay.ID, first.ID)
	}
	if srv.Calls() != callsBefore {
		t.Fatal("a replay of an existing booking asked for standing")
	}
	if _, err := service.CreateBookingWithKey(ctx, customer, uuid.NewString(), validCreateInput()); !errors.Is(err, customerstanding.ErrBookingHold) {
		t.Fatalf("new booking under hold: %v", err)
	}
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM dispatch_bookings WHERE customer_id = $1`, customer).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(payments.holds) != 1 {
		t.Fatalf("replay or refusal had side effects: rows=%d holds=%d", n, len(payments.holds))
	}
	// A replay of a still-searching booking re-drives its own (idempotent)
	// broadcast; the refused request must not have started any other.
	for _, id := range broadcast.started {
		if id != first.ID {
			t.Fatalf("broadcast started for %s, only %s exists", id, first.ID)
		}
	}
}
