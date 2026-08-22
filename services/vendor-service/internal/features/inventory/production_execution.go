package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ProductionActualLine struct {
	InventoryItemID string `json:"inventory_item_id"`
	UsedQuantity    int64  `json:"used_quantity"`
	WasteQuantity   int64  `json:"waste_quantity"`
}

type ProductionOutputInput struct {
	InventoryItemID string `json:"inventory_item_id"`
	Quantity        int64  `json:"quantity"`
}

type ProductionInventorySplit struct {
	ReservationID   string `json:"reservation_id"`
	InventoryItemID string `json:"inventory_item_id"`
	LotID           string `json:"lot_id"`

	ReservedQuantity int64 `json:"reserved_quantity"`
	UsedQuantity     int64 `json:"used_quantity"`
	WasteQuantity    int64 `json:"waste_quantity"`
	ReleasedQuantity int64 `json:"released_quantity"`
}

type ProductionPostingResult struct {
	BatchRef string `json:"batch_ref"`

	Splits []ProductionInventorySplit `json:"splits"`

	OutputInventoryItemID string `json:"output_inventory_item_id,omitempty"`
	OutputQuantity        int64  `json:"output_quantity,omitempty"`
	OutputLotID           string `json:"output_lot_id,omitempty"`

	Replayed bool `json:"replayed"`
}

type productionAllocation struct {
	ID                string
	ReservationID     string
	ReservationLineID string
	InventoryItemID   string
	LotID             string
	Quantity          int64
}

type productionActualTotals struct {
	Used  int64
	Waste int64
}

func productionActorRef(actor Actor) string {
	if strings.TrimSpace(actor.StaffID) != "" {
		return strings.TrimSpace(actor.StaffID)
	}
	return strings.TrimSpace(actor.ActorRef)
}

func addProductionQuantity(a, b int64) (int64, error) {
	if a < 0 || b < 0 || a > math.MaxInt64-b {
		return 0, ErrInvalidQuantity
	}
	return a + b, nil
}

func productionPostingFingerprint(
	batchRef string,
	reservationIDs []string,
	actual []ProductionActualLine,
	output *ProductionOutputInput,
) string {
	ids := append([]string(nil), reservationIDs...)
	sort.Strings(ids)

	lines := append([]ProductionActualLine(nil), actual...)
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].InventoryItemID < lines[j].InventoryItemID
	})

	payload := struct {
		BatchRef       string                 `json:"batch_ref"`
		ReservationIDs []string               `json:"reservation_ids"`
		Actual         []ProductionActualLine `json:"actual"`
		Output         *ProductionOutputInput `json:"output,omitempty"`
	}{
		BatchRef:       strings.TrimSpace(batchRef),
		ReservationIDs: ids,
		Actual:         lines,
		Output:         output,
	}

	raw, _ := json.Marshal(payload)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func bumpProductionInventoryRevisionTx(
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
        DO UPDATE SET
            revision=inventory_revisions.revision+1,
            updated_at=now()
        RETURNING revision
        `,
		businessID,
		branchID,
	).Scan(&revision)

	return revision, err
}

func insertProductionMovementTx(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	branchID string,
	itemID string,
	lotID string,
	movementType string,
	quantity int64,
	onHandDelta int64,
	reservedDelta int64,
	sourceRef string,
	reasonCode string,
	inventoryRevision int64,
	balanceRevision int64,
) error {
	if quantity <= 0 {
		return nil
	}

	_, err := tx.Exec(
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
            reserved_delta,
            quarantined_delta,
            in_transit_delta,
            incoming_delta,
            backorder_delta,
            source_type,
            source_ref,
            reason_code,
            actor_ref,
            inventory_revision,
            balance_revision
        )
        VALUES(
            $1,$2,$3,$4,NULLIF($5,''),
            $6,$7,$8,$9,
            0,0,0,0,
            'PRODUCTION',$10,$11,NULLIF($12,''),
            $13,$14
        )
        `,
		"im_"+uuid.NewString(),
		actor.BusinessID,
		branchID,
		itemID,
		lotID,
		movementType,
		quantity,
		onHandDelta,
		reservedDelta,
		sourceRef,
		reasonCode,
		productionActorRef(actor),
		inventoryRevision,
		balanceRevision,
	)

	return err
}

// FinalizeProductionBatch is the Section 14 authority seam used by Section 16C.
//
// It atomically:
//   - consumes exact used quantity from held lots,
//   - posts WASTE for actual waste,
//   - releases reservation excess,
//   - optionally receives prepared output,
//   - writes the immutable Inventory movement ledger,
//   - emits Inventory mutation evidence,
//   - and records an exactly-once posting marker.
//
// Section 16C never directly edits Inventory balances.
func (s *StockService) FinalizeProductionBatch(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	branchID string,
	batchRef string,
	reservationIDs []string,
	actual []ProductionActualLine,
	output *ProductionOutputInput,
) (ProductionPostingResult, error) {
	if s == nil || s.db == nil {
		return ProductionPostingResult{}, ErrDependencyMissing
	}

	actor.BusinessID = strings.TrimSpace(actor.BusinessID)
	branchID = strings.TrimSpace(branchID)
	batchRef = strings.TrimSpace(batchRef)

	if actor.BusinessID == "" ||
		branchID == "" ||
		batchRef == "" ||
		!scope.Covers(actor.BusinessID, branchID) {
		return ProductionPostingResult{}, ErrInvalidQuantity
	}

	actualTotals := make(map[string]productionActualTotals, len(actual))

	for _, line := range actual {
		itemID := strings.TrimSpace(line.InventoryItemID)

		if itemID == "" ||
			line.UsedQuantity < 0 ||
			line.WasteQuantity < 0 {
			return ProductionPostingResult{}, ErrInvalidQuantity
		}

		current := actualTotals[itemID]

		var err error
		current.Used, err = addProductionQuantity(
			current.Used,
			line.UsedQuantity,
		)
		if err != nil {
			return ProductionPostingResult{}, err
		}

		current.Waste, err = addProductionQuantity(
			current.Waste,
			line.WasteQuantity,
		)
		if err != nil {
			return ProductionPostingResult{}, err
		}

		actualTotals[itemID] = current
	}

	if output != nil {
		output.InventoryItemID = strings.TrimSpace(output.InventoryItemID)
		if output.InventoryItemID == "" || output.Quantity <= 0 {
			return ProductionPostingResult{}, ErrInvalidQuantity
		}
	}

	reservationIDs = append([]string(nil), reservationIDs...)
	for i := range reservationIDs {
		reservationIDs[i] = strings.TrimSpace(reservationIDs[i])
		if reservationIDs[i] == "" {
			return ProductionPostingResult{}, ErrInvalidQuantity
		}
	}
	sort.Strings(reservationIDs)

	fingerprint := productionPostingFingerprint(
		batchRef,
		reservationIDs,
		actual,
		output,
	)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ProductionPostingResult{}, err
	}
	defer tx.Rollback(ctx)

	if err = enableInventoryMutation(ctx, tx); err != nil {
		return ProductionPostingResult{}, err
	}

	// Exactly-once replay.
	var existingFingerprint string
	var existingJSON []byte

	err = tx.QueryRow(
		ctx,
		`
        SELECT request_fingerprint,result_json
          FROM inventory_production_postings
         WHERE business_id=$1
           AND batch_ref=$2
         FOR UPDATE
        `,
		actor.BusinessID,
		batchRef,
	).Scan(
		&existingFingerprint,
		&existingJSON,
	)

	if err == nil {
		if existingFingerprint != fingerprint {
			return ProductionPostingResult{},
				fmt.Errorf("%w: production posting fingerprint mismatch", ErrReservationIdempotencyConflict)
		}

		var replay ProductionPostingResult
		if err = json.Unmarshal(existingJSON, &replay); err != nil {
			return ProductionPostingResult{}, err
		}

		replay.Replayed = true

		if err = tx.Commit(ctx); err != nil {
			return ProductionPostingResult{}, err
		}

		return replay, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return ProductionPostingResult{}, err
	}

	// Lock reservation headers in deterministic order and prove ownership,
	// branch, state and expiry.
	for _, reservationID := range reservationIDs {
		var status string
		var sourceType string
		var expiresAt *time.Time

		err = tx.QueryRow(
			ctx,
			`
            SELECT status,source_type,expires_at
              FROM inventory_reservations
             WHERE id=$1
               AND business_id=$2
               AND branch_id=$3
             FOR UPDATE
            `,
			reservationID,
			actor.BusinessID,
			branchID,
		).Scan(
			&status,
			&sourceType,
			&expiresAt,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ProductionPostingResult{}, ErrReservationNotFound
		}
		if err != nil {
			return ProductionPostingResult{}, err
		}

		if sourceType != ReservationSourceInternal {
			return ProductionPostingResult{}, ErrReservationStateConflict
		}

		if status != ReservationHeld &&
			status != ReservationConfirmed {
			return ProductionPostingResult{}, ErrReservationStateConflict
		}

		if expiresAt != nil && !expiresAt.After(time.Now().UTC()) {
			return ProductionPostingResult{}, ErrReservationExpired
		}
	}

	allocations := make([]productionAllocation, 0)

	if len(reservationIDs) > 0 {
		rows, queryErr := tx.Query(
			ctx,
			`
            SELECT
                id,
                reservation_id,
                reservation_line_id,
                inventory_item_id,
                lot_id,
                quantity
              FROM inventory_reservation_allocations
             WHERE business_id=$1
               AND branch_id=$2
               AND reservation_id=ANY($3)
               AND status='HELD'
             ORDER BY
                inventory_item_id,
                lot_id,
                reservation_id,
                id
             FOR UPDATE
            `,
			actor.BusinessID,
			branchID,
			reservationIDs,
		)
		if queryErr != nil {
			return ProductionPostingResult{}, queryErr
		}

		for rows.Next() {
			var row productionAllocation
			if scanErr := rows.Scan(
				&row.ID,
				&row.ReservationID,
				&row.ReservationLineID,
				&row.InventoryItemID,
				&row.LotID,
				&row.Quantity,
			); scanErr != nil {
				rows.Close()
				return ProductionPostingResult{}, scanErr
			}
			allocations = append(allocations, row)
		}

		if rows.Err() != nil {
			rows.Close()
			return ProductionPostingResult{}, rows.Err()
		}
		rows.Close()
	}

	reservedByItem := map[string]int64{}
	lotIDs := make([]string, 0, len(allocations))
	itemIDs := make([]string, 0)
	itemSeen := map[string]bool{}

	for _, allocation := range allocations {
		next, addErr := addProductionQuantity(
			reservedByItem[allocation.InventoryItemID],
			allocation.Quantity,
		)
		if addErr != nil {
			return ProductionPostingResult{}, addErr
		}

		reservedByItem[allocation.InventoryItemID] = next
		lotIDs = append(lotIDs, allocation.LotID)

		if !itemSeen[allocation.InventoryItemID] {
			itemSeen[allocation.InventoryItemID] = true
			itemIDs = append(itemIDs, allocation.InventoryItemID)
		}
	}

	for itemID, totals := range actualTotals {
		total, addErr := addProductionQuantity(
			totals.Used,
			totals.Waste,
		)
		if addErr != nil {
			return ProductionPostingResult{}, addErr
		}

		if total > reservedByItem[itemID] {
			return ProductionPostingResult{},
				fmt.Errorf("%w: production actual quantity exceeds reserved quantity for %s", ErrInsufficientStock, itemID)
		}
	}

	sort.Strings(lotIDs)
	sort.Strings(itemIDs)

	if len(lotIDs) > 0 {
		rows, lockErr := tx.Query(
			ctx,
			`
            SELECT id
              FROM inventory_lots
             WHERE business_id=$1
               AND branch_id=$2
               AND id=ANY($3)
             ORDER BY id
             FOR UPDATE
            `,
			actor.BusinessID,
			branchID,
			lotIDs,
		)
		if lockErr != nil {
			return ProductionPostingResult{}, lockErr
		}
		for rows.Next() {
			var ignored string
			if lockErr = rows.Scan(&ignored); lockErr != nil {
				rows.Close()
				return ProductionPostingResult{}, lockErr
			}
		}
		if rows.Err() != nil {
			rows.Close()
			return ProductionPostingResult{}, rows.Err()
		}
		rows.Close()
	}

	if len(itemIDs) > 0 {
		rows, lockErr := tx.Query(
			ctx,
			`
            SELECT inventory_item_id
              FROM inventory_balances
             WHERE business_id=$1
               AND branch_id=$2
               AND inventory_item_id=ANY($3)
             ORDER BY inventory_item_id
             FOR UPDATE
            `,
			actor.BusinessID,
			branchID,
			itemIDs,
		)
		if lockErr != nil {
			return ProductionPostingResult{}, lockErr
		}

		locked := 0
		for rows.Next() {
			var ignored string
			if lockErr = rows.Scan(&ignored); lockErr != nil {
				rows.Close()
				return ProductionPostingResult{}, lockErr
			}
			locked++
		}
		if rows.Err() != nil {
			rows.Close()
			return ProductionPostingResult{}, rows.Err()
		}
		rows.Close()

		if locked != len(itemIDs) {
			return ProductionPostingResult{}, ErrInsufficientStock
		}
	}

	remaining := make(map[string]productionActualTotals, len(actualTotals))
	for itemID, totals := range actualTotals {
		remaining[itemID] = totals
	}

	result := ProductionPostingResult{
		BatchRef: batchRef,
		Splits:   make([]ProductionInventorySplit, 0, len(allocations)),
	}

	consumedByReservation := map[string]int64{}
	consumedByLine := map[string]int64{}

	for _, allocation := range allocations {
		state := remaining[allocation.InventoryItemID]

		used := int64(0)
		if state.Used > 0 {
			used = state.Used
			if used > allocation.Quantity {
				used = allocation.Quantity
			}
			state.Used -= used
		}

		left := allocation.Quantity - used

		waste := int64(0)
		if left > 0 && state.Waste > 0 {
			waste = state.Waste
			if waste > left {
				waste = left
			}
			state.Waste -= waste
		}

		released := allocation.Quantity - used - waste

		remaining[allocation.InventoryItemID] = state

		consumed := used + waste

		command, updateErr := tx.Exec(
			ctx,
			`
            UPDATE inventory_lots
               SET on_hand_quantity=on_hand_quantity-$3,
                   reserved_quantity=reserved_quantity-$4,
                   status=CASE
                       WHEN on_hand_quantity-$3=0 THEN 'DEPLETED'
                       ELSE status
                   END,
                   updated_at=now(),
                   version=version+1
             WHERE id=$1
               AND business_id=$2
               AND branch_id=$5
               AND reserved_quantity >= $4
               AND on_hand_quantity >= $3
            `,
			allocation.LotID,
			actor.BusinessID,
			consumed,
			allocation.Quantity,
			branchID,
		)
		if updateErr != nil {
			return ProductionPostingResult{}, updateErr
		}
		if command.RowsAffected() != 1 {
			return ProductionPostingResult{}, ErrInsufficientStock
		}

		allocStatus := ReservationReleased
		if consumed > 0 {
			allocStatus = ReservationCommitted
		}

		if _, updateErr = tx.Exec(
			ctx,
			`
            UPDATE inventory_reservation_allocations
               SET status=$3,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
            `,
			allocation.ID,
			actor.BusinessID,
			allocStatus,
		); updateErr != nil {
			return ProductionPostingResult{}, updateErr
		}

		consumedByReservation[allocation.ReservationID] += consumed
		consumedByLine[allocation.ReservationLineID] += consumed

		result.Splits = append(
			result.Splits,
			ProductionInventorySplit{
				ReservationID:    allocation.ReservationID,
				InventoryItemID:  allocation.InventoryItemID,
				LotID:            allocation.LotID,
				ReservedQuantity: allocation.Quantity,
				UsedQuantity:     used,
				WasteQuantity:    waste,
				ReleasedQuantity: released,
			},
		)
	}

	for itemID, state := range remaining {
		if state.Used != 0 || state.Waste != 0 {
			return ProductionPostingResult{}, ErrInsufficientStock
		}
		_ = itemID
	}

	lastInventoryRevision := int64(0)

	for _, itemID := range itemIDs {
		totals := actualTotals[itemID]
		consumed := totals.Used + totals.Waste
		reserved := reservedByItem[itemID]
		released := reserved - consumed

		var balanceRevision int64

		err = tx.QueryRow(
			ctx,
			`
            UPDATE inventory_balances
               SET on_hand_quantity=on_hand_quantity-$4,
                   reserved_quantity=reserved_quantity-$5,
                   revision=revision+1,
                   updated_at=now()
             WHERE business_id=$1
               AND branch_id=$2
               AND inventory_item_id=$3
               AND on_hand_quantity >= $4
               AND reserved_quantity >= $5
             RETURNING revision
            `,
			actor.BusinessID,
			branchID,
			itemID,
			consumed,
			reserved,
		).Scan(&balanceRevision)

		if errors.Is(err, pgx.ErrNoRows) {
			return ProductionPostingResult{}, ErrInsufficientStock
		}
		if err != nil {
			return ProductionPostingResult{}, err
		}

		for _, split := range result.Splits {
			if split.InventoryItemID != itemID {
				continue
			}

			if split.UsedQuantity > 0 {
				lastInventoryRevision, err =
					bumpProductionInventoryRevisionTx(
						ctx,
						tx,
						actor.BusinessID,
						branchID,
					)
				if err != nil {
					return ProductionPostingResult{}, err
				}

				if err = insertProductionMovementTx(
					ctx,
					tx,
					actor,
					branchID,
					itemID,
					split.LotID,
					MovementCommit,
					split.UsedQuantity,
					-split.UsedQuantity,
					-split.UsedQuantity,
					batchRef,
					"PRODUCTION_USE",
					lastInventoryRevision,
					balanceRevision,
				); err != nil {
					return ProductionPostingResult{}, err
				}
			}

			if split.WasteQuantity > 0 {
				lastInventoryRevision, err =
					bumpProductionInventoryRevisionTx(
						ctx,
						tx,
						actor.BusinessID,
						branchID,
					)
				if err != nil {
					return ProductionPostingResult{}, err
				}

				if err = insertProductionMovementTx(
					ctx,
					tx,
					actor,
					branchID,
					itemID,
					split.LotID,
					MovementWaste,
					split.WasteQuantity,
					-split.WasteQuantity,
					-split.WasteQuantity,
					batchRef,
					"PRODUCTION_WASTE",
					lastInventoryRevision,
					balanceRevision,
				); err != nil {
					return ProductionPostingResult{}, err
				}
			}

			if split.ReleasedQuantity > 0 {
				lastInventoryRevision, err =
					bumpProductionInventoryRevisionTx(
						ctx,
						tx,
						actor.BusinessID,
						branchID,
					)
				if err != nil {
					return ProductionPostingResult{}, err
				}

				if err = insertProductionMovementTx(
					ctx,
					tx,
					actor,
					branchID,
					itemID,
					split.LotID,
					MovementRelease,
					split.ReleasedQuantity,
					0,
					-split.ReleasedQuantity,
					batchRef,
					"PRODUCTION_EXCESS_RELEASE",
					lastInventoryRevision,
					balanceRevision,
				); err != nil {
					return ProductionPostingResult{}, err
				}
			}
		}

		_ = released
	}

	for lineID, consumed := range consumedByLine {
		status := ReservationReleased
		if consumed > 0 {
			status = ReservationCommitted
		}

		if _, err = tx.Exec(
			ctx,
			`
            UPDATE inventory_reservation_lines
               SET status=$3,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
            `,
			lineID,
			actor.BusinessID,
			status,
		); err != nil {
			return ProductionPostingResult{}, err
		}
	}

	for _, reservationID := range reservationIDs {
		consumed := consumedByReservation[reservationID]

		if consumed > 0 {
			if _, err = tx.Exec(
				ctx,
				`
                UPDATE inventory_reservations
                   SET status='COMMITTED',
                       committed_at=now(),
                       updated_at=now(),
                       version=version+1
                 WHERE id=$1
                   AND business_id=$2
                `,
				reservationID,
				actor.BusinessID,
			); err != nil {
				return ProductionPostingResult{}, err
			}
		} else {
			if _, err = tx.Exec(
				ctx,
				`
                UPDATE inventory_reservations
                   SET status='RELEASED',
                       released_at=now(),
                       updated_at=now(),
                       version=version+1
                 WHERE id=$1
                   AND business_id=$2
                `,
				reservationID,
				actor.BusinessID,
			); err != nil {
				return ProductionPostingResult{}, err
			}
		}
	}

	// Optional BATCH_STOCK output.
	if output != nil {
		var itemType string
		var itemStatus string

		err = tx.QueryRow(
			ctx,
			`
            SELECT item_type,status
              FROM inventory_items
             WHERE id=$1
               AND business_id=$2
               AND branch_id=$3
             FOR UPDATE
            `,
			output.InventoryItemID,
			actor.BusinessID,
			branchID,
		).Scan(
			&itemType,
			&itemStatus,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ProductionPostingResult{}, ErrItemNotFound
		}
		if err != nil {
			return ProductionPostingResult{}, err
		}

		if itemType != ItemTypeProductionComponent ||
			itemStatus != ItemStatusActive {
			return ProductionPostingResult{}, ErrSourceInactive
		}

		if _, err = tx.Exec(
			ctx,
			`
            INSERT INTO inventory_balances(
                id,
                business_id,
                branch_id,
                inventory_item_id,
                on_hand_quantity,
                reserved_quantity,
                quarantined_quantity,
                in_transit_quantity,
                incoming_quantity,
                backorder_quantity,
                revision,
                updated_at
            )
            VALUES(
                $1,$2,$3,$4,
                0,0,0,0,0,0,
                0,now()
            )
            ON CONFLICT(
                business_id,
                branch_id,
                inventory_item_id
            )
            DO NOTHING
            `,
			"ib_"+uuid.NewString(),
			actor.BusinessID,
			branchID,
			output.InventoryItemID,
		); err != nil {
			return ProductionPostingResult{}, err
		}

		outputLotID := "ilot_" + uuid.NewString()

		if _, err = tx.Exec(
			ctx,
			`
            INSERT INTO inventory_lots(
                id,
                business_id,
                branch_id,
                inventory_item_id,
                source_type,
                source_ref,
                received_at,
                on_hand_quantity,
                reserved_quantity,
                quarantined_quantity,
                status
            )
            VALUES(
                $1,$2,$3,$4,
                'PRODUCTION_OUTPUT',$5,now(),
                $6,0,0,'ACTIVE'
            )
            `,
			outputLotID,
			actor.BusinessID,
			branchID,
			output.InventoryItemID,
			batchRef,
			output.Quantity,
		); err != nil {
			return ProductionPostingResult{}, err
		}

		var balanceRevision int64

		err = tx.QueryRow(
			ctx,
			`
            UPDATE inventory_balances
               SET on_hand_quantity=on_hand_quantity+$4,
                   revision=revision+1,
                   updated_at=now()
             WHERE business_id=$1
               AND branch_id=$2
               AND inventory_item_id=$3
             RETURNING revision
            `,
			actor.BusinessID,
			branchID,
			output.InventoryItemID,
			output.Quantity,
		).Scan(&balanceRevision)
		if err != nil {
			return ProductionPostingResult{}, err
		}

		lastInventoryRevision, err =
			bumpProductionInventoryRevisionTx(
				ctx,
				tx,
				actor.BusinessID,
				branchID,
			)
		if err != nil {
			return ProductionPostingResult{}, err
		}

		if err = insertProductionMovementTx(
			ctx,
			tx,
			actor,
			branchID,
			output.InventoryItemID,
			outputLotID,
			MovementReceive,
			output.Quantity,
			output.Quantity,
			0,
			batchRef,
			"PRODUCTION_OUTPUT",
			lastInventoryRevision,
			balanceRevision,
		); err != nil {
			return ProductionPostingResult{}, err
		}

		result.OutputInventoryItemID = output.InventoryItemID
		result.OutputQuantity = output.Quantity
		result.OutputLotID = outputLotID
	}

	result.Replayed = false

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return ProductionPostingResult{}, err
	}

	if _, err = tx.Exec(
		ctx,
		`
        INSERT INTO inventory_production_postings(
            id,
            business_id,
            branch_id,
            batch_ref,
            request_fingerprint,
            result_json
        )
        VALUES($1,$2,$3,$4,$5,$6)
        `,
		"ipp_"+uuid.NewString(),
		actor.BusinessID,
		branchID,
		batchRef,
		fingerprint,
		resultJSON,
	); err != nil {
		return ProductionPostingResult{}, err
	}

	if lastInventoryRevision == 0 {
		lastInventoryRevision, err =
			bumpProductionInventoryRevisionTx(
				ctx,
				tx,
				actor.BusinessID,
				branchID,
			)
		if err != nil {
			return ProductionPostingResult{}, err
		}
	}

	inventoryTx := postgresInventoryTransaction{
		ctx: ctx,
		tx:  tx,
	}

	if err = s.recordMutation(
		ctx,
		inventoryTx,
		actor,
		scope,
		"inventory.stock.adjusted",
		"inventory.production.finalized",
		"inventory_production_posting",
		batchRef,
		"PRODUCTION",
		lastInventoryRevision,
		map[string]any{
			"batch_ref":       batchRef,
			"split_count":     len(result.Splits),
			"output_item_id":  result.OutputInventoryItemID,
			"output_quantity": result.OutputQuantity,
		},
	); err != nil {
		return ProductionPostingResult{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return ProductionPostingResult{}, err
	}

	return result, nil
}
