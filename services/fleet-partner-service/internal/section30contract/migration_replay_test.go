// Migration replay tests required by Section 30 WP8. Each test is gated on
// FLEET_PARTNER_TEST_DATABASE_URL (skips cleanly when unset, exactly like
// every other PostgreSQL-backed suite in this service) and runs against its
// own disposable, uniquely-named PHYSICAL DATABASE on that server -- see
// replay_helpers_test.go's doc comment for why a physical database, not
// merely a schema, is required. No production database is ever touched:
// the database-name safety guard (requireTestDatabaseName) refuses to run
// against anything whose name does not look test-specific.
package section30contract

import (
	"context"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

// TestMigrationReplay_EmptySchemaAppliesCleanly is WP8(A): apply
// 000001->tip against a genuinely empty database and verify the resulting
// schema, constraints, and indexes -- representative objects spanning the
// earliest (000001-000002) and the latest (000077) migrations.
func TestMigrationReplay_EmptySchemaAppliesCleanly(t *testing.T) {
	pool := isolatedTestDatabase(t, "fleet_partner_s30_replay_empty")
	ctx := context.Background()

	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("apply migrations to empty database: %v", err)
	}

	applied := appliedMigrationVersions(t, pool)
	if len(applied) < 76 {
		t.Fatalf("recorded %d applied migrations, want at least 76", len(applied))
	}
	if got := applied[len(applied)-1]; !strings.HasPrefix(got, "000077") {
		t.Fatalf("last applied migration = %s, want prefix 000077", got)
	}

	for _, table := range []string{
		"fleet_partner_organizations",            // 000002
		"fleet_partner_permissions",              // 000005
		"fleet_partner_export_jobs",              // 000055
		"fleet_partner_retention_run_records",    // 000072
		"fleet_partner_organization_api_clients", // 000074
		"fleet_partner_webhook_subscriptions",    // 000074
	} {
		if !tableExists(t, pool, table) {
			t.Errorf("table %s does not exist after full apply", table)
		}
	}

	// 000073's additive columns and index.
	if !columnExists(t, pool, "fleet_partner_export_jobs", "media_deletion_status") {
		t.Error("fleet_partner_export_jobs.media_deletion_status missing after full apply")
	}
	if !columnExists(t, pool, "fleet_partner_export_jobs", "media_deletion_request_id") {
		t.Error("fleet_partner_export_jobs.media_deletion_request_id missing after full apply")
	}
	if !indexExists(t, pool, "idx_fleet_export_pending_media_deletion") {
		t.Error("idx_fleet_export_pending_media_deletion missing after full apply")
	}

	// A representative CHECK constraint (organizations.status, 000002 widened
	// by 000008) actually rejects an invalid value -- proves constraints, not
	// just tables/columns, replayed correctly.
	_, err := pool.Exec(ctx, `INSERT INTO fleet_partner_organizations
		(id, legal_name, registration_number, country_code, timezone, default_currency, status, created_at, updated_at)
		VALUES ('org_replay_bad_status', 'Bad Status Co', 'RC-BAD-1', 'NG', 'Africa/Lagos', 'NGN', 'NOT_A_REAL_STATUS', now(), now())`)
	if err == nil {
		t.Error("expected the organizations.status CHECK constraint to reject an invalid value after replay, insert succeeded")
	}
}

// TestMigrationReplay_LatestMigrationRollbackAndReapply is WP8(B): apply
// everything, roll back the safely-reversible latest migration (000077),
// verify its additions are actually gone, then reapply and verify the
// state is fully restored. Earlier migrations (including 000074 and 000075)
// must survive rolling back 000077.
func TestMigrationReplay_LatestMigrationRollbackAndReapply(t *testing.T) {
	pool := isolatedTestDatabase(t, "fleet_partner_s30_replay_rollback")
	ctx := context.Background()

	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if !tableExists(t, pool, "fleet_partner_completed_work_records") {
		t.Fatal("precondition failed: completed-work records should exist right after full apply")
	}

	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("rollback last migration: %v", err)
	}

	applied := appliedMigrationVersions(t, pool)
	for _, v := range applied {
		if strings.HasPrefix(v, "000077") {
			t.Fatal("000077 is still recorded as applied after RollbackLastMigration")
		}
	}
	// Earlier migrations must survive rolling back 000077.
	if !tableExists(t, pool, "fleet_partner_export_jobs") {
		t.Error("fleet_partner_export_jobs table was removed by rolling back 000077 -- rollback affected the wrong migration")
	}
	if !tableExists(t, pool, "fleet_partner_organization_api_clients") {
		t.Error("000076 organization API clients table was removed by rolling back 000077")
	}
	if !columnExists(t, pool, "fleet_partner_export_jobs", "media_deletion_status") {
		t.Error("000073 media_deletion_status column was removed by rolling back 000077")
	}

	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("reapply after rollback: %v", err)
	}
	applied = appliedMigrationVersions(t, pool)
	if got := applied[len(applied)-1]; !strings.HasPrefix(got, "000077") {
		t.Fatalf("last applied migration after reapply = %s, want prefix 000077", got)
	}
	if !tableExists(t, pool, "fleet_partner_completed_work_records") {
		t.Error("completed-work records table not restored after reapply")
	}
	if !tableExists(t, pool, "fleet_partner_managed_work_settlement_instructions") {
		t.Error("settlement instructions table not restored after reapply")
	}
}

// TestMigrationReplay_MultiStepRangeAcrossSections is WP8(C): replay a
// range spanning Section 25's introduction of fleet_partner_export_jobs
// (000055) through the release tip (000077) in two separate apply calls,
// proving the schema is valid and correctly incremental at the pause point,
// not just at the final tip.
func TestMigrationReplay_MultiStepRangeAcrossSections(t *testing.T) {
	pool := isolatedTestDatabase(t, "fleet_partner_s30_replay_multistep")
	ctx := context.Background()

	// Stage 1: stop one migration short of fleet_partner_export_jobs's own
	// creation (000055) -- it must not exist yet.
	if err := database.ApplyMigrationsUpTo(ctx, pool, migrationsDir, "000054"); err != nil {
		t.Fatalf("apply migrations up to 000054: %v", err)
	}
	if tableExists(t, pool, "fleet_partner_export_jobs") {
		t.Fatal("fleet_partner_export_jobs exists after stopping at 000054, but it is created by 000055")
	}
	applied := appliedMigrationVersions(t, pool)
	if got := applied[len(applied)-1]; !strings.HasPrefix(got, "000054") {
		t.Fatalf("last applied migration after staged apply = %s, want prefix 000054", got)
	}

	// Stage 2: resume to the tip (000077). The table must now exist WITH
	// both its founding (000055) and later-added (000073) columns.
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("resume migrations to tip: %v", err)
	}
	if !tableExists(t, pool, "fleet_partner_export_jobs") {
		t.Fatal("fleet_partner_export_jobs still missing after resuming to tip")
	}
	if !columnExists(t, pool, "fleet_partner_export_jobs", "status") { // 000055
		t.Error("fleet_partner_export_jobs.status (000055) missing after resumed replay")
	}
	if !columnExists(t, pool, "fleet_partner_export_jobs", "media_deletion_status") { // 000073
		t.Error("fleet_partner_export_jobs.media_deletion_status (000073) missing after resumed replay")
	}
	applied = appliedMigrationVersions(t, pool)
	if got := applied[len(applied)-1]; !strings.HasPrefix(got, "000077") {
		t.Fatalf("last applied migration after resumed replay = %s, want prefix 000077", got)
	}
}

// TestMigrationReplay_ExistingDataSurvivesUpgrade is WP8(D): seed a
// representative row partway through the migration timeline (right after
// the table that owns it is created), then apply every remaining migration
// -- including 000008's CHECK-widening of organizations.status and every
// later ADD COLUMN -- and prove the seeded row is still present and its
// original field values are unchanged.
func TestMigrationReplay_ExistingDataSurvivesUpgrade(t *testing.T) {
	pool := isolatedTestDatabase(t, "fleet_partner_s30_replay_existingdata")
	ctx := context.Background()

	// fleet_partner_organizations is created by 000002; seed right after it.
	if err := database.ApplyMigrationsUpTo(ctx, pool, migrationsDir, "000002"); err != nil {
		t.Fatalf("apply migrations up to 000002: %v", err)
	}
	const orgID = "org_replay_preexisting"
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_organizations
		(id, legal_name, registration_number, country_code, timezone, default_currency, status, created_at, updated_at)
		VALUES ($1, 'Pre-Existing Data Co', 'RC-PRE-1', 'NG', 'Africa/Lagos', 'NGN', 'PENDING_REVIEW', now(), now())`, orgID); err != nil {
		t.Fatalf("seed pre-existing organization row: %v", err)
	}

	// Apply every remaining migration up to the tip, including 000008's
	// status CHECK widen and every later ADD COLUMN touching this table.
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("apply remaining migrations after seeding pre-existing data: %v", err)
	}

	var legalName, registrationNumber, countryCode, status string
	err := pool.QueryRow(ctx,
		`SELECT legal_name, registration_number, country_code, status FROM fleet_partner_organizations WHERE id = $1`, orgID,
	).Scan(&legalName, &registrationNumber, &countryCode, &status)
	if err != nil {
		t.Fatalf("pre-existing organization row is not readable after upgrade: %v", err)
	}
	if legalName != "Pre-Existing Data Co" || registrationNumber != "RC-PRE-1" || countryCode != "NG" || status != "PENDING_REVIEW" {
		t.Errorf("pre-existing row's original field values changed across upgrade: legal_name=%q registration_number=%q country_code=%q status=%q",
			legalName, registrationNumber, countryCode, status)
	}
}
