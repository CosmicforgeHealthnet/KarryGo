package productionbatch

import "time"

const (
	PermissionRead          = "vendor.production.read"
	PermissionManage        = "vendor.production.manage"
	PermissionOperate       = "vendor.production.operate"
	PermissionWasteRecord   = "vendor.production.waste.record"
	PermissionPreparedStock = "vendor.production.prepared_stock"
)

const (
	StatusDraft      = "DRAFT"
	StatusPlanned    = "PLANNED"
	StatusInProgress = "IN_PROGRESS"
	StatusCompleted  = "COMPLETED"
	StatusCancelled  = "CANCELLED"
	StatusFailed     = "FAILED"
)

const (
	OperatingOnDemand   = "ON_DEMAND"
	OperatingBatchStock = "BATCH_STOCK"
)

const (
	MaterialIngredient = "INGREDIENT"
	MaterialPackaging  = "PACKAGING"
)

type Batch struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	RecipeID            string `json:"recipe_id"`
	RecipeVersionID     string `json:"recipe_version_id"`
	RecipeVersionNumber int    `json:"recipe_version_number"`

	TargetType       string `json:"target_type"`
	TargetRef        string `json:"target_ref"`
	TargetVariantRef string `json:"target_variant_ref"`

	ProductionJobID string `json:"production_job_id,omitempty"`

	PlannedOutputNumerator   int64  `json:"planned_output_numerator"`
	PlannedOutputDenominator int64  `json:"planned_output_denominator"`
	OutputUnitCode           string `json:"output_unit_code"`

	ExpectedOutputNumerator   int64 `json:"expected_output_numerator"`
	ExpectedOutputDenominator int64 `json:"expected_output_denominator"`
	RecipeExpectedWasteBPS    int   `json:"recipe_expected_waste_bps"`

	MultiplierNumerator   int64 `json:"multiplier_numerator"`
	MultiplierDenominator int64 `json:"multiplier_denominator"`

	OperatingModel string `json:"operating_model"`

	PreparedInventoryItemID    string `json:"prepared_inventory_item_id,omitempty"`
	OutputInventoryNumerator   int64  `json:"output_inventory_numerator"`
	OutputInventoryDenominator int64  `json:"output_inventory_denominator"`

	InventoryReservationID string `json:"inventory_reservation_id,omitempty"`

	Status string `json:"status"`

	ActualConsumptionRecorded bool `json:"actual_consumption_recorded"`

	ActualOutputNumerator   int64 `json:"actual_output_numerator"`
	ActualOutputDenominator int64 `json:"actual_output_denominator"`

	CreatedByStaffID   string `json:"created_by_staff_id,omitempty"`
	PlannedByStaffID   string `json:"planned_by_staff_id,omitempty"`
	StartedByStaffID   string `json:"started_by_staff_id,omitempty"`
	CompletedByStaffID string `json:"completed_by_staff_id,omitempty"`
	CancelledByStaffID string `json:"cancelled_by_staff_id,omitempty"`

	PlannedAt   *time.Time `json:"planned_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`

	CancelReason string `json:"cancel_reason,omitempty"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ComponentSnapshot struct {
	ID string `json:"id"`

	IngredientID      string `json:"ingredient_id"`
	RecipeComponentID string `json:"recipe_component_id"`

	UnitCode string `json:"unit_code"`

	PlannedQuantityNumerator   int64 `json:"planned_quantity_numerator"`
	PlannedQuantityDenominator int64 `json:"planned_quantity_denominator"`

	ExpectedWasteBPS int  `json:"expected_waste_bps"`
	Optional         bool `json:"optional"`

	InventoryControlled bool   `json:"inventory_controlled"`
	InventoryItemID     string `json:"inventory_item_id,omitempty"`
	ReservedQuantity    int64  `json:"reserved_quantity"`

	ActualUsedNumerator   int64 `json:"actual_used_numerator"`
	ActualUsedDenominator int64 `json:"actual_used_denominator"`

	ActualWasteNumerator   int64 `json:"actual_waste_numerator"`
	ActualWasteDenominator int64 `json:"actual_waste_denominator"`

	ActualRecorded bool `json:"actual_recorded"`

	VarianceNumerator   int64 `json:"variance_numerator"`
	VarianceDenominator int64 `json:"variance_denominator"`

	Sequence int `json:"sequence"`
}

type PackagingSnapshot struct {
	ID string `json:"id"`

	PackagingMaterialID          string `json:"packaging_material_id"`
	RecipePackagingRequirementID string `json:"recipe_packaging_requirement_id"`

	UnitCode string `json:"unit_code"`

	PlannedQuantityNumerator   int64 `json:"planned_quantity_numerator"`
	PlannedQuantityDenominator int64 `json:"planned_quantity_denominator"`

	ProductionConsumptionPoint string `json:"production_consumption_point"`

	InventoryControlled bool   `json:"inventory_controlled"`
	InventoryItemID     string `json:"inventory_item_id,omitempty"`
	ReservedQuantity    int64  `json:"reserved_quantity"`

	ActualUsedNumerator   int64 `json:"actual_used_numerator"`
	ActualUsedDenominator int64 `json:"actual_used_denominator"`

	ActualWasteNumerator   int64 `json:"actual_waste_numerator"`
	ActualWasteDenominator int64 `json:"actual_waste_denominator"`

	ActualRecorded bool `json:"actual_recorded"`

	VarianceNumerator   int64 `json:"variance_numerator"`
	VarianceDenominator int64 `json:"variance_denominator"`

	Sequence int `json:"sequence"`
}

type BatchDetail struct {
	Batch      Batch               `json:"batch"`
	Components []ComponentSnapshot `json:"components"`
	Packaging  []PackagingSnapshot `json:"packaging"`
}

type CreateBatchInput struct {
	BranchID string `json:"branch_id"`

	RecipeID        string `json:"recipe_id"`
	RecipeVersionID string `json:"recipe_version_id"`

	PlannedOutputNumerator   int64 `json:"planned_output_numerator"`
	PlannedOutputDenominator int64 `json:"planned_output_denominator"`

	ProductionJobID string `json:"production_job_id,omitempty"`
}

type RequirementPreview struct {
	Kind        string `json:"kind"`
	ReferenceID string `json:"reference_id"`

	UnitCode string `json:"unit_code"`

	QuantityNumerator   int64 `json:"quantity_numerator"`
	QuantityDenominator int64 `json:"quantity_denominator"`

	ExpectedWasteBPS int `json:"expected_waste_bps,omitempty"`

	InventoryControlled bool   `json:"inventory_controlled"`
	InventoryItemID     string `json:"inventory_item_id,omitempty"`
	ReservationQuantity int64  `json:"reservation_quantity,omitempty"`

	ProductionConsumptionPoint string `json:"production_consumption_point,omitempty"`
	Optional                   bool   `json:"optional,omitempty"`
}

type PreviewResult struct {
	BatchID      string               `json:"batch_id"`
	Requirements []RequirementPreview `json:"requirements"`
}

type TopUpLine struct {
	Kind        string `json:"kind"`
	ReferenceID string `json:"reference_id"`
	Quantity    int64  `json:"quantity"`
}

type TopUpInput struct {
	Lines []TopUpLine `json:"lines"`
}

type ActualConsumptionInput struct {
	Kind        string `json:"kind"`
	ReferenceID string `json:"reference_id"`

	QuantityNumerator   int64 `json:"quantity_numerator"`
	QuantityDenominator int64 `json:"quantity_denominator"`
}

type WasteInput struct {
	Kind        string `json:"kind"`
	ReferenceID string `json:"reference_id"`

	QuantityNumerator   int64 `json:"quantity_numerator"`
	QuantityDenominator int64 `json:"quantity_denominator"`

	ReasonCode string `json:"reason_code"`
}

type CompleteInput struct {
	ActualOutputNumerator   int64 `json:"actual_output_numerator"`
	ActualOutputDenominator int64 `json:"actual_output_denominator"`
}

type CancelInput struct {
	Reason string `json:"reason"`
}

type StockPolicy struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	TargetType       string `json:"target_type"`
	TargetRef        string `json:"target_ref"`
	TargetVariantRef string `json:"target_variant_ref"`

	OperatingModel string `json:"operating_model"`

	PreparedInventoryItemID string `json:"prepared_inventory_item_id,omitempty"`

	OutputInventoryNumerator   int64 `json:"output_inventory_numerator"`
	OutputInventoryDenominator int64 `json:"output_inventory_denominator"`

	Status string `json:"status"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PutStockPolicyInput struct {
	TargetType       string `json:"target_type"`
	TargetRef        string `json:"target_ref"`
	TargetVariantRef string `json:"target_variant_ref,omitempty"`

	OperatingModel string `json:"operating_model"`

	PreparedInventoryItemID string `json:"prepared_inventory_item_id,omitempty"`

	OutputInventoryNumerator   int64 `json:"output_inventory_numerator"`
	OutputInventoryDenominator int64 `json:"output_inventory_denominator"`
}

type ReservationView struct {
	ID                     string    `json:"id"`
	Kind                   string    `json:"kind"`
	Sequence               int       `json:"sequence"`
	InventoryReservationID string    `json:"inventory_reservation_id"`
	SourceRef              string    `json:"source_ref"`
	Status                 string    `json:"status"`
	CreatedAt              time.Time `json:"created_at"`
}

type OutputView struct {
	ID string `json:"id"`

	InventoryItemID    string `json:"inventory_item_id"`
	Quantity           int64  `json:"quantity"`
	InventorySourceRef string `json:"inventory_source_ref"`
	InventoryLotID     string `json:"inventory_lot_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}
