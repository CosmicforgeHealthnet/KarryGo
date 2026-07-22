// Command seed applies seeds/dev_seed.sql: dev dispatch riders that can log
// into the dispatch_provider app and go online immediately (verified provider,
// verified_to_go_online, verified active bike, approved licence).
//
// Run from the service directory: go run ./cmd/seed
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/database"
)

func main() {
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		fatal("load config: %v", err)
	}

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
	fmt.Println("  2 Lagos dispatch riders, fully verified and cleared to go online")
	fmt.Println("  (verified provider + verified active bike + approved licence).")
	fmt.Println()
	fmt.Println("  Phone logins (POST /api/v1/auth/login/start):")
	fmt.Println("    +2348077770001  Emeka Dispatch  (motorcycle, LAG-101-DR)")
	fmt.Println("    +2348077770002  Bisi Rider      (dispatch_bike, LAG-102-DR)")
	fmt.Println()
	fmt.Println("  NOTE: with DISPATCH_RIDER_DEBUG_OTP=true the OTP code is printed in")
	fmt.Println("  the dispatch SERVICE LOGS (\"development dispatch rider login otp ...\").")
	fmt.Println("  Unlike hauling, it is NOT returned in the HTTP response.")
	fmt.Println()
	fmt.Println("  After login: go online from the rider app (or PATCH")
	fmt.Println("  /api/v1/provider/availability), then create a Lagos bike delivery from")
	fmt.Println("  the customer app to receive a request.")
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
	fmt.Printf("\033[1;36m[dispatch-seed] %-12s\033[0m "+format+"\n", append([]any{label}, args...)...)
}

func success(label, format string, args ...any) {
	fmt.Printf("\033[1;32m[dispatch-seed] %-12s\033[0m "+format+"\n", append([]any{label}, args...)...)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[1;31m[dispatch-seed] fatal\033[0m "+format+"\n", args...)
	os.Exit(1)
}
