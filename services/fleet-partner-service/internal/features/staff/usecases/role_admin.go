package usecases

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Custom role administration (spec Sections 9 and 38).
//
// Custom roles are organization-owned bundles of catalogue permissions.
// System roles are platform templates and are never edited. Every write
// passes the same escalation guard used for role bindings: an actor may
// only put into a role permissions that they currently hold themselves,
// otherwise "create a role with everything, then bind it to myself" would
// be a privilege escalation path.

var roleCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,62}$`)

// RoleInput carries the editable fields of a custom role.
type RoleInput struct {
	Tenant           tenant.Context
	RoleID           string
	Code             string
	Name             string
	Description      string
	Permissions      []string
	ExpectedVersion  int64
	ActorMemberID    string
	ActorRoleVersion int64
}

func (s *Service) validatePermissions(ctx context.Context, guard PermissionGuard, input RoleInput) ([]string, error) {
	if len(input.Permissions) == 0 {
		return nil, apperrors.Validation("A role must grant at least one permission.",
			[]apperrors.FieldViolation{{Field: "permissions", Message: "Select at least one permission."}})
	}
	catalogue, err := s.opts.Roles.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(catalogue))
	for _, permission := range catalogue {
		known[permission.Code] = struct{}{}
	}
	seen := map[string]struct{}{}
	cleaned := make([]string, 0, len(input.Permissions))
	for _, raw := range input.Permissions {
		code := strings.TrimSpace(raw)
		if _, ok := known[code]; !ok {
			return nil, apperrors.Validation("One or more permissions are not in the permission catalogue.",
				[]apperrors.FieldViolation{{Field: "permissions", Message: "Unknown permission: " + code}})
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		cleaned = append(cleaned, code)
	}
	if err := guard.RequireGrantable(ctx, input.Tenant.OrganizationID, input.ActorMemberID, input.ActorRoleVersion, cleaned); err != nil {
		return nil, err
	}
	return cleaned, nil
}

func validateRoleName(name string) error {
	if strings.TrimSpace(name) == "" || len(name) > 120 {
		return apperrors.Validation("Role name is required (at most 120 characters).",
			[]apperrors.FieldViolation{{Field: "name", Message: "Required."}})
	}
	return nil
}

// CreateRole creates an organization custom role.
func (s *Service) CreateRole(ctx context.Context, guard PermissionGuard, input RoleInput) (*models.Role, error) {
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	if !roleCodePattern.MatchString(code) {
		return nil, apperrors.Validation("Role code must be 2-63 upper-case letters, digits or underscores.",
			[]apperrors.FieldViolation{{Field: "code", Message: "Invalid code."}})
	}
	if err := validateRoleName(input.Name); err != nil {
		return nil, err
	}
	permissions, err := s.validatePermissions(ctx, guard, input)
	if err != nil {
		return nil, err
	}
	role := &models.Role{Code: code, Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description)}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Roles.CreateRole(ctx, tx, input.Tenant.OrganizationID, role, permissions); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return apperrors.Conflict("A role with this code already exists.", nil)
			}
			return err
		}
		entry := audit.EntryFromTenant(input.Tenant, "role.created", "role", role.ID)
		entry.NewValues = map[string]any{"code": code, "name": role.Name, "permissions": permissions}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "role.created.v1", "role", role.ID, 1,
			input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{"roleId": role.ID, "code": code, "permissions": permissions})
	})
	if err != nil {
		return nil, err
	}
	return s.opts.Roles.GetRole(ctx, input.Tenant.OrganizationID, role.ID)
}

func roleWriteError(err error) error {
	switch {
	case errors.Is(err, staffrepos.ErrNotFound):
		return apperrors.NotFound("Role not found.", nil)
	case errors.Is(err, staffrepos.ErrSystemRoleImmutable):
		return errorcodes.Conflict(errorcodes.RoleIsSystemManaged, "System roles cannot be changed.")
	default:
		return concurrency.AsConflict(err)
	}
}

// UpdateRole edits a custom role (compare-and-swap) and invalidates the
// cached authorization of every holder, because the meaning of their
// bindings changed.
func (s *Service) UpdateRole(ctx context.Context, guard PermissionGuard, input RoleInput) (*models.Role, error) {
	if err := validateRoleName(input.Name); err != nil {
		return nil, err
	}
	permissions, err := s.validatePermissions(ctx, guard, input)
	if err != nil {
		return nil, err
	}
	before, err := s.opts.Roles.GetRole(ctx, input.Tenant.OrganizationID, input.RoleID)
	if err != nil {
		return nil, roleWriteError(err)
	}
	expected := input.ExpectedVersion
	if expected == 0 {
		expected = before.Version
	}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Roles.UpdateRole(ctx, tx, input.Tenant.OrganizationID, input.RoleID,
			strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), permissions, expected); err != nil {
			return roleWriteError(err)
		}
		if err := s.bumpHolders(ctx, tx, input.Tenant.OrganizationID, input.RoleID); err != nil {
			return err
		}
		entry := audit.EntryFromTenant(input.Tenant, "role.updated", "role", input.RoleID)
		entry.PreviousValues = map[string]any{"name": before.Name, "permissions": before.Permissions}
		entry.NewValues = map[string]any{"name": input.Name, "permissions": permissions}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "role.updated.v1", "role", input.RoleID, expected+1,
			input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{"roleId": input.RoleID, "permissions": permissions})
	})
	if err != nil {
		return nil, err
	}
	return s.opts.Roles.GetRole(ctx, input.Tenant.OrganizationID, input.RoleID)
}

// ArchiveRole retires a custom role. A role that is still bound to anyone
// is refused: silently stripping authority from staff mid-shift is worse
// than asking the administrator to rebind them first.
func (s *Service) ArchiveRole(ctx context.Context, input RoleInput) error {
	before, err := s.opts.Roles.GetRole(ctx, input.Tenant.OrganizationID, input.RoleID)
	if err != nil {
		return roleWriteError(err)
	}
	expected := input.ExpectedVersion
	if expected == 0 {
		expected = before.Version
	}
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		var bound int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM fleet_partner_role_bindings
			WHERE organization_id = $1 AND role_id = $2 AND revoked_at IS NULL
			  AND (expires_at IS NULL OR expires_at > now())`,
			input.Tenant.OrganizationID, input.RoleID).Scan(&bound); err != nil {
			return err
		}
		if bound > 0 {
			return apperrors.Conflict("This role is still assigned to staff. Remove it from them before archiving.", nil)
		}
		if err := s.opts.Roles.ArchiveRole(ctx, tx, input.Tenant.OrganizationID, input.RoleID, expected); err != nil {
			return roleWriteError(err)
		}
		entry := audit.EntryFromTenant(input.Tenant, "role.archived", "role", input.RoleID)
		entry.PreviousValues = map[string]any{"status": before.Status}
		entry.NewValues = map[string]any{"status": "ARCHIVED"}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "role.archived.v1", "role", input.RoleID, expected+1,
			input.Tenant.OrganizationID, input.Tenant.CorrelationID, map[string]any{"roleId": input.RoleID})
	})
}

func (s *Service) bumpHolders(ctx context.Context, tx pgx.Tx, organizationID string, roleID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE fleet_partner_organization_users SET role_version = role_version + 1, updated_at = now()
		WHERE organization_id = $1 AND id IN (
			SELECT organization_user_id FROM fleet_partner_role_bindings
			WHERE organization_id = $1 AND role_id = $2 AND revoked_at IS NULL)`,
		organizationID, roleID)
	return err
}

// BindingSpec describes one role grant with an optional narrower scope and
// an optional expiry (spec 9: branch/department scope, role expiry).
type BindingSpec struct {
	RoleID    string
	ScopeType string
	ScopeID   string
	ExpiresAt *time.Time
}

// ListRoleBindings returns the live bindings of one membership.
func (s *Service) ListRoleBindings(ctx context.Context, organizationID string, memberID string) ([]models.RoleBinding, error) {
	if _, err := s.GetStaff(ctx, organizationID, memberID); err != nil {
		return nil, err
	}
	return s.opts.Roles.ListBindings(ctx, organizationID, memberID)
}

// validateBindingSpecs checks scope references stay inside the tenant.
func (s *Service) validateBindingSpecs(ctx context.Context, organizationID string, specs []BindingSpec) error {
	now := s.opts.Now()
	for index := range specs {
		spec := &specs[index]
		spec.ScopeType = strings.ToUpper(strings.TrimSpace(spec.ScopeType))
		if spec.ScopeType == "" {
			spec.ScopeType = models.ScopeOrganization
		}
		if spec.ExpiresAt != nil && !spec.ExpiresAt.After(now) {
			return apperrors.Validation("A role expiry must be in the future.",
				[]apperrors.FieldViolation{{Field: "bindings.expiresAt", Message: "Must be in the future."}})
		}
		var table string
		switch spec.ScopeType {
		case models.ScopeOrganization:
			spec.ScopeID = ""
			continue
		case models.ScopeBranch:
			table = "fleet_partner_branches"
		case models.ScopeDepartment:
			table = "fleet_partner_departments"
		default:
			return apperrors.New(http.StatusUnprocessableEntity, errorcodes.RoleScopeInvalid, "Scope must be ORGANIZATION, BRANCH or DEPARTMENT.", nil)
		}
		if spec.ScopeID == "" {
			return apperrors.Validation("A branch or department scope needs its id.",
				[]apperrors.FieldViolation{{Field: "bindings.scopeId", Message: "Required."}})
		}
		var exists bool
		if err := s.opts.Pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM `+table+` WHERE organization_id = $1 AND id = $2 AND status = 'ACTIVE')`,
			organizationID, spec.ScopeID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			// Never confirm an id that belongs to another organization.
			return apperrors.Validation("The selected scope is not available to this organization.",
				[]apperrors.FieldViolation{{Field: "bindings.scopeId", Message: "Unknown scope."}})
		}
	}
	return nil
}
