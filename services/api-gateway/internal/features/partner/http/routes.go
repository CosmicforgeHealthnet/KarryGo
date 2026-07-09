package http

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
	"cosmicforge/logistics/services/api-gateway/internal/features/partner/ratelimit"
	"cosmicforge/logistics/services/api-gateway/internal/features/partner/webhooks"
	"cosmicforge/logistics/services/api-gateway/internal/features/partneraccount"
	"cosmicforge/logistics/services/api-gateway/internal/features/partnerauth"
	"cosmicforge/logistics/services/api-gateway/internal/pkg/pricing"
)

// PartnerAPIBasePath is the external Partner API route prefix.
const PartnerAPIBasePath = "/api/v1/partner/v1"

// Dependencies holds what the Partner API routes need to be registered.
// Webhooks, RateLimiter, and AccountRepo are optional: when nil, webhook routes
// return 501, rate limiting is skipped, and active-account enforcement is skipped
// (used by unit tests without Redis/DB).
type Dependencies struct {
	KeyRepo     partnerauth.APIKeyRepository
	Deliveries  *delivery.Service
	Webhooks    *webhooks.Service
	RateLimiter *ratelimit.Limiter
	AccountRepo partneraccount.AccountRepository
	Pricing     pricing.Config
}

// RegisterPartnerRoutes mounts the Partner API route surface under
// /api/v1/partner/v1. Middleware order is: API key auth -> rate limit -> handler.
//
// TODO later: add idempotency middleware to POST /deliveries (idempotency is
// currently enforced inside the delivery service).
func RegisterPartnerRoutes(r *gin.Engine, deps Dependencies) {
	partner := r.Group(PartnerAPIBasePath)
	partner.Use(partnerauth.RequirePartnerAPIKey(deps.KeyRepo))
	if deps.AccountRepo != nil {
		partner.Use(partneraccount.RequireActiveAccount(deps.AccountRepo))
	}
	if deps.RateLimiter != nil {
		partner.Use(deps.RateLimiter.Middleware())
	}

	handler := NewHandler(deps.Deliveries, deps.Webhooks, deps.Pricing)

	partner.POST("/quotes", handler.GetQuote)

	partner.POST("/deliveries", handler.CreateDelivery)
	partner.GET("/deliveries", handler.ListDeliveries)
	partner.GET("/deliveries/:id", handler.GetDelivery)
	partner.DELETE("/deliveries/:id", handler.CancelDelivery)

	partner.POST("/webhooks", handler.RegisterWebhook)
	partner.GET("/webhooks", handler.ListWebhooks)
	partner.DELETE("/webhooks/:id", handler.DeleteWebhook)
	partner.POST("/webhooks/:id/test", handler.TestWebhook)
}
