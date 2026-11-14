package integration_test

// FLEET-13.11: a committed managed settlement instruction reaches
// payment-wallet exactly once, through the production posting adapter and a
// signed HTTP client, against a Wallet fake that behaves like Wallet's
// managed-settlement contract (idempotent on instruction identity, readable
// back by key).

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

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fleetpartner"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

const walletSettlementPath = "/api/v1/payment-wallet/internal/finance/v1/managed-settlements"

// fakeWallet mimics payment-wallet's managed-settlement contract.
type fakeWallet struct {
	mu sync.Mutex
	// postings is what Wallet holds, keyed by idempotency key.
	postings map[string]map[string]any
	// posts counts every POST that reached the server; journals counts
	// postings actually created.
	posts, journals, reads int
	bodies                 []map[string]any

	// down makes every call fail with 503.
	down bool
	// refuse makes POST answer 409, as Wallet does for a job already settled
	// by another path.
	refuse bool
	// lostResponses: the next N POSTs record the posting, then stall past the
	// client timeout so the caller never sees the answer.
	lostResponses int
	// droppedRequests: the next N POSTs stall past the client timeout
	// without recording anything.
	droppedRequests int
	stall           time.Duration
}

func newFakeWallet() *fakeWallet {
	return &fakeWallet{postings: map[string]map[string]any{}, stall: 600 * time.Millisecond}
}

func (f *fakeWallet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	if f.down {
		f.mu.Unlock()
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == walletSettlementPath:
		f.posts++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.bodies = append(f.bodies, body)
		if f.refuse {
			f.mu.Unlock()
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":"conflict","message":"This job was already settled by another path."}}`))
			return
		}
		if f.droppedRequests > 0 {
			f.droppedRequests--
			stall := f.stall
			f.mu.Unlock()
			time.Sleep(stall)
			return
		}
		key, _ := body["idempotency_key"].(string)
		posting, exists := f.postings[key]
		if !exists {
			f.journals++
			posting = map[string]any{"id": "ms_" + ids.New("x"), "instruction_version": body["instruction_version"], "status": "POSTED"}
			f.postings[key] = posting
		}
		if f.lostResponses > 0 {
			f.lostResponses--
			stall := f.stall
			f.mu.Unlock()
			time.Sleep(stall)
			return
		}
		f.mu.Unlock()
		writeWalletData(w, posting)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, walletSettlementPath+"/by-key/"):
		f.reads++
		posting, ok := f.postings[strings.TrimPrefix(r.URL.Path, walletSettlementPath+"/by-key/")]
		f.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":"not_found","message":"Managed settlement was not found."}}`))
			return
		}
		writeWalletData(w, posting)
	default:
		f.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
	}
}

func writeWalletData(w http.ResponseWriter, data map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

func (f *fakeWallet) counts() (posts, journals, reads int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posts, f.journals, f.reads
}

func (f *fakeWallet) set(fn func(*fakeWallet)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

type settlementHarness struct {
	pool        *pgxpool.Pool
	wallet      *fakeWallet
	server      *httptest.Server
	finance     *financeops.Service
	fleet       *fleetpartner.Service
	orgID       string
	instruction fleetpartner.ManagedWorkSettlementInstruction
}

func settlementFlags() fleetpartner.Flags {
	return fleetpartner.Flags{
		OperatingModeEnforcement: true, CompletedWorkProjection: true,
		ResolverMode: fleetpartner.ResolverOn, SettlementInstruction: true,
		TypedFinancialVisibility: true,
	}
}

// newSettlementHarness wires the production delivery path over a Wallet fake
// and records one managed completion, which commits one instruction and its
// delivery job.
func newSettlementHarness(t *testing.T, financeFlags financeops.Flags) *settlementHarness {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	h := &settlementHarness{pool: pool, wallet: newFakeWallet()}
	h.server = httptest.NewServer(h.wallet)
	t.Cleanup(h.server.Close)

	client := platformclients.New(platformclients.Options{
		ServiceName: "payment-wallet-service", BaseURL: h.server.URL,
		Secret: []byte("test-wallet-secret"), Timeout: 250 * time.Millisecond,
		RetryBackoffBase: time.Millisecond, RetryBackoffMax: 5 * time.Millisecond,
	})
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	h.finance = financeops.NewService(financeops.Options{
		Pool: pool, Events: publisher,
		Wallet: app.NewFinancePostingAdapter(client, nil), Flags: financeFlags,
	})
	h.fleet = fleetpartner.NewService(pool, fleetpartner.NewRepository(pool), membershipStub{},
		audit.NewRecorder(), publisher, settlementFlags())
	h.fleet.SetSettlementSubmitter(app.ManagedSettlementSubmitter{Finance: h.finance})

	orgID, membershipID, providerID := seedManagedMembership(t, pool, fleetpartner.ServiceTaxi)
	h.orgID = orgID
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_organization_settings (organization_id, operating_mode) VALUES ($1,'CENTRALIZED') ON CONFLICT (organization_id) DO NOTHING`, orgID); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	owner := tenant.Context{OrganizationID: orgID, ActorType: tenant.ActorTypeOrganizationUser, ActorID: "owner", OrganizationUserID: "owner"}
	plan, err := h.fleet.CreateCompensationPlan(ctx, owner, fleetpartner.CompensationPlan{
		Name: "Commission", Currency: "NGN", Status: fleetpartner.PlanActive,
		EffectiveFrom: time.Now().UTC().Add(-time.Hour),
	}, []fleetpartner.CompensationRule{{
		RuleType: fleetpartner.RuleCommission, Status: fleetpartner.PlanActive,
		EffectiveFrom: time.Now().UTC().Add(-time.Hour),
		Config:        fleetpartner.RuleConfig{RateBps: 1500, Basis: fleetpartner.CommissionBasisEligibleJobGross},
	}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := h.fleet.AssignCompensationPlan(ctx, owner, fleetpartner.MembershipCompensationAssignment{
		MembershipID: membershipID, CompensationPlanID: plan.ID, CompensationPlanVersion: plan.Version,
		Currency: "NGN", EffectiveFrom: time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("assign plan: %v", err)
	}
	jobID := ids.New("job")
	_, _, instruction, err := h.fleet.RecordCompletedWork(ctx, "driver-taxi-service",
		asInput(canonicalRequest(orgID, membershipID, providerID, fleetpartner.ServiceTaxi, jobID)))
	if err != nil {
		t.Fatalf("record completed work: %v", err)
	}
	if instruction.InstructionID == "" {
		t.Fatal("a resolved managed completion must commit a settlement instruction")
	}
	h.instruction = instruction
	return h
}

func postingOn() financeops.Flags { return financeops.Flags{Posting: true} }

func (h *settlementHarness) deliver(t *testing.T) (fleetpartner.ManagedWorkSettlementInstruction, error) {
	t.Helper()
	return h.fleet.DeliverSettlementInstruction(context.Background(), h.orgID, h.instruction.InstructionID, "corr-test")
}

func (h *settlementHarness) reload(t *testing.T) fleetpartner.ManagedWorkSettlementInstruction {
	t.Helper()
	in, err := fleetpartner.NewRepository(h.pool).SettlementInstruction(context.Background(), h.pool, h.orgID, h.instruction.InstructionID, false)
	if err != nil {
		t.Fatalf("reload instruction: %v", err)
	}
	return in
}

func (h *settlementHarness) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func (h *settlementHarness) postingOperations(t *testing.T) int {
	return h.count(t, `SELECT count(*) FROM fleet_partner_financial_posting_operations WHERE organization_id=$1 AND operation_type=$2`, h.orgID, financeops.OperationManagedWorkSettlement)
}

func (h *settlementHarness) events(t *testing.T, eventType string) int {
	return h.count(t, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND event_type=$2 AND aggregate_id=$3`, h.orgID, eventType, h.instruction.InstructionID)
}

func jobClass(err error) jobs.ErrorClass {
	class, _, _ := jobs.Classify(err)
	return class
}

func TestManagedSettlementInstructionCommitsWithItsDeliveryJob(t *testing.T) {
	h := newSettlementHarness(t, postingOn())
	if got := h.count(t, `SELECT count(*) FROM fleet_partner_jobs WHERE organization_id=$1 AND job_type=$2 AND dedupe_key=$3 AND status='PENDING'`,
		h.orgID, fleetpartner.JobManagedSettlementDelivery, "settlement-instruction:"+h.instruction.InstructionID); got != 1 {
		t.Fatalf("delivery jobs = %d, want exactly 1 committed with the instruction", got)
	}
	if posts, _, _ := h.wallet.counts(); posts != 0 {
		t.Fatalf("recording completed work must not call Wallet inline; posts = %d", posts)
	}
}

func TestManagedSettlementDeliverySuccessIsRecordedOnce(t *testing.T) {
	h := newSettlementHarness(t, postingOn())
	ctx := context.Background()

	// Drive it the way the worker does: through the job handler, with the
	// durable job row.
	queue := jobs.NewQueue(h.pool)
	list, err := queue.List(ctx, h.orgID, jobs.ListFilter{})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	var job jobs.Job
	for _, j := range list {
		if j.JobType == fleetpartner.JobManagedSettlementDelivery {
			job = j
		}
	}
	if job.ID == "" {
		t.Fatal("no delivery job")
	}
	if err := h.fleet.HandleSettlementDeliveryJob(ctx, job); err != nil {
		t.Fatalf("delivery: %v", err)
	}
	got := h.reload(t)
	if got.Status != fleetpartner.InstructionPosted || got.PostedAt == nil || got.WalletPostingOperationID == "" || got.WalletSettlementReference == "" {
		t.Fatalf("instruction after delivery = %+v", got)
	}
	posts, journals, _ := h.wallet.counts()
	if posts != 1 || journals != 1 {
		t.Fatalf("wallet posts=%d journals=%d, want 1/1", posts, journals)
	}
	body := h.wallet.bodies[0]
	// The exact key set payment-wallet's production route is certified
	// against (payment-wallet-service/internal/integration/
	// fleet_wire_managed_settlement_test.go). Changing one side without the
	// other must fail a test.
	wantKeys := []string{
		"instruction_id", "instruction_version", "organization_id", "membership_id",
		"provider_id", "provider_type", "source_service", "source_job_id", "source_assignment_id",
		"currency", "gross_basis_minor", "provider_share_minor", "organization_share_minor",
		"compensation_plan_id", "compensation_plan_version", "compensation_resolution_version",
		"correlation_id", "idempotency_key",
		"payloadVersion", "organizationId", "idempotencyKey", "expectedVersion",
	}
	if len(body) != len(wantKeys) {
		t.Fatalf("wire body has %d keys, want %d: %v", len(body), len(wantKeys), body)
	}
	for _, key := range wantKeys {
		if _, ok := body[key]; !ok {
			t.Fatalf("wire body is missing %q: %v", key, body)
		}
	}
	for field, want := range map[string]any{
		"instruction_id": h.instruction.InstructionID, "organization_id": h.orgID,
		"source_service": "taxi", "provider_type": "taxi", "currency": "NGN",
		"idempotency_key": h.instruction.IdempotencyKey,
	} {
		if body[field] != want {
			t.Fatalf("wire %s = %v, want %v", field, body[field], want)
		}
	}
	provider, _ := body["provider_share_minor"].(float64)
	organization, _ := body["organization_share_minor"].(float64)
	gross, _ := body["gross_basis_minor"].(float64)
	if int64(gross) != h.instruction.GrossBasisMinor || int64(provider+organization) != h.instruction.GrossBasisMinor {
		t.Fatalf("wire shares %v + %v != gross %v", provider, organization, gross)
	}
	if h.events(t, "fleet.settlement_instruction.posted.v1") != 1 {
		t.Fatal("a posted instruction must emit exactly one posted event")
	}
	if n := h.count(t, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND action='fleet.settlement_instruction.posted' AND resource_id=$2`, h.orgID, h.instruction.InstructionID); n != 1 {
		t.Fatalf("posted audit rows = %d, want 1", n)
	}
	if n := h.count(t, `SELECT count(*) FROM fleet_partner_work_settled_components WHERE organization_id=$1 AND settlement_instruction_id=$2 AND wallet_posting_operation_id=$3`,
		h.orgID, h.instruction.InstructionID, got.WalletPostingOperationID); n == 0 {
		t.Fatal("work-settled components must point at the posting that paid them")
	}

	// A second delivery (a replayed job, a sweep) is a no-op: nothing is sent.
	if _, err := h.deliver(t); err != nil {
		t.Fatalf("redelivery of a posted instruction: %v", err)
	}
	if posts, _, _ := h.wallet.counts(); posts != 1 {
		t.Fatalf("a posted instruction was sent again; posts = %d", posts)
	}
	if h.postingOperations(t) != 1 {
		t.Fatal("one instruction must have exactly one posting operation")
	}
}

func TestManagedSettlementTransientFailureRetriesWithoutDuplicating(t *testing.T) {
	h := newSettlementHarness(t, postingOn())
	h.wallet.set(func(f *fakeWallet) { f.down = true })

	_, err := h.deliver(t)
	if err == nil || jobClass(err) != jobs.ErrorClassTransient {
		t.Fatalf("a Wallet outage must be a transient job failure, got %v (%s)", err, jobClass(err))
	}
	if got := h.reload(t); got.Status != fleetpartner.InstructionPendingWallet || got.DeliveryAttempts != 1 {
		t.Fatalf("after outage: status=%s attempts=%d", got.Status, got.DeliveryAttempts)
	}

	h.wallet.set(func(f *fakeWallet) { f.down = false })
	if _, err := h.deliver(t); err != nil {
		t.Fatalf("retry after recovery: %v", err)
	}
	got := h.reload(t)
	if got.Status != fleetpartner.InstructionPosted || got.DeliveryAttempts != 2 {
		t.Fatalf("after retry: status=%s attempts=%d", got.Status, got.DeliveryAttempts)
	}
	if _, journals, _ := h.wallet.counts(); journals != 1 {
		t.Fatalf("journals = %d, want 1", journals)
	}
	if h.postingOperations(t) != 1 {
		t.Fatal("a retry must reuse the posting operation, not create a second")
	}
}

func TestManagedSettlementLostResponseIsReadBackNotResent(t *testing.T) {
	h := newSettlementHarness(t, postingOn())
	// Wallet posts, then the response is lost.
	h.wallet.set(func(f *fakeWallet) { f.lostResponses = 1 })

	if _, err := h.deliver(t); err != nil {
		t.Fatalf("a lost response that Wallet did post must resolve by read-back: %v", err)
	}
	if got := h.reload(t); got.Status != fleetpartner.InstructionPosted {
		t.Fatalf("status = %s, want POSTED", got.Status)
	}
	posts, journals, reads := h.wallet.counts()
	if journals != 1 || posts != 1 || reads < 1 {
		t.Fatalf("posts=%d journals=%d reads=%d: the outcome must be read back, never resent", posts, journals, reads)
	}
	if n := h.count(t, `SELECT count(*) FROM fleet_partner_financial_posting_attempts a JOIN fleet_partner_financial_posting_operations o ON o.id=a.operation_id WHERE o.organization_id=$1 AND o.operation_type=$2 AND a.outcome='READ_BACK_CONFIRMED'`,
		h.orgID, financeops.OperationManagedWorkSettlement); n != 1 {
		t.Fatalf("READ_BACK_CONFIRMED attempts = %d, want 1", n)
	}
}

func TestManagedSettlementUnknownOutcomeReconcilesOnNextAttempt(t *testing.T) {
	h := newSettlementHarness(t, postingOn())
	// The request never reaches Wallet's ledger and the response is lost:
	// read-back finds nothing, so the outcome stays unknown.
	h.wallet.set(func(f *fakeWallet) { f.droppedRequests = 1 })

	_, err := h.deliver(t)
	if err == nil || jobClass(err) != jobs.ErrorClassUnknownExternalOutcome {
		t.Fatalf("want an unknown external outcome, got %v (%s)", err, jobClass(err))
	}
	if got := h.reload(t); got.Status != fleetpartner.InstructionReconciliationRequired {
		t.Fatalf("status = %s, want RECONCILIATION_REQUIRED", got.Status)
	}
	if h.events(t, "fleet.settlement_instruction.reconciliation_required.v1") != 1 {
		t.Fatal("entering reconciliation must emit one event")
	}

	// Next attempt reads back first; Wallet has nothing, so this is the first
	// real send.
	postsBefore, _, readsBefore := h.wallet.counts()
	if _, err := h.deliver(t); err != nil {
		t.Fatalf("reconciling attempt: %v", err)
	}
	posts, journals, reads := h.wallet.counts()
	if reads <= readsBefore {
		t.Fatal("an unknown outcome must be read back before anything is resent")
	}
	if posts != postsBefore+1 || journals != 1 {
		t.Fatalf("posts=%d (before %d) journals=%d", posts, postsBefore, journals)
	}
	if got := h.reload(t); got.Status != fleetpartner.InstructionPosted {
		t.Fatalf("status = %s, want POSTED", got.Status)
	}
}

func TestManagedSettlementRefusalIsTerminal(t *testing.T) {
	h := newSettlementHarness(t, postingOn())
	h.wallet.set(func(f *fakeWallet) { f.refuse = true })

	_, err := h.deliver(t)
	if err == nil || jobClass(err) != jobs.ErrorClassPermanent {
		t.Fatalf("a Wallet refusal must stop the job, got %v (%s)", err, jobClass(err))
	}
	got := h.reload(t)
	if got.Status != fleetpartner.InstructionFailedTerminal || got.LastErrorCode != "wallet_refused_conflict" {
		t.Fatalf("after refusal: %+v", got)
	}
	if h.events(t, "fleet.settlement_instruction.failed_terminal.v1") != 1 {
		t.Fatal("a refused instruction must emit one failed event")
	}
	if n := h.count(t, `SELECT count(*) FROM fleet_partner_financial_posting_operations WHERE organization_id=$1 AND operation_type=$2 AND status='FAILED'`,
		h.orgID, financeops.OperationManagedWorkSettlement); n != 1 {
		t.Fatal("the refused posting operation must be FAILED so reconciliation never resends it")
	}
	h.wallet.set(func(f *fakeWallet) { f.refuse = false })
	if _, err := h.deliver(t); err != nil {
		t.Fatalf("redelivering a terminal instruction must be a no-op: %v", err)
	}
	if posts, _, _ := h.wallet.counts(); posts != 1 {
		t.Fatalf("a refused instruction was resent; posts = %d", posts)
	}
}

func TestManagedSettlementConcurrentDeliveriesSendOnce(t *testing.T) {
	h := newSettlementHarness(t, postingOn())
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.deliver(t)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && jobClass(err) != jobs.ErrorClassTransient {
			t.Fatalf("a concurrent delivery may only lose the race transiently, got %v", err)
		}
	}
	posts, journals, _ := h.wallet.counts()
	if posts != 1 || journals != 1 {
		t.Fatalf("concurrent deliveries: posts=%d journals=%d, want 1/1", posts, journals)
	}
	if got := h.reload(t); got.Status != fleetpartner.InstructionPosted {
		t.Fatalf("status = %s", got.Status)
	}
	if h.events(t, "fleet.settlement_instruction.posted.v1") != 1 {
		t.Fatal("exactly one posted event")
	}
}

func TestManagedSettlementNeverPostsWhenDisabled(t *testing.T) {
	cases := map[string]financeops.Flags{
		"posting off": {Posting: false},
		"kill switch": {Posting: true, KillSwitch: true},
	}
	for name, flags := range cases {
		t.Run(name, func(t *testing.T) {
			h := newSettlementHarness(t, flags)
			_, err := h.deliver(t)
			if err == nil || jobClass(err) != jobs.ErrorClassDependencyUnavailable {
				t.Fatalf("want dependency unavailable, got %v (%s)", err, jobClass(err))
			}
			if posts, _, reads := h.wallet.counts(); posts != 0 || reads != 0 {
				t.Fatalf("Wallet was called while posting is disabled: posts=%d reads=%d", posts, reads)
			}
			if h.postingOperations(t) != 0 {
				t.Fatal("a disabled delivery must not record a posting operation")
			}
			if got := h.reload(t); got.Status != fleetpartner.InstructionPendingWallet {
				t.Fatalf("status = %s, want PENDING_WALLET", got.Status)
			}
		})
	}

	// The instruction stage itself switched off: delivery refuses before
	// reading anything.
	pool := testPool(t)
	disabled := fleetpartner.NewService(pool, fleetpartner.NewRepository(pool), membershipStub{}, nil, nil, sectionsFlags())
	disabled.SetSettlementSubmitter(app.ManagedSettlementSubmitter{})
	if _, err := disabled.DeliverSettlementInstruction(context.Background(), "org", "inst", ""); jobClass(err) != jobs.ErrorClassDependencyUnavailable {
		t.Fatalf("instruction stage off: want dependency unavailable, got %v", err)
	}
}

func TestManagedSettlementRecoverySweepRestartsDeadDelivery(t *testing.T) {
	h := newSettlementHarness(t, postingOn())
	ctx := context.Background()
	// The original job exhausted its attempts during an outage (or the
	// process died and the job was cancelled by an operator).
	if _, err := h.pool.Exec(ctx, `UPDATE fleet_partner_jobs SET status='DEAD_LETTERED', dead_lettered_at=now() WHERE organization_id=$1 AND job_type=$2`,
		h.orgID, fleetpartner.JobManagedSettlementDelivery); err != nil {
		t.Fatal(err)
	}
	n, err := h.fleet.ScheduleSettlementDelivery(ctx, 1000)
	if err != nil || n < 1 {
		t.Fatalf("sweep scheduled %d, err %v", n, err)
	}
	// The sweep is idempotent while a live job exists.
	if _, err := h.fleet.ScheduleSettlementDelivery(ctx, 1000); err != nil {
		t.Fatal(err)
	}
	live := h.count(t, `SELECT count(*) FROM fleet_partner_jobs WHERE organization_id=$1 AND job_type=$2 AND status='PENDING'`, h.orgID, fleetpartner.JobManagedSettlementDelivery)
	if live != 1 {
		t.Fatalf("live delivery jobs = %d, want 1", live)
	}
	list, err := jobs.NewQueue(h.pool).List(ctx, h.orgID, jobs.ListFilter{Status: jobs.StatusPending})
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range list {
		if j.JobType == fleetpartner.JobManagedSettlementDelivery {
			if err := h.fleet.HandleSettlementDeliveryJob(ctx, j); err != nil {
				t.Fatalf("recovered delivery: %v", err)
			}
		}
	}
	if got := h.reload(t); got.Status != fleetpartner.InstructionPosted {
		t.Fatalf("status = %s, want POSTED", got.Status)
	}
	// A posted instruction is no longer swept.
	if n := h.count(t, `SELECT count(*) FROM fleet_partner_managed_work_settlement_instructions WHERE instruction_id=$1 AND status IN ('CREATED','PENDING_WALLET','RECONCILIATION_REQUIRED')`, h.instruction.InstructionID); n != 0 {
		t.Fatal("posted instruction still undelivered")
	}
}

func TestManagedSettlementJobRejectsForeignTenantPayload(t *testing.T) {
	h := newSettlementHarness(t, postingOn())
	payload, _ := json.Marshal(map[string]string{"organizationId": "another-org", "instructionId": h.instruction.InstructionID})
	err := h.fleet.HandleSettlementDeliveryJob(context.Background(), jobs.Job{OrganizationID: h.orgID, Payload: payload})
	if !errors.Is(err, jobs.ErrTenantMismatch) || jobClass(err) != jobs.ErrorClassValidation {
		t.Fatalf("want tenant mismatch validation error, got %v", err)
	}
	// An instruction id from another organization is simply not found.
	other := createOrganization(t, h.pool, "Other Ltd "+ids.New("x"), ids.New("rc"))
	_, err = h.fleet.DeliverSettlementInstruction(context.Background(), other.ID, h.instruction.InstructionID, "")
	if jobClass(err) != jobs.ErrorClassValidation {
		t.Fatalf("cross-tenant delivery must not find the instruction, got %v", err)
	}
	if posts, _, _ := h.wallet.counts(); posts != 0 {
		t.Fatal("no call may be made for a foreign tenant")
	}
}
