package usecases

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ListStaff returns memberships for the organization.
func (s *Service) ListStaff(ctx context.Context, query tenant.Query) ([]models.OrganizationUser, error) {
	return s.opts.Members.List(ctx, query)
}

// GetStaff loads one membership inside the tenant scope.
func (s *Service) GetStaff(ctx context.Context, organizationID string, memberID string) (*models.OrganizationUser, error) {
	member, err := s.opts.Members.GetByID(ctx, organizationID, memberID)
	if err != nil {
		if errors.Is(err, staffrepos.ErrNotFound) {
			return nil, apperrors.NotFound("Staff member not found.", nil)
		}
		return nil, err
	}
	return member, nil
}

// UpdateStaffInput changes mutable membership fields.
type UpdateStaffInput struct {
	Tenant          tenant.Context
	MemberID        string
	FullName        string
	JobTitle        *string
	BranchIDs       []string
	DepartmentIDs   []string
	ExpectedVersion int64
}

// UpdateStaff applies a compare-and-swap profile update.
func (s *Service) UpdateStaff(ctx context.Context, input UpdateStaffInput) error {
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		before, err := s.opts.Members.GetByID(ctx, input.Tenant.OrganizationID, input.MemberID)
		if err != nil {
			if errors.Is(err, staffrepos.ErrNotFound) {
				return apperrors.NotFound("Staff member not found.", nil)
			}
			return err
		}
		if err := s.opts.Members.UpdateProfile(ctx, tx, input.Tenant.OrganizationID, input.MemberID,
			input.FullName, input.JobTitle, input.BranchIDs, input.DepartmentIDs, input.ExpectedVersion); err != nil {
			return concurrency.AsConflict(err)
		}
		entry := audit.EntryFromTenant(input.Tenant, "staff.updated", "organization_user", input.MemberID)
		entry.PreviousValues = map[string]any{"fullName": before.FullName, "branchIds": before.PermittedBranchIDs}
		entry.NewValues = map[string]any{"fullName": input.FullName, "branchIds": input.BranchIDs}
		return s.opts.Audit.Record(ctx, tx, entry)
	})
}

// ChangeStatusInput drives suspend / reactivate / end.
type ChangeStatusInput struct {
	Tenant          tenant.Context
	MemberID        string
	Status          string
	ReasonCode      string
	ExpectedVersion int64
}

// ChangeStaffStatus suspends, reactivates or ends a membership.
//
// Suspension and ending revoke the membership's organization-scoped sessions
// in the same transaction, so access stops immediately rather than at the
// next token expiry. The last active owner is protected.
func (s *Service) ChangeStaffStatus(ctx context.Context, input ChangeStatusInput) error {
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		before, err := s.opts.Members.GetByID(ctx, input.Tenant.OrganizationID, input.MemberID)
		if err != nil {
			if errors.Is(err, staffrepos.ErrNotFound) {
				return apperrors.NotFound("Staff member not found.", nil)
			}
			return err
		}

		if err := s.opts.Members.UpdateStatus(ctx, tx, input.Tenant.OrganizationID, input.MemberID,
			input.Status, input.ExpectedVersion); err != nil {
			switch {
			case errors.Is(err, staffrepos.ErrLastOwner):
				return errorcodes.Conflict(errorcodes.LastOwnerRequired,
					"An organization must always keep at least one active owner.")
			case errors.Is(err, staffrepos.ErrNotFound):
				return apperrors.NotFound("Staff member not found.", nil)
			default:
				return concurrency.AsConflict(err)
			}
		}

		if input.Status == models.UserStatusSuspended || input.Status == models.UserStatusEnded {
			if _, err := s.opts.Sessions.RevokeForOrganizationUser(ctx, tx, input.MemberID, "membership_"+input.Status); err != nil {
				return err
			}
			// Bumping the role version invalidates cached authorization for
			// this membership immediately.
			if err := s.opts.Members.BumpRoleVersion(ctx, tx, input.Tenant.OrganizationID, input.MemberID); err != nil {
				return err
			}
		}

		entry := audit.EntryFromTenant(input.Tenant, "staff.status_changed", "organization_user", input.MemberID)
		entry.PreviousValues = map[string]any{"status": before.Status}
		entry.NewValues = map[string]any{"status": input.Status}
		entry.Reason = input.ReasonCode
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}

		eventType := "organization_user.suspended.v1"
		switch input.Status {
		case models.UserStatusActive:
			eventType = "organization_user.reactivated.v1"
		case models.UserStatusEnded:
			eventType = "organization_user.ended.v1"
		}
		return s.opts.Events.Publish(ctx, tx, eventType, "organization_user", input.MemberID,
			before.Version+1, input.Tenant.OrganizationID, input.Tenant.CorrelationID, map[string]any{
				"organizationUserId": input.MemberID,
				"organizationId":     input.Tenant.OrganizationID,
				"status":             input.Status,
			})
	})
}

// ReplaceRoleBindingsInput sets the complete role set of a membership.
type ReplaceRoleBindingsInput struct {
	Tenant   tenant.Context
	MemberID string
	RoleIDs  []string
	// ActorMemberID and ActorRoleVersion drive the escalation check.
	ActorMemberID    string
	ActorRoleVersion int64
}

// PermissionGuard checks that an actor may grant a permission set.
type PermissionGuard interface {
	RequireGrantable(ctx context.Context, organizationID string, actorMemberID string, actorRoleVersion int64, requested []string) error
}

// ReplaceRoleBindings replaces a membership's organization-scoped bindings.
//
// The actor may only grant permissions they themselves hold, so a staff
// member cannot escalate beyond their own authority. The membership's role
// version is bumped so cached decisions and live sessions become stale.
func (s *Service) ReplaceRoleBindings(ctx context.Context, guard PermissionGuard, input ReplaceRoleBindingsInput) error {
	requested := map[string]struct{}{}
	for _, roleID := range input.RoleIDs {
		role, err := s.opts.Roles.GetRole(ctx, input.Tenant.OrganizationID, roleID)
		if err != nil {
			return apperrors.Validation("One or more roles are not available to this organization.",
				[]apperrors.FieldViolation{{Field: "roleIds", Message: "Unknown role."}})
		}
		for _, permission := range role.Permissions {
			requested[permission] = struct{}{}
		}
	}
	permissionList := make([]string, 0, len(requested))
	for permission := range requested {
		permissionList = append(permissionList, permission)
	}
	if err := guard.RequireGrantable(ctx, input.Tenant.OrganizationID, input.ActorMemberID, input.ActorRoleVersion, permissionList); err != nil {
		return err
	}

	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		member, err := s.opts.Members.GetByID(ctx, input.Tenant.OrganizationID, input.MemberID)
		if err != nil {
			if errors.Is(err, staffrepos.ErrNotFound) {
				return apperrors.NotFound("Staff member not found.", nil)
			}
			return err
		}

		existing, err := s.opts.Roles.ListBindings(ctx, input.Tenant.OrganizationID, input.MemberID)
		if err != nil {
			return err
		}
		for _, binding := range existing {
			if _, err := s.opts.Roles.RevokeBinding(ctx, tx, input.Tenant.OrganizationID, binding.ID, "replaced"); err != nil &&
				!errors.Is(err, staffrepos.ErrNotFound) {
				return err
			}
		}
		for _, roleID := range input.RoleIDs {
			if err := s.opts.Roles.CreateBinding(ctx, tx, input.Tenant.OrganizationID, &models.RoleBinding{
				OrganizationUserID: input.MemberID,
				RoleID:             roleID,
				ScopeType:          models.ScopeOrganization,
				GrantedBy:          input.Tenant.ActorID,
			}); err != nil {
				return err
			}
		}
		if err := s.opts.Members.BumpRoleVersion(ctx, tx, input.Tenant.OrganizationID, input.MemberID); err != nil {
			return err
		}

		entry := audit.EntryFromTenant(input.Tenant, "staff.roles_replaced", "organization_user", input.MemberID)
		entry.PreviousValues = map[string]any{"roleVersion": member.RoleVersion}
		entry.NewValues = map[string]any{"roleIds": input.RoleIDs}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "role_binding.created.v1", "organization_user",
			input.MemberID, member.Version, input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{"organizationUserId": input.MemberID, "roleIds": input.RoleIDs})
	})
}

// Roles exposes the role repository for read paths.
func (s *Service) Roles() *staffrepos.RoleRepository { return s.opts.Roles }

// Members exposes the member repository for read paths.
func (s *Service) Members() *staffrepos.MemberRepository { return s.opts.Members }
