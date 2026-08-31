package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
)

// ResendInput asks for a fresh code on an existing challenge.
type ResendInput struct {
	ChallengeID   string
	IPHash        string
	RequestID     string
	CorrelationID string
}

// Resend issues a new code for an existing challenge.
//
// The destination email and phone come from the stored challenge, never from
// the request, so a resend can never redirect a code to another address.
func (s *Service) Resend(ctx context.Context, input ResendInput) (StartResult, error) {
	if err := s.enforceLimits(ctx, ratelimit.ScopeResend, map[string]string{
		"challenge": input.ChallengeID, "ip": input.IPHash,
	}, ratelimit.Rule{Limit: 5, Window: time.Hour}); err != nil {
		return StartResult{}, err
	}

	challenge, err := s.opts.Challenges.GetByID(ctx, input.ChallengeID)
	if err != nil {
		if errors.Is(err, authrepos.ErrNotFound) {
			return StartResult{}, errorcodes.Unauthorized(errorcodes.OTPChallengeNotFound, "This verification request is no longer valid.")
		}
		return StartResult{}, err
	}

	now := s.opts.Now()
	if challenge.Status != models.ChallengeStatusPending {
		return StartResult{}, errorcodes.Unauthorized(errorcodes.OTPChallengeExpired, "This verification request is no longer valid.")
	}
	if now.Before(challenge.ResendAvailableAt) {
		return StartResult{}, errorcodes.TooManyRequests(errorcodes.OTPResendTooSoon,
			"Please wait before requesting another code.")
	}

	code, err := auth.GenerateNumericOTP(s.opts.OTPLength)
	if err != nil {
		return StartResult{}, err
	}
	// Rotating the digest invalidates the previous code immediately.
	digest := auth.HashOTP(s.opts.OTPSecret, challenge.ID, challenge.PhoneE164, code)
	expiresAt := now.Add(s.opts.OTPExpiry)
	resendAt := now.Add(s.opts.OTPResendCooldown)

	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Challenges.Rotate(ctx, tx, challenge.ID, digest, expiresAt, resendAt)
	}); err != nil {
		return StartResult{}, errorcodes.Unauthorized(errorcodes.OTPChallengeExpired, "This verification request is no longer valid.")
	}

	challenge.ExpiresAt = expiresAt
	if err := s.deliver(ctx, challenge, code, challenge.Purpose); err != nil {
		return StartResult{}, err
	}

	return StartResult{
		ChallengeID:        challenge.ID,
		DeliveryChannel:    "EMAIL",
		MaskedDestination:  maskStored(challenge.EmailNormalized),
		ExpiresInSeconds:   int(s.opts.OTPExpiry.Seconds()),
		ResendAfterSeconds: int(s.opts.OTPResendCooldown.Seconds()),
	}, nil
}

// RefreshInput presents an opaque refresh token.
type RefreshInput struct {
	RefreshToken  string
	IPHash        string
	UserAgentHash string
	RequestID     string
	CorrelationID string
}

// Refresh rotates the refresh token and re-issues an access token.
//
// Reuse detection: refresh tokens are single-use, so presenting a token whose
// session is no longer ACTIVE means the token leaked. The entire family is
// revoked and the caller is refused.
func (s *Service) Refresh(ctx context.Context, input RefreshInput) (*TokenPair, error) {
	if input.RefreshToken == "" {
		return nil, errorcodes.Unauthorized(errorcodes.SessionExpired, "Your session has expired. Please sign in again.")
	}
	if err := s.enforceLimits(ctx, ratelimit.ScopeRefresh, map[string]string{
		"ip": input.IPHash,
	}, ratelimit.Rule{Limit: 60, Window: time.Hour}); err != nil {
		return nil, err
	}

	presentedHash := auth.HashRefreshToken(s.opts.SessionSecret, input.RefreshToken)

	var tokens *TokenPair
	// reusedSession is set when a spent refresh token was replayed; the
	// containment action runs after the transaction rolls back.
	var reusedSession *models.Session

	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		session, err := s.opts.Sessions.LockByRefreshHash(ctx, tx, presentedHash)
		if err != nil {
			if errors.Is(err, authrepos.ErrNotFound) {
				return errorcodes.Unauthorized(errorcodes.SessionExpired, "Your session has expired. Please sign in again.")
			}
			return err
		}

		if session.Status != models.SessionStatusActive {
			// A non-active session matching this hash is token reuse: the
			// token was already spent (rotated) or explicitly revoked.
			//
			// Returning an error here rolls this transaction back, so the
			// family revocation must happen afterwards on the pool —
			// otherwise the containment action would be undone with it.
			reusedSession = session
			return errorcodes.Unauthorized(errorcodes.RefreshTokenReused,
				"Your session is no longer valid. Please sign in again.")
		}

		now := s.opts.Now()
		if !now.Before(session.RefreshExpiresAt) {
			return errorcodes.Unauthorized(errorcodes.SessionExpired, "Your session has expired. Please sign in again.")
		}

		// Re-check identity, membership and organization state on every
		// refresh: a suspension must not survive a token rotation.
		identity, err := s.opts.Identities.GetByID(ctx, session.IdentityID)
		if err != nil {
			return err
		}
		if !identity.Usable(now) {
			_, _ = s.opts.Sessions.RevokeForIdentity(ctx, tx, identity.ID, "identity_not_usable")
			return errorcodes.Forbidden(errorcodes.IdentitySuspended, "This account is not available. Contact support.")
		}

		roleVersion := session.RoleVersion
		if session.OrganizationUserID != nil {
			membership, err := s.opts.Memberships.GetForIdentity(ctx, identity.ID, *session.OrganizationUserID)
			if err != nil {
				_ = s.opts.Sessions.Revoke(ctx, tx, session.ID, "membership_unavailable")
				return errorcodes.Forbidden(errorcodes.SessionRevoked, "Your organization access has changed. Please sign in again.")
			}
			if membership.Status != "ACTIVE" || membership.OrganizationStatus == "SUSPENDED" || membership.OrganizationStatus == "CLOSED" {
				_ = s.opts.Sessions.Revoke(ctx, tx, session.ID, "membership_or_organization_not_active")
				return errorcodes.Forbidden(errorcodes.SessionRevoked, "Your organization access has changed. Please sign in again.")
			}
			roleVersion = membership.RoleVersion
		}

		nextToken, err := auth.GenerateOpaqueToken(32)
		if err != nil {
			return err
		}
		nextHash := auth.HashRefreshToken(s.opts.SessionSecret, nextToken)
		refreshExpiresAt := now.Add(s.opts.RefreshTokenTTL)

		session.RoleVersion = roleVersion
		successor, err := s.opts.Sessions.RotateRefresh(ctx, tx, session, nextHash, refreshExpiresAt)
		if err != nil {
			// Lost a concurrent rotation race.
			return errorcodes.Unauthorized(errorcodes.SessionExpired, "Your session has expired. Please sign in again.")
		}
		if err := s.opts.Sessions.AppendHistory(ctx, tx, session.ID, session.IdentityID,
			models.SessionStatusActive, models.SessionStatusRevoked, "rotated",
			input.RequestID, input.CorrelationID); err != nil {
			return err
		}

		accessToken, accessExpiresAt, err := s.signAccessToken(successor)
		if err != nil {
			return err
		}
		s.opts.Metrics.IncCounter("fleet_partner_sessions_refreshed_total", nil, 1)

		tokens = &TokenPair{
			AccessToken:      accessToken,
			RefreshToken:     nextToken,
			AccessExpiresAt:  accessExpiresAt,
			RefreshExpiresAt: refreshExpiresAt,
			SessionID:        successor.ID,
		}
		return nil
	})

	if reusedSession != nil {
		s.containRefreshReuse(ctx, reusedSession, input)
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

// containRefreshReuse revokes the whole token family after a spent refresh
// token is replayed, so an attacker holding a stolen token loses every
// descendant session — including the one the legitimate user is holding.
func (s *Service) containRefreshReuse(ctx context.Context, session *models.Session, input RefreshInput) {
	var count int64
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Sessions.LockFamily(ctx, tx, session.SessionFamilyID); err != nil {
			return err
		}
		var err error
		count, err = s.opts.Sessions.RevokeFamily(ctx, tx, session.SessionFamilyID, "refresh_token_reuse")
		return err
	})
	if err != nil {
		return
	}
	_ = s.opts.SecurityEvents.Record(ctx, s.opts.Pool, models.SecurityEvent{
		EventType: models.SecurityEventRefreshReuse, Severity: models.SeverityCritical,
		IdentityID: &session.IdentityID, OrganizationID: session.OrganizationID,
		Detail:    map[string]any{"revokedSessions": count},
		RequestID: input.RequestID, CorrelationID: input.CorrelationID,
	})
	s.opts.Metrics.IncCounter("fleet_partner_refresh_reuse_detected_total", nil, 1)
}

// LogoutRefresh resolves the browser credential through the existing session
// authority. Retired credentials revoke the same family, including a successor
// that won a concurrent refresh; no cookie secret is stored in history.
func (s *Service) LogoutRefresh(ctx context.Context, token, requestID, correlationID string) error {
	if token == "" {
		return nil
	}
	hash := auth.HashRefreshToken(s.opts.SessionSecret, token)
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		session, err := s.opts.Sessions.LockByRefreshHash(ctx, tx, hash)
		if errors.Is(err, authrepos.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		count, err := s.opts.Sessions.LogoutFamily(ctx, tx, session.SessionFamilyID)
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		return s.opts.Sessions.AppendHistory(ctx, tx, session.ID, session.IdentityID, session.Status, models.SessionStatusRevoked, "logout", requestID, correlationID)
	})
}

// Logout revokes the current browser session family. It is idempotent: logging out twice
// succeeds both times.
func (s *Service) Logout(ctx context.Context, sessionID string, identityID string, requestID string, correlationID string) error {
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		session, err := s.opts.Sessions.LockByID(ctx, tx, sessionID)
		if err != nil {
			if errors.Is(err, authrepos.ErrNotFound) {
				return nil // already gone
			}
			return err
		}
		if session.IdentityID != identityID {
			// Never reveal that someone else's session exists.
			return nil
		}
		count, err := s.opts.Sessions.LogoutFamily(ctx, tx, session.SessionFamilyID)
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		s.opts.Metrics.IncCounter("fleet_partner_sessions_revoked_total", map[string]string{"reason": "logout"}, 1)
		return s.opts.Sessions.AppendHistory(ctx, tx, sessionID, identityID, session.Status, models.SessionStatusRevoked, "logout", requestID, correlationID)
	})
}

// ListSessions returns the caller's own sessions.
func (s *Service) ListSessions(ctx context.Context, identityID string) ([]models.Session, error) {
	return s.opts.Sessions.ListForIdentity(ctx, identityID)
}

// RevokeSession revokes one of the caller's own logical browser session families. A session belonging
// to another identity is reported as not found.
func (s *Service) RevokeSession(ctx context.Context, identityID string, sessionID string, requestID string, correlationID string) error {
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		session, err := s.opts.Sessions.LockByID(ctx, tx, sessionID)
		if errors.Is(err, authrepos.ErrNotFound) {
			return apperrors.NotFound("Session not found.", nil)
		}
		if err != nil {
			return err
		}
		if session.IdentityID != identityID {
			return apperrors.NotFound("Session not found.", nil)
		}
		count, err := s.opts.Sessions.RevokeActiveFamily(ctx, tx, session.SessionFamilyID, "user_revoked")
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		s.opts.Metrics.IncCounter("fleet_partner_sessions_revoked_total", map[string]string{"reason": "user"}, 1)
		return s.opts.Sessions.AppendHistory(ctx, tx, sessionID, identityID, session.Status, models.SessionStatusRevoked, "user_revoked", requestID, correlationID)
	})
}

// SelectOrganizationInput chooses one membership.
type SelectOrganizationInput struct {
	IdentityID    string
	SessionID     string
	MembershipID  string
	IPHash        string
	UserAgentHash string
	DeviceLabel   string
	RequestID     string
	CorrelationID string
}

// SelectOrganization rotates an active source into an organization-scoped
// successor in the SAME browser session family. JWT validity alone is not
// authority to mint a session after persistent logout or replay containment.
//
// The membership must belong to the authenticated identity; an arbitrary
// organization ID is never trusted, and the organization must be usable.
func (s *Service) SelectOrganization(ctx context.Context, input SelectOrganizationInput) (*TokenPair, error) {
	if err := s.enforceLimits(ctx, ratelimit.ScopeOrganizationSwitch, map[string]string{
		"identity": input.IdentityID,
	}, ratelimit.Rule{Limit: 30, Window: time.Hour}); err != nil {
		return nil, err
	}

	var tokens *TokenPair
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		source, err := s.opts.Sessions.LockByID(ctx, tx, input.SessionID)
		if errors.Is(err, authrepos.ErrNotFound) {
			return invalidOrganizationSession()
		}
		if err != nil {
			return err
		}
		if err := validateOrganizationSource(source, input.IdentityID, s.opts.Now()); err != nil {
			return err
		}
		compromised, err := s.opts.Sessions.FamilyCompromised(ctx, tx, source.SessionFamilyID)
		if err != nil {
			return err
		}
		if compromised {
			return invalidOrganizationSession()
		}
		identity, err := s.opts.Identities.GetByIDUsing(ctx, tx, input.IdentityID)
		if errors.Is(err, authrepos.ErrNotFound) {
			return invalidOrganizationSession()
		}
		if err != nil {
			return err
		}
		if !identity.Usable(s.opts.Now()) {
			return invalidOrganizationSession()
		}
		membership, err := s.opts.Memberships.GetForIdentity(ctx, input.IdentityID, input.MembershipID)
		if err != nil {
			if errors.Is(err, ErrMembershipNotFound) {
				return apperrors.NotFound("Membership not found.", nil)
			}
			return err
		}
		if membership.Status != "ACTIVE" || membership.OrganizationStatus == "SUSPENDED" || membership.OrganizationStatus == "CLOSED" {
			return errorcodes.Forbidden(errorcodes.SessionRevoked, "Your organization access is not active.")
		}
		// Recheck expiry after authority read-back; keep both locks until commit.
		if err := validateOrganizationSource(source, input.IdentityID, s.opts.Now()); err != nil {
			return err
		}
		nextToken, err := auth.GenerateOpaqueToken(32)
		if err != nil {
			return err
		}
		source.OrganizationUserID = &membership.ID
		source.OrganizationID = &membership.OrganizationID
		source.RoleVersion = membership.RoleVersion
		source.IPHash, source.UserAgentHash, source.DeviceLabel = optional(input.IPHash), optional(input.UserAgentHash), optional(input.DeviceLabel)
		expires := s.opts.Now().Add(s.opts.RefreshTokenTTL)
		successor, err := s.opts.Sessions.RotateRefresh(ctx, tx, source, auth.HashRefreshToken(s.opts.SessionSecret, nextToken), expires)
		if errors.Is(err, authrepos.ErrSessionNotRotatable) {
			return invalidOrganizationSession()
		}
		if err != nil {
			return err
		}
		if err := s.opts.Sessions.AppendHistory(ctx, tx, source.ID, source.IdentityID, models.SessionStatusActive, models.SessionStatusRevoked, "organization_selected", input.RequestID, input.CorrelationID); err != nil {
			return err
		}
		if err := s.opts.Sessions.AppendHistory(ctx, tx, successor.ID, successor.IdentityID, "NONE", models.SessionStatusActive, "organization_selected", input.RequestID, input.CorrelationID); err != nil {
			return err
		}
		access, accessExpiry, err := s.signAccessToken(successor)
		if err != nil {
			return err
		}
		tokens = &TokenPair{AccessToken: access, AccessExpiresAt: accessExpiry, RefreshToken: nextToken, RefreshExpiresAt: expires, SessionID: successor.ID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

func invalidOrganizationSession() error {
	return errorcodes.Unauthorized(errorcodes.SessionExpired, "Your session has expired. Please sign in again.")
}

func validateOrganizationSource(session *models.Session, identityID string, now time.Time) error {
	if session == nil || identityID == "" || session.IdentityID != identityID || session.ID == "" || session.SessionFamilyID == "" || session.Status != models.SessionStatusActive || session.RevokedAt != nil || !now.Before(session.RefreshExpiresAt) {
		return invalidOrganizationSession()
	}
	return nil
}

// StepUpGrant is a verified step-up reference.
type StepUpGrant struct {
	Reference      string
	IdentityID     string
	SessionID      string
	OrganizationID string
	ActionClass    string
	ExpiresAt      time.Time
}

// VerifyStepUpGrant checks that a step-up reference is valid for this exact
// session, organization and action class, and has not expired.
//
// A grant cannot be transferred to another session or reused for a different
// action category.
func (s *Service) VerifyStepUpGrant(ctx context.Context, reference string, identityID string, sessionID string, organizationID string, actionClass string) error {
	if reference == "" {
		return errorcodes.Forbidden(errorcodes.StepUpRequired, "This action requires additional verification.")
	}
	challenge, err := s.opts.Challenges.GetByID(ctx, reference)
	if err != nil {
		return errorcodes.Forbidden(errorcodes.StepUpRequired, "This action requires additional verification.")
	}
	if challenge.Purpose != models.PurposeStepUp || challenge.Status != models.ChallengeStatusConsumed || challenge.VerifiedAt == nil {
		return errorcodes.Forbidden(errorcodes.StepUpRequired, "This action requires additional verification.")
	}
	if s.opts.Now().After(challenge.VerifiedAt.Add(s.opts.StepUpTTL)) {
		return errorcodes.Forbidden(errorcodes.StepUpExpired, "Your verification has expired. Please confirm again.")
	}
	if derefString(challenge.IdentityID) != identityID ||
		derefString(challenge.SessionID) != sessionID ||
		derefString(challenge.OrganizationID) != organizationID ||
		derefString(challenge.ActionClass) != actionClass {
		return errorcodes.Forbidden(errorcodes.StepUpRequired, "This action requires additional verification.")
	}
	return nil
}

// IdentityByID loads an identity; used by step-up to resolve the caller's own
// verified contact pair rather than trusting request input.
func (s *Service) IdentityByID(ctx context.Context, identityID string) (*models.Identity, error) {
	return s.opts.Identities.GetByID(ctx, identityID)
}

// SenderConfigured reports whether live OTP delivery is possible; used by the
// readiness endpoint.
func (s *Service) SenderConfigured() bool {
	if configurable, ok := s.opts.Sender.(interface{ Configured() bool }); ok {
		return configurable.Configured()
	}
	// A fake sender is only ever wired in tests.
	_, isFake := s.opts.Sender.(*authclients.FakeSender)
	return isFake
}

func maskStored(email string) string {
	at := len(email)
	for i, r := range email {
		if r == '@' {
			at = i
			break
		}
	}
	if at <= 0 || at >= len(email) {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}
