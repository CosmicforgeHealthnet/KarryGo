package walletmodels

import "time"

// DeliveryResponsibilityTransition is Wallet's immutable evidence that a
// future Vendor-owned delivery payable became a platform-provider payable.
// The original checkout allocation and customer/subsidy attribution remain
// untouched.
type DeliveryResponsibilityTransition struct {
	ID                                 string    `json:"id"`
	AllocationID                       string    `json:"allocation_id"`
	VendorOrderID                      string    `json:"vendor_order_id"`
	OperationRef                       string    `json:"operation_ref"`
	FromResponsibility                 string    `json:"from_responsibility"`
	ToResponsibility                   string    `json:"to_responsibility"`
	VendorDeliveryPayableBeforeMinor   int64     `json:"vendor_delivery_payable_before_minor"`
	ProviderDeliveryPayableBeforeMinor int64     `json:"provider_delivery_payable_before_minor"`
	VendorDeliveryPayableAfterMinor    int64     `json:"vendor_delivery_payable_after_minor"`
	ProviderDeliveryPayableAfterMinor  int64     `json:"provider_delivery_payable_after_minor"`
	PlatformExecutionCostMinor         int64     `json:"platform_execution_cost_minor"`
	FallbackPlatformFundingMinor       int64     `json:"fallback_platform_funding_minor"`
	CustomerContributionMinor          int64     `json:"customer_contribution_minor"`
	VendorSubsidyMinor                 int64     `json:"vendor_subsidy_minor"`
	PlatformSubsidyMinor               int64     `json:"platform_subsidy_minor"`
	Status                             string    `json:"status"`
	CreatedAt                          time.Time `json:"created_at"`
	IdempotencyKey, RequestFingerprint string
}
