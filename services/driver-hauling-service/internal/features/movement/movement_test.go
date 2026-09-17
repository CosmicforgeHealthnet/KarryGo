package movement

import (
	"testing"

	"cosmicforge/logistics/shared/go/events"
)

func TestStateMapsEveryHaulageStatus(t *testing.T) {
	cases := map[string]string{
		"pending_match":       events.MovementStateSearching,
		"awaiting_acceptance": events.MovementStateSearching,
		"accepted":            events.MovementStateAssigned,
		"en_route_pickup":     events.MovementStateAssigned,
		"arrived_at_pickup":   events.MovementStateArrived,
		"picked_up":           events.MovementStateInProgress,
		"en_route_delivery":   events.MovementStateInProgress,
		"delivered":           events.MovementStateDelivered,
		"completed":           events.MovementStateCompleted,
		"cancelled":           events.MovementStateCancelled,
		"unmatched":           events.MovementStateUnmatched,
	}
	for status, want := range cases {
		if got := stateFor(status); got != want {
			t.Errorf("%s -> %s, want %s", status, got, want)
		}
	}
	// An unknown status publishes nothing rather than guessing a state the
	// customer would then see in their history.
	if got := stateFor("some_future_status"); got != "" {
		t.Errorf("unknown status -> %q, want no movement state", got)
	}
}
