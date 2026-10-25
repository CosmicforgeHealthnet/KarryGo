package http

import (
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"

	policymodels "cosmicforge/logistics/services/pricing-service/internal/features/policies/models"
	"cosmicforge/logistics/services/pricing-service/internal/platform/servicetrust"
	"cosmicforge/logistics/shared/go/adminread"
)

var errAdminScope = errors.New("pricing: delegated Admin scope does not authorize this policy")

type delegatedAdminScope struct {
	Market, Zone, Organization, Tenant string
}

func bridgeScope(c *gin.Context) (delegatedAdminScope, bool, error) {
	if servicetrust.CallerFrom(c) != "admin-backoffice-service" {
		return delegatedAdminScope{}, false, nil
	}
	scope := delegatedAdminScope{
		Market: c.Query("admin_market"), Zone: c.Query("admin_zone"),
		Organization: c.Query("admin_organization"), Tenant: c.Query("admin_tenant"),
	}
	if !adminread.Opaque(scope.Market) || !adminread.Opaque(scope.Zone) || !adminread.Opaque(scope.Organization) || !adminread.Opaque(scope.Tenant) {
		return delegatedAdminScope{}, true, fmt.Errorf("%w: complete signed scope is required", errAdminScope)
	}
	return scope, true, nil
}

func (h *Handler) authorizeVersionScope(c *gin.Context, versionID string) error {
	scope, required, err := bridgeScope(c)
	if err != nil || !required {
		return err
	}
	version, err := h.policies.GetVersion(c.Request.Context(), versionID)
	if err != nil {
		return err
	}
	return h.authorizePolicyScope(c, version.PolicyID, scope)
}

func (h *Handler) authorizePolicyIDScope(c *gin.Context, policyID string) error {
	scope, required, err := bridgeScope(c)
	if err != nil || !required {
		return err
	}
	return h.authorizePolicyScope(c, policyID, scope)
}

func (h *Handler) authorizePolicyScope(c *gin.Context, policyID string, scope delegatedAdminScope) error {
	policy, err := h.policies.GetPolicy(c.Request.Context(), policyID)
	if err != nil {
		return err
	}
	if policy.GeographyLevel == policymodels.LevelZone && policy.GeographyRef == scope.Zone || policy.GeographyLevel == policymodels.LevelMarket && policy.GeographyRef == scope.Market {
		return nil
	}
	return fmt.Errorf("%w: policy geography is outside delegated market and zone", errAdminScope)
}

func authorizeDirectScope(c *gin.Context, level, reference string) error {
	scope, required, err := bridgeScope(c)
	if err != nil || !required {
		return err
	}
	if level == policymodels.LevelZone && reference == scope.Zone || level == policymodels.LevelMarket && reference == scope.Market {
		return nil
	}
	return fmt.Errorf("%w: requested geography is outside delegated market and zone", errAdminScope)
}

func authorizeMarketScope(c *gin.Context, market string) error {
	scope, required, err := bridgeScope(c)
	if err != nil || !required {
		return err
	}
	if market != scope.Market {
		return fmt.Errorf("%w: market mismatch", errAdminScope)
	}
	return nil
}

func authorizeResolveScope(c *gin.Context, request resolveRequest) error {
	scope, required, err := bridgeScope(c)
	if err != nil || !required {
		return err
	}
	if request.MarketID != scope.Market || request.ZoneID != scope.Zone {
		return fmt.Errorf("%w: resolution must use the exact delegated market and zone", errAdminScope)
	}
	return nil
}
