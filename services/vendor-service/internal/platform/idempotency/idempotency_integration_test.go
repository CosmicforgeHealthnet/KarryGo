package idempotency

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = database.ApplyMigrations(context.Background(), pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestStoreConcurrentClaimTenantIsolationAndRestartReplay(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()
	scope, endpoint, key := "tenant-a-"+suffix, "/orders", "key-"+suffix
	fingerprint := Fingerprint("POST", endpoint, []byte(`{"amount":10,"branch":"b1"}`))
	store := NewStore(pool, time.Hour)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.Begin(ctx, scope, endpoint, key, fingerprint)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	winners, inProgress := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrInProgress):
			inProgress++
		default:
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if winners != 1 || inProgress != 1 {
		t.Fatalf("winners=%d inProgress=%d", winners, inProgress)
	}
	if err := store.Complete(ctx, scope, endpoint, key, 201, []byte(`{"operation_ref":"op_safe"}`)); err != nil {
		t.Fatal(err)
	}
	var actorRef, operationType, resourceTarget string
	if err := pool.QueryRow(ctx, `SELECT actor_ref,operation_type,resource_target
		FROM vendor_idempotency_keys WHERE scope_id=$1 AND endpoint=$2 AND idempotency_key=$3`,
		scope, endpoint, key).Scan(&actorRef, &operationType, &resourceTarget); err != nil {
		t.Fatal(err)
	}
	if actorRef != scope || operationType != endpoint || resourceTarget != endpoint {
		t.Fatalf("metadata actor=%q operation=%q resource=%q", actorRef, operationType, resourceTarget)
	}
	restarted := NewStore(pool, time.Hour)
	replay, err := restarted.Begin(ctx, scope, endpoint, key, fingerprint)
	if err != nil || replay == nil || replay.ResponseStatus != 201 || string(replay.ResponseBody) != `{"operation_ref":"op_safe"}` {
		t.Fatalf("restart replay=%+v err=%v", replay, err)
	}
	if _, err = restarted.Begin(ctx, scope, endpoint, key, Fingerprint("POST", endpoint, []byte(`{"amount":11}`))); !errors.Is(err, ErrFingerprintMismatch) {
		t.Fatalf("different body err=%v", err)
	}
	if record, err := restarted.Begin(ctx, "tenant-b-"+suffix, endpoint, key, fingerprint); err != nil || record != nil {
		t.Fatalf("unrelated tenant collided: record=%+v err=%v", record, err)
	}
}

func TestExpiredKeyReclaimHasOneWinner(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	suffix := uuid.NewString()
	scope, endpoint, key := "tenant-expired-"+suffix, "/checkout", "key-"+suffix
	_, err := pool.Exec(ctx, `INSERT INTO vendor_idempotency_keys
		(scope_id,endpoint,idempotency_key,request_fingerprint,status,expires_at)
		VALUES($1,$2,$3,'old','COMPLETED',now()-interval '1 second')`, scope, endpoint, key)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool, time.Hour)
	fingerprint := Fingerprint("POST", endpoint, []byte(`{"quote":"q1"}`))
	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() { <-start; _, claimErr := store.Begin(ctx, scope, endpoint, key, fingerprint); errs <- claimErr }()
	}
	close(start)
	winners := 0
	for range 2 {
		if claimErr := <-errs; claimErr == nil {
			winners++
		} else if !errors.Is(claimErr, ErrInProgress) {
			t.Fatalf("unexpected reclaim error: %v", claimErr)
		}
	}
	if winners != 1 {
		t.Fatalf("expired reclaim winners=%d want 1", winners)
	}
}
