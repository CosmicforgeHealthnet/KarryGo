package database

import (
	"context"
	"testing"
	"time"
)

func TestSection13AMigrationReplayAndBoundary(t *testing.T) {
	pool, migrationsDir := historicalMigrationPool(t, "000047")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var err error

	// Apply every currently known Vendor migration through Section 13A.
	if err = ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("fresh replay through Section 13A: %v", err)
	}

	var tip bool
	if err = pool.QueryRow(ctx, `
        SELECT EXISTS(
            SELECT 1
            FROM vendor_service_schema_migrations
            WHERE version='000047_vendor_canonical_taxonomy_foundation'
        )
    `).Scan(&tip); err != nil || !tip {
		t.Fatalf("Section 13A migration tip missing: tip=%v err=%v", tip, err)
	}

	// Section 13A canonical taxonomy foundation.
	for _, table := range []string{
		"canonical_taxonomy_namespaces",
		"canonical_taxonomy_versions",
		"canonical_taxonomy_facets",
		"canonical_taxonomy_nodes",
		"canonical_taxonomy_commands",
		"canonical_taxonomy_change_history",
		"canonical_taxonomy_publication_history",
	} {
		var exists bool
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`,
			table,
		).Scan(&exists); err != nil || !exists {
			t.Errorf("required Section 13A table %s missing: exists=%v err=%v", table, exists, err)
		}
	}

	// Only namespace identities are seeded in 13A.
	var foodCount, shopCount int
	if err = pool.QueryRow(ctx, `
        SELECT count(*)
        FROM canonical_taxonomy_namespaces
        WHERE code='FOOD'
    `).Scan(&foodCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `
        SELECT count(*)
        FROM canonical_taxonomy_namespaces
        WHERE code='SHOP'
    `).Scan(&shopCount); err != nil {
		t.Fatal(err)
	}
	if foodCount != 1 || shopCount != 1 {
		t.Fatalf("canonical namespace seed mismatch: FOOD=%d SHOP=%d", foodCount, shopCount)
	}

	var namespaceTotal int
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM canonical_taxonomy_namespaces`,
	).Scan(&namespaceTotal); err != nil {
		t.Fatal(err)
	}
	if namespaceTotal != 2 {
		t.Fatalf("unexpected Section 13A namespace seed count=%d want=2", namespaceTotal)
	}

	var facetSeedCount, nodeSeedCount int
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM canonical_taxonomy_facets`,
	).Scan(&facetSeedCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM canonical_taxonomy_nodes`,
	).Scan(&nodeSeedCount); err != nil {
		t.Fatal(err)
	}
	if facetSeedCount != 0 || nodeSeedCount != 0 {
		t.Fatalf(
			"Section 13B content leaked into 13A: facets=%d nodes=%d",
			facetSeedCount,
			nodeSeedCount,
		)
	}

	// Search infrastructure belongs to later Section 13 phases.
	for _, table := range []string{
		"food_search_documents",
		"shop_search_documents",
		"search_projection_generations",
	} {
		var exists bool
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`,
			table,
		).Scan(&exists); err != nil {
			t.Errorf("check later-phase table %s: %v", table, err)
			continue
		}
		if exists {
			t.Errorf("later Section 13 table %s leaked into 13A", table)
		}
	}

	// Existing Shop authority must still exist before and after 13A rollback.
	var shopCategoryExists, shopInventoryExists bool
	if err = pool.QueryRow(ctx,
		`SELECT to_regclass('public.shop_categories') IS NOT NULL`,
	).Scan(&shopCategoryExists); err != nil || !shopCategoryExists {
		t.Fatalf("Section 12 Shop catalogue missing before rollback: exists=%v err=%v", shopCategoryExists, err)
	}
	if err = pool.QueryRow(ctx,
		`SELECT to_regclass('public.shop_inventory_balances') IS NOT NULL`,
	).Scan(&shopInventoryExists); err != nil || !shopInventoryExists {
		t.Fatalf("Section 12 Shop inventory missing before rollback: exists=%v err=%v", shopInventoryExists, err)
	}

	// Roll back exactly the latest migration: 000047.
	if err = RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("rollback Section 13A: %v", err)
	}

	for _, table := range []string{
		"canonical_taxonomy_namespaces",
		"canonical_taxonomy_versions",
		"canonical_taxonomy_facets",
		"canonical_taxonomy_nodes",
		"canonical_taxonomy_commands",
		"canonical_taxonomy_change_history",
		"canonical_taxonomy_publication_history",
	} {
		var gone bool
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NULL`,
			table,
		).Scan(&gone); err != nil || !gone {
			t.Errorf("Section 13A rollback left table %s: gone=%v err=%v", table, gone, err)
		}
	}

	// Rollback must not damage Section 12.
	if err = pool.QueryRow(ctx,
		`SELECT to_regclass('public.shop_categories') IS NOT NULL`,
	).Scan(&shopCategoryExists); err != nil || !shopCategoryExists {
		t.Fatalf("Section 12 Shop catalogue damaged by 13A rollback: exists=%v err=%v", shopCategoryExists, err)
	}
	if err = pool.QueryRow(ctx,
		`SELECT to_regclass('public.shop_inventory_balances') IS NOT NULL`,
	).Scan(&shopInventoryExists); err != nil || !shopInventoryExists {
		t.Fatalf("Section 12 Shop inventory damaged by 13A rollback: exists=%v err=%v", shopInventoryExists, err)
	}

	// Reapply Section 13A.
	if err = ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("reapply Section 13A: %v", err)
	}

	if err = pool.QueryRow(ctx, `
        SELECT EXISTS(
            SELECT 1
            FROM vendor_service_schema_migrations
            WHERE version='000047_vendor_canonical_taxonomy_foundation'
        )
    `).Scan(&tip); err != nil || !tip {
		t.Fatalf("Section 13A tip missing after reapply: tip=%v err=%v", tip, err)
	}

	var taxonomyExists bool
	if err = pool.QueryRow(ctx,
		`SELECT to_regclass('public.canonical_taxonomy_namespaces') IS NOT NULL`,
	).Scan(&taxonomyExists); err != nil || !taxonomyExists {
		t.Fatalf("Section 13A tables missing after reapply: exists=%v err=%v", taxonomyExists, err)
	}
}
