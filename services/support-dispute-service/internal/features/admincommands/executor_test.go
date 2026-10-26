package admincommands

import (
	"testing"

	"cosmicforge/logistics/shared/go/admincommand"
)

func TestEveryActionHasExactlyOneNativeGrant(t *testing.T) {
	spec := Spec()
	if spec.Owner != "support-dispute-service" || len(spec.Actions) != 9 || len(permissions) != len(spec.Actions) {
		t.Fatalf("dictionary drift: %d actions, %d grants", len(spec.Actions), len(permissions))
	}
	for action := range spec.Actions {
		if Permission(action) == "" {
			t.Fatalf("%s has no native grant", action)
		}
	}
	for _, action := range []string{"RESOLVE_DISPUTE", "RECORD_DISPUTE_REMEDY"} {
		if !spec.Actions[action].RequiresApproval {
			t.Fatalf("%s must require independent approval", action)
		}
	}
}

func TestTransitionAdjacencyHasNoTerminalExit(t *testing.T) {
	for _, terminalState := range []string{"resolved", "closed"} {
		if len(transitions[terminalState]) != 0 {
			t.Fatalf("%s has an exit", terminalState)
		}
	}
	if transitions["open"]["awaiting_evidence"] || !transitions["open"]["under_review"] || !transitions["under_review"]["awaiting_evidence"] {
		t.Fatal("adjacency drifted")
	}
}

func TestCommandRuntimeDefaultsOff(t *testing.T) {
	h, err := admincommand.Runtime(nil, Spec(), Executor{}, "SUPPORT_DISPUTE")
	if err != nil || h.Service.WriteEnabled || h.Service.ReadEnabled {
		t.Fatalf("command runtime must default off: %v", err)
	}
}
