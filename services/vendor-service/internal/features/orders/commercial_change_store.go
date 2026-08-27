package orders

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type commercialEventWriter interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func appendCommercialChangeEvent(ctx context.Context, q commercialEventWriter, changeID, parentID, customerID, action, actorType, actorRef, fromState, toState, walletReference string, now time.Time) error {
	_, err := q.Exec(ctx, `INSERT INTO commerce_order_change_events(id,change_id,parent_order_id,customer_id,action,actor_type,actor_ref,from_state,to_state,wallet_reference,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, "coce_"+uuid.NewString(), changeID, parentID, customerID, action, actorType, actorRef, fromState, toState, walletReference, now)
	return err
}

func (s *Store) CreateCommercialChange(ctx context.Context, parent ParentOrder, request ChangeRequest, quote ReplacementQuote, now time.Time) (CommercialChange, error) {
	if s == nil || s.db == nil {
		return CommercialChange{}, ErrDependencyMissing
	}
	originalAddress, err := json.Marshal(parent.Address)
	if err != nil {
		return CommercialChange{}, err
	}
	replacementAddress, err := json.Marshal(quote.Address)
	if err != nil {
		return CommercialChange{}, err
	}
	if parent.CustomerApprovedTotalMinor < 0 || quote.TotalMinor < 0 {
		return CommercialChange{}, ErrCommercialChangeConflict
	}
	delta := quote.TotalMinor - parent.CustomerApprovedTotalMinor
	changeID := "coc_" + uuid.NewString()
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return CommercialChange{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var parentState string
	if err = tx.QueryRow(ctx, `SELECT state FROM commerce_parent_orders WHERE id=$1 AND customer_id=$2 FOR UPDATE`, parent.ID, parent.CustomerID).Scan(&parentState); errors.Is(err, pgx.ErrNoRows) {
		return CommercialChange{}, ErrOrderNotFound
	} else if err != nil {
		return CommercialChange{}, err
	}
	if TerminalParentState(ParentState(parentState)) {
		return CommercialChange{}, ErrOrderConflict
	}
	inserted := false
	err = tx.QueryRow(ctx, `
		INSERT INTO commerce_order_changes(
		 id,parent_order_id,customer_id,original_quote_id,original_quote_version,original_quote_fingerprint,
		 original_address_snapshot,original_fulfilment_plan_id,original_total_minor,
		 requested_checkout_session_id,requested_fulfilment_plan_id,replacement_quote_id,
		 replacement_quote_version,replacement_quote_fingerprint,replacement_address_snapshot,
		 replacement_fulfilment_plan_id,replacement_total_minor,replacement_quote_expires_at,delta_minor,currency,state,version,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,'AWAITING_CUSTOMER_APPROVAL',1,$21,$21)
		ON CONFLICT (parent_order_id,requested_checkout_session_id,requested_fulfilment_plan_id)
		DO NOTHING
		RETURNING id`, changeID, parent.ID, parent.CustomerID, parent.QuoteID, parent.QuoteVersion, parent.QuoteFingerprint,
		originalAddress, parent.FulfilmentPlanID, parent.CustomerApprovedTotalMinor, request.CheckoutSessionID,
		request.FulfilmentPlanID, quote.ID, quote.Version, quote.Fingerprint, replacementAddress, quote.FulfilmentPlanID,
		quote.TotalMinor, quote.ExpiresAt, delta, quote.Currency, now).Scan(&changeID)
	if err == nil {
		inserted = true
	} else if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM commerce_order_changes WHERE parent_order_id=$1 AND requested_checkout_session_id=$2 AND requested_fulfilment_plan_id=$3`, parent.ID, request.CheckoutSessionID, request.FulfilmentPlanID).Scan(&changeID)
	}
	if err != nil {
		return CommercialChange{}, err
	}
	if inserted {
		if err = appendCommercialChangeEvent(ctx, tx, changeID, parent.ID, parent.CustomerID, "commerce.order.change_requested", "CUSTOMER", parent.CustomerID, "", string(ChangeAwaitingApproval), "", now); err != nil {
			return CommercialChange{}, err
		}
		if err = appendCommercialChangeEvent(ctx, tx, changeID, parent.ID, parent.CustomerID, "commerce.order.requote_created", "SYSTEM", "", string(ChangeRequested), string(ChangeAwaitingApproval), "", now); err != nil {
			return CommercialChange{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return CommercialChange{}, err
	}
	return s.GetCommercialChange(ctx, parent.CustomerID, changeID)
}

func (s *Store) ExistingCommercialChange(ctx context.Context, customerID, parentOrderID, sessionID, planID string) (CommercialChange, error) {
	var id string
	err := s.db.QueryRow(ctx, `SELECT id FROM commerce_order_changes WHERE customer_id=$1 AND parent_order_id=$2 AND requested_checkout_session_id=$3 AND requested_fulfilment_plan_id=$4`, customerID, parentOrderID, sessionID, planID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return CommercialChange{}, ErrCommercialChangeNotFound
	}
	if err != nil {
		return CommercialChange{}, err
	}
	return s.GetCommercialChange(ctx, customerID, id)
}

func (s *Store) GetCommercialChange(ctx context.Context, customerID, changeID string) (CommercialChange, error) {
	if s == nil || s.db == nil {
		return CommercialChange{}, ErrDependencyMissing
	}
	var c CommercialChange
	var originalAddress, replacementAddress []byte
	var approvedAt, appliedAt *time.Time
	var state, walletStatus string
	err := s.db.QueryRow(ctx, `
		SELECT id,parent_order_id,customer_id,original_quote_id,original_quote_version,original_quote_fingerprint,
		 original_address_snapshot,original_fulfilment_plan_id,original_total_minor,
		 requested_checkout_session_id,requested_fulfilment_plan_id,replacement_quote_id,replacement_quote_version,
		 replacement_quote_fingerprint,replacement_address_snapshot,replacement_fulfilment_plan_id,
		 replacement_total_minor,replacement_quote_expires_at,delta_minor,currency,state,approval_fingerprint,wallet_operation_ref,wallet_status,
		 version,approved_at,applied_at,created_at,updated_at
		 FROM commerce_order_changes WHERE id=$1 AND customer_id=$2`, changeID, customerID).Scan(
		&c.ID, &c.ParentOrderID, &c.CustomerID, &c.OriginalQuote.QuoteID, &c.OriginalQuote.QuoteVersion,
		&c.OriginalQuote.QuoteFingerprint, &originalAddress, &c.OriginalPlanID, &c.OriginalTotal,
		&c.RequestedSessionID, &c.RequestedPlanID, &c.ReplacementQuote.ID, &c.ReplacementQuote.Version,
		&c.ReplacementQuote.Fingerprint, &replacementAddress, &c.ReplacementQuote.FulfilmentPlanID,
		&c.ReplacementQuote.TotalMinor, &c.ReplacementQuote.ExpiresAt, &c.DeltaMinor, &c.Currency, &state, &c.ApprovalFingerprint,
		&c.WalletOperationRef, &walletStatus, &c.Version, &approvedAt, &appliedAt, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CommercialChange{}, ErrCommercialChangeNotFound
	}
	if err != nil {
		return CommercialChange{}, err
	}
	if err = json.Unmarshal(originalAddress, &c.OriginalAddress); err != nil {
		return CommercialChange{}, err
	}
	if err = json.Unmarshal(replacementAddress, &c.ReplacementQuote.Address); err != nil {
		return CommercialChange{}, err
	}
	c.OriginalQuote.CustomerID = c.CustomerID
	c.OriginalQuote.QuotedTotalMinor = c.OriginalTotal
	c.OriginalQuote.Currency = c.Currency
	c.ReplacementQuote.CustomerID = c.CustomerID
	c.ReplacementQuote.Currency = c.Currency
	c.ReplacementQuote.Approved = c.ApprovalFingerprint != ""
	c.State = CommercialChangeState(state)
	c.WalletStatus = CommercialWalletStatus(walletStatus)
	c.ApprovedAt = approvedAt
	c.AppliedAt = appliedAt
	return c, nil
}

func (s *Store) MarkCommercialChangeApproved(ctx context.Context, changeID, customerID string, quote ReplacementQuote, fingerprint string, now time.Time) (CommercialChange, error) {
	var version int64
	err := s.db.QueryRow(ctx, `UPDATE commerce_order_changes SET state='APPROVED',approval_fingerprint=$3,approved_at=$4,version=version+1,updated_at=$4 WHERE id=$1 AND customer_id=$2 AND state IN ('AWAITING_CUSTOMER_APPROVAL','REQUOTE_READY') RETURNING version`, changeID, customerID, fingerprint, now).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		current, loadErr := s.GetCommercialChange(ctx, customerID, changeID)
		if loadErr != nil {
			return CommercialChange{}, loadErr
		}
		if (current.State == ChangeApproved || current.State == ChangeApplied) && current.ApprovalFingerprint == fingerprint {
			return current, nil
		}
		return CommercialChange{}, ErrCommercialChangeConflict
	}
	if err != nil {
		return CommercialChange{}, err
	}
	var parentID string
	if err = s.db.QueryRow(ctx, `SELECT parent_order_id FROM commerce_order_changes WHERE id=$1`, changeID).Scan(&parentID); err != nil {
		return CommercialChange{}, err
	}
	if err = appendCommercialChangeEvent(ctx, s.db, changeID, parentID, customerID, "commerce.order.requote_approved", "CUSTOMER", customerID, string(ChangeAwaitingApproval), string(ChangeApproved), "", now); err != nil {
		return CommercialChange{}, err
	}
	return s.GetCommercialChange(ctx, customerID, changeID)
}

func (s *Store) RejectCommercialChange(ctx context.Context, customerID, changeID string, now time.Time) (CommercialChange, error) {
	var parentID string
	err := s.db.QueryRow(ctx, `UPDATE commerce_order_changes SET state='REJECTED',version=version+1,updated_at=$3 WHERE id=$1 AND customer_id=$2 AND state IN ('AWAITING_CUSTOMER_APPROVAL','REQUOTE_READY') RETURNING parent_order_id`, changeID, customerID, now).Scan(&parentID)
	if errors.Is(err, pgx.ErrNoRows) {
		current, loadErr := s.GetCommercialChange(ctx, customerID, changeID)
		if loadErr != nil {
			return CommercialChange{}, loadErr
		}
		if current.State == ChangeRejected {
			return current, nil
		}
		return CommercialChange{}, ErrCommercialChangeConflict
	}
	if err != nil {
		return CommercialChange{}, err
	}
	if err = appendCommercialChangeEvent(ctx, s.db, changeID, parentID, customerID, "commerce.order.change_rejected", "CUSTOMER", customerID, string(ChangeAwaitingApproval), string(ChangeRejected), "", now); err != nil {
		return CommercialChange{}, err
	}
	return s.GetCommercialChange(ctx, customerID, changeID)
}

func (s *Store) MarkCommercialChangeExpired(ctx context.Context, customerID, changeID string, now time.Time) error {
	var parentID string
	err := s.db.QueryRow(ctx, `UPDATE commerce_order_changes SET state='EXPIRED',version=version+1,updated_at=$3 WHERE id=$1 AND customer_id=$2 AND state IN ('AWAITING_CUSTOMER_APPROVAL','REQUOTE_READY') RETURNING parent_order_id`, changeID, customerID, now).Scan(&parentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return appendCommercialChangeEvent(ctx, s.db, changeID, parentID, customerID, "commerce.order.change_expired", "SYSTEM", "expiry", string(ChangeAwaitingApproval), string(ChangeExpired), "", now)
}

func (s *Store) MarkCommercialWalletRequested(ctx context.Context, changeID string, now time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE commerce_order_changes SET wallet_status='REQUESTED',version=version+1,updated_at=$2 WHERE id=$1 AND state='APPROVED'`, changeID, now)
	return err
}

func (s *Store) MarkCommercialWallet(ctx context.Context, changeID, reference string, status CommercialWalletStatus, now time.Time) (CommercialChange, error) {
	_, err := s.db.Exec(ctx, `UPDATE commerce_order_changes SET wallet_operation_ref=CASE WHEN $2='' THEN wallet_operation_ref ELSE $2 END,wallet_status=$3,version=version+1,updated_at=$4 WHERE id=$1`, changeID, reference, status, now)
	if err != nil {
		return CommercialChange{}, err
	}
	var customer string
	if err = s.db.QueryRow(ctx, `SELECT customer_id FROM commerce_order_changes WHERE id=$1`, changeID).Scan(&customer); err != nil {
		return CommercialChange{}, err
	}
	return s.GetCommercialChange(ctx, customer, changeID)
}

func (s *Store) ListCommercialChangesForRecovery(ctx context.Context, limit int) ([]CommercialChange, error) {
	rows, err := s.db.Query(ctx, `SELECT id,customer_id FROM commerce_order_changes WHERE state='APPROVED' AND wallet_status IN ('REQUESTED','UNKNOWN','HELD','REFUNDED') ORDER BY updated_at,id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommercialChange
	for rows.Next() {
		var id, customer string
		if err = rows.Scan(&id, &customer); err != nil {
			return nil, err
		}
		change, loadErr := s.GetCommercialChange(ctx, customer, id)
		if loadErr != nil {
			return nil, loadErr
		}
		out = append(out, change)
	}
	return out, rows.Err()
}

func (s *Store) ApplyCommercialChange(ctx context.Context, change CommercialChange, now time.Time) (CommercialChange, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return CommercialChange{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentState string
	if err = tx.QueryRow(ctx, `SELECT state FROM commerce_order_changes WHERE id=$1 AND customer_id=$2 FOR UPDATE`, change.ID, change.CustomerID).Scan(&currentState); err != nil {
		return CommercialChange{}, err
	}
	if currentState == string(ChangeApplied) {
		return change, nil
	}
	if currentState != string(ChangeApproved) {
		return CommercialChange{}, ErrCommercialChangeConflict
	}
	address, err := json.Marshal(change.ReplacementQuote.Address)
	if err != nil {
		return CommercialChange{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO commerce_order_change_applications(id,change_id,parent_order_id,customer_id,replacement_quote_id,replacement_quote_fingerprint,applied_address_snapshot,applied_fulfilment_plan_id,applied_total_minor,applied_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(change_id) DO NOTHING`, "coca_"+uuid.NewString(), change.ID, change.ParentOrderID, change.CustomerID, change.ReplacementQuote.ID, change.ReplacementQuote.Fingerprint, address, change.ReplacementQuote.FulfilmentPlanID, change.ReplacementQuote.TotalMinor, now)
	if err != nil {
		return CommercialChange{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_order_changes SET state='APPLIED',applied_at=$2,version=version+1,updated_at=$2 WHERE id=$1`, change.ID, now); err != nil {
		return CommercialChange{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_parent_orders SET version=version+1,updated_at=$2 WHERE id=$1`, change.ParentOrderID, now); err != nil {
		return CommercialChange{}, err
	}
	if err = appendCommercialChangeEvent(ctx, tx, change.ID, change.ParentOrderID, change.CustomerID, "commerce.order.change_applied", "SYSTEM", "", string(ChangeApproved), string(ChangeApplied), change.WalletOperationRef, now); err != nil {
		return CommercialChange{}, err
	}
	// A substitution-triggered change becomes applicable only after the
	// replacement quote and Wallet delta are applied. The idempotency key is a
	// durable link back to the proposal without adding a second substitution
	// authority/table.
	var substitutionVendor, substitutionProposal string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(vendor_order_id,''),replace(idempotency_key,'substitution-change-','') FROM commerce_order_adjustments WHERE change_id=$1 AND idempotency_key LIKE 'substitution-change-%' LIMIT 1`, change.ID).Scan(&substitutionVendor, &substitutionProposal); err == nil && substitutionVendor != "" && substitutionProposal != "" {
		if _, err = tx.Exec(ctx, `UPDATE commerce_substitution_proposals SET status='APPROVED',version=version+1,responded_at=$1,updated_at=$1,response_ref=$2 WHERE id=$3 AND customer_id=$4 AND status='PENDING'`, now, change.ID, substitutionProposal, change.CustomerID); err != nil {
			return CommercialChange{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE commerce_substitution_cases c SET state='APPROVED',updated_at=$1 WHERE c.id=(SELECT p.case_id FROM commerce_substitution_proposals p WHERE p.id=$2 AND p.customer_id=$3) AND c.state IN ('OPEN','AWAITING_CUSTOMER')`, now, substitutionProposal, change.CustomerID); err != nil {
			return CommercialChange{}, err
		}
		if err = appendEvent(ctx, tx, nil, change.ParentOrderID, substitutionVendor, change.CustomerID, "commerce.vendor_order.substitution_approved", "SYSTEM", change.ID, "SUBSTITUTION_REQUIRED", "PICKING", now); err != nil {
			return CommercialChange{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE commerce_vendor_orders SET status='PICKING',version=version+1,updated_at=$1 WHERE id=$2 AND status IN ('SUBSTITUTION_REQUIRED','AWAITING_CUSTOMER')`, now, substitutionVendor); err != nil {
			return CommercialChange{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return CommercialChange{}, err
	}
	return s.GetCommercialChange(ctx, change.CustomerID, change.ID)
}

func (s *Store) FirstRefundableLeg(operation PaymentOperation) (PaymentLegRecord, error) {
	for _, leg := range operation.Legs {
		// The v1 Wallet refund contract accepts held escrow only. A cheaper
		// benefit is therefore reconciled before that leg is settled; the
		// authority remains the source of truth for the final outcome.
		if leg.Status == LegHeld && leg.IntentReference != "" && leg.RefundedMinor < leg.AmountMinor {
			return leg, nil
		}
	}
	return PaymentLegRecord{}, ErrAdjustmentNotAvailable
}

func (s *Store) FirstRefundableDeliveryLeg(operation PaymentOperation, order ParentOrder) (PaymentLegRecord, error) {
	for _, vendor := range order.VendorOrders {
		if vendor.DeliveryCustomerMinor <= 0 {
			continue
		}
		for _, leg := range operation.Legs {
			if leg.VendorOrderID == vendor.ID && leg.Status == LegHeld && leg.IntentReference != "" && leg.RefundedMinor < leg.AmountMinor {
				return leg, nil
			}
		}
	}
	return PaymentLegRecord{}, ErrAdjustmentNotAvailable
}

func (s *Store) EnsureAdjustment(ctx context.Context, order ParentOrder, leg PaymentLegRecord, difference, original, eligible int64, policy, key string, now time.Time) (AdjustmentEvidence, bool, error) {
	return s.ensureAdjustment(ctx, order, leg, "CHEAPER_REPLAN", difference, original, eligible, policy, "REFUND", key, now)
}

func (s *Store) EnsureCancellationAdjustment(ctx context.Context, order ParentOrder, vendor VendorOrder, leg PaymentLegRecord, policy, key string, now time.Time) (AdjustmentEvidence, error) {
	adj, _, err := s.ensureAdjustment(ctx, order, leg, "CANCELLATION_REFUND", leg.AmountMinor, leg.AmountMinor, 0, policy, "REFUND", key, now)
	if err != nil {
		return AdjustmentEvidence{}, err
	}
	return adj, nil
}

func (s *Store) EnsureChangeAdjustment(ctx context.Context, change CommercialChange, typ, action, policy, key string, now time.Time) (AdjustmentEvidence, error) {
	var id string
	err := s.db.QueryRow(ctx, `INSERT INTO commerce_order_adjustments(id,parent_order_id,change_id,adjustment_type,original_commitment_minor,eligible_final_minor,difference_minor,policy,action,idempotency_key,wallet_status,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'PENDING',1,$11,$11) ON CONFLICT(idempotency_key) DO UPDATE SET id=commerce_order_adjustments.id RETURNING id`, "coa_"+uuid.NewString(), change.ParentOrderID, change.ID, typ, change.OriginalTotal, change.ReplacementQuote.TotalMinor, abs(change.DeltaMinor), policy, action, key, now).Scan(&id)
	if err != nil {
		return AdjustmentEvidence{}, err
	}
	adjustment, err := s.GetAdjustment(ctx, id)
	if err != nil {
		return AdjustmentEvidence{}, err
	}
	difference := abs(change.DeltaMinor)
	if adjustment.ParentOrderID != change.ParentOrderID || adjustment.ChangeID != change.ID || adjustment.Type != typ ||
		adjustment.OriginalCommitmentMinor != change.OriginalTotal || adjustment.EligibleFinalMinor != change.ReplacementQuote.TotalMinor ||
		adjustment.DifferenceMinor != difference || adjustment.Policy != policy || adjustment.Action != action {
		return AdjustmentEvidence{}, ErrOrderConflict
	}
	return adjustment, nil
}

func abs(value int64) int64 {
	if value == -1<<63 {
		return 0
	}
	if value < 0 {
		return -value
	}
	return value
}

func (s *Store) ensureAdjustment(ctx context.Context, order ParentOrder, leg PaymentLegRecord, typ string, difference, original, eligible int64, policy, action, key string, now time.Time) (AdjustmentEvidence, bool, error) {
	var id string
	tag, err := s.db.Exec(ctx, `INSERT INTO commerce_order_adjustments(id,parent_order_id,vendor_order_id,adjustment_type,original_commitment_minor,eligible_final_minor,difference_minor,policy,action,idempotency_key,wallet_status,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'PENDING',1,$11,$11) ON CONFLICT(idempotency_key) DO NOTHING`, "coa_"+uuid.NewString(), order.ID, nullableVendor(leg.VendorOrderID), typ, original, eligible, difference, policy, action, key, now)
	if err != nil {
		return AdjustmentEvidence{}, false, err
	}
	created := tag.RowsAffected() == 1
	if !created {
		if err = s.db.QueryRow(ctx, `SELECT id FROM commerce_order_adjustments WHERE idempotency_key=$1`, key).Scan(&id); err != nil {
			return AdjustmentEvidence{}, false, err
		}
	} else {
		if err = s.db.QueryRow(ctx, `SELECT id FROM commerce_order_adjustments WHERE idempotency_key=$1`, key).Scan(&id); err != nil {
			return AdjustmentEvidence{}, false, err
		}
	}
	adj, err := s.GetAdjustment(ctx, id)
	if err != nil {
		return AdjustmentEvidence{}, false, err
	}
	if adj.ParentOrderID != order.ID || adj.VendorOrderID != leg.VendorOrderID || adj.Type != typ ||
		adj.OriginalCommitmentMinor != original || adj.EligibleFinalMinor != eligible || adj.DifferenceMinor != difference ||
		adj.Policy != policy || adj.Action != action {
		return AdjustmentEvidence{}, false, ErrOrderConflict
	}
	return adj, created, nil
}

func nullableVendor(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func (s *Store) GetAdjustment(ctx context.Context, id string) (AdjustmentEvidence, error) {
	var a AdjustmentEvidence
	var typ, action, status string
	var changeID, vendorID, ref *string
	err := s.db.QueryRow(ctx, `SELECT id,parent_order_id,change_id,vendor_order_id,adjustment_type,original_commitment_minor,eligible_final_minor,difference_minor,policy,action,idempotency_key,wallet_reference,wallet_status,version,created_at,updated_at FROM commerce_order_adjustments WHERE id=$1`, id).Scan(&a.ID, &a.ParentOrderID, &changeID, &vendorID, &typ, &a.OriginalCommitmentMinor, &a.EligibleFinalMinor, &a.DifferenceMinor, &a.Policy, &action, &a.IdempotencyKey, &ref, &status, &a.Version, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrAdjustmentNotAvailable
	}
	if err != nil {
		return a, err
	}
	if changeID != nil {
		a.ChangeID = *changeID
	}
	if vendorID != nil {
		a.VendorOrderID = *vendorID
	}
	if ref != nil {
		a.WalletReference = *ref
	}
	a.Type = typ
	a.Action = action
	a.WalletStatus = CommercialWalletStatus(status)
	return a, nil
}
func (s *Store) MarkAdjustmentRequested(ctx context.Context, id string, now time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE commerce_order_adjustments SET wallet_status='REFUND_REQUESTED',version=version+1,updated_at=$2 WHERE id=$1`, id, now)
	return err
}
func (s *Store) MarkAdjustmentWallet(ctx context.Context, id, ref string, status CommercialWalletStatus, now time.Time) (AdjustmentEvidence, error) {
	_, err := s.db.Exec(ctx, `UPDATE commerce_order_adjustments SET wallet_reference=$2,wallet_status=$3,version=version+1,updated_at=$4 WHERE id=$1`, id, ref, status, now)
	if err != nil {
		return AdjustmentEvidence{}, err
	}
	return s.GetAdjustment(ctx, id)
}

func (s *Store) ListAdjustmentsForRecovery(ctx context.Context, limit int) ([]AdjustmentEvidence, error) {
	rows, err := s.db.Query(ctx, `SELECT a.id FROM commerce_order_adjustments a WHERE a.action <> 'AUTHORIZE' AND a.wallet_status IN ('REFUND_REQUESTED','UNKNOWN') ORDER BY a.updated_at,a.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdjustmentEvidence
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		adj, loadErr := s.GetAdjustment(ctx, id)
		if loadErr != nil {
			return nil, loadErr
		}
		out = append(out, adj)
	}
	return out, rows.Err()
}

// MarkAdjustmentRefundedOnLeg records a completed refund on BOTH the
// adjustment and the payment leg it was taken from, atomically.
//
// Recording it on the leg is what keeps the schema's own over-refund guard
// meaningful. commerce_payment_legs enforces refunded + settled <= amount, but
// that guard only protects money it can see: a refund that updated the
// adjustment alone left refunded_minor understated, so a second refund path
// (a cheaper-replan benefit and a commercial-change benefit, say) could take
// the same leg twice past its own amount without the database noticing.
//
// The leg increment is conditional on the adjustment actually transitioning
// into REFUNDED, so a replay or a recovery pass moves the money exactly once.
func (s *Store) MarkAdjustmentRefundedOnLeg(
	ctx context.Context, adjustmentID, legID, reference string, amount int64, now time.Time,
) (AdjustmentEvidence, error) {
	if s == nil || s.db == nil {
		return AdjustmentEvidence{}, ErrDependencyMissing
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return AdjustmentEvidence{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Claim the transition. Only the caller that moves the adjustment into
	// REFUNDED may touch the leg.
	tag, err := tx.Exec(ctx, `
		UPDATE commerce_order_adjustments
		   SET wallet_reference=$2,wallet_status='REFUNDED',version=version+1,updated_at=$3
		 WHERE id=$1 AND wallet_status <> 'REFUNDED'`, adjustmentID, reference, now)
	if err != nil {
		return AdjustmentEvidence{}, err
	}

	if tag.RowsAffected() == 1 && legID != "" && amount > 0 {
		// The guard is in the WHERE clause: an increment that would take the
		// leg past its own authorized amount simply does not apply, and is
		// reported rather than silently dropped.
		legTag, legErr := tx.Exec(ctx, `
			UPDATE commerce_payment_legs
			   SET refunded_minor=refunded_minor+$2,
			       status=CASE WHEN refunded_minor+$2 >= amount_minor
			                   THEN 'REFUNDED' ELSE status END,
			       version=version+1,updated_at=$3
			 WHERE id=$1 AND refunded_minor+$2+settled_minor <= amount_minor`,
			legID, amount, now)
		if legErr != nil {
			return AdjustmentEvidence{}, legErr
		}
		if legTag.RowsAffected() != 1 {
			return AdjustmentEvidence{}, ErrAllocationImbalance
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return AdjustmentEvidence{}, err
	}
	return s.GetAdjustment(ctx, adjustmentID)
}
