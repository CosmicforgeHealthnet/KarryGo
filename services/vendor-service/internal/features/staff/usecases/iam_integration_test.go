package staffusecases_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
)

// fakeUserResolver is an in-memory stand-in for the auth feature's
// UserRepository — staffusecases must not import
// internal/features/auth/repositories directly (cross-feature guardrail), so
// these tests exercise StaffService/RoleService against real Postgres for
// everything staff itself owns, with login-identity resolution faked.
type fakeUserResolver struct {
	pool  *pgxpool.Pool
	mu    sync.Mutex
	byKey map[string]string // phone/email -> user id
}

func newFakeUserResolver(pool *pgxpool.Pool) *fakeUserResolver {
	return &fakeUserResolver{pool: pool, byKey: map[string]string{}}
}

// resolve mirrors the real UserRepository's upsert-by-identifier idempotency,
// but also inserts a minimal real vendor_users row: vendor_staff.user_id is a
// genuine foreign key to vendor_users (id) (migrations/000004), enforced
// deliberately, so any test that provisions staff needs a real backing row.
func (f *fakeUserResolver) resolve(ctx context.Context, key, phone, email string) (staffusecases.UserRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.byKey[key]; ok {
		return staffusecases.UserRef{ID: id}, nil
	}
	id := "vu_" + uuid.NewString()
	var phoneArg, emailArg any
	if phone != "" {
		phoneArg = phone
	}
	if email != "" {
		emailArg = email
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO vendor_users (id, phone, email) VALUES ($1,$2,$3)`, id, phoneArg, emailArg); err != nil {
		return staffusecases.UserRef{}, err
	}
	f.byKey[key] = id
	return staffusecases.UserRef{ID: id}, nil
}

func (f *fakeUserResolver) ResolveOrCreateByPhone(ctx context.Context, phone string) (staffusecases.UserRef, error) {
	return f.resolve(ctx, "phone:"+phone, phone, "")
}
func (f *fakeUserResolver) ResolveOrCreateByEmail(ctx context.Context, email string) (staffusecases.UserRef, error) {
	return f.resolve(ctx, "email:"+email, "", email)
}

// capturingAudit records every entry for assertions (Section 5 rule 19/34:
// IAM mutations must be auditable).
type capturingAudit struct {
	mu      sync.Mutex
	entries []staffusecases.AuditEntry
}

func (a *capturingAudit) Record(ctx context.Context, entry staffusecases.AuditEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, entry)
	return nil
}
func (a *capturingAudit) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.entries)
}

type noopEvents struct{}

func (noopEvents) PublishStaffEvent(ctx context.Context, eventType, vendorID, staffID string, payload any) error {
	return nil
}

// controllableStepUp lets tests toggle whether a step-up grant is presented
// as valid, without needing the full OTP/auth flow.
type controllableStepUp struct {
	valid bool
}

func (c *controllableStepUp) ConsumeGrant(ctx context.Context, grantID, sessionID, staffID, vendorID, purpose string) error {
	if !c.valid {
		return errors.New("step-up required")
	}
	return nil
}

func iamTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres staff IAM integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.ApplyMigrations(ctx, pool, "../../../../migrations"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `TRUNCATE vendor_staff_role_assignments, vendor_roles, vendor_staff, vendor_users, vendor_audit_log RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// System roles/permissions are seeded by migration 000005 but the
	// TRUNCATE above only clears vendor_roles rows this test owns — reseed is
	// unnecessary since TRUNCATE CASCADE on vendor_roles would also empty the
	// system role catalogue. Reapply just the seed migration's data instead
	// of the whole schema.
	seedSystemRolesIfMissing(t, pool)
	return pool
}

func seedSystemRolesIfMissing(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM vendor_roles WHERE is_system = true`).Scan(&count); err != nil {
		t.Fatalf("count system roles: %v", err)
	}
	if count > 0 {
		return
	}
	sql, err := os.ReadFile("../../../../migrations/000005_vendor_default_permissions_and_roles.up.sql")
	if err != nil {
		t.Fatalf("read seed migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("reseed system roles/permissions: %v", err)
	}
}

func newTestServices(pool *pgxpool.Pool, users *fakeUserResolver, audit *capturingAudit, stepUp *controllableStepUp) (*staffusecases.StaffService, *staffusecases.RoleService) {
	staffRepo := staffrepositories.NewStaffRepository(pool)
	roleRepo := staffrepositories.NewRoleRepository(pool)
	permRepo := staffrepositories.NewPermissionRepository(pool)
	assignRepo := staffrepositories.NewAssignmentRepository(pool)
	evaluator := staffusecases.NewAuthorizationEvaluator(assignRepo)

	staffSvc := staffusecases.NewStaffService(staffusecases.StaffServiceOptions{
		Users: users, Staff: staffRepo, Roles: roleRepo, Assignments: assignRepo, Evaluator: evaluator, Audit: audit, Events: noopEvents{},
	})
	roleSvc := staffusecases.NewRoleService(staffusecases.RoleServiceOptions{
		Roles: roleRepo, Permissions: permRepo, Assignments: assignRepo, Evaluator: evaluator, StepUp: stepUp, Audit: audit, Events: noopEvents{},
	})
	return staffSvc, roleSvc
}

func TestIAM_ProvisionOwnerThenInviteBranchManager(t *testing.T) {
	pool := iamTestPool(t)
	users := newFakeUserResolver(pool)
	audit := &capturingAudit{}
	staffSvc, roleSvc := newTestServices(pool, users, audit, &controllableStepUp{valid: true})
	ctx := context.Background()

	owner, err := staffSvc.ProvisionOwner(ctx, staffusecases.ProvisionOwnerInput{VendorID: "vendor-1", Email: "owner@a.com"})
	if err != nil {
		t.Fatalf("ProvisionOwner: %v", err)
	}

	branch := "branch-1"
	staffMember, err := staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-1", Email: "bm@a.com", RoleID: "role_branch_manager", BranchID: &branch,
	})
	if err != nil {
		t.Fatalf("InviteStaff (owner delegating Branch Manager): %v", err)
	}

	assignments, err := roleSvc.ListAssignmentsForStaff(ctx, owner.ID, "vendor-1", staffMember.ID, func(ctx context.Context, vendorID, id string) error { return nil })
	if err != nil {
		t.Fatalf("ListAssignmentsForStaff: %v", err)
	}
	if len(assignments) != 1 || assignments[0].RoleID != "role_branch_manager" || assignments[0].BranchID == nil || *assignments[0].BranchID != branch {
		t.Fatalf("unexpected assignments: %+v", assignments)
	}
	if audit.count() < 2 {
		t.Fatalf("expected at least 2 audit entries (provision + invite), got %d", audit.count())
	}
}

func TestIAM_TenantIsolation_VendorACannotAccessVendorBStaff(t *testing.T) {
	pool := iamTestPool(t)
	users := newFakeUserResolver(pool)
	audit := &capturingAudit{}
	staffSvc, roleSvc := newTestServices(pool, users, audit, &controllableStepUp{valid: true})
	ctx := context.Background()

	ownerA, err := staffSvc.ProvisionOwner(ctx, staffusecases.ProvisionOwnerInput{VendorID: "vendor-a", Email: "owner-a@x.com"})
	if err != nil {
		t.Fatalf("provision owner A: %v", err)
	}
	ownerB, err := staffSvc.ProvisionOwner(ctx, staffusecases.ProvisionOwnerInput{VendorID: "vendor-b", Email: "owner-b@x.com"})
	if err != nil {
		t.Fatalf("provision owner B: %v", err)
	}

	// Vendor A's owner cannot read Vendor B's owner staff record.
	if _, err := staffSvc.Get(ctx, ownerA.ID, "vendor-a", ownerB.ID); err == nil {
		t.Fatal("expected Vendor A to be unable to read Vendor B's staff record")
	}

	// Vendor A's owner cannot suspend Vendor B's staff (even naming the right
	// vendor_id in the request — GetByIDForVendor scopes on the ACTOR's own
	// vendor via the input.VendorID the HTTP layer would set from authctx,
	// simulated here directly).
	if err := staffSvc.Suspend(ctx, staffusecases.SuspendStaffInput{
		ActorStaffID: ownerA.ID, VendorID: "vendor-a", TargetStaffID: ownerB.ID, ExpectedVersion: 1,
	}, func(ctx context.Context, staffID, reason string) error { return nil }); err == nil {
		t.Fatal("expected Vendor A to be unable to suspend Vendor B's staff")
	}

	// Vendor A's owner cannot read Vendor B's custom roles.
	_, err = roleSvc.CreateCustomRole(ctx, staffusecases.CreateCustomRoleInput{
		ActorStaffID: ownerB.ID, VendorID: "vendor-b", Code: "b_custom", Name: "B Custom", PermissionCodes: []string{"vendor.orders.read"},
	})
	if err != nil {
		t.Fatalf("create Vendor B custom role: %v", err)
	}
	roles, err := roleSvc.ListRoles(ctx, ownerA.ID, "vendor-a")
	if err != nil {
		t.Fatalf("ListRoles for Vendor A: %v", err)
	}
	for _, r := range roles {
		if r.Code == "b_custom" {
			t.Fatal("Vendor A must not see Vendor B's custom role in its own role list")
		}
	}

	// Vendor A cannot assign Vendor B's staff to a role at all — the target
	// vendor-match callback (as wired in staffhttp) refuses it; simulate that
	// callback here the same way the real handler does.
	_, err = roleSvc.AssignRole(ctx, staffusecases.AssignRoleInput{
		ActorStaffID: ownerA.ID, VendorID: "vendor-a", TargetStaffID: ownerB.ID, RoleID: "role_analyst_auditor",
	}, func(ctx context.Context, vendorID, staffID string) error {
		_, err := staffSvc.GetTargetForAssignment(ctx, vendorID, staffID)
		return err
	})
	if err == nil {
		t.Fatal("expected cross-vendor role assignment to be refused")
	}
}

func TestIAM_PrivilegeEscalation_ActorCannotDelegatePermissionTheyLack(t *testing.T) {
	pool := iamTestPool(t)
	users := newFakeUserResolver(pool)
	audit := &capturingAudit{}
	staffSvc, roleSvc := newTestServices(pool, users, audit, &controllableStepUp{valid: true})
	ctx := context.Background()

	owner, err := staffSvc.ProvisionOwner(ctx, staffusecases.ProvisionOwnerInput{VendorID: "vendor-esc", Email: "owner@esc.com"})
	if err != nil {
		t.Fatalf("provision owner: %v", err)
	}
	// Owner delegates a narrow Marketing role (marketing.read/manage only) to a new staff member.
	marketer, err := staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-esc", Email: "marketer@esc.com", RoleID: "role_marketing",
	})
	if err != nil {
		t.Fatalf("invite marketer: %v", err)
	}

	// The Marketing staff member (holds only vendor.marketing.*) tries to
	// invite someone into the Finance role, which carries vendor.finance.* —
	// permissions the Marketing staffer does not hold. Must be refused.
	_, err = staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: marketer.ID, VendorID: "vendor-esc", Email: "sneaky-finance@esc.com", RoleID: "role_finance",
	})
	if err == nil {
		t.Fatal("expected a Marketing-only staff member to be refused delegating the Finance role (privilege escalation)")
	}

	// The same Marketing staffer trying to grant themselves Owner-level
	// access via a custom role containing an owner-only permission must also
	// be refused outright, regardless of delegation, since vendor.owner.transfer
	// is never delegatable to any non-Owner role.
	_, err = roleSvc.CreateCustomRole(ctx, staffusecases.CreateCustomRoleInput{
		ActorStaffID: marketer.ID, VendorID: "vendor-esc", Code: "self_promote", Name: "Self Promote",
		PermissionCodes: []string{"vendor.owner.transfer"},
	})
	if err == nil {
		t.Fatal("expected creating a role with an owner-only permission to be refused")
	}
}

func TestIAM_BranchScopedActorCannotEscapeBranch(t *testing.T) {
	pool := iamTestPool(t)
	users := newFakeUserResolver(pool)
	audit := &capturingAudit{}
	staffSvc, _ := newTestServices(pool, users, audit, &controllableStepUp{valid: true})
	ctx := context.Background()

	owner, err := staffSvc.ProvisionOwner(ctx, staffusecases.ProvisionOwnerInput{VendorID: "vendor-branch", Email: "owner@branch.com"})
	if err != nil {
		t.Fatalf("provision owner: %v", err)
	}
	branchA := "branch-a"
	bm, err := staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-branch", Email: "bm@branch.com", RoleID: "role_branch_manager", BranchID: &branchA,
	})
	if err != nil {
		t.Fatalf("invite branch manager: %v", err)
	}

	// The Branch Manager (scoped to branch-a only) tries to invite a new
	// staff member into branch-b using the SAME role — the evaluator must
	// refuse: the actor's own grant does not cover branch-b.
	branchB := "branch-b"
	_, err = staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: bm.ID, VendorID: "vendor-branch", Email: "other@branch.com", RoleID: "role_branch_manager", BranchID: &branchB,
	})
	if err == nil {
		t.Fatal("expected a branch-scoped actor to be refused granting access to a different branch")
	}

	// The same actor delegating within THEIR OWN branch succeeds.
	_, err = staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: bm.ID, VendorID: "vendor-branch", Email: "colleague@branch.com", RoleID: "role_branch_manager", BranchID: &branchA,
	})
	if err != nil {
		t.Fatalf("expected delegation within the actor's own branch to succeed: %v", err)
	}
}

func TestIAM_RoleNameAloneDoesNotAuthorize_OnlyPermissionDoes(t *testing.T) {
	pool := iamTestPool(t)
	users := newFakeUserResolver(pool)
	audit := &capturingAudit{}
	staffSvc, _ := newTestServices(pool, users, audit, &controllableStepUp{valid: true})
	ctx := context.Background()

	owner, err := staffSvc.ProvisionOwner(ctx, staffusecases.ProvisionOwnerInput{VendorID: "vendor-perm", Email: "owner@perm.com"})
	if err != nil {
		t.Fatalf("provision owner: %v", err)
	}
	// Analyst/Auditor role: read-only, no vendor.staff.manage.
	analyst, err := staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-perm", Email: "analyst@perm.com", RoleID: "role_analyst_auditor",
	})
	if err != nil {
		t.Fatalf("invite analyst: %v", err)
	}

	// Despite holding a named role, the Analyst cannot perform a write
	// action (invite staff) that depends on vendor.staff.manage, which their
	// role does not grant — proving authorization checks the permission, not
	// the role's mere existence/name.
	_, err = staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: analyst.ID, VendorID: "vendor-perm", Email: "blocked@perm.com", RoleID: "role_analyst_auditor",
	})
	if err == nil {
		t.Fatal("expected a read-only Analyst/Auditor to be refused a staff.manage action")
	}
	// But the Analyst CAN read staff (vendor.staff.read is in their set).
	if _, err := staffSvc.List(ctx, analyst.ID, "vendor-perm"); err != nil {
		t.Fatalf("expected the Analyst to be able to read staff (has vendor.staff.read): %v", err)
	}
}

func TestIAM_HighRiskRoleAssignmentRequiresStepUp(t *testing.T) {
	pool := iamTestPool(t)
	users := newFakeUserResolver(pool)
	audit := &capturingAudit{}
	stepUp := &controllableStepUp{valid: false}
	staffSvc, roleSvc := newTestServices(pool, users, audit, stepUp)
	ctx := context.Background()

	owner, err := staffSvc.ProvisionOwner(ctx, staffusecases.ProvisionOwnerInput{VendorID: "vendor-risk", Email: "owner@risk.com"})
	if err != nil {
		t.Fatalf("provision owner: %v", err)
	}
	finance, err := staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-risk", Email: "finance@risk.com", RoleID: "role_finance",
	})
	if err != nil {
		t.Fatalf("invite finance: %v", err)
	}

	// Owner assigns the Finance role (carries the high-risk
	// vendor.finance.bank_change permission) to a second staff member WITHOUT
	// step-up — must be refused even though Owner is authorized to delegate
	// every permission Finance carries.
	target, err := staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-risk", Email: "target@risk.com", RoleID: "role_analyst_auditor",
	})
	if err != nil {
		t.Fatalf("invite target: %v", err)
	}
	_, err = roleSvc.AssignRole(ctx, staffusecases.AssignRoleInput{
		ActorStaffID: owner.ID, VendorID: "vendor-risk", TargetStaffID: target.ID, RoleID: "role_finance",
	}, func(ctx context.Context, vendorID, staffID string) error {
		_, err := staffSvc.GetTargetForAssignment(ctx, vendorID, staffID)
		return err
	})
	if err == nil {
		t.Fatal("expected assigning a high-risk role to be refused without a valid step-up grant")
	}

	// With a valid step-up grant, the same assignment succeeds.
	stepUp.valid = true
	assignment, err := roleSvc.AssignRole(ctx, staffusecases.AssignRoleInput{
		ActorStaffID: owner.ID, VendorID: "vendor-risk", TargetStaffID: target.ID, RoleID: "role_finance", StepUpGrantID: "grant-1",
	}, func(ctx context.Context, vendorID, staffID string) error {
		_, err := staffSvc.GetTargetForAssignment(ctx, vendorID, staffID)
		return err
	})
	if err != nil {
		t.Fatalf("expected assignment to succeed once step-up is valid: %v", err)
	}
	if assignment.ID == "" {
		t.Fatal("expected a real assignment id")
	}
	_ = finance
}

func TestIAM_DuplicateAssignmentPrevented(t *testing.T) {
	pool := iamTestPool(t)
	users := newFakeUserResolver(pool)
	audit := &capturingAudit{}
	staffSvc, roleSvc := newTestServices(pool, users, audit, &controllableStepUp{valid: true})
	ctx := context.Background()

	owner, err := staffSvc.ProvisionOwner(ctx, staffusecases.ProvisionOwnerInput{VendorID: "vendor-dup", Email: "owner@dup.com"})
	if err != nil {
		t.Fatalf("provision owner: %v", err)
	}
	target, err := staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-dup", Email: "target@dup.com", RoleID: "role_analyst_auditor",
	})
	if err != nil {
		t.Fatalf("invite target: %v", err)
	}
	// InviteStaff already created one active assignment (role_analyst_auditor,
	// vendor-wide). Assigning the exact same role at the exact same scope
	// again must be refused, not silently duplicated.
	_, err = roleSvc.AssignRole(ctx, staffusecases.AssignRoleInput{
		ActorStaffID: owner.ID, VendorID: "vendor-dup", TargetStaffID: target.ID, RoleID: "role_analyst_auditor",
	}, func(ctx context.Context, vendorID, staffID string) error {
		_, err := staffSvc.GetTargetForAssignment(ctx, vendorID, staffID)
		return err
	})
	if err == nil {
		t.Fatal("expected a duplicate active assignment to be refused")
	}
}

func TestIAM_SuspendReactivate_VersionConflictAndAuditTrail(t *testing.T) {
	pool := iamTestPool(t)
	users := newFakeUserResolver(pool)
	audit := &capturingAudit{}
	staffSvc, _ := newTestServices(pool, users, audit, &controllableStepUp{valid: true})
	ctx := context.Background()

	owner, err := staffSvc.ProvisionOwner(ctx, staffusecases.ProvisionOwnerInput{VendorID: "vendor-susp", Email: "owner@susp.com"})
	if err != nil {
		t.Fatalf("provision owner: %v", err)
	}
	target, err := staffSvc.InviteStaff(ctx, staffusecases.InviteStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-susp", Email: "target@susp.com", RoleID: "role_analyst_auditor",
	})
	if err != nil {
		t.Fatalf("invite target: %v", err)
	}

	var sessionsRevoked bool
	revoke := func(ctx context.Context, staffID, reason string) error { sessionsRevoked = true; return nil }

	// Stale version is refused.
	if err := staffSvc.Suspend(ctx, staffusecases.SuspendStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-susp", TargetStaffID: target.ID, Reason: "policy violation", ExpectedVersion: 99,
	}, revoke); err == nil {
		t.Fatal("expected a stale expected_version to be refused")
	}

	if err := staffSvc.Suspend(ctx, staffusecases.SuspendStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-susp", TargetStaffID: target.ID, Reason: "policy violation", ExpectedVersion: 1,
	}, revoke); err != nil {
		t.Fatalf("expected suspend with correct version to succeed: %v", err)
	}
	if !sessionsRevoked {
		t.Fatal("expected suspension to revoke the target's sessions")
	}

	// Suspending an already-suspended staff member is refused (not a silent no-op).
	if err := staffSvc.Suspend(ctx, staffusecases.SuspendStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-susp", TargetStaffID: target.ID, ExpectedVersion: 2,
	}, revoke); err == nil {
		t.Fatal("expected suspending an already-suspended staff member to be refused")
	}

	// The Owner (vendor) is unaffected — a second Owner-suspended-status
	// check would be a different assertion; here we confirm suspending one
	// staff member never mutates another's row.
	ownerStill, err := staffSvc.Get(ctx, owner.ID, "vendor-susp", owner.ID)
	if err != nil {
		t.Fatalf("Get owner: %v", err)
	}
	if ownerStill.Status != "active" {
		t.Fatal("expected the Owner to remain active — suspension must not disable other staff")
	}

	// Reactivate.
	if err := staffSvc.Reactivate(ctx, staffusecases.ReactivateStaffInput{
		ActorStaffID: owner.ID, VendorID: "vendor-susp", TargetStaffID: target.ID, ExpectedVersion: 2,
	}); err != nil {
		t.Fatalf("Reactivate: %v", err)
	}

	if audit.count() < 4 { // provision, invite, suspend, reactivate
		t.Fatalf("expected at least 4 audit entries, got %d", audit.count())
	}
}
