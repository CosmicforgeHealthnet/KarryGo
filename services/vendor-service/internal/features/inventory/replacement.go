package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ReservationReplacementRequest describes one approved substitution at the
// physical-stock boundary. Orders supplies evidence; Inventory owns all stock
// arithmetic and row locking.
type ReservationReplacementRequest struct {
	ReservationID      string
	SourceType         string
	SourceRef          string
	OriginalItemID     string
	OriginalOrderedQty int64
	OriginalFoundQty   int64
	MissingQty         int64
	ReplacementItemID  string
	ReplacementQty     int64
	ExpectedVersion    int64
	IdempotencyKey     string
	CorrelationID      string
}

type ReservationReplacementResult struct {
	ReservationID       string
	OriginalItemID      string
	ReplacementItemID   string
	OriginalFulfilled   int64
	OriginalReleased    int64
	ReplacementReserved int64
	Version             int64
}

var ErrReplacementIdempotencyConflict = errors.New("inventory replacement idempotency conflict")

func replacementFingerprint(r ReservationReplacementRequest) string {
	value := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d|%d|%d|%d", r.ReservationID, r.SourceType, r.SourceRef, r.OriginalItemID, r.ReplacementItemID, r.OriginalOrderedQty, r.OriginalFoundQty, r.MissingQty, r.ReplacementQty, r.ExpectedVersion)
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}

// ReplaceReservationAllocation atomically preserves the found original
// quantity, releases only the superseded hold, and reserves the replacement
// quantity in the same reservation. A retry with the same key returns the
// durable result; reusing it for different material evidence is rejected.
func (s *StockService) ReplaceReservationAllocation(ctx context.Context, actor Actor, scope BranchScope, request ReservationReplacementRequest) (ReservationReplacementResult, error) {
	return s.replaceReservationAllocation(ctx, actor, scope, request, s.withWriteTransaction)
}

// ReplaceReservationAllocationInTx preserves the native Inventory engine's
// guards, locking, arithmetic, audit and outbox in the caller's source-owned
// transaction. It cannot commit independently of the administrative receipt.
func (s *StockService) ReplaceReservationAllocationInTx(ctx context.Context, tx pgx.Tx, actor Actor, scope BranchScope, request ReservationReplacementRequest) (ReservationReplacementResult, error) {
	return s.replaceReservationAllocation(ctx, actor, scope, request, func(ctx context.Context, actor Actor, scope BranchScope, branch string, work func(InventoryTransaction, pgx.Tx) error) error {
		if tx == nil {
			return ErrDependencyMissing
		}
		if err := s.foundation.RequireBranch(ctx, actor, scope, actor.BusinessID, branch, true); err != nil {
			return err
		}
		if err := enableInventoryMutation(ctx, tx); err != nil {
			return err
		}
		return work(postgresInventoryTransaction{ctx: ctx, tx: tx}, tx)
	})
}
func (s *StockService) replaceReservationAllocation(ctx context.Context, actor Actor, scope BranchScope, request ReservationReplacementRequest, within func(context.Context, Actor, BranchScope, string, func(InventoryTransaction, pgx.Tx) error) error) (ReservationReplacementResult, error) {
	if err := s.requireDependencies(); err != nil {
		return ReservationReplacementResult{}, err
	}
	request.ReservationID = strings.TrimSpace(request.ReservationID)
	request.SourceType = strings.TrimSpace(request.SourceType)
	request.SourceRef = strings.TrimSpace(request.SourceRef)
	request.OriginalItemID = strings.TrimSpace(request.OriginalItemID)
	request.ReplacementItemID = strings.TrimSpace(request.ReplacementItemID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.ReservationID == "" || request.OriginalItemID == "" || request.ReplacementItemID == "" || request.OriginalItemID == request.ReplacementItemID || request.IdempotencyKey == "" || request.MissingQty <= 0 || request.ReplacementQty != request.MissingQty || request.OriginalFoundQty < 0 || request.OriginalOrderedQty <= 0 || request.OriginalFoundQty > request.OriginalOrderedQty || request.MissingQty != request.OriginalOrderedQty-request.OriginalFoundQty {
		return ReservationReplacementResult{}, ErrReservationStateConflict
	}
	fingerprint := replacementFingerprint(request)
	var out ReservationReplacementResult
	err := within(ctx, actor, scope, scopeBranchID(scope), func(inventoryTx InventoryTransaction, tx pgx.Tx) error {
		var branchID, businessID, sourceType, sourceRef, status string
		var version int64
		if err := tx.QueryRow(ctx, `SELECT branch_id,business_id,source_type,source_ref,status,version FROM inventory_reservations WHERE id=$1 AND business_id=$2 FOR UPDATE`, request.ReservationID, actor.BusinessID).Scan(&branchID, &businessID, &sourceType, &sourceRef, &status, &version); errors.Is(err, pgx.ErrNoRows) {
			return ErrReservationNotFound
		} else if err != nil {
			return err
		}
		var existingFingerprint string
		if err := tx.QueryRow(ctx, `SELECT request_fingerprint FROM inventory_reservation_replacements WHERE reservation_id=$1 AND idempotency_key=$2 FOR UPDATE`, request.ReservationID, request.IdempotencyKey).Scan(&existingFingerprint); err == nil {
			if existingFingerprint != fingerprint {
				return ErrReplacementIdempotencyConflict
			}
			out = ReservationReplacementResult{ReservationID: request.ReservationID, OriginalItemID: request.OriginalItemID, ReplacementItemID: request.ReplacementItemID, OriginalFulfilled: request.OriginalFoundQty, OriginalReleased: request.MissingQty, ReplacementReserved: request.ReplacementQty, Version: version}
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if request.ExpectedVersion > 0 && request.ExpectedVersion != version {
			return ErrReservationStateConflict
		}
		if request.SourceType != "" && request.SourceType != sourceType || request.SourceRef != "" && request.SourceRef != sourceRef {
			return ErrReservationStateConflict
		}
		if status != ReservationHeld && status != ReservationConfirmed {
			return ErrReservationStateConflict
		}
		states, err := lockItemsDeterministicallyTx(ctx, tx, actor.BusinessID, branchID, []string{request.OriginalItemID, request.ReplacementItemID})
		if err != nil {
			return err
		}
		original := states[request.OriginalItemID]
		replacement := states[request.ReplacementItemID]
		if original.ID == "" || replacement.ID == "" {
			return ErrStockNotFound
		}
		reservation, err := loadReservationQ(ctx, tx, actor.BusinessID, request.ReservationID, true)
		if err != nil {
			return err
		}
		var originalLine ReservationLine
		for _, line := range reservation.Lines {
			if line.InventoryItemID == request.OriginalItemID {
				originalLine = line
				break
			}
		}
		if originalLine.ID == "" || originalLine.RequestedQuantity != request.OriginalOrderedQty {
			return ErrReservationStateConflict
		}
		remaining := request.MissingQty
		var released int64
		var releaseRevision, releaseBalanceRevision int64
		for _, allocation := range originalLine.Allocations {
			if remaining == 0 {
				break
			}
			var committed, alreadyReleased int64
			if err = tx.QueryRow(ctx, `SELECT committed_quantity,released_quantity FROM inventory_reservation_allocations WHERE id=$1 AND reservation_id=$2 FOR UPDATE`, allocation.ID, request.ReservationID).Scan(&committed, &alreadyReleased); err != nil {
				return err
			}
			outstanding := allocation.Quantity - committed - alreadyReleased
			if outstanding <= 0 {
				continue
			}
			delta := outstanding
			if delta > remaining {
				delta = remaining
			}
			tag, e := tx.Exec(ctx, `UPDATE inventory_lots SET reserved_quantity=reserved_quantity-$3,version=version+1,updated_at=now() WHERE id=$1 AND business_id=$2 AND reserved_quantity >= $3`, allocation.LotID, actor.BusinessID, delta)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return ErrInsufficientStock
			}
			newReleased := alreadyReleased + delta
			allocationStatus := ReservationHeld
			if committed+newReleased == allocation.Quantity {
				allocationStatus = ReservationReleased
			}
			if _, e = tx.Exec(ctx, `UPDATE inventory_reservation_allocations SET released_quantity=$1,status=$2,updated_at=now() WHERE id=$3`, newReleased, allocationStatus, allocation.ID); e != nil {
				return e
			}
			released += delta
			remaining -= delta
			releaseRevision, e = bumpInventoryRevisionTx(ctx, tx, actor.BusinessID, branchID)
			if e != nil {
				return e
			}
			releaseBalanceRevision, e = updateBalanceTx(ctx, tx, original, 0, -delta, 0, 0, 0, 0)
			if e != nil {
				return e
			}
			if _, e = appendMovementTx(ctx, tx, actor, original, allocation.LotID, MovementRelease, delta, 0, -delta, 0, 0, 0, 0, "RESERVATION_REPLACEMENT", request.ReservationID, request.CorrelationID, releaseRevision, releaseBalanceRevision); e != nil {
				return e
			}
		}
		if remaining != 0 {
			return ErrInsufficientStock
		}
		if released == 0 {
			return ErrInsufficientStock
		}
		if err = ensureBalanceTx(ctx, tx, replacement); err != nil {
			return err
		}
		lots, err := allocateAvailableLotsTx(ctx, tx, replacement, request.ReplacementQty)
		if err != nil {
			return err
		}
		replacementBalanceRevision, err := updateBalanceTx(ctx, tx, replacement, 0, request.ReplacementQty, 0, 0, 0, 0)
		if err != nil {
			return err
		}
		replacementInventoryRevision, err := bumpInventoryRevisionTx(ctx, tx, actor.BusinessID, branchID)
		if err != nil {
			return err
		}
		lineID := stockID("iresl_")
		if _, err = tx.Exec(ctx, `INSERT INTO inventory_reservation_lines(id,business_id,branch_id,reservation_id,inventory_item_id,requested_quantity,physical_quantity,backorder_quantity,status) VALUES($1,$2,$3,$4,$5,$6,$6,0,'HELD')`, lineID, actor.BusinessID, branchID, request.ReservationID, replacement.ID, request.ReplacementQty); err != nil {
			return err
		}
		for _, lot := range lots {
			if _, err = tx.Exec(ctx, `UPDATE inventory_lots SET reserved_quantity=reserved_quantity+$3,version=version+1,updated_at=now() WHERE id=$1 AND business_id=$2 AND status='ACTIVE' AND (expires_at IS NULL OR expires_at > now()) AND on_hand_quantity-reserved_quantity-quarantined_quantity >= $3`, lot.ID, actor.BusinessID, lot.Quantity); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO inventory_reservation_allocations(id,business_id,branch_id,reservation_id,reservation_line_id,inventory_item_id,lot_id,quantity,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'HELD')`, stockID("iresa_"), actor.BusinessID, branchID, request.ReservationID, lineID, replacement.ID, lot.ID, lot.Quantity); err != nil {
				return err
			}
			if _, err = appendMovementTx(ctx, tx, actor, replacement, lot.ID, MovementReserve, lot.Quantity, 0, lot.Quantity, 0, 0, 0, 0, "RESERVATION_REPLACEMENT", request.ReservationID, request.CorrelationID, replacementInventoryRevision, replacementBalanceRevision); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE inventory_reservations SET version=version+1,updated_at=now() WHERE id=$1 AND version=$2`, request.ReservationID, version); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO inventory_reservation_replacements(id,reservation_id,business_id,branch_id,idempotency_key,request_fingerprint,original_item_id,replacement_item_id,original_found_quantity,superseded_quantity,replacement_quantity) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, stockID("irpl_"), request.ReservationID, actor.BusinessID, branchID, request.IdempotencyKey, fingerprint, request.OriginalItemID, request.ReplacementItemID, request.OriginalFoundQty, request.MissingQty, request.ReplacementQty); err != nil {
			return err
		}
		if err = s.recordMutation(ctx, inventoryTx, actor, scope, "inventory.reservation.replaced", "inventory.reservation.replaced", "inventory_reservation", request.ReservationID, "SHOP_SUBSTITUTION", version+1, map[string]any{"original_item_id": request.OriginalItemID, "replacement_item_id": request.ReplacementItemID, "released_quantity": released, "reserved_quantity": request.ReplacementQty, "idempotency_key": request.IdempotencyKey}); err != nil {
			return err
		}
		out = ReservationReplacementResult{ReservationID: request.ReservationID, OriginalItemID: request.OriginalItemID, ReplacementItemID: request.ReplacementItemID, OriginalFulfilled: request.OriginalFoundQty, OriginalReleased: released, ReplacementReserved: request.ReplacementQty, Version: version + 1}
		return nil
	})
	return out, err
}

func scopeBranchID(scope BranchScope) string {
	if scope.BranchID == nil {
		return ""
	}
	return strings.TrimSpace(*scope.BranchID)
}
