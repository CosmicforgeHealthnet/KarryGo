package inventory

import (
	"errors"
	"time"
)

const (
	MovementReceive             = "RECEIVE"
	MovementReserve             = "RESERVE"
	MovementRelease             = "RELEASE"
	MovementCommit              = "COMMIT"
	MovementAdjustmentIn        = "ADJUSTMENT_IN"
	MovementAdjustmentOut       = "ADJUSTMENT_OUT"
	MovementTransferOut         = "TRANSFER_OUT"
	MovementTransferIn          = "TRANSFER_IN"
	MovementReturn              = "RETURN"
	MovementDamage              = "DAMAGE"
	MovementWaste               = "WASTE"
	MovementExpiry              = "EXPIRY"
	MovementQuarantineIn        = "QUARANTINE_IN"
	MovementQuarantineRelease   = "QUARANTINE_RELEASE"
	MovementStocktakeCorrection = "STOCKTAKE_CORRECTION"
)

var (
	ErrStockNotFound        = errors.New("inventory stock balance not found")
	ErrInsufficientStock    = errors.New("inventory stock is insufficient")
	ErrInvalidQuantity      = errors.New("inventory quantity is invalid")
	ErrInvalidMovement      = errors.New("inventory movement is invalid")
	ErrReceiptNotFound      = errors.New("inventory receipt not found")
	ErrReceiptStateConflict = errors.New("inventory receipt state conflict")
	ErrReceiptNotReversible = errors.New("inventory posted receipt stock is no longer intact")
)

// Balance is generic Inventory physical stock truth.
//
// Available is derived and is never persisted as independently editable state.
type Balance struct {
	ID string `json:"id"`

	BusinessID      string `json:"business_id"`
	BranchID        string `json:"branch_id"`
	InventoryItemID string `json:"inventory_item_id"`

	// Name is the merchant-facing label of the stocked item (ingredient,
	// packaging material or shop product variant). Read-only; filled on
	// list reads and empty when the source cannot be resolved.
	Name string `json:"name,omitempty"`

	OnHandQuantity      int64 `json:"on_hand"`
	ReservedQuantity    int64 `json:"reserved"`
	QuarantinedQuantity int64 `json:"quarantined"`
	InTransitQuantity   int64 `json:"in_transit"`
	IncomingQuantity    int64 `json:"incoming"`
	BackorderQuantity   int64 `json:"backorder"`

	AvailableQuantity int64 `json:"available"`
	ExpiredQuantity   int64 `json:"expired_quantity"`

	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (b Balance) CalculatedAvailable() int64 {
	value :=
		b.OnHandQuantity -
			b.ReservedQuantity -
			b.QuarantinedQuantity

	if value < 0 {
		return 0
	}

	return value
}

type Lot struct {
	ID string `json:"id"`

	BusinessID      string `json:"business_id"`
	BranchID        string `json:"branch_id"`
	InventoryItemID string `json:"inventory_item_id"`

	LotNumber   string `json:"lot_number,omitempty"`
	BatchNumber string `json:"batch_number,omitempty"`

	SourceType string `json:"source_type"`
	SourceRef  string `json:"source_ref"`

	ReceivedAt     time.Time  `json:"received_at"`
	ManufacturedAt *time.Time `json:"manufactured_at,omitempty"`
	BestBeforeAt   *time.Time `json:"best_before_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`

	OnHandQuantity      int64 `json:"on_hand"`
	ReservedQuantity    int64 `json:"reserved"`
	QuarantinedQuantity int64 `json:"quarantined"`

	Status string `json:"status"`

	Version int64 `json:"version"`
}

type Movement struct {
	ID string `json:"id"`

	BusinessID      string `json:"business_id"`
	BranchID        string `json:"branch_id"`
	InventoryItemID string `json:"inventory_item_id"`

	LotID string `json:"lot_id,omitempty"`

	Type     string `json:"movement_type"`
	Quantity int64  `json:"quantity"`

	OnHandDelta      int64 `json:"on_hand_delta"`
	ReservedDelta    int64 `json:"reserved_delta"`
	QuarantinedDelta int64 `json:"quarantined_delta"`
	InTransitDelta   int64 `json:"in_transit_delta"`
	IncomingDelta    int64 `json:"incoming_delta"`
	BackorderDelta   int64 `json:"backorder_delta"`

	SourceType string `json:"source_type"`
	SourceRef  string `json:"source_ref"`

	ReasonCode string `json:"reason_code,omitempty"`
	ActorRef   string `json:"actor_ref,omitempty"`

	OccurredAt time.Time `json:"occurred_at"`

	InventoryRevision int64 `json:"inventory_revision"`
	BalanceRevision   int64 `json:"balance_revision"`
}

type LotMetadata struct {
	LotNumber   string
	BatchNumber string

	ManufacturedAt *time.Time
	BestBeforeAt   *time.Time
	ExpiresAt      *time.Time
}

type ReceiveInput struct {
	BranchID        string
	InventoryItemID string
	Quantity        int64

	SourceType string
	SourceRef  string

	ReasonCode string

	Lot LotMetadata
}

type QuantityMutationInput struct {
	BranchID        string
	InventoryItemID string
	Quantity        int64

	SourceType string
	SourceRef  string

	ReasonCode string
}

// ApplyMovementProjection is the deterministic projection rule used by tests,
// reconciliation tooling and future repair diagnostics.
//
// It never permits an impossible physical result.
func ApplyMovementProjection(
	balance Balance,
	movement Movement,
) (Balance, error) {
	if movement.Quantity <= 0 {
		return Balance{}, ErrInvalidMovement
	}

	balance.OnHandQuantity +=
		movement.OnHandDelta

	balance.ReservedQuantity +=
		movement.ReservedDelta

	balance.QuarantinedQuantity +=
		movement.QuarantinedDelta

	balance.InTransitQuantity +=
		movement.InTransitDelta

	balance.IncomingQuantity +=
		movement.IncomingDelta

	balance.BackorderQuantity +=
		movement.BackorderDelta

	if balance.OnHandQuantity < 0 ||
		balance.ReservedQuantity < 0 ||
		balance.QuarantinedQuantity < 0 ||
		balance.InTransitQuantity < 0 ||
		balance.IncomingQuantity < 0 ||
		balance.BackorderQuantity < 0 {

		return Balance{}, ErrInvalidMovement
	}

	if balance.ReservedQuantity+
		balance.QuarantinedQuantity >
		balance.OnHandQuantity {

		return Balance{}, ErrInvalidMovement
	}

	balance.AvailableQuantity =
		balance.CalculatedAvailable()

	if movement.BalanceRevision > 0 {
		balance.Revision =
			movement.BalanceRevision
	}

	return balance, nil
}
