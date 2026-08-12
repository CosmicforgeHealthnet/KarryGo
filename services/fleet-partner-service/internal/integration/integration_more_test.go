package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"

	"github.com/jackc/pgx/v5"
)

func TestSchemaObjectsExist(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	for _, table := range []string{
		"fleet_partner_schema_migrations", "fleet_partner_outbox", "fleet_partner_inbox",
		"fleet_partner_jobs", "fleet_partner_idempotency_keys", "fleet_partner_audit_records",
		"fleet_partner_file_references", "fleet_partner_organizations", "fleet_partner_branches",
		"fleet_partner_departments", "fleet_partner_organization_settings",
		"fleet_partner_organization_status_history",
	} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, table,
		).Scan(&exists); err != nil || !exists {
			t.Errorf("table %s missing (err=%v)", table, err)
		}
	}
	// Local foreign keys reference same-service tables only.
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT ccu.table_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.constraint_column_usage ccu ON ccu.constraint_name = tc.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_name LIKE 'fleet_partner_%'`)
	if err != nil {
		t.Fatalf("query fks: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var referenced string
		_ = rows.Scan(&referenced)
		if len(referenced) < len("fleet_partner_") || referenced[:len("fleet_partner_")] != "fleet_partner_" {
			t.Errorf("foreign key references non-service table %s", referenced)
		}
	}
}

func TestMigrationFailureRollsBack(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	dir := t.TempDir()
	good := "CREATE TABLE IF NOT EXISTS fp_mig_ok (id text PRIMARY KEY);"
	bad := "CREATE TABLE fp_mig_bad (id text PRIMARY KEY); INSERT INTO no_such_table VALUES (1);"
	if err := os.WriteFile(filepath.Join(dir, "000001_ok.up.sql"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "000002_bad.up.sql"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS fp_mig_ok")
		_, _ = pool.Exec(ctx, "DELETE FROM fleet_partner_schema_migrations WHERE version IN ('000001_ok','000002_bad')")
	})

	if err := database.ApplyMigrations(ctx, pool, dir); err == nil {
		t.Fatal("bad migration should fail the run")
	}
	// Good migration recorded, bad one fully rolled back.
	var okApplied, badApplied, badTable bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM fleet_partner_schema_migrations WHERE version = '000001_ok')`).Scan(&okApplied)
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM fleet_partner_schema_migrations WHERE version = '000002_bad')`).Scan(&badApplied)
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'fp_mig_bad')`).Scan(&badTable)
	if !okApplied || badApplied || badTable {
		t.Fatalf("failure rollback broken: ok=%v bad=%v badTable=%v", okApplied, badApplied, badTable)
	}
}

func TestOrganizationValidationAndArchive(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := repositories.NewOrganizationRepository(pool)

	// Timezone and currency validation.
	err := repo.Create(ctx, pool, &models.Organization{
		LegalName: "Bad TZ", RegistrationNumber: "RC-TZ-1",
		CountryCode: "NG", Timezone: "Not/AZone", DefaultCurrency: "NGN",
	})
	if err == nil {
		t.Fatal("invalid timezone must be rejected")
	}
	err = repo.Create(ctx, pool, &models.Organization{
		LegalName: "Bad Currency", RegistrationNumber: "RC-CUR-1",
		CountryCode: "NG", Timezone: "Africa/Lagos", DefaultCurrency: "naira",
	})
	if err == nil {
		t.Fatal("invalid currency must be rejected")
	}

	// Archive is a status transition, never a hard delete.
	org := createOrganization(t, pool, "Archive Org", "RC-ARC-1")
	err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.UpdateStatus(ctx, tx, org.ID, 1, models.StatusChange{
			PreviousStatus: models.OrganizationStatusPendingReview,
			NewStatus:      models.OrganizationStatusArchived,
			ActorType:      "platform_service", ActorID: "admin-backoffice-service",
			ReasonCode: "closed", RequestID: "req_1", CorrelationID: "cor_1",
		})
	})
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	loaded, err := repo.GetByID(ctx, org.ID)
	if err != nil {
		t.Fatalf("archived organization must remain readable: %v", err)
	}
	if loaded.Status != models.OrganizationStatusArchived || loaded.ArchivedAt == nil || loaded.Version != 2 {
		t.Fatalf("archived = %+v", loaded)
	}
}

func TestOrganizationSettings(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Settings Org", "RC-SET-1")
	settingsRepo := repositories.NewSettingsRepository(pool)

	// Timeout bounds enforced.
	err := settingsRepo.Create(ctx, pool, org.ID, &models.Settings{AssignmentTimeoutSeconds: 5})
	if err == nil {
		t.Fatal("out-of-bounds assignment timeout must be rejected")
	}
	// Un-namespaced feature flags rejected.
	err = settingsRepo.Create(ctx, pool, org.ID, &models.Settings{
		AssignmentTimeoutSeconds: 300,
		FeatureFlags:             map[string]bool{"randomFlag": true},
	})
	if err == nil {
		t.Fatal("un-namespaced feature flag must be rejected")
	}

	if err := settingsRepo.Create(ctx, pool, org.ID, &models.Settings{
		AssignmentTimeoutSeconds: 300,
		FeatureFlags:             map[string]bool{"fleet.dispatch_preview": true},
	}); err != nil {
		t.Fatalf("create settings: %v", err)
	}
	loaded, err := settingsRepo.Get(ctx, org.ID)
	if err != nil || loaded.Version != 1 || !loaded.FeatureFlags["fleet.dispatch_preview"] {
		t.Fatalf("get settings = %+v, %v", loaded, err)
	}

	// Compare-and-swap update.
	loaded.AssignmentTimeoutSeconds = 600
	if err := settingsRepo.Update(ctx, pool, org.ID, 99, loaded); !errors.Is(err, concurrency.ErrVersionMismatch) {
		t.Fatalf("stale settings version = %v", err)
	}
	if err := settingsRepo.Update(ctx, pool, org.ID, 1, loaded); err != nil {
		t.Fatalf("settings update: %v", err)
	}
	reloaded, _ := settingsRepo.Get(ctx, org.ID)
	if reloaded.Version != 2 || reloaded.AssignmentTimeoutSeconds != 600 {
		t.Fatalf("reloaded = %+v", reloaded)
	}
}

func TestBranchPermittedScopeEnforced(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Scope Org", "RC-SC-1")
	branches := repositories.NewBranchRepository(pool)

	first := &models.Branch{Code: "B-1", Name: "One", Type: "BRANCH", Timezone: "Africa/Lagos"}
	second := &models.Branch{Code: "B-2", Name: "Two", Type: "BRANCH", Timezone: "Africa/Lagos"}
	if err := branches.Create(ctx, pool, org.ID, first); err != nil {
		t.Fatal(err)
	}
	if err := branches.Create(ctx, pool, org.ID, second); err != nil {
		t.Fatal(err)
	}

	all, err := branches.List(ctx, tenant.Query{OrganizationID: org.ID})
	if err != nil || len(all) != 2 {
		t.Fatalf("unrestricted list = %d, %v", len(all), err)
	}
	scoped, err := branches.List(ctx, tenant.Query{OrganizationID: org.ID, PermittedBranchIDs: []string{first.ID}})
	if err != nil || len(scoped) != 1 || scoped[0].ID != first.ID {
		t.Fatalf("scoped list = %+v, %v", scoped, err)
	}
}

func TestConcurrentArchiveSingleWinner(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Race Org", "RC-RACE-1")
	branches := repositories.NewBranchRepository(pool)
	branch := &models.Branch{Code: "R-1", Name: "Race", Type: "BRANCH", Timezone: "Africa/Lagos"}
	if err := branches.Create(ctx, pool, org.ID, branch); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make([]error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			results[slot] = branches.Archive(ctx, org.ID, branch.ID, 1)
		}(i)
	}
	wg.Wait()

	winners, conflicts := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, concurrency.ErrVersionMismatch):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent archive result: %v", err)
		}
	}
	if winners != 1 || conflicts != 4 {
		t.Fatalf("winners=%d conflicts=%d", winners, conflicts)
	}
	// Version incremented exactly once; no mutation after mismatch.
	loaded, _ := branches.GetByID(ctx, org.ID, branch.ID)
	if loaded.Version != 2 || loaded.Status != models.BranchStatusArchived {
		t.Fatalf("post-race branch = %+v", loaded)
	}
}

func TestOutboxRetryDeadLetterAndHeaders(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Outbox Retry Org", "RC-OR-1")
	outbox := events.NewOutboxRepository(pool)

	envelope, _ := events.NewEnvelope("corporate.test.retry", 1, "organization", org.ID, 1, org.ID, "cor_retry", nil, false)
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return outbox.Append(ctx, tx, envelope)
	}); err != nil {
		t.Fatal(err)
	}

	// Event IDs are globally unique (primary key).
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return outbox.Append(ctx, tx, envelope)
	})
	if err == nil {
		t.Fatal("duplicate event id must be rejected")
	}

	// Organization and correlation IDs are stored.
	var storedOrg string
	var headers []byte
	if err := pool.QueryRow(ctx,
		`SELECT organization_id, headers FROM fleet_partner_outbox WHERE id = $1`, envelope.EventID,
	).Scan(&storedOrg, &headers); err != nil {
		t.Fatal(err)
	}
	var headerMap map[string]string
	_ = json.Unmarshal(headers, &headerMap)
	if storedOrg != org.ID || headerMap["correlationId"] != "cor_retry" {
		t.Fatalf("stored org=%s headers=%v", storedOrg, headerMap)
	}

	// Failure schedules a redacted retry; exhaustion dead-letters.
	claimed, err := outbox.ClaimBatch(ctx, "w1", time.Minute, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %d, %v", len(claimed), err)
	}
	if err := outbox.MarkFailed(ctx, "w1", envelope.EventID, errors.New("broker exploded with secret=abc"), 2); err != nil {
		t.Fatal(err)
	}
	var status, lastError string
	var attempts int
	_ = pool.QueryRow(ctx, `SELECT status, attempts, last_error_redacted FROM fleet_partner_outbox WHERE id = $1`, envelope.EventID).
		Scan(&status, &attempts, &lastError)
	if status != events.OutboxStatusPending || attempts != 1 || lastError == "" {
		t.Fatalf("after failure: status=%s attempts=%d", status, attempts)
	}

	// Make it due again and exhaust attempts.
	_, _ = pool.Exec(ctx, `UPDATE fleet_partner_outbox SET available_at = now(), lease_expires_at = NULL WHERE id = $1`, envelope.EventID)
	claimed, err = outbox.ClaimBatch(ctx, "w2", time.Minute, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("reclaim: %d, %v", len(claimed), err)
	}
	if err := outbox.MarkFailed(ctx, "w2", envelope.EventID, errors.New("boom"), 2); err != nil {
		t.Fatal(err)
	}
	_ = pool.QueryRow(ctx, `SELECT status FROM fleet_partner_outbox WHERE id = $1`, envelope.EventID).Scan(&status)
	if status != events.OutboxStatusDeadLettered {
		t.Fatalf("after exhaustion status = %s", status)
	}
}

func TestOutboxExpiredLeaseRecovery(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Outbox Lease Org", "RC-OL-1")
	outbox := events.NewOutboxRepository(pool)

	envelope, _ := events.NewEnvelope("corporate.test.lease", 1, "organization", org.ID, 1, org.ID, "cor_l", nil, false)
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return outbox.Append(ctx, tx, envelope)
	}); err != nil {
		t.Fatal(err)
	}

	// Crashed worker: lease expires immediately.
	claimed, err := outbox.ClaimBatch(ctx, "crashed", time.Millisecond, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	recovered, err := outbox.ClaimBatch(ctx, "recoverer", time.Minute, 10)
	if err != nil || len(recovered) != 1 {
		t.Fatalf("expired outbox lease not recovered: %d, %v", len(recovered), err)
	}
	if err := outbox.MarkPublished(ctx, "crashed", envelope.EventID); err == nil {
		t.Fatal("crashed worker must not complete a recovered lease")
	}
	if err := outbox.MarkPublished(ctx, "recoverer", envelope.EventID); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxWorkerDisabledPublisherKeepsEventsPending(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Disabled Pub Org", "RC-DP-1")
	outbox := events.NewOutboxRepository(pool)

	envelope, _ := events.NewEnvelope("corporate.test.disabled", 1, "organization", org.ID, 1, org.ID, "cor_d", nil, false)
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return outbox.Append(ctx, tx, envelope)
	}); err != nil {
		t.Fatal(err)
	}

	metrics := observability.NewMetrics()
	worker := &events.OutboxWorker{
		Repository:   outbox,
		Publisher:    events.DisabledPublisher{},
		Metrics:      metrics,
		Owner:        "disabled-worker",
		PollInterval: 20 * time.Millisecond,
		Lease:        100 * time.Millisecond,
	}
	runCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	worker.Run(runCtx)
	cancel()

	var status string
	var publishedAt *time.Time
	var attempts int
	if err := pool.QueryRow(ctx,
		`SELECT status, published_at, attempts FROM fleet_partner_outbox WHERE id = $1`, envelope.EventID,
	).Scan(&status, &publishedAt, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != events.OutboxStatusPending || publishedAt != nil || attempts != 0 {
		t.Fatalf("disabled publisher must keep event pending without attempts: status=%s attempts=%d", status, attempts)
	}
	if metrics.CounterValue("fleet_partner_outbox_publisher_unconfigured_total", nil) == 0 {
		t.Fatal("unconfigured publisher condition not observed in metrics")
	}
}

func TestIdempotencyCrossOrganizationIndependence(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	store := idempotency.NewStore(pool, time.Hour)
	endpoint := "POST /fleet-partner/v1/things"
	fingerprint := idempotency.Fingerprint("POST", endpoint, []byte(`{"a":1}`))

	// The same key in two organizations is independent.
	record, err := store.Begin(ctx, "org_one", endpoint, "shared-key", fingerprint)
	if err != nil || record != nil {
		t.Fatalf("org_one claim: %+v, %v", record, err)
	}
	record, err = store.Begin(ctx, "org_two", endpoint, "shared-key", fingerprint)
	if err != nil || record != nil {
		t.Fatalf("org_two claim must be independent: %+v, %v", record, err)
	}
}

func TestJobsCrossOrganizationDedupeIsolation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	orgA := createOrganization(t, pool, "Jobs Org A", "RC-JA-1")
	orgB := createOrganization(t, pool, "Jobs Org B", "RC-JB-1")

	idA, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: orgA.ID, JobType: "test.dedupe", DedupeKey: "same-key",
		Payload: map[string]string{"organizationId": orgA.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	idB, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: orgB.ID, JobType: "test.dedupe", DedupeKey: "same-key",
		Payload: map[string]string{"organizationId": orgB.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if idA == idB {
		t.Fatal("dedupe keys must be organization-scoped")
	}
	// Same org, same key still dedupes.
	idA2, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: orgA.ID, JobType: "test.dedupe", DedupeKey: "same-key",
		Payload: map[string]string{"organizationId": orgA.ID},
	})
	if err != nil || idA2 != idA {
		t.Fatalf("same-org dedupe broken: %s vs %s, %v", idA2, idA, err)
	}
}

func TestJobWorkerExecutesAndStopsClaimingOnShutdown(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Worker Org", "RC-W-1")

	executed := make(chan string, 1)
	worker := &jobs.Worker{
		Queue:             queue,
		Metrics:           observability.NewMetrics(),
		Owner:             "it-worker",
		PollInterval:      20 * time.Millisecond,
		LeaseDuration:     2 * time.Second,
		HeartbeatInterval: 500 * time.Millisecond,
	}
	worker.Register("test.execute", func(ctx context.Context, job jobs.Job) error {
		executed <- job.ID
		return nil
	})

	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.execute",
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { worker.Run(runCtx); close(done) }()

	select {
	case gotID := <-executed:
		if gotID != jobID {
			t.Fatalf("executed %s want %s", gotID, jobID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("job was not executed")
	}

	// Wait for completion to land, then shut down.
	deadline := time.Now().Add(2 * time.Second)
	for {
		var status string
		_ = pool.QueryRow(ctx, `SELECT status FROM fleet_partner_jobs WHERE id = $1`, jobID).Scan(&status)
		if status == jobs.StatusSucceeded {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job not completed, status=%s", status)
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	<-done
	if worker.Ready() {
		t.Fatal("worker must report not-ready after shutdown")
	}

	// No claims after shutdown: a new job stays PENDING.
	lateID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.execute",
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM fleet_partner_jobs WHERE id = $1`, lateID).Scan(&status)
	if status != jobs.StatusPending {
		t.Fatalf("job claimed after shutdown: %s", status)
	}
}

func TestJobTenantMismatchDeadLettersSafely(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Mismatch Org", "RC-M-1")

	// Payload claims a different organization than the durable row.
	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.mismatch",
		Payload: map[string]string{"organizationId": "org_attacker"},
	})
	if err != nil {
		t.Fatal(err)
	}

	worker := &jobs.Worker{
		Queue:             queue,
		Metrics:           observability.NewMetrics(),
		Owner:             "mismatch-worker",
		PollInterval:      20 * time.Millisecond,
		LeaseDuration:     time.Second,
		HeartbeatInterval: 500 * time.Millisecond,
	}
	worker.Register("test.mismatch", func(context.Context, jobs.Job) error {
		t.Error("handler must never run on tenant mismatch")
		return nil
	})
	runCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	worker.Run(runCtx)
	cancel()

	var status, lastError string
	_ = pool.QueryRow(ctx, `SELECT status, COALESCE(last_error_redacted,'') FROM fleet_partner_jobs WHERE id = $1`, jobID).Scan(&status, &lastError)
	if status != jobs.StatusDeadLettered {
		t.Fatalf("mismatched job must dead-letter, status=%s", status)
	}
	if lastError == "" {
		t.Fatal("security reason missing on dead-lettered job")
	}
}

func TestAuditRecorderTenantFilteringAndRedaction(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Audit Org A", "RC-AA-1")
	orgB := createOrganization(t, pool, "Audit Org B", "RC-AB-1")
	recorder := audit.NewRecorder()

	err := recorder.Record(ctx, pool, audit.Entry{
		OrganizationID: orgA.ID,
		ActorType:      "organization_user", ActorID: "usr_1",
		Action: "organization.update", ResourceType: "organization", ResourceID: orgA.ID,
		NewValues: map[string]any{"legal_name": "New Name", "bank_account_number": "0123456789", "api_token": "secret"},
		RequestID: "req_a", CorrelationID: "cor_a",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = recorder.Record(ctx, pool, audit.Entry{
		OrganizationID: orgB.ID,
		ActorType:      "organization_user", ActorID: "usr_2",
		Action: "organization.update", ResourceType: "organization", ResourceID: orgB.ID,
		RequestID: "req_b", CorrelationID: "cor_b",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Tenant filtering.
	var count int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id = $1`, orgA.ID).Scan(&count)
	if count != 1 {
		t.Fatalf("org A audit count = %d", count)
	}

	// Redaction and tracing IDs persisted.
	var newSummary []byte
	var requestID, correlationID string
	_ = pool.QueryRow(ctx, `
		SELECT new_summary, request_id, correlation_id FROM fleet_partner_audit_records
		WHERE organization_id = $1`, orgA.ID,
	).Scan(&newSummary, &requestID, &correlationID)
	var summary map[string]any
	_ = json.Unmarshal(newSummary, &summary)
	if summary["bank_account_number"] != "[REDACTED]" || summary["api_token"] != "[REDACTED]" {
		t.Fatalf("summary not redacted: %v", summary)
	}
	if summary["legal_name"] != "New Name" || requestID != "req_a" || correlationID != "cor_a" {
		t.Fatalf("summary/tracing wrong: %v %s %s", summary, requestID, correlationID)
	}
}

func TestDepartmentCrossTenantReadIsNotFound(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Dept Read A", "RC-DR-1")
	orgB := createOrganization(t, pool, "Dept Read B", "RC-DR-2")
	departments := repositories.NewDepartmentRepository(pool)

	department := &models.Department{Code: "OPS", Name: "Operations"}
	if err := departments.Create(ctx, pool, orgA.ID, department); err != nil {
		t.Fatal(err)
	}
	if _, err := departments.GetByID(ctx, orgB.ID, department.ID); !errors.Is(err, repositories.ErrNotFound) {
		t.Fatalf("cross-tenant department read must be ErrNotFound, got %v", err)
	}
	// Same department code allowed in a different organization.
	if err := departments.Create(ctx, pool, orgB.ID, &models.Department{Code: "OPS", Name: "Ops B"}); err != nil {
		t.Fatalf("same code in other organization must be allowed: %v", err)
	}
}
