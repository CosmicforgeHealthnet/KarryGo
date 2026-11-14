package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	assetsfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	dispatchfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	workforcefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/queryallow"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// fixedMembershipResolver resolves every bearer subject to one seeded member,
// so the conventions tests drive the production router (BuildRouter: bearer
// auth, tenant resolution, lifecycle gate, query allowlist, every feature's
// real handler and service over PostgreSQL) without an OTP round trip.
type fixedMembershipResolver struct{ membership tenant.Membership }

func (r fixedMembershipResolver) ResolveMembership(context.Context, string) (tenant.Membership, error) {
	return r.membership, nil
}

type conventionsHarness struct {
	pool   *pgxpool.Pool
	router *gin.Engine
	token  string
	org    string
	member string
}

// allFeaturesConfig switches every feature flag on (workers stay off), so each
// list route answers from its real service instead of a disabled-feature 404.
func allFeaturesConfig() config.Config {
	cfg := config.Config{AppEnv: "test", MaxRequestBodyBytes: 32 << 20}
	v := reflect.ValueOf(&cfg).Elem()
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		if field.Type.Kind() == reflect.Bool && strings.HasSuffix(field.Name, "Enabled") &&
			!strings.Contains(field.Name, "Worker") {
			v.Field(i).SetBool(true)
		}
	}
	return cfg
}

func newConventionsHarness(t *testing.T) *conventionsHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	pool := testPool(t)
	org := createOrganization(t, pool, "Conventions "+randomHex()[:6], "RC-CONV-"+randomHex()[:8])
	if _, err := pool.Exec(context.Background(), `UPDATE fleet_partner_organizations SET status='ACTIVE' WHERE id=$1`, org.ID); err != nil {
		t.Fatal(err)
	}
	member := seedAuditRouteUser(t, pool, org.ID, staffmodels.SystemOwnerRoleID)

	cfg := allFeaturesConfig()
	// A configured (but empty) Wallet authority, so the finance operator
	// routes are served by their real service and read the local projection
	// tables instead of answering "integration not available".
	wallet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":"not_found","message":"not found"}}`))
	}))
	t.Cleanup(wallet.Close)
	cfg.HTTPClientTimeout = 2 * time.Second
	cfg.Clients = map[string]config.ClientConfig{
		"payment-wallet-service": {BaseURL: wallet.URL, Secret: []byte("conventions-wallet-secret")},
	}
	components := app.BuildComponents(cfg, pool, nil, observability.NewMetrics())
	_, _, tenantRoutes := components.Registrars(cfg)
	secret := []byte("conventions-access-secret")
	router := app.BuildRouter(app.Options{
		Metrics:            observability.NewMetrics(),
		Config:             cfg,
		TokenSigner:        auth.NewTokenSigner(secret),
		Resolver:           fixedMembershipResolver{tenant.Membership{OrganizationID: org.ID, OrganizationUserID: member, Status: tenant.MembershipStatusActive, RoleVersion: 1}},
		OrganizationStatus: app.OrganizationStatusFromPool(pool),
		TenantRoutes:       tenantRoutes,
	})
	token, err := auth.NewTokenSigner(secret).Sign(auth.Claims{
		Subject: "cid_conventions", Role: config.CorporateRole, Service: config.ServiceName,
		SessionID: "ses_conventions", ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &conventionsHarness{pool: pool, router: router, token: token, org: org.ID, member: member}
}

func (h *conventionsHarness) get(t *testing.T, target string) (int, map[string]any, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("X-Request-ID", "req-conventions")
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, req)
	var body map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder.Code, body, recorder.Body.String()
}

// tenantGETRoutes lists every GET route the production router mounts on the
// corporate tenant surface.
func tenantGETRoutes(router *gin.Engine) []string {
	var out []string
	for _, route := range router.Routes() {
		if route.Method != http.MethodGet || !strings.HasPrefix(route.Path, queryallow.TenantPrefix+"/") {
			continue
		}
		if strings.HasPrefix(route.Path, queryallow.TenantPrefix+"/meta") {
			continue
		}
		out = append(out, route.Path)
	}
	sort.Strings(out)
	return out
}

func isCollectionRoute(path string) bool {
	segments := strings.Split(path, "/")
	return !strings.HasPrefix(segments[len(segments)-1], ":")
}

func concrete(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") {
			segments[i] = "unknown_" + strings.TrimLeft(segment, ":*")
		}
	}
	return strings.Join(segments, "/")
}

func validationFields(body map[string]any) []string {
	envelope, _ := body["error"].(map[string]any)
	details, _ := envelope["details"].([]any)
	var out []string
	for _, detail := range details {
		if field, ok := detail.(map[string]any)["field"].(string); ok {
			out = append(out, field)
		}
	}
	sort.Strings(out)
	return out
}

// TestTenantListRoutesRejectUnknownQueryAndBadPaging probes every tenant GET
// route the production router mounts (spec 6 and 36; audit defects 5 and 6).
//
//   - Every collection/singleton route (last segment static) is declared in
//     internal/platform/queryallow, so its filters are explicit.
//   - offset=-1&limit=100000 never produces a 5xx: before this change
//     GET /assets?offset=-1 reached PostgreSQL and returned HTTP 500.
//   - On declared routes, an unknown filter and invalid paging are refused
//     with 422 naming the parameter, and the error envelope carries the
//     caller's request id.
func TestTenantListRoutesRejectUnknownQueryAndBadPaging(t *testing.T) {
	h := newConventionsHarness(t)
	routes := tenantGETRoutes(h.router)
	if len(routes) < 250 {
		t.Fatalf("only %d tenant GET routes mounted; expected the full surface", len(routes))
	}
	declared := map[string]bool{}
	for _, key := range queryallow.Declared() {
		declared[strings.TrimPrefix(key, "GET ")] = true
	}
	var undeclared []string
	for _, path := range routes {
		if isCollectionRoute(path) && !declared[path] {
			undeclared = append(undeclared, path)
		}
		delete(declared, path)
	}
	if len(undeclared) > 0 {
		t.Fatalf("collection routes missing from internal/platform/queryallow: %v", undeclared)
	}
	if len(declared) > 0 {
		stale := make([]string, 0, len(declared))
		for path := range declared {
			stale = append(stale, path)
		}
		sort.Strings(stale)
		t.Fatalf("queryallow declares routes the router does not mount: %v", stale)
	}

	probed := 0
	for _, path := range routes {
		target := concrete(path)
		code, body, raw := h.get(t, target+"?offset=-1&limit=100000")
		if code >= 500 {
			t.Errorf("GET %s?offset=-1&limit=100000 = %d: %s", target, code, raw)
			continue
		}
		probed++
		if !isCollectionRoute(path) {
			continue
		}
		// Both parameters are refused either way: as out of range where the
		// route pages by them, or as unknown where it does not.
		schema, _ := queryallow.Lookup(http.MethodGet, path)
		want := []string{"limit", "offset"}
		if code != http.StatusUnprocessableEntity || !reflect.DeepEqual(validationFields(body), want) {
			t.Errorf("GET %s?offset=-1&limit=100000 = %d %v, want 422 naming %v", path, code, validationFields(body), want)
		}
		envelope, _ := body["error"].(map[string]any)
		if envelope["requestId"] != "req-conventions" {
			t.Errorf("GET %s: error envelope lacks the request id: %s", path, raw)
		}
		if code, body, raw := h.get(t, target+"?bogusFilter=1"); code != http.StatusUnprocessableEntity ||
			!reflect.DeepEqual(validationFields(body), []string{"bogusFilter"}) {
			t.Errorf("GET %s?bogusFilter=1 = %d %s, want 422", path, code, raw)
		}
		if _, ok := schema.Fields["offset"]; ok {
			if code, _, raw := h.get(t, target+"?offset=10001"); code != http.StatusUnprocessableEntity {
				t.Errorf("GET %s?offset=10001 = %d %s, want 422", path, code, raw)
			}
		}
	}
	if probed != len(routes) {
		t.Fatalf("probed %d of %d routes without a 5xx", probed, len(routes))
	}
}

// TestOffsetListsServeValidPagesAfterValidation proves the validation does
// not stop legitimate offset paging: page two of the asset list holds exactly
// the rows after page one.
func TestOffsetListsServeValidPagesAfterValidation(t *testing.T) {
	h := newConventionsHarness(t)
	ctx := context.Background()
	branch := seedBranch(t, h.pool, h.org, "Assets "+randomHex()[:6])
	for i := 0; i < 3; i++ {
		if _, err := h.pool.Exec(ctx, `INSERT INTO fleet_partner_assets
			(id, organization_id, branch_id, asset_code, asset_type, ownership_type, created_at)
			VALUES ($1, $2, $3, $4, 'VEHICLE', 'OWNED', now() - make_interval(mins => $5))`,
			ids.New("ast"), h.org, branch, fmt.Sprintf("CONV-%d-%s", i, randomHex()[:6]), i); err != nil {
			t.Fatalf("seed asset: %v", err)
		}
	}
	code, first, raw := h.get(t, "/fleet-partner/v1/assets?limit=2&offset=0")
	if code != http.StatusOK {
		t.Fatalf("page one = %d %s", code, raw)
	}
	code, second, raw := h.get(t, "/fleet-partner/v1/assets?limit=2&offset=2")
	if code != http.StatusOK {
		t.Fatalf("page two = %d %s", code, raw)
	}
	a, _ := first["data"].([]any)
	b, _ := second["data"].([]any)
	if len(a) != 2 || len(b) != 1 {
		t.Fatalf("pages = %d + %d, want 2 + 1", len(a), len(b))
	}
	for _, x := range a {
		if x.(map[string]any)["id"] == b[0].(map[string]any)["id"] {
			t.Fatal("offset page repeated a row")
		}
	}
	if meta, _ := first["meta"].(map[string]any); meta["requestId"] != "req-conventions" {
		t.Fatalf("success envelope meta = %v", first["meta"])
	}
	// Defence in depth: a caller that bypasses HTTP (worker, future handler)
	// cannot reach PostgreSQL with a negative OFFSET either; the services
	// clamp it, which used to surface as HTTP 500.
	scope := tenant.Context{OrganizationID: h.org, OrganizationUserID: h.member,
		ActorType: tenant.ActorTypeOrganizationUser, ActorID: h.member}
	if rows, err := assetsfeature.NewService(assetsfeature.Options{Pool: h.pool, Flags: assetsfeature.Flags{Assets: true, Inspections: true}}).
		ListAssets(ctx, scope, assetsfeature.Page{Offset: -1, Limit: -7}); err != nil || len(rows) != 3 {
		t.Fatalf("asset service with offset -1 = %d rows, %v", len(rows), err)
	}
	if _, err := workforcefeature.NewService(workforcefeature.Options{Pool: h.pool, Flags: workforcefeature.Flags{Workforce: true, Shifts: true, Leave: true, Training: true}}).
		ListProfiles(ctx, scope, workforcefeature.Page{Offset: -1}); err != nil {
		t.Fatalf("workforce service with offset -1: %v", err)
	}
	if _, err := dispatchfeature.NewService(dispatchfeature.Options{Pool: h.pool, Flags: dispatchfeature.Flags{DispatchPlanning: true, RoutePlanning: true, Geofences: true, OperationalAlerts: true}}).
		ListAlerts(ctx, scope, dispatchfeature.PageFilter{Offset: -1}); err != nil {
		t.Fatalf("dispatch service with offset -1: %v", err)
	}
	for _, target := range []string{"/fleet-partner/v1/assets?offset=-1", "/fleet-partner/v1/assets?limit=abc",
		"/fleet-partner/v1/workforce/profiles?offset=-5", "/fleet-partner/v1/dispatch/plans?offset=-1",
		"/fleet-partner/v1/geofence-events?offset=-1", "/fleet-partner/v1/rescue-cases?limit=-1"} {
		if code, _, raw := h.get(t, target); code != http.StatusUnprocessableEntity {
			t.Fatalf("%s = %d %s, want 422", target, code, raw)
		}
	}
}

// TestStaffDirectoryPagesWithCursor covers audit defect 6 / FLEET-9.1: the
// staff list stopped at 200 rows with no cursor. It now pages by keyset on
// (created_at DESC, id DESC) and returns meta.nextCursor while rows remain.
func TestStaffDirectoryPagesWithCursor(t *testing.T) {
	h := newConventionsHarness(t)
	expected := map[string]bool{h.member: true}
	for i := 0; i < 4; i++ {
		identity := seedIdentity(t, h.pool, "staffpage-"+randomHex()[:8]+"@example.com", "+23480"+randomHex()[:8])
		expected[seedMember(t, h.pool, h.org, identity, false, nil).ID] = true
	}
	// Equal creation timestamps: only the id tie-breaker keeps pages disjoint.
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE fleet_partner_organization_users SET created_at = '2026-01-01T00:00:00Z' WHERE organization_id = $1`, h.org); err != nil {
		t.Fatal(err)
	}
	other := createOrganization(t, h.pool, "Other staff org "+randomHex()[:6], "RC-STAFF-"+randomHex()[:8])
	foreign := seedMember(t, h.pool, other.ID, seedIdentity(t, h.pool, "foreign-"+randomHex()[:8]+"@example.com", "+23480"+randomHex()[:8]), false, nil)

	seen := map[string]bool{}
	next := ""
	for page := 0; page < 10; page++ {
		target := "/fleet-partner/v1/staff?limit=2"
		if next != "" {
			target += "&cursor=" + next
		}
		code, body, raw := h.get(t, target)
		if code != http.StatusOK {
			t.Fatalf("staff page %d = %d %s", page, code, raw)
		}
		for _, item := range body["data"].([]any) {
			id := item.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("staff member %s repeated across pages", id)
			}
			seen[id] = true
		}
		meta, _ := body["meta"].(map[string]any)
		next, _ = meta["nextCursor"].(string)
		if next == "" {
			break
		}
	}
	if !reflect.DeepEqual(seen, expected) || seen[foreign.ID] {
		t.Fatalf("paged staff %v, want %v", seen, expected)
	}
	// No parameters: the whole directory in one page, no cursor (unchanged
	// behaviour for directories of up to 200 members).
	code, body, raw := h.get(t, "/fleet-partner/v1/staff")
	if meta, _ := body["meta"].(map[string]any); code != http.StatusOK || len(body["data"].([]any)) != 5 || meta["nextCursor"] != nil {
		t.Fatalf("unpaged staff = %d %s", code, raw)
	}
	for _, target := range []string{"/fleet-partner/v1/staff?limit=201", "/fleet-partner/v1/staff?cursor=not-a-cursor",
		"/fleet-partner/v1/staff?offset=0", "/fleet-partner/v1/staff?limit=0"} {
		if code, _, raw := h.get(t, target); code != http.StatusUnprocessableEntity {
			t.Fatalf("%s = %d %s, want 422", target, code, raw)
		}
	}
}

// TestProviderHistoryPagesWithCursor covers audit defect 6: membership
// history was an unbounded query. It now pages like the provider list.
func TestProviderHistoryPagesWithCursor(t *testing.T) {
	h := newConventionsHarness(t)
	ctx := context.Background()
	providerID := "prv_conv_" + randomHex()
	branch := seedBranch(t, h.pool, h.org, "Conv "+randomHex()[:6])
	membership := &providermodels.Membership{
		ProviderService: "driver-dispatch-delivery-service", ServiceCategory: providermodels.CategoryDispatch,
		OnboardingSource: providermodels.SourceSponsored, RelationshipType: providermodels.RelationshipEmployee,
		Status: providermodels.StatusActive, ProviderID: &providerID, HomeBranchID: &branch,
	}
	if err := providerrepos.NewRepository(h.pool).CreateMembership(ctx, h.pool, h.org, membership); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for i := 0; i < 5; i++ {
		id := ids.New("pmh")
		want[id] = true
		if _, err := h.pool.Exec(ctx, `INSERT INTO fleet_partner_provider_membership_history
			(id, organization_id, membership_id, from_status, to_status, actor_type, actor_id,
			 reason_code, request_id, correlation_id, occurred_at)
			VALUES ($1, $2, $3, 'ACTIVE', 'ACTIVE', 'USER', 'oru_conv', 'TEST', 'req', 'cor', '2026-02-01T00:00:00Z')`,
			id, h.org, membership.ID); err != nil {
			t.Fatalf("seed history: %v", err)
		}
	}
	seen := map[string]bool{}
	next := ""
	for page := 0; page < 10; page++ {
		target := "/fleet-partner/v1/providers/" + membership.ID + "/history?limit=2"
		if next != "" {
			target += "&cursor=" + next
		}
		code, body, raw := h.get(t, target)
		if code != http.StatusOK {
			t.Fatalf("history page %d = %d %s", page, code, raw)
		}
		for _, item := range body["data"].([]any) {
			id := item.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("history entry %s repeated", id)
			}
			seen[id] = true
		}
		meta, _ := body["meta"].(map[string]any)
		next, _ = meta["nextCursor"].(string)
		if next == "" {
			break
		}
	}
	for id := range want {
		if !seen[id] {
			t.Fatalf("history entry %s never paged (saw %d)", id, len(seen))
		}
	}
	if code, _, _ := h.get(t, "/fleet-partner/v1/providers/opm_unknown/history"); code != http.StatusNotFound {
		t.Fatalf("unknown membership history = %d, want 404", code)
	}
	if code, _, raw := h.get(t, "/fleet-partner/v1/providers/"+membership.ID+"/history?limit=500"); code != http.StatusUnprocessableEntity {
		t.Fatalf("history limit 500 = %d %s", code, raw)
	}
}

// TestFormerEnvelopeBypassReturnsStandardMeta covers audit defect 10 on a
// route that used to write {"data": ...} with no meta: every corporate
// success response carries meta.requestId.
func TestFormerEnvelopeBypassReturnsStandardMeta(t *testing.T) {
	h := newConventionsHarness(t)
	code, body, raw := h.get(t, "/fleet-partner/v1/specialized/capabilities")
	if code != http.StatusOK {
		t.Fatalf("specialized capabilities = %d %s", code, raw)
	}
	meta, _ := body["meta"].(map[string]any)
	if _, ok := body["data"]; !ok || meta["requestId"] != "req-conventions" {
		t.Fatalf("envelope = %s", raw)
	}
}
