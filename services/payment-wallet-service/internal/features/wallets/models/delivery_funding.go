package walletmodels

import "time"

const (
	AccountPlatformPromotionExpense = "platform_promotion_expense"
	StatusFundingApplied            = "APPLIED"
	StatusFundingReversed           = "REVERSED"
)

type DeliveryFundingAllocation struct {
	ID                         string    `json:"id"`
	SourceService              string    `json:"source_service"`
	SourceReference            string    `json:"source_reference"`
	VendorOrderID              string    `json:"vendor_order_id"`
	SponsoringVendorID         string    `json:"sponsoring_vendor_id"`
	ProviderType               string    `json:"provider_type"`
	ProviderID                 string    `json:"provider_id"`
	Currency                   string    `json:"currency"`
	AuthoritativeDeliveryMinor int64     `json:"authoritative_delivery_minor"`
	CustomerContributionMinor  int64     `json:"customer_contribution_minor"`
	VendorSubsidyMinor         int64     `json:"vendor_subsidy_minor"`
	PlatformSubsidyMinor       int64     `json:"platform_subsidy_minor"`
	PlatformApprovalRef        string    `json:"platform_approval_ref,omitempty"`
	CustomerPaymentReference   string    `json:"customer_payment_reference,omitempty"`
	LedgerTransactionRef       string    `json:"ledger_transaction_ref,omitempty"`
	Status                     string    `json:"status"`
	IdempotencyKey             string    `json:"-"`
	RequestFingerprint         string    `json:"-"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}
