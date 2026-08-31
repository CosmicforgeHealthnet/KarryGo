package basket

// What the customer actually needs, stated so a machine can shop for it.
//
// The central idea is that a requirement is a NEED, not a SKU. "Two litres of
// permitted milk" survives a vendor going out of stock, changing its price or
// leaving the platform; "listing lst_9f21, quantity 2" does not. Storing the
// second and calling it the first is how a recurring basket quietly stops
// being buyable.
//
// A requirement therefore says WHAT is needed, HOW MUCH, and WHICH
// SUBSTITUTIONS the customer will accept. The last part is what keeps the
// optimizer honest: a constraint marked hard is not a preference the
// optimizer may spend to save money, and there is no threshold anywhere below
// at which a cheaper basket is allowed to violate one.

import (
	"errors"
	"fmt"
	"strings"
)

// Bounds on one optimization request.
//
// They exist because budget optimization over a catalogue is combinatorial.
// Without hard limits a single request can be made to enumerate an arbitrary
// number of baskets, which is a denial-of-service vector with an optimizer's
// name on it. The numbers are deliberately modest: a real grocery list is a
// dozen items, not two hundred.
const (
	MaxRequirements               = 40
	MaxRequirementQuantity        = 1000
	MaxCandidateListingsPerNeed   = 12
	MaxHardBrandConstraintsLength = 120
)

var (
	// ErrNoRequirements means nothing was asked for.
	ErrNoRequirements = errors.New("basket optimization needs at least one requirement")

	// ErrTooManyRequirements means the request exceeded the configured bound.
	ErrTooManyRequirements = errors.New("basket optimization request exceeds the requirement limit")

	// ErrInvalidRequirement means one requirement could not be understood.
	ErrInvalidRequirement = errors.New("basket requirement is not valid")

	// ErrNoDestination means no delivery address was resolved, so no delivery
	// can be planned and no delivered total can be produced.
	ErrNoDestination = errors.New("basket optimization needs a delivery address")

	// ErrCurrencyRequired means no currency was stated. Money without a
	// currency is a number, and comparing two of them is meaningless.
	ErrCurrencyRequired = errors.New("basket optimization needs a currency")

	// ErrProposalNotFound is returned for an unknown or foreign proposal.
	ErrProposalNotFound = errors.New("basket optimization proposal not found")

	// ErrProposalExpired means the proposal is past its expiry. Its prices
	// and stock were true when it was made and are no longer evidence.
	ErrProposalExpired = errors.New("basket optimization proposal has expired")

	// ErrProposalStale means the cart moved underneath the proposal. Applying
	// it would silently discard whatever the customer did in between.
	ErrProposalStale = errors.New("basket optimization proposal is stale")

	// ErrTemplateNotFound is returned for an unknown or foreign template.
	ErrTemplateNotFound = errors.New("recurring basket template not found")

	// ErrTemplateConflict means a concurrent edit won. The caller re-reads
	// rather than overwriting preferences somebody else just set.
	ErrTemplateConflict = errors.New("recurring basket template changed")

	// ErrDependencyMissing means the feature is not fully wired.
	ErrDependencyMissing = errors.New("basket optimization dependency is not connected")
)

// SubstitutionAllowance is how far a requirement may be satisfied by
// something other than exactly what was named.
//
// The vocabulary is closed and mirrors the platform's existing substitution
// policy vocabulary in meaning, deliberately without importing it: that
// policy governs what a PICKER may swap in after an order exists, and this
// one governs what an OPTIMIZER may propose before one does. They answer to
// different people at different moments, and collapsing them would let a
// pre-purchase suggestion inherit a post-purchase authority.
type SubstitutionAllowance string

const (
	// AllowExactOnly permits nothing but the named product. It is the default
	// for a requirement that names a specific listing, because a customer who
	// pointed at one thing did not ask for something like it.
	AllowExactOnly SubstitutionAllowance = "EXACT_ONLY"

	// AllowSameProductAnyVendor permits the SAME manufactured product from a
	// different vendor.
	//
	// This is the safest and most useful allowance, and the one most baskets
	// are optimized under. It is not really a substitution at all: a tin of
	// the same brand with the same trade identifier is the same tin wherever
	// it is bought.
	AllowSameProductAnyVendor SubstitutionAllowance = "SAME_PRODUCT_ANY_VENDOR"

	// AllowEquivalentProduct permits a different product the catalogue
	// authority classifies under the same canonical node, in a compatible
	// unit, subject to any brand and quality constraints the customer set.
	AllowEquivalentProduct SubstitutionAllowance = "EQUIVALENT_PRODUCT"
)

var substitutionAllowances = map[SubstitutionAllowance]struct{}{
	AllowExactOnly:            {},
	AllowSameProductAnyVendor: {},
	AllowEquivalentProduct:    {},
}

// Valid reports membership of the closed vocabulary.
func (a SubstitutionAllowance) Valid() bool {
	_, ok := substitutionAllowances[a]

	return ok
}

// ParseSubstitutionAllowance resolves a customer-supplied allowance.
//
// Empty means EXACT_ONLY. The default is the most restrictive value on
// purpose: a customer who said nothing about substitution has not consented
// to any, and defaulting the other way would treat silence as permission.
func ParseSubstitutionAllowance(value string) (SubstitutionAllowance, error) {
	trimmed := SubstitutionAllowance(
		strings.ToUpper(strings.TrimSpace(value)),
	)

	if trimmed == "" {
		return AllowExactOnly, nil
	}

	if !trimmed.Valid() {
		return "", fmt.Errorf(
			"%w: unknown substitution allowance %q",
			ErrInvalidRequirement, value,
		)
	}

	return trimmed, nil
}

// Objective is what a result variant is optimizing for.
type Objective string

const (
	// ObjectiveValue minimises the final delivered total: merchandise plus
	// delivery, which is the only number the customer actually pays.
	ObjectiveValue Objective = "VALUE"

	// ObjectiveFewestVendors minimises vendor count and delivery cost.
	//
	// Fewer vendors is NOT automatically better and this objective does not
	// claim it is. It exists because fewer deliveries genuinely means fewer
	// arrivals to wait in for, and some customers want that even at a
	// slightly higher merchandise price. It never buys that with a violated
	// constraint.
	ObjectiveFewestVendors Objective = "FEWEST_VENDORS"

	// ObjectiveQualityBalanced ranks by an authoritative quality signal
	// before price.
	//
	// It is generated ONLY where the catalogue actually carries such a
	// signal. Where it does not, the variant is reported as unavailable with
	// the reason, and no quality score is invented to fill the gap. An
	// invented quality ranking is worse than an absent one: it looks like
	// advice.
	ObjectiveQualityBalanced Objective = "QUALITY_BALANCED"
)

var objectives = map[Objective]struct{}{
	ObjectiveValue:           {},
	ObjectiveFewestVendors:   {},
	ObjectiveQualityBalanced: {},
}

// Valid reports membership of the closed vocabulary.
func (o Objective) Valid() bool {
	_, ok := objectives[o]

	return ok
}

// DefaultObjectives is what a request that names none is evaluated for.
func DefaultObjectives() []Objective {
	return []Objective{ObjectiveValue, ObjectiveFewestVendors}
}

// Requirement is one thing the customer needs.
type Requirement struct {
	// Ref names this requirement in the request and in the answer. It is
	// caller-chosen and carries no authority.
	Ref string `json:"ref"`

	// ProductRef and VariantRef name a SPECIFIC catalogue product, when the
	// customer pointed at one. Both are resolved against the authoritative
	// catalogue; neither is trusted as a description of what it refers to.
	ProductRef string `json:"product_ref,omitempty"`
	VariantRef string `json:"variant_ref,omitempty"`

	// ListingRef names a specific vendor's offer. Naming one narrows the
	// requirement to that vendor unless the allowance widens it again.
	ListingRef string `json:"listing_ref,omitempty"`

	// CategoryRef names a canonical taxonomy node, for a requirement stated
	// as a NEED rather than as a product: "rice", not "this bag of rice".
	//
	// This is what makes a recurring basket survive its own history. A
	// template bound to a category re-shops every occurrence; one bound to a
	// listing re-buys a decision made months ago.
	CategoryRef string `json:"category_ref,omitempty"`

	// Quantity is how many sell units are needed. It is satisfied exactly or
	// reported as unmet; a cheaper basket is never produced by quietly
	// delivering less.
	Quantity int64 `json:"quantity"`

	// SellUnitRef is the unit the quantity is expressed in. Without it, "2"
	// could mean two sachets or two cartons, and a comparison between the two
	// is a comparison between different requirements.
	SellUnitRef string `json:"sell_unit_ref,omitempty"`

	// RequiredBrand is a HARD constraint when set. A cheaper substitute of
	// another brand is not offered at any price, in any objective, under any
	// budget shortfall.
	RequiredBrand string `json:"required_brand,omitempty"`

	// MinimumQualityGrade is a HARD constraint when set: the candidate's
	// listing must offer this quality grade as an authoritative catalogue
	// option.
	//
	// Note the asymmetry with ObjectiveQualityBalanced. A customer STATING a
	// required grade is checkable against catalogue data. RANKING two
	// listings by quality is not, because the catalogue holds no ordinal
	// quality signal. The constraint is supported and the ranking is not, and
	// pretending otherwise in either direction would be a lie.
	MinimumQualityGrade string `json:"minimum_quality_grade,omitempty"`

	// Substitution is how far the optimizer may stray from the named product.
	Substitution SubstitutionAllowance `json:"substitution"`

	// PreferredBrands are SOFT. They break ties among candidates that already
	// satisfy every hard constraint, and they never override an objective.
	PreferredBrands []string `json:"preferred_brands,omitempty"`
}

// Named reports whether the requirement points at a specific catalogue item
// rather than at a category.
func (r Requirement) Named() bool {
	return strings.TrimSpace(r.ProductRef) != "" ||
		strings.TrimSpace(r.VariantRef) != "" ||
		strings.TrimSpace(r.ListingRef) != ""
}

// Normalize trims and defaults a requirement, and refuses one that cannot be
// shopped for.
func (r Requirement) Normalize(index int) (Requirement, error) {
	r.Ref = strings.TrimSpace(r.Ref)

	if r.Ref == "" {
		r.Ref = fmt.Sprintf("req_%d", index+1)
	}

	r.ProductRef = strings.TrimSpace(r.ProductRef)
	r.VariantRef = strings.TrimSpace(r.VariantRef)
	r.ListingRef = strings.TrimSpace(r.ListingRef)
	r.CategoryRef = strings.TrimSpace(r.CategoryRef)
	r.SellUnitRef = strings.TrimSpace(r.SellUnitRef)
	r.RequiredBrand = strings.TrimSpace(r.RequiredBrand)
	r.MinimumQualityGrade = strings.ToUpper(
		strings.TrimSpace(r.MinimumQualityGrade),
	)

	if len(r.RequiredBrand) > MaxHardBrandConstraintsLength {
		return Requirement{}, fmt.Errorf(
			"%w: required brand for %q is too long",
			ErrInvalidRequirement, r.Ref,
		)
	}

	if !r.Named() && r.CategoryRef == "" {
		return Requirement{}, fmt.Errorf(
			"%w: %q names neither a product nor a category",
			ErrInvalidRequirement, r.Ref,
		)
	}

	if r.Quantity <= 0 || r.Quantity > MaxRequirementQuantity {
		return Requirement{}, fmt.Errorf(
			"%w: %q has an unusable quantity", ErrInvalidRequirement, r.Ref,
		)
	}

	allowance, err := ParseSubstitutionAllowance(string(r.Substitution))
	if err != nil {
		return Requirement{}, err
	}

	// A requirement stated as a CATEGORY cannot be EXACT_ONLY: there is no
	// exact thing to be only. Widening it to "any product in this category"
	// is the only reading that makes sense, and reading it any other way
	// would make every category requirement unsatisfiable.
	if !r.Named() && allowance == AllowExactOnly {
		allowance = AllowEquivalentProduct
	}

	r.Substitution = allowance

	preferred := make([]string, 0, len(r.PreferredBrands))

	for _, brand := range r.PreferredBrands {
		brand = strings.TrimSpace(brand)

		if brand != "" {
			preferred = append(preferred, brand)
		}
	}

	r.PreferredBrands = preferred

	return r, nil
}

// Destination is where the basket would be delivered.
//
// It is resolved server-side from the customer's own saved address or their
// cart, never accepted as raw coordinates from a request body: a delivery
// quote for coordinates the customer does not own is a quote for somebody
// else's route.
type Destination struct {
	AddressRef string  `json:"address_ref"`
	Address    string  `json:"address"`
	Latitude   float64 `json:"-"`
	Longitude  float64 `json:"-"`
}

// Resolved reports whether the destination can be planned against.
func (d Destination) Resolved() bool {
	return d.Latitude != 0 || d.Longitude != 0
}

// Request is one optimization question.
type Request struct {
	CustomerID string `json:"-"`

	Currency string `json:"currency"`

	// BudgetMinor is the customer's ceiling on the FINAL DELIVERED TOTAL:
	// merchandise plus delivery.
	//
	// Zero means no budget was stated, which is a different question from a
	// budget of zero. Evaluating a merchandise-only budget and then adding
	// delivery on top is how a basket gets reported as fitting a budget it
	// does not fit, so the whole engine works in delivered totals.
	BudgetMinor int64 `json:"budget_minor,omitempty"`

	Destination Destination `json:"destination"`

	// FulfilmentPreference selects the delivery objective every candidate
	// basket is planned under. It is passed straight through to the
	// fulfilment planner and never reinterpreted here.
	FulfilmentPreference string `json:"fulfilment_preference,omitempty"`

	Requirements []Requirement `json:"requirements"`

	Objectives []Objective `json:"objectives,omitempty"`
}

// Normalize validates and defaults a request.
func (r Request) Normalize() (Request, error) {
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))

	if r.Currency == "" {
		return Request{}, ErrCurrencyRequired
	}

	if !r.Destination.Resolved() {
		return Request{}, ErrNoDestination
	}

	if len(r.Requirements) == 0 {
		return Request{}, ErrNoRequirements
	}

	if len(r.Requirements) > MaxRequirements {
		return Request{}, ErrTooManyRequirements
	}

	if r.BudgetMinor < 0 {
		return Request{}, fmt.Errorf(
			"%w: budget cannot be negative", ErrInvalidRequirement,
		)
	}

	normalized := make([]Requirement, 0, len(r.Requirements))

	seen := map[string]bool{}

	for index, requirement := range r.Requirements {
		next, err := requirement.Normalize(index)
		if err != nil {
			return Request{}, err
		}

		// Two requirements sharing a ref would make the answer ambiguous:
		// there would be no way to say which one a substitution belonged to.
		if seen[next.Ref] {
			return Request{}, fmt.Errorf(
				"%w: duplicate requirement ref %q",
				ErrInvalidRequirement, next.Ref,
			)
		}

		seen[next.Ref] = true

		normalized = append(normalized, next)
	}

	r.Requirements = normalized

	objectives := make([]Objective, 0, len(r.Objectives))

	for _, objective := range r.Objectives {
		objective = Objective(strings.ToUpper(strings.TrimSpace(
			string(objective),
		)))

		if !objective.Valid() {
			return Request{}, fmt.Errorf(
				"%w: unknown objective %q", ErrInvalidRequirement, objective,
			)
		}

		objectives = append(objectives, objective)
	}

	if len(objectives) == 0 {
		objectives = DefaultObjectives()
	}

	r.Objectives = objectives

	return r, nil
}

// HasBudget reports whether a budget ceiling was stated.
func (r Request) HasBudget() bool { return r.BudgetMinor > 0 }
