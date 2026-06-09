package authusecases

import (
	"context"
	"testing"
	"time"

	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	"karrygo/shared/go/apperrors"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func newSignupLoginUsecase(idRepo *fakeIdentityRepository, notifier *fakeNotifier, pub *fakePublisher) *AuthUsecase {
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
		Publisher:          pub,
		AccessTokenSecret:  []byte("access-secret-32-bytes-long-xxxx"),
		RefreshTokenSecret: []byte("refresh-secret-32-bytes-long-xxx"),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
		OTPDebug:           true,
	})
}

// seedIdentity plants a pre-existing identity by phone+email in the fake repo.
func seedIdentity(repo *fakeIdentityRepository, phone, email string) {
	var emailPtr *string
	if email != "" {
		emailPtr = &email
	}
	repo.identities[phone] = authmodels.Identity{
		ID:          "existing-" + phone,
		PhoneNumber: phone,
		Email:       emailPtr,
		Status:      authmodels.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// ── SignupStart tests ─────────────────────────────────────────────────────────

func TestSignupStart_Success(t *testing.T) {
	idRepo := newFakeIdentityRepository()
	notifier := &fakeNotifier{}
	pub := &fakePublisher{}

	svc := newSignupLoginUsecase(idRepo, notifier, pub)
	result, err := svc.SignupStart(context.Background(), SignupStartInput{
		PhoneNumber:   "+2348012345678",
		Email:         "newuser@example.com",
		CorrelationID: "corr-signup-001",
	})
	if err != nil {
		t.Fatalf("SignupStart() error = %v", err)
	}
	if result.ExpiresInSeconds != 600 {
		t.Errorf("ExpiresInSeconds = %d, want 600", result.ExpiresInSeconds)
	}
	// OTP event published with purpose=signup
	if len(pub.otpEvents) != 1 {
		t.Fatalf("published events = %d, want 1", len(pub.otpEvents))
	}
	if pub.otpEvents[0].Purpose != "signup" {
		t.Errorf("event.Purpose = %q, want signup", pub.otpEvents[0].Purpose)
	}
	// Notifier called (phone OTP delivery)
	if len(notifier.calls) != 1 {
		t.Fatalf("notifier calls = %d, want 1", len(notifier.calls))
	}
}

func TestSignupStart_DuplicatePhone_ReturnsConflict(t *testing.T) {
	idRepo := newFakeIdentityRepository()
	seedIdentity(idRepo, "+2348012345678", "existing@example.com")

	svc := newSignupLoginUsecase(idRepo, &fakeNotifier{}, &fakePublisher{})
	_, err := svc.SignupStart(context.Background(), SignupStartInput{
		PhoneNumber: "+2348012345678",
		Email:       "newuser@example.com",
	})
	if err == nil {
		t.Fatal("expected conflict error for duplicate phone, got nil")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok || appErr.Code != apperrors.CodeConflict {
		t.Errorf("code = %v, want conflict", err)
	}
}

func TestSignupStart_DuplicateEmail_ReturnsConflict(t *testing.T) {
	idRepo := newFakeIdentityRepository()
	seedIdentity(idRepo, "+2347012345678", "taken@example.com")

	svc := newSignupLoginUsecase(idRepo, &fakeNotifier{}, &fakePublisher{})
	// Different phone, same email
	_, err := svc.SignupStart(context.Background(), SignupStartInput{
		PhoneNumber: "+2348012345678",
		Email:       "taken@example.com",
	})
	if err == nil {
		t.Fatal("expected conflict error for duplicate email, got nil")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok || appErr.Code != apperrors.CodeConflict {
		t.Errorf("code = %v, want conflict", err)
	}
}

func TestSignupStart_InvalidPhone(t *testing.T) {
	svc := newSignupLoginUsecase(newFakeIdentityRepository(), &fakeNotifier{}, &fakePublisher{})
	_, err := svc.SignupStart(context.Background(), SignupStartInput{
		PhoneNumber: "08012345678",
		Email:       "user@example.com",
	})
	if err == nil {
		t.Fatal("expected validation error for non-E.164 phone")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok || appErr.Code != apperrors.CodeValidationFailed {
		t.Errorf("code = %v, want validation_failed", err)
	}
}

func TestSignupStart_InvalidEmail(t *testing.T) {
	svc := newSignupLoginUsecase(newFakeIdentityRepository(), &fakeNotifier{}, &fakePublisher{})
	_, err := svc.SignupStart(context.Background(), SignupStartInput{
		PhoneNumber: "+2348012345678",
		Email:       "not-an-email",
	})
	if err == nil {
		t.Fatal("expected validation error for bad email")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok || appErr.Code != apperrors.CodeValidationFailed {
		t.Errorf("code = %v, want validation_failed", err)
	}
}

func TestSignupStart_OTPNotExposedInResult(t *testing.T) {
	svc := newSignupLoginUsecase(newFakeIdentityRepository(), &fakeNotifier{}, &fakePublisher{})
	result, err := svc.SignupStart(context.Background(), SignupStartInput{
		PhoneNumber: "+2348012345678",
		Email:       "user@example.com",
	})
	if err != nil {
		t.Fatalf("SignupStart() error = %v", err)
	}
	// StartResult struct only has ExpiresInSeconds — no OTP field possible.
	if result.ExpiresInSeconds == 0 {
		t.Error("ExpiresInSeconds must not be zero")
	}
}

// ── LoginStart tests ──────────────────────────────────────────────────────────

func TestLoginStart_SuccessByPhone(t *testing.T) {
	idRepo := newFakeIdentityRepository()
	seedIdentity(idRepo, "+2348012345678", "user@example.com")
	notifier := &fakeNotifier{}
	pub := &fakePublisher{}

	svc := newSignupLoginUsecase(idRepo, notifier, pub)
	result, err := svc.LoginStart(context.Background(), LoginStartInput{
		Identifier:    "+2348012345678",
		CorrelationID: "corr-login-001",
	})
	if err != nil {
		t.Fatalf("LoginStart() error = %v", err)
	}
	if result.ExpiresInSeconds == 0 {
		t.Error("ExpiresInSeconds must not be zero")
	}
	if len(pub.otpEvents) != 1 {
		t.Fatalf("published events = %d, want 1", len(pub.otpEvents))
	}
	if pub.otpEvents[0].Purpose != "login" {
		t.Errorf("event.Purpose = %q, want login", pub.otpEvents[0].Purpose)
	}
}

func TestLoginStart_SuccessByEmail(t *testing.T) {
	idRepo := newFakeIdentityRepository()
	seedIdentity(idRepo, "+2348012345678", "user@example.com")

	svc := newSignupLoginUsecase(idRepo, &fakeNotifier{}, &fakePublisher{})
	_, err := svc.LoginStart(context.Background(), LoginStartInput{
		Identifier: "user@example.com",
	})
	if err != nil {
		t.Fatalf("LoginStart() by email error = %v", err)
	}
}

func TestLoginStart_AccountNotFound_ByPhone(t *testing.T) {
	svc := newSignupLoginUsecase(newFakeIdentityRepository(), &fakeNotifier{}, &fakePublisher{})
	_, err := svc.LoginStart(context.Background(), LoginStartInput{
		Identifier: "+2348099999999",
	})
	if err == nil {
		t.Fatal("expected not_found error for unknown phone, got nil")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok || appErr.Code != apperrors.CodeNotFound {
		t.Errorf("code = %v, want not_found", err)
	}
}

func TestLoginStart_AccountNotFound_ByEmail(t *testing.T) {
	svc := newSignupLoginUsecase(newFakeIdentityRepository(), &fakeNotifier{}, &fakePublisher{})
	_, err := svc.LoginStart(context.Background(), LoginStartInput{
		Identifier: "nobody@example.com",
	})
	if err == nil {
		t.Fatal("expected not_found error for unknown email, got nil")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok || appErr.Code != apperrors.CodeNotFound {
		t.Errorf("code = %v, want not_found", err)
	}
}

func TestLoginStart_EmptyIdentifier(t *testing.T) {
	svc := newSignupLoginUsecase(newFakeIdentityRepository(), &fakeNotifier{}, &fakePublisher{})
	_, err := svc.LoginStart(context.Background(), LoginStartInput{Identifier: ""})
	if err == nil {
		t.Fatal("expected validation error for empty identifier")
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok || appErr.Code != apperrors.CodeValidationFailed {
		t.Errorf("code = %v, want validation_failed", err)
	}
}

func TestLoginStart_NoOTPSentForUnknownPhone(t *testing.T) {
	notifier := &fakeNotifier{}
	pub := &fakePublisher{}
	svc := newSignupLoginUsecase(newFakeIdentityRepository(), notifier, pub)

	_, _ = svc.LoginStart(context.Background(), LoginStartInput{Identifier: "+2348099999999"})

	if len(notifier.calls) != 0 {
		t.Errorf("notifier calls = %d, want 0 (no OTP for unknown account)", len(notifier.calls))
	}
	if len(pub.otpEvents) != 0 {
		t.Errorf("published events = %d, want 0 (no OTP for unknown account)", len(pub.otpEvents))
	}
}

// ── Verify: signup path creates account/session ───────────────────────────────

func TestVerifySignup_CreatesNewAccount(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 10,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})

	// Create a signup OTP (with email stored)
	phone := "+2348012345678"
	email := "newuser@example.com"
	emailPtr := email
	_, code, err := otpUC.CreateForSignup(context.Background(), phone, emailPtr)
	if err != nil {
		t.Fatalf("CreateForSignup: %v", err)
	}

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, pub)
	result, err := svc.Verify(context.Background(), VerifyInput{
		PhoneNumber:   phone,
		Identifier:    phone,
		OTPCode:       code,
		Purpose:       "signup",
		CorrelationID: "corr-signup-verify-001",
	})
	if err != nil {
		t.Fatalf("Verify(signup) error = %v", err)
	}
	if result.ProviderID == "" {
		t.Error("ProviderID must not be empty after signup")
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		t.Error("tokens must be non-empty after signup")
	}

	// Identity created with phone AND email
	created, exists := idRepo.identities[phone]
	if !exists {
		t.Fatal("identity was not created in repository")
	}
	if created.Email == nil || *created.Email != email {
		t.Errorf("identity.Email = %v, want %q", created.Email, email)
	}

	// Session created
	if len(sessRepo.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessRepo.sessions))
	}
}

func TestVerifyLogin_DoesNotCreateAccount(t *testing.T) {
	otpRepo := newFakeOTPRepository()
	idRepo := newFakeIdentityRepository()
	sessRepo := newFakeSessionRepository()
	pub := &capturePublisher{}

	// Pre-seed an existing identity
	phone := "+2348012345678"
	email := "existing@example.com"
	emailPtr := email
	idRepo.identities[phone] = authmodels.Identity{
		ID:          "existing-id",
		PhoneNumber: phone,
		Email:       &emailPtr,
		Status:      authmodels.StatusActive,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	otpUC := NewOTPUsecase(OTPOptions{
		Repository:  otpRepo,
		Secret:      []byte("test-otp-secret"),
		TTL:         10 * time.Minute,
		RateWindow:  10 * time.Minute,
		MaxRequests: 10,
		MaxAttempts: 3,
		LockoutTTL:  30 * time.Minute,
	})
	code, err := seedOTPForVerify(otpUC, otpRepo, phone)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := newVerifyUsecase(otpRepo, idRepo, sessRepo, pub)
	result, err := svc.Verify(context.Background(), VerifyInput{
		Identifier:  phone,
		OTPCode:     code,
		Purpose:     "login",
	})
	if err != nil {
		t.Fatalf("Verify(login) error = %v", err)
	}

	// Same provider ID as the pre-seeded identity
	if result.ProviderID != "existing-id" {
		t.Errorf("ProviderID = %q, want existing-id (must not create a new account)", result.ProviderID)
	}
	// Still only 1 identity in repo (no new one created)
	if len(idRepo.identities) != 1 {
		t.Errorf("identities count = %d, want 1 (login must not create new account)", len(idRepo.identities))
	}
}
