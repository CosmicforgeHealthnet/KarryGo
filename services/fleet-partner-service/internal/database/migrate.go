package database

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrations use the paired up/down convention (000001_name.up.sql /
// 000001_name.down.sql) with a service-local tracking table so replay and
// rollback are deterministic.

const migrationsTable = `
CREATE TABLE IF NOT EXISTS fleet_partner_schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
)`

// ApplyMigrations applies every unapplied *.up.sql from dir in order.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if _, err := pool.Exec(ctx, migrationsTable); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	versions, err := listVersions(os.DirFS(dir))
	if err != nil {
		return err
	}

	for _, version := range versions {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM fleet_partner_schema_migrations WHERE version = $1)`, version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if exists {
			continue
		}

		sqlBytes, err := os.ReadFile(filepath.Join(dir, version+".up.sql"))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO fleet_partner_schema_migrations (version) VALUES ($1)`, version,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}
	}
	return nil
}

// RollbackLastMigration applies the down file of the most recently applied
// migration. Used by migration replay tests and operational rollback.
func RollbackLastMigration(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	var version string
	err := pool.QueryRow(ctx,
		`SELECT version FROM fleet_partner_schema_migrations ORDER BY version DESC LIMIT 1`,
	).Scan(&version)
	if err != nil {
		return fmt.Errorf("find last migration: %w", err)
	}

	sqlBytes, err := os.ReadFile(filepath.Join(dir, version+".down.sql"))
	if err != nil {
		return fmt.Errorf("read down migration %s: %w", version, err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rollback %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("apply down migration %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM fleet_partner_schema_migrations WHERE version = $1`, version,
	); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("unrecord migration %s: %w", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rollback %s: %w", version, err)
	}
	return nil
}

// MigrationsCurrent reports whether every migration in dir has been applied;
// used by the readiness endpoint.
func MigrationsCurrent(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	versions, err := listVersions(os.DirFS(dir))
	if err != nil {
		return err
	}
	for _, version := range versions {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM fleet_partner_schema_migrations WHERE version = $1)`, version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if !exists {
			return fmt.Errorf("migration %s has not been applied", version)
		}
	}
	return nil
}

func listVersions(dir fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(dir, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		versions = append(versions, strings.TrimSuffix(name, ".up.sql"))
	}
	sort.Strings(versions)
	return versions, nil
}
