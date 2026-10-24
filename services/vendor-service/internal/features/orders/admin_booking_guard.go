package orders

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/vendors"
	"errors"
	"github.com/jackc/pgx/v5"
	"sort"
)

// guardAdministrativeBookability fences the NEW order commit against native
// merchant/branch pauses and catalogue takedowns, including an already approved
// quote. It never changes prices, consumes stock or affects existing orders.
// The private boundary cannot operate without its owner migration; an older
// installation without that boundary retains its existing booking contract.
func guardAdministrativeBookability(ctx context.Context, tx pgx.Tx, source QuoteSource) error {
	var installed bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('admin_owner_command_decisions') IS NOT NULL`).Scan(&installed); err != nil {
		return err
	}
	if !installed {
		return nil
	}
	type target struct{ kind, id, business string }
	targets := map[string]target{}
	add := func(kind, id, business string) {
		if id != "" {
			targets[kind+"/"+id] = target{kind, id, business}
		}
	}
	groups := map[string]GroupSource{}
	for _, g := range source.Groups {
		groups[g.QuoteGroupRef] = g
		add("merchants", g.BusinessID, g.BusinessID)
		add("branches", g.BranchID, g.BusinessID)
	}
	for _, line := range source.Lines {
		group, ok := groups[line.QuoteGroupRef]
		if !ok {
			return ErrQuoteNotOrderable
		}
		switch line.Domain {
		case "FOOD":
			add("food-items", line.ItemRef, group.BusinessID)
		case "SHOP":
			add("shop-listings", line.ListingRef, group.BusinessID)
		}
	}
	keys := make([]string, 0, len(targets))
	for k := range targets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		t := targets[key]
		// Fixed source dictionaries only; a quote/request cannot supply SQL.
		query := map[string]string{
			"merchants":     `SELECT status FROM vendor_businesses WHERE id=$1 AND id=$2 FOR SHARE`,
			"branches":      `SELECT status FROM vendor_branches WHERE id=$1 AND business_id=$2 FOR SHARE`,
			"food-items":    `SELECT status FROM food_items WHERE id=$1 AND business_id=$2 FOR SHARE`,
			"shop-listings": `SELECT status FROM shop_branch_listings WHERE id=$1 AND business_id=$2 FOR SHARE`,
		}[t.kind]
		var state string
		err := tx.QueryRow(ctx, query, t.id, t.business).Scan(&state)
		if errors.Is(err, pgx.ErrNoRows) {
			// Legacy quote authorities can carry historical references without a
			// currently materialized catalogue row. They never become administerable
			// records: an administered/deleted reference must fail closed.
			var administered bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_owner_command_decisions WHERE owner='vendor-service' AND subject_kind=$1 AND subject_id=$2 AND action IN ('PAUSE_MERCHANT','REOPEN_MERCHANT','PAUSE_BRANCH','REOPEN_BRANCH','TAKE_DOWN_LISTING','RESTORE_LISTING'))`, t.kind, t.id).Scan(&administered); err != nil {
				return err
			}
			if administered {
				return ErrQuoteNotOrderable
			}
			continue
		}
		if err != nil {
			return err
		}
		if state != "ACTIVE" {
			return ErrQuoteNotOrderable
		}
	}
	for _, group := range groups {
		eligible, _, err := vendors.AdministrativeBranchDomainEligibility(ctx, tx, group.BusinessID, group.BranchID, group.Domain)
		if err != nil {
			return err
		}
		if !eligible {
			return ErrQuoteNotOrderable
		}
	}
	return nil
}
