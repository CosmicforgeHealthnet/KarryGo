// Package database provides vendor-service's Postgres connection pool and its
// self-contained migration runner.
//
// Migrations use the paired up/down convention (000001_name.up.sql /
// 000001_name.down.sql) with a service-local tracking table, so apply and
// rollback are both deterministic and query-driven from the tracking table's
// actual state rather than from a caller-supplied step count. This
// deliberately avoids a known bug class seen elsewhere in the platform: a
// rollback helper that assumes a fixed number of migrations exist above a
// given point breaks the moment a later migration is added on top, and can
// leave a shared test database mid-rollback. See
// docs/architecture/12_MIGRATION_OWNERSHIP.md.
package database

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationsTable = `
CREATE TABLE IF NOT EXISTS vendor_service_schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
)`

// Migration is one discovered migration file pair.
type Migration struct {
	Version  string // file stem, e.g. "000001_vendor_service_foundation"
	Number   int    // parsed numeric prefix
	UpFile   string
	DownFile string
}

// ErrNoDownMigration is returned when a rollback is requested for a migration
// that has no down file.
type ErrNoDownMigration struct{ Version string }

func (e *ErrNoDownMigration) Error() string {
	return fmt.Sprintf("migration %s has no down file", e.Version)
}

// ListMigrations discovers migrations in dir ordered by numeric prefix.
// Duplicate numeric prefixes are a hard error: two migrations claiming the
// same slot means the applied order is undefined across environments.
func ListMigrations(dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(os.DirFS(dir), ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	byVersion := map[string]*Migration{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}

		stem := strings.TrimSuffix(name, ".sql")
		direction := "up"
		switch {
		case strings.HasSuffix(stem, ".up"):
			stem = strings.TrimSuffix(stem, ".up")
		case strings.HasSuffix(stem, ".down"):
			stem = strings.TrimSuffix(stem, ".down")
			direction = "down"
		}

		number, err := parseNumber(stem)
		if err != nil {
			return nil, err
		}

		migration := byVersion[stem]
		if migration == nil {
			migration = &Migration{Version: stem, Number: number}
			byVersion[stem] = migration
		}
		if direction == "up" {
			migration.UpFile = name
		} else {
			migration.DownFile = name
		}
	}

	migrations := make([]Migration, 0, len(byVersion))
	seenNumbers := map[int]string{}
	for _, migration := range byVersion {
		if migration.UpFile == "" {
			return nil, fmt.Errorf("migration %s has a down file with no up file", migration.Version)
		}
		if other, duplicate := seenNumbers[migration.Number]; duplicate {
			return nil, fmt.Errorf("duplicate migration number %d: %s and %s", migration.Number, other, migration.Version)
		}
		seenNumbers[migration.Number] = migration.Version
		migrations = append(migrations, *migration)
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Number < migrations[j].Number })
	return migrations, nil
}

func parseNumber(stem string) (int, error) {
	prefix, _, ok := strings.Cut(stem, "_")
	if !ok {
		return 0, fmt.Errorf("migration %q does not start with a numeric prefix", stem)
	}
	number, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("migration %q has a non-numeric prefix", stem)
	}
	return number, nil
}

// ApplyMigrations applies every unapplied migration from dir in numeric order.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if _, err := pool.Exec(ctx, migrationsTable); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	migrations, err := ListMigrations(dir)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM vendor_service_schema_migrations WHERE version = $1)`, migration.Version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", migration.Version, err)
		}
		if exists {
			continue
		}

		sqlBytes, err := os.ReadFile(filepath.Join(dir, migration.UpFile))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", migration.Version, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", migration.Version, err)
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", migration.Version, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO vendor_service_schema_migrations (version) VALUES ($1)`, migration.Version,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", migration.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", migration.Version, err)
		}
	}
	return nil
}

// RollbackLastMigration applies the down file of the most recently applied
// migration, determined by querying the tracking table (not a caller-supplied
// count). Used by migration replay tests and operational rollback.
func RollbackLastMigration(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	migrations, err := ListMigrations(dir)
	if err != nil {
		return err
	}

	var applied []Migration
	for _, migration := range migrations {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM vendor_service_schema_migrations WHERE version = $1)`, migration.Version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", migration.Version, err)
		}
		if exists {
			applied = append(applied, migration)
		}
	}
	if len(applied) == 0 {
		return fmt.Errorf("no applied migration to roll back")
	}

	last := applied[len(applied)-1]
	if last.DownFile == "" {
		return &ErrNoDownMigration{Version: last.Version}
	}

	sqlBytes, err := os.ReadFile(filepath.Join(dir, last.DownFile))
	if err != nil {
		return fmt.Errorf("read down migration %s: %w", last.Version, err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rollback %s: %w", last.Version, err)
	}
	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("apply down migration %s: %w", last.Version, err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM vendor_service_schema_migrations WHERE version = $1`, last.Version,
	); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("unrecord migration %s: %w", last.Version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rollback %s: %w", last.Version, err)
	}
	return nil
}

// MigrationsCurrent reports whether every migration in dir has been applied.
func MigrationsCurrent(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	migrations, err := ListMigrations(dir)
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM vendor_service_schema_migrations WHERE version = $1)`, migration.Version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", migration.Version, err)
		}
		if !exists {
			return fmt.Errorf("migration %s has not been applied", migration.Version)
		}
	}
	return nil
}
