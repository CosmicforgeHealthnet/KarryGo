package integration_test

// FLEET-13.11: the managed-settlement route as Fleet actually calls it.
//
// The other managed-settlement tests drive the repository directly. This one
// goes through the production gin route and HMAC middleware with the exact
// body Fleet's posting adapter sends (financeops.ManagedSettlementSubmission
// payload plus the adapter's envelope fields), signed the way Fleet's
// platform client signs, so a drift in field names, casing, the source and
// provider identity Fleet maps to, or the caller identity is caught here.
// Fleet pins the same body from its side in
// services/fleet-partner-service/internal/integration/managed_settlement_delivery_test.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	wallethttp "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/http"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/managedsettlement"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/walletclient"
)

const fleetWireSecret = "fleet-wire-test-secret"

func fleetWireRouter(t *testing.T, repo *walletrepositories.PostgresWalletRepository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	service := walletusecases.NewWalletService(walletusecases.Options{Repository: repo})
	wallethttp.RegisterRoutes(router.Group("/api/v1/payment-wallet"), service,
		[]byte("customer-secret"), map[string][]byte{}, serviceauth.Secrets{
			"fleet-partner-service": []byte(fleetWireSecret),
			"driver-taxi-service":   []byte("taxi-secret"),
		})
	return router
}

func fleetWireCall(t *testing.T, router *gin.Engine, caller, secret, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(request, caller, []byte(secret), payload, time.Time{}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder.Code, decoded
}

// fleetWireBody is the body Fleet's adapter sends, field for field.
func fleetWireBody(source, providerType, jobID string, gross, provider, organization int64) map[string]any {
	return map[string]any{
		// financeops.ManagedSettlementSubmission.payload()
		"instruction_id": "mwsi_" + jobID, "instruction_version": 1,
		"organization_id": "org_wire", "membership_id": "mem_wire",
		"provider_id": "prov_wire", "provider_type": providerType,
		"source_service": source, "source_job_id": jobID, "source_assignment_id": "asg_" + jobID,
		"currency": "NGN", "gross_basis_minor": gross,
		"provider_share_minor": provider, "organization_share_minor": organization,
		"compensation_plan_id": "cplan_wire", "compensation_plan_version": 1,
		"compensation_resolution_version": 1, "correlation_id": "corr_" + jobID,
		"idempotency_key": "settlement:" + jobID,
		// walletPostingAdapter envelope fields, ignored by this contract.
		"payloadVersion": 1, "organizationId": "org_wire",
		"idempotencyKey": "settlement:" + jobID, "expectedVersion": 0,
	}
}

func TestFleetWireBodyPostsThroughTheProductionRoute(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	router := fleetWireRouter(t, repo)
	const base = "/api/v1/payment-wallet/internal/finance/v1/managed-settlements"

	cases := []struct {
		source, providerType, ownerID string
	}{
		{"taxi", "taxi", "taxi:prov_wire"},
		{"dispatch", "dispatch", "dispatch:prov_wire"},
		// Hauling providers read their balance under truck_provider.
		{"hauling", "truck_provider", "truck_provider:prov_wire"},
	}
	for _, c := range cases {
		jobID := "job_wire_" + c.source
		heldJob(t, pool, c.source, jobID, 10000, 1000)
		body := fleetWireBody(c.source, c.providerType, jobID, 9000, 1500, 7500)

		status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, base, body)
		if status != http.StatusOK {
			t.Fatalf("%s: post status=%d body=%v", c.source, status, response)
		}
		data, _ := response["data"].(map[string]any)
		id, _ := data["id"].(string)
		if data["status"] != "POSTED" || id == "" || data["instruction_version"].(float64) != 1 {
			t.Fatalf("%s: response data=%v", c.source, data)
		}
		if got := accountBalanceMinor(t, pool, "provider", c.ownerID, "provider_payable"); got != 1500 {
			t.Fatalf("%s: provider payable=%d, want 1500", c.source, got)
		}

		// A retry of the same body (Fleet after a lost response) returns the
		// original posting.
		status, response = fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, base, body)
		if again, _ := response["data"].(map[string]any); status != http.StatusOK || again["id"] != id {
			t.Fatalf("%s: retry status=%d data=%v", c.source, status, response)
		}

		// Fleet's read-back route.
		status, response = fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodGet, base+"/by-key/settlement:"+jobID, nil)
		if read, _ := response["data"].(map[string]any); status != http.StatusOK || read["id"] != id || read["status"] != "POSTED" {
			t.Fatalf("%s: read back status=%d data=%v", c.source, status, response)
		}
	}
	if got := journalCount(t, pool, "managed_work_settlement"); got != len(cases) {
		t.Fatalf("journals=%d, want one per instruction (%d)", got, len(cases))
	}
	assertEveryJournalBalances(t, pool)

	// Fleet's read-back of a key Wallet never saw answers 404, which Fleet
	// treats as "safe to send", never as "posted".
	if status, _ := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodGet, base+"/by-key/settlement:never-sent", nil); status != http.StatusNotFound {
		t.Fatalf("unknown key read back status=%d, want 404", status)
	}
	// Only Fleet may instruct a settlement.
	jobID := "job_wire_foreign"
	heldJob(t, pool, "taxi", jobID, 10000, 1000)
	if status, _ := fleetWireCall(t, router, "driver-taxi-service", "taxi-secret", http.MethodPost, base, fleetWireBody("taxi", "taxi", jobID, 9000, 1500, 7500)); status != http.StatusForbidden {
		t.Fatalf("execution service instructing a settlement: status=%d, want 403", status)
	}
	// An unsigned call never reaches the handler.
	if status, _ := fleetWireCall(t, router, "fleet-partner-service", "wrong-secret", http.MethodPost, base, fleetWireBody("taxi", "taxi", jobID, 9000, 1500, 7500)); status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Fatalf("badly signed call: status=%d", status)
	}
}

// ---------------------------------------------------------------------------
// FLEET-13.11: settlement identity end to end.
//
// Everything above seeds payment intents under the canonical names Fleet
// settles with ("taxi", "dispatch", "hauling"). No execution service creates
// them that way. Taxi pays as "taxi-service", Dispatch as
// "dispatch-delivery-service" keyed by the BOOKING (while it settles by the
// TRIP), Hauling as "driver-hauling-service". The tests below create each
// payment exactly as its execution service does -- through the production
// route, with the shared wallet client, under the service's own HMAC name,
// source spelling, reference shape, provider type and idempotency key -- and
// then post Fleet's exact instruction body against it.
// ---------------------------------------------------------------------------

const identityPlatformFeeBPS = 1000 // 10%, so the platform's share is visibly checked.

// executionSecrets are the HMAC identities the three execution services sign
// with, as PAYMENT_WALLET_SERVICE_SECRETS provisions them.
var executionSecrets = map[string]string{
	"taxi-service":              "taxi-wire-secret",
	"dispatch-delivery-service": "dispatch-wire-secret",
	"driver-hauling-service":    "hauling-wire-secret",
}

func identityRouter(t *testing.T, repo *walletrepositories.PostgresWalletRepository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	service := walletusecases.NewWalletService(walletusecases.Options{
		Repository: repo, PlatformFeeBPS: identityPlatformFeeBPS,
	})
	secrets := serviceauth.Secrets{"fleet-partner-service": []byte(fleetWireSecret)}
	for name, secret := range executionSecrets {
		secrets[name] = []byte(secret)
	}
	wallethttp.RegisterRoutes(router.Group("/api/v1/payment-wallet"), service,
		[]byte("customer-secret"), map[string][]byte{}, secrets)
	return router
}

// executionWallet is the wallet client an execution service is configured
// with: the same shared client, its own service name and secret.
func executionWallet(server *httptest.Server, serviceName string) walletclient.Client {
	return walletclient.Client{BaseURL: server.URL, ServiceName: serviceName, Secret: []byte(executionSecrets[serviceName])}
}

// executionReader is the read-only settlement client an execution service
// reconciles its own completions with.
func executionReader(server *httptest.Server, serviceName string) managedsettlement.Client {
	return managedsettlement.Client{BaseURL: server.URL, ServiceName: serviceName, Secret: []byte(executionSecrets[serviceName])}
}

// executionJob is one job's money, held the way its execution service holds it.
type executionJob struct {
	canonical    string // settlement source, as Fleet sends it
	providerType string // provider namespace Fleet sends
	walletName   string // the execution service's HMAC identity and payment spelling
	jobID        string // the execution service's own job id (Fleet's source_job_id)
	paymentRef   string // the reference the payment intent was created under
	intent       walletclient.PaymentIntent
}

// holdTaxi mirrors driver-taxi-service createPaymentIntentForCandidate: one
// intent per offered candidate, keyed by the booking, provider attached.
func holdTaxi(t *testing.T, ctx context.Context, server *httptest.Server, repo *walletrepositories.PostgresWalletRepository, amount int64) executionJob {
	t.Helper()
	bookingID, customerID, providerID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	fundCustomer(t, repo, customerID, amount)
	wallet := executionWallet(server, "taxi-service")
	intent, err := wallet.CreatePaymentIntent(ctx, walletclient.PaymentIntentRequest{
		SourceService: "taxi-service", SourceReference: bookingID, CustomerID: customerID,
		ProviderID: providerID, ProviderType: "taxi", AmountKobo: amount, Currency: "NGN",
		PaymentMethod: "wallet", IdempotencyKey: "taxi-intent-" + bookingID + "-" + providerID,
	})
	if err != nil {
		t.Fatalf("taxi create intent: %v", err)
	}
	if intent, err = wallet.PayFromWallet(ctx, intent.ID, "taxi-pay-"+bookingID); err != nil {
		t.Fatalf("taxi hold: %v", err)
	}
	return executionJob{canonical: "taxi", providerType: "taxi", walletName: "taxi-service",
		jobID: bookingID, paymentRef: bookingID, intent: intent}
}

// holdDispatch mirrors driver-dispatch-delivery-service booking payments: the
// intent is keyed by the BOOKING and the rider is attached on acceptance; the
// mission that completes -- and that Fleet settles -- is the TRIP.
func holdDispatch(t *testing.T, ctx context.Context, server *httptest.Server, repo *walletrepositories.PostgresWalletRepository, amount int64) executionJob {
	t.Helper()
	bookingID, tripID, customerID, riderID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	fundCustomer(t, repo, customerID, amount)
	wallet := executionWallet(server, "dispatch-delivery-service")
	intent, err := wallet.CreatePaymentIntent(ctx, walletclient.PaymentIntentRequest{
		SourceService: "dispatch-delivery-service", SourceReference: bookingID, CustomerID: customerID,
		ProviderType: "dispatch", AmountKobo: amount, Currency: "NGN",
		PaymentMethod: "wallet", IdempotencyKey: "dispatch-hold-" + bookingID,
	})
	if err != nil {
		t.Fatalf("dispatch create intent: %v", err)
	}
	if intent, err = wallet.PayFromWallet(ctx, intent.ID, "dispatch-pay-"+bookingID); err != nil {
		t.Fatalf("dispatch hold: %v", err)
	}
	if intent, err = wallet.AssignProvider(ctx, intent.Reference, "dispatch", riderID); err != nil {
		t.Fatalf("dispatch assign rider: %v", err)
	}
	return executionJob{canonical: "dispatch", providerType: "dispatch", walletName: "dispatch-delivery-service",
		jobID: tripID, paymentRef: bookingID, intent: intent}
}

// holdHauling mirrors driver-hauling-service HoldFromWallet: keyed by the
// booking, provider namespace truck_provider.
func holdHauling(t *testing.T, ctx context.Context, server *httptest.Server, repo *walletrepositories.PostgresWalletRepository, amount int64) executionJob {
	t.Helper()
	bookingID, customerID, providerID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	fundCustomer(t, repo, customerID, amount)
	wallet := executionWallet(server, "driver-hauling-service")
	intent, err := wallet.CreatePaymentIntent(ctx, walletclient.PaymentIntentRequest{
		SourceService: "driver-hauling-service", SourceReference: bookingID, CustomerID: customerID,
		ProviderID: providerID, ProviderType: "truck_provider", AmountKobo: amount, Currency: "NGN",
		PaymentMethod: "wallet", IdempotencyKey: "hauling-hold-" + bookingID,
	})
	if err != nil {
		t.Fatalf("hauling create intent: %v", err)
	}
	if intent, err = wallet.PayFromWallet(ctx, intent.ID, "hauling-pay-"+bookingID); err != nil {
		t.Fatalf("hauling hold: %v", err)
	}
	return executionJob{canonical: "hauling", providerType: "truck_provider", walletName: "driver-hauling-service",
		jobID: bookingID, paymentRef: bookingID, intent: intent}
}

// fleetBodyFor is Fleet's exact instruction body for one job. The payment
// reference is sent only when it differs from the job id, exactly as Fleet's
// financeops payload does (and only then, so earlier instructions keep their
// byte-identical payload and fingerprint).
func fleetBodyFor(job executionJob, provider, organization int64) map[string]any {
	gross := provider + organization
	body := fleetWireBody(job.canonical, job.providerType, job.jobID, gross, provider, organization)
	if job.paymentRef != job.jobID {
		body["payment_source_reference"] = job.paymentRef
	}
	return body
}

func requestStatus(err error) int {
	var requestErr *walletclient.RequestError
	if errors.As(err, &requestErr) {
		return requestErr.Status
	}
	return 0
}

const managedSettlementsPath = "/api/v1/payment-wallet/internal/finance/v1/managed-settlements"

func TestFleetSettlesPaymentsExactlyAsExecutionServicesCreatedThem(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	router := identityRouter(t, repo)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	ctx := context.Background()

	const amount = 20000 // platform fee 2,000 at 10%; 18,000 distributable
	jobs := []executionJob{
		holdTaxi(t, ctx, server, repo, amount),
		holdDispatch(t, ctx, server, repo, amount),
		holdHauling(t, ctx, server, repo, amount),
	}
	for _, job := range jobs {
		if job.intent.Status != "held" || job.intent.PlatformFeeKobo != 2000 {
			t.Fatalf("%s: intent not held as expected: %+v", job.canonical, job.intent)
		}
		escrowOwner := job.walletName + ":" + job.paymentRef
		if got := accountBalanceMinor(t, pool, "job", escrowOwner, "job_escrow"); got != amount {
			t.Fatalf("%s: escrow %s holds %d, want %d", job.canonical, escrowOwner, got, amount)
		}

		body := fleetBodyFor(job, 4500, 13500)
		status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, body)
		if status != http.StatusOK {
			t.Fatalf("%s: post status=%d body=%v", job.canonical, status, response)
		}
		data, _ := response["data"].(map[string]any)
		settlementID, _ := data["id"].(string)
		if data["status"] != "POSTED" || settlementID == "" ||
			data["payment_intent_id"] != job.intent.ID ||
			data["payment_source_service"] != job.walletName ||
			data["payment_source_reference"] != job.paymentRef ||
			data["source_service"] != job.canonical || data["source_job_id"] != job.jobID {
			t.Fatalf("%s: posted settlement does not bind the payment: %v", job.canonical, data)
		}

		// One balanced journal released exactly the escrow the execution
		// service funded: nothing left in it, nothing invented elsewhere.
		if got := accountBalanceMinor(t, pool, "job", job.walletName+":"+job.paymentRef, "job_escrow"); got != 0 {
			t.Fatalf("%s: escrow left %d after release", job.canonical, got)
		}
		if got := accountBalanceMinor(t, pool, "provider", job.providerType+":prov_wire", "provider_payable"); got != 4500 {
			t.Fatalf("%s: provider payable=%d, want 4500", job.canonical, got)
		}
		var mode, intentStatus string
		if err := pool.QueryRow(ctx, `SELECT settlement_mode, status FROM payment_intents WHERE id=$1`, job.intent.ID).Scan(&mode, &intentStatus); err != nil {
			t.Fatal(err)
		}
		if mode != "ORGANIZATION_MANAGED" || intentStatus != "completed" {
			t.Fatalf("%s: intent mode=%s status=%s", job.canonical, mode, intentStatus)
		}

		// Fleet's retry after a lost response returns the original posting.
		status, response = fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, body)
		if again, _ := response["data"].(map[string]any); status != http.StatusOK || again["id"] != settlementID {
			t.Fatalf("%s: retry status=%d data=%v", job.canonical, status, response)
		}

		// The execution service, reconciling its own completion under the
		// identity its handoff derives (canonical name + its own job id),
		// sees this very settlement.
		outcome, err := executionReader(server, job.walletName).Lookup(ctx,
			managedsettlement.IdentityFor(job.canonical, job.jobID))
		if err != nil || !outcome.Settled || len(outcome.Settlements) != 1 ||
			outcome.Settlements[0].InstructionID != "mwsi_"+job.jobID {
			t.Fatalf("%s: execution service lookup outcome=%+v err=%v", job.canonical, outcome, err)
		}
		// Asked under its payment spelling, it gets the same answer.
		outcome, err = executionReader(server, job.walletName).ForJob(ctx, job.walletName, job.jobID)
		if err != nil || !outcome.Settled {
			t.Fatalf("%s: lookup by payment spelling outcome=%+v err=%v", job.canonical, outcome, err)
		}

		// The legacy direct-payout path, called by the execution service with
		// its own payment identity, cannot pay the same job again: the payment
		// is already completed, so the call is the idempotent no-op and moves
		// nothing.
		completed, err := executionWallet(server, job.walletName).CompleteJobAt(ctx, job.walletName, job.paymentRef, time.Now())
		if err != nil || completed.Status != "completed" {
			t.Fatalf("%s: legacy CompleteJob after managed settlement: intent=%+v err=%v", job.canonical, completed, err)
		}
		if got := accountBalanceMinor(t, pool, "provider", job.providerType+":prov_wire", "provider_payable"); got != 4500 {
			t.Fatalf("%s: legacy CompleteJob moved money: provider payable=%d", job.canonical, got)
		}
	}
	if got := journalCount(t, pool, "managed_work_settlement"); got != len(jobs) {
		t.Fatalf("journals=%d, want exactly one per job (%d)", got, len(jobs))
	}
	if got := journalCount(t, pool, "job_settlement"); got != 0 {
		t.Fatalf("legacy job_settlement journals=%d, want 0", got)
	}
	assertEveryJournalBalances(t, pool)
}

// Fleet delivers with bounded concurrency and may race a retry against the
// first send. However many arrive at once, one journal is posted.
func TestConcurrentFleetSendsAgainstARealDispatchPaymentPostOnce(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	router := identityRouter(t, repo)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	job := holdDispatch(t, context.Background(), server, repo, 10000)
	body := fleetBodyFor(job, 0, 9000) // salaried rider: the organization keeps the job share

	var wg sync.WaitGroup
	ids := make(chan string, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, body)
			if status != http.StatusOK {
				t.Errorf("concurrent send status=%d body=%v", status, response)
				return
			}
			data, _ := response["data"].(map[string]any)
			id, _ := data["id"].(string)
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	var first string
	for id := range ids {
		if first == "" {
			first = id
		} else if id != first {
			t.Fatalf("concurrent sends produced two settlements: %s and %s", first, id)
		}
	}
	if got := journalCount(t, pool, "managed_work_settlement"); got != 1 {
		t.Fatalf("journals=%d, want 1", got)
	}
	// A salaried rider gets no provider line at all.
	if got := accountBalanceMinor(t, pool, "provider", "dispatch:prov_wire", "provider_payable"); got != 0 {
		t.Fatalf("salaried rider credited %d", got)
	}
	assertEveryJournalBalances(t, pool)
}

// A Dispatch instruction naming only the trip cannot find the booking's
// payment. That is the failure every Dispatch delivery hit before the payment
// reference travelled: refused, nothing moved, and still settleable once the
// correct reference arrives under a new instruction.
func TestDispatchTripWithoutItsPaymentReferenceIsRefusedAndMovesNothing(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	router := identityRouter(t, repo)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	job := holdDispatch(t, context.Background(), server, repo, 10000)

	body := fleetWireBody(job.canonical, job.providerType, job.jobID, 9000, 1500, 7500)
	if status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, body); status != http.StatusNotFound {
		t.Fatalf("trip-only dispatch instruction status=%d body=%v, want 404", status, response)
	}
	if got := accountBalanceMinor(t, pool, "job", job.walletName+":"+job.paymentRef, "job_escrow"); got != 10000 {
		t.Fatalf("refused instruction moved money: escrow=%d", got)
	}
	if got := journalCount(t, pool, "managed_work_settlement"); got != 0 {
		t.Fatalf("refused instruction posted %d journals", got)
	}

	// A wrong payment reference is refused the same way, not matched loosely.
	wrong := fleetBodyFor(job, 1500, 7500)
	wrong["instruction_id"], wrong["idempotency_key"] = "mwsi_wrong_"+job.jobID, "settlement:wrong:"+job.jobID
	wrong["payment_source_reference"] = uuid.NewString()
	if status, _ := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, wrong); status != http.StatusNotFound {
		t.Fatalf("wrong payment reference status=%d, want 404", status)
	}

	corrected := fleetBodyFor(job, 1500, 7500)
	corrected["instruction_version"] = 2
	corrected["idempotency_key"] = "settlement:v2:" + job.jobID
	if status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, corrected); status != http.StatusOK {
		t.Fatalf("corrected instruction status=%d body=%v", status, response)
	}
	assertEveryJournalBalances(t, pool)
}

// One booking can produce more than one trip. Both trips resolve to the one
// payment, and only the first may release it.
func TestASecondTripOfTheSameBookingCannotSettleThePaymentAgain(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	router := identityRouter(t, repo)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	job := holdDispatch(t, context.Background(), server, repo, 10000)

	if status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, fleetBodyFor(job, 1500, 7500)); status != http.StatusOK {
		t.Fatalf("first trip status=%d body=%v", status, response)
	}
	second := job
	second.jobID = uuid.NewString()
	if status, _ := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, fleetBodyFor(second, 1500, 7500)); status != http.StatusConflict {
		t.Fatalf("second trip of the same booking status=%d, want 409", status)
	}
	if got := journalCount(t, pool, "managed_work_settlement"); got != 1 {
		t.Fatalf("journals=%d, want 1", got)
	}
}

// Two spellings of one service holding a payment under the same reference is
// a state no instruction may resolve by choosing: it is refused, and so is the
// marker.
func TestAnAmbiguousPaymentIdentityIsRefusedNotPicked(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	router := identityRouter(t, repo)
	jobID := uuid.NewString()
	heldJob(t, pool, "taxi-service", jobID, 10000, 1000)
	heldJob(t, pool, "driver-taxi-service", jobID, 10000, 1000)

	status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath,
		fleetWireBody("taxi", "taxi", jobID, 9000, 1500, 7500))
	if status != http.StatusConflict {
		t.Fatalf("ambiguous identity status=%d body=%v, want 409", status, response)
	}
	if got := journalCount(t, pool, "managed_work_settlement"); got != 0 {
		t.Fatalf("ambiguous identity posted %d journals", got)
	}
	if status, _ := fleetWireCall(t, router, "taxi-service", executionSecrets["taxi-service"], http.MethodPut,
		"/api/v1/payment-wallet/internal/finance/v1/managed-jobs/TAXI/"+jobID, nil); status != http.StatusConflict {
		t.Fatalf("ambiguous marker status=%d, want 409", status)
	}
}

// A settlement is named canonically. An instruction naming a payment spelling
// would store a settlement no execution service could find, so it is refused.
func TestAnInstructionMustNameTheCanonicalService(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	router := identityRouter(t, repo)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	job := holdTaxi(t, context.Background(), server, repo, 10000)
	for _, source := range []string{"taxi-service", "driver-taxi-service", "vendor"} {
		body := fleetBodyFor(job, 1500, 7500)
		body["source_service"] = source
		if status, _ := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, body); status != http.StatusUnprocessableEntity {
			t.Fatalf("source %q status=%d, want 422", source, status)
		}
	}
	// The canonical name in any case is accepted.
	body := fleetBodyFor(job, 1500, 7500)
	body["source_service"] = "TAXI"
	if status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, body); status != http.StatusOK {
		t.Fatalf("upper-case canonical status=%d body=%v", status, response)
	}
}

// The execution service's own marker addresses the job the same way a
// settlement does, closes the legacy path on the real payment, and refuses a
// job it cannot find instead of reporting success.
func TestExecutionServiceMarkerFindsTheRealPayment(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	router := identityRouter(t, repo)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	ctx := context.Background()
	const marker = "/api/v1/payment-wallet/internal/finance/v1/managed-jobs/"

	for _, job := range []executionJob{
		holdTaxi(t, ctx, server, repo, 10000),
		holdDispatch(t, ctx, server, repo, 10000),
		holdHauling(t, ctx, server, repo, 10000),
	} {
		canonical, _ := managedsettlement.CanonicalSource(job.canonical)
		// Marked under the canonical name and the payment's own reference.
		if status, response := fleetWireCall(t, router, job.walletName, executionSecrets[job.walletName], http.MethodPut,
			marker+canonical+"/"+job.paymentRef, nil); status != http.StatusOK {
			t.Fatalf("%s: mark status=%d body=%v", job.canonical, status, response)
		}
		// Marking again, under the payment spelling, is the same no-op.
		if status, _ := fleetWireCall(t, router, job.walletName, executionSecrets[job.walletName], http.MethodPut,
			marker+job.walletName+"/"+job.paymentRef, nil); status != http.StatusOK {
			t.Fatalf("%s: re-mark status=%d", job.canonical, status)
		}
		var mode string
		if err := pool.QueryRow(ctx, `SELECT settlement_mode FROM payment_intents WHERE id=$1`, job.intent.ID).Scan(&mode); err != nil || mode != "ORGANIZATION_MANAGED" {
			t.Fatalf("%s: marker did not reach the payment: mode=%q err=%v", job.canonical, mode, err)
		}
		// The sweep sees a managed job still holding money...
		stuck, err := repo.FindStuckManagedJobs(ctx, time.Now().Add(time.Hour), 100)
		if err != nil || !containsStuck(stuck, job.intent.ID) {
			t.Fatalf("%s: marked unsettled job not reported stuck: %v %v", job.canonical, stuck, err)
		}
		// ...the legacy path refuses it...
		if _, err := executionWallet(server, job.walletName).CompleteJobAt(ctx, job.walletName, job.paymentRef, time.Now()); requestStatus(err) != http.StatusConflict {
			t.Fatalf("%s: legacy CompleteJob on a marked job: err=%v, want 409", job.canonical, err)
		}
		// ...Fleet's settlement still posts against it...
		if status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, fleetBodyFor(job, 1500, 7500)); status != http.StatusOK {
			t.Fatalf("%s: settle marked job status=%d body=%v", job.canonical, status, response)
		}
		// ...and once settled it is no longer stuck.
		stuck, err = repo.FindStuckManagedJobs(ctx, time.Now().Add(time.Hour), 100)
		if err != nil || containsStuck(stuck, job.intent.ID) {
			t.Fatalf("%s: settled job still reported stuck: %v %v", job.canonical, stuck, err)
		}
	}

	// A job with no payment is refused, not silently "marked".
	if status, _ := fleetWireCall(t, router, "taxi-service", executionSecrets["taxi-service"], http.MethodPut,
		marker+"TAXI/"+uuid.NewString(), nil); status != http.StatusNotFound {
		t.Fatalf("marking an unknown job status=%d, want 404", status)
	}
}

func containsStuck(stuck []walletrepositories.StuckManagedJob, intentID string) bool {
	for _, job := range stuck {
		if job.PaymentIntentID == intentID {
			return true
		}
	}
	return false
}

// A reversal returns the money to the escrow it came from -- the payment's,
// named by the execution service's own spelling -- not to an escrow named
// after the settlement's canonical identity, which never held it.
func TestReversingARealPaymentSettlementRestoresItsOwnEscrow(t *testing.T) {
	pool := walletLedgerPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	router := identityRouter(t, repo)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	job := holdDispatch(t, context.Background(), server, repo, 10000)

	if status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost, managedSettlementsPath, fleetBodyFor(job, 1500, 7500)); status != http.StatusOK {
		t.Fatalf("post status=%d body=%v", status, response)
	}
	status, response := fleetWireCall(t, router, "fleet-partner-service", fleetWireSecret, http.MethodPost,
		managedSettlementsPath+"/mwsi_"+job.jobID+"/1/reverse", map[string]any{"reason_code": "COMPENSATION_CORRECTED"})
	if status != http.StatusOK {
		t.Fatalf("reverse status=%d body=%v", status, response)
	}
	if got := accountBalanceMinor(t, pool, "job", job.walletName+":"+job.paymentRef, "job_escrow"); got != 10000 {
		t.Fatalf("payment escrow after reversal=%d, want 10000", got)
	}
	if got := accountBalanceMinor(t, pool, "job", job.canonical+":"+job.jobID, "job_escrow"); got != 0 {
		t.Fatalf("reversal credited an escrow named after the settlement: %d", got)
	}
	if got := accountBalanceMinor(t, pool, "provider", "dispatch:prov_wire", "provider_payable"); got != 0 {
		t.Fatalf("provider payable after reversal=%d, want 0", got)
	}
	assertEveryJournalBalances(t, pool)
}

// Migration 000023 must remove exactly what it added, and putting it back must
// re-bind settlements that posted before it existed.
func TestPaymentBindingMigrationRollsBackAndBackfills(t *testing.T) {
	pool := walletLedgerPool(t)
	ctx := context.Background()
	t.Cleanup(func() {
		if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
			t.Errorf("reapply after rollback: %v", err)
		}
	})
	column := func() bool {
		var found bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_name='managed_settlements' AND column_name='payment_intent_id')`).Scan(&found); err != nil {
			t.Fatal(err)
		}
		return found
	}
	if !column() {
		t.Fatal("000023 is not applied")
	}
	migrations, err := database.ListMigrations(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	for i := len(migrations) - 1; i >= 0 && migrations[i].Number > 23; i-- {
		if err = database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
			t.Fatalf("rollback %s: %v", migrations[i].Version, err)
		}
	}
	if err = database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("rollback 000023: %v", err)
	}
	if column() {
		t.Fatal("rolling back 000023 left payment_intent_id behind")
	}
	if !exists(t, pool, `SELECT to_regclass('public.managed_settlements') IS NOT NULL`) {
		t.Fatal("rolling back 000023 removed something it does not own")
	}

	// A settlement posted under the pre-binding schema, by exact identity.
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	jobID := uuid.NewString()
	intentID := heldJob(t, pool, "taxi", jobID, 10000, 1000)
	if _, err := pool.Exec(ctx, `
		INSERT INTO managed_settlements (id, instruction_id, instruction_version, organization_id, membership_id,
			provider_id, provider_type, source_service, source_job_id, currency, gross_basis_minor,
			provider_share_minor, organization_share_minor, platform_share_minor, status,
			idempotency_key, request_fingerprint)
		VALUES ($1,'mwsi_legacy',1,'org','mem','prov','taxi','taxi',$2,'NGN',9000,1500,7500,1000,'POSTED',
			'settlement:legacy','fp')`, uuid.NewString(), jobID); err != nil {
		t.Fatalf("seed pre-binding settlement: %v", err)
	}
	if err = database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("reapply: %v", err)
	}
	settlement, err := repo.GetManagedSettlementByInstruction(ctx, "mwsi_legacy", 1)
	if err != nil {
		t.Fatal(err)
	}
	if settlement.PaymentIntentID != intentID || settlement.PaymentSourceService != "taxi" || settlement.PaymentSourceReference != jobID {
		t.Fatalf("backfill did not bind the legacy settlement: %+v", settlement)
	}
}

func exists(t *testing.T, pool *pgxpool.Pool, query string) bool {
	t.Helper()
	var found bool
	if err := pool.QueryRow(context.Background(), query).Scan(&found); err != nil {
		t.Fatal(err)
	}
	return found
}
