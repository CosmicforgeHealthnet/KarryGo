package main

import (
	"context"
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
	"cosmicforge/logistics/shared/go/walletclient"
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

	var payments *walletclient.Client
	if cfg.PaymentURL != "" && cfg.PaymentSecret != "" {
		payments = &walletclient.Client{
			BaseURL:     cfg.PaymentURL,
			ServiceName: "support-dispute-service",
			Secret:      []byte(cfg.PaymentSecret),
		}
	}

	service := supportusecases.NewSupportService(repo, supportusecases.Options{
		Notifier: notifier,
		Identity: identity,
		Payments: payments,
	})

	customerSecret := []byte(cfg.CustomerAccessTokenSecret)
	serviceSecrets := serviceauth.ParseSecrets(cfg.ServiceSecrets)
	providerGroups := []supporthttp.ProviderGroup{
		{Prefix: "/provider", Secret: []byte(cfg.HaulingProviderTokenSecret), Role: "truck_provider", Service: "hauling"},
		{Prefix: "/taxi-provider", Secret: []byte(cfg.TaxiProviderTokenSecret), Role: "taxi_provider", Service: "taxi"},
		{Prefix: "/dispatch-provider", Secret: []byte(cfg.DispatchProviderTokenSecret), Role: "dispatch_provider", Service: "dispatch"},
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
			"dispute escalation + admin resolution + refund hook",
			"support chat (realtime + unread)",
			"categories + FAQ self-service",
			"emergency SOS",
			"cross-service identity enrichment",
		},
		ReadyChecks: []func(context.Context) error{
			db.Ping,
		},
		Register: func(group *gin.RouterGroup) {
			supporthttp.RegisterRoutes(group, service, customerSecret, providerGroups, serviceSecrets)
			notificationhttp.RegisterNotificationRoutes(group, notificationClient, customerSecret, providerGroups)
		},
	})
}
