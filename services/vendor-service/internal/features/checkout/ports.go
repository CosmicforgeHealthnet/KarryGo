package checkout

import (
	"context"
	"errors"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
)

// Narrow ports. Every adapter lives in the composition root, so this package
// never imports Food, Shop or Inventory, and a checkout decision can be tested
// without any of them.

// LineRequest is one cart line presented for authoritative revalidation.
//
// It carries no price, no availability and no vendor authority: business_id
// and branch_id are the cart's server-resolved evidence and are re-derived by
// the authority, never trusted from here.
type LineRequest struct {
	CartItemID string

	Domain cart.Domain

	// Catalogue coordinates the customer selected.
	ItemRef     string
	VariantRef  string
	MenuRef     string
	ListingRef  string
	SellUnitRef string
	SizeRef     string

	Quantity int64

	PricingBasis            string
	RequestedMeasureRef     string
	ExpectedMeasureQuantity int64

	Options []OptionRequest

	// At is the instant the whole validation is evaluated against, so every
	// line in one session sees the same service window.
	At time.Time
}

// OptionRequest is one Food modifier selection carried forward from the cart.
type OptionRequest struct {
	GroupRef  string
	OptionRef string
	Quantity  int64
}

// LineDecision is the authority's current answer for one line.
//
// Sellable false is a normal, expected answer: it is how an authority refuses
// a line without an error, and it always carries a reason code.
type LineDecision struct {
	Sellable bool

	ReasonCode   string
	ReasonDetail string

	// Re-derived authoritative grouping evidence.
	BusinessID string
	BranchID   string

	DisplayName string

	UnitPriceMinor    int64
	Currency          string
	PriceReference    string
	CatalogueRevision int64
	InventoryRevision *int64

	// Variable-measure tolerance owned by the Shop sell unit. Both are basis
	// points and both are zero for a fixed line.
	MeasureUnderfillBps int
	MeasureOverfillBps  int

	// Section 14 reservation basis. Food always leaves these empty: it has no
	// reservation interface, so Section 17B reserves nothing for it.
	InventoryItemRef    string
	ReservationBasis    ReservationBasis
	ReservationQuantity int64
}

// FoodAuthority revalidates one Food line against Section 9 authority.
//
// The implementation must answer business, branch, capability, menu, service
// window, item, variant, operational and production/inventory availability as
// one decision, plus the current authoritative price.
type FoodAuthority interface {
	RevalidateFoodLine(
		ctx context.Context,
		request LineRequest,
	) (LineDecision, error)
}

// ShopAuthority revalidates one Shop line against Section 12 catalogue
// authority and resolves its Section 14 reservation basis.
//
// Stock truth is deliberately NOT this port's answer. Section 14 is the stock
// authority; this port establishes what to reserve, not whether it is there.
type ShopAuthority interface {
	RevalidateShopLine(
		ctx context.Context,
		request LineRequest,
	) (LineDecision, error)
}

// ReserveLine is one Section 14 reservation line.
type ReserveLine struct {
	InventoryItemID string
	Quantity        int64
}

// ReserveRequest is one Vendor group's all-or-nothing stock hold.
//
// SourceRef is the durable checkout group identity. It is committed before
// this call is made, which is what makes both retry and crash recovery
// idempotent: Section 14 keys reservations on (source_type, source_ref).
type ReserveRequest struct {
	BusinessID string
	BranchID   string

	SourceRef string

	ExpiresAt time.Time

	CorrelationID string

	Lines []ReserveLine
}

// ReserveResult is the Section 14 reservation identity.
type ReserveResult struct {
	ReservationRef string
	ExpiresAt      *time.Time
}

// ErrStockUnavailable is the adapter's classification of a Section 14 refusal
// that is about stock rather than about infrastructure. It is a normal
// checkout outcome, not a fault.
var ErrStockUnavailable = errors.New(
	"commerce checkout stock is unavailable",
)

// StockAuthority is Section 17B's only stock dependency.
//
// Section 17B never writes an inventory table. Every hold, release and lookup
// goes through Section 14.
type StockAuthority interface {
	Reserve(
		ctx context.Context,
		request ReserveRequest,
	) (ReserveResult, error)

	// Release settles one reservation back to available stock. It must be
	// idempotent: compensation, cancellation, TTL sweep and crash recovery
	// can all reach the same reservation, and an already-terminal
	// reservation is a success, not an error.
	Release(
		ctx context.Context,
		businessID string,
		branchID string,
		reservationRef string,
	) error

	// FindBySource resolves a reservation from the durable source reference
	// alone. It is what lets recovery release a hold that no group row ever
	// got to record the identity of. An absent reservation returns an empty
	// reference and no error.
	FindBySource(
		ctx context.Context,
		businessID string,
		sourceRef string,
	) (string, error)
}
