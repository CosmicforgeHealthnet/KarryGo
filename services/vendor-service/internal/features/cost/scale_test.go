package cost

// Section 16D scale and query-plan certification.
//
// The corpus is synthetic and built set-based with generate_series rather than
// row by row: 100,000 component rows inserted one at a time would dominate the
// runtime and prove nothing about the queries under test. Tenant and branch
// relationships are preserved exactly - every row belongs to a real business
// and a real branch - because the whole point is to measure the predicates
// that keep one vendor's cost out of another's reads.
//
// Two different questions are asked, and they are different questions.
//
// The behavioural one: at this size, do the bounded reads stay bounded, do the
// tenant and branch predicates still hold, and is anything accidentally
// quadratic?
//
// The structural one: are the plans PostgreSQL chooses index-capable? That is
// deliberately NOT asked as "no plan may contain a Seq Scan" - a small lookup
// table is correctly scanned sequentially, and asserting otherwise pins a
// planner decision rather than an architectural property. What is asserted is
// narrower and actually meaningful: the indexes the large paths depend on
// exist, and the planner can satisfy the real queries through them.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Corpus sizes. These are test load, not a claim about platform usage.
const (
	scaleIngredients   = 5000
	scalePackaging     = 1000
	scaleRecipes       = 2000
	scaleVersions      = 4000
	scaleComponents    = 20000
	scalePackagingReqs = 4000
	scalePurchaseRows  = 50000
	scaleBatches       = 5000
	// 15,000 recipe snapshots plus one expected-cost record per batch.
	scaleRecipeSnapshots = 15000
	scaleSnapshots       = scaleRecipeSnapshots + scaleBatches
	// Four component rows per snapshot, giving 80,000 evidence rows.
	scaleComponentsPerSnapshot = 4
	scaleSnapshotLines         = scaleSnapshots * scaleComponentsPerSnapshot

	scaleBusiness      = "vcost_scale_biz"
	scaleOtherBusiness = "vcost_scale_biz_other"
	scaleBranch        = "vcost_scale_branch"
	scaleBranchTwo     = "vcost_scale_branch_2"
)

var (
	scaleOnce  sync.Once
	scaleErr   error
	scaleBuild time.Duration
)

// ensureScaleCorpus builds the corpus at most once for the whole package.
//
// Every scale test reads the same read-only corpus, so rebuilding it per test
// would multiply a minute of setup by the number of tests for no extra
// coverage.
func ensureScaleCorpus(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	pool, _ := testPool(t)

	scaleOnce.Do(func() {
		buildCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		started := time.Now()
		scaleErr = buildScaleCorpus(buildCtx, pool)
		scaleBuild = time.Since(started)
	})
	if scaleErr != nil {
		t.Fatalf("build scale corpus: %v", scaleErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return pool, ctx
}

func buildScaleCorpus(ctx context.Context, pool *pgxpool.Pool) error {
	// The corpus is built once and reused. Recipe versions and cost snapshots
	// are immutable once written - a version cannot return to DRAFT and
	// evidence cannot be deleted - so re-seeding an existing corpus would not
	// merely be wasteful, it would be refused by the database. Detecting
	// completeness and returning is the honest way to be idempotent here.
	var built int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_cost_snapshot_components WHERE business_id=$1`,
		scaleBusiness).Scan(&built); err != nil {
		return err
	}
	if built >= scaleSnapshotLines {
		return nil
	}

	// Two businesses and two branches, so tenant and branch predicates have
	// something real to exclude rather than being trivially true.
	for _, business := range []string{scaleBusiness, scaleOtherBusiness} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO vendor_users(id,email) VALUES($1,$1||'@example.test')
			ON CONFLICT (id) DO NOTHING`, business+"_user"); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO vendor_staff(id,vendor_id,user_id,status)
			VALUES($1,$2,$3,'active') ON CONFLICT (id) DO NOTHING`,
			business+"_owner", business, business+"_user"); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone)
			VALUES($1,$1||' Ltd','ACTIVE',$2,'NGN','Africa/Lagos')
			ON CONFLICT (id) DO NOTHING`, business, business+"_owner"); err != nil {
			return err
		}
	}
	for _, branch := range []struct{ id, business string }{
		{scaleBranch, scaleBusiness},
		{scaleBranchTwo, scaleBusiness},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO vendor_branches(
				id,business_id,display_name,address_line1,city,region,
				country_code,latitude,longitude,timezone,status)
			VALUES($1,$2,$1,'1 Scale Road','Lagos','Lagos','NG',6.5,3.4,
			       'Africa/Lagos','ACTIVE')
			ON CONFLICT (id) DO NOTHING`, branch.id, branch.business); err != nil {
			return err
		}
	}

	// Idempotent rebuild. Policies are mutable configuration and are cleared;
	// snapshots and their component evidence are NOT, because they are
	// immutable by design and the database refuses to delete them. The corpus
	// is therefore built once and reused, which is also what makes repeated
	// query-plan runs cheap.
	if _, err := pool.Exec(ctx,
		`DELETE FROM vendor_cost_policies WHERE business_id LIKE 'vcost_scale%'`); err != nil {
		return err
	}
	// Snapshots are immutable evidence; the trigger refuses DELETE for the
	// historical types, so a rebuild only proceeds when the corpus is absent.
	var existing int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_cost_snapshots WHERE business_id=$1`,
		scaleBusiness).Scan(&existing); err != nil {
		return err
	}

	steps := []struct {
		name string
		sql  string
		args []any
	}{
		{"ingredients", `
			INSERT INTO vendor_ingredients(
				id,business_id,code,name,description,measurement_family,
				default_unit_code,inventory_control_mode,perishable,requires_expiry,
				status,version)
			SELECT 'sc_ing_'||i,$1,'SC_ING_'||i,'Ingredient '||i,'scale','MASS','kg',
			       'INVENTORY_CONTROLLED',false,false,'ACTIVE',1
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`, []any{scaleBusiness, scaleIngredients}},

		{"packaging", `
			INSERT INTO vendor_packaging_materials(
				id,business_id,code,name,description,measurement_family,
				default_unit_code,inventory_control_mode,status,version)
			SELECT 'sc_pkg_'||i,$1,'SC_PKG_'||i,'Packaging '||i,'scale','COUNT','box',
			       'INVENTORY_CONTROLLED','ACTIVE',1
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`, []any{scaleBusiness, scalePackaging}},

		{"inventory items", `
			INSERT INTO inventory_items(
				id,business_id,branch_id,item_type,source_ref,stock_measure_ref,
				stock_measure_owner_key,stock_unit_kind,stock_basis_mode,
				tracking_mode,lot_policy,lot_tracking_enabled,status,version)
			SELECT 'sc_item_'||i,$1,$3,'INGREDIENT','sc_ing_'||i,'scale','scale',
			       'MASS','SAME_UNIT','SIMPLE','FIFO',false,'ACTIVE',1
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`, []any{scaleBusiness, scaleIngredients, scaleBranch}},

		{"branch configs", `
			INSERT INTO vendor_ingredient_branch_configs(
				id,business_id,branch_id,ingredient_id,inventory_item_id,version)
			SELECT 'sc_cfg_'||i,$1,$3,'sc_ing_'||i,'sc_item_'||i,1
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`, []any{scaleBusiness, scaleIngredients, scaleBranch}},

		// Cost policies: a business default for every ingredient, plus a
		// branch override on a slice of them, so the precedence query has both
		// shapes to choose between at size.
		{"cost policies", `
			INSERT INTO vendor_cost_policies(
				id,business_id,branch_id,subject_type,subject_ref,strategy,currency,
				weighted_average_receipt_limit,created_by_staff_id,updated_by_staff_id)
			SELECT 'sc_pol_'||i,$1,NULL,'INGREDIENT','sc_ing_'||i,
			       'LATEST_ACCEPTED_PURCHASE_COST','NGN',10,$3,$3
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`,
			[]any{scaleBusiness, scaleIngredients, scaleBusiness + "_owner"}},

		{"branch policy overrides", `
			INSERT INTO vendor_cost_policies(
				id,business_id,branch_id,subject_type,subject_ref,strategy,currency,
				weighted_average_receipt_limit,manual_unit_cost_minor,manual_unit_code,
				created_by_staff_id,updated_by_staff_id)
			SELECT 'sc_polb_'||i,$1,$4,'INGREDIENT','sc_ing_'||i,
			       'MANUAL_CONFIGURED_COST','NGN',10,500+i,'kg',$3,$3
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`,
			[]any{scaleBusiness, scaleIngredients / 5, scaleBusiness + "_owner", scaleBranch}},

		{"recipes", `
			INSERT INTO vendor_recipes(
				id,business_id,branch_id,target_type,target_ref,name,status,version)
			SELECT 'sc_rec_'||i,$1,$3,'FOOD_ITEM','sc_target_'||i,'Recipe '||i,'ACTIVE',1
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`, []any{scaleBusiness, scaleRecipes, scaleBranch}},

		// Two versions per recipe: one ACTIVE, one RETIRED, so version history
		// reads have real depth.
		{"recipe versions", `
			INSERT INTO vendor_recipe_versions(
				id,business_id,recipe_id,version_number,status,
				expected_output_numerator,expected_output_denominator,
				output_unit_code,serving_count,expected_waste_bps,version)
			SELECT 'sc_ver_'||i,$1,'sc_rec_'||(((i-1)%$3)+1),
			       ((i-1)/$3)+1,
			       'DRAFT',
			       10,1,'kg',8,1000,1
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`, []any{scaleBusiness, scaleVersions, scaleRecipes}},

		{"recipe components", `
			INSERT INTO vendor_recipe_components(
				id,business_id,recipe_version_id,ingredient_id,
				quantity_numerator,quantity_denominator,unit_code,
				normalization_status,normalized_quantity_numerator,
				normalized_quantity_denominator,normalized_unit_code,expected_waste_bps)
			SELECT 'sc_comp_'||i,$1,'sc_ver_'||(((i-1)%$3)+1),
			       'sc_ing_'||(((i-1)%$4)+1),
			       ((i%9)+1),1,'kg','EXACT',((i%9)+1),1,'kg',1000
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`,
			[]any{scaleBusiness, scaleComponents, scaleVersions, scaleIngredients}},

		{"packaging requirements", `
			INSERT INTO vendor_recipe_packaging_requirements(
				id,business_id,recipe_version_id,packaging_material_id,
				quantity_numerator,quantity_denominator,unit_code,
				normalization_status,normalized_quantity_numerator,
				normalized_quantity_denominator,normalized_unit_code,sequence,version)
			SELECT 'sc_preq_'||i,$1,'sc_ver_'||(((i-1)%$3)+1),
			       'sc_pkg_'||(((i-1)%$4)+1),1,1,'box','EXACT',1,1,'box',1,1
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`,
			[]any{scaleBusiness, scalePackagingReqs, scaleVersions, scalePackaging}},

		// Purchase-cost evidence. Written directly here rather than through
		// receiving: the receiving path is certified in the purchasing package
		// (section16d_cost_evidence_test.go), and 50,000 real receipts would
		// take far longer than the queries this corpus exists to measure.
		// One in every twenty observations is reversed, so the eligibility
		// predicate has real work to do.
		// The purchasing spine the cost history references. One supplier,
		// one product, one PO and one receipt per inventory item, so every
		// cost row is anchored to a real purchasing record exactly as
		// receiving anchors it.
		{"scale suppliers", `
			INSERT INTO vendor_suppliers(id,business_id,code,name,legal_name)
			VALUES('sc_sup',$1,'SCSUP','Scale Supplier','Scale Supplier Ltd')
			ON CONFLICT (id) DO NOTHING`, []any{scaleBusiness}},

		{"scale supplier products", `
			INSERT INTO vendor_supplier_products(
				id,business_id,supplier_id,branch_id,inventory_item_id,supplier_sku,
				purchase_unit_code,purchase_unit_name,conversion_mode,
				inventory_units_per_purchase_unit,currency_code,
				inventory_measure_ref,inventory_measure_owner_key,inventory_unit_kind)
			SELECT 'sc_sp_'||i,$1,'sc_sup',$3,'sc_item_'||i,'SKU'||i,
			       'KG','Kilogram','SAME_AS_INVENTORY',1,'NGN','scale','scale','MASS'
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`, []any{scaleBusiness, scaleIngredients, scaleBranch}},

		{"scale purchase orders", `
			INSERT INTO vendor_purchase_orders(
				id,business_id,branch_id,supplier_id,purchase_order_number,
				supplier_name_snapshot,currency_code,created_by_staff_id)
			VALUES('sc_po',$1,$2,'sc_sup','PO-SCALE','Scale Supplier','NGN',$3)
			ON CONFLICT (id) DO NOTHING`,
			[]any{scaleBusiness, scaleBranch, scaleBusiness + "_owner"}},

		{"scale purchase order lines", `
			INSERT INTO vendor_purchase_order_lines(
				id,business_id,purchase_order_id,supplier_product_id,
				supplier_product_version,inventory_item_id,purchase_unit_code_snapshot,
				purchase_unit_name_snapshot,conversion_mode_snapshot,
				inventory_units_per_purchase_unit_snapshot,inventory_measure_ref_snapshot,
				inventory_measure_owner_key_snapshot,inventory_unit_kind_snapshot,
				purchase_quantity,converted_inventory_quantity,unit_cost_minor,
				line_total_minor,currency_code_snapshot)
			SELECT 'sc_pol_line_'||i,$1,'sc_po','sc_sp_'||(((i-1)%$3)+1),1,
			       'sc_item_'||(((i-1)%$3)+1),'KG','Kilogram','SAME_AS_INVENTORY',1,
			       'scale','scale','MASS',100,100,(100+(i%400)),(100+(i%400))*100,'NGN'
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`,
			[]any{scaleBusiness, scalePurchaseRows, scaleIngredients}},

		{"scale goods receipts", `
			INSERT INTO vendor_goods_receipts(
				id,business_id,purchase_order_id,supplier_id,branch_id,currency,
				received_at,received_by_staff_id)
			SELECT 'sc_gr_'||i,$1,'sc_po','sc_sup',$3,'NGN',now(),$4
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`,
			[]any{scaleBusiness, scalePurchaseRows, scaleBranch, scaleBusiness + "_owner"}},

		{"scale goods receipt lines", `
			INSERT INTO vendor_goods_receipt_lines(
				id,business_id,goods_receipt_id,purchase_order_id,
				purchase_order_line_id,supplier_product_id,inventory_item_id,
				ordered_quantity_snapshot,received_before_snapshot,
				expected_unit_cost_minor,purchase_unit_code,
				inventory_units_per_purchase_unit,delivered_quantity,
				accepted_quantity,currency)
			SELECT 'sc_grl_'||i,$1,'sc_gr_'||i,'sc_po','sc_pol_line_'||i,
			       'sc_sp_'||(((i-1)%$3)+1),'sc_item_'||(((i-1)%$3)+1),
			       100,0,(100+(i%400)),'KG',1,100,100,'NGN'
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`,
			[]any{scaleBusiness, scalePurchaseRows, scaleIngredients}},

		{"purchase cost history", `
			INSERT INTO vendor_purchase_cost_history(
				id,business_id,branch_id,supplier_id,supplier_product_id,
				inventory_item_id,purchase_order_id,purchase_order_line_id,
				goods_receipt_id,goods_receipt_line_id,evidence_type,
				purchase_unit_code,inventory_units_per_purchase_unit,
				purchase_quantity,inventory_quantity,purchase_unit_cost_minor,
				total_cost_minor,inventory_unit_cost_minor,currency,
				observed_at,recorded_by_staff_id)
			SELECT 'sc_pch_'||i,$1,$4,'sc_sup','sc_sp_'||(((i-1)%$3)+1),'sc_item_'||(((i-1)%$3)+1),
			       'sc_po','sc_pol_line_'||i,'sc_gr_'||i,'sc_grl_'||i,
			       CASE WHEN i%20=0 THEN 'REVERSAL' ELSE 'OBSERVED' END,
			       'KG',1,100,100,(100+(i%400)),(100+(i%400))*100,(100+(i%400)),'NGN',
			       now()-(i||' minutes')::interval,$5
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`,
			[]any{scaleBusiness, scalePurchaseRows, scaleIngredients, scaleBranch,
				scaleBusiness + "_owner"}},

		{"production batches", `
			INSERT INTO vendor_production_batches(
				id,business_id,branch_id,recipe_id,recipe_version_id,
				recipe_version_number,target_type,target_ref,target_variant_ref,
				planned_output_numerator,planned_output_denominator,output_unit_code,
				expected_output_numerator,expected_output_denominator,
				recipe_expected_waste_bps,multiplier_numerator,multiplier_denominator,
				operating_model,output_inventory_numerator,output_inventory_denominator,
				status,actual_consumption_recorded,actual_output_numerator,
				actual_output_denominator,cancel_reason,version)
			SELECT 'sc_batch_'||i,$1,$4,'sc_rec_'||(((i-1)%$3)+1),
			       'sc_ver_'||(((i-1)%$3)+1),1,'FOOD_ITEM','sc_target_'||(((i-1)%$3)+1),'',
			       10,1,'kg',10,1,1000,1,1,'BATCH_STOCK',10,1,
			       'COMPLETED',true,10,1,'',1
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`,
			[]any{scaleBusiness, scaleBatches, scaleRecipes, scaleBranch}},
	}

	for _, step := range steps {
		if _, err := pool.Exec(ctx, step.sql, step.args...); err != nil {
			return fmt.Errorf("seed %s: %w", step.name, err)
		}
	}

	// Section 16B permits composition only while a version is DRAFT, so the
	// corpus is built the way the product builds one: draft, compose, then
	// activate. One version per recipe becomes ACTIVE and the rest RETIRE, so
	// version-history reads have real depth.
	if _, err := pool.Exec(ctx, `
		UPDATE vendor_recipe_versions
		   SET status='ACTIVE', activated_at=now()
		 WHERE business_id=$1 AND status='DRAFT' AND version_number=1`,
		scaleBusiness); err != nil {
		return fmt.Errorf("activate scale versions: %w", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE vendor_recipe_versions
		   SET status='RETIRED', retired_at=now()
		 WHERE business_id=$1 AND status='DRAFT'`, scaleBusiness); err != nil {
		return fmt.Errorf("retire scale versions: %w", err)
	}

	// Cost snapshots and their component evidence, only if absent: they are
	// immutable, so a rebuild cannot rewrite them.
	if existing == 0 {
		// Recipe snapshots have no per-subject uniqueness, so a recipe version
		// can accumulate a real history.
		if _, err := pool.Exec(ctx, `
			INSERT INTO vendor_cost_snapshots(
				id,business_id,branch_id,calculation_type,calculation_version,
				recipe_id,recipe_version_id,strategy,currency,completeness,
				total_cost_minor,ingredient_cost_minor,packaging_cost_minor,
				waste_cost_minor,output_quantity_numerator,output_quantity_denominator,
				serving_count,expected_waste_bps,calculated_at,calculated_by_staff_id)
			SELECT 'sc_snap_r_'||i,$1,$5,'RECIPE_SNAPSHOT',1,
			       'sc_rec_'||(((i-1)%$3)+1),'sc_ver_'||(((i-1)%$3)+1),
			       'LATEST_ACCEPTED_PURCHASE_COST','NGN','COMPLETE',
			       10000+i,9000+i,1000,500,10,1,8,1000,
			       now()-(i||' minutes')::interval,$4
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`,
			scaleBusiness, scaleRecipeSnapshots, scaleRecipes,
			scaleBusiness+"_owner", scaleBranch); err != nil {
			return fmt.Errorf("seed recipe snapshots: %w", err)
		}

		// Production snapshots are unique per batch and calculation type, so
		// exactly one expected record per batch - which is the invariant the
		// index exists to enforce, honoured here rather than fought.
		if _, err := pool.Exec(ctx, `
			INSERT INTO vendor_cost_snapshots(
				id,business_id,branch_id,calculation_type,calculation_version,
				batch_id,strategy,currency,completeness,
				total_cost_minor,ingredient_cost_minor,packaging_cost_minor,
				waste_cost_minor,output_quantity_numerator,output_quantity_denominator,
				expected_waste_bps,calculated_at,calculated_by_staff_id)
			SELECT 'sc_snap_p_'||i,$1,$4,'PRODUCTION_EXPECTED',1,
			       'sc_batch_'||i,'LATEST_ACCEPTED_PURCHASE_COST','NGN','COMPLETE',
			       12000+i,11000+i,1000,500,10,1,1000,
			       now()-(i||' minutes')::interval,$3
			  FROM generate_series(1,$2) AS i
			ON CONFLICT DO NOTHING`,
			scaleBusiness, scaleBatches, scaleBusiness+"_owner", scaleBranch); err != nil {
			return fmt.Errorf("seed production snapshots: %w", err)
		}

		// Component evidence is derived from the snapshots that actually
		// exist, rather than from arithmetic that assumes they all do.
		if _, err := pool.Exec(ctx, `
			INSERT INTO vendor_cost_snapshot_components(
				id,business_id,snapshot_id,line_number,subject_type,subject_ref,
				inventory_item_id,quantity_numerator,quantity_denominator,
				quantity_unit_code,cost_source,source_evidence_ref,source_receipt_count,
				unit_cost_minor,currency,component_cost_minor,expected_waste_bps,
				waste_cost_minor)
			SELECT 'sc_sc_'||s.id||'_'||g,$1,s.id,g,'INGREDIENT',
			       'sc_ing_'||(((g-1)%$2)+1),'sc_item_'||(((g-1)%$2)+1),
			       ((g%9)+1),1,'kg','LATEST_ACCEPTED_PURCHASE_COST','sc_pch_'||g,1,
			       (100+(g%400)),'NGN',(100+(g%400))*((g%9)+1),1000,50
			  FROM vendor_cost_snapshots s
			  CROSS JOIN generate_series(1,$3) AS g
			 WHERE s.business_id=$1
			ON CONFLICT DO NOTHING`,
			scaleBusiness, scaleIngredients, scaleComponentsPerSnapshot); err != nil {
			return fmt.Errorf("seed snapshot components: %w", err)
		}
	}

	// ANALYZE, so every plan inspected below is chosen from real statistics
	// rather than from the planner's defaults for an unanalysed table.
	for _, table := range []string{
		"vendor_cost_policies", "vendor_cost_snapshots",
		"vendor_cost_snapshot_components", "vendor_purchase_cost_history",
		"vendor_recipe_components", "vendor_recipe_versions",
		"vendor_production_batches",
	} {
		if _, err := pool.Exec(ctx, "ANALYZE "+table); err != nil {
			return fmt.Errorf("analyze %s: %w", table, err)
		}
	}
	return nil
}

func scaleCount(t *testing.T, pool *pgxpool.Pool, ctx context.Context, table, predicate string) int64 {
	t.Helper()
	var count int64
	query := "SELECT count(*) FROM " + table
	if predicate != "" {
		query += " WHERE " + predicate
	}
	if err := pool.QueryRow(ctx, query).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

// TestScaleCorpusCountsAreRepresentative records the fixture sizes the
// certification report quotes, and fails if the corpus silently shrank.
func TestScaleCorpusCountsAreRepresentative(t *testing.T) {
	pool, ctx := ensureScaleCorpus(t)

	for _, expectation := range []struct {
		table     string
		predicate string
		atLeast   int64
	}{
		{"vendor_ingredients", "business_id='" + scaleBusiness + "'", scaleIngredients},
		{"vendor_packaging_materials", "business_id='" + scaleBusiness + "'", scalePackaging},
		{"vendor_recipes", "business_id='" + scaleBusiness + "'", scaleRecipes},
		{"vendor_recipe_versions", "business_id='" + scaleBusiness + "'", scaleVersions},
		{"vendor_recipe_components", "business_id='" + scaleBusiness + "'", scaleComponents},
		{"vendor_recipe_packaging_requirements", "business_id='" + scaleBusiness + "'", scalePackagingReqs},
		{"vendor_purchase_cost_history", "business_id='" + scaleBusiness + "'", scalePurchaseRows},
		{"vendor_production_batches", "business_id='" + scaleBusiness + "'", scaleBatches},
		{"vendor_cost_snapshots", "business_id='" + scaleBusiness + "'", scaleSnapshots},
		{"vendor_cost_snapshot_components", "business_id='" + scaleBusiness + "'", scaleSnapshotLines},
		{"vendor_cost_policies", "business_id='" + scaleBusiness + "'", scaleIngredients},
	} {
		got := scaleCount(t, pool, ctx, expectation.table, expectation.predicate)
		if got < expectation.atLeast {
			t.Errorf("%s has %d rows; the corpus requires at least %d",
				expectation.table, got, expectation.atLeast)
		}
		t.Logf("%-44s %d", expectation.table, got)
	}
	t.Logf("corpus build took %s", scaleBuild.Round(time.Millisecond))
}

// TestScaleReadsStayBoundedAndTenantScoped is the behavioural half.
//
// Every read is timed for the report and checked for two things: that it
// returns a bounded page, and that it returns nothing at all for a tenant that
// owns none of the corpus.
func TestScaleReadsStayBoundedAndTenantScoped(t *testing.T) {
	pool, ctx := ensureScaleCorpus(t)
	engine := NewEngine(pool, NewResolver(pool, NewPostgresPurchasingCostReader(pool)))
	resolver := NewResolver(pool, NewPostgresPurchasingCostReader(pool))

	t.Run("cost policy lookup prefers the branch override", func(t *testing.T) {
		started := time.Now()
		policy, found, err := resolver.ResolvePolicy(
			ctx, scaleBusiness, scaleBranch, SubjectIngredient, "sc_ing_7")
		if err != nil || !found {
			t.Fatalf("found=%t err=%v", found, err)
		}
		if !strings.HasPrefix(policy.ID, "sc_polb_") {
			t.Fatalf("resolved %s; the branch override must win inside its branch", policy.ID)
		}
		t.Logf("branch-override policy lookup: %s", time.Since(started).Round(time.Microsecond))

		// A branch with no override falls back to the business default.
		fallback, found, err := resolver.ResolvePolicy(
			ctx, scaleBusiness, scaleBranchTwo, SubjectIngredient, "sc_ing_7")
		if err != nil || !found {
			t.Fatalf("fallback found=%t err=%v", found, err)
		}
		if !strings.HasPrefix(fallback.ID, "sc_pol_") {
			t.Fatalf("resolved %s; a branch without an override must use the default", fallback.ID)
		}

		// Another tenant sees nothing.
		if _, found, err = resolver.ResolvePolicy(
			ctx, scaleOtherBusiness, scaleBranch, SubjectIngredient, "sc_ing_7"); err != nil {
			t.Fatal(err)
		} else if found {
			t.Fatal("another tenant resolved this tenant's cost policy at scale")
		}
	})

	t.Run("latest accepted purchase cost is bounded", func(t *testing.T) {
		reader := NewPostgresPurchasingCostReader(pool)
		started := time.Now()
		_, found, err := reader.LatestAccepted(ctx, scaleBusiness, scaleBranch, "sc_item_11")
		if err != nil || !found {
			t.Fatalf("found=%t err=%v", found, err)
		}
		t.Logf("latest accepted purchase cost over %d rows: %s",
			scalePurchaseRows, time.Since(started).Round(time.Microsecond))

		if _, found, err = reader.LatestAccepted(
			ctx, scaleOtherBusiness, scaleBranch, "sc_item_11"); err != nil {
			t.Fatal(err)
		} else if found {
			t.Fatal("another tenant read this tenant's purchase cost at scale")
		}
	})

	t.Run("weighted average window is capped", func(t *testing.T) {
		reader := NewPostgresPurchasingCostReader(pool)
		started := time.Now()
		observations, err := reader.RecentAccepted(ctx, scaleBusiness, scaleBranch, "sc_item_11", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(observations) > 10 {
			t.Fatalf("%d observations returned for a bound of 10; the window is not capped",
				len(observations))
		}
		t.Logf("bounded weighted-average window: %d rows in %s",
			len(observations), time.Since(started).Round(time.Microsecond))
	})

	t.Run("batched cost resolution avoids N+1", func(t *testing.T) {
		// Forty items resolved in ONE query. This is the read recipe costing
		// uses; doing it per component is the N+1 this guards against.
		items := make([]string, 0, 40)
		for i := 1; i <= 40; i++ {
			items = append(items, fmt.Sprintf("sc_item_%d", i))
		}
		reader := NewPostgresPurchasingCostReader(pool)

		started := time.Now()
		resolved, err := reader.LatestAcceptedBatch(ctx, scaleBusiness, scaleBranch, items)
		if err != nil {
			t.Fatal(err)
		}
		batched := time.Since(started)
		if len(resolved) == 0 {
			t.Fatal("batched resolution returned nothing")
		}

		// The same work one item at a time, for comparison.
		started = time.Now()
		for _, item := range items {
			if _, _, err = reader.LatestAccepted(ctx, scaleBusiness, scaleBranch, item); err != nil {
				t.Fatal(err)
			}
		}
		serial := time.Since(started)

		t.Logf("batched %d items in %s; one-at-a-time %s (%.1fx)",
			len(items), batched.Round(time.Microsecond),
			serial.Round(time.Microsecond),
			float64(serial)/float64(batched))
		if batched > serial {
			t.Fatalf("the batched read (%s) was slower than %d serial reads (%s)",
				batched, len(items), serial)
		}
	})

	t.Run("recipe cost history is paginated", func(t *testing.T) {
		started := time.Now()
		page, err := engine.RecipeCostHistory(ctx, scaleBusiness, "", "", 25, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) > 25 {
			t.Fatalf("history returned %d rows for a page size of 25", len(page))
		}
		t.Logf("recipe cost history page over %d snapshots: %s",
			scaleSnapshots, time.Since(started).Round(time.Microsecond))

		// An oversized request is clamped rather than honoured.
		big, err := engine.RecipeCostHistory(ctx, scaleBusiness, "", "", 10000, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(big) > 100 {
			t.Fatalf("an unbounded page size returned %d rows", len(big))
		}

		if other, err := engine.RecipeCostHistory(ctx, scaleOtherBusiness, "", "", 25, 0); err != nil {
			t.Fatal(err)
		} else if len(other) != 0 {
			t.Fatalf("another tenant read %d cost snapshots", len(other))
		}
	})

	t.Run("production cost history is paginated and branch scoped", func(t *testing.T) {
		started := time.Now()
		page, err := engine.ProductionCostHistory(ctx, scaleBusiness, scaleBranch, 25, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) > 25 {
			t.Fatalf("production history returned %d rows for a page size of 25", len(page))
		}
		t.Logf("production cost history page: %s", time.Since(started).Round(time.Microsecond))

		if other, err := engine.ProductionCostHistory(
			ctx, scaleBusiness, scaleBranchTwo, 25, 0); err != nil {
			t.Fatal(err)
		} else if len(other) != 0 {
			t.Fatalf("a branch with no production cost read %d snapshots", len(other))
		}
	})

	t.Run("highest cost subjects is bounded", func(t *testing.T) {
		started := time.Now()
		subjects, err := engine.HighestCostSubjects(ctx, scaleBusiness, scaleBranch, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(subjects) > 20 {
			t.Fatalf("%d subjects returned for a bound of 20", len(subjects))
		}
		t.Logf("highest-cost subjects over %d component rows: %s",
			scaleSnapshotLines, time.Since(started).Round(time.Millisecond))

		if other, err := engine.HighestCostSubjects(ctx, scaleOtherBusiness, "", 20); err != nil {
			t.Fatal(err)
		} else if len(other) != 0 {
			t.Fatalf("another tenant read %d cost subjects", len(other))
		}
	})
}
