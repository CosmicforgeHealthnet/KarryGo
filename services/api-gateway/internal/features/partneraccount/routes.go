package partneraccount

import "github.com/gin-gonic/gin"

// Dependencies holds what the dashboard/internal routes need.
type Dependencies struct {
	Service            *Service
	InternalServiceKey string
}

// RegisterAccountRoutes mounts the partner DASHBOARD auth + key management routes
// (session JWT) and the internal service-to-service routes (X-Internal-Service-Key).
// These are separate from the partner API KEY execution routes.
func RegisterAccountRoutes(r *gin.Engine, deps Dependencies) {
	h := NewHandlers(deps.Service)

	// 1. Dashboard auth: register/login public; logout/me require session.
	auth := r.Group("/api/v1/partner/v1/auth")
	{
		auth.POST("/register", h.Register)
		auth.POST("/login", h.Login)

		protected := auth.Group("")
		protected.Use(RequirePartnerSession(deps.Service))
		{
			protected.POST("/logout", h.Logout)
			protected.GET("/me", h.Me)
		}
	}

	// 2. Dashboard key management: all require session.
	keys := r.Group("/api/v1/partner/v1/keys")
	keys.Use(RequirePartnerSession(deps.Service))
	{
		keys.POST("", h.GenerateKey)
		keys.GET("", h.ListKeys)
		keys.DELETE("/:id", h.RevokeKey)
	}

	// 4. Internal partner routes: service-to-service only.
	internal := r.Group("/api/v1/internal/partner")
	internal.Use(RequireServiceKey(deps.InternalServiceKey))
	{
		internal.POST("/accounts/:id/activate-live", h.ActivateLive)
		internal.PATCH("/accounts/:id/status", h.UpdateStatus)
	}
}
