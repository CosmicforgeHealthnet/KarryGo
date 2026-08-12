package wallethttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

const vendorBankProviderType = "vendor_business"

type vendorBankRequest struct {
	AccountNumber string `json:"account_number"`
	BankCode      string `json:"bank_code"`
	Currency      string `json:"currency"`
}

type vendorBankView struct {
	ExternalRef         string    `json:"external_ref"`
	BankCode            string    `json:"bank_code"`
	MaskedAccount       string    `json:"masked_account"`
	ResolvedAccountName string    `json:"resolved_account_name"`
	SourceVersion       int64     `json:"source_version"`
	Status              string    `json:"status"`
	ObservedAt          time.Time `json:"observed_at"`
}

func requireVendorCaller(c *gin.Context) bool {
	raw, _ := c.Get("service_name")
	if raw != "vendor-service" {
		httpx.Abort(c, apperrors.Forbidden("Caller is not allowed to manage Vendor bank profiles.", nil))
		return false
	}
	return true
}

func maskAccount(number string) string {
	number = strings.TrimSpace(number)
	if len(number) <= 4 {
		return strings.Repeat("*", len(number))
	}
	return strings.Repeat("*", len(number)-4) + number[len(number)-4:]
}

func bankView(account walletmodels.ProviderBankAccount) vendorBankView {
	return vendorBankView{ExternalRef: account.ID, BankCode: account.BankCode, MaskedAccount: maskAccount(account.AccountNumber), ResolvedAccountName: account.AccountName, SourceVersion: account.UpdatedAt.UTC().UnixMilli(), Status: "VERIFIED", ObservedAt: account.UpdatedAt.UTC()}
}

func (h *Handler) ResolveVendorBankProfile(c *gin.Context) {
	if !requireVendorCaller(c) {
		return
	}
	var request vendorBankRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.ResolveBankAccount(c, walletusecases.ResolveBankAccountInput{AccountNumber: request.AccountNumber, BankCode: request.BankCode})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"bank_code": request.BankCode, "masked_account": maskAccount(result.AccountNumber), "resolved_account_name": result.AccountName})
}

func (h *Handler) PutVendorBankProfile(c *gin.Context) {
	if !requireVendorCaller(c) {
		return
	}
	businessID := strings.TrimSpace(c.Param("businessId"))
	if businessID == "" || c.GetHeader("Idempotency-Key") == "" {
		httpx.Abort(c, apperrors.BadRequest("Business id and Idempotency-Key are required.", nil))
		return
	}
	var request vendorBankRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	accounts, err := h.service.ListBankAccounts(c, vendorBankProviderType, businessID)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	for _, account := range accounts {
		if account.BankCode == strings.TrimSpace(request.BankCode) && account.AccountNumber == strings.TrimSpace(request.AccountNumber) {
			if !account.IsDefault {
				account, err = h.service.SetDefaultBankAccount(c, vendorBankProviderType, businessID, account.ID)
				if err != nil {
					httpx.Abort(c, err)
					return
				}
			}
			respondOK(c, bankView(account))
			return
		}
	}
	account, err := h.service.RegisterBankAccount(c, walletusecases.RegisterBankAccountInput{ProviderType: vendorBankProviderType, ProviderID: businessID, BankCode: request.BankCode, AccountNumber: request.AccountNumber, Currency: request.Currency})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if !account.IsDefault {
		account, err = h.service.SetDefaultBankAccount(c, vendorBankProviderType, businessID, account.ID)
		if err != nil {
			httpx.Abort(c, err)
			return
		}
	}
	respond(c, http.StatusCreated, bankView(account))
}

func (h *Handler) GetVendorBankProfile(c *gin.Context) {
	if !requireVendorCaller(c) {
		return
	}
	accounts, err := h.service.ListBankAccounts(c, vendorBankProviderType, strings.TrimSpace(c.Param("businessId")))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	for _, account := range accounts {
		if account.IsDefault {
			respondOK(c, bankView(account))
			return
		}
	}
	httpx.Abort(c, apperrors.NotFound("Vendor bank profile could not be found.", nil))
}
