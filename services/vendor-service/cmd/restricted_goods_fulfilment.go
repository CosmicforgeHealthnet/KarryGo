package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/features/taxonomy"
)

// restrictedGoodsFulfilmentAuthority is the final defence-in-depth gate. It
// reads current Shop product facts but does not mutate catalogue or order
// history. A policy change therefore blocks new custody without rewriting the
// immutable commercial snapshot the customer accepted.
type restrictedGoodsFulfilmentAuthority struct{ db *pgxpool.Pool }

func newRestrictedGoodsFulfilmentAuthority(db *pgxpool.Pool) *restrictedGoodsFulfilmentAuthority {
	if db == nil {
		return nil
	}
	return &restrictedGoodsFulfilmentAuthority{db: db}
}

func (a *restrictedGoodsFulfilmentAuthority) ValidateOrderForFulfilment(ctx context.Context, order orders.ParentOrder) error {
	if a == nil || a.db == nil {
		return orders.ErrDependencyMissing
	}
	for _, vendorOrder := range order.VendorOrders {
		for _, line := range vendorOrder.Lines {
			facts := taxonomy.RestrictedGoodsFacts{Name: line.ItemName}
			if strings.EqualFold(vendorOrder.Domain, "SHOP") && strings.TrimSpace(line.ItemRef) != "" {
				err := a.db.QueryRow(ctx, `SELECT name,COALESCE(description,''),COALESCE(brand,''),COALESCE(manufacturer,''),product_class,COALESCE(regulatory_reference,''),COALESCE(manufacturer_code,'') FROM shop_products WHERE business_id=$1 AND id=$2`, vendorOrder.BusinessID, line.ItemRef).Scan(
					&facts.Name, &facts.Description, &facts.Brand, &facts.Manufacturer,
					&facts.ProductClass, &facts.RegulatoryReference, &facts.ManufacturerCode,
				)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("restricted goods authority unavailable: %w", err)
				}
			}
			decision := taxonomy.EvaluateRestrictedGoods(facts)
			if !decision.Allowed {
				return fmt.Errorf("%w: %s (%s)", orders.ErrRestrictedGoods, decision.ReasonCode, decision.PolicyVersion)
			}
		}
	}
	return nil
}

var _ orders.RestrictedGoodsAuthority = (*restrictedGoodsFulfilmentAuthority)(nil)
