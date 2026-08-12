package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// TestGetMembershipSummaryForProvider_IndependentProviderHasNoMembership is
// Section 30 WP19's live-DB proof: a provider identity with NO membership
// row anywhere reports ProviderMode == INDEPENDENT, with nothing else
// populated, against a real Postgres lookup (not a mock).
func TestGetMembershipSummaryForProvider_IndependentProviderHasNoMembership(t *testing.T) {
	pool := testPool(t)
	metrics := observability.NewMetrics()
	orgSvc := orgusecases.NewService(orgusecases.Options{
		Pool: pool, Organizations: orgrepos.NewOrganizationRepository(pool), Branches: orgrepos.NewBranchRepository(pool),
	})
	svc := providerusecases.NewService(providerusecases.Options{
		Pool: pool, Providers: providerrepos.NewRepository(pool), Organizations: orgSvc, Metrics: metrics,
	})

	summary, err := svc.GetMembershipSummaryForProvider(context.Background(), "driver-hauling-service", "drv_independent_"+randomHex())
	if err != nil {
		t.Fatalf("GetMembershipSummaryForProvider: %v", err)
	}
	if summary.ProviderMode != providermodels.ProviderModeIndependent {
		t.Errorf("ProviderMode = %q, want %q", summary.ProviderMode, providermodels.ProviderModeIndependent)
	}
	if summary.Organization != nil {
		t.Errorf("Organization should be nil for an independent provider, got %+v", summary.Organization)
	}
	if got := metrics.CounterValue("fleet_partner_provider_contract_responses_total", map[string]string{
		"contract_version": "1", "provider_mode": providermodels.ProviderModeIndependent,
	}); got != 1 {
		t.Errorf("independent provider contract metric = %v, want 1", got)
	}
}

// TestGetMembershipSummaryForProvider_OrganizationManagedReturnsFullSummary
// is the live-DB half of WP20: seed a real organization, branch, ACTIVE
// membership (with a home branch) and policy through the SAME repositories
// production code uses, then prove GetMembershipSummaryForProvider derives
// exactly that state -- never a caller-supplied value, always read back
// from the authoritative row.
func TestGetMembershipSummaryForProvider_OrganizationManagedReturnsFullSummary(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Managed Provider Summary Org", "RC-MPS-1")
	branchID := seedBranch(t, pool, org.ID, "Summary Test Hub")

	providerID := "drv_managed_" + randomHex()
	repo := providerrepos.NewRepository(pool)
	membership := &providermodels.Membership{
		ProviderService:  "driver-hauling-service",
		ServiceCategory:  providermodels.CategoryHauling,
		ProviderID:       &providerID,
		OnboardingSource: providermodels.SourceSponsored,
		RelationshipType: providermodels.RelationshipEmployee,
		Status:           providermodels.StatusActive,
		HomeBranchID:     &branchID,
	}
	if err := repo.CreateMembership(context.Background(), pool, org.ID, membership); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	seedEligibleOrganization(t, pool, org.ID)
	if err := repo.UpsertPolicy(context.Background(), pool, org.ID, &providermodels.Policy{MembershipID: membership.ID}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}

	orgSvc := orgusecases.NewService(orgusecases.Options{
		Pool: pool, Organizations: orgrepos.NewOrganizationRepository(pool), Branches: orgrepos.NewBranchRepository(pool),
	})
	svc := providerusecases.NewService(providerusecases.Options{
		Pool: pool, Providers: repo, Organizations: orgSvc,
	})

	summary, err := svc.GetMembershipSummaryForProvider(context.Background(), membership.ProviderService, providerID)
	if err != nil {
		t.Fatalf("GetMembershipSummaryForProvider: %v", err)
	}
	if summary.ProviderMode != providermodels.ProviderModeOrganizationManaged {
		t.Fatalf("ProviderMode = %q, want %q", summary.ProviderMode, providermodels.ProviderModeOrganizationManaged)
	}
	if summary.Organization == nil || summary.Organization.ID != org.ID {
		t.Fatalf("Organization = %+v, want ID %q", summary.Organization, org.ID)
	}
	if summary.Organization.BranchID != branchID || summary.Organization.BranchName != "Summary Test Hub" {
		t.Errorf("Organization branch = %+v, want id=%q name=%q", summary.Organization, branchID, "Summary Test Hub")
	}
	if summary.MembershipStatus != providermodels.StatusActive {
		t.Errorf("MembershipStatus = %q, want %q", summary.MembershipStatus, providermodels.StatusActive)
	}
	if summary.AssignmentMode != "OFFER" || summary.FieldVisibilityPolicy != "ORGANIZATION" {
		t.Errorf("policy fields = assignmentMode=%q fieldVisibilityPolicy=%q, want OFFER/ORGANIZATION", summary.AssignmentMode, summary.FieldVisibilityPolicy)
	}
	if !summary.ProviderDeclineRequired {
		t.Error("ProviderDeclineRequired must always be true -- it is a platform invariant, DB-enforced")
	}
}

// TestGetMembershipSummaryForProvider_SuspendedMembershipIsNotLive proves a
// SUSPENDED membership -- which still exists as a row, and is still
// "live" for the single-settlement-slot invariant -- continues to report
// ORGANIZATION_MANAGED (suspension is an operational state within the
// managed relationship, not a return to independence; see Section 30 WP36
// "suspended membership" matrix: operational actions are denied elsewhere,
// but the provider does not silently become independent).
func TestGetMembershipSummaryForProvider_SuspendedMembershipIsNotLive(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Suspended Provider Summary Org", "RC-SPS-1")
	providerID := "drv_suspended_" + randomHex()
	membership := seedProviderMembership(t, pool, org.ID, providerID)

	repo := providerrepos.NewRepository(pool)
	if err := database.WithinTx(context.Background(), pool, func(tx pgx.Tx) error {
		return repo.TransitionMembership(context.Background(), tx, org.ID, membership.ID, providermodels.StatusSuspended,
			membership.Version, providermodels.HistoryEntry{ActorType: "system", ActorID: "test", ReasonCode: "summary_test_suspend"})
	}); err != nil {
		t.Fatalf("suspend membership: %v", err)
	}

	orgSvc := orgusecases.NewService(orgusecases.Options{
		Pool: pool, Organizations: orgrepos.NewOrganizationRepository(pool), Branches: orgrepos.NewBranchRepository(pool),
	})
	svc := providerusecases.NewService(providerusecases.Options{Pool: pool, Providers: repo, Organizations: orgSvc})

	summary, err := svc.GetMembershipSummaryForProvider(context.Background(), membership.ProviderService, providerID)
	if err != nil {
		t.Fatalf("GetMembershipSummaryForProvider: %v", err)
	}
	if summary.ProviderMode != providermodels.ProviderModeOrganizationManaged {
		t.Errorf("ProviderMode = %q, want %q (suspended is still a live managed membership)", summary.ProviderMode, providermodels.ProviderModeOrganizationManaged)
	}
	if summary.MembershipStatus != providermodels.StatusSuspended {
		t.Errorf("MembershipStatus = %q, want %q", summary.MembershipStatus, providermodels.StatusSuspended)
	}
}
