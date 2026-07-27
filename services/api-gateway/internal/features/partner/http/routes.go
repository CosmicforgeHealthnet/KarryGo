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
//
// AccountService is also optional: when nil, the read-only routes fall back to
// API-key-only auth exactly as before, so existing tests that don't wire a
// Service keep passing unchanged. When set, those routes accept EITHER a
// dashboard session token OR a partner API key.
type Dependencies struct {
	KeyRepo        partnerauth.APIKeyRepository
	Deliveries     *delivery.Service
	Webhooks       *webhooks.Service
	RateLimiter    *ratelimit.Limiter
	AccountRepo    partneraccount.AccountRepository
	AccountService *partneraccount.Service
	Pricing        pricing.Config
}

// RegisterPartnerRoutes mounts the Partner API route surface under
// /api/v1/partner/v1.
//
// Two route groups are registered under the same prefix:
//
//  1. Write/mutate routes (create/cancel delivery, register/delete/test webhook,
//     quotes) stay API-key-only, unchanged from before: API key auth -> active
//     account check -> rate limit -> handler.
//
//  2. Read-only routes (list/get deliveries, list webhooks, list webhook
//     deliveries) accept EITHER a dashboard session token OR a partner API key.
//
// Every route (except POST /quotes, which touches no partner-owned data) is
// additionally gated by partnerauth.RequirePermission - an API key must hold at
// least one of the listed permission scopes (migration 005) or the request is
// rejected with 403 insufficient_scope. Session-authenticated dashboard
// requests are never scope-checked (see RequirePermission's doc comment): the
// logged-in business owner always has full access to their own data; scoping
// only matters for API keys that might be handed to a third party.
//
// Permission-to-route mapping (best interpretation of the Figma checkboxes,
// which name four scopes but do not specify which routes each one covers):
//   - create_deliveries: POST /deliveries, DELETE /deliveries/:id (cancel)
//   - read_deliveries OR track_deliveries: GET /deliveries, GET /deliveries/:id
//     (this codebase has one endpoint doing both "read" and "track" a delivery,
//     so either scope is accepted)
//   - manage_webhooks: every /webhooks* route
//
// TODO later: add idempotency middleware to POST /deliveries (idempotency is
// currently enforced inside the delivery service).
func RegisterPartnerRoutes(r *gin.Engine, deps Dependencies) {
	partner := r.Group(PartnerAPIBasePath)
	handler := NewHandler(deps.Deliveries, deps.Webhooks, deps.AccountService, deps.Pricing)

	// --- 1. API-key-only: writes + quote ------------------------------------
	writes := partner.Group("")
	writes.Use(partnerauth.RequirePartnerAPIKey(deps.KeyRepo))
	if deps.AccountRepo != nil {
		writes.Use(partneraccount.RequireActiveAccount(deps.AccountRepo))
	}
	if deps.RateLimiter != nil {
		writes.Use(deps.RateLimiter.Middleware())
	}

	writes.POST("/quotes", handler.GetQuote)
	writes.POST("/deliveries",
		partnerauth.RequirePermission(partnerauth.PermissionCreateDeliveries), handler.CreateDelivery)
	writes.DELETE("/deliveries/:id",
		partnerauth.RequirePermission(partnerauth.PermissionCreateDeliveries), handler.CancelDelivery)
	writes.POST("/webhooks",
		partnerauth.RequirePermission(partnerauth.PermissionManageWebhooks), handler.RegisterWebhook)
	writes.DELETE("/webhooks/:id",
		partnerauth.RequirePermission(partnerauth.PermissionManageWebhooks), handler.DeleteWebhook)
	writes.POST("/webhooks/:id/test",
		partnerauth.RequirePermission(partnerauth.PermissionManageWebhooks), handler.TestWebhook)

	// --- 2. Session-or-API-key: dashboard + integrator reads ----------------
	readPerm := partnerauth.RequirePermission(partnerauth.PermissionReadDeliveries, partnerauth.PermissionTrackDeliveries)
	webhookPerm := partnerauth.RequirePermission(partnerauth.PermissionManageWebhooks)
	// API key management permissions - requires manage_webhooks scope
	keyPerm := partnerauth.RequirePermission(partnerauth.PermissionManageWebhooks)

	if deps.AccountService != nil {
		reads := partner.Group("")
		reads.Use(partneraccount.RequirePartnerSessionOrAPIKey(deps.AccountService, deps.KeyRepo))
		if deps.AccountRepo != nil {
			reads.Use(partneraccount.RequireActiveAccount(deps.AccountRepo))
		}
		if deps.RateLimiter != nil {
			reads.Use(deps.RateLimiter.Middleware())
		}

		reads.GET("/deliveries", readPerm, handler.ListDeliveries)
		reads.GET("/deliveries/:id", readPerm, handler.GetDelivery)
		reads.GET("/webhooks", webhookPerm, handler.ListWebhooks)
		reads.GET("/webhooks/:id/deliveries", webhookPerm, handler.ListWebhookDeliveries)

		// Webhook log endpoints for dashboard
		reads.GET("/webhook-logs", webhookPerm, handler.ListWebhookLogs)
		reads.GET("/webhook-logs/:id", webhookPerm, handler.GetWebhookLog)

		// NEW: API key management endpoints (requires manage_webhooks scope)
		reads.POST("/api-keys", keyPerm, handler.CreateAPIKey)
		reads.GET("/api-keys", keyPerm, handler.ListAPIKeys)
		reads.DELETE("/api-keys/:id", keyPerm, handler.RevokeAPIKey)
	} else {
		// AccountService not wired (e.g. existing unit tests that only build
		// Dependencies with KeyRepo/Deliveries/Webhooks) - keep the old
		// API-key-only behavior for these reads so nothing breaks.
		writes.GET("/deliveries", readPerm, handler.ListDeliveries)
		writes.GET("/deliveries/:id", readPerm, handler.GetDelivery)
		writes.GET("/webhooks", webhookPerm, handler.ListWebhooks)
		writes.GET("/webhooks/:id/deliveries", webhookPerm, handler.ListWebhookDeliveries)

		// Webhook log endpoints for API-key users
		writes.GET("/webhook-logs", webhookPerm, handler.ListWebhookLogs)
		writes.GET("/webhook-logs/:id", webhookPerm, handler.GetWebhookLog)

		// NEW: API key management endpoints for API-key users
		writes.POST("/api-keys", keyPerm, handler.CreateAPIKey)
		writes.GET("/api-keys", keyPerm, handler.ListAPIKeys)
		writes.DELETE("/api-keys/:id", keyPerm, handler.RevokeAPIKey)
	}
}