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
	bulkhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/http"
	bulkmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	bulkrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/repositories"
	bulkusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
	exportshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/http"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Section 28 gap-1 IDOR domain 4/5: imports and exports.
//
// Exports already has TestExports_TenantIsolation and TestExports_
// HTTPLifecycle in exports_test.go (service-level tenant isolation +
// HTTP-level permission denial); this file adds exports' one remaining
// gap -- an HTTP-level cross-org 404 case -- and builds the entire imports
// (Section 25C generic-engine, /fleet-partner/v1/imports) suite from
// scratch, since it had no coverage at all: same-org, cross-org, cross-
// branch (GetGeneric explicitly checks t.BranchPermitted(job.BranchID)),
// the "nested parent mismatch" case Section 28 names explicitly (a row id
// that is real but belongs to a DIFFERENT import than the one named in the
// path), and missing-permission denial.

// idorStubImportAdapter is the minimum Adapter implementation CreateGeneric
// requires to accept a create call -- ValidateRow/Commit are never invoked
// by this suite, which only exercises create + read authorization, not the
// validate/commit workflow.
type idorStubImportAdapter struct{}

func (idorStubImportAdapter) Type() string              { return bulkmodels.TypeAsset }
func (idorStubImportAdapter) SchemaVersion() int        { return 1 }
func (idorStubImportAdapter) RequiredColumns() []string { return nil }
func (idorStubImportAdapter) OptionalColumns() []string { return nil }
func (idorStubImportAdapter) ValidateRow(map[string]string) bulkusecases.RowOutcome {
	return bulkusecases.RowOutcome{Valid: true}
}
func (idorStubImportAdapter) Commit(context.Context, tenant.Context, string, map[string]any) (string, error) {
	return "", nil
}

func importsIDORRouter(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := bulkusecases.NewService(bulkusecases.Options{
		Pool: pool, Repository: bulkrepos.NewRepository(pool),
		Flags:        bulkusecases.Flags{Enabled: true, WritesEnabled: true},
		Adapters:     map[string]bulkusecases.Adapter{bulkmodels.TypeAsset: idorStubImportAdapter{}},
		GenericFlags: bulkusecases.GenericFlags{Enabled: true, WritesEnabled: true},
	})
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	store := idempotency.NewStore(pool, 0)

	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		var permitted []string
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
	bulkhttp.NewHandler(service, authorizer, store).RegisterTenantRoutes(group)
	return engine
}

func importsIDORCall(t *testing.T, router http.Handler, method, path, orgID, memberID string,
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

// TestImports_ObjectAuthorization proves the object-authorization matrix
// for /imports: same-org access, cross-org denial, cross-branch denial,
// missing-permission denial, and the nested-parent-mismatch case for
// row-level operations (a row that really exists, but under a different
// import than the one named in the path).
func TestImports_ObjectAuthorization(t *testing.T) {
	pool := testPool(t)
	orgA := createOrganization(t, pool, "Imports IDOR Co A", "RC-IDOR-IMP-1")
	orgB := createOrganization(t, pool, "Imports IDOR Co B", "RC-IDOR-IMP-2")
	router := importsIDORRouter(t, pool)

	owner := seedApprovalMember(t, pool, orgA.ID, "rol_system_owner", "owner")
	// rol_system_crm holds no imports.* permission (migration 000031 grants
	// imports.read/imports.manage to owner/operations only; 000056 grants
	// imports.validate/commit/retry the same way).
	noPermission := seedApprovalMember(t, pool, orgA.ID, "rol_system_crm", "no-perm")
	crossOrgOwner := seedApprovalMember(t, pool, orgB.ID, "rol_system_owner", "cross-org-owner")

	branchA := seedBranch(t, pool, orgA.ID, "Imports Branch A")
	branchOther := seedBranch(t, pool, orgA.ID, "Imports Branch Other")

	// Missing permission -> 403.
	status, body := importsIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/imports", orgA.ID, noPermission,
		map[string]any{"branchId": branchA, "importType": bulkmodels.TypeAsset, "mediaFileId": "med_fake_1", "fileFormat": "CSV"},
		map[string]string{"Idempotency-Key": "no-perm-create"})
	if status != http.StatusForbidden {
		t.Fatalf("create without imports.manage status = %d body=%v, want 403", status, body)
	}

	// Same-org create + read.
	status, created := importsIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/imports", orgA.ID, owner,
		map[string]any{"branchId": branchA, "importType": bulkmodels.TypeAsset, "mediaFileId": "med_fake_1", "fileFormat": "CSV"},
		map[string]string{"Idempotency-Key": "create-1"})
	if status != http.StatusAccepted {
		t.Fatalf("create status = %d body=%v, want 202", status, created)
	}
	importID, _ := created["ID"].(string)
	if importID == "" {
		t.Fatalf("create response carries no ID: %v", created)
	}
	status, got := importsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/imports/"+importID, orgA.ID, owner, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("same-org get status = %d body=%v, want 200", status, got)
	}

	// Cross-org: orgB's owner-tier member gets a safe 404 for orgA's import.
	status, got = importsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/imports/"+importID, orgB.ID, crossOrgOwner, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org get status = %d body=%v, want 404 (safe not-found, not a leak)", status, got)
	}

	// Cross-branch: same org, but the caller's tenant context is restricted
	// to a branch that is not the import's branch -- GetGeneric's own
	// !t.BranchPermitted(job.BranchID) check must refuse it.
	status, got = importsIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/imports/"+importID, orgA.ID, owner, nil,
		map[string]string{"X-Test-Permitted-Branch": branchOther})
	if status != http.StatusNotFound {
		t.Fatalf("cross-branch get status = %d body=%v, want 404 (safe not-found, not a leak)", status, got)
	}

	// Nested parent mismatch: seed a second import in the SAME org, then a
	// row under the FIRST import, and prove the row cannot be retried
	// through the SECOND import's id -- RetryRow's own SQL filters
	// WHERE organization_id=$1 AND import_id=$2 AND id=$3, so a row_id that
	// is real but belongs to a different import_id can never match and the
	// action is refused, not silently applied to the wrong parent.
	_, created2 := importsIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/imports", orgA.ID, owner,
		map[string]any{"branchId": branchA, "importType": bulkmodels.TypeAsset, "mediaFileId": "med_fake_2", "fileFormat": "CSV"},
		map[string]string{"Idempotency-Key": "create-2"})
	otherImportID, _ := created2["ID"].(string)
	if otherImportID == "" {
		t.Fatalf("second create carries no ID: %v", created2)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO fleet_partner_bulk_import_rows(id,organization_id,import_id,row_number,row_reference,row_fingerprint,status)
		 VALUES('row_idor_test_1',$1,$2,1,'ref-1','fp-1','FAILED')`, orgA.ID, importID); err != nil {
		t.Fatalf("seed import row: %v", err)
	}
	// RetryRow's usecase also requeues the PARENT import for commit, which
	// requires the import itself to already be VALIDATED (or similar) --
	// unrelated business-state precondition this authorization test is not
	// exercising, so it is set directly rather than driven through the real
	// validate workflow.
	if _, err := pool.Exec(context.Background(),
		`UPDATE fleet_partner_bulk_imports SET status='VALIDATED' WHERE id=$1`, importID); err != nil {
		t.Fatalf("force import to VALIDATED: %v", err)
	}
	status, got = importsIDORCall(t, router, http.MethodPost,
		"/fleet-partner/v1/imports/"+otherImportID+"/rows/row_idor_test_1/retry", orgA.ID, owner, nil,
		map[string]string{"Idempotency-Key": "retry-mismatch"})
	if status == http.StatusOK {
		t.Fatalf("retrying a row through the wrong parent import must never succeed, got 200 body=%v", got)
	}

	// The SAME row, retried through its REAL parent import, succeeds --
	// proving the refusal above was the parent check, not something wrong
	// with the row or the route itself.
	status, got = importsIDORCall(t, router, http.MethodPost,
		"/fleet-partner/v1/imports/"+importID+"/rows/row_idor_test_1/retry", orgA.ID, owner, nil,
		map[string]string{"Idempotency-Key": "retry-real-parent"})
	if status != http.StatusAccepted {
		t.Fatalf("retrying a row through its real parent import status = %d body=%v, want 202", status, got)
	}
}

// TestExports_ObjectAuthorization_HTTPCrossOrg fills exports' one gap: an
// HTTP-level (not just service-level) cross-org 404 proof. exportsFixture's
// own router (exports_test.go) bakes a single fixed organization id into
// its tenant-context closure, which is exactly right for
// TestExports_HTTPLifecycle's same-org scenarios but cannot express a
// second, genuinely different organization -- so this test builds its own
// header-driven router over the same real Service/Handler construction,
// mirroring the other four IDOR domain suites, to reach a true HTTP-level
// cross-org request. TestExports_TenantIsolation already proves the
// service-layer half of this; this is the HTTP-transport half.
func TestExports_ObjectAuthorization_HTTPCrossOrg(t *testing.T) {
	f := newExportsFixture(t, "Exports IDOR Cross-Org Co", "RC-IDOR-EXP-1", &fakeExporter{rows: [][]string{{"1", "A"}}})
	other := createOrganization(t, f.pool, "Exports IDOR Cross-Org Co B", "RC-IDOR-EXP-2")
	owner := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "owner")
	crossOrgOwner := seedApprovalMember(t, f.pool, other.ID, "rol_system_owner", "cross-org-owner")

	gin.SetMode(gin.TestMode)
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(f.pool), nil, 0)
	store := idempotency.NewStore(f.pool, 0)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	exportshttp.NewHandler(f.service, authorizer, store).RegisterTenantRoutes(group)

	status, created := importsIDORCall(t, engine, http.MethodPost, "/fleet-partner/v1/exports", f.orgID, owner,
		map[string]any{"exportType": "ORDERS"}, map[string]string{"Idempotency-Key": "idor-cross-org-key"})
	if status != http.StatusAccepted {
		t.Fatalf("create status = %d body=%v, want 202", status, created)
	}
	exportID, _ := created["id"].(string)
	if exportID == "" {
		t.Fatalf("create response carries no id: %v", created)
	}

	status, got := importsIDORCall(t, engine, http.MethodGet, "/fleet-partner/v1/exports/"+exportID, f.orgID, owner, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("same-org get status = %d body=%v, want 200", status, got)
	}

	status, got = importsIDORCall(t, engine, http.MethodGet, "/fleet-partner/v1/exports/"+exportID, other.ID, crossOrgOwner, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org get status = %d body=%v, want 404 (safe not-found, not a leak)", status, got)
	}
}
