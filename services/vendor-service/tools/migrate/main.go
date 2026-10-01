package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/database"
)

const migrationLockID int64 = 0x4b47564d494752 // "KGVMIGR"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	databaseURL := strings.TrimSpace(os.Getenv("VENDOR_DATABASE_URL"))
	if databaseURL == "" {
		log.Fatal("vendor migration: VENDOR_DATABASE_URL is required")
	}

	migrationsDir := strings.TrimSpace(os.Getenv("VENDOR_MIGRATIONS_DIR"))
	if migrationsDir == "" {
		migrationsDir = "/srv/vendor-service/migrations"
	}

	timeout, err := migrationTimeout(os.Getenv("VENDOR_MIGRATION_TIMEOUT"))
	if err != nil {
		log.Fatalf("vendor migration: %v", err)
	}
	migrationCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pool, err := database.NewPool(migrationCtx, databaseURL)
	if err != nil {
		log.Fatalf("vendor migration: open database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(migrationCtx); err != nil {
		log.Fatalf("vendor migration: database readiness: %v", err)
	}

	lockConn, err := pool.Acquire(migrationCtx)
	if err != nil {
		log.Fatalf("vendor migration: acquire advisory-lock connection: %v", err)
	}
	defer lockConn.Release()
	if _, err := lockConn.Exec(migrationCtx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		log.Fatalf("vendor migration: acquire advisory lock: %v", err)
	}
	defer func() {
		unlockCtx, unlockCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer unlockCancel()
		if _, err := lockConn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, migrationLockID); err != nil {
			log.Printf("vendor migration: advisory unlock failed: %v", err)
		}
	}()

	if err := database.ApplyMigrations(migrationCtx, pool, migrationsDir); err != nil {
		log.Fatalf("vendor migration: apply: %v", err)
	}
	if err := database.MigrationsCurrent(migrationCtx, pool, migrationsDir); err != nil {
		log.Fatalf("vendor migration: verify tip: %v", err)
	}

	var applied int
	if err := pool.QueryRow(migrationCtx,
		`SELECT count(*) FROM vendor_service_schema_migrations`,
	).Scan(&applied); err != nil {
		log.Fatalf("vendor migration: read applied count: %v", err)
	}
	log.Printf("vendor migration: success; applied migration records=%d", applied)
}

func migrationTimeout(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 30 * time.Minute, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("VENDOR_MIGRATION_TIMEOUT must be a duration: %w", err)
	}
	if timeout < time.Minute || timeout > 2*time.Hour {
		return 0, fmt.Errorf("VENDOR_MIGRATION_TIMEOUT must be between 1m and 2h")
	}
	return timeout, nil
}
