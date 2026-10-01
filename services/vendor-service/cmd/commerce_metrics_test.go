package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/observability"
)

func TestCommerceFamilyIsBounded(t *testing.T) {
	for route, want := range map[string]string{
		"/api/v1/commerce/carts/:cartId/items":               "cart",
		"/api/v1/commerce/checkout-sessions":                 "checkout",
		"/api/v1/commerce/checkout/:sessionId/quotes":        "checkout",
		"/api/v1/commerce/quotes/:quoteId/approve":           "quote",
		"/api/v1/commerce/quotes/:quoteId/payment":           "payment",
		"/api/v1/commerce/orders/:orderId":                   "order",
		"/api/v1/commerce/substitutions/:proposalId/approve": "order",
		"/api/v1/commerce/food/restaurants":                  "discovery",
		"/api/v1/vendor/staff":                               "",
		"":                                                   "",
	} {
		if got := commerceFamily(route); got != want {
			t.Errorf("%q -> %q, want %q", route, got, want)
		}
	}
}

// Customer V4 §27.2: every durable commerce gauge runs against the migrated
// Vendor schema.
func TestCommerceGaugeQueriesRunAgainstMigratedSchema(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOT EXECUTED: VENDOR_SERVICE_TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	m := observability.NewMetrics()
	if err = observability.CollectGauges(context.Background(), observability.PoolQuerier(pool), m, commerceGaugeQueries); err != nil {
		t.Fatalf("gauge collection: %v", err)
	}
	if out := m.Render(); !strings.Contains(out, "karry_commerce_substitution_pending") || !strings.Contains(out, "karry_commerce_stock_conflicts_24h") {
		t.Fatalf("durable gauges missing:\n%s", out)
	}
}
