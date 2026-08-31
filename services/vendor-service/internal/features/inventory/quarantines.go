package inventory

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *StockService) CreateQuarantine(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input CreateQuarantineInput,
) (
	QuarantineCase,
	error,
) {
	input.BranchID =
		strings.TrimSpace(
			input.BranchID,
		)

	input.InventoryItemID =
		strings.TrimSpace(
			input.InventoryItemID,
		)

	input.ReasonCode =
		strings.ToUpper(
			strings.TrimSpace(
				input.ReasonCode,
			),
		)

	if input.Quantity <= 0 ||
		input.Quantity > maxStockQuantity {
		return QuarantineCase{},
			ErrInvalidQuantity
	}

	if input.ReasonCode == "" {
		return QuarantineCase{},
			ErrReasonRequired
	}

	if len(input.Note) > 1000 {
		return QuarantineCase{},
			ErrInvalidQuantity
	}

	var out QuarantineCase

	err := s.withWriteTransaction(
		ctx,
		actor,
		scope,
		input.BranchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			item, err :=
				loadStockItemForUpdate(
					ctx,
					tx,
					actor.BusinessID,
					input.BranchID,
					input.InventoryItemID,
				)

			if err != nil {
				return err
			}

			allocations, err :=
				allocateAvailableLotsTx(
					ctx,
					tx,
					item,
					input.Quantity,
				)

			if err != nil {
				return err
			}

			out = QuarantineCase{
				ID: stockID("iq_"),

				BusinessID: actor.BusinessID,

				BranchID: input.BranchID,

				InventoryItemID: input.InventoryItemID,

				Quantity: input.Quantity,

				ReasonCode: input.ReasonCode,

				Note: input.Note,

				Status: QuarantineActive,

				Version: 1,
			}

			err = tx.QueryRow(
				ctx,
				`
                INSERT INTO inventory_quarantines (
                    id,
                    business_id,
                    branch_id,
                    inventory_item_id,
                    quantity,
                    reason_code,
                    note
                )
                VALUES (
                    $1,$2,$3,$4,$5,$6,$7
                )
                RETURNING created_at
                `,
				out.ID,
				out.BusinessID,
				out.BranchID,
				out.InventoryItemID,
				out.Quantity,
				out.ReasonCode,
				out.Note,
			).Scan(
				&out.CreatedAt,
			)

			if err != nil {
				return err
			}

			for _, allocation := range allocations {

				tag, err :=
					tx.Exec(
						ctx,
						`
                        UPDATE inventory_lots
                           SET quarantined_quantity=
                               quarantined_quantity+$3,
                               status=
                               CASE
                                   WHEN quarantined_quantity+$3
                                        = on_hand_quantity
                                   THEN 'QUARANTINED'
                                   ELSE status
                               END,
                               updated_at=now(),
                               version=version+1
                         WHERE id=$1
                           AND business_id=$2
                           AND status='ACTIVE'
                           AND (
                               expires_at IS NULL
                               OR expires_at > now()
                           )
                           AND (
                               on_hand_quantity
                               - reserved_quantity
                               - quarantined_quantity
                           ) >= $3
                        `,
						allocation.ID,
						actor.BusinessID,
						allocation.Quantity,
					)

				if err != nil {
					return err
				}

				if tag.RowsAffected() != 1 {
					return ErrInsufficientStock
				}

				_, err =
					tx.Exec(
						ctx,
						`
                        INSERT INTO inventory_quarantine_allocations (
                            id,
                            business_id,
                            quarantine_id,
                            inventory_item_id,
                            lot_id,
                            quantity
                        )
                        VALUES (
                            $1,$2,$3,$4,$5,$6
                        )
                        `,
						stockID("iqa_"),
						actor.BusinessID,
						out.ID,
						item.ID,
						allocation.ID,
						allocation.Quantity,
					)

				if err != nil {
					return err
				}
			}

			balanceRevision, err :=
				updateBalanceTx(
					ctx,
					tx,
					item,
					0,
					0,
					input.Quantity,
					0,
					0,
					0,
				)

			if err != nil {
				return err
			}

			inventoryRevision, err :=
				bumpInventoryRevisionTx(
					ctx,
					tx,
					item.BusinessID,
					item.BranchID,
				)

			if err != nil {
				return err
			}

			for _, allocation := range allocations {

				_, err :=
					appendMovementTx(
						ctx,
						tx,
						actor,
						item,
						allocation.ID,
						MovementQuarantineIn,
						allocation.Quantity,
						0,
						0,
						allocation.Quantity,
						0,
						0,
						0,
						"QUARANTINE",
						out.ID,
						input.ReasonCode,
						inventoryRevision,
						balanceRevision,
					)

				if err != nil {
					return err
				}
			}

			exactScope, _ :=
				ExactBranchScope(
					actor.BusinessID,
					input.BranchID,
				)

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				exactScope,
				"inventory.quarantine.created",
				"inventory.quarantine.created",
				"inventory_quarantine",
				out.ID,
				out.ReasonCode,
				inventoryRevision,
				map[string]any{
					"quarantine": out,
				},
			)
		},
	)

	return out, err
}

func loadQuarantineTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	id string,
) (
	QuarantineCase,
	error,
) {
	var out QuarantineCase

	err := tx.QueryRow(
		ctx,
		`
        SELECT
            id,
            business_id,
            branch_id,
            inventory_item_id,
            quantity,
            reason_code,
            note,
            status,
            created_at,
            resolved_at,
            version
          FROM inventory_quarantines
         WHERE id=$1
           AND business_id=$2
         FOR UPDATE
        `,
		id,
		businessID,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.InventoryItemID,
		&out.Quantity,
		&out.ReasonCode,
		&out.Note,
		&out.Status,
		&out.CreatedAt,
		&out.ResolvedAt,
		&out.Version,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return QuarantineCase{},
			ErrQuarantineNotFound
	}

	return out, err
}

func (s *StockService) QuarantineBranch(
	ctx context.Context,
	businessID string,
	id string,
) (
	string,
	error,
) {
	var branchID string

	err := s.db.QueryRow(
		ctx,
		`
        SELECT branch_id
          FROM inventory_quarantines
         WHERE id=$1
           AND business_id=$2
        `,
		id,
		businessID,
	).Scan(
		&branchID,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return "",
			ErrQuarantineNotFound
	}

	return branchID, err
}

func (s *StockService) ResolveQuarantine(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	id string,
	dispose bool,
	reason string,
) (
	QuarantineCase,
	error,
) {
	branchID, err :=
		s.QuarantineBranch(
			ctx,
			actor.BusinessID,
			id,
		)

	if err != nil {
		return QuarantineCase{}, err
	}

	if dispose &&
		strings.TrimSpace(reason) == "" {
		return QuarantineCase{},
			ErrReasonRequired
	}

	var out QuarantineCase

	err = s.withWriteTransaction(
		ctx,
		actor,
		scope,
		branchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			current, err :=
				loadQuarantineTx(
					ctx,
					tx,
					actor.BusinessID,
					id,
				)

			if err != nil {
				return err
			}

			target :=
				QuarantineReleased

			if dispose {
				target =
					QuarantineDisposed
			}

			if current.Status ==
				target {

				out = current
				return nil
			}

			if current.Status !=
				QuarantineActive {

				return ErrQuarantineStateConflict
			}

			item, err :=
				loadStockItemForUpdate(
					ctx,
					tx,
					actor.BusinessID,
					current.BranchID,
					current.InventoryItemID,
				)

			if err != nil {
				return err
			}

			rows, err :=
				tx.Query(
					ctx,
					`
                    SELECT
                        id,
                        lot_id,
                        quantity
                      FROM inventory_quarantine_allocations
                     WHERE quarantine_id=$1
                       AND business_id=$2
                       AND status='ACTIVE'
                     ORDER BY lot_id
                     FOR UPDATE
                    `,
					current.ID,
					current.BusinessID,
				)

			if err != nil {
				return err
			}

			type allocation struct {
				ID       string
				LotID    string
				Quantity int64
			}

			var allocations []allocation

			for rows.Next() {
				var entry allocation

				if err := rows.Scan(
					&entry.ID,
					&entry.LotID,
					&entry.Quantity,
				); err != nil {
					rows.Close()
					return err
				}

				allocations =
					append(
						allocations,
						entry,
					)
			}

			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}

			rows.Close()

			if len(allocations) == 0 {
				return ErrQuarantineStateConflict
			}

			for _, allocation := range allocations {

				if dispose {
					tag, err :=
						tx.Exec(
							ctx,
							`
                            UPDATE inventory_lots
                               SET on_hand_quantity=
                                   on_hand_quantity-$3,
                                   quarantined_quantity=
                                   quarantined_quantity-$3,
                                   status=
                                   CASE
                                       WHEN on_hand_quantity-$3=0
                                       THEN 'DEPLETED'
                                       ELSE 'ACTIVE'
                                   END,
                                   updated_at=now(),
                                   version=version+1
                             WHERE id=$1
                               AND business_id=$2
                               AND quarantined_quantity >= $3
                               AND on_hand_quantity >= $3
                            `,
							allocation.LotID,
							actor.BusinessID,
							allocation.Quantity,
						)

					if err != nil {
						return err
					}

					if tag.RowsAffected() != 1 {
						return ErrQuarantineStateConflict
					}
				} else {
					tag, err :=
						tx.Exec(
							ctx,
							`
                            UPDATE inventory_lots
                               SET quarantined_quantity=
                                   quarantined_quantity-$3,
                                   status=
                                   CASE
                                       WHEN expires_at IS NOT NULL
                                        AND expires_at <= now()
                                       THEN 'EXPIRED'
                                       ELSE 'ACTIVE'
                                   END,
                                   updated_at=now(),
                                   version=version+1
                             WHERE id=$1
                               AND business_id=$2
                               AND quarantined_quantity >= $3
                            `,
							allocation.LotID,
							actor.BusinessID,
							allocation.Quantity,
						)

					if err != nil {
						return err
					}

					if tag.RowsAffected() != 1 {
						return ErrQuarantineStateConflict
					}
				}
			}

			onHandDelta :=
				int64(0)

			if dispose {
				onHandDelta =
					-current.Quantity
			}

			balanceRevision, err :=
				updateBalanceTx(
					ctx,
					tx,
					item,
					onHandDelta,
					0,
					-current.Quantity,
					0,
					0,
					0,
				)

			if err != nil {
				return err
			}

			inventoryRevision, err :=
				bumpInventoryRevisionTx(
					ctx,
					tx,
					item.BusinessID,
					item.BranchID,
				)

			if err != nil {
				return err
			}

			movementType :=
				MovementQuarantineRelease

			movementReason :=
				"QUARANTINE_RELEASE"

			if dispose {
				movementType =
					MovementWaste

				movementReason =
					strings.ToUpper(
						strings.TrimSpace(
							reason,
						),
					)
			}

			for _, allocation := range allocations {

				_, err :=
					appendMovementTx(
						ctx,
						tx,
						actor,
						item,
						allocation.LotID,
						movementType,
						allocation.Quantity,
						func() int64 {
							if dispose {
								return -allocation.Quantity
							}

							return 0
						}(),
						0,
						-allocation.Quantity,
						0,
						0,
						0,
						"QUARANTINE",
						current.ID,
						movementReason,
						inventoryRevision,
						balanceRevision,
					)

				if err != nil {
					return err
				}

				_, err =
					tx.Exec(
						ctx,
						`
                        UPDATE inventory_quarantine_allocations
                           SET status=$3,
                               updated_at=now()
                         WHERE id=$1
                           AND business_id=$2
                        `,
						allocation.ID,
						current.BusinessID,
						target,
					)

				if err != nil {
					return err
				}
			}

			err = tx.QueryRow(
				ctx,
				`
                UPDATE inventory_quarantines
                   SET status=$3,
                       resolved_at=now(),
                       version=version+1
                 WHERE id=$1
                   AND business_id=$2
                RETURNING
                    status,
                    resolved_at,
                    version
                `,
				current.ID,
				current.BusinessID,
				target,
			).Scan(
				&current.Status,
				&current.ResolvedAt,
				&current.Version,
			)

			if err != nil {
				return err
			}

			out = current

			exactScope, _ :=
				ExactBranchScope(
					actor.BusinessID,
					current.BranchID,
				)

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				exactScope,
				"inventory.quarantine.resolved",
				"inventory.quarantine.resolved",
				"inventory_quarantine",
				current.ID,
				movementReason,
				inventoryRevision,
				map[string]any{
					"quarantine": current,
				},
			)
		},
	)

	return out, err
}
