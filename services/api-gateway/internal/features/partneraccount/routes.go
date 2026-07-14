package partneraccount

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/billing"
)

// Dependencies holds what the dashboard/internal routes need.
//
// BillingService is optional: when nil, the /billing routes are not
// registered at all (rather than registered and 500-ing), so existing wiring
// that doesn't yet construct a billing.Service keeps working unchanged.
type Dependencies struct {
	Service            *Service
	BillingService     *billing.Service
	InternalServiceKey string
}

// RegisterAccountRoutes mounts the partner DASHBOARD auth + key management +
// profile + billing routes (session JWT) and the internal service-to-service
// routes (X-Internal-Service-Key). These are separate from the partner API KEY
// execution routes registered by partner/http.RegisterPartnerRoutes.
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

	// Public OTP web signup flow. These routes intentionally do not use session,
	// API-key, or active-account middleware; email ownership is proven by OTP.
	signup := r.Group("/api/v1/partner/v1/auth/signup")
	{
		signup.POST("/start", h.StartSignup)
		signup.POST("/verify", h.VerifySignup)
		signup.POST("/resend", h.ResendSignup)
	}
	// 2. Dashboard key management: all require session.
	keys := r.Group("/api/v1/partner/v1/keys")
	keys.Use(RequirePartnerSession(deps.Service))
	{
		keys.POST("", h.GenerateKey)
		keys.GET("", h.ListKeys)
		keys.DELETE("/:id", h.RevokeKey)
	}

	// 3. Dashboard profile (Settings > Profile): requires session.
	profile := r.Group("/api/v1/partner/v1/profile")
	profile.Use(RequirePartnerSession(deps.Service))
	{
		profile.PATCH("", h.UpdateProfile)
	}

	// 4. Dashboard billing (Billing & Usage): requires session. Registered
	// only when a billing.Service was actually wired in.
	if deps.BillingService != nil {
		bh := NewBillingHandlers(deps.BillingService)
		billingGroup := r.Group("/api/v1/partner/v1/billing")
		billingGroup.Use(RequirePartnerSession(deps.Service))
		{
			billingGroup.GET("/overview", bh.GetOverview)
			billingGroup.GET("/invoices", bh.ListInvoices)
			billingGroup.POST("/payment-methods", bh.AttachPaymentMethod)
		}
	}

	// 5. Internal partner routes: service-to-service only.
	internal := r.Group("/api/v1/internal/partner")
	internal.Use(RequireServiceKey(deps.InternalServiceKey))
	{
		internal.POST("/accounts/:id/activate-live", h.ActivateLive)
		internal.PATCH("/accounts/:id/status", h.UpdateStatus)
	}
}
