package bookingusecases

import (
	"context"
	"fmt"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
)

func TestListTripHistory_OnlyThisDriversFinishedRecentRidesCapped(t *testing.T) {
	repo := newFakeBookingRepo()
	me, other := "driver-1", "driver-2"
	now := time.Now()
	add := func(id string, provider *string, status bookingmodels.Status, age time.Duration) {
		repo.byID[id] = bookingmodels.Booking{ID: id, ProviderID: provider, Status: status, CreatedAt: now.Add(-age)}
	}
	for i := 0; i < TripHistoryLimit+5; i++ {
		add(fmt.Sprintf("done-%d", i), &me, bookingmodels.StatusCompleted, time.Duration(i)*time.Minute)
	}
	add("cancelled", &me, bookingmodels.StatusCancelled, time.Hour)
	add("active", &me, bookingmodels.StatusInProgress, time.Minute)
	add("old", &me, bookingmodels.StatusCompleted, TripHistoryWindow+time.Hour)
	add("theirs", &other, bookingmodels.StatusCompleted, time.Minute)

	svc := newService(repo, newFakeMarkets(), newFakePricing())
	trips, err := svc.ListTripHistory(context.Background(), me)
	if err != nil {
		t.Fatal(err)
	}
	if len(trips) != TripHistoryLimit {
		t.Fatalf("want the history capped at %d, got %d", TripHistoryLimit, len(trips))
	}
	for _, b := range trips {
		if b.ID == "active" || b.ID == "old" || b.ID == "theirs" {
			t.Fatalf("history must hold only this driver's finished rides from the window, got %s", b.ID)
		}
	}
}
