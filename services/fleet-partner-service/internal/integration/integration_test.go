// Package integration_test holds PostgreSQL repository integration tests.
//
// The suite is gated on FLEET_PARTNER_TEST_DATABASE_URL. It applies the real
// migrations into the target database (use a disposable database), exercises
// tenant isolation, optimistic concurrency, outbox atomicity, inbox
// deduplication, idempotency and durable-job leasing, then rolls migrations
// back. Tests skip cleanly when the variable is unset.
package integration_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

const migrationsDir = "../../migrations"

// migrateOnce guards one-time schema setup for the whole test binary.
var (
	migrateOnce sync.Once
	migrateErr  error
	// Tests in this package deliberately share one disposable PostgreSQL
	// database and reset it with TRUNCATE. Several tests use t.Parallel, so one
	// test must own that database until its cleanup completes; otherwise a
	// concurrent reset can delete another test's organization mid-assertion or
	// deadlock two catalogue seed passes.
	databaseTestMu sync.Mutex
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseTestMu.Lock()
	t.Cleanup(databaseTestMu.Unlock)
	rawURL := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL not set; skipping PostgreSQL integration tests (CI must always set it)")
	}
	requireTestDatabaseName(t, rawURL)

	pool, err := database.NewPool(context.Background(), rawURL, database.PoolOptions{MaxConns: 10})
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	// Also serialize against a second `go test` process pointed at the same
	// disposable database (for example an IDE run overlapping the required
	// repository-wide command). A process-local mutex cannot protect that
	// boundary.
	lockConn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire database test lock connection: %v", err)
	}
	if _, err = lockConn.Exec(context.Background(), `SELECT pg_advisory_lock(hashtext('fleet-partner-integration-suite'))`); err != nil {
		lockConn.Release()
		t.Fatalf("acquire database test advisory lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('fleet-partner-integration-suite'))`)
		lockConn.Release()
	})

	// Migrations are applied ONCE per test process, not once per test.
	//
	// They are idempotent, so re-applying them 100+ times was correct but
	// wasteful: it dominated the suite's runtime without changing any
	// outcome. The per-test clean slate below is what actually provides
	// isolation.
	migrateOnce.Do(func() {
		_, migrateErr = pool.Exec(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
		if migrateErr == nil {
			migrateErr = database.ApplyMigrations(context.Background(), pool, migrationsDir)
		}
		if migrateErr == nil {
			migrateErr = restoreCatalogueFromMigrations(context.Background(), pool)
		}
		if migrateErr == nil {
			migrateErr = snapshotCatalogueSeed(context.Background(), pool)
		}
	})
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}

	// Clean slate per test so the suite reruns safely against the same
	// disposable database. TRUNCATE bypasses row-level append-only triggers
	// by design — it is test tooling, not a runtime path.
	// The table list is derived from the live schema rather than hand-written.
	// A hand-maintained list silently stops cleaning tables that a later
	// migration adds, and stale rows then surface as confusing failures in
	// unrelated tests — that has already happened twice in this service.
	// Deriving it means a new migration is covered the moment it is applied.
	rows, err := pool.Query(context.Background(), `
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public'
		  AND tablename LIKE 'fleet\_partner\_%'
		  AND tablename <> 'fleet_partner_schema_migrations'`)
	if err != nil {
		t.Fatalf("list test tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			rows.Close()
			t.Fatalf("scan test table name: %v", scanErr)
		}
		tables = append(tables, pgx.Identifier{name}.Sanitize())
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("list test tables: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("no fleet_partner tables found; migrations did not apply")
	}
	// TRUNCATE bypasses row-level append-only triggers by design — it is test
	// tooling, not a runtime path.
	if err := resetTestData(context.Background(), pool, tables); err != nil {
		t.Fatalf("reset test database: %v", err)
	}

	// Truncating wipes the seeded platform catalogue (system role templates,
	// permissions and their grants). Re-run the seeding migrations — their
	// seed statements are idempotent — to restore it.
	return pool
}

var catalogueSeedTables = []string{
	"fleet_partner_permissions",
	"fleet_partner_roles",
	"fleet_partner_eligibility_reason_codes",
	"fleet_partner_eligibility_policies",
	"fleet_partner_role_permissions",
}

func restoreCatalogueFromMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	for _, seedMigration := range []string{
		"000005_staff_and_iam.up.sql",
		"000010_trip_projections_and_permissions.up.sql",
		"000014_safety_references.up.sql",
		"000018_finance_operator_access.up.sql",
		"000019_eligibility_decisions.up.sql",
		"000021_fleet_documents.up.sql",
	} {
		seed, err := os.ReadFile(filepath.Join(migrationsDir, seedMigration))
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(seed)); err != nil {
			return err
		}
	}
	return nil
}

func snapshotCatalogueSeed(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS fleet_partner_test_seed CASCADE; CREATE SCHEMA fleet_partner_test_seed`); err != nil {
		return err
	}
	for _, table := range catalogueSeedTables {
		identifier := pgx.Identifier{table}.Sanitize()
		backup := pgx.Identifier{"fleet_partner_test_seed", table}.Sanitize()
		if _, err := pool.Exec(ctx, "CREATE TABLE "+backup+" (LIKE "+identifier+" INCLUDING ALL); INSERT INTO "+backup+" SELECT * FROM "+identifier); err != nil {
			return err
		}
	}
	return nil
}

func resetTestData(ctx context.Context, pool *pgxpool.Pool, tables []string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// This runs only against the explicitly validated disposable test database.
	// Disabling constraint and append-only triggers for this transaction avoids
	// the catalogue-lock cost of TRUNCATE CASCADE across the large FK graph.
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		return err
	}
	for _, table := range tables {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}
	for _, table := range catalogueSeedTables {
		identifier := pgx.Identifier{table}.Sanitize()
		backup := pgx.Identifier{"fleet_partner_test_seed", table}.Sanitize()
		if _, err := tx.Exec(ctx, "INSERT INTO "+identifier+" SELECT * FROM "+backup); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// requireTestDatabaseName refuses to run against anything that does not look
// like a dedicated disposable test database.
func requireTestDatabaseName(t *testing.T, rawURL string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("FLEET_PARTNER_TEST_DATABASE_URL is not a valid URL")
	}
	name := strings.ToLower(strings.TrimPrefix(parsed.Path, "/"))
	if !strings.Contains(name, "test") {
		t.Fatalf("refusing to run: database name %q does not look test-specific (must contain \"test\")", name)
	}
	for _, forbidden := range []string{"prod", "production", "staging", "live"} {
		if strings.Contains(name, forbidden) {
			t.Fatalf("refusing to run: database name %q looks like a %s database", name, forbidden)
		}
	}
}

func createOrganization(t *testing.T, pool *pgxpool.Pool, legalName string, registration string) *models.Organization {
	t.Helper()
	repo := repositories.NewOrganizationRepository(pool)
	org := &models.Organization{
		LegalName:          legalName,
		RegistrationNumber: registration,
		CountryCode:        "NG",
		Timezone:           "Africa/Lagos",
		DefaultCurrency:    "NGN",
	}
	if err := repo.Create(context.Background(), pool, org); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	return org
}

func TestMigrationsReplayAndRollback(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Fresh apply already ran in testPool; verify current.
	if err := database.MigrationsCurrent(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("migrations not current after apply: %v", err)
	}
	// Re-apply is a no-op.
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	// Roll back the last migration, then re-apply.
	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := database.MigrationsCurrent(ctx, pool, migrationsDir); err == nil {
		t.Fatal("migrations should not be current after rollback")
	}
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("re-apply after rollback: %v", err)
	}
	if err := database.MigrationsCurrent(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("migrations not current after re-apply: %v", err)
	}
}

func TestOrganizationFoundationAndTenantScopedLookup(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := repositories.NewOrganizationRepository(pool)

	org := createOrganization(t, pool, "Acme Logistics Ltd", "RC-100001")
	loaded, err := repo.GetByID(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.LegalName != "Acme Logistics Ltd" || loaded.Version != 1 || loaded.Status != models.OrganizationStatusPendingReview {
		t.Fatalf("loaded = %+v", loaded)
	}

	if _, err := repo.GetByID(ctx, "org_00000000000000000000000000000000"); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("unknown org should be ErrNotFound, got %v", err)
	}
}

func TestRegistrationUniquenessPerCountry(t *testing.T) {
	pool := testPool(t)
	createOrganization(t, pool, "Dup One", "RC-DUP-1")
	repo := repositories.NewOrganizationRepository(pool)
	err := repo.Create(context.Background(), pool, &models.Organization{
		LegalName: "Dup Two", RegistrationNumber: "RC-DUP-1",
		CountryCode: "NG", Timezone: "Africa/Lagos", DefaultCurrency: "NGN",
	})
	if err == nil {
		t.Fatal("duplicate registration in same country must fail")
	}
}

func TestBranchTenantFilteringAndOptimisticVersion(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Org A", "RC-A-1")
	orgB := createOrganization(t, pool, "Org B", "RC-B-1")
	branches := repositories.NewBranchRepository(pool)

	branch := &models.Branch{Code: "LAG-01", Name: "Lagos Hub", Type: "HUB", Timezone: "Africa/Lagos"}
	if err := branches.Create(ctx, pool, orgA.ID, branch); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	// Cross-organization access is indistinguishable from not-found.
	if _, err := branches.GetByID(ctx, orgB.ID, branch.ID); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("cross-tenant read must be ErrNotFound, got %v", err)
	}
	if err := branches.Archive(ctx, orgB.ID, branch.ID, 1); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("cross-tenant archive must be ErrNotFound, got %v", err)
	}

	// Tenant-scoped list contains only own branches.
	listed, err := branches.List(ctx, tenant.Query{OrganizationID: orgB.ID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("org B sees %d foreign branches", len(listed))
	}

	// Optimistic concurrency: wrong expected version conflicts.
	if err := branches.Archive(ctx, orgA.ID, branch.ID, 99); !errors.Is(err, concurrency.ErrVersionMismatch) {
		t.Fatalf("wrong version must be ErrVersionMismatch, got %v", err)
	}
	if err := branches.Archive(ctx, orgA.ID, branch.ID, 1); err != nil {
		t.Fatalf("archive with correct version: %v", err)
	}

	// Branch code composite uniqueness inside the organization.
	err = branches.Create(ctx, pool, orgA.ID, &models.Branch{Code: "LAG-01", Name: "Duplicate", Type: "HUB", Timezone: "Africa/Lagos"})
	if err == nil {
		t.Fatal("duplicate branch code in same organization must fail")
	}
	// The same code in another organization is fine.
	if err := branches.Create(ctx, pool, orgB.ID, &models.Branch{Code: "LAG-01", Name: "Org B Lagos", Type: "HUB", Timezone: "Africa/Lagos"}); err != nil {
		t.Fatalf("same code in other organization should succeed: %v", err)
	}
}

func TestDepartmentParentTenantMismatch(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Dept Org A", "RC-DA-1")
	orgB := createOrganization(t, pool, "Dept Org B", "RC-DB-1")
	departments := repositories.NewDepartmentRepository(pool)

	parent := &models.Department{Code: "OPS", Name: "Operations"}
	if err := departments.Create(ctx, pool, orgA.ID, parent); err != nil {
		t.Fatalf("create parent: %v", err)
	}

	// Parent from another organization is rejected.
	child := &models.Department{Code: "OPS-1", Name: "Ops North", ParentID: &parent.ID}
	err := departments.Create(ctx, pool, orgB.ID, child)
	if !errors.Is(err, repositories.ErrTenantMismatch) {
		t.Fatalf("cross-tenant parent must be ErrTenantMismatch, got %v", err)
	}

	// Branch from another organization is rejected.
	branches := repositories.NewBranchRepository(pool)
	branch := &models.Branch{Code: "AB-01", Name: "A Branch", Type: "BRANCH", Timezone: "Africa/Lagos"}
	if err := branches.Create(ctx, pool, orgA.ID, branch); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	withForeignBranch := &models.Department{Code: "FIN", Name: "Finance", BranchID: &branch.ID}
	err = departments.Create(ctx, pool, orgB.ID, withForeignBranch)
	if !errors.Is(err, repositories.ErrTenantMismatch) {
		t.Fatalf("cross-tenant branch must be ErrTenantMismatch, got %v", err)
	}
}

func TestStatusHistoryAppendOnly(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "History Org", "RC-H-1")
	repo := repositories.NewOrganizationRepository(pool)

	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.UpdateStatus(ctx, tx, org.ID, 1, models.StatusChange{
			PreviousStatus: models.OrganizationStatusPendingReview,
			NewStatus:      models.OrganizationStatusActive,
			ActorType:      "platform_service",
			ActorID:        "admin-backoffice-service",
			ReasonCode:     "review_approved",
			RequestID:      "req_1",
			CorrelationID:  "cor_1",
		})
	})
	if err != nil {
		t.Fatalf("status transition: %v", err)
	}

	history, err := repo.StatusHistory(ctx, org.ID)
	if err != nil || len(history) != 1 {
		t.Fatalf("history = %d, %v", len(history), err)
	}

	// Append-only trigger blocks UPDATE and DELETE.
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_organization_status_history SET reason_code = 'tampered' WHERE id = $1`, history[0].ID); err == nil {
		t.Fatal("status history update must be blocked")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM fleet_partner_organization_status_history WHERE id = $1`, history[0].ID); err == nil {
		t.Fatal("status history delete must be blocked")
	}
	// Audit records table is equally append-only.
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_audit_records (id, organization_id, actor_type, actor_id, action, resource_type, resource_id, request_id, correlation_id)
		VALUES ('aud_t1', $1, 'system', 's', 'test', 'organization', $1, 'req', 'cor')`, org.ID); err != nil {
		t.Fatalf("insert audit: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM fleet_partner_audit_records WHERE id = 'aud_t1'`); err == nil {
		t.Fatal("audit delete must be blocked")
	}
}

func TestOutboxTransactionAtomicity(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Outbox Org", "RC-O-1")
	outbox := events.NewOutboxRepository(pool)

	envelope, err := events.NewEnvelope("corporate.test.created", 1, "organization", org.ID, 1, org.ID, "cor_1", map[string]string{"k": "v"}, false)
	if err != nil {
		t.Fatalf("envelope: %v", err)
	}

	// Rolled-back transaction leaves no outbox row.
	rollbackErr := errors.New("force rollback")
	err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := outbox.Append(ctx, tx, envelope); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("expected forced rollback, got %v", err)
	}
	var count int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE id = $1`, envelope.EventID).Scan(&count)
	if count != 0 {
		t.Fatal("outbox row survived a rolled-back transaction")
	}

	// Committed transaction persists the event; claim leases it exactly once.
	err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return outbox.Append(ctx, tx, envelope)
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	claimed, err := outbox.ClaimBatch(ctx, "worker-1", time.Minute, 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	found := false
	for _, record := range claimed {
		if record.ID == envelope.EventID {
			found = true
		}
	}
	if !found {
		t.Fatal("appended event not claimable")
	}
	// A second claimant must not receive the leased record.
	claimedAgain, err := outbox.ClaimBatch(ctx, "worker-2", time.Minute, 10)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	for _, record := range claimedAgain {
		if record.ID == envelope.EventID {
			t.Fatal("leased outbox record claimed twice")
		}
	}
	// Owner-only completion.
	if err := outbox.MarkPublished(ctx, "worker-2", envelope.EventID); err == nil {
		t.Fatal("non-owner must not complete an outbox lease")
	}
	if err := outbox.MarkPublished(ctx, "worker-1", envelope.EventID); err != nil {
		t.Fatalf("owner completion failed: %v", err)
	}
}

func TestInboxDeduplication(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	inbox := events.NewInboxRepository(pool)
	payload := []byte(`{"organizationId":"org_x","value":1}`)

	// First delivery processes and completes.
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := inbox.Begin(ctx, tx, "corporate-consumer", "evt_1", "corporate.test", 1, payload); err != nil {
			return err
		}
		return inbox.MarkProcessed(ctx, tx, "corporate-consumer", "evt_1")
	})
	if err != nil {
		t.Fatalf("first delivery: %v", err)
	}

	// Duplicate delivery is acknowledged safely.
	err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return inbox.Begin(ctx, tx, "corporate-consumer", "evt_1", "corporate.test", 1, payload)
	})
	if !errors.Is(err, events.ErrDuplicateEvent) {
		t.Fatalf("duplicate should be ErrDuplicateEvent, got %v", err)
	}

	// Same ID, different payload: contract/security error.
	err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return inbox.Begin(ctx, tx, "corporate-consumer", "evt_1", "corporate.test", 1, []byte(`{"tampered":true}`))
	})
	if !errors.Is(err, events.ErrPayloadMismatch) {
		t.Fatalf("tampered payload should be ErrPayloadMismatch, got %v", err)
	}

	// Failed processing stays recoverable: Begin again succeeds.
	err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return inbox.Begin(ctx, tx, "corporate-consumer", "evt_2", "corporate.test", 1, payload)
	})
	if err != nil {
		t.Fatalf("begin evt_2: %v", err)
	}
	if err := inbox.MarkFailed(ctx, "corporate-consumer", "evt_2", "handler_error"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := inbox.Begin(ctx, tx, "corporate-consumer", "evt_2", "corporate.test", 1, payload); err != nil {
			return err
		}
		return inbox.MarkProcessed(ctx, tx, "corporate-consumer", "evt_2")
	})
	if err != nil {
		t.Fatalf("retry after failure: %v", err)
	}

	// Concurrent duplicate delivery: exactly one processes.
	var wg sync.WaitGroup
	results := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			results[slot] = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
				if err := inbox.Begin(ctx, tx, "corporate-consumer", "evt_concurrent", "corporate.test", 1, payload); err != nil {
					return err
				}
				return inbox.MarkProcessed(ctx, tx, "corporate-consumer", "evt_concurrent")
			})
		}(i)
	}
	wg.Wait()
	succeeded, duplicates := 0, 0
	for _, resultErr := range results {
		switch {
		case resultErr == nil:
			succeeded++
		case errors.Is(resultErr, events.ErrDuplicateEvent):
			duplicates++
		default:
			t.Fatalf("unexpected concurrent result: %v", resultErr)
		}
	}
	if succeeded != 1 || duplicates != 3 {
		t.Fatalf("concurrent dedupe: succeeded=%d duplicates=%d", succeeded, duplicates)
	}
}

func TestIdempotencyFoundation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	store := idempotency.NewStore(pool, time.Hour)
	org := "org_idem_test"
	endpoint := "POST /fleet-partner/v1/things"
	fingerprint := idempotency.Fingerprint("POST", endpoint, []byte(`{"a":1}`))

	// First use claims.
	record, err := store.Begin(ctx, org, endpoint, "key-1", fingerprint)
	if err != nil || record != nil {
		t.Fatalf("first Begin = %+v, %v", record, err)
	}
	// Same key while in progress conflicts.
	if _, err := store.Begin(ctx, org, endpoint, "key-1", fingerprint); !errors.Is(err, idempotency.ErrInProgress) {
		t.Fatalf("in-progress reuse = %v", err)
	}
	// Complete stores the response for replay.
	if err := store.Complete(ctx, org, endpoint, "key-1", 201, []byte(`{"data":{"id":"thing_1"}}`)); err != nil {
		t.Fatalf("complete: %v", err)
	}
	record, err = store.Begin(ctx, org, endpoint, "key-1", fingerprint)
	if err != nil || record == nil || record.ResponseStatus != 201 {
		t.Fatalf("replay = %+v, %v", record, err)
	}
	// Same key, different request: conflict.
	otherFingerprint := idempotency.Fingerprint("POST", endpoint, []byte(`{"a":2}`))
	if _, err := store.Begin(ctx, org, endpoint, "key-1", otherFingerprint); !errors.Is(err, idempotency.ErrFingerprintMismatch) {
		t.Fatalf("fingerprint conflict = %v", err)
	}
	// Server failures are not cached: key released and reusable.
	record, err = store.Begin(ctx, org, endpoint, "key-500", fingerprint)
	if err != nil || record != nil {
		t.Fatalf("claim key-500 = %+v, %v", record, err)
	}
	if err := store.Complete(ctx, org, endpoint, "key-500", 500, nil); err != nil {
		t.Fatalf("complete 500: %v", err)
	}
	record, err = store.Begin(ctx, org, endpoint, "key-500", fingerprint)
	if err != nil || record != nil {
		t.Fatalf("500 must not be replayed: %+v, %v", record, err)
	}

	// Concurrency: exactly one of N concurrent claims wins.
	var wg sync.WaitGroup
	wins := make([]bool, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			record, err := store.Begin(ctx, org, endpoint, "key-race", fingerprint)
			wins[slot] = err == nil && record == nil
		}(i)
	}
	wg.Wait()
	winners := 0
	for _, won := range wins {
		if won {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent idempotency claims: winners=%d", winners)
	}
}

func TestDurableJobsLeasing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Jobs Org", "RC-J-1")

	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID,
		JobType:        "test.reconcile",
		DedupeKey:      "reconcile-1",
		Payload:        map[string]string{"organizationId": org.ID},
		MaxAttempts:    2,
	})
	if err != nil || jobID == "" {
		t.Fatalf("enqueue: %s, %v", jobID, err)
	}
	// Dedupe: same active key returns the same job.
	dupID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID,
		JobType:        "test.reconcile",
		DedupeKey:      "reconcile-1",
		Payload:        map[string]string{"organizationId": org.ID},
	})
	if err != nil || dupID != jobID {
		t.Fatalf("dedupe: %s vs %s, %v", dupID, jobID, err)
	}

	// Concurrent claim with SKIP LOCKED: one worker gets the job.
	var wg sync.WaitGroup
	claims := make([][]jobs.Job, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			claimed, claimErr := queue.ClaimBatch(ctx, "worker-"+string(rune('a'+slot)), time.Minute, 5)
			if claimErr != nil {
				t.Errorf("claim: %v", claimErr)
			}
			claims[slot] = claimed
		}(i)
	}
	wg.Wait()
	totalForJob, owner := 0, ""
	for slot, claimed := range claims {
		for _, job := range claimed {
			if job.ID == jobID {
				totalForJob++
				owner = "worker-" + string(rune('a'+slot))
			}
		}
	}
	if totalForJob != 1 {
		t.Fatalf("job claimed %d times", totalForJob)
	}

	// Tenant revalidation passes for the matching payload.
	for _, claimed := range claims {
		for _, job := range claimed {
			if job.ID == jobID {
				if err := jobs.ValidateTenant(job); err != nil {
					t.Fatalf("tenant validation: %v", err)
				}
			}
		}
	}

	// Owner-only heartbeat.
	if err := queue.Heartbeat(ctx, "intruder", jobID, time.Minute); !errors.Is(err, jobs.ErrNotLeaseOwner) {
		t.Fatalf("intruder heartbeat = %v", err)
	}
	if err := queue.Heartbeat(ctx, owner, jobID, time.Minute); err != nil {
		t.Fatalf("owner heartbeat: %v", err)
	}
	// Owner-only completion.
	if err := queue.Complete(ctx, "intruder", jobID); !errors.Is(err, jobs.ErrNotLeaseOwner) {
		t.Fatalf("intruder complete = %v", err)
	}
	if err := queue.Complete(ctx, owner, jobID); err != nil {
		t.Fatalf("owner complete: %v", err)
	}
}

func TestExpiredLeaseRecovery(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Lease Org", "RC-L-1")

	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID,
		JobType:        "test.lease",
		Payload:        map[string]string{"organizationId": org.ID},
		MaxAttempts:    5,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Crash simulation: claim with an immediately-expiring lease.
	claimed, err := queue.ClaimBatch(ctx, "crashed-worker", time.Millisecond, 5)
	if err != nil || !containsJob(claimed, jobID) {
		t.Fatalf("initial claim: %v (%d)", err, len(claimed))
	}
	time.Sleep(50 * time.Millisecond)

	// Another worker recovers the expired lease.
	recovered, err := queue.ClaimBatch(ctx, "recovery-worker", time.Minute, 5)
	if err != nil || !containsJob(recovered, jobID) {
		t.Fatalf("lease recovery failed: %v (%d)", err, len(recovered))
	}
	// The crashed worker can no longer complete it.
	if err := queue.Complete(ctx, "crashed-worker", jobID); !errors.Is(err, jobs.ErrNotLeaseOwner) {
		t.Fatalf("crashed worker completion = %v", err)
	}
	if err := queue.Complete(ctx, "recovery-worker", jobID); err != nil {
		t.Fatalf("recovery worker completion: %v", err)
	}
}

func TestJobRetryAndDeadLetter(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)

	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		JobType:     "test.retry",
		Payload:     map[string]string{},
		MaxAttempts: 2,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Attempt 1 fails -> RETRY_SCHEDULED.
	claimed, err := queue.ClaimBatch(ctx, "w1", time.Minute, 5)
	if err != nil || !containsJob(claimed, jobID) {
		t.Fatalf("claim 1: %v", err)
	}
	if err := queue.Fail(ctx, "w1", jobID, errors.New("boom")); err != nil {
		t.Fatalf("fail 1: %v", err)
	}
	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM fleet_partner_jobs WHERE id = $1`, jobID).Scan(&status)
	if status != jobs.StatusRetryScheduled {
		t.Fatalf("after first failure status = %s", status)
	}

	// Make it due now, fail again -> DEAD_LETTERED (attempts == max).
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_jobs SET scheduled_at = now() WHERE id = $1`, jobID); err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	claimed, err = queue.ClaimBatch(ctx, "w2", time.Minute, 5)
	if err != nil || !containsJob(claimed, jobID) {
		t.Fatalf("claim 2: %v", err)
	}
	if err := queue.Fail(ctx, "w2", jobID, errors.New("boom again")); err != nil {
		t.Fatalf("fail 2: %v", err)
	}
	_ = pool.QueryRow(ctx, `SELECT status FROM fleet_partner_jobs WHERE id = $1`, jobID).Scan(&status)
	if status != jobs.StatusDeadLettered {
		t.Fatalf("after exhausting attempts status = %s", status)
	}
}

func containsJob(claimed []jobs.Job, jobID string) bool {
	for _, job := range claimed {
		if job.ID == jobID {
			return true
		}
	}
	return false
}
