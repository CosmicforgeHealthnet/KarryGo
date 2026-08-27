package fulfilment

// Vehicle class selection and the mapping from a class to the logistics
// authority that serves it.
//
// Pure and deterministic. Selection answers "what does this load actually
// need to be carried in", never "what does it cost" — the fee belongs to the
// logistics authority's own quote and is never derived here.
//
// TAXI. driver-taxi-service has no production goods-delivery capability: it
// is a passenger service with no goods concept in code. VehicleCar therefore
// exists as a named future class but is EXCLUDED from selection by
// selectableClasses below, and ServiceFor refuses it. That is a structural
// exclusion, not a comment: Section 17C must work without taxi, and a car
// class that could be selected would eventually reach a taxi route that is
// not production-ready.

import (
	"errors"
	"strings"
)

// VehicleClass is the carrier class a load requires.
type VehicleClass string

const (
	VehicleMotorcycle VehicleClass = "MOTORCYCLE"

	// VehicleCar is reserved for a future driver-taxi-service GOODS_DELIVERY
	// capability. It is never selected today. See the package note above.
	VehicleCar VehicleClass = "CAR"

	VehicleVan   VehicleClass = "VAN"
	VehicleTruck VehicleClass = "TRUCK"
)

// Service is the logistics authority that executes a class.
type Service string

const (
	// ServiceDispatch is driver-dispatch-delivery-service (motorcycle).
	ServiceDispatch Service = "DISPATCH"

	// ServiceHauling is driver-hauling-service, which owns BOTH van and
	// truck. Vendor Service must never create a separate van service or route
	// van missions anywhere else.
	ServiceHauling Service = "HAULING"

	// ServiceTaxi is driver-taxi-service. It is modelled so the car class has
	// a named owner and a future goods mode is a configuration change rather
	// than a redesign. It has NO production goods capability today:
	// driver-taxi-service is a passenger service with no goods concept in
	// code, so ServiceFor still refuses the car class and the capability
	// policy marks it not executable. Naming the owner is honest; routing a
	// mission to it would not be.
	ServiceTaxi Service = "TAXI"

	// ServiceVendorOwn is internal/features/delivery. It is not a platform
	// logistics authority and creates no mission in any driver service.
	ServiceVendorOwn Service = "VENDOR_OWN_DELIVERY"
)

// ErrNoSuitableVehicle means no available class can carry this load.
//
// It is a normal planning answer for an unserviceable group, not a fault, and
// it always arrives with a reason code the customer can be told.
var ErrNoSuitableVehicle = errors.New("fulfilment: no suitable vehicle class")

// Refusal reason codes for vehicle selection.
const (
	ReasonVehicleSelected = "VEHICLE_SELECTED"

	ReasonExceedsAllCapacity  = "EXCEEDS_ALL_VEHICLE_CAPACITY"
	ReasonNoCapableVehicle    = "NO_VEHICLE_WITH_REQUIRED_HANDLING"
	ReasonHintsExcludeAll     = "VENDOR_VEHICLE_HINTS_EXCLUDE_ALL_CLASSES"
	ReasonTaxiNotAvailable    = "CAR_CLASS_NOT_AVAILABLE"
	ReasonServiceNotAvailable = "LOGISTICS_SERVICE_NOT_AVAILABLE"
)

// Capability is one class's physical envelope and handling competence.
//
// These are REQUIREMENT thresholds owned by Vendor Service, which the
// architecture makes its job ("determines the requested vehicle class"). They
// are not fares and they are not the logistics authority's own eligibility
// rule: hauling still re-checks truck capacity in its own matcher, and a
// class chosen here can still be refused there.
type Capability struct {
	Class VehicleClass

	MaxWeightGrams        int64
	MaxVolumeMl           int64
	MaxLongestDimensionMm int

	AllowsLargeLoad bool

	Handles []HandlingCapability
}

func (c Capability) handles(want HandlingCapability) bool {
	for _, have := range c.Handles {
		if have == want {
			return true
		}
	}

	return false
}

// Fits reports whether a class can physically carry a requirement.
func (c Capability) Fits(requirement Requirement) bool {
	if requirement.TotalWeightGrams > c.MaxWeightGrams {
		return false
	}

	if requirement.TotalVolumeMl > c.MaxVolumeMl {
		return false
	}

	if requirement.LongestDimensionMm > c.MaxLongestDimensionMm {
		return false
	}

	if requirement.LargeLoad && !c.AllowsLargeLoad {
		return false
	}

	for _, needed := range requirement.Capabilities {
		if !c.handles(needed) {
			return false
		}
	}

	return true
}

// DefaultCapabilities is the platform's vehicle envelope, smallest first.
//
// It is a NARROW VIEW of DefaultCapabilityPolicy, not a second table. The
// thresholds are stated exactly once, in vehicle_capability.go; stating them
// twice is how two parts of one engine start disagreeing about how heavy a
// motorcycle load may be.
//
// The car class is filtered out here because this view feeds SelectVehicle,
// whose ranking excludes it. The richer engine models the car explicitly and
// then refuses to execute it, which is the more honest answer; this view
// exists for callers that only need the executable ladder.
func DefaultCapabilities() []Capability {
	policy := DefaultCapabilityPolicy().Normalized()

	capabilities := make([]Capability, 0, len(policy.Classes))

	for _, capability := range policy.Classes {
		if _, selectable := classRank(capability.Class); !selectable {
			continue
		}

		capabilities = append(capabilities, Capability{
			Class:                 capability.Class,
			MaxWeightGrams:        capability.MaxWeightGrams,
			MaxVolumeMl:           capability.MaxVolumeMl,
			MaxLongestDimensionMm: capability.MaxLongestDimensionMm,
			AllowsLargeLoad:       capability.AllowsLargeLoad,
			Handles: append(
				[]HandlingCapability(nil), capability.Handles...,
			),
		})
	}

	return capabilities
}

// classRank orders classes from smallest to largest. VehicleCar deliberately
// has no rank: it is not selectable.
func classRank(class VehicleClass) (int, bool) {
	switch class {
	case VehicleMotorcycle:
		return 1, true
	case VehicleVan:
		return 2, true
	case VehicleTruck:
		return 3, true
	default:
		return 0, false
	}
}

// hintFloor turns the vendor's own vehicle hints into a minimum selectable
// class.
//
// A hint is read as "this item needs at least this class". Taking the maximum
// over the hinted classes is what stops one item hinted VAN from being sent by
// motorcycle because another item hinted MOTORCYCLE.
//
// A CAR hint is honoured as "at least a car", which — because CAR itself is
// not selectable — floors the load at VAN. Silently downgrading it to
// motorcycle would ignore the vendor's stated need, and refusing outright
// would make an ordinary hint unserviceable.
func hintFloor(hints []string) (int, bool) {
	floor := 0
	sawKnown := false

	for _, raw := range hints {
		switch VehicleClass(strings.ToUpper(strings.TrimSpace(raw))) {
		case VehicleMotorcycle:
			sawKnown = true

			if floor < 1 {
				floor = 1
			}

		case VehicleCar, VehicleVan:
			sawKnown = true

			if floor < 2 {
				floor = 2
			}

		case VehicleTruck:
			sawKnown = true

			if floor < 3 {
				floor = 3
			}
		}
	}

	return floor, sawKnown
}

// Selection is the chosen class and the authority that serves it.
type Selection struct {
	Class   VehicleClass `json:"vehicle_class"`
	Service Service      `json:"service"`

	ReasonCode string `json:"reason_code"`

	// HintFloorApplied records that the vendor's own hints, not the physical
	// measurements, are what pushed the load up a class. Without it an
	// operator cannot tell an over-hinted catalogue from a heavy order.
	HintFloorApplied bool `json:"hint_floor_applied"`
}

// SelectVehicle chooses the smallest class that can carry the requirement.
//
// available names the logistics services this deployment may actually use, so
// a deployment with dispatch integration switched off never selects a class it
// cannot execute. Passing an empty set refuses everything rather than
// defaulting to "all", because defaulting would call a service the operator
// deliberately turned off.
func SelectVehicle(
	requirement Requirement,
	capabilities []Capability,
	available map[Service]bool,
) (Selection, error) {
	floor, hinted := hintFloor(requirement.VehicleHints)

	var (
		fittedAny  bool
		capableAny bool
	)

	for _, capability := range capabilities {
		rank, selectable := classRank(capability.Class)
		if !selectable {
			continue
		}

		if !capability.Fits(requirement) {
			continue
		}

		fittedAny = true

		if rank < floor {
			continue
		}

		service, err := ServiceFor(capability.Class)
		if err != nil {
			continue
		}

		if !available[service] {
			continue
		}

		capableAny = true

		return Selection{
			Class:            capability.Class,
			Service:          service,
			ReasonCode:       ReasonVehicleSelected,
			HintFloorApplied: hinted && rank == floor && floor > 1,
		}, nil
	}

	switch {
	case !fittedAny && requirement.RequiresCoolingChain():
		return Selection{ReasonCode: ReasonNoCapableVehicle},
			ErrNoSuitableVehicle

	case !fittedAny:
		return Selection{ReasonCode: ReasonExceedsAllCapacity},
			ErrNoSuitableVehicle

	case !capableAny && hinted:
		return Selection{ReasonCode: ReasonHintsExcludeAll},
			ErrNoSuitableVehicle

	default:
		// Something fits physically but no enabled service can carry it.
		return Selection{ReasonCode: ReasonServiceNotAvailable},
			ErrNoSuitableVehicle
	}
}

// ServiceFor maps a class to the logistics authority that executes it.
//
// VehicleCar returns an error rather than a service. This is the single point
// where a car class would have to become a taxi call, and it refuses — so no
// amount of upstream change can quietly start calling a taxi route that is
// not production-ready.
func ServiceFor(class VehicleClass) (Service, error) {
	switch class {
	case VehicleMotorcycle:
		return ServiceDispatch, nil

	case VehicleVan, VehicleTruck:
		return ServiceHauling, nil

	default:
		return "", ErrNoSuitableVehicle
	}
}
