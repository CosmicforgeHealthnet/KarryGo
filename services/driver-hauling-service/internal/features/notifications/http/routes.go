package notificationhttp

import (
	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/notifications"
)

// RegisterNotificationRoutes wires the provider- and customer-app notification
// proxies. Each route requires the matching bearer token; the recipient is the
// token subject. Registration is skipped when the notification client is not
// configured.
func RegisterNotificationRoutes(
	group *gin.RouterGroup,
	client notifications.Client,
	accessSigner *sharedauth.TokenSigner,
	customerSigner *sharedauth.TokenSigner,
) {
	if client.BaseURL == "" {
		return
	}

	providerHandler := NewHandlerFor(client, notifications.RecipientProvider)
	provider := group.Group("/provider/notifications")
	provider.Use(sharedauth.BearerMiddleware(accessSigner, providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	provider.GET("", providerHandler.ListFeed)
	provider.POST("/realtime-token", providerHandler.RealtimeToken)
	provider.POST("/devices", providerHandler.RegisterDevice)

	// Customer realtime channel: the fast path for booking status + driver
	// updates on the customer app, mirroring the provider setup.
	customerHandler := NewHandlerFor(client, notifications.RecipientCustomer)
	customer := group.Group("/customer/notifications")
	customer.Use(sharedauth.BearerMiddleware(customerSigner, "customer", "customer"))
	customer.GET("", customerHandler.ListFeed)
	customer.POST("/realtime-token", customerHandler.RealtimeToken)
	customer.POST("/devices", customerHandler.RegisterDevice)
}
