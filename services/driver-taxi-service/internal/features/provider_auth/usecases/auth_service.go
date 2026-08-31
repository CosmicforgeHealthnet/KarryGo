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
	now             func() time.Time
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
		now:             time.Now,
	}
}

// ─── Start Auth ───────────────────────────────────────────────────────────────

type StartAuthInput struct {
	Phone string
}

type StartAuthResult struct {
	ChallengeID string `json:"challenge_id"`
	ExpiresIn   int64  `json:"expires_in"`

	// DebugOTP is tagged json:"-" so it can never be serialized by accident. The
	// HTTP layer copies it into the DTO explicitly, and only when OTPDebug is on.
	DebugOTP string `json:"-"`
}

// StartAuth issues an OTP challenge for a driver phone number. Taxi driver auth
// is phone-only; there is no email path.
func (s *AuthService) StartAuth(ctx context.Context, input StartAuthInput) (StartAuthResult, error) {
	phone := strings.TrimSpace(input.Phone)
	if phone == "" {
		return StartAuthResult{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone", Message: "Phone number is required."},
		})
	}

	normalized, err := phonenumber.NormalizeNigerianPhoneNumber(phone)
	if err != nil {
		return StartAuthResult{}, err
	}
	identifierKey := "phone:" + normalized

	otp, err := sharedauth.GenerateNumericOTP(sharedauth.DefaultOTPLength)
	if err != nil {
		return StartAuthResult{}, apperrors.Internal("OTP could not be generated.", err)
	}

	challengeID := uuid.NewString()
	challenge := providerauthmodels.OTPChallenge{
		ID:              challengeID,
		IdentifierType:  "phone",
		IdentifierValue: normalized,
		// The hash binds the OTP secret, the challenge id and the identifier, so a
		// code is only ever valid for the challenge and phone it was minted for.
		OTPHash:   sharedauth.HashOTP(s.otpSecret, challengeID, identifierKey, otp),
		ExpiresAt: s.now().Add(s.otpTTL),
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

// ─── Verify Auth ──────────────────────────────────────────────────────────────

type VerifyAuthInput struct {
	Phone       string
	OTP         string
	ChallengeID string
	DeviceID    *string
	UserAgent   string
	IPAddress   string
}

type TokenResult struct {
	AccessToken  string                            `json:"access_token"`
	RefreshToken string                            `json:"refresh_token"`
	ExpiresIn    int64                             `json:"expires_in"`
	Provider     providerauthmodels.PublicProvider `json:"provider"`
}

// VerifyAuth exchanges a correct OTP for a token pair, creating the driver on
// first login with onboarding_status=profile_required.
func (s *AuthService) VerifyAuth(ctx context.Context, input VerifyAuthInput) (TokenResult, error) {
	phone := strings.TrimSpace(input.Phone)
	if phone == "" {
		return TokenResult{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone", Message: "Phone number is required."},
		})
	}
	if input.OTP == "" {
		return TokenResult{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "otp", Message: "Verification code is required."},
		})
	}

	normalized, err := phonenumber.NormalizeNigerianPhoneNumber(phone)
	if err != nil {
		return TokenResult{}, err
	}
	identifierKey := "phone:" + normalized

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

	provider, err := s.providers.UpsertByPhone(ctx, normalized)
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

	return TokenResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.accessTokenTTL.Seconds()),
		Provider:     provider.Public(),
	}, nil
}

func parseRefreshSessionID(refreshToken string) (string, error) {
	idx := strings.IndexByte(refreshToken, '.')
	if idx <= 0 {
		return "", apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}
	return refreshToken[:idx], nil
}
