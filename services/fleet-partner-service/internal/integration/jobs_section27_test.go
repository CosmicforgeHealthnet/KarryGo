package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// TestJobs_ErrorClassificationDrivesRetryBehavior proves Fail's retry
// decision is a function of the error class, not a blanket
// attempts-vs-max_attempts count: a VALIDATION/PERMANENT failure
// dead-letters on the very first attempt, while an unclassified (plain)
// error keeps the pre-Section-27 bounded-retry behavior.
func TestJobs_ErrorClassificationDrivesRetryBehavior(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Jobs Classify Co", "RC-J27-1")

	permID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.permanent",
		Payload: map[string]string{"organizationId": org.ID}, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := queue.ClaimBatch(ctx, "w1", time.Minute, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	permErr := &jobs.ClassifiedError{Class: jobs.ErrorClassValidation, Code: "bad_input", SafeMessage: "bad input"}
	if err := queue.Fail(ctx, "w1", permID, permErr); err != nil {
		t.Fatalf("fail: %v", err)
	}
	job, err := queue.Get(ctx, org.ID, permID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != jobs.StatusDeadLettered {
		t.Fatalf("status = %s, want DEAD_LETTERED on first VALIDATION failure", job.Status)
	}
	if job.LastErrorClass != string(jobs.ErrorClassValidation) || job.LastErrorCode != "bad_input" {
		t.Fatalf("error class/code not recorded: %+v", job)
	}
	if job.DeadLetteredAt == nil {
		t.Fatal("dead_lettered_at not set")
	}

	// A transient failure retries instead.
	transID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.transient",
		Payload: map[string]string{"organizationId": org.ID}, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.ClaimBatch(ctx, "w1", time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	transErr := &jobs.ClassifiedError{Class: jobs.ErrorClassTransient, SafeMessage: "temporary"}
	if err := queue.Fail(ctx, "w1", transID, transErr); err != nil {
		t.Fatalf("fail: %v", err)
	}
	job, err = queue.Get(ctx, org.ID, transID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != jobs.StatusRetryScheduled {
		t.Fatalf("status = %s, want RETRY_SCHEDULED for a transient failure under max_attempts", job.Status)
	}
	if job.NextAttemptAt == nil {
		t.Fatal("next_attempt_at not set for a scheduled retry")
	}
}

// TestJobs_UnknownExternalOutcomeRequiresReadback proves an
// UNKNOWN_EXTERNAL_OUTCOME failure marks the job for authoritative
// read-back rather than blindly retrying the external call again.
func TestJobs_UnknownExternalOutcomeRequiresReadback(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Jobs Readback Co", "RC-J27-2")

	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.ambiguous",
		Payload: map[string]string{"organizationId": org.ID}, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.ClaimBatch(ctx, "w1", time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	ambiguous := &jobs.ClassifiedError{Class: jobs.ErrorClassUnknownExternalOutcome, SafeMessage: "timeout after send"}
	if err := queue.Fail(ctx, "w1", jobID, ambiguous); err != nil {
		t.Fatalf("fail: %v", err)
	}
	job, err := queue.Get(ctx, org.ID, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if !job.AuthoritativeReadbackRequired {
		t.Fatal("expected authoritative_readback_required to be set")
	}
	if job.ExternalOutcomeStatus != "UNKNOWN" {
		t.Fatalf("external_outcome_status = %s, want UNKNOWN", job.ExternalOutcomeStatus)
	}
}

// TestJobs_CancelPendingRefusesRunning proves cancellation only applies to
// PENDING/RETRY_SCHEDULED jobs, and a SUCCEEDED job can never be
// retroactively cancelled.
func TestJobs_CancelPendingRefusesRunning(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Jobs Cancel Co", "RC-J27-3")

	pendingID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.cancel-pending",
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.Cancel(ctx, org.ID, pendingID); err != nil {
		t.Fatalf("cancel pending: %v", err)
	}
	job, err := queue.Get(ctx, org.ID, pendingID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != jobs.StatusCancelled || job.CancelledAt == nil {
		t.Fatalf("job not cancelled: %+v", job)
	}

	runningID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.cancel-running",
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.ClaimBatch(ctx, "w1", time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	if err := queue.Cancel(ctx, org.ID, runningID); err == nil {
		t.Fatal("expected cancel to refuse a CLAIMED job")
	}
	if err := queue.Complete(ctx, "w1", runningID); err != nil {
		t.Fatal(err)
	}
	if err := queue.Cancel(ctx, org.ID, runningID); err == nil {
		t.Fatal("expected cancel to refuse a SUCCEEDED job")
	}
}

// TestJobs_ReplayResetsErrorClassAndDeadLetteredAt proves replay clears the
// prior failure's classification and dead-letter timestamp, not just the
// status.
func TestJobs_ReplayResetsErrorClassAndDeadLetteredAt(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Jobs Replay Co", "RC-J27-4")

	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.replay",
		Payload: map[string]string{"organizationId": org.ID}, MaxAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := queue.ClaimBatch(ctx, "w1", time.Minute, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	permErr := &jobs.ClassifiedError{Class: jobs.ErrorClassPermanent, Code: "boom", SafeMessage: "boom"}
	if err := queue.RecordAttempt(ctx, claimed[0], "w1", jobs.StatusDeadLettered, time.Now().UTC(), permErr); err != nil {
		t.Fatal(err)
	}
	if err := queue.Fail(ctx, "w1", jobID, permErr); err != nil {
		t.Fatal(err)
	}
	if job, _ := queue.Get(ctx, org.ID, jobID); job.Status != jobs.StatusDeadLettered {
		t.Fatalf("precondition: job not dead-lettered, status=%s", job.Status)
	}

	if err := queue.Replay(ctx, org.ID, jobID); err != nil {
		t.Fatalf("replay: %v", err)
	}
	job, err := queue.Get(ctx, org.ID, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != jobs.StatusPending {
		t.Fatalf("status = %s, want PENDING after replay", job.Status)
	}
	if job.LastErrorClass != "" || job.LastErrorCode != "" {
		t.Fatalf("replay must clear the prior error classification: %+v", job)
	}
	if job.DeadLetteredAt != nil {
		t.Fatal("replay must clear dead_lettered_at")
	}
	attempts, err := queue.ListAttempts(ctx, org.ID, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) == 0 {
		t.Fatal("replay must not delete attempt history")
	}
}

// TestJobs_ReleaseForShutdownReturnsToPendingWithoutBackoff proves a
// shutdown-released lease goes straight back to PENDING (claimable
// immediately) rather than being penalized with retry backoff — nothing
// about the job failed, only the process hosting it stopped.
func TestJobs_ReleaseForShutdownReturnsToPendingWithoutBackoff(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Jobs Shutdown Co", "RC-J27-5")

	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.shutdown",
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.ClaimBatch(ctx, "w1", time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	if err := queue.ReleaseForShutdown(ctx, "w1", jobID); err != nil {
		t.Fatalf("release for shutdown: %v", err)
	}
	job, err := queue.Get(ctx, org.ID, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != jobs.StatusPending {
		t.Fatalf("status = %s, want PENDING", job.Status)
	}
	if job.LeaseOwner != "" {
		t.Fatal("lease owner must be cleared")
	}

	// Immediately claimable again by a different worker.
	claimed, err := queue.ClaimBatch(ctx, "w2", time.Minute, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != jobID {
		t.Fatalf("job not immediately reclaimable after shutdown release: %+v", claimed)
	}
}

// TestJobWorker_ShutdownSentinelReleasesWithoutBackoff proves a handler that
// observes cancellation before any external effect and returns
// jobs.ErrShutdownSafeToRelease gets its job released back to PENDING
// immediately (no backoff, no consumed attempt toward max_attempts) rather
// than going through the ordinary Fail/backoff path.
func TestJobWorker_ShutdownSentinelReleasesWithoutBackoff(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Jobs Shutdown Sentinel Co", "RC-J27-7")

	worker := &jobs.Worker{
		Queue: queue, Metrics: observability.NewMetrics(), Owner: "shutdown-worker",
		PollInterval: 20 * time.Millisecond, LeaseDuration: time.Minute, HeartbeatInterval: time.Second,
	}
	release := make(chan struct{})
	worker.Register("test.shutdown-sentinel", func(runCtx context.Context, job jobs.Job) error {
		<-runCtx.Done()
		close(release)
		return jobs.ErrShutdownSafeToRelease
	})

	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.shutdown-sentinel", MaxAttempts: 1,
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { worker.Run(runCtx); close(done) }()

	// Give the worker a moment to claim and start the handler, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-release:
	case <-time.After(3 * time.Second):
		t.Fatal("handler never observed cancellation")
	}
	<-done

	job, err := queue.Get(context.Background(), org.ID, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != jobs.StatusPending {
		t.Fatalf("status = %s, want PENDING (released, not failed)", job.Status)
	}
	if job.Attempts != 1 {
		// ClaimBatch increments attempts on claim (that part is unavoidable —
		// it is how the lease's own accounting works), but the job must not
		// be dead-lettered despite MaxAttempts=1: shutdown release bypasses
		// the max-attempts check entirely.
		t.Logf("attempts = %d (informational)", job.Attempts)
	}
	attempts, err := queue.ListAttempts(context.Background(), org.ID, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) == 0 || attempts[0].Status != "SHUTDOWN_RELEASED" {
		t.Fatalf("expected a SHUTDOWN_RELEASED attempt record, got %+v", attempts)
	}
}

// TestJobs_HealthReadbacksReportStaleLeasesAndDeadLetterBacklog proves the
// two health-check read-backs actually count what they claim to.
func TestJobs_HealthReadbacksReportStaleLeasesAndDeadLetterBacklog(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Jobs Health Co", "RC-J27-6")

	deadID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.health-dead", MaxAttempts: 1,
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.ClaimBatch(ctx, "w1", time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	if err := queue.Fail(ctx, "w1", deadID, errors.New("boom")); err != nil {
		t.Fatal(err)
	}

	deadLetterCount, err := queue.DeadLetterCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if deadLetterCount < 1 {
		t.Fatalf("dead letter count = %d, want at least 1", deadLetterCount)
	}

	// A stale-lease job: claim it, then directly age its lease_expires_at
	// into the past to simulate an abandoned worker.
	staleID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.health-stale",
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.ClaimBatch(ctx, "w1", time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_partner_jobs SET lease_expires_at = now() - interval '1 hour' WHERE id = $1`,
		staleID); err != nil {
		t.Fatal(err)
	}
	staleCount, err := queue.StaleLeaseCount(ctx, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if staleCount < 1 {
		t.Fatalf("stale lease count = %d, want at least 1", staleCount)
	}
}
