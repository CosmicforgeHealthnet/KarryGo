package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVerificationMigrationFilesExist(t *testing.T) {
	required := []string{
		"000008_create_verification_steps.up.sql",
		"000008_create_verification_steps.down.sql",
		"000009_create_verification_documents.up.sql",
		"000009_create_verification_documents.down.sql",
		"000010_create_face_checks.up.sql",
		"000010_create_face_checks.down.sql",
		"000011_create_verification_audit.up.sql",
		"000011_create_verification_audit.down.sql",
		"000012_expand_verification_audit_actions.up.sql",
		"000012_expand_verification_audit_actions.down.sql",
		"000013_add_fully_approved_audit.up.sql",
		"000013_add_fully_approved_audit.down.sql",
	}
	for _, name := range required {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(".", name)); err != nil {
				t.Fatalf("migration %s is missing: %v", name, err)
			}
		})
	}
}

func TestVerificationMigrationsApplyValidateAndRollback(t *testing.T) {
	databaseURL := os.Getenv("DISPATCH_VERIFICATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("set DISPATCH_VERIFICATION_TEST_DATABASE_URL or TEST_DATABASE_URL to run database migration checks")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()

	schema := "verification_migration_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema+", public"); err != nil {
		t.Fatalf("set search_path: %v", err)
	}

	upFiles := []string{
		"000004_create_providers.up.sql",
		"000005_create_emergency_contacts.up.sql",
		"000006_create_guarantors.up.sql",
		"000007_create_ratings.up.sql",
		"000008_create_verification_steps.up.sql",
		"000009_create_verification_documents.up.sql",
		"000010_create_face_checks.up.sql",
		"000011_create_verification_audit.up.sql",
		"000012_expand_verification_audit_actions.up.sql",
		"000013_add_fully_approved_audit.up.sql",
	}
	for _, name := range upFiles {
		execMigrationFile(t, ctx, conn, name)
	}

	assertUniqueProviderStep(t, ctx, conn)
	assertInvalidVerificationStepRejected(t, ctx, conn)
	assertInvalidVerificationStatusRejected(t, ctx, conn)
	assertVerificationDocumentIndexes(t, ctx, conn, schema)
	assertFaceCheckResultConstraint(t, ctx, conn)
	assertVerificationAuditIndexes(t, ctx, conn, schema)

	downFiles := []string{
		"000013_add_fully_approved_audit.down.sql",
		"000012_expand_verification_audit_actions.down.sql",
		"000011_create_verification_audit.down.sql",
		"000010_create_face_checks.down.sql",
		"000009_create_verification_documents.down.sql",
		"000008_create_verification_steps.down.sql",
	}
	for _, name := range downFiles {
		execMigrationFile(t, ctx, conn, name)
	}
	assertTableDropped(t, ctx, conn, schema, "verification_audit")
	assertTableDropped(t, ctx, conn, schema, "face_checks")
	assertTableDropped(t, ctx, conn, schema, "verification_documents")
	assertTableDropped(t, ctx, conn, schema, "verification_steps")
}

func execMigrationFile(t *testing.T, ctx context.Context, conn *pgxpool.Conn, name string) {
	t.Helper()
	sql, err := os.ReadFile(filepath.Join(".", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	results := conn.Conn().PgConn().Exec(ctx, string(sql))
	if _, err := results.ReadAll(); err != nil {
		t.Fatalf("execute %s: %v", name, err)
	}
}

func assertUniqueProviderStep(t *testing.T, ctx context.Context, conn *pgxpool.Conn) {
	t.Helper()
	var count int
	if err := conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_constraint
		WHERE conrelid = 'verification_steps'::regclass
			AND contype = 'u'
			AND pg_get_constraintdef(oid) LIKE '%provider_id, step%'
	`).Scan(&count); err != nil {
		t.Fatalf("query unique provider step constraint: %v", err)
	}
	if count != 1 {
		t.Fatalf("unique provider step constraints = %d, want 1", count)
	}
}

func assertInvalidVerificationStepRejected(t *testing.T, ctx context.Context, conn *pgxpool.Conn) {
	t.Helper()
	providerID := "11111111-1111-1111-1111-111111111111"
	if _, err := conn.Exec(ctx, `INSERT INTO providers (id, phone) VALUES ($1, $2)`, providerID, "+2348000000001"); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	_, err := conn.Exec(ctx, `
		INSERT INTO verification_steps (provider_id, step, status)
		VALUES ($1, 'passport', 'pending')
	`, providerID)
	if err == nil {
		t.Fatal("invalid verification step was accepted")
	}
}

func assertInvalidVerificationStatusRejected(t *testing.T, ctx context.Context, conn *pgxpool.Conn) {
	t.Helper()
	_, err := conn.Exec(ctx, `
		INSERT INTO verification_steps (provider_id, step, status)
		VALUES ($1, 'identity', 'waiting')
	`, "11111111-1111-1111-1111-111111111111")
	if err == nil {
		t.Fatal("invalid verification status was accepted")
	}
}

func assertVerificationDocumentIndexes(t *testing.T, ctx context.Context, conn *pgxpool.Conn, schema string) {
	t.Helper()
	assertIndexExists(t, ctx, conn, schema, "idx_vdocs_step")
	assertIndexExists(t, ctx, conn, schema, "idx_vdocs_provider")
}

func assertFaceCheckResultConstraint(t *testing.T, ctx context.Context, conn *pgxpool.Conn) {
	t.Helper()
	var stepID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO verification_steps (provider_id, step, status)
		VALUES ($1, 'face', 'pending')
		RETURNING id::text
	`, "11111111-1111-1111-1111-111111111111").Scan(&stepID); err != nil {
		t.Fatalf("insert face step: %v", err)
	}
	for _, result := range []any{nil, "pass", "fail"} {
		if _, err := conn.Exec(ctx, `
			INSERT INTO face_checks (provider_id, step_id, selfie_url, id_doc_url, result)
			VALUES ($1, $2, 'https://example.com/selfie.jpg', 'https://example.com/id.jpg', $3)
		`, "11111111-1111-1111-1111-111111111111", stepID, result); err != nil {
			t.Fatalf("valid face check result %v rejected: %v", result, err)
		}
	}
	_, err := conn.Exec(ctx, `
		INSERT INTO face_checks (provider_id, step_id, selfie_url, id_doc_url, result)
		VALUES ($1, $2, 'https://example.com/selfie.jpg', 'https://example.com/id.jpg', 'maybe')
	`, "11111111-1111-1111-1111-111111111111", stepID)
	if err == nil {
		t.Fatal("invalid face check result was accepted")
	}
}

func assertVerificationAuditIndexes(t *testing.T, ctx context.Context, conn *pgxpool.Conn, schema string) {
	t.Helper()
	assertIndexExists(t, ctx, conn, schema, "idx_vaudit_provider")
}

func assertIndexExists(t *testing.T, ctx context.Context, conn *pgxpool.Conn, schema string, name string) {
	t.Helper()
	var count int
	if err := conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_indexes
		WHERE schemaname = $1 AND indexname = $2
	`, schema, name).Scan(&count); err != nil {
		t.Fatalf("query index %s: %v", name, err)
	}
	if count != 1 {
		t.Fatalf("index %s count = %d, want 1", name, count)
	}
}

func assertTableDropped(t *testing.T, ctx context.Context, conn *pgxpool.Conn, schema string, table string) {
	t.Helper()
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, schema+"."+table).Scan(&exists); err != nil {
		t.Fatalf("query table %s: %v", table, err)
	}
	if exists {
		t.Fatalf("table %s still exists after down migrations", table)
	}
}
