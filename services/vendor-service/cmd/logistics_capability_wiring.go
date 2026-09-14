package main

// Logistics capability wiring (VND-005 … VND-009, VND-031, VND-032).
//
// One place builds the capability clients, the registry that gates missions
// against declared capability, and the readiness surface an operator can read.
// It is driven entirely by the existing per-authority feature flags and
// credentials — there is no separate capability flag, because an authority
// Vendor may not send work to is an authority Vendor has no business probing.

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/logisticscapability"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// logisticsAuthorityFlags reports the deployment's per-authority switches.
//
// These are the flags that already exist. VND-032 asks for exactly this and
// warns against introducing duplicates, so no new names are added: the
// existing DISPATCH_COMMERCE_INTEGRATION_ENABLED, TAXI_GOODS_DELIVERY_ENABLED
// and HAULING_COMMERCE_INTEGRATION_ENABLED remain the single source of truth,
// and config validation already refuses a flag without its credentials.
func logisticsAuthorityFlags(cfg config.Config) map[logistics.Authority]bool {
	return map[logistics.Authority]bool{
		logistics.Dispatch: cfg.Flags.DispatchCommerceIntegrationEnabled,
		logistics.Taxi:     cfg.Flags.TaxiGoodsDeliveryEnabled,
		logistics.Hauling:  cfg.Flags.HaulingCommerceIntegrationEnabled,
	}
}

// newLogisticsCapabilityClients builds one capability client per authority
// that is both enabled and credentialled.
//
// A disabled authority gets no client at all, which is what makes "do not
// select an authority whose integration is disabled" structural rather than a
// runtime check somebody can forget.
func newLogisticsCapabilityClients(cfg config.Config, timeout time.Duration) logisticscapability.Set {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	httpClient := &http.Client{Timeout: timeout}
	clients := logisticscapability.Set{}

	type binding struct {
		authority logistics.Authority
		enabled   bool
		url       string
		secret    string
		apiBase   string
	}
	for _, item := range []binding{
		{logistics.Dispatch, cfg.Flags.DispatchCommerceIntegrationEnabled, cfg.DispatchURL, cfg.DispatchSecret, commercemission.DispatchAPIBase},
		{logistics.Taxi, cfg.Flags.TaxiGoodsDeliveryEnabled, cfg.TaxiURL, cfg.TaxiSecret, commercemission.TaxiAPIBase},
		{logistics.Hauling, cfg.Flags.HaulingCommerceIntegrationEnabled, cfg.HaulingURL, cfg.HaulingSecret, commercemission.HaulingAPIBase},
	} {
		if !item.enabled || item.url == "" || item.secret == "" {
			continue
		}
		clients[item.authority] = logisticscapability.New(logisticscapability.Options{
			BaseURL: item.url, APIBase: item.apiBase,
			Caller: commercemission.SignatureCaller, Secret: []byte(item.secret),
			Authority: item.authority, Timeout: timeout, HTTP: httpClient,
		})
	}
	return clients
}

// newLogisticsRegistry builds the capability registry used to gate missions.
func newLogisticsRegistry(cfg config.Config, timeout time.Duration) *logistics.Registry {
	return logistics.NewRegistryFromClientSet(
		newLogisticsCapabilityClients(cfg, timeout),
		logisticsAuthorityFlags(cfg),
		time.Now,
	)
}

// registerLogisticsReadinessRoutes exposes the adapter readiness surface
// (VND-031).
//
// It is an internal, HMAC-protected operator surface rather than a public one:
// which logistics integrations a deployment has, and which of them are
// answering, is operational detail no customer should be able to enumerate.
// It reports; it never changes anything.
func registerLogisticsReadinessRoutes(
	router gin.IRouter, registry *logistics.Registry,
	verifier *serviceauth.Verifier, allowedCallers map[string]bool,
) {
	if router == nil || registry == nil || verifier == nil {
		return
	}
	group := router.Group("/internal/vendor/v1/logistics")
	group.Use(serviceauth.Middleware(verifier), allowLogisticsReadinessCaller(allowedCallers))
	group.GET("/readiness", func(c *gin.Context) {
		states := registry.ReadinessAll(c)
		ready := true
		for _, state := range states {
			// An authority this deployment does not use is not a readiness
			// problem; only an enabled one that cannot be reached is.
			if state.Enabled && !state.Reachable {
				ready = false
			}
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
			"ready": ready, "authorities": states,
		}})
	})
}

// allowLogisticsReadinessCaller keeps the surface to the services that
// legitimately supervise Vendor.
func allowLogisticsReadinessCaller(allowed map[string]bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(allowed) == 0 {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		caller, _ := c.Get("service_name")
		name, _ := caller.(string)
		if !allowed[name] {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}
