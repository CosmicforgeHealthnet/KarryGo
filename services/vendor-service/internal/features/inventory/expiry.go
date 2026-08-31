package inventory

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type dueLot struct {
	ID string

	BusinessID string
	BranchID   string
	ItemID     string
}

func (s *StockService) ExpireDueLots(
	ctx context.Context,
	limit int,
) (
	int,
	error,
) {
	if s == nil ||
		s.db == nil {
		return 0,
			ErrDependencyMissing
	}

	if limit <= 0 {
		limit = 100
	}

	if limit > 1000 {
		limit = 1000
	}

	rows, err :=
		s.db.Query(
			ctx,
			`
            SELECT
                id,
                business_id,
                branch_id,
                inventory_item_id
              FROM inventory_lots
             WHERE expires_at IS NOT NULL
               AND expires_at <= now()
               AND on_hand_quantity > 0
               AND status IN (
                   'ACTIVE',
                   'EXPIRED'
               )
             ORDER BY expires_at,id
             LIMIT $1
            `,
			limit,
		)

	if err != nil {
		return 0, err
	}

	var due []dueLot

	for rows.Next() {
		var item dueLot

		if err := rows.Scan(
			&item.ID,
			&item.BusinessID,
			&item.BranchID,
			&item.ItemID,
		); err != nil {
			rows.Close()
			return 0, err
		}

		due =
			append(
				due,
				item,
			)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}

	rows.Close()

	processed := 0

	for _, candidate := range due {

		scope, err :=
			ExactBranchScope(
				candidate.BusinessID,
				candidate.BranchID,
			)

		if err != nil {
			return processed, err
		}

		actor :=
			Actor{
				ActorRef: "inventory-expiry",

				BusinessID: candidate.BusinessID,
			}

		expired :=
			false

		err = s.withWriteTransaction(
			ctx,
			actor,
			scope,
			candidate.BranchID,
			func(
				inventoryTx InventoryTransaction,
				tx pgx.Tx,
			) error {
				item, err :=
					loadStockItemForUpdate(
						ctx,
						tx,
						candidate.BusinessID,
						candidate.BranchID,
						candidate.ItemID,
					)

				if err != nil {
					return err
				}

				var (
					onHand      int64
					reserved    int64
					quarantined int64
				)

				err =
					tx.QueryRow(
						ctx,
						`
                        SELECT
                            on_hand_quantity,
                            reserved_quantity,
                            quarantined_quantity
                          FROM inventory_lots
                         WHERE id=$1
                           AND business_id=$2
                           AND inventory_item_id=$3
                           AND expires_at IS NOT NULL
                           AND expires_at <= now()
                         FOR UPDATE
                        `,
						candidate.ID,
						candidate.BusinessID,
						candidate.ItemID,
					).Scan(
						&onHand,
						&reserved,
						&quarantined,
					)

				if errors.Is(
					err,
					pgx.ErrNoRows,
				) {
					return nil
				}

				if err != nil {
					return err
				}

				if onHand <= 0 {
					return nil
				}

				// Encumbered expired goods are already excluded from sellability.
				// They require reservation/quarantine resolution before physical
				// expiry disposal can safely alter on_hand.
				if reserved > 0 ||
					quarantined > 0 {

					_, err =
						upsertAlertTx(
							ctx,
							tx,
							candidate.BusinessID,
							candidate.BranchID,
							candidate.ItemID,
							AlertExpired,
							"CRITICAL",
							"EXPIRED:"+
								candidate.BranchID+
								":"+
								candidate.ItemID,
							map[string]any{
								"lot_id": candidate.ID,

								"on_hand_quantity": onHand,

								"reserved_quantity": reserved,

								"quarantined_quantity": quarantined,

								"requires_resolution": true,
							},
						)

					return err
				}

				tag, err :=
					tx.Exec(
						ctx,
						`
                        UPDATE inventory_lots
                           SET on_hand_quantity=0,
                               status='EXPIRED',
                               updated_at=now(),
                               version=version+1
                         WHERE id=$1
                           AND business_id=$2
                           AND reserved_quantity=0
                           AND quarantined_quantity=0
                        `,
						candidate.ID,
						candidate.BusinessID,
					)

				if err != nil {
					return err
				}

				if tag.RowsAffected() != 1 {
					return nil
				}

				balanceRevision, err :=
					updateBalanceTx(
						ctx,
						tx,
						item,
						-onHand,
						0,
						0,
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

				_, err =
					appendMovementTx(
						ctx,
						tx,
						actor,
						item,
						candidate.ID,
						MovementExpiry,
						onHand,
						-onHand,
						0,
						0,
						0,
						0,
						0,
						"LOT_EXPIRY",
						candidate.ID,
						"LOT_EXPIRED",
						inventoryRevision,
						balanceRevision,
					)

				if err != nil {
					return err
				}

				_, err =
					upsertAlertTx(
						ctx,
						tx,
						candidate.BusinessID,
						candidate.BranchID,
						candidate.ItemID,
						AlertExpired,
						"CRITICAL",
						"EXPIRED:"+
							candidate.BranchID+
							":"+
							candidate.ItemID,
						map[string]any{
							"lot_id": candidate.ID,

							"expired_quantity": onHand,
						},
					)

				if err != nil {
					return err
				}

				expired = true

				return s.recordMutation(
					ctx,
					inventoryTx,
					actor,
					scope,
					"inventory.stock.expired",
					"inventory.stock.expired",
					"inventory_item",
					item.ID,
					"LOT_EXPIRED",
					inventoryRevision,
					map[string]any{
						"lot_id": candidate.ID,

						"quantity": onHand,
					},
				)
			},
		)

		if err != nil {
			return processed, err
		}

		if expired {
			processed++
		}
	}

	return processed, nil
}
