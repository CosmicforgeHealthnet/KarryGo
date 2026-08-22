package main

// Section 17C adapter integration tests.
//
// These run against real Postgres because the thing under test IS the SQL: the
// sell-unit conversion, the tenant scoping and the handling-flag mapping are
// all statements, and an in-memory fake would prove none of them.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
)

type s17cEnv struct {
	t    *testing.T
	pool *pgxpool.Pool

	suffix string

	businessA string
	businessB string

	branchA string
	branchB string

	categoryA string
	productA  string

	measureID string
}

func newSection17CEnv(t *testing.T) *s17cEnv {
	t.Helper()

	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")

	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping Section 17C adapter integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(pool.Close)

	suffix := uuid.NewString()[:12]

	env := &s17cEnv{
		t:      t,
		pool:   pool,
		suffix: suffix,

		businessA: "s17c_biz_a_" + suffix,
		businessB: "s17c_biz_b_" + suffix,
		branchA:   "s17c_br_a_" + suffix,
		branchB:   "s17c_br_b_" + suffix,
		categoryA: "s17c_cat_a_" + suffix,
		productA:  "s17c_prod_a_" + suffix,
		measureID: "s17c_measure_" + suffix,
	}

	env.seed(ctx)

	return env
}

func (e *s17cEnv) exec(ctx context.Context, sql string, args ...any) {
	e.t.Helper()

	if _, err := e.pool.Exec(ctx, sql, args...); err != nil {
		e.t.Fatalf("seed failed: %v\nsql: %s", err, sql)
	}
}

func (e *s17cEnv) seed(ctx context.Context) {
	e.t.Helper()

	for index, seed := range []struct{ business, branch string }{
		{e.businessA, e.branchA},
		{e.businessB, e.branchB},
	} {
		user := "s17c_user_" + string(rune('a'+index)) + "_" + e.suffix
		staff := "s17c_staff_" + string(rune('a'+index)) + "_" + e.suffix

		e.exec(ctx,
			`INSERT INTO vendor_users(id,email) VALUES($1,$2)`,
			user, user+"@example.test")

		e.exec(ctx,
			`INSERT INTO vendor_staff(id,vendor_id,user_id) VALUES($1,$2,$3)`,
			staff, seed.business, user)

		e.exec(ctx,
			`INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone)
			 VALUES($1,'Section 17C test','ACTIVE',$2,'NGN','Africa/Lagos')`,
			seed.business, staff)

		e.exec(ctx,
			`INSERT INTO vendor_branches(
				id,business_id,display_name,address_line1,city,region,
				country_code,latitude,longitude,timezone,status)
			 VALUES($1,$2,$3,'1 Test Street','Lagos','Lagos','NG',6.4550,3.4100,'Africa/Lagos','ACTIVE')`,
			seed.branch, seed.business, "Branch "+seed.branch)
	}

	e.exec(ctx,
		`INSERT INTO shop_categories(id,business_id,name,slug,status)
		 VALUES($1,$2,'Test category',$3,'ACTIVE')`,
		e.categoryA, e.businessA, "s17c-cat-"+e.suffix)

	e.exec(ctx,
		`INSERT INTO shop_products(id,business_id,category_id,name,product_class,status)
		 VALUES($1,$2,$3,'Test product','DRY_BULK','ACTIVE')`,
		e.productA, e.businessA, e.categoryA)

	e.exec(ctx,
		// owner_key is GENERATED ALWAYS AS COALESCE(business_id,'PLATFORM'),
		// so a platform measure is created by leaving business_id NULL.
		`INSERT INTO shop_measure_definitions(
			id,code,display_name,measurement_family,measure_origin,status)
		 VALUES($1,$2,'Kilogram','MASS','STANDARD','ACTIVE')`,
		e.measureID, "S17CKG"+strings.ToUpper(e.suffix[:6]))
}

type s17cVariantOptions struct {
	id string

	netWeightGrams *int64
	netVolumeMl    *int64
	lengthMm       *int
	temperature    string
	consolidation  string
	separation     string
	stackable      bool
	largeLoad      bool

	handlingFlags []string
}

func (e *s17cEnv) seedVariant(
	ctx context.Context,
	options s17cVariantOptions,
) string {
	e.t.Helper()

	variantID := options.id
	if variantID == "" {
		variantID = "s17c_var_" + uuid.NewString()[:8]
	}

	temperature := options.temperature
	if temperature == "" {
		temperature = "AMBIENT"
	}

	e.exec(ctx,
		`INSERT INTO shop_variants(
			id,business_id,product_id,name,net_weight_grams,net_volume_ml,
			length_mm,temperature_zone,consolidation_class,separation_group,
			stackable,large_load,status)
		 VALUES($1,$2,$3,'Test variant',$4,$5,$6,$7,
		        NULLIF($8,''),NULLIF($9,''),$10,$11,'ACTIVE')`,
		variantID, e.businessA, e.productA,
		options.netWeightGrams, options.netVolumeMl, options.lengthMm,
		temperature, options.consolidation, options.separation,
		options.stackable, options.largeLoad)

	for index, flag := range options.handlingFlags {
		e.exec(ctx,
			`INSERT INTO shop_variant_handling_flags(id,business_id,variant_id,handling_flag)
			 VALUES($1,$2,$3,$4)`,
			variantID+"_flag_"+string(rune('a'+index)),
			e.businessA, variantID, flag)
	}

	return variantID
}

func (e *s17cEnv) seedListing(
	ctx context.Context,
	variantID string,
) string {
	e.t.Helper()

	listingID := "s17c_lst_" + uuid.NewString()[:8]

	e.exec(ctx,
		`INSERT INTO shop_branch_listings(
			id,business_id,branch_id,product_id,variant_id,status)
		 VALUES($1,$2,$3,$4,$5,'ACTIVE')`,
		listingID, e.businessA, e.branchA, e.productA, variantID)

	return listingID
}

func (e *s17cEnv) seedSellUnit(
	ctx context.Context,
	variantID, conversionType string,
	baseFamily *string,
	baseQuantity *int64,
	calibrationID *string,
	estimated *int64,
) string {
	e.t.Helper()

	sellUnitID := "s17c_su_" + uuid.NewString()[:8]

	e.exec(ctx,
		`INSERT INTO shop_sell_units(
			id,business_id,product_id,variant_id,measure_definition_id,
			measure_owner_key,display_name,measurement_mode,conversion_type,
			exact_base_family,exact_base_quantity,calibration_id,
			estimated_base_quantity,status)
		 VALUES($1,$2,$3,$4,$5,'PLATFORM','Test unit','FIXED',$6,$7,$8,$9,$10,'ACTIVE')`,
		sellUnitID, e.businessA, e.productA, variantID, e.measureID,
		conversionType, baseFamily, baseQuantity, calibrationID, estimated)

	return sellUnitID
}

func shopLine(id, listingRef, variantRef, sellUnitRef string, quantity int64) fulfilment.LineLogistics {
	return fulfilment.LineLogistics{
		CheckoutLineID: id,
		Domain:         "SHOP",
		ItemRef:        "item_" + id,
		VariantRef:     variantRef,
		ListingRef:     listingRef,
		SellUnitRef:    sellUnitRef,
		Quantity:       quantity,
	}
}

func ptrInt64(v int64) *int64    { return &v }
func ptrInt(v int) *int          { return &v }
func ptrString(v string) *string { return &v }

func TestSection17CLogistics_ShopVariantPhysicalMetadata(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	variantID := env.seedVariant(ctx, s17cVariantOptions{
		netWeightGrams: ptrInt64(2500),
		netVolumeMl:    ptrInt64(1800),
		lengthMm:       ptrInt(450),
		temperature:    "CHILLED",
		consolidation:  "CHILLED",
		separation:     "raw-protein",
		stackable:      true,
		largeLoad:      false,
		handlingFlags:  []string{"FRAGILE", "LEAK_RISK"},
	})

	listingID := env.seedListing(ctx, variantID)

	authority := newSection17CLogisticsAuthority(env.pool)

	profiles, err := authority.ResolveLineProfiles(
		ctx, env.businessA,
		[]fulfilment.LineLogistics{shopLine("l1", listingID, "", "", 2)},
	)
	if err != nil {
		t.Fatal(err)
	}

	profile := profiles["l1"]

	if !profile.Known {
		t.Fatal("expected a known profile")
	}

	if profile.WeightGrams == nil || *profile.WeightGrams != 2500 {
		t.Fatalf("weight = %v, want 2500", profile.WeightGrams)
	}

	if profile.TemperatureClass != "CHILLED" {
		t.Fatalf("temperature = %q, want CHILLED", profile.TemperatureClass)
	}

	if !profile.Fragile || !profile.SpillRisk {
		t.Fatalf("handling flags not mapped: %+v", profile)
	}

	if profile.SeparationGroup != "raw-protein" {
		t.Fatalf("separation = %q", profile.SeparationGroup)
	}

	// End to end: a chilled fragile line must escalate past a motorcycle.
	requirement := fulfilment.Aggregate(
		env.businessA, env.branchA,
		[]fulfilment.LineLogistics{shopLine("l1", listingID, "", "", 2)},
		profiles,
	)

	if requirement.TotalWeightGrams != 5000 {
		t.Fatalf("aggregated weight = %d, want 5000",
			requirement.TotalWeightGrams)
	}

	selection, err := fulfilment.SelectVehicle(
		requirement,
		fulfilment.DefaultCapabilities(),
		map[fulfilment.Service]bool{
			fulfilment.ServiceDispatch: true,
			fulfilment.ServiceHauling:  true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if selection.Class != fulfilment.VehicleVan {
		t.Fatalf("class = %q, want VAN for a chilled fragile load",
			selection.Class)
	}
}

// The sell-unit conversion is the correctness point: a 2 kg portion of a 50 kg
// sack must weigh 2 kg, not 50.
func TestSection17CLogistics_ExactSellUnitReplacesVariantMass(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	variantID := env.seedVariant(ctx, s17cVariantOptions{
		netWeightGrams: ptrInt64(50000),
		netVolumeMl:    ptrInt64(60000),
		stackable:      true,
	})

	listingID := env.seedListing(ctx, variantID)

	sellUnitID := env.seedSellUnit(
		ctx, variantID, "EXACT",
		ptrString("MASS"), ptrInt64(2000), nil, nil,
	)

	authority := newSection17CLogisticsAuthority(env.pool)

	profiles, err := authority.ResolveLineProfiles(
		ctx, env.businessA,
		[]fulfilment.LineLogistics{shopLine("l1", listingID, "", sellUnitID, 3)},
	)
	if err != nil {
		t.Fatal(err)
	}

	profile := profiles["l1"]

	if profile.WeightGrams == nil || *profile.WeightGrams != 2000 {
		t.Fatalf("weight = %v, want the sell unit's 2000g, not the sack's 50000g",
			profile.WeightGrams)
	}

	// The variant's volume describes a whole sack, not this portion of it.
	if profile.VolumeMl != nil {
		t.Fatalf("volume = %v, want nil — the sack's volume is not this portion's",
			profile.VolumeMl)
	}

	requirement := fulfilment.Aggregate(
		env.businessA, env.branchA,
		[]fulfilment.LineLogistics{shopLine("l1", listingID, "", sellUnitID, 3)},
		profiles,
	)

	if requirement.TotalWeightGrams != 6000 {
		t.Fatalf("aggregated weight = %d, want 6000",
			requirement.TotalWeightGrams)
	}

	// 6 kg still fits a motorcycle; using the sack weight would have been
	// 150 kg and wrongly demanded a van.
	selection, err := fulfilment.SelectVehicle(
		requirement, fulfilment.DefaultCapabilities(),
		map[fulfilment.Service]bool{
			fulfilment.ServiceDispatch: true,
			fulfilment.ServiceHauling:  true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if selection.Class != fulfilment.VehicleMotorcycle {
		t.Fatalf("class = %q, want MOTORCYCLE", selection.Class)
	}
}

func TestSection17CLogistics_CountSellUnitScalesVariant(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	variantID := env.seedVariant(ctx, s17cVariantOptions{
		netWeightGrams: ptrInt64(400),
		netVolumeMl:    ptrInt64(500),
		stackable:      true,
	})

	listingID := env.seedListing(ctx, variantID)

	sellUnitID := env.seedSellUnit(
		ctx, variantID, "EXACT",
		ptrString("COUNT"), ptrInt64(12), nil, nil,
	)

	authority := newSection17CLogisticsAuthority(env.pool)

	profiles, err := authority.ResolveLineProfiles(
		ctx, env.businessA,
		[]fulfilment.LineLogistics{shopLine("l1", listingID, "", sellUnitID, 1)},
	)
	if err != nil {
		t.Fatal(err)
	}

	profile := profiles["l1"]

	if profile.WeightGrams == nil || *profile.WeightGrams != 4800 {
		t.Fatalf("weight = %v, want 12 x 400g", profile.WeightGrams)
	}

	if profile.VolumeMl == nil || *profile.VolumeMl != 6000 {
		t.Fatalf("volume = %v, want 12 x 500ml", profile.VolumeMl)
	}
}

// Section 12 forbids treating a calibration as exact, so an approximate
// conversion must never surface as a measured weight.
func TestSection17CLogistics_CalibratedApproximateIsNotMeasured(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	variantID := env.seedVariant(ctx, s17cVariantOptions{
		netWeightGrams: ptrInt64(50000),
		stackable:      true,
	})

	listingID := env.seedListing(ctx, variantID)

	calibrationID := "s17c_cal_" + uuid.NewString()[:8]

	env.exec(ctx,
		`INSERT INTO shop_measure_calibrations(
			id,business_id,product_id,variant_id,measure_definition_id,
			measure_owner_key,approximate_weight_grams,calibration_source,status)
		 VALUES($1,$2,$3,$4,$5,'PLATFORM',2400,'VENDOR_MEASURED','ACTIVE')`,
		calibrationID, env.businessA, env.productA, variantID, env.measureID)

	sellUnitID := env.seedSellUnit(
		ctx, variantID, "CALIBRATED_APPROXIMATE",
		nil, nil, ptrString(calibrationID), ptrInt64(2400),
	)

	authority := newSection17CLogisticsAuthority(env.pool)

	profiles, err := authority.ResolveLineProfiles(
		ctx, env.businessA,
		[]fulfilment.LineLogistics{shopLine("l1", listingID, "", sellUnitID, 1)},
	)
	if err != nil {
		t.Fatal(err)
	}

	profile := profiles["l1"]

	if profile.WeightGrams != nil {
		t.Fatalf("weight = %v, want nil — a calibration is not a measurement",
			profile.WeightGrams)
	}

	if profile.PackageSizeClass == "" {
		t.Fatal("expected the approximation to travel as a size class")
	}

	requirement := fulfilment.Aggregate(
		env.businessA, env.branchA,
		[]fulfilment.LineLogistics{shopLine("l1", listingID, "", sellUnitID, 1)},
		profiles,
	)

	if requirement.WeightEvidence == fulfilment.EvidenceComplete {
		t.Fatal("an approximate conversion must never report COMPLETE weight evidence")
	}

	if requirement.TotalWeightGrams == 0 {
		t.Fatal("the approximation should still contribute a lower-bound weight")
	}
}

// A KEEP_COLD flag may raise AMBIENT to CHILLED but must never demote FROZEN.
func TestSection17CLogistics_HandlingFlagOnlyRaisesTemperature(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	authority := newSection17CLogisticsAuthority(env.pool)

	ambient := env.seedVariant(ctx, s17cVariantOptions{
		temperature:   "AMBIENT",
		stackable:     true,
		handlingFlags: []string{"KEEP_COLD"},
	})

	frozen := env.seedVariant(ctx, s17cVariantOptions{
		temperature:   "FROZEN",
		stackable:     true,
		handlingFlags: []string{"KEEP_COLD"},
	})

	profiles, err := authority.ResolveLineProfiles(
		ctx, env.businessA,
		[]fulfilment.LineLogistics{
			shopLine("ambient", "", ambient, "", 1),
			shopLine("frozen", "", frozen, "", 1),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if profiles["ambient"].TemperatureClass != "CHILLED" {
		t.Fatalf("ambient + KEEP_COLD = %q, want CHILLED",
			profiles["ambient"].TemperatureClass)
	}

	if profiles["frozen"].TemperatureClass != "FROZEN" {
		t.Fatalf("frozen + KEEP_COLD = %q, want FROZEN — a flag must not demote",
			profiles["frozen"].TemperatureClass)
	}
}

// Vendor A's catalogue must not resolve under Vendor B's tenant.
func TestSection17CLogistics_TenantIsolation(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	variantID := env.seedVariant(ctx, s17cVariantOptions{
		netWeightGrams: ptrInt64(1000),
		stackable:      true,
	})

	listingID := env.seedListing(ctx, variantID)

	authority := newSection17CLogisticsAuthority(env.pool)

	profiles, err := authority.ResolveLineProfiles(
		ctx, env.businessB,
		[]fulfilment.LineLogistics{
			shopLine("l1", listingID, variantID, "", 1),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if profiles["l1"].Known {
		t.Fatal("vendor B resolved vendor A's variant")
	}
}

func TestSection17CLogistics_FoodLogisticsProfile(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	menuID := "s17c_menu_" + env.suffix
	sectionID := "s17c_sec_" + env.suffix
	itemID := "s17c_item_" + env.suffix

	env.exec(ctx,
		`INSERT INTO food_menus(id,business_id,branch_id,capability_type,name,status)
		 VALUES($1,$2,$3,'RESTAURANT','Test menu','ACTIVE')`,
		menuID, env.businessA, env.branchA)

	env.exec(ctx,
		`INSERT INTO food_menu_sections(id,business_id,menu_id,name,position,status)
		 VALUES($1,$2,$3,'Mains',0,'ACTIVE')`,
		sectionID, env.businessA, menuID)

	env.exec(ctx,
		`INSERT INTO food_items(id,business_id,branch_id,menu_id,section_id,name,base_price_minor,status,position)
		 VALUES($1,$2,$3,$4,$5,'Jollof rice',250000,'ACTIVE',0)`,
		itemID, env.businessA, env.branchA, menuID, sectionID)

	env.exec(ctx,
		`INSERT INTO food_item_logistics_profiles(
			item_id,business_id,temperature_class,package_size_class,
			spill_risk,fragile,approx_weight_grams,vehicle_hints)
		 VALUES($1,$2,'HOT','MEDIUM',true,false,900,ARRAY['MOTORCYCLE']::text[])`,
		itemID, env.businessA)

	authority := newSection17CLogisticsAuthority(env.pool)

	line := fulfilment.LineLogistics{
		CheckoutLineID: "f1",
		Domain:         "FOOD",
		ItemRef:        itemID,
		Quantity:       2,
	}

	profiles, err := authority.ResolveLineProfiles(
		ctx, env.businessA, []fulfilment.LineLogistics{line},
	)
	if err != nil {
		t.Fatal(err)
	}

	profile := profiles["f1"]

	if !profile.Known || profile.TemperatureClass != "HOT" {
		t.Fatalf("unexpected food profile: %+v", profile)
	}

	if profile.WeightGrams == nil || *profile.WeightGrams != 900 {
		t.Fatalf("weight = %v, want 900", profile.WeightGrams)
	}

	if len(profile.VehicleHints) != 1 || profile.VehicleHints[0] != "MOTORCYCLE" {
		t.Fatalf("hints = %v", profile.VehicleHints)
	}

	requirement := fulfilment.Aggregate(
		env.businessA, env.branchA,
		[]fulfilment.LineLogistics{line}, profiles,
	)

	if requirement.Urgency != fulfilment.UrgencyImmediate {
		t.Fatalf("urgency = %q, want IMMEDIATE for hot food",
			requirement.Urgency)
	}

	if !requirement.HasCapability(fulfilment.CapabilityHotHold) {
		t.Fatalf("missing hot-hold capability: %v", requirement.Capabilities)
	}
}

// A line whose item the vendor never described must be reported as missing
// evidence, not silently treated as weightless.
func TestSection17CLogistics_UnknownItemIsReportedNotDefaulted(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	authority := newSection17CLogisticsAuthority(env.pool)

	line := fulfilment.LineLogistics{
		CheckoutLineID: "ghost",
		Domain:         "FOOD",
		ItemRef:        "s17c_missing_" + env.suffix,
		Quantity:       4,
	}

	profiles, err := authority.ResolveLineProfiles(
		ctx, env.businessA, []fulfilment.LineLogistics{line},
	)
	if err != nil {
		t.Fatal(err)
	}

	if profiles["ghost"].Known {
		t.Fatal("a missing item must not resolve to a known profile")
	}

	requirement := fulfilment.Aggregate(
		env.businessA, env.branchA,
		[]fulfilment.LineLogistics{line}, profiles,
	)

	if requirement.PackageCount != 4 {
		t.Fatalf("package count = %d, want 4", requirement.PackageCount)
	}

	if requirement.WeightEvidence != fulfilment.EvidenceAbsent {
		t.Fatalf("weight evidence = %q, want ABSENT",
			requirement.WeightEvidence)
	}
}

func TestSection17CBranchAuthority(t *testing.T) {
	env := newSection17CEnv(t)
	ctx := context.Background()

	authority := newSection17CBranchAuthority(env.pool)

	// No service area configured yet: found, but with no stated reach.
	branch, found, err := authority.ResolveBranch(ctx, env.businessA, env.branchA)
	if err != nil {
		t.Fatal(err)
	}

	if !found {
		t.Fatal("expected the branch to resolve")
	}

	if branch.HasServiceArea {
		t.Fatal("a branch with no service area must not claim one")
	}

	if !branch.Active() {
		t.Fatalf("status = %q, want ACTIVE", branch.Status)
	}

	env.exec(ctx,
		`INSERT INTO vendor_branch_service_areas(
			branch_id,business_id,mode,center_latitude,center_longitude,radius_meters)
		 VALUES($1,$2,'RADIUS',6.4550,3.4100,9000)`,
		env.branchA, env.businessA)

	branch, _, err = authority.ResolveBranch(ctx, env.businessA, env.branchA)
	if err != nil {
		t.Fatal(err)
	}

	if !branch.HasServiceArea || branch.AreaRadiusMeters != 9000 {
		t.Fatalf("service area not resolved: %+v", branch)
	}

	// Vendor B must not be able to use Vendor A's branch as a pickup point.
	if _, foundCross, err := authority.ResolveBranch(
		ctx, env.businessB, env.branchA,
	); err != nil {
		t.Fatal(err)
	} else if foundCross {
		t.Fatal("vendor B resolved vendor A's branch as a pickup point")
	}
}
