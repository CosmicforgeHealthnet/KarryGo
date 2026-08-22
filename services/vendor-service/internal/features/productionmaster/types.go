package productionmaster

import (
	"errors"
	"strings"
	"time"
)

const (
	PermissionIngredientRead             = "vendor.ingredients.read"
	PermissionIngredientManage           = "vendor.ingredients.manage"
	PermissionIngredientInventoryMapping = "vendor.ingredients.inventory_mapping"

	PermissionPackagingRead             = "vendor.packaging.read"
	PermissionPackagingManage           = "vendor.packaging.manage"
	PermissionPackagingInventoryMapping = "vendor.packaging.inventory_mapping"
)

const (
	MeasurementMass   = "MASS"
	MeasurementVolume = "VOLUME"
	MeasurementCount  = "COUNT"
)

const (
	InventoryUntracked  = "UNTRACKED"
	InventoryControlled = "INVENTORY_CONTROLLED"
)

const (
	StatusDraft    = "DRAFT"
	StatusActive   = "ACTIVE"
	StatusInactive = "INACTIVE"
	StatusArchived = "ARCHIVED"
)

const (
	ConversionExactAuthoritative = "EXACT_AUTHORITATIVE"
	ConversionDisplayOnly        = "DISPLAY_ONLY"
)

var (
	ErrNotFound         = errors.New("production master resource not found")
	ErrInvalidInput     = errors.New("production master input is invalid")
	ErrVersionConflict  = errors.New("production master version conflict")
	ErrInvalidState     = errors.New("production master state conflict")
	ErrInventoryMapping = errors.New("production master inventory mapping is invalid")
	ErrUnsafeConversion = errors.New("unsafe authoritative conversion")
)

type Actor struct {
	StaffID       string
	BusinessID    string
	SessionID     string
	CorrelationID string
	CausationID   string
	RequestID     string
}

type Ingredient struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`

	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	MeasurementFamily string `json:"measurement_family"`
	DefaultUnitCode   string `json:"default_unit_code"`

	InventoryControlMode string `json:"inventory_control_mode"`

	Perishable     bool `json:"perishable"`
	RequiresExpiry bool `json:"requires_expiry"`

	Status string `json:"status"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateIngredientInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	MeasurementFamily string `json:"measurement_family"`
	DefaultUnitCode   string `json:"default_unit_code"`

	InventoryControlMode string `json:"inventory_control_mode"`

	Perishable     bool `json:"perishable"`
	RequiresExpiry bool `json:"requires_expiry"`

	Status string `json:"status,omitempty"`
}

type UpdateIngredientInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	MeasurementFamily string `json:"measurement_family"`
	DefaultUnitCode   string `json:"default_unit_code"`

	InventoryControlMode string `json:"inventory_control_mode"`

	Perishable     bool `json:"perishable"`
	RequiresExpiry bool `json:"requires_expiry"`

	Status string `json:"status"`
}

type PackagingMaterial struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`

	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	MeasurementFamily string `json:"measurement_family"`
	DefaultUnitCode   string `json:"default_unit_code"`

	InventoryControlMode string `json:"inventory_control_mode"`

	Status string `json:"status"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreatePackagingMaterialInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	MeasurementFamily string `json:"measurement_family"`
	DefaultUnitCode   string `json:"default_unit_code"`

	InventoryControlMode string `json:"inventory_control_mode"`

	Status string `json:"status,omitempty"`
}

type UpdatePackagingMaterialInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	MeasurementFamily string `json:"measurement_family"`
	DefaultUnitCode   string `json:"default_unit_code"`

	InventoryControlMode string `json:"inventory_control_mode"`

	Status string `json:"status"`
}

type UnitConversion struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`

	OwnerID string `json:"owner_id"`

	MeasurementFamily string `json:"measurement_family"`

	FromUnitCode string `json:"from_unit_code"`
	ToUnitCode   string `json:"to_unit_code"`

	ConversionMode string `json:"conversion_mode"`

	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`

	SourceContext string `json:"source_context,omitempty"`

	Status string `json:"status"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PutUnitConversionInput struct {
	MeasurementFamily string `json:"measurement_family"`

	FromUnitCode string `json:"from_unit_code"`
	ToUnitCode   string `json:"to_unit_code"`

	ConversionMode string `json:"conversion_mode"`

	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`

	SourceContext string `json:"source_context,omitempty"`

	Status string `json:"status,omitempty"`
}

type BranchConfig struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`
	OwnerID    string `json:"owner_id"`

	Enabled            bool `json:"enabled"`
	ProductionEligible bool `json:"production_eligible"`

	InventoryControlModeOverride string `json:"inventory_control_mode_override,omitempty"`
	InventoryItemID              string `json:"inventory_item_id,omitempty"`

	EffectiveInventoryControlMode string `json:"effective_inventory_control_mode"`
	InventoryMappingStatus        string `json:"inventory_mapping_status"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PutBranchConfigInput struct {
	Enabled            bool `json:"enabled"`
	ProductionEligible bool `json:"production_eligible"`

	InventoryControlModeOverride string `json:"inventory_control_mode_override,omitempty"`
	InventoryItemID              string `json:"inventory_item_id,omitempty"`
}

func normalizeCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeUnit(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeEnum(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func validMeasurementFamily(value string) bool {
	switch normalizeEnum(value) {
	case MeasurementMass,
		MeasurementVolume,
		MeasurementCount:
		return true
	default:
		return false
	}
}

func validInventoryControlMode(value string) bool {
	switch normalizeEnum(value) {
	case InventoryUntracked,
		InventoryControlled:
		return true
	default:
		return false
	}
}

func validMasterStatus(value string) bool {
	switch normalizeEnum(value) {
	case StatusDraft,
		StatusActive,
		StatusInactive,
		StatusArchived:
		return true
	default:
		return false
	}
}

func validConversionStatus(value string) bool {
	switch normalizeEnum(value) {
	case StatusActive,
		StatusInactive,
		StatusArchived:
		return true
	default:
		return false
	}
}

func validConversionMode(value string) bool {
	switch normalizeEnum(value) {
	case ConversionExactAuthoritative,
		ConversionDisplayOnly:
		return true
	default:
		return false
	}
}

func standardExactUnit(value string) bool {
	switch normalizeUnit(value) {
	case "G",
		"GRAM",
		"GRAMS",
		"KG",
		"KILOGRAM",
		"KILOGRAMS",
		"ML",
		"MILLILITRE",
		"MILLILITRES",
		"MILLILITER",
		"MILLILITERS",
		"L",
		"LITRE",
		"LITRES",
		"LITER",
		"LITERS",
		"EA",
		"EACH",
		"COUNT",
		"UNIT",
		"UNITS":
		return true
	default:
		return false
	}
}
