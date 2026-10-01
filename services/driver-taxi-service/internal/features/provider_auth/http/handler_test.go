package providerauthhttp

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	providerauthusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

const testAccessSecret = "access-secret"

// ─── minimal in-memory repos (the auth boundary is what is under test here) ───

type stubProviders struct {
	byID map[string]providerauthmodels.Provider
}

func (s *stubProviders) UpsertByPhone(_ context.Context, phone string, _ providerauthmodels.Signup) (providerauthmodels.Provider, error) {
	p := providerauthmodels.Provider{ID: "driver-1", Phone: phone, AccountStatus: providerauthmodels.AccountStatusActive}
	s.byID[p.ID] = p
	return p, nil
}

func (s *stubProviders) GetByID(_ context.Context, id string) (providerauthmodels.Provider, error) {
	p, ok := s.byID[id]
	if !ok {
		return providerauthmodels.Provider{}, apperrors.NotFound("Driver could not be found.", nil)
	}
	return p, nil
}

func (s *stubProviders) UpdatePhone(_ context.Context, id, phone string) (providerauthmodels.Provider, error) {
	p := s.byID[id]
	p.Phone = phone
	s.byID[id] = p
	return p, nil
}

func (s *stubProviders) ContactTaken(context.Context, string, string, string) (bool, bool, error) {
	return false, false, nil
}

type stubSessions struct{}

func (stubSessions) Create(context.Context, providerauthmodels.RefreshSession) error { return nil }
func (stubSessions) GetByID(context.Context, string) (providerauthmodels.RefreshSession, error) {
	return providerauthmodels.RefreshSession{}, apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
}
func (stubSessions) Revoke(context.Context, string) error { return nil }

type stubChallenges struct {
	byKey map[string]providerauthmodels.OTPChallenge
}

func (s *stubChallenges) Save(_ context.Context, c providerauthmodels.OTPChallenge, _, _ time.Duration, _ int) error {
	s.byKey[c.IdentifierKey()] = c
	return nil
}
func (s *stubChallenges) Get(_ context.Context, k string) (providerauthmodels.OTPChallenge, bool, error) {
	c, ok := s.byKey[k]
	return c, ok, nil
}
func (s *stubChallenges) RecordFailedAttempt(context.Context, providerauthmodels.OTPChallenge, time.Duration) error {
	return nil
}
func (s *stubChallenges) Delete(_ context.Context, k string) error { delete(s.byKey, k); return nil }

// ─── harness ──────────────────────────────────────────────────────────────────

func newTestRouter(t *testing.T, otpDebug bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := providerauthusecases.NewAuthService(providerauthusecases.Options{
		Providers:          &stubProviders{byID: map[string]providerauthmodels.Provider{"driver-1": {ID: "driver-1", Phone: "+2348011112222", AccountStatus: providerauthmodels.AccountStatusActive}}},
		Sessions:           stubSessions{},
		Challenges:         &stubChallenges{byKey: map[string]providerauthmodels.OTPChallenge{}},
		AccessTokenSecret:  []byte(testAccessSecret),
		RefreshTokenSecret: []byte("refresh-secret"),
		OTPSecret:          []byte("otp-secret"),
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    24 * time.Hour,
		OTPTTL:             10 * time.Minute,
		OTPRateWindow:      time.Minute,
		OTPMaxRequests:     5,
		OTPMaxAttempts:     3,
		OTPDebug:           otpDebug,
	})

	router := gin.New()
	router.Use(httpx.ErrorHandler())
	group := router.Group("/api/v1/taxi")
	RegisterProviderAuthRoutes(group, svc, otpDebug)
	return router
}

// signToken mints an access token with arbitrary claims so each test can probe
// one specific mismatch against the bearer middleware.
func signToken(t *testing.T, role, service string, tokenType string) string {
	t.Helper()
	signer := sharedauth.NewTokenSigner([]byte(testAccessSecret))
	token, err := signer.Sign(sharedauth.Claims{
		Subject:   "driver-1",
		Role:      role,
		Service:   service,
		SessionID: "session-1",
		Type:      tokenType,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func get(t *testing.T, router *gin.Engine, path, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(nethttp.MethodGet, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestProtectedRouteRejectsMissingToken(t *testing.T) {
	rec := get(t, newTestRouter(t, false), "/api/v1/taxi/provider/me", "")
	if rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("missing token should be 401, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestProtectedRouteRejectsMalformedAuthorizationHeader(t *testing.T) {
	router := newTestRouter(t, false)
	req := httptest.NewRequest(nethttp.MethodGet, "/api/v1/taxi/provider/me", nil)
	req.Header.Set("Authorization", "Token abc123") // not a Bearer scheme
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("a non-Bearer scheme should be 401, got %d", rec.Code)
	}
}

func TestProtectedRouteRejectsWrongRole(t *testing.T) {
	// A hauling driver's token must not open a taxi route even though both are
	// signed by services this platform trusts.
	token := signToken(t, "truck_provider", "taxi", sharedauth.TokenTypeAccess)
	rec := get(t, newTestRouter(t, false), "/api/v1/taxi/provider/me", token)
	if rec.Code != nethttp.StatusForbidden {
		t.Fatalf("wrong role should be 403, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestProtectedRouteRejectsWrongService(t *testing.T) {
	token := signToken(t, "taxi_provider", "hauling", sharedauth.TokenTypeAccess)
	rec := get(t, newTestRouter(t, false), "/api/v1/taxi/provider/me", token)
	if rec.Code != nethttp.StatusForbidden {
		t.Fatalf("wrong service should be 403, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestProtectedRouteRejectsRefreshTokenAsBearer(t *testing.T) {
	// Only access tokens open protected routes; a refresh token presented as a
	// bearer must not be accepted.
	token := signToken(t, "taxi_provider", "taxi", sharedauth.TokenTypeRefresh)
	rec := get(t, newTestRouter(t, false), "/api/v1/taxi/provider/me", token)
	if rec.Code != nethttp.StatusForbidden {
		t.Fatalf("a refresh token should not open a protected route, got %d", rec.Code)
	}
}

func TestProtectedRouteRejectsTokenSignedWithAnotherSecret(t *testing.T) {
	foreign := sharedauth.NewTokenSigner([]byte("some-other-service-secret"))
	token, err := foreign.Sign(sharedauth.Claims{
		Subject: "driver-1", Role: "taxi_provider", Service: "taxi",
		SessionID: "s", Type: sharedauth.TokenTypeAccess,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	rec := get(t, newTestRouter(t, false), "/api/v1/taxi/provider/me", token)
	if rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("a foreign-signed token should be 401, got %d", rec.Code)
	}
}

func TestProtectedRouteRejectsExpiredToken(t *testing.T) {
	signer := sharedauth.NewTokenSigner([]byte(testAccessSecret))
	token, err := signer.Sign(sharedauth.Claims{
		Subject: "driver-1", Role: "taxi_provider", Service: "taxi",
		SessionID: "s", Type: sharedauth.TokenTypeAccess,
		ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	rec := get(t, newTestRouter(t, false), "/api/v1/taxi/provider/me", token)
	if rec.Code != nethttp.StatusUnauthorized {
		t.Fatalf("an expired token should be 401, got %d", rec.Code)
	}
}

func TestProtectedRouteAcceptsCorrectToken(t *testing.T) {
	token := signToken(t, "taxi_provider", "taxi", sharedauth.TokenTypeAccess)
	rec := get(t, newTestRouter(t, false), "/api/v1/taxi/provider/me", token)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("a correct taxi_provider/taxi access token should be 200, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestStartAuthResponseOmitsDebugOTPWhenDisabled(t *testing.T) {
	// The handler is the last gate before the wire. Even if a future change
	// populated the usecase field, this must not serialize it.
	for _, tc := range []struct {
		name      string
		otpDebug  bool
		wantField bool
	}{
		{"debug off", false, false},
		{"debug on", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := newTestRouter(t, tc.otpDebug)
			req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/taxi/provider/auth/start",
				strings.NewReader(`{"phone":"08011112222"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != nethttp.StatusCreated {
				t.Fatalf("start should be 201, got %d (%s)", rec.Code, rec.Body.String())
			}

			var body struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			_, present := body.Data["debug_otp"]
			if present != tc.wantField {
				t.Fatalf("debug_otp present=%v, want %v (body %s)", present, tc.wantField, rec.Body.String())
			}
		})
	}
}
