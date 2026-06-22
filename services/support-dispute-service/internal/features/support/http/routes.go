package supporthttp

import (
	"time"

	"github.com/gin-gonic/gin"

	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// RegisterRoutes wires all support & dispute routes.
//
// Customer / provider callers use bearer tokens.
// Admin / internal callers use HMAC service-auth.
func RegisterRoutes(
	group *gin.RouterGroup,
	service *supportusecases.SupportService,
	customerAccessSecret []byte,
	serviceSecrets serviceauth.Secrets,
) {
	handler := NewHandler(service)

	// ── Customer routes ──────────────────────────────────────────────────────
	customer := group.Group("")
	customer.Use(sharedauth.BearerMiddleware(
		sharedauth.NewTokenSigner(customerAccessSecret),
		"customer",
		"customer",
	))
	customer.POST("/complaints", handler.CreateComplaint)
	customer.GET("/complaints", handler.MyComplaints)
	customer.GET("/complaints/:id", handler.GetComplaint)
	customer.POST("/complaints/:id/evidence", handler.AddEvidence)
	customer.GET("/complaints/:id/evidence", handler.ListEvidence)
	customer.POST("/complaints/:id/dispute", handler.EscalateToDispute)
	customer.GET("/complaints/:id/dispute", handler.GetDispute)
	customer.POST("/support-chat/start", handler.StartSupportChat)
	customer.POST("/complaints/:id/messages", handler.SendMessage)
	customer.GET("/complaints/:id/messages", handler.ListMessages)

	// ── Admin / internal routes (service-auth) ───────────────────────────────
	admin := group.Group("/admin")
	admin.Use(serviceauth.Middleware(serviceauth.NewVerifier(serviceSecrets, 5*time.Minute)))
	admin.PUT("/complaints/:id/status", handler.AdminUpdateStatus)
	admin.POST("/disputes/:id/resolve", handler.AdminResolveDispute)
	admin.POST("/complaints/:id/messages", handler.AdminSendMessage)
	admin.GET("/complaints/:id/messages", handler.AdminListMessages)
}
