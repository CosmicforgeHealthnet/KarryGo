package persistence

import (
	"context"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/services/admin-backoffice-service/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"testing"
)

func TestRuntimeRoleGuardAndGrants(t *testing.T) {
	root := testutil.Database(t)
	ctx := context.Background()
	owner := &Store{Pool: root}
	if owner.ValidateRuntime(ctx) == nil {
		t.Fatal("migration owner accepted as runtime")
	}
	role := "admin_runtime_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := root.Exec(ctx, `CREATE ROLE `+role+` NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(ctx, `DROP OWNED BY `+role); _, _ = root.Exec(ctx, `DROP ROLE `+role) })
	var schema string
	if err := root.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	grantBytes, err := os.ReadFile("../../../../deploy/platform/bin/admin-db-grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	grants := strings.ReplaceAll(string(grantBytes), `:"runtime_role"`, pgx.Identifier{role}.Sanitize())
	grants = strings.ReplaceAll(grants, "SCHEMA public", "SCHEMA "+pgx.Identifier{schema}.Sanitize())
	if _, err = root.Exec(ctx, grants); err != nil {
		t.Fatal(err)
	}
	cfg := root.Config()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error { _, err := c.Exec(ctx, `SET ROLE `+role); return err }
	runtime, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	s := &Store{Pool: runtime}
	if err = s.ValidateRuntime(ctx); err != nil {
		t.Fatal("least privilege role refused", err)
	}
	if err = s.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Audit(ctx, Audit{Correlation: uuid.NewString(), Outcome: "TEST"}); err != nil {
		t.Fatal("runtime cannot append audit", err)
	}
	for _, sql := range []string{`UPDATE admin_audit_entries SET outcome='forged'`, `DELETE FROM admin_audit_entries`, `TRUNCATE admin_audit_entries`, `ALTER TABLE admin_audit_entries DISABLE TRIGGER ALL`, `INSERT INTO admin_revocations(kind,reference) VALUES('staff','forged')`, `SELECT * FROM admin_audit_entries`, `CREATE TABLE forged_domain_state(id int)`} {
		if _, err = runtime.Exec(ctx, sql); err == nil {
			t.Fatalf("runtime privilege escaped: %s", sql)
		}
	}
}

func TestMigrationFailureRollsBackAndDownIsIrreversible(t *testing.T) {
	root := testutil.Database(t)
	ctx := context.Background()
	down, err := os.ReadFile("../../migrations/001_bridge_down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = root.Exec(ctx, string(down)); err == nil {
		t.Fatal("down migration destroyed evidence")
	}
	schema := "admin_broken_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = root.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer root.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`)
	cfg := root.Config()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `CREATE TABLE admin_delegated_operations(id int)`); err != nil {
		t.Fatal(err)
	}
	if err = migrations.Apply(ctx, pool); err == nil {
		t.Fatal("conflicting migration succeeded")
	}
	var absent bool
	if err = pool.QueryRow(ctx, `SELECT to_regclass('admin_remote_principals') IS NULL AND to_regclass('admin_schema_versions') IS NULL`).Scan(&absent); err != nil || !absent {
		t.Fatal("partial migration committed", err)
	}
}
