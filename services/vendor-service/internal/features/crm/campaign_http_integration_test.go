package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

// Section 8E HTTP contract tests. They drive the real production router, so the
// permission checks, the Idempotency-Key edge and the registered surface are
// exercised exactly as deployed rather than through the service API underneath.

func (e *testEnv) campaignRouter(staffID, business string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: staffID, VendorID: business, SessionID: "session"})
		c.Next()
	})
	RegisterCampaignRoutes(group, NewCampaignHandler(e.service, idempotency.NewStore(e.pool, 0)), e.service.FlagSet())
	return router
}

func campaignRequest(t *testing.T, router *gin.Engine, method, path, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestCampaignHTTP_RegisteredSurfaceIsExactlyTheDocumentedTen is the
// router-side half of the OpenAPI parity check, and the guard against Section
// 8E quietly growing a delivery endpoint.
func TestCampaignHTTP_RegisteredSurfaceIsExactlyTheDocumentedTen(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	router := env.campaignRouter("vs_a", businessA)

	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	want := []string{
		"GET /api/v1/vendor/crm/campaigns",
		"POST /api/v1/vendor/crm/campaigns",
		"GET /api/v1/vendor/crm/campaigns/:campaignId",
		"PATCH /api/v1/vendor/crm/campaigns/:campaignId",
		"POST /api/v1/vendor/crm/campaigns/:campaignId/archive",
		"POST /api/v1/vendor/crm/campaigns/:campaignId/preview",
		"POST /api/v1/vendor/crm/campaigns/:campaignId/prepare",
		"GET /api/v1/vendor/crm/campaigns/:campaignId/snapshots",
		"GET /api/v1/vendor/crm/campaigns/:campaignId/snapshots/:snapshotId",
		"GET /api/v1/vendor/crm/campaigns/:campaignId/snapshots/:snapshotId/targets",
	}
	if len(registered) != len(want) {
		t.Fatalf("registered %d campaign routes; want exactly %d: %v", len(registered), len(want), registered)
	}
	for _, route := range want {
		if !registered[route] {
			t.Errorf("route %s is not registered", route)
		}
	}

	// Nothing that would deliver, schedule or hand off a message exists.
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router = env.campaignRouter("vs_a", businessA)
	for _, path := range []string{
		"/api/v1/vendor/crm/campaigns/crmcmp_1/send",
		"/api/v1/vendor/crm/campaigns/crmcmp_1/schedule",
		"/api/v1/vendor/crm/campaigns/crmcmp_1/deliver",
		"/api/v1/vendor/crm/campaigns/crmcmp_1/dispatch",
	} {
		if rec := campaignRequest(t, router, http.MethodPost, path, "k", map[string]any{}); rec.Code != http.StatusNotFound {
			t.Errorf("%s exists (status=%d); Section 8E must expose no delivery route", path, rec.Code)
		}
	}
}

// TestCampaignHTTP_EachRouteRequiresItsOwnPermission proves the five
// permissions are genuinely distinct — in particular that reading a campaign
// does not let a caller list the customers inside its audience.
func TestCampaignHTTP_EachRouteRequiresItsOwnPermission(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_http"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("HTTP campaign", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	base := "/api/v1/vendor/crm/campaigns"
	type call struct {
		method, path, key string
		body              any
		permission        string
	}
	calls := []call{
		{http.MethodGet, base, "", nil, PermCampaignsRead},
		{http.MethodGet, base + "/" + campaign.ID, "", nil, PermCampaignsRead},
		{http.MethodGet, base + "/" + campaign.ID + "/snapshots", "", nil, PermCampaignsRead},
		{http.MethodGet, base + "/" + campaign.ID + "/snapshots/" + snapshot.ID, "", nil, PermCampaignsRead},
		{http.MethodGet, base + "/" + campaign.ID + "/snapshots/" + snapshot.ID + "/targets", "", nil, PermCampaignTargetsRead},
		{http.MethodPost, base + "/" + campaign.ID + "/preview", "", nil, PermCampaignsPreview},
		{http.MethodPost, base, "k_create", map[string]any{
			"name": "Another campaign", "channels": []string{"EMAIL"}, "source_segment_ref": audience.ID},
			PermCampaignsManage},
		{http.MethodPost, base + "/" + campaign.ID + "/prepare", "k_prepare", map[string]any{}, PermCampaignsPrepare},
	}

	// Holding nothing denies everything.
	env.service.scopes = fixedScopes{}
	router := env.campaignRouter("vs_a", businessA)
	for _, c := range calls {
		rec := campaignRequest(t, router, c.method, c.path, c.key+"_none", c.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with no permission: status=%d want 403", c.method, c.path, rec.Code)
		}
	}

	// Holding only read allows the four reads and nothing else. Listing the
	// customers inside a snapshot is deliberately not one of them.
	env.service.scopes = permissionScopes{PermCampaignsRead: {Global: true}}
	router = env.campaignRouter("vs_a", businessA)
	for _, c := range calls {
		rec := campaignRequest(t, router, c.method, c.path, c.key+"_read", c.body)
		if c.permission == PermCampaignsRead {
			if rec.Code != http.StatusOK {
				t.Errorf("%s %s with read: status=%d body=%s", c.method, c.path, rec.Code, rec.Body.String())
			}
			continue
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with only read: status=%d want 403", c.method, c.path, rec.Code)
		}
	}

	// Preview does not imply prepare: evaluating an audience and freezing one
	// are different authorities.
	env.service.scopes = permissionScopes{PermCampaignsPreview: {Global: true}}
	router = env.campaignRouter("vs_a", businessA)
	if rec := campaignRequest(t, router, http.MethodPost, base+"/"+campaign.ID+"/preview", "", nil); rec.Code != http.StatusOK {
		t.Errorf("preview with the preview permission: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := campaignRequest(t, router, http.MethodPost, base+"/"+campaign.ID+"/prepare", "k_prev_only",
		map[string]any{}); rec.Code != http.StatusForbidden {
		t.Errorf("prepare with only the preview permission: status=%d want 403", rec.Code)
	}
}

// TestCampaignHTTP_TargetListCarriesNoContactData is the disclosure test on the
// wire shape itself: whatever a client asks for, the response cannot contain a
// contact detail, because no field on it could hold one.
func TestCampaignHTTP_TargetListCarriesNoContactData(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_pii"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("PII check", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.campaignRouter("vs_a", businessA)
	rec := campaignRequest(t, router, http.MethodGet,
		"/api/v1/vendor/crm/campaigns/"+campaign.ID+"/snapshots/"+snapshot.ID+"/targets", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list targets: status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Field names, not values: EMAIL is a legitimate channel name, while an
	// `"email"` key or an "@" anywhere would mean a contact detail had reached
	// the wire.
	for _, forbidden := range []string{
		`"email":`, `"phone":`, `"phone_number":`, `"address":`, `"device_token":`, `"push_token":`,
		`"first_name":`, `"last_name":`, "@",
	} {
		if containsFold(body, forbidden) {
			t.Fatalf("the target list mentions %q: %s", forbidden, body)
		}
	}
	// The recheck obligation is restated on every page, so a client paging a
	// large audience cannot lose sight of it.
	if !containsFold(body, "send_time_recheck_required") {
		t.Fatalf("the target list does not restate the send-time recheck: %s", body)
	}

	var envelope struct {
		Data struct {
			Targets []map[string]any `json:"targets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Targets) != 1 {
		t.Fatalf("expected one target, got %d", len(envelope.Data.Targets))
	}
	if _, present := envelope.Data.Targets[0]["profile_ref"]; !present {
		t.Fatal("a target must carry its opaque relationship reference")
	}
}

// TestCampaignHTTP_CommandsAreIdempotentAtTheEdge checks the shared
// Idempotency-Key contract: the same key and body replays, and the same key
// with a different body is a conflict.
func TestCampaignHTTP_CommandsAreIdempotentAtTheEdge(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone")
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.campaignRouter("vs_a", businessA)

	body := map[string]any{"name": "Idempotent", "channels": []string{"EMAIL"}, "source_segment_ref": audience.ID}
	first := campaignRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/campaigns", "key-1", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", first.Code, first.Body.String())
	}
	replay := campaignRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/campaigns", "key-1", body)
	if replay.Code != http.StatusCreated || replay.Body.String() != first.Body.String() {
		t.Fatalf("a replay should return the original response: status=%d", replay.Code)
	}
	// Only one campaign was created.
	var campaigns int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*) FROM crm_campaigns WHERE business_id=$1`,
		businessA).Scan(&campaigns); err != nil {
		t.Fatal(err)
	}
	if campaigns != 1 {
		t.Fatalf("a replayed create produced %d campaigns", campaigns)
	}

	different := map[string]any{"name": "Different", "channels": []string{"EMAIL"}, "source_segment_ref": audience.ID}
	conflict := campaignRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/campaigns", "key-1", different)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("reusing a key with a different body should conflict: status=%d body=%s",
			conflict.Code, conflict.Body.String())
	}

	// A command without a key at all is refused, so idempotency is not opt-in.
	missing := campaignRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/campaigns", "", different)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("a command without an Idempotency-Key should be refused: status=%d", missing.Code)
	}
}

// TestCampaignHTTP_DisabledFlagRegistersNothing keeps the rollout switch
// meaningful: with campaigns off the surface does not exist at all.
func TestCampaignHTTP_DisabledFlagRegistersNothing(t *testing.T) {
	flags := allFlagsOn()
	flags.CampaignsEnabled = false
	env := newTestEnv(t, flags)
	router := env.campaignRouter("vs_a", businessA)
	if len(router.Routes()) != 0 {
		t.Fatalf("campaigns registered %d routes while disabled", len(router.Routes()))
	}
}

// TestCampaignHTTP_PreparationCanBeFrozenWithoutHidingPreparedAudiences proves
// the second flag is a brake on new work only. Freezing preparation must never
// make an audience that already exists look empty.
func TestCampaignHTTP_PreparationCanBeFrozenWithoutHidingPreparedAudiences(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_1"))
	env.allowMarketing(t, businessA, "vs_a", profile.ID, ChannelEmail, 1)
	audience := env.staticAudience(t, businessA, "vs_a", "Everyone", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Frozen prep", audience.ID))
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)

	env.service.flags.CampaignPreparationEnabled = false
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.campaignRouter("vs_a", businessA)

	if rec := campaignRequest(t, router, http.MethodPost,
		"/api/v1/vendor/crm/campaigns/"+campaign.ID+"/prepare", "k", map[string]any{}); rec.Code == http.StatusAccepted {
		t.Fatal("a new preparation was accepted while preparation is frozen")
	}
	rec := campaignRequest(t, router, http.MethodGet,
		"/api/v1/vendor/crm/campaigns/"+campaign.ID+"/snapshots/"+snapshot.ID, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("an already-prepared audience became unreadable: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !containsFold(rec.Body.String(), "\"target_count\":1") {
		t.Fatalf("the prepared audience lost its size: %s", rec.Body.String())
	}
}

// containsFold is a case-insensitive substring check, used so a disclosure
// assertion cannot be defeated by a change of casing in a JSON key.
func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
