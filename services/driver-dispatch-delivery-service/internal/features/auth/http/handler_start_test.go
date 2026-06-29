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

func init() {
	gin.SetMode(gin.TestMode)
}

// ── fakes re-declared locally (handler package) ───────────────────────────────

type handlerFakeOTPRepo struct {
	otps   map[string]authmodels.OTP
	latest map[string]authmodels.OTP
}

func newHandlerFakeOTPRepo() *handlerFakeOTPRepo {
	return &handlerFakeOTPRepo{
		otps:   make(map[string]authmodels.OTP),
		latest: make(map[string]authmodels.OTP),
	}
}

func (f *handlerFakeOTPRepo) Create(ctx context.Context, otp authmodels.OTP) (authmodels.OTP, error) {
	otp.CreatedAt = time.Now()
	otp.UpdatedAt = time.Now()
	f.otps[otp.ID] = otp
	f.latest[otp.PhoneNumber] = otp
	return otp, nil
}
func (f *handlerFakeOTPRepo) LatestByPhone(ctx context.Context, phone string) (authmodels.OTP, bool, error) {
	otp, ok := f.latest[phone]
	return otp, ok, nil
}
func (f *handlerFakeOTPRepo) MarkVerified(ctx context.Context, id string) error { return nil }
func (f *handlerFakeOTPRepo) RecordFailedAttempt(ctx context.Context, id string, attempts int, lockedUntil *time.Time) error {
	return nil
}
func (f *handlerFakeOTPRepo) LatestByPhoneAndPurpose(ctx context.Context, phone, purpose string) (authmodels.OTP, bool, error) {
	otp, ok := f.latest[phone]
	if ok && otp.Purpose == purpose {
		return otp, true, nil
	}
	return authmodels.OTP{}, false, nil
}

// compile-time check
var _ authrepositories.OTPRepository = (*handlerFakeOTPRepo)(nil)

type handlerFakeNotifier struct{}

func (n *handlerFakeNotifier) SendOTP(ctx context.Context, phone, otp string) error { return nil }

type handlerFakePublisher struct{}

func (p *handlerFakePublisher) PublishOTPRequested(ctx context.Context, event authclients.OTPRequestedEvent) error {
	return nil
}

func (p *handlerFakePublisher) PublishSessionCreated(ctx context.Context, event authclients.SessionCreatedEvent) error {
	return nil
}

func (p *handlerFakePublisher) PublishLoggedOut(ctx context.Context, event authclients.LoggedOutEvent) error {
	return nil
}

func (p *handlerFakePublisher) PublishPhoneChanged(ctx context.Context, event authclients.PhoneChangedEvent) error {
	return nil
}

// ── router builder ─────────────────────────────────────────────────────────────

func buildTestRouter(otpUC *authusecases.OTPUsecase) *gin.Engine {
	authSvc := authusecases.NewAuthUsecase(authusecases.Options{
		OTPUsecase:         otpUC,
		Notifier:           &handlerFakeNotifier{},
		Publisher:          &handlerFakePublisher{},
		AccessTokenSecret:  []byte("access"),
		RefreshTokenSecret: []byte("refresh"),
		OTPDebug:           false,
	})

	r := gin.New()
	r.Use(httpx.RequestID())
	r.Use(httpx.Recovery())
	r.Use(httpx.ErrorHandler())

	authGroup := r.Group("/api/v1/auth")
	RegisterRoutes(authGroup, authSvc)
	return r
}

func buildDefaultTestRouter() *gin.Engine {
	otpUC := authusecases.NewOTPUsecase(authusecases.OTPOptions{
		Repository:  newHandlerFakeOTPRepo(),
		Secret:      []byte("test-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 3,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	return buildTestRouter(otpUC)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestPhoneChangeRoutesRequireJWT(t *testing.T) {
	router := buildDefaultTestRouter()
	for _, path := range []string{
		"/api/v1/auth/phone-change/start",
		"/api/v1/auth/phone-change/verify",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"new_phone_number":"+2348098765432","otp_code":"123456"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want 401; body = %s", path, w.Code, w.Body.String())
		}
	}
}

func TestHandlerStartSuccess(t *testing.T) {
	r := buildDefaultTestRouter()

	body := `{"phone_number":"+2348012345678"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		t.Fatal("data field is missing")
	}
	if data["message"] != "OTP sent successfully." {
		t.Errorf("message = %v, want 'OTP sent successfully.'", data["message"])
	}
	if data["expires_in_seconds"] == nil {
		t.Error("expires_in_seconds is missing")
	}
}

func TestHandlerStartOTPNotInResponse(t *testing.T) {
	r := buildDefaultTestRouter()

	body := `{"phone_number":"+2348012345678"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	raw := w.Body.String()
	lc := strings.ToLower(raw)
	// OTP must not appear in any form
	for _, suspect := range []string{"otp_code", "debug_otp", "otp\":", "\"otp\""} {
		if strings.Contains(lc, suspect) {
			t.Errorf("response body contains suspicious key %q: %s", suspect, raw)
		}
	}
}

func TestHandlerStartInvalidPhone_MissingPlus(t *testing.T) {
	r := buildDefaultTestRouter()

	body := `{"phone_number":"08012345678"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/start", bytes.NewBufferString(body))
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

func TestHandlerStartInvalidJSON(t *testing.T) {
	r := buildDefaultTestRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/start", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerStartRateLimit(t *testing.T) {
	callCount := 0
	otpUC := authusecases.NewOTPUsecase(authusecases.OTPOptions{
		Repository:  newHandlerFakeOTPRepo(),
		Secret:      []byte("test-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 3,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	}).WithRateLimiter(func(ctx context.Context, phone string) error {
		callCount++
		if callCount > 3 {
			return apperrors.RateLimited("Too many verification code requests. Try again later.", nil)
		}
		return nil
	})

	r := buildTestRouter(otpUC)
	body := `{"phone_number":"+2348012345678"}`

	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/start", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i, w.Code)
		}
	}

	// 4th request must be rate limited
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("4th request: status = %d, want 429; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != string(apperrors.CodeRateLimited) {
		t.Errorf("code = %v, want rate_limited", errObj["code"])
	}
}

func TestHandlerStartRequestIDInErrorResponse(t *testing.T) {
	r := buildDefaultTestRouter()

	body := `{"phone_number":"bad-number"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/start", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-req-123")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj["request_id"] != "test-req-123" {
		t.Errorf("request_id = %v, want test-req-123", errObj["request_id"])
	}
}
