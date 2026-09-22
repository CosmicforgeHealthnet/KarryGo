package main

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/support-dispute-service/internal/config"
	"cosmicforge/logistics/services/support-dispute-service/internal/database"
	notificationhttp "cosmicforge/logistics/services/support-dispute-service/internal/features/notifications/http"
	supportclients "cosmicforge/logistics/services/support-dispute-service/internal/features/support/clients"
	supporthttp "cosmicforge/logistics/services/support-dispute-service/internal/features/support/http"
	supportrepositories "cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	logging.Notice("support-dispute-service config", "migration=%t database=%s", cfg.Migration, cfg.DatabaseURL)

	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("database", "create pool: %v", err)
	}
	defer db.Close()

	if cfg.Migration {
		logging.Notice("migration", "mode enabled")
		migCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := database.ApplyMigrations(migCtx, db); err != nil {
			cancel()
			logging.Fatal("migration", "apply migrations: %v", err)
		}
		cancel()
		logging.Success("migration", "applied successfully")
	}

	repo := supportrepositories.NewPostgresSupportRepository(db)

	// Outbound collaborators. Each is a no-op when its config is empty (local dev).
	notifier := supportclients.NewSupportNotifier(cfg.NotificationURL, []byte(cfg.NotificationSecret))
	identity := supportclients.NewHTTPIdentityResolver(supportclients.IdentityConfig{
		CustomerURL:    cfg.CustomerServiceURL,
		CustomerSecret: []byte(cfg.CustomerServiceSecret),
		HaulingURL:     cfg.HaulingServiceURL,
		HaulingSecret:  []byte(cfg.HaulingServiceSecret),
		TaxiURL:        cfg.TaxiServiceURL,
		TaxiSecret:     []byte(cfg.TaxiServiceSecret),
		DispatchURL:    cfg.DispatchServiceURL,
		DispatchSecret: []byte(cfg.DispatchServiceSecret),
	})

	// Nil when Fleet is unconfigured, which is a working configuration: the
	// organization snapshot is a reporting dimension, not a prerequisite for
	// opening a case.
	corporate := supportclients.NewFleetOrganizationResolver(
		cfg.FleetPartnerServiceURL, []byte(cfg.FleetPartnerServiceSecret), 3*time.Second)

	service := supportusecases.NewSupportService(repo, supportusecases.Options{
		Notifier:            notifier,
		Identity:            identity,
		Corporate:           corporate,
		ResolutionPublisher: supportclients.NewCommerceResolutionClient(cfg.VendorServiceURL, cfg.VendorServiceSecret),
	})

	customerSecret := []byte(cfg.CustomerAccessTokenSecret)
	serviceSecrets := serviceauth.ParseSecrets(cfg.ServiceSecrets)
	providerGroups := []supporthttp.ProviderGroup{
		{Prefix: "/provider", Secret: []byte(cfg.HaulingProviderTokenSecret), Role: "truck_provider", Service: "hauling"},
		{Prefix: "/taxi-provider", Secret: []byte(cfg.TaxiProviderTokenSecret), Role: "taxi_provider", Service: "taxi"},
		{Prefix: "/dispatch-provider", Secret: []byte(cfg.DispatchProviderTokenSecret), Role: "dispatch_provider", Service: "dispatch"},
	}

	// Section 15: owner context reads and action routing. Each owner base is
	// origin plus that owner's own API base -- the services do not share one.
	var networkHandler *supporthttp.NetworkContextHandler
	workers := []func(context.Context) error{service.RunCaseOutbox}
	if cfg.NetworkContextEnabled {
		registry := supportclients.NewContextRegistryFromConfig(
			joinBase(cfg.TaxiServiceURL, "/api/v1/taxi"), cfg.TaxiServiceSecret,
			joinBase(cfg.DispatchServiceURL, "/api/v1/dispatch"), cfg.DispatchServiceSecret,
			joinBase(cfg.HaulingServiceURL, "/api/v1/hauling"), cfg.HaulingServiceSecret,
			3*time.Second)
		options := supportusecases.NetworkContextOptions{
			Support: service, Store: supportrepositories.NewNetworkContextStore(db), Contexts: registry,
			Notifier: notifier.Transport(),
		}
		if escalation := supportclients.NewHaulingRecoveryEscalation(
			joinBase(cfg.HaulingServiceURL, "/api/v1/hauling"), cfg.HaulingServiceSecret,
			5*time.Second); escalation.Configured() {
			options.HaulingRecovery = escalation
		}
		network := supportusecases.NewNetworkContextService(options)
		networkHandler = supporthttp.NewNetworkContextHandler(network)
		workers = append(workers, func(runCtx context.Context) error {
			// Re-sends action requests whose earlier attempt was ambiguous.
			// Each retry is the same operation to the owner, so it replays.
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return nil
				case <-ticker.C:
					if _, err := network.DeliverPending(runCtx, 20); err != nil {
						logging.Error("support-actions", "deliver pending: %v", err)
					}
				}
			}
		})
	}

	// notificationClient brokers app realtime/feed/device access to
	// notification-service. Empty BaseURL skips the proxy routes (poll fallback).
	notificationClient := notifications.Client{
		BaseURL:     cfg.NotificationURL,
		ServiceName: "support-dispute-service",
		Secret:      []byte(cfg.NotificationSecret),
	}

	serviceapp.Run(serviceapp.Options{
		Name:        "support-dispute-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/support-disputes",
		Capabilities: []string{
			"complaints (customer, taxi, dispatch, hauling)",
			"evidence collection",
			"dispute escalation + admin resolution events (no ledger authority)",
			"typed Commerce cases + durable resolution outbox",
			"support chat (realtime + unread)",
			"categories + FAQ self-service",
			"emergency SOS",
			"cross-service identity enrichment",
		},
		ReadyChecks: []func(context.Context) error{
			db.Ping,
		},
		Workers: workers,
		Register: func(group *gin.RouterGroup) {
			supporthttp.RegisterRoutes(group, service, customerSecret, providerGroups, serviceSecrets)
			notificationhttp.RegisterNotificationRoutes(group, notificationClient, customerSecret, providerGroups)
			supporthttp.RegisterNetworkContextRoutes(group, networkHandler, customerSecret,
				providerGroups, serviceSecrets)
		},
	})
}

// joinBase appends an API base to a bare origin, and yields "" when the origin
// is unset so an unconfigured owner stays unconfigured rather than becoming a
// relative path.
func joinBase(origin, apiBase string) string {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin == "" {
		return ""
	}
	return origin + apiBase
}
