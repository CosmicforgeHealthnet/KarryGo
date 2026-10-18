package operations

import "testing"

func TestUnknownOutcomeRequiresAuthoritativeResolution(t *testing.T) {
	if CanTransition("UNKNOWN_OUTCOME", "EXECUTING", true) || CanTransition("UNKNOWN_OUTCOME", "CONFIRMED", false) || CanTransition("CONFIRMED", "EXECUTING", true) {
		t.Fatal("unsafe command repetition")
	}
	if !CanTransition("EXECUTING", "UNKNOWN_OUTCOME", false) || !CanTransition("UNKNOWN_OUTCOME", "CONFIRMED", true) {
		t.Fatal("cannot track authoritative outcome")
	}
}
