package authhttp

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

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	authclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/clients"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	authrepositories "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

// ── Fakes specific to verify handler tests ────────────────────────────────────

type verifyOTPRepo struct {
	otps   map[string]authmodels.OTP
	latest map[string]authmodels.OTP
}

func newVerifyOTPRepo() *verifyOTPRepo {
	return &verifyOTPRepo{otps: make(map[string]authmodels.OTP), latest: make(map[string]authmodels.OTP)}
}

func (f *verifyOTPRepo) Create(ctx context.Context, otp authmodels.OTP) (authmodels.OTP, error) {
	otp.CreatedAt = time.Now()
	otp.UpdatedAt = time.Now()
	f.otps[otp.ID] = otp
	f.latest[otp.PhoneNumber] = otp
	return otp, nil
}
func (f *verifyOTPRepo) LatestByPhone(ctx context.Context, phone string) (authmodels.OTP, bool, error) {
	otp, ok := f.latest[phone]
	return otp, ok, nil
}
func (f *verifyOTPRepo) MarkVerified(ctx context.Context, id string) error {
	if otp, ok := f.otps[id]; ok {
		otp.Verified = true
		f.otps[id] = otp
		if l := f.latest[otp.PhoneNumber]; l.ID == id {
			l.Verified = true
			f.latest[otp.PhoneNumber] = l
		}
	}
	return nil
}
func (f *verifyOTPRepo) RecordFailedAttempt(ctx context.Context, id string, attempts int, lockedUntil *time.Time) error {
	if otp, ok := f.otps[id]; ok {
		otp.Attempts = attempts
		otp.LockedUntil = lockedUntil
		f.otps[id] = otp
		if l := f.latest[otp.PhoneNumber]; l.ID == id {
			l.Attempts = attempts
			l.LockedUntil = lockedUntil
			f.latest[otp.PhoneNumber] = l
		}
	}
	return nil
}

func (f *verifyOTPRepo) LatestByPhoneAndPurpose(ctx context.Context, phone, purpose string) (authmodels.OTP, bool, error) {
	otp, ok := f.latest[phone]
	if ok && otp.Purpose == purpose {
		return otp, true, nil
	}
	return authmodels.OTP{}, false, nil
}

var _ authrepositories.OTPRepository = (*verifyOTPRepo)(nil)

type verifyIdentityRepo struct {
	identities map[string]authmodels.Identity
}

func newVerifyIdentityRepo() *verifyIdentityRepo {
	return &verifyIdentityRepo{identities: make(map[string]authmodels.Identity)}
}
func (r *verifyIdentityRepo) FindByPhone(ctx context.Context, phone string) (authmodels.Identity, bool, error) {
	id, ok := r.identities[phone]
	return id, ok, nil
}
func (r *verifyIdentityRepo) FindByEmail(ctx context.Context, email string) (authmodels.Identity, bool, error) {
	for _, identity := range r.identities {
		if identity.Email != nil && *identity.Email == email {
			return identity, true, nil
		}
	}
	return authmodels.Identity{}, false, nil
}
func (r *verifyIdentityRepo) GetByID(ctx context.Context, id string) (authmodels.Identity, bool, error) {
	for _, identity := range r.identities {
		if identity.ID == id {
			return identity, true, nil
		}
	}
	return authmodels.Identity{}, false, nil
}
func (r *verifyIdentityRepo) UpsertByPhone(ctx context.Context, phone string) (authmodels.Identity, error) {
	if existing, ok := r.identities[phone]; ok {
		return existing, nil
	}
	id := authmodels.Identity{
		ID: "htest-id-" + phone, PhoneNumber: phone, Status: authmodels.StatusActive,
	}
	r.identities[phone] = id
	return id, nil
}
func (r *verifyIdentityRepo) CreateForSignup(ctx context.Context, phone, email string) (authmodels.Identity, error) {
	id := authmodels.Identity{
		ID: "htest-id-" + phone, PhoneNumber: phone, Status: authmodels.StatusActive,
	}
	r.identities[phone] = id
	return id, nil
}

func (r *verifyIdentityRepo) UpdatePhone(_ context.Context, _, _, _ string) error { return nil }
func (r *verifyIdentityRepo) UpdateEmail(_ context.Context, _, _ string) error   { return nil }

var _ authrepositories.IdentityRepository = (*verifyIdentityRepo)(nil)

type verifySuspendedIdentityRepo struct{}

func (r *verifySuspendedIdentityRepo) FindByPhone(ctx context.Context, phone string) (authmodels.Identity, bool, error) {
	return authmodels.Identity{Status: authmodels.StatusSuspended}, true, nil
}
func (r *verifySuspendedIdentityRepo) FindByEmail(ctx context.Context, email string) (authmodels.Identity, bool, error) {
	return authmodels.Identity{}, false, nil
}
func (r *verifySuspendedIdentityRepo) GetByID(ctx context.Context, id string) (authmodels.Identity, bool, error) {
	return authmodels.Identity{ID: id, Status: authmodels.StatusSuspended}, true, nil
}
func (r *verifySuspendedIdentityRepo) UpsertByPhone(ctx context.Context, phone string) (authmodels.Identity, error) {
	return authmodels.Identity{ID: "sus-1", PhoneNumber: phone, Status: authmodels.StatusSuspended}, nil
}
func (r *verifySuspendedIdentityRepo) CreateForSignup(ctx context.Context, phone, email string) (authmodels.Identity, error) {
	return authmodels.Identity{ID: "sus-1", PhoneNumber: phone, Status: authmodels.StatusSuspended}, nil
}
func (r *verifySuspendedIdentityRepo) UpdatePhone(_ context.Context, _, _, _ string) error { return nil }
func (r *verifySuspendedIdentityRepo) UpdateEmail(_ context.Context, _, _ string) error   { return nil }

var _ authrepositories.IdentityRepository = (*verifySuspendedIdentityRepo)(nil)

type verifySessionRepo struct{}

func (r *verifySessionRepo) Create(ctx context.Context, s authmodels.Session) (authmodels.Session, error) {
	s.CreatedAt = time.Now()
	return s, nil
}
func (r *verifySessionRepo) FindByRefreshTokenHash(ctx context.Context, h string) (authmodels.Session, bool, error) {
	return authmodels.Session{}, false, nil
}
func (r *verifySessionRepo) GetByID(ctx context.Context, id string) (authmodels.Session, bool, error) {
	return authmodels.Session{}, false, nil
}
func (r *verifySessionRepo) RotateRefreshToken(ctx context.Context, id string, hash string) error {
	return nil
}
func (r *verifySessionRepo) Revoke(ctx context.Context, id string) error { return nil }
func (r *verifySessionRepo) RevokeAllByDispatchRiderID(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

var _ authrepositories.SessionRepository = (*verifySessionRepo)(nil)

type verifyPublisher struct{}

func (p *verifyPublisher) PublishOTPRequested(ctx context.Context, e authclients.OTPRequestedEvent) error {
	return nil
}
func (p *verifyPublisher) PublishSessionCreated(ctx context.Context, e authclients.SessionCreatedEvent) error {
	return nil
}
func (p *verifyPublisher) PublishLoggedOut(ctx context.Context, e authclients.LoggedOutEvent) error {
	return nil
}

func (p *verifyPublisher) PublishPhoneChanged(ctx context.Context, e authclients.PhoneChangedEvent) error {
	return nil
}

// ── Test router helpers ───────────────────────────────────────────────────────

func buildVerifyTestRouter(otpRepo *verifyOTPRepo, idRepo authrepositories.IdentityRepository) *gin.Engine {
	otpUC := authusecases.NewOTPUsecase(authusecases.OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	authSvc := authusecases.NewAuthUsecase(authusecases.Options{
		OTPUsecase:         otpUC,
		Identities:         idRepo,
		Sessions:           &verifySessionRepo{},
		Publisher:          &verifyPublisher{},
		AccessTokenSecret:  []byte("access-secret-32-bytes-long-xxxx"),
		RefreshTokenSecret: []byte("refresh-secret-32-bytes-long-xxx"),
		OTPDebug:           false,
	})

	r := gin.New()
	r.Use(httpx.RequestID())
	r.Use(httpx.Recovery())
	r.Use(httpx.ErrorHandler())
	RegisterRoutes(r.Group("/api/v1/auth"), authSvc)
	return r
}

// seedOTP plants a fresh OTP via the OTPUsecase and returns the plain code.
func seedOTP(otpRepo *verifyOTPRepo, phone string) string {
	otpUC := authusecases.NewOTPUsecase(authusecases.OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	_, code, _ := otpUC.Create(context.Background(), phone)
	return code
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestHandlerVerifySuccess(t *testing.T) {
	otpRepo := newVerifyOTPRepo()
	code := seedOTP(otpRepo, "+2348012345678")

	r := buildVerifyTestRouter(otpRepo, newVerifyIdentityRepo())
	body, _ := json.Marshal(map[string]any{
		"phone_number": "+2348012345678",
		"otp_code":     code,
		"device_id":    "dev-001",
		"device_type":  "android",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewReader(body))
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
	for _, field := range []string{"provider_id", "role", "access_token", "refresh_token", "token_type", "expires_in_seconds"} {
		if data[field] == nil || data[field] == "" {
			t.Errorf("response.data.%s is missing or empty", field)
		}
	}
	if data["role"] != authmodels.RoleDispatchProvider {
		t.Errorf("role = %v, want dispatch_provider", data["role"])
	}
	if data["token_type"] != "Bearer" {
		t.Errorf("token_type = %v, want Bearer", data["token_type"])
	}
}

func TestHandlerVerifyTokensNotInErrorResponse(t *testing.T) {
	// Wrong OTP must not leak any token
	otpRepo := newVerifyOTPRepo()
	seedOTP(otpRepo, "+2348012345678")

	r := buildVerifyTestRouter(otpRepo, newVerifyIdentityRepo())
	body := `{"phone_number":"+2348012345678","otp_code":"000000"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatal("expected non-200 for wrong OTP")
	}
	raw := strings.ToLower(w.Body.String())
	for _, tok := range []string{"access_token", "refresh_token"} {
		if strings.Contains(raw, tok) {
			t.Errorf("error response must not contain %q", tok)
		}
	}
}

func TestHandlerVerifyInvalidPhone(t *testing.T) {
	r := buildVerifyTestRouter(newVerifyOTPRepo(), newVerifyIdentityRepo())
	body := `{"phone_number":"08012345678","otp_code":"123456"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != string(apperrors.CodeValidationFailed) {
		t.Errorf("code = %v, want validation_failed", errObj["code"])
	}
}

func TestHandlerVerifyInvalidOTPFormat(t *testing.T) {
	r := buildVerifyTestRouter(newVerifyOTPRepo(), newVerifyIdentityRepo())
	body := `{"phone_number":"+2348012345678","otp_code":"abc123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("expected error for invalid OTP format; body = %s", w.Body.String())
	}
	assertVerifyValidationField(t, w.Body.Bytes(), "otp_code")
}

func TestHandlerVerifyMissingOTPCodeReturnsOTPCodeField(t *testing.T) {
	r := buildVerifyTestRouter(newVerifyOTPRepo(), newVerifyIdentityRepo())
	body := `{"phone_number":"+2348012345678"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
	assertVerifyValidationField(t, w.Body.Bytes(), "otp_code")
}

func TestHandlerVerifyNoOTPRecord(t *testing.T) {
	// No OTP seeded — should return error
	r := buildVerifyTestRouter(newVerifyOTPRepo(), newVerifyIdentityRepo())
	body := `{"phone_number":"+2348012345678","otp_code":"123456"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("expected error when no OTP exists; body = %s", w.Body.String())
	}
}

func TestHandlerVerifyWrongOTP(t *testing.T) {
	otpRepo := newVerifyOTPRepo()
	seedOTP(otpRepo, "+2348012345678")

	r := buildVerifyTestRouter(otpRepo, newVerifyIdentityRepo())
	body := `{"phone_number":"+2348012345678","otp_code":"000000"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerVerifySuspendedIdentityForbidden(t *testing.T) {
	otpRepo := newVerifyOTPRepo()
	code := seedOTP(otpRepo, "+2348012345678")

	r := buildVerifyTestRouter(otpRepo, &verifySuspendedIdentityRepo{})
	body, _ := json.Marshal(map[string]any{"phone_number": "+2348012345678", "otp_code": code})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != string(apperrors.CodeForbidden) {
		t.Errorf("code = %v, want forbidden", errObj["code"])
	}
}

func TestHandlerVerifyRequestIDInErrorResponse(t *testing.T) {
	r := buildVerifyTestRouter(newVerifyOTPRepo(), newVerifyIdentityRepo())
	body := `{"phone_number":"+2348012345678","otp_code":"123456"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "verify-req-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj["request_id"] != "verify-req-123" {
		t.Errorf("request_id = %v, want verify-req-123", errObj["request_id"])
	}
}

func TestHandlerVerifyInvalidJSON(t *testing.T) {
	r := buildVerifyTestRouter(newVerifyOTPRepo(), newVerifyIdentityRepo())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}

func assertVerifyValidationField(t *testing.T, body []byte, want string) {
	t.Helper()

	var resp struct {
		Error struct {
			Code   string `json:"code"`
			Fields []struct {
				Field string `json:"field"`
			} `json:"fields"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error.Code != string(apperrors.CodeValidationFailed) {
		t.Fatalf("code = %q, want validation_failed", resp.Error.Code)
	}
	if len(resp.Error.Fields) == 0 {
		t.Fatalf("expected validation fields, got none")
	}
	if resp.Error.Fields[0].Field != want {
		t.Fatalf("field = %q, want %q", resp.Error.Fields[0].Field, want)
	}
}
