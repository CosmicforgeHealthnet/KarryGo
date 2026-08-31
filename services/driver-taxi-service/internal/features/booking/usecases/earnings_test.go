package bookingusecases

import (
	"context"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
)

func earningsTs(minute int) time.Time {
	return time.Date(2026, 1, 1, 0, minute, 0, 0, time.UTC)
}

func terminalBooking(id, providerID string, status bookingmodels.Status, fareFinalKobo *int64) bookingmodels.Booking {
	pid := providerID
	return bookingmodels.Booking{ID: id, ProviderID: &pid, Status: status, FareFinalKobo: fareFinalKobo, CreatedAt: earningsTs(1)}
}

type fakeOperationalHistory struct {
	asOfState string
	asOfErr   error
	history   []availabilityrepositories.HistoryEntry
	listErr   error
}

func (f *fakeOperationalHistory) StateAsOf(ctx context.Context, providerID string, at time.Time) (string, error) {
	return f.asOfState, f.asOfErr
}

func (f *fakeOperationalHistory) ListHistory(ctx context.Context, providerID string, from, until time.Time) ([]availabilityrepositories.HistoryEntry, error) {
	return f.history, f.listErr
}

func (f *fakeOperationalHistory) ListHistoryForBooking(ctx context.Context, bookingID string) ([]availabilityrepositories.HistoryEntry, error) {
	return f.history, f.listErr
}

func TestGetProviderEarningsInsights_NilHistoryIsUnavailable(t *testing.T) {
	svc := NewBookingService(Options{Bookings: newFakeBookingRepo()})
	_, err := svc.GetProviderEarningsInsights(context.Background(), "provider-1", earningsTs(0), earningsTs(60))
	if err == nil {
		t.Fatal("expected an error when OperationalHistory is not configured")
	}
}

func TestGetProviderEarningsInsights_RejectsInvertedWindow(t *testing.T) {
	svc := NewBookingService(Options{Bookings: newFakeBookingRepo(), OperationalHistory: &fakeOperationalHistory{}})
	_, err := svc.GetProviderEarningsInsights(context.Background(), "provider-1", earningsTs(60), earningsTs(0))
	if err == nil {
		t.Fatal("expected an error for until <= from")
	}
}

func TestGetProviderEarningsInsights_AggregatesOnlyThisProvidersTerminalBookings(t *testing.T) {
	bookings := newFakeBookingRepo()
	fare := int64(150000)
	pid := "provider-1"
	otherPid := "provider-2"
	bookings.byID["b1"] = terminalBooking("b1", pid, bookingmodels.StatusCompleted, &fare)
	bookings.byID["b2"] = terminalBooking("b2", otherPid, bookingmodels.StatusCompleted, &fare) // a different provider's trip
	svc := NewBookingService(Options{Bookings: bookings, OperationalHistory: &fakeOperationalHistory{}})

	insights, err := svc.GetProviderEarningsInsights(context.Background(), pid, earningsTs(0), earningsTs(60))
	if err != nil {
		t.Fatalf("GetProviderEarningsInsights: %v", err)
	}
	if insights.TripsCompleted != 1 || insights.GrossEarningsKobo != 150000 {
		t.Errorf("insights = %+v, want exactly provider-1's own single completed trip", insights)
	}
}

func TestGetProviderEarningsInsights_PropagatesHistoryLookupErrors(t *testing.T) {
	svc := NewBookingService(Options{
		Bookings:           newFakeBookingRepo(),
		OperationalHistory: &fakeOperationalHistory{asOfErr: context.DeadlineExceeded},
	})
	_, err := svc.GetProviderEarningsInsights(context.Background(), "provider-1", earningsTs(0), earningsTs(60))
	if err == nil {
		t.Fatal("expected the StateAsOf error to propagate")
	}
}
