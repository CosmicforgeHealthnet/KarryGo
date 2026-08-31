package searchprojection

// Text-array normalisation for projected search documents.
//
// Every text[] column on food_search_documents and shop_search_documents is
// NOT NULL DEFAULT '{}'. A Go nil slice binds as SQL NULL, not as an empty
// array, so a document whose optional vocabulary happens to be empty is
// rejected by the database rather than stored as "this document has no menus /
// no aliases / no barcodes".
//
// That is a real state, not an edge case: a Food item that has been created
// but not yet placed on a menu has no menu refs, and a Shop variant without a
// barcode has no barcode terms. Before this, projecting either one failed
// outright and the item simply never became discoverable — which is exactly
// the class of silent discovery gap Section 13D's isolation gate exists to
// catch, and did.
//
// The normalisation is applied once, in the document builders, so it holds for
// every backend rather than only for the PostgreSQL one.

// emptyIfNil returns a non-nil slice, so the value binds as an empty SQL array
// instead of NULL. The original slice is returned untouched when it already
// has elements; only the nil case allocates.
func emptyIfNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// normalizeArrays makes every text[] field of a Food document non-nil.
func (document FoodSearchDocument) normalizeArrays() FoodSearchDocument {
	document.MenuRefs = emptyIfNil(document.MenuRefs)
	document.SearchTokens = emptyIfNil(document.SearchTokens)
	document.Aliases = emptyIfNil(document.Aliases)
	document.Synonyms = emptyIfNil(document.Synonyms)
	document.ProviderTypes = emptyIfNil(document.ProviderTypes)
	document.CuisineTerms = emptyIfNil(document.CuisineTerms)
	document.MealPeriods = emptyIfNil(document.MealPeriods)
	document.DishTypes = emptyIfNil(document.DishTypes)
	document.FoodGroups = emptyIfNil(document.FoodGroups)
	document.Occasions = emptyIfNil(document.Occasions)
	document.DietaryClaims = emptyIfNil(document.DietaryClaims)
	document.PreferenceClaims = emptyIfNil(document.PreferenceClaims)
	return document
}

// normalizeArrays makes every text[] field of a Shop document non-nil.
func (document ShopSearchDocument) normalizeArrays() ShopSearchDocument {
	document.SearchTokens = emptyIfNil(document.SearchTokens)
	document.Aliases = emptyIfNil(document.Aliases)
	document.Synonyms = emptyIfNil(document.Synonyms)
	document.SellUnits = emptyIfNil(document.SellUnits)
	document.MeasureNames = emptyIfNil(document.MeasureNames)
	document.LocalMeasureAliases = emptyIfNil(document.LocalMeasureAliases)
	document.PackagingTerms = emptyIfNil(document.PackagingTerms)
	document.SKUTerms = emptyIfNil(document.SKUTerms)
	document.BarcodeTerms = emptyIfNil(document.BarcodeTerms)
	document.FreshAttributes = emptyIfNil(document.FreshAttributes)
	return document
}
