package bookingusecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingreviews "cosmicforge/logistics/services/taxi-service/internal/features/booking/reviews"
	"cosmicforge/logistics/shared/go/apperrors"
)

// SubmitReviewInput is one side's rating of the other over a completed
// booking. Which direction it becomes is decided by which of
// SubmitCustomerReview/SubmitProviderReview the caller invokes, not by a
// field on this struct -- the HTTP layer already knows which role is
// authenticated, so there is nothing for a caller to get wrong here.
type SubmitReviewInput struct {
	Rating           int
	ReviewText       string
	Tags             []string
	RecommendsDriver *bool
}

func (s *BookingService) submitReview(ctx context.Context, booking bookingmodels.Booking, direction bookingreviews.Direction, input SubmitReviewInput) (bookingreviews.Review, error) {
	if s.reviews == nil {
		return bookingreviews.Review{}, apperrors.Unavailable("Reviews are not available right now.", nil)
	}
	if input.Rating < 1 || input.Rating > 5 {
		return bookingreviews.Review{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "rating", Message: "Rating must be between 1 and 5."},
		})
	}
	if booking.Status != bookingmodels.StatusCompleted {
		return bookingreviews.Review{}, apperrors.Conflict("You can only review a completed ride.", nil)
	}
	if booking.ProviderID == nil {
		// Cannot happen for a genuinely completed booking (MarkCompleted's
		// own guard requires an assigned provider), but a nil-pointer
		// dereference below would be a far worse failure mode than a clear
		// error, so guard it explicitly rather than trust the invariant
		// silently.
		return bookingreviews.Review{}, apperrors.Conflict("This ride has no assigned driver to review.", nil)
	}

	review, err := s.reviews.Create(ctx, bookingreviews.Review{
		BookingID: booking.ID, Direction: direction,
		CustomerID: booking.CustomerID, ProviderID: *booking.ProviderID,
		Rating: input.Rating, ReviewText: strings.TrimSpace(input.ReviewText),
		Tags: input.Tags, RecommendsDriver: input.RecommendsDriver,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return bookingreviews.Review{}, apperrors.Conflict("You have already reviewed this ride.", nil)
		}
		return bookingreviews.Review{}, err
	}
	return review, nil
}

// SubmitCustomerReview rates the driver over a completed ride the customer
// booked.
func (s *BookingService) SubmitCustomerReview(ctx context.Context, bookingID, customerID string, input SubmitReviewInput) (bookingreviews.Review, error) {
	booking, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID)
	if err != nil {
		return bookingreviews.Review{}, err
	}
	return s.submitReview(ctx, booking, bookingreviews.DirectionCustomerToProvider, input)
}

// SubmitProviderReview rates the customer over a completed ride the driver
// carried.
func (s *BookingService) SubmitProviderReview(ctx context.Context, bookingID, providerID string, input SubmitReviewInput) (bookingreviews.Review, error) {
	booking, err := s.bookings.GetByIDForProvider(ctx, bookingID, providerID)
	if err != nil {
		return bookingreviews.Review{}, err
	}
	return s.submitReview(ctx, booking, bookingreviews.DirectionProviderToCustomer, input)
}

// ReviewView is what one party is allowed to see of one direction's review.
// Rating is NEVER withheld once submitted -- only ReviewText/Tags/
// RecommendsDriver are gated behind Visible, per the addendum §13
// requirement that avoids a retaliatory-review timing problem (seeing the
// other side's score is fine; reading their detailed complaint before you
// have committed to your own is what gets gamed).
type ReviewView struct {
	Submitted        bool     `json:"submitted"`
	Visible          bool     `json:"visible"`
	Rating           *int     `json:"rating,omitempty"`
	ReviewText       string   `json:"review_text,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	RecommendsDriver *bool    `json:"recommends_driver,omitempty"`
}

// BookingReviewsView is the response shape for either GET endpoint: "Mine"
// is always fully visible to its own author (an author can always re-read
// what they wrote), "Theirs" is gated by the reveal condition.
type BookingReviewsView struct {
	Mine   ReviewView `json:"mine"`
	Theirs ReviewView `json:"theirs"`
}

func reviewViewFrom(r *bookingreviews.Review, revealText bool) ReviewView {
	if r == nil {
		return ReviewView{Submitted: false}
	}
	rating := r.Rating
	v := ReviewView{Submitted: true, Visible: revealText, Rating: &rating}
	if revealText {
		v.ReviewText = r.ReviewText
		v.Tags = r.Tags
		v.RecommendsDriver = r.RecommendsDriver
	}
	return v
}

// revealTheirs implements the addendum §13 withholding rule: the
// counterpart's text/tags unlock once BOTH sides have submitted, or once
// ReviewRevealWindow has elapsed since completion, whichever comes first --
// a party who never gets a review back is not locked out of ever reading
// what was said about them once enough time has passed.
func (s *BookingService) revealTheirs(mine, theirs *bookingreviews.Review, completedAt *time.Time) bool {
	if mine != nil && theirs != nil {
		return true
	}
	if completedAt == nil || s.reviewRevealWindow <= 0 {
		return false
	}
	return time.Since(*completedAt) > s.reviewRevealWindow
}

func (s *BookingService) reviewsView(ctx context.Context, booking bookingmodels.Booking, myDirection, theirDirection bookingreviews.Direction) (BookingReviewsView, error) {
	if s.reviews == nil {
		return BookingReviewsView{}, apperrors.Unavailable("Reviews are not available right now.", nil)
	}
	all, err := s.reviews.GetForBooking(ctx, booking.ID)
	if err != nil {
		return BookingReviewsView{}, err
	}
	var mine, theirs *bookingreviews.Review
	for i := range all {
		switch all[i].Direction {
		case myDirection:
			mine = &all[i]
		case theirDirection:
			theirs = &all[i]
		}
	}
	reveal := s.revealTheirs(mine, theirs, booking.CompletedAt)
	return BookingReviewsView{
		Mine:   reviewViewFrom(mine, true),
		Theirs: reviewViewFrom(theirs, reveal),
	}, nil
}

// GetCustomerReviewsView is the customer's own view of both directions'
// reviews over a booking they made.
func (s *BookingService) GetCustomerReviewsView(ctx context.Context, bookingID, customerID string) (BookingReviewsView, error) {
	booking, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID)
	if err != nil {
		return BookingReviewsView{}, err
	}
	return s.reviewsView(ctx, booking, bookingreviews.DirectionCustomerToProvider, bookingreviews.DirectionProviderToCustomer)
}

// GetProviderReviewsView is the driver's own view of both directions'
// reviews over a booking they carried.
func (s *BookingService) GetProviderReviewsView(ctx context.Context, bookingID, providerID string) (BookingReviewsView, error) {
	booking, err := s.bookings.GetByIDForProvider(ctx, bookingID, providerID)
	if err != nil {
		return BookingReviewsView{}, err
	}
	return s.reviewsView(ctx, booking, bookingreviews.DirectionProviderToCustomer, bookingreviews.DirectionCustomerToProvider)
}
