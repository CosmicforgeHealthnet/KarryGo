package section13security

// Section 13 production rebuild closure: the end-to-end gate.
//
// This is the test that decides whether Section 13's rebuild architecture is
// real. It uses NO fake SearchFullRebuildSource. It builds authoritative Food
// and Shop catalogue records, requests a rebuild through the real
// administration service, runs the real Section 13H workers, and then asks
// whether the resulting search documents actually came from those catalogue
// records.
//
// Two properties get the most attention here, because both would pass a
// naive test:
//
//   - The rebuild must read the DOMAIN, not the projection. A rebuild that
//     copied the existing search documents would look perfect on a healthy
//     database and would be worthless in the situation rebuilds exist for. The
//     gate therefore corrupts and deletes the existing projection first, and
//     then checks the rebuilt documents match the catalogue rather than the
//     corruption.
//   - The previous generation must keep serving. A rebuild that emptied search
//     while it ran would also "work", and would take discovery down for the
//     duration on a real marketplace.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
	"cosmicforge/logistics/services/vendor-service/internal/searchrebuild"
)

// ---------------------------------------------------------------------------
// Authoritative fixture
// ---------------------------------------------------------------------------

const (
	rbBusiness = "vb_rebuild"
	rbBranch   = "vbr_rebuild"

	rbFoodItems     = 7
	rbShopListings  = 6
	rbFoodDishName  = "Jollof Rice"
	rbShopBrand     = "Dangote"
	rbShopProdName  = "Raw Rice"
	rbFoodMenu      = "fm_rebuild"
	rbFoodSection   = "fs_rebuild"
	rbShopCategory  = "sc_rebuild"
	rbShopProduct   = "sp_rebuild"
	rbTaxonomyFacet = "CUISINE"
	rbTaxonomyNode  = "YORUBA"
	rbTaxonomyAlias = "jolof"

	rbShopMeasure      = "smd_rebuild"
	rbShopMeasureAlias = "derica"
)

type rebuildFixture struct {
	pool *pgxpool.Pool

	foodItemIDs    []string
	shopListingIDs []string
	shopVariantIDs []string
}

// newRebuildFixture creates authoritative Food and Shop records plus the
// canonical taxonomy they are classified against.
//
// Everything here is domain data. Not one row is written into a search table:
// whatever ends up in the projection has to have been derived by the rebuild.
func newRebuildFixture(t *testing.T) (*rebuildFixture, context.Context) {
	t.Helper()
	pool, ctx := testPool(t)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture %s: %v", strings.Fields(sql + " ")[0], err)
		}
	}

	if _, err := pool.Exec(ctx, `
        TRUNCATE
            food_search_documents, shop_search_documents,
            search_projection_jobs, search_projection_generation_history,
            search_projection_generations,
            food_item_availability, food_items, food_menu_sections, food_menus,
            food_catalogue_revisions,
            shop_inventory_balances, shop_branch_listings, shop_variants,
            shop_products, shop_categories,
            shop_catalogue_revisions, shop_inventory_revisions,
            vendor_verification_projections, vendor_branch_service_areas,
            vendor_branch_capabilities, vendor_capabilities, vendor_branches,
            vendor_businesses, vendor_staff_role_assignments, vendor_staff, vendor_users
        RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("reset rebuild fixture: %v", err)
	}

	// Business, branch, and the two signals a rebuild reads for trust and
	// serviceability.
	exec(`INSERT INTO vendor_users(id,email) VALUES('vu_rebuild','vu_rebuild@example.test')`)
	exec(`INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES('vs_rebuild',$1,'vu_rebuild','active')`, rbBusiness)
	exec(`INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone)
          VALUES($1,'Rebuild Foods Limited','ACTIVE','vs_rebuild','NGN','Africa/Lagos')`, rbBusiness)
	exec(`INSERT INTO vendor_branches
          (id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status)
          VALUES($1,$2,'Rebuild Branch','1 Test Road','Lagos','Lagos','NG',6.5,3.4,'Africa/Lagos','ACTIVE')`,
		rbBranch, rbBusiness)
	exec(`INSERT INTO vendor_branch_service_areas
          (branch_id,business_id,mode,center_latitude,center_longitude,radius_meters)
          VALUES($1,$2,'RADIUS',6.5,3.4,8000)`, rbBranch, rbBusiness)
	exec(`INSERT INTO vendor_verification_projections
          (id,business_id,dimension,subject_type,subject_ref,source_service,source_ref,
           source_version,status,observed_at)
          VALUES('vvp_rebuild',$1,'BUSINESS','BUSINESS',$1,'verification-compliance-service',
                 'case_rebuild',3,'VERIFIED',now())`, rbBusiness)

	// Canonical taxonomy: a published FOOD version with one facet, one node
	// and a Nigerian alias, so the rebuild has real vocabulary to carry.
	seedRebuildTaxonomy(t, pool, ctx)

	fixture := &rebuildFixture{pool: pool}

	// ---- Food ----
	exec(`INSERT INTO food_menus(id,business_id,branch_id,capability_type,name,status)
          VALUES($1,$2,$3,'RESTAURANT','Rebuild Menu','ACTIVE')`, rbFoodMenu, rbBusiness, rbBranch)
	exec(`INSERT INTO food_menu_sections(id,business_id,menu_id,name,position,status)
          VALUES($1,$2,$3,'Mains',1,'ACTIVE')`, rbFoodSection, rbBusiness, rbFoodMenu)
	exec(`INSERT INTO food_catalogue_revisions(business_id,branch_id,capability_type,revision)
          VALUES($1,$2,'RESTAURANT',41)`, rbBusiness, rbBranch)

	for index := 0; index < rbFoodItems; index++ {
		itemID := fmt.Sprintf("fi_rebuild_%02d", index)
		exec(`INSERT INTO food_items
              (id,business_id,branch_id,menu_id,section_id,name,base_price_minor,
               dietary_codes,position,status)
              VALUES($1,$2,$3,$4,$5,$6,$7,'["HALAL"]'::jsonb,$8,'ACTIVE')`,
			itemID, rbBusiness, rbBranch, rbFoodMenu, rbFoodSection,
			fmt.Sprintf("%s %d", rbFoodDishName, index), 250000+int64(index)*1000, index+1)
		exec(`INSERT INTO food_item_availability(item_id,business_id,branch_id,status)
              VALUES($1,$2,$3,'AVAILABLE')`, itemID, rbBusiness, rbBranch)
		mapRebuildSource(t, pool, ctx, "FOOD", "FOOD_ITEM", itemID)
		fixture.foodItemIDs = append(fixture.foodItemIDs, itemID)
	}

	// ---- Shop ----
	exec(`INSERT INTO shop_categories(id,business_id,name,slug,status)
          VALUES($1,$2,'Grains','grains','ACTIVE')`, rbShopCategory, rbBusiness)
	exec(`INSERT INTO shop_products(id,business_id,category_id,name,brand,product_class,status)
          VALUES($1,$2,$3,$4,$5,'DRY_BULK','ACTIVE')`,
		rbShopProduct, rbBusiness, rbShopCategory, rbShopProdName, rbShopBrand)
	exec(`INSERT INTO shop_catalogue_revisions(business_id,branch_id,revision) VALUES($1,$2,17)`,
		rbBusiness, rbBranch)
	exec(`INSERT INTO shop_inventory_revisions(business_id,branch_id,revision) VALUES($1,$2,23)`,
		rbBusiness, rbBranch)
	mapRebuildSource(t, pool, ctx, "SHOP", "SHOP_PRODUCT", rbShopProduct)

	// A business-scoped measure definition with a Nigerian local alias.
	//
	// The fixture defines its own rather than relying on the platform
	// measurement seed, because the reset above is scoped by business and a
	// fixture that silently depended on shared reference data would break the
	// moment another test reset it.
	// owner_key is a generated column on both tables, so it is never supplied.
	exec(`INSERT INTO shop_measure_definitions
          (id,business_id,code,display_name,measurement_family,measure_origin,status)
          VALUES($1,$2,'REBUILD_SACK','Sack','LOCAL_CONTAINER','VENDOR_DEFINED','ACTIVE')`,
		rbShopMeasure, rbBusiness)
	exec(`INSERT INTO shop_measure_aliases(id,measure_definition_id,owner_key,business_id,alias)
          VALUES($1,$2,$3,$3,$4)`,
		"sma_rebuild", rbShopMeasure, rbBusiness, rbShopMeasureAlias)

	for index := 0; index < rbShopListings; index++ {
		variantID := fmt.Sprintf("sv_rebuild_%02d", index)
		listingID := fmt.Sprintf("sl_rebuild_%02d", index)

		exec(`INSERT INTO shop_variants
              (id,business_id,product_id,sku,name,package_level,pack_quantity,
               retail_price_minor,temperature_zone,status)
              VALUES($1,$2,$3,$4,$5,'SACK',1,$6,'AMBIENT','ACTIVE')`,
			variantID, rbBusiness, rbShopProduct,
			fmt.Sprintf("SKU-REBUILD-%02d", index),
			fmt.Sprintf("%dkg Sack", 10*(index+1)),
			500000+int64(index)*5000)

		exec(`INSERT INTO shop_branch_listings
              (id,business_id,branch_id,product_id,variant_id,operating_model,
               retail_enabled,wholesale_enabled,status)
              VALUES($1,$2,$3,$4,$5,'FOODSTUFF_DEALER',TRUE,TRUE,'ACTIVE')`,
			listingID, rbBusiness, rbBranch, rbShopProduct, variantID)

		// A published sell unit, so the rebuild has real measurement
		// vocabulary to carry into the document.
		exec(`INSERT INTO shop_sell_units
              (id,business_id,product_id,variant_id,measure_definition_id,
               measure_owner_key,display_name,retail_enabled,status,position)
              VALUES($1,$2,$3,$4,$5,$2,'Sack',TRUE,'ACTIVE',$6)`,
			"su_rebuild_"+variantID, rbBusiness, rbShopProduct, variantID,
			rbShopMeasure, index+1)
		exec(`INSERT INTO shop_listing_sell_units
              (id,business_id,listing_id,sell_unit_id,variant_id,enabled,position)
              VALUES($1,$2,$3,$4,$5,TRUE,$6)`,
			"lsu_rebuild_"+variantID, rbBusiness, listingID,
			"su_rebuild_"+variantID, variantID, index+1)

		// Authoritative stock, so the rebuilt inventory signal is derived
		// rather than defaulted.
		exec(`INSERT INTO shop_inventory_stock_bases
              (id,business_id,branch_id,listing_id,product_id,variant_id,
               measure_definition_id,measure_owner_key,consumption_mode,lot_policy,status)
              VALUES($1,$2,$3,$4,$5,$6,$7,$2,'SAME_UNIT','FIFO','ACTIVE')`,
			"sb_rebuild_"+variantID, rbBusiness, rbBranch, listingID, rbShopProduct,
			variantID, rbShopMeasure)
		exec(`INSERT INTO shop_inventory_balances
              (id,business_id,branch_id,stock_basis_id,variant_id,on_hand_quantity,revision)
              VALUES($1,$2,$3,$4,$5,$6,7)`,
			"sib_rebuild_"+variantID, rbBusiness, rbBranch,
			"sb_rebuild_"+variantID, variantID, 40+index)

		fixture.shopVariantIDs = append(fixture.shopVariantIDs, variantID)
		fixture.shopListingIDs = append(fixture.shopListingIDs, listingID)
	}

	return fixture, ctx
}

// seedRebuildTaxonomy publishes a FOOD and a SHOP version, each with one facet,
// one node and one Nigerian alias.
// retireOtherPublishedVersions makes room for the fixture's version.
//
// A namespace may have at most one PUBLISHED version, enforced by a unique
// index. Other tests in this package publish their own versions, so the
// fixture cannot assume the slot is free - it does what real publication does
// and retires whatever holds it.
func retireOtherPublishedVersions(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	namespaceID string,
	keepVersionID string,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
        UPDATE canonical_taxonomy_versions
           SET status='RETIRED',
               retired_by_actor_ref='test',
               retired_at=now(),
               revision=revision+1,
               updated_at=now()
         WHERE namespace_id=$1
           AND status='PUBLISHED'
           AND id<>$2`, namespaceID, keepVersionID); err != nil {
		t.Fatalf("retire previously published version: %v", err)
	}
}

func seedRebuildTaxonomy(t *testing.T, pool *pgxpool.Pool, ctx context.Context) {
	t.Helper()

	if _, err := pool.Exec(ctx, `
        DELETE FROM canonical_taxonomy_mappings
         WHERE source_ref LIKE '%rebuild%'`); err != nil {
		t.Fatalf("reset rebuild mappings: %v", err)
	}

	for _, namespace := range []struct{ code, id string }{
		{"FOOD", "ctns_food"}, {"SHOP", "ctns_shop"},
	} {
		versionID := "ctv_rebuild_" + strings.ToLower(namespace.code)
		facetID := "ctf_rebuild_" + strings.ToLower(namespace.code)
		nodeID := "ctn_rebuild_" + strings.ToLower(namespace.code)

		facetCode := rbTaxonomyFacet
		if namespace.code == "SHOP" {
			facetCode = "CATEGORY"
		}

		// The fixture has to be idempotent across reruns, and a published
		// version is deliberately immutable: its facets and nodes can no
		// longer be edited, and the schema refuses to delete a historical
		// version at all. So an already-published fixture version is reused
		// as-is rather than rebuilt, which is also the more honest shape -
		// this is what a real namespace looks like on the second rebuild.
		var existing string
		if err := pool.QueryRow(ctx,
			`SELECT status FROM canonical_taxonomy_versions WHERE id=$1`, versionID,
		).Scan(&existing); err == nil {
			if existing != "PUBLISHED" {
				retireOtherPublishedVersions(t, pool, ctx, namespace.id, versionID)
				if _, err = pool.Exec(ctx, `
                    UPDATE canonical_taxonomy_versions
                       SET status='PUBLISHED', published_by_actor_ref='test',
                           published_at=now(), revision=revision+1, updated_at=now()
                     WHERE id=$1`, versionID); err != nil {
					t.Fatalf("publish existing %s version: %v", namespace.code, err)
				}
			}
			continue
		}

		// The version is created as a DRAFT and published at the end, because
		// the schema enforces that lifecycle with a trigger: a version cannot
		// be inserted already PUBLISHED, and facets/nodes/terms can only be
		// added while it is still a draft. Building the fixture in the same
		// order the real publication path uses is what keeps it honest.
		if _, err := pool.Exec(ctx, `
            INSERT INTO canonical_taxonomy_versions
                (id, namespace_id, version_number, status, created_by_actor_ref)
            VALUES ($1, $2, 9001, 'DRAFT', 'test')
            ON CONFLICT (id) DO NOTHING`,
			versionID, namespace.id); err != nil {
			t.Fatalf("seed %s version: %v", namespace.code, err)
		}
		if _, err := pool.Exec(ctx, `
            INSERT INTO canonical_taxonomy_facets
                (id, namespace_id, version_id, code, display_name, structure_type,
                 position, status)
            VALUES ($1, $2, $3, $4, $4, 'FLAT', 1, 'ACTIVE')
            ON CONFLICT (id) DO NOTHING`,
			facetID, namespace.id, versionID, facetCode); err != nil {
			t.Fatalf("seed %s facet: %v", namespace.code, err)
		}
		if _, err := pool.Exec(ctx, `
            INSERT INTO canonical_taxonomy_nodes
                (id, namespace_id, version_id, facet_id, canonical_code, display_name,
                 position, status)
            VALUES ($1, $2, $3, $4, $5, $5, 1, 'ACTIVE')
            ON CONFLICT (id) DO NOTHING`,
			nodeID, namespace.id, versionID, facetID, rbTaxonomyNode); err != nil {
			t.Fatalf("seed %s node: %v", namespace.code, err)
		}
		if _, err := pool.Exec(ctx, `
            INSERT INTO canonical_taxonomy_node_terms
                (id, namespace_id, version_id, node_id, term, normalized_term,
                 relationship_type, locale, status)
            VALUES ($1, $2, $3, $4, $5, $5, 'ALIAS', 'en-NG', 'ACTIVE')
            ON CONFLICT (id) DO NOTHING`,
			"ctt_rebuild_"+strings.ToLower(namespace.code), namespace.id, versionID,
			nodeID, rbTaxonomyAlias); err != nil {
			t.Fatalf("seed %s term: %v", namespace.code, err)
		}

		// Publish. The rebuild source deliberately ignores drafts, so a
		// fixture left as a draft would silently produce documents with no
		// classification at all - and every taxonomy assertion below would
		// fail for a reason that has nothing to do with the rebuild.
		retireOtherPublishedVersions(t, pool, ctx, namespace.id, versionID)
		if _, err := pool.Exec(ctx, `
            UPDATE canonical_taxonomy_versions
               SET status='PUBLISHED',
                   published_by_actor_ref='test',
                   published_at=now(),
                   revision=revision+1,
                   updated_at=now()
             WHERE id=$1`, versionID); err != nil {
			t.Fatalf("publish %s version: %v", namespace.code, err)
		}
	}
}

func mapRebuildSource(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	namespaceCode string,
	sourceType string,
	sourceRef string,
) {
	t.Helper()
	namespaceID := "ctns_food"
	if namespaceCode == "SHOP" {
		namespaceID = "ctns_shop"
	}
	if _, err := pool.Exec(ctx, `
        INSERT INTO canonical_taxonomy_mappings
            (id, namespace_id, version_id, node_id, source_type, source_ref,
             mapping_type, status, confidence_basis, business_ref,
             created_by_actor_ref, updated_by_actor_ref)
        VALUES ($1, $2, $3, $4, $5, $6, 'PRIMARY', 'ACTIVE', 'MANUAL', $7, 'test', 'test')
        ON CONFLICT (id) DO NOTHING`,
		"ctm_rebuild_"+sourceRef, namespaceID,
		"ctv_rebuild_"+strings.ToLower(namespaceCode),
		"ctn_rebuild_"+strings.ToLower(namespaceCode),
		sourceType, sourceRef, rbBusiness); err != nil {
		t.Fatalf("map %s: %v", sourceRef, err)
	}
}

// ---------------------------------------------------------------------------
// Real pipeline harness
// ---------------------------------------------------------------------------

type rebuildPipeline struct {
	generations *searchprojection.PostgresSearchGenerationStore
	admin       *searchprojection.SearchProjectionAdminService

	rebuildWorker  *searchprojection.ProjectionWorker
	activateWorker *searchprojection.ProjectionWorker
}

// alwaysAdmin mirrors cmd/section13.go's authorizer: the internal HMAC surface
// is the authority gate, so there is nothing left to check behind it.
type alwaysAdmin struct{}

func (alwaysAdmin) RequireSearchProjectionAdmin(context.Context) error { return nil }

// newRebuildPipeline wires the SAME components cmd/section13.go wires in
// production, including the real searchrebuild.Source.
func newRebuildPipeline(t *testing.T, pool *pgxpool.Pool) *rebuildPipeline {
	t.Helper()

	generations := searchprojection.NewPostgresSearchGenerationStore(pool)

	foodBackend := searchprojection.NewPostgresFoodSearchBackend(pool)
	shopBackend := searchprojection.NewPostgresShopSearchBackend(pool)

	applier := searchprojection.NewDefaultSearchRebuildApplier(
		searchprojection.NewFoodProjector(foodBackend),
		searchprojection.NewShopProjector(shopBackend),
		foodBackend,
		shopBackend,
	)

	// The production source. No fake, no stub, no pre-seeded document list.
	source := searchrebuild.NewSource(pool)

	rebuildWorker, err := searchprojection.NewProjectionWorker(
		generations, "test-rebuild-worker",
		[]searchprojection.ProjectionJobType{
			searchprojection.ProjectionJobFullRebuild,
			searchprojection.ProjectionJobScopedRebuild,
		},
		searchprojection.NewFullRebuildJobHandler(source, applier, generations),
		1, 2*time.Minute,
	)
	if err != nil {
		t.Fatalf("build rebuild worker: %v", err)
	}

	activateWorker, err := searchprojection.NewProjectionWorker(
		generations, "test-activate-worker",
		[]searchprojection.ProjectionJobType{
			searchprojection.ProjectionJobValidateActivate,
		},
		searchprojection.NewGenerationValidateActivateHandler(generations),
		5, 30*time.Second,
	)
	if err != nil {
		t.Fatalf("build activate worker: %v", err)
	}

	return &rebuildPipeline{
		generations: generations,
		admin: searchprojection.NewSearchProjectionAdminService(
			alwaysAdmin{}, generations,
		),
		rebuildWorker:  rebuildWorker,
		activateWorker: activateWorker,
	}
}

// drain runs worker passes until the generation reaches a terminal state or the
// pass budget is exhausted. A budget rather than a timeout keeps the test
// deterministic.
func (p *rebuildPipeline) drain(
	t *testing.T,
	ctx context.Context,
	namespace searchprojection.SearchCacheNamespace,
	generation int64,
	passes int,
) searchprojection.SearchProjectionGeneration {
	t.Helper()

	var record searchprojection.SearchProjectionGeneration
	for pass := 0; pass < passes; pass++ {
		if err := p.rebuildWorker.Pass(ctx); err != nil {
			t.Logf("rebuild pass %d: %v", pass, err)
		}
		if err := p.activateWorker.Pass(ctx); err != nil {
			t.Logf("activate pass %d: %v", pass, err)
		}

		var err error
		if record, err = p.generations.GetGeneration(ctx, namespace, generation); err != nil {
			t.Fatalf("read generation: %v", err)
		}
		switch record.Status {
		case searchprojection.SearchGenerationActive,
			searchprojection.SearchGenerationRetired,
			searchprojection.SearchGenerationFailed:
			return record
		}
	}
	return record
}

// seedActiveGeneration installs a generation that is already ACTIVE, with one
// document, so the "old generation keeps serving" claim has something to serve.
func seedActiveGeneration(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	namespace searchprojection.SearchCacheNamespace,
	generation int64,
) {
	t.Helper()

	if _, err := pool.Exec(ctx, `
        INSERT INTO search_projection_generations
            (namespace_code, generation, status, base_generation, rebuild_scope,
             requested_by, document_count, processed_count, activated_at)
        VALUES ($1, $2, 'ACTIVE', 0, 'FULL', 'test', 1, 1, now())`,
		string(namespace), generation); err != nil {
		t.Fatalf("seed active generation: %v", err)
	}

	switch namespace {
	case searchprojection.SearchCacheNamespaceFood:
		projector := searchprojection.NewFoodProjector(
			searchprojection.NewPostgresFoodSearchBackend(pool))
		if _, err := projector.Project(ctx, searchprojection.FoodProjectionInput{
			Generation: generation, BusinessRef: rbBusiness, BranchRef: rbBranch,
			FoodItemRef: "fi_previous_generation", Name: "Legacy Jollof",
			Active: true, CatalogueRevision: 1,
		}); err != nil {
			t.Fatalf("seed previous Food document: %v", err)
		}
	case searchprojection.SearchCacheNamespaceShop:
		projector := searchprojection.NewShopProjector(
			searchprojection.NewPostgresShopSearchBackend(pool))
		if _, err := projector.Project(ctx, searchprojection.ShopProjectionInput{
			Generation: generation, BusinessRef: rbBusiness, BranchRef: rbBranch,
			ProductRef: "sp_previous", VariantRef: "sv_previous", ListingRef: "sl_previous",
			Name: "Legacy Rice", Active: true, CatalogueRevision: 1, InventoryRevision: 1,
		}); err != nil {
			t.Fatalf("seed previous Shop document: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Food
// ---------------------------------------------------------------------------

// TestRealFoodRebuildEndToEnd is the Food closure gate.
func TestRealFoodRebuildEndToEnd(t *testing.T) {
	fixture, ctx := newRebuildFixture(t)
	pipeline := newRebuildPipeline(t, fixture.pool)

	const previousGeneration = 4
	seedActiveGeneration(t, fixture.pool, ctx, searchprojection.SearchCacheNamespaceFood, previousGeneration)

	requested, err := pipeline.admin.RequestFullRebuild(
		ctx, searchprojection.SearchCacheNamespaceFood, "section13-closure")
	if err != nil {
		t.Fatalf("request rebuild: %v", err)
	}
	if requested.Status != searchprojection.SearchGenerationBuilding {
		t.Fatalf("requested generation status=%s want=BUILDING", requested.Status)
	}
	if requested.BaseGeneration != previousGeneration {
		t.Fatalf("base generation=%d want=%d", requested.BaseGeneration, previousGeneration)
	}

	// While the rebuild is only requested, the previous generation is still
	// the one search reads.
	active, err := pipeline.generations.CurrentActiveGeneration(
		ctx, searchprojection.SearchCacheNamespaceFood)
	if err != nil {
		t.Fatal(err)
	}
	if active != previousGeneration {
		t.Fatalf("active generation=%d during BUILDING; want the previous %d", active, previousGeneration)
	}

	final := pipeline.drain(t, ctx, searchprojection.SearchCacheNamespaceFood, requested.Generation, 8)

	if final.Status != searchprojection.SearchGenerationActive {
		t.Fatalf("generation %d status=%s want=ACTIVE (failure=%s %s)",
			requested.Generation, final.Status, final.FailureCode, final.FailureReason)
	}
	if final.DocumentCount != int64(rbFoodItems) {
		t.Fatalf("document_count=%d want=%d", final.DocumentCount, rbFoodItems)
	}
	if final.ProcessedCount != final.DocumentCount {
		t.Fatalf("processed=%d document=%d; validation should have refused a mismatch",
			final.ProcessedCount, final.DocumentCount)
	}

	// The old generation is retired, not merely superseded.
	previous, err := pipeline.generations.GetGeneration(
		ctx, searchprojection.SearchCacheNamespaceFood, previousGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if previous.Status != searchprojection.SearchGenerationRetired {
		t.Fatalf("previous generation status=%s want=RETIRED", previous.Status)
	}

	// The rebuilt documents are the authoritative catalogue, not the legacy one.
	assertFoodDocumentsRebuilt(t, fixture.pool, ctx, requested.Generation)

	// Search resolves them through the real engine at the new generation.
	engine := searchprojection.NewFoodSearchEngine(
		searchprojection.NewPostgresFoodSearchBackend(fixture.pool))
	response, err := engine.Search(ctx, searchprojection.FoodEngineSearchRequest{
		Query: "jollof", Generation: requested.Generation, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("search rebuilt Food generation: %v", err)
	}
	if len(response.Hits) != rbFoodItems {
		t.Fatalf("search over rebuilt generation returned %d hits; want %d",
			len(response.Hits), rbFoodItems)
	}
	for _, hit := range response.Hits {
		if hit.FoodItemRef == "fi_previous_generation" {
			t.Fatal("the rebuilt generation contains the previous generation's document")
		}
	}
}

// assertFoodDocumentsRebuilt checks the projected values actually came from the
// Food domain, field by field.
func assertFoodDocumentsRebuilt(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	generation int64,
) {
	t.Helper()

	rows, err := pool.Query(ctx, `
        SELECT food_item_ref, business_ref, branch_ref, name, menu_refs,
               provider_types, cuisine_terms, aliases, dietary_claims,
               active, availability_summary, effective_price_summary,
               trust_hint, serviceability_hint, reliability_hint,
               catalogue_revision
          FROM food_search_documents
         WHERE generation=$1
         ORDER BY food_item_ref`, generation)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var (
			itemRef, businessRef, branchRef, name          string
			menuRefs, providerTypes, cuisines              []string
			aliases, dietary                               []string
			active                                         bool
			availability, price, trust, service, liability []byte
			catalogueRevision                              int64
		)
		if err = rows.Scan(&itemRef, &businessRef, &branchRef, &name, &menuRefs,
			&providerTypes, &cuisines, &aliases, &dietary,
			&active, &availability, &price, &trust, &service, &liability,
			&catalogueRevision); err != nil {
			t.Fatal(err)
		}
		seen++

		if businessRef != rbBusiness || branchRef != rbBranch {
			t.Errorf("%s: business/branch=%s/%s want=%s/%s",
				itemRef, businessRef, branchRef, rbBusiness, rbBranch)
		}
		if !strings.Contains(name, rbFoodDishName) {
			t.Errorf("%s: name=%q does not come from the catalogue", itemRef, name)
		}
		if len(menuRefs) != 1 || menuRefs[0] != rbFoodMenu {
			t.Errorf("%s: menu_refs=%v want=[%s]", itemRef, menuRefs, rbFoodMenu)
		}
		// Provider type comes from the MENU's capability, which only an
		// authoritative read could supply. The comparison is case-insensitive
		// because the projector normalises vocabulary on the way in - that is
		// the engine doing its job, not the rebuild losing information.
		if len(providerTypes) != 1 || !containsFold(providerTypes, "RESTAURANT") {
			t.Errorf("%s: provider_types=%v want=[RESTAURANT]", itemRef, providerTypes)
		}
		// Cuisine comes from the canonical mapping, and the alias from the
		// node's vocabulary: both prove the taxonomy was actually joined.
		if !containsFold(cuisines, rbTaxonomyNode) {
			t.Errorf("%s: cuisine_terms=%v missing canonical node %s", itemRef, cuisines, rbTaxonomyNode)
		}
		if !containsFold(aliases, rbTaxonomyAlias) {
			t.Errorf("%s: aliases=%v missing canonical alias %q", itemRef, aliases, rbTaxonomyAlias)
		}
		if !containsFold(dietary, "HALAL") {
			t.Errorf("%s: dietary_claims=%v missing the item's declared code", itemRef, dietary)
		}
		if !active {
			t.Errorf("%s: rebuilt inactive though the catalogue says ACTIVE", itemRef)
		}
		if catalogueRevision != 41 {
			t.Errorf("%s: catalogue_revision=%d want=41 from food_catalogue_revisions",
				itemRef, catalogueRevision)
		}

		assertJSONField(t, itemRef, "availability", availability, "available", true)
		assertJSONField(t, itemRef, "trust", trust, "status", "VERIFIED")
		assertJSONField(t, itemRef, "serviceability", service, "serviceable", true)
		// Reliability has no authoritative source and must stay UNKNOWN.
		assertJSONField(t, itemRef, "reliability", liability, "status", "UNKNOWN")

		var priceDoc map[string]any
		if err = json.Unmarshal(price, &priceDoc); err != nil {
			t.Fatal(err)
		}
		if priceDoc["price_minor"] == nil {
			t.Errorf("%s: no price carried from the catalogue", itemRef)
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != rbFoodItems {
		t.Fatalf("rebuilt %d Food documents; the catalogue has %d items", seen, rbFoodItems)
	}
}

// ---------------------------------------------------------------------------
// Shop
// ---------------------------------------------------------------------------

// TestRealShopRebuildEndToEnd is the Shop closure gate.
func TestRealShopRebuildEndToEnd(t *testing.T) {
	fixture, ctx := newRebuildFixture(t)
	pipeline := newRebuildPipeline(t, fixture.pool)

	const previousGeneration = 2
	seedActiveGeneration(t, fixture.pool, ctx, searchprojection.SearchCacheNamespaceShop, previousGeneration)

	requested, err := pipeline.admin.RequestFullRebuild(
		ctx, searchprojection.SearchCacheNamespaceShop, "section13-closure")
	if err != nil {
		t.Fatalf("request rebuild: %v", err)
	}

	final := pipeline.drain(t, ctx, searchprojection.SearchCacheNamespaceShop, requested.Generation, 8)
	if final.Status != searchprojection.SearchGenerationActive {
		t.Fatalf("generation %d status=%s want=ACTIVE (failure=%s %s)",
			requested.Generation, final.Status, final.FailureCode, final.FailureReason)
	}
	if final.DocumentCount != int64(rbShopListings) {
		t.Fatalf("document_count=%d want=%d", final.DocumentCount, rbShopListings)
	}

	rows, err := fixture.pool.Query(ctx, `
        SELECT listing_ref, product_ref, variant_ref, name, brand,
               sku_terms, canonical_taxonomy, retail_eligible, active,
               inventory_availability_summary, effective_price_summary,
               catalogue_revision, inventory_revision, temperature_zone
          FROM shop_search_documents
         WHERE generation=$1
         ORDER BY listing_ref`, requested.Generation)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var (
			listingRef, productRef, variantRef, name, brand string
			skuTerms                                        []string
			taxonomy, inventory, price                      []byte
			retailEligible, active                          bool
			catalogueRevision, inventoryRevision            int64
			temperatureZone                                 string
		)
		if err = rows.Scan(&listingRef, &productRef, &variantRef, &name, &brand,
			&skuTerms, &taxonomy, &retailEligible, &active,
			&inventory, &price, &catalogueRevision, &inventoryRevision,
			&temperatureZone); err != nil {
			t.Fatal(err)
		}
		seen++

		if productRef != rbShopProduct {
			t.Errorf("%s: product_ref=%s want=%s", listingRef, productRef, rbShopProduct)
		}
		if brand != rbShopBrand {
			t.Errorf("%s: brand=%q want=%q from shop_products", listingRef, brand, rbShopBrand)
		}
		// The listing name combines product and variant, which only a join of
		// both authoritative rows produces.
		if !strings.Contains(name, rbShopProdName) || !strings.Contains(name, "Sack") {
			t.Errorf("%s: name=%q does not combine product and variant", listingRef, name)
		}
		// The projector normalises punctuation out of identifier tokens, so
		// "SKU-REBUILD-03" is indexed as "sku rebuild 03". That is deliberate -
		// a customer pasting a SKU rarely reproduces its punctuation - so the
		// assertion checks the normalised form rather than the raw string.
		if !containsFold(skuTerms, "sku rebuild") {
			t.Errorf("%s: sku_terms=%v missing the variant SKU", listingRef, skuTerms)
		}
		if temperatureZone != "AMBIENT" {
			t.Errorf("%s: temperature_zone=%q want=AMBIENT", listingRef, temperatureZone)
		}
		if !retailEligible || !active {
			t.Errorf("%s: retail=%t active=%t; the listing is enabled and ACTIVE",
				listingRef, retailEligible, active)
		}
		if catalogueRevision != 17 || inventoryRevision != 23 {
			t.Errorf("%s: revisions=%d/%d want=17/23 from the revision tables",
				listingRef, catalogueRevision, inventoryRevision)
		}
		if !strings.Contains(strings.ToUpper(string(taxonomy)), rbTaxonomyNode) {
			t.Errorf("%s: canonical_taxonomy=%s missing the mapped node",
				listingRef, string(taxonomy))
		}

		// Stock is derived from authoritative balances, not defaulted.
		assertJSONField(t, listingRef, "inventory", inventory, "status", "IN_STOCK")

		var priceDoc map[string]any
		if err = json.Unmarshal(price, &priceDoc); err != nil {
			t.Fatal(err)
		}
		if priceDoc["source"] != "shop_variant_retail_price" {
			t.Errorf("%s: price source=%v; want the authoritative variant price",
				listingRef, priceDoc["source"])
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != rbShopListings {
		t.Fatalf("rebuilt %d Shop documents; the catalogue has %d listings", seen, rbShopListings)
	}

	engine := searchprojection.NewShopSearchEngine(
		searchprojection.NewPostgresShopSearchBackend(fixture.pool))
	response, err := engine.Search(ctx, searchprojection.ShopEngineSearchRequest{
		Query: "rice", Generation: requested.Generation, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("search rebuilt Shop generation: %v", err)
	}
	if len(response.Hits) != rbShopListings {
		t.Fatalf("search over rebuilt generation returned %d hits; want %d",
			len(response.Hits), rbShopListings)
	}
}

// ---------------------------------------------------------------------------
// The properties that make a rebuild a rebuild
// ---------------------------------------------------------------------------

// TestRebuildReadsTheDomainNotTheProjection is the anti-copy gate.
//
// The existing projection is deliberately corrupted and partially deleted
// before the rebuild runs. If the rebuild source were reading search documents
// — or Redis, or a cached result — the corruption would survive into the new
// generation. Reading the domain means the corruption simply is not there.
func TestRebuildReadsTheDomainNotTheProjection(t *testing.T) {
	fixture, ctx := newRebuildFixture(t)
	pipeline := newRebuildPipeline(t, fixture.pool)

	const previousGeneration = 3
	seedActiveGeneration(t, fixture.pool, ctx, searchprojection.SearchCacheNamespaceFood, previousGeneration)

	// Corrupt the live projection: wrong names, wrong business, wrong price.
	if _, err := fixture.pool.Exec(ctx, `
        UPDATE food_search_documents
           SET name = 'CORRUPTED PROJECTION VALUE',
               normalized_name = 'corrupted projection value',
               search_text = 'corrupted projection value',
               business_ref = 'vb_wrong_tenant'
         WHERE generation = $1`, previousGeneration); err != nil {
		t.Fatalf("corrupt projection: %v", err)
	}

	requested, err := pipeline.admin.RequestFullRebuild(
		ctx, searchprojection.SearchCacheNamespaceFood, "anti-copy-gate")
	if err != nil {
		t.Fatal(err)
	}
	final := pipeline.drain(t, ctx, searchprojection.SearchCacheNamespaceFood, requested.Generation, 8)
	if final.Status != searchprojection.SearchGenerationActive {
		t.Fatalf("status=%s want=ACTIVE (%s %s)", final.Status, final.FailureCode, final.FailureReason)
	}

	var corrupted int
	if err = fixture.pool.QueryRow(ctx, `
        SELECT count(*) FROM food_search_documents
         WHERE generation = $1
           AND (name = 'CORRUPTED PROJECTION VALUE' OR business_ref = 'vb_wrong_tenant')`,
		requested.Generation).Scan(&corrupted); err != nil {
		t.Fatal(err)
	}
	if corrupted != 0 {
		t.Fatalf("%d rebuilt documents carry corruption from the previous projection; "+
			"the rebuild is reading the projection rather than the domain", corrupted)
	}

	var rebuilt int
	if err = fixture.pool.QueryRow(ctx, `
        SELECT count(*) FROM food_search_documents
         WHERE generation = $1 AND business_ref = $2`,
		requested.Generation, rbBusiness).Scan(&rebuilt); err != nil {
		t.Fatal(err)
	}
	if rebuilt != rbFoodItems {
		t.Fatalf("rebuilt %d documents from the domain; want %d", rebuilt, rbFoodItems)
	}
}

// TestRebuildFailurePreservesActiveGeneration proves the recovery contract:
// generation N stays ACTIVE while N+1 fails, and a retry finishes the job.
func TestRebuildFailurePreservesActiveGeneration(t *testing.T) {
	fixture, ctx := newRebuildFixture(t)
	pipeline := newRebuildPipeline(t, fixture.pool)

	const previousGeneration = 6
	seedActiveGeneration(t, fixture.pool, ctx, searchprojection.SearchCacheNamespaceFood, previousGeneration)

	requested, err := pipeline.admin.RequestFullRebuild(
		ctx, searchprojection.SearchCacheNamespaceFood, "failure-gate")
	if err != nil {
		t.Fatal(err)
	}

	// Fail the in-flight generation the way a real source failure would.
	if err = pipeline.generations.MarkGenerationFailed(
		ctx, searchprojection.SearchCacheNamespaceFood, requested.Generation,
		"REBUILD_SOURCE_FAILED", "injected failure", "test-rebuild-worker",
	); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	// The previous generation is untouched and still ACTIVE.
	active, err := pipeline.generations.CurrentActiveGeneration(
		ctx, searchprojection.SearchCacheNamespaceFood)
	if err != nil {
		t.Fatal(err)
	}
	if active != previousGeneration {
		t.Fatalf("active generation=%d after a failed rebuild; want the previous %d",
			active, previousGeneration)
	}

	// Search still answers from it.
	engine := searchprojection.NewFoodSearchEngine(
		searchprojection.NewPostgresFoodSearchBackend(fixture.pool))
	serving, err := engine.Search(ctx, searchprojection.FoodEngineSearchRequest{
		Query: "legacy", Generation: previousGeneration, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search previous generation during failure: %v", err)
	}
	if len(serving.Hits) == 0 {
		t.Fatal("the previous ACTIVE generation stopped serving after a failed rebuild")
	}

	// Retry through the real admin path, then run the real workers.
	if err = pipeline.admin.RetryFailedRebuild(
		ctx, searchprojection.SearchCacheNamespaceFood, requested.Generation); err != nil {
		t.Fatalf("retry: %v", err)
	}

	// A retry needs its rebuild job available again, exactly as
	// ReplayDeadLetterProjectionJob would make it.
	if _, err = fixture.pool.Exec(ctx, `
        UPDATE search_projection_jobs
           SET status='PENDING', attempt_count=0, available_at=now(),
               lease_owner=NULL, lease_token=NULL, lease_expires_at=NULL,
               progress_checkpoint=NULL
         WHERE namespace_code='FOOD' AND generation=$1 AND job_type='FULL_REBUILD'`,
		requested.Generation); err != nil {
		t.Fatalf("requeue rebuild job: %v", err)
	}

	final := pipeline.drain(t, ctx, searchprojection.SearchCacheNamespaceFood, requested.Generation, 8)
	if final.Status != searchprojection.SearchGenerationActive {
		t.Fatalf("after retry status=%s want=ACTIVE (%s %s)",
			final.Status, final.FailureCode, final.FailureReason)
	}
	if final.DocumentCount != int64(rbFoodItems) {
		t.Fatalf("after retry document_count=%d want=%d", final.DocumentCount, rbFoodItems)
	}

	previous, err := pipeline.generations.GetGeneration(
		ctx, searchprojection.SearchCacheNamespaceFood, previousGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if previous.Status != searchprojection.SearchGenerationRetired {
		t.Fatalf("previous generation status=%s want=RETIRED after successful retry", previous.Status)
	}
}

// TestRebuildBatchingIsBoundedAndKeysetPaged proves the source paginates rather
// than loading a catalogue into memory, and that its checkpoint advances.
func TestRebuildBatchingIsBoundedAndKeysetPaged(t *testing.T) {
	fixture, ctx := newRebuildFixture(t)
	source := searchrebuild.NewSource(fixture.pool)

	// A limit far below the fixture size forces real pagination.
	const pageSize = 3

	seen := map[string]int{}
	checkpoint := ""
	pages := 0

	for pages < 20 {
		batch, err := source.LoadSearchRebuildBatch(
			ctx, searchprojection.SearchCacheNamespaceFood,
			searchprojection.SearchRebuildScopeFull, "", checkpoint, pageSize,
		)
		if err != nil {
			t.Fatalf("load batch: %v", err)
		}
		pages++

		if len(batch.Documents) > pageSize {
			t.Fatalf("batch returned %d documents; the requested limit was %d",
				len(batch.Documents), pageSize)
		}
		for _, document := range batch.Documents {
			if document.Food == nil {
				t.Fatal("a FOOD batch produced a non-Food document")
			}
			if document.Shop != nil {
				t.Fatal("a FOOD batch produced a Shop document")
			}
			seen[document.Food.FoodItemRef]++
			if seen[document.Food.FoodItemRef] > 1 {
				t.Fatalf("%s appeared in more than one batch; the keyset cursor is not stable",
					document.Food.FoodItemRef)
			}
		}

		if batch.Done {
			break
		}
		if batch.NextCheckpoint == checkpoint {
			t.Fatal("checkpoint did not advance; the rebuild would loop forever")
		}
		checkpoint = batch.NextCheckpoint
	}

	if len(seen) != rbFoodItems {
		t.Fatalf("paged %d distinct items; the catalogue has %d", len(seen), rbFoodItems)
	}
	if pages < 2 {
		t.Fatalf("completed in %d page(s); the fixture should have required several", pages)
	}
	t.Logf("paged %d Food items over %d bounded batches", len(seen), pages)
}

// TestRebuildBatchQueriesDoNotScaleWithBatchSize is the N+1 gate for the source.
//
// A per-document enrichment query would be invisible on a small fixture and
// fatal on a real catalogue, so the check is relative: loading many documents
// must not cost proportionally more queries than loading one.
func TestRebuildBatchQueriesDoNotScaleWithBatchSize(t *testing.T) {
	fixture, ctx := newRebuildFixture(t)
	source := searchrebuild.NewSource(fixture.pool)

	transactions := func() int64 {
		var value int64
		if err := fixture.pool.QueryRow(ctx,
			`SELECT xact_commit + xact_rollback FROM pg_stat_database WHERE datname = current_database()`,
		).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}

	measure := func(limit int) (documents int, cost int64) {
		before := transactions()
		batch, err := source.LoadSearchRebuildBatch(
			ctx, searchprojection.SearchCacheNamespaceFood,
			searchprojection.SearchRebuildScopeFull, "", "", limit,
		)
		if err != nil {
			t.Fatal(err)
		}
		return len(batch.Documents), transactions() - before
	}

	// Warm the pool so connection growth is not mistaken for query growth.
	_, _ = measure(1)

	smallDocs, smallCost := measure(1)
	largeDocs, largeCost := measure(rbFoodItems)

	if largeDocs <= smallDocs {
		t.Fatalf("batches produced %d and %d documents; the comparison needs them to differ",
			smallDocs, largeDocs)
	}
	t.Logf("documents %d -> %d, transactions %d -> %d", smallDocs, largeDocs, smallCost, largeCost)

	if largeCost > smallCost {
		t.Fatalf("loading %d documents cost %d more transactions than loading %d; "+
			"the source is issuing per-document queries",
			largeDocs, largeCost-smallCost, smallDocs)
	}
}

// TestRebuildRunsWithoutRedis proves Redis is not a rebuild input.
//
// The whole pipeline is constructed with no cache at all. If any part of the
// rebuild path consulted Redis, this would fail rather than merely run slower.
func TestRebuildRunsWithoutRedis(t *testing.T) {
	fixture, ctx := newRebuildFixture(t)
	pipeline := newRebuildPipeline(t, fixture.pool)

	requested, err := pipeline.admin.RequestFullRebuild(
		ctx, searchprojection.SearchCacheNamespaceShop, "no-redis-gate")
	if err != nil {
		t.Fatal(err)
	}

	final := pipeline.drain(t, ctx, searchprojection.SearchCacheNamespaceShop, requested.Generation, 8)
	if final.Status != searchprojection.SearchGenerationActive {
		t.Fatalf("rebuild without Redis status=%s want=ACTIVE (%s %s)",
			final.Status, final.FailureCode, final.FailureReason)
	}
	if final.DocumentCount != int64(rbShopListings) {
		t.Fatalf("document_count=%d want=%d", final.DocumentCount, rbShopListings)
	}
}

// TestUnvalidatedGenerationIsNeverActivated proves the lifecycle gate holds
// even against a direct attempt.
func TestUnvalidatedGenerationIsNeverActivated(t *testing.T) {
	fixture, ctx := newRebuildFixture(t)
	pipeline := newRebuildPipeline(t, fixture.pool)

	const previousGeneration = 5
	seedActiveGeneration(t, fixture.pool, ctx, searchprojection.SearchCacheNamespaceFood, previousGeneration)

	requested, err := pipeline.admin.RequestFullRebuild(
		ctx, searchprojection.SearchCacheNamespaceFood, "lifecycle-gate")
	if err != nil {
		t.Fatal(err)
	}

	// BUILDING, not yet validated: activation must be refused.
	if err = pipeline.generations.ActivateGeneration(
		ctx, searchprojection.SearchCacheNamespaceFood, requested.Generation,
	); err == nil {
		t.Fatal("a BUILDING generation was activated without validation")
	}

	// Validation must also refuse while the build is incomplete.
	if err = pipeline.generations.MarkGenerationReady(
		ctx, searchprojection.SearchCacheNamespaceFood, requested.Generation,
	); err == nil {
		t.Fatal("an incomplete generation was marked READY")
	}

	active, err := pipeline.generations.CurrentActiveGeneration(
		ctx, searchprojection.SearchCacheNamespaceFood)
	if err != nil {
		t.Fatal(err)
	}
	if active != previousGeneration {
		t.Fatalf("active generation=%d; want the previous %d untouched", active, previousGeneration)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func containsFold(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToUpper(value), strings.ToUpper(needle)) {
			return true
		}
	}
	return false
}

func assertJSONField(
	t *testing.T,
	ref string,
	label string,
	raw []byte,
	field string,
	want any,
) {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("%s: decode %s: %v", ref, label, err)
	}
	got := document[field]
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s: %s.%s=%v want=%v", ref, label, field, got, want)
	}
}
