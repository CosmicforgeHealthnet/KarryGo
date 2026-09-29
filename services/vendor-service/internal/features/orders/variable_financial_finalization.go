package orders

import (
	"context"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"
)

// FinalizeVariableMeasurement executes the Wallet-supported CAP_AND_REFUND
// strategy for one measured line. The payment leg is authorized to the frozen
// line maximum at payment start; finalization refunds only the unused portion,
// then reconciles the mutable VendorOrder settlement aggregate to the actual.
func (s *Service) FinalizeVariableMeasurement(ctx context.Context, actor Actor, vendorOrderID, lineID string, expectedVersion int64) (VariableMeasurement, error) {
	if !s.enabled {
		return VariableMeasurement{}, ErrDisabled
	}
	scope, err := s.authorize(ctx, actor, PermOrdersManage)
	if err != nil {
		return VariableMeasurement{}, err
	}
	order, err := s.store.GetVendorOrderForVendor(ctx, actor.VendorID, vendorOrderID)
	if err != nil || !scope.Covers(order.BranchID) {
		return VariableMeasurement{}, ErrOrderNotFound
	}
	var m VariableMeasurement
	var parentID, operationRef, capability string
	var lineIDRead string
	var lineQuoted int64
	tx, err := s.store.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return VariableMeasurement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = tx.QueryRow(ctx, `SELECT vm.measurement_mode,vm.estimated_quantity,vm.unit_price_minor,vm.estimated_amount_minor,vm.tolerance_quantity,vm.hard_max_quantity,vm.reserved_upper_quantity,vm.actual_quantity,vm.final_amount_minor,vm.financial_state,v.parent_order_id,l.id,l.quoted_total_minor FROM commerce_variable_measurements vm JOIN commerce_vendor_orders v ON v.id=vm.vendor_order_id JOIN commerce_vendor_order_lines l ON l.id=vm.order_line_id WHERE vm.vendor_order_id=$1 AND vm.order_line_id=$2 AND vm.version=$3 FOR UPDATE`, vendorOrderID, lineID, expectedVersion).Scan(&m.Mode, &m.EstimatedQuantity, &m.UnitPriceMinor, &m.EstimatedAmountMinor, &m.ToleranceQuantity, &m.HardMaxQuantity, &m.ReservedUpperQuantity, &m.ActualQuantity, &m.FinalAmountMinor, &m.State, &parentID, &lineIDRead, &lineQuoted); errors.Is(err, pgx.ErrNoRows) {
		return VariableMeasurement{}, ErrVersionConflict
	} else if err != nil {
		return VariableMeasurement{}, err
	}
	if m.State == FinancialFinalized {
		return m, nil
	}
	if m.State == FinancialReapprovalRequired {
		return m, ErrCommercialChangeRequired
	}
	if m.State != FinancialActualRecorded && m.State != FinancialPending {
		return m, ErrOrderConflict
	}
	if err = tx.QueryRow(ctx, `SELECT payment_operation_ref FROM commerce_parent_orders WHERE id=$1 FOR UPDATE`, parentID).Scan(&operationRef); err != nil {
		return VariableMeasurement{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT capability FROM commerce_payment_capability_snapshots WHERE parent_order_id=$1 AND payment_operation_ref=$2`, parentID, operationRef).Scan(&capability); err != nil {
		return VariableMeasurement{}, ErrUnknownPaymentCapability
	}
	parsed, err := ParsePaymentCapability(capability)
	if err != nil || parsed != PaymentCapAndRefund {
		return VariableMeasurement{}, ErrUnknownPaymentCapability
	}

	// Locking the parent before evaluating the aggregate makes two otherwise
	// individually-valid variable lines serialize against the same cap.
	var parentMax, platformFeeTax, deliveryCustomer int64
	if err = tx.QueryRow(ctx, `SELECT max_authorized_total_minor,platform_fee_tax_minor FROM commerce_parent_orders WHERE id=$1`, parentID).Scan(&parentMax, &platformFeeTax); err != nil {
		return VariableMeasurement{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(delivery_customer_minor),0) FROM commerce_vendor_orders WHERE parent_order_id=$1`, parentID).Scan(&deliveryCustomer); err != nil {
		return VariableMeasurement{}, err
	}
	rows, err := tx.Query(ctx, `SELECT l.id,l.pricing_basis,l.quoted_total_minor,COALESCE(vm.estimated_amount_minor,0),COALESCE(vm.final_amount_minor,0),COALESCE(vm.financial_state,'') FROM commerce_vendor_order_lines l LEFT JOIN commerce_variable_measurements vm ON vm.order_line_id=l.id WHERE l.parent_order_id=$1 ORDER BY l.id`, parentID)
	if err != nil {
		return VariableMeasurement{}, err
	}
	var aggregate int64
	for rows.Next() {
		var id, basis, state string
		var quoted, estimated, final int64
		if err = rows.Scan(&id, &basis, &quoted, &estimated, &final, &state); err != nil {
			rows.Close()
			return VariableMeasurement{}, err
		}
		amount := quoted
		if basis == "VARIABLE_MEASURE" {
			amount = estimated
			if id == lineIDRead {
				amount = m.FinalAmountMinor
			} else if final > 0 && (state == string(FinancialActualRecorded) || state == string(FinancialPending) || state == string(FinancialFinalized)) {
				amount = final
			}
		}
		if amount < 0 || aggregate > math.MaxInt64-amount {
			rows.Close()
			return VariableMeasurement{}, ErrAllocationImbalance
		}
		aggregate += amount
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return VariableMeasurement{}, err
	}
	rows.Close()
	if aggregate > math.MaxInt64-deliveryCustomer-platformFeeTax || aggregate+deliveryCustomer+platformFeeTax > parentMax {
		if _, err = tx.Exec(ctx, `UPDATE commerce_variable_measurements SET financial_state='REAPPROVAL_REQUIRED',version=version+1,updated_at=$1 WHERE vendor_order_id=$2 AND order_line_id=$3 AND version=$4`, s.now().UTC(), vendorOrderID, lineID, expectedVersion); err != nil {
			return VariableMeasurement{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return VariableMeasurement{}, err
		}
		m.State = FinancialReapprovalRequired
		return m, ErrCommercialChangeRequired
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_variable_measurements SET financial_state='FINANCIAL_PENDING',wallet_capability_snapshot=$1,version=version+1,updated_at=$2 WHERE vendor_order_id=$3 AND order_line_id=$4 AND version=$5`, string(parsed), s.now().UTC(), vendorOrderID, lineID, expectedVersion); err != nil {
		return VariableMeasurement{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return VariableMeasurement{}, err
	}

	leg, _, err := s.store.PaymentLegForVendorOrder(ctx, order.CustomerID, vendorOrderID)
	if err != nil {
		return VariableMeasurement{}, err
	}
	lineAuthorized, err := CalculateMeasuredAmount(m.UnitPriceMinor, m.HardMaxQuantity)
	var pricingBasis string
	var frozenLineCap int64
	if readErr := s.store.db.QueryRow(ctx, `SELECT pricing_basis,max_authorized_total_minor FROM commerce_vendor_order_lines WHERE id=$1 AND vendor_order_id=$2`, lineID, vendorOrderID).Scan(&pricingBasis, &frozenLineCap); readErr != nil {
		return VariableMeasurement{}, readErr
	}
	if pricingBasis == "VARIABLE_MEASURE" {
		lineAuthorized, err = frozenLineCap, nil
	}
	if err != nil {
		return VariableMeasurement{}, err
	}
	difference := lineAuthorized - m.FinalAmountMinor
	if difference < 0 {
		difference = 0
	}
	if difference > 0 {
		if leg.IntentReference == "" {
			return s.markVariableFinancialState(ctx, vendorOrderID, lineID, FinancialFailedRecoverable, m)
		}
		key := "variable-measurement-refund-" + vendorOrderID + "-" + lineID
		if _, ok := s.payments.(CommerceSettlementAuthority); ok {
			authorizationRelease := lineAuthorized - lineQuoted
			if authorizationRelease > difference {
				authorizationRelease = difference
			}
			decision, policyErr := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerVariableFinal, Order: order, OrderLineID: lineID, DifferenceMinor: difference, AuthorizationReleaseMinor: authorizationRelease})
			if policyErr != nil {
				return s.markVariableFinancialState(ctx, vendorOrderID, lineID, FinancialFailedRecoverable, m)
			}
			record, _, policyErr := s.store.EnsureRefundPolicyDecision(ctx, parentID, vendorOrderID, "", key, decision, s.now().UTC())
			if policyErr != nil {
				return s.markVariableFinancialState(ctx, vendorOrderID, lineID, FinancialFailedRecoverable, m)
			}
			result, walletErr := s.executeCommerceRefund(ctx, record, false)
			if walletErr != nil || !result.Applied() || result.AmountMinor != difference {
				return s.markVariableFinancialState(ctx, vendorOrderID, lineID, FinancialFailedRecoverable, m)
			}
		} else {
			result, walletErr := s.payments.RefundLeg(ctx, RefundLegRequest{IdempotencyKey: key, PaymentReference: leg.IntentReference, SourceService: PaymentSourceService, SourceReference: vendorOrderID, AmountKobo: difference, Currency: order.Currency, Reason: "variable measurement finalization"})
			if walletErr != nil || !result.Refunded() || (result.AmountKobo != 0 && result.AmountKobo != difference) {
				return s.markVariableFinancialState(ctx, vendorOrderID, lineID, FinancialFailedRecoverable, m)
			}
		}
	}

	if err = s.completeVariableFinancialProjection(ctx, order, lineID, m.FinalAmountMinor, lineQuoted, actor.StaffID); err != nil {
		return s.markVariableFinancialState(ctx, vendorOrderID, lineID, FinancialFailedRecoverable, m)
	}
	m.State = FinancialFinalized
	return m, nil
}

func (s *Service) completeVariableFinancialProjection(ctx context.Context, order VendorOrder, lineID string, finalAmount, lineQuoted int64, actorRef string) error {
	if finalAmount < 0 || lineQuoted < 0 {
		return ErrAllocationImbalance
	}
	delta := finalAmount - lineQuoted
	tx, err := s.store.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var state string
	if err = tx.QueryRow(ctx, `SELECT financial_state FROM commerce_variable_measurements WHERE vendor_order_id=$1 AND order_line_id=$2 FOR UPDATE`, order.ID, lineID).Scan(&state); err != nil {
		return err
	}
	if state == string(FinancialFinalized) {
		return tx.Commit(ctx)
	}
	if state != string(FinancialPending) && state != string(FinancialFailedRecoverable) {
		return ErrOrderConflict
	}
	aggregateTag, err := tx.Exec(ctx, `UPDATE commerce_vendor_orders SET product_subtotal_minor=product_subtotal_minor+$1,customer_payable_minor=customer_payable_minor+$1,vendor_payable_minor=vendor_payable_minor+$1,version=version+1,updated_at=$2 WHERE id=$3 AND parent_order_id=$4 AND product_subtotal_minor+$1>=0 AND max_product_subtotal_minor>=product_subtotal_minor+$1 AND customer_payable_minor+$1>=0 AND vendor_payable_minor+$1>=0`, delta, s.now().UTC(), order.ID, order.ParentOrderID)
	if err != nil {
		return err
	}
	if aggregateTag.RowsAffected() != 1 {
		return ErrAllocationImbalance
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_variable_measurements SET financial_state='FINALIZED',version=version+1,updated_at=$1 WHERE vendor_order_id=$2 AND order_line_id=$3 AND financial_state IN ('FINANCIAL_PENDING','FAILED_RECOVERABLE')`, s.now().UTC(), order.ID, lineID); err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, s.store.outbox, order.ParentOrderID, order.ID, order.CustomerID, "commerce.vendor_order.variable_measurement_finalized", "SYSTEM", actorRef, state, "FINALIZED", s.now().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) markVariableFinancialState(ctx context.Context, vendorOrderID, lineID string, state FinancialFinalizationState, m VariableMeasurement) (VariableMeasurement, error) {
	_, err := s.store.db.Exec(ctx, `UPDATE commerce_variable_measurements SET financial_state=$1,version=version+1,updated_at=$2 WHERE vendor_order_id=$3 AND order_line_id=$4 AND financial_state='FINANCIAL_PENDING'`, state, s.now().UTC(), vendorOrderID, lineID)
	if err != nil {
		return VariableMeasurement{}, err
	}
	m.State = state
	return m, errors.New("wallet finalization unresolved")
}
