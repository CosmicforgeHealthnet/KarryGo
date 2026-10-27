package engine

import (
	"fmt"
	"sort"

	"cosmicforge/logistics/services/pricing-service/internal/platform/canonical"
	"cosmicforge/logistics/services/pricing-service/internal/platform/money"
)

// KindFuelAdjustment is the rule book that turns the drivers' daily fuel index
// into PROPOSED tariff changes. It never prices anything itself and a proposal
// changes no fare until an approver approves and activates it.
const KindFuelAdjustment = "fuel.adjustment"

// Fuel types the index tracks.
const (
	FuelPetrol = "PETROL"
	FuelDiesel = "DIESEL"
	FuelGas    = "GAS"
)

func validFuel(f string) bool { return f == FuelPetrol || f == FuelDiesel || f == FuelGas }

// FuelAdjustmentV1 is schema version 1 of the fuel adjustment rules.
//
// FuelShareBPS is how much of a tariff's per-km rate is fuel: with a 40% share,
// a 10% rise in fuel moves the per-km rate by 4%. Only the per-km rate moves;
// base fares, time rates and minimums are left for a human to judge.
type FuelAdjustmentV1 struct {
	SchemaVersion int `json:"schema_version"`
	// FuelFor names the fuel each tariff kind runs on, e.g. haulage on DIESEL.
	FuelFor map[string]string `json:"fuel_for"`
	// BaselineKoboPerLitre is the fuel price the current tariffs were set at.
	BaselineKoboPerLitre map[string]int64 `json:"baseline_kobo_per_litre"`
	FuelShareBPS         map[string]int64 `json:"fuel_share_bps"`
	// ThresholdBPS is the smallest fuel move worth proposing a change for.
	ThresholdBPS int64 `json:"threshold_bps"`
	// MinReports is how many distinct drivers' reports an index needs.
	MinReports int `json:"min_reports"`
	// CooldownDays stops a rejected proposal from being re-proposed every day.
	CooldownDays int `json:"cooldown_days"`
}

// Kind returns the policy kind this body belongs to.
func (FuelAdjustmentV1) Kind() string { return KindFuelAdjustment }

// Validate reports every problem with the body at once.
func (f FuelAdjustmentV1) Validate() error {
	var problems []string
	if f.SchemaVersion != 1 {
		problems = append(problems, "schema_version must be 1")
	}
	if len(f.FuelFor) == 0 {
		problems = append(problems, "fuel_for must name at least one tariff kind")
	}
	for _, kind := range sortedKeys(f.FuelFor) {
		fuel := f.FuelFor[kind]
		if kind != KindTaxiTariff && kind != KindDispatchTariff && kind != KindHaulageTariff {
			problems = append(problems, fmt.Sprintf("fuel_for has unknown tariff kind %q", kind))
		}
		if !validFuel(fuel) {
			problems = append(problems, fmt.Sprintf("fuel_for[%s] must be PETROL, DIESEL or GAS", kind))
		}
		if f.BaselineKoboPerLitre[fuel] <= 0 {
			problems = append(problems, fmt.Sprintf("baseline_kobo_per_litre[%s] is required for %s", fuel, kind))
		}
		share, ok := f.FuelShareBPS[kind]
		if !ok || share < 0 || share > 10_000 {
			problems = append(problems, fmt.Sprintf("fuel_share_bps[%s] must be between 0 and 10000", kind))
		}
	}
	for fuel, price := range f.BaselineKoboPerLitre {
		if !validFuel(fuel) || price <= 0 {
			problems = append(problems, fmt.Sprintf("baseline_kobo_per_litre[%s] must be a known fuel with a positive price", fuel))
		}
	}
	if f.ThresholdBPS < 1 || f.ThresholdBPS > 10_000 {
		problems = append(problems, "threshold_bps must be between 1 and 10000")
	}
	if f.MinReports < 1 {
		problems = append(problems, "min_reports must be at least 1")
	}
	if f.CooldownDays < 0 || f.CooldownDays > 90 {
		problems = append(problems, "cooldown_days must be between 0 and 90")
	}
	if len(problems) > 0 {
		return fmt.Errorf("fuel.adjustment v1: %s", join(problems))
	}
	return nil
}

// FuelChangeBPS is how far index has moved from baseline, in basis points,
// truncated toward zero so a proposal never overstates the move.
func FuelChangeBPS(index, baseline int64) int64 {
	if baseline <= 0 {
		return 0
	}
	return (index - baseline) * 10_000 / baseline
}

// PerKmChange is one per-km rate a proposal moves.
type PerKmChange struct {
	Class   string `json:"class,omitempty"`
	OldKobo int64  `json:"old_kobo"`
	NewKobo int64  `json:"new_kobo"`
}

// AdjustPerKm returns a tariff body whose per-km rates are moved by the fuel
// share of a fuel price change, rounded to the nearest naira. Every other field
// is copied unchanged.
func AdjustPerKm(kind string, schemaVersion int, body []byte, shareBPS, changeBPS int64) ([]byte, []PerKmChange, error) {
	tariff, err := DecodeTariff(kind, schemaVersion, body)
	if err != nil {
		return nil, nil, err
	}
	// rate * (1 + share*change/1e8), exactly, then to the nearest naira.
	factor := money.Rational{Num: 100_000_000 + shareBPS*changeBPS, Den: 100_000_000}
	if factor.Num <= 0 {
		return nil, nil, fmt.Errorf("engine: fuel change of %d bps would make the per-km rate negative", changeBPS)
	}
	move := func(old int64) (int64, error) {
		scaled, err := money.Multiply(old, factor, money.HalfUp)
		if err != nil {
			return 0, err
		}
		return money.RoundToStep(scaled, 100, money.HalfUp)
	}

	var changes []PerKmChange
	var adjusted Body
	switch t := tariff.(type) {
	case TaxiTariffV1:
		classes := make(map[string]TaxiClassRate, len(t.Classes))
		for _, name := range sortedKeys(t.Classes) {
			rate := t.Classes[name]
			next, err := move(rate.PerKmKobo)
			if err != nil {
				return nil, nil, err
			}
			changes = append(changes, PerKmChange{Class: name, OldKobo: rate.PerKmKobo, NewKobo: next})
			rate.PerKmKobo = next
			classes[name] = rate
		}
		t.Classes = classes
		adjusted = t
	case DispatchTariffV1:
		next, err := move(t.PerKmKobo)
		if err != nil {
			return nil, nil, err
		}
		changes = append(changes, PerKmChange{OldKobo: t.PerKmKobo, NewKobo: next})
		t.PerKmKobo = next
		adjusted = t
	case HaulageTariffV1:
		next, err := move(t.PerKmKobo)
		if err != nil {
			return nil, nil, err
		}
		changes = append(changes, PerKmChange{OldKobo: t.PerKmKobo, NewKobo: next})
		t.PerKmKobo = next
		adjusted = t
	default:
		return nil, nil, fmt.Errorf("engine: no per-km rate on %s", kind)
	}
	if err := adjusted.Validate(); err != nil {
		return nil, nil, err
	}
	encoded, err := canonical.Marshal(adjusted)
	if err != nil {
		return nil, nil, err
	}
	return encoded, changes, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
