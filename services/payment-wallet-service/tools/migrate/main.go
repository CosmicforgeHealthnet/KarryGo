package main

import (
	"context"
	"cosmicforge/logistics/services/payment-wallet-service/internal/config"
	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"github.com/joho/godotenv"
	"log"
	"time"
)

func main() {
	_ = godotenv.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := database.NewPool(ctx, config.Load().DatabaseURL)
	if err != nil {
		log.Fatal("Database connection failed")
	}
	defer pool.Close()
	if err := database.ApplyMigrations(ctx, pool, "migrations"); err != nil {
		log.Fatalf("Wallet migration failed: %v", err)
	}
	log.Print("Wallet migrations applied")
}
