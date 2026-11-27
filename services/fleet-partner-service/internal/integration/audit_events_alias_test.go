package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	audithttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/http"
	auditrepositories "cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/repositories"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// TestAuditEventsAliasServesTheAuditLogList proves the spec 56.3/59.10 path
// GET /audit-events is the same audit list as GET /audit-logs -- same
// handler, filters, tenant scope and permission -- not a second audit history.
func TestAuditEventsAliasServesTheAuditLogList(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Audit Alias "+ids.New("o"), "AUD-"+ids.New("r"))
	other := createOrganization(t, pool, "Audit Alias Other "+ids.New("o"), "AUDO-"+ids.New("r"))
	stamp := fmt.Sprint(time.Now().UnixNano() % 100000000)
	owner := seedApprovalMember(t, pool, org.ID, "rol_system_owner", "alias-owner-"+stamp)
	nobody := seedMember(t, pool, org.ID, seedIdentity(t, pool, ids.New("b")+"@example.test", "+23482"+stamp), false, nil).ID

	recorder := audit.NewRecorder()
	for _, o := range []string{org.ID, org.ID, other.ID} {
		if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			return recorder.Record(ctx, tx, audit.Entry{OrganizationID: o, ActorType: "organization_user", ActorID: owner, Action: "alias.probe", ResourceType: "probe", ResourceID: ids.New("prb"), RequestID: ids.New("req")})
		}); err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{OrganizationID: org.ID, OrganizationUserID: member, ActorType: tenant.ActorTypeOrganizationUser, ActorID: member})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	audithttp.NewHandler(auditrepositories.NewRepository(pool), authorizer).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))

	get := func(path, member string) (int, []any) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Test-Member", member)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		var envelope struct {
			Data []any `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &envelope)
		return rec.Code, envelope.Data
	}
	const query = "?action=alias.probe&limit=50"
	logsCode, logs := get("/fleet-partner/v1/audit-logs"+query, owner)
	eventsCode, events := get("/fleet-partner/v1/audit-events"+query, owner)
	if logsCode != http.StatusOK || eventsCode != http.StatusOK {
		t.Fatalf("audit-logs = %d, audit-events = %d, want 200", logsCode, eventsCode)
	}
	if len(events) != 2 {
		t.Fatalf("audit-events returned %d entries, want 2 (the other organization's entry must not appear)", len(events))
	}
	a, _ := json.Marshal(logs)
	b, _ := json.Marshal(events)
	if string(a) != string(b) {
		t.Fatalf("audit-events differs from audit-logs:\n logs   %s\n events %s", a, b)
	}
	for _, path := range []string{"/fleet-partner/v1/audit-logs", "/fleet-partner/v1/audit-events"} {
		if code, _ := get(path, nobody); code != http.StatusForbidden {
			t.Fatalf("%s without audit.read = %d, want 403", path, code)
		}
	}
}
