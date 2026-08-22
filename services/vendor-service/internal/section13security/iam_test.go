// Package section13security is Section 13's IAM and isolation certification.
//
// It lives outside internal/features on purpose. Certifying that a vendor
// Owner cannot publish global taxonomy means composing Section 5's staff IAM
// (roles, assignments, the authorization evaluator) with Section 13's taxonomy
// and search surfaces, and a test inside either feature would have to import
// the other's repositories - exactly what internal/architecture_test.go
// forbids. A neutral package composes both legitimately, the same way
// internal/crmcontract composes every feature's routes.
//
// Everything here runs against real PostgreSQL with real migrations applied.
// The permission grants under test are rows written by migration 000059, not
// constants in Go, so these tests fail if the migration is edited to hand a
// platform capability to a vendor role.
package section13security

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/vendor-service/internal/features/taxonomy"
)

// Section 13 capability codes, named here as the test's own expectation rather
// than imported, so a rename in the feature package cannot silently retarget
// the assertion at a permission that no longer means what it did.
const (
	permTaxonomyRead      = "vendor.taxonomy.read"
	permTaxonomyMapping   = "vendor.taxonomy.mapping.manage"
	permTaxonomyPlatform  = "vendor.taxonomy.platform.admin"
	permSearchRead        = "vendor.search.read"
	permSearchRanking     = "vendor.search.ranking.admin"
	permSearchProjections = "vendor.search.projection.admin"
)

// platformCapabilities are the three that must never reach a vendor role.
var platformCapabilities = []string{permTaxonomyPlatform, permSearchRanking, permSearchProjections}

func testPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping Section 13 IAM certification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err = database.ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("apply Vendor migrations: %v", err)
	}
	return pool, ctx
}

// ---------------------------------------------------------------------------
// Migration-level separation
// ---------------------------------------------------------------------------

// TestSection13PermissionsExistWithTheExpectedRisk pins the vocabulary itself.
func TestSection13PermissionsExistWithTheExpectedRisk(t *testing.T) {
	pool, ctx := testPool(t)

	expected := map[string]bool{ // code -> is_high_risk
		permTaxonomyRead:      false,
		permTaxonomyMapping:   true,
		permTaxonomyPlatform:  true,
		permSearchRead:        false,
		permSearchRanking:     true,
		permSearchProjections: true,
	}

	for code, wantHighRisk := range expected {
		var highRisk bool
		if err := pool.QueryRow(ctx,
			`SELECT is_high_risk FROM vendor_permissions WHERE code=$1`, code,
		).Scan(&highRisk); err != nil {
			t.Fatalf("permission %s is not catalogued: %v", code, err)
		}
		if highRisk != wantHighRisk {
			t.Errorf("permission %s is_high_risk=%t want=%t", code, highRisk, wantHighRisk)
		}
	}
}

// TestPlatformCapabilitiesAreGrantedToNoVendorRole is the central IAM claim of
// Section 13I: VENDOR STAFF IS NOT GLOBAL PLATFORM TAXONOMY ADMIN.
//
// It asserts against vendor_role_permissions directly - every role, including
// role_owner and role_general_manager, and including any custom role a vendor
// defined - rather than against a list of role names. A future migration that
// grants one of these to a role, or a vendor that builds a custom role and
// tries to attach one, fails here.
func TestPlatformCapabilitiesAreGrantedToNoVendorRole(t *testing.T) {
	pool, ctx := testPool(t)

	for _, code := range platformCapabilities {
		rows, err := pool.Query(ctx,
			`SELECT r.id, COALESCE(r.vendor_id, '<system>')
               FROM vendor_role_permissions rp
               JOIN vendor_permissions p ON p.id = rp.permission_id
               JOIN vendor_roles r ON r.id = rp.role_id
              WHERE p.code = $1`, code)
		if err != nil {
			t.Fatal(err)
		}
		var offenders []string
		for rows.Next() {
			var roleID, vendorID string
			if err = rows.Scan(&roleID, &vendorID); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			offenders = append(offenders, roleID+" (vendor="+vendorID+")")
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		if len(offenders) != 0 {
			t.Errorf("platform capability %s is granted to vendor roles %v; it must be granted to none", code, offenders)
		}
	}
}

// TestOwnerAndGeneralManagerHoldOnlyVendorScopedSection13Capabilities is the
// same claim from the other direction, and the one most likely to regress:
// migration 000005 grants role_owner "every permission" as a snapshot, and it
// would be easy for a later migration to re-run that pattern and sweep the
// platform capabilities in with it.
func TestOwnerAndGeneralManagerHoldOnlyVendorScopedSection13Capabilities(t *testing.T) {
	pool, ctx := testPool(t)

	for _, roleID := range []string{"role_owner", "role_general_manager"} {
		held := map[string]bool{}
		rows, err := pool.Query(ctx,
			`SELECT p.code
               FROM vendor_role_permissions rp
               JOIN vendor_permissions p ON p.id = rp.permission_id
              WHERE rp.role_id = $1
                AND (p.code LIKE 'vendor.taxonomy.%' OR p.code LIKE 'vendor.search.%')`, roleID)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var code string
			if err = rows.Scan(&code); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			held[code] = true
		}
		rows.Close()

		for _, want := range []string{permTaxonomyRead, permTaxonomyMapping, permSearchRead} {
			if !held[want] {
				t.Errorf("%s does not hold %s; vendors cannot classify their own catalogue", roleID, want)
			}
		}
		for _, forbidden := range platformCapabilities {
			if held[forbidden] {
				t.Errorf("%s holds platform capability %s", roleID, forbidden)
			}
		}
	}
}

// TestReadOnlyRolesCannotMutateClassification checks the read/write split
// inside the vendor-scoped half of the vocabulary.
func TestReadOnlyRolesCannotMutateClassification(t *testing.T) {
	pool, ctx := testPool(t)

	for _, roleID := range []string{"role_analyst_auditor", "role_marketing", "role_procurement"} {
		var granted bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(
                 SELECT 1 FROM vendor_role_permissions rp
                   JOIN vendor_permissions p ON p.id = rp.permission_id
                  WHERE rp.role_id=$1 AND p.code=$2)`,
			roleID, permTaxonomyMapping,
		).Scan(&granted); err != nil {
			t.Fatal(err)
		}
		if granted {
			t.Errorf("read-only role %s holds %s", roleID, permTaxonomyMapping)
		}
	}
}

// ---------------------------------------------------------------------------
// Evaluator-level separation
// ---------------------------------------------------------------------------

type iamFixture struct {
	pool      *pgxpool.Pool
	evaluator *staffusecases.AuthorizationEvaluator
	access    *taxonomy.VendorAccess
}

// scopeAdapter mirrors cmd/section13.go's production adapter exactly, so these
// tests measure the real resolution path rather than a test-only one.
type scopeAdapter struct {
	evaluator *staffusecases.AuthorizationEvaluator
}

func (a scopeAdapter) ResolveScope(ctx context.Context, staffID, permission string) (taxonomy.Scope, error) {
	grants, err := a.evaluator.EffectivePermissions(ctx, staffID)
	if err != nil {
		return taxonomy.Scope{}, err
	}
	var scope taxonomy.Scope
	for _, grant := range grants {
		if grant.PermissionCode != permission {
			continue
		}
		if grant.BranchID == nil {
			return taxonomy.Scope{Global: true}, nil
		}
		scope.BranchIDs = append(scope.BranchIDs, *grant.BranchID)
	}
	return scope, nil
}

// newIAMFixture builds two businesses, three branches, catalogue resources in
// each, and staff holding deliberately different grants.
func newIAMFixture(t *testing.T) (*iamFixture, context.Context) {
	t.Helper()
	pool, ctx := testPool(t)

	if _, err := pool.Exec(ctx, `
        TRUNCATE
            food_item_modifier_groups, food_modifiers, food_modifier_groups,
            food_item_variants, food_item_media, food_items,
            food_menu_sections, food_menus,
            shop_variants, shop_products, shop_categories,
            vendor_outbox, vendor_audit_log, vendor_idempotency_keys,
            vendor_branch_capabilities, vendor_capabilities, vendor_branches,
            vendor_businesses, vendor_staff_role_assignments, vendor_staff, vendor_users
        RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("reset Section 13 IAM fixture: %v", err)
	}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	for _, staff := range []struct{ business, id, user string }{
		{businessA, staffOwnerA, "vu_s13_owner_a"},
		{businessA, staffReaderA, "vu_s13_reader_a"},
		{businessA, staffBranchA, "vu_s13_branch_a"},
		{businessB, staffOwnerB, "vu_s13_owner_b"},
	} {
		exec(`INSERT INTO vendor_users(id,email) VALUES($1,$1||'@example.test')`, staff.user)
		exec(`INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES($1,$2,$3,'active')`,
			staff.id, staff.business, staff.user)
	}
	for _, business := range []struct{ id, owner string }{
		{businessA, staffOwnerA}, {businessB, staffOwnerB},
	} {
		exec(`INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone)
              VALUES($1,$1||' Limited','ACTIVE',$2,'NGN','Africa/Lagos')`, business.id, business.owner)
	}
	for _, branch := range []struct{ id, business string }{
		{branchA1, businessA}, {branchA2, businessA}, {branchB1, businessB},
	} {
		exec(`INSERT INTO vendor_branches
              (id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status)
              VALUES($1,$2,$1||' Branch','1 Test Road','Lagos','Lagos','NG',6.5,3.4,'Africa/Lagos','ACTIVE')`,
			branch.id, branch.business)
	}

	// Menus and Food items: branch-scoped resources.
	for _, item := range []struct{ id, business, branch string }{
		{foodItemA1, businessA, branchA1},
		{foodItemA2, businessA, branchA2},
		{foodItemB1, businessB, branchB1},
	} {
		menuID := "fm_" + item.id
		sectionID := "fs_" + item.id
		exec(`INSERT INTO food_menus(id,business_id,branch_id,capability_type,name,status)
              VALUES($1,$2,$3,'RESTAURANT',$1||' Menu','ACTIVE')`, menuID, item.business, item.branch)
		exec(`INSERT INTO food_menu_sections(id,business_id,menu_id,name,position,status)
              VALUES($1,$2,$3,'Mains',1,'ACTIVE')`, sectionID, item.business, menuID)
		exec(`INSERT INTO food_items(id,business_id,branch_id,menu_id,section_id,name,base_price_minor,status)
              VALUES($1,$2,$3,$4,$5,'Jollof Rice',250000,'ACTIVE')`,
			item.id, item.business, item.branch, menuID, sectionID)
	}

	// Shop products: business-wide resources. Each business owns its own
	// category, because shop_products carries a composite (category, business)
	// foreign key - a product can never point at another business's category.
	for _, product := range []struct{ id, business string }{
		{shopProductA, businessA}, {shopProductB, businessB},
	} {
		categoryID := "sc_" + product.business
		exec(`INSERT INTO shop_categories(id,business_id,name,slug,status)
              VALUES($1,$2,'Grains','grains','ACTIVE')`, categoryID, product.business)
		exec(`INSERT INTO shop_products(id,business_id,category_id,name,product_class,status)
              VALUES($1,$2,$3,'Raw Rice 50kg','DRY_BULK','ACTIVE')`,
			product.id, product.business, categoryID)
	}

	// Grants. The Owner rows use the seeded system roles, so the grant under
	// test is whatever migration 000059 actually wrote - not a grant this test
	// invented for itself.
	assign := func(id, staff, role string, branch *string) {
		exec(`INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id,branch_id)
              VALUES($1,$2,$3,$4)`, id, staff, role, branch)
	}
	assign("vsra_s13_owner_a", staffOwnerA, "role_owner", nil)
	assign("vsra_s13_owner_b", staffOwnerB, "role_owner", nil)
	assign("vsra_s13_reader_a", staffReaderA, "role_analyst_auditor", nil)
	branchScope := branchA1
	assign("vsra_s13_branch_a", staffBranchA, "role_branch_manager", &branchScope)

	evaluator := staffusecases.NewAuthorizationEvaluator(staffrepositories.NewAssignmentRepository(pool))
	return &iamFixture{
		pool:      pool,
		evaluator: evaluator,
		access:    taxonomy.NewVendorAccess(pool, scopeAdapter{evaluator: evaluator}),
	}, ctx
}

const (
	businessA = "vb_s13_a"
	businessB = "vb_s13_b"

	branchA1 = "vbr_s13_a1"
	branchA2 = "vbr_s13_a2"
	branchB1 = "vbr_s13_b1"

	staffOwnerA  = "vs_s13_owner_a"
	staffReaderA = "vs_s13_reader_a"
	staffBranchA = "vs_s13_branch_a"
	staffOwnerB  = "vs_s13_owner_b"

	foodItemA1 = "fi_s13_a1"
	foodItemA2 = "fi_s13_a2"
	foodItemB1 = "fi_s13_b1"

	shopProductA = "sp_s13_a"
	shopProductB = "sp_s13_b"
)

// TestDenyByDefaultWhenPermissionIsAbsent proves the evaluator refuses rather
// than falls through when a staff member holds nothing relevant.
func TestDenyByDefaultWhenPermissionIsAbsent(t *testing.T) {
	fixture, ctx := newIAMFixture(t)

	// The read-only analyst holds taxonomy.read but not mapping.manage.
	if _, err := fixture.access.Authorize(ctx, staffReaderA, permTaxonomyRead); err != nil {
		t.Fatalf("analyst should hold %s: %v", permTaxonomyRead, err)
	}
	if _, err := fixture.access.Authorize(ctx, staffReaderA, permTaxonomyMapping); err != taxonomy.ErrForbidden {
		t.Fatalf("analyst holding %s: err=%v want=ErrForbidden", permTaxonomyMapping, err)
	}

	// A staff id that holds nothing at all.
	if _, err := fixture.access.Authorize(ctx, "vs_nonexistent", permTaxonomyRead); err != taxonomy.ErrForbidden {
		t.Fatalf("unknown staff: err=%v want=ErrForbidden", err)
	}
}

// TestNoVendorStaffCanReachAPlatformCapability walks every seeded staff member,
// including both Owners, against all three platform capabilities.
func TestNoVendorStaffCanReachAPlatformCapability(t *testing.T) {
	fixture, ctx := newIAMFixture(t)

	for _, staffID := range []string{staffOwnerA, staffOwnerB, staffReaderA, staffBranchA} {
		for _, capability := range platformCapabilities {
			scope, err := fixture.access.Authorize(ctx, staffID, capability)
			if err == nil {
				t.Errorf("staff %s was authorized for platform capability %s (scope=%+v)", staffID, capability, scope)
				continue
			}
			if err != taxonomy.ErrForbidden {
				t.Errorf("staff %s / %s: err=%v want=ErrForbidden", staffID, capability, err)
			}
		}
	}
}

// TestCapabilitiesDoNotImplyOneAnother is the cross-capability separation
// matrix Section 13I calls for: holding one Section 13 capability must never
// confer another.
func TestCapabilitiesDoNotImplyOneAnother(t *testing.T) {
	fixture, ctx := newIAMFixture(t)

	// The analyst holds read capabilities only.
	for _, mustHold := range []string{permTaxonomyRead, permSearchRead} {
		if _, err := fixture.access.Authorize(ctx, staffReaderA, mustHold); err != nil {
			t.Fatalf("analyst should hold %s: %v", mustHold, err)
		}
	}
	// ... and therefore none of: mapping management, ranking administration,
	// projection administration, or taxonomy publication.
	for _, mustNotHold := range []string{
		permTaxonomyMapping, permSearchRanking, permSearchProjections, permTaxonomyPlatform,
	} {
		if _, err := fixture.access.Authorize(ctx, staffReaderA, mustNotHold); err != taxonomy.ErrForbidden {
			t.Errorf("search/taxonomy read implied %s: err=%v", mustNotHold, err)
		}
	}

	// The Owner holds mapping management, and that must not imply publication
	// of the global vocabulary or administration of ranking/projections.
	if _, err := fixture.access.Authorize(ctx, staffOwnerA, permTaxonomyMapping); err != nil {
		t.Fatalf("owner should hold %s: %v", permTaxonomyMapping, err)
	}
	for _, mustNotHold := range platformCapabilities {
		if _, err := fixture.access.Authorize(ctx, staffOwnerA, mustNotHold); err != taxonomy.ErrForbidden {
			t.Errorf("mapping management implied %s: err=%v", mustNotHold, err)
		}
	}
}

// TestHoldingOnePlatformCapabilityConfersNoOther certifies the separation
// between the three platform capabilities themselves.
//
// In production no vendor role grants any of them, so the requirement holds
// vacuously and a test that only checked "nobody has them" would prove nothing
// about whether they are actually distinct. This grants exactly ONE of them to
// a purpose-built role and then asks for the other two - which is the question
// the specification actually poses: ranking administration must not confer
// taxonomy publication, and projection administration must not confer platform
// taxonomy administration.
func TestHoldingOnePlatformCapabilityConfersNoOther(t *testing.T) {
	fixture, ctx := newIAMFixture(t)

	for index, granted := range platformCapabilities {
		roleID := "vr_s13_platform_" + strconv.Itoa(index)
		assignmentID := "vsra_s13_platform_" + strconv.Itoa(index)

		if _, err := fixture.pool.Exec(ctx,
			`INSERT INTO vendor_roles(id,vendor_id,code,name,description,is_system)
             VALUES($1,$2,$3,'Platform probe','Holds exactly one platform capability',false)`,
			roleID, businessA, "platform_probe_"+strconv.Itoa(index),
		); err != nil {
			t.Fatalf("create probe role: %v", err)
		}
		if _, err := fixture.pool.Exec(ctx,
			`INSERT INTO vendor_role_permissions(role_id,permission_id)
             SELECT $1, id FROM vendor_permissions WHERE code=$2`,
			roleID, granted,
		); err != nil {
			t.Fatalf("grant %s: %v", granted, err)
		}
		if _, err := fixture.pool.Exec(ctx,
			`INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id) VALUES($1,$2,$3)`,
			assignmentID, staffReaderA, roleID,
		); err != nil {
			t.Fatalf("assign probe role: %v", err)
		}

		t.Run(granted, func(t *testing.T) {
			// The granted capability resolves, so the probe is real.
			if _, err := fixture.access.Authorize(ctx, staffReaderA, granted); err != nil {
				t.Fatalf("granted capability %s did not resolve: %v", granted, err)
			}
			// Neither of the other two does.
			for _, other := range platformCapabilities {
				if other == granted {
					continue
				}
				if _, err := fixture.access.Authorize(ctx, staffReaderA, other); err != taxonomy.ErrForbidden {
					t.Errorf("holding %s conferred %s: err=%v", granted, other, err)
				}
			}
			// Nor does it confer the vendor-scoped write capability.
			if _, err := fixture.access.Authorize(ctx, staffReaderA, permTaxonomyMapping); err != taxonomy.ErrForbidden {
				t.Errorf("holding %s conferred %s", granted, permTaxonomyMapping)
			}
		})

		// Revoke before the next capability, so each iteration tests exactly
		// one grant rather than an accumulating pile of them.
		if _, err := fixture.pool.Exec(ctx,
			`UPDATE vendor_staff_role_assignments SET revoked_at=now() WHERE id=$1`, assignmentID,
		); err != nil {
			t.Fatalf("revoke probe role: %v", err)
		}
	}
}

// TestBranchScopedGrantCannotEscapeItsBranch proves a Branch Manager's grant is
// bounded at assignment time, not at request time.
func TestBranchScopedGrantCannotEscapeItsBranch(t *testing.T) {
	fixture, ctx := newIAMFixture(t)

	scope, err := fixture.access.Authorize(ctx, staffBranchA, permTaxonomyMapping)
	if err != nil {
		t.Fatalf("branch manager should hold %s: %v", permTaxonomyMapping, err)
	}
	if scope.Global {
		t.Fatal("branch-scoped assignment produced a global scope")
	}

	// In-branch resource: allowed.
	if _, err = fixture.access.ResolveSource(
		ctx, taxonomy.NamespaceFood, taxonomy.SourceTypeFoodItem, foodItemA1, businessA, scope,
	); err != nil {
		t.Fatalf("in-branch Food item refused: %v", err)
	}

	// Same business, different branch: refused, and refused as "not found" so
	// the caller learns nothing about the other branch's catalogue.
	if _, err = fixture.access.ResolveSource(
		ctx, taxonomy.NamespaceFood, taxonomy.SourceTypeFoodItem, foodItemA2, businessA, scope,
	); err != taxonomy.ErrNotFound {
		t.Fatalf("out-of-branch Food item: err=%v want=ErrNotFound", err)
	}

	// A business-wide Shop product needs a business-wide grant; a
	// branch-scoped one must not reach it.
	if _, err = fixture.access.ResolveSource(
		ctx, taxonomy.NamespaceShop, taxonomy.SourceTypeShopProduct, shopProductA, businessA, scope,
	); err != taxonomy.ErrForbidden {
		t.Fatalf("branch-scoped grant on a business-wide product: err=%v want=ErrForbidden", err)
	}
}

// TestRoleNameAloneDoesNotAuthorize guards against the failure mode the spec
// names explicitly: authorizing on a role's display name instead of its
// permissions.
//
// A custom vendor role called "Owner" is created for business B and assigned to
// a staff member with NO permissions attached. If anything anywhere keyed off
// the name, this staff member would be omnipotent.
func TestRoleNameAloneDoesNotAuthorize(t *testing.T) {
	fixture, ctx := newIAMFixture(t)

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO vendor_roles(id,vendor_id,code,name,description,is_system)
         VALUES('vr_s13_fake_owner',$1,'owner','Owner','Impostor role with no permissions',false)`,
		businessB,
	); err != nil {
		t.Fatalf("create impostor role: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id) VALUES($1,$2,$3)`,
		"vsra_s13_fake_owner", staffReaderA, "vr_s13_fake_owner",
	); err != nil {
		t.Fatalf("assign impostor role: %v", err)
	}

	for _, capability := range append([]string{permTaxonomyMapping}, platformCapabilities...) {
		if _, err := fixture.access.Authorize(ctx, staffReaderA, capability); err != taxonomy.ErrForbidden {
			t.Errorf("a role named Owner with no permissions authorized %s: err=%v", capability, err)
		}
	}
}
