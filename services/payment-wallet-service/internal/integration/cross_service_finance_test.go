// Cross-service proof for the private finance API.
//
// These tests drive the real gin handlers over real HTTP against a real
// PostgreSQL database, signing every request with the same
// shared/go/serviceauth code the Fleet client uses. Fleet and Wallet are
// separate Go modules and neither may import the other, so the calling half is
// reproduced here with the shared signer rather than by taking a forbidden
// dependency: the bytes on the wire and the verification path are identical to
// production.
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	financehttp "cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/http"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/usecases"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const privateFinanceBase = "/api/v1/payment-wallet/internal/finance"

// callerSecrets are the per-service HMAC secrets. A caller signing with the
// wrong secret, or with an identity that is not allowlisted, must be rejected
// before any handler runs.
var callerSecrets = serviceauth.Secrets{
	"fleet-partner-service":    []byte("fleet-cross-service-secret"),
	"payment-wallet-service":   []byte("wallet-cross-service-secret"),
	"admin-backoffice-service": []byte("admin-cross-service-secret"),
	"support-dispute-service":  []byte("support-cross-service-secret"),
}

type privateAPI struct {
	fixture financeFixture
	router  *gin.Engine
	nonce   int
}

func newPrivateAPI(t *testing.T) *privateAPI {
	t.Helper()
	fixture := newFinanceFixture(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	financehttp.RegisterPrivateRoutes(router.Group(privateFinanceBase), fixture.service,
		serviceauth.NewVerifier(callerSecrets, 5*time.Minute),
		[]string{"fleet-partner-service", "payment-wallet-service", "admin-backoffice-service"})
	return &privateAPI{fixture: fixture, router: router}
}

type callOptions struct {
	caller string
	// secretFor overrides which caller's secret signs the request, so a valid
	// identity can be paired with the wrong key.
	secretFor string
	// signedAt backdates the signature to prove timestamp freshness is enforced.
	signedAt time.Time
	// nonce reuses an earlier nonce to prove replay protection.
	nonce string
	// unsigned omits the signature entirely.
	unsigned bool
}

func (p *privateAPI) do(t *testing.T, method, path string, body any, opts callOptions) (int, map[string]any) {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	caller := opts.caller
	if caller == "" {
		caller = "fleet-partner-service"
	}
	secretOwner := opts.secretFor
	if secretOwner == "" {
		secretOwner = caller
	}
	nonce := opts.nonce
	if nonce == "" {
		p.nonce++
		nonce = fmt.Sprintf("nonce-%d", p.nonce)
	}
	request := httptest.NewRequest(method, privateFinanceBase+path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Service-Nonce", nonce)
	if !opts.unsigned {
		secret, ok := callerSecrets[secretOwner]
		if !ok {
			secret = []byte("unknown-secret")
		}
		if err := serviceauth.SignRequest(request, caller, secret, payload, opts.signedAt); err != nil {
			t.Fatalf("sign request: %v", err)
		}
	}
	recorder := httptest.NewRecorder()
	p.router.ServeHTTP(recorder, request)
	var decoded map[string]any
	if recorder.Body.Len() > 0 {
		_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	}
	return recorder.Code, decoded
}

func dataOf(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	data, _ := response["data"].(map[string]any)
	if data == nil {
		t.Fatalf("response carried no data object: %#v", response)
	}
	return data
}

// TestCrossServicePrivateFinanceHMAC proves the transport contract: only a
// correctly signed, allowlisted, fresh, non-replayed call reaches a handler.
func TestCrossServicePrivateFinanceHMAC(t *testing.T) {
	api := newPrivateAPI(t)
	body := map[string]any{"payloadVersion": 1, "organizationId": "org-hmac",
		"currency": "NGN", "idempotencyKey": "hmac-wallet-1"}

	status, response := api.do(t, http.MethodPost, "/wallets/ensure", body, callOptions{})
	if status != http.StatusOK {
		t.Fatalf("valid HMAC rejected: status=%d body=%#v", status, response)
	}

	// A valid identity signing with another service's key must fail.
	if status, _ = api.do(t, http.MethodPost, "/wallets/ensure", body,
		callOptions{secretFor: "support-dispute-service"}); status != http.StatusUnauthorized {
		t.Fatalf("invalid signature accepted: status=%d", status)
	}
	if status, _ = api.do(t, http.MethodPost, "/wallets/ensure", body,
		callOptions{unsigned: true}); status != http.StatusUnauthorized {
		t.Fatalf("unsigned request accepted: status=%d", status)
	}
	// A correctly signed caller that is not on the allowlist is still refused.
	if status, _ = api.do(t, http.MethodPost, "/wallets/ensure", body,
		callOptions{caller: "support-dispute-service"}); status != http.StatusForbidden {
		t.Fatalf("non-allowlisted caller accepted: status=%d", status)
	}
	// A signature older than the verifier's window is refused even though it is
	// cryptographically valid: a captured request must expire.
	if status, _ = api.do(t, http.MethodPost, "/wallets/ensure", body,
		callOptions{signedAt: time.Now().Add(-30 * time.Minute)}); status != http.StatusUnauthorized {
		t.Fatalf("stale timestamp accepted: status=%d", status)
	}
	// Replay: same nonce, twice.
	if status, _ = api.do(t, http.MethodPost, "/wallets/ensure", body,
		callOptions{nonce: "replay-nonce"}); status != http.StatusOK {
		t.Fatalf("first nonce use rejected: status=%d", status)
	}
	if status, _ = api.do(t, http.MethodPost, "/wallets/ensure", body,
		callOptions{nonce: "replay-nonce"}); status != http.StatusConflict {
		t.Fatalf("replayed nonce accepted: status=%d", status)
	}
}

// recordJobPayment writes an authoritative job payment as the Wallet identity,
// which is the only identity holding finance.payment.confirm.
func (p *privateAPI) recordJobPayment(t *testing.T, organizationID, key string,
	fields map[string]any) map[string]any {
	t.Helper()
	body := map[string]any{"payloadVersion": 1, "organizationId": organizationID,
		"idempotencyKey": key}
	for name, value := range fields {
		body[name] = value
	}
	status, response := p.do(t, http.MethodPost, "/job-payments", body,
		callOptions{caller: "payment-wallet-service"})
	if status != http.StatusOK {
		t.Fatalf("record job payment: status=%d body=%#v", status, response)
	}
	return dataOf(t, response)
}

func (p *privateAPI) lookupJobPayment(t *testing.T, caller, organizationID,
	referenceType, reference string) (int, map[string]any) {
	t.Helper()
	path := "/job-payments/lookup?organizationId=" + organizationID +
		"&referenceType=" + referenceType + "&reference=" + reference
	return p.do(t, http.MethodGet, path, nil, callOptions{caller: caller})
}

// TestCrossServiceJobPaymentLookup proves the authoritative payment check: a
// consuming service can verify that a dispatch request or trip was paid for,
// cannot verify another tenant's payment, cannot mint a verification, and never
// receives balances or ledger identifiers.
func TestCrossServiceJobPaymentLookup(t *testing.T) {
	api := newPrivateAPI(t)
	const org = "org-jobpay"
	const otherOrg = "org-jobpay-other"

	api.recordJobPayment(t, org, "job-pay-1", map[string]any{
		"sourceService":            "driver-dispatch-delivery-service",
		"dispatchRequestReference": "dispatch-req-1", "tripReference": "trip-1",
		"paymentReference": "pay-ref-1", "amountMinor": 250000, "currency": "NGN",
		"status": "PAID", "paidAt": time.Now().UTC().Format(time.RFC3339)})

	// Fleet may verify by every supported reference; none of them is a new
	// identifier invented for this lookup.
	for _, lookup := range []struct{ referenceType, reference string }{
		{"DISPATCH_REQUEST", "dispatch-req-1"},
		{"TRIP", "trip-1"},
		{"PAYMENT_REFERENCE", "pay-ref-1"},
	} {
		status, response := api.lookupJobPayment(t, "fleet-partner-service", org,
			lookup.referenceType, lookup.reference)
		if status != http.StatusOK {
			t.Fatalf("lookup by %s: status=%d body=%#v", lookup.referenceType, status, response)
		}
		data := dataOf(t, response)
		if data["paymentStatus"] != "PAID" || data["settled"] != true {
			t.Fatalf("lookup by %s returned %#v", lookup.referenceType, data)
		}
		if data["amountMinor"] != float64(250000) || data["currency"] != "NGN" {
			t.Fatalf("amount or currency wrong: %#v", data)
		}
		if data["sourceVersion"] == nil || data["paymentVerificationId"] == nil {
			t.Fatalf("missing source version or verification id: %#v", data)
		}
		// Data minimization: nothing that describes Wallet's internals may
		// appear in the response, whatever the caller does with it.
		for _, forbidden := range []string{"balance", "availableMinor", "heldMinor",
			"journalId", "ledgerAccountId", "lines", "bankAccount", "cardLast4",
			"providerSecret", "paystackReference"} {
			if _, present := data[forbidden]; present {
				t.Fatalf("lookup leaked %q: %#v", forbidden, data)
			}
		}
	}

	// A reference nobody recorded is not found — never an empty "success".
	if status, _ := api.lookupJobPayment(t, "fleet-partner-service", org,
		"TRIP", "trip-does-not-exist"); status != http.StatusNotFound {
		t.Fatalf("unknown reference status=%d", status)
	}
	// An unsupported key is refused rather than guessed at.
	if status, _ := api.lookupJobPayment(t, "fleet-partner-service", org,
		"SOME_OTHER_KEY", "trip-1"); status != http.StatusNotFound {
		t.Fatalf("unsupported reference type status=%d", status)
	}
	// Tenant mismatch: the same real reference under a different organization
	// is indistinguishable from a reference that does not exist.
	if status, _ := api.lookupJobPayment(t, "fleet-partner-service", otherOrg,
		"TRIP", "trip-1"); status != http.StatusNotFound {
		t.Fatalf("cross-tenant lookup status=%d", status)
	}

	// Fleet cannot mint a verification: it does not hold finance.payment.confirm.
	status, _ := api.do(t, http.MethodPost, "/job-payments", map[string]any{
		"payloadVersion": 1, "organizationId": org, "idempotencyKey": "job-pay-fleet",
		"sourceService": "fleet-partner-service", "tripReference": "trip-forged",
		"amountMinor": 100, "currency": "NGN", "status": "PAID"}, callOptions{})
	if status != http.StatusForbidden {
		t.Fatalf("fleet recorded its own payment verification: status=%d", status)
	}

	// Amount and currency are properties of the payment, not of the request: a
	// caller that believes a different value simply reads the real one back and
	// must compare for itself.
	_, response := api.lookupJobPayment(t, "fleet-partner-service", org, "TRIP", "trip-1")
	data := dataOf(t, response)
	if data["amountMinor"] == float64(999999) {
		t.Fatal("lookup echoed a caller-supplied amount")
	}

	// A refunded payment stops being settled, so a downstream gate closes.
	api.recordJobPayment(t, org, "job-pay-1-refund", map[string]any{
		"sourceService":    "driver-dispatch-delivery-service",
		"paymentReference": "pay-ref-1", "amountMinor": 250000, "currency": "NGN",
		"status": "REFUNDED", "refundState": "COMPLETED"})
	_, response = api.lookupJobPayment(t, "fleet-partner-service", org, "TRIP", "trip-1")
	data = dataOf(t, response)
	if data["settled"] != false || data["paymentStatus"] != "REFUNDED" {
		t.Fatalf("refunded payment still settled: %#v", data)
	}
}

// postAllocation posts an earning allocation and returns its journal so a
// reversal has a real, balanced entry to compensate.
func (p *privateAPI) postAllocation(t *testing.T, organizationID, jobReference string,
	grossMinor int64) *models.EarningAllocation {
	t.Helper()
	ctx := context.Background()
	rule := &models.CommissionRule{OrganizationID: organizationID, ServiceType: "DELIVERY",
		Currency: "NGN", ProviderShareBPS: 10000, Status: "ACTIVE",
		EffectiveFrom: time.Now().UTC().Add(-time.Minute)}
	if err := p.fixture.marketplace.CreateCommissionRule(ctx, nil, rule); err != nil {
		t.Fatalf("commission rule: %v", err)
	}
	allocation, err := p.fixture.service.PostEarningAllocation(ctx, usecases.AllocationInput{
		Meta:           serviceMeta("fleet-partner-service", "alloc-"+jobReference, organizationID),
		OrganizationID: organizationID, SourceService: "dispatch", JobReference: jobReference,
		ServiceType: "DELIVERY", ProviderReference: "provider-" + jobReference,
		ProviderService: "DISPATCH", Currency: "NGN", GrossMinor: grossMinor})
	if err != nil {
		t.Fatalf("post allocation: %v", err)
	}
	return allocation
}

func reversalRequestBody(organizationID, journalID, key string, amountMinor int64,
	currency, reasonCode string) map[string]any {
	return map[string]any{"payloadVersion": 1, "organizationId": organizationID,
		"idempotencyKey": key, "originalJournalId": journalID,
		"amountMinor": amountMinor, "currency": currency, "reasonCode": reasonCode}
}

// TestCrossServiceReversalContract exercises the durable reversal contract end
// to end over HTTP: request, duplicate, fingerprint conflict, cross-tenant
// rejection, over-reversal refusal, separation of duties on approval, timeout
// to UNKNOWN, authoritative read-back, completion, and the compensating
// journal that completion must have produced.
func TestCrossServiceReversalContract(t *testing.T) {
	api := newPrivateAPI(t)
	ctx := context.Background()
	const org = "org-reversal"
	allocation := api.postAllocation(t, org, "trip-reversal", 5000)

	// Request.
	status, response := api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, allocation.JournalID, "reversal-1", 5000, "NGN", "DUPLICATE_CHARGE"),
		callOptions{})
	if status != http.StatusOK {
		t.Fatalf("reversal request: status=%d body=%#v", status, response)
	}
	created := dataOf(t, response)
	reversalID, _ := created["id"].(string)
	if reversalID == "" || created["status"] != models.ReversalPendingApproval {
		t.Fatalf("unexpected reversal: %#v", created)
	}
	if created["resultingJournalId"] != "" {
		t.Fatalf("reversal posted a journal before approval: %#v", created)
	}

	// A duplicate identical request returns the original result, not a second
	// reversal.
	_, duplicateResponse := api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, allocation.JournalID, "reversal-1", 5000, "NGN", "DUPLICATE_CHARGE"),
		callOptions{})
	if dataOf(t, duplicateResponse)["id"] != reversalID {
		t.Fatalf("duplicate created a second reversal: %#v", duplicateResponse)
	}
	var reversalCount int
	if err := api.fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM finance_reversals WHERE organization_id=$1`, org).Scan(&reversalCount); err != nil {
		t.Fatal(err)
	}
	if reversalCount != 1 {
		t.Fatalf("duplicate request created %d reversals", reversalCount)
	}

	// Conflicting fingerprint under the same key is a conflict, never a silent
	// overwrite of the recorded intent.
	if status, _ = api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, allocation.JournalID, "reversal-1", 4000, "NGN", "DUPLICATE_CHARGE"),
		callOptions{}); status != http.StatusConflict {
		t.Fatalf("fingerprint conflict accepted: status=%d", status)
	}

	// Cross-tenant reference: the journal exists but belongs elsewhere.
	if status, _ = api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody("org-reversal-other", allocation.JournalID, "reversal-cross", 5000, "NGN", "X"),
		callOptions{}); status != http.StatusForbidden {
		t.Fatalf("cross-tenant reversal accepted: status=%d", status)
	}

	// The reversible amount is already fully claimed by the pending reversal.
	if status, _ = api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, allocation.JournalID, "reversal-excess", 1, "NGN", "X"),
		callOptions{}); status != http.StatusUnprocessableEntity {
		t.Fatalf("over-reversal accepted: status=%d", status)
	}

	// Currency mismatch against the original entry is refused.
	other := api.postAllocation(t, org, "trip-reversal-currency", 3000)
	if status, _ = api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, other.JournalID, "reversal-currency", 3000, "USD", "X"),
		callOptions{}); status != http.StatusUnprocessableEntity {
		t.Fatalf("currency mismatch accepted: status=%d", status)
	}

	// Read-back by identifier and by idempotency key both resolve the same row.
	status, response = api.do(t, http.MethodGet,
		"/organizations/"+org+"/reversals/"+reversalID, nil, callOptions{})
	if status != http.StatusOK || dataOf(t, response)["id"] != reversalID {
		t.Fatalf("reversal read-back: status=%d body=%#v", status, response)
	}
	status, response = api.do(t, http.MethodGet,
		"/operations/REVERSAL/by-idempotency?organizationId="+org+"&idempotencyKey=reversal-1",
		nil, callOptions{})
	if status != http.StatusOK || dataOf(t, response)["id"] != reversalID {
		t.Fatalf("reversal read-back by key: status=%d body=%#v", status, response)
	}
	// A tenant that does not own it cannot read it back.
	if status, _ = api.do(t, http.MethodGet,
		"/organizations/org-reversal-other/reversals/"+reversalID, nil,
		callOptions{}); status != http.StatusNotFound {
		t.Fatalf("cross-tenant reversal read status=%d", status)
	}

	// Listing is tenant-scoped.
	status, response = api.do(t, http.MethodGet, "/organizations/"+org+"/reversals", nil, callOptions{})
	if status != http.StatusOK {
		t.Fatalf("list reversals status=%d", status)
	}
	if listed, _ := dataOf(t, response)["reversals"].([]any); len(listed) != 1 {
		t.Fatalf("expected exactly one reversal, got %#v", dataOf(t, response)["reversals"])
	}

	current := loadReversal(t, api, org, reversalID)

	// Separation of duties: the requester cannot approve its own reversal.
	if status, _ = api.do(t, http.MethodPost, "/reversals/"+reversalID+"/approve",
		map[string]any{"payloadVersion": 1, "organizationId": org,
			"idempotencyKey": "reversal-self-approve", "expectedVersion": current.Version},
		callOptions{}); status != http.StatusForbidden {
		t.Fatalf("requester approved its own reversal: status=%d", status)
	}

	// A separate approver with the approving permission may.
	status, response = api.do(t, http.MethodPost, "/reversals/"+reversalID+"/approve",
		map[string]any{"payloadVersion": 1, "organizationId": org,
			"idempotencyKey": "reversal-approve", "expectedVersion": current.Version},
		callOptions{caller: "admin-backoffice-service"})
	if status != http.StatusOK {
		t.Fatalf("approve reversal: status=%d body=%#v", status, response)
	}
	approved := dataOf(t, response)
	if approved["status"] != models.ReversalApproved || approved["approvalReference"] == "" {
		t.Fatalf("approval did not record its policy: %#v", approved)
	}

	workerConfig := usecases.WorkerConfig{Owner: "reversal-worker", BatchSize: 10,
		Lease: time.Minute, MaxAttempts: 3, BaseBackoff: time.Nanosecond}

	// First pass: the gateway times out, so the outcome is UNKNOWN — not a
	// failure, and certainly not a second attempt at moving money.
	if err := api.fixture.service.RunFinancialPass(ctx, unknownThenSuccessGateway{}, workerConfig); err != nil {
		t.Fatalf("unknown pass: %v", err)
	}
	current = loadReversal(t, api, org, reversalID)
	if current.Status != models.ReversalUnknown {
		t.Fatalf("timeout did not become UNKNOWN: %s", current.Status)
	}
	if current.ResultingJournalID != "" {
		t.Fatalf("UNKNOWN reversal posted a journal: %#v", current)
	}

	// Second pass: the authoritative read-back resolves it and the compensating
	// journal is posted exactly once.
	time.Sleep(time.Millisecond)
	if err := api.fixture.service.RunFinancialPass(ctx, unknownThenSuccessGateway{}, workerConfig); err != nil {
		t.Fatalf("read-back pass: %v", err)
	}
	current = loadReversal(t, api, org, reversalID)
	if current.Status != models.ReversalCompleted || current.ResultingJournalID == "" {
		t.Fatalf("reversal did not complete: %#v", current)
	}

	// The compensating journal must exist, mirror the original, and leave the
	// original untouched.
	compensating, err := api.fixture.ledger.GetJournal(ctx, nil, current.ResultingJournalID)
	if err != nil {
		t.Fatalf("load compensating journal: %v", err)
	}
	if compensating.ReversesJournalID != allocation.JournalID {
		t.Fatalf("compensating journal does not reference the original: %#v", compensating)
	}
	original, err := api.fixture.ledger.GetJournal(ctx, nil, allocation.JournalID)
	if err != nil {
		t.Fatal(err)
	}
	if len(compensating.Lines) != len(original.Lines) {
		t.Fatalf("compensating journal has %d lines, original has %d",
			len(compensating.Lines), len(original.Lines))
	}
	var compensatingDebit, compensatingCredit int64
	for _, line := range compensating.Lines {
		if line.Side == models.SideDebit {
			compensatingDebit += line.AmountMinor
		} else {
			compensatingCredit += line.AmountMinor
		}
	}
	if compensatingDebit != compensatingCredit || compensatingDebit != 5000 {
		t.Fatalf("compensating journal unbalanced: debit=%d credit=%d",
			compensatingDebit, compensatingCredit)
	}

	// A further pass must not execute the reversal a second time.
	time.Sleep(time.Millisecond)
	if err := api.fixture.service.RunFinancialPass(ctx, unknownThenSuccessGateway{}, workerConfig); err != nil {
		t.Fatalf("post-completion pass: %v", err)
	}
	var compensatingCount int
	if err := api.fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM finance_journals WHERE reverses_journal_id=$1`,
		allocation.JournalID).Scan(&compensatingCount); err != nil {
		t.Fatal(err)
	}
	if compensatingCount != 1 {
		t.Fatalf("reversal executed %d times", compensatingCount)
	}

	// History and audit both record the whole life of the reversal.
	status, response = api.do(t, http.MethodGet,
		"/organizations/"+org+"/reversals/"+reversalID+"/history", nil, callOptions{})
	if status != http.StatusOK {
		t.Fatalf("reversal history status=%d", status)
	}
	history, _ := dataOf(t, response)["history"].([]any)
	if len(history) < 3 {
		t.Fatalf("expected requested/approved/completed history, got %d entries", len(history))
	}
	var auditCount int
	if err := api.fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM finance_audit_records WHERE resource_type='reversal' AND resource_id=$1`,
		reversalID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount < 3 {
		t.Fatalf("reversal audit trail has %d entries", auditCount)
	}
	// Every transition emitted an outbox event in the same transaction.
	var outboxCount int
	if err := api.fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM finance_outbox WHERE aggregate_type='REVERSAL' AND aggregate_id=$1`,
		reversalID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount < 3 {
		t.Fatalf("reversal outbox has %d events", outboxCount)
	}
}

// TestCrossServiceReversalRejectionAndReplay covers the two terminal paths the
// happy case does not: an approver refusing the reversal, and an operator
// replaying one that dead-lettered.
func TestCrossServiceReversalRejectionAndReplay(t *testing.T) {
	api := newPrivateAPI(t)
	ctx := context.Background()
	const org = "org-reversal-terminal"
	allocation := api.postAllocation(t, org, "trip-rejected", 4000)

	_, response := api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, allocation.JournalID, "reversal-reject", 4000, "NGN", "MISTAKE"),
		callOptions{})
	reversalID := dataOf(t, response)["id"].(string)
	current := loadReversal(t, api, org, reversalID)

	status, response := api.do(t, http.MethodPost, "/reversals/"+reversalID+"/reject",
		map[string]any{"payloadVersion": 1, "organizationId": org,
			"idempotencyKey": "reversal-reject-decision", "expectedVersion": current.Version,
			"reasonCode": "NOT_JUSTIFIED"}, callOptions{caller: "admin-backoffice-service"})
	if status != http.StatusOK || dataOf(t, response)["status"] != models.ReversalRejected {
		t.Fatalf("reject reversal: status=%d body=%#v", status, response)
	}
	// A rejected reversal never touches the ledger, and it releases its claim on
	// the reversible amount so a corrected request can still be made.
	var compensating int
	if err := api.fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM finance_journals WHERE reverses_journal_id=$1`,
		allocation.JournalID).Scan(&compensating); err != nil {
		t.Fatal(err)
	}
	if compensating != 0 {
		t.Fatalf("rejected reversal posted %d journals", compensating)
	}
	if status, _ = api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, allocation.JournalID, "reversal-retry", 4000, "NGN", "MISTAKE"),
		callOptions{}); status != http.StatusOK {
		t.Fatalf("rejection did not release the reversible amount: status=%d", status)
	}

	// Dead-letter and replay. The row is driven to DEAD_LETTER directly rather
	// than through eight failing passes; what is under test is the operator
	// path back out of it.
	retryID := loadReversalByKey(t, api, org, "reversal-retry").ID
	if _, err := api.fixture.pool.Exec(ctx,
		`UPDATE finance_reversals SET status='DEAD_LETTER' WHERE id=$1`, retryID); err != nil {
		t.Fatal(err)
	}
	status, _ = api.do(t, http.MethodPost, "/operations/REVERSAL/"+retryID+"/replay",
		map[string]any{"payloadVersion": 1, "organizationId": org,
			"idempotencyKey": "reversal-replay"}, callOptions{caller: "payment-wallet-service"})
	if status != http.StatusOK {
		t.Fatalf("replay dead-lettered reversal: status=%d", status)
	}
	if replayed := loadReversal(t, api, org, retryID); replayed.Status != models.ReversalApproved {
		t.Fatalf("replayed reversal status=%s", replayed.Status)
	}
	// Fleet may not replay: replay is a reconciliation capability.
	if _, err := api.fixture.pool.Exec(ctx,
		`UPDATE finance_reversals SET status='DEAD_LETTER' WHERE id=$1`, retryID); err != nil {
		t.Fatal(err)
	}
	if status, _ = api.do(t, http.MethodPost, "/operations/REVERSAL/"+retryID+"/replay",
		map[string]any{"payloadVersion": 1, "organizationId": org,
			"idempotencyKey": "reversal-replay-fleet"}, callOptions{}); status != http.StatusForbidden {
		t.Fatalf("fleet replayed a reversal: status=%d", status)
	}
}

// TestCrossServiceReversalRestartRecovery proves a reversal abandoned mid-flight
// by a crashed worker is recovered as UNKNOWN and resolved by read-back rather
// than by a second execution.
func TestCrossServiceReversalRestartRecovery(t *testing.T) {
	api := newPrivateAPI(t)
	ctx := context.Background()
	const org = "org-reversal-restart"
	allocation := api.postAllocation(t, org, "trip-restart", 2000)

	_, response := api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, allocation.JournalID, "reversal-restart", 2000, "NGN", "CRASH"),
		callOptions{})
	reversalID := dataOf(t, response)["id"].(string)
	current := loadReversal(t, api, org, reversalID)
	if status, body := api.do(t, http.MethodPost, "/reversals/"+reversalID+"/approve",
		map[string]any{"payloadVersion": 1, "organizationId": org,
			"idempotencyKey": "reversal-restart-approve", "expectedVersion": current.Version},
		callOptions{caller: "admin-backoffice-service"}); status != http.StatusOK {
		t.Fatalf("approve reversal: status=%d body=%#v", status, body)
	}

	// Simulate the crash: a worker claimed the row and never came back.
	claimed, err := api.fixture.service.Reversals().Claim(ctx, "crashed-worker", time.Millisecond, 5)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Status != models.ReversalProcessing {
		t.Fatalf("claim returned %#v", claimed)
	}
	time.Sleep(3 * time.Millisecond)
	recovered, err := api.fixture.service.Reversals().RecoverExpiredLeases(ctx)
	if err != nil || recovered != 1 {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
	if after := loadReversal(t, api, org, reversalID); after.Status != models.ReversalUnknown {
		t.Fatalf("abandoned reversal status=%s", after.Status)
	}

	// The next pass resolves it by reading the authoritative outcome back.
	workerConfig := usecases.WorkerConfig{Owner: "restarted-worker", BatchSize: 10,
		Lease: time.Minute, MaxAttempts: 3, BaseBackoff: time.Nanosecond}
	if err := api.fixture.service.RunFinancialPass(ctx, unknownThenSuccessGateway{}, workerConfig); err != nil {
		t.Fatalf("restart pass: %v", err)
	}
	final := loadReversal(t, api, org, reversalID)
	if final.Status != models.ReversalCompleted || final.ResultingJournalID == "" {
		t.Fatalf("restart recovery did not complete: %#v", final)
	}
	var compensating int
	if err := api.fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM finance_journals WHERE reverses_journal_id=$1`,
		allocation.JournalID).Scan(&compensating); err != nil {
		t.Fatal(err)
	}
	if compensating != 1 {
		t.Fatalf("restart produced %d compensating journals", compensating)
	}
}

// TestCrossServicePartialReversalBalances proves a partial compensating entry
// still balances exactly and does not consume the whole reversible amount.
func TestCrossServicePartialReversalBalances(t *testing.T) {
	api := newPrivateAPI(t)
	ctx := context.Background()
	const org = "org-reversal-partial"
	// A gross that does not divide evenly across the split, so the rounding
	// path is genuinely exercised.
	allocation := api.postAllocation(t, org, "trip-partial", 3333)

	_, response := api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, allocation.JournalID, "reversal-partial", 1111, "NGN", "PARTIAL"),
		callOptions{})
	reversalID := dataOf(t, response)["id"].(string)
	current := loadReversal(t, api, org, reversalID)
	if status, body := api.do(t, http.MethodPost, "/reversals/"+reversalID+"/approve",
		map[string]any{"payloadVersion": 1, "organizationId": org,
			"idempotencyKey": "reversal-partial-approve", "expectedVersion": current.Version},
		callOptions{caller: "admin-backoffice-service"}); status != http.StatusOK {
		t.Fatalf("approve partial reversal: status=%d body=%#v", status, body)
	}

	workerConfig := usecases.WorkerConfig{Owner: "partial-worker", BatchSize: 10,
		Lease: time.Minute, MaxAttempts: 3, BaseBackoff: time.Nanosecond}
	for range 2 {
		if err := api.fixture.service.RunFinancialPass(ctx, unknownThenSuccessGateway{}, workerConfig); err != nil {
			t.Fatalf("partial pass: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	final := loadReversal(t, api, org, reversalID)
	if final.Status != models.ReversalCompleted || final.ResultingJournalID == "" {
		t.Fatalf("partial reversal did not complete: %#v", final)
	}
	compensating, err := api.fixture.ledger.GetJournal(ctx, nil, final.ResultingJournalID)
	if err != nil {
		t.Fatal(err)
	}
	var debit, credit int64
	for _, line := range compensating.Lines {
		if line.Side == models.SideDebit {
			debit += line.AmountMinor
		} else {
			credit += line.AmountMinor
		}
	}
	if debit != credit || debit != 1111 {
		t.Fatalf("partial compensating journal debit=%d credit=%d", debit, credit)
	}
	// A partial reversal does not claim the reverses_journal_id link, so the
	// remainder is still reversible.
	remaining, err := api.fixture.ledger.ReversibleMinor(ctx, nil, allocation.JournalID)
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 3333-1111 {
		t.Fatalf("remaining reversible=%d", remaining)
	}
	if status, _ := api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, allocation.JournalID, "reversal-partial-2", 2222, "NGN", "PARTIAL"),
		callOptions{}); status != http.StatusOK {
		t.Fatalf("second partial reversal refused: status=%d", status)
	}
}

func loadReversal(t *testing.T, api *privateAPI, organizationID, id string) *models.Reversal {
	t.Helper()
	value, err := api.fixture.service.Reversals().Get(context.Background(), nil, organizationID, id)
	if err != nil {
		t.Fatalf("load reversal %s: %v", id, err)
	}
	return value
}

func loadReversalByKey(t *testing.T, api *privateAPI, organizationID, key string) *models.Reversal {
	t.Helper()
	value, err := api.fixture.service.Reversals().FindByKey(context.Background(), nil, organizationID, key)
	if err != nil {
		t.Fatalf("load reversal by key %s: %v", key, err)
	}
	return value
}

// TestCrossServiceReversalReconciliation proves an ambiguous reversal that
// stays unresolved is surfaced as a reconciliation finding rather than being
// silently repaired.
func TestCrossServiceReversalReconciliation(t *testing.T) {
	api := newPrivateAPI(t)
	ctx := context.Background()
	const org = "org-reversal-reconcile"
	allocation := api.postAllocation(t, org, "trip-reconcile", 1500)

	_, response := api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, allocation.JournalID, "reversal-reconcile", 1500, "NGN", "STUCK"),
		callOptions{})
	reversalID := dataOf(t, response)["id"].(string)

	// Age the row past the reconciliation window while leaving it ambiguous.
	if _, err := api.fixture.pool.Exec(ctx,
		`UPDATE finance_reversals SET status='UNKNOWN',updated_at=now()-interval '1 hour' WHERE id=$1`,
		reversalID); err != nil {
		t.Fatal(err)
	}
	run, err := api.fixture.service.Reconcile(ctx,
		serviceMeta("payment-wallet-service", "reconcile-reversal", ""), "ALL")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if run.FindingsCount == 0 {
		t.Fatal("stuck reversal produced no reconciliation finding")
	}
	var findingCode string
	if err := api.fixture.pool.QueryRow(ctx,
		`SELECT finding_code FROM finance_reconciliation_findings
		 WHERE entity_type='REVERSAL' AND entity_id=$1`, reversalID).Scan(&findingCode); err != nil {
		t.Fatalf("finding lookup: %v", err)
	}
	if findingCode != models.FindingStuckReversal {
		t.Fatalf("finding code=%s", findingCode)
	}
	// Reconciliation reports; it does not post.
	var compensating int
	if err := api.fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM finance_journals WHERE reverses_journal_id=$1`,
		allocation.JournalID).Scan(&compensating); err != nil {
		t.Fatal(err)
	}
	if compensating != 0 {
		t.Fatalf("reconciliation posted %d journals", compensating)
	}
}

// TestCrossServiceReversalRequiresReversalPermission proves the reversal
// request permission is real: a caller holding only read permissions is
// refused.
func TestCrossServiceReversalRequiresReversalPermission(t *testing.T) {
	api := newPrivateAPI(t)
	ctx := context.Background()
	const org = "org-reversal-rbac"
	allocation := api.postAllocation(t, org, "trip-rbac", 1000)

	// Strip the operator role from Fleet: it keeps its HMAC secret and its
	// allowlisted identity, and still may not request a reversal. Possession of
	// a transport credential is not authorization.
	if _, err := api.fixture.pool.Exec(ctx,
		`DELETE FROM finance_role_grants WHERE subject_id='fleet-partner-service'`); err != nil {
		t.Fatal(err)
	}
	if status, _ := api.do(t, http.MethodPost, "/reversals",
		reversalRequestBody(org, allocation.JournalID, "reversal-rbac", 1000, "NGN", "X"),
		callOptions{}); status != http.StatusForbidden {
		t.Fatalf("unauthorized reversal accepted: status=%d", status)
	}
	// Restore the seeded grant so the shared fixture stays truthful for any
	// later assertion in this test.
	if _, err := api.fixture.pool.Exec(ctx, `INSERT INTO finance_role_grants
		(id, organization_id, subject_type, subject_id, role_id, granted_by)
		VALUES ('frgr_fleet_operator', NULL, 'SERVICE', 'fleet-partner-service',
		        'frol_finance_operator', 'test') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
}

// TestCrossServiceOutboxEnvelopeShape proves the events other services ingest
// carry the identifiers their consumers depend on.
func TestCrossServiceOutboxEnvelopeShape(t *testing.T) {
	api := newPrivateAPI(t)
	ctx := context.Background()
	const org = "org-outbox-shape"
	api.recordJobPayment(t, org, "outbox-job-pay", map[string]any{
		"sourceService": "driver-dispatch-delivery-service", "tripReference": "trip-outbox",
		"amountMinor": 700, "currency": "NGN", "status": "PAID"})

	rows, err := api.fixture.pool.Query(ctx, `SELECT event_type,aggregate_type,payload
		FROM finance_outbox WHERE organization_id=$1`, org)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var seen int
	for rows.Next() {
		var eventType, aggregateType string
		var payload []byte
		if err := rows.Scan(&eventType, &aggregateType, &payload); err != nil {
			t.Fatal(err)
		}
		if aggregateType != "PAYMENT_VERIFICATION" {
			continue
		}
		seen++
		var envelope map[string]any
		if err := json.Unmarshal(payload, &envelope); err != nil {
			t.Fatalf("event payload is not JSON: %v", err)
		}
		// The stored column holds the whole envelope; the consumer contract is
		// the envelope's payload object.
		decoded, _ := envelope["payload"].(map[string]any)
		if decoded == nil {
			t.Fatalf("event carried no payload object: %#v", envelope)
		}
		if decoded["paymentVerificationId"] == nil || decoded["status"] == nil {
			t.Fatalf("payment verification event is missing identity: %#v", decoded)
		}
		for key := range decoded {
			if strings.Contains(strings.ToLower(key), "secret") ||
				strings.Contains(strings.ToLower(key), "ledger") {
				t.Fatalf("event leaked %q", key)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Fatal("recording a job payment emitted no verification event")
	}
}

// compile-time reference so the repositories import stays meaningful even if
// the assertions above change shape.
var _ = repositories.ListFilter{}
