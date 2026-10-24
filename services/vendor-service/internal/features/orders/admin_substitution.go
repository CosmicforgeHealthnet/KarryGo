package orders

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// TransactionalReplacementInventoryAuthority is implemented only by the local
// Vendor-owned Inventory adapter. Remote/independently committing adapters
// cannot participate in this atomic administrative command.
type TransactionalReplacementInventoryAuthority interface {
	ReplacementInventoryAuthority
	ReplaceReservationAllocationInTx(context.Context, pgx.Tx, string, string, InventoryReplacementRequest) (InventoryReplacementResult, error)
}

// ProcessApprovedSubstitutionInTx processes an existing customer's decision.
// It cannot accept an approval, replacement identity, quantity or price input.
// The caller must supply the exact branch scope it authorized and locked in tx;
// resolving staff scope through the pool while tx is held can exhaust the pool
// when commands for the same subject arrive concurrently.
func (s *Service) ProcessApprovedSubstitutionInTx(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, id string, expected int64) (int64, string, error) {
	if !s.enabled || actor.StaffID == "" || actor.VendorID == "" || scope.Empty() {
		return 0, "", ErrForbidden
	}
	var err error
	var customer string
	if err = tx.QueryRow(ctx, `SELECT customer_id FROM commerce_substitution_proposals WHERE id=$1 AND business_id=$2`, id, actor.VendorID).Scan(&customer); err != nil {
		return 0, "", err
	}
	p, err := scanSubstitution(tx.QueryRow(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,response_ref,version,created_at,responded_at,updated_at,COALESCE(case_id,'') FROM commerce_substitution_proposals WHERE id=$1 AND customer_id=$2 AND business_id=$3 FOR UPDATE`, id, customer, actor.VendorID))
	if err != nil {
		return 0, "", err
	}
	if !scope.Covers(p.BranchID) {
		return 0, "", ErrOrderNotFound
	}
	var processingState string
	if err = tx.QueryRow(ctx, `SELECT admin_inventory_processing_state FROM commerce_substitution_proposals WHERE id=$1`, id).Scan(&processingState); err != nil {
		return 0, "", err
	}
	if processingState != "NOT_RECORDED" {
		return 0, "", ErrOrderConflict
	}
	if p.Version != expected {
		return 0, "", ErrVersionConflict
	}
	if p.Status != SubstitutionApproved || p.ResponseRef != p.CustomerID {
		return 0, "", ErrOrderConflict
	}
	order, err := loadVendorOrderBy(ctx, tx, `v.id=$1 AND v.business_id=$2 FOR UPDATE OF v`, p.VendorOrderID, actor.VendorID)
	if err != nil {
		return 0, "", err
	}
	if order.CustomerID != p.CustomerID || order.BranchID != p.BranchID || order.ParentOrderID != p.ParentOrderID {
		return 0, "", ErrOrderNotFound
	}
	if order.Status != VendorStatusPicking && order.Status != VendorStatusSubstitutionReq && order.Status != VendorStatusAwaitingCustomer {
		return 0, "", ErrOrderConflict
	}
	if !order.ReservationRequired || order.ReservationRef == "" {
		return 0, "", ErrDependencyMissing
	}
	authority, ok := s.inventory.(TransactionalReplacementInventoryAuthority)
	if !ok {
		return 0, "", ErrDependencyMissing
	}
	request, needed, err := substitutionInventoryRequest(ctx, tx, authority, order, p)
	if err != nil {
		return 0, "", err
	}
	processing := "NOT_REQUIRED"
	if needed {
		if _, err = authority.ReplaceReservationAllocationInTx(ctx, tx, order.BusinessID, order.BranchID, request); err != nil {
			return 0, "", err
		}
		processing = "APPLIED"
	}
	tag, err := tx.Exec(ctx, `UPDATE commerce_substitution_proposals SET admin_inventory_processing_state=$2,admin_inventory_processed_at=now(),admin_inventory_operation_key=$3,version=version+1,updated_at=now() WHERE id=$1 AND version=$4 AND status='APPROVED'`, id, processing, "substitution-inventory-"+p.ID, expected)
	if err != nil {
		return 0, "", err
	}
	if tag.RowsAffected() != 1 {
		return 0, "", ErrVersionConflict
	}
	return p.Version, string(p.Status), nil
}
