package http

import (
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// SelfResolver resolves which managed-provider membership an internal
// provider self-service request (FLEET-13.7, FLEET-17.15) is about.
//
// Providers never call Fleet directly. Their owning provider service has
// authenticated the provider session and calls Fleet over HMAC-authenticated
// internal routes naming :providerId. Fleet trusts that vouching only as far
// as the verified caller goes:
//
//   - the caller must be on the configured allowlist;
//   - the caller must itself be an owning provider service, and it can only
//     speak for providers of its own service (the membership lookup is keyed
//     by the caller's service name, so a dispatch caller can never resolve a
//     hauling membership);
//   - the provider must hold a live membership there.
//
// The membership is never taken from request input. Every refusal writes its
// own response and returns ok=false. It satisfies both the companywallet and
// workforce self-service resolver interfaces structurally.
type SelfResolver struct {
	service *usecases.Service
	allowed map[string]bool
}

// NewSelfResolver builds the resolver over the caller allowlist.
func NewSelfResolver(service *usecases.Service, allowedCallers []string) *SelfResolver {
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		if name := strings.TrimSpace(caller); name != "" {
			allowed[name] = true
		}
	}
	return &SelfResolver{service: service, allowed: allowed}
}

// ownsProviders reports whether the caller is an owning provider service.
func ownsProviders(caller string) bool {
	for _, category := range []string{platformclients.CategoryDispatch, platformclients.CategoryTaxi, platformclients.CategoryHauling} {
		if service, ok := platformclients.OwningService(category); ok && service == caller {
			return true
		}
	}
	return false
}

// ResolveMembership implements the self-service resolver contract.
func (r *SelfResolver) ResolveMembership(c *gin.Context) (string, string, bool) {
	caller := c.GetString(middleware.ServiceCallerKey)
	if caller == "" || !r.allowed[caller] || !ownsProviders(caller) {
		respond.Error(c, apperrors.Forbidden("Service is not allowed to act for providers.", nil))
		return "", "", false
	}
	providerID := strings.TrimSpace(c.Param("providerId"))
	if providerID == "" {
		respond.Error(c, apperrors.BadRequest("Provider identity is required.", nil))
		return "", "", false
	}
	membership, err := r.service.LiveMembershipForProvider(c.Request.Context(), caller, providerID)
	if err != nil {
		respond.Error(c, err)
		return "", "", false
	}
	return membership.OrganizationID, membership.ID, true
}
