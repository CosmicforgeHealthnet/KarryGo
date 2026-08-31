package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Section 9A migration guard.
//
// The pair `000029`/`000030` is purely additive — eight new tables and three
// new permissions — so it must apply on the Section 8 tip, replay on a fresh
// database, and roll back without touching anything Sections 5-8 own.
//
// The structural assertions below matter more than the table list. Ownership,
// money bounds, lifecycle vocabularies and the archived/archived_at agreement
// are enforced by constraints rather than by application code remembering to
// check, and a migration that quietly dropped one would leave the service
// looking correct while the database accepted a row it must not.

func section9aPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 9A migration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return pool, ctx
}

func TestSection9AMigrationCreatesTheCatalogueSchema(t *testing.T) {
	pool, ctx := section9aPool(t)

	for _, table := range []string{
		"food_menus", "food_menu_sections", "food_items", "food_item_media",
		"food_item_variants", "food_modifier_groups", "food_modifiers",
		"food_item_modifier_groups",
	} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("%s is missing at the migration tip exists=%t err=%v", table, exists, err)
		}
	}

	// Composite ownership keys are what close the nested-IDOR cases in the
	// database rather than only in the service.
	for _, constraint := range []string{
		"food_menus_branch_scope_fk",
		"food_menu_sections_menu_scope_fk",
		"food_items_section_scope_fk",
		"food_items_menu_scope_fk",
		"food_item_variants_item_scope_fk",
		"food_modifiers_group_scope_fk",
		"food_item_modifier_groups_item_scope_fk",
		"food_item_modifier_groups_group_scope_fk",
		"food_item_media_item_scope_fk",
	} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname=$1)`, constraint).Scan(&exists); err != nil || !exists {
			t.Errorf("ownership constraint %s is missing (exists=%t err=%v)", constraint, exists, err)
		}
	}

	// Ordering is unique per parent and deferred, which is what lets one atomic
	// statement permute positions while the committed state stays provably
	// unique.
	for _, constraint := range []string{
		"food_menu_sections_position_unique",
		"food_items_position_unique",
		"food_item_variants_position_unique",
		"food_modifiers_position_unique",
		"food_item_modifier_groups_position_unique",
		"food_item_media_position_unique",
	} {
		var deferrable, deferred bool
		if err := pool.QueryRow(ctx,
			`SELECT condeferrable, condeferred FROM pg_constraint WHERE conname=$1`, constraint).
			Scan(&deferrable, &deferred); err != nil {
			t.Errorf("position constraint %s is missing: %v", constraint, err)
			continue
		}
		if !deferrable || !deferred {
			t.Errorf("%s is not DEFERRABLE INITIALLY DEFERRED, so an atomic reorder cannot permute positions", constraint)
		}
	}

	// The query-supporting indexes for the access patterns this section
	// actually issues.
	for _, index := range []string{
		"food_menus_branch_status_idx",
		"food_menus_capability_idx",
		"food_menus_live_name_unique",
		"food_menu_sections_menu_position_idx",
		"food_items_section_position_idx",
		"food_items_business_branch_status_idx",
		"food_item_media_item_position_idx",
		"food_item_media_single_primary_idx",
		"food_item_variants_item_position_idx",
		"food_modifier_groups_business_status_idx",
		"food_modifiers_group_position_idx",
		"food_item_modifier_groups_item_position_idx",
		"food_item_modifier_groups_group_idx",
	} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='public' AND indexname=$1)`, index).
			Scan(&exists); err != nil || !exists {
			t.Errorf("index %s is missing (exists=%t err=%v)", index, exists, err)
		}
	}
}

// TestSection9AConstraintsRefuseImpossibleRows proves the constraints do the
// work rather than merely existing.
func TestSection9AConstraintsRefuseImpossibleRows(t *testing.T) {
	pool, ctx := section9aPool(t)

	if _, err := pool.Exec(ctx, `TRUNCATE food_item_modifier_groups, food_modifiers, food_modifier_groups,
		food_item_variants, food_item_media, food_items, food_menu_sections, food_menus,
		vendor_branch_capabilities, vendor_capabilities, vendor_branches, vendor_businesses,
		vendor_staff_role_assignments, vendor_staff, vendor_users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	seed := []string{
		`INSERT INTO vendor_users(id,email) VALUES('mu_9a','mu_9a@example.test')`,
		`INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES('ms_9a','mb_9a','mu_9a','active')`,
		`INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone)
		 VALUES('mb_9a','Migration Guard Ltd','ACTIVE','ms_9a','NGN','Africa/Lagos')`,
		`INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,
			latitude,longitude,timezone,status)
		 VALUES('mbr_9a','mb_9a','Branch','1 Road','Lagos','Lagos','NG',6.5,3.4,'Africa/Lagos','ACTIVE')`,
		`INSERT INTO food_menus(id,business_id,branch_id,capability_type,name,status)
		 VALUES('mfm_9a','mb_9a','mbr_9a','RESTAURANT','Main Menu','DRAFT')`,
	}
	for _, statement := range seed {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	refused := []struct{ name, statement string }{
		{"a non-food capability", `INSERT INTO food_menus(id,business_id,branch_id,capability_type,name,status)
			VALUES('mfm_bad','mb_9a','mbr_9a','CLOUD_KITCHEN','Cloud','DRAFT')`},
		{"a menu on another business's branch", `INSERT INTO food_menus(id,business_id,branch_id,capability_type,name,status)
			VALUES('mfm_bad2','mb_other','mbr_9a','RESTAURANT','Foreign','DRAFT')`},
		{"an archived menu without archived_at", `INSERT INTO food_menus(id,business_id,branch_id,capability_type,name,status)
			VALUES('mfm_bad3','mb_9a','mbr_9a','FAST_FOOD','Quick','ARCHIVED')`},
		{"a negative item price", `INSERT INTO food_items(id,business_id,branch_id,menu_id,section_id,name,base_price_minor)
			VALUES('mfi_bad','mb_9a','mbr_9a','mfm_9a','mfs_missing','Bad',-1)`},
		{"a modifier group whose maximum is below its minimum",
			`INSERT INTO food_modifier_groups(id,business_id,name,min_select,max_select)
			 VALUES('mfg_bad','mb_9a','Impossible',3,2)`},
		{"an unbounded selection limit",
			`INSERT INTO food_modifier_groups(id,business_id,name,min_select,max_select)
			 VALUES('mfg_bad2','mb_9a','Unbounded',0,1000)`},
		{"an invalid lifecycle value", `INSERT INTO food_menus(id,business_id,branch_id,capability_type,name,status)
			VALUES('mfm_bad4','mb_9a','mbr_9a','RESTAURANT','Weird','SOLD_OUT')`},
	}
	for _, probe := range refused {
		if _, err := pool.Exec(ctx, probe.statement); err == nil {
			t.Errorf("the database accepted %s", probe.name)
		}
	}

	// A negative modifier delta is refused by the database too, not only by the
	// service: a discount is a promotion, and this is not a promotion system.
	if _, err := pool.Exec(ctx, `INSERT INTO food_modifier_groups(id,business_id,name,min_select,max_select)
		VALUES('mfg_ok','mb_9a','Add extras',0,5)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO food_modifiers(id,business_id,modifier_group_id,name,price_delta_minor)
		VALUES('mfd_bad','mb_9a','mfg_ok','Discount',-500)`); err == nil {
		t.Error("the database accepted a negative modifier price delta")
	}
	// Zero is legitimate.
	if _, err := pool.Exec(ctx, `INSERT INTO food_modifiers(id,business_id,modifier_group_id,name,price_delta_minor)
		VALUES('mfd_ok','mb_9a','mfg_ok','No sauce',0)`); err != nil {
		t.Errorf("the database refused a zero-priced modifier: %v", err)
	}
	// And a modifier cannot be created under another business's group.
	if _, err := pool.Exec(ctx, `INSERT INTO food_modifiers(id,business_id,modifier_group_id,name,price_delta_minor)
		VALUES('mfd_bad2','mb_other','mfg_ok','Cross tenant',0)`); err == nil {
		t.Error("the database accepted a modifier under another business's group")
	}
}

// TestSection9APermissionSeedApplies proves the IAM half of the pair landed.
func TestSection9APermissionSeedApplies(t *testing.T) {
	pool, ctx := section9aPool(t)
	for _, code := range []string{
		"vendor.food.catalogue.read", "vendor.food.catalogue.manage", "vendor.food.catalogue.publish",
	} {
		var category string
		var highRisk bool
		if err := pool.QueryRow(ctx,
			`SELECT category, is_high_risk FROM vendor_permissions WHERE code=$1`, code).
			Scan(&category, &highRisk); err != nil {
			t.Fatalf("permission %s is not seeded: %v", code, err)
		}
		if category != "food" {
			t.Errorf("%s has category %q; want food", code, category)
		}
		if highRisk {
			t.Errorf("%s is marked high-risk; catalogue editing is not a step-up act", code)
		}
	}
	var grants int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id = rp.permission_id
		WHERE p.code IN ('vendor.food.catalogue.read','vendor.food.catalogue.manage','vendor.food.catalogue.publish')`).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	// 3 + 3 + 3 for owner/GM/branch manager, 2 for kitchen, 1 for analyst.
	if grants != 12 {
		t.Errorf("Section 9A role grants = %d; want 12", grants)
	}
}

// TestSection9AMigrationsRollBackAndReapplyAsAPair guards the additive claim:
// after down/up, Sections 5-8 are untouched and the catalogue is back.
func TestSection9AMigrationsRollBackAndReapplyAsAPair(t *testing.T) {
	pool, ctx := section9aPool(t)

	migrations, err := ListMigrations("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	tip := migrations[len(migrations)-1]
	if tip.Version != "000030_vendor_section9a_permissions" {
		// Later accepted sections now own the tip, so rolling back the latest two
		// would test those sections rather than Section 9A and could violate their
		// foreign-key dependencies. At a later tip, prove the pair is applied;
		// the global latest-migration replay and each later section's pair replay
		// own destructive rollback certification.
		var applied int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_service_schema_migrations WHERE version IN ('000029_vendor_food_catalogue','000030_vendor_section9a_permissions')`).Scan(&applied); err != nil {
			t.Fatal(err)
		}
		if applied != 2 {
			t.Fatalf("Section 9A migration pair applied=%d want=2 at tip %s", applied, tip.Version)
		}
		return
	}

	for pass := 0; pass < 2; pass++ {
		if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback pass %d: %v", pass, err)
		}
	}
	// The catalogue is gone, and Section 8's tables are not.
	var cataloguePresent, crmPresent bool
	if err = pool.QueryRow(ctx, `SELECT to_regclass('public.food_menus') IS NOT NULL`).Scan(&cataloguePresent); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT to_regclass('public.crm_loyalty_tier_definitions') IS NOT NULL`).Scan(&crmPresent); err != nil {
		t.Fatal(err)
	}
	if cataloguePresent {
		t.Error("food_menus survived the Section 9A rollback")
	}
	if !crmPresent {
		t.Error("the Section 9A rollback damaged Section 8's schema")
	}
	var permissions int
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_permissions WHERE code LIKE 'vendor.food.%'`).Scan(&permissions); err != nil {
		t.Fatal(err)
	}
	if permissions != 0 {
		t.Errorf("%d Section 9A permissions survived the rollback", permissions)
	}

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply: %v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT to_regclass('public.food_menus') IS NOT NULL`).Scan(&cataloguePresent); err != nil {
		t.Fatal(err)
	}
	if !cataloguePresent {
		t.Fatal("food_menus did not come back after reapplying")
	}
}
