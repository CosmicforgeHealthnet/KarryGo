package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVendorAuditAppendOnlyMigrationShape(t *testing.T) {
	up, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000108_vendor_audit_append_only.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000108_vendor_audit_append_only.down.sql"))
	if err != nil {
		t.Fatal(err)
	}

	upSQL := string(up)
	for _, required := range []string{
		"vendor_audit_log_append_only_guard",
		"BEFORE UPDATE OR DELETE",
		"ON vendor_audit_log",
		"ERRCODE = '55000'",
	} {
		if !strings.Contains(upSQL, required) {
			t.Errorf("up migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(upSQL), "DROP TABLE") {
		t.Error("up migration must not destroy audit data")
	}

	downSQL := string(down)
	for _, required := range []string{
		"DROP TRIGGER IF EXISTS vendor_audit_log_append_only_trg ON vendor_audit_log",
		"DROP FUNCTION IF EXISTS vendor_audit_log_append_only_guard()",
	} {
		if !strings.Contains(downSQL, required) {
			t.Errorf("down migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(downSQL), "DROP TABLE") || strings.Contains(strings.ToUpper(downSQL), "DELETE FROM") {
		t.Error("down migration must remove only the guard and preserve audit history")
	}
}

func TestVendorAuditLogDatabaseAppendOnly(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres append-only certification")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	if err := ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	insertAudit := func(t *testing.T, suffix string, action func() error) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		id := fmt.Sprintf("audit-append-only-%d-%s", time.Now().UnixNano(), suffix)
		if _, err := tx.Exec(ctx, `
			INSERT INTO vendor_audit_log
				(id, actor_type, action, aggregate_type, aggregate_id, reason)
			VALUES ($1, 'SYSTEM', 'SECURITY_TEST', 'VENDOR_AUDIT_LOG', $1, 'append-only certification')
		`, id); err != nil {
			t.Fatalf("insert audit row: %v", err)
		}
		if err := action(); err != nil {
			t.Fatal(err)
		}
	}

	insertAudit(t, "insert", func() error { return nil })

	for _, mutation := range []struct {
		name string
		sql  string
	}{
		{name: "update", sql: `UPDATE vendor_audit_log SET reason = 'rewritten' WHERE action = 'SECURITY_TEST'`},
		{name: "delete", sql: `DELETE FROM vendor_audit_log WHERE action = 'SECURITY_TEST'`},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			id := fmt.Sprintf("audit-append-only-%d-%s", time.Now().UnixNano(), mutation.name)
			if _, err := tx.Exec(ctx, `
				INSERT INTO vendor_audit_log
					(id, actor_type, action, aggregate_type, aggregate_id, reason)
				VALUES ($1, 'SYSTEM', 'SECURITY_TEST', 'VENDOR_AUDIT_LOG', $1, 'append-only certification')
			`, id); err != nil {
				t.Fatalf("insert audit row: %v", err)
			}
			if _, err := tx.Exec(ctx, strings.ReplaceAll(mutation.sql, "action = 'SECURITY_TEST'", "id = '"+id+"'")); err == nil {
				t.Fatalf("expected direct %s to be rejected", mutation.name)
			}
		})
	}
}
