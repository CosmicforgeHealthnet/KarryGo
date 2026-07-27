// Package usecases implements the corporate passwordless authentication
// flows: start, verify, resend, refresh, logout, organization selection and
// step-up.
//
// There is no password anywhere in this package. A caller proves control of
// an email + phone pair by entering a code that was emailed to them.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/contact"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
)

// MembershipReader exposes the membership lookups authentication needs. It is
// an interface so the auth feature never imports the staff feature's
// concrete repository.
type MembershipReader interface {
	// ListActiveForIdentity returns the identity's live organization
	// memberships, newest first.
	ListActiveForIdentity(ctx context.Context, identityID string) ([]MembershipSummary, error)
	// GetForIdentity loads one membership and confirms it belongs to the
	// identity. It returns ErrMembershipNotFound when it does not.
	GetForIdentity(ctx context.Context, identityID string, membershipID string) (MembershipSummary, error)
}

// MembershipSummary is the authentication-facing view of a membership.
type MembershipSummary struct {
	ID                 string
	OrganizationID     string
	OrganizationName   string
	OrganizationStatus string
	Status             string
	IsOwner            bool
	RoleVersion        int64
	PermittedBranchIDs []string
}

// ErrMembershipNotFound means the membership does not exist for this identity.
var ErrMembershipNotFound = errors.New("auth: membership not found for identity")

// InvitationAcceptor completes a staff invitation during OTP verification. It
// is an interface for the same layering reason as MembershipReader.
type InvitationAcceptor interface {
	// AcceptByToken consumes the invitation and returns the activated
	// membership. It must be called inside the caller's transaction.
	AcceptByToken(ctx context.Context, tx pgx.Tx, token string, identityID string, pair contact.Pair) (MembershipSummary, error)
}

// Options configure the authentication service.
type Options struct {
	Pool           *pgxpool.Pool
	Identities     *authrepos.IdentityRepository
	Challenges     *authrepos.ChallengeRepository
	Sessions       *authrepos.SessionRepository
	SecurityEvents *authrepos.SecurityEventRepository
	Sender         authclients.Sender
	Limiter        ratelimit.Limiter
	Metrics        *observability.Metrics
	Memberships    MembershipReader
	Invitations    InvitationAcceptor

	OTPSecret     []byte
	SessionSecret []byte
	ContactSecret []byte
	AccessSigner  *auth.TokenSigner

	OTPLength          int
	OTPExpiry          time.Duration
	OTPMaxAttempts     int
	OTPResendCooldown  time.Duration
	OTPMaxPerHour      int
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	StepUpTTL          time.Duration
	DefaultCountryCode string

	Now func() time.Time
}

// Service implements corporate authentication.
type Service struct {
	opts Options
}

// NewService validates options and builds the service.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.OTPLength <= 0 {
		opts.OTPLength = auth.DefaultOTPLength
	}
	if opts.OTPExpiry <= 0 {
		opts.OTPExpiry = 10 * time.Minute
	}
	if opts.OTPMaxAttempts <= 0 {
		opts.OTPMaxAttempts = 5
	}
	if opts.OTPResendCooldown <= 0 {
		opts.OTPResendCooldown = 60 * time.Second
	}
	if opts.OTPMaxPerHour <= 0 {
		opts.OTPMaxPerHour = 10
	}
	if opts.Limiter == nil {
		opts.Limiter = ratelimit.NewMemoryLimiter()
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}
	return &Service{opts: opts}
}

// StartInput is the request to begin signup or login.
type StartInput struct {
	Email           string
	Phone           string
	InvitationToken string
	Purpose         string
	IPHash          string
	UserAgentHash   string
	RequestID       string
	CorrelationID   string
	// Step-up bindings; only set for PurposeStepUp.
	IdentityID     string
	SessionID      string
	OrganizationID string
	ActionClass    string
}

// StartResult is the deliberately generic response to a start request.
// It reveals nothing about whether the account exists.
type StartResult struct {
	ChallengeID        string
	DeliveryChannel    string
	MaskedDestination  string
	ExpiresInSeconds   int
	ResendAfterSeconds int
}

// Start begins a passwordless authentication challenge.
//
// Both email and phone are mandatory. The response is identical whether or
// not an account exists, so it cannot be used to enumerate accounts. An
// identity is never created here — only after the code is verified.
func (s *Service) Start(ctx context.Context, input StartInput) (StartResult, error) {
	pair, err := contact.NormalizePair(input.Email, input.Phone, s.opts.DefaultCountryCode)
	if err != nil {
		return StartResult{}, apperrors.Validation("Enter a valid email address and phone number.", []apperrors.FieldViolation{
			{Field: "email", Message: "A valid email address and phone number are both required."},
		})
	}

	purpose := input.Purpose
	if purpose == "" {
		purpose = models.PurposeSignUpOrLogin
	}
	if input.InvitationToken != "" && purpose == models.PurposeSignUpOrLogin {
		purpose = models.PurposeStaffInvitation
	}

	emailHash := contact.Hash(s.opts.ContactSecret, pair.EmailNormalized)
	phoneHash := contact.Hash(s.opts.ContactSecret, pair.PhoneE164)

	if err := s.enforceLimits(ctx, ratelimit.ScopeChallengeCreate, map[string]string{
		"email": emailHash, "phone": phoneHash, "ip": input.IPHash,
	}, ratelimit.Rule{Limit: s.opts.OTPMaxPerHour, Window: time.Hour}); err != nil {
		s.recordSecurityEvent(ctx, models.SecurityEvent{
			EventType: models.SecurityEventRateLimited, Severity: models.SeverityWarn,
			EmailHash: &emailHash, PhoneHash: &phoneHash, IPHash: optional(input.IPHash),
			RequestID: input.RequestID, CorrelationID: input.CorrelationID,
		})
		s.opts.Metrics.IncCounter("fleet_partner_otp_rate_limited_total", nil, 1)
		return StartResult{}, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_auth_start_requests_total", map[string]string{"purpose": purpose}, 1)

	// Resolve the pair. A conflict is recorded as a security event but the
	// caller still receives the generic response.
	var identityID *string
	existing, resolveErr := s.opts.Identities.ResolvePair(ctx, s.opts.Pool, pair.EmailNormalized, pair.PhoneE164)
	switch {
	case resolveErr == nil:
		if !existing.Usable(s.opts.Now()) {
			s.recordSecurityEvent(ctx, models.SecurityEvent{
				EventType: models.SecurityEventSuspendedAttempt, Severity: models.SeverityWarn,
				IdentityID: &existing.ID, EmailHash: &emailHash, PhoneHash: &phoneHash,
				RequestID: input.RequestID, CorrelationID: input.CorrelationID,
			})
			// Generic response: do not disclose suspension.
			return s.genericResult(pair), nil
		}
		identityID = &existing.ID
	case errors.Is(resolveErr, authrepos.ErrIdentityPairConflict):
		s.recordSecurityEvent(ctx, models.SecurityEvent{
			EventType: models.SecurityEventIdentityPairConflict, Severity: models.SeverityCritical,
			EmailHash: &emailHash, PhoneHash: &phoneHash, IPHash: optional(input.IPHash),
			RequestID: input.RequestID, CorrelationID: input.CorrelationID,
		})
		// Generic response: a conflict must not reveal which half matched.
		return s.genericResult(pair), nil
	case errors.Is(resolveErr, authrepos.ErrNotFound):
		identityID = nil // new signup; identity is created after verification
	default:
		return StartResult{}, resolveErr
	}

	// Step-up binds to the caller's existing identity and session.
	if purpose == models.PurposeStepUp && input.IdentityID != "" {
		identityID = &input.IdentityID
	}

	challenge, code, err := s.buildChallenge(pair, emailHash, phoneHash, purpose, identityID, input)
	if err != nil {
		return StartResult{}, err
	}

	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Challenges.Create(ctx, tx, challenge)
	}); err != nil {
		return StartResult{}, err
	}

	if err := s.deliver(ctx, challenge, code, purpose); err != nil {
		return StartResult{}, err
	}

	return StartResult{
		ChallengeID:        challenge.ID,
		DeliveryChannel:    "EMAIL",
		MaskedDestination:  contact.MaskEmail(pair.EmailNormalized),
		ExpiresInSeconds:   int(s.opts.OTPExpiry.Seconds()),
		ResendAfterSeconds: int(s.opts.OTPResendCooldown.Seconds()),
	}, nil
}

// genericResult returns an indistinguishable response for cases where no
// challenge was actually created (suspended identity, pair conflict). It
// carries a random challenge ID that will simply never verify.
func (s *Service) genericResult(pair contact.Pair) StartResult {
	return StartResult{
		ChallengeID:        ids.New("ach"),
		DeliveryChannel:    "EMAIL",
		MaskedDestination:  contact.MaskEmail(pair.EmailNormalized),
		ExpiresInSeconds:   int(s.opts.OTPExpiry.Seconds()),
		ResendAfterSeconds: int(s.opts.OTPResendCooldown.Seconds()),
	}
}

func (s *Service) buildChallenge(pair contact.Pair, emailHash string, phoneHash string, purpose string, identityID *string, input StartInput) (*models.Challenge, string, error) {
	code, err := auth.GenerateNumericOTP(s.opts.OTPLength)
	if err != nil {
		return nil, "", fmt.Errorf("auth: generate otp: %w", err)
	}
	now := s.opts.Now()
	challengeID := ids.New("ach")

	challenge := &models.Challenge{
		ID:              challengeID,
		IdentityID:      identityID,
		EmailNormalized: pair.EmailNormalized,
		EmailHash:       emailHash,
		PhoneE164:       pair.PhoneE164,
		PhoneHash:       phoneHash,
		Purpose:         purpose,
		Status:          models.ChallengeStatusPending,
		// Keyed digest binds the code to this challenge and this phone.
		CodeDigest:        auth.HashOTP(s.opts.OTPSecret, challengeID, pair.PhoneE164, code),
		ExpiresAt:         now.Add(s.opts.OTPExpiry),
		ResendAvailableAt: now.Add(s.opts.OTPResendCooldown),
		MaximumAttempts:   s.opts.OTPMaxAttempts,
		RequestID:         input.RequestID,
		CorrelationID:     input.CorrelationID,
		RequestedIPHash:   optional(input.IPHash),
		UserAgentHash:     optional(input.UserAgentHash),
		SessionID:         optional(input.SessionID),
		OrganizationID:    optional(input.OrganizationID),
		ActionClass:       optional(input.ActionClass),
		InvitationID:      optional(input.InvitationToken),
	}
	return challenge, code, nil
}

// deliver hands the code to notification-service. A delivery failure is a
// hard 503: the API never claims an OTP was sent when it was not.
func (s *Service) deliver(ctx context.Context, challenge *models.Challenge, code string, purpose string) error {
	s.opts.Metrics.IncCounter("fleet_partner_otp_delivery_attempts_total", nil, 1)

	reference, err := s.opts.Sender.SendOTP(ctx, authclients.OTPMessage{
		Email:          challenge.EmailNormalized,
		Phone:          challenge.PhoneE164,
		Code:           code,
		Purpose:        purpose,
		PurposeText:    purposeText(purpose),
		ExpiresIn:      s.opts.OTPExpiry,
		IdempotencyKey: "otp:" + challenge.ID,
		RequestID:      challenge.RequestID,
		CorrelationID:  challenge.CorrelationID,
	})
	if err != nil {
		_ = s.opts.Challenges.RecordDelivery(ctx, challenge.ID, models.DeliveryStatusUnavailable, "")
		s.opts.Metrics.IncCounter("fleet_partner_otp_delivery_failures_total", nil, 1)
		emailHash := challenge.EmailHash
		s.recordSecurityEvent(ctx, models.SecurityEvent{
			EventType: models.SecurityEventOTPDeliveryFailed, Severity: models.SeverityWarn,
			EmailHash: &emailHash, RequestID: challenge.RequestID, CorrelationID: challenge.CorrelationID,
		})
		return errorcodes.Unavailable(errorcodes.OTPDeliveryUnavailable,
			"We could not send your verification code right now. Please try again shortly.")
	}

	_ = s.opts.Challenges.RecordDelivery(ctx, challenge.ID, models.DeliveryStatusAccepted, reference)
	return nil
}

func (s *Service) enforceLimits(ctx context.Context, scope string, keys map[string]string, rule ratelimit.Rule) error {
	for name, key := range keys {
		if key == "" {
			continue
		}
		allowed, err := s.opts.Limiter.Allow(ctx, scope+":"+name, key, rule)
		if err != nil {
			continue // infrastructure failure must not lock everyone out
		}
		if !allowed {
			return errorcodes.TooManyRequests(errorcodes.AuthenticationRateLimited,
				"Too many attempts. Please wait before trying again.")
		}
	}
	return nil
}

func (s *Service) recordSecurityEvent(ctx context.Context, event models.SecurityEvent) {
	if s.opts.SecurityEvents == nil {
		return
	}
	_ = s.opts.SecurityEvents.Record(ctx, s.opts.Pool, event)
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func purposeText(purpose string) string {
	switch purpose {
	case models.PurposeStaffInvitation:
		return "accept your organization invitation"
	case models.PurposeStepUp:
		return "confirm a sensitive action"
	case models.PurposeOrganizationSelection:
		return "select your organization"
	case models.PurposeContactChange:
		return "confirm your contact change"
	default:
		return "sign in to KarryGo"
	}
}
