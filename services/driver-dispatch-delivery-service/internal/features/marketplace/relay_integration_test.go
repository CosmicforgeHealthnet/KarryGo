package marketplace_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
)

// countingResponder records deliveries so tests can prove a decision is never
// delivered as two different decisions.
type countingResponder struct {
	mu         sync.Mutex
	keys       []string
	result     marketplace.RelayDeliveryResult
	err        error
	configured bool
}

func (c *countingResponder) Configured() bool { return c.configured }

func (c *countingResponder) Deliver(_ context.Context, _ marketplace.CallMeta,
	op marketplace.RelayOperation) (marketplace.RelayDeliveryResult, error) {
	c.mu.Lock()
	c.keys = append(c.keys, op.IdempotencyKey)
	c.mu.Unlock()
	if c.err != nil {
		return marketplace.RelayDeliveryResult{}, c.err
	}
	return c.result, nil
}

func (c *countingResponder) deliveries() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.keys))
	copy(out, c.keys)
	return out
}

func seedRelay(t *testing.T, pool *pgxpool.Pool, providerID string, assignmentID string,
	key string, fingerprint string) *marketplace.RelayOperation {
	t.Helper()
	repo := marketplace.NewRepository(pool)
	op := &marketplace.RelayOperation{
		OperationType:  marketplace.RelayProviderAssignmentAccept,
		ProviderID:     providerID,
		AssignmentID:   &assignmentID,
		IdempotencyKey: key, RequestFingerprint: fingerprint,
		Payload: []byte(`{"decision":"accept"}`),
	}
	var stored *marketplace.RelayOperation
	err := repo.WithinTx(context.Background(), func(tx pgx.Tx) error {
		var createErr error
		stored, _, createErr = repo.CreateRelay(context.Background(), tx, op)
		return createErr
	})
	if err != nil {
		t.Fatalf("seed relay: %v", err)
	}
	return stored
}

func TestRelayCreateIsIdempotentAndDetectsFingerprintConflict(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	providerID := seedProvider(t, pool, "+2348030000001")

	first := seedRelay(t, pool, providerID, "asg_relay_1", "relay-key-1", "fp-1")
	if first.Status != marketplace.RelayPending {
		t.Fatalf("status = %s, want PENDING", first.Status)
	}

	// Same key, same fingerprint: the ORIGINAL decision is returned.
	var replayed bool
	var second *marketplace.RelayOperation
	if err := repo.WithinTx(ctx, func(tx pgx.Tx) error {
		var err error
		second, replayed, err = repo.CreateRelay(ctx, tx, &marketplace.RelayOperation{
			OperationType: marketplace.RelayProviderAssignmentAccept, ProviderID: providerID,
			AssignmentID: strPtr("asg_relay_1"), IdempotencyKey: "relay-key-1",
			RequestFingerprint: "fp-1", Payload: []byte(`{"decision":"accept"}`),
		})
		return err
	}); err != nil {
		t.Fatalf("replay create: %v", err)
	}
	if !replayed || second.ID != first.ID {
		t.Fatalf("replay = %v, id %s want %s", replayed, second.ID, first.ID)
	}

	// Same key, DIFFERENT fingerprint: two different decisions claiming one
	// identity. That must be refused, not silently merged.
	err := repo.WithinTx(ctx, func(tx pgx.Tx) error {
		_, _, createErr := repo.CreateRelay(ctx, tx, &marketplace.RelayOperation{
			OperationType: marketplace.RelayProviderAssignmentAccept, ProviderID: providerID,
			AssignmentID: strPtr("asg_relay_1"), IdempotencyKey: "relay-key-1",
			RequestFingerprint: "fp-DIFFERENT", Payload: []byte(`{"decision":"decline"}`),
		})
		return createErr
	})
	if !errors.Is(err, marketplace.ErrIdempotencyConflict) {
		t.Fatalf("fingerprint conflict = %v, want ErrIdempotencyConflict", err)
	}

	var count int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&count)
	if count != 1 {
		t.Fatalf("relay rows = %d, want exactly 1", count)
	}
}

func TestRelayWorkerConfirmsAndReusesOriginalKey(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	providerID := seedProvider(t, pool, "+2348030000002")
	seedRelay(t, pool, providerID, "asg_relay_ok", "relay-ok", "fp-ok")

	responder := &countingResponder{
		configured: true,
		result:     marketplace.RelayDeliveryResult{Accepted: true, Snapshot: map[string]any{"ok": true}},
	}
	worker := marketplace.NewRelayWorker(repo, responder, marketplace.RelayWorkerConfig{})

	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Claimed != 1 || result.Confirmed != 1 {
		t.Fatalf("claimed=%d confirmed=%d, want 1/1", result.Claimed, result.Confirmed)
	}

	stored, err := repo.GetRelayByIdempotencyKey(ctx, nil,
		marketplace.RelayProviderAssignmentAccept, "relay-ok")
	if err != nil || stored.Status != marketplace.RelayConfirmed {
		t.Fatalf("status = %v, %v; want CONFIRMED", stored, err)
	}
	if stored.CompletedAt == nil {
		t.Fatal("a confirmed relay must record completion")
	}
	// The delivery used the ORIGINAL key, which is what lets fleet recognise a
	// redelivery as the same decision.
	deliveries := responder.deliveries()
	if len(deliveries) != 1 || deliveries[0] != "relay-ok" {
		t.Fatalf("deliveries = %v, want exactly [relay-ok]", deliveries)
	}

	// A second pass finds nothing: a confirmed relay is terminal.
	again, _ := worker.RunOnce(ctx)
	if again.Claimed != 0 {
		t.Fatalf("second pass claimed %d, want 0", again.Claimed)
	}
}

func TestRelayWorkerUnknownOutcomeIsNotRedelivered(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	providerID := seedProvider(t, pool, "+2348030000003")
	seedRelay(t, pool, providerID, "asg_relay_unk", "relay-unk", "fp-unk")

	// Fleet is unavailable: ambiguous, not a rejection.
	down := &countingResponder{configured: true, err: marketplace.ErrFleetUnavailable}
	worker := marketplace.NewRelayWorker(repo, down, marketplace.RelayWorkerConfig{
		BaseBackoff: time.Millisecond, MaxAttempts: 5,
	})
	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Unknown != 1 {
		t.Fatalf("unknown = %d, want 1", result.Unknown)
	}
	stored, _ := repo.GetRelayByIdempotencyKey(ctx, nil,
		marketplace.RelayProviderAssignmentAccept, "relay-unk")
	if stored.Status != marketplace.RelayUnknownOutcome {
		t.Fatalf("status = %s, want UNKNOWN_OUTCOME", stored.Status)
	}
	if !stored.NeedsReadBack() {
		t.Fatal("an unknown outcome must require read-back")
	}
	// The provider's decision is preserved. Compare semantically: PostgreSQL
	// stores JSONB in canonical form, so byte equality would fail on
	// whitespace alone and tell us nothing about the decision.
	if stored.IdempotencyKey != "relay-unk" {
		t.Fatalf("idempotency key mutated: %s", stored.IdempotencyKey)
	}
	var decision map[string]any
	if err := json.Unmarshal(stored.Payload, &decision); err != nil {
		t.Fatalf("stored payload is not valid JSON: %s", stored.Payload)
	}
	if decision["decision"] != "accept" {
		t.Fatalf("decision mutated: %v", decision)
	}

	// Fleet recovering does not prove the outcome. Force the retry due so this
	// test cannot accidentally pass because the backoff has not elapsed.
	if _, err := pool.Exec(ctx, `UPDATE dispatch_fleet_relay_operations SET next_attempt_at=now()-interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	up := &countingResponder{configured: true,
		result: marketplace.RelayDeliveryResult{Accepted: true}}
	recovered := marketplace.NewRelayWorker(repo, up, marketplace.RelayWorkerConfig{
		BaseBackoff: time.Millisecond,
	})
	if _, err := recovered.RunOnce(ctx); err != nil {
		t.Fatalf("recovery run: %v", err)
	}
	final, _ := repo.GetRelayByIdempotencyKey(ctx, nil,
		marketplace.RelayProviderAssignmentAccept, "relay-unk")
	if final.Status != marketplace.RelayUnknownOutcome {
		t.Fatalf("after recovery status = %s, want UNKNOWN_OUTCOME", final.Status)
	}
	if keys := up.deliveries(); len(keys) != 0 {
		t.Fatalf("uncertain decision was redelivered: %v", keys)
	}

	var rows int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&rows)
	if rows != 1 {
		t.Fatalf("relay rows = %d, want exactly 1 across the whole outage", rows)
	}
}

func TestRelayWorkerPermanentRejectionStopsRetrying(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	providerID := seedProvider(t, pool, "+2348030000004")
	seedRelay(t, pool, providerID, "asg_relay_rej", "relay-rej", "fp-rej")

	responder := &countingResponder{configured: true, result: marketplace.RelayDeliveryResult{
		Permanent: true, ErrorCode: "assignment_already_resolved",
	}}
	worker := marketplace.NewRelayWorker(repo, responder, marketplace.RelayWorkerConfig{
		BaseBackoff: time.Millisecond,
	})
	if _, err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	stored, _ := repo.GetRelayByIdempotencyKey(ctx, nil,
		marketplace.RelayProviderAssignmentAccept, "relay-rej")
	if stored.Status != marketplace.RelayRejected {
		t.Fatalf("status = %s, want REJECTED", stored.Status)
	}
	if !stored.Terminal() {
		t.Fatal("a permanent rejection must be terminal")
	}
	// No further delivery attempt is made.
	if _, err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(responder.deliveries()) != 1 {
		t.Fatalf("deliveries = %d, want 1; a permanent rejection must not retry",
			len(responder.deliveries()))
	}
}

func TestRelayLeaseRecoveryDeadLetterAndReplay(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	providerID := seedProvider(t, pool, "+2348030000005")
	op := seedRelay(t, pool, providerID, "asg_relay_lease", "relay-lease", "fp-lease")

	// Claim, then simulate the worker dying.
	if err := repo.WithinTx(ctx, func(tx pgx.Tx) error {
		_, err := repo.ClaimRelays(ctx, tx, "worker-a", time.Minute, 10)
		return err
	}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE dispatch_fleet_relay_operations
		SET lease_expires_at = now() - interval '1 minute' WHERE id = $1`, op.ID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	recovered, err := repo.RecoverExpiredRelayLeases(ctx, nil)
	if err != nil || recovered != 1 {
		t.Fatalf("recovered = %d, %v; want 1", recovered, err)
	}
	// Recovery lands in UNKNOWN_OUTCOME, not PENDING: the dead worker may have
	// already reached fleet, so the next pass must not assume it did not.
	reloaded, _ := repo.GetRelayByID(ctx, nil, op.ID)
	if reloaded.Status != marketplace.RelayUnknownOutcome {
		t.Fatalf("recovered status = %s, want UNKNOWN_OUTCOME", reloaded.Status)
	}

	// An attempt budget must not erase uncertainty.
	for i := 0; i < 3; i++ {
		if err := repo.MarkRelayUnknown(ctx, nil, op.ID, "fleet_service_unavailable", 0, 3); err != nil {
			t.Fatalf("mark unknown: %v", err)
		}
	}
	dead, _ := repo.GetRelayByID(ctx, nil, op.ID)
	if dead.Status != marketplace.RelayUnknownOutcome {
		t.Fatalf("status = %s, want UNKNOWN_OUTCOME", dead.Status)
	}

	// Neither UNKNOWN_OUTCOME nor a legacy dead-letter fence may be replayed.
	if err := repo.ReplayRelay(ctx, op.ID); !errors.Is(err, marketplace.ErrNotFound) {
		t.Fatalf("replay of live relay = %v, want ErrNotFound", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE dispatch_fleet_relay_operations SET status='DEAD_LETTER' WHERE id=$1`, op.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplayRelay(ctx, op.ID); !errors.Is(err, marketplace.ErrNotFound) {
		t.Fatalf("replay of fenced dead letter = %v, want ErrNotFound", err)
	}
}

func TestConcurrentRelayWorkersDeliverEachDecisionOnce(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	providerID := seedProvider(t, pool, "+2348030000006")

	// Five distinct provider decisions.
	for i := 0; i < 5; i++ {
		seedRelay(t, pool, providerID, "asg_conc_"+string(rune('a'+i)),
			"relay-conc-"+string(rune('a'+i)), "fp-conc")
	}

	responder := &countingResponder{configured: true,
		result: marketplace.RelayDeliveryResult{Accepted: true}}

	// Four workers race. FOR UPDATE SKIP LOCKED must ensure each decision is
	// delivered exactly once, never twice.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			worker := marketplace.NewRelayWorker(repo, responder, marketplace.RelayWorkerConfig{
				Owner: "worker-" + string(rune('a'+slot)), BaseBackoff: time.Millisecond,
			})
			_, _ = worker.RunOnce(ctx)
		}(i)
	}
	wg.Wait()

	deliveries := responder.deliveries()
	seen := map[string]int{}
	for _, key := range deliveries {
		seen[key]++
	}
	for key, count := range seen {
		if count != 1 {
			t.Fatalf("decision %s delivered %d times; each must be delivered once", key, count)
		}
	}

	var confirmed int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_fleet_relay_operations WHERE status = 'CONFIRMED'`).
		Scan(&confirmed)
	if confirmed != 5 {
		t.Fatalf("confirmed = %d, want 5", confirmed)
	}
}

func TestRelayWorkerFailsClosedWhenResponderUnconfigured(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	providerID := seedProvider(t, pool, "+2348030000007")
	seedRelay(t, pool, providerID, "asg_relay_off", "relay-off", "fp-off")

	// nil responder => UnconfiguredResponder => never reports success.
	worker := marketplace.NewRelayWorker(repo, nil, marketplace.RelayWorkerConfig{
		BaseBackoff: time.Millisecond,
	})
	if _, err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	stored, _ := repo.GetRelayByIdempotencyKey(ctx, nil,
		marketplace.RelayProviderAssignmentAccept, "relay-off")
	if stored.Status == marketplace.RelayConfirmed {
		t.Fatal("an unconfigured responder must never confirm a decision")
	}
	// The decision stays durably queued rather than being discarded.
	if stored.Terminal() {
		t.Fatalf("status = %s; the decision must remain recoverable", stored.Status)
	}
}

func strPtr(value string) *string { return &value }

// TestConcurrentAcceptAndDeclineUnderOneClientResponseID proves the database,
// not application logic, is the arbiter.
//
// Two goroutines submit contradictory decisions under one clientResponseId at
// the same instant. Check-then-insert alone cannot prevent this: both can read
// "no conflict" before either inserts.
func TestConcurrentAcceptAndDeclineUnderOneClientResponseID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	providerID := seedProvider(t, pool, "+2348050000001")

	const key = "same-client-response-id"
	assignmentID := "asg-contradiction"

	build := func(operationType string, decision string) *marketplace.RelayOperation {
		return &marketplace.RelayOperation{
			OperationType: operationType, ProviderID: providerID,
			AssignmentID: &assignmentID, IdempotencyKey: key,
			RequestFingerprint: "fp-" + decision,
			Payload:            []byte(`{"decision":"` + decision + `"}`),
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	operations := []*marketplace.RelayOperation{
		build(marketplace.RelayProviderAssignmentAccept, "accept"),
		build(marketplace.RelayProviderAssignmentDecline, "decline"),
	}
	for index := range operations {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			errs[slot] = repo.WithinTx(ctx, func(tx pgx.Tx) error {
				_, _, createErr := repo.CreateRelay(ctx, tx, operations[slot])
				return createErr
			})
		}(index)
	}
	wg.Wait()

	winners, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, marketplace.ErrIdempotencyConflict):
			conflicts++
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1 (errs=%v)", winners, errs)
	}
	if conflicts != 1 {
		t.Fatalf("conflicts = %d, want exactly 1 (errs=%v)", conflicts, errs)
	}

	// Exactly one relay, so only one decision can ever be delivered.
	var rows int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_fleet_relay_operations WHERE idempotency_key = $1`,
		key).Scan(&rows)
	if rows != 1 {
		t.Fatalf("relay rows = %d; contradictory decisions were both stored", rows)
	}

	// And it is one decision, not a merge of both.
	var operationType string
	_ = pool.QueryRow(ctx,
		`SELECT operation_type FROM dispatch_fleet_relay_operations WHERE idempotency_key = $1`,
		key).Scan(&operationType)
	if operationType != marketplace.RelayProviderAssignmentAccept &&
		operationType != marketplace.RelayProviderAssignmentDecline {
		t.Fatalf("stored operation type = %q", operationType)
	}
}

// TestDifferentProvidersMayShareAClientResponseID proves the idempotency
// namespace is per provider.
//
// clientResponseId is generated by a provider's own device. Two providers
// independently choosing the same value is an ordinary collision, not a
// contradiction; a global namespace would let one provider's decision block
// another's entirely unrelated one.
func TestDifferentProvidersMayShareAClientResponseID(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	providerA := seedProvider(t, pool, "+2348060000001")
	providerB := seedProvider(t, pool, "+2348060000002")

	const sharedKey = "client-response-1"
	build := func(providerID string, assignmentID string) *marketplace.RelayOperation {
		return &marketplace.RelayOperation{
			OperationType: marketplace.RelayProviderAssignmentAccept,
			ProviderID:    providerID, AssignmentID: &assignmentID,
			IdempotencyKey: sharedKey, RequestFingerprint: "fp-" + providerID,
			Payload: []byte(`{"decision":"accept"}`),
		}
	}

	for _, spec := range []struct{ provider, assignment string }{
		{providerA, "asg-a"}, {providerB, "asg-b"},
	} {
		if err := repo.WithinTx(ctx, func(tx pgx.Tx) error {
			_, _, createErr := repo.CreateRelay(ctx, tx, build(spec.provider, spec.assignment))
			return createErr
		}); err != nil {
			t.Fatalf("provider %s could not use the shared key: %v", spec.provider, err)
		}
	}

	var rows int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_fleet_relay_operations WHERE idempotency_key = $1`,
		sharedKey).Scan(&rows)
	if rows != 2 {
		t.Fatalf("relay rows = %d, want 2; the namespace must be per provider", rows)
	}

	// Each provider's own replay still resolves to their own relay.
	first, err := repo.GetRelayByKeyAnyType(ctx, nil, providerA, sharedKey)
	if err != nil {
		t.Fatalf("provider A lookup: %v", err)
	}
	second, err := repo.GetRelayByKeyAnyType(ctx, nil, providerB, sharedKey)
	if err != nil {
		t.Fatalf("provider B lookup: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("two providers resolved to the same relay")
	}
	if *first.AssignmentID != "asg-a" || *second.AssignmentID != "asg-b" {
		t.Fatalf("relays crossed providers: A=%s B=%s", *first.AssignmentID, *second.AssignmentID)
	}
}
