package authusecases

import (
	"context"
	"encoding/hex"
	"log"
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

// ── Start (legacy) ─────────────────────────────────────────────────────────────

type StartInput struct {
	PhoneNumber   string
	CorrelationID string
}

// StartResult contains only what the HTTP response needs.
// OTP is never included in StartResult; it must not appear in API responses.
type StartResult struct {
	ExpiresInSeconds int64 `json:"expires_in_seconds"`
}

// ── SignupStart ────────────────────────────────────────────────────────────────

type SignupStartInput struct {
	PhoneNumber   string
	Email         string
	CorrelationID string
}

// ── LoginStart ─────────────────────────────────────────────────────────────────

type LoginStartInput struct {
	// Identifier is either a phone number (E.164) or an email address.
	Identifier    string
	CorrelationID string
}

// ── Verify ────────────────────────────────────────────────────────────────────

type VerifyInput struct {
	// Legacy field: phone number.  Ignored when Identifier is non-empty.
	PhoneNumber string
	// Identifier: phone (E.164) or email. Takes precedence over PhoneNumber.
	Identifier string
	OTPCode    string
	// Purpose drives which identity operation is performed:
	//   ""       – legacy upsert (backward compatible with existing clients)
	//   "login"  – find existing identity; 404 if not found
	//   "signup" – create new identity; 409 if phone/email already exists
	Purpose       string
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

// Start handles the legacy POST /api/v1/auth/start (phone-only, no login/signup distinction).
func (u *AuthUsecase) Start(ctx context.Context, input StartInput) (StartResult, error) {
	phoneNumber, err := NormalizePhoneNumber(input.PhoneNumber)
	if err != nil {
		return StartResult{}, err
	}

	otp, code, err := u.otp.Create(ctx, phoneNumber)
	if err != nil {
		return StartResult{}, err
	}

	if u.notifier != nil {
		if err := u.notifier.SendOTP(ctx, phoneNumber, code); err != nil {
			return StartResult{}, apperrors.Unavailable("Verification code delivery is temporarily unavailable.", err)
		}
	}

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

// SignupStart handles POST /api/v1/auth/signup/start.
// Validates phone and email, checks neither is already registered, creates a
// signup OTP with the email stored in the OTP row for retrieval during Verify.
// OTP is sent to phone (via notifier) and logged to email in dev mode.
func (u *AuthUsecase) SignupStart(ctx context.Context, input SignupStartInput) (StartResult, error) {
	phoneNumber, err := NormalizePhoneNumber(input.PhoneNumber)
	if err != nil {
		return StartResult{}, err
	}

	email, err := NormalizeEmail(input.Email)
	if err != nil {
		return StartResult{}, err
	}

	// Reject if phone already registered.
	_, phoneExists, err := u.identities.FindByPhone(ctx, phoneNumber)
	if err != nil {
		return StartResult{}, err
	}
	if phoneExists {
		return StartResult{}, apperrors.Conflict("An account with this phone number already exists.", nil)
	}

	// Reject if email already registered.
	_, emailExists, err := u.identities.FindByEmail(ctx, email)
	if err != nil {
		return StartResult{}, err
	}
	if emailExists {
		return StartResult{}, apperrors.Conflict("An account with this email address already exists.", nil)
	}

	// Create signup OTP with email stored for the Verify step.
	otp, code, err := u.otp.CreateForSignup(ctx, phoneNumber, email)
	if err != nil {
		return StartResult{}, err
	}

	// Send OTP to phone.
	if u.notifier != nil {
		if err := u.notifier.SendOTP(ctx, phoneNumber, code); err != nil {
			return StartResult{}, apperrors.Unavailable("Verification code delivery is temporarily unavailable.", err)
		}
	}

	// Dev-only: log OTP for both delivery channels.  Never log in production.
	if u.otpDebug {
		log.Printf("development dispatch rider signup otp phone_number=%s otp=%s", phoneNumber, code)
		log.Printf("development dispatch rider signup otp email=%s otp=%s", email, code)
	}

	if u.publisher != nil {
		event := authclients.OTPRequestedEvent{
			Event:         authclients.TopicOTPRequested,
			CorrelationID: input.CorrelationID,
			PhoneNumber:   otp.PhoneNumber,
			OTPCode:       code,
			Purpose:       "signup",
			ExpiresIn:     int(u.otp.TTLSeconds()),
			CreatedAt:     time.Now().UTC(),
		}
		if err := u.publisher.PublishOTPRequested(ctx, event); err != nil {
			return StartResult{}, apperrors.Internal("Event publishing failed.", err)
		}
	}

	return StartResult{ExpiresInSeconds: u.otp.TTLSeconds()}, nil
}

// LoginStart handles POST /api/v1/auth/login/start.
// Accepts a phone number or email as identifier, looks up the existing identity,
// and returns 404 if no account is found.  Creates and sends a login OTP.
func (u *AuthUsecase) LoginStart(ctx context.Context, input LoginStartInput) (StartResult, error) {
	identifier := strings.TrimSpace(input.Identifier)
	if identifier == "" {
		return StartResult{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "identifier", Message: "Phone number or email address is required."},
		})
	}

	var phoneNumber string
	// identityEmail is non-nil when the account's email is known; used for
	// dev-mode OTP logging.  Nil for legacy phone-only accounts.
	var identityEmail *string

	if LooksLikeEmail(identifier) {
		// Email identifier — look up identity by email to get phone.
		email := strings.ToLower(identifier)
		identity, exists, err := u.identities.FindByEmail(ctx, email)
		if err != nil {
			return StartResult{}, err
		}
		if !exists {
			return StartResult{}, apperrors.NotFound("No account found with this email address.", nil)
		}
		phoneNumber = identity.PhoneNumber
		identityEmail = &email
	} else {
		// Phone identifier.
		var err error
		phoneNumber, err = NormalizePhoneNumber(identifier)
		if err != nil {
			return StartResult{}, err
		}
		identity, exists, err := u.identities.FindByPhone(ctx, phoneNumber)
		if err != nil {
			return StartResult{}, err
		}
		if !exists {
			return StartResult{}, apperrors.NotFound("No account found with this phone number.", nil)
		}
		identityEmail = identity.Email
	}

	// Create login OTP keyed by phone.
	otp, code, err := u.otp.Create(ctx, phoneNumber)
	if err != nil {
		return StartResult{}, err
	}

	if u.notifier != nil {
		if err := u.notifier.SendOTP(ctx, phoneNumber, code); err != nil {
			return StartResult{}, apperrors.Unavailable("Verification code delivery is temporarily unavailable.", err)
		}
	}

	// Dev-only: log OTP for all available delivery channels.  Never log in production.
	if u.otpDebug {
		log.Printf("development dispatch rider login otp phone_number=%s otp=%s", phoneNumber, code)
		if identityEmail != nil {
			log.Printf("development dispatch rider login otp email=%s otp=%s", *identityEmail, code)
		} else {
			log.Printf("development dispatch rider login otp warning: no email on record for phone_number=%s", phoneNumber)
		}
	}

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

// Verify validates the OTP, resolves/creates the identity based on purpose, creates
// a session, publishes the session-created event, and returns JWT access + refresh tokens.
//
// Purpose routing:
//   - ""       (legacy / no purpose) → UpsertByPhone (backward compat)
//   - "signup" → CreateForSignup (fail 409 if phone/email already registered)
//   - "login"  → FindByPhone/FindByEmail (fail 404 if not registered)
//
// Identifier routing:
//   - If Identifier is non-empty, it is used (phone or email).
//   - If Identifier is empty, PhoneNumber is used (legacy backward compat).
func (u *AuthUsecase) Verify(ctx context.Context, input VerifyInput) (TokenResult, error) {
	// Resolve the identifier to use (new field takes precedence over legacy).
	identifier := strings.TrimSpace(input.Identifier)
	if identifier == "" {
		identifier = strings.TrimSpace(input.PhoneNumber)
	}

	// Determine phone number for OTP lookup.
	var phoneNumber string
	var resolvedIdentity *authmodels.Identity // pre-fetched for login path

	if LooksLikeEmail(identifier) {
		// Email-based login: look up identity to get phone.
		email := strings.ToLower(identifier)
		identity, exists, err := u.identities.FindByEmail(ctx, email)
		if err != nil {
			return TokenResult{}, err
		}
		if !exists {
			return TokenResult{}, apperrors.Unauthorized("No account found.", nil)
		}
		phoneNumber = identity.PhoneNumber
		resolvedIdentity = &identity
	} else {
		var err error
		phoneNumber, err = NormalizePhoneNumber(identifier)
		if err != nil {
			return TokenResult{}, err
		}
	}

	// Verify OTP (handles expiry, lock, attempt tracking).
	verifiedOTP, err := u.otp.VerifyLatest(ctx, phoneNumber, input.OTPCode)
	if err != nil {
		return TokenResult{}, err
	}

	// Resolve identity based on purpose.
	var identity authmodels.Identity

	switch input.Purpose {
	case "signup":
		// Extract email stored in the OTP row during SignupStart.
		email := ""
		if verifiedOTP.Email != nil {
			email = *verifiedOTP.Email
		}
		identity, err = u.identities.CreateForSignup(ctx, phoneNumber, email)
		if err != nil {
			return TokenResult{}, err
		}

	case "login":
		if resolvedIdentity != nil {
			// Already fetched during email-path identifier resolution.
			identity = *resolvedIdentity
		} else {
			var exists bool
			identity, exists, err = u.identities.FindByPhone(ctx, phoneNumber)
			if err != nil {
				return TokenResult{}, err
			}
			if !exists {
				return TokenResult{}, apperrors.NotFound("No account found with this phone number.", nil)
			}
		}

	default:
		// Legacy path: upsert (create if new, touch updated_at if existing).
		identity, err = u.identities.UpsertByPhone(ctx, phoneNumber)
		if err != nil {
			return TokenResult{}, err
		}
	}

	// Block suspended / deleted identities.
	if !identity.CanCreateSession() {
		return TokenResult{}, apperrors.Forbidden("This account cannot create a session.", nil)
	}

	// Generate tokens.
	sessionID := uuid.NewString()
	accessToken, _, err := u.tokens.GenerateAccessToken(identity.ID, phoneNumber, sessionID)
	if err != nil {
		return TokenResult{}, apperrors.Internal("Access token could not be generated.", err)
	}
	refreshToken, err := u.sessions.GenerateSecureToken(32)
	if err != nil {
		return TokenResult{}, apperrors.Internal("Refresh token could not be generated.", err)
	}

	// Create session (DB + Redis cache).
	if _, err := u.sessions.Create(ctx, sessionID, identity.ID, phoneNumber, refreshToken, input.Metadata); err != nil {
		return TokenResult{}, err
	}

	// Publish session-created event.
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
