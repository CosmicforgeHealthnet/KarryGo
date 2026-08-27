package wallethttp

import (
	"io"
	nethttp "net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	service *walletusecases.WalletService
}

type deliveryFundingRequest struct {
	SourceReference            string `json:"source_reference" binding:"required"`
	VendorOrderID              string `json:"vendor_order_id" binding:"required"`
	SponsoringVendorID         string `json:"sponsoring_vendor_id" binding:"required"`
	ProviderType               string `json:"provider_type" binding:"required"`
	ProviderID                 string `json:"provider_id" binding:"required"`
	Currency                   string `json:"currency" binding:"required"`
	AuthoritativeDeliveryMinor int64  `json:"authoritative_delivery_minor"`
	CustomerContributionMinor  int64  `json:"customer_contribution_minor"`
	VendorSubsidyMinor         int64  `json:"vendor_subsidy_minor"`
	PlatformSubsidyMinor       int64  `json:"platform_subsidy_minor"`
	PlatformApprovalRef        string `json:"platform_approval_ref,omitempty"`
	CustomerPaymentReference   string `json:"customer_payment_reference,omitempty"`
	IdempotencyKey             string `json:"idempotency_key" binding:"required"`
}

func (h *Handler) ApplyDeliveryFunding(c *gin.Context) {
	var request deliveryFundingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	caller := c.GetString("service_name")
	if caller != "vendor-service" {
		httpx.Abort(c, apperrors.Forbidden("Caller may not allocate commerce delivery funding.", nil))
		return
	}
	result, err := h.service.ApplyDeliveryFunding(c, candidateDeliveryFunding(caller, request))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, result)
}
func candidateDeliveryFunding(caller string, r deliveryFundingRequest) walletusecases.DeliveryFundingInput {
	return walletusecases.DeliveryFundingInput{SourceService: caller, SourceReference: r.SourceReference, VendorOrderID: r.VendorOrderID, SponsoringVendorID: r.SponsoringVendorID, ProviderType: r.ProviderType, ProviderID: r.ProviderID, Currency: r.Currency, AuthoritativeDeliveryMinor: r.AuthoritativeDeliveryMinor, CustomerContributionMinor: r.CustomerContributionMinor, VendorSubsidyMinor: r.VendorSubsidyMinor, PlatformSubsidyMinor: r.PlatformSubsidyMinor, PlatformApprovalRef: r.PlatformApprovalRef, CustomerPaymentReference: r.CustomerPaymentReference, IdempotencyKey: r.IdempotencyKey}
}
func (h *Handler) GetDeliveryFunding(c *gin.Context) {
	caller := c.GetString("service_name")
	if caller != "vendor-service" {
		httpx.Abort(c, apperrors.Forbidden("Caller may not read commerce delivery funding.", nil))
		return
	}
	result, err := h.service.DeliveryFunding(c, caller, c.Param("sourceReference"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func NewHandler(service *walletusecases.WalletService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CustomerWallet(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	result, err := h.service.WalletSummary(c.Request.Context(), walletmodels.OwnerTypeCustomer, claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) CustomerTransactions(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	result, err := h.service.WalletTransactions(c.Request.Context(), walletmodels.OwnerTypeCustomer, claims.Subject, limit)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"transactions": result})
}

func (h *Handler) CreateTopUp(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var request topUpRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	// CustomerID is always the token subject. CustomerEmail comes from the body
	// because access-token claims carry no email; it is used only as the Paystack
	// receipt address, not for customer lookup or authorization.
	result, err := h.service.CreateTopUp(c.Request.Context(), walletusecases.TopUpInput{
		CustomerID:     claims.Subject,
		CustomerEmail:  request.CustomerEmail,
		AmountKobo:     request.AmountKobo,
		Currency:       request.Currency,
		IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, result)
}

func (h *Handler) VerifyTopUp(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	result, err := h.service.VerifyTopUp(c.Request.Context(), claims.Subject, c.Param("reference"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) ProviderEarnings(c *gin.Context) {
	providerType, providerID := providerIdentity(c)
	result, err := h.service.WalletSummary(c.Request.Context(), walletmodels.OwnerTypeProvider, providerType+":"+providerID)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) ListBankAccounts(c *gin.Context) {
	providerType, providerID := providerIdentity(c)
	result, err := h.service.ListBankAccounts(c.Request.Context(), providerType, providerID)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"bank_accounts": result})
}

func (h *Handler) ResolveBankAccount(c *gin.Context) {
	var request resolveBankAccountRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.ResolveBankAccount(c.Request.Context(), walletusecases.ResolveBankAccountInput{
		AccountNumber: request.AccountNumber,
		BankCode:      request.BankCode,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) RegisterBankAccount(c *gin.Context) {
	providerType, providerID := providerIdentity(c)
	var request registerBankAccountRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.RegisterBankAccount(c.Request.Context(), walletusecases.RegisterBankAccountInput{
		ProviderType:  providerType,
		ProviderID:    providerID,
		BankCode:      request.BankCode,
		BankName:      request.BankName,
		AccountNumber: request.AccountNumber,
		Currency:      request.Currency,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, result)
}

func (h *Handler) DeleteBankAccount(c *gin.Context) {
	providerType, providerID := providerIdentity(c)
	if err := h.service.DeleteBankAccount(c.Request.Context(), providerType, providerID, c.Param("id")); err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"deleted": true})
}

func (h *Handler) SetDefaultBankAccount(c *gin.Context) {
	providerType, providerID := providerIdentity(c)
	result, err := h.service.SetDefaultBankAccount(c.Request.Context(), providerType, providerID, c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) RequestWithdrawal(c *gin.Context) {
	providerType, providerID := providerIdentity(c)
	var request withdrawalRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.RequestWithdrawal(c.Request.Context(), walletusecases.RequestWithdrawalInput{
		ProviderType:   providerType,
		ProviderID:     providerID,
		BankAccountID:  request.BankAccountID,
		AmountKobo:     request.AmountKobo,
		Currency:       request.Currency,
		IdempotencyKey: request.IdempotencyKey,
		Pin:            request.Pin,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, result)
}

func (h *Handler) TransactionPinStatus(c *gin.Context) {
	providerType, providerID := providerIdentity(c)
	pinSet, err := h.service.TransactionPinStatus(c.Request.Context(), providerType, providerID)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"pin_set": pinSet})
}

func (h *Handler) SetTransactionPin(c *gin.Context) {
	providerType, providerID := providerIdentity(c)
	var request setPinRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if err := h.service.SetTransactionPin(c.Request.Context(), providerType, providerID, request.Pin); err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, gin.H{"pin_set": true})
}

func (h *Handler) ChangeTransactionPin(c *gin.Context) {
	providerType, providerID := providerIdentity(c)
	var request changePinRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if err := h.service.ChangeTransactionPin(c.Request.Context(), providerType, providerID, request.CurrentPin, request.NewPin); err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"pin_set": true})
}

func (h *Handler) CreatePaymentIntent(c *gin.Context) {
	var request createPaymentIntentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.CreatePaymentIntent(c.Request.Context(), walletusecases.CreatePaymentIntentInput{
		SourceService:   request.SourceService,
		SourceReference: request.SourceReference,
		CustomerID:      request.CustomerID,
		CustomerEmail:   request.CustomerEmail,
		ProviderID:      request.ProviderID,
		ProviderType:    request.ProviderType,
		AmountKobo:      request.AmountKobo,
		Currency:        request.Currency,
		PaymentMethod:   request.PaymentMethod,
		IdempotencyKey:  request.IdempotencyKey,
		Metadata:        request.Metadata,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, result)
}

func (h *Handler) PayFromWallet(c *gin.Context) {
	var request walletPayRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.PayFromWallet(c.Request.Context(), c.Param("id"), request.IdempotencyKey)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) CompleteJob(c *gin.Context) {
	result, err := h.service.CompleteJob(c.Request.Context(), c.Param("source_service"), c.Param("source_reference"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) AssignPaymentProvider(c *gin.Context) {
	var request assignProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.AssignPaymentProvider(c.Request.Context(), c.Param("reference"), request.ProviderType, request.ProviderID)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) RequestRefund(c *gin.Context) {
	var request refundRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.RequestRefund(c.Request.Context(), walletusecases.RefundInput{
		PaymentReference: request.PaymentReference,
		SourceService:    request.SourceService,
		SourceReference:  request.SourceReference,
		AmountKobo:       request.AmountKobo,
		Currency:         request.Currency,
		Reason:           request.Reason,
		IdempotencyKey:   request.IdempotencyKey,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, result)
}

func (h *Handler) GetInternalPayment(c *gin.Context) {
	result, err := h.service.PaymentByReference(c.Request.Context(), c.Param("reference"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) PaystackWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if err := h.service.HandlePaystackWebhook(c.Request.Context(), body, c.GetHeader("x-paystack-signature")); err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"received": true})
}
