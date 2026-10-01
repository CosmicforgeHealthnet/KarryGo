package bookinghttp

import (
	"github.com/gin-gonic/gin"

	bookingusecases "cosmicforge/logistics/services/taxi-service/internal/features/booking/usecases"
	providerauthusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/usecases"
	"cosmicforge/logistics/services/taxi-service/internal/idempotency"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

// RegisterBookingRoutes mounts the passenger-facing booking surface, the
// driver-facing lifecycle transitions (phase B3), and matching (phase B2):
// a booking now reaches a provider-owned status through the real matcher,
// spawned from CreateBooking, not just direct database assignment.
func RegisterBookingRoutes(
	group *gin.RouterGroup,
	bookings *bookingusecases.BookingService,
	providerSigner *sharedauth.TokenSigner,
	customerSigner *sharedauth.TokenSigner,
	idempotencyStore *idempotency.Store,
	// financialPrivacy resolves the provider's corporate visibility policy
	// before any provider handler runs. Nil is valid and means "no Fleet
	// integration", in which case every provider is independent and the
	// response contract is unchanged.
	financialPrivacy gin.HandlerFunc,
) {
	handler := NewBookingHandler(bookings)

	// Public: the fare-tile screen a passenger sees before logging in.
	group.POST("/customer/bookings/estimate", handler.EstimateFare)

	customer := group.Group("/customer")
	customer.Use(sharedauth.BearerMiddleware(customerSigner, "customer", "customer"))

	// Addendum §7.1: creation requires an Idempotency-Key and a resend with a
	// different body on the same key is a 409, not a silent double-booking.
	// The idempotency scope is the AUTHENTICATED customer (claims, set by the
	// BearerMiddleware above), never a caller-supplied header -- otherwise one
	// customer could collide with or read back another's cached response by
	// guessing their key.
	customer.POST("/bookings", func(c *gin.Context) {
		id, ok := customerID(c)
		if !ok {
			return
		}
		idempotency.Execute(c, idempotencyStore, id, "POST /customer/bookings", func() {
			handler.CreateBooking(c)
		})
	})
	customer.GET("/bookings", handler.ListBookings)
	customer.GET("/bookings/:id", handler.GetBooking)
	customer.PUT("/bookings/:id/cancel", handler.CancelByCustomer)
	// Pickup verification (phase B5): the passenger's own display screen.
	customer.GET("/bookings/:id/pickup-code", handler.GetPickupCode)
	customer.POST("/bookings/:id/pickup-code/resend", handler.ResendPickupCode)
	// Parcel drop-off: the receiver's code (404 on a passenger ride, so the
	// concept is never discoverable where it does not apply).
	customer.GET("/bookings/:id/delivery-code", handler.GetDeliveryCode)
	customer.POST("/bookings/:id/delivery-code/resend", handler.ResendDeliveryCode)
	// Two-way reviews (phase B8).
	customer.POST("/bookings/:id/review", handler.SubmitCustomerReview)
	customer.GET("/bookings/:id/review", handler.GetCustomerReviews)

	provider := group.Group("/provider")
	provider.Use(sharedauth.BearerMiddleware(providerSigner, providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	// Registered after BearerMiddleware so the provider identity is already
	// resolved, and before every provider route so no handler can answer
	// before the visibility policy is known (Section 8 TAX-018).
	if financialPrivacy != nil {
		provider.Use(financialPrivacy)
	}
	provider.GET("/bookings", handler.ListActiveBookings)
	provider.GET("/bookings/:id", handler.GetBookingForProvider)
	provider.PUT("/bookings/:id/accept", handler.AcceptBooking)
	provider.PUT("/bookings/:id/reject", handler.RejectBooking)
	provider.PUT("/bookings/:id/en-route-pickup", handler.MarkEnRoutePickup)
	provider.PUT("/bookings/:id/arrived", handler.MarkArrivedAtPickup)
	provider.POST("/bookings/:id/verify-pickup", handler.VerifyPickup)
	provider.POST("/bookings/:id/verify-delivery", handler.VerifyDelivery)
	provider.PUT("/bookings/:id/start", handler.StartTrip)
	provider.PUT("/bookings/:id/complete", handler.CompleteTrip)
	provider.PUT("/bookings/:id/cancel", handler.CancelByProvider)
	// Active-trip telemetry (phase B6): batched GPS upload, not scoped to one
	// booking id in the path since a driver app may queue points across a
	// brief connectivity gap that spans more than one status change.
	provider.POST("/location/events", handler.IngestLocationPoints)
	// Two-way reviews + earnings insights (phase B8).
	provider.POST("/bookings/:id/review", handler.SubmitProviderReview)
	provider.GET("/bookings/:id/review", handler.GetProviderReviews)
	provider.GET("/earnings/insights", handler.GetEarningsInsights)
}
