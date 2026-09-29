// Package orders owns Section 17E's durable order orchestration.
//
// It turns one approved, unexpired Section 17D quote into exactly one
// ParentOrder and one financially and operationally isolated VendorOrder per
// vendor/branch allocation. It creates no payment operation, moves no money,
// and executes no logistics mission: those belong to later sections.
//
// The package deliberately does not import Section 17D's feature package. It
// receives an already-loaded, already-verified QuoteSource through the
// QuoteAuthority port, exactly as Section 17C receives checkout evidence.
package orders

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// -----------------------------------------------------------------------------
// Lifecycle vocabulary
// -----------------------------------------------------------------------------

// ParentState is the ParentOrder's aggregate state. It is always derived from
// the VendorOrders, never set directly, so a mixed outcome cannot be hidden
// behind a state somebody chose to write.
type ParentState string

const (
	ParentStateCreated    ParentState = "CREATED"
	ParentStateInProgress ParentState = "IN_PROGRESS"
	ParentStateCompleted  ParentState = "COMPLETED"

	// ParentStatePartiallyCompleted is the honest answer when every VendorOrder
	// is terminal but they did not all end the same way.
	ParentStatePartiallyCompleted ParentState = "PARTIALLY_COMPLETED"

	ParentStateCancelled         ParentState = "CANCELLED"
	ParentStateRefundPending     ParentState = "REFUND_PENDING"
	ParentStateRefunded          ParentState = "REFUNDED"
	ParentStatePartiallyRefunded ParentState = "PARTIALLY_REFUNDED"
)

// VendorStatus is one VendorOrder's own independent status.
type VendorStatus string

const (
	VendorStatusCreated    VendorStatus = "CREATED"
	VendorStatusPreparing  VendorStatus = "PREPARING"
	VendorStatusReady      VendorStatus = "READY"
	VendorStatusHandedOver VendorStatus = "HANDED_OVER"
	VendorStatusDelivered  VendorStatus = "DELIVERED"
	VendorStatusCompleted  VendorStatus = "COMPLETED"
	VendorStatusCancelled  VendorStatus = "CANCELLED"
	// Section 18 keeps the historical Section 17 vocabulary above for
	// compatibility and adds the explicit operational states used by paid
	// food and shop orders.
	VendorStatusPendingPayment    VendorStatus = "PENDING_PAYMENT"
	VendorStatusPaid              VendorStatus = "PAID"
	VendorStatusSentToVendor      VendorStatus = "SENT_TO_VENDOR"
	VendorStatusAccepted          VendorStatus = "ACCEPTED"
	VendorStatusPicking           VendorStatus = "PICKING"
	VendorStatusSubstitutionReq   VendorStatus = "SUBSTITUTION_REQUIRED"
	VendorStatusAwaitingCustomer  VendorStatus = "AWAITING_CUSTOMER"
	VendorStatusPacking           VendorStatus = "PACKING"
	VendorStatusReadyForPickup    VendorStatus = "READY_FOR_PICKUP"
	VendorStatusPickedUp          VendorStatus = "PICKED_UP"
	VendorStatusDelivering        VendorStatus = "DELIVERING"
	VendorStatusRejected          VendorStatus = "REJECTED"
	VendorStatusRefundPending     VendorStatus = "REFUND_PENDING"
	VendorStatusRefunded          VendorStatus = "REFUNDED"
	VendorStatusPartiallyRefunded VendorStatus = "PARTIALLY_REFUNDED"
)

// Terminal reports whether the status can still change.
func (s VendorStatus) Terminal() bool {
	return s == VendorStatusCompleted || s == VendorStatusCancelled ||
		s == VendorStatusRejected || s == VendorStatusRefunded ||
		s == VendorStatusPartiallyRefunded
}

// DeliveryResponsibility names who executes this VendorOrder's delivery.
type DeliveryResponsibility string

const (
	DeliveryPlatform  DeliveryResponsibility = "PLATFORM_LOGISTICS"
	DeliveryVendorOwn DeliveryResponsibility = "VENDOR_OWN_DELIVERY"
	DeliveryNone      DeliveryResponsibility = "NONE"
)

// FeeBeneficiary names who the delivery fee is owed to. It is derived from the
// responsibility and never configured, so platform delivery money can never be
// pointed at a vendor payable.
type FeeBeneficiary string

const (
	BeneficiaryVendor           FeeBeneficiary = "VENDOR"
	BeneficiaryPlatformProvider FeeBeneficiary = "PLATFORM_PROVIDER"
	BeneficiaryNone             FeeBeneficiary = "NONE"
)

func beneficiaryFor(responsibility DeliveryResponsibility) FeeBeneficiary {
	switch responsibility {
	case DeliveryVendorOwn:
		return BeneficiaryVendor
	case DeliveryPlatform:
		return BeneficiaryPlatformProvider
	default:
		return BeneficiaryNone
	}
}

// VendorOwnDeliveryState is the vendor's own delivery execution state. It is
// only meaningful when the vendor performs the delivery itself.
type VendorOwnDeliveryState string

const (
	VendorOwnNotApplicable VendorOwnDeliveryState = "NOT_APPLICABLE"
	VendorOwnPending       VendorOwnDeliveryState = "PENDING"
	VendorOwnDispatched    VendorOwnDeliveryState = "DISPATCHED"
	VendorOwnDelivered     VendorOwnDeliveryState = "DELIVERED"
	VendorOwnCancelled     VendorOwnDeliveryState = "CANCELLED"
)

// CancelActor names who cancelled a VendorOrder.
type CancelActor string

const (
	CancelByVendor   CancelActor = "VENDOR"
	CancelByCustomer CancelActor = "CUSTOMER"
	CancelByPlatform CancelActor = "PLATFORM"
)

// -----------------------------------------------------------------------------
// Allocation vocabulary
// -----------------------------------------------------------------------------

// AllocationType is what part of the purchase an allocation row accounts for.
type AllocationType string

const (
	AllocationProduct  AllocationType = "PRODUCT"
	AllocationDelivery AllocationType = "DELIVERY"
	AllocationFee      AllocationType = "FEE"
	AllocationTax      AllocationType = "TAX"
	AllocationDiscount AllocationType = "DISCOUNT"
)

// BeneficiaryType is who an allocation is owed to.
type BeneficiaryType string

const (
	BeneficiaryTypeVendor          BeneficiaryType = "VENDOR"
	BeneficiaryTypePlatformProvide BeneficiaryType = "PLATFORM_PROVIDER"
	BeneficiaryTypePlatform        BeneficiaryType = "PLATFORM"
	BeneficiaryTypeCustomer        BeneficiaryType = "CUSTOMER"
)

// FundingSource is who paid for an allocation. It mirrors Section 17D's own
// funding vocabulary so attribution survives the quote/order boundary.
type FundingSource string

const (
	FundingCustomer        FundingSource = "CUSTOMER"
	FundingVendor          FundingSource = "VENDOR"
	FundingPlatform        FundingSource = "PLATFORM"
	FundingDeliverySubsidy FundingSource = "DELIVERY_SUBSIDY"
	FundingOtherApproved   FundingSource = "OTHER_APPROVED"
)

// Adjustment scopes, mirroring Section 17D.
const (
	ScopeProduct  = "PRODUCT"
	ScopeDelivery = "DELIVERY"
	ScopeOrder    = "ORDER"
)

// Adjustment types, mirroring Section 17D.
const (
	AdjustmentDiscount = "DISCOUNT"
	AdjustmentFee      = "FEE"
	AdjustmentTax      = "TAX"
)

// -----------------------------------------------------------------------------
// Errors
// -----------------------------------------------------------------------------

var (
	ErrOrderNotFound = errors.New("commerce order not found")

	// ErrQuoteNotOrderable covers every reason an approved agreement can no
	// longer become an order: not approved, expired, or its checkout,
	// reservation or fulfilment evidence has moved since approval.
	ErrQuoteNotOrderable = errors.New("commerce quote cannot become an order")

	ErrOrderConflict = errors.New("commerce order conflicts with its current state")

	ErrFingerprintMismatch = errors.New("commerce quote fingerprint mismatch")

	ErrVersionConflict = errors.New("commerce order changed since it was read")

	// ErrNotAttributable is returned rather than guessing. A vendor-funded
	// benefit that names no vendor cannot be split across vendors without one
	// vendor paying for another's promotion, which Section 17E forbids.
	ErrNotAttributable = errors.New("commercial allocation cannot be attributed to a vendor")

	ErrAllocationImbalance = errors.New("commercial allocation does not reconcile to the approved total")

	ErrForbidden = errors.New("actor may not act on this commerce order")

	ErrDependencyMissing = errors.New("required order dependency is not connected")

	ErrDisabled = errors.New("commerce order orchestration is not enabled")
)

// -----------------------------------------------------------------------------
// Source evidence
// -----------------------------------------------------------------------------

// AddressSnapshot is the delivery address exactly as Section 17D froze it.
type AddressSnapshot struct {
	SourceAddressRef string    `json:"source_address_ref"`
	Title            string    `json:"title,omitempty"`
	Address          string    `json:"address"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	Description      string    `json:"description,omitempty"`
	SnapshottedAt    time.Time `json:"snapshotted_at"`
}

// ReceiverSnapshot preserves delivery contact evidence. An optional account
// binding is separate from the contact and is used only for secure handover
// reads; a phone number alone never implies an account identity.
//
// Section 17D does not fingerprint the receiver, so the order store reads it
// from the cart under the cart version the quote already binds. A receiver
// change bumps that version, so a changed receiver can never be silently
// snapshotted onto an order the customer approved before the change.
type ReceiverSnapshot struct {
	ReceiverName       string `json:"receiver_name,omitempty"`
	ReceiverPhone      string `json:"receiver_phone,omitempty"`
	ReceiverCustomerID string `json:"receiver_customer_id,omitempty"`
}

// GroupSource is one vendor/branch allocation of the approved quote.
type GroupSource struct {
	QuoteGroupRef string
	BusinessID    string
	BranchID      string
	Domain        string

	ProductSubtotalMinor    int64
	MaxProductSubtotalMinor int64

	ReservationRequired  bool
	ReservationRef       string
	ReservationSourceRef string

	LogisticsPlanGroupRef string
	LogisticsMissionRef   string

	DeliveryResponsibility DeliveryResponsibility

	DeliveryGrossMinor          int64
	DeliveryCustomerMinor       int64
	DeliveryVendorFundedMinor   int64
	DeliveryPlatformFundedMinor int64
}

// LineSource is one approved item snapshot.
type LineSource struct {
	QuoteLineRef  string
	QuoteGroupRef string
	CartItemID    string

	ItemName    string
	ItemRef     string
	VariantRef  string
	ListingRef  string
	SellUnitRef string
	Domain      string

	Quantity                int64
	UnitPriceMinor          int64
	QuotedTotalMinor        int64
	MaxAuthorizedTotalMinor int64

	PricingBasis            string
	RequestedMeasureRef     string
	ExpectedMeasureQuantity int64
	MeasureUnderfillBps     int
	MeasureOverfillBps      int

	PriceReference    string
	CatalogueRevision int64
	InventoryRevision *int64
	InventoryItemRef  string

	OnlinePriceDisclosure string

	// Shop fulfilment terms frozen from the approved quote. These are optional
	// for FOOD and legacy lines, which retain fixed-unit behaviour.
	MeasurementMode              string
	MeasurementBasis             string
	MeasurementMinQuantity       int64
	MeasurementMaxQuantity       int64
	MeasurementToleranceQuantity int64
	VariableCountAllowed         bool
	SubstitutionPolicy           string
	QualityPreferenceSnapshot    json.RawMessage
	TaxonomyVersion              int64
	TaxonomyCategoryCode         string
}

// AdjustmentSource is one approved commercial adjustment.
type AdjustmentSource struct {
	AdjustmentID          string
	Type                  string
	Scope                 string
	FundingSource         FundingSource
	SourceRef             string
	Description           string
	AmountMinor           int64
	BusinessID            string
	BranchID              string
	PromotionType         string
	CampaignVersion       int64
	EligibilityEvidence   json.RawMessage
	CouponDigest          string
	PlatformApprovalRef   string
	CalculationBaseMinor  int64
	DeliveryDiscountMinor int64
	StackPriority         int
	AppliedAt             *time.Time
}

// QuoteSource is the complete, already-verified approved agreement an order is
// cut from. Every commercial figure here comes from the frozen quote; nothing
// in Section 17E recomputes a price from the live catalogue.
type QuoteSource struct {
	QuoteID          string
	QuoteVersion     int64
	QuoteFingerprint string
	ApprovedAt       time.Time
	ExpiresAt        time.Time

	CustomerID                 string
	CheckoutSessionID          string
	CheckoutValidationRevision int64
	CartID                     string
	CartVersion                int64
	FulfilmentPlanID           string
	FulfilmentPlanRevision     int64
	FulfilmentPlanVersion      int64

	Domain                 string
	Currency               string
	SelectedFulfilmentMode string

	Address     AddressSnapshot
	HasReceiver bool
	Receiver    ReceiverSnapshot

	ProductSubtotalMinor    int64
	MaxProductSubtotalMinor int64
	DeliverySubtotalMinor   int64
	FeeSubtotalMinor        int64
	TaxSubtotalMinor        int64
	DiscountTotalMinor      int64
	SubsidyTotalMinor       int64
	FinalPayableMinor       int64
	MaxAuthorizedTotalMinor int64

	FundingBreakdown        json.RawMessage
	PriceDisclosureSnapshot json.RawMessage

	Groups      []GroupSource
	Lines       []LineSource
	Adjustments []AdjustmentSource
}

// -----------------------------------------------------------------------------
// Read models
// -----------------------------------------------------------------------------

// Allocation is one explicitly recorded part of the customer's approved total.
type Allocation struct {
	ID              string          `json:"id"`
	VendorOrderID   string          `json:"vendor_order_id,omitempty"`
	AllocationType  AllocationType  `json:"allocation_type"`
	BeneficiaryType BeneficiaryType `json:"beneficiary_type"`
	BeneficiaryRef  string          `json:"beneficiary_ref,omitempty"`
	FundingSource   FundingSource   `json:"funding_source"`
	SourceRef       string          `json:"source_ref,omitempty"`
	AmountMinor     int64           `json:"amount_minor"`
}

// OrderLine is one immutable item snapshot on a VendorOrder.
type OrderLine struct {
	ID             string `json:"id"`
	QuoteLineRef   string `json:"quote_line_ref"`
	CartItemID     string `json:"cart_item_id"`
	ItemName       string `json:"item_name"`
	ItemRef        string `json:"item_ref"`
	VariantRef     string `json:"variant_ref,omitempty"`
	ListingRef     string `json:"listing_ref,omitempty"`
	SellUnitRef    string `json:"sell_unit_ref,omitempty"`
	Quantity       int64  `json:"quantity"`
	UnitPriceMinor int64  `json:"unit_price_minor"`

	QuotedTotalMinor        int64 `json:"quoted_total_minor"`
	MaxAuthorizedTotalMinor int64 `json:"max_authorized_total_minor"`
	DiscountAllocatedMinor  int64 `json:"discount_allocated_minor"`
	FeeAllocatedMinor       int64 `json:"fee_allocated_minor"`
	TaxAllocatedMinor       int64 `json:"tax_allocated_minor"`

	PricingBasis            string `json:"pricing_basis"`
	RequestedMeasureRef     string `json:"requested_measure_ref,omitempty"`
	ExpectedMeasureQuantity int64  `json:"expected_measure_quantity,omitempty"`
	MeasureUnderfillBps     int    `json:"measure_underfill_bps"`
	MeasureOverfillBps      int    `json:"measure_overfill_bps"`

	PriceReference    string `json:"price_reference"`
	CatalogueRevision int64  `json:"catalogue_revision"`
	InventoryRevision *int64 `json:"inventory_revision,omitempty"`
	InventoryItemRef  string `json:"inventory_item_ref,omitempty"`

	OnlinePriceDisclosure   string          `json:"online_price_disclosure"`
	StorePriceParityClaimed bool            `json:"store_price_parity_claimed"`
	DeliveryCommitment      json.RawMessage `json:"delivery_commitment"`
}

// VendorOrder is one vendor/branch allocation with its own independent status,
// stock relationship, fulfilment state and money.
type VendorOrder struct {
	ID            string `json:"vendor_order_id"`
	ParentOrderID string `json:"parent_order_id"`
	CustomerID    string `json:"-"`

	BusinessID    string `json:"business_id"`
	BranchID      string `json:"branch_id"`
	Domain        string `json:"domain"`
	Currency      string `json:"currency"`
	QuoteGroupRef string `json:"quote_group_ref"`

	ProductSubtotalMinor    int64 `json:"product_subtotal_minor"`
	MaxProductSubtotalMinor int64 `json:"max_product_subtotal_minor"`

	ProductVendorFundedMinor   int64 `json:"product_vendor_funded_minor"`
	ProductPlatformFundedMinor int64 `json:"product_platform_funded_minor"`
	ProductOtherFundedMinor    int64 `json:"product_other_funded_minor"`
	OrderVendorFundedMinor     int64 `json:"order_vendor_funded_minor"`
	OrderPlatformFundedMinor   int64 `json:"order_platform_funded_minor"`
	OrderOtherFundedMinor      int64 `json:"order_other_funded_minor"`

	DeliveryResponsibility DeliveryResponsibility `json:"delivery_responsibility"`
	DeliveryFeeBeneficiary FeeBeneficiary         `json:"delivery_fee_beneficiary"`

	DeliveryGrossMinor          int64 `json:"delivery_gross_minor"`
	DeliveryCustomerMinor       int64 `json:"delivery_customer_minor"`
	DeliveryVendorFundedMinor   int64 `json:"delivery_vendor_funded_minor"`
	DeliveryPlatformFundedMinor int64 `json:"delivery_platform_funded_minor"`
	DeliveryOtherFundedMinor    int64 `json:"delivery_other_funded_minor"`

	CustomerPayableMinor int64 `json:"customer_payable_minor"`
	VendorPayableMinor   int64 `json:"vendor_payable_minor"`

	ReservationRequired bool `json:"reservation_required"`
	// InventoryState is where this allocation's stock stands. Explicit so it
	// is never inferred, which is how stock gets double-committed.
	InventoryState       string `json:"inventory_state"`
	ReservationRef       string `json:"inventory_reservation_ref,omitempty"`
	ReservationSourceRef string `json:"inventory_reservation_source_ref,omitempty"`

	FulfilmentMode          string                 `json:"fulfilment_mode"`
	LogisticsPlanGroupRef   string                 `json:"logistics_plan_group_ref,omitempty"`
	LogisticsMissionRef     string                 `json:"logistics_mission_ref,omitempty"`
	VendorOwnDeliveryState  VendorOwnDeliveryState `json:"vendor_own_delivery_state"`
	PromotionSnapshot       json.RawMessage        `json:"promotion_snapshot"`
	DeliveryFundingSnapshot json.RawMessage        `json:"delivery_funding_snapshot"`

	Status           VendorStatus `json:"status"`
	CancelReasonCode string       `json:"cancel_reason_code,omitempty"`
	CancelledBy      string       `json:"cancelled_by,omitempty"`

	RefundAllocatedMinor     int64  `json:"refund_allocated_minor"`
	SettlementAllocatedMinor int64  `json:"settlement_allocated_minor"`
	RefundState              string `json:"refund_state"`
	RefundOperationRef       string `json:"refund_operation_ref,omitempty"`
	RefundRefundedMinor      int64  `json:"refund_refunded_minor"`
	RefundPolicyVersion      string `json:"refund_policy_version"`
	RejectionReason          string `json:"rejection_reason,omitempty"`

	Version    int64      `json:"version"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	TerminalAt *time.Time `json:"terminal_at,omitempty"`

	Lines       []OrderLine  `json:"lines,omitempty"`
	Allocations []Allocation `json:"allocations,omitempty"`
	DeliveryETA *DeliveryETA `json:"delivery_eta,omitempty"`
}

// ParentOrder is one customer commerce purchase.
type ParentOrder struct {
	ID         string `json:"parent_order_id"`
	CustomerID string `json:"-"`

	QuoteID          string `json:"quote_ref"`
	QuoteVersion     int64  `json:"quote_version"`
	QuoteFingerprint string `json:"quote_fingerprint"`

	CheckoutSessionID          string `json:"checkout_session_id"`
	CheckoutValidationRevision int64  `json:"checkout_validation_revision"`
	CartID                     string `json:"cart_ref"`
	CartVersion                int64  `json:"cart_version"`
	FulfilmentPlanID           string `json:"fulfilment_plan_ref"`
	FulfilmentPlanRevision     int64  `json:"fulfilment_plan_revision"`
	FulfilmentPlanVersion      int64  `json:"fulfilment_plan_version"`

	Domain                 string `json:"domain"`
	Currency               string `json:"currency"`
	SelectedFulfilmentMode string `json:"selected_fulfilment_mode"`

	Address     AddressSnapshot  `json:"address_snapshot"`
	HasReceiver bool             `json:"has_receiver"`
	Receiver    ReceiverSnapshot `json:"receiver_snapshot,omitempty"`

	CustomerApprovedTotalMinor int64 `json:"customer_approved_total_minor"`
	MaxAuthorizedTotalMinor    int64 `json:"max_authorized_total_minor"`
	ProductSubtotalMinor       int64 `json:"product_subtotal_minor"`
	DeliverySubtotalMinor      int64 `json:"delivery_subtotal_minor"`
	FeeSubtotalMinor           int64 `json:"fee_subtotal_minor"`
	TaxSubtotalMinor           int64 `json:"tax_subtotal_minor"`
	DiscountTotalMinor         int64 `json:"discount_total_minor"`
	SubsidyTotalMinor          int64 `json:"subsidy_total_minor"`
	PlatformFeeTaxMinor        int64 `json:"platform_fee_tax_minor"`

	FundingBreakdown        json.RawMessage `json:"funding_breakdown"`
	PriceDisclosureSnapshot json.RawMessage `json:"price_disclosure_snapshot"`
	PromotionSnapshot       json.RawMessage `json:"promotion_snapshot"`
	DeliveryFundingSnapshot json.RawMessage `json:"delivery_funding_snapshot"`

	PaymentOperationRef string `json:"payment_operation_ref,omitempty"`

	State            ParentState `json:"state"`
	VendorOrderCount int         `json:"vendor_order_count"`

	// PaymentState mirrors the Section 17F payment operation, so a reader
	// never has to join to learn whether this order may be worked. An order
	// that is not HELD has not been paid.
	PaymentState string `json:"payment_state"`

	RefundPreparedMinor     int64 `json:"refund_prepared_minor"`
	SettlementPreparedMinor int64 `json:"settlement_prepared_minor"`

	Version    int64      `json:"version"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	TerminalAt *time.Time `json:"terminal_at,omitempty"`

	VendorOrders []VendorOrder `json:"vendor_orders,omitempty"`

	// PlatformAllocations are the parent-scope rows that belong to no single
	// vendor: the platform's own fee and tax. They are surfaced so the whole
	// approved total is readable as attributable rows, not only the part that
	// happens to sit on a vendor allocation.
	PlatformAllocations []Allocation `json:"platform_allocations,omitempty"`
}

// -----------------------------------------------------------------------------
// Ports
// -----------------------------------------------------------------------------

// QuoteAuthority resolves an approved Section 17D quote into order source
// evidence. It is a port so this feature never imports Section 17D's package.
type QuoteAuthority interface {
	// LoadApprovedQuote returns the approved agreement for this customer, or
	// ErrQuoteNotOrderable when it is not approved, has expired, or its
	// underlying checkout/reservation/fulfilment evidence has moved.
	LoadApprovedQuote(ctx context.Context, customerID, quoteID string, now time.Time) (QuoteSource, error)
}

// Actor is the authenticated vendor staff member acting on a VendorOrder.
type Actor struct {
	StaffID       string
	VendorID      string
	SessionID     string
	CorrelationID string
}

// Scope is a staff member's authorised reach for one permission.
type Scope struct {
	Global    bool
	BranchIDs []string
}

// Empty reports whether the scope authorises nothing.
func (s Scope) Empty() bool { return !s.Global && len(s.BranchIDs) == 0 }

// Covers reports whether the scope reaches a branch.
func (s Scope) Covers(branchID string) bool {
	if s.Global {
		return true
	}
	for _, id := range s.BranchIDs {
		if id == branchID {
			return true
		}
	}
	return false
}

// ScopeResolver answers "at which scopes does this staff member hold this
// permission". A port, so this feature never imports the staff repositories.
type ScopeResolver interface {
	ResolveScope(ctx context.Context, staffID, permission string) (Scope, error)
}

// Vendor staff permissions this feature evaluates. Both already exist in the
// Section 5 seed, so Section 17E adds no permission and no role mapping.
const (
	PermOrdersRead   = "vendor.orders.read"
	PermOrdersManage = "vendor.orders.manage"
)
