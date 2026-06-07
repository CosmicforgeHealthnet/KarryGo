package authhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

func TestDispatchRiderAuthRequiredAllowsValidAccessTokenAndAttachesContext(t *testing.T) {
	tokens := authusecases.NewTokenUsecase([]byte("middleware-secret"), 15*time.Minute, 30*24*time.Hour)
	rawToken, _, err := tokens.GenerateAccessToken("rider-123", "+2348012345678", "session-123")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	router := buildMiddlewareRouter(tokens)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["dispatch_rider_id"] != "rider-123" {
		t.Fatalf("dispatch_rider_id = %v, want rider-123", data["dispatch_rider_id"])
	}
	if data["phone_number"] != "+2348012345678" {
		t.Fatalf("phone_number = %v, want +2348012345678", data["phone_number"])
	}
	if data["session_id"] != "session-123" {
		t.Fatalf("session_id = %v, want session-123", data["session_id"])
	}
	if data["role"] != authmodels.RoleDispatchProvider {
		t.Fatalf("role = %v, want %s", data["role"], authmodels.RoleDispatchProvider)
	}
}

func TestDispatchRiderAuthRequiredRejectsMissingAuthorizationHeader(t *testing.T) {
	tokens := authusecases.NewTokenUsecase([]byte("middleware-secret"), 15*time.Minute, 30*24*time.Hour)
	router := buildMiddlewareRouter(tokens)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("X-Request-ID", "missing-auth-req")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertUnauthorizedMiddlewareResponse(t, w, "missing-auth-req")
}

func TestDispatchRiderAuthRequiredRejectsMalformedAuthorizationHeader(t *testing.T) {
	tokens := authusecases.NewTokenUsecase([]byte("middleware-secret"), 15*time.Minute, 30*24*time.Hour)
	router := buildMiddlewareRouter(tokens)

	for _, header := range []string{
		"token-only",
		"Bearer",
		"Basic token",
		"Bearer token extra",
	} {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", header)
		req.Header.Set("X-Request-ID", "malformed-auth-req")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assertUnauthorizedMiddlewareResponse(t, w, "malformed-auth-req")
	}
}

func TestDispatchRiderAuthRequiredRejectsExpiredAccessToken(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	tokens := authusecases.NewTokenUsecase([]byte("middleware-secret"), time.Minute, 30*24*time.Hour)
	tokens.WithClock(func() time.Time { return now })
	rawToken, _, err := tokens.GenerateAccessToken("rider-123", "+2348012345678", "session-123")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	tokens.WithClock(func() time.Time { return now.Add(2 * time.Minute) })

	router := buildMiddlewareRouter(tokens)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("X-Request-ID", "expired-auth-req")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertUnauthorizedMiddlewareResponse(t, w, "expired-auth-req")
}

func TestDispatchRiderAuthRequiredRejectsWrongSignature(t *testing.T) {
	signingTokens := authusecases.NewTokenUsecase([]byte("other-secret"), 15*time.Minute, 30*24*time.Hour)
	rawToken, _, err := signingTokens.GenerateAccessToken("rider-123", "+2348012345678", "session-123")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	validationTokens := authusecases.NewTokenUsecase([]byte("middleware-secret"), 15*time.Minute, 30*24*time.Hour)
	router := buildMiddlewareRouter(validationTokens)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("X-Request-ID", "wrong-signature-req")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertUnauthorizedMiddlewareResponse(t, w, "wrong-signature-req")
}

func TestDispatchRiderAuthRequiredRejectsRefreshTokenAsAccessToken(t *testing.T) {
	tokens := authusecases.NewTokenUsecase([]byte("middleware-secret"), 15*time.Minute, 30*24*time.Hour)
	rawToken, _, err := tokens.GenerateRefreshToken("rider-123", "+2348012345678", "session-123")
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}

	router := buildMiddlewareRouter(tokens)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("X-Request-ID", "refresh-as-access-req")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertUnauthorizedMiddlewareResponse(t, w, "refresh-as-access-req")
}

func buildMiddlewareRouter(tokens *authusecases.TokenUsecase) *gin.Engine {
	r := gin.New()
	r.Use(httpx.RequestID())
	r.Use(httpx.Recovery())
	r.Use(httpx.ErrorHandler())
	r.GET("/protected", DispatchRiderAuthRequired(tokens), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"dispatch_rider_id": DispatchRiderID(c),
				"phone_number":      PhoneNumber(c),
				"session_id":        SessionID(c),
				"role":              Role(c),
			},
		})
	})
	return r
}

func assertUnauthorizedMiddlewareResponse(t *testing.T, w *httptest.ResponseRecorder, requestID string) {
	t.Helper()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != string(apperrors.CodeUnauthorized) {
		t.Fatalf("code = %v, want unauthorized", errObj["code"])
	}
	if requestID != "" && errObj["request_id"] != requestID {
		t.Fatalf("request_id = %v, want %s", errObj["request_id"], requestID)
	}
}
