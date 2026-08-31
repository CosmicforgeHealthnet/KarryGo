package walletmodels

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	OwnerTypeVendor = "vendor"

	AccountCommerceEscrow       = "commerce_escrow"
	AccountVendorPayable        = "vendor_payable"
	AccountOtherApprovedFunding = "other_approved_funding"

	CommerceIntentCreated           = "CREATED"
	CommerceIntentHeld              = "HELD"
	CommerceIntentPartiallyReleased = "PARTIALLY_RELEASED"
	CommerceIntentReleased          = "RELEASED"
	CommerceIntentPartiallyRefunded = "PARTIALLY_REFUNDED"
	CommerceIntentRefunded          = "REFUNDED"

	CommerceAllocationVendor   = "VENDOR"
	CommerceAllocationPlatform = "PLATFORM"

	DeliveryResponsibilityNone     = "NONE"
	DeliveryResponsibilityPlatform = "PLATFORM"
	DeliveryResponsibilityVendor   = "VENDOR"
)

var ErrInvalidCommerceAllocation = errors.New("commerce allocation is invalid or does not conserve money")

type CommercePaymentIntent struct {
	PaymentIntentID         string               `json:"payment_intent_id"`
	PaymentReference        string               `json:"payment_reference"`
	ParentCommerceReference string               `json:"parent_commerce_reference"`
	CustomerID              string               `json:"customer_id"`
	QuoteReference          string               `json:"quote_reference"`
	QuoteVersion            int64                `json:"quote_version"`
	QuoteFingerprint        string               `json:"quote_fingerprint"`
	FinalAuthorizedMinor    int64                `json:"final_authorized_minor"`
	MaxAuthorizedMinor      int64                `json:"max_authorized_minor"`
	AllocationFingerprint   string               `json:"allocation_fingerprint"`
	Currency                string               `json:"currency"`
	PaymentMethod           string               `json:"payment_method"`
	Status                  string               `json:"status"`
	IdempotencyKey          string               `json:"idempotency_key"`
	RequestFingerprint      string               `json:"-"`
	Version                 int64                `json:"version"`
	Allocations             []CommerceAllocation `json:"allocations"`
	CreatedAt               time.Time            `json:"created_at"`
	UpdatedAt               time.Time            `json:"updated_at"`
}

type CommerceAllocation struct {
	ID                                    string          `json:"id"`
	PaymentIntentID                       string          `json:"payment_intent_id"`
	AllocationReference                   string          `json:"allocation_reference"`
	Scope                                 string          `json:"scope"`
	VendorOrderID                         string          `json:"vendor_order_id,omitempty"`
	VendorID                              string          `json:"vendor_id,omitempty"`
	BranchID                              string          `json:"branch_id,omitempty"`
	AssignedProviderType                  string          `json:"assigned_provider_type,omitempty"`
	AssignedProviderID                    string          `json:"assigned_provider_id,omitempty"`
	DeliveryResponsibility                string          `json:"delivery_responsibility"`
	Currency                              string          `json:"currency"`
	CustomerPayableMinor                  int64           `json:"customer_payable_minor"`
	MaxAuthorizedMinor                    int64           `json:"max_authorized_minor"`
	MerchandiseGrossMinor                 int64           `json:"merchandise_gross_minor"`
	VendorProductDiscountMinor            int64           `json:"vendor_product_discount_minor"`
	PlatformProductDiscountMinor          int64           `json:"platform_product_discount_minor"`
	PlatformCommissionMinor               int64           `json:"platform_commission_minor"`
	DeliveryCustomerMinor                 int64           `json:"delivery_customer_minor"`
	VendorDeliverySubsidyMinor            int64           `json:"vendor_delivery_subsidy_minor"`
	PlatformDeliverySubsidyMinor          int64           `json:"platform_delivery_subsidy_minor"`
	OtherFundingMinor                     int64           `json:"other_funding_minor"`
	VendorMerchandisePayableMinor         int64           `json:"vendor_merchandise_payable_minor"`
	VendorDeliveryPayableMinor            int64           `json:"vendor_delivery_payable_minor"`
	ProviderDeliveryPayableMinor          int64           `json:"provider_delivery_payable_minor"`
	PlatformRevenueMinor                  int64           `json:"platform_revenue_minor"`
	VendorReleasedMinor                   int64           `json:"vendor_released_minor"`
	DeliveryReleasedMinor                 int64           `json:"delivery_released_minor"`
	RefundedMinor                         int64           `json:"refunded_minor"`
	RefundCustomerMinor                   int64           `json:"refund_customer_minor"`
	RefundPlatformFundingMinor            int64           `json:"refund_platform_funding_minor"`
	RefundOtherFundingMinor               int64           `json:"refund_other_funding_minor"`
	RefundEscrowRecoveryMinor             int64           `json:"refund_escrow_recovery_minor"`
	RefundVendorRecoveryMinor             int64           `json:"refund_vendor_recovery_minor"`
	RefundProviderRecoveryMinor           int64           `json:"refund_provider_recovery_minor"`
	RefundPlatformRecoveryMinor           int64           `json:"refund_platform_recovery_minor"`
	RefundPlatformLossFundingMinor        int64           `json:"refund_platform_loss_funding_minor"`
	RefundVendorMerchandiseReductionMinor int64           `json:"refund_vendor_merchandise_reduction_minor"`
	RefundVendorDeliveryReductionMinor    int64           `json:"refund_vendor_delivery_reduction_minor"`
	RefundProviderDeliveryReductionMinor  int64           `json:"refund_provider_delivery_reduction_minor"`
	RefundPlatformRevenueReductionMinor   int64           `json:"refund_platform_revenue_reduction_minor"`
	RefundAuthorizationReleaseMinor       int64           `json:"refund_authorization_release_minor"`
	Status                                string          `json:"status"`
	Snapshot                              json.RawMessage `json:"snapshot"`
	SnapshotFingerprint                   string          `json:"snapshot_fingerprint"`
	Version                               int64           `json:"version"`
	CreatedAt                             time.Time       `json:"created_at"`
	UpdatedAt                             time.Time       `json:"updated_at"`
}

func (a CommerceAllocation) PlatformFundingMinor() int64 {
	return a.PlatformProductDiscountMinor + a.PlatformDeliverySubsidyMinor
}

func (a CommerceAllocation) ExternalFundingMinor() int64 {
	return a.PlatformFundingMinor() + a.OtherFundingMinor
}

func (a CommerceAllocation) OutputMinor() int64 {
	return a.VendorMerchandisePayableMinor + a.VendorDeliveryPayableMinor + a.ProviderDeliveryPayableMinor + a.PlatformRevenueMinor
}

func (a CommerceAllocation) Validate(currency string) error {
	values := []int64{a.CustomerPayableMinor, a.MaxAuthorizedMinor, a.MerchandiseGrossMinor, a.VendorProductDiscountMinor,
		a.PlatformProductDiscountMinor, a.PlatformCommissionMinor, a.DeliveryCustomerMinor,
		a.VendorDeliverySubsidyMinor, a.PlatformDeliverySubsidyMinor, a.OtherFundingMinor, a.VendorMerchandisePayableMinor,
		a.VendorDeliveryPayableMinor, a.ProviderDeliveryPayableMinor, a.PlatformRevenueMinor}
	for _, value := range values {
		if value < 0 {
			return ErrInvalidCommerceAllocation
		}
	}
	platformFunding, platformOK := checkedCommerceSum(a.PlatformProductDiscountMinor, a.PlatformDeliverySubsidyMinor)
	externalFunding, externalOK := checkedCommerceSum(platformFunding, a.OtherFundingMinor)
	input, inputOK := checkedCommerceSum(a.CustomerPayableMinor, externalFunding)
	output, outputOK := checkedCommerceSum(a.VendorMerchandisePayableMinor, a.VendorDeliveryPayableMinor, a.ProviderDeliveryPayableMinor, a.PlatformRevenueMinor)
	if strings.TrimSpace(a.AllocationReference) == "" || len(strings.TrimSpace(currency)) != 3 || a.MaxAuthorizedMinor < a.CustomerPayableMinor ||
		!platformOK || !externalOK || !inputOK || !outputOK || input != output {
		return ErrInvalidCommerceAllocation
	}
	switch a.Scope {
	case CommerceAllocationVendor:
		if strings.TrimSpace(a.VendorOrderID) == "" || strings.TrimSpace(a.VendorID) == "" {
			return ErrInvalidCommerceAllocation
		}
	case CommerceAllocationPlatform:
		if a.VendorOrderID != "" || a.VendorID != "" || a.VendorDeliveryPayableMinor != 0 || a.ProviderDeliveryPayableMinor != 0 {
			return ErrInvalidCommerceAllocation
		}
	default:
		return ErrInvalidCommerceAllocation
	}
	if a.DeliveryResponsibility != DeliveryResponsibilityNone && a.DeliveryResponsibility != DeliveryResponsibilityPlatform && a.DeliveryResponsibility != DeliveryResponsibilityVendor {
		return ErrInvalidCommerceAllocation
	}
	if a.DeliveryResponsibility == DeliveryResponsibilityVendor && a.ProviderDeliveryPayableMinor != 0 {
		return ErrInvalidCommerceAllocation
	}
	if a.DeliveryResponsibility == DeliveryResponsibilityPlatform && a.VendorDeliveryPayableMinor != 0 {
		return ErrInvalidCommerceAllocation
	}
	return nil
}

func checkedCommerceSum(values ...int64) (int64, bool) {
	var total int64
	for _, value := range values {
		if value < 0 || total > math.MaxInt64-value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func CommerceAllocationFingerprint(allocations []CommerceAllocation) (string, error) {
	copyOf := append([]CommerceAllocation(nil), allocations...)
	sort.Slice(copyOf, func(i, j int) bool { return copyOf[i].AllocationReference < copyOf[j].AllocationReference })
	for i := range copyOf {
		copyOf[i].ID = ""
		copyOf[i].PaymentIntentID = ""
		copyOf[i].VendorReleasedMinor = 0
		copyOf[i].DeliveryReleasedMinor = 0
		copyOf[i].RefundedMinor = 0
		copyOf[i].RefundCustomerMinor = 0
		copyOf[i].RefundPlatformFundingMinor = 0
		copyOf[i].RefundOtherFundingMinor = 0
		copyOf[i].RefundEscrowRecoveryMinor = 0
		copyOf[i].RefundVendorRecoveryMinor = 0
		copyOf[i].RefundProviderRecoveryMinor = 0
		copyOf[i].RefundPlatformRecoveryMinor = 0
		copyOf[i].RefundPlatformLossFundingMinor = 0
		copyOf[i].RefundVendorMerchandiseReductionMinor = 0
		copyOf[i].RefundVendorDeliveryReductionMinor = 0
		copyOf[i].RefundProviderDeliveryReductionMinor = 0
		copyOf[i].RefundPlatformRevenueReductionMinor = 0
		copyOf[i].RefundAuthorizationReleaseMinor = 0
		copyOf[i].Status = ""
		copyOf[i].Version = 0
		copyOf[i].CreatedAt = time.Time{}
		copyOf[i].UpdatedAt = time.Time{}
	}
	raw, err := json.Marshal(copyOf)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

type CommerceFinancialOperation struct {
	ID                 string          `json:"id"`
	PaymentIntentID    string          `json:"payment_intent_id"`
	AllocationID       string          `json:"allocation_id,omitempty"`
	OperationType      string          `json:"operation_type"`
	SourceReference    string          `json:"source_reference"`
	ItemReference      string          `json:"item_reference,omitempty"`
	IdempotencyKey     string          `json:"idempotency_key"`
	RequestFingerprint string          `json:"-"`
	Status             string          `json:"status"`
	AmountMinor        int64           `json:"amount_minor"`
	LedgerReference    string          `json:"ledger_reference,omitempty"`
	Result             json.RawMessage `json:"result"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}
