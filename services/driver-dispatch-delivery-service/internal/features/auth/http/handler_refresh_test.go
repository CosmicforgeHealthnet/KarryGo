package authhttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	authrepositories "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

type refreshHTTPIdentityRepo struct {
	byPhone map[string]authmodels.Identity
}

func (r *refreshHTTPIdentityRepo) FindByPhone(ctx context.Context, phone string) (authmodels.Identity, bool, error) {
	identity, ok := r.byPhone[phone]
	return identity, ok, nil
}

func (r *refreshHTTPIdentityRepo) GetByID(ctx context.Context, id string) (authmodels.Identity, bool, error) {
	for _, identity := range r.byPhone {
		if identity.ID == id {
			return identity, true, nil
		}
	}
	return authmodels.Identity{}, false, nil
}

func (r *refreshHTTPIdentityRepo) FindByEmail(ctx context.Context, email string) (authmodels.Identity, bool, error) {
	return authmodels.Identity{}, false, nil
}

func (r *refreshHTTPIdentityRepo) UpsertByPhone(ctx context.Context, phone string) (authmodels.Identity, error) {
	if identity, ok := r.byPhone[phone]; ok {
		return identity, nil
	}
	identity := authmodels.Identity{ID: "rider-http-1", PhoneNumber: phone, Status: authmodels.StatusActive}
	r.byPhone[phone] = identity
	return identity, nil
}

func (r *refreshHTTPIdentityRepo) CreateForSignup(ctx context.Context, phone, email string) (authmodels.Identity, error) {
	identity := authmodels.Identity{ID: "rider-http-1", PhoneNumber: phone, Status: authmodels.StatusActive}
	r.byPhone[phone] = identity
	return identity, nil
}

func (r *refreshHTTPIdentityRepo) UpdatePhone(_ context.Context, _, _, _ string) error { return nil }
func (r *refreshHTTPIdentityRepo) UpdateEmail(_ context.Context, _, _ string) error   { return nil }

var _ authrepositories.IdentityRepository = (*refreshHTTPIdentityRepo)(nil)

type refreshHTTPSessionRepo struct {
	sessions map[string]authmodels.Session
}

func (r *refreshHTTPSessionRepo) Create(ctx context.Context, session authmodels.Session) (authmodels.Session, error) {
	r.sessions[session.ID] = session
	return session, nil
}

func (r *refreshHTTPSessionRepo) FindByRefreshTokenHash(ctx context.Context, hash string) (authmodels.Session, bool, error) {
	for _, session := range r.sessions {
		if session.RefreshTokenHash == hash && session.RevokedAt == nil {
			return session, true, nil
		}
	}
	return authmodels.Session{}, false, nil
}

func (r *refreshHTTPSessionRepo) GetByID(ctx context.Context, id string) (authmodels.Session, bool, error) {
	session, ok := r.sessions[id]
	return session, ok, nil
}

func (r *refreshHTTPSessionRepo) RotateRefreshToken(ctx context.Context, id string, hash string) error {
	session, ok := r.sessions[id]
	if !ok || session.RevokedAt != nil {
		return authrepositories.ErrSessionNotFound
	}
	session.RefreshTokenHash = hash
	session.UpdatedAt = time.Now()
	r.sessions[id] = session
	return nil
}

func (r *refreshHTTPSessionRepo) Revoke(ctx context.Context, id string) error {
	session, ok := r.sessions[id]
	if !ok || session.RevokedAt != nil {
		return authrepositories.ErrSessionNotFound
	}
	now := time.Now()
	session.RevokedAt = &now
	session.UpdatedAt = now
	r.sessions[id] = session
	return nil
}
func (r *refreshHTTPSessionRepo) RevokeAllByDispatchRiderID(ctx context.Context, dispatchRiderID string) (int64, error) {
	var count int64
	for id, s := range r.sessions {
		if s.DispatchRiderID == dispatchRiderID && s.RevokedAt == nil {
			now := time.Now()
			s.RevokedAt = &now
			s.UpdatedAt = now
			r.sessions[id] = s
			count++
		}
	}
	return count, nil
}

var _ authrepositories.SessionRepository = (*refreshHTTPSessionRepo)(nil)

func TestHandlerRefreshSuccessAndOldTokenRejected(t *testing.T) {
	oldRefreshToken := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	identity := authmodels.Identity{
		ID:          "rider-http-1",
		PhoneNumber: "+2348012345678",
		Status:      authmodels.StatusActive,
	}
	identityRepo := &refreshHTTPIdentityRepo{byPhone: map[string]authmodels.Identity{identity.PhoneNumber: identity}}
	sessionRepo := &refreshHTTPSessionRepo{sessions: map[string]authmodels.Session{
		"session-http-1": {
			ID:               "session-http-1",
			DispatchRiderID:  identity.ID,
			PhoneNumber:      identity.PhoneNumber,
			RefreshTokenHash: sha256Hex(oldRefreshToken),
			ExpiresAt:        time.Now().Add(30 * 24 * time.Hour),
		},
	}}
	router := buildRefreshHTTPRouter(identityRepo, sessionRepo)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refresh_token":"`+oldRefreshToken+`"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken      string `json:"access_token"`
			RefreshToken     string `json:"refresh_token"`
			TokenType        string `json:"token_type"`
			ExpiresInSeconds int64  `json:"expires_in_seconds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Success {
		t.Fatal("success must be true")
	}
	if resp.Data.AccessToken == "" {
		t.Fatal("access_token must not be empty")
	}
	if len(resp.Data.RefreshToken) != 64 {
		t.Fatalf("refresh_token len = %d, want 64", len(resp.Data.RefreshToken))
	}
	if resp.Data.TokenType != "Bearer" {
		t.Fatalf("token_type = %q, want Bearer", resp.Data.TokenType)
	}
	if resp.Data.ExpiresInSeconds != 900 {
		t.Fatalf("expires_in_seconds = %d, want 900", resp.Data.ExpiresInSeconds)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refresh_token":"`+oldRefreshToken+`"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("old token status = %d, want 401; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerRefreshMissingTokenReturnsValidationWithRequestID(t *testing.T) {
	router := buildRefreshHTTPRouter(
		&refreshHTTPIdentityRepo{byPhone: map[string]authmodels.Identity{}},
		&refreshHTTPSessionRepo{sessions: map[string]authmodels.Session{}},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "refresh-req-123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != string(apperrors.CodeValidationFailed) {
		t.Fatalf("code = %v, want validation_failed", errObj["code"])
	}
	if errObj["request_id"] != "refresh-req-123" {
		t.Fatalf("request_id = %v, want refresh-req-123", errObj["request_id"])
	}
}

func buildRefreshHTTPRouter(identities authrepositories.IdentityRepository, sessions authrepositories.SessionRepository) *gin.Engine {
	authSvc := authusecases.NewAuthUsecase(authusecases.Options{
		Identities:         identities,
		Sessions:           sessions,
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
	return r
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
