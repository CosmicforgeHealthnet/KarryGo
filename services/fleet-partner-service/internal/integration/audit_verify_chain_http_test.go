package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	audithttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/http"
	auditrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/repositories"
	authmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// seedAuditRouteUser creates a corporate member and, when roleID is
// non-empty, grants it a seeded system role through the real binding
// repository -- the same authority path production goes through, not a raw
// INSERT, mirroring seedOrganizationUser in safety_reference_http_test.go
// (kept separate rather than shared: coupling this file to safetyFixture
// would pull in an unrelated fixture just for one helper).
func seedAuditRouteUser(t *testing.T, pool *pgxpool.Pool, orgID string, roleID string) string {
	t.Helper()
	ctx := context.Background()
	members := staffrepos.NewMemberRepository(pool)
	roles := staffrepos.NewRoleRepository(pool)

	identity := &authmodels.Identity{
		EmailNormalized: "audit-chain-" + ids.New("id") + "@example.com",
		EmailDisplay:    "audit-chain@example.com",
		PhoneE164:       "+234801" + ids.New("p")[len(ids.New("p"))-7:],
		PhoneDisplay:    "08010000000",
		Status:          authmodels.IdentityStatusActive,
	}
	member := &staffmodels.OrganizationUser{Status: staffmodels.UserStatusActive}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := authrepos.NewIdentityRepository(pool).Create(ctx, tx, identity); err != nil {
			return err
		}
		member.IdentityID = identity.ID
		if err := members.Create(ctx, tx, orgID, member); err != nil {
			return err
		}
		if roleID == "" {
			return nil
		}
		return roles.CreateBinding(ctx, tx, orgID, &staffmodels.RoleBinding{
			OrganizationUserID: member.ID,
			RoleID:             roleID,
			ScopeType:          staffmodels.ScopeOrganization,
			GrantedBy:          "test",
		})
	}); err != nil {
		t.Fatalf("seed organization user: %v", err)
	}
	return member.ID
}

// auditVerifyChainRouter mounts the real audit routes behind an injected
// tenant context and the real, database-backed authorizer -- proving the
// route enforces security.audit_chain.verify for real, not just that the
// handler function exists.
func auditVerifyChainRouter(t *testing.T, pool *pgxpool.Pool, orgID, organizationUserID string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := audithttp.NewHandler(auditrepos.NewRepository(pool),
		accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0))

	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID:     orgID,
			OrganizationUserID: organizationUserID,
			ActorType:          tenant.ActorTypeOrganizationUser,
			ActorID:            organizationUserID,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	handler.RegisterTenantRoutes(group)
	return engine
}

func postVerifyChain(t *testing.T, router http.Handler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(http.MethodPost, "/fleet-partner/v1/audit/verify-chain", reader)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

// TestAuditVerifyChainRoute_HappyPathThroughRealRouter proves the whole
// wire: real gin router -> real accesscontrol.Authorizer (DB-backed
// rol_system_owner binding, granted security.audit_chain.verify by
// migration 000069) -> repositories.Repository.VerifyChain ->
// platform/audit.VerifyChain, for entries actually written through
// audit.Recorder.Record against the bare pool (the exact call shape this
// session's Record() fix targets).
func TestAuditVerifyChainRoute_HappyPathThroughRealRouter(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Audit Chain Route Co", "RC-ACHR-1")
	userID := seedAuditRouteUser(t, pool, org.ID, "rol_system_owner")

	recorder := audit.NewRecorder()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := recorder.Record(ctx, pool, audit.Entry{
			OrganizationID: org.ID, ActorType: "organization_user", ActorID: userID,
			Action: "audit_chain_route_test.recorded", ResourceType: "test_resource",
			ResourceID: ids.New("res"),
		}); err != nil {
			t.Fatalf("record entry %d: %v", i, err)
		}
	}

	router := auditVerifyChainRouter(t, pool, org.ID, userID)
	resp := postVerifyChain(t, router, map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}

	var payload struct {
		Data audithttp.VerifyChainView `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !payload.Data.Valid {
		t.Fatalf("expected a valid chain, got breaks: %+v", payload.Data.Breaks)
	}
	if payload.Data.EntriesChecked < 3 {
		t.Fatalf("entriesChecked = %d, want at least 3", payload.Data.EntriesChecked)
	}
	if payload.Data.Month != time.Now().UTC().Format("2006-01") {
		t.Fatalf("month = %q, want current UTC month", payload.Data.Month)
	}
}

// TestAuditVerifyChainRoute_DeniesWithoutPermission proves a member with no
// role binding (so no security.audit_chain.verify grant) is refused, even
// though they belong to the same organization whose chain they are asking
// about.
func TestAuditVerifyChainRoute_DeniesWithoutPermission(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Audit Chain Route Deny Co", "RC-ACHR-2")
	userID := seedAuditRouteUser(t, pool, org.ID, "")

	router := auditVerifyChainRouter(t, pool, org.ID, userID)
	resp := postVerifyChain(t, router, map[string]any{})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body = %s", resp.Code, resp.Body.String())
	}
}

// TestAuditVerifyChainRoute_RejectsMalformedMonth proves a caller-supplied
// month that is not YYYY-MM is refused with 400, never silently coerced into
// some other partition.
func TestAuditVerifyChainRoute_RejectsMalformedMonth(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Audit Chain Route Bad Month Co", "RC-ACHR-3")
	userID := seedAuditRouteUser(t, pool, org.ID, "rol_system_owner")

	router := auditVerifyChainRouter(t, pool, org.ID, userID)
	resp := postVerifyChain(t, router, map[string]any{"month": "not-a-month"})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", resp.Code, resp.Body.String())
	}
}

// TestAuditVerifyChainRoute_ScopedToCallersOwnOrganization proves that an
// organization's verify-chain call reports on ITS OWN chain only: entries
// recorded for a different organization in the same calendar month never
// appear in this organization's EntriesChecked count or break report -- the
// route can never be used to probe another tenant's audit trail.
func TestAuditVerifyChainRoute_ScopedToCallersOwnOrganization(t *testing.T) {
	pool := testPool(t)
	orgA := createOrganization(t, pool, "Audit Chain Route Org A", "RC-ACHR-4A")
	orgB := createOrganization(t, pool, "Audit Chain Route Org B", "RC-ACHR-4B")
	userA := seedAuditRouteUser(t, pool, orgA.ID, "rol_system_owner")
	userB := seedAuditRouteUser(t, pool, orgB.ID, "rol_system_owner")

	recorder := audit.NewRecorder()
	ctx := context.Background()
	if err := recorder.Record(ctx, pool, audit.Entry{
		OrganizationID: orgA.ID, ActorType: "organization_user", ActorID: userA,
		Action: "audit_chain_route_test.org_a", ResourceType: "test_resource", ResourceID: ids.New("res"),
	}); err != nil {
		t.Fatalf("record org A entry: %v", err)
	}

	routerB := auditVerifyChainRouter(t, pool, orgB.ID, userB)
	resp := postVerifyChain(t, routerB, map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Data audithttp.VerifyChainView `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Data.EntriesChecked != 0 {
		t.Fatalf("org B saw %d entries; org A's entry must not be visible", payload.Data.EntriesChecked)
	}
	if !payload.Data.Valid {
		t.Fatalf("expected an empty-but-valid chain for org B, got breaks: %+v", payload.Data.Breaks)
	}
}
