package inventory

import (
	"context"
	"time"
)

type alertEventCandidate struct {
	ID              string
	BusinessID      string
	BranchID        string
	InventoryItemID string
	AlertType       string
}

func (s *StockService) GenerateInventoryAlerts(
	ctx context.Context,
	limit int,
) (
	int,
	error,
) {
	count,
		err :=
		s.generateInventoryAlertsPrimary(
			ctx,
			limit,
		)

	if err != nil {
		return count, err
	}

	if s == nil ||
		s.transactions == nil ||
		s.events == nil {

		return count, nil
	}

	if err :=
		s.emitLowStockEvents(
			ctx,
			limit,
		); err != nil {

		return count, err
	}

	return count, nil
}

func (s *StockService) emitLowStockEvents(
	ctx context.Context,
	limit int,
) error {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows,
		err :=
		s.db.Query(
			ctx,
			`SELECT
                id,
                business_id,
                branch_id,
                inventory_item_id,
                alert_type
             FROM inventory_alerts
             WHERE status = 'OPEN'
               AND alert_type = 'LOW_STOCK'
             ORDER BY updated_at, id
             LIMIT $1`,
			limit,
		)

	if err != nil {
		return err
	}

	candidates :=
		make(
			[]alertEventCandidate,
			0,
			limit,
		)

	for rows.Next() {
		var item alertEventCandidate

		if err =
			rows.Scan(
				&item.ID,
				&item.BusinessID,
				&item.BranchID,
				&item.InventoryItemID,
				&item.AlertType,
			); err != nil {

			rows.Close()
			return err
		}

		candidates =
			append(
				candidates,
				item,
			)
	}

	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}

	rows.Close()

	for _, item := range candidates {
		item := item

		err =
			s.transactions.
				WithinInventoryTransaction(
					ctx,
					func(
						inventoryTx InventoryTransaction,
					) error {
						postgresTx,
							ok :=
							inventoryTx.(PostgresTransactionAccessor)

						if !ok {
							return ErrDependencyMissing
						}

						tag,
							err :=
							postgresTx.
								PGX().
								Exec(
									ctx,
									`INSERT INTO inventory_alert_event_emissions (
                                        business_id,
                                        alert_id,
                                        event_type
                                    )
                                    VALUES ($1,$2,$3)
                                    ON CONFLICT DO NOTHING`,
									item.BusinessID,
									item.ID,
									EventInventoryLowStock,
								)

						if err != nil {
							return err
						}

						if tag.RowsAffected() == 0 {
							return nil
						}

						branchID :=
							item.BranchID

						return s.events.
							PublishInventoryEvent(
								ctx,
								inventoryTx,
								Event{
									Type: EventInventoryLowStock,

									BusinessID: item.BusinessID,

									BranchID: &branchID,

									AggregateType: "inventory_alert",

									AggregateRef: item.ID,

									OccurredAt: time.Now().UTC(),

									Payload: map[string]any{
										"alert_id": item.ID,

										"inventory_item_id": item.InventoryItemID,

										"alert_type": item.AlertType,
									},
								},
							)
					},
				)

		if err != nil {
			return err
		}
	}

	return nil
}
