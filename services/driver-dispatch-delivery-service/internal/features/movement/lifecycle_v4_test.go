package movement

import (
	"testing"

	"cosmicforge/logistics/shared/go/events"
)

// Customer V4 §22.2 SEND: every minimum lifecycle step is a distinct
// published view, derived only from authority evidence.
func TestSendLifecycleCoverage(t *testing.T) {
	type step struct {
		name, trip, booking, broadcast, returnState string
		deliveryChallenge                           bool
		wantState, wantStage                        string
	}
	steps := []step{
		{name: "created/searching", booking: "searching", broadcast: "broadcasting", wantState: events.MovementStateSearching, wantStage: events.MovementStageSearching},
		{name: "provider_assigned", trip: "assigned", wantState: events.MovementStateAssigned, wantStage: events.MovementStageProviderAssigned},
		{name: "en_route_pickup", trip: "en_route_pickup", wantState: events.MovementStateAssigned, wantStage: events.MovementStageEnRoutePickup},
		{name: "pickup_arrived", trip: "arrived_pickup", wantState: events.MovementStateArrived, wantStage: events.MovementStageArrivedPickup},
		{name: "custody_started/in_transit", trip: "in_progress", wantState: events.MovementStateInProgress, wantStage: events.MovementStageInTransit},
		{name: "dropoff_arrived", trip: "in_progress", deliveryChallenge: true, wantState: events.MovementStateInProgress, wantStage: events.MovementStageArrivedDropoff},
		{name: "delivered", trip: "proof_submitted", wantState: events.MovementStateDelivered, wantStage: events.MovementStageDelivered},
		{name: "completed", trip: "completed", wantState: events.MovementStateCompleted, wantStage: events.MovementStageCompleted},
		{name: "cancelled", trip: "cancelled", wantState: events.MovementStateCancelled, wantStage: events.MovementStageCancelled},
		{name: "return", trip: "failed", returnState: "RETURNING", wantState: events.MovementStateCancelled, wantStage: events.MovementStageReturning},
		{name: "returned", trip: "failed", returnState: "RETURNED", wantState: events.MovementStateCancelled, wantStage: events.MovementStageReturned},
	}
	seen := map[string]string{}
	for _, s := range steps {
		state := stateFor(s.trip, s.booking, s.broadcast, false)
		stage := stageFor(state, s.trip, s.deliveryChallenge, s.returnState)
		if state != s.wantState || stage != s.wantStage {
			t.Errorf("%s: state=%s stage=%s, want %s/%s", s.name, state, stage, s.wantState, s.wantStage)
		}
		if !events.IsMovementStage(stage) {
			t.Errorf("%s: stage %q outside the shared vocabulary", s.name, stage)
		}
		if prior, dup := seen[state+"/"+stage]; dup && prior != "custody_started/in_transit" {
			t.Errorf("%s collapses into %s", s.name, prior)
		}
		seen[state+"/"+stage] = s.name
	}
}
