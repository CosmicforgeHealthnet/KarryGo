package recipes

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

type queryRower interface {
	QueryRow(
		ctx context.Context,
		sql string,
		args ...any,
	) pgx.Row
}

// ResolveTarget resolves one typed catalogue target through the existing
// Food/Pastry/Cloud Kitchen tables.
//
// It deliberately does not import those feature packages. The recipe domain
// consumes only their stable PostgreSQL identity boundary and never becomes
// catalogue authority.
func (s *Service) ResolveTarget(
	ctx context.Context,
	businessID string,
	targetType string,
	targetRef string,
	variantRef string,
) (Target, error) {
	return resolveTargetQ(
		ctx,
		s.db,
		businessID,
		targetType,
		targetRef,
		variantRef,
	)
}

func resolveTargetQ(
	ctx context.Context,
	q queryRower,
	businessID string,
	targetType string,
	targetRef string,
	variantRef string,
) (Target, error) {
	targetType = normalizeEnum(targetType)
	targetRef = strings.TrimSpace(targetRef)
	variantRef = strings.TrimSpace(variantRef)

	if strings.TrimSpace(businessID) == "" ||
		!validTargetType(targetType) ||
		targetRef == "" {
		return Target{}, ErrTargetInvalid
	}

	var branchID string
	var status string

	switch targetType {
	case TargetFoodItem:
		err := q.QueryRow(
			ctx,
			`
            SELECT
                branch_id,
                status
              FROM food_items
             WHERE id=$1
               AND business_id=$2
            `,
			targetRef,
			businessID,
		).Scan(
			&branchID,
			&status,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return Target{}, ErrTargetInvalid
		}
		if err != nil {
			return Target{}, err
		}

		if variantRef != "" {
			var exists bool

			err = q.QueryRow(
				ctx,
				`
                SELECT EXISTS(
                    SELECT 1
                      FROM food_item_variants v
                      JOIN food_items i
                        ON i.id=v.item_id
                     WHERE v.id=$1
                       AND i.id=$2
                       AND i.business_id=$3
                )
                `,
				variantRef,
				targetRef,
				businessID,
			).Scan(&exists)

			if err != nil {
				return Target{}, err
			}

			if !exists {
				return Target{}, ErrTargetInvalid
			}
		}

	case TargetPastryProduct:
		err := q.QueryRow(
			ctx,
			`
            SELECT
                branch_id,
                status
              FROM pastry_products
             WHERE id=$1
               AND business_id=$2
            `,
			targetRef,
			businessID,
		).Scan(
			&branchID,
			&status,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return Target{}, ErrTargetInvalid
		}
		if err != nil {
			return Target{}, err
		}

		if variantRef != "" {
			var exists bool

			err = q.QueryRow(
				ctx,
				`
                SELECT EXISTS(
                    SELECT 1
                      FROM pastry_product_option_groups pg
                      JOIN pastry_options o
                        ON o.group_id=pg.group_id
                     WHERE pg.product_id=$1
                       AND o.id=$2
                )
                `,
				targetRef,
				variantRef,
			).Scan(&exists)

			if err != nil {
				return Target{}, err
			}

			if !exists {
				return Target{}, ErrTargetInvalid
			}
		}

	case TargetCloudKitchenPackage:
		if variantRef != "" {
			return Target{}, ErrTargetInvalid
		}

		err := q.QueryRow(
			ctx,
			`
            SELECT
                branch_id,
                status
              FROM cloud_kitchen_packages
             WHERE id=$1
               AND business_id=$2
            `,
			targetRef,
			businessID,
		).Scan(
			&branchID,
			&status,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return Target{}, ErrTargetInvalid
		}
		if err != nil {
			return Target{}, err
		}
	}

	if strings.EqualFold(
		strings.TrimSpace(status),
		"ARCHIVED",
	) {
		return Target{}, ErrTargetInvalid
	}

	return Target{
		Type:       targetType,
		Ref:        targetRef,
		VariantRef: variantRef,
		BranchID:   branchID,
		Status:     status,
	}, nil
}
