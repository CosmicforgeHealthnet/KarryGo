package engine

import (
	"fmt"

	"cosmicforge/logistics/services/pricing-service/internal/platform/money"
)

// Duration units a taxi policy may charge by.
const (
	DurationUnitWholeMinutesFloor = "WHOLE_MINUTES_FLOOR"
	DurationUnitSeconds           = "SECONDS"
)

// Time bases. A policy that charges TRAFFIC_DURATION must not also carry a
// traffic market-pressure signal, or traffic is charged for twice -- one of the
// named risks in the pricing architecture.
const (
	TimeBasisRouteDuration   = "ROUTE_DURATION"
	TimeBasisTrafficDuration = "TRAFFIC_DURATION"
)

// How a policy treats a route it cannot trust.
const (
	LowConfidenceAllowWithFlag = "ALLOW_WITH_FLAG"
	LowConfidenceRefuse        = "REFUSE"
)

// TaxiClassRate is one class's complete rate set. Each taxi class carries its own
// base, distance, time and minimum rather than a multiplier off a shared base:
// premium classes differ in their time economics, not just their headline price,
// and the existing taxi design already treats classes as independent rate sets.
type TaxiClassRate struct {
	BaseKobo   int64 `json:"base_kobo"`
	PerKmKobo  int64 `json:"per_km_kobo"`
	PerMinKobo int64 `json:"per_min_kobo"`
	MinKobo    int64 `json:"min_kobo"`
}

// TaxiTariffV1 is schema version 1 of a taxi tariff.
//
// Its defaults are chosen to reproduce the legacy provisional provider exactly:
// truncated distance, whole floored minutes, round half-up to 50 kobo, then the
// class minimum. Every one of those choices is stated rather than implied,
// because the three services each rounded differently and a migration that
// guesses would move real prices.
type TaxiTariffV1 struct {
	SchemaVersion int                      `json:"schema_version"`
	Currency      string                   `json:"currency"`
	Classes       map[string]TaxiClassRate `json:"classes"`

	DistanceBasis    string             `json:"distance_basis"`
	DistanceRounding money.RoundingMode `json:"distance_rounding"`
	DurationUnit     string             `json:"duration_unit"`
	TimeBasis        string             `json:"time_basis"`

	TotalRounding      Rounding `json:"total_rounding"`
	MinimumAfterRound  bool     `json:"minimum_after_rounding"`
	LowConfidenceRoute string   `json:"low_confidence_route"`

	// HourlyMode records what the policy does about hourly reservations. The
	// legacy service prices them point to point and ignores the reserved
	// duration; PRICE_AS_POINT_TO_POINT states that gap openly instead of hiding
	// it, so it can be closed deliberately in a later schema version.
	HourlyMode string `json:"hourly_mode"`
}

// Kind returns the policy kind this body belongs to.
func (TaxiTariffV1) Kind() string { return KindTaxiTariff }

// Validate reports every problem with the body at once.
func (t TaxiTariffV1) Validate() error {
	var problems []string
	if t.SchemaVersion != 1 {
		problems = append(problems, "schema_version must be 1")
	}
	if t.Currency == "" {
		problems = append(problems, "currency is required")
	}
	if len(t.Classes) == 0 {
		problems = append(problems, "at least one class is required")
	}
	for name, rate := range t.Classes {
		if rate.BaseKobo < 0 || rate.PerKmKobo < 0 || rate.PerMinKobo < 0 || rate.MinKobo < 0 {
			problems = append(problems, fmt.Sprintf("class %q has a negative rate", name))
		}
	}
	switch t.DistanceBasis {
	case DistanceBasisRoadRoute, DistanceBasisStraightLine:
	default:
		problems = append(problems, fmt.Sprintf("distance_basis %q is unknown", t.DistanceBasis))
	}
	if !t.DistanceRounding.Valid() {
		problems = append(problems, fmt.Sprintf("distance_rounding %q is unknown", t.DistanceRounding))
	}
	switch t.DurationUnit {
	case DurationUnitWholeMinutesFloor, DurationUnitSeconds:
	default:
		problems = append(problems, fmt.Sprintf("duration_unit %q is unknown", t.DurationUnit))
	}
	switch t.TimeBasis {
	case TimeBasisRouteDuration, TimeBasisTrafficDuration:
	default:
		problems = append(problems, fmt.Sprintf("time_basis %q is unknown", t.TimeBasis))
	}
	switch t.LowConfidenceRoute {
	case LowConfidenceAllowWithFlag, LowConfidenceRefuse:
	default:
		problems = append(problems, fmt.Sprintf("low_confidence_route %q is unknown", t.LowConfidenceRoute))
	}
	problems = append(problems, t.TotalRounding.Validate("total_rounding")...)

	if len(problems) > 0 {
		return fmt.Errorf("taxi.tariff v1: %s", join(problems))
	}
	return nil
}

// Evaluate prices req.
func (t TaxiTariffV1) Evaluate(req Request) ([]Component, int64, []string, error) {
	rate, ok := t.Classes[req.Class]
	if !ok {
		return nil, 0, nil, fmt.Errorf("%w: taxi class %q has no rates in this policy", ErrUnsupported, req.Class)
	}

	flags := routeFlags(req)
	if t.LowConfidenceRoute == LowConfidenceRefuse && (req.RouteDegraded || req.RouteConfidence == RouteConfidenceLow) {
		return nil, 0, nil, fmt.Errorf("%w: this policy refuses to price an untrusted route", ErrUnsupported)
	}

	metres, err := distanceFor(t.DistanceBasis, req)
	if err != nil {
		return nil, 0, nil, err
	}

	seconds := req.DurationSeconds
	if t.TimeBasis == TimeBasisTrafficDuration && req.TrafficDurationSeconds > 0 {
		seconds = req.TrafficDurationSeconds
	}

	b := &builder{}
	b.add(ComponentBase, rate.BaseKobo, "base fare for %s", req.Class)

	distanceKobo, err := money.PerUnit(rate.PerKmKobo, metres, 1_000, t.DistanceRounding)
	if err != nil {
		return nil, 0, nil, err
	}
	b.add(ComponentDistance, distanceKobo, "%s metres at %s per km (%s)", withThousands(metres), kobo(rate.PerKmKobo), t.DistanceRounding)

	var timeKobo int64
	switch t.DurationUnit {
	case DurationUnitWholeMinutesFloor:
		minutes, minErr := money.PerUnit(seconds, 1, 60, money.Floor)
		if minErr != nil {
			return nil, 0, nil, minErr
		}
		// Multiplied through money rather than with * so the product is
		// overflow-checked like every other amount.
		timeKobo, err = money.PerUnit(rate.PerMinKobo, minutes, 1, money.Truncate)
		if err != nil {
			return nil, 0, nil, err
		}
		b.add(ComponentTime, timeKobo, "%d whole minutes at %s per minute (%d seconds does not buy a further minute)",
			minutes, kobo(rate.PerMinKobo), seconds-minutes*60)
	case DurationUnitSeconds:
		timeKobo, err = money.PerUnit(rate.PerMinKobo, seconds, 60, money.HalfAwayFromZero)
		if err != nil {
			return nil, 0, nil, err
		}
		b.add(ComponentTime, timeKobo, "%s seconds at %s per minute", withThousands(seconds), kobo(rate.PerMinKobo))
	}

	components, gross, finishFlags, err := b.finish(t.TotalRounding, rate.MinKobo, t.MinimumAfterRound)
	if err != nil {
		return nil, 0, nil, err
	}
	return components, gross, append(flags, finishFlags...), nil
}
