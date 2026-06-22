package notificationhttp

import (
	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/notifications"
)

// RegisterNotificationRoutes wires the provider-app notification proxy. All
// routes require a provider bearer token; the recipient is the token subject.
// Registration is skipped when the notification client is not configured.
func RegisterNotificationRoutes(group *gin.RouterGroup, client notifications.Client, accessSigner *sharedauth.TokenSigner) {
	if client.BaseURL == "" {
		return
	}
	handler := NewHandler(client)

	protected := group.Group("/provider/notifications")
	protected.Use(sharedauth.BearerMiddleware(accessSigner, providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	protected.GET("", handler.ListFeed)
	protected.POST("/realtime-token", handler.RealtimeToken)
	protected.POST("/devices", handler.RegisterDevice)
}
