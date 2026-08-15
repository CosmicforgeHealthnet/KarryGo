package crm

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

// Section 8G HTTP contract tests. They drive the real production router, so the
// permission checks, the Idempotency-Key edge and the registered surface are
// exercised exactly as deployed.

func (e *testEnv) corporateRouter(staffID, business string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: staffID, VendorID: business, SessionID: "session"})
		c.Next()
	})
	RegisterCorporateRoutes(group, NewCorporateHandler(e.service, idempotency.NewStore(e.pool, 0)), e.service.FlagSet())
	return router
}

// TestCorporateHTTP_RegisteredSurfaceIsExactlyTheDocumentedForm is the
// router-side half of the OpenAPI parity check, and the guard against Section
// 8G growing an order, invoice or credit endpoint.
func TestCorporateHTTP_RegisteredSurfaceIsExactlyTheDocumentedForm(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	router := env.corporateRouter("vs_a", businessA)

	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	base := "/api/v1/vendor/crm/corporate-customers"
	want := []string{
		"GET " + base, "POST " + base,
		"GET " + base + "/:corporateId", "PATCH " + base + "/:corporateId",
		"POST " + base + "/:corporateId/activate", "POST " + base + "/:corporateId/hold",
		"POST " + base + "/:corporateId/deactivate", "POST " + base + "/:corporateId/archive",
		"GET " + base + "/:corporateId/branches", "PUT " + base + "/:corporateId/branches",
		"GET " + base + "/:corporateId/contacts", "POST " + base + "/:corporateId/contacts",
		"GET " + base + "/:corporateId/contacts/:contactId",
		"PATCH " + base + "/:corporateId/contacts/:contactId",
		"POST " + base + "/:corporateId/contacts/:contactId/archive",
		"GET " + base + "/:corporateId/memberships", "POST " + base + "/:corporateId/memberships",
		"PATCH " + base + "/:corporateId/memberships/:membershipId",
		"POST " + base + "/:corporateId/memberships/:membershipId/archive",
		"GET " + base + "/:corporateId/locations", "POST " + base + "/:corporateId/locations",
		"GET " + base + "/:corporateId/locations/:locationId",
		"PATCH " + base + "/:corporateId/locations/:locationId",
		"POST " + base + "/:corporateId/locations/:locationId/archive",
		"GET " + base + "/:corporateId/cost-centers", "POST " + base + "/:corporateId/cost-centers",
		"GET " + base + "/:corporateId/cost-centers/:costCenterId",
		"PATCH " + base + "/:corporateId/cost-centers/:costCenterId",
		"POST " + base + "/:corporateId/cost-centers/:costCenterId/archive",
		"GET " + base + "/:corporateId/interactions", "POST " + base + "/:corporateId/interactions",
		"GET " + base + "/:corporateId/interactions/:interactionId",
		"POST " + base + "/:corporateId/interactions/:interactionId/correct",
		"GET " + base + "/:corporateId/tasks", "POST " + base + "/:corporateId/tasks",
		"GET " + base + "/:corporateId/tasks/:taskId", "PATCH " + base + "/:corporateId/tasks/:taskId",
		"POST " + base + "/:corporateId/tasks/:taskId/start",
		"POST " + base + "/:corporateId/tasks/:taskId/complete",
		"POST " + base + "/:corporateId/tasks/:taskId/cancel",
		"POST " + base + "/:corporateId/tasks/:taskId/reassign",
	}
	if len(registered) != len(want) {
		t.Fatalf("registered %d corporate routes; want exactly %d: %v", len(registered), len(want), registered)
	}
	for _, route := range want {
		if !registered[route] {
			t.Errorf("route %s is not registered", route)
		}
	}

	// Nothing that would make Section 8G a commerce, finance or approval
	// surface exists.
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router = env.corporateRouter("vs_a", businessA)
	for _, path := range []string{
		base + "/crmcorp_1/orders", base + "/crmcorp_1/invoices", base + "/crmcorp_1/credit",
		base + "/crmcorp_1/quotes", base + "/crmcorp_1/contracts", base + "/crmcorp_1/approvals",
		base + "/crmcorp_1/wallet", base + "/crmcorp_1/merge",
	} {
		if rec := campaignRequest(t, router, http.MethodPost, path, "k", map[string]any{}); rec.Code != http.StatusNotFound {
			t.Errorf("%s exists (status=%d); Section 8G exposes no commerce or finance route", path, rec.Code)
		}
	}
	// Nor any internal ingress: corporate contact privacy and merge are handled
	// through the existing Section 8A event path.
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "/internal") {
			t.Errorf("Section 8G registered an internal route: %s %s", route.Method, route.Path)
		}
	}
}

// TestCorporateHTTP_EachRouteRequiresItsOwnPermission proves the five
// permissions are genuinely distinct, and that corporate activity requires BOTH
// a Section 8C activity grant and corporate read.
func TestCorporateHTTP_EachRouteRequiresItsOwnPermission(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_http"))
	contact := env.addCorporateContact(t, businessA, "vs_a", profile.ID, person.ID, ContactRoleProcurement, false)

	base := "/api/v1/vendor/crm/corporate-customers"
	type call struct {
		method, path, key string
		body              any
		permission        string
	}
	calls := []call{
		{http.MethodGet, base, "", nil, PermCorporateRead},
		{http.MethodGet, base + "/" + profile.ID, "", nil, PermCorporateRead},
		{http.MethodGet, base + "/" + profile.ID + "/contacts", "", nil, PermCorporateRead},
		{http.MethodGet, base + "/" + profile.ID + "/locations", "", nil, PermCorporateRead},
		{http.MethodGet, base + "/" + profile.ID + "/cost-centers", "", nil, PermCorporateRead},
		{http.MethodGet, base + "/" + profile.ID + "/memberships", "", nil, PermCorporateRead},
		{http.MethodGet, base + "/" + profile.ID + "/branches", "", nil, PermCorporateRead},
		{http.MethodPost, base, "k_create", map[string]any{"organization_name": "Another Company"},
			PermCorporateManage},
		{http.MethodPost, base + "/" + profile.ID + "/contacts", "k_contact",
			map[string]any{"customer_profile_ref": person.ID}, PermCorporateContactsManage},
		{http.MethodPost, base + "/" + profile.ID + "/memberships", "k_member",
			map[string]any{"contact_ref": contact.ID}, PermCorporateContactsManage},
		{http.MethodPost, base + "/" + profile.ID + "/locations", "k_location",
			map[string]any{"location_code": "HQ", "name": "Head office"}, PermCorporateLocationsManage},
		{http.MethodPost, base + "/" + profile.ID + "/cost-centers", "k_center",
			map[string]any{"code": "OPS", "name": "Operations"}, PermCorporateCostCentersManag},
	}

	// Holding nothing denies everything.
	env.service.scopes = fixedScopes{}
	router := env.corporateRouter("vs_a", businessA)
	for _, c := range calls {
		rec := campaignRequest(t, router, c.method, c.path, c.key+"_none", c.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with no permission: status=%d want 403", c.method, c.path, rec.Code)
		}
	}

	// Holding only read allows the reads and nothing else.
	env.service.scopes = permissionScopes{PermCorporateRead: {Global: true}}
	router = env.corporateRouter("vs_a", businessA)
	for _, c := range calls {
		rec := campaignRequest(t, router, c.method, c.path, c.key+"_read", c.body)
		if c.permission == PermCorporateRead {
			if rec.Code != http.StatusOK {
				t.Errorf("%s %s with read: status=%d body=%s", c.method, c.path, rec.Code, rec.Body.String())
			}
			continue
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with only read: status=%d want 403", c.method, c.path, rec.Code)
		}
	}

	// The three sub-resource permissions do not imply one another: managing
	// contacts is not managing the organization's cost-centre taxonomy.
	env.service.scopes = permissionScopes{
		PermCorporateRead: {Global: true}, PermCorporateContactsManage: {Global: true}}
	router = env.corporateRouter("vs_a", businessA)
	another := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_http_2"))
	if rec := campaignRequest(t, router, http.MethodPost, base+"/"+profile.ID+"/contacts", "k_c_ok",
		map[string]any{"customer_profile_ref": another.ID, "relationship_role": "FINANCE"}); rec.Code != http.StatusCreated {
		t.Errorf("contacts.manage should allow a contact: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := campaignRequest(t, router, http.MethodPost, base+"/"+profile.ID+"/cost-centers", "k_c_deny",
		map[string]any{"code": "FIN", "name": "Finance"}); rec.Code != http.StatusForbidden {
		t.Errorf("contacts.manage should not allow a cost centre: status=%d", rec.Code)
	}

	// Corporate activity requires the Section 8C grant AND corporate read: a
	// caller who may log interactions does not thereby learn which
	// organizations this vendor works with.
	env.service.scopes = permissionScopes{PermInteractionsCreate: {Global: true}}
	router = env.corporateRouter("vs_a", businessA)
	if rec := campaignRequest(t, router, http.MethodPost, base+"/"+profile.ID+"/interactions", "k_i_deny",
		map[string]any{"type": "NOTE", "direction": "INTERNAL", "subject": "Note"}); rec.Code != http.StatusForbidden {
		t.Errorf("an activity grant alone should not reach a corporate account: status=%d", rec.Code)
	}
	env.service.scopes = permissionScopes{
		PermInteractionsCreate: {Global: true}, PermCorporateRead: {Global: true}}
	router = env.corporateRouter("vs_a", businessA)
	if rec := campaignRequest(t, router, http.MethodPost, base+"/"+profile.ID+"/interactions", "k_i_ok",
		map[string]any{"type": "NOTE", "direction": "INTERNAL", "subject": "Note"}); rec.Code != http.StatusCreated {
		t.Errorf("activity plus corporate read should be allowed: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestCorporateHTTP_ResponsesCarryNoContactPII is the disclosure test on the
// wire shape itself.
func TestCorporateHTTP_ResponsesCarryNoContactPII(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_pii"))
	env.addCorporateContact(t, businessA, "vs_a", profile.ID, person.ID, ContactRoleProcurement, true)

	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.corporateRouter("vs_a", businessA)
	rec := campaignRequest(t, router, http.MethodGet,
		"/api/v1/vendor/crm/corporate-customers/"+profile.ID+"/contacts", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list contacts: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, forbidden := range []string{
		`"email":`, `"phone":`, `"phone_number":`, `"address":`, `"date_of_birth":`,
		`"first_name":`, `"last_name":`, `"government_id":`, "@",
	} {
		if containsFold(body, forbidden) {
			t.Fatalf("the contact list mentions %q: %s", forbidden, body)
		}
	}
	var envelope struct {
		Data struct {
			Contacts []map[string]any `json:"contacts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Contacts) != 1 {
		t.Fatalf("expected one contact, got %d", len(envelope.Data.Contacts))
	}
	if _, present := envelope.Data.Contacts[0]["customer_profile_ref"]; !present {
		t.Fatal("a contact must carry its opaque relationship reference")
	}
}

// TestCorporateHTTP_CommandsAreIdempotentAtTheEdge checks the shared
// Idempotency-Key contract.
func TestCorporateHTTP_CommandsAreIdempotentAtTheEdge(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.corporateRouter("vs_a", businessA)
	base := "/api/v1/vendor/crm/corporate-customers"

	body := map[string]any{"organization_name": "Idempotent Industries"}
	first := campaignRequest(t, router, http.MethodPost, base, "key-1", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", first.Code, first.Body.String())
	}
	replay := campaignRequest(t, router, http.MethodPost, base, "key-1", body)
	if replay.Code != http.StatusCreated || replay.Body.String() != first.Body.String() {
		t.Fatalf("a replay should return the original response: status=%d", replay.Code)
	}
	var accounts int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*) FROM corporate_customer_profiles
		WHERE business_id=$1`, businessA).Scan(&accounts); err != nil {
		t.Fatal(err)
	}
	if accounts != 1 {
		t.Fatalf("a replayed create produced %d accounts", accounts)
	}
	conflict := campaignRequest(t, router, http.MethodPost, base, "key-1",
		map[string]any{"organization_name": "Different Industries"})
	if conflict.Code != http.StatusConflict {
		t.Fatalf("reusing a key with a different body should conflict: status=%d", conflict.Code)
	}
	missing := campaignRequest(t, router, http.MethodPost, base, "",
		map[string]any{"organization_name": "Keyless Industries"})
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("a command without an Idempotency-Key should be refused: status=%d", missing.Code)
	}
}

// TestCorporateHTTP_DisabledFlagsRegisterNothing keeps the rollout switches
// meaningful.
func TestCorporateHTTP_DisabledFlagsRegisterNothing(t *testing.T) {
	off := allFlagsOn()
	off.CorporateEnabled = false
	env := newTestEnv(t, off)
	if router := env.corporateRouter("vs_a", businessA); len(router.Routes()) != 0 {
		t.Fatalf("corporate CRM registered %d routes while disabled", len(router.Routes()))
	}

	// With the Section 8C engine off, the corporate activity routes it
	// delegates to are not registered either: a route that could only answer
	// "writes disabled" would be worse than not existing.
	noActivity := allFlagsOn()
	noActivity.InteractionsEnabled = false
	noActivity.TasksEnabled = false
	quiet := newTestEnv(t, noActivity)
	router := quiet.corporateRouter("vs_a", businessA)
	for _, route := range router.Routes() {
		if containsFold(route.Path, "/interactions") || containsFold(route.Path, "/tasks") {
			t.Errorf("corporate activity route %s registered while Section 8C is disabled", route.Path)
		}
	}
	if len(router.Routes()) != 29 {
		t.Fatalf("expected the 29 non-activity corporate routes, got %d", len(router.Routes()))
	}
}
