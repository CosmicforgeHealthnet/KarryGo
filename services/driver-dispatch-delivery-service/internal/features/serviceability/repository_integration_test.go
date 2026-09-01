package serviceability

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrationApplyRollbackReapplyAndIdempotencyConstraint(t *testing.T) {
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL not set; Sections 4-6 PostgreSQL certification is infrastructure-gated")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	root := filepath.Join("..", "..", "..", "migrations")
	up, err := os.ReadFile(filepath.Join(root, "000044_delivery_serviceability_foundation.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	// Preserve coverage of recreation against the current migrated database,
	// but undo that recreation before testing historical rollback boundaries.
	if _, err = tx.Exec(ctx, "SAVEPOINT current_schema_recreation"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(up)); err != nil {
		t.Fatalf("current-schema recreation: %v", err)
	}
	if _, err = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT current_schema_recreation"); err != nil {
		t.Fatal(err)
	}
	// Exercise recreation and rollback at the foundation's actual schema
	// boundary, without dropping tables referenced by later migrations. The
	// schema and all its changes are removed by the transaction rollback.
	schema := "serviceability_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = tx.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "SET LOCAL search_path TO "+schema+",public"); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(root, "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if filepath.Base(path) > "000044_delivery_serviceability_foundation.up.sql" {
			break
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(path), err)
		}
	}
	down, err := os.ReadFile(filepath.Join(root, "000044_delivery_serviceability_foundation.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(up)); err != nil {
		t.Fatalf("fresh/current-schema apply: %v", err)
	}
	insert := `INSERT INTO dispatch_serviceability_quotes(id,customer_id,idempotency_key,request_fingerprint,pickup_latitude,pickup_longitude,dropoff_latitude,dropoff_longitude,vehicle_class,decision,policy_version,package_declaration,package_admission,expires_at) VALUES('dsq-test','9bcd1c37-edcc-4206-a2a8-37622f999999','key','fingerprint',6.5,3.3,6.6,3.4,'MOTORCYCLE','OPERATIONAL_REVIEW','v1','{}','{}',now()+interval '1 minute')`
	if _, err = tx.Exec(ctx, insert); err != nil {
		t.Fatalf("quote insert: %v", err)
	}
	if _, err = tx.Exec(ctx, "SAVEPOINT duplicate_guard"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, insert); err == nil {
		t.Fatal("duplicate customer idempotency key accepted")
	}
	if _, err = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT duplicate_guard"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if _, err = tx.Exec(ctx, string(up)); err != nil {
		t.Fatalf("reapply: %v", err)
	}
}
