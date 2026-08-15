package production

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

func TestProductionTenantBranchAndNestedIDORFailClosed(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	foreignPool := env.seedPool(t, businessB, branchB1, "CLOUD_KITCHEN", "FOREIGN_POOL", 20)
	if _, _, err := env.service.Reserve(context.Background(), actorFor(businessA, staffA), globalScope(), ReserveInput{
		BranchID: branchA1, Capability: "CLOUD_KITCHEN", WindowStart: env.now.AddDate(0, 0, 1), WindowEnd: env.now.AddDate(0, 0, 1).Add(2 * time.Hour),
		Allocations: []AllocationRequest{{CapacityPoolID: foreignPool, Quantity: 1}},
	}); errorCode(t, err) != CodePoolNotFound {
		t.Fatalf("cross-tenant pool reference did not fail closed: %v", err)
	}

	otherBranchPool := env.seedPool(t, businessA, branchA2, "CLOUD_KITCHEN", "OTHER_BRANCH_POOL", 20)
	branchOnly := Scope{BranchIDs: []string{branchA1}}
	if _, _, err := env.service.Reserve(context.Background(), actorFor(businessA, staffA), branchOnly, ReserveInput{
		BranchID: branchA2, Capability: "CLOUD_KITCHEN", WindowStart: env.now.AddDate(0, 0, 1), WindowEnd: env.now.AddDate(0, 0, 1).Add(2 * time.Hour),
		Allocations: []AllocationRequest{{CapacityPoolID: otherBranchPool, Quantity: 1}},
	}); err == nil || apperrors.From(err).Status != 403 {
		t.Fatalf("branch scope did not fail closed: %v", err)
	}

	localPool := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "LOCAL_POOL", 20)
	reservation, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(localPool, 1), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = env.service.GetReservation(context.Background(), actorFor(businessB, staffB), globalScope(), reservation.ID); errorCode(t, err) != CodeReservationNotFound {
		t.Fatalf("nested reservation IDOR did not return not-found: %v", err)
	}
}

func TestSections1011IAMRoleSeparation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	has := func(role, permission string) bool {
		var value bool
		if err := env.pool.QueryRow(context.Background(), `SELECT EXISTS(
			SELECT 1 FROM vendor_role_permissions rp
			JOIN vendor_permissions p ON p.id=rp.permission_id
			WHERE rp.role_id=$1 AND p.code=$2)`, role, permission).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}

	for _, permission := range []string{PermProductionRead, PermProductionOperate, "vendor.pastry.read", "vendor.pastry.production"} {
		if !has("role_kitchen_pastry", permission) {
			t.Errorf("kitchen/pastry role lacks %s", permission)
		}
	}
	for _, permission := range []string{PermProductionManage, "vendor.cloud_kitchen.manage", "vendor.pastry.manage", "vendor.pastry.quote.manage", "vendor.pastry.quote.approve"} {
		if has("role_kitchen_pastry", permission) {
			t.Errorf("kitchen/pastry role unexpectedly has %s", permission)
		}
	}
	if !has("role_owner", "vendor.pastry.quote.approve") {
		t.Error("owner lacks high-value quote approval")
	}
	if has("role_finance", PermProductionRead) || has("role_finance", PermProductionManage) || has("role_finance", PermProductionOperate) {
		t.Error("finance role unexpectedly has production authority")
	}
	if !has("role_analyst_auditor", PermProductionRead) || has("role_analyst_auditor", PermProductionManage) || has("role_analyst_auditor", PermProductionOperate) {
		t.Error("analyst/auditor production access is not read-only")
	}
}
