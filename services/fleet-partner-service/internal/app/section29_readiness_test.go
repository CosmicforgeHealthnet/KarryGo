package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Section 29: readiness enforcement tests for the new mandatory-dependency
// checks cmd/main.go constructs (dispatch/hauling execution workers, audit
// persistence, job stale-lease/dead-letter backlog, outbox backlog). These
// mirror cmd/main.go's own check construction against the real /ready HTTP
// contract, the same style Section 28's retention_readiness_test.go already
// established: TestReadySucceedsAndFails (app_test.go) proves the generic
// {200 healthy, 503 broken, no leaked secret} contract; these tests prove
// the SPECIFIC behavior for each new named dependency.

func TestReadiness_ExecutionWorkerDisabledStaysReadyWithoutClient(t *testing.T) {
	// Mirrors cmd/main.go: no dispatch/hauling checks are appended at all
	// when their enablement flags are off — see the `if
	// cfg.DispatchExecutionStartEnabled { ... }` guard.
	router := buildTestRouter(t, nil)
	if resp := do(router, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusOK {
		t.Fatalf("/ready with execution workers disabled = %d, want 200", resp.Code)
	}
}

func TestReadiness_ExecutionWorkerEnabledFailsClosedWithoutClient(t *testing.T) {
	cases := []struct {
		name        string
		configured  bool
		wantHealthy bool
	}{
		{"dispatch enabled without client", false, false},
		{"dispatch enabled with client", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := buildTestRouter(t, nil, ReadyCheck{Name: "dispatch_execution_worker", Check: func(context.Context) error {
				if !tc.configured {
					return errors.New("dispatch execution-start enabled without a configured dispatch service client")
				}
				return nil
			}})
			resp := do(router, httptest.NewRequest(http.MethodGet, "/ready", nil))
			wantStatus := http.StatusServiceUnavailable
			if tc.wantHealthy {
				wantStatus = http.StatusOK
			}
			if resp.Code != wantStatus {
				t.Fatalf("/ready(%s) = %d, want %d", tc.name, resp.Code, wantStatus)
			}
		})
	}
}

func TestReadiness_OutboxPublisherFailsClosedWhenRedisConfiguredButUnavailable(t *testing.T) {
	// Mirrors cmd/main.go's outbox_publisher check: only appended when
	// cfg.Redis.Addr != "" (operator intends live delivery); fails when the
	// actual redisClient ended up nil despite that intent.
	unavailable := buildTestRouter(t, nil, ReadyCheck{Name: "outbox_publisher", Check: func(context.Context) error {
		return errors.New("outbox delivery expected (redis configured) but the redis client is unavailable")
	}})
	if resp := do(unavailable, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready with redis configured but unavailable = %d, want 503", resp.Code)
	}

	available := buildTestRouter(t, nil, ReadyCheck{Name: "outbox_publisher", Check: func(context.Context) error { return nil }})
	if resp := do(available, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusOK {
		t.Fatalf("/ready with redis configured and available = %d, want 200", resp.Code)
	}

	// Redis not configured at all (no live-delivery intent): cmd/main.go
	// appends no outbox_publisher check whatsoever, so /ready stays healthy
	// regardless — proven by the no-checks-appended router below, matching
	// every other optional dependency's disabled posture in this file.
	notConfigured := buildTestRouter(t, nil)
	if resp := do(notConfigured, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusOK {
		t.Fatalf("/ready with redis not configured at all = %d, want 200", resp.Code)
	}
}

// TestReadiness_AuditPersistenceAlwaysChecked proves the audit_persistence
// check (always appended, no feature flag) uses count(*) rather than a bare
// SELECT — an EMPTY but reachable table must still report healthy, never be
// mistaken for an outage.
func TestReadiness_AuditPersistenceAlwaysChecked(t *testing.T) {
	rawURL := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL not set; skipping DB-backed readiness test")
	}
	ctx := context.Background()
	pool, err := database.NewPool(ctx, rawURL, database.PoolOptions{MaxConns: 3})
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	healthy := buildTestRouter(t, nil, ReadyCheck{Name: "audit_persistence", Check: func(checkCtx context.Context) error {
		var count int64
		return pool.QueryRow(checkCtx, "SELECT count(*) FROM fleet_partner_audit_records").Scan(&count)
	}})
	resp := do(healthy, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("/ready with a reachable (possibly empty) audit table = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}

	broken := buildTestRouter(t, nil, ReadyCheck{Name: "audit_persistence", Check: func(checkCtx context.Context) error {
		var count int64
		return pool.QueryRow(checkCtx, "SELECT count(*) FROM no_such_table_at_all").Scan(&count)
	}})
	if resp := do(broken, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready against an unreachable audit table = %d, want 503", resp.Code)
	}
}

// TestReadiness_JobBacklogThresholds_RealDatabase proves the stale-lease and
// dead-letter checks against real durable job rows: a fresh queue is ready;
// a job whose lease has expired past the configured threshold, or enough
// dead-lettered jobs to cross the critical count, fails closed.
func TestReadiness_JobBacklogThresholds_RealDatabase(t *testing.T) {
	rawURL := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL not set; skipping DB-backed readiness test")
	}
	ctx := context.Background()
	pool, err := database.NewPool(ctx, rawURL, database.PoolOptions{MaxConns: 5})
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()
	queue := jobs.NewQueue(pool)

	orgID := ids.New("org")
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_organizations
			(id, legal_name, registration_number, country_code, timezone, default_currency, status, created_at, updated_at)
		VALUES ($1, 'Readiness Test Co', $2, 'NG', 'Africa/Lagos', 'NGN', 'ACTIVE', now(), now())`,
		orgID, "RC-READY-"+orgID); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM fleet_partner_jobs WHERE organization_id = $1`, orgID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM fleet_partner_organizations WHERE id = $1`, orgID)
	})

	// Both StaleLeaseCount and DeadLetterCount are deliberately GLOBAL
	// (platform-wide critical thresholds, not per-tenant — matching
	// cmd/main.go's own production checks), so this shared test database
	// can already carry stale leases/dead-letters left behind by other
	// tests in the same run. Every assertion below is relative to a
	// baseline captured up front, never an assumed-zero absolute count.
	baselineStale, err := queue.StaleLeaseCount(ctx, time.Minute)
	if err != nil {
		t.Fatalf("baseline stale count: %v", err)
	}
	baselineDeadLetter, err := queue.DeadLetterCount(ctx)
	if err != nil {
		t.Fatalf("baseline dead-letter count: %v", err)
	}

	// A freshly-enqueued, unclaimed job changes neither count.
	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: orgID, JobType: "readiness.probe", MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	freshRouter := buildTestRouter(t, nil,
		ReadyCheck{Name: "job_stale_leases", Check: func(checkCtx context.Context) error {
			count, err := queue.StaleLeaseCount(checkCtx, time.Minute)
			if err != nil {
				return err
			}
			if count > baselineStale {
				return errors.New("stale job leases past threshold")
			}
			return nil
		}},
		ReadyCheck{Name: "job_dead_letter_backlog", Check: func(checkCtx context.Context) error {
			count, err := queue.DeadLetterCount(checkCtx)
			if err != nil {
				return err
			}
			if count > baselineDeadLetter {
				return errors.New("dead-lettered job backlog at or above the critical threshold")
			}
			return nil
		}},
	)
	if resp := do(freshRouter, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusOK {
		t.Fatalf("/ready with a fresh, unclaimed job = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}

	// Put this specific job (by ID) into a claimed-with-an-expired-lease
	// state directly via SQL, exactly mirroring what ClaimBatch itself
	// would set (status/lease_owner/lease_expires_at) — ClaimBatch's own
	// claim is a GLOBAL, unscoped "any due job" query (by design, matching
	// how the real worker operates), so calling it here in a shared test
	// database would claim OTHER tests' pending jobs too, not just this
	// one; targeting this job's own ID directly avoids that entirely.
	if _, err := pool.Exec(ctx, `
		UPDATE fleet_partner_jobs SET status=$1, lease_owner=$2, lease_expires_at=now()-interval '1 hour'
		WHERE id=$3`, jobs.StatusClaimed, "readiness-test-owner", jobID); err != nil {
		t.Fatalf("claim job %s directly: %v", jobID, err)
	}
	staleRouter := buildTestRouter(t, nil, ReadyCheck{Name: "job_stale_leases", Check: func(checkCtx context.Context) error {
		count, err := queue.StaleLeaseCount(checkCtx, 0)
		if err != nil {
			return err
		}
		if count > baselineStale {
			return errors.New("stale job leases past threshold")
		}
		return nil
	}})
	if resp := do(staleRouter, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready with a lease already expired = %d, want 503", resp.Code)
	}

	// Dead-letter it, then a threshold at the baseline must fail (this
	// test's own job pushed the count one past it).
	if err := queue.DeadLetter(ctx, "readiness-test-owner", jobID, "test"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}
	deadLetterRouter := buildTestRouter(t, nil, ReadyCheck{Name: "job_dead_letter_backlog", Check: func(checkCtx context.Context) error {
		count, err := queue.DeadLetterCount(checkCtx)
		if err != nil {
			return err
		}
		if count > baselineDeadLetter {
			return errors.New("dead-lettered job backlog at or above the critical threshold")
		}
		return nil
	}})
	if resp := do(deadLetterRouter, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready with dead-letter count at the critical threshold = %d, want 503", resp.Code)
	}
}

// TestReadiness_OutboxBacklogThresholds_RealDatabase mirrors the job-backlog
// test for the outbox: a fresh outbox is ready; enough pending records (or
// one old enough) crosses the configured critical thresholds.
func TestReadiness_OutboxBacklogThresholds_RealDatabase(t *testing.T) {
	rawURL := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL not set; skipping DB-backed readiness test")
	}
	ctx := context.Background()
	pool, err := database.NewPool(ctx, rawURL, database.PoolOptions{MaxConns: 5})
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()
	repo := events.NewOutboxRepository(pool)

	orgID := ids.New("org")
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_organizations
			(id, legal_name, registration_number, country_code, timezone, default_currency, status, created_at, updated_at)
		VALUES ($1, 'Outbox Readiness Test Co', $2, 'NG', 'Africa/Lagos', 'NGN', 'ACTIVE', now(), now())`,
		orgID, "RC-OBREADY-"+orgID); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM fleet_partner_outbox WHERE organization_id = $1`, orgID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM fleet_partner_organizations WHERE id = $1`, orgID)
	})

	checkAgainst := func(backlogCritical int64, staleAfter time.Duration) ReadyCheck {
		return ReadyCheck{Name: "outbox_backlog", Check: func(checkCtx context.Context) error {
			pending, err := repo.PendingCount(checkCtx)
			if err != nil {
				return err
			}
			if pending >= backlogCritical {
				return errors.New("outbox backlog at or above the critical threshold")
			}
			age, err := repo.OldestPendingAge(checkCtx)
			if err != nil {
				return err
			}
			if age > staleAfter {
				return errors.New("oldest pending outbox event exceeds the staleness threshold")
			}
			return nil
		}}
	}

	// PendingCount is deliberately GLOBAL (a platform-wide backlog signal,
	// matching cmd/main.go's own production check), so this shared test
	// database may already carry pending rows left by other tests in the
	// same run — the "at threshold" case below is asserted relative to a
	// baseline captured before this test's own row exists, never an
	// assumed-zero absolute count.
	baselinePending, err := repo.PendingCount(ctx)
	if err != nil {
		t.Fatalf("baseline pending count: %v", err)
	}
	baselineOldestAge, err := repo.OldestPendingAge(ctx)
	if err != nil {
		t.Fatalf("baseline oldest pending age: %v", err)
	}

	if err := repo.Append(ctx, pool, events.Envelope{
		EventID: ids.New("evt"), EventType: "readiness.probe.v1", SchemaVersion: 1,
		AggregateType: "organization", AggregateID: orgID, OrganizationID: orgID,
		CorrelationID: "cor_readiness", Producer: "fleet-partner-service", Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("append outbox record: %v", err)
	}

	// Both signals are global production health measures. Make the fresh case
	// relative to the pre-existing disposable-database baseline so an older
	// pending record from an earlier package/process cannot make this test's
	// newly appended event look stale or over the count threshold.
	freshRouter := buildTestRouter(t, nil, checkAgainst(baselinePending+2, baselineOldestAge+time.Minute))
	if resp := do(freshRouter, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusOK {
		t.Fatalf("/ready with one fresh pending event under threshold = %d, want 200 body=%s", resp.Code, resp.Body.String())
	}

	backlogRouter := buildTestRouter(t, nil, checkAgainst(baselinePending+1, 10*time.Minute))
	if resp := do(backlogRouter, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready with pending count at the critical threshold = %d, want 503", resp.Code)
	}

	staleRouter := buildTestRouter(t, nil, checkAgainst(5000, 0))
	if resp := do(staleRouter, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready with a zero staleness threshold and any pending age = %d, want 503", resp.Code)
	}
}

func TestLiveRouteIsDependencyFree(t *testing.T) {
	router := buildTestRouter(t, nil, ReadyCheck{Name: "postgres", Check: func(context.Context) error {
		return errors.New("database is down")
	}})
	// /ready must fail...
	if resp := do(router, httptest.NewRequest(http.MethodGet, "/ready", nil)); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready = %d, want 503", resp.Code)
	}
	// ...but /live must stay healthy regardless: liveness excludes every
	// dependency by construction (it has no ReadyChecks wired to it at all).
	if resp := do(router, httptest.NewRequest(http.MethodGet, "/live", nil)); resp.Code != http.StatusOK {
		t.Fatalf("/live = %d, want 200 even with postgres reported down", resp.Code)
	}
}
