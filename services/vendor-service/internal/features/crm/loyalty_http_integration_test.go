package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

// Section 8D HTTP contract tests. These drive the real production router so
// the permission checks, the Idempotency-Key edge and the wire shapes are
// exercised exactly as deployed, not through the service API underneath them.

func (e *testEnv) loyaltyRouter(staffID, business string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: staffID, VendorID: business, SessionID: "session"})
		c.Next()
	})
	RegisterLoyaltyRoutes(group, NewLoyaltyHandler(e.service, idempotency.NewStore(e.pool, 0)), e.service.FlagSet())
	return router
}

func loyaltyRequest(t *testing.T, router *gin.Engine, method, path, key string, body any) *httptest.ResponseRecorder {
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

// TestLoyaltyHTTP_EachRouteRequiresItsOwnPermission proves the four
// permissions are genuinely distinct: read cannot configure, configure cannot
// adjust, and adjust cannot reverse.
func TestLoyaltyHTTP_EachRouteRequiresItsOwnPermission(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_http_perm"))
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	entry := env.credit(t, businessA, "vs_a", profile, 100, "ord_http_perm")

	base := "/api/v1/vendor/crm"
	type call struct {
		method, path, key string
		body              any
		permission        string
	}
	calls := []call{
		{http.MethodGet, base + "/loyalty/program", "", nil, PermLoyaltyRead},
		{http.MethodGet, base + "/customers/" + profile.ID + "/loyalty", "", nil, PermLoyaltyRead},
		{http.MethodGet, base + "/customers/" + profile.ID + "/loyalty/entries", "", nil, PermLoyaltyRead},
		{http.MethodPatch, base + "/loyalty/program", "k1",
			map[string]any{"status": "PAUSED", "expected_version": 1}, PermLoyaltyManage},
		{http.MethodPost, base + "/customers/" + profile.ID + "/loyalty/adjustments", "k2",
			map[string]any{"direction": "CREDIT", "points": 10, "reason": "goodwill"}, PermLoyaltyAdjust},
		{http.MethodPost, base + "/loyalty/entries/" + entry.ID + "/reverse", "k3",
			map[string]any{"reason": "posted in error"}, PermLoyaltyReverse},
	}

	// Holding nothing denies everything.
	env.service.scopes = fixedScopes{}
	router := env.loyaltyRouter("vs_a", businessA)
	for _, c := range calls {
		rec := loyaltyRequest(t, router, c.method, c.path, c.key+"_none", c.body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with no permission: status=%d want 403", c.method, c.path, rec.Code)
		}
	}

	// Holding only read denies every command but allows the three reads.
	env.service.scopes = permissionScopes{PermLoyaltyRead: {Global: true}}
	router = env.loyaltyRouter("vs_a", businessA)
	for _, c := range calls {
		rec := loyaltyRequest(t, router, c.method, c.path, c.key+"_read", c.body)
		if c.permission == PermLoyaltyRead {
			if rec.Code != http.StatusOK {
				t.Errorf("%s %s with read: status=%d body=%s", c.method, c.path, rec.Code, rec.Body.String())
			}
			continue
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with only read: status=%d want 403", c.method, c.path, rec.Code)
		}
	}

	// Adjust does not imply reverse.
	env.service.scopes = permissionScopes{PermLoyaltyRead: {Global: true}, PermLoyaltyAdjust: {Global: true}}
	router = env.loyaltyRouter("vs_a", businessA)
	rec := loyaltyRequest(t, router, http.MethodPost, base+"/loyalty/entries/"+entry.ID+"/reverse", "k_adj_only",
		map[string]any{"reason": "posted in error"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("reverse with only the adjust permission: status=%d want 403", rec.Code)
	}
	rec = loyaltyRequest(t, router, http.MethodPost, base+"/customers/"+profile.ID+"/loyalty/adjustments", "k_adj_ok",
		map[string]any{"direction": "CREDIT", "points": 10, "reason": "goodwill"})
	if rec.Code != http.StatusCreated {
		t.Errorf("adjust with the adjust permission: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// permissionScopes resolves per-permission rather than per-staff, so a test
// can hold one loyalty permission and not another.
type permissionScopes map[string]Scope

func (p permissionScopes) ResolveScope(_ context.Context, _, permission string) (Scope, error) {
	return p[permission], nil
}

// TestLoyaltyHTTP_AdjustmentIsIdempotentAtTheEdge checks the shared
// Idempotency-Key contract: same key and body replays, same key with a
// different body conflicts.
func TestLoyaltyHTTP_AdjustmentIsIdempotentAtTheEdge(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_http_idem"))
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.loyaltyRouter("vs_a", businessA)
	path := "/api/v1/vendor/crm/customers/" + profile.ID + "/loyalty/adjustments"
	body := map[string]any{"direction": "CREDIT", "points": 250, "reason": "service recovery"}

	first := loyaltyRequest(t, router, http.MethodPost, path, "idem_http_1", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", first.Code, first.Body.String())
	}
	replay := loyaltyRequest(t, router, http.MethodPost, path, "idem_http_1", body)
	if replay.Code != http.StatusCreated || replay.Body.String() != first.Body.String() {
		t.Fatalf("replay status=%d body=%s; want the stored response replayed", replay.Code, replay.Body.String())
	}
	if account := env.loyaltyAccount(t, businessA, profile.ID); account.CurrentPoints != 250 {
		t.Fatalf("balance = %d after a replayed adjustment; want 250", account.CurrentPoints)
	}

	// Same key, different request: a conflict, never a second grant.
	changed := loyaltyRequest(t, router, http.MethodPost, path, "idem_http_1",
		map[string]any{"direction": "CREDIT", "points": 9999, "reason": "service recovery"})
	if changed.Code != http.StatusConflict {
		t.Fatalf("reused key with a different body: status=%d want 409", changed.Code)
	}
	if account := env.loyaltyAccount(t, businessA, profile.ID); account.CurrentPoints != 250 {
		t.Fatalf("balance = %d after a conflicting replay", account.CurrentPoints)
	}
	// A missing key is refused outright.
	if rec := loyaltyRequest(t, router, http.MethodPost, path, "", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing Idempotency-Key: status=%d want 400", rec.Code)
	}
}

// TestLoyaltyHTTP_ResponsesCarryNoPIIOrCommandKeys is the wire-level
// disclosure guard.
func TestLoyaltyHTTP_ResponsesCarryNoPIIOrCommandKeys(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_http_pii"))
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, intPtr(30))
	env.credit(t, businessA, "vs_a", profile, 400, "ord_http_pii")
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.loyaltyRouter("vs_a", businessA)

	adjust := loyaltyRequest(t, router, http.MethodPost,
		"/api/v1/vendor/crm/customers/"+profile.ID+"/loyalty/adjustments", "idem_http_pii",
		map[string]any{"direction": "DEBIT", "points": 100, "reason": "customer disputed the award"})
	if adjust.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", adjust.Code, adjust.Body.String())
	}

	for _, rec := range []*httptest.ResponseRecorder{
		adjust,
		loyaltyRequest(t, router, http.MethodGet, "/api/v1/vendor/crm/customers/"+profile.ID+"/loyalty", "", nil),
		loyaltyRequest(t, router, http.MethodGet, "/api/v1/vendor/crm/customers/"+profile.ID+"/loyalty/entries", "", nil),
	} {
		body := rec.Body.String()
		for _, forbidden := range []string{"idem_http_pii", "customer disputed", "idempotency", "cus_http_pii", "@"} {
			if bytes.Contains([]byte(body), []byte(forbidden)) {
				t.Errorf("response leaked %q: %s", forbidden, body)
			}
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"email", "phone", "address", "date_of_birth", "password",
			"currency", "amount_minor", "wallet"} {
			if keyExistsDeep(payload, forbidden) {
				t.Errorf("loyalty response exposed a %q field: %s", forbidden, body)
			}
		}
	}
}

// TestLoyaltyHTTP_LedgerPageShapeAndReversalMarking checks the history wire
// contract, including that a reversed entry is marked as such so a screen does
// not have to infer it from neighbouring rows.
func TestLoyaltyHTTP_LedgerPageShapeAndReversalMarking(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_http_ledger"))
	env.seedLoyaltyProgram(t, businessA, "vs_a", LoyaltyProgramActive, nil)
	credited := env.credit(t, businessA, "vs_a", profile, 100, "ord_http_ledger")
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.loyaltyRouter("vs_a", businessA)

	rec := loyaltyRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/loyalty/entries/"+credited.ID+"/reverse",
		"idem_http_rev", map[string]any{"reason": "credited the wrong customer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("reverse status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = loyaltyRequest(t, router, http.MethodGet,
		"/api/v1/vendor/crm/customers/"+profile.ID+"/loyalty/entries?limit=10", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data struct {
			Entries []LoyaltyEntryView `json:"entries"`
			HasMore bool               `json:"has_more"`
			Limit   int                `json:"limit"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.Entries) != 2 || payload.Data.HasMore || payload.Data.Limit != 10 {
		t.Fatalf("ledger page = %+v; want two entries on a limit-10 page", payload.Data)
	}
	var original, reversal LoyaltyEntryView
	for _, entry := range payload.Data.Entries {
		if entry.EntryRef == credited.ID {
			original = entry
		} else {
			reversal = entry
		}
	}
	if original.ReversedByRef == nil || *original.ReversedByRef != reversal.EntryRef {
		t.Fatalf("the reversed entry is not marked: %+v", original)
	}
	if reversal.ReversesEntryRef == nil || *reversal.ReversesEntryRef != credited.ID {
		t.Fatalf("the reversal does not point at its original: %+v", reversal)
	}
	if reversal.DeltaPoints != -100 || reversal.BalanceAfter != 0 {
		t.Fatalf("reversal wire values = %+v; want -100 to a zero balance", reversal)
	}
}

// TestLoyaltyHTTP_NoEarnOrRedeemRouteExists pins the deliberate omission: the
// engines are internal primitives, and no vendor-callable endpoint lets a
// caller name its own points total.
func TestLoyaltyHTTP_NoEarnOrRedeemRouteExists(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_http_noearn"))
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	router := env.loyaltyRouter("vs_a", businessA)

	for _, path := range []string{
		"/api/v1/vendor/crm/customers/" + profile.ID + "/loyalty/earn",
		"/api/v1/vendor/crm/customers/" + profile.ID + "/loyalty/redeem",
		"/api/v1/vendor/crm/loyalty/earn",
		"/api/v1/vendor/crm/loyalty/redeem",
		"/internal/vendor/v1/loyalty",
	} {
		if rec := loyaltyRequest(t, router, http.MethodPost, path, "k", map[string]any{"points": 10}); rec.Code != http.StatusNotFound {
			t.Errorf("%s exists (status=%d); Section 8D must expose no arbitrary earn or redeem route", path, rec.Code)
		}
	}

	// The registered set is exactly the six documented routes.
	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	want := []string{
		"GET /api/v1/vendor/crm/loyalty/program",
		"PATCH /api/v1/vendor/crm/loyalty/program",
		"POST /api/v1/vendor/crm/loyalty/entries/:entryId/reverse",
		"GET /api/v1/vendor/crm/customers/:profileId/loyalty",
		"GET /api/v1/vendor/crm/customers/:profileId/loyalty/entries",
		"POST /api/v1/vendor/crm/customers/:profileId/loyalty/adjustments",
	}
	if len(registered) != len(want) {
		t.Fatalf("registered %d loyalty routes; want exactly %d: %v", len(registered), len(want), registered)
	}
	for _, route := range want {
		if !registered[route] {
			t.Errorf("route %s is not registered", route)
		}
	}
}

// TestLoyaltyHTTP_DisabledFlagRegistersNothing keeps the rollout switch
// meaningful: with loyalty off the surface does not exist at all.
func TestLoyaltyHTTP_DisabledFlagRegistersNothing(t *testing.T) {
	flags := allFlagsOn()
	flags.LoyaltyEnabled = false
	env := newTestEnv(t, flags)
	router := env.loyaltyRouter("vs_a", businessA)
	if len(router.Routes()) != 0 {
		t.Fatalf("loyalty registered %d routes while disabled", len(router.Routes()))
	}
}
