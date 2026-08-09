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
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/synchronization"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Section 28 gap-1 IDOR domain 5/5: reconciliation and dead letters
// (synchronization).
//
// Every one of this package's service methods (GetCase, Reconcile,
// GetDeadLetter, Replay) filters directly on organization_id and maps ANY
// error -- not-found, wrong org, wrong status -- to the same
// apperrors.NotFound("Synchronization record not found.") (http.go:80-83),
// so cross-org denial is uniform and safe by construction: a caller can
// never distinguish "does not exist" from "exists in another
// organization". This domain has no branch concept (neither
// fleet_partner_synchronization_cases nor _dead_letters carries a
// branch_id column) and no nested-parent-in-URL shape (both resources are
// flat, single-parameter routes), so this suite proves same-org access,
// cross-org denial (safe 404) and missing-permission denial (403) -- the
// full subset of Section 28's matrix that applies to this domain.

func synchronizationIDORRouter(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := synchronization.New(pool, synchronization.Flags{Enabled: true})
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)

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
	synchronization.NewHandler(service, authorizer).RegisterTenantRoutes(group)
	return engine
}

func synchronizationIDORCall(t *testing.T, router http.Handler, method, path, orgID, memberID string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(nil))
	request.Header.Set("X-Test-Org", orgID)
	request.Header.Set("X-Test-Member", memberID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	envelope := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	if data, ok := envelope["data"].(map[string]any); ok {
		return recorder.Code, data
	}
	return recorder.Code, envelope
}

// seedSynchronizationProjection inserts the minimal projection row a case
// or dead letter needs to exist meaningfully -- ApplyTx's full event
// pipeline (exercised elsewhere, see synchronization_section26_test.go) is
// not what this authorization suite is testing.
func seedSynchronizationProjection(t *testing.T, pool *pgxpool.Pool, id, orgID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO fleet_partner_synchronization_projections
		 (id,organization_id,projection_type,source_service,subject_reference,source_version,payload_hash,last_event_at)
		 VALUES($1,$2,'test.projection','test-service',$1,1,'hash-1',now())`, id, orgID); err != nil {
		t.Fatalf("seed synchronization projection: %v", err)
	}
}

// TestSynchronizationReconciliationCases_ObjectAuthorization proves the
// object-authorization matrix for /synchronization/reconciliation-cases.
func TestSynchronizationReconciliationCases_ObjectAuthorization(t *testing.T) {
	pool := testPool(t)
	orgA := createOrganization(t, pool, "Synchronization IDOR Co A", "RC-IDOR-SYN-1")
	orgB := createOrganization(t, pool, "Synchronization IDOR Co B", "RC-IDOR-SYN-2")
	router := synchronizationIDORRouter(t, pool)

	owner := seedApprovalMember(t, pool, orgA.ID, "rol_system_owner", "owner")
	// rol_system_crm holds no synchronization.* permission (migration
	// 000062 grants those to owner/admin/operations only).
	noPermission := seedApprovalMember(t, pool, orgA.ID, "rol_system_crm", "no-perm")
	crossOrgOwner := seedApprovalMember(t, pool, orgB.ID, "rol_system_owner", "cross-org-owner")

	seedSynchronizationProjection(t, pool, "proj_idor_case_1", orgA.ID)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO fleet_partner_synchronization_cases(id,organization_id,projection_id,reason) VALUES('case_idor_test_1',$1,'proj_idor_case_1','VERSION_GAP')`,
		orgA.ID); err != nil {
		t.Fatalf("seed synchronization case: %v", err)
	}

	// Missing permission -> 403.
	status, body := synchronizationIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/synchronization/reconciliation-cases/case_idor_test_1", orgA.ID, noPermission)
	if status != http.StatusForbidden {
		t.Fatalf("get without synchronization.read status = %d body=%v, want 403", status, body)
	}

	// Same-org read.
	status, got := synchronizationIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/synchronization/reconciliation-cases/case_idor_test_1", orgA.ID, owner)
	if status != http.StatusOK {
		t.Fatalf("same-org get status = %d body=%v, want 200", status, got)
	}
	if got["id"] != "case_idor_test_1" {
		t.Fatalf("case id = %v, want case_idor_test_1", got["id"])
	}

	// Cross-org: orgB's owner-tier member gets a safe 404 for orgA's case --
	// GetCase's error is indistinguishable from "does not exist".
	status, got = synchronizationIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/synchronization/reconciliation-cases/case_idor_test_1", orgB.ID, crossOrgOwner)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org get status = %d body=%v, want 404 (safe not-found, not a leak)", status, got)
	}

	// Cross-org reconcile: same safe 404, and the mutation must not happen.
	status, got = synchronizationIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/synchronization/reconciliation-cases/case_idor_test_1/reconcile", orgB.ID, crossOrgOwner)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org reconcile status = %d body=%v, want 404 (safe not-found, not a leak)", status, got)
	}

	// Same-org reconcile succeeds (the case was still OPEN -- untouched by
	// the cross-org attempt above).
	status, got = synchronizationIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/synchronization/reconciliation-cases/case_idor_test_1/reconcile", orgA.ID, owner)
	if status != http.StatusAccepted {
		t.Fatalf("same-org reconcile status = %d body=%v, want 202", status, got)
	}
}

// TestSynchronizationDeadLetters_ObjectAuthorization proves the
// object-authorization matrix for /synchronization/dead-letters.
func TestSynchronizationDeadLetters_ObjectAuthorization(t *testing.T) {
	pool := testPool(t)
	orgA := createOrganization(t, pool, "Synchronization IDOR DL Co A", "RC-IDOR-SYN-3")
	orgB := createOrganization(t, pool, "Synchronization IDOR DL Co B", "RC-IDOR-SYN-4")
	router := synchronizationIDORRouter(t, pool)

	owner := seedApprovalMember(t, pool, orgA.ID, "rol_system_owner", "owner")
	noPermission := seedApprovalMember(t, pool, orgA.ID, "rol_system_crm", "no-perm")
	crossOrgOwner := seedApprovalMember(t, pool, orgB.ID, "rol_system_owner", "cross-org-owner")

	seedSynchronizationProjection(t, pool, "proj_idor_dl_1", orgA.ID)
	if _, err := pool.Exec(context.Background(),
		`UPDATE fleet_partner_synchronization_projections SET source_service='dl-source',projection_type='dl-event' WHERE id='proj_idor_dl_1'`); err != nil {
		t.Fatalf("align projection to dead letter join keys: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO fleet_partner_synchronization_dead_letters
		 (id,organization_id,source_service,event_id,event_type,subject_reference,source_version,payload_hash)
		 VALUES('dl_idor_test_1',$1,'dl-source','evt-1','dl-event','proj_idor_dl_1',1,'hash-1')`, orgA.ID); err != nil {
		t.Fatalf("seed synchronization dead letter: %v", err)
	}

	// Missing permission -> 403.
	status, body := synchronizationIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/synchronization/dead-letters/dl_idor_test_1", orgA.ID, noPermission)
	if status != http.StatusForbidden {
		t.Fatalf("get without synchronization.read status = %d body=%v, want 403", status, body)
	}

	// Same-org read.
	status, got := synchronizationIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/synchronization/dead-letters/dl_idor_test_1", orgA.ID, owner)
	if status != http.StatusOK {
		t.Fatalf("same-org get status = %d body=%v, want 200", status, got)
	}
	if got["id"] != "dl_idor_test_1" {
		t.Fatalf("dead letter id = %v, want dl_idor_test_1", got["id"])
	}

	// Cross-org read -> safe 404.
	status, got = synchronizationIDORCall(t, router, http.MethodGet, "/fleet-partner/v1/synchronization/dead-letters/dl_idor_test_1", orgB.ID, crossOrgOwner)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org get status = %d body=%v, want 404 (safe not-found, not a leak)", status, got)
	}

	// Cross-org replay -> safe 404, and the dead letter must not move.
	status, got = synchronizationIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/synchronization/dead-letters/dl_idor_test_1/replay", orgB.ID, crossOrgOwner)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org replay status = %d body=%v, want 404 (safe not-found, not a leak)", status, got)
	}

	// Same-org replay succeeds -- proving the cross-org attempt above truly
	// left the row untouched (status still OPEN) rather than silently
	// consuming it.
	status, got = synchronizationIDORCall(t, router, http.MethodPost, "/fleet-partner/v1/synchronization/dead-letters/dl_idor_test_1/replay", orgA.ID, owner)
	if status != http.StatusAccepted {
		t.Fatalf("same-org replay status = %d body=%v, want 202", status, got)
	}
}
