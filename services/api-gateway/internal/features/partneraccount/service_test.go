package partneraccount

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

const testJWTSecret = "test-secret-at-least-32-characters-long!!"

type harness struct {
	svc      *Service
	accounts *fakeAccountRepo
	sessions *fakeSessionRepo
	keys     *fakeKeyStore
	otps     *fakeOTPRepo
	email    *fakeOTPSender
}

func newHarness() *harness {
	h := &harness{accounts: newFakeAccountRepo(), sessions: newFakeSessionRepo(), keys: newFakeKeyStore(), otps: newFakeOTPRepo(), email: &fakeOTPSender{}}
	h.svc = NewService(Options{
		Accounts: h.accounts, Sessions: h.sessions, Keys: h.keys, OTPs: h.otps, EmailSender: h.email,
		OTPGenerator: func() (string, error) { return "042391", nil },
		JWTSecret:    testJWTSecret, JWTTTL: time.Hour,
	})
	return h
}

func wantErr(t *testing.T, err error, code string, status int) {
	t.Helper()
	var e *delivery.APIError
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want *delivery.APIError", err)
	}
	if e.Code != code || e.Status != status {
		t.Fatalf("err = %s/%d, want %s/%d", e.Code, e.Status, code, status)
	}
}

func registerInput() RegisterInput {
	return RegisterInput{BusinessName: "PharmaCare", BusinessEmail: "Dev@Pharmacare.com", Password: "supersecret"}
}

func (h *harness) mustRegister(t *testing.T) *RegisterResult {
	t.Helper()
	res, err := h.svc.Register(context.Background(), registerInput(), RequestMeta{})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	return res
}

// --- register ----------------------------------------------------------------

func TestRegisterSuccess(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)

	if res.Account.BusinessEmail != "dev@pharmacare.com" {
		t.Fatalf("email not normalized: %q", res.Account.BusinessEmail)
	}
	if res.Account.Status != StatusActive || res.Account.Plan != PlanSandbox {
		t.Fatalf("account = %+v, want active/sandbox", res.Account)
	}
	// password stored as bcrypt hash, not raw
	storedPtr := h.accounts.passwordHash(res.Account.ID)
	if storedPtr == nil {
		t.Fatal("password hash is nil")
	}
	stored := *storedPtr
	if stored == "" || stored == "supersecret" {
		t.Fatal("password not stored as a hash")
	}
	if bcrypt.CompareHashAndPassword([]byte(stored), []byte("supersecret")) != nil {
		t.Fatal("stored password hash does not verify")
	}
	// first sandbox key returned once
	if !strings.HasPrefix(res.APIKey.Key, "sk_test_") {
		t.Fatalf("first key = %q, want sk_test_ prefix", res.APIKey.Key)
	}
	if len(h.keys.keys) != 1 || h.keys.keys[0].env != EnvironmentSandbox {
		t.Fatalf("expected one sandbox key stored, got %+v", h.keys.keys)
	}
	// stored key is a hash, not the raw key
	if h.keys.keys[0].keyHash == res.APIKey.Key {
		t.Fatal("raw API key stored instead of hash")
	}
	if bcrypt.CompareHashAndPassword([]byte(h.keys.keys[0].keyHash), []byte(res.APIKey.Key)) != nil {
		t.Fatal("stored key hash does not verify against issued key")
	}
	// session token issued and stored (hash only)
	if res.Token == "" {
		t.Fatal("no session token issued")
	}
	if _, ok := h.sessions.byHash[hashToken(res.Token)]; !ok {
		t.Fatal("session token hash not stored")
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	h := newHarness()
	h.mustRegister(t)
	_, err := h.svc.Register(context.Background(), registerInput(), RequestMeta{})
	wantErr(t, err, "email_taken", 409)
}

func TestRegisterShortPassword(t *testing.T) {
	h := newHarness()
	in := registerInput()
	in.Password = "short"
	_, err := h.svc.Register(context.Background(), in, RequestMeta{})
	wantErr(t, err, "validation_error", 400)
}

// --- login -------------------------------------------------------------------

func TestLoginSuccess(t *testing.T) {
	h := newHarness()
	h.mustRegister(t)
	res, err := h.svc.Login(context.Background(), LoginInput{Email: "dev@pharmacare.com", Password: "supersecret"}, RequestMeta{})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if res.Token == "" {
		t.Fatal("no token issued")
	}
	if _, ok := h.sessions.byHash[hashToken(res.Token)]; !ok {
		t.Fatal("login session not stored")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	h := newHarness()
	h.mustRegister(t)
	_, err := h.svc.Login(context.Background(), LoginInput{Email: "dev@pharmacare.com", Password: "wrong"}, RequestMeta{})
	wantErr(t, err, "invalid_credentials", 401)
}

func TestLoginUnknownEmailIsGeneric(t *testing.T) {
	h := newHarness()
	_, err := h.svc.Login(context.Background(), LoginInput{Email: "nobody@x.com", Password: "whatever"}, RequestMeta{})
	wantErr(t, err, "invalid_credentials", 401)
}

func TestLoginInactiveAccount(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	h.accounts.byID[res.Account.ID].Status = StatusSuspended
	_, err := h.svc.Login(context.Background(), LoginInput{Email: "dev@pharmacare.com", Password: "supersecret"}, RequestMeta{})
	wantErr(t, err, "account_inactive", 403)
}

// --- sessions / logout -------------------------------------------------------

func TestAuthenticateSessionValid(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	pid, _, acc, err := h.svc.AuthenticateSession(context.Background(), res.Token)
	if err != nil {
		t.Fatalf("AuthenticateSession() error = %v", err)
	}
	if pid != res.Account.ID || acc.ID != res.Account.ID {
		t.Fatalf("wrong partner: %s vs %s", pid, res.Account.ID)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	if err := h.svc.Logout(context.Background(), hashToken(res.Token)); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	_, _, _, err := h.svc.AuthenticateSession(context.Background(), res.Token)
	wantErr(t, err, "session_revoked", 401)
}

func TestAuthenticateSessionExpired(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	// Craft a genuinely-expired JWT (exp in the past) for the same partner+secret,
	// and store a matching session so the failure is on the JWT expiry check.
	token, exp, err := signSessionJWT(testJWTSecret, res.Account.ID, -time.Hour, time.Now())
	if err != nil {
		t.Fatalf("signSessionJWT() error = %v", err)
	}
	_ = h.sessions.CreateSession(context.Background(), CreateSessionParams{
		PartnerID: res.Account.ID, TokenHash: hashToken(token), ExpiresAt: exp,
	})
	_, _, _, err = h.svc.AuthenticateSession(context.Background(), token)
	wantErr(t, err, "invalid_token", 401)
}

func TestAuthenticateSessionExpiredInDB(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	// Valid JWT, but the stored session is expired in the DB.
	past := time.Now().Add(-time.Minute)
	h.sessions.byHash[hashToken(res.Token)].ExpiresAt = past
	_, _, _, err := h.svc.AuthenticateSession(context.Background(), res.Token)
	wantErr(t, err, "invalid_token", 401)
}

func TestAuthenticateSessionBadSignature(t *testing.T) {
	h := newHarness()
	other := NewService(Options{Accounts: h.accounts, Sessions: h.sessions, Keys: h.keys, JWTSecret: "a-completely-different-secret-value-xxx", JWTTTL: time.Hour})
	res := h.mustRegister(t)
	// Token signed by h.svc; validated by `other` with a different secret.
	_, _, _, err := other.AuthenticateSession(context.Background(), res.Token)
	wantErr(t, err, "invalid_token", 401)
}

// --- keys --------------------------------------------------------------------

func TestCreateSandboxKey(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	issued, err := h.svc.CreateKey(context.Background(), res.Account.ID, PlanSandbox, CreateKeyInput{Environment: "sandbox", Label: "Prod"})
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}
	if !strings.HasPrefix(issued.Key, "sk_test_") {
		t.Fatalf("key = %q, want sk_test_", issued.Key)
	}
}

func TestSandboxPartnerCannotCreateLiveKey(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	_, err := h.svc.CreateKey(context.Background(), res.Account.ID, PlanSandbox, CreateKeyInput{Environment: "live"})
	wantErr(t, err, "sandbox_only", 403)
}

func TestLivePartnerCanCreateLiveKey(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	issued, err := h.svc.CreateKey(context.Background(), res.Account.ID, PlanLive, CreateKeyInput{Environment: "live"})
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}
	if !strings.HasPrefix(issued.Key, "sk_live_") {
		t.Fatalf("key = %q, want sk_live_", issued.Key)
	}
}

func TestListKeysHasNoSecret(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	views, err := h.svc.ListKeys(context.Background(), res.Account.ID)
	if err != nil {
		t.Fatalf("ListKeys() error = %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("len(views) = %d, want 1", len(views))
	}
	if strings.Contains(views[0].Prefix, res.APIKey.Key) || views[0].Prefix == res.APIKey.Key {
		t.Fatal("list must not contain the raw key")
	}
	// KeyView has no key or key_hash field (compile-time guarantee); prefix only.
}

func TestRevokeKeyOwnershipEnforced(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	keyID := h.keys.keys[0].id

	// Another partner cannot revoke it.
	if err := h.svc.RevokeKey(context.Background(), "other-partner", keyID); err == nil {
		t.Fatal("expected not_found revoking another partner's key")
	} else {
		wantErr(t, err, "not_found", 404)
	}
	if h.keys.keys[0].status == "revoked" {
		t.Fatal("key revoked by non-owner")
	}
	// Owner can revoke.
	if err := h.svc.RevokeKey(context.Background(), res.Account.ID, keyID); err != nil {
		t.Fatalf("RevokeKey() error = %v", err)
	}
	if h.keys.keys[0].status != "revoked" {
		t.Fatal("key not revoked by owner")
	}
}

// --- internal ----------------------------------------------------------------

func TestActivateLiveThenLiveKey(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	if err := h.svc.ActivateLive(context.Background(), res.Account.ID); err != nil {
		t.Fatalf("ActivateLive() error = %v", err)
	}
	if h.accounts.byID[res.Account.ID].Plan != PlanLive {
		t.Fatal("plan not upgraded to live")
	}
	// The partner (now live) can create a live key.
	if _, err := h.svc.CreateKey(context.Background(), res.Account.ID, PlanLive, CreateKeyInput{Environment: "live"}); err != nil {
		t.Fatalf("live key creation after activate-live failed: %v", err)
	}
}

func TestActivateLiveNotFound(t *testing.T) {
	h := newHarness()
	err := h.svc.ActivateLive(context.Background(), "11111111-1111-1111-1111-111111111111")
	wantErr(t, err, "partner_not_found", 404)
}

func TestUpdateStatusValidation(t *testing.T) {
	h := newHarness()
	res := h.mustRegister(t)
	if err := h.svc.UpdateStatus(context.Background(), res.Account.ID, "suspended"); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}
	if h.accounts.byID[res.Account.ID].Status != StatusSuspended {
		t.Fatal("status not updated")
	}
	if err := h.svc.UpdateStatus(context.Background(), res.Account.ID, "bogus"); err == nil {
		t.Fatal("expected validation error for bad status")
	} else {
		wantErr(t, err, "validation_error", 400)
	}
}

// --- OTP helpers -------------------------------------------------------------

func TestGenerateOTPCodeFormat(t *testing.T) {
	for i := 0; i < 50; i++ {
		code, err := generateOTPCode()
		if err != nil {
			t.Fatalf("generateOTPCode() error = %v", err)
		}
		if !validOTPCode(code) {
			t.Fatalf("code %q is not a six-digit OTP", code)
		}
	}
}

func TestHashOTPCodeIsSHA256Hex(t *testing.T) {
	got := hashOTPCode("042391")
	want := "01a24f804a95b680412a1d1beaf88ca7e20802322d576f184d4f52900e22fb25"
	if got != want {
		t.Fatalf("hashOTPCode() = %q, want %q", got, want)
	}
	if got == "042391" {
		t.Fatal("hash must not be plaintext OTP")
	}
}

// --- OTP signup --------------------------------------------------------------

func TestStartSignupCreatesHashedChallengeAndSendsEmail(t *testing.T) {
	h := newHarness()
	if err := h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: " Dev@PharmaCare.com "}); err != nil {
		t.Fatalf("StartSignup() error = %v", err)
	}
	challenge := latestChallenge(t, h, "dev@pharmacare.com")
	if challenge.OTPCodeHash != hashOTPCode("042391") {
		t.Fatalf("stored hash = %q", challenge.OTPCodeHash)
	}
	if challenge.OTPCodeHash == "042391" {
		t.Fatal("stored plaintext OTP")
	}
	if len(h.email.sent) != 1 || h.email.sent[0].Email != "dev@pharmacare.com" || h.email.sent[0].Code != "042391" {
		t.Fatalf("email sender calls = %+v", h.email.sent)
	}
}

func TestStartSignupExistingAccountReturnsEmailTaken(t *testing.T) {
	h := newHarness()
	h.mustRegister(t)
	err := h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"})
	wantErr(t, err, "email_taken", 409)
}

func TestStartSignupEmailFailureDeletesChallenge(t *testing.T) {
	h := newHarness()
	h.email.fail = ErrPartnerEmailDeliveryFailed
	err := h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"})
	wantErr(t, err, "email_delivery_failed", 502)
	if latest, _ := h.otps.FindLatestOTPChallenge(context.Background(), "dev@pharmacare.com"); latest != nil {
		t.Fatalf("challenge was not deleted after email failure: %+v", latest)
	}
}

func TestStartSignupMissingEmailSenderReturnsSafeError(t *testing.T) {
	h := newHarness()
	h.svc.emailSender = nil
	err := h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"})
	wantErr(t, err, "email_not_configured", 500)
	if latest, _ := h.otps.FindLatestOTPChallenge(context.Background(), "dev@pharmacare.com"); latest != nil {
		t.Fatalf("challenge should not be created when email sender is missing: %+v", latest)
	}
}

func TestStartSignupEmailNotConfiguredDeletesChallenge(t *testing.T) {
	h := newHarness()
	h.email.fail = ErrPartnerEmailNotConfigured
	err := h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"})
	wantErr(t, err, "email_not_configured", 500)
	if latest, _ := h.otps.FindLatestOTPChallenge(context.Background(), "dev@pharmacare.com"); latest != nil {
		t.Fatalf("challenge was not deleted after email config failure: %+v", latest)
	}
}

func TestVerifySignupCreatesActiveSandboxAccountAndSession(t *testing.T) {
	h := newHarness()
	if err := h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"}); err != nil {
		t.Fatalf("StartSignup() error = %v", err)
	}
	res, err := h.svc.VerifySignup(context.Background(), SignupVerifyInput{BusinessEmail: "dev@pharmacare.com", OTPCode: "042391"}, RequestMeta{})
	if err != nil {
		t.Fatalf("VerifySignup() error = %v", err)
	}
	if res.Token == "" || res.Account.BusinessEmail != "dev@pharmacare.com" {
		t.Fatalf("bad verify result: %+v", res)
	}
	if res.Account.Status != StatusActive || res.Account.Plan != PlanSandbox {
		t.Fatalf("account = %+v, want active/sandbox", res.Account)
	}
	if res.Account.BusinessName != nil {
		t.Fatalf("business_name = %q, want nil", *res.Account.BusinessName)
	}
	if h.accounts.passwordHash(res.Account.ID) != nil {
		t.Fatal("OTP-created account should have nil password_hash")
	}
	if len(h.keys.keys) != 0 {
		t.Fatalf("OTP verify should not auto-generate API keys, got %+v", h.keys.keys)
	}
	if _, ok := h.sessions.byHash[hashToken(res.Token)]; !ok {
		t.Fatal("session token hash not stored")
	}
	if _, _, _, err := h.svc.AuthenticateSession(context.Background(), res.Token); err != nil {
		t.Fatalf("issued session does not authenticate: %v", err)
	}
}

func TestVerifySignupWrongOTPIncrementsAttempts(t *testing.T) {
	h := newHarness()
	_ = h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"})
	_, err := h.svc.VerifySignup(context.Background(), SignupVerifyInput{BusinessEmail: "dev@pharmacare.com", OTPCode: "000000"}, RequestMeta{})
	api := wantAPIError(t, err, "otp_invalid", 400)
	if api.Meta["attempts_remaining"] != 4 {
		t.Fatalf("attempts_remaining = %#v, want 4", api.Meta["attempts_remaining"])
	}
	if latestChallenge(t, h, "dev@pharmacare.com").Attempts != 1 {
		t.Fatal("attempt count was not incremented")
	}
}

func TestVerifySignupMaxAttempts(t *testing.T) {
	h := newHarness()
	_ = h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"})
	latestChallenge(t, h, "dev@pharmacare.com").Attempts = 5
	_, err := h.svc.VerifySignup(context.Background(), SignupVerifyInput{BusinessEmail: "dev@pharmacare.com", OTPCode: "042391"}, RequestMeta{})
	wantErr(t, err, "otp_max_attempts", 429)
}

func TestVerifySignupExpiredAndConsumedCodesFail(t *testing.T) {
	h := newHarness()
	_ = h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"})
	latestChallenge(t, h, "dev@pharmacare.com").ExpiresAt = time.Now().Add(-time.Minute)
	_, err := h.svc.VerifySignup(context.Background(), SignupVerifyInput{BusinessEmail: "dev@pharmacare.com", OTPCode: "042391"}, RequestMeta{})
	wantErr(t, err, "otp_expired", 400)

	h = newHarness()
	_ = h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"})
	if _, err := h.svc.VerifySignup(context.Background(), SignupVerifyInput{BusinessEmail: "dev@pharmacare.com", OTPCode: "042391"}, RequestMeta{}); err != nil {
		t.Fatalf("first verify error = %v", err)
	}
	_, err = h.svc.VerifySignup(context.Background(), SignupVerifyInput{BusinessEmail: "dev@pharmacare.com", OTPCode: "042391"}, RequestMeta{})
	wantErr(t, err, "otp_expired", 400)
}

func TestResendSignupRateLimitAndLatestCodeWins(t *testing.T) {
	h := newHarness()
	_ = h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"})
	err := h.svc.ResendSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"})
	api := wantAPIError(t, err, "resend_too_soon", 429)
	if api.Meta["seconds_remaining"] == nil {
		t.Fatal("seconds_remaining missing")
	}

	latestChallenge(t, h, "dev@pharmacare.com").CreatedAt = time.Now().Add(-2 * time.Minute)
	h.svc.otpGenerator = func() (string, error) { return "111111", nil }
	if err := h.svc.ResendSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"}); err != nil {
		t.Fatalf("ResendSignup() error = %v", err)
	}
	if len(h.email.sent) != 2 || h.email.sent[1].Code != "111111" {
		t.Fatalf("email calls = %+v", h.email.sent)
	}
	_, err = h.svc.VerifySignup(context.Background(), SignupVerifyInput{BusinessEmail: "dev@pharmacare.com", OTPCode: "042391"}, RequestMeta{})
	wantErr(t, err, "otp_invalid", 400)
	if _, err := h.svc.VerifySignup(context.Background(), SignupVerifyInput{BusinessEmail: "dev@pharmacare.com", OTPCode: "111111"}, RequestMeta{}); err != nil {
		t.Fatalf("latest code verify error = %v", err)
	}
}

func TestLoginOTPOnlyAccountFailsWithInvalidCredentials(t *testing.T) {
	h := newHarness()
	_ = h.svc.StartSignup(context.Background(), SignupEmailInput{BusinessEmail: "dev@pharmacare.com"})
	if _, err := h.svc.VerifySignup(context.Background(), SignupVerifyInput{BusinessEmail: "dev@pharmacare.com", OTPCode: "042391"}, RequestMeta{}); err != nil {
		t.Fatalf("VerifySignup() error = %v", err)
	}
	_, err := h.svc.Login(context.Background(), LoginInput{Email: "dev@pharmacare.com", Password: "supersecret"}, RequestMeta{})
	wantErr(t, err, "invalid_credentials", 401)
}

func latestChallenge(t *testing.T, h *harness, email string) *OTPChallenge {
	t.Helper()
	h.otps.mu.Lock()
	defer h.otps.mu.Unlock()
	ids := h.otps.byEmail[email]
	if len(ids) == 0 {
		t.Fatalf("no challenge for %s", email)
	}
	return h.otps.byID[ids[len(ids)-1]]
}

func wantAPIError(t *testing.T, err error, code string, status int) *delivery.APIError {
	t.Helper()
	var e *delivery.APIError
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want *delivery.APIError", err)
	}
	if e.Code != code || e.Status != status {
		t.Fatalf("err = %s/%d, want %s/%d", e.Code, e.Status, code, status)
	}
	return e
}
