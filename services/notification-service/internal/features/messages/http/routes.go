package messagehttp

import (
	"time"

	"github.com/gin-gonic/gin"

	messageclients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	messageusecases "cosmicforge/logistics/services/notification-service/internal/features/messages/usecases"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func RegisterRoutes(group *gin.RouterGroup, service *messageusecases.NotificationService, hub *messageclients.WebSocketHub, secrets serviceauth.Secrets) {
	handler := NewHandler(service, hub)
	verifier := serviceauth.NewVerifier(secrets, 5*time.Minute)

	protected := group.Group("")
	protected.Use(serviceauth.Middleware(verifier))
	protected.POST("/send", handler.Send)
	protected.POST("/devices", handler.RegisterDevice)
	protected.POST("/realtime/token", handler.RealtimeToken)
	protected.GET("/messages", handler.ListMessages)
	protected.GET("/messages/:id", handler.GetMessage)

	group.GET("/ws", handler.WebSocket)
}
