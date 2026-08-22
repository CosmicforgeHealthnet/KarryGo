package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestMigrationReplay applies every migration, verifies the foundation table
// is reachable, rolls back the LATEST migration determined by querying the
// tracker (never a hardcoded migration name/position â€” a fixed assumption
// here is exactly the bug class this runner exists to avoid, see
// docs/architecture/12_MIGRATION_OWNERSHIP.md), verifies its down file
// actually undid the change, then reapplies and verifies consistency. Skips
// cleanly if no test database is configured.
func TestMigrationReplay(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres migration replay test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	dir := "../../migrations"

	if err := ApplyMigrations(ctx, pool, dir); err != nil {
		t.Fatalf("apply migrations before cleanup: %v", err)
	}
	// A shared external test database may carry rows left behind by another
	// package's tests run earlier in the same `go test -p 1 ./...` invocation
	// (each package is independently correct in isolation â€” see
	// docs/IMPLEMENTATION_STATUS.md's Section 5 entry for this exact,
	// documented test-infrastructure caveat). Clearing dependent Section 5
	// rows here keeps this migration-runner test robust regardless of what
	// ran before it, without weakening what it actually proves about
	// ApplyMigrations/RollbackLastMigration.
	if _, err := pool.Exec(ctx, `TRUNCATE vendor_staff_role_assignments, vendor_step_up_grants, vendor_sessions, vendor_otp_challenges, vendor_staff, vendor_users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("clear dependent rows before rollback test: %v", err)
	}

	migrations, err := ListMigrations(dir)
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("expected at least one migration to exist")
	}
	last := migrations[len(migrations)-1]
	if last.DownFile == "" {
		t.Fatalf("last migration %s has no down file; this test requires one to exercise rollback", last.Version)
	}

	if err := ApplyMigrations(ctx, pool, dir); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := MigrationsCurrent(ctx, pool, dir); err != nil {
		t.Fatalf("expected all migrations applied: %v", err)
	}

	var identityExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vendor_service_identity WHERE id = true)`).Scan(&identityExists); err != nil {
		t.Fatalf("query identity row: %v", err)
	}
	if !identityExists {
		t.Fatal("expected the foundation migration's identity row to exist")
	}

	var lastVersionApplied bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vendor_service_schema_migrations WHERE version = $1)`, last.Version).Scan(&lastVersionApplied); err != nil {
		t.Fatalf("query tracker for last migration: %v", err)
	}
	if !lastVersionApplied {
		t.Fatalf("expected %s to be tracked as applied", last.Version)
	}

	// Roll back the last migration and confirm the tracker forgot it.
	if err := RollbackLastMigration(ctx, pool, dir); err != nil {
		t.Fatalf("rollback last migration: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vendor_service_schema_migrations WHERE version = $1)`, last.Version).Scan(&lastVersionApplied); err != nil {
		t.Fatalf("query tracker after rollback: %v", err)
	}
	if lastVersionApplied {
		t.Fatalf("expected %s to be untracked after rollback", last.Version)
	}
	if err := MigrationsCurrent(ctx, pool, dir); err == nil {
		t.Fatal("expected MigrationsCurrent to report the rolled-back migration as missing")
	}

	// Reapply and confirm the tracker and schema are consistent again.
	if err := ApplyMigrations(ctx, pool, dir); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	if err := MigrationsCurrent(ctx, pool, dir); err != nil {
		t.Fatalf("expected all migrations applied after reapply: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vendor_service_schema_migrations WHERE version = $1)`, last.Version).Scan(&lastVersionApplied); err != nil {
		t.Fatalf("query tracker after reapply: %v", err)
	}
	if !lastVersionApplied {
		t.Fatalf("expected %s to be tracked again after reapply", last.Version)
	}

	// TestMigrationReplay deliberately performs destructive shared-database
	// cleanup above using TRUNCATE ... CASCADE so that rollback can be tested
	// regardless of rows left by earlier integration packages. That cascade
	// can also empty platform-owned seed tables introduced by older migrations
	// while those migrations remain recorded as already applied.
	//
	// Before returning, restore a pristine all-migrations-applied database.
	// This prevents later packages in `go test -p 1 ./...` from inheriting a
	// schema whose migration tracker is current but whose immutable/platform
	// seed data was removed by this test.
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE`); err != nil {
		t.Fatalf("reset shared migration-test schema: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA public`); err != nil {
		t.Fatalf("recreate shared migration-test schema: %v", err)
	}
	if err := ApplyMigrations(ctx, pool, dir); err != nil {
		t.Fatalf("restore shared migration-test database: %v", err)
	}
	if err := MigrationsCurrent(ctx, pool, dir); err != nil {
		t.Fatalf("restored migration-test database is not current: %v", err)
	}

	// Section 12A's controlled measurement catalogue is a useful canary for
	// destructive CASCADE leakage because later Shop integration tests depend
	// on it while it is owned by an earlier migration.
	var restoredMeasureDefinitions int
	if err := pool.QueryRow(ctx, `
        SELECT count(*)
        FROM shop_measure_definitions
    `).Scan(&restoredMeasureDefinitions); err != nil {
		t.Fatalf("verify restored measurement seed: %v", err)
	}
	if restoredMeasureDefinitions < 40 {
		t.Fatalf(
			"shared migration-test database restoration incomplete: measurement definitions=%d",
			restoredMeasureDefinitions,
		)
	}
}

// TestSection5SchemaExists is a focused, migration-position-independent check
// that Section 5's own tables and seed data exist once all migrations are
// applied â€” unlike TestMigrationReplay, this does not care which migration
// number introduced them, so it never needs updating when a later section
// adds more migrations on top.
func TestSection5SchemaExists(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres schema test")
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

	for _, table := range []string{
		"vendor_users", "vendor_staff", "vendor_otp_challenges", "vendor_sessions",
		"vendor_step_up_grants", "vendor_permissions", "vendor_roles",
		"vendor_role_permissions", "vendor_staff_role_assignments", "vendor_audit_log",
	} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)`, table).Scan(&exists); err != nil {
			t.Fatalf("query table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	var permissionCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions`).Scan(&permissionCount); err != nil {
		t.Fatalf("count permissions: %v", err)
	}
	if permissionCount < 30 {
		t.Fatalf("expected at least 30 seeded permissions, got %d", permissionCount)
	}

	var systemRoleCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM vendor_roles WHERE is_system = true`).Scan(&systemRoleCount); err != nil {
		t.Fatalf("count system roles: %v", err)
	}
	if systemRoleCount != 11 {
		t.Fatalf("expected 11 default role families, got %d", systemRoleCount)
	}

	// Owner holds every VENDOR-SCOPED permission.
	//
	// This assertion used to be "Owner holds every permission", which was true
	// while every permission in the catalogue was a vendor capability. Section
	// 13 introduced the platform capabilities - global taxonomy publication,
	// ranking administration and projection administration - which are
	// deliberately granted to no vendor role, Owner included, because they
	// decide how every vendor on the marketplace is classified and ranked.
	//
	// The check is therefore split rather than relaxed: Owner must hold all of
	// one set and none of the other. That is a stronger statement than the
	// original count comparison, and it fails if a future migration either
	// forgets to grant Owner a new vendor capability or sweeps a platform one
	// into a vendor role.
	platformOnly := []string{
		"vendor.taxonomy.platform.admin",
		"vendor.search.ranking.admin",
		"vendor.search.projection.admin",
	}

	var vendorScopedCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_permissions WHERE code <> ALL($1)`, platformOnly,
	).Scan(&vendorScopedCount); err != nil {
		t.Fatalf("count vendor-scoped permissions: %v", err)
	}

	var ownerPermissionCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_role_permissions WHERE role_id = 'role_owner'`,
	).Scan(&ownerPermissionCount); err != nil {
		t.Fatalf("count owner permissions: %v", err)
	}
	if ownerPermissionCount != vendorScopedCount {
		t.Fatalf("expected Owner to hold every vendor-scoped permission (%d of %d catalogued), got %d",
			vendorScopedCount, permissionCount, ownerPermissionCount)
	}

	var ownerPlatformGrants int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_role_permissions rp
           JOIN vendor_permissions p ON p.id = rp.permission_id
          WHERE rp.role_id = 'role_owner' AND p.code = ANY($1)`, platformOnly,
	).Scan(&ownerPlatformGrants); err != nil {
		t.Fatalf("count owner platform grants: %v", err)
	}
	if ownerPlatformGrants != 0 {
		t.Fatalf("Owner holds %d platform-only capabilities; it must hold none", ownerPlatformGrants)
	}
}

func TestSection6SchemaAndPermissionCatalogue(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 6 schema test")
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
	for _, table := range []string{"vendor_businesses", "vendor_capabilities", "vendor_brands", "vendor_branches", "vendor_branch_hours", "vendor_branch_service_areas", "vendor_branch_settings", "vendor_branch_capabilities"} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name=$1)`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("Section 6 table %s missing (err=%v)", table, err)
		}
	}
	permissions := []string{"vendor.capabilities.read", "vendor.capabilities.manage", "vendor.brands.read", "vendor.brands.manage", "vendor.branch.hours.manage", "vendor.branch.settings.manage", "vendor.branch.capabilities.manage"}
	for _, code := range permissions {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code=$1`, code).Scan(&count); err != nil || count != 1 {
			t.Fatalf("permission %s count=%d err=%v", code, count, err)
		}
	}
}

// rollbackUntilRemoved reverses migrations one at a time until version is no
// longer tracked.
//
// A section's rollback test has to step past every section stacked on top of it
// first. Doing that by walking down to a named version, rather than by listing
// the later versions by name, means adding a new section never requires editing
// an older section's test â€” a list like that goes stale silently and turns into
// a confusing failure in unrelated code.
func rollbackUntilRemoved(t *testing.T, ctx context.Context, pool *pgxpool.Pool, version string) {
	t.Helper()

	// The bound is derived from how many migrations are actually tracked, not
	// from a constant. A fixed cap is the exact bug this package's own doc
	// comment warns about: 64 was ample when it was written and silently
	// became too small the moment the repository grew past it, turning a
	// healthy rollback into a confusing failure in unrelated code. Rolling
	// back can never need more steps than there are applied migrations, and
	// the loop also stops the moment it stops making progress.
	var applied int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM vendor_service_schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}

	for step := 0; step <= applied; step++ {
		var tracked bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_service_schema_migrations WHERE version=$1)`, version).Scan(&tracked); err != nil {
			t.Fatalf("check %s: %v", version, err)
		}
		if !tracked {
			return
		}
		if err := RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback towards %s: %v", version, err)
		}
	}
	t.Fatalf("%s was still tracked after rolling back all %d applied migrations", version, applied)
}

func TestSection7MigrationsRollbackAndReapplyAsAPair(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 7 replay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `TRUNCATE vendor_businesses,vendor_staff_role_assignments,vendor_step_up_grants,vendor_sessions,vendor_otp_challenges,vendor_staff,vendor_users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	// Later sections sit on top of Section 7, so step back past them first: this
	// test is about the Section 7 pair rolling back and reapplying cleanly, not
	// about which migration happens to be the current tip.
	rollbackUntilRemoved(t, ctx, pool, "000008_vendor_section7_onboarding")
	for _, version := range []string{"000009_vendor_section7_permissions", "000008_vendor_section7_onboarding"} {
		var tracked bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_service_schema_migrations WHERE version=$1)`, version).Scan(&tracked); err != nil || tracked {
			t.Fatalf("version %s tracked=%t err=%v", version, tracked, err)
		}
	}
	var onboardingTable bool
	if err = pool.QueryRow(ctx, `SELECT to_regclass('public.vendor_onboarding_profiles') IS NOT NULL`).Scan(&onboardingTable); err != nil || onboardingTable {
		t.Fatalf("Section 7 table survived rollback exists=%t err=%v", onboardingTable, err)
	}
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply Section 7: %v", err)
	}
	for _, table := range []string{"vendor_onboarding_profiles", "vendor_authorized_representatives", "vendor_onboarding_documents", "vendor_verification_projections", "vendor_verification_history", "vendor_compliance_requirements", "vendor_bank_profiles", "vendor_verification_operations", "vendor_service_nonces"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("reapplied table %s exists=%t err=%v", table, exists, err)
		}
	}
	for _, constraint := range []string{"vendor_verification_projections_status_check", "vendor_verification_history_status_check", "vendor_bank_profiles_status_check"} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		for _, status := range []string{"NOT_STARTED", "IN_PROGRESS", "PENDING_REVIEW", "VERIFIED", "REJECTED", "EXPIRED", "SUSPENDED"} {
			if !strings.Contains(definition, "'"+status+"'") {
				t.Fatalf("%s does not contain %s: %s", constraint, status, definition)
			}
		}
		if strings.Contains(definition, "'PENDING'") {
			t.Fatalf("%s incorrectly permits plain PENDING: %s", constraint, definition)
		}
	}
	var permissionCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN ('vendor.onboarding.read','vendor.onboarding.manage','vendor.verification.read','vendor.verification.submit','vendor.bank_profile.read','vendor.bank_profile.manage')`).Scan(&permissionCount); err != nil || permissionCount != 6 {
		t.Fatalf("Section 7 permissions=%d err=%v", permissionCount, err)
	}
}

// TestSection8AMigrationsRollbackAndReapplyAsAPair proves the CRM foundation
// and its permission seed roll back and reapply cleanly as a pair, that the
// tenancy/privacy/consent vocabularies are enforced by real CHECK constraints
// rather than only in Go, and that the composite (id, business_id) foreign
// keys every CRM child table depends on actually exist.
func TestSection8AMigrationsRollbackAndReapplyAsAPair(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 8A replay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	crmTables := []string{
		"vendor_customer_profiles", "crm_customer_identity_links", "crm_customer_branch_relations",
		"crm_customer_order_facts", "crm_consent_projections", "crm_privacy_projections",
		"crm_privacy_processing_history",
	}

	// Step past Section 8B, which stacks on top of this one.
	rollbackUntilRemoved(t, ctx, pool, "000010_vendor_crm_foundation")
	for _, version := range []string{"000011_vendor_section8a_crm_permissions", "000010_vendor_crm_foundation"} {
		var tracked bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_service_schema_migrations WHERE version=$1)`, version).Scan(&tracked); err != nil || tracked {
			t.Fatalf("version %s tracked=%t err=%v", version, tracked, err)
		}
	}
	for _, table := range crmTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("CRM table %s survived rollback exists=%t err=%v", table, exists, err)
		}
	}
	// The inbox fingerprint column is additive and must also be reversible.
	var fingerprint bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='vendor_inbox_processed_events' AND column_name='payload_fingerprint')`).Scan(&fingerprint); err != nil || fingerprint {
		t.Fatalf("inbox fingerprint survived rollback exists=%t err=%v", fingerprint, err)
	}

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply Section 8A: %v", err)
	}
	for _, table := range crmTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("reapplied table %s exists=%t err=%v", table, exists, err)
		}
	}

	// Exact vocabularies, enforced in SQL.
	vocabularies := map[string][]string{
		"vendor_customer_profiles_subject_type_check":     {"CUSTOMER", "GUEST", "EXTERNAL"},
		"vendor_customer_profiles_status_check":           {"ACTIVE", "ARCHIVED"},
		"vendor_customer_profiles_privacy_state_check":    {"ACTIVE", "RESTRICTED", "REDACTED", "MERGED"},
		"vendor_customer_profiles_merge_state_check":      {"NONE", "LINK_PENDING", "MERGED_SOURCE", "MERGED_TARGET"},
		"crm_consent_projections_decision_check":          {"ALLOW", "DENY", "UNKNOWN"},
		"crm_privacy_projections_action_check":            {"RESTRICT", "REDACT", "MARKETING_SUPPRESS", "IDENTITY_MERGE"},
		"crm_privacy_projections_processing_status_check": {"PENDING", "PROCESSING", "APPLIED", "FAILED", "DEAD_LETTER"},
	}
	for constraint, values := range vocabularies {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		for _, value := range values {
			if !strings.Contains(definition, "'"+value+"'") {
				t.Fatalf("%s does not contain %s: %s", constraint, value, definition)
			}
		}
	}

	// Tenancy: every CRM child reaches its profile through the composite key,
	// so a child can never point at another business's profile.
	for _, constraint := range []string{
		"crm_identity_link_source_fk", "crm_identity_link_target_fk",
		"crm_branch_relation_profile_fk", "crm_branch_relation_branch_fk",
		"crm_order_fact_profile_fk", "crm_order_fact_branch_fk",
		"crm_consent_profile_fk", "crm_privacy_profile_fk",
	} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		if !strings.Contains(definition, "business_id") {
			t.Fatalf("%s is not tenant-composite: %s", constraint, definition)
		}
	}

	var permissionCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN ('vendor.crm.customers.read','vendor.crm.customers.manage','vendor.crm.customer360.read','vendor.crm.consent.read','vendor.crm.privacy.read')`).Scan(&permissionCount); err != nil || permissionCount != 5 {
		t.Fatalf("Section 8A permissions=%d err=%v", permissionCount, err)
	}
	// No override permission may exist for any role, at any time.
	var overrides int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN ('vendor.crm.consent.override','vendor.crm.privacy.override','vendor.crm.identity.force_merge')`).Scan(&overrides); err != nil || overrides != 0 {
		t.Fatalf("forbidden override permissions=%d err=%v", overrides, err)
	}
	// Marketing must never hold privacy read or lifecycle management.
	var marketingLeak int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp JOIN vendor_permissions p ON p.id=rp.permission_id WHERE rp.role_id='role_marketing' AND p.code IN ('vendor.crm.privacy.read','vendor.crm.customers.manage')`).Scan(&marketingLeak); err != nil || marketingLeak != 0 {
		t.Fatalf("marketing over-granted=%d err=%v", marketingLeak, err)
	}
}

func TestSection8BMigrationsRollbackAndReapplyAsAPair(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 8B replay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	segmentationTables := []string{
		"crm_tags", "crm_customer_tag_memberships", "crm_segments", "crm_segment_versions",
		"crm_segment_evaluations", "crm_segment_memberships", "crm_segment_exclusions",
	}
	// Section 8A's tables must be untouched by an 8B rollback: a section may
	// only ever drop the schema it owns.
	section8ATables := []string{
		"vendor_customer_profiles", "crm_consent_projections", "crm_privacy_projections",
		"crm_customer_order_facts", "crm_customer_identity_links", "crm_customer_branch_relations",
	}

	// Step past Section 8C before removing the 8B pair. Older-section tests do
	// not name future migrations, so adding a section never makes them stale.
	rollbackUntilRemoved(t, ctx, pool, "000012_vendor_section8b_tags_segments")
	for _, version := range []string{"000013_vendor_section8b_permissions", "000012_vendor_section8b_tags_segments"} {
		var tracked bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_service_schema_migrations WHERE version=$1)`, version).Scan(&tracked); err != nil || tracked {
			t.Fatalf("version %s tracked=%t err=%v", version, tracked, err)
		}
	}
	for _, table := range segmentationTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("Section 8B table %s survived rollback exists=%t err=%v", table, exists, err)
		}
	}
	for _, table := range section8ATables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("Section 8A table %s was dropped by a Section 8B rollback exists=%t err=%v", table, exists, err)
		}
	}
	var strandedPermissions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions
		WHERE code LIKE 'vendor.crm.tags.%' OR code LIKE 'vendor.crm.segments%'`).Scan(&strandedPermissions); err != nil || strandedPermissions != 0 {
		t.Fatalf("Section 8B permissions survived rollback count=%d err=%v", strandedPermissions, err)
	}
	var section8APermissions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code='vendor.crm.customers.read'`).Scan(&section8APermissions); err != nil || section8APermissions != 1 {
		t.Fatalf("Section 8A permissions damaged by an 8B rollback count=%d err=%v", section8APermissions, err)
	}

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply Section 8B: %v", err)
	}
	for _, table := range segmentationTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("reapplied table %s exists=%t err=%v", table, exists, err)
		}
	}

	// Exact vocabularies, enforced in SQL rather than only in Go.
	vocabularies := map[string][]string{
		"crm_tags_scope_type_check":                       {"BUSINESS", "BRANCH"},
		"crm_tags_status_check":                           {"ACTIVE", "ARCHIVED"},
		"crm_customer_tag_memberships_source_type_check":  {"MANUAL", "SYSTEM"},
		"crm_segments_segment_type_check":                 {"STATIC", "DYNAMIC"},
		"crm_segments_status_check":                       {"DRAFT", "ACTIVE", "ARCHIVED"},
		"crm_segments_evaluation_status_check":            {"NOT_EVALUATED", "QUEUED", "RUNNING", "CURRENT", "STALE", "FAILED"},
		"crm_segment_evaluations_status_check":            {"QUEUED", "RUNNING", "SUCCESS", "FAILED", "DEAD_LETTER", "SUPERSEDED"},
		"crm_segment_memberships_membership_source_check": {"STATIC", "DYNAMIC"},
	}
	for constraint, values := range vocabularies {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		for _, value := range values {
			if !strings.Contains(definition, "'"+value+"'") {
				t.Fatalf("%s does not contain %s: %s", constraint, value, definition)
			}
		}
	}

	// Tenancy: every Section 8B child reaches its parent through the composite
	// (id, business_id) key, so PostgreSQL â€” not only the HTTP layer â€” refuses a
	// tag pointing at another business's customer.
	for _, constraint := range []string{
		"crm_tag_branch_fk", "crm_tag_membership_tag_fk", "crm_tag_membership_profile_fk",
		"crm_tag_membership_branch_fk", "crm_segment_branch_fk", "crm_segment_version_segment_fk",
		"crm_segment_evaluation_segment_fk", "crm_segment_membership_segment_fk",
		"crm_segment_membership_profile_fk", "crm_segment_exclusion_segment_fk",
		"crm_segment_exclusion_profile_fk",
	} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		if !strings.Contains(definition, "business_id") {
			t.Fatalf("%s is not tenant-composite: %s", constraint, definition)
		}
	}

	// Indexes the hot paths depend on. A missing one turns a segment evaluation
	// into a sequential scan of every relationship in the business.
	for _, index := range []string{
		"crm_tags_business_name_uq", "crm_tags_branch_name_uq", "crm_tags_business_status_idx",
		"crm_tag_memberships_profile_idx", "crm_tag_memberships_tag_idx",
		"crm_segments_business_name_uq", "crm_segments_business_idx",
		"crm_segment_evaluations_due_idx", "crm_segment_evaluations_segment_idx",
		"crm_segment_memberships_current_idx", "crm_segment_memberships_profile_idx",
		"crm_segment_exclusions_segment_idx", "crm_segment_exclusions_profile_idx",
	} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname=$1)`, index).Scan(&exists); err != nil || !exists {
			t.Fatalf("index %s exists=%t err=%v", index, exists, err)
		}
	}

	var permissionCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN
		('vendor.crm.tags.read','vendor.crm.tags.manage','vendor.crm.segments.read',
		 'vendor.crm.segments.manage','vendor.crm.segments.members.read')`).Scan(&permissionCount); err != nil || permissionCount != 5 {
		t.Fatalf("Section 8B permissions=%d err=%v", permissionCount, err)
	}
	// Section 8B adds audiences, never the authority to contact them. Campaigns
	// became Section 8E's and are no longer forbidden here; the authority to
	// send remains unbuilt, and a send permission appearing anywhere would mean
	// the delivery boundary had moved.
	var forbidden int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions
		WHERE code LIKE '%.send%'`).Scan(&forbidden); err != nil || forbidden != 0 {
		t.Fatalf("send permissions leaked into the vendor permission set count=%d err=%v", forbidden, err)
	}
	// Roles with no segmentation need must hold none of it.
	var operationalRoles int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE rp.role_id IN ('role_finance','role_kitchen_pastry','role_shop_inventory','role_procurement','role_vendor_rider')
		  AND (p.code LIKE 'vendor.crm.tags.%' OR p.code LIKE 'vendor.crm.segments%')`).Scan(&operationalRoles); err != nil || operationalRoles != 0 {
		t.Fatalf("unrelated operational roles hold Section 8B permissions count=%d err=%v", operationalRoles, err)
	}
	// Analyst/Auditor is read-only by policy.
	var analystWrites int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE rp.role_id='role_analyst_auditor'
		  AND p.code IN ('vendor.crm.tags.manage','vendor.crm.segments.manage')`).Scan(&analystWrites); err != nil || analystWrites != 0 {
		t.Fatalf("analyst/auditor holds Section 8B write permissions count=%d err=%v", analystWrites, err)
	}
}

func TestSection8CMigrationsRollbackAndReapplyAsAPair(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 8C replay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	section8CTables := []string{"crm_interactions", "crm_interaction_revisions", "crm_tasks", "crm_task_history"}
	rollbackUntilRemoved(t, ctx, pool, "000014_vendor_crm_interactions_tasks")
	for _, table := range section8CTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("Section 8C table %s survived rollback exists=%t err=%v", table, exists, err)
		}
	}
	for _, table := range []string{"crm_tags", "crm_segments", "vendor_customer_profiles"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("accepted baseline table %s damaged exists=%t err=%v", table, exists, err)
		}
	}
	var permissions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code LIKE 'vendor.crm.interactions.%' OR code LIKE 'vendor.crm.tasks.%'`).Scan(&permissions); err != nil || permissions != 0 {
		t.Fatalf("Section 8C permissions survived rollback count=%d err=%v", permissions, err)
	}
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("8B to 8C upgrade/reapply: %v", err)
	}
	for _, table := range section8CTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("reapplied table %s exists=%t err=%v", table, exists, err)
		}
	}
	for constraint, values := range map[string][]string{
		"crm_interactions_interaction_type_check": {"NOTE", "PHONE_CALL", "IN_PERSON", "EMAIL", "SMS", "IN_APP", "SOCIAL", "SALES_FOLLOW_UP", "SERVICE_FOLLOW_UP", "FEEDBACK", "ACCOUNT_REVIEW", "OTHER"},
		"crm_interactions_direction_check":        {"INBOUND", "OUTBOUND", "INTERNAL", "NOT_APPLICABLE"},
		"crm_interactions_outcome_check":          {"NONE", "SUCCESSFUL", "NO_RESPONSE", "FOLLOW_UP_REQUIRED", "RESOLVED", "UNRESOLVED", "CUSTOMER_DECLINED", "VENDOR_DECLINED", "ESCALATED", "CANCELLED"},
		"crm_tasks_status_check":                  {"OPEN", "IN_PROGRESS", "COMPLETED", "CANCELLED"},
		"crm_tasks_priority_check":                {"LOW", "NORMAL", "HIGH", "URGENT"},
	} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		for _, value := range values {
			if !strings.Contains(definition, "'"+value+"'") {
				t.Fatalf("%s missing %s: %s", constraint, value, definition)
			}
		}
	}
	for _, constraint := range []string{"crm_interaction_profile_fk", "crm_interaction_original_profile_fk", "crm_interaction_branch_fk", "crm_interaction_recorder_fk", "crm_interaction_revision_parent_fk", "crm_task_profile_fk", "crm_task_original_profile_fk", "crm_task_branch_fk", "crm_task_assignee_fk", "crm_task_creator_fk", "crm_task_source_interaction_fk", "crm_task_history_parent_fk"} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		if !strings.Contains(definition, "business_id") {
			t.Fatalf("%s is not tenant-composite: %s", constraint, definition)
		}
	}
	for _, index := range []string{"crm_interactions_customer_timeline_idx", "crm_interactions_branch_timeline_idx", "crm_interactions_type_timeline_idx", "crm_interactions_recorder_idx", "crm_tasks_customer_status_idx", "crm_tasks_branch_status_idx", "crm_tasks_assignee_status_idx", "crm_tasks_status_due_idx", "crm_tasks_branch_status_due_idx"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname=$1)`, index).Scan(&exists); err != nil || !exists {
			t.Fatalf("index %s exists=%t err=%v", index, exists, err)
		}
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN ('vendor.crm.interactions.read','vendor.crm.interactions.create','vendor.crm.interactions.manage','vendor.crm.tasks.read','vendor.crm.tasks.create','vendor.crm.tasks.manage')`).Scan(&permissions); err != nil || permissions != 6 {
		t.Fatalf("Section 8C permissions=%d err=%v", permissions, err)
	}
	// Loyalty was future work when Section 8C landed and is now Section 8D's;
	// campaigns were future work and are now Section 8E's. Neither is forbidden
	// any longer. The authority to send is still unbuilt and must stay so.
	var forbidden int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code LIKE '%.send%'`).Scan(&forbidden); err != nil || forbidden != 0 {
		t.Fatalf("send permissions leaked count=%d err=%v", forbidden, err)
	}
}

// TestSection8DMigrationsRollbackAndReapplyAsAPair proves the loyalty schema
// and its permissions install, roll back cleanly to the accepted Section 8C
// baseline, and reapply â€” and that rolling 8D back damages nothing behind it.
func TestSection8DMigrationsRollbackAndReapplyAsAPair(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 8D replay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	section8DTables := []string{"crm_loyalty_program_configs", "crm_loyalty_accounts",
		"crm_loyalty_entries", "crm_loyalty_point_lots", "crm_loyalty_lot_consumptions"}

	rollbackUntilRemoved(t, ctx, pool, "000016_vendor_crm_loyalty")
	for _, table := range section8DTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("Section 8D table %s survived rollback exists=%t err=%v", table, exists, err)
		}
	}
	// Everything Sections 5-8C own must be untouched by an 8D rollback.
	for _, table := range []string{"crm_tasks", "crm_interactions", "crm_tags", "crm_segments",
		"vendor_customer_profiles", "vendor_staff", "vendor_businesses"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("accepted baseline table %s damaged by an 8D rollback exists=%t err=%v", table, exists, err)
		}
	}
	var permissions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code LIKE 'vendor.crm.loyalty.%'`).Scan(&permissions); err != nil || permissions != 0 {
		t.Fatalf("Section 8D permissions survived rollback count=%d err=%v", permissions, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions
		WHERE code LIKE 'vendor.crm.interactions.%' OR code LIKE 'vendor.crm.tasks.%'`).Scan(&permissions); err != nil || permissions != 6 {
		t.Fatalf("Section 8C permissions damaged by an 8D rollback count=%d err=%v", permissions, err)
	}

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("8C to 8D upgrade/reapply: %v", err)
	}
	for _, table := range section8DTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("reapplied table %s exists=%t err=%v", table, exists, err)
		}
	}

	// Controlled vocabularies live in the schema, not only in Go.
	for constraint, values := range map[string][]string{
		"crm_loyalty_program_configs_status_check":        {"DISABLED", "ACTIVE", "PAUSED"},
		"crm_loyalty_program_configs_expiry_policy_check": {"NEVER", "FIXED_DAYS"},
		"crm_loyalty_accounts_status_check":               {"ACTIVE", "MERGED", "REDACTED"},
		"crm_loyalty_entries_entry_type_check": {"EARN", "REDEEM", "EXPIRE", "ADJUSTMENT_CREDIT",
			"ADJUSTMENT_DEBIT", "REVERSAL", "MERGE_IN", "MERGE_OUT"},
		"crm_loyalty_entries_source_type_check": {"MANUAL_ADJUSTMENT", "ORDER", "SYSTEM_EXPIRY",
			"REVERSAL", "IDENTITY_MERGE", "EXTERNAL"},
	} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		for _, value := range values {
			if !strings.Contains(definition, "'"+value+"'") {
				t.Fatalf("%s missing %s: %s", constraint, value, definition)
			}
		}
	}

	// The invariants that keep a points balance honest are database
	// constraints, not merely application checks.
	for _, constraint := range []string{
		"crm_loyalty_accounts_current_points_check", // balance never negative
		"crm_loyalty_account_balance_ck",            // aggregate agrees with the ledger
		"crm_loyalty_entries_balance_after_check",   // a posted balance is never negative
		"crm_loyalty_entries_delta_points_check",    // no zero-delta entry
		"crm_loyalty_entry_direction_ck",            // credit types credit, debit types debit
		"crm_loyalty_entry_reversal_ck",             // only a REVERSAL references an original
		"crm_loyalty_lot_remaining_ck",              // remaining never exceeds issued
		"crm_loyalty_consumption_restore_ck",        // restored never exceeds consumed
		"crm_loyalty_program_expiry_ck",             // policy and window agree
	} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname=$1)`, constraint).Scan(&exists); err != nil || !exists {
			t.Fatalf("constraint %s exists=%t err=%v", constraint, exists, err)
		}
	}

	// Tenant-composite foreign keys: a loyalty row can never reference another
	// business's account, entry or lot.
	for _, constraint := range []string{"crm_loyalty_account_profile_fk", "crm_loyalty_entry_account_fk",
		"crm_loyalty_entry_reverses_fk", "crm_loyalty_entry_staff_fk", "crm_loyalty_lot_account_fk",
		"crm_loyalty_lot_entry_fk", "crm_loyalty_consumption_account_fk", "crm_loyalty_consumption_entry_fk",
		"crm_loyalty_consumption_lot_fk"} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		if !strings.Contains(definition, "business_id") {
			t.Fatalf("%s is not tenant-composite: %s", constraint, definition)
		}
	}

	// Uniqueness that the concurrency design depends on.
	for _, index := range []string{
		"crm_loyalty_entries_one_reversal_uk", // exactly one reversal per entry
		"crm_loyalty_entries_command_uk",      // durable command-level exactly-once
		"crm_loyalty_entries_source_event_uk", // a redelivered producer event credits once
		"crm_loyalty_accounts_status_idx",
		"crm_loyalty_entries_account_sequence_idx",
		"crm_loyalty_entries_account_created_idx",
		"crm_loyalty_entries_account_type_idx",
		"crm_loyalty_point_lots_fefo_idx",
		"crm_loyalty_point_lots_due_idx",
		"crm_loyalty_point_lots_source_idx",
		"crm_loyalty_lot_consumptions_entry_idx",
		"crm_loyalty_lot_consumptions_lot_idx",
	} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname=$1)`, index).Scan(&exists); err != nil || !exists {
			t.Fatalf("index %s exists=%t err=%v", index, exists, err)
		}
	}

	// One programme per business and one account per relationship.
	for _, unique := range []struct{ table, columns string }{
		{"crm_loyalty_program_configs", "business_id"},
		{"crm_loyalty_accounts", "business_id, customer_profile_id"},
		{"crm_loyalty_entries", "business_id, loyalty_account_id, sequence"},
		{"crm_loyalty_lot_consumptions", "business_id, debit_entry_id, lot_id"},
	} {
		var found bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint c
			JOIN pg_class t ON t.oid=c.conrelid
			WHERE t.relname=$1 AND c.contype='u' AND pg_get_constraintdef(c.oid)='UNIQUE ('||$2||')')`,
			unique.table, unique.columns).Scan(&found); err != nil || !found {
			t.Fatalf("%s is missing UNIQUE (%s) found=%t err=%v", unique.table, unique.columns, found, err)
		}
	}

	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN
		('vendor.crm.loyalty.read','vendor.crm.loyalty.manage','vendor.crm.loyalty.adjust','vendor.crm.loyalty.reverse')`).Scan(&permissions); err != nil || permissions != 4 {
		t.Fatalf("Section 8D permissions=%d err=%v", permissions, err)
	}
	// Adjustment and reversal are marked high-risk; reading a balance is not.
	//
	// This names Section 8D's own four permissions rather than matching the
	// `vendor.crm.loyalty.` prefix, because later sections legitimately add
	// loyalty permissions of their own (the Section 8 completion adds a
	// high-risk `vendor.crm.loyalty.tiers.manage`) and a prefix count would
	// fail on their arrival without anything about Section 8D having changed.
	var highRisk int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions
		WHERE code IN ('vendor.crm.loyalty.read','vendor.crm.loyalty.manage',
		               'vendor.crm.loyalty.adjust','vendor.crm.loyalty.reverse')
		  AND is_high_risk`).Scan(&highRisk); err != nil || highRisk != 2 {
		t.Fatalf("high-risk Section 8D loyalty permissions=%d; want adjust and reverse err=%v", highRisk, err)
	}
	// Points-moving authority is not granted broadly.
	var overreach int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE p.code IN ('vendor.crm.loyalty.adjust','vendor.crm.loyalty.reverse')
		  AND rp.role_id NOT IN ('role_owner','role_general_manager','role_crm_support')`).Scan(&overreach); err != nil || overreach != 0 {
		t.Fatalf("roles beyond owner/GM/CRM hold points-moving authority count=%d err=%v", overreach, err)
	}
	// Operations roles hold no loyalty permission at all.
	var operational int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE rp.role_id IN ('role_kitchen_pastry','role_shop_inventory','role_procurement','role_vendor_rider')
		  AND p.code LIKE 'vendor.crm.loyalty.%'`).Scan(&operational); err != nil || operational != 0 {
		t.Fatalf("operational roles hold loyalty permissions count=%d err=%v", operational, err)
	}
	// Branch manager, marketing, analyst and finance are read-only.
	var readOnlyWrites int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE rp.role_id IN ('role_branch_manager','role_marketing','role_analyst_auditor','role_finance')
		  AND p.code LIKE 'vendor.crm.loyalty.%' AND p.code <> 'vendor.crm.loyalty.read'`).Scan(&readOnlyWrites); err != nil || readOnlyWrites != 0 {
		t.Fatalf("a read-only role holds a loyalty write permission count=%d err=%v", readOnlyWrites, err)
	}
	// What is still genuinely unbuilt stays out. Campaigns landed in Section 8E
	// and loyalty tiers in the Section 8 completion, so neither is a marker of
	// future work any more; a direct send permission and a rewards engine still
	// are. Sending is worker-driven under Section 8F's own grants â€” there is no
	// permission that hands a staff member a send button â€” and rewards would be
	// a commerce feature, because a tier that conferred value would make points
	// money and payment-wallet-service owns money.
	var future int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions
		WHERE code LIKE '%.send%' OR code LIKE '%reward%'`).Scan(&future); err != nil || future != 0 {
		t.Fatalf("later-section permissions leaked count=%d err=%v", future, err)
	}

	// The loyalty schema stores no money and no personal data.
	var suspicious int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name LIKE 'crm_loyalty%'
		  AND (column_name ~ '(email|phone|address|first_name|last_name|password)'
			OR column_name ~ '(currency|amount_minor|kobo|wallet|payout|settlement)')`).Scan(&suspicious); err != nil || suspicious != 0 {
		t.Fatalf("loyalty schema holds personal or monetary columns count=%d err=%v", suspicious, err)
	}
}

func TestSection8FMigrationsRollbackAndReapplyAsAPair(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 8F replay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	tables := []string{"crm_campaign_runs", "crm_campaign_deliveries", "crm_campaign_delivery_history", "crm_campaign_contact_history", "crm_campaign_delivery_event_inbox"}
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("Section 8F table %s exists=%t err=%v", table, exists, err)
		}
	}
	var permissions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN ('vendor.crm.campaigns.execute','vendor.crm.campaigns.runs.read','vendor.crm.campaigns.runs.manage','vendor.crm.campaigns.deliveries.read')`).Scan(&permissions); err != nil || permissions != 4 {
		t.Fatalf("Section 8F permission count=%d err=%v", permissions, err)
	}
	rollbackUntilRemoved(t, ctx, pool, "000020_vendor_crm_campaign_execution")
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("Section 8F table %s survived rollback exists=%t err=%v", table, exists, err)
		}
	}
	for _, table := range []string{"crm_campaigns", "crm_campaign_audience_snapshots", "crm_campaign_targets"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("Section 8E baseline %s damaged exists=%t err=%v", table, exists, err)
		}
	}
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("8E to 8F reapply: %v", err)
	}
	for _, table := range tables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("reapplied table %s exists=%t err=%v", table, exists, err)
		}
	}
}
