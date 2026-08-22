package fulfilment

// Logistics requirement aggregation.
//
// One vendor group's revalidated lines in, one honest statement of what the
// delivery actually needs out. Pure and deterministic: the catalogue lookups
// happen behind a port, so the aggregation rules can be tested without a
// vendor, a branch or a database.
//
// The governing rule of this file is that MISSING EVIDENCE IS REPORTED, NEVER
// INVENTED. A vendor who has not stated a weight produces WeightEvidence
// PARTIAL or ABSENT, not a total of zero that would silently justify a
// motorcycle. What the planner does about weak evidence is the planner's
// decision, taken in the open.

import (
	"sort"
	"strings"
)

// HandlingCapability is something the carrier must actually be able to do.
//
// Deliberately expressed as a required CAPABILITY rather than as a
// temperature label, because Food and Shop use different vocabularies for the
// same physical need and a carrier cares about the need, not the vocabulary.
type HandlingCapability string

const (
	// CapabilityHotHold keeps prepared hot food hot. An insulated delivery
	// box satisfies it.
	CapabilityHotHold HandlingCapability = "HOT_HOLD"

	// CapabilityInsulated keeps something cool for a short urban trip. An
	// insulated box satisfies it; it is NOT a cold chain.
	CapabilityInsulated HandlingCapability = "INSULATED"

	// CapabilityColdChain is an actively refrigerated compartment held within
	// a temperature band for the whole journey.
	CapabilityColdChain HandlingCapability = "COLD_CHAIN"

	// CapabilityFrozenChain is an actively frozen compartment.
	CapabilityFrozenChain HandlingCapability = "FROZEN_CHAIN"

	// CapabilityUprightCarry is a load that must not be laid down or
	// inverted.
	CapabilityUprightCarry HandlingCapability = "UPRIGHT_CARRY"

	// CapabilityFragileCarry is a load that must not be stacked under or
	// jolted.
	CapabilityFragileCarry HandlingCapability = "FRAGILE_CARRY"

	// CapabilityLeakContainment is a spill/leak risk.
	CapabilityLeakContainment HandlingCapability = "LEAK_CONTAINMENT"
)

// Evidence states how complete the physical measurements behind a requirement
// are. It travels with the plan so a later reader never has to guess whether
// a total means "small" or "unmeasured".
type Evidence string

const (
	// EvidenceComplete means every line stated the measurement.
	EvidenceComplete Evidence = "COMPLETE"

	// EvidencePartial means some lines stated it and some did not, so the
	// total is a LOWER BOUND on the real value.
	EvidencePartial Evidence = "PARTIAL"

	// EvidenceAbsent means no line stated it at all.
	EvidenceAbsent Evidence = "ABSENT"
)

// Urgency is the service urgency of a group.
type Urgency string

const (
	// UrgencyStandard is the default.
	UrgencyStandard Urgency = "STANDARD"

	// UrgencyImmediate is a group that degrades if it waits — prepared hot
	// food is the case that exists today. It is what makes a group a poor
	// candidate for consolidation with a slower one.
	UrgencyImmediate Urgency = "IMMEDIATE"
)

// LineLogistics is one revalidated checkout line presented for logistics
// resolution. It carries no price and no availability: this is a physical
// question, not a commercial one.
type LineLogistics struct {
	CheckoutLineID string

	Domain string

	BusinessID string
	BranchID   string

	ItemRef     string
	VariantRef  string
	ListingRef  string
	SellUnitRef string

	Quantity int64
}

// LineProfile is the catalogue authority's physical answer for ONE unit of a
// line. Every measurement is optional, and a nil measurement means the vendor
// has not stated one — which is different from stating zero.
type LineProfile struct {
	// Known is false when the authority holds no logistics profile for this
	// line at all.
	Known bool

	WeightGrams *int64
	VolumeMl    *int64

	LengthMm *int
	WidthMm  *int
	HeightMm *int

	// TemperatureClass is the owning authority's own vocabulary:
	// Food AMBIENT|HOT|COLD|FROZEN, Shop AMBIENT|FRESH|CHILLED|FROZEN.
	// It is normalized into HandlingCapability here rather than at the seam,
	// so a new authority vocabulary is a one-file change.
	TemperatureClass string

	Fragile   bool
	SpillRisk bool

	// UprightRequired and StackRestricted come from Shop handling flags.
	UprightRequired bool
	StackRestricted bool

	LargeLoad bool

	// PackageSizeClass is the Food SMALL|MEDIUM|LARGE hint. It is fallback
	// evidence for a line with no stated weight, never a substitute for one
	// that has it.
	PackageSizeClass string

	ConsolidationClass string
	SeparationGroup    string

	// VehicleHints is the vendor's own suitability statement
	// (MOTORCYCLE|CAR|VAN|TRUCK). Empty means the vendor stated nothing.
	VehicleHints []string
}

// Requirement is one vendor group's aggregated logistics need.
type Requirement struct {
	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	// PackageCount is the number of physical units to be carried, which is
	// the sum of line quantities and not the number of lines.
	PackageCount int64 `json:"package_count"`

	TotalWeightGrams int64    `json:"total_weight_grams"`
	WeightEvidence   Evidence `json:"weight_evidence"`

	TotalVolumeMl  int64    `json:"total_volume_ml"`
	VolumeEvidence Evidence `json:"volume_evidence"`

	// LongestDimensionMm is the longest single edge of any one unit, not a
	// sum: it answers "will it physically go in", which stacking cannot fix.
	LongestDimensionMm int      `json:"longest_dimension_mm"`
	DimensionEvidence  Evidence `json:"dimension_evidence"`

	Capabilities []HandlingCapability `json:"capabilities"`

	// LargeLoad is any line the vendor marked as a large load.
	LargeLoad bool `json:"large_load"`

	Urgency Urgency `json:"urgency"`

	// ConsolidationClasses and SeparationGroups are carried as evidence for
	// the consolidation decision. A group carrying two different separation
	// groups is telling the planner those items must not share a compartment.
	ConsolidationClasses []string `json:"consolidation_classes,omitempty"`
	SeparationGroups     []string `json:"separation_groups,omitempty"`

	// VehicleHints is the intersection-style constraint built from every
	// line's hints: see MinimumHintedClass. Empty means unconstrained.
	VehicleHints []string `json:"vehicle_hints,omitempty"`

	// ProfiledLines and TotalLines make the evidence quality auditable
	// without recomputing it.
	ProfiledLines int `json:"profiled_lines"`
	TotalLines    int `json:"total_lines"`
}

// SeparationConflict reports that this group carries items the vendor has
// declared must not travel together.
func (r Requirement) SeparationConflict() bool {
	return len(r.SeparationGroups) > 1
}

// RequiresCoolingChain reports whether an actively temperature-controlled
// compartment is needed. An insulated box does not satisfy it.
func (r Requirement) RequiresCoolingChain() bool {
	return r.HasCapability(CapabilityColdChain) ||
		r.HasCapability(CapabilityFrozenChain)
}

// HasCapability reports whether the requirement includes a capability.
func (r Requirement) HasCapability(want HandlingCapability) bool {
	for _, capability := range r.Capabilities {
		if capability == want {
			return true
		}
	}

	return false
}

// capabilitiesFor normalizes one authority's temperature vocabulary and
// handling flags into required carrier capabilities.
//
// The FRESH/COLD split is the one judgement call here and it is deliberate:
// a Food COLD item is a drink or a salad that survives an insulated box on a
// short urban trip, while a Shop CHILLED grocery is a cold-chain product
// whose safety depends on staying in band. Treating them identically would
// either put chilled meat on a motorcycle or refuse to let a bike carry a
// cold drink; neither is acceptable, so they map to different capabilities.
func capabilitiesFor(profile LineProfile) []HandlingCapability {
	var required []HandlingCapability

	switch strings.ToUpper(strings.TrimSpace(profile.TemperatureClass)) {
	case "HOT":
		required = append(required, CapabilityHotHold)
	case "COLD", "FRESH":
		required = append(required, CapabilityInsulated)
	case "CHILLED":
		required = append(required, CapabilityColdChain)
	case "FROZEN":
		required = append(required, CapabilityFrozenChain)
	}

	if profile.Fragile || profile.StackRestricted {
		required = append(required, CapabilityFragileCarry)
	}

	if profile.UprightRequired {
		required = append(required, CapabilityUprightCarry)
	}

	if profile.SpillRisk {
		required = append(required, CapabilityLeakContainment)
	}

	return required
}

// sizeClassWeightGrams is the fallback weight a Food package-size class
// implies when the vendor stated no weight at all.
//
// It is explicitly a FALLBACK and never overrides a stated weight. It exists
// because refusing to plan a group with an unstated weight would break every
// vendor who has not filled the field in, while assuming zero would quietly
// put a large order on a motorcycle.
func sizeClassWeightGrams(class string) (int64, bool) {
	switch strings.ToUpper(strings.TrimSpace(class)) {
	case "SMALL":
		return 1000, true
	case "MEDIUM":
		return 4000, true
	case "LARGE":
		return 12000, true
	default:
		return 0, false
	}
}

func appendUnique(existing []string, value string) []string {
	value = strings.ToUpper(strings.TrimSpace(value))

	if value == "" {
		return existing
	}

	for _, current := range existing {
		if current == value {
			return existing
		}
	}

	return append(existing, value)
}

func evidenceFor(stated, total int) Evidence {
	switch {
	case total == 0 || stated == 0:
		return EvidenceAbsent
	case stated == total:
		return EvidenceComplete
	default:
		return EvidencePartial
	}
}

// Aggregate builds one group's requirement from its lines and their resolved
// profiles.
//
// profiles is indexed by CheckoutLineID. A line with no entry is counted in
// TotalLines but contributes no measurement, which is exactly how a group
// ends up PARTIAL rather than silently light.
func Aggregate(
	businessID, branchID string,
	lines []LineLogistics,
	profiles map[string]LineProfile,
) Requirement {
	requirement := Requirement{
		BusinessID: businessID,
		BranchID:   branchID,
		Urgency:    UrgencyStandard,
		TotalLines: len(lines),
	}

	var (
		weightStated     int
		volumeStated     int
		dimensionStated  int
		capabilitiesSeen = map[HandlingCapability]bool{}
	)

	for _, line := range lines {
		if line.Quantity <= 0 {
			continue
		}

		requirement.PackageCount += line.Quantity

		profile, ok := profiles[line.CheckoutLineID]
		if !ok || !profile.Known {
			continue
		}

		requirement.ProfiledLines++

		switch {
		case profile.WeightGrams != nil && *profile.WeightGrams > 0:
			requirement.TotalWeightGrams += *profile.WeightGrams * line.Quantity
			weightStated++

		default:
			// Fallback evidence only. It still counts as PARTIAL, never
			// COMPLETE, so the plan never claims a measured total it does not
			// have.
			if fallback, ok := sizeClassWeightGrams(
				profile.PackageSizeClass,
			); ok {
				requirement.TotalWeightGrams += fallback * line.Quantity
			}
		}

		if profile.VolumeMl != nil && *profile.VolumeMl > 0 {
			requirement.TotalVolumeMl += *profile.VolumeMl * line.Quantity
			volumeStated++
		}

		longest := 0

		for _, dimension := range []*int{
			profile.LengthMm, profile.WidthMm, profile.HeightMm,
		} {
			if dimension != nil && *dimension > longest {
				longest = *dimension
			}
		}

		if longest > 0 {
			dimensionStated++

			if longest > requirement.LongestDimensionMm {
				requirement.LongestDimensionMm = longest
			}
		}

		for _, capability := range capabilitiesFor(profile) {
			capabilitiesSeen[capability] = true
		}

		if profile.LargeLoad {
			requirement.LargeLoad = true
		}

		requirement.ConsolidationClasses = appendUnique(
			requirement.ConsolidationClasses, profile.ConsolidationClass,
		)

		requirement.SeparationGroups = appendUnique(
			requirement.SeparationGroups, profile.SeparationGroup,
		)

		for _, hint := range profile.VehicleHints {
			requirement.VehicleHints = appendUnique(
				requirement.VehicleHints, hint,
			)
		}
	}

	for capability := range capabilitiesSeen {
		requirement.Capabilities = append(requirement.Capabilities, capability)
	}

	// Deterministic ordering. These values end up persisted on a plan and
	// compared across replans, so map iteration order must not leak into
	// them.
	sort.Slice(requirement.Capabilities, func(i, j int) bool {
		return requirement.Capabilities[i] < requirement.Capabilities[j]
	})

	sort.Strings(requirement.ConsolidationClasses)
	sort.Strings(requirement.SeparationGroups)
	sort.Strings(requirement.VehicleHints)

	requirement.WeightEvidence = evidenceFor(weightStated, len(lines))
	requirement.VolumeEvidence = evidenceFor(volumeStated, len(lines))
	requirement.DimensionEvidence = evidenceFor(dimensionStated, len(lines))

	// Hot food is the only urgency the platform can currently evidence, and
	// it is evidenced rather than assumed: a group is IMMEDIATE because a
	// line in it must be kept hot, not because it came from the Food domain.
	if capabilitiesSeen[CapabilityHotHold] {
		requirement.Urgency = UrgencyImmediate
	}

	return requirement
}
