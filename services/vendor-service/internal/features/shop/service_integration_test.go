package shop

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/events"
)

type allowAllScopes struct{}

func (allowAllScopes) ResolveScope(context.Context, string, string) (Scope, error) {
	return Scope{Global: true}, nil
}

type shopFixture struct {
	pool             *pgxpool.Pool
	service          *Service
	actorA, actorB   Actor
	branchShop       string
	branchShop2      string
	branchRestaurant string
}

func section12Fixture(t *testing.T) shopFixture {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping Shop integration")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	suffix := uuid.NewString()
	businessA, businessB := "s12a_a_"+suffix, "s12a_b_"+suffix
	staffA, staffB := "s12a_staff_a_"+suffix, "s12a_staff_b_"+suffix
	userA, userB := "s12a_user_a_"+suffix, "s12a_user_b_"+suffix
	branchShop, branchShop2 := "s12a_shop_"+suffix, "s12a_shop2_"+suffix
	branchRestaurant, branchB := "s12a_rest_"+suffix, "s12a_bbranch_"+suffix

	for _, seed := range []struct{ user, staff, business string }{{userA, staffA, businessA}, {userB, staffB, businessB}} {
		if _, err = pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES($1,$2)`, seed.user, seed.user+"@example.test"); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO vendor_staff(id,vendor_id,user_id) VALUES($1,$2,$3)`, seed.staff, seed.business, seed.user); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone) VALUES($1,'Section 12 test','ACTIVE',$2,'NGN','Africa/Lagos')`, seed.business, seed.staff); err != nil {
			t.Fatal(err)
		}
	}
	for _, branch := range []struct{ id, business, name string }{{branchShop, businessA, "Mixed Shop"}, {branchShop2, businessA, "Second Shop"}, {branchRestaurant, businessA, "Restaurant Only"}, {branchB, businessB, "Foreign Shop"}} {
		if _, err = pool.Exec(ctx, `INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status) VALUES($1,$2,$3,'1 Test Street','Lagos','Lagos','NG',6.5244,3.3792,'Africa/Lagos','ACTIVE')`, branch.id, branch.business, branch.name); err != nil {
			t.Fatal(err)
		}
	}
	capabilities := []struct{ business, branch, capability string }{
		{businessA, branchShop, "SHOP_GROCERY"}, {businessA, branchShop, "WHOLESALE"}, {businessA, branchShop, "MARKET"}, {businessA, branchShop, "RESTAURANT"},
		{businessA, branchShop2, "SHOP_GROCERY"},
		{businessA, branchRestaurant, "RESTAURANT"}, {businessB, branchB, "SHOP_GROCERY"},
	}
	capabilityIDs := map[string]string{}
	for i, cap := range capabilities {
		key := cap.business + ":" + cap.capability
		capID := capabilityIDs[key]
		if capID == "" {
			capID = "s12a_cap_" + itoa(i) + "_" + suffix
			capabilityIDs[key] = capID
			if _, err = pool.Exec(ctx, `INSERT INTO vendor_capabilities(id,business_id,capability_type,status) VALUES($1,$2,$3,'ACTIVE')`, capID, cap.business, cap.capability); err != nil {
				t.Fatal(err)
			}
		}
		assignmentID := "s12a_bcap_" + itoa(i) + "_" + suffix
		if _, err = pool.Exec(ctx, `INSERT INTO vendor_branch_capabilities(id,business_id,branch_id,vendor_capability_id,capability_type,status) VALUES($1,$2,$3,$4,$5,'ACTIVE')`, assignmentID, cap.business, cap.branch, capID, cap.capability); err != nil {
			t.Fatal(err)
		}
	}
	flags := Flags{ShopEnabled: true, WritesEnabled: true}
	return shopFixture{pool: pool, service: NewService(Options{DB: pool, Scopes: allowAllScopes{}, Flags: flags, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool))}),
		actorA: Actor{StaffID: staffA, VendorID: businessA, CorrelationID: "s12a-test"}, actorB: Actor{StaffID: staffB, VendorID: businessB, CorrelationID: "s12a-test"},
		branchShop: branchShop, branchShop2: branchShop2, branchRestaurant: branchRestaurant}
}

func intp(v int) *int         { return &v }
func int64p(v int64) *int64   { return &v }
func int64pp(v int64) **int64 { p := &v; return &p }
func boolp(v bool) *bool      { return &v }
func stringPtrPtr(v string) **string {
	p := &v
	return &p
}

func TestSection12ACommercialDomainAndIsolation(t *testing.T) {
	f := section12Fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scope := Scope{Global: true}

	config, err := f.service.SetOperatingModels(ctx, f.actorA, scope, f.branchShop, SetOperatingModelsInput{PrimaryModel: "SUPERMARKET", Models: []string{"SUPERMARKET", "MARKET_TRADER", "WHOLESALER"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Models) != 3 {
		t.Fatalf("operating models=%d want=3", len(config.Models))
	}
	if _, err = f.service.SetOperatingModels(ctx, f.actorA, scope, f.branchRestaurant, SetOperatingModelsInput{Models: []string{"SUPERMARKET"}}); err == nil {
		t.Fatal("restaurant-only branch accepted Shop operating model")
	}

	foodstuff, err := f.service.CreateCategory(ctx, f.actorA, scope, CreateCategoryInput{Name: "Foodstuff"})
	if err != nil {
		t.Fatal(err)
	}
	riceCategory, err := f.service.CreateCategory(ctx, f.actorA, scope, CreateCategoryInput{ParentID: foodstuff.ID, Name: "Rice"})
	if err != nil {
		t.Fatal(err)
	}
	self := riceCategory.ID
	if _, err = f.service.UpdateCategory(ctx, f.actorA, scope, riceCategory.ID, UpdateCategoryInput{ParentID: &self, ExpectedVersion: riceCategory.Version}); err == nil {
		t.Fatal("category self-cycle accepted")
	}

	rice, err := f.service.CreateProduct(ctx, f.actorA, scope, CreateProductInput{CategoryID: riceCategory.ID, Name: "Local Rice", ProductClass: "LOCAL_FOODSTUFF", OperatingModels: []string{"FOODSTUFF_DEALER", "WHOLESALER"}})
	if err != nil {
		t.Fatal(err)
	}
	beans, err := f.service.CreateProduct(ctx, f.actorA, scope, CreateProductInput{CategoryID: riceCategory.ID, Name: "Brown Beans", ProductClass: "LOCAL_FOODSTUFF", OperatingModels: []string{"FOODSTUFF_DEALER"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.GetProduct(ctx, f.actorB, scope, rice.ID); err == nil {
		t.Fatal("cross-tenant product read succeeded")
	}
	customMeasure, err := f.service.CreateMeasure(ctx, f.actorA, scope, CreateMeasureInput{Code: "VENDOR_BOWL", DisplayName: "Vendor Bowl", Family: "LOCAL_CONTAINER", RegionCode: "NG-LA"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.AddMeasureAlias(ctx, f.actorA, scope, customMeasure.ID, "Big Market Bowl", "NG-LA"); err != nil {
		t.Fatal(err)
	}
	foreignMeasures, _, err := f.service.ListMeasures(ctx, f.actorB, scope, MeasureListOptions{Search: "Vendor Bowl", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(foreignMeasures) != 0 {
		t.Fatalf("tenant custom measure leaked: %+v", foreignMeasures)
	}
	paintAlias, err := f.service.AddMeasureAlias(ctx, f.actorA, scope, "smd_paint", "Paint Bucket "+suffixTail(rice.ID), "NG-LA")
	if err != nil {
		t.Fatalf("add tenant-scoped alias to platform PAINT: %v", err)
	}
	ownPaint, _, err := f.service.ListMeasures(ctx, f.actorA, scope, MeasureListOptions{Search: paintAlias.Alias, Limit: 100})
	if err != nil || len(ownPaint) != 1 || ownPaint[0].ID != "smd_paint" {
		t.Fatalf("platform measure not searchable by tenant alias: measures=%+v err=%v", ownPaint, err)
	}
	foreignPaint, _, err := f.service.ListMeasures(ctx, f.actorB, scope, MeasureListOptions{Search: paintAlias.Alias, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(foreignPaint) != 0 {
		t.Fatalf("tenant alias on platform PAINT leaked across businesses: %+v", foreignPaint)
	}
	foreignAliases, err := f.service.ListMeasureAliases(ctx, f.actorB, scope, "smd_paint")
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range foreignAliases {
		if alias.ID == paintAlias.ID {
			t.Fatal("tenant alias appeared in another business's PAINT aliases")
		}
	}

	bag, err := f.service.CreateVariant(ctx, f.actorA, scope, rice.ID, CreateVariantInput{SKU: "RICE-50KG-" + rice.ID[len(rice.ID)-6:], Name: "Rice 50kg Bag", PackageLevel: "BAG", NetWeightGrams: int64p(50000), RetailPriceMinor: int64p(5500000), WholesalePriceMinor: int64p(5000000), TemperatureZone: "AMBIENT", HandlingFlags: []string{"KEEP_DRY"}})
	if err != nil {
		t.Fatal(err)
	}
	pack, err := f.service.CreateVariant(ctx, f.actorA, scope, rice.ID, CreateVariantInput{Name: "Rice 5kg Pack", PackageLevel: "PACK", NetWeightGrams: int64p(5000), RetailPriceMinor: int64p(600000)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.AddPackageRelation(ctx, f.actorA, scope, rice.ID, bag.ID, pack.ID, 10); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.AddPackageRelation(ctx, f.actorA, scope, rice.ID, pack.ID, bag.ID, 1); err == nil {
		t.Fatal("packaging cycle accepted")
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO shop_package_relations
		(id,business_id,product_id,parent_variant_id,child_variant_id,units_per_parent,status)
		VALUES($1,$2,$3,$4,$5,1,'ACTIVE')`, "spk_direct_"+uuid.NewString(), f.actorA.VendorID, rice.ID, pack.ID, bag.ID); err == nil {
		t.Fatal("database accepted a direct packaging cycle")
	}
	if _, err = f.pool.Exec(ctx, `UPDATE shop_categories SET parent_id=$3
		WHERE business_id=$1 AND id=$2`, f.actorA.VendorID, foodstuff.ID, riceCategory.ID); err == nil {
		t.Fatal("database accepted a direct category cycle")
	}
	identifierValue := "KARRYGO-" + bag.ID
	identifier, err := f.service.AddIdentifier(ctx, f.actorA, scope, bag.ID, "BARCODE", identifierValue, "BAG")
	if err != nil {
		t.Fatal(err)
	}
	lookedUp, lookedVariant, lookedProduct, err := f.service.LookupByIdentifier(ctx, f.actorA, scope, identifierValue, "BARCODE")
	if err != nil || lookedUp.ID != identifier.ID || lookedVariant.ID != bag.ID || lookedProduct.ID != rice.ID {
		t.Fatalf("identifier lookup mismatch: id=%+v variant=%+v product=%+v err=%v", lookedUp, lookedVariant, lookedProduct, err)
	}

	ricePaint, err := f.service.CreateCalibration(ctx, f.actorA, scope, rice.ID, CreateCalibrationInput{VariantID: bag.ID, MeasureID: "smd_paint", WeightGrams: int64p(4200), ToleranceBps: 300, Source: "VENDOR_MEASURED"})
	if err != nil {
		t.Fatal(err)
	}
	beansVariant, err := f.service.CreateVariant(ctx, f.actorA, scope, beans.ID, CreateVariantInput{Name: "Beans Bulk", PackageLevel: "BAG"})
	if err != nil {
		t.Fatal(err)
	}
	beansPaint, err := f.service.CreateCalibration(ctx, f.actorA, scope, beans.ID, CreateCalibrationInput{VariantID: beansVariant.ID, MeasureID: "smd_paint", WeightGrams: int64p(3600), ToleranceBps: 300, Source: "VENDOR_MEASURED"})
	if err != nil {
		t.Fatal(err)
	}
	if *ricePaint.WeightGrams == *beansPaint.WeightGrams {
		t.Fatal("different commodities collapsed to one Paint weight")
	}
	branchDerica, err := f.service.CreateCalibration(ctx, f.actorA, scope, rice.ID, CreateCalibrationInput{
		BranchID: f.branchShop, VariantID: bag.ID, MeasureID: "smd_derica",
		WeightGrams: int64p(1800), ToleranceBps: 250, Source: "VENDOR_MEASURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	branchTwoDerica, err := f.service.CreateCalibration(ctx, f.actorA, scope, rice.ID, CreateCalibrationInput{
		BranchID: f.branchShop2, VariantID: bag.ID, MeasureID: "smd_derica",
		WeightGrams: int64p(1700), ToleranceBps: 250, Source: "VENDOR_MEASURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	branchOneCalibrations, err := f.service.ListCalibrations(ctx, f.actorA, Scope{BranchIDs: []string{f.branchShop}}, rice.ID, false)
	if err != nil || !containsCalibration(branchOneCalibrations, branchDerica.ID) {
		t.Fatalf("own branch calibration missing: calibrations=%+v err=%v", branchOneCalibrations, err)
	}
	branchTwoCalibrations, err := f.service.ListCalibrations(ctx, f.actorA, Scope{BranchIDs: []string{f.branchShop2}}, rice.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if containsCalibration(branchTwoCalibrations, branchDerica.ID) {
		t.Fatal("branch-specific calibration leaked to a different branch scope")
	}
	if _, err = f.service.CreateSellUnit(ctx, f.actorA, scope, rice.ID, CreateSellUnitInput{
		VariantID: bag.ID, MeasureID: "smd_derica", MeasurementMode: "LOCAL_CONTAINER",
		CalibrationID: branchDerica.ID,
	}); err == nil {
		t.Fatal("branch calibration was promoted to a business-wide sell-unit conversion")
	}

	halfPaint, err := f.service.CreateSellUnit(ctx, f.actorA, scope, rice.ID, CreateSellUnitInput{VariantID: bag.ID, MeasureID: "smd_paint", FractionNumerator: intp(1), FractionDenominator: intp(2), DisplayName: "Half Paint", MeasurementMode: "LOCAL_CONTAINER", CalibrationID: ricePaint.ID, PriceMinor: int64p(250000), RetailEnabled: boolp(true)})
	if err != nil {
		t.Fatal(err)
	}
	if halfPaint.ConversionType != ConversionCalibrated || halfPaint.FractionNumerator != 1 || halfPaint.FractionDenominator != 2 {
		t.Fatalf("bad calibrated fraction: %+v", halfPaint)
	}
	kg, err := f.service.CreateSellUnit(ctx, f.actorA, scope, rice.ID, CreateSellUnitInput{VariantID: bag.ID, MeasureID: "smd_kilogram", MeasurementMode: "FIXED", PriceMinor: int64p(120000), RetailEnabled: boolp(true), WholesaleEnabled: boolp(true)})
	if err != nil {
		t.Fatal(err)
	}
	if kg.ConversionType != ConversionExact || kg.ExactBaseQuantity == nil || *kg.ExactBaseQuantity != 1000 {
		t.Fatalf("kg exact conversion missing: %+v", kg)
	}
	if _, err = f.service.CreateSellUnit(ctx, f.actorA, scope, rice.ID, CreateSellUnitInput{VariantID: bag.ID, MeasureID: "smd_paint", FractionNumerator: intp(1), FractionDenominator: intp(0), MeasurementMode: "LOCAL_CONTAINER", CalibrationID: ricePaint.ID}); err == nil {
		t.Fatal("zero fraction denominator accepted")
	}
	derica, err := f.service.CreateSellUnit(ctx, f.actorA, scope, rice.ID, CreateSellUnitInput{
		VariantID: bag.ID, MeasureID: "smd_derica", MeasurementMode: "LOCAL_CONTAINER",
		PriceMinor: int64p(150000), RetailEnabled: boolp(true),
	})
	if err != nil || derica.ConversionType != ConversionNone {
		t.Fatalf("universal Derica conversion should remain NONE: unit=%+v err=%v", derica, err)
	}

	listing, err := f.service.CreateListing(ctx, f.actorA, scope, f.branchShop, CreateListingInput{VariantID: bag.ID, OperatingModel: "WHOLESALER", RetailEnabled: boolp(true), WholesaleEnabled: boolp(true), MinimumOrderQty: int64p(5), OrderMultiple: int64p(5), SellUnitIDs: []string{halfPaint.ID, kg.ID, derica.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.service.GetListing(ctx, f.actorA, Scope{BranchIDs: []string{f.branchShop2}}, listing.ID); err == nil {
		t.Fatal("cross-branch listing read succeeded")
	}
	if _, _, err = f.service.ListListings(ctx, f.actorA, Scope{BranchIDs: []string{f.branchRestaurant}}, f.branchRestaurant, ListingListOptions{}); err == nil {
		t.Fatal("restaurant-only branch read Shop listings")
	}
	retail, err := f.service.ResolvePrice(ctx, f.actorA, scope, listing.ID, halfPaint.ID, 1, false)
	if err != nil || retail.UnitPriceMinor == nil || *retail.UnitPriceMinor != 250000 || retail.Source != SourceSellUnitPrice {
		t.Fatalf("sell-unit retail precedence wrong: %+v err=%v", retail, err)
	}
	listing, err = f.service.UpdateListing(ctx, f.actorA, scope, listing.ID, UpdateListingInput{RetailOverride: int64pp(230000), ExpectedVersion: listing.Version})
	if err != nil {
		t.Fatal(err)
	}
	retail, err = f.service.ResolvePrice(ctx, f.actorA, scope, listing.ID, halfPaint.ID, 1, false)
	if err != nil || retail.UnitPriceMinor == nil || *retail.UnitPriceMinor != 230000 || retail.Source != SourceListingOverride {
		t.Fatalf("listing retail precedence wrong: %+v err=%v", retail, err)
	}
	_, listingUnits, err := f.service.GetListing(ctx, f.actorA, scope, listing.ID)
	if err != nil {
		t.Fatal(err)
	}
	var halfPaintListingUnit string
	var dericaListingUnit ListingSellUnit
	for _, unit := range listingUnits {
		if unit.SellUnitID == halfPaint.ID {
			halfPaintListingUnit = unit.ID
		}
		if unit.SellUnitID == derica.ID {
			dericaListingUnit = unit
		}
	}
	if halfPaintListingUnit == "" {
		t.Fatal("selected sell unit absent from listing")
	}
	if _, err = f.service.SetListingSellUnitPrice(ctx, f.actorA, scope, listing.ID, halfPaintListingUnit, SetListingSellUnitPriceInput{PriceOverride: int64pp(220000), ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	retail, err = f.service.ResolvePrice(ctx, f.actorA, scope, listing.ID, halfPaint.ID, 1, false)
	if err != nil || retail.UnitPriceMinor == nil || *retail.UnitPriceMinor != 220000 || retail.Source != SourceListingSellUnitOverride {
		t.Fatalf("branch sell-unit precedence wrong: %+v err=%v", retail, err)
	}
	if _, err = f.service.SetListingSellUnitPrice(ctx, f.actorA, scope, listing.ID, dericaListingUnit.ID,
		SetListingSellUnitPriceInput{CalibrationID: stringPtrPtr(branchTwoDerica.ID), ExpectedVersion: dericaListingUnit.Version}); err == nil {
		t.Fatal("calibration from another branch attached to this listing")
	}
	calibratedDerica, err := f.service.SetListingSellUnitPrice(ctx, f.actorA, scope, listing.ID, dericaListingUnit.ID,
		SetListingSellUnitPriceInput{CalibrationID: stringPtrPtr(branchDerica.ID), ExpectedVersion: dericaListingUnit.Version})
	if err != nil || calibratedDerica.CalibrationID != branchDerica.ID {
		t.Fatalf("branch calibration assignment failed: unit=%+v err=%v", calibratedDerica, err)
	}
	if _, err = f.service.ArchiveCalibration(ctx, f.actorA, scope, rice.ID, branchDerica.ID, branchDerica.Version); err == nil {
		t.Fatal("calibration archived while a listing sell unit still referenced it")
	}
	disabledDerica, err := f.service.SetListingSellUnitPrice(ctx, f.actorA, scope, listing.ID, dericaListingUnit.ID,
		SetListingSellUnitPriceInput{Enabled: boolp(false), ExpectedVersion: calibratedDerica.Version})
	if err != nil || disabledDerica.Enabled {
		t.Fatalf("disable branch sell unit: unit=%+v err=%v", disabledDerica, err)
	}
	disabledPrice, err := f.service.ResolvePrice(ctx, f.actorA, scope, listing.ID, derica.ID, 5, false)
	if err != nil || disabledPrice.UnitPriceMinor != nil || disabledPrice.Reason == "" {
		t.Fatalf("disabled sell unit still resolved a price: price=%+v err=%v", disabledPrice, err)
	}
	reenabledDerica, err := f.service.SetListingSellUnitPrice(ctx, f.actorA, scope, listing.ID, dericaListingUnit.ID,
		SetListingSellUnitPriceInput{Enabled: boolp(true), ExpectedVersion: disabledDerica.Version})
	if err != nil || !reenabledDerica.Enabled || reenabledDerica.CalibrationID != branchDerica.ID {
		t.Fatalf("re-enable branch sell unit lost calibration: unit=%+v err=%v", reenabledDerica, err)
	}
	first, err := f.service.CreateQuantityBreak(ctx, f.actorA, scope, listing.ID, CreateQuantityBreakInput{SellUnitID: kg.ID, MinQuantity: 1, MaxQuantity: int64p(4), UnitPriceMinor: 110000})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.CreateQuantityBreak(ctx, f.actorA, scope, listing.ID, CreateQuantityBreakInput{SellUnitID: kg.ID, MinQuantity: 5, UnitPriceMinor: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.CreateQuantityBreak(ctx, f.actorA, scope, listing.ID, CreateQuantityBreakInput{SellUnitID: kg.ID, MinQuantity: 4, MaxQuantity: int64p(8), UnitPriceMinor: 90000}); err == nil {
		t.Fatal("overlapping quantity band accepted")
	}
	resolved, err := f.service.ResolvePrice(ctx, f.actorA, scope, listing.ID, kg.ID, 6, true)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.UnitPriceMinor == nil || *resolved.UnitPriceMinor != 100000 || resolved.Source != SourceQuantityBreak {
		t.Fatalf("quantity tier precedence wrong: %+v", resolved)
	}
	if resolved.Satisfied {
		t.Fatal("quantity 6 incorrectly satisfied MOQ/order-multiple 5")
	}
	resolved, err = f.service.ResolvePrice(ctx, f.actorA, scope, listing.ID, kg.ID, 10, true)
	if err != nil || !resolved.Satisfied {
		t.Fatalf("quantity 10 did not satisfy MOQ/order multiple: %+v err=%v", resolved, err)
	}
	if first.Version != 1 {
		t.Fatalf("initial quantity break version=%d", first.Version)
	}
	updatedRice, err := f.service.UpdateProduct(ctx, f.actorA, scope, rice.ID, UpdateProductInput{Description: stringPtrOrNil("A versioned product"), ExpectedVersion: rice.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.UpdateProduct(ctx, f.actorA, scope, rice.ID, UpdateProductInput{Description: stringPtrOrNil("Stale overwrite"), ExpectedVersion: rice.Version}); err == nil {
		t.Fatal("stale product version accepted")
	}
	if updatedRice.Version != rice.Version+1 {
		t.Fatalf("product version=%d want=%d", updatedRice.Version, rice.Version+1)
	}

	freshCategory, err := f.service.CreateCategory(ctx, f.actorA, scope, CreateCategoryInput{Name: "Fresh Produce"})
	if err != nil {
		t.Fatal(err)
	}
	tomato, err := f.service.CreateProduct(ctx, f.actorA, scope, CreateProductInput{CategoryID: freshCategory.ID, Name: "Fresh Tomatoes", ProductClass: "FRESH_PRODUCE", OperatingModels: []string{"MARKET_TRADER"}})
	if err != nil {
		t.Fatal(err)
	}
	attribute, err := f.service.ConfigureFreshAttribute(ctx, f.actorA, scope, tomato.ID, ConfigureFreshAttributeInput{AttributeType: "RIPENESS", Required: boolp(true), Options: []FreshOptionInput{{Value: "GREEN", DisplayName: "Green"}, {Value: "RIPE", DisplayName: "Ripe", IsDefault: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(attribute.Options) != 2 {
		t.Fatalf("fresh options=%d want=2", len(attribute.Options))
	}
	if _, err = f.service.ConfigureFreshAttribute(ctx, f.actorA, scope, rice.ID, ConfigureFreshAttributeInput{AttributeType: "RIPENESS", Options: []FreshOptionInput{{Value: "RIPE"}}}); err == nil {
		t.Fatal("ripeness attached to dry rice")
	}
	if _, err = f.service.GetVariant(ctx, f.actorB, scope, bag.ID); err == nil {
		t.Fatal("cross-tenant variant read succeeded")
	}
	if foreign, err := f.service.ListSellUnits(ctx, f.actorB, scope, rice.ID, "", true); err != nil || len(foreign) != 0 {
		t.Fatalf("cross-tenant sell units leaked: values=%+v err=%v", foreign, err)
	}
	if foreign, err := f.service.ListIdentifiers(ctx, f.actorB, scope, bag.ID, true); err != nil || len(foreign) != 0 {
		t.Fatalf("cross-tenant identifiers leaked: values=%+v err=%v", foreign, err)
	}
	if foreign, err := f.service.ListPackageRelations(ctx, f.actorB, scope, rice.ID, true); err != nil || len(foreign) != 0 {
		t.Fatalf("cross-tenant packages leaked: values=%+v err=%v", foreign, err)
	}
	if foreign, err := f.service.ListCalibrations(ctx, f.actorB, scope, rice.ID, true); err != nil || len(foreign) != 0 {
		t.Fatalf("cross-tenant calibrations leaked: values=%+v err=%v", foreign, err)
	}
	if foreign, err := f.service.ListFreshAttributes(ctx, f.actorB, scope, tomato.ID, true); err != nil || len(foreign) != 0 {
		t.Fatalf("cross-tenant fresh attributes leaked: values=%+v err=%v", foreign, err)
	}
	if _, _, err = f.service.GetListing(ctx, f.actorB, scope, listing.ID); err == nil {
		t.Fatal("cross-tenant listing read succeeded")
	}
	if _, _, err = f.service.GetOperatingModels(ctx, f.actorB, scope, f.branchShop); err == nil {
		t.Fatal("cross-tenant operating-model read succeeded")
	}

	// Same business and same visible name in Food remains a different table and
	// therefore a different authority. Shop reads cannot return the Food row.
	menu, section, foodItem := "s12a_food_menu_"+uuid.NewString(), "s12a_food_section_"+uuid.NewString(), "s12a_food_item_"+uuid.NewString()
	if _, err = f.pool.Exec(ctx, `INSERT INTO food_menus(id,business_id,branch_id,capability_type,name,status) VALUES($1,$2,$3,'RESTAURANT','Restaurant','ACTIVE')`, menu, f.actorA.VendorID, f.branchShop); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO food_menu_sections(id,business_id,menu_id,name,position) VALUES($1,$2,$3,'Meals',0)`, section, f.actorA.VendorID, menu); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO food_items(id,business_id,branch_id,menu_id,section_id,name,base_price_minor,status,position) VALUES($1,$2,$3,$4,$5,'Local Rice',500000,'ACTIVE',0)`, foodItem, f.actorA.VendorID, f.branchShop, menu, section); err != nil {
		t.Fatal(err)
	}
	products, _, err := f.service.ListProducts(ctx, f.actorA, scope, ProductListOptions{Search: "Local Rice", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0].ID != rice.ID {
		t.Fatalf("Shop query crossed into Food: %+v", products)
	}
	if !contains(products[0].OperatingModels, ModelFoodstuffDealer) || !contains(products[0].OperatingModels, ModelWholesaler) {
		t.Fatalf("batched product hydration lost operating models: %+v", products[0].OperatingModels)
	}
	riceVariants, err := f.service.ListVariants(ctx, f.actorA, scope, rice.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	var bagHydrated bool
	for _, variant := range riceVariants {
		if variant.ID == bag.ID {
			bagHydrated = contains(variant.HandlingFlags, "KEEP_DRY")
		}
	}
	if !bagHydrated {
		t.Fatalf("batched variant hydration lost handling flags: %+v", riceVariants)
	}
	var foodCount int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM food_items WHERE business_id=$1 AND name='Local Rice'`, f.actorA.VendorID).Scan(&foodCount); err != nil || foodCount != 1 {
		t.Fatalf("Food authority fixture invalid: count=%d err=%v", foodCount, err)
	}

	history, _, err := f.service.ListPriceHistory(ctx, f.actorA, scope, PriceHistoryOptions{ProductRef: rice.ID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 6 {
		t.Fatalf("append-only price evidence too short: %d", len(history))
	}
	branchTwoHistory, _, err := f.service.ListPriceHistory(ctx, f.actorA, Scope{BranchIDs: []string{f.branchShop2}}, PriceHistoryOptions{ProductRef: rice.ID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range branchTwoHistory {
		if version.BranchID != "" {
			t.Fatalf("branch price evidence leaked across scope: %+v", version)
		}
	}
	revision, _, err := f.service.CatalogueRevision(ctx, f.actorA, scope, f.branchShop)
	if err != nil || revision < 1 {
		t.Fatalf("catalogue revision=%d err=%v", revision, err)
	}
	if _, _, err = f.service.CatalogueRevision(ctx, f.actorA, Scope{BranchIDs: []string{f.branchRestaurant}}, f.branchRestaurant); err == nil {
		t.Fatal("restaurant-only branch read a Shop catalogue revision")
	}
	var auditRows, outboxRows int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_audit_log WHERE vendor_id=$1 AND action LIKE 'shop.%'`, f.actorA.VendorID).Scan(&auditRows); err != nil || auditRows < 20 {
		t.Fatalf("transactional Shop audit count=%d err=%v", auditRows, err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE vendor_id=$1 AND event_type LIKE 'shop.%'`, f.actorA.VendorID).Scan(&outboxRows); err != nil || outboxRows < 20 {
		t.Fatalf("transactional Shop outbox count=%d err=%v", outboxRows, err)
	}
}

func suffixTail(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[len(value)-8:]
}

func containsCalibration(values []MeasureCalibration, id string) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}
