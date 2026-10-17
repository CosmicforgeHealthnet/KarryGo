package wallethttp

import (
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

func (h *Handler) SupportedBanks(c *gin.Context) {
	banks, err := h.service.SupportedBanks(c.Request.Context())
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"banks": banks, "country": "NG", "currency": "NGN"})
}
