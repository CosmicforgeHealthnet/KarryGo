package authhttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	authclients "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/clients"
	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	authrepositories "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/repositories"
	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

type logoutHTTPFixture struct {
	router       *gin.Engine
	service      *authusecases.AuthUsecase
	sessions     *refreshHTTPSessionRepo
	identity     authmodels.Identity
	accessToken  string
	refreshToken string
	sessionID    string
}

func newLogoutHTTPFixture(t *testing.T) logoutHTTPFixture {
	t.Helper()

	refreshToken := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sessionID := "session-http-logout-1"
	identity := authmodels.Identity{
		ID:          "rider-http-logout-1",
		PhoneNumber: "+2348012345678",
		Status:      authmodels.StatusActive,
	}
	identityRepo := &refreshHTTPIdentityRepo{byPhone: map[string]authmodels.Identity{identity.PhoneNumber: identity}}
	sessionRepo := &refreshHTTPSessionRepo{sessions: map[string]authmodels.Session{
		sessionID: {
			ID:               sessionID,
			DispatchRiderID:  identity.ID,
			PhoneNumber:      identity.PhoneNumber,
			RefreshTokenHash: sha256Hex(refreshToken),
			ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
		},
	}}

	router, service := buildLogoutHTTPRouter(identityRepo, sessionRepo, nil)
	accessToken, _, err := service.TokenUsecase().GenerateAccessToken(identity.ID, identity.PhoneNumber, sessionID)
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	return logoutHTTPFixture{
		router:       router,
		service:      service,
		sessions:     sessionRepo,
		identity:     identity,
		accessToken:  accessToken,
		refreshToken: refreshToken,
		sessionID:    sessionID,
	}
}

func TestHandlerLogoutSuccessWithoutRefreshToken(t *testing.T) {
	fx := newLogoutHTTPFixture(t)

	w := performLogoutRequest(fx.router, fx.accessToken, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	assertLogoutSuccessResponse(t, w.Body.Bytes())
	if fx.sessions.sessions[fx.sessionID].RevokedAt == nil {
		t.Fatal("session revoked_at must be set")
	}
	if raw := w.Body.String(); strings.Contains(raw, fx.accessToken) || strings.Contains(raw, fx.refreshToken) {
		t.Fatal("logout success response must not contain token values")
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refresh_token":"`+fx.refreshToken+`"}`))
	req.Header.Set("Content-Type", "application/json")
	fx.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout status = %d, want 401; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerLogoutSuccessWithOptionalRefreshToken(t *testing.T) {
	fx := newLogoutHTTPFixture(t)
	body := []byte(`{"refresh_token":"` + fx.refreshToken + `"}`)

	w := performLogoutRequest(fx.router, fx.accessToken, body, "logout-http-optional")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	assertLogoutSuccessResponse(t, w.Body.Bytes())
	if fx.sessions.sessions[fx.sessionID].RevokedAt == nil {
		t.Fatal("session revoked_at must be set")
	}
}

func TestHandlerLogoutMissingTokenRejectedByMiddleware(t *testing.T) {
	fx := newLogoutHTTPFixture(t)

	w := performLogoutRequest(fx.router, "", nil, "logout-missing-token")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", w.Code, w.Body.String())
	}
	assertLogoutErrorResponse(t, w.Body.Bytes(), apperrors.CodeUnauthorized, "logout-missing-token", false)
}

func TestHandlerLogoutInvalidTokenRejectedByMiddleware(t *testing.T) {
	fx := newLogoutHTTPFixture(t)

	w := performLogoutRequest(fx.router, "not-a-valid-token", nil, "logout-invalid-token")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", w.Code, w.Body.String())
	}
	assertLogoutErrorResponse(t, w.Body.Bytes(), apperrors.CodeUnauthorized, "logout-invalid-token", false)
}

func TestHandlerLogoutInvalidOptionalRefreshTokenReturnsValidation(t *testing.T) {
	fx := newLogoutHTTPFixture(t)

	w := performLogoutRequest(fx.router, fx.accessToken, []byte(`{"refresh_token":"bad"}`), "logout-bad-refresh")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
	assertLogoutErrorResponse(t, w.Body.Bytes(), apperrors.CodeValidationFailed, "logout-bad-refresh", true)
	if fx.sessions.sessions[fx.sessionID].RevokedAt != nil {
		t.Fatal("session must not be revoked when refresh_token validation fails")
	}
}

func TestHandlerLogoutAlreadyRevokedReturnsUnauthorized(t *testing.T) {
	fx := newLogoutHTTPFixture(t)

	w := performLogoutRequest(fx.router, fx.accessToken, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("first logout status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	w = performLogoutRequest(fx.router, fx.accessToken, nil, "logout-already-revoked")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("second logout status = %d, want 401; body = %s", w.Code, w.Body.String())
	}
	assertLogoutErrorResponse(t, w.Body.Bytes(), apperrors.CodeUnauthorized, "logout-already-revoked", false)
}

func performLogoutRequest(router *gin.Engine, accessToken string, body []byte, requestID string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", reader)
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func assertLogoutSuccessResponse(t *testing.T, body []byte) {
	t.Helper()

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.Success {
		t.Fatal("success must be true")
	}
	if resp.Data.Message != "Logged out successfully." {
		t.Fatalf("message = %q, want logged out success", resp.Data.Message)
	}
}

func assertLogoutErrorResponse(t *testing.T, body []byte, code apperrors.Code, requestID string, wantFields bool) {
	t.Helper()

	var resp struct {
		Success bool `json:"success"`
		Error   struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
			Fields    []struct {
				Field string `json:"field"`
			} `json:"fields"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Success {
		t.Fatal("success must be false")
	}
	if resp.Error.Code != string(code) {
		t.Fatalf("code = %q, want %q", resp.Error.Code, code)
	}
	if resp.Error.RequestID != requestID {
		t.Fatalf("request_id = %q, want %q", resp.Error.RequestID, requestID)
	}
	if wantFields {
		if len(resp.Error.Fields) == 0 {
			t.Fatal("expected validation fields")
		}
		if resp.Error.Fields[0].Field != "refresh_token" {
			t.Fatalf("field = %q, want refresh_token", resp.Error.Fields[0].Field)
		}
		return
	}
	if len(resp.Error.Fields) != 0 {
		t.Fatalf("fields = %v, want none", resp.Error.Fields)
	}
}

func buildLogoutHTTPRouter(
	identities authrepositories.IdentityRepository,
	sessions authrepositories.SessionRepository,
	publisher authclients.EventPublisher,
) (*gin.Engine, *authusecases.AuthUsecase) {
	authSvc := authusecases.NewAuthUsecase(authusecases.Options{
		Identities:         identities,
		Sessions:           sessions,
		Publisher:          publisher,
		AccessTokenSecret:  []byte("access-secret-32-bytes-long-xxxx"),
		RefreshTokenSecret: []byte("refresh-secret-32-bytes-long-xxx"),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
	})

	r := gin.New()
	r.Use(httpx.RequestID())
	r.Use(httpx.Recovery())
	r.Use(httpx.ErrorHandler())
	RegisterRoutes(r.Group("/api/v1/auth"), authSvc)
	return r, authSvc
}
