package staffrepositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	staffmodels "cosmicforge/logistics/services/vendor-service/internal/features/staff/models"
)

type PermissionRepository struct {
	db *pgxpool.Pool
}

func NewPermissionRepository(db *pgxpool.Pool) *PermissionRepository {
	return &PermissionRepository{db: db}
}

const permissionColumns = `id, code, description, category, is_high_risk`

func (r *PermissionRepository) List(ctx context.Context) ([]staffmodels.Permission, error) {
	rows, err := r.db.Query(ctx, `SELECT `+permissionColumns+` FROM vendor_permissions ORDER BY category, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []staffmodels.Permission
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PermissionRepository) GetByCode(ctx context.Context, code string) (staffmodels.Permission, error) {
	row := r.db.QueryRow(ctx, `SELECT `+permissionColumns+` FROM vendor_permissions WHERE code = $1`, code)
	p, err := scanPermission(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return staffmodels.Permission{}, fmt.Errorf("permission %s: %w", code, ErrNotFound)
	}
	return p, err
}

// GetManyByIDs resolves a set of permission ids to their full records,
// preserving no particular order — used to validate a custom role's proposed
// permission set (are these ids real, and are any of them high-risk).
func (r *PermissionRepository) GetManyByIDs(ctx context.Context, ids []string) ([]staffmodels.Permission, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `SELECT `+permissionColumns+` FROM vendor_permissions WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []staffmodels.Permission
	for rows.Next() {
		p, err := scanPermission(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanPermission(row pgx.Row) (staffmodels.Permission, error) {
	var p staffmodels.Permission
	err := row.Scan(&p.ID, &p.Code, &p.Description, &p.Category, &p.IsHighRisk)
	return p, err
}
