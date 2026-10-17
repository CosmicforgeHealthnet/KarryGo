package wallethttp

import (
	"strconv"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ProviderTransactions(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 100000 {
		httpx.Abort(c, apperrors.BadRequest("Page must be between 1 and 100000.", nil))
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		httpx.Abort(c, apperrors.BadRequest("Limit must be between 1 and 100.", nil))
		return
	}
	providerType, providerID := providerIdentity(c)
	items, err := h.service.ProviderTransactions(c.Request.Context(), providerType, providerID, limit+1, (page-1)*limit)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	respondOK(c, gin.H{"transactions": items, "page": page, "limit": limit, "has_more": hasMore})
}
