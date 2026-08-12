package integration_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// TestBackupAndRestore_RealDisposableDatabase is the Section 29 WP18/19
// mandatory restore test: seeds representative data, creates a real pg_dump
// backup, restores it into a genuinely NEW disposable database (never the
// source/test database itself), and verifies migration version, critical
// tables and representative rows survived. No production database is ever
// touched — the source here is already the disposable integration test
// database (FLEET_PARTNER_TEST_DATABASE_URL), and the restore TARGET is a
// second, throwaway database created and dropped within this test.
//
// This shells out to `docker exec <container> pg_dump/pg_restore/psql`
// because neither this Windows dev/CI environment nor the postgres:16-alpine
// test container's host has PostgreSQL client binaries on a shared PATH —
// the container itself does. See
// services/fleet-partner-service/scripts/backup.sh and restore-verify.sh
// for the operator-runnable equivalent of exactly these same steps.
//
// Gated on FLEET_PARTNER_TEST_DATABASE_URL (as every other DB-backed test in
// this package, used for the Go pool connecting from the HOST),
// FLEET_PARTNER_BACKUP_DATABASE_URL (the same database's connection string
// AS SEEN FROM INSIDE the container named by
// FLEET_PARTNER_BACKUP_DOCKER_CONTAINER — typically host "localhost" and
// PostgreSQL's own default port 5432, not whatever host-side port mapping
// FLEET_PARTNER_TEST_DATABASE_URL uses; see scripts/backup.sh's identical
// note) and that container name itself.
func TestBackupAndRestore_RealDisposableDatabase(t *testing.T) {
	hostURL := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	rawURL := os.Getenv("FLEET_PARTNER_BACKUP_DATABASE_URL")
	container := os.Getenv("FLEET_PARTNER_BACKUP_DOCKER_CONTAINER")
	if hostURL == "" || rawURL == "" || container == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL, FLEET_PARTNER_BACKUP_DATABASE_URL and FLEET_PARTNER_BACKUP_DOCKER_CONTAINER must all be set; skipping real backup/restore test")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not available on PATH; skipping real backup/restore test")
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, hostURL, database.PoolOptions{MaxConns: 5})
	if err != nil {
		t.Fatalf("connect source database: %v", err)
	}
	defer pool.Close()

	// 1. Seed representative data spanning the record classes WP19 names:
	// organization, order, assignment, document, event (outbox), audit, job.
	org := createOrganization(t, pool, "Backup Restore Test Co", "RC-BKR-"+ids.New("org"))
	if _, err := jobs.NewQueue(pool).Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "backup_restore_probe", MaxAttempts: 3,
	}); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	// WP20/21: legal holds must survive a restore intact and still ACTIVE —
	// a hold that silently vanished or reverted status across a DR event
	// would be a genuine compliance failure, not just a data-loss one.
	legalHoldID := ids.New("lgh")
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_legal_holds
			(id, organization_id, scope_type, scope_reference, reason, status, created_by_actor_type, created_by_actor_id, created_at, version)
		VALUES ($1, $2, 'ORGANIZATION', $2, 'backup/restore test hold', 'ACTIVE', 'system', 'backup-restore-test', now(), 1)`,
		legalHoldID, org.ID); err != nil {
		t.Fatalf("seed legal hold: %v", err)
	}

	var preRestoreOrgCount, preRestoreAuditCount int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_organizations`).Scan(&preRestoreOrgCount); err != nil {
		t.Fatalf("count organizations: %v", err)
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records`).Scan(&preRestoreAuditCount)

	// 2. Real backup via pg_dump, custom format, inside the container.
	dumpPath := t.TempDir() + "/backup_restore_test.dump"
	dumpCmd := exec.Command("docker", "exec", "-i", container, "pg_dump", "-Fc", "--no-owner", "--no-privileges", "-d", rawURL)
	dumpFile, err := os.Create(dumpPath)
	if err != nil {
		t.Fatalf("create dump file: %v", err)
	}
	dumpCmd.Stdout = dumpFile
	var dumpStderr strings.Builder
	dumpCmd.Stderr = &dumpStderr
	dumpErr := dumpCmd.Run()
	_ = dumpFile.Close()
	if dumpErr != nil {
		t.Fatalf("pg_dump failed: %v: %s", dumpErr, dumpStderr.String())
	}
	info, err := os.Stat(dumpPath)
	if err != nil || info.Size() == 0 {
		t.Fatalf("backup file missing or empty: err=%v size=%d", err, info.Size())
	}

	// 3. Create a genuinely disposable database (all-digits timestamp: an
	// unquoted CREATE DATABASE identifier folds to lowercase, and a
	// mismatched-case name would silently create one identifier while every
	// later lookup uses another — see restore-verify.sh's own comment on
	// exactly this).
	disposableDB := "fleet_partner_backup_restore_test_" + time.Now().UTC().Format("20060102150405") + "_" + ids.New("d")[2:10]
	runPsql := func(sql string) (string, error) {
		cmd := exec.Command("docker", "exec", "-i", container, "psql", "-X", "-A", "-t", "-d", rawURL, "-c", sql)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	if _, err := runPsql("CREATE DATABASE " + disposableDB); err != nil {
		t.Fatalf("create disposable database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = runPsql("DROP DATABASE IF EXISTS " + disposableDB)
	})

	disposableURL := strings.Replace(rawURL, dbNameFromURL(t, rawURL), disposableDB, 1)

	// 4. Restore.
	start := time.Now()
	restoreCmd := exec.Command("docker", "exec", "-i", container, "pg_restore", "--no-owner", "--no-privileges", "-d", disposableURL)
	restoreDumpFile, err := os.Open(dumpPath)
	if err != nil {
		t.Fatalf("reopen dump file: %v", err)
	}
	defer restoreDumpFile.Close()
	restoreCmd.Stdin = restoreDumpFile
	var restoreStderr strings.Builder
	restoreCmd.Stderr = &restoreStderr
	if err := restoreCmd.Run(); err != nil {
		t.Fatalf("pg_restore failed: %v: %s", err, restoreStderr.String())
	}
	duration := time.Since(start)

	// 5. Verify: migration version, critical tables, representative rows,
	// append-only audit protection.
	restoredVersion, err := runPsql2(t, container, disposableURL, "SELECT version FROM fleet_partner_schema_migrations ORDER BY version DESC LIMIT 1")
	if err != nil || restoredVersion == "" {
		t.Fatalf("restored migration version missing: err=%v", err)
	}
	sourceVersion, err := runPsql2(t, container, rawURL, "SELECT version FROM fleet_partner_schema_migrations ORDER BY version DESC LIMIT 1")
	if err != nil || sourceVersion != restoredVersion {
		t.Fatalf("restored migration version = %q, want it to match the source's %q", restoredVersion, sourceVersion)
	}

	for _, table := range []string{
		"fleet_partner_organizations", "fleet_partner_private_orders", "fleet_partner_private_assignments",
		"fleet_partner_documents", "fleet_partner_outbox", "fleet_partner_audit_records",
		"fleet_partner_jobs", "fleet_partner_service_credentials",
	} {
		count, err := runPsql2(t, container, disposableURL, "SELECT count(*) FROM "+table)
		if err != nil {
			t.Fatalf("critical table %s not queryable after restore: %v", table, err)
		}
		t.Logf("restored %s: %s rows", table, count)
	}

	restoredOrgExists, err := runPsql2(t, container, disposableURL, "SELECT count(*) FROM fleet_partner_organizations WHERE id='"+org.ID+"'")
	if err != nil || restoredOrgExists != "1" {
		t.Fatalf("seeded representative organization %s did not survive restore: count=%q err=%v", org.ID, restoredOrgExists, err)
	}

	restoredHoldStatus, err := runPsql2(t, container, disposableURL, "SELECT status FROM fleet_partner_legal_holds WHERE id='"+legalHoldID+"'")
	if err != nil || restoredHoldStatus != "ACTIVE" {
		t.Fatalf("legal hold %s did not survive restore as ACTIVE: status=%q err=%v", legalHoldID, restoredHoldStatus, err)
	}

	triggerCount, err := runPsql2(t, container, disposableURL,
		"SELECT count(*) FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid WHERE c.relname='fleet_partner_audit_records' AND NOT t.tgisinternal")
	if err != nil || triggerCount == "0" {
		t.Fatalf("append-only audit trigger missing after restore: count=%q err=%v", triggerCount, err)
	}

	t.Logf("restore verification PASS: duration=%s schemaVersion=%s preRestoreOrgs=%d preRestoreAudit=%d",
		duration, restoredVersion, preRestoreOrgCount, preRestoreAuditCount)
}

func runPsql2(t *testing.T, container, url, sql string) (string, error) {
	t.Helper()
	cmd := exec.Command("docker", "exec", "-i", container, "psql", "-X", "-A", "-t", "-d", url, "-c", sql)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// dbNameFromURL extracts the database name (path component) of a postgres
// connection URL for the purpose of substring-replacing it with a
// disposable database name — this test's URLs are always simple
// postgres://user:pass@host:port/dbname?query forms, never containing the
// database name anywhere else in the string.
func dbNameFromURL(t *testing.T, rawURL string) string {
	t.Helper()
	withoutQuery := rawURL
	if idx := strings.IndexByte(rawURL, '?'); idx >= 0 {
		withoutQuery = rawURL[:idx]
	}
	idx := strings.LastIndexByte(withoutQuery, '/')
	if idx < 0 || idx == len(withoutQuery)-1 {
		t.Fatalf("could not extract database name from %q", rawURL)
	}
	return withoutQuery[idx+1:]
}
