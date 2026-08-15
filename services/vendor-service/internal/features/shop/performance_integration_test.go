package shop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestSection12AModerateCatalogueQueries guards the management read shapes
// against accidental N+1 regressions with the specification's moderate
// 2,000-product / 5,000-variant scale. Rows are inserted set-wise because the
// subject is read performance, not repeating command validation 7,000 times.
func TestSection12AModerateCatalogueQueries(t *testing.T) {
	f := section12Fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scope := Scope{Global: true}
	category, err := f.service.CreateCategory(ctx, f.actorA, scope, CreateCategoryInput{Name: "Performance Fixture"})
	if err != nil {
		t.Fatal(err)
	}
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	productPrefix, variantPrefix := "s12perf_p_"+suffix+"_", "s12perf_v_"+suffix+"_"
	identifierPrefix := "PERF" + suffix
	if _, err = f.pool.Exec(ctx, `INSERT INTO shop_products
		(id,business_id,category_id,name,product_class,status)
		SELECT $1||g,$2,$3,'Performance Product '||g,'PACKAGED','ACTIVE'
		FROM generate_series(1,2000) g`, productPrefix, f.actorA.VendorID, category.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO shop_variants
		(id,business_id,product_id,name,package_level,status,position)
		SELECT $1||g,$2,$3||(((g-1)%2000)+1),'Performance Variant '||g,'EACH','ACTIVE',g
		FROM generate_series(1,5000) g`, variantPrefix, f.actorA.VendorID, productPrefix); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO shop_variant_handling_flags
		(id,business_id,variant_id,handling_flag)
		SELECT 'svh_'||$1||g,$2,$3||g,'KEEP_DRY' FROM generate_series(1,5000) g`, suffix, f.actorA.VendorID, variantPrefix); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO shop_product_identifiers
		(id,business_id,product_id,variant_id,identifier_type,identifier_value,status)
		SELECT 'sid_'||$1||g,$2,$3||(((g-1)%2000)+1),$4||g,'BARCODE',$5||g,'ACTIVE'
		FROM generate_series(1,5000) g`, suffix, f.actorA.VendorID, productPrefix, variantPrefix, identifierPrefix); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	products, more, err := f.service.ListProducts(ctx, f.actorA, scope, ProductListOptions{
		CategoryID: category.ID, Limit: 100,
	})
	if err != nil || len(products) != 100 || !more {
		t.Fatalf("moderate product page: count=%d more=%v err=%v", len(products), more, err)
	}
	variants, err := f.service.ListVariants(ctx, f.actorA, scope, productPrefix+"1", false)
	if err != nil || len(variants) < 2 {
		t.Fatalf("moderate product traversal: count=%d err=%v", len(variants), err)
	}
	for _, variant := range variants {
		if !contains(variant.HandlingFlags, "KEEP_DRY") {
			t.Fatalf("batched handling flags missing from %s", variant.ID)
		}
	}
	identifier, variant, product, err := f.service.LookupByIdentifier(ctx, f.actorA, scope, identifierPrefix+"5000", "BARCODE")
	if err != nil || identifier.VariantID != variant.ID || variant.ProductID != product.ID {
		t.Fatalf("moderate barcode lookup mismatch: identifier=%+v variant=%+v product=%+v err=%v", identifier, variant, product, err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("moderate catalogue reads took %s", elapsed)
	}
}

// TestSection12BDiscoveryAndInventoryScale exercises the final discovery read
// shape at the specification's 10k-product/20k-variant example scale. The
// fixture deliberately includes inventory lots, handling flags, fresh options,
// wholesale breaks and sell units while the read remains a bounded page.
func TestSection12BDiscoveryAndInventoryScale(t *testing.T) {
	f := section12Fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	scope := Scope{Global: true}
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	categoryID := "s12bperf_cat_" + suffix
	productPrefix := "s12bperf_p_" + suffix + "_"
	variantPrefix := "s12bperf_v_" + suffix + "_"
	unitPrefix := "s12bperf_u_" + suffix + "_"
	listingPrefix := "s12bperf_l_" + suffix + "_"
	basisPrefix := "s12bperf_b_" + suffix + "_"
	if _, err := f.pool.Exec(ctx, `INSERT INTO shop_categories(id,business_id,name,slug,status) VALUES($1,$2,'Section 12B Scale',$3,'ACTIVE')`, categoryID, f.actorA.VendorID, "s12b-scale-"+suffix); err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO shop_products(id,business_id,category_id,name,product_class,status)
		 SELECT $1||g,$2,$3,'Scale Product '||g,CASE WHEN g<=100 THEN 'FRESH_PRODUCE' ELSE 'PACKAGED' END,'ACTIVE' FROM generate_series(1,10000) g`, []any{productPrefix, f.actorA.VendorID, categoryID}},
		{`INSERT INTO shop_variants(id,business_id,product_id,name,package_level,retail_price_minor,wholesale_price_minor,status,position)
		 SELECT $1||g,$2,$3||(((g-1)/2)+1),'Scale Variant '||g,'EACH',1000,900,'ACTIVE',g FROM generate_series(1,20000) g`, []any{variantPrefix, f.actorA.VendorID, productPrefix}},
		{`INSERT INTO shop_variant_handling_flags(id,business_id,variant_id,handling_flag)
		 SELECT 's12bperf_h_'||$1||g,$2,$3||g,'KEEP_DRY' FROM generate_series(1,20000) g`, []any{suffix, f.actorA.VendorID, variantPrefix}},
		{`INSERT INTO shop_sell_units(id,business_id,product_id,variant_id,measure_definition_id,measure_owner_key,display_name,measurement_mode,conversion_type,exact_base_family,exact_base_quantity,price_minor,wholesale_price_minor,retail_enabled,wholesale_enabled,status)
		 SELECT $1||g,$2,$3||(((g-1)/2)+1),$4||g,'smd_piece','PLATFORM','each','FIXED','EXACT','COUNT',1,1000,900,true,g<=100,'ACTIVE' FROM generate_series(1,20000) g`, []any{unitPrefix, f.actorA.VendorID, productPrefix, variantPrefix}},
		{`INSERT INTO shop_branch_listings(id,business_id,branch_id,product_id,variant_id,retail_enabled,wholesale_enabled,status)
		 SELECT $1||g,$2,$3,$4||(((g-1)/2)+1),$5||g,true,g<=100,'ACTIVE' FROM generate_series(1,20000) g`, []any{listingPrefix, f.actorA.VendorID, f.branchShop, productPrefix, variantPrefix}},
		{`INSERT INTO shop_listing_sell_units(id,business_id,listing_id,sell_unit_id,variant_id)
		 SELECT 's12bperf_lu_'||$1||g,$2,$3||g,$4||g,$5||g FROM generate_series(1,20000) g`, []any{suffix, f.actorA.VendorID, listingPrefix, unitPrefix, variantPrefix}},
		{`INSERT INTO shop_inventory_stock_bases(id,business_id,branch_id,listing_id,product_id,variant_id,measure_definition_id,measure_owner_key,consumption_mode,lot_policy,lot_tracking_enabled,status)
		 SELECT $1||g,$2,$3,$4||g,$5||(((g-1)/2)+1),$6||g,'smd_piece','PLATFORM','SAME_UNIT','FIFO',true,'ACTIVE' FROM generate_series(1,20000) g`, []any{basisPrefix, f.actorA.VendorID, f.branchShop, listingPrefix, productPrefix, variantPrefix}},
		{`INSERT INTO shop_inventory_balances(id,business_id,branch_id,stock_basis_id,variant_id,on_hand_quantity)
		 SELECT 's12bperf_bal_'||$1||g,$2,$3,$4||g,$5||g,100 FROM generate_series(1,20000) g`, []any{suffix, f.actorA.VendorID, f.branchShop, basisPrefix, variantPrefix}},
		{`INSERT INTO shop_inventory_lots(id,business_id,branch_id,stock_basis_id,variant_id,source_ref,received_at,on_hand_quantity,status)
		 SELECT 's12bperf_lot_'||$1||g,$2,$3,$4||g,$5||g,'SCALE_FIXTURE',now(),100,'ACTIVE' FROM generate_series(1,20000) g`, []any{suffix, f.actorA.VendorID, f.branchShop, basisPrefix, variantPrefix}},
		{`INSERT INTO shop_fresh_attributes(id,business_id,product_id,attribute_type,selection_required,status,position)
		 SELECT 's12bperf_f_'||$1||g,$2,$3||g,'RIPENESS',true,'ACTIVE',0 FROM generate_series(1,100) g`, []any{suffix, f.actorA.VendorID, productPrefix}},
		{`INSERT INTO shop_fresh_attribute_options(id,business_id,attribute_id,option_value,display_name,is_default,status)
		 SELECT 's12bperf_fo_'||$1||g,$2,'s12bperf_f_'||$1||g,'RIPE','Ripe',true,'ACTIVE' FROM generate_series(1,100) g`, []any{suffix, f.actorA.VendorID}},
		{`INSERT INTO shop_quantity_breaks(id,business_id,listing_id,sell_unit_id,min_quantity,max_quantity,unit_price_minor,status)
		 SELECT 's12bperf_q_'||$1||g,$2,$3||g,$4||g,10,NULL,850,'ACTIVE' FROM generate_series(1,100) g`, []any{suffix, f.actorA.VendorID, listingPrefix, unitPrefix}},
	}
	for index, statement := range statements {
		if _, err := f.pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("scale seed statement %d: %v", index+1, err)
		}
	}
	started := time.Now()
	items, more, _, _, err := f.service.DiscoverCatalogue(ctx, f.actorA, scope, f.branchShop, DiscoveryOptions{Limit: 100})
	if err != nil || len(items) != 100 || !more {
		t.Fatalf("scale discovery count=%d more=%v err=%v", len(items), more, err)
	}
	for _, item := range items {
		if item.Sellability.Availability.PhysicalAvailable != 100 || len(item.SellUnits) != 1 || len(item.HandlingFlags) != 1 {
			t.Fatalf("unhydrated scale item=%+v", item)
		}
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("20k-variant bounded discovery took %s", elapsed)
	}
	if _, err = f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "scale-reservation-" + suffix, Lines: []ReserveInventoryLineInput{{StockBasisID: basisPrefix + "20000", Quantity: 25}}}); err != nil {
		t.Fatal(err)
	}
	availability, err := f.service.EffectiveAvailability(ctx, f.actorA, scope, f.branchShop, variantPrefix+"20000")
	if err != nil || availability.PhysicalAvailable != 75 {
		t.Fatalf("indexed scale reservation availability=%+v err=%v", availability, err)
	}
}
