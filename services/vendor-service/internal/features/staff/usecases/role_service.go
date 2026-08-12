package staffusecases

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"

	staffmodels "cosmicforge/logistics/services/vendor-service/internal/features/staff/models"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
)

// HighRiskIAMChangePurpose is the step-up purpose required before a role's
// permission set is changed to include, or an assignment is made of a role
// that carries, any high-risk permission (Section 5 rule 14/21).
const HighRiskIAMChangePurpose = "iam.high_risk_permission_change"

// StepUpConsumer is the narrow port RoleService needs from
// internal/features/auth: consume an already-verified step-up grant for a
// specific session/staff/vendor/purpose. Defined here (consumer's own
// usecases package), with the real adapter wired in cmd/main.go — mirrors the
// architecture's cross-feature port-interface convention.
type StepUpConsumer interface {
	ConsumeGrant(ctx context.Context, grantID, sessionID, staffID, vendorID, purpose string) error
}

type RoleService struct {
	roles       *staffrepositories.RoleRepository
	permissions *staffrepositories.PermissionRepository
	assignments *staffrepositories.AssignmentRepository
	evaluator   *AuthorizationEvaluator
	stepUp      StepUpConsumer
	audit       AuditRecorder
	events      EventPublisher
	branches    BranchScopeValidator
}

type RoleServiceOptions struct {
	Roles       *staffrepositories.RoleRepository
	Permissions *staffrepositories.PermissionRepository
	Assignments *staffrepositories.AssignmentRepository
	Evaluator   *AuthorizationEvaluator
	StepUp      StepUpConsumer
	Audit       AuditRecorder
	Events      EventPublisher
	Branches    BranchScopeValidator
}

func NewRoleService(opts RoleServiceOptions) *RoleService {
	return &RoleService{
		roles: opts.Roles, permissions: opts.Permissions, assignments: opts.Assignments,
		evaluator: opts.Evaluator, stepUp: opts.StepUp, audit: opts.Audit, events: opts.Events, branches: opts.Branches,
	}
}

// ─── Reads ───────────────────────────────────────────────────────────────

func (s *RoleService) ListPermissionCatalogue(ctx context.Context, actorStaffID string) ([]staffmodels.Permission, error) {
	if err := s.requirePermission(ctx, actorStaffID, "vendor.roles.read", Target{}); err != nil {
		return nil, err
	}
	return s.permissions.List(ctx)
}

func (s *RoleService) ListRoles(ctx context.Context, actorStaffID, vendorID string) ([]staffmodels.Role, error) {
	if err := s.requirePermission(ctx, actorStaffID, "vendor.roles.read", Target{}); err != nil {
		return nil, err
	}
	return s.roles.ListVisibleToVendor(ctx, vendorID)
}

func (s *RoleService) GetRole(ctx context.Context, actorStaffID, vendorID, roleID string) (staffmodels.Role, []string, error) {
	if err := s.requirePermission(ctx, actorStaffID, "vendor.roles.read", Target{}); err != nil {
		return staffmodels.Role{}, nil, err
	}
	role, err := s.roles.GetVisibleToVendor(ctx, vendorID, roleID)
	if err != nil {
		return staffmodels.Role{}, nil, apperrors.NotFound("Role not found.", err)
	}
	codes, err := s.roles.PermissionCodesForRole(ctx, role.ID)
	if err != nil {
		return staffmodels.Role{}, nil, err
	}
	return role, codes, nil
}

func (s *RoleService) ListAssignmentsForStaff(ctx context.Context, actorStaffID, vendorID, targetStaffID string, staffLookup func(ctx context.Context, vendorID, id string) error) ([]staffmodels.Assignment, error) {
	if err := s.requirePermission(ctx, actorStaffID, "vendor.staff.read", Target{}); err != nil {
		return nil, err
	}
	if staffLookup != nil {
		if err := staffLookup(ctx, vendorID, targetStaffID); err != nil {
			return nil, err
		}
	}
	return s.assignments.ListActiveForStaff(ctx, targetStaffID)
}

// ─── Custom role management ────────────────────────────────────────────────

type CreateCustomRoleInput struct {
	ActorStaffID    string
	VendorID        string
	Code            string
	Name            string
	Description     string
	PermissionCodes []string
	StepUpGrantID   string
	SessionID       string
}

func (s *RoleService) CreateCustomRole(ctx context.Context, in CreateCustomRoleInput) (staffmodels.Role, error) {
	if err := s.requirePermission(ctx, in.ActorStaffID, "vendor.roles.manage", Target{}); err != nil {
		return staffmodels.Role{}, err
	}
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	if code == "" || name == "" {
		return staffmodels.Role{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "code", Message: "Role code and name are required."},
		})
	}

	permissions, err := s.permissions.List(ctx)
	if err != nil {
		return staffmodels.Role{}, err
	}
	permIDs, permCodes, highRisk, err := resolvePermissionSet(permissions, in.PermissionCodes)
	if err != nil {
		return staffmodels.Role{}, err
	}

	actorGrants, err := s.evaluator.EffectivePermissions(ctx, in.ActorStaffID)
	if err != nil {
		return staffmodels.Role{}, err
	}
	if err := delegationCheck(permCodes, actorGrants, nil); err != nil {
		return staffmodels.Role{}, err
	}
	if highRisk {
		if err := s.requireStepUp(ctx, in); err != nil {
			return staffmodels.Role{}, err
		}
	}

	role := staffmodels.Role{ID: "vr_" + uuid.NewString(), VendorID: &in.VendorID, Code: code, Name: name, Description: in.Description}
	if err := s.roles.CreateCustom(ctx, role); err != nil {
		return staffmodels.Role{}, err
	}
	if err := s.roles.SetPermissions(ctx, in.VendorID, role.ID, permIDs, 1); err != nil {
		return staffmodels.Role{}, err
	}

	s.recordAudit(ctx, AuditEntry{
		ActorType: "vendor_staff", ActorID: in.ActorStaffID, Action: "role.created",
		AggregateType: "vendor_role", AggregateID: role.ID, VendorID: in.VendorID, After: role,
	})
	s.publishStaffEvent(ctx, "vendor.role.changed", in.VendorID, in.ActorStaffID, map[string]string{"role_id": role.ID, "action": "created"})
	return role, nil
}

type SetRolePermissionsInput struct {
	ActorStaffID    string
	VendorID        string
	RoleID          string
	PermissionCodes []string
	ExpectedVersion int64
	StepUpGrantID   string
	SessionID       string
}

func (s *RoleService) SetRolePermissions(ctx context.Context, in SetRolePermissionsInput) error {
	if err := s.requirePermission(ctx, in.ActorStaffID, "vendor.roles.manage", Target{}); err != nil {
		return err
	}
	role, err := s.roles.GetVisibleToVendor(ctx, in.VendorID, in.RoleID)
	if err != nil {
		return apperrors.NotFound("Role not found.", err)
	}
	if role.IsSystem {
		return apperrors.Forbidden("Default role families cannot be edited.", nil)
	}

	permissions, err := s.permissions.List(ctx)
	if err != nil {
		return err
	}
	permIDs, permCodes, highRisk, err := resolvePermissionSet(permissions, in.PermissionCodes)
	if err != nil {
		return err
	}

	actorGrants, err := s.evaluator.EffectivePermissions(ctx, in.ActorStaffID)
	if err != nil {
		return err
	}
	if err := delegationCheck(permCodes, actorGrants, nil); err != nil {
		return err
	}
	if highRisk {
		if err := s.requireStepUp(ctx, in); err != nil {
			return err
		}
	}

	before, _ := s.roles.PermissionCodesForRole(ctx, role.ID)
	if err := s.roles.SetPermissions(ctx, in.VendorID, role.ID, permIDs, in.ExpectedVersion); err != nil {
		if err == staffrepositories.ErrVersionConflict {
			return apperrors.Conflict("This role changed since you last loaded it.", err)
		}
		return err
	}

	s.recordAudit(ctx, AuditEntry{
		ActorType: "vendor_staff", ActorID: in.ActorStaffID, Action: "role.permissions_changed",
		AggregateType: "vendor_role", AggregateID: role.ID, VendorID: in.VendorID, Before: before, After: permCodes,
	})
	s.publishStaffEvent(ctx, "vendor.role.changed", in.VendorID, in.ActorStaffID, map[string]string{"role_id": role.ID, "action": "permissions_changed"})
	return nil
}

// ─── Role assignment ────────────────────────────────────────────────────

type AssignRoleInput struct {
	ActorStaffID  string
	VendorID      string
	TargetStaffID string
	RoleID        string
	BranchID      *string
	StepUpGrantID string
	SessionID     string
}

func (s *RoleService) AssignRole(ctx context.Context, in AssignRoleInput, targetVendorMatches func(ctx context.Context, vendorID, staffID string) error) (staffmodels.Assignment, error) {
	if err := s.requirePermission(ctx, in.ActorStaffID, "vendor.roles.assign", Target{BranchID: in.BranchID}); err != nil {
		return staffmodels.Assignment{}, err
	}
	if err := validateBranchScope(ctx, s.branches, in.VendorID, in.BranchID); err != nil {
		return staffmodels.Assignment{}, err
	}
	if targetVendorMatches != nil {
		if err := targetVendorMatches(ctx, in.VendorID, in.TargetStaffID); err != nil {
			return staffmodels.Assignment{}, err
		}
	}
	role, err := s.roles.GetVisibleToVendor(ctx, in.VendorID, in.RoleID)
	if err != nil {
		return staffmodels.Assignment{}, apperrors.NotFound("Role not found.", err)
	}

	rolePermissions, err := s.roles.PermissionCodesForRole(ctx, role.ID)
	if err != nil {
		return staffmodels.Assignment{}, err
	}
	actorGrants, err := s.evaluator.EffectivePermissions(ctx, in.ActorStaffID)
	if err != nil {
		return staffmodels.Assignment{}, err
	}
	if err := delegationCheck(rolePermissions, actorGrants, in.BranchID); err != nil {
		return staffmodels.Assignment{}, err
	}
	if roleCarriesHighRisk(rolePermissions, s.permissionIsHighRisk(ctx)) {
		if err := s.requireStepUp(ctx, in); err != nil {
			return staffmodels.Assignment{}, err
		}
	}

	assignment := staffmodels.Assignment{
		ID: "vsa_" + uuid.NewString(), StaffID: in.TargetStaffID, RoleID: in.RoleID,
		BranchID: in.BranchID, AssignedBy: &in.ActorStaffID,
	}
	if err := s.assignments.Create(ctx, assignment); err != nil {
		if err == staffrepositories.ErrDuplicateAssignment {
			return staffmodels.Assignment{}, apperrors.Conflict("This staff member already holds this role at this scope.", err)
		}
		return staffmodels.Assignment{}, err
	}

	s.recordAudit(ctx, AuditEntry{
		ActorType: "vendor_staff", ActorID: in.ActorStaffID, Action: "role.assigned",
		AggregateType: "vendor_staff_role_assignment", AggregateID: assignment.ID, VendorID: in.VendorID,
		BranchID: derefOr(in.BranchID, ""), After: assignment,
	})
	s.publishStaffEvent(ctx, "vendor.staff.roles_changed", in.VendorID, in.TargetStaffID, map[string]string{"assignment_id": assignment.ID, "action": "assigned"})
	return assignment, nil
}

func (s *RoleService) UnassignRole(ctx context.Context, actorStaffID, vendorID, assignmentID string) error {
	if err := s.requirePermission(ctx, actorStaffID, "vendor.roles.assign", Target{}); err != nil {
		return err
	}
	assignment, err := s.assignments.GetByID(ctx, assignmentID)
	if err != nil {
		return apperrors.NotFound("Assignment not found.", err)
	}
	if err := s.assignments.Revoke(ctx, assignmentID, actorStaffID); err != nil {
		return err
	}
	s.recordAudit(ctx, AuditEntry{
		ActorType: "vendor_staff", ActorID: actorStaffID, Action: "role.unassigned",
		AggregateType: "vendor_staff_role_assignment", AggregateID: assignmentID, VendorID: vendorID, Before: assignment,
	})
	s.publishStaffEvent(ctx, "vendor.staff.roles_changed", vendorID, assignment.StaffID, map[string]string{"assignment_id": assignmentID, "action": "unassigned"})
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────

func (s *RoleService) requirePermission(ctx context.Context, actorStaffID, permission string, target Target) error {
	decision, err := s.evaluator.Authorize(ctx, actorStaffID, permission, target)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return apperrors.Forbidden("You do not have access to this action.", nil)
	}
	return nil
}

type stepUpRequest interface {
	stepUpArgs() (grantID, sessionID, actorStaffID, vendorID string)
}

func (i CreateCustomRoleInput) stepUpArgs() (string, string, string, string) {
	return i.StepUpGrantID, i.SessionID, i.ActorStaffID, i.VendorID
}
func (i SetRolePermissionsInput) stepUpArgs() (string, string, string, string) {
	return i.StepUpGrantID, i.SessionID, i.ActorStaffID, i.VendorID
}
func (i AssignRoleInput) stepUpArgs() (string, string, string, string) {
	return i.StepUpGrantID, i.SessionID, i.ActorStaffID, i.VendorID
}

// requireStepUp consumes a step-up grant for HighRiskIAMChangePurpose. A
// missing/wrong/expired grant fails the whole command — high-risk IAM
// mutations are refused outright, never applied "pending" step-up.
func (s *RoleService) requireStepUp(ctx context.Context, req stepUpRequest) error {
	grantID, sessionID, actorStaffID, vendorID := req.stepUpArgs()
	if s.stepUp == nil || grantID == "" {
		return apperrors.Forbidden("Step-up authentication is required for this change.", nil)
	}
	if err := s.stepUp.ConsumeGrant(ctx, grantID, sessionID, actorStaffID, vendorID, HighRiskIAMChangePurpose); err != nil {
		return apperrors.Forbidden("Step-up authentication is required for this change.", err)
	}
	return nil
}

func (s *RoleService) permissionIsHighRisk(ctx context.Context) map[string]bool {
	permissions, err := s.permissions.List(ctx)
	if err != nil {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(permissions))
	for _, p := range permissions {
		out[p.Code] = p.IsHighRisk
	}
	return out
}

func roleCarriesHighRisk(codes []string, highRisk map[string]bool) bool {
	for _, c := range codes {
		if highRisk[c] {
			return true
		}
	}
	return false
}

// resolvePermissionSet validates that every requested code exists in the
// catalogue and returns (ids, codes, anyHighRisk).
func resolvePermissionSet(catalogue []staffmodels.Permission, requestedCodes []string) ([]string, []string, bool, error) {
	byCode := make(map[string]staffmodels.Permission, len(catalogue))
	for _, p := range catalogue {
		byCode[p.Code] = p
	}
	ids := make([]string, 0, len(requestedCodes))
	codes := make([]string, 0, len(requestedCodes))
	anyHighRisk := false
	seen := map[string]bool{}
	for _, code := range requestedCodes {
		p, ok := byCode[code]
		if !ok {
			return nil, nil, false, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "permission_codes", Message: "Unknown permission: " + code},
			})
		}
		if seen[code] {
			continue
		}
		seen[code] = true
		ids = append(ids, p.ID)
		codes = append(codes, code)
		if p.IsHighRisk {
			anyHighRisk = true
		}
	}
	return ids, codes, anyHighRisk, nil
}

func (s *RoleService) recordAudit(ctx context.Context, entry AuditEntry) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, entry)
}

func (s *RoleService) publishStaffEvent(ctx context.Context, eventType, vendorID, staffID string, payload any) {
	if s.events == nil {
		return
	}
	_ = s.events.PublishStaffEvent(ctx, eventType, vendorID, staffID, payload)
}
