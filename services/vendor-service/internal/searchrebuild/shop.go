package searchrebuild

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
)

// shopRow is one authoritative branch listing joined to the product and variant
// it publishes.
//
// The LISTING is the unit of Shop discovery, not the product: the same variant
// can be listed at several branches with different prices, eligibility and
// operating models, and each of those is a separately discoverable thing.
type shopRow struct {
	listingID  string
	businessID string
	branchID   string
	productID  string
	variantID  string

	operatingModel    string
	retailEnabled     bool
	wholesaleEnabled  bool
	retailOverride    *int64
	wholesaleOverride *int64
	preorderPolicy    string
	backorderPolicy   string
	logisticsAvail    bool
	listingStatus     string

	productName   string
	brand         *string
	productClass  string
	productStatus string
	categoryID    string
	categoryName  string

	variantName     string
	sku             *string
	packageLevel    string
	packQuantity    int
	retailPrice     *int64
	wholesalePrice  *int64
	temperatureZone *string
	variantStatus   string

	catalogueRevision int64
	inventoryRevision int64
}

// loadShopBatch reads one bounded, keyset-paginated batch of Shop listings and
// enriches it into Shop search documents.
func (s *Source) loadShopBatch(
	ctx context.Context,
	scope searchprojection.SearchRebuildScope,
	scopeRef string,
	checkpoint string,
	limit int,
) (searchprojection.SearchRebuildBatch, error) {
	filter, scopeArg, err := scopeFilter(scope, scopeRef, "listing.business_id", "listing.branch_id")
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}

	args := []any{checkpoint, limit}
	scopeClause := ""
	if filter != "" {
		args = append(args, scopeArg)
		scopeClause = fmt.Sprintf(filter, len(args))
	}

	rows, err := s.pool.Query(
		ctx,
		`
        SELECT
            listing.id,
            listing.business_id,
            listing.branch_id,
            listing.product_id,
            listing.variant_id,
            listing.operating_model,
            listing.retail_enabled,
            listing.wholesale_enabled,
            listing.retail_price_override_minor,
            listing.wholesale_price_override_minor,
            listing.preorder_policy,
            listing.backorder_policy,
            listing.logistics_available,
            listing.status,
            product.name,
            product.brand,
            product.product_class,
            product.status,
            product.category_id,
            COALESCE(category.name, ''),
            variant.name,
            variant.sku,
            variant.package_level,
            variant.pack_quantity,
            variant.retail_price_minor,
            variant.wholesale_price_minor,
            variant.temperature_zone,
            variant.status,
            COALESCE(catalogue.revision, 0),
            COALESCE(inventory.revision, 0)
          FROM shop_branch_listings listing
          JOIN shop_products product
            ON product.id = listing.product_id
          JOIN shop_variants variant
            ON variant.id = listing.variant_id
          LEFT JOIN shop_categories category
            ON category.id = product.category_id
           AND category.business_id = product.business_id
          LEFT JOIN shop_catalogue_revisions catalogue
            ON catalogue.business_id = listing.business_id
           AND catalogue.branch_id = listing.branch_id
          LEFT JOIN shop_inventory_revisions inventory
            ON inventory.business_id = listing.business_id
           AND inventory.branch_id = listing.branch_id
         WHERE listing.id > $1`+scopeClause+`
         ORDER BY listing.id
         LIMIT $2
        `,
		args...,
	)
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}

	var (
		listings   []shopRow
		productIDs []string
		variantIDs []string
		listingIDs []string
		businesses = map[string]struct{}{}
		branches   = map[string]struct{}{}
	)
	for rows.Next() {
		var row shopRow
		if err = rows.Scan(
			&row.listingID, &row.businessID, &row.branchID, &row.productID, &row.variantID,
			&row.operatingModel, &row.retailEnabled, &row.wholesaleEnabled,
			&row.retailOverride, &row.wholesaleOverride,
			&row.preorderPolicy, &row.backorderPolicy, &row.logisticsAvail, &row.listingStatus,
			&row.productName, &row.brand, &row.productClass, &row.productStatus,
			&row.categoryID, &row.categoryName,
			&row.variantName, &row.sku, &row.packageLevel, &row.packQuantity,
			&row.retailPrice, &row.wholesalePrice, &row.temperatureZone, &row.variantStatus,
			&row.catalogueRevision, &row.inventoryRevision,
		); err != nil {
			rows.Close()
			return searchprojection.SearchRebuildBatch{}, err
		}
		listings = append(listings, row)
		productIDs = append(productIDs, row.productID)
		variantIDs = append(variantIDs, row.variantID)
		listingIDs = append(listingIDs, row.listingID)
		businesses[row.businessID] = struct{}{}
		branches[row.branchID] = struct{}{}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}

	if len(listings) == 0 {
		return searchprojection.SearchRebuildBatch{
			Done:            true,
			NextCheckpoint:  checkpoint,
			SourceRevisions: json.RawMessage(`{}`),
		}, nil
	}

	productIDs = dedupeStrings(productIDs)
	variantIDs = dedupeStrings(variantIDs)

	// Every enrichment below is one bulk query over the batch.
	identifiers, err := s.loadShopIdentifiers(ctx, variantIDs)
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}
	measures, err := s.loadShopMeasures(ctx, listingIDs)
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}
	fresh, err := s.loadShopFreshAttributes(ctx, productIDs)
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}
	handling, err := s.loadShopHandlingFlags(ctx, variantIDs)
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}
	stock, err := s.loadShopInventory(ctx, variantIDs)
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}

	productClassification, err := s.loadClassification(ctx, sourceTypeShopProduct, productIDs, "SHOP")
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}
	variantClassification, err := s.loadClassification(ctx, sourceTypeShopVariant, variantIDs, "SHOP")
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}

	trust, err := s.loadTrust(ctx, keys(businesses))
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}
	serviceability, err := s.loadServiceability(ctx, keys(branches))
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}

	documents := make([]searchprojection.RebuildDocument, 0, len(listings))
	maxCatalogueRevision := int64(0)
	maxInventoryRevision := int64(0)

	for _, row := range listings {
		if row.catalogueRevision > maxCatalogueRevision {
			maxCatalogueRevision = row.catalogueRevision
		}
		if row.inventoryRevision > maxInventoryRevision {
			maxInventoryRevision = row.inventoryRevision
		}

		// A variant classification is more specific than a product one, so both
		// are carried: filtering by the product's category must still find the
		// variant, and a variant-specific mapping must not be lost.
		merged := mergeClassification(
			productClassification[row.productID],
			variantClassification[row.variantID],
		)

		documents = append(documents, s.buildShopDocument(
			row, merged,
			identifiers[row.variantID], measures[row.listingID],
			fresh[row.productID], handling[row.variantID], stock[row.variantID],
			trust[row.businessID], serviceability[row.branchID],
		))
	}

	revisions, err := json.Marshal(map[string]any{
		"source":                 "shop_catalogue",
		"max_catalogue_revision": maxCatalogueRevision,
		"max_inventory_revision": maxInventoryRevision,
		"batch_documents":        len(documents),
		"last_listing_ref":       listings[len(listings)-1].listingID,
		"authoritative_table":    "shop_branch_listings",
	})
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}

	return searchprojection.SearchRebuildBatch{
		Documents:       documents,
		NextCheckpoint:  listings[len(listings)-1].listingID,
		Done:            len(listings) < limit,
		SourceRevisions: revisions,
	}, nil
}

func (s *Source) buildShopDocument(
	row shopRow,
	classified classification,
	identifiers shopIdentifiers,
	measures shopMeasures,
	freshAttributes []string,
	handlingFlags []string,
	stock inventorySignal,
	trust trustSignal,
	serviceable serviceabilitySignal,
) searchprojection.RebuildDocument {
	retailPrice := int64(0)
	priceSource := "unavailable"
	switch {
	case row.retailOverride != nil:
		retailPrice, priceSource = *row.retailOverride, "shop_branch_listing_override"
	case row.retailPrice != nil:
		retailPrice, priceSource = *row.retailPrice, "shop_variant_retail_price"
	}

	wholesalePrice := int64(0)
	switch {
	case row.wholesaleOverride != nil:
		wholesalePrice = *row.wholesaleOverride
	case row.wholesalePrice != nil:
		wholesalePrice = *row.wholesalePrice
	}

	// Discoverable only if the listing, its product and its variant are all
	// active. A listing pointing at an archived product must not remain
	// searchable just because nobody archived the listing too.
	active := strings.EqualFold(row.listingStatus, "ACTIVE") &&
		strings.EqualFold(row.productStatus, "ACTIVE") &&
		strings.EqualFold(row.variantStatus, "ACTIVE")

	brand := ""
	if row.brand != nil {
		brand = *row.brand
	}
	temperatureZone := ""
	if row.temperatureZone != nil {
		temperatureZone = *row.temperatureZone
	}

	// The listing name a customer sees combines the product and the variant,
	// because "Rice" and "50kg Sack" are only useful together.
	name := strings.TrimSpace(row.productName)
	if variant := strings.TrimSpace(row.variantName); variant != "" &&
		!strings.EqualFold(variant, name) {
		name = name + " " + variant
	}

	categories := dedupeStrings(append(
		classified.facetValues("CATEGORY"),
		row.categoryName,
	))

	priceSummary := map[string]any{
		"price_minor": retailPrice,
		"currency":    "NGN",
		"source":      priceSource,
	}
	if wholesalePrice > 0 {
		priceSummary["wholesale_price_minor"] = wholesalePrice
	}

	input := searchprojection.ShopProjectionInput{
		BusinessRef: row.businessID,
		BranchRef:   row.branchID,

		ProductRef: row.productID,
		VariantRef: row.variantID,
		ListingRef: row.listingID,

		Name:  name,
		Brand: brand,

		Aliases:  classified.aliases,
		Synonyms: classified.synonyms,

		CanonicalTaxonomy: classified.taxonomyJSON(),
		TaxonomyTerms:     categories,

		SellUnits:           measures.sellUnits,
		MeasureNames:        measures.measureNames,
		LocalMeasureAliases: measures.localAliases,
		PackagingTerms:      dedupeStrings([]string{row.packageLevel}),

		SKUTerms:     identifiers.skus,
		BarcodeTerms: identifiers.barcodes,

		RetailEligible:    row.retailEnabled,
		WholesaleEligible: row.wholesaleEnabled,
		Active:            active,

		EffectivePriceSummary: jsonObject(priceSummary),
		InventoryAvailabilitySummary: jsonObject(map[string]any{
			"available": stock.status == searchprojection.ShopInventoryInStock ||
				stock.status == searchprojection.ShopInventoryLowStock,
			"status":   string(stock.status),
			"source":   stock.source,
			"revision": stock.revision,
		}),

		PreorderHint:  !strings.EqualFold(row.preorderPolicy, "DISABLED") && row.preorderPolicy != "",
		BackorderHint: !strings.EqualFold(row.backorderPolicy, "DISABLED") && row.backorderPolicy != "",

		FreshAttributes: freshAttributes,
		TemperatureZone: temperatureZone,

		HandlingMetadata: jsonObject(map[string]any{
			"operating_model":     row.operatingModel,
			"product_class":       row.productClass,
			"pack_quantity":       row.packQuantity,
			"handling_flags":      handlingFlags,
			"logistics_available": row.logisticsAvail,
		}),

		AdditionalSearchTerms: dedupeStrings(append(
			append([]string{}, classified.tagCodes...),
			row.productClass, row.operatingModel,
		)),

		CatalogueRevision: maxInt64(row.catalogueRevision, 1),
		InventoryRevision: maxInt64(row.inventoryRevision, 1),
	}

	if sku := identifiers.primarySKU(row.sku); sku != "" {
		input.SKUTerms = dedupeStrings(append(input.SKUTerms, sku))
	}

	signals := searchprojection.ShopOperationalSignals{
		DocumentID: searchprojection.ShopSearchDocumentID(
			row.businessID,
			row.branchID,
			row.productID,
			row.variantID,
			row.listingID,
		),

		InventoryStatus:   stock.status,
		InventorySource:   stock.source,
		InventoryRevision: stock.revision,

		ServiceabilityStatus:   serviceable.status,
		ServiceabilitySource:   serviceable.source,
		ServiceabilityRevision: serviceable.revision,

		TrustStatus:   trust.status,
		TrustSource:   trust.source,
		TrustRevision: trust.revision,

		Reliability: searchprojection.ReliabilitySignal{
			Status: searchprojection.SearchReliabilityUnknown,
			Source: "unavailable",
		},
		Commercial: searchprojection.CommercialSignal{
			Source:             "unavailable",
			PromotionIndicator: false,
		},
	}

	return searchprojection.RebuildDocument{
		Shop:        &input,
		ShopSignals: &signals,
	}
}

// mergeClassification combines the product-level and variant-level
// classification of one listing.
func mergeClassification(product, variant classification) classification {
	merged := newClassification()
	for _, source := range []classification{product, variant} {
		for facet, nodes := range source.nodesByFacet {
			merged.nodesByFacet[facet] = append(merged.nodesByFacet[facet], nodes...)
		}
		for facet, names := range source.displayByFacet {
			merged.displayByFacet[facet] = append(merged.displayByFacet[facet], names...)
		}
		merged.aliases = append(merged.aliases, source.aliases...)
		merged.synonyms = append(merged.synonyms, source.synonyms...)
		merged.tagCodes = append(merged.tagCodes, source.tagCodes...)
		merged.preferenceTags = append(merged.preferenceTags, source.preferenceTags...)
		merged.dietaryClaims = append(merged.dietaryClaims, source.dietaryClaims...)
	}
	merged.aliases = dedupeStrings(merged.aliases)
	merged.synonyms = dedupeStrings(merged.synonyms)
	merged.tagCodes = dedupeStrings(merged.tagCodes)
	merged.preferenceTags = dedupeStrings(merged.preferenceTags)
	merged.dietaryClaims = dedupeStrings(merged.dietaryClaims)
	for facet := range merged.nodesByFacet {
		merged.nodesByFacet[facet] = dedupeStrings(merged.nodesByFacet[facet])
		merged.displayByFacet[facet] = dedupeStrings(merged.displayByFacet[facet])
	}
	return *merged
}

// ---------------------------------------------------------------------------
// Bulk Shop enrichment
// ---------------------------------------------------------------------------

type shopIdentifiers struct {
	skus     []string
	barcodes []string
}

// primarySKU falls back to the variant's own SKU column when no identifier row
// carries one.
func (i shopIdentifiers) primarySKU(variantSKU *string) string {
	if variantSKU == nil {
		return ""
	}
	return strings.TrimSpace(*variantSKU)
}

func (s *Source) loadShopIdentifiers(
	ctx context.Context,
	variantIDs []string,
) (map[string]shopIdentifiers, error) {
	out := map[string]shopIdentifiers{}
	if len(variantIDs) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(
		ctx,
		`
        SELECT variant_id, identifier_type, identifier_value
          FROM shop_product_identifiers
         WHERE variant_id = ANY($1)
           AND status = 'ACTIVE'
        `,
		variantIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var variantID, identifierType, value string
		if err = rows.Scan(&variantID, &identifierType, &value); err != nil {
			return nil, err
		}
		entry := out[variantID]
		// GTIN/EAN/UPC-shaped identifiers are barcodes a customer may scan or
		// paste; everything else behaves as an SKU-style lookup token.
		switch strings.ToUpper(identifierType) {
		case "GTIN", "EAN", "UPC", "BARCODE", "GTIN13", "GTIN14":
			entry.barcodes = append(entry.barcodes, value)
		default:
			entry.skus = append(entry.skus, value)
		}
		out[variantID] = entry
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	for variantID, entry := range out {
		entry.skus = dedupeStrings(entry.skus)
		entry.barcodes = dedupeStrings(entry.barcodes)
		out[variantID] = entry
	}
	return out, nil
}

type shopMeasures struct {
	sellUnits    []string
	measureNames []string
	localAliases []string
}

// loadShopMeasures reads the sell units a listing publishes, together with the
// measure definitions behind them and their local aliases.
//
// The local aliases matter more than they look: "derica", "paint", "mudu" are
// how Nigerian customers actually ask for a quantity, and a rebuild that
// dropped them would make those queries stop working while every structural
// test still passed.
func (s *Source) loadShopMeasures(
	ctx context.Context,
	listingIDs []string,
) (map[string]shopMeasures, error) {
	out := map[string]shopMeasures{}
	if len(listingIDs) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(
		ctx,
		`
        SELECT
            listing_unit.listing_id,
            COALESCE(sell_unit.display_name, ''),
            COALESCE(measure.display_name, ''),
            COALESCE(measure.code, ''),
            COALESCE(alias.alias, '')
          FROM shop_listing_sell_units listing_unit
          JOIN shop_sell_units sell_unit
            ON sell_unit.id = listing_unit.sell_unit_id
          LEFT JOIN shop_measure_definitions measure
            ON measure.id = sell_unit.measure_definition_id
          LEFT JOIN shop_measure_aliases alias
            ON alias.measure_definition_id = measure.id
         WHERE listing_unit.listing_id = ANY($1)
           AND listing_unit.enabled = TRUE
           AND sell_unit.status = 'ACTIVE'
        `,
		listingIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var listingID, sellUnitName, measureName, measureCode, alias string
		if err = rows.Scan(&listingID, &sellUnitName, &measureName, &measureCode, &alias); err != nil {
			return nil, err
		}
		entry := out[listingID]
		entry.sellUnits = append(entry.sellUnits, sellUnitName)
		entry.measureNames = append(entry.measureNames, measureName, measureCode)
		entry.localAliases = append(entry.localAliases, alias)
		out[listingID] = entry
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	for listingID, entry := range out {
		entry.sellUnits = dedupeStrings(entry.sellUnits)
		entry.measureNames = dedupeStrings(entry.measureNames)
		entry.localAliases = dedupeStrings(entry.localAliases)
		out[listingID] = entry
	}
	return out, nil
}

func (s *Source) loadShopFreshAttributes(
	ctx context.Context,
	productIDs []string,
) (map[string][]string, error) {
	out := map[string][]string{}
	if len(productIDs) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(
		ctx,
		`
        SELECT product_id, attribute_type
          FROM shop_fresh_attributes
         WHERE product_id = ANY($1)
           AND status = 'ACTIVE'
        `,
		productIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var productID, attributeType string
		if err = rows.Scan(&productID, &attributeType); err != nil {
			return nil, err
		}
		out[productID] = append(out[productID], attributeType)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for productID, values := range out {
		out[productID] = dedupeStrings(values)
	}
	return out, nil
}

func (s *Source) loadShopHandlingFlags(
	ctx context.Context,
	variantIDs []string,
) (map[string][]string, error) {
	out := map[string][]string{}
	if len(variantIDs) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(
		ctx,
		`
        SELECT variant_id, handling_flag
          FROM shop_variant_handling_flags
         WHERE variant_id = ANY($1)
        `,
		variantIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var variantID, flag string
		if err = rows.Scan(&variantID, &flag); err != nil {
			return nil, err
		}
		out[variantID] = append(out[variantID], flag)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for variantID, values := range out {
		out[variantID] = dedupeStrings(values)
	}
	return out, nil
}

type inventorySignal struct {
	status   searchprojection.ShopInventoryStatus
	source   string
	revision int64
}

// loadShopInventory derives a stock SIGNAL from authoritative balances.
//
// This is a signal, never an authorisation. It says what inventory looked like
// when the rebuild ran; Section 13F's checkout guard revalidates against the
// same tables before anything is reserved, and a variant with no balance row is
// UNKNOWN rather than out of stock - the difference between "we know there is
// none" and "nobody has told us".
func (s *Source) loadShopInventory(
	ctx context.Context,
	variantIDs []string,
) (map[string]inventorySignal, error) {
	out := map[string]inventorySignal{}
	if len(variantIDs) == 0 {
		return out, nil
	}

	rows, err := s.pool.Query(
		ctx,
		`
        SELECT
            variant_id,
            sum(greatest(on_hand_quantity - reserved_quantity - quarantined_quantity, 0)),
            sum(backorder_quantity),
            max(revision)
          FROM shop_inventory_balances
         WHERE variant_id = ANY($1)
         GROUP BY variant_id
        `,
		variantIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var variantID string
		var sellable, backorder, revision *int64
		if err = rows.Scan(&variantID, &sellable, &backorder, &revision); err != nil {
			return nil, err
		}

		signal := inventorySignal{source: "shop_inventory_balances"}
		if revision != nil {
			signal.revision = *revision
		}
		switch {
		case sellable != nil && *sellable > 0:
			signal.status = searchprojection.ShopInventoryInStock
		case backorder != nil && *backorder > 0:
			signal.status = searchprojection.ShopInventoryBackorder
		default:
			signal.status = searchprojection.ShopInventoryOutOfStock
		}
		out[variantID] = signal
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	for _, variantID := range variantIDs {
		if _, ok := out[variantID]; !ok {
			out[variantID] = inventorySignal{
				status: searchprojection.ShopInventoryUnknown,
				source: "unavailable",
			}
		}
	}
	return out, nil
}
