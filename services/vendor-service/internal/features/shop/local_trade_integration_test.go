package shop

import (
	"context"
	"testing"
	"time"
)

// TestSection12ALocalTradeAndFreshMatrix exercises the concrete Nigerian
// examples in the Section 12A contract. It intentionally proves that local
// containers remain NONE or CALIBRATED_APPROXIMATE while standard units are
// exact; no test assigns a universal kg value to Paint, Mudu or Derica.
func TestSection12ALocalTradeAndFreshMatrix(t *testing.T) {
	f := section12Fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scope := Scope{Global: true}

	category, err := f.service.CreateCategory(ctx, f.actorA, scope, CreateCategoryInput{Name: "Local Trade Matrix"})
	if err != nil {
		t.Fatal(err)
	}
	type item struct {
		product Product
		variant Variant
	}
	makeItem := func(name, class, packageLevel, temperature string) item {
		t.Helper()
		product, createErr := f.service.CreateProduct(ctx, f.actorA, scope, CreateProductInput{
			CategoryID: category.ID, Name: name, ProductClass: class,
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		variant, createErr := f.service.CreateVariant(ctx, f.actorA, scope, product.ID, CreateVariantInput{
			Name: name, PackageLevel: packageLevel, TemperatureZone: temperature,
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return item{product: product, variant: variant}
	}
	addUnit := func(value item, measure, mode string, numerator, denominator int, calibrationID string, price *int64) SellUnit {
		t.Helper()
		input := CreateSellUnitInput{VariantID: value.variant.ID, MeasureID: measure,
			MeasurementMode: mode, CalibrationID: calibrationID, PriceMinor: price}
		if numerator != 0 {
			input.FractionNumerator = intp(numerator)
		}
		if denominator != 0 {
			input.FractionDenominator = intp(denominator)
		}
		unit, createErr := f.service.CreateSellUnit(ctx, f.actorA, scope, value.product.ID, input)
		if createErr != nil {
			t.Fatalf("create %s unit for %s: %v", measure, value.product.Name, createErr)
		}
		return unit
	}
	calibrate := func(value item, measure string, grams int64) MeasureCalibration {
		t.Helper()
		calibration, createErr := f.service.CreateCalibration(ctx, f.actorA, scope, value.product.ID,
			CreateCalibrationInput{VariantID: value.variant.ID, MeasureID: measure,
				WeightGrams: int64p(grams), ToleranceBps: 300, Source: CalibrationVendor})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return calibration
	}

	garri := makeItem("Garri", "LOCAL_FOODSTUFF", "BAG", "AMBIENT")
	garriPaint := calibrate(garri, "smd_paint", 3900)
	for _, unit := range []SellUnit{
		addUnit(garri, "smd_mudu", ModeLocalContainer, 1, 1, "", nil),
		addUnit(garri, "smd_paint", ModeLocalContainer, 1, 1, garriPaint.ID, nil),
		addUnit(garri, "smd_bag", ModePackageCount, 1, 1, "", nil),
	} {
		if unit.MeasureID != "smd_paint" && unit.ConversionType != ConversionNone {
			t.Fatalf("local Garri unit became exact: %+v", unit)
		}
	}

	eggs := makeItem("Eggs", "PACKAGED", "TRAY", "AMBIENT")
	piece := addUnit(eggs, "smd_piece", ModeFixed, 1, 1, "", nil)
	sixPack := addUnit(eggs, "smd_piece", ModeFixed, 6, 1, "", nil)
	dozen := addUnit(eggs, "smd_dozen", ModeFixed, 1, 1, "", nil)
	tray := addUnit(eggs, "smd_tray", ModePackageCount, 1, 1, "", nil)
	if piece.ConversionType != ConversionExact || sixPack.ExactBaseQuantity == nil || *sixPack.ExactBaseQuantity != 6 ||
		dozen.ExactBaseQuantity == nil || *dozen.ExactBaseQuantity != 12 || tray.ConversionType != ConversionNone {
		t.Fatalf("egg measurement matrix invalid: piece=%+v six=%+v dozen=%+v tray=%+v", piece, sixPack, dozen, tray)
	}

	yam := makeItem("Yam Tubers", "FRESH_PRODUCE", "EACH", "FRESH")
	addUnit(yam, "smd_tuber", ModeVariableCount, 1, 1, "", nil)
	addUnit(yam, "smd_piece", ModeVariableCount, 1, 1, "", nil)
	addUnit(yam, "smd_kilogram", ModeVariableWeight, 1, 1, "", nil)

	tomatoes := makeItem("Fresh Tomatoes Matrix", "FRESH_PRODUCE", "OTHER_CONTROLLED", "FRESH")
	basketCalibration := calibrate(tomatoes, "smd_basket", 28000)
	addUnit(tomatoes, "smd_kilogram", ModeVariableWeight, 1, 1, "", nil)
	addUnit(tomatoes, "smd_basket", ModeLocalContainer, 1, 1, basketCalibration.ID, nil)
	halfBasket := addUnit(tomatoes, "smd_basket", ModeLocalContainer, 1, 2, basketCalibration.ID, nil)
	portionPrice := int64(50000)
	pricePortion := addUnit(tomatoes, "smd_price_portion", ModePricePortion, 1, 1, "", &portionPrice)
	if halfBasket.FractionNumerator != 1 || halfBasket.FractionDenominator != 2 ||
		halfBasket.ConversionType != ConversionCalibrated || pricePortion.ConversionType != ConversionNone {
		t.Fatalf("tomato local units invalid: half=%+v portion=%+v", halfBasket, pricePortion)
	}

	plantain := makeItem("Plantain", "FRESH_PRODUCE", "BUNDLE", "FRESH")
	addUnit(plantain, "smd_bunch", ModeVariableCount, 1, 1, "", nil)
	addUnit(plantain, "smd_stick", ModeVariableCount, 1, 1, "", nil)

	oil := makeItem("Palm Oil", "LOCAL_FOODSTUFF", "OTHER_CONTROLLED", "AMBIENT")
	addUnit(oil, "smd_litre", ModeFixed, 1, 1, "", nil)
	addUnit(oil, "smd_jerrycan", ModeLocalContainer, 1, 1, "", nil)

	fish := makeItem("Frozen Fish", "FISH", "CARTON", "FROZEN")
	cartonCalibration := calibrate(fish, "smd_carton", 20000)
	addUnit(fish, "smd_kilogram", ModeVariableWeight, 1, 1, "", nil)
	addUnit(fish, "smd_carton", ModePackageCount, 1, 2, cartonCalibration.ID, nil)
	addUnit(fish, "smd_carton", ModePackageCount, 1, 1, cartonCalibration.ID, nil)

	meat := makeItem("Fresh Beef", "MEAT", "EACH", "FRESH")
	leafy := makeItem("Leafy Vegetable", "FRESH_PRODUCE", "BUNDLE", "FRESH")
	addUnit(leafy, "smd_bunch", ModeVariableCount, 1, 1, "", nil)

	configure := func(value item, attribute string, options ...string) {
		t.Helper()
		inputs := make([]FreshOptionInput, 0, len(options))
		for _, option := range options {
			inputs = append(inputs, FreshOptionInput{Value: option})
		}
		if _, configureErr := f.service.ConfigureFreshAttribute(ctx, f.actorA, scope, value.product.ID,
			ConfigureFreshAttributeInput{AttributeType: attribute, Options: inputs}); configureErr != nil {
			t.Fatalf("configure %s on %s: %v", attribute, value.product.Name, configureErr)
		}
	}
	configure(tomatoes, AttrRipeness, "GREEN", "RIPE")
	configure(tomatoes, AttrQuality, "STANDARD", "PREMIUM")
	configure(plantain, AttrRipeness, "GREEN", "TURNING", "RIPE")
	configure(fish, AttrCut, "WHOLE", "CUT")
	configure(fish, AttrCleaning, "CLEANED", "UNCLEANED")
	configure(fish, AttrDescaling, "DESCALED", "NOT_DESCALED")
	configure(meat, AttrCut, "WHOLE", "PORTIONED")
	configure(meat, AttrBone, "BONE_IN", "BONELESS")
	configure(yam, AttrSize, "SMALL", "MEDIUM", "LARGE")
	configure(yam, AttrPeeling, "PEELED", "UNPEELED")
	configure(leafy, AttrCut, "WHOLE", "CHOPPED")
	configure(leafy, AttrCleaning, "WASHED", "UNWASHED")
}
