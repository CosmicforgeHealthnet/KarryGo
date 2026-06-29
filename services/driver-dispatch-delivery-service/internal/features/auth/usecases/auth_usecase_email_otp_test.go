package authusecases

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	authclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/clients"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
)

// ── Fake email client ────────────────────────────────────────────────────────

type fakeEmailClient struct {
	mu    sync.Mutex
	calls []struct{ to, code string }
	err   error
}

func (f *fakeEmailClient) SendOTP(_ context.Context, to, code string) error {
	f.mu.Lock()
	f.calls = append(f.calls, struct{ to, code string }{to, code})
	f.mu.Unlock()
	return f.err
}

func (f *fakeEmailClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeEmailClient) lastCall() (to, code string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return "", ""
	}
	c := f.calls[len(f.calls)-1]
	return c.to, c.code
}

// yieldForGoroutine gives the goroutine scheduler a chance to run the
// fire-and-forget email goroutine before we make assertions.
func yieldForGoroutine() {
	for i := 0; i < 20; i++ {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
}

// ── Helper constructors ───────────────────────────────────────────────────────

func newStartUsecaseWithEmail(
	notifier *fakeNotifier,
	pub authclients.EventPublisher,
	emailClient authclients.EmailClient,
) *AuthUsecase {
	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  newFakeOTPRepository(),
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	return NewAuthUsecase(Options{
		OTPUsecase:         otpUC,
		Notifier:           notifier,
		EmailClient:        emailClient,
		Publisher:          pub,
		AccessTokenSecret:  []byte("access-secret"),
		RefreshTokenSecret: []byte("refresh-secret"),
		OTPDebug:           false,
	})
}

func newSignupLoginUsecaseWithEmail(
	idRepo *fakeIdentityRepository,
	notifier *fakeNotifier,
	pub *fakePublisher,
	emailClient authclients.EmailClient,
) *AuthUsecase {
	otpRepo := newFakeOTPRepository()
	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 10,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	return NewAuthUsecase(Options{
		OTPUsecase:         otpUC,
		Identities:         idRepo,
		Notifier:           notifier,
		EmailClient:        emailClient,
		Publisher:          pub,
		AccessTokenSecret:  []byte("access-secret-32-bytes-long-xxxx"),
		RefreshTokenSecret: []byte("refresh-secret-32-bytes-long-xxx"),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
		OTPDebug:           false,
	})
}

// ── Start (legacy) + email ────────────────────────────────────────────────────

// TestStart_PhoneOnly_NoEmailClient verifies that Start still succeeds when no
// email client is wired in (SMS-only backward compat).
func TestStart_PhoneOnly_NoEmailClient(t *testing.T) {
	notifier := &fakeNotifier{}
	svc := newStartUsecaseWithEmail(notifier, &fakePublisher{}, nil)

	_, err := svc.Start(context.Background(), StartInput{PhoneNumber: "+2348012345678"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("notifier calls = %d, want 1", len(notifier.calls))
	}
}

// TestStart_WithEmail_SendsToEmailClient verifies that providing an email in
// StartInput causes the email client to receive the same OTP code.
func TestStart_WithEmail_SendsToEmailClient(t *testing.T) {
	notifier := &fakeNotifier{}
	ec := &fakeEmailClient{}
	svc := newStartUsecaseWithEmail(notifier, &fakePublisher{}, ec)

	_, err := svc.Start(context.Background(), StartInput{
		PhoneNumber: "+2348012345678",
		Email:       "rider@example.com",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	yieldForGoroutine()

	if ec.callCount() != 1 {
		t.Fatalf("email client calls = %d, want 1", ec.callCount())
	}
	to, code := ec.lastCall()
	if to != "rider@example.com" {
		t.Errorf("email to = %q, want rider@example.com", to)
	}
	// Same 6-digit code goes to both channels.
	smsCode := notifier.calls[0].otp
	if code != smsCode {
		t.Errorf("email OTP code %q does not match SMS OTP code %q", code, smsCode)
	}
}

// TestStart_EmailFailure_DoesNotBlockSMS verifies that an email client error
// never causes Start to return an error — SMS is the primary channel.
func TestStart_EmailFailure_DoesNotBlockSMS(t *testing.T) {
	notifier := &fakeNotifier{}
	ec := &fakeEmailClient{err: errors.New("smtp connection refused")}
	svc := newStartUsecaseWithEmail(notifier, &fakePublisher{}, ec)

	_, err := svc.Start(context.Background(), StartInput{
		PhoneNumber: "+2348012345678",
		Email:       "rider@example.com",
	})
	if err != nil {
		t.Fatalf("Start() returned error despite email failure: %v", err)
	}
	// SMS was still delivered.
	if len(notifier.calls) != 1 {
		t.Fatalf("notifier calls = %d, want 1", len(notifier.calls))
	}
}

// TestStart_NoEmail_EmailClientNotCalled verifies that no email is sent when
// the request has no email field (phone-only request).
func TestStart_NoEmail_EmailClientNotCalled(t *testing.T) {
	ec := &fakeEmailClient{}
	svc := newStartUsecaseWithEmail(&fakeNotifier{}, &fakePublisher{}, ec)

	_, err := svc.Start(context.Background(), StartInput{PhoneNumber: "+2348012345678"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	yieldForGoroutine()

	if ec.callCount() != 0 {
		t.Fatalf("email client called %d times, want 0 (no email in request)", ec.callCount())
	}
}

// ── SignupStart + email ───────────────────────────────────────────────────────

// TestSignupStart_SendsEmailViaDualChannel verifies that SignupStart sends the
// same OTP to both SMS (via notifier) and email (via emailClient).
func TestSignupStart_SendsEmailViaDualChannel(t *testing.T) {
	idRepo := newFakeIdentityRepository()
	notifier := &fakeNotifier{}
	ec := &fakeEmailClient{}
	pub := &fakePublisher{}

	svc := newSignupLoginUsecaseWithEmail(idRepo, notifier, pub, ec)
	_, err := svc.SignupStart(context.Background(), SignupStartInput{
		PhoneNumber: "+2348012345678",
		Email:       "newrider@example.com",
	})
	if err != nil {
		t.Fatalf("SignupStart() error = %v", err)
	}

	yieldForGoroutine()

	if ec.callCount() != 1 {
		t.Fatalf("email client calls = %d, want 1", ec.callCount())
	}
	to, emailCode := ec.lastCall()
	if to != "newrider@example.com" {
		t.Errorf("email to = %q, want newrider@example.com", to)
	}
	smsCode := notifier.calls[0].otp
	if emailCode != smsCode {
		t.Errorf("email OTP %q != SMS OTP %q — must be the same code", emailCode, smsCode)
	}
}

// TestSignupStart_EmailFailure_DoesNotBlock verifies signup OTP succeeds even
// when the email client fails.
func TestSignupStart_EmailFailure_DoesNotBlock(t *testing.T) {
	idRepo := newFakeIdentityRepository()
	ec := &fakeEmailClient{err: errors.New("smtp timeout")}
	svc := newSignupLoginUsecaseWithEmail(idRepo, &fakeNotifier{}, &fakePublisher{}, ec)

	_, err := svc.SignupStart(context.Background(), SignupStartInput{
		PhoneNumber: "+2348012345678",
		Email:       "newrider@example.com",
	})
	if err != nil {
		t.Fatalf("SignupStart() returned error despite email failure: %v", err)
	}
}

// ── LoginStart + stored email ─────────────────────────────────────────────────

// TestLoginStart_StoredEmail_SentOnLogin verifies that when an identity has an
// email on record, LoginStart sends the OTP to that stored email address.
func TestLoginStart_StoredEmail_SentOnLogin(t *testing.T) {
	idRepo := newFakeIdentityRepository()
	storedEmail := "stored@example.com"
	idRepo.identities["+2348012345678"] = authmodels.Identity{
		ID:          "identity-001",
		PhoneNumber: "+2348012345678",
		Email:       &storedEmail,
		Status:      authmodels.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	notifier := &fakeNotifier{}
	ec := &fakeEmailClient{}
	svc := newSignupLoginUsecaseWithEmail(idRepo, notifier, &fakePublisher{}, ec)

	_, err := svc.LoginStart(context.Background(), LoginStartInput{
		Identifier: "+2348012345678",
	})
	if err != nil {
		t.Fatalf("LoginStart() error = %v", err)
	}

	yieldForGoroutine()

	if ec.callCount() != 1 {
		t.Fatalf("email client calls = %d, want 1", ec.callCount())
	}
	to, emailCode := ec.lastCall()
	if to != storedEmail {
		t.Errorf("email to = %q, want %q", to, storedEmail)
	}
	// Same code as SMS.
	smsCode := notifier.calls[0].otp
	if emailCode != smsCode {
		t.Errorf("email OTP %q != SMS OTP %q", emailCode, smsCode)
	}
}

// TestLoginStart_NoStoredEmail_EmailClientNotCalled verifies that login with a
// phone-only account (no email) does not attempt email delivery.
func TestLoginStart_NoStoredEmail_EmailClientNotCalled(t *testing.T) {
	idRepo := newFakeIdentityRepository()
	idRepo.identities["+2348012345678"] = authmodels.Identity{
		ID:          "identity-002",
		PhoneNumber: "+2348012345678",
		Email:       nil, // no email on record
		Status:      authmodels.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ec := &fakeEmailClient{}
	svc := newSignupLoginUsecaseWithEmail(idRepo, &fakeNotifier{}, &fakePublisher{}, ec)

	_, err := svc.LoginStart(context.Background(), LoginStartInput{
		Identifier: "+2348012345678",
	})
	if err != nil {
		t.Fatalf("LoginStart() error = %v", err)
	}

	yieldForGoroutine()

	if ec.callCount() != 0 {
		t.Fatalf("email client called %d times, want 0 (no stored email)", ec.callCount())
	}
}

// TestLoginStart_EmailFailure_DoesNotBlock verifies login OTP succeeds even
// when the email client fails.
func TestLoginStart_EmailFailure_DoesNotBlock(t *testing.T) {
	storedEmail := "user@example.com"
	idRepo := newFakeIdentityRepository()
	idRepo.identities["+2348012345678"] = authmodels.Identity{
		ID:          "identity-003",
		PhoneNumber: "+2348012345678",
		Email:       &storedEmail,
		Status:      authmodels.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ec := &fakeEmailClient{err: errors.New("smtp connection refused")}
	svc := newSignupLoginUsecaseWithEmail(idRepo, &fakeNotifier{}, &fakePublisher{}, ec)

	_, err := svc.LoginStart(context.Background(), LoginStartInput{
		Identifier: "+2348012345678",
	})
	if err != nil {
		t.Fatalf("LoginStart() returned error despite email failure: %v", err)
	}
}

// ── Legacy Start: new email resolution tests ──────────────────────────────────

// newStartUsecaseWithIdentities builds a Start usecase that includes an identity
// repository for duplicate-email checks, stored-email lookups, and email persistence.
func newStartUsecaseWithIdentities(
	idRepo *fakeIdentityRepository,
	notifier *fakeNotifier,
	emailClient authclients.EmailClient,
) *AuthUsecase {
	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  newFakeOTPRepository(),
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 10,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	return NewAuthUsecase(Options{
		OTPUsecase:         otpUC,
		Identities:         idRepo,
		Notifier:           notifier,
		EmailClient:        emailClient,
		Publisher:          &fakePublisher{},
		AccessTokenSecret:  []byte("access-secret-32-bytes-long-xxxx"),
		RefreshTokenSecret: []byte("refresh-secret-32-bytes-long-xxx"),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
	})
}

// TestStart_EmailNormalized_LowercasedBeforeDelivery verifies that the email
// address is lower-cased before being passed to the email client.
func TestStart_EmailNormalized_LowercasedBeforeDelivery(t *testing.T) {
	idRepo := newFakeIdentityRepository()
	notifier := &fakeNotifier{}
	ec := &fakeEmailClient{}
	svc := newStartUsecaseWithIdentities(idRepo, notifier, ec)

	_, err := svc.Start(context.Background(), StartInput{
		PhoneNumber: "+2348012345678",
		Email:       "Rider@Example.COM",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	yieldForGoroutine()

	if ec.callCount() != 1 {
		t.Fatalf("email client calls = %d, want 1", ec.callCount())
	}
	to, _ := ec.lastCall()
	if to != "rider@example.com" {
		t.Errorf("email to = %q, want rider@example.com (lowercased)", to)
	}
}

// TestStart_StoredEmail_UsedOnFuturePhoneOnlyStart verifies that when an identity
// already has a stored email, a subsequent phone-only Start delivers to that address.
func TestStart_StoredEmail_UsedOnFuturePhoneOnlyStart(t *testing.T) {
	storedEmail := "stored@example.com"
	idRepo := newFakeIdentityRepository()
	idRepo.identities["+2348012345678"] = authmodels.Identity{
		ID:          "identity-stored",
		PhoneNumber: "+2348012345678",
		Email:       &storedEmail,
		Status:      authmodels.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	notifier := &fakeNotifier{}
	ec := &fakeEmailClient{}
	svc := newStartUsecaseWithIdentities(idRepo, notifier, ec)

	// Phone-only request — no email field.
	_, err := svc.Start(context.Background(), StartInput{
		PhoneNumber: "+2348012345678",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	yieldForGoroutine()

	if ec.callCount() != 1 {
		t.Fatalf("email client calls = %d, want 1 (should use stored email)", ec.callCount())
	}
	to, emailCode := ec.lastCall()
	if to != storedEmail {
		t.Errorf("email to = %q, want %q (stored identity email)", to, storedEmail)
	}
	if emailCode != notifier.calls[0].otp {
		t.Errorf("email code %q != SMS code %q", emailCode, notifier.calls[0].otp)
	}
}

// TestStart_DuplicateEmail_SkippedEmailDelivery verifies that when the provided
// email already belongs to a different phone, email delivery is skipped entirely.
func TestStart_DuplicateEmail_SkippedEmailDelivery(t *testing.T) {
	conflictEmail := "taken@example.com"
	idRepo := newFakeIdentityRepository()
	// Identity for a different phone owns the email.
	idRepo.identities["+2347000000001"] = authmodels.Identity{
		ID:          "identity-other",
		PhoneNumber: "+2347000000001",
		Email:       &conflictEmail,
		Status:      authmodels.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	ec := &fakeEmailClient{}
	svc := newStartUsecaseWithIdentities(idRepo, &fakeNotifier{}, ec)

	// A different phone tries to use the same email.
	_, err := svc.Start(context.Background(), StartInput{
		PhoneNumber: "+2348012345678",
		Email:       conflictEmail,
	})
	if err != nil {
		t.Fatalf("Start() error = %v (duplicate email must not block SMS)", err)
	}

	yieldForGoroutine()

	// Email delivery must be silently skipped — no goroutine launched.
	if ec.callCount() != 0 {
		t.Fatalf("email client calls = %d, want 0 (duplicate email must be skipped)", ec.callCount())
	}
}

// TestVerifyLegacy_AttachesOTPEmailToIdentity verifies that when the legacy Verify
// path upserts an identity that has no email, it attaches the email from the OTP row.
func TestVerifyLegacy_AttachesOTPEmailToIdentity(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	const phone = "+2348012345678"
	const email = "attach@example.com"

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 5,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})

	// Plant an OTP that carries an email (as if Start was called with email).
	_, code, err := otpUC.CreateWithEmail(context.Background(), phone, email)
	if err != nil {
		t.Fatalf("CreateWithEmail: %v", err)
	}

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, pub)
	_, err = svc.Verify(context.Background(), VerifyInput{
		PhoneNumber: phone,
		OTPCode:     code,
	})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	// The upserted identity must now have the OTP email attached.
	identity, ok := idRepo.identities[phone]
	if !ok {
		t.Fatal("identity not found in repo after Verify")
	}
	if identity.Email == nil {
		t.Fatal("identity.Email must not be nil after legacy Verify with OTP email")
	}
	if *identity.Email != email {
		t.Errorf("identity.Email = %q, want %q", *identity.Email, email)
	}
}
