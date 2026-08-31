package shop

import (
	"context"
	"errors"
	"strings"

	genericinventory "cosmicforge/logistics/services/vendor-service/internal/features/inventory"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GenericInventoryResolver adapts Shop catalogue authority into the generic
// Inventory ports.
//
// Dependency direction is:
//
//	Shop adapter -> generic Inventory
//
// generic Inventory never imports Shop.
type GenericInventoryResolver struct {
	pool *pgxpool.Pool
}

func NewGenericInventoryResolver(
	pool *pgxpool.Pool,
) *GenericInventoryResolver {
	return &GenericInventoryResolver{
		pool: pool,
	}
}

func (r *GenericInventoryResolver) ResolveInventorySource(
	ctx context.Context,
	actor genericinventory.Actor,
	scope genericinventory.BranchScope,
	source genericinventory.SourceReference,
) (
	genericinventory.ResolvedInventorySource,
	error,
) {
	if r == nil || r.pool == nil {
		return genericinventory.ResolvedInventorySource{},
			genericinventory.ErrDependencyMissing
	}

	if strings.ToUpper(strings.TrimSpace(source.Type)) !=
		genericinventory.ItemTypeShopVariant {
		return genericinventory.ResolvedInventorySource{},
			genericinventory.ErrUnsupportedItemType
	}

	if scope.BranchID == nil {
		return genericinventory.ResolvedInventorySource{},
			genericinventory.ErrInvalidScope
	}

	branchID := strings.TrimSpace(*scope.BranchID)

	if !scope.Covers(
		actor.BusinessID,
		branchID,
	) {
		return genericinventory.ResolvedInventorySource{},
			genericinventory.ErrScopeViolation
	}

	var variantID string
	var businessID string
	var resolvedBranchID string
	var variantStatus string
	var listingStatus string

	err := r.pool.QueryRow(
		ctx,
		`
        SELECT
            v.id,
            v.business_id,
            l.branch_id,
            v.status,
            l.status
          FROM shop_variants v
          JOIN shop_branch_listings l
            ON l.business_id=v.business_id
           AND l.variant_id=v.id
         WHERE v.id=$1
           AND v.business_id=$2
           AND l.branch_id=$3
         ORDER BY l.id
         LIMIT 1
        `,
		strings.TrimSpace(source.Ref),
		strings.TrimSpace(actor.BusinessID),
		branchID,
	).Scan(
		&variantID,
		&businessID,
		&resolvedBranchID,
		&variantStatus,
		&listingStatus,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return genericinventory.ResolvedInventorySource{},
			genericinventory.ErrSourceNotFound
	}

	if err != nil {
		return genericinventory.ResolvedInventorySource{},
			err
	}

	active :=
		variantStatus == "ACTIVE" &&
			listingStatus == "ACTIVE"

	if !active {
		return genericinventory.ResolvedInventorySource{
				Source: genericinventory.SourceReference{
					Type: genericinventory.ItemTypeShopVariant,
					Ref:  variantID,
				},
				BusinessID: businessID,
				BranchID:   &resolvedBranchID,
				Active:     false,
			},
			nil
	}

	return genericinventory.ResolvedInventorySource{
			Source: genericinventory.SourceReference{
				Type: genericinventory.ItemTypeShopVariant,
				Ref:  variantID,
			},
			BusinessID: businessID,
			BranchID:   &resolvedBranchID,
			Active:     true,
		},
		nil
}

func (r *GenericInventoryResolver) ResolveStockMeasure(
	ctx context.Context,
	actor genericinventory.Actor,
	scope genericinventory.BranchScope,
	source genericinventory.SourceReference,
	measureRef string,
) (
	genericinventory.ResolvedStockMeasure,
	error,
) {
	if r == nil || r.pool == nil {
		return genericinventory.ResolvedStockMeasure{},
			genericinventory.ErrDependencyMissing
	}

	if strings.ToUpper(strings.TrimSpace(source.Type)) !=
		genericinventory.ItemTypeShopVariant {
		return genericinventory.ResolvedStockMeasure{},
			genericinventory.ErrUnsupportedItemType
	}

	if scope.BranchID == nil {
		return genericinventory.ResolvedStockMeasure{},
			genericinventory.ErrInvalidScope
	}

	branchID := strings.TrimSpace(*scope.BranchID)

	if !scope.Covers(
		actor.BusinessID,
		branchID,
	) {
		return genericinventory.ResolvedStockMeasure{},
			genericinventory.ErrScopeViolation
	}

	var ownerKey string
	var family string
	var baseQuantity *int64
	var status string

	err := r.pool.QueryRow(
		ctx,
		`
        SELECT
            owner_key,
            measurement_family,
            base_quantity,
            status
          FROM shop_measure_definitions
         WHERE id=$1
           AND owner_key IN ('PLATFORM',$2)
         LIMIT 1
        `,
		strings.TrimSpace(measureRef),
		strings.TrimSpace(actor.BusinessID),
	).Scan(
		&ownerKey,
		&family,
		&baseQuantity,
		&status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return genericinventory.ResolvedStockMeasure{},
			genericinventory.ErrInvalidMeasure
	}

	if err != nil {
		return genericinventory.ResolvedStockMeasure{},
			err
	}

	if status != "ACTIVE" {
		return genericinventory.ResolvedStockMeasure{
				Ref:      strings.TrimSpace(measureRef),
				OwnerKey: ownerKey,
				Family:   family,
				Active:   false,
			},
			nil
	}

	var sameUnitEligible bool

	err = r.pool.QueryRow(
		ctx,
		`
        SELECT EXISTS (
            SELECT 1
              FROM shop_branch_listings l
              JOIN shop_listing_sell_units lu
                ON lu.business_id=l.business_id
               AND lu.listing_id=l.id
              JOIN shop_sell_units su
                ON su.id=lu.sell_unit_id
               AND su.business_id=lu.business_id
             WHERE l.business_id=$1
               AND l.branch_id=$2
               AND l.variant_id=$3
               AND l.status='ACTIVE'
               AND lu.enabled
               AND su.status='ACTIVE'
               AND su.measure_definition_id=$4
               AND su.conversion_type <> 'CALIBRATED_APPROXIMATE'
        )
        `,
		strings.TrimSpace(actor.BusinessID),
		branchID,
		strings.TrimSpace(source.Ref),
		strings.TrimSpace(measureRef),
	).Scan(
		&sameUnitEligible,
	)
	if err != nil {
		return genericinventory.ResolvedStockMeasure{},
			err
	}

	unitKind := genericinventory.StockUnitConfiguredMeasure

	switch strings.ToUpper(strings.TrimSpace(family)) {
	case "COUNT":
		unitKind = genericinventory.StockUnitCount
	case "MASS":
		unitKind = genericinventory.StockUnitMass
	case "VOLUME":
		unitKind = genericinventory.StockUnitVolume
	}

	// Reuse the already-certified Shop conversion-family rule rather than
	// reimplementing Section 12 measurement semantics in Inventory.
	exactConvertible :=
		IsConvertibleFamily(family) &&
			baseQuantity != nil &&
			*baseQuantity > 0

	return genericinventory.ResolvedStockMeasure{
			Ref:      strings.TrimSpace(measureRef),
			OwnerKey: ownerKey,
			Family:   family,
			UnitKind: unitKind,

			Active: true,

			SameUnitEligible:   sameUnitEligible,
			ExactConvertible:   exactConvertible,
			VariableFulfilment: true,
		},
		nil
}
