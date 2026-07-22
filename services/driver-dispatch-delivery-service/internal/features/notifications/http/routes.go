package notificationhttp

import (
	"github.com/gin-gonic/gin"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/notifications"
)

// RegisterNotificationRoutes wires the rider- and customer-app notification
// proxies (feed, realtime token, device registration). Each route requires the
// matching bearer token; the recipient is the token subject. Registration is
// skipped entirely when the notification client is not configured, so local
// dev without notification-service keeps working (apps fall back to polling).
func RegisterNotificationRoutes(
	engine *gin.Engine,
	client notifications.Client,
	riderSigner *sharedauth.TokenSigner,
	customerSigner *sharedauth.TokenSigner,
) {
	if client.BaseURL == "" {
		return
	}

	riderHandler := NewHandlerFor(client, notifications.RecipientProvider)
	rider := engine.Group("/api/v1/provider/notifications")
	rider.Use(sharedauth.BearerMiddleware(riderSigner, "dispatch_provider", "dispatch"))
	rider.GET("", riderHandler.ListFeed)
	rider.POST("/realtime-token", riderHandler.RealtimeToken)
	rider.POST("/devices", riderHandler.RegisterDevice)

	// Customer realtime channel: the fast path for booking status + rider
	// updates on the customer app, mirroring the hauling setup.
	customerHandler := NewHandlerFor(client, notifications.RecipientCustomer)
	customer := engine.Group("/api/v1/customer/notifications")
	customer.Use(sharedauth.BearerMiddleware(customerSigner, "customer", "customer"))
	customer.GET("", customerHandler.ListFeed)
	customer.POST("/realtime-token", customerHandler.RealtimeToken)
	customer.POST("/devices", customerHandler.RegisterDevice)
}
