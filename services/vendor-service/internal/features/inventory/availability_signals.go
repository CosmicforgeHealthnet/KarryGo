package inventory

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	EventInventoryBalanceChanged = "vendor.inventory.balance.changed"

	EventInventoryItemOutOfStock = "vendor.inventory.item.out_of_stock"

	EventInventoryItemAvailable = "vendor.inventory.item.available"
)

type availabilitySignalTarget struct {
	BusinessID string
	BranchID   string
	ItemID     string
}

type availabilitySignalState struct {
	AvailableQuantity int64
	Revision          int64
	IsAvailable       bool
}

func actionChangesInventoryAvailability(
	action string,
) bool {
	switch action {
	case "inventory.stock.received",
		"inventory.stock.reserved",
		"inventory.stock.released",
		"inventory.stock.quarantined",
		"inventory.stock.quarantine_released",
		"inventory.stock.wasted",
		"inventory.stock.adjusted",
		"inventory.stock.expired",
		"inventory.receipt.posted",
		"inventory.receipt.cancelled",
		"inventory.stocktake.posted",
		"inventory.transfer.dispatched",
		"inventory.transfer.received",
		"inventory.quarantine.created",
		"inventory.quarantine.resolved",
		"inventory.reservation.HELD",
		"inventory.reservation.COMMITTED",
		"inventory.reservation.RELEASED",
		"inventory.reservation.EXPIRED",
		"inventory.reservation.CANCELLED":

		return true
	}

	return false
}

func appendAvailabilityTarget(
	targets map[string]availabilitySignalTarget,
	target availabilitySignalTarget,
) {
	target.BusinessID =
		strings.TrimSpace(
			target.BusinessID,
		)

	target.BranchID =
		strings.TrimSpace(
			target.BranchID,
		)

	target.ItemID =
		strings.TrimSpace(
			target.ItemID,
		)

	if target.BusinessID == "" ||
		target.BranchID == "" ||
		target.ItemID == "" {

		return
	}

	key :=
		target.BusinessID +
			"\x00" +
			target.BranchID +
			"\x00" +
			target.ItemID

	targets[key] =
		target
}

func availabilitySignalTargets(
	actor Actor,
	scope BranchScope,
	aggregateType string,
	aggregateRef string,
	metadata map[string]any,
) []availabilitySignalTarget {
	targets :=
		map[string]availabilitySignalTarget{}

	branchID := ""

	if scope.BranchID != nil {
		branchID =
			*scope.BranchID
	}

	if aggregateType ==
		"inventory_item" {

		appendAvailabilityTarget(
			targets,
			availabilitySignalTarget{
				BusinessID: actor.BusinessID,

				BranchID: branchID,

				ItemID: aggregateRef,
			},
		)
	}

	if reservation, ok :=
		metadata["reservation"].(Reservation); ok {

		for _, line := range reservation.Lines {

			appendAvailabilityTarget(
				targets,
				availabilitySignalTarget{
					BusinessID: reservation.BusinessID,

					BranchID: reservation.BranchID,

					ItemID: line.InventoryItemID,
				},
			)
		}
	}

	if receipt, ok :=
		metadata["receipt"].(Receipt); ok {

		for _, line := range receipt.Lines {

			appendAvailabilityTarget(
				targets,
				availabilitySignalTarget{
					BusinessID: receipt.BusinessID,

					BranchID: receipt.BranchID,

					ItemID: line.InventoryItemID,
				},
			)
		}
	}

	if quarantine, ok :=
		metadata["quarantine"].(QuarantineCase); ok {

		appendAvailabilityTarget(
			targets,
			availabilitySignalTarget{
				BusinessID: quarantine.BusinessID,

				BranchID: quarantine.BranchID,

				ItemID: quarantine.InventoryItemID,
			},
		)
	}

	if stocktake, ok :=
		metadata["stocktake"].(Stocktake); ok {

		for _, line := range stocktake.Lines {

			appendAvailabilityTarget(
				targets,
				availabilitySignalTarget{
					BusinessID: stocktake.BusinessID,

					BranchID: stocktake.BranchID,

					ItemID: line.InventoryItemID,
				},
			)
		}
	}

	if transfer, ok :=
		metadata["transfer"].(Transfer); ok {

		for _, line := range transfer.Lines {

			appendAvailabilityTarget(
				targets,
				availabilitySignalTarget{
					BusinessID: transfer.BusinessID,

					BranchID: transfer.SourceBranchID,

					ItemID: line.SourceInventoryItemID,
				},
			)

			appendAvailabilityTarget(
				targets,
				availabilitySignalTarget{
					BusinessID: transfer.BusinessID,

					BranchID: transfer.DestinationBranchID,

					ItemID: line.DestinationInventoryItemID,
				},
			)
		}
	}

	keys :=
		make(
			[]string,
			0,
			len(targets),
		)

	for key := range targets {

		keys =
			append(
				keys,
				key,
			)
	}

	sort.Strings(keys)

	out :=
		make(
			[]availabilitySignalTarget,
			0,
			len(keys),
		)

	for _, key := range keys {

		out =
			append(
				out,
				targets[key],
			)
	}

	return out
}

func currentAvailabilitySignalStateTx(
	ctx context.Context,
	tx pgx.Tx,
	target availabilitySignalTarget,
) (
	availabilitySignalState,
	error,
) {
	var state availabilitySignalState

	err :=
		tx.QueryRow(
			ctx,
			`
            SELECT
                available_quantity,
                revision
              FROM inventory_balance_availability
             WHERE business_id=$1
               AND branch_id=$2
               AND inventory_item_id=$3
            `,
			target.BusinessID,
			target.BranchID,
			target.ItemID,
		).Scan(
			&state.AvailableQuantity,
			&state.Revision,
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		// An inventory item with no stock balance has zero authoritative
		// sellable quantity.
		state.AvailableQuantity = 0
		state.Revision = 0
		state.IsAvailable = false

		return state, nil
	}

	if err != nil {
		return availabilitySignalState{},
			err
	}

	state.IsAvailable =
		state.AvailableQuantity > 0

	return state, nil
}

func availabilityTransitionEvent(
	previous *bool,
	current bool,
) string {
	if previous != nil &&
		*previous == current {

		return ""
	}

	if current {
		return EventInventoryItemAvailable
	}

	return EventInventoryItemOutOfStock
}

func (s *StockService) publishAvailabilitySignals(
	ctx context.Context,
	inventoryTx InventoryTransaction,
	actor Actor,
	action string,
	aggregateType string,
	aggregateRef string,
	scope BranchScope,
	metadata map[string]any,
) error {
	if !actionChangesInventoryAvailability(
		action,
	) {
		return nil
	}

	postgresTx, ok :=
		inventoryTx.(PostgresTransactionAccessor)

	if !ok {
		return ErrDependencyMissing
	}

	tx :=
		postgresTx.PGX()

	targets :=
		availabilitySignalTargets(
			actor,
			scope,
			aggregateType,
			aggregateRef,
			metadata,
		)

	for _, target := range targets {

		current, err :=
			currentAvailabilitySignalStateTx(
				ctx,
				tx,
				target,
			)

		if err != nil {
			return err
		}

		var previous *bool

		var previousValue bool

		err =
			tx.QueryRow(
				ctx,
				`
                SELECT is_available
                  FROM inventory_availability_signal_state
                 WHERE business_id=$1
                   AND branch_id=$2
                   AND inventory_item_id=$3
                 FOR UPDATE
                `,
				target.BusinessID,
				target.BranchID,
				target.ItemID,
			).Scan(
				&previousValue,
			)

		switch {
		case err == nil:
			previous =
				&previousValue

		case errors.Is(
			err,
			pgx.ErrNoRows,
		):
			previous = nil

		default:
			return err
		}

		_, err =
			tx.Exec(
				ctx,
				`
                INSERT INTO inventory_availability_signal_state (
                    business_id,
                    branch_id,
                    inventory_item_id,
                    is_available,
                    available_quantity,
                    inventory_revision,
                    updated_at
                )
                VALUES (
                    $1,$2,$3,$4,$5,$6,now()
                )
                ON CONFLICT (
                    business_id,
                    branch_id,
                    inventory_item_id
                )
                DO UPDATE SET
                    is_available=
                        EXCLUDED.is_available,
                    available_quantity=
                        EXCLUDED.available_quantity,
                    inventory_revision=
                        EXCLUDED.inventory_revision,
                    updated_at=now()
                `,
				target.BusinessID,
				target.BranchID,
				target.ItemID,
				current.IsAvailable,
				current.AvailableQuantity,
				current.Revision,
			)

		if err != nil {
			return err
		}

		revision :=
			current.Revision

		if revision <= 0 {
			revision = 1
		}

		branch :=
			target.BranchID

		payload :=
			map[string]any{
				"business_id": target.BusinessID,

				"branch_id": target.BranchID,

				"inventory_item_id": target.ItemID,

				"available_quantity": current.AvailableQuantity,

				"is_available": current.IsAvailable,

				"inventory_revision": current.Revision,

				"projection_invalidation": []string{
					"FOOD",
					"SHOP",
				},
			}

		if err :=
			s.events.
				PublishInventoryEvent(
					ctx,
					inventoryTx,
					Event{
						Type: EventInventoryBalanceChanged,

						BusinessID: target.BusinessID,

						BranchID: &branch,

						AggregateType: "inventory_item",

						AggregateRef: target.ItemID,

						Revision: revision,

						CorrelationID: actor.CorrelationID,

						CausationID: actor.CausationID,

						OccurredAt: s.foundation.Clock().Now(),

						Payload: payload,
					},
				); err != nil {

			return err
		}

		transition :=
			availabilityTransitionEvent(
				previous,
				current.IsAvailable,
			)

		if transition == "" {
			continue
		}

		if err :=
			s.events.
				PublishInventoryEvent(
					ctx,
					inventoryTx,
					Event{
						Type: transition,

						BusinessID: target.BusinessID,

						BranchID: &branch,

						AggregateType: "inventory_item",

						AggregateRef: target.ItemID,

						Revision: revision,

						CorrelationID: actor.CorrelationID,

						CausationID: actor.CausationID,

						OccurredAt: s.foundation.Clock().Now(),

						Payload: payload,
					},
				); err != nil {

			return err
		}
	}

	return nil
}
