package food

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"
)

func TestSection9BCertificationPureRules(t *testing.T) {
	for _, label := range []string{ScheduleBreakfast, ScheduleLunch, ScheduleDinner, ScheduleSeasonal, ScheduleCustom} {
		if _, err := validateScheduleFields(label, []int{1}, "09:00", "11:00", nil, nil); err != nil {
			t.Fatalf("label %s rejected: %v", label, err)
		}
	}
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	spring := Schedule{DaysOfWeek: []int{7}, StartLocal: "01:00", EndLocal: "03:00"}
	if !scheduleMatches(spring, time.Date(2024, 3, 10, 6, 30, 0, 0, time.UTC), location) {
		t.Fatal("DST spring-forward window should match before the transition")
	}
	if scheduleMatches(spring, time.Date(2024, 3, 10, 7, 30, 0, 0, time.UTC), location) {
		t.Fatal("DST spring-forward window should close after 03:00 local")
	}
	overnight := Schedule{DaysOfWeek: []int{5}, StartLocal: "22:00", EndLocal: "02:00"}
	if !scheduleMatches(overnight, time.Date(2024, 1, 6, 6, 0, 0, 0, time.UTC), location) {
		t.Fatal("overnight Friday window should match Saturday small hours")
	}
	if scheduleMatches(overnight, time.Date(2024, 1, 7, 6, 0, 0, 0, time.UTC), location) {
		t.Fatal("overnight window should not match the following night")
	}
	availability := ItemAvailability{Status: OperationalSoldOut, EffectiveUntil: timePtr(time.Unix(100, 0))}
	if availability.Effective(time.Unix(101, 0)) != OperationalAvailable || !availability.Expired(time.Unix(101, 0)) {
		t.Fatal("expired temporary availability must restore to AVAILABLE")
	}
	if _, err = addMinor(math.MaxInt64, 1); err == nil {
		t.Fatal("minor-unit overflow was not rejected")
	}
	if validatePrepRange(30, 10) == nil || validatePrepRange(0, MaxPrepMinutes) != nil {
		t.Fatal("prep range validation is incorrect")
	}
}

func TestSection9BCertificationIAMSplit(t *testing.T) {
	scopes := scopesByPermission{
		"owner":   {PermPricingManage: {Global: true}, PermOperationsManage: {Global: true}, PermCatalogueRead: {Global: true}},
		"branch":  {PermPricingManage: {BranchIDs: []string{"brn_a"}}, PermOperationsManage: {BranchIDs: []string{"brn_a"}}},
		"kitchen": {PermOperationsManage: {BranchIDs: []string{"brn_a"}}, PermCatalogueRead: {BranchIDs: []string{"brn_a"}}},
		"analyst": {PermCatalogueRead: {Global: true}},
	}
	service := NewService(Options{Scopes: scopes})
	if err := service.requirePricingAuthority(context.Background(), Actor{StaffID: "branch", VendorID: "vendor"}, "brn_b"); err == nil {
		t.Fatal("branch-scoped pricing authority crossed its branch boundary")
	}
	if err := service.requirePricingAuthority(context.Background(), Actor{StaffID: "kitchen", VendorID: "vendor"}, "brn_a"); err == nil {
		t.Fatal("kitchen role received unrestricted pricing authority")
	}
	if _, err := service.Authorize(context.Background(), Actor{StaffID: "kitchen", VendorID: "vendor"}, PermOperationsManage); err != nil {
		t.Fatalf("kitchen operations permission denied: %v", err)
	}
	if _, err := service.Authorize(context.Background(), Actor{StaffID: "analyst", VendorID: "vendor"}, PermPricingManage); err == nil {
		t.Fatal("analyst received pricing authority")
	}
}

func timePtr(value time.Time) *time.Time { return &value }

func TestSection9BCertificationDatabaseFlow(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section, err := env.service.CreateSection(ctx, actor, globalScope(), menu.ID, CreateSectionInput{Name: "Operations"})
	if err != nil {
		t.Fatal(err)
	}
	item, err := env.service.CreateItem(ctx, actor, globalScope(), menu.ID, section.ID, CreateItemInput{Name: "Test item", BasePriceMinor: 1000})
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := env.service.CreateSchedule(ctx, actor, globalScope(), menu.ID, CreateScheduleInput{
		Label: ScheduleBreakfast, DaysOfWeek: []int{1, 2, 3, 4, 5, 6, 7}, StartLocalTime: "00:00", EndLocalTime: "23:59",
	})
	if err != nil || schedule.Overnight() {
		t.Fatalf("schedule create: %+v %v", schedule, err)
	}
	if _, err = env.service.ArchiveSchedule(ctx, actor, globalScope(), menu.ID, schedule.ID, schedule.Version); err != nil {
		t.Fatal(err)
	}
	decision, err := env.service.evaluateSchedules(ctx, env.pool, menu, time.Now().UTC())
	if err != nil || !decision.Allowed {
		t.Fatalf("archived-only schedules should leave eligibility unrestricted: %+v %v", decision, err)
	}
	if _, err = env.service.SetItemAvailability(ctx, actor, globalScope(), item.ID, SetAvailabilityInput{Status: OperationalSoldOut, ExpectedVersion: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetPrepProfile(ctx, actor, globalScope(), item.ID, SetPrepProfileInput{PrepMinMinutes: 10, PrepMaxMinutes: 20, ExpectedVersion: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetLogisticsProfile(ctx, actor, globalScope(), item.ID, SetLogisticsProfileInput{
		TemperatureClass: "HOT", PackageSizeClass: "MEDIUM", ApproxWeightGrams: intPtr(500), VehicleHints: []string{"CAR"}, ExpectedVersion: 0,
	}); err != nil {
		t.Fatal(err)
	}
	override, err := env.service.SetBranchOverride(ctx, actor, globalScope(), item.ID, SetBranchOverrideInput{BasePriceOverrideMinor: int64Ptr(900), ExpectedVersion: 0}, nil)
	if err != nil || override.BasePriceOverrideMinor == nil || *override.BasePriceOverrideMinor != 900 {
		t.Fatalf("branch override: %+v %v", override, err)
	}
	resolved, err := env.service.ResolvePrice(ctx, actor, globalScope(), item.ID, ResolvePriceInput{})
	if err != nil || resolved.BasePriceMinor != 900 || resolved.BaseSource != BaseSourceBranch || resolved.ItemPriceVersionRef == "" || resolved.BranchOverridePriceVersionRef == "" || resolved.CatalogueRevision == 0 {
		t.Fatalf("effective price: %+v %v", resolved, err)
	}
	variant, err := env.service.CreateVariant(ctx, actor, globalScope(), item.ID, CreateVariantInput{Name: "Large", PriceOverrideMinor: int64Ptr(1200)})
	if err != nil {
		t.Fatal(err)
	}
	variantPrice, err := env.service.ResolvePrice(ctx, actor, globalScope(), item.ID, ResolvePriceInput{VariantRef: variant.ID})
	if err != nil || variantPrice.BaseSource != BaseSourceVariant || variantPrice.BasePriceMinor != 1200 || variantPrice.VariantPriceVersionRef == "" {
		t.Fatalf("variant precedence: %+v %v", variantPrice, err)
	}
	variant, err = env.service.UpdateVariant(ctx, actor, globalScope(), item.ID, variant.ID, UpdateVariantInput{ClearPriceOverride: true, ExpectedVersion: variant.Version})
	if err != nil {
		t.Fatal(err)
	}
	variantPrice, err = env.service.ResolvePrice(ctx, actor, globalScope(), item.ID, ResolvePriceInput{VariantRef: variant.ID})
	if err != nil || variantPrice.BaseSource != BaseSourceBranch || variantPrice.BasePriceMinor != 900 {
		t.Fatalf("variant clear-to-inherit: %+v %v", variantPrice, err)
	}
	group, err := env.service.CreateModifierGroup(ctx, actor, globalScope(), CreateModifierGroupInput{Name: "Extras", MinSelect: 1, MaxSelect: 1})
	if err != nil {
		t.Fatal(err)
	}
	modifier, err := env.service.CreateModifier(ctx, actor, globalScope(), group.ID, CreateModifierInput{Name: "Cheese", PriceDeltaMinor: 100})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.AttachModifierGroup(ctx, actor, globalScope(), item.ID, group.ID); err != nil {
		t.Fatal(err)
	}
	withModifier, err := env.service.ResolvePrice(ctx, actor, globalScope(), item.ID, ResolvePriceInput{ModifierRefs: []string{modifier.ID}})
	if err != nil || withModifier.EffectiveUnitPriceMinor != 1000 || len(withModifier.Modifiers) != 1 {
		t.Fatalf("modifier pricing: %+v %v", withModifier, err)
	}
	if _, err = env.service.ResolvePrice(ctx, actor, globalScope(), item.ID, ResolvePriceInput{}); err == nil {
		t.Fatal("required modifier selection was accepted")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, amount := range []int64{901, 902} {
		wg.Add(1)
		go func(value int64) {
			defer wg.Done()
			_, callErr := env.service.SetBranchOverride(ctx, actor, globalScope(), item.ID,
				SetBranchOverrideInput{BasePriceOverrideMinor: &value, ExpectedVersion: override.Version}, nil)
			results <- callErr
		}(amount)
	}
	wg.Wait()
	close(results)
	winners := 0
	for callErr := range results {
		if callErr == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent branch override edits had %d winners, want exactly one", winners)
	}
	history, _, err := env.service.ListItemPriceHistory(ctx, actor, globalScope(), item.ID, PriceHistoryOptions{})
	if err != nil || len(history) < 2 {
		t.Fatalf("price history: %d %v", len(history), err)
	}
	if _, err = env.pool.Exec(ctx, `UPDATE food_price_versions SET amount_minor=amount_minor WHERE id=$1`, history[0].ID); err == nil {
		t.Fatal("immutable price history accepted an update")
	}
	if _, err = env.service.SetAvailabilitySource(ctx, actor, globalScope(), item.ID, SetAvailabilitySourceInput{Mode: SourceInventoryLinked, InventoryRecipeRef: stringPtr("recipe-1"), ExpectedVersion: 0}); err != nil {
		t.Fatal(err)
	}
	source, err := env.service.GetAvailabilitySource(ctx, actor, globalScope(), item.ID)
	if err != nil || source.Mode != SourceInventoryLinked || InventoryAvailabilitySupport != "NOT_PRESENT" {
		t.Fatalf("inventory seam: %+v %v", source, err)
	}
	if _, err = env.service.SetItemAvailability(ctx, actor, globalScope(), item.ID, SetAvailabilityInput{Status: OperationalAvailable, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	menu, err = env.service.SetMenuStatus(ctx, actor, globalScope(), menu.ID, StatusActive, menu.Version)
	if err != nil {
		t.Fatal(err)
	}
	item, err = env.service.UpdateItem(ctx, actor, globalScope(), item.ID, UpdateItemInput{Status: stringPtr(StatusActive), ExpectedVersion: item.Version})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := env.service.ResolveOperationalProfile(ctx, actor, globalScope(), item.ID, time.Now().UTC())
	if err != nil || profile.Sellability.ReasonCode != ReasonInventorySourceGone {
		t.Fatalf("inventory-linked availability must fail closed: %+v %v", profile, err)
	}
}

func intPtr(value int) *int { return &value }
