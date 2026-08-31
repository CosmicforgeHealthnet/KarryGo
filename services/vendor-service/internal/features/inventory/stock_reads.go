package inventory

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func scanBalance(
	row interface {
		Scan(dest ...any) error
	},
) (
	Balance,
	error,
) {
	var balance Balance

	err := row.Scan(
		&balance.ID,
		&balance.BusinessID,
		&balance.BranchID,
		&balance.InventoryItemID,
		&balance.OnHandQuantity,
		&balance.ReservedQuantity,
		&balance.QuarantinedQuantity,
		&balance.InTransitQuantity,
		&balance.IncomingQuantity,
		&balance.BackorderQuantity,
		&balance.AvailableQuantity,
		&balance.ExpiredQuantity,
		&balance.Revision,
		&balance.UpdatedAt,
	)

	return balance, err
}

func loadBalanceTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	itemID string,
) (
	Balance,
	error,
) {
	balance, err :=
		scanBalance(
			tx.QueryRow(
				ctx,
				`
                SELECT
                    id,
                    business_id,
                    branch_id,
                    inventory_item_id,
                    on_hand_quantity,
                    reserved_quantity,
                    quarantined_quantity,
                    in_transit_quantity,
                    incoming_quantity,
                    backorder_quantity,
                    available_quantity,
                    expired_quantity,
                    revision,
                    updated_at
                  FROM inventory_balance_availability
                 WHERE business_id=$1
                   AND inventory_item_id=$2
                `,
				businessID,
				itemID,
			),
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return Balance{},
			ErrStockNotFound
	}

	return balance, err
}

func (s *StockService) ListBalances(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	branchID string,
	limit int,
	offset int,
) (
	[]Balance,
	error,
) {
	if err := s.requireDependencies(); err != nil {
		return nil, err
	}

	if err := s.foundation.RequireBranch(
		ctx,
		actor,
		scope,
		actor.BusinessID,
		branchID,
		false,
	); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 100
	}

	if limit > 500 {
		limit = 500
	}

	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.Query(
		ctx,
		`
        SELECT
            id,
            business_id,
            branch_id,
            inventory_item_id,
            on_hand_quantity,
            reserved_quantity,
            quarantined_quantity,
            in_transit_quantity,
            incoming_quantity,
            backorder_quantity,
            available_quantity,
            expired_quantity,
            revision,
            updated_at
          FROM inventory_balance_availability
         WHERE business_id=$1
           AND branch_id=$2
         ORDER BY inventory_item_id
         LIMIT $3
         OFFSET $4
        `,
		actor.BusinessID,
		strings.TrimSpace(branchID),
		limit,
		offset,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []Balance

	for rows.Next() {
		var item Balance

		if err := rows.Scan(
			&item.ID,
			&item.BusinessID,
			&item.BranchID,
			&item.InventoryItemID,
			&item.OnHandQuantity,
			&item.ReservedQuantity,
			&item.QuarantinedQuantity,
			&item.InTransitQuantity,
			&item.IncomingQuantity,
			&item.BackorderQuantity,
			&item.AvailableQuantity,
			&item.ExpiredQuantity,
			&item.Revision,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}

		out = append(
			out,
			item,
		)
	}

	return out, rows.Err()
}

type MovementListOptions struct {
	MovementType string
	SourceRef    string

	Limit  int
	Offset int
}

func (s *StockService) ListMovements(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	branchID string,
	opts MovementListOptions,
) (
	[]Movement,
	error,
) {
	if err := s.requireDependencies(); err != nil {
		return nil, err
	}

	if err := s.foundation.RequireBranch(
		ctx,
		actor,
		scope,
		actor.BusinessID,
		branchID,
		false,
	); err != nil {
		return nil, err
	}

	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	if opts.Limit > 500 {
		opts.Limit = 500
	}

	if opts.Offset < 0 {
		opts.Offset = 0
	}

	rows, err := s.db.Query(
		ctx,
		`
        SELECT
            id,
            business_id,
            branch_id,
            inventory_item_id,
            COALESCE(lot_id,''),
            movement_type,
            quantity,
            on_hand_delta,
            reserved_delta,
            quarantined_delta,
            in_transit_delta,
            incoming_delta,
            backorder_delta,
            source_type,
            source_ref,
            COALESCE(reason_code,''),
            COALESCE(actor_ref,''),
            occurred_at,
            inventory_revision,
            balance_revision
          FROM inventory_movements
         WHERE business_id=$1
           AND branch_id=$2
           AND (
               $3=''
               OR movement_type=$3
           )
           AND (
               $4=''
               OR source_ref=$4
           )
         ORDER BY occurred_at DESC,id DESC
         LIMIT $5
         OFFSET $6
        `,
		actor.BusinessID,
		strings.TrimSpace(branchID),
		strings.TrimSpace(opts.MovementType),
		strings.TrimSpace(opts.SourceRef),
		opts.Limit,
		opts.Offset,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []Movement

	for rows.Next() {
		var item Movement

		if err := rows.Scan(
			&item.ID,
			&item.BusinessID,
			&item.BranchID,
			&item.InventoryItemID,
			&item.LotID,
			&item.Type,
			&item.Quantity,
			&item.OnHandDelta,
			&item.ReservedDelta,
			&item.QuarantinedDelta,
			&item.InTransitDelta,
			&item.IncomingDelta,
			&item.BackorderDelta,
			&item.SourceType,
			&item.SourceRef,
			&item.ReasonCode,
			&item.ActorRef,
			&item.OccurredAt,
			&item.InventoryRevision,
			&item.BalanceRevision,
		); err != nil {
			return nil, err
		}

		out = append(
			out,
			item,
		)
	}

	return out, rows.Err()
}

func (s *StockService) ItemBranch(
	ctx context.Context,
	businessID string,
	itemID string,
) (
	string,
	error,
) {
	if s == nil || s.db == nil {
		return "", ErrDependencyMissing
	}

	var branchID string

	err := s.db.QueryRow(
		ctx,
		`
        SELECT branch_id
          FROM inventory_items
         WHERE id=$1
           AND business_id=$2
           AND status='ACTIVE'
        `,
		strings.TrimSpace(itemID),
		strings.TrimSpace(businessID),
	).Scan(
		&branchID,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return "", ErrItemNotFound
	}

	return branchID, err
}

func (s *StockService) Availability(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	itemID string,
) (
	Balance,
	error,
) {
	if err := s.requireDependencies(); err != nil {
		return Balance{}, err
	}

	branchID, err :=
		s.ItemBranch(
			ctx,
			actor.BusinessID,
			itemID,
		)

	if err != nil {
		return Balance{}, err
	}

	if err := s.foundation.RequireBranch(
		ctx,
		actor,
		scope,
		actor.BusinessID,
		branchID,
		false,
	); err != nil {
		return Balance{}, err
	}

	balance, err :=
		scanBalance(
			s.db.QueryRow(
				ctx,
				`
                SELECT
                    id,
                    business_id,
                    branch_id,
                    inventory_item_id,
                    on_hand_quantity,
                    reserved_quantity,
                    quarantined_quantity,
                    in_transit_quantity,
                    incoming_quantity,
                    backorder_quantity,
                    available_quantity,
                    expired_quantity,
                    revision,
                    updated_at
                  FROM inventory_balance_availability
                 WHERE business_id=$1
                   AND inventory_item_id=$2
                `,
				actor.BusinessID,
				itemID,
			),
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		// The item exists but has never received physical stock.
		return Balance{
			BusinessID: actor.BusinessID,

			BranchID: branchID,

			InventoryItemID: itemID,

			UpdatedAt: time.Time{},
		}, nil
	}

	return balance, err
}

func (s *StockService) Revision(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	branchID string,
) (
	int64,
	time.Time,
	error,
) {
	if err := s.requireDependencies(); err != nil {
		return 0,
			time.Time{},
			err
	}

	if err := s.foundation.RequireBranch(
		ctx,
		actor,
		scope,
		actor.BusinessID,
		branchID,
		false,
	); err != nil {
		return 0,
			time.Time{},
			err
	}

	var (
		revision  int64
		updatedAt time.Time
	)

	err := s.db.QueryRow(
		ctx,
		`
        SELECT
            revision,
            updated_at
          FROM inventory_revisions
         WHERE business_id=$1
           AND branch_id=$2
        `,
		actor.BusinessID,
		strings.TrimSpace(branchID),
	).Scan(
		&revision,
		&updatedAt,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return 0,
			time.Time{},
			nil
	}

	return revision,
		updatedAt,
		err
}
