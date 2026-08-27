package fulfilment

// The vehicle capability policy.
//
// One versioned, explicit statement of what each carrier class can do. It
// replaces thresholds scattered through business logic with a single value
// that can be swapped by a deployment, embedded in a decision snapshot, and
// read back years later to explain a decision taken under rules that have
// since changed.
//
// Two distinctions this file exists to keep apart, because collapsing them is
// how a platform quietly promises a delivery it cannot perform:
//
//   - PHYSICAL SUITABILITY is whether a class of vehicle could carry the load.
//     It is a fact about boxes and vehicles.
//
//   - RUNTIME EXECUTABILITY is whether the owning logistics service can
//     actually perform that mission today. A car is physically able to carry
//     a medium load; driver-taxi-service has no goods-delivery mode, so no
//     car mission can be executed.
//
// The policy states both, separately, per class.
//
// PROVIDER SUPPLY IS DELIBERATELY ABSENT. Nothing in this file, and nothing
// in the serviceability snapshot beside it, records how many riders or trucks
// happen to be online. Supply belongs to the logistics authorities and is
// answered in their own quotes. Keeping it out is what structurally prevents
// the worst failure this engine could have: quietly choosing a smaller,
// unsafe vehicle because the correct one is scarce.

import (
	"sort"
	"strings"
	"time"
)

// MissingMetadataPolicy is what the engine does when the load's measurements
// are known to be incomplete.
//
// It is a POLICY rather than a hardcoded behaviour because the right answer
// differs by deployment: a marketplace onboarding vendors who have not filled
// in weights cannot refuse every order, and a platform carrying regulated
// goods cannot guess.
type MissingMetadataPolicy string

const (
	// MissingMetadataProceed plans on the stated lower bound and records that
	// the evidence was weak.
	//
	// This is the platform's current product policy and the behaviour every
	// existing caller already relies on: aggregation reports PARTIAL or
	// ABSENT evidence and planning proceeds in the open rather than refusing.
	// It is named here so that behaviour is a stated choice rather than an
	// accident of where the check was not written.
	MissingMetadataProceed MissingMetadataPolicy = "PROCEED_ON_LOWER_BOUND"

	// MissingMetadataConservativeEscalation refuses the smallest class when
	// evidence is absent, on the reasoning that an unmeasured load is more
	// likely to be too big for a bike than too small for a van.
	MissingMetadataConservativeEscalation MissingMetadataPolicy = "CONSERVATIVE_ESCALATION"

	// MissingMetadataRequire refuses to decide at all without measurements.
	MissingMetadataRequire MissingMetadataPolicy = "REQUIRE_METADATA"

	// MissingMetadataManualReview refuses to decide automatically and asks
	// for a human.
	MissingMetadataManualReview MissingMetadataPolicy = "MANUAL_REVIEW"
)

// Valid reports whether the policy is known.
func (m MissingMetadataPolicy) Valid() bool {
	switch m {
	case MissingMetadataProceed, MissingMetadataConservativeEscalation,
		MissingMetadataRequire, MissingMetadataManualReview:
		return true
	default:
		return false
	}
}

// VehicleCapability is one class's complete competence statement.
//
// A zero limit means UNBOUNDED for that dimension, not "cannot carry
// anything". That reading is what lets a deployment state only the limits it
// actually knows without accidentally refusing everything.
type VehicleCapability struct {
	Class VehicleClass `json:"vehicle_class"`

	// Rank orders classes smallest to largest. Selection walks it in
	// ascending order, so it is what makes "smallest suitable vehicle" a
	// property of the configuration rather than of the alphabet.
	Rank int `json:"rank"`

	// Service is the logistics authority that would execute this class.
	Service Service `json:"service"`

	MaxWeightGrams              int64 `json:"max_weight_grams"`
	MaxVolumeMl                 int64 `json:"max_volume_ml"`
	MaxLongestDimensionMm       int   `json:"max_longest_dimension_mm"`
	MaxPackageCount             int64 `json:"max_package_count"`
	MaxSinglePackageWeightGrams int64 `json:"max_single_package_weight_grams"`
	MaxRouteDistanceMeters      int64 `json:"max_route_distance_meters"`

	AllowsLargeLoad bool `json:"allows_large_load"`
	AllowsHighValue bool `json:"allows_high_value"`

	Handles         []HandlingCapability `json:"handles"`
	HandlesSpecial  []SpecialHandling    `json:"handles_special"`
	SupportsMulti   bool                 `json:"supports_multi_pickup"`
	GoodsModeActive bool                 `json:"goods_mode_active"`
}

// handles reports whether the class is competent in a handling capability.
func (c VehicleCapability) handlesCapability(want HandlingCapability) bool {
	for _, have := range c.Handles {
		if have == want {
			return true
		}
	}

	return false
}

// handlesSpecial reports whether the class is competent in a special-handling
// classification.
func (c VehicleCapability) handlesSpecial(want SpecialHandling) bool {
	for _, have := range c.HandlesSpecial {
		if have == want {
			return true
		}
	}

	return false
}

// CapabilityPolicy is the versioned set of vehicle capabilities plus the
// deterministic rules applied around them.
type CapabilityPolicy struct {
	// Version identifies the rules in force. It is embedded in every decision
	// snapshot so a historical decision stays explainable after the limits
	// change.
	Version string `json:"version"`

	// EffectiveFrom is when this policy came into force. It is evidence, not
	// a gate: the engine never picks a policy by date, because a decision
	// must use exactly one coherent policy chosen by the caller.
	EffectiveFrom time.Time `json:"effective_from"`

	// NonStackableVolumeFactorBP inflates the load's volume, in basis points,
	// when the load may not be stacked.
	//
	// A conservative deterministic derate rather than a packing simulation.
	// Non-stackable goods cannot use the full cube of a compartment, and
	// pretending they can is how a van arrives that the load will not go
	// into. 10000 means no derate.
	NonStackableVolumeFactorBP int `json:"non_stackable_volume_factor_bp"`

	MissingMetadata MissingMetadataPolicy `json:"missing_metadata_policy"`

	Classes []VehicleCapability `json:"classes"`
}

// DefaultNonStackableVolumeFactorBP derates usable volume by thirty per cent
// for a load that may not be stacked.
const DefaultNonStackableVolumeFactorBP = 13000

// DefaultCapabilityPolicyVersion names the rules stated below.
//
// It changes whenever any threshold, handling column or executability flag in
// DefaultCapabilityPolicy changes. Snapshots taken under the old version keep
// naming it, which is the whole point.
const DefaultCapabilityPolicyVersion = "vehicle-capability/1"

// DefaultCapabilityPolicy is the platform's vehicle envelope.
//
// The motorcycle column is not invented here: it mirrors the envelope
// driver-dispatch-delivery-service enforces in its own commerce quote
// (25 kg, 80 L, 700 mm, insulated box handling). Stating a looser limit would
// produce plans dispatch then refuses; stating a tighter one would refuse
// work dispatch would happily take.
//
// Van and truck are both driver-hauling-service, which owns the two classes
// together and resolves the actual truck type - flatbed, box, refrigerated
// and the rest - inside its own matcher. Vendor Service deliberately does not
// model those sub-types: choosing between a flatbed and a tipper is the
// hauling matcher's job, and duplicating its taxonomy here would create two
// vocabularies that drift.
//
// The car row is modelled and permanently not executable. See the file note.
func DefaultCapabilityPolicy() CapabilityPolicy {
	return CapabilityPolicy{
		Version:                    DefaultCapabilityPolicyVersion,
		NonStackableVolumeFactorBP: DefaultNonStackableVolumeFactorBP,
		MissingMetadata:            MissingMetadataProceed,
		Classes: []VehicleCapability{
			{
				Class:                       VehicleMotorcycle,
				Rank:                        1,
				Service:                     ServiceDispatch,
				MaxWeightGrams:              25_000,
				MaxVolumeMl:                 80_000,
				MaxLongestDimensionMm:       700,
				MaxPackageCount:             12,
				MaxSinglePackageWeightGrams: 25_000,
				MaxRouteDistanceMeters:      60_000,
				AllowsLargeLoad:             false,
				// An open courier box is not a secure compartment.
				AllowsHighValue: false,
				Handles: []HandlingCapability{
					CapabilityHotHold,
					CapabilityInsulated,
					CapabilityUprightCarry,
					CapabilityLeakContainment,
				},
				HandlesSpecial: []SpecialHandling{
					SpecialHandlingOdorSensitive,
					SpecialHandlingRawProtein,
				},
				SupportsMulti: false,
				// Dispatch has a production commerce mission ingress.
				GoodsModeActive: true,
			},
			{
				Class:                       VehicleCar,
				Rank:                        2,
				Service:                     ServiceTaxi,
				MaxWeightGrams:              200_000,
				MaxVolumeMl:                 400_000,
				MaxLongestDimensionMm:       1_600,
				MaxPackageCount:             25,
				MaxSinglePackageWeightGrams: 50_000,
				MaxRouteDistanceMeters:      150_000,
				AllowsLargeLoad:             false,
				AllowsHighValue:             true,
				Handles: []HandlingCapability{
					CapabilityHotHold,
					CapabilityInsulated,
					CapabilityUprightCarry,
					CapabilityFragileCarry,
					CapabilityLeakContainment,
				},
				HandlesSpecial: []SpecialHandling{
					SpecialHandlingOdorSensitive,
					SpecialHandlingRawProtein,
					SpecialHandlingKeepDry,
					SpecialHandlingHighValue,
				},
				SupportsMulti: false,
				// driver-taxi-service is a passenger service with no goods
				// concept in code. Modelling the class is honest; marking it
				// executable would be a delivery nobody can perform.
				GoodsModeActive: false,
			},
			{
				Class:                       VehicleVan,
				Rank:                        3,
				Service:                     ServiceHauling,
				MaxWeightGrams:              1_000_000,
				MaxVolumeMl:                 6_000_000,
				MaxLongestDimensionMm:       2_400,
				MaxPackageCount:             250,
				MaxSinglePackageWeightGrams: 100_000,
				MaxRouteDistanceMeters:      400_000,
				AllowsLargeLoad:             true,
				AllowsHighValue:             true,
				Handles: []HandlingCapability{
					CapabilityHotHold,
					CapabilityInsulated,
					CapabilityColdChain,
					CapabilityFrozenChain,
					CapabilityUprightCarry,
					CapabilityFragileCarry,
					CapabilityLeakContainment,
				},
				HandlesSpecial: []SpecialHandling{
					SpecialHandlingOdorSensitive,
					SpecialHandlingRawProtein,
					SpecialHandlingKeepDry,
					SpecialHandlingHighValue,
				},
				SupportsMulti:   false,
				GoodsModeActive: true,
			},
			{
				Class:                       VehicleTruck,
				Rank:                        4,
				Service:                     ServiceHauling,
				MaxWeightGrams:              30_000_000,
				MaxVolumeMl:                 80_000_000,
				MaxLongestDimensionMm:       12_000,
				MaxPackageCount:             5_000,
				MaxSinglePackageWeightGrams: 2_000_000,
				MaxRouteDistanceMeters:      1_500_000,
				AllowsLargeLoad:             true,
				AllowsHighValue:             true,
				Handles: []HandlingCapability{
					CapabilityHotHold,
					CapabilityInsulated,
					CapabilityColdChain,
					CapabilityFrozenChain,
					CapabilityUprightCarry,
					CapabilityFragileCarry,
					CapabilityLeakContainment,
				},
				HandlesSpecial: []SpecialHandling{
					SpecialHandlingOdorSensitive,
					SpecialHandlingRawProtein,
					SpecialHandlingKeepDry,
					SpecialHandlingHighValue,
				},
				SupportsMulti:   false,
				GoodsModeActive: true,
			},
		},
	}
}

// Normalized returns the policy with defaults applied and classes ordered
// smallest first.
//
// Ordering here rather than at every call site is what makes "take the first
// class that fits" a correct implementation of "take the smallest class that
// fits".
func (p CapabilityPolicy) Normalized() CapabilityPolicy {
	if strings.TrimSpace(p.Version) == "" {
		p.Version = DefaultCapabilityPolicyVersion
	}

	if p.NonStackableVolumeFactorBP <= 0 {
		p.NonStackableVolumeFactorBP = DefaultNonStackableVolumeFactorBP
	}

	if !p.MissingMetadata.Valid() {
		p.MissingMetadata = MissingMetadataProceed
	}

	classes := append([]VehicleCapability(nil), p.Classes...)

	sort.SliceStable(classes, func(i, j int) bool {
		if classes[i].Rank != classes[j].Rank {
			return classes[i].Rank < classes[j].Rank
		}

		return classes[i].Class < classes[j].Class
	})

	p.Classes = classes

	return p
}

// Lookup returns the capability for one class.
func (p CapabilityPolicy) Lookup(class VehicleClass) (VehicleCapability, bool) {
	for _, capability := range p.Classes {
		if capability.Class == class {
			return capability, true
		}
	}

	return VehicleCapability{}, false
}

// ServiceabilitySnapshot is the operational reality the decision was taken
// against, captured at one moment.
//
// It answers three questions per authority, and deliberately not a fourth:
//
//   - Is the integration enabled in this deployment?
//   - Does this authority operate this route?
//   - Can this authority actually drive a multi-stop collection?
//
// The absent fourth question is provider supply. See the file note: leaving
// it out is the structural reason a scarce truck can never cause a bike to be
// chosen.
type ServiceabilitySnapshot struct {
	// Integrations reports which authorities this deployment may call. An
	// authority absent from the map is treated as unavailable rather than as
	// available-by-default, because defaulting would call a service the
	// operator deliberately switched off.
	Integrations map[Service]bool `json:"integrations,omitempty"`

	// ServiceArea reports which authorities operate this particular route. An
	// authority absent from the map has made no statement, which is read as
	// "no restriction known" rather than as a refusal - the authority still
	// gets the final word in its own quote.
	ServiceArea map[Service]bool `json:"service_area,omitempty"`

	// MultiPickup reports which authorities can execute a multi-stop
	// collection today. Absent means not supported, which is the honest
	// default: neither dispatch nor hauling models a multi-pickup mission.
	MultiPickup map[Service]bool `json:"multi_pickup,omitempty"`

	CapturedAt time.Time `json:"captured_at"`
}

// IntegrationEnabled reports whether an authority may be called at all.
func (s ServiceabilitySnapshot) IntegrationEnabled(service Service) bool {
	return s.Integrations[service]
}

// RouteServed reports whether an authority operates this route. No statement
// means no known restriction.
func (s ServiceabilitySnapshot) RouteServed(service Service) bool {
	served, stated := s.ServiceArea[service]

	return !stated || served
}

// MultiPickupSupported reports whether an authority can drive a multi-stop
// collection.
func (s ServiceabilitySnapshot) MultiPickupSupported(service Service) bool {
	return s.MultiPickup[service]
}

// ServiceabilityFrom builds a snapshot from a plain enabled-services map.
//
// It is the bridge from the existing planner, which knows only which
// integrations are switched on, and it states the rest honestly: no service
// area restriction is known, and no authority supports multi-pickup.
func ServiceabilityFrom(
	available map[Service]bool,
	capturedAt time.Time,
) ServiceabilitySnapshot {
	integrations := make(map[Service]bool, len(available))

	for service, enabled := range available {
		integrations[service] = enabled
	}

	return ServiceabilitySnapshot{
		Integrations: integrations,
		CapturedAt:   capturedAt.UTC(),
	}
}
