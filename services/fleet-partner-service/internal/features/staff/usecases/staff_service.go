// Package usecases implements company staff and IAM flows: invitations,
// membership lifecycle, roles and role bindings.
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
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/contact"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// OrganizationReader exposes the organization facts staff flows need without
// importing the organizations feature's repositories.
type OrganizationReader interface {
	OrganizationStatus(ctx context.Context, organizationID string) (name string, status string, err error)
}

// EventPublisher appends domain events to the transactional outbox.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string, aggregateVersion int64, organizationID string, correlationID string, payload any) error
}

// SessionRevoker revokes sessions when membership changes.
type SessionRevoker interface {
	RevokeForOrganizationUser(ctx context.Context, q database.Querier, organizationUserID string, reason string) (int64, error)
}

// Options configure the staff service.
type Options struct {
	Pool          *pgxpool.Pool
	Members       *staffrepos.MemberRepository
	Invitations   *staffrepos.InvitationRepository
	Roles         *staffrepos.RoleRepository
	Organizations OrganizationReader
	Sender        authclients.Sender
	Sessions      SessionRevoker
	Events        EventPublisher
	Audit         *audit.Recorder
	Metrics       *observability.Metrics

	InvitationSecret   []byte
	InvitationTTL      time.Duration
	AcceptURLTemplate  string
	DefaultCountryCode string
	Now                func() time.Time
}

// Service implements staff and IAM use cases.
type Service struct {
	opts Options
}

// NewService builds the service.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.InvitationTTL <= 0 {
		opts.InvitationTTL = 7 * 24 * time.Hour
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}
	if opts.Audit == nil {
		opts.Audit = audit.NewRecorder()
	}
	return &Service{opts: opts}
}

// ---- MembershipReader (consumed by the auth feature) ----

// ListActiveForIdentity returns the identity's live memberships.
func (s *Service) ListActiveForIdentity(ctx context.Context, identityID string) ([]authusecases.MembershipSummary, error) {
	members, err := s.opts.Members.ListForIdentity(ctx, identityID)
	if err != nil {
		return nil, err
	}
	summaries := make([]authusecases.MembershipSummary, 0, len(members))
	for _, member := range members {
		name, status, err := s.opts.Organizations.OrganizationStatus(ctx, member.OrganizationID)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, toSummary(member, name, status))
	}
	return summaries, nil
}

// GetForIdentity loads one membership and confirms it belongs to the identity.
func (s *Service) GetForIdentity(ctx context.Context, identityID string, membershipID string) (authusecases.MembershipSummary, error) {
	members, err := s.opts.Members.ListForIdentity(ctx, identityID)
	if err != nil {
		return authusecases.MembershipSummary{}, err
	}
	for _, member := range members {
		if member.ID == membershipID {
			name, status, err := s.opts.Organizations.OrganizationStatus(ctx, member.OrganizationID)
			if err != nil {
				return authusecases.MembershipSummary{}, err
			}
			return toSummary(member, name, status), nil
		}
	}
	return authusecases.MembershipSummary{}, authusecases.ErrMembershipNotFound
}

func toSummary(member models.OrganizationUser, organizationName string, organizationStatus string) authusecases.MembershipSummary {
	return authusecases.MembershipSummary{
		ID:                 member.ID,
		OrganizationID:     member.OrganizationID,
		OrganizationName:   organizationName,
		OrganizationStatus: organizationStatus,
		Status:             member.Status,
		IsOwner:            member.IsOwner,
		RoleVersion:        member.RoleVersion,
		PermittedBranchIDs: member.PermittedBranchIDs,
	}
}

// ---- InvitationAcceptor (consumed by the auth feature) ----

// AcceptByToken consumes a staff invitation during OTP verification.
//
// It runs inside the verification transaction, so an invitation can never be
// half-accepted. The invitation is bound to the exact normalized email and
// phone it was issued for.
func (s *Service) AcceptByToken(ctx context.Context, tx pgx.Tx, token string, identityID string, pair contact.Pair) (authusecases.MembershipSummary, error) {
	tokenHash := auth.HashRefreshToken(s.opts.InvitationSecret, token)

	invitation, err := s.opts.Invitations.LockByTokenHash(ctx, tx, tokenHash)
	if err != nil {
		if errors.Is(err, staffrepos.ErrNotFound) {
			return authusecases.MembershipSummary{}, errorcodes.Unauthorized(errorcodes.StaffInvitationConsumed,
				"This invitation is no longer valid.")
		}
		return authusecases.MembershipSummary{}, err
	}

	now := s.opts.Now()
	if invitation.Status != models.InvitationStatusPending {
		return authusecases.MembershipSummary{}, errorcodes.Unauthorized(errorcodes.StaffInvitationConsumed,
			"This invitation has already been used.")
	}
	if !now.Before(invitation.ExpiresAt) {
		return authusecases.MembershipSummary{}, errorcodes.Unauthorized(errorcodes.StaffInvitationExpired,
			"This invitation has expired. Ask for a new one.")
	}
	// The invitation is bound to the exact contact pair it was issued for.
	if invitation.EmailNormalized != pair.EmailNormalized || invitation.PhoneE164 != pair.PhoneE164 {
		return authusecases.MembershipSummary{}, errorcodes.Unauthorized(errorcodes.StaffInvitationConsumed,
			"This invitation is no longer valid.")
	}

	// An identity already holding a live membership cannot accept a second
	// invitation into the same organization.
	if existing, err := s.opts.Members.GetByIdentity(ctx, invitation.OrganizationID, identityID); err == nil && existing.Live() {
		if existing.Status == models.UserStatusActive {
			_ = s.opts.Invitations.MarkAccepted(ctx, tx, invitation.ID, existing.ID)
			name, status, _ := s.opts.Organizations.OrganizationStatus(ctx, invitation.OrganizationID)
			return toSummary(*existing, name, status), nil
		}
		return authusecases.MembershipSummary{}, errorcodes.Conflict(errorcodes.StaffAlreadyMember,
			"You already belong to this organization.")
	} else if err != nil && !errors.Is(err, staffrepos.ErrNotFound) {
		return authusecases.MembershipSummary{}, err
	}

	member := &models.OrganizationUser{
		ID:                     ids.New("ousr"),
		IdentityID:             identityID,
		FullName:               invitation.FullName,
		Status:                 models.UserStatusActive,
		PermittedBranchIDs:     invitation.PermittedBranchIDs,
		PermittedDepartmentIDs: invitation.PermittedDepartmentIDs,
		RoleVersion:            1,
		InvitedBy:              &invitation.CreatedBy,
		InvitedAt:              &invitation.CreatedAt,
		ActivatedAt:            &now,
	}
	if err := s.opts.Members.Create(ctx, tx, invitation.OrganizationID, member); err != nil {
		return authusecases.MembershipSummary{}, err
	}

	for _, roleID := range invitation.RoleIDs {
		if err := s.opts.Roles.CreateBinding(ctx, tx, invitation.OrganizationID, &models.RoleBinding{
			OrganizationUserID: member.ID,
			RoleID:             roleID,
			ScopeType:          models.ScopeOrganization,
			GrantedBy:          invitation.CreatedBy,
		}); err != nil {
			return authusecases.MembershipSummary{}, err
		}
	}

	if err := s.opts.Invitations.MarkAccepted(ctx, tx, invitation.ID, member.ID); err != nil {
		return authusecases.MembershipSummary{}, errorcodes.Unauthorized(errorcodes.StaffInvitationConsumed,
			"This invitation has already been used.")
	}

	if err := s.opts.Events.Publish(ctx, tx, "organization_user.activated.v1", "organization_user",
		member.ID, 1, invitation.OrganizationID, "", map[string]any{
			"organizationUserId": member.ID,
			"organizationId":     invitation.OrganizationID,
			"acceptedInvitation": invitation.ID,
		}); err != nil {
		return authusecases.MembershipSummary{}, err
	}

	s.opts.Metrics.IncCounter("fleet_partner_staff_invitations_total", map[string]string{"outcome": "accepted"}, 1)

	name, status, err := s.opts.Organizations.OrganizationStatus(ctx, invitation.OrganizationID)
	if err != nil {
		return authusecases.MembershipSummary{}, err
	}
	return toSummary(*member, name, status), nil
}

// ---- Invitation issuing ----

// InviteInput describes a new staff invitation.
type InviteInput struct {
	Tenant        tenant.Context
	FullName      string
	Email         string
	Phone         string
	RoleIDs       []string
	BranchIDs     []string
	DepartmentIDs []string
	ExpiresIn     time.Duration
}

// Invite creates and emails a staff invitation.
//
// Both email and phone are mandatory. No password or temporary credential is
// ever created or sent: the invitee authenticates with email + phone + an
// emailed OTP.
func (s *Service) Invite(ctx context.Context, input InviteInput) (*models.StaffInvitation, error) {
	if input.FullName == "" {
		return nil, apperrors.Validation("A full name is required.", []apperrors.FieldViolation{
			{Field: "fullName", Message: "Required."}})
	}
	pair, err := contact.NormalizePair(input.Email, input.Phone, s.opts.DefaultCountryCode)
	if err != nil {
		return nil, apperrors.Validation("A valid email address and phone number are both required.",
			[]apperrors.FieldViolation{{Field: "email", Message: "A valid email and phone are required."}})
	}
	if len(input.RoleIDs) == 0 {
		return nil, apperrors.Validation("At least one role is required.", []apperrors.FieldViolation{
			{Field: "roleIds", Message: "Required."}})
	}
	for _, roleID := range input.RoleIDs {
		if _, err := s.opts.Roles.GetRole(ctx, input.Tenant.OrganizationID, roleID); err != nil {
			return nil, apperrors.Validation("One or more roles are not available to this organization.",
				[]apperrors.FieldViolation{{Field: "roleIds", Message: "Unknown role."}})
		}
	}

	token, err := auth.GenerateOpaqueToken(32)
	if err != nil {
		return nil, err
	}
	ttl := input.ExpiresIn
	if ttl <= 0 {
		ttl = s.opts.InvitationTTL
	}
	invitation := &models.StaffInvitation{
		ID:                     ids.New("inv"),
		FullName:               input.FullName,
		EmailNormalized:        pair.EmailNormalized,
		PhoneE164:              pair.PhoneE164,
		InvitationTokenHash:    auth.HashRefreshToken(s.opts.InvitationSecret, token),
		RoleIDs:                input.RoleIDs,
		PermittedBranchIDs:     input.BranchIDs,
		PermittedDepartmentIDs: input.DepartmentIDs,
		ExpiresAt:              s.opts.Now().Add(ttl),
		CreatedBy:              input.Tenant.ActorID,
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Invitations.Create(ctx, tx, input.Tenant.OrganizationID, invitation); err != nil {
			return err
		}
		entry := audit.EntryFromTenant(input.Tenant, "staff.invitation.created", "staff_invitation", invitation.ID)
		entry.NewValues = map[string]any{"email": pair.EmailNormalized, "roleIds": input.RoleIDs}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization_user.invited.v1", "staff_invitation",
			invitation.ID, 1, input.Tenant.OrganizationID, input.Tenant.CorrelationID, map[string]any{
				"invitationId":   invitation.ID,
				"organizationId": input.Tenant.OrganizationID,
				// Deliberately no token and no OTP in the event payload.
				"maskedEmail": contact.MaskEmail(pair.EmailNormalized),
			})
	})
	if err != nil {
		return nil, err
	}

	s.deliverInvitation(ctx, input.Tenant, invitation, token)
	s.opts.Metrics.IncCounter("fleet_partner_staff_invitations_total", map[string]string{"outcome": "created"}, 1)
	return invitation, nil
}

func (s *Service) deliverInvitation(ctx context.Context, tenantCtx tenant.Context, invitation *models.StaffInvitation, token string) {
	organizationName, _, _ := s.opts.Organizations.OrganizationStatus(ctx, invitation.OrganizationID)
	acceptURL := s.opts.AcceptURLTemplate
	if acceptURL != "" {
		acceptURL = fmt.Sprintf("%s?token=%s", acceptURL, token)
	}

	reference, err := s.opts.Sender.SendInvitation(ctx, authclients.InvitationMessage{
		Email:            invitation.EmailNormalized,
		Phone:            invitation.PhoneE164,
		OrganizationName: organizationName,
		InvitationToken:  token,
		AcceptURL:        acceptURL,
		ExpiresAt:        invitation.ExpiresAt,
		EventType:        authclients.EventCorporateStaffInvitation,
		IdempotencyKey:   "staff-invite:" + invitation.ID,
		RequestID:        tenantCtx.RequestID,
		CorrelationID:    tenantCtx.CorrelationID,
	})
	if err != nil {
		_ = s.opts.Invitations.RecordDelivery(ctx, invitation.ID, models.InvitationStatusPending, "")
		s.opts.Metrics.IncCounter("fleet_partner_staff_invitations_total", map[string]string{"outcome": "delivery_failed"}, 1)
		return
	}
	_ = s.opts.Invitations.RecordDelivery(ctx, invitation.ID, "ACCEPTED", reference)
}

// ResendInvitation rotates the token and re-sends the email.
func (s *Service) ResendInvitation(ctx context.Context, tenantCtx tenant.Context, invitationID string) (*models.StaffInvitation, error) {
	invitation, err := s.opts.Invitations.GetByID(ctx, tenantCtx.OrganizationID, invitationID)
	if err != nil {
		if errors.Is(err, staffrepos.ErrNotFound) {
			return nil, apperrors.NotFound("Invitation not found.", nil)
		}
		return nil, err
	}
	if invitation.Status != models.InvitationStatusPending {
		return nil, errorcodes.Conflict(errorcodes.StaffInvitationConsumed, "This invitation is no longer pending.")
	}

	token, err := auth.GenerateOpaqueToken(32)
	if err != nil {
		return nil, err
	}
	expiresAt := s.opts.Now().Add(s.opts.InvitationTTL)
	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Invitations.RotateToken(ctx, tx, tenantCtx.OrganizationID, invitationID,
			auth.HashRefreshToken(s.opts.InvitationSecret, token), expiresAt)
	}); err != nil {
		return nil, errorcodes.Conflict(errorcodes.StaffInvitationConsumed, "This invitation is no longer pending.")
	}
	invitation.ExpiresAt = expiresAt

	s.deliverInvitation(ctx, tenantCtx, invitation, token)
	return invitation, nil
}

// CancelInvitation cancels a pending invitation.
func (s *Service) CancelInvitation(ctx context.Context, tenantCtx tenant.Context, invitationID string) error {
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Invitations.Cancel(ctx, tx, tenantCtx.OrganizationID, invitationID); err != nil {
			if errors.Is(err, staffrepos.ErrInvitationNotUsable) {
				return errorcodes.Conflict(errorcodes.StaffInvitationConsumed, "This invitation is no longer pending.")
			}
			return err
		}
		entry := audit.EntryFromTenant(tenantCtx, "staff.invitation.cancelled", "staff_invitation", invitationID)
		return s.opts.Audit.Record(ctx, tx, entry)
	})
}

// ListInvitations returns the organization's invitations.
func (s *Service) ListInvitations(ctx context.Context, tenantCtx tenant.Context, limit int) ([]models.StaffInvitation, error) {
	return s.opts.Invitations.List(ctx, tenantCtx.OrganizationID, limit)
}
