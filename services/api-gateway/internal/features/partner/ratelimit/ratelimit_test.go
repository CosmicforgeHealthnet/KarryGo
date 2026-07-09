package ratelimit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partnerauth"
)

// fakeStore is an in-memory rate-limit Store keyed by the exact rl:... key.
type fakeStore struct {
	mu     sync.Mutex
	counts map[string]int64
}

func newFakeStore() *fakeStore { return &fakeStore{counts: map[string]int64{}} }

func (s *fakeStore) Incr(_ context.Context, key string, _ time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[key]++
	return s.counts[key], nil
}

// limiterWith builds a Limiter with a single low limit for testing.
func limiterWith(store Store, method, fullPath string, limit int64) *Limiter {
	return &Limiter{store: store, limits: map[string]int64{method + " " + fullPath: limit}, window: Window}
}

// routerFor mounts the limiter on a group and a handler that reports partnerID.
func routerFor(l *Limiter, partnerID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v1/partner/v1")
	// Inject partner identity (auth would normally do this).
	g.Use(func(c *gin.Context) { c.Set("partner_id", partnerID); c.Next() })
	g.Use(l.Middleware())
	g.POST("/quotes", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	g.GET("/deliveries/:id", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func hit(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestWithinLimitPassesAndSetsHeaders(t *testing.T) {
	l := limiterWith(newFakeStore(), "POST", "/api/v1/partner/v1/quotes", 3)
	r := routerFor(l, "p1")
	w := hit(r, http.MethodPost, "/api/v1/partner/v1/quotes")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Header().Get("X-RateLimit-Limit") != "3" {
		t.Fatalf("X-RateLimit-Limit = %q, want 3", w.Header().Get("X-RateLimit-Limit"))
	}
	if w.Header().Get("X-RateLimit-Remaining") != "2" {
		t.Fatalf("X-RateLimit-Remaining = %q, want 2", w.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestOverLimitReturns429Envelope(t *testing.T) {
	l := limiterWith(newFakeStore(), "POST", "/api/v1/partner/v1/quotes", 2)
	r := routerFor(l, "p1")
	hit(r, http.MethodPost, "/api/v1/partner/v1/quotes")
	hit(r, http.MethodPost, "/api/v1/partner/v1/quotes")
	w := hit(r, http.MethodPost, "/api/v1/partner/v1/quotes") // 3rd exceeds

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 (body=%s)", w.Code, w.Body.String())
	}
	var body partnerauth.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "rate_limit_exceeded" {
		t.Fatalf("code = %q, want rate_limit_exceeded", body.Error.Code)
	}
	if body.Success {
		t.Fatal("success = true, want false")
	}
}

func TestCountersArePerPartner(t *testing.T) {
	store := newFakeStore()
	l := limiterWith(store, "POST", "/api/v1/partner/v1/quotes", 1)

	// p1 uses its single allowance.
	if w := hit(routerFor(l, "p1"), http.MethodPost, "/api/v1/partner/v1/quotes"); w.Code != http.StatusOK {
		t.Fatalf("p1 first request = %d, want 200", w.Code)
	}
	if w := hit(routerFor(l, "p1"), http.MethodPost, "/api/v1/partner/v1/quotes"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("p1 second request = %d, want 429", w.Code)
	}
	// p2 has its own counter and still passes.
	if w := hit(routerFor(l, "p2"), http.MethodPost, "/api/v1/partner/v1/quotes"); w.Code != http.StatusOK {
		t.Fatalf("p2 first request = %d, want 200 (separate counter)", w.Code)
	}
}

func TestCountersArePerRoutePattern(t *testing.T) {
	store := newFakeStore()
	l := &Limiter{store: store, limits: map[string]int64{
		"POST /api/v1/partner/v1/quotes":        1,
		"GET /api/v1/partner/v1/deliveries/:id": 1,
	}, window: Window}
	r := routerFor(l, "p1")

	// Distinct :id values collapse to the same route-pattern key.
	if w := hit(r, http.MethodGet, "/api/v1/partner/v1/deliveries/abc"); w.Code != http.StatusOK {
		t.Fatalf("first GET = %d, want 200", w.Code)
	}
	if w := hit(r, http.MethodGet, "/api/v1/partner/v1/deliveries/xyz"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("second GET (different id) = %d, want 429 (pattern-keyed)", w.Code)
	}
	// A different route/pattern has its own counter and still passes.
	if w := hit(r, http.MethodPost, "/api/v1/partner/v1/quotes"); w.Code != http.StatusOK {
		t.Fatalf("POST /quotes = %d, want 200 (separate route counter)", w.Code)
	}

	// Verify the stored keys are pattern-based, not raw-URL-based.
	if _, ok := store.counts["rl:p1:GET:/api/v1/partner/v1/deliveries/:id"]; !ok {
		t.Fatalf("expected pattern-keyed rate-limit key, got keys: %v", keysOf(store.counts))
	}
}

func keysOf(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
