package orchestration

import (
	"context"
	"errors"
	"testing"
)

type branchTestAuthorizer struct{}

func (branchTestAuthorizer) Authorize(_ context.Context, _, _ string, branch *string) (bool, error) {
	return branch != nil && *branch == "branch-a", nil
}

func TestVendorOrchestrationScopes(t *testing.T) {
	h := &Handler{authorizer: branchTestAuthorizer{}}
	pickups := []Pickup{{BusinessID: "vendor-a", BranchID: "branch-a"}, {BusinessID: "vendor-b", BranchID: "branch-b"}}
	visible, err := h.vendorPickups(context.Background(), "staff-a", "vendor-a", PermissionOrchestrationRead, pickups, false)
	if err != nil || len(visible) != 1 || visible[0].BusinessID != "vendor-a" {
		t.Fatal("read leaked another vendor or rejected own branch")
	}
	if _, err = h.vendorPickups(context.Background(), "staff-a", "outsider", PermissionOrchestrationRead, pickups, false); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-vendor group must stay opaque")
	}
	if _, err = h.vendorPickups(context.Background(), "staff-a", "vendor-a", PermissionOrchestrationOverride, pickups, true); !errors.Is(err, ErrForbidden) {
		t.Fatal("shared group override was allowed")
	}
	pickups[1].BusinessID = "vendor-a"
	if _, err = h.vendorPickups(context.Background(), "staff-a", "vendor-a", PermissionOrchestrationOverride, pickups, true); !errors.Is(err, ErrForbidden) {
		t.Fatal("unauthorized branch override was allowed")
	}
	if _, err = h.vendorPickups(context.Background(), "staff-a", "vendor-a", PermissionOrchestrationOverride, pickups[:1], true); err != nil {
		t.Fatal(err)
	}
}

func TestOverrideCannotChangeExecutingOrTerminalWork(t *testing.T) {
	for _, state := range []State{StateCommitting, StateCommitted, StateInExecution, StateRecovering, StateCompleted, StateCancelled, StateFailed} {
		if overrideStateError(Orchestration{State: state}) == nil {
			t.Fatalf("override allowed in %s", state)
		}
	}
	if overrideStateError(Orchestration{State: StateOpsReview, CurrentMissionRef: "mission-a"}) == nil {
		t.Fatal("existing execution mission was ignored")
	}
	if overrideStateError(Orchestration{State: StateWarmMatch}) != nil {
		t.Fatal("precommit override refused")
	}
}
