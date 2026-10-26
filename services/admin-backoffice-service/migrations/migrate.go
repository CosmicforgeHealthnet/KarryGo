package migrations

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed 001_bridge.sql
var Schema string

//go:embed 002_audit_zone.sql
var zoneSchema string

//go:embed 003_policy_revisions.sql
var policySchema string

//go:embed 004_customer_review_operations.sql
var customerReviewSchema string

//go:embed 005_customer_work_item_operations.sql
var customerWorkItemSchema string

//go:embed 006_owner_command_operations.sql
var ownerCommandSchema string

//go:embed 007_hq_integration.sql
var hqIntegrationSchema string

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
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_schema_versions WHERE version=2)`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec(ctx, zoneSchema); err != nil {
			return fmt.Errorf("bridge zone migration: %w", err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO admin_schema_versions(version) VALUES(2)`); err != nil {
			return err
		}
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_schema_versions WHERE version=3)`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec(ctx, policySchema); err != nil {
			return fmt.Errorf("bridge policy migration: %w", err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO admin_schema_versions(version) VALUES(3)`); err != nil {
			return err
		}
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_schema_versions WHERE version=4)`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec(ctx, customerReviewSchema); err != nil {
			return fmt.Errorf("bridge customer review migration: %w", err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO admin_schema_versions(version) VALUES(4)`); err != nil {
			return err
		}
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_schema_versions WHERE version=5)`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec(ctx, customerWorkItemSchema); err != nil {
			return fmt.Errorf("bridge work item migration: %w", err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO admin_schema_versions(version)VALUES(5)`); err != nil {
			return err
		}
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_schema_versions WHERE version=6)`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec(ctx, ownerCommandSchema); err != nil {
			return fmt.Errorf("bridge owner command migration: %w", err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO admin_schema_versions(version)VALUES(6)`); err != nil {
			return err
		}
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_schema_versions WHERE version=7)`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec(ctx, hqIntegrationSchema); err != nil {
			return fmt.Errorf("bridge HQ integration migration: %w", err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO admin_schema_versions(version)VALUES(7)`); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
