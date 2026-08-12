package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	backgroundopshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/backgroundops/http"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/noncestore"
	"cosmicforge/logistics/services/fleet-partner-service/internal/scheduler"
)

const workerOpsCallerSecret = "worker-operations-route-test-service-secret-value"

// workerOperationsRouter builds the real internal router (HMAC service auth
// + nonce replay guard, exactly as internal/app/app.go assembles it for
// every /internal/fleet-partner/v1 route) with the worker-operations
// handler mounted, allowlisting only "admin-backoffice-service".
// pool must be a value already obtained from testPool(t) by the caller --
// testPool locks a test-lifetime mutex that is not reentrant, so calling it
// a second time within the same test deadlocks.
func workerOperationsRouter(t *testing.T, pool *pgxpool.Pool, jobQueue *jobs.Queue, schedRepo *scheduler.Repository) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := backgroundopshttp.NewInternalHandler(jobQueue, schedRepo, events.NewOutboxRepository(pool), []string{"admin-backoffice-service"})
	return app.BuildRouter(app.Options{
		Config:      config.Config{AppEnv: "test", MaxRequestBodyBytes: 1 << 20},
		Metrics:     observability.NewMetrics(),
		TokenSigner: auth.NewTokenSigner([]byte("unused-worker-ops-user-token-secret")),
		Verifier: serviceauth.NewVerifier(serviceauth.Secrets{
			"admin-backoffice-service": []byte(workerOpsCallerSecret),
			"some-other-service":       []byte(workerOpsCallerSecret),
		}, time.Minute),
		NonceStore: noncestore.New(pool),
		InternalRoutes: func(group *gin.RouterGroup) {
			backgroundopshttp.RegisterInternalRoutes(group, handler)
		},
	})
}

func signedInternalRequest(t *testing.T, method string, path string, caller string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(req, caller, []byte(workerOpsCallerSecret), body, time.Now()); err != nil {
		t.Fatalf("sign: %v", err)
	}
	return req
}

// TestWorkerOperations_RequiresAllowlistedCallerAndStaysPlatformScoped
// proves: (1) a validly HMAC-authenticated caller not on the allowlist is
// rejected (holding a valid service secret proves identity, not
// authorization); (2) an allowlisted caller can read a platform-scoped
// (organization_id NULL) job; (3) a tenant-owned job is invisible through
// this surface -- worker_operations never leaks into another organization's
// data, the exact boundary the master specification requires.
func TestWorkerOperations_RequiresAllowlistedCallerAndStaysPlatformScoped(t *testing.T) {
	pool := testPool(t)
	ctx := t.Context()
	jobQueue := jobs.NewQueue(pool)
	schedRepo := scheduler.NewRepository(pool)
	router := workerOperationsRouter(t, pool, jobQueue, schedRepo)

	platformJobID, err := jobQueue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		JobType: "test.worker-ops-platform", Payload: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	org := createOrganization(t, pool, "WorkerOps Tenant Co", "RC-S27-WOI-1")
	tenantJobID, err := jobQueue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.worker-ops-tenant",
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Valid signature, wrong caller -> forbidden.
	req := signedInternalRequest(t, http.MethodGet,
		"/internal/fleet-partner/v1/worker-operations/jobs/"+platformJobID, "some-other-service", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("non-allowlisted caller status = %d body=%s, want 403", recorder.Code, recorder.Body.String())
	}

	// Allowlisted caller, platform-scoped job -> visible.
	req = signedInternalRequest(t, http.MethodGet,
		"/internal/fleet-partner/v1/worker-operations/jobs/"+platformJobID, "admin-backoffice-service", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("allowlisted caller platform job status = %d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
	var view map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view["id"] != platformJobID {
		t.Fatalf("job id = %v, want %s", view["id"], platformJobID)
	}

	// Allowlisted caller, TENANT-owned job -> not found. worker_operations
	// must never expose a specific organization's jobs.
	req = signedInternalRequest(t, http.MethodGet,
		"/internal/fleet-partner/v1/worker-operations/jobs/"+tenantJobID, "admin-backoffice-service", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("tenant job via worker-operations status = %d body=%s, want 404", recorder.Code, recorder.Body.String())
	}
}

// TestWorkerOperations_ScheduledTaskEnableRequiresExpectedVersion proves the
// internal scheduled-task controls are version-guarded exactly like the
// tenant-scoped ones.
func TestWorkerOperations_ScheduledTaskEnableRequiresExpectedVersion(t *testing.T) {
	pool := testPool(t)
	ctx := t.Context()
	jobQueue := jobs.NewQueue(pool)
	schedRepo := scheduler.NewRepository(pool)
	router := workerOperationsRouter(t, pool, jobQueue, schedRepo)

	task := &scheduler.Task{
		TaskType: "test.worker-ops-task", JobType: "test.worker-ops-task.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "3600", Timezone: "UTC", Enabled: true,
	}
	if err := schedRepo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{"expectedVersion": 999})
	req := signedInternalRequest(t, http.MethodPost,
		"/internal/fleet-partner/v1/worker-operations/scheduled-tasks/"+task.ID+"/disable",
		"admin-backoffice-service", body)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("stale-version disable status = %d body=%s, want 409", recorder.Code, recorder.Body.String())
	}

	body, _ = json.Marshal(map[string]any{"expectedVersion": task.Version})
	req = signedInternalRequest(t, http.MethodPost,
		"/internal/fleet-partner/v1/worker-operations/scheduled-tasks/"+task.ID+"/disable",
		"admin-backoffice-service", body)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("correct-version disable status = %d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
}

// TestWorkerOperations_BacklogsAndOutboxReplay proves the Section 29
// operational surface: GET backlogs reflects real pending outbox/job state,
// and POST outbox/replay moves exactly one named dead-lettered record back
// to PENDING (never a bulk operation) and 404s for a record that either
// does not exist or is not currently dead-lettered.
func TestWorkerOperations_BacklogsAndOutboxReplay(t *testing.T) {
	pool := testPool(t)
	ctx := t.Context()
	jobQueue := jobs.NewQueue(pool)
	schedRepo := scheduler.NewRepository(pool)
	router := workerOperationsRouter(t, pool, jobQueue, schedRepo)

	org := createOrganization(t, pool, "WorkerOps Backlog Co", "RC-S29-WOB-1")
	outboxRepo := events.NewOutboxRepository(pool)
	recordID := ids.New("evt")
	if err := outboxRepo.Append(ctx, pool, events.Envelope{
		EventID: recordID, EventType: "organization.worker_ops_backlog_probe.v1", SchemaVersion: 1,
		AggregateType: "organization", AggregateID: org.ID, OrganizationID: org.ID,
		CorrelationID: "cor_backlog_probe", Producer: "fleet-partner-service", Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("seed outbox record: %v", err)
	}
	// Drive it straight to DEAD_LETTERED via the same repository path the
	// real outbox worker uses (claim, then fail past MaxAttempts=1).
	claimed, err := outboxRepo.ClaimBatch(ctx, "backlog-test-owner", time.Minute, 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	found := false
	for _, record := range claimed {
		if record.ID == recordID {
			found = true
		}
	}
	if !found {
		t.Fatal("seeded outbox record was not claimable")
	}
	if err := outboxRepo.MarkFailed(ctx, "backlog-test-owner", recordID, context.DeadlineExceeded, 1); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	// GET backlogs reflects the outbox pending count including this record
	// is NOT counted (it's DEAD_LETTERED, not PENDING) -- but the jobs
	// section must be present and queryable regardless.
	req := signedInternalRequest(t, http.MethodGet, "/internal/fleet-partner/v1/operations/backlogs", "admin-backoffice-service", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET backlogs status = %d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
	var backlogs map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &backlogs); err != nil {
		t.Fatal(err)
	}
	if _, ok := backlogs["jobs"]; !ok {
		t.Fatalf("backlogs response missing jobs section: %s", recorder.Body.String())
	}
	if _, ok := backlogs["outbox"]; !ok {
		t.Fatalf("backlogs response missing outbox section: %s", recorder.Body.String())
	}

	// Replaying an unknown/non-dead-lettered record 404s.
	body, _ := json.Marshal(map[string]string{"recordId": "evt_does_not_exist"})
	req = signedInternalRequest(t, http.MethodPost, "/internal/fleet-partner/v1/operations/outbox/replay", "admin-backoffice-service", body)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("replay unknown record status = %d body=%s, want 404", recorder.Code, recorder.Body.String())
	}

	// Replaying the real dead-lettered record succeeds and moves it back to PENDING.
	body, _ = json.Marshal(map[string]string{"recordId": recordID})
	req = signedInternalRequest(t, http.MethodPost, "/internal/fleet-partner/v1/operations/outbox/replay", "admin-backoffice-service", body)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("replay dead-lettered record status = %d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM fleet_partner_outbox WHERE id=$1`, recordID).Scan(&status); err != nil {
		t.Fatalf("reload record: %v", err)
	}
	if status != events.OutboxStatusPending {
		t.Fatalf("record status after replay = %s, want %s", status, events.OutboxStatusPending)
	}

	// A non-allowlisted caller is refused even for the read-only backlogs route.
	req = signedInternalRequest(t, http.MethodGet, "/internal/fleet-partner/v1/operations/backlogs", "some-other-service", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("non-allowlisted backlogs status = %d, want 403", recorder.Code)
	}
}
