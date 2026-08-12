package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ApplyMigrations(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS hauling_schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create migration tracker: %w", err)
	}
	entries, err := os.ReadDir("migrations")
	if err != nil {
		return err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") || strings.HasSuffix(entry.Name(), ".down.sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var applied bool
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hauling_schema_migrations WHERE name=$1)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("read migration tracker for %s: %w", name, err)
		}
		if applied {
			continue
		}
		migrationPath := filepath.Join("migrations", name)
		migration, err := os.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", migrationPath, err)
		}
		tx, err := db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", migrationPath, err)
		}
		if _, err := tx.Exec(ctx, string(migration)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", migrationPath, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO hauling_schema_migrations(name) VALUES($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", migrationPath, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", migrationPath, err)
		}
	}

	return nil
}
