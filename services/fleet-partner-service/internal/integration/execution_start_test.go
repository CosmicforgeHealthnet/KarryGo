package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
)

// seedExecutionStart queues durable execution work for an assignment.
func seedExecutionStart(t *testing.T, pool *pgxpool.Pool, organizationID string,
	assignment *mktmodels.Assignment, responseID string, projectionID string) *mktmodels.ExecutionStartOperation {
	t.Helper()
	repo := mktrepos.NewExecutionStartRepository(pool)
	op := &mktmodels.ExecutionStartOperation{
		OrganizationID:      organizationID,
		AssignmentID:        assignment.ID,
		ProviderResponseID:  responseID,
		RequestProjectionID: projectionID,
		SourceService:       mktmodels.SourceDispatch,
		IdempotencyKey:      "exec-" + assignment.ID,
		RequestFingerprint:  "fp-" + assignment.ID,
	}
	if err := database.WithinTx(context.Background(), pool, func(tx pgx.Tx) error {
		return repo.Create(context.Background(), tx, op)
	}); err != nil {
		t.Fatalf("seed execution start: %v", err)
	}
	stored, err := repo.GetByAssignment(context.Background(), assignment.ID)
	if err != nil {
		t.Fatalf("reload execution start: %v", err)
	}
	return stored
}

func TestExecutionStartIsQueuedOncePerAssignment(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewExecutionStartRepository(pool)
	org := createOrganization(t, pool, "ExecStart Org", "RC-EXS-1")
	projection := seedProjection(t, pool, org.ID, "req-exs-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-exs-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Minute))

	first := seedExecutionStart(t, pool, org.ID, assignment, "resp-1", projection.ID)
	if first.Status != mktmodels.ExecutionStartPending {
		t.Fatalf("status = %s, want PENDING", first.Status)
	}

	// A second queue attempt for the same assignment is a no-op, so a retried
	// acceptance cannot schedule the canonical trip twice.
	second := &mktmodels.ExecutionStartOperation{
		OrganizationID: org.ID, AssignmentID: assignment.ID, ProviderResponseID: "resp-2",
		RequestProjectionID: projection.ID, SourceService: mktmodels.SourceDispatch,
		IdempotencyKey: "exec-other-key", RequestFingerprint: "fp-other",
	}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return repo.Create(ctx, tx, second)
	}); err != nil {
		t.Fatalf("second create: %v", err)
	}

	var count int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_execution_start_operations WHERE assignment_id = $1`,
		assignment.ID).Scan(&count)
	if count != 1 {
		t.Fatalf("execution-start rows = %d, want exactly 1", count)
	}

	// The surviving row keeps the ORIGINAL idempotency key, which is what makes
	// read-back resolve the first command rather than a new one.
	stored, _ := repo.GetByAssignment(ctx, assignment.ID)
	if stored.IdempotencyKey != "exec-"+assignment.ID {
		t.Fatalf("idempotency key = %s, want the original", stored.IdempotencyKey)
	}
}

func TestExecutionStartClaimIsExclusiveAndLeaseRecovers(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewExecutionStartRepository(pool)
	org := createOrganization(t, pool, "Claim Org", "RC-CLM-1")
	projection := seedProjection(t, pool, org.ID, "req-clm-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-clm-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Minute))
	seedExecutionStart(t, pool, org.ID, assignment, "resp-clm", projection.ID)

	// Worker A claims it.
	var claimedA []mktmodels.ExecutionStartOperation
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		claimedA, err = repo.Claim(ctx, tx, "worker-a", time.Minute, 10)
		return err
	}); err != nil {
		t.Fatalf("claim A: %v", err)
	}
	if len(claimedA) != 1 {
		t.Fatalf("worker A claimed %d, want 1", len(claimedA))
	}

	// Worker B sees nothing: the row is leased.
	var claimedB []mktmodels.ExecutionStartOperation
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		claimedB, err = repo.Claim(ctx, tx, "worker-b", time.Minute, 10)
		return err
	}); err != nil {
		t.Fatalf("claim B: %v", err)
	}
	if len(claimedB) != 0 {
		t.Fatalf("worker B claimed %d rows that worker A already holds", len(claimedB))
	}

	// Worker A dies. Expire its lease and prove the work is recovered rather
	// than stranded forever.
	if _, err := pool.Exec(ctx, `
		UPDATE fleet_partner_execution_start_operations
		SET lease_expires_at = now() - interval '1 minute'
		WHERE assignment_id = $1`, assignment.ID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	recovered, err := repo.RecoverExpiredLeases(ctx, nil)
	if err != nil || recovered != 1 {
		t.Fatalf("recovered = %d, %v; want 1", recovered, err)
	}

	var claimedC []mktmodels.ExecutionStartOperation
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		claimedC, err = repo.Claim(ctx, tx, "worker-c", time.Minute, 10)
		return err
	}); err != nil {
		t.Fatalf("claim C: %v", err)
	}
	if len(claimedC) != 1 {
		t.Fatalf("after lease recovery worker C claimed %d, want 1", len(claimedC))
	}
}

func TestExecutionStartFailureBackoffAndDeadLetterAndReplay(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewExecutionStartRepository(pool)
	org := createOrganization(t, pool, "Fail Org", "RC-FAI-1")
	projection := seedProjection(t, pool, org.ID, "req-fai-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-fai-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Minute))
	op := seedExecutionStart(t, pool, org.ID, assignment, "resp-fai", projection.ID)

	// A temporary failure schedules a retry.
	if err := repo.Fail(ctx, nil, op.ID, "source_service_unavailable", time.Second, 3, false); err != nil {
		t.Fatalf("temporary fail: %v", err)
	}
	reloaded, _ := repo.GetByAssignment(ctx, assignment.ID)
	if reloaded.Status != mktmodels.ExecutionStartRetryScheduled || reloaded.AttemptCount != 1 {
		t.Fatalf("after temporary failure: status=%s attempts=%d",
			reloaded.Status, reloaded.AttemptCount)
	}

	// A permanent rejection stops immediately rather than burning the budget.
	if err := repo.Fail(ctx, nil, op.ID, "execution_creation_rejected", 0, 3, true); err != nil {
		t.Fatalf("permanent fail: %v", err)
	}
	rejected, _ := repo.GetByAssignment(ctx, assignment.ID)
	if rejected.Status != mktmodels.ExecutionStartRejected {
		t.Fatalf("permanent rejection status = %s, want REJECTED", rejected.Status)
	}

	// Exhausting attempts dead-letters the operation.
	if _, err := pool.Exec(ctx, `
		UPDATE fleet_partner_execution_start_operations
		SET status = 'RETRY_SCHEDULED', attempt_count = 2 WHERE id = $1`, op.ID); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := repo.Fail(ctx, nil, op.ID, "source_service_unavailable", time.Second, 3, false); err != nil {
		t.Fatalf("exhausting fail: %v", err)
	}
	dead, _ := repo.GetByAssignment(ctx, assignment.ID)
	if dead.Status != mktmodels.ExecutionStartDeadLetter {
		t.Fatalf("status = %s, want DEAD_LETTER", dead.Status)
	}

	// An operator can replay a dead-lettered operation.
	if err := repo.Replay(ctx, op.ID); err != nil {
		t.Fatalf("replay: %v", err)
	}
	replayed, _ := repo.GetByAssignment(ctx, assignment.ID)
	if replayed.Status != mktmodels.ExecutionStartPending || replayed.AttemptCount != 0 {
		t.Fatalf("after replay: status=%s attempts=%d", replayed.Status, replayed.AttemptCount)
	}
	// Replaying a non-dead-lettered operation is refused.
	if err := repo.Replay(ctx, op.ID); !errors.Is(err, mktrepos.ErrNotFound) {
		t.Fatalf("replay of live operation = %v, want ErrNotFound", err)
	}
}

func TestExecutionStartUnknownOutcomeKeepsOriginalIdempotencyKey(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := mktrepos.NewExecutionStartRepository(pool)
	org := createOrganization(t, pool, "Unknown Org", "RC-UNK-1")
	projection := seedProjection(t, pool, org.ID, "req-unk-1", mktmodels.ProjectionOrganizationAccepted)
	membership := seedProviderMembership(t, pool, org.ID, "drv-unk-1")
	assignment := seedAssignment(t, pool, org.ID, projection.ID, membership, time.Now().Add(time.Minute))
	op := seedExecutionStart(t, pool, org.ID, assignment, "resp-unk", projection.ID)
	originalKey := op.IdempotencyKey

	if err := repo.MarkUnknown(ctx, nil, op.ID, "execution_outcome_unknown", time.Second); err != nil {
		t.Fatalf("mark unknown: %v", err)
	}
	unknown, _ := repo.GetByAssignment(ctx, assignment.ID)
	if unknown.Status != mktmodels.ExecutionStartUnknown {
		t.Fatalf("status = %s, want UNKNOWN_OUTCOME", unknown.Status)
	}
	// This is the property that prevents duplicate trips: the next attempt asks
	// "did my earlier command succeed?" using the same key.
	if unknown.IdempotencyKey != originalKey {
		t.Fatalf("idempotency key changed after unknown outcome: %s -> %s",
			originalKey, unknown.IdempotencyKey)
	}
	if !unknown.NeedsReadBack() {
		t.Fatal("an unknown outcome must require read-back")
	}

	// Confirming records the canonical trip and completes the operation.
	if err := repo.Confirm(ctx, nil, op.ID, "trip-123", map[string]any{"status": "CREATED"}); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	confirmed, _ := repo.GetByAssignment(ctx, assignment.ID)
	if confirmed.Status != mktmodels.ExecutionStartConfirmed ||
		confirmed.TripID == nil || *confirmed.TripID != "trip-123" {
		t.Fatalf("confirmed = %s trip=%v", confirmed.Status, confirmed.TripID)
	}
	if confirmed.CompletedAt == nil {
		t.Fatal("a confirmed operation must record completion")
	}
}
