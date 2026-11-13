package integration_test

import (
	"context"
	"testing"

	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
)

// TestNewCompanyHasActiveHeadOffice: activation requires an active default
// branch. A newly created company gets one, so the branch gate passes.
func TestNewCompanyHasActiveHeadOffice(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	ctx := context.Background()
	owner := onboardingLogin(t, router, h, "branch-owner@example.com", "+2348012345641")
	company := map[string]any{"legalName": "Branch Logistics", "registrationNumber": "RC-DEFAULT-BRANCH", "organizationType": "PRIVATE_LIMITED", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Branch Owner"}
	code, body := onboardCall(t, router, owner, "POST", "/organizations", company, 0)
	if code != 201 {
		t.Fatal(body)
	}
	orgID := documentData(t, body)["organizationId"].(string)

	var count int
	var name, status, timezone string
	if err := h.pool.QueryRow(ctx, `SELECT count(*) OVER (), name, status, timezone FROM fleet_partner_branches WHERE organization_id=$1 AND code='HQ'`, orgID).Scan(&count, &name, &status, &timezone); err != nil {
		t.Fatal(err)
	}
	if count != 1 || name != "Head Office" || status != "ACTIVE" || timezone != "Africa/Lagos" {
		t.Fatalf("head office: %d %s %s %s", count, name, status, timezone)
	}

	// Re-submitting the same company is idempotent: still one branch.
	if code, body := onboardCall(t, router, owner, "POST", "/organizations", company, 0); code != 201 && code != 200 {
		t.Fatalf("repeat create %d %v", code, body)
	}
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_branches WHERE organization_id=$1`, orgID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("branches after repeat: %d %v", count, err)
	}

	svc := orgusecases.NewService(orgusecases.Options{Pool: h.pool, Organizations: orgrepos.NewOrganizationRepository(h.pool), Onboarding: orgrepos.NewOnboardingRepository(h.pool), Branches: orgrepos.NewBranchRepository(h.pool), Settings: orgrepos.NewSettingsRepository(h.pool)})
	assessment, err := svc.EvaluateActivation(ctx, orgID)
	if err != nil {
		t.Fatal(err)
	}
	for _, gate := range assessment.Gates {
		if (gate.Name == "active_default_branch" || gate.Name == "settings_valid") && !gate.Passed {
			t.Fatalf("gate %s not passed", gate.Name)
		}
	}
}
