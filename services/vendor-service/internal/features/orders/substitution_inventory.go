package orders

import (
	"context"
	"encoding/json"
	"strings"
)

// applySubstitutionInventory is deliberately a post-decision, retryable step.
// The Inventory authority's idempotency key makes a caller timeout safe, while
// the packing gate keeps the order from advancing until replacement stock is
// actually present in the reservation.
func (s *Service) applySubstitutionInventory(ctx context.Context, proposal SubstitutionProposal) error {
	order, err := s.store.GetVendorOrderForCustomer(ctx, proposal.CustomerID, proposal.VendorOrderID)
	if err != nil {
		return err
	}
	if !order.ReservationRequired || strings.TrimSpace(order.ReservationRef) == "" {
		return nil
	}
	authority, ok := s.inventory.(ReplacementInventoryAuthority)
	if !ok || authority == nil {
		return ErrDependencyMissing
	}
	var originalItem string
	var ordered int64
	var found int64
	if err = s.store.db.QueryRow(ctx, `SELECT l.inventory_item_ref,l.quantity,COALESCE(o.picked_quantity,0) FROM commerce_vendor_order_lines l LEFT JOIN commerce_vendor_order_line_outcomes o ON o.order_line_id=l.id WHERE l.id=$1 AND l.vendor_order_id=$2`, proposal.OrderLineID, proposal.VendorOrderID).Scan(&originalItem, &ordered, &found); err != nil {
		return err
	}
	var snapshot struct {
		VariantRef string `json:"variant_ref"`
	}
	if err = json.Unmarshal(proposal.ReplacementSnapshot, &snapshot); err != nil || strings.TrimSpace(snapshot.VariantRef) == "" {
		return ErrOrderConflict
	}
	replacementItem, err := authority.ResolveShopVariantItem(ctx, order.BusinessID, order.BranchID, snapshot.VariantRef)
	if err != nil {
		return err
	}
	missing := ordered - found
	if missing <= 0 {
		return nil
	}
	_, err = authority.ReplaceReservationAllocation(ctx, order.BusinessID, order.BranchID, InventoryReplacementRequest{
		ReservationRef: order.ReservationRef, SourceType: "CHECKOUT", SourceRef: order.ReservationSourceRef,
		OriginalItemRef: originalItem, OriginalOrderedQty: ordered, OriginalFoundQty: found,
		MissingQty: missing, ReplacementItemRef: replacementItem, ReplacementQty: missing,
		IdempotencyKey: "substitution-inventory-" + proposal.ID, CorrelationID: proposal.ID,
	})
	return err
}

// RecoverApprovedSubstitutionInventory replays the post-approval Inventory
// boundary after a caller or worker crash. Inventory idempotency guarantees
// that this never duplicates a release, reservation or movement.
func (s *Service) RecoverApprovedSubstitutionInventory(ctx context.Context, limit int) (int, error) {
	if s == nil || s.store == nil {
		return 0, ErrDependencyMissing
	}
	proposals, err := s.store.ListApprovedSubstitutions(ctx, limit)
	if err != nil {
		return 0, err
	}
	var recovered int
	for _, proposal := range proposals {
		if err = s.applySubstitutionInventory(ctx, proposal); err == nil {
			recovered++
		}
	}
	return recovered, nil
}
