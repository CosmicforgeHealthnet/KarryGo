package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/hauling-service/internal/config"
	"cosmicforge/logistics/services/hauling-service/internal/database"

	availabilityhttp "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/http"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	availabilityusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/usecases"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookinghttp "cosmicforge/logistics/services/hauling-service/internal/features/booking/http"
	bookingpayments "cosmicforge/logistics/services/hauling-service/internal/features/booking/payments"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"
	"cosmicforge/logistics/services/hauling-service/internal/features/movement"

	haulingcommercemission "cosmicforge/logistics/services/hauling-service/internal/features/commercemission"
	haulingcommercequote "cosmicforge/logistics/services/hauling-service/internal/features/commercequote"
	fleetauthority "cosmicforge/logistics/services/hauling-service/internal/features/fleetauthority"
	identityhttp "cosmicforge/logistics/services/hauling-service/internal/features/identity/http"
	notificationhttp "cosmicforge/logistics/services/hauling-service/internal/features/notifications/http"
	privateexecution "cosmicforge/logistics/services/hauling-service/internal/features/privateexecution"
	providerassignments "cosmicforge/logistics/services/hauling-service/internal/features/providerassignments"
	restrictions "cosmicforge/logistics/services/hauling-service/internal/features/restrictions"

	providerauthhttp "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/http"
	providerauthrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/repositories"
	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"

	providerprofilehttp "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/http"
	providerprofilerepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/repositories"
	providerprofileusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/usecases"

	"cosmicforge/logistics/services/hauling-service/internal/apiresponse"
	haulingsettlement "cosmicforge/logistics/services/hauling-service/internal/features/booking/settlement"
	"cosmicforge/logistics/services/hauling-service/internal/features/completedwork"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/custodyops"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/fleetproviderwork"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/managedsettlement"
	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/outboxmetrics"
	"cosmicforge/logistics/shared/go/providercompat"
	"cosmicforge/logistics/shared/go/routing"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logging.Fatal("configuration", "%v", err)
	}
	logging.Notice("hauling-service", "migration=%t redis=%s", cfg.Migration, cfg.Redis.Addr)

	// ctx is cancelled when main returns (after serviceapp.Run exits on SIGTERM).
	// Background goroutines (auto-complete worker) use this to stop cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("database", "create hauling database pool: %v", err)
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	// The notification queue is consumed by notification-service, so it lives on
	// whichever Redis that service reads — usually a different instance from our
	// own. Reuse the main client when both point at the same address so we do not
	// hold a second pool for nothing.
	notificationRedis := redisClient
	if cfg.NotificationRedis.Addr != cfg.Redis.Addr || cfg.NotificationRedis.DB != cfg.Redis.DB {
		notificationRedis = redis.NewClient(&redis.Options{
			Addr:     cfg.NotificationRedis.Addr,
			Password: cfg.NotificationRedis.Password,
			DB:       cfg.NotificationRedis.DB,
		})
		defer notificationRedis.Close()
		logging.Notice("notification-queue", "redis=%s", cfg.NotificationRedis.Addr)
	}

	logConnectivity(ctx, cfg.DatabaseURL, cfg.Redis.Addr, db, redisClient)

	if cfg.Migration {
		logging.Notice("migration", "mode enabled")
		migrationCtx, mcancel := context.WithTimeout(ctx, 30*time.Second)
		if err := database.ApplyMigrations(migrationCtx, db); err != nil {
			mcancel()
			logging.Fatal("migration", "apply hauling-service migrations: %v", err)
		}
		mcancel()
		logging.Success("migration", "applied successfully")
	}

	// ─── Repositories ─────────────────────────────────────────────────────────
	providerRepo := providerauthrepositories.NewPostgresProviderRepository(db)
	sessionRepo := providerauthrepositories.NewPostgresRefreshSessionRepository(db)
	challengeStore := providerauthrepositories.NewRedisOTPChallengeRepository(redisClient)
	profileRepo := providerprofilerepositories.NewPostgresProfileRepository(db)
	truckRepo := providerprofilerepositories.NewPostgresTruckRepository(db)
	// Shared multi-manifest custody: a commerce haulage's vendor manifests are
	// collected and delivered through the same ledger Dispatch and Taxi use.
	bookingRepo := bookingrepo.NewPostgresBookingRepository(db).WithCustody(custodyops.NewRepository(db, "HAULING"))

	// ─── Auth service ──────────────────────────────────────────────────────────
	var membershipReader providercompat.Reader
	// fleetCompat is the concrete Fleet client. It backs three distinct
	// consumers: the legacy membership-summary read, the CANONICAL Section 7
	// corporate-context read, and canonical completed-work submission. One
	// client, because all three speak to the same service with the same
	// credentials.
	var fleetCompat *providercompat.Client
	var fleetCorporateContext providercompat.CorporateContextReader
	var fleetProviderWorkClient *fleetproviderwork.Client
	if strings.TrimSpace(cfg.FleetServiceURL) != "" && len(cfg.FleetServiceSecret) > 0 {
		fleetCompat = providercompat.NewClient(cfg.FleetServiceURL, cfg.FleetServiceCaller,
			cfg.FleetServiceSecret, cfg.FleetServiceTimeout)
		fleetCorporateContext = fleetCompat
		fleetProviderWorkClient, _ = fleetproviderwork.NewClient(fleetproviderwork.Config{
			BaseURL:         cfg.FleetServiceURL,
			ServiceName:     cfg.FleetServiceCaller,
			Secret:          cfg.FleetServiceSecret,
			ProviderService: providerauthusecases.ServiceCategory,
			Timeout:         cfg.FleetServiceTimeout,
		})
	}
	if cfg.ProviderOrganizationModeEnabled && fleetCompat != nil {
		membershipReader = fleetCompat
	}
	authService := providerauthusecases.NewAuthService(providerauthusecases.Options{
		Providers:          providerRepo,
		Sessions:           sessionRepo,
		Challenges:         challengeStore,
		AccessTokenSecret:  cfg.ProviderTokenSecret,
		RefreshTokenSecret: cfg.ProviderRefreshSecret,
		OTPSecret:          cfg.ProviderOTPSecret,
		AccessTokenTTL:     cfg.AccessTokenTTL,
		RefreshTokenTTL:    cfg.RefreshTokenTTL,
		OTPTTL:             cfg.OTPTTL,
		OTPRateWindow:      cfg.OTPRateWindow,
		OTPMaxRequests:     cfg.OTPMaxRequests,
		OTPMaxAttempts:     cfg.OTPMaxAttempts,
		OTPDebug:           cfg.OTPDebug,
		MembershipReader:   membershipReader,
		CorporateContext:   fleetCorporateContext,
		ProviderService:    cfg.FleetServiceCaller,
	})

	// customerSigner verifies customer bearer tokens (read-only; same secret as customer-service).
	customerSigner := sharedauth.NewTokenSigner(cfg.CustomerTokenSecret)

	// ─── Profile service ───────────────────────────────────────────────────────
	profileService := providerprofileusecases.NewProfileService(profileRepo, truckRepo)

	// ─── Availability service ──────────────────────────────────────────────────
	// routesClient is nil unless HAULING_ROUTES_API_KEY is set. Route geometry
	// is optional everywhere it is used.
	routesClient := routing.New(cfg.RoutesAPIKey)
	if routesClient == nil {
		logging.Notice("routing", "disabled (HAULING_ROUTES_API_KEY unset) - trip maps and progress fall back to straight lines")
	}

	onlineTTL := time.Duration(cfg.ProviderOnlineTTL) * time.Second
	// The live-tracking index expires fixes on the same clock the availability
	// keys use, so a provider can never be matchable from a position that has
	// already aged out of their status key.
	availabilityStore := availabilityrepositories.NewRedisAvailabilityStoreWithTTL(redisClient, onlineTTL)
	restrictionService := restrictions.New(db)
	availService := availabilityusecases.NewAvailabilityServiceWithAuthority(availabilityStore, truckRepo, onlineTTL, availabilityrepositories.NewPostgresAuthorityStore(db)).WithRestrictionGate(restrictionService)

	// ─── Booking service ───────────────────────────────────────────────────────
	// bookingNotifier sends booking-lifecycle notifications via notification-service.
	// Incomplete configuration produces a delivery error; it never reports a
	// fabricated notification success.
	bookingNotifier := bookingclients.NewDurableBookingNotifier(notificationRedis)

	// paymentClient binds booking fares to payment-wallet-service. When
	// HAULING_PAYMENT_URL/SECRET are unset payment-required paths fail closed.
	var paymentClient bookingusecases.PaymentClient
	var commerceSettlement bookingusecases.CommerceDeliverySettlementAuthority
	if cfg.PaymentURL != "" && len(cfg.PaymentSecret) > 0 {
		walletPayments := bookingpayments.NewWalletPaymentClient(cfg.PaymentURL, "driver-hauling-service", cfg.PaymentSecret)
		paymentClient = walletPayments
		commerceSettlement = walletPayments
	}

	// Section 12 (WAL-013). Hauling never settles corporate work -- Fleet
	// resolves the compensation and instructs Wallet. The handoff's only job
	// is to find out whether that money moved, so an ambiguous completion is
	// visible rather than assumed. Without wallet credentials there is nothing
	// to ask, so no handoff is wired and the completion is left to Fleet.
	var managedHandoff bookingusecases.ManagedSettlementHandoff
	if strings.TrimSpace(cfg.PaymentURL) != "" && len(cfg.PaymentSecret) > 0 {
		managedHandoff = haulingsettlement.New(managedsettlement.Client{
			BaseURL:     cfg.PaymentURL,
			ServiceName: "driver-hauling-service",
			Secret:      cfg.PaymentSecret,
			HTTPClient:  &http.Client{Timeout: 10 * time.Second},
		})
	}

	bookingService := bookingusecases.NewBookingService(bookingusecases.Options{
		Bookings:           bookingRepo,
		Availability:       availabilityStore,
		Trucks:             truckLookup{trucks: truckRepo},
		Payments:           paymentClient,
		CommerceSettlement: commerceSettlement,
		// Section 10 corporate completion. Both default off; see each flag's
		// own doc comment in internal/config for what turning it on changes.
		ManagedSettlement:    bookingusecases.ManagedSettlementConfig{Enabled: cfg.HaulingManagedSettlementEnabled},
		ManagedHandoff:       managedHandoff,
		CompletedWorkEnabled: cfg.HaulingFleetCompletedWorkEnabled,
		Notifier:             bookingNotifier,
		MatchTimeoutSeconds:  cfg.BookingMatchTimeout,
		SearchWindowSeconds:  cfg.BookingSearchWindow,
		MaxRadiusKm:          cfg.MatchMaxRadiusKm,
		HandoverSecret:       cfg.HandoverSecret,
		AllowCashPayments:    cfg.CashPaymentsEnabled,
		Restrictions:         restrictionService,
		// nil when HAULING_ROUTES_API_KEY is unset: routes are then never
		// fetched and the apps fall back to straight-line geometry.
		Routes: routesClient,
	})
	authorityHandler := fleetauthority.New(db, fleetauthority.Flags{
		Enabled: cfg.FleetIntegrationEnabled, KillSwitch: cfg.FleetIntegrationKillSwitch,
	})
	inboundVerifier := serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute)
	var custodyMedia *mediaclient.Client
	if strings.TrimSpace(cfg.MediaURL) != "" && strings.TrimSpace(cfg.MediaToken) != "" {
		custodyMedia = mediaclient.New(mediaclient.Config{BaseURL: cfg.MediaURL, ServiceName: "driver-hauling-service", Token: cfg.MediaToken, HTTPClient: &http.Client{Timeout: 10 * time.Second}})
	}
	privateExecutionService := privateexecution.NewService(
		privateexecution.NewRepository(db),
		privateexecution.NewHTTPAcceptanceVerifier(cfg.FleetServiceURL, cfg.FleetServiceCaller,
			cfg.FleetServiceSecret, cfg.FleetServiceTimeout),
		cfg.PrivateExecutionEnabled, cfg.FleetIntegrationKillSwitch).WithRestrictionGate(restrictionService)
	providerAssignmentClient := providerassignments.NewClient(cfg.FleetServiceURL,
		cfg.FleetServiceCaller, cfg.FleetServiceSecret, cfg.FleetServiceTimeout)
	// HLG-018: canonical completed-work delivery rides the SAME outbox table
	// as the Fleet domain events above, with its own target and its own
	// outcome classification. It is gated separately because a deployment can
	// legitimately emit domain events without yet being certified to report
	// completed work.
	if cfg.HaulingFleetCompletedWorkEnabled && fleetCompat != nil && !cfg.FleetIntegrationKillSwitch {
		go (&completedwork.Worker{DB: db, Client: fleetCompat, Enabled: true}).Run(ctx, 5*time.Second)
		logging.Notice("hauling-fleet", "canonical completed-work delivery enabled")
	}
	if cfg.OutboxWorkerEnabled && cfg.FleetIntegrationEnabled && !cfg.FleetIntegrationKillSwitch {
		go (&fleetauthority.OutboxWorker{DB: db, URL: cfg.FleetEventURL, Secret: cfg.FleetEventSecret}).Run(ctx)
	}

	// notificationClient brokers provider-app notification access to
	// notification-service (feed, realtime token, device registration). Empty
	// BaseURL skips the proxy routes (local dev without notification-service).
	notificationClient := notifications.Client{
		BaseURL:     cfg.NotificationURL,
		ServiceName: "driver-hauling-service",
		Secret:      cfg.NotificationSecret,
	}

	// Auto-complete worker: promotes delivered bookings to completed after the
	// grace period. Runs until ctx is cancelled (i.e. service shutdown).
	if !cfg.FleetIntegrationKillSwitch {
		go bookingService.RunAutoCompleteWorker(ctx)
	}

	// Match recovery worker: re-adopts bookings whose in-memory matcher was lost
	// (e.g. across a restart) so they resume matching or go unmatched instead of
	// stranding in pending_match forever.
	if !cfg.FleetIntegrationKillSwitch {
		go bookingService.RunMatchRecoveryWorker(ctx)
	}

	// Outbox and reconciliation health, in the same metric names every other
	// service reports (OBS-006, OBS-007). Hauling's completed-work outbox
	// carries the corporate money path to Fleet; before this, a row stuck in
	// RECONCILIATION_REQUIRED was only visible to someone running psql.
	//
	// The source description lives with the table's writer, where a test can
	// run it against the real migrated schema.
	haulingMetrics := outboxmetrics.NewReporter(db, completedwork.MetricsInterval,
		completedwork.MetricsSource()).WithAuthToken(cfg.MetricsAuthToken)

	// ─── HTTP ──────────────────────────────────────────────────────────────────
	// Customer movement events. The sweeper reads committed booking state and
	// queues what changed; the publisher delivers it to the customer context
	// owner. Both are additive: neither can fail a haulage.
	if cfg.CustomerMovementEventsEnabled {
		go movement.NewPublisher(db).Run(ctx, 10*time.Second)
		go (&events.MovementPublisher{
			Pool:  db,
			Owner: movement.SourceService,
			Deliverer: &events.HTTPDeliverer{
				BaseURL: cfg.CustomerServiceURL, Path: events.CustomerMovementIngressPath,
				ServiceName: movement.SourceService, Secret: cfg.CustomerServiceSecret,
			},
		}).Run(ctx, 5*time.Second)
	}

	serviceapp.Run(serviceapp.Options{
		Name:        "hauling-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/hauling",
		Capabilities: []string{
			"truck provider auth (OTP phone)",
			"truck provider profiles and truck records",
			"provider availability management (online/offline via Redis)",
			"customer availability check",
			"haulage bookings with async provider matching",
			"booking lifecycle: accept -> pickup -> deliver -> complete",
			"fare estimation",
		},
		ReadyChecks: []func(context.Context) error{
			db.Ping,
			func(ctx context.Context) error {
				return redisClient.Ping(ctx).Err()
			},
		},
		Workers: []func(context.Context) error{haulingMetrics.Run},
		Register: func(group *gin.RouterGroup) {
			haulingMetrics.RegisterRoutes(group)
			// HLG-021: the server-side typed financial projection. Mounted on
			// the ROOT group so every provider-facing response is covered,
			// including any route added later; it no-ops for customer and
			// internal requests, and for an independent driver it never
			// touches the body at all.
			group.Use(apiresponse.ProviderFinancialPrivacy(
				fleetCorporateContext, cfg.HaulingTypedFinancialVisibilityEnabled,
				providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
			group.Use(func(c *gin.Context) {
				if cfg.FleetIntegrationKillSwitch && c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
					c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": gin.H{"code": "integration_kill_switch", "message": "Hauling writes are temporarily disabled."}})
					return
				}
				if cfg.CommerceIntegrationKillSwitch && strings.Contains(c.Request.URL.Path, "/internal/commerce/") &&
					c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
					c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": gin.H{"code": "commerce_integration_kill_switch", "message": "Hauling commerce writes are temporarily disabled."}})
					return
				}
				c.Next()
			})
			providerauthhttp.RegisterProviderAuthRoutes(group, authService)
			providerprofilehttp.RegisterProfileRoutes(group, profileService, authService)
			availabilityhttp.RegisterAvailabilityRoutes(group, availService, authService, customerSigner)
			bookinghttp.RegisterBookingRoutes(group, bookingService, authService, customerSigner, providerRepo, truckRepo, redisClient)
			notificationhttp.RegisterNotificationRoutes(group, notificationClient, authService.AccessSigner(), customerSigner)
			identityhttp.RegisterIdentityRoutes(group, authService, serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute))
			if len(serviceauth.ParseSecrets(cfg.ServiceSecrets)) > 0 {
				custodyops.RegisterInternal(group, custodyops.NewRepository(db, "HAULING"), inboundVerifier,
					custodyops.MediaFileAuthorizer{Client: custodyMedia, AllowedOwners: map[string]struct{}{"driver-hauling-service": {}, "vendor-service": {}}},
					custodyops.Policy{}, []string{"vendor-service", "operations-service"})
			}
			custodyops.RegisterProvider(group, custodyops.NewRepository(db, "HAULING"), authService.AccessSigner(), providerauthusecases.ProviderRole, providerauthusecases.ProviderService)

			// The internal commerce delivery-quote contract (Vendor Service
			// Section 17C). It answers serviceability and price for a route;
			// it creates no booking and matches no provider.
			//
			// Registration is skipped entirely when this deployment holds no
			// secret for the vendor caller, so a deployment that does not
			// serve commerce quotes exposes no route to probe rather than one
			// that answers 401 and confirms it exists.
			if _, trusted := serviceauth.ParseSecrets(cfg.ServiceSecrets)[commercequote.SignatureCaller]; trusted && cfg.CommerceIntegrationEnabled {
				haulingcommercequote.Register(
					group,
					haulingcommercequote.NewHandler(haulingcommercequote.New(
						haulingcommercequote.Options{
							Store:        haulingcommercequote.NewRepository(db),
							Availability: availabilityStore,
							Trucks:       commerceTruckLookup{trucks: truckRepo},
							MaxRadiusKm:  cfg.MatchMaxRadiusKm,
							TTL:          cfg.CommerceQuoteTTL,
						},
					)),
					db,
					serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
					[]string{commercequote.SignatureCaller},
				)
			}

			// The internal commerce mission contract (Vendor Service Section
			// 17F). Unlike the quote surface above this one EXECUTES: it
			// creates a real booking and starts provider matching for a fee
			// the customer has already paid. It is skipped without a booking
			// service rather than accepting hauls nothing would ever match.
			if _, trusted := serviceauth.ParseSecrets(cfg.ServiceSecrets)[commercemission.SignatureCaller]; trusted && cfg.CommerceIntegrationEnabled &&
				bookingService != nil {
				bookinghttp.RegisterCommerceHandover(group, bookingService, db,
					serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
					[]string{commercemission.SignatureCaller})
				haulingcommercemission.Register(
					group,
					haulingcommercemission.NewHandler(haulingcommercemission.New(
						haulingcommercemission.Options{
							Store:   haulingcommercemission.NewRepository(db).WithCustody(custodyops.NewRepository(db, "HAULING")),
							Matcher: bookingService,
						},
					)),
					db,
					serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
					[]string{commercemission.SignatureCaller},
				)
				// The shared cross-authority capability contract Vendor reads
				// to decide whether Hauling may be selected at all. Mounted
				// beside the mission routes and behind the same boundary.
				haulingcommercemission.RegisterCapability(
					group,
					haulingcommercemission.CapabilityProvider{Options: haulingcommercemission.CapabilityOptions{
						Enabled:                    true,
						CorporateProviderSupported: cfg.HaulingFleetProviderWorkEnabled,
					}},
					db,
					serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
					[]string{commercemission.SignatureCaller},
				)
			}
			fleetauthority.Register(group, authorityHandler, inboundVerifier, splitCSV(cfg.FleetAllowedCallers))
			restrictions.Register(group, restrictionService, authorityHandler, inboundVerifier, splitCSV(cfg.FleetAllowedCallers))
			privateexecution.Register(group, privateexecution.NewHandler(privateExecutionService),
				authorityHandler, inboundVerifier, splitCSV(cfg.FleetAllowedCallers))
			providerassignments.Register(group, authService, providerAssignmentClient)
			// Section 10: the canonical provider-facing corporate surface
			// (company-assignment LIST/detail/accept/decline and organization
			// offer LIST/detail/accept/decline), shared with Taxi. The LIST
			// reads are what make reconnect recovery work: a restarted
			// provider app reads its corporate work back rather than hoping a
			// notification arrived.
			providerassignments.RegisterFleetWork(group, authService,
				fleetProviderWorkClient, fleetCorporateContext,
				cfg.HaulingFleetProviderWorkEnabled)
			completedwork.RegisterRoutes(group,
				completedwork.NewHandler(&completedwork.Store{DB: db}, splitCSV(cfg.FleetAllowedCallers)),
				inboundVerifier)
		},
	})
}

func splitCSV(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// truckLookup adapts the provider_profile truck repository to the booking
// usecase TruckLookup interface, so the matcher can filter providers by truck
// type and capacity without the usecase importing the provider_profile package.
type truckLookup struct {
	trucks providerprofilerepositories.TruckRepository
}

func (t truckLookup) GetTruck(ctx context.Context, truckID string) (bookingusecases.TruckInfo, error) {
	truck, err := t.trucks.GetByIDAnywhere(ctx, truckID)
	if err != nil {
		return bookingusecases.TruckInfo{}, err
	}
	return bookingusecases.TruckInfo{
		TruckType:  truck.TruckType,
		CapacityKg: truck.CapacityKg,
		Status:     truck.Status,
	}, nil
}

// commerceTruckLookup adapts the same truck repository to the commerce quote
// service's own narrow lookup, so a quote applies exactly the truck-capacity
// and truck-type filters the matcher will later apply. A quote issued on
// looser rules than the matcher enforces is a quote that cannot be filled.
type commerceTruckLookup struct {
	trucks providerprofilerepositories.TruckRepository
}

func (t commerceTruckLookup) GetTruck(ctx context.Context, truckID string) (haulingcommercequote.TruckInfo, error) {
	truck, err := t.trucks.GetByIDAnywhere(ctx, truckID)
	if err != nil {
		return haulingcommercequote.TruckInfo{}, err
	}
	return haulingcommercequote.TruckInfo{
		TruckType:  truck.TruckType,
		CapacityKg: truck.CapacityKg,
		Status:     truck.Status,
	}, nil
}

func logConnectivity(ctx context.Context, databaseURL, redisAddr string, db interface{ Ping(context.Context) error }, redisClient interface {
	Ping(context.Context) *redis.StatusCmd
}) {
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.Ping(pingCtx); err != nil {
		logging.Error("database", "connectivity check failed: %v", err)
	} else {
		logging.Success("database", "connected")
	}

	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		logging.Error("redis", "failed addr=%s err=%v", redisAddr, err)
	} else {
		logging.Success("redis", "connected addr=%s", redisAddr)
	}
}
