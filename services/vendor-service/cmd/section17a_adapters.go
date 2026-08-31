package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
)

// -----------------------------------------------------------------------------
// Food cart authority adapter
// -----------------------------------------------------------------------------

// section17AFoodAuthority adapts the existing Food catalogue/price/sellability
// authority to the Cart port.
//
// Cart never accepts client-supplied Vendor, branch or money authority.
type section17AFoodAuthority struct {
	db      *pgxpool.Pool
	service *food.Service
}

func newSection17AFoodAuthority(
	db *pgxpool.Pool,
	service *food.Service,
) *section17AFoodAuthority {
	return &section17AFoodAuthority{
		db:      db,
		service: service,
	}
}

func (a *section17AFoodAuthority) businessForItem(
	ctx context.Context,
	itemRef string,
) (string, error) {
	if a == nil || a.db == nil || a.service == nil {
		return "", cart.ErrDependencyMissing
	}

	var businessID string

	err := a.db.QueryRow(
		ctx,
		`
        SELECT business_id
        FROM food_items
        WHERE id=$1
          AND status <> 'ARCHIVED'
        `,
		strings.TrimSpace(itemRef),
	).Scan(&businessID)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", cart.ErrInvalidCatalogueItem
	}

	if err != nil {
		return "", err
	}

	businessID = strings.TrimSpace(businessID)

	if businessID == "" {
		return "", cart.ErrInvalidCatalogueItem
	}

	return businessID, nil
}

func section17AFoodPriceReference(
	price food.ResolvedPrice,
) string {
	refs := []string{
		price.VariantPriceVersionRef,
		price.BranchOverridePriceVersionRef,
		price.ItemPriceVersionRef,
	}

	for _, ref := range refs {
		if ref = strings.TrimSpace(ref); ref != "" {
			return ref
		}
	}

	return ""
}

func (a *section17AFoodAuthority) ValidateFoodCartItem(
	ctx context.Context,
	input cart.AddItemInput,
) (cart.ValidatedItem, error) {
	input.ItemRef = strings.TrimSpace(input.ItemRef)

	if input.ItemRef == "" {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	businessID, err := a.businessForItem(
		ctx,
		input.ItemRef,
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	actor := food.Actor{
		VendorID: businessID,
	}

	scope := food.Scope{
		Global: true,
	}

	item, _, currency, err := a.service.GetItem(
		ctx,
		actor,
		scope,
		input.ItemRef,
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	if item.BusinessID != businessID ||
		item.ID != input.ItemRef ||
		item.BranchID == "" ||
		item.MenuID == "" {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	if requestedMenu := strings.TrimSpace(input.MenuRef); requestedMenu != "" &&
		requestedMenu != item.MenuID {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	if requestedSize := strings.TrimSpace(input.SizeRef); requestedSize != "" {
		requestedVariant := strings.TrimSpace(input.VariantRef)

		if requestedVariant == "" || requestedSize != requestedVariant {
			return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
		}
	}

	modifierRefs := make([]string, 0)
	groupNames := make(map[string]string)

	for _, requested := range input.Options {
		requested.GroupRef = strings.TrimSpace(requested.GroupRef)
		requested.OptionRef = strings.TrimSpace(requested.OptionRef)

		if requested.GroupRef == "" ||
			requested.OptionRef == "" ||
			requested.Quantity < 1 ||
			requested.Quantity > cart.MaxOptionQuantity {
			return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
		}

		group, groupErr := a.service.GetModifierGroup(
			ctx,
			actor,
			requested.GroupRef,
		)
		if groupErr != nil {
			return cart.ValidatedItem{}, groupErr
		}

		if group.ID != requested.GroupRef ||
			group.BusinessID != businessID {
			return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
		}

		groupNames[group.ID] = group.Name

		if int64(len(modifierRefs))+requested.Quantity > 200 {
			return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
		}

		for i := int64(0); i < requested.Quantity; i++ {
			modifierRefs = append(
				modifierRefs,
				requested.OptionRef,
			)
		}
	}

	resolved, err := a.service.ResolvePrice(
		ctx,
		actor,
		scope,
		item.ID,
		food.ResolvePriceInput{
			VariantRef:   strings.TrimSpace(input.VariantRef),
			ModifierRefs: modifierRefs,
		},
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	if resolved.ItemRef != item.ID {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	requestedVariant := strings.TrimSpace(input.VariantRef)

	if requestedVariant != "" &&
		resolved.VariantRef != requestedVariant {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	priceReference := section17AFoodPriceReference(resolved)

	if priceReference == "" {
		// A cart snapshot must point to real authoritative price evidence.
		// Never fabricate a pseudo price-version identifier.
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	sellability, err := a.service.ResolveSellability(
		ctx,
		actor,
		scope,
		item.ID,
		time.Time{},
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	if sellability.ItemRef != item.ID ||
		sellability.BranchID != item.BranchID ||
		sellability.MenuRef != item.MenuID {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	variantName := ""

	if resolved.VariantRef != "" {
		variants, variantErr := a.service.ListVariants(
			ctx,
			actor,
			scope,
			item.ID,
			false,
		)
		if variantErr != nil {
			return cart.ValidatedItem{}, variantErr
		}

		found := false

		for _, variant := range variants {
			if variant.ID == resolved.VariantRef {
				if variant.BusinessID != businessID ||
					variant.ItemID != item.ID {
					return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
				}

				variantName = variant.Name
				found = true
				break
			}
		}

		if !found {
			return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
		}
	}

	componentByRef := make(map[string]food.PriceComponent)

	for _, component := range resolved.Modifiers {
		componentByRef[component.ModifierRef] = component
	}

	validatedOptions := make(
		[]cart.ValidatedOption,
		0,
		len(input.Options),
	)

	for _, requested := range input.Options {
		component, ok := componentByRef[requested.OptionRef]

		if !ok {
			return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
		}

		if component.ModifierGroupRef != requested.GroupRef {
			return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
		}

		optionPriceReference := strings.TrimSpace(
			component.PriceVersionRef,
		)

		if optionPriceReference == "" {
			// The effective price still has immutable evidence. Do not invent a
			// separate modifier price identifier when the modifier inherits the
			// effective catalogue price record.
			optionPriceReference = priceReference
		}

		validatedOptions = append(
			validatedOptions,
			cart.ValidatedOption{
				GroupRef:          requested.GroupRef,
				OptionRef:         requested.OptionRef,
				Quantity:          requested.Quantity,
				GroupName:         groupNames[requested.GroupRef],
				OptionName:        component.Name,
				PriceDeltaMinor:   component.PriceDeltaMinor,
				PriceReference:    optionPriceReference,
				CatalogueRevision: resolved.CatalogueRevision,
			},
		)
	}

	catalogueRevision := resolved.CatalogueRevision

	if sellability.CatalogueRevision > catalogueRevision {
		catalogueRevision = sellability.CatalogueRevision
	}

	validated := cart.ValidatedItem{
		Domain:     cart.DomainFood,
		BusinessID: item.BusinessID,
		BranchID:   item.BranchID,

		ItemRef:    item.ID,
		VariantRef: resolved.VariantRef,
		MenuRef:    item.MenuID,

		Quantity: input.Quantity,

		UnitPriceMinor:    resolved.EffectiveUnitPriceMinor,
		Currency:          currency,
		PriceReference:    priceReference,
		CatalogueRevision: catalogueRevision,

		DisplayName: item.Name,
		VariantName: variantName,

		PricingBasis: cart.PricingBasisFixedUnit,

		Options: validatedOptions,
	}

	return cart.ValidateValidatedItem(
		cart.DomainFood,
		validated,
	)
}

// -----------------------------------------------------------------------------
// Shop cart authority adapter
// -----------------------------------------------------------------------------

type section17AShopAuthority struct {
	db      *pgxpool.Pool
	service *shop.Service
}

func newSection17AShopAuthority(
	db *pgxpool.Pool,
	service *shop.Service,
) *section17AShopAuthority {
	return &section17AShopAuthority{
		db:      db,
		service: service,
	}
}

type section17AShopSellUnit struct {
	ID                  string
	BusinessID          string
	ProductID           string
	VariantID           string
	MeasureID           string
	DisplayName         string
	MeasurementMode     string
	MeasurementRequired bool
	UnderfillBps        int
	OverfillBps         int
	Version             int64
	Status              string
}

func (a *section17AShopAuthority) businessForListing(
	ctx context.Context,
	listingRef string,
) (string, error) {
	if a == nil || a.db == nil || a.service == nil {
		return "", cart.ErrDependencyMissing
	}

	var businessID string

	err := a.db.QueryRow(
		ctx,
		`
        SELECT business_id
        FROM shop_branch_listings
        WHERE id=$1
          AND status <> 'ARCHIVED'
        `,
		strings.TrimSpace(listingRef),
	).Scan(&businessID)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", cart.ErrInvalidCatalogueItem
	}

	if err != nil {
		return "", err
	}

	businessID = strings.TrimSpace(businessID)

	if businessID == "" {
		return "", cart.ErrInvalidCatalogueItem
	}

	return businessID, nil
}

func (a *section17AShopAuthority) loadSellUnit(
	ctx context.Context,
	businessID string,
	sellUnitRef string,
) (section17AShopSellUnit, error) {
	var unit section17AShopSellUnit

	err := a.db.QueryRow(
		ctx,
		`
        SELECT
            id,
            business_id,
            product_id,
            variant_id,
            measure_definition_id,
            display_name,
            measurement_mode,
            actual_measurement_required,
            underfill_tolerance_bps,
            overfill_tolerance_bps,
            version,
            status
        FROM shop_sell_units
        WHERE id=$1
          AND business_id=$2
        `,
		strings.TrimSpace(sellUnitRef),
		businessID,
	).Scan(
		&unit.ID,
		&unit.BusinessID,
		&unit.ProductID,
		&unit.VariantID,
		&unit.MeasureID,
		&unit.DisplayName,
		&unit.MeasurementMode,
		&unit.MeasurementRequired,
		&unit.UnderfillBps,
		&unit.OverfillBps,
		&unit.Version,
		&unit.Status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return section17AShopSellUnit{},
			cart.ErrInvalidCatalogueItem
	}

	if err != nil {
		return section17AShopSellUnit{}, err
	}

	if unit.Status != "ACTIVE" {
		return section17AShopSellUnit{},
			cart.ErrInvalidCatalogueItem
	}

	return unit, nil
}

func (a *section17AShopAuthority) latestPriceReference(
	ctx context.Context,
	businessID string,
	listingID string,
	listingUnitID string,
	sellUnitID string,
	variantID string,
	quantityBreakID string,
) (string, error) {
	var priceVersionID string

	err := a.db.QueryRow(
		ctx,
		`
        SELECT id
        FROM shop_price_versions
        WHERE business_id=$1
          AND subject_ref IN ($2,$3,$4,$5,$6)
        ORDER BY revision DESC
        LIMIT 1
        `,
		businessID,
		listingID,
		listingUnitID,
		sellUnitID,
		variantID,
		quantityBreakID,
	).Scan(&priceVersionID)

	if errors.Is(err, pgx.ErrNoRows) {
		// Section 17A requires authoritative snapshot evidence. Absence of
		// immutable price history is not converted into a fabricated reference.
		return "", cart.ErrInvalidCatalogueItem
	}

	if err != nil {
		return "", err
	}

	priceVersionID = strings.TrimSpace(priceVersionID)

	if priceVersionID == "" {
		return "", cart.ErrInvalidCatalogueItem
	}

	return priceVersionID, nil
}

func (a *section17AShopAuthority) ValidateShopCartItem(
	ctx context.Context,
	input cart.AddItemInput,
) (cart.ValidatedItem, error) {
	if len(input.Options) > 0 {
		// Generic arbitrary options are not accepted for Shop. Fresh-goods
		// attribute selections need their own authoritative typed contract.
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	input.ListingRef = strings.TrimSpace(input.ListingRef)
	input.ItemRef = strings.TrimSpace(input.ItemRef)
	input.VariantRef = strings.TrimSpace(input.VariantRef)
	input.SellUnitRef = strings.TrimSpace(input.SellUnitRef)

	if input.ListingRef == "" ||
		input.ItemRef == "" ||
		input.VariantRef == "" ||
		input.SellUnitRef == "" {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	businessID, err := a.businessForListing(
		ctx,
		input.ListingRef,
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	actor := shop.Actor{
		VendorID: businessID,
	}

	scope := shop.Scope{
		Global: true,
	}

	listing, listingUnits, err := a.service.GetListing(
		ctx,
		actor,
		scope,
		input.ListingRef,
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	if listing.ID != input.ListingRef ||
		listing.BusinessID != businessID ||
		listing.ProductID != input.ItemRef ||
		listing.VariantID != input.VariantRef ||
		listing.BranchID == "" {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	var listingUnit shop.ListingSellUnit
	foundListingUnit := false

	for _, candidate := range listingUnits {
		if candidate.SellUnitID != input.SellUnitRef {
			continue
		}

		if candidate.BusinessID != businessID ||
			candidate.ListingID != listing.ID ||
			candidate.VariantID != listing.VariantID ||
			!candidate.Enabled {
			return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
		}

		listingUnit = candidate
		foundListingUnit = true
		break
	}

	if !foundListingUnit {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	product, err := a.service.GetProduct(
		ctx,
		actor,
		scope,
		listing.ProductID,
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	if product.ID != listing.ProductID ||
		product.BusinessID != businessID {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	variant, err := a.service.GetVariant(
		ctx,
		actor,
		scope,
		listing.VariantID,
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	if variant.ID != listing.VariantID ||
		variant.BusinessID != businessID ||
		variant.ProductID != product.ID {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	unit, err := a.loadSellUnit(
		ctx,
		businessID,
		input.SellUnitRef,
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	if unit.ProductID != product.ID ||
		unit.VariantID != variant.ID {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	price, err := a.service.ResolvePrice(
		ctx,
		actor,
		scope,
		listing.ID,
		unit.ID,
		input.Quantity,
		false,
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	if price.ListingRef != listing.ID ||
		price.BranchRef != listing.BranchID ||
		price.VariantRef != variant.ID ||
		price.SellUnitRef != unit.ID ||
		price.UnitPriceMinor == nil ||
		!price.Satisfied {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	sellability, err := a.service.ResolveSellability(
		ctx,
		actor,
		scope,
		listing.BranchID,
		variant.ID,
		unit.ID,
		input.Quantity,
		false,
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	priceReference, err := a.latestPriceReference(
		ctx,
		businessID,
		listing.ID,
		listingUnit.ID,
		unit.ID,
		variant.ID,
		price.QuantityBreakID,
	)
	if err != nil {
		return cart.ValidatedItem{}, err
	}

	pricingBasis := cart.PricingBasisFixedUnit
	requestedMeasureRef := ""
	expectedMeasureQuantity := int64(0)

	if unit.MeasurementRequired {
		pricingBasis = cart.PricingBasisVariableMeasure
		requestedMeasureRef = strings.TrimSpace(
			input.RequestedMeasureRef,
		)
		expectedMeasureQuantity = input.ExpectedMeasureQuantity

		if requestedMeasureRef == "" ||
			requestedMeasureRef != unit.MeasureID ||
			expectedMeasureQuantity <= 0 {
			return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
		}
	} else if requested := strings.TrimSpace(input.RequestedMeasureRef); requested != "" &&
		requested != unit.MeasureID {
		return cart.ValidatedItem{}, cart.ErrInvalidCatalogueItem
	}

	inventoryRevision := sellability.InventoryRevision

	validated := cart.ValidatedItem{
		Domain:     cart.DomainShop,
		BusinessID: businessID,
		BranchID:   listing.BranchID,

		ItemRef:     product.ID,
		VariantRef:  variant.ID,
		ListingRef:  listing.ID,
		SellUnitRef: unit.ID,

		Quantity: input.Quantity,

		UnitPriceMinor:    *price.UnitPriceMinor,
		Currency:          price.Currency,
		PriceReference:    priceReference,
		CatalogueRevision: sellability.CatalogueRevision,
		InventoryRevision: &inventoryRevision,

		DisplayName:  product.Name,
		VariantName:  variant.Name,
		SellUnitName: unit.DisplayName,

		RequestedMeasureRef:     requestedMeasureRef,
		ExpectedMeasureQuantity: expectedMeasureQuantity,
		PricingBasis:            pricingBasis,

		EstimatedWeightGrams: variant.NetWeightGrams,
		TemperatureZone:      variant.TemperatureZone,
	}

	return cart.ValidateValidatedItem(
		cart.DomainShop,
		validated,
	)
}
