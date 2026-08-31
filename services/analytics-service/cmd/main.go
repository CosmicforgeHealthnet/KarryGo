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
	handler := commercehttp.NewHandler(
		commerceService,
		serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute),
		strings.Split(cfg.AllowedCallers, ","),
	).WithReadRateLimit(ratelimit.New(redisClient, cfg.ReadRatePrefix), cfg.ReadRateMax, cfg.ReadRateWindow)

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
		},
		ReadyChecks: []func(context.Context) error{db.Ping, func(ctx context.Context) error { return redisClient.Ping(ctx).Err() }},
		Register: func(group *gin.RouterGroup) {
			commercehttp.RegisterRoutes(group, handler)
		},
		RegisterRoot: func(router *gin.Engine) {
			commercehttp.RegisterInternalRoutes(router, handler)
		},
	})
}
