package movement

import (
	"testing"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/events"
)

// Customer V4 §22.2 RIDE: every minimum lifecycle step is a distinct view.
func TestRideLifecycleCoverage(t *testing.T) {
	seen := map[string]bookingmodels.Status{}
	for status, want := range map[bookingmodels.Status]string{
		bookingmodels.StatusPendingMatch: events.MovementStageSearching, bookingmodels.StatusAccepted: events.MovementStageProviderAssigned,
		bookingmodels.StatusEnRoutePickup: events.MovementStageEnRoutePickup, bookingmodels.StatusArrivedAtPickup: events.MovementStageArrivedPickup,
		bookingmodels.StatusInProgress: events.MovementStageInTransit, bookingmodels.StatusCompleted: events.MovementStageCompleted,
		bookingmodels.StatusCancelled: events.MovementStageCancelled,
	} {
		state, ok := BookingState(status)
		stage := BookingStage(status)
		if !ok || stage != want {
			t.Errorf("%s: state=%s stage=%s, want stage %s", status, state, stage, want)
		}
		if prior, dup := seen[state+"/"+stage]; dup {
			t.Errorf("%s collapses into %s", status, prior)
		}
		seen[state+"/"+stage] = status
	}
}

// Taxi Goods carries a SEND-style lifecycle including custody and drop-off.
func TestTaxiGoodsLifecycleCoverage(t *testing.T) {
	seen := map[string]string{}
	for _, status := range []string{"MATCHING", "ASSIGNED", "EN_ROUTE_PICKUP", "AT_PICKUP", "PICKED_UP", "IN_TRANSIT", "AT_DROPOFF", "DELIVERED", "COMPLETED", "CANCELLED"} {
		state, ok := GoodsState(status)
		stage := GoodsStage(status)
		if !ok || !events.IsMovementStage(stage) {
			t.Errorf("%s: state=%s stage=%s", status, state, stage)
		}
		if prior, dup := seen[state+"/"+stage]; dup {
			t.Errorf("%s collapses into %s", status, prior)
		}
		seen[state+"/"+stage] = status
	}
}
