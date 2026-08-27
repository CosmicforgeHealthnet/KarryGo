package orders

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// PickingQueue returns the immutable line snapshot plus current operational
// evidence. It never rehydrates a line from the live Shop catalogue.
func (s *Service) PickingQueue(ctx context.Context, actor Actor, vendorOrderID string) ([]ShopPickingLine, error) {
	if !s.enabled {
		return nil, ErrDisabled
	}
	scope, err := s.authorize(ctx, actor, PermOrdersRead)
	if err != nil {
		return nil, err
	}
	order, err := s.store.GetVendorOrderForVendor(ctx, actor.VendorID, strings.TrimSpace(vendorOrderID))
	if err != nil {
		return nil, err
	}
	if !scope.Covers(order.BranchID) || order.Domain != "SHOP" {
		return nil, ErrOrderNotFound
	}
	rows, err := s.store.db.Query(ctx, `SELECT l.parent_order_id,l.vendor_order_id,l.id,l.quantity,l.variant_ref,l.sell_unit_ref,
		COALESCE(p.measurement_mode,'FIXED_UNIT'),COALESCE(p.estimated_variable_quantity,0),
		COALESCE(p.substitution_policy,'EXACT_ONLY'),COALESCE(p.quality_preference_snapshot,'[]'::jsonb),COALESCE(p.preselected_replacement_ref,''),
		COALESCE(o.picked_quantity,0),COALESCE(o.picking_state,'PENDING'),COALESCE(o.version,1),v.version
		FROM commerce_vendor_order_lines l JOIN commerce_vendor_orders v ON v.id=l.vendor_order_id
		LEFT JOIN commerce_shop_picking_snapshots p ON p.order_line_id=l.id
		LEFT JOIN commerce_vendor_order_line_outcomes o ON o.order_line_id=l.id
		WHERE l.vendor_order_id=$1 ORDER BY l.id`, order.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ShopPickingLine, 0)
	for rows.Next() {
		var p ShopPickingLine
		var policy string
		var raw json.RawMessage
		var state string
		if err = rows.Scan(&p.ParentOrderID, &p.VendorOrderID, &p.OrderLineID, &p.OrderedQuantity, &p.VariantRef, &p.SellUnitRef, &p.MeasurementMode, &p.EstimatedVariableQuantity, &policy, &raw, &p.PreselectedReplacementRef, &p.QuantityFound, &state, &p.Version, &p.ExpectedVendorOrderVersion); err != nil {
			return nil, err
		}
		p.RemainingQuantity = p.OrderedQuantity - p.QuantityFound
		p.SubstitutionPolicy = SubstitutionPolicy(policy)
		p.State = PickingState(state)
		if len(raw) > 0 && string(raw) != "[]" {
			_ = json.Unmarshal(raw, &p.QualityPreferences)
		}
		result = append(result, p)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// RecordQualityResult writes picker evidence under the line outcome OCC lock.
// A quality failure is evidence only; it does not silently choose a substitute.
func (s *Service) RecordQualityResult(ctx context.Context, actor Actor, vendorOrderID, lineID string, expectedVersion int64, result QualityResult, observed, reason string) error {
	if !s.enabled {
		return ErrDisabled
	}
	if err := ValidateQualityResult(result); err != nil {
		return ErrOrderConflict
	}
	scope, err := s.authorize(ctx, actor, PermOrdersManage)
	if err != nil {
		return err
	}
	order, err := s.store.GetVendorOrderForVendor(ctx, actor.VendorID, vendorOrderID)
	if err != nil || !scope.Covers(order.BranchID) {
		return ErrOrderNotFound
	}
	tx, err := s.store.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var outcome string
	if err = tx.QueryRow(ctx, `SELECT outcome FROM commerce_vendor_order_line_outcomes WHERE vendor_order_id=$1 AND order_line_id=$2 AND version=$3 FOR UPDATE`, vendorOrderID, lineID, expectedVersion).Scan(&outcome); errors.Is(err, pgx.ErrNoRows) {
		return ErrVersionConflict
	} else if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_vendor_order_line_outcomes SET quality_result=$1,version=version+1,updated_at=$2 WHERE vendor_order_id=$3 AND order_line_id=$4 AND version=$5`, result, s.now().UTC(), vendorOrderID, lineID, expectedVersion); err != nil {
		return err
	}
	var parentID, customerID, lifecycleStatus string
	if err = tx.QueryRow(ctx, `SELECT v.parent_order_id,v.customer_id,v.status FROM commerce_vendor_orders v WHERE v.id=$1`, vendorOrderID).Scan(&parentID, &customerID, &lifecycleStatus); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO commerce_order_line_quality_results(
			id,preference_id,order_line_id,result,observed_value,reason,picker_ref)
		SELECT 'qres_'||gen_random_uuid()::text,p.id,$1,$2,$3,$4,$5
		  FROM commerce_order_line_quality_preferences p
		 WHERE p.order_line_id=$1
		ON CONFLICT(preference_id) DO UPDATE SET
			result=EXCLUDED.result,observed_value=EXCLUDED.observed_value,
			reason=EXCLUDED.reason,picker_ref=EXCLUDED.picker_ref`,
		lineID, result, observed, reason, actor.StaffID)
	if err != nil {
		return err
	}
	if err = appendEvent(ctx, tx, s.store.outbox, parentID, vendorOrderID, customerID, "commerce.vendor_order.quality_recorded", "VENDOR", actor.StaffID, lifecycleStatus, lifecycleStatus, s.now().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RecordActualQuantity records a variable line's measured quantity and exact
// integer amount. The Wallet cap is checked before the row may become final.
func (s *Service) RecordActualQuantity(ctx context.Context, actor Actor, vendorOrderID, lineID string, expectedVersion, actual, authorizedMax int64) (VariableMeasurement, error) {
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
	tx, err := s.store.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return VariableMeasurement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var m VariableMeasurement
	err = tx.QueryRow(ctx, `SELECT measurement_mode,estimated_quantity,unit_price_minor,estimated_amount_minor,tolerance_quantity,hard_max_quantity,reserved_upper_quantity,actual_quantity,final_amount_minor,financial_state FROM commerce_variable_measurements WHERE vendor_order_id=$1 AND order_line_id=$2 AND version=$3 FOR UPDATE`, vendorOrderID, lineID, expectedVersion).Scan(&m.Mode, &m.EstimatedQuantity, &m.UnitPriceMinor, &m.EstimatedAmountMinor, &m.ToleranceQuantity, &m.HardMaxQuantity, &m.ReservedUpperQuantity, &m.ActualQuantity, &m.FinalAmountMinor, &m.State)
	if errors.Is(err, pgx.ErrNoRows) {
		return VariableMeasurement{}, ErrOrderNotFound
	}
	if err != nil {
		return VariableMeasurement{}, err
	}
	m, err = m.RecordActual(actual)
	if err != nil {
		return VariableMeasurement{}, ErrOrderConflict
	}
	// The caller's line cap is necessary but not sufficient. Re-derive the
	// parent authorization from the locked order aggregate so two individually
	// valid variable lines cannot silently over-consume one parent payment cap.
	var parentCap, fixedOther, variableOther int64
	var parentID string
	if err = tx.QueryRow(ctx, `SELECT parent_order_id FROM commerce_vendor_orders WHERE id=$1`, vendorOrderID).Scan(&parentID); err != nil {
		return VariableMeasurement{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT max_authorized_total_minor FROM commerce_parent_orders WHERE id=$1 FOR UPDATE`, parentID).Scan(&parentCap); err != nil {
		return VariableMeasurement{}, err
	}
	if err = tx.QueryRow(ctx, `
		SELECT COALESCE((SELECT SUM(l.quoted_total_minor)
		                   FROM commerce_vendor_order_lines l
		                  WHERE l.parent_order_id=$1
		                    AND NOT EXISTS (SELECT 1 FROM commerce_variable_measurements vm WHERE vm.order_line_id=l.id)),0),
		       COALESCE((SELECT SUM(CASE WHEN vm.financial_state IN ('ACTUAL_RECORDED','FINALIZED','REAPPROVAL_REQUIRED','FINANCIAL_PENDING')
		                                THEN vm.final_amount_minor ELSE vm.estimated_amount_minor END)
		                   FROM commerce_variable_measurements vm
		                  WHERE vm.parent_order_id=$1 AND vm.order_line_id<>$2),0)`, parentID, lineID).Scan(&fixedOther, &variableOther); err != nil {
		return VariableMeasurement{}, err
	}
	if m.FinalAmountMinor > authorizedMax || !ParentCapAllows(parentCap, fixedOther, variableOther+m.FinalAmountMinor, 0, 0, 0, 0) {
		m.State = FinancialReapprovalRequired
	}
	tag, err := tx.Exec(ctx, `UPDATE commerce_variable_measurements SET actual_quantity=$1,final_amount_minor=$2,financial_state=$3,version=version+1,updated_at=$4 WHERE vendor_order_id=$5 AND order_line_id=$6 AND version=$7`, m.ActualQuantity, m.FinalAmountMinor, m.State, s.now().UTC(), vendorOrderID, lineID, expectedVersion)
	if err != nil {
		return VariableMeasurement{}, err
	}
	if tag.RowsAffected() != 1 {
		return VariableMeasurement{}, ErrVersionConflict
	}
	var customerID, lifecycleStatus string
	if err = tx.QueryRow(ctx, `SELECT v.parent_order_id,v.customer_id,v.status FROM commerce_vendor_orders v WHERE v.id=$1`, vendorOrderID).Scan(&parentID, &customerID, &lifecycleStatus); err != nil {
		return VariableMeasurement{}, err
	}
	if err = appendEvent(ctx, tx, s.store.outbox, parentID, vendorOrderID, customerID, "commerce.vendor_order.variable_measurement_recorded", "VENDOR", actor.StaffID, lifecycleStatus, lifecycleStatus, s.now().UTC()); err != nil {
		return VariableMeasurement{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return VariableMeasurement{}, err
	}
	return m, nil
}

// CompletePacking applies the existing PACK transition only after all new
// picking gates have passed. The transition remains the sole lifecycle owner.
func (s *Service) CompletePacking(ctx context.Context, actor Actor, vendorOrderID string, expectedVersion int64) (VendorOrder, ParentOrder, error) {
	if !s.enabled {
		return VendorOrder{}, ParentOrder{}, ErrDisabled
	}
	_, err := s.store.GetVendorOrderForVendor(ctx, actor.VendorID, vendorOrderID)
	if err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	if _, err = s.PickingQueue(ctx, actor, vendorOrderID); err != nil {
		return VendorOrder{}, ParentOrder{}, err
	}
	return s.Advance(ctx, actor, vendorOrderID, TransitionPack, "", expectedVersion)
}

// ExpireSubstitutionCases is a bounded, idempotent recovery operation. It
// moves only expired unresolved cases and leaves customer-visible history.
func (s *Store) ExpireSubstitutionCases(ctx context.Context, now time.Time, limit int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrDependencyMissing
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	tag, err := s.db.Exec(ctx, `UPDATE commerce_substitution_cases SET state=CASE WHEN fallback_policy='AUTO_REFUND' THEN 'AUTO_REFUND_PENDING' ELSE 'EXPIRED' END,version=version+1,updated_at=$1,resolved_at=CASE WHEN fallback_policy='AUTO_REFUND' THEN NULL ELSE $1 END WHERE id IN (SELECT id FROM commerce_substitution_cases WHERE state IN ('OPEN','AWAITING_CUSTOMER') AND deadline_at IS NOT NULL AND deadline_at <= $1 ORDER BY deadline_at,id LIMIT $2)`, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ExpireSubstitutionProposals advances the existing Section 18 proposal
// authority and closes its case only when no other pending proposal remains.
// The statement is idempotent and safe for duplicate worker executions.
func (s *Store) ExpireSubstitutionProposals(ctx context.Context, now time.Time, limit int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, ErrDependencyMissing
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `
		WITH due AS (
			SELECT id,case_id
			  FROM commerce_substitution_proposals
			 WHERE status='PENDING' AND expires_at IS NOT NULL AND expires_at <= $1
			 ORDER BY expires_at,id LIMIT $2 FOR UPDATE SKIP LOCKED
		), changed AS (
			UPDATE commerce_substitution_proposals p
			   SET status='EXPIRED',version=version+1,responded_at=$1,updated_at=$1
			  FROM due WHERE p.id=due.id
			 RETURNING p.id,p.case_id
		)
		SELECT id,case_id FROM changed`, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	var count int64
	var caseIDs []string
	for rows.Next() {
		var id, caseID string
		if err = rows.Scan(&id, &caseID); err != nil {
			rows.Close()
			return 0, err
		}
		count++
		if caseID != "" {
			caseIDs = append(caseIDs, caseID)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, caseID := range caseIDs {
		if _, err = tx.Exec(ctx, `
			UPDATE commerce_substitution_cases c
			   SET state='EXPIRED',version=version+1,updated_at=$1,resolved_at=$1
			 WHERE c.id=$2 AND c.state IN ('OPEN','AWAITING_CUSTOMER')
			   AND NOT EXISTS (SELECT 1 FROM commerce_substitution_proposals p WHERE p.case_id=c.id AND p.status='PENDING')`, now.UTC(), caseID); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}
