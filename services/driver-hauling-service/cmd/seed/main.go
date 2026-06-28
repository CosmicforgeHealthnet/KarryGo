package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/hauling-service/internal/config"
	"cosmicforge/logistics/services/hauling-service/internal/database"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("connect to database: %v", err)
	}
	defer db.Close()

	notice("database", "connected to %s", cfg.DatabaseURL)

	if err := applySeed(ctx, db, "seeds/dev_seed.sql"); err != nil {
		fatal("apply seed: %v", err)
	}

	success("seed", "dev_seed.sql applied")
	fmt.Println()
	fmt.Println("  6 Lagos providers, each fully onboarded with one ACTIVE truck of every")
	fmt.Println("  bookable type (van, flatbed, refrigerated, container, tipper) — so any")
	fmt.Println("  account matches ANY Lagos booking. Each also has 1 completed trip.")
	fmt.Println()
	fmt.Println("  Phone logins (log in via OTP; HAULING_OTP_DEBUG=true returns the code):")
	fmt.Println("    +2348011111001  Emeka Okonkwo")
	fmt.Println("    +2348011111002  Biodun Adeyemi")
	fmt.Println("    +2348011111003  Chidi Eze")
	fmt.Println()
	fmt.Println("  Email logins (log in via OTP; HAULING_OTP_DEBUG=true returns the code):")
	fmt.Println("    tunde@karrygo.dev    Tunde Bakare")
	fmt.Println("    ngozi@karrygo.dev    Ngozi Eze")
	fmt.Println("    samuel@karrygo.dev   Samuel Adeniyi")
	fmt.Println()
	fmt.Println("  After login: tap \"Go Online\" on the home screen, then create a Lagos")
	fmt.Println("  booking from the customer app to receive a request.")
	fmt.Println()
	fmt.Println("  Re-runnable — inserts are idempotent (fixed UUIDs + ON CONFLICT DO NOTHING).")
}

func applySeed(ctx context.Context, db *pgxpool.Pool, path string) error {
	sql, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if _, err := db.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("exec %s: %w", path, err)
	}
	return nil
}

func notice(label, format string, args ...any) {
	fmt.Printf("\033[1;36m[hauling-seed] %-12s\033[0m "+format+"\n", append([]any{label}, args...)...)
}

func success(label, format string, args ...any) {
	fmt.Printf("\033[1;32m[hauling-seed] %-12s\033[0m "+format+"\n", append([]any{label}, args...)...)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[1;31m[hauling-seed] fatal\033[0m "+format+"\n", args...)
	os.Exit(1)
}
