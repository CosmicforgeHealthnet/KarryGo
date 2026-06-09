package authhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// ── In-memory identity repo for signup/login handler tests ───────────────────

type slIdentityRepo struct {
	byPhone map[string]authmodels.Identity
	byEmail map[string]authmodels.Identity
}

func newSLIdentityRepo() *slIdentityRepo {
	return &slIdentityRepo{
		byPhone: make(map[string]authmodels.Identity),
		byEmail: make(map[string]authmodels.Identity),
	}
}

func (r *slIdentityRepo) seed(phone, email string) authmodels.Identity {
	id := authmodels.Identity{
		ID:          "id-" + phone,
		PhoneNumber: phone,
		Status:      authmodels.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if email != "" {
		id.Email = &email
	}
	r.byPhone[phone] = id
	if email != "" {
		r.byEmail[email] = id
	}
	return id
}

func (r *slIdentityRepo) FindByPhone(ctx context.Context, phone string) (authmodels.Identity, bool, error) {
	id, ok := r.byPhone[phone]
	return id, ok, nil
}
func (r *slIdentityRepo) FindByEmail(ctx context.Context, email string) (authmodels.Identity, bool, error) {
	id, ok := r.byEmail[email]
	return id, ok, nil
}
func (r *slIdentityRepo) GetByID(ctx context.Context, id string) (authmodels.Identity, bool, error) {
	for _, identity := range r.byPhone {
		if identity.ID == id {
			return identity, true, nil
		}
	}
	return authmodels.Identity{}, false, nil
}
func (r *slIdentityRepo) UpsertByPhone(ctx context.Context, phone string) (authmodels.Identity, error) {
	if existing, ok := r.byPhone[phone]; ok {
		return existing, nil
	}
	id := authmodels.Identity{ID: "id-" + phone, PhoneNumber: phone, Status: authmodels.StatusActive}
	r.byPhone[phone] = id
	return id, nil
}
func (r *slIdentityRepo) CreateForSignup(ctx context.Context, phone, email string) (authmodels.Identity, error) {
	if _, ok := r.byPhone[phone]; ok {
		return authmodels.Identity{}, apperrors.Conflict("An account with this phone number or email already exists.", nil)
	}
	var emailPtr *string
	if email != "" {
		if _, emailExists := r.byEmail[email]; emailExists {
			return authmodels.Identity{}, apperrors.Conflict("An account with this phone number or email already exists.", nil)
		}
		emailPtr = &email
	}
	id := authmodels.Identity{
		ID:          "id-" + phone,
		PhoneNumber: phone,
		Email:       emailPtr,
		Status:      authmodels.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	r.byPhone[phone] = id
	if emailPtr != nil {
		r.byEmail[email] = id
	}
	return id, nil
}

var _ authrepositories.IdentityRepository = (*slIdentityRepo)(nil)

// ── Session repo stub ─────────────────────────────────────────────────────────

type slSessionRepo struct {
	sessions map[string]authmodels.Session
}

func newSLSessionRepo() *slSessionRepo {
	return &slSessionRepo{sessions: make(map[string]authmodels.Session)}
}

func (r *slSessionRepo) Create(ctx context.Context, s authmodels.Session) (authmodels.Session, error) {
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	r.sessions[s.ID] = s
	return s, nil
}
func (r *slSessionRepo) FindByRefreshTokenHash(ctx context.Context, hash string) (authmodels.Session, bool, error) {
	for _, s := range r.sessions {
		if s.RefreshTokenHash == hash && s.RevokedAt == nil {
			return s, true, nil
		}
	}
	return authmodels.Session{}, false, nil
}
func (r *slSessionRepo) GetByID(ctx context.Context, id string) (authmodels.Session, bool, error) {
	s, ok := r.sessions[id]
	return s, ok, nil
}
func (r *slSessionRepo) RotateRefreshToken(ctx context.Context, id string, hash string) error {
	s, ok := r.sessions[id]
	if !ok {
		return authrepositories.ErrSessionNotFound
	}
	s.RefreshTokenHash = hash
	r.sessions[id] = s
	return nil
}
func (r *slSessionRepo) Revoke(ctx context.Context, id string) error {
	s, ok := r.sessions[id]
	if !ok {
		return authrepositories.ErrSessionNotFound
	}
	now := time.Now()
	s.RevokedAt = &now
	r.sessions[id] = s
	return nil
}

var _ authrepositories.SessionRepository = (*slSessionRepo)(nil)

// ── Router builder for signup/login tests ─────────────────────────────────────

func buildSLRouter(identityRepo authrepositories.IdentityRepository, otpRepo authrepositories.OTPRepository) (*gin.Engine, *authusecases.OTPUsecase) {
	otpUC := authusecases.NewOTPUsecase(authusecases.OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 10,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	authSvc := authusecases.NewAuthUsecase(authusecases.Options{
		OTPUsecase:         otpUC,
		Identities:         identityRepo,
		Sessions:           newSLSessionRepo(),
		Notifier:           &slFakeNotifier{},
		Publisher:          &slFakePublisher{},
		AccessTokenSecret:  []byte("access-secret-32-bytes-long-xxxx"),
		RefreshTokenSecret: []byte("refresh-secret-32-bytes-long-xxx"),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
		OTPDebug:           true, // enable for test visibility
	})

	r := gin.New()
	r.Use(httpx.RequestID())
	r.Use(httpx.Recovery())
	r.Use(httpx.ErrorHandler())
	authGroup := r.Group("/api/v1/auth")
	RegisterRoutes(authGroup, authSvc)
	return r, otpUC
}

type slFakeNotifier struct{}

func (n *slFakeNotifier) SendOTP(ctx context.Context, phone, otp string) error { return nil }

type slFakePublisher struct{}

func (p *slFakePublisher) PublishOTPRequested(ctx context.Context, e authclients.OTPRequestedEvent) error {
	return nil
}
func (p *slFakePublisher) PublishSessionCreated(ctx context.Context, e authclients.SessionCreatedEvent) error {
	return nil
}
func (p *slFakePublisher) PublishLoggedOut(ctx context.Context, e authclients.LoggedOutEvent) error {
	return nil
}

// ── Route registration: endpoints must not 404 ────────────────────────────────

func TestRoutes_SignupStartNotFound(t *testing.T) {
	r, _ := buildSLRouter(newSLIdentityRepo(), newHandlerFakeOTPRepo())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup/start", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatalf("POST /api/v1/auth/signup/start returned 404 — route is not registered")
	}
}

func TestRoutes_LoginStartNotFound(t *testing.T) {
	r, _ := buildSLRouter(newSLIdentityRepo(), newHandlerFakeOTPRepo())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/start", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatalf("POST /api/v1/auth/login/start returned 404 — route is not registered")
	}
}

func TestRoutes_VerifyNotFound(t *testing.T) {
	r, _ := buildSLRouter(newSLIdentityRepo(), newHandlerFakeOTPRepo())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatalf("POST /api/v1/auth/verify returned 404 — route is not registered")
	}
}

// ── SignupStart handler tests ─────────────────────────────────────────────────

func TestHandlerSignupStart_Success(t *testing.T) {
	r, _ := buildSLRouter(newSLIdentityRepo(), newHandlerFakeOTPRepo())

	body := `{"phone_number":"+2348012345678","email":"newuser@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		t.Fatal("data field is missing")
	}
	if data["expires_in_seconds"] == nil {
		t.Error("expires_in_seconds is missing")
	}
}

func TestHandlerSignupStart_OTPNotInResponse(t *testing.T) {
	r, _ := buildSLRouter(newSLIdentityRepo(), newHandlerFakeOTPRepo())

	body := `{"phone_number":"+2348012345678","email":"newuser@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	raw := w.Body.String()
	for _, suspect := range []string{"\"otp\"", "otp_code", "debug_otp"} {
		if containsCI(raw, suspect) {
			t.Errorf("response body contains OTP key %q: %s", suspect, raw)
		}
	}
}

func TestHandlerSignupStart_DuplicatePhone_Conflict(t *testing.T) {
	idRepo := newSLIdentityRepo()
	idRepo.seed("+2348012345678", "existing@example.com")

	r, _ := buildSLRouter(idRepo, newHandlerFakeOTPRepo())

	body := `{"phone_number":"+2348012345678","email":"newuser@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != string(apperrors.CodeConflict) {
		t.Errorf("code = %v, want conflict", errObj["code"])
	}
}

func TestHandlerSignupStart_DuplicateEmail_Conflict(t *testing.T) {
	idRepo := newSLIdentityRepo()
	idRepo.seed("+2347012345678", "taken@example.com")

	r, _ := buildSLRouter(idRepo, newHandlerFakeOTPRepo())

	// Different phone, same email
	body := `{"phone_number":"+2348012345678","email":"taken@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerSignupStart_InvalidPhone(t *testing.T) {
	r, _ := buildSLRouter(newSLIdentityRepo(), newHandlerFakeOTPRepo())

	body := `{"phone_number":"08012345678","email":"new@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerSignupStart_InvalidEmail(t *testing.T) {
	r, _ := buildSLRouter(newSLIdentityRepo(), newHandlerFakeOTPRepo())

	body := `{"phone_number":"+2348012345678","email":"not-an-email"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
}

// ── LoginStart handler tests ──────────────────────────────────────────────────

func TestHandlerLoginStart_SuccessByPhone(t *testing.T) {
	idRepo := newSLIdentityRepo()
	idRepo.seed("+2348012345678", "user@example.com")

	r, _ := buildSLRouter(idRepo, newHandlerFakeOTPRepo())

	body := `{"identifier":"+2348012345678"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}
}

func TestHandlerLoginStart_SuccessByEmail(t *testing.T) {
	idRepo := newSLIdentityRepo()
	idRepo.seed("+2348012345678", "user@example.com")

	r, _ := buildSLRouter(idRepo, newHandlerFakeOTPRepo())

	body := `{"identifier":"user@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerLoginStart_AccountNotFound_ByPhone(t *testing.T) {
	r, _ := buildSLRouter(newSLIdentityRepo(), newHandlerFakeOTPRepo())

	body := `{"identifier":"+2348099999999"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != string(apperrors.CodeNotFound) {
		t.Errorf("code = %v, want not_found", errObj["code"])
	}
}

func TestHandlerLoginStart_AccountNotFound_ByEmail(t *testing.T) {
	r, _ := buildSLRouter(newSLIdentityRepo(), newHandlerFakeOTPRepo())

	body := `{"identifier":"nobody@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerLoginStart_EmptyIdentifier(t *testing.T) {
	r, _ := buildSLRouter(newSLIdentityRepo(), newHandlerFakeOTPRepo())

	body := `{"identifier":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("expected error for empty identifier, got 200; body = %s", w.Body.String())
	}
}

func TestHandlerLoginStart_OTPNotInResponse(t *testing.T) {
	idRepo := newSLIdentityRepo()
	idRepo.seed("+2348012345678", "user@example.com")

	r, _ := buildSLRouter(idRepo, newHandlerFakeOTPRepo())

	body := `{"identifier":"+2348012345678"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	raw := w.Body.String()
	for _, suspect := range []string{"\"otp\"", "otp_code", "debug_otp"} {
		if containsCI(raw, suspect) {
			t.Errorf("response body contains OTP key %q: %s", suspect, raw)
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func containsCI(s, sub string) bool {
	return len(s) >= len(sub) &&
		bytes.Contains(bytes.ToLower([]byte(s)), bytes.ToLower([]byte(sub)))
}
