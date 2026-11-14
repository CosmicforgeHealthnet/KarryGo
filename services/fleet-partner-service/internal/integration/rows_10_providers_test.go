package integration_test

// Requirement-row tests for spec Section 10 (organization-managed provider
// accounts) and its detail Section 39: provisioning status (10.3c), the
// mandatory sub-wallet activation gate (10.13) and the corporate context the
// execution services read for an organization-managed provider (10.16b).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/noncestore"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// rows10Authority is the owning provider service (Dispatch) for these tests:
// every contact resolves to an existing provider whose platform status the
// test controls, and associations wait for the provider's own decision.
type rows10Authority struct {
	mu       sync.Mutex
	statuses map[string]string // providerID -> platform status
	revoked  []string          // providerID|organizationID eligibility revocations
}

func rows10ProviderID(email string) string {
	return "prv_" + strings.ReplaceAll(strings.SplitN(email, "@", 2)[0], ".", "_")
}

func (a *rows10Authority) setPlatformStatus(providerID, status string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statuses[providerID] = status
}

func (a *rows10Authority) Resolve(_ context.Context, _ platformclients.CallMeta, request platformclients.ProviderResolveRequest) (platformclients.ProviderResolution, error) {
	return platformclients.ProviderResolution{Outcome: platformclients.ProviderResolveMatched,
		ProviderID: rows10ProviderID(request.EmailNormalized), PlatformStatus: "ACTIVE",
		ServiceCategory: platformclients.CategoryDispatch, SourceVersion: 1}, nil
}

func (a *rows10Authority) ProvisionSponsored(context.Context, platformclients.CallMeta, platformclients.ProviderProvisionRequest) (platformclients.ProviderIdentity, error) {
	return platformclients.ProviderIdentity{}, &platformclients.Error{Service: "dispatch", Outcome: platformclients.OutcomePermanent}
}

func (a *rows10Authority) ReadProvider(_ context.Context, _ platformclients.CallMeta, providerID string) (platformclients.ProviderIdentity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	status, ok := a.statuses[providerID]
	if !ok {
		status = "ACTIVE"
	}
	return platformclients.ProviderIdentity{ProviderID: providerID, PlatformStatus: status, SourceVersion: 1}, nil
}

func (a *rows10Authority) CreateAssociation(_ context.Context, _ platformclients.CallMeta, request platformclients.ProviderAssociationRequest) (platformclients.ProviderAssociation, error) {
	return platformclients.ProviderAssociation{AssociationID: "asc_" + request.MembershipID,
		Status: platformclients.AssociationAwaitingProvider, SourceVersion: 1}, nil
}

func (a *rows10Authority) ReadAssociation(_ context.Context, _ platformclients.CallMeta, id string) (platformclients.ProviderAssociation, error) {
	return platformclients.ProviderAssociation{AssociationID: id, Status: platformclients.AssociationAwaitingProvider, SourceVersion: 1}, nil
}

func (a *rows10Authority) RevokeOrganizationEligibility(_ context.Context, _ platformclients.CallMeta, providerID, organizationID, _ string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revoked = append(a.revoked, providerID+"|"+organizationID)
	return nil
}

func (a *rows10Authority) Configured() bool { return true }

// rows10Organizations reads organization facts from the database, so the
// organization-active activation gate sees real lifecycle state.
type rows10Organizations struct{ pool *pgxpool.Pool }

func (r rows10Organizations) OrganizationStatus(ctx context.Context, id string) (string, string, error) {
	var name, status string
	err := r.pool.QueryRow(ctx, `SELECT legal_name, status FROM fleet_partner_organizations WHERE id = $1`, id).Scan(&name, &status)
	return name, status, err
}

func (r rows10Organizations) OrganizationCurrency(ctx context.Context, id string) (string, error) {
	var currency string
	err := r.pool.QueryRow(ctx, `SELECT default_currency FROM fleet_partner_organizations WHERE id = $1`, id).Scan(&currency)
	return currency, err
}

func (r rows10Organizations) BranchName(ctx context.Context, organizationID, branchID string) (string, string, error) {
	var name, branchType string
	err := r.pool.QueryRow(ctx, `SELECT name, type FROM fleet_partner_branches WHERE organization_id = $1 AND id = $2`, organizationID, branchID).Scan(&name, &branchType)
	return name, branchType, err
}

type rows10Fixture struct {
	pool      *pgxpool.Pool
	org       string
	branch    string
	owner     string
	authority *rows10Authority
	wallet    *rows13Wallet
	service   *providerusecases.Service
	router    http.Handler
}

func newRows10Fixture(t *testing.T) *rows10Fixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, "Rows10 Managed Providers Ltd", "RC-R10-"+randomHex()[:8])
	if _, err := pool.Exec(context.Background(), `UPDATE fleet_partner_organizations SET status = 'ACTIVE' WHERE id = $1`, org.ID); err != nil {
		t.Fatal(err)
	}
	seedEligibleOrganization(t, pool, org.ID)
	branch := seedBranch(t, pool, org.ID, "Rows10 Hub "+randomHex()[:4])
	owner := seedApprovalMember(t, pool, org.ID, staffmodels.SystemOwnerRoleID, "r10-owner")
	authority := &rows10Authority{statuses: map[string]string{}}
	wallet := newRows13Wallet(t)
	service := providerusecases.NewService(providerusecases.Options{
		Pool: pool, Providers: providerrepos.NewRepository(pool), Organizations: rows10Organizations{pool: pool},
		Contracts: platformclients.ContractSet{
			Providers: map[string]platformclients.ProviderContract{platformclients.CategoryDispatch: authority},
			Wallet: platformclients.NewWalletClient(platformclients.Options{ServiceName: "payment-wallet-service",
				BaseURL: wallet.server.URL, Secret: []byte(rows13WalletSecret), Timeout: 5 * time.Second}),
		},
		Eligibility:        newEligibilityService(pool, healthyAuthority("", "")),
		Sender:             authclients.NewFakeSender(),
		Events:             events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		InvitationSecret:   []byte("rows10-provider-invitation-secret-value"),
		AcceptURLTemplate:  "https://fleet.example.test/provider/accept",
		DefaultCountryCode: "+234",
	})
	return &rows10Fixture{pool: pool, org: org.ID, branch: branch, owner: owner, authority: authority,
		wallet: wallet, service: service,
		router: providerAdminRouter(t, pool, service, org.ID, owner, nil)}
}

// createProvider runs the real POST /providers command and returns the
// membership id.
func (f *rows10Fixture) createProvider(t *testing.T, email, phone string) string {
	t.Helper()
	code, body := rowsCall(t, f.router, "POST", "/fleet-partner/v1/providers", map[string]any{
		"legalName": "Rows Ten Rider", "email": email, "phone": phone, "serviceCategory": "DISPATCH",
		"homeBranchId": f.branch, "relationshipType": "EMPLOYEE"}, map[string]string{"Idempotency-Key": "r10-" + email})
	if code != http.StatusCreated {
		t.Fatalf("POST /providers = %d %v", code, body)
	}
	return rowsData(t, body)["membershipId"].(string)
}

func (f *rows10Fixture) membershipStatus(t *testing.T, id string) string {
	t.Helper()
	var status string
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM fleet_partner_organization_provider_memberships WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func gateNamed(gates []providerusecases.ActivationGate, name string) providerusecases.ActivationGate {
	for _, gate := range gates {
		if gate.Name == name {
			return gate
		}
	}
	return providerusecases.ActivationGate{Name: name}
}

// FLEET-10.3c GET /providers/{id}/provisioning-status: the provisioning saga
// of a real create command is readable step by step, inside the tenant and
// permission boundary.
func TestRow10_3c_ProviderProvisioningStatus(t *testing.T) {
	f := newRows10Fixture(t)
	membership := f.createProvider(t, "ada.status@example.com", "+2348031110001")

	code, body := rowsCall(t, f.router, "GET", "/fleet-partner/v1/providers/"+membership+"/provisioning-status", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("provisioning status = %d %v", code, body)
	}
	saga := rowsData(t, body)
	for _, field := range []string{"overallStatus", "providerResolutionStatus", "providerIdentityStatus", "associationStatus", "subWalletStatus", "notificationStatus"} {
		if _, ok := saga[field]; !ok {
			t.Fatalf("provisioning status lacks %q: %v", field, saga)
		}
	}
	if saga["providerResolutionStatus"] == "" || saga["overallStatus"] == "" {
		t.Fatalf("an existing identity was resolved but the saga does not say so: %v", saga)
	}
	if strings.Contains(strings.ToUpper(saga["overallStatus"].(string)), "COMPLETED") {
		t.Fatalf("provisioning cannot be complete before the provider has consented: %v", saga)
	}

	// Refusals: unknown id, another organization, no providers.read.
	if code, _ := rowsCall(t, f.router, "GET", "/fleet-partner/v1/providers/opm_unknown/provisioning-status", nil, nil); code != http.StatusNotFound {
		t.Fatalf("unknown membership = %d, want 404", code)
	}
	other := createOrganization(t, f.pool, "Rows10 Other Org", "RC-R10-OTHER-"+randomHex()[:6])
	otherOwner := seedApprovalMember(t, f.pool, other.ID, staffmodels.SystemOwnerRoleID, "r10-other")
	if code, _ := rowsCall(t, providerAdminRouter(t, f.pool, f.service, other.ID, otherOwner, nil), "GET", "/fleet-partner/v1/providers/"+membership+"/provisioning-status", nil, nil); code != http.StatusNotFound {
		t.Fatalf("cross-tenant provisioning status = %d, want 404", code)
	}
	analyst := seedApprovalMember(t, f.pool, f.org, "rol_system_crm", "r10-crm")
	if code, _ := rowsCall(t, providerAdminRouter(t, f.pool, f.service, f.org, analyst, nil), "GET", "/fleet-partner/v1/providers/"+membership+"/provisioning-status", nil, nil); code != http.StatusForbidden {
		t.Fatalf("member without providers.read = %d, want 403", code)
	}
}

// FLEET-10.13 / 13.2 / 39.2: every active managed provider has a provider
// sub-wallet. Consent requests the sub-wallet from payment-wallet-service;
// activation is refused until Wallet has confirmed it and every other gate
// passes, and succeeds (with the default decline-right policy and an
// activation event) once they all do.
func TestRow10_13_SubWalletIsMandatoryForActivation(t *testing.T) {
	f := newRows10Fixture(t)
	ctx := context.Background()
	membership := f.createProvider(t, "ada.wallet@example.com", "+2348031110002")
	if status := f.membershipStatus(t, membership); status != providermodels.StatusPendingProviderAcceptance {
		t.Fatalf("after create = %s, want pending provider acceptance", status)
	}

	// The provider consents while Wallet is unavailable: no sub-wallet, so
	// the membership must not become ACTIVE.
	f.wallet.setFailSubmissions(true)
	if err := f.service.RecordAssociationOutcome(ctx, f.org, membership, true, "corr-r10-consent"); err != nil {
		t.Fatal(err)
	}
	if status := f.membershipStatus(t, membership); status != providermodels.StatusPendingSetup {
		t.Fatalf("consented without a sub-wallet = %s, want PENDING_SETUP", status)
	}
	if f.wallet.callCount("POST /earning-accounts/ensure") == 0 {
		t.Fatal("consent never asked payment-wallet-service for the sub-wallet")
	}
	gates, ready, err := f.service.EvaluateActivation(ctx, f.org, membership)
	if err != nil {
		t.Fatal(err)
	}
	if ready || gateNamed(gates, "subwallet_active").Passed {
		t.Fatalf("activation ready=%v with an unconfirmed sub-wallet: %+v", ready, gates)
	}
	for _, name := range []string{"provider_identity_exists", "association_accepted", "membership_pending_setup", "home_branch_assigned", "organization_active", "provider_platform_status_active"} {
		if !gateNamed(gates, name).Passed {
			t.Fatalf("gate %s should pass: %+v", name, gates)
		}
	}
	var linkStatus string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM fleet_partner_provider_subwallet_links WHERE provider_membership_id = $1`, membership).Scan(&linkStatus); err != nil {
		t.Fatal(err)
	}
	if linkStatus == providermodels.SubWalletActive {
		t.Fatal("a failed Wallet call produced an ACTIVE sub-wallet link")
	}
	if activated, err := f.service.TryActivate(ctx, f.org, membership, "corr-r10-try"); err != nil || activated {
		t.Fatalf("TryActivate without a sub-wallet = %v, %v", activated, err)
	}
	code, body := rowsCall(t, f.router, "GET", "/fleet-partner/v1/providers/"+membership+"/provisioning-status", nil, nil)
	if code != http.StatusOK || rowsData(t, body)["subWalletStatus"] == providermodels.SubWalletActive {
		t.Fatalf("provisioning status after a failed sub-wallet = %d %v", code, body)
	}

	// Wallet recovers: the sub-wallet is confirmed and activation succeeds.
	f.wallet.setFailSubmissions(false)
	f.service.EnsureSubWallet(ctx, f.org, membership, "corr-r10-retry")
	activated, err := f.service.TryActivate(ctx, f.org, membership, "corr-r10-activate")
	if err != nil || !activated {
		t.Fatalf("TryActivate with every gate passed = %v, %v", activated, err)
	}
	if status := f.membershipStatus(t, membership); status != providermodels.StatusActive {
		t.Fatalf("after activation = %s, want ACTIVE", status)
	}
	var accountID string
	if err := f.pool.QueryRow(ctx, `SELECT status, COALESCE(wallet_account_id, '') FROM fleet_partner_provider_subwallet_links WHERE provider_membership_id = $1`, membership).Scan(&linkStatus, &accountID); err != nil {
		t.Fatal(err)
	}
	if linkStatus != providermodels.SubWalletActive || accountID != "fear_"+membership {
		t.Fatalf("sub-wallet link = %s/%s, want ACTIVE/fear_%s", linkStatus, accountID, membership)
	}
	sent := f.wallet.lastBody("POST /earning-accounts/ensure")
	if sent["organizationId"] != f.org || sent["providerReference"] != membership || sent["currency"] != "NGN" {
		t.Fatalf("sub-wallet request = %v", sent)
	}
	if n := rowsCount(t, f.pool, `SELECT count(*) FROM fleet_partner_provider_policies WHERE membership_id = $1 AND provider_decline_required`, membership); n != 1 {
		t.Fatalf("default policy with the decline right = %d rows, want 1", n)
	}
	if n := rowsCount(t, f.pool, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id = $1 AND event_type = 'organization.provider.activated.v1' AND aggregate_id = $2`, f.org, membership); n != 1 {
		t.Fatalf("activation events = %d, want 1", n)
	}
	if n := rowsCount(t, f.pool, `SELECT count(*) FROM fleet_partner_provider_membership_history WHERE membership_id = $1 AND to_status = 'ACTIVE' AND reason_code = 'all_activation_gates_passed'`, membership); n != 1 {
		t.Fatalf("activation history = %d, want 1", n)
	}
	// Activating again changes nothing.
	if again, err := f.service.TryActivate(ctx, f.org, membership, "corr-r10-again"); err != nil || again {
		t.Fatalf("second TryActivate = %v, %v; want no-op", again, err)
	}

	// Platform compliance still applies: a confirmed sub-wallet does not
	// activate a provider the owning service has suspended.
	blocked := f.createProvider(t, "bayo.blocked@example.com", "+2348031110003")
	f.authority.setPlatformStatus(rows10ProviderID("bayo.blocked@example.com"), "SUSPENDED")
	if err := f.service.RecordAssociationOutcome(ctx, f.org, blocked, true, "corr-r10-blocked"); err != nil {
		t.Fatal(err)
	}
	if status := f.membershipStatus(t, blocked); status != providermodels.StatusPendingSetup {
		t.Fatalf("platform-suspended provider = %s, want PENDING_SETUP", status)
	}
	gates, ready, _ = f.service.EvaluateActivation(ctx, f.org, blocked)
	if ready || gateNamed(gates, "provider_platform_status_active").Passed || !gateNamed(gates, "subwallet_active").Passed {
		t.Fatalf("platform-suspended gates = %+v", gates)
	}

	// An organization that is no longer ACTIVE activates nobody.
	f.authority.setPlatformStatus(rows10ProviderID("bayo.blocked@example.com"), "ACTIVE")
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_organizations SET status = 'SUSPENDED' WHERE id = $1`, f.org); err != nil {
		t.Fatal(err)
	}
	if activated, err := f.service.TryActivate(ctx, f.org, blocked, "corr-r10-suspended-org"); err != nil || activated {
		t.Fatalf("TryActivate in a suspended organization = %v, %v", activated, err)
	}
	gates, _, _ = f.service.EvaluateActivation(ctx, f.org, blocked)
	if gateNamed(gates, "organization_active").Passed {
		t.Fatalf("organization_active passed for a SUSPENDED organization: %+v", gates)
	}

	// Another organization cannot evaluate this membership.
	other := createOrganization(t, f.pool, "Rows10 Other Activation", "RC-R10-OTHER-"+randomHex()[:6])
	if _, _, err := f.service.EvaluateActivation(ctx, other.ID, membership); err == nil {
		t.Fatal("a membership was evaluated under another organization")
	}
}

const rows10CallerSecret = "rows10-execution-service-shared-secret"

// rows10InternalRouter is the production internal surface for the Fleet
// partner routes: the real composition root's FleetPartner service (with its
// real membership reader), behind HMAC service auth and the nonce guard.
func rows10InternalRouter(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.Config{AppEnv: "test", MaxRequestBodyBytes: 1 << 20,
		ProviderMembershipAllowedCallers: "driver-dispatch-delivery-service"}
	components := app.BuildComponents(cfg, pool, nil, observability.NewMetrics())
	return app.BuildRouter(app.Options{
		Config: cfg, Metrics: observability.NewMetrics(),
		TokenSigner: auth.NewTokenSigner([]byte("rows10-unused-user-token-secret")),
		Verifier: serviceauth.NewVerifier(serviceauth.Secrets{
			"driver-dispatch-delivery-service": []byte(rows10CallerSecret),
			"vendor-service":                   []byte(rows10CallerSecret),
		}, time.Minute),
		NonceStore:     noncestore.New(pool),
		InternalRoutes: components.InternalFleetPartnerRoutes(cfg),
	})
}

func rows10Signed(t *testing.T, router http.Handler, caller, path string, sign bool) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, bytes.NewReader(nil))
	if sign {
		if err := serviceauth.SignRequest(request, caller, []byte(rows10CallerSecret), nil, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	envelope := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	return recorder.Code, envelope
}

// FLEET-10.16b GET /internal/fleet-partner/v1/providers/{providerId}/
// corporate-context: an execution service reads, for an organization-managed
// provider, the membership, organization, operating mode and service
// eligibility -- derived from the authoritative rows, never from the caller.
func TestRow10_16b_CorporateContextForManagedProvider(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Rows10 Corporate Context Ltd", "RC-R10-CTX-1")
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organizations SET status = 'ACTIVE' WHERE id = $1`, org.ID); err != nil {
		t.Fatal(err)
	}
	if err := orgrepos.NewSettingsRepository(pool).Create(ctx, pool, org.ID, &orgmodels.Settings{OperatingMode: orgmodels.OperatingModeCentralized}); err != nil {
		t.Fatal(err)
	}
	branch := seedBranch(t, pool, org.ID, "Rows10 Ctx Hub")
	providerID := "prv_rows10_ctx_" + randomHex()[:8]
	membership := &providermodels.Membership{ProviderService: "driver-dispatch-delivery-service",
		ServiceCategory: providermodels.CategoryDispatch, OnboardingSource: providermodels.SourceSponsored,
		RelationshipType: providermodels.RelationshipEmployee, Status: providermodels.StatusActive,
		ProviderID: &providerID, HomeBranchID: &branch}
	if err := providerrepos.NewRepository(pool).CreateMembership(ctx, pool, org.ID, membership); err != nil {
		t.Fatal(err)
	}
	router := rows10InternalRouter(t, pool)
	path := "/internal/fleet-partner/v1/providers/" + providerID + "/corporate-context?service=DISPATCH"

	code, body := rows10Signed(t, router, "driver-dispatch-delivery-service", path, true)
	if code != http.StatusOK {
		t.Fatalf("corporate context = %d %v", code, body)
	}
	corporate := rowsData(t, body)
	if corporate["isManagedProvider"] != true || corporate["organizationId"] != org.ID || corporate["membershipId"] != membership.ID ||
		corporate["membershipStatus"] != providermodels.StatusActive || corporate["operatingMode"] != orgmodels.OperatingModeCentralized ||
		corporate["organizationRole"] != providermodels.RelationshipEmployee {
		t.Fatalf("managed context = %v", corporate)
	}
	if eligibility, _ := corporate["serviceEligibility"].(map[string]any); eligibility["DISPATCH"] != true {
		t.Fatalf("an ACTIVE dispatch membership is eligible for dispatch work: %v", corporate)
	}
	if _, ok := corporate["financialVisibilityPolicy"].(map[string]any); !ok {
		t.Fatalf("financial visibility policy missing: %v", corporate)
	}

	// The membership is for dispatch, not taxi.
	_, body = rows10Signed(t, router, "driver-dispatch-delivery-service",
		"/internal/fleet-partner/v1/providers/"+providerID+"/corporate-context?service=TAXI", true)
	if eligibility, _ := rowsData(t, body)["serviceEligibility"].(map[string]any); eligibility["TAXI"] == true {
		t.Fatalf("dispatch membership reported eligible for taxi: %v", body)
	}

	// Spec 10.3: organization suspension immediately removes every managed
	// provider from organization work, even though the membership row itself
	// is still ACTIVE.
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organizations SET status = 'SUSPENDED' WHERE id = $1`, org.ID); err != nil {
		t.Fatal(err)
	}
	_, body = rows10Signed(t, router, "driver-dispatch-delivery-service", path, true)
	suspended := rowsData(t, body)
	if eligibility, _ := suspended["serviceEligibility"].(map[string]any); eligibility["DISPATCH"] == true {
		t.Fatalf("a provider of a SUSPENDED organization is still eligible: %v", suspended)
	}
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organizations SET status = 'ACTIVE' WHERE id = $1`, org.ID); err != nil {
		t.Fatal(err)
	}

	// A suspended membership stays managed but is not eligible.
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organization_provider_memberships SET status = 'SUSPENDED' WHERE id = $1`, membership.ID); err != nil {
		t.Fatal(err)
	}
	_, body = rows10Signed(t, router, "driver-dispatch-delivery-service", path, true)
	paused := rowsData(t, body)
	if paused["isManagedProvider"] != true || paused["membershipStatus"] != providermodels.StatusSuspended {
		t.Fatalf("suspended membership context = %v", paused)
	}
	if eligibility, _ := paused["serviceEligibility"].(map[string]any); eligibility["DISPATCH"] == true {
		t.Fatalf("a SUSPENDED membership is eligible: %v", paused)
	}

	// An independent provider is not managed and leaks nothing.
	_, body = rows10Signed(t, router, "driver-dispatch-delivery-service",
		"/internal/fleet-partner/v1/providers/prv_independent_r10/corporate-context?service=DISPATCH", true)
	independent := rowsData(t, body)
	if independent["isManagedProvider"] != false || independent["organizationId"] != nil {
		t.Fatalf("independent context = %v", independent)
	}

	// Refusals: unsigned, a signed caller that is not allowlisted, and a
	// missing service parameter.
	if code, _ := rows10Signed(t, router, "driver-dispatch-delivery-service", path, false); code != http.StatusUnauthorized {
		t.Fatalf("unsigned = %d, want 401", code)
	}
	if code, _ := rows10Signed(t, router, "vendor-service", path, true); code != http.StatusForbidden {
		t.Fatalf("non-allowlisted caller = %d, want 403", code)
	}
	if code, _ := rows10Signed(t, router, "driver-dispatch-delivery-service",
		"/internal/fleet-partner/v1/providers/"+providerID+"/corporate-context", true); code != http.StatusBadRequest {
		t.Fatalf("missing service = %d, want 400", code)
	}
}

// FLEET-10.5 POST /providers/{membershipId}/suspend over HTTP: suspends an
// ACTIVE managed provider from company operations (audit, event, history),
// asks the owning provider service to revoke organization matching
// eligibility without touching the platform identity, and refuses stale
// versions, repeated suspension, missing permission and other tenants.
func TestRow10_5_SuspendProviderOverHTTP(t *testing.T) {
	f := newRows10Fixture(t)
	ctx := context.Background()
	membership := f.createProvider(t, "chidi.suspend@example.com", "+2348031110004")
	if err := f.service.RecordAssociationOutcome(ctx, f.org, membership, true, "corr-r105"); err != nil {
		t.Fatal(err)
	}
	if status := f.membershipStatus(t, membership); status != providermodels.StatusActive {
		t.Fatalf("seeded provider = %s, want ACTIVE", status)
	}
	var version int
	if err := f.pool.QueryRow(ctx, `SELECT version FROM fleet_partner_organization_provider_memberships WHERE id = $1`, membership).Scan(&version); err != nil {
		t.Fatal(err)
	}
	path := "/fleet-partner/v1/providers/" + membership + "/suspend"

	crm := seedApprovalMember(t, f.pool, f.org, "rol_system_crm", "r105-crm")
	if code, _ := rowsCall(t, providerAdminRouter(t, f.pool, f.service, f.org, crm, nil), "POST", path, map[string]any{}, map[string]string{"If-Match": itoaRows(version)}); code != http.StatusForbidden {
		t.Fatalf("suspend without providers.suspend = %d, want 403", code)
	}
	other := createOrganization(t, f.pool, "Rows105 Other Org", "RC-R105-"+randomHex()[:6])
	otherOwner := seedApprovalMember(t, f.pool, other.ID, staffmodels.SystemOwnerRoleID, "r105-other")
	if code, _ := rowsCall(t, providerAdminRouter(t, f.pool, f.service, other.ID, otherOwner, nil), "POST", path, map[string]any{}, map[string]string{"If-Match": itoaRows(version)}); code != http.StatusNotFound {
		t.Fatalf("cross-tenant suspend = %d, want 404", code)
	}
	if code, _ := rowsCall(t, f.router, "POST", path, map[string]any{}, map[string]string{"If-Match": itoaRows(version + 7)}); code != http.StatusConflict {
		t.Fatalf("stale suspend = %d, want 409", code)
	}
	code, body := rowsCall(t, f.router, "POST", path, map[string]any{}, map[string]string{"If-Match": itoaRows(version)})
	if code != http.StatusOK || rowsData(t, body)["status"] != providermodels.StatusSuspended {
		t.Fatalf("suspend = %d %v", code, body)
	}
	if status := f.membershipStatus(t, membership); status != providermodels.StatusSuspended {
		t.Fatalf("after suspend = %s", status)
	}
	if n := rowsCount(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id = $1 AND action = 'provider.status_changed' AND resource_id = $2`, f.org, membership); n != 1 {
		t.Fatalf("suspension audit rows = %d", n)
	}
	if n := rowsCount(t, f.pool, `SELECT count(*) FROM fleet_partner_provider_membership_history WHERE membership_id = $1 AND to_status = 'SUSPENDED'`, membership); n != 1 {
		t.Fatalf("suspension history rows = %d", n)
	}
	f.authority.mu.Lock()
	revoked := append([]string(nil), f.authority.revoked...)
	f.authority.mu.Unlock()
	if len(revoked) != 1 || revoked[0] != rows10ProviderID("chidi.suspend@example.com")+"|"+f.org {
		t.Fatalf("owning service eligibility revocations = %v", revoked)
	}
	// The provider identity is untouched: the membership still names it.
	var providerID string
	if err := f.pool.QueryRow(ctx, `SELECT provider_id FROM fleet_partner_organization_provider_memberships WHERE id = $1`, membership).Scan(&providerID); err != nil || providerID != rows10ProviderID("chidi.suspend@example.com") {
		t.Fatalf("provider identity after suspension = %q (%v)", providerID, err)
	}
	if code, _ := rowsCall(t, f.router, "POST", path, map[string]any{}, map[string]string{"If-Match": itoaRows(version + 1)}); code != http.StatusConflict {
		t.Fatalf("suspending a suspended provider = %d, want 409", code)
	}
	// The detail read reflects it.
	if code, body := rowsCall(t, f.router, "GET", "/fleet-partner/v1/providers/"+membership, nil, nil); code != http.StatusOK || rowsData(t, body)["status"] != providermodels.StatusSuspended {
		t.Fatalf("provider detail after suspension = %d %v", code, body)
	}
}
