package shop

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// AdministrativeListingTx withholds/restores only the native listing. It never
// edits a price, quantity, reservation or product verification fact.
func (s *Service) AdministrativeListingTx(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, id string, restore bool, expected int64) (int64, string, string, error) {
	if err := s.requireWrites(); err != nil {
		return 0, "", "", err
	}
	current, err := loadListing(ctx, tx, actor.VendorID, id, true)
	if err != nil {
		return 0, "", "", err
	}
	if err = s.requireListingAccess(ctx, tx, actor, scope, current); err != nil {
		return 0, "", "", err
	}
	if err = expectedVersionMatches(current.Version, expected); err != nil {
		return 0, "", "", err
	}
	next := StatusInactive
	if restore {
		next = StatusActive
		if current.Status != StatusInactive {
			return 0, "", "", errArchived("Listing is not eligible for restoration")
		}
	} else if current.Status != StatusActive {
		return 0, "", "", errArchived("Listing is not active")
	}
	if restore {
		product, e := loadProduct(ctx, tx, actor.VendorID, current.ProductID, false)
		if e != nil {
			return 0, "", "", e
		}
		if e = validateRestrictedGoodsProduct("product", product); e != nil {
			return 0, "", "", e
		}
		if product.Status != StatusActive {
			return 0, "", "", errArchived("Product is not active")
		}
	}
	updated, err := scanListing(tx.QueryRow(ctx, `UPDATE shop_branch_listings SET status=$3,version=version+1,updated_at=now() WHERE business_id=$1 AND id=$2 RETURNING `+listingCols, actor.VendorID, id, next))
	if err != nil {
		return 0, "", "", err
	}
	_, err = bumpBranchRevision(ctx, tx, actor.VendorID, current.BranchID)
	if err != nil {
		return 0, "", "", err
	}
	err = s.record(ctx, tx, actor, evidence{action: "shop.listing.updated", eventType: "shop.listing.changed", aggregateType: "shop_branch_listing", aggregateID: id, branchID: current.BranchID, version: updated.Version, before: ListingViewOf(current), after: ListingViewOf(updated), payload: ListingViewOf(updated)})
	return current.Version, current.Status, next, err
}
