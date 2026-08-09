package integration_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type ownerMembershipRecorder struct {
	calls int
}

func (r *ownerMembershipRecorder) CreateOwnerMembership(context.Context, pgx.Tx, string, string, string) (string, error) {
	r.calls++
	return "membership_section31_owner", nil
}

func TestOrganizationOnboardingCreatesResumableDraftAndFailsClosedOnMissingPrerequisites(t *testing.T) {
	pool := testPool(t)
	memberships := &ownerMembershipRecorder{}
	service := orgusecases.NewService(orgusecases.Options{
		Pool:          pool,
		Organizations: orgrepos.NewOrganizationRepository(pool),
		Onboarding:    orgrepos.NewOnboardingRepository(pool),
		Branches:      orgrepos.NewBranchRepository(pool),
		Settings:      orgrepos.NewSettingsRepository(pool),
		Memberships:   memberships,
		Events:        events.NewDomainPublisher(events.NewOutboxRepository(pool)),
	})

	input := orgusecases.CreateDraftInput{
		IdentityID: "identity_section31_owner", OwnerName: "Synthetic Owner",
		LegalName: "Section 31 Resumable Logistics", RegistrationNumber: "S31-ONBOARD-RESUME",
		CountryCode: "NG", OrganizationType: "LOGISTICS_COMPANY", Timezone: "Africa/Lagos",
		DefaultCurrency: "NGN", RequestID: "request-section31-onboarding", CorrelationID: "correlation-section31-onboarding",
	}
	organization, err := service.CreateDraft(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if organization.Status != orgmodels.OrganizationStatusDraft || memberships.calls != 1 {
		t.Fatalf("status=%q owner membership calls=%d, want DRAFT and 1", organization.Status, memberships.calls)
	}

	resumed, err := service.GetOnboardingStatus(context.Background(), organization.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.OrganizationID != organization.ID || resumed.OrganizationStatus != orgmodels.OrganizationStatusDraft || len(resumed.Stages) == 0 {
		t.Fatalf("resumed status=%+v, want durable draft with stages", resumed)
	}
	for _, action := range resumed.AllowedNextActions {
		if action == "SUBMIT_FOR_VERIFICATION" {
			t.Fatal("incomplete onboarding advertised submission as allowed")
		}
	}

	if _, err := service.Submit(context.Background(), orgusecases.SubmitInput{Tenant: tenant.Context{
		OrganizationID: organization.ID, ActorType: tenant.ActorTypeOrganizationUser, ActorID: "user_section31_owner",
	}, ExpectedVersion: organization.Version}); err == nil {
		t.Fatal("submission with missing prerequisites succeeded")
	}
	if _, err := service.CreateDraft(context.Background(), input); err == nil {
		t.Fatal("duplicate organization registration succeeded")
	}
	if memberships.calls != 1 {
		t.Fatalf("duplicate draft created another owner membership: calls=%d", memberships.calls)
	}
	var organizationCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_organizations WHERE registration_number=$1`, input.RegistrationNumber).Scan(&organizationCount); err != nil {
		t.Fatal(err)
	}
	if organizationCount != 1 {
		t.Fatalf("organization rows=%d after duplicate create, want 1", organizationCount)
	}
}
