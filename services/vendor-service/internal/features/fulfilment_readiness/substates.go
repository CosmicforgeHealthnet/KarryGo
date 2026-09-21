package fulfilment_readiness

import (
	"fmt"
	"strings"
)

// VendorType is the category an order is prepared under.
//
// The first seven values are exactly the platform's own capability vocabulary
// (vendor_capabilities.capability_type). They are not a parallel list: a
// readiness row's vendor_type is the capability the branch is fulfilling this
// order with, and a value the platform cannot configure would describe an order
// nobody can place.
type VendorType string

const (
	VendorRestaurant   VendorType = "RESTAURANT"
	VendorFastFood     VendorType = "FAST_FOOD"
	VendorCloudKitchen VendorType = "CLOUD_KITCHEN"
	VendorPastryBakery VendorType = "PASTRY_BAKERY"
	VendorShopGrocery  VendorType = "SHOP_GROCERY"
	VendorMarket       VendorType = "MARKET"
	VendorWholesale    VendorType = "WHOLESALE"

	// The last two are defined because Section 11 specifies their substates,
	// and left unreachable because the platform's capability vocabulary does
	// not yet admit them. Defining them now means the day that vocabulary grows
	// is a one-line change here rather than a redesign; a test below pins the
	// gap rather than letting it look like an oversight.
	VendorCatering     VendorType = "CATERING"
	VendorManufacturer VendorType = "MANUFACTURER"
)

// PlatformVendorTypes are the capability types the platform can actually
// configure today.
func PlatformVendorTypes() []VendorType {
	return []VendorType{
		VendorRestaurant, VendorFastFood, VendorCloudKitchen, VendorPastryBakery,
		VendorShopGrocery, VendorMarket, VendorWholesale,
	}
}

// AllVendorTypes includes the two categories Section 11 specifies but the
// capability vocabulary cannot yet express.
func AllVendorTypes() []VendorType {
	return append(PlatformVendorTypes(), VendorCatering, VendorManufacturer)
}

// ParseVendorType resolves a wire value.
func ParseVendorType(value string) (VendorType, error) {
	candidate := VendorType(strings.ToUpper(strings.TrimSpace(value)))
	for _, known := range AllVendorTypes() {
		if candidate == known {
			return known, nil
		}
	}
	return "", fmt.Errorf("%w: %q is not a vendor type", ErrValidation, value)
}

// ConfigurableOnPlatform reports whether a vendor type can be reached today.
func (v VendorType) ConfigurableOnPlatform() bool {
	for _, known := range PlatformVendorTypes() {
		if v == known {
			return true
		}
	}
	return false
}

// Substate is the vendor-type-specific detail beneath a readiness state.
//
// It exists because PREPARING means something different in each category, and
// a branch screen that says only "preparing" tells staff nothing they did not
// already know. Substates are typed per category rather than pooled, so a
// grocery order cannot be reported as COOLING_CHILLING and a bakery order
// cannot be reported as WEIGHING.
type Substate string

// Food preparation.
const (
	SubCooking    Substate = "COOKING"
	SubAssembling Substate = "ASSEMBLING"
	SubPacking    Substate = "PACKING"
)

// Retail picking.
const (
	SubPicking              Substate = "PICKING"
	SubSubstitutionRequired Substate = "SUBSTITUTION_REQUIRED"
	SubCustomerDecisionWait Substate = "CUSTOMER_DECISION_WAIT"
)

// Fresh market.
const (
	SubWeighing             Substate = "WEIGHING"
	SubCustomerApprovalWait Substate = "CUSTOMER_APPROVAL_WAIT"
)

// Bakery and pastry.
const (
	SubProduction      Substate = "PRODUCTION"
	SubCoolingChilling Substate = "COOLING_CHILLING"
	SubDecorating      Substate = "DECORATING"
	SubPackaging       Substate = "PACKAGING"
)

// Cloud kitchen.
const (
	SubQueue           Substate = "QUEUE"
	SubBatchCompletion Substate = "BATCH_COMPLETION"
)

// Bulk and warehouse.
const (
	SubProductionBatches Substate = "PRODUCTION_BATCHES"
	SubStaging           Substate = "STAGING"
	SubLoadReady         Substate = "LOAD_READY"
	SubLoadingPrep       Substate = "LOADING_PREP"
)

// Manufacturing.
const (
	SubProductionComplete Substate = "PRODUCTION_COMPLETE"
	SubWarehouseRelease   Substate = "WAREHOUSE_RELEASE"
	SubLoadingBayReady    Substate = "LOADING_BAY_READY"
)

// substatesByType is the whole of the vendor-type adapter.
//
// Order matters: it is the sequence staff move through, and the predictor uses
// the position to estimate how much of the work remains. A category whose
// substates were listed in an arbitrary order would produce a progress estimate
// that moved backwards.
var substatesByType = map[VendorType][]Substate{
	VendorRestaurant:   {SubCooking, SubAssembling, SubPacking},
	VendorFastFood:     {SubCooking, SubAssembling, SubPacking},
	VendorCloudKitchen: {SubQueue, SubProduction, SubBatchCompletion, SubPacking},
	VendorPastryBakery: {SubProduction, SubCoolingChilling, SubDecorating, SubPackaging},
	VendorShopGrocery:  {SubPicking, SubSubstitutionRequired, SubCustomerDecisionWait, SubPacking},
	VendorMarket:       {SubPicking, SubWeighing, SubCustomerApprovalWait, SubPacking},
	VendorWholesale:    {SubPicking, SubStaging, SubLoadingPrep, SubLoadReady},
	VendorCatering:     {SubProductionBatches, SubStaging, SubLoadReady},
	VendorManufacturer: {SubProductionComplete, SubWarehouseRelease, SubLoadingBayReady},
}

// SubstatesFor returns the ordered substates for a vendor type.
func SubstatesFor(vendorType VendorType) []Substate {
	ordered := substatesByType[vendorType]
	out := make([]Substate, len(ordered))
	copy(out, ordered)
	return out
}

// waitingSubstates are the ones where the branch is blocked on somebody else.
//
// They matter to prediction rather than to display: time spent waiting for a
// customer to approve a substitution is not time the branch is working, so it
// must not be counted against the branch's own baseline.
var waitingSubstates = map[Substate]bool{
	SubSubstitutionRequired: true,
	SubCustomerDecisionWait: true,
	SubCustomerApprovalWait: true,
}

// Waiting reports whether a substate means the branch is waiting on somebody.
func (s Substate) Waiting() bool { return waitingSubstates[s] }

// ValidateSubstate refuses a substate that does not belong to this category.
//
// The failure it prevents is subtle: a grocery order reported as COOKING is
// accepted by a looser model, displayed to staff who do not cook, and factored
// into a bakery baseline it has nothing to do with.
func ValidateSubstate(vendorType VendorType, state State, substate Substate) error {
	if substate == "" {
		return nil
	}
	// A substate describes work in progress. Declaring one on a finished,
	// cancelled or not-yet-started order is describing work that is not
	// happening.
	switch state {
	case StatePreparing, StateReadySoon, StateBlocked, StateDelayed:
	default:
		return fmt.Errorf("%w: %s carries no substate", ErrValidation, state)
	}
	for _, allowed := range SubstatesFor(vendorType) {
		if allowed == substate {
			return nil
		}
	}
	return fmt.Errorf("%w: %q is not a %s substate", ErrValidation, substate, vendorType)
}

// SubstateProgress is how far through its own sequence a substate sits, as a
// fraction between 0 and 1.
//
// It is used only to sharpen an estimate, never to decide readiness: being at
// the last substate is not the same as being ready, and a branch that packs
// before it finishes cooking has not finished cooking.
func SubstateProgress(vendorType VendorType, substate Substate) float64 {
	ordered := SubstatesFor(vendorType)
	if len(ordered) == 0 || substate == "" {
		return 0
	}
	for index, candidate := range ordered {
		if candidate == substate {
			// A waiting substate makes no progress against the branch's own
			// work, so it reports the position of the step before it.
			if substate.Waiting() && index > 0 {
				index--
			}
			return float64(index) / float64(len(ordered))
		}
	}
	return 0
}
