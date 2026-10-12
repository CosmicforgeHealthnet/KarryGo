package purchasing

import (
	"context"
	"time"
)

const (
	PermissionApproveBusiness = "vendor.purchasing.approve.business"

	PermissionApproveHighValue = "vendor.purchasing.approve.high_value"

	ApprovalScopeBranchOrBusiness = "BRANCH_OR_BUSINESS"

	ApprovalScopeBusinessOnly = "BUSINESS_ONLY"

	PurchaseOrderDraft = "DRAFT"

	PurchaseOrderSubmitted = "SUBMITTED"

	PurchaseOrderApproved = "APPROVED"

	PurchaseOrderOrdered = "ORDERED"

	PurchaseOrderPartiallyReceived = "PARTIALLY_RECEIVED"

	PurchaseOrderReceived = "RECEIVED"

	PurchaseOrderClosed = "CLOSED"

	PurchaseOrderRejected = "REJECTED"

	PurchaseOrderCancelled = "CANCELLED"

	ApprovalDecisionApproved = "APPROVED"

	ApprovalDecisionRejected = "REJECTED"

	StepUpPurposePurchaseOrderApprove = "purchasing.purchase_order.approve"
)

type StepUpConsumer interface {
	ConsumeGrant(
		ctx context.Context,
		grantID string,
		sessionID string,
		staffID string,
		vendorID string,
		purpose string,
	) error
}

type ApprovalPolicy struct {
	RulesComplete bool   `json:"rules_complete"`
	ID            string `json:"id"`
	BusinessID    string `json:"business_id"`

	Name string `json:"name"`

	AllowSelfApproval bool `json:"allow_self_approval"`

	Status string `json:"status"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Rules []ApprovalRule `json:"rules"`
}

type ApprovalRule struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`
	PolicyID   string `json:"policy_id"`

	BranchID *string `json:"branch_id,omitempty"`

	MinimumAmountMinor int64  `json:"minimum_amount_minor"`
	MaximumAmountMinor *int64 `json:"maximum_amount_minor,omitempty"`

	RequiredApprovalCount int `json:"required_approval_count"`

	ApprovalScope string `json:"approval_scope"`

	RequiredPermissionCode string `json:"required_permission_code"`

	StepUpRequired bool `json:"step_up_required"`

	Position int `json:"position"`

	PolicyVersion int64 `json:"policy_version"`

	Status  string `json:"status"`
	Version int64  `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ApprovalRuleInput struct {
	BranchID *string `json:"branch_id,omitempty"`

	MinimumAmountMinor int64  `json:"minimum_amount_minor"`
	MaximumAmountMinor *int64 `json:"maximum_amount_minor,omitempty"`

	RequiredApprovalCount int `json:"required_approval_count"`

	ApprovalScope string `json:"approval_scope"`

	RequiredPermissionCode string `json:"required_permission_code"`

	StepUpRequired bool `json:"step_up_required"`
}

type ReplaceApprovalPolicyInput struct {
	Name string `json:"name"`

	AllowSelfApproval bool `json:"allow_self_approval"`

	ExpectedVersion int64 `json:"expected_version"`

	Rules []ApprovalRuleInput `json:"rules"`
}

type PurchaseOrder struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`
	SupplierID string `json:"supplier_id"`

	PurchaseOrderNumber string `json:"purchase_order_number"`

	SupplierCodeSnapshot      string `json:"supplier_code_snapshot"`
	SupplierNameSnapshot      string `json:"supplier_name_snapshot"`
	SupplierLegalNameSnapshot string `json:"supplier_legal_name_snapshot"`

	PaymentTermsDaysSnapshot int    `json:"payment_terms_days_snapshot"`
	DeliveryTermsSnapshot    string `json:"delivery_terms_snapshot"`

	CurrencyCode string `json:"currency_code"`

	SubtotalMinor    int64 `json:"subtotal_minor"`
	TotalAmountMinor int64 `json:"total_amount_minor"`

	Status string `json:"status"`

	Notes string `json:"notes"`

	ExpectedDeliveryDate *time.Time `json:"expected_delivery_date,omitempty"`

	CreatedByStaffID   string `json:"created_by_staff_id"`
	SubmittedByStaffID string `json:"submitted_by_staff_id,omitempty"`
	OrderedByStaffID   string `json:"ordered_by_staff_id,omitempty"`
	CancelledByStaffID string `json:"cancelled_by_staff_id,omitempty"`
	ClosedByStaffID    string `json:"closed_by_staff_id,omitempty"`

	ApprovalPolicyID      string `json:"approval_policy_id,omitempty"`
	ApprovalPolicyVersion int64  `json:"approval_policy_version"`

	ApprovalRuleID      string `json:"approval_rule_id,omitempty"`
	ApprovalRuleVersion int64  `json:"approval_rule_version"`

	RequiredApprovalCount int `json:"required_approval_count"`

	ApprovalPermissionCode string `json:"approval_permission_code,omitempty"`
	ApprovalScope          string `json:"approval_scope,omitempty"`

	SelfApprovalAllowed    bool `json:"self_approval_allowed"`
	ApprovalStepUpRequired bool `json:"approval_step_up_required"`

	ApprovalGeneration int64 `json:"approval_generation"`
	ApprovalCount      int   `json:"approval_count"`

	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	ApprovedAt  *time.Time `json:"approved_at,omitempty"`
	OrderedAt   *time.Time `json:"ordered_at,omitempty"`
	ReceivedAt  *time.Time `json:"received_at,omitempty"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	RejectedAt  *time.Time `json:"rejected_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Lines []PurchaseOrderLine `json:"lines"`
}

type PurchaseOrderLine struct {
	ID string `json:"id"`

	BusinessID      string `json:"business_id"`
	PurchaseOrderID string `json:"purchase_order_id"`

	SupplierProductID      string `json:"supplier_product_id"`
	SupplierProductVersion int64  `json:"supplier_product_version"`

	InventoryItemID string `json:"inventory_item_id"`

	SupplierSKUSnapshot string `json:"supplier_sku_snapshot"`

	PurchaseUnitCodeSnapshot string `json:"purchase_unit_code_snapshot"`
	PurchaseUnitNameSnapshot string `json:"purchase_unit_name_snapshot"`

	ConversionModeSnapshot string `json:"conversion_mode_snapshot"`

	InventoryUnitsPerPurchaseUnitSnapshot int64 `json:"inventory_units_per_purchase_unit_snapshot"`

	InventoryMeasureRefSnapshot      string `json:"inventory_measure_ref_snapshot"`
	InventoryMeasureOwnerKeySnapshot string `json:"inventory_measure_owner_key_snapshot"`
	InventoryUnitKindSnapshot        string `json:"inventory_unit_kind_snapshot"`

	PurchaseQuantity int64 `json:"purchase_quantity"`

	ConvertedInventoryQuantity int64 `json:"converted_inventory_quantity"`

	UnitCostMinor  int64 `json:"unit_cost_minor"`
	LineTotalMinor int64 `json:"line_total_minor"`

	CurrencyCodeSnapshot string `json:"currency_code_snapshot"`

	ReceivedPurchaseQuantity int64 `json:"received_purchase_quantity"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PurchaseOrderApproval struct {
	ID string `json:"id"`

	BusinessID      string `json:"business_id"`
	PurchaseOrderID string `json:"purchase_order_id"`

	ApprovalGeneration int64 `json:"approval_generation"`

	ApproverStaffID string `json:"approver_staff_id"`

	Decision string `json:"decision"`

	PermissionCodeSnapshot string `json:"permission_code_snapshot"`
	ApprovalScopeSnapshot  string `json:"approval_scope_snapshot"`

	AmountMinorSnapshot int64 `json:"amount_minor_snapshot"`

	PurchaseOrderVersionBefore int64 `json:"purchase_order_version_before"`

	StepUpConsumed bool `json:"step_up_consumed"`

	Note string `json:"note"`

	CreatedAt time.Time `json:"created_at"`
}

type PurchaseOrderLineInput struct {
	SupplierProductID string `json:"supplier_product_id"`

	PurchaseQuantity int64 `json:"purchase_quantity"`

	// nil means use the current SupplierProduct typical cost.
	// Whatever value is resolved is snapshotted onto the PO line.
	UnitCostMinor *int64 `json:"unit_cost_minor,omitempty"`
}

type CreatePurchaseOrderInput struct {
	BranchID   string `json:"branch_id"`
	SupplierID string `json:"supplier_id"`

	CurrencyCode string `json:"currency_code"`

	ExpectedDeliveryDate *time.Time `json:"expected_delivery_date,omitempty"`

	Notes string `json:"notes"`

	Lines []PurchaseOrderLineInput `json:"lines"`
}

type UpdatePurchaseOrderInput struct {
	ExpectedDeliveryDate *time.Time `json:"expected_delivery_date,omitempty"`

	Notes string `json:"notes"`

	ExpectedVersion int64 `json:"expected_version"`
}

type ReplacePurchaseOrderLinesInput struct {
	Lines []PurchaseOrderLineInput `json:"lines"`

	ExpectedVersion int64 `json:"expected_version"`
}

type PurchaseOrderTransitionInput struct {
	ExpectedVersion int64 `json:"expected_version"`

	Note string `json:"note,omitempty"`
}

type PurchaseOrderApprovalInput struct {
	ExpectedVersion int64 `json:"expected_version"`

	StepUpGrantID string `json:"step_up_grant_id,omitempty"`

	Note string `json:"note,omitempty"`
}

type ReceivingProgressLine struct {
	PurchaseOrderLineID string

	// Absolute received quantity, not a delta.
	// This makes a Section 15C replay naturally idempotent.
	ReceivedPurchaseQuantity int64
}
