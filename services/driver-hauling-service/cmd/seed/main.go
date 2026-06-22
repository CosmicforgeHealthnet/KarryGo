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
	fmt.Println("  Seed phone numbers (log in via OTP in debug mode):")
	fmt.Println("    +2348011111001  Emeka Okonkwo   — flatbed + container")
	fmt.Println("    +2348011111002  Biodun Adeyemi  — tipper + van")
	fmt.Println("    +2348011111003  Chidi Eze       — refrigerated + flatbed")
	fmt.Println()
	fmt.Println("  Seed emails (log in via OTP in debug mode — fully onboarded, online-ready):")
	fmt.Println("    tunde@karrygo.dev    Tunde Bakare    — flatbed + container")
	fmt.Println("    ngozi@karrygo.dev    Ngozi Eze       — tipper + van")
	fmt.Println("    samuel@karrygo.dev   Samuel Adeniyi  — refrigerated + flatbed")
	fmt.Println()
	fmt.Println("  Run again at any time — inserts are idempotent (ON CONFLICT DO NOTHING).")
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
