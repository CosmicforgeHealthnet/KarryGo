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
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, result)
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

func (h *Handler) RequestRefund(c *gin.Context) {
	var request refundRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.RequestRefund(c.Request.Context(), walletusecases.RefundInput{
		PaymentReference: request.PaymentReference,
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
