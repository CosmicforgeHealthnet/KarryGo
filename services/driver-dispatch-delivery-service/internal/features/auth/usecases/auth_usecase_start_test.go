package authusecases

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	authclients "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/clients"
	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	"karrygo/shared/go/apperrors"
)

// ── Fakes ─────────────────────────────────────────────────────────────────────

type fakeOTPRepository struct {
	otps   map[string]authmodels.OTP
	latest map[string]authmodels.OTP
}

func newFakeOTPRepository() *fakeOTPRepository {
	return &fakeOTPRepository{
		otps:   make(map[string]authmodels.OTP),
		latest: make(map[string]authmodels.OTP),
	}
}

func (f *fakeOTPRepository) Create(ctx context.Context, otp authmodels.OTP) (authmodels.OTP, error) {
	otp.CreatedAt = time.Now()
	otp.UpdatedAt = time.Now()
	f.otps[otp.ID] = otp
	f.latest[otp.PhoneNumber] = otp
	return otp, nil
}
func (f *fakeOTPRepository) LatestByPhone(ctx context.Context, phone string) (authmodels.OTP, bool, error) {
	otp, ok := f.latest[phone]
	return otp, ok, nil
}
func (f *fakeOTPRepository) MarkVerified(ctx context.Context, id string) error {
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

func (f *fakeOTPRepository) RecordFailedAttempt(ctx context.Context, id string, attempts int, lockedUntil *time.Time) error {
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

type fakeNotifier struct {
	calls []struct{ phone, otp string }
}

func (f *fakeNotifier) SendOTP(ctx context.Context, phone, otp string) error {
	f.calls = append(f.calls, struct{ phone, otp string }{phone, otp})
	return nil
}

type fakePublisher struct {
	otpEvents       []authclients.OTPRequestedEvent
	sessionEvents   []authclients.SessionCreatedEvent
	loggedOutEvents []authclients.LoggedOutEvent
}

func (f *fakePublisher) PublishOTPRequested(ctx context.Context, event authclients.OTPRequestedEvent) error {
	f.otpEvents = append(f.otpEvents, event)
	return nil
}

func (f *fakePublisher) PublishSessionCreated(ctx context.Context, event authclients.SessionCreatedEvent) error {
	f.sessionEvents = append(f.sessionEvents, event)
	return nil
}

func (f *fakePublisher) PublishLoggedOut(ctx context.Context, event authclients.LoggedOutEvent) error {
	f.loggedOutEvents = append(f.loggedOutEvents, event)
	return nil
}

// errPublisher always returns an error on publish.
type errPublisher struct{}

func (e *errPublisher) PublishOTPRequested(ctx context.Context, event authclients.OTPRequestedEvent) error {
	return apperrors.Internal("redis publish failed", nil)
}

func (e *errPublisher) PublishSessionCreated(ctx context.Context, event authclients.SessionCreatedEvent) error {
	return apperrors.Internal("redis publish failed", nil)
}

func (e *errPublisher) PublishLoggedOut(ctx context.Context, event authclients.LoggedOutEvent) error {
	return apperrors.Internal("redis publish failed", nil)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func newStartUsecase(repo *fakeOTPRepository, notifier *fakeNotifier, pub authclients.EventPublisher, rateLimitFn func(context.Context, string) error) *AuthUsecase {
	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  repo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 3,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	if rateLimitFn != nil {
		otpUC = otpUC.WithRateLimiter(rateLimitFn)
	}
	return NewAuthUsecase(Options{
		OTPUsecase:         otpUC,
		Notifier:           notifier,
		Publisher:          pub,
		AccessTokenSecret:  []byte("access-secret"),
		RefreshTokenSecret: []byte("refresh-secret"),
		OTPDebug:           false,
	})
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestStartSuccess(t *testing.T) {
	repo := newFakeOTPRepository()
	notifier := &fakeNotifier{}
	pub := &fakePublisher{}

	svc := newStartUsecase(repo, notifier, pub, nil)
	result, err := svc.Start(context.Background(), StartInput{
		PhoneNumber:   "+2348012345678",
		CorrelationID: "corr-001",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if result.ExpiresInSeconds != 600 {
		t.Errorf("ExpiresInSeconds = %d, want 600", result.ExpiresInSeconds)
	}
}

func TestStartOTPNotInResult(t *testing.T) {
	repo := newFakeOTPRepository()
	pub := &fakePublisher{}

	svc := newStartUsecase(repo, &fakeNotifier{}, pub, nil)
	result, err := svc.Start(context.Background(), StartInput{PhoneNumber: "+2348012345678"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Marshal the result and ensure no OTP appears
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "otp") {
		t.Errorf("StartResult JSON contains 'otp': %s", raw)
	}
}

func TestStartInvalidPhone_MissingPlus(t *testing.T) {
	svc := newStartUsecase(newFakeOTPRepository(), &fakeNotifier{}, &fakePublisher{}, nil)
	_, err := svc.Start(context.Background(), StartInput{PhoneNumber: "08012345678"})
	if err == nil {
		t.Fatal("expected validation error for non-E.164 phone, got nil")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if appErr.Code != apperrors.CodeValidationFailed {
		t.Errorf("code = %q, want validation_failed", appErr.Code)
	}
}

func TestStartInvalidPhone_NoCountryCode(t *testing.T) {
	svc := newStartUsecase(newFakeOTPRepository(), &fakeNotifier{}, &fakePublisher{}, nil)
	_, err := svc.Start(context.Background(), StartInput{PhoneNumber: "2348012345678"})
	if err == nil {
		t.Fatal("expected validation error for number without +, got nil")
	}
}

func TestStartInvalidPhone_Empty(t *testing.T) {
	svc := newStartUsecase(newFakeOTPRepository(), &fakeNotifier{}, &fakePublisher{}, nil)
	_, err := svc.Start(context.Background(), StartInput{PhoneNumber: ""})
	if err == nil {
		t.Fatal("expected validation error for empty phone, got nil")
	}
}

func TestStartRateLimit(t *testing.T) {
	callCount := 0
	rateLimitFn := func(ctx context.Context, phone string) error {
		callCount++
		if callCount > 3 {
			return apperrors.RateLimited("Too many verification code requests. Try again later.", nil)
		}
		return nil
	}

	repo := newFakeOTPRepository()
	pub := &fakePublisher{}
	svc := newStartUsecase(repo, &fakeNotifier{}, pub, rateLimitFn)

	ctx := context.Background()
	phone := "+2348012345678"

	// First 3 requests must succeed
	for i := 1; i <= 3; i++ {
		if _, err := svc.Start(ctx, StartInput{PhoneNumber: phone}); err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
	}

	// 4th request must be rate limited
	_, err := svc.Start(ctx, StartInput{PhoneNumber: phone})
	if err == nil {
		t.Fatal("4th request: expected rate_limited error, got nil")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if appErr.Code != apperrors.CodeRateLimited {
		t.Errorf("code = %q, want rate_limited", appErr.Code)
	}
	if len(repo.otps) != 3 {
		t.Fatalf("stored OTP count = %d, want 3; rate-limited request must not create an OTP", len(repo.otps))
	}
	if len(pub.otpEvents) != 3 {
		t.Fatalf("published OTP events = %d, want 3; rate-limited request must not publish", len(pub.otpEvents))
	}
}

func TestStartInvalidPhoneDoesNotCreateOTP(t *testing.T) {
	repo := newFakeOTPRepository()
	pub := &fakePublisher{}
	svc := newStartUsecase(repo, &fakeNotifier{}, pub, nil)

	_, err := svc.Start(context.Background(), StartInput{PhoneNumber: "08012345678"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if len(repo.otps) != 0 {
		t.Fatalf("stored OTP count = %d, want 0", len(repo.otps))
	}
	if len(pub.otpEvents) != 0 {
		t.Fatalf("published OTP events = %d, want 0", len(pub.otpEvents))
	}
}

func TestStartEventPublished(t *testing.T) {
	repo := newFakeOTPRepository()
	pub := &fakePublisher{}
	notifier := &fakeNotifier{}

	svc := newStartUsecase(repo, notifier, pub, nil)
	_, err := svc.Start(context.Background(), StartInput{
		PhoneNumber:   "+2348012345678",
		CorrelationID: "corr-xyz",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if len(pub.otpEvents) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.otpEvents))
	}
	ev := pub.otpEvents[0]
	if ev.Event != authclients.TopicOTPRequested {
		t.Errorf("event.Event = %q, want %q", ev.Event, authclients.TopicOTPRequested)
	}
	if ev.CorrelationID != "corr-xyz" {
		t.Errorf("event.CorrelationID = %q, want %q", ev.CorrelationID, "corr-xyz")
	}
	if ev.PhoneNumber != "+2348012345678" {
		t.Errorf("event.PhoneNumber = %q, want +2348012345678", ev.PhoneNumber)
	}
	if ev.Purpose != "login" {
		t.Errorf("event.Purpose = %q, want login", ev.Purpose)
	}
	if ev.CreatedAt.IsZero() {
		t.Error("event.CreatedAt must be populated")
	}
}

func TestStartEventContainsOTPCode(t *testing.T) {
	repo := newFakeOTPRepository()
	pub := &fakePublisher{}
	notifier := &fakeNotifier{}

	svc := newStartUsecase(repo, notifier, pub, nil)
	_, err := svc.Start(context.Background(), StartInput{PhoneNumber: "+2348012345678"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(pub.otpEvents) == 0 {
		t.Fatal("no event published")
	}
	if len(pub.otpEvents[0].OTPCode) != 6 {
		t.Errorf("event.OTPCode len = %d, want 6", len(pub.otpEvents[0].OTPCode))
	}
	for _, c := range pub.otpEvents[0].OTPCode {
		if c < '0' || c > '9' {
			t.Errorf("event.OTPCode contains non-digit char %q", c)
		}
	}
}

func TestStartDevOTPLoggingGated(t *testing.T) {
	repo := newFakeOTPRepository()
	notifier := &fakeNotifier{}
	pub := &fakePublisher{}

	// With OTPDebug=false, notifier receives the code but in prod it should not log it.
	// The LoggingNotificationClient gates its log.Printf on debugOTP == true.
	// Here we just verify the notifier was called (code reached it).
	svc := newStartUsecase(repo, notifier, pub, nil)
	_, err := svc.Start(context.Background(), StartInput{PhoneNumber: "+2348012345678"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("notifier.calls = %d, want 1", len(notifier.calls))
	}
	// The notifier received the plain OTP — it's responsible for gating production logs.
	// Verify the OTP passed to notifier is 6 digits.
	otp := notifier.calls[0].otp
	if len(otp) != 6 {
		t.Errorf("notifier otp len = %d, want 6", len(otp))
	}
}

func TestStartPublishError_ReturnsInternalError(t *testing.T) {
	svc := newStartUsecase(newFakeOTPRepository(), &fakeNotifier{}, &errPublisher{}, nil)
	_, err := svc.Start(context.Background(), StartInput{PhoneNumber: "+2348012345678"})
	if err == nil {
		t.Fatal("expected internal error from publish failure, got nil")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if appErr.Code != apperrors.CodeInternal {
		t.Errorf("code = %q, want internal_error", appErr.Code)
	}
}

func TestStartOTPStoredAsHash(t *testing.T) {
	repo := newFakeOTPRepository()
	pub := &fakePublisher{}

	svc := newStartUsecase(repo, &fakeNotifier{}, pub, nil)
	_, err := svc.Start(context.Background(), StartInput{PhoneNumber: "+2348012345678"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Exactly one OTP row stored
	if len(repo.otps) != 1 {
		t.Fatalf("expected 1 OTP row, got %d", len(repo.otps))
	}
	for _, otp := range repo.otps {
		// Hash must not equal a 6-digit plain code
		if len(otp.OTPCodeHash) == 6 {
			t.Error("OTPCodeHash looks like a plain 6-digit code — must be a hash")
		}
		// verified must be false, attempts must be 0
		if otp.Verified {
			t.Error("new OTP must have verified=false")
		}
		if otp.Attempts != 0 {
			t.Errorf("new OTP attempts = %d, want 0", otp.Attempts)
		}
		if otp.LockedUntil != nil {
			t.Error("new OTP must have locked_until=nil")
		}
	}
}
