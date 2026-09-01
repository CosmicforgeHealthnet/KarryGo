package migrations_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/assignment"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/deliveryrecovery"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/securehandover"
	tripfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/trip"
)

func TestReliabilityReconciliationMigrationShape(t *testing.T) {
	up, err := os.ReadFile("000050_dispatch_reliability_reconciliation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("000050_dispatch_reliability_reconciliation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, required := range []string{
		"wallet_attempt_count", "wallet_lease_expires_at", "notification_attempt_count",
		"notification_lease_expires_at", "assignment_attempt_count", "assignment_lease_expires_at",
		"dispatch_outbox_payload_is_safe", "dispatch_marketplace_outbox_secret_safe",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToLower(text), "create table dispatch_marketplace_outbox") {
		t.Fatal("reliability migration created a second outbox")
	}
	for _, required := range []string{"DROP FUNCTION IF EXISTS dispatch_outbox_payload_is_safe", "DROP COLUMN IF EXISTS assignment_state", "DROP COLUMN IF EXISTS notification_attempt_count", "DROP COLUMN IF EXISTS wallet_attempt_count"} {
		if !strings.Contains(string(down), required) {
			t.Fatalf("down migration missing %q", required)
		}
	}
}

func TestSettlementReconciliationConcurrentClaimAndLateWorker(t *testing.T) {
	_, pool, f, cleanup := seedHandoverFixture(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE trips SET status='completed',completed_at=now() WHERE id=$1`, f.trip); err != nil {
		t.Fatal(err)
	}
	repo := tripfeature.NewPostgresRepository(pool)
	policy := tripfeature.SettlementRetryPolicy{Lease: time.Minute, MaxAttempts: 10}
	var wg sync.WaitGroup
	counts := make(chan int, 2)
	owners := make(chan string, 2)
	errs := make(chan error, 2)
	for _, owner := range []string{"a", "b"} {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			items, err := repo.ClaimSettlements(ctx, time.Now(), owner, policy)
			counts <- len(items)
			errs <- err
			if len(items) == 1 {
				owners <- owner
			}
		}(owner)
	}
	wg.Wait()
	close(counts)
	close(owners)
	close(errs)
	n := 0
	for count := range counts {
		n += count
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n != 1 {
		t.Fatalf("concurrent claims=%d", n)
	}
	old := <-owners
	if _, err := pool.Exec(ctx, `UPDATE trips SET wallet_settlement_lease_expires_at=now()-interval '1 second' WHERE id=$1`, f.trip); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ClaimSettlements(ctx, time.Now(), "replacement", policy)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	if err = repo.FinishSettlement(ctx, f.trip, "replacement", true, time.Now(), time.Second, 10); err != nil {
		t.Fatal(err)
	}
	if err = repo.FinishSettlement(ctx, f.trip, old, false, time.Now(), time.Second, 10); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = pool.QueryRow(ctx, `SELECT wallet_settlement_state FROM trips WHERE id=$1`, f.trip).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "ACKNOWLEDGED" {
		t.Fatal("late worker regressed settlement", state)
	}
}

func TestOutboxReconciliationLeaseFencesLateFailure(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	repo := marketplace.NewRepository(pool)
	if err := repo.PublishEventTo(ctx, pool, "notification-service", "test.safe.v1", "test", "opaque", 1, "", "", map[string]any{"status": "READY"}); err != nil {
		t.Fatal(err)
	}
	first, err := repo.ClaimOutboxForTarget(ctx, pool, "worker", "notification-service", time.Minute, 1)
	if err != nil || len(first) != 1 {
		t.Fatal(first, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE dispatch_marketplace_outbox SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, first[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.RecoverExpiredLeases(ctx, pool); err != nil {
		t.Fatal(err)
	}
	second, err := repo.ClaimOutboxForTarget(ctx, pool, "worker", "notification-service", time.Minute, 1)
	if err != nil || len(second) != 1 || first[0].LeaseOwner == second[0].LeaseOwner {
		t.Fatal("lease fencing identity not advanced", err)
	}
	if err = repo.MarkPublished(ctx, pool, second[0].ID, second[0].LeaseOwner); err != nil {
		t.Fatal(err)
	}
	if err = repo.MarkDeliveryFailed(ctx, pool, first[0].ID, "LATE_TIMEOUT", time.Second, 10, first[0].LeaseOwner); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = pool.QueryRow(ctx, `SELECT status FROM dispatch_marketplace_outbox WHERE id=$1`, first[0].ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "PUBLISHED" {
		t.Fatalf("stale worker regressed acknowledged event: %s", state)
	}
}

type failingSecretTransport struct{}

func (failingSecretTransport) Deliver(context.Context, securehandover.SecretDeliveryIntent) (string, string, []string, error) {
	return "", "FAILED", []string{"in_app"}, errors.New("response lost")
}

func TestHandoverNotificationReconciliationClaimRace(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	provider, customer, booking, trip := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO providers(id,phone,verification_status,is_active,onboarding_complete) VALUES($1,$2,'verified',true,true)`, provider, "+23483"+provider[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,$2,'accepted','sender',6.5,3.4,'receiver',6.6,3.5,1500,'receiver','+2348011111111')`, booking, customer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO trips(id,booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,$2,$3,$4,'in_progress','sender',6.5,3.4,'receiver',6.6,3.5,1500,'receiver','+2348011111111')`, trip, booking, provider, customer); err != nil {
		t.Fatal(err)
	}
	repo, err := securehandover.NewRepository(pool, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := securehandover.NewService(repo, []byte("0123456789abcdef0123456789abcdef"), securehandover.Policy{Enabled: true, InAppAllowed: true})
	if err != nil {
		t.Fatal(err)
	}
	service.WithRateLimiter(allowHandoverLimiter{}).WithNotifier(failingSecretTransport{})
	if _, err = service.IssueDelivery(ctx, trip, "notification-race"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE dispatch_handover_challenges SET notification_next_attempt_at=now()`); err != nil {
		t.Fatal(err)
	}
	counts := make(chan int, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rows, e := repo.ClaimPendingNotifications(ctx, time.Now().UTC(), "notify-worker-"+string(rune('a'+n)), time.Minute, 10, 10)
			counts <- len(rows)
			errs <- e
		}(i)
	}
	wg.Wait()
	close(counts)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	total := 0
	for n := range counts {
		total += n
	}
	if total != 1 {
		t.Fatalf("claimed=%d want 1", total)
	}
}

func TestQueueWalletReconciliationClaimRace(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	customer, booking, group, mission := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,$2,'unmatched','sender',6.5,3.4,'receiver',6.6,3.5,1500,'receiver','+2348011111111')`, booking, customer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_delivery_groups(id,customer_id,source_type,source_ref,source_version,booking_id,state) VALUES($1,$2,'CUSTOMER',$3,1,$4,'READY_FOR_DISPATCH')`, group, customer, "queue:"+booking, booking); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_delivery_missions(id,delivery_group_id,source_type,source_ref,source_version,state,booking_id) VALUES($1,$2,'CUSTOMER',$3,1,'PLANNED',$4)`, mission, group, "queue:"+booking, booking); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_queue_entries(mission_id,status,expires_at,idempotency_key,request_fingerprint,wallet_operation_ref,wallet_state) VALUES($1,'EXPIRED',now()+interval '1 hour',$2,'fp',$3,'REQUESTED')`, mission, "queue-op:"+mission, "wallet-op:"+mission); err != nil {
		t.Fatal(err)
	}
	repo := assignment.NewRepository(pool)
	counts := make(chan int, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rows, e := repo.ClaimPendingQueueFinancial(ctx, time.Now().UTC(), "worker-"+string(rune('a'+n)), time.Minute, 10, 10)
			counts <- len(rows)
			errs <- e
		}(i)
	}
	wg.Wait()
	close(counts)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	total := 0
	for n := range counts {
		total += n
	}
	if total != 1 {
		t.Fatalf("claimed=%d want 1", total)
	}
}

func TestScheduledRecoveryActivationRace(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	provider, customer, booking, trip := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO providers(id,phone,verification_status,is_active,onboarding_complete) VALUES($1,$2,'verified',true,true)`, provider, "+23482"+provider[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,$2,'accepted','sender',6.5,3.4,'receiver',6.6,3.5,1500,'receiver','+2348011111111')`, booking, customer); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO trips(id,booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,$2,$3,$4,'in_progress','sender',6.5,3.4,'receiver',6.6,3.5,1500,'receiver','+2348011111111')`, trip, booking, provider, customer); err != nil {
		t.Fatal(err)
	}
	repo := deliveryrecovery.NewRepository(pool)
	service := deliveryrecovery.New(repo, deliveryrecovery.Policy{Enabled: true, ReturnEnabled: true, GracePeriod: time.Millisecond})
	attempt, err := service.StartAttempt(ctx, deliveryrecovery.StartAttemptCommand{TripID: trip, ProviderID: provider, IdempotencyKey: "scheduled-start"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.RecordContact(ctx, deliveryrecovery.ContactCommand{AttemptID: attempt.ID, ProviderID: provider, Method: "PHONE", Outcome: "UNREACHABLE", IdempotencyKey: "scheduled-contact"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err = service.FailAttempt(ctx, deliveryrecovery.FailAttemptCommand{AttemptID: attempt.ID, ProviderID: provider, ReasonCode: "RECEIVER_NO_ANSWER", IdempotencyKey: "scheduled-fail"}); err != nil {
		t.Fatal(err)
	}
	when := time.Now().UTC().Add(20 * time.Millisecond)
	if _, err = service.SelectRecovery(ctx, deliveryrecovery.SelectRecoveryCommand{BookingID: booking, CustomerID: customer, Action: deliveryrecovery.RetrySameDay, ScheduledFor: &when, ExpectedTripVersion: 1, IdempotencyKey: "scheduled-select"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := repo.ActivateScheduled(ctx, time.Now().UTC(), 10); errCh <- e }()
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		if e != nil {
			t.Fatal(e)
		}
	}
	var state string
	if err = pool.QueryRow(ctx, `SELECT state FROM dispatch_delivery_recovery_actions WHERE idempotency_key='scheduled-select'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "READY" {
		t.Fatalf("state=%s", state)
	}
	var events int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_marketplace_outbox WHERE event_type='dispatch.delivery_recovery.activated.v1' AND payload->>'trip_id'=$1`, trip).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("activation events=%d", events)
	}
	next, err := service.StartAttempt(ctx, deliveryrecovery.StartAttemptCommand{TripID: trip, ProviderID: provider, IdempotencyKey: "scheduled-next"})
	if err != nil || next.AttemptNumber != 2 {
		t.Fatalf("next arrival attempt=%+v err=%v", next, err)
	}
}

func TestReliabilityMigrationApplyRollbackReapplyAndSecretGuard(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	down, err := os.ReadFile("000050_dispatch_reliability_reconciliation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("000050_dispatch_reliability_reconciliation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("reapply: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service) VALUES('test.safe.v1','test','safe',1,'{"status":"READY"}','hash','vendor-service')`); err != nil {
		t.Fatalf("safe event rejected: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service) VALUES('test.secret.v1','test','secret',1,'{"nested":{"otp":"123456"}}','hash','vendor-service')`); err == nil {
		t.Fatal("secret-bearing generic event was accepted")
	}
}
