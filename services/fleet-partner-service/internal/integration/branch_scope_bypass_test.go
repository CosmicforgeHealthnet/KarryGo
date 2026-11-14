package integration_test

import (
	"net/http"
	"testing"

	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
)

// TestBranchScopedRoleCannotBeWidenedByQueryParameter is the regression for
// the confirmed branch-scope escape (spec 7b, 9.9): authorization used to
// check a BRANCH-scoped binding against the ?branchId the caller supplied,
// while the object itself was checked only against the membership's
// permitted branches (empty = all). A member whose only role is Owner at
// branch A could read branch B by adding ?branchId=<A>.
func TestBranchScopedRoleCannotBeWidenedByQueryParameter(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Branch Scope Co", "RC-BSCOPE-1")
	router := assetsIDORRouter(t, pool)
	owner := seedApprovalMember(t, pool, org.ID, "rol_system_owner", "scope-owner")
	branchA := seedBranch(t, pool, org.ID, "Scope Branch A")
	branchB := seedBranch(t, pool, org.ID, "Scope Branch B")

	create := func(branch, code string) string {
		t.Helper()
		status, created := assetsIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/assets", org.ID, owner,
			map[string]any{"BranchID": branch, "AssetCode": code, "AssetType": "TRUCK"},
			map[string]string{"Idempotency-Key": "scope-" + code})
		if status != http.StatusCreated {
			t.Fatalf("create %s = %d %v", code, status, created)
		}
		return created["id"].(string)
	}
	assetA := create(branchA, "SCOPE-A")
	assetB := create(branchB, "SCOPE-B")

	// A second member whose ONLY grant is the owner role at branch A.
	branchOnly := seedMember(t, pool, org.ID, seedIdentity(t, pool, "branch-only-"+randomHex()[:6]+"@example.com", "+23481"+randomHex()[:8]), false, nil)
	bindRole(t, pool, org.ID, branchOnly.ID, staffmodels.SystemOwnerRoleID, staffmodels.ScopeBranch, branchA)

	for _, path := range []string{
		"/fleet-partner/v1/assets/" + assetB,
		"/fleet-partner/v1/assets/" + assetB + "?branchId=" + branchA,
	} {
		if status, body := assetsIDORCall(t, router, http.MethodGet, path, org.ID, branchOnly.ID, nil, nil); status != http.StatusNotFound && status != http.StatusForbidden {
			t.Fatalf("branch-A-only member reading branch B via %s = %d %v, want 404/403", path, status, body)
		}
	}
	// Its own branch still works, with or without the parameter.
	for _, path := range []string{"/fleet-partner/v1/assets/" + assetA, "/fleet-partner/v1/assets/" + assetA + "?branchId=" + branchA} {
		if status, body := assetsIDORCall(t, router, http.MethodGet, path, org.ID, branchOnly.ID, nil, nil); status != http.StatusOK {
			t.Fatalf("branch-A-only member reading branch A via %s = %d %v, want 200", path, status, body)
		}
	}
	// It cannot create in branch B either.
	if status, body := assetsIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/assets?branchId="+branchA, org.ID, branchOnly.ID,
		map[string]any{"BranchID": branchB, "AssetCode": "SCOPE-B2", "AssetType": "TRUCK"},
		map[string]string{"Idempotency-Key": "scope-b2"}); status == http.StatusCreated {
		t.Fatalf("branch-A-only member created an asset in branch B: %v", body)
	}
	// Lists contain only branch A.
	status, list := assetsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/assets?branchId="+branchA, org.ID, branchOnly.ID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("list = %d %v", status, list)
	}
	if items, ok := list["data"].([]any); ok {
		for _, item := range items {
			if item.(map[string]any)["branchId"] == branchB {
				t.Fatal("branch B asset listed for a branch-A-only member")
			}
		}
	}
	// Without any parameter the list is still limited to branch A, in SQL,
	// so the page holds branch A's asset rather than being cut short.
	status, list = assetsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/assets", org.ID, branchOnly.ID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("unfiltered list = %d %v", status, list)
	}
	sawA := false
	if items, ok := list["data"].([]any); ok {
		for _, item := range items {
			switch item.(map[string]any)["branchId"] {
			case branchB:
				t.Fatal("branch B asset in the unfiltered list of a branch-A-only member")
			case branchA:
				sawA = true
			}
		}
	}
	if !sawA {
		t.Fatalf("branch-A-only member's unfiltered list lacks their own asset: %v", list)
	}
	// The organization-wide owner still sees both.
	if status, _ := assetsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/assets/"+assetB, org.ID, owner, nil, nil); status != http.StatusOK {
		t.Fatalf("org-wide owner reading branch B = %d", status)
	}
}
