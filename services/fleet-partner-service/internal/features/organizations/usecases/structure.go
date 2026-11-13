package usecases

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Organization structure: branches, departments and settings (spec
// Sections 7, 8 stage 5/8 and 37.3). The Head Office branch created with
// every company stays the default; these use cases let a company add and
// maintain the rest of its structure.

var structureCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,31}$`)

var branchTypes = map[string]bool{"BRANCH": true, "HUB": true, "DEPOT": true, "WAREHOUSE": true, "OFFICE": true}

// SetDepartments wires the department repository (kept optional so older
// constructors keep compiling).
func (s *Service) SetDepartments(departments *orgrepos.DepartmentRepository) {
	s.opts.Departments = departments
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func structureError(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, orgrepos.ErrNotFound):
		return apperrors.NotFound(what+" not found.", nil)
	case errors.Is(err, orgrepos.ErrTenantMismatch):
		return apperrors.Validation("A referenced branch or department is not available to this organization.",
			[]apperrors.FieldViolation{{Field: "reference", Message: "Unknown or invalid reference."}})
	case errors.Is(err, orgrepos.ErrHasActiveChildren):
		return apperrors.Conflict("Archive or move the child departments first.", nil)
	case isUnique(err):
		return apperrors.Conflict("The code is already used in this organization.", nil)
	case errors.Is(err, concurrency.ErrVersionMismatch):
		return concurrency.AsConflict(err)
	}
	var appErr *apperrors.Error
	if errors.As(err, &appErr) {
		return err
	}
	if strings.Contains(err.Error(), "timezone") {
		return apperrors.Validation("Timezone must be an IANA timezone such as Africa/Lagos.",
			[]apperrors.FieldViolation{{Field: "timezone", Message: "Invalid timezone."}})
	}
	return err
}

// BranchInput carries branch fields for create and update.
type BranchInput struct {
	Tenant          tenant.Context
	BranchID        string
	Code            string
	Name            string
	Type            string
	Address         string
	Latitude        *float64
	Longitude       *float64
	OperatingHours  string
	Timezone        string
	Status          string
	ExpectedVersion int64
}

func (in *BranchInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	if in.Type == "" {
		in.Type = "BRANCH"
	}
	var violations []apperrors.FieldViolation
	if in.Name == "" || len(in.Name) > 120 {
		violations = append(violations, apperrors.FieldViolation{Field: "name", Message: "Required (at most 120 characters)."})
	}
	if !branchTypes[in.Type] {
		violations = append(violations, apperrors.FieldViolation{Field: "type", Message: "Use BRANCH, HUB, DEPOT, WAREHOUSE or OFFICE."})
	}
	if (in.Latitude == nil) != (in.Longitude == nil) ||
		(in.Latitude != nil && (*in.Latitude < -90 || *in.Latitude > 90 || *in.Longitude < -180 || *in.Longitude > 180)) {
		violations = append(violations, apperrors.FieldViolation{Field: "latitude", Message: "Give both coordinates, within range."})
	}
	if len(violations) > 0 {
		return apperrors.Validation("Check the branch details.", violations)
	}
	return nil
}

// ListBranches returns the branches the caller may see.
func (s *Service) ListBranches(ctx context.Context, query tenant.Query) ([]models.Branch, error) {
	if query.Limit <= 0 {
		query.Limit = 100
	}
	return s.opts.Branches.List(ctx, query)
}

// GetBranch loads one branch inside the tenant.
func (s *Service) GetBranch(ctx context.Context, organizationID string, branchID string) (*models.Branch, error) {
	branch, err := s.opts.Branches.GetByID(ctx, organizationID, branchID)
	return branch, structureError(err, "Branch")
}

// CreateBranch adds a branch to the organization.
func (s *Service) CreateBranch(ctx context.Context, input BranchInput) (*models.Branch, error) {
	if err := input.normalize(); err != nil {
		return nil, err
	}
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	if !structureCodePattern.MatchString(code) {
		return nil, apperrors.Validation("Branch code must be 1-32 letters, digits, '-' or '_'.",
			[]apperrors.FieldViolation{{Field: "code", Message: "Invalid code."}})
	}
	if input.Timezone == "" {
		organization, err := s.opts.Organizations.GetByID(ctx, input.Tenant.OrganizationID)
		if err != nil {
			return nil, err
		}
		input.Timezone = organization.Timezone
	}
	branch := &models.Branch{
		Code: code, Name: input.Name, Type: input.Type, Address: strings.TrimSpace(input.Address),
		Latitude: input.Latitude, Longitude: input.Longitude, OperatingHours: input.OperatingHours,
		Timezone: input.Timezone, Status: models.BranchStatusActive,
	}
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Branches.Create(ctx, tx, input.Tenant.OrganizationID, branch); err != nil {
			return structureError(err, "Branch")
		}
		entry := audit.EntryFromTenant(input.Tenant, "branch.created", "branch", branch.ID)
		entry.NewValues = map[string]any{"code": code, "name": branch.Name, "type": branch.Type}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "branch.created.v1", "branch", branch.ID, 1,
			input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{"branchId": branch.ID, "code": code, "name": branch.Name, "type": branch.Type})
	})
	if err != nil {
		return nil, err
	}
	return s.opts.Branches.GetByID(ctx, input.Tenant.OrganizationID, branch.ID)
}

// UpdateBranch edits a branch. Deactivating or archiving the last active
// branch is refused because an ACTIVE organization must keep one.
func (s *Service) UpdateBranch(ctx context.Context, input BranchInput) (*models.Branch, error) {
	before, err := s.opts.Branches.GetByID(ctx, input.Tenant.OrganizationID, input.BranchID)
	if err != nil {
		return nil, structureError(err, "Branch")
	}
	// Absent fields keep their current values (PATCH semantics).
	if input.Name == "" {
		input.Name = before.Name
	}
	if input.Type == "" {
		input.Type = before.Type
	}
	if input.Latitude == nil && input.Longitude == nil {
		input.Latitude, input.Longitude = before.Latitude, before.Longitude
	}
	if input.Timezone == "" {
		input.Timezone = before.Timezone
	}
	if input.Address == "" {
		input.Address = before.Address
	}
	if input.OperatingHours == "" {
		input.OperatingHours = before.OperatingHours
	}
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = before.Status
	}
	if input.Status != models.BranchStatusActive && input.Status != models.BranchStatusInactive {
		return nil, apperrors.Validation("Status must be ACTIVE or INACTIVE; use archive to retire a branch.",
			[]apperrors.FieldViolation{{Field: "status", Message: "Invalid status."}})
	}
	if err := input.normalize(); err != nil {
		return nil, err
	}
	expected := input.ExpectedVersion
	if expected == 0 {
		expected = before.Version
	}
	updated := *before
	updated.Name, updated.Type, updated.Address = input.Name, input.Type, strings.TrimSpace(input.Address)
	updated.Latitude, updated.Longitude = input.Latitude, input.Longitude
	updated.OperatingHours, updated.Timezone, updated.Status = input.OperatingHours, input.Timezone, input.Status
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if before.Status == models.BranchStatusActive && updated.Status != models.BranchStatusActive {
			if err := s.requireAnotherActiveBranch(ctx, tx, input.Tenant.OrganizationID, before.ID); err != nil {
				return err
			}
		}
		if err := s.opts.Branches.Update(ctx, tx, input.Tenant.OrganizationID, &updated, expected); err != nil {
			return structureError(err, "Branch")
		}
		entry := audit.EntryFromTenant(input.Tenant, "branch.updated", "branch", before.ID)
		entry.PreviousValues = map[string]any{"name": before.Name, "type": before.Type, "status": before.Status}
		entry.NewValues = map[string]any{"name": updated.Name, "type": updated.Type, "status": updated.Status}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "branch.updated.v1", "branch", before.ID, expected+1,
			input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{"branchId": before.ID, "status": updated.Status})
	})
	if err != nil {
		return nil, err
	}
	return s.opts.Branches.GetByID(ctx, input.Tenant.OrganizationID, before.ID)
}

func (s *Service) requireAnotherActiveBranch(ctx context.Context, tx pgx.Tx, organizationID string, branchID string) error {
	var others int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_branches
		WHERE organization_id = $1 AND status = 'ACTIVE' AND id <> $2`, organizationID, branchID).Scan(&others); err != nil {
		return err
	}
	if others == 0 {
		return apperrors.Conflict("An organization must keep at least one active branch.", nil)
	}
	return nil
}

// ArchiveBranch retires a branch. History that references it is kept.
func (s *Service) ArchiveBranch(ctx context.Context, tenantCtx tenant.Context, branchID string, expectedVersion int64) error {
	before, err := s.opts.Branches.GetByID(ctx, tenantCtx.OrganizationID, branchID)
	if err != nil {
		return structureError(err, "Branch")
	}
	if before.Status == models.BranchStatusArchived {
		return nil
	}
	if expectedVersion == 0 {
		expectedVersion = before.Version
	}
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if before.Status == models.BranchStatusActive {
			if err := s.requireAnotherActiveBranch(ctx, tx, tenantCtx.OrganizationID, branchID); err != nil {
				return err
			}
		}
		var departments int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_departments
			WHERE organization_id = $1 AND branch_id = $2 AND status = 'ACTIVE'`, tenantCtx.OrganizationID, branchID).Scan(&departments); err != nil {
			return err
		}
		if departments > 0 {
			return apperrors.Conflict("Archive or move this branch's departments first.", nil)
		}
		if err := s.opts.Branches.Archive(ctx, tenantCtx.OrganizationID, branchID, expectedVersion); err != nil {
			return structureError(err, "Branch")
		}
		entry := audit.EntryFromTenant(tenantCtx, "branch.archived", "branch", branchID)
		entry.PreviousValues = map[string]any{"status": before.Status}
		entry.NewValues = map[string]any{"status": models.BranchStatusArchived}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "branch.archived.v1", "branch", branchID, expectedVersion+1,
			tenantCtx.OrganizationID, tenantCtx.CorrelationID, map[string]any{"branchId": branchID})
	})
}

// DepartmentInput carries department fields.
type DepartmentInput struct {
	Tenant          tenant.Context
	DepartmentID    string
	Code            string
	Name            string
	BranchID        *string
	ParentID        *string
	CostCentreID    *string
	ExpectedVersion int64
}

func emptyToNil(value *string) *string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func (s *Service) departments() (*orgrepos.DepartmentRepository, error) {
	if s.opts.Departments == nil {
		return nil, apperrors.Unavailable("Departments are not available.", nil)
	}
	return s.opts.Departments, nil
}

// ListDepartments returns the organization's departments.
func (s *Service) ListDepartments(ctx context.Context, organizationID string, branchID string, includeArchived bool) ([]models.Department, error) {
	repo, err := s.departments()
	if err != nil {
		return nil, err
	}
	return repo.List(ctx, organizationID, branchID, includeArchived)
}

// GetDepartment loads one department.
func (s *Service) GetDepartment(ctx context.Context, organizationID string, departmentID string) (*models.Department, error) {
	repo, err := s.departments()
	if err != nil {
		return nil, err
	}
	department, err := repo.GetByID(ctx, organizationID, departmentID)
	return department, structureError(err, "Department")
}

// CreateDepartment adds a department.
func (s *Service) CreateDepartment(ctx context.Context, input DepartmentInput) (*models.Department, error) {
	repo, err := s.departments()
	if err != nil {
		return nil, err
	}
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	name := strings.TrimSpace(input.Name)
	if !structureCodePattern.MatchString(code) || name == "" || len(name) > 120 {
		return nil, apperrors.Validation("Check the department details.", []apperrors.FieldViolation{
			{Field: "code", Message: "1-32 letters, digits, '-' or '_'."}, {Field: "name", Message: "Required."}})
	}
	department := &models.Department{Code: code, Name: name, BranchID: emptyToNil(input.BranchID),
		ParentID: emptyToNil(input.ParentID), CostCentreID: emptyToNil(input.CostCentreID)}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.Create(ctx, tx, input.Tenant.OrganizationID, department); err != nil {
			return structureError(err, "Department")
		}
		entry := audit.EntryFromTenant(input.Tenant, "department.created", "department", department.ID)
		entry.NewValues = map[string]any{"code": code, "name": name, "branchId": department.BranchID}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "department.created.v1", "department", department.ID, 1,
			input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{"departmentId": department.ID, "code": code, "branchId": department.BranchID})
	})
	if err != nil {
		return nil, err
	}
	return repo.GetByID(ctx, input.Tenant.OrganizationID, department.ID)
}

// UpdateDepartment renames or moves a department.
func (s *Service) UpdateDepartment(ctx context.Context, input DepartmentInput) (*models.Department, error) {
	repo, err := s.departments()
	if err != nil {
		return nil, err
	}
	before, err := repo.GetByID(ctx, input.Tenant.OrganizationID, input.DepartmentID)
	if err != nil {
		return nil, structureError(err, "Department")
	}
	updated := *before
	if name := strings.TrimSpace(input.Name); name != "" {
		updated.Name = name
	}
	if input.BranchID != nil {
		updated.BranchID = emptyToNil(input.BranchID)
	}
	if input.ParentID != nil {
		updated.ParentID = emptyToNil(input.ParentID)
	}
	if input.CostCentreID != nil {
		updated.CostCentreID = emptyToNil(input.CostCentreID)
	}
	if updated.ParentID != nil && *updated.ParentID == updated.ID {
		return nil, structureError(orgrepos.ErrTenantMismatch, "Department")
	}
	expected := input.ExpectedVersion
	if expected == 0 {
		expected = before.Version
	}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.Update(ctx, tx, input.Tenant.OrganizationID, &updated, expected); err != nil {
			return structureError(err, "Department")
		}
		entry := audit.EntryFromTenant(input.Tenant, "department.updated", "department", before.ID)
		entry.PreviousValues = map[string]any{"name": before.Name, "branchId": before.BranchID, "parentId": before.ParentID}
		entry.NewValues = map[string]any{"name": updated.Name, "branchId": updated.BranchID, "parentId": updated.ParentID}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "department.updated.v1", "department", before.ID, expected+1,
			input.Tenant.OrganizationID, input.Tenant.CorrelationID, map[string]any{"departmentId": before.ID})
	})
	if err != nil {
		return nil, err
	}
	return repo.GetByID(ctx, input.Tenant.OrganizationID, before.ID)
}

// ArchiveDepartment retires a department.
func (s *Service) ArchiveDepartment(ctx context.Context, tenantCtx tenant.Context, departmentID string, expectedVersion int64) error {
	repo, err := s.departments()
	if err != nil {
		return err
	}
	before, err := repo.GetByID(ctx, tenantCtx.OrganizationID, departmentID)
	if err != nil {
		return structureError(err, "Department")
	}
	if before.Status == "ARCHIVED" {
		return nil
	}
	if expectedVersion == 0 {
		expectedVersion = before.Version
	}
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := repo.Archive(ctx, tx, tenantCtx.OrganizationID, departmentID, expectedVersion); err != nil {
			return structureError(err, "Department")
		}
		entry := audit.EntryFromTenant(tenantCtx, "department.archived", "department", departmentID)
		entry.NewValues = map[string]any{"status": "ARCHIVED"}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "department.archived.v1", "department", departmentID, expectedVersion+1,
			tenantCtx.OrganizationID, tenantCtx.CorrelationID, map[string]any{"departmentId": departmentID})
	})
}

// GetSettings returns the organization's current settings.
func (s *Service) GetSettings(ctx context.Context, organizationID string) (*models.Settings, error) {
	return s.opts.Settings.Get(ctx, organizationID)
}

// SettingsPatch carries the organization-editable settings. Operating mode,
// retention and feature flags have their own governed routes (operating
// mode, retention policy, platform capability flags) and are never changed
// here, so this route cannot become a side door around those controls.
type SettingsPatch struct {
	Tenant                   tenant.Context
	AssignmentTimeoutSeconds *int
	ProviderVisibility       *string
	ApprovalThresholds       map[string]any
	ExpectedVersion          int64
}

// UpdateSettings applies a compare-and-swap settings change.
func (s *Service) UpdateSettings(ctx context.Context, patch SettingsPatch) (*models.Settings, error) {
	current, err := s.opts.Settings.Get(ctx, patch.Tenant.OrganizationID)
	if err != nil {
		return nil, err
	}
	next := *current
	if patch.AssignmentTimeoutSeconds != nil {
		if *patch.AssignmentTimeoutSeconds < 30 || *patch.AssignmentTimeoutSeconds > 3600 {
			return nil, apperrors.Validation("Assignment timeout must be between 30 and 3600 seconds.",
				[]apperrors.FieldViolation{{Field: "assignmentTimeoutSeconds", Message: "Out of range."}})
		}
		next.AssignmentTimeoutSeconds = *patch.AssignmentTimeoutSeconds
	}
	if patch.ProviderVisibility != nil {
		value := strings.ToUpper(strings.TrimSpace(*patch.ProviderVisibility))
		if value != "ORGANIZATION" && value != "BRANCH" {
			return nil, apperrors.Validation("Provider visibility must be ORGANIZATION or BRANCH.",
				[]apperrors.FieldViolation{{Field: "providerVisibility", Message: "Invalid value."}})
		}
		next.ProviderVisibility = value
	}
	if patch.ApprovalThresholds != nil {
		for key, value := range patch.ApprovalThresholds {
			// Thresholds are money in minor units: whole, non-negative numbers.
			number, ok := value.(float64)
			if !ok || number < 0 || number != float64(int64(number)) {
				return nil, apperrors.Validation("Approval thresholds must be whole amounts in minor units.",
					[]apperrors.FieldViolation{{Field: "approvalThresholds." + key, Message: "Invalid amount."}})
			}
			patch.ApprovalThresholds[key] = int64(number)
		}
		next.ApprovalThresholds = patch.ApprovalThresholds
	}
	expected := patch.ExpectedVersion
	if expected == 0 {
		expected = current.Version
	}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Settings.Update(ctx, tx, patch.Tenant.OrganizationID, expected, &next); err != nil {
			if errors.Is(err, concurrency.ErrVersionMismatch) {
				return concurrency.AsConflict(err)
			}
			return apperrors.Validation("Check the settings.", []apperrors.FieldViolation{{Field: "settings", Message: err.Error()}})
		}
		entry := audit.EntryFromTenant(patch.Tenant, "organization.settings_updated", "organization_settings", patch.Tenant.OrganizationID)
		entry.PreviousValues = map[string]any{"assignmentTimeoutSeconds": current.AssignmentTimeoutSeconds,
			"providerVisibility": current.ProviderVisibility, "approvalThresholds": current.ApprovalThresholds}
		entry.NewValues = map[string]any{"assignmentTimeoutSeconds": next.AssignmentTimeoutSeconds,
			"providerVisibility": next.ProviderVisibility, "approvalThresholds": next.ApprovalThresholds}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.settings_updated.v1", "organization_settings",
			patch.Tenant.OrganizationID, expected+1, patch.Tenant.OrganizationID, patch.Tenant.CorrelationID,
			map[string]any{"organizationId": patch.Tenant.OrganizationID})
	})
	if err != nil {
		return nil, err
	}
	return s.opts.Settings.Get(ctx, patch.Tenant.OrganizationID)
}
