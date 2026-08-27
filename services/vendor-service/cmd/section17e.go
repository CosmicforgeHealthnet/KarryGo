package main

// Section 17E composition root.
//
// It wires order orchestration to the approved-quote authority and to the
// Section 5 permission evaluator, and to nothing else. Notably absent: any
// Wallet, payment-intent or mission-execution dependency. Section 17E creates
// orders and allocations; it moves no money and executes no delivery, and a
// dependency wired here would be the first step towards doing so.

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	vendorevents "cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	supportintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/support"
	"cosmicforge/logistics/services/vendor-service/internal/jobs"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/vendor-service/internal/platform/ratelimit"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func buildSection17EOrderService(
	cfg config.Config,
	db *pgxpool.Pool,
	evaluator *staffusecases.AuthorizationEvaluator,
	stock *inventory.StockService,
	quoteService *checkoutquote.Service,
	productionService *production.Service,
	abuseLimiter *ratelimit.Limiter,
	outboxes ...*vendorevents.DomainPublisher,
) *orders.Service {
	if !cfg.Flags.CommerceOrdersEnabled || db == nil || evaluator == nil {
		return nil
	}
	orderStore := orders.NewStore(db)
	if len(outboxes) > 0 {
		orderStore.SetEventPublisher(outboxes[0])
	}
	service, err := orders.NewService(orders.Options{
		Store:   orderStore,
		Quotes:  newSection17EQuoteAuthority(db),
		Scopes:  section17EScopeAdapter{evaluator: evaluator},
		Enabled: cfg.Flags.CommerceOrdersEnabled,
	})
	if err != nil {
		return nil
	}

	// Section 17F. The money authority is wired separately, so an order
	// service without Wallet credentials is fully working and simply exposes
	// no payment surface. PAYMENT_WALLET_COMMERCE_ENABLED already requires
	// VENDOR_WALLET_URL and VENDOR_WALLET_SECRET at config validation.
	// Section 17F fulfilment. Stock finalization and logistics execution are
	// wired together: activating one without the other would confirm stock for
	// a delivery nothing will ever run, or dispatch a vehicle for goods
	// nothing reserved.
	service.SetFulfilmentAuthorities(
		newSection17FInventoryAuthority(stock),
		newSection17FMissionAuthority(
			cfg.Flags.DispatchCommerceIntegrationEnabled,
			cfg.DispatchURL, cfg.DispatchSecret,
			cfg.Flags.HaulingCommerceIntegrationEnabled,
			cfg.HaulingURL, cfg.HaulingSecret,
			cfg.Fulfilment.QuoteTimeout,
		),
	)
	service.SetRestrictedGoodsAuthority(newRestrictedGoodsFulfilmentAuthority(db))
	if productionService != nil {
		service.SetProductionActivator(productionService)
	}

	walletAuthority := newSection17FPaymentAuthority(
		cfg.Flags.PaymentWalletCommerceEnabled,
		cfg.WalletURL,
		cfg.WalletSecret,
		cfg.Fulfilment.QuoteTimeout,
	)
	service.SetPaymentAuthority(orders.PaymentOptions{
		Authority: walletAuthority,
	})
	service.SetDeliveryFundingAuthority(walletAuthority)
	if cfg.SupportURL != "" && cfg.SupportSecret != "" {
		supportClient := supportintegration.NewClient(cfg.SupportURL, cfg.SupportSecret, cfg.Fulfilment.QuoteTimeout)
		service.SetRefundSupportAuthority(supportClient)
		service.SetCommerceCaseAuthority(supportClient)
	}
	if quoteService != nil {
		service.SetCommercialChangeAuthority(section17FChangeQuoteAuthority{quotes: quoteService})
	}
	service.SetSupportCaseRateLimiter(abuseLimiter, cfg.AbuseProtection.SupportMax, time.Duration(cfg.AbuseProtection.SupportWindowSeconds)*time.Second)
	return service
}

// section17FRecoveryJob is the durable payment-recovery sweep.
//
// It is what makes a crash survivable. Payment legs are committed before the
// money authority is called, so after any restart this pass asks the authority
// what actually happened to every unresolved leg, and unwinds through the
// authority anything that can never complete. Nothing here depends on an
// in-memory goroutine having finished, because that is exactly what a crash
// destroys.
func section17FRecoveryJob(
	cfg config.Config,
	service *orders.Service,
	coordinator *jobs.DurableCoordinator,
	owner string,
) (jobs.DurableJob, bool) {
	if service == nil || !service.PaymentEnabled() ||
		!cfg.Flags.CommerceOrdersEnabled ||
		!cfg.Flags.CommerceFulfilmentWorkersEnabled {
		return jobs.DurableJob{}, false
	}

	interval := cfg.Fulfilment.ExpiryInterval
	limits := orders.RecoveryLimits{
		// The grace keeps the sweep from racing a request that is still in
		// flight; the deadline stops an unanswerable leg holding stock forever.
		Grace:    2 * interval,
		Deadline: cfg.Checkout.ReservationTTL,
		Batch:    100,
	}

	return jobs.DurableJob{
		Job: jobs.Job{
			Name:     "commerce-payment-recovery",
			Interval: interval,
			Pass: func(runCtx context.Context) error {
				_, err := service.RecoverPayments(runCtx, limits)

				return err
			},
		},
		Coordinator: coordinator,
		Owner:       owner,
		LeaseFor:    2 * interval,
		MaxAttempts: 5,
		RetryBase:   interval,
	}, true
}

// registerSection17ECustomerOrderRoutes mounts the customer purchase surface
// under the commerce base, alongside the Section 17D quote it is created from.
func registerSection17ECustomerOrderRoutes(
	router *gin.Engine,
	cfg config.Config,
	service *orders.Service,
	idempotencyStore *idempotency.Store,
) {
	if router == nil || service == nil {
		return
	}
	orders.RegisterCustomerRoutes(
		router.Group("/api/v1/commerce"),
		service,
		sharedauth.NewTokenSigner(cfg.CustomerAccessTokenSecret),
		idempotencyStore,
		cfg.Flags.CommerceOrdersEnabled,
		// Section 17F. The payment surface appears only when a real money
		// authority is wired to the service.
		service.PaymentEnabled(),
	)
}
