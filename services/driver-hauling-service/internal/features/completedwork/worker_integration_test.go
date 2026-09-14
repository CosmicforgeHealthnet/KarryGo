package completedwork

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/database"
	"cosmicforge/logistics/shared/go/providercompat"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required for completed-work delivery tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, sourceFile, _, _ := runtime.Caller(0)
	serviceRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(serviceRoot); err != nil {
		t.Fatal(err)
	}
	if err = database.ApplyMigrations(context.Background(), pool); err != nil {
		_ = os.Chdir(previous)
		t.Fatalf("apply migrations: %v", err)
	}
	if err = os.Chdir(previous); err != nil {
		t.Fatal(err)
	}
	return pool
}

type stubSubmitter struct {
	result   providercompat.SubmitResult
	err      error
	requests []providercompat.CompletedWorkRequest
}

func (s *stubSubmitter) SubmitCompletedWork(_ context.Context, request providercompat.CompletedWorkRequest) (providercompat.SubmitResult, error) {
	s.requests = append(s.requests, request)
	return s.result, s.err
}

// enqueueRow writes one completed-work row directly, standing in for the
// completion transaction so these tests exercise the WORKER in isolation.
func enqueueRow(t *testing.T, pool *pgxpool.Pool, bookingID string, version int64) providercompat.CompletedWorkRequest {
	t.Helper()
	binding := Binding{
		ProviderID: "prov-" + bookingID, OrganizationID: "org-1", MembershipID: "mem-1",
		FleetAssignmentID: "asg-" + bookingID, ExecutionMode: ExecutionModeManaged,
		CompletionVersion: version, CorrelationID: "corr-" + bookingID,
		FareMinor: 500000, Currency: "NGN",
	}
	request := RequestFor(bookingID, binding, time.Now().UTC().Truncate(time.Second))
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(context.Background(), tx, bookingID, request); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatalf("enqueue: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return request
}

func countRows(t *testing.T, pool *pgxpool.Pool, bookingID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM hauling_domain_outbox WHERE target=$1 AND aggregate_id=$2::uuid`,
		Target, bookingID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func stateOf(t *testing.T, pool *pgxpool.Pool, bookingID string) (string, int) {
	t.Helper()
	var state string
	var attempts int
	if err := pool.QueryRow(context.Background(),
		`SELECT reconciliation_state, attempts FROM hauling_domain_outbox
		  WHERE target=$1 AND aggregate_id=$2::uuid`, Target, bookingID).
		Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	return state, attempts
}

// HLG-019: enqueuing the same completion twice leaves exactly one row, so a
// retried or replayed completion cannot produce two Fleet records.
func TestEnqueueIsIdempotentPerCompletion(t *testing.T) {
	pool := testPool(t)
	bookingID := uuid.NewString()

	enqueueRow(t, pool, bookingID, 3)
	enqueueRow(t, pool, bookingID, 3)
	enqueueRow(t, pool, bookingID, 3)

	if got := countRows(t, pool, bookingID); got != 1 {
		t.Fatalf("rows = %d, want exactly 1 for one completion version", got)
	}
}

// HLG-018 / Scenario T: Fleet unavailable leaves the completion recorded and
// the delivery queued for retry, never lost and never dead-lettered early.
func TestWorkerRetriesWhileFleetIsUnavailable(t *testing.T) {
	pool := testPool(t)
	bookingID := uuid.NewString()
	enqueueRow(t, pool, bookingID, 1)

	submitter := &stubSubmitter{
		result: providercompat.SubmitResult{Outcome: providercompat.SubmitRetryable, Code: "http_503"},
		err:    errors.New("fleet down"),
	}
	worker := &Worker{DB: pool, Client: submitter, Enabled: true, MaxAttempts: 5}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("worker: %v", err)
	}

	state, attempts := stateOf(t, pool, bookingID)
	if state != ReconRetryScheduled {
		t.Fatalf("state = %q, want RETRY_SCHEDULED", state)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if got := countRows(t, pool, bookingID); got != 1 {
		t.Fatalf("rows = %d, want 1", got)
	}
}

// HLG-019 / Scenario T: once Fleet answers, the record is delivered exactly
// once and the redelivery carries the SAME idempotency key.
func TestWorkerDeliversOnceWithStableIdentity(t *testing.T) {
	pool := testPool(t)
	bookingID := uuid.NewString()
	request := enqueueRow(t, pool, bookingID, 2)

	submitter := &stubSubmitter{result: providercompat.SubmitResult{Outcome: providercompat.SubmitUnknown}}
	worker := &Worker{DB: pool, Client: submitter, Enabled: true, MaxAttempts: 5,
		BaseBackoff: time.Nanosecond, MaxBackoff: time.Nanosecond}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("first pass: %v", err)
	}

	// Fleet actually held it: the redelivery answers DUPLICATE, which is a
	// success because it proves exactly one record exists.
	submitter.result = providercompat.SubmitResult{Outcome: providercompat.SubmitDuplicate}
	time.Sleep(10 * time.Millisecond)
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("second pass: %v", err)
	}

	state, _ := stateOf(t, pool, bookingID)
	if state != ReconDelivered {
		t.Fatalf("state = %q, want DELIVERED", state)
	}
	if len(submitter.requests) != 2 {
		t.Fatalf("submissions = %d, want 2", len(submitter.requests))
	}
	if submitter.requests[0].IdempotencyKey != submitter.requests[1].IdempotencyKey {
		t.Fatal("a redelivery must reuse the original idempotency key")
	}
	if submitter.requests[0].IdempotencyKey != request.IdempotencyKey {
		t.Fatal("the submitted key must be the one enqueued")
	}
}

// HLG-020: a terminal Fleet rejection dead-letters immediately instead of
// burning the whole attempt budget first.
func TestWorkerTerminalRejectionDeadLettersImmediately(t *testing.T) {
	pool := testPool(t)
	bookingID := uuid.NewString()
	enqueueRow(t, pool, bookingID, 1)

	submitter := &stubSubmitter{result: providercompat.SubmitResult{
		Outcome: providercompat.SubmitTerminal, Code: "forbidden"}}
	worker := &Worker{DB: pool, Client: submitter, Enabled: true, MaxAttempts: 10}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("worker: %v", err)
	}

	state, attempts := stateOf(t, pool, bookingID)
	if state != ReconFailedTerminal {
		t.Fatalf("state = %q, want FAILED_TERMINAL", state)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d; a terminal rejection must not consume the budget", attempts)
	}
}

// HLG-020: an AMBIGUOUS outcome that exhausts the budget becomes
// RECONCILIATION_REQUIRED, not a claimed failure -- Fleet may hold the record.
func TestWorkerExhaustedAmbiguityNeedsReconciliation(t *testing.T) {
	pool := testPool(t)
	bookingID := uuid.NewString()
	enqueueRow(t, pool, bookingID, 1)

	submitter := &stubSubmitter{result: providercompat.SubmitResult{Outcome: providercompat.SubmitUnknown}}
	worker := &Worker{DB: pool, Client: submitter, Enabled: true, MaxAttempts: 1}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("worker: %v", err)
	}

	state, _ := stateOf(t, pool, bookingID)
	if state != ReconReconciliationRequired {
		t.Fatalf("state = %q, want RECONCILIATION_REQUIRED", state)
	}

	// HLG-020: an operator can requeue it, and the requeue is a REDELIVERY --
	// the payload and idempotency key are untouched.
	store := &Store{DB: pool}
	rows, err := store.List(context.Background(), ReconReconciliationRequired, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var target string
	for _, row := range rows {
		if row.SourceJobID == bookingID {
			target = row.ID
			if row.OrganizationID != "org-1" {
				t.Fatalf("operator view missing organization: %+v", row)
			}
		}
	}
	if target == "" {
		t.Fatal("the stuck row must appear in the reconciliation view")
	}
	if err := store.Requeue(context.Background(), target); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	state, attempts := stateOf(t, pool, bookingID)
	if state != ReconPending || attempts != 0 {
		t.Fatalf("after requeue state=%q attempts=%d, want PENDING/0", state, attempts)
	}

	submitter.result = providercompat.SubmitResult{Outcome: providercompat.SubmitDuplicate}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("post-requeue worker: %v", err)
	}
	if state, _ = stateOf(t, pool, bookingID); state != ReconDelivered {
		t.Fatalf("state = %q, want DELIVERED after repair", state)
	}
	if got := countRows(t, pool, bookingID); got != 1 {
		t.Fatalf("rows = %d; repair must never create a second record", got)
	}
}

// A disabled worker leaves rows untouched: turning the flag off must not
// silently dead-letter authoritative completions.
func TestWorkerDisabledLeavesRowsQueued(t *testing.T) {
	pool := testPool(t)
	bookingID := uuid.NewString()
	enqueueRow(t, pool, bookingID, 1)

	worker := &Worker{DB: pool, Client: &stubSubmitter{}, Enabled: false}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("worker: %v", err)
	}
	state, attempts := stateOf(t, pool, bookingID)
	if state != ReconPending || attempts != 0 {
		t.Fatalf("state=%q attempts=%d, want untouched PENDING/0", state, attempts)
	}
}
