package integration_test

import (
	"context"
	"sync"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type providerProvisioningOrganizationStub struct{}

func (providerProvisioningOrganizationStub) OrganizationStatus(context.Context, string) (string, string, error) {
	return "Section 31 Synthetic Logistics", "ACTIVE", nil
}
func (providerProvisioningOrganizationStub) OrganizationCurrency(context.Context, string) (string, error) {
	return "NGN", nil
}
func (providerProvisioningOrganizationStub) BranchName(context.Context, string, string) (string, string, error) {
	return "", "", nil
}

type unknownProvisioningContract struct {
	mu               sync.Mutex
	providerExists   bool
	resolveCalls     int
	provisionCalls   int
	associationCalls int
}

func (c *unknownProvisioningContract) Resolve(context.Context, platformclients.CallMeta, platformclients.ProviderResolveRequest) (platformclients.ProviderResolution, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resolveCalls++
	if c.providerExists {
		return platformclients.ProviderResolution{Outcome: platformclients.ProviderResolveMatched, ProviderID: "provider_section31_unknown", PlatformStatus: "ACTIVE", ServiceCategory: platformclients.CategoryDispatch, SourceVersion: 2}, nil
	}
	return platformclients.ProviderResolution{Outcome: platformclients.ProviderResolveNotFound}, nil
}

func (c *unknownProvisioningContract) ProvisionSponsored(context.Context, platformclients.CallMeta, platformclients.ProviderProvisionRequest) (platformclients.ProviderIdentity, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.provisionCalls++
	c.providerExists = true // the remote write committed, but its response was lost.
	return platformclients.ProviderIdentity{}, &platformclients.Error{Service: "dispatch-provider-test", Outcome: platformclients.OutcomeUnknown}
}

func (c *unknownProvisioningContract) ReadProvider(context.Context, platformclients.CallMeta, string) (platformclients.ProviderIdentity, error) {
	return platformclients.ProviderIdentity{ProviderID: "provider_section31_unknown", PlatformStatus: "ACTIVE", SourceVersion: 2}, nil
}

func (c *unknownProvisioningContract) CreateAssociation(context.Context, platformclients.CallMeta, platformclients.ProviderAssociationRequest) (platformclients.ProviderAssociation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.associationCalls++
	return platformclients.ProviderAssociation{AssociationID: "association_section31_unknown", Status: platformclients.AssociationAwaitingProvider, SourceVersion: 1}, nil
}

func (*unknownProvisioningContract) ReadAssociation(context.Context, platformclients.CallMeta, string) (platformclients.ProviderAssociation, error) {
	return platformclients.ProviderAssociation{AssociationID: "association_section31_unknown", Status: platformclients.AssociationAwaitingProvider, SourceVersion: 1}, nil
}
func (*unknownProvisioningContract) RevokeOrganizationEligibility(context.Context, platformclients.CallMeta, string, string, string) error {
	return nil
}
func (*unknownProvisioningContract) Configured() bool { return true }

func TestProviderProvisioningUnknownOutcomeReconcilesByContactWithoutDuplicateIdentity(t *testing.T) {
	pool := testPool(t)
	organization := createOrganization(t, pool, "Section 31 Synthetic Logistics", "S31-PROVIDER-UNKNOWN")
	seedEligibleOrganization(t, pool, organization.ID)
	contract := &unknownProvisioningContract{}
	sender := &authclients.FakeSender{}
	repository := providerrepos.NewRepository(pool)
	service := providerusecases.NewService(providerusecases.Options{
		Pool: pool, Providers: repository, Organizations: providerProvisioningOrganizationStub{},
		Contracts: platformclients.ContractSet{
			Providers: map[string]platformclients.ProviderContract{platformclients.CategoryDispatch: contract},
			Wallet:    platformclients.UnconfiguredWallet{Service: "wallet-test"},
		},
		Eligibility:        newEligibilityService(pool, healthyAuthority("", "")),
		Sender:             sender,
		Events:             events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		InvitationSecret:   []byte("section31-provider-invitation-secret-32bytes"),
		AcceptURLTemplate:  "https://synthetic.invalid/provider/accept",
		DefaultCountryCode: "+234",
	})

	_, err := service.CreateProvider(context.Background(), providerusecases.CreateProviderInput{
		Tenant: tenant.Context{
			OrganizationID: organization.ID, OrganizationUserID: "user_section31_owner",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "user_section31_owner",
			RequestID: "request-section31-provider", CorrelationID: "correlation-section31-provider",
		},
		LegalName: "Synthetic Provider", Email: "provider.section31@example.test", Phone: "+2348012345678",
		ServiceCategory: platformclients.CategoryDispatch, RelationshipType: "CONTRACTOR",
		IdempotencyKey: "section31-provider-unknown-outcome",
	})
	if err == nil {
		t.Fatal("lost provisioning response was treated as confirmed success")
	}

	var membershipID string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1`, organization.ID).Scan(&membershipID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE fleet_partner_provider_provisioning_sagas SET next_attempt_at=now() WHERE membership_id=$1`, membershipID); err != nil {
		t.Fatal(err)
	}

	progressed, err := service.Reconcile(context.Background(), 10)
	if err != nil || progressed != 1 {
		t.Fatalf("reconcile progressed=%d err=%v, want 1 and nil", progressed, err)
	}
	contract.mu.Lock()
	resolveCalls, provisionCalls, associationCalls := contract.resolveCalls, contract.provisionCalls, contract.associationCalls
	contract.mu.Unlock()
	if resolveCalls != 2 || provisionCalls != 1 || associationCalls != 1 {
		t.Fatalf("resolve=%d provision=%d association=%d; reconciliation must read back once and never repeat provisioning", resolveCalls, provisionCalls, associationCalls)
	}

	var providerID, membershipStatus string
	if err := pool.QueryRow(context.Background(), `SELECT provider_id,status FROM fleet_partner_organization_provider_memberships WHERE id=$1`, membershipID).Scan(&providerID, &membershipStatus); err != nil {
		t.Fatal(err)
	}
	if providerID != "provider_section31_unknown" || membershipStatus != "PENDING_PROVIDER_ACCEPTANCE" {
		t.Fatalf("providerID=%q status=%q, want authoritative ID and pending provider consent", providerID, membershipStatus)
	}
	var pending, cancelled int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE status='PENDING'), count(*) FILTER (WHERE status='CANCELLED') FROM fleet_partner_provider_invitations WHERE membership_id=$1`, membershipID).Scan(&pending, &cancelled); err != nil {
		t.Fatal(err)
	}
	if pending != 1 || cancelled != 1 || len(sender.SentInvitations) != 1 {
		t.Fatalf("pending invitations=%d cancelled anchors=%d sent=%d, want 1/1/1", pending, cancelled, len(sender.SentInvitations))
	}
}
