package bookingusecases

import (
	"context"
	"testing"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

func newServiceForReviewTests(repo *fakeBookingRepo) *BookingService {
	return NewBookingService(Options{
		Bookings:            repo,
		Availability:        noopAvailabilityStore{},
		Notifier:            bookingclients.NewBookingNotifierWith(&recordingNotifier{}),
		MatchTimeoutSeconds: 30,
	})
}

func TestSubmitReview_SecondAttemptReturnsConflict(t *testing.T) {
	providerID := "provider-1"
	repo := &fakeBookingRepo{booking: bookingmodels.Booking{
		ID:         "booking-1",
		CustomerID: "customer-1",
		ProviderID: &providerID,
		Status:     bookingmodels.StatusCompleted,
	}}
	svc := newServiceForReviewTests(repo)

	if _, err := svc.SubmitReview(context.Background(), SubmitReviewInput{
		BookingID:  "booking-1",
		CustomerID: "customer-1",
		Rating:     5,
	}); err != nil {
		t.Fatalf("first SubmitReview() error = %v", err)
	}

	_, err := svc.SubmitReview(context.Background(), SubmitReviewInput{
		BookingID:  "booking-1",
		CustomerID: "customer-1",
		Rating:     4,
	})
	if err == nil {
		t.Fatal("expected second SubmitReview() to fail, got nil error")
	}
	if got := apperrors.From(err).Code; got != apperrors.CodeConflict {
		t.Fatalf("expected conflict code, got %s", got)
	}
}

func TestGetCustomerBookingReview_ReturnsSubmittedReview(t *testing.T) {
	providerID := "provider-1"
	repo := &fakeBookingRepo{booking: bookingmodels.Booking{
		ID:         "booking-1",
		CustomerID: "customer-1",
		ProviderID: &providerID,
		Status:     bookingmodels.StatusCompleted,
	}}
	svc := newServiceForReviewTests(repo)

	if _, err := svc.SubmitReview(context.Background(), SubmitReviewInput{
		BookingID:  "booking-1",
		CustomerID: "customer-1",
		Rating:     5,
		ReviewText: "Great trip",
	}); err != nil {
		t.Fatalf("SubmitReview() error = %v", err)
	}

	review, err := svc.GetCustomerBookingReview(context.Background(), "booking-1", "customer-1")
	if err != nil {
		t.Fatalf("GetCustomerBookingReview() error = %v", err)
	}
	if review.Rating != 5 || review.ReviewText != "Great trip" {
		t.Fatalf("unexpected review returned: %+v", review)
	}
}

func TestGetCustomerBookingReview_NotFoundWhenNoReviewYet(t *testing.T) {
	providerID := "provider-1"
	repo := &fakeBookingRepo{booking: bookingmodels.Booking{
		ID:         "booking-1",
		CustomerID: "customer-1",
		ProviderID: &providerID,
		Status:     bookingmodels.StatusCompleted,
	}}
	svc := newServiceForReviewTests(repo)

	_, err := svc.GetCustomerBookingReview(context.Background(), "booking-1", "customer-1")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if got := apperrors.From(err).Code; got != apperrors.CodeNotFound {
		t.Fatalf("expected not-found code, got %s", got)
	}
}

func TestGetCustomerBookingReview_WrongCustomerRejected(t *testing.T) {
	providerID := "provider-1"
	repo := &fakeBookingRepo{booking: bookingmodels.Booking{
		ID:         "booking-1",
		CustomerID: "customer-1",
		ProviderID: &providerID,
		Status:     bookingmodels.StatusCompleted,
	}}
	svc := newServiceForReviewTests(repo)

	if _, err := svc.SubmitReview(context.Background(), SubmitReviewInput{
		BookingID:  "booking-1",
		CustomerID: "customer-1",
		Rating:     5,
	}); err != nil {
		t.Fatalf("SubmitReview() error = %v", err)
	}

	if _, err := svc.GetCustomerBookingReview(context.Background(), "booking-1", "someone-else"); err == nil {
		t.Fatal("expected ownership check to reject a different customer")
	}
}
