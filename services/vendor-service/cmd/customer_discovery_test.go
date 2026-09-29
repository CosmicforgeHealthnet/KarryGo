package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

type discoveryTestLimiter struct {
	mu          sync.Mutex
	counts      map[string]int
	max         int
	unavailable bool
}

func TestCustomerCommerce14MountedRoutesMatchOpenAPIAndRequireAuth(t *testing.T) {
	cfg := config.Config{CustomerAccessTokenSecret: []byte("routes-test-secret")}
	cfg.Flags.CommerceFulfilmentPlanningEnabled = true
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	searchFlags := searchprojection.Flags{SearchEnabled: true}
	search := section13{searchFlags: searchFlags, searchHandler: searchprojection.NewInternalHandler(searchprojection.InternalHandlerOptions{Flags: searchFlags})}
	registerCustomerDiscoveryRoutes(router, cfg, nil, nil, search, &discoveryTestLimiter{max: 100})
	registerCartDeliveryPreview(router, cfg, &previewMemoryCart{}, previewCatalogue{}, previewCatalogue{}, &fulfilment.PlanningService{}, &discoveryTestLimiter{max: 100})
	raw, err := os.ReadFile("../docs/openapi/customer-commerce-discovery-preview.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{}
	params := regexp.MustCompile(`:([A-Za-z0-9]+)`)
	for path, methods := range spec.Paths {
		for method := range methods {
			expected[strings.ToUpper(method)+" /api/v1/commerce"+path] = true
		}
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + params.ReplaceAllString(route.Path, "{$1}")
		if !expected[key] {
			t.Fatalf("undocumented mounted route %s", key)
		}
		delete(expected, key)
		req := httptest.NewRequest(route.Method, params.ReplaceAllString(route.Path, "opaque"), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatalf("%s auth bypass: %d", key, w.Code)
		}
	}
	if len(expected) != 0 {
		t.Fatalf("documented unmounted routes: %v", expected)
	}
}

func (l *discoveryTestLimiter) Allow(_ context.Context, bucket, key string, _ int, _ time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.unavailable {
		return apperrors.Unavailable("Redis unavailable", nil)
	}
	if l.counts == nil {
		l.counts = map[string]int{}
	}
	l.counts[bucket+key]++
	if l.counts[bucket+key] > l.max {
		return apperrors.RateLimited("limited", nil)
	}
	return nil
}
func discoveryToken(t *testing.T, secret []byte, subject, role, service, kind string) string {
	t.Helper()
	token, err := sharedauth.NewTokenSigner(secret).Sign(sharedauth.Claims{Subject: subject, Role: role, Service: service, SessionID: "test", Type: kind, ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	return token
}
func TestCustomerDiscoveryGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.Config{CustomerAccessTokenSecret: []byte("discovery-test-secret")}
	cfg.Flags.CommerceFoodEnabled = true
	cfg.Flags.CustomerFoodEnabled = true
	limiter := &discoveryTestLimiter{max: 1}
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	guards := customerDiscoveryGuards(cfg, cart.DomainFood, limiter)
	router.GET("/read", append(guards, func(c *gin.Context) { c.Status(200) })...)
	read := func(token string) int {
		r := httptest.NewRequest("GET", "/read", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	for _, token := range []string{"", "bad", discoveryToken(t, cfg.CustomerAccessTokenSecret, "staff", "staff", "vendor", "access"), discoveryToken(t, cfg.CustomerAccessTokenSecret, "a", "customer", "customer", "refresh")} {
		if status := read(token); status != 401 && status != 403 {
			t.Fatalf("invalid identity accepted: %d", status)
		}
	}
	a := discoveryToken(t, cfg.CustomerAccessTokenSecret, "a", "customer", "customer", "access")
	b := discoveryToken(t, cfg.CustomerAccessTokenSecret, "b", "customer", "customer", "access")
	if read(a) != 200 || read(a) != 429 || read(b) != 200 {
		t.Fatal("budget must isolate bearer subjects")
	}
	limiter.unavailable = true
	if read(b) != 503 {
		t.Fatal("Redis outage must fail closed")
	}
	for _, change := range []func(*config.Config){func(c *config.Config) { c.Flags.CustomerFoodEnabled = false }, func(c *config.Config) { c.Flags.CommerceFoodEnabled = false }, func(c *config.Config) { c.Flags.CommerceGlobalKillSwitch = true }} {
		disabled := cfg
		change(&disabled)
		r := gin.New()
		r.Use(httpx.ErrorHandler())
		r.GET("/read", append(customerDiscoveryGuards(disabled, cart.DomainFood, limiter), func(c *gin.Context) { t.Fatal("disabled handler reached") })...)
		req := httptest.NewRequest("GET", "/read", nil)
		req.Header.Set("Authorization", "Bearer "+a)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 404 {
			t.Fatalf("disabled flag: %d", w.Code)
		}
	}
}
