package food

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

// buildRouter mounts the real registration helper behind a stub that installs
// the same authenticated actor the production middleware would. The middleware
// itself is Section 5's and is covered by cmd/http_contract_test.go; what is
// under test here is the command edge — idempotency, envelopes, status codes.
func (e *testEnv) buildRouter(t *testing.T, staffID, vendorID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	if staffID != "" {
		group.Use(func(c *gin.Context) {
			authctx.Set(c, authctx.StaffContext{StaffID: staffID, VendorID: vendorID, SessionID: "sess_food"})
			c.Next()
		})
	}
	handler := NewHandler(e.service, idempotency.NewStore(e.pool, time.Hour))
	RegisterRoutes(group, handler, e.service.FlagSet())
	return router
}

func (e *testEnv) do(t *testing.T, router *gin.Engine, method, path string, body any, key string) *httptest.ResponseRecorder {
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
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// TestHTTP_IdempotentCreateRetryDoesNotDuplicate is the Section 9A half of the
// shared command contract: same key and same request replays the stored
// response; same key with a different request is a conflict.
func TestHTTP_IdempotentCreateRetryDoesNotDuplicate(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	router := env.buildRouter(t, staffA, businessA)

	request := map[string]any{"branch_id": branchA1, "capability_type": "RESTAURANT", "name": "Main Menu"}
	first := env.do(t, router, http.MethodPost, "/api/v1/vendor/food/menus", request, "key-menu-1")
	if first.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", first.Code, first.Body.String())
	}
	second := env.do(t, router, http.MethodPost, "/api/v1/vendor/food/menus", request, "key-menu-1")
	if second.Code != http.StatusCreated || second.Body.String() != first.Body.String() {
		t.Fatalf("expected the retry to replay the stored response, got %d: %s", second.Code, second.Body.String())
	}
	menus, _, err := env.service.ListMenus(context.Background(), actorFor(businessA, staffA), globalScope(), MenuListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(menus) != 1 {
		t.Fatalf("the retry created a duplicate menu: %d exist", len(menus))
	}

	// The same key with a different body is a conflict, not a silent replay.
	changed := map[string]any{"branch_id": branchA1, "capability_type": "FAST_FOOD", "name": "Different"}
	conflict := env.do(t, router, http.MethodPost, "/api/v1/vendor/food/menus", changed, "key-menu-1")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a reused key with a different request, got %d: %s",
			conflict.Code, conflict.Body.String())
	}

	// A failed command releases the key so a genuine retry can run.
	invalid := map[string]any{"branch_id": branchA1, "capability_type": "CLOUD_KITCHEN", "name": "Bad"}
	if failed := env.do(t, router, http.MethodPost, "/api/v1/vendor/food/menus", invalid, "key-menu-2"); failed.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", failed.Code, failed.Body.String())
	}
	retried := env.do(t, router, http.MethodPost, "/api/v1/vendor/food/menus",
		map[string]any{"branch_id": branchA1, "capability_type": "FAST_FOOD", "name": "Quick Bites"}, "key-menu-2")
	if retried.Code != http.StatusCreated {
		t.Fatalf("expected the released key to be reusable, got %d: %s", retried.Code, retried.Body.String())
	}
}

// TestHTTP_EveryFoodRouteRequiresAuthentication counts what the production
// registration helper actually mounts, so the number cannot drift away from the
// code, and proves not one route answers without a session.
func TestHTTP_EveryFoodRouteRequiresAuthentication(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	router := env.buildRouter(t, "", "")

	checked := 0
	for _, route := range router.Routes() {
		if !strings.HasPrefix(route.Path, "/api/v1/vendor/food/") {
			continue
		}
		path := strings.NewReplacer(":menuId", "unknown", ":sectionId", "unknown", ":itemId", "unknown",
			":variantId", "unknown", ":groupId", "unknown", ":modifierId", "unknown").Replace(route.Path)
		recorder := env.do(t, router, route.Method, path, map[string]any{}, "auth-probe")
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without auth=%d body=%s", route.Method, path, recorder.Code, recorder.Body.String())
		}
		checked++
	}
	if checked != 39 {
		t.Fatalf("checked %d Section 9A routes, want 39", checked)
	}
}

// TestHTTP_RoutesAreAbsentWhenTheFeatureIsDisabled proves a disabled feature has
// no attack surface at all, rather than routes that answer 503 and thereby
// confirm the feature exists.
func TestHTTP_RoutesAreAbsentWhenTheFeatureIsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	RegisterRoutes(group, nil, Flags{CatalogueEnabled: false, WritesEnabled: true})
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "/food/") {
			t.Fatalf("route %s %s was registered with the food catalogue disabled", route.Method, route.Path)
		}
	}
}

// TestHTTP_ErrorEnvelopesCarryStableCodes proves a client can branch on a code
// rather than on prose, and that no raw SQL text reaches a response.
func TestHTTP_ErrorEnvelopesCarryStableCodes(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	router := env.buildRouter(t, staffA, businessA)

	recorder := env.do(t, router, http.MethodGet, "/api/v1/vendor/food/menus/fmn_missing", nil, "")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Success {
		t.Fatal("an error envelope must not report success")
	}
	if envelope.Error.Details["code"] != CodeMenuNotFound {
		t.Fatalf("expected the stable code %s, got %+v", CodeMenuNotFound, envelope.Error.Details)
	}
	for _, leak := range []string{"SELECT", "food_menus", "pgx", "SQLSTATE"} {
		if strings.Contains(recorder.Body.String(), leak) {
			t.Errorf("the response leaks %q: %s", leak, recorder.Body.String())
		}
	}

	// An unknown field is rejected rather than silently ignored, so a typo in a
	// price field cannot look like a successful edit.
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	bad := env.do(t, router, http.MethodPatch, "/api/v1/vendor/food/menus/"+menu.ID,
		map[string]any{"nmae": "typo", "expected_version": menu.Version}, "key-typo")
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("expected an unknown field to be refused, got %d: %s", bad.Code, bad.Body.String())
	}
}
