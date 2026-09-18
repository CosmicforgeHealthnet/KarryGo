package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/config"
	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/services/hauling-service/internal/features/truckcapability"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// nearbyProviderAdapter narrows the full availability store to the two fields
// the supply preview may see.
//
// The narrowing is the point. The store's ProviderStatus carries a provider id,
// a truck id and a live GPS fix; handing all of that to a preview that must
// expose no identity would leave the guarantee resting on the preview's
// discipline rather than on its inputs.
type nearbyProviderAdapter struct {
	store availabilityrepositories.AvailabilityStore
}

func (a nearbyProviderAdapter) NearbyProviders(
	ctx context.Context, center geo.Point, radiusKm float64, limit int,
) ([]truckcapability.NearbyProvider, error) {
	statuses, err := a.store.NearbyProviders(ctx, center, radiusKm, limit)
	if err != nil {
		return nil, err
	}
	out := make([]truckcapability.NearbyProvider, 0, len(statuses))
	for _, status := range statuses {
		out = append(out, truckcapability.NearbyProvider{
			ProviderID: status.ProviderID,
			DistanceKm: geo.HaversineKm(center, geo.Point{Lat: status.Lat, Lng: status.Lng}),
		})
	}
	return out, nil
}

// capabilityWiring is everything Section 9 adds, built once and handed to the
// route registrar and the worker list.
type capabilityWiring struct {
	service   *truckcapability.Service
	handler   *truckcapability.Handler
	previewer *truckcapability.Previewer
	authority capabilityAuthority
	config    config.Config
}

// buildCapabilityWiring constructs the capability feature.
//
// It is always constructed, even with every flag off, because the provider
// capability endpoints are how a fleet gets surveyed in the first place: a
// deployment that cannot record capability until capability matching is on
// could never reach the completeness needed to turn it on.
func buildCapabilityWiring(
	cfg config.Config,
	db *pgxpool.Pool,
	availability availabilityrepositories.AvailabilityStore,
) capabilityWiring {
	service := truckcapability.New(truckcapability.Options{
		Repository:                    truckcapability.NewRepository(db),
		CertificationRequiredProducts: cfg.CertificationRequiredProducts,
	})
	return capabilityWiring{
		service: service,
		handler: truckcapability.NewHandler(service),
		previewer: truckcapability.NewPreviewer(truckcapability.PreviewOptions{
			Service:  service,
			Nearby:   nearbyProviderAdapter{store: availability},
			RadiusKm: cfg.MatchMaxRadiusKm,
			Enabled:  cfg.SupplyPreviewEnabled,
		}),
		authority: capabilityAuthority{service: service},
		config:    cfg,
	}
}

// registerRoutes mounts the public, provider and internal capability surfaces.
func (w capabilityWiring) registerRoutes(
	group *gin.RouterGroup,
	authService *providerauthusecases.AuthService,
	serviceSecrets string,
) {
	// Public: the product catalogue. No supply, no prices, no identities.
	truckcapability.RegisterPublic(group, w.handler)

	// Provider: the same bearer boundary the rest of the provider surface
	// uses, so capability edits are authenticated exactly like truck edits.
	provider := group.Group("/provider")
	provider.Use(sharedauth.BearerMiddleware(
		authService.AccessSigner(),
		providerauthusecases.ProviderRole,
		providerauthusecases.ProviderService,
	))
	truckcapability.RegisterProvider(provider, w.handler)

	// Internal: HMAC service auth, mounted on its own group so the boundary is
	// installed before the routes rather than alongside them.
	verifier := serviceauth.NewVerifier(serviceauth.ParseSecrets(serviceSecrets), serviceAuthSkew)
	internal := group.Group("/internal")
	internal.Use(serviceauth.Middleware(verifier))
	truckcapability.RegisterInternal(internal, w.handler)
}

// serviceAuthSkew matches the tolerance every other internal surface in this
// service uses. Restating it as a constant keeps a future change from moving
// one surface's clock tolerance without the others.
const serviceAuthSkew = 5 * time.Minute
