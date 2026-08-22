package main

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
)

// Section 17B adapters.
//
// Each one turns an existing authority into the narrow port the checkout
// feature declares. They deliberately live in the composition root: checkout
// must not import Food, Shop or Inventory, and none of those three learns
// anything about checkout.
//
// The Section 17A adapters already resolve a business from a catalogue
// reference and read Shop sell-unit and price-version evidence. Those helpers
// are reused rather than copied - two implementations of "which vendor owns
// this listing" would be two places for the answer to drift.

// -----------------------------------------------------------------------------
// Reason codes
// -----------------------------------------------------------------------------

const (
	// section17BReasonCatalogueGone is the answer when a catalogue reference
	// the cart holds no longer resolves at all.
	section17BReasonCatalogueGone = "CATALOGUE_ITEM_GONE"

	// section17BReasonPriceUnresolved is the answer when an item is
	// administratively sellable but has no authoritative current price.
	section17BReasonPriceUnresolved = "PRICE_UNRESOLVED"

	// section17BReasonNotSellable is the fallback when an authority refuses
	// without naming a reason of its own.
	section17BReasonNotSellable = "NOT_SELLABLE"
)

// shopStockDeferredReasons are the Section 12B sellability outcomes Section
// 17B deliberately does NOT act on, because Section 14 answers the same
// question authoritatively a moment later.
//
// The list is an allowlist rather than a list of denials, and that direction
// is the point. Shop owns its own reason vocabulary and may grow it; if a new
// catalogue denial appeared and Section 17B were denylisting, the new reason
// would fall through and sell. Deferring only these, and refusing everything
// else it does not recognise, means a Section 12 change fails closed here
// instead of silently opening a hole.
//
// Asking two ledgers the same stock question is how a checkout ends up
// holding stock one of them never had, which is why the stock answers below
// are dropped rather than merged.
var shopStockDeferredReasons = map[string]bool{
	// Nothing to defer: Shop is satisfied.
	"SELLABLE": true,

	// Stock answers, all owned by Section 14 for this service.
	"OUT_OF_STOCK":                    true,
	"INSUFFICIENT_STOCK":              true,
	"BACKORDER_AVAILABLE":             true,
	"PREORDER_AVAILABLE":              true,
	"INVENTORY_CONVERSION_UNRESOLVED": true,
}

// -----------------------------------------------------------------------------
// Food
// -----------------------------------------------------------------------------

// section17BFoodAuthority revalidates one Food line.
//
// Food's own sellability resolver already answers business, branch,
// capability, menu, schedule, section, item, operational state and the
// INVENTORY_LINKED production seam as one decision, so Section 17B enforces
// that decision rather than re-deriving any part of it. Section 17A
// deliberately let an unsellable item sit in a cart; checkout is where that
// stops being allowed.
type section17BFoodAuthority struct {
	catalogue *section17AFoodAuthority
	service   *food.Service
}

func newSection17BFoodAuthority(
	db *pgxpool.Pool,
	service *food.Service,
) *section17BFoodAuthority {
	return &section17BFoodAuthority{
		catalogue: newSection17AFoodAuthority(db, service),
		service:   service,
	}
}

func section17BDenied(
	reasonCode string,
	detail string,
) checkout.LineDecision {
	return checkout.LineDecision{
		Sellable:     false,
		ReasonCode:   reasonCode,
		ReasonDetail: detail,
	}
}

func (a *section17BFoodAuthority) RevalidateFoodLine(
	ctx context.Context,
	request checkout.LineRequest,
) (checkout.LineDecision, error) {
	if a == nil || a.service == nil || a.catalogue == nil {
		return checkout.LineDecision{}, checkout.ErrDependencyMissing
	}

	itemRef := strings.TrimSpace(request.ItemRef)

	if itemRef == "" {
		return section17BDenied(
			section17BReasonCatalogueGone,
			"This item is no longer on the menu.",
		), nil
	}

	businessID, err := a.catalogue.businessForItem(ctx, itemRef)

	if err != nil {
		if errors.Is(err, cart.ErrInvalidCatalogueItem) {
			return section17BDenied(
				section17BReasonCatalogueGone,
				"This item is no longer on the menu.",
			), nil
		}

		return checkout.LineDecision{}, err
	}

	actor := food.Actor{VendorID: businessID}
	scope := food.Scope{Global: true}

	item, _, currency, err := a.service.GetItem(
		ctx,
		actor,
		scope,
		itemRef,
	)
	if err != nil {
		return section17BDenied(
			section17BReasonCatalogueGone,
			"This item is no longer on the menu.",
		), nil
	}

	if item.ID != itemRef ||
		item.BusinessID != businessID ||
		item.BranchID == "" ||
		item.MenuID == "" {
		return section17BDenied(
			section17BReasonCatalogueGone,
			"This item is no longer on the menu.",
		), nil
	}

	if requested := strings.TrimSpace(request.MenuRef); requested != "" &&
		requested != item.MenuID {
		return section17BDenied(
			checkout.ReasonGroupingViolation,
			"This item has moved to a different menu.",
		), nil
	}

	// The service window is evaluated at the session's single instant, so
	// every line in one checkout sees the same clock.
	sellability, err := a.service.ResolveSellability(
		ctx,
		actor,
		scope,
		item.ID,
		request.At,
	)
	if err != nil {
		return checkout.LineDecision{}, err
	}

	if sellability.ItemRef != item.ID ||
		sellability.BranchID != item.BranchID ||
		sellability.MenuRef != item.MenuID {
		return section17BDenied(
			checkout.ReasonGroupingViolation,
			"This item has moved to a different branch or menu.",
		), nil
	}

	if !sellability.Sellable {
		return section17BDenied(
			firstNonBlank(
				sellability.ReasonCode,
				section17BReasonNotSellable,
			),
			sellability.Detail,
		), nil
	}

	// Modifier selections are re-proved to belong to this vendor and are
	// expanded into the exact reference list the price resolver expects. The
	// per-option display names Section 17A snapshots are deliberately not
	// carried here: checkout revalidates commercial truth, and the cart
	// already holds what the customer was shown.
	modifierRefs, err := a.resolveModifiers(
		ctx,
		actor,
		businessID,
		request.Options,
	)
	if err != nil {
		if errors.Is(err, cart.ErrInvalidCatalogueItem) {
			return section17BDenied(
				section17BReasonCatalogueGone,
				"A selected option is no longer available.",
			), nil
		}

		return checkout.LineDecision{}, err
	}

	resolved, err := a.service.ResolvePrice(
		ctx,
		actor,
		scope,
		item.ID,
		food.ResolvePriceInput{
			VariantRef:   strings.TrimSpace(request.VariantRef),
			ModifierRefs: modifierRefs,
		},
	)
	if err != nil {
		return section17BDenied(
			section17BReasonPriceUnresolved,
			"This item has no current price.",
		), nil
	}

	if resolved.ItemRef != item.ID {
		return section17BDenied(
			section17BReasonCatalogueGone,
			"This item is no longer on the menu.",
		), nil
	}

	if requestedVariant := strings.TrimSpace(request.VariantRef); requestedVariant != "" &&
		resolved.VariantRef != requestedVariant {
		return section17BDenied(
			section17BReasonCatalogueGone,
			"The selected size is no longer available.",
		), nil
	}

	priceReference := section17AFoodPriceReference(resolved)

	if priceReference == "" {
		// A checkout candidate must point at real immutable price evidence.
		// Never fabricate one.
		return section17BDenied(
			section17BReasonPriceUnresolved,
			"This item has no current price record.",
		), nil
	}

	catalogueRevision := resolved.CatalogueRevision

	if sellability.CatalogueRevision > catalogueRevision {
		catalogueRevision = sellability.CatalogueRevision
	}

	return checkout.LineDecision{
		Sellable:   true,
		ReasonCode: checkout.ReasonSellable,

		BusinessID: item.BusinessID,
		BranchID:   item.BranchID,

		DisplayName: item.Name,

		UnitPriceMinor:    resolved.EffectiveUnitPriceMinor,
		Currency:          currency,
		PriceReference:    priceReference,
		CatalogueRevision: catalogueRevision,

		// Food reserves nothing in Section 17B. Its production architecture
		// exposes availability, not a reservation interface, and reserving
		// raw Shop stock on its behalf would be inventing a contract.
		ReservationBasis: checkout.BasisNone,
	}, nil
}

func (a *section17BFoodAuthority) resolveModifiers(
	ctx context.Context,
	actor food.Actor,
	businessID string,
	options []checkout.OptionRequest,
) ([]string, error) {
	modifierRefs := make([]string, 0, len(options))

	for _, option := range options {
		groupRef := strings.TrimSpace(option.GroupRef)
		optionRef := strings.TrimSpace(option.OptionRef)

		if groupRef == "" ||
			optionRef == "" ||
			option.Quantity < 1 ||
			option.Quantity > cart.MaxOptionQuantity {
			return nil, cart.ErrInvalidCatalogueItem
		}

		group, err := a.service.GetModifierGroup(ctx, actor, groupRef)

		if err != nil {
			return nil, cart.ErrInvalidCatalogueItem
		}

		if group.ID != groupRef || group.BusinessID != businessID {
			return nil, cart.ErrInvalidCatalogueItem
		}

		if int64(len(modifierRefs))+option.Quantity > 200 {
			return nil, cart.ErrInvalidCatalogueItem
		}

		for i := int64(0); i < option.Quantity; i++ {
			modifierRefs = append(modifierRefs, optionRef)
		}
	}

	return modifierRefs, nil
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

// -----------------------------------------------------------------------------
// Shop
// -----------------------------------------------------------------------------

// section17BShopSellUnit is the sell-unit evidence Section 17B needs beyond
// what Section 17A read: the fraction and the measurement tolerances.
//
// The fraction matters because a "half paint" sell unit and a "paint" stock
// basis are the same measure but not the same quantity, and Vendor Service
// has no authority anywhere that converts between them.
type section17BShopSellUnit struct {
	ID                  string
	MeasureID           string
	FractionNumerator   int
	FractionDenominator int
	MeasurementMode     string
	MeasurementRequired bool
	UnderfillBps        int
	OverfillBps         int
	DisplayName         string
	Status              string
}

// section17BShopAuthority revalidates one Shop line and resolves the Section
// 14 reservation basis it needs.
type section17BShopAuthority struct {
	db        *pgxpool.Pool
	catalogue *section17AShopAuthority
	service   *shop.Service
	stock     *inventory.StockService
}

func newSection17BShopAuthority(
	db *pgxpool.Pool,
	service *shop.Service,
	stock *inventory.StockService,
) *section17BShopAuthority {
	return &section17BShopAuthority{
		db:        db,
		catalogue: newSection17AShopAuthority(db, service),
		service:   service,
		stock:     stock,
	}
}

func (a *section17BShopAuthority) loadSellUnit(
	ctx context.Context,
	businessID string,
	sellUnitRef string,
) (section17BShopSellUnit, error) {
	var unit section17BShopSellUnit

	err := a.db.QueryRow(
		ctx,
		`
        SELECT
            id,
            measure_definition_id,
            fraction_numerator,
            fraction_denominator,
            measurement_mode,
            actual_measurement_required,
            underfill_tolerance_bps,
            overfill_tolerance_bps,
            display_name,
            status
        FROM shop_sell_units
        WHERE id=$1
          AND business_id=$2
        `,
		strings.TrimSpace(sellUnitRef),
		businessID,
	).Scan(
		&unit.ID,
		&unit.MeasureID,
		&unit.FractionNumerator,
		&unit.FractionDenominator,
		&unit.MeasurementMode,
		&unit.MeasurementRequired,
		&unit.UnderfillBps,
		&unit.OverfillBps,
		&unit.DisplayName,
		&unit.Status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return section17BShopSellUnit{}, cart.ErrInvalidCatalogueItem
	}

	return unit, err
}

// section17BShopTenantStanding is the vendor-standing check Section 12 does
// not perform.
//
// The required checkout sequence puts "validate Vendor active" and "validate
// branch active" ahead of catalogue, price and stock. Food's resolver answers
// both as part of one sellability decision. Shop's `requireShopBranch` reads
// the branch status and never inspects it, admits a capability in DRAFT or
// PAUSED, and never reads the business row at all - which is correct for a
// vendor still building a catalogue, and wrong for selling from one. Without
// this, a SUSPENDED vendor whose catalogue rows are untouched validates a
// checkout and holds real stock.
func (a *section17BShopAuthority) tenantStanding(
	ctx context.Context,
	businessID string,
	branchID string,
) (checkout.LineDecision, bool, error) {
	var (
		businessStatus   string
		branchStatus     string
		capabilityActive bool
	)

	err := a.db.QueryRow(
		ctx,
		`
        SELECT
            biz.status,
            br.status,
            EXISTS(
                SELECT 1
                FROM vendor_branch_capabilities bc
                WHERE bc.business_id = br.business_id
                  AND bc.branch_id = br.id
                  AND bc.capability_type = ANY($3)
                  AND bc.status = 'ACTIVE'
            )
        FROM vendor_branches br
        JOIN vendor_businesses biz ON biz.id = br.business_id
        WHERE br.business_id = $1
          AND br.id = $2
        `,
		businessID,
		branchID,
		shop.ShopCapabilities,
	).Scan(
		&businessStatus,
		&branchStatus,
		&capabilityActive,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return section17BDenied(
			checkout.ReasonBranchInactive,
			"This branch is no longer trading.",
		), false, nil
	}

	if err != nil {
		return checkout.LineDecision{}, false, err
	}

	if businessStatus != "ACTIVE" {
		return section17BDenied(
			checkout.ReasonBusinessInactive,
			"This vendor is not currently trading.",
		), false, nil
	}

	if branchStatus != "ACTIVE" {
		return section17BDenied(
			checkout.ReasonBranchInactive,
			"This branch is not currently trading.",
		), false, nil
	}

	if !capabilityActive {
		return section17BDenied(
			checkout.ReasonCapabilityInactive,
			"This branch is not currently selling from its shop.",
		), false, nil
	}

	return checkout.LineDecision{}, true, nil
}

func (a *section17BShopAuthority) RevalidateShopLine(
	ctx context.Context,
	request checkout.LineRequest,
) (checkout.LineDecision, error) {
	if a == nil ||
		a.db == nil ||
		a.service == nil ||
		a.stock == nil ||
		a.catalogue == nil {
		return checkout.LineDecision{}, checkout.ErrDependencyMissing
	}

	listingRef := strings.TrimSpace(request.ListingRef)
	sellUnitRef := strings.TrimSpace(request.SellUnitRef)

	if listingRef == "" || sellUnitRef == "" {
		return section17BDenied(
			section17BReasonCatalogueGone,
			"This product is no longer listed.",
		), nil
	}

	businessID, err := a.catalogue.businessForListing(ctx, listingRef)

	if err != nil {
		if errors.Is(err, cart.ErrInvalidCatalogueItem) {
			return section17BDenied(
				section17BReasonCatalogueGone,
				"This product is no longer listed.",
			), nil
		}

		return checkout.LineDecision{}, err
	}

	actor := shop.Actor{VendorID: businessID}
	scope := shop.Scope{Global: true}

	listing, listingUnits, err := a.service.GetListing(
		ctx,
		actor,
		scope,
		listingRef,
	)
	if err != nil {
		return section17BDenied(
			section17BReasonCatalogueGone,
			"This product is no longer listed.",
		), nil
	}

	if listing.ID != listingRef ||
		listing.BusinessID != businessID ||
		listing.ProductID != strings.TrimSpace(request.ItemRef) ||
		listing.VariantID != strings.TrimSpace(request.VariantRef) ||
		listing.BranchID == "" {
		return section17BDenied(
			checkout.ReasonGroupingViolation,
			"This product listing has changed.",
		), nil
	}

	// Vendor standing, before anything commercial is read. A suspended vendor
	// or a closed branch is not a catalogue question and must not depend on
	// one.
	if denied, ok, standingErr := a.tenantStanding(
		ctx,
		businessID,
		listing.BranchID,
	); standingErr != nil {
		return checkout.LineDecision{}, standingErr
	} else if !ok {
		return denied, nil
	}

	enabledUnit := false

	for _, candidate := range listingUnits {
		if candidate.SellUnitID != sellUnitRef {
			continue
		}

		enabledUnit = candidate.BusinessID == businessID &&
			candidate.ListingID == listing.ID &&
			candidate.VariantID == listing.VariantID &&
			candidate.Enabled

		break
	}

	if !enabledUnit {
		return section17BDenied(
			"SELL_UNIT_INACTIVE",
			"This selling unit is no longer offered.",
		), nil
	}

	product, err := a.service.GetProduct(ctx, actor, scope, listing.ProductID)

	if err != nil {
		return section17BDenied(
			section17BReasonCatalogueGone,
			"This product is no longer listed.",
		), nil
	}

	variant, err := a.service.GetVariant(ctx, actor, scope, listing.VariantID)

	if err != nil {
		return section17BDenied(
			section17BReasonCatalogueGone,
			"This product is no longer listed.",
		), nil
	}

	unit, err := a.loadSellUnit(ctx, businessID, sellUnitRef)

	if err != nil {
		if errors.Is(err, cart.ErrInvalidCatalogueItem) {
			return section17BDenied(
				"SELL_UNIT_INACTIVE",
				"This selling unit is no longer offered.",
			), nil
		}

		return checkout.LineDecision{}, err
	}

	if unit.Status != "ACTIVE" {
		return section17BDenied(
			"SELL_UNIT_INACTIVE",
			"This selling unit is no longer offered.",
		), nil
	}

	// Catalogue-side sellability. Stock reason codes are deliberately not
	// treated as a refusal here; Section 14 answers that question below.
	sellability, err := a.service.ResolveSellability(
		ctx,
		actor,
		scope,
		listing.BranchID,
		variant.ID,
		unit.ID,
		request.Quantity,
		false,
	)
	if err != nil {
		return section17BDenied(
			firstNonBlank(
				sellability.ReasonCode,
				section17BReasonNotSellable,
			),
			"This product cannot currently be sold from this branch.",
		), nil
	}

	if !shopStockDeferredReasons[sellability.ReasonCode] {
		return section17BDenied(
			firstNonBlank(
				sellability.ReasonCode,
				section17BReasonNotSellable,
			),
			"This product cannot currently be sold in the requested quantity.",
		), nil
	}

	price, err := a.service.ResolvePrice(
		ctx,
		actor,
		scope,
		listing.ID,
		unit.ID,
		request.Quantity,
		false,
	)
	if err != nil {
		return section17BDenied(
			section17BReasonPriceUnresolved,
			"This product has no current price.",
		), nil
	}

	if price.ListingRef != listing.ID ||
		price.BranchRef != listing.BranchID ||
		price.VariantRef != variant.ID ||
		price.SellUnitRef != unit.ID ||
		price.UnitPriceMinor == nil ||
		!price.Satisfied {
		return section17BDenied(
			section17BReasonPriceUnresolved,
			"This product has no current price.",
		), nil
	}

	priceReference, err := a.catalogue.latestPriceReference(
		ctx,
		businessID,
		listing.ID,
		listingUnitIDFor(listingUnits, sellUnitRef),
		unit.ID,
		variant.ID,
		price.QuantityBreakID,
	)
	if err != nil {
		if errors.Is(err, cart.ErrInvalidCatalogueItem) {
			return section17BDenied(
				section17BReasonPriceUnresolved,
				"This product has no current price record.",
			), nil
		}

		return checkout.LineDecision{}, err
	}

	decision := checkout.LineDecision{
		Sellable:   true,
		ReasonCode: checkout.ReasonSellable,

		BusinessID: businessID,
		BranchID:   listing.BranchID,

		DisplayName: product.Name,

		UnitPriceMinor:    *price.UnitPriceMinor,
		Currency:          price.Currency,
		PriceReference:    priceReference,
		CatalogueRevision: sellability.CatalogueRevision,

		MeasureUnderfillBps: unit.UnderfillBps,
		MeasureOverfillBps:  unit.OverfillBps,
	}

	// The cart's measurement decision must still match the sell unit's own
	// current requirement. A unit that started requiring an actual
	// measurement after the line was added is not the same commercial offer.
	variableLine := request.PricingBasis == cart.PricingBasisVariableMeasure

	if unit.MeasurementRequired != variableLine {
		return section17BDenied(
			checkout.ReasonMeasureBasisMismatch,
			"How this item is measured has changed. Add it to your cart again.",
		), nil
	}

	if variableLine &&
		(strings.TrimSpace(request.RequestedMeasureRef) != unit.MeasureID ||
			request.ExpectedMeasureQuantity <= 0) {
		return section17BDenied(
			checkout.ReasonMeasureBasisMismatch,
			"The measurement for this item is no longer valid.",
		), nil
	}

	item, err := a.stock.ResolveShopVariantItem(
		ctx,
		businessID,
		listing.BranchID,
		variant.ID,
	)
	if err != nil {
		if errors.Is(err, inventory.ErrItemNotFound) {
			// Sellable, but with no generic Inventory identity to reserve
			// against. Checkout fails closed on this; it does not silently
			// treat "no stock record" as "unlimited stock".
			return decision, nil
		}

		return checkout.LineDecision{}, err
	}

	basis, err := section17BReservationBasis(item, unit, variableLine)

	if err != nil {
		return section17BDenied(
			checkout.ReasonReservationBasisUnknown,
			"This item's stock basis cannot be resolved for checkout.",
		), nil
	}

	quantity, err := checkout.ReservationQuantity(
		basis,
		request.Quantity,
		request.ExpectedMeasureQuantity,
	)
	if err != nil {
		return section17BDenied(
			checkout.ReasonReservationBasisUnknown,
			"The stock quantity this item requires could not be computed.",
		), nil
	}

	revision, _, err := a.stock.Revision(
		ctx,
		section17BInventoryActor(businessID, request.CartItemID),
		mustExactBranchScope(businessID, listing.BranchID),
		listing.BranchID,
	)
	if err == nil {
		decision.InventoryRevision = &revision
	}

	decision.InventoryItemRef = item.ID
	decision.ReservationBasis = basis
	decision.ReservationQuantity = quantity

	return decision, nil
}

func listingUnitIDFor(
	units []shop.ListingSellUnit,
	sellUnitRef string,
) string {
	for _, unit := range units {
		if unit.SellUnitID == sellUnitRef {
			return unit.ID
		}
	}

	return ""
}

// section17BReservationBasis decides, fail-closed, what a Shop line reserves.
//
// There are exactly two answers Vendor Service can defend today:
//
//   - a VARIABLE_FULFILMENT stock basis is held in its own stock measure, and
//     that is the same basis an actual fulfilment measurement is later
//     compared against;
//   - a SAME_UNIT stock basis whose measure is the sell unit's measure, at a
//     whole-unit fraction, is held one-for-one - which is exactly how the
//     Section 12B sellability engine already compares a requested quantity.
//
// Everything else - EXACT_CONVERSION, a fractional sell unit, a measure that
// does not match - has no conversion authority anywhere in this platform.
// Section 14 explicitly refuses to invent conversion values, so a checkout
// that guessed one would be reserving a number that means something else.
func section17BReservationBasis(
	item inventory.InventoryItem,
	unit section17BShopSellUnit,
	variableLine bool,
) (checkout.ReservationBasis, error) {
	switch item.StockBasisMode {
	case inventory.StockBasisVariableFulfilment:
		if !variableLine ||
			strings.TrimSpace(unit.MeasureID) != strings.TrimSpace(item.StockMeasureRef) {
			return checkout.BasisNone, cart.ErrInvalidCatalogueItem
		}

		return checkout.BasisStockMeasure, nil

	case inventory.StockBasisSameUnit:
		if variableLine ||
			unit.FractionNumerator != 1 ||
			unit.FractionDenominator != 1 ||
			strings.TrimSpace(unit.MeasureID) != strings.TrimSpace(item.StockMeasureRef) {
			return checkout.BasisNone, cart.ErrInvalidCatalogueItem
		}

		return checkout.BasisSellUnit, nil

	default:
		return checkout.BasisNone, cart.ErrInvalidCatalogueItem
	}
}

// -----------------------------------------------------------------------------
// Stock
// -----------------------------------------------------------------------------

// section17BInventoryActor is the checkout subsystem acting on a vendor's own
// Inventory.
//
// It deliberately carries no customer identity. A vendor's stock evidence
// records that a checkout held stock and which session did it; it is not a
// place to copy Customer Service identity into.
func section17BInventoryActor(
	businessID string,
	correlationID string,
) inventory.Actor {
	return inventory.Actor{
		ActorRef:      "commerce-checkout",
		BusinessID:    businessID,
		CorrelationID: correlationID,
	}
}

func mustExactBranchScope(
	businessID string,
	branchID string,
) inventory.BranchScope {
	scope, err := inventory.ExactBranchScope(businessID, branchID)

	if err != nil {
		// An empty business or branch never reaches here: both are
		// server-resolved catalogue evidence. Returning the zero scope keeps
		// the failure at the Inventory boundary, which refuses it.
		return inventory.BranchScope{}
	}

	return scope
}

// section17BStockAuthority is Section 17B's only stock dependency.
//
// Every call is a Section 14 call. Nothing here writes an inventory table.
type section17BStockAuthority struct {
	stock *inventory.StockService
}

func newSection17BStockAuthority(
	stock *inventory.StockService,
) *section17BStockAuthority {
	return &section17BStockAuthority{stock: stock}
}

func (a *section17BStockAuthority) Reserve(
	ctx context.Context,
	request checkout.ReserveRequest,
) (checkout.ReserveResult, error) {
	if a == nil || a.stock == nil {
		return checkout.ReserveResult{}, checkout.ErrDependencyMissing
	}

	lines := make([]inventory.ReservationLineInput, 0, len(request.Lines))

	for _, line := range request.Lines {
		lines = append(
			lines,
			inventory.ReservationLineInput{
				InventoryItemID: line.InventoryItemID,
				Quantity:        line.Quantity,
			},
		)
	}

	expiresAt := request.ExpiresAt

	reservation, err := a.stock.CreateReservation(
		ctx,
		section17BInventoryActor(
			request.BusinessID,
			request.CorrelationID,
		),
		mustExactBranchScope(
			request.BusinessID,
			request.BranchID,
		),
		inventory.CreateReservationInput{
			BranchID:   request.BranchID,
			SourceType: inventory.ReservationSourceCheckout,
			SourceRef:  request.SourceRef,
			ExpiresAt:  &expiresAt,
			Lines:      lines,
		},
	)
	if err != nil {
		if section17BStockRefusal(err) {
			return checkout.ReserveResult{}, checkout.ErrStockUnavailable
		}

		return checkout.ReserveResult{}, err
	}

	return checkout.ReserveResult{
		ReservationRef: reservation.ID,
		ExpiresAt:      reservation.ExpiresAt,
	}, nil
}

// section17BStockRefusal separates "there is not enough stock" from "the
// Inventory authority failed".
//
// The first is a normal checkout outcome the customer can act on. The second
// must surface as an error, because retrying it may succeed and quietly
// reporting it as out of stock would hide an outage behind a shopping message.
func section17BStockRefusal(err error) bool {
	return errors.Is(err, inventory.ErrInsufficientStock) ||
		errors.Is(err, inventory.ErrStockNotFound) ||
		errors.Is(err, inventory.ErrItemNotFound) ||
		errors.Is(err, inventory.ErrReservationStockExpired)
}

func (a *section17BStockAuthority) Release(
	ctx context.Context,
	businessID string,
	branchID string,
	reservationRef string,
) error {
	if a == nil || a.stock == nil {
		return checkout.ErrDependencyMissing
	}

	reservationRef = strings.TrimSpace(reservationRef)

	if reservationRef == "" {
		return nil
	}

	_, err := a.stock.ReleaseReservation(
		ctx,
		section17BInventoryActor(businessID, reservationRef),
		mustExactBranchScope(businessID, branchID),
		reservationRef,
	)

	if err == nil {
		return nil
	}

	// Release is reached by compensation, cancellation, the TTL sweep and
	// crash recovery, and all four may legally race. A reservation that is
	// already gone, or already terminal, is the outcome all of them wanted.
	if errors.Is(err, inventory.ErrReservationNotFound) ||
		errors.Is(err, inventory.ErrReservationStateConflict) ||
		errors.Is(err, inventory.ErrReservationExpired) {
		return nil
	}

	return err
}

func (a *section17BStockAuthority) FindBySource(
	ctx context.Context,
	businessID string,
	sourceRef string,
) (string, error) {
	if a == nil || a.stock == nil {
		return "", checkout.ErrDependencyMissing
	}

	reservationID, status, err := a.stock.FindReservationBySource(
		ctx,
		businessID,
		inventory.ReservationSourceCheckout,
		sourceRef,
	)
	if err != nil {
		return "", err
	}

	if reservationID == "" || inventory.ReservationIsTerminal(status) {
		// Nothing to release. Reporting an already-settled reservation as
		// releasable would only produce a guaranteed conflict downstream.
		return "", nil
	}

	return reservationID, nil
}
