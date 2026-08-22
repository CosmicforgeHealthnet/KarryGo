package inventory

import (
	"errors"
	"time"
)

const (
	StocktakeFull  = "FULL_STOCKTAKE"
	StocktakeCycle = "CYCLE_COUNT"

	StocktakeCounting  = "COUNTING"
	StocktakePosted    = "POSTED"
	StocktakeStale     = "STALE"
	StocktakeCancelled = "CANCELLED"
)

const (
	TransferDraft             = "DRAFT"
	TransferApproved          = "APPROVED"
	TransferInTransit         = "IN_TRANSIT"
	TransferPartiallyReceived = "PARTIALLY_RECEIVED"
	TransferReceived          = "RECEIVED"
	TransferCancelled         = "CANCELLED"
)

const (
	TransferLinePending           = "PENDING"
	TransferLineInTransit         = "IN_TRANSIT"
	TransferLinePartiallyReceived = "PARTIALLY_RECEIVED"
	TransferLineReceived          = "RECEIVED"
	TransferLineCancelled         = "CANCELLED"
)

const (
	QuarantineActive   = "ACTIVE"
	QuarantineReleased = "RELEASED"
	QuarantineDisposed = "DISPOSED"
)

const (
	ReorderActive   = "ACTIVE"
	ReorderInactive = "INACTIVE"
	ReorderArchived = "ARCHIVED"
)

const (
	AlertLowStock          = "LOW_STOCK"
	AlertOutOfStock        = "OUT_OF_STOCK"
	AlertExpiringSoon      = "EXPIRING_SOON"
	AlertExpired           = "EXPIRED"
	AlertBackorder         = "BACKORDER"
	AlertBalanceMismatch   = "BALANCE_MISMATCH"
	AlertTransferException = "TRANSFER_EXCEPTION"

	AlertOpen         = "OPEN"
	AlertAcknowledged = "ACKNOWLEDGED"
	AlertResolved     = "RESOLVED"
)

var (
	ErrStocktakeNotFound = errors.New(
		"inventory stocktake not found",
	)

	ErrStocktakeStateConflict = errors.New(
		"inventory stocktake state conflict",
	)

	ErrStocktakeStale = errors.New(
		"inventory moved after the stocktake snapshot",
	)

	ErrStocktakeCountIncomplete = errors.New(
		"inventory stocktake contains uncounted lines",
	)

	ErrTransferNotFound = errors.New(
		"inventory transfer not found",
	)

	ErrTransferStateConflict = errors.New(
		"inventory transfer state conflict",
	)

	ErrTransferIdentityMismatch = errors.New(
		"inventory transfer items do not represent the same stock identity",
	)

	ErrTransferCrossTenant = errors.New(
		"inventory transfer cannot cross business ownership",
	)

	ErrTransferIdempotencyConflict = errors.New(
		"inventory transfer receipt source was reused with different details",
	)

	ErrQuarantineNotFound = errors.New(
		"inventory quarantine not found",
	)

	ErrQuarantineStateConflict = errors.New(
		"inventory quarantine state conflict",
	)

	ErrReorderRuleInvalid = errors.New(
		"inventory reorder rule is invalid",
	)

	ErrAlertNotFound = errors.New(
		"inventory alert not found",
	)

	ErrReasonRequired = errors.New(
		"inventory controlled loss requires a reason code",
	)
)

type StocktakeLine struct {
	ID string `json:"id"`

	InventoryItemID string `json:"inventory_item_id"`

	ExpectedQuantity int64  `json:"expected_quantity"`
	CountedQuantity  *int64 `json:"counted_quantity,omitempty"`
	VarianceQuantity *int64 `json:"variance_quantity,omitempty"`
}

type Stocktake struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	Type string `json:"stocktake_type"`

	SnapshotRevision int64 `json:"snapshot_revision"`

	Status string `json:"status"`

	StartedAt   time.Time  `json:"started_at"`
	PostedAt    *time.Time `json:"posted_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`

	Version int64 `json:"version"`

	Lines []StocktakeLine `json:"lines"`
}

type CreateStocktakeInput struct {
	BranchID string `json:"branch_id"`

	Type string `json:"stocktake_type"`

	InventoryItemIDs []string `json:"inventory_item_ids,omitempty"`
}

type TransferLineInput struct {
	SourceInventoryItemID      string `json:"source_inventory_item_id"`
	DestinationInventoryItemID string `json:"destination_inventory_item_id"`
	Quantity                   int64  `json:"quantity"`
}

type CreateTransferInput struct {
	SourceBranchID      string `json:"source_branch_id"`
	DestinationBranchID string `json:"destination_branch_id"`

	Notes string `json:"notes"`

	Lines []TransferLineInput `json:"lines"`
}

type TransferLine struct {
	ID string `json:"id"`

	SourceInventoryItemID      string `json:"source_inventory_item_id"`
	DestinationInventoryItemID string `json:"destination_inventory_item_id"`

	SentQuantity     int64 `json:"sent_quantity"`
	ReceivedQuantity int64 `json:"received_quantity"`
	DamagedQuantity  int64 `json:"damaged_quantity"`
	MissingQuantity  int64 `json:"missing_quantity"`

	Status string `json:"status"`
}

type Transfer struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`

	SourceBranchID      string `json:"source_branch_id"`
	DestinationBranchID string `json:"destination_branch_id"`

	Status string `json:"status"`
	Notes  string `json:"notes"`

	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	DispatchedAt *time.Time `json:"dispatched_at,omitempty"`
	ReceivedAt   *time.Time `json:"received_at,omitempty"`
	CancelledAt  *time.Time `json:"cancelled_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Version int64 `json:"version"`

	Lines []TransferLine `json:"lines"`
}

type ReceiveTransferLineInput struct {
	LineID string `json:"line_id"`

	ReceivedQuantity int64 `json:"received_quantity"`
	DamagedQuantity  int64 `json:"damaged_quantity"`
	MissingQuantity  int64 `json:"missing_quantity"`
}

type ReceiveTransferInput struct {
	SourceRef string `json:"source_ref"`

	Lines []ReceiveTransferLineInput `json:"lines"`
}

type QuarantineCase struct {
	ID string `json:"id"`

	BusinessID      string `json:"business_id"`
	BranchID        string `json:"branch_id"`
	InventoryItemID string `json:"inventory_item_id"`

	Quantity int64 `json:"quantity"`

	ReasonCode string `json:"reason_code"`
	Note       string `json:"note,omitempty"`

	Status string `json:"status"`

	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`

	Version int64 `json:"version"`
}

type CreateQuarantineInput struct {
	BranchID        string `json:"branch_id"`
	InventoryItemID string `json:"inventory_item_id"`

	Quantity int64 `json:"quantity"`

	ReasonCode string `json:"reason_code"`
	Note       string `json:"note"`
}

type ReorderRule struct {
	ID string `json:"id"`

	BusinessID      string `json:"business_id"`
	BranchID        string `json:"branch_id"`
	InventoryItemID string `json:"inventory_item_id"`

	MinimumQuantity int64 `json:"minimum_quantity"`
	TargetQuantity  int64 `json:"target_quantity"`

	ExpiringSoonHours *int `json:"expiring_soon_hours,omitempty"`

	SupplierRef string `json:"supplier_ref,omitempty"`

	Status string `json:"status"`

	Version int64 `json:"version"`
}

type SetReorderRuleInput struct {
	BranchID        string `json:"branch_id"`
	InventoryItemID string `json:"inventory_item_id"`

	MinimumQuantity int64 `json:"minimum_quantity"`
	TargetQuantity  int64 `json:"target_quantity"`

	ExpiringSoonHours *int `json:"expiring_soon_hours,omitempty"`

	SupplierRef string `json:"supplier_ref,omitempty"`

	Status string `json:"status"`
}

type InventoryAlert struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	InventoryItemID string `json:"inventory_item_id,omitempty"`

	Type     string `json:"alert_type"`
	Severity string `json:"severity"`

	DedupeKey string         `json:"dedupe_key"`
	Details   map[string]any `json:"details"`

	Status string `json:"status"`

	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}
