package main

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/analytics-service/internal/config"
	"cosmicforge/logistics/services/analytics-service/internal/database"
	commercehttp "cosmicforge/logistics/services/analytics-service/internal/features/commerceanalytics/http"
	"cosmicforge/logistics/services/analytics-service/internal/features/commerceanalytics/repositories"
	"cosmicforge/logistics/services/analytics-service/internal/features/commerceanalytics/usecases"
	networkhttp "cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/http"
	networkrepositories "cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/repositories"
	networkusecases "cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/usecases"
	"cosmicforge/logistics/shared/go/ratelimit"
	"cosmicforge/logistics/shared/go/redisx"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	redisClient, err := redisx.NewClient(ctx, cfg.Redis)
	if err != nil {
		panic(err)
	}
	defer redisClient.Close()

	if cfg.Migration {
		migCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := database.ApplyMigrations(migCtx, db); err != nil {
			cancel()
			panic(err)
		}
		cancel()
	}

	repo := repositories.NewRepository(db)
	commerceService := usecases.NewService(repo)
	replayCache := commercehttp.NewReplayCache(10 * time.Minute)
	handler := commercehttp.NewHandler(
		commerceService,
		serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
		strings.Split(cfg.AllowedCallers, ","),
	).WithReplayCache(replayCache).WithReadRateLimit(ratelimit.New(redisClient, cfg.ReadRatePrefix), cfg.ReadRateMax, cfg.ReadRateWindow)

	// Section 13. The network handler shares the commerce handler's verifier
	// and replay cache, so a nonce spent on one surface is spent on both.
	verifier := serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute)
	networkHandler := networkhttp.NewHandler(networkhttp.Options{
		Service:       networkusecases.NewService(networkrepositories.NewRepository(db)),
		Verifier:      verifier,
		Nonces:        replayCache,
		IngestCallers: strings.Split(cfg.NetworkEventCallers, ","),
		ReadCallers:   strings.Split(cfg.NetworkReadCallers, ","),
		AdminCaller:   cfg.AdminCaller,
	})

	serviceapp.Run(serviceapp.Options{
		Name:        "analytics-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/analytics",
		Capabilities: []string{
			"reports",
			"metrics",
			"revenue dashboards",
			"service performance dashboards",
			"commerce operational analytics",
			"vendor reliability scoring",
			"demand and stock-out intelligence",
			"future-disabled order guarantee evidence",
			"network analytics (advisory)",
		},
		ReadyChecks: []func(context.Context) error{db.Ping, func(ctx context.Context) error { return redisClient.Ping(ctx).Err() }},
		Register: func(group *gin.RouterGroup) {
			commercehttp.RegisterRoutes(group, handler)
			networkhttp.RegisterAdminRoutes(group, networkHandler)
		},
		RegisterRoot: func(router *gin.Engine) {
			commercehttp.RegisterInternalRoutes(router, handler)
			networkhttp.RegisterInternalRoutes(router, networkHandler)
		},
	})
}
