package shop

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

type RecordFulfilmentInput struct {
	ReservationLineID string    `json:"reservation_line_id"`
	ActualQuantity    int64     `json:"actual_quantity"`
	ActualMeasureID   string    `json:"actual_measure_id"`
	MeasuredAt        time.Time `json:"measured_at"`
	MeasuredBy        string    `json:"measured_by"`
	SourceRef         string    `json:"source_ref"`
}

type FulfilmentMeasurement struct {
	ID                  string    `json:"id"`
	ReservationLineRef  string    `json:"reservation_line_ref"`
	BranchRef           string    `json:"branch_ref"`
	RequestedQuantity   int64     `json:"requested_quantity"`
	RequestedMeasureRef string    `json:"requested_measure_ref"`
	ActualQuantity      int64     `json:"actual_quantity"`
	ActualMeasureRef    string    `json:"actual_measure_ref"`
	ToleranceResult     string    `json:"tolerance_result"`
	MeasuredAt          time.Time `json:"measured_at"`
	MeasuredBy          string    `json:"measured_by,omitempty"`
	SourceRef           string    `json:"source_ref"`
	Version             int64     `json:"version"`
}

func (s *Service) RecordFulfilmentMeasurement(ctx context.Context, actor Actor, scope Scope, input RecordFulfilmentInput) (FulfilmentMeasurement, error) {
	if err := s.requireWrites(); err != nil {
		return FulfilmentMeasurement{}, err
	}
	if input.ActualQuantity <= 0 || input.ActualQuantity > MaxInventoryQuantityPerLine {
		return FulfilmentMeasurement{}, fieldError("actual_quantity", "Actual quantity must be positive and within the controlled per-line limit.")
	}
	if input.SourceRef == "" {
		return FulfilmentMeasurement{}, fieldError("source_ref", "Source reference is required.")
	}
	if input.MeasuredAt.IsZero() {
		input.MeasuredAt = s.now()
	}
	var out FulfilmentMeasurement
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var reservationID, branchID, basisID, variantID, listingID, mode, status, measureID string
		var requested, physical int64
		err := tx.QueryRow(ctx, `SELECT l.reservation_id,l.branch_id,l.stock_basis_id,l.variant_id,l.listing_id,l.consumption_mode,l.status,l.requested_quantity,l.physical_quantity,b.measure_definition_id
			FROM shop_inventory_reservation_lines l JOIN shop_inventory_stock_bases b ON b.id=l.stock_basis_id AND b.business_id=l.business_id
			WHERE l.id=$1 AND l.business_id=$2 FOR UPDATE OF l`, input.ReservationLineID, actor.VendorID).Scan(&reservationID, &branchID, &basisID, &variantID, &listingID, &mode, &status, &requested, &physical, &measureID)
		if err != nil {
			return mapDB(err, "Reservation line", CodeReservationNotFound)
		}
		if err = s.requireBranchScope(scope, branchID); err != nil {
			return err
		}
		if mode != ConsumptionVariable {
			return coded(apperrors.Conflict("Actual fulfilment measurement is only valid for VARIABLE_FULFILMENT stock basis.", nil), CodeInventoryConversionMissing)
		}
		if status != "HELD" {
			return coded(apperrors.Conflict("Reservation line is not held.", nil), CodeReservationStateConflict)
		}
		if input.ActualMeasureID != measureID {
			return coded(apperrors.Conflict("Actual fulfilment must be recorded in the authoritative stock-basis measure.", nil), CodeInventoryConversionMissing)
		}
		var underfill, overfill int64
		if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(su.underfill_tolerance_bps),0),COALESCE(MAX(su.overfill_tolerance_bps),0)
			FROM shop_listing_sell_units lu JOIN shop_sell_units su ON su.id=lu.sell_unit_id AND su.business_id=lu.business_id
			WHERE lu.business_id=$1 AND lu.listing_id=$2 AND lu.enabled AND su.variant_id=$3`, actor.VendorID, listingID, variantID).Scan(&underfill, &overfill); err != nil {
			return err
		}
		deviation := (input.ActualQuantity - requested) * 10000 / requested
		result := "WITHIN_TOLERANCE"
		if deviation < -underfill {
			result = "UNDER_TOLERANCE"
		}
		if deviation > overfill {
			result = "OVER_TOLERANCE"
		}
		out = FulfilmentMeasurement{ID: newID("sifm_"), ReservationLineRef: input.ReservationLineID, BranchRef: branchID, RequestedQuantity: requested, RequestedMeasureRef: measureID, ActualQuantity: input.ActualQuantity, ActualMeasureRef: input.ActualMeasureID, ToleranceResult: result, MeasuredAt: input.MeasuredAt, MeasuredBy: input.MeasuredBy, SourceRef: input.SourceRef, Version: 1}
		if _, err = tx.Exec(ctx, `INSERT INTO shop_fresh_fulfilment_measurements(id,business_id,branch_id,reservation_line_id,requested_quantity,requested_measure_id,actual_quantity,actual_measure_id,tolerance_result,measured_by,measured_at,source_ref)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11,$12)`, out.ID, actor.VendorID, branchID, input.ReservationLineID, requested, measureID, input.ActualQuantity, input.ActualMeasureID, result, input.MeasuredBy, input.MeasuredAt, input.SourceRef); err != nil {
			return mapDB(err, "Fulfilment measurement", CodeDuplicate)
		}
		if result == "WITHIN_TOLERANCE" && input.ActualQuantity != physical {
			if err = resizeReservationAllocation(ctx, tx, actor.VendorID, branchID, basisID, variantID, input.ReservationLineID, input.ActualQuantity-physical, reservationID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_reservation_lines SET physical_quantity=$3,requested_quantity=backorder_quantity+$3,updated_at=now() WHERE id=$1 AND business_id=$2`, input.ReservationLineID, actor.VendorID, input.ActualQuantity); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.fulfilment.measured", eventType: "shop.inventory.fulfilment.measured", aggregateType: "shop_fulfilment_measurement", aggregateID: out.ID, branchID: branchID, version: 1, after: out, payload: out, reason: result})
	})
	return out, err
}

func resizeReservationAllocation(ctx context.Context, tx pgx.Tx, businessID, branchID, basisID, variantID, lineID string, delta int64, reservationID string) error {
	if _, _, _, err := lockInventoryBalance(ctx, tx, businessID, basisID); err != nil {
		return err
	}
	if delta > 0 {
		var onHand, reserved, quarantined int64
		if err := tx.QueryRow(ctx, `SELECT on_hand_quantity,reserved_quantity,quarantined_quantity FROM shop_inventory_balances WHERE business_id=$1 AND stock_basis_id=$2`, businessID, basisID).Scan(&onHand, &reserved, &quarantined); err != nil {
			return err
		}
		if delta > onHand-reserved-quarantined {
			return coded(apperrors.Conflict("Measured quantity exceeds remaining sellable stock.", nil), CodeInventoryInsufficient)
		}
		rows, err := tx.Query(ctx, `SELECT id,on_hand_quantity-reserved_quantity-quarantined_quantity FROM shop_inventory_lots WHERE business_id=$1 AND stock_basis_id=$2 AND status='ACTIVE' AND (expires_at IS NULL OR expires_at>now()) AND on_hand_quantity>reserved_quantity+quarantined_quantity ORDER BY expires_at ASC NULLS LAST,received_at,id FOR UPDATE`, businessID, basisID)
		if err != nil {
			return err
		}
		type lot struct {
			id        string
			available int64
		}
		var lots []lot
		for rows.Next() {
			var item lot
			if err = rows.Scan(&item.id, &item.available); err != nil {
				rows.Close()
				return err
			}
			lots = append(lots, item)
		}
		rows.Close()
		remaining := delta
		for _, item := range lots {
			if remaining == 0 {
				break
			}
			take := item.available
			if take > remaining {
				take = remaining
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET reserved_quantity=reserved_quantity+$3,updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2`, item.id, businessID, take); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_reservation_allocations(id,business_id,reservation_line_id,lot_id,quantity) VALUES($1,$2,$3,$4,$5) ON CONFLICT(reservation_line_id,lot_id) DO UPDATE SET quantity=shop_inventory_reservation_allocations.quantity+EXCLUDED.quantity,updated_at=now()`, newID("sirva_"), businessID, lineID, item.id, take); err != nil {
				return err
			}
			remaining -= take
		}
		if remaining != 0 {
			return coded(apperrors.Conflict("Measured quantity cannot be allocated from valid lots.", nil), CodeInventoryInsufficient)
		}
	} else {
		remaining := -delta
		rows, err := tx.Query(ctx, `SELECT a.id,a.lot_id,a.quantity FROM shop_inventory_reservation_allocations a JOIN shop_inventory_lots l ON l.id=a.lot_id AND l.business_id=a.business_id WHERE a.business_id=$1 AND a.reservation_line_id=$2 AND a.status='HELD' ORDER BY l.expires_at DESC NULLS FIRST,l.received_at DESC,l.id DESC FOR UPDATE OF a,l`, businessID, lineID)
		if err != nil {
			return err
		}
		type allocation struct {
			id, lot  string
			quantity int64
		}
		var allocations []allocation
		for rows.Next() {
			var a allocation
			if err = rows.Scan(&a.id, &a.lot, &a.quantity); err != nil {
				rows.Close()
				return err
			}
			allocations = append(allocations, a)
		}
		rows.Close()
		for _, a := range allocations {
			if remaining == 0 {
				break
			}
			give := a.quantity
			if give > remaining {
				give = remaining
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET reserved_quantity=reserved_quantity-$3,updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2`, a.lot, businessID, give); err != nil {
				return err
			}
			if give == a.quantity {
				_, err = tx.Exec(ctx, `UPDATE shop_inventory_reservation_allocations SET status='RELEASED',updated_at=now() WHERE id=$1 AND business_id=$2`, a.id, businessID)
			} else {
				_, err = tx.Exec(ctx, `UPDATE shop_inventory_reservation_allocations SET quantity=quantity-$3,updated_at=now() WHERE id=$1 AND business_id=$2`, a.id, businessID, give)
			}
			if err != nil {
				return err
			}
			remaining -= give
		}
		if remaining != 0 {
			return coded(apperrors.Conflict("Measured quantity release exceeds held allocation.", nil), CodeReservationStateConflict)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE shop_inventory_balances SET reserved_quantity=reserved_quantity+$3,revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2`, businessID, basisID, delta); err != nil {
		return err
	}
	revision, err := bumpInventoryRevision(ctx, tx, businessID, branchID)
	if err != nil {
		return err
	}
	movement := "RESERVATION"
	if delta < 0 {
		movement = "RESERVATION_RELEASE"
	}
	quantity := delta
	if quantity < 0 {
		quantity = -quantity
	}
	_, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,movement_type,quantity,reserved_delta,source_type,source_ref,inventory_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'FULFILMENT',$9,$10)`, newID("sim_"), businessID, branchID, basisID, variantID, movement, quantity, delta, reservationID, revision)
	return err
}
