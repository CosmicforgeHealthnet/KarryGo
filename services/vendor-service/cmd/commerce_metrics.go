package main

// Customer V4 §27.2 commerce metrics: request latency/outcome per bounded
// customer-contract family, plus durable, database-derived gauges collected at
// scrape time. Labels are closed vocabularies; no order, customer, vendor or
// operation identifiers ever become a series.

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/observability"
	"cosmicforge/logistics/shared/go/serviceauth"
)

var commerceMetrics = observability.NewMetrics()

// commerceFamily maps a matched route template to its contract family.
func commerceFamily(route string) string {
	switch {
	case !strings.HasPrefix(route, "/api/v1/commerce/"):
		return ""
	case strings.HasSuffix(route, "/payment"):
		return "payment"
	case strings.HasPrefix(route, "/api/v1/commerce/carts"):
		return "cart"
	case strings.HasPrefix(route, "/api/v1/commerce/checkout"):
		return "checkout"
	case strings.Contains(route, "/quotes"):
		return "quote"
	case strings.HasPrefix(route, "/api/v1/commerce/orders"), strings.HasPrefix(route, "/api/v1/commerce/vendor-orders"),
		strings.HasPrefix(route, "/api/v1/commerce/substitution"), strings.HasPrefix(route, "/api/v1/commerce/section21"):
		return "order"
	case strings.HasPrefix(route, "/api/v1/commerce/food"), strings.HasPrefix(route, "/api/v1/commerce/shop"),
		strings.HasPrefix(route, "/api/v1/commerce/search"), strings.HasPrefix(route, "/api/v1/commerce/discovery"),
		strings.HasPrefix(route, "/api/v1/commerce/catalogue"):
		return "discovery"
	}
	return "other"
}

// commerceRequestMetrics records latency and the bounded V4 outcome.
func commerceRequestMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		family := commerceFamily(c.FullPath())
		if family == "" {
			return
		}
		commerceMetrics.IncCounter("karry_commerce_requests_total", map[string]string{
			"family": family, "outcome": string(observability.OutcomeForStatus(c.Writer.Status())),
		}, 1)
		commerceMetrics.ObserveDuration("karry_commerce_request", map[string]string{"family": family}, time.Since(started))
	}
}

var commerceGaugeQueries = []observability.GaugeQuery{
	{Name: "karry_commerce_stock_conflicts_24h",
		SQL: `SELECT count(*)::float8 FROM commerce_checkout_groups WHERE reservation_failure_code <> '' AND updated_at > now()-interval '24 hours'`},
	{Name: "karry_commerce_substitution_pending",
		SQL: `SELECT count(*)::float8 FROM commerce_substitution_proposals WHERE status='PENDING'`},
	{Name: "karry_commerce_substitution_oldest_pending_age_seconds",
		SQL: `SELECT COALESCE(EXTRACT(EPOCH FROM now()-min(created_at)),0)::float8 FROM commerce_substitution_proposals WHERE status='PENDING'`},
	{Name: "karry_commerce_mission_unresolved", Labels: []string{"service"},
		SQL: `SELECT service, count(*)::float8 FROM commerce_order_missions WHERE status='PENDING' AND updated_at < now()-interval '5 minutes' GROUP BY service`},
	{Name: "karry_commerce_payment_legs_unresolved",
		SQL: `SELECT count(*)::float8 FROM commerce_payment_legs WHERE status='PENDING' AND updated_at < now()-interval '5 minutes'`},
	{Name: "karry_commerce_refunds_open", Labels: []string{"wallet_status"},
		SQL: `SELECT wallet_status, count(*)::float8 FROM commerce_refund_policy_decisions WHERE wallet_status IN ('PENDING','REQUESTED','UNKNOWN','PENDING_LIABILITY','SUPPORT_REQUIRED') GROUP BY wallet_status`},
	{Name: "karry_commerce_refund_oldest_open_age_seconds",
		SQL: `SELECT COALESCE(EXTRACT(EPOCH FROM now()-min(created_at)),0)::float8 FROM commerce_refund_policy_decisions WHERE wallet_status IN ('PENDING','REQUESTED','UNKNOWN')`},
	{Name: "karry_vendor_outbox_backlog", Labels: []string{"status"},
		SQL: `SELECT status, count(*)::float8 FROM vendor_outbox WHERE status <> 'PUBLISHED' GROUP BY status`},
	{Name: "karry_vendor_outbox_oldest_pending_age_seconds",
		SQL: `SELECT COALESCE(EXTRACT(EPOCH FROM now()-min(created_at)),0)::float8 FROM vendor_outbox WHERE status='PENDING'`},
}

// registerCommerceMetricsRoute mounts the private scrape beside the other
// operator surfaces: root-mounted, HMAC-verified, supervisor callers only.
func registerCommerceMetricsRoute(router gin.IRouter, db *pgxpool.Pool, verifier *serviceauth.Verifier, allowed map[string]bool) {
	if router == nil || verifier == nil {
		return
	}
	router.GET("/internal/vendor/v1/metrics", serviceauth.Middleware(verifier), allowLogisticsReadinessCaller(allowed), func(c *gin.Context) {
		_ = observability.CollectGauges(c.Request.Context(), observability.PoolQuerier(db), commerceMetrics, commerceGaugeQueries)
		c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(commerceMetrics.Render()))
	})
}
