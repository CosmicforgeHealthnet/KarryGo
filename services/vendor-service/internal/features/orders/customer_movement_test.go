package orders

import (
	"testing"

	sharedevents "cosmicforge/logistics/shared/go/events"
)

// Customer V4 §22.2 FOOD/SHOP purchase row: every ParentOrder state maps to a
// valid shared view; terminal purchase states are terminal in Activity.
func TestCustomerOrderMovementStateCoversEveryParentState(t *testing.T) {
	for _, state := range []ParentState{ParentStateCreated, ParentStateInProgress, ParentStateCompleted, ParentStatePartiallyCompleted,
		ParentStateCancelled, ParentStateRefundPending, ParentStateRefunded, ParentStatePartiallyRefunded} {
		movement, stage := CustomerOrderMovementState(state)
		if movement == "" {
			t.Fatalf("%s publishes nothing", state)
		}
		if stage != "" && !sharedevents.IsMovementStage(stage) {
			t.Fatalf("%s: stage %q outside the vocabulary", state, stage)
		}
		if TerminalParentState(state) != sharedevents.IsTerminalMovementState(movement) {
			t.Fatalf("%s terminality differs in Activity (%s)", state, movement)
		}
	}
	if movement, _ := CustomerOrderMovementState("SOMETHING_ELSE"); movement != "" {
		t.Fatal("an unknown state must publish nothing")
	}
}
