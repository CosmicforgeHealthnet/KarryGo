package notificationhttp

import (
	"github.com/gin-gonic/gin"

	supporthttp "cosmicforge/logistics/services/support-dispute-service/internal/features/support/http"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/notifications"
)

// RegisterNotificationRoutes wires the realtime/feed/device proxy for the
// customer + provider apps so support chat can run live over websockets.
// Registration is skipped when the notification client is not configured.
func RegisterNotificationRoutes(
	group *gin.RouterGroup,
	client notifications.Client,
	customerSecret []byte,
	providerGroups []supporthttp.ProviderGroup,
) {
	if client.BaseURL == "" {
		return
	}

	customerHandler := NewHandlerFor(client, notifications.RecipientCustomer)
	customer := group.Group("/customer/notifications")
	customer.Use(sharedauth.BearerMiddleware(sharedauth.NewTokenSigner(customerSecret), "customer", "customer"))
	customer.GET("", customerHandler.ListFeed)
	customer.POST("/realtime-token", customerHandler.RealtimeToken)
	customer.POST("/devices", customerHandler.RegisterDevice)

	providerHandler := NewHandlerFor(client, notifications.RecipientProvider)
	for _, pg := range providerGroups {
		if len(pg.Secret) == 0 {
			continue
		}
		grp := group.Group(pg.Prefix + "/notifications")
		grp.Use(sharedauth.BearerMiddleware(sharedauth.NewTokenSigner(pg.Secret), pg.Role, pg.Service))
		grp.GET("", providerHandler.ListFeed)
		grp.POST("/realtime-token", providerHandler.RealtimeToken)
		grp.POST("/devices", providerHandler.RegisterDevice)
	}
}
