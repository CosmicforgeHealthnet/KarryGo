package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

func main() {
	dsn := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "FLEET_PARTNER_TEST_DATABASE_URL is required")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := database.NewPool(ctx, dsn, database.PoolOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect Fleet test database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := database.ApplyMigrations(ctx, pool, "migrations"); err != nil {
		fmt.Fprintf(os.Stderr, "apply Fleet migrations: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Fleet test database migrations applied")
}
