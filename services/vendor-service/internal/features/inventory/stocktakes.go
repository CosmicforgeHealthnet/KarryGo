package inventory

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

func normalizeItemIDs(
	values []string,
) (
	[]string,
	error,
) {
	seen :=
		map[string]bool{}

	var out []string

	for _, value := range values {

		value =
			strings.TrimSpace(
				value,
			)

		if value == "" {
			return nil,
				ErrInvalidSource
		}

		if seen[value] {
			continue
		}

		seen[value] = true

		out =
			append(
				out,
				value,
			)
	}

	sort.Strings(out)

	return out, nil
}

func currentInventoryRevisionQ(
	ctx context.Context,
	q interface {
		QueryRow(
			context.Context,
			string,
			...any,
		) pgx.Row
	},
	businessID string,
	branchID string,
) (
	int64,
	error,
) {
	var revision int64

	err :=
		q.QueryRow(
			ctx,
			`
            SELECT COALESCE(
                (
                    SELECT revision
                      FROM inventory_revisions
                     WHERE business_id=$1
                       AND branch_id=$2
                ),
                0
            )
            `,
			businessID,
			branchID,
		).Scan(
			&revision,
		)

	return revision, err
}

func (s *StockService) CreateStocktake(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input CreateStocktakeInput,
) (
	Stocktake,
	error,
) {
	input.BranchID =
		strings.TrimSpace(
			input.BranchID,
		)

	input.Type =
		strings.ToUpper(
			strings.TrimSpace(
				input.Type,
			),
		)

	if input.Type !=
		StocktakeFull &&
		input.Type !=
			StocktakeCycle {

		return Stocktake{},
			ErrInvalidSource
	}

	itemIDs, err :=
		normalizeItemIDs(
			input.InventoryItemIDs,
		)

	if err != nil {
		return Stocktake{}, err
	}

	if input.Type ==
		StocktakeCycle &&
		len(itemIDs) == 0 {

		return Stocktake{},
			ErrInvalidSource
	}

	if len(itemIDs) > 500 {
		return Stocktake{},
			ErrInvalidSource
	}

	var out Stocktake

	err = s.withWriteTransaction(
		ctx,
		actor,
		scope,
		input.BranchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			revision, err :=
				currentInventoryRevisionQ(
					ctx,
					tx,
					actor.BusinessID,
					input.BranchID,
				)

			if err != nil {
				return err
			}

			out = Stocktake{
				ID: stockID("ist_"),

				BusinessID: actor.BusinessID,

				BranchID: input.BranchID,

				Type: input.Type,

				SnapshotRevision: revision,

				Status: StocktakeCounting,

				Version: 1,
			}

			err = tx.QueryRow(
				ctx,
				`
                INSERT INTO inventory_stocktakes (
                    id,
                    business_id,
                    branch_id,
                    stocktake_type,
                    snapshot_revision,
                    status
                )
                VALUES (
                    $1,$2,$3,$4,$5,'COUNTING'
                )
                RETURNING started_at
                `,
				out.ID,
				out.BusinessID,
				out.BranchID,
				out.Type,
				out.SnapshotRevision,
			).Scan(
				&out.StartedAt,
			)

			if err != nil {
				return err
			}

			query := `
                SELECT
                    inventory_item_id,
                    on_hand_quantity
                  FROM inventory_balances
                 WHERE business_id=$1
                   AND branch_id=$2
            `

			args :=
				[]any{
					actor.BusinessID,
					input.BranchID,
				}

			if input.Type ==
				StocktakeCycle {

				query += `
                   AND inventory_item_id = ANY($3)
                `

				args =
					append(
						args,
						itemIDs,
					)
			}

			query += `
                 ORDER BY inventory_item_id
            `

			// The expected-quantity snapshot must be read to completion before
			// any line is written. A connection carrying an open result set
			// cannot also run a statement, so writing inside the scan loop
			// fails with "conn busy" and no stocktake can ever be created.
			rows, err :=
				tx.Query(
					ctx,
					query,
					args...,
				)

			if err != nil {
				return err
			}

			for rows.Next() {
				var line StocktakeLine

				line.ID =
					stockID("istl_")

				if err := rows.Scan(
					&line.InventoryItemID,
					&line.ExpectedQuantity,
				); err != nil {
					rows.Close()
					return err
				}

				out.Lines =
					append(
						out.Lines,
						line,
					)
			}

			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}

			rows.Close()

			for _, line := range out.Lines {

				_, err :=
					tx.Exec(
						ctx,
						`
                        INSERT INTO inventory_stocktake_lines (
                            id,
                            business_id,
                            branch_id,
                            stocktake_id,
                            inventory_item_id,
                            expected_quantity
                        )
                        VALUES (
                            $1,$2,$3,$4,$5,$6
                        )
                        `,
						line.ID,
						actor.BusinessID,
						input.BranchID,
						out.ID,
						line.InventoryItemID,
						line.ExpectedQuantity,
					)

				if err != nil {
					return err
				}
			}

			if input.Type ==
				StocktakeCycle &&
				len(out.Lines) !=
					len(itemIDs) {

				return ErrItemNotFound
			}

			exactScope, _ :=
				ExactBranchScope(
					actor.BusinessID,
					input.BranchID,
				)

			eventRevision :=
				revision

			if eventRevision == 0 {
				eventRevision = 1
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				exactScope,
				"inventory.stocktake.started",
				"inventory.stocktake.started",
				"inventory_stocktake",
				out.ID,
				"",
				eventRevision,
				map[string]any{
					"stocktake": out,
				},
			)
		},
	)

	return out, err
}

func (s *StockService) StocktakeBranch(
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
          FROM inventory_stocktakes
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
			ErrStocktakeNotFound
	}

	return branchID, err
}

func (s *StockService) CountStocktakeLine(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	stocktakeID string,
	itemID string,
	counted int64,
) error {
	if counted < 0 ||
		counted > maxStockQuantity {
		return ErrInvalidQuantity
	}

	branchID, err :=
		s.StocktakeBranch(
			ctx,
			actor.BusinessID,
			stocktakeID,
		)

	if err != nil {
		return err
	}

	return s.withWriteTransaction(
		ctx,
		actor,
		scope,
		branchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			var status string

			err :=
				tx.QueryRow(
					ctx,
					`
                    SELECT status
                      FROM inventory_stocktakes
                     WHERE id=$1
                       AND business_id=$2
                     FOR UPDATE
                    `,
					stocktakeID,
					actor.BusinessID,
				).Scan(
					&status,
				)

			if err != nil {
				return err
			}

			if status !=
				StocktakeCounting {

				return ErrStocktakeStateConflict
			}

			tag, err :=
				tx.Exec(
					ctx,
					`
                    UPDATE inventory_stocktake_lines
                       SET counted_quantity=$4,
                           variance_quantity=
                               $4-expected_quantity,
                           updated_at=now()
                     WHERE stocktake_id=$1
                       AND business_id=$2
                       AND inventory_item_id=$3
                    `,
					stocktakeID,
					actor.BusinessID,
					itemID,
					counted,
				)

			if err != nil {
				return err
			}

			if tag.RowsAffected() != 1 {
				return ErrItemNotFound
			}

			_ = inventoryTx

			return nil
		},
	)
}

func loadStocktakeTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	id string,
) (
	Stocktake,
	error,
) {
	var out Stocktake

	err := tx.QueryRow(
		ctx,
		`
        SELECT
            id,
            business_id,
            branch_id,
            stocktake_type,
            snapshot_revision,
            status,
            started_at,
            posted_at,
            cancelled_at,
            version
          FROM inventory_stocktakes
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
		&out.Type,
		&out.SnapshotRevision,
		&out.Status,
		&out.StartedAt,
		&out.PostedAt,
		&out.CancelledAt,
		&out.Version,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return Stocktake{},
			ErrStocktakeNotFound
	}

	if err != nil {
		return Stocktake{}, err
	}

	rows, err :=
		tx.Query(
			ctx,
			`
            SELECT
                id,
                inventory_item_id,
                expected_quantity,
                counted_quantity,
                variance_quantity
              FROM inventory_stocktake_lines
             WHERE stocktake_id=$1
               AND business_id=$2
             ORDER BY inventory_item_id
             FOR UPDATE
            `,
			out.ID,
			out.BusinessID,
		)

	if err != nil {
		return Stocktake{}, err
	}

	defer rows.Close()

	for rows.Next() {
		var line StocktakeLine

		if err := rows.Scan(
			&line.ID,
			&line.InventoryItemID,
			&line.ExpectedQuantity,
			&line.CountedQuantity,
			&line.VarianceQuantity,
		); err != nil {
			return Stocktake{}, err
		}

		out.Lines =
			append(
				out.Lines,
				line,
			)
	}

	return out, rows.Err()
}

func (s *StockService) PostStocktake(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	id string,
) (
	Stocktake,
	error,
) {
	branchID, err :=
		s.StocktakeBranch(
			ctx,
			actor.BusinessID,
			id,
		)

	if err != nil {
		return Stocktake{}, err
	}

	var (
		out   Stocktake
		stale bool
	)

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
				loadStocktakeTx(
					ctx,
					tx,
					actor.BusinessID,
					id,
				)

			if err != nil {
				return err
			}

			if current.Status ==
				StocktakePosted {

				out = current
				return nil
			}

			if current.Status !=
				StocktakeCounting {

				return ErrStocktakeStateConflict
			}

			itemIDs :=
				make(
					[]string,
					0,
					len(current.Lines),
				)

			for _, line := range current.Lines {

				if line.CountedQuantity ==
					nil {

					return ErrStocktakeCountIncomplete
				}

				itemIDs =
					append(
						itemIDs,
						line.InventoryItemID,
					)
			}

			states, err :=
				lockItemsDeterministicallyTx(
					ctx,
					tx,
					actor.BusinessID,
					current.BranchID,
					itemIDs,
				)

			if err != nil &&
				len(itemIDs) > 0 {

				return err
			}

			var currentRevision int64

			err =
				tx.QueryRow(
					ctx,
					`
                    SELECT revision
                      FROM inventory_revisions
                     WHERE business_id=$1
                       AND branch_id=$2
                     FOR UPDATE
                    `,
					actor.BusinessID,
					current.BranchID,
				).Scan(
					&currentRevision,
				)

			if errors.Is(
				err,
				pgx.ErrNoRows,
			) {
				currentRevision = 0
				err = nil
			}

			if err != nil {
				return err
			}

			exactScope, _ :=
				ExactBranchScope(
					actor.BusinessID,
					current.BranchID,
				)

			if currentRevision !=
				current.SnapshotRevision {

				stale = true

				current.Status =
					StocktakeStale

				current.Version++

				_, err :=
					tx.Exec(
						ctx,
						`
                        UPDATE inventory_stocktakes
                           SET status='STALE',
                               updated_at=now(),
                               version=version+1
                         WHERE id=$1
                           AND business_id=$2
                        `,
						current.ID,
						current.BusinessID,
					)

				if err != nil {
					return err
				}

				out = current

				eventRevision :=
					currentRevision

				if eventRevision == 0 {
					eventRevision = 1
				}

				return s.recordMutation(
					ctx,
					inventoryTx,
					actor,
					exactScope,
					"inventory.stocktake.stale",
					"inventory.stocktake.stale",
					"inventory_stocktake",
					current.ID,
					"INVENTORY_REVISION_CHANGED",
					eventRevision,
					map[string]any{
						"snapshot_revision": current.SnapshotRevision,

						"current_revision": currentRevision,
					},
				)
			}

			lastRevision :=
				currentRevision

			for _, line := range current.Lines {

				counted :=
					*line.CountedQuantity

				delta :=
					counted -
						line.ExpectedQuantity

				if delta == 0 {
					continue
				}

				item :=
					states[line.InventoryItemID]

				if delta > 0 {
					lotID, err :=
						createLotTx(
							ctx,
							tx,
							item,
							delta,
							"STOCKTAKE",
							current.ID,
							LotMetadata{},
						)

					if err != nil {
						return err
					}

					balanceRevision, err :=
						updateBalanceTx(
							ctx,
							tx,
							item,
							delta,
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

					lastRevision =
						inventoryRevision

					_, err =
						appendMovementTx(
							ctx,
							tx,
							actor,
							item,
							lotID,
							MovementStocktakeCorrection,
							delta,
							delta,
							0,
							0,
							0,
							0,
							0,
							"STOCKTAKE",
							current.ID,
							"STOCKTAKE_VARIANCE",
							inventoryRevision,
							balanceRevision,
						)

					if err != nil {
						return err
					}
				} else {
					remove :=
						-delta

					allocations, err :=
						allocateAvailableLotsTx(
							ctx,
							tx,
							item,
							remove,
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
                                   SET on_hand_quantity=
                                       on_hand_quantity-$3,
                                       status=
                                       CASE
                                           WHEN on_hand_quantity-$3=0
                                           THEN 'DEPLETED'
                                           ELSE status
                                       END,
                                       updated_at=now(),
                                       version=version+1
                                 WHERE id=$1
                                   AND business_id=$2
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
					}

					balanceRevision, err :=
						updateBalanceTx(
							ctx,
							tx,
							item,
							-remove,
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

					lastRevision =
						inventoryRevision

					for _, allocation := range allocations {

						_, err :=
							appendMovementTx(
								ctx,
								tx,
								actor,
								item,
								allocation.ID,
								MovementStocktakeCorrection,
								allocation.Quantity,
								-allocation.Quantity,
								0,
								0,
								0,
								0,
								0,
								"STOCKTAKE",
								current.ID,
								"STOCKTAKE_VARIANCE",
								inventoryRevision,
								balanceRevision,
							)

						if err != nil {
							return err
						}
					}
				}
			}

			current.Status =
				StocktakePosted

			current.Version++

			now :=
				s.foundation.Clock().Now()

			current.PostedAt =
				&now

			err =
				tx.QueryRow(
					ctx,
					`
                    UPDATE inventory_stocktakes
                       SET status='POSTED',
                           posted_at=$3,
                           updated_at=now(),
                           version=version+1
                     WHERE id=$1
                       AND business_id=$2
                    RETURNING version
                    `,
					current.ID,
					current.BusinessID,
					now,
				).Scan(
					&current.Version,
				)

			if err != nil {
				return err
			}

			out = current

			if lastRevision == 0 {
				lastRevision = 1
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				exactScope,
				"inventory.stocktake.posted",
				"inventory.stocktake.posted",
				"inventory_stocktake",
				current.ID,
				"",
				lastRevision,
				map[string]any{
					"stocktake": current,
				},
			)
		},
	)

	if err != nil {
		return out, err
	}

	if stale {
		return out,
			ErrStocktakeStale
	}

	return out, nil
}

func (s *StockService) CancelStocktake(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	id string,
) (
	Stocktake,
	error,
) {
	branchID, err :=
		s.StocktakeBranch(
			ctx,
			actor.BusinessID,
			id,
		)

	if err != nil {
		return Stocktake{}, err
	}

	var out Stocktake

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
				loadStocktakeTx(
					ctx,
					tx,
					actor.BusinessID,
					id,
				)

			if err != nil {
				return err
			}

			if current.Status ==
				StocktakeCancelled {

				out = current
				return nil
			}

			if current.Status !=
				StocktakeCounting {

				return ErrStocktakeStateConflict
			}

			now :=
				s.foundation.Clock().Now()

			current.Status =
				StocktakeCancelled

			current.CancelledAt =
				&now

			err = tx.QueryRow(
				ctx,
				`
                UPDATE inventory_stocktakes
                   SET status='CANCELLED',
                       cancelled_at=$3,
                       updated_at=now(),
                       version=version+1
                 WHERE id=$1
                   AND business_id=$2
                RETURNING version
                `,
				current.ID,
				current.BusinessID,
				now,
			).Scan(
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

			revision, _ :=
				currentInventoryRevisionQ(
					ctx,
					tx,
					actor.BusinessID,
					current.BranchID,
				)

			if revision == 0 {
				revision = 1
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				exactScope,
				"inventory.stocktake.cancelled",
				"inventory.stocktake.cancelled",
				"inventory_stocktake",
				current.ID,
				"",
				revision,
				map[string]any{
					"stocktake": current,
				},
			)
		},
	)

	return out, err
}
