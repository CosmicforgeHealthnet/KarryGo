package authusecases

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	authclients "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/clients"
	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	authrepositories "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/repositories"
	"karrygo/shared/go/apperrors"
)

// compile-time check: RedisEventPublisher implements EventPublisher
var _ authclients.EventPublisher = (*authclients.RedisEventPublisher)(nil)

type AuthUsecase struct {
	identities authrepositories.IdentityRepository
	otp        *OTPUsecase
	tokens     *TokenUsecase
	sessions   *SessionUsecase
	notifier   authclients.NotificationClient
	publisher  authclients.EventPublisher
	otpDebug   bool
}

type Options struct {
	Identities authrepositories.IdentityRepository
	OTPs       authrepositories.OTPRepository
	Sessions   authrepositories.SessionRepository
	Notifier   authclients.NotificationClient
	Publisher  authclients.EventPublisher
	// OTPUsecase is an optional override for testing (e.g. to inject a fake rate-limiter).
	OTPUsecase         *OTPUsecase
	AccessTokenSecret  []byte
	RefreshTokenSecret []byte
	OTPSecret          []byte
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	OTPTTL             time.Duration
	OTPRateWindow      time.Duration
	OTPMaxRequests     int
	OTPMaxAttempts     int
	OTPLockoutTTL      time.Duration
	OTPDebug           bool
	Redis              *redis.Client
}

type RequestMetadata struct {
	DeviceID   *string
	DeviceType *string
	IPAddress  string
	UserAgent  string
}

type StartInput struct {
	PhoneNumber   string
	CorrelationID string
}

// StartResult contains only what the HTTP response needs.
// OTP is never included in StartResult; it must not appear in API responses.
type StartResult struct {
	ExpiresInSeconds int64 `json:"expires_in_seconds"`
}

type VerifyInput struct {
	PhoneNumber   string
	OTPCode       string
	CorrelationID string
	Metadata      RequestMetadata
}

// TokenResult is returned by Verify and maps directly to the HTTP response data payload.
// provider_id is the existing response field for the stable dispatch rider UUID.
// role is always "dispatch_provider".
// OTP and hashed tokens are never included here.
type TokenResult struct {
	ProviderID       string `json:"provider_id"`
	Role             string `json:"role"`
	TokenType        string `json:"token_type"`
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

type RefreshInput struct {
	RefreshToken string
}

type RefreshResult struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

type LogoutInput struct {
	SessionID       string
	DispatchRiderID string
	PhoneNumber     string
	Role            string
	RefreshToken    *string
	CorrelationID   string
}

type LogoutResult struct {
	Message string `json:"message"`
}

func NewAuthUsecase(opts Options) *AuthUsecase {
	otpUsecase := opts.OTPUsecase
	if otpUsecase == nil {
		otpUsecase = NewOTPUsecase(OTPOptions{
			Repository:  opts.OTPs,
			Redis:       opts.Redis,
			Secret:      opts.OTPSecret,
			TTL:         opts.OTPTTL,
			RateWindow:  opts.OTPRateWindow,
			MaxRequests: opts.OTPMaxRequests,
			MaxAttempts: opts.OTPMaxAttempts,
			LockoutTTL:  opts.OTPLockoutTTL,
		})
	}
	sessions := NewSessionUsecase(opts.Sessions, opts.RefreshTokenSecret, opts.RefreshTokenTTL)
	if opts.Redis != nil {
		sessions = sessions.WithRedis(opts.Redis)
	}

	return &AuthUsecase{
		identities: opts.Identities,
		otp:        otpUsecase,
		tokens:     NewTokenUsecase(opts.AccessTokenSecret, opts.AccessTokenTTL, opts.RefreshTokenTTL),
		sessions:   sessions,
		notifier:   opts.Notifier,
		publisher:  opts.Publisher,
		otpDebug:   opts.OTPDebug,
	}
}

func (u *AuthUsecase) Start(ctx context.Context, input StartInput) (StartResult, error) {
	phoneNumber, err := NormalizePhoneNumber(input.PhoneNumber)
	if err != nil {
		return StartResult{}, err
	}

	otp, code, err := u.otp.Create(ctx, phoneNumber)
	if err != nil {
		return StartResult{}, err
	}

	// Dev-only: log plain OTP when debug flag is set. Never log in production.
	if u.notifier != nil {
		if err := u.notifier.SendOTP(ctx, phoneNumber, code); err != nil {
			return StartResult{}, apperrors.Unavailable("Verification code delivery is temporarily unavailable.", err)
		}
	}

	// Phase 1E: publish the OTP-requested event for notification-service.
	// otp_code is included so notification-service can embed it in the SMS body.
	// If no subscriber is listening, Redis pub/sub silently drops the event.
	if u.publisher != nil {
		event := authclients.OTPRequestedEvent{
			Event:         authclients.TopicOTPRequested,
			CorrelationID: input.CorrelationID,
			PhoneNumber:   otp.PhoneNumber,
			OTPCode:       code,
			Purpose:       "login",
			ExpiresIn:     int(u.otp.TTLSeconds()),
			CreatedAt:     time.Now().UTC(),
		}
		if err := u.publisher.PublishOTPRequested(ctx, event); err != nil {
			return StartResult{}, apperrors.Internal("Event publishing failed.", err)
		}
	}

	return StartResult{ExpiresInSeconds: u.otp.TTLSeconds()}, nil
}

// Verify validates the OTP, upserts the dispatch rider identity, creates a session,
// publishes the session-created event, and returns JWT access + refresh tokens.
//
// Processing order (Phase 1F/1G):
//  1. Normalise phone number (E.164)
//  2. Verify OTP (expiry, lock, attempt tracking via OTPUsecase)
//  3. Upsert rider identity (stable provider_id response field per phone number)
//  4. Block suspended/deleted identities
//  5. Generate access token (15 min) + opaque refresh token (30 days)
//  6. Create session row (refresh token stored as SHA-256 hash)
//  7. Cache session in Redis (non-fatal)
//  8. Publish the session-created event
//  9. Return tokens; plain refresh token is never stored in DB or logged
func (u *AuthUsecase) Verify(ctx context.Context, input VerifyInput) (TokenResult, error) {
	phoneNumber, err := NormalizePhoneNumber(input.PhoneNumber)
	if err != nil {
		return TokenResult{}, err
	}

	// Step 2: verify OTP (handles expiry, lock, attempt increments internally)
	if _, err := u.otp.VerifyLatest(ctx, phoneNumber, input.OTPCode); err != nil {
		return TokenResult{}, err
	}

	// Step 3: upsert identity; same phone always returns the same provider_id response field.
	identity, err := u.identities.UpsertByPhone(ctx, phoneNumber)
	if err != nil {
		return TokenResult{}, err
	}

	// Step 4: block suspended / deleted identities
	if !identity.CanCreateSession() {
		return TokenResult{}, apperrors.Forbidden("This account cannot create a session.", nil)
	}

	// Step 5: generate tokens
	sessionID := uuid.NewString()
	accessToken, _, err := u.tokens.GenerateAccessToken(identity.ID, phoneNumber, sessionID)
	if err != nil {
		return TokenResult{}, apperrors.Internal("Access token could not be generated.", err)
	}
	refreshToken, err := u.sessions.GenerateSecureToken(32)
	if err != nil {
		return TokenResult{}, apperrors.Internal("Refresh token could not be generated.", err)
	}

	// Step 6+7: create session (DB + Redis cache via SessionUsecase)
	if _, err := u.sessions.Create(ctx, sessionID, identity.ID, phoneNumber, refreshToken, input.Metadata); err != nil {
		return TokenResult{}, err
	}

	// Step 8: publish the session-created event.
	if u.publisher != nil {
		event := authclients.SessionCreatedEvent{
			Event:         authclients.TopicSessionCreated,
			CorrelationID: input.CorrelationID,
			ProviderID:    identity.ID,
			PhoneNumber:   phoneNumber,
			Role:          authmodels.RoleDispatchProvider,
			SessionID:     sessionID,
			CreatedAt:     time.Now().UTC(),
		}
		if err := u.publisher.PublishSessionCreated(ctx, event); err != nil {
			return TokenResult{}, apperrors.Internal("Session event publishing failed.", err)
		}
	}

	return TokenResult{
		ProviderID:       identity.ID,
		Role:             authmodels.RoleDispatchProvider,
		TokenType:        "Bearer",
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresInSeconds: u.tokens.AccessTTLSeconds(),
	}, nil
}

func (u *AuthUsecase) Refresh(ctx context.Context, input RefreshInput) (RefreshResult, error) {
	refreshToken, err := validateRefreshTokenInput(input.RefreshToken)
	if err != nil {
		return RefreshResult{}, err
	}

	session, err := u.sessions.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return RefreshResult{}, err
	}

	identity, ok, err := u.identities.GetByID(ctx, session.DispatchRiderID)
	if err != nil {
		return RefreshResult{}, err
	}
	if !ok {
		return RefreshResult{}, refreshSessionUnauthorized()
	}
	if !identity.CanCreateSession() {
		return RefreshResult{}, apperrors.Forbidden("This account cannot refresh session.", nil)
	}

	accessToken, _, err := u.tokens.GenerateAccessToken(session.DispatchRiderID, session.PhoneNumber, session.ID)
	if err != nil {
		return RefreshResult{}, apperrors.Internal("Access token could not be generated.", err)
	}

	newRefreshToken, err := u.sessions.GenerateSecureToken(32)
	if err != nil {
		return RefreshResult{}, apperrors.Internal("Refresh token could not be generated.", err)
	}
	if err := u.sessions.RotateRefreshToken(ctx, session.ID, newRefreshToken); err != nil {
		return RefreshResult{}, err
	}

	return RefreshResult{
		AccessToken:      accessToken,
		RefreshToken:     newRefreshToken,
		TokenType:        "Bearer",
		ExpiresInSeconds: u.tokens.AccessTTLSeconds(),
	}, nil
}

func validateRefreshTokenInput(refreshToken string) (string, error) {
	token := strings.TrimSpace(refreshToken)
	if token == "" {
		return "", apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "refresh_token", Message: "Refresh token is required."},
		})
	}
	if len(token) != 64 {
		return "", apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "refresh_token", Message: "Refresh token must be a 64-character hex string."},
		})
	}
	if _, err := hex.DecodeString(token); err != nil {
		return "", apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "refresh_token", Message: "Refresh token must be a 64-character hex string."},
		})
	}

	return token, nil
}

func (u *AuthUsecase) Logout(ctx context.Context, input LogoutInput) (LogoutResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	dispatchRiderID := strings.TrimSpace(input.DispatchRiderID)
	if sessionID == "" || dispatchRiderID == "" {
		return LogoutResult{}, logoutSessionUnauthorized()
	}

	if input.RefreshToken != nil {
		refreshToken, err := validateRefreshTokenInput(*input.RefreshToken)
		if err != nil {
			return LogoutResult{}, err
		}
		session, err := u.sessions.ValidateRefreshToken(ctx, refreshToken)
		if err != nil {
			return LogoutResult{}, err
		}
		if session.ID != sessionID || session.DispatchRiderID != dispatchRiderID {
			return LogoutResult{}, logoutSessionUnauthorized()
		}
	}

	if err := u.sessions.Revoke(ctx, sessionID); err != nil {
		return LogoutResult{}, err
	}

	// TODO(Phase 1J): JWT jti claim is not currently included; access-token blacklist will be completed when jti support is added.
	if u.publisher != nil {
		event := authclients.LoggedOutEvent{
			Event:         authclients.TopicLoggedOut,
			CorrelationID: input.CorrelationID,
			ProviderID:    dispatchRiderID,
			SessionID:     sessionID,
			CreatedAt:     time.Now().UTC(),
		}
		if err := u.publisher.PublishLoggedOut(ctx, event); err != nil {
			return LogoutResult{}, apperrors.Internal("Logout event publishing failed.", err)
		}
	}

	return LogoutResult{Message: "Logged out successfully."}, nil
}

func (u *AuthUsecase) TokenUsecase() *TokenUsecase {
	return u.tokens
}
