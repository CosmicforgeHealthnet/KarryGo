package recovery

// Section 17 gap closure: the durable payment rebind obligation, against a
// real database and a fake Wallet that implements the real execution-provider
// contract (version fence, idempotency by key with a request fingerprint,
// not-held refusal, no-change refusal, 404 for a booking with no payment).

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	bookingpayments "cosmicforge/logistics/services/hauling-service/internal/features/booking/payments"
	"cosmicforge/logistics/shared/go/walletclient"
)

// ── Fake Wallet ───────────────────────────────────────────────────────────────

type walletRebindRecord struct {
	fingerprint string
	rebind      walletclient.ExecutionProviderRebind
}

type contractWallet struct {
	mu           sync.Mutex
	exists       bool
	status       string
	providerType string
	providerID   string
	version      int64
	history      []walletclient.ExecutionProviderRebind
	keys         map[string]walletRebindRecord

	// Fault injection.
	readOutage   bool // GET answers 503
	sendOutage   bool // POST answers 503 WITHOUT applying
	dropAfterOne int  // POST applies, then the connection is cut (lost response)
	readDelay    time.Duration

	gets, posts, applied int
}

func newContractWallet(providerID string) *contractWallet {
	return &contractWallet{exists: true, status: "held", providerType: "truck_provider",
		providerID: providerID, version: 1, keys: map[string]walletRebindRecord{}}
}

func writeEnvelope(rw http.ResponseWriter, status int, data any, code, message string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	if status >= 400 {
		_ = json.NewEncoder(rw).Encode(map[string]any{"success": false,
			"error": map[string]any{"code": code, "message": message}})
		return
	}
	_ = json.NewEncoder(rw).Encode(map[string]any{"success": true, "data": data})
}

func (w *contractWallet) server(t *testing.T, bookingID string) *httptest.Server {
	t.Helper()
	path := "/api/v1/payment-wallet/internal/jobs/driver-hauling-service/" + bookingID + "/execution-provider"
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("unexpected wallet path %s", r.URL.Path)
			writeEnvelope(rw, http.StatusNotFound, nil, "not_found", "Not found.")
			return
		}
		if r.Method == http.MethodGet {
			w.serveRead(rw)
			return
		}
		w.serveRebind(rw, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func (w *contractWallet) serveRead(rw http.ResponseWriter) {
	w.mu.Lock()
	w.gets++
	delay, outage := w.readDelay, w.readOutage
	w.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if outage {
		writeEnvelope(rw, http.StatusServiceUnavailable, nil, "unavailable", "Wallet unavailable.")
		return
	}
	if !w.exists {
		writeEnvelope(rw, http.StatusNotFound, nil, "not_found", "Payment not found.")
		return
	}
	writeEnvelope(rw, http.StatusOK, walletclient.ExecutionProviderBinding{
		Version: w.version, ProviderType: w.providerType, ProviderID: w.providerID,
		Status: w.status, History: append([]walletclient.ExecutionProviderRebind{}, w.history...),
	}, "", "")
}

func (w *contractWallet) serveRebind(rw http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var request walletclient.ExecutionProviderRebindRequest
	_ = json.Unmarshal(raw, &request)

	w.mu.Lock()
	w.posts++
	if w.sendOutage {
		w.mu.Unlock()
		writeEnvelope(rw, http.StatusServiceUnavailable, nil, "unavailable", "Wallet unavailable.")
		return
	}
	if !w.exists {
		w.mu.Unlock()
		writeEnvelope(rw, http.StatusNotFound, nil, "not_found", "Payment not found.")
		return
	}
	fingerprint := strings.Join([]string{request.ProviderType, request.ProviderID,
		strconv.FormatInt(request.ExpectedVersion, 10), request.RecoveryCaseRef, request.ReasonCode}, "|")
	// Real Wallet order: the key is looked up before anything else is judged.
	if record, ok := w.keys[request.IdempotencyKey]; ok {
		w.mu.Unlock()
		if record.fingerprint != fingerprint {
			writeEnvelope(rw, http.StatusConflict, nil, "idempotency_key_reused", "Key reused.")
			return
		}
		replay := record.rebind
		replay.Replayed = true
		writeEnvelope(rw, http.StatusOK, replay, "", "")
		return
	}
	switch {
	case w.status != "held":
		w.mu.Unlock()
		writeEnvelope(rw, http.StatusConflict, nil, walletclient.CodePaymentNotHeld, "Not held.")
		return
	case w.version != request.ExpectedVersion:
		w.mu.Unlock()
		writeEnvelope(rw, http.StatusConflict, nil, walletclient.CodeBindingVersionConflict, "Version conflict.")
		return
	case w.providerID == request.ProviderID && w.providerType == request.ProviderType:
		w.mu.Unlock()
		writeEnvelope(rw, http.StatusConflict, nil, walletclient.CodeProviderAlreadyBound, "Already bound.")
		return
	}
	rebind := walletclient.ExecutionProviderRebind{
		ID: "rebind-" + strconv.Itoa(len(w.history)+1), PreviousProviderID: w.providerID,
		NewProviderType: request.ProviderType, NewProviderID: request.ProviderID,
		FromVersion: w.version, ToVersion: w.version + 1, RecoveryCaseRef: request.RecoveryCaseRef,
	}
	w.providerID, w.providerType, w.version = request.ProviderID, request.ProviderType, w.version+1
	w.history = append(w.history, rebind)
	w.keys[request.IdempotencyKey] = walletRebindRecord{fingerprint: fingerprint, rebind: rebind}
	w.applied++
	drop := w.dropAfterOne > 0
	if drop {
		w.dropAfterOne--
	}
	w.mu.Unlock()
	if drop {
		// Applied, but the caller never hears about it.
		if hijacker, ok := rw.(http.Hijacker); ok {
			if conn, _, err := hijacker.Hijack(); err == nil {
				_ = conn.Close()
				return
			}
		}
	}
	writeEnvelope(rw, http.StatusOK, rebind, "", "")
}

func (w *contractWallet) set(apply func(*contractWallet)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	apply(w)
}

func (w *contractWallet) bound() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.providerID
}

func (w *contractWallet) hasKey(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.keys[key]
	return ok
}

func (w *contractWallet) counts() (gets, posts, applied int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.gets, w.posts, w.applied
}

// ── Harness ───────────────────────────────────────────────────────────────────

func rebindWorker(pool *pgxpool.Pool, server *httptest.Server) *PaymentRebindWorker {
	return &PaymentRebindWorker{
		DB: pool,
		Wallet: walletclient.Client{BaseURL: server.URL, ServiceName: "driver-hauling-service",
			Secret: []byte("test-secret"), HTTPClient: &http.Client{Timeout: 5 * time.Second}},
		MaxAttempts: 5, BaseBackoff: time.Second, MaxBackoff: time.Minute,
	}
}

func obligations(t *testing.T, pool *pgxpool.Pool, bookingID string) []PaymentRebindObligation {
	t.Helper()
	found, err := NewRepository(pool).PaymentRebindsForBooking(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("read obligations: %v", err)
	}
	return found
}

func onlyObligation(t *testing.T, pool *pgxpool.Pool, bookingID string) PaymentRebindObligation {
	t.Helper()
	found := obligations(t, pool, bookingID)
	if len(found) != 1 {
		t.Fatalf("obligations = %d, want exactly 1: %+v", len(found), found)
	}
	return found[0]
}

// makeDue lets the next sweep pick the obligation up without waiting out its
// backoff.
func makeDue(t *testing.T, pool *pgxpool.Pool, bookingID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE hauling_payment_rebind_obligations SET next_attempt_at=now()-interval '1 second' WHERE booking_id=$1`,
		bookingID); err != nil {
		t.Fatalf("make due: %v", err)
	}
}

// replacedBooking runs an automatic replacement (providerA -> providerC) and
// returns the booking and case ids.
func replacedBooking(t *testing.T, pool *pgxpool.Pool, operationRef string) (string, string) {
	t.Helper()
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	opened := startCase(t, service, bookingID, operationRef)
	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err != nil {
		t.Fatalf("select replacement: %v", err)
	}
	return bookingID, opened.RecoveryCaseID
}

// ── Obligation is written atomically with every replacement path ─────────────

func TestPaymentRebindObligationWrittenWithAutomaticReplacement(t *testing.T) {
	pool := testPool(t)
	bookingID, caseID := replacedBooking(t, pool, "op-rebind-auto")
	obligation := onlyObligation(t, pool, bookingID)
	if obligation.RecoveryCaseID != caseID || obligation.ProviderID != providerC ||
		obligation.State != RebindPending || obligation.Attempts != 0 {
		t.Fatalf("unexpected obligation %+v", obligation)
	}
	var caseReplacement string
	_ = pool.QueryRow(context.Background(),
		`SELECT replacement_provider_id::text FROM hauling_recovery_cases WHERE id=$1`, caseID).Scan(&caseReplacement)
	if caseReplacement != providerC {
		t.Fatal("the case advance did not commit with the booking rebind and the obligation")
	}
}

func TestPaymentRebindObligationWrittenWithCustodyTransfer(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "picked_up")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	opened := startCase(t, service, bookingID, "op-rebind-transfer")
	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if found := obligations(t, pool, bookingID); len(found) != 0 {
		t.Fatalf("an obligation was written before the cargo changed hands: %+v", found)
	}
	if _, err := service.CompleteTransfer(context.Background(), TransferCommand{
		BookingID: bookingID, OperationRef: "t-rebind", TransferRef: "TR-RB", EvidenceRef: "EV-RB",
		OldProviderConfirmed: true, NewProviderConfirmed: true, Actor: "test",
	}); err != nil {
		t.Fatalf("complete transfer: %v", err)
	}
	obligation := onlyObligation(t, pool, bookingID)
	if obligation.RecoveryCaseID != opened.RecoveryCaseID || obligation.ProviderID != providerC ||
		obligation.State != RebindPending {
		t.Fatalf("unexpected obligation %+v", obligation)
	}
}

func TestPaymentRebindObligationWrittenWithOperatorReplacementAndRolledBackWithIt(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(), fittingCapability())
	opened := startCase(t, service, bookingID, "op-rebind-admin")

	// An admin command transaction that rolls back takes the obligation with it.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SelectNamedReplacementTx(ctx, tx, opened.RecoveryCaseID, providerC, truckC, "karryam-admin:test"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("named replacement: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if found := obligations(t, pool, bookingID); len(found) != 0 {
		t.Fatalf("a rolled-back replacement left an obligation: %+v", found)
	}
	var provider string
	_ = pool.QueryRow(ctx, `SELECT provider_id::text FROM haulage_bookings WHERE id=$1`, bookingID).Scan(&provider)
	if provider != providerA {
		t.Fatal("the rolled-back replacement rebound the booking")
	}

	// The committed one writes it, once.
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SelectNamedReplacementTx(ctx, tx, opened.RecoveryCaseID, providerC, truckC, "karryam-admin:test"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("named replacement: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	obligation := onlyObligation(t, pool, bookingID)
	if obligation.RecoveryCaseID != opened.RecoveryCaseID || obligation.ProviderID != providerC ||
		obligation.State != RebindPending {
		t.Fatalf("unexpected obligation %+v", obligation)
	}
	// A duplicate write for the same (case, provider) collapses onto it.
	tx, _ = pool.Begin(ctx)
	if err := NewRepository(pool).RecordPaymentRebindTx(ctx, tx, bookingID, opened.RecoveryCaseID, providerC); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit(ctx)
	onlyObligation(t, pool, bookingID)
}

func TestPaymentRebindObligationNotWrittenWhenReplacementRollsBack(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	startCase(t, service, bookingID, "op-rebind-lost")
	if _, err := pool.Exec(context.Background(),
		`UPDATE haulage_bookings SET status='completed' WHERE id=$1`, bookingID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err == nil {
		t.Fatal("a replacement was bound to a completed booking")
	}
	if found := obligations(t, pool, bookingID); len(found) != 0 {
		t.Fatalf("a failed replacement left an obligation: %+v", found)
	}
}

// ── Worker ────────────────────────────────────────────────────────────────────

func TestPaymentRebindWorkerCompletesTheRebind(t *testing.T) {
	pool := testPool(t)
	bookingID, caseID := replacedBooking(t, pool, "op-rebind-worker")
	wallet := newContractWallet(providerA)
	worker := rebindWorker(pool, wallet.server(t, bookingID))

	claimed, err := worker.RunOnce(context.Background())
	if err != nil || claimed != 1 {
		t.Fatalf("claimed=%d err=%v", claimed, err)
	}
	obligation := onlyObligation(t, pool, bookingID)
	if obligation.State != RebindCompleted || obligation.WalletBindingVersion == nil ||
		*obligation.WalletBindingVersion != 2 || obligation.WalletRebindID != "rebind-1" ||
		obligation.CompletedAt == nil || obligation.Attempts != 0 {
		t.Fatalf("unexpected obligation %+v", obligation)
	}
	_, posts, applied := wallet.counts()
	if posts != 1 || applied != 1 || wallet.bound() != providerC {
		t.Fatalf("posts=%d applied=%d bound=%s", posts, applied, wallet.bound())
	}
	if !wallet.hasKey(RebindIdempotencyKey(bookingID, caseID, 1)) {
		t.Fatal("the rebind was not keyed by case and binding version")
	}
	// A completed obligation is never claimed again.
	if claimed, _ := worker.RunOnce(context.Background()); claimed != 0 {
		t.Fatal("a completed obligation was claimed again")
	}
}

func TestPaymentRebindLostResponseResolvesByReadbackWithoutASecondEffect(t *testing.T) {
	pool := testPool(t)
	bookingID, _ := replacedBooking(t, pool, "op-rebind-lostresp")
	wallet := newContractWallet(providerA)
	wallet.dropAfterOne = 1
	worker := rebindWorker(pool, wallet.server(t, bookingID))

	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	obligation := onlyObligation(t, pool, bookingID)
	if obligation.State != RebindUnknownOutcome || obligation.Attempts != 1 ||
		obligation.LastErrorCode != RebindErrWalletUnreachable || obligation.CompletedAt != nil {
		t.Fatalf("a lost response must be UNKNOWN_OUTCOME, got %+v", obligation)
	}

	makeDue(t, pool, bookingID)
	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	obligation = onlyObligation(t, pool, bookingID)
	if obligation.State != RebindCompleted || obligation.WalletRebindID != "rebind-1" ||
		*obligation.WalletBindingVersion != 2 {
		t.Fatalf("readback did not resolve the outcome: %+v", obligation)
	}
	_, posts, applied := wallet.counts()
	if posts != 1 || applied != 1 {
		t.Fatalf("readback must resolve without re-sending: posts=%d applied=%d", posts, applied)
	}
}

func TestPaymentRebindConcurrentWorkersProcessOneObligationOnce(t *testing.T) {
	pool := testPool(t)
	bookingID, _ := replacedBooking(t, pool, "op-rebind-race")
	wallet := newContractWallet(providerA)
	wallet.readDelay = 50 * time.Millisecond // widen the window while a lease is held
	server := wallet.server(t, bookingID)

	const workers = 8
	var wait sync.WaitGroup
	claims := make([]int, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func(slot int) {
			defer wait.Done()
			<-start
			claims[slot], _ = rebindWorker(pool, server).RunOnce(context.Background())
		}(i)
	}
	close(start)
	wait.Wait()

	total := 0
	for _, claimed := range claims {
		total += claimed
	}
	gets, posts, applied := wallet.counts()
	if total != 1 || gets != 1 || posts != 1 || applied != 1 {
		t.Fatalf("claims=%d gets=%d posts=%d applied=%d, want exactly one of each", total, gets, posts, applied)
	}
	if obligation := onlyObligation(t, pool, bookingID); obligation.State != RebindCompleted {
		t.Fatalf("state = %s", obligation.State)
	}
}

func TestPaymentRebindWalletOutageThenRecovery(t *testing.T) {
	pool := testPool(t)
	bookingID, _ := replacedBooking(t, pool, "op-rebind-outage")
	wallet := newContractWallet(providerA)
	worker := rebindWorker(pool, wallet.server(t, bookingID))
	ctx := context.Background()

	// Read outage: nothing was sent, so the obligation is still PENDING.
	wallet.set(func(w *contractWallet) { w.readOutage = true })
	_, _ = worker.RunOnce(ctx)
	if obligation := onlyObligation(t, pool, bookingID); obligation.State != RebindPending ||
		obligation.Attempts != 1 || obligation.LastErrorCode != RebindErrWalletUnreachable {
		t.Fatalf("read outage: %+v", obligation)
	}

	// Send outage after a good read: the rebind may have landed -> unknown.
	wallet.set(func(w *contractWallet) { w.readOutage, w.sendOutage = false, true })
	makeDue(t, pool, bookingID)
	_, _ = worker.RunOnce(ctx)
	obligation := onlyObligation(t, pool, bookingID)
	if obligation.State != RebindUnknownOutcome || obligation.Attempts != 2 {
		t.Fatalf("send outage: %+v", obligation)
	}
	if !obligation.NextAttemptAt.After(time.Now().Add(-time.Second)) {
		t.Fatal("an unknown outcome must back off before the next attempt")
	}
	// Not yet due: the backoff holds it.
	if claimed, _ := worker.RunOnce(ctx); claimed != 0 {
		t.Fatal("an obligation in backoff was claimed early")
	}

	// Wallet back: readback shows nothing applied, so it is sent once.
	wallet.set(func(w *contractWallet) { w.sendOutage = false })
	makeDue(t, pool, bookingID)
	_, _ = worker.RunOnce(ctx)
	if obligation := onlyObligation(t, pool, bookingID); obligation.State != RebindCompleted {
		t.Fatalf("recovery: %+v", obligation)
	}
	if _, _, applied := wallet.counts(); applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
}

func TestPaymentRebindNotHeldIsTerminalWithoutASend(t *testing.T) {
	pool := testPool(t)
	for name, prepare := range map[string]func(*contractWallet){
		"refunded":          func(w *contractWallet) { w.status = "refunded" },
		"settled":           func(w *contractWallet) { w.status = "completed" },
		"no wallet payment": func(w *contractWallet) { w.exists = false },
	} {
		t.Run(name, func(t *testing.T) {
			bookingID, _ := replacedBooking(t, pool, "op-rebind-notheld-"+strings.ReplaceAll(name, " ", "-"))
			wallet := newContractWallet(providerA)
			prepare(wallet)
			worker := rebindWorker(pool, wallet.server(t, bookingID))
			_, _ = worker.RunOnce(context.Background())
			obligation := onlyObligation(t, pool, bookingID)
			if obligation.State != RebindCompletedNotHeld || obligation.CompletedAt == nil {
				t.Fatalf("state %+v", obligation)
			}
			if _, posts, _ := wallet.counts(); posts != 0 {
				t.Fatal("a payment that holds nothing was sent a rebind")
			}
		})
	}
}

func TestPaymentRebindStaleProviderIsSupersededWithoutCallingWallet(t *testing.T) {
	pool := testPool(t)
	for name, staleify := range map[string]string{
		// The booking was replaced again by someone else.
		"booking re-replaced": `UPDATE haulage_bookings SET provider_id=$2::uuid WHERE id=$1`,
		// The case no longer names this provider.
		"case moved": `UPDATE hauling_recovery_cases SET replacement_provider_id=$2::uuid WHERE booking_id=$1`,
	} {
		t.Run(name, func(t *testing.T) {
			bookingID, _ := replacedBooking(t, pool, "op-rebind-stale-"+strings.ReplaceAll(name, " ", "-"))
			if _, err := pool.Exec(context.Background(), staleify, bookingID, providerB); err != nil {
				t.Fatal(err)
			}
			wallet := newContractWallet(providerA)
			worker := rebindWorker(pool, wallet.server(t, bookingID))
			_, _ = worker.RunOnce(context.Background())
			obligation := onlyObligation(t, pool, bookingID)
			if obligation.State != RebindSuperseded || obligation.LastErrorCode != RebindErrStaleProvider ||
				obligation.CompletedAt == nil {
				t.Fatalf("state %+v", obligation)
			}
			if gets, posts, _ := wallet.counts(); gets != 0 || posts != 0 {
				t.Fatalf("a stale obligation called Wallet: gets=%d posts=%d", gets, posts)
			}
		})
	}
}

func TestPaymentRebindMaxAttemptsParksForReconciliation(t *testing.T) {
	pool := testPool(t)
	bookingID, caseID := replacedBooking(t, pool, "op-rebind-max")
	wallet := newContractWallet(providerA)
	wallet.sendOutage = true
	worker := rebindWorker(pool, wallet.server(t, bookingID))
	worker.MaxAttempts = 3
	ctx := context.Background()
	for attempt := 0; attempt < 3; attempt++ {
		makeDue(t, pool, bookingID)
		if claimed, _ := worker.RunOnce(ctx); claimed != 1 {
			t.Fatalf("attempt %d: nothing claimed", attempt+1)
		}
	}
	obligation := onlyObligation(t, pool, bookingID)
	if obligation.State != RebindReconciliationRequired || obligation.Attempts != 3 ||
		obligation.CompletedAt == nil || obligation.LastErrorCode != RebindErrWalletUnreachable {
		t.Fatalf("state %+v", obligation)
	}
	makeDue(t, pool, bookingID)
	if claimed, _ := worker.RunOnce(ctx); claimed != 0 {
		t.Fatal("a parked obligation was claimed again")
	}
	// Still explainable: the recovery case still names the replacement, so
	// the settlement guard can justify the rebind.
	if ref, err := NewRepository(pool).CaseForReplacement(ctx, bookingID, providerC); err != nil || ref != caseID {
		t.Fatalf("case lookup = %q, %v", ref, err)
	}
}

func TestPaymentRebindPostCommitNudgeRecordsTheOutcome(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	wallet := newContractWallet(providerA)
	worker := rebindWorker(pool, wallet.server(t, bookingID))
	service := New(Options{
		Repository: NewRepository(pool), Candidates: nearby(providerC), Capability: fittingCapability(),
		Events: NewOutboxSink(pool, "analytics-service", true, nil), Enabled: true,
		PaymentRebinds: worker,
	})
	startCase(t, service, bookingID, "op-rebind-nudge")
	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err != nil {
		t.Fatal(err)
	}
	if obligation := onlyObligation(t, pool, bookingID); obligation.State != RebindCompleted {
		t.Fatalf("the fast path did not record its outcome: %+v", obligation)
	}
	if _, _, applied := wallet.counts(); applied != 1 {
		t.Fatalf("applied = %d", applied)
	}
}

// ── Settlement guard (unchanged behaviour, real recovery lookup) ─────────────

func TestSettlementGuardStillRefusesAnUnexplainedProviderAndAcceptsAnExplainedOne(t *testing.T) {
	pool := testPool(t)
	bookingID, caseID := replacedBooking(t, pool, "op-rebind-guard")
	// Even with the obligation parked, the case explains the replacement.
	if _, err := pool.Exec(context.Background(), `UPDATE hauling_payment_rebind_obligations
		SET state='RECONCILIATION_REQUIRED', completed_at=now() WHERE booking_id=$1`, bookingID); err != nil {
		t.Fatal(err)
	}
	wallet := newContractWallet(providerA)
	server := wallet.server(t, bookingID)
	guard := bookingpayments.NewWalletPaymentClient(server.URL, "driver-hauling-service", []byte("secret")).
		WithRecoveryCases(NewRepository(pool).CaseForReplacement)

	// providerB never replaced anyone on this booking: refused, nothing sent.
	if err := guard.EnsureExecutionProvider(context.Background(), bookingID, providerB); err == nil {
		t.Fatal("the settlement guard paid an unexplained provider")
	}
	if _, posts, _ := wallet.counts(); posts != 0 {
		t.Fatal("an unexplained provider caused a rebind")
	}
	// providerC is explained by the recovery case: rebound, keyed like the worker.
	if err := guard.EnsureExecutionProvider(context.Background(), bookingID, providerC); err != nil {
		t.Fatalf("explained provider refused: %v", err)
	}
	if !wallet.hasKey(RebindIdempotencyKey(bookingID, caseID, 1)) || wallet.bound() != providerC {
		t.Fatal("the guard's rebind did not use the shared case+version key")
	}
}
