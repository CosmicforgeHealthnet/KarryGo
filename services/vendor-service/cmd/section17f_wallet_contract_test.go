package main

// Section 17 closure, Part 5: the production Wallet adapter against the real
// wire contract.
//
// Every other Section 17 payment test substitutes a fake authority so it can
// stage outcomes. That proves what Vendor Service does with an answer, not
// that it can obtain one. This file drives the actual
// section17FPaymentAuthority — the type main() wires — over HTTP against a
// server that speaks payment-wallet-service's own protocol: the same paths,
// the same success/error envelope, the same HMAC v2 signature requirement.
//
// It deliberately claims no capability Wallet does not have. Wallet settles
// and refunds a payment intent as a whole to a single provider, which is
// exactly why Section 17F binds one intent per VendorOrder, and the tests
// below assert against that contract rather than around it.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const walletContractSecret = "section17f-wallet-contract-secret"

// walletCall records one request the adapter actually made, so the test can
// assert on the wire rather than on the adapter's own report of itself.
type walletCall struct {
	Method string
	Path   string
	Body   map[string]any
	Signed bool
	Caller string
}

// fakeWalletService speaks payment-wallet-service's internal contract. It
// verifies the HMAC the real service verifies, and keeps intents in memory so
// idempotency is answered the way the real service answers it: by returning
// the existing row rather than creating a second one.
type fakeWalletService struct {
	t *testing.T

	mu      sync.Mutex
	calls   []walletCall
	intents map[string]map[string]any // idempotency key -> intent
	byRef   map[string]map[string]any // reference        -> intent
	refunds map[string]map[string]any // idempotency key  -> refund

	// failPayFromWallet makes the hold step fail after the intent exists,
	// which is the ambiguous outcome recovery has to resolve.
	failPayFromWallet bool
	// payTimesOut makes the request hang past the adapter's timeout.
	payTimesOut bool
}

func newFakeWalletService(t *testing.T) *fakeWalletService {
	return &fakeWalletService{
		t:       t,
		intents: map[string]map[string]any{},
		byRef:   map[string]map[string]any{},
		refunds: map[string]map[string]any{},
	}
}

func (w *fakeWalletService) record(c walletCall) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, c)
}

func (w *fakeWalletService) recorded() []walletCall {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]walletCall, len(w.calls))
	copy(out, w.calls)
	return out
}

func (w *fakeWalletService) callsTo(fragment string) int {
	count := 0
	for _, call := range w.recorded() {
		if strings.Contains(call.Path, fragment) {
			count++
		}
	}
	return count
}

func writeWalletData(rw http.ResponseWriter, status int, data any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(map[string]any{"success": true, "data": data})
}

func writeWalletError(rw http.ResponseWriter, status int, message string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(map[string]any{
		"success": false,
		"error":   map[string]any{"code": "wallet_error", "message": message},
	})
}

func (w *fakeWalletService) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)

	var body map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}

	// The real service refuses an unsigned or wrongly-signed internal call.
	// Verifying it with the platform's own verifier, allowlisting exactly the
	// caller payment-wallet-service would allowlist, is what makes this a
	// contract test rather than a stub. The verifier reads the body itself, so
	// it is restored first.
	r.Body = io.NopCloser(bytes.NewReader(raw))
	caller, err := serviceauth.NewVerifier(serviceauth.Secrets{
		orders.PaymentSourceService: []byte(walletContractSecret),
	}, 5*time.Minute).VerifyRequest(r)

	w.record(walletCall{
		Method: r.Method,
		Path:   r.URL.Path,
		Body:   body,
		Signed: err == nil,
		Caller: caller,
	})

	if err != nil {
		writeWalletError(rw, http.StatusUnauthorized, "Service authentication failed.")
		return
	}

	path := r.URL.Path
	switch {
	case r.Method == http.MethodPost &&
		path == "/api/v1/payment-wallet/internal/payment-intents":
		w.createIntent(rw, body)

	case r.Method == http.MethodPost && strings.HasSuffix(path, "/pay-from-wallet"):
		w.payFromWallet(rw, path)

	case r.Method == http.MethodPost &&
		path == "/api/v1/payment-wallet/internal/refunds":
		w.refund(rw, body)

	case r.Method == http.MethodGet &&
		strings.HasPrefix(path, "/api/v1/payment-wallet/internal/payments/"):
		w.getPayment(rw, path)

	case r.Method == http.MethodPost && strings.HasSuffix(path, "/complete"):
		w.completeJob(rw, path)

	default:
		writeWalletError(rw, http.StatusNotFound, "Unknown wallet route.")
	}
}

func (w *fakeWalletService) createIntent(rw http.ResponseWriter, body map[string]any) {
	key, _ := body["idempotency_key"].(string)

	w.mu.Lock()
	defer w.mu.Unlock()

	// Idempotent creation: the same key returns the same intent, exactly as
	// payment-wallet-service does.
	if existing, ok := w.intents[key]; ok {
		writeWalletData(rw, http.StatusOK, existing)
		return
	}

	amount, _ := body["amount_kobo"].(float64)
	id := fmt.Sprintf("pi-%d", len(w.intents)+1)
	intent := map[string]any{
		"id":               id,
		"reference":        "ref-" + id,
		"source_service":   body["source_service"],
		"source_reference": body["source_reference"],
		"customer_id":      body["customer_id"],
		"amount_kobo":      int64(amount),
		"currency":         body["currency"],
		"payment_method":   body["payment_method"],
		"status":           "pending",
	}
	w.intents[key] = intent
	w.byRef[intent["reference"].(string)] = intent
	writeWalletData(rw, http.StatusCreated, intent)
}

func (w *fakeWalletService) payFromWallet(rw http.ResponseWriter, path string) {
	if w.payTimesOut {
		time.Sleep(3 * time.Second)
		return
	}
	if w.failPayFromWallet {
		writeWalletError(rw, http.StatusBadRequest, "Insufficient wallet balance.")
		return
	}

	id := strings.TrimSuffix(strings.TrimPrefix(
		path, "/api/v1/payment-wallet/internal/payment-intents/"), "/pay-from-wallet")

	w.mu.Lock()
	defer w.mu.Unlock()
	for _, intent := range w.intents {
		if intent["id"] == id {
			intent["status"] = "held"
			writeWalletData(rw, http.StatusOK, intent)
			return
		}
	}
	writeWalletError(rw, http.StatusNotFound, "Payment intent not found.")
}

func (w *fakeWalletService) getPayment(rw http.ResponseWriter, path string) {
	reference := strings.TrimPrefix(path, "/api/v1/payment-wallet/internal/payments/")

	w.mu.Lock()
	defer w.mu.Unlock()
	if intent, ok := w.byRef[reference]; ok {
		writeWalletData(rw, http.StatusOK, intent)
		return
	}
	writeWalletError(rw, http.StatusNotFound, "Payment not found.")
}

func (w *fakeWalletService) refund(rw http.ResponseWriter, body map[string]any) {
	key, _ := body["idempotency_key"].(string)

	w.mu.Lock()
	defer w.mu.Unlock()

	if existing, ok := w.refunds[key]; ok {
		writeWalletData(rw, http.StatusOK, existing)
		return
	}

	amount, _ := body["amount_kobo"].(float64)
	reference, _ := body["payment_reference"].(string)

	// Wallet refunds against an existing intent. A refund naming no intent it
	// knows is refused, not invented.
	if _, ok := w.byRef[reference]; !ok {
		writeWalletError(rw, http.StatusNotFound, "Payment not found.")
		return
	}

	refund := map[string]any{
		"id":          fmt.Sprintf("rf-%d", len(w.refunds)+1),
		"reference":   fmt.Sprintf("refref-%d", len(w.refunds)+1),
		"amount_kobo": int64(amount),
		"currency":    body["currency"],
		"status":      "processing",
	}
	w.refunds[key] = refund
	writeWalletData(rw, http.StatusOK, refund)
}

func (w *fakeWalletService) completeJob(rw http.ResponseWriter, path string) {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(
		path, "/api/v1/payment-wallet/internal/jobs/"), "/complete")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		writeWalletError(rw, http.StatusBadRequest, "Malformed job path.")
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	for _, intent := range w.intents {
		if intent["source_reference"] == parts[1] {
			intent["status"] = "success"
			writeWalletData(rw, http.StatusOK, intent)
			return
		}
	}
	writeWalletError(rw, http.StatusNotFound, "No held payment for that job.")
}

func walletContractAuthority(
	t *testing.T, wallet *fakeWalletService,
) (*section17FPaymentAuthority, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(wallet)
	t.Cleanup(server.Close)

	// The base URL is a BARE ORIGIN. walletclient appends the API path itself,
	// and passing a full path here is the doubled-path mistake this platform
	// already made once with HAULING_PAYMENT_URL.
	authority := newSection17FPaymentAuthority(
		true, server.URL, walletContractSecret, 2*time.Second)

	if !authority.Configured() {
		t.Fatal("the production adapter reported itself unconfigured with real credentials")
	}
	return authority, server
}

func holdRequest(reference, key string, amount int64) orders.HoldLegRequest {
	return orders.HoldLegRequest{
		SourceService:   orders.PaymentSourceService,
		SourceReference: reference,
		CustomerID:      "cus-contract",
		CustomerEmail:   "buyer@example.test",
		AmountKobo:      amount,
		Currency:        "NGN",
		IdempotencyKey:  key,
	}
}

// A positive delta is authorized by creating an intent and paying it from the
// wallet, through the production adapter, signed as this service.
func TestSection17FWallet_PositiveDeltaIsAuthorizedThroughTheRealContract(t *testing.T) {
	wallet := newFakeWalletService(t)
	authority, _ := walletContractAuthority(t, wallet)

	result, err := authority.HoldLeg(
		t.Context(), holdRequest("vo-1", "commerce-hold-vo-1", 250_00))
	if err != nil {
		t.Fatalf("holding a delta through the real contract failed: %v", err)
	}

	if result.Status != "held" {
		t.Errorf("leg status = %q, want held", result.Status)
	}
	if result.AmountKobo != 250_00 {
		t.Errorf("held %d kobo, want 25000", result.AmountKobo)
	}
	if result.Reference == "" {
		t.Error("the adapter returned no reference, so recovery could not re-read this leg")
	}

	// The hold is two authority calls, and both must be signed as this
	// service: an unsigned internal call is refused by the real service.
	calls := wallet.recorded()
	if len(calls) != 2 {
		t.Fatalf("made %d wallet calls, want create-intent then pay-from-wallet: %+v",
			len(calls), calls)
	}
	for _, call := range calls {
		if !call.Signed {
			t.Errorf("%s %s was not signed and the real service would refuse it",
				call.Method, call.Path)
		}
		if call.Caller != orders.PaymentSourceService {
			t.Errorf("signed as %q, want %q", call.Caller, orders.PaymentSourceService)
		}
	}

	// Vendor Service never tells Wallet what the customer's balance is, nor
	// what the payment's outcome should be.
	body := calls[0].Body
	for _, forbidden := range []string{"status", "balance", "wallet_balance", "settled"} {
		if _, present := body[forbidden]; present {
			t.Errorf("the intent request carries authority-owned field %q", forbidden)
		}
	}
	if body["payment_method"] != "wallet" {
		t.Errorf("payment_method = %v, want wallet", body["payment_method"])
	}
}

// A retried hold must not create a second intent. Idempotency is the
// deterministic key the caller supplies and the authority honours.
func TestSection17FWallet_RepeatedHoldReusesTheSameIntent(t *testing.T) {
	wallet := newFakeWalletService(t)
	authority, _ := walletContractAuthority(t, wallet)

	first, err := authority.HoldLeg(
		t.Context(), holdRequest("vo-1", "commerce-hold-vo-1", 900_00))
	if err != nil {
		t.Fatal(err)
	}
	second, err := authority.HoldLeg(
		t.Context(), holdRequest("vo-1", "commerce-hold-vo-1", 900_00))
	if err != nil {
		t.Fatal(err)
	}

	if first.IntentID != second.IntentID {
		t.Fatalf("a retry created a second intent: %s then %s",
			first.IntentID, second.IntentID)
	}
	if len(wallet.intents) != 1 {
		t.Fatalf("the authority holds %d intents for one leg, want 1", len(wallet.intents))
	}

	// The second hold found the intent already past pending and reported it as
	// it stands rather than paying it again.
	if wallet.callsTo("pay-from-wallet") != 1 {
		t.Fatalf("pay-from-wallet was called %d times for one leg",
			wallet.callsTo("pay-from-wallet"))
	}
	if second.Status != "held" {
		t.Errorf("the retry reported status %q, want the authority's held", second.Status)
	}
}

// A cheaper replan refunds exactly the difference, against the one intent that
// vendor's allocation was paid on.
func TestSection17FWallet_CheaperReplanRefundsThroughTheRealContract(t *testing.T) {
	wallet := newFakeWalletService(t)
	authority, _ := walletContractAuthority(t, wallet)

	held, err := authority.HoldLeg(
		t.Context(), holdRequest("vo-1", "commerce-hold-vo-1", 500_00))
	if err != nil {
		t.Fatal(err)
	}

	refund, err := authority.RefundLeg(t.Context(), orders.RefundLegRequest{
		PaymentReference: held.Reference,
		SourceService:    orders.PaymentSourceService,
		SourceReference:  "vo-1",
		AmountKobo:       120_00,
		Currency:         "NGN",
		Reason:           "cheaper fulfilment replan",
		IdempotencyKey:   "commerce-cheaper-replan-order-1",
	})
	if err != nil {
		t.Fatalf("refunding the benefit failed: %v", err)
	}

	if refund.AmountKobo != 120_00 {
		t.Errorf("refunded %d kobo, want 12000", refund.AmountKobo)
	}
	if refund.Status == "" || refund.Reference == "" {
		t.Errorf("refund carries no status/reference to reconcile: %+v", refund)
	}

	// A repeated benefit must not refund twice.
	again, err := authority.RefundLeg(t.Context(), orders.RefundLegRequest{
		PaymentReference: held.Reference,
		SourceService:    orders.PaymentSourceService,
		SourceReference:  "vo-1",
		AmountKobo:       120_00,
		Currency:         "NGN",
		Reason:           "cheaper fulfilment replan",
		IdempotencyKey:   "commerce-cheaper-replan-order-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.Reference != refund.Reference {
		t.Fatalf("a repeated benefit created a second refund: %s then %s",
			refund.Reference, again.Reference)
	}
	if len(wallet.refunds) != 1 {
		t.Fatalf("the authority holds %d refunds for one benefit, want 1",
			len(wallet.refunds))
	}
}

// A refund naming a payment Wallet does not know is refused rather than
// invented. Vendor Service is not the money authority.
func TestSection17FWallet_RefundOfAnUnknownPaymentIsRefused(t *testing.T) {
	wallet := newFakeWalletService(t)
	authority, _ := walletContractAuthority(t, wallet)

	_, err := authority.RefundLeg(t.Context(), orders.RefundLegRequest{
		PaymentReference: "ref-does-not-exist",
		SourceService:    orders.PaymentSourceService,
		SourceReference:  "vo-1",
		AmountKobo:       100_00,
		Currency:         "NGN",
		IdempotencyKey:   "commerce-refund-unknown",
	})
	if err == nil {
		t.Fatal("refunding an unknown payment succeeded; Vendor Service invented money movement")
	}
}

// The ambiguous case. A hold whose pay step fails is not a decision: the
// intent exists and may have moved. The adapter must return the reference it
// does know, so recovery can ask the authority what really happened.
func TestSection17FWallet_FailedPayStepStillYieldsAReferenceToReconcile(t *testing.T) {
	wallet := newFakeWalletService(t)
	wallet.failPayFromWallet = true
	authority, _ := walletContractAuthority(t, wallet)

	result, err := authority.HoldLeg(
		t.Context(), holdRequest("vo-1", "commerce-hold-vo-1", 300_00))
	if err == nil {
		t.Fatal("a failing pay step reported success")
	}
	if result.Reference == "" {
		t.Fatal("the adapter discarded the reference, leaving recovery nothing to re-read")
	}

	// Recovery's read is the authority's own current view, not a guess.
	wallet.failPayFromWallet = false
	current, err := authority.GetLeg(t.Context(), result.Reference)
	if err != nil {
		t.Fatalf("recovery could not read the leg back: %v", err)
	}
	if current.Status != "pending" {
		t.Errorf("recovery read status %q; the authority never moved this leg", current.Status)
	}
	if current.Reference != result.Reference {
		t.Errorf("recovery read a different leg: %s vs %s",
			current.Reference, result.Reference)
	}
}

// A timeout is the same class of unknown: no answer is not a failure answer.
// The reference the adapter holds is what lets the sweep resolve it later.
func TestSection17FWallet_TimeoutIsUnknownAndResolvableOnTheNextSweep(t *testing.T) {
	wallet := newFakeWalletService(t)
	wallet.payTimesOut = true

	server := httptest.NewServer(wallet)
	t.Cleanup(server.Close)
	authority := newSection17FPaymentAuthority(
		true, server.URL, walletContractSecret, 200*time.Millisecond)

	result, err := authority.HoldLeg(
		t.Context(), holdRequest("vo-1", "commerce-hold-vo-1", 400_00))
	if err == nil {
		t.Fatal("a timed-out hold reported success")
	}
	if result.Reference == "" {
		t.Fatal("a timed-out hold left no reference, so the money could never be found again")
	}

	// The intent exists at the authority even though the caller never got an
	// answer: treating the timeout as a definite failure would have stranded
	// it. The next sweep reads it.
	wallet.payTimesOut = false
	current, err := authority.GetLeg(t.Context(), result.Reference)
	if err != nil {
		t.Fatalf("the sweep could not resolve the unknown leg: %v", err)
	}
	if current.IntentID != result.IntentID {
		t.Errorf("the sweep resolved a different intent: %s vs %s",
			current.IntentID, result.IntentID)
	}
}

// Settlement is per VendorOrder, because Wallet settles an intent as a whole
// to a single provider. One completion must never settle a sibling.
func TestSection17FWallet_SettlementIsScopedToOneVendorOrder(t *testing.T) {
	wallet := newFakeWalletService(t)
	authority, _ := walletContractAuthority(t, wallet)

	a, err := authority.HoldLeg(
		t.Context(), holdRequest("vo-a", "commerce-hold-vo-a", 100_00))
	if err != nil {
		t.Fatal(err)
	}
	b, err := authority.HoldLeg(
		t.Context(), holdRequest("vo-b", "commerce-hold-vo-b", 200_00))
	if err != nil {
		t.Fatal(err)
	}
	if a.IntentID == b.IntentID {
		t.Fatal("two vendor orders shared one intent; their money is not isolated")
	}

	settled, err := authority.CompleteLeg(
		t.Context(), orders.PaymentSourceService, "vo-a")
	if err != nil {
		t.Fatalf("settling vendor A failed: %v", err)
	}
	if settled.Status != "success" {
		t.Errorf("vendor A settled with status %q", settled.Status)
	}

	// Vendor B is untouched: still held, still refundable.
	stillHeld, err := authority.GetLeg(t.Context(), b.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if stillHeld.Status != "held" {
		t.Fatalf("settling vendor A moved vendor B to %q", stillHeld.Status)
	}
}

// Credentials are the switch. A deployment without them exposes no authority
// at all, rather than one that fails at the moment money is due.
func TestSection17FWallet_MissingCredentialsMeanNoAuthority(t *testing.T) {
	for name, authority := range map[string]*section17FPaymentAuthority{
		"no base URL": newSection17FPaymentAuthority(true, "", walletContractSecret, time.Second),
		"no secret":   newSection17FPaymentAuthority(true, "http://wallet.invalid", "", time.Second),
		"disabled":    newSection17FPaymentAuthority(false, "http://wallet.invalid", walletContractSecret, time.Second),
	} {
		if authority.Configured() {
			t.Errorf("%s: the adapter reported itself configured", name)
		}
		if _, err := authority.HoldLeg(t.Context(), holdRequest("vo-1", "k", 1)); err !=
			orders.ErrPaymentNotConfigured {
			t.Errorf("%s: holding returned %v, want ErrPaymentNotConfigured", name, err)
		}
	}
}

// The adapter must sign as this service. An unsigned call is refused by the
// real payment-wallet-service, so a wrong secret must fail loudly here rather
// than appear to work.
func TestSection17FWallet_AWrongSecretIsRefusedByTheAuthority(t *testing.T) {
	wallet := newFakeWalletService(t)
	server := httptest.NewServer(wallet)
	t.Cleanup(server.Close)

	authority := newSection17FPaymentAuthority(
		true, server.URL, "not-the-agreed-secret", 2*time.Second)

	if _, err := authority.HoldLeg(
		t.Context(), holdRequest("vo-1", "commerce-hold-vo-1", 100_00)); err == nil {
		t.Fatal("a wrongly-signed hold succeeded")
	}

	calls := wallet.recorded()
	if len(calls) == 0 {
		t.Fatal("no call reached the authority")
	}
	if calls[0].Signed {
		t.Error("a call signed with the wrong secret verified")
	}
	if len(wallet.intents) != 0 {
		t.Fatalf("%d intents were created by an unauthenticated caller", len(wallet.intents))
	}
}
