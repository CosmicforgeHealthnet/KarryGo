// Package identityhttp exposes an internal, HMAC-protected provider lookup used
// by other services (e.g. support-dispute-service) to resolve a provider's
// display identity by ID. It returns only minimal identity fields.
package identityhttp

import (
	nethttp "net/http"
	"strings"

	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	auth *providerauthusecases.AuthService
}

func NewHandler(auth *providerauthusecases.AuthService) *Handler {
	return &Handler{auth: auth}
}

// GET /internal/providers/:id
func (h *Handler) GetProvider(c *gin.Context) {
	id := c.Param("id")
	p, err := h.auth.Me(c.Request.Context(), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	name := strings.TrimSpace(p.FirstName + " " + p.LastName)
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{
		"id":     id,
		"name":   name,
		"phone":  p.Phone,
		"status": p.Status,
	}})
}
