package inventory

// Partial reservation finalisation is deliberately implemented in the
// Inventory authority package. Commerce supplies only the fulfilled quantity
// per inventory item; this code owns lot locking, stock movements, balances and
// retry semantics.

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// FinalizeReservationPartial commits fulfilled units and releases the
// remainder of one reservation in one Inventory transaction. A reservation
// created by the existing authority remains all-or-nothing at hold time; the
// split is recorded per allocation at finalisation.
func (s *StockService) FinalizeReservationPartial(ctx context.Context, actor Actor, scope BranchScope, reservationID string, fulfilled map[string]int64) (Reservation, error) {
	if err := s.requireDependencies(); err != nil {
		return Reservation{}, err
	}
	reservationID = strings.TrimSpace(reservationID)
	if reservationID == "" {
		return Reservation{}, ErrReservationNotFound
	}
	branchID, err := reservationBranchQ(ctx, s.db, actor.BusinessID, reservationID)
	if err != nil {
		return Reservation{}, err
	}
	var out Reservation
	err = s.withWriteTransaction(ctx, actor, scope, branchID, func(_ InventoryTransaction, tx pgx.Tx) error {
		states, err := lockReservationItemsTx(ctx, tx, actor.BusinessID, branchID, reservationID)
		if err != nil {
			return err
		}
		reservation, err := loadReservationQ(ctx, tx, actor.BusinessID, reservationID, true)
		if err != nil {
			return err
		}
		if reservation.Status == ReservationCommitted || reservation.Status == ReservationReleased {
			// Finalisation is idempotent once every allocation has a settled
			// committed/released split. Do not permit a second quantity opinion.
			var unsettled int
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM inventory_reservation_allocations WHERE reservation_id=$1 AND business_id=$2 AND committed_quantity+released_quantity<quantity`, reservationID, actor.BusinessID).Scan(&unsettled); err != nil {
				return err
			}
			if unsettled == 0 {
				out = reservation
				return nil
			}
			return ErrReservationStateConflict
		}
		if reservation.Status != ReservationHeld && reservation.Status != ReservationConfirmed {
			return ErrReservationStateConflict
		}
		for item, amount := range fulfilled {
			if amount < 0 {
				return ErrInsufficientStock
			}
			found := false
			for _, line := range reservation.Lines {
				if line.InventoryItemID == item {
					found = true
					if amount > line.RequestedQuantity {
						return ErrInsufficientStock
					}
					break
				}
			}
			if !found {
				return ErrReservationNotFound
			}
		}
		for _, line := range reservation.Lines {
			desired := fulfilled[line.InventoryItemID]
			remainingCommit := desired
			var commitTotal, releaseTotal int64
			for _, allocation := range line.Allocations {
				var alreadyCommit, alreadyRelease int64
				if err = tx.QueryRow(ctx, `SELECT committed_quantity,released_quantity FROM inventory_reservation_allocations WHERE id=$1 AND business_id=$2 FOR UPDATE`, allocation.ID, actor.BusinessID).Scan(&alreadyCommit, &alreadyRelease); err != nil {
					return err
				}
				outstanding := allocation.Quantity - alreadyCommit - alreadyRelease
				if outstanding < 0 {
					return ErrReservationStateConflict
				}
				commitDelta := remainingCommit
				if commitDelta > outstanding {
					commitDelta = outstanding
				}
				if commitDelta < 0 {
					commitDelta = 0
				}
				releaseDelta := outstanding - commitDelta
				remainingCommit -= commitDelta
				if commitDelta > 0 {
					tag, e := tx.Exec(ctx, `UPDATE inventory_lots SET on_hand_quantity=on_hand_quantity-$3,reserved_quantity=reserved_quantity-$3,version=version+1,updated_at=now() WHERE id=$1 AND business_id=$2 AND on_hand_quantity >= $3 AND reserved_quantity >= $3 AND (expires_at IS NULL OR expires_at > now())`, allocation.LotID, actor.BusinessID, commitDelta)
					if e != nil {
						return e
					}
					if tag.RowsAffected() != 1 {
						return ErrInsufficientStock
					}
				}
				if releaseDelta > 0 {
					tag, e := tx.Exec(ctx, `UPDATE inventory_lots SET reserved_quantity=reserved_quantity-$3,version=version+1,updated_at=now() WHERE id=$1 AND business_id=$2 AND reserved_quantity >= $3`, allocation.LotID, actor.BusinessID, releaseDelta)
					if e != nil {
						return e
					}
					if tag.RowsAffected() != 1 {
						return ErrInsufficientStock
					}
				}
				newCommit, newRelease := alreadyCommit+commitDelta, alreadyRelease+releaseDelta
				status := "HELD"
				if newCommit == allocation.Quantity {
					status = ReservationCommitted
				} else if newRelease == allocation.Quantity {
					status = ReservationReleased
				}
				if _, err = tx.Exec(ctx, `UPDATE inventory_reservation_allocations SET committed_quantity=$1,released_quantity=$2,status=$3,updated_at=now() WHERE id=$4 AND business_id=$5`, newCommit, newRelease, status, allocation.ID, actor.BusinessID); err != nil {
					return err
				}
				commitTotal += commitDelta
				releaseTotal += releaseDelta
			}
			if remainingCommit != 0 {
				return ErrInsufficientStock
			}
			item := states[line.InventoryItemID]
			balanceRevision, e := updateBalanceTx(ctx, tx, item, -commitTotal, -(commitTotal + releaseTotal), 0, 0, 0, 0)
			if e != nil {
				return e
			}
			inventoryRevision, e := bumpInventoryRevisionTx(ctx, tx, actor.BusinessID, branchID)
			if e != nil {
				return e
			}
			for _, allocation := range line.Allocations {
				var c, r int64
				if e = tx.QueryRow(ctx, `SELECT committed_quantity,released_quantity FROM inventory_reservation_allocations WHERE id=$1`, allocation.ID).Scan(&c, &r); e != nil {
					return e
				}
				// Movement rows are append-only evidence. Deltas are derived from
				// the newly persisted split, so retries emit none.
				if c > 0 {
					if _, e = appendMovementTx(ctx, tx, actor, item, allocation.LotID, MovementCommit, c, -c, -c, 0, 0, 0, 0, "RESERVATION", reservation.ID, "PARTIAL_COMMIT", inventoryRevision, balanceRevision); e != nil {
						return e
					}
				}
				if r > 0 {
					if _, e = appendMovementTx(ctx, tx, actor, item, allocation.LotID, MovementRelease, r, 0, -r, 0, 0, 0, 0, "RESERVATION", reservation.ID, "PARTIAL_RELEASE", inventoryRevision, balanceRevision); e != nil {
						return e
					}
				}
			}
			if _, err = tx.Exec(ctx, `UPDATE inventory_reservation_lines SET status='COMMITTED',updated_at=now() WHERE id=$1 AND business_id=$2`, line.ID, actor.BusinessID); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE inventory_reservations SET status='COMMITTED',committed_at=COALESCE(committed_at,now()),updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2`, reservationID, actor.BusinessID); err != nil {
			return err
		}
		out, err = loadReservationQ(ctx, tx, actor.BusinessID, reservationID, false)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrReservationNotFound
	}
	return out, err
}
