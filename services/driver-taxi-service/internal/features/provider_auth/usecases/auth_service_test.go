package providerauthusecases

import (
	"context"
	"strings"
	"testing"
	"time"

	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ─── in-memory fakes ──────────────────────────────────────────────────────────

type fakeProviders struct {
	byID    map[string]providerauthmodels.Provider
	byPhone map[string]string // phone -> id
	nextID  int
}

func newFakeProviders() *fakeProviders {
	return &fakeProviders{byID: map[string]providerauthmodels.Provider{}, byPhone: map[string]string{}}
}

func (f *fakeProviders) UpsertByPhone(_ context.Context, phone string, _ providerauthmodels.Signup) (providerauthmodels.Provider, error) {
	if id, ok := f.byPhone[phone]; ok {
		return f.byID[id], nil
	}
	f.nextID++
	id := string(rune('a'+f.nextID)) + "-provider"
	p := providerauthmodels.Provider{
		ID:               id,
		Phone:            phone,
		OnboardingStatus: providerauthmodels.OnboardingProfileNeeded,
		AccountStatus:    providerauthmodels.AccountStatusActive,
		Rating:           5,
	}
	f.byID[id] = p
	f.byPhone[phone] = id
	return p, nil
}

// UpsertByEmail shares the phone index, keyed "email:<address>".
func (f *fakeProviders) UpsertByEmail(_ context.Context, email string, _ providerauthmodels.Signup) (providerauthmodels.Provider, error) {
	key := "email:" + email
	if id, ok := f.byPhone[key]; ok {
		return f.byID[id], nil
	}
	f.nextID++
	id := string(rune('a'+f.nextID)) + "-provider"
	p := providerauthmodels.Provider{
		ID:               id,
		Email:            email,
		OnboardingStatus: providerauthmodels.OnboardingProfileNeeded,
		AccountStatus:    providerauthmodels.AccountStatusActive,
		Rating:           5,
	}
	f.byID[id] = p
	f.byPhone[key] = id
	return p, nil
}

func (f *fakeProviders) GetByID(_ context.Context, id string) (providerauthmodels.Provider, error) {
	p, ok := f.byID[id]
	if !ok {
		return providerauthmodels.Provider{}, apperrors.NotFound("Driver could not be found.", nil)
	}
	return p, nil
}

func (f *fakeProviders) UpdatePhone(_ context.Context, id, phone string) (providerauthmodels.Provider, error) {
	p, ok := f.byID[id]
	if !ok {
		return providerauthmodels.Provider{}, apperrors.NotFound("Driver could not be found.", nil)
	}
	delete(f.byPhone, p.Phone)
	p.Phone = phone
	f.byID[id] = p
	f.byPhone[phone] = id
	return p, nil
}

func (f *fakeProviders) ContactTaken(context.Context, string, string, string) (bool, bool, error) {
	return false, false, nil
}

type fakeSessions struct {
	byID map[string]providerauthmodels.RefreshSession
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{byID: map[string]providerauthmodels.RefreshSession{}}
}

func (f *fakeSessions) Create(_ context.Context, s providerauthmodels.RefreshSession) error {
	f.byID[s.ID] = s
	return nil
}

func (f *fakeSessions) GetByID(_ context.Context, id string) (providerauthmodels.RefreshSession, error) {
	s, ok := f.byID[id]
	if !ok {
		return providerauthmodels.RefreshSession{}, apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}
	return s, nil
}

func (f *fakeSessions) Revoke(_ context.Context, id string) error {
	s, ok := f.byID[id]
	if !ok {
		return nil
	}
	now := time.Now()
	s.RevokedAt = &now
	f.byID[id] = s
	return nil
}

type fakeChallenges struct {
	byKey    map[string]providerauthmodels.OTPChallenge
	saveErr  error
	saveCall int
}

func newFakeChallenges() *fakeChallenges {
	return &fakeChallenges{byKey: map[string]providerauthmodels.OTPChallenge{}}
}

func (f *fakeChallenges) Save(_ context.Context, c providerauthmodels.OTPChallenge, _, _ time.Duration, _ int) error {
	f.saveCall++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.byKey[c.IdentifierKey()] = c
	return nil
}

func (f *fakeChallenges) Get(_ context.Context, key string) (providerauthmodels.OTPChallenge, bool, error) {
	c, ok := f.byKey[key]
	return c, ok, nil
}

func (f *fakeChallenges) RecordFailedAttempt(_ context.Context, c providerauthmodels.OTPChallenge, _ time.Duration) error {
	c.Attempts++
	f.byKey[c.IdentifierKey()] = c
	return nil
}

func (f *fakeChallenges) Delete(_ context.Context, key string) error {
	delete(f.byKey, key)
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newTestService(t *testing.T, otpDebug bool) (*AuthService, *fakeProviders, *fakeSessions, *fakeChallenges) {
	t.Helper()
	providers, sessions, challenges := newFakeProviders(), newFakeSessions(), newFakeChallenges()
	svc := NewAuthService(Options{
		Providers:          providers,
		Sessions:           sessions,
		Challenges:         challenges,
		AccessTokenSecret:  []byte("access-secret"),
		RefreshTokenSecret: []byte("refresh-secret"),
		OTPSecret:          []byte("otp-secret"),
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    24 * time.Hour,
		OTPTTL:             10 * time.Minute,
		OTPRateWindow:      time.Minute,
		OTPMaxRequests:     5,
		OTPMaxAttempts:     3,
		OTPDebug:           otpDebug,
	})
	return svc, providers, sessions, challenges
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestStartAuthNormalizesPhoneToE164(t *testing.T) {
	// The challenge is keyed by the normalized number, so a driver who types
	// 0801... and later +234801... must land on the same challenge rather than
	// silently minting a second one.
	svc, _, _, challenges := newTestService(t, true)

	if _, err := svc.StartAuth(context.Background(), StartAuthInput{Phone: "08011112222"}); err != nil {
		t.Fatalf("StartAuth: %v", err)
	}

	if _, ok := challenges.byKey["phone:+2348011112222"]; !ok {
		t.Fatalf("challenge should be keyed by the normalized phone, got keys %v", keysOf(challenges))
	}
}

func TestStartAuthRequiresPhone(t *testing.T) {
	svc, _, _, _ := newTestService(t, true)
	_, err := svc.StartAuth(context.Background(), StartAuthInput{})
	if err == nil {
		t.Fatal("expected a validation error when no phone is supplied")
	}
}

func TestDebugOTPOnlyPopulatedUnderFlag(t *testing.T) {
	// The result field is json:"-" so it can never serialize itself, but it must
	// also stay empty when the flag is off, so the handler has nothing to copy.
	on, _, _, _ := newTestService(t, true)
	res, err := on.StartAuth(context.Background(), StartAuthInput{Phone: "08011112222"})
	if err != nil {
		t.Fatalf("StartAuth: %v", err)
	}
	if len(res.DebugOTP) != 6 {
		t.Fatalf("debug build should expose a 6-digit OTP, got %q", res.DebugOTP)
	}

	off, _, _, _ := newTestService(t, false)
	res, err = off.StartAuth(context.Background(), StartAuthInput{Phone: "08011112222"})
	if err != nil {
		t.Fatalf("StartAuth: %v", err)
	}
	if res.DebugOTP != "" {
		t.Fatalf("OTP must never be returned when debug is off, got %q", res.DebugOTP)
	}
}

func TestVerifyAuthIssuesTokensAndCreatesDriver(t *testing.T) {
	svc, providers, _, _ := newTestService(t, true)
	ctx := context.Background()

	start, err := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	if err != nil {
		t.Fatalf("StartAuth: %v", err)
	}

	tokens, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	})
	if err != nil {
		t.Fatalf("VerifyAuth: %v", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("both tokens should be issued")
	}
	// First login auto-creates the driver, still needing profile setup.
	if tokens.Provider.OnboardingStatus != providerauthmodels.OnboardingProfileNeeded {
		t.Fatalf("new driver should require profile setup, got %q", tokens.Provider.OnboardingStatus)
	}
	if len(providers.byID) != 1 {
		t.Fatalf("expected exactly one driver created, got %d", len(providers.byID))
	}

	// The refresh token is prefixed by its session id so the session can be
	// resolved without storing the token.
	if !strings.HasPrefix(tokens.RefreshToken, strings.Split(tokens.RefreshToken, ".")[0]+".") {
		t.Fatal("refresh token should be sessionID.secret")
	}
}

func TestVerifyAuthTokenClaimsCarryTaxiRoleAndService(t *testing.T) {
	// payment-wallet-service and support-dispute-service both key on these two
	// values for taxi tokens. A rename here silently breaks driver earnings,
	// withdrawals and complaints.
	svc, _, _, _ := newTestService(t, true)
	ctx := context.Background()

	start, _ := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	tokens, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	})
	if err != nil {
		t.Fatalf("VerifyAuth: %v", err)
	}

	claims, err := svc.AccessSigner().Verify(tokens.AccessToken)
	if err != nil {
		t.Fatalf("token should verify with its own signer: %v", err)
	}
	if claims.Role != "taxi_provider" {
		t.Fatalf("role must be taxi_provider, got %q", claims.Role)
	}
	if claims.Service != "taxi" {
		t.Fatalf("service must be taxi, got %q", claims.Service)
	}
	if claims.SessionID == "" {
		t.Fatal("claims must carry the session id")
	}
}

func TestVerifyAuthWrongCodeBurnsAnAttempt(t *testing.T) {
	svc, _, _, challenges := newTestService(t, true)
	ctx := context.Background()

	start, _ := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	if _, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: "000000", ChallengeID: start.ChallengeID,
	}); err == nil {
		t.Fatal("a wrong code should be rejected")
	}

	if got := challenges.byKey["phone:+2348011112222"].Attempts; got != 1 {
		t.Fatalf("a wrong code should consume one attempt, got %d", got)
	}
	// The correct code still works while attempts remain.
	if _, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	}); err != nil {
		t.Fatalf("correct code should still verify after one failure: %v", err)
	}
}

func TestVerifyAuthRejectsAfterMaxAttempts(t *testing.T) {
	svc, _, _, challenges := newTestService(t, true)
	ctx := context.Background()

	start, _ := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	for i := 0; i < 3; i++ {
		_, _ = svc.VerifyAuth(ctx, VerifyAuthInput{
			Phone: "08011112222", OTP: "000000", ChallengeID: start.ChallengeID,
		})
	}

	// Even the correct code is refused once the cap is reached; the driver has
	// to request a fresh challenge.
	_, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	})
	if err == nil {
		t.Fatal("expected the attempt cap to refuse even a correct code")
	}
	if !strings.Contains(err.Error(), "Too many incorrect attempts") {
		t.Fatalf("error should tell the driver to request a new code, got %q", err)
	}
	_ = challenges
}

func TestVerifyAuthRejectsExpiredChallenge(t *testing.T) {
	svc, _, _, challenges := newTestService(t, true)
	ctx := context.Background()

	start, _ := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	expired := challenges.byKey["phone:+2348011112222"]
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	challenges.byKey["phone:+2348011112222"] = expired

	if _, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	}); err == nil {
		t.Fatal("an expired challenge should be rejected")
	}
}

func TestVerifyAuthRejectsMismatchedChallengeID(t *testing.T) {
	// Binding the code to its challenge id stops a code minted for one flow
	// being replayed into another.
	svc, _, _, _ := newTestService(t, true)
	ctx := context.Background()

	start, _ := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	if _, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: "some-other-challenge",
	}); err == nil {
		t.Fatal("a mismatched challenge id should be rejected")
	}
}

func TestVerifyAuthConsumesChallengeOnce(t *testing.T) {
	svc, _, _, challenges := newTestService(t, true)
	ctx := context.Background()

	start, _ := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	if _, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	}); err != nil {
		t.Fatalf("VerifyAuth: %v", err)
	}
	if len(challenges.byKey) != 0 {
		t.Fatal("a consumed challenge must be deleted so the code cannot be replayed")
	}
	if _, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	}); err == nil {
		t.Fatal("replaying a consumed code should fail")
	}
}

func TestRefreshRotatesAndRevokesPreviousSession(t *testing.T) {
	svc, _, _, _ := newTestService(t, true)
	ctx := context.Background()

	start, _ := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	first, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	})
	if err != nil {
		t.Fatalf("VerifyAuth: %v", err)
	}

	second, err := svc.Refresh(ctx, RefreshInput{RefreshToken: first.RefreshToken})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh must rotate the token")
	}

	// The old token is now dead: a leaked refresh token is usable at most once.
	if _, err := svc.Refresh(ctx, RefreshInput{RefreshToken: first.RefreshToken}); err == nil {
		t.Fatal("the rotated-away refresh token must be rejected")
	}
}

func TestRefreshRejectsForgedToken(t *testing.T) {
	svc, _, _, _ := newTestService(t, true)
	ctx := context.Background()

	start, _ := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	tokens, _ := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	})

	sessionID := strings.Split(tokens.RefreshToken, ".")[0]
	forged := sessionID + ".not-the-real-secret"
	if _, err := svc.Refresh(ctx, RefreshInput{RefreshToken: forged}); err == nil {
		t.Fatal("a token with a valid session id but wrong secret must be rejected")
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	svc, _, _, _ := newTestService(t, true)
	ctx := context.Background()

	start, _ := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	tokens, _ := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	})

	if err := svc.Logout(ctx, tokens.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := svc.Refresh(ctx, RefreshInput{RefreshToken: tokens.RefreshToken}); err == nil {
		t.Fatal("a logged-out session must not refresh")
	}
}

func TestChangePhoneVerifyUpdatesTheNumber(t *testing.T) {
	svc, providers, _, _ := newTestService(t, true)
	ctx := context.Background()

	start, _ := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	tokens, _ := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	})
	driverID := tokens.Provider.ID

	// The code goes to the NEW number; the driver is identified by their token.
	change, err := svc.ChangePhoneStart(ctx, driverID, "08099998888")
	if err != nil {
		t.Fatalf("ChangePhoneStart: %v", err)
	}
	updated, err := svc.ChangePhoneVerify(ctx, driverID, "08099998888", change.DebugOTP, change.ChallengeID)
	if err != nil {
		t.Fatalf("ChangePhoneVerify: %v", err)
	}
	if updated.Phone != "+2348099998888" {
		t.Fatalf("phone should be updated and normalized, got %q", updated.Phone)
	}
	if providers.byID[driverID].Phone != "+2348099998888" {
		t.Fatal("the change should be persisted")
	}
}

func TestChangePhoneVerifyRejectsWrongCode(t *testing.T) {
	svc, _, _, _ := newTestService(t, true)
	ctx := context.Background()

	start, _ := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	tokens, _ := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	})

	change, _ := svc.ChangePhoneStart(ctx, tokens.Provider.ID, "08099998888")
	if _, err := svc.ChangePhoneVerify(ctx, tokens.Provider.ID, "08099998888", "000000", change.ChallengeID); err == nil {
		t.Fatal("a wrong code must not change the phone number")
	}
}

func keysOf(c *fakeChallenges) []string {
	out := make([]string, 0, len(c.byKey))
	for k := range c.byKey {
		out = append(out, k)
	}
	return out
}
