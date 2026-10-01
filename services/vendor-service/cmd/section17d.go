package main

// Section 17D composition root. The service consumes immutable checkout and
// fulfilment evidence and persists a customer-approved commercial quote. It
// deliberately composes no Wallet, payment, ParentOrder or VendorOrder port.

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/vendor-service/internal/platform/ratelimit"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func buildSection17DQuoteService(cfg config.Config, db *pgxpool.Pool, promotionAuthority checkoutquote.PromotionAuthority) *checkoutquote.Service {
	if !cfg.Flags.CommerceQuoteLockEnabled || db == nil {
		return nil
	}
	service, err := checkoutquote.NewService(checkoutquote.Options{
		Store:      checkoutquote.NewStore(db, events.NewDomainPublisher(events.NewOutboxRepository(db))),
		Promotions: promotionAuthority,
		QuoteTTL:   cfg.Quote.TTL,
	})
	if err != nil {
		return nil
	}
	return service
}

func registerSection17DQuoteRoutes(router *gin.Engine, cfg config.Config, service *checkoutquote.Service, idempotencyStore *idempotency.Store, limiter *ratelimit.Limiter) {
	if router == nil || service == nil {
		return
	}
	checkoutquote.RegisterCustomerRoutes(
		router.Group("/api/v1/commerce"),
		service,
		sharedauth.NewTokenSigner(cfg.CustomerAccessTokenSecret),
		idempotencyStore,
		cfg.Flags.CommerceQuoteLockEnabled,
		checkoutquote.RequestLimitPolicy{Limiter: limiter, Max: cfg.AbuseProtection.QuoteMax, Window: time.Duration(cfg.AbuseProtection.QuoteWindowSeconds) * time.Second},
	)
}
