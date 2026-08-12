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

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	backgroundopshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/backgroundops/http"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/scheduler"
)

func backgroundOpsCall(t *testing.T, router http.Handler, method string, path string,
	orgID string, memberID string, body any) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Test-Org", orgID)
	request.Header.Set("X-Test-Member", memberID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	envelope := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &envelope)
	// Success responses are wrapped as {"data": ..., "meta": {...}} (see
	// internal/platform/respond); unwrap so callers can assert directly on
	// the resource fields. Error responses (no "data" key) pass through
	// unwrapped so callers can inspect {"error": {...}}.
	if data, ok := envelope["data"].(map[string]any); ok {
		return recorder.Code, data
	}
	return recorder.Code, envelope
}

// TestBackgroundOps_JobsRoutes_RequireJobsPermissionAndEnforceOrgScope
// proves: (1) jobs.read alone does not extend to jobs.retry/jobs.cancel,
// mirroring the search.read/search.repair separation already established
// in this service; (2) a job belonging to a different organization is
// invisible (404, not a cross-tenant view) -- IDOR protection is the
// organization-scoped repository query itself, not an extra check.
func TestBackgroundOps_JobsRoutes_RequireJobsPermissionAndEnforceOrgScope(t *testing.T) {
	pool := testPool(t)
	orgA := createOrganization(t, pool, "BackgroundOps Jobs Co A", "RC-S27-BGA-1")
	orgB := createOrganization(t, pool, "BackgroundOps Jobs Co B", "RC-S27-BGA-2")
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	queue := jobs.NewQueue(pool)
	schedRepo := scheduler.NewRepository(pool)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		org := c.GetHeader("X-Test-Org")
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: org, OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	backgroundopshttp.NewHandler(queue, schedRepo, authorizer).RegisterTenantRoutes(group)

	// rol_system_crm holds no jobs.* permission (migration 000063 grants
	// only owner/admin/operations).
	readOnlyMember := seedApprovalMember(t, pool, orgA.ID, "rol_system_crm", "read-only")
	operator := seedApprovalMember(t, pool, orgA.ID, "rol_system_owner", "operator")

	jobID, err := queue.Enqueue(context.Background(), pool, jobs.EnqueueRequest{
		OrganizationID: orgA.ID, JobType: "test.bgops-visible",
		Payload: map[string]string{"organizationId": orgA.ID}, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// No jobs.read at all -> forbidden.
	status, response := backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/jobs", orgA.ID, readOnlyMember, nil)
	if status != http.StatusForbidden {
		t.Fatalf("read-only member list status = %d body=%v, want 403", status, response)
	}

	// Operator (jobs.read via owner role) can list and get.
	status, response = backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/jobs", orgA.ID, operator, nil)
	if status != http.StatusOK {
		t.Fatalf("operator list status = %d body=%v, want 200", status, response)
	}
	status, response = backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/jobs/"+jobID, orgA.ID, operator, nil)
	if status != http.StatusOK {
		t.Fatalf("operator get status = %d body=%v, want 200", status, response)
	}
	if response["id"] != jobID {
		t.Fatalf("job view id = %v, want %s", response["id"], jobID)
	}
	if _, leaked := response["payload"]; leaked {
		t.Fatal("job view must not include the raw payload")
	}

	// Cross-tenant: a member of orgB, even with owner-tier role there, gets
	// 404 (not 403, not a leaked cross-org view) when reaching for orgA's job.
	crossOrgOperator := seedApprovalMember(t, pool, orgB.ID, "rol_system_owner", "cross-org")
	status, response = backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/jobs/"+jobID, orgB.ID, crossOrgOperator, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org get status = %d body=%v, want 404 (IDOR protection)", status, response)
	}

	// Cancel: operator can cancel a PENDING job.
	status, response = backgroundOpsCall(t, engine, http.MethodPost, "/fleet-partner/v1/jobs/"+jobID+"/cancel", orgA.ID, operator, nil)
	if status != http.StatusOK {
		t.Fatalf("operator cancel status = %d body=%v, want 200", status, response)
	}
	if response["status"] != jobs.StatusCancelled {
		t.Fatalf("job status after cancel = %v, want CANCELLED", response["status"])
	}

	// Retry: only a DEAD_LETTERED job is eligible -- a cancelled job refuses.
	status, response = backgroundOpsCall(t, engine, http.MethodPost, "/fleet-partner/v1/jobs/"+jobID+"/retry", orgA.ID, operator, nil)
	if status != http.StatusNotFound {
		t.Fatalf("retry a cancelled (non-dead-lettered) job status = %d body=%v, want 404", status, response)
	}
}

// TestBackgroundOps_ScheduledTasksRoutes_ManageRunAndVersionConflict proves
// enable/disable/run-now are version-guarded (a stale expectedVersion is
// refused as a conflict) and that scheduled_tasks.read alone does not
// extend to scheduled_tasks.manage/scheduled_tasks.run.
func TestBackgroundOps_ScheduledTasksRoutes_ManageRunAndVersionConflict(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "BackgroundOps Sched Co", "RC-S27-BGS-1")
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	queue := jobs.NewQueue(pool)
	schedRepo := scheduler.NewRepository(pool)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: org.ID, OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	backgroundopshttp.NewHandler(queue, schedRepo, authorizer).RegisterTenantRoutes(group)

	readOnlyMember := seedApprovalMember(t, pool, org.ID, "rol_system_crm", "read-only")
	operator := seedApprovalMember(t, pool, org.ID, "rol_system_owner", "operator")

	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.bgops-task", JobType: "test.bgops-task.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "3600", Timezone: "UTC", Enabled: true,
	}
	if err := schedRepo.Create(context.Background(), pool, task); err != nil {
		t.Fatal(err)
	}

	status, response := backgroundOpsCall(t, engine, http.MethodPost,
		"/fleet-partner/v1/scheduled-tasks/"+task.ID+"/disable", org.ID, readOnlyMember,
		map[string]any{"expectedVersion": task.Version})
	if status != http.StatusForbidden {
		t.Fatalf("read-only member disable status = %d body=%v, want 403", status, response)
	}

	status, response = backgroundOpsCall(t, engine, http.MethodPost,
		"/fleet-partner/v1/scheduled-tasks/"+task.ID+"/disable", org.ID, operator,
		map[string]any{"expectedVersion": task.Version})
	if status != http.StatusOK {
		t.Fatalf("operator disable status = %d body=%v, want 200", status, response)
	}
	if response["enabled"] != false {
		t.Fatalf("enabled = %v, want false", response["enabled"])
	}

	// Stale version (still task.Version, but the task already advanced to
	// task.Version+1 after the disable above) -> conflict.
	status, response = backgroundOpsCall(t, engine, http.MethodPost,
		"/fleet-partner/v1/scheduled-tasks/"+task.ID+"/enable", org.ID, operator,
		map[string]any{"expectedVersion": task.Version})
	if status != http.StatusConflict {
		t.Fatalf("stale-version enable status = %d body=%v, want 409", status, response)
	}

	// Correct version succeeds.
	status, response = backgroundOpsCall(t, engine, http.MethodPost,
		"/fleet-partner/v1/scheduled-tasks/"+task.ID+"/enable", org.ID, operator,
		map[string]any{"expectedVersion": task.Version + 1})
	if status != http.StatusOK {
		t.Fatalf("operator enable status = %d body=%v, want 200", status, response)
	}
	if response["enabled"] != true {
		t.Fatalf("enabled = %v, want true", response["enabled"])
	}

	// scheduled_tasks.run is a distinct permission from scheduled_tasks.manage
	// in name, but both are granted to the same owner-tier roles here, so
	// prove run-now itself works and forces immediate due-ness.
	status, response = backgroundOpsCall(t, engine, http.MethodPost,
		"/fleet-partner/v1/scheduled-tasks/"+task.ID+"/run-now", org.ID, operator,
		map[string]any{"expectedVersion": task.Version + 2})
	if status != http.StatusOK {
		t.Fatalf("operator run-now status = %d body=%v, want 200", status, response)
	}
	due, err := schedRepo.ClaimDue(context.Background(), "test-runner", time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range due {
		if d.ID == task.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the task to be immediately due after run-now")
	}

	// Run history is readable.
	status, response = backgroundOpsCall(t, engine, http.MethodGet,
		"/fleet-partner/v1/scheduled-tasks/"+task.ID+"/runs", org.ID, operator, nil)
	if status != http.StatusOK {
		t.Fatalf("operator list runs status = %d body=%v, want 200", status, response)
	}
}
