package jobs

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Section 14I durable-worker certification against real PostgreSQL.
//
// Section 14H's existing coverage is a static text assertion that the
// migration mentions lease columns, plus unit tests that bounded constants are
// bounded. Neither establishes that the coordinator actually holds a lease,
// refuses a second owner, recovers an expired one, dead-letters at the cap, or
// replays. Those are claims about SQL under concurrency, so they are answered
// here by the database rather than by reading the file.

func durablePool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres durable worker certification")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	var present bool
	if err = pool.QueryRow(ctx,
		`SELECT to_regclass('public.vendor_job_controls') IS NOT NULL`).Scan(&present); err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Skip("vendor_job_controls is absent; migrations have not been applied to the test database")
	}

	return pool, ctx
}

// jobName keeps every test on its own row so the package can run without
// tests interfering with one another's lease state.
func jobName(t *testing.T) string {
	t.Helper()
	return "s14i-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24]
}

// TestSection14IDurableLeaseHasOneOwner is the core exclusion guarantee: two
// instances of the service racing for the same named job must not both run it.
func TestSection14IDurableLeaseHasOneOwner(t *testing.T) {
	pool, ctx := durablePool(t)
	coordinator := NewDurableCoordinator(pool)
	name := jobName(t)

	first, err := coordinator.Claim(ctx, name, "owner-a", time.Minute, 3, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		t.Fatal("the first worker could not claim an unheld job")
	}

	second, err := coordinator.Claim(ctx, name, "owner-b", time.Minute, 3, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if second {
		t.Fatal("a second worker claimed a job that is already leased")
	}

	// The holder can heartbeat; the other instance cannot.
	if err = coordinator.Heartbeat(ctx, name, "owner-a", time.Minute); err != nil {
		t.Fatalf("the lease holder could not heartbeat: %v", err)
	}
	if err = coordinator.Heartbeat(ctx, name, "owner-b", time.Minute); err == nil {
		t.Fatal("a worker without the lease was allowed to heartbeat")
	}

	// Completion is likewise restricted to the holder.
	if err = coordinator.Complete(ctx, name, "owner-b", time.Second); err == nil {
		t.Fatal("a worker without the lease completed the job")
	}
	if err = coordinator.Complete(ctx, name, "owner-a", time.Millisecond); err != nil {
		t.Fatalf("the lease holder could not complete: %v", err)
	}
}

func TestDurableWorkerHealthSurfacesBacklogAndDeadLetters(t *testing.T) {
	pool, ctx := durablePool(t)
	coordinator := NewDurableCoordinator(pool)
	readyName, deadName := jobName(t), jobName(t)
	if err := coordinator.ensure(ctx, readyName, 3); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.ensure(ctx, deadName, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE vendor_job_controls SET status='DEAD_LETTER' WHERE job_name=$1`, deadName); err != nil {
		t.Fatal(err)
	}
	health, err := coordinator.Health(ctx, true, true, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if health.State != WorkerBlocked || health.PendingCount < 1 || health.DeadLetterCount < 1 {
		t.Fatalf("health did not surface durable backlog: %+v", health)
	}
	metrics := health.Metrics()
	if metrics["vendor_worker_dead_letter_total"] < 1 || metrics["vendor_worker_pending_total"] < 1 {
		t.Fatalf("worker metrics incomplete: %+v", metrics)
	}
}

// TestSection14IExpiredLeaseIsRecovered proves a worker that died holding a
// lease does not strand the job forever.
func TestSection14IExpiredLeaseIsRecovered(t *testing.T) {
	pool, ctx := durablePool(t)
	coordinator := NewDurableCoordinator(pool)
	name := jobName(t)

	claimed, err := coordinator.Claim(ctx, name, "owner-dead", time.Minute, 3, time.Millisecond)
	if err != nil || !claimed {
		t.Fatalf("initial claim failed claimed=%t err=%v", claimed, err)
	}

	// Simulate the holder disappearing: the lease is still recorded, but it has
	// expired and no heartbeat is arriving.
	if _, err = pool.Exec(ctx,
		`UPDATE vendor_job_controls
		    SET lease_expires_at = now() - interval '1 minute',
		        heartbeat_at = now() - interval '1 minute'
		  WHERE job_name = $1`, name); err != nil {
		t.Fatal(err)
	}

	// Recovery releases the dead lease and schedules a retry after the backoff;
	// it deliberately does not hand the job straight to the next caller, so a
	// crash-looping worker cannot spin. Asserting immediate claimability here
	// would be racing that backoff, not testing recovery.
	// A long retry backoff makes this call's outcome certain: recovery reschedules
	// the job an hour out, so the claim inside the very same call cannot also
	// succeed. With a millisecond backoff the two outcomes race each other.
	claimedDuringRecovery, err := coordinator.Claim(
		ctx, name, "owner-live", time.Minute, 3, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if claimedDuringRecovery {
		t.Fatal("a job was claimed in the same call that recovered its lease, " +
			"so a crash-looping worker would spin without backoff")
	}

	var status, lastError string
	var attempts int
	var leaseOwner, leaseExpires *string

	if err = pool.QueryRow(ctx,
		`SELECT status, attempt_count, last_error_redacted,
		        lease_owner, lease_expires_at::text
		   FROM vendor_job_controls WHERE job_name = $1`,
		name).Scan(&status, &attempts, &lastError, &leaseOwner, &leaseExpires); err != nil {
		t.Fatal(err)
	}

	if leaseOwner != nil || leaseExpires != nil {
		t.Fatalf("the dead worker still holds the lease: owner=%v expires=%v",
			leaseOwner, leaseExpires)
	}
	if status != "RETRY" {
		t.Fatalf("status after lease expiry = %q, want RETRY", status)
	}
	if attempts != 1 {
		t.Fatalf("attempt_count after lease expiry = %d, want 1", attempts)
	}
	if lastError == "" {
		t.Error("lease expiry recorded no reason, so an operator cannot diagnose it")
	}

	// Once the retry is due, a live worker takes it. This is the part that
	// proves the job is not stranded.
	if _, err = pool.Exec(ctx,
		`UPDATE vendor_job_controls
		    SET next_run_at = now() - interval '1 second'
		  WHERE job_name = $1`, name); err != nil {
		t.Fatal(err)
	}

	recovered, err := coordinator.Claim(ctx, name, "owner-live", time.Minute, 3, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !recovered {
		t.Fatal("an expired lease was never recovered; the job is stranded")
	}

	var owner string
	if err = pool.QueryRow(ctx,
		`SELECT lease_owner FROM vendor_job_controls WHERE job_name = $1`,
		name).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != "owner-live" {
		t.Fatalf("lease owner after recovery = %q, want owner-live", owner)
	}
}

// TestSection14IRetriesAreBoundedThenDeadLettered walks a job to its attempt
// cap and proves it stops retrying rather than looping forever, then proves a
// dead-lettered job can be replayed.
// waitUntilClaimable blocks until a RETRY job's backoff has elapsed.
//
// It is a condition, not a delay: it returns the instant the row says the job
// is due, and fails rather than continuing blind if that never happens. The
// retry base doubles per attempt (1ms, 2ms, 4ms...), so a loop that simply
// re-claimed immediately raced its own backoff and failed intermittently -
// which is what this replaces. The test is about attempt bounding and
// dead-lettering, not about how long a backoff lasts.
func waitUntilClaimable(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	name string,
) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for {
		var due bool
		if err := pool.QueryRow(ctx, `
			SELECT next_run_at IS NULL OR next_run_at <= now()
			  FROM vendor_job_controls WHERE job_name = $1`, name).Scan(&due); err != nil {
			t.Fatalf("check job readiness: %v", err)
		}
		if due {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("job %s never became claimable within the bound", name)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSection14IRetriesAreBoundedThenDeadLettered(t *testing.T) {
	pool, ctx := durablePool(t)
	coordinator := NewDurableCoordinator(pool)
	name := jobName(t)

	const maxAttempts = 3
	passErr := errors.New("certification failure")

	var dead bool

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Wait for the previous attempt's backoff to elapse before claiming.
		if attempt > 1 {
			waitUntilClaimable(t, pool, ctx, name)
		}

		claimed, err := coordinator.Claim(
			ctx, name, "owner-a", time.Minute, maxAttempts, time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		if !claimed {
			t.Fatalf("attempt %d could not claim a retryable job", attempt)
		}

		dead, err = coordinator.Fail(ctx, name, "owner-a", time.Millisecond, passErr)
		if err != nil {
			t.Fatal(err)
		}

		if attempt < maxAttempts && dead {
			t.Fatalf("job dead-lettered on attempt %d of %d", attempt, maxAttempts)
		}
	}

	if !dead {
		t.Fatalf("job never dead-lettered after %d attempts", maxAttempts)
	}

	var status string
	var attemptCount int
	var lastError string
	if err := pool.QueryRow(ctx,
		`SELECT status, attempt_count, last_error_redacted
		   FROM vendor_job_controls WHERE job_name = $1`,
		name).Scan(&status, &attemptCount, &lastError); err != nil {
		t.Fatal(err)
	}

	if status != "DEAD_LETTER" {
		t.Errorf("status after exhausting attempts = %q, want DEAD_LETTER", status)
	}
	if attemptCount != maxAttempts {
		t.Errorf("attempt_count = %d, want %d", attemptCount, maxAttempts)
	}

	// A dead-lettered job must not be claimable until someone replays it.
	claimed, err := coordinator.Claim(ctx, name, "owner-a", time.Minute, maxAttempts, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("a dead-lettered job was claimed without a replay")
	}

	if err = coordinator.Replay(ctx, name, "section 14I certification"); err != nil {
		t.Fatalf("replay failed: %v", err)
	}

	if err = pool.QueryRow(ctx,
		`SELECT status, attempt_count FROM vendor_job_controls WHERE job_name = $1`,
		name).Scan(&status, &attemptCount); err != nil {
		t.Fatal(err)
	}
	if status != "READY" || attemptCount != 0 {
		t.Fatalf("after replay status=%q attempts=%d; want READY/0", status, attemptCount)
	}

	claimed, err = coordinator.Claim(ctx, name, "owner-a", time.Minute, maxAttempts, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("a replayed job could not be claimed")
	}
}

// TestSection14IConcurrentClaimsElectOneWinner is the multi-instance case run
// as an actual race rather than as two sequential calls.
func TestSection14IConcurrentClaimsElectOneWinner(t *testing.T) {
	pool, ctx := durablePool(t)
	coordinator := NewDurableCoordinator(pool)

	const contenders = 8

	for round := 0; round < 5; round++ {
		name := jobName(t)

		// Materialise the row first so every contender races the same claim
		// rather than racing row creation.
		if _, err := coordinator.Claim(ctx, name, "seed", time.Minute, 3, time.Millisecond); err != nil {
			t.Fatal(err)
		}
		if err := coordinator.Complete(ctx, name, "seed", time.Millisecond); err != nil {
			t.Fatal(err)
		}

		// Make the job due explicitly. Sleeping until a millisecond interval
		// elapses is a timing assumption, and a loaded machine can miss it,
		// which would fail this test for a reason that has nothing to do with
		// the exclusion property under test.
		if _, err := pool.Exec(ctx,
			`UPDATE vendor_job_controls
			    SET next_run_at = now() - interval '1 second'
			  WHERE job_name = $1`, name); err != nil {
			t.Fatal(err)
		}

		results := make(chan bool, contenders)
		start := make(chan struct{})

		for i := 0; i < contenders; i++ {
			owner := "owner-" + uuid.NewString()[:8]
			go func() {
				<-start
				claimed, err := coordinator.Claim(
					ctx, name, owner, time.Minute, 3, time.Millisecond)
				if err != nil {
					results <- false
					return
				}
				results <- claimed
			}()
		}

		close(start)

		winners := 0
		for i := 0; i < contenders; i++ {
			if <-results {
				winners++
			}
		}

		if winners != 1 {
			t.Fatalf("round %d: %d workers claimed the same job; want exactly 1", round, winners)
		}
	}
}
