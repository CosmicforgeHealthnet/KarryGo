package inventory

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSection14DGenericReadRoutes(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	root := router.Group(
		"/api/v1/vendor",
	)

	RegisterRoutes(
		root,
		&Handler{},
		Flags{
			Enabled: true,
		},
	)

	found := map[string]bool{}

	for _, route := range router.Routes() {
		found[route.Method+" "+route.Path] = true
	}

	required := []string{
		"GET /api/v1/vendor/inventory/branches/:branchId/balances",
		"GET /api/v1/vendor/inventory/branches/:branchId/movements",
		"GET /api/v1/vendor/inventory/items/:inventoryItemId/availability",
		"GET /api/v1/vendor/inventory/branches/:branchId/revision",
	}

	for _, route := range required {
		if !found[route] {
			t.Fatalf(
				"missing generic Inventory route %q",
				route,
			)
		}
	}
}
