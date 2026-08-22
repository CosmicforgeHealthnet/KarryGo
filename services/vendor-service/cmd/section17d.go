package main

// Section 17D composition root. The service consumes immutable checkout and
// fulfilment evidence and persists a customer-approved commercial quote. It
// deliberately composes no Wallet, payment, ParentOrder or VendorOrder port.

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func buildSection17DQuoteService(cfg config.Config, db *pgxpool.Pool) *checkoutquote.Service {
	if !cfg.Flags.CommerceQuoteLockEnabled || db == nil {
		return nil
	}
	service, err := checkoutquote.NewService(checkoutquote.Options{
		Store: checkoutquote.NewStore(db),
		// No promotion authority is production-ready yet. An empty authority is
		// explicit and fail-closed; authoritative delivery subsidies still come
		// from the selected Section 17C plan and remain fully attributed.
		Promotions: checkoutquote.NoPromotions{},
		QuoteTTL:   cfg.Quote.TTL,
	})
	if err != nil {
		return nil
	}
	return service
}

func registerSection17DQuoteRoutes(router *gin.Engine, cfg config.Config, service *checkoutquote.Service, idempotencyStore *idempotency.Store) {
	if router == nil || service == nil {
		return
	}
	checkoutquote.RegisterCustomerRoutes(
		router.Group("/api/v1/commerce"),
		service,
		sharedauth.NewTokenSigner(cfg.CustomerAccessTokenSecret),
		idempotencyStore,
		cfg.Flags.CommerceQuoteLockEnabled,
	)
}
