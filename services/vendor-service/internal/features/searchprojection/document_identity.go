package searchprojection

// Exported document identity.
//
// A search document's id is derived deterministically from the refs that
// identify the thing it describes, so the same catalogue resource always
// projects onto the same document across generations and rebuilds.
//
// Operational signals are applied by document id, which means anything that
// produces both a document and its signals in one pass - the Section 13
// rebuild source, most obviously - needs to know the id before the projector
// has run. These wrappers exist so that caller uses THE SAME derivation the
// projector uses rather than reimplementing the hash, because a second
// implementation that drifted by one separator would silently attach every
// signal to a document that does not exist.

// FoodSearchDocumentID is the identity of the Food search document describing
// one food item at one branch.
func FoodSearchDocumentID(
	businessRef string,
	branchRef string,
	foodItemRef string,
) string {
	return deterministicDocumentID(
		"food_",
		businessRef,
		branchRef,
		foodItemRef,
	)
}

// ShopSearchDocumentID is the identity of the Shop search document describing
// one listing of one variant at one branch.
func ShopSearchDocumentID(
	businessRef string,
	branchRef string,
	productRef string,
	variantRef string,
	listingRef string,
) string {
	return deterministicDocumentID(
		"shop_",
		businessRef,
		branchRef,
		productRef,
		variantRef,
		listingRef,
	)
}
