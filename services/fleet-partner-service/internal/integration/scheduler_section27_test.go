package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/scheduler"
)

// TestScheduler_CronComputesNextRunOnCreate proves a CRON task gets an
// initial next_run_at computed from its own timezone at creation time.
func TestScheduler_CronComputesNextRunOnCreate(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	org := createOrganization(t, pool, "Scheduler Cron Co", "RC-SCH-1")

	task := &scheduler.Task{
		OrganizationID:     org.ID,
		TaskType:           "test.cron",
		JobType:            "test.cron.job",
		ScheduleType:       scheduler.ScheduleCron,
		ScheduleExpression: "0 9 * * *",
		Timezone:           "UTC",
		Enabled:            true,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatalf("create: %v", err)
	}
	if task.ID == "" {
		t.Fatal("expected an assigned ID")
	}
	if task.NextRunAt == nil {
		t.Fatal("expected next_run_at to be computed on create")
	}
	if task.Version != 1 {
		t.Fatalf("version = %d, want 1", task.Version)
	}
}

// TestScheduler_IntervalAndOnceScheduleTypes proves INTERVAL and ONCE
// schedules compute correctly and that a ONCE task's occurrence, once
// passed, leaves next_run_at nil (no further occurrences).
func TestScheduler_IntervalAndOnceScheduleTypes(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	org := createOrganization(t, pool, "Scheduler Interval Co", "RC-SCH-2")

	interval := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.interval", JobType: "test.interval.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "3600", Timezone: "UTC", Enabled: true,
	}
	if err := repo.Create(ctx, pool, interval); err != nil {
		t.Fatalf("create interval: %v", err)
	}
	if interval.NextRunAt == nil {
		t.Fatal("expected next_run_at for INTERVAL task")
	}

	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	once := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.once", JobType: "test.once.job",
		ScheduleType: scheduler.ScheduleOnce, ScheduleExpression: past, Timezone: "UTC", Enabled: true,
	}
	if err := repo.Create(ctx, pool, once); err != nil {
		t.Fatalf("create once: %v", err)
	}
	if once.NextRunAt != nil {
		t.Fatalf("expected nil next_run_at for a ONCE task whose instant already passed, got %v", once.NextRunAt)
	}
}

// TestScheduler_InvalidTimezoneRejected proves task creation validates the
// IANA timezone rather than silently storing an unusable value.
func TestScheduler_InvalidTimezoneRejected(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	org := createOrganization(t, pool, "Scheduler TZ Co", "RC-SCH-3")

	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.badtz", JobType: "test.badtz.job",
		ScheduleType: scheduler.ScheduleCron, ScheduleExpression: "* * * * *", Timezone: "Not/AZone",
	}
	if err := repo.Create(ctx, pool, task); err == nil {
		t.Fatal("expected an error for an invalid timezone")
	}
}

// TestScheduler_ClaimDueIsSkipLockedAcrossConcurrentSchedulers proves two
// concurrent scheduler instances claiming the same due task never both
// succeed -- FOR UPDATE SKIP LOCKED gives it to exactly one.
func TestScheduler_ClaimDueIsSkipLockedAcrossConcurrentSchedulers(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	org := createOrganization(t, pool, "Scheduler Concurrent Co", "RC-SCH-4")

	task := dueTaskNow(t, ctx, repo, pool, org.ID, "test.concurrent-claim")

	type claimResult struct {
		tasks []scheduler.Task
		err   error
	}
	results := make(chan claimResult, 2)
	start := make(chan struct{})
	claim := func(owner string) {
		<-start
		tasks, err := repo.ClaimDue(ctx, owner, time.Minute, 10)
		results <- claimResult{tasks: tasks, err: err}
	}
	go claim("scheduler-a")
	go claim("scheduler-b")
	close(start)

	var totalClaimed int
	for i := 0; i < 2; i++ {
		res := <-results
		if res.err != nil {
			t.Fatalf("claim: %v", res.err)
		}
		for _, claimed := range res.tasks {
			if claimed.ID == task.ID {
				totalClaimed++
			}
		}
	}
	if totalClaimed != 1 {
		t.Fatalf("task claimed %d times across concurrent schedulers, want exactly 1", totalClaimed)
	}
}

// TestScheduler_EnableDisableVersionGuarded proves SetEnabled is optimistic-
// concurrency guarded: a stale expected version is refused.
func TestScheduler_EnableDisableVersionGuarded(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	org := createOrganization(t, pool, "Scheduler Enable Co", "RC-SCH-5")

	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.enable", JobType: "test.enable.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "60", Timezone: "UTC", Enabled: true,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.SetEnabled(ctx, org.ID, task.ID, false, task.Version, "tester")
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if updated.Enabled {
		t.Fatal("expected task disabled")
	}
	if updated.Version != task.Version+1 {
		t.Fatalf("version = %d, want %d", updated.Version, task.Version+1)
	}
	if _, err := repo.SetEnabled(ctx, org.ID, task.ID, true, task.Version, "tester"); err != scheduler.ErrVersionConflict {
		t.Fatalf("expected ErrVersionConflict on stale version, got %v", err)
	}
}

// TestScheduler_RunNowForcesImmediateDue proves RunNow makes a task claimable
// on the very next Tick regardless of its ordinary schedule.
func TestScheduler_RunNowForcesImmediateDue(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	org := createOrganization(t, pool, "Scheduler RunNow Co", "RC-SCH-6")

	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.runnow", JobType: "test.runnow.job",
		ScheduleType: scheduler.ScheduleOnce, ScheduleExpression: future, Timezone: "UTC", Enabled: true,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}
	due, err := repo.ClaimDue(ctx, "w1", time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range due {
		if d.ID == task.ID {
			t.Fatal("a future ONCE task must not be due yet")
		}
	}
	if _, err := repo.RunNow(ctx, org.ID, task.ID, task.Version, "operator"); err != nil {
		t.Fatalf("run now: %v", err)
	}
	due, err = repo.ClaimDue(ctx, "w1", time.Minute, 10)
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
		t.Fatal("expected task to be immediately due after RunNow")
	}
}

// dueTaskNow creates an enabled INTERVAL task whose next_run_at is already
// in the past (due immediately), for tests that need a claimable task
// without waiting on real time.
func dueTaskNow(t *testing.T, ctx context.Context, repo *scheduler.Repository, pool *pgxpool.Pool, orgID string, taskType string) scheduler.Task {
	t.Helper()
	task := &scheduler.Task{
		OrganizationID: orgID, TaskType: taskType, JobType: taskType + ".job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "3600", Timezone: "UTC", Enabled: true,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatalf("create due task: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_scheduled_tasks SET next_run_at = now() - interval '5 minutes' WHERE id = $1`,
		task.ID); err != nil {
		t.Fatalf("backdate next_run_at: %v", err)
	}
	backdated, err := repo.Get(ctx, orgID, task.ID)
	if err != nil {
		t.Fatalf("reload backdated task: %v", err)
	}
	return backdated
}

func newRunner(pool *pgxpool.Pool, repo *scheduler.Repository, queue *jobs.Queue, owner string) *scheduler.Runner {
	return &scheduler.Runner{Repo: repo, Jobs: queue, DB: pool, Owner: owner}
}

// TestScheduler_RunnerConcurrencyAllow_AlwaysEnqueuesEachOccurrence proves
// ALLOW never blocks a new occurrence on a still-in-flight prior one.
func TestScheduler_RunnerConcurrencyAllow_AlwaysEnqueuesEachOccurrence(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Scheduler Allow Co", "RC-SCH-7")

	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.allow", JobType: "test.allow.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "3600", Timezone: "UTC", Enabled: true,
		ConcurrencyPolicy: scheduler.ConcurrencyAllow, MaxAttempts: 5,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}
	backdate(t, ctx, pool, task.ID)

	runner := newRunner(pool, repo, queue, "runner-allow")
	result, err := runner.Tick(ctx)
	if err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if result.Enqueued != 1 {
		t.Fatalf("tick 1 enqueued = %d, want 1: %+v", result.Enqueued, result)
	}

	// The just-enqueued job is still PENDING (never claimed) -- ALLOW must
	// still enqueue a second occurrence once the task is due again.
	backdate(t, ctx, pool, task.ID)
	result, err = runner.Tick(ctx)
	if err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if result.Enqueued != 1 {
		t.Fatalf("tick 2 enqueued = %d, want 1 under ALLOW despite an in-flight prior job: %+v", result.Enqueued, result)
	}

	jobsList, err := queue.List(ctx, org.ID, jobs.ListFilter{JobType: "test.allow.job"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobsList) != 2 {
		t.Fatalf("expected 2 distinct jobs enqueued under ALLOW, got %d", len(jobsList))
	}
}

// TestScheduler_RunnerConcurrencyForbid_SkipsWhilePriorInFlight proves
// FORBID skips a new occurrence while an earlier one has not reached a
// terminal state, and records SKIPPED_FORBID in the run history.
func TestScheduler_RunnerConcurrencyForbid_SkipsWhilePriorInFlight(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Scheduler Forbid Co", "RC-SCH-8")

	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.forbid", JobType: "test.forbid.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "3600", Timezone: "UTC", Enabled: true,
		ConcurrencyPolicy: scheduler.ConcurrencyForbid, MaxAttempts: 5,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}
	backdate(t, ctx, pool, task.ID)

	runner := newRunner(pool, repo, queue, "runner-forbid")
	result, err := runner.Tick(ctx)
	if err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if result.Enqueued != 1 {
		t.Fatalf("tick 1 enqueued = %d, want 1", result.Enqueued)
	}

	backdate(t, ctx, pool, task.ID)
	result, err = runner.Tick(ctx)
	if err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if result.SkippedForbid != 1 {
		t.Fatalf("tick 2 skipped-forbid = %d, want 1 while the prior job is still PENDING: %+v", result.SkippedForbid, result)
	}

	jobsList, err := queue.List(ctx, org.ID, jobs.ListFilter{JobType: "test.forbid.job"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobsList) != 1 {
		t.Fatalf("expected exactly 1 job under FORBID with a prior job in flight, got %d", len(jobsList))
	}

	runs, err := repo.ListRuns(ctx, org.ID, task.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	foundSkip := false
	for _, run := range runs {
		if run.Outcome == scheduler.OutcomeSkippedForbid {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Fatalf("expected a SKIPPED_FORBID run record, got %+v", runs)
	}
}

// TestScheduler_RunnerConcurrencyReplace_CancelsPendingAndEnqueuesNew proves
// REPLACE cancels a not-yet-started prior occurrence and enqueues the new
// one in its place.
func TestScheduler_RunnerConcurrencyReplace_CancelsPendingAndEnqueuesNew(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Scheduler Replace Co", "RC-SCH-9")

	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.replace", JobType: "test.replace.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "3600", Timezone: "UTC", Enabled: true,
		ConcurrencyPolicy: scheduler.ConcurrencyReplace, MaxAttempts: 5,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}
	backdate(t, ctx, pool, task.ID)

	runner := newRunner(pool, repo, queue, "runner-replace")
	if _, err := runner.Tick(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	firstBatch, err := queue.List(ctx, org.ID, jobs.ListFilter{JobType: "test.replace.job"})
	if err != nil || len(firstBatch) != 1 {
		t.Fatalf("expected 1 job after tick 1: %+v %v", firstBatch, err)
	}
	firstJobID := firstBatch[0].ID

	backdate(t, ctx, pool, task.ID)
	result, err := runner.Tick(ctx)
	if err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if result.Enqueued != 1 {
		t.Fatalf("tick 2 enqueued = %d, want 1 (the replacement)", result.Enqueued)
	}

	firstJob, err := queue.Get(ctx, org.ID, firstJobID)
	if err != nil {
		t.Fatal(err)
	}
	if firstJob.Status != jobs.StatusCancelled {
		t.Fatalf("first job status = %s, want CANCELLED (superseded by REPLACE)", firstJob.Status)
	}

	allJobs, err := queue.List(ctx, org.ID, jobs.ListFilter{JobType: "test.replace.job"})
	if err != nil {
		t.Fatal(err)
	}
	if len(allJobs) != 2 {
		t.Fatalf("expected 2 total job rows (cancelled original + replacement), got %d", len(allJobs))
	}
}

// TestScheduler_RunnerMisfireSkip_RecordsAndAdvancesPastMissedOccurrence
// proves SKIP (the default misfire policy) records the missed occurrence
// and advances next_run_at without ever enqueueing a job for it.
func TestScheduler_RunnerMisfireSkip_RecordsAndAdvancesPastMissedOccurrence(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Scheduler MisfireSkip Co", "RC-SCH-10")

	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.misfire-skip", JobType: "test.misfire-skip.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "60", Timezone: "UTC", Enabled: true,
		MisfirePolicy: scheduler.MisfireSkip, MaxAttempts: 5,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}
	// Missed by well over the misfire grace window.
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_scheduled_tasks SET next_run_at = now() - interval '2 hours' WHERE id = $1`,
		task.ID); err != nil {
		t.Fatal(err)
	}

	runner := newRunner(pool, repo, queue, "runner-misfire-skip")
	result, err := runner.Tick(ctx)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if result.SkippedMisfire != 1 {
		t.Fatalf("skipped-misfire = %d, want 1: %+v", result.SkippedMisfire, result)
	}
	jobsList, err := queue.List(ctx, org.ID, jobs.ListFilter{JobType: "test.misfire-skip.job"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobsList) != 0 {
		t.Fatalf("expected no job enqueued for a skipped misfire, got %d", len(jobsList))
	}
	updated, err := repo.Get(ctx, org.ID, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.NextRunAt == nil || !updated.NextRunAt.After(time.Now().Add(-time.Minute)) {
		t.Fatalf("expected next_run_at advanced close to now, got %v", updated.NextRunAt)
	}
}

// TestScheduler_RunnerMisfireRunOnce_EnqueuesExactlyOneOccurrence proves
// RUN_ONCE enqueues a single job for a missed window no matter how many
// occurrences were actually missed.
func TestScheduler_RunnerMisfireRunOnce_EnqueuesExactlyOneOccurrence(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Scheduler MisfireRunOnce Co", "RC-SCH-11")

	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.misfire-run-once", JobType: "test.misfire-run-once.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "60", Timezone: "UTC", Enabled: true,
		MisfirePolicy: scheduler.MisfireRunOnce, MaxAttempts: 5,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}
	// 10 missed 60s occurrences behind.
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_scheduled_tasks SET next_run_at = now() - interval '10 minutes' WHERE id = $1`,
		task.ID); err != nil {
		t.Fatal(err)
	}

	runner := newRunner(pool, repo, queue, "runner-misfire-run-once")
	result, err := runner.Tick(ctx)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if result.Enqueued != 1 {
		t.Fatalf("enqueued = %d, want exactly 1 under RUN_ONCE regardless of how many occurrences were missed", result.Enqueued)
	}
	jobsList, err := queue.List(ctx, org.ID, jobs.ListFilter{JobType: "test.misfire-run-once.job"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobsList) != 1 {
		t.Fatalf("expected exactly 1 job row, got %d", len(jobsList))
	}
}

// TestScheduler_RunnerMisfireCatchUpLimited_BoundedByMaxCatchUp proves
// CATCH_UP_LIMITED enqueues missed occurrences up to max_catch_up and no
// further, even when more were actually missed.
func TestScheduler_RunnerMisfireCatchUpLimited_BoundedByMaxCatchUp(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Scheduler MisfireCatchUp Co", "RC-SCH-12")

	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.misfire-catchup", JobType: "test.misfire-catchup.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "60", Timezone: "UTC", Enabled: true,
		MisfirePolicy: scheduler.MisfireCatchUpLimited, MaxCatchUp: 3, MaxAttempts: 5,
		// ALLOW so successive catch-up occurrences are not blocked by
		// concurrency policy against each other (they are enqueued in the
		// same tick, all PENDING).
		ConcurrencyPolicy: scheduler.ConcurrencyAllow,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}
	// 10 missed 60s occurrences behind -- more than max_catch_up=3.
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_scheduled_tasks SET next_run_at = now() - interval '10 minutes' WHERE id = $1`,
		task.ID); err != nil {
		t.Fatal(err)
	}

	runner := newRunner(pool, repo, queue, "runner-misfire-catchup")
	result, err := runner.Tick(ctx)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if result.Enqueued != 1 {
		// Tick counts the tick's outcome once per task, not per occurrence --
		// verify occurrence count via the jobs list instead.
		t.Logf("tick-level enqueued = %d (informational)", result.Enqueued)
	}
	jobsList, err := queue.List(ctx, org.ID, jobs.ListFilter{JobType: "test.misfire-catchup.job"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobsList) != 3 {
		t.Fatalf("expected exactly max_catch_up=3 job occurrences, got %d", len(jobsList))
	}
}

// TestScheduler_RunnerEnqueueFailure_RecordsAndAdvancesWithoutStuckLease
// proves an enqueue failure (empty job_type) is recorded as ENQUEUE_FAILED,
// increments failure_count, and still advances/releases the task rather
// than leaving it stuck holding a scheduler lease.
func TestScheduler_RunnerEnqueueFailure_RecordsAndAdvancesWithoutStuckLease(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Scheduler EnqueueFail Co", "RC-SCH-13")

	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.enqueue-fail", JobType: "",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "3600", Timezone: "UTC", Enabled: true,
		MaxAttempts: 5,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}
	backdate(t, ctx, pool, task.ID)

	runner := newRunner(pool, repo, queue, "runner-enqueue-fail")
	result, err := runner.Tick(ctx)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("failed = %d, want 1: %+v", result.Failed, result)
	}
	updated, err := repo.Get(ctx, org.ID, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.FailureCount < 1 {
		t.Fatalf("failure_count = %d, want at least 1", updated.FailureCount)
	}
	if updated.LeaseOwner != "" {
		t.Fatal("task must not be left holding a scheduler lease after an enqueue failure")
	}
	runs, err := repo.ListRuns(ctx, org.ID, task.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, run := range runs {
		if run.Outcome == scheduler.OutcomeEnqueueFailed {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an ENQUEUE_FAILED run record, got %+v", runs)
	}
}

// TestScheduler_LeaseRecoveryAfterCrash proves an abandoned scheduler lease
// (the claiming instance crashed before Advance) is reclaimed by another
// instance once the lease expires, rather than stranding the task forever.
func TestScheduler_LeaseRecoveryAfterCrash(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	org := createOrganization(t, pool, "Scheduler LeaseRecover Co", "RC-SCH-14")

	task := dueTaskNow(t, ctx, repo, pool, org.ID, "test.lease-recover")

	// Claim with a short lease and never advance -- simulates a crash
	// between claim and processing.
	claimed, err := repo.ClaimDue(ctx, "crashed-scheduler", 300*time.Millisecond, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range claimed {
		if c.ID == task.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected task claimed by the first (crashing) scheduler")
	}

	// Immediately re-claiming must not succeed (lease still held).
	reclaimed, err := repo.ClaimDue(ctx, "healthy-scheduler", time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reclaimed {
		if r.ID == task.ID {
			t.Fatal("task must not be claimable while another scheduler's lease is still valid")
		}
	}

	time.Sleep(500 * time.Millisecond) // let the 300ms lease expire
	recovered, err := repo.ClaimDue(ctx, "healthy-scheduler", time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, r := range recovered {
		if r.ID == task.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("expected the task to be reclaimed by another scheduler after the crashed lease expired")
	}
}

// TestScheduler_ScheduleHistoryIsPreservedAcrossOccurrences proves run
// history accumulates across multiple ticks rather than being overwritten.
func TestScheduler_ScheduleHistoryIsPreservedAcrossOccurrences(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := scheduler.NewRepository(pool)
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Scheduler History Co", "RC-SCH-15")

	task := &scheduler.Task{
		OrganizationID: org.ID, TaskType: "test.history", JobType: "test.history.job",
		ScheduleType: scheduler.ScheduleInterval, ScheduleExpression: "3600", Timezone: "UTC", Enabled: true,
		ConcurrencyPolicy: scheduler.ConcurrencyAllow, MaxAttempts: 5,
	}
	if err := repo.Create(ctx, pool, task); err != nil {
		t.Fatal(err)
	}
	runner := newRunner(pool, repo, queue, "runner-history")
	for i := 0; i < 3; i++ {
		backdate(t, ctx, pool, task.ID)
		if _, err := runner.Tick(ctx); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
	}
	runs, err := repo.ListRuns(ctx, org.ID, task.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 accumulated run records, got %d", len(runs))
	}
}

func backdate(t *testing.T, ctx context.Context, pool *pgxpool.Pool, taskID string) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_scheduled_tasks SET next_run_at = now() - interval '1 second', lease_owner = NULL, lease_expires_at = NULL WHERE id = $1`,
		taskID); err != nil {
		t.Fatalf("backdate: %v", err)
	}
}
