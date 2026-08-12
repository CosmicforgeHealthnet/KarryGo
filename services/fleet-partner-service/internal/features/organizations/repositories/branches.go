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
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// BranchRepository stores branches with mandatory tenant filtering.
type BranchRepository struct {
	db database.Querier
}

// NewBranchRepository wires the repository.
func NewBranchRepository(db database.Querier) *BranchRepository {
	return &BranchRepository{db: db}
}

// Create inserts a branch for the given organization.
func (r *BranchRepository) Create(ctx context.Context, q database.Querier, organizationID string, branch *models.Branch) error {
	if organizationID == "" {
		return fmt.Errorf("branches: organization id is required")
	}
	if err := ValidateTimezone(branch.Timezone); err != nil {
		return err
	}
	if branch.ID == "" {
		branch.ID = ids.New(ids.PrefixBranch)
	}
	if branch.Status == "" {
		branch.Status = models.BranchStatusActive
	}
	branch.OrganizationID = organizationID
	branch.Version = 1
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_branches
			(id, organization_id, code, name, type, address, latitude, longitude,
			 geofence_reference, operating_hours, timezone, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''), $11, $12, $13, now(), now())`,
		branch.ID, organizationID, branch.Code, branch.Name, branch.Type, branch.Address,
		branch.Latitude, branch.Longitude, branch.GeofenceReference, branch.OperatingHours,
		branch.Timezone, branch.Status, branch.Version,
	)
	if err != nil {
		return fmt.Errorf("branches: create: %w", err)
	}
	return nil
}

// GetByID loads one branch inside the organization scope.
func (r *BranchRepository) GetByID(ctx context.Context, organizationID string, branchID string) (*models.Branch, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, organization_id, code, name, type, address, latitude, longitude,
			COALESCE(geofence_reference, ''), COALESCE(operating_hours, ''), timezone,
			status, version, created_at, updated_at, archived_at
		FROM fleet_partner_branches
		WHERE organization_id = $1 AND id = $2`,
		organizationID, branchID,
	)
	return scanBranch(row)
}

// List returns branches for a tenant query with stable ordering and a
// deterministic ID tie-breaker. When the tenant context restricts the actor
// to specific branches, only those branches are returned — enforced in SQL,
// independent of any user-supplied filter.
func (r *BranchRepository) List(ctx context.Context, query tenant.Query) ([]models.Branch, error) {
	if query.OrganizationID == "" {
		return nil, fmt.Errorf("branches: organization id is required")
	}
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, code, name, type, address, latitude, longitude,
			COALESCE(geofence_reference, ''), COALESCE(operating_hours, ''), timezone,
			status, version, created_at, updated_at, archived_at
		FROM fleet_partner_branches
		WHERE organization_id = $1
		  AND (cardinality($2::text[]) = 0 OR id = ANY($2::text[]))
		ORDER BY created_at, id
		LIMIT $3`,
		query.OrganizationID, permittedBranchArray(query.PermittedBranchIDs), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("branches: list: %w", err)
	}
	defer rows.Close()

	var branches []models.Branch
	for rows.Next() {
		branch, err := scanBranchRows(rows)
		if err != nil {
			return nil, err
		}
		branches = append(branches, *branch)
	}
	return branches, rows.Err()
}

// Archive performs a tenant-scoped compare-and-swap archive.
func (r *BranchRepository) Archive(ctx context.Context, organizationID string, branchID string, expectedVersion int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_branches SET
			status = $1, archived_at = now(), version = version + 1, updated_at = now()
		WHERE organization_id = $2 AND id = $3 AND version = $4`,
		models.BranchStatusArchived, organizationID, branchID, expectedVersion,
	)
	if err != nil {
		return fmt.Errorf("branches: archive: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either not in this organization (report not found) or version moved.
		exists, err := r.existsInTenant(ctx, organizationID, branchID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// permittedBranchArray normalizes the permitted-branch list for SQL binding;
// an empty list means "all branches of the organization".
func permittedBranchArray(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

func (r *BranchRepository) existsInTenant(ctx context.Context, organizationID string, branchID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM fleet_partner_branches WHERE organization_id = $1 AND id = $2
		)`,
		organizationID, branchID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("branches: exists: %w", err)
	}
	return exists, nil
}

func scanBranch(row pgx.Row) (*models.Branch, error) {
	var branch models.Branch
	err := row.Scan(
		&branch.ID, &branch.OrganizationID, &branch.Code, &branch.Name, &branch.Type,
		&branch.Address, &branch.Latitude, &branch.Longitude, &branch.GeofenceReference,
		&branch.OperatingHours, &branch.Timezone, &branch.Status, &branch.Version,
		&branch.CreatedAt, &branch.UpdatedAt, &branch.ArchivedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("branches: scan: %w", err)
	}
	return &branch, nil
}

func scanBranchRows(rows pgx.Rows) (*models.Branch, error) {
	var branch models.Branch
	err := rows.Scan(
		&branch.ID, &branch.OrganizationID, &branch.Code, &branch.Name, &branch.Type,
		&branch.Address, &branch.Latitude, &branch.Longitude, &branch.GeofenceReference,
		&branch.OperatingHours, &branch.Timezone, &branch.Status, &branch.Version,
		&branch.CreatedAt, &branch.UpdatedAt, &branch.ArchivedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("branches: scan: %w", err)
	}
	return &branch, nil
}
