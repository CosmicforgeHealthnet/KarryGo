package usecases

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// MembershipResolver resolves the tenant scope of an authenticated identity
// from trusted local state. It replaces the Sections 3–7 placeholder that
// always failed closed.
//
// When an identity belongs to several organizations, no single scope can be
// inferred, so it reports ErrMembershipNotFound and the caller must select an
// organization explicitly through /auth/select-organization.
type MembershipResolver struct {
	members       *staffrepos.MemberRepository
	organizations OrganizationReader
}

// NewMembershipResolver builds the resolver.
func NewMembershipResolver(members *staffrepos.MemberRepository, organizations OrganizationReader) *MembershipResolver {
	return &MembershipResolver{members: members, organizations: organizations}
}

// ResolveMembership implements tenant.MembershipResolver.
func (r *MembershipResolver) ResolveMembership(ctx context.Context, subjectID string) (tenant.Membership, error) {
	members, err := r.members.ListForIdentity(ctx, subjectID)
	if err != nil {
		return tenant.Membership{}, err
	}

	live := make([]models.OrganizationUser, 0, len(members))
	for _, member := range members {
		if member.Status == models.UserStatusActive || member.Status == models.UserStatusSuspended {
			live = append(live, member)
		}
	}

	switch len(live) {
	case 0:
		return tenant.Membership{}, tenant.ErrMembershipNotFound
	case 1:
		member := live[0]
		status := tenant.MembershipStatusActive
		if member.Status == models.UserStatusSuspended {
			status = tenant.MembershipStatusSuspended
		}
		return tenant.Membership{
			OrganizationID:     member.OrganizationID,
			OrganizationUserID: member.ID,
			Status:             status,
			PermittedBranchIDs: member.PermittedBranchIDs,
		}, nil
	default:
		// Ambiguous scope: the caller must choose an organization.
		return tenant.Membership{}, tenant.ErrMembershipNotFound
	}
}

// ResolveForOrganization resolves the membership of an identity in one
// specific organization — used by organization-scoped sessions where the
// token already names the organization.
func (r *MembershipResolver) ResolveForOrganization(ctx context.Context, subjectID string, organizationID string) (tenant.Membership, error) {
	member, err := r.members.GetByIdentity(ctx, organizationID, subjectID)
	if err != nil {
		if errors.Is(err, staffrepos.ErrNotFound) {
			return tenant.Membership{}, tenant.ErrMembershipNotFound
		}
		return tenant.Membership{}, err
	}
	status := tenant.MembershipStatusActive
	if member.Status != models.UserStatusActive {
		status = tenant.MembershipStatusSuspended
	}
	return tenant.Membership{
		OrganizationID:     member.OrganizationID,
		OrganizationUserID: member.ID,
		Status:             status,
		PermittedBranchIDs: member.PermittedBranchIDs,
	}, nil
}

// CreateOwnerMembership creates the founding owner membership and binds the
// system owner role. It runs inside the organization-creation transaction.
func (s *Service) CreateOwnerMembership(ctx context.Context, tx pgx.Tx, organizationID string, identityID string, fullName string) (string, error) {
	if fullName == "" {
		fullName = "Organization Owner"
	}
	now := s.opts.Now()
	member := &models.OrganizationUser{
		ID:          ids.New("ousr"),
		IdentityID:  identityID,
		FullName:    fullName,
		Status:      models.UserStatusActive,
		IsOwner:     true,
		RoleVersion: 1,
		ActivatedAt: &now,
	}
	if err := s.opts.Members.Create(ctx, tx, organizationID, member); err != nil {
		return "", err
	}
	if err := s.opts.Roles.CreateBinding(ctx, tx, organizationID, &models.RoleBinding{
		OrganizationUserID: member.ID,
		RoleID:             models.SystemOwnerRoleID,
		ScopeType:          models.ScopeOrganization,
		GrantedBy:          identityID,
	}); err != nil {
		return "", err
	}
	return member.ID, nil
}
