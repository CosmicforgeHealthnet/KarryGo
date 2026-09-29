package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type customerDiscoveryLimiter interface {
	Allow(context.Context, string, string, int, time.Duration) error
}

// Every public read shares a bearer-derived budget, including catalogue and
// suggestions. Missing Redis fails closed; staff identity cannot spend it.
func customerDiscoveryGuards(cfg config.Config, domain cart.Domain, limiter customerDiscoveryLimiter) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		sharedauth.BearerMiddleware(sharedauth.NewTokenSigner(cfg.CustomerAccessTokenSecret), "customer", "customer"),
		func(c *gin.Context) {
			if !customerDomainGate(cfg)(domain) {
				httpx.RespondError(c, apperrors.NotFound("Commerce discovery is unavailable.", nil))
				c.Abort()
				return
			}
			claims, ok := sharedauth.ClaimsFromContext(c)
			if !ok || claims.Subject == "" {
				c.AbortWithStatus(401)
				return
			}
			if limiter == nil {
				httpx.RespondError(c, apperrors.Unavailable("Discovery is temporarily unavailable.", nil))
				c.Abort()
				return
			}
			if err := limiter.Allow(c, "customer_commerce_discovery", claims.Subject, 120, time.Minute); err != nil {
				httpx.RespondError(c, err)
				c.Abort()
				return
			}
			c.Next()
		},
	}
}

func registerCustomerDiscoveryRoutes(router *gin.Engine, cfg config.Config, foodService *food.Service, shopService *shop.Service, search section13, limiter customerDiscoveryLimiter) {
	root := router.Group("/api/v1/commerce")
	if search.searchHandler != nil && foodService != nil {
		search.searchHandler.SetPublicTrustReader(foodService.CustomerTrust)
	}
	foodGuards := customerDiscoveryGuards(cfg, cart.DomainFood, limiter)
	shopGuards := customerDiscoveryGuards(cfg, cart.DomainShop, limiter)
	food.RegisterCustomerDiscoveryRoutes(root, food.NewInternalHandler(foodService), foodGuards...)
	shop.RegisterCustomerDiscoveryRoutes(root, shop.NewInventoryInternalHandler(shopService, nil), shopGuards...)
	searchprojection.RegisterCustomerSearchRoutes(root, search.searchHandler, search.searchFlags, foodGuards, shopGuards)
}
