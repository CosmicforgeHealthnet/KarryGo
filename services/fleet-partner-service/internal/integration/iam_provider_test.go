package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// seedIdentity inserts a corporate identity directly (the auth flow is
// covered separately; these tests focus on IAM and providers).
func seedIdentity(t *testing.T, pool *pgxpool.Pool, email string, phone string) string {
	t.Helper()
	identityID := ids.New("cid")
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_corporate_identities
			(id, email_normalized, email_display, phone_e164, phone_display, status,
			 email_verified_at, phone_bound_at)
		VALUES ($1, $2, $2, $3, $3, 'ACTIVE', now(), now())`,
		identityID, email, phone); err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	return identityID
}

func seedMember(t *testing.T, pool *pgxpool.Pool, organizationID string, identityID string, isOwner bool, branches []string) *staffmodels.OrganizationUser {
	t.Helper()
	members := staffrepos.NewMemberRepository(pool)
	member := &staffmodels.OrganizationUser{
		IdentityID:         identityID,
		FullName:           "Test Member",
		Status:             staffmodels.UserStatusActive,
		IsOwner:            isOwner,
		PermittedBranchIDs: branches,
		RoleVersion:        1,
	}
	if err := members.Create(context.Background(), pool, organizationID, member); err != nil {
		t.Fatalf("seed member: %v", err)
	}
	return member
}

func bindRole(t *testing.T, pool *pgxpool.Pool, organizationID string, memberID string, roleID string, scopeType string, scopeID string) {
	t.Helper()
	roles := staffrepos.NewRoleRepository(pool)
	binding := &staffmodels.RoleBinding{
		OrganizationUserID: memberID,
		RoleID:             roleID,
		ScopeType:          scopeType,
		GrantedBy:          "test",
	}
	if scopeID != "" {
		binding.ScopeID = &scopeID
	}
	if err := roles.CreateBinding(context.Background(), pool, organizationID, binding); err != nil {
		t.Fatalf("bind role: %v", err)
	}
}

func TestSystemRolesAreSeededAndImmutable(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	roles := staffrepos.NewRoleRepository(pool)
	org := createOrganization(t, pool, "Roles Org", "RC-ROLES-1")

	list, err := roles.ListRoles(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	systemRoles := 0
	var ownerRole *staffmodels.Role
	for index := range list {
		if list[index].IsSystem {
			systemRoles++
		}
		if list[index].Code == staffmodels.RoleOrganizationOwner {
			ownerRole = &list[index]
		}
	}
	if systemRoles != 12 {
		t.Fatalf("system roles = %d, want 12", systemRoles)
	}
	if ownerRole == nil || len(ownerRole.Permissions) == 0 {
		t.Fatal("owner role template must exist with permissions")
	}

	// A system role cannot be updated or archived by an organization.
	err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return roles.UpdateRole(ctx, tx, org.ID, staffmodels.SystemOwnerRoleID, "Hijacked", "", nil, 1)
	})
	if !errors.Is(err, staffrepos.ErrSystemRoleImmutable) {
		t.Fatalf("system role update = %v, want ErrSystemRoleImmutable", err)
	}
	err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return roles.ArchiveRole(ctx, tx, org.ID, staffmodels.SystemOwnerRoleID, 1)
	})
	if !errors.Is(err, staffrepos.ErrSystemRoleImmutable) {
		t.Fatalf("system role archive = %v, want ErrSystemRoleImmutable", err)
	}
}

func TestAuthorizationIsDenyByDefaultAndScoped(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	roles := staffrepos.NewRoleRepository(pool)
	authorizer := accesscontrol.NewAuthorizer(roles, nil, time.Second)

	org := createOrganization(t, pool, "Authz Org", "RC-AUTHZ-1")
	branches := orgrepos.NewBranchRepository(pool)
	branchA := &orgmodels.Branch{Code: "A", Name: "A", Type: "BRANCH", Timezone: "Africa/Lagos"}
	branchB := &orgmodels.Branch{Code: "B", Name: "B", Type: "BRANCH", Timezone: "Africa/Lagos"}
	if err := branches.Create(ctx, pool, org.ID, branchA); err != nil {
		t.Fatal(err)
	}
	if err := branches.Create(ctx, pool, org.ID, branchB); err != nil {
		t.Fatal(err)
	}

	identity := seedIdentity(t, pool, "authz@example.com", "+2348010000001")
	member := seedMember(t, pool, org.ID, identity, false, nil)

	// No bindings yet: deny by default.
	decision, err := authorizer.Authorize(ctx, org.ID, member.ID, 1, "staff.read", accesscontrol.Target{})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if decision.Allowed {
		t.Fatal("authorization must deny by default")
	}

	// A BRANCH-scoped dispatcher role grants only within its own branch.
	bindRole(t, pool, org.ID, member.ID, "rol_system_dispatcher", staffmodels.ScopeBranch, branchA.ID)

	inBranch, _ := authorizer.Authorize(ctx, org.ID, member.ID, 2, "providers.read",
		accesscontrol.Target{BranchID: branchA.ID})
	if !inBranch.Allowed {
		t.Fatal("branch-scoped role must grant within its branch")
	}
	otherBranch, _ := authorizer.Authorize(ctx, org.ID, member.ID, 2, "providers.read",
		accesscontrol.Target{BranchID: branchB.ID})
	if otherBranch.Allowed {
		t.Fatal("branch-scoped role must not reach another branch")
	}
	unheld, _ := authorizer.Authorize(ctx, org.ID, member.ID, 2, "staff.manage",
		accesscontrol.Target{BranchID: branchA.ID})
	if unheld.Allowed {
		t.Fatal("a role must not grant a permission it does not include")
	}
}

func TestRevokedAndExpiredBindingsGrantNothing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	roles := staffrepos.NewRoleRepository(pool)
	authorizer := accesscontrol.NewAuthorizer(roles, nil, time.Second)

	org := createOrganization(t, pool, "Binding Org", "RC-BIND-1")
	identity := seedIdentity(t, pool, "binding@example.com", "+2348010000002")
	member := seedMember(t, pool, org.ID, identity, false, nil)
	bindRole(t, pool, org.ID, member.ID, "rol_system_admin", staffmodels.ScopeOrganization, "")

	allowed, _ := authorizer.Authorize(ctx, org.ID, member.ID, 1, "staff.read", accesscontrol.Target{})
	if !allowed.Allowed {
		t.Fatal("active binding must grant")
	}

	// Expire it.
	if _, err := pool.Exec(ctx, `
		UPDATE fleet_partner_role_bindings SET expires_at = now() - interval '1 hour'
		WHERE organization_user_id = $1`, member.ID); err != nil {
		t.Fatal(err)
	}
	expired, _ := authorizer.Authorize(ctx, org.ID, member.ID, 2, "staff.read", accesscontrol.Target{})
	if expired.Allowed {
		t.Fatal("an expired binding must grant nothing")
	}

	// Revoke it.
	if _, err := pool.Exec(ctx, `
		UPDATE fleet_partner_role_bindings SET expires_at = NULL, revoked_at = now()
		WHERE organization_user_id = $1`, member.ID); err != nil {
		t.Fatal(err)
	}
	revoked, _ := authorizer.Authorize(ctx, org.ID, member.ID, 3, "staff.read", accesscontrol.Target{})
	if revoked.Allowed {
		t.Fatal("a revoked binding must grant nothing")
	}
}

func TestPrivilegeEscalationIsRefused(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	roles := staffrepos.NewRoleRepository(pool)
	authorizer := accesscontrol.NewAuthorizer(roles, nil, time.Second)

	org := createOrganization(t, pool, "Escalation Org", "RC-ESC-1")
	identity := seedIdentity(t, pool, "escalate@example.com", "+2348010000003")
	actor := seedMember(t, pool, org.ID, identity, false, nil)
	// A dispatcher holds only read permissions.
	bindRole(t, pool, org.ID, actor.ID, "rol_system_dispatcher", staffmodels.ScopeOrganization, "")

	// They may grant what they hold.
	if err := authorizer.RequireGrantable(ctx, org.ID, actor.ID, 1, []string{"providers.read"}); err != nil {
		t.Fatalf("granting a held permission must be allowed: %v", err)
	}
	// They may not grant what they do not hold.
	err := authorizer.RequireGrantable(ctx, org.ID, actor.ID, 1, []string{"organization.close"})
	if apperrors.From(err).Code != errorcodes.PermissionEscalationForbidden {
		t.Fatalf("escalation error = %v, want permission_escalation_forbidden", err)
	}
}

func TestLastOwnerCannotBeSuspendedOrEnded(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	members := staffrepos.NewMemberRepository(pool)
	org := createOrganization(t, pool, "Owner Org", "RC-OWNER-1")

	ownerIdentity := seedIdentity(t, pool, "owner1@example.com", "+2348010000004")
	owner := seedMember(t, pool, org.ID, ownerIdentity, true, nil)

	// Only owner: suspension and ending are refused.
	for _, status := range []string{staffmodels.UserStatusSuspended, staffmodels.UserStatusEnded} {
		err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
			return members.UpdateStatus(ctx, tx, org.ID, owner.ID, status, owner.Version)
		})
		if !errors.Is(err, staffrepos.ErrLastOwner) {
			t.Fatalf("removing the last owner (%s) = %v, want ErrLastOwner", status, err)
		}
	}

	// With a second active owner, the first may be suspended.
	secondIdentity := seedIdentity(t, pool, "owner2@example.com", "+2348010000005")
	seedMember(t, pool, org.ID, secondIdentity, true, nil)
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return members.UpdateStatus(ctx, tx, org.ID, owner.ID, staffmodels.UserStatusSuspended, owner.Version)
	}); err != nil {
		t.Fatalf("suspending a non-last owner: %v", err)
	}
}

func TestStaffCrossTenantIsolation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	members := staffrepos.NewMemberRepository(pool)

	orgA := createOrganization(t, pool, "Staff A", "RC-SA-1")
	orgB := createOrganization(t, pool, "Staff B", "RC-SB-1")
	identity := seedIdentity(t, pool, "shared@example.com", "+2348010000006")
	memberA := seedMember(t, pool, orgA.ID, identity, false, nil)

	// Organization B cannot read organization A's membership.
	if _, err := members.GetByID(ctx, orgB.ID, memberA.ID); !errors.Is(err, staffrepos.ErrNotFound) {
		t.Fatalf("cross-tenant staff read = %v, want ErrNotFound", err)
	}
	// Nor mutate it.
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return members.UpdateStatus(ctx, tx, orgB.ID, memberA.ID, staffmodels.UserStatusSuspended, memberA.Version)
	})
	if !errors.Is(err, staffrepos.ErrNotFound) {
		t.Fatalf("cross-tenant staff mutation = %v, want ErrNotFound", err)
	}

	// The same identity may hold a separate membership in B.
	seedMember(t, pool, orgB.ID, identity, false, nil)
	forIdentity, err := members.ListForIdentity(ctx, identity)
	if err != nil || len(forIdentity) != 2 {
		t.Fatalf("identity memberships = %d, %v; one identity may belong to many organizations",
			len(forIdentity), err)
	}
}

func TestStaffOptimisticConcurrency(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	members := staffrepos.NewMemberRepository(pool)
	org := createOrganization(t, pool, "Concurrency Org", "RC-CONC-1")

	identity := seedIdentity(t, pool, "conc@example.com", "+2348010000007")
	seedMember(t, pool, org.ID, identity, true, nil) // keeps an owner present
	target := seedMember(t, pool, org.ID,
		seedIdentity(t, pool, "conc2@example.com", "+2348010000008"), false, nil)

	// A stale expected version conflicts.
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return members.UpdateStatus(ctx, tx, org.ID, target.ID, staffmodels.UserStatusSuspended, 99)
	})
	if !errors.Is(err, concurrency.ErrVersionMismatch) {
		t.Fatalf("stale version = %v, want ErrVersionMismatch", err)
	}
	// The correct version succeeds and increments exactly once.
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return members.UpdateStatus(ctx, tx, org.ID, target.ID, staffmodels.UserStatusSuspended, target.Version)
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	reloaded, _ := members.GetByID(ctx, org.ID, target.ID)
	if reloaded.Version != target.Version+1 {
		t.Fatalf("version = %d, want %d", reloaded.Version, target.Version+1)
	}
}

func TestProviderRepositoryEnforcesOneLiveMembership(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := providerrepos.NewRepository(pool)

	orgA := createOrganization(t, pool, "Provider Org A", "RC-PA-1")
	orgB := createOrganization(t, pool, "Provider Org B", "RC-PB-1")
	providerID := "drv_external_123"

	first := &providermodels.Membership{
		ProviderService:  "driver-dispatch-delivery-service",
		ServiceCategory:  providermodels.CategoryDispatch,
		ProviderID:       &providerID,
		OnboardingSource: providermodels.SourceLinked,
		RelationshipType: providermodels.RelationshipEmployee,
		Status:           providermodels.StatusActive,
	}
	if err := repo.CreateMembership(ctx, pool, orgA.ID, first); err != nil {
		t.Fatalf("create first membership: %v", err)
	}

	// A second live membership for the same provider is refused by the
	// partial unique index: one settlement organization per provider.
	second := &providermodels.Membership{
		ProviderService:  "driver-dispatch-delivery-service",
		ServiceCategory:  providermodels.CategoryDispatch,
		ProviderID:       &providerID,
		OnboardingSource: providermodels.SourceLinked,
		RelationshipType: providermodels.RelationshipEmployee,
		Status:           providermodels.StatusActive,
	}
	if err := repo.CreateMembership(ctx, pool, orgB.ID, second); err == nil {
		t.Fatal("a provider must not hold two live memberships")
	}

	// The lookup used before provisioning finds the existing live one.
	found, err := repo.FindLiveByProvider(ctx, pool, "driver-dispatch-delivery-service", providerID)
	if err != nil || found.ID != first.ID {
		t.Fatalf("FindLiveByProvider = %+v, %v", found, err)
	}

	// After removal the identity is retained and the slot frees up.
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := repo.TransitionMembership(ctx, tx, orgA.ID, first.ID, providermodels.StatusEnding,
			first.Version, providermodels.HistoryEntry{ActorType: "system", ActorID: "test", ReasonCode: "offboard"}); err != nil {
			return err
		}
		return repo.TransitionMembership(ctx, tx, orgA.ID, first.ID, providermodels.StatusRemoved,
			first.Version+1, providermodels.HistoryEntry{ActorType: "system", ActorID: "test", ReasonCode: "offboard"})
	}); err != nil {
		t.Fatalf("offboard: %v", err)
	}
	removed, _ := repo.GetMembership(ctx, orgA.ID, first.ID)
	if removed.ProviderID == nil || *removed.ProviderID != providerID {
		t.Fatal("the provider identity reference must survive removal")
	}
	if err := repo.CreateMembership(ctx, pool, orgB.ID, second); err != nil {
		t.Fatalf("after removal another organization may engage the provider: %v", err)
	}
}

func TestProviderStateMachineRejectsIllegalTransitions(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := providerrepos.NewRepository(pool)
	org := createOrganization(t, pool, "Provider FSM", "RC-FSM-1")

	membership := &providermodels.Membership{
		ProviderService:  "driver-taxi-service",
		ServiceCategory:  providermodels.CategoryTaxi,
		OnboardingSource: providermodels.SourceSponsored,
		RelationshipType: providermodels.RelationshipContractor,
		Status:           providermodels.StatusInvited,
	}
	if err := repo.CreateMembership(ctx, pool, org.ID, membership); err != nil {
		t.Fatal(err)
	}

	// INVITED cannot jump straight to ACTIVE: consent and setup come first.
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.TransitionMembership(ctx, tx, org.ID, membership.ID, providermodels.StatusActive,
			membership.Version, providermodels.HistoryEntry{ActorType: "system", ActorID: "test"})
	})
	if err == nil {
		t.Fatal("INVITED -> ACTIVE must be rejected; provider consent cannot be skipped")
	}

	// The legal path works and is recorded in append-only history.
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.TransitionMembership(ctx, tx, org.ID, membership.ID,
			providermodels.StatusPendingProviderAcceptance, membership.Version,
			providermodels.HistoryEntry{ActorType: "organization_user", ActorID: "usr", ReasonCode: "invited"})
	}); err != nil {
		t.Fatalf("legal transition: %v", err)
	}
	history, err := repo.ListHistory(ctx, org.ID, membership.ID)
	if err != nil || len(history) != 1 {
		t.Fatalf("history = %d, %v", len(history), err)
	}
	if history[0].FromStatus != providermodels.StatusInvited {
		t.Fatalf("history entry = %+v", history[0])
	}

	// History is append-only.
	if _, err := pool.Exec(ctx,
		`DELETE FROM fleet_partner_provider_membership_history WHERE id = $1`, history[0].ID); err == nil {
		t.Fatal("provider membership history must be append-only")
	}
}

func TestProviderDeclineRightCannotBeDisabled(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := providerrepos.NewRepository(pool)
	org := createOrganization(t, pool, "Decline Org", "RC-DEC-1")

	membership := &providermodels.Membership{
		ProviderService:  "driver-hauling-service",
		ServiceCategory:  providermodels.CategoryHauling,
		OnboardingSource: providermodels.SourceSponsored,
		RelationshipType: providermodels.RelationshipEmployee,
		Status:           providermodels.StatusInvited,
	}
	if err := repo.CreateMembership(ctx, pool, org.ID, membership); err != nil {
		t.Fatal(err)
	}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.UpsertPolicy(ctx, tx, org.ID, &providermodels.Policy{
			MembershipID: membership.ID, OrganizationID: org.ID, AssignmentMode: "OFFER",
		})
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}

	policy, err := repo.GetPolicy(ctx, org.ID, membership.ID)
	if err != nil || !policy.ProviderDeclineRequired {
		t.Fatalf("policy = %+v, %v; decline must be required", policy, err)
	}

	// The database itself refuses to switch the invariant off.
	if _, err := pool.Exec(ctx, `
		UPDATE fleet_partner_provider_policies SET provider_decline_required = false
		WHERE membership_id = $1`, membership.ID); err == nil {
		t.Fatal("provider_decline_required = false must be rejected by the database")
	}
}

func TestProviderSubWalletProjectionHoldsNoBalance(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := providerrepos.NewRepository(pool)
	org := createOrganization(t, pool, "SubWallet Org", "RC-SW-1")

	membership := &providermodels.Membership{
		ProviderService:  "driver-dispatch-delivery-service",
		ServiceCategory:  providermodels.CategoryDispatch,
		OnboardingSource: providermodels.SourceSponsored,
		RelationshipType: providermodels.RelationshipEmployee,
		Status:           providermodels.StatusPendingSetup,
	}
	if err := repo.CreateMembership(ctx, pool, org.ID, membership); err != nil {
		t.Fatal(err)
	}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.EnsureSubWalletLink(ctx, tx, org.ID, membership.ID, "idem-1")
	}); err != nil {
		t.Fatalf("ensure link: %v", err)
	}

	link, err := repo.GetSubWalletLink(ctx, org.ID, membership.ID)
	if err != nil {
		t.Fatalf("get link: %v", err)
	}
	if link.Status != providermodels.SubWalletRequested || link.WalletAccountID != nil {
		t.Fatalf("link = %+v; the account id is only stored after confirmation", link)
	}

	// Confirmation records the identity only.
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.SetSubWalletStatus(ctx, tx, membership.ID, providermodels.SubWalletActive, "wal_123", 7)
	}); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	confirmed, _ := repo.GetSubWalletLink(ctx, org.ID, membership.ID)
	if confirmed.WalletAccountID == nil || *confirmed.WalletAccountID != "wal_123" {
		t.Fatalf("confirmed link = %+v", confirmed)
	}

	// The table has no balance column at all.
	var balanceColumns int
	_ = pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'fleet_partner_provider_subwallet_links' AND column_name LIKE '%balance%'`).
		Scan(&balanceColumns)
	if balanceColumns != 0 {
		t.Fatal("the sub-wallet projection must never carry a balance")
	}
}

func TestProviderMembershipsAreBranchScoped(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := providerrepos.NewRepository(pool)
	branches := orgrepos.NewBranchRepository(pool)
	org := createOrganization(t, pool, "Branch Scope Org", "RC-BSC-1")

	branchA := &orgmodels.Branch{Code: "PA", Name: "PA", Type: "BRANCH", Timezone: "Africa/Lagos"}
	branchB := &orgmodels.Branch{Code: "PB", Name: "PB", Type: "BRANCH", Timezone: "Africa/Lagos"}
	if err := branches.Create(ctx, pool, org.ID, branchA); err != nil {
		t.Fatal(err)
	}
	if err := branches.Create(ctx, pool, org.ID, branchB); err != nil {
		t.Fatal(err)
	}

	for _, branch := range []*orgmodels.Branch{branchA, branchB} {
		membership := &providermodels.Membership{
			ProviderService:  "driver-taxi-service",
			ServiceCategory:  providermodels.CategoryTaxi,
			OnboardingSource: providermodels.SourceSponsored,
			RelationshipType: providermodels.RelationshipEmployee,
			HomeBranchID:     &branch.ID,
			Status:           providermodels.StatusInvited,
		}
		if err := repo.CreateMembership(ctx, pool, org.ID, membership); err != nil {
			t.Fatal(err)
		}
	}

	all, err := repo.ListMemberships(ctx, tenant.Query{OrganizationID: org.ID})
	if err != nil || len(all) != 2 {
		t.Fatalf("unrestricted list = %d, %v", len(all), err)
	}
	scoped, err := repo.ListMemberships(ctx, tenant.Query{
		OrganizationID: org.ID, PermittedBranchIDs: []string{branchA.ID},
	})
	if err != nil || len(scoped) != 1 {
		t.Fatalf("branch-scoped list = %d, %v", len(scoped), err)
	}
	if scoped[0].HomeBranchID == nil || *scoped[0].HomeBranchID != branchA.ID {
		t.Fatalf("scoped result = %+v", scoped[0])
	}
}

func TestProviderInvitationTokensAreHashedAndSingleUse(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := providerrepos.NewRepository(pool)
	org := createOrganization(t, pool, "Invitation Org", "RC-INV-1")

	membership := &providermodels.Membership{
		ProviderService:  "driver-dispatch-delivery-service",
		ServiceCategory:  providermodels.CategoryDispatch,
		OnboardingSource: providermodels.SourceSponsored,
		RelationshipType: providermodels.RelationshipEmployee,
		Status:           providermodels.StatusInvited,
	}
	if err := repo.CreateMembership(ctx, pool, org.ID, membership); err != nil {
		t.Fatal(err)
	}

	invitation := &providermodels.Invitation{
		MembershipID:        &membership.ID,
		ProviderService:     membership.ProviderService,
		LegalName:           "Test Rider",
		EmailNormalized:     "rider@example.com",
		PhoneE164:           "+2348010000009",
		InvitationTokenHash: "hashed-token-value",
		ExpiresAt:           time.Now().Add(time.Hour),
		CreatedBy:           "usr_test",
	}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.CreateInvitation(ctx, tx, org.ID, invitation)
	}); err != nil {
		t.Fatalf("create invitation: %v", err)
	}

	// Acceptance consumes it once.
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.SetInvitationOutcome(ctx, tx, invitation.ID, providermodels.InvitationAccepted)
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.SetInvitationOutcome(ctx, tx, invitation.ID, providermodels.InvitationDeclined)
	})
	if !errors.Is(err, providerrepos.ErrInvitationNotUsable) {
		t.Fatalf("replayed invitation = %v, want ErrInvitationNotUsable", err)
	}

	// No raw token column exists on the table.
	var rawColumns int
	_ = pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'fleet_partner_provider_invitations'
		  AND column_name = 'invitation_token'`).Scan(&rawColumns)
	if rawColumns != 0 {
		t.Fatal("provider invitations must store only the token hash")
	}
}

func TestProviderInvitationExpirySweep(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := providerrepos.NewRepository(pool)
	org := createOrganization(t, pool, "Expiry Org", "RC-EXP-1")

	invitation := &providermodels.Invitation{
		ProviderService:     "driver-taxi-service",
		LegalName:           "Expiring",
		EmailNormalized:     "expiring@example.com",
		PhoneE164:           "+2348010000010",
		InvitationTokenHash: "hash-expiring",
		ExpiresAt:           time.Now().Add(-time.Hour),
		CreatedBy:           "usr_test",
	}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.CreateInvitation(ctx, tx, org.ID, invitation)
	}); err != nil {
		t.Fatal(err)
	}

	expired, err := repo.ExpireDueInvitations(ctx, 100)
	if err != nil || expired == 0 {
		t.Fatalf("ExpireDueInvitations = %d, %v", expired, err)
	}
	var status string
	_ = pool.QueryRow(ctx,
		`SELECT status FROM fleet_partner_provider_invitations WHERE id = $1`, invitation.ID).Scan(&status)
	if status != providermodels.InvitationExpired {
		t.Fatalf("status = %s, want EXPIRED", status)
	}
}

// TestOrganizationLifecycleStatusesAreAccepted guards the Section 8 status
// domain. Migration 000002 originally constrained status to the Sections 3–7
// set; migration 000008 widens it. A live smoke run caught the gap, so this
// asserts every lifecycle status the state machine can produce is storable.
func TestOrganizationLifecycleStatusesAreAccepted(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Lifecycle Statuses", "RC-LC-1")

	for _, status := range []string{
		"DRAFT", "CONTACT_VERIFIED", "BUSINESS_SUBMITTED", "UNDER_REVIEW",
		"CHANGES_REQUIRED", "ACTIVE", "SUSPENDED", "CLOSED", "ARCHIVED",
		"PENDING_REVIEW",
	} {
		if _, err := pool.Exec(ctx,
			`UPDATE fleet_partner_organizations SET status = $2 WHERE id = $1`,
			org.ID, status); err != nil {
			t.Errorf("status %q rejected by the database: %v", status, err)
		}
	}

	// An invented status is still refused.
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_organizations SET status = 'NOT_A_STATUS' WHERE id = $1`,
		org.ID); err == nil {
		t.Fatal("an unknown organization status must be rejected")
	}
}

// TestOrganizationDraftCreationUsesDraftStatus asserts a new organization
// starts in DRAFT (never active) and can be stored.
func TestOrganizationDraftCreationUsesDraftStatus(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := orgrepos.NewOrganizationRepository(pool)

	org := &orgmodels.Organization{
		Status:             orgmodels.OrganizationStatusDraft,
		LegalName:          "Draft Co",
		RegistrationNumber: "RC-DRAFT-1",
		CountryCode:        "NG",
		Timezone:           "Africa/Lagos",
		DefaultCurrency:    "NGN",
	}
	if err := repo.Create(ctx, pool, org); err != nil {
		t.Fatalf("create draft organization: %v", err)
	}
	loaded, err := repo.GetByID(ctx, org.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.Status != orgmodels.OrganizationStatusDraft {
		t.Fatalf("status = %s, want DRAFT; onboarding must never auto-activate", loaded.Status)
	}
}
