package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// seedFile is the shape of a file in seeds/v1.
type seedFile struct {
	Kind           string          `json:"kind"`
	GeographyLevel string          `json:"geography_level"`
	GeographyRef   string          `json:"geography_ref"`
	SchemaVersion  int             `json:"schema_version"`
	ChangeReason   string          `json:"change_reason"`
	Body           json.RawMessage `json:"body"`
}

func loadSeed(t *testing.T, name string) seedFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "seeds", "v1", name))
	if err != nil {
		t.Fatalf("read seed %s: %v", name, err)
	}
	var seed seedFile
	if err := json.Unmarshal(raw, &seed); err != nil {
		t.Fatalf("parse seed %s: %v", name, err)
	}
	if seed.ChangeReason == "" {
		t.Fatalf("seed %s has no change_reason; every version needs a stated reason", name)
	}
	return seed
}

func decodeSeed[T Body](t *testing.T, name string) T {
	t.Helper()
	seed := loadSeed(t, name)
	decoded, err := Decode(seed.Kind, seed.SchemaVersion, seed.Body)
	if err != nil {
		t.Fatalf("decode seed %s: %v", name, err)
	}
	typed, ok := decoded.(T)
	if !ok {
		t.Fatalf("seed %s decoded to %T, not the expected type", name, decoded)
	}
	return typed
}

// v1PolicySet builds the resolved V1 policy set for a scope.
func v1PolicySet(t *testing.T, tariffSeed string) PolicySet {
	t.Helper()
	var tariff Tariff
	switch tariffSeed {
	case "taxi_tariff_ng.json":
		tariff = decodeSeed[TaxiTariffV1](t, tariffSeed)
	case "dispatch_tariff_ng.json":
		tariff = decodeSeed[DispatchTariffV1](t, tariffSeed)
	case "haulage_tariff_ng.json":
		tariff = decodeSeed[HaulageTariffV1](t, tariffSeed)
	default:
		t.Fatalf("unknown tariff seed %s", tariffSeed)
	}
	return PolicySet{
		Tariff:     tariff,
		Commission: decodeSeed[CommissionV1](t, "commission_ng.json"),
		Regulatory: decodeSeed[RegulatoryChargesV1](t, "regulatory_charges_ng.json"),
	}
}

// TestEverySeedIsValid proves the shipped V1 bodies decode strictly and pass
// their own validation. A seed that cannot be decoded is a rate that can never be
// approved.
func TestEverySeedIsValid(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "seeds", "v1"))
	if err != nil {
		t.Fatalf("read seeds dir: %v", err)
	}
	found := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		found++
		seed := loadSeed(t, entry.Name())
		if _, err := Decode(seed.Kind, seed.SchemaVersion, seed.Body); err != nil {
			t.Errorf("seed %s does not decode: %v", entry.Name(), err)
		}
	}
	if found != len(KnownKinds()) {
		t.Errorf("found %d seed files for %d known policy kinds; every kind needs a V1 body", found, len(KnownKinds()))
	}
}

// TestV1ReproducesLegacyDispatchPrices is the price-neutrality proof for
// dispatch. The expected values come from shared/go/dispatchpricing and its
// characterization tests, written out here rather than imported: pricing-service
// must not depend on the formula it is replacing.
func TestV1ReproducesLegacyDispatchPrices(t *testing.T) {
	policies := v1PolicySet(t, "dispatch_tariff_ng.json")

	cases := []struct {
		name   string
		metres int64
		size   string
		tier   string
		want   int64
	}{
		{"5km small standard", 5_000, "small", "standard", 170_000},
		{"5km medium standard", 5_000, "medium", "standard", 200_000},
		{"5km large standard", 5_000, "large", "standard", 240_000},
		{"5km small express", 5_000, "small", "express", 229_500},
		{"500m hits the minimum", 500, "small", "standard", 100_000},
		{"10km small standard", 10_000, "small", "standard", 260_000},
		{"unknown size is treated as small", 5_000, "envelope", "standard", 170_000},
		{"unknown tier is treated as standard", 5_000, "small", "priority", 170_000},
		{"size is case and space insensitive", 5_000, " Medium ", "standard", 200_000},
		{"tier is case insensitive", 5_000, "small", "EXPRESS", 229_500},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Evaluate(Request{
				Scope: ScopeDispatch, Channel: ChannelCustomerApp, Class: "MOTORCYCLE",
				DistanceMeters: tc.metres, PackageSize: tc.size, ServiceTier: tc.tier,
				RouteConfidence: RouteConfidenceHigh, AsOf: time.Now(),
			}, policies)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if result.GrossKobo != tc.want {
				t.Fatalf("gross = %d kobo, want %d (legacy dispatchpricing)", result.GrossKobo, tc.want)
			}
		})
	}
}

// TestV1ReproducesLegacyHaulagePrices is the price-neutrality proof for haulage.
func TestV1ReproducesLegacyHaulagePrices(t *testing.T) {
	policies := v1PolicySet(t, "haulage_tariff_ng.json")

	cases := []struct {
		name    string
		metres  int64
		grams   int64
		helpers int64
		want    int64
	}{
		{"10km empty", 10_000, 0, 0, 750_000},
		{"10km exactly 500kg is not surcharged", 10_000, 500_000, 0, 750_000},
		{"10km 501kg is surcharged", 10_000, 501_000, 0, 825_000},
		{"10km with three helpers", 10_000, 0, 3, 1_350_000},
		{"25.5km", 25_500, 0, 0, 1_137_500},
		{"zero distance is the base fare, with no minimum", 0, 0, 0, 500_000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Evaluate(Request{
				Scope: ScopeHaulage, Channel: ChannelCustomerApp, Class: "flatbed",
				DistanceMeters: tc.metres, WeightGrams: tc.grams, Helpers: tc.helpers,
				RouteConfidence: RouteConfidenceHigh, AsOf: time.Now(),
			}, policies)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if result.GrossKobo != tc.want {
				t.Fatalf("gross = %d kobo, want %d (legacy CalculateFare)", result.GrossKobo, tc.want)
			}
		})
	}
}

// TestV1ReproducesLegacyTaxiPrices is the price-neutrality proof for taxi,
// against the DEVELOPMENT rates the seed currently carries. Production sets
// TAXI_PROVISIONAL_RATES explicitly and that value must replace the seed's
// classes block before a real cutover.
func TestV1ReproducesLegacyTaxiPrices(t *testing.T) {
	policies := v1PolicySet(t, "taxi_tariff_ng.json")

	cases := []struct {
		name    string
		class   string
		metres  int64
		seconds int64
		want    int64
	}{
		// economy: base 500, 100/km, 20/min, min 700
		{"economy 8km 20m30s", "economy", 8_000, 1_230, 170_000},
		{"economy short trip hits the minimum", "economy", 300, 60, 70_000},
		{"economy rounds up to the nearest 50 kobo", "economy", 11_234, 0, 162_350},
		{"comfort 8km 20m30s", "comfort", 8_000, 1_230, 224_000},
		{"xl 8km 20m30s", "xl", 8_000, 1_230, 278_000},
		{"29 extra seconds buy no further minute", "economy", 8_000, 1_259, 170_000},
		{"one more second does", "economy", 8_000, 1_260, 172_000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Evaluate(Request{
				Scope: ScopeTaxi, Channel: ChannelCustomerApp, Class: tc.class,
				DistanceMeters: tc.metres, DurationSeconds: tc.seconds,
				RouteConfidence: RouteConfidenceHigh, AsOf: time.Now(),
			}, policies)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if result.GrossKobo != tc.want {
				t.Fatalf("gross = %d kobo, want %d", result.GrossKobo, tc.want)
			}
		})
	}
}

// TestKnownQuantizationDifferenceFromLegacyFloats documents the ONE way V1 is
// expected to differ from the legacy code, and bounds it.
//
// The legacy taxi provider computes int64(per_km * float64(metres)/1000). For
// 29 metres at 100 naira per km the true product is exactly 290 kobo, but the
// float lands on 289.99999999999994 and truncates to 289. The engine, working in
// exact integers, gets 290. The engine is right; the difference is one kobo.
//
// The shadow-comparison phase classifies differences of this size as
// QUANTIZATION rather than MATERIAL for exactly this reason. Pinning it here
// means nobody has to rediscover the cause during a cutover review.
func TestKnownQuantizationDifferenceFromLegacyFloats(t *testing.T) {
	policies := v1PolicySet(t, "taxi_tariff_ng.json")

	result, err := Evaluate(Request{
		Scope: ScopeTaxi, Channel: ChannelCustomerApp, Class: "economy",
		DistanceMeters: 29, DurationSeconds: 0,
		RouteConfidence: RouteConfidenceHigh, AsOf: time.Now(),
	}, policies)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	var distance int64
	for _, component := range result.Components {
		if component.Code == ComponentDistance {
			distance = component.AmountKobo
		}
	}

	const legacyFloatResult = 289 // int64(10000 * (29/1000.0))
	const exactResult = 290       // 10000 * 29 / 1000

	if distance != exactResult {
		t.Fatalf("distance component = %d, want the exact %d", distance, exactResult)
	}
	if exactResult-legacyFloatResult != 1 {
		t.Fatalf("the documented divergence is 1 kobo, got %d", exactResult-legacyFloatResult)
	}
}

// TestCommerceDeliveriesPayTheRiderEverything is the finding that would have cut
// rider pay if V1 had used one flat commission.
func TestCommerceDeliveriesPayTheRiderEverything(t *testing.T) {
	policies := v1PolicySet(t, "dispatch_tariff_ng.json")

	request := Request{
		Scope: ScopeDispatch, Class: "MOTORCYCLE", DistanceMeters: 5_000,
		PackageSize: "small", ServiceTier: "standard",
		RouteConfidence: RouteConfidenceHigh, AsOf: time.Now(),
	}

	commerce := request
	commerce.Channel = ChannelCommerce
	commerceResult, err := Evaluate(commerce, policies)
	if err != nil {
		t.Fatalf("evaluate commerce: %v", err)
	}
	if commerceResult.Allocation.PlatformFeeKobo != 0 {
		t.Fatalf("commerce platform fee = %d, want 0", commerceResult.Allocation.PlatformFeeKobo)
	}
	if commerceResult.Allocation.ProviderKobo != commerceResult.GrossKobo {
		t.Fatalf("commerce rider gets %d of %d; today the rider receives the whole delivery fee",
			commerceResult.Allocation.ProviderKobo, commerceResult.GrossKobo)
	}

	direct := request
	direct.Channel = ChannelCustomerApp
	directResult, err := Evaluate(direct, policies)
	if err != nil {
		t.Fatalf("evaluate direct: %v", err)
	}
	// 15% of 170,000 floored.
	if directResult.Allocation.PlatformFeeKobo != 25_500 {
		t.Fatalf("direct platform fee = %d, want 25500 (15%%)", directResult.Allocation.PlatformFeeKobo)
	}
	if directResult.Allocation.ProviderKobo != 144_500 {
		t.Fatalf("direct provider allocation = %d, want 144500", directResult.Allocation.ProviderKobo)
	}
}
