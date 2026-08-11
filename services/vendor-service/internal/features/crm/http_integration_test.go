package crm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
)

// routerFor builds the production router shape with a stubbed authentication
// middleware, so the handlers, permission checks and scoping run for real.
func (e *testEnv) routerFor(staffID, business string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: staffID, VendorID: business, SessionID: "session"})
		c.Next()
	})
	RegisterRoutes(group, NewHandler(e.service), e.service.FlagSet())
	return router
}

func doRequest(t *testing.T, router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// TestHTTPPermissionsAreEnforced proves each route requires its own permission
// and that holding none denies access.
func TestHTTPPermissionsAreEnforced(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_http"))

	// A staff member holding no CRM permission at all.
	env.service.scopes = fixedScopes{}
	router := env.routerFor("vs_a", businessA)

	for _, path := range []string{
		"/api/v1/vendor/crm/customers",
		"/api/v1/vendor/crm/customers/" + profile.ID,
		"/api/v1/vendor/crm/customers/" + profile.ID + "/consent",
		"/api/v1/vendor/crm/customers/" + profile.ID + "/privacy",
	} {
		rec := doRequest(t, router, http.MethodGet, path)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d want 403 for a staff member with no CRM permission", path, rec.Code)
		}
	}
}

// TestHTTPCustomer360ReturnsNoCanonicalPII is the contract-level privacy guard:
// the response must contain no email, phone, address or date-of-birth field,
// whatever the underlying data.
func TestHTTPCustomer360ReturnsNoCanonicalPII(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_pii"))
	env.seedOrder(t, businessA, "vs_a", profile, "ord_pii", branchA1, 12_000, 12_000, env.now.AddDate(0, 0, -1), 1)

	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.routerFor("vs_a", businessA)

	rec := doRequest(t, router, http.MethodGet, "/api/v1/vendor/crm/customers/"+profile.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"email", "phone", "msisdn", "date_of_birth", "dob", "address", "password"} {
		if keyExistsDeep(payload, forbidden) {
			t.Fatalf("Customer 360 exposed a %q field: %s", forbidden, rec.Body.String())
		}
	}
}

// keyExistsDeep reports whether any object key anywhere in the payload matches.
func keyExistsDeep(node any, key string) bool {
	switch typed := node.(type) {
	case map[string]any:
		for name, value := range typed {
			if name == key {
				return true
			}
			if keyExistsDeep(value, key) {
				return true
			}
		}
	case []any:
		for _, value := range typed {
			if keyExistsDeep(value, key) {
				return true
			}
		}
	}
	return false
}

// TestHTTPListRejectsUnsafeFilters proves the search surface refuses filters
// over data CRM does not hold, and refuses to surface redacted relationships.
func TestHTTPListRejectsUnsafeFilters(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.routerFor("vs_a", businessA)

	// Filtering by REDACTED is rejected outright, so redacted relationships
	// cannot be pulled into an ordinary working view. The platform's
	// validation envelope is 422, matching Sections 5-7.
	rec := doRequest(t, router, http.MethodGet, "/api/v1/vendor/crm/customers?privacy_state=REDACTED")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("privacy_state=REDACTED status=%d want 422", rec.Code)
	}

	// Unsupported sorts are rejected rather than injected.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/vendor/crm/customers?sort=customer_ref")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported sort status=%d want 422", rec.Code)
	}

	// The page size is bounded.
	rec = doRequest(t, router, http.MethodGet, "/api/v1/vendor/crm/customers?limit=5000")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("limit=5000 status=%d want 422", rec.Code)
	}
}

// TestHTTPArchiveRequiresExpectedVersion proves optimistic concurrency is
// enforced at the edge, not merely available.
func TestHTTPArchiveRequiresExpectedVersion(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_archive"))
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.routerFor("vs_a", businessA)

	base := "/api/v1/vendor/crm/customers/" + profile.ID + "/archive"

	if rec := doRequest(t, router, http.MethodPost, base); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("archive without a version status=%d want 422", rec.Code)
	}
	if rec := doRequest(t, router, http.MethodPost, base+"?expected_version=999"); rec.Code != http.StatusConflict {
		t.Fatalf("archive with a wrong version status=%d want 409", rec.Code)
	}
	rec := doRequest(t, router, http.MethodPost, base+"?expected_version=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("archive status=%d body=%s", rec.Code, rec.Body.String())
	}
	if env.reload(t, businessA, profile.ID).Status != StatusArchived {
		t.Fatal("archive did not take effect")
	}

	// Restore round-trips with the bumped version.
	restored := env.reload(t, businessA, profile.ID)
	rec = doRequest(t, router, http.MethodPost,
		"/api/v1/vendor/crm/customers/"+profile.ID+"/restore?expected_version="+itoa(int(restored.Version)))
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status=%d body=%s", rec.Code, rec.Body.String())
	}
	if env.reload(t, businessA, profile.ID).Status != StatusActive {
		t.Fatal("restore did not take effect")
	}
}

// TestNoRoutesWhenFeatureDisabled proves a disabled CRM has no surface at all.
func TestNoRoutesWhenFeatureDisabled(t *testing.T) {
	env := newTestEnv(t, Flags{CRMEnabled: false})
	router := env.routerFor("vs_a", businessA)
	if rec := doRequest(t, router, http.MethodGet, "/api/v1/vendor/crm/customers"); rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 when the CRM feature is disabled", rec.Code)
	}
}

// TestHTTPBranchScopedListIsBranchScoped proves the edge honours branch scope,
// not just the service layer.
func TestHTTPBranchScopedListIsBranchScoped(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	shared := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_http_scope"))
	env.seedOrder(t, businessA, "vs_a", shared, "ord_h1", branchA1, 100_000, 100_000, env.now.AddDate(0, 0, -6), 1)
	env.seedOrder(t, businessA, "vs_a", shared, "ord_h2", branchA2, 300_000, 300_000, env.now.AddDate(0, 0, -5), 1)

	env.service.scopes = fixedScopes{"vs_a": {BranchIDs: []string{branchA1}}}
	router := env.routerFor("vs_a", businessA)

	rec := doRequest(t, router, http.MethodGet, "/api/v1/vendor/crm/customers")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			Items []struct {
				Commercial struct {
					Scope         string `json:"scope"`
					NetSpendMinor *int64 `json:"net_spend_minor"`
				} `json:"commercial"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Items) != 1 {
		t.Fatalf("items=%d want 1", len(response.Data.Items))
	}
	item := response.Data.Items[0]
	if item.Commercial.Scope != "BRANCH" {
		t.Fatalf("scope=%s want BRANCH", item.Commercial.Scope)
	}
	if item.Commercial.NetSpendMinor == nil || *item.Commercial.NetSpendMinor != 100_000 {
		t.Fatalf("list net spend=%v; want the branch figure 100000, not the business-wide 400000", item.Commercial.NetSpendMinor)
	}
}
