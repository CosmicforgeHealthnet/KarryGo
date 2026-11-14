package integration_test

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// An organization-wide caller carries a nil PermittedBranchIDs slice, which
// pgx sends as NULL. The list must still return branch rows: before the
// COALESCE fix, cardinality(NULL)=0 was NULL and only branchless expenses came
// back. A branch-confined caller still sees only its branch plus branchless
// rows.
func TestOrganizationWideListIncludesBranchRows(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Null Scope Co", "RC-NULLSCOPE-1")
	lagos := seedBranch(t, pool, org.ID, "Null Scope Lagos")
	abuja := seedBranch(t, pool, org.ID, "Null Scope Abuja")
	for i, branch := range []any{lagos, abuja, nil} {
		if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_expenses(id,organization_id,branch_id,category,cost_centre,amount_minor,currency,description,submitted_by)
			VALUES($1,$2,$3,'FUEL','OPS',1000,'NGN','scope check','usr_test')`, "exp_nullscope_"+string(rune('a'+i)), org.ID, branch); err != nil {
			t.Fatal(err)
		}
	}
	svc := financeops.NewService(financeops.Options{Pool: pool, Flags: financeops.Flags{Expenses: true}})

	all, err := svc.ListExpenses(ctx, tenant.Context{OrganizationID: org.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("organization-wide caller saw %d expenses, want 3 (branch rows hidden by a NULL scope guard)", len(all))
	}
	confined, err := svc.ListExpenses(ctx, tenant.Context{OrganizationID: org.ID, PermittedBranchIDs: []string{lagos}})
	if err != nil {
		t.Fatal(err)
	}
	if len(confined) != 2 {
		t.Fatalf("Lagos-confined caller saw %d expenses, want 2 (Lagos + branchless)", len(confined))
	}
	for _, row := range confined {
		if row["branch_id"] == abuja {
			t.Fatal("confined caller saw another branch's expense")
		}
	}
}
