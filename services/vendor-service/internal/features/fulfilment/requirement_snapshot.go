package fulfilment

// The requirement snapshot.
//
// One immutable record of one delivery-requirement decision: the inputs it
// was taken from, the policy in force, the operational reality at the time,
// every class considered, the class chosen and why.
//
// It exists because a decision that can only be explained by re-running
// today's rules cannot be explained at all. Thresholds change, integrations
// are switched on and off, a vendor corrects a weight - and a customer still
// asks in April why their February order went by van. The snapshot answers
// that from stored evidence alone, with no live service reachable and no
// dependence on the configuration currently deployed.
//
// It is APPEND-ONLY. A requirement that changes produces a NEW snapshot on a
// NEW fulfilment plan version; it never edits an existing one. Rewriting a
// snapshot would destroy the only record of what was actually decided.
//
// It creates no financial obligation. Declared value is risk metadata, and
// the cost of a delivery remains the logistics authority's own quote.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrSnapshotInvalid means a snapshot is not internally consistent and must
// not be persisted.
var ErrSnapshotInvalid = errors.New("fulfilment: invalid requirement snapshot")

// RequirementSnapshot is the complete, immutable evidence of one decision.
type RequirementSnapshot struct {
	// GroupID and PlanID tie the snapshot to the fulfilment plan version it
	// belongs to. SlotRef names the plan mission slot when the decision was
	// taken for a specific mission rather than for the group as a whole.
	GroupID string `json:"fulfilment_group_id,omitempty"`
	PlanID  string `json:"plan_id,omitempty"`
	SlotRef string `json:"slot_ref,omitempty"`

	BusinessID string `json:"business_id,omitempty"`
	BranchID   string `json:"branch_id,omitempty"`

	// Requirement is the full input, both the estimate and any confirmed
	// packing. Both are kept: an escalation is only explainable if the number
	// it escalated from survives.
	Requirement DeliveryRequirement `json:"requirement"`

	// Policy is the capability policy AS IT WAS. The whole set is embedded,
	// not just its version string, so the decision stays reproducible even
	// after the version is retired and its thresholds are gone from the code.
	Policy CapabilityPolicy `json:"policy"`

	// Serviceability is the operational reality the decision was taken
	// against. It records integration, service-area and multi-pickup
	// statements, and deliberately records no provider supply - see
	// ServiceabilitySnapshot.
	Serviceability ServiceabilitySnapshot `json:"serviceability"`

	Decision VehicleDecision `json:"decision"`

	// Fingerprint is a stable digest of the decision-bearing content. Two
	// snapshots with the same fingerprint represent the same decision, which
	// is what lets a replan detect that nothing operationally changed.
	Fingerprint string `json:"fingerprint"`

	CreatedAt time.Time `json:"created_at"`
}

// Snapshot builds the immutable record of one evaluation.
//
// The request is copied wholesale rather than summarised. A summary is a
// second implementation of the decision, and the two would eventually
// disagree about what was decided.
func Snapshot(
	request SelectionRequest,
	decision VehicleDecision,
) RequirementSnapshot {
	snapshot := RequirementSnapshot{
		BusinessID:     request.Requirement.BusinessID,
		BranchID:       request.Requirement.BranchID,
		Requirement:    request.Requirement,
		Policy:         request.Policy.Normalized(),
		Serviceability: request.Serviceability,
		Decision:       decision,
		CreatedAt:      decision.EvaluatedAt,
	}

	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}

	snapshot.Fingerprint = snapshot.computeFingerprint()

	return snapshot
}

// Attach returns a copy of the snapshot bound to a plan version and, when the
// decision was taken for one mission slot, to that slot.
//
// Binding is separate from building because the plan version does not exist
// until the plan is written, and the snapshot must be identical either way -
// which is why the identifiers are excluded from the fingerprint.
func (s RequirementSnapshot) Attach(
	groupID, planID, slotRef string,
) RequirementSnapshot {
	s.GroupID = strings.TrimSpace(groupID)
	s.PlanID = strings.TrimSpace(planID)
	s.SlotRef = strings.TrimSpace(slotRef)

	return s
}

// Validate reports the first reason a snapshot must not be persisted.
func (s RequirementSnapshot) Validate() error {
	if err := s.Requirement.Validate(); err != nil {
		return err
	}

	if strings.TrimSpace(s.Policy.Version) == "" {
		return ErrSnapshotInvalid
	}

	if !KnownReasonCode(s.Decision.PrimaryReason) {
		// A free-text reason on a stored decision is a reason nobody can
		// count, translate or act on.
		return ErrSnapshotInvalid
	}

	for _, reason := range s.Decision.SupportingReasons {
		if !KnownReasonCode(reason) {
			return ErrSnapshotInvalid
		}
	}

	for _, candidate := range s.Decision.Candidates {
		if !KnownReasonCode(candidate.ReasonCode) {
			return ErrSnapshotInvalid
		}
	}

	// An executable decision must name both halves of the answer, and a
	// non-executable one must name neither: a stored class with no service is
	// a mission nobody owns.
	switch {
	case s.Decision.Executable &&
		(s.Decision.SelectedClass == "" || s.Decision.SelectedService == ""):
		return ErrSnapshotInvalid

	case !s.Decision.Executable &&
		(s.Decision.SelectedClass != "" || s.Decision.SelectedService != ""):
		return ErrSnapshotInvalid
	}

	if strings.TrimSpace(s.Fingerprint) == "" {
		return ErrSnapshotInvalid
	}

	return nil
}

// computeFingerprint digests everything that makes two decisions
// operationally different.
//
// It deliberately excludes identifiers and timestamps. Two evaluations of the
// same load under the same rules are the same decision even though they
// happened at different moments on different plan versions, and a fingerprint
// that changed with the clock could never be used to detect that a replan
// changed nothing.
func (s RequirementSnapshot) computeFingerprint() string {
	measurement := s.Requirement.Authoritative()

	digest := sha256.New()

	fmt.Fprintf(digest, "policy=%s\n", s.Policy.Version)

	fmt.Fprintf(digest,
		"state=%s|weight=%d|volume=%d|effective_volume=%d|longest=%d|packages=%d|heaviest=%d|stackable=%t|large=%t\n",
		measurement.State,
		measurement.TotalWeightGrams,
		measurement.TotalVolumeMl,
		s.Decision.EffectiveVolumeMl,
		measurement.LongestDimensionMm,
		measurement.PackageCount,
		measurement.MaxSinglePackageWeightGrams,
		measurement.Stackable,
		measurement.LargeLoad,
	)

	capabilities := append(
		[]HandlingCapability(nil), s.Requirement.Handling.Capabilities...,
	)
	sort.Slice(capabilities, func(i, j int) bool {
		return capabilities[i] < capabilities[j]
	})

	for _, capability := range capabilities {
		fmt.Fprintf(digest, "handling=%s\n", capability)
	}

	special := append(
		[]SpecialHandling(nil), s.Requirement.Handling.SpecialHandling...,
	)
	sort.Slice(special, func(i, j int) bool { return special[i] < special[j] })

	for _, handling := range special {
		fmt.Fprintf(digest, "special=%s\n", handling)
	}

	fmt.Fprintf(digest, "risk=%s|pickups=%d|distance=%d\n",
		s.Requirement.Value.RiskClass,
		s.Requirement.Route.PickupCount,
		s.Requirement.Route.DistanceMeters,
	)

	fmt.Fprintf(digest, "minimum=%s|selected=%s|service=%s|executable=%t|reason=%s\n",
		s.Decision.MinimumPhysicalClass,
		s.Decision.SelectedClass,
		s.Decision.SelectedService,
		s.Decision.Executable,
		s.Decision.PrimaryReason,
	)

	for _, reason := range s.Decision.SupportingReasons {
		fmt.Fprintf(digest, "  supporting=%s\n", reason)
	}

	candidates := append(
		[]CandidateEvaluation(nil), s.Decision.Candidates...,
	)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Rank != candidates[j].Rank {
			return candidates[i].Rank < candidates[j].Rank
		}

		return candidates[i].Class < candidates[j].Class
	})

	for _, candidate := range candidates {
		fmt.Fprintf(digest, "  candidate=%s|fits=%t|executable=%t|reason=%s\n",
			candidate.Class, candidate.PhysicallyFits,
			candidate.Executable, candidate.ReasonCode,
		)
	}

	return hex.EncodeToString(digest.Sum(nil))
}

// EvaluateAndSnapshot runs the engine and records the result in one call.
//
// It is the ordinary entry point. Building a snapshot separately from the
// evaluation that produced it would allow the two to describe different
// decisions.
func EvaluateAndSnapshot(
	request SelectionRequest,
) (RequirementSnapshot, error) {
	decision, err := Evaluate(request)
	if err != nil {
		return RequirementSnapshot{}, err
	}

	snapshot := Snapshot(request, decision)

	if err = snapshot.Validate(); err != nil {
		return RequirementSnapshot{}, err
	}

	return snapshot, nil
}

// MarshalEvidence renders the snapshot as the JSON stored on a plan version.
//
// A single object rather than scattered columns, because the evidence has to
// be read back as one coherent decision and a partially-migrated set of
// columns is how half a decision gets stored.
func (s RequirementSnapshot) MarshalEvidence() (json.RawMessage, error) {
	payload, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	return payload, nil
}

// UnmarshalSnapshot reads back stored evidence.
func UnmarshalSnapshot(payload []byte) (RequirementSnapshot, error) {
	var snapshot RequirementSnapshot

	if len(payload) == 0 {
		return RequirementSnapshot{}, ErrSnapshotInvalid
	}

	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return RequirementSnapshot{}, err
	}

	return snapshot, nil
}

// Changed reports whether another snapshot represents a materially different
// decision.
//
// Material means operationally different, not textually different: a new
// evaluation at a later time with identical inputs and an identical outcome
// is not a change, and replanning on it would produce a plan version that
// says nothing.
func (s RequirementSnapshot) Changed(other RequirementSnapshot) bool {
	return s.Fingerprint != other.Fingerprint
}

// ReplanReason maps a requirement change to the fulfilment plan reason that
// should carry it.
//
// The vocabulary is the delivery feature's existing overflow set, reused
// rather than duplicated: a capacity overflow that forced a replan and a
// capacity overflow that forced a hybrid fallback are the same operational
// condition, and two spellings would make them impossible to count together.
//
// It returns false when the change does not justify a replan at all.
func (s RequirementSnapshot) ReplanReason(
	previous RequirementSnapshot,
) (string, bool) {
	if !previous.Changed(s) {
		return "", false
	}

	switch {
	case s.Decision.PrimaryReason == ReasonSLANotMet:
		return "SLA_OVERFLOW", true

	case s.Decision.SelectedClass != previous.Decision.SelectedClass:
		// The vehicle itself changed. That is the vehicle overflow condition
		// whether it escalated or shrank.
		return "VEHICLE_OVERFLOW", true

	case s.Decision.Executable != previous.Decision.Executable:
		return "PROVIDER_UNAVAILABLE", true

	default:
		return "CAPACITY_OVERFLOW", true
	}
}
