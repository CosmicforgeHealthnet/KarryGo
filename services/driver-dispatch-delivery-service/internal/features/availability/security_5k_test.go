package availability

// Phase 5K — Security tests.
// Covers: status=busy rejection, GPS rate limiting (30/min), and
// GPS coordinate validation (lat=999 etc.).

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

// ── status=busy rejection (Phase 5K §5) ──────────────────────────────────────

func Test5K_StatusBusyViaAPIReturns400(t *testing.T) {
	router, tokens := newAvailabilityTestRouter()
	tok, _, err := tokens.GenerateAccessToken(uuid.NewString(), "+2348000000000", uuid.NewString())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	tok = wsInjectRole(t, tok, "dispatch_provider", []byte("test-access-secret"))

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/availability",
		strings.NewReader(`{"status":"busy"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5K_StatusOnlineAndOfflineAreAccepted(t *testing.T) {
	// Only checks that the validation layer doesn't block online/offline.
	// (fakeAvailabilityService returns a blank success for both).
	router, tokens := newAvailabilityTestRouter()
	tok, _, err := tokens.GenerateAccessToken(uuid.NewString(), "+2348000000000", uuid.NewString())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	tok = wsInjectRole(t, tok, "dispatch_provider", []byte("test-access-secret"))

	for _, status := range []string{"online", "offline"} {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/availability",
			strings.NewReader(fmt.Sprintf(`{"status":%q}`, status)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code == http.StatusBadRequest {
			t.Fatalf("status=%q returned 400, want non-400; body=%s", status, w.Body.String())
		}
	}
}

// ── GPS validation ────────────────────────────────────────────────────────────

func Test5K_LatOut999Returns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)

	w := postLocation(router, tok, `{"lat":999,"lng":3,"heading":0,"speed":0,"accuracy":0}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

func Test5K_LngOutMinus999Returns400(t *testing.T) {
	router, tokens, store, _ := newLocationTestEnv(t)
	ctx := context.Background()
	providerID := uuid.NewString()
	_ = store.SetStatus(ctx, providerID, StatusOnline)
	tok := providerJWT(t, tokens, providerID)

	w := postLocation(router, tok, `{"lat":6,"lng":-999,"heading":0,"speed":0,"accuracy":0}`)
	assertHTTPCode(t, w, http.StatusBadRequest)
	assertHTTPErrorCode(t, w, apperrors.CodeValidationFailed)
}

// ── Rate limiting (Phase 5K §4) ───────────────────────────────────────────────

func newRateLimitTestEnv(t *testing.T) (*gin.Engine, string, *authusecases.TokenUsecase, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	gin.SetMode(gin.TestMode)
	tokens := authusecases.NewTokenUsecase([]byte("rate-test-secret"), time.Hour, time.Hour)
	providerID := uuid.NewString()
	tok, _, err := tokens.GenerateAccessToken(providerID, "+2348000000000", uuid.NewString())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	tok = wsInjectRole(t, tok, "dispatch_provider", []byte("rate-test-secret"))

	store := NewRedisLiveStore(client)
	_ = store.SetStatus(context.Background(), providerID, StatusOnline)

	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	handler := NewHandlerWithService(svc, tokens, client)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.ErrorHandler())
	RegisterRoutes(router, tokens, "test-service-key", handler)
	return router, tok, tokens, client
}

func Test5K_First30LocationRequestsSucceed(t *testing.T) {
	router, tok, _, _ := newRateLimitTestEnv(t)
	body := `{"lat":6.5244,"lng":3.3792,"heading":0,"speed":0,"accuracy":0}`

	for i := 1; i <= LocationRateLimitMaxPerMin; i++ {
		w := postLocation(router, tok, body)
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d returned 429 — should succeed within limit", i)
		}
	}
}

func Test5K_31stRequestReturns429(t *testing.T) {
	router, tok, _, _ := newRateLimitTestEnv(t)
	body := `{"lat":6.5244,"lng":3.3792,"heading":0,"speed":0,"accuracy":0}`

	// Exhaust the limit.
	for i := 0; i < LocationRateLimitMaxPerMin; i++ {
		postLocation(router, tok, body)
	}
	// 31st request must be rate limited.
	w := postLocation(router, tok, body)
	assertHTTPCode(t, w, http.StatusTooManyRequests)
	assertHTTPErrorCode(t, w, apperrors.CodeRateLimited)
}

func Test5K_RateLimitIsPerProvider(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	gin.SetMode(gin.TestMode)
	tokens := authusecases.NewTokenUsecase([]byte("rate-sep-secret"), time.Hour, time.Hour)

	// Two separate providers.
	makeToken := func(id string) string {
		tok, _, _ := tokens.GenerateAccessToken(id, "+23480000000000", uuid.NewString())
		return wsInjectRole(t, tok, "dispatch_provider", []byte("rate-sep-secret"))
	}
	p1 := uuid.NewString()
	p2 := uuid.NewString()
	tok1 := makeToken(p1)
	tok2 := makeToken(p2)

	store := NewRedisLiveStore(client)
	ctx := context.Background()
	_ = store.SetStatus(ctx, p1, StatusOnline)
	_ = store.SetStatus(ctx, p2, StatusOnline)

	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	handler := NewHandlerWithService(svc, tokens, client)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.ErrorHandler())
	RegisterRoutes(router, tokens, "test-service-key", handler)

	body := `{"lat":6,"lng":3,"heading":0,"speed":0,"accuracy":0}`

	// Exhaust p1's limit.
	for i := 0; i < LocationRateLimitMaxPerMin; i++ {
		postLocation(router, tok1, body)
	}
	// p1's 31st should be rate limited.
	w := postLocation(router, tok1, body)
	assertHTTPCode(t, w, http.StatusTooManyRequests)

	// p2 should still be fine — separate counter.
	w2 := postLocation(router, tok2, body)
	if w2.Code == http.StatusTooManyRequests {
		t.Fatal("provider 2 hit rate limit but should have its own independent counter")
	}
}

func Test5K_RateLimitCounterExpiresAfterWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	gin.SetMode(gin.TestMode)
	tokens := authusecases.NewTokenUsecase([]byte("rate-exp-secret"), time.Hour, time.Hour)
	providerID := uuid.NewString()
	tok, _, _ := tokens.GenerateAccessToken(providerID, "+2348000000000", uuid.NewString())
	tok = wsInjectRole(t, tok, "dispatch_provider", []byte("rate-exp-secret"))

	store := NewRedisLiveStore(client)
	_ = store.SetStatus(context.Background(), providerID, StatusOnline)

	repo := newFakeAvailabilityRepository()
	svc := NewService(repo, store)
	handler := NewHandlerWithService(svc, tokens, client)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.ErrorHandler())
	RegisterRoutes(router, tokens, "test-service-key", handler)

	body := `{"lat":6,"lng":3,"heading":0,"speed":0,"accuracy":0}`

	// Exhaust limit.
	for i := 0; i < LocationRateLimitMaxPerMin; i++ {
		postLocation(router, tok, body)
	}
	w := postLocation(router, tok, body)
	assertHTTPCode(t, w, http.StatusTooManyRequests)

	// Advance past the window — counter should expire.
	mr.FastForward(LocationRateLimitWindow + time.Second)
	// Re-seed status (it also expires).
	_ = store.SetStatus(context.Background(), providerID, StatusOnline)

	w2 := postLocation(router, tok, body)
	if w2.Code == http.StatusTooManyRequests {
		t.Fatal("rate limit should have reset after window expired")
	}
}
