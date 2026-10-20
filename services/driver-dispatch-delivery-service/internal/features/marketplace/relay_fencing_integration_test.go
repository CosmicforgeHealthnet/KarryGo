package marketplace_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
)

func TestFencedLegacyDecisionCannotBeRecreatedWithFreshKey(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	provider := seedProvider(t, pool, "+2348030070003")
	original := seedRelay(t, pool, provider, "asg-legacy-fenced", "old-key", "old-fp")
	if _, err := pool.Exec(ctx, `UPDATE dispatch_fleet_relay_operations SET status='DEAD_LETTER',readback_required=true WHERE id=$1`, original.ID); err != nil {
		t.Fatal(err)
	}
	err := repo.WithinTx(ctx, func(tx pgx.Tx) error {
		_, _, err := repo.CreateRelay(ctx, tx, &marketplace.RelayOperation{OperationType: marketplace.RelayProviderAssignmentAccept, ProviderID: provider, AssignmentID: original.AssignmentID, IdempotencyKey: "fresh-key", RequestFingerprint: "fresh-fp", Payload: []byte(`{"decision":"accept"}`)})
		return err
	})
	if err == nil || apperrors.From(err).Status != 409 {
		t.Fatalf("new identity bypassed uncertainty fence: %v", err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("decision rows=%d err=%v", count, err)
	}
	stored, err := repo.GetRelayByID(ctx, nil, original.ID)
	if err != nil || !stored.NeedsReadBack() {
		t.Fatalf("legacy uncertainty hidden: %+v %v", stored, err)
	}
}

func TestImmediateRelayClaimFencesDuplicateRequestsAndWorker(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	provider := seedProvider(t, pool, "+2348030070001")
	op := seedRelay(t, pool, provider, "asg-fenced", "relay-fenced", "fp-fenced")
	var wg sync.WaitGroup
	var wins atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.ClaimRelayForImmediateDelivery(ctx, op.ID)
			if err != nil {
				t.Error(err)
			}
			if ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("immediate delivery claims=%d", wins.Load())
	}
	responder := &countingResponder{configured: true, result: marketplace.RelayDeliveryResult{Accepted: true}}
	worker := marketplace.NewRelayWorker(repo, responder, marketplace.RelayWorkerConfig{})
	result, err := worker.RunOnce(ctx)
	if err != nil || result.Claimed != 0 || len(responder.deliveries()) != 0 {
		t.Fatalf("worker raced immediate delivery: %+v %v", result, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE dispatch_fleet_relay_operations SET lease_expires_at=now()-interval '1 minute' WHERE id=$1`, op.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = worker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetRelayByID(ctx, nil, op.ID)
	if err != nil || stored.Status != marketplace.RelayUnknownOutcome || len(responder.deliveries()) != 0 {
		t.Fatalf("crashed immediate delivery replayed: %+v %v", stored, err)
	}
}

func TestImmediateDeliveryPersistenceFailureStaysPending(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	provider := seedProvider(t, pool, "+2348030070002")
	if _, err := pool.Exec(ctx, `CREATE FUNCTION test_relay_confirmation_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test persistence fault'; END $$;
 CREATE TRIGGER test_relay_confirmation_failure BEFORE UPDATE ON dispatch_fleet_relay_operations FOR EACH ROW WHEN (NEW.status='CONFIRMED') EXECUTE FUNCTION test_relay_confirmation_failure()`); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DROP TRIGGER test_relay_confirmation_failure ON dispatch_fleet_relay_operations; DROP FUNCTION test_relay_confirmation_failure()`)
	responder := &scriptedResponder{configured: true, result: marketplace.RelayDeliveryResult{Accepted: true}}
	server := newProviderServer(t, pool, responder, providerSession(provider, "session-fence", authmodels.RoleDispatchProvider), providerFlags())
	for i := 0; i < 2; i++ {
		status, body := providerCall(t, server, http.MethodPost, "/api/v1/provider/company-assignments/asg-fault/accept", map[string]any{"clientResponseId": "c-fault"})
		data, _ := body["data"].(map[string]any)
		if status != 200 || data["status"] != marketplace.ProviderActionPending {
			t.Fatalf("failed durable confirmation reported success: %d %+v", status, body)
		}
	}
	if len(responder.seen) != 1 {
		t.Fatalf("duplicate request redelivered uncertain decision %d times", len(responder.seen))
	}
	var state string
	var expiry time.Time
	if err := pool.QueryRow(ctx, `SELECT status,lease_expires_at FROM dispatch_fleet_relay_operations WHERE idempotency_key='c-fault'`).Scan(&state, &expiry); err != nil {
		t.Fatal(err)
	}
	if state != marketplace.RelayDelivering || expiry.IsZero() {
		t.Fatalf("missing crash recovery lease: %s %v", state, expiry)
	}
}
