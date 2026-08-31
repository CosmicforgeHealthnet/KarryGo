package shop

import (
	"context"
	"time"

	genericinventory "cosmicforge/logistics/services/vendor-service/internal/features/inventory"

	"github.com/jackc/pgx/v5"
)

const shopInventoryCompatibilityBackfillJob = "SHOP_STOCK_BASIS_TO_GENERIC_INVENTORY_V1"

const maxInventoryCompatibilityBackfillBatch = 500

type InventoryCompatibilityBackfillResult struct {
	Processed int
	Changed   int

	Cursor string

	Completed bool
}

// RunInventoryCompatibilityBackfillBatch performs one bounded, restartable
// identity backfill.
//
// It never reconstructs balances or operational stock state.
//
// Multiple callers serialize on the durable job row.
func (s *Service) RunInventoryCompatibilityBackfillBatch(
	ctx context.Context,
	batchSize int,
) (
	InventoryCompatibilityBackfillResult,
	error,
) {
	if s == nil || s.db == nil {
		return InventoryCompatibilityBackfillResult{},
			genericinventory.ErrDependencyMissing
	}

	if batchSize <= 0 {
		batchSize = 100
	}

	if batchSize > maxInventoryCompatibilityBackfillBatch {
		batchSize =
			maxInventoryCompatibilityBackfillBatch
	}

	startedAt := s.now()

	tx, err := s.db.BeginTx(
		ctx,
		pgx.TxOptions{
			IsoLevel: pgx.ReadCommitted,
		},
	)
	if err != nil {
		return InventoryCompatibilityBackfillResult{},
			err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	_, err = tx.Exec(
		ctx,
		`
        INSERT INTO inventory_shop_compat_backfill_jobs (
            job_key,
            status
        )
        VALUES (
            $1,
            'PENDING'
        )
        ON CONFLICT (job_key)
        DO NOTHING
        `,
		shopInventoryCompatibilityBackfillJob,
	)
	if err != nil {
		return InventoryCompatibilityBackfillResult{},
			err
	}

	var (
		status string
		cursor string
	)

	err = tx.QueryRow(
		ctx,
		`
        SELECT
            status,
            cursor_stock_basis_id
          FROM inventory_shop_compat_backfill_jobs
         WHERE job_key=$1
         FOR UPDATE
        `,
		shopInventoryCompatibilityBackfillJob,
	).Scan(
		&status,
		&cursor,
	)
	if err != nil {
		return InventoryCompatibilityBackfillResult{},
			err
	}

	if status == "COMPLETED" {
		if err := tx.Commit(ctx); err != nil {
			return InventoryCompatibilityBackfillResult{},
				err
		}

		return InventoryCompatibilityBackfillResult{
			Cursor:    cursor,
			Completed: true,
		}, nil
	}

	_, err = tx.Exec(
		ctx,
		`
        UPDATE inventory_shop_compat_backfill_jobs
           SET status='RUNNING',
               attempt_count=attempt_count+1,
               started_at=COALESCE(started_at,now()),
               last_error=NULL,
               updated_at=now()
         WHERE job_key=$1
        `,
		shopInventoryCompatibilityBackfillJob,
	)
	if err != nil {
		return InventoryCompatibilityBackfillResult{},
			err
	}

	type legacyBasis struct {
		ID string

		BusinessID string
		BranchID   string
		VariantID  string

		MeasureRef      string
		MeasureOwnerKey string
		MeasureFamily   string

		ConsumptionMode string
		LotPolicy       string

		LotTrackingEnabled bool

		Status string

		Version int64

		CreatedAt time.Time
		UpdatedAt time.Time
	}

	rows, err := tx.Query(
		ctx,
		`
        SELECT
            b.id,
            b.business_id,
            b.branch_id,
            b.variant_id,
            b.measure_definition_id,
            b.measure_owner_key,
            m.measurement_family,
            b.consumption_mode,
            b.lot_policy,
            b.lot_tracking_enabled,
            b.status,
            b.version,
            b.created_at,
            b.updated_at
          FROM shop_inventory_stock_bases b
          JOIN shop_measure_definitions m
            ON m.id=b.measure_definition_id
           AND m.owner_key=b.measure_owner_key
         WHERE b.id > $1
         ORDER BY b.id
         LIMIT $2
        `,
		cursor,
		batchSize,
	)
	if err != nil {
		return InventoryCompatibilityBackfillResult{},
			err
	}

	legacy := make(
		[]legacyBasis,
		0,
		batchSize,
	)

	for rows.Next() {
		var item legacyBasis

		if err := rows.Scan(
			&item.ID,
			&item.BusinessID,
			&item.BranchID,
			&item.VariantID,
			&item.MeasureRef,
			&item.MeasureOwnerKey,
			&item.MeasureFamily,
			&item.ConsumptionMode,
			&item.LotPolicy,
			&item.LotTrackingEnabled,
			&item.Status,
			&item.Version,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			rows.Close()

			return InventoryCompatibilityBackfillResult{},
				err
		}

		legacy = append(
			legacy,
			item,
		)
	}

	if err := rows.Err(); err != nil {
		rows.Close()

		return InventoryCompatibilityBackfillResult{},
			err
	}

	rows.Close()

	if len(legacy) == 0 {
		_, err = tx.Exec(
			ctx,
			`
            UPDATE inventory_shop_compat_backfill_jobs
               SET status='COMPLETED',
                   completed_at=COALESCE(completed_at,now()),
                   updated_at=now()
             WHERE job_key=$1
            `,
			shopInventoryCompatibilityBackfillJob,
		)
		if err != nil {
			return InventoryCompatibilityBackfillResult{},
				err
		}

		if err := tx.Commit(ctx); err != nil {
			return InventoryCompatibilityBackfillResult{},
				err
		}

		return InventoryCompatibilityBackfillResult{
			Cursor:    cursor,
			Completed: true,
		}, nil
	}

	changed := 0

	for _, legacyItem := range legacy {
		_, didChange, err :=
			genericinventory.UpsertShopStockBasisCompatibilityTx(
				ctx,
				tx,
				genericinventory.ShopStockBasisCompatibilityInput{
					StockBasisID: legacyItem.ID,

					BusinessID: legacyItem.BusinessID,

					BranchID: legacyItem.BranchID,

					VariantID: legacyItem.VariantID,

					MeasureRef: legacyItem.MeasureRef,

					MeasureOwnerKey: legacyItem.MeasureOwnerKey,

					MeasureFamily: legacyItem.MeasureFamily,

					ConsumptionMode: legacyItem.ConsumptionMode,

					LotPolicy: legacyItem.LotPolicy,

					LotTrackingEnabled: legacyItem.LotTrackingEnabled,

					Status: legacyItem.Status,

					Version: legacyItem.Version,

					CreatedAt: legacyItem.CreatedAt,

					UpdatedAt: legacyItem.UpdatedAt,
				},
			)
		if err != nil {
			return InventoryCompatibilityBackfillResult{},
				err
		}

		if didChange {
			changed++
		}
	}

	fromCursor := cursor
	toCursor := legacy[len(legacy)-1].ID

	var more bool

	err = tx.QueryRow(
		ctx,
		`
        SELECT EXISTS (
            SELECT 1
              FROM shop_inventory_stock_bases
             WHERE id > $1
        )
        `,
		toCursor,
	).Scan(
		&more,
	)
	if err != nil {
		return InventoryCompatibilityBackfillResult{},
			err
	}

	completed := !more

	jobStatus := "PENDING"

	if completed {
		jobStatus = "COMPLETED"
	}

	completedAt := s.now()

	_, err = tx.Exec(
		ctx,
		`
        INSERT INTO inventory_shop_compat_backfill_batches (
            id,
            job_key,
            from_cursor,
            to_cursor,
            processed_count,
            changed_count,
            status,
            started_at,
            completed_at
        )
        VALUES (
            $1,$2,$3,$4,$5,$6,
            'COMPLETED',
            $7,$8
        )
        `,
		newID("icb_"),
		shopInventoryCompatibilityBackfillJob,
		fromCursor,
		toCursor,
		len(legacy),
		changed,
		startedAt,
		completedAt,
	)
	if err != nil {
		return InventoryCompatibilityBackfillResult{},
			err
	}

	_, err = tx.Exec(
		ctx,
		`
        UPDATE inventory_shop_compat_backfill_jobs
           SET status=$2,
               cursor_stock_basis_id=$3,
               processed_count=processed_count+$4,
               batch_count=batch_count+1,
               completed_at=
                   CASE
                       WHEN $2='COMPLETED'
                       THEN COALESCE(completed_at,now())
                       ELSE NULL
                   END,
               updated_at=now()
         WHERE job_key=$1
        `,
		shopInventoryCompatibilityBackfillJob,
		jobStatus,
		toCursor,
		len(legacy),
	)
	if err != nil {
		return InventoryCompatibilityBackfillResult{},
			err
	}

	if err := tx.Commit(ctx); err != nil {
		return InventoryCompatibilityBackfillResult{},
			err
	}

	return InventoryCompatibilityBackfillResult{
		Processed: len(legacy),
		Changed:   changed,
		Cursor:    toCursor,
		Completed: completed,
	}, nil
}
