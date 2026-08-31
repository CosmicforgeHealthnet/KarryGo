package inventory

import "context"

const (
	StockUnitCount             = "COUNT"
	StockUnitMass              = "MASS"
	StockUnitVolume            = "VOLUME"
	StockUnitConfiguredMeasure = "CONFIGURED_MEASURE"

	StockBasisSameUnit           = "SAME_UNIT"
	StockBasisExactConversion    = "EXACT_CONVERSION"
	StockBasisVariableFulfilment = "VARIABLE_FULFILMENT"

	TrackingNone            = "NONE"
	TrackingSimple          = "SIMPLE"
	TrackingLot             = "LOT"
	TrackingExpiry          = "EXPIRY"
	TrackingVariableMeasure = "VARIABLE_MEASURE"

	LotPolicyFIFO = "FIFO"
	LotPolicyFEFO = "FEFO"
)

// ResolvedStockMeasure is bounded measurement evidence supplied by the source
// catalogue adapter.
//
// Inventory never derives local measurement truth from labels.
type ResolvedStockMeasure struct {
	Ref      string
	OwnerKey string
	Family   string

	UnitKind string

	Active bool

	SameUnitEligible   bool
	ExactConvertible   bool
	VariableFulfilment bool
}

// StockMeasureResolver consumes measurement truth owned by another domain.
//
// For SHOP_VARIANT, Shop remains authority for measures, sell units and
// conversion/calibration rules.
type StockMeasureResolver interface {
	ResolveStockMeasure(
		ctx context.Context,
		actor Actor,
		scope BranchScope,
		source SourceReference,
		measureRef string,
	) (ResolvedStockMeasure, error)
}
