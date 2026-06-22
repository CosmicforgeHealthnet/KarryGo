package notificationhttp

import (
	"github.com/gin-gonic/gin"

	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/notifications"
)

// RegisterNotificationRoutes wires the customer-app notification proxy. All
// routes require a customer bearer token; the recipient is taken from the token
// subject. Registration is skipped entirely when the notification client is not
// configured (no base URL), so local dev without notification-service is fine.
func RegisterNotificationRoutes(group *gin.RouterGroup, client notifications.Client, accessSigner *sharedauth.TokenSigner) {
	if client.BaseURL == "" {
		return
	}
	handler := NewHandler(client)

	protected := group.Group("/notifications")
	protected.Use(sharedauth.BearerMiddleware(accessSigner, authusecases.CustomerRole, authusecases.CustomerService))
	protected.GET("", handler.ListFeed)
	protected.POST("/realtime-token", handler.RealtimeToken)
	protected.POST("/devices", handler.RegisterDevice)
}
