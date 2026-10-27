package bookinghttp

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"
	providerauthrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/repositories"
	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	providerprofilerepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/repositories"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func RegisterBookingRoutes(
	group *gin.RouterGroup,
	svc *bookingusecases.BookingService,
	authSvc *providerauthusecases.AuthService,
	customerSigner *sharedauth.TokenSigner,
	providerRepo providerauthrepositories.ProviderRepository,
	truckRepo providerprofilerepositories.TruckRepository,
	redisClient *redis.Client,
) {
	handler := NewBookingHandler(svc, providerRepo, truckRepo)

	// Public fare estimation (no auth)
	group.POST("/customer/bookings/estimate", handler.EstimateFare)

	// Live-track websocket. Registered outside the bearer group because a
	// websocket handshake cannot carry an Authorization header; the handler
	// validates the token from the query string and checks that the caller
	// owns the booking before subscribing them to anything.
	if redisClient != nil {
		track := NewTrackHandler(svc, customerSigner, redisClient)
		group.GET("/customer/bookings/:id/track", track.StreamBookingTrack)
	}

	// Customer routes (bearer auth: role=customer, service=customer)
	customerGroup := group.Group("/customer")
	customerGroup.Use(sharedauth.BearerMiddleware(customerSigner, "customer", "customer"))
	customerGroup.POST("/bookings", handler.CreateBooking)
	customerGroup.GET("/bookings", handler.ListCustomerBookings)
	customerGroup.GET("/bookings/:id", handler.GetCustomerBooking)
	customerGroup.GET("/bookings/:id/waiting", handler.CustomerWaiting)
	customerGroup.GET("/bookings/:id/location", handler.GetBookingLocation)
	customerGroup.POST("/bookings/:id/card-payment", handler.InitiateCardPayment)
	customerGroup.PUT("/bookings/:id/cancel", handler.CancelBooking)
	customerGroup.POST("/bookings/:id/review", handler.SubmitReview)
	customerGroup.GET("/bookings/:id/review", handler.GetCustomerBookingReview)
	customerGroup.POST("/bookings/:id/verify-driver", handler.VerifyDriverAtPickup)
	customerGroup.GET("/bookings/:id/verification-code", handler.GetVerificationCode)
	RegisterReceiverCredentialRoute(customerGroup, handler)
	customerGroup.POST("/bookings/:id/verification-code/resend", handler.ResendVerificationCode)
	customerGroup.GET("/providers/:id", handler.GetPublicProvider)
	customerGroup.GET("/trucks/:id", handler.GetPublicTruck)

	// Provider routes (bearer auth: role=truck_provider, service=hauling)
	providerGroup := group.Group("/provider")
	providerGroup.Use(sharedauth.BearerMiddleware(authSvc.AccessSigner(), providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	providerGroup.GET("/earnings", handler.GetProviderEarnings)
	providerGroup.GET("/bookings", handler.ListProviderBookings)
	providerGroup.GET("/bookings/:id", handler.GetProviderBooking)
	providerGroup.GET("/bookings/:id/review", handler.GetProviderBookingReview)
	providerGroup.PUT("/bookings/:id/accept", handler.AcceptBooking)
	providerGroup.PUT("/bookings/:id/reject", handler.RejectBooking)
	providerGroup.PUT("/bookings/:id/en-route-pickup", handler.MarkEnRoutePickup)
	providerGroup.PUT("/bookings/:id/arrived", handler.MarkArrivedAtPickup)
	providerGroup.GET("/bookings/:id/pickup-credential", handler.GetPickupCredential)
	providerGroup.POST("/bookings/:id/pickup-credential/resend", handler.ResendPickupCredential)
	providerGroup.PUT("/bookings/:id/pickup-confirmed", handler.ConfirmPickup)
	providerGroup.PUT("/bookings/:id/en-route-delivery", handler.MarkEnRouteDelivery)
	providerGroup.PUT("/bookings/:id/arrived-dropoff", handler.MarkArrivedAtDropoff)
	providerGroup.GET("/bookings/:id/waiting", handler.ProviderWaiting)
	providerGroup.PUT("/bookings/:id/delivered", handler.ConfirmDelivery)
	providerGroup.PUT("/bookings/:id/cancel", handler.CancelByProvider)
}

func RegisterReceiverCredentialRoute(customerGroup *gin.RouterGroup, handler *BookingHandler) {
	customerGroup.GET("/receivers/bookings/:id/credentials/:challengeRef", handler.ReadReceiverCredential)
}
