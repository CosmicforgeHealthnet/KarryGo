package food

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// AdministrativeListingTx performs only a native pause or restoration, with
// unchanged prices, stock and production. Native catalogue revision and outbox
// evidence advance in the same source transaction as the command receipt.
func (s *Service) AdministrativeListingTx(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, id string, restore bool, expected int64) (int64, string, string, error) {
	if err := s.requireWrites(); err != nil {
		return 0, "", "", err
	}
	current, err := s.lockedItem(ctx, tx, actor, scope, id)
	if err != nil {
		return 0, "", "", err
	}
	if current.Version != expected || expected < 1 {
		return 0, "", "", errStaleVersion()
	}
	next := StatusPaused
	if restore {
		next = StatusActive
	}
	if restore && current.Status != StatusPaused || !restore && current.Status != StatusActive {
		return 0, "", "", errArchived("Food item is not eligible for this moderation transition")
	}
	if err = itemTransition(current.Status, next); err != nil {
		return 0, "", "", err
	}
	if restore {
		menu, e := loadMenu(ctx, tx, actor.VendorID, current.MenuID, false)
		if e != nil {
			return 0, "", "", e
		}
		if menu.Status == StatusArchived {
			return 0, "", "", errArchived("Menu")
		}
	}
	updated, err := scanItem(tx.QueryRow(ctx, `UPDATE food_items SET status=$3,version=version+1,updated_at=now() WHERE business_id=$1 AND id=$2 RETURNING `+itemCols, actor.VendorID, id, next))
	if err != nil {
		return 0, "", "", err
	}
	catalogue, err := scopeOfItem(ctx, tx, updated)
	if err != nil {
		return 0, "", "", err
	}
	revision, err := bumpRevision(ctx, tx, catalogue)
	if err != nil {
		return 0, "", "", err
	}
	err = s.record(ctx, tx, actor, evidence{action: "food.item.updated", eventType: "food.item.updated", aggregateType: "food_item", aggregateID: id, branchID: current.BranchID, version: updated.Version, before: ItemViewOf(current, "", nil), after: ItemViewOf(updated, "", nil), payload: map[string]any{"item": ItemViewOf(updated, "", nil), "catalogue_revision": revision}})
	return current.Version, current.Status, next, err
}
