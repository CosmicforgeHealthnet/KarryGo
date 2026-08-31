package wallethttp

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type commerceIntentRequest struct {
	ParentCommerceReference string                            `json:"parent_commerce_reference"`
	CustomerID              string                            `json:"customer_id"`
	CustomerEmail           string                            `json:"customer_email"`
	QuoteReference          string                            `json:"quote_reference"`
	QuoteVersion            int64                             `json:"quote_version"`
	QuoteFingerprint        string                            `json:"quote_fingerprint"`
	FinalAuthorizedMinor    int64                             `json:"final_authorized_minor"`
	MaxAuthorizedMinor      int64                             `json:"max_authorized_minor"`
	AllocationFingerprint   string                            `json:"allocation_fingerprint"`
	Currency                string                            `json:"currency"`
	PaymentMethod           string                            `json:"payment_method"`
	IdempotencyKey          string                            `json:"idempotency_key"`
	Allocations             []walletmodels.CommerceAllocation `json:"allocations"`
}

type commerceHoldRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type vendorReleaseRequest struct {
	CompletionReference         string `json:"completion_reference"`
	ExpectedSnapshotFingerprint string `json:"expected_snapshot_fingerprint"`
	IdempotencyKey              string `json:"idempotency_key"`
}

type deliveryReleaseRequest struct {
	ProviderType                string `json:"provider_type"`
	ProviderID                  string `json:"provider_id"`
	CompletionReference         string `json:"completion_reference"`
	ExpectedSnapshotFingerprint string `json:"expected_snapshot_fingerprint"`
	IdempotencyKey              string `json:"idempotency_key"`
}

type allocationRefundRequest struct {
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

type vendorWithdrawalRequest struct {
	BankAccountID   string `json:"bank_account_id"`
	AmountKobo      int64  `json:"amount_kobo"`
	Currency        string `json:"currency"`
	IdempotencyKey  string `json:"idempotency_key"`
	StepUpReference string `json:"step_up_reference"`
}

func callerName(c *gin.Context) string {
	raw, _ := c.Get("service_name")
	value, _ := raw.(string)
	return value
}

func requireCaller(c *gin.Context, allowed ...string) bool {
	caller := callerName(c)
	for _, value := range allowed {
		if caller == value {
			return true
		}
	}
	httpx.Abort(c, apperrors.Forbidden("Caller is not allowed to perform this financial operation.", nil))
	return false
}

func deliveryReleaseAuthority(caller, requestedProviderType string) (providerType string, trustedServiceCompletion bool) {
	switch caller {
	case "dispatch-delivery-service", "driver-dispatch-delivery-service":
		return "dispatch_provider", true
	case "driver-hauling-service":
		return "truck_provider", true
	default:
		return requestedProviderType, false
	}
}

func (h *Handler) CreateCommerceIntent(c *gin.Context) {
	if !requireCaller(c, "vendor-service") {
		return
	}
	var request commerceIntentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	value, err := h.service.CreateCommerceIntent(c, walletusecases.CreateCommerceIntentInput{ParentCommerceReference: request.ParentCommerceReference, CustomerID: request.CustomerID, CustomerEmail: request.CustomerEmail, QuoteReference: request.QuoteReference, QuoteVersion: request.QuoteVersion, QuoteFingerprint: request.QuoteFingerprint, FinalAuthorizedMinor: request.FinalAuthorizedMinor, MaxAuthorizedMinor: request.MaxAuthorizedMinor, AllocationFingerprint: request.AllocationFingerprint, Currency: request.Currency, PaymentMethod: request.PaymentMethod, IdempotencyKey: request.IdempotencyKey, Allocations: request.Allocations})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusCreated, value)
}

func (h *Handler) GetCommerceIntent(c *gin.Context) {
	if !requireCaller(c, "vendor-service") {
		return
	}
	value, err := h.service.CommerceIntent(c, c.Param("parentReference"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, value)
}

func (h *Handler) HoldCommerceIntent(c *gin.Context) {
	if !requireCaller(c, "vendor-service") {
		return
	}
	var request commerceHoldRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	value, err := h.service.HoldCommerceIntent(c, c.Param("parentReference"), request.IdempotencyKey)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, value)
}

func (h *Handler) ReleaseVendorPayable(c *gin.Context) {
	if !requireCaller(c, "vendor-service") {
		return
	}
	var request vendorReleaseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	value, err := h.service.ReleaseVendorPayable(c, walletrepositories.ReleaseVendorPayableInput{VendorOrderID: c.Param("vendorOrderId"), CompletionReference: request.CompletionReference, ExpectedSnapshotFingerprint: request.ExpectedSnapshotFingerprint, IdempotencyKey: request.IdempotencyKey})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, value)
}

func (h *Handler) ReleaseCommerceDelivery(c *gin.Context) {
	if !requireCaller(c, "vendor-service", "dispatch-delivery-service", "driver-dispatch-delivery-service", "driver-hauling-service") {
		return
	}
	var request deliveryReleaseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	providerType, trustedServiceCompletion := deliveryReleaseAuthority(callerName(c), request.ProviderType)
	request.ProviderType = providerType
	value, err := h.service.ReleaseCommerceDelivery(c, walletrepositories.ReleaseCommerceDeliveryInput{VendorOrderID: c.Param("vendorOrderId"), ProviderType: request.ProviderType, ProviderID: request.ProviderID, CompletionReference: request.CompletionReference, ExpectedSnapshotFingerprint: request.ExpectedSnapshotFingerprint, IdempotencyKey: request.IdempotencyKey, TrustedServiceCompletion: trustedServiceCompletion})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, value)
}

func (h *Handler) RefundCommerceAllocation(c *gin.Context) {
	if !requireCaller(c, "vendor-service") {
		return
	}
	var request allocationRefundRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	value, err := h.service.RefundCommerceAllocation(c, walletrepositories.RefundCommerceAllocationInput{VendorOrderID: c.Param("vendorOrderId"), ItemReference: request.ItemReference, CustomerRefundMinor: request.CustomerRefundMinor, PlatformFundingReversalMinor: request.PlatformFundingReversalMinor, OtherFundingReversalMinor: request.OtherFundingReversalMinor, EscrowRecoveryMinor: request.EscrowRecoveryMinor, VendorRecoveryMinor: request.VendorRecoveryMinor, ProviderRecoveryMinor: request.ProviderRecoveryMinor, PlatformRecoveryMinor: request.PlatformRecoveryMinor, PlatformLossFundingMinor: request.PlatformLossFundingMinor, VendorMerchandiseReductionMinor: request.VendorMerchandiseReductionMinor, VendorDeliveryReductionMinor: request.VendorDeliveryReductionMinor, ProviderDeliveryReductionMinor: request.ProviderDeliveryReductionMinor, PlatformRevenueReductionMinor: request.PlatformRevenueReductionMinor, AuthorizationReleaseMinor: request.AuthorizationReleaseMinor, ExpectedSnapshotFingerprint: request.ExpectedSnapshotFingerprint, IdempotencyKey: request.IdempotencyKey})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, value)
}

func (h *Handler) GetCommerceOperation(c *gin.Context) {
	if !requireCaller(c, "vendor-service", "dispatch-delivery-service", "driver-dispatch-delivery-service", "driver-hauling-service") {
		return
	}
	value, err := h.service.CommerceOperation(c, c.Param("operationType"), c.Param("idempotencyKey"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, value)
}

func (h *Handler) VendorPayableSummary(c *gin.Context) {
	if !requireVendorCaller(c) {
		return
	}
	value, err := h.service.VendorPayableSummary(c, c.Param("businessId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, value)
}
func (h *Handler) VendorFinanceTransactions(c *gin.Context) {
	if !requireVendorCaller(c) {
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	value, err := h.service.VendorTransactions(c, c.Param("businessId"), limit)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, value)
}
func (h *Handler) VendorFinanceBankAccounts(c *gin.Context) {
	if !requireVendorCaller(c) {
		return
	}
	values, err := h.service.ListBankAccounts(c, vendorBankProviderType, c.Param("businessId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	views := make([]vendorBankView, 0, len(values))
	for _, value := range values {
		views = append(views, bankView(value))
	}
	respondOK(c, views)
}
func (h *Handler) RequestVendorWithdrawal(c *gin.Context) {
	if !requireVendorCaller(c) {
		return
	}
	var request vendorWithdrawalRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	value, err := h.service.RequestVendorWithdrawal(c, walletusecases.VendorWithdrawalInput{VendorID: c.Param("businessId"), BankAccountID: request.BankAccountID, AmountKobo: request.AmountKobo, Currency: request.Currency, IdempotencyKey: request.IdempotencyKey, StepUpReference: request.StepUpReference})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, http.StatusAccepted, value)
}
func (h *Handler) VendorWithdrawals(c *gin.Context) {
	if !requireVendorCaller(c) {
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	values, err := h.service.VendorWithdrawals(c, c.Param("businessId"), limit)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, values)
}
func (h *Handler) VendorWithdrawal(c *gin.Context) {
	if !requireVendorCaller(c) {
		return
	}
	value, err := h.service.VendorWithdrawal(c, c.Param("businessId"), strings.TrimSpace(c.Param("reference")))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, value)
}
