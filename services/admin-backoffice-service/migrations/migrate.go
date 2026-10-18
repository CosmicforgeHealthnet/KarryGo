package migrations

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed 001_bridge.sql
var Schema string

// Apply serializes schema installation across replicas and rolls back on failure.
// Use a migration role; the runtime role must not own the evidence tables.
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(81102501)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS admin_schema_versions(version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_schema_versions WHERE version=1)`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec(ctx, Schema); err != nil {
			return fmt.Errorf("bridge schema transaction failed: %w", err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO admin_schema_versions(version) VALUES(1)`); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
