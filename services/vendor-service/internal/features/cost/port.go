package cost

// The Section 15 read boundary.
//
// Section 16D never reaches into purchasing's service internals and never
// copies its history. It asks this port two questions - "what did we last pay
// for this?" and "what have we recently paid for this?" - and Section 15's
// table answers them.
//
// ELIGIBILITY, derived from the Section 15 write path rather than assumed
// (internal/features/purchasing/goods_receiving_service.go):
//
//   - An OBSERVED row is written only for a receipt line that was actually
//     accepted: AcceptedQuantity > 0, not flagged WrongItem, with a supplier
//     product. Draft and cancelled purchase orders, unconfirmed deliveries,
//     rejected and damaged quantities and quotation-only data never produce a
//     row at all, so eligibility is a property of the table, not a filter this
//     package has to reimplement.
//
//   - A REVERSAL row is written when a confirmed receipt is reversed. It
//     carries the SAME positive magnitudes as the observation it undoes, not a
//     negative amount, and the table's UNIQUE(business_id,
//     goods_receipt_line_id, evidence_type) permits at most one of each per
//     receipt line. A reversal therefore CANCELS its observation; it does not
//     subtract from a running total. Netting is consequently "exclude any
//     observation whose receipt line also has a reversal", which is what the
//     NOT EXISTS below does. Treating reversals as negative quantities would
//     corrupt the weighted average, and ignoring them entirely would keep
//     reversed evidence in the cost basis - the regression test covers both
//     mistakes.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PurchasingCostReader is the whole of Section 16D's dependency on Section 15.
type PurchasingCostReader interface {
	// LatestAccepted returns the most recent accepted, unreversed observation
	// for one inventory item in one branch, or ok=false when there is none.
	LatestAccepted(
		ctx context.Context,
		businessID, branchID, inventoryItemID string,
	) (PurchaseObservation, bool, error)

	// RecentAccepted returns up to limit most-recent accepted, unreversed
	// observations, newest first. The limit is mandatory: an unbounded scan of
	// a vendor's purchasing history is exactly what this section must not do.
	RecentAccepted(
		ctx context.Context,
		businessID, branchID, inventoryItemID string,
		limit int,
	) ([]PurchaseObservation, error)

	// LatestAcceptedBatch resolves many inventory items in ONE query. Costing a
	// recipe with forty components must not become forty round trips; every
	// caller in this package uses this rather than looping over LatestAccepted.
	LatestAcceptedBatch(
		ctx context.Context,
		businessID, branchID string,
		inventoryItemIDs []string,
	) (map[string]PurchaseObservation, error)
}

// PostgresPurchasingCostReader reads Section 15's authoritative table.
type PostgresPurchasingCostReader struct {
	db *pgxpool.Pool
}

// NewPostgresPurchasingCostReader builds the Section 15 adapter.
func NewPostgresPurchasingCostReader(db *pgxpool.Pool) *PostgresPurchasingCostReader {
	return &PostgresPurchasingCostReader{db: db}
}

// acceptedEvidencePredicate is the eligibility rule, in one place so latest,
// recent and batch reads cannot diverge on what "accepted" means.
const acceptedEvidencePredicate = `
	    h.business_id = $1
	    AND h.branch_id = $2
	    AND h.evidence_type = 'OBSERVED'
	    AND h.inventory_unit_cost_minor IS NOT NULL
	    AND NOT EXISTS (
	        SELECT 1
	          FROM vendor_purchase_cost_history r
	         WHERE r.business_id = h.business_id
	           AND r.goods_receipt_line_id = h.goods_receipt_line_id
	           AND r.evidence_type = 'REVERSAL'
	    )`

const observationColumns = `
	    h.id,
	    h.inventory_item_id,
	    h.inventory_quantity,
	    h.inventory_unit_cost_minor,
	    h.currency,
	    h.observed_at,
	    h.goods_receipt_id,
	    h.goods_receipt_line_id,
	    h.purchase_order_id,
	    h.supplier_id`

// LatestAccepted implements PurchasingCostReader.
//
// Ordering is observed_at DESC, id DESC. The id tie-break is not decoration:
// two receipts confirmed in the same transaction share a now() timestamp, and
// without a stable second key the "latest" cost would vary between calls and a
// snapshot would stop being reproducible.
func (r *PostgresPurchasingCostReader) LatestAccepted(
	ctx context.Context,
	businessID, branchID, inventoryItemID string,
) (PurchaseObservation, bool, error) {
	rows, err := r.db.Query(ctx, `
		SELECT`+observationColumns+`
		  FROM vendor_purchase_cost_history h
		 WHERE`+acceptedEvidencePredicate+`
		   AND h.inventory_item_id = $3
		 ORDER BY h.observed_at DESC, h.id DESC
		 LIMIT 1`, businessID, branchID, inventoryItemID)
	if err != nil {
		return PurchaseObservation{}, false, err
	}
	defer rows.Close()

	if !rows.Next() {
		return PurchaseObservation{}, false, rows.Err()
	}
	observation, err := scanObservation(rows)
	if err != nil {
		return PurchaseObservation{}, false, err
	}
	return observation, true, rows.Err()
}

// RecentAccepted implements PurchasingCostReader.
func (r *PostgresPurchasingCostReader) RecentAccepted(
	ctx context.Context,
	businessID, branchID, inventoryItemID string,
	limit int,
) ([]PurchaseObservation, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows, err := r.db.Query(ctx, `
		SELECT`+observationColumns+`
		  FROM vendor_purchase_cost_history h
		 WHERE`+acceptedEvidencePredicate+`
		   AND h.inventory_item_id = $3
		 ORDER BY h.observed_at DESC, h.id DESC
		 LIMIT $4`, businessID, branchID, inventoryItemID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var observations []PurchaseObservation
	for rows.Next() {
		observation, scanErr := scanObservation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		observations = append(observations, observation)
	}
	return observations, rows.Err()
}

// LatestAcceptedBatch implements PurchasingCostReader.
//
// DISTINCT ON gives the newest surviving observation per item in a single
// index-ordered pass, which is what keeps recipe costing off the N+1 path.
func (r *PostgresPurchasingCostReader) LatestAcceptedBatch(
	ctx context.Context,
	businessID, branchID string,
	inventoryItemIDs []string,
) (map[string]PurchaseObservation, error) {
	result := map[string]PurchaseObservation{}
	if len(inventoryItemIDs) == 0 {
		return result, nil
	}

	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ON (h.inventory_item_id)`+observationColumns+`
		  FROM vendor_purchase_cost_history h
		 WHERE`+acceptedEvidencePredicate+`
		   AND h.inventory_item_id = ANY($3)
		 ORDER BY h.inventory_item_id, h.observed_at DESC, h.id DESC`,
		businessID, branchID, inventoryItemIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		observation, scanErr := scanObservation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result[observation.InventoryItemID] = observation
	}
	return result, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanObservation(row scannable) (PurchaseObservation, error) {
	var observation PurchaseObservation
	err := row.Scan(
		&observation.EvidenceRef,
		&observation.InventoryItemID,
		&observation.InventoryQuantity,
		&observation.InventoryUnitCostMinor,
		&observation.Currency,
		&observation.ObservedAt,
		&observation.GoodsReceiptID,
		&observation.GoodsReceiptLineID,
		&observation.PurchaseOrderID,
		&observation.SupplierID,
	)
	return observation, err
}
