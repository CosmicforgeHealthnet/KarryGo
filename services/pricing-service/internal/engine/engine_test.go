package engine

import (
	"errors"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/pricing-service/internal/platform/canonical"
	"cosmicforge/logistics/services/pricing-service/internal/platform/money"
)

func TestUnknownPolicyFieldsAreRejected(t *testing.T) {
	// A body carrying a misremembered field name must fail loudly at approval
	// time rather than be ignored and quietly priced as zero.
	body := []byte(`{
		"schema_version": 1,
		"currency": "NGN",
		"base_kobo": 80000,
		"per_km_kobo_v2": 18000,
		"size_addons_kobo": {"small": 0},
		"express_multiplier": {"num": 135, "den": 100},
		"express_applied_to": "SUBTOTAL_BEFORE_ROUNDING",
		"distance_rounding": "HALF_AWAY_FROM_ZERO",
		"default_distance_basis": "ROAD_ROUTE",
		"total_rounding": {"step_kobo": 50, "mode": "HALF_AWAY_FROM_ZERO"},
		"minimum_kobo": 100000,
		"minimum_after_rounding": true
	}`)

	_, err := Decode(KindDispatchTariff, 1, body)
	if err == nil {
		t.Fatal("an unknown field must be rejected")
	}
	if !strings.Contains(err.Error(), "per_km_kobo_v2") {
		t.Fatalf("error should name the offending field, got: %v", err)
	}
}

func TestUnknownKindOrSchemaVersionIsRejected(t *testing.T) {
	if _, err := Decode("taxi.surge", 1, []byte(`{}`)); err == nil {
		t.Fatal("an unknown kind must be rejected")
	}
	if _, err := Decode(KindTaxiTariff, 99, []byte(`{}`)); err == nil {
		t.Fatal("an unknown schema version must be rejected")
	}
}

func TestInvalidBodiesAreRejectedWithEveryProblem(t *testing.T) {
	body := []byte(`{
		"schema_version": 1,
		"currency": "",
		"classes": {},
		"distance_basis": "TELEPORT",
		"distance_rounding": "VIBES",
		"duration_unit": "FORTNIGHTS",
		"time_basis": "GUESS",
		"total_rounding": {"step_kobo": 50, "mode": "VIBES"},
		"minimum_after_rounding": true,
		"low_confidence_route": "SHRUG",
		"hourly_mode": "PRICE_AS_POINT_TO_POINT"
	}`)

	_, err := Decode(KindTaxiTariff, 1, body)
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, want := range []string{"currency", "class", "distance_basis", "distance_rounding", "duration_unit", "time_basis", "low_confidence_route"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q so one submission shows every problem; got: %v", want, err)
		}
	}
}

func TestAnUnpriceableClassIsRefusedNotPricedAtZero(t *testing.T) {
	policies := v1PolicySet(t, "taxi_tariff_ng.json")

	_, err := Evaluate(Request{
		Scope: ScopeTaxi, Channel: ChannelCustomerApp, Class: "helicopter",
		DistanceMeters: 5_000, DurationSeconds: 600, AsOf: time.Now(),
	}, policies)

	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error = %v, want ErrUnsupported: a class with no rates must surface as unpriced, never as a free ride", err)
	}
}

func TestScopeAndTariffMustAgree(t *testing.T) {
	// Handing the taxi tariff a dispatch request would price a bike delivery on
	// passenger rates.
	policies := v1PolicySet(t, "taxi_tariff_ng.json")
	_, err := Evaluate(Request{
		Scope: ScopeDispatch, Channel: ChannelCustomerApp, Class: "MOTORCYCLE",
		DistanceMeters: 5_000, AsOf: time.Now(),
	}, policies)
	if err == nil {
		t.Fatal("a scope/tariff mismatch must be refused")
	}
}

func TestComponentsAlwaysSumToGrossAndFundingBalances(t *testing.T) {
	// A property test across all three services and a spread of awkward inputs.
	// Conservation is the rule the wallet depends on: an instruction that does
	// not balance cannot be executed without inventing or losing money.
	taxi := v1PolicySet(t, "taxi_tariff_ng.json")
	dispatch := v1PolicySet(t, "dispatch_tariff_ng.json")
	haulage := v1PolicySet(t, "haulage_tariff_ng.json")

	metres := []int64{0, 1, 29, 500, 1_234, 5_000, 11_234, 25_500, 480_000}
	seconds := []int64{0, 1, 59, 60, 1_230, 7_199}

	for _, m := range metres {
		for _, s := range seconds {
			checks := []struct {
				name     string
				request  Request
				policies PolicySet
			}{
				{"taxi", Request{Scope: ScopeTaxi, Channel: ChannelCustomerApp, Class: "economy",
					DistanceMeters: m, DurationSeconds: s, AsOf: time.Now()}, taxi},
				{"dispatch", Request{Scope: ScopeDispatch, Channel: ChannelCustomerApp, Class: "MOTORCYCLE",
					DistanceMeters: m, PackageSize: "large", ServiceTier: "express", AsOf: time.Now()}, dispatch},
				{"haulage", Request{Scope: ScopeHaulage, Channel: ChannelCustomerApp, Class: "flatbed",
					DistanceMeters: m, WeightGrams: 900_000, Helpers: 2, AsOf: time.Now()}, haulage},
			}

			for _, check := range checks {
				result, err := Evaluate(check.request, check.policies)
				if err != nil {
					t.Fatalf("%s at %dm/%ds: %v", check.name, m, s, err)
				}

				var sum int64
				for _, component := range result.Components {
					total, sumErr := money.Sum(sum, component.AmountKobo)
					if sumErr != nil {
						t.Fatalf("%s: %v", check.name, sumErr)
					}
					sum = total
				}
				if sum != result.GrossKobo {
					t.Fatalf("%s at %dm/%ds: components sum to %d but gross is %d",
						check.name, m, s, sum, result.GrossKobo)
				}
				if result.Funding.Total() != result.GrossKobo {
					t.Fatalf("%s at %dm/%ds: funding %d != gross %d",
						check.name, m, s, result.Funding.Total(), result.GrossKobo)
				}
				if result.Allocation.Total() != result.GrossKobo {
					t.Fatalf("%s at %dm/%ds: allocation %d != gross %d",
						check.name, m, s, result.Allocation.Total(), result.GrossKobo)
				}
				if result.Allocation.ProviderKobo < 0 || result.Allocation.PlatformFeeKobo < 0 {
					t.Fatalf("%s at %dm/%ds: a negative allocation", check.name, m, s)
				}
			}
		}
	}
}

func TestRoundingAndMinimumAreVisibleComponents(t *testing.T) {
	// An operator reading a quote must be able to see WHY the total is what it is,
	// including the rounding step and any top-up to a minimum.
	dispatch := v1PolicySet(t, "dispatch_tariff_ng.json")

	result, err := Evaluate(Request{
		Scope: ScopeDispatch, Channel: ChannelCustomerApp, Class: "MOTORCYCLE",
		DistanceMeters: 500, PackageSize: "small", ServiceTier: "standard", AsOf: time.Now(),
	}, dispatch)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	codes := map[string]int64{}
	for _, component := range result.Components {
		codes[component.Code] = component.AmountKobo
	}
	if _, ok := codes[ComponentMinimumTopUp]; !ok {
		t.Fatalf("a fare raised to the minimum must show a %s component; got %v", ComponentMinimumTopUp, codes)
	}
	if !contains(result.Flags, FlagMinimumApplied) {
		t.Errorf("expected the %s flag, got %v", FlagMinimumApplied, result.Flags)
	}
	// base 80,000 + 9,000 distance = 89,000, topped up by 11,000 to 100,000.
	if codes[ComponentMinimumTopUp] != 11_000 {
		t.Errorf("minimum top-up = %d, want 11000", codes[ComponentMinimumTopUp])
	}
}

func TestExpressAppliesToTheWholeSubtotalNotJustDistance(t *testing.T) {
	dispatch := v1PolicySet(t, "dispatch_tariff_ng.json")

	result, err := Evaluate(Request{
		Scope: ScopeDispatch, Channel: ChannelCustomerApp, Class: "MOTORCYCLE",
		DistanceMeters: 5_000, PackageSize: "small", ServiceTier: "express", AsOf: time.Now(),
	}, dispatch)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	var express int64
	for _, component := range result.Components {
		if component.Code == ComponentExpress {
			express = component.AmountKobo
		}
	}
	// 35% of (80,000 + 90,000) = 59,500. If express were applied to distance
	// only it would be 31,500, and every express delivery would be underpriced.
	if express != 59_500 {
		t.Fatalf("express surcharge = %d, want 59500 (35%% of the whole subtotal)", express)
	}
}

func TestADegradedRouteIsFlaggedButStillPriced(t *testing.T) {
	// V1 must stay price-neutral, and the legacy services priced a fallback route
	// silently. So the price stands and the fact is recorded, making it visible
	// in reports for the first time.
	taxi := v1PolicySet(t, "taxi_tariff_ng.json")

	result, err := Evaluate(Request{
		Scope: ScopeTaxi, Channel: ChannelCustomerApp, Class: "economy",
		DistanceMeters: 8_000, DurationSeconds: 1_200,
		RouteConfidence: RouteConfidenceLow, RouteDegraded: true, AsOf: time.Now(),
	}, taxi)
	if err != nil {
		t.Fatalf("a degraded route must still price in V1: %v", err)
	}
	if !contains(result.Flags, FlagRouteDegraded) || !contains(result.Flags, FlagRouteLowConf) {
		t.Fatalf("expected route flags, got %v", result.Flags)
	}
}

func TestAPolicyMayRefuseAnUntrustedRoute(t *testing.T) {
	// The same tariff with low_confidence_route: REFUSE, which a later approved
	// version can adopt per market.
	tariff := decodeSeed[TaxiTariffV1](t, "taxi_tariff_ng.json")
	tariff.LowConfidenceRoute = LowConfidenceRefuse
	policies := PolicySet{Tariff: tariff, Commission: decodeSeed[CommissionV1](t, "commission_ng.json")}

	_, err := Evaluate(Request{
		Scope: ScopeTaxi, Channel: ChannelCustomerApp, Class: "economy",
		DistanceMeters: 8_000, DurationSeconds: 1_200,
		RouteConfidence: RouteConfidenceLow, RouteDegraded: true, AsOf: time.Now(),
	}, policies)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error = %v, want ErrUnsupported", err)
	}
}

func TestCanonicalBodyHashIgnoresFormatting(t *testing.T) {
	// An approval binds to the body hash. If reformatting changed the hash, an
	// approval would be invalidated by whitespace.
	compact := []byte(`{"schema_version":1,"default_bps":1500,"applied_to":"CUSTOMER_PAYABLE","rounding":"FLOOR"}`)
	reordered := []byte(`{
		"rounding": "FLOOR",
		"applied_to": "CUSTOMER_PAYABLE",
		"default_bps": 1500,
		"schema_version": 1
	}`)

	_, firstHash, err := CanonicalBody(KindCommission, 1, compact)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	_, secondHash, err := CanonicalBody(KindCommission, 1, reordered)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("formatting changed the body hash: %s vs %s", firstHash, secondHash)
	}

	// A real change must change the hash.
	changed := []byte(`{"schema_version":1,"default_bps":1600,"applied_to":"CUSTOMER_PAYABLE","rounding":"FLOOR"}`)
	_, changedHash, err := CanonicalBody(KindCommission, 1, changed)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	if changedHash == firstHash {
		t.Fatal("changing the commission rate did not change the body hash")
	}
	if len(firstHash) != 64 {
		t.Fatalf("hash is %d characters, want 64", len(firstHash))
	}
	// And the stored form must itself be canonical.
	stored, _, err := CanonicalBody(KindCommission, 1, reordered)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	if !strings.HasPrefix(string(stored), `{"applied_to"`) {
		t.Fatalf("stored body is not canonically ordered: %s", stored)
	}
	_ = canonical.Combine("a", "b")
}

func TestTheExplanationIsReadableAndComplete(t *testing.T) {
	policies := v1PolicySet(t, "dispatch_tariff_ng.json")
	result, err := Evaluate(Request{
		Scope: ScopeDispatch, Channel: ChannelCustomerApp, Class: "MOTORCYCLE",
		DistanceMeters: 5_000, PackageSize: "medium", ServiceTier: "express",
		RouteConfidence: RouteConfidenceHigh, AsOf: time.Now(),
	}, policies)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	joined := strings.Join(result.Explanation, "\n")
	for _, want := range []string{"DISPATCH", "Route:", "BASE", "DISTANCE", "Economic fare", "Funded by", "Allocated to", "15%"} {
		if !strings.Contains(joined, want) {
			t.Errorf("explanation should mention %q; got:\n%s", want, joined)
		}
	}
}

func TestAnInvalidRequestIsRefused(t *testing.T) {
	policies := v1PolicySet(t, "taxi_tariff_ng.json")
	cases := map[string]Request{
		"no scope":          {Channel: ChannelCustomerApp, Class: "economy"},
		"no channel":        {Scope: ScopeTaxi, Class: "economy"},
		"no class":          {Scope: ScopeTaxi, Channel: ChannelCustomerApp},
		"negative distance": {Scope: ScopeTaxi, Channel: ChannelCustomerApp, Class: "economy", DistanceMeters: -1},
		"negative duration": {Scope: ScopeTaxi, Channel: ChannelCustomerApp, Class: "economy", DurationSeconds: -1},
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Evaluate(request, policies); err == nil {
				t.Fatal("expected the request to be refused")
			}
		})
	}
}

func TestLifecycleTTLAndBindGrace(t *testing.T) {
	lifecycle := decodeSeed[QuoteLifecycleV1](t, "quote_lifecycle_ng.json")

	if got := lifecycle.TTLFor(ScopeTaxi, ChannelCustomerApp); got != 5*time.Minute {
		t.Errorf("taxi app TTL = %v, want 5m", got)
	}
	if got := lifecycle.TTLFor(ScopeHaulage, ChannelCommerce); got != 20*time.Minute {
		t.Errorf("haulage commerce TTL = %v, want 20m", got)
	}
	// An unlisted combination falls back to the default rather than to zero,
	// which would make every such quote expire instantly.
	if got := lifecycle.TTLFor(ScopeTaxi, ChannelOps); got != 15*time.Minute {
		t.Errorf("unlisted TTL = %v, want the 15m default", got)
	}
	// Commerce orders are paid after approval, sometimes well after, so they get
	// a grace window; an app quote does not.
	if got := lifecycle.BindGraceFor(ChannelCommerce); got != 24*time.Hour {
		t.Errorf("commerce bind grace = %v, want 24h", got)
	}
	if got := lifecycle.BindGraceFor(ChannelCustomerApp); got != 0 {
		t.Errorf("app bind grace = %v, want 0", got)
	}
}

func TestV1ChargesNoTaxOrTolls(t *testing.T) {
	// The legacy services charge none, so V1 charges none. Adding one is a
	// governed policy change with an official source, not a code change.
	regulatory := decodeSeed[RegulatoryChargesV1](t, "regulatory_charges_ng.json")
	if len(regulatory.Charges) != 0 {
		t.Fatalf("V1 must charge no regulatory amounts, found %d", len(regulatory.Charges))
	}

	policies := v1PolicySet(t, "taxi_tariff_ng.json")
	result, err := Evaluate(Request{
		Scope: ScopeTaxi, Channel: ChannelCustomerApp, Class: "economy",
		DistanceMeters: 8_000, DurationSeconds: 1_200, AsOf: time.Now(),
	}, policies)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result.Allocation.TaxKobo != 0 {
		t.Fatalf("tax = %d, want 0 in V1", result.Allocation.TaxKobo)
	}
}

func TestARegulatoryChargeNeedsAnOfficialSource(t *testing.T) {
	body := []byte(`{"schema_version":1,"charges":[{"code":"LAGOS_TOLL","amount_kobo":50000,"applies_to":"ROUTE"}]}`)
	_, err := Decode(KindRegulatoryCharges, 1, body)
	if err == nil {
		t.Fatal("a charge without a source reference must be refused")
	}
	if !strings.Contains(err.Error(), "source_reference") {
		t.Fatalf("error should require an official source, got: %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
