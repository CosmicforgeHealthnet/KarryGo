// Package identityhttp exposes an internal, HMAC-protected rider lookup used
// by other services (e.g. support-dispute-service) to resolve a rider's
// display identity by ID. It returns only minimal identity fields. Mirrors the
// hauling service's identity feature.
package identityhttp

import (
	"context"
	nethttp "net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/profile"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// ProviderLookup resolves a rider profile by id. Satisfied by
// *profile.PostgresRepository.
type ProviderLookup interface {
	GetProviderByID(ctx context.Context, providerID string) (profile.Provider, bool, error)
}

type Handler struct {
	providers ProviderLookup
}

func NewHandler(providers ProviderLookup) *Handler {
	return &Handler{providers: providers}
}

// GET /api/v1/dispatch/internal/providers/:id
func (h *Handler) GetProvider(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	provider, ok, err := h.providers.GetProviderByID(c.Request.Context(), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if !ok {
		httpx.Abort(c, apperrors.NotFound("Rider was not found.", nil))
		return
	}

	name := ""
	if provider.FullName != nil {
		name = strings.TrimSpace(*provider.FullName)
	}
	status := "inactive"
	if provider.IsActive {
		status = "active"
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{
		"id":     provider.ID,
		"name":   name,
		"phone":  provider.Phone,
		"status": status,
	}})
}
