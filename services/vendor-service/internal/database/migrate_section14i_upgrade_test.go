package database

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Section 14I migration scenarios B, D and E.
//
// Scenario A (empty database to tip) and C (roll the tip back and reapply) are
// covered in migrate_section14i_certification_test.go. The three below are the
// upgrade question rather than the fresh-install question: what happens to a
// database that already ran Section 12 in production when Section 14 arrives.

// section14IMigrationsDir lets these scenarios run against Section 14's own
// migration tip even after a later section lands on top of it. Rolling a
// later section's migrations back to reach Section 14 would certify that
// section's down files instead, which is not this section's business, so the
// pinned directory is supplied explicitly rather than taken by rollback.
func section14IMigrationsDir() string {
	if pinned := strings.TrimSpace(
		os.Getenv("VENDOR_SERVICE_TEST_MIGRATIONS_DIR")); pinned != "" {
		return pinned
	}
	return "../../migrations"
}

// TestSection14IUpgradesAPreSection14Database is scenario B. A database at the
// last pre-Section-14 migration must reach the tip, because that is what every
// existing deployment will actually do.
func TestSection14IUpgradesAPreSection14Database(t *testing.T) {
	pool, migrationsDir := historicalMigrationPool(t, "000066")
	ctx := context.Background()

	migrations, err := ListMigrations(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}

	// Count Section 14's own migrations so the rollback stops exactly at the
	// pre-Section-14 boundary rather than at a hardcoded depth.
	var section14 int
	for _, migration := range migrations {
		if strings.HasPrefix(migration.Version, "00006") &&
			migration.Version >= "000060" {
			section14++
		}
	}
	if section14 == 0 {
		t.Fatal("no Section 14 migrations were found")
	}

	for i := 0; i < section14; i++ {
		if err = RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
			t.Fatalf("rolling back to the pre-Section-14 state failed at step %d: %v", i+1, err)
		}
	}

	// This is now a Section 13 database: Shop exists, generic Inventory does not.
	var shopPresent, inventoryPresent bool
	if err = pool.QueryRow(ctx,
		`SELECT to_regclass('public.shop_inventory_stock_bases') IS NOT NULL`).Scan(&shopPresent); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx,
		`SELECT to_regclass('public.inventory_items') IS NOT NULL`).Scan(&inventoryPresent); err != nil {
		t.Fatal(err)
	}
	if !shopPresent {
		t.Fatal("rolling Section 14 back destroyed the Section 12 Shop schema")
	}
	if inventoryPresent {
		t.Fatal("generic Inventory survived a full Section 14 rollback")
	}

	// Scenario B proper: the upgrade an existing deployment performs.
	if err = ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("a pre-Section-14 database could not upgrade to the tip: %v", err)
	}

	for _, table := range []string{
		"inventory_items", "inventory_balances", "inventory_movements",
		"inventory_lots", "inventory_reservations", "vendor_job_controls",
	} {
		var present bool
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&present); err != nil {
			t.Fatal(err)
		}
		if !present {
			t.Errorf("%s is missing after upgrading a pre-Section-14 database", table)
		}
	}
}

// TestSection14IPreservesExistingShopInventoryRows is scenario D. Section 14's
// migrations are additive, but "additive" is a claim about behaviour, and the
// rows in question are a deployed vendor's real stock.
func TestSection14IPreservesExistingShopInventoryRows(t *testing.T) {
	pool, migrationsDir := historicalMigrationPool(t, "000066")
	ctx := context.Background()

	var err error

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	user := "s14i_up_user_" + suffix
	staff := "s14i_up_staff_" + suffix
	business := "s14i_up_biz_" + suffix
	branch := "s14i_up_branch_" + suffix

	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO vendor_users(id,email) VALUES($1,$2)`, []any{user, user + "@example.test"}},
		{`INSERT INTO vendor_staff(id,vendor_id,user_id) VALUES($1,$2,$3)`, []any{staff, business, user}},
		{`INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone)
		   VALUES($1,'Section 14I upgrade','ACTIVE',$2,'NGN','Africa/Lagos')`, []any{business, staff}},
		{`INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status)
		   VALUES($1,$2,'Upgrade Branch','1 Test Street','Lagos','Lagos','NG',6.5244,3.3792,'Africa/Lagos','ACTIVE')`,
			[]any{branch, business}},
	} {
		if _, err = pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	// A Section 12 stock basis, the row shape that existed before Section 14.
	// It hangs off a listing, variant, product and category, so the whole
	// catalogue chain is built rather than faked: a stock basis with dangling
	// parents would not be the row a real deployment is carrying.
	category := "cat_s14i_" + suffix
	product := "prd_s14i_" + suffix
	variant := "var_s14i_" + suffix
	listing := "lst_s14i_" + suffix
	basis := "sib_s14i_" + suffix

	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO shop_categories(id,business_id,name,slug)
		   VALUES($1,$2,'Section 14I upgrade',$3)`,
			[]any{category, business, "s14i-upgrade-" + suffix}},
		{`INSERT INTO shop_products(id,business_id,category_id,name,product_class)
		   VALUES($1,$2,$3,'Upgrade Product','PACKAGED')`,
			[]any{product, business, category}},
		{`INSERT INTO shop_variants(id,business_id,product_id,name)
		   VALUES($1,$2,$3,'Upgrade Variant')`,
			[]any{variant, business, product}},
		{`INSERT INTO shop_branch_listings(id,business_id,branch_id,product_id,variant_id)
		   VALUES($1,$2,$3,$4,$5)`,
			[]any{listing, business, branch, product, variant}},
		{`INSERT INTO shop_inventory_stock_bases(
			id,business_id,branch_id,listing_id,product_id,variant_id,
			measure_definition_id,measure_owner_key,consumption_mode,
			lot_policy,lot_tracking_enabled)
		   VALUES($1,$2,$3,$4,$5,$6,'smd_piece','PLATFORM','SAME_UNIT','FIFO',false)`,
			[]any{basis, business, branch, listing, product, variant}},
	} {
		if _, err = pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("building the Section 12 catalogue chain failed: %v", err)
		}
	}

	// Roll Section 14 off and back on underneath that row.
	migrations, err := ListMigrations(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	var section14 int
	for _, migration := range migrations {
		if strings.HasPrefix(migration.Version, "00006") && migration.Version >= "000060" {
			section14++
		}
	}

	for i := 0; i < section14; i++ {
		if err = RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
			t.Fatalf("rollback step %d: %v", i+1, err)
		}
	}

	var survives bool
	if err = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM shop_inventory_stock_bases WHERE id=$1)`,
		basis).Scan(&survives); err != nil {
		t.Fatal(err)
	}
	if !survives {
		t.Fatal("a Section 12 stock basis was destroyed by rolling Section 14 back")
	}

	if err = ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("reapply after the round trip: %v", err)
	}

	if err = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM shop_inventory_stock_bases WHERE id=$1)`,
		basis).Scan(&survives); err != nil {
		t.Fatal(err)
	}
	if !survives {
		t.Fatal("a Section 12 stock basis did not survive migrating through Section 14")
	}

	// Scenario E: the 14C identity bridge must be replay-safe. Running the
	// compatibility upsert twice must not produce two identities for one basis.
	for pass := 0; pass < 2; pass++ {
		if _, err = pool.Exec(ctx,
			`INSERT INTO inventory_items(
				id,business_id,branch_id,item_type,source_ref,
				stock_measure_ref,stock_measure_owner_key,stock_unit_kind,
				stock_basis_mode,tracking_mode)
			 VALUES($1,$2,$3,'SHOP_VARIANT',$4,'smd_piece','PLATFORM','COUNT','SAME_UNIT','SIMPLE')
			 ON CONFLICT (id) DO UPDATE SET updated_at = now()`,
			basis, business, branch, variant); err != nil {
			t.Fatalf("identity replay pass %d: %v", pass+1, err)
		}
	}

	var identities int
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM inventory_items WHERE id=$1`, basis).Scan(&identities); err != nil {
		t.Fatal(err)
	}
	if identities != 1 {
		t.Fatalf("replaying the identity bridge produced %d identities for one stock basis; want 1", identities)
	}
}
