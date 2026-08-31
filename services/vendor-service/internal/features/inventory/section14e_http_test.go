package inventory

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSection14EReservationRoutes(
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

	handler :=
		&Handler{}

	RegisterReservationRoutes(
		root,
		handler,
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
			"POST /api/v1/vendor/inventory/reservations",
			"GET /api/v1/vendor/inventory/reservations/:reservationId",
			"POST /api/v1/vendor/inventory/reservations/:reservationId/confirm",
			"POST /api/v1/vendor/inventory/reservations/:reservationId/commit",
			"POST /api/v1/vendor/inventory/reservations/:reservationId/release",
			"POST /api/v1/vendor/inventory/reservations/:reservationId/cancel",
			"GET /api/v1/vendor/inventory/items/:inventoryItemId/sellability",
		}

	for _, route := range required {

		if !found[route] {
			t.Fatalf(
				"missing 14E route %q",
				route,
			)
		}
	}
}
