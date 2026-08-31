package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/contact"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
)

// Next-step values returned after verification.
const (
	NextStepOnboardingRequired    = "ONBOARDING_REQUIRED"
	NextStepOrganizationSelection = "ORGANIZATION_SELECTION_REQUIRED"
	NextStepReady                 = "READY"
	NextStepStepUpGranted         = "STEP_UP_GRANTED"
)

// VerifyInput is a code submission.
type VerifyInput struct {
	ChallengeID   string
	Code          string
	IPHash        string
	UserAgentHash string
	DeviceLabel   string
	RequestID     string
	CorrelationID string
}

// TokenPair is an issued access + refresh token pair.
type TokenPair struct {
	AccessToken      string
	RefreshToken     string `json:"-"`
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
	SessionID        string
}

// VerifyResult is the outcome of a successful verification.
type VerifyResult struct {
	NextStep      string
	Tokens        *TokenPair
	IdentityID    string
	Organizations []MembershipSummary
	// StepUpReference is set when a STEP_UP challenge was verified.
	StepUpReference string
	StepUpExpiresAt time.Time
}

// Verify validates a submitted code and, on success, issues a session.
//
// The whole check-and-consume runs in one transaction with the challenge row
// locked FOR UPDATE, so concurrent submissions of the same challenge produce
// exactly one winner.
func (s *Service) Verify(ctx context.Context, input VerifyInput) (VerifyResult, error) {
	if input.ChallengeID == "" || input.Code == "" {
		return VerifyResult{}, errorcodes.BadRequest(errorcodes.OTPInvalid, "Enter the verification code that was emailed to you.")
	}

	if err := s.enforceLimits(ctx, ratelimit.ScopeVerify, map[string]string{
		"challenge": input.ChallengeID, "ip": input.IPHash,
	}, ratelimit.Rule{Limit: s.opts.OTPMaxAttempts * 3, Window: 10 * time.Minute}); err != nil {
		return VerifyResult{}, err
	}

	var result VerifyResult
	// failedChallenge is set when the code was wrong; the attempt is recorded
	// after the transaction rolls back.
	var failedChallenge *models.Challenge

	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		challenge, err := s.opts.Challenges.LockForVerification(ctx, tx, input.ChallengeID)
		if err != nil {
			if errors.Is(err, authrepos.ErrNotFound) {
				return errorcodes.Unauthorized(errorcodes.OTPChallengeNotFound, "This verification request is no longer valid.")
			}
			return err
		}

		now := s.opts.Now()
		switch {
		case challenge.Status == models.ChallengeStatusConsumed:
			return errorcodes.Unauthorized(errorcodes.OTPChallengeConsumed, "This verification code has already been used.")
		case challenge.Status == models.ChallengeStatusLocked || challenge.FailedAttempts >= challenge.MaximumAttempts:
			return errorcodes.Unauthorized(errorcodes.OTPAttemptLimitExceeded, "Too many incorrect attempts. Request a new code.")
		case challenge.Status != models.ChallengeStatusPending:
			return errorcodes.Unauthorized(errorcodes.OTPChallengeExpired, "This verification request is no longer valid.")
		case !now.Before(challenge.ExpiresAt):
			return errorcodes.Unauthorized(errorcodes.OTPChallengeExpired, "This verification code has expired. Request a new one.")
		}

		// Constant-time comparison of the keyed digest.
		if !auth.VerifyOTP(s.opts.OTPSecret, challenge.ID, challenge.PhoneE164, input.Code, challenge.CodeDigest) {
			// Returning an error from this function rolls the transaction
			// back, so the attempt counter must be persisted OUTSIDE it —
			// otherwise a wrong code would never consume an attempt and the
			// limit could never be reached.
			failedChallenge = challenge
			return errorcodes.Unauthorized(errorcodes.OTPInvalid, "That code is not correct.")
		}

		// Consume atomically; a concurrent winner makes this fail.
		if err := s.opts.Challenges.Consume(ctx, tx, challenge.ID); err != nil {
			return errorcodes.Unauthorized(errorcodes.OTPChallengeConsumed, "This verification code has already been used.")
		}
		s.opts.Metrics.IncCounter("fleet_partner_otp_verify_success_total", nil, 1)

		result, err = s.completeVerification(ctx, tx, challenge, input)
		return err
	})
	if failedChallenge != nil {
		return VerifyResult{}, s.recordFailedAttempt(ctx, failedChallenge, input)
	}
	if err != nil {
		return VerifyResult{}, err
	}
	return result, nil
}

// recordFailedAttempt durably consumes one attempt after the verification
// transaction rolled back, and reports whether the limit is now exhausted.
func (s *Service) recordFailedAttempt(ctx context.Context, challenge *models.Challenge, input VerifyInput) error {
	s.opts.Metrics.IncCounter("fleet_partner_otp_verify_failure_total", nil, 1)

	if err := s.opts.Challenges.RecordFailedAttempt(ctx, s.opts.Pool, challenge.ID); err != nil {
		return err
	}

	if challenge.FailedAttempts+1 >= challenge.MaximumAttempts {
		emailHash := challenge.EmailHash
		_ = s.opts.SecurityEvents.Record(ctx, s.opts.Pool, models.SecurityEvent{
			EventType: models.SecurityEventOTPAttemptExceeded, Severity: models.SeverityWarn,
			IdentityID: challenge.IdentityID, EmailHash: &emailHash,
			RequestID: input.RequestID, CorrelationID: input.CorrelationID,
		})
		return errorcodes.Unauthorized(errorcodes.OTPAttemptLimitExceeded,
			"Too many incorrect attempts. Request a new code.")
	}
	return errorcodes.Unauthorized(errorcodes.OTPInvalid, "That code is not correct.")
}

// completeVerification runs inside the verification transaction.
func (s *Service) completeVerification(ctx context.Context, tx pgx.Tx, challenge *models.Challenge, input VerifyInput) (VerifyResult, error) {
	pair := contact.Pair{
		EmailNormalized: challenge.EmailNormalized,
		EmailDisplay:    challenge.EmailNormalized,
		PhoneE164:       challenge.PhoneE164,
		PhoneDisplay:    challenge.PhoneE164,
	}

	// Step-up: no new session; grant a short-lived reference bound to the
	// originating session, organization and action class.
	if challenge.Purpose == models.PurposeStepUp {
		return VerifyResult{
			NextStep:        NextStepStepUpGranted,
			IdentityID:      derefString(challenge.IdentityID),
			StepUpReference: challenge.ID,
			StepUpExpiresAt: s.opts.Now().Add(s.opts.StepUpTTL),
		}, nil
	}

	identity, err := s.resolveOrCreateIdentity(ctx, tx, challenge, pair)
	if err != nil {
		return VerifyResult{}, err
	}

	// Staff invitation acceptance completes in the same transaction as the
	// verification, so a half-accepted invitation cannot exist.
	var invitedMembership *MembershipSummary
	if challenge.Purpose == models.PurposeStaffInvitation && challenge.InvitationID != nil && s.opts.Invitations != nil {
		membership, err := s.opts.Invitations.AcceptByToken(ctx, tx, *challenge.InvitationID, identity.ID, pair)
		if err != nil {
			return VerifyResult{}, err
		}
		invitedMembership = &membership
	}

	memberships, err := s.opts.Memberships.ListActiveForIdentity(ctx, identity.ID)
	if err != nil {
		return VerifyResult{}, err
	}
	if invitedMembership != nil {
		memberships = mergeMembership(memberships, *invitedMembership)
	}

	switch {
	case len(memberships) == 0:
		// New owner: an identity-scoped session with no organization claim so
		// they can create their organization draft.
		tokens, err := s.issueSession(ctx, tx, identity.ID, nil, input)
		if err != nil {
			return VerifyResult{}, err
		}
		return VerifyResult{NextStep: NextStepOnboardingRequired, Tokens: tokens, IdentityID: identity.ID}, nil

	case len(memberships) == 1:
		tokens, err := s.issueSession(ctx, tx, identity.ID, &memberships[0], input)
		if err != nil {
			return VerifyResult{}, err
		}
		return VerifyResult{NextStep: NextStepReady, Tokens: tokens, IdentityID: identity.ID, Organizations: memberships}, nil

	default:
		// Multiple memberships: issue a selection-scoped session (no
		// organization claim) plus the allowed organizations.
		tokens, err := s.issueSession(ctx, tx, identity.ID, nil, input)
		if err != nil {
			return VerifyResult{}, err
		}
		return VerifyResult{
			NextStep: NextStepOrganizationSelection, Tokens: tokens,
			IdentityID: identity.ID, Organizations: memberships,
		}, nil
	}
}

func (s *Service) resolveOrCreateIdentity(ctx context.Context, tx pgx.Tx, challenge *models.Challenge, pair contact.Pair) (*models.Identity, error) {
	existing, err := s.opts.Identities.ResolvePair(ctx, tx, pair.EmailNormalized, pair.PhoneE164)
	switch {
	case err == nil:
		if !existing.Usable(s.opts.Now()) {
			return nil, errorcodes.Forbidden(errorcodes.IdentitySuspended, "This account is not available. Contact support.")
		}
		if err := s.opts.Identities.MarkContactVerified(ctx, tx, existing.ID, existing.Version); err != nil {
			return nil, err
		}
		return existing, nil

	case errors.Is(err, authrepos.ErrIdentityPairConflict):
		return nil, errorcodes.Conflict(errorcodes.IdentityPairConflict,
			"These details do not match a single account. Contact support to continue.")

	case errors.Is(err, authrepos.ErrNotFound):
		identity := &models.Identity{
			ID:              ids.New("cid"),
			EmailNormalized: pair.EmailNormalized,
			EmailDisplay:    pair.EmailDisplay,
			PhoneE164:       pair.PhoneE164,
			PhoneDisplay:    pair.PhoneDisplay,
			Status:          models.IdentityStatusPending,
		}
		if err := s.opts.Identities.Create(ctx, tx, identity); err != nil {
			return nil, err
		}
		if err := s.opts.Identities.MarkContactVerified(ctx, tx, identity.ID, 1); err != nil {
			return nil, err
		}
		identity.Status = models.IdentityStatusActive
		return identity, nil

	default:
		return nil, err
	}
}

// issueSession creates a session row and signs the access token.
func (s *Service) issueSession(ctx context.Context, q database.Querier, identityID string, membership *MembershipSummary, input VerifyInput) (*TokenPair, error) {
	refreshToken, err := auth.GenerateOpaqueToken(32)
	if err != nil {
		return nil, err
	}
	now := s.opts.Now()
	session := &models.Session{
		ID:               ids.New("ses"),
		IdentityID:       identityID,
		SessionFamilyID:  ids.New("sfm"),
		RefreshTokenHash: auth.HashRefreshToken(s.opts.SessionSecret, refreshToken),
		Status:           models.SessionStatusActive,
		RefreshExpiresAt: now.Add(s.opts.RefreshTokenTTL),
		IPHash:           optional(input.IPHash),
		UserAgentHash:    optional(input.UserAgentHash),
		DeviceLabel:      optional(input.DeviceLabel),
		RoleVersion:      1,
	}
	if membership != nil {
		session.OrganizationID = &membership.OrganizationID
		session.OrganizationUserID = &membership.ID
		session.RoleVersion = membership.RoleVersion
	}
	if err := s.opts.Sessions.Create(ctx, q, session); err != nil {
		return nil, err
	}
	if err := s.opts.Sessions.AppendHistory(ctx, q, session.ID, identityID, "NONE", models.SessionStatusActive, "created", input.RequestID, input.CorrelationID); err != nil {
		return nil, err
	}

	accessToken, accessExpiresAt, err := s.signAccessToken(session)
	if err != nil {
		return nil, err
	}
	s.opts.Metrics.IncCounter("fleet_partner_sessions_created_total", nil, 1)

	return &TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		AccessExpiresAt:  accessExpiresAt,
		RefreshExpiresAt: session.RefreshExpiresAt,
		SessionID:        session.ID,
	}, nil
}

func (s *Service) signAccessToken(session *models.Session) (string, time.Time, error) {
	expiresAt := s.opts.Now().Add(s.opts.AccessTokenTTL)
	token, err := s.opts.AccessSigner.Sign(auth.Claims{
		Subject:   session.IdentityID,
		Role:      config.CorporateRole,
		Service:   config.ServiceName,
		SessionID: session.ID,
		Type:      auth.TokenTypeAccess,
		IssuedAt:  s.opts.Now().Unix(),
		ExpiresAt: expiresAt.Unix(),
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func mergeMembership(list []MembershipSummary, membership MembershipSummary) []MembershipSummary {
	for _, existing := range list {
		if existing.ID == membership.ID {
			return list
		}
	}
	return append(list, membership)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
