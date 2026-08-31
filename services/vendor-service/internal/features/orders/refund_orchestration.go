package orders

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type RefundPolicyRecord struct {
	ID, ParentOrderID, VendorOrderID, MissionRef, IdempotencyKey string
	WalletOperationRef, WalletStatus, SupportCaseRef             string
	ProjectionApplied                                            bool
	Decision                                                     RefundPolicyDecision
	CreatedAt, UpdatedAt                                         time.Time
}

type RefundSupportRequest struct {
	OrganizationReference string
	IdempotencyKey        string
	ReasonCode            string
	CustomerReference     string
	ProviderReference     string
	MissionReference      string
	ParentOrderReference  string
	VendorOrderReference  string
}

type RefundSupportResult struct {
	CaseID, Status string
}

type RefundSupportAuthority interface {
	CreateRefundCase(ctx context.Context, request RefundSupportRequest) (RefundSupportResult, error)
}

func (s *Store) EnsureRefundPolicyDecision(ctx context.Context, parentOrderID, vendorOrderID, missionRef, key string, decision RefundPolicyDecision, now time.Time) (RefundPolicyRecord, bool, error) {
	if s == nil || s.db == nil {
		return RefundPolicyRecord{}, false, ErrDependencyMissing
	}
	if strings.TrimSpace(parentOrderID) == "" || strings.TrimSpace(vendorOrderID) == "" || strings.TrimSpace(key) == "" {
		return RefundPolicyRecord{}, false, ErrOrderConflict
	}
	if err := decision.Validate(); err != nil {
		return RefundPolicyRecord{}, false, err
	}
	tag, err := s.db.Exec(ctx, `INSERT INTO commerce_refund_policy_decisions(
		id,parent_order_id,vendor_order_id,order_line_id,mission_ref,trigger,action,reason_code,policy_version,plan_version,
		automatic,support_required,customer_refund_minor,platform_funding_reversal_minor,other_funding_reversal_minor,
		platform_loss_funding_minor,vendor_merchandise_reduction_minor,vendor_delivery_reduction_minor,
		provider_delivery_reduction_minor,platform_revenue_reduction_minor,authorization_release_minor,idempotency_key,
		wallet_status,projection_applied,created_at,updated_at)
		VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$25)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		"crpd_"+uuid.NewString(), parentOrderID, vendorOrderID, decision.OrderLineID, strings.TrimSpace(missionRef),
		decision.Trigger, decision.Action, decision.ReasonCode, decision.PolicyVersion, decision.PlanVersion,
		decision.Automatic, decision.SupportRequired, decision.CustomerRefundMinor, decision.PlatformFundingReversalMinor,
		decision.OtherFundingReversalMinor, decision.PlatformLossFundingMinor, decision.VendorMerchandiseReductionMinor,
		decision.VendorDeliveryReductionMinor, decision.ProviderDeliveryReductionMinor, decision.PlatformRevenueReductionMinor,
		decision.AuthorizationReleaseMinor, key, initialRefundPolicyStatus(decision), !decision.Automatic, now)
	if err != nil {
		return RefundPolicyRecord{}, false, err
	}
	record, err := s.GetRefundPolicyDecision(ctx, key)
	if err != nil {
		return RefundPolicyRecord{}, false, err
	}
	// An idempotency key is also an immutable economic-command identity. A
	// replay with different evidence is a conflict, never a rewrite.
	if record.ParentOrderID != parentOrderID || record.VendorOrderID != vendorOrderID || record.MissionRef != strings.TrimSpace(missionRef) || record.Decision != decision {
		return RefundPolicyRecord{}, false, ErrOrderConflict
	}
	return record, tag.RowsAffected() == 1, nil
}

func initialRefundPolicyStatus(decision RefundPolicyDecision) string {
	if decision.SupportRequired {
		return "SUPPORT_REQUIRED"
	}
	if !decision.Automatic || decision.Action == RefundActionNone {
		return "APPLIED"
	}
	return "PENDING"
}

func (s *Store) GetRefundPolicyDecision(ctx context.Context, key string) (RefundPolicyRecord, error) {
	if s == nil || s.db == nil {
		return RefundPolicyRecord{}, ErrDependencyMissing
	}
	return scanRefundPolicyRecord(s.db.QueryRow(ctx, refundPolicySelect+` WHERE idempotency_key=$1`, strings.TrimSpace(key)))
}

const refundPolicySelect = `SELECT id,parent_order_id,vendor_order_id,COALESCE(order_line_id,''),mission_ref,
	trigger,action,reason_code,policy_version,plan_version,automatic,support_required,
	customer_refund_minor,platform_funding_reversal_minor,other_funding_reversal_minor,platform_loss_funding_minor,
	vendor_merchandise_reduction_minor,vendor_delivery_reduction_minor,provider_delivery_reduction_minor,
	platform_revenue_reduction_minor,authorization_release_minor,idempotency_key,wallet_operation_ref,wallet_status,
	projection_applied,support_case_ref,created_at,updated_at FROM commerce_refund_policy_decisions`

type refundPolicyScanner interface{ Scan(...any) error }

func scanRefundPolicyRecord(row refundPolicyScanner) (RefundPolicyRecord, error) {
	var record RefundPolicyRecord
	var trigger, action string
	err := row.Scan(&record.ID, &record.ParentOrderID, &record.VendorOrderID, &record.Decision.OrderLineID, &record.MissionRef,
		&trigger, &action, &record.Decision.ReasonCode, &record.Decision.PolicyVersion, &record.Decision.PlanVersion,
		&record.Decision.Automatic, &record.Decision.SupportRequired, &record.Decision.CustomerRefundMinor,
		&record.Decision.PlatformFundingReversalMinor, &record.Decision.OtherFundingReversalMinor,
		&record.Decision.PlatformLossFundingMinor, &record.Decision.VendorMerchandiseReductionMinor,
		&record.Decision.VendorDeliveryReductionMinor, &record.Decision.ProviderDeliveryReductionMinor,
		&record.Decision.PlatformRevenueReductionMinor, &record.Decision.AuthorizationReleaseMinor,
		&record.IdempotencyKey, &record.WalletOperationRef, &record.WalletStatus, &record.ProjectionApplied, &record.SupportCaseRef,
		&record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefundPolicyRecord{}, ErrOrderNotFound
	}
	record.Decision.Trigger, record.Decision.Action = RefundPolicyTrigger(trigger), RefundPolicyAction(action)
	return record, err
}

func (s *Store) MarkRefundPolicyWallet(ctx context.Context, id, operationRef, status string, now time.Time) error {
	if status != "REQUESTED" && status != "APPLIED" && status != "PENDING_LIABILITY" && status != "UNKNOWN" && status != "FAILED" {
		return ErrOrderConflict
	}
	tag, err := s.db.Exec(ctx, `UPDATE commerce_refund_policy_decisions SET wallet_operation_ref=CASE WHEN $2<>'' THEN $2 ELSE wallet_operation_ref END,wallet_status=$3,updated_at=$4 WHERE id=$1`, id, operationRef, status, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrOrderNotFound
	}
	return nil
}

func (s *Store) MarkRefundPolicySupportCase(ctx context.Context, id, caseRef string, now time.Time) error {
	if strings.TrimSpace(caseRef) == "" {
		return ErrOrderConflict
	}
	tag, err := s.db.Exec(ctx, `UPDATE commerce_refund_policy_decisions SET support_case_ref=$2,updated_at=$3 WHERE id=$1 AND support_required`, id, caseRef, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrOrderNotFound
	}
	return nil
}

func (s *Store) MarkRefundPolicyProjected(ctx context.Context, id string, now time.Time) error {
	tag, err := s.db.Exec(ctx, `UPDATE commerce_refund_policy_decisions SET projection_applied=true,updated_at=$2 WHERE id=$1 AND wallet_status='APPLIED'`, id, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrOrderNotFound
	}
	return nil
}

func (s *Store) PreviousTerminalStatus(ctx context.Context, vendorOrderID string) (VendorStatus, error) {
	var status string
	err := s.db.QueryRow(ctx, `SELECT from_status FROM commerce_order_events WHERE vendor_order_id=$1 AND to_status IN ('CANCELLED','REJECTED') ORDER BY occurred_at DESC,id DESC LIMIT 1`, vendorOrderID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrOrderNotFound
	}
	return VendorStatus(status), err
}

func (s *Store) ListRefundPolicyRecovery(ctx context.Context, limit int) ([]RefundPolicyRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, refundPolicySelect+` WHERE wallet_status IN ('PENDING','REQUESTED','UNKNOWN','PENDING_LIABILITY')
		OR (wallet_status='SUPPORT_REQUIRED' AND support_case_ref='')
		OR (wallet_status='SUPPORT_REQUIRED' AND support_case_ref<>'' AND EXISTS (SELECT 1 FROM commerce_support_case_links l WHERE l.support_case_ref=commerce_refund_policy_decisions.support_case_ref AND l.resolution='REFUND_AUTHORIZED'))
		OR (wallet_status='APPLIED' AND NOT projection_applied)
		OR (wallet_status='APPLIED' AND trigger='VARIABLE_WEIGHT_FINALIZATION' AND EXISTS (
			SELECT 1 FROM commerce_variable_measurements vm WHERE vm.vendor_order_id=commerce_refund_policy_decisions.vendor_order_id
			AND vm.order_line_id=commerce_refund_policy_decisions.order_line_id AND vm.financial_state<>'FINALIZED'))
		ORDER BY updated_at,id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RefundPolicyRecord, 0)
	for rows.Next() {
		record, scanErr := scanRefundPolicyRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func BuildCommerceRefundRequest(record RefundPolicyRecord, state CommerceAllocationState) (CommerceRefundRequest, error) {
	d := record.Decision
	if err := d.Validate(); err != nil || !d.Automatic || d.SupportRequired {
		return CommerceRefundRequest{}, ErrOrderConflict
	}
	vendorRecovery, providerRecovery, platformRecovery := int64(0), int64(0), int64(0)
	var ok bool
	if state.VendorReleasedMinor > 0 {
		vendorRecovery = d.VendorMerchandiseReductionMinor
		platformRecovery = d.PlatformRevenueReductionMinor
	}
	if state.DeliveryReleasedMinor > 0 {
		vendorRecovery, ok = checkedMinorSum(vendorRecovery, d.VendorDeliveryReductionMinor)
		if !ok {
			return CommerceRefundRequest{}, ErrAllocationImbalance
		}
		providerRecovery = d.ProviderDeliveryReductionMinor
	}
	output, outputOK := checkedMinorSum(d.CustomerRefundMinor, d.PlatformFundingReversalMinor, d.OtherFundingReversalMinor)
	releasedRecovery, releasedOK := checkedMinorSum(vendorRecovery, providerRecovery, platformRecovery)
	if !outputOK || !releasedOK || d.PlatformLossFundingMinor > output || releasedRecovery > output-d.PlatformLossFundingMinor {
		return CommerceRefundRequest{}, ErrAllocationImbalance
	}
	escrowRecovery := output - d.PlatformLossFundingMinor - releasedRecovery
	if strings.TrimSpace(state.SnapshotFingerprint) == "" {
		return CommerceRefundRequest{}, ErrAllocationImbalance
	}
	return CommerceRefundRequest{
		VendorOrderID: record.VendorOrderID, ItemReference: "refund-policy:" + record.ID,
		CustomerRefundMinor: d.CustomerRefundMinor, PlatformFundingReversalMinor: d.PlatformFundingReversalMinor,
		OtherFundingReversalMinor: d.OtherFundingReversalMinor, EscrowRecoveryMinor: escrowRecovery,
		VendorRecoveryMinor: vendorRecovery, ProviderRecoveryMinor: providerRecovery, PlatformRecoveryMinor: platformRecovery,
		PlatformLossFundingMinor:        d.PlatformLossFundingMinor,
		VendorMerchandiseReductionMinor: d.VendorMerchandiseReductionMinor, VendorDeliveryReductionMinor: d.VendorDeliveryReductionMinor,
		ProviderDeliveryReductionMinor: d.ProviderDeliveryReductionMinor, PlatformRevenueReductionMinor: d.PlatformRevenueReductionMinor,
		AuthorizationReleaseMinor: d.AuthorizationReleaseMinor, SnapshotFingerprint: state.SnapshotFingerprint,
		IdempotencyKey: record.IdempotencyKey,
	}, nil
}

func (s *Service) executeCommerceRefund(ctx context.Context, record RefundPolicyRecord, terminalOrder bool) (CommerceFinancialResult, error) {
	authority, ok := s.payments.(CommerceAllocationStateAuthority)
	settlement, canSettle := s.payments.(CommerceSettlementAuthority)
	if !ok || !canSettle {
		return CommerceFinancialResult{}, ErrDependencyMissing
	}
	if record.WalletStatus == "APPLIED" {
		if !record.ProjectionApplied {
			if state, stateErr := authority.GetCommerceAllocationState(ctx, record.ParentOrderID, record.VendorOrderID); stateErr == nil {
				_ = s.store.SyncPaymentLegRefundedTotal(ctx, record.VendorOrderID, state.RefundCustomerMinor, s.now().UTC())
				if s.projectCommerceRefundOutcome(ctx, record.VendorOrderID, state, record.WalletOperationRef) == nil {
					_ = s.store.MarkRefundPolicyProjected(ctx, record.ID, s.now().UTC())
				}
			}
		}
		return CommerceFinancialResult{Reference: record.WalletOperationRef, Status: "APPLIED", AmountMinor: record.Decision.OutputMinor()}, nil
	}
	if existing, err := authority.GetCommerceRefundOperation(ctx, record.IdempotencyKey); err == nil && existing.Applied() {
		_ = s.store.MarkRefundPolicyWallet(ctx, record.ID, existing.Reference, "APPLIED", s.now().UTC())
		if state, stateErr := authority.GetCommerceAllocationState(ctx, record.ParentOrderID, record.VendorOrderID); stateErr == nil {
			_ = s.store.SyncPaymentLegRefundedTotal(ctx, record.VendorOrderID, state.RefundCustomerMinor, s.now().UTC())
			if s.projectCommerceRefundOutcome(ctx, record.VendorOrderID, state, existing.Reference) == nil {
				_ = s.store.MarkRefundPolicyProjected(ctx, record.ID, s.now().UTC())
			}
		}
		return existing, nil
	}
	state, err := authority.GetCommerceAllocationState(ctx, record.ParentOrderID, record.VendorOrderID)
	if err != nil {
		return CommerceFinancialResult{}, err
	}
	request, err := BuildCommerceRefundRequest(record, state)
	if err != nil {
		return CommerceFinancialResult{}, err
	}
	_ = s.store.MarkRefundPolicyWallet(ctx, record.ID, "", "REQUESTED", s.now().UTC())
	_ = s.store.SetRefundState(ctx, record.VendorOrderID, "REFUND_PENDING", "", state.RefundCustomerMinor, s.now().UTC())
	result, err := settlement.RefundCommerceAllocation(ctx, request)
	if err != nil {
		_ = s.store.MarkRefundPolicyWallet(ctx, record.ID, result.Reference, "UNKNOWN", s.now().UTC())
		_ = s.store.SetRefundState(ctx, record.VendorOrderID, "UNKNOWN", result.Reference, state.RefundCustomerMinor, s.now().UTC())
		return result, err
	}
	status := result.Status
	if status != "APPLIED" && status != "PENDING_LIABILITY" {
		_ = s.store.MarkRefundPolicyWallet(ctx, record.ID, result.Reference, "UNKNOWN", s.now().UTC())
		_ = s.store.SetRefundState(ctx, record.VendorOrderID, "UNKNOWN", result.Reference, state.RefundCustomerMinor, s.now().UTC())
		return result, ErrPaymentAuthority
	}
	_ = s.store.MarkRefundPolicyWallet(ctx, record.ID, result.Reference, status, s.now().UTC())
	if status == "APPLIED" {
		if updated, stateErr := authority.GetCommerceAllocationState(ctx, record.ParentOrderID, record.VendorOrderID); stateErr == nil {
			_ = s.store.SyncPaymentLegRefundedTotal(ctx, record.VendorOrderID, updated.RefundCustomerMinor, s.now().UTC())
			if s.projectCommerceRefundOutcome(ctx, record.VendorOrderID, updated, result.Reference) == nil {
				_ = s.store.MarkRefundPolicyProjected(ctx, record.ID, s.now().UTC())
			}
		}
	}
	if status == "PENDING_LIABILITY" {
		_ = s.store.SetRefundState(ctx, record.VendorOrderID, "REFUND_PENDING", result.Reference, state.RefundCustomerMinor, s.now().UTC())
	}
	return result, nil
}

func (s *Service) projectCommerceRefundOutcome(ctx context.Context, vendorOrderID string, state CommerceAllocationState, operationRef string) error {
	if state.RefundCustomerMinor < 0 || state.MaxAuthorizedMinor < 0 || state.RefundCustomerMinor > state.MaxAuthorizedMinor {
		return ErrAllocationImbalance
	}
	projection := "PARTIALLY_REFUNDED"
	if state.MaxAuthorizedMinor > 0 && state.RefundCustomerMinor == state.MaxAuthorizedMinor {
		projection = "REFUNDED"
	}
	return s.store.SetRefundState(ctx, vendorOrderID, projection, operationRef, state.RefundCustomerMinor, s.now().UTC())
}

func (s *Store) SyncPaymentLegRefundedTotal(ctx context.Context, vendorOrderID string, refundedTotal int64, now time.Time) error {
	if refundedTotal < 0 {
		return ErrAllocationImbalance
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var legID string
	var amount, settled, currentRefunded int64
	err = tx.QueryRow(ctx, `SELECT id,amount_minor,settled_minor,refunded_minor FROM commerce_payment_legs WHERE vendor_order_id=$1 FOR UPDATE`, vendorOrderID).Scan(&legID, &amount, &settled, &currentRefunded)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderNotFound
	}
	if err != nil {
		return err
	}
	if refundedTotal > amount || refundedTotal < currentRefunded {
		return ErrAllocationImbalance
	}
	if refundedTotal == currentRefunded {
		return tx.Commit(ctx)
	}
	maximumSettled := amount - refundedTotal
	if settled > maximumSettled {
		settled = maximumSettled
	}
	status := string(LegHeld)
	if refundedTotal == amount {
		status = string(LegRefunded)
	} else if settled > 0 {
		status = string(LegSettled)
	}
	_, err = tx.Exec(ctx, `UPDATE commerce_payment_legs SET refunded_minor=$2,settled_minor=$3,status=$4,version=version+1,updated_at=$5 WHERE id=$1`, legID, refundedTotal, settled, status, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ensureAndExecuteRefundDecision(ctx context.Context, order VendorOrder, missionRef, key string, decision RefundPolicyDecision, terminalOrder bool) (RefundPolicyRecord, error) {
	record, _, err := s.store.EnsureRefundPolicyDecision(ctx, order.ParentOrderID, order.ID, missionRef, key, decision, s.now().UTC())
	if err != nil {
		return RefundPolicyRecord{}, err
	}
	if err = s.releaseRefundInventory(ctx, order, decision, key); err != nil {
		return record, err
	}
	if decision.SupportRequired || !decision.Automatic {
		if decision.SupportRequired && record.SupportCaseRef == "" {
			if s.refundSupport == nil {
				return record, ErrDependencyMissing
			}
			created, createErr := s.refundSupport.CreateRefundCase(ctx, RefundSupportRequest{
				OrganizationReference: order.BusinessID, IdempotencyKey: "commerce-refund-support-" + record.ID,
				ReasonCode: decision.ReasonCode, CustomerReference: order.CustomerID,
				MissionReference: missionRef, ParentOrderReference: order.ParentOrderID, VendorOrderReference: order.ID,
			})
			if createErr != nil || created.CaseID == "" {
				if createErr != nil {
					return record, createErr
				}
				return record, ErrDependencyMissing
			}
			if createErr = s.store.MarkRefundPolicySupportCase(ctx, record.ID, created.CaseID, s.now().UTC()); createErr != nil {
				return record, createErr
			}
			if createErr = s.store.LinkRefundSupportCase(ctx, order, record, created, s.now().UTC()); createErr != nil {
				return record, createErr
			}
			record.SupportCaseRef = created.CaseID
		}
		return record, nil
	}
	_, err = s.executeCommerceRefund(ctx, record, terminalOrder)
	return record, err
}

func (s *Service) releaseRefundInventory(ctx context.Context, order VendorOrder, decision RefundPolicyDecision, key string) error {
	if decision.Trigger != RefundTriggerItemUnavailable || !decision.Automatic || !order.ReservationRequired {
		return nil
	}
	authority, ok := s.inventory.(ItemInventoryAuthority)
	if !ok {
		return ErrDependencyMissing
	}
	var inventoryItemRef string
	err := s.store.db.QueryRow(ctx, `SELECT inventory_item_ref FROM commerce_vendor_order_lines WHERE id=$1 AND vendor_order_id=$2 AND inventory_item_ref<>''`, decision.OrderLineID, order.ID).Scan(&inventoryItemRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderNotFound
	}
	if err != nil {
		return err
	}
	return authority.ReleaseReservationItem(ctx, order.BusinessID, order.BranchID, order.ReservationRef, inventoryItemRef, key)
}

func (s *Service) RecoverCommerceRefundPolicies(ctx context.Context, limit int) (int, error) {
	if s == nil || s.payments == nil {
		return 0, nil
	}
	records, err := s.store.ListRefundPolicyRecovery(ctx, limit)
	if err != nil {
		return 0, err
	}
	var recovered int
	for _, record := range records {
		terminal := record.Decision.Trigger == RefundTriggerVendorCancelled || record.Decision.Trigger == RefundTriggerVendorRejected
		order, loadErr := s.store.getVendorOrderByID(ctx, record.VendorOrderID)
		if loadErr != nil {
			continue
		}
		if record.Decision.SupportRequired {
			var resolution string
			_ = s.store.db.QueryRow(ctx, `SELECT resolution FROM commerce_support_case_links WHERE support_case_ref=$1`, record.SupportCaseRef).Scan(&resolution)
			if resolution == "REFUND_AUTHORIZED" {
				if result, supportErr := s.executeCommerceRefund(ctx, record, terminal); supportErr == nil && result.Applied() {
					recovered++
				}
				continue
			}
			if updated, supportErr := s.ensureAndExecuteRefundDecision(ctx, order, record.MissionRef, record.IdempotencyKey, record.Decision, false); supportErr == nil && updated.SupportCaseRef != "" {
				recovered++
			}
			continue
		}
		if s.releaseRefundInventory(ctx, order, record.Decision, record.IdempotencyKey) != nil {
			continue
		}
		result, executeErr := s.executeCommerceRefund(ctx, record, terminal)
		if executeErr == nil && result.Applied() && record.Decision.Trigger == RefundTriggerVariableFinal {
			var finalAmount, quotedAmount int64
			projectionErr := s.store.db.QueryRow(ctx, `SELECT vm.final_amount_minor,l.quoted_total_minor FROM commerce_variable_measurements vm JOIN commerce_vendor_order_lines l ON l.id=vm.order_line_id WHERE vm.vendor_order_id=$1 AND vm.order_line_id=$2`, record.VendorOrderID, record.Decision.OrderLineID).Scan(&finalAmount, &quotedAmount)
			if projectionErr != nil || s.completeVariableFinancialProjection(ctx, order, record.Decision.OrderLineID, finalAmount, quotedAmount, "payment-recovery") != nil {
				continue
			}
		}
		if executeErr == nil && (result.Applied() || result.Status == "PENDING_LIABILITY") {
			recovered++
		}
	}
	return recovered, nil
}

// RecoverMissingRefundObligations closes the crash window between an
// authoritative operational fact and creation of its immutable policy row.
// It reconstructs only from frozen database evidence; no live prices or
// caller-supplied amounts participate.
func (s *Service) RecoverMissingRefundObligations(ctx context.Context, limit int) (int, error) {
	if s == nil || s.store == nil || s.store.db == nil || s.payments == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	created := 0

	deliveryRows, err := s.store.db.Query(ctx, `SELECT e.event_id,e.mission_ref,e.vendor_order_id,e.payload
		FROM commerce_logistics_order_events e WHERE e.event_type='DELIVERY_FAILED' AND e.applied
		AND NOT EXISTS (SELECT 1 FROM commerce_refund_policy_decisions d WHERE d.idempotency_key='commerce-delivery-failure-'||e.event_id)
		ORDER BY e.received_at,e.id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	for deliveryRows.Next() {
		var eventID, missionRef, vendorOrderID string
		var payload []byte
		if err = deliveryRows.Scan(&eventID, &missionRef, &vendorOrderID, &payload); err != nil {
			deliveryRows.Close()
			return created, err
		}
		order, loadErr := s.store.getVendorOrderByID(ctx, vendorOrderID)
		failure, parseErr := parseDeliveryFailurePayload(payload)
		if loadErr != nil || parseErr != nil {
			continue
		}
		leg, _, legErr := s.store.PaymentLegForVendorOrder(ctx, order.CustomerID, vendorOrderID)
		if legErr != nil {
			continue
		}
		decision, policyErr := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerDeliveryFailure, Order: order, MaxAuthorizedMinor: leg.AmountMinor, DeliveryFailurePolicy: failure.Policy, PreserveProviderEarning: failure.PreserveProviderEarning})
		if policyErr == nil {
			if _, policyErr = s.ensureAndExecuteRefundDecision(ctx, order, missionRef, "commerce-delivery-failure-"+eventID, decision, false); policyErr == nil {
				created++
			}
		}
	}
	if err = deliveryRows.Err(); err != nil {
		deliveryRows.Close()
		return created, err
	}
	deliveryRows.Close()

	itemRows, err := s.store.db.Query(ctx, `SELECT o.vendor_order_id,o.order_line_id
		FROM commerce_vendor_order_line_outcomes o JOIN commerce_shop_picking_snapshots p ON p.order_line_id=o.order_line_id
		WHERE o.outcome='UNAVAILABLE' AND p.substitution_policy='AUTO_REFUND'
		AND NOT EXISTS (SELECT 1 FROM commerce_refund_policy_decisions d WHERE d.idempotency_key='commerce-item-unavailable-'||o.order_line_id)
		ORDER BY o.updated_at,o.id LIMIT $1`, limit)
	if err != nil {
		return created, err
	}
	for itemRows.Next() {
		var vendorOrderID, lineID string
		if err = itemRows.Scan(&vendorOrderID, &lineID); err != nil {
			itemRows.Close()
			return created, err
		}
		order, loadErr := s.store.getVendorOrderByID(ctx, vendorOrderID)
		if loadErr != nil {
			continue
		}
		decision, policyErr := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerItemUnavailable, Order: order, OrderLineID: lineID, SubstitutionPolicy: PolicyAutoRefund})
		if policyErr == nil {
			if _, policyErr = s.ensureAndExecuteRefundDecision(ctx, order, "", "commerce-item-unavailable-"+lineID, decision, false); policyErr == nil {
				created++
			}
		}
	}
	if err = itemRows.Err(); err != nil {
		itemRows.Close()
		return created, err
	}
	itemRows.Close()

	variableRows, err := s.store.db.Query(ctx, `SELECT vm.vendor_order_id,vm.order_line_id,vm.unit_price_minor,vm.hard_max_quantity,vm.final_amount_minor,l.quoted_total_minor
		FROM commerce_variable_measurements vm JOIN commerce_vendor_order_lines l ON l.id=vm.order_line_id
		WHERE vm.financial_state IN ('FINANCIAL_PENDING','FAILED_RECOVERABLE')
		AND NOT EXISTS (SELECT 1 FROM commerce_refund_policy_decisions d WHERE d.idempotency_key='variable-measurement-refund-'||vm.vendor_order_id||'-'||vm.order_line_id)
		ORDER BY vm.updated_at,vm.id LIMIT $1`, limit)
	if err != nil {
		return created, err
	}
	for variableRows.Next() {
		var vendorOrderID, lineID string
		var unitPrice, hardMax, finalAmount, quotedAmount int64
		if err = variableRows.Scan(&vendorOrderID, &lineID, &unitPrice, &hardMax, &finalAmount, &quotedAmount); err != nil {
			variableRows.Close()
			return created, err
		}
		order, loadErr := s.store.getVendorOrderByID(ctx, vendorOrderID)
		authorized, amountErr := CalculateMeasuredAmount(unitPrice, hardMax)
		if loadErr != nil || amountErr != nil || finalAmount < 0 || finalAmount > authorized {
			continue
		}
		difference := authorized - finalAmount
		if difference == 0 {
			if s.completeVariableFinancialProjection(ctx, order, lineID, finalAmount, quotedAmount, "payment-recovery") == nil {
				created++
			}
			continue
		}
		authorizationRelease := authorized - quotedAmount
		if authorizationRelease < 0 {
			continue
		}
		if authorizationRelease > difference {
			authorizationRelease = difference
		}
		decision, policyErr := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerVariableFinal, Order: order, OrderLineID: lineID, DifferenceMinor: difference, AuthorizationReleaseMinor: authorizationRelease})
		if policyErr == nil {
			if _, _, policyErr = s.store.EnsureRefundPolicyDecision(ctx, order.ParentOrderID, order.ID, "", "variable-measurement-refund-"+order.ID+"-"+lineID, decision, s.now().UTC()); policyErr == nil {
				created++
			}
		}
	}
	if err = variableRows.Err(); err != nil {
		variableRows.Close()
		return created, err
	}
	variableRows.Close()
	return created, nil
}
