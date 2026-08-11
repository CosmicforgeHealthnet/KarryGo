package staffusecases

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"

	staffmodels "cosmicforge/logistics/services/vendor-service/internal/features/staff/models"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
)

// UserResolver is the narrow port StaffService needs from the auth feature's
// login-identity store (vendor_users) — defined here rather than importing
// internal/features/auth/repositories directly, which the cross-feature
// import guardrail (internal/architecture_test.go) forbids. The real adapter,
// wrapping auth's UserRepository, is wired in cmd/main.go.
type UserResolver interface {
	ResolveOrCreateByPhone(ctx context.Context, phone string) (UserRef, error)
	ResolveOrCreateByEmail(ctx context.Context, email string) (UserRef, error)
}

// UserRef is the minimal login-identity reference StaffService needs.
type UserRef struct {
	ID string
}

// AuditRecorder is the narrow port StaffService needs from internal/audit —
// defined here (in the consuming feature's own usecases package) rather than
// importing internal/audit's concrete type directly by name, so this package
// stays testable with a fake and the composition root controls the real
// wiring (see docs/architecture/03_VENDOR_SERVICE_MODULE_MAP.md).
type AuditRecorder interface {
	Record(ctx context.Context, entry AuditEntry) error
}

// AuditEntry mirrors internal/audit.Entry's shape without importing that
// package's type directly, keeping this feature's usecases free of a
// dependency on a specific audit implementation.
type AuditEntry struct {
	ActorType     string
	ActorID       string
	Action        string
	AggregateType string
	AggregateID   string
	VendorID      string
	BranchID      string
	Before        any
	After         any
	Reason        string
}

// EventPublisher is the narrow port for durable outbox events. Nil-safe.
type EventPublisher interface {
	PublishStaffEvent(ctx context.Context, eventType, vendorID, staffID string, payload any) error
}

// BranchScopeValidator prevents an IAM assignment from persisting an opaque
// branch id that belongs to another business. It is optional only for legacy
// Section 5 tests and internal bootstrapping; production wires Section 6's
// branch aggregate as the authority.
type BranchScopeValidator interface {
	BranchBelongsToVendor(ctx context.Context, vendorID, branchID string) (bool, error)
}

type StaffService struct {
	users       UserResolver
	staff       *staffrepositories.StaffRepository
	roles       *staffrepositories.RoleRepository
	assignments *staffrepositories.AssignmentRepository
	evaluator   *AuthorizationEvaluator
	audit       AuditRecorder
	events      EventPublisher
	branches    BranchScopeValidator
}

type StaffServiceOptions struct {
	Users       UserResolver
	Staff       *staffrepositories.StaffRepository
	Roles       *staffrepositories.RoleRepository
	Assignments *staffrepositories.AssignmentRepository
	Evaluator   *AuthorizationEvaluator
	Audit       AuditRecorder
	Events      EventPublisher
	Branches    BranchScopeValidator
}

func NewStaffService(opts StaffServiceOptions) *StaffService {
	return &StaffService{
		users: opts.Users, staff: opts.Staff, roles: opts.Roles, assignments: opts.Assignments,
		evaluator: opts.Evaluator, audit: opts.Audit, events: opts.Events, branches: opts.Branches,
	}
}

// ─── Provisioning ───────────────────────────────────────────────────────────

// ProvisionOwnerInput bootstraps the very first staff member (Owner) for a
// vendor business. This is an INTERNAL-authority operation (called by an
// internal caller such as a future Section 6 vendor-onboarding flow, or an
// operator), never a public self-serve signup route — Section 5 does not own
// vendor business creation (see Section 6 boundary, rule 20/23).
type ProvisionOwnerInput struct {
	VendorID string
	Phone    string
	Email    string
}

func (s *StaffService) ProvisionOwner(ctx context.Context, in ProvisionOwnerInput) (staffmodels.Staff, error) {
	if strings.TrimSpace(in.VendorID) == "" {
		return staffmodels.Staff{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "vendor_id", Message: "Vendor id is required."},
		})
	}
	user, err := s.resolveOrCreateUser(ctx, in.Phone, in.Email)
	if err != nil {
		return staffmodels.Staff{}, err
	}

	if existing, err := s.staff.GetByVendorAndUser(ctx, in.VendorID, user.ID); err == nil {
		return existing, nil // idempotent: provisioning the same owner twice is a no-op, not an error
	}

	staffRecord := staffmodels.Staff{ID: "vs_" + uuid.NewString(), VendorID: in.VendorID, UserID: user.ID, Status: staffmodels.StaffStatusActive}
	if err := s.staff.Create(ctx, staffRecord); err != nil {
		return staffmodels.Staff{}, err
	}

	assignment := staffmodels.Assignment{ID: "vsa_" + uuid.NewString(), StaffID: staffRecord.ID, RoleID: "role_owner"}
	if err := s.assignments.Create(ctx, assignment); err != nil {
		return staffmodels.Staff{}, err
	}

	s.recordAudit(ctx, AuditEntry{
		ActorType: "system", Action: "staff.provisioned_owner", AggregateType: "vendor_staff",
		AggregateID: staffRecord.ID, VendorID: in.VendorID, After: staffRecord,
	})
	s.publishStaffEvent(ctx, "vendor.staff.created", in.VendorID, staffRecord.ID, staffRecord)
	return staffRecord, nil
}

// InviteStaffInput provisions a new staff member at an existing vendor
// business, performed by an actor who already holds vendor.staff.manage.
// This is the minimum secure backend mechanism (no separate invite/accept
// token flow — Section 5's own rule 26 explicitly asks for the minimum, not a
// large invitation product workflow): the new staff member gains access on
// their next OTP login with the matching identifier.
type InviteStaffInput struct {
	ActorStaffID string
	VendorID     string
	Phone        string
	Email        string
	RoleID       string
	BranchID     *string
}

func (s *StaffService) InviteStaff(ctx context.Context, in InviteStaffInput) (staffmodels.Staff, error) {
	if err := s.requirePermission(ctx, in.ActorStaffID, "vendor.staff.manage", Target{BranchID: in.BranchID}); err != nil {
		return staffmodels.Staff{}, err
	}
	if err := validateBranchScope(ctx, s.branches, in.VendorID, in.BranchID); err != nil {
		return staffmodels.Staff{}, err
	}
	role, err := s.roles.GetVisibleToVendor(ctx, in.VendorID, in.RoleID)
	if err != nil {
		return staffmodels.Staff{}, apperrors.NotFound("Role not found.", err)
	}
	if err := s.assertActorMayDelegateRole(ctx, in.ActorStaffID, role, in.BranchID); err != nil {
		return staffmodels.Staff{}, err
	}

	user, err := s.resolveOrCreateUser(ctx, in.Phone, in.Email)
	if err != nil {
		return staffmodels.Staff{}, err
	}
	if _, err := s.staff.GetByVendorAndUser(ctx, in.VendorID, user.ID); err == nil {
		return staffmodels.Staff{}, apperrors.Conflict("This person is already staff at this vendor business.", nil)
	}

	staffRecord := staffmodels.Staff{ID: "vs_" + uuid.NewString(), VendorID: in.VendorID, UserID: user.ID, Status: staffmodels.StaffStatusActive}
	if err := s.staff.Create(ctx, staffRecord); err != nil {
		return staffmodels.Staff{}, err
	}
	assignment := staffmodels.Assignment{
		ID: "vsa_" + uuid.NewString(), StaffID: staffRecord.ID, RoleID: in.RoleID,
		BranchID: in.BranchID, AssignedBy: &in.ActorStaffID,
	}
	if err := s.assignments.Create(ctx, assignment); err != nil {
		return staffmodels.Staff{}, err
	}

	s.recordAudit(ctx, AuditEntry{
		ActorType: "vendor_staff", ActorID: in.ActorStaffID, Action: "staff.invited",
		AggregateType: "vendor_staff", AggregateID: staffRecord.ID, VendorID: in.VendorID,
		BranchID: derefOr(in.BranchID, ""), After: staffRecord,
	})
	s.publishStaffEvent(ctx, "vendor.staff.created", in.VendorID, staffRecord.ID, staffRecord)
	return staffRecord, nil
}

// ─── Suspension / reactivation ──────────────────────────────────────────────

type SuspendStaffInput struct {
	ActorStaffID    string
	VendorID        string
	TargetStaffID   string
	Reason          string
	ExpectedVersion int64
}

func (s *StaffService) Suspend(ctx context.Context, in SuspendStaffInput, revokeSessions func(ctx context.Context, staffID, reason string) error) error {
	if err := s.requirePermission(ctx, in.ActorStaffID, "vendor.staff.suspend", Target{}); err != nil {
		return err
	}
	target, err := s.staff.GetByIDForVendor(ctx, in.VendorID, in.TargetStaffID)
	if err != nil {
		return apperrors.NotFound("Staff member not found.", err)
	}
	if !target.CanTransitionTo(staffmodels.StaffStatusSuspended) {
		return apperrors.Conflict("This staff member is already suspended.", nil)
	}

	reason := in.Reason
	if err := s.staff.SetStatus(ctx, in.VendorID, in.TargetStaffID, staffmodels.StaffStatusSuspended, &reason, in.ExpectedVersion); err != nil {
		if errors.Is(err, staffrepositories.ErrVersionConflict) {
			return apperrors.Conflict("This staff record changed since you last loaded it.", err)
		}
		return err
	}

	// Suspension must not leave an already-issued refresh token usable —
	// revoke every active session for the target immediately (Section 5 rule
	// 13). This never disables other staff and never pauses the vendor
	// business itself: only vendor_staff.status for this one row changed.
	if revokeSessions != nil {
		if err := revokeSessions(ctx, in.TargetStaffID, "staff_suspended"); err != nil {
			return err
		}
	}

	s.recordAudit(ctx, AuditEntry{
		ActorType: "vendor_staff", ActorID: in.ActorStaffID, Action: "staff.suspended",
		AggregateType: "vendor_staff", AggregateID: in.TargetStaffID, VendorID: in.VendorID,
		Reason: in.Reason, Before: target, After: staffmodels.StaffStatusSuspended,
	})
	s.publishStaffEvent(ctx, "vendor.staff.suspended", in.VendorID, in.TargetStaffID, map[string]string{"reason": in.Reason})
	return nil
}

type ReactivateStaffInput struct {
	ActorStaffID    string
	VendorID        string
	TargetStaffID   string
	ExpectedVersion int64
}

// Reactivate is explicit and audited (Section 5 rule 13/19): it never
// happens as a side effect of anything else, and it does NOT silently restore
// sessions revoked at suspension time — the staff member must log in again.
func (s *StaffService) Reactivate(ctx context.Context, in ReactivateStaffInput) error {
	if err := s.requirePermission(ctx, in.ActorStaffID, "vendor.staff.suspend", Target{}); err != nil {
		return err
	}
	target, err := s.staff.GetByIDForVendor(ctx, in.VendorID, in.TargetStaffID)
	if err != nil {
		return apperrors.NotFound("Staff member not found.", err)
	}
	if !target.CanTransitionTo(staffmodels.StaffStatusActive) {
		return apperrors.Conflict("This staff member is not suspended.", nil)
	}
	if err := s.staff.SetStatus(ctx, in.VendorID, in.TargetStaffID, staffmodels.StaffStatusActive, nil, in.ExpectedVersion); err != nil {
		if errors.Is(err, staffrepositories.ErrVersionConflict) {
			return apperrors.Conflict("This staff record changed since you last loaded it.", err)
		}
		return err
	}
	s.recordAudit(ctx, AuditEntry{
		ActorType: "vendor_staff", ActorID: in.ActorStaffID, Action: "staff.reactivated",
		AggregateType: "vendor_staff", AggregateID: in.TargetStaffID, VendorID: in.VendorID,
		Before: target, After: staffmodels.StaffStatusActive,
	})
	s.publishStaffEvent(ctx, "vendor.staff.reactivated", in.VendorID, in.TargetStaffID, nil)
	return nil
}

// ─── Reads ───────────────────────────────────────────────────────────────

func (s *StaffService) List(ctx context.Context, actorStaffID, vendorID string) ([]staffmodels.Staff, error) {
	if err := s.requirePermission(ctx, actorStaffID, "vendor.staff.read", Target{}); err != nil {
		return nil, err
	}
	return s.staff.ListByVendor(ctx, vendorID)
}

func (s *StaffService) Get(ctx context.Context, actorStaffID, vendorID, targetStaffID string) (staffmodels.Staff, error) {
	if err := s.requirePermission(ctx, actorStaffID, "vendor.staff.read", Target{}); err != nil {
		return staffmodels.Staff{}, err
	}
	target, err := s.staff.GetByIDForVendor(ctx, vendorID, targetStaffID)
	if err != nil {
		return staffmodels.Staff{}, apperrors.NotFound("Staff member not found.", err)
	}
	return target, nil
}

// GetTargetForAssignment is a narrow, permission-check-free vendor-scoped
// existence lookup — used only by RoleService.AssignRole's
// targetVendorMatches callback, where the actor's authorization has already
// been established via vendor.roles.assign and no separate
// vendor.staff.read grant should be required just to name a target staff id.
// A staff id belonging to another vendor simply fails to resolve here, at
// the SQL layer (see StaffRepository.GetByIDForVendor's own doc comment).
func (s *StaffService) GetTargetForAssignment(ctx context.Context, vendorID, targetStaffID string) (staffmodels.Staff, error) {
	target, err := s.staff.GetByIDForVendor(ctx, vendorID, targetStaffID)
	if err != nil {
		return staffmodels.Staff{}, apperrors.NotFound("Staff member not found.", err)
	}
	return target, nil
}

// ─── shared helpers ──────────────────────────────────────────────────────

func (s *StaffService) requirePermission(ctx context.Context, actorStaffID, permission string, target Target) error {
	decision, err := s.evaluator.Authorize(ctx, actorStaffID, permission, target)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return apperrors.Forbidden("You do not have access to this action.", nil)
	}
	return nil
}

// assertActorMayDelegateRole enforces "an actor cannot assign a permission
// they are not authorized to delegate": every permission the target role
// carries must be one the actor themselves already holds at a scope covering
// branchID. This is what stops a Branch Manager from inviting someone into a
// vendor-wide role, and stops any non-Owner from delegating an owner-only
// permission (which no non-Owner role ever carries in the first place, per
// the seeded catalogue, but this check holds even for future custom roles).
func (s *StaffService) assertActorMayDelegateRole(ctx context.Context, actorStaffID string, role staffmodels.Role, branchID *string) error {
	rolePermissions, err := s.roles.PermissionCodesForRole(ctx, role.ID)
	if err != nil {
		return err
	}
	actorGrants, err := s.evaluator.EffectivePermissions(ctx, actorStaffID)
	if err != nil {
		return err
	}
	return delegationCheck(rolePermissions, actorGrants, branchID)
}

// delegationCheck is shared by StaffService (role assignment at invite time)
// and RoleService (role assignment / custom-role permission edits): every
// permission code in the proposed set must be one the actor already holds
// themselves, at a scope covering branchID, and none may be owner-only.
func delegationCheck(codes []string, actorGrants []EffectiveGrant, branchID *string) error {
	for _, code := range codes {
		if staffmodels.OwnerOnlyPermissionCodes[code] {
			return apperrors.Forbidden("This permission can only be held by the Owner role.", nil)
		}
		if !actorCovers(actorGrants, code, branchID) {
			return apperrors.Forbidden("You cannot grant a permission you do not hold yourself.", nil)
		}
	}
	return nil
}

func actorCovers(grants []EffectiveGrant, code string, branchID *string) bool {
	for _, g := range grants {
		if g.PermissionCode != code {
			continue
		}
		if g.BranchID == nil {
			return true // vendor-wide grant covers any branch target
		}
		if branchID != nil && *branchID == *g.BranchID {
			return true
		}
	}
	return false
}

func (s *StaffService) resolveOrCreateUser(ctx context.Context, phone, email string) (UserRef, error) {
	phone = strings.TrimSpace(phone)
	email = strings.ToLower(strings.TrimSpace(email))
	if phone == "" && email == "" {
		return UserRef{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone", Message: "Phone number or email address is required."},
		})
	}
	if phone != "" {
		return s.users.ResolveOrCreateByPhone(ctx, phone)
	}
	return s.users.ResolveOrCreateByEmail(ctx, email)
}

func (s *StaffService) recordAudit(ctx context.Context, entry AuditEntry) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, entry)
}

func (s *StaffService) publishStaffEvent(ctx context.Context, eventType, vendorID, staffID string, payload any) {
	if s.events == nil {
		return
	}
	_ = s.events.PublishStaffEvent(ctx, eventType, vendorID, staffID, payload)
}

func derefOr(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return *v
}
