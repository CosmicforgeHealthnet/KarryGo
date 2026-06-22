package bookinghttp

import (
	"github.com/gin-gonic/gin"

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
) {
	handler := NewBookingHandler(svc, providerRepo, truckRepo)

	// Public fare estimation (no auth)
	group.POST("/customer/bookings/estimate", handler.EstimateFare)

	// Customer routes (bearer auth: role=customer, service=customer)
	customerGroup := group.Group("/customer")
	customerGroup.Use(sharedauth.BearerMiddleware(customerSigner, "customer", "customer"))
	customerGroup.POST("/bookings", handler.CreateBooking)
	customerGroup.GET("/bookings", handler.ListCustomerBookings)
	customerGroup.GET("/bookings/:id", handler.GetCustomerBooking)
	customerGroup.PUT("/bookings/:id/cancel", handler.CancelBooking)
	customerGroup.POST("/bookings/:id/review", handler.SubmitReview)
	customerGroup.GET("/providers/:id", handler.GetPublicProvider)
	customerGroup.GET("/trucks/:id", handler.GetPublicTruck)

	// Provider routes (bearer auth: role=truck_provider, service=hauling)
	providerGroup := group.Group("/provider")
	providerGroup.Use(sharedauth.BearerMiddleware(authSvc.AccessSigner(), providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	providerGroup.GET("/bookings", handler.ListProviderBookings)
	providerGroup.GET("/bookings/:id", handler.GetProviderBooking)
	providerGroup.PUT("/bookings/:id/accept", handler.AcceptBooking)
	providerGroup.PUT("/bookings/:id/reject", handler.RejectBooking)
	providerGroup.PUT("/bookings/:id/pickup-confirmed", handler.ConfirmPickup)
	providerGroup.PUT("/bookings/:id/delivered", handler.ConfirmDelivery)
	providerGroup.PUT("/bookings/:id/cancel", handler.CancelByProvider)
}
