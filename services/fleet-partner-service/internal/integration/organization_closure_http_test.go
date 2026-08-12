package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	orghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/http"
	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	staffusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/usecases"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// closureRouter mounts the real organization routes (including the four
// closure-workflow steps this session added: assess/approve/reject/execute)
// behind an injected tenant context and the real, database-backed
// authorizer + session repository -- proving the whole wire, not just that
// the handler functions compile.
func closureRouter(t *testing.T, pool *pgxpool.Pool, orgID, organizationUserID string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)

	organizations := orgusecases.NewService(orgusecases.Options{
		Pool:          pool,
		Organizations: orgrepos.NewOrganizationRepository(pool),
		Onboarding:    orgrepos.NewOnboardingRepository(pool),
		Branches:      orgrepos.NewBranchRepository(pool),
		Settings:      orgrepos.NewSettingsRepository(pool),
		Events:        events.NewDomainPublisher(events.NewOutboxRepository(pool)),
	})
	providers := providerusecases.NewService(providerusecases.Options{
		Pool:      pool,
		Providers: providerrepos.NewRepository(pool),
	})
	sessions := authrepos.NewSessionRepository(pool)
	var staffService *staffusecases.Service
	handler := orghttp.NewHandler(organizations, staffService, providers,
		accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0), sessions)

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

func postClosureStep(t *testing.T, router http.Handler, path string, body map[string]any) *httptest.ResponseRecorder {
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
	req := httptest.NewRequest(http.MethodPost, path, reader)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func getClosureStatus(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

// TestOrganizationClosure_FullLifecycleReachesCompleted is a regression
// guard for a real, previously-undiscovered gap this session found while
// investigating whether "organization closure orchestration" already
// existed: RequestClosure, AssessClosure and ExecuteClosure were all
// implemented, but nothing in the entire service ever moved a request from
// APPROVAL_REQUIRED to APPROVED -- ExecuteClosure's own precondition
// (status == APPROVED) could therefore never be satisfied by any reachable
// code path, and the workflow was permanently stuck one step before
// completion. This test drives the whole state machine end to end through
// the real HTTP routes and proves it now actually reaches COMPLETED:
// REQUESTED -> (assess, no live blockers) -> APPROVAL_REQUIRED -> (approve)
// -> APPROVED -> (execute) -> COMPLETED, with the organization itself
// marked CLOSED as a side effect.
func TestOrganizationClosure_FullLifecycleReachesCompleted(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Closure Lifecycle Co", "RC-CLOSE-1")
	userID := seedAuditRouteUser(t, pool, org.ID, "rol_system_owner")
	router := closureRouter(t, pool, org.ID, userID)

	base := "/fleet-partner/v1/organizations/" + org.ID

	// 1. Request closure.
	resp := postClosureStep(t, router, base+"/close-request", map[string]any{"reasonCode": "test_wind_down"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("close-request status = %d, body = %s", resp.Code, resp.Body.String())
	}

	// 2. Assess -- DefaultClosureCheckers() are all UNKNOWN-by-default
	// (fail-closed) except the real provider-membership count, which is
	// zero for a brand-new organization with no providers, so this
	// organization has real, expected blockers from every service-owned
	// checker whose contract does not exist yet. That is fine: this test
	// proves the STATE MACHINE now reaches COMPLETED, not that a real
	// organization can always close immediately -- assert on the shape of
	// the response, then drive past BLOCKED using a second, purpose-built
	// assessment (see below) so the test does not depend on those external
	// contracts ever existing.
	resp = postClosureStep(t, router, base+"/closure/assess", map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("assess status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var assessed struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &assessed); err != nil {
		t.Fatalf("decode assess: %v", err)
	}
	if assessed.Data.Status != orgmodels.ClosureStatusBlocked {
		t.Fatalf("status after assess = %q, want BLOCKED (DefaultClosureCheckers are UNKNOWN by default)", assessed.Data.Status)
	}

	// Approve must be refused while BLOCKED -- approval can never itself
	// clear a blocker.
	resp = postClosureStep(t, router, base+"/closure/approve", map[string]any{})
	if resp.Code != http.StatusConflict {
		t.Fatalf("approve-while-blocked status = %d, want 409, body = %s", resp.Code, resp.Body.String())
	}

	// Directly re-run assessment with an empty checker set (simulating every
	// external obligation having cleared) to reach APPROVAL_REQUIRED, the
	// state ApproveClosure actually requires. This exercises the usecase
	// layer directly rather than fabricating a second HTTP route only for
	// the test.
	organizations := orgusecases.NewService(orgusecases.Options{
		Pool:          pool,
		Organizations: orgrepos.NewOrganizationRepository(pool),
		Onboarding:    orgrepos.NewOnboardingRepository(pool),
		Branches:      orgrepos.NewBranchRepository(pool),
		Settings:      orgrepos.NewSettingsRepository(pool),
		Events:        events.NewDomainPublisher(events.NewOutboxRepository(pool)),
	})
	if _, blockers, err := organizations.AssessClosure(context.Background(), org.ID, nil, nil); err != nil {
		t.Fatalf("re-assess with no checkers: %v", err)
	} else if len(blockers) != 0 {
		t.Fatalf("expected zero blockers with an empty checker set, got %+v", blockers)
	}

	resp = getClosureStatus(t, router, base+"/closure-status")
	if resp.Code != http.StatusOK {
		t.Fatalf("closure-status status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var status struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Data.Status != orgmodels.ClosureStatusApprovalRequired {
		t.Fatalf("status after clean re-assess = %q, want APPROVAL_REQUIRED", status.Data.Status)
	}

	// 3. Approve -- this is the exact transition that did not exist before
	// this session: APPROVAL_REQUIRED -> APPROVED.
	resp = postClosureStep(t, router, base+"/closure/approve", map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var approved struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &approved); err != nil {
		t.Fatalf("decode approve: %v", err)
	}
	if approved.Data.Status != orgmodels.ClosureStatusApproved {
		t.Fatalf("status after approve = %q, want APPROVED", approved.Data.Status)
	}

	// 4. Execute -- now satisfiable for the first time, since a request can
	// finally reach APPROVED through a real code path.
	resp = postClosureStep(t, router, base+"/closure/execute", map[string]any{})
	if resp.Code != http.StatusOK {
		t.Fatalf("execute status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var executed struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &executed); err != nil {
		t.Fatalf("decode execute: %v", err)
	}
	if executed.Data.Status != orgmodels.ClosureStatusCompleted {
		t.Fatalf("status after execute = %q, want COMPLETED", executed.Data.Status)
	}

	// The organization itself must be marked CLOSED as a side effect.
	stored, err := orgrepos.NewOrganizationRepository(pool).GetByID(context.Background(), org.ID)
	if err != nil {
		t.Fatalf("reload organization: %v", err)
	}
	if stored.Status != orgmodels.OrganizationStatusClosed {
		t.Fatalf("organization status = %q, want CLOSED", stored.Status)
	}
}

// TestOrganizationClosure_RejectStopsAnInFlightRequest proves a request can
// be called off at any point before execution actually starts, and that a
// rejected request no longer counts as "open" -- a fresh close-request can
// be filed afterward.
func TestOrganizationClosure_RejectStopsAnInFlightRequest(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Closure Reject Co", "RC-CLOSE-2")
	userID := seedAuditRouteUser(t, pool, org.ID, "rol_system_owner")
	router := closureRouter(t, pool, org.ID, userID)
	base := "/fleet-partner/v1/organizations/" + org.ID

	resp := postClosureStep(t, router, base+"/close-request", map[string]any{"reasonCode": "test_reject"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("close-request status = %d, body = %s", resp.Code, resp.Body.String())
	}

	resp = postClosureStep(t, router, base+"/closure/reject", map[string]any{"reasonText": "changed our mind"})
	if resp.Code != http.StatusOK {
		t.Fatalf("reject status = %d, body = %s", resp.Code, resp.Body.String())
	}
	var rejected struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &rejected); err != nil {
		t.Fatalf("decode reject: %v", err)
	}
	if rejected.Data.Status != orgmodels.ClosureStatusRejected {
		t.Fatalf("status after reject = %q, want REJECTED", rejected.Data.Status)
	}

	// A rejected request is no longer "open" -- closure-status must now
	// 404/error rather than surface the rejected request as if it were live.
	resp = getClosureStatus(t, router, base+"/closure-status")
	if resp.Code == http.StatusOK {
		t.Fatalf("closure-status after reject = 200, want an error (no open request), body = %s", resp.Body.String())
	}

	// A fresh request can be filed.
	resp = postClosureStep(t, router, base+"/close-request", map[string]any{"reasonCode": "test_retry"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("second close-request status = %d, body = %s", resp.Code, resp.Body.String())
	}
}

// TestOrganizationClosure_DeniesWithoutPermission proves a member with no
// role binding cannot drive any closure-workflow step, including the three
// new ones this session added.
func TestOrganizationClosure_DeniesWithoutPermission(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Closure Deny Co", "RC-CLOSE-3")
	userID := seedAuditRouteUser(t, pool, org.ID, "")
	router := closureRouter(t, pool, org.ID, userID)
	base := "/fleet-partner/v1/organizations/" + org.ID

	for _, path := range []string{
		base + "/close-request", base + "/closure/assess",
		base + "/closure/approve", base + "/closure/reject", base + "/closure/execute",
	} {
		resp := postClosureStep(t, router, path, map[string]any{})
		if resp.Code != http.StatusForbidden {
			t.Fatalf("%s status = %d, want 403, body = %s", path, resp.Code, resp.Body.String())
		}
	}
}
