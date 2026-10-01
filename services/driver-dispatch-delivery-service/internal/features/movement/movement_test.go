package movement

import (
	"testing"

	"cosmicforge/logistics/shared/go/events"
)

// Dispatch's derived wire status and the published movement state must agree:
// a customer reading the delivery in dispatch and the same movement in their
// history must never see two different stories.
func TestStateMirrorsTheDerivedWireStatus(t *testing.T) {
	cases := []struct {
		trip, booking, broadcast string
		cancelled                bool
		want                     string
	}{
		{trip: "assigned", want: events.MovementStateAssigned},
		{trip: "en_route_pickup", want: events.MovementStateAssigned},
		{trip: "arrived_pickup", want: events.MovementStateArrived},
		{trip: "in_progress", want: events.MovementStateInProgress},
		{trip: "proof_submitted", want: events.MovementStateDelivered},
		{trip: "completed", want: events.MovementStateCompleted},
		{trip: "failed", want: events.MovementStateCancelled},
		{booking: "unmatched", cancelled: true, want: events.MovementStateUnmatched},
		{booking: "cancelled", cancelled: true, want: events.MovementStateCancelled},
		{broadcast: "no_provider_found", want: events.MovementStateUnmatched},
		{broadcast: "accepted", want: events.MovementStateAssigned},
		{broadcast: "broadcasting", want: events.MovementStateSearching},
		{want: events.MovementStateSearching},
	}
	for _, c := range cases {
		if got := stateFor(c.trip, c.booking, c.broadcast, c.cancelled); got != c.want {
			t.Errorf("trip=%q booking=%q broadcast=%q -> %s, want %s", c.trip, c.booking, c.broadcast, got, c.want)
		}
	}
	// A terminal trip outranks a stale broadcast row, exactly as the read
	// model's own precedence does.
	if got := stateFor("completed", "cancelled", "no_provider_found", true); got != events.MovementStateCompleted {
		t.Errorf("trip precedence = %s", got)
	}
}
