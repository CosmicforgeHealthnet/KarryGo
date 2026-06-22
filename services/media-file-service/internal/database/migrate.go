package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ApplyMigrations(ctx context.Context, db *pgxpool.Pool) error {
	entries, err := os.ReadDir("migrations")
	if err != nil {
		return err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		migrationPath := filepath.Join("migrations", name)
		migration, err := os.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", migrationPath, err)
		}
		if _, err := db.Exec(ctx, string(migration)); err != nil {
			return fmt.Errorf("apply migration %s: %w", migrationPath, err)
		}
	}

	return nil
}
