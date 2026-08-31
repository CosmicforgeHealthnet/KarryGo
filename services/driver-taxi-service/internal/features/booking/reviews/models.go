// Package bookingreviews persists the addendum §13 two-way review: a
// customer rates their driver, a driver rates their customer, over the same
// completed booking. Withholding the counterpart's free text/tags until
// both sides have submitted (or a configured window expires) is a usecase-
// layer decision (see bookingusecases' reviews.go) -- this package only
// stores and retrieves the two rows, it does not decide what a caller is
// allowed to see.
package bookingreviews

import "time"

// Direction distinguishes which party is rating whom over one booking.
// booking_reviews enforces UNIQUE (booking_id, direction): each side may
// review a given ride exactly once.
type Direction string

const (
	DirectionCustomerToProvider Direction = "customer_to_provider"
	DirectionProviderToCustomer Direction = "provider_to_customer"
)

// Review is one submitted rating. JSON tags matter here: this struct is
// returned directly from the two POST .../review endpoints (the caller is
// always the review's own author, so there is nothing to strip the way
// bookingmodels.Booking strips internal-only fields via its own Public()) --
// found live serializing as bare Go field names (ID, BookingID, ...) before
// these tags were added, breaking this service's snake_case wire convention.
type Review struct {
	ID               string    `json:"id"`
	BookingID        string    `json:"booking_id"`
	Direction        Direction `json:"direction"`
	CustomerID       string    `json:"customer_id"`
	ProviderID       string    `json:"provider_id"`
	Rating           int       `json:"rating"`
	ReviewText       string    `json:"review_text"`
	Tags             []string  `json:"tags"`
	RecommendsDriver *bool     `json:"recommends_driver,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}
