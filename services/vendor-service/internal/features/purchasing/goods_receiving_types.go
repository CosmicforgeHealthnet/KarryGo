package purchasing

import (
	"errors"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
)

const (
	PermPurchasingBaseRead            = "vendor.purchasing.read"
	PermPurchasingBaseManage          = "vendor.purchasing.manage"
	PermPurchasingReceive             = "vendor.purchasing.receive"
	PermPurchasingDiscrepancyManage   = "vendor.purchasing.discrepancies.manage"
	PermPurchasingCostRead            = "vendor.purchasing.cost.read"
	PermPurchasingCostVarianceApprove = "vendor.purchasing.cost_variance.approve"
)

const (
	GoodsReceiptDraft     = "DRAFT"
	GoodsReceiptInspected = "INSPECTED"
	GoodsReceiptPosted    = "POSTED"
	GoodsReceiptReversed  = "REVERSED"
	GoodsReceiptCancelled = "CANCELLED"
)

const (
	DiscrepancyShortDelivery = "SHORT_DELIVERY"
	DiscrepancyOverDelivery  = "OVER_DELIVERY"
	DiscrepancyDamaged       = "DAMAGED"
	DiscrepancyRejected      = "REJECTED"
	DiscrepancyWrongItem     = "WRONG_ITEM"
	DiscrepancyQualityIssue  = "QUALITY_ISSUE"
	DiscrepancyCostVariance  = "COST_VARIANCE"
)

const (
	DiscrepancyOpen             = "OPEN"
	DiscrepancyApprovalRequired = "APPROVAL_REQUIRED"
	DiscrepancyApproved         = "APPROVED"
	DiscrepancyRejectedStatus   = "REJECTED"
	DiscrepancyResolved         = "RESOLVED"
	DiscrepancyVoided           = "VOIDED"
)

const (
	OverDeliveryDeny               = "DENY"
	OverDeliveryAllowWithTolerance = "ALLOW_WITH_TOLERANCE"
)

var (
	ErrGoodsReceiptNotFound = errors.New("goods receipt not found")
	ErrGoodsReceiptState    = errors.New("goods receipt state conflict")
	ErrGoodsReceiptVersion  = errors.New("goods receipt version conflict")
	ErrGoodsReceiptInvalid  = errors.New("goods receipt input is invalid")

	ErrSupplierDiscrepancyNotFound = errors.New("supplier discrepancy not found")
	ErrSupplierDiscrepancyState    = errors.New("supplier discrepancy state conflict")

	ErrPurchasingOverDelivery    = errors.New("purchase order over-delivery is not permitted")
	ErrPurchasingExactConversion = errors.New("purchase quantity cannot be converted exactly into inventory quantity")
	ErrPurchasingIdempotency     = errors.New("goods receipt idempotency conflict")
	ErrPurchasingReversalUnsafe  = errors.New("goods receipt cannot be reversed because accepted inventory has changed")
)

type GoodsReceivingActor struct {
	StaffID       string
	BusinessID    string
	SessionID     string
	CorrelationID string
	CausationID   string
	CommandRef    string
	RequestID     string
}

func (a GoodsReceivingActor) InventoryActor() inventory.Actor {
	return inventory.Actor{
		ActorRef:      a.StaffID,
		StaffID:       a.StaffID,
		BusinessID:    a.BusinessID,
		CorrelationID: a.CorrelationID,
		CausationID:   a.CausationID,
		RequestID:     a.RequestID,
	}
}

type GoodsReceivingPolicy struct {
	ID string `json:"id"`

	BusinessID string  `json:"business_id"`
	BranchID   *string `json:"branch_id,omitempty"`

	OverDeliveryMode         string `json:"over_delivery_mode"`
	OverDeliveryToleranceBps int64  `json:"over_delivery_tolerance_bps"`

	CostVarianceApprovalBps   int64 `json:"cost_variance_approval_bps"`
	CostVarianceApprovalMinor int64 `json:"cost_variance_approval_minor"`

	RequireCostVarianceApproval bool `json:"require_cost_variance_approval"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PutGoodsReceivingPolicyInput struct {
	OverDeliveryMode         string `json:"over_delivery_mode"`
	OverDeliveryToleranceBps int64  `json:"over_delivery_tolerance_bps"`

	CostVarianceApprovalBps   int64 `json:"cost_variance_approval_bps"`
	CostVarianceApprovalMinor int64 `json:"cost_variance_approval_minor"`

	RequireCostVarianceApproval bool `json:"require_cost_variance_approval"`
}

type GoodsReceipt struct {
	ID string `json:"id"`

	BusinessID      string `json:"business_id"`
	PurchaseOrderID string `json:"purchase_order_id"`
	SupplierID      string `json:"supplier_id"`
	BranchID        string `json:"branch_id"`

	SupplierDeliveryReference string `json:"supplier_delivery_reference,omitempty"`
	InvoiceReference          string `json:"invoice_reference,omitempty"`
	InvoiceTotalMinor         *int64 `json:"invoice_total_minor,omitempty"`

	Currency string `json:"currency"`
	Status   string `json:"status"`

	ReceivedAt  time.Time  `json:"received_at"`
	InspectedAt *time.Time `json:"inspected_at,omitempty"`
	PostedAt    *time.Time `json:"posted_at,omitempty"`
	ReversedAt  *time.Time `json:"reversed_at,omitempty"`

	ReceivedByStaffID  string `json:"received_by_staff_id"`
	InspectedByStaffID string `json:"inspected_by_staff_id,omitempty"`
	ReversedByStaffID  string `json:"reversed_by_staff_id,omitempty"`

	InventoryReceiptID string `json:"inventory_receipt_id,omitempty"`

	Notes          string `json:"notes,omitempty"`
	ReversalReason string `json:"reversal_reason,omitempty"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Lines []GoodsReceiptLine `json:"lines,omitempty"`
}

type GoodsReceiptLine struct {
	ID string `json:"id"`

	GoodsReceiptID      string `json:"goods_receipt_id"`
	PurchaseOrderID     string `json:"purchase_order_id"`
	PurchaseOrderLineID string `json:"purchase_order_line_id,omitempty"`

	SupplierProductID string `json:"supplier_product_id,omitempty"`
	InventoryItemID   string `json:"inventory_item_id,omitempty"`

	PurchaseUnitCode              string `json:"purchase_unit_code"`
	InventoryUnitsPerPurchaseUnit int64  `json:"inventory_units_per_purchase_unit"`

	OrderedQuantitySnapshot *int64 `json:"ordered_quantity_snapshot,omitempty"`
	ReceivedBeforeSnapshot  *int64 `json:"received_before_snapshot,omitempty"`

	DeliveredQuantity int64 `json:"delivered_quantity"`
	AcceptedQuantity  int64 `json:"accepted_quantity"`
	DamagedQuantity   int64 `json:"damaged_quantity"`
	RejectedQuantity  int64 `json:"rejected_quantity"`
	ShortQuantity     int64 `json:"short_quantity"`

	InventoryQuantity int64 `json:"inventory_quantity"`

	WrongItem          bool   `json:"wrong_item"`
	WrongItemReference string `json:"wrong_item_reference,omitempty"`

	QualityIssue bool   `json:"quality_issue"`
	QualityNotes string `json:"quality_notes,omitempty"`

	ExpectedUnitCostMinor *int64 `json:"expected_unit_cost_minor,omitempty"`
	ActualUnitCostMinor   *int64 `json:"actual_unit_cost_minor,omitempty"`

	Currency string `json:"currency"`

	LotNumber   string `json:"lot_number,omitempty"`
	BatchNumber string `json:"batch_number,omitempty"`

	ManufacturedAt *time.Time `json:"manufactured_at,omitempty"`
	BestBeforeAt   *time.Time `json:"best_before_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`

	InventoryReceiptLineID string `json:"inventory_receipt_line_id,omitempty"`
	InventoryMovementID    string `json:"inventory_movement_id,omitempty"`
	InventoryLotID         string `json:"inventory_lot_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

type CreateGoodsReceiptInput struct {
	PurchaseOrderID string `json:"purchase_order_id"`

	SupplierDeliveryReference string `json:"supplier_delivery_reference,omitempty"`
	InvoiceReference          string `json:"invoice_reference,omitempty"`
	InvoiceTotalMinor         *int64 `json:"invoice_total_minor,omitempty"`

	ReceivedAt time.Time `json:"received_at"`

	Notes string `json:"notes,omitempty"`
}

type InspectGoodsReceiptLineInput struct {
	PurchaseOrderLineID string `json:"purchase_order_line_id,omitempty"`

	DeliveredQuantity int64 `json:"delivered_quantity"`
	AcceptedQuantity  int64 `json:"accepted_quantity"`
	DamagedQuantity   int64 `json:"damaged_quantity"`
	RejectedQuantity  int64 `json:"rejected_quantity"`
	ShortQuantity     int64 `json:"short_quantity"`

	WrongItem          bool   `json:"wrong_item"`
	WrongItemReference string `json:"wrong_item_reference,omitempty"`
	PurchaseUnitCode   string `json:"purchase_unit_code,omitempty"`

	QualityIssue bool   `json:"quality_issue"`
	QualityNotes string `json:"quality_notes,omitempty"`

	ActualUnitCostMinor *int64 `json:"actual_unit_cost_minor,omitempty"`

	LotNumber   string `json:"lot_number,omitempty"`
	BatchNumber string `json:"batch_number,omitempty"`

	ManufacturedAt *time.Time `json:"manufactured_at,omitempty"`
	BestBeforeAt   *time.Time `json:"best_before_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

type InspectGoodsReceiptInput struct {
	Lines []InspectGoodsReceiptLineInput `json:"lines"`
}

type ReverseGoodsReceiptInput struct {
	Reason string `json:"reason"`
}

type GoodsReceiptDocument struct {
	ID string `json:"id"`

	GoodsReceiptID string `json:"goods_receipt_id"`
	DocumentType   string `json:"document_type"`
	MediaRef       string `json:"media_ref"`

	ExternalReference string `json:"external_reference,omitempty"`

	AttachedByStaffID string    `json:"attached_by_staff_id"`
	CreatedAt         time.Time `json:"created_at"`
}

type AttachGoodsReceiptDocumentInput struct {
	DocumentType      string `json:"document_type"`
	MediaRef          string `json:"media_ref"`
	ExternalReference string `json:"external_reference,omitempty"`
}

type SupplierDiscrepancy struct {
	ID string `json:"id"`

	SupplierID string `json:"supplier_id"`

	PurchaseOrderID     string `json:"purchase_order_id"`
	PurchaseOrderLineID string `json:"purchase_order_line_id,omitempty"`

	GoodsReceiptID     string `json:"goods_receipt_id"`
	GoodsReceiptLineID string `json:"goods_receipt_line_id,omitempty"`

	BranchID string `json:"branch_id"`

	Type   string `json:"type"`
	Status string `json:"status"`

	Quantity *int64 `json:"quantity,omitempty"`

	ExpectedCostMinor *int64 `json:"expected_cost_minor,omitempty"`
	ActualCostMinor   *int64 `json:"actual_cost_minor,omitempty"`

	VarianceMinor *int64 `json:"variance_minor,omitempty"`
	VarianceBps   *int64 `json:"variance_bps,omitempty"`

	Currency string `json:"currency,omitempty"`

	ReasonCode string `json:"reason_code,omitempty"`
	Notes      string `json:"notes,omitempty"`

	OpenedByStaffID   string `json:"opened_by_staff_id"`
	DecidedByStaffID  string `json:"decided_by_staff_id,omitempty"`
	ResolvedByStaffID string `json:"resolved_by_staff_id,omitempty"`

	DecidedAt  *time.Time `json:"decided_at,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`

	Version int64 `json:"version"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DiscrepancyDecisionInput struct {
	Notes string `json:"notes,omitempty"`
}

type PurchaseCostHistory struct {
	ID string `json:"id"`

	BranchID string `json:"branch_id"`

	SupplierID        string `json:"supplier_id"`
	SupplierProductID string `json:"supplier_product_id"`
	InventoryItemID   string `json:"inventory_item_id"`

	PurchaseOrderID     string `json:"purchase_order_id"`
	PurchaseOrderLineID string `json:"purchase_order_line_id"`

	GoodsReceiptID     string `json:"goods_receipt_id"`
	GoodsReceiptLineID string `json:"goods_receipt_line_id"`

	EvidenceType string `json:"evidence_type"`

	PurchaseUnitCode              string `json:"purchase_unit_code"`
	InventoryUnitsPerPurchaseUnit int64  `json:"inventory_units_per_purchase_unit"`

	PurchaseQuantity  int64 `json:"purchase_quantity"`
	InventoryQuantity int64 `json:"inventory_quantity"`

	PurchaseUnitCostMinor *int64 `json:"purchase_unit_cost_minor"`
	TotalCostMinor        int64  `json:"total_cost_minor"`

	InventoryUnitCostMinor *int64 `json:"inventory_unit_cost_minor,omitempty"`

	Currency string `json:"currency"`

	ObservedAt time.Time `json:"observed_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type ConfirmGoodsReceiptResult struct {
	Receipt GoodsReceipt `json:"receipt"`

	InventoryReceiptID string `json:"inventory_receipt_id,omitempty"`

	AcceptedPurchaseQuantity  int64 `json:"accepted_purchase_quantity"`
	AcceptedInventoryQuantity int64 `json:"accepted_inventory_quantity"`

	DiscrepancyCount int `json:"discrepancy_count"`
	CostHistoryCount int `json:"cost_history_count"`

	Replayed bool `json:"replayed"`
}
