package integration_test

import (
	"context"
	"net/http"
	"testing"

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

// Section 28 gap-1 IDOR domain 3/5: jobs and scheduled tasks (tenant
// surface, /fleet-partner/v1/jobs and /fleet-partner/v1/scheduled-tasks --
// NOT the /internal/worker-operations HMAC surface, already covered
// elsewhere).
//
// TestBackgroundOps_JobsRoutes_RequireJobsPermissionAndEnforceOrgScope in
// backgroundops_http_section27_test.go already proves cross-org denial for
// getJob and missing-permission for listJobs; this suite fills the gaps
// found while building this registry: listJobAttempts, retryJob and
// cancelJob had no cross-org case, and NONE of getScheduledTask,
// listScheduledTaskRuns, enableScheduledTask, disableScheduledTask or
// runScheduledTaskNow had one either.
//
// Both jobs.Queue and scheduler.Repository filter every read/write by
// organization_id (Job/Task rows use a NULLable column; a real
// organization id can only ever match a row carrying that same id, per
// COALESCE(organization_id,'')=$N), so an org mismatch always resolves to
// either an explicit safe 404 (getJob, retryJob, getScheduledTask) or a
// safe 409 conflict indistinguishable from "wrong version"
// (cancelJob, enableScheduledTask, disableScheduledTask,
// runScheduledTaskNow -- see schedulerConflict/apperrors.Conflict in
// internal/features/backgroundops/http/handlers.go) or, for the two
// history listings (listJobAttempts, listScheduledTaskRuns), a safe empty
// result rather than an error at all. All three are safe: none of them
// discloses another organization's data or existence.

// TestJobs_ObjectAuthorization_AttemptsRetryCancel fills the cross-org gap
// for listJobAttempts, retryJob and cancelJob left by the existing Section
// 27 suite (which only covers listJobs and getJob).
func TestJobs_ObjectAuthorization_AttemptsRetryCancel(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Jobs IDOR Co A", "RC-IDOR-JOB-1")
	orgB := createOrganization(t, pool, "Jobs IDOR Co B", "RC-IDOR-JOB-2")
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	queue := jobs.NewQueue(pool)
	schedRepo := scheduler.NewRepository(pool)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: c.GetHeader("X-Test-Member"),
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: c.GetHeader("X-Test-Member"),
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	backgroundopshttp.NewHandler(queue, schedRepo, authorizer).RegisterTenantRoutes(group)

	owner := seedApprovalMember(t, pool, orgA.ID, "rol_system_owner", "owner")
	crossOrgOwner := seedApprovalMember(t, pool, orgB.ID, "rol_system_owner", "cross-org")
	noPermission := seedApprovalMember(t, pool, orgA.ID, "rol_system_crm", "no-perm")

	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: orgA.ID, JobType: "test.idor-attempts",
		Payload: map[string]string{"organizationId": orgA.ID}, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO fleet_partner_job_attempts(id, job_id, organization_id, attempt_number, worker_owner, status, started_at)
		 VALUES('att_idor_test_1',$1,$2,1,'test-worker','FAILED',now())`, jobID, orgA.ID); err != nil {
		t.Fatalf("seed job attempt: %v", err)
	}

	// Missing permission -> 403.
	status, body := backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/jobs/"+jobID+"/attempts", orgA.ID, noPermission, nil)
	if status != http.StatusForbidden {
		t.Fatalf("list attempts without jobs.read status = %d body=%v, want 403", status, body)
	}

	// Same-org: the seeded attempt is visible.
	status, body = backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/jobs/"+jobID+"/attempts", orgA.ID, owner, nil)
	if status != http.StatusOK {
		t.Fatalf("same-org list attempts status = %d body=%v, want 200", status, body)
	}
	attempts, _ := body["data"].([]any)
	if len(attempts) != 1 {
		t.Fatalf("same-org attempts = %v, want exactly the 1 seeded attempt", body["data"])
	}

	// Cross-org: orgB reaching for orgA's job's attempts gets an EMPTY list
	// (the join filters on organization_id, so no row can ever match), not
	// orgA's attempt data -- safe, even though the status code is 200
	// rather than 404 for this particular read.
	status, body = backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/jobs/"+jobID+"/attempts", orgB.ID, crossOrgOwner, nil)
	if status != http.StatusOK {
		t.Fatalf("cross-org list attempts status = %d body=%v, want 200 (empty, not an error)", status, body)
	}
	if crossAttempts, _ := body["data"].([]any); len(crossAttempts) != 0 {
		t.Fatalf("cross-org attempts leaked orgA data: %v", body["data"])
	}

	// Force the job to DEAD_LETTERED directly (bypassing the real
	// retry-exhaustion loop, which is not what this suite is testing) so
	// retryJob has something eligible to act on within the right org.
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_jobs SET status='DEAD_LETTERED' WHERE id=$1`, jobID); err != nil {
		t.Fatalf("force dead-letter: %v", err)
	}

	// Cross-org retry: orgB can never retry orgA's job, dead-lettered or
	// not -- Replay's own WHERE clause requires organization_id to match,
	// so this is a safe, explicit 404, not a leak or a real retry.
	status, body = backgroundOpsCall(t, engine, http.MethodPost, "/fleet-partner/v1/jobs/"+jobID+"/retry", orgB.ID, crossOrgOwner, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org retry status = %d body=%v, want 404", status, body)
	}

	// Same-org retry succeeds now that the job is dead-lettered.
	status, body = backgroundOpsCall(t, engine, http.MethodPost, "/fleet-partner/v1/jobs/"+jobID+"/retry", orgA.ID, owner, nil)
	if status != http.StatusOK {
		t.Fatalf("same-org retry status = %d body=%v, want 200", status, body)
	}
	if body["status"] != jobs.StatusPending {
		t.Fatalf("job status after retry = %v, want PENDING", body["status"])
	}

	// Cross-org cancel: orgB cannot cancel orgA's (now PENDING) job --
	// Cancel's WHERE clause also requires organization_id to match, so 0
	// rows are affected and the handler answers a safe 409 (the same
	// response a genuine "not cancellable" state would produce), never a
	// leak of orgA's job.
	status, body = backgroundOpsCall(t, engine, http.MethodPost, "/fleet-partner/v1/jobs/"+jobID+"/cancel", orgB.ID, crossOrgOwner, nil)
	if status != http.StatusConflict {
		t.Fatalf("cross-org cancel status = %d body=%v, want 409 (safe, not a leak)", status, body)
	}

	// Same-org cancel succeeds.
	status, body = backgroundOpsCall(t, engine, http.MethodPost, "/fleet-partner/v1/jobs/"+jobID+"/cancel", orgA.ID, owner, nil)
	if status != http.StatusOK {
		t.Fatalf("same-org cancel status = %d body=%v, want 200", status, body)
	}
	if body["status"] != jobs.StatusCancelled {
		t.Fatalf("job status after cancel = %v, want CANCELLED", body["status"])
	}
}

// TestScheduledTasks_ObjectAuthorization proves the object-authorization
// matrix for the entire /scheduled-tasks family, none of which had any
// cross-org case before this suite.
func TestScheduledTasks_ObjectAuthorization(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Sched IDOR Co A", "RC-IDOR-SCH-1")
	orgB := createOrganization(t, pool, "Sched IDOR Co B", "RC-IDOR-SCH-2")
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	queue := jobs.NewQueue(pool)
	schedRepo := scheduler.NewRepository(pool)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: c.GetHeader("X-Test-Org"), OrganizationUserID: c.GetHeader("X-Test-Member"),
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: c.GetHeader("X-Test-Member"),
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	backgroundopshttp.NewHandler(queue, schedRepo, authorizer).RegisterTenantRoutes(group)

	owner := seedApprovalMember(t, pool, orgA.ID, "rol_system_owner", "owner")
	crossOrgOwner := seedApprovalMember(t, pool, orgB.ID, "rol_system_owner", "cross-org")
	noPermission := seedApprovalMember(t, pool, orgA.ID, "rol_system_crm", "no-perm")

	task := &scheduler.Task{
		OrganizationID: orgA.ID, TaskType: "test.idor-sched-task", JobType: "test.idor-sched-task.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "3600", Timezone: "UTC", Enabled: true,
	}
	if err := schedRepo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO fleet_partner_scheduled_task_runs(id, scheduled_task_id, organization_id, scheduled_for, outcome, occurred_at)
		 VALUES('run_idor_test_1',$1,$2,now(),'ENQUEUED',now())`, task.ID, orgA.ID); err != nil {
		t.Fatalf("seed scheduled task run: %v", err)
	}

	// Missing permission -> 403.
	status, body := backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/scheduled-tasks/"+task.ID, orgA.ID, noPermission, nil)
	if status != http.StatusForbidden {
		t.Fatalf("get without scheduled_tasks.read status = %d body=%v, want 403", status, body)
	}

	// Same-org get.
	status, body = backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/scheduled-tasks/"+task.ID, orgA.ID, owner, nil)
	if status != http.StatusOK {
		t.Fatalf("same-org get status = %d body=%v, want 200", status, body)
	}

	// Cross-org get -> explicit safe 404.
	status, body = backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/scheduled-tasks/"+task.ID, orgB.ID, crossOrgOwner, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-org get status = %d body=%v, want 404", status, body)
	}

	// Same-org run history is visible.
	status, body = backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/scheduled-tasks/"+task.ID+"/runs", orgA.ID, owner, nil)
	if status != http.StatusOK {
		t.Fatalf("same-org list runs status = %d body=%v, want 200", status, body)
	}
	if runs, _ := body["data"].([]any); len(runs) != 1 {
		t.Fatalf("same-org runs = %v, want exactly the 1 seeded run", body["data"])
	}

	// Cross-org run history -> safe empty list, never orgA's run data.
	status, body = backgroundOpsCall(t, engine, http.MethodGet, "/fleet-partner/v1/scheduled-tasks/"+task.ID+"/runs", orgB.ID, crossOrgOwner, nil)
	if status != http.StatusOK {
		t.Fatalf("cross-org list runs status = %d body=%v, want 200 (empty, not an error)", status, body)
	}
	if crossRuns, _ := body["data"].([]any); len(crossRuns) != 0 {
		t.Fatalf("cross-org runs leaked orgA data: %v", body["data"])
	}

	// Cross-org disable/enable/run-now: the underlying UPDATE's WHERE
	// clause requires organization_id to match, so 0 rows are affected and
	// SetEnabled/RunNow map that to ErrVersionConflict -> a safe 409,
	// exactly the same response a genuine stale-version request gets, so a
	// caller cannot distinguish "wrong org" from "wrong version".
	status, body = backgroundOpsCall(t, engine, http.MethodPost, "/fleet-partner/v1/scheduled-tasks/"+task.ID+"/disable",
		orgB.ID, crossOrgOwner, map[string]any{"expectedVersion": task.Version})
	if status != http.StatusConflict {
		t.Fatalf("cross-org disable status = %d body=%v, want 409 (safe, not a leak)", status, body)
	}
	status, body = backgroundOpsCall(t, engine, http.MethodPost, "/fleet-partner/v1/scheduled-tasks/"+task.ID+"/enable",
		orgB.ID, crossOrgOwner, map[string]any{"expectedVersion": task.Version})
	if status != http.StatusConflict {
		t.Fatalf("cross-org enable status = %d body=%v, want 409 (safe, not a leak)", status, body)
	}
	status, body = backgroundOpsCall(t, engine, http.MethodPost, "/fleet-partner/v1/scheduled-tasks/"+task.ID+"/run-now",
		orgB.ID, crossOrgOwner, map[string]any{"expectedVersion": task.Version})
	if status != http.StatusConflict {
		t.Fatalf("cross-org run-now status = %d body=%v, want 409 (safe, not a leak)", status, body)
	}

	// The task genuinely never moved: same-org disable with the ORIGINAL
	// version still succeeds after all three cross-org attempts above.
	status, body = backgroundOpsCall(t, engine, http.MethodPost, "/fleet-partner/v1/scheduled-tasks/"+task.ID+"/disable",
		orgA.ID, owner, map[string]any{"expectedVersion": task.Version})
	if status != http.StatusOK {
		t.Fatalf("same-org disable after cross-org attempts status = %d body=%v, want 200 (cross-org attempts must not have mutated the task)", status, body)
	}
}
