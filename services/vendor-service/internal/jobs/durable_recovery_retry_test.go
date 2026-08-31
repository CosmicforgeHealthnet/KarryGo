package jobs

// Section 17 Part 3: a transient failure must be retried and then succeed.
//
// Section 14I already proved the bounded-retry-then-dead-letter path and the
// replay that follows it. What it did not prove is the ordinary case an outage
// actually produces: a pass fails while the dependency is down, is retried
// once the backoff elapses, and completes when the dependency returns — with
// the attempt budget restored, so a transient outage does not silently spend
// the allowance a genuine fault would need.
//
// This is the behaviour commerce-payment-recovery relies on. Its pass now
// returns sweep-level errors rather than discarding them, and this is what the
// coordinator does with them.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func jobControlRow(
	t *testing.T, pool *pgxpool.Pool, ctx context.Context, name string,
) (status string, attempts int, lastError string) {
	t.Helper()

	if err := pool.QueryRow(ctx,
		`SELECT status, attempt_count, last_error_redacted
		   FROM vendor_job_controls WHERE job_name = $1`,
		name).Scan(&status, &attempts, &lastError); err != nil {
		t.Fatalf("read job control row: %v", err)
	}
	return status, attempts, lastError
}

func TestSection17TransientFailureIsRetriedThenCompletes(t *testing.T) {
	pool, ctx := durablePool(t)
	coordinator := NewDurableCoordinator(pool)
	name := jobName(t)

	// The dependency is down for the first two passes, then returns.
	outage := errors.New("recovery sweep could not reach the database")
	var passes int

	job := DurableJob{
		Job: Job{
			Name:     name,
			Interval: time.Millisecond,
			Pass: func(context.Context) error {
				passes++
				if passes <= 2 {
					return outage
				}
				return nil
			},
		},
		Coordinator: coordinator,
		Owner:       "owner-a",
		LeaseFor:    time.Minute,
		MaxAttempts: 5,
		RetryBase:   time.Millisecond,
	}

	var workerErrs []error
	record := func(_ string, err error) { workerErrs = append(workerErrs, err) }

	// Pass one: the outage is recorded, and the job is queued for retry rather
	// than abandoned.
	job.runOnce(ctx, record)

	status, attempts, lastError := jobControlRow(t, pool, ctx, name)
	if status != "RETRY" {
		t.Fatalf("status after a transient failure = %q, want RETRY", status)
	}
	if attempts != 1 {
		t.Fatalf("attempt_count after one failure = %d, want 1", attempts)
	}
	if lastError == "" {
		t.Error("the failure was recorded without any cause for the operator")
	}
	if len(workerErrs) != 1 {
		t.Fatalf("the worker reported %d errors after one failing pass, want 1",
			len(workerErrs))
	}

	// Pass two: still down. The budget is spent one attempt at a time, not all
	// at once, and the job is still short of its cap.
	waitUntilClaimable(t, pool, ctx, name)
	job.runOnce(ctx, record)

	status, attempts, _ = jobControlRow(t, pool, ctx, name)
	if status != "RETRY" {
		t.Fatalf("status after a second transient failure = %q, want RETRY", status)
	}
	if attempts != 2 {
		t.Fatalf("attempt_count after two failures = %d, want 2", attempts)
	}

	// Pass three: the dependency is back. The job completes.
	waitUntilClaimable(t, pool, ctx, name)
	job.runOnce(ctx, record)

	status, attempts, lastError = jobControlRow(t, pool, ctx, name)
	if status != "READY" {
		t.Fatalf("status after a successful retry = %q, want READY", status)
	}
	// The budget is restored: two failures during an outage must not leave the
	// job one fault away from dead-lettering forever after.
	if attempts != 0 {
		t.Fatalf("attempt_count after a successful retry = %d, want 0", attempts)
	}
	if lastError != "" {
		t.Errorf("a stale failure %q survived a successful pass", lastError)
	}
	if passes != 3 {
		t.Fatalf("the pass ran %d times, want 3", passes)
	}
	if len(workerErrs) != 2 {
		t.Fatalf("the worker reported %d errors, want the 2 failing passes only",
			len(workerErrs))
	}
}

// A pass that keeps failing must still reach the cap: retrying transient
// failures must not make the dead-letter bound unreachable.
func TestSection17PersistentFailureStillReachesTheDeadLetterBound(t *testing.T) {
	pool, ctx := durablePool(t)
	coordinator := NewDurableCoordinator(pool)
	name := jobName(t)

	const maxAttempts = 3

	job := DurableJob{
		Job: Job{
			Name:     name,
			Interval: time.Millisecond,
			Pass: func(context.Context) error {
				return errors.New("the dependency never came back")
			},
		},
		Coordinator: coordinator,
		Owner:       "owner-a",
		LeaseFor:    time.Minute,
		MaxAttempts: maxAttempts,
		RetryBase:   time.Millisecond,
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			waitUntilClaimable(t, pool, ctx, name)
		}
		job.runOnce(ctx, nil)
	}

	status, attempts, lastError := jobControlRow(t, pool, ctx, name)
	if status != "DEAD_LETTER" {
		t.Fatalf("status after %d failing passes = %q, want DEAD_LETTER",
			maxAttempts, status)
	}
	if attempts != maxAttempts {
		t.Fatalf("attempt_count = %d, want %d", attempts, maxAttempts)
	}
	if lastError == "" {
		t.Error("a dead-lettered job carries no cause for the operator")
	}

	// And it stays parked until a human replays it, rather than looping.
	waitUntilClaimable(t, pool, ctx, name)
	job.runOnce(ctx, nil)

	if status, _, _ = jobControlRow(t, pool, ctx, name); status != "DEAD_LETTER" {
		t.Fatalf("a dead-lettered job ran again on its own: status=%q", status)
	}
}
