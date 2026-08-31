package shop

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ConfirmReservation records that the future caller has accepted a held
// reservation. It does not consume physical stock and is idempotent.
func (s *Service) ConfirmReservation(ctx context.Context, actor Actor, scope Scope, reservationID string) (InventoryReservation, error) {
	return s.changeReservationState(ctx, actor, scope, reservationID, ReservationConfirmed)
}

// ReleaseReservation returns every uncommitted allocation to sellable stock.
func (s *Service) ReleaseReservation(ctx context.Context, actor Actor, scope Scope, reservationID string) (InventoryReservation, error) {
	return s.changeReservationState(ctx, actor, scope, reservationID, ReservationReleased)
}

// CommitReservation consumes physical stock. It creates append-only movement
// evidence but never creates or advances an Order or payment.
func (s *Service) CommitReservation(ctx context.Context, actor Actor, scope Scope, reservationID string) (InventoryReservation, error) {
	return s.changeReservationState(ctx, actor, scope, reservationID, ReservationCommitted)
}

func (s *Service) changeReservationState(ctx context.Context, actor Actor, scope Scope, reservationID, target string) (InventoryReservation, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryReservation{}, err
	}
	var out InventoryReservation
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var current string
		if err := tx.QueryRow(ctx, `SELECT status FROM shop_inventory_reservations
			WHERE id=$1 AND business_id=$2 FOR UPDATE`, reservationID, actor.VendorID).Scan(&current); err != nil {
			return mapDB(err, "Reservation", CodeReservationNotFound)
		}
		if current == target {
			loaded, err := loadReservation(ctx, tx, actor.VendorID, reservationID)
			out = loaded
			return err
		}
		if target == ReservationConfirmed {
			if current != ReservationHeld {
				return coded(apperrors.Conflict("Only a held reservation can be confirmed.", nil), CodeReservationStateConflict)
			}
			branchRows, queryErr := tx.Query(ctx, `SELECT DISTINCT branch_id FROM shop_inventory_reservation_lines
				WHERE reservation_id=$1 AND business_id=$2 ORDER BY branch_id`, reservationID, actor.VendorID)
			if queryErr != nil {
				return queryErr
			}
			var branches []string
			for branchRows.Next() {
				var branchID string
				if err := branchRows.Scan(&branchID); err != nil {
					branchRows.Close()
					return err
				}
				branches = append(branches, branchID)
			}
			branchRows.Close()
			if err := branchRows.Err(); err != nil {
				return err
			}
			for _, branchID := range branches {
				if err := s.requireBranchScope(scope, branchID); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE shop_inventory_reservations SET status='CONFIRMED',updated_at=now(),version=version+1
				WHERE id=$1 AND business_id=$2`, reservationID, actor.VendorID); err != nil {
				return err
			}
			loaded, err := loadReservation(ctx, tx, actor.VendorID, reservationID)
			out = loaded
			if err != nil {
				return err
			}
			branchID := ""
			if len(branches) > 0 {
				branchID = branches[0]
			}
			return s.record(ctx, tx, actor, evidence{action: "shop.inventory.reservation.confirmed", eventType: "shop.inventory.reservation.confirmed", aggregateType: "shop_inventory_reservation", aggregateID: reservationID, branchID: branchID, version: out.Version, after: out, payload: out})
		}
		if current != ReservationHeld && current != ReservationConfirmed {
			return coded(apperrors.Conflict("This reservation has already reached a terminal state.", nil), CodeReservationStateConflict)
		}
		rows, err := tx.Query(ctx, `SELECT l.id,l.branch_id,l.stock_basis_id,l.variant_id,l.physical_quantity,l.backorder_quantity,l.consumption_mode
			FROM shop_inventory_reservation_lines l WHERE l.reservation_id=$1 AND l.business_id=$2
			ORDER BY l.branch_id,l.stock_basis_id,l.id FOR UPDATE`, reservationID, actor.VendorID)
		if err != nil {
			return err
		}
		type line struct {
			id, branch, basis, variant, mode string
			physical, backorder              int64
		}
		var lines []line
		for rows.Next() {
			var line line
			if err = rows.Scan(&line.id, &line.branch, &line.basis, &line.variant, &line.physical, &line.backorder, &line.mode); err != nil {
				rows.Close()
				return err
			}
			if err = s.requireBranchScope(scope, line.branch); err != nil {
				rows.Close()
				return err
			}
			lines = append(lines, line)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		for _, line := range lines {
			if target == ReservationCommitted && line.backorder > 0 {
				return coded(apperrors.Conflict("Outstanding preorder/backorder quantity must be physically allocated before commit.", nil), CodeReservationStateConflict)
			}
			if target == ReservationCommitted && line.mode == ConsumptionVariable {
				var measured bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shop_fresh_fulfilment_measurements
					WHERE business_id=$1 AND reservation_line_id=$2 AND tolerance_result='WITHIN_TOLERANCE')`, actor.VendorID, line.id).Scan(&measured); err != nil {
					return err
				}
				if !measured {
					return coded(apperrors.Conflict("Variable fulfilment requires an actual measurement within configured tolerance before commit.", nil), CodeReservationStateConflict)
				}
			}
			if _, err = tx.Exec(ctx, `SELECT 1 FROM shop_inventory_balances WHERE business_id=$1 AND stock_basis_id=$2 FOR UPDATE`, actor.VendorID, line.basis); err != nil {
				return err
			}
			allocationRows, queryErr := tx.Query(ctx, `SELECT a.id,a.lot_id,a.quantity,l.status,l.expires_at FROM shop_inventory_reservation_allocations a
				JOIN shop_inventory_lots l ON l.id=a.lot_id AND l.business_id=a.business_id
				WHERE a.business_id=$1 AND a.reservation_line_id=$2 AND a.status='HELD' ORDER BY a.lot_id FOR UPDATE OF a,l`, actor.VendorID, line.id)
			if queryErr != nil {
				return queryErr
			}
			type allocation struct {
				id, lot, status string
				quantity        int64
				expires         *time.Time
			}
			var allocations []allocation
			for allocationRows.Next() {
				var a allocation
				if err = allocationRows.Scan(&a.id, &a.lot, &a.quantity, &a.status, &a.expires); err != nil {
					allocationRows.Close()
					return err
				}
				allocations = append(allocations, a)
			}
			allocationRows.Close()
			for _, allocation := range allocations {
				if target == ReservationCommitted && (allocation.status != "ACTIVE" || (allocation.expires != nil && !allocation.expires.After(s.now()))) {
					return coded(apperrors.Conflict("Reservation allocation contains expired or inactive stock and cannot be committed.", nil), CodeInventoryInsufficient)
				}
				if target == ReservationCommitted {
					_, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET on_hand_quantity=on_hand_quantity-$3,reserved_quantity=reserved_quantity-$3,
						status=CASE WHEN on_hand_quantity-$3=0 THEN 'DEPLETED' ELSE status END,updated_at=now(),version=version+1
						WHERE id=$1 AND business_id=$2`, allocation.lot, actor.VendorID, allocation.quantity)
				} else {
					_, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET reserved_quantity=reserved_quantity-$3,updated_at=now(),version=version+1
						WHERE id=$1 AND business_id=$2`, allocation.lot, actor.VendorID, allocation.quantity)
				}
				if err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, `UPDATE shop_inventory_reservation_allocations SET status=$3,updated_at=now()
					WHERE id=$1 AND business_id=$2`, allocation.id, actor.VendorID, target); err != nil {
					return err
				}
			}
			onHandDelta := int64(0)
			movement := "RESERVATION_RELEASE"
			if target == ReservationCommitted {
				onHandDelta = -line.physical
				movement = "SALE_COMMIT"
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_balances SET on_hand_quantity=on_hand_quantity+$3,
				reserved_quantity=reserved_quantity-$4,backorder_quantity=GREATEST(0,backorder_quantity-$5),
				revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2`, actor.VendorID, line.basis, onHandDelta, line.physical, line.backorder); err != nil {
				return err
			}
			revision, revisionErr := bumpInventoryRevision(ctx, tx, actor.VendorID, line.branch)
			if revisionErr != nil {
				return revisionErr
			}
			if line.physical > 0 {
				if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements
					(id,business_id,branch_id,stock_basis_id,variant_id,movement_type,quantity,on_hand_delta,reserved_delta,source_type,source_ref,actor_ref,inventory_revision)
					VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'RESERVATION',$10,NULLIF($11,''),$12)`, newID("sim_"), actor.VendorID, line.branch, line.basis, line.variant, movement, line.physical, onHandDelta, -line.physical, reservationID, actor.StaffID, revision); err != nil {
					return err
				}
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_reservation_lines SET status=$3,updated_at=now()
				WHERE id=$1 AND business_id=$2`, line.id, actor.VendorID, target); err != nil {
				return err
			}
		}
		timestampColumn := "released_at"
		if target == ReservationCommitted {
			timestampColumn = "committed_at"
		}
		if _, err = tx.Exec(ctx, `UPDATE shop_inventory_reservations SET status=$3,`+timestampColumn+`=now(),updated_at=now(),version=version+1
			WHERE id=$1 AND business_id=$2`, reservationID, actor.VendorID, target); err != nil {
			return err
		}
		loaded, err := loadReservation(ctx, tx, actor.VendorID, reservationID)
		out = loaded
		if err != nil {
			return err
		}
		action := "shop.inventory.reservation.released"
		if target == ReservationCommitted {
			action = "shop.inventory.reservation.committed"
		}
		branchID := ""
		if len(lines) > 0 {
			branchID = lines[0].branch
		}
		return s.record(ctx, tx, actor, evidence{action: action, eventType: action, aggregateType: "shop_inventory_reservation", aggregateID: reservationID, branchID: branchID, version: out.Version, after: out, payload: out})
	})
	return out, err
}
