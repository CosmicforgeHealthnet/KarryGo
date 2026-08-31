package bookingusecases

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingreviews "cosmicforge/logistics/services/taxi-service/internal/features/booking/reviews"
)

type fakeReviewsRepo struct {
	byBookingAndDirection map[string]bookingreviews.Review
	createErr             error
	nextID                int
}

func newFakeReviewsRepo() *fakeReviewsRepo {
	return &fakeReviewsRepo{byBookingAndDirection: map[string]bookingreviews.Review{}}
}

func reviewKey(bookingID string, direction bookingreviews.Direction) string {
	return bookingID + "|" + string(direction)
}

func (f *fakeReviewsRepo) Create(ctx context.Context, r bookingreviews.Review) (bookingreviews.Review, error) {
	if f.createErr != nil {
		return bookingreviews.Review{}, f.createErr
	}
	key := reviewKey(r.BookingID, r.Direction)
	if _, exists := f.byBookingAndDirection[key]; exists {
		return bookingreviews.Review{}, &pgconn.PgError{Code: "23505", ConstraintName: "booking_reviews_booking_id_direction_key"}
	}
	f.nextID++
	r.ID = "review-" + itoa(f.nextID)
	r.CreatedAt = time.Now()
	f.byBookingAndDirection[key] = r
	return r, nil
}

func (f *fakeReviewsRepo) GetForBooking(ctx context.Context, bookingID string) ([]bookingreviews.Review, error) {
	var out []bookingreviews.Review
	for _, r := range f.byBookingAndDirection {
		if r.BookingID == bookingID {
			out = append(out, r)
		}
	}
	return out, nil
}

func completedBooking(id, customerID, providerID string, completedAt *time.Time) bookingmodels.Booking {
	pid := providerID
	return bookingmodels.Booking{
		ID: id, CustomerID: customerID, ProviderID: &pid,
		Status: bookingmodels.StatusCompleted, CompletedAt: completedAt,
	}
}

func newReviewTestService(bookings *fakeBookingRepo, reviews bookingreviews.Repository, revealWindow time.Duration) *BookingService {
	return NewBookingService(Options{Bookings: bookings, Reviews: reviews, ReviewRevealWindow: revealWindow})
}

// ─── SubmitCustomerReview / SubmitProviderReview ───────────────────────────

func TestSubmitCustomerReview_NilReviewsIsUnavailable(t *testing.T) {
	svc := NewBookingService(Options{Bookings: newFakeBookingRepo()})
	_, err := svc.SubmitCustomerReview(context.Background(), "b1", "customer-1", SubmitReviewInput{Rating: 5})
	if err == nil {
		t.Fatal("expected an error when Reviews is not configured")
	}
}

func TestSubmitCustomerReview_RatingOutOfRangeRejected(t *testing.T) {
	bookings := newFakeBookingRepo()
	now := time.Now()
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &now)
	svc := newReviewTestService(bookings, newFakeReviewsRepo(), time.Hour)

	for _, rating := range []int{0, 6, -1} {
		if _, err := svc.SubmitCustomerReview(context.Background(), "b1", "customer-1", SubmitReviewInput{Rating: rating}); err == nil {
			t.Errorf("rating %d: expected a validation error", rating)
		}
	}
}

func TestSubmitCustomerReview_BlockedUnlessCompleted(t *testing.T) {
	bookings := newFakeBookingRepo()
	pid := "provider-1"
	bookings.byID["b1"] = bookingmodels.Booking{ID: "b1", CustomerID: "customer-1", ProviderID: &pid, Status: bookingmodels.StatusInProgress}
	svc := newReviewTestService(bookings, newFakeReviewsRepo(), time.Hour)

	if _, err := svc.SubmitCustomerReview(context.Background(), "b1", "customer-1", SubmitReviewInput{Rating: 5}); err == nil {
		t.Fatal("expected a conflict for a non-completed ride")
	}
}

func TestSubmitCustomerReview_WrongCustomerRejected(t *testing.T) {
	bookings := newFakeBookingRepo()
	now := time.Now()
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &now)
	svc := newReviewTestService(bookings, newFakeReviewsRepo(), time.Hour)

	if _, err := svc.SubmitCustomerReview(context.Background(), "b1", "someone-else", SubmitReviewInput{Rating: 5}); err == nil {
		t.Fatal("expected an error for a customer who did not book this ride")
	}
}

func TestSubmitCustomerReview_Succeeds(t *testing.T) {
	bookings := newFakeBookingRepo()
	now := time.Now()
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &now)
	svc := newReviewTestService(bookings, newFakeReviewsRepo(), time.Hour)

	recommends := true
	review, err := svc.SubmitCustomerReview(context.Background(), "b1", "customer-1", SubmitReviewInput{
		Rating: 4, ReviewText: "Good ride", Tags: []string{"clean_car"}, RecommendsDriver: &recommends,
	})
	if err != nil {
		t.Fatalf("SubmitCustomerReview: %v", err)
	}
	if review.Direction != bookingreviews.DirectionCustomerToProvider || review.ProviderID != "provider-1" {
		t.Errorf("review = %+v, want customer_to_provider direction with provider-1", review)
	}
}

func TestSubmitCustomerReview_DuplicateSubmissionRefused(t *testing.T) {
	bookings := newFakeBookingRepo()
	now := time.Now()
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &now)
	svc := newReviewTestService(bookings, newFakeReviewsRepo(), time.Hour)

	if _, err := svc.SubmitCustomerReview(context.Background(), "b1", "customer-1", SubmitReviewInput{Rating: 5}); err != nil {
		t.Fatalf("first submission: %v", err)
	}
	if _, err := svc.SubmitCustomerReview(context.Background(), "b1", "customer-1", SubmitReviewInput{Rating: 3}); err == nil {
		t.Fatal("expected the second submission over the same booking to be refused")
	}
}

func TestSubmitProviderReview_Succeeds(t *testing.T) {
	bookings := newFakeBookingRepo()
	now := time.Now()
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &now)
	svc := newReviewTestService(bookings, newFakeReviewsRepo(), time.Hour)

	review, err := svc.SubmitProviderReview(context.Background(), "b1", "provider-1", SubmitReviewInput{Rating: 5})
	if err != nil {
		t.Fatalf("SubmitProviderReview: %v", err)
	}
	if review.Direction != bookingreviews.DirectionProviderToCustomer || review.CustomerID != "customer-1" {
		t.Errorf("review = %+v, want provider_to_customer direction with customer-1", review)
	}
}

func TestSubmitProviderReview_WrongProviderRejected(t *testing.T) {
	bookings := newFakeBookingRepo()
	now := time.Now()
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &now)
	svc := newReviewTestService(bookings, newFakeReviewsRepo(), time.Hour)

	if _, err := svc.SubmitProviderReview(context.Background(), "b1", "someone-else", SubmitReviewInput{Rating: 5}); err == nil {
		t.Fatal("expected an error for a provider who did not carry this ride")
	}
}

// ─── GetCustomerReviewsView / GetProviderReviewsView (withholding rule) ────

func TestGetReviewsView_NeitherSubmittedYet(t *testing.T) {
	bookings := newFakeBookingRepo()
	now := time.Now()
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &now)
	svc := newReviewTestService(bookings, newFakeReviewsRepo(), time.Hour)

	view, err := svc.GetCustomerReviewsView(context.Background(), "b1", "customer-1")
	if err != nil {
		t.Fatalf("GetCustomerReviewsView: %v", err)
	}
	if view.Mine.Submitted || view.Theirs.Submitted {
		t.Errorf("view = %+v, want neither submitted", view)
	}
}

func TestGetReviewsView_OwnReviewAlwaysFullyVisible(t *testing.T) {
	bookings := newFakeBookingRepo()
	now := time.Now()
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &now)
	reviews := newFakeReviewsRepo()
	svc := newReviewTestService(bookings, reviews, time.Hour)

	if _, err := svc.SubmitCustomerReview(context.Background(), "b1", "customer-1", SubmitReviewInput{Rating: 4, ReviewText: "secret complaint"}); err != nil {
		t.Fatalf("SubmitCustomerReview: %v", err)
	}
	view, err := svc.GetCustomerReviewsView(context.Background(), "b1", "customer-1")
	if err != nil {
		t.Fatalf("GetCustomerReviewsView: %v", err)
	}
	if !view.Mine.Submitted || !view.Mine.Visible || view.Mine.ReviewText != "secret complaint" {
		t.Errorf("Mine = %+v, want the author's own review always fully visible", view.Mine)
	}
}

func TestGetReviewsView_CounterpartTextWithheldUntilBothSubmit(t *testing.T) {
	bookings := newFakeBookingRepo()
	now := time.Now()
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &now)
	reviews := newFakeReviewsRepo()
	svc := newReviewTestService(bookings, reviews, time.Hour) // reveal window not yet elapsed

	if _, err := svc.SubmitProviderReview(context.Background(), "b1", "provider-1", SubmitReviewInput{Rating: 2, ReviewText: "rude passenger"}); err != nil {
		t.Fatalf("SubmitProviderReview: %v", err)
	}
	view, err := svc.GetCustomerReviewsView(context.Background(), "b1", "customer-1")
	if err != nil {
		t.Fatalf("GetCustomerReviewsView: %v", err)
	}
	if !view.Theirs.Submitted {
		t.Fatal("Theirs.Submitted = false, want true -- the driver DID submit")
	}
	if view.Theirs.Visible {
		t.Error("Theirs.Visible = true, want false -- the customer has not submitted their own review yet")
	}
	if view.Theirs.Rating == nil || *view.Theirs.Rating != 2 {
		t.Errorf("Theirs.Rating = %v, want 2 -- ratings are NEVER withheld", view.Theirs.Rating)
	}
	if view.Theirs.ReviewText != "" {
		t.Errorf("Theirs.ReviewText = %q, want empty while withheld", view.Theirs.ReviewText)
	}
}

func TestGetReviewsView_CounterpartTextRevealedOnceBothSubmit(t *testing.T) {
	bookings := newFakeBookingRepo()
	now := time.Now()
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &now)
	reviews := newFakeReviewsRepo()
	svc := newReviewTestService(bookings, reviews, time.Hour)

	if _, err := svc.SubmitProviderReview(context.Background(), "b1", "provider-1", SubmitReviewInput{Rating: 2, ReviewText: "rude passenger"}); err != nil {
		t.Fatalf("SubmitProviderReview: %v", err)
	}
	if _, err := svc.SubmitCustomerReview(context.Background(), "b1", "customer-1", SubmitReviewInput{Rating: 5, ReviewText: "great driver"}); err != nil {
		t.Fatalf("SubmitCustomerReview: %v", err)
	}

	view, err := svc.GetCustomerReviewsView(context.Background(), "b1", "customer-1")
	if err != nil {
		t.Fatalf("GetCustomerReviewsView: %v", err)
	}
	if !view.Theirs.Visible || view.Theirs.ReviewText != "rude passenger" {
		t.Errorf("Theirs = %+v, want fully revealed once both sides have submitted", view.Theirs)
	}
}

func TestGetReviewsView_CounterpartTextRevealedAfterWindowExpires(t *testing.T) {
	bookings := newFakeBookingRepo()
	longAgo := time.Now().Add(-2 * time.Hour)
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &longAgo)
	reviews := newFakeReviewsRepo()
	svc := newReviewTestService(bookings, reviews, time.Hour) // window already elapsed since completion

	if _, err := svc.SubmitProviderReview(context.Background(), "b1", "provider-1", SubmitReviewInput{Rating: 2, ReviewText: "rude passenger"}); err != nil {
		t.Fatalf("SubmitProviderReview: %v", err)
	}
	// Customer never submits their own review.
	view, err := svc.GetCustomerReviewsView(context.Background(), "b1", "customer-1")
	if err != nil {
		t.Fatalf("GetCustomerReviewsView: %v", err)
	}
	if !view.Theirs.Visible {
		t.Error("Theirs.Visible = false, want true -- the reveal window has elapsed since completion")
	}
}

func TestGetReviewsView_ProviderPerspectiveMirrorsCustomer(t *testing.T) {
	bookings := newFakeBookingRepo()
	now := time.Now()
	bookings.byID["b1"] = completedBooking("b1", "customer-1", "provider-1", &now)
	reviews := newFakeReviewsRepo()
	svc := newReviewTestService(bookings, reviews, time.Hour)

	if _, err := svc.SubmitCustomerReview(context.Background(), "b1", "customer-1", SubmitReviewInput{Rating: 5, ReviewText: "great driver"}); err != nil {
		t.Fatalf("SubmitCustomerReview: %v", err)
	}
	view, err := svc.GetProviderReviewsView(context.Background(), "b1", "provider-1")
	if err != nil {
		t.Fatalf("GetProviderReviewsView: %v", err)
	}
	if !view.Theirs.Submitted || view.Theirs.Visible {
		t.Errorf("Theirs = %+v, want submitted but withheld from the provider's own perspective too", view.Theirs)
	}
	if view.Mine.Submitted {
		t.Error("Mine.Submitted = true, want false -- the provider has not reviewed yet")
	}
}
