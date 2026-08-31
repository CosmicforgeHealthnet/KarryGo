package inventory

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSection14FOperationalRoutes(
	t *testing.T,
) {
	gin.SetMode(
		gin.TestMode,
	)

	router :=
		gin.New()

	root :=
		router.Group(
			"/api/v1/vendor",
		)

	RegisterOperationalRoutes(
		root,
		&Handler{},
		Flags{
			Enabled: true,
		},
	)

	found :=
		map[string]bool{}

	for _, route := range router.Routes() {

		found[route.Method+" "+route.Path] =
			true
	}

	required :=
		[]string{
			"POST /api/v1/vendor/inventory/stocktakes",
			"PUT /api/v1/vendor/inventory/stocktakes/:stocktakeId/items/:inventoryItemId/count",
			"POST /api/v1/vendor/inventory/stocktakes/:stocktakeId/post",
			"POST /api/v1/vendor/inventory/transfers",
			"POST /api/v1/vendor/inventory/transfers/:transferId/dispatch",
			"POST /api/v1/vendor/inventory/transfers/:transferId/receive",
			"POST /api/v1/vendor/inventory/quarantines",
			"POST /api/v1/vendor/inventory/quarantines/:quarantineId/release",
			"POST /api/v1/vendor/inventory/quarantines/:quarantineId/dispose",
			"PUT /api/v1/vendor/inventory/items/:inventoryItemId/reorder-rule",
			"GET /api/v1/vendor/inventory/branches/:branchId/alerts",
		}

	for _, route := range required {

		if !found[route] {
			t.Fatalf(
				"missing 14F route %q",
				route,
			)
		}
	}
}
