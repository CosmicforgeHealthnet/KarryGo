package booking

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCompletionTimeoutDoesNotClaimPaidAndRetryUsesSameBooking(t *testing.T) {
	repo := newFakeRepo()
	payments := &fakePayments{settlementErr: errors.New("response lost")}
	service := newTestService(repo, &fakeBroadcast{repo: repo}, &fakeTrips{}, &fakeLive{}, payments)
	view := createSearchingBooking(t, service)
	when := time.Now().UTC()
	if err := service.HandleTripCompleted(context.Background(), view.ID, when); err == nil {
		t.Fatal("unknown outcome reported as confirmed")
	}
	if got := repo.bookings[view.ID]; got.Status != StatusCompleted || got.PaymentStatus != PaymentHeld {
		t.Fatalf("completion projection=%s/%s", got.Status, got.PaymentStatus)
	}
	payments.settlementErr = nil
	if err := service.HandleTripCompleted(context.Background(), view.ID, when); err != nil {
		t.Fatal(err)
	}
	if repo.bookings[view.ID].PaymentStatus != PaymentPaid {
		t.Fatal("authoritative confirmation was not projected")
	}
	if len(payments.settlements) != 2 || payments.settlements[0] != view.ID || payments.settlements[1] != view.ID {
		t.Fatalf("unstable financial identity: %v", payments.settlements)
	}
}
