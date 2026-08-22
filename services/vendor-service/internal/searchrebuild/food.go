package searchrebuild

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
)

// foodRow is one authoritative Food item joined to the menu that carries its
// provider capability and the branch override that may replace its price.
type foodRow struct {
	itemID     string
	businessID string
	branchID   string
	menuID     string

	name          string
	description   string
	basePrice     int64
	priceOverride *int64
	dietaryCodes  []byte
	status        string

	menuName       string
	capabilityType string
	menuStatus     string

	availabilityStatus string
	availabilityReason string
	availabilityRev    int64

	catalogueRevision int64
}

// loadFoodBatch reads one bounded, keyset-paginated batch of Food items and
// enriches it with everything a Food search document needs.
//
// Enrichment is bulk: one query per concern over the whole batch, never one per
// item. A rebuild that issued a query per document would still be correct and
// would still pass a functional test, and would take hours on a real catalogue.
func (s *Source) loadFoodBatch(
	ctx context.Context,
	scope searchprojection.SearchRebuildScope,
	scopeRef string,
	checkpoint string,
	limit int,
) (searchprojection.SearchRebuildBatch, error) {
	filter, scopeArg, err := scopeFilter(scope, scopeRef, "item.business_id", "item.branch_id")
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}

	args := []any{checkpoint, limit}
	scopeClause := ""
	if filter != "" {
		args = append(args, scopeArg)
		scopeClause = fmt.Sprintf(filter, len(args))
	}

	// Keyset, not OFFSET: item ids are immutable, so a cursor over them cannot
	// skip or repeat rows while the catalogue changes underneath a long rebuild.
	rows, err := s.pool.Query(
		ctx,
		`
        SELECT
            item.id,
            item.business_id,
            item.branch_id,
            item.menu_id,
            item.name,
            item.description,
            item.base_price_minor,
            override.base_price_override_minor,
            item.dietary_codes,
            item.status,
            COALESCE(menu.name, ''),
            COALESCE(menu.capability_type, ''),
            COALESCE(menu.status, ''),
            COALESCE(availability.status, ''),
            COALESCE(availability.reason, ''),
            COALESCE(availability.version, 0),
            COALESCE(revision.revision, 0)
          FROM food_items item
          LEFT JOIN food_menus menu
            ON menu.id = item.menu_id
          LEFT JOIN food_branch_item_overrides override
            ON override.item_id = item.id
           AND override.branch_id = item.branch_id
          LEFT JOIN food_item_availability availability
            ON availability.item_id = item.id
           AND availability.branch_id = item.branch_id
          LEFT JOIN food_catalogue_revisions revision
            ON revision.business_id = item.business_id
           AND revision.branch_id = item.branch_id
           AND revision.capability_type = menu.capability_type
         WHERE item.id > $1`+scopeClause+`
         ORDER BY item.id
         LIMIT $2
        `,
		args...,
	)
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}

	var (
		items      []foodRow
		itemIDs    []string
		businesses = map[string]struct{}{}
		branches   = map[string]struct{}{}
	)
	for rows.Next() {
		var row foodRow
		if err = rows.Scan(
			&row.itemID, &row.businessID, &row.branchID, &row.menuID,
			&row.name, &row.description, &row.basePrice, &row.priceOverride,
			&row.dietaryCodes, &row.status,
			&row.menuName, &row.capabilityType, &row.menuStatus,
			&row.availabilityStatus, &row.availabilityReason, &row.availabilityRev,
			&row.catalogueRevision,
		); err != nil {
			rows.Close()
			return searchprojection.SearchRebuildBatch{}, err
		}
		items = append(items, row)
		itemIDs = append(itemIDs, row.itemID)
		businesses[row.businessID] = struct{}{}
		branches[row.branchID] = struct{}{}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}

	if len(items) == 0 {
		return searchprojection.SearchRebuildBatch{
			Done:            true,
			NextCheckpoint:  checkpoint,
			SourceRevisions: json.RawMessage(`{}`),
		}, nil
	}

	classification, err := s.loadClassification(
		ctx, sourceTypeFoodItem, itemIDs, "FOOD",
	)
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

	documents := make([]searchprojection.RebuildDocument, 0, len(items))
	maxCatalogueRevision := int64(0)

	for _, row := range items {
		if row.catalogueRevision > maxCatalogueRevision {
			maxCatalogueRevision = row.catalogueRevision
		}
		documents = append(documents, s.buildFoodDocument(
			row, classification[row.itemID], trust[row.businessID], serviceability[row.branchID],
		))
	}

	revisions, err := json.Marshal(map[string]any{
		"source":                 "food_catalogue",
		"max_catalogue_revision": maxCatalogueRevision,
		"batch_documents":        len(documents),
		"last_food_item_ref":     items[len(items)-1].itemID,
		"authoritative_table":    "food_items",
	})
	if err != nil {
		return searchprojection.SearchRebuildBatch{}, err
	}

	return searchprojection.SearchRebuildBatch{
		Documents:       documents,
		NextCheckpoint:  items[len(items)-1].itemID,
		Done:            len(items) < limit,
		SourceRevisions: revisions,
	}, nil
}

// buildFoodDocument turns one authoritative row plus its enrichment into a
// projection input. It invents nothing: a value the domain does not provide is
// left empty or UNKNOWN.
func (s *Source) buildFoodDocument(
	row foodRow,
	classified classification,
	trust trustSignal,
	serviceable serviceabilitySignal,
) searchprojection.RebuildDocument {
	price := row.basePrice
	priceSource := "food_item_base_price"
	if row.priceOverride != nil {
		price = *row.priceOverride
		priceSource = "food_branch_item_override"
	}

	// An item is discoverable only if the item, its menu and its availability
	// all say so. Any one of them saying otherwise wins, because a paused menu
	// hides its items just as effectively as a paused item.
	active := strings.EqualFold(row.status, "ACTIVE") &&
		(row.menuStatus == "" || strings.EqualFold(row.menuStatus, "ACTIVE"))

	availability := foodAvailabilityFromDomain(row.availabilityStatus)
	if availability == searchprojection.FoodAvailabilityUnavailable {
		active = active && false
	}

	providerTypes := []string{}
	if row.capabilityType != "" {
		providerTypes = append(providerTypes, row.capabilityType)
	}

	// Dietary codes declared on the item itself, plus any canonical dietary
	// claim assigned through Section 13C. Both are vendor declarations; neither
	// is upgraded to a verified claim here.
	dietary := dedupeStrings(append(decodeStringArray(row.dietaryCodes), classified.dietaryClaims...))

	menuRefs := []string{}
	if row.menuID != "" {
		menuRefs = append(menuRefs, row.menuID)
	}

	input := searchprojection.FoodProjectionInput{
		BusinessRef: row.businessID,
		BranchRef:   row.branchID,
		FoodItemRef: row.itemID,

		MenuRefs: menuRefs,

		Name: row.name,

		Aliases:  classified.aliases,
		Synonyms: classified.synonyms,

		CanonicalTaxonomy: classified.taxonomyJSON(),

		ProviderTypes: providerTypes,
		CuisineTerms:  classified.facetValues("CUISINE"),
		MealPeriods:   classified.facetValues("MEAL_PERIOD"),
		DishTypes:     classified.facetValues("DISH_TYPE"),
		FoodGroups:    classified.facetValues("FOOD_GROUP"),
		Occasions:     classified.facetValues("OCCASION"),

		DietaryClaims:    dietary,
		PreferenceClaims: classified.preferenceTags,

		AdditionalSearchTerms: classified.tagCodes,

		Active: active,

		AvailabilitySummary: jsonObject(map[string]any{
			"available": availability == searchprojection.FoodAvailabilityAvailable,
			"status":    string(availability),
			"source":    "food_item_availability",
			"revision":  row.availabilityRev,
		}),
		EffectivePriceSummary: jsonObject(map[string]any{
			"price_minor": price,
			"currency":    "NGN",
			"source":      priceSource,
		}),
		ServiceabilityHint: jsonObject(map[string]any{
			"serviceable": serviceable.status == searchprojection.ServiceabilityServiceable,
			"status":      string(serviceable.status),
			"source":      serviceable.source,
		}),
		TrustHint: jsonObject(map[string]any{
			"status": string(trust.status),
			"source": trust.source,
		}),
		// Reliability is UNKNOWN by construction: nothing in Vendor Service
		// records fulfilment completion, cancellation or SLA attainment, and a
		// fabricated default would be indistinguishable from a real perfect
		// score to every consumer downstream.
		ReliabilityHint: jsonObject(map[string]any{
			"status": string(searchprojection.SearchReliabilityUnknown),
			"source": "unavailable",
		}),

		// No authoritative promotion source exists in Vendor Service. A
		// rebuild therefore never promotes anything; promotion arrives, if at
		// all, through an incremental commercial signal from whichever service
		// owns it.
		PromotionIndicator: false,

		CatalogueRevision: maxInt64(row.catalogueRevision, 1),
	}

	signals := searchprojection.FoodOperationalSignals{
		// Derived with the projector's own identity function, so the signals
		// land on the document this same batch is about to write.
		DocumentID: searchprojection.FoodSearchDocumentID(
			row.businessID,
			row.branchID,
			row.itemID,
		),

		AvailabilityStatus:   availability,
		AvailabilitySource:   "food_item_availability",
		AvailabilityRevision: row.availabilityRev,

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
		Food:        &input,
		FoodSignals: &signals,
	}
}

// foodAvailabilityFromDomain maps the Food domain's own availability vocabulary
// onto the search signal vocabulary.
//
// An absent availability row means the domain has never recorded one, which is
// genuinely unknown rather than available.
func foodAvailabilityFromDomain(status string) searchprojection.FoodAvailabilityStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "AVAILABLE":
		return searchprojection.FoodAvailabilityAvailable
	case "SOLD_OUT":
		return searchprojection.FoodAvailabilityUnavailable
	case "PAUSED":
		return searchprojection.FoodAvailabilityUnavailable
	default:
		return searchprojection.FoodAvailabilityUnknown
	}
}

// decodeStringArray reads a jsonb array of strings, tolerating an absent or
// malformed value as "no codes" rather than failing the whole rebuild for one
// item.
func decodeStringArray(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return values
}

func maxInt64(value int64, floor int64) int64 {
	if value < floor {
		return floor
	}
	return value
}

func keys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	return out
}
