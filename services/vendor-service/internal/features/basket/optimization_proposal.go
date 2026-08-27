package basket

// The answer, and the proposal a customer may accept.
//
// Two things live here and the difference between them is the whole safety
// model of this feature.
//
// A RESULT is information. It says what a basket would cost and why. It is
// produced by a read-only evaluation, it commits nobody to anything, and it
// can be requested a hundred times without consequence.
//
// A PROPOSAL is an offer. It is persisted, owned by one customer, bound to
// one cart version, and it expires. Accepting one is the only act in this
// package that changes anything, and what it changes is the cart - by
// creating a NEW version of it, never by editing the one the proposal was
// built against.
//
// Nothing here charges anybody, reserves anything or creates an order. A
// proposal is a suggestion with an expiry date.

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ProposalState is a proposal's lifecycle.
type ProposalState string

const (
	// ProposalOpen is an offer the customer may still accept.
	ProposalOpen ProposalState = "OPEN"

	// ProposalAccepted means a new cart version was created from it. Terminal:
	// accepting twice must never make two cart versions, and the state is
	// what makes the second attempt a no-op rather than a duplicate.
	ProposalAccepted ProposalState = "ACCEPTED"

	// ProposalExpired means the prices and stock it quoted are no longer
	// evidence.
	ProposalExpired ProposalState = "EXPIRED"

	// ProposalSuperseded means the customer changed their cart, so the
	// proposal describes a starting point that no longer exists.
	ProposalSuperseded ProposalState = "SUPERSEDED"
)

// DefaultProposalTTL is how long an optimization proposal stands.
//
// Short, because it is quoting live prices, live stock and live logistics
// quotes. A proposal that outlived those would be an offer nobody is behind.
const DefaultProposalTTL = 10 * time.Minute

// ProposedLine is one line of a proposed basket.
type ProposedLine struct {
	RequirementRef string `json:"requirement_ref"`

	ListingRef  string `json:"listing_ref"`
	ProductRef  string `json:"product_ref,omitempty"`
	VariantRef  string `json:"variant_ref,omitempty"`
	SellUnitRef string `json:"sell_unit_ref,omitempty"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	DisplayName string `json:"display_name"`
	Brand       string `json:"brand,omitempty"`

	Quantity int64 `json:"quantity"`

	UnitPriceMinor int64  `json:"unit_price_minor"`
	LineTotalMinor int64  `json:"line_total_minor"`
	Currency       string `json:"currency"`

	// PriceReference and CatalogueRevision are the evidence the price was
	// read from. Without them a proposal cannot be revalidated later, and an
	// offer that cannot be revalidated is one that has to be trusted.
	PriceReference    string `json:"price_reference,omitempty"`
	CatalogueRevision int64  `json:"catalogue_revision,omitempty"`

	// QuotedAvailableQuantity is stock AT THE TIME OF QUOTING. It is
	// deliberately named for what it is: nothing here reserved it, and
	// checkout revalidates it. Calling it "available" without the
	// qualification would promise something this package cannot deliver.
	QuotedAvailableQuantity int64 `json:"quoted_available_quantity"`
}

// Substitution records one place the proposal differs from what was asked
// for.
//
// Every field exists so a customer can judge the swap rather than take it on
// trust: what they asked for, what is offered instead, on whose authority the
// two are considered equivalent, and what it does to the price.
type Substitution struct {
	RequirementRef string `json:"requirement_ref"`

	FromListingRef  string `json:"from_listing_ref,omitempty"`
	FromDisplayName string `json:"from_display_name,omitempty"`
	FromBrand       string `json:"from_brand,omitempty"`
	FromUnitPrice   int64  `json:"from_unit_price_minor,omitempty"`

	ToListingRef  string `json:"to_listing_ref"`
	ToDisplayName string `json:"to_display_name"`
	ToBrand       string `json:"to_brand,omitempty"`
	ToUnitPrice   int64  `json:"to_unit_price_minor"`

	// Basis names the authority that says these two are interchangeable.
	Basis EquivalenceBasis `json:"equivalence_basis"`

	// UnitPriceDeltaMinor is negative when the substitute is cheaper.
	UnitPriceDeltaMinor int64 `json:"unit_price_delta_minor"`

	// SellUnitRef on both sides, so a customer can see the swap is
	// like-for-like in unit terms rather than only in name.
	FromSellUnitRef string `json:"from_sell_unit_ref,omitempty"`
	ToSellUnitRef   string `json:"to_sell_unit_ref,omitempty"`

	// VendorChanged is called out separately from the product change. A
	// customer often cares more about which shop it comes from than about
	// which tin, and burying that inside a price delta hides it.
	VendorChanged bool `json:"vendor_changed"`
}

// DeliveryRange is the expected arrival window for a variant.
//
// It is built from the fulfilment planner's own quoted transit, never from an
// average. A range with nothing behind it is a promise the platform has not
// made.
type DeliveryRange struct {
	EarliestMinutes int `json:"earliest_minutes"`
	LatestMinutes   int `json:"latest_minutes"`
}

// Variant is one complete optimized basket.
type Variant struct {
	// ProposalID is set when this variant was persisted as an acceptable
	// offer. A variant that only answers "what would this cost" has none, and
	// there is deliberately nothing to accept.
	ProposalID string `json:"proposal_id,omitempty"`

	Objective Objective `json:"objective"`

	Currency string `json:"currency"`

	MerchandiseTotalMinor int64 `json:"merchandise_total_minor"`
	DeliveryTotalMinor    int64 `json:"delivery_total_minor"`

	// FinalDeliveredTotalMinor is the only number that matters to a customer
	// and the only one the budget is ever evaluated against. Reporting a
	// merchandise total against a budget and adding delivery afterwards is
	// how a basket gets called affordable when it is not.
	FinalDeliveredTotalMinor int64 `json:"final_delivered_total_minor"`

	VendorCount  int `json:"vendor_count"`
	MissionCount int `json:"mission_count"`

	FulfilmentMode string        `json:"fulfilment_mode"`
	DeliveryWindow DeliveryRange `json:"expected_delivery_range"`

	Lines         []ProposedLine `json:"lines"`
	Substitutions []Substitution `json:"substitutions,omitempty"`

	// ConstraintsSatisfied lists the customer's own hard constraints this
	// basket respects. It is stated positively and explicitly because the
	// alternative - silence - reads identically whether the constraints were
	// honoured or never checked.
	ConstraintsSatisfied []string `json:"constraints_satisfied,omitempty"`

	// Reasons explain the shape of this basket in the terms a customer would
	// use: how many shops, what it saves, why it could not be fewer.
	Reasons []string `json:"reasons,omitempty"`

	// PlanSummary carries the fulfilment planner's own candidate evidence, so
	// "why could this not come in one delivery" is answerable from the
	// proposal rather than only from a checkout that has not happened.
	PlanSummary PlanSummary `json:"fulfilment_plan_summary"`

	// FitsBudget is per variant. A request may produce one variant inside the
	// budget and another outside it, and collapsing that to a single flag on
	// the result would hide the affordable one.
	FitsBudget bool `json:"fits_budget"`

	Fingerprint string `json:"fingerprint"`
}

// PlanSummary is the delivery plan behind one variant.
type PlanSummary struct {
	Mode string `json:"fulfilment_mode"`

	MissionCount     int `json:"mission_count"`
	VendorGroupCount int `json:"vendor_group_count"`

	DeliveryGrossMinor    int64 `json:"delivery_gross_minor"`
	DeliveryCustomerMinor int64 `json:"delivery_customer_minor"`

	CompletionMinutes int `json:"completion_minutes"`

	// ConsolidationReasons are the candidate reason codes from the fulfilment
	// planner: why the deliveries were combined, or why they could not be.
	ConsolidationReasons []string `json:"consolidation_reasons,omitempty"`

	PolicyVersion int64 `json:"optimization_policy_version"`
}

// UnmetRequirement is a need nothing could satisfy.
type UnmetRequirement struct {
	RequirementRef string `json:"requirement_ref"`

	// Constraints are the hard constraints that excluded every candidate,
	// most informative first. This is what turns a dead end into an action:
	// "the brand you require is out of stock" tells a customer what to change.
	Constraints []string `json:"binding_constraints"`
}

// Result is the complete answer to one optimization question.
type Result struct {
	Currency string `json:"currency"`

	// Variants are the baskets that could actually be bought and delivered,
	// best first under the requested objectives.
	Variants []Variant `json:"variants"`

	BudgetMinor int64 `json:"budget_minor,omitempty"`

	// FitsBudget is true when at least one variant comes in at or under the
	// budget on its FINAL DELIVERED total.
	FitsBudget bool `json:"fits_budget"`

	// MinimumFeasibleDeliveredTotalMinor is the cheapest complete basket that
	// satisfies every hard constraint, whatever it costs.
	//
	// It is computed from real candidates rather than extrapolated, and it is
	// only meaningful when every requirement is satisfiable - which is why
	// the no-complete-basket case reports itself separately below rather than
	// quoting a minimum for a basket that cannot be assembled.
	MinimumFeasibleDeliveredTotalMinor int64 `json:"minimum_feasible_delivered_total_minor,omitempty"`

	// BudgetShortfallMinor is how far the cheapest feasible basket exceeds
	// the budget. Zero when the budget is met or when no budget was stated.
	BudgetShortfallMinor int64 `json:"budget_shortfall_minor,omitempty"`

	// NoCompleteBasket means at least one requirement cannot be satisfied at
	// ANY price.
	//
	// This is a different answer from "too expensive" and the two are kept
	// apart deliberately. Quoting a minimum feasible total for a basket that
	// is missing an item would be quoting a price for something that cannot
	// be bought.
	NoCompleteBasket bool `json:"no_complete_basket"`

	UnmetRequirements []UnmetRequirement `json:"unmet_requirements,omitempty"`

	// BindingConstraints is the union of the constraints that stopped a
	// cheaper valid basket existing.
	BindingConstraints []string `json:"binding_constraints,omitempty"`

	// SearchTruncated reports that a configured bound stopped the search
	// before every candidate basket was evaluated.
	//
	// It is reported rather than hidden, and the accompanying note says so in
	// words. An optimizer that silently truncates and then presents its
	// answer as the best available is claiming a thoroughness it does not
	// have.
	SearchTruncated bool `json:"search_truncated"`

	// OptimalityNote states, in plain words, exactly what the answer claims.
	OptimalityNote string `json:"optimality_note"`

	// QualityVariantUnavailableReason explains an absent QUALITY_BALANCED
	// variant.
	//
	// The platform holds no ordinal quality signal for most goods, so for
	// most requests this variant genuinely cannot be produced. Saying so is
	// the honest answer; manufacturing a quality score to fill the slot would
	// be presenting an invention as advice.
	QualityVariantUnavailableReason string `json:"quality_variant_unavailable_reason,omitempty"`

	EvaluatedBaskets int `json:"evaluated_baskets"`
}

// OptimalityUnderBoundedSearch is the exact claim this package makes.
//
// Not "the cheapest basket". The search is bounded, so for most inputs some
// baskets were never looked at, and a stronger word would be a stronger claim
// than the implementation supports.
const OptimalityUnderBoundedSearch = "BEST_VALID_CANDIDATE_UNDER_CONFIGURED_SEARCH"

// Proposal is a persisted, acceptable offer.
type Proposal struct {
	ID string `json:"proposal_id"`

	CustomerID string `json:"-"`

	// BaseCartID and BaseCartVersion are the starting point this proposal was
	// computed against.
	//
	// The version is the concurrency guard. A proposal built on cart v5 and
	// accepted after the customer reached v6 must not silently discard what
	// they did in between, and comparing versions is how that is detected
	// rather than hoped for.
	BaseCartID      string `json:"base_cart_ref,omitempty"`
	BaseCartVersion int64  `json:"base_cart_version,omitempty"`

	Objective Objective `json:"objective"`

	State ProposalState `json:"state"`

	Currency string `json:"currency"`

	MerchandiseTotalMinor    int64 `json:"merchandise_total_minor"`
	DeliveryTotalMinor       int64 `json:"delivery_total_minor"`
	FinalDeliveredTotalMinor int64 `json:"final_delivered_total_minor"`

	VendorCount int `json:"vendor_count"`

	Lines         []ProposedLine `json:"lines"`
	Substitutions []Substitution `json:"substitutions,omitempty"`

	// Fingerprint is the proposal's semantic identity: the basket it
	// proposes, independent of when it was made. Two evaluations reaching the
	// same basket produce the same fingerprint, which is what makes a repeat
	// request recognisable as a repeat rather than a new offer.
	Fingerprint string `json:"fingerprint"`

	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`

	// AcceptedCartVersion is the cart version acceptance produced. It is what
	// makes a second acceptance a no-op that returns the same answer rather
	// than a second cart version.
	AcceptedCartVersion int64      `json:"accepted_cart_version,omitempty"`
	AcceptedAt          *time.Time `json:"accepted_at,omitempty"`
}

// Expired reports whether the proposal has lapsed.
func (p Proposal) Expired(now time.Time) bool {
	return !p.ExpiresAt.After(now)
}

// Acceptable reports whether the proposal may still be acted on.
func (p Proposal) Acceptable(now time.Time) bool {
	return p.State == ProposalOpen && !p.Expired(now)
}

// AcceptanceResult is what accepting a proposal produced.
type AcceptanceResult struct {
	ProposalID string `json:"proposal_id"`

	CartID      string `json:"cart_ref"`
	CartVersion int64  `json:"cart_version"`

	// Replayed is true when this acceptance had already happened. The caller
	// gets the same cart version rather than a second one, which is what
	// makes a double-tapped Accept button harmless.
	Replayed bool `json:"replayed"`

	// RequoteRequired is always true.
	//
	// A proposal's totals are an ESTIMATE made against prices and stock at
	// the moment it was built. The authoritative money is the CheckoutQuote,
	// produced after acceptance from the new cart version. Settling against a
	// proposal would be settling against a number that was never anybody's
	// commitment.
	RequoteRequired bool `json:"requote_required"`
}

// fingerprintOf derives a basket's semantic identity.
//
// Volatile values are excluded on purpose: no timestamps, no proposal id, no
// quote reference. Two evaluations of the same catalogue producing the same
// basket must produce the same fingerprint, which is what deduplication and
// stale-detection both rest on.
func fingerprintOf(lines []ProposedLine) string {
	parts := make([]string, 0, len(lines))

	for _, line := range lines {
		parts = append(parts, strings.Join([]string{
			line.RequirementRef,
			line.ListingRef,
			line.SellUnitRef,
			strconv.FormatInt(line.Quantity, 10),
			strconv.FormatInt(line.UnitPriceMinor, 10),
		}, ":"))
	}

	sort.Strings(parts)

	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))

	return hex.EncodeToString(sum[:])
}
