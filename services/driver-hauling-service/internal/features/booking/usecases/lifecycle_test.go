package bookingusecases

import (
	"context"
	"testing"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
)

// lifecycleService builds a BookingService with no availability/payment wiring —
// enough to exercise the provider trip-progress usecases (ownership + status).
func lifecycleService(repo *trackingRepo) *BookingService {
	return NewBookingService(Options{
		Bookings: repo,
		Notifier: bookingclients.NewBookingNotifierWith(&recordingNotifier{}),
	})
}

// TestTripLifecycle_AdvancesThroughAllStages walks an accepted booking through
// the full provider-driven trip lifecycle and asserts each usecase advances the
// status. This is the path that was previously dead-ended: nothing could move a
// booking past `accepted`.
func TestTripLifecycle_AdvancesThroughAllStages(t *testing.T) {
	const provider = "p1"
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", ProviderID: ptr(provider),
		Status: bookingmodels.StatusAccepted,
	})
	svc := lifecycleService(repo)
	ctx := context.Background()

	steps := []struct {
		name string
		call func() (bookingmodels.PublicBooking, error)
		want string
	}{
		{"en_route_pickup", func() (bookingmodels.PublicBooking, error) { return svc.MarkEnRoutePickup(ctx, "b1", provider) }, bookingmodels.StatusEnRoutePickup},
		{"arrived", func() (bookingmodels.PublicBooking, error) { return svc.MarkArrivedAtPickup(ctx, "b1", provider) }, bookingmodels.StatusArrivedAtPickup},
		{"picked_up", func() (bookingmodels.PublicBooking, error) { return svc.ConfirmPickup(ctx, "b1", provider) }, bookingmodels.StatusPickedUp},
		{"en_route_delivery", func() (bookingmodels.PublicBooking, error) { return svc.MarkEnRouteDelivery(ctx, "b1", provider) }, bookingmodels.StatusEnRouteDelivery},
		{"delivered", func() (bookingmodels.PublicBooking, error) { return svc.ConfirmDelivery(ctx, "b1", provider) }, bookingmodels.StatusDelivered},
	}
	for _, s := range steps {
		if _, err := s.call(); err != nil {
			t.Fatalf("%s: unexpected error: %v", s.name, err)
		}
		if got := repo.statusOf("b1"); got != s.want {
			t.Fatalf("after %s: status = %s, want %s", s.name, got, s.want)
		}
	}
}

// TestTripLifecycle_RejectsWrongProvider ensures a provider cannot advance a
// booking they don't own.
func TestTripLifecycle_RejectsWrongProvider(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", ProviderID: ptr("owner"),
		Status: bookingmodels.StatusAccepted,
	})
	svc := lifecycleService(repo)

	if _, err := svc.MarkEnRoutePickup(context.Background(), "b1", "intruder"); err == nil {
		t.Fatal("expected a non-owner provider to be rejected")
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusAccepted {
		t.Fatalf("status should be unchanged, got %s", got)
	}
}

func ptr(s string) *string { return &s }
