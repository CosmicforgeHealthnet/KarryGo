package standingtest

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/customerstanding"
)

func TestFakeSpeaksTheRealContract(t *testing.T) {
	srv := New(t, "driver-hauling-service")
	c := srv.Client(200 * time.Millisecond)
	ctx := context.Background()
	if s, err := c.Check(ctx, "c1"); err != nil || !s.BookingEligible || s.StandingVersion != 0 {
		t.Fatal("default", s, err)
	}
	srv.Hold("c1", "BOOKING_HOLD", 1)
	if _, err := c.Check(ctx, "c1"); !errors.Is(err, customerstanding.ErrBookingHold) {
		t.Fatal("hold", err)
	}
	srv.Lift("c1", 2)
	if s, err := c.Check(ctx, "c1"); err != nil || s.StandingVersion != 2 {
		t.Fatal("lift", s, err)
	}
	srv.StaleBy(time.Minute)
	if _, err := c.Check(ctx, "c1"); !errors.Is(err, customerstanding.ErrUnavailable) {
		t.Fatal("stale", err)
	}
	srv.StaleBy(0)
	srv.FailWith(http.StatusServiceUnavailable)
	if _, err := c.Check(ctx, "c1"); !errors.Is(err, customerstanding.ErrUnavailable) {
		t.Fatal("fail", err)
	}
	srv.FailWith(0)
	srv.Delay(time.Second)
	if _, err := c.Check(ctx, "c1"); !errors.Is(err, customerstanding.ErrUnavailable) {
		t.Fatal("timeout", err)
	}
	if srv.Calls() != 6 {
		t.Fatal("calls", srv.Calls())
	}
}
