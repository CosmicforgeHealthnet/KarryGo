package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/notification-service/internal/config"
	"cosmicforge/logistics/services/notification-service/internal/database"
)

// Seeds notification_templates with the platform event-type templates. Idempotent
// (ON CONFLICT upsert), so it is safe to re-run after editing copy.
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

	if err := applySeed(ctx, db, "seeds/dev_templates.sql"); err != nil {
		fatal("apply seed: %v", err)
	}

	success("seed", "dev_templates.sql applied")
	fmt.Println()
	fmt.Println("  Seeded notification templates for booking and payment events.")
	fmt.Println("  Run again at any time — the upsert keeps copy in sync.")
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
	fmt.Printf("\033[1;36m[notification-seed] %-12s\033[0m "+format+"\n", append([]any{label}, args...)...)
}

func success(label, format string, args ...any) {
	fmt.Printf("\033[1;32m[notification-seed] %-12s\033[0m "+format+"\n", append([]any{label}, args...)...)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[1;31m[notification-seed] fatal\033[0m "+format+"\n", args...)
	os.Exit(1)
}
