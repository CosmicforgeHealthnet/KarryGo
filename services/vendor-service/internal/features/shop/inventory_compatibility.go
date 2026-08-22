package shop

import (
	"context"

	genericinventory "cosmicforge/logistics/services/vendor-service/internal/features/inventory"

	"github.com/jackc/pgx/v5"
)

// ensureGenericInventoryCompatibilityTx synchronises one certified Shop stock
// basis into the generic Inventory identity layer.
//
// Existing Shop stock-basis ID is preserved exactly.
func (s *Service) ensureGenericInventoryCompatibilityTx(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	basis InventoryStockBasis,
	measureOwner string,
	measureFamily string,
) error {
	_, _, err :=
		genericinventory.UpsertShopStockBasisCompatibilityTx(
			ctx,
			tx,
			genericinventory.ShopStockBasisCompatibilityInput{
				StockBasisID: basis.ID,

				BusinessID: actor.VendorID,
				BranchID:   basis.BranchRef,
				VariantID:  basis.VariantRef,

				MeasureRef:      basis.MeasureRef,
				MeasureOwnerKey: measureOwner,
				MeasureFamily:   measureFamily,

				ConsumptionMode: basis.ConsumptionMode,

				LotPolicy: basis.AllocationPolicy,

				LotTrackingEnabled: basis.LotTrackingEnabled,

				Status: basis.Status,

				Version: basis.Version,

				CreatedAt: basis.CreatedAt,

				UpdatedAt: basis.UpdatedAt,
			},
		)

	return err
}
