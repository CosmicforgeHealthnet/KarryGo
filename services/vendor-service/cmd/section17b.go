package main

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
	"cosmicforge/logistics/services/vendor-service/internal/jobs"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

// buildSection17BCheckoutService composes the Section 17B boundary.
//
// It returns nil when checkout is disabled or a dependency is missing, so a
// deployment that has not enabled Section 17B never registers a route and
// never starts a worker.
func buildSection17BCheckoutService(
	cfg config.Config,
	db *pgxpool.Pool,
	foodService *food.Service,
	shopService *shop.Service,
	stockService *inventory.StockService,
) *checkout.Service {
	if !cfg.Flags.CommerceCheckoutEnabled ||
		db == nil ||
		foodService == nil ||
		shopService == nil ||
		stockService == nil {
		return nil
	}

	service, err := checkout.NewService(
		checkout.Options{
			Store: checkout.NewStore(db),

			Food: newSection17BFoodAuthority(
				db,
				foodService,
			),

			Shop: newSection17BShopAuthority(
				db,
				shopService,
				stockService,
			),

			Stock: newSection17BStockAuthority(
				stockService,
			),

			ReservationTTL: cfg.Checkout.ReservationTTL,
			StrandedGrace:  cfg.Checkout.StrandedGrace,
		},
	)
	if err != nil {
		// Config.Validate already refuses an out-of-range TTL, so reaching
		// here means a dependency is genuinely absent. Registering nothing
		// is the fail-closed answer.
		return nil
	}

	return service
}

// registerSection17BCheckoutRoutes mounts the customer checkout surface.
//
// It deliberately composes no Wallet, Dispatch, Hauling, Taxi, ParentOrder or
// VendorOrder dependency: Section 17B validates and reserves, and every one of
// those belongs to a later section.
func registerSection17BCheckoutRoutes(
	router *gin.Engine,
	cfg config.Config,
	service *checkout.Service,
	idempotencyStore *idempotency.Store,
) {
	if router == nil || service == nil {
		return
	}

	customerSigner := sharedauth.NewTokenSigner(
		cfg.CustomerAccessTokenSecret,
	)

	checkout.RegisterCustomerRoutes(
		router.Group("/api/v1/commerce"),
		service,
		customerSigner,
		idempotencyStore,
		cfg.Flags.CommerceCheckoutEnabled,
	)
}

// section17BRecoveryJob is the sweep that keeps a crash or an abandoned
// checkout from costing a vendor its stock.
//
// It is a durable job for the same reason every Section 14 sweep is: several
// instances run this service, and releasing the same reservation from several
// of them at once is safe but pointless work.
func section17BRecoveryJob(
	cfg config.Config,
	service *checkout.Service,
	coordinator *jobs.DurableCoordinator,
	owner string,
) (jobs.DurableJob, bool) {
	if service == nil ||
		!cfg.Flags.CommerceCheckoutEnabled ||
		!cfg.Flags.CommerceCheckoutRecoveryEnabled {
		return jobs.DurableJob{}, false
	}

	return jobs.DurableJob{
		Job: jobs.Job{
			Name:     "commerce-checkout-recovery",
			Interval: cfg.Checkout.RecoveryInterval,
			Pass: func(runCtx context.Context) error {
				_, err := service.SweepDueSessions(
					runCtx,
					checkout.MaxSweepBatch,
				)

				return err
			},
		},
		Coordinator: coordinator,
		Owner:       owner,
		LeaseFor:    2 * cfg.Checkout.RecoveryInterval,
		MaxAttempts: 5,
		RetryBase:   5 * cfg.Checkout.RecoveryInterval / 5,
	}, true
}
