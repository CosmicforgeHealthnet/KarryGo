package main

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/config"
	bookingpayments "cosmicforge/logistics/services/hauling-service/internal/features/booking/payments"
	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/services/hauling-service/internal/features/recovery"
	"cosmicforge/logistics/services/hauling-service/internal/features/supportcontext"
	"cosmicforge/logistics/services/hauling-service/internal/features/truckcapability"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/custodyops"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/policyingest"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/supportcase"
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
	policy    *truckcapability.PolicyIngest
	recovery  *recovery.Handler
	// recoveryService is the same recovery authority the handler wraps; the
	// owner administrative command path binds named replacements through it.
	recoveryService *recovery.Service
	support         *supportcontext.Projector
	config          config.Config
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
	notifier notifications.Notifier,
) capabilityWiring {
	service := truckcapability.New(truckcapability.Options{
		Repository:                    truckcapability.NewRepository(db),
		CertificationRequiredProducts: cfg.CertificationRequiredProducts,
	})
	recoveryService := recovery.New(recovery.Options{
		Repository: recovery.NewRepository(db),
		Candidates: recoveryCandidates{store: availability},
		// The SAME capability service the matcher uses. Recovery has no
		// fit rules of its own, so a replacement is evaluated by exactly
		// the call that chose the original provider.
		Capability:  service,
		Custody:     custodyops.NewRepository(db, "HAULING"),
		Events:      recovery.NewOutboxSink(db, cfg.RecoveryEventTarget, cfg.RecoveryEventsEnabled, nil),
		MaxRadiusKm: cfg.MatchMaxRadiusKm,
		Enabled:     cfg.RecoveryEnabled,
		Payments:    recoveryPayments(cfg),
		Notifier:    notifier,
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
		authority:       capabilityAuthority{service: service},
		recovery:        recovery.NewHandler(recoveryService),
		recoveryService: recoveryService,
		policy: truckcapability.NewPolicyIngest(truckcapability.PolicyIngestOptions{
			DB:                db,
			CallerName:        cfg.AdminServiceCaller,
			AdminSecret:       cfg.AdminServiceSecret,
			AdminHosts:        truckcapability.AdminHostsFrom(cfg.AdminAllowedHosts),
			ReconcileInterval: cfg.PolicyReconcileInterval,
		}),
		support: supportcontext.NewProjector(db),
		config:  cfg,
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

	// Section 15: Support's minimized read of a booking. Mounted on the api
	// base rather than the /internal group because supportcase.ContextPath
	// carries its own /internal prefix, and it verifies its own caller: only
	// support-dispute-service may read, and only about a booking the named
	// complainant is party to.
	supportcase.RegisterContextRoute(group, verifier, w.support)

	// Recovery is an internal/operator surface only. Nothing here lets a
	// provider choose who replaces them.
	if w.config.RecoveryEnabled {
		recovery.RegisterInternal(internal, w.recovery)
	}

	// The control-plane publication sink. Mounted from the api base rather
	// than the /internal group above because policyingest.IngressPath already
	// carries its own /internal prefix, and behind the same HMAC verifier --
	// this endpoint decides which trucks may take which work, so it is the
	// last surface that should get a boundary of its own.
	if w.config.PolicyIngestEnabled {
		publications := group.Group("")
		publications.Use(serviceauth.Middleware(verifier))
		policyingest.RegisterRoutes(publications, w.policy.Ingestor())
	}
}

// runPolicyReconciler retries publications whose read-back outcome was unknown.
func (w capabilityWiring) runPolicyReconciler(ctx context.Context) {
	if !w.config.PolicyIngestEnabled {
		return
	}
	w.policy.RunReconcileWorker(ctx)
}

// serviceAuthSkew matches the tolerance every other internal surface in this
// service uses. Restating it as a constant keeps a future change from moving
// one surface's clock tolerance without the others.
const serviceAuthSkew = 5 * time.Minute

// recoveryCandidates narrows the availability store to the two fields
// replacement search may see.
//
// The same narrowing the supply preview uses, for the same reason: handing a
// selector the full provider status -- live GPS, truck id, heading -- when it
// only needs "who is near and how far" leaves the boundary resting on the
// selector's discipline rather than on its inputs.
type recoveryCandidates struct {
	store availabilityrepositories.AvailabilityStore
}

func (c recoveryCandidates) NearbyProviders(
	ctx context.Context, center geo.Point, radiusKm float64, limit int,
) ([]recovery.NearbyProvider, error) {
	statuses, err := c.store.NearbyProviders(ctx, center, radiusKm, limit)
	if err != nil {
		return nil, err
	}
	out := make([]recovery.NearbyProvider, 0, len(statuses))
	for _, status := range statuses {
		out = append(out, recovery.NearbyProvider{
			ProviderID: status.ProviderID,
			DistanceKm: geo.HaversineKm(center, geo.Point{Lat: status.Lat, Lng: status.Lng}),
		})
	}
	return out, nil
}

// recoveryPayments moves a booking's held payment to its replacement (Section
// 17). Without wallet credentials there is no payment to move, and the
// settlement guard is the only protection -- which it is anyway.
func recoveryPayments(cfg config.Config) recovery.PaymentRebinder {
	if strings.TrimSpace(cfg.PaymentURL) == "" || len(cfg.PaymentSecret) == 0 {
		return nil
	}
	return bookingpayments.NewWalletPaymentClient(cfg.PaymentURL, "driver-hauling-service", cfg.PaymentSecret)
}
