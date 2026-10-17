// Package vendorauthusecases implements Vendor Service's AUTHENTICATION
// business rules — OTP start/resend/verify, refresh, logout, step-up. Never
// imports internal/features/staff/{http,repositories} directly (the
// cross-feature import guardrail forbids it); staff/session-status lookups go
// through the StaffResolver port, with the real adapter wired in
// cmd/main.go.
package vendorauthusecases

import (
	"context"
	"crypto/hmac"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/phonenumber"

	vendorauthmodels "cosmicforge/logistics/services/vendor-service/internal/features/auth/models"
	vendorauthrepositories "cosmicforge/logistics/services/vendor-service/internal/features/auth/repositories"
)

const (
	AccessRole    = "vendor_staff"
	AccessService = "vendor"
)

// StaffResolver is the narrow port into the staff feature's data that
// authentication needs: which vendor(s) a login identity has active staff
// access to, and a staff member's current status (for refresh/step-up, which
// must re-check status on every use, not just at login).
type StaffResolver interface {
	ListActiveByUser(ctx context.Context, userID string) ([]StaffAffiliation, error)
	GetByVendorAndUser(ctx context.Context, vendorID, userID string) (StaffAffiliation, error)
	GetByID(ctx context.Context, staffID string) (StaffAffiliation, error)
}

// StaffAffiliation is the minimal staff-status view authentication needs.
// UserID lets AuthService look up the associated login identity's own
// phone/email (via its own same-feature UserRepository) for step-up OTP
// delivery — step-up always sends to the ALREADY-KNOWN identity's registered
// contact, never to a caller-supplied one.
type StaffAffiliation struct {
	StaffID  string
	VendorID string
	UserID   string
	Active   bool
}

// NotificationSender is the narrow port for OTP delivery via
// notification-service (see internal/features/auth/clients).
type NotificationSender interface {
	SendOTP(ctx context.Context, identifierType, identifierValue, purpose, otp string) error
}

type RegistrationInput struct {
	LegalName string
	Currency  string
	Timezone  string
}

type OwnerRegistration interface {
	RegisterOwner(context.Context, string, RegistrationInput) error
}

type AuthService struct {
	users        *vendorauthrepositories.UserRepository
	challenges   *vendorauthrepositories.OTPChallengeRepository
	sessions     *vendorauthrepositories.SessionRepository
	stepUps      *vendorauthrepositories.StepUpRepository
	staff        StaffResolver
	registration OwnerRegistration
	notifier     NotificationSender

	accessSigner  *sharedauth.TokenSigner
	otpSecret     []byte
	refreshSecret []byte

	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	otpTTL          time.Duration
	otpMaxAttempts  int
	otpMaxResends   int
	stepUpTTL       time.Duration
	otpDebug        bool
	now             func() time.Time
}

type Options struct {
	Users        *vendorauthrepositories.UserRepository
	Challenges   *vendorauthrepositories.OTPChallengeRepository
	Sessions     *vendorauthrepositories.SessionRepository
	StepUps      *vendorauthrepositories.StepUpRepository
	Staff        StaffResolver
	Registration OwnerRegistration
	Notifier     NotificationSender

	AccessTokenSecret  []byte
	RefreshTokenSecret []byte
	OTPSecret          []byte

	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	OTPTTL          time.Duration
	OTPMaxAttempts  int
	OTPMaxResends   int
	StepUpTTL       time.Duration
	OTPDebug        bool
}

func NewAuthService(opts Options) *AuthService {
	return &AuthService{
		users: opts.Users, challenges: opts.Challenges, sessions: opts.Sessions, stepUps: opts.StepUps,
		staff: opts.Staff, notifier: opts.Notifier, registration: opts.Registration,
		accessSigner:  sharedauth.NewTokenSigner(opts.AccessTokenSecret),
		otpSecret:     opts.OTPSecret,
		refreshSecret: opts.RefreshTokenSecret,

		accessTokenTTL: opts.AccessTokenTTL, refreshTokenTTL: opts.RefreshTokenTTL,
		otpTTL: opts.OTPTTL, otpMaxAttempts: opts.OTPMaxAttempts, otpMaxResends: opts.OTPMaxResends,
		stepUpTTL: opts.StepUpTTL, otpDebug: opts.OTPDebug, now: time.Now,
	}
}

func (s *AuthService) AccessSigner() *sharedauth.TokenSigner { return s.accessSigner }

// ─── Start ──────────────────────────────────────────────────────────────

type StartInput struct {
	Phone string
	Email string
}

// StartResult never includes the OTP itself outside DebugOTP, which is only
// ever populated when otpDebug is true — a non-production-only shortcut, per
// Section 5's own rule.
type StartResult struct {
	ChallengeID string
	ExpiresIn   int64
	DebugOTP    string
}

func (s *AuthService) Start(ctx context.Context, in StartInput) (StartResult, error) {
	identifierType, identifierValue, identifierKey, err := normalizeIdentifier(in.Phone, in.Email)
	if err != nil {
		return StartResult{}, err
	}

	otp, err := sharedauth.GenerateNumericOTP(sharedauth.DefaultOTPLength)
	if err != nil {
		return StartResult{}, apperrors.Internal("Verification code could not be generated.", err)
	}

	challengeID := "voc_" + uuid.NewString()
	challenge := vendorauthmodels.OTPChallenge{
		ID: challengeID, Purpose: vendorauthmodels.PurposeLogin,
		IdentifierType: identifierType, IdentifierValue: identifierValue,
		Generation: 1, ExpiresAt: s.now().Add(s.otpTTL),
	}
	challenge.OTPHash = challenge.HashFor(s.otpSecret, otp, 1)

	// A resend of an already-active challenge for this identifier replaces it
	// in place rather than creating a second concurrently-valid row — start
	// is idempotent-by-identifier this way, matching "a resend must not
	// create multiple simultaneously valid credentials" even when the caller
	// hits Start twice instead of the dedicated resend endpoint.
	if existing, ok, err := s.challenges.GetActiveByIdentifier(ctx, identifierKey, vendorauthmodels.PurposeLogin); err == nil && ok {
		if err := s.challenges.Resend(ctx, existing.ID, existing.HashFor(s.otpSecret, otp, existing.Generation+1), s.now().Add(s.otpTTL)); err != nil {
			return StartResult{}, err
		}
		result := StartResult{ChallengeID: existing.ID, ExpiresIn: int64(s.otpTTL.Seconds())}
		if s.otpDebug {
			result.DebugOTP = otp
		}
		return result, s.deliverOTP(ctx, identifierType, identifierValue, vendorauthmodels.PurposeLogin, otp)
	}

	if err := s.challenges.Create(ctx, challenge); err != nil {
		return StartResult{}, err
	}
	result := StartResult{ChallengeID: challengeID, ExpiresIn: int64(s.otpTTL.Seconds())}
	if s.otpDebug {
		result.DebugOTP = otp
	}
	return result, s.deliverOTP(ctx, identifierType, identifierValue, vendorauthmodels.PurposeLogin, otp)
}

// ─── Resend ─────────────────────────────────────────────────────────────

type ResendInput struct {
	Phone       string
	Email       string
	ChallengeID string
}

func (s *AuthService) Resend(ctx context.Context, in ResendInput) (StartResult, error) {
	identifierType, identifierValue, identifierKey, err := normalizeIdentifier(in.Phone, in.Email)
	if err != nil {
		return StartResult{}, err
	}
	existing, ok, err := s.challenges.GetActiveByIdentifier(ctx, identifierKey, vendorauthmodels.PurposeLogin)
	if err != nil {
		return StartResult{}, err
	}
	if !ok || (in.ChallengeID != "" && in.ChallengeID != existing.ID) {
		return StartResult{}, apperrors.NotFound("No active verification code to resend. Start again.", nil)
	}
	if existing.ResendCount >= s.otpMaxResends {
		return StartResult{}, apperrors.RateLimited("You've reached the resend limit. Please start again.", nil)
	}

	otp, err := sharedauth.GenerateNumericOTP(sharedauth.DefaultOTPLength)
	if err != nil {
		return StartResult{}, apperrors.Internal("Verification code could not be generated.", err)
	}
	newHash := existing.HashFor(s.otpSecret, otp, existing.Generation+1)
	if err := s.challenges.Resend(ctx, existing.ID, newHash, s.now().Add(s.otpTTL)); err != nil {
		return StartResult{}, err
	}

	result := StartResult{ChallengeID: existing.ID, ExpiresIn: int64(s.otpTTL.Seconds())}
	if s.otpDebug {
		result.DebugOTP = otp
	}
	return result, s.deliverOTP(ctx, identifierType, identifierValue, vendorauthmodels.PurposeLogin, otp)
}

// ─── Verify ─────────────────────────────────────────────────────────────

type VerifyInput struct {
	Phone       string
	Email       string
	OTP         string
	ChallengeID string
	// VendorID disambiguates which vendor business to act as when the login
	// identity has active staff access at more than one. Optional when there
	// is exactly one.
	VendorID  string
	DeviceID  *string
	UserAgent string
	IPAddress string
}

type TokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	StaffID      string
	VendorID     string
}

// VerifyResult is either a completed login (TokenResult populated) or an
// ambiguous-vendor state (AvailableVendorIDs populated, no tokens issued, and
// — critically — the OTP has NOT been consumed, so the caller can resubmit
// with VendorID set using the SAME still-active code).
type VerifyResult struct {
	Tokens               *TokenResult
	RequiresVendorChoice bool
	AvailableVendorIDs   []string
}

func (s *AuthService) Verify(ctx context.Context, in VerifyInput) (VerifyResult, error) {
	return s.verify(ctx, in, nil)
}

func (s *AuthService) Register(ctx context.Context, in VerifyInput, registration RegistrationInput) (VerifyResult, error) {
	if s.registration == nil {
		return VerifyResult{}, apperrors.Forbidden("Self-registration is unavailable on this server.", nil)
	}
	if in.VendorID != "" {
		return VerifyResult{}, apperrors.Validation("Registration cannot select an existing business.", nil)
	}
	return s.verify(ctx, in, &registration)
}

func (s *AuthService) verify(ctx context.Context, in VerifyInput, registration *RegistrationInput) (VerifyResult, error) {
	_, _, identifierKey, err := normalizeIdentifier(in.Phone, in.Email)
	if err != nil {
		return VerifyResult{}, err
	}
	if strings.TrimSpace(in.OTP) == "" {
		return VerifyResult{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "otp", Message: "Verification code is required."},
		})
	}

	challenge, ok, err := s.challenges.GetActiveByIdentifier(ctx, identifierKey, vendorauthmodels.PurposeLogin)
	if err != nil {
		return VerifyResult{}, err
	}
	if !ok {
		return VerifyResult{}, apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}

	if err := challenge.Verify(s.otpSecret, in.ChallengeID, in.OTP, s.otpMaxAttempts, s.now()); err != nil {
		_ = s.challenges.RecordFailedAttempt(ctx, challenge.ID, s.otpMaxAttempts)
		return VerifyResult{}, err
	}

	// Code is correct. Resolve the login identity and its staff affiliations
	// BEFORE consuming the challenge, so an ambiguous-vendor response never
	// burns a valid code (see package doc / VerifyResult).
	user, err := s.upsertUser(ctx, in.Phone, in.Email)
	if err != nil {
		return VerifyResult{}, err
	}

	if registration != nil {
		if err := s.registration.RegisterOwner(ctx, user.ID, *registration); err != nil {
			return VerifyResult{}, err
		}
	}

	var target StaffAffiliation
	if in.VendorID != "" {
		target, err = s.staff.GetByVendorAndUser(ctx, in.VendorID, user.ID)
		if err != nil {
			return VerifyResult{}, apperrors.Unauthorized("You do not have staff access to this vendor business.", err)
		}
	} else {
		affiliations, err := s.staff.ListActiveByUser(ctx, user.ID)
		if err != nil {
			return VerifyResult{}, err
		}
		switch len(affiliations) {
		case 0:
			return VerifyResult{}, apperrors.Forbidden("No active business is linked to this account. Create a business account or ask the business owner to invite you.", nil)
		case 1:
			target = affiliations[0]
		default:
			ids := make([]string, len(affiliations))
			for i, a := range affiliations {
				ids[i] = a.VendorID
			}
			return VerifyResult{RequiresVendorChoice: true, AvailableVendorIDs: ids}, nil
		}
	}

	if !target.Active {
		return VerifyResult{}, apperrors.Forbidden("Your access has been suspended. Contact your business owner.", nil)
	}

	consumed, err := s.challenges.Consume(ctx, challenge.ID)
	if err != nil {
		return VerifyResult{}, err
	}
	if !consumed {
		// Someone else (a concurrent request with the same code) already
		// consumed it first — this request must not also succeed.
		return VerifyResult{}, apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}

	tokens, err := s.createSessionTokens(ctx, target.StaffID, target.VendorID, in.DeviceID, in.UserAgent, in.IPAddress)
	if err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{Tokens: &tokens}, nil
}

// ─── Refresh ────────────────────────────────────────────────────────────

type RefreshInput struct {
	RefreshToken string
	DeviceID     *string
	UserAgent    string
	IPAddress    string
}

func (s *AuthService) Refresh(ctx context.Context, in RefreshInput) (TokenResult, error) {
	sessionID, err := parseSessionID(in.RefreshToken)
	if err != nil {
		return TokenResult{}, err
	}
	existing, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return TokenResult{}, apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}
	if !existing.IsActive(s.now()) {
		return TokenResult{}, apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}
	tokenHash := sharedauth.HashRefreshToken(s.refreshSecret, in.RefreshToken)
	if !hmac.Equal([]byte(tokenHash), []byte(existing.RefreshTokenHash)) {
		return TokenResult{}, apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}

	// Re-check staff status on EVERY refresh, not only at login — this is
	// what makes suspension take effect immediately rather than only at
	// access-token expiry (Section 5 rule 10/13).
	staff, err := s.staff.GetByID(ctx, existing.StaffID)
	if err != nil {
		return TokenResult{}, apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}
	if !staff.Active {
		_ = s.sessions.Revoke(ctx, existing.ID, "staff_suspended")
		return TokenResult{}, apperrors.Forbidden("Your access has been suspended. Contact your business owner.", nil)
	}

	if err := s.sessions.Revoke(ctx, existing.ID, "rotated"); err != nil {
		return TokenResult{}, err
	}
	return s.createSessionTokens(ctx, staff.StaffID, staff.VendorID, in.DeviceID, in.UserAgent, in.IPAddress)
}

// ─── Logout ─────────────────────────────────────────────────────────────

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	sessionID, err := parseSessionID(refreshToken)
	if err != nil {
		return err
	}
	existing, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}
	tokenHash := sharedauth.HashRefreshToken(s.refreshSecret, refreshToken)
	if !hmac.Equal([]byte(tokenHash), []byte(existing.RefreshTokenHash)) {
		return apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}
	if err := s.sessions.Revoke(ctx, sessionID, "logout"); err != nil {
		return err
	}
	return s.stepUps.RevokeForSession(ctx, sessionID)
}

// RevokeSession is used by other Section 5 flows (e.g. staff suspension) that
// need to end a session server-side, not just by logout.
func (s *AuthService) RevokeSession(ctx context.Context, sessionID, reason string) error {
	if err := s.sessions.Revoke(ctx, sessionID, reason); err != nil {
		return err
	}
	return s.stepUps.RevokeForSession(ctx, sessionID)
}

// RevokeAllSessionsForStaff revokes every active session for a staff member —
// used at suspension time so an already-issued refresh token cannot keep a
// suspended staff member's access alive.
func (s *AuthService) RevokeAllSessionsForStaff(ctx context.Context, staffID, reason string) error {
	_, err := s.sessions.RevokeAllForStaff(ctx, staffID, reason)
	return err
}

// ─── helpers ──────────────────────────────────────────────────────────────

func (s *AuthService) createSessionTokens(ctx context.Context, staffID, vendorID string, deviceID *string, userAgent, ipAddress string) (TokenResult, error) {
	sessionID := "vsess_" + uuid.NewString()
	secret, err := sharedauth.GenerateOpaqueToken(32)
	if err != nil {
		return TokenResult{}, apperrors.Internal("Refresh token could not be generated.", err)
	}
	refreshToken := sessionID + "." + secret
	expiresAt := s.now().Add(s.refreshTokenTTL)

	session := vendorauthmodels.Session{
		ID: sessionID, StaffID: staffID,
		RefreshTokenHash: sharedauth.HashRefreshToken(s.refreshSecret, refreshToken),
		DeviceID:         deviceID, UserAgent: userAgent, IPAddress: ipAddress, ExpiresAt: expiresAt,
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return TokenResult{}, err
	}

	// Access-token claims are deliberately minimal (Subject/Role/Service/
	// SessionID only) — full vendor/branch/permission scope is resolved
	// server-side from this SessionID on every request, never embedded in
	// the JWT. See docs/architecture/09_SECURITY_AND_AUTHORITY_MODEL.md for
	// why: this reuses shared/go/auth.Claims exactly as every other service
	// does (no competing token standard), and it makes suspension/permission
	// changes take effect on the very next request rather than only at
	// token expiry.
	accessExpiresAt := s.now().Add(s.accessTokenTTL)
	accessToken, err := s.accessSigner.Sign(sharedauth.Claims{
		Subject: staffID, Role: AccessRole, Service: AccessService, SessionID: sessionID,
		Type: sharedauth.TokenTypeAccess, ExpiresAt: accessExpiresAt.Unix(),
	})
	if err != nil {
		return TokenResult{}, apperrors.Internal("Access token could not be generated.", err)
	}

	return TokenResult{
		AccessToken: accessToken, RefreshToken: refreshToken, ExpiresIn: int64(s.accessTokenTTL.Seconds()),
		StaffID: staffID, VendorID: vendorID,
	}, nil
}

func (s *AuthService) upsertUser(ctx context.Context, phone, email string) (vendorauthrepositories.User, error) {
	phone = strings.TrimSpace(phone)
	email = strings.ToLower(strings.TrimSpace(email))
	if phone != "" {
		normalized, err := phonenumber.NormalizeNigerianPhoneNumber(phone)
		if err != nil {
			return vendorauthrepositories.User{}, err
		}
		return s.users.UpsertByPhone(ctx, normalized)
	}
	return s.users.UpsertByEmail(ctx, email)
}

// deliverOTP sends the code through the configured Notifier. In debug mode
// (local/test environments only — see Config.Validate, which refuses
// OTPDebug=true in production) delivery is skipped entirely: the caller
// already has the code via StartResult.DebugOTP, and skipping the real send
// avoids depending on a channel that may not be configured in dev. This
// never logs the code either way — DebugOTP travels only in the direct API
// response to the caller who just proved they control the identifier via a
// legitimate Start/Resend call.
func (s *AuthService) deliverOTP(ctx context.Context, identifierType, identifierValue, purpose, otp string) error {
	if s.otpDebug {
		return nil
	}
	if s.notifier == nil {
		return apperrors.Unavailable("Verification code delivery is not configured.", nil)
	}
	return s.notifier.SendOTP(ctx, identifierType, identifierValue, purpose, otp)
}

func normalizeIdentifier(phone, email string) (identifierType, identifierValue, identifierKey string, err error) {
	phone = strings.TrimSpace(phone)
	email = strings.ToLower(strings.TrimSpace(email))
	if phone == "" && email == "" {
		return "", "", "", apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone", Message: "Phone number or email address is required."},
		})
	}
	if phone != "" {
		normalized, nerr := phonenumber.NormalizeNigerianPhoneNumber(phone)
		if nerr != nil {
			return "", "", "", nerr
		}
		return vendorauthmodels.IdentifierPhone, normalized, vendorauthmodels.IdentifierPhone + ":" + normalized, nil
	}
	if !strings.Contains(email, "@") {
		return "", "", "", apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "email", Message: "Email address is invalid."},
		})
	}
	return vendorauthmodels.IdentifierEmail, email, vendorauthmodels.IdentifierEmail + ":" + email, nil
}

func parseSessionID(refreshToken string) (string, error) {
	idx := strings.IndexByte(refreshToken, '.')
	if idx <= 0 {
		return "", apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}
	return refreshToken[:idx], nil
}
