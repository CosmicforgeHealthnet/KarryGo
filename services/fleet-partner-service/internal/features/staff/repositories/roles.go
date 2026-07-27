package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ErrSystemRoleImmutable protects seeded system role templates.
var ErrSystemRoleImmutable = errors.New("staff: system roles cannot be modified")

// RoleRepository stores roles, their permissions and role bindings.
type RoleRepository struct {
	db database.Querier
}

// NewRoleRepository wires the repository.
func NewRoleRepository(db database.Querier) *RoleRepository {
	return &RoleRepository{db: db}
}

// ListPermissions returns the platform permission catalogue.
func (r *RoleRepository) ListPermissions(ctx context.Context) ([]models.Permission, error) {
	rows, err := r.db.Query(ctx,
		`SELECT code, category, description, is_active FROM fleet_partner_permissions ORDER BY category, code`)
	if err != nil {
		return nil, fmt.Errorf("staff: list permissions: %w", err)
	}
	defer rows.Close()

	var permissions []models.Permission
	for rows.Next() {
		var permission models.Permission
		if err := rows.Scan(&permission.Code, &permission.Category, &permission.Description, &permission.IsActive); err != nil {
			return nil, fmt.Errorf("staff: scan permission: %w", err)
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}

// ListRoles returns system templates plus the organization's own roles.
func (r *RoleRepository) ListRoles(ctx context.Context, organizationID string) ([]models.Role, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, code, name, description, is_system, status, version, created_at, updated_at
		FROM fleet_partner_roles
		WHERE (organization_id IS NULL OR organization_id = $1) AND status = 'ACTIVE'
		ORDER BY is_system DESC, code`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("staff: list roles: %w", err)
	}
	defer rows.Close()

	var roles []models.Role
	for rows.Next() {
		var role models.Role
		if err := rows.Scan(&role.ID, &role.OrganizationID, &role.Code, &role.Name, &role.Description,
			&role.IsSystem, &role.Status, &role.Version, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, fmt.Errorf("staff: scan role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for index := range roles {
		permissions, err := r.RolePermissions(ctx, roles[index].ID)
		if err != nil {
			return nil, err
		}
		roles[index].Permissions = permissions
	}
	return roles, nil
}

// GetRole loads a role visible to the organization (its own or a system one).
func (r *RoleRepository) GetRole(ctx context.Context, organizationID string, roleID string) (*models.Role, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, organization_id, code, name, description, is_system, status, version, created_at, updated_at
		FROM fleet_partner_roles
		WHERE id = $1 AND (organization_id IS NULL OR organization_id = $2)`, roleID, organizationID)

	var role models.Role
	err := row.Scan(&role.ID, &role.OrganizationID, &role.Code, &role.Name, &role.Description,
		&role.IsSystem, &role.Status, &role.Version, &role.CreatedAt, &role.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("staff: scan role: %w", err)
	}
	permissions, err := r.RolePermissions(ctx, role.ID)
	if err != nil {
		return nil, err
	}
	role.Permissions = permissions
	return &role, nil
}

// RolePermissions returns the permission codes a role grants.
func (r *RoleRepository) RolePermissions(ctx context.Context, roleID string) ([]string, error) {
	rows, err := r.db.Query(ctx,
		`SELECT permission_code FROM fleet_partner_role_permissions WHERE role_id = $1 ORDER BY permission_code`, roleID)
	if err != nil {
		return nil, fmt.Errorf("staff: role permissions: %w", err)
	}
	defer rows.Close()

	permissions := []string{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("staff: scan role permission: %w", err)
		}
		permissions = append(permissions, code)
	}
	return permissions, rows.Err()
}

// CreateRole inserts a custom organization role with its permissions.
func (r *RoleRepository) CreateRole(ctx context.Context, q database.Querier, organizationID string, role *models.Role, permissions []string) error {
	if role.ID == "" {
		role.ID = ids.New("rol")
	}
	role.OrganizationID = &organizationID
	role.IsSystem = false
	role.Version = 1

	if _, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_roles
			(id, organization_id, code, name, description, is_system, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, false, 'ACTIVE', 1, now(), now())`,
		role.ID, organizationID, role.Code, role.Name, role.Description); err != nil {
		return fmt.Errorf("staff: create role: %w", err)
	}
	return r.replacePermissions(ctx, q, role.ID, permissions)
}

// UpdateRole updates a custom role with compare-and-swap. System roles are
// refused: their platform-defined meaning must not change silently.
func (r *RoleRepository) UpdateRole(ctx context.Context, q database.Querier, organizationID string, roleID string, name string, description string, permissions []string, expectedVersion int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_roles SET
			name = $3, description = $4, version = version + 1, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND is_system = false AND status = 'ACTIVE' AND version = $5`,
		roleID, organizationID, name, description, expectedVersion)
	if err != nil {
		return fmt.Errorf("staff: update role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return r.classifyRoleFailure(ctx, q, organizationID, roleID)
	}
	if permissions != nil {
		return r.replacePermissions(ctx, q, roleID, permissions)
	}
	return nil
}

// ArchiveRole archives a custom role.
func (r *RoleRepository) ArchiveRole(ctx context.Context, q database.Querier, organizationID string, roleID string, expectedVersion int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_roles SET status = 'ARCHIVED', version = version + 1, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND is_system = false AND status = 'ACTIVE' AND version = $3`,
		roleID, organizationID, expectedVersion)
	if err != nil {
		return fmt.Errorf("staff: archive role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return r.classifyRoleFailure(ctx, q, organizationID, roleID)
	}
	return nil
}

func (r *RoleRepository) classifyRoleFailure(ctx context.Context, q database.Querier, organizationID string, roleID string) error {
	var isSystem bool
	err := q.QueryRow(ctx, `SELECT is_system FROM fleet_partner_roles WHERE id = $1`, roleID).Scan(&isSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("staff: classify role failure: %w", err)
	}
	if isSystem {
		return ErrSystemRoleImmutable
	}
	var belongs bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM fleet_partner_roles WHERE id = $1 AND organization_id = $2)`,
		roleID, organizationID).Scan(&belongs); err != nil {
		return fmt.Errorf("staff: classify role ownership: %w", err)
	}
	if !belongs {
		return ErrNotFound
	}
	return concurrency.ErrVersionMismatch
}

func (r *RoleRepository) replacePermissions(ctx context.Context, q database.Querier, roleID string, permissions []string) error {
	if _, err := q.Exec(ctx, `DELETE FROM fleet_partner_role_permissions WHERE role_id = $1`, roleID); err != nil {
		return fmt.Errorf("staff: clear role permissions: %w", err)
	}
	for _, code := range permissions {
		if _, err := q.Exec(ctx, `
			INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
			VALUES ($1, $2) ON CONFLICT DO NOTHING`, roleID, code); err != nil {
			return fmt.Errorf("staff: grant role permission: %w", err)
		}
	}
	return nil
}

// CreateBinding grants a role to a membership within a scope.
func (r *RoleRepository) CreateBinding(ctx context.Context, q database.Querier, organizationID string, binding *models.RoleBinding) error {
	if binding.ID == "" {
		binding.ID = ids.New("rbn")
	}
	binding.OrganizationID = organizationID
	binding.Version = 1
	if binding.ScopeType == "" {
		binding.ScopeType = models.ScopeOrganization
	}
	if binding.EffectiveFrom.IsZero() {
		binding.EffectiveFrom = time.Now().UTC()
	}

	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_role_bindings
			(id, organization_id, organization_user_id, role_id, scope_type, scope_id,
			 effective_from, expires_at, granted_by, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 1, now(), now())`,
		binding.ID, organizationID, binding.OrganizationUserID, binding.RoleID,
		binding.ScopeType, binding.ScopeID, binding.EffectiveFrom, binding.ExpiresAt, binding.GrantedBy)
	if err != nil {
		return fmt.Errorf("staff: create role binding: %w", err)
	}
	return nil
}

// RevokeBinding revokes a binding inside the organization scope.
func (r *RoleRepository) RevokeBinding(ctx context.Context, q database.Querier, organizationID string, bindingID string, reason string) (string, error) {
	var memberID string
	err := q.QueryRow(ctx, `
		UPDATE fleet_partner_role_bindings SET
			revoked_at = now(), revocation_reason = $3, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND revoked_at IS NULL
		RETURNING organization_user_id`,
		organizationID, bindingID, reason).Scan(&memberID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("staff: revoke role binding: %w", err)
	}
	return memberID, nil
}

// ListBindings returns live bindings for a membership (or all memberships
// when memberID is empty), always tenant scoped.
func (r *RoleRepository) ListBindings(ctx context.Context, organizationID string, memberID string) ([]models.RoleBinding, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, organization_user_id, role_id, scope_type, scope_id,
			effective_from, expires_at, granted_by, revoked_at, revocation_reason, version, created_at, updated_at
		FROM fleet_partner_role_bindings
		WHERE organization_id = $1 AND ($2 = '' OR organization_user_id = $2) AND revoked_at IS NULL
		ORDER BY created_at DESC, id`, organizationID, memberID)
	if err != nil {
		return nil, fmt.Errorf("staff: list bindings: %w", err)
	}
	defer rows.Close()

	var bindings []models.RoleBinding
	for rows.Next() {
		var binding models.RoleBinding
		if err := rows.Scan(&binding.ID, &binding.OrganizationID, &binding.OrganizationUserID,
			&binding.RoleID, &binding.ScopeType, &binding.ScopeID, &binding.EffectiveFrom,
			&binding.ExpiresAt, &binding.GrantedBy, &binding.RevokedAt, &binding.RevocationReason,
			&binding.Version, &binding.CreatedAt, &binding.UpdatedAt); err != nil {
			return nil, fmt.Errorf("staff: scan binding: %w", err)
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

// EffectivePermissions computes the permission set a membership holds right
// now, honouring binding expiry and revocation. Deny-by-default: an empty
// result means no authority.
func (r *RoleRepository) EffectivePermissions(ctx context.Context, organizationID string, memberID string) (map[string][]models.RoleBinding, error) {
	rows, err := r.db.Query(ctx, `
		SELECT rp.permission_code, rb.id, rb.scope_type, rb.scope_id, rb.effective_from, rb.expires_at
		FROM fleet_partner_role_bindings rb
		JOIN fleet_partner_role_permissions rp ON rp.role_id = rb.role_id
		JOIN fleet_partner_roles r ON r.id = rb.role_id
		WHERE rb.organization_id = $1
		  AND rb.organization_user_id = $2
		  AND rb.revoked_at IS NULL
		  AND rb.effective_from <= now()
		  AND (rb.expires_at IS NULL OR rb.expires_at > now())
		  AND r.status = 'ACTIVE'`, organizationID, memberID)
	if err != nil {
		return nil, fmt.Errorf("staff: effective permissions: %w", err)
	}
	defer rows.Close()

	permissions := map[string][]models.RoleBinding{}
	for rows.Next() {
		var code string
		var binding models.RoleBinding
		if err := rows.Scan(&code, &binding.ID, &binding.ScopeType, &binding.ScopeID,
			&binding.EffectiveFrom, &binding.ExpiresAt); err != nil {
			return nil, fmt.Errorf("staff: scan effective permission: %w", err)
		}
		permissions[code] = append(permissions[code], binding)
	}
	return permissions, rows.Err()
}
