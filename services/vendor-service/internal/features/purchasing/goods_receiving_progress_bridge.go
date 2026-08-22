package purchasing

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"
)

func (s *GoodsReceivingService) applyPOReceivingProgressTx(
	ctx context.Context,
	tx pgx.Tx,
	actor GoodsReceivingActor,
	purchaseOrderID string,
	progress map[string]int64,
) error {
	lines := make([]ReceivingProgressLine, 0, len(progress))

	for lineID, quantity := range progress {
		lines = append(
			lines,
			ReceivingProgressLine{
				PurchaseOrderLineID:      lineID,
				ReceivedPurchaseQuantity: quantity,
			},
		)
	}

	sort.Slice(
		lines,
		func(i, j int) bool {
			return lines[i].PurchaseOrderLineID <
				lines[j].PurchaseOrderLineID
		},
	)

	poActor := Actor{
		StaffID:       actor.StaffID,
		VendorID:      actor.BusinessID,
		SessionID:     actor.SessionID,
		CorrelationID: actor.CorrelationID,
		CausationID:   actor.CausationID,
		CommandRef:    actor.CommandRef,
	}

	_, err := s.po.ApplyReceivingProgressTx(
		ctx,
		tx,
		poActor,
		purchaseOrderID,
		lines,
	)
	return err
}
