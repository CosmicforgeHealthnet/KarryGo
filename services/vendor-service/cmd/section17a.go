package main

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
	customerintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/customer"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

// registerSection17ACustomerCartRoutes composes the customer cart boundary.
//
// It deliberately does not compose checkout, Inventory reservation, Wallet,
// Dispatch, Hauling, Taxi, ParentOrder or VendorOrder. Those remain later
// Section 17 responsibilities.
func registerSection17ACustomerCartRoutes(
	router *gin.Engine,
	cfg config.Config,
	db *pgxpool.Pool,
	foodService *food.Service,
	shopService *shop.Service,
	idempotencyStore *idempotency.Store,
) {
	if router == nil ||
		db == nil ||
		foodService == nil ||
		shopService == nil {
		return
	}

	customerSigner := sharedauth.NewTokenSigner(
		cfg.CustomerAccessTokenSecret,
	)

	customerAddresses := customerintegration.NewClient(
		cfg.CustomerURL,
		[]byte(cfg.CustomerSecret),
		cfg.InternalServiceCallTimeout,
	)

	cartService := cart.NewService(
		cart.NewStore(db),
		newSection17AFoodAuthority(
			db,
			foodService,
		),
		newSection17AShopAuthority(
			db,
			shopService,
		),
		customerAddresses,
		events.NewDomainPublisher(
			events.NewOutboxRepository(db),
		),
	)

	customerCommerce := router.Group(
		"/api/v1/commerce",
	)

	cart.RegisterCustomerRoutes(
		customerCommerce,
		cartService,
		customerSigner,
		idempotencyStore,
	)
}
