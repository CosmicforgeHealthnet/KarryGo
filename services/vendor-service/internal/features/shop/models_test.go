package shop

import "testing"

func TestOperatingModelCapabilityBoundaries(t *testing.T) {
	want := map[string][]string{
		ModelSupermarket: {"SHOP_GROCERY"}, ModelGroceryShop: {"SHOP_GROCERY"},
		ModelFoodstuffDealer: {"SHOP_GROCERY", "WHOLESALE"}, ModelMarketTrader: {"MARKET"},
		ModelWholesaler: {"WHOLESALE"},
	}
	for model, capabilities := range want {
		if !IsOperatingModel(model) {
			t.Errorf("model %s not recognized", model)
		}
		actual := CapabilitiesForOperatingModel(model)
		if len(actual) != len(capabilities) {
			t.Errorf("%s capabilities=%v want=%v", model, actual, capabilities)
			continue
		}
		for i := range actual {
			if actual[i] != capabilities[i] {
				t.Errorf("%s capabilities=%v want=%v", model, actual, capabilities)
			}
		}
	}
	for _, foreign := range []string{"RESTAURANT", "FAST_FOOD", "CLOUD_KITCHEN", "PASTRY_BAKERY"} {
		if IsShopCapability(foreign) {
			t.Errorf("foreign capability %s entered Shop", foreign)
		}
	}
}

func TestFractionsAreBoundedRationalsNotDuplicatedUnitEnums(t *testing.T) {
	n, d, err := normalizeFraction(intp(2), intp(4))
	if err != nil || n != 1 || d != 2 {
		t.Fatalf("2/4 normalized to %d/%d err=%v", n, d, err)
	}
	for _, pair := range [][2]int{{0, 1}, {1, 0}, {-1, 2}, {1, -2}, {MaxFractionNumerator + 1, 1}, {1, MaxFractionDenominator + 1}} {
		numerator, denominator := pair[0], pair[1]
		if _, _, err = normalizeFraction(&numerator, &denominator); err == nil {
			t.Errorf("invalid fraction %d/%d accepted", numerator, denominator)
		}
	}
}

func TestFreshGoodsAttributeApplicability(t *testing.T) {
	cases := []struct {
		example, class  string
		allowed, denied []string
	}{
		{"tomato ripeness/quality", "FRESH_PRODUCE", []string{AttrRipeness, AttrQuality}, []string{AttrDescaling, AttrBone}},
		{"plantain ripeness", "FRESH_PRODUCE", []string{AttrRipeness, AttrSize}, []string{AttrGutting}},
		{"fish cut/clean", "FISH", []string{AttrCut, AttrCleaning, AttrGutting, AttrDescaling, AttrHead}, []string{AttrRipeness, AttrBone}},
		{"meat cut/bone", "MEAT", []string{AttrCut, AttrCleaning, AttrBone}, []string{AttrRipeness, AttrDescaling}},
		{"yam size/tuber preparation", "FRESH_PRODUCE", []string{AttrSize, AttrCut, AttrPeeling}, []string{AttrHead}},
		{"leafy vegetable bunch/chopped", "FRESH_PRODUCE", []string{AttrCut, AttrCleaning, AttrTrimming}, []string{AttrBone}},
	}
	for _, tc := range cases {
		for _, attribute := range tc.allowed {
			if !AttributeAllowedForClass(tc.class, attribute) {
				t.Errorf("%s: %s denied", tc.example, attribute)
			}
		}
		for _, attribute := range tc.denied {
			if AttributeAllowedForClass(tc.class, attribute) {
				t.Errorf("%s: %s incorrectly allowed", tc.example, attribute)
			}
		}
	}
}

func TestCommercialVocabulariesCoverNigerianTradeScenarios(t *testing.T) {
	for _, family := range []string{FamilyMass, FamilyVolume, FamilyCount, FamilyLocalContainer, FamilyPackage, FamilyGroupedCount, FamilyLoad, FamilyVariableWeight, FamilyVariableCount, FamilyPricePortion} {
		if !IsMeasurementFamily(family) {
			t.Errorf("measurement family %s missing", family)
		}
	}
	for _, level := range []string{"EACH", "PACK", "TRAY", "BAG", "SACK", "CARTON", "CRATE", "DRUM", "PALLET"} {
		if !IsPackageLevel(level) {
			t.Errorf("package level %s missing", level)
		}
	}
	for _, mode := range []string{ModeFixed, ModeVariableWeight, ModeVariableCount, ModeLocalContainer, ModePackageCount, ModePricePortion} {
		if !contains(MeasurementModes, mode) {
			t.Errorf("measurement mode %s missing", mode)
		}
	}
}
