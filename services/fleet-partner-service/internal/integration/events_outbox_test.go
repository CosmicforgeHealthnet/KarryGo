package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
)

// TestOutbox_DeadLetterThenReplay proves the outbox gains the same
// dead-letter/replay recovery path every other durable-worker subsystem in
// this service already has: a record that exhausts its attempts is
// dead-lettered, is visible to ListDeadLettered, and ReplayDeadLettered
// returns it to PENDING (claimable again) exactly once.
func TestOutbox_DeadLetterThenReplay(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Outbox Replay Co", "RC-OUTBOX-1")
	ctx := context.Background()

	repo := events.NewOutboxRepository(pool)
	envelope, err := events.NewEnvelope("organization.test.outbox_replay.v1", 1, "test_aggregate",
		"agg_1", 1, org.ID, "cor_1", map[string]any{"k": "v"}, false)
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	if err := repo.Append(ctx, pool, envelope); err != nil {
		t.Fatalf("append: %v", err)
	}

	owner := "test-outbox-worker"
	claimed, err := repo.ClaimBatch(ctx, owner, time.Minute, 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed = %d records, want 1", len(claimed))
	}
	record := claimed[0]

	// Fail it enough times to exhaust a maxAttempts=1 ceiling, dead-lettering
	// it on the very first failure.
	if err := repo.MarkFailed(ctx, owner, record.ID, errors.New("simulated transport failure"), 1); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	deadLettered, err := repo.ListDeadLettered(ctx, 100)
	if err != nil {
		t.Fatalf("list dead-lettered: %v", err)
	}
	found := false
	for _, r := range deadLettered {
		if r.ID == record.ID {
			found = true
			if r.LastError == "" {
				t.Error("expected a redacted error message on the dead-lettered record")
			}
		}
	}
	if !found {
		t.Fatalf("record %s not found in dead-lettered list: %+v", record.ID, deadLettered)
	}

	// A dead-lettered record is not claimable.
	claimedAgain, err := repo.ClaimBatch(ctx, owner, time.Minute, 10)
	if err != nil {
		t.Fatalf("claim after dead-letter: %v", err)
	}
	for _, r := range claimedAgain {
		if r.ID == record.ID {
			t.Fatal("a dead-lettered record must not be claimable")
		}
	}

	if err := repo.ReplayDeadLettered(ctx, record.ID); err != nil {
		t.Fatalf("replay: %v", err)
	}

	// Replaying an already-pending record is refused, not silently repeated.
	if err := repo.ReplayDeadLettered(ctx, record.ID); !errors.Is(err, events.ErrOutboxRecordNotFound) {
		t.Fatalf("second replay err = %v, want ErrOutboxRecordNotFound", err)
	}

	// It is claimable again after replay.
	claimedAfterReplay, err := repo.ClaimBatch(ctx, owner+"-2", time.Minute, 10)
	if err != nil {
		t.Fatalf("claim after replay: %v", err)
	}
	replayedFound := false
	for _, r := range claimedAfterReplay {
		if r.ID == record.ID {
			replayedFound = true
			if r.Attempts != 0 {
				t.Errorf("attempts after replay = %d, want 0", r.Attempts)
			}
		}
	}
	if !replayedFound {
		t.Fatalf("replayed record %s was not claimable", record.ID)
	}
}

// TestOutbox_ReplayUnknownRecordIsNotFound proves replaying an id that was
// never dead-lettered (or never existed) is refused identically.
func TestOutbox_ReplayUnknownRecordIsNotFound(t *testing.T) {
	pool := testPool(t)
	repo := events.NewOutboxRepository(pool)
	if err := repo.ReplayDeadLettered(context.Background(), "evt_does_not_exist"); !errors.Is(err, events.ErrOutboxRecordNotFound) {
		t.Fatalf("err = %v, want ErrOutboxRecordNotFound", err)
	}
}

// TestOutbox_StaleCountDistinguishesAgeFromBacklog proves StaleCount answers
// "how many records have been stuck for longer than X", not just "how many
// are pending" — the read-back a reconciliation/alerting loop needs to tell
// a merely-busy worker apart from a transport that has been down for a
// while.
func TestOutbox_StaleCountDistinguishesAgeFromBacklog(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Outbox Stale Co", "RC-OUTBOX-2")
	ctx := context.Background()
	repo := events.NewOutboxRepository(pool)

	envelope, err := events.NewEnvelope("organization.test.outbox_stale.v1", 1, "test_aggregate",
		"agg_1", 1, org.ID, "cor_1", map[string]any{"k": "v"}, false)
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	if err := repo.Append(ctx, pool, envelope); err != nil {
		t.Fatalf("append: %v", err)
	}

	// Freshly appended: not yet stale under any reasonable threshold.
	stale, err := repo.StaleCount(ctx, time.Hour)
	if err != nil {
		t.Fatalf("stale count: %v", err)
	}
	if stale != 0 {
		t.Fatalf("stale count = %d, want 0 for a fresh record under a 1h threshold", stale)
	}

	// The same record counts as stale once the threshold is set to
	// something the record's actual age already exceeds.
	stale, err = repo.StaleCount(ctx, -time.Second)
	if err != nil {
		t.Fatalf("stale count: %v", err)
	}
	if stale < 1 {
		t.Fatalf("stale count = %d, want at least 1 with a negative (always-exceeded) threshold", stale)
	}
}
