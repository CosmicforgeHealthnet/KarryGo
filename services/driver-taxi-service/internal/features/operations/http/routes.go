package operationshttp

import (
	"github.com/gin-gonic/gin"

	bookingusecases "cosmicforge/logistics/services/taxi-service/internal/features/booking/usecases"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// RegisterOperationsRoutes mounts /metrics (public -- see GetMetrics's own
// doc comment) always, and the full /internal ops surface only when
// verifier is non-nil (no TAXI_SERVICE_SECRETS configured means no internal
// caller is trusted yet, matching the same "disabled integration, no
// half-open surface" convention as every other optional integration in this
// service's cmd/main.go).
func RegisterOperationsRoutes(group *gin.RouterGroup, bookings *bookingusecases.BookingService, verifier *serviceauth.Verifier) {
	handler := NewHandler(bookings)
	group.GET("/metrics", handler.GetMetrics)

	if verifier == nil {
		return
	}
	internal := group.Group("/internal")
	internal.Use(serviceauth.Middleware(verifier))

	internal.GET("/operations/live", handler.GetLive)
	internal.GET("/operations/unmatched", handler.GetUnmatched)
	internal.GET("/bookings/:id/replay", handler.GetReplay)
	internal.POST("/bookings/:id/actions/cancel", handler.PostCancel)
	internal.POST("/bookings/:id/actions/force-recover", handler.PostForceRecover)
	internal.POST("/bookings/:id/actions/pause-provider", handler.PostPauseProvider)
	internal.POST("/bookings/:id/actions/release-hold", handler.PostReleaseHold)
}
