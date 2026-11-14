package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/orgprovider"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// FLEET-10.2 for DISPATCH and TAXI: Fleet's real Dispatch/Taxi
// managed-provider clients drive an httptest stand-in of the owning service's
// private contract (shared/go/orgprovider). The provider's own accept or
// decline happens on the owning service; Fleet learns it only by reading the
// association back, and only then leaves PENDING_PROVIDER_ACCEPTANCE.

const managedProviderSecret = "fleet-managed-provider-test-secret"

type fakeOwningProvider struct {
	email, phone, org, accountState string
	version                         int64
}

type fakeOwningLink struct {
	view orgprovider.LinkView
	key  string
}

// fakeOwningService is the owning service's private contract, in memory. It
// verifies Fleet's HMAC signature and refuses writes without the three write
// headers, exactly as the real services do.
type fakeOwningService struct {
	mu          sync.Mutex
	category    string
	providers   map[string]*fakeOwningProvider
	links       map[string]*fakeOwningLink
	sponsorKeys map[string]string
	sponsored   []orgprovider.SponsoredRequest
	server      *httptest.Server
}

func newFakeOwningService(t *testing.T, category string, base string) *fakeOwningService {
	t.Helper()
	f := &fakeOwningService{category: category, providers: map[string]*fakeOwningProvider{},
		links: map[string]*fakeOwningLink{}, sponsorKeys: map[string]string{}}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group(base)
	group.Use(serviceauth.Middleware(serviceauth.NewVerifier(serviceauth.Secrets{"fleet-partner-service": []byte(managedProviderSecret)}, time.Minute)))
	writeHeaders := func(c *gin.Context) bool {
		for _, h := range []string{"Idempotency-Key", "X-Correlation-ID", "X-Causation-ID"} {
			if strings.TrimSpace(c.GetHeader(h)) == "" {
				c.JSON(400, gin.H{"error": gin.H{"code": orgprovider.CodeWriteHeadersRequired}})
				return false
			}
		}
		return true
	}
	ok := func(c *gin.Context, status int, v any) { c.JSON(status, gin.H{"success": true, "data": v}) }
	group.POST("/providers/resolve", func(c *gin.Context) {
		var body orgprovider.ResolveRequest
		_ = c.ShouldBindJSON(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		var byEmail, byPhone *orgprovider.Match
		for id, p := range f.providers {
			m := &orgprovider.Match{ProviderID: id, PlatformStatus: p.accountState, ManagedByOrgID: p.org, SourceVersion: p.version}
			if p.email == body.EmailNormalized {
				byEmail = m
			}
			if p.phone == body.PhoneE164 {
				byPhone = m
			}
		}
		ok(c, 200, orgprovider.Resolution(f.category, byEmail, byPhone))
	})
	group.POST("/providers/sponsored", func(c *gin.Context) {
		if !writeHeaders(c) {
			return
		}
		var body orgprovider.SponsoredRequest
		_ = c.ShouldBindJSON(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		id, seen := f.sponsorKeys[c.GetHeader("Idempotency-Key")]
		if !seen {
			id = uuid.NewString()
			f.providers[id] = &fakeOwningProvider{email: body.EmailNormalized, phone: body.PhoneE164,
				org: body.OrganizationID, accountState: orgprovider.AccountPendingVerification, version: 1}
			f.sponsorKeys[c.GetHeader("Idempotency-Key")] = id
			f.sponsored = append(f.sponsored, body)
		}
		ok(c, 201, orgprovider.ProviderView{ProviderID: id, AccountState: f.providers[id].accountState, SourceVersion: 1})
	})
	group.GET("/providers/:id", func(c *gin.Context) {
		f.mu.Lock()
		defer f.mu.Unlock()
		p, found := f.providers[c.Param("id")]
		if !found {
			c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
			return
		}
		ok(c, 200, orgprovider.ProviderView{ProviderID: c.Param("id"), AccountState: p.accountState, SourceVersion: p.version})
	})
	group.POST("/provider-links", func(c *gin.Context) {
		if !writeHeaders(c) {
			return
		}
		var body orgprovider.LinkRequest
		_ = c.ShouldBindJSON(&body)
		if body.OrganizationID != c.GetHeader("X-Organization-ID") {
			c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, l := range f.links {
			if l.key == c.GetHeader("Idempotency-Key") {
				ok(c, 200, l.view)
				return
			}
		}
		id := uuid.NewString()
		f.links[id] = &fakeOwningLink{key: c.GetHeader("Idempotency-Key"), view: orgprovider.LinkView{LinkID: id,
			OrganizationID: body.OrganizationID, MembershipID: body.MembershipID, ProviderID: body.ProviderID,
			Status: orgprovider.StateRequested, SourceVersion: 1}}
		ok(c, 201, f.links[id].view)
	})
	group.GET("/provider-links/:id", func(c *gin.Context) {
		f.mu.Lock()
		defer f.mu.Unlock()
		l, found := f.links[c.Param("id")]
		if !found || l.view.OrganizationID != c.GetHeader("X-Organization-ID") {
			c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
			return
		}
		ok(c, 200, l.view)
	})
	f.server = httptest.NewServer(engine)
	t.Cleanup(f.server.Close)
	return f
}

// decide is the provider deciding in their own app on the owning service.
func (f *fakeOwningService) decide(t *testing.T, providerID string, accept bool) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, l := range f.links {
		if l.view.ProviderID == providerID && l.view.Status == orgprovider.StateRequested {
			now := time.Now().UTC()
			l.view.Status, l.view.DecidedAt = orgprovider.ProviderDecision(accept), &now
			l.view.SourceVersion++
			return
		}
	}
	t.Fatalf("no requested association for provider %s", providerID)
}

func (f *fakeOwningService) verify(providerID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.providers[providerID].accountState = orgprovider.AccountActive
	f.providers[providerID].version++
}

type activeSubWallet struct{}

func (activeSubWallet) EnsureOrganizationWallet(context.Context, platformclients.CallMeta, platformclients.WalletProvisionRequest) (platformclients.WalletAccount, error) {
	return platformclients.WalletAccount{AccountID: "wal_org", Status: "ACTIVE", SourceVersion: 1}, nil
}
func (activeSubWallet) EnsureProviderSubWallet(_ context.Context, _ platformclients.CallMeta, r platformclients.WalletProvisionRequest) (platformclients.WalletAccount, error) {
	return platformclients.WalletAccount{AccountID: "wal_" + r.ProviderMembershipID, Status: "ACTIVE", SourceVersion: 1}, nil
}
func (activeSubWallet) ReadAccount(_ context.Context, _ platformclients.CallMeta, id string) (platformclients.WalletAccount, error) {
	return platformclients.WalletAccount{AccountID: id, Status: "ACTIVE", SourceVersion: 1}, nil
}
func (activeSubWallet) DisableForClosure(context.Context, platformclients.CallMeta, string, string) error {
	return nil
}
func (activeSubWallet) Configured() bool { return true }

func TestManagedProviderCreationForDispatchAndTaxiReachesActiveOnlyAfterProviderAccepts(t *testing.T) {
	for _, tc := range []struct {
		category string
		base     string
		client   func(platformclients.Options) *platformclients.ManagedProviderClient
	}{
		{platformclients.CategoryDispatch, platformclients.DispatchManagedProviderBase, platformclients.NewDispatchProviderClient},
		{platformclients.CategoryTaxi, platformclients.TaxiManagedProviderBase, platformclients.NewTaxiProviderClient},
	} {
		t.Run(tc.category, func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()
			owning := newFakeOwningService(t, tc.category, tc.base)
			client := tc.client(platformclients.Options{BaseURL: owning.server.URL, Secret: []byte(managedProviderSecret),
				Timeout: 5 * time.Second, Metrics: observability.NewMetrics()})
			organization := createOrganization(t, pool, "Managed "+tc.category+" Co", "RC-MP-"+tc.category+"-"+uuid.NewString()[:6])
			other := createOrganization(t, pool, "Managed Other "+tc.category, "RC-MPO-"+tc.category+"-"+uuid.NewString()[:6])
			seedEligibleOrganization(t, pool, organization.ID)
			home := seedBranch(t, pool, organization.ID, "Managed "+tc.category+" Home")
			service := providerusecases.NewService(providerusecases.Options{
				Pool: pool, Providers: providerrepos.NewRepository(pool), Organizations: providerProvisioningOrganizationStub{},
				Contracts: platformclients.ContractSet{
					Providers: map[string]platformclients.ProviderContract{tc.category: client},
					Wallet:    activeSubWallet{},
				},
				Eligibility:        newEligibilityService(pool, healthyAuthority("", "")),
				Sender:             &authclients.FakeSender{},
				Events:             events.NewDomainPublisher(events.NewOutboxRepository(pool)),
				InvitationSecret:   []byte("fleet-managed-provider-invitation-secret-32b"),
				DefaultCountryCode: "+234",
			})
			actor := tenant.Context{OrganizationID: organization.ID, OrganizationUserID: "oru_mp", ActorType: tenant.ActorTypeOrganizationUser,
				ActorID: "oru_mp", RequestID: "req-mp-" + tc.category, CorrelationID: "cor-mp-" + tc.category}
			create := func(email, phone string) string {
				result, err := service.CreateProvider(ctx, providerusecases.CreateProviderInput{Tenant: actor, LegalName: "Ada Okafor",
					Email: email, Phone: phone, ServiceCategory: tc.category, RelationshipType: "EMPLOYEE",
					IdempotencyKey: "mp-create:" + tc.category + ":" + email})
				if err != nil {
					t.Fatalf("create %s provider: %v", tc.category, err)
				}
				return result.Membership.ID
			}
			membershipState := func(id string) (status, providerID, source string) {
				if err := pool.QueryRow(ctx, `SELECT status, COALESCE(provider_id,''), onboarding_source
					FROM fleet_partner_organization_provider_memberships WHERE id=$1`, id).Scan(&status, &providerID, &source); err != nil {
					t.Fatal(err)
				}
				return
			}
			reconcile := func() {
				if _, err := pool.Exec(ctx, `UPDATE fleet_partner_provider_provisioning_sagas SET next_attempt_at=now()
					WHERE organization_id=$1`, organization.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := service.Reconcile(ctx, 50); err != nil {
					t.Fatal(err)
				}
			}

			// Sponsored creation: the owning service creates the identity and a
			// REQUESTED association; Fleet waits for the provider.
			membershipID := create("ada."+strings.ToLower(tc.category)+"@example.test", "+2348031234567")
			status, providerID, source := membershipState(membershipID)
			if status != "PENDING_PROVIDER_ACCEPTANCE" || providerID == "" || source != "ORGANIZATION_SPONSORED" {
				t.Fatalf("after create: status=%s provider=%q source=%s", status, providerID, source)
			}
			if len(owning.sponsored) != 1 || owning.sponsored[0].OnboardingMode != "ORGANIZATION_SPONSORED" || owning.sponsored[0].ServiceCategory != tc.category {
				t.Fatalf("sponsored requests: %+v", owning.sponsored)
			}
			var associationID string
			if err := pool.QueryRow(ctx, `SELECT COALESCE(association_id,'') FROM fleet_partner_provider_provisioning_sagas WHERE membership_id=$1`, membershipID).Scan(&associationID); err != nil || associationID == "" {
				t.Fatalf("association id not recorded: %q %v", associationID, err)
			}

			// Without the provider's decision nothing moves, however often Fleet
			// reconciles.
			reconcile()
			if status, _, _ := membershipState(membershipID); status != "PENDING_PROVIDER_ACCEPTANCE" {
				t.Fatalf("membership moved without consent: %s", status)
			}
			// Another organization cannot read or apply this association.
			if _, err := service.SyncAssociation(ctx, other.ID, membershipID); err == nil {
				t.Fatal("cross-organization association sync succeeded")
			}

			// The provider accepts in their own app; Fleet reads it back.
			owning.decide(t, providerID, true)
			if _, err := service.UpdateMembership(ctx, providerusecases.UpdateMembershipInput{Tenant: actor, MembershipID: membershipID, HomeBranchID: &home}); err != nil {
				t.Fatal(err)
			}

			// Concurrent read-backs apply the decision exactly once.
			var wg sync.WaitGroup
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); _, _ = service.SyncAssociation(ctx, organization.ID, membershipID) }()
			}
			wg.Wait()
			if status, _, _ := membershipState(membershipID); status != "PENDING_SETUP" {
				t.Fatalf("after acceptance: %s (provider not yet verified must stop at PENDING_SETUP)", status)
			}
			var accepted, audits, histories int
			_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id=$1
				AND event_type='organization.provider.association_accepted.v1'`, membershipID).Scan(&accepted)
			_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records WHERE resource_id=$1
				AND action='provider.association_decided'`, membershipID).Scan(&audits)
			_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_provider_membership_history WHERE membership_id=$1
				AND to_status='PENDING_SETUP' AND actor_type='provider'`, membershipID).Scan(&histories)
			if accepted != 1 || audits != 1 || histories != 1 {
				t.Fatalf("acceptance applied %d events, %d audits, %d history rows; want exactly one each", accepted, audits, histories)
			}
			var subWallet string
			if err := pool.QueryRow(ctx, `SELECT status FROM fleet_partner_provider_subwallet_links WHERE provider_membership_id=$1`, membershipID).Scan(&subWallet); err != nil || subWallet != "ACTIVE" {
				t.Fatalf("sub-wallet after acceptance: %q %v", subWallet, err)
			}

			// Platform verification still gates activation; once the owning
			// service reports ACTIVE the reconciler activates and completes.
			owning.verify(providerID)
			reconcile()
			if status, _, _ := membershipState(membershipID); status != "ACTIVE" {
				t.Fatalf("after verification: %s", status)
			}
			var overall string
			_ = pool.QueryRow(ctx, `SELECT overall_status FROM fleet_partner_provider_provisioning_sagas WHERE membership_id=$1`, membershipID).Scan(&overall)
			if overall != "COMPLETED" {
				t.Fatalf("saga overall=%s", overall)
			}

			// A provider who declines ends DECLINED, with no sub-wallet and a
			// saga that stops reconciling.
			declinedID := create("bola."+strings.ToLower(tc.category)+"@example.test", "+2348037654321")
			_, declinedProvider, _ := membershipState(declinedID)
			owning.decide(t, declinedProvider, false)
			reconcile()
			if status, _, _ := membershipState(declinedID); status != "DECLINED" {
				t.Fatalf("after decline: %s", status)
			}
			var declinedOverall string
			var wallets int
			_ = pool.QueryRow(ctx, `SELECT overall_status FROM fleet_partner_provider_provisioning_sagas WHERE membership_id=$1`, declinedID).Scan(&declinedOverall)
			_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_provider_subwallet_links WHERE provider_membership_id=$1`, declinedID).Scan(&wallets)
			if declinedOverall != "FAILED_PERMANENT" || wallets != 0 {
				t.Fatalf("declined saga=%s sub-wallets=%d", declinedOverall, wallets)
			}
		})
	}
}

// Without the flag-gated client the category stays fail-closed.
func TestManagedProviderCreationFailsClosedWhenServiceUnconfigured(t *testing.T) {
	pool := testPool(t)
	organization := createOrganization(t, pool, "Managed Unconfigured Co", "RC-MP-UNCONF-"+uuid.NewString()[:6])
	seedEligibleOrganization(t, pool, organization.ID)
	service := providerusecases.NewService(providerusecases.Options{
		Pool: pool, Providers: providerrepos.NewRepository(pool), Organizations: providerProvisioningOrganizationStub{},
		Contracts:   platformclients.NewUnconfiguredContracts(),
		Eligibility: newEligibilityService(pool, healthyAuthority("", "")),
		Sender:      &authclients.FakeSender{}, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		InvitationSecret: []byte("fleet-managed-provider-invitation-secret-32b"), DefaultCountryCode: "+234",
	})
	_, err := service.CreateProvider(context.Background(), providerusecases.CreateProviderInput{
		Tenant:    tenant.Context{OrganizationID: organization.ID, OrganizationUserID: "oru_u", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "oru_u"},
		LegalName: "Ada Okafor", Email: "unconfigured@example.test", Phone: "+2348031111111", ServiceCategory: platformclients.CategoryTaxi,
		IdempotencyKey: "mp-unconfigured-taxi",
	})
	if appStatus(err) != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured taxi provider service: %v", err)
	}
}
