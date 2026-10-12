package crm

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

// Section 8H HTTP contract tests, driven through the real production router.

func (e *testEnv) section8hRouter(staffID, business string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: staffID, VendorID: business, SessionID: "session"})
		c.Next()
	})
	store := idempotency.NewStore(e.pool, 0)
	RegisterIntelligenceRoutes(group, NewIntelligenceHandler(e.service, store), e.service.FlagSet())
	RegisterExportRoutes(group, NewExportHandler(e.service, store), e.service.FlagSet())
	return router
}

// TestSection8HHTTP_RegisteredSurfaceIsExactlyEleven is the router-side half of the
// OpenAPI parity check, and the guard against Section 8H growing a route that
// writes a score or accepts a query.
func TestSection8HHTTP_RegisteredSurfaceIsExactlyEleven(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	router := env.section8hRouter("vs_a", businessA)

	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	want := []string{
		"GET /api/v1/vendor/crm/intelligence/overview",
		"GET /api/v1/vendor/crm/intelligence/rfm",
		"GET /api/v1/vendor/crm/intelligence/churn",
		"GET /api/v1/vendor/crm/customers/:profileId/intelligence",
		"POST /api/v1/vendor/crm/intelligence/rebuild",
		"GET /api/v1/vendor/crm/exports",
		"POST /api/v1/vendor/crm/exports",
		"GET /api/v1/vendor/crm/exports/:exportId",
		"POST /api/v1/vendor/crm/exports/:exportId/cancel",
		"POST /api/v1/vendor/crm/exports/:exportId/download",
		"POST /api/v1/vendor/crm/exports/:exportId/file",
	}
	if len(registered) != len(want) {
		t.Fatalf("registered %d Section 8H routes; want exactly %d: %v", len(registered), len(want), registered)
	}
	for _, route := range want {
		if !registered[route] {
			t.Errorf("route %s is not registered", route)
		}
	}

	// No route writes a score by hand, and none exposes a query surface.
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router = env.section8hRouter("vs_a", businessA)
	for _, path := range []string{
		"/api/v1/vendor/crm/customers/crmp_1/intelligence",
		"/api/v1/vendor/crm/customers/crmp_1/rfm",
		"/api/v1/vendor/crm/customers/crmp_1/churn-score",
		"/api/v1/vendor/crm/intelligence/scores",
		"/api/v1/vendor/crm/exports/query",
		"/api/v1/vendor/crm/exports/sql",
	} {
		for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodPut} {
			rec := campaignRequest(t, router, method, path, "k", map[string]any{"risk_score": 90})
			if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s exists (status=%d); intelligence is derived and exports take no query",
					method, path, rec.Code)
			}
		}
	}
	// And no internal ingress: nothing outside this service produces a score or
	// consumes an export.
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "/internal") {
			t.Errorf("Section 8H registered an internal route: %s %s", route.Method, route.Path)
		}
	}
}

// TestSection8HHTTP_EachRouteRequiresItsOwnPermission proves the six
// permissions are genuinely distinct.
func TestSection8HHTTP_EachRouteRequiresItsOwnPermission(t *testing.T) {
	env, _, stepUp := newExportEnv(t)
	env.seedExportFixture(t)
	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCustomerSummary, Format: FormatCSV, ScopeType: IntelligenceScopeBusiness})
	env.generateExports(t)
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_http_intel"))

	type call struct {
		method, path, key string
		body              any
		permission        string
	}
	base := "/api/v1/vendor/crm"
	calls := []call{
		{http.MethodGet, base + "/intelligence/overview", "", nil, PermIntelligenceRead},
		{http.MethodGet, base + "/intelligence/rfm", "", nil, PermIntelligenceRead},
		{http.MethodGet, base + "/intelligence/churn", "", nil, PermIntelligenceRead},
		{http.MethodGet, base + "/customers/" + profile.ID + "/intelligence", "", nil, PermIntelligenceRead},
		{http.MethodPost, base + "/intelligence/rebuild", "k_rebuild",
			map[string]any{"model_key": ModelRFM, "scope_type": IntelligenceScopeBusiness},
			PermIntelligenceRebuild},
		{http.MethodGet, base + "/exports", "", nil, PermExportsRead},
		{http.MethodGet, base + "/exports/" + export.ID, "", nil, PermExportsRead},
		{http.MethodPost, base + "/exports", "k_create",
			map[string]any{"export_type": ExportCustomerSummary, "format": FormatCSV,
				"scope_type": IntelligenceScopeBusiness, "step_up_grant_id": "g"}, PermExportsCreate},
		{http.MethodPost, base + "/exports/" + export.ID + "/cancel", "k_cancel",
			map[string]any{"expected_version": 0}, PermExportsCancel},
		{http.MethodPost, base + "/exports/" + export.ID + "/download", "k_dl",
			map[string]any{"step_up_grant_id": "g"}, PermExportsDownload},
	}

	// Holding nothing denies everything.
	env.service.scopes = fixedScopes{}
	router := env.section8hRouter("vs_a", businessA)
	for _, c := range calls {
		rec := campaignRequest(t, router, c.method, c.path, c.key+"_none", c.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with no permission: status=%d want 403", c.method, c.path, rec.Code)
		}
	}

	// Holding only intelligence read allows the four reads and nothing else.
	env.service.scopes = permissionScopes{PermIntelligenceRead: {Global: true}}
	router = env.section8hRouter("vs_a", businessA)
	for _, c := range calls {
		rec := campaignRequest(t, router, c.method, c.path, c.key+"_ir", c.body)
		if c.permission == PermIntelligenceRead {
			if rec.Code != http.StatusOK {
				t.Errorf("%s %s with intelligence read: status=%d body=%s",
					c.method, c.path, rec.Code, rec.Body.String())
			}
			continue
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with only intelligence read: status=%d want 403", c.method, c.path, rec.Code)
		}
	}

	// Reading exports is not creating, cancelling or downloading one. These are
	// separated because they are four different disclosures.
	env.service.scopes = permissionScopes{PermExportsRead: {Global: true}}
	router = env.section8hRouter("vs_a", businessA)
	if rec := campaignRequest(t, router, http.MethodGet, base+"/exports", "", nil); rec.Code != http.StatusOK {
		t.Errorf("exports.read did not allow listing: status=%d", rec.Code)
	}
	for _, c := range calls {
		if c.permission == PermExportsRead || c.permission == PermIntelligenceRead {
			continue
		}
		rec := campaignRequest(t, router, c.method, c.path, c.key+"_er", c.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with only exports.read: status=%d want 403", c.method, c.path, rec.Code)
		}
	}
}

// TestSection8HHTTP_DownloadResponseCarriesNoDurableCredential pins the wire
// shape of the one route that releases customer data.
func TestSection8HHTTP_DownloadResponseCarriesNoDurableCredential(t *testing.T) {
	env, _, stepUp := newExportEnv(t)
	env.seedExportFixture(t)
	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportIntelligenceScores, Format: FormatCSV, ScopeType: IntelligenceScopeBusiness})
	env.generateExports(t)

	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.section8hRouter("vs_a", businessA)
	stepUp.issue("grant_http_dl", StepUpPurposeExportDownload)
	rec := campaignRequest(t, router, http.MethodPost,
		"/api/v1/vendor/crm/exports/"+export.ID+"/download", "k_http_dl",
		map[string]any{"step_up_grant_id": "grant_http_dl"})
	if rec.Code != http.StatusOK {
		t.Fatalf("download: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// A short-lived token and path, with an expiry — and no permanent URL, no
	// storage handle and no bytes.
	for _, required := range []string{"download_token", "download_path", "expires_at"} {
		if !strings.Contains(body, required) {
			t.Errorf("the access response is missing %s: %s", required, body)
		}
	}
	for _, forbidden := range []string{"artifact_ref", "\"url\"", "signed_url", "public_url", "https://"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the access response exposes %q: %s", forbidden, body)
		}
	}

	// The export view itself never carries a storage handle either.
	rec = campaignRequest(t, router, http.MethodGet, "/api/v1/vendor/crm/exports/"+export.ID, "", nil)
	if strings.Contains(rec.Body.String(), "artifact_ref") {
		t.Errorf("the export view exposes its storage handle: %s", rec.Body.String())
	}
}

// TestSection8HHTTP_DisabledFlagsRegisterNothing keeps both switches meaningful.
func TestSection8HHTTP_DisabledFlagsRegisterNothing(t *testing.T) {
	noIntelligence := allFlagsOn()
	noIntelligence.IntelligenceEnabled = false
	env := newTestEnv(t, noIntelligence)
	for _, route := range env.section8hRouter("vs_a", businessA).Routes() {
		if strings.Contains(route.Path, "/intelligence") {
			t.Errorf("intelligence route %s registered while the feature is off", route.Path)
		}
	}

	noExports := allFlagsOn()
	noExports.ExportsEnabled = false
	quiet := newTestEnv(t, noExports)
	for _, route := range quiet.section8hRouter("vs_a", businessA).Routes() {
		if strings.Contains(route.Path, "/exports") {
			t.Errorf("export route %s registered while the feature is off", route.Path)
		}
	}

	both := allFlagsOn()
	both.IntelligenceEnabled = false
	both.ExportsEnabled = false
	off := newTestEnv(t, both)
	if routes := off.section8hRouter("vs_a", businessA).Routes(); len(routes) != 0 {
		t.Fatalf("Section 8H registered %d routes while fully disabled", len(routes))
	}
}
