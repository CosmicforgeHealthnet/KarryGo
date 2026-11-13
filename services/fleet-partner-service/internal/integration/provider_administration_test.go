package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type activeOrgReader struct{}

func (activeOrgReader) OrganizationStatus(context.Context, string) (string, string, error) {
	return "Org", "ACTIVE", nil
}
func (activeOrgReader) OrganizationCurrency(context.Context, string) (string, error) {
	return "NGN", nil
}
func (activeOrgReader) BranchName(context.Context, string, string) (string, string, error) {
	return "Branch", "BRANCH", nil
}

func appStatus(err error) int {
	var appErr *apperrors.Error
	if errors.As(err, &appErr) {
		return appErr.Status
	}
	return 0
}

// TestManagedProviderAdministration covers spec Sections 10 and 39.3:
// edit, capabilities, policies (decline right is fixed), reactivation gates,
// removal that keeps the platform identity, decline history and tenant
// isolation.
func TestManagedProviderAdministration(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := providerrepos.NewRepository(pool)
	service := providerusecases.NewService(providerusecases.Options{
		Pool: pool, Providers: repo, Organizations: activeOrgReader{},
		Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)),
	})
	org := createOrganization(t, pool, "Provider Admin Co", "RC-PADMIN-1")
	other := createOrganization(t, pool, "Provider Admin Other", "RC-PADMIN-2")
	branches := orgrepos.NewBranchRepository(pool)
	home := &orgmodels.Branch{Code: "HQ", Name: "HQ", Type: "OFFICE", Timezone: "Africa/Lagos"}
	foreign := &orgmodels.Branch{Code: "HQ", Name: "HQ", Type: "OFFICE", Timezone: "Africa/Lagos"}
	if err := branches.Create(ctx, pool, org.ID, home); err != nil {
		t.Fatal(err)
	}
	if err := branches.Create(ctx, pool, other.ID, foreign); err != nil {
		t.Fatal(err)
	}
	providerID := "prv_admin_1"
	membership := &providermodels.Membership{
		ProviderService: "driver-dispatch-delivery-service", ServiceCategory: providermodels.CategoryDispatch,
		OnboardingSource: providermodels.SourceSponsored, RelationshipType: providermodels.RelationshipEmployee,
		Status: providermodels.StatusActive, ProviderID: &providerID,
	}
	if err := repo.CreateMembership(ctx, pool, org.ID, membership); err != nil {
		t.Fatal(err)
	}
	tc := tenant.Context{OrganizationID: org.ID, OrganizationUserID: "oru_admin", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "oru_admin", CorrelationID: "corr-padmin"}

	// Edit: another organization's branch is not a valid home branch.
	if _, err := service.UpdateMembership(ctx, providerusecases.UpdateMembershipInput{Tenant: tc, MembershipID: membership.ID, HomeBranchID: &foreign.ID}); appStatus(err) != 422 {
		t.Fatalf("foreign home branch = %v", err)
	}
	contractor := "contractor"
	updated, err := service.UpdateMembership(ctx, providerusecases.UpdateMembershipInput{Tenant: tc, MembershipID: membership.ID, HomeBranchID: &home.ID, RelationshipType: &contractor})
	if err != nil || updated.RelationshipType != "CONTRACTOR" || *updated.HomeBranchID != home.ID {
		t.Fatalf("update: %+v %v", updated, err)
	}

	// Capabilities: the service category cannot be widened.
	if _, err := service.ReplaceCapabilities(ctx, tc, membership.ID, []providermodels.Capability{{ServiceType: "HAULING"}}); appStatus(err) != 422 {
		t.Fatalf("cross-category capability = %v", err)
	}
	future := time.Now().Add(30 * 24 * time.Hour)
	saved, err := service.ReplaceCapabilities(ctx, tc, membership.ID, []providermodels.Capability{{VehicleClasses: []string{"MOTORCYCLE"}, OperatingZones: []string{"LAGOS_MAINLAND"}, ValidUntil: &future}})
	if err != nil || len(saved) != 1 || saved[0].ServiceType != "DISPATCH" {
		t.Fatalf("capabilities: %+v %v", saved, err)
	}

	// Policy: default is OFFER with the decline right; it cannot be removed.
	policy, err := service.Policy(ctx, org.ID, membership.ID)
	if err != nil || !policy.ProviderDeclineRequired || policy.AssignmentMode != "OFFER" {
		t.Fatalf("default policy: %+v %v", policy, err)
	}
	if _, err := service.ReplacePolicy(ctx, tc, providermodels.Policy{MembershipID: membership.ID, ProviderDeclineRequired: false}, true); appStatus(err) != 422 {
		t.Fatalf("disabling the decline right = %v", err)
	}
	policy, err = service.ReplacePolicy(ctx, tc, providermodels.Policy{MembershipID: membership.ID, AssignmentMode: "directed_offer", VisibilityPolicy: "branch"}, false)
	if err != nil || policy.AssignmentMode != "DIRECTED_OFFER" || !policy.ProviderDeclineRequired {
		t.Fatalf("policy: %+v %v", policy, err)
	}

	// Decline history starts empty and is tenant scoped.
	if declines, err := service.Declines(ctx, org.ID, membership.ID, 0); err != nil || len(declines) != 0 {
		t.Fatalf("declines: %v %v", declines, err)
	}
	if _, err := service.Declines(ctx, other.ID, membership.ID, 0); appStatus(err) != 404 {
		t.Fatalf("cross-tenant declines = %v", err)
	}

	// Suspend, then reactivation is gated: no sub-wallet, no platform
	// confirmation, so the provider stays suspended.
	if err := service.ChangeStatus(ctx, providerusecases.ChangeStatusInput{Tenant: tc, MembershipID: membership.ID, TargetStatus: providermodels.StatusSuspended}); err != nil {
		t.Fatal(err)
	}
	if err := service.Reactivate(ctx, tc, membership.ID, 0); appStatus(err) != 409 {
		t.Fatalf("reactivation without gates = %v", err)
	}

	// Removal from SUSPENDED goes through ENDING to REMOVED and keeps the
	// provider's platform identity reference.
	if err := service.Remove(ctx, tc, membership.ID, "", 0); err != nil {
		t.Fatalf("remove: %v", err)
	}
	removed, _ := repo.GetMembership(ctx, org.ID, membership.ID)
	if removed.Status != providermodels.StatusRemoved || removed.ProviderID == nil || *removed.ProviderID != providerID {
		t.Fatalf("removed membership: %+v", removed)
	}
	history, _ := repo.ListHistory(ctx, org.ID, membership.ID)
	if len(history) < 3 {
		t.Fatalf("history entries = %d, want suspend, ending, removed", len(history))
	}
	if _, err := service.UpdateMembership(ctx, providerusecases.UpdateMembershipInput{Tenant: tc, MembershipID: membership.ID, RelationshipType: &contractor}); appStatus(err) != 409 {
		t.Fatalf("editing a removed provider = %v", err)
	}
	if err := service.Remove(ctx, tenant.Context{OrganizationID: other.ID}, membership.ID, "", 0); appStatus(err) != 404 {
		t.Fatalf("cross-tenant removal = %v", err)
	}
}
