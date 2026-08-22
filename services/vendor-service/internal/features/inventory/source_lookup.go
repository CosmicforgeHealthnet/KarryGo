package inventory

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// FindReservationBySource resolves a reservation from its durable source
// reference alone.
//
// Reservations are already keyed on (business_id, source_type, source_ref) so
// that a replayed request is idempotent. This read exposes the same key to a
// caller that has lost the reservation identity - a crashed coordinator that
// committed its source reference before calling Inventory, and now has to
// find out whether a hold was actually created before it can release it.
//
// An absent reservation is an empty identity and no error: "nothing was
// created" is a legitimate, expected answer to that question.
func (s *StockService) FindReservationBySource(
	ctx context.Context,
	businessID string,
	sourceType string,
	sourceRef string,
) (
	string,
	string,
	error,
) {
	if s == nil || s.db == nil {
		return "", "", ErrDependencyMissing
	}

	businessID = strings.TrimSpace(businessID)
	sourceType = strings.ToUpper(strings.TrimSpace(sourceType))
	sourceRef = strings.TrimSpace(sourceRef)

	if businessID == "" || sourceRef == "" {
		return "", "", ErrInvalidSource
	}

	switch sourceType {
	case ReservationSourceOrder,
		ReservationSourceCheckout,
		ReservationSourceRFQ,
		ReservationSourceManual,
		ReservationSourceInternal:
	default:
		return "", "", ErrReservationSourceInvalid
	}

	var (
		reservationID string
		status        string
	)

	err := s.db.QueryRow(
		ctx,
		`
        SELECT
            id,
            status
          FROM inventory_reservations
         WHERE business_id=$1
           AND source_type=$2
           AND source_ref=$3
        `,
		businessID,
		sourceType,
		sourceRef,
	).Scan(
		&reservationID,
		&status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}

	if err != nil {
		return "", "", err
	}

	return reservationID, status, nil
}

// ReservationIsTerminal reports whether a reservation status can no longer be
// transitioned.
//
// Callers that release defensively need this: releasing an already-committed,
// released, expired or cancelled reservation is a success, not a fault, and
// treating it as an error would turn every idempotent retry into a failure.
func ReservationIsTerminal(status string) bool {
	return reservationTerminal(
		strings.ToUpper(strings.TrimSpace(status)),
	)
}

// ResolveShopVariantItem resolves the generic Inventory identity of one Shop
// variant in one branch.
//
// Section 14C established that the generic identity is created alongside the
// Shop stock basis and keyed on (business_id, branch_id, source_ref) for
// item_type SHOP_VARIANT. This is the read that turns a catalogue selection
// into the stock identity a reservation is made against.
//
// A variant with no generic identity returns ErrItemNotFound. That is a real
// answer, and callers must fail closed on it rather than reserving against a
// guess.
func (s *StockService) ResolveShopVariantItem(
	ctx context.Context,
	businessID string,
	branchID string,
	variantID string,
) (
	InventoryItem,
	error,
) {
	if s == nil || s.db == nil {
		return InventoryItem{}, ErrDependencyMissing
	}

	businessID = strings.TrimSpace(businessID)
	branchID = strings.TrimSpace(branchID)
	variantID = strings.TrimSpace(variantID)

	if businessID == "" || branchID == "" || variantID == "" {
		return InventoryItem{}, ErrInvalidSource
	}

	var item InventoryItem

	err := s.db.QueryRow(
		ctx,
		`
        SELECT
            id,
            business_id,
            branch_id,
            item_type,
            source_ref,
            stock_measure_ref,
            stock_measure_owner_key,
            stock_unit_kind,
            stock_basis_mode,
            tracking_mode,
            lot_policy,
            lot_tracking_enabled,
            status,
            version,
            created_at,
            updated_at
          FROM inventory_items
         WHERE business_id=$1
           AND branch_id=$2
           AND item_type='SHOP_VARIANT'
           AND source_ref=$3
           AND status='ACTIVE'
        `,
		businessID,
		branchID,
		variantID,
	).Scan(
		&item.ID,
		&item.BusinessID,
		&item.BranchID,
		&item.ItemType,
		&item.SourceRef,
		&item.StockMeasureRef,
		&item.StockMeasureOwnerKey,
		&item.StockUnitKind,
		&item.StockBasisMode,
		&item.TrackingMode,
		&item.LotPolicy,
		&item.LotTrackingEnabled,
		&item.Status,
		&item.Version,
		&item.CreatedAt,
		&item.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return InventoryItem{}, ErrItemNotFound
	}

	if err != nil {
		return InventoryItem{}, err
	}

	return item, nil
}
