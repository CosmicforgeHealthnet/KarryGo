package inventory

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresInventoryItemStore persists generic Inventory item identity.
//
// It does not read or mutate Shop inventory balances.
type PostgresInventoryItemStore struct {
	pool *pgxpool.Pool
}

func NewPostgresInventoryItemStore(
	pool *pgxpool.Pool,
) *PostgresInventoryItemStore {
	return &PostgresInventoryItemStore{
		pool: pool,
	}
}

func (s *PostgresInventoryItemStore) CreateInventoryItem(
	ctx context.Context,
	item InventoryItem,
) (
	InventoryItem,
	error,
) {
	if s == nil || s.pool == nil {
		return InventoryItem{}, ErrDependencyMissing
	}

	tx, err := s.pool.BeginTx(
		ctx,
		pgx.TxOptions{
			IsoLevel: pgx.ReadCommitted,
		},
	)
	if err != nil {
		return InventoryItem{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var out InventoryItem

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO inventory_items (
            id,
            business_id,
            branch_id,
            item_type,
            source_ref,
            stock_measure_ref,
            stock_measure_owner_key,
            stock_unit_kind,
            stock_basis_mode,
            tracking_mode,
            lot_policy,
            lot_tracking_enabled,
            status,
            version
        )
        VALUES (
            $1,$2,$3,$4,$5,$6,$7,$8,
            $9,$10,$11,$12,$13,$14
        )
        RETURNING
            id,
            business_id,
            branch_id,
            item_type,
            source_ref,
            stock_measure_ref,
            stock_measure_owner_key,
            stock_unit_kind,
            stock_basis_mode,
            tracking_mode,
            lot_policy,
            lot_tracking_enabled,
            status,
            version,
            created_at,
            updated_at
        `,
		item.ID,
		item.BusinessID,
		item.BranchID,
		item.ItemType,
		item.SourceRef,
		item.StockMeasureRef,
		item.StockMeasureOwnerKey,
		item.StockUnitKind,
		item.StockBasisMode,
		item.TrackingMode,
		item.LotPolicy,
		item.LotTrackingEnabled,
		item.Status,
		item.Version,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.ItemType,
		&out.SourceRef,
		&out.StockMeasureRef,
		&out.StockMeasureOwnerKey,
		&out.StockUnitKind,
		&out.StockBasisMode,
		&out.TrackingMode,
		&out.LotPolicy,
		&out.LotTrackingEnabled,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" {
			return InventoryItem{}, ErrDuplicateItem
		}

		return InventoryItem{}, err
	}

	_, err = tx.Exec(
		ctx,
		`
        INSERT INTO inventory_revisions (
            business_id,
            branch_id,
            revision,
            updated_at
        )
        VALUES (
            $1,$2,1,now()
        )
        ON CONFLICT (
            business_id,
            branch_id
        )
        DO UPDATE SET
            revision=inventory_revisions.revision+1,
            updated_at=now()
        `,
		out.BusinessID,
		out.BranchID,
	)
	if err != nil {
		return InventoryItem{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return InventoryItem{}, err
	}

	return out, nil
}

func (s *PostgresInventoryItemStore) GetInventoryItem(
	ctx context.Context,
	businessID string,
	itemID string,
) (
	InventoryItem,
	error,
) {
	if s == nil || s.pool == nil {
		return InventoryItem{}, ErrDependencyMissing
	}

	businessID = strings.TrimSpace(businessID)
	itemID = strings.TrimSpace(itemID)

	var out InventoryItem

	err := s.pool.QueryRow(
		ctx,
		`
        SELECT
            id,
            business_id,
            branch_id,
            item_type,
            source_ref,
            stock_measure_ref,
            stock_measure_owner_key,
            stock_unit_kind,
            stock_basis_mode,
            tracking_mode,
            lot_policy,
            lot_tracking_enabled,
            status,
            version,
            created_at,
            updated_at
          FROM inventory_items
         WHERE id=$1
           AND business_id=$2
        `,
		itemID,
		businessID,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.ItemType,
		&out.SourceRef,
		&out.StockMeasureRef,
		&out.StockMeasureOwnerKey,
		&out.StockUnitKind,
		&out.StockBasisMode,
		&out.TrackingMode,
		&out.LotPolicy,
		&out.LotTrackingEnabled,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return InventoryItem{}, ErrItemNotFound
	}

	if err != nil {
		return InventoryItem{}, err
	}

	return out, nil
}

func (s *PostgresInventoryItemStore) InventoryRevision(
	ctx context.Context,
	businessID string,
	branchID string,
) (
	int64,
	time.Time,
	error,
) {
	if s == nil || s.pool == nil {
		return 0, time.Time{}, ErrDependencyMissing
	}

	var revision int64
	var updatedAt time.Time

	err := s.pool.QueryRow(
		ctx,
		`
        SELECT
            revision,
            updated_at
          FROM inventory_revisions
         WHERE business_id=$1
           AND branch_id=$2
        `,
		strings.TrimSpace(businessID),
		strings.TrimSpace(branchID),
	).Scan(
		&revision,
		&updatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return 0, time.Time{}, nil
	}

	if err != nil {
		return 0, time.Time{}, err
	}

	return revision, updatedAt, nil
}
