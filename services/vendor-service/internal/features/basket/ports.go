package basket

// Narrow ports.
//
// Every authority this package needs is expressed as the smallest interface
// that answers one question, with the adapter in the composition root. That
// keeps the boundary rule true - basket never imports another feature's HTTP
// or repositories - and, more usefully, it makes the whole optimizer testable
// without a catalogue, an inventory or a logistics service.
//
// One rule runs through all of them: this package RECEIVES authority, it
// never asserts it. It does not decide whether a listing is sellable, what a
// price is, whether stock exists or what a delivery costs. It asks whoever
// owns each answer and refuses to proceed without one.

import (
	"context"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
)

// Listing is one real, currently sellable vendor offer.
//
// Everything here is authoritative catalogue and inventory state. Nothing in
// it is derived, cached beyond the caller's own revision checks, or inferred
// from a name.
type Listing struct {
	ListingRef string
	ProductRef string
	VariantRef string

	BusinessID string
	BranchID   string

	// SellUnitRef and SellUnitName identify the unit the price is per. A
	// price without its unit cannot be compared with another price.
	SellUnitRef  string
	SellUnitName string

	DisplayName string
	Brand       string

	// CanonicalNodeRef is the taxonomy node the catalogue authority maps this
	// product to. It is the ONLY basis on which two different products from
	// two different vendors may be considered interchangeable, and it comes
	// from the taxonomy authority rather than from a similarity guess.
	CanonicalNodeRef string

	// TradeIdentifiers are manufacturer-assigned global identifiers - GTIN,
	// EAN, UPC. They are the strongest equivalence evidence that exists,
	// because two vendors listing the same GTIN are selling the same
	// manufactured article by definition rather than by resemblance.
	//
	// Vendor-private identifiers - internal SKUs, manufacturer codes - are
	// deliberately excluded by the adapter. They are unique only within one
	// tenant, so matching on them across vendors would find collisions and
	// call them equivalences.
	TradeIdentifiers []string

	// UnitPriceMinor is the authoritative current price for one sell unit.
	UnitPriceMinor int64
	Currency       string

	// PriceReference and CatalogueRevision are the evidence behind the price.
	// A proposal that cannot say which price version it quoted cannot be
	// revalidated later.
	PriceReference    string
	CatalogueRevision int64

	// AvailableQuantity is authoritative SELLABLE stock in sell units.
	//
	// It is availability at the moment of the read and it is not a
	// reservation. Nothing here holds it, and checkout revalidates it, which
	// is why a proposal is careful to describe it as quoted availability
	// rather than as guaranteed.
	AvailableQuantity int64
	InventoryRevision int64

	// QualityGrades are the authoritative quality-grade options this listing
	// offers, where the catalogue states any.
	//
	// They support a customer's HARD minimum-grade constraint. They are
	// deliberately not an ordinal score and are never used to rank one
	// listing above another - the catalogue holds no such ranking, and
	// manufacturing one would be advice with nothing behind it.
	QualityGrades []string

	// Logistics is the physical profile the fulfilment planner needs. It is
	// carried through verbatim so a candidate basket is planned against the
	// same physics a real checkout would be.
	Logistics fulfilment.LineProfile
}

// CandidateQuery asks the catalogue for the real listings that could satisfy
// one requirement.
type CandidateQuery struct {
	Requirement Requirement

	Currency string

	// Destination lets the catalogue authority restrict to branches that
	// could plausibly serve this address. It is an efficiency filter, never a
	// serviceability decision: the fulfilment planner and the logistics
	// authorities own that answer.
	Destination Destination

	// Limit bounds how many listings are returned per requirement. It is what
	// stops one request enumerating a catalogue.
	Limit int
}

// CatalogueAuthority resolves requirements into real sellable listings.
//
// The implementation reads Shop catalogue, pricing, taxonomy and Inventory
// through their own owning services. It must return ONLY listings that are
// active, sellable, priced and in stock right now: a candidate this package
// cannot buy is worse than no candidate, because it produces a basket the
// customer is then told does not exist.
type CatalogueAuthority interface {
	// FindCandidates returns listings that could satisfy the requirement,
	// cheapest first, bounded by the query limit.
	//
	// An empty result is a normal answer meaning "nothing available", not an
	// error. The difference matters: an error is a failure to look, and an
	// empty result is a successful look that found nothing, and only the
	// second one may be reported to a customer as an unmet requirement.
	FindCandidates(
		ctx context.Context,
		query CandidateQuery,
	) ([]Listing, error)

	// ResolveListings loads specific listings by reference, for revalidating
	// a proposal against current price and stock before it is accepted.
	//
	// A reference that no longer resolves is omitted rather than errored: a
	// listing being withdrawn between proposal and acceptance is an ordinary
	// event with a correct customer-facing answer.
	ResolveListings(
		ctx context.Context,
		refs []string,
	) (map[string]Listing, error)
}

// PlanQuoter prices the delivery of a candidate basket.
//
// It is the quote-only fulfilment planner and nothing else. Handing this
// package anything that could persist a plan or create a mission would make
// "evaluating a hundred baskets creates zero missions" a matter of discipline
// rather than of construction.
type PlanQuoter interface {
	QuotePlan(
		ctx context.Context,
		request fulfilment.QuoteOnlyRequest,
	) (fulfilment.QuoteOnlyResult, error)
}

// CartSnapshot is the customer's cart as the optimizer needs to see it.
type CartSnapshot struct {
	CartID  string
	Version int64

	Currency string

	Destination Destination

	Lines []CartLine
}

// CartLine is one line of the customer's current cart.
type CartLine struct {
	LineID string

	ListingRef  string
	ProductRef  string
	VariantRef  string
	SellUnitRef string

	BusinessID string
	BranchID   string

	DisplayName string

	Quantity int64

	UnitPriceMinor int64
	Currency       string
}

// CartAuthority reads the customer's cart and applies an accepted proposal.
//
// Applying is deliberately part of this port rather than of this package. The
// cart owns its own versioning, history and expected-version conflict rules,
// and a second place that knows how to mutate a cart is a second place those
// rules can be got wrong.
type CartAuthority interface {
	// LoadForOptimization reads the customer's active cart.
	LoadForOptimization(
		ctx context.Context,
		customerID string,
	) (CartSnapshot, error)

	// ApplyProposal replaces the cart's lines with the proposed ones,
	// creating a NEW cart version.
	//
	// expectedVersion is the version the proposal was built against. The
	// cart authority refuses the write when it no longer matches, which is
	// what stops an accepted proposal silently discarding whatever the
	// customer did in the meantime.
	ApplyProposal(
		ctx context.Context,
		customerID string,
		expectedVersion int64,
		lines []ProposedLine,
	) (CartSnapshot, error)
}

// Clock is the time source, injected so expiry and recurrence are testable
// without waiting.
type Clock func() time.Time
