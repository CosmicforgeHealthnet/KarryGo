package main

// Basket optimization adapters.
//
// The optimizer states its needs as narrow ports and knows nothing about SQL.
// These are the implementations, and they live here for the same reason the
// Section 17C adapters do: the composition root is where a feature is allowed
// to learn what a table is called.
//
// The catalogue adapter is the important one, and its whole job is to hand
// back ONLY listings that can actually be bought right now. Every filter in
// its query exists because a candidate that cannot be bought is worse than no
// candidate - it produces a basket the customer is then told does not exist.

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/basket"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
)

// -----------------------------------------------------------------------------
// Catalogue
// -----------------------------------------------------------------------------

type basketCatalogue struct {
	db *pgxpool.Pool

	// logistics resolves each candidate's physical profile through the SAME
	// authority checkout planning uses.
	//
	// Sharing it is the point. A basket priced for delivery against different
	// physics from the one a real checkout would use is a basket whose quoted
	// delivery is fiction, and two implementations of "how heavy is this" is
	// how that starts.
	logistics *section17CLogisticsAuthority
}

func newBasketCatalogue(db *pgxpool.Pool) *basketCatalogue {
	return &basketCatalogue{
		db:        db,
		logistics: newSection17CLogisticsAuthority(db),
	}
}

// candidateSQL selects buyable listings.
//
// Every predicate below is load-bearing:
//
//	l.status = 'ACTIVE'          an archived or paused listing is not for sale
//	l.retail_enabled             a wholesale-only listing is not a retail offer
//	l.logistics_available        the vendor has said this cannot be delivered
//	b.status = 'ACTIVE'          a closed branch cannot be collected from
//	p.status = 'ACTIVE'          a withdrawn product is not sellable
//	lsu.enabled                  a disabled sell unit has no price to quote
//	available > 0                stock that is reserved or quarantined is not
//	                             sellable, so on_hand alone would overstate it
//
// The price ladder mirrors the Shop pricing authority's own precedence:
// listing-unit override, then the sell unit's own price, then the listing
// override, then the variant's base retail price. Nothing is invented and no
// discount is applied here - promotions are the promotion engine's business
// and are evaluated at quote time, not by an optimizer.
const candidateSQL = `
	SELECT l.id, l.product_id, l.variant_id, l.business_id, l.branch_id,
	       su.id, su.display_name,
	       p.name, COALESCE(p.brand, ''),
	       COALESCE(
	           lsu.price_override_minor,
	           su.price_minor,
	           l.retail_price_override_minor,
	           v.retail_price_minor
	       ) AS unit_price_minor,
	       biz.currency,
	       COALESCE(bal.on_hand_quantity, 0)
	           - COALESCE(bal.reserved_quantity, 0)
	           - COALESCE(bal.quarantined_quantity, 0) AS available,
	       COALESCE(bal.revision, 0),
	       l.version,
	       COALESCE(map.node_id, ''),
	       COALESCE(
	           ARRAY(
	               SELECT i.identifier_value
	                 FROM shop_product_identifiers i
	                WHERE i.variant_id = l.variant_id
	                  AND i.status = 'ACTIVE'
	                  AND i.identifier_type IN ('GTIN', 'EAN', 'UPC', 'BARCODE')
	                ORDER BY i.identifier_value
	           ),
	           ARRAY[]::text[]
	       ) AS trade_identifiers,
	       COALESCE(
	           ARRAY(
	               SELECT o.option_value
	                 FROM shop_fresh_attributes fa
	                 JOIN shop_fresh_attribute_options o
	                   ON o.attribute_id = fa.id AND o.status = 'ACTIVE'
	                WHERE fa.product_id = l.product_id
	                  AND fa.business_id = l.business_id
	                  AND fa.attribute_type = 'QUALITY_GRADE'
	                  AND fa.status = 'ACTIVE'
	                ORDER BY o.position, o.option_value
	           ),
	           ARRAY[]::text[]
	       ) AS quality_grades
	  FROM shop_branch_listings l
	  JOIN shop_products p
	    ON p.id = l.product_id AND p.business_id = l.business_id
	  JOIN shop_variants v
	    ON v.id = l.variant_id AND v.business_id = l.business_id
	  JOIN vendor_branches b
	    ON b.id = l.branch_id AND b.business_id = l.business_id
	  JOIN vendor_businesses biz
	    ON biz.id = l.business_id
	  JOIN shop_listing_sell_units lsu
	    ON lsu.listing_id = l.id AND lsu.business_id = l.business_id
	  JOIN shop_sell_units su
	    ON su.id = lsu.sell_unit_id AND su.business_id = l.business_id
	  LEFT JOIN shop_inventory_balances bal
	    ON bal.business_id = l.business_id
	   AND bal.branch_id = l.branch_id
	   AND bal.variant_id = l.variant_id
	  LEFT JOIN canonical_taxonomy_mappings map
	    ON map.source_type = 'SHOP_CATEGORY'
	   AND map.source_ref = p.category_id
	   AND map.mapping_type = 'PRIMARY'
	   AND map.status = 'ACTIVE'
	 WHERE l.status = 'ACTIVE'
	   AND l.retail_enabled
	   AND l.logistics_available
	   AND p.status = 'ACTIVE'
	   AND b.status = 'ACTIVE'
	   AND lsu.enabled
	   AND su.retail_enabled
	   AND biz.currency = $1
	   AND COALESCE(bal.on_hand_quantity, 0)
	       - COALESCE(bal.reserved_quantity, 0)
	       - COALESCE(bal.quarantined_quantity, 0) > 0
	   AND COALESCE(
	           lsu.price_override_minor,
	           su.price_minor,
	           l.retail_price_override_minor,
	           v.retail_price_minor
	       ) IS NOT NULL`

// FindCandidates implements basket.CatalogueAuthority.
//
// The requirement decides which extra predicate narrows the search. A
// requirement naming a listing narrows to that listing; one naming a product
// narrows to that product across vendors; one naming a category narrows
// through the taxonomy authority's own mapping. There is deliberately no
// name-matching branch: two products with similar names are not evidence of
// anything, and a LIKE over display names is exactly how an optimizer starts
// substituting condensed milk for fresh.
func (a *basketCatalogue) FindCandidates(
	ctx context.Context,
	query basket.CandidateQuery,
) ([]basket.Listing, error) {
	if a == nil || a.db == nil {
		return nil, nil
	}

	currency := strings.ToUpper(strings.TrimSpace(query.Currency))

	args := []any{currency}

	var filter string

	requirement := query.Requirement

	switch {
	case requirement.ListingRef != "" &&
		requirement.Substitution == basket.AllowExactOnly:
		args = append(args, requirement.ListingRef)
		filter = ` AND l.id = $2`

	case requirement.VariantRef != "":
		args = append(args, requirement.VariantRef)
		filter = ` AND l.variant_id = $2`

	case requirement.ProductRef != "":
		args = append(args, requirement.ProductRef)
		filter = ` AND l.product_id = $2`

	case requirement.CategoryRef != "":
		args = append(args, requirement.CategoryRef)
		filter = ` AND map.node_id = $2`

	case requirement.ListingRef != "":
		// A named listing under a widened allowance: the listing itself, plus
		// anything sharing its product. The product is resolved through the
		// listing rather than trusted from the request, so a customer cannot
		// widen their own search into another product by asserting one.
		args = append(args, requirement.ListingRef)
		filter = ` AND (l.id = $2 OR l.product_id = (
			SELECT product_id FROM shop_branch_listings WHERE id = $2
		))`

	default:
		return nil, nil
	}

	limit := query.Limit

	if limit <= 0 || limit > basket.MaxCandidateListingsPerNeed {
		limit = basket.MaxCandidateListingsPerNeed
	}

	args = append(args, requirement.SellUnitRef, limit)

	// Cheapest first, then a total order on the listing reference so the same
	// catalogue always returns the same shortlist. Without the tie-break the
	// optimizer would be non-deterministic in a way no test reliably catches.
	sql := candidateSQL + filter + `
	   AND ($` + itoa(len(args)-1) + ` = '' OR su.id = $` + itoa(len(args)-1) + `)
	 ORDER BY unit_price_minor, l.id, su.id
	 LIMIT $` + itoa(len(args))

	rows, err := a.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var listings []basket.Listing

	for rows.Next() {
		var listing basket.Listing

		if err = rows.Scan(
			&listing.ListingRef, &listing.ProductRef, &listing.VariantRef,
			&listing.BusinessID, &listing.BranchID,
			&listing.SellUnitRef, &listing.SellUnitName,
			&listing.DisplayName, &listing.Brand,
			&listing.UnitPriceMinor, &listing.Currency,
			&listing.AvailableQuantity, &listing.InventoryRevision,
			&listing.CatalogueRevision, &listing.CanonicalNodeRef,
			&listing.TradeIdentifiers, &listing.QualityGrades,
		); err != nil {
			return nil, err
		}

		listing.PriceReference = listing.ListingRef + ":" + listing.SellUnitRef

		listings = append(listings, listing)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return a.attachLogistics(ctx, listings)
}

// attachLogistics fills in each candidate's physical profile.
//
// Batched per business rather than per listing: a twelve-candidate shortlist
// across four vendors is four calls, not twelve. The optimizer evaluates
// several baskets per request, so an N+1 here would multiply straight through
// into the request latency.
func (a *basketCatalogue) attachLogistics(
	ctx context.Context,
	listings []basket.Listing,
) ([]basket.Listing, error) {
	if len(listings) == 0 {
		return listings, nil
	}

	byBusiness := map[string][]fulfilment.LineLogistics{}

	for index, listing := range listings {
		byBusiness[listing.BusinessID] = append(
			byBusiness[listing.BusinessID], fulfilment.LineLogistics{
				CheckoutLineID: itoa(index),
				Domain:         "SHOP",
				BusinessID:     listing.BusinessID,
				BranchID:       listing.BranchID,
				ItemRef:        listing.ProductRef,
				VariantRef:     listing.VariantRef,
				ListingRef:     listing.ListingRef,
				SellUnitRef:    listing.SellUnitRef,
				Quantity:       1,
			},
		)
	}

	for businessID, lines := range byBusiness {
		profiles, err := a.logistics.ResolveLineProfiles(
			ctx, businessID, lines,
		)
		if err != nil {
			return nil, err
		}

		for _, line := range lines {
			index := atoi(line.CheckoutLineID)

			if index < 0 || index >= len(listings) {
				continue
			}

			// A listing with no stated profile keeps a zero one, which the
			// requirement engine reports as missing evidence rather than
			// treating as weightless.
			listings[index].Logistics = profiles[line.CheckoutLineID]
		}
	}

	return listings, nil
}

// ResolveListings implements basket.CatalogueAuthority.
//
// Used to reprice a previous recurring basket against today. A reference that
// no longer resolves is simply absent from the map: a listing withdrawn
// between one week and the next is an ordinary event with a correct answer,
// not a failure.
func (a *basketCatalogue) ResolveListings(
	ctx context.Context,
	refs []string,
) (map[string]basket.Listing, error) {
	out := map[string]basket.Listing{}

	if a == nil || a.db == nil || len(refs) == 0 {
		return out, nil
	}

	for _, ref := range refs {
		ref = strings.TrimSpace(ref)

		if ref == "" {
			continue
		}

		listings, err := a.FindCandidates(ctx, basket.CandidateQuery{
			Requirement: basket.Requirement{
				ListingRef:   ref,
				Quantity:     1,
				Substitution: basket.AllowExactOnly,
			},
			Limit: 1,
		})
		if err != nil {
			return nil, err
		}

		for _, listing := range listings {
			if listing.ListingRef == ref {
				out[ref] = listing
			}
		}
	}

	return out, nil
}

// -----------------------------------------------------------------------------
// Cart
// -----------------------------------------------------------------------------

type basketCarts struct {
	db *pgxpool.Pool
}

func newBasketCarts(db *pgxpool.Pool) *basketCarts {
	return &basketCarts{db: db}
}

// LoadForOptimization implements basket.CartAuthority.
func (a *basketCarts) LoadForOptimization(
	ctx context.Context,
	customerID string,
) (basket.CartSnapshot, error) {
	if a == nil || a.db == nil {
		return basket.CartSnapshot{}, basket.ErrDependencyMissing
	}

	var (
		snapshot basket.CartSnapshot
		address  basket.Destination
	)

	err := a.db.QueryRow(ctx, `
		SELECT c.id, c.version,
		       COALESCE(a.source_address_ref, ''),
		       COALESCE(a.address_snapshot, ''),
		       COALESCE(a.latitude_snapshot, 0),
		       COALESCE(a.longitude_snapshot, 0)
		  FROM commerce_carts c
		  LEFT JOIN commerce_cart_addresses a ON a.cart_id = c.id
		 WHERE c.customer_id = $1 AND c.status = 'ACTIVE'
		   AND c.domain = 'SHOP'
		 ORDER BY c.updated_at DESC
		 LIMIT 1`,
		strings.TrimSpace(customerID),
	).Scan(
		&snapshot.CartID, &snapshot.Version,
		&address.AddressRef, &address.Address,
		&address.Latitude, &address.Longitude,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return basket.CartSnapshot{}, basket.ErrNoRequirements
	}
	if err != nil {
		return basket.CartSnapshot{}, err
	}

	snapshot.Destination = address

	rows, err := a.db.Query(ctx, `
		SELECT id, COALESCE(listing_ref, ''), item_ref,
		       COALESCE(variant_ref, ''), COALESCE(sell_unit_ref, ''),
		       business_id, branch_id, display_name, quantity,
		       unit_price_minor_snapshot, currency_snapshot
		  FROM commerce_cart_items
		 WHERE cart_id = $1
		 ORDER BY id`,
		snapshot.CartID,
	)
	if err != nil {
		return basket.CartSnapshot{}, err
	}

	defer rows.Close()

	for rows.Next() {
		var line basket.CartLine

		if err = rows.Scan(
			&line.LineID, &line.ListingRef, &line.ProductRef,
			&line.VariantRef, &line.SellUnitRef, &line.BusinessID,
			&line.BranchID, &line.DisplayName, &line.Quantity,
			&line.UnitPriceMinor, &line.Currency,
		); err != nil {
			return basket.CartSnapshot{}, err
		}

		if snapshot.Currency == "" {
			snapshot.Currency = line.Currency
		}

		snapshot.Lines = append(snapshot.Lines, line)
	}

	return snapshot, rows.Err()
}

// ApplyProposal implements basket.CartAuthority.
//
// The whole write is one transaction guarded by the expected version, and the
// version is bumped as part of it. That is what makes acceptance produce a
// NEW cart version rather than an edit of the old one, and what makes a
// concurrent change refuse the write instead of losing it.
func (a *basketCarts) ApplyProposal(
	ctx context.Context,
	customerID string,
	expectedVersion int64,
	lines []basket.ProposedLine,
) (basket.CartSnapshot, error) {
	if a == nil || a.db == nil {
		return basket.CartSnapshot{}, basket.ErrDependencyMissing
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return basket.CartSnapshot{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	var (
		cartID  string
		version int64
	)

	// The expected version is part of the WHERE clause rather than checked
	// beforehand, so two concurrent acceptances cannot both pass the check
	// and both write.
	err = tx.QueryRow(ctx, `
		UPDATE commerce_carts
		   SET version = version + 1, updated_at = now()
		 WHERE customer_id = $1 AND status = 'ACTIVE' AND domain = 'SHOP'
		   AND version = $2
		RETURNING id, version`,
		strings.TrimSpace(customerID), expectedVersion,
	).Scan(&cartID, &version)

	if errors.Is(err, pgx.ErrNoRows) {
		return basket.CartSnapshot{}, basket.ErrProposalStale
	}
	if err != nil {
		return basket.CartSnapshot{}, err
	}

	if _, err = tx.Exec(ctx,
		`DELETE FROM commerce_cart_items WHERE cart_id = $1`, cartID,
	); err != nil {
		return basket.CartSnapshot{}, err
	}

	for index, line := range lines {
		if _, err = tx.Exec(ctx, `
			INSERT INTO commerce_cart_items (
				id, cart_id, business_id, branch_id, domain, item_ref,
				variant_ref, listing_ref, sell_unit_ref, quantity,
				unit_price_minor_snapshot, currency_snapshot,
				price_reference, catalogue_revision, display_name,
				pricing_basis
			) VALUES ($1,$2,$3,$4,'SHOP',$5,
			          NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,
			          $10,$11,$12,$13,$14,'UNIT')`,
			cartID+":"+itoa(index), cartID, line.BusinessID, line.BranchID,
			line.ProductRef, line.VariantRef, line.ListingRef,
			line.SellUnitRef, line.Quantity, line.UnitPriceMinor,
			line.Currency, line.PriceReference, line.CatalogueRevision,
			line.DisplayName,
		); err != nil {
			return basket.CartSnapshot{}, err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return basket.CartSnapshot{}, err
	}

	return a.LoadForOptimization(ctx, customerID)
}

// -----------------------------------------------------------------------------
// Saved addresses
// -----------------------------------------------------------------------------

type basketAddresses struct {
	db *pgxpool.Pool
}

func newBasketAddresses(db *pgxpool.Pool) *basketAddresses {
	return &basketAddresses{db: db}
}

// ResolveSavedAddress implements basket.AddressResolver.
//
// Scoped by customer AND reference. A customer must never be able to price a
// delivery to an address they have no connection to, both because it is not
// theirs and because serviceability answers about arbitrary points on a map
// are information they have not earned.
func (a *basketAddresses) ResolveSavedAddress(
	ctx context.Context,
	customerID, addressRef string,
) (basket.Destination, error) {
	if a == nil || a.db == nil {
		return basket.Destination{}, basket.ErrDependencyMissing
	}

	destination := basket.Destination{AddressRef: addressRef}

	err := a.db.QueryRow(ctx, `
		SELECT COALESCE(address_snapshot, ''),
		       COALESCE(latitude_snapshot, 0),
		       COALESCE(longitude_snapshot, 0)
		  FROM commerce_cart_addresses a
		  JOIN commerce_carts c ON c.id = a.cart_id
		 WHERE c.customer_id = $1 AND a.source_address_ref = $2
		 ORDER BY a.snapshotted_at DESC
		 LIMIT 1`,
		strings.TrimSpace(customerID), strings.TrimSpace(addressRef),
	).Scan(&destination.Address, &destination.Latitude, &destination.Longitude)

	if errors.Is(err, pgx.ErrNoRows) {
		return basket.Destination{}, basket.ErrNoDestination
	}
	if err != nil {
		return basket.Destination{}, err
	}

	return destination, nil
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}

	negative := value < 0

	if negative {
		value = -value
	}

	var digits []byte

	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}

	if negative {
		return "-" + string(digits)
	}

	return string(digits)
}

func atoi(value string) int {
	result := 0

	for _, character := range value {
		if character < '0' || character > '9' {
			return -1
		}

		result = result*10 + int(character-'0')
	}

	return result
}
