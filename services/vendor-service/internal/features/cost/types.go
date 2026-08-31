package cost

import "time"

// Persisted enum values. These mirror migration 000074 exactly; the schema's
// CHECK constraints are the real guard, and these constants exist so Go code
// cannot drift from them silently.
const (
	StrategyLatest          = "LATEST_ACCEPTED_PURCHASE_COST"
	StrategyWeightedAverage = "WEIGHTED_AVERAGE_PURCHASE_COST"
	StrategyManual          = "MANUAL_CONFIGURED_COST"
	// StrategyMixed is a snapshot-only value: a recipe whose components
	// resolved through different policies is honestly recorded as MIXED
	// rather than attributed to whichever strategy happened to come first.
	StrategyMixed = "MIXED"

	SubjectIngredient = "INGREDIENT"
	SubjectPackaging  = "PACKAGING"

	CalcRecipePreview      = "RECIPE_PREVIEW"
	CalcRecipeSnapshot     = "RECIPE_SNAPSHOT"
	CalcProductionExpected = "PRODUCTION_EXPECTED"
	CalcProductionActual   = "PRODUCTION_ACTUAL"

	CompletenessComplete   = "COMPLETE"
	CompletenessIncomplete = "INCOMPLETE"
	CompletenessUnknown    = "UNKNOWN"

	SourceUnresolved = "UNRESOLVED"

	ReasonNoPurchaseEvidence = "NO_PURCHASE_EVIDENCE"
	ReasonNoPolicy           = "NO_POLICY"
	ReasonCurrencyMismatch   = "CURRENCY_MISMATCH"
	ReasonNoInventoryMapping = "NO_INVENTORY_MAPPING"
	ReasonNoManualCost       = "NO_MANUAL_COST"
	ReasonUnitNotConvertible = "UNIT_NOT_CONVERTIBLE"

	StatusActive   = "ACTIVE"
	StatusArchived = "ARCHIVED"
)

// Permission codes seeded by migration 000074.
const (
	PermCostRead           = "vendor.cost.read"
	PermCostPolicyManage   = "vendor.cost.policy.manage"
	PermProductionCostRead = "vendor.production.cost.read"
)

// Event types, following the repository's vendor.<aggregate>.<event> naming.
const (
	EventCostPolicyChanged      = "vendor.cost_policy.changed"
	EventRecipeCostCalculated   = "vendor.recipe.cost.calculated"
	EventProductionCostCalcd    = "vendor.production.cost.calculated"
	EventProductionCostVariance = "vendor.production.cost.variance_recorded"
)

// calculationVersion is stamped on every snapshot. Bump it when the arithmetic
// or resolution rules change, so an old snapshot is never mistaken for one
// this engine would produce today.
const calculationVersion = 1

// Policy is a vendor's chosen costing rule for one ingredient or packaging
// material, optionally overridden per branch.
type Policy struct {
	ID       string `json:"id"`
	BranchID string `json:"branch_id,omitempty"`

	SubjectType string `json:"subject_type"`
	SubjectRef  string `json:"subject_ref"`

	Strategy string `json:"strategy"`
	Currency string `json:"currency"`

	WeightedAverageReceiptLimit int `json:"weighted_average_receipt_limit"`

	ManualUnitCostMinor *int64 `json:"manual_unit_cost_minor,omitempty"`
	ManualUnitCode      string `json:"manual_unit_code,omitempty"`
	ManualCostReason    string `json:"manual_cost_reason,omitempty"`

	Status  string `json:"status"`
	Version int64  `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PurchaseObservation is one accepted purchase-cost fact from Section 15.
//
// It is a read-only projection of vendor_purchase_cost_history. Section 16D
// keeps no copy of it: the fields below are carried only far enough to compute
// a cost and to record, on the resulting snapshot, which evidence produced it.
type PurchaseObservation struct {
	// EvidenceRef is the vendor_purchase_cost_history row id, recorded on the
	// snapshot component so a historical figure can be traced to its receipt.
	EvidenceRef string

	InventoryItemID string

	// InventoryQuantity is the accepted quantity in normalized inventory
	// units, which is the weighting basis for the weighted average.
	InventoryQuantity int64

	// InventoryUnitCostMinor is the cost of one normalized inventory unit, so
	// no unit conversion is needed downstream.
	InventoryUnitCostMinor int64

	Currency   string
	ObservedAt time.Time

	GoodsReceiptID     string
	GoodsReceiptLineID string
	PurchaseOrderID    string
	SupplierID         string
}

// ResolvedCost is the outcome of applying a policy to available evidence.
//
// Resolved=false is a first-class answer, not an error: "we do not know what
// this costs" is information the vendor needs, and the Reason says why.
type ResolvedCost struct {
	Resolved bool

	Source        string
	UnitCostMinor int64
	Currency      string

	EvidenceRef  string
	ReceiptCount int

	Reason string
}

// Component is one costed line of a calculation.
type Component struct {
	LineNumber int `json:"line_number"`

	SubjectType     string `json:"subject_type"`
	SubjectRef      string `json:"subject_ref"`
	InventoryItemID string `json:"inventory_item_id,omitempty"`

	// Quantity is the quantity actually costed: waste-inclusive for a recipe
	// estimate (grossed up here), already waste-inclusive for production
	// expected cost (frozen by Section 16C), and the measured figure for
	// production actual cost.
	Quantity     Quantity `json:"-"`
	QuantityNum  int64    `json:"quantity_numerator"`
	QuantityDen  int64    `json:"quantity_denominator"`
	QuantityUnit string   `json:"quantity_unit_code,omitempty"`

	CostSource         string `json:"cost_source"`
	SourceEvidenceRef  string `json:"source_evidence_ref,omitempty"`
	SourceReceiptCount int    `json:"source_receipt_count,omitempty"`

	UnitCostMinor      *int64 `json:"unit_cost_minor,omitempty"`
	Currency           string `json:"currency,omitempty"`
	ComponentCostMinor *int64 `json:"component_cost_minor,omitempty"`

	ExpectedWasteBPS int    `json:"expected_waste_bps"`
	WasteCostMinor   *int64 `json:"waste_cost_minor,omitempty"`

	IncompleteReason string `json:"incomplete_reason,omitempty"`
}

// Snapshot is a calculation result. RECIPE_PREVIEW instances are recalculable;
// every other calculation type is immutable once written, enforced by database
// triggers in migration 000074 as well as by this package.
type Snapshot struct {
	ID       string `json:"id"`
	BranchID string `json:"branch_id,omitempty"`

	CalculationType    string `json:"calculation_type"`
	CalculationVersion int    `json:"calculation_version"`

	RecipeID        string `json:"recipe_id,omitempty"`
	RecipeVersionID string `json:"recipe_version_id,omitempty"`
	BatchID         string `json:"batch_id,omitempty"`

	Strategy     string `json:"strategy"`
	Currency     string `json:"currency"`
	Completeness string `json:"completeness"`

	TotalCostMinor      *int64 `json:"total_cost_minor,omitempty"`
	IngredientCostMinor *int64 `json:"ingredient_cost_minor,omitempty"`
	PackagingCostMinor  *int64 `json:"packaging_cost_minor,omitempty"`
	WasteCostMinor      *int64 `json:"waste_cost_minor,omitempty"`

	OutputQuantityNum *int64 `json:"output_quantity_numerator,omitempty"`
	OutputQuantityDen *int64 `json:"output_quantity_denominator,omitempty"`
	ServingCount      *int64 `json:"serving_count,omitempty"`

	// CostPerServingMinor and CostPerUnitMinor are derived, not stored: they
	// are a division of the persisted total by the persisted denominator, and
	// storing them would create a second figure that could disagree with it.
	CostPerServingMinor *int64 `json:"cost_per_serving_minor,omitempty"`
	CostPerUnitMinor    *int64 `json:"cost_per_unit_minor,omitempty"`

	ExpectedWasteBPS int `json:"expected_waste_bps"`

	CalculatedAt time.Time `json:"calculated_at"`
	CalculatedBy string    `json:"calculated_by_staff_id"`

	Components []Component `json:"components,omitempty"`
}

// Variance compares a batch's expected and actual operational cost.
//
// It is an observation. Nothing acts on it: no recipe is edited, no price
// moves, no supplier is adjusted and no financial entry is made anywhere.
type Variance struct {
	BatchID string `json:"batch_id"`

	Currency string `json:"currency"`

	ExpectedTotalMinor *int64 `json:"expected_total_minor,omitempty"`
	ActualTotalMinor   *int64 `json:"actual_total_minor,omitempty"`
	VarianceMinor      *int64 `json:"variance_minor,omitempty"`

	ExpectedIngredientMinor *int64 `json:"expected_ingredient_minor,omitempty"`
	ActualIngredientMinor   *int64 `json:"actual_ingredient_minor,omitempty"`

	ExpectedPackagingMinor *int64 `json:"expected_packaging_minor,omitempty"`
	ActualPackagingMinor   *int64 `json:"actual_packaging_minor,omitempty"`

	ExpectedWasteMinor *int64 `json:"expected_waste_minor,omitempty"`
	ActualWasteMinor   *int64 `json:"actual_waste_minor,omitempty"`

	Completeness string `json:"completeness"`

	Components []ComponentVariance `json:"components,omitempty"`
}

// ComponentVariance is planned-versus-actual for one material.
type ComponentVariance struct {
	SubjectType string `json:"subject_type"`
	SubjectRef  string `json:"subject_ref"`

	ExpectedQuantityNum int64 `json:"expected_quantity_numerator"`
	ExpectedQuantityDen int64 `json:"expected_quantity_denominator"`
	ActualQuantityNum   int64 `json:"actual_quantity_numerator"`
	ActualQuantityDen   int64 `json:"actual_quantity_denominator"`

	ExpectedCostMinor *int64 `json:"expected_cost_minor,omitempty"`
	ActualCostMinor   *int64 `json:"actual_cost_minor,omitempty"`
	VarianceMinor     *int64 `json:"variance_minor,omitempty"`
}

func int64Ptr(v int64) *int64 { return &v }
