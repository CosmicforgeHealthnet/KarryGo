package orchestration

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	readiness "cosmicforge/logistics/services/vendor-service/internal/features/fulfilment_readiness"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/shared/go/supplypreview"
)

// The commit path against a real PostgreSQL.
//
// Everything interesting here is about a create whose outcome is unknown. That
// is the failure this section exists to survive, and it cannot be observed
// without the durable row: the whole point is what a crashed or timed-out
// process leaves behind for the next pass to find.

func orchestrationTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres orchestration tests")
	}
	if !strings.Contains(strings.ToLower(dsn), "test") {
		t.Fatal("refusing a database whose name does not identify it as a test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	return pool
}

// recordingCommitter stands in for a logistics authority.
//
// It counts creates, which is the assertion that actually matters: "exactly
// once" is a claim about how many vehicles were dispatched.
type recordingCommitter struct {
	mu sync.Mutex

	creates int
	reads   int

	// outcome is what CreateMission returns. An indefinite outcome models a
	// timeout: the request may or may not have landed.
	outcome MissionOutcome
	err     error

	// existsOnRead is what a read-back finds. It is set independently of
	// outcome so a test can model the dangerous case: the create DID land, and
	// the caller never heard so.
	existsOnRead bool
	readErr      error
}

func (c *recordingCommitter) CreateMission(context.Context, Orchestration, []Pickup) (MissionOutcome, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.creates++
	return c.outcome, c.err
}

func (c *recordingCommitter) ReadMission(_ context.Context, _ logistics.Authority,
	missionRef string) (MissionOutcome, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads++
	if c.readErr != nil {
		return MissionOutcome{}, false, c.readErr
	}
	if !c.existsOnRead {
		return MissionOutcome{}, false, nil
	}
	return MissionOutcome{Definite: true, Accepted: true, MissionRef: missionRef}, true, nil
}

func (c *recordingCommitter) counts() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.creates, c.reads
}

// staticGate answers serviceability without a network.
type staticGate struct {
	usable  map[logistics.Authority]bool
	answers map[logistics.Authority]logistics.Serviceability
	errs    map[logistics.Authority]error
}

func (g staticGate) Usable(authority logistics.Authority) bool { return g.usable[authority] }

func (g staticGate) Serviceability(_ context.Context, authority logistics.Authority,
	_ logistics.Requirement) (logistics.Serviceability, error) {
	if err, failed := g.errs[authority]; failed {
		return logistics.Serviceability{}, err
	}
	return g.answers[authority], nil
}

func (g staticGate) Capability(context.Context, logistics.Authority) (logistics.Capability, error) {
	return logistics.Capability{}, nil
}

// staticReadiness supplies a prediction without a readiness row.
type staticReadiness struct{ state readiness.Readiness }

func (s staticReadiness) Read(context.Context, string) (readiness.Readiness, error) {
	return s.state, nil
}

type orchestrationHarness struct {
	pool      *pgxpool.Pool
	service   *Service
	committer *recordingCommitter
	now       time.Time
}

func newOrchestrationHarness(t *testing.T, committer *recordingCommitter) *orchestrationHarness {
	t.Helper()
	pool := orchestrationTestPool(t)
	now := time.Now().UTC()

	// Ready already, so the commit window is open on the first evaluation and
	// the test is about committing rather than about waiting.
	readyAt := now.Add(-time.Minute)
	harness := &orchestrationHarness{pool: pool, committer: committer, now: now}
	harness.service = NewService(Options{
		Repository: NewRepository(pool),
		Gate: staticGate{
			usable: map[logistics.Authority]bool{logistics.Dispatch: true},
			answers: map[logistics.Authority]logistics.Serviceability{
				logistics.Dispatch: {Authority: logistics.Dispatch, Serviceable: true, Version: 3},
			},
		},
		Preview:   staticPreview{},
		Committer: committer,
		Readiness: staticReadiness{state: readiness.Readiness{
			VendorType: readiness.VendorRestaurant, State: readiness.StateReady,
			PredictedReadyStart: &readyAt, PredictedReadyEnd: &readyAt,
			ConfidenceBand: readiness.ConfidenceHigh, UpdatedAt: now,
		}},
		JITEnabled: true, AutoSelectionEnabled: true,
		Now: func() time.Time { return harness.now },
	})
	return harness
}

type staticPreview struct{}

func (staticPreview) Preview(context.Context, logistics.Authority,
	supplypreview.Request) (supplypreview.Response, error) {
	return supplypreview.Response{Authority: string(logistics.Dispatch), Serviceable: true,
		SupplyStatus: supplypreview.StatusAvailable, ProductCode: "DISPATCH_STANDARD"}, nil
}

func (h *orchestrationHarness) open(t *testing.T) Orchestration {
	t.Helper()
	suffix := uuid.NewString()
	opened, err := h.service.Open(context.Background(), OpenRequest{
		FulfilmentGroupRef: "fg_" + suffix,
		ParentOrderID:      "po_" + suffix,
		Requirement: Requirement{
			PickupCount: 1, DropoffCount: 1, WeightGrams: 2000,
			PickupLatitude: 6.43, PickupLongitude: 3.42,
			DropoffLatitude: 6.45, DropoffLongitude: 3.40,
			DesiredPickupStart: h.now, DesiredPickupEnd: h.now.Add(10 * time.Minute),
		},
		Pickups: []Pickup{{VendorOrderID: "vo_" + suffix, Sequence: 1,
			BusinessID: "biz-1", BranchID: "br-1"}},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return opened
}

// TestCommitCreatesExactlyOneMission is mandatory test 35.
func TestCommitCreatesExactlyOneMission(t *testing.T) {
	committer := &recordingCommitter{
		outcome: MissionOutcome{Definite: true, Accepted: true, MissionRef: "mission-1"}}
	h := newOrchestrationHarness(t, committer)
	opened := h.open(t)
	ctx := context.Background()

	committed, err := h.service.Evaluate(ctx, opened.ID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if committed.State != StateCommitted {
		t.Fatalf("state = %s, want COMMITTED", committed.State)
	}
	if committed.CurrentMissionRef != "mission-1" {
		t.Fatalf("mission ref = %q", committed.CurrentMissionRef)
	}

	// Every subsequent evaluation is a no-op. A planner that kept committing
	// would keep dispatching.
	for i := 0; i < 5; i++ {
		if _, err = h.service.Evaluate(ctx, opened.ID); err != nil {
			t.Fatalf("re-evaluate %d: %v", i, err)
		}
	}
	if creates, _ := committer.counts(); creates != 1 {
		t.Fatalf("creates = %d, want exactly 1", creates)
	}

	// The requirement is frozen: an authority was asked about a specific one.
	var frozen *time.Time
	if err = h.pool.QueryRow(ctx,
		`SELECT requirements_frozen_at FROM vendor_fulfilment_orchestrations WHERE id=$1`,
		opened.ID).Scan(&frozen); err != nil {
		t.Fatal(err)
	}
	if frozen == nil {
		t.Fatal("a committed orchestration left its requirement unfrozen")
	}
}

// TestUnknownCommitOutcomeReadsBackInsteadOfRetrying is mandatory test 34, and
// the single most consequential behaviour in this section.
//
// The create timed out. It may have landed. Retrying would dispatch a second
// vehicle for one order, so the orchestration is parked and the authority is
// asked what actually happened.
func TestUnknownCommitOutcomeReadsBackInsteadOfRetrying(t *testing.T) {
	committer := &recordingCommitter{
		// A timeout: no definite answer either way.
		outcome: MissionOutcome{Definite: false},
		err:     errors.New("context deadline exceeded"),
		// The create DID land. The caller never heard so.
		existsOnRead: true,
	}
	h := newOrchestrationHarness(t, committer)
	opened := h.open(t)
	ctx := context.Background()

	after, err := h.service.Evaluate(ctx, opened.ID)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if after.RecoveryState != RecoveryReconciling {
		t.Fatalf("recovery state = %s, want RECONCILING after an unknown outcome", after.RecoveryState)
	}
	if after.State.Committed() {
		t.Fatal("an unknown outcome was recorded as committed")
	}

	creates, _ := committer.counts()
	if creates != 1 {
		t.Fatalf("creates = %d, want 1: an unknown outcome must not retry the create", creates)
	}

	// The reconciler asks, finds the mission, and adopts it.
	reconciled, err := h.service.Reconcile(ctx, opened.ID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled.State != StateCommitted {
		t.Fatalf("state after reconciliation = %s, want COMMITTED", reconciled.State)
	}
	creates, reads := committer.counts()
	if creates != 1 {
		t.Fatalf("creates = %d after reconciliation, want still 1: the mission already existed", creates)
	}
	if reads != 1 {
		t.Fatalf("reads = %d, want 1", reads)
	}
}

// TestAbsentMissionReturnsToTheCommitQueue is the other half of read-back.
//
// The authority has confirmed nothing exists, so one more attempt is safe
// precisely because somebody asked.
func TestAbsentMissionReturnsToTheCommitQueue(t *testing.T) {
	committer := &recordingCommitter{
		outcome:      MissionOutcome{Definite: false},
		err:          errors.New("connection reset"),
		existsOnRead: false,
	}
	h := newOrchestrationHarness(t, committer)
	opened := h.open(t)
	ctx := context.Background()

	if _, err := h.service.Evaluate(ctx, opened.ID); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	reconciled, err := h.service.Reconcile(ctx, opened.ID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled.State != StateCommitDue {
		t.Fatalf("state = %s, want COMMIT_DUE once the mission is confirmed absent", reconciled.State)
	}
	if reconciled.RecoveryState != RecoveryNone {
		t.Fatalf("recovery state = %s, want NONE", reconciled.RecoveryState)
	}
}

// TestAnUnanswerableReadBackStaysReconciling keeps the dangerous case parked.
//
// An unanswered question is not an answer. Guessing here is how one order
// receives two vehicles.
func TestAnUnanswerableReadBackStaysReconciling(t *testing.T) {
	committer := &recordingCommitter{
		outcome: MissionOutcome{Definite: false},
		err:     errors.New("timeout"),
		readErr: errors.New("authority still unreachable"),
	}
	h := newOrchestrationHarness(t, committer)
	opened := h.open(t)
	ctx := context.Background()

	if _, err := h.service.Evaluate(ctx, opened.ID); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	after, err := h.service.Reconcile(ctx, opened.ID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if after.RecoveryState != RecoveryReconciling {
		t.Fatalf("recovery state = %s, want it to stay RECONCILING", after.RecoveryState)
	}
	if creates, _ := committer.counts(); creates != 1 {
		t.Fatalf("creates = %d, want 1: an unanswerable read-back must not commit again", creates)
	}
}

// TestOneVendorOrderCannotBeInTwoLiveOrchestrations pins the database
// invariant that stops two planners collecting the same goods.
func TestOneVendorOrderCannotBeInTwoLiveOrchestrations(t *testing.T) {
	committer := &recordingCommitter{
		outcome: MissionOutcome{Definite: true, Accepted: true, MissionRef: "mission-1"}}
	h := newOrchestrationHarness(t, committer)
	ctx := context.Background()

	shared := "vo_shared_" + uuid.NewString()
	requirement := Requirement{
		PickupCount: 1, DropoffCount: 1,
		PickupLatitude: 6.43, PickupLongitude: 3.42,
		DropoffLatitude: 6.45, DropoffLongitude: 3.40,
		DesiredPickupStart: h.now, DesiredPickupEnd: h.now.Add(10 * time.Minute),
	}
	first, err := h.service.Open(ctx, OpenRequest{
		FulfilmentGroupRef: "fg_first_" + uuid.NewString(), ParentOrderID: "po-1",
		Requirement: requirement,
		Pickups:     []Pickup{{VendorOrderID: shared, Sequence: 1, BusinessID: "b", BranchID: "br"}},
	})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}

	_, err = h.service.Open(ctx, OpenRequest{
		FulfilmentGroupRef: "fg_second_" + uuid.NewString(), ParentOrderID: "po-1",
		Requirement: requirement,
		Pickups:     []Pickup{{VendorOrderID: shared, Sequence: 1, BusinessID: "b", BranchID: "br"}},
	})
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("second open = %v, want a refusal: two orchestrations would both collect %s", err, shared)
	}

	// Cancelling the first releases the order for replanning.
	if _, err = h.pool.Exec(ctx,
		`UPDATE vendor_fulfilment_orchestrations SET state='CANCELLED' WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.service.Open(ctx, OpenRequest{
		FulfilmentGroupRef: "fg_third_" + uuid.NewString(), ParentOrderID: "po-1",
		Requirement: requirement,
		Pickups:     []Pickup{{VendorOrderID: shared, Sequence: 1, BusinessID: "b", BranchID: "br"}},
	}); err != nil {
		t.Fatalf("replanning a released order = %v, want it to succeed", err)
	}
}

// TestSelectionEvidenceRecordsEveryCandidate is requirement 4.10: the decision
// must be explainable months later, including why the authorities that were not
// chosen were not chosen.
func TestSelectionEvidenceRecordsEveryCandidate(t *testing.T) {
	committer := &recordingCommitter{
		outcome: MissionOutcome{Definite: true, Accepted: true, MissionRef: "mission-1"}}
	h := newOrchestrationHarness(t, committer)
	opened := h.open(t)
	ctx := context.Background()

	if _, err := h.service.Evaluate(ctx, opened.ID); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	events, err := h.service.repo.Decisions(ctx, opened.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	authorities := map[logistics.Authority]bool{}
	for _, event := range events {
		seen[event.EventType] = true
		if event.EventType == EventServiceabilityChecked {
			authorities[event.CandidateAuthority] = true
		}
	}
	for _, required := range []string{EventRequirementBuilt, EventServiceabilityChecked,
		EventAuthoritySelected, EventMissionCommitted} {
		if !seen[required] {
			t.Errorf("no %s evidence was recorded", required)
		}
	}
	// All three authorities were considered, including the two that were not
	// usable -- "we never asked" is itself the answer to why they lost.
	for _, authority := range logistics.PlatformAuthorities() {
		if !authorities[authority] {
			t.Errorf("no serviceability evidence for %s", authority)
		}
	}
}

// TestEvaluationPassSurvivesOneBadOrchestration keeps one unplannable order
// from stopping every other order being planned.
func TestEvaluationPassSurvivesOneBadOrchestration(t *testing.T) {
	committer := &recordingCommitter{
		outcome: MissionOutcome{Definite: true, Accepted: true, MissionRef: "mission-ok"}}
	h := newOrchestrationHarness(t, committer)
	ctx := context.Background()

	healthy := h.open(t)
	// An orchestration with no pickups at all: Open refuses one, so it is
	// written directly to model a row damaged by an earlier partial failure.
	broken := "orc_broken_" + uuid.NewString()
	if _, err := h.pool.Exec(ctx, `
		INSERT INTO vendor_fulfilment_orchestrations(
			id, fulfilment_group_ref, parent_order_id, state, next_evaluation_at)
		VALUES($1,$2,'po-broken','PLANNED', now() - interval '1 minute')`,
		broken, "fg_broken_"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	result, err := h.service.RunEvaluationPass(ctx)
	if err != nil {
		t.Fatalf("evaluation pass returned an error instead of continuing: %v", err)
	}
	if result.Claimed == 0 {
		t.Fatal("the pass claimed nothing")
	}

	// The healthy one still committed.
	after, err := h.service.repo.Get(ctx, healthy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.State.Committed() && after.State != StateCommitDue {
		t.Fatalf("the healthy orchestration ended at %s", after.State)
	}
}
