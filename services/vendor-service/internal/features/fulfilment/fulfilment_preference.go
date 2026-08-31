package fulfilment

// The customer's fulfilment preference, and the policy each preference is
// decided under.
//
// Three modes, one meaning each:
//
//   - FASTEST   minimise time to the last delivery.
//   - BALANCED  the default trade-off between time and delivered cost.
//   - SAVER     minimise delivered cost inside the window the customer accepted.
//
// A mode is a CHOICE, never a price. It selects among candidates the server
// computed and quoted; it can no more set a fee than a customer can. That is
// why it is the one value in this package's request contract that may come
// from a customer request body at all.
//
// Every threshold a mode decides against lives here, versioned. Thresholds
// move - a wider Saver window, a tighter Fastest tolerance - and a decision
// taken last month has to stay explainable after they do. Scattering the
// numbers through the comparison code is how that stops being possible.

import (
	"strings"
	"time"
)

// DefaultMode is BALANCED.
//
// Not FASTEST, which would quietly buy the customer the most expensive plan
// they never asked for, and not SAVER, which would quietly make them wait.
const DefaultMode = ModeBalanced

// ParseMode resolves a customer-supplied preference.
//
// An empty value is the default rather than an error: most callers state no
// preference, and refusing them would make the default unreachable. An
// unrecognised value IS an error - silently downgrading an unknown mode to
// BALANCED would hide a client bug behind a plausible plan.
func ParseMode(value string) (Mode, bool) {
	trimmed := strings.ToUpper(strings.TrimSpace(value))

	switch trimmed {
	case "":
		return DefaultMode, true
	case string(ModeFastest):
		return ModeFastest, true
	case string(ModeBalanced):
		return ModeBalanced, true
	case string(ModeSaver):
		return ModeSaver, true
	default:
		return "", false
	}
}

// Selectable reports whether a mode may be requested by a customer.
//
// SINGLE is deliberately excluded. It is the historical value meaning "the
// optimizer produced exactly one plan"; a customer asking for it would be
// asking for an implementation detail.
func (m Mode) Selectable() bool {
	return m == ModeFastest || m == ModeBalanced || m == ModeSaver
}

// ModePolicy is the decision envelope for one mode.
//
// Every field is a TOLERANCE, not an objective. Objectives are expressed by
// the comparison in plan_comparison.go; these are the bounds inside which a
// candidate is allowed to be considered at all.
type ModePolicy struct {
	// ReadyGapTolerance is how far apart two vendors ready times may be
	// before one vendor is being made to wait for another.
	//
	// It is the whole reason SAVER can consolidate where FASTEST cannot: the
	// physical facts are identical, and only the customer's accepted patience
	// differs.
	ReadyGapTolerance time.Duration

	// DetourTolerance is how much longer a consolidated mission may take than
	// the slowest leg of the equivalent split plan.
	//
	// It is measured against the SPLIT alternative's own quoted transit, not
	// against an estimate, because both alternatives are quoted before either
	// is compared.
	DetourTolerance time.Duration

	// ETASlackTolerance is how far past a stated promise a candidate may
	// still be considered. It is zero for every mode: a promise the customer
	// was given is not a threshold to be traded away, and a mode that could
	// widen it would let SAVER buy cheapness with a broken commitment.
	ETASlackTolerance time.Duration

	// MaterialCostSavingMinor is the delivered-cost difference below which
	// two candidates are treated as equally priced.
	//
	// Without it BALANCED would chase one kobo of saving into an hour of
	// delay, which is not a balance.
	MaterialCostSavingMinor int64

	// MaterialTimeSaving is the completion-time difference below which two
	// candidates are treated as equally fast.
	MaterialTimeSaving time.Duration
}

// OptimizationPolicy is the versioned set of every mode's envelope.
//
// The version is recorded on every candidate. A candidate explains itself
// against the thresholds it was actually decided under, never against
// today's.
type OptimizationPolicy struct {
	Version int64 `json:"version"`

	Fastest  ModePolicy `json:"-"`
	Balanced ModePolicy `json:"-"`
	Saver    ModePolicy `json:"-"`

	// ProximityRadiusMeters bounds CANDIDATE CONSTRUCTION: two pickups
	// further apart than this are not operationally related enough to be
	// worth evaluating as one mission at all.
	//
	// It is deliberately much wider than the co-location radius. A candidate
	// inside it may still be rejected - most are, today, because no logistics
	// runtime can drive a multi-stop collection - but rejecting it WITH a
	// reason is different from never considering it. The first is an answer
	// Support can give; the second is silence.
	ProximityRadiusMeters int `json:"proximity_radius_meters"`

	// ColocationRadiusMeters is the tighter bound inside which several
	// vendors genuinely share ONE physical collection point.
	//
	// This is the distinction the whole multi-stop gate turns on: two kitchens
	// in one food court are one stop, and two shops a kilometre apart are two
	// stops however close a map makes them look.
	ColocationRadiusMeters int `json:"colocation_radius_meters"`
}

// DefaultOptimizationPolicyVersion is bumped whenever any threshold below
// changes, so stored evidence keeps pointing at the numbers it was decided
// under.
const DefaultOptimizationPolicyVersion = 1

// DefaultOptimizationPolicy is the platform envelope.
func DefaultOptimizationPolicy() OptimizationPolicy {
	return OptimizationPolicy{
		Version: DefaultOptimizationPolicyVersion,

		Fastest: ModePolicy{
			ReadyGapTolerance:       2 * time.Minute,
			DetourTolerance:         3 * time.Minute,
			MaterialCostSavingMinor: 0,
			MaterialTimeSaving:      time.Minute,
		},

		Balanced: ModePolicy{
			ReadyGapTolerance:       10 * time.Minute,
			DetourTolerance:         12 * time.Minute,
			MaterialCostSavingMinor: 20000,
			MaterialTimeSaving:      5 * time.Minute,
		},

		Saver: ModePolicy{
			ReadyGapTolerance:       30 * time.Minute,
			DetourTolerance:         35 * time.Minute,
			MaterialCostSavingMinor: 5000,
			MaterialTimeSaving:      15 * time.Minute,
		},

		ProximityRadiusMeters:  3000,
		ColocationRadiusMeters: DefaultConsolidationRadiusMeters,
	}
}

// For returns the envelope for one mode.
//
// An unknown mode gets BALANCED. That is safe precisely because it is the
// most conservative of the three: it neither buys speed the customer did not
// ask for nor spends patience they did not offer.
func (p OptimizationPolicy) For(mode Mode) ModePolicy {
	switch mode {
	case ModeFastest:
		return p.Fastest
	case ModeSaver:
		return p.Saver
	default:
		return p.Balanced
	}
}

// Normalized fills in any threshold left at zero from the platform default,
// so a partially-configured policy is completed rather than silently applied
// with a zero tolerance that would reject every consolidation.
func (p OptimizationPolicy) Normalized() OptimizationPolicy {
	base := DefaultOptimizationPolicy()

	if p.Version <= 0 {
		p.Version = base.Version
	}

	p.Fastest = normalizeModePolicy(p.Fastest, base.Fastest)
	p.Balanced = normalizeModePolicy(p.Balanced, base.Balanced)
	p.Saver = normalizeModePolicy(p.Saver, base.Saver)

	if p.ProximityRadiusMeters <= 0 {
		p.ProximityRadiusMeters = base.ProximityRadiusMeters
	}

	if p.ColocationRadiusMeters <= 0 {
		p.ColocationRadiusMeters = base.ColocationRadiusMeters
	}

	// A co-location radius wider than the proximity radius would make every
	// candidate a single stop by arithmetic rather than by geography, which
	// is exactly the lie the multi-stop gate exists to prevent.
	if p.ColocationRadiusMeters > p.ProximityRadiusMeters {
		p.ColocationRadiusMeters = p.ProximityRadiusMeters
	}

	return p
}

func normalizeModePolicy(policy, base ModePolicy) ModePolicy {
	if policy.ReadyGapTolerance <= 0 {
		policy.ReadyGapTolerance = base.ReadyGapTolerance
	}

	if policy.DetourTolerance <= 0 {
		policy.DetourTolerance = base.DetourTolerance
	}

	if policy.MaterialCostSavingMinor < 0 {
		policy.MaterialCostSavingMinor = base.MaterialCostSavingMinor
	}

	if policy.MaterialTimeSaving <= 0 {
		policy.MaterialTimeSaving = base.MaterialTimeSaving
	}

	// ETASlackTolerance is never defaulted upwards. Zero is the intended
	// value and the only correct one.
	policy.ETASlackTolerance = 0

	return policy
}
