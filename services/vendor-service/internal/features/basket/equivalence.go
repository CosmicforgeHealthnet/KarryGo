package basket

// When two products may be treated as interchangeable.
//
// This is the single most dangerous question in the whole feature, and the
// danger is not that it will be answered too narrowly. A false equivalence
// puts the wrong item in someone's shopping basket and does it under the
// heading of saving them money. So the rule here is deliberately strict, and
// strictness is expressed as a positive requirement rather than a filter:
// nothing is equivalent unless an AUTHORITY says so.
//
// The three authoritative bases, strongest first:
//
//  1. THE SAME TRADE IDENTIFIER. A GTIN, EAN or UPC is assigned by the
//     manufacturer to one article. Two vendors listing the same one are
//     selling the same tin, and that is a fact rather than an inference.
//
//  2. THE SAME CATALOGUE PRODUCT. Same product and variant reference is the
//     same product by construction.
//
//  3. THE SAME CANONICAL TAXONOMY NODE. The taxonomy authority has mapped
//     both products to one node. This is weaker than the first two - two
//     brands of rice are the same NEED and not the same ARTICLE - so it only
//     ever satisfies a requirement whose customer explicitly allowed an
//     equivalent product.
//
// And the rule that is not here: names. Two products with similar names are
// not evidence of anything. "Milk" matches full cream, skimmed, evaporated,
// condensed, powdered and soy, and a customer who wanted one of them did not
// want a discount on another. No function in this file reads a display name.

import "strings"

// EquivalenceBasis records WHY a candidate was considered to satisfy a
// requirement.
//
// It is stored on every proposed substitution. A customer asking "why did you
// put this in my basket" gets an answer naming the authority, and an operator
// auditing the optimizer can tell a same-article swap from a same-category
// one without re-deriving either.
type EquivalenceBasis string

const (
	// BasisExactListing is the listing the customer already named. No
	// substitution occurred.
	BasisExactListing EquivalenceBasis = "EXACT_LISTING"

	// BasisSameCatalogueProduct is the same product and variant from another
	// vendor.
	BasisSameCatalogueProduct EquivalenceBasis = "SAME_CATALOGUE_PRODUCT"

	// BasisSameTradeIdentifier is a manufacturer-assigned global identifier
	// match: the same manufactured article.
	BasisSameTradeIdentifier EquivalenceBasis = "SAME_TRADE_IDENTIFIER"

	// BasisSameCanonicalCategory is the taxonomy authority's own mapping.
	BasisSameCanonicalCategory EquivalenceBasis = "SAME_CANONICAL_CATEGORY"
)

// ConstraintFailure names a hard constraint a candidate violated.
//
// These are the reasons a candidate is EXCLUDED. Nothing below returns them
// as a score or a penalty: a hard constraint is not a cost the optimizer may
// pay, and a shortfall against a budget never widens one.
const (
	FailureBrandMismatch      = "REQUIRED_BRAND_MISMATCH"
	FailureQualityBelowFloor  = "QUALITY_BELOW_REQUIRED_GRADE"
	FailureUnitIncompatible   = "SELL_UNIT_INCOMPATIBLE"
	FailureNoEquivalence      = "NO_AUTHORITATIVE_EQUIVALENCE"
	FailureSubstitutionDenied = "SUBSTITUTION_NOT_PERMITTED"
	FailureInsufficientStock  = "INSUFFICIENT_SELLABLE_STOCK"
	FailureCurrencyMismatch   = "CURRENCY_MISMATCH"
)

// equivalenceOf decides on what basis, if any, a listing satisfies a
// requirement.
//
// The second return value is false when no authority supports the match. That
// is the default answer, and every path to true has to earn it.
//
// original is the listing the requirement came FROM, when there is one. Cart
// optimization has it - the customer already put something in their basket -
// and budget optimization does not, because there is no basket yet. It is the
// only path on which a manufacturer trade identifier can be compared at all,
// since a requirement carries no identifiers of its own.
func equivalenceOf(
	requirement Requirement,
	candidate Listing,
	original *Listing,
) (EquivalenceBasis, bool) {
	// The customer named this exact offer.
	if requirement.ListingRef != "" &&
		strings.EqualFold(requirement.ListingRef, candidate.ListingRef) {
		return BasisExactListing, true
	}

	// The same catalogue product from another vendor. Variant is compared
	// when the requirement stated one: a requirement for a 1 kg bag is not
	// satisfied by the 5 kg bag of the same product.
	if requirement.ProductRef != "" &&
		strings.EqualFold(requirement.ProductRef, candidate.ProductRef) {
		if requirement.VariantRef == "" ||
			strings.EqualFold(requirement.VariantRef, candidate.VariantRef) {
			return BasisSameCatalogueProduct, true
		}
	}

	// A variant reference IS a catalogue product identity, so matching one
	// is the same-product case rather than a weaker one. It is checked
	// separately because a requirement may name a variant without naming its
	// parent product.
	if requirement.VariantRef != "" && requirement.ProductRef == "" &&
		strings.EqualFold(requirement.VariantRef, candidate.VariantRef) {
		return BasisSameCatalogueProduct, true
	}

	// The same manufactured article, by the manufacturer's own global
	// identifier. This is the strongest evidence available and the only one
	// that can cross a catalogue boundary without going through taxonomy:
	// two vendors listing one GTIN are selling one article.
	if original != nil && tradeIdentifierMatch(*original, candidate) {
		return BasisSameTradeIdentifier, true
	}

	// The taxonomy authority mapped both to one node.
	if requirement.CategoryRef != "" &&
		candidate.CanonicalNodeRef != "" &&
		strings.EqualFold(requirement.CategoryRef, candidate.CanonicalNodeRef) {
		return BasisSameCanonicalCategory, true
	}

	// Two products the platform holds no authoritative link between. They may
	// well be interchangeable in a shopper's mind; nothing here is entitled
	// to that opinion.
	return "", false
}

// tradeIdentifierMatch reports a manufacturer-identifier match between a
// named product's listing and a candidate.
//
// It is separated from equivalenceOf because it needs the ORIGINAL listing,
// which only the cart-optimization path has. Budget optimization starts from
// a requirement rather than from something already in a basket, so there is
// no original article to match against.
func tradeIdentifierMatch(original, candidate Listing) bool {
	if len(original.TradeIdentifiers) == 0 ||
		len(candidate.TradeIdentifiers) == 0 {
		return false
	}

	seen := make(map[string]bool, len(original.TradeIdentifiers))

	for _, identifier := range original.TradeIdentifiers {
		normalized := strings.ToUpper(strings.TrimSpace(identifier))

		if normalized != "" {
			seen[normalized] = true
		}
	}

	for _, identifier := range candidate.TradeIdentifiers {
		if seen[strings.ToUpper(strings.TrimSpace(identifier))] {
			return true
		}
	}

	return false
}

// allowanceAdmits reports whether the customer's stated substitution
// allowance permits a match found on a given basis.
//
// This is where consent is enforced, and it is separate from equivalence on
// purpose. "These two are the same product" and "this customer agreed to be
// given the other one" are different questions, and answering only the first
// is how an optimizer starts making decisions the customer did not authorise.
func allowanceAdmits(
	allowance SubstitutionAllowance,
	basis EquivalenceBasis,
) bool {
	if basis == BasisExactListing {
		return true
	}

	switch allowance {
	case AllowExactOnly:
		return false

	case AllowSameProductAnyVendor:
		// The same article from another shop. Not really a substitution: the
		// customer gets exactly what they asked for, bought elsewhere.
		return basis == BasisSameCatalogueProduct ||
			basis == BasisSameTradeIdentifier

	case AllowEquivalentProduct:
		return true

	default:
		return false
	}
}

// satisfies runs every HARD constraint against a candidate.
//
// It returns the equivalence basis when the candidate is admissible, and the
// specific failure when it is not. Naming the failure rather than returning a
// bare false is what lets a no-solution answer say WHICH constraint is
// binding, which is the difference between "no basket fits your budget" and
// "no basket fits your budget because the brand you require is only stocked
// by one vendor and it is out of stock".
//
// The order is chosen so the most informative failure wins. A candidate that
// is both the wrong brand and out of stock is reported as the wrong brand,
// because restocking would not help.
func satisfies(
	requirement Requirement,
	candidate Listing,
	original *Listing,
	currency string,
) (EquivalenceBasis, string) {
	basis, matched := equivalenceOf(requirement, candidate, original)

	if !matched {
		return "", FailureNoEquivalence
	}

	if !allowanceAdmits(requirement.Substitution, basis) {
		return "", FailureSubstitutionDenied
	}

	// A hard brand constraint. No objective, no budget shortfall and no
	// saving may set this aside.
	if requirement.RequiredBrand != "" &&
		!strings.EqualFold(
			strings.TrimSpace(requirement.RequiredBrand),
			strings.TrimSpace(candidate.Brand),
		) {
		return "", FailureBrandMismatch
	}

	// A hard quality floor, checkable because the catalogue states which
	// grades a listing offers.
	if requirement.MinimumQualityGrade != "" &&
		!offersGrade(candidate, requirement.MinimumQualityGrade) {
		return "", FailureQualityBelowFloor
	}

	// Units must agree when the requirement named one. Comparing a price per
	// kilogram with a price per piece produces a number, and the number is
	// meaningless.
	if requirement.SellUnitRef != "" &&
		!strings.EqualFold(requirement.SellUnitRef, candidate.SellUnitRef) {
		return "", FailureUnitIncompatible
	}

	if currency != "" && candidate.Currency != "" &&
		!strings.EqualFold(currency, candidate.Currency) {
		return "", FailureCurrencyMismatch
	}

	// Quantity is satisfied exactly or not at all. Quietly proposing fewer
	// items to fit a budget is not a cheaper basket, it is a different one.
	if candidate.AvailableQuantity < requirement.Quantity {
		return "", FailureInsufficientStock
	}

	return basis, ""
}

// offersGrade reports whether a listing authoritatively offers a quality
// grade.
//
// Exact membership of the catalogue's own stated options, with no ordering
// implied. The platform holds no ranking of QUALITY_GRADE values - it is a
// selection vocabulary, not a scale - so "at least grade B" cannot be
// evaluated as an inequality and is not pretended to be one. A customer
// requiring a grade gets listings that offer that grade.
func offersGrade(candidate Listing, grade string) bool {
	for _, offered := range candidate.QualityGrades {
		if strings.EqualFold(strings.TrimSpace(offered), grade) {
			return true
		}
	}

	return false
}

// hasQualitySignal reports whether a listing carries any authoritative
// quality metadata at all.
//
// It is what gates the QUALITY_BALANCED variant. Where the catalogue states
// nothing, the variant is not generated and the reason is reported, because
// an invented quality ranking presented to a shopper is advice with nothing
// behind it.
func hasQualitySignal(candidate Listing) bool {
	return len(candidate.QualityGrades) > 0
}

// preferredBrandRank scores a candidate against SOFT brand preferences.
//
// Lower is better, and it is only ever consulted AFTER every hard constraint
// has passed and the objective has been applied. A preference that could
// outrank an objective would be a hard constraint wearing a soft label.
func preferredBrandRank(requirement Requirement, candidate Listing) int {
	if len(requirement.PreferredBrands) == 0 {
		return 0
	}

	for index, brand := range requirement.PreferredBrands {
		if strings.EqualFold(
			strings.TrimSpace(brand), strings.TrimSpace(candidate.Brand),
		) {
			return index
		}
	}

	// Not a preferred brand. Ranked after every preferred one, and still
	// perfectly eligible.
	return len(requirement.PreferredBrands) + 1
}
