package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Cost centres, teams and operating locations (spec 37.1/37.3, 57.1): the
// rest of the organization structure alongside branches and departments.
// Every unit carries organization_id, every reference is checked to belong to
// the same organization, and every write is one transaction with its audit
// record and outbox event.

// maxStructureJSONBytes bounds address and operating-hours documents.
const maxStructureJSONBytes = 4096

func (s *Service) structureUnits() *orgrepos.StructureRepository {
	return orgrepos.NewStructureRepository(s.opts.Pool)
}

func costCentreReferenceError(err error) error {
	if errors.Is(err, orgrepos.ErrTenantMismatch) {
		return apperrors.Validation("The cost centre is not an active cost centre of this organization.",
			[]apperrors.FieldViolation{{Field: "costCentreId", Message: "Unknown or archived cost centre."}})
	}
	return err
}

func unitError(err error, what string) error {
	switch {
	case errors.Is(err, orgrepos.ErrInUse):
		return apperrors.Conflict(what+" is still in use by active structure.", nil)
	case errors.Is(err, orgrepos.ErrTenantMismatch):
		return apperrors.Validation("A referenced branch, department or staff member is not available to this organization.",
			[]apperrors.FieldViolation{{Field: "reference", Message: "Unknown, archived or inactive reference."}})
	}
	return structureError(err, what)
}

func structureCode(code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !structureCodePattern.MatchString(code) {
		return "", apperrors.Validation("Code must be 1-32 letters, digits, '-' or '_'.",
			[]apperrors.FieldViolation{{Field: "code", Message: "Invalid code."}})
	}
	return code, nil
}

func structureName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return "", apperrors.Validation("Name is required (at most 120 characters).",
			[]apperrors.FieldViolation{{Field: "name", Message: "Required (at most 120 characters)."}})
	}
	return name, nil
}

func (s *Service) recordStructureChange(ctx context.Context, tx pgx.Tx, tenantCtx tenant.Context,
	action string, resourceType string, resourceID string, version int64,
	previous map[string]any, next map[string]any) error {
	entry := audit.EntryFromTenant(tenantCtx, action, resourceType, resourceID)
	entry.PreviousValues = previous
	entry.NewValues = next
	if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
		return err
	}
	payload := map[string]any{"id": resourceID, "organizationId": tenantCtx.OrganizationID}
	for key, value := range next {
		payload[key] = value
	}
	return s.opts.Events.Publish(ctx, tx, action+".v1", resourceType, resourceID, version,
		tenantCtx.OrganizationID, tenantCtx.CorrelationID, payload)
}

// ---- cost centres ----

// CostCentreInput carries cost-centre fields. On update, nil pointers keep
// the current value and an empty string clears an optional reference.
type CostCentreInput struct {
	Tenant            tenant.Context
	CostCentreID      string
	Code              string
	Name              string
	BranchID          *string
	BudgetOwnerUserID *string
	ExpectedVersion   int64
}

// ListCostCentres returns the organization's cost centres.
func (s *Service) ListCostCentres(ctx context.Context, organizationID string, branchID string,
	includeArchived bool) ([]models.CostCentre, error) {
	return s.structureUnits().ListCostCentres(ctx, organizationID, branchID, includeArchived)
}

// GetCostCentre loads one cost centre.
func (s *Service) GetCostCentre(ctx context.Context, organizationID string, id string) (*models.CostCentre, error) {
	unit, err := s.structureUnits().GetCostCentre(ctx, organizationID, id)
	return unit, structureError(err, "Cost centre")
}

// CreateCostCentre adds a cost centre to the register.
func (s *Service) CreateCostCentre(ctx context.Context, input CostCentreInput) (*models.CostCentre, error) {
	code, err := structureCode(input.Code)
	if err != nil {
		return nil, err
	}
	name, err := structureName(input.Name)
	if err != nil {
		return nil, err
	}
	repo := s.structureUnits()
	unit := &models.CostCentre{Code: code, Name: name, BranchID: emptyToNil(input.BranchID),
		BudgetOwnerUserID: emptyToNil(input.BudgetOwnerUserID)}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.CreateCostCentre(ctx, tx, input.Tenant.OrganizationID, unit); err != nil {
			return unitError(err, "Cost centre")
		}
		return s.recordStructureChange(ctx, tx, input.Tenant, "cost_centre.created", "cost_centre",
			unit.ID, 1, nil, map[string]any{"code": code, "name": name, "branchId": unit.BranchID,
				"budgetOwnerUserId": unit.BudgetOwnerUserID})
	})
	if err != nil {
		return nil, err
	}
	return repo.GetCostCentre(ctx, input.Tenant.OrganizationID, unit.ID)
}

// UpdateCostCentre renames or re-scopes a cost centre.
func (s *Service) UpdateCostCentre(ctx context.Context, input CostCentreInput) (*models.CostCentre, error) {
	repo := s.structureUnits()
	before, err := repo.GetCostCentre(ctx, input.Tenant.OrganizationID, input.CostCentreID)
	if err != nil {
		return nil, structureError(err, "Cost centre")
	}
	if before.Status == models.UnitStatusArchived {
		return nil, apperrors.Conflict("Cost centre is archived and can no longer be changed.", nil)
	}
	updated := *before
	if strings.TrimSpace(input.Name) != "" {
		if updated.Name, err = structureName(input.Name); err != nil {
			return nil, err
		}
	}
	if input.BranchID != nil {
		updated.BranchID = emptyToNil(input.BranchID)
	}
	if input.BudgetOwnerUserID != nil {
		updated.BudgetOwnerUserID = emptyToNil(input.BudgetOwnerUserID)
	}
	expected := input.ExpectedVersion
	if expected == 0 {
		expected = before.Version
	}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.UpdateCostCentre(ctx, tx, input.Tenant.OrganizationID, &updated, expected); err != nil {
			return unitError(err, "Cost centre")
		}
		return s.recordStructureChange(ctx, tx, input.Tenant, "cost_centre.updated", "cost_centre",
			before.ID, expected+1,
			map[string]any{"name": before.Name, "branchId": before.BranchID, "budgetOwnerUserId": before.BudgetOwnerUserID},
			map[string]any{"name": updated.Name, "branchId": updated.BranchID, "budgetOwnerUserId": updated.BudgetOwnerUserID})
	})
	if err != nil {
		return nil, err
	}
	return repo.GetCostCentre(ctx, input.Tenant.OrganizationID, before.ID)
}

// ArchiveCostCentre retires a cost centre no active department uses.
func (s *Service) ArchiveCostCentre(ctx context.Context, tenantCtx tenant.Context, id string,
	expectedVersion int64) error {
	repo := s.structureUnits()
	before, err := repo.GetCostCentre(ctx, tenantCtx.OrganizationID, id)
	if err != nil {
		return structureError(err, "Cost centre")
	}
	if before.Status == models.UnitStatusArchived {
		return nil
	}
	if expectedVersion == 0 {
		expectedVersion = before.Version
	}
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.ArchiveCostCentre(ctx, tx, tenantCtx.OrganizationID, id, expectedVersion); err != nil {
			return unitError(err, "Cost centre")
		}
		return s.recordStructureChange(ctx, tx, tenantCtx, "cost_centre.archived", "cost_centre", id,
			expectedVersion+1, map[string]any{"status": before.Status},
			map[string]any{"status": models.UnitStatusArchived})
	})
}

// ---- teams ----

// TeamInput carries team fields. On update, nil pointers keep the current
// value and an empty string clears an optional reference.
type TeamInput struct {
	Tenant           tenant.Context
	TeamID           string
	Code             string
	Name             string
	BranchID         *string
	DepartmentID     *string
	SupervisorUserID *string
	ExpectedVersion  int64
}

// ListTeams returns the organization's teams.
func (s *Service) ListTeams(ctx context.Context, organizationID string,
	filter orgrepos.TeamFilter) ([]models.Team, error) {
	return s.structureUnits().ListTeams(ctx, organizationID, filter)
}

// GetTeam loads one team.
func (s *Service) GetTeam(ctx context.Context, organizationID string, id string) (*models.Team, error) {
	team, err := s.structureUnits().GetTeam(ctx, organizationID, id)
	return team, structureError(err, "Team")
}

// CreateTeam adds a team.
func (s *Service) CreateTeam(ctx context.Context, input TeamInput) (*models.Team, error) {
	code, err := structureCode(input.Code)
	if err != nil {
		return nil, err
	}
	name, err := structureName(input.Name)
	if err != nil {
		return nil, err
	}
	repo := s.structureUnits()
	team := &models.Team{Code: code, Name: name, BranchID: emptyToNil(input.BranchID),
		DepartmentID: emptyToNil(input.DepartmentID), SupervisorUserID: emptyToNil(input.SupervisorUserID)}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.CreateTeam(ctx, tx, input.Tenant.OrganizationID, team); err != nil {
			return unitError(err, "Team")
		}
		return s.recordStructureChange(ctx, tx, input.Tenant, "team.created", "team", team.ID, 1, nil,
			map[string]any{"code": code, "name": name, "branchId": team.BranchID,
				"departmentId": team.DepartmentID, "supervisorUserId": team.SupervisorUserID})
	})
	if err != nil {
		return nil, err
	}
	return repo.GetTeam(ctx, input.Tenant.OrganizationID, team.ID)
}

// UpdateTeam renames or moves a team.
func (s *Service) UpdateTeam(ctx context.Context, input TeamInput) (*models.Team, error) {
	repo := s.structureUnits()
	before, err := repo.GetTeam(ctx, input.Tenant.OrganizationID, input.TeamID)
	if err != nil {
		return nil, structureError(err, "Team")
	}
	if before.Status == models.UnitStatusArchived {
		return nil, apperrors.Conflict("Team is archived and can no longer be changed.", nil)
	}
	updated := *before
	if strings.TrimSpace(input.Name) != "" {
		if updated.Name, err = structureName(input.Name); err != nil {
			return nil, err
		}
	}
	if input.BranchID != nil {
		updated.BranchID = emptyToNil(input.BranchID)
	}
	if input.DepartmentID != nil {
		updated.DepartmentID = emptyToNil(input.DepartmentID)
	}
	if input.SupervisorUserID != nil {
		updated.SupervisorUserID = emptyToNil(input.SupervisorUserID)
	}
	expected := input.ExpectedVersion
	if expected == 0 {
		expected = before.Version
	}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.UpdateTeam(ctx, tx, input.Tenant.OrganizationID, &updated, expected); err != nil {
			return unitError(err, "Team")
		}
		return s.recordStructureChange(ctx, tx, input.Tenant, "team.updated", "team", before.ID, expected+1,
			map[string]any{"name": before.Name, "branchId": before.BranchID, "departmentId": before.DepartmentID,
				"supervisorUserId": before.SupervisorUserID},
			map[string]any{"name": updated.Name, "branchId": updated.BranchID, "departmentId": updated.DepartmentID,
				"supervisorUserId": updated.SupervisorUserID})
	})
	if err != nil {
		return nil, err
	}
	return repo.GetTeam(ctx, input.Tenant.OrganizationID, before.ID)
}

// ArchiveTeam retires a team.
func (s *Service) ArchiveTeam(ctx context.Context, tenantCtx tenant.Context, id string, expectedVersion int64) error {
	repo := s.structureUnits()
	before, err := repo.GetTeam(ctx, tenantCtx.OrganizationID, id)
	if err != nil {
		return structureError(err, "Team")
	}
	if before.Status == models.UnitStatusArchived {
		return nil
	}
	if expectedVersion == 0 {
		expectedVersion = before.Version
	}
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.ArchiveTeam(ctx, tx, tenantCtx.OrganizationID, id, expectedVersion); err != nil {
			return unitError(err, "Team")
		}
		return s.recordStructureChange(ctx, tx, tenantCtx, "team.archived", "team", id, expectedVersion+1,
			map[string]any{"status": before.Status}, map[string]any{"status": models.UnitStatusArchived})
	})
}

// ---- operating locations ----

// LocationInput carries operating-location fields. On update, nil pointers
// and nil maps keep the current value.
type LocationInput struct {
	Tenant          tenant.Context
	LocationID      string
	Code            string
	Name            string
	LocationType    string
	BranchID        *string
	Address         map[string]any
	Latitude        *float64
	Longitude       *float64
	OperatingHours  map[string]any
	Status          string
	ExpectedVersion int64
}

func validateLocation(location *models.OperatingLocation) error {
	var violations []apperrors.FieldViolation
	if !models.OperatingLocationTypes[location.LocationType] {
		violations = append(violations, apperrors.FieldViolation{Field: "locationType",
			Message: "Use OFFICE, DEPOT, YARD, PARKING, LOADING_BAY, PICKUP_POINT, DROPOFF_POINT, SERVICE_POINT, FUEL_STATION or OTHER."})
	}
	if (location.Latitude == nil) != (location.Longitude == nil) ||
		(location.Latitude != nil && (*location.Latitude < -90 || *location.Latitude > 90 ||
			*location.Longitude < -180 || *location.Longitude > 180)) {
		violations = append(violations, apperrors.FieldViolation{Field: "latitude", Message: "Give both coordinates, within range."})
	}
	for field, document := range map[string]map[string]any{"address": location.Address, "operatingHours": location.OperatingHours} {
		encoded, err := json.Marshal(document)
		if err != nil || len(encoded) > maxStructureJSONBytes {
			violations = append(violations, apperrors.FieldViolation{Field: field, Message: "Must be a JSON object of at most 4 KB."})
		}
	}
	if location.Status != models.UnitStatusActive && location.Status != models.UnitStatusInactive {
		violations = append(violations, apperrors.FieldViolation{Field: "status",
			Message: "Status must be ACTIVE or INACTIVE; use archive to retire a location."})
	}
	if len(violations) > 0 {
		return apperrors.Validation("Check the location details.", violations)
	}
	return nil
}

// ListLocations returns the organization's operating locations.
func (s *Service) ListLocations(ctx context.Context, organizationID string,
	filter orgrepos.LocationFilter) ([]models.OperatingLocation, error) {
	filter.LocationType = strings.ToUpper(strings.TrimSpace(filter.LocationType))
	return s.structureUnits().ListLocations(ctx, organizationID, filter)
}

// GetLocation loads one operating location.
func (s *Service) GetLocation(ctx context.Context, organizationID string, id string) (*models.OperatingLocation, error) {
	location, err := s.structureUnits().GetLocation(ctx, organizationID, id)
	return location, structureError(err, "Location")
}

// CreateLocation adds an operating location.
func (s *Service) CreateLocation(ctx context.Context, input LocationInput) (*models.OperatingLocation, error) {
	code, err := structureCode(input.Code)
	if err != nil {
		return nil, err
	}
	name, err := structureName(input.Name)
	if err != nil {
		return nil, err
	}
	location := &models.OperatingLocation{Code: code, Name: name,
		LocationType: strings.ToUpper(strings.TrimSpace(input.LocationType)),
		BranchID:     emptyToNil(input.BranchID), Address: input.Address,
		Latitude: input.Latitude, Longitude: input.Longitude,
		OperatingHours: input.OperatingHours, Status: models.UnitStatusActive}
	if location.Address == nil {
		location.Address = map[string]any{}
	}
	if location.OperatingHours == nil {
		location.OperatingHours = map[string]any{}
	}
	if err := validateLocation(location); err != nil {
		return nil, err
	}
	repo := s.structureUnits()
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.CreateLocation(ctx, tx, input.Tenant.OrganizationID, location); err != nil {
			return unitError(err, "Location")
		}
		return s.recordStructureChange(ctx, tx, input.Tenant, "location.created", "operating_location",
			location.ID, 1, nil, map[string]any{"code": code, "name": name,
				"locationType": location.LocationType, "branchId": location.BranchID})
	})
	if err != nil {
		return nil, err
	}
	return repo.GetLocation(ctx, input.Tenant.OrganizationID, location.ID)
}

// UpdateLocation edits an operating location.
func (s *Service) UpdateLocation(ctx context.Context, input LocationInput) (*models.OperatingLocation, error) {
	repo := s.structureUnits()
	before, err := repo.GetLocation(ctx, input.Tenant.OrganizationID, input.LocationID)
	if err != nil {
		return nil, structureError(err, "Location")
	}
	if before.Status == models.UnitStatusArchived {
		return nil, apperrors.Conflict("Location is archived and can no longer be changed.", nil)
	}
	updated := *before
	if strings.TrimSpace(input.Name) != "" {
		if updated.Name, err = structureName(input.Name); err != nil {
			return nil, err
		}
	}
	if value := strings.ToUpper(strings.TrimSpace(input.LocationType)); value != "" {
		updated.LocationType = value
	}
	if input.BranchID != nil {
		updated.BranchID = emptyToNil(input.BranchID)
	}
	if input.Address != nil {
		updated.Address = input.Address
	}
	if input.OperatingHours != nil {
		updated.OperatingHours = input.OperatingHours
	}
	if input.Latitude != nil || input.Longitude != nil {
		updated.Latitude, updated.Longitude = input.Latitude, input.Longitude
	}
	if value := strings.ToUpper(strings.TrimSpace(input.Status)); value != "" {
		updated.Status = value
	}
	if err := validateLocation(&updated); err != nil {
		return nil, err
	}
	expected := input.ExpectedVersion
	if expected == 0 {
		expected = before.Version
	}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.UpdateLocation(ctx, tx, input.Tenant.OrganizationID, &updated, expected); err != nil {
			return unitError(err, "Location")
		}
		return s.recordStructureChange(ctx, tx, input.Tenant, "location.updated", "operating_location",
			before.ID, expected+1,
			map[string]any{"name": before.Name, "locationType": before.LocationType, "branchId": before.BranchID,
				"status": before.Status},
			map[string]any{"name": updated.Name, "locationType": updated.LocationType, "branchId": updated.BranchID,
				"status": updated.Status})
	})
	if err != nil {
		return nil, err
	}
	return repo.GetLocation(ctx, input.Tenant.OrganizationID, before.ID)
}

// ArchiveLocation retires an operating location.
func (s *Service) ArchiveLocation(ctx context.Context, tenantCtx tenant.Context, id string, expectedVersion int64) error {
	repo := s.structureUnits()
	before, err := repo.GetLocation(ctx, tenantCtx.OrganizationID, id)
	if err != nil {
		return structureError(err, "Location")
	}
	if before.Status == models.UnitStatusArchived {
		return nil
	}
	if expectedVersion == 0 {
		expectedVersion = before.Version
	}
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.ArchiveLocation(ctx, tx, tenantCtx.OrganizationID, id, expectedVersion); err != nil {
			return unitError(err, "Location")
		}
		return s.recordStructureChange(ctx, tx, tenantCtx, "location.archived", "operating_location", id,
			expectedVersion+1, map[string]any{"status": before.Status},
			map[string]any{"status": models.UnitStatusArchived})
	})
}
