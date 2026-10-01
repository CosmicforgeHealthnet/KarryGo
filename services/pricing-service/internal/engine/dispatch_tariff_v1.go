package engine

import (
	"fmt"
	"strings"

	"cosmicforge/logistics/services/pricing-service/internal/platform/money"
)

// Where an express multiplier is applied. The legacy formula multiplies the whole
// subtotal including the base, before rounding -- not just the distance -- so
// this is stated explicitly rather than assumed.
const (
	ExpressOnSubtotalBeforeRounding = "SUBTOTAL_BEFORE_ROUNDING"
	ExpressOnDistanceOnly           = "DISTANCE_ONLY"
)

// Service tiers.
const (
	TierStandard = "standard"
	TierExpress  = "express"
)

// DispatchTariffV1 is schema version 1 of a dispatch tariff.
//
// DistanceBasisByChannel exists because the legacy surfaces genuinely disagree:
// the serviceability and multi-stop quotes price a real road route, while the
// booking estimate, commerce quote and partner API price a straight line. V1
// reproduces that per channel rather than quietly unifying it, because unifying
// it would change live prices. Unification is a later, approved version.
type DispatchTariffV1 struct {
	SchemaVersion int    `json:"schema_version"`
	Currency      string `json:"currency"`

	BaseKobo       int64            `json:"base_kobo"`
	PerKmKobo      int64            `json:"per_km_kobo"`
	SizeAddonsKobo map[string]int64 `json:"size_addons_kobo"`

	ExpressMultiplier money.Rational `json:"express_multiplier"`
	ExpressAppliedTo  string         `json:"express_applied_to"`

	DistanceRounding       money.RoundingMode `json:"distance_rounding"`
	DistanceBasisByChannel map[string]string  `json:"distance_basis_by_channel"`
	DefaultDistanceBasis   string             `json:"default_distance_basis"`

	TotalRounding     Rounding `json:"total_rounding"`
	MinimumKobo       int64    `json:"minimum_kobo"`
	MinimumAfterRound bool     `json:"minimum_after_rounding"`
}

// Kind returns the policy kind this body belongs to.
func (DispatchTariffV1) Kind() string { return KindDispatchTariff }

// Validate reports every problem with the body at once.
func (d DispatchTariffV1) Validate() error {
	var problems []string
	if d.SchemaVersion != 1 {
		problems = append(problems, "schema_version must be 1")
	}
	if d.Currency == "" {
		problems = append(problems, "currency is required")
	}
	if d.BaseKobo < 0 || d.PerKmKobo < 0 || d.MinimumKobo < 0 {
		problems = append(problems, "base, per-km and minimum cannot be negative")
	}
	for size, addon := range d.SizeAddonsKobo {
		if addon < 0 {
			problems = append(problems, fmt.Sprintf("size addon %q cannot be negative", size))
		}
	}
	if !d.ExpressMultiplier.Valid() {
		problems = append(problems, "express_multiplier has a zero denominator")
	} else if d.ExpressMultiplier.Num < d.ExpressMultiplier.Den {
		problems = append(problems, "express_multiplier must be at least 1 (express cannot be cheaper than standard)")
	}
	switch d.ExpressAppliedTo {
	case ExpressOnSubtotalBeforeRounding, ExpressOnDistanceOnly:
	default:
		problems = append(problems, fmt.Sprintf("express_applied_to %q is unknown", d.ExpressAppliedTo))
	}
	if !d.DistanceRounding.Valid() {
		problems = append(problems, fmt.Sprintf("distance_rounding %q is unknown", d.DistanceRounding))
	}
	for channel, basis := range d.DistanceBasisByChannel {
		if !Channel(channel).Valid() {
			problems = append(problems, fmt.Sprintf("distance_basis_by_channel has unknown channel %q", channel))
		}
		if basis != DistanceBasisRoadRoute && basis != DistanceBasisStraightLine {
			problems = append(problems, fmt.Sprintf("distance basis %q for channel %q is unknown", basis, channel))
		}
	}
	if d.DefaultDistanceBasis != DistanceBasisRoadRoute && d.DefaultDistanceBasis != DistanceBasisStraightLine {
		problems = append(problems, fmt.Sprintf("default_distance_basis %q is unknown", d.DefaultDistanceBasis))
	}
	problems = append(problems, d.TotalRounding.Validate("total_rounding")...)

	if len(problems) > 0 {
		return fmt.Errorf("dispatch.tariff v1: %s", join(problems))
	}
	return nil
}

// Evaluate prices req.
func (d DispatchTariffV1) Evaluate(req Request) ([]Component, int64, []string, error) {
	basis := d.DefaultDistanceBasis
	if fromChannel, ok := d.DistanceBasisByChannel[string(req.Channel)]; ok {
		basis = fromChannel
	}
	metres, err := distanceFor(basis, req)
	if err != nil {
		return nil, 0, nil, err
	}

	size := normalizeSize(req.PackageSize)
	tier := normalizeTier(req.ServiceTier)

	b := &builder{}
	b.add(ComponentBase, d.BaseKobo, "base delivery cost")

	distanceKobo, err := money.PerUnit(d.PerKmKobo, metres, 1_000, d.DistanceRounding)
	if err != nil {
		return nil, 0, nil, err
	}
	b.add(ComponentDistance, distanceKobo, "%s metres at %s per km (%s basis)", withThousands(metres), kobo(d.PerKmKobo), strings.ToLower(basis))

	if addon := d.SizeAddonsKobo[size]; addon != 0 {
		b.add(ComponentSizeSurcharge, addon, "%s package", size)
	}
	if b.err != nil {
		return nil, 0, nil, b.err
	}

	if tier == TierExpress {
		subtotal := b.total
		if d.ExpressAppliedTo == ExpressOnDistanceOnly {
			subtotal = distanceKobo
		}
		multiplied, mulErr := money.Multiply(subtotal, d.ExpressMultiplier, money.HalfAwayFromZero)
		if mulErr != nil {
			return nil, 0, nil, mulErr
		}
		b.add(ComponentExpress, multiplied-subtotal, "express: %s applied to %s",
			d.ExpressMultiplier, strings.ToLower(strings.ReplaceAll(d.ExpressAppliedTo, "_", " ")))
	}

	components, gross, finishFlags, err := b.finish(d.TotalRounding, d.MinimumKobo, d.MinimumAfterRound)
	if err != nil {
		return nil, 0, nil, err
	}
	return components, gross, append(routeFlags(req), finishFlags...), nil
}

// normalizeSize mirrors the legacy shared package: an unrecognised size is
// treated as small rather than refused, because the legacy surfaces accept free
// text and a migration must not start rejecting requests they used to price.
func normalizeSize(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "medium":
		return "medium"
	case "large":
		return "large"
	default:
		return "small"
	}
}

// normalizeTier mirrors the legacy shared package: anything that is not express
// is standard.
func normalizeTier(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), TierExpress) {
		return TierExpress
	}
	return TierStandard
}
