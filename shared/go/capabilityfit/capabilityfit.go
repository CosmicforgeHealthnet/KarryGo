// Package capabilityfit holds the vocabulary and the primitives that both
// Dispatch and Hauling use to decide whether one concrete asset can physically
// carry one concrete load.
//
// The two services ask very different questions -- a courier box against a
// parcel, a truck body against a pallet -- so the RULES live in each service.
// What lives here is the part that must not diverge:
//
//  1. the stable reason codes a refusal resolves to, so an operator reading
//     Dispatch evidence and Hauling evidence sees one vocabulary; and
//
//  2. Tri, the three-valued capability fact, plus the checks built on it.
//
// Tri is the load-bearing idea. The failure this package exists to prevent is
// an unknown capability being read as a satisfied one: a bike whose thermal
// bag was never recorded is not a bike without a thermal bag, and a truck
// whose payload was never measured is not a truck that can carry anything. A
// plain bool cannot express that difference, so every service that used one
// eventually shipped a match it could not physically honour. Tri makes the
// third state unavoidable at the type level, and the Require* helpers turn it
// into a refusal rather than a silent pass.
package capabilityfit

import "strings"

// Stable reason codes. These are the Appendix B vocabulary for physical fit
// and asset eligibility, shared across every logistics authority.
//
// They are deliberately distinguishable from one another in the way an
// operator actually needs. In particular the first three are three genuinely
// different situations that a single "not eligible" would flatten:
//
//   - REQUIRED_CAPABILITY_UNAVAILABLE: we know the asset, and it cannot.
//   - CAPABILITY_UNKNOWN: we know the asset, and we do not know whether it can.
//   - DEPENDENCY_UNAVAILABLE: we could not even find out about the asset.
//
// Only the first is a permanent fact about the asset. Treating the other two
// as permanent is how a data gap or an outage becomes "this vehicle can never
// do this job" in an operator's mind.
const (
	// ReasonEligible is the success code.
	ReasonEligible = "ELIGIBLE"

	// ReasonRequiredCapabilityUnavailable means a declared capability fact
	// says the asset cannot meet a hard requirement.
	ReasonRequiredCapabilityUnavailable = "REQUIRED_CAPABILITY_UNAVAILABLE"

	// ReasonCapabilityUnknown means a capability the requirement depends on
	// has never been recorded for this asset. It fails closed.
	ReasonCapabilityUnknown = "CAPABILITY_UNKNOWN"

	// ReasonDependencyUnavailable means a service or store needed to answer
	// did not respond. It is never a statement about the asset.
	ReasonDependencyUnavailable = "DEPENDENCY_UNAVAILABLE"

	// ReasonCapacityExceeded means a declared numeric bound was exceeded.
	ReasonCapacityExceeded = "CAPACITY_EXCEEDED"

	// ReasonCargoFitFailed means the load does not fit the carrier's usable
	// space or openings.
	ReasonCargoFitFailed = "CARGO_FIT_FAILED"

	// ReasonWrongAsset means the asset is of a class or body family the
	// requirement excludes.
	ReasonWrongAsset = "WRONG_ASSET"

	// ReasonAssetNotCertified means the asset holds no valid eligibility for
	// the service product this work requires.
	ReasonAssetNotCertified = "ASSET_NOT_CERTIFIED"

	// ReasonNetworkProductUnavailable means no active product covers the
	// request at all.
	ReasonNetworkProductUnavailable = "NETWORK_PRODUCT_UNAVAILABLE"

	// ReasonNoEligibleProvider means candidates existed and none passed hard
	// eligibility.
	ReasonNoEligibleProvider = "NO_ELIGIBLE_PROVIDER"

	// ReasonServiceNotServiceable means the location is outside any market
	// this authority operates.
	ReasonServiceNotServiceable = "SERVICE_NOT_SERVICEABLE"

	// ReasonSupplyCurrentlyUnavailable means no candidate supply exists right
	// now, independent of whether any would have fitted.
	ReasonSupplyCurrentlyUnavailable = "SUPPLY_CURRENTLY_UNAVAILABLE"

	// ReasonProviderNotEligible covers non-physical provider gates:
	// suspension, incomplete onboarding, failed compliance, restriction.
	ReasonProviderNotEligible = "PROVIDER_NOT_ELIGIBLE"

	// ReasonGuaranteeBreachDetected marks work whose service guarantee has
	// already been broken.
	ReasonGuaranteeBreachDetected = "GUARANTEE_BREACH_DETECTED"

	// ReasonRecoveryInProgress / ReasonRecoveryRequiresOperator /
	// ReasonProviderReplaced describe recovery outcomes.
	ReasonRecoveryInProgress       = "RECOVERY_IN_PROGRESS"
	ReasonRecoveryRequiresOperator = "RECOVERY_REQUIRES_OPERATOR"
	ReasonProviderReplaced         = "PROVIDER_REPLACED"

	// ReasonReconciliationRequired marks an outcome that must be read back
	// from the authoritative side before anything is retried.
	ReasonReconciliationRequired = "RECONCILIATION_REQUIRED"
)

// Tri is a capability fact that may genuinely not be known.
//
// The zero value is Unknown on purpose: a struct field nobody has populated
// must not default to "yes, it can". Every fit rule in this package treats
// Unknown as a refusal when the requirement is hard, so forgetting to set a
// field costs a match rather than a delivery.
type Tri int8

const (
	// Unknown means the fact has never been recorded.
	Unknown Tri = 0
	// Yes means the asset has the capability.
	Yes Tri = 1
	// No means the asset provably does not.
	No Tri = -1
)

// TriFromPointer converts a nullable database boolean into a Tri, which is the
// only safe translation: SQL NULL and false are different claims, and a
// scanner that collapses them destroys exactly the distinction this type
// exists to preserve.
func TriFromPointer(value *bool) Tri {
	if value == nil {
		return Unknown
	}
	if *value {
		return Yes
	}
	return No
}

// Pointer converts back for persistence.
func (t Tri) Pointer() *bool {
	switch t {
	case Yes:
		yes := true
		return &yes
	case No:
		no := false
		return &no
	default:
		return nil
	}
}

// Known reports whether the fact was recorded either way.
func (t Tri) Known() bool { return t != Unknown }

// String renders the three states for evidence and logs.
func (t Tri) String() string {
	switch t {
	case Yes:
		return "YES"
	case No:
		return "NO"
	default:
		return "UNKNOWN"
	}
}

// Decision is the outcome of evaluating one asset against one requirement.
//
// Reasons is a list rather than a single code because an operator debugging a
// match failure needs every reason the asset failed, not just the first: an
// asset that is both too small and uncertified is a different remediation from
// one that is merely too small.
type Decision struct {
	Eligible bool
	Reasons  []string
	// Details carries human-oriented evidence keyed by the check that
	// produced it. It is bounded, low-cardinality and never holds identity.
	Details map[string]string
}

// NewDecision starts an eligible decision that subsequent checks may refuse.
func NewDecision() *Decision {
	return &Decision{Eligible: true, Details: map[string]string{}}
}

// Refuse records a refusal reason. A decision that has refused once can never
// become eligible again, which is what makes "ranking cannot resurrect an
// ineligible candidate" a property of the type rather than a review comment.
func (d *Decision) Refuse(reason string, detail ...string) {
	if d == nil {
		return
	}
	d.Eligible = false
	for _, existing := range d.Reasons {
		if existing == reason {
			if len(detail) > 0 && detail[0] != "" {
				d.note(reason, detail[0])
			}
			return
		}
	}
	d.Reasons = append(d.Reasons, reason)
	if len(detail) > 0 && detail[0] != "" {
		d.note(reason, detail[0])
	}
}

func (d *Decision) note(key, value string) {
	if d.Details == nil {
		d.Details = map[string]string{}
	}
	if existing, ok := d.Details[key]; ok && existing != "" {
		d.Details[key] = existing + "; " + value
		return
	}
	d.Details[key] = value
}

// PrimaryReason is the first refusal, or ReasonEligible.
func (d *Decision) PrimaryReason() string {
	if d == nil || d.Eligible || len(d.Reasons) == 0 {
		return ReasonEligible
	}
	return d.Reasons[0]
}

// ReasonString joins the reasons for compact evidence storage.
func (d *Decision) ReasonString() string {
	if d == nil || len(d.Reasons) == 0 {
		return ReasonEligible
	}
	return strings.Join(d.Reasons, ",")
}

// RequireCapability enforces a hard boolean requirement against a three-valued
// declared fact.
//
// When the requirement is not asked for, the declared value is irrelevant and
// nothing is checked -- a bike without a thermal bag is perfectly eligible for
// a parcel that does not need one, and refusing it would make every unknown
// fact universally disqualifying rather than conditionally so.
func (d *Decision) RequireCapability(required bool, declared Tri, detail string) {
	if !required {
		return
	}
	switch declared {
	case Yes:
		return
	case No:
		d.Refuse(ReasonRequiredCapabilityUnavailable, detail)
	default:
		d.Refuse(ReasonCapabilityUnknown, detail)
	}
}

// RequireBound enforces a hard numeric bound against a declared capacity.
//
// declaredKnown is separate from declared because zero is a real measurement
// for some bounds and an absent one for others; the caller knows which, and
// passing it explicitly stops this package guessing. An unknown bound refuses
// with ReasonCapabilityUnknown whenever the requirement actually needs it,
// which is the numeric counterpart of the Tri rule above.
func (d *Decision) RequireBound(requiredValue, declared int64, declaredKnown bool, exceededReason, detail string) {
	if requiredValue <= 0 {
		return
	}
	if !declaredKnown {
		d.Refuse(ReasonCapabilityUnknown, detail)
		return
	}
	if requiredValue > declared {
		d.Refuse(exceededReason, detail)
	}
}
