package providerauthusecases

import (
	"context"
	"crypto/hmac"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	providerauthrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/phonenumber"
	"cosmicforge/logistics/shared/go/providercompat"
)

// ProviderRole and ProviderService are the bearer-middleware contract for every
// driver route. payment-wallet-service and support-dispute-service already
// assume both values for taxi tokens, so neither may be renamed.
const (
	ProviderRole    = "taxi_provider"
	ProviderService = "taxi"
)

type AuthService struct {
	providers  providerauthrepositories.ProviderRepository
	sessions   providerauthrepositories.RefreshSessionRepository
	challenges providerauthrepositories.OTPChallengeRepository

	accessSigner  *sharedauth.TokenSigner
	otpSecret     []byte
	refreshSecret []byte

	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	otpTTL          time.Duration
	otpRateWindow   time.Duration
	otpMaxRequests  int
	otpMaxAttempts  int
	otpDebug        bool
	testLogin       *sharedauth.TestLogin
	otpSender       OTPSender
	now             func() time.Time
	corporate       providercompat.CorporateContextReader
}

type Options struct {
	Providers          providerauthrepositories.ProviderRepository
	Sessions           providerauthrepositories.RefreshSessionRepository
	Challenges         providerauthrepositories.OTPChallengeRepository
	AccessTokenSecret  []byte
	RefreshTokenSecret []byte
	OTPSecret          []byte
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	OTPTTL             time.Duration
	OTPRateWindow      time.Duration
	OTPMaxRequests     int
	OTPMaxAttempts     int
	OTPDebug           bool
	// TestLogin gives listed test numbers a fixed code (staging only).
	TestLogin        *sharedauth.TestLogin
	CorporateContext providercompat.CorporateContextReader
	// OTPSender delivers login codes. Nil keeps the old behaviour (codes
	// only reach the driver as debug codes).
	OTPSender OTPSender
}

// OTPSender delivers a login code to the identifier it was issued for.
type OTPSender interface {
	SendLoginCode(ctx context.Context, identifierType, identifier, code string) error
}

func NewAuthService(opts Options) *AuthService {
	return &AuthService{
		providers:       opts.Providers,
		sessions:        opts.Sessions,
		challenges:      opts.Challenges,
		accessSigner:    sharedauth.NewTokenSigner(opts.AccessTokenSecret),
		otpSecret:       opts.OTPSecret,
		refreshSecret:   opts.RefreshTokenSecret,
		accessTokenTTL:  opts.AccessTokenTTL,
		refreshTokenTTL: opts.RefreshTokenTTL,
		otpTTL:          opts.OTPTTL,
		otpRateWindow:   opts.OTPRateWindow,
		otpMaxRequests:  opts.OTPMaxRequests,
		otpMaxAttempts:  opts.OTPMaxAttempts,
		otpDebug:        opts.OTPDebug,
		testLogin:       opts.TestLogin,
		otpSender:       opts.OTPSender,
		now:             time.Now,
		corporate:       opts.CorporateContext,
	}
}

// ─── Start Auth ───────────────────────────────────────────────────────────────

type StartAuthInput struct {
	Phone string
	Email string
}

type StartAuthResult struct {
	ChallengeID string `json:"challenge_id"`
	ExpiresIn   int64  `json:"expires_in"`

	// DebugOTP is tagged json:"-" so it can never be serialized by accident. The
	// HTTP layer copies it into the DTO explicitly, and only when OTPDebug is on.
	DebugOTP string `json:"-"`
}

// loginIdentifier is the one identifier a sign-in uses: a normalised
// Nigerian phone number, or a trimmed lowercase email address. Phone wins
// when both are sent.
type loginIdentifier struct {
	Type  string // "phone" or "email"
	Value string
}

func (id loginIdentifier) key() string { return id.Type + ":" + id.Value }

func normalizeLoginIdentifier(phone, email string) (loginIdentifier, error) {
	phone = strings.TrimSpace(phone)
	email = strings.ToLower(strings.TrimSpace(email))
	switch {
	case phone != "":
		normalized, err := phonenumber.NormalizeNigerianPhoneNumber(phone)
		if err != nil {
			return loginIdentifier{}, err
		}
		return loginIdentifier{Type: "phone", Value: normalized}, nil
	case email != "":
		at := strings.Index(email, "@")
		if at < 1 || at == len(email)-1 || strings.ContainsAny(email, " \t") {
			return loginIdentifier{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "email", Message: "Email address is invalid."},
			})
		}
		return loginIdentifier{Type: "email", Value: email}, nil
	default:
		return loginIdentifier{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone", Message: "Phone number or email address is required."},
		})
	}
}

// StartAuth issues an OTP challenge for a driver phone number or email
// address, and sends email codes through OTPSender.
// loginCode is the fixed code for a listed test number (staging only),
// otherwise a fresh random one.
func (s *AuthService) loginCode(identifierType, value string) (string, error) {
	if code, ok := s.testLogin.CodeFor(identifierType, value); ok {
		return code, nil
	}
	return sharedauth.GenerateNumericOTP(sharedauth.DefaultOTPLength)
}

func (s *AuthService) StartAuth(ctx context.Context, input StartAuthInput) (StartAuthResult, error) {
	identifier, err := normalizeLoginIdentifier(input.Phone, input.Email)
	if err != nil {
		return StartAuthResult{}, err
	}
	identifierKey := identifier.key()

	otp, err := s.loginCode(identifier.Type, identifier.Value)
	if err != nil {
		return StartAuthResult{}, apperrors.Internal("OTP could not be generated.", err)
	}

	challengeID := uuid.NewString()
	challenge := providerauthmodels.OTPChallenge{
		ID:              challengeID,
		IdentifierType:  identifier.Type,
		IdentifierValue: identifier.Value,
		// The hash binds the OTP secret, the challenge id and the identifier, so a
		// code is only ever valid for the challenge and identifier it was minted for.
		OTPHash:   sharedauth.HashOTP(s.otpSecret, challengeID, identifierKey, otp),
		ExpiresAt: s.now().Add(s.otpTTL),
	}

	if err := s.challenges.Save(ctx, challenge, s.otpTTL, s.otpRateWindow, s.otpMaxRequests); err != nil {
		return StartAuthResult{}, err
	}

	if s.otpSender != nil {
		if err := s.otpSender.SendLoginCode(ctx, identifier.Type, identifier.Value, otp); err != nil {
			return StartAuthResult{}, apperrors.Unavailable("We could not send your code right now. Please try again.", err)
		}
	}

	result := StartAuthResult{
		ChallengeID: challengeID,
		ExpiresIn:   int64(s.otpTTL.Seconds()),
	}
	if s.otpDebug {
		result.DebugOTP = otp
	}
	return result, nil
}

// ─── Verify Auth ──────────────────────────────────────────────────────────────

type VerifyAuthInput struct {
	Phone       string
	Email       string
	OTP         string
	ChallengeID string
	DeviceID    *string
	UserAgent   string
	IPAddress   string
	// ReferralLinkCode and ReferralCode are an optional referral code from a
	// shared link or typed by the driver. They matter only when this
	// verification creates the account.
	ReferralLinkCode string
	ReferralCode     string
}

type TokenResult struct {
	AccessToken               string                                    `json:"access_token"`
	RefreshToken              string                                    `json:"refresh_token"`
	ExpiresIn                 int64                                     `json:"expires_in"`
	Provider                  providerauthmodels.PublicProvider         `json:"provider"`
	ProviderMode              string                                    `json:"providerMode,omitempty"`
	CorporateContext          *providercompat.CorporateContext          `json:"corporateContext,omitempty"`
	FinancialVisibilityPolicy *providercompat.FinancialVisibilityPolicy `json:"financialVisibilityPolicy,omitempty"`
}

// VerifyAuth exchanges a correct OTP for a token pair, creating the driver on
// first login with onboarding_status=profile_required.
func (s *AuthService) VerifyAuth(ctx context.Context, input VerifyAuthInput) (TokenResult, error) {
	identifier, err := normalizeLoginIdentifier(input.Phone, input.Email)
	if err != nil {
		return TokenResult{}, err
	}
	if input.OTP == "" {
		return TokenResult{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "otp", Message: "Verification code is required."},
		})
	}

	identifierKey := identifier.key()

	challenge, ok, err := s.challenges.Get(ctx, identifierKey)
	if err != nil {
		return TokenResult{}, err
	}
	if !ok {
		return TokenResult{}, apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}

	if err := providerauthmodels.VerifyOTPChallenge(s.otpSecret, challenge, input.ChallengeID, input.OTP, s.otpMaxAttempts, s.now()); err != nil {
		// Burn an attempt under the challenge's remaining TTL, so a wrong code
		// costs a try without extending how long the challenge lives.
		_ = s.challenges.RecordFailedAttempt(ctx, challenge, time.Until(challenge.ExpiresAt))
		return TokenResult{}, err
	}

	// One-shot: a correct code is consumed even if session creation then fails,
	// so a captured code cannot be replayed.
	_ = s.challenges.Delete(ctx, identifierKey)

	signup := providerauthmodels.Signup{LinkCode: input.ReferralLinkCode, TypedCode: input.ReferralCode}
	if input.DeviceID != nil {
		signup.DeviceID = *input.DeviceID
	}
	var provider providerauthmodels.Provider
	if identifier.Type == "email" {
		provider, err = s.providers.UpsertByEmail(ctx, identifier.Value, signup)
	} else {
		provider, err = s.providers.UpsertByPhone(ctx, identifier.Value, signup)
	}
	if err != nil {
		return TokenResult{}, err
	}

	return s.createSessionTokens(ctx, provider, input.DeviceID, input.UserAgent, input.IPAddress)
}

// ─── Refresh ──────────────────────────────────────────────────────────────────

type RefreshInput struct {
	RefreshToken string
	DeviceID     *string
	UserAgent    string
	IPAddress    string
}

// Refresh rotates the session: the presented session is revoked and a fresh
// pair issued, so a leaked refresh token is usable at most once.
func (s *AuthService) Refresh(ctx context.Context, input RefreshInput) (TokenResult, error) {
	sessionID, err := parseRefreshSessionID(input.RefreshToken)
	if err != nil {
		return TokenResult{}, err
	}

	existing, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return TokenResult{}, err
	}
	if !existing.IsActive(s.now()) {
		return TokenResult{}, apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}

	tokenHash := sharedauth.HashRefreshToken(s.refreshSecret, input.RefreshToken)
	if !hmac.Equal([]byte(tokenHash), []byte(existing.RefreshTokenHash)) {
		return TokenResult{}, apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}

	provider, err := s.providers.GetByID(ctx, existing.ProviderID)
	if err != nil {
		return TokenResult{}, err
	}
	if err := s.sessions.Revoke(ctx, existing.ID); err != nil {
		return TokenResult{}, err
	}
	return s.createSessionTokens(ctx, provider, input.DeviceID, input.UserAgent, input.IPAddress)
}

// ─── Logout ───────────────────────────────────────────────────────────────────

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	sessionID, err := parseRefreshSessionID(refreshToken)
	if err != nil {
		return err
	}
	existing, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if !existing.IsActive(s.now()) {
		return apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}
	tokenHash := sharedauth.HashRefreshToken(s.refreshSecret, refreshToken)
	if !hmac.Equal([]byte(tokenHash), []byte(existing.RefreshTokenHash)) {
		return apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}
	return s.sessions.Revoke(ctx, sessionID)
}

// ─── Change Phone ─────────────────────────────────────────────────────────────
// Reuses the OTP infrastructure: a code is sent to the NEW phone and verified
// before the driver's number is updated. Bearer-protected — the driver is
// identified by the access token, never by the OTP.

func (s *AuthService) ChangePhoneStart(ctx context.Context, providerID, newPhone string) (StartAuthResult, error) {
	normalized, err := phonenumber.NormalizeNigerianPhoneNumber(strings.TrimSpace(newPhone))
	if err != nil {
		return StartAuthResult{}, err
	}

	otp, err := sharedauth.GenerateNumericOTP(sharedauth.DefaultOTPLength)
	if err != nil {
		return StartAuthResult{}, apperrors.Internal("OTP could not be generated.", err)
	}

	identifierKey := "phone:" + normalized
	challengeID := uuid.NewString()
	challenge := providerauthmodels.OTPChallenge{
		ID:              challengeID,
		IdentifierType:  "phone",
		IdentifierValue: normalized,
		OTPHash:         sharedauth.HashOTP(s.otpSecret, challengeID, identifierKey, otp),
		ExpiresAt:       s.now().Add(s.otpTTL),
	}
	if err := s.challenges.Save(ctx, challenge, s.otpTTL, s.otpRateWindow, s.otpMaxRequests); err != nil {
		return StartAuthResult{}, err
	}

	result := StartAuthResult{
		ChallengeID: challengeID,
		ExpiresIn:   int64(s.otpTTL.Seconds()),
	}
	if s.otpDebug {
		result.DebugOTP = otp
	}
	return result, nil
}

func (s *AuthService) ChangePhoneVerify(ctx context.Context, providerID, newPhone, otp, challengeID string) (providerauthmodels.PublicProvider, error) {
	normalized, err := phonenumber.NormalizeNigerianPhoneNumber(strings.TrimSpace(newPhone))
	if err != nil {
		return providerauthmodels.PublicProvider{}, err
	}
	if otp == "" {
		return providerauthmodels.PublicProvider{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "otp", Message: "Verification code is required."},
		})
	}

	identifierKey := "phone:" + normalized
	challenge, ok, err := s.challenges.Get(ctx, identifierKey)
	if err != nil {
		return providerauthmodels.PublicProvider{}, err
	}
	if !ok {
		return providerauthmodels.PublicProvider{}, apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}
	if err := providerauthmodels.VerifyOTPChallenge(s.otpSecret, challenge, challengeID, otp, s.otpMaxAttempts, s.now()); err != nil {
		_ = s.challenges.RecordFailedAttempt(ctx, challenge, time.Until(challenge.ExpiresAt))
		return providerauthmodels.PublicProvider{}, err
	}
	_ = s.challenges.Delete(ctx, identifierKey)

	provider, err := s.providers.UpdatePhone(ctx, providerID, normalized)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return providerauthmodels.PublicProvider{}, apperrors.Validation("This phone number is already in use.", []apperrors.FieldViolation{
				{Field: "phone", Message: "This phone number is already in use."},
			})
		}
		return providerauthmodels.PublicProvider{}, err
	}
	return provider.Public(), nil
}

func (s *AuthService) Me(ctx context.Context, providerID string) (providerauthmodels.PublicProvider, error) {
	p, err := s.providers.GetByID(ctx, providerID)
	if err != nil {
		return providerauthmodels.PublicProvider{}, err
	}
	return p.Public(), nil
}

func (s *AuthService) AccessSigner() *sharedauth.TokenSigner {
	return s.accessSigner
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func (s *AuthService) createSessionTokens(ctx context.Context, provider providerauthmodels.Provider, deviceID *string, userAgent, ipAddress string) (TokenResult, error) {
	sessionID := uuid.NewString()
	secret, err := sharedauth.GenerateOpaqueToken(32)
	if err != nil {
		return TokenResult{}, apperrors.Internal("Refresh token could not be generated.", err)
	}
	// The refresh token is opaque and prefixed by its session id so the session
	// can be looked up without storing the token itself; only the HMAC is stored.
	refreshToken := sessionID + "." + secret
	expiresAt := s.now().Add(s.refreshTokenTTL)

	session := providerauthmodels.RefreshSession{
		ID:               sessionID,
		ProviderID:       provider.ID,
		RefreshTokenHash: sharedauth.HashRefreshToken(s.refreshSecret, refreshToken),
		DeviceID:         deviceID,
		UserAgent:        userAgent,
		IPAddress:        ipAddress,
		ExpiresAt:        expiresAt,
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return TokenResult{}, err
	}

	accessExpiresAt := s.now().Add(s.accessTokenTTL)
	accessToken, err := s.accessSigner.Sign(sharedauth.Claims{
		Subject:   provider.ID,
		Role:      ProviderRole,
		Service:   ProviderService,
		SessionID: sessionID,
		Type:      sharedauth.TokenTypeAccess,
		ExpiresAt: accessExpiresAt.Unix(),
	})
	if err != nil {
		return TokenResult{}, apperrors.Internal("Access token could not be generated.", err)
	}

	result := TokenResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.accessTokenTTL.Seconds()),
		Provider:     provider.Public(),
	}
	if s.corporate != nil {
		context, err := s.corporate.ReadCorporateContext(ctx, "TAXI", provider.ID)
		if err != nil {
			return TokenResult{}, apperrors.Unavailable("Fleet corporate context could not be verified. Please try again.", err)
		}
		if context.IsManagedProvider {
			result.ProviderMode = providercompat.ModeOrganizationManaged
			result.CorporateContext = &context
			result.FinancialVisibilityPolicy = &context.FinancialVisibilityPolicy
		} else {
			result.ProviderMode = providercompat.ModeIndependent
		}
	}
	return result, nil
}

func parseRefreshSessionID(refreshToken string) (string, error) {
	idx := strings.IndexByte(refreshToken, '.')
	if idx <= 0 {
		return "", apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}
	return refreshToken[:idx], nil
}
