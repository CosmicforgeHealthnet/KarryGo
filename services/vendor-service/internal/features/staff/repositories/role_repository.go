package staffrepositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	staffmodels "cosmicforge/logistics/services/vendor-service/internal/features/staff/models"
)

type RoleRepository struct {
	db *pgxpool.Pool
}

func NewRoleRepository(db *pgxpool.Pool) *RoleRepository {
	return &RoleRepository{db: db}
}

const roleColumns = `id, vendor_id, code, name, description, is_system, version, created_at, updated_at`

// ListVisibleToVendor returns every system role template (vendor_id IS NULL)
// plus that vendor's own custom roles — never another vendor's custom roles.
func (r *RoleRepository) ListVisibleToVendor(ctx context.Context, vendorID string) ([]staffmodels.Role, error) {
	rows, err := r.db.Query(ctx, `SELECT `+roleColumns+` FROM vendor_roles
		WHERE vendor_id IS NULL OR vendor_id = $1 ORDER BY is_system DESC, name`, vendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []staffmodels.Role
	for rows.Next() {
		role, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

// GetVisibleToVendor loads one role by id, only if it is a system template or
// belongs to vendorID — a vendor cannot read another vendor's custom role,
// full stop.
func (r *RoleRepository) GetVisibleToVendor(ctx context.Context, vendorID, id string) (staffmodels.Role, error) {
	row := r.db.QueryRow(ctx, `SELECT `+roleColumns+` FROM vendor_roles
		WHERE id = $1 AND (vendor_id IS NULL OR vendor_id = $2)`, id, vendorID)
	role, err := scanRole(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return staffmodels.Role{}, fmt.Errorf("role %s: %w", id, ErrNotFound)
	}
	return role, err
}

func (r *RoleRepository) GetByID(ctx context.Context, id string) (staffmodels.Role, error) {
	row := r.db.QueryRow(ctx, `SELECT `+roleColumns+` FROM vendor_roles WHERE id = $1`, id)
	role, err := scanRole(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return staffmodels.Role{}, fmt.Errorf("role %s: %w", id, ErrNotFound)
	}
	return role, err
}

// CreateCustom creates a vendor-scoped custom role. Never usable to create a
// system role (VendorID is always required by the caller/usecase).
func (r *RoleRepository) CreateCustom(ctx context.Context, role staffmodels.Role) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO vendor_roles (id, vendor_id, code, name, description, is_system)
		VALUES ($1,$2,$3,$4,$5,false)`, role.ID, role.VendorID, role.Code, role.Name, role.Description)
	return err
}

// UpdateCustom version-guards edits to a vendor's own custom role. System
// roles are never editable through this path (the usecase never calls it for
// one).
func (r *RoleRepository) UpdateCustom(ctx context.Context, vendorID, id, name, description string, expectedVersion int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE vendor_roles SET name = $1, description = $2, updated_at = now(), version = version + 1
		WHERE id = $3 AND vendor_id = $4 AND is_system = false AND version = $5`,
		name, description, id, vendorID, expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (r *RoleRepository) PermissionCodesForRole(ctx context.Context, roleID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.code FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = $1 ORDER BY p.code`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}

// SetPermissions replaces a custom role's full permission set atomically
// (delete then insert, in one transaction) and bumps version — so any cached
// or in-flight authorization decision keyed on the old version is naturally
// invalidated (see fleet-partner-service's identical roleVersion-in-cache-key
// pattern, referenced in docs/architecture/09).
func (r *RoleRepository) SetPermissions(ctx context.Context, vendorID, roleID string, permissionIDs []string, expectedVersion int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE vendor_roles SET updated_at = now(), version = version + 1
		WHERE id = $1 AND vendor_id = $2 AND is_system = false AND version = $3`,
		roleID, vendorID, expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}

	if _, err := tx.Exec(ctx, `DELETE FROM vendor_role_permissions WHERE role_id = $1`, roleID); err != nil {
		return err
	}
	for _, permID := range permissionIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO vendor_role_permissions (role_id, permission_id) VALUES ($1,$2)`, roleID, permID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func scanRole(row pgx.Row) (staffmodels.Role, error) {
	var role staffmodels.Role
	err := row.Scan(&role.ID, &role.VendorID, &role.Code, &role.Name, &role.Description, &role.IsSystem, &role.Version, &role.CreatedAt, &role.UpdatedAt)
	return role, err
}
