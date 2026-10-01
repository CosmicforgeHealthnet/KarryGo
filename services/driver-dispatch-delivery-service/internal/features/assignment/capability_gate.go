package assignment

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/parcelfit"
	"cosmicforge/logistics/shared/go/capabilityfit"
	"cosmicforge/logistics/shared/go/logging"
)

// CapabilityAuthority is Dispatch's own bike-capability store, seen from
// assignment.
//
// It is an interface for the usual reason (assignment must not import the
// whole capability feature) and one specific one: LoadProviderBikes returning
// an error is what lets the gate tell a store outage from an empty fleet. A
// concrete call returning an empty map on failure would reproduce the
// fail-open this gate exists to close.
type CapabilityAuthority interface {
	// LoadProviderBikes returns every active bike, with capability, for each
	// provider, with product eligibility already projected from the
	// requirement. A provider absent from the map has no active bike; an error
	// means the store could not answer at all.
	LoadProviderBikes(ctx context.Context, providerIDs []string, requirement parcelfit.Requirement) (map[string][]parcelfit.Capability, error)

	// RecordDecisions persists hard-fit evidence. Failures are swallowed by
	// the caller: an assignment must never fail because an operations table
	// was unavailable.
	RecordDecisions(ctx context.Context, decisions []CapabilityDecision) error

	// PolicyVersion is the locally-snapshotted policy this evaluation ran
	// against.
	PolicyVersion(ctx context.Context) string

	// ResolveProduct names the service product a requirement runs under and
	// whether this deployment demands a certificate for it. That is
	// deployment policy, not a property of the requirement, so the authority
	// answers it.
	ResolveProduct(requirement parcelfit.Requirement) (productCode string, certificationRequired bool)
}

// CapabilityDecision is one candidate's recorded outcome.
type CapabilityDecision struct {
	MissionID         string
	ProviderID        string
	BikeID            string
	Decision          string
	ReasonCodes       string
	Evidence          map[string]string
	DistanceM         int64
	CapabilityVersion int64
	PolicyVersion     string
	Shadow            bool
}

// Decision outcomes.
const (
	decisionEligible = "ELIGIBLE"
	decisionRejected = "REJECTED"
)

// CapabilityConfig carries the Section 8.12 flag pair.
//
// The pair expresses three states, and the middle one is the point:
//
//	Enabled=false                     legacy bike_type matching only
//	Enabled=true,  Enforce=false      SHADOW: decide, record, do not enforce
//	Enabled=true,  Enforce=true       ENFORCED: hard fit selects
//
// Shadow exists because switching hard fit on blind would, on a fleet that has
// never been surveyed, refuse nearly every candidate as CAPABILITY_UNKNOWN and
// be indistinguishable from a total outage.
type CapabilityConfig struct {
	Enabled bool
	Enforce bool
}

// Shadow reports whether decisions are computed but not enforced.
func (c CapabilityConfig) Shadow() bool { return c.Enabled && !c.Enforce }

// CapabilityGate applies hard parcel fit to a candidate set.
type CapabilityGate struct {
	authority CapabilityAuthority
	config    CapabilityConfig
	events    *CapabilityEventSink
}

// NewCapabilityGate builds the gate. A nil authority makes every method a
// pass-through, so a deployment that has not enabled Section 8 behaves exactly
// as it did before.
func NewCapabilityGate(authority CapabilityAuthority, config CapabilityConfig) *CapabilityGate {
	return &CapabilityGate{authority: authority, config: config}
}

// WithEvents attaches the capability-fact sink. It is a separate call so every
// existing construction site keeps compiling and simply publishes nothing.
func (g *CapabilityGate) WithEvents(sink *CapabilityEventSink) *CapabilityGate {
	if g != nil {
		g.events = sink
	}
	return g
}

// Active reports whether the gate will do anything at all.
func (g *CapabilityGate) Active() bool {
	return g != nil && g.authority != nil && g.config.Enabled
}

// Filter removes the candidates whose bikes cannot carry the parcel.
//
// It returns the survivors and the evidence for every candidate considered. In
// shadow mode the caller keeps its legacy list and only stores the evidence,
// which is what makes the rollout measurable without changing a single
// assignment.
//
// Hard eligibility and ranking stay separate: Filter only ever REMOVES from an
// already-ordered list, so a nearer rider can never be scored back into a job
// their bike cannot do.
func (g *CapabilityGate) Filter(
	ctx context.Context,
	missionID string,
	requirement parcelfit.Requirement,
	candidates []Candidate,
) (eligible []Candidate, decisions []CapabilityDecision) {
	if !g.Active() || len(candidates) == 0 {
		return candidates, nil
	}

	providerIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		providerIDs = append(providerIDs, candidate.ProviderID)
	}

	requirement.ProductCode, requirement.CertificationRequired = g.authority.ResolveProduct(requirement)

	resolution := parcelfit.ResolutionFound
	byProvider, err := g.authority.LoadProviderBikes(ctx, providerIDs, requirement)
	if err != nil {
		// The store did not answer. Every candidate is therefore
		// DEPENDENCY_UNAVAILABLE -- not "too heavy", not "wrong bike". The
		// legacy SQL had no way to express this at all: an unreachable store
		// simply meant the EXISTS clause failed and the rider vanished from
		// the pool with no explanation.
		resolution = parcelfit.ResolutionFailed
		byProvider = nil
		observability.CapabilityLookupFailure()
		logging.Error("dispatch-capability", "bike capability lookup failed for mission %s: %v", missionID, err)
	}

	policyVersion := g.authority.PolicyVersion(ctx)
	shadow := g.config.Shadow()

	for _, candidate := range candidates {
		bike, decision := evaluateProvider(byProvider[candidate.ProviderID], resolution, requirement)

		outcome := decisionRejected
		if decision.Eligible {
			outcome = decisionEligible
			eligible = append(eligible, candidate)
		}

		observability.CapabilityMismatch(decision.PrimaryReason(), shadow)

		decisions = append(decisions, CapabilityDecision{
			MissionID:         missionID,
			ProviderID:        candidate.ProviderID,
			BikeID:            bike.BikeID,
			Decision:          outcome,
			ReasonCodes:       decision.ReasonString(),
			Evidence:          decision.Details,
			DistanceM:         candidate.DistanceM,
			CapabilityVersion: bike.CapabilityVersion,
			PolicyVersion:     policyVersion,
			Shadow:            shadow,
		})
	}

	observability.EligibleCandidates(len(candidates), len(eligible), shadow)
	return eligible, decisions
}

// Apply filters and records in one call, returning the list the caller should
// actually use.
//
// In shadow mode it returns the ORIGINAL list. That asymmetry is the whole
// point of the mode and is kept in one place rather than at each call site, so
// no caller can accidentally enforce a decision that was only meant to be
// measured.
func (g *CapabilityGate) Apply(
	ctx context.Context,
	missionID string,
	requirement parcelfit.Requirement,
	candidates []Candidate,
) []Candidate {
	if !g.Active() {
		return candidates
	}
	eligible, decisions := g.Filter(ctx, missionID, requirement, candidates)
	g.Record(ctx, decisions)

	if !g.config.Enforce {
		if len(eligible) < len(candidates) {
			logging.Notice("dispatch-capability",
				"shadow: mission %s would have lost %d of %d candidates (%s)",
				missionID, len(candidates)-len(eligible), len(candidates), shortfall(decisions))
		}
		return candidates
	}
	if len(eligible) < len(candidates) {
		logging.Notice("dispatch-capability",
			"mission %s: %d of %d candidates failed hard parcel fit (%s)",
			missionID, len(candidates)-len(eligible), len(candidates), shortfall(decisions))
	}
	// Only a mission that found NOBODY is worth an event. A mission that lost
	// three of forty candidates matched fine, and publishing that would drown
	// the signal that actually needs acting on.
	if len(eligible) == 0 && len(candidates) > 0 {
		g.events.PublishMismatch(ctx, missionID, len(candidates), shortfall(decisions))
	}
	return eligible
}

// Allows reports whether one specific provider may take this parcel.
//
// It exists for the validation path, where a provider has already been named
// (an accept, a direct assignment) and the question is not "who fits" but
// "does this one". It runs the same engine, so an accept cannot bypass a gate
// that discovery would have applied.
func (g *CapabilityGate) Allows(
	ctx context.Context,
	missionID string,
	requirement parcelfit.Requirement,
	providerID string,
) (bool, string) {
	if !g.Active() || !g.config.Enforce {
		return true, capabilityfit.ReasonEligible
	}
	eligible, decisions := g.Filter(ctx, missionID, requirement,
		[]Candidate{{ProviderID: providerID}})
	g.Record(ctx, decisions)
	if len(eligible) > 0 {
		return true, capabilityfit.ReasonEligible
	}
	if len(decisions) > 0 {
		return false, decisions[0].ReasonCodes
	}
	return false, capabilityfit.ReasonNoEligibleProvider
}

// Record persists evidence without ever failing an assignment.
func (g *CapabilityGate) Record(ctx context.Context, decisions []CapabilityDecision) {
	if !g.Active() || len(decisions) == 0 {
		return
	}
	// A detached context: the caller's context may be cancelled the moment a
	// rider accepts, and losing the evidence for the assignment that just
	// succeeded is exactly the case an operator most wants to read.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := g.authority.RecordDecisions(writeCtx, decisions); err != nil {
		logging.Error("dispatch-capability", "record match decisions: %v", err)
	}
}

// evaluateProvider runs hard fit for one provider across all of their active
// bikes.
//
// A provider is eligible if ANY of their bikes fits, because the rider brings
// one machine and gets to choose which. The reasons returned are those of
// their best attempt, so an operator sees why the closest thing to a fit still
// failed rather than an arbitrary bike's complaints.
func evaluateProvider(
	bikes []parcelfit.Capability,
	resolution parcelfit.Resolution,
	requirement parcelfit.Requirement,
) (parcelfit.Capability, *capabilityfit.Decision) {
	if resolution != parcelfit.ResolutionFound || len(bikes) == 0 {
		// Which of the two this is matters: a store that did not answer is a
		// dependency problem, an empty list is a provider with no active bike.
		if resolution == parcelfit.ResolutionFound {
			resolution = parcelfit.ResolutionMissing
		}
		return parcelfit.Capability{},
			parcelfit.Evaluate(resolution, parcelfit.Capability{}, requirement)
	}

	var best *capabilityfit.Decision
	var bestBike parcelfit.Capability
	for _, bike := range bikes {
		candidate := parcelfit.Evaluate(parcelfit.ResolutionFound, bike, requirement)
		if candidate.Eligible {
			return bike, candidate
		}
		if best == nil || len(candidate.Reasons) < len(best.Reasons) {
			best, bestBike = candidate, bike
		}
	}
	return bestBike, best
}

// shortfall summarizes why a candidate set emptied out, commonest first.
func shortfall(decisions []CapabilityDecision) string {
	counts := map[string]int{}
	for _, decision := range decisions {
		if decision.Decision != decisionRejected {
			continue
		}
		for _, reason := range strings.Split(decision.ReasonCodes, ",") {
			if reason != "" {
				counts[reason]++
			}
		}
	}
	if len(counts) == 0 {
		return ""
	}
	reasons := make([]string, 0, len(counts))
	for reason := range counts {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if counts[reasons[i]] != counts[reasons[j]] {
			return counts[reasons[i]] > counts[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	return strings.Join(reasons, ",")
}

// RequirementFromMissionSnapshot builds a parcel requirement from the
// aggregated parcel rows a commerce mission already carries.
//
// The mission has stored weight, volume, dimensions, package count and a
// handling snapshot per parcel since migration 000042. Nothing has ever read
// them for eligibility; this is where that starts. Dimensions are the MAXIMUM
// of the parcels rather than the sum, because a carrier has to fit the biggest
// single item, while weight and volume aggregate because they accumulate.
func RequirementFromMissionSnapshot(parcels []ParcelSnapshot) parcelfit.Requirement {
	requirement := parcelfit.Requirement{}
	for _, parcel := range parcels {
		requirement.WeightGrams += parcel.WeightGrams
		requirement.VolumeMl += parcel.VolumeMl
		requirement.PackageCount += parcel.PackageCount
		requirement.LengthMm = maxInt64(requirement.LengthMm, parcel.LengthMm)
		requirement.WidthMm = maxInt64(requirement.WidthMm, parcel.WidthMm)
		requirement.HeightMm = maxInt64(requirement.HeightMm, parcel.HeightMm)
		applyHandling(&requirement, parcel.Handling)
	}
	return requirement
}

// ParcelSnapshot is one parcel row, as assignment needs it.
type ParcelSnapshot struct {
	WeightGrams  int64
	VolumeMl     int64
	LengthMm     int64
	WidthMm      int64
	HeightMm     int64
	PackageCount int64
	// Handling is the parcel's handling_snapshot JSONB, decoded lazily so a
	// malformed snapshot degrades to "no special handling" rather than failing
	// the whole assignment.
	Handling json.RawMessage
}

// applyHandling folds a parcel's handling snapshot into the requirement.
//
// Handling only ever ADDS constraints across parcels: one fragile item in a
// consignment makes the whole consignment fragile, because the rider carries
// them together. A snapshot that cannot be decoded contributes nothing, which
// is the safe direction only because the physical bounds above are unaffected.
func applyHandling(requirement *parcelfit.Requirement, snapshot json.RawMessage) {
	if len(snapshot) == 0 {
		return
	}
	var handling struct {
		Fragile           bool     `json:"fragile"`
		Upright           bool     `json:"upright"`
		UprightRequired   bool     `json:"upright_required"`
		WeatherProtection bool     `json:"weather_protection_required"`
		Enclosed          bool     `json:"enclosed_required"`
		TemperatureClass  string   `json:"temperature_class"`
		SecureHandover    bool     `json:"secure_handover"`
		Codes             []string `json:"codes"`
	}
	if err := json.Unmarshal(snapshot, &handling); err != nil {
		return
	}

	requirement.Fragile = requirement.Fragile || handling.Fragile
	requirement.UprightRequired = requirement.UprightRequired || handling.Upright || handling.UprightRequired
	requirement.WeatherProtectionRequired = requirement.WeatherProtectionRequired || handling.WeatherProtection
	requirement.EnclosureRequired = requirement.EnclosureRequired || handling.Enclosed
	requirement.SecureHandover = requirement.SecureHandover || handling.SecureHandover

	// A fragile parcel must stay the way it was loaded. Inferring upright from
	// fragile can only narrow the candidate set, which is the safe direction.
	if handling.Fragile {
		requirement.UprightRequired = true
	}

	for _, code := range handling.Codes {
		switch strings.ToUpper(strings.TrimSpace(code)) {
		case "FRAGILE":
			requirement.Fragile = true
			requirement.UprightRequired = true
		case "UPRIGHT":
			requirement.UprightRequired = true
		case "CHILLED":
			requirement.TemperatureClass = coldest(requirement.TemperatureClass, "CHILLED")
		case "FROZEN":
			requirement.TemperatureClass = coldest(requirement.TemperatureClass, "FROZEN")
		case "WEATHER_PROTECTED":
			requirement.WeatherProtectionRequired = true
		case "ENCLOSED", "HIGH_VALUE":
			requirement.EnclosureRequired = true
		}
	}
	if class := strings.ToUpper(strings.TrimSpace(handling.TemperatureClass)); class != "" {
		requirement.TemperatureClass = coldest(requirement.TemperatureClass, class)
	}
}

// coldest keeps the strictest temperature requirement across parcels. A
// consignment with one frozen item must be carried frozen; taking the last
// parcel's class instead would silently thaw it.
func coldest(current, candidate string) string {
	rank := map[string]int{"": 0, "AMBIENT": 1, "CHILLED": 2, "FROZEN": 3}
	if rank[candidate] > rank[current] {
		return candidate
	}
	return current
}

func maxInt64(a, b int64) int64 {
	if b > a {
		return b
	}
	return a
}
