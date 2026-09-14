package providerassignments

import (
	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/fleetproviderwork"
	"cosmicforge/logistics/shared/go/providercompat"
)

// RegisterFleetWork mounts Hauling's canonical provider-facing corporate
// surface: company-assignment LIST/detail/accept/decline and organization
// offer LIST/detail/accept/decline.
//
// It is the same shared facade Taxi uses, against the same Fleet routes, for
// two reasons. First, Fleet already speaks HAULING on those routes (its
// caller allowlist maps driver-hauling-service -> HAULING), so no new Fleet
// contract is needed. Second, the LIST reads are what make reconnect recovery
// work (HLG-005): a provider app that restarts recovers its corporate work by
// reading it back, never by hoping a notification arrived.
//
// The pre-existing private-assignment relay routes registered by Register are
// deliberately left in place. They serve Fleet's separate private-operations
// assignment family and are a working contract; this surface is additive.
func RegisterFleetWork(
	group *gin.RouterGroup,
	authService *providerauthusecases.AuthService,
	client *fleetproviderwork.Client,
	corporate providercompat.CorporateContextReader,
	enabled bool,
) {
	if authService == nil {
		return
	}
	handler := fleetproviderwork.NewHandler(fleetproviderwork.HandlerConfig{
		Client:    client,
		Corporate: corporate,
		Service:   providerauthusecases.ServiceCategory,
		Enabled:   enabled,
	})
	provider := group.Group("/provider")
	provider.Use(sharedauth.BearerMiddleware(authService.AccessSigner(),
		providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	fleetproviderwork.RegisterRoutes(provider, handler)
}
