package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/referral-service/internal/database"
	"cosmicforge/logistics/services/referral-service/internal/events"
	referralusecases "cosmicforge/logistics/services/referral-service/internal/features/referrals/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestMigrationsReplayAndRollback(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := database.MigrationsCurrent(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("not current after apply: %v", err)
	}
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := database.MigrationsCurrent(ctx, pool, migrationsDir); err == nil {
		t.Fatal("reported current after rollback")
	}
	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("re-apply after rollback: %v", err)
	}
}

// Step 2 "done when": one paid job is one activity record for the customer
// and one for the driver.
func TestJobSettledRecordsActivityForBothSides(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	customer, driver := "cust_"+uuid.NewString(), "drv_"+uuid.NewString()
	job := jobSettled(customer, driver, time.Now().UTC().Add(-time.Hour))

	outcome := mustHandle(t, svc, referralusecases.ProducerWallet,
		envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job))
	if outcome != referralusecases.OutcomeProcessed {
		t.Fatalf("outcome = %s", outcome)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM actor_activity_jobs WHERE actor_type = 'customer' AND actor_id = $1`, customer); n != 1 {
		t.Fatalf("customer activity rows = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM actor_activity_jobs WHERE actor_type = 'taxi' AND actor_id = $1`, driver); n != 1 {
		t.Fatalf("driver activity rows = %d, want 1", n)
	}
	count, err := svc.ActivityCount(context.Background(), "taxi", driver, time.Time{})
	if err != nil || count != 1 {
		t.Fatalf("ActivityCount = %d, %v; want 1", count, err)
	}
}

// Nothing is applied twice: not a redelivered event, and not the same job
// arriving under a brand-new event id (the case the inbox alone cannot catch).
func TestDuplicateDeliveryAndReplayedJobHaveNoEffect(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	job := jobSettled("cust_"+uuid.NewString(), "drv_"+uuid.NewString(), time.Now().UTC())
	raw := envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job)

	mustHandle(t, svc, referralusecases.ProducerWallet, raw)
	if outcome := mustHandle(t, svc, referralusecases.ProducerWallet, raw); outcome != referralusecases.OutcomeDuplicate {
		t.Fatalf("redelivery outcome = %s, want duplicate", outcome)
	}
	fresh := envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job)
	if outcome := mustHandle(t, svc, referralusecases.ProducerWallet, fresh); outcome != referralusecases.OutcomeDuplicate {
		t.Fatalf("same job under a new event id: outcome = %s, want duplicate", outcome)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM actor_activity_jobs`); n != 2 {
		t.Fatalf("activity rows = %d, want 2 (one per side, once)", n)
	}
}

// Vendor shop orders are recorded but have no referral effect.
func TestVendorOrderIsIgnored(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	job := jobSettled("cust_"+uuid.NewString(), "biz_"+uuid.NewString(), time.Now().UTC())
	job.ProviderType = "vendor_business"
	job.PlatformFeeKobo = 0
	outcome := mustHandle(t, svc, referralusecases.ProducerWallet,
		envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job))
	if outcome != referralusecases.OutcomeIgnored {
		t.Fatalf("outcome = %s, want ignored", outcome)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM actor_activity_jobs`); n != 0 {
		t.Fatalf("vendor order created %d activity rows", n)
	}
}

// A malformed event is acknowledged (so the producer stops retrying) and
// kept as evidence, but never applied.
func TestMalformedEventIsRejectedAndAcknowledged(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	job := jobSettled("cust_"+uuid.NewString(), "drv_"+uuid.NewString(), time.Now().UTC())
	job.PlatformFeeKobo = job.AmountKobo + 1 // impossible commission
	raw := envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job)
	if outcome := mustHandle(t, svc, referralusecases.ProducerWallet, raw); outcome != referralusecases.OutcomeRejected {
		t.Fatalf("outcome = %s, want rejected", outcome)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM referral_inbox WHERE status = 'REJECTED' AND error_code = 'invalid_job'`); n != 1 {
		t.Fatalf("rejected inbox rows = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM settled_jobs`); n != 0 {
		t.Fatal("a rejected event must not be applied")
	}
	if outcome := mustHandle(t, svc, referralusecases.ProducerWallet, raw); outcome != referralusecases.OutcomeDuplicate {
		t.Fatalf("redelivered rejected event outcome = %s, want duplicate", outcome)
	}
}

// Only the wallet may report paid jobs; a service cannot sign as one
// producer and claim to be another.
func TestProducerMustOwnTheEventType(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	job := jobSettled("cust_"+uuid.NewString(), "drv_"+uuid.NewString(), time.Now().UTC())

	forged := envelopeJSON(t, referralusecases.ProducerCustomer, referralcontract.EventJobSettled, job.PaymentIntentID, job)
	if _, err := svc.HandleEvent(context.Background(), referralusecases.ProducerCustomer, forged); apperrors.From(err).Code != apperrors.CodeForbidden {
		t.Fatalf("customer-service reporting a paid job: err = %v, want forbidden", err)
	}
	spoofed := envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job)
	if _, err := svc.HandleEvent(context.Background(), referralusecases.ProducerHauling, spoofed); apperrors.From(err).Code != apperrors.CodeForbidden {
		t.Fatalf("envelope producer differing from the signer: err = %v, want forbidden", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM settled_jobs`); n != 0 {
		t.Fatal("a forged event was applied")
	}
}

func TestPayloadMismatchIsAConflict(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	job := jobSettled("cust_"+uuid.NewString(), "drv_"+uuid.NewString(), time.Now().UTC())
	raw := envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job)
	mustHandle(t, svc, referralusecases.ProducerWallet, raw)

	var envelope map[string]any
	_ = json.Unmarshal(raw, &envelope)
	envelope["occurredAt"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	tampered, _ := json.Marshal(envelope)
	if _, err := svc.HandleEvent(context.Background(), referralusecases.ProducerWallet, tampered); apperrors.From(err).Code != apperrors.CodeConflict {
		t.Fatalf("same event id, different body: err = %v, want conflict", err)
	}
}

// Two copies arriving at once -- by redelivery or replay -- still apply once.
func TestConcurrentDuplicateDeliveries(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	job := jobSettled("cust_"+uuid.NewString(), "drv_"+uuid.NewString(), time.Now().UTC())
	sameEvent := envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job)

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		raw := sameEvent
		if i%2 == 1 {
			raw = envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job)
		}
		wg.Add(1)
		go func(raw []byte) {
			defer wg.Done()
			if _, err := svc.HandleEvent(context.Background(), referralusecases.ProducerWallet, raw); err != nil {
				errs <- err
			}
		}(raw)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		// A losing concurrent insert may surface as retryable; the relay
		// redelivers it. It must never double-apply.
		if apperrors.From(err).Code != apperrors.CodeUnavailable {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if n := countRows(t, pool, `SELECT count(*) FROM actor_activity_jobs`); n != 2 {
		t.Fatalf("activity rows = %d, want exactly 2", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM settled_jobs`); n != 1 {
		t.Fatalf("settled jobs = %d, want exactly 1", n)
	}
}

// The inbox record commits or rolls back with the business effect. (The
// vendor-service inbox commits separately -- this pins that we did not copy
// that flaw.)
func TestInboxRollsBackWithTheTransaction(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	inbox := events.NewInbox(pool)
	err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		if err := inbox.Begin(ctx, tx, "evt_rollback", "x.v1", "payment-wallet-service", []byte(`{}`)); err != nil {
			return err
		}
		return errors.New("business effect failed")
	})
	if err == nil {
		t.Fatal("expected the transaction to fail")
	}
	if n := countRows(t, pool, `SELECT count(*) FROM referral_inbox WHERE event_id = 'evt_rollback'`); n != 0 {
		t.Fatal("inbox row survived a rolled-back transaction")
	}
}

// The window is (asOf-30d, asOf]: exactly thirty days back is out, a second
// later is in, and anything after asOf is invisible.
func TestActivityWindowBoundaries(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	ctx := context.Background()
	driver := "drv_" + uuid.NewString()
	asOf := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	window := 30 * 24 * time.Hour
	for _, at := range []time.Time{
		asOf.Add(-window),               // excluded: boundary
		asOf.Add(-window + time.Second), // included
		asOf,                            // included: inclusive upper bound
		asOf.Add(time.Second),           // excluded: after asOf
	} {
		job := jobSettled("cust_"+uuid.NewString(), driver, at)
		mustHandle(t, svc, referralusecases.ProducerWallet,
			envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job))
	}
	count, err := svc.ActivityCount(ctx, "taxi", driver, asOf)
	if err != nil || count != 2 {
		t.Fatalf("ActivityCount = %d, %v; want 2", count, err)
	}
}

// The activity check must be able to use its partial index; a sequential
// scan here grows with every job ever done.
func TestActivityQueryUsesThePartialIndex(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SET enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(ctx, `RESET enable_seqscan`) //nolint:errcheck
	rows, err := conn.Query(ctx, `EXPLAIN
		SELECT count(*) FROM (
			SELECT 1 FROM actor_activity_jobs
			WHERE actor_type = $1 AND actor_id = $2 AND reversed_at IS NULL
			  AND job_completed_at > $3 AND job_completed_at <= $4
			LIMIT $5
		) recent`, "taxi", "drv", time.Now().Add(-time.Hour), time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	for rows.Next() {
		var line string
		_ = rows.Scan(&line)
		plan.WriteString(line + "\n")
	}
	rows.Close()
	if !strings.Contains(plan.String(), "actor_activity_recent_idx") {
		t.Fatalf("activity query does not use actor_activity_recent_idx:\n%s", plan.String())
	}
}

// End to end through the real HTTP stack: signature, caller allowlist and
// single-use nonces.
func TestSignedIngestEndToEnd(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	walletSecret := []byte("wallet-secret")
	router := signedRouter(t, pool, svc, serviceauth.Secrets{
		referralusecases.ProducerWallet: walletSecret,
		"media-file-service":            []byte("media-secret"),
	})
	job := jobSettled("cust_"+uuid.NewString(), "drv_"+uuid.NewString(), time.Now().UTC())
	body := envelopeJSON(t, referralusecases.ProducerWallet, referralcontract.EventJobSettled, job.PaymentIntentID, job)

	req := signedRequest(t, http.MethodPost, referralcontract.IngestPath, referralusecases.ProducerWallet, walletSecret, body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"processed"`) {
		t.Fatalf("delivery: %d %s", rec.Code, rec.Body.String())
	}

	// Replaying the identical signed request (same nonce) is refused.
	replay := httptest.NewRequest(http.MethodPost, referralcontract.IngestPath, strings.NewReader(string(body)))
	replay.Header = req.Header.Clone()
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, replay)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("replayed request: %d, want 401", rec.Code)
	}

	// A service with a valid secret but no business sending events is refused.
	other := signedRequest(t, http.MethodPost, referralcontract.IngestPath, "media-file-service", []byte("media-secret"), body)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, other)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-producer caller: %d, want 403", rec.Code)
	}
}
