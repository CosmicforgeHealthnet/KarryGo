package integration_test

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

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/browserauth"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	authhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/http"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// buildAuthRouter mounts the REAL router with the real authentication
// handler, backed by the test database and a fake OTP sender.
func buildAuthRouter(t *testing.T) (*gin.Engine, *authHarness) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := newAuthHarness(t, nil)

	handler := authhttp.NewHandler(h.service, []byte("test-contact-secret"), browserauth.Policy{Name: "karrygo_fleet_refresh", Path: "/fleet-partner/v1/auth", Secure: true, SameSite: http.SameSiteLaxMode, Origins: []string{"https://fleet.example.test"}})
	router := app.BuildRouter(app.Options{
		Config: config.Config{
			AppEnv: "test", MetricsEnabled: true, MetricsAuthToken: "metrics-token",
			MaxRequestBodyBytes: 1 << 20,
		},
		Metrics:     observability.NewMetrics(),
		TokenSigner: auth.NewTokenSigner([]byte("test-access-secret")),
		Verifier:    serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": []byte("svc")}, time.Minute),
		Resolver:    tenant.UnconfiguredResolver{},
		PublicAuthRoutes: func(group *gin.RouterGroup) {
			handler.RegisterPublicRoutes(group)
		},
		IdentityRoutes: func(group *gin.RouterGroup) {
			handler.RegisterAuthenticatedRoutes(group)
		},
	})
	return router, h
}

func postJSON(router *gin.Engine, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	encoded, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://fleet.example.test")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func decodeEnvelope(t *testing.T, resp *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, resp.Body.String())
	}
	return envelope
}

func TestAuthStartHTTPNeverReturnsOTP(t *testing.T) {
	router, h := buildAuthRouter(t)

	resp := postJSON(router, "/fleet-partner/v1/auth/start", map[string]string{
		"email": "http@example.com", "phone": "08012345678",
	}, nil)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", resp.Code, resp.Body.String())
	}

	envelope := decodeEnvelope(t, resp)
	data := envelope["data"].(map[string]any)
	if data["deliveryChannel"] != "EMAIL" {
		t.Fatalf("deliveryChannel = %v, want EMAIL", data["deliveryChannel"])
	}
	if data["maskedDestination"] != "h***@example.com" {
		t.Fatalf("maskedDestination = %v", data["maskedDestination"])
	}

	// The response must not carry the code in any field.
	message, _ := h.sender.LastOTP()
	if strings.Contains(resp.Body.String(), message.Code) {
		t.Fatal("the OTP must never appear in the API response")
	}
	for _, forbidden := range []string{"code", "otp", "password"} {
		if _, present := data[forbidden]; present {
			t.Fatalf("response exposes forbidden field %q", forbidden)
		}
	}
	if envelope["meta"].(map[string]any)["requestId"] == "" {
		t.Fatal("requestId missing from success envelope")
	}
}

func TestAuthStartRejectsPasswordAndMissingFields(t *testing.T) {
	router, h := buildAuthRouter(t)

	// A password field is simply not part of the contract: supplying one
	// still requires email + phone, and no credential is ever consulted.
	resp := postJSON(router, "/fleet-partner/v1/auth/start", map[string]string{
		"email": "pw@example.com", "password": "hunter2",
	}, nil)
	if resp.Code != http.StatusUnprocessableEntity && resp.Code != http.StatusBadRequest {
		t.Fatalf("missing phone status = %d, want 4xx; body=%s", resp.Code, resp.Body.String())
	}
	if h.sender.OTPCount() != 0 {
		t.Fatal("no OTP may be sent when the request is invalid")
	}

	// Missing email.
	resp = postJSON(router, "/fleet-partner/v1/auth/start", map[string]string{
		"phone": "08012345678",
	}, nil)
	if resp.Code < 400 {
		t.Fatalf("missing email status = %d, want 4xx", resp.Code)
	}
}

func TestAuthVerifyHTTPIssuesSessionAndEnvelope(t *testing.T) {
	router, h := buildAuthRouter(t)

	start := postJSON(router, "/fleet-partner/v1/auth/start", map[string]string{
		"email": "verify@example.com", "phone": "08012345670",
	}, nil)
	challengeID := decodeEnvelope(t, start)["data"].(map[string]any)["challengeId"].(string)
	message, _ := h.sender.LastOTP()

	resp := postJSON(router, "/fleet-partner/v1/auth/verify", map[string]string{
		"challengeId": challengeID, "code": message.Code,
	}, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("verify status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	data := decodeEnvelope(t, resp)["data"].(map[string]any)
	if data["nextStep"] != "ONBOARDING_REQUIRED" {
		t.Fatalf("nextStep = %v", data["nextStep"])
	}
	tokens, ok := data["tokens"].(map[string]any)
	if !ok || tokens["accessToken"] == "" || tokens["refreshToken"] != nil {
		t.Fatalf("tokens missing: %v", data)
	}
	cookies := resp.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value == "" || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].Path != "/fleet-partner/v1/auth" || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("invalid refresh cookie")
	}

	// Wrong code returns the canonical error envelope.
	bad := postJSON(router, "/fleet-partner/v1/auth/verify", map[string]string{
		"challengeId": challengeID, "code": "000000",
	}, nil)
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad code status = %d, want 401", bad.Code)
	}
	errorBody := decodeEnvelope(t, bad)["error"].(map[string]any)
	if errorBody["requestId"] == "" {
		t.Fatal("error envelope must carry requestId")
	}
	if errorBody["code"] == "" {
		t.Fatal("error envelope must carry a stable code")
	}
}

func TestAuthenticatedRoutesRequireBearer(t *testing.T) {
	router, _ := buildAuthRouter(t)

	for _, path := range []string{
		"/fleet-partner/v1/auth/select-organization",
	} {
		resp := postJSON(router, path, map[string]string{}, nil)
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("%s without bearer = %d, want 401", path, resp.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/sessions", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("GET /sessions without bearer = %d, want 401", resp.Code)
	}
}

func TestSessionsListNeverLeaksRefreshHashes(t *testing.T) {
	router, h := buildAuthRouter(t)
	ctx := context.Background()

	challengeID, code := h.startAndCode(t, "sessions@example.com", "08012345671")
	result, err := h.service.Verify(ctx, verifyInputFor(challengeID, code))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	token := signTestAccessToken(t, result.IdentityID, result.Tokens.SessionID)

	req := httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("sessions status = %d; body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	// Neither the refresh token nor its hash may be serialized.
	var stored string
	_ = h.pool.QueryRow(ctx,
		`SELECT refresh_token_hash FROM fleet_partner_corporate_sessions WHERE id = $1`,
		result.Tokens.SessionID).Scan(&stored)
	if stored != "" && strings.Contains(body, stored) {
		t.Fatal("session list leaks the refresh-token hash")
	}
	if strings.Contains(body, result.Tokens.RefreshToken) {
		t.Fatal("session list leaks the refresh token")
	}
}

func TestLogoutHTTPIsIdempotent(t *testing.T) {
	router, h := buildAuthRouter(t)
	ctx := context.Background()

	challengeID, code := h.startAndCode(t, "httplogout@example.com", "08012345672")
	result, err := h.service.Verify(ctx, verifyInputFor(challengeID, code))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		resp := postJSON(router, "/fleet-partner/v1/auth/logout", map[string]string{},
			map[string]string{"Cookie": "karrygo_fleet_refresh=" + result.Tokens.RefreshToken})
		if resp.Code != http.StatusOK {
			t.Fatalf("logout attempt %d = %d; body=%s", attempt, resp.Code, resp.Body.String())
		}
		cookies := resp.Result().Cookies()
		if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
			t.Fatal("logout must expire cookie")
		}
	}
	resp := postJSON(router, "/fleet-partner/v1/auth/refresh", nil, map[string]string{"Cookie": "karrygo_fleet_refresh=" + result.Tokens.RefreshToken})
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("revoked refresh status %d", resp.Code)
	}
}

func TestUnsupportedMediaTypeOnAuthRoutes(t *testing.T) {
	router, _ := buildAuthRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/fleet-partner/v1/auth/start",
		strings.NewReader("email=a@b.com&phone=080"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("form content type = %d, want 415", resp.Code)
	}
}

func TestRequestAndCorrelationIDsPropagate(t *testing.T) {
	router, _ := buildAuthRouter(t)

	encoded, _ := json.Marshal(map[string]string{"email": "ids@example.com", "phone": "08012345673"})
	req := httptest.NewRequest(http.MethodPost, "/fleet-partner/v1/auth/start", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req_supplied")
	req.Header.Set("X-Correlation-ID", "cor_supplied")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Header().Get("X-Request-ID") != "req_supplied" {
		t.Fatalf("request id not echoed: %q", resp.Header().Get("X-Request-ID"))
	}
	if resp.Header().Get("X-Correlation-ID") != "cor_supplied" {
		t.Fatalf("correlation id not echoed: %q", resp.Header().Get("X-Correlation-ID"))
	}
	meta := decodeEnvelope(t, resp)["meta"].(map[string]any)
	if meta["requestId"] != "req_supplied" {
		t.Fatalf("envelope requestId = %v", meta["requestId"])
	}
}

func signTestAccessToken(t *testing.T, identityID string, sessionID string) string {
	t.Helper()
	signer := auth.NewTokenSigner([]byte("test-access-secret"))
	token, err := signer.Sign(auth.Claims{
		Subject:   identityID,
		Role:      config.CorporateRole,
		Service:   config.ServiceName,
		SessionID: sessionID,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}
