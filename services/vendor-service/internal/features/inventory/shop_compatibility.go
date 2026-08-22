package inventory

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrLegacyIdentityConflict means a generic Inventory identity already
	// exists for the Shop variant under an ID different from the authoritative
	// legacy Shop stock-basis ID.
	//
	// 14C never guesses which identity should win.
	ErrLegacyIdentityConflict = errors.New(
		"inventory legacy stock identity conflicts with generic inventory identity",
	)
)

// ShopStockBasisCompatibilityInput is the exact information already proved by
// the certified Shop stock-basis workflow.
//
// Generic Inventory does not query Shop repositories to discover this data.
type ShopStockBasisCompatibilityInput struct {
	StockBasisID string

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

// StockUnitKindFromMeasurementFamily maps the Shop measurement family into the
// deliberately coarse generic Inventory unit-kind vocabulary.
//
// It does not invent conversion values.
func StockUnitKindFromMeasurementFamily(
	family string,
) string {
	switch strings.ToUpper(
		strings.TrimSpace(family),
	) {
	case "COUNT":
		return StockUnitCount

	case "MASS":
		return StockUnitMass

	case "VOLUME":
		return StockUnitVolume

	default:
		return StockUnitConfiguredMeasure
	}
}

// ShopTrackingMode derives the generic tracking representation while leaving
// Shop as the compatibility authority during Section 14C.
func ShopTrackingMode(
	consumptionMode string,
	lotTracking bool,
) string {
	if strings.EqualFold(
		strings.TrimSpace(consumptionMode),
		StockBasisVariableFulfilment,
	) {
		return TrackingVariableMeasure
	}

	if lotTracking {
		return TrackingLot
	}

	return TrackingSimple
}

type compatibilityRowScanner interface {
	Scan(dest ...any) error
}

func scanCompatibilityInventoryItem(
	row compatibilityRowScanner,
) (
	InventoryItem,
	error,
) {
	var item InventoryItem

	err := row.Scan(
		&item.ID,
		&item.BusinessID,
		&item.BranchID,
		&item.ItemType,
		&item.SourceRef,
		&item.StockMeasureRef,
		&item.StockMeasureOwnerKey,
		&item.StockUnitKind,
		&item.StockBasisMode,
		&item.TrackingMode,
		&item.LotPolicy,
		&item.LotTrackingEnabled,
		&item.Status,
		&item.Version,
		&item.CreatedAt,
		&item.UpdatedAt,
	)

	return item, err
}

// UpsertShopStockBasisCompatibilityTx creates or synchronises the generic
// Inventory identity for one authoritative Shop stock basis.
//
// Critical compatibility rule:
//
//	generic inventory_items.id == shop_inventory_stock_bases.id
//
// Existing Shop stock IDs therefore remain valid throughout the migration.
//
// The operation is idempotent. inventory_revisions advances only when generic
// state actually changes.
func UpsertShopStockBasisCompatibilityTx(
	ctx context.Context,
	tx pgx.Tx,
	input ShopStockBasisCompatibilityInput,
) (
	InventoryItem,
	bool,
	error,
) {
	if tx == nil {
		return InventoryItem{}, false, ErrDependencyMissing
	}

	input.StockBasisID =
		strings.TrimSpace(input.StockBasisID)

	input.BusinessID =
		strings.TrimSpace(input.BusinessID)

	input.BranchID =
		strings.TrimSpace(input.BranchID)

	input.VariantID =
		strings.TrimSpace(input.VariantID)

	input.MeasureRef =
		strings.TrimSpace(input.MeasureRef)

	input.MeasureOwnerKey =
		strings.TrimSpace(input.MeasureOwnerKey)

	input.ConsumptionMode =
		strings.ToUpper(
			strings.TrimSpace(input.ConsumptionMode),
		)

	input.LotPolicy =
		strings.ToUpper(
			strings.TrimSpace(input.LotPolicy),
		)

	input.Status =
		strings.ToUpper(
			strings.TrimSpace(input.Status),
		)

	if input.StockBasisID == "" ||
		input.BusinessID == "" ||
		input.BranchID == "" ||
		input.VariantID == "" ||
		input.MeasureRef == "" ||
		input.MeasureOwnerKey == "" {
		return InventoryItem{}, false, ErrInvalidSource
	}

	switch input.ConsumptionMode {
	case StockBasisSameUnit,
		StockBasisExactConversion,
		StockBasisVariableFulfilment:
	default:
		return InventoryItem{}, false, ErrInvalidMeasure
	}

	if input.LotPolicy == "" {
		input.LotPolicy = LotPolicyFIFO
	}

	if input.LotPolicy != LotPolicyFIFO &&
		input.LotPolicy != LotPolicyFEFO {
		return InventoryItem{}, false, ErrInvalidSource
	}

	// Generic Inventory uses ACTIVE/ARCHIVED at this phase.
	//
	// Shop remains lifecycle authority. INACTIVE legacy stock bases therefore
	// become non-active generic identities rather than introducing a third
	// generic lifecycle state.
	genericStatus := ItemStatusArchived

	if input.Status == ItemStatusActive {
		genericStatus = ItemStatusActive
	}

	if input.Version <= 0 {
		input.Version = 1
	}

	now := time.Now().UTC()

	if input.CreatedAt.IsZero() {
		input.CreatedAt = now
	}

	if input.UpdatedAt.IsZero() {
		input.UpdatedAt = input.CreatedAt
	}

	unitKind :=
		StockUnitKindFromMeasurementFamily(
			input.MeasureFamily,
		)

	trackingMode :=
		ShopTrackingMode(
			input.ConsumptionMode,
			input.LotTrackingEnabled,
		)

	// One Shop variant must not already have a different generic identity.
	var existingIdentity string

	err := tx.QueryRow(
		ctx,
		`
        SELECT id
          FROM inventory_items
         WHERE business_id=$1
           AND branch_id=$2
           AND item_type='SHOP_VARIANT'
           AND source_ref=$3
         LIMIT 1
        `,
		input.BusinessID,
		input.BranchID,
		input.VariantID,
	).Scan(
		&existingIdentity,
	)

	if err != nil &&
		!errors.Is(
			err,
			pgx.ErrNoRows,
		) {
		return InventoryItem{}, false, err
	}

	if existingIdentity != "" &&
		existingIdentity != input.StockBasisID {
		return InventoryItem{},
			false,
			ErrLegacyIdentityConflict
	}

	query := `
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
            version,
            created_at,
            updated_at
        )
        VALUES (
            $1,$2,$3,'SHOP_VARIANT',$4,$5,$6,$7,
            $8,$9,$10,$11,$12,$13,$14,$15
        )
        ON CONFLICT (id)
        DO UPDATE SET
            business_id=EXCLUDED.business_id,
            branch_id=EXCLUDED.branch_id,
            item_type=EXCLUDED.item_type,
            source_ref=EXCLUDED.source_ref,
            stock_measure_ref=EXCLUDED.stock_measure_ref,
            stock_measure_owner_key=EXCLUDED.stock_measure_owner_key,
            stock_unit_kind=EXCLUDED.stock_unit_kind,
            stock_basis_mode=EXCLUDED.stock_basis_mode,
            tracking_mode=EXCLUDED.tracking_mode,
            lot_policy=EXCLUDED.lot_policy,
            lot_tracking_enabled=EXCLUDED.lot_tracking_enabled,
            status=EXCLUDED.status,
            version=GREATEST(
                inventory_items.version,
                EXCLUDED.version
            ),
            updated_at=EXCLUDED.updated_at
        WHERE (
            inventory_items.business_id,
            inventory_items.branch_id,
            inventory_items.item_type,
            inventory_items.source_ref,
            inventory_items.stock_measure_ref,
            inventory_items.stock_measure_owner_key,
            inventory_items.stock_unit_kind,
            inventory_items.stock_basis_mode,
            inventory_items.tracking_mode,
            inventory_items.lot_policy,
            inventory_items.lot_tracking_enabled,
            inventory_items.status,
            inventory_items.version
        ) IS DISTINCT FROM (
            EXCLUDED.business_id,
            EXCLUDED.branch_id,
            EXCLUDED.item_type,
            EXCLUDED.source_ref,
            EXCLUDED.stock_measure_ref,
            EXCLUDED.stock_measure_owner_key,
            EXCLUDED.stock_unit_kind,
            EXCLUDED.stock_basis_mode,
            EXCLUDED.tracking_mode,
            EXCLUDED.lot_policy,
            EXCLUDED.lot_tracking_enabled,
            EXCLUDED.status,
            EXCLUDED.version
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
    `

	item, err :=
		scanCompatibilityInventoryItem(
			tx.QueryRow(
				ctx,
				query,
				input.StockBasisID,
				input.BusinessID,
				input.BranchID,
				input.VariantID,
				input.MeasureRef,
				input.MeasureOwnerKey,
				unitKind,
				input.ConsumptionMode,
				trackingMode,
				input.LotPolicy,
				input.LotTrackingEnabled,
				genericStatus,
				input.Version,
				input.CreatedAt.UTC(),
				input.UpdatedAt.UTC(),
			),
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		// ON CONFLICT found an identical row and deliberately performed no
		// update. Return current state without advancing the revision.
		item, err =
			scanCompatibilityInventoryItem(
				tx.QueryRow(
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
					input.StockBasisID,
					input.BusinessID,
				),
			)

		if err != nil {
			return InventoryItem{}, false, err
		}

		return item, false, nil
	}

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(
			err,
			&pgErr,
		) &&
			pgErr.Code == "23505" {
			return InventoryItem{},
				false,
				ErrLegacyIdentityConflict
		}

		return InventoryItem{}, false, err
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
		input.BusinessID,
		input.BranchID,
	)
	if err != nil {
		return InventoryItem{}, false, err
	}

	return item, true, nil
}
