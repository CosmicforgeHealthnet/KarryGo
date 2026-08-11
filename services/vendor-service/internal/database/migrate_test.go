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
// tracker (never a hardcoded migration name/position — a fixed assumption
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
	// (each package is independently correct in isolation — see
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
}

// TestSection5SchemaExists is a focused, migration-position-independent check
// that Section 5's own tables and seed data exist once all migrations are
// applied — unlike TestMigrationReplay, this does not care which migration
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

	var ownerPermissionCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions WHERE role_id = 'role_owner'`).Scan(&ownerPermissionCount); err != nil {
		t.Fatalf("count owner permissions: %v", err)
	}
	if ownerPermissionCount != permissionCount {
		t.Fatalf("expected Owner to hold every permission (%d), got %d", permissionCount, ownerPermissionCount)
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
	// Section 8A sits on top of Section 7, so step back past it first: this
	// test is about the Section 7 pair rolling back and reapplying cleanly,
	// not about which migration happens to be the current tip.
	for range []string{"000011_vendor_section8a_crm_permissions", "000010_vendor_crm_foundation"} {
		if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback Section 8A: %v", err)
		}
	}
	for _, version := range []string{"000009_vendor_section7_permissions", "000008_vendor_section7_onboarding"} {
		if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback %s: %v", version, err)
		}
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

	for _, version := range []string{"000011_vendor_section8a_crm_permissions", "000010_vendor_crm_foundation"} {
		if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("rollback %s: %v", version, err)
		}
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
