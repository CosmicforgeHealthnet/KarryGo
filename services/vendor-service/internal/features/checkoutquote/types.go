// Package checkoutquote owns Section 17D's immutable commercial agreement.
//
// It consumes, but never rewrites, Section 17B checkout evidence and Section
// 17C fulfilment plans. It creates no payment, ParentOrder or VendorOrder.
package checkoutquote

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type State string

const (
	StateQuoted   State = "QUOTED"
	StateApproved State = "APPROVED"
	StateExpired  State = "EXPIRED"
)

type ReplanDisposition string

const (
	ReplanUnchanged                 ReplanDisposition = "UNCHANGED"
	ReplanReleaseOrRefundDifference ReplanDisposition = "RELEASE_OR_REFUND_DIFFERENCE"
	ReplanCustomerReapproval        ReplanDisposition = "CUSTOMER_REAPPROVAL_REQUIRED"
)

type ReplanAssessment struct {
	CurrentQuoteID      string            `json:"current_quote_id"`
	CandidateQuoteID    string            `json:"candidate_quote_id"`
	CurrentTotalMinor   int64             `json:"current_total_minor"`
	CandidateTotalMinor int64             `json:"candidate_total_minor"`
	DifferenceMinor     int64             `json:"difference_minor"`
	Disposition         ReplanDisposition `json:"disposition"`
}

type AdjustmentType string

const (
	AdjustmentDiscount AdjustmentType = "DISCOUNT"
	AdjustmentFee      AdjustmentType = "FEE"
	AdjustmentTax      AdjustmentType = "TAX"
)

type FundingSource string

const (
	FundingCustomer        FundingSource = "CUSTOMER"
	FundingVendor          FundingSource = "VENDOR"
	FundingPlatform        FundingSource = "PLATFORM"
	FundingDeliverySubsidy FundingSource = "DELIVERY_SUBSIDY"
	FundingOtherApproved   FundingSource = "OTHER_APPROVED"
)

const (
	ScopeProduct  = "PRODUCT"
	ScopeDelivery = "DELIVERY"
	ScopeOrder    = "ORDER"
)

var (
	ErrQuoteNotFound       = errors.New("commerce quote not found")
	ErrSourceNotQuotable   = errors.New("checkout or fulfilment plan cannot support a quote")
	ErrQuoteExpired        = errors.New("commerce quote expired")
	ErrQuoteConflict       = errors.New("commerce quote conflicts with current commercial evidence")
	ErrFingerprintMismatch = errors.New("commerce quote fingerprint mismatch")
	ErrInvalidAdjustment   = errors.New("commercial adjustment is invalid")
	ErrDependencyMissing   = errors.New("required quote dependency is not connected")
	ErrInvalidTTL          = errors.New("commerce quote ttl is invalid")
)

const (
	MinQuoteTTL     = time.Minute
	MaxQuoteTTL     = 24 * time.Hour
	DefaultQuoteTTL = 5 * time.Minute
)

type AddressSnapshot struct {
	SourceAddressRef string    `json:"source_address_ref"`
	Title            string    `json:"title,omitempty"`
	Address          string    `json:"address"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	Description      string    `json:"description,omitempty"`
	SnapshottedAt    time.Time `json:"snapshotted_at"`
}

type Adjustment struct {
	ID                    string          `json:"id"`
	Type                  AdjustmentType  `json:"type"`
	Scope                 string          `json:"scope"`
	FundingSource         FundingSource   `json:"funding_source"`
	SourceRef             string          `json:"source_ref"`
	Description           string          `json:"description,omitempty"`
	AmountMinor           int64           `json:"amount_minor"`
	BusinessID            string          `json:"business_id,omitempty"`
	BranchID              string          `json:"branch_id,omitempty"`
	PromotionType         string          `json:"promotion_type,omitempty"`
	CampaignVersion       int64           `json:"campaign_version,omitempty"`
	EligibilityEvidence   json.RawMessage `json:"eligibility_evidence,omitempty"`
	CouponDigest          string          `json:"coupon_digest,omitempty"`
	PlatformApprovalRef   string          `json:"platform_approval_ref,omitempty"`
	CalculationBaseMinor  int64           `json:"calculation_base_minor,omitempty"`
	DeliveryDiscountMinor int64           `json:"delivery_discount_minor,omitempty"`
	StackPriority         int             `json:"stack_priority,omitempty"`
	AppliedAt             *time.Time      `json:"applied_at,omitempty"`
}

// FundingBreakdown keeps the customer benefit attributable. A delivery
// subsidy is never silently deducted from a vendor or platform payable.
type FundingBreakdown struct {
	ProductCustomerMinor       int64 `json:"product_customer_minor"`
	ProductVendorFundedMinor   int64 `json:"product_vendor_funded_minor"`
	ProductPlatformFundedMinor int64 `json:"product_platform_funded_minor"`
	ProductOtherFundedMinor    int64 `json:"product_other_funded_minor"`

	DeliveryGrossMinor          int64 `json:"delivery_gross_minor"`
	DeliveryCustomerMinor       int64 `json:"delivery_customer_minor"`
	DeliveryVendorFundedMinor   int64 `json:"delivery_vendor_funded_minor"`
	DeliveryPlatformFundedMinor int64 `json:"delivery_platform_funded_minor"`
	DeliveryOtherFundedMinor    int64 `json:"delivery_other_funded_minor"`

	OrderVendorFundedMinor   int64 `json:"order_vendor_funded_minor"`
	OrderPlatformFundedMinor int64 `json:"order_platform_funded_minor"`
	OrderOtherFundedMinor    int64 `json:"order_other_funded_minor"`

	FeeCustomerMinor int64 `json:"fee_customer_minor"`
	TaxCustomerMinor int64 `json:"tax_customer_minor"`
}

type PriceDisclosureSnapshot struct {
	ProductSubtotalMinor  int64 `json:"product_subtotal_minor"`
	DeliverySubtotalMinor int64 `json:"delivery_subtotal_minor"`
	FeeSubtotalMinor      int64 `json:"fee_subtotal_minor"`
	TaxSubtotalMinor      int64 `json:"tax_subtotal_minor"`
	DiscountTotalMinor    int64 `json:"discount_total_minor"`
	SubsidyTotalMinor     int64 `json:"subsidy_total_minor"`
	FinalPayableMinor     int64 `json:"final_payable_minor"`

	PriceContext            string `json:"price_context"`
	StorePriceParityClaimed bool   `json:"store_price_parity_claimed"`
}

type LineSnapshot struct {
	CheckoutLineID  string `json:"checkout_line_id"`
	CheckoutGroupID string `json:"checkout_group_id"`
	CartItemID      string `json:"cart_item_id"`
	Domain          string `json:"domain"`
	BusinessID      string `json:"business_id"`
	BranchID        string `json:"branch_id"`
	ItemRef         string `json:"item_ref"`
	VariantRef      string `json:"variant_ref,omitempty"`
	ListingRef      string `json:"listing_ref,omitempty"`
	SellUnitRef     string `json:"sell_unit_ref,omitempty"`
	Quantity        int64  `json:"quantity"`

	VendorOnlineUnitPriceMinor int64  `json:"vendor_online_unit_price_minor"`
	PriceReference             string `json:"price_reference"`
	CatalogueRevision          int64  `json:"catalogue_revision"`
	InventoryRevision          *int64 `json:"inventory_revision,omitempty"`

	PricingBasis            string `json:"pricing_basis"`
	RequestedMeasureRef     string `json:"requested_measure_ref,omitempty"`
	ExpectedMeasureQuantity int64  `json:"expected_measure_quantity"`
	MeasureUnderfillBps     int    `json:"measure_underfill_bps"`
	MeasureOverfillBps      int    `json:"measure_overfill_bps"`
	QuotedTotalMinor        int64  `json:"quoted_total_minor"`
	MaxAuthorizedTotalMinor int64  `json:"max_authorized_total_minor"`
	InventoryItemRef        string `json:"inventory_item_ref,omitempty"`
	InventoryReservationRef string `json:"inventory_reservation_ref,omitempty"`

	OnlinePriceDisclosure   string `json:"online_price_disclosure"`
	StorePriceParityClaimed bool   `json:"store_price_parity_claimed"`
}

type GroupSnapshot struct {
	CheckoutGroupID               string          `json:"checkout_group_id"`
	BusinessID                    string          `json:"business_id"`
	BranchID                      string          `json:"branch_id"`
	ProductSubtotalMinor          int64           `json:"product_subtotal_minor"`
	MaxProductSubtotalMinor       int64           `json:"max_product_subtotal_minor"`
	InventoryReservationRef       string          `json:"inventory_reservation_ref,omitempty"`
	InventoryReservationSourceRef string          `json:"inventory_reservation_source_ref,omitempty"`
	LogisticsPlanGroupRef         string          `json:"logistics_plan_group_ref"`
	LogisticsMissionRef           string          `json:"logistics_mission_ref,omitempty"`
	DeliveryFunding               DeliveryFunding `json:"delivery_funding"`
}

type DeliveryFunding struct {
	GrossMinor          int64 `json:"gross_minor"`
	CustomerMinor       int64 `json:"customer_minor"`
	VendorFundedMinor   int64 `json:"vendor_funded_minor"`
	PlatformFundedMinor int64 `json:"platform_funded_minor"`
}

type MissionSnapshot struct {
	MissionID               string          `json:"mission_id"`
	Authority               string          `json:"authority"`
	Service                 string          `json:"service"`
	VehicleClass            string          `json:"vehicle_class,omitempty"`
	AuthorityQuoteID        string          `json:"authority_quote_id,omitempty"`
	AuthorityQuoteReference string          `json:"authority_quote_reference,omitempty"`
	AuthorityQuoteRevision  int64           `json:"authority_quote_revision,omitempty"`
	AuthorityQuoteExpiresAt *time.Time      `json:"authority_quote_expires_at,omitempty"`
	GroupIDs                []string        `json:"group_ids"`
	Funding                 DeliveryFunding `json:"funding"`
}

// SourceSnapshot is the exact immutable evidence a quote is built from.
type SourceSnapshot struct {
	CheckoutSessionID            string
	CheckoutValidationRevision   int64
	CheckoutExpiresAt            time.Time
	CartID                       string
	CartVersion                  int64
	CustomerID                   string
	Currency                     string
	Address                      AddressSnapshot
	QuotedItemsTotalMinor        int64
	MaxAuthorizedItemsTotalMinor int64

	FulfilmentPlanID        string
	FulfilmentPlanRevision  int64
	FulfilmentPlanVersion   int64
	FulfilmentPlanExpiresAt time.Time
	SelectedFulfilmentMode  string
	DeliveryFunding         DeliveryFunding

	Lines    []LineSnapshot
	Groups   []GroupSnapshot
	Missions []MissionSnapshot
}

type PromotionContext struct {
	CustomerID            string
	Currency              string
	ProductSubtotalMinor  int64
	DeliverySubtotalMinor int64
	Groups                []GroupSnapshot
	Lines                 []LineSnapshot
	CouponCode            string
	At                    time.Time
}

type PromotionAuthority interface {
	Evaluate(ctx context.Context, input PromotionContext) ([]Adjustment, error)
}

type Quote struct {
	ID                         string          `json:"quote_id"`
	QuoteVersion               int64           `json:"quote_version"`
	QuoteFingerprint           string          `json:"quote_fingerprint"`
	MaterialFingerprint        string          `json:"-"`
	CustomerID                 string          `json:"customer_ref"`
	CheckoutSessionID          string          `json:"checkout_session_id"`
	CheckoutValidationRevision int64           `json:"checkout_validation_revision"`
	CartID                     string          `json:"cart_ref"`
	CartVersion                int64           `json:"cart_version"`
	FulfilmentPlanID           string          `json:"fulfilment_plan_ref"`
	FulfilmentPlanRevision     int64           `json:"fulfilment_plan_revision"`
	FulfilmentPlanVersion      int64           `json:"fulfilment_plan_version"`
	Currency                   string          `json:"currency"`
	SelectedFulfilmentMode     string          `json:"selected_fulfilment_mode"`
	Address                    AddressSnapshot `json:"address_snapshot"`
	ExpiresAt                  time.Time       `json:"expires_at"`

	ProductSubtotalMinor    int64 `json:"product_subtotal_minor"`
	MaxProductSubtotalMinor int64 `json:"max_product_subtotal_minor"`
	DeliverySubtotalMinor   int64 `json:"delivery_subtotal_minor"`
	FeeSubtotalMinor        int64 `json:"fee_subtotal_minor"`
	TaxSubtotalMinor        int64 `json:"tax_subtotal_minor"`
	DiscountTotalMinor      int64 `json:"discount_total_minor"`
	SubsidyTotalMinor       int64 `json:"subsidy_total_minor"`
	FinalPayableMinor       int64 `json:"final_payable_minor"`
	QuotedTotalMinor        int64 `json:"quoted_total_minor"`
	MaxAuthorizedTotalMinor int64 `json:"max_authorized_total_minor"`

	FundingBreakdown FundingBreakdown        `json:"funding_breakdown"`
	PriceDisclosure  PriceDisclosureSnapshot `json:"price_disclosure_snapshot"`
	Groups           []GroupSnapshot         `json:"vendor_branch_groups"`
	Lines            []LineSnapshot          `json:"lines"`
	Missions         []MissionSnapshot       `json:"logistics_quotes"`
	Adjustments      []Adjustment            `json:"commercial_adjustments"`

	State      State      `json:"state"`
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	Expired    bool       `json:"expired"`
	CreatedAt  time.Time  `json:"created_at"`
}
