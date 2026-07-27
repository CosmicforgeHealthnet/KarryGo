package integration_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"

	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	authmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
)

// authHarness wires the real authentication service against the test
// database with a fake OTP sender (no live email is ever sent).
type authHarness struct {
	service *authusecases.Service
	sender  *authclients.FakeSender
	pool    *pgxpool.Pool
	events  *authrepos.SecurityEventRepository
}

// stubMemberships reports no organizations by default, so verification
// returns ONBOARDING_REQUIRED.
type stubMemberships struct {
	memberships []authusecases.MembershipSummary
}

func (s *stubMemberships) ListActiveForIdentity(context.Context, string) ([]authusecases.MembershipSummary, error) {
	return s.memberships, nil
}

func (s *stubMemberships) GetForIdentity(_ context.Context, _ string, membershipID string) (authusecases.MembershipSummary, error) {
	for _, membership := range s.memberships {
		if membership.ID == membershipID {
			return membership, nil
		}
	}
	return authusecases.MembershipSummary{}, authusecases.ErrMembershipNotFound
}

func newAuthHarness(t *testing.T, memberships *stubMemberships) *authHarness {
	t.Helper()
	pool := testPool(t)
	sender := authclients.NewFakeSender()
	if memberships == nil {
		memberships = &stubMemberships{}
	}

	service := authusecases.NewService(authusecases.Options{
		Pool:               pool,
		Identities:         authrepos.NewIdentityRepository(pool),
		Challenges:         authrepos.NewChallengeRepository(pool),
		Sessions:           authrepos.NewSessionRepository(pool),
		SecurityEvents:     authrepos.NewSecurityEventRepository(pool),
		Sender:             sender,
		Limiter:            ratelimit.NewMemoryLimiter(),
		Metrics:            observability.NewMetrics(),
		Memberships:        memberships,
		OTPSecret:          []byte("test-otp-pepper"),
		SessionSecret:      []byte("test-session-secret"),
		ContactSecret:      []byte("test-contact-secret"),
		AccessSigner:       auth.NewTokenSigner([]byte("test-access-secret")),
		OTPLength:          6,
		OTPExpiry:          10 * time.Minute,
		OTPMaxAttempts:     5,
		OTPResendCooldown:  60 * time.Second,
		OTPMaxPerHour:      10,
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    72 * time.Hour,
		StepUpTTL:          5 * time.Minute,
		DefaultCountryCode: "234",
	})
	return &authHarness{
		service: service, sender: sender, pool: pool,
		events: authrepos.NewSecurityEventRepository(pool),
	}
}

func (h *authHarness) startAndCode(t *testing.T, email string, phone string) (string, string) {
	t.Helper()
	result, err := h.service.Start(context.Background(), authusecases.StartInput{
		Email: email, Phone: phone, RequestID: "req_test", CorrelationID: "cor_test",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	message, ok := h.sender.LastOTP()
	if !ok {
		t.Fatal("no OTP was sent")
	}
	return result.ChallengeID, message.Code
}

func TestPasswordlessSignupAndLogin(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()

	// New owner signs up with email AND phone; OTP goes to email only.
	challengeID, code := h.startAndCode(t, "Owner@Example.com", "08012345678")
	message, _ := h.sender.LastOTP()
	if message.Email != "owner@example.com" {
		t.Fatalf("OTP destination = %q, must be the normalized email", message.Email)
	}
	if len(code) != 6 {
		t.Fatalf("OTP length = %d, want 6", len(code))
	}

	result, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if result.NextStep != authusecases.NextStepOnboardingRequired {
		t.Fatalf("nextStep = %s, want ONBOARDING_REQUIRED for a new owner", result.NextStep)
	}
	if result.Tokens == nil || result.Tokens.AccessToken == "" || result.Tokens.RefreshToken == "" {
		t.Fatal("verification must issue a session")
	}

	// The identity is now active with contact verified.
	identity, err := h.service.IdentityByID(ctx, result.IdentityID)
	if err != nil {
		t.Fatalf("IdentityByID: %v", err)
	}
	if identity.Status != authmodels.IdentityStatusActive || !identity.ContactVerified() {
		t.Fatalf("identity = %+v", identity)
	}

	// Existing owner logs in with the same pair; no second identity appears.
	secondChallenge, secondCode := h.startAndCode(t, "owner@example.com", "+2348012345678")
	secondResult, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: secondChallenge, Code: secondCode})
	if err != nil {
		t.Fatalf("login Verify() error = %v", err)
	}
	if secondResult.IdentityID != result.IdentityID {
		t.Fatal("the same email and phone must resolve to one identity")
	}
	var identityCount int
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_corporate_identities`).Scan(&identityCount)
	if identityCount != 1 {
		t.Fatalf("identity count = %d, want 1", identityCount)
	}
}

func TestBothEmailAndPhoneAreMandatory(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()

	if _, err := h.service.Start(ctx, authusecases.StartInput{Email: "owner@example.com"}); err == nil {
		t.Fatal("missing phone must be rejected")
	}
	if _, err := h.service.Start(ctx, authusecases.StartInput{Phone: "08012345678"}); err == nil {
		t.Fatal("missing email must be rejected")
	}
	if h.sender.OTPCount() != 0 {
		t.Fatal("no OTP may be sent for an invalid request")
	}
}

func TestOTPIsNeverStoredRawAndNeverReturned(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()

	result, err := h.service.Start(ctx, authusecases.StartInput{
		Email: "raw@example.com", Phone: "08012345679",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	message, _ := h.sender.LastOTP()

	// The start result carries no code field at all; assert the stored digest
	// is not the code and that no column holds it.
	var digest string
	if err := h.pool.QueryRow(ctx,
		`SELECT code_digest FROM fleet_partner_auth_otp_challenges WHERE id = $1`, result.ChallengeID).Scan(&digest); err != nil {
		t.Fatalf("read digest: %v", err)
	}
	if digest == message.Code || strings.Contains(digest, message.Code) {
		t.Fatal("the raw OTP must never be stored")
	}
	// The digest must be the keyed HMAC, verifiable only with the pepper.
	if !auth.VerifyOTP([]byte("test-otp-pepper"), result.ChallengeID, "+2348012345679", message.Code, digest) {
		t.Fatal("stored digest is not the expected keyed HMAC")
	}
	if auth.VerifyOTP([]byte("wrong-pepper"), result.ChallengeID, "+2348012345679", message.Code, digest) {
		t.Fatal("digest must depend on the server pepper")
	}
}

func TestIncorrectOTPAndAttemptLimit(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()
	challengeID, code := h.startAndCode(t, "attempts@example.com", "08012345680")

	for attempt := 1; attempt <= 4; attempt++ {
		_, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: "000000"})
		if apperrors.From(err).Code != errorcodes.OTPInvalid {
			t.Fatalf("attempt %d error = %v, want otp_invalid", attempt, err)
		}
	}
	// The fifth wrong attempt exhausts the limit.
	_, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: "000000"})
	if apperrors.From(err).Code != errorcodes.OTPAttemptLimitExceeded {
		t.Fatalf("fifth attempt error = %v, want otp_attempt_limit_exceeded", err)
	}
	// Even the correct code no longer works.
	_, err = h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code})
	if apperrors.From(err).Code != errorcodes.OTPAttemptLimitExceeded {
		t.Fatalf("after lockout error = %v", err)
	}

	count, err := h.events.CountByType(ctx, authmodels.SecurityEventOTPAttemptExceeded)
	if err != nil || count == 0 {
		t.Fatalf("attempt-limit security event not recorded (count=%d, err=%v)", count, err)
	}
}

func TestExpiredAndConsumedOTP(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()

	// Consumed.
	challengeID, code := h.startAndCode(t, "consumed@example.com", "08012345681")
	if _, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code}); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	_, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code})
	if apperrors.From(err).Code != errorcodes.OTPChallengeConsumed {
		t.Fatalf("replay error = %v, want otp_challenge_consumed", err)
	}

	// Expired.
	expiredID, expiredCode := h.startAndCode(t, "expired@example.com", "08012345682")
	if _, err := h.pool.Exec(ctx,
		`UPDATE fleet_partner_auth_otp_challenges SET expires_at = now() - interval '1 minute' WHERE id = $1`,
		expiredID); err != nil {
		t.Fatalf("expire challenge: %v", err)
	}
	_, err = h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: expiredID, Code: expiredCode})
	if apperrors.From(err).Code != errorcodes.OTPChallengeExpired {
		t.Fatalf("expired error = %v, want otp_challenge_expired", err)
	}
}

func TestResendCooldownAndInvalidation(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()
	challengeID, firstCode := h.startAndCode(t, "resend@example.com", "08012345683")

	// Cooldown blocks an immediate resend.
	_, err := h.service.Resend(ctx, authusecases.ResendInput{ChallengeID: challengeID})
	if apperrors.From(err).Code != errorcodes.OTPResendTooSoon {
		t.Fatalf("immediate resend error = %v, want otp_resend_too_soon", err)
	}

	// Make the cooldown elapse and resend.
	if _, err := h.pool.Exec(ctx,
		`UPDATE fleet_partner_auth_otp_challenges SET resend_available_at = now() - interval '1 second' WHERE id = $1`,
		challengeID); err != nil {
		t.Fatalf("clear cooldown: %v", err)
	}
	if _, err := h.service.Resend(ctx, authusecases.ResendInput{ChallengeID: challengeID}); err != nil {
		t.Fatalf("Resend: %v", err)
	}
	message, _ := h.sender.LastOTP()
	if message.Code == firstCode {
		t.Fatal("resend must issue a new code")
	}
	// The previous code is now invalid.
	if _, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: firstCode}); err == nil {
		t.Fatal("the superseded code must not verify")
	}
	// The new code works.
	if _, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: message.Code}); err != nil {
		t.Fatalf("new code must verify: %v", err)
	}
}

func TestConcurrentVerificationSingleWinner(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()
	challengeID, code := h.startAndCode(t, "race@example.com", "08012345684")

	var wg sync.WaitGroup
	results := make([]error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			_, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code})
			results[slot] = err
		}(i)
	}
	wg.Wait()

	winners := 0
	for _, err := range results {
		if err == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent verifications produced %d winners, want exactly 1", winners)
	}
	var sessionCount int
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_corporate_sessions`).Scan(&sessionCount)
	if sessionCount != 1 {
		t.Fatalf("session count = %d, want 1", sessionCount)
	}
}

func TestIdentityPairConflict(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()

	// Two separate identities.
	firstChallenge, firstCode := h.startAndCode(t, "first@example.com", "08011111111")
	if _, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: firstChallenge, Code: firstCode}); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	secondChallenge, secondCode := h.startAndCode(t, "second@example.com", "08022222222")
	if _, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: secondChallenge, Code: secondCode}); err != nil {
		t.Fatalf("second verify: %v", err)
	}

	// Mixing the halves is a conflict. Start still returns generically.
	result, err := h.service.Start(ctx, authusecases.StartInput{
		Email: "first@example.com", Phone: "08022222222",
	})
	if err != nil {
		t.Fatalf("Start on conflict must stay generic, got %v", err)
	}
	if result.ChallengeID == "" || result.MaskedDestination == "" {
		t.Fatal("generic response must still look like a normal start")
	}
	count, _ := h.events.CountByType(ctx, authmodels.SecurityEventIdentityPairConflict)
	if count == 0 {
		t.Fatal("identity pair conflict must be recorded as a security event")
	}
}

func TestStartIsEnumerationSafe(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()

	// Unknown account.
	unknown, err := h.service.Start(ctx, authusecases.StartInput{
		Email: "nobody@example.com", Phone: "08033333333",
	})
	if err != nil {
		t.Fatalf("Start unknown: %v", err)
	}

	// Known account.
	knownChallenge, knownCode := h.startAndCode(t, "known@example.com", "08044444444")
	if _, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: knownChallenge, Code: knownCode}); err != nil {
		t.Fatalf("verify known: %v", err)
	}
	known, err := h.service.Start(ctx, authusecases.StartInput{
		Email: "known@example.com", Phone: "08044444444",
	})
	if err != nil {
		t.Fatalf("Start known: %v", err)
	}

	// Both responses have the same shape and fields.
	if unknown.DeliveryChannel != known.DeliveryChannel ||
		unknown.ExpiresInSeconds != known.ExpiresInSeconds ||
		unknown.ResendAfterSeconds != known.ResendAfterSeconds {
		t.Fatal("start responses must be indistinguishable between known and unknown accounts")
	}
	if unknown.ChallengeID == "" || known.ChallengeID == "" {
		t.Fatal("both responses must carry a challenge id")
	}
}

func TestOTPDeliveryFailureDoesNotReportSuccess(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()
	h.sender.FailWith = authclients.ErrDeliveryUnavailable

	_, err := h.service.Start(ctx, authusecases.StartInput{
		Email: "down@example.com", Phone: "08055555555",
	})
	if apperrors.From(err).Code != errorcodes.OTPDeliveryUnavailable {
		t.Fatalf("delivery failure error = %v, want otp_delivery_unavailable", err)
	}
	if apperrors.From(err).Status != 503 {
		t.Fatalf("status = %d, want 503", apperrors.From(err).Status)
	}

	// The challenge records the failure rather than claiming delivery.
	var deliveryStatus string
	if err := h.pool.QueryRow(ctx,
		`SELECT delivery_status FROM fleet_partner_auth_otp_challenges ORDER BY created_at DESC LIMIT 1`).
		Scan(&deliveryStatus); err != nil {
		t.Fatalf("read delivery status: %v", err)
	}
	if deliveryStatus != authmodels.DeliveryStatusUnavailable {
		t.Fatalf("delivery_status = %s, want UNAVAILABLE", deliveryStatus)
	}
}

func TestRefreshRotatesAndDetectsReuse(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()
	challengeID, code := h.startAndCode(t, "refresh@example.com", "08066666666")
	result, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	original := result.Tokens.RefreshToken

	rotated, err := h.service.Refresh(ctx, authusecases.RefreshInput{RefreshToken: original})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if rotated.RefreshToken == original {
		t.Fatal("refresh must rotate the token")
	}

	// Presenting the old token is reuse: the family is revoked.
	_, err = h.service.Refresh(ctx, authusecases.RefreshInput{RefreshToken: original})
	if apperrors.From(err).Code != errorcodes.RefreshTokenReused {
		t.Fatalf("reuse error = %v, want refresh_token_reused", err)
	}
	// The rotated token is now dead too.
	if _, err := h.service.Refresh(ctx, authusecases.RefreshInput{RefreshToken: rotated.RefreshToken}); err == nil {
		t.Fatal("the whole token family must be revoked after reuse")
	}
	count, _ := h.events.CountByType(ctx, authmodels.SecurityEventRefreshReuse)
	if count == 0 {
		t.Fatal("refresh reuse must be recorded as a security event")
	}
}

func TestLogoutIsIdempotent(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()
	challengeID, code := h.startAndCode(t, "logout@example.com", "08077777777")
	result, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	for attempt := 0; attempt < 3; attempt++ {
		if err := h.service.Logout(ctx, result.Tokens.SessionID, result.IdentityID, "req", "cor"); err != nil {
			t.Fatalf("logout attempt %d: %v", attempt, err)
		}
	}
	// A revoked session cannot refresh.
	if _, err := h.service.Refresh(ctx, authusecases.RefreshInput{RefreshToken: result.Tokens.RefreshToken}); err == nil {
		t.Fatal("a logged-out session must not refresh")
	}
}

func TestSuspendedIdentityCannotAuthenticate(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()
	challengeID, code := h.startAndCode(t, "suspended@example.com", "08088888888")
	result, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if _, err := h.pool.Exec(ctx,
		`UPDATE fleet_partner_corporate_identities SET status = 'SUSPENDED' WHERE id = $1`,
		result.IdentityID); err != nil {
		t.Fatalf("suspend identity: %v", err)
	}

	// Refresh is refused.
	_, err = h.service.Refresh(ctx, authusecases.RefreshInput{RefreshToken: result.Tokens.RefreshToken})
	if apperrors.From(err).Code != errorcodes.IdentitySuspended {
		t.Fatalf("suspended refresh error = %v, want identity_suspended", err)
	}
	// Start stays generic and sends no OTP.
	before := h.sender.OTPCount()
	if _, err := h.service.Start(ctx, authusecases.StartInput{
		Email: "suspended@example.com", Phone: "08088888888",
	}); err != nil {
		t.Fatalf("start for suspended identity must stay generic: %v", err)
	}
	if h.sender.OTPCount() != before {
		t.Fatal("no OTP may be sent to a suspended identity")
	}
}

func TestMultipleMembershipsRequireSelection(t *testing.T) {
	memberships := &stubMemberships{memberships: []authusecases.MembershipSummary{
		{ID: "ousr_a", OrganizationID: "org_a", OrganizationStatus: "ACTIVE", Status: "ACTIVE", RoleVersion: 1},
		{ID: "ousr_b", OrganizationID: "org_b", OrganizationStatus: "ACTIVE", Status: "ACTIVE", RoleVersion: 1},
	}}
	h := newAuthHarness(t, memberships)
	ctx := context.Background()

	challengeID, code := h.startAndCode(t, "multi@example.com", "08099999999")
	result, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.NextStep != authusecases.NextStepOrganizationSelection {
		t.Fatalf("nextStep = %s, want ORGANIZATION_SELECTION_REQUIRED", result.NextStep)
	}
	if len(result.Organizations) != 2 {
		t.Fatalf("organizations = %d, want 2", len(result.Organizations))
	}

	// Selecting a membership of this identity works.
	tokens, err := h.service.SelectOrganization(ctx, authusecases.SelectOrganizationInput{
		IdentityID: result.IdentityID, SessionID: result.Tokens.SessionID, MembershipID: "ousr_a",
	})
	if err != nil {
		t.Fatalf("SelectOrganization: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Fatal("organization selection must issue a scoped session")
	}

	// Selecting a membership that is not this identity's is refused.
	_, err = h.service.SelectOrganization(ctx, authusecases.SelectOrganizationInput{
		IdentityID: result.IdentityID, SessionID: result.Tokens.SessionID, MembershipID: "ousr_someone_else",
	})
	if err == nil || apperrors.From(err).Status != 404 {
		t.Fatalf("selecting another identity's membership must fail, got %v", err)
	}
}

func TestSingleMembershipIssuesScopedSession(t *testing.T) {
	memberships := &stubMemberships{memberships: []authusecases.MembershipSummary{
		{ID: "ousr_only", OrganizationID: "org_only", OrganizationStatus: "ACTIVE", Status: "ACTIVE", RoleVersion: 3},
	}}
	h := newAuthHarness(t, memberships)
	ctx := context.Background()

	challengeID, code := h.startAndCode(t, "single@example.com", "08010101010")
	result, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.NextStep != authusecases.NextStepReady {
		t.Fatalf("nextStep = %s, want READY", result.NextStep)
	}

	var organizationID string
	var roleVersion int64
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(organization_id,''), role_version FROM fleet_partner_corporate_sessions WHERE id = $1`,
		result.Tokens.SessionID).Scan(&organizationID, &roleVersion); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if organizationID != "org_only" || roleVersion != 3 {
		t.Fatalf("session scope = %s / v%d", organizationID, roleVersion)
	}
}

func TestSuspendedOrganizationCannotRefresh(t *testing.T) {
	memberships := &stubMemberships{memberships: []authusecases.MembershipSummary{
		{ID: "ousr_susp", OrganizationID: "org_susp", OrganizationStatus: "ACTIVE", Status: "ACTIVE", RoleVersion: 1},
	}}
	h := newAuthHarness(t, memberships)
	ctx := context.Background()

	challengeID, code := h.startAndCode(t, "orgsusp@example.com", "08012121212")
	result, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	// The organization is suspended between refreshes.
	memberships.memberships[0].OrganizationStatus = "SUSPENDED"
	_, err = h.service.Refresh(ctx, authusecases.RefreshInput{RefreshToken: result.Tokens.RefreshToken})
	if apperrors.From(err).Code != errorcodes.SessionRevoked {
		t.Fatalf("suspended organization refresh error = %v, want session_revoked", err)
	}
}

func TestStepUpIsSessionAndActionBound(t *testing.T) {
	h := newAuthHarness(t, nil)
	ctx := context.Background()

	challengeID, code := h.startAndCode(t, "stepup@example.com", "08013131313")
	login, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: challengeID, Code: code})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	stepUp, err := h.service.Start(ctx, authusecases.StartInput{
		Email: "stepup@example.com", Phone: "08013131313",
		Purpose: authmodels.PurposeStepUp, IdentityID: login.IdentityID,
		SessionID: login.Tokens.SessionID, OrganizationID: "org_x", ActionClass: "organization.close",
	})
	if err != nil {
		t.Fatalf("step-up start: %v", err)
	}
	message, _ := h.sender.LastOTP()
	granted, err := h.service.Verify(ctx, authusecases.VerifyInput{ChallengeID: stepUp.ChallengeID, Code: message.Code})
	if err != nil {
		t.Fatalf("step-up verify: %v", err)
	}
	if granted.NextStep != authusecases.NextStepStepUpGranted || granted.StepUpReference == "" {
		t.Fatalf("step-up result = %+v", granted)
	}

	// The grant is valid for exactly this session, organization and action.
	if err := h.service.VerifyStepUpGrant(ctx, granted.StepUpReference, login.IdentityID,
		login.Tokens.SessionID, "org_x", "organization.close"); err != nil {
		t.Fatalf("valid grant rejected: %v", err)
	}
	for name, check := range map[string][4]string{
		"other session":      {login.IdentityID, "ses_other", "org_x", "organization.close"},
		"other organization": {login.IdentityID, login.Tokens.SessionID, "org_y", "organization.close"},
		"other action":       {login.IdentityID, login.Tokens.SessionID, "org_x", "staff.suspend"},
		"other identity":     {"cid_other", login.Tokens.SessionID, "org_x", "organization.close"},
	} {
		if err := h.service.VerifyStepUpGrant(ctx, granted.StepUpReference, check[0], check[1], check[2], check[3]); err == nil {
			t.Fatalf("grant must not transfer to %s", name)
		}
	}
}

func TestRateLimitBlocksRepeatedStarts(t *testing.T) {
	pool := testPool(t)
	sender := authclients.NewFakeSender()
	service := authusecases.NewService(authusecases.Options{
		Pool:               pool,
		Identities:         authrepos.NewIdentityRepository(pool),
		Challenges:         authrepos.NewChallengeRepository(pool),
		Sessions:           authrepos.NewSessionRepository(pool),
		SecurityEvents:     authrepos.NewSecurityEventRepository(pool),
		Sender:             sender,
		Limiter:            ratelimit.NewMemoryLimiter(),
		Metrics:            observability.NewMetrics(),
		Memberships:        &stubMemberships{},
		OTPSecret:          []byte("pepper"),
		SessionSecret:      []byte("session"),
		ContactSecret:      []byte("contact"),
		AccessSigner:       auth.NewTokenSigner([]byte("access")),
		OTPMaxPerHour:      3, // deliberately low for the test
		AccessTokenTTL:     time.Minute,
		RefreshTokenTTL:    time.Hour,
		DefaultCountryCode: "234",
	})

	ctx := context.Background()
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		_, lastErr = service.Start(ctx, authusecases.StartInput{
			Email: "flood@example.com", Phone: "08014141414",
		})
	}
	if apperrors.From(lastErr).Code != errorcodes.AuthenticationRateLimited {
		t.Fatalf("final error = %v, want authentication_rate_limited", lastErr)
	}
}

func TestChallengeExpirySweep(t *testing.T) {
	pool := testPool(t)
	challenges := authrepos.NewChallengeRepository(pool)
	h := newAuthHarnessWith(t, pool)

	challengeID, _ := h.startAndCode(t, "sweep@example.com", "08015151515")
	if _, err := pool.Exec(context.Background(),
		`UPDATE fleet_partner_auth_otp_challenges SET expires_at = now() - interval '1 hour' WHERE id = $1`,
		challengeID); err != nil {
		t.Fatalf("age challenge: %v", err)
	}

	expired, err := challenges.ExpireDue(context.Background(), 100)
	if err != nil || expired == 0 {
		t.Fatalf("ExpireDue = %d, %v", expired, err)
	}
	var status string
	_ = pool.QueryRow(context.Background(),
		`SELECT status FROM fleet_partner_auth_otp_challenges WHERE id = $1`, challengeID).Scan(&status)
	if status != authmodels.ChallengeStatusExpired {
		t.Fatalf("status = %s, want EXPIRED", status)
	}
}

// newAuthHarnessWith reuses an already-prepared pool (the caller has run
// testPool, so truncation already happened).
func newAuthHarnessWith(t *testing.T, pool *pgxpool.Pool) *authHarness {
	t.Helper()
	sender := authclients.NewFakeSender()
	service := authusecases.NewService(authusecases.Options{
		Pool:               pool,
		Identities:         authrepos.NewIdentityRepository(pool),
		Challenges:         authrepos.NewChallengeRepository(pool),
		Sessions:           authrepos.NewSessionRepository(pool),
		SecurityEvents:     authrepos.NewSecurityEventRepository(pool),
		Sender:             sender,
		Limiter:            ratelimit.NewMemoryLimiter(),
		Metrics:            observability.NewMetrics(),
		Memberships:        &stubMemberships{},
		OTPSecret:          []byte("test-otp-pepper"),
		SessionSecret:      []byte("test-session-secret"),
		ContactSecret:      []byte("test-contact-secret"),
		AccessSigner:       auth.NewTokenSigner([]byte("test-access-secret")),
		OTPExpiry:          10 * time.Minute,
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    72 * time.Hour,
		DefaultCountryCode: "234",
	})
	return &authHarness{service: service, sender: sender, pool: pool,
		events: authrepos.NewSecurityEventRepository(pool)}
}

var _ = errors.Is

// verifyInputFor builds a minimal verification input for HTTP tests.
func verifyInputFor(challengeID string, code string) authusecases.VerifyInput {
	return authusecases.VerifyInput{ChallengeID: challengeID, Code: code}
}
