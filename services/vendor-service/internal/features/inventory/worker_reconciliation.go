package inventory

import "context"

// ReconcileBalances detects authoritative balance invariant violations.
// It never silently rewrites quantities.
func (s *StockService) ReconcileBalances(
	ctx context.Context,
	limit int,
) (
	int,
	error,
) {
	if s == nil || s.db == nil {
		return 0, ErrDependencyMissing
	}

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var mismatches int

	err :=
		s.db.QueryRow(
			ctx,
			`SELECT count(*)
             FROM (
                SELECT 1
                FROM inventory_balances
                WHERE
                    on_hand_quantity < 0
                    OR reserved_quantity < 0
                    OR quarantined_quantity < 0
                    OR reserved_quantity +
                       quarantined_quantity >
                       on_hand_quantity
                ORDER BY updated_at, inventory_item_id
                LIMIT $1
             ) q`,
			limit,
		).Scan(
			&mismatches,
		)

	if err != nil {
		return 0, err
	}

	if mismatches > 0 {
		_,
			err =
			s.GenerateInventoryAlerts(
				ctx,
				limit,
			)
	}

	return mismatches, err
}

// ReconcileBackorders exposes outstanding Inventory-owned backorder state.
// It does not create/cancel Orders or create purchasing authority.
func (s *StockService) ReconcileBackorders(
	ctx context.Context,
	limit int,
) (
	int,
	error,
) {
	if s == nil || s.db == nil {
		return 0, ErrDependencyMissing
	}

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var outstanding int

	err :=
		s.db.QueryRow(
			ctx,
			`SELECT count(*)
             FROM (
                SELECT 1
                FROM inventory_balances
                WHERE backorder_quantity > 0
                ORDER BY updated_at, inventory_item_id
                LIMIT $1
             ) q`,
			limit,
		).Scan(
			&outstanding,
		)

	if err != nil {
		return 0, err
	}

	if outstanding > 0 {
		_,
			err =
			s.GenerateInventoryAlerts(
				ctx,
				limit,
			)
	}

	return outstanding, err
}
