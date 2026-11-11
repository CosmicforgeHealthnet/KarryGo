// Package usecases implements the corporate passwordless authentication
// flows: start, verify, resend, refresh, logout, organization selection and
// step-up.
//
// There is no password anywhere in this package. A caller proves control of
// their registered email by entering a code. Phone association is not proof
// of phone ownership.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	ValidateForAuthentication(ctx context.Context, tx pgx.Tx, token string, pair contact.Pair) error
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
	Email             string
	Identifier        string
	ResendChallengeID string
	Phone             string
	InvitationToken   string
	Purpose           string
	IPHash            string
	UserAgentHash     string
	RequestID         string
	CorrelationID     string
	// Step-up bindings; only set for PurposeStepUp.
	IdentityID     string
	SessionID      string
	OrganizationID string
	ActionClass    string
}

// StartResult describes an accepted email challenge with absolute deadlines.
type StartResult struct {
	ChallengeID        string
	DeliveryChannel    string
	MaskedDestination  string
	ExpiresInSeconds   int
	ResendAfterSeconds int
	// Absolute server deadlines keep the mobile browser countdown accurate
	// across refreshes and background-tab timer throttling.
	ExpiresAt         time.Time
	ResendAvailableAt time.Time
}

// Start is the signup-only compatibility entry point. The public handler never
// accepts a purpose; only dedicated handlers and authenticated step-up set it.
func (s *Service) Start(ctx context.Context, input StartInput) (StartResult, error) {
	if input.Purpose == "" {
		input.Purpose = models.PurposeSignUp
	}
	if input.InvitationToken != "" && input.Purpose == models.PurposeSignUp {
		input.Purpose = models.PurposeStaffInvitation
	}
	pair, err := contact.NormalizePair(input.Email, input.Phone, s.opts.DefaultCountryCode)
	if err != nil {
		fields := []apperrors.FieldViolation{}
		for _, v := range contact.Violations(input.Email, input.Phone, s.opts.DefaultCountryCode) {
			fields = append(fields, apperrors.FieldViolation{Field: v.Field, Message: v.Message})
		}
		return StartResult{}, apperrors.Validation("Enter a valid email address and phone number.", fields)
	}
	emailHash := contact.Hash(s.opts.ContactSecret, pair.EmailNormalized)
	phoneHash := contact.Hash(s.opts.ContactSecret, pair.PhoneE164)
	if err := s.enforceLimits(ctx, ratelimit.ScopeChallengeCreate, map[string]string{"email": emailHash, "phone": phoneHash, "ip": input.IPHash}, ratelimit.Rule{Limit: s.opts.OTPMaxPerHour, Window: time.Hour}); err != nil {
		return StartResult{}, err
	}
	var challenge *models.Challenge
	var code string
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		// All starts and verifications lock identifiers in this order, including
		// requests from different service replicas. No raw identifiers in locks.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0)), pg_advisory_xact_lock(hashtextextended($2,0))`, "fleet-email:"+emailHash, "fleet-phone:"+phoneHash); err != nil {
			return err
		}
		if input.ResendChallengeID != "" {
			old, err := s.opts.Challenges.LockForVerification(ctx, tx, input.ResendChallengeID)
			if err != nil {
				return errorcodes.Unauthorized(errorcodes.OTPChallengeNotFound, "This verification request is no longer valid.")
			}
			if old.Purpose != input.Purpose || old.EmailHash != emailHash || old.PhoneHash != phoneHash {
				return errorcodes.Unauthorized(errorcodes.OTPInvalid, "Start again.")
			}
			if old.Status != models.ChallengeStatusPending && old.Status != models.ChallengeStatusExpired {
				return challengeVerificationError(old, s.opts.Now())
			}
			if s.opts.Now().After(old.ExpiresAt.Add(30 * time.Minute)) {
				return errorcodes.Unauthorized(errorcodes.OTPChallengeExpired, "This request has expired. Start again.")
			}
			if s.opts.Now().Before(old.ResendAvailableAt) {
				return errorcodes.TooManyRequests(errorcodes.OTPResendTooSoon, "Please wait before requesting another code.")
			}
		}
		identityID, err := s.startEligibility(ctx, tx, pair, input)
		if err != nil {
			return err
		}
		latest, err := s.opts.Challenges.Latest(ctx, tx, emailHash, phoneHash, input.Purpose, input.SessionID, input.ActionClass)
		if err != nil && !errors.Is(err, authrepos.ErrNotFound) {
			return err
		}
		if latest != nil && s.opts.Now().Before(latest.ResendAvailableAt) {
			if input.ResendChallengeID != "" {
				return errorcodes.TooManyRequests(errorcodes.OTPResendTooSoon, "Please wait before requesting another code.")
			}
			if latest.Status == models.ChallengeStatusPending && latest.DeliveryStatus == models.DeliveryStatusAccepted {
				challenge = latest
				return nil
			}
			return errorcodes.TooManyRequests(errorcodes.OTPResendTooSoon, "Please wait before requesting another code.")
		}
		challenge, code, err = s.buildChallenge(pair, emailHash, phoneHash, input.Purpose, identityID, input)
		if err != nil {
			return err
		}
		return s.opts.Challenges.Create(ctx, tx, challenge)
	})
	if err != nil {
		code := apperrors.From(err).Code
		if code == errorcodes.EmailAlreadyRegistered || code == errorcodes.PhoneAlreadyAssociated || code == errorcodes.IdentityPairConflict {
			s.recordSecurityEvent(ctx, models.SecurityEvent{EventType: models.SecurityEventIdentityPairConflict, Severity: models.SeverityWarn, EmailHash: &emailHash, PhoneHash: &phoneHash, IPHash: optional(input.IPHash), RequestID: input.RequestID, CorrelationID: input.CorrelationID})
		}
		return StartResult{}, err
	}
	if code != "" {
		if err := s.deliver(ctx, challenge, code, input.Purpose); err != nil {
			return StartResult{}, err
		}
	}
	s.opts.Metrics.IncCounter("fleet_partner_auth_start_requests_total", map[string]string{"purpose": input.Purpose}, 1)
	return s.challengeResult(challenge), nil
}

func (s *Service) SignIn(ctx context.Context, input StartInput) (StartResult, error) {
	raw := strings.TrimSpace(input.Identifier)
	var identity *models.Identity
	var err error
	if err = s.enforceLimits(ctx, "sign-in-lookup", map[string]string{"identifier": contact.Hash(s.opts.ContactSecret, strings.ToLower(raw)), "ip": input.IPHash}, ratelimit.Rule{Limit: s.opts.OTPMaxPerHour, Window: time.Hour}); err != nil {
		return StartResult{}, err
	}
	if strings.Contains(raw, "@") {
		value, _, e := contact.NormalizeEmail(raw)
		if e != nil {
			return StartResult{}, apperrors.Validation("Enter a valid email address.", []apperrors.FieldViolation{{Field: "identifier", Message: "Enter a valid email address."}})
		}
		identity, err = s.opts.Identities.FindByEmail(ctx, s.opts.Pool, value)
	} else {
		value, _, e := contact.NormalizePhone(raw, s.opts.DefaultCountryCode)
		if e != nil {
			return StartResult{}, apperrors.Validation("Enter a valid phone number.", []apperrors.FieldViolation{{Field: "identifier", Message: "Enter a valid phone number with its country code."}})
		}
		identity, err = s.opts.Identities.FindByPhone(ctx, s.opts.Pool, value)
	}
	if err != nil && !errors.Is(err, authrepos.ErrNotFound) {
		return StartResult{}, err
	}
	if identity == nil || !identity.Usable(s.opts.Now()) || identity.EmailVerifiedAt == nil || identity.Status != models.IdentityStatusActive {
		if identity != nil && !identity.Usable(s.opts.Now()) {
			s.recordSecurityEvent(ctx, models.SecurityEvent{EventType: models.SecurityEventSuspendedAttempt, Severity: models.SeverityWarn, IdentityID: &identity.ID, RequestID: input.RequestID, CorrelationID: input.CorrelationID})
		}
		return StartResult{}, signInUnavailable()
	}
	// The client never supplies the delivery email in this contract.
	input.Email = identity.EmailNormalized
	input.Phone = identity.PhoneE164
	input.IdentityID = identity.ID
	input.Purpose = models.PurposeSignIn
	return s.Start(ctx, input)
}

func signInUnavailable() error {
	return errorcodes.Unauthorized(errorcodes.SignInUnavailable, "We could not start sign-in with these details. Check them, create an account or contact support.")
}

func (s *Service) startEligibility(ctx context.Context, tx pgx.Tx, pair contact.Pair, input StartInput) (*string, error) {
	if input.Purpose == models.PurposeSignUp {
		if err := s.signupEligibility(ctx, tx, pair); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if input.Purpose != models.PurposeSignIn && input.Purpose != models.PurposeStaffInvitation && input.Purpose != models.PurposeStepUp {
		return nil, errorcodes.BadRequest(errorcodes.OTPInvalid, "Unsupported verification purpose. Start again.")
	}
	if input.Purpose == models.PurposeStaffInvitation {
		if s.opts.Invitations == nil {
			return nil, errorcodes.Unavailable(errorcodes.OTPDeliveryUnavailable, "Invitation authentication is temporarily unavailable.")
		}
		if err := s.opts.Invitations.ValidateForAuthentication(ctx, tx, input.InvitationToken, pair); err != nil {
			return nil, err
		}
	}
	identity, err := s.opts.Identities.ResolvePair(ctx, tx, pair.EmailNormalized, pair.PhoneE164)
	if input.Purpose == models.PurposeStaffInvitation && errors.Is(err, authrepos.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		if errors.Is(err, authrepos.ErrNotFound) || errors.Is(err, authrepos.ErrIdentityPairConflict) {
			return nil, signInUnavailable()
		}
		return nil, err
	}
	if !identity.Usable(s.opts.Now()) {
		s.recordSecurityEvent(ctx, models.SecurityEvent{EventType: models.SecurityEventSuspendedAttempt, Severity: models.SeverityWarn, IdentityID: &identity.ID, RequestID: input.RequestID, CorrelationID: input.CorrelationID})
		return nil, signInUnavailable()
	}
	if input.Purpose == models.PurposeSignIn && (identity.ID != input.IdentityID || identity.EmailVerifiedAt == nil || identity.Status != models.IdentityStatusActive) {
		return nil, signInUnavailable()
	}
	if input.Purpose == models.PurposeStepUp && (identity.ID != input.IdentityID || input.SessionID == "") {
		return nil, signInUnavailable()
	}
	return &identity.ID, nil
}

func (s *Service) signupEligibility(ctx context.Context, q database.Querier, pair contact.Pair) error {
	email, err := s.opts.Identities.FindByEmail(ctx, q, pair.EmailNormalized)
	if err != nil && !errors.Is(err, authrepos.ErrNotFound) {
		return err
	}
	if email != nil {
		return errorcodes.Conflict(errorcodes.EmailAlreadyRegistered, "This email is already registered with Karryam Fleet. Please sign in or use another email address.")
	}
	phone, err := s.opts.Identities.FindByPhone(ctx, q, pair.PhoneE164)
	if err != nil && !errors.Is(err, authrepos.ErrNotFound) {
		return err
	}
	if phone != nil {
		return errorcodes.Conflict(errorcodes.PhoneAlreadyAssociated, "This phone number is already associated with a Karryam Fleet account. Use another number or contact support.")
	}
	return nil
}

func (s *Service) challengeResult(c *models.Challenge) StartResult {
	return StartResult{ChallengeID: c.ID, DeliveryChannel: "EMAIL", MaskedDestination: contact.MaskEmail(c.EmailNormalized), ExpiresInSeconds: max(0, int(c.ExpiresAt.Sub(s.opts.Now()).Seconds())), ResendAfterSeconds: max(0, int(c.ResendAvailableAt.Sub(s.opts.Now()).Seconds())), ExpiresAt: c.ExpiresAt, ResendAvailableAt: c.ResendAvailableAt}
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
		CodeDigest:        auth.HashOTP(s.opts.OTPSecret, challengeID, purpose+":"+pair.PhoneE164, code),
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

	if err := s.opts.Challenges.RecordDelivery(ctx, challenge.ID, models.DeliveryStatusAccepted, reference); err != nil {
		return errorcodes.Unavailable(errorcodes.OTPDeliveryUnavailable, "We could not confirm your verification request. Please try again shortly.")
	}
	return nil
}

func (s *Service) enforceLimits(ctx context.Context, scope string, keys map[string]string, rule ratelimit.Rule) error {
	for name, key := range keys {
		if key == "" {
			continue
		}
		allowed, err := s.opts.Limiter.Allow(ctx, scope+":"+name, key, rule)
		if err != nil {
			return errorcodes.Unavailable(errorcodes.OTPDeliveryUnavailable, "Authentication is temporarily unavailable. Please try again shortly.")
		}
		if !allowed {
			s.recordSecurityEvent(ctx, models.SecurityEvent{EventType: models.SecurityEventRateLimited, Severity: models.SeverityWarn})
			s.opts.Metrics.IncCounter("fleet_partner_otp_rate_limited_total", nil, 1)
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
	case models.PurposeSignUp:
		return "create your Karryam Fleet account"
	case models.PurposeSignIn:
		return "sign in to Karryam Fleet"
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
