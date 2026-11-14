package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Shared harness for the Section 14-20 requirement-row tests (rows_*_test.go).
//
// Each test mounts the feature's REAL tenant handler (permission gate,
// request parsing, use case, SQL) on a gin engine whose only stand-in is the
// session middleware: the tenant context comes from test headers instead of
// a session cookie. Authorization still runs through the production
// accesscontrol.Authorizer against role bindings seeded in PostgreSQL, so a
// member without the permission gets 403 and another organization's object
// gets the safe 404 exactly as in production.

type rowsRegistrar func(g *gin.RouterGroup, a *accesscontrol.Authorizer, s *idempotency.Store)

func rowsRouter(t *testing.T, pool *pgxpool.Pool, register rowsRegistrar) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	store := idempotency.NewStore(pool, 0)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		var permitted []string
		if restrict := c.GetHeader("X-Test-Permitted-Branch"); restrict != "" {
			permitted = []string{restrict}
		}
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member,
			PermittedBranchIDs: permitted, CorrelationID: "rows-test",
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	register(engine.Group("/fleet-partner/v1"), authorizer, store)
	return engine
}

// rowsResponse is one decoded HTTP response.
type rowsResponse struct {
	Status int
	Data   json.RawMessage
	Error  struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	Raw []byte
}

// Object decodes the data member as an object.
func (r rowsResponse) Object(t *testing.T) map[string]any {
	t.Helper()
	out := map[string]any{}
	if err := json.Unmarshal(r.Data, &out); err != nil {
		t.Fatalf("decode object (status %d): %v body=%s", r.Status, err, r.Raw)
	}
	return out
}

// Items decodes the data member as a list. Some features wrap the list in an
// object ({"items": [...]}); both shapes are accepted.
func (r rowsResponse) Items(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(r.Data, &out); err == nil {
		return out
	}
	var wrapped struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(r.Data, &wrapped); err != nil {
		t.Fatalf("decode list (status %d): %v body=%s", r.Status, err, r.Raw)
	}
	return wrapped.Items
}

type rowsActor struct {
	Org, Member, Branch string
}

func rowsDo(t *testing.T, router http.Handler, actor rowsActor, method, path string, body any,
	headers map[string]string) rowsResponse {
	t.Helper()
	var reader *bytes.Reader
	switch v := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(v))
	default:
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Org", actor.Org)
	request.Header.Set("X-Test-Member", actor.Member)
	if actor.Branch != "" {
		request.Header.Set("X-Test-Permitted-Branch", actor.Branch)
	}
	if method != http.MethodGet {
		if _, ok := headers["Idempotency-Key"]; !ok {
			request.Header.Set("Idempotency-Key", ids.New("idem"))
		}
	}
	for k, v := range headers {
		if v == "" {
			request.Header.Del(k)
			continue
		}
		request.Header.Set(k, v)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	out := rowsResponse{Status: recorder.Code, Raw: recorder.Body.Bytes()}
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(out.Raw, &envelope)
	out.Data = envelope.Data
	out.Error = envelope.Error
	return out
}

// rowsExpect fails unless the response has the wanted status.
func rowsExpect(t *testing.T, r rowsResponse, want int, what string) rowsResponse {
	t.Helper()
	if r.Status != want {
		t.Fatalf("%s: status %d, want %d; body=%s", what, r.Status, want, r.Raw)
	}
	return r
}

// rowsIDs collects the value of key from each item.
func rowsIDs(items []map[string]any, key string) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		if v, ok := item[key].(string); ok {
			out[v] = true
		}
	}
	return out
}

// rowsOrgs is the standard two-organization setup: an owner in each
// organization and a member in the primary organization who holds a role
// lacking the feature's permissions.
type rowsOrgs struct {
	pool               *pgxpool.Pool
	A, B, NoPermission rowsActor
}

func newRowsOrgs(t *testing.T, label string, noPermissionRole string) rowsOrgs {
	t.Helper()
	pool := testPool(t)
	orgA := createOrganization(t, pool, label+" A", "RC-"+ids.New("rows"))
	orgB := createOrganization(t, pool, label+" B", "RC-"+ids.New("rows"))
	out := rowsOrgs{pool: pool}
	out.A = rowsActor{Org: orgA.ID, Member: seedApprovalMember(t, pool, orgA.ID, "rol_system_owner", "owner-a")}
	out.B = rowsActor{Org: orgB.ID, Member: seedApprovalMember(t, pool, orgB.ID, "rol_system_owner", "owner-b")}
	if noPermissionRole != "" {
		out.NoPermission = rowsActor{Org: orgA.ID, Member: seedApprovalMember(t, pool, orgA.ID, noPermissionRole, "no-perm")}
	}
	return out
}

func (o rowsOrgs) tenantA() tenant.Context {
	return tenant.Context{OrganizationID: o.A.Org, OrganizationUserID: o.A.Member,
		ActorType: tenant.ActorTypeOrganizationUser, ActorID: o.A.Member, CorrelationID: "rows-test"}
}

func (o rowsOrgs) tenantB() tenant.Context {
	return tenant.Context{OrganizationID: o.B.Org, OrganizationUserID: o.B.Member,
		ActorType: tenant.ActorTypeOrganizationUser, ActorID: o.B.Member, CorrelationID: "rows-test"}
}

// rowsAuditCount counts audit records for one resource.
func rowsAuditCount(t *testing.T, pool *pgxpool.Pool, org, resourceID string) int {
	t.Helper()
	return countWhere(t, pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND resource_id=$2`, org, resourceID)
}

// rowsOutboxCount counts outbox events for one aggregate and event type.
func rowsOutboxCount(t *testing.T, pool *pgxpool.Pool, aggregateID, eventType string) int {
	t.Helper()
	return countWhere(t, pool, `SELECT count(*) FROM fleet_partner_outbox WHERE aggregate_id=$1 AND event_type=$2`, aggregateID, eventType)
}
