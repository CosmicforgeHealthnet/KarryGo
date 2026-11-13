package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/contact"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Provider login-notice resend (spec Section 10 "resend secure login
// instructions", FLEET-10.8).
//
// Fleet never issues, stores or resets a provider credential. A managed
// provider signs in through the OWNING provider service's passwordless flow
// (email + phone + emailed OTP). What an organization can resend is therefore
// only the notice that tells the provider to do that:
//
//   - PENDING_PROVIDER_ACCEPTANCE: the association invitation. Its single-use
//     token is rotated, so the previous link stops working, and its expiry is
//     renewed. An invitation that already expired is replaced by a fresh one
//     carrying the same contact pair.
//   - PENDING_SETUP: the first-login notice. The provider has consented; this
//     tells them to finish setup by signing in to their provider app. It
//     carries no token, code or credential.
//
// Any other membership state has nothing pending and is refused. Resends are
// rate limited per invitation (cooldown plus a cap), serialised by a row lock,
// and idempotent on the caller's Idempotency-Key. Delivery is email only.

const (
	// LoginResendCooldown is the minimum time between two notices.
	LoginResendCooldown = 60 * time.Second
	// MaxLoginResends caps organization-requested resends per invitation.
	MaxLoginResends = 5

	NoticeAssociationInvitation = "ASSOCIATION_INVITATION"
	NoticeFirstLogin            = "FIRST_LOGIN_NOTICE"

	resendLoginEndpoint = "POST /providers/:membershipId/resend-login"
)

// ResendLoginInput asks to re-send a pending provider's login notice.
type ResendLoginInput struct {
	Tenant         tenant.Context
	MembershipID   string
	IdempotencyKey string
}

// ResendLoginResult describes what was sent. It never carries a token.
type ResendLoginResult struct {
	MembershipID          string     `json:"membershipId"`
	MembershipStatus      string     `json:"membershipStatus"`
	Notice                string     `json:"notice"`
	InvitationID          string     `json:"invitationId"`
	DeliveryStatus        string     `json:"deliveryStatus"`
	Channel               string     `json:"channel"`
	MaskedEmail           string     `json:"maskedEmail"`
	SignInMethod          string     `json:"signInMethod"`
	ResendCount           int        `json:"resendCount"`
	ResendsRemaining      int        `json:"resendsRemaining"`
	InvitationExpiresAt   *time.Time `json:"invitationExpiresAt,omitempty"`
	NextResendAvailableAt time.Time  `json:"nextResendAvailableAt"`
	Replayed              bool       `json:"replayed"`
}

// ResendLogin re-sends the association invitation or first-login notice.
func (s *Service) ResendLogin(ctx context.Context, input ResendLoginInput) (*ResendLoginResult, error) {
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		return nil, apperrors.BadRequest("Idempotency-Key header is required.", nil)
	}
	if _, err := s.VisibleMembership(ctx, input.Tenant, input.MembershipID); err != nil {
		return nil, err
	}

	store := idempotency.NewStore(s.opts.Pool, 0)
	fingerprint := idempotency.Fingerprint("POST", resendLoginEndpoint, []byte(input.MembershipID))
	record, err := store.Begin(ctx, input.Tenant.OrganizationID, resendLoginEndpoint, key, fingerprint)
	if err != nil {
		return nil, idempotency.AsAppError(err)
	}
	if record != nil {
		var replay ResendLoginResult
		if err := json.Unmarshal(record.ResponseBody, &replay); err != nil {
			return nil, err
		}
		replay.Replayed = true
		return &replay, nil
	}

	result, err := s.resendLogin(ctx, input)
	if err != nil {
		_ = store.Release(ctx, input.Tenant.OrganizationID, resendLoginEndpoint, key)
		return nil, err
	}
	body, _ := json.Marshal(result)
	if err := store.Complete(ctx, input.Tenant.OrganizationID, resendLoginEndpoint, key, 200, body); err != nil {
		return nil, err
	}
	return result, nil
}

type resendClaim struct {
	invitation   models.Invitation
	membership   models.Membership
	notice       string
	token        string
	previousSent *time.Time
}

func (s *Service) resendLogin(ctx context.Context, input ResendLoginInput) (*ResendLoginResult, error) {
	now := s.opts.Now().UTC()
	var claim resendClaim
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		membership, err := s.opts.Providers.GetMembershipForUpdate(ctx, tx, input.Tenant.OrganizationID, input.MembershipID)
		if err != nil {
			if errors.Is(err, providerrepos.ErrNotFound) {
				return apperrors.NotFound("Provider not found.", nil)
			}
			return err
		}
		switch membership.Status {
		case models.StatusPendingProviderAcceptance:
			claim.notice = NoticeAssociationInvitation
		case models.StatusPendingSetup:
			claim.notice = NoticeFirstLogin
		default:
			return errorcodes.Conflict(errorcodes.ProviderLoginNotPending,
				"Login instructions can only be resent while the provider has not yet accepted or finished setup.")
		}
		invitation, err := s.opts.Providers.LockLatestInvitation(ctx, tx, input.Tenant.OrganizationID, input.MembershipID)
		if err != nil {
			if errors.Is(err, providerrepos.ErrNotFound) {
				return errorcodes.Conflict(errorcodes.ProviderLoginNotPending,
					"This provider has no invitation on record to resend.")
			}
			return err
		}

		var expiresAt *time.Time
		tokenHash := ""
		if claim.notice == NoticeAssociationInvitation {
			token, err := auth.GenerateOpaqueToken(32)
			if err != nil {
				return err
			}
			claim.token = token
			tokenHash = auth.HashRefreshToken(s.opts.InvitationSecret, token)
			expiry := now.Add(s.opts.InvitationTTL)
			expiresAt = &expiry
			switch {
			case invitation.Status == models.InvitationPending && invitation.ExpiresAt.After(now):
			case invitation.Status == models.InvitationPending || invitation.Status == models.InvitationExpired:
				// The previous link has lapsed. A fresh invitation carrying the
				// same contact pair replaces it; the lapsed row is kept as history.
				replacement := &models.Invitation{
					MembershipID: &membership.ID, ProviderService: invitation.ProviderService,
					LegalName: invitation.LegalName, EmailNormalized: invitation.EmailNormalized,
					PhoneE164: invitation.PhoneE164, ExistingProviderID: membership.ProviderID,
					InvitationTokenHash: tokenHash, ExpiresAt: expiry, CreatedBy: input.Tenant.ActorID,
				}
				if err := s.opts.Providers.ExpireInvitation(ctx, tx, invitation.ID); err != nil {
					return err
				}
				if err := s.opts.Providers.CreateInvitation(ctx, tx, input.Tenant.OrganizationID, replacement); err != nil {
					return err
				}
				replacement.LastSentAt, replacement.ResendCount = nil, 0
				invitation = replacement
			default:
				return errorcodes.Conflict(errorcodes.ProviderLoginNotPending,
					"The provider has already answered this invitation.")
			}
		}

		if invitation.LastSentAt != nil && now.Sub(invitation.LastSentAt.UTC()) < LoginResendCooldown {
			return errorcodes.TooManyRequests(errorcodes.ProviderLoginResendTooSoon,
				fmt.Sprintf("Login instructions were sent recently. Try again after %s.",
					invitation.LastSentAt.UTC().Add(LoginResendCooldown).Format(time.RFC3339)))
		}
		if invitation.ResendCount >= MaxLoginResends {
			return errorcodes.TooManyRequests(errorcodes.ProviderLoginResendLimitReached,
				"Login instructions have been resent the maximum number of times for this invitation.")
		}

		if err := s.opts.Providers.ClaimInvitationResend(ctx, tx, invitation.ID, invitation.Version, tokenHash, expiresAt); err != nil {
			if errors.Is(err, providerrepos.ErrInvitationNotUsable) {
				return errorcodes.Conflict(errorcodes.ProviderLoginNotPending,
					"The invitation changed while resending. Refresh and try again.")
			}
			return err
		}
		claim.previousSent = invitation.LastSentAt
		invitation.ResendCount++
		invitation.Version++
		sentAt := now
		invitation.LastSentAt = &sentAt
		if expiresAt != nil {
			invitation.ExpiresAt = *expiresAt
		}
		claim.invitation = *invitation
		claim.membership = *membership

		entry := audit.EntryFromTenant(input.Tenant, "provider.login_resend_requested", "provider_membership", membership.ID)
		entry.NewValues = map[string]any{"notice": claim.notice, "invitationId": invitation.ID, "resendCount": invitation.ResendCount}
		return s.opts.Audit.Record(ctx, tx, entry)
	})
	if err != nil {
		return nil, err
	}

	reference, sendErr := s.sendLoginNotice(ctx, input.Tenant, claim)
	if sendErr != nil {
		_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			if err := s.opts.Providers.ReleaseInvitationResend(ctx, tx, claim.invitation.ID, claim.previousSent); err != nil {
				return err
			}
			entry := audit.EntryFromTenant(input.Tenant, "provider.login_resend_failed", "provider_membership", claim.membership.ID)
			entry.NewValues = map[string]any{"notice": claim.notice, "invitationId": claim.invitation.ID, "deliveryStatus": "UNAVAILABLE"}
			return s.opts.Audit.Record(ctx, tx, entry)
		})
		s.opts.Metrics.IncCounter("fleet_partner_provider_login_resend_total", map[string]string{"outcome": "delivery_unavailable"}, 1)
		return nil, errorcodes.Unavailable(errorcodes.ProviderLoginDeliveryUnavailable,
			"The login instructions could not be sent right now. Try again shortly.")
	}

	maskedEmail := contact.MaskEmail(claim.invitation.EmailNormalized)
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Providers.ConfirmInvitationResend(ctx, tx, claim.invitation.ID, reference); err != nil {
			return err
		}
		entry := audit.EntryFromTenant(input.Tenant, "provider.login_resent", "provider_membership", claim.membership.ID)
		entry.NewValues = map[string]any{"notice": claim.notice, "invitationId": claim.invitation.ID,
			"resendCount": claim.invitation.ResendCount, "channel": "email"}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.provider.login_resent.v1", "provider_membership",
			claim.membership.ID, claim.membership.Version, input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{
				"membershipId": claim.membership.ID, "organizationId": input.Tenant.OrganizationID,
				"notice": claim.notice, "invitationId": claim.invitation.ID,
				"resendCount": claim.invitation.ResendCount, "channel": "email",
				// No token, no OTP and no credential in the event payload.
				"maskedEmail": maskedEmail,
			})
	})
	if err != nil {
		return nil, err
	}
	s.opts.Metrics.IncCounter("fleet_partner_provider_login_resend_total", map[string]string{"outcome": "sent"}, 1)

	result := &ResendLoginResult{
		MembershipID: claim.membership.ID, MembershipStatus: claim.membership.Status,
		Notice: claim.notice, InvitationID: claim.invitation.ID, DeliveryStatus: "ACCEPTED",
		Channel: "email", MaskedEmail: maskedEmail, SignInMethod: "email_and_phone_otp",
		ResendCount: claim.invitation.ResendCount, ResendsRemaining: MaxLoginResends - claim.invitation.ResendCount,
		NextResendAvailableAt: claim.invitation.LastSentAt.Add(LoginResendCooldown),
	}
	if claim.notice == NoticeAssociationInvitation {
		expires := claim.invitation.ExpiresAt.UTC()
		result.InvitationExpiresAt = &expires
	}
	return result, nil
}

func (s *Service) sendLoginNotice(ctx context.Context, tenantCtx tenant.Context, claim resendClaim) (string, error) {
	if s.opts.Sender == nil {
		return "", authclients.ErrDeliveryUnavailable
	}
	organizationName := ""
	if s.opts.Organizations != nil {
		organizationName, _, _ = s.opts.Organizations.OrganizationStatus(ctx, tenantCtx.OrganizationID)
	}
	message := authclients.InvitationMessage{
		Email: claim.invitation.EmailNormalized, Phone: claim.invitation.PhoneE164,
		OrganizationName: organizationName,
		IdempotencyKey:   fmt.Sprintf("provider-login-resend:%s:%d", claim.invitation.ID, claim.invitation.ResendCount),
		RequestID:        tenantCtx.RequestID, CorrelationID: tenantCtx.CorrelationID,
	}
	if claim.notice == NoticeAssociationInvitation {
		message.EventType = authclients.EventCorporateProviderInvitation
		message.ExpiresAt = claim.invitation.ExpiresAt
		if s.opts.AcceptURLTemplate != "" {
			message.AcceptURL = fmt.Sprintf("%s?token=%s", s.opts.AcceptURLTemplate, claim.token)
		}
	} else {
		message.EventType = authclients.EventCorporateProviderLoginNotice
		message.TemplateKey = "corporate_provider_login_notice"
		message.Title = "Finish setting up your " + organizationName + " provider account"
		message.Body = "Open your provider app and sign in with this email address and your phone number. " +
			"We will email you a one-time code; no password is needed."
	}
	return s.opts.Sender.SendInvitation(ctx, message)
}
