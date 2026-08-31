package inventory

import (
	"errors"
	"time"
)

const (
	ReservationHeld      = "HELD"
	ReservationConfirmed = "CONFIRMED"
	ReservationCommitted = "COMMITTED"
	ReservationReleased  = "RELEASED"
	ReservationExpired   = "EXPIRED"
	ReservationCancelled = "CANCELLED"
)

const (
	ReservationSourceOrder    = "ORDER"
	ReservationSourceCheckout = "CHECKOUT"
	ReservationSourceRFQ      = "RFQ"
	ReservationSourceManual   = "MANUAL"
	ReservationSourceInternal = "INTERNAL"
)

const ReservationAllocationAllOrNothing = "ALL_OR_NOTHING"

var (
	ErrReservationNotFound = errors.New(
		"inventory reservation not found",
	)

	ErrReservationStateConflict = errors.New(
		"inventory reservation state conflict",
	)

	ErrReservationIdempotencyConflict = errors.New(
		"inventory reservation source reference was reused with a different request fingerprint",
	)

	ErrReservationExpired = errors.New(
		"inventory reservation has expired",
	)

	ErrReservationStockExpired = errors.New(
		"inventory reservation contains expired stock",
	)

	ErrReservationSourceInvalid = errors.New(
		"inventory reservation source is invalid",
	)
)

type ReservationLineInput struct {
	InventoryItemID string `json:"inventory_item_id"`
	Quantity        int64  `json:"quantity"`
}

type CreateReservationInput struct {
	BranchID string `json:"branch_id"`

	SourceType string `json:"source_type"`
	SourceRef  string `json:"source_ref"`

	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	Lines []ReservationLineInput `json:"lines"`
}

type ReservationAllocation struct {
	ID string `json:"id"`

	InventoryItemID string `json:"inventory_item_id"`
	LotID           string `json:"lot_id"`

	Quantity int64 `json:"quantity"`

	Status string `json:"status"`
}

type ReservationLine struct {
	ID string `json:"id"`

	InventoryItemID string `json:"inventory_item_id"`

	RequestedQuantity int64 `json:"requested_quantity"`
	PhysicalQuantity  int64 `json:"physical_quantity"`
	BackorderQuantity int64 `json:"backorder_quantity"`

	Status string `json:"status"`

	Allocations []ReservationAllocation `json:"allocations"`
}

type Reservation struct {
	ID string `json:"id"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	SourceType string `json:"source_type"`
	SourceRef  string `json:"source_ref"`

	RequestFingerprint string `json:"request_fingerprint"`

	AllocationPolicy string `json:"allocation_policy"`

	Status string `json:"status"`

	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	HeldAt      time.Time  `json:"held_at"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	CommittedAt *time.Time `json:"committed_at,omitempty"`
	ReleasedAt  *time.Time `json:"released_at,omitempty"`
	ExpiredAt   *time.Time `json:"expired_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Version int64 `json:"version"`

	Lines []ReservationLine `json:"lines"`
}

func reservationTerminal(
	status string,
) bool {
	switch status {
	case ReservationCommitted,
		ReservationReleased,
		ReservationExpired,
		ReservationCancelled:
		return true
	default:
		return false
	}
}
