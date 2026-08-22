package orders

// Handler contract tests.
//
// Everything asserted here happens BEFORE the service is reached: the auth
// gate, the strict request body and the concurrency header. That is deliberate
// — these are the checks that must hold even if every dependency behind them
// is unavailable.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

const httpTestSecret = "section17e-http-secret"

func ordersHTTPFixture(t *testing.T, enabled bool) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.ErrorHandler())

	signer := sharedauth.NewTokenSigner([]byte(httpTestSecret))
	now := time.Now().UTC()
	token, err := signer.Sign(sharedauth.Claims{
		Subject: "customer_http", Role: "customer", Service: "customer",
		SessionID: "customer-session", Type: sharedauth.TokenTypeAccess,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// A zero service is enough: no assertion here depends on a stored order,
	// and every check under test refuses before the service is consulted.
	service := &Service{}

	RegisterCustomerRoutes(
		router.Group("/api/v1/commerce"), service, signer,
		idempotency.NewStore(nil, time.Hour), enabled, enabled,
	)
	RegisterVendorRoutes(router.Group("/api/v1/vendor"), service, enabled)

	return router, token
}

func do(t *testing.T, router *gin.Engine, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestHandler_UnauthenticatedRequestsAreRefused(t *testing.T) {
	router, _ := ordersHTTPFixture(t, true)

	for _, target := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/commerce/quotes/cq_1/orders"},
		{http.MethodGet, "/api/v1/commerce/orders"},
		{http.MethodGet, "/api/v1/commerce/orders/cpo_1"},
		{http.MethodPost, "/api/v1/commerce/vendor-orders/cvo_1/cancel"},
		{http.MethodGet, "/api/v1/vendor/commerce/orders"},
		{http.MethodGet, "/api/v1/vendor/commerce/orders/cvo_1"},
		{http.MethodPost, "/api/v1/vendor/commerce/orders/cvo_1/accept"},
	} {
		recorder := do(t, router, target.method, target.path, "", "")
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status=%d, want 401",
				target.method, target.path, recorder.Code)
		}
	}
}

// A client may commit to the fingerprint it displayed and to nothing else.
func TestHandler_CreateRejectsAnyFieldOtherThanTheFingerprint(t *testing.T) {
	router, token := ordersHTTPFixture(t, true)

	cases := map[string]string{
		"vendor allocation":   `{"quote_fingerprint":"f","business_id":"biz_a"}`,
		"price override":      `{"quote_fingerprint":"f","customer_approved_total_minor":1}`,
		"payment reference":   `{"quote_fingerprint":"f","payment_operation_ref":"pi_1"}`,
		"funding split":       `{"quote_fingerprint":"f","funding_breakdown":{}}`,
		"state override":      `{"quote_fingerprint":"f","state":"COMPLETED"}`,
		"trailing object":     `{"quote_fingerprint":"f"}{"quote_fingerprint":"g"}`,
		"missing fingerprint": `{}`,
		"empty fingerprint":   `{"quote_fingerprint":"   "}`,
	}

	for name, body := range cases {
		recorder := do(t, router, http.MethodPost,
			"/api/v1/commerce/quotes/cq_1/orders", token, body)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status=%d, want 400 (body=%s)", name, recorder.Code, recorder.Body)
		}
	}
}

func TestHandler_CreateRejectsAnOversizedBody(t *testing.T) {
	router, token := ordersHTTPFixture(t, true)

	body := `{"quote_fingerprint":"` + strings.Repeat("a", maxOrderCommandBody) + `"}`
	recorder := do(t, router, http.MethodPost,
		"/api/v1/commerce/quotes/cq_1/orders", token, body)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", recorder.Code)
	}
}

// A lifecycle command without the version the caller read would be applied
// blind onto a state it never saw.
func TestHandler_LifecycleCommandsRequireIfMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.ErrorHandler())
	// Stand in for the vendor staff middleware the production router applies
	// to the whole protected group.
	router.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{
			StaffID: "staff_1", VendorID: "biz_a", SessionID: "session_1",
		})
	})
	RegisterVendorRoutes(router.Group("/api/v1/vendor"), &Service{}, true)

	for _, transition := range AllTransitions {
		path := "/api/v1/vendor/commerce/orders/cvo_1/" +
			strings.ToLower(string(transition))

		recorder := do(t, router, http.MethodPost, path, "", "")
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s without If-Match: status=%d, want 400", transition, recorder.Code)
		}

		request := httptest.NewRequest(http.MethodPost, path, nil)
		request.Header.Set("If-Match", "0")
		bad := httptest.NewRecorder()
		router.ServeHTTP(bad, request)
		if bad.Code != http.StatusBadRequest {
			t.Errorf("%s with If-Match 0: status=%d, want 400", transition, bad.Code)
		}
	}
}

func TestHandler_TransitionBodyRejectsUnknownFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.ErrorHandler())
	router.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{
			StaffID: "staff_1", VendorID: "biz_a", SessionID: "session_1",
		})
	})
	RegisterVendorRoutes(router.Group("/api/v1/vendor"), &Service{}, true)

	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/vendor/commerce/orders/cvo_1/cancel",
		bytes.NewBufferString(`{"reason_code":"X","refund_allocated_minor":5000}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", "1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 (body=%s)", recorder.Code, recorder.Body)
	}
}

// A disabled capability has no surface at all, rather than routes that answer
// 503 and confirm it exists.
func TestHandler_DisabledCapabilityRegistersNoRoute(t *testing.T) {
	router, token := ordersHTTPFixture(t, false)

	for _, target := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/commerce/quotes/cq_1/orders"},
		{http.MethodGet, "/api/v1/commerce/orders"},
		{http.MethodGet, "/api/v1/vendor/commerce/orders"},
		{http.MethodPost, "/api/v1/vendor/commerce/orders/cvo_1/accept"},
	} {
		recorder := do(t, router, target.method, target.path, token, "")
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s %s: status=%d, want 404 when disabled",
				target.method, target.path, recorder.Code)
		}
	}
}
