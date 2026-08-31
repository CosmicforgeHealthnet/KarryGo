package orders

import (
	"context"
	"errors"
	"time"
)

type OrderAdjustmentEvidence struct {
	ID                     string    `json:"id"`
	ParentOrderID          string    `json:"parent_order_id"`
	VendorOrderID          string    `json:"vendor_order_id"`
	OrderLineID            string    `json:"order_line_id"`
	CaseID                 string    `json:"case_id,omitempty"`
	ProposalID             string    `json:"proposal_id,omitempty"`
	CommercialReference    string    `json:"commercial_reference,omitempty"`
	AdjustmentType         string    `json:"adjustment_type"`
	OriginalAmountMinor    int64     `json:"original_amount_minor"`
	ReplacementAmountMinor int64     `json:"replacement_amount_minor"`
	DifferenceMinor        int64     `json:"difference_minor"`
	Quantity               int64     `json:"quantity"`
	FundingEffect          string    `json:"funding_effect,omitempty"`
	InventoryEffect        string    `json:"inventory_effect,omitempty"`
	CustomerDecision       string    `json:"customer_decision,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	CorrelationID          string    `json:"correlation_id,omitempty"`
	IdempotencyKey         string    `json:"idempotency_key,omitempty"`
}

type FinalReceiptEvidence struct {
	EstimatedAmountMinor     int64  `json:"estimated_amount_minor"`
	MaxAuthorizedAmountMinor int64  `json:"max_authorized_amount_minor"`
	OriginalAmountMinor      int64  `json:"original_amount_minor"`
	ReplacementAmountMinor   int64  `json:"replacement_amount_minor"`
	ActualAmountMinor        int64  `json:"actual_amount_minor"`
	RefundAmountMinor        int64  `json:"refund_amount_minor"`
	ReapprovalReference      string `json:"reapproval_reference,omitempty"`
	FinalTotalMinor          int64  `json:"final_total_minor"`
}

// FinalReceiptLine is a bounded, customer-scoped reconstruction of the
// original agreement and the fulfilled result. It deliberately reads the
// immutable order line plus append-only operational evidence; it never edits
// the original quote to make the final outcome look pre-approved.
type FinalReceiptLine struct {
	OrderLineID              string `json:"order_line_id"`
	OriginalItemRef          string `json:"original_item_ref"`
	OriginalQuantity         int64  `json:"original_quantity"`
	OriginalAmountMinor      int64  `json:"original_amount_minor"`
	ReplacementItemRef       string `json:"replacement_item_ref,omitempty"`
	ReplacementAmountMinor   int64  `json:"replacement_amount_minor"`
	EstimatedQuantity        int64  `json:"estimated_quantity"`
	ActualQuantity           int64  `json:"actual_quantity"`
	EstimatedAmountMinor     int64  `json:"estimated_amount_minor"`
	MaxAuthorizedAmountMinor int64  `json:"max_authorized_amount_minor"`
	FinalAmountMinor         int64  `json:"final_amount_minor"`
	RefundAmountMinor        int64  `json:"refund_amount_minor"`
	SubstitutionDecision     string `json:"substitution_decision,omitempty"`
	CaseState                string `json:"case_state,omitempty"`
	ProposalStatus           string `json:"proposal_status,omitempty"`
	CustomerPayableMinor     int64  `json:"customer_payable_minor"`
	VendorPayableMinor       int64  `json:"vendor_payable_minor"`
}

// FinalReceiptForCustomer enforces the customer/vendor-order ownership guard
// in SQL, preventing nested-id IDOR while retaining both original and final
// commercial evidence.
func (s *Store) FinalReceiptForCustomer(ctx context.Context, customerID, vendorOrderID string) ([]FinalReceiptLine, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx, `
		SELECT l.id,l.item_ref,l.quantity,l.quoted_total_minor,
		       COALESCE(p.proposed_item_ref,''),COALESCE(p.proposed_price_minor,0),
		       COALESCE(vm.estimated_quantity,0),COALESCE(vm.actual_quantity,0),
		       COALESCE(vm.estimated_amount_minor,l.quoted_total_minor),COALESCE(vm.hard_max_quantity*vm.unit_price_minor,l.max_authorized_total_minor),
		       COALESCE(vm.final_amount_minor,0),
		       COALESCE(a.difference_minor,0),COALESCE(c.state,''),COALESCE(p.status,''),
		       COALESCE(v.customer_payable_minor,0),COALESCE(v.vendor_payable_minor,0)
		  FROM commerce_vendor_order_lines l
		  JOIN commerce_vendor_orders v ON v.id=l.vendor_order_id AND v.parent_order_id=l.parent_order_id
		  LEFT JOIN commerce_variable_measurements vm ON vm.order_line_id=l.id
		  LEFT JOIN LATERAL (
			SELECT p.* FROM commerce_substitution_proposals p
			 WHERE p.order_line_id=l.id ORDER BY p.created_at DESC,p.id DESC LIMIT 1
		  ) p ON true
		  LEFT JOIN commerce_substitution_cases c ON c.id=p.case_id
		  LEFT JOIN LATERAL (
			SELECT oa.difference_minor FROM commerce_order_adjustment_evidence oa
			 WHERE oa.order_line_id=l.id ORDER BY oa.created_at DESC,oa.id DESC LIMIT 1
		  ) a ON true
		 WHERE l.vendor_order_id=$1 AND v.customer_id=$2
		 ORDER BY l.id`, vendorOrderID, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]FinalReceiptLine, 0)
	for rows.Next() {
		var item FinalReceiptLine
		var diff int64
		if err = rows.Scan(&item.OrderLineID, &item.OriginalItemRef, &item.OriginalQuantity, &item.OriginalAmountMinor,
			&item.ReplacementItemRef, &item.ReplacementAmountMinor, &item.EstimatedQuantity, &item.ActualQuantity,
			&item.EstimatedAmountMinor, &item.MaxAuthorizedAmountMinor, &item.FinalAmountMinor, &diff, &item.CaseState,
			&item.ProposalStatus, &item.CustomerPayableMinor, &item.VendorPayableMinor); err != nil {
			return nil, err
		}
		if item.ReplacementItemRef != "" {
			item.SubstitutionDecision = item.ProposalStatus
		}
		if diff < 0 {
			item.RefundAmountMinor = -diff
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		var exists bool
		if err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_vendor_orders WHERE id=$1 AND customer_id=$2)`, vendorOrderID, customerID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrOrderNotFound
		}
	}
	return result, nil
}

func BuildSubstitutionAdjustment(caseID, proposalID, parentID, vendorID, lineID string, original, replacement, quantity int64, decision, commercialRef, inventoryEffect, idempotencyKey string, now time.Time) (OrderAdjustmentEvidence, error) {
	if caseID == "" || parentID == "" || vendorID == "" || lineID == "" || idempotencyKey == "" || original < 0 || replacement < 0 || quantity < 0 {
		return OrderAdjustmentEvidence{}, errors.New("invalid substitution adjustment evidence")
	}
	return OrderAdjustmentEvidence{ID: "adj_" + idempotencyKey, ParentOrderID: parentID, VendorOrderID: vendorID, OrderLineID: lineID, CaseID: caseID, ProposalID: proposalID, AdjustmentType: "SUBSTITUTION", OriginalAmountMinor: original, ReplacementAmountMinor: replacement, DifferenceMinor: replacement - original, Quantity: quantity, InventoryEffect: inventoryEffect, CustomerDecision: decision, CommercialReference: commercialRef, IdempotencyKey: idempotencyKey, CreatedAt: now.UTC()}, nil
}
