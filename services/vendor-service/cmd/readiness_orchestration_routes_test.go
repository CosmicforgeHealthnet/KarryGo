package main

// Route-shape certification for readiness and just-in-time orchestration.
//
// This exists because both surfaces mount into trees that already have
// neighbours, and gin settles route shapes at REGISTRATION time. A registration
// that gin refuses outright surfaces as a service that will not start, and a
// registration it accepts but resolves differently than intended surfaces as a
// request reaching the wrong handler -- neither of which a unit test of a
// handler would catch.
//
// So the test registers the real neighbours alongside the real new routes and
// asserts the engine assembles with the paths actually intended. It pins the
// paths themselves, because both live inside a neighbour's tree by design and
// moving one back out is a silent change to what else could match it.

import (
	"testing"

	"github.com/gin-gonic/gin"

	readiness "cosmicforge/logistics/services/vendor-service/internal/features/fulfilment_readiness"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/services/vendor-service/internal/features/orchestration"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// registerNeighbours mounts the already-shipped vendor surfaces whose trees the
// two new ones join.
func registerNeighbours(t *testing.T, group *gin.RouterGroup) {
	t.Helper()
	fulfilmentgroup.RegisterRoutes(group,
		fulfilmentgroup.NewHandler(fulfilmentgroup.NewService(fulfilmentgroup.Options{})), true)
	orders.RegisterVendorRoutes(group, &orders.Service{}, true)
}

func TestReadinessAndOrchestrationRoutesCoexistWithTheirNeighbours(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/v1/vendor")

	registerNeighbours(t, group)

	// Enabled, so the routes are actually mounted. A disabled feature mounts
	// nothing and would make this test pass without proving anything.
	readiness.RegisterRoutes(group, readiness.NewHandler(readiness.NewService(readiness.Options{}), nil), true)
	orchestration.RegisterRoutes(group, orchestration.NewHandler(nil,
		orchestration.NewRepository(nil), nil), true)

	paths := map[string]bool{}
	for _, route := range engine.Routes() {
		paths[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"GET /api/v1/vendor/commerce/orders/:vendorOrderId/readiness",
		"POST /api/v1/vendor/commerce/orders/:vendorOrderId/readiness/ready",
		"GET /api/v1/vendor/fulfilment/groups/:groupId/orchestration",
		"POST /api/v1/vendor/fulfilment/groups/:groupId/orchestration/authority-override",
	} {
		if !paths[want] {
			t.Fatalf("expected route %q to be mounted", want)
		}
	}
}

func TestOrchestrationInternalReadCoexistsWithTheVendorSurface(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	handler := orchestration.NewHandler(nil, orchestration.NewRepository(nil), nil)
	orchestration.RegisterRoutes(engine.Group("/api/v1/vendor"), handler, true)
	orchestration.RegisterInternalRoutes(engine.Group("/internal/vendor/v1"), handler, true)

	found := false
	for _, route := range engine.Routes() {
		if route.Method == "GET" &&
			route.Path == "/internal/vendor/v1/fulfilment/groups/:groupId/orchestration" {
			found = true
		}
	}
	if !found {
		t.Fatal("the internal orchestration read is not mounted at the engine root")
	}
}

// A disabled feature must mount nothing at all. A route that exists and
// refuses still confirms the capability is there and still has to be reasoned
// about by anyone reading the route table.
func TestDisabledFeaturesMountNoRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/v1/vendor")

	readiness.RegisterRoutes(group, readiness.NewHandler(readiness.NewService(readiness.Options{}), nil), false)
	orchestration.RegisterRoutes(group, orchestration.NewHandler(nil,
		orchestration.NewRepository(nil), nil), false)

	if routes := engine.Routes(); len(routes) != 0 {
		t.Fatalf("expected no routes when both features are off, got %d", len(routes))
	}
}
