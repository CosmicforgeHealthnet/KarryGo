package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

var (
	testAccessSecret  = []byte("test-access-secret")
	testServiceSecret = []byte("test-service-secret")
	testMetricsToken  = "test-metrics-token"
)

type staticResolver struct {
	membership tenant.Membership
	err        error
}

func (r staticResolver) ResolveMembership(context.Context, string) (tenant.Membership, error) {
	return r.membership, r.err
}

func testConfig() config.Config {
	return config.Config{
		AppEnv:              "test",
		LogLevel:            "info",
		MetricsEnabled:      true,
		MetricsAuthToken:    testMetricsToken,
		MaxRequestBodyBytes: 1 << 20,
	}
}

func buildTestRouter(t *testing.T, resolver tenant.MembershipResolver, readyChecks ...ReadyCheck) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if resolver == nil {
		resolver = staticResolver{membership: tenant.Membership{
			OrganizationID:     "org_11111111111111111111111111111111",
			OrganizationUserID: "usr_1",
		}}
	}
	return BuildRouter(Options{
		Config:      testConfig(),
		Metrics:     observability.NewMetrics(),
		TokenSigner: auth.NewTokenSigner(testAccessSecret),
		Verifier:    serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": testServiceSecret}, 5*time.Minute),
		Resolver:    resolver,
		ReadyChecks: readyChecks,
	})
}

func corporateToken(t *testing.T) string {
	t.Helper()
	signer := auth.NewTokenSigner(testAccessSecret)
	token, err := signer.Sign(auth.Claims{
		Subject:   "usr_1",
		Role:      "organization_user",
		Service:   config.ServiceName,
		SessionID: "ses_1",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func do(router *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestHealthIsLivenessOnly(t *testing.T) {
	// No ready checks and no database wired at all: /health must still pass.
	router := buildTestRouter(t, nil)
	resp := do(router, httptest.NewRequest(http.MethodGet, "/health", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("/health = %d", resp.Code)
	}
}

func TestOrganizationAPIUsesSeparateAuthAndCannotReachManagementRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := BuildRouter(Options{
		Config:      testConfig(),
		Metrics:     observability.NewMetrics(),
		TokenSigner: auth.NewTokenSigner(testAccessSecret),
		Verifier:    serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": testServiceSecret}, 5*time.Minute),
		Resolver:    staticResolver{err: tenant.ErrMembershipNotFound},
		OrganizationAPIAuth: func(c *gin.Context) {
			clientID, secret, ok := c.Request.BasicAuth()
			if !ok || clientID != "client_1" || secret != "secret_1" {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			c.Next()
		},
		OrganizationAPIRoutes: func(group *gin.RouterGroup) {
			group.GET("/context", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"routeClass": "organization_api"}) })
		},
	})

	valid := httptest.NewRequest(http.MethodGet, "/fleet-partner/integrations/v1/context", nil)
	valid.SetBasicAuth("client_1", "secret_1")
	if response := do(router, valid); response.Code != http.StatusOK {
		t.Fatalf("organization API credential route = %d", response.Code)
	}
	invalid := httptest.NewRequest(http.MethodGet, "/fleet-partner/integrations/v1/context", nil)
	if response := do(router, invalid); response.Code != http.StatusUnauthorized {
		t.Fatalf("missing organization API credential = %d", response.Code)
	}
	management := httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/integrations/api-clients", nil)
	management.SetBasicAuth("client_1", "secret_1")
	if response := do(router, management); response.Code >= 200 && response.Code < 300 {
		t.Fatalf("organization API credential reached corporate management surface: %d", response.Code)
	}
}

func TestReadySucceedsAndFails(t *testing.T) {
	healthy := ReadyCheck{Name: "postgres", Check: func(context.Context) error { return nil }}
	router := buildTestRouter(t, nil, healthy)
	if resp := do(router, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusOK {
		t.Fatalf("/ready healthy = %d", resp.Code)
	}

	broken := ReadyCheck{Name: "postgres", Check: func(context.Context) error {
		return errors.New("dial tcp: password=SECRET authentication failed")
	}}
	router = buildTestRouter(t, nil, broken)
	resp := do(router, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready broken = %d", resp.Code)
	}
	if strings.Contains(resp.Body.String(), "SECRET") || strings.Contains(resp.Body.String(), "password") {
		t.Fatalf("/ready leaks dependency error detail: %s", resp.Body.String())
	}
}

func TestMetricsProtected(t *testing.T) {
	router := buildTestRouter(t, nil)

	if resp := do(router, httptest.NewRequest(http.MethodGet, "/metrics", nil)); resp.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /metrics = %d", resp.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	if resp := do(router, req); resp.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-token /metrics = %d", resp.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+testMetricsToken)
	resp := do(router, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("authorized /metrics = %d", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "fleet_partner_http_requests_total") {
		t.Fatalf("metrics body missing foundation metrics: %s", body)
	}
	if strings.Contains(body, testMetricsToken) || strings.Contains(body, "org_1111") {
		t.Fatal("metrics body must not contain secrets or organization ids")
	}
}

func TestCorporateRouteRequiresBearer(t *testing.T) {
	router := buildTestRouter(t, nil)
	resp := do(router, httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/meta", nil))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated corporate route = %d", resp.Code)
	}
	assertErrorEnvelope(t, resp)
}

func TestCorporateMetaWithValidToken(t *testing.T) {
	router := buildTestRouter(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/meta", nil)
	req.Header.Set("Authorization", "Bearer "+corporateToken(t))
	resp := do(router, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("corporate meta = %d body=%s", resp.Code, resp.Body.String())
	}
	envelope := assertSuccessEnvelope(t, resp)
	data := envelope["data"].(map[string]any)
	if data["actorId"] != "usr_1" {
		t.Fatalf("actorId = %v", data["actorId"])
	}
	if resp.Header().Get("X-Request-ID") == "" || resp.Header().Get("X-Correlation-ID") == "" {
		t.Fatal("request/correlation IDs missing on response")
	}
}

func TestSuspendedMembershipRejected(t *testing.T) {
	router := buildTestRouter(t, staticResolver{membership: tenant.Membership{
		OrganizationID:     "org_11111111111111111111111111111111",
		OrganizationUserID: "usr_1",
		Status:             tenant.MembershipStatusSuspended,
	}})
	req := httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/meta/tenant", nil)
	req.Header.Set("Authorization", "Bearer "+corporateToken(t))
	resp := do(router, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("suspended membership = %d, want 403", resp.Code)
	}
	assertErrorEnvelope(t, resp)
}

func TestTenantScopedRouteFailsClosedWhenUnresolved(t *testing.T) {
	router := buildTestRouter(t, staticResolver{err: tenant.ErrMembershipNotFound})
	req := httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/meta/tenant", nil)
	req.Header.Set("Authorization", "Bearer "+corporateToken(t))
	resp := do(router, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("unresolved tenant = %d", resp.Code)
	}

	router = buildTestRouter(t, staticResolver{err: tenant.ErrResolverUnconfigured})
	req = httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/meta/tenant", nil)
	req.Header.Set("Authorization", "Bearer "+corporateToken(t))
	resp = do(router, req)
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured resolver = %d", resp.Code)
	}
}

func TestOrganizationOverrideIgnored(t *testing.T) {
	router := buildTestRouter(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/meta/tenant?organizationId=org_attacker", nil)
	req.Header.Set("Authorization", "Bearer "+corporateToken(t))
	req.Header.Set("X-Organization-ID", "org_attacker")
	resp := do(router, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("tenant meta = %d body=%s", resp.Code, resp.Body.String())
	}
	envelope := assertSuccessEnvelope(t, resp)
	data := envelope["data"].(map[string]any)
	if data["organizationId"] != "org_11111111111111111111111111111111" {
		t.Fatalf("organizationId = %v (client override must be ignored)", data["organizationId"])
	}
}

func TestInternalRouteRejectsUserTokens(t *testing.T) {
	router := buildTestRouter(t, nil)

	// Bearer-only request: rejected.
	req := httptest.NewRequest(http.MethodGet, "/internal/fleet-partner/v1/meta", nil)
	req.Header.Set("Authorization", "Bearer "+corporateToken(t))
	resp := do(router, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("internal route with user token = %d", resp.Code)
	}
	assertErrorEnvelope(t, resp)
}

func TestInternalRouteWithServiceSignature(t *testing.T) {
	router := buildTestRouter(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/internal/fleet-partner/v1/meta", nil)
	if err := serviceauth.SignRequest(req, "admin-backoffice-service", testServiceSecret, nil, time.Now()); err != nil {
		t.Fatalf("sign request: %v", err)
	}
	resp := do(router, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("signed internal meta = %d body=%s", resp.Code, resp.Body.String())
	}
	envelope := assertSuccessEnvelope(t, resp)
	data := envelope["data"].(map[string]any)
	if data["caller"] != "admin-backoffice-service" {
		t.Fatalf("caller = %v", data["caller"])
	}
}

func TestUnsupportedMediaType(t *testing.T) {
	router := buildTestRouter(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/fleet-partner/v1/meta/echo", strings.NewReader("a=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+corporateToken(t))
	resp := do(router, req)
	if resp.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("unsupported media type = %d", resp.Code)
	}
	assertErrorEnvelope(t, resp)
}

func TestErrorEnvelopeShape(t *testing.T) {
	router := buildTestRouter(t, nil)
	resp := do(router, httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/meta", nil))
	body := assertErrorEnvelope(t, resp)
	errBody := body["error"].(map[string]any)
	if errBody["code"] != "unauthorized" {
		t.Fatalf("code = %v", errBody["code"])
	}
	if errBody["requestId"] == "" {
		t.Fatal("requestId missing in error envelope")
	}
	if _, hasDetails := errBody["details"]; !hasDetails {
		t.Fatal("details missing in error envelope")
	}
}

func assertSuccessEnvelope(t *testing.T, resp *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, resp.Body.String())
	}
	if _, ok := envelope["data"]; !ok {
		t.Fatalf("missing data: %s", resp.Body.String())
	}
	meta, ok := envelope["meta"].(map[string]any)
	if !ok || meta["requestId"] == "" {
		t.Fatalf("missing meta.requestId: %s", resp.Body.String())
	}
	return envelope
}

func assertErrorEnvelope(t *testing.T, resp *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, resp.Body.String())
	}
	errBody, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error envelope: %s", resp.Body.String())
	}
	if errBody["requestId"] == "" {
		t.Fatalf("missing requestId: %s", resp.Body.String())
	}
	return envelope
}
