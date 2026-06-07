package verification

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

func TestProviderVerificationRoutesReturn401WithoutJWT(t *testing.T) {
	router, _ := buildVerificationTestRouter(newFakeVerificationRepository())
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/provider/verification/identity"},
		{http.MethodPost, "/api/v1/provider/verification/licence"},
		{http.MethodPost, "/api/v1/provider/verification/face"},
		{http.MethodGet, "/api/v1/provider/verification/status"},
		{http.MethodGet, "/api/v1/provider/verification/status/identity"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body = %s", w.Code, w.Body.String())
			}
			assertVerificationErrorCode(t, w.Body.Bytes(), apperrors.CodeUnauthorized)
		})
	}
}

func TestAdminVerificationReviewReturns401WithoutJWT(t *testing.T) {
	router, _ := buildVerificationTestRouter(newFakeVerificationRepository())

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/verification/11111111-1111-1111-1111-111111111111/review", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), apperrors.CodeUnauthorized)
}

func TestAdminVerificationReviewReturns403ForDispatchProviderJWT(t *testing.T) {
	router, tokens := buildVerificationTestRouter(newFakeVerificationRepository())
	token, _, err := tokens.GenerateAccessToken("11111111-1111-1111-1111-111111111111", "+2348000000001", "session-123")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/verification/11111111-1111-1111-1111-111111111111/review", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), apperrors.CodeForbidden)
}

func TestAdminVerificationReviewRouteAllowsPlatformAdminJWTAndValidates(t *testing.T) {
	// Phase 3I: AdminReview is now fully implemented.
	// An empty body returns 400 validation_failed (missing step and action).
	router, tokens := buildVerificationTestRouter(newFakeVerificationRepository())
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/verification/11111111-1111-1111-1111-111111111111/review", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Auth passes; now hits validation — expects 400 (missing step/action)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Fatalf("status = %d, admin JWT must pass auth; body = %s", w.Code, w.Body.String())
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 validation error on empty body; body = %s", w.Code, w.Body.String())
	}
}

func buildVerificationTestRouter(repo Repository) (*gin.Engine, *authusecases.TokenUsecase) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.Recovery())
	router.Use(httpx.ErrorHandler())

	tokens := authusecases.NewTokenUsecase([]byte("verification-test-secret"), 15*time.Minute, 30*24*time.Hour)
	handler := NewHandlerWithService(NewService(repo, NewStubSmileIdentityClient()))
	RegisterRoutes(router, tokens, handler)
	return router, tokens
}

func mustRoleToken(t *testing.T, tokens *authusecases.TokenUsecase, role string) string {
	t.Helper()
	token, _, err := tokens.GenerateAccessToken("11111111-1111-1111-1111-111111111111", "+2348000000001", "session-123")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	claims["role"] = role
	updatedPayload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	unsigned := parts[0] + "." + base64.RawURLEncoding.EncodeToString(updatedPayload)

	// TokenUsecase intentionally keeps signing private, so tests reproduce
	// the same HMAC by using the known test secret.
	return unsigned + "." + signJWTForTest([]byte("verification-test-secret"), unsigned)
}

func assertVerificationErrorCode(t *testing.T, raw []byte, code apperrors.Code) {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errorObj, _ := resp["error"].(map[string]any)
	if errorObj["code"] != string(code) {
		t.Fatalf("code = %v, want %s; body = %s", errorObj["code"], code, raw)
	}
}
