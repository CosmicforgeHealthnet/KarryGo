package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Identifier prefixes for structure units.
const (
	prefixCostCentre = "cc"
	prefixTeam       = "team"
	prefixLocation   = "oloc"
)

// ErrInUse refuses archiving a unit that live structure still references.
var ErrInUse = errors.New("structure: unit is still referenced by active structure")

// StructureRepository stores cost centres, teams and operating locations.
//
// Every statement is tenant-scoped, and every reference to another unit
// (branch, department, member) is checked to belong to the same
// organization before it is written: a foreign key proves the row exists, not
// that it is this tenant's.
type StructureRepository struct {
	db database.Querier
}

// NewStructureRepository wires the repository.
func NewStructureRepository(db database.Querier) *StructureRepository {
	return &StructureRepository{db: db}
}

// referenceCheck is one same-organization existence check.
type referenceCheck struct {
	table  string
	id     *string
	status string // required status, empty for any non-archived
}

func (r *StructureRepository) checkReferences(ctx context.Context, q database.Querier,
	organizationID string, checks ...referenceCheck) error {
	for _, check := range checks {
		if check.id == nil {
			continue
		}
		statement := `SELECT EXISTS (SELECT 1 FROM ` + check.table + `
			WHERE organization_id = $1 AND id = $2 AND status <> 'ARCHIVED')`
		arguments := []any{organizationID, *check.id}
		if check.status != "" {
			statement = `SELECT EXISTS (SELECT 1 FROM ` + check.table + `
				WHERE organization_id = $1 AND id = $2 AND status = $3)`
			arguments = append(arguments, check.status)
		}
		var exists bool
		if err := q.QueryRow(ctx, statement, arguments...).Scan(&exists); err != nil {
			return fmt.Errorf("structure: reference check: %w", err)
		}
		if !exists {
			return ErrTenantMismatch
		}
	}
	return nil
}

// CostCentreActive reports whether id is an ACTIVE cost centre of the
// organization. Departments use it to validate their cost_centre_id.
func (r *StructureRepository) CostCentreActive(ctx context.Context, q database.Querier,
	organizationID string, id string) error {
	return r.checkReferences(ctx, q, organizationID,
		referenceCheck{table: "fleet_partner_cost_centres", id: &id, status: "ACTIVE"})
}

// ---- cost centres ----

const costCentreColumns = `id, organization_id, branch_id, code, name, budget_owner_user_id,
	status, version, created_at, updated_at, archived_at`

func scanCostCentre(row pgx.Row) (*models.CostCentre, error) {
	var unit models.CostCentre
	err := row.Scan(&unit.ID, &unit.OrganizationID, &unit.BranchID, &unit.Code, &unit.Name,
		&unit.BudgetOwnerUserID, &unit.Status, &unit.Version, &unit.CreatedAt,
		&unit.UpdatedAt, &unit.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cost centres: scan: %w", err)
	}
	return &unit, nil
}

// CreateCostCentre inserts a cost centre.
func (r *StructureRepository) CreateCostCentre(ctx context.Context, q database.Querier,
	organizationID string, unit *models.CostCentre) error {
	if unit.ID == "" {
		unit.ID = ids.New(prefixCostCentre)
	}
	unit.OrganizationID, unit.Status, unit.Version = organizationID, models.UnitStatusActive, 1
	if err := r.checkReferences(ctx, q, organizationID,
		referenceCheck{table: "fleet_partner_branches", id: unit.BranchID},
		referenceCheck{table: "fleet_partner_organization_users", id: unit.BudgetOwnerUserID, status: "ACTIVE"}); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_cost_centres
			(id, organization_id, branch_id, code, name, budget_owner_user_id)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		unit.ID, organizationID, unit.BranchID, unit.Code, unit.Name, unit.BudgetOwnerUserID)
	if err != nil {
		return fmt.Errorf("cost centres: create: %w", err)
	}
	return nil
}

// GetCostCentre loads one cost centre inside the tenant.
func (r *StructureRepository) GetCostCentre(ctx context.Context, organizationID string,
	id string) (*models.CostCentre, error) {
	return scanCostCentre(r.db.QueryRow(ctx, `SELECT `+costCentreColumns+`
		FROM fleet_partner_cost_centres WHERE organization_id = $1 AND id = $2`, organizationID, id))
}

// ListCostCentres returns the organization's cost centres.
func (r *StructureRepository) ListCostCentres(ctx context.Context, organizationID string,
	branchID string, includeArchived bool) ([]models.CostCentre, error) {
	rows, err := r.db.Query(ctx, `SELECT `+costCentreColumns+`
		FROM fleet_partner_cost_centres
		WHERE organization_id = $1 AND ($2 = '' OR branch_id = $2) AND ($3 OR status = 'ACTIVE')
		ORDER BY code, id LIMIT 500`, organizationID, branchID, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("cost centres: list: %w", err)
	}
	defer rows.Close()
	units := []models.CostCentre{}
	for rows.Next() {
		unit, err := scanCostCentre(rows)
		if err != nil {
			return nil, err
		}
		units = append(units, *unit)
	}
	return units, rows.Err()
}

// UpdateCostCentre edits an ACTIVE cost centre (compare-and-swap).
func (r *StructureRepository) UpdateCostCentre(ctx context.Context, q database.Querier,
	organizationID string, unit *models.CostCentre, expectedVersion int64) error {
	if err := r.checkReferences(ctx, q, organizationID,
		referenceCheck{table: "fleet_partner_branches", id: unit.BranchID},
		referenceCheck{table: "fleet_partner_organization_users", id: unit.BudgetOwnerUserID, status: "ACTIVE"}); err != nil {
		return err
	}
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_cost_centres SET
			name = $4, branch_id = $5, budget_owner_user_id = $6,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = 'ACTIVE'`,
		organizationID, unit.ID, expectedVersion, unit.Name, unit.BranchID, unit.BudgetOwnerUserID)
	if err != nil {
		return fmt.Errorf("cost centres: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// ArchiveCostCentre retires a cost centre no active department uses.
func (r *StructureRepository) ArchiveCostCentre(ctx context.Context, q database.Querier,
	organizationID string, id string, expectedVersion int64) error {
	var users int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_departments
		WHERE organization_id = $1 AND cost_centre_id = $2 AND status = 'ACTIVE'`,
		organizationID, id).Scan(&users); err != nil {
		return fmt.Errorf("cost centres: usage check: %w", err)
	}
	if users > 0 {
		return ErrInUse
	}
	return r.archive(ctx, q, "fleet_partner_cost_centres", organizationID, id, expectedVersion, "status = 'ACTIVE'")
}

// ---- teams ----

const teamColumns = `id, organization_id, branch_id, department_id, code, name,
	supervisor_user_id, status, version, created_at, updated_at, archived_at`

func scanTeam(row pgx.Row) (*models.Team, error) {
	var team models.Team
	err := row.Scan(&team.ID, &team.OrganizationID, &team.BranchID, &team.DepartmentID,
		&team.Code, &team.Name, &team.SupervisorUserID, &team.Status, &team.Version,
		&team.CreatedAt, &team.UpdatedAt, &team.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("teams: scan: %w", err)
	}
	return &team, nil
}

func (r *StructureRepository) teamReferences(ctx context.Context, q database.Querier,
	organizationID string, team *models.Team) error {
	if err := r.checkReferences(ctx, q, organizationID,
		referenceCheck{table: "fleet_partner_branches", id: team.BranchID},
		referenceCheck{table: "fleet_partner_departments", id: team.DepartmentID, status: "ACTIVE"},
		referenceCheck{table: "fleet_partner_organization_users", id: team.SupervisorUserID, status: "ACTIVE"}); err != nil {
		return err
	}
	// A team inside a branch-bound department must sit in that branch.
	if team.DepartmentID != nil && team.BranchID != nil {
		var departmentBranch *string
		if err := q.QueryRow(ctx, `SELECT branch_id FROM fleet_partner_departments
			WHERE organization_id = $1 AND id = $2`, organizationID, *team.DepartmentID).
			Scan(&departmentBranch); err != nil {
			return fmt.Errorf("teams: department branch: %w", err)
		}
		if departmentBranch != nil && *departmentBranch != *team.BranchID {
			return ErrTenantMismatch
		}
	}
	return nil
}

// CreateTeam inserts a team.
func (r *StructureRepository) CreateTeam(ctx context.Context, q database.Querier,
	organizationID string, team *models.Team) error {
	if team.ID == "" {
		team.ID = ids.New(prefixTeam)
	}
	team.OrganizationID, team.Status, team.Version = organizationID, models.UnitStatusActive, 1
	if err := r.teamReferences(ctx, q, organizationID, team); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_teams
			(id, organization_id, branch_id, department_id, code, name, supervisor_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		team.ID, organizationID, team.BranchID, team.DepartmentID, team.Code, team.Name,
		team.SupervisorUserID)
	if err != nil {
		return fmt.Errorf("teams: create: %w", err)
	}
	return nil
}

// GetTeam loads one team inside the tenant.
func (r *StructureRepository) GetTeam(ctx context.Context, organizationID string,
	id string) (*models.Team, error) {
	return scanTeam(r.db.QueryRow(ctx, `SELECT `+teamColumns+`
		FROM fleet_partner_teams WHERE organization_id = $1 AND id = $2`, organizationID, id))
}

// TeamFilter narrows a team listing.
type TeamFilter struct {
	BranchID        string
	DepartmentID    string
	IncludeArchived bool
}

// ListTeams returns the organization's teams.
func (r *StructureRepository) ListTeams(ctx context.Context, organizationID string,
	filter TeamFilter) ([]models.Team, error) {
	rows, err := r.db.Query(ctx, `SELECT `+teamColumns+`
		FROM fleet_partner_teams
		WHERE organization_id = $1 AND ($2 = '' OR branch_id = $2)
		  AND ($3 = '' OR department_id = $3) AND ($4 OR status = 'ACTIVE')
		ORDER BY code, id LIMIT 500`, organizationID, filter.BranchID, filter.DepartmentID,
		filter.IncludeArchived)
	if err != nil {
		return nil, fmt.Errorf("teams: list: %w", err)
	}
	defer rows.Close()
	teams := []models.Team{}
	for rows.Next() {
		team, err := scanTeam(rows)
		if err != nil {
			return nil, err
		}
		teams = append(teams, *team)
	}
	return teams, rows.Err()
}

// UpdateTeam edits an ACTIVE team (compare-and-swap).
func (r *StructureRepository) UpdateTeam(ctx context.Context, q database.Querier,
	organizationID string, team *models.Team, expectedVersion int64) error {
	if err := r.teamReferences(ctx, q, organizationID, team); err != nil {
		return err
	}
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_teams SET
			name = $4, branch_id = $5, department_id = $6, supervisor_user_id = $7,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = 'ACTIVE'`,
		organizationID, team.ID, expectedVersion, team.Name, team.BranchID, team.DepartmentID,
		team.SupervisorUserID)
	if err != nil {
		return fmt.Errorf("teams: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// ArchiveTeam retires a team.
func (r *StructureRepository) ArchiveTeam(ctx context.Context, q database.Querier,
	organizationID string, id string, expectedVersion int64) error {
	return r.archive(ctx, q, "fleet_partner_teams", organizationID, id, expectedVersion, "status = 'ACTIVE'")
}

// ---- operating locations ----

const locationColumns = `id, organization_id, branch_id, code, name, location_type, address,
	latitude, longitude, operating_hours, status, version, created_at, updated_at, archived_at`

func scanLocation(row pgx.Row) (*models.OperatingLocation, error) {
	var location models.OperatingLocation
	var address, hours []byte
	err := row.Scan(&location.ID, &location.OrganizationID, &location.BranchID, &location.Code,
		&location.Name, &location.LocationType, &address, &location.Latitude,
		&location.Longitude, &hours, &location.Status, &location.Version,
		&location.CreatedAt, &location.UpdatedAt, &location.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("operating locations: scan: %w", err)
	}
	location.Address, location.OperatingHours = map[string]any{}, map[string]any{}
	_ = json.Unmarshal(address, &location.Address)
	_ = json.Unmarshal(hours, &location.OperatingHours)
	return &location, nil
}

func encodeObject(value map[string]any) []byte {
	if value == nil {
		return []byte("{}")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}

// CreateLocation inserts an operating location.
func (r *StructureRepository) CreateLocation(ctx context.Context, q database.Querier,
	organizationID string, location *models.OperatingLocation) error {
	if location.ID == "" {
		location.ID = ids.New(prefixLocation)
	}
	location.OrganizationID, location.Status, location.Version = organizationID, models.UnitStatusActive, 1
	if err := r.checkReferences(ctx, q, organizationID,
		referenceCheck{table: "fleet_partner_branches", id: location.BranchID}); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_operating_locations
			(id, organization_id, branch_id, code, name, location_type, address,
			 latitude, longitude, operating_hours)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		location.ID, organizationID, location.BranchID, location.Code, location.Name,
		location.LocationType, encodeObject(location.Address), location.Latitude,
		location.Longitude, encodeObject(location.OperatingHours))
	if err != nil {
		return fmt.Errorf("operating locations: create: %w", err)
	}
	return nil
}

// GetLocation loads one operating location inside the tenant.
func (r *StructureRepository) GetLocation(ctx context.Context, organizationID string,
	id string) (*models.OperatingLocation, error) {
	return scanLocation(r.db.QueryRow(ctx, `SELECT `+locationColumns+`
		FROM fleet_partner_operating_locations WHERE organization_id = $1 AND id = $2`,
		organizationID, id))
}

// LocationFilter narrows a location listing.
type LocationFilter struct {
	BranchID        string
	LocationType    string
	IncludeArchived bool
}

// ListLocations returns the organization's operating locations.
func (r *StructureRepository) ListLocations(ctx context.Context, organizationID string,
	filter LocationFilter) ([]models.OperatingLocation, error) {
	rows, err := r.db.Query(ctx, `SELECT `+locationColumns+`
		FROM fleet_partner_operating_locations
		WHERE organization_id = $1 AND ($2 = '' OR branch_id = $2)
		  AND ($3 = '' OR location_type = $3) AND ($4 OR status <> 'ARCHIVED')
		ORDER BY code, id LIMIT 500`, organizationID, filter.BranchID, filter.LocationType,
		filter.IncludeArchived)
	if err != nil {
		return nil, fmt.Errorf("operating locations: list: %w", err)
	}
	defer rows.Close()
	locations := []models.OperatingLocation{}
	for rows.Next() {
		location, err := scanLocation(rows)
		if err != nil {
			return nil, err
		}
		locations = append(locations, *location)
	}
	return locations, rows.Err()
}

// UpdateLocation edits a non-archived operating location (compare-and-swap).
func (r *StructureRepository) UpdateLocation(ctx context.Context, q database.Querier,
	organizationID string, location *models.OperatingLocation, expectedVersion int64) error {
	if err := r.checkReferences(ctx, q, organizationID,
		referenceCheck{table: "fleet_partner_branches", id: location.BranchID}); err != nil {
		return err
	}
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_operating_locations SET
			name = $4, branch_id = $5, location_type = $6, address = $7, latitude = $8,
			longitude = $9, operating_hours = $10, status = $11,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status <> 'ARCHIVED'`,
		organizationID, location.ID, expectedVersion, location.Name, location.BranchID,
		location.LocationType, encodeObject(location.Address), location.Latitude,
		location.Longitude, encodeObject(location.OperatingHours), location.Status)
	if err != nil {
		return fmt.Errorf("operating locations: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// ArchiveLocation retires an operating location.
func (r *StructureRepository) ArchiveLocation(ctx context.Context, q database.Querier,
	organizationID string, id string, expectedVersion int64) error {
	return r.archive(ctx, q, "fleet_partner_operating_locations", organizationID, id,
		expectedVersion, "status <> 'ARCHIVED'")
}

// ---- shared ----

func (r *StructureRepository) archive(ctx context.Context, q database.Querier, table string,
	organizationID string, id string, expectedVersion int64, liveCondition string) error {
	tag, err := q.Exec(ctx, `UPDATE `+table+` SET status = 'ARCHIVED', archived_at = now(),
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND `+liveCondition,
		organizationID, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("structure: archive: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// ActiveUnitsInBranch counts live teams, cost centres and operating locations
// in a branch, so a branch cannot be archived out from under them.
func (r *StructureRepository) ActiveUnitsInBranch(ctx context.Context, q database.Querier,
	organizationID string, branchID string) (int, error) {
	var count int
	err := q.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM fleet_partner_teams
		        WHERE organization_id = $1 AND branch_id = $2 AND status = 'ACTIVE')
		     + (SELECT count(*) FROM fleet_partner_cost_centres
		        WHERE organization_id = $1 AND branch_id = $2 AND status = 'ACTIVE')
		     + (SELECT count(*) FROM fleet_partner_operating_locations
		        WHERE organization_id = $1 AND branch_id = $2 AND status <> 'ARCHIVED')`,
		organizationID, branchID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("structure: branch usage: %w", err)
	}
	return count, nil
}

// ActiveTeamsInDepartment counts live teams inside a department.
func (r *StructureRepository) ActiveTeamsInDepartment(ctx context.Context, q database.Querier,
	organizationID string, departmentID string) (int, error) {
	var count int
	err := q.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_teams
		WHERE organization_id = $1 AND department_id = $2 AND status = 'ACTIVE'`,
		organizationID, departmentID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("structure: department usage: %w", err)
	}
	return count, nil
}
