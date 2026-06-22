package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/support-dispute-service/internal/config"
	"cosmicforge/logistics/services/support-dispute-service/internal/database"
	supporthttp "cosmicforge/logistics/services/support-dispute-service/internal/features/support/http"
	supportrepositories "cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	"cosmicforge/logistics/shared/go/logging"
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
	service := supportusecases.NewSupportService(repo)

	customerSecret := []byte(cfg.CustomerAccessTokenSecret)
	serviceSecrets := serviceauth.ParseSecrets(cfg.ServiceSecrets)

	serviceapp.Run(serviceapp.Options{
		Name:        "support-dispute-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/support-disputes",
		Capabilities: []string{
			"complaints (customer, taxi, dispatch, hauling)",
			"evidence collection",
			"dispute escalation",
			"admin resolution",
		},
		ReadyChecks: []func(context.Context) error{
			db.Ping,
		},
		Register: func(group *gin.RouterGroup) {
			supporthttp.RegisterRoutes(group, service, customerSecret, serviceSecrets)
		},
	})
}
