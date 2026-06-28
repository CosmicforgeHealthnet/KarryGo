package bookingusecases

import (
	"context"
	"testing"
	"time"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/notifications"
)

// recordingNotifier captures every notification request it is asked to send.
type recordingNotifier struct {
	requests []notifications.Request
}

func (r *recordingNotifier) Send(_ context.Context, request notifications.Request) (notifications.SendResponse, error) {
	r.requests = append(r.requests, request)
	return notifications.SendResponse{MessageID: "test", Status: "queued"}, nil
}

func newServiceWithNotifier(repo *fakeBookingRepo) (*BookingService, *recordingNotifier) {
	rec := &recordingNotifier{}
	notifier := bookingclients.NewBookingNotifierWith(rec)
	svc := NewBookingService(Options{
		Bookings:            repo,
		Availability:        noopAvailabilityStore{},
		Notifier:            notifier,
		MatchTimeoutSeconds: 30,
	})
	return svc, rec
}

func TestAcceptBookingNotifiesCustomer(t *testing.T) {
	providerID := "provider-1"
	repo := &fakeBookingRepo{booking: bookingmodels.Booking{
		ID:         "booking-1",
		CustomerID: "customer-1",
		ProviderID: &providerID,
		Status:     bookingmodels.StatusAwaitingAcceptance,
	}}
	svc, rec := newServiceWithNotifier(repo)

	if _, err := svc.AcceptBooking(context.Background(), "booking-1", providerID); err != nil {
		t.Fatalf("AcceptBooking() error = %v", err)
	}

	assertNotified(t, rec, notifications.EventBookingAccepted, notifications.RecipientCustomer, "customer-1")
}

func TestConfirmPickupNotifiesCustomer(t *testing.T) {
	providerID := "provider-1"
	repo := &fakeBookingRepo{booking: bookingmodels.Booking{
		ID:         "booking-1",
		CustomerID: "customer-1",
		ProviderID: &providerID,
		Status:     bookingmodels.StatusAccepted,
	}}
	svc, rec := newServiceWithNotifier(repo)

	if _, err := svc.ConfirmPickup(context.Background(), "booking-1", providerID); err != nil {
		t.Fatalf("ConfirmPickup() error = %v", err)
	}

	assertNotified(t, rec, notifications.EventCargoPickedUp, notifications.RecipientCustomer, "customer-1")
}

func TestConfirmDeliveryNotifiesCustomer(t *testing.T) {
	providerID := "provider-1"
	repo := &fakeBookingRepo{booking: bookingmodels.Booking{
		ID:         "booking-1",
		CustomerID: "customer-1",
		ProviderID: &providerID,
		Status:     bookingmodels.StatusPickedUp,
	}}
	svc, rec := newServiceWithNotifier(repo)

	if _, err := svc.ConfirmDelivery(context.Background(), "booking-1", providerID); err != nil {
		t.Fatalf("ConfirmDelivery() error = %v", err)
	}

	assertNotified(t, rec, notifications.EventCargoDelivered, notifications.RecipientCustomer, "customer-1")
}

func TestCancelByProviderNotifiesCustomer(t *testing.T) {
	providerID := "provider-1"
	repo := &fakeBookingRepo{booking: bookingmodels.Booking{
		ID:         "booking-1",
		CustomerID: "customer-1",
		ProviderID: &providerID,
		Status:     bookingmodels.StatusCancelled,
	}}
	svc, rec := newServiceWithNotifier(repo)

	if _, err := svc.CancelByProvider(context.Background(), "booking-1", providerID, "unavailable"); err != nil {
		t.Fatalf("CancelByProvider() error = %v", err)
	}

	assertNotified(t, rec, notifications.EventBookingCancelledByProvider, notifications.RecipientCustomer, "customer-1")
}

func assertNotified(t *testing.T, rec *recordingNotifier, eventType, recipientType, recipientID string) {
	t.Helper()
	for _, req := range rec.requests {
		if req.EventType == eventType {
			if req.Recipient.Type != recipientType || req.Recipient.ID != recipientID {
				t.Fatalf("event %s sent to %s/%s, want %s/%s", eventType, req.Recipient.Type, req.Recipient.ID, recipientType, recipientID)
			}
			if req.IDempotencyKey == "" {
				t.Fatalf("event %s missing idempotency key", eventType)
			}
			return
		}
	}
	t.Fatalf("expected notification for event %s, got %d requests", eventType, len(rec.requests))
}

// ─── fakes ──────────────────────────────────────────────────────────────────

// fakeBookingRepo returns a single configured booking for every read and echoes
// it back for every mutation, so usecase notification side-effects can be tested
// without a database.
type fakeBookingRepo struct {
	booking bookingmodels.Booking
}

func (f *fakeBookingRepo) Create(_ context.Context, b bookingmodels.Booking) (bookingmodels.Booking, error) {
	return b, nil
}
func (f *fakeBookingRepo) GetByID(_ context.Context, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) GetByIDForCustomer(_ context.Context, _, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) ListByCustomer(_ context.Context, _ string, _, _ int) ([]bookingmodels.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListByProvider(_ context.Context, _ string, _, _ int) ([]bookingmodels.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) UpdateStatus(_ context.Context, _, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) AssignProvider(_ context.Context, _, _, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) MarkMatched(_ context.Context, _, _, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) MarkAccepted(_ context.Context, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) MarkEnRoutePickup(_ context.Context, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) MarkArrivedAtPickup(_ context.Context, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) MarkPickedUp(_ context.Context, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) MarkEnRouteDelivery(_ context.Context, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) MarkDelivered(_ context.Context, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) MarkCompleted(_ context.Context, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) CancelByCustomer(_ context.Context, _, _, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) CancelByProvider(_ context.Context, _, _, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) ResetToMatching(_ context.Context, _ string) (bookingmodels.Booking, error) {
	return f.booking, nil
}
func (f *fakeBookingRepo) SetPayment(_ context.Context, _, paymentStatus, paymentIntentID string) (bookingmodels.Booking, error) {
	f.booking.PaymentStatus = paymentStatus
	if paymentIntentID != "" {
		f.booking.PaymentIntentID = &paymentIntentID
	}
	return f.booking, nil
}
func (f *fakeBookingRepo) ListDeliveredForAutoComplete(_ context.Context, _ time.Time) ([]bookingmodels.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) AddEvent(_ context.Context, _ bookingmodels.BookingEvent) error {
	return nil
}
func (f *fakeBookingRepo) CreateReview(_ context.Context, r bookingmodels.BookingReview) (bookingmodels.BookingReview, error) {
	return r, nil
}
func (f *fakeBookingRepo) GetReviewByBooking(_ context.Context, _ string) (bookingmodels.BookingReview, error) {
	return bookingmodels.BookingReview{}, nil
}

// noopAvailabilityStore satisfies AvailabilityStore for paths that never touch it.
type noopAvailabilityStore struct{}

func (noopAvailabilityStore) SetOnline(context.Context, availabilityrepositories.ProviderStatus, time.Duration) error {
	return nil
}
func (noopAvailabilityStore) SetOffline(context.Context, string) error { return nil }
func (noopAvailabilityStore) Heartbeat(context.Context, string, float64, float64, time.Duration) error {
	return nil
}
func (noopAvailabilityStore) CountOnline(context.Context) (int64, error) { return 0, nil }
func (noopAvailabilityStore) GetOnlineProviders(context.Context) ([]availabilityrepositories.ProviderStatus, error) {
	return nil, nil
}
func (noopAvailabilityStore) GetProviderStatus(context.Context, string) (availabilityrepositories.ProviderStatus, bool, error) {
	return availabilityrepositories.ProviderStatus{}, false, nil
}
func (noopAvailabilityStore) AcquireMatchLock(context.Context, string, string, time.Duration) (bool, error) {
	return false, nil
}
func (noopAvailabilityStore) ReleaseMatchLock(context.Context, string) error { return nil }
