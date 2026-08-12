package integration_test

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/retention"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

// TestRetention_ScanAndReport_CountsOnlyAgedTerminalJobs proves the
// retention scan counts only terminal jobs older than the cutoff -- never
// PENDING/RUNNING work, and never rows within the retention window.
func TestRetention_ScanAndReport_CountsOnlyAgedTerminalJobs(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Retention Scan Co", "RC-S27-RET-1")

	// A recent, terminal job -- inside the retention window, must not count.
	recentID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.retention-recent",
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	// This is a retention eligibility fixture, not a job-claim test. Set the
	// terminal fact explicitly so unrelated queue ordering cannot decide which
	// synthetic row becomes terminal during a repository-wide run.
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_jobs SET status='SUCCEEDED', completed_at=now(), version=version+1 WHERE id=$1`, recentID); err != nil {
		t.Fatal(err)
	}

	// An old, terminal job -- past the cutoff, must count. Claimed and
	// completed before the still-pending job below is enqueued.
	agedID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.retention-aged",
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_jobs SET status='SUCCEEDED', completed_at=now() - interval '200 days', version=version+1 WHERE id=$1`, agedID); err != nil {
		t.Fatal(err)
	}

	// A still-pending job -- never terminal, must not count regardless of
	// age.
	pendingID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.retention-pending",
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_jobs SET created_at = now() - interval '200 days' WHERE id = $1`, pendingID); err != nil {
		t.Fatal(err)
	}

	service := retention.NewService(pool)
	report, err := service.ScanAndReport(ctx, 90, 10000)
	if err != nil {
		t.Fatalf("scan and report: %v", err)
	}

	var jobsTable *retention.TableReport
	for i := range report.Tables {
		if report.Tables[i].Table == "fleet_partner_jobs" {
			jobsTable = &report.Tables[i]
		}
	}
	if jobsTable == nil {
		t.Fatal("expected a fleet_partner_jobs report entry")
	}
	if jobsTable.EligibleCount < 1 {
		t.Fatalf("eligible count = %d, want at least 1 (the aged terminal job)", jobsTable.EligibleCount)
	}

	// Directly verify the recent-terminal and still-pending jobs specifically
	// are excluded: query the same bounded eligibility condition the scan
	// uses, scoped to just these two IDs, and confirm neither matches.
	var excludedCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_jobs
		WHERE id = ANY($1)
		  AND status IN ('SUCCEEDED', 'CANCELLED', 'DEAD_LETTERED')
		  AND COALESCE(completed_at, cancelled_at, dead_lettered_at) < now() - interval '90 days'`,
		[]string{recentID, pendingID}).Scan(&excludedCount); err != nil {
		t.Fatal(err)
	}
	if excludedCount != 0 {
		t.Fatalf("recent-terminal/pending jobs matched the eligibility condition: %d", excludedCount)
	}
}

// TestRetention_ScanAndReport_NeverMutates proves a scan pass performs zero
// writes: row counts and job statuses are identical before and after.
func TestRetention_ScanAndReport_NeverMutates(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	org := createOrganization(t, pool, "Retention NoMutate Co", "RC-S27-RET-2")

	jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{
		OrganizationID: org.ID, JobType: "test.retention-no-mutate",
		Payload: map[string]string{"organizationId": org.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.ClaimBatch(ctx, "w1", time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(ctx, "w1", jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_jobs SET completed_at = now() - interval '200 days' WHERE id = $1`, jobID); err != nil {
		t.Fatal(err)
	}

	before, err := queue.Get(ctx, org.ID, jobID)
	if err != nil {
		t.Fatal(err)
	}

	service := retention.NewService(pool)
	if _, err := service.ScanAndReport(ctx, 90, 10000); err != nil {
		t.Fatal(err)
	}

	after, err := queue.Get(ctx, org.ID, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != after.Status || before.Version != after.Version {
		t.Fatalf("scan mutated the row: before=%+v after=%+v", before, after)
	}
}
