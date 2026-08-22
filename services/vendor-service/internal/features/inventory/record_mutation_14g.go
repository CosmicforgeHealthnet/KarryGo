package inventory

import "context"

func (s *StockService) recordMutation(
	ctx context.Context,
	inventoryTx InventoryTransaction,
	actor Actor,
	scope BranchScope,
	action string,
	eventType string,
	aggregateType string,
	aggregateRef string,
	reason string,
	revision int64,
	metadata map[string]any,
) error {
	eventType =
		canonicalInventoryEventType(
			action,
			eventType,
		)

	if err :=
		s.recordMutationPrimary(
			ctx,
			inventoryTx,
			actor,
			scope,
			action,
			eventType,
			aggregateType,
			aggregateRef,
			reason,
			revision,
			metadata,
		); err != nil {

		return err
	}

	return s.publishAvailabilitySignals(
		ctx,
		inventoryTx,
		actor,
		action,
		aggregateType,
		aggregateRef,
		scope,
		metadata,
	)
}
