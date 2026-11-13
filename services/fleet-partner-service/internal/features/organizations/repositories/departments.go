package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// DepartmentRepository stores departments with tenant filtering and
// same-organization referential checks for branch and parent.
type DepartmentRepository struct {
	db database.Querier
}

// NewDepartmentRepository wires the repository.
func NewDepartmentRepository(db database.Querier) *DepartmentRepository {
	return &DepartmentRepository{db: db}
}

// Create inserts a department after validating that any referenced branch
// and parent department belong to the same organization and that the parent
// is not the department itself.
func (r *DepartmentRepository) Create(ctx context.Context, q database.Querier, organizationID string, department *models.Department) error {
	if organizationID == "" {
		return fmt.Errorf("departments: organization id is required")
	}
	if department.ID == "" {
		department.ID = ids.New(ids.PrefixDepartment)
	}
	if department.Status == "" {
		department.Status = models.DepartmentStatusActive
	}
	department.OrganizationID = organizationID
	department.Version = 1

	if department.ParentID != nil && *department.ParentID == department.ID {
		return fmt.Errorf("departments: a department cannot be its own parent")
	}
	if department.BranchID != nil {
		var exists bool
		if err := q.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM fleet_partner_branches WHERE organization_id = $1 AND id = $2)`,
			organizationID, *department.BranchID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("departments: check branch: %w", err)
		}
		if !exists {
			return ErrTenantMismatch
		}
	}
	if department.ParentID != nil {
		var exists bool
		if err := q.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM fleet_partner_departments WHERE organization_id = $1 AND id = $2)`,
			organizationID, *department.ParentID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("departments: check parent: %w", err)
		}
		if !exists {
			return ErrTenantMismatch
		}
	}

	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_departments
			(id, organization_id, branch_id, parent_id, cost_centre_id, code, name,
			 status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now())`,
		department.ID, organizationID, department.BranchID, department.ParentID,
		department.CostCentreID, department.Code, department.Name,
		department.Status, department.Version,
	)
	if err != nil {
		return fmt.Errorf("departments: create: %w", err)
	}
	return nil
}

// GetByID loads one department inside the organization scope.
func (r *DepartmentRepository) GetByID(ctx context.Context, organizationID string, departmentID string) (*models.Department, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, organization_id, branch_id, parent_id, cost_centre_id, code, name,
			status, version, created_at, updated_at, archived_at
		FROM fleet_partner_departments
		WHERE organization_id = $1 AND id = $2`,
		organizationID, departmentID,
	)
	var department models.Department
	err := row.Scan(
		&department.ID, &department.OrganizationID, &department.BranchID, &department.ParentID,
		&department.CostCentreID, &department.Code, &department.Name, &department.Status,
		&department.Version, &department.CreatedAt, &department.UpdatedAt, &department.ArchivedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("departments: scan: %w", err)
	}
	return &department, nil
}

const departmentColumns = `id, organization_id, branch_id, parent_id, cost_centre_id, code, name,
	status, version, created_at, updated_at, archived_at`

func scanDepartment(row pgx.Row) (*models.Department, error) {
	var department models.Department
	err := row.Scan(
		&department.ID, &department.OrganizationID, &department.BranchID, &department.ParentID,
		&department.CostCentreID, &department.Code, &department.Name, &department.Status,
		&department.Version, &department.CreatedAt, &department.UpdatedAt, &department.ArchivedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("departments: scan: %w", err)
	}
	return &department, nil
}

// List returns the organization's departments, optionally for one branch.
func (r *DepartmentRepository) List(ctx context.Context, organizationID string, branchID string, includeArchived bool) ([]models.Department, error) {
	rows, err := r.db.Query(ctx, `SELECT `+departmentColumns+`
		FROM fleet_partner_departments
		WHERE organization_id = $1 AND ($2 = '' OR branch_id = $2) AND ($3 OR status = 'ACTIVE')
		ORDER BY name, id LIMIT 500`, organizationID, branchID, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("departments: list: %w", err)
	}
	defer rows.Close()
	departments := []models.Department{}
	for rows.Next() {
		department, err := scanDepartment(rows)
		if err != nil {
			return nil, err
		}
		departments = append(departments, *department)
	}
	return departments, rows.Err()
}

// Update renames or re-parents a department (compare-and-swap). The parent
// chain must stay inside the organization and must not form a cycle.
func (r *DepartmentRepository) Update(ctx context.Context, q database.Querier, organizationID string, department *models.Department, expectedVersion int64) error {
	if department.ParentID != nil {
		var cycle bool
		if err := q.QueryRow(ctx, `
			WITH RECURSIVE chain(id, parent_id) AS (
				SELECT id, parent_id FROM fleet_partner_departments WHERE organization_id = $1 AND id = $2
				UNION ALL
				SELECT d.id, d.parent_id FROM fleet_partner_departments d
				JOIN chain c ON d.id = c.parent_id WHERE d.organization_id = $1
			)
			SELECT EXISTS (SELECT 1 FROM chain WHERE id = $3)`,
			organizationID, *department.ParentID, department.ID).Scan(&cycle); err != nil {
			return fmt.Errorf("departments: cycle check: %w", err)
		}
		if cycle {
			return ErrTenantMismatch
		}
	}
	for _, check := range []struct {
		table string
		id    *string
	}{{"fleet_partner_branches", department.BranchID}, {"fleet_partner_departments", department.ParentID}} {
		if check.id == nil {
			continue
		}
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+check.table+` WHERE organization_id = $1 AND id = $2)`,
			organizationID, *check.id).Scan(&exists); err != nil {
			return fmt.Errorf("departments: reference check: %w", err)
		}
		if !exists {
			return ErrTenantMismatch
		}
	}
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_departments SET
			name = $4, branch_id = $5, parent_id = $6, cost_centre_id = $7,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = 'ACTIVE'`,
		organizationID, department.ID, expectedVersion, department.Name,
		department.BranchID, department.ParentID, department.CostCentreID)
	if err != nil {
		return fmt.Errorf("departments: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetByID(ctx, organizationID, department.ID); err != nil {
			return err
		}
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// Archive retires a department that has no active child departments.
func (r *DepartmentRepository) Archive(ctx context.Context, q database.Querier, organizationID string, departmentID string, expectedVersion int64) error {
	var children int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_departments
		WHERE organization_id = $1 AND parent_id = $2 AND status = 'ACTIVE'`, organizationID, departmentID).Scan(&children); err != nil {
		return fmt.Errorf("departments: child check: %w", err)
	}
	if children > 0 {
		return ErrHasActiveChildren
	}
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_departments SET status = 'ARCHIVED', archived_at = now(),
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = 'ACTIVE'`,
		organizationID, departmentID, expectedVersion)
	if err != nil {
		return fmt.Errorf("departments: archive: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetByID(ctx, organizationID, departmentID); err != nil {
			return err
		}
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// ErrHasActiveChildren refuses archiving a department with live children.
var ErrHasActiveChildren = errors.New("departments: department has active child departments")
