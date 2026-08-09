package models

import "testing"

func TestManifestTransitions(t *testing.T) {
	for _, e := range [][2]string{{StatusDraft, StatusOpen}, {StatusOpen, StatusClosed}, {StatusClosed, StatusDispatched}, {StatusDispatched, StatusCompleted}} {
		if !CanTransition(e[0], e[1]) {
			t.Fatalf("missing %v", e)
		}
	}
	if CanTransition(StatusOpen, StatusCompleted) || Editable(StatusClosed) {
		t.Fatal("invalid manifest behavior")
	}
}
