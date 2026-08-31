package inventory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PurchasingReceiptLineInput is already converted into authoritative Inventory
// units by Purchasing from the frozen exact PurchaseOrderLine conversion.
//
// Purchasing cannot pass damaged/rejected/short quantities here. Only accepted
// physical stock is represented by this contract.
type PurchasingReceiptLineInput struct {
	ExternalLineRef string

	InventoryItemID string
	Quantity        int64

	LotNumber   string
	BatchNumber string

	ManufacturedAt *time.Time
	BestBeforeAt   *time.Time
	ExpiresAt      *time.Time

	// InventoryUnitCostMinor is present only when Purchasing can divide the
	// purchase-unit cost into Inventory units exactly. No rounding is allowed.
	InventoryUnitCostMinor *int64
}

type PurchasingReceiptPostingInput struct {
	BusinessID string
	BranchID   string

	GoodsReceiptID string
	SupplierRef    string

	ReceivedAt time.Time

	Lines []PurchasingReceiptLineInput
}

type PurchasingReceiptPostingLineResult struct {
	ExternalLineRef string

	InventoryReceiptLineID string
	MovementID             string
	LotID                  string
}

type PurchasingReceiptPostingResult struct {
	InventoryReceiptID string

	Lines []PurchasingReceiptPostingLineResult
}

func purchasingReceiptSource(goodsReceiptID string) string {
	return "PURCHASING:" + strings.TrimSpace(goodsReceiptID)
}

// purchasingReceiptRequiresLotMetadata reports whether the operator must supply
// a lot or batch number for this item. Only genuinely tracked items force it.
//
// This is deliberately separate from whether a lot row is created. Every posted
// line creates a lot (see below); this only decides whether the receiver has to
// name one.
func purchasingReceiptRequiresLotMetadata(
	trackingMode string,
	lotTracking bool,
) bool {
	return lotTracking ||
		trackingMode == "LOT" ||
		trackingMode == "EXPIRY"
}

func nextPurchasingInventoryRevisionTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
) (int64, error) {
	var revision int64

	err := tx.QueryRow(
		ctx,
		`
        INSERT INTO inventory_revisions(
            business_id,
            branch_id,
            revision,
            updated_at
        )
        VALUES($1,$2,1,now())
        ON CONFLICT(business_id,branch_id)
        DO UPDATE
           SET revision=inventory_revisions.revision+1,
               updated_at=now()
        RETURNING revision
        `,
		businessID,
		branchID,
	).Scan(&revision)

	return revision, err
}

func (s *StockService) replayPurchasingReceiptTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	sourceRef string,
) (PurchasingReceiptPostingResult, bool, error) {
	var receiptID string
	var status string

	err := tx.QueryRow(
		ctx,
		`
        SELECT id,status
          FROM inventory_receipts
         WHERE business_id=$1
           AND source_ref=$2
         FOR UPDATE
        `,
		businessID,
		sourceRef,
	).Scan(
		&receiptID,
		&status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return PurchasingReceiptPostingResult{}, false, nil
	}

	if err != nil {
		return PurchasingReceiptPostingResult{}, false, err
	}

	if status != "POSTED" {
		return PurchasingReceiptPostingResult{},
			false,
			fmt.Errorf(
				"%w: purchasing Inventory receipt exists in non-posted state",
				ErrPurchasingReceiptStateConflict,
			)
	}

	result := PurchasingReceiptPostingResult{
		InventoryReceiptID: receiptID,
	}

	// Resolve the immutable movement source_ref back to the external
	// GoodsReceiptLine. inventory_receipt_lines itself intentionally remains
	// the certified Section 14 schema and is not expanded for Purchasing.
	rows2, queryErr := tx.Query(
		ctx,
		`
        SELECT
            m.source_ref,
            rl.id,
            COALESCE(rl.posted_movement_id,''),
            COALESCE(rl.lot_id,'')
          FROM inventory_receipt_lines rl
          JOIN inventory_movements m
            ON m.id=rl.posted_movement_id
           AND m.business_id=rl.business_id
           AND m.inventory_item_id=rl.inventory_item_id
         WHERE rl.business_id=$1
           AND rl.receipt_id=$2
         ORDER BY rl.id
        `,
		businessID,
		receiptID,
	)
	if queryErr != nil {
		return PurchasingReceiptPostingResult{}, false, queryErr
	}
	defer rows2.Close()

	for rows2.Next() {
		var line PurchasingReceiptPostingLineResult

		if queryErr = rows2.Scan(
			&line.ExternalLineRef,
			&line.InventoryReceiptLineID,
			&line.MovementID,
			&line.LotID,
		); queryErr != nil {
			return PurchasingReceiptPostingResult{}, false, queryErr
		}

		result.Lines = append(result.Lines, line)
	}

	if queryErr = rows2.Err(); queryErr != nil {
		return PurchasingReceiptPostingResult{}, false, queryErr
	}

	return result, true, nil
}

// PostPurchasingReceiptTx is the Section 15C transaction-scoped Inventory
// authority seam.
//
// Caller transaction:
//
//	GoodsReceipt inspection/commercial evidence
//	    â†“
//	PostPurchasingReceiptTx
//	    â†“
//	Inventory balance + lot + immutable movement
//	    â†“
//	PurchaseOrder receiving progress
//	    â†“
//	one PostgreSQL COMMIT
//
// Only accepted quantities are present here.
func (s *StockService) PostPurchasingReceiptTx(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	scope BranchScope,
	input PurchasingReceiptPostingInput,
) (PurchasingReceiptPostingResult, error) {
	if s == nil ||
		s.db == nil ||
		s.foundation == nil ||
		tx == nil {
		return PurchasingReceiptPostingResult{}, ErrDependencyMissing
	}

	input.BusinessID = strings.TrimSpace(input.BusinessID)
	input.BranchID = strings.TrimSpace(input.BranchID)
	input.GoodsReceiptID = strings.TrimSpace(input.GoodsReceiptID)
	input.SupplierRef = strings.TrimSpace(input.SupplierRef)

	if input.BusinessID == "" ||
		input.BranchID == "" ||
		input.GoodsReceiptID == "" ||
		input.BusinessID != strings.TrimSpace(actor.BusinessID) {
		return PurchasingReceiptPostingResult{}, ErrPurchasingReceiptInvalid
	}

	if err := s.foundation.RequireBranch(
		ctx,
		actor,
		scope,
		input.BusinessID,
		input.BranchID,
		true,
	); err != nil {
		return PurchasingReceiptPostingResult{}, err
	}

	if !scope.Covers(input.BusinessID, input.BranchID) {
		return PurchasingReceiptPostingResult{}, ErrInvalidScope
	}

	sourceRef := purchasingReceiptSource(input.GoodsReceiptID)

	if replay, ok, err := s.replayPurchasingReceiptTx(
		ctx,
		tx,
		input.BusinessID,
		sourceRef,
	); err != nil {
		return PurchasingReceiptPostingResult{}, err
	} else if ok {
		return replay, nil
	}

	if len(input.Lines) == 0 {
		return PurchasingReceiptPostingResult{}, nil
	}

	if input.ReceivedAt.IsZero() {
		input.ReceivedAt = time.Now().UTC()
	}

	if _, err := tx.Exec(
		ctx,
		`SELECT set_config('karrygo.inventory_mutation','1',true)`,
	); err != nil {
		return PurchasingReceiptPostingResult{}, err
	}

	receiptID := "ir_" + uuid.NewString()

	if _, err := tx.Exec(
		ctx,
		`
        INSERT INTO inventory_receipts(
            id,
            business_id,
            branch_id,
            source_ref,
            supplier_ref,
            notes,
            received_at,
            status,
            posted_at,
            version
        )
        VALUES(
            $1,$2,$3,$4,NULLIF($5,''),
            'Section 15C Purchasing GoodsReceipt',
            $6,'POSTED',now(),2
        )
        `,
		receiptID,
		input.BusinessID,
		input.BranchID,
		sourceRef,
		input.SupplierRef,
		input.ReceivedAt,
	); err != nil {
		return PurchasingReceiptPostingResult{}, err
	}

	result := PurchasingReceiptPostingResult{
		InventoryReceiptID: receiptID,
		Lines: make(
			[]PurchasingReceiptPostingLineResult,
			0,
			len(input.Lines),
		),
	}

	for _, line := range input.Lines {
		line.ExternalLineRef = strings.TrimSpace(line.ExternalLineRef)
		line.InventoryItemID = strings.TrimSpace(line.InventoryItemID)
		line.LotNumber = strings.TrimSpace(line.LotNumber)
		line.BatchNumber = strings.TrimSpace(line.BatchNumber)

		if line.ExternalLineRef == "" ||
			line.InventoryItemID == "" ||
			line.Quantity <= 0 {
			return PurchasingReceiptPostingResult{}, ErrInvalidQuantity
		}

		var trackingMode string
		var lotTracking bool
		var itemStatus string

		err := tx.QueryRow(
			ctx,
			`
            SELECT
                tracking_mode,
                lot_tracking_enabled,
                status
              FROM inventory_items
             WHERE id=$1
               AND business_id=$2
               AND branch_id=$3
             FOR UPDATE
            `,
			line.InventoryItemID,
			input.BusinessID,
			input.BranchID,
		).Scan(
			&trackingMode,
			&lotTracking,
			&itemStatus,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return PurchasingReceiptPostingResult{},
				ErrItemNotFound
		}
		if err != nil {
			return PurchasingReceiptPostingResult{}, err
		}
		if itemStatus != "ACTIVE" {
			return PurchasingReceiptPostingResult{}, ErrPurchasingReceiptStateConflict
		}

		requiresLotMetadata := purchasingReceiptRequiresLotMetadata(
			trackingMode,
			lotTracking,
		)

		// Every posted line creates an inventory lot, whatever the tracking
		// mode. Generic Inventory allocates reservations and adjustments out of
		// lots, so a purchasing receipt that wrote only a balance row left stock
		// that read as available but could not be reserved, adjusted or sold.
		// Section 14's own Receive always creates a lot; purchasing has to post
		// stock the same way or it is not the same stock.
		needsLot := true

		if requiresLotMetadata &&
			line.LotNumber == "" &&
			line.BatchNumber == "" {
			return PurchasingReceiptPostingResult{},
				fmt.Errorf(
					"%w: tracked purchasing receipt requires lot or batch",
					ErrPurchasingReceiptInvalid,
				)
		}

		if trackingMode == "EXPIRY" &&
			line.ExpiresAt == nil {
			return PurchasingReceiptPostingResult{},
				fmt.Errorf(
					"%w: expiry-tracked purchasing receipt requires expiry",
					ErrPurchasingReceiptInvalid,
				)
		}

		if line.ExpiresAt != nil &&
			!line.ExpiresAt.After(input.ReceivedAt) {
			return PurchasingReceiptPostingResult{},
				fmt.Errorf(
					"%w: received stock is already expired",
					ErrPurchasingReceiptInvalid,
				)
		}

		var lotID string

		if needsLot {
			lotID = "il_" + uuid.NewString()

			if _, err = tx.Exec(
				ctx,
				`
                INSERT INTO inventory_lots(
                    id,
                    business_id,
                    branch_id,
                    inventory_item_id,
                    lot_number,
                    batch_number,
                    source_type,
                    source_ref,
                    received_at,
                    manufactured_at,
                    best_before_at,
                    expires_at,
                    on_hand_quantity,
                    reserved_quantity,
                    quarantined_quantity,
                    status,
                    version
                )
                VALUES(
                    $1,$2,$3,$4,
                    NULLIF($5,''),
                    NULLIF($6,''),
                    'PURCHASE_RECEIPT_LINE',
                    $7,
                    $8,$9,$10,$11,
                    $12,0,0,'ACTIVE',1
                )
                `,
				lotID,
				input.BusinessID,
				input.BranchID,
				line.InventoryItemID,
				line.LotNumber,
				line.BatchNumber,
				line.ExternalLineRef,
				input.ReceivedAt,
				line.ManufacturedAt,
				line.BestBeforeAt,
				line.ExpiresAt,
				line.Quantity,
			); err != nil {
				return PurchasingReceiptPostingResult{}, err
			}
		}

		var balanceRevision int64

		err = tx.QueryRow(
			ctx,
			`
            INSERT INTO inventory_balances(
                id,
                business_id,
                branch_id,
                inventory_item_id,
                on_hand_quantity,
                revision,
                updated_at
            )
            VALUES($1,$2,$3,$4,$5,1,now())
            ON CONFLICT(business_id,branch_id,inventory_item_id)
            DO UPDATE
               SET on_hand_quantity=
                   inventory_balances.on_hand_quantity+
                   EXCLUDED.on_hand_quantity,
                   revision=inventory_balances.revision+1,
                   updated_at=now()
            RETURNING revision
            `,
			"ib_"+uuid.NewString(),
			input.BusinessID,
			input.BranchID,
			line.InventoryItemID,
			line.Quantity,
		).Scan(&balanceRevision)
		if err != nil {
			return PurchasingReceiptPostingResult{}, err
		}

		inventoryRevision, err :=
			nextPurchasingInventoryRevisionTx(
				ctx,
				tx,
				input.BusinessID,
				input.BranchID,
			)
		if err != nil {
			return PurchasingReceiptPostingResult{}, err
		}

		movementID := "im_" + uuid.NewString()

		if _, err = tx.Exec(
			ctx,
			`
            INSERT INTO inventory_movements(
                id,
                business_id,
                branch_id,
                inventory_item_id,
                lot_id,
                movement_type,
                quantity,
                on_hand_delta,
                source_type,
                source_ref,
                reason_code,
                actor_ref,
                occurred_at,
                inventory_revision,
                balance_revision
            )
            VALUES(
                $1,$2,$3,$4,NULLIF($5,''),
                'RECEIVE',$6,$6,
                'PURCHASE_RECEIPT',$7,
                'SUPPLIER_ACCEPTED',
                NULLIF($8,''),
                now(),$9,$10
            )
            `,
			movementID,
			input.BusinessID,
			input.BranchID,
			line.InventoryItemID,
			lotID,
			line.Quantity,
			line.ExternalLineRef,
			actor.ActorRef,
			inventoryRevision,
			balanceRevision,
		); err != nil {
			return PurchasingReceiptPostingResult{}, err
		}

		receiptLineID := "irl_" + uuid.NewString()

		if _, err = tx.Exec(
			ctx,
			`
            INSERT INTO inventory_receipt_lines(
                id,
                business_id,
                branch_id,
                receipt_id,
                inventory_item_id,
                quantity,
                lot_number,
                batch_number,
                manufactured_at,
                best_before_at,
                expires_at,
                unit_cost_minor,
                lot_id,
                posted_movement_id
            )
            VALUES(
                $1,$2,$3,$4,$5,$6,
                NULLIF($7,''),
                NULLIF($8,''),
                $9,$10,$11,$12,
                NULLIF($13,''),
                $14
            )
            `,
			receiptLineID,
			input.BusinessID,
			input.BranchID,
			receiptID,
			line.InventoryItemID,
			line.Quantity,
			line.LotNumber,
			line.BatchNumber,
			line.ManufacturedAt,
			line.BestBeforeAt,
			line.ExpiresAt,
			line.InventoryUnitCostMinor,
			lotID,
			movementID,
		); err != nil {
			return PurchasingReceiptPostingResult{}, err
		}

		wrapped := postgresInventoryTransaction{
			ctx: ctx,
			tx:  tx,
		}

		if err = s.recordMutation(
			ctx,
			wrapped,
			actor,
			scope,
			"inventory.purchase_receipt.accepted",
			"vendor.inventory.purchase_receipt.accepted",
			"inventory_item",
			line.InventoryItemID,
			"SUPPLIER_ACCEPTED",
			inventoryRevision,
			map[string]any{
				"goods_receipt_id":  input.GoodsReceiptID,
				"receipt_line_ref":  line.ExternalLineRef,
				"inventory_item_id": line.InventoryItemID,
				"accepted_quantity": line.Quantity,
				"movement_id":       movementID,
				"lot_id":            lotID,
				"balance_revision":  balanceRevision,
			},
		); err != nil {
			return PurchasingReceiptPostingResult{}, err
		}

		result.Lines = append(
			result.Lines,
			PurchasingReceiptPostingLineResult{
				ExternalLineRef:        line.ExternalLineRef,
				InventoryReceiptLineID: receiptLineID,
				MovementID:             movementID,
				LotID:                  lotID,
			},
		)
	}

	return result, nil
}

// ReversePurchasingReceiptTx performs a deliberately strict reversal.
//
// Lot-tracked stock can only reverse while the exact receipt lot remains
// completely untouched. Untracked stock can only reverse if no later physical
// movement exists for that item. This prevents a receipt reversal from
// silently subtracting somebody else's stock.
func (s *StockService) ReversePurchasingReceiptTx(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	scope BranchScope,
	businessID string,
	branchID string,
	goodsReceiptID string,
) error {
	if s == nil ||
		s.db == nil ||
		s.foundation == nil ||
		tx == nil {
		return ErrDependencyMissing
	}

	businessID = strings.TrimSpace(businessID)
	branchID = strings.TrimSpace(branchID)
	goodsReceiptID = strings.TrimSpace(goodsReceiptID)

	if businessID == "" ||
		branchID == "" ||
		goodsReceiptID == "" ||
		businessID != strings.TrimSpace(actor.BusinessID) {
		return ErrPurchasingReceiptInvalid
	}

	if err := s.foundation.RequireBranch(
		ctx,
		actor,
		scope,
		businessID,
		branchID,
		true,
	); err != nil {
		return err
	}

	sourceRef := purchasingReceiptSource(goodsReceiptID)

	var receiptID string
	var status string

	err := tx.QueryRow(
		ctx,
		`
        SELECT id,status
          FROM inventory_receipts
         WHERE business_id=$1
           AND branch_id=$2
           AND source_ref=$3
         FOR UPDATE
        `,
		businessID,
		branchID,
		sourceRef,
	).Scan(
		&receiptID,
		&status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		// A confirmed GoodsReceipt can legitimately have zero accepted lines.
		return nil
	}
	if err != nil {
		return err
	}

	if status == "CANCELLED" {
		return nil
	}
	if status != "POSTED" {
		return ErrPurchasingReceiptStateConflict
	}

	type reversalLine struct {
		receiptLineID string
		itemID        string
		quantity      int64
		lotID         string
		movementID    string
		movementAt    time.Time
	}

	rows, err := tx.Query(
		ctx,
		`
        SELECT
            rl.id,
            rl.inventory_item_id,
            rl.quantity,
            COALESCE(rl.lot_id,''),
            rl.posted_movement_id,
            m.occurred_at
          FROM inventory_receipt_lines rl
          JOIN inventory_movements m
            ON m.id=rl.posted_movement_id
           AND m.business_id=rl.business_id
           AND m.inventory_item_id=rl.inventory_item_id
         WHERE rl.business_id=$1
           AND rl.branch_id=$2
           AND rl.receipt_id=$3
         ORDER BY rl.inventory_item_id,rl.id
         FOR UPDATE OF rl,m
        `,
		businessID,
		branchID,
		receiptID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var lines []reversalLine

	for rows.Next() {
		var line reversalLine

		if err = rows.Scan(
			&line.receiptLineID,
			&line.itemID,
			&line.quantity,
			&line.lotID,
			&line.movementID,
			&line.movementAt,
		); err != nil {
			return err
		}

		lines = append(lines, line)
	}

	if err = rows.Err(); err != nil {
		return err
	}

	// Release the result set explicitly before issuing any further statement.
	// The deferred Close alone is not enough: the connection still carries this
	// query until it is closed, and the next Exec on the same transaction is
	// refused with "conn busy", which the caller then reports as an unsafe
	// reversal rather than as the plumbing fault it is.
	rows.Close()

	if err = rows.Err(); err != nil {
		return err
	}

	if _, err = tx.Exec(
		ctx,
		`SELECT set_config('karrygo.inventory_mutation','1',true)`,
	); err != nil {
		return err
	}

	for _, line := range lines {
		if line.lotID != "" {
			var onHand int64
			var reserved int64
			var quarantined int64
			var lotStatus string

			err = tx.QueryRow(
				ctx,
				`
                SELECT
                    on_hand_quantity,
                    reserved_quantity,
                    quarantined_quantity,
                    status
                  FROM inventory_lots
                 WHERE id=$1
                   AND business_id=$2
                   AND branch_id=$3
                   AND inventory_item_id=$4
                 FOR UPDATE
                `,
				line.lotID,
				businessID,
				branchID,
				line.itemID,
			).Scan(
				&onHand,
				&reserved,
				&quarantined,
				&lotStatus,
			)
			if err != nil {
				return err
			}

			if onHand != line.quantity ||
				reserved != 0 ||
				quarantined != 0 ||
				lotStatus != "ACTIVE" {
				return fmt.Errorf(
					"%w: receipt lot has already been consumed, reserved, quarantined or changed",
					ErrPurchasingReceiptStateConflict,
				)
			}

			var later int

			if err = tx.QueryRow(
				ctx,
				`
                SELECT count(*)
                  FROM inventory_movements
                 WHERE business_id=$1
                   AND inventory_item_id=$2
                   AND lot_id=$3
                   AND id<>$4
                   AND occurred_at >= $5
                `,
				businessID,
				line.itemID,
				line.lotID,
				line.movementID,
				line.movementAt,
			).Scan(&later); err != nil {
				return err
			}

			if later != 0 {
				return fmt.Errorf(
					"%w: receipt lot has later movement history",
					ErrPurchasingReceiptStateConflict,
				)
			}
		} else {
			var later int

			if err = tx.QueryRow(
				ctx,
				`
                SELECT count(*)
                  FROM inventory_movements
                 WHERE business_id=$1
                   AND branch_id=$2
                   AND inventory_item_id=$3
                   AND id<>$4
                   AND occurred_at >= $5
                `,
				businessID,
				branchID,
				line.itemID,
				line.movementID,
				line.movementAt,
			).Scan(&later); err != nil {
				return err
			}

			if later != 0 {
				return fmt.Errorf(
					"%w: untracked receipt stock has later physical movement",
					ErrPurchasingReceiptStateConflict,
				)
			}
		}

		var balanceRevision int64

		err = tx.QueryRow(
			ctx,
			`
            UPDATE inventory_balances
               SET on_hand_quantity=on_hand_quantity-$4,
                   revision=revision+1,
                   updated_at=now()
             WHERE business_id=$1
               AND branch_id=$2
               AND inventory_item_id=$3
               AND on_hand_quantity >= $4
               AND on_hand_quantity-$4 >=
                   reserved_quantity+quarantined_quantity
            RETURNING revision
            `,
			businessID,
			branchID,
			line.itemID,
			line.quantity,
		).Scan(&balanceRevision)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf(
				"%w: insufficient untouched stock for reversal",
				ErrPurchasingReceiptStateConflict,
			)
		}
		if err != nil {
			return err
		}

		if line.lotID != "" {
			tag, updateErr := tx.Exec(
				ctx,
				`
                UPDATE inventory_lots
                   SET on_hand_quantity=0,
                       status='ARCHIVED',
                       updated_at=now(),
                       version=version+1
                 WHERE id=$1
                   AND business_id=$2
                   AND on_hand_quantity=$3
                   AND reserved_quantity=0
                   AND quarantined_quantity=0
                `,
				line.lotID,
				businessID,
				line.quantity,
			)
			if updateErr != nil {
				return updateErr
			}
			if tag.RowsAffected() != 1 {
				return ErrPurchasingReceiptStateConflict
			}
		}

		inventoryRevision, revErr :=
			nextPurchasingInventoryRevisionTx(
				ctx,
				tx,
				businessID,
				branchID,
			)
		if revErr != nil {
			return revErr
		}

		reversalMovementID := "im_" + uuid.NewString()

		if _, err = tx.Exec(
			ctx,
			`
            INSERT INTO inventory_movements(
                id,
                business_id,
                branch_id,
                inventory_item_id,
                lot_id,
                movement_type,
                quantity,
                on_hand_delta,
                source_type,
                source_ref,
                reason_code,
                actor_ref,
                occurred_at,
                inventory_revision,
                balance_revision
            )
            VALUES(
                $1,$2,$3,$4,NULLIF($5,''),
                -- $6 must be cast before the unary minus: PostgreSQL cannot
                -- resolve "- unknown" and refuses the whole statement, which
                -- surfaced as a false "reversal unsafe" refusal.
                'RETURN',$6,-($6::bigint),
                'PURCHASE_RECEIPT_REVERSAL',
                $7,
                'GOODS_RECEIPT_REVERSED',
                NULLIF($8,''),
                now(),$9,$10
            )
            `,
			reversalMovementID,
			businessID,
			branchID,
			line.itemID,
			line.lotID,
			line.quantity,
			goodsReceiptID,
			actor.ActorRef,
			inventoryRevision,
			balanceRevision,
		); err != nil {
			return err
		}

		wrapped := postgresInventoryTransaction{
			ctx: ctx,
			tx:  tx,
		}

		if err = s.recordMutation(
			ctx,
			wrapped,
			actor,
			scope,
			"inventory.purchase_receipt.reversed",
			"vendor.inventory.purchase_receipt.reversed",
			"inventory_item",
			line.itemID,
			"GOODS_RECEIPT_REVERSED",
			inventoryRevision,
			map[string]any{
				"goods_receipt_id":     goodsReceiptID,
				"inventory_receipt_id": receiptID,
				"original_movement_id": line.movementID,
				"reversal_movement_id": reversalMovementID,
				"quantity":             line.quantity,
				"lot_id":               line.lotID,
			},
		); err != nil {
			return err
		}
	}

	tag, err := tx.Exec(
		ctx,
		`
        UPDATE inventory_receipts
           SET status='CANCELLED',
               cancelled_at=now(),
               updated_at=now(),
               version=version+1
         WHERE id=$1
           AND business_id=$2
           AND status='POSTED'
        `,
		receiptID,
		businessID,
	)
	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return ErrPurchasingReceiptStateConflict
	}

	return nil
}
