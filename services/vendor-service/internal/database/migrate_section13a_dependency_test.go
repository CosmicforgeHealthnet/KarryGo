package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection13AMigrationRollbackDependencyChain(
	t *testing.T,
) {
	dsn := os.Getenv(
		"VENDOR_SERVICE_TEST_DATABASE_URL",
	)
	if dsn == "" {
		t.Skip(
			"VENDOR_SERVICE_TEST_DATABASE_URL not configured",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}

	// Migration files contain multiple SQL statements.
	config.ConnConfig.DefaultQueryExecMode =
		pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(
		ctx,
		config,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Every schema mutation performed by this test is rolled
	// back so other database-package tests receive the same
	// fully migrated database.
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil &&
			rollbackErr != pgx.ErrTxClosed {
			t.Errorf(
				"rollback dependency test transaction: %v",
				rollbackErr,
			)
		}
	}()

	migrationsDir :=
		filepath.Join(
			"..",
			"..",
			"migrations",
		)

	downMigrations := []string{
		"000053_vendor_taxonomy_governance_workflow.down.sql",
		"000052_vendor_taxonomy_mapping_engine.down.sql",
		"000051_vendor_taxonomy_tags_dietary_claims.down.sql",
		"000050_vendor_taxonomy_mapping_governance.down.sql",
		"000049_vendor_canonical_taxonomy_seed_publication.down.sql",
		"000048_vendor_canonical_taxonomy_vocabulary.down.sql",
		"000047_vendor_canonical_taxonomy_foundation.down.sql",
	}

	for _, name := range downMigrations {
		path := filepath.Join(
			migrationsDir,
			name,
		)

		sqlBytes, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf(
				"read %s: %v",
				name,
				readErr,
			)
		}

		if _, execErr := tx.Exec(
			ctx,
			string(sqlBytes),
		); execErr != nil {
			t.Fatalf(
				"execute dependency-aware DOWN %s: %v",
				name,
				execErr,
			)
		}
	}

	section13ATables := []string{
		"canonical_taxonomy_namespaces",
		"canonical_taxonomy_versions",
		"canonical_taxonomy_facets",
		"canonical_taxonomy_nodes",
		"canonical_taxonomy_commands",
		"canonical_taxonomy_change_history",
		"canonical_taxonomy_publication_history",
	}

	for _, table := range section13ATables {
		var exists bool

		query := fmt.Sprintf(
			`SELECT to_regclass('public.%s') IS NOT NULL`,
			table,
		)

		if err = tx.QueryRow(
			ctx,
			query,
		).Scan(&exists); err != nil {
			t.Fatalf(
				"check rollback table %s: %v",
				table,
				err,
			)
		}

		if exists {
			t.Fatalf(
				"dependency-aware Section 13A rollback left table %s",
				table,
			)
		}
	}
}
