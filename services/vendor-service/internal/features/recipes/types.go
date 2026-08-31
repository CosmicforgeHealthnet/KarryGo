package recipes

import (
	"errors"
	"strings"
	"time"
)

const (
	PermissionRead   = "vendor.recipes.read"
	PermissionManage = "vendor.recipes.manage"
)

const (
	TargetFoodItem            = "FOOD_ITEM"
	TargetPastryProduct       = "PASTRY_PRODUCT"
	TargetCloudKitchenPackage = "CLOUD_KITCHEN_PACKAGE"
)

const (
	RecipeStatusActive   = "ACTIVE"
	RecipeStatusArchived = "ARCHIVED"
)

const (
	VersionDraft   = "DRAFT"
	VersionActive  = "ACTIVE"
	VersionRetired = "RETIRED"
)

const (
	NormalizationUnresolved = "UNRESOLVED"
	NormalizationExact      = "EXACT"
)

const (
	ControlUntracked  = "UNTRACKED"
	ControlControlled = "INVENTORY_CONTROLLED"
)

var (
	ErrNotFound         = errors.New("recipe resource not found")
	ErrInvalidInput     = errors.New("recipe input is invalid")
	ErrVersionConflict  = errors.New("recipe version conflict")
	ErrInvalidState     = errors.New("recipe state conflict")
	ErrTargetInvalid    = errors.New("recipe catalogue target is invalid")
	ErrValidationFailed = errors.New("recipe activation validation failed")
	ErrExactConversion  = errors.New("required exact recipe conversion is unavailable")
)

type Actor struct {
	StaffID       string
	BusinessID    string
	SessionID     string
	CorrelationID string
	CausationID   string
	RequestID     string
}

type Target struct {
	Type       string `json:"type"`
	Ref        string `json:"ref"`
	VariantRef string `json:"variant_ref,omitempty"`
	BranchID   string `json:"branch_id"`
	Status     string `json:"status"`
}

type Recipe struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	TargetType       string `json:"target_type"`
	TargetRef        string `json:"target_ref"`
	TargetVariantRef string `json:"target_variant_ref,omitempty"`

	Name                string `json:"name"`
	InternalDescription string `json:"internal_description,omitempty"`

	Status string `json:"status"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateRecipeInput struct {
	TargetType       string `json:"target_type"`
	TargetRef        string `json:"target_ref"`
	TargetVariantRef string `json:"target_variant_ref,omitempty"`

	Name                string `json:"name"`
	InternalDescription string `json:"internal_description,omitempty"`
}

type RecipeVersion struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`
	RecipeID   string `json:"recipe_id"`

	VersionNumber int `json:"version_number"`

	Status string `json:"status"`

	InternalNotes string `json:"internal_notes,omitempty"`

	ExpectedOutputNumerator   int64  `json:"expected_output_numerator"`
	ExpectedOutputDenominator int64  `json:"expected_output_denominator"`
	OutputUnitCode            string `json:"output_unit_code"`

	ServingCount int64 `json:"serving_count,omitempty"`

	ExpectedWasteBPS int `json:"expected_waste_bps"`

	ActivatedAt *time.Time `json:"activated_at,omitempty"`
	RetiredAt   *time.Time `json:"retired_at,omitempty"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateVersionInput struct {
	InternalNotes string `json:"internal_notes,omitempty"`

	ExpectedOutputNumerator   int64  `json:"expected_output_numerator"`
	ExpectedOutputDenominator int64  `json:"expected_output_denominator"`
	OutputUnitCode            string `json:"output_unit_code"`

	ServingCount int64 `json:"serving_count,omitempty"`

	ExpectedWasteBPS int `json:"expected_waste_bps"`
}

type UpdateVersionInput = CreateVersionInput

type IngredientComponent struct {
	ID string `json:"id"`

	BusinessID      string `json:"business_id"`
	RecipeVersionID string `json:"recipe_version_id"`

	IngredientID string `json:"ingredient_id"`

	QuantityNumerator   int64  `json:"quantity_numerator"`
	QuantityDenominator int64  `json:"quantity_denominator"`
	UnitCode            string `json:"unit_code"`

	NormalizationStatus string `json:"normalization_status"`

	NormalizedQuantityNumerator   int64  `json:"normalized_quantity_numerator"`
	NormalizedQuantityDenominator int64  `json:"normalized_quantity_denominator"`
	NormalizedUnitCode            string `json:"normalized_unit_code,omitempty"`

	ExpectedWasteBPS int `json:"expected_waste_bps"`

	Optional bool `json:"optional"`

	Sequence int `json:"sequence"`

	PreparationNote string `json:"preparation_note,omitempty"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type IngredientComponentInput struct {
	IngredientID string `json:"ingredient_id"`

	QuantityNumerator   int64  `json:"quantity_numerator"`
	QuantityDenominator int64  `json:"quantity_denominator"`
	UnitCode            string `json:"unit_code"`

	ExpectedWasteBPS int `json:"expected_waste_bps"`

	Optional bool `json:"optional"`

	Sequence int `json:"sequence"`

	PreparationNote string `json:"preparation_note,omitempty"`
}

type ReplaceComponentsInput struct {
	Components []IngredientComponentInput `json:"components"`
}

type PackagingRequirement struct {
	ID string `json:"id"`

	BusinessID      string `json:"business_id"`
	RecipeVersionID string `json:"recipe_version_id"`

	PackagingMaterialID string `json:"packaging_material_id"`

	QuantityNumerator   int64  `json:"quantity_numerator"`
	QuantityDenominator int64  `json:"quantity_denominator"`
	UnitCode            string `json:"unit_code"`

	NormalizationStatus string `json:"normalization_status"`

	NormalizedQuantityNumerator   int64  `json:"normalized_quantity_numerator"`
	NormalizedQuantityDenominator int64  `json:"normalized_quantity_denominator"`
	NormalizedUnitCode            string `json:"normalized_unit_code,omitempty"`

	Sequence int `json:"sequence"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PackagingRequirementInput struct {
	PackagingMaterialID string `json:"packaging_material_id"`

	QuantityNumerator   int64  `json:"quantity_numerator"`
	QuantityDenominator int64  `json:"quantity_denominator"`
	UnitCode            string `json:"unit_code"`

	Sequence int `json:"sequence"`
}

type ReplacePackagingInput struct {
	Requirements []PackagingRequirementInput `json:"requirements"`
}

type VersionView struct {
	Version RecipeVersion `json:"version"`

	Components []IngredientComponent `json:"components"`

	Packaging []PackagingRequirement `json:"packaging_requirements"`
}

type ValidationIssue struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Issues []ValidationIssue `json:"issues"`
}

type ScaleInput struct {
	OutputNumerator   int64  `json:"output_numerator"`
	OutputDenominator int64  `json:"output_denominator"`
	OutputUnitCode    string `json:"output_unit_code"`
}

type ScaledRequirement struct {
	Kind string `json:"kind"`

	ReferenceID string `json:"reference_id"`

	QuantityNumerator   int64 `json:"quantity_numerator"`
	QuantityDenominator int64 `json:"quantity_denominator"`

	UnitCode string `json:"unit_code"`

	ExpectedWasteBPS int `json:"expected_waste_bps,omitempty"`

	NormalizationStatus string `json:"normalization_status"`
}

type ScaleResult struct {
	RecipeVersionID string `json:"recipe_version_id"`

	MultiplierNumerator   int64 `json:"multiplier_numerator"`
	MultiplierDenominator int64 `json:"multiplier_denominator"`

	Requirements []ScaledRequirement `json:"requirements"`
}

func normalizeEnum(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func validTargetType(value string) bool {
	switch normalizeEnum(value) {
	case TargetFoodItem,
		TargetPastryProduct,
		TargetCloudKitchenPackage:
		return true
	default:
		return false
	}
}
