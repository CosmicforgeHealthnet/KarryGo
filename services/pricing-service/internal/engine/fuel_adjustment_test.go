package engine

import (
	"encoding/json"
	"testing"
)

func TestFuelAdjustmentSeedIsValid(t *testing.T) {
	rules := decodeSeed[FuelAdjustmentV1](t, "fuel_adjustment_ng.json")
	if rules.FuelFor[KindHaulageTariff] != FuelDiesel || rules.ThresholdBPS != 500 {
		t.Fatalf("seed = %+v", rules)
	}
}

func TestFuelAdjustmentRefusesIncompleteRules(t *testing.T) {
	body := []byte(`{"schema_version":1,"fuel_for":{"haulage.tariff":"DIESEL"},"baseline_kobo_per_litre":{},
		"fuel_share_bps":{},"threshold_bps":0,"min_reports":0,"cooldown_days":0}`)
	if _, err := Decode(KindFuelAdjustment, 1, body); err == nil {
		t.Fatal("rules with no baseline, share or threshold were accepted")
	}
}

func TestFuelChangeBPS(t *testing.T) {
	cases := []struct{ index, baseline, want int64 }{
		{110_000, 100_000, 1000},
		{95_000, 100_000, -500},
		{100_049, 100_000, 4}, // truncated, never overstated
		{100_000, 0, 0},
	}
	for _, c := range cases {
		if got := FuelChangeBPS(c.index, c.baseline); got != c.want {
			t.Errorf("FuelChangeBPS(%d, %d) = %d, want %d", c.index, c.baseline, got, c.want)
		}
	}
}

// Only per-km rates move, by share x change, to the nearest naira; everything
// else in the tariff is copied as it was.
func TestAdjustPerKmMovesOnlyTheDistanceRate(t *testing.T) {
	seed := loadSeed(t, "taxi_tariff_ng.json")
	before := decodeSeed[TaxiTariffV1](t, "taxi_tariff_ng.json")

	// Fuel up 10%, fuel is 40% of the per-km rate: per-km up 4%.
	body, changes, err := AdjustPerKm(KindTaxiTariff, 1, seed.Body, 4000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(KindTaxiTariff, 1, body)
	if err != nil {
		t.Fatalf("adjusted body no longer valid: %v", err)
	}
	after := decoded.(TaxiTariffV1)
	if len(changes) != len(before.Classes) {
		t.Fatalf("%d changes for %d classes", len(changes), len(before.Classes))
	}
	for name, old := range before.Classes {
		got := after.Classes[name]
		want := (old.PerKmKobo*104/100 + 50) / 100 * 100
		if got.PerKmKobo != want {
			t.Errorf("%s per-km %d -> %d, want %d", name, old.PerKmKobo, got.PerKmKobo, want)
		}
		if got.BaseKobo != old.BaseKobo || got.PerMinKobo != old.PerMinKobo || got.MinKobo != old.MinKobo {
			t.Errorf("%s: a rate other than per-km moved: %+v -> %+v", name, old, got)
		}
	}
	if after.TotalRounding != before.TotalRounding || after.DurationUnit != before.DurationUnit {
		t.Fatal("tariff settings changed")
	}
}

func TestAdjustPerKmForDispatchAndHaulage(t *testing.T) {
	for _, name := range []string{"dispatch_tariff_ng.json", "haulage_tariff_ng.json"} {
		seed := loadSeed(t, name)
		var original struct {
			PerKmKobo int64 `json:"per_km_kobo"`
		}
		_ = json.Unmarshal(seed.Body, &original)
		// Fuel down 20% at a 50% share: per-km down 10%.
		body, changes, err := AdjustPerKm(seed.Kind, 1, seed.Body, 5000, -2000)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var adjusted struct {
			PerKmKobo int64 `json:"per_km_kobo"`
		}
		_ = json.Unmarshal(body, &adjusted)
		want := (original.PerKmKobo*90/100 + 50) / 100 * 100
		if adjusted.PerKmKobo != want || changes[0].NewKobo != want || changes[0].OldKobo != original.PerKmKobo {
			t.Fatalf("%s: per-km %d -> %d, want %d (%+v)", name, original.PerKmKobo, adjusted.PerKmKobo, want, changes)
		}
	}
	if _, _, err := AdjustPerKm(KindCommission, 1, []byte(`{}`), 5000, 1000); err == nil {
		t.Fatal("a non-tariff was adjusted")
	}
}
