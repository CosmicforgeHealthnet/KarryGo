package movement

import (
	"testing"

	"cosmicforge/logistics/shared/go/events"
)

// Customer V4 §22.2 TRUCK: every minimum lifecycle step is a distinct view.
func TestTruckLifecycleCoverage(t *testing.T) {
	seen := map[string]bool{}
	for status, want := range map[string]string{
		"pending_match": events.MovementStageSearching, "accepted": events.MovementStageProviderAssigned,
		"en_route_pickup": events.MovementStageEnRoutePickup, "arrived_at_pickup": events.MovementStageArrivedPickup,
		"picked_up": events.MovementStageCustodyStarted, "en_route_delivery": events.MovementStageInTransit,
		"delivered": events.MovementStageDelivered, "completed": events.MovementStageCompleted,
		"cancelled": events.MovementStageCancelled, "unmatched": events.MovementStageUnmatched,
	} {
		state, stage := stateFor(status), stageFor(status)
		if stage != want || state == "" {
			t.Errorf("%s: state=%s stage=%s, want stage %s", status, state, stage, want)
		}
		if seen[state+"/"+stage] {
			t.Errorf("%s collapses into another step", status)
		}
		seen[state+"/"+stage] = true
	}
	if stageFor("teleporting") != "" || stateFor("teleporting") != "" {
		t.Fatal("an unknown status must publish nothing")
	}
}
