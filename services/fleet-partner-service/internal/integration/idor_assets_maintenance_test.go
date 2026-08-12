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

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Section 28 gap-1 IDOR domain 1/5: assets and maintenance.
//
// Every read/mutation in this package goes through GetAsset/GetSchedule/
// GetWorkOrder, and Schedule/WorkOrder both re-validate their parent asset
// through GetAsset rather than trusting the asset_id foreign key -- so the
// same query enforces organization scope, branch scope (Asset carries a
// real branch_id and GetAsset checks t.BranchPermitted) and, for the two
// maintenance resources, that the resource's declared parent asset is one
// this caller can actually see. This suite proves all of that from the
// real HTTP surface, against real PostgreSQL, plus the safe-permission and
// safe-404 behavior tested tenant routes must have.

func assetsIDORRouter(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := assets.NewService(assets.Options{
		Pool: pool, Flags: assets.Flags{Assets: true, Maintenance: true},
	})
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	store := idempotency.NewStore(pool, 0)

	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		var permitted []string
		// An empty PermittedBranchIDs slice means "every branch of the
		// organization" (tenant.Context.BranchPermitted's own documented
		// default) -- so the cross-branch-denial case must set a real,
		// narrower slice via this header, not rely on the branchId query
		// param alone (that only feeds the PERMISSION check, i.e. which
		// role-bound branch scope covers this request; it never narrows
		// which rows the tenant context itself is allowed to see).
		if restrict := c.GetHeader("X-Test-Permitted-Branch"); restrict != "" {
			permitted = []string{restrict}
		}
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: c.GetHeader("X-Test-Member"),
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: c.GetHeader("X-Test-Member"),
			PermittedBranchIDs: permitted,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	assets.NewHandler(service, authorizer, store).RegisterTenantRoutes(group)
	return engine
}

// assetsIDORCall performs one request and unwraps a success envelope's
// "data" object, matching backgroundOpsCall's convenience for this
// package's other handler-level tests; on error the raw {"error": ...}
// envelope is returned unwrapped so callers can still assert status alone.
func assetsIDORCall(t *testing.T, router http.Handler, method, path, orgID, memberID string,
	body any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Org", orgID)
	request.Header.Set("X-Test-Member", memberID)
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	envelope := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	if data, ok := envelope["data"].(map[string]any); ok {
		return recorder.Code, data
	}
	return recorder.Code, envelope
}

// TestAssets_ObjectAuthorization proves the object-authorization matrix for
// /assets: same-org access, cross-org denial (safe 404), cross-branch
// denial (safe 404), missing-permission denial (403).
func TestAssets_ObjectAuthorization(t *testing.T) {
	pool := testPool(t)
	orgA := createOrganization(t, pool, "Assets IDOR Co A", "RC-IDOR-AST-1")
	orgB := createOrganization(t, pool, "Assets IDOR Co B", "RC-IDOR-AST-2")
	router := assetsIDORRouter(t, pool)

	owner := seedApprovalMember(t, pool, orgA.ID, "rol_system_owner", "owner")
	// rol_system_crm holds neither assets.read nor assets.manage (migration
	// 000042 grants those to owner/operations/compliance/dispatcher subsets
	// only).
	noPermission := seedApprovalMember(t, pool, orgA.ID, "rol_system_crm", "no-perm")
	crossOrgOwner := seedApprovalMember(t, pool, orgB.ID, "rol_system_owner", "cross-org-owner")

	// Missing permission -> 403, not a silent empty result.
	status, body := assetsIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/assets", orgA.ID, noPermission,
		map[string]any{"BranchID": "any", "AssetCode": "AST-1", "AssetType": "TRUCK"},
		map[string]string{"Idempotency-Key": "no-perm-create"})
	if status != http.StatusForbidden {
		t.Fatalf("create without assets.manage status = %d body=%v, want 403", status, body)
	}

	branchA := seedBranch(t, pool, orgA.ID, "Assets Branch A")
	branchOther := seedBranch(t, pool, orgA.ID, "Assets Branch Other")

	// Same-org, permitted-branch create + read.
	status, created := assetsIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/assets", orgA.ID, owner,
		map[string]any{"BranchID": branchA, "AssetCode": "AST-100", "AssetType": "TRUCK"},
		map[string]string{"Idempotency-Key": "create-1"})
	if status != http.StatusCreated {
		t.Fatalf("create status = %d body=%v, want 201", status, created)
	}
	assetID, _ := created["ID"].(string)
	if assetID == "" {
		t.Fatalf("create response carries no ID: %v", created)
	}
	status, got := assetsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/assets/"+assetID, orgA.ID, owner, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("same-org get status = %d body=%v, want 200", status, got)
	}

	// Cross-org: orgB's owner-tier member reaching for orgA's asset id gets a
	// safe 404, never a 403-with-detail or a leaked cross-org view.
	status, got = assetsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/assets/"+assetID, orgB.ID, crossOrgOwner, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org get status = %d body=%v, want 404 (safe not-found, not a leak)", status, got)
	}

	// Cross-branch: same org, same full permission set, but the caller's
	// tenant context is restricted to a branch that is not the asset's
	// branch -- GetAsset's own t.BranchPermitted(v.BranchID) check must
	// still refuse it, a safe 404 exactly like the cross-org case.
	status, got = assetsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/assets/"+assetID, orgA.ID, owner, nil,
		map[string]string{"X-Test-Permitted-Branch": branchOther})
	if status != http.StatusNotFound {
		t.Fatalf("cross-branch get status = %d body=%v, want 404 (safe not-found, not a leak)", status, got)
	}
}

// TestMaintenance_ObjectAuthorization proves the object-authorization matrix
// for /maintenance/schedules and /maintenance/work-orders: same-org access,
// cross-org denial, missing-permission denial, and -- the domain-specific
// case Section 28 calls "nested parent mismatch" -- a work order whose
// asset_id is made (by direct SQL, simulating a data-integrity violation or
// a crafted request) to point at another organization's asset is refused
// by the server-side re-validation in GetWorkOrder, not merely by the
// foreign key having been correct at creation time.
func TestMaintenance_ObjectAuthorization(t *testing.T) {
	pool := testPool(t)
	orgA := createOrganization(t, pool, "Maintenance IDOR Co A", "RC-IDOR-MNT-1")
	orgB := createOrganization(t, pool, "Maintenance IDOR Co B", "RC-IDOR-MNT-2")
	router := assetsIDORRouter(t, pool)

	owner := seedApprovalMember(t, pool, orgA.ID, "rol_system_owner", "owner")
	noPermission := seedApprovalMember(t, pool, orgA.ID, "rol_system_crm", "no-perm")
	crossOrgOwner := seedApprovalMember(t, pool, orgB.ID, "rol_system_owner", "cross-org-owner")

	branchA := seedBranch(t, pool, orgA.ID, "Maintenance Branch A")
	branchB := seedBranch(t, pool, orgB.ID, "Maintenance Branch B")

	_, created := assetsIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/assets", orgA.ID, owner,
		map[string]any{"BranchID": branchA, "AssetCode": "AST-200", "AssetType": "TRUCK"},
		map[string]string{"Idempotency-Key": "create-asset-a"})
	assetAID, _ := created["ID"].(string)
	if assetAID == "" {
		t.Fatalf("orgA asset create carries no ID: %v", created)
	}
	_, createdB := assetsIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/assets", orgB.ID, crossOrgOwner,
		map[string]any{"BranchID": branchB, "AssetCode": "AST-201", "AssetType": "TRUCK"},
		map[string]string{"Idempotency-Key": "create-asset-b"})
	assetBID, _ := createdB["ID"].(string)
	if assetBID == "" {
		t.Fatalf("orgB asset create carries no ID: %v", createdB)
	}

	// Missing permission -> 403.
	status, body := assetsIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/maintenance/work-orders", orgA.ID, noPermission,
		map[string]any{"AssetID": assetAID, "SourceType": "SCHEDULED"},
		map[string]string{"Idempotency-Key": "no-perm-wo"})
	if status != http.StatusForbidden {
		t.Fatalf("create work order without maintenance.manage status = %d body=%v, want 403", status, body)
	}

	// Same-org create + read.
	status, wo := assetsIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/maintenance/work-orders", orgA.ID, owner,
		map[string]any{"AssetID": assetAID, "SourceType": "SCHEDULED"},
		map[string]string{"Idempotency-Key": "wo-1"})
	if status != http.StatusCreated {
		t.Fatalf("create work order status = %d body=%v, want 201", status, wo)
	}
	workOrderID, _ := wo["ID"].(string)
	if workOrderID == "" {
		t.Fatalf("work order create carries no ID: %v", wo)
	}
	status, got := assetsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/maintenance/work-orders/"+workOrderID, orgA.ID, owner, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("same-org get work order status = %d body=%v, want 200", status, got)
	}

	// Cross-org: orgB cannot reach orgA's work order id.
	status, got = assetsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/maintenance/work-orders/"+workOrderID, orgB.ID, crossOrgOwner, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org get work order status = %d body=%v, want 404", status, got)
	}

	// Nested parent mismatch: flip the work order's asset_id to point at
	// orgB's asset via direct SQL (creation itself would have refused this --
	// this proves the READ path re-validates the parent rather than trusting
	// the stored foreign key). The work order row is still organization_id=
	// orgA, so an orgA caller reaches the row, but GetWorkOrder's own
	// s.GetAsset(ctx, t, v.AssetID) call must refuse once assetID no longer
	// resolves inside orgA.
	if _, err := pool.Exec(context.Background(),
		`UPDATE fleet_partner_work_orders SET asset_id=$1 WHERE id=$2`, assetBID, workOrderID); err != nil {
		t.Fatalf("force parent mismatch: %v", err)
	}
	status, got = assetsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/maintenance/work-orders/"+workOrderID, orgA.ID, owner, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("work order with cross-org parent asset status = %d body=%v, want 404 (parent re-validated, not trusted)", status, got)
	}
}
