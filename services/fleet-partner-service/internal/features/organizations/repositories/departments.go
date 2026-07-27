package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
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
