package purchasing

import (
	"context"
	"strings"
	"time"
)

const (
	PermissionRead    = "vendor.purchasing.read"
	PermissionManage  = "vendor.purchasing.manage"
	PermissionApprove = "vendor.purchasing.approve"

	StatusActive    = "ACTIVE"
	StatusInactive  = "INACTIVE"
	StatusSuspended = "SUSPENDED"
	StatusArchived  = "ARCHIVED"

	ContactRolePrimary    = "PRIMARY"
	ContactRoleSales      = "SALES"
	ContactRoleOperations = "OPERATIONS"
	ContactRoleFinance    = "FINANCE"
	ContactRoleOther      = "OTHER"

	ConversionSameAsInventory = "SAME_AS_INVENTORY"
	ConversionExactInteger    = "EXACT_INTEGER_MULTIPLE"
)

type Actor struct {
	StaffID       string
	VendorID      string
	SessionID     string
	CorrelationID string
	CausationID   string
	CommandRef    string
}

func (a Actor) Validate() bool {
	return strings.TrimSpace(a.StaffID) != "" &&
		strings.TrimSpace(a.VendorID) != ""
}

type Scope struct {
	Global    bool
	BranchIDs []string
}

func (s Scope) Covers(branchID string) bool {
	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		return false
	}
	if s.Global {
		return true
	}
	for _, candidate := range s.BranchIDs {
		if strings.TrimSpace(candidate) == branchID {
			return true
		}
	}
	return false
}

func (s Scope) HasAny() bool {
	return s.Global || len(s.BranchIDs) > 0
}

type ScopeResolver interface {
	ResolveScope(
		ctx context.Context,
		staffID string,
		permission string,
	) (Scope, error)
}

type SupplierCategory struct {
	ID          string    `json:"id"`
	BusinessID  string    `json:"business_id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Supplier struct {
	ID                   string    `json:"id"`
	BusinessID           string    `json:"business_id"`
	CategoryID           *string   `json:"category_id,omitempty"`
	Code                 string    `json:"code"`
	Name                 string    `json:"name"`
	LegalName            string    `json:"legal_name"`
	DefaultLeadTimeHours int       `json:"default_lead_time_hours"`
	Status               string    `json:"status"`
	Notes                string    `json:"notes"`
	Version              int64     `json:"version"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type SupplierContact struct {
	ID          string    `json:"id"`
	BusinessID  string    `json:"business_id"`
	SupplierID  string    `json:"supplier_id"`
	ContactRole string    `json:"contact_role"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	IsPrimary   bool      `json:"is_primary"`
	Status      string    `json:"status"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SupplierTerms struct {
	SupplierID       string    `json:"supplier_id"`
	BusinessID       string    `json:"business_id"`
	PaymentTermsDays int       `json:"payment_terms_days"`
	CurrencyCode     string    `json:"currency_code"`
	DeliveryTerms    string    `json:"delivery_terms"`
	Version          int64     `json:"version"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type SupplierProduct struct {
	ID              string `json:"id"`
	BusinessID      string `json:"business_id"`
	SupplierID      string `json:"supplier_id"`
	BranchID        string `json:"branch_id"`
	InventoryItemID string `json:"inventory_item_id"`

	SupplierSKU      string `json:"supplier_sku"`
	PurchaseUnitCode string `json:"purchase_unit_code"`
	PurchaseUnitName string `json:"purchase_unit_name"`

	ConversionMode                string `json:"conversion_mode"`
	InventoryUnitsPerPurchaseUnit int64  `json:"inventory_units_per_purchase_unit"`

	MinimumOrderQuantity int64 `json:"minimum_order_quantity"`
	PurchaseMultiple     int64 `json:"purchase_multiple"`

	TypicalCostMinor *int64 `json:"typical_cost_minor,omitempty"`
	LastCostMinor    *int64 `json:"last_cost_minor,omitempty"`

	CurrencyCode string `json:"currency_code"`

	LeadTimeHours *int `json:"lead_time_hours,omitempty"`

	InventoryMeasureRef      string `json:"inventory_measure_ref"`
	InventoryMeasureOwnerKey string `json:"inventory_measure_owner_key"`
	InventoryUnitKind        string `json:"inventory_unit_kind"`

	Status string `json:"status"`

	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateCategoryInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UpdateCategoryInput struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	ExpectedVersion int64  `json:"expected_version"`
}

type CreateSupplierInput struct {
	CategoryID           *string `json:"category_id,omitempty"`
	Code                 string  `json:"code"`
	Name                 string  `json:"name"`
	LegalName            string  `json:"legal_name"`
	DefaultLeadTimeHours int     `json:"default_lead_time_hours"`
	Status               string  `json:"status"`
	Notes                string  `json:"notes"`
}

type UpdateSupplierInput struct {
	CategoryID           *string `json:"category_id,omitempty"`
	Code                 string  `json:"code"`
	Name                 string  `json:"name"`
	LegalName            string  `json:"legal_name"`
	DefaultLeadTimeHours int     `json:"default_lead_time_hours"`
	Status               string  `json:"status"`
	Notes                string  `json:"notes"`
	ExpectedVersion      int64   `json:"expected_version"`
}

type CreateContactInput struct {
	ContactRole string `json:"contact_role"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	IsPrimary   bool   `json:"is_primary"`
}

type UpdateContactInput struct {
	ContactRole     string `json:"contact_role"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	Phone           string `json:"phone"`
	IsPrimary       bool   `json:"is_primary"`
	ExpectedVersion int64  `json:"expected_version"`
}

type UpsertTermsInput struct {
	PaymentTermsDays int    `json:"payment_terms_days"`
	CurrencyCode     string `json:"currency_code"`
	DeliveryTerms    string `json:"delivery_terms"`
	ExpectedVersion  int64  `json:"expected_version"`
}

type CreateSupplierProductInput struct {
	BranchID        string `json:"branch_id"`
	InventoryItemID string `json:"inventory_item_id"`

	SupplierSKU      string `json:"supplier_sku"`
	PurchaseUnitCode string `json:"purchase_unit_code"`
	PurchaseUnitName string `json:"purchase_unit_name"`

	ConversionMode                string `json:"conversion_mode"`
	InventoryUnitsPerPurchaseUnit int64  `json:"inventory_units_per_purchase_unit"`

	MinimumOrderQuantity int64 `json:"minimum_order_quantity"`
	PurchaseMultiple     int64 `json:"purchase_multiple"`

	TypicalCostMinor *int64 `json:"typical_cost_minor,omitempty"`
	CurrencyCode     string `json:"currency_code"`
	LeadTimeHours    *int   `json:"lead_time_hours,omitempty"`
}

type UpdateSupplierProductInput struct {
	SupplierSKU string `json:"supplier_sku"`

	MinimumOrderQuantity int64 `json:"minimum_order_quantity"`
	PurchaseMultiple     int64 `json:"purchase_multiple"`

	TypicalCostMinor *int64 `json:"typical_cost_minor,omitempty"`
	CurrencyCode     string `json:"currency_code"`
	LeadTimeHours    *int   `json:"lead_time_hours,omitempty"`

	ExpectedVersion int64 `json:"expected_version"`
}

type VersionCommand struct {
	ExpectedVersion int64 `json:"expected_version"`
}
