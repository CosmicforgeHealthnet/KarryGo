package engine

import (
	"fmt"

	"cosmicforge/logistics/services/pricing-service/internal/platform/money"
)

// What a haulage weight surcharge is charged on.
const (
	SurchargeOnBasePlusDistance = "BASE_PLUS_DISTANCE"
	SurchargeOnBaseOnly         = "BASE_ONLY"
)

// WeightSurchargeV1 is a percentage surcharge above a threshold.
//
// ThresholdGramsExclusive is exclusive because the legacy formula uses a strict
// greater-than: a load of exactly 500kg is NOT surcharged. That is a real
// boundary customers sit on, so it is named in the field rather than left to a
// comparison operator buried in code.
type WeightSurchargeV1 struct {
	ThresholdGramsExclusive int64              `json:"threshold_grams_exclusive"`
	Rate                    money.Rational     `json:"rate"`
	On                      string             `json:"on"`
	Rounding                money.RoundingMode `json:"rounding"`
}

// HaulageTariffV1 is schema version 1 of a haulage tariff.
//
// Two legacy quirks are reproduced deliberately: there is no total rounding and
// no minimum fare (unlike taxi and dispatch), and the weight surcharge truncates
// because the legacy code used integer division. V1 must be price-neutral, so
// both are expressed as policy rather than corrected here.
//
// Its distance basis is STRAIGHT_LINE, which under-measures a real road journey.
// Moving haulage to road routing is a genuine price change and therefore a later,
// approved version with recalibrated rates -- not a quiet fix.
type HaulageTariffV1 struct {
	SchemaVersion int    `json:"schema_version"`
	Currency      string `json:"currency"`

	BaseKobo         int64              `json:"base_kobo"`
	PerKmKobo        int64              `json:"per_km_kobo"`
	DistanceRounding money.RoundingMode `json:"distance_rounding"`
	DistanceBasis    string             `json:"distance_basis"`

	WeightSurcharge WeightSurchargeV1 `json:"weight_surcharge"`
	HelperFeeKobo   int64             `json:"helper_fee_kobo"`

	TotalRounding     Rounding `json:"total_rounding"`
	MinimumKobo       int64    `json:"minimum_kobo"`
	MinimumAfterRound bool     `json:"minimum_after_rounding"`

	// Classes lists the truck classes this policy may price. V1 shares one rate
	// set across classes, matching the legacy formula, which ignored truck type
	// entirely. Per-class economics is a later version, and listing the classes
	// now means an unknown truck type is refused rather than silently priced as
	// if it were any other.
	Classes []string `json:"classes"`
}

// Kind returns the policy kind this body belongs to.
func (HaulageTariffV1) Kind() string { return KindHaulageTariff }

// Validate reports every problem with the body at once.
func (h HaulageTariffV1) Validate() error {
	var problems []string
	if h.SchemaVersion != 1 {
		problems = append(problems, "schema_version must be 1")
	}
	if h.Currency == "" {
		problems = append(problems, "currency is required")
	}
	if h.BaseKobo < 0 || h.PerKmKobo < 0 || h.HelperFeeKobo < 0 || h.MinimumKobo < 0 {
		problems = append(problems, "amounts cannot be negative")
	}
	if !h.DistanceRounding.Valid() {
		problems = append(problems, fmt.Sprintf("distance_rounding %q is unknown", h.DistanceRounding))
	}
	if h.DistanceBasis != DistanceBasisRoadRoute && h.DistanceBasis != DistanceBasisStraightLine {
		problems = append(problems, fmt.Sprintf("distance_basis %q is unknown", h.DistanceBasis))
	}
	if h.WeightSurcharge.ThresholdGramsExclusive < 0 {
		problems = append(problems, "weight_surcharge.threshold_grams_exclusive cannot be negative")
	}
	if !h.WeightSurcharge.Rate.Valid() {
		problems = append(problems, "weight_surcharge.rate has a zero denominator")
	}
	switch h.WeightSurcharge.On {
	case SurchargeOnBasePlusDistance, SurchargeOnBaseOnly:
	default:
		problems = append(problems, fmt.Sprintf("weight_surcharge.on %q is unknown", h.WeightSurcharge.On))
	}
	if !h.WeightSurcharge.Rounding.Valid() {
		problems = append(problems, fmt.Sprintf("weight_surcharge.rounding %q is unknown", h.WeightSurcharge.Rounding))
	}
	if len(h.Classes) == 0 {
		problems = append(problems, "at least one truck class is required")
	}
	problems = append(problems, h.TotalRounding.Validate("total_rounding")...)

	if len(problems) > 0 {
		return fmt.Errorf("haulage.tariff v1: %s", join(problems))
	}
	return nil
}

// Evaluate prices req.
func (h HaulageTariffV1) Evaluate(req Request) ([]Component, int64, []string, error) {
	if !h.supportsClass(req.Class) {
		return nil, 0, nil, fmt.Errorf("%w: haulage class %q is not priced by this policy", ErrUnsupported, req.Class)
	}

	metres, err := distanceFor(h.DistanceBasis, req)
	if err != nil {
		return nil, 0, nil, err
	}

	b := &builder{}
	b.add(ComponentBase, h.BaseKobo, "base haulage cost")

	distanceKobo, err := money.PerUnit(h.PerKmKobo, metres, 1_000, h.DistanceRounding)
	if err != nil {
		return nil, 0, nil, err
	}
	b.add(ComponentDistance, distanceKobo, "%s metres at %s per km", withThousands(metres), kobo(h.PerKmKobo))
	if b.err != nil {
		return nil, 0, nil, b.err
	}

	if req.WeightGrams > h.WeightSurcharge.ThresholdGramsExclusive {
		surchargeOn := b.total
		if h.WeightSurcharge.On == SurchargeOnBaseOnly {
			surchargeOn = h.BaseKobo
		}
		surcharge, surErr := money.Multiply(surchargeOn, h.WeightSurcharge.Rate, h.WeightSurcharge.Rounding)
		if surErr != nil {
			return nil, 0, nil, surErr
		}
		b.add(ComponentWeight, surcharge, "%s exceeds %s: %s surcharge (%s)",
			grams(req.WeightGrams), grams(h.WeightSurcharge.ThresholdGramsExclusive),
			h.WeightSurcharge.Rate, h.WeightSurcharge.Rounding)
	}

	if req.Helpers > 0 {
		helpers, helpErr := money.PerUnit(h.HelperFeeKobo, req.Helpers, 1, money.Truncate)
		if helpErr != nil {
			return nil, 0, nil, helpErr
		}
		b.add(ComponentHelpers, helpers, "%d helper(s) at %s each", req.Helpers, kobo(h.HelperFeeKobo))
	}

	components, gross, finishFlags, err := b.finish(h.TotalRounding, h.MinimumKobo, h.MinimumAfterRound)
	if err != nil {
		return nil, 0, nil, err
	}
	return components, gross, append(routeFlags(req), finishFlags...), nil
}

func (h HaulageTariffV1) supportsClass(class string) bool {
	for _, supported := range h.Classes {
		if supported == class {
			return true
		}
	}
	return false
}

// grams renders a weight for explanation text. Display only.
func grams(value int64) string {
	if value%1000 == 0 {
		return fmt.Sprintf("%skg", withThousands(value/1000))
	}
	return fmt.Sprintf("%sg", withThousands(value))
}
