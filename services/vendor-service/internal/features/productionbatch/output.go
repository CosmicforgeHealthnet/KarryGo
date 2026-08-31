package productionbatch

import "cosmicforge/logistics/services/vendor-service/internal/features/inventory"

// preparedOutputInput is the single BATCH_STOCK output boundary. It derives
// saleable prepared quantity from accepted actual output and the frozen policy
// factor; ON_DEMAND deliberately returns no stock posting.
func preparedOutputInput(operatingModel string, actualOutput, outputFactor fraction, inventoryItemID string) (*inventory.ProductionOutputInput, error) {
	if operatingModel != OperatingBatchStock {
		return nil, nil
	}
	if inventoryItemID == "" {
		return nil, ErrInventoryRequired
	}
	quantity, err := multiplyFraction(actualOutput, outputFactor)
	if err != nil {
		return nil, err
	}
	quantityInt, err := toExactInt64(quantity)
	if err != nil || quantityInt <= 0 {
		return nil, ErrInventoryRequired
	}
	return &inventory.ProductionOutputInput{InventoryItemID: inventoryItemID, Quantity: quantityInt}, nil
}
