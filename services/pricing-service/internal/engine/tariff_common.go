package engine

import (
	"fmt"

	"cosmicforge/logistics/services/pricing-service/internal/platform/money"
)

// Rounding is a rounding step stated by a policy: round to the nearest Step
// kobo using Mode. Step 0 or 1 means no rounding, which is how haulage is
// expressed -- it rounds its distance component but never its total.
type Rounding struct {
	StepKobo int64              `json:"step_kobo"`
	Mode     money.RoundingMode `json:"mode"`
}

// Validate checks the rounding step is usable.
func (r Rounding) Validate(field string) []string {
	var problems []string
	if r.StepKobo < 0 {
		problems = append(problems, field+".step_kobo cannot be negative")
	}
	if r.StepKobo > 1 && !r.Mode.Valid() {
		problems = append(problems, fmt.Sprintf("%s.mode %q is not a known rounding mode", field, r.Mode))
	}
	return problems
}

// apply rounds amount and returns the adjustment, so the caller can record
// ROUNDING as its own component and keep the component sum exact.
func (r Rounding) apply(amount int64) (rounded int64, adjustment int64, err error) {
	if r.StepKobo <= 1 {
		return amount, 0, nil
	}
	rounded, err = money.RoundToStep(amount, r.StepKobo, r.Mode)
	if err != nil {
		return 0, 0, err
	}
	return rounded, rounded - amount, nil
}

// builder accumulates components while keeping a running total, so the
// "components sum to gross" invariant holds by construction rather than by a
// later check.
type builder struct {
	components []Component
	total      int64
	err        error
}

func (b *builder) add(code string, amount int64, basisFormat string, args ...any) {
	if b.err != nil {
		return
	}
	if amount == 0 && code != ComponentBase {
		return // an absent surcharge is not a line on the receipt
	}
	total, err := money.Sum(b.total, amount)
	if err != nil {
		b.err = err
		return
	}
	b.total = total
	b.components = append(b.components, Component{
		Code:       code,
		AmountKobo: amount,
		Basis:      fmt.Sprintf(basisFormat, args...),
	})
}

// finish applies the rounding step and the minimum fare, recording each as its
// own component.
//
// Order matters and belongs to the policy: legacy dispatch rounds and THEN
// applies its minimum, which can leave a total that is the minimum rather than a
// multiple of the rounding step. Reproducing that ordering is the difference
// between a price-neutral migration and thousands of one-off differences.
func (b *builder) finish(rounding Rounding, minimumKobo int64, minimumAfterRounding bool) ([]Component, int64, []string, error) {
	if b.err != nil {
		return nil, 0, nil, b.err
	}
	var flags []string

	applyMinimum := func() {
		if minimumKobo > 0 && b.total < minimumKobo {
			topUp := minimumKobo - b.total
			b.add(ComponentMinimumTopUp, topUp, "raised to the %s minimum fare", kobo(minimumKobo))
			flags = append(flags, FlagMinimumApplied)
		}
	}

	if !minimumAfterRounding {
		applyMinimum()
	}

	rounded, adjustment, err := rounding.apply(b.total)
	if err != nil {
		return nil, 0, nil, err
	}
	if adjustment != 0 {
		b.components = append(b.components, Component{
			Code:       ComponentRounding,
			AmountKobo: adjustment,
			Basis:      fmt.Sprintf("rounded to the nearest %s (%s)", kobo(rounding.StepKobo), rounding.Mode),
		})
		b.total = rounded
	}

	if minimumAfterRounding {
		applyMinimum()
	}

	if b.err != nil {
		return nil, 0, nil, b.err
	}
	return b.components, b.total, flags, nil
}

// kobo renders an amount as naira for explanation text. Display only: never used
// in a calculation.
func kobo(amount int64) string {
	naira := amount / 100
	remainder := amount % 100
	if remainder == 0 {
		return fmt.Sprintf("NGN %s", withThousands(naira))
	}
	if remainder < 0 {
		remainder = -remainder
	}
	return fmt.Sprintf("NGN %s.%02d", withThousands(naira), remainder)
}

func withThousands(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	digits := fmt.Sprintf("%d", value)
	var out []byte
	for i, digit := range []byte(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digit)
	}
	if negative {
		return "-" + string(out)
	}
	return string(out)
}

// distanceFor returns the distance a policy should price with, honouring its
// stated basis. STRAIGHT_LINE policies are priced from the straight-line distance
// the caller supplies, which is how several legacy surfaces behave; the caller
// computes it deterministically from whole-metre coordinates.
func distanceFor(basis string, req Request) (int64, error) {
	switch basis {
	case DistanceBasisRoadRoute, DistanceBasisStraightLine, "":
		return req.DistanceMeters, nil
	default:
		return 0, fmt.Errorf("engine: unknown distance basis %q", basis)
	}
}

// routeFlags records what the route the price rests on was actually like.
func routeFlags(req Request) []string {
	var flags []string
	if req.RouteDegraded {
		flags = append(flags, FlagRouteDegraded)
	}
	if req.RouteConfidence == RouteConfidenceLow {
		flags = append(flags, FlagRouteLowConf)
	}
	return flags
}
