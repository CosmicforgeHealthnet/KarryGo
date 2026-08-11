package staffrepositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	staffmodels "cosmicforge/logistics/services/vendor-service/internal/features/staff/models"
)

var ErrDuplicateAssignment = errors.New("staffrepositories: duplicate active assignment")

type AssignmentRepository struct {
	db *pgxpool.Pool
}

func NewAssignmentRepository(db *pgxpool.Pool) *AssignmentRepository {
	return &AssignmentRepository{db: db}
}

const assignmentColumns = `id, staff_id, role_id, branch_id, assigned_by, created_at, revoked_at, revoked_by`

// Create relies on the migration's partial unique index
// (staff_id, role_id, COALESCE(branch_id,”)) WHERE revoked_at IS NULL to
// refuse a duplicate active assignment at the database layer, not just in
// application logic — a race between two concurrent "assign this role"
// requests can only ever produce one active row.
func (r *AssignmentRepository) Create(ctx context.Context, a staffmodels.Assignment) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO vendor_staff_role_assignments (id, staff_id, role_id, branch_id, assigned_by)
		VALUES ($1,$2,$3,$4,$5)`, a.ID, a.StaffID, a.RoleID, a.BranchID, a.AssignedBy)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateAssignment
	}
	return err
}

func (r *AssignmentRepository) Revoke(ctx context.Context, id, revokedBy string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE vendor_staff_role_assignments SET revoked_at = now(), revoked_by = $2
		WHERE id = $1 AND revoked_at IS NULL`, id, revokedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("assignment %s: %w", id, ErrNotFound)
	}
	return nil
}

func (r *AssignmentRepository) GetByID(ctx context.Context, id string) (staffmodels.Assignment, error) {
	row := r.db.QueryRow(ctx, `SELECT `+assignmentColumns+` FROM vendor_staff_role_assignments WHERE id = $1`, id)
	a, err := scanAssignment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return staffmodels.Assignment{}, fmt.Errorf("assignment %s: %w", id, ErrNotFound)
	}
	return a, err
}

func (r *AssignmentRepository) ListActiveForStaff(ctx context.Context, staffID string) ([]staffmodels.Assignment, error) {
	rows, err := r.db.Query(ctx, `SELECT `+assignmentColumns+` FROM vendor_staff_role_assignments
		WHERE staff_id = $1 AND revoked_at IS NULL ORDER BY created_at`, staffID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []staffmodels.Assignment
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// EffectivePermissions resolves the full set of (permission code -> whether
// any active assignment grants it, and at which branch scopes) for a staff
// member in one query, joining assignment -> role -> role_permissions ->
// permissions. A NULL branch_id in the result means "vendor-wide" for that
// permission via that assignment.
type EffectivePermissionRow struct {
	PermissionCode string
	BranchID       *string
	IsHighRisk     bool
}

func (r *AssignmentRepository) EffectivePermissions(ctx context.Context, staffID string) ([]EffectivePermissionRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.code, a.branch_id, p.is_high_risk
		FROM vendor_staff_role_assignments a
		JOIN vendor_role_permissions rp ON rp.role_id = a.role_id
		JOIN vendor_permissions p ON p.id = rp.permission_id
		WHERE a.staff_id = $1 AND a.revoked_at IS NULL`, staffID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EffectivePermissionRow
	for rows.Next() {
		var row EffectivePermissionRow
		if err := rows.Scan(&row.PermissionCode, &row.BranchID, &row.IsHighRisk); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func scanAssignment(row pgx.Row) (staffmodels.Assignment, error) {
	var a staffmodels.Assignment
	err := row.Scan(&a.ID, &a.StaffID, &a.RoleID, &a.BranchID, &a.AssignedBy, &a.CreatedAt, &a.RevokedAt, &a.RevokedBy)
	return a, err
}
