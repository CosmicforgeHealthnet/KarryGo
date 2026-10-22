package authusecases

import (
	"context"
	"crypto/hmac"
	"time"

	"github.com/google/uuid"

	authclients "cosmicforge/logistics/services/customer-service/internal/features/auth/clients"
	authmodels "cosmicforge/logistics/services/customer-service/internal/features/auth/models"
	authrepositories "cosmicforge/logistics/services/customer-service/internal/features/auth/repositories"
	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	profilerepositories "cosmicforge/logistics/services/customer-service/internal/features/profile/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

const (
	CustomerRole    = "customer"
	CustomerService = "customer"
)

// AccountStanding is the port through which authentication consults the
// customer's source-owned administrative standing. A login-blocked customer
// cannot complete OTP login or rotate a refresh session.
type AccountStanding interface {
	LoginBlocked(ctx context.Context, customerID string) (bool, error)
}

type AuthService struct {
	standing   AccountStanding
	customers  profilerepositories.CustomerRepository
	sessions   authrepositories.RefreshSessionRepository
	challenges authrepositories.OTPChallengeRepository
	otpSender  authclients.OTPSender

	accessSigner *sharedauth.TokenSigner

	otpSecret          []byte
	refreshTokenSecret []byte
	accessTokenTTL     time.Duration
	refreshTokenTTL    time.Duration
	otpTTL             time.Duration
	otpRateWindow      time.Duration
	otpMaxRequests     int
	otpMaxAttempts     int
	otpDebug           bool
	testLogin          *sharedauth.TestLogin
	now                func() time.Time
}

type Options struct {
	Customers          profilerepositories.CustomerRepository
	Sessions           authrepositories.RefreshSessionRepository
	Challenges         authrepositories.OTPChallengeRepository
	OTPSender          authclients.OTPSender
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
	TestLogin *sharedauth.TestLogin
	// Standing, when set, refuses login and refresh for login-blocked
	// customers. The database guard on customer_sessions backs it up.
	Standing AccountStanding
}

func NewAuthService(opts Options) *AuthService {
	return &AuthService{
		standing:           opts.Standing,
		customers:          opts.Customers,
		sessions:           opts.Sessions,
		challenges:         opts.Challenges,
		otpSender:          opts.OTPSender,
		accessSigner:       sharedauth.NewTokenSigner(opts.AccessTokenSecret),
		otpSecret:          opts.OTPSecret,
		refreshTokenSecret: opts.RefreshTokenSecret,
		accessTokenTTL:     opts.AccessTokenTTL,
		refreshTokenTTL:    opts.RefreshTokenTTL,
		otpTTL:             opts.OTPTTL,
		otpRateWindow:      opts.OTPRateWindow,
		otpMaxRequests:     opts.OTPMaxRequests,
		otpMaxAttempts:     opts.OTPMaxAttempts,
		otpDebug:           opts.OTPDebug,
		testLogin:          opts.TestLogin,
		now:                time.Now,
	}
}

type StartAuthInput struct {
	Phone string
	Email string
}

type StartAuthResult struct {
	ChallengeID string `json:"challenge_id"`
	ExpiresIn   int64  `json:"expires_in"`
	DebugOTP    string `json:"debug_otp,omitempty"`
}

// loginCode is the fixed code for a listed test number (staging only),
// otherwise a fresh random one.
func (s *AuthService) loginCode(identifierType, value string) (string, error) {
	if code, ok := s.testLogin.CodeFor(identifierType, value); ok {
		return code, nil
	}
	return sharedauth.GenerateNumericOTP(sharedauth.DefaultOTPLength)
}

func (s *AuthService) StartAuth(ctx context.Context, input StartAuthInput) (StartAuthResult, error) {
	identifier, err := authmodels.NormalizeAuthIdentifier(input.Phone, input.Email)
	if err != nil {
		return StartAuthResult{}, err
	}

	otp, err := s.loginCode(identifier.Type, identifier.Value)
	if err != nil {
		return StartAuthResult{}, apperrors.Internal("OTP could not be generated.", err)
	}

	challengeID := uuid.NewString()
	challenge := authmodels.OTPChallenge{
		ID:              challengeID,
		IdentifierType:  identifier.Type,
		IdentifierValue: identifier.Value,
		OTPHash:         sharedauth.HashOTP(s.otpSecret, challengeID, identifier.Key(), otp),
		ExpiresAt:       s.now().Add(s.otpTTL),
	}

	if err := s.challenges.Save(ctx, challenge, s.otpTTL, s.otpRateWindow, s.otpMaxRequests); err != nil {
		return StartAuthResult{}, err
	}
	if s.otpSender != nil {
		destination := authclients.OTPDestination{Type: identifier.Type, Value: identifier.Value}
		if err := s.otpSender.SendOTP(ctx, destination, otp); err != nil {
			return StartAuthResult{}, apperrors.Unavailable("OTP delivery is temporarily unavailable.", err)
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

type VerifyAuthInput struct {
	Phone       string
	Email       string
	OTP         string
	ChallengeID string
	DeviceID    *string
	UserAgent   string
	IPAddress   string
	// ReferralLinkCode and ReferralCode are an optional referral code from a
	// shared link or typed by the user. They matter only when this
	// verification creates the account; a returning login ignores them.
	ReferralLinkCode string
	ReferralCode     string
}

type TokenResult struct {
	AccessToken  string                       `json:"access_token"`
	RefreshToken string                       `json:"refresh_token"`
	ExpiresIn    int64                        `json:"expires_in"`
	Customer     profilemodels.PublicCustomer `json:"customer"`
}

func (s *AuthService) VerifyAuth(ctx context.Context, input VerifyAuthInput) (TokenResult, error) {
	identifier, err := authmodels.NormalizeAuthIdentifier(input.Phone, input.Email)
	if err != nil {
		return TokenResult{}, err
	}
	if input.OTP == "" {
		return TokenResult{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "otp", Message: "Verification code is required."},
		})
	}

	challenge, ok, err := s.challenges.Get(ctx, identifier)
	if err != nil {
		return TokenResult{}, err
	}
	if !ok {
		return TokenResult{}, apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}

	if err := authmodels.VerifyOTPChallenge(s.otpSecret, challenge, input.ChallengeID, input.OTP, s.otpMaxAttempts, s.now()); err != nil {
		_ = s.challenges.RecordFailedAttempt(ctx, challenge, time.Until(challenge.ExpiresAt))
		return TokenResult{}, err
	}

	if err := s.challenges.Delete(ctx, identifier); err != nil {
		return TokenResult{}, err
	}

	signup := profilemodels.Signup{LinkCode: input.ReferralLinkCode, TypedCode: input.ReferralCode}
	if input.DeviceID != nil {
		signup.DeviceID = *input.DeviceID
	}
	var customer profilemodels.Customer
	switch identifier.Type {
	case authmodels.IdentifierTypePhone:
		customer, err = s.customers.UpsertByPhone(ctx, identifier.Value, signup)
	case authmodels.IdentifierTypeEmail:
		customer, err = s.customers.UpsertByEmail(ctx, identifier.Value, signup)
	default:
		err = apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "identifier", Message: "Phone number or email address is required."},
		})
	}
	if err != nil {
		return TokenResult{}, err
	}
	if err := s.ensureLoginAllowed(ctx, customer.ID); err != nil {
		return TokenResult{}, err
	}

	return s.createSessionTokens(ctx, customer, input.DeviceID, input.UserAgent, input.IPAddress)
}

// ensureLoginAllowed refuses a customer whose administrative standing blocks
// login. Any failure to read the standing fails closed.
func (s *AuthService) ensureLoginAllowed(ctx context.Context, customerID string) error {
	if s.standing == nil {
		return nil
	}
	blocked, err := s.standing.LoginBlocked(ctx, customerID)
	if err != nil {
		return apperrors.Unavailable("Sign-in is temporarily unavailable.", err)
	}
	if blocked {
		return authrepositories.AccountRestrictedError()
	}
	return nil
}

type RefreshInput struct {
	RefreshToken string
	DeviceID     *string
	UserAgent    string
	IPAddress    string
}

func (s *AuthService) Refresh(ctx context.Context, input RefreshInput) (TokenResult, error) {
	sessionID, err := authmodels.ParseRefreshTokenSessionID(input.RefreshToken)
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

	hash := sharedauth.HashRefreshToken(s.refreshTokenSecret, input.RefreshToken)
	if !hmac.Equal([]byte(hash), []byte(existing.RefreshTokenHash)) {
		return TokenResult{}, apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}

	customer, err := s.customers.GetByID(ctx, existing.CustomerID)
	if err != nil {
		return TokenResult{}, err
	}
	if err := s.ensureLoginAllowed(ctx, customer.ID); err != nil {
		_ = s.sessions.Revoke(ctx, existing.ID)
		return TokenResult{}, err
	}
	if err := s.sessions.Revoke(ctx, existing.ID); err != nil {
		return TokenResult{}, err
	}

	return s.createSessionTokens(ctx, customer, input.DeviceID, input.UserAgent, input.IPAddress)
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	sessionID, err := authmodels.ParseRefreshTokenSessionID(refreshToken)
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
	hash := sharedauth.HashRefreshToken(s.refreshTokenSecret, refreshToken)
	if !hmac.Equal([]byte(hash), []byte(existing.RefreshTokenHash)) {
		return apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}

	return s.sessions.Revoke(ctx, sessionID)
}

func (s *AuthService) Me(ctx context.Context, customerID string) (profilemodels.PublicCustomer, error) {
	foundCustomer, err := s.customers.GetByID(ctx, customerID)
	if err != nil {
		return profilemodels.PublicCustomer{}, err
	}

	return foundCustomer.Public(), nil
}

func (s *AuthService) AccessSigner() *sharedauth.TokenSigner {
	return s.accessSigner
}

func (s *AuthService) createSessionTokens(ctx context.Context, foundCustomer profilemodels.Customer, deviceID *string, userAgent string, ipAddress string) (TokenResult, error) {
	sessionID := uuid.NewString()
	refreshSecret, err := sharedauth.GenerateOpaqueToken(32)
	if err != nil {
		return TokenResult{}, apperrors.Internal("Refresh token could not be generated.", err)
	}
	refreshToken := sessionID + "." + refreshSecret

	expiresAt := s.now().Add(s.refreshTokenTTL)
	session := authmodels.RefreshSession{
		ID:               sessionID,
		CustomerID:       foundCustomer.ID,
		RefreshTokenHash: sharedauth.HashRefreshToken(s.refreshTokenSecret, refreshToken),
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
		Subject:   foundCustomer.ID,
		Role:      CustomerRole,
		Service:   CustomerService,
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
		Customer:     foundCustomer.Public(),
	}, nil
}
