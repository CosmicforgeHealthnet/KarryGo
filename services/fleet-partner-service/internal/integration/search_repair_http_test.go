package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	searchhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/http"
	searchrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/repositories"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func searchRepairCall(t *testing.T, router http.Handler, method string, path string,
	memberID string, body any) (int, map[string]any) {
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
	request.Header.Set("X-Test-Member", memberID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	response := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)
	return recorder.Code, response
}

// TestSearchRepair_RequiresRepairPermissionNotJustRead proves search.read
// alone (the ordinary search permission) does not extend to the repair
// routes, and that a caller who does hold search.repair can force a reindex
// and a removal.
func TestSearchRepair_RequiresRepairPermissionNotJustRead(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Search Repair Co", "SRCH-REPAIR-2")
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	service := searchusecases.NewService(searchusecases.Options{
		Pool: pool, Repository: searchrepos.NewRepository(pool), Authorizer: authorizer,
		Flags: searchusecases.Flags{Enabled: true},
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: org.ID, OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	searchhttp.NewHandler(service, authorizer).RegisterTenantRoutes(group)

	// rol_system_crm holds search.read (migration 000053) but not
	// search.repair (migration 000058 grants only owner/admin/operations).
	readOnlyMember := seedApprovalMember(t, pool, org.ID, "rol_system_crm", "read-only")
	status, response := searchRepairCall(t, engine, http.MethodPost, "/fleet-partner/v1/search/reindex",
		readOnlyMember, map[string]any{
			"subjectType": "ORDER", "subjectReference": "ord_repair_1", "title": "ORD-REPAIR-1",
			"requiredPermission": "search.read", "sourceVersion": 1,
		})
	if status != http.StatusForbidden {
		t.Fatalf("read-only member reindex status = %d body=%v, want 403", status, response)
	}

	operator := seedApprovalMember(t, pool, org.ID, "rol_system_owner", "operator")
	status, response = searchRepairCall(t, engine, http.MethodPost, "/fleet-partner/v1/search/reindex",
		operator, map[string]any{
			"subjectType": "ORDER", "subjectReference": "ord_repair_1", "title": "ORD-REPAIR-1",
			"requiredPermission": "search.read", "sourceVersion": 1,
		})
	if status != http.StatusOK {
		t.Fatalf("operator reindex status = %d body=%v, want 200", status, response)
	}

	// The reindexed subject is now findable by the operator via ordinary
	// search.read.
	results, err := service.Search(context.Background(), tenant.Context{
		OrganizationID: org.ID, OrganizationUserID: operator, ActorType: tenant.ActorTypeOrganizationUser,
	}, searchusecases.SearchInput{Text: "ORD-REPAIR-1"})
	if err != nil {
		t.Fatalf("search after reindex: %v", err)
	}
	if len(results) != 1 || results[0].SubjectReference != "ord_repair_1" {
		t.Fatalf("search after operator reindex = %+v, want the repaired subject", results)
	}

	// A read-only member cannot remove it either.
	status, response = searchRepairCall(t, engine, http.MethodPost, "/fleet-partner/v1/search/remove",
		readOnlyMember, map[string]any{"subjectType": "ORDER", "subjectReference": "ord_repair_1"})
	if status != http.StatusForbidden {
		t.Fatalf("read-only member remove status = %d body=%v, want 403", status, response)
	}

	// The operator can remove it.
	status, response = searchRepairCall(t, engine, http.MethodPost, "/fleet-partner/v1/search/remove",
		operator, map[string]any{"subjectType": "ORDER", "subjectReference": "ord_repair_1"})
	if status != http.StatusOK {
		t.Fatalf("operator remove status = %d body=%v, want 200", status, response)
	}

	results, err = service.Search(context.Background(), tenant.Context{
		OrganizationID: org.ID, OrganizationUserID: operator, ActorType: tenant.ActorTypeOrganizationUser,
	}, searchusecases.SearchInput{Text: "ORD-REPAIR-1"})
	if err != nil {
		t.Fatalf("search after remove: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("search after operator remove = %+v, want no results", results)
	}
}
