package checkout

import (
	"errors"
	"fmt"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
)

// Status is the checkout session lifecycle.
type Status string

const (
	// StatusValidating means the session exists, the cart is frozen, and
	// reservation coordination may still be in flight.
	StatusValidating Status = "VALIDATING"

	// StatusValidated means every line is currently sellable and every
	// required group holds real stock until ExpiresAt.
	StatusValidated Status = "VALIDATED"

	// StatusFailed means validation or reservation coordination did not
	// succeed. No hold remains.
	StatusFailed Status = "FAILED"

	// StatusExpired means the reservation TTL elapsed. No hold remains.
	StatusExpired Status = "EXPIRED"

	// StatusCancelled means the customer abandoned the checkout candidate.
	StatusCancelled Status = "CANCELLED"
)

// Live reports whether the session still owns the cart and may still hold
// stock.
func (s Status) Live() bool {
	return s == StatusValidating || s == StatusValidated
}

// CoordinationState is durable multi-vendor reservation progress.
//
// It exists so that a process that dies between "I am about to reserve" and
// "the reservation succeeded" leaves a record that says so.
type CoordinationState string

const (
	CoordinationNone         CoordinationState = "NONE"
	CoordinationInProgress   CoordinationState = "IN_PROGRESS"
	CoordinationComplete     CoordinationState = "COMPLETE"
	CoordinationCompensating CoordinationState = "COMPENSATING"
	CoordinationCompensated  CoordinationState = "COMPENSATED"
	CoordinationOrphaned     CoordinationState = "ORPHANED"
)

// LineStatus is one revalidated cart line's outcome.
type LineStatus string

const (
	// LineValid means the authoritative answer matches the cart snapshot.
	LineValid LineStatus = "VALID"

	// LineChanged means the line is still sellable but its authoritative
	// price or catalogue revision differs materially from the snapshot.
	// Section 17B reports it; Section 17D decides what to do about it.
	LineChanged LineStatus = "CHANGED"

	// LineUnavailable means the authority currently refuses to sell it.
	LineUnavailable LineStatus = "UNAVAILABLE"

	// LineInvalid means the line no longer resolves to coherent
	// authoritative state at all.
	LineInvalid LineStatus = "INVALID"
)

// Blocking reports whether this outcome prevents a validated checkout.
func (s LineStatus) Blocking() bool {
	return s == LineUnavailable || s == LineInvalid
}

// ReservationStatus is one group's stock-hold state.
type ReservationStatus string

const (
	// ReservationNotRequired is every Food group, and any Shop group whose
	// lines do not consume physical stock.
	ReservationNotRequired ReservationStatus = "NOT_REQUIRED"

	// ReservationPending is committed before the Inventory call. It is the
	// state a crash leaves behind, and the reason recovery is possible.
	ReservationPending ReservationStatus = "PENDING"

	ReservationReserved ReservationStatus = "RESERVED"
	ReservationFailed   ReservationStatus = "FAILED"
	ReservationReleased ReservationStatus = "RELEASED"
)

// ReservationBasis names the unit a reservation quantity is expressed in.
//
// Vendor Service does not invent a conversion between the two. Section 14
// refuses to derive one, and Section 12 keeps measurement authority, so a
// line whose basis cannot be established fails closed instead of reserving a
// number that means something else.
type ReservationBasis string

const (
	// BasisNone is a line that reserves nothing.
	BasisNone ReservationBasis = "NONE"

	// BasisSellUnit reserves in the ordered sell-unit count, which is what
	// the Section 12B sellability engine already compares against for a
	// SAME_UNIT stock basis.
	BasisSellUnit ReservationBasis = "SELL_UNIT"

	// BasisStockMeasure reserves in the stock basis measure. It is the basis
	// a VARIABLE_FULFILMENT stock basis records, and the one an actual
	// fulfilment measurement is later compared against.
	BasisStockMeasure ReservationBasis = "STOCK_MEASURE"
)

// Session-level failure codes.
const (
	FailureLineNotSellable       = "LINE_NOT_SELLABLE"
	FailureStockUnavailable      = "STOCK_UNAVAILABLE"
	FailureReservationFailed     = "RESERVATION_FAILED"
	FailureCurrencyMismatch      = "CURRENCY_MISMATCH"
	FailureCoordinationOrphaned  = "COORDINATION_ORPHANED"
	FailureCartGroupingViolation = "CART_GROUPING_VIOLATION"
)

// Line-level reason codes owned by Section 17B itself. Food and Shop reason
// codes arrive from their own authorities and are passed through unchanged.
const (
	ReasonSellable                 = "SELLABLE"
	ReasonPriceChanged             = "PRICE_CHANGED"
	ReasonCatalogueChanged         = "CATALOGUE_CHANGED"
	ReasonCurrencyMismatch         = "CURRENCY_MISMATCH"
	ReasonReservationBasisUnknown  = "RESERVATION_BASIS_UNRESOLVED"
	ReasonInventoryIdentityMissing = "INVENTORY_IDENTITY_MISSING"
	ReasonStockUnavailable         = "STOCK_UNAVAILABLE"
	ReasonMeasureBasisMismatch     = "MEASURE_BASIS_MISMATCH"
	ReasonGroupingViolation        = "GROUPING_VIOLATION"

	// Tenant standing. Food's own resolver already answers these three as
	// part of one sellability decision and Section 17B passes its codes
	// through; Shop's does not answer them at all, so the Shop adapter
	// asserts them itself and reports the same vocabulary. One reason code
	// per fact, whichever domain the line came from.
	ReasonBusinessInactive     = "BUSINESS_INACTIVE"
	ReasonBranchInactive       = "BRANCH_INACTIVE"
	ReasonCapabilityInactive   = "CAPABILITY_INACTIVE"
	ReasonAuthorityUnavailable = "AUTHORITY_UNAVAILABLE"
)

// Append-only checkout evidence actions.
const (
	EventValidationStarted     = "commerce.checkout.validation_started"
	EventValidationFailed      = "commerce.checkout.validation_failed"
	EventReservationRequested  = "commerce.checkout.reservation_requested"
	EventReservationHeld       = "commerce.checkout.reservation_held"
	EventReservationFailed     = "commerce.checkout.reservation_failed"
	EventReservationReleased   = "commerce.checkout.reservation_released"
	EventCompensationStarted   = "commerce.checkout.compensation_started"
	EventCompensationCompleted = "commerce.checkout.compensation_completed"
	EventValidated             = "commerce.checkout.validated"
	EventCancelled             = "commerce.checkout.cancelled"
	EventExpired               = "commerce.checkout.expired"
	EventOrphaned              = "commerce.checkout.orphaned"
)

// TTL bounds. The section requires a configurable checkout reservation TTL and
// forbids hardcoding one, so these are the outer sanity limits a configured
// value must fall inside, not the value itself.
const (
	MinReservationTTL     = time.Minute
	MaxReservationTTL     = 24 * time.Hour
	DefaultReservationTTL = 10 * time.Minute

	// DefaultStrandedGrace is how long a session may sit mid-coordination
	// before recovery treats it as orphaned by a dead process. It is
	// deliberately shorter than any sane TTL so a crash heals well before the
	// stock would have expired anyway.
	DefaultStrandedGrace = 2 * time.Minute

	// MaxSweepBatch bounds one recovery pass.
	MaxSweepBatch = 500
)

var (
	ErrSessionNotFound = errors.New("commerce checkout session not found")

	ErrCartNotCheckoutable = errors.New(
		"commerce cart cannot start checkout",
	)

	ErrCheckoutConflict = errors.New(
		"commerce checkout session conflicts with the current cart state",
	)

	ErrSessionNotLive = errors.New(
		"commerce checkout session is no longer live",
	)

	ErrDependencyMissing = errors.New(
		"required commerce checkout authority is not connected",
	)

	ErrInvalidTTL = errors.New(
		"commerce checkout reservation TTL is invalid",
	)
)

// Cart precondition codes. These are states the customer must fix before a
// checkout candidate is even meaningful, so they are errors rather than a
// per-line validation outcome.
const (
	PreconditionCartNotActive      = "CART_NOT_ACTIVE"
	PreconditionCartEmpty          = "CART_EMPTY"
	PreconditionDeliveryAddress    = "DELIVERY_ADDRESS_REQUIRED"
	PreconditionCheckoutInProgress = "CHECKOUT_ALREADY_IN_PROGRESS"
)

// CartPreconditionError names which checkout precondition the cart fails.
type CartPreconditionError struct {
	Code string
}

func (e CartPreconditionError) Error() string {
	return fmt.Sprintf(
		"%s: %s",
		ErrCartNotCheckoutable.Error(),
		e.Code,
	)
}

func (e CartPreconditionError) Unwrap() error {
	return ErrCartNotCheckoutable
}

// Line is one revalidated cart line as persisted and returned.
type Line struct {
	ID         string `json:"id"`
	GroupID    string `json:"group_id,omitempty"`
	CartItemID string `json:"cart_item_id"`

	Domain cart.Domain `json:"domain"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	ItemRef     string `json:"item_ref"`
	VariantRef  string `json:"variant_ref,omitempty"`
	MenuRef     string `json:"menu_ref,omitempty"`
	ListingRef  string `json:"listing_ref,omitempty"`
	SellUnitRef string `json:"sell_unit_ref,omitempty"`

	Quantity int64 `json:"quantity"`

	DisplayName string `json:"display_name,omitempty"`

	UnitPriceMinor    int64  `json:"unit_price_minor"`
	Currency          string `json:"currency,omitempty"`
	PriceReference    string `json:"price_reference,omitempty"`
	CatalogueRevision int64  `json:"catalogue_revision"`
	InventoryRevision *int64 `json:"inventory_revision,omitempty"`

	SnapshotUnitPriceMinor    int64  `json:"snapshot_unit_price_minor"`
	SnapshotPriceReference    string `json:"snapshot_price_reference,omitempty"`
	SnapshotCatalogueRevision int64  `json:"snapshot_catalogue_revision"`

	PriceChanged     bool `json:"price_changed"`
	CatalogueChanged bool `json:"catalogue_changed"`

	PricingBasis            string `json:"pricing_basis"`
	RequestedMeasureRef     string `json:"requested_measure_ref,omitempty"`
	ExpectedMeasureQuantity int64  `json:"expected_measure_quantity,omitempty"`
	MeasureUnderfillBps     int    `json:"measure_underfill_bps"`
	MeasureOverfillBps      int    `json:"measure_overfill_bps"`

	QuotedTotalMinor        int64            `json:"quoted_total_minor"`
	MaxAuthorizedTotalMinor int64            `json:"max_authorized_total_minor"`
	InventoryItemRef        string           `json:"inventory_item_ref,omitempty"`
	ReservationQuantity     int64            `json:"reservation_quantity"`
	ReservationBasis        ReservationBasis `json:"reservation_basis"`

	Status       LineStatus `json:"validation_status"`
	ReasonCode   string     `json:"reason_code,omitempty"`
	ReasonDetail string     `json:"reason_detail,omitempty"`
}

// Group is one Vendor business/branch allocation of the checkout candidate.
type Group struct {
	ID string `json:"id"`

	Domain cart.Domain `json:"domain"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	RequiresStockReservation bool `json:"requires_stock_reservation"`

	ReservationStatus ReservationStatus `json:"reservation_status"`

	// The Section 14 idempotency key. Exposed because it is the durable
	// handle recovery uses, and because an operator debugging a stuck hold
	// needs to be able to find it.
	ReservationSourceRef string `json:"reservation_source_ref,omitempty"`

	InventoryReservationRef string     `json:"inventory_reservation_ref,omitempty"`
	ReservationExpiresAt    *time.Time `json:"reservation_expires_at,omitempty"`
	ReservationFailureCode  string     `json:"reservation_failure_code,omitempty"`

	CatalogueRevision int64  `json:"catalogue_revision"`
	InventoryRevision *int64 `json:"inventory_revision,omitempty"`

	Currency string `json:"currency,omitempty"`

	QuotedTotalMinor        int64 `json:"quoted_total_minor"`
	MaxAuthorizedTotalMinor int64 `json:"max_authorized_total_minor"`

	Lines []Line `json:"lines"`
}

// Session is the durable bounded checkout validation record.
//
// It is not a quote. Section 17D builds and locks the quote from this
// evidence; nothing here may be treated as an authorization to charge.
type Session struct {
	ID string `json:"id"`

	CustomerID string `json:"-"`

	CartID      string `json:"cart_id"`
	CartVersion int64  `json:"cart_version"`

	Domain cart.Domain `json:"domain"`

	Status Status `json:"status"`

	ValidationRevision int64 `json:"validation_revision"`

	AddressRef           string     `json:"address_ref,omitempty"`
	AddressSnapshottedAt *time.Time `json:"address_snapshotted_at,omitempty"`

	Currency string `json:"currency,omitempty"`

	QuotedItemsTotalMinor        int64 `json:"quoted_items_total_minor"`
	MaxAuthorizedItemsTotalMinor int64 `json:"max_authorized_items_total_minor"`

	PriceChanged     bool `json:"price_changed"`
	CatalogueChanged bool `json:"catalogue_changed"`

	FailureCode string `json:"failure_code,omitempty"`

	ReservationTTLSeconds int `json:"reservation_ttl_seconds"`

	ExpiresAt time.Time `json:"expires_at"`

	ReservationCoordinationState CoordinationState `json:"reservation_coordination_state"`

	Version int64 `json:"version"`

	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ValidatedAt *time.Time `json:"validated_at,omitempty"`
	SettledAt   *time.Time `json:"settled_at,omitempty"`

	Groups []Group `json:"groups"`
}

// Expired reports whether the reservation TTL has elapsed at the given
// instant. A live session past its TTL is not treated as valid by any read.
func (s Session) Expired(now time.Time) bool {
	return !s.ExpiresAt.After(now)
}
