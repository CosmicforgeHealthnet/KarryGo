package walletclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	MethodWallet   = "wallet"
	MethodPaystack = "paystack"
)

type Client struct {
	BaseURL     string
	HTTPClient  *http.Client
	ServiceName string
	Secret      []byte
}

type PaymentIntentRequest struct {
	SourceService   string                 `json:"source_service"`
	SourceReference string                 `json:"source_reference"`
	CustomerID      string                 `json:"customer_id"`
	CustomerEmail   string                 `json:"customer_email"`
	ProviderID      string                 `json:"provider_id"`
	ProviderType    string                 `json:"provider_type"`
	AmountKobo      int64                  `json:"amount_kobo"`
	Currency        string                 `json:"currency"`
	PaymentMethod   string                 `json:"payment_method"`
	IdempotencyKey  string                 `json:"idempotency_key"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type PaymentIntent struct {
	ID               string                 `json:"id"`
	Reference        string                 `json:"reference"`
	SourceService    string                 `json:"source_service"`
	SourceReference  string                 `json:"source_reference"`
	CustomerID       string                 `json:"customer_id"`
	CustomerEmail    string                 `json:"customer_email,omitempty"`
	ProviderID       string                 `json:"provider_id,omitempty"`
	ProviderType     string                 `json:"provider_type,omitempty"`
	AmountKobo       int64                  `json:"amount_kobo"`
	PlatformFeeKobo  int64                  `json:"platform_fee_kobo"`
	Currency         string                 `json:"currency"`
	PaymentMethod    string                 `json:"payment_method"`
	Status           string                 `json:"status"`
	PaystackRef      string                 `json:"paystack_reference,omitempty"`
	AuthorizationURL string                 `json:"authorization_url,omitempty"`
	AccessCode       string                 `json:"access_code,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

type WalletPayRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type RefundRequest struct {
	PaymentReference string `json:"payment_reference"`
	SourceService    string `json:"source_service,omitempty"`
	SourceReference  string `json:"source_reference,omitempty"`
	AmountKobo       int64  `json:"amount_kobo"`
	Currency         string `json:"currency"`
	Reason           string `json:"reason"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type Refund struct {
	ID                string `json:"id"`
	Reference         string `json:"reference"`
	PaymentIntentID   string `json:"payment_intent_id"`
	AmountKobo        int64  `json:"amount_kobo"`
	Currency          string `json:"currency"`
	Reason            string `json:"reason,omitempty"`
	Status            string `json:"status"`
	PaystackRefundRef string `json:"paystack_refund_reference,omitempty"`
}

type DeliveryFundingRequest struct {
	SourceReference            string `json:"source_reference"`
	VendorOrderID              string `json:"vendor_order_id"`
	SponsoringVendorID         string `json:"sponsoring_vendor_id"`
	ProviderType               string `json:"provider_type"`
	ProviderID                 string `json:"provider_id"`
	Currency                   string `json:"currency"`
	AuthoritativeDeliveryMinor int64  `json:"authoritative_delivery_minor"`
	CustomerContributionMinor  int64  `json:"customer_contribution_minor"`
	VendorSubsidyMinor         int64  `json:"vendor_subsidy_minor"`
	PlatformSubsidyMinor       int64  `json:"platform_subsidy_minor"`
	PlatformApprovalRef        string `json:"platform_approval_ref,omitempty"`
	CustomerPaymentReference   string `json:"customer_payment_reference,omitempty"`
	IdempotencyKey             string `json:"idempotency_key"`
}

type DeliveryFunding struct {
	ID                         string `json:"id"`
	SourceReference            string `json:"source_reference"`
	VendorOrderID              string `json:"vendor_order_id"`
	SponsoringVendorID         string `json:"sponsoring_vendor_id"`
	AuthoritativeDeliveryMinor int64  `json:"authoritative_delivery_minor"`
	CustomerContributionMinor  int64  `json:"customer_contribution_minor"`
	VendorSubsidyMinor         int64  `json:"vendor_subsidy_minor"`
	PlatformSubsidyMinor       int64  `json:"platform_subsidy_minor"`
	Status                     string `json:"status"`
}

type DeliveryResponsibilityTransitionRequest struct {
	OperationRef               string `json:"operation_ref"`
	IdempotencyKey             string `json:"idempotency_key"`
	ReasonCode                 string `json:"reason_code"`
	PlatformExecutionCostMinor int64  `json:"platform_execution_cost_minor"`
}

type DeliveryResponsibilityTransition struct {
	ID                                string `json:"id"`
	AllocationID                      string `json:"allocation_id"`
	VendorOrderID                     string `json:"vendor_order_id"`
	OperationRef                      string `json:"operation_ref"`
	FromResponsibility                string `json:"from_responsibility"`
	ToResponsibility                  string `json:"to_responsibility"`
	VendorDeliveryPayableBeforeMinor  int64  `json:"vendor_delivery_payable_before_minor"`
	ProviderDeliveryPayableAfterMinor int64  `json:"provider_delivery_payable_after_minor"`
	PlatformExecutionCostMinor        int64  `json:"platform_execution_cost_minor"`
	FallbackPlatformFundingMinor      int64  `json:"fallback_platform_funding_minor"`
	CustomerContributionMinor         int64  `json:"customer_contribution_minor"`
	VendorSubsidyMinor                int64  `json:"vendor_subsidy_minor"`
	PlatformSubsidyMinor              int64  `json:"platform_subsidy_minor"`
	Status                            string `json:"status"`
}

type CommerceAllocation struct {
	AllocationReference                   string                 `json:"allocation_reference"`
	Scope                                 string                 `json:"scope"`
	VendorOrderID                         string                 `json:"vendor_order_id,omitempty"`
	VendorID                              string                 `json:"vendor_id,omitempty"`
	BranchID                              string                 `json:"branch_id,omitempty"`
	DeliveryResponsibility                string                 `json:"delivery_responsibility"`
	Currency                              string                 `json:"currency"`
	CustomerPayableMinor                  int64                  `json:"customer_payable_minor"`
	MaxAuthorizedMinor                    int64                  `json:"max_authorized_minor"`
	MerchandiseGrossMinor                 int64                  `json:"merchandise_gross_minor"`
	VendorProductDiscountMinor            int64                  `json:"vendor_product_discount_minor"`
	PlatformProductDiscountMinor          int64                  `json:"platform_product_discount_minor"`
	PlatformCommissionMinor               int64                  `json:"platform_commission_minor"`
	DeliveryCustomerMinor                 int64                  `json:"delivery_customer_minor"`
	VendorDeliverySubsidyMinor            int64                  `json:"vendor_delivery_subsidy_minor"`
	PlatformDeliverySubsidyMinor          int64                  `json:"platform_delivery_subsidy_minor"`
	OtherFundingMinor                     int64                  `json:"other_funding_minor"`
	VendorMerchandisePayableMinor         int64                  `json:"vendor_merchandise_payable_minor"`
	VendorDeliveryPayableMinor            int64                  `json:"vendor_delivery_payable_minor"`
	ProviderDeliveryPayableMinor          int64                  `json:"provider_delivery_payable_minor"`
	PlatformRevenueMinor                  int64                  `json:"platform_revenue_minor"`
	VendorReleasedMinor                   int64                  `json:"vendor_released_minor,omitempty"`
	DeliveryReleasedMinor                 int64                  `json:"delivery_released_minor,omitempty"`
	RefundedMinor                         int64                  `json:"refunded_minor,omitempty"`
	RefundCustomerMinor                   int64                  `json:"refund_customer_minor,omitempty"`
	RefundPlatformFundingMinor            int64                  `json:"refund_platform_funding_minor,omitempty"`
	RefundOtherFundingMinor               int64                  `json:"refund_other_funding_minor,omitempty"`
	RefundEscrowRecoveryMinor             int64                  `json:"refund_escrow_recovery_minor,omitempty"`
	RefundVendorRecoveryMinor             int64                  `json:"refund_vendor_recovery_minor,omitempty"`
	RefundProviderRecoveryMinor           int64                  `json:"refund_provider_recovery_minor,omitempty"`
	RefundPlatformRecoveryMinor           int64                  `json:"refund_platform_recovery_minor,omitempty"`
	RefundPlatformLossFundingMinor        int64                  `json:"refund_platform_loss_funding_minor,omitempty"`
	RefundVendorMerchandiseReductionMinor int64                  `json:"refund_vendor_merchandise_reduction_minor,omitempty"`
	RefundVendorDeliveryReductionMinor    int64                  `json:"refund_vendor_delivery_reduction_minor,omitempty"`
	RefundProviderDeliveryReductionMinor  int64                  `json:"refund_provider_delivery_reduction_minor,omitempty"`
	RefundPlatformRevenueReductionMinor   int64                  `json:"refund_platform_revenue_reduction_minor,omitempty"`
	RefundAuthorizationReleaseMinor       int64                  `json:"refund_authorization_release_minor,omitempty"`
	Snapshot                              map[string]interface{} `json:"snapshot,omitempty"`
	SnapshotFingerprint                   string                 `json:"snapshot_fingerprint"`
	Status                                string                 `json:"status,omitempty"`
}

type CommerceIntentRequest struct {
	ParentCommerceReference string               `json:"parent_commerce_reference"`
	CustomerID              string               `json:"customer_id"`
	CustomerEmail           string               `json:"customer_email"`
	QuoteReference          string               `json:"quote_reference"`
	QuoteVersion            int64                `json:"quote_version"`
	QuoteFingerprint        string               `json:"quote_fingerprint"`
	FinalAuthorizedMinor    int64                `json:"final_authorized_minor"`
	MaxAuthorizedMinor      int64                `json:"max_authorized_minor"`
	AllocationFingerprint   string               `json:"allocation_fingerprint"`
	Currency                string               `json:"currency"`
	PaymentMethod           string               `json:"payment_method"`
	IdempotencyKey          string               `json:"idempotency_key"`
	Allocations             []CommerceAllocation `json:"allocations"`
}

type CommerceIntent struct {
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
	Allocations             []CommerceAllocation `json:"allocations"`
}

type CommerceOperation struct {
	ID              string `json:"id"`
	OperationType   string `json:"operation_type"`
	SourceReference string `json:"source_reference"`
	ItemReference   string `json:"item_reference,omitempty"`
	IdempotencyKey  string `json:"idempotency_key"`
	Status          string `json:"status"`
	AmountMinor     int64  `json:"amount_minor"`
	LedgerReference string `json:"ledger_reference,omitempty"`
}

type VendorReleaseRequest struct {
	CompletionReference         string `json:"completion_reference"`
	ExpectedSnapshotFingerprint string `json:"expected_snapshot_fingerprint"`
	IdempotencyKey              string `json:"idempotency_key"`
}

type DeliveryReleaseRequest struct {
	ProviderType                string `json:"provider_type"`
	ProviderID                  string `json:"provider_id"`
	CompletionReference         string `json:"completion_reference"`
	ExpectedSnapshotFingerprint string `json:"expected_snapshot_fingerprint"`
	IdempotencyKey              string `json:"idempotency_key"`
}

type CommerceRefundRequest struct {
	ItemReference                   string `json:"item_reference"`
	CustomerRefundMinor             int64  `json:"customer_refund_minor"`
	PlatformFundingReversalMinor    int64  `json:"platform_funding_reversal_minor"`
	OtherFundingReversalMinor       int64  `json:"other_funding_reversal_minor"`
	EscrowRecoveryMinor             int64  `json:"escrow_recovery_minor"`
	VendorRecoveryMinor             int64  `json:"vendor_recovery_minor"`
	ProviderRecoveryMinor           int64  `json:"provider_recovery_minor"`
	PlatformRecoveryMinor           int64  `json:"platform_recovery_minor"`
	PlatformLossFundingMinor        int64  `json:"platform_loss_funding_minor"`
	VendorMerchandiseReductionMinor int64  `json:"vendor_merchandise_reduction_minor"`
	VendorDeliveryReductionMinor    int64  `json:"vendor_delivery_reduction_minor"`
	ProviderDeliveryReductionMinor  int64  `json:"provider_delivery_reduction_minor"`
	PlatformRevenueReductionMinor   int64  `json:"platform_revenue_reduction_minor"`
	AuthorizationReleaseMinor       int64  `json:"authorization_release_minor"`
	ExpectedSnapshotFingerprint     string `json:"expected_snapshot_fingerprint"`
	IdempotencyKey                  string `json:"idempotency_key"`
}

type VendorPayableSummary struct {
	OwnerID       string `json:"owner_id"`
	Currency      string `json:"currency"`
	AvailableKobo int64  `json:"available_kobo"`
	PendingKobo   int64  `json:"pending_kobo"`
}

type VendorTransaction struct {
	Reference       string    `json:"reference"`
	TransactionType string    `json:"transaction_type"`
	Side            string    `json:"side"`
	AmountKobo      int64     `json:"amount_kobo"`
	Currency        string    `json:"currency"`
	Memo            string    `json:"memo"`
	CreatedAt       time.Time `json:"created_at"`
}

type VendorBankAccount struct {
	ExternalRef         string `json:"external_ref"`
	BankCode            string `json:"bank_code"`
	MaskedAccount       string `json:"masked_account"`
	ResolvedAccountName string `json:"resolved_account_name"`
	Status              string `json:"status"`
}

type VendorWithdrawalRequest struct {
	BankAccountID   string `json:"bank_account_id"`
	AmountKobo      int64  `json:"amount_kobo"`
	Currency        string `json:"currency"`
	IdempotencyKey  string `json:"idempotency_key"`
	StepUpReference string `json:"step_up_reference"`
}

type VendorWithdrawal struct {
	ID            string `json:"id"`
	Reference     string `json:"reference"`
	BankAccountID string `json:"bank_account_id"`
	AmountKobo    int64  `json:"amount_kobo"`
	Currency      string `json:"currency"`
	Status        string `json:"status"`
}

func (c Client) CreateCommerceIntent(ctx context.Context, request CommerceIntentRequest) (CommerceIntent, error) {
	var response CommerceIntent
	err := c.post(ctx, "/api/v1/payment-wallet/internal/commerce/payment-intents", request, &response)
	return response, err
}

func (c Client) GetCommerceIntent(ctx context.Context, parentReference string) (CommerceIntent, error) {
	var response CommerceIntent
	err := c.get(ctx, "/api/v1/payment-wallet/internal/commerce/payment-intents/"+url.PathEscape(parentReference), &response)
	return response, err
}

func (c Client) HoldCommerceIntent(ctx context.Context, parentReference, idempotencyKey string) (CommerceIntent, error) {
	var response CommerceIntent
	err := c.post(ctx, "/api/v1/payment-wallet/internal/commerce/payment-intents/"+url.PathEscape(parentReference)+"/hold", map[string]string{"idempotency_key": idempotencyKey}, &response)
	return response, err
}

func (c Client) ReleaseVendorPayable(ctx context.Context, vendorOrderID string, request VendorReleaseRequest) (CommerceOperation, error) {
	var response CommerceOperation
	err := c.post(ctx, "/api/v1/payment-wallet/internal/commerce/allocations/"+url.PathEscape(vendorOrderID)+"/vendor-release", request, &response)
	return response, err
}

func (c Client) ReleaseCommerceDelivery(ctx context.Context, vendorOrderID string, request DeliveryReleaseRequest) (CommerceOperation, error) {
	var response CommerceOperation
	err := c.post(ctx, "/api/v1/payment-wallet/internal/commerce/allocations/"+url.PathEscape(vendorOrderID)+"/delivery-release", request, &response)
	return response, err
}

func (c Client) RefundCommerceAllocation(ctx context.Context, vendorOrderID string, request CommerceRefundRequest) (CommerceOperation, error) {
	var response CommerceOperation
	err := c.post(ctx, "/api/v1/payment-wallet/internal/commerce/allocations/"+url.PathEscape(vendorOrderID)+"/refunds", request, &response)
	return response, err
}

func (c Client) GetCommerceOperation(ctx context.Context, operationType, idempotencyKey string) (CommerceOperation, error) {
	var response CommerceOperation
	err := c.get(ctx, "/api/v1/payment-wallet/internal/commerce/operations/"+url.PathEscape(operationType)+"/"+url.PathEscape(idempotencyKey), &response)
	return response, err
}

func (c Client) VendorSummary(ctx context.Context, businessID string) (VendorPayableSummary, error) {
	var response VendorPayableSummary
	err := c.get(ctx, "/api/v1/payment-wallet/internal/vendor-finance/"+url.PathEscape(businessID)+"/summary", &response)
	return response, err
}

func (c Client) VendorBankAccounts(ctx context.Context, businessID string) ([]VendorBankAccount, error) {
	var response []VendorBankAccount
	err := c.get(ctx, "/api/v1/payment-wallet/internal/vendor-finance/"+url.PathEscape(businessID)+"/bank-accounts", &response)
	return response, err
}

func (c Client) VendorTransactions(ctx context.Context, businessID string, limit int) ([]VendorTransaction, error) {
	var response []VendorTransaction
	path := "/api/v1/payment-wallet/internal/vendor-finance/" + url.PathEscape(businessID) + "/transactions"
	if limit > 0 {
		path += "?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit))
	}
	err := c.get(ctx, path, &response)
	return response, err
}

func (c Client) RequestVendorWithdrawal(ctx context.Context, businessID string, request VendorWithdrawalRequest) (VendorWithdrawal, error) {
	var response VendorWithdrawal
	err := c.post(ctx, "/api/v1/payment-wallet/internal/vendor-finance/"+url.PathEscape(businessID)+"/withdrawals", request, &response)
	return response, err
}

func (c Client) VendorWithdrawals(ctx context.Context, businessID string) ([]VendorWithdrawal, error) {
	var response []VendorWithdrawal
	err := c.get(ctx, "/api/v1/payment-wallet/internal/vendor-finance/"+url.PathEscape(businessID)+"/withdrawals", &response)
	return response, err
}

func (c Client) VendorWithdrawal(ctx context.Context, businessID, reference string) (VendorWithdrawal, error) {
	var response VendorWithdrawal
	err := c.get(ctx, "/api/v1/payment-wallet/internal/vendor-finance/"+url.PathEscape(businessID)+"/withdrawals/"+url.PathEscape(reference), &response)
	return response, err
}

func (c Client) ApplyDeliveryFunding(ctx context.Context, request DeliveryFundingRequest) (DeliveryFunding, error) {
	var response DeliveryFunding
	err := c.post(ctx, "/api/v1/payment-wallet/internal/commerce/delivery-funding", request, &response)
	return response, err
}
func (c Client) GetDeliveryFunding(ctx context.Context, sourceReference string) (DeliveryFunding, error) {
	var response DeliveryFunding
	err := c.get(ctx, "/api/v1/payment-wallet/internal/commerce/delivery-funding/"+url.PathEscape(sourceReference), &response)
	return response, err
}

func (c Client) TransitionDeliveryResponsibility(ctx context.Context, vendorOrderID string, request DeliveryResponsibilityTransitionRequest) (DeliveryResponsibilityTransition, error) {
	var response DeliveryResponsibilityTransition
	err := c.post(ctx, "/api/v1/payment-wallet/internal/commerce/allocations/"+url.PathEscape(vendorOrderID)+"/delivery-responsibility", request, &response)
	return response, err
}

func (c Client) GetDeliveryResponsibilityTransition(ctx context.Context, operationRef string) (DeliveryResponsibilityTransition, error) {
	var response DeliveryResponsibilityTransition
	err := c.get(ctx, "/api/v1/payment-wallet/internal/commerce/delivery-responsibility-transitions/"+url.PathEscape(operationRef), &response)
	return response, err
}

func (c Client) CreatePaymentIntent(ctx context.Context, request PaymentIntentRequest) (PaymentIntent, error) {
	var response PaymentIntent
	err := c.post(ctx, "/api/v1/payment-wallet/internal/payment-intents", request, &response)
	return response, err
}

func (c Client) PayFromWallet(ctx context.Context, paymentIntentID string, idempotencyKey string) (PaymentIntent, error) {
	var response PaymentIntent
	err := c.post(ctx, "/api/v1/payment-wallet/internal/payment-intents/"+paymentIntentID+"/pay-from-wallet", WalletPayRequest{IdempotencyKey: idempotencyKey}, &response)
	return response, err
}

func (c Client) CompleteJob(ctx context.Context, sourceService string, sourceReference string) (PaymentIntent, error) {
	return c.CompleteJobAt(ctx, sourceService, sourceReference, time.Time{})
}

// CompleteJobRequest is the optional body of the job-completion call.
type CompleteJobRequest struct {
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// CompleteJobAt settles a held job payment and reports when the job itself
// finished. The referral programme's time rules (30-day activity, 90-day
// earning window) run off this time rather than the settlement time. A zero
// completedAt behaves exactly like CompleteJob.
func (c Client) CompleteJobAt(ctx context.Context, sourceService string, sourceReference string, completedAt time.Time) (PaymentIntent, error) {
	var response PaymentIntent
	path := "/api/v1/payment-wallet/internal/jobs/" + url.PathEscape(sourceService) + "/" + url.PathEscape(sourceReference) + "/complete"
	request := CompleteJobRequest{}
	if !completedAt.IsZero() {
		at := completedAt.UTC()
		request.CompletedAt = &at
	}
	err := c.post(ctx, path, request, &response)
	return response, err
}

func (c Client) RequestRefund(ctx context.Context, request RefundRequest) (Refund, error) {
	var response Refund
	err := c.post(ctx, "/api/v1/payment-wallet/internal/refunds", request, &response)
	return response, err
}

type AssignProviderRequest struct {
	ProviderType string `json:"provider_type"`
	ProviderID   string `json:"provider_id"`
}

// AssignProvider attaches the matched provider to an existing payment intent
// (looked up by reference) so a later CompleteJob can settle escrow to the
// provider. Idempotent for the same provider.
func (c Client) AssignProvider(ctx context.Context, reference string, providerType string, providerID string) (PaymentIntent, error) {
	var response PaymentIntent
	path := "/api/v1/payment-wallet/internal/payments/" + url.PathEscape(reference) + "/provider"
	err := c.put(ctx, path, AssignProviderRequest{ProviderType: providerType, ProviderID: providerID}, &response)
	return response, err
}

func (c Client) GetPayment(ctx context.Context, reference string) (PaymentIntent, error) {
	var response PaymentIntent
	err := c.get(ctx, "/api/v1/payment-wallet/internal/payments/"+url.PathEscape(reference), &response)
	return response, err
}

func (c Client) get(ctx context.Context, path string, data interface{}) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return err
	}
	return c.do(request, nil, data)
}

func (c Client) post(ctx context.Context, path string, payload interface{}, data interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request, body, data)
}

func (c Client) put(ctx context.Context, path string, payload interface{}, data interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.url(path), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request, body, data)
}

func (c Client) do(request *http.Request, body []byte, data interface{}) error {
	if body == nil {
		body = []byte{}
	}
	if err := serviceauth.SignRequest(request, c.ServiceName, c.Secret, body, time.Now()); err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return err
	}
	if response.StatusCode >= http.StatusBadRequest || !envelope.Success {
		if envelope.Error.Message == "" {
			envelope.Error.Message = "Wallet request failed."
		}
		return &RequestError{Status: response.StatusCode, Code: envelope.Error.Code,
			Message: envelope.Error.Message}
	}
	if data != nil && len(envelope.Data) > 0 {
		return json.Unmarshal(envelope.Data, data)
	}
	return nil
}

func (c Client) url(path string) string {
	baseURL := strings.TrimRight(c.BaseURL, "/")
	const apiBasePath = "/api/v1/payment-wallet"
	if strings.HasSuffix(baseURL, apiBasePath) && strings.HasPrefix(path, apiBasePath) {
		path = strings.TrimPrefix(path, apiBasePath)
	}
	return baseURL + path
}

// ReferralPostingRequest is a referral reward credit or reversal. For
// customers OwnerID is the customer id; for drivers it is the provider id and
// ProviderType names the driver's ledger (truck_provider, dispatch, taxi).
// Balance is "earnings" (withdrawable/spendable) or "promotional"
// (spend-only).
type ReferralPostingRequest struct {
	OwnerType      string `json:"owner_type"`
	OwnerID        string `json:"owner_id"`
	ProviderType   string `json:"provider_type,omitempty"`
	Balance        string `json:"balance"`
	AmountKobo     int64  `json:"amount_kobo"`
	Currency       string `json:"currency"`
	IdempotencyKey string `json:"idempotency_key"`
	Reference      string `json:"reference"`
	Reason         string `json:"reason"`
}

// ReferralPosting is the ledger outcome. Duplicate means the posting already
// existed for this idempotency key and nothing new was written.
type ReferralPosting struct {
	TransactionReference string `json:"transaction_reference"`
	Duplicate            bool   `json:"duplicate"`
	AccountType          string `json:"account_type"`
	BalanceKobo          int64  `json:"balance_kobo"`
}

// ReferralTotals is one owner's referral postings and balances.
type ReferralTotals struct {
	OwnerType              string `json:"owner_type"`
	OwnerID                string `json:"owner_id"`
	CreditedKobo           int64  `json:"credited_kobo"`
	ReversedKobo           int64  `json:"reversed_kobo"`
	EarningsBalanceKobo    int64  `json:"earnings_balance_kobo"`
	PromotionalBalanceKobo int64  `json:"promotional_balance_kobo"`
}

// CreditReferral pays a referral reward. Safe to retry with the same
// idempotency key.
func (c Client) CreditReferral(ctx context.Context, request ReferralPostingRequest) (ReferralPosting, error) {
	var response ReferralPosting
	err := c.post(ctx, "/api/v1/payment-wallet/internal/referral/credits", request, &response)
	return response, err
}

// ReverseReferral takes a referral reward back; the balance may go negative.
// Safe to retry with the same idempotency key.
func (c Client) ReverseReferral(ctx context.Context, request ReferralPostingRequest) (ReferralPosting, error) {
	var response ReferralPosting
	err := c.post(ctx, "/api/v1/payment-wallet/internal/referral/reversals", request, &response)
	return response, err
}

// ReferralTotals reads one owner's referral postings for reconciliation.
func (c Client) ReferralTotals(ctx context.Context, ownerType, ownerID, providerType string) (ReferralTotals, error) {
	var response ReferralTotals
	query := url.Values{"owner_type": {ownerType}, "owner_id": {ownerID}}
	if providerType != "" {
		query.Set("provider_type", providerType)
	}
	err := c.get(ctx, "/api/v1/payment-wallet/internal/referral/totals?"+query.Encode(), &response)
	return response, err
}
