// Command seed applies seeds/dev_seed.sql against the configured database.
// Re-runnable -- every insert in that file is idempotent (fixed UUIDs +
// ON CONFLICT DO NOTHING), so running this against a database that already
// has some or all of the seeded rows is always a safe no-op for what
// already exists.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/taxi-service/internal/config"
	"cosmicforge/logistics/services/taxi-service/internal/database"
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
	fmt.Println("  5 Lagos providers, one per addendum compliance dimension:")
	fmt.Println("    +2348099990001  Ada Okafor        -- fully clean/ready, has 1 completed + 1 in-progress trip")
	fmt.Println("    +2348099990002  Bola Suspended     -- account_status=suspended")
	fmt.Println("    +2348099990003  Chidi IdentityFailed -- identity_verification_status=failed")
	fmt.Println("    +2348099990004  Dupe VehiclePending  -- vehicle_verification_status=pending")
	fmt.Println("    +2348099990005  Emeka DocsExpired    -- document_compliance_status=expired")
	fmt.Println()
	fmt.Println("  5 bookings on the clean provider's customer: completed, cancelled,")
	fmt.Println("  unmatched, in_progress, and scheduled (tomorrow, not yet promoted).")
	fmt.Println()
	fmt.Println("  Log in via OTP (TAXI_OTP_DEBUG=true returns the code in the response).")
	fmt.Println("  Only the clean provider can actually go online -- run ./cmd/online to")
	fmt.Println("  put them into Redis without needing a driver app.")
	fmt.Println()
	fmt.Println("  Re-runnable -- inserts are idempotent (fixed UUIDs + ON CONFLICT DO NOTHING).")
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
	fmt.Printf("\033[1;36m[taxi-seed] %-10s\033[0m "+format+"\n", append([]any{label}, args...)...)
}

func success(label, format string, args ...any) {
	fmt.Printf("\033[1;32m[taxi-seed] %-10s\033[0m "+format+"\n", append([]any{label}, args...)...)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[1;31m[taxi-seed] fatal\033[0m "+format+"\n", args...)
	os.Exit(1)
}
