package orders

import "context"

// settleOrRefundTerminalLeg performs the Wallet action after the lifecycle
// transaction has committed. If the process dies here, RecoverPayments finds
// the same HELD leg and retries the deterministic authority operation.
func (s *Service) settleOrRefundTerminalLeg(ctx context.Context, vendor VendorOrder) error {
	if s == nil || s.payments == nil {
		return ErrDependencyMissing
	}
	leg, operation, err := s.store.PaymentLegForVendorOrder(ctx, vendor.CustomerID, vendor.ID)
	if err != nil {
		return err
	}
	if vendor.Status == VendorStatusCancelled || vendor.Status == VendorStatusRejected {
		if leg.Status == LegRefunded || leg.Status == LegSettled {
			return nil
		}
		adjustment, err := s.store.EnsureCancellationAdjustment(ctx, operationParent(operation), vendor, leg, "cancelled vendor allocation", "commerce-cancel-refund-"+vendor.ID, s.now().UTC())
		if err != nil {
			return err
		}
		if adjustment.WalletStatus == WalletRefunded {
			return nil
		}
		if err = s.store.MarkAdjustmentRequested(ctx, adjustment.ID, s.now().UTC()); err != nil {
			return err
		}
		_ = s.store.SetRefundState(ctx, vendor.ID, "REFUND_PENDING", "", 0, s.now().UTC())
		result, err := s.payments.RefundLeg(ctx, RefundLegRequest{
			IdempotencyKey:   "commerce-cancel-refund-" + vendor.ID,
			PaymentReference: leg.IntentReference, SourceService: PaymentSourceService,
			SourceReference: refundSourceReference(leg, operation), AmountKobo: leg.AmountMinor,
			Currency: leg.Currency, Reason: "vendor allocation cancelled",
		})
		if err != nil {
			return err
		}
		if !result.Refunded() || result.AmountKobo < 0 || result.AmountKobo > leg.AmountMinor {
			return ErrPaymentAuthority
		}
		if _, err = s.store.MarkAdjustmentWallet(ctx, adjustment.ID, result.Reference, WalletRefunded, s.now().UTC()); err != nil {
			return err
		}
		refundedAmount := result.AmountKobo
		if refundedAmount == 0 {
			refundedAmount = leg.AmountMinor
		}
		refundState := "REFUNDED"
		if refundedAmount < leg.AmountMinor {
			refundState = "PARTIALLY_REFUNDED"
		}
		_ = s.store.SetRefundState(ctx, vendor.ID, refundState, result.Reference, refundedAmount, s.now().UTC())
		_, err = s.store.RecordLegRefund(ctx, leg.ID, refundedAmount, s.now().UTC())
		return err
	}
	if vendor.Status == VendorStatusCompleted {
		if leg.Status == LegSettled || leg.Status == LegRefunded {
			return nil
		}
		authority, ok := s.payments.(PaymentSettlementAuthority)
		if !ok {
			return ErrDependencyMissing
		}
		result, err := authority.CompleteLeg(ctx, PaymentSourceService, vendor.ID)
		if err != nil {
			return err
		}
		if !result.Settled() || (result.AmountKobo != 0 && result.AmountKobo != leg.AmountMinor) {
			return ErrPaymentAuthority
		}
		settledAmount := result.AmountKobo
		if settledAmount == 0 {
			settledAmount = leg.AmountMinor
		}
		_, err = s.store.RecordLegSettled(ctx, leg.ID, settledAmount, s.now().UTC())
		return err
	}
	return nil
}

func operationParent(operation PaymentOperation) ParentOrder {
	return ParentOrder{ID: operation.ParentOrderID, CustomerID: operation.CustomerID}
}

func (s *Service) recoverTerminalPaymentLegs(ctx context.Context, limit int) (int, error) {
	legs, err := s.store.ListTerminalPaymentLegs(ctx, limit)
	if err != nil {
		return 0, err
	}
	var count int
	for _, item := range legs {
		vendor, err := s.store.GetVendorOrderForCustomer(ctx, item.CustomerID, item.VendorOrderID)
		if err != nil {
			continue
		}
		if err = s.settleOrRefundTerminalLeg(ctx, vendor); err == nil {
			count++
		}
	}
	return count, nil
}
