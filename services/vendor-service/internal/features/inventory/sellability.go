package inventory

import (
	"context"
)

type Sellability struct {
	InventoryItemID string `json:"inventory_item_id"`
	BranchID        string `json:"branch_id"`

	RequestedQuantity int64 `json:"requested_quantity"`

	OnHandQuantity      int64 `json:"on_hand"`
	ReservedQuantity    int64 `json:"reserved"`
	QuarantinedQuantity int64 `json:"quarantined"`
	AvailableQuantity   int64 `json:"available"`

	Sellable bool   `json:"sellable"`
	Reason   string `json:"reason"`
}

func (s *StockService) AuthoritativeSellability(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	inventoryItemID string,
	quantity int64,
) (
	Sellability,
	error,
) {
	if quantity <= 0 ||
		quantity > maxStockQuantity {
		return Sellability{},
			ErrInvalidQuantity
	}

	balance, err :=
		s.Availability(
			ctx,
			actor,
			scope,
			inventoryItemID,
		)

	if err != nil {
		return Sellability{}, err
	}

	out :=
		Sellability{
			InventoryItemID: inventoryItemID,

			BranchID: balance.BranchID,

			RequestedQuantity: quantity,

			OnHandQuantity: balance.OnHandQuantity,

			ReservedQuantity: balance.ReservedQuantity,

			QuarantinedQuantity: balance.QuarantinedQuantity,

			AvailableQuantity: balance.AvailableQuantity,
		}

	switch {
	case balance.AvailableQuantity <= 0:
		out.Reason = "OUT_OF_STOCK"

	case balance.AvailableQuantity <
		quantity:

		out.Reason =
			"INSUFFICIENT_STOCK"

	default:
		out.Sellable = true
		out.Reason = "AVAILABLE"
	}

	return out, nil
}
