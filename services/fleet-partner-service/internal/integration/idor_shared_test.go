package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
)

// Section 28 gap-1: shared helpers for the five new object-authorization
// (IDOR) domain suites -- assets/maintenance, warehouse/fulfilment,
// jobs/scheduled-tasks, imports, and reconciliation/dead-letters. Kept in
// one file so all five reuse exactly one branch-seeding recipe rather than
// five slightly different ones.

// seedBranch creates a real branch through the same repository production
// code path uses and returns its id.
func seedBranch(t *testing.T, pool *pgxpool.Pool, organizationID string, name string) string {
	t.Helper()
	branches := orgrepos.NewBranchRepository(pool)
	code := "BR-" + strings.ToUpper(strings.ReplaceAll(name, " ", "-"))
	branch := &orgmodels.Branch{
		Code: code, Name: name, Type: "HUB", Timezone: "Africa/Lagos",
	}
	if err := branches.Create(context.Background(), pool, organizationID, branch); err != nil {
		t.Fatalf("seed branch %q: %v", name, err)
	}
	return branch.ID
}
