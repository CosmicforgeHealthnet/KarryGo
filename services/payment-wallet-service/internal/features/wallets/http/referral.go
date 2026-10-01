package wallethttp

import (
	"github.com/gin-gonic/gin"

	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// referralCaller is the only service allowed to move money through the
// referral routes. Holding an internal secret is not enough.
const referralCaller = "referral-service"

func requireReferralCaller(c *gin.Context) bool {
	if c.GetString("service_name") != referralCaller {
		httpx.Abort(c, apperrors.Forbidden("Only referral-service may use the referral routes.", nil))
		return false
	}
	return true
}

func bindReferralPosting(c *gin.Context) (walletusecases.ReferralPostingRequest, bool) {
	var request walletusecases.ReferralPostingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return request, false
	}
	return request, true
}

// PostReferralCredit pays a referral reward into a referral balance.
func (h *Handler) PostReferralCredit(c *gin.Context) {
	if !requireReferralCaller(c) {
		return
	}
	request, ok := bindReferralPosting(c)
	if !ok {
		return
	}
	result, err := h.service.PostReferralCredit(c.Request.Context(), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

// PostReferralReversal takes a referral reward back.
func (h *Handler) PostReferralReversal(c *gin.Context) {
	if !requireReferralCaller(c) {
		return
	}
	request, ok := bindReferralPosting(c)
	if !ok {
		return
	}
	result, err := h.service.PostReferralReversal(c.Request.Context(), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

// ReferralTotals reports one owner's referral postings and balances.
func (h *Handler) ReferralTotals(c *gin.Context) {
	if !requireReferralCaller(c) {
		return
	}
	result, err := h.service.ReferralTotals(c.Request.Context(), c.Query("owner_type"), c.Query("owner_id"), c.Query("provider_type"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}
