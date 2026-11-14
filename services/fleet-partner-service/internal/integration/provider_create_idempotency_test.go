package integration_test

// Spec 6j: provisioning a provider requires a caller-supplied Idempotency-Key.
// POST /providers used to derive a key from the email and service category
// when the header was missing, which gave no replay protection and made two
// deliberate attempts look like one retry.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	orghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/http"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

func TestProviderProvisioningRequiresIdempotencyKey(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	owning := newFakeOwningService(t, platformclients.CategoryDispatch, platformclients.DispatchManagedProviderBase)
	client := platformclients.NewDispatchProviderClient(platformclients.Options{BaseURL: owning.server.URL,
		Secret: []byte(managedProviderSecret), Timeout: 5 * time.Second, Metrics: observability.NewMetrics()})
	organization := createOrganization(t, pool, "Provider Idempotency Co", "RC-PIK-"+uuid.NewString()[:6])
	seedEligibleOrganization(t, pool, organization.ID)
	service := providerusecases.NewService(providerusecases.Options{
		Pool: pool, Providers: providerrepos.NewRepository(pool), Organizations: providerProvisioningOrganizationStub{},
		Contracts: platformclients.ContractSet{
			Providers: map[string]platformclients.ProviderContract{platformclients.CategoryDispatch: client},
			Wallet:    activeSubWallet{},
		},
		Eligibility:        newEligibilityService(pool, healthyAuthority("", "")),
		Sender:             &authclients.FakeSender{},
		Events:             events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		InvitationSecret:   []byte("fleet-managed-provider-invitation-secret-32b"),
		DefaultCountryCode: "+234",
	})
	memberships := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1`, organization.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// The usecase refuses a missing key before creating anything.
	actor := tenant.Context{OrganizationID: organization.ID, OrganizationUserID: "oru_pik", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "oru_pik"}
	_, err := service.CreateProvider(ctx, providerusecases.CreateProviderInput{Tenant: actor, LegalName: "Ada Okafor",
		Email: "pik-usecase@example.test", Phone: "+2348039990001", ServiceCategory: platformclients.CategoryDispatch})
	if appStatus(err) != http.StatusBadRequest || memberships() != 0 {
		t.Fatalf("missing key: status %d, memberships %d; want 400 and nothing created", appStatus(err), memberships())
	}

	// Through the production handler with the shared idempotency store.
	owner := seedApprovalMember(t, pool, organization.ID, "rol_system_owner", "pikowner")
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{OrganizationID: organization.ID, OrganizationUserID: owner,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: owner})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	orghttp.NewHandler(nil, nil, service, accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0), nil).
		WithIdempotency(idempotency.NewStore(pool, 0)).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))

	body := map[string]any{"legalName": "Ada Okafor", "email": "pik-http@example.test", "phone": "+2348039990002",
		"serviceCategory": platformclients.CategoryDispatch, "relationshipType": "EMPLOYEE"}
	if status, response := gateCall(t, engine, http.MethodPost, "/fleet-partner/v1/providers", body, nil); status != http.StatusBadRequest {
		t.Fatalf("no Idempotency-Key: status %d %v, want 400", status, response)
	}
	if memberships() != 0 {
		t.Fatal("a refused request must create nothing")
	}
	key := map[string]string{"Idempotency-Key": "pik-" + uuid.NewString()}
	status, first := gateCall(t, engine, http.MethodPost, "/fleet-partner/v1/providers", body, key)
	if status != http.StatusCreated {
		t.Fatalf("create: status %d %v", status, first)
	}
	firstID, _ := first["data"].(map[string]any)["membershipId"].(string)
	status, replay := gateCall(t, engine, http.MethodPost, "/fleet-partner/v1/providers", body, key)
	replayID, _ := replay["data"].(map[string]any)["membershipId"].(string)
	if status != http.StatusCreated || replayID != firstID || firstID == "" {
		t.Fatalf("replay: status %d id %q, want the original %q", status, replayID, firstID)
	}
	if memberships() != 1 {
		t.Fatalf("memberships = %d after a replay, want 1", memberships())
	}
	var sagaKey string
	if err := pool.QueryRow(ctx, `SELECT idempotency_key FROM fleet_partner_provider_provisioning_sagas WHERE membership_id=$1`, firstID).Scan(&sagaKey); err != nil {
		t.Fatal(err)
	}
	if sagaKey != key["Idempotency-Key"] {
		t.Fatalf("saga key = %q, want the caller's key", sagaKey)
	}
	body["email"] = "pik-other@example.test"
	if status, _ := gateCall(t, engine, http.MethodPost, "/fleet-partner/v1/providers", body, key); status != http.StatusConflict {
		t.Fatalf("same key, different body: status %d, want 409", status)
	}
}
