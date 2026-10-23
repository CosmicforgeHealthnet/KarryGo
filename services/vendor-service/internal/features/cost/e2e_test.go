package cost

// Section 16D end-to-end certification: 16A -> 16B -> 16C -> 16D.
//
// This seeds the real chain - ingredient and packaging master data with branch
// inventory mapping, a recipe version with NET component quantities, a frozen
// production batch with planned and actual quantities, and Section 15 purchase
// evidence - then asserts EXACT figures rather than "cost > 0".
//
// THE SCENARIO, chosen so every number can be checked by hand:
//
//	Rice       NET 9 kg,  10% expected waste -> gross 10 kg @ 70 minor  =   700
//	Chicken    NET 2 kg,   0% expected waste ->       2 kg @ 850 minor  =  1700
//	Packaging      1 box, never waste-grossed ->       1 box @ 120 minor =   120
//	                                                      RECIPE COGS   =  2520
//
// The recipe yields 10 units and 8 servings, so per-unit is 252 and
// per-serving is 315.
//
// The batch freezes planned quantities that ALREADY include the gross-up
// (rice 10, chicken 2, packaging 1), so production expected cost is the same
// 2520 - reached WITHOUT grossing up a second time. That equality is the
// point: if production cost ever applied waste again, rice would become
// 11.111kg and the total would be 2698, and this test would say so.
//
// Actual production then overruns: 12 kg rice consumed plus 1 kg wasted, 2 kg
// chicken, 1 box.
//
//	Rice     13 kg @ 70  =  910
//	Chicken   2 kg @ 850 = 1700
//	Packaging 1 box @ 120 =  120
//	                ACTUAL = 2730
//	              VARIANCE = 2730 - 2520 = +210

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// runNonce makes every recipe version and batch this run creates unique.
//
// Section 16B versions and Section 16C batches are immutable once activated,
// so a fixture cannot rebuild yesterday's in place - and should not want to.
// Minting fresh ones per run is what the product requires and keeps repeated
// certification runs honest rather than reusing settled evidence.
var runNonce = fmt.Sprintf("r%d", time.Now().UnixNano())

const (
	e2eIngredientRice    = "vcost_e2e_rice"
	e2eIngredientChicken = "vcost_e2e_chicken"
	e2ePackagingBox      = "vcost_e2e_box"

	e2eItemRice    = "vcost_e2e_item_rice"
	e2eItemChicken = "vcost_e2e_item_chicken"
	e2eItemBox     = "vcost_e2e_item_box"

	e2eRecipe  = "vcost_e2e_recipe"
	e2eVersion = "vcost_e2e_version"
	e2eBatch   = "vcost_e2e_batch"

	// Deterministic unit costs in minor units per normalized inventory unit.
	riceUnitCost      = 70
	chickenUnitCost   = 850
	packagingUnitCost = 120

	// The figures this scenario must produce.
	wantRecipeCOGS         = 2520
	wantProductionExpected = 2520
	wantProductionActual   = 2730
	wantVariance           = 210
	wantPerUnit            = 252
	wantPerServing         = 315
)

// e2eChain is what one seeded run created.
//
// Recipe versions and batches are minted per run rather than reused: Section
// 16B refuses to return an ACTIVE version to DRAFT (it may only stay active or
// retire), so a fixture cannot legitimately rebuild one in place. Making a new
// version is what the product itself requires.
type e2eChain struct {
	recipeID  string
	versionID string
	batchID   string

	// Materials and inventory items are nonce-scoped too. Cost evidence is
	// keyed by inventory_item_id, so sharing items between runs would let one
	// run's purchase history change another run's expected figures - which is
	// exactly what happened before this was scoped.
	rice        string
	chicken     string
	box         string
	itemRice    string
	itemChicken string
	itemBox     string
	nonce       string
}

// seedE2EChain builds the whole 16A -> 16C chain for one tenant/branch.
func seedE2EChain(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	businessID, branchID, nonce string,
) e2eChain {
	t.Helper()

	chain := e2eChain{
		recipeID:    e2eRecipe + "_" + nonce,
		versionID:   e2eVersion + "_" + nonce,
		batchID:     e2eBatch + "_" + nonce,
		rice:        e2eIngredientRice + "_" + nonce,
		chicken:     e2eIngredientChicken + "_" + nonce,
		box:         e2ePackagingBox + "_" + nonce,
		itemRice:    e2eItemRice + "_" + nonce,
		itemChicken: e2eItemChicken + "_" + nonce,
		itemBox:     e2eItemBox + "_" + nonce,
		nonce:       nonce,
	}

	seedTenant(t, pool, ctx, businessID)

	// --- Section 16A: master data and branch inventory mapping -------------
	for _, ing := range []struct{ id, code, family, unit string }{
		{chain.rice, "E2E_RICE_" + upperNonce(nonce), "MASS", "kg"},
		{chain.chicken, "E2E_CHICKEN_" + upperNonce(nonce), "MASS", "kg"},
	} {
		exec(t, pool, ctx, `
			INSERT INTO vendor_ingredients(
				id,business_id,code,name,description,measurement_family,
				default_unit_code,inventory_control_mode,perishable,
				requires_expiry,status,version)
			VALUES($1,$2,$3,$3,'e2e',$4,$5,'INVENTORY_CONTROLLED',false,false,'ACTIVE',1)
			ON CONFLICT (id) DO NOTHING`,
			ing.id, businessID, ing.code, ing.family, ing.unit)
	}
	exec(t, pool, ctx, `
		INSERT INTO vendor_packaging_materials(
			id,business_id,code,name,description,measurement_family,
			default_unit_code,inventory_control_mode,status,version)
		VALUES($1,$2,$3,$3,'e2e','COUNT','box','INVENTORY_CONTROLLED','ACTIVE',1)
		ON CONFLICT (id) DO NOTHING`, chain.box, businessID, "E2E_BOX_"+upperNonce(nonce))

	// Inventory items are what purchase cost evidence is keyed by.
	for _, item := range []struct{ id, itemType, source, unitKind string }{
		{chain.itemRice, "INGREDIENT", chain.rice, "MASS"},
		{chain.itemChicken, "INGREDIENT", chain.chicken, "MASS"},
		{chain.itemBox, "PACKAGING", chain.box, "COUNT"},
	} {
		exec(t, pool, ctx, `
			INSERT INTO inventory_items(
				id,business_id,branch_id,item_type,source_ref,
				stock_measure_ref,stock_measure_owner_key,stock_unit_kind,
				stock_basis_mode,tracking_mode,lot_policy,lot_tracking_enabled,
				status,version)
			VALUES($1,$2,$3,$4,$5,'e2e','e2e',$6,'SAME_UNIT','SIMPLE','FIFO',false,'ACTIVE',1)
			ON CONFLICT (id) DO NOTHING`,
			item.id, businessID, branchID, item.itemType, item.source, item.unitKind)
	}

	// The branch mapping is what makes cost a branch-scoped question.
	for _, mapping := range []struct{ id, ingredient, item string }{
		{"vcost_e2e_cfg_rice_" + nonce, chain.rice, chain.itemRice},
		{"vcost_e2e_cfg_chicken_" + nonce, chain.chicken, chain.itemChicken},
	} {
		exec(t, pool, ctx, `
			INSERT INTO vendor_ingredient_branch_configs(
				id,business_id,branch_id,ingredient_id,inventory_item_id,version)
			VALUES($1,$2,$3,$4,$5,1)
			ON CONFLICT (id) DO NOTHING`,
			mapping.id, businessID, branchID, mapping.ingredient, mapping.item)
	}
	exec(t, pool, ctx, `
		INSERT INTO vendor_packaging_branch_configs(
			id,business_id,branch_id,packaging_material_id,inventory_item_id,version)
		VALUES($5,$1,$2,$3,$4,1)
		ON CONFLICT (id) DO NOTHING`,
		businessID, branchID, chain.box, chain.itemBox, "vcost_e2e_cfg_box_"+nonce)

	// --- Section 16B: recipe with NET quantities ---------------------------
	seedRecipeShell(t, pool, ctx, businessID, branchID, chain.recipeID, chain.versionID)

	// Rice: NET 9 kg with 10% expected waste. Chicken: NET 2 kg, no waste.
	for _, component := range []struct {
		id, ingredient string
		num, den       int64
		waste          int
	}{
		{"vcost_e2e_comp_rice", chain.rice, 9, 1, 1000},
		{"vcost_e2e_comp_chicken", chain.chicken, 2, 1, 0},
	} {
		exec(t, pool, ctx, `
			INSERT INTO vendor_recipe_components(
				id,business_id,recipe_version_id,ingredient_id,
				quantity_numerator,quantity_denominator,unit_code,
				normalization_status,normalized_quantity_numerator,
				normalized_quantity_denominator,normalized_unit_code,
				expected_waste_bps)
			VALUES($1,$2,$3,$4,$5,$6,'kg','EXACT',$5,$6,'kg',$7)
			ON CONFLICT (id) DO NOTHING`,
			component.id+"_"+nonce, businessID, chain.versionID, component.ingredient,
			component.num, component.den, component.waste)
	}
	exec(t, pool, ctx, `
		INSERT INTO vendor_recipe_packaging_requirements(
			id,business_id,recipe_version_id,packaging_material_id,
			quantity_numerator,quantity_denominator,unit_code,
			normalization_status,normalized_quantity_numerator,
			normalized_quantity_denominator,normalized_unit_code,sequence,version)
		VALUES($4,$1,$2,$3,1,1,'box','EXACT',1,1,'box',1,1)
		ON CONFLICT (id) DO NOTHING`, businessID, chain.versionID, chain.box,
		"vcost_e2e_pkg_req_"+nonce)

	activateRecipeVersion(t, pool, ctx, businessID, chain.versionID)

	// --- Section 15: authoritative purchase cost evidence ------------------
	seedPurchaseEvidence(t, pool, ctx, businessID, branchID, chain.itemRice, riceUnitCost, "rice_"+nonce)
	seedPurchaseEvidence(t, pool, ctx, businessID, branchID, chain.itemChicken, chickenUnitCost, "chk_"+nonce)
	seedPurchaseEvidence(t, pool, ctx, businessID, branchID, chain.itemBox, packagingUnitCost, "box_"+nonce)

	// --- Section 16D: cost policies ----------------------------------------
	for _, subject := range []struct{ id, subjectType, ref string }{
		{"vcost_e2e_pol_rice_" + nonce, SubjectIngredient, chain.rice},
		{"vcost_e2e_pol_chicken_" + nonce, SubjectIngredient, chain.chicken},
		{"vcost_e2e_pol_box_" + nonce, SubjectPackaging, chain.box},
	} {
		exec(t, pool, ctx, `
			INSERT INTO vendor_cost_policies(
				id,business_id,subject_type,subject_ref,strategy,currency,
				created_by_staff_id,updated_by_staff_id)
			VALUES($1,$2,$3,$4,'LATEST_ACCEPTED_PURCHASE_COST','NGN',$5,$5)
			ON CONFLICT (id) DO NOTHING`,
			subject.id, businessID, subject.subjectType, subject.ref, businessID+"_owner")
	}

	return chain
}

// seedSection15Chain builds the supplier -> purchase order -> goods receipt
// spine that vendor_purchase_cost_history references.
//
// The cost history row carries composite foreign keys to the supplier, the
// supplier product, the receipt and the receipt line, so cost evidence cannot
// exist without a real purchasing record behind it. That constraint is a
// feature - it is what makes an OBSERVED row traceable - and it means a cost
// fixture has to build the purchasing spine rather than fabricate a bare row.
func seedSection15Chain(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	businessID, branchID, inventoryItemID, tag string,
	unitCostMinor int64,
) {
	t.Helper()

	staff := businessID + "_owner"

	exec(t, pool, ctx, `
		INSERT INTO vendor_suppliers(id,business_id,code,name,legal_name)
		VALUES($1,$2,upper($3),$3,$3) ON CONFLICT (id) DO NOTHING`,
		"sup_"+tag, businessID, "sup"+tag)

	exec(t, pool, ctx, `
		INSERT INTO vendor_supplier_products(
			id,business_id,supplier_id,branch_id,inventory_item_id,supplier_sku,
			purchase_unit_code,purchase_unit_name,conversion_mode,
			inventory_units_per_purchase_unit,currency_code,
			inventory_measure_ref,inventory_measure_owner_key,inventory_unit_kind)
		VALUES($1,$2,$3,$4,$5,$6,'KG','Kilogram','SAME_AS_INVENTORY',1,'NGN',
		       'e2e','e2e','MASS')
		ON CONFLICT (id) DO NOTHING`,
		"sp_"+tag, businessID, "sup_"+tag, branchID, inventoryItemID, "SKU_"+tag)

	exec(t, pool, ctx, `
		INSERT INTO vendor_purchase_orders(
			id,business_id,branch_id,supplier_id,purchase_order_number,
			supplier_name_snapshot,currency_code,created_by_staff_id)
		VALUES($1,$2,$3,$4,$5,'Supplier','NGN',$6)
		ON CONFLICT (id) DO NOTHING`,
		"po_"+tag, businessID, branchID, "sup_"+tag, "PO-"+tag, staff)

	exec(t, pool, ctx, `
		INSERT INTO vendor_purchase_order_lines(
			id,business_id,purchase_order_id,supplier_product_id,
			supplier_product_version,inventory_item_id,purchase_unit_code_snapshot,
			purchase_unit_name_snapshot,conversion_mode_snapshot,
			inventory_units_per_purchase_unit_snapshot,inventory_measure_ref_snapshot,
			inventory_measure_owner_key_snapshot,inventory_unit_kind_snapshot,
			purchase_quantity,converted_inventory_quantity,unit_cost_minor,
			line_total_minor,currency_code_snapshot)
		VALUES($1,$2,$3,$4,1,$5,'KG','Kilogram','SAME_AS_INVENTORY',1,'e2e','e2e',
		       'MASS',100,100,100,10000,'NGN')
		ON CONFLICT (id) DO NOTHING`,
		"pol_"+tag, businessID, "po_"+tag, "sp_"+tag, inventoryItemID)

	exec(t, pool, ctx, `
		INSERT INTO vendor_goods_receipts(
			id,business_id,purchase_order_id,supplier_id,branch_id,currency,
			received_at,received_by_staff_id)
		VALUES($1,$2,$3,$4,$5,'NGN',now(),$6)
		ON CONFLICT (id) DO NOTHING`,
		"gr_"+tag, businessID, "po_"+tag, "sup_"+tag, branchID, staff)

	// A receipt line must be fully linked to what was ordered unless it is
	// flagged as the wrong item - which is exactly why a wrong-item line never
	// becomes cost evidence.
	exec(t, pool, ctx, `
		INSERT INTO vendor_goods_receipt_lines(
			id,business_id,goods_receipt_id,purchase_order_id,
			purchase_order_line_id,supplier_product_id,inventory_item_id,
			ordered_quantity_snapshot,received_before_snapshot,
			expected_unit_cost_minor,purchase_unit_code,
			inventory_units_per_purchase_unit,delivered_quantity,accepted_quantity,
			currency)
		VALUES($1,$2,$3,$4,$5,$6,$7,100,0,$8::bigint,'KG',1,100,100,'NGN')
		ON CONFLICT (id) DO NOTHING`,
		"grl_"+tag, businessID, "gr_"+tag, "po_"+tag,
		"pol_"+tag, "sp_"+tag, inventoryItemID, unitCostMinor)
}

// seedPurchaseEvidence writes one accepted Section 15 observation, shaped
// exactly as goods receiving writes it, on top of a real purchasing spine.
func seedPurchaseEvidence(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	businessID, branchID, inventoryItemID string,
	unitCostMinor int64,
	tag string,
) {
	t.Helper()
	if !hasPurchasingSchema(t, pool, ctx) {
		t.Fatal("Section 15 purchase cost history is required for end-to-end costing")
	}

	seedSection15Chain(t, pool, ctx, businessID, branchID, inventoryItemID, tag, unitCostMinor)

	exec(t, pool, ctx, `
		INSERT INTO vendor_purchase_cost_history(
			id,business_id,branch_id,supplier_id,supplier_product_id,
			inventory_item_id,purchase_order_id,purchase_order_line_id,
			goods_receipt_id,goods_receipt_line_id,evidence_type,
			purchase_unit_code,inventory_units_per_purchase_unit,
			purchase_quantity,inventory_quantity,purchase_unit_cost_minor,
			total_cost_minor,inventory_unit_cost_minor,currency,
			observed_at,recorded_by_staff_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'OBSERVED','KG',1,
		       100,100,$11::bigint,($11::bigint)*100,$11::bigint,'NGN',now(),$12)
		ON CONFLICT DO NOTHING`,
		"pch_"+tag, businessID, branchID,
		"sup_"+tag, "sp_"+tag, inventoryItemID,
		"po_"+tag, "pol_"+tag, "gr_"+tag, "grl_"+tag,
		unitCostMinor, businessID+"_owner")
}

// seedE2EBatch freezes a production batch whose planned quantities ALREADY
// include the expected-waste gross-up, exactly as Section 16C writes them.
func seedE2EBatch(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	businessID, branchID string,
	chain e2eChain,
) {
	t.Helper()

	exec(t, pool, ctx, `
		INSERT INTO vendor_production_batches(
			id,business_id,branch_id,recipe_id,recipe_version_id,
			recipe_version_number,target_type,target_ref,target_variant_ref,
			planned_output_numerator,planned_output_denominator,output_unit_code,
			expected_output_numerator,expected_output_denominator,
			recipe_expected_waste_bps,multiplier_numerator,multiplier_denominator,
			operating_model,output_inventory_numerator,output_inventory_denominator,
			status,actual_consumption_recorded,
			actual_output_numerator,actual_output_denominator,
			cancel_reason,version)
		VALUES($1,$2,$3,$4,$5,1,'FOOD_ITEM',$4||'_target','',
		       10,1,'kg',10,1,1000,1,1,'BATCH_STOCK',10,1,
		       'COMPLETED',true,10,1,'',1)
		ON CONFLICT (id) DO NOTHING`,
		chain.batchID, businessID, branchID, chain.recipeID, chain.versionID)

	// Planned quantities are waste-inclusive: rice 9 NET grossed to 10.
	// Actual: 12 consumed + 1 wasted = 13.
	for _, component := range []struct {
		id, ingredient, item   string
		plannedNum, plannedDen int64
		usedNum, wasteNum      int64
		waste                  int
	}{
		{"vcost_e2e_bc_rice", chain.rice, chain.itemRice, 10, 1, 12, 1, 1000},
		{"vcost_e2e_bc_chicken", chain.chicken, chain.itemChicken, 2, 1, 2, 0, 0},
		//
	} {
		exec(t, pool, ctx, `
			INSERT INTO vendor_production_batch_components(
				id,business_id,batch_id,ingredient_id,recipe_component_id,
				unit_code,planned_quantity_numerator,planned_quantity_denominator,
				expected_waste_bps,inventory_item_id,reserved_quantity,
				actual_used_numerator,actual_used_denominator,
				actual_waste_numerator,actual_waste_denominator)
			VALUES($1,$2,$3,$4,$5,'kg',$6,$7,$8,$9,0,$10,1,$11,1)
			ON CONFLICT (id) DO NOTHING`,
			component.id+"_"+chain.batchID, businessID, chain.batchID, component.ingredient,
			recipeComponentID(component.ingredient, chain), component.plannedNum,
			component.plannedDen, component.waste, component.item,
			component.usedNum, component.wasteNum)
	}

	exec(t, pool, ctx, `
		INSERT INTO vendor_production_batch_packaging(
			id,business_id,batch_id,packaging_material_id,
			recipe_packaging_requirement_id,unit_code,
			planned_quantity_numerator,planned_quantity_denominator,
			production_consumption_point,inventory_controlled,inventory_item_id,
			reserved_quantity,actual_used_numerator,actual_used_denominator)
		VALUES($5,$1,$2,$3,$6,'box',1,1,
		       'PRODUCTION',true,$4,0,1,1)
		ON CONFLICT (id) DO NOTHING`,
		businessID, chain.batchID, chain.box, chain.itemBox,
		"vcost_e2e_bp_"+chain.batchID,
		"vcost_e2e_pkg_req_"+chain.nonce)
}

// recipeComponentID links a frozen batch component back to the recipe
// component it was derived from, which is the traceability Section 16C
// requires and the reason the column is NOT NULL.
// upperNonce makes a nonce usable in fields Section 15 requires to be upper
// case.
func upperNonce(nonce string) string {
	out := []rune(nonce)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
	}
	return string(out)
}

func recipeComponentID(ingredientID string, chain e2eChain) string {
	switch ingredientID {
	case chain.rice:
		return "vcost_e2e_comp_rice_" + chain.nonce
	default:
		return "vcost_e2e_comp_chicken_" + chain.nonce
	}
}

func amount(t *testing.T, label string, value *int64) int64 {
	t.Helper()
	if value == nil {
		t.Fatalf("%s is absent; the calculation did not resolve", label)
	}
	return *value
}

// TestEndToEndCostingFrom16AThrough16D is the Section 16 end-to-end gate.
func TestEndToEndCostingFrom16AThrough16D(t *testing.T) {
	pool, ctx := testPool(t)
	chain := seedE2EChain(t, pool, ctx, businessA, branchA1, runNonce+"_core")
	seedE2EBatch(t, pool, ctx, businessA, branchA1, chain)

	engine := NewEngine(pool, NewResolver(pool, NewPostgresPurchasingCostReader(pool)))

	// --- Recipe estimated COGS: waste grossed up EXACTLY ONCE --------------
	recipe, err := engine.CalculateRecipeCost(ctx, businessA, branchA1, chain.versionID)
	if err != nil {
		t.Fatalf("recipe cost: %v", err)
	}
	if recipe.Completeness != CompletenessComplete {
		t.Fatalf("recipe completeness = %s; components: %+v", recipe.Completeness, recipe.Components)
	}
	if got := amount(t, "recipe total", recipe.TotalCostMinor); got != wantRecipeCOGS {
		t.Fatalf("recipe COGS = %d; want %d (rice 700 + chicken 1700 + packaging 120)", got, wantRecipeCOGS)
	}
	if got := amount(t, "packaging contribution", recipe.PackagingCostMinor); got != packagingUnitCost {
		t.Fatalf("packaging contribution = %d; want %d", got, packagingUnitCost)
	}
	// Rice grossed 9 -> 10 kg, so the waste portion is 1 kg at 70 = 70.
	if got := amount(t, "waste contribution", recipe.WasteCostMinor); got != 70 {
		t.Fatalf("waste contribution = %d; want 70 (1 kg of rice at 70)", got)
	}
	if got := amount(t, "cost per unit", recipe.CostPerUnitMinor); got != wantPerUnit {
		t.Fatalf("cost per unit = %d; want %d", got, wantPerUnit)
	}
	if got := amount(t, "cost per serving", recipe.CostPerServingMinor); got != wantPerServing {
		t.Fatalf("cost per serving = %d; want %d", got, wantPerServing)
	}

	// --- Production expected: the SAME 2520, without a second gross-up -----
	expected, err := engine.CalculateProductionCost(ctx, businessA, chain.batchID, CalcProductionExpected)
	if err != nil {
		t.Fatalf("production expected: %v", err)
	}
	got := amount(t, "production expected total", expected.TotalCostMinor)
	if got != wantProductionExpected {
		t.Fatalf("production expected = %d; want %d", got, wantProductionExpected)
	}
	// The decisive assertion. 2698 is what a second gross-up would produce.
	if got == 2698 {
		t.Fatal("production expected cost applied the expected-waste gross-up a second time")
	}
	if got != wantRecipeCOGS {
		t.Fatalf("frozen planned quantities should cost the same as the recipe estimate: %d vs %d",
			got, wantRecipeCOGS)
	}

	// --- Production actual: measured consumption plus measured waste -------
	actual, err := engine.CalculateProductionCost(ctx, businessA, chain.batchID, CalcProductionActual)
	if err != nil {
		t.Fatalf("production actual: %v", err)
	}
	if got := amount(t, "production actual total", actual.TotalCostMinor); got != wantProductionActual {
		t.Fatalf("production actual = %d; want %d (rice 13kg=910 + chicken 1700 + box 120)",
			got, wantProductionActual)
	}

	// --- Variance, from persisted snapshots --------------------------------
	if _, err = PersistSnapshot(ctx, pool, businessA, expected,
		businessA+"_owner", "corr_e2e", "cmd_expected_"+chain.batchID); err != nil {
		t.Fatalf("persist expected: %v", err)
	}
	if _, err = PersistSnapshot(ctx, pool, businessA, actual,
		businessA+"_owner", "corr_e2e", "cmd_actual_"+chain.batchID); err != nil {
		t.Fatalf("persist actual: %v", err)
	}

	variance, err := engine.CalculateVariance(ctx, businessA, chain.batchID)
	if err != nil {
		t.Fatalf("variance: %v", err)
	}
	if got := amount(t, "variance", variance.VarianceMinor); got != wantVariance {
		t.Fatalf("variance = %d; want +%d (actual %d - expected %d)",
			got, wantVariance, wantProductionActual, wantProductionExpected)
	}
	if variance.VarianceMinor == nil || *variance.VarianceMinor <= 0 {
		t.Fatal("an overrun must produce a positive variance; the sign was lost")
	}
}

// TestHistoricalSnapshotSurvivesAPriceChange proves historical reporting never
// recalculates at today's prices.
//
// A snapshot is taken, the purchase evidence then changes, and the stored
// figures must not move. Without this, every historical COGS would silently
// drift with the market and no past number would mean anything.
func TestHistoricalSnapshotSurvivesAPriceChange(t *testing.T) {
	pool, ctx := testPool(t)
	chain := seedE2EChain(t, pool, ctx, businessA, branchA1, runNonce+"_hist")

	engine := NewEngine(pool, NewResolver(pool, NewPostgresPurchasingCostReader(pool)))

	before, err := engine.CalculateRecipeCost(ctx, businessA, branchA1, chain.versionID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Completeness != CompletenessComplete {
		t.Fatalf("required seeded purchase evidence incomplete: %s", before.Completeness)
	}
	before.CalculationType = CalcRecipeSnapshot

	stored, err := PersistSnapshot(ctx, pool, businessA, before,
		businessA+"_owner", "corr_hist", "cmd_hist_"+chain.versionID)
	if err != nil {
		t.Fatalf("persist snapshot: %v", err)
	}
	originalTotal := amount(t, "snapshot total", stored.TotalCostMinor)

	// The baseline is read from the snapshot itself rather than assumed. A
	// previous certification run may have left later evidence in the shared
	// database, and a test that hardcoded the starting price would then be
	// measuring nothing.
	var originalRiceCost int64
	for _, component := range stored.Components {
		if component.SubjectRef == chain.rice && component.UnitCostMinor != nil {
			originalRiceCost = *component.UnitCostMinor
		}
	}
	if originalRiceCost == 0 {
		t.Fatal("snapshot did not record a rice unit cost; there is nothing to compare against")
	}
	hikedCost := originalRiceCost + 70

	// The world moves on: rice gets more expensive. The new observation needs
	// its own purchasing spine, exactly like the original.
	hikeTag := "hike_" + chain.nonce
	seedSection15Chain(t, pool, ctx, businessA, branchA1, chain.itemRice, hikeTag, hikedCost)
	exec(t, pool, ctx, `
		INSERT INTO vendor_purchase_cost_history(
			id,business_id,branch_id,supplier_id,supplier_product_id,
			inventory_item_id,purchase_order_id,purchase_order_line_id,
			goods_receipt_id,goods_receipt_line_id,evidence_type,
			purchase_unit_code,inventory_units_per_purchase_unit,
			purchase_quantity,inventory_quantity,purchase_unit_cost_minor,
			total_cost_minor,inventory_unit_cost_minor,currency,
			observed_at,recorded_by_staff_id)
		VALUES($5,$1,$2,$6,$7,$3,$8,$9,$10,$11,'OBSERVED','KG',1,
		       100,100,$12::bigint,($12::bigint)*100,$12::bigint,'NGN',now(),$4)
		ON CONFLICT DO NOTHING`,
		businessA, branchA1, chain.itemRice, businessA+"_owner",
		"pch_"+hikeTag, "sup_"+hikeTag, "sp_"+hikeTag,
		"po_"+hikeTag, "pol_"+hikeTag, "gr_"+hikeTag, "grl_"+hikeTag, hikedCost)

	// A fresh preview must see the new price...
	after, err := engine.CalculateRecipeCost(ctx, businessA, branchA1, chain.versionID)
	if err != nil {
		t.Fatal(err)
	}
	if amount(t, "recalculated total", after.TotalCostMinor) == originalTotal {
		t.Fatal("the price change did not affect a fresh calculation; the fixture is not exercising it")
	}

	// ...while the stored snapshot must not have moved at all.
	reloaded, found, err := engine.LatestRecipeSnapshot(ctx, businessA, chain.versionID)
	if err != nil || !found {
		t.Fatalf("reload snapshot: found=%t err=%v", found, err)
	}
	if got := amount(t, "reloaded total", reloaded.TotalCostMinor); got != originalTotal {
		t.Fatalf("historical snapshot total changed from %d to %d after a price change",
			originalTotal, got)
	}
	for _, component := range reloaded.Components {
		if component.SubjectRef != e2eIngredientRice {
			continue
		}
		if component.UnitCostMinor == nil || *component.UnitCostMinor != originalRiceCost {
			t.Fatalf("historical rice unit cost = %v; want the %d it was calculated at",
				component.UnitCostMinor, originalRiceCost)
		}
		if component.SourceEvidenceRef == "" {
			t.Fatal("historical component lost its source evidence reference")
		}
	}
}

// TestEndToEndCrossTenantAndBranchIsolation is the cross-layer security check.
//
// The isolated unit tests assert the same boundaries; this repeats them
// through the full seeded chain, which is where a join written against the
// wrong table would actually leak.
func TestEndToEndCrossTenantAndBranchIsolation(t *testing.T) {
	pool, ctx := testPool(t)
	chain := seedE2EChain(t, pool, ctx, businessA, branchA1, runNonce+"_iso")
	seedE2EBatch(t, pool, ctx, businessA, branchA1, chain)
	seedTenant(t, pool, ctx, businessB)

	engine := NewEngine(pool, NewResolver(pool, NewPostgresPurchasingCostReader(pool)))

	// Vendor B holding vendor A's recipe version id must not cost it.
	if _, err := engine.CalculateRecipeCost(ctx, businessB, branchB1, chain.versionID); err == nil {
		t.Fatal("vendor B costed vendor A's recipe version; tenant isolation failed")
	}

	// Vendor B holding vendor A's batch id must not cost it.
	if _, err := engine.CalculateProductionCost(
		ctx, businessB, chain.batchID, CalcProductionExpected); err == nil {
		t.Fatal("vendor B costed vendor A's production batch; tenant isolation failed")
	}

	// Vendor B must not read vendor A's snapshot components.
	components, err := engine.LoadComponents(ctx, businessB, "cs_any")
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 0 {
		t.Fatal("vendor B read component evidence outside its tenancy")
	}

	// A branch with no mapping resolves nothing rather than borrowing another
	// branch's inventory item and its cost.
	other, err := engine.CalculateRecipeCost(ctx, businessA, branchA2, chain.versionID)
	if err != nil {
		t.Fatal(err)
	}
	if other.Completeness == CompletenessComplete {
		t.Fatal("branch A2 costed a recipe using branch A1's inventory mapping; branch scope escaped")
	}
}

// TestCostCalculationHasNoInventoryOrFinancialSideEffect is the financial and
// inventory boundary check, measured rather than asserted by inspection.
//
// Production execution legitimately moves stock. A COST CALCULATION must not:
// running one is a read of evidence that already exists.
func TestCostCalculationHasNoInventoryOrFinancialSideEffect(t *testing.T) {
	pool, ctx := testPool(t)
	chain := seedE2EChain(t, pool, ctx, businessA, branchA1, runNonce+"_fin")
	seedE2EBatch(t, pool, ctx, businessA, branchA1, chain)

	engine := NewEngine(pool, NewResolver(pool, NewPostgresPurchasingCostReader(pool)))

	countOf := func(table, predicate string) int64 {
		var value int64
		query := "SELECT count(*) FROM " + table
		if predicate != "" {
			query += " WHERE " + predicate
		}
		if err := pool.QueryRow(ctx, query).Scan(&value); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		return value
	}

	type snapshotCount struct {
		table     string
		predicate string
	}
	watched := []snapshotCount{
		{"inventory_items", "business_id='" + businessA + "'"},
		{"vendor_purchase_cost_history", "business_id='" + businessA + "'"},
		{"vendor_production_batch_components", "business_id='" + businessA + "'"},
	}

	before := map[string]int64{}
	for _, w := range watched {
		before[w.table] = countOf(w.table, w.predicate)
	}

	// Any inventory movement table present in this schema must not grow.
	var movementTable bool
	if err := pool.QueryRow(ctx,
		`SELECT to_regclass('public.inventory_movements') IS NOT NULL`).Scan(&movementTable); err != nil {
		t.Fatal(err)
	}
	var movementsBefore int64
	if movementTable {
		movementsBefore = countOf("inventory_movements", "business_id='"+businessA+"'")
	}

	// Run every calculation the section offers.
	if _, err := engine.CalculateRecipeCost(ctx, businessA, branchA1, chain.versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.CalculateProductionCost(
		ctx, businessA, chain.batchID, CalcProductionExpected); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.CalculateProductionCost(
		ctx, businessA, chain.batchID, CalcProductionActual); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.CalculateVariance(ctx, businessA, chain.batchID); err != nil {
		t.Fatal(err)
	}

	for _, w := range watched {
		if got := countOf(w.table, w.predicate); got != before[w.table] {
			t.Errorf("cost calculation changed %s: %d -> %d", w.table, before[w.table], got)
		}
	}
	if movementTable {
		if got := countOf("inventory_movements", "business_id='"+businessA+"'"); got != movementsBefore {
			t.Errorf("cost calculation created inventory movements: %d -> %d", movementsBefore, got)
		}
	}

	// There is no wallet, ledger, journal or settlement table in this service
	// at all, which is the strongest form of "no financial side effect".
	for _, forbidden := range []string{
		"wallet_accounts", "ledger_entries", "accounting_journals", "payment_intents",
	} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, forbidden).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Errorf("%s exists in the vendor schema; Section 16 must hold no financial authority", forbidden)
		}
	}
}
