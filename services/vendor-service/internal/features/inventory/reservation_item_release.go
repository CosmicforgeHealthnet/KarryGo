package inventory

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ReleaseReservationItem releases only one unavailable item's outstanding
// hold. Sibling reservation lines remain held and can still be fulfilled.
func (s *StockService) ReleaseReservationItem(ctx context.Context, actor Actor, scope BranchScope, reservationID, inventoryItemID, idempotencyKey string) error {
	if err := s.requireDependencies(); err != nil {
		return err
	}
	reservationID = strings.TrimSpace(reservationID)
	inventoryItemID = strings.TrimSpace(inventoryItemID)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if reservationID == "" || inventoryItemID == "" || idempotencyKey == "" {
		return ErrReservationNotFound
	}
	branchID, err := reservationBranchQ(ctx, s.db, actor.BusinessID, reservationID)
	if err != nil {
		return err
	}
	return s.withWriteTransaction(ctx, actor, scope, branchID, func(inventoryTx InventoryTransaction, tx pgx.Tx) error {
		states, lockErr := lockReservationItemsTx(ctx, tx, actor.BusinessID, branchID, reservationID)
		if lockErr != nil {
			return lockErr
		}
		item, ok := states[inventoryItemID]
		if !ok {
			return ErrReservationNotFound
		}
		reservation, loadErr := loadReservationQ(ctx, tx, actor.BusinessID, reservationID, true)
		if loadErr != nil {
			return loadErr
		}
		var line ReservationLine
		for _, candidate := range reservation.Lines {
			if candidate.InventoryItemID == inventoryItemID {
				line = candidate
				break
			}
		}
		if line.ID == "" {
			return ErrReservationNotFound
		}
		if reservation.Status != ReservationHeld && reservation.Status != ReservationConfirmed {
			for _, allocation := range line.Allocations {
				var committed, released int64
				if loadErr = tx.QueryRow(ctx, `SELECT committed_quantity,released_quantity FROM inventory_reservation_allocations WHERE id=$1 AND reservation_id=$2`, allocation.ID, reservationID).Scan(&committed, &released); loadErr != nil {
					return loadErr
				}
				if committed != 0 || released != allocation.Quantity {
					return ErrReservationStateConflict
				}
			}
			return nil
		}
		var released int64
		for _, allocation := range line.Allocations {
			var committed, alreadyReleased int64
			if loadErr = tx.QueryRow(ctx, `SELECT committed_quantity,released_quantity FROM inventory_reservation_allocations WHERE id=$1 AND reservation_id=$2 FOR UPDATE`, allocation.ID, reservationID).Scan(&committed, &alreadyReleased); loadErr != nil {
				return loadErr
			}
			if committed != 0 || alreadyReleased < 0 || alreadyReleased > allocation.Quantity {
				return ErrReservationStateConflict
			}
			delta := allocation.Quantity - alreadyReleased
			if delta == 0 {
				continue
			}
			tag, updateErr := tx.Exec(ctx, `UPDATE inventory_lots SET reserved_quantity=reserved_quantity-$3,version=version+1,updated_at=now() WHERE id=$1 AND business_id=$2 AND reserved_quantity >= $3`, allocation.LotID, actor.BusinessID, delta)
			if updateErr != nil {
				return updateErr
			}
			if tag.RowsAffected() != 1 {
				return ErrInsufficientStock
			}
			if _, updateErr = tx.Exec(ctx, `UPDATE inventory_reservation_allocations SET released_quantity=quantity,status=$1,updated_at=now() WHERE id=$2 AND reservation_id=$3`, ReservationReleased, allocation.ID, reservationID); updateErr != nil {
				return updateErr
			}
			balanceRevision, updateErr := updateBalanceTx(ctx, tx, item, 0, -delta, 0, 0, 0, 0)
			if updateErr != nil {
				return updateErr
			}
			inventoryRevision, updateErr := bumpInventoryRevisionTx(ctx, tx, actor.BusinessID, branchID)
			if updateErr != nil {
				return updateErr
			}
			if _, updateErr = appendMovementTx(ctx, tx, actor, item, allocation.LotID, MovementRelease, delta, 0, -delta, 0, 0, 0, 0, "COMMERCE_AUTO_REFUND", reservationID, idempotencyKey, inventoryRevision, balanceRevision); updateErr != nil {
				return updateErr
			}
			released += delta
		}
		if released == 0 {
			return nil
		}
		if _, loadErr = tx.Exec(ctx, `UPDATE inventory_reservation_lines SET status=$1,updated_at=now() WHERE id=$2 AND business_id=$3`, ReservationReleased, line.ID, actor.BusinessID); loadErr != nil {
			return loadErr
		}
		var version int64
		if loadErr = tx.QueryRow(ctx, `UPDATE inventory_reservations SET version=version+1,updated_at=now() WHERE id=$1 AND business_id=$2 RETURNING version`, reservationID, actor.BusinessID).Scan(&version); errors.Is(loadErr, pgx.ErrNoRows) {
			return ErrReservationNotFound
		} else if loadErr != nil {
			return loadErr
		}
		return s.recordMutation(ctx, inventoryTx, actor, scope, "inventory.reservation.item_released", "inventory.reservation.item_released", "inventory_reservation", reservationID, "COMMERCE_AUTO_REFUND", version, map[string]any{"inventory_item_id": inventoryItemID, "released_quantity": released, "idempotency_key": idempotencyKey})
	})
}
