// Package identityhttp exposes an internal, HMAC-protected customer lookup used
// by other services (e.g. support-dispute-service) to resolve a customer's
// display identity by ID. It returns only minimal identity fields, never auth
// material.
package identityhttp

import (
	nethttp "net/http"
	"strings"

	"github.com/gin-gonic/gin"

	profileusecases "cosmicforge/logistics/services/customer-service/internal/features/profile/usecases"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	profile *profileusecases.ProfileService
}

func NewHandler(profile *profileusecases.ProfileService) *Handler {
	return &Handler{profile: profile}
}

// GET /internal/customers/:id
func (h *Handler) GetCustomer(c *gin.Context) {
	customer, err := h.profile.GetProfile(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	name := ""
	if customer.FirstName != nil {
		name = strings.TrimSpace(*customer.FirstName)
	}
	if customer.LastName != nil {
		name = strings.TrimSpace(name + " " + *customer.LastName)
	}

	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{
		"id":     customer.ID,
		"name":   name,
		"phone":  customer.Phone,
		"email":  customer.Email,
		"status": customer.Status,
	}})
}
