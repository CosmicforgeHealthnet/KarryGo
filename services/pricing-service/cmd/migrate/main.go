// Command migrate applies pricing-service's migrations and exits. The local
// bootstrap uses it; deployed services can equally set PRICING_MIGRATION.
// Applied migrations are tracked, so running it again is a no-op.
package main

import (
	"context"
	"time"

	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/pricing-service/internal/config"
	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/shared/go/logging"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("migrate", "connect: %v", err)
	}
	defer pool.Close()
	if err := database.ApplyMigrations(ctx, pool, "migrations"); err != nil {
		logging.Fatal("migrate", "apply pricing-service migrations: %v", err)
	}
	logging.Success("migrate", "pricing-service migrations are up to date")
}
