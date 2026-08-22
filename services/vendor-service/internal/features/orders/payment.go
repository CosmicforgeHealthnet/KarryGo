package orders

// Section 17F payment binding.
//
// Vendor Commerce is not a money authority. Nothing in this file computes a
// balance, edits a ledger, settles vendor funds or decides that a payment
// succeeded. It binds an approved commercial agreement to a payment operation
// and records what the Wallet authority answered.
//
// The binding is the security boundary: a payment operation carries the exact
// quote, version, fingerprint, totals, currency, customer and commercial
// allocation set it was started for. A materially different input is a
// different binding, and a different binding is refused under the same
// operation rather than quietly re-priced.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"
)

// PaymentState is the lifecycle of one order's payment operation.
type PaymentState string

const (
	// PaymentPending is the state an order is created in. It is never
	// fulfilment-ready: an order that has not been paid may not be worked.
	PaymentPending PaymentState = "PENDING_PAYMENT"

	// PaymentAuthorizing means legs exist and the Wallet authority has been
	// asked, but not every leg has answered yet. A crash here is recoverable
	// because the legs are durable before the first call is made.
	PaymentAuthorizing PaymentState = "AUTHORIZING"

	// PaymentHeld means every leg is held in escrow by the Wallet authority.
	PaymentHeld PaymentState = "HELD"

	// PaymentFailed is terminal for this operation. Inventory is released and
	// any held leg is compensated.
	PaymentFailed PaymentState = "FAILED"

	// PaymentCompensating and PaymentCompensated cover the unwind of a
	// partially-held operation that can never complete.
	PaymentCompensating PaymentState = "COMPENSATING"
	PaymentCompensated  PaymentState = "COMPENSATED"
)

// Live reports whether a payment operation may still progress.
func (s PaymentState) Live() bool {
	return s == PaymentPending || s == PaymentAuthorizing || s == PaymentHeld
}

// Terminal reports whether a payment operation can no longer change.
func (s PaymentState) Terminal() bool {
	return s == PaymentFailed || s == PaymentCompensated
}

// PaymentScope names what one payment leg pays for.
type PaymentScope string

const (
	// ScopeVendor is one VendorOrder's own share of the customer's approved
	// total. It settles to that vendor and to nobody else.
	ScopeVendorLeg PaymentScope = "VENDOR"

	// ScopePlatformLeg is the parent-scope fee and tax that belongs to no
	// single vendor.
	ScopePlatformLeg PaymentScope = "PLATFORM"
)

// LegStatus is one payment leg's own state, mirroring what the Wallet
// authority reported. Vendor Commerce never invents a value here.
type LegStatus string

const (
	LegPending  LegStatus = "PENDING"
	LegHeld     LegStatus = "HELD"
	LegSettled  LegStatus = "SETTLED"
	LegRefunded LegStatus = "REFUNDED"
	LegFailed   LegStatus = "FAILED"
)

var (
	ErrPaymentNotBound = errors.New("payment operation is not bound to this agreement")

	// ErrBindingMismatch is the tamper guard. The commercial inputs a payment
	// operation was started for no longer match the ones presented.
	ErrBindingMismatch = errors.New("payment binding does not match the approved agreement")

	ErrPaymentConflict = errors.New("payment operation conflicts with its current state")

	ErrPaymentAuthority = errors.New("wallet authority could not complete the request")

	ErrPaymentNotConfigured = errors.New("wallet authority is not configured")
)

// PaymentBinding is the exact commercial agreement a payment operation is
// bound to. Every field is server-resolved from approved evidence; none of it
// is ever accepted from a request body.
type PaymentBinding struct {
	QuoteID                 string `json:"quote_id"`
	QuoteVersion            int64  `json:"quote_version"`
	QuoteFingerprint        string `json:"quote_fingerprint"`
	QuotedTotalMinor        int64  `json:"quoted_total_minor"`
	MaxAuthorizedTotalMinor int64  `json:"max_authorized_total_minor"`
	Currency                string `json:"currency"`
	CustomerID              string `json:"customer_id"`

	// AllocationFingerprint binds the commercial allocation set itself, so a
	// re-allocated order is a different binding even when its total is
	// unchanged. This is what stops one vendor's share being moved to another
	// under an already-started payment.
	AllocationFingerprint string `json:"allocation_fingerprint"`
}

// Matches reports whether two bindings describe the same agreement.
func (b PaymentBinding) Matches(other PaymentBinding) bool {
	return b == other
}

// BindingFor derives the payment binding from an order that already exists.
//
// It reads the stored order, never a request, so a client cannot present a
// total, a currency or an allocation of its own.
func BindingFor(order ParentOrder) (PaymentBinding, error) {
	if order.ID == "" || order.QuoteID == "" || order.CustomerID == "" ||
		order.Currency == "" || order.QuoteFingerprint == "" {
		return PaymentBinding{}, ErrPaymentNotBound
	}
	allocation, err := AllocationFingerprint(order)
	if err != nil {
		return PaymentBinding{}, err
	}
	return PaymentBinding{
		QuoteID:                 order.QuoteID,
		QuoteVersion:            order.QuoteVersion,
		QuoteFingerprint:        order.QuoteFingerprint,
		QuotedTotalMinor:        order.CustomerApprovedTotalMinor,
		MaxAuthorizedTotalMinor: order.MaxAuthorizedTotalMinor,
		Currency:                order.Currency,
		CustomerID:              order.CustomerID,
		AllocationFingerprint:   allocation,
	}, nil
}

// allocationMaterial is the canonical, order-independent shape the allocation
// fingerprint is computed over.
type allocationMaterial struct {
	ParentOrderID       string             `json:"parent_order_id"`
	Currency            string             `json:"currency"`
	ApprovedTotalMinor  int64              `json:"approved_total_minor"`
	PlatformFeeTaxMinor int64              `json:"platform_fee_tax_minor"`
	Vendors             []vendorAllocation `json:"vendors"`
}

type vendorAllocation struct {
	VendorOrderID        string `json:"vendor_order_id"`
	BusinessID           string `json:"business_id"`
	BranchID             string `json:"branch_id"`
	QuoteGroupRef        string `json:"quote_group_ref"`
	CustomerPayableMinor int64  `json:"customer_payable_minor"`
	VendorPayableMinor   int64  `json:"vendor_payable_minor"`
}

// AllocationFingerprint deterministically binds an order's commercial
// allocation set.
func AllocationFingerprint(order ParentOrder) (string, error) {
	material := allocationMaterial{
		ParentOrderID:       order.ID,
		Currency:            order.Currency,
		ApprovedTotalMinor:  order.CustomerApprovedTotalMinor,
		PlatformFeeTaxMinor: order.PlatformFeeTaxMinor,
		Vendors:             make([]vendorAllocation, 0, len(order.VendorOrders)),
	}
	for _, vendorOrder := range order.VendorOrders {
		material.Vendors = append(material.Vendors, vendorAllocation{
			VendorOrderID:        vendorOrder.ID,
			BusinessID:           vendorOrder.BusinessID,
			BranchID:             vendorOrder.BranchID,
			QuoteGroupRef:        vendorOrder.QuoteGroupRef,
			CustomerPayableMinor: vendorOrder.CustomerPayableMinor,
			VendorPayableMinor:   vendorOrder.VendorPayableMinor,
		})
	}
	// A stable order keeps the fingerprint independent of query ordering.
	sort.Slice(material.Vendors, func(i, j int) bool {
		return material.Vendors[i].VendorOrderID < material.Vendors[j].VendorOrderID
	})

	encoded, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// PaymentLeg is one payment the Wallet authority is asked to hold.
//
// Deliberately one leg per VendorOrder. The Wallet contract settles and
// refunds a payment intent as a whole, to a single provider, so a single
// parent intent could never refund one vendor while settling another. Splitting
// at creation is what makes multi-vendor refund isolation structural rather
// than something a later section has to work around.
type PaymentLeg struct {
	Scope         PaymentScope
	VendorOrderID string
	BusinessID    string
	BranchID      string
	AmountMinor   int64

	// IdempotencyKey is deterministic, so a retried start reuses the same key
	// and the Wallet authority returns the same intent rather than a second one.
	IdempotencyKey string
}

// PlanPaymentLegs derives the leg set from an order's own allocations.
//
// The guard it enforces: the legs must sum to exactly the customer-approved
// total. If they do not, no payment is started at all, because the customer
// would otherwise be charged an amount nobody agreed to.
func PlanPaymentLegs(order ParentOrder) ([]PaymentLeg, error) {
	if len(order.VendorOrders) == 0 {
		return nil, ErrPaymentNotBound
	}

	legs := make([]PaymentLeg, 0, len(order.VendorOrders)+1)
	var total int64

	vendors := append([]VendorOrder(nil), order.VendorOrders...)
	sort.Slice(vendors, func(i, j int) bool { return vendors[i].ID < vendors[j].ID })

	for _, vendorOrder := range vendors {
		if vendorOrder.CustomerPayableMinor < 0 {
			return nil, ErrPaymentNotBound
		}
		total += vendorOrder.CustomerPayableMinor
		if vendorOrder.CustomerPayableMinor == 0 {
			// A zero-value allocation needs no payment. It still exists as a
			// VendorOrder; it simply has nothing for the customer to pay.
			continue
		}
		legs = append(legs, PaymentLeg{
			Scope:          ScopeVendorLeg,
			VendorOrderID:  vendorOrder.ID,
			BusinessID:     vendorOrder.BusinessID,
			BranchID:       vendorOrder.BranchID,
			AmountMinor:    vendorOrder.CustomerPayableMinor,
			IdempotencyKey: "vendor-order-payment-" + vendorOrder.ID,
		})
	}

	if order.PlatformFeeTaxMinor < 0 {
		return nil, ErrPaymentNotBound
	}
	total += order.PlatformFeeTaxMinor
	if order.PlatformFeeTaxMinor > 0 {
		legs = append(legs, PaymentLeg{
			Scope:          ScopePlatformLeg,
			AmountMinor:    order.PlatformFeeTaxMinor,
			IdempotencyKey: "parent-order-platform-" + order.ID,
		})
	}

	if total != order.CustomerApprovedTotalMinor {
		return nil, ErrAllocationImbalance
	}
	if len(legs) == 0 {
		return nil, ErrPaymentNotBound
	}
	return legs, nil
}

// PaymentOperation is the durable record binding one order to one payment.
type PaymentOperation struct {
	ID            string         `json:"payment_operation_id"`
	ParentOrderID string         `json:"parent_order_id"`
	CustomerID    string         `json:"-"`
	Binding       PaymentBinding `json:"binding"`
	PaymentMethod string         `json:"payment_method"`
	State         PaymentState   `json:"state"`
	FailureCode   string         `json:"failure_code,omitempty"`

	Version    int64      `json:"version"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	SettledAt  *time.Time `json:"settled_at,omitempty"`
	TerminalAt *time.Time `json:"terminal_at,omitempty"`

	Legs []PaymentLegRecord `json:"legs,omitempty"`
}

// PaymentLegRecord is one persisted leg and whatever the Wallet authority last
// reported about it.
type PaymentLegRecord struct {
	ID                 string       `json:"payment_leg_id"`
	PaymentOperationID string       `json:"-"`
	Scope              PaymentScope `json:"scope"`
	VendorOrderID      string       `json:"vendor_order_id,omitempty"`
	BusinessID         string       `json:"business_id,omitempty"`
	AmountMinor        int64        `json:"amount_minor"`
	Currency           string       `json:"currency"`

	IdempotencyKey string `json:"-"`

	// IntentReference and IntentID are the Wallet authority's own identities.
	// They are recorded, never generated here.
	IntentReference string `json:"intent_reference,omitempty"`
	IntentID        string `json:"intent_id,omitempty"`

	// AuthorityStatus is the raw status the Wallet authority reported, kept
	// verbatim so a Vendor-side reading can always be checked against it.
	AuthorityStatus string    `json:"authority_status,omitempty"`
	Status          LegStatus `json:"status"`
	FailureReason   string    `json:"failure_reason,omitempty"`

	RefundedMinor int64 `json:"refunded_minor"`
	SettledMinor  int64 `json:"settled_minor"`

	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DerivePaymentState computes the operation state from its legs.
//
// It is derived rather than asserted for the same reason the ParentOrder's
// aggregate state is: a partially-held payment must never be able to report
// itself as fully held because somebody wrote that value.
func DerivePaymentState(legs []PaymentLegRecord) PaymentState {
	if len(legs) == 0 {
		return PaymentPending
	}

	var held, failed int
	for _, leg := range legs {
		switch leg.Status {
		case LegHeld, LegSettled, LegRefunded:
			// Settled and refunded legs were necessarily held first, so they
			// still count as "the authority took the money".
			held++
		case LegFailed:
			failed++
		}
	}

	switch {
	case failed > 0:
		// Any failed leg means this operation can never be a complete payment.
		// It unwinds rather than half-succeeding.
		return PaymentCompensating
	case held == len(legs):
		return PaymentHeld
	default:
		return PaymentAuthorizing
	}
}

// PaymentAuthority is the narrow port onto the platform's money authority.
//
// Every method maps to an existing payment-wallet-service contract. There is
// deliberately no method that computes a balance, writes a ledger entry or
// declares a payment successful: those are the authority's own, and Vendor
// Commerce may only ask and record.
type PaymentAuthority interface {
	// Configured reports whether a real Wallet authority is wired. With it
	// false no payment route is registered at all.
	Configured() bool

	// HoldLeg creates and holds one payment leg in escrow. It must be
	// idempotent for the same idempotency key.
	HoldLeg(ctx context.Context, request HoldLegRequest) (LegResult, error)

	// GetLeg re-reads the authority's own current view of a leg, used by
	// recovery to learn what actually happened across a crash.
	GetLeg(ctx context.Context, reference string) (LegResult, error)

	// RefundLeg returns one leg's money to the customer. It must be idempotent
	// for the same idempotency key, and it refunds exactly one leg: refunding
	// one vendor's allocation never touches another's.
	RefundLeg(ctx context.Context, request RefundLegRequest) (RefundResult, error)
}

// PaymentSettlementAuthority is the optional extension implemented by the
// real Wallet adapter. Keeping it separate preserves compatibility with
// narrow test authorities and makes settlement capability explicit.
type PaymentSettlementAuthority interface {
	CompleteLeg(ctx context.Context, sourceService, sourceReference string) (LegResult, error)
}

// HoldLegRequest asks the Wallet authority to hold one leg in escrow.
//
// Every field is derived from the stored order. There is no field a client can
// influence, and no field that names an amount the order did not allocate.
type HoldLegRequest struct {
	IdempotencyKey string
	SourceService  string
	// SourceReference is the VendorOrder id for a vendor leg and the
	// ParentOrder id for the platform leg, so the authority's own record points
	// back at exactly one allocation.
	SourceReference string
	CustomerID      string
	CustomerEmail   string
	AmountKobo      int64
	Currency        string
	// ProviderType and ProviderID name the beneficiary of a vendor leg, which
	// the authority requires before it can ever settle. The platform leg names
	// no provider.
	ProviderType string
	ProviderID   string
	Metadata     map[string]any
}

// LegResult is the Wallet authority's answer about one leg.
type LegResult struct {
	IntentID   string
	Reference  string
	Status     string
	AmountKobo int64
}

// Held maps the authority's own status vocabulary onto this feature's.
//
// Only the authority's word is trusted: an unrecognised status is never
// optimistically treated as success.
func (r LegResult) Held() bool {
	switch r.Status {
	case "held", "HELD":
		return true
	default:
		return false
	}
}

// Settled reports a leg the authority has already released to its beneficiary.
func (r LegResult) Settled() bool {
	switch r.Status {
	case "completed", "COMPLETED":
		return true
	default:
		return false
	}
}

// Refunded reports a leg the authority has refunded in whole or in part.
func (r LegResult) Refunded() bool {
	switch r.Status {
	case "refunded", "REFUNDED", "part_refunded", "PART_REFUNDED":
		return true
	default:
		return false
	}
}

// Failed reports a leg the authority terminally rejected.
func (r LegResult) Failed() bool {
	switch r.Status {
	case "failed", "FAILED", "cancelled", "CANCELLED":
		return true
	default:
		return false
	}
}

// StatusFor maps an authority answer onto the leg status this feature stores.
// An unknown answer keeps the leg PENDING so recovery re-reads it rather than
// guessing.
func StatusFor(result LegResult) LegStatus {
	switch {
	case result.Refunded():
		return LegRefunded
	case result.Settled():
		return LegSettled
	case result.Held():
		return LegHeld
	case result.Failed():
		return LegFailed
	default:
		return LegPending
	}
}
