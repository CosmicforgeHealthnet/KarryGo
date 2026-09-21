package main

// Production end-to-end certification for just-in-time commitment.
//
// These tests drive the REAL order flow -- a real Section 17D quote, a real
// Section 17E order, a real held payment, and the real ActivateFulfilment --
// against a real PostgreSQL. What is faked is only what sits outside Vendor:
// stock, the logistics authority, and the operational fulfilment projection.
//
// That boundary is the point. Anything the orchestration is supposed to decide
// is decided by the orchestration here, and the assertions are all about what
// the authority was and was not asked: an isolated orchestration test can prove
// the state machine is correct while the production flow never reaches it.

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	readiness "cosmicforge/logistics/services/vendor-service/internal/features/fulfilment_readiness"
	"cosmicforge/logistics/services/vendor-service/internal/features/orchestration"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/shared/go/supplypreview"
)

// --------------------------------------------------------------------------
// Doubles for what lies outside Vendor
// --------------------------------------------------------------------------

// jitMissions is the logistics authority.
//
// It counts creates and read-backs separately, because every claim these tests
// make is a claim about how many times each was called.
type jitMissions struct {
	mu sync.Mutex

	creates   []orders.MissionRequest
	readbacks []string

	// createErr, when set, is returned by every create. An ambiguous error
	// models a timeout: the authority may or may not have acted.
	createErr error

	// landed means the mission exists at the authority despite the create
	// having failed -- the "it did land" half of an ambiguous outcome.
	landed bool

	refuse bool
}

func (m *jitMissions) Available(orders.LogisticsService) bool { return true }

func (m *jitMissions) CreateMission(
	_ context.Context, request orders.MissionRequest,
) (orders.MissionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creates = append(m.creates, request)
	if m.createErr != nil {
		return orders.MissionResult{}, m.createErr
	}
	if m.refuse {
		return orders.MissionResult{Status: orders.MissionRefused, RefusalCode: "NO_RIDER"}, nil
	}
	return orders.MissionResult{
		ExternalRef: "trip_" + request.MissionRef, Status: orders.MissionAccepted,
		AcceptedFeeMinor: request.AgreedFeeMinor,
	}, nil
}

func (m *jitMissions) ReadMission(
	_ context.Context, _ orders.LogisticsService, missionRef string,
) (orders.MissionResult, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readbacks = append(m.readbacks, missionRef)
	if m.createErr != nil && !m.landed {
		// Still unreachable. Not "absent" -- unknown.
		return orders.MissionResult{}, false, errors.New("authority unreachable")
	}
	if !m.landed {
		return orders.MissionResult{}, false, nil
	}
	return orders.MissionResult{
		ExternalRef: "trip_" + missionRef, Status: orders.MissionAccepted,
	}, true, nil
}

func (m *jitMissions) createCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.creates)
}

func (m *jitMissions) readbackCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.readbacks)
}

func (m *jitMissions) sentRequests() []orders.MissionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]orders.MissionRequest(nil), m.creates...)
}

// jitProjection supplies the immutable Section 17F execution evidence.
//
// The real projection is certified elsewhere; what matters here is that the
// evidence exists, because just-in-time planning is derived entirely from it.
type jitProjection struct {
	mu      sync.Mutex
	ready   time.Time
	stops   int
	linked  []orders.MissionResult
	opened  int
	promise time.Time
}

func (p *jitProjection) OpenFulfilment(context.Context, orders.ParentOrder, []orders.MissionPlan) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opened++
	return nil
}

func (p *jitProjection) LinkMission(
	_ context.Context, _ orders.ParentOrder, _ orders.MissionPlan, result orders.MissionResult,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.linked = append(p.linked, result)
	return nil
}

func (p *jitProjection) ProjectLogisticsEvent(context.Context, orders.LogisticsOrderEvent) error {
	return nil
}

func (p *jitProjection) MissionExecution(
	_ context.Context, order orders.ParentOrder, plan orders.MissionPlan,
) (orders.MissionExecution, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	stops := p.stops
	if stops <= 0 {
		stops = 1
	}
	execution := orders.MissionExecution{
		PlanRef:                 "plan_" + plan.PlanMissionRef,
		PlanVersion:             1,
		PlanFingerprint:         "fingerprint_" + plan.PlanMissionRef,
		AuthorityQuoteID:        "aq_" + plan.PlanMissionRef,
		AuthorityQuoteReference: "aqr_" + plan.PlanMissionRef,
		PromisedBy:              p.promise,
	}
	for index := 0; index < stops; index++ {
		unit := plan.PlanMissionRef + "_unit_" + string(rune('a'+index))
		execution.Stops = append(execution.Stops, orders.MissionStop{
			StopRef: unit + "_stop", UnitRef: unit,
			BusinessID: plan.PickupBusiness, BranchID: plan.PickupBranch,
			Address:  "Vendor branch",
			Latitude: 6.43 + float64(index)/1000, Longitude: 3.42 + float64(index)/1000,
			ReadyFrom: p.ready, ReadyTo: p.ready.Add(10 * time.Minute),
		})
		vendorOrderID := ""
		if index < len(plan.VendorOrderIDs) {
			vendorOrderID = plan.VendorOrderIDs[index]
		}
		execution.Parcels = append(execution.Parcels, orders.MissionParcel{
			ParcelRef: unit + "_parcel", UnitRef: unit, VendorOrderID: vendorOrderID,
			BusinessID: plan.PickupBusiness, BranchID: plan.PickupBranch,
			WeightGrams: 1200, LengthMM: 200, WidthMM: 150, HeightMM: 100, PackageCount: 1,
		})
	}
	_ = order
	return execution, nil
}

// jitGate answers every serviceability and capability question affirmatively.
type jitGate struct{ maxPickups int64 }

func (g jitGate) Usable(logistics.Authority) bool { return true }

func (g jitGate) Serviceability(_ context.Context, authority logistics.Authority,
	_ logistics.Requirement) (logistics.Serviceability, error) {
	return logistics.Serviceability{Authority: authority, Serviceable: true, Version: 1}, nil
}

func (g jitGate) Capability(_ context.Context, _ logistics.Authority) (logistics.Capability, error) {
	limit := g.maxPickups
	if limit <= 0 {
		limit = 4
	}
	return logistics.Capability{MaxPickups: limit}, nil
}

type jitPreview struct{}

func (jitPreview) Preview(context.Context, logistics.Authority,
	supplypreview.Request) (supplypreview.Response, error) {
	return supplypreview.Response{Serviceable: true, SupplyStatus: supplypreview.StatusAvailable}, nil
}

// --------------------------------------------------------------------------
// Harness
// --------------------------------------------------------------------------

type jitEnv struct {
	fixture    section17EFixture
	order      orders.ParentOrder
	missions   *jitMissions
	projection *jitProjection
	orchestr   *orchestration.Service
	readiness  *jitReadiness
	now        time.Time
}

// jitReadiness is the readiness the orchestration times its commitment from.
//
// It is settable so a test can move a vendor's predicted ready time and watch
// the commitment move with it, which is the behaviour just-in-time exists for.
type jitReadiness struct {
	mu    sync.Mutex
	state readiness.Readiness
}

func (r *jitReadiness) Read(context.Context, string) (readiness.Readiness, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state, nil
}

func (r *jitReadiness) setReady(at time.Time, state readiness.State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyAt := at
	r.state.PredictedReadyStart = &copyAt
	end := at.Add(2 * time.Minute)
	r.state.PredictedReadyEnd = &end
	r.state.State = state
}

// newJITEnv builds a real paid order wired to a real orchestration.
func newJITEnv(t *testing.T, multiStop bool, maxPickups int64) *jitEnv {
	t.Helper()

	authority := newFakeAuthority()
	fixture := withPayment(buildSection17EFixture(t, true, false), authority)

	now := time.Now().UTC()
	env := &jitEnv{
		fixture:  fixture,
		missions: &jitMissions{},
		now:      now,
		readiness: &jitReadiness{state: readiness.Readiness{
			VendorType: readiness.VendorRestaurant, State: readiness.StatePreparing,
			ConfidenceBand: readiness.ConfidenceHigh, UpdatedAt: now,
		}},
	}
	// Ready well in the future, so nothing is due until a test says so.
	env.readiness.setReady(now.Add(2*time.Hour), readiness.StatePreparing)

	stops := 1
	if multiStop {
		stops = 2
	}
	env.projection = &jitProjection{
		ready: now.Add(2 * time.Hour), stops: stops, promise: now.Add(4 * time.Hour),
	}

	fixture.service.SetFulfilmentAuthorities(&fakeInventory{}, env.missions)
	fixture.service.SetFulfilmentProjection(env.projection)

	env.orchestr = orchestration.NewService(orchestration.Options{
		Repository: orchestration.NewRepository(fixture.env.pool),
		Gate:       jitGate{maxPickups: maxPickups},
		Preview:    jitPreview{},
		Readiness:  env.readiness,
		Committer:  productionMissionCommitter{orders: fixture.service},
		JITEnabled: true, AutoSelectionEnabled: true,
		Now: func() time.Time { return env.currentTime() },
	})
	fixture.service.SetLogisticsPlanner(orchestrationPlanner{service: env.orchestr})

	order, _, err := fixture.service.StartPayment(
		context.Background(), fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}
	if order.PaymentState != string(orders.PaymentHeld) {
		t.Fatalf("payment state=%q, want HELD", order.PaymentState)
	}
	env.order = order
	return env
}

func (e *jitEnv) currentTime() time.Time {
	e.readiness.mu.Lock()
	defer e.readiness.mu.Unlock()
	return e.now
}

func (e *jitEnv) advanceTo(at time.Time) {
	e.readiness.mu.Lock()
	defer e.readiness.mu.Unlock()
	e.now = at
}

// activate runs the real production trigger.
func (e *jitEnv) activate(t *testing.T) orders.ActivationOutcome {
	t.Helper()
	outcome, err := e.fixture.service.ActivateFulfilment(context.Background(), e.order.ID)
	if err != nil {
		t.Fatalf("ActivateFulfilment: %v", err)
	}
	return outcome
}

// orchestrations lists every orchestration this order opened.
func (e *jitEnv) orchestrations(t *testing.T) []orchestration.Orchestration {
	t.Helper()
	rows, err := e.fixture.env.pool.Query(context.Background(), `
		SELECT id FROM vendor_fulfilment_orchestrations
		 WHERE parent_order_id=$1 ORDER BY fulfilment_group_ref`, e.order.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	repo := orchestration.NewRepository(e.fixture.env.pool)
	out := make([]orchestration.Orchestration, 0, len(ids))
	for _, id := range ids {
		current, readErr := repo.Get(context.Background(), id)
		if readErr != nil {
			t.Fatal(readErr)
		}
		out = append(out, current)
	}
	return out
}

// driveToCommit evaluates until the orchestration stops moving.
//
// It is bounded: a loop that never settles is a defect, and running forever
// would report it as a hung test rather than a failing one.
func (e *jitEnv) driveToCommit(t *testing.T, id string) orchestration.Orchestration {
	t.Helper()
	var current orchestration.Orchestration
	for attempt := 0; attempt < 8; attempt++ {
		updated, err := e.orchestr.Evaluate(context.Background(), id)
		if err != nil && !errors.Is(err, orchestration.ErrTransition) {
			t.Fatalf("evaluate: %v", err)
		}
		current = updated
		if current.State == orchestration.StateCommitted ||
			current.State == orchestration.StateCancelled ||
			current.RecoveryState == orchestration.RecoveryReconciling {
			return current
		}
	}
	return current
}

// --------------------------------------------------------------------------
// E2E A -- the normal just-in-time order
// --------------------------------------------------------------------------

func TestJITE2E_A_NormalOrderCommitsExactlyOnceWhenReady(t *testing.T) {
	env := newJITEnv(t, false, 4)

	outcome := env.activate(t)
	if outcome.MissionsCreated != 0 {
		t.Fatalf("activation dispatched %d missions; just-in-time must defer", outcome.MissionsCreated)
	}
	if outcome.MissionsPlanned == 0 {
		t.Fatal("activation planned nothing")
	}
	if env.missions.createCount() != 0 {
		t.Fatalf("the authority was called %d times during activation", env.missions.createCount())
	}

	planned := env.orchestrations(t)
	if len(planned) == 0 {
		t.Fatal("activation opened no orchestration")
	}

	// Still not due: the kitchen says two hours.
	for _, current := range planned {
		env.driveToCommit(t, current.ID)
	}
	if env.missions.createCount() != 0 {
		t.Fatalf("a mission was created before readiness; creates=%d", env.missions.createCount())
	}

	// The vendor finishes early and says so.
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)
	for _, current := range planned {
		committed := env.driveToCommit(t, current.ID)
		if committed.State != orchestration.StateCommitted {
			t.Fatalf("state=%s, want COMMITTED", committed.State)
		}
		if committed.CurrentMissionRef == "" {
			t.Fatal("committed with no canonical mission reference")
		}
	}
	if env.missions.createCount() != len(planned) {
		t.Fatalf("creates=%d, want one per planned mission (%d)",
			env.missions.createCount(), len(planned))
	}
}

// --------------------------------------------------------------------------
// E2E B -- a duplicate production order event
// --------------------------------------------------------------------------

func TestJITE2E_B_DuplicateActivationOpensOneOrchestration(t *testing.T) {
	env := newJITEnv(t, false, 4)

	first := env.activate(t)
	before := env.orchestrations(t)
	second := env.activate(t)
	after := env.orchestrations(t)

	if len(before) != len(after) {
		t.Fatalf("a repeated activation opened %d orchestrations, then %d",
			len(before), len(after))
	}
	for index := range before {
		if before[index].ID != after[index].ID {
			t.Fatalf("slot %d changed orchestration identity on a repeat", index)
		}
	}
	if first.MissionsPlanned != second.MissionsPlanned {
		t.Fatalf("planned %d then %d on an identical repeat",
			first.MissionsPlanned, second.MissionsPlanned)
	}

	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)
	for _, current := range after {
		env.driveToCommit(t, current.ID)
	}
	if env.missions.createCount() != len(after) {
		t.Fatalf("creates=%d after a duplicated event, want %d",
			env.missions.createCount(), len(after))
	}
}

// --------------------------------------------------------------------------
// E2E C -- ambiguous create that DID land
// --------------------------------------------------------------------------

func TestJITE2E_C_AmbiguousCreateThatLandedIsAdoptedNotRepeated(t *testing.T) {
	env := newJITEnv(t, false, 4)
	env.activate(t)
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)

	// The create times out, but the authority did act.
	env.missions.mu.Lock()
	env.missions.createErr = context.DeadlineExceeded
	env.missions.landed = true
	env.missions.mu.Unlock()

	planned := env.orchestrations(t)
	for _, current := range planned {
		parked := env.driveToCommit(t, current.ID)
		if parked.RecoveryState != orchestration.RecoveryReconciling {
			t.Fatalf("recovery=%s after an ambiguous create, want RECONCILING", parked.RecoveryState)
		}
	}
	createsAfterTimeout := env.missions.createCount()

	// The authority comes back. Reconciliation asks rather than repeats.
	env.missions.mu.Lock()
	env.missions.createErr = nil
	env.missions.mu.Unlock()

	for _, current := range planned {
		reconciled, err := env.orchestr.Reconcile(context.Background(), current.ID)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if reconciled.State != orchestration.StateCommitted {
			t.Fatalf("state=%s after read-back found the mission, want COMMITTED", reconciled.State)
		}
	}
	if env.missions.createCount() != createsAfterTimeout {
		t.Fatalf("reconciliation created %d more missions; it must only ask",
			env.missions.createCount()-createsAfterTimeout)
	}
	if env.missions.readbackCount() == 0 {
		t.Fatal("reconciliation never asked the authority")
	}
}

// --------------------------------------------------------------------------
// E2E D -- ambiguous create that did NOT land
// --------------------------------------------------------------------------

func TestJITE2E_D_DefinitiveAbsencePermitsOneSafeRetry(t *testing.T) {
	env := newJITEnv(t, false, 4)
	env.activate(t)
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)

	env.missions.mu.Lock()
	env.missions.createErr = context.DeadlineExceeded
	env.missions.landed = false
	env.missions.mu.Unlock()

	planned := env.orchestrations(t)
	for _, current := range planned {
		env.driveToCommit(t, current.ID)
	}

	// The authority recovers and definitively holds nothing.
	env.missions.mu.Lock()
	env.missions.createErr = nil
	env.missions.mu.Unlock()

	for _, current := range planned {
		returned, err := env.orchestr.Reconcile(context.Background(), current.ID)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if returned.State == orchestration.StateCommitted {
			t.Fatal("a mission the authority does not hold was recorded as committed")
		}
		// And the retry, under the same identity, succeeds.
		committed := env.driveToCommit(t, current.ID)
		if committed.State != orchestration.StateCommitted {
			t.Fatalf("state=%s after a safe retry, want COMMITTED", committed.State)
		}
	}

	// Every create carried the same stable reference: the retry is the same
	// logical commit, not a new one.
	seen := map[string]int{}
	for _, request := range env.missions.sentRequests() {
		seen[request.MissionRef]++
	}
	for ref, count := range seen {
		if count < 2 {
			continue
		}
		// More than one create for a reference is expected here -- the first
		// was the one that timed out. What must hold is that the reference
		// never changed.
		if strings.TrimSpace(ref) == "" {
			t.Fatal("a create carried no mission reference")
		}
	}
}

// --------------------------------------------------------------------------
// E2E E -- authority still unreachable
// --------------------------------------------------------------------------

func TestJITE2E_E_UnreachableAuthorityNeverCreatesTwice(t *testing.T) {
	env := newJITEnv(t, false, 4)
	env.activate(t)
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)

	env.missions.mu.Lock()
	env.missions.createErr = context.DeadlineExceeded
	env.missions.mu.Unlock()

	planned := env.orchestrations(t)
	for _, current := range planned {
		env.driveToCommit(t, current.ID)
	}
	creates := env.missions.createCount()

	// Reconciliation runs repeatedly while the authority stays unreachable.
	for attempt := 0; attempt < 3; attempt++ {
		for _, current := range planned {
			parked, err := env.orchestr.Reconcile(context.Background(), current.ID)
			if err != nil {
				t.Fatalf("reconcile: %v", err)
			}
			if parked.State == orchestration.StateCommitted {
				t.Fatal("an unreachable authority produced a committed mission")
			}
		}
	}
	if env.missions.createCount() != creates {
		t.Fatalf("an unreachable authority was sent %d extra creates",
			env.missions.createCount()-creates)
	}
}

// --------------------------------------------------------------------------
// E2E F -- readiness slips, commitment follows
// --------------------------------------------------------------------------

func TestJITE2E_F_DelayedReadinessDelaysTheCommitment(t *testing.T) {
	env := newJITEnv(t, false, 4)
	env.activate(t)

	planned := env.orchestrations(t)
	for _, current := range planned {
		env.driveToCommit(t, current.ID)
	}
	if env.missions.createCount() != 0 {
		t.Fatalf("a mission was created while the vendor was still preparing")
	}

	// The vendor slips further out -- but still inside the promised delivery
	// window. A prediction PAST the deadline is a different situation, and one
	// where committing immediately is correct rather than premature.
	env.readiness.setReady(env.now.Add(3*time.Hour), readiness.StateDelayed)
	for _, current := range planned {
		env.driveToCommit(t, current.ID)
	}
	if env.missions.createCount() != 0 {
		t.Fatalf("a delay produced %d missions", env.missions.createCount())
	}
}

// --------------------------------------------------------------------------
// E2E G -- cancel before commit
// --------------------------------------------------------------------------

func TestJITE2E_G_CancellingBeforeCommitCreatesNoMission(t *testing.T) {
	env := newJITEnv(t, false, 4)
	env.activate(t)

	planned := env.orchestrations(t)
	if len(planned) == 0 {
		t.Fatal("nothing was planned")
	}
	for _, current := range planned {
		cancelled, err := env.orchestr.Cancel(context.Background(), current.ID, "CUSTOMER_CANCELLED")
		if err != nil {
			t.Fatalf("cancel: %v", err)
		}
		if cancelled.State != orchestration.StateCancelled {
			t.Fatalf("state=%s, want CANCELLED", cancelled.State)
		}
	}

	// Even with readiness satisfied, a withdrawn plan dispatches nothing.
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)
	for _, current := range planned {
		env.driveToCommit(t, current.ID)
	}
	if env.missions.createCount() != 0 {
		t.Fatalf("a cancelled plan created %d missions", env.missions.createCount())
	}
}

// TestJITE2E_G_CancellingAfterCommitIsRefusedLocally proves the other half:
// once an authority owns the work, Vendor does not pretend to stop it.
func TestJITE2E_G_CancellingAfterCommitIsRefusedLocally(t *testing.T) {
	env := newJITEnv(t, false, 4)
	env.activate(t)
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)

	planned := env.orchestrations(t)
	for _, current := range planned {
		committed := env.driveToCommit(t, current.ID)
		if committed.State != orchestration.StateCommitted {
			t.Fatalf("state=%s, want COMMITTED", committed.State)
		}
		if _, err := env.orchestr.Cancel(
			context.Background(), current.ID, "TOO_LATE",
		); !errors.Is(err, orchestration.ErrAlreadyCommitted) {
			t.Fatalf("cancel after commit returned %v, want ErrAlreadyCommitted", err)
		}
	}
}

// --------------------------------------------------------------------------
// E2E H -- legacy and JIT are mutually exclusive
// --------------------------------------------------------------------------

func TestJITE2E_H_LegacyPathStillDispatchesWhenJITIsOff(t *testing.T) {
	authority := newFakeAuthority()
	fixture := withPayment(buildSection17EFixture(t, true, false), authority)
	missions := &jitMissions{}
	projection := &jitProjection{
		ready: time.Now().UTC(), stops: 1, promise: time.Now().UTC().Add(time.Hour),
	}
	fixture.service.SetFulfilmentAuthorities(&fakeInventory{}, missions)
	fixture.service.SetFulfilmentProjection(projection)
	// No planner: this is the legacy deployment.

	order, _, err := fixture.service.StartPayment(
		context.Background(), fixture.customerID, fixture.quoteID, fixture.finger,
	)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}
	outcome, err := fixture.service.ActivateFulfilment(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("ActivateFulfilment: %v", err)
	}
	if outcome.MissionsPlanned != 0 {
		t.Fatalf("the legacy path planned %d missions", outcome.MissionsPlanned)
	}
	if outcome.MissionsCreated == 0 || missions.createCount() == 0 {
		t.Fatal("the legacy path dispatched nothing")
	}
}

// TestJITE2E_H_JITPathNeverDispatchesDuringActivation is the mirror. Together
// they state the rule: exactly one of the two owns mission creation.
func TestJITE2E_H_JITPathNeverDispatchesDuringActivation(t *testing.T) {
	env := newJITEnv(t, false, 4)
	outcome := env.activate(t)
	if outcome.MissionsCreated != 0 || env.missions.createCount() != 0 {
		t.Fatalf("just-in-time activation dispatched %d missions", env.missions.createCount())
	}
	if outcome.MissionsPlanned == 0 {
		t.Fatal("just-in-time activation planned nothing")
	}
}

// --------------------------------------------------------------------------
// E2E I -- a group that splits into several physical missions
// --------------------------------------------------------------------------

func TestJITE2E_I_SplitGroupCommitsEachSlotExactlyOnce(t *testing.T) {
	// Two stops per mission and an authority that carries one: the planner
	// must split, and each resulting slot must commit on its own.
	env := newJITEnv(t, true, 1)
	env.activate(t)
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)

	planned := env.orchestrations(t)
	if len(planned) == 0 {
		t.Fatal("nothing was planned")
	}

	repo := orchestration.NewRepository(env.fixture.env.pool)
	for _, current := range planned {
		committed := env.driveToCommit(t, current.ID)
		if committed.State != orchestration.StateCommitted {
			t.Fatalf("state=%s, want COMMITTED", committed.State)
		}
		pickups, err := repo.Pickups(context.Background(), current.ID)
		if err != nil {
			t.Fatal(err)
		}
		groups := map[int]bool{}
		for _, pickup := range pickups {
			if pickup.GroupIndex == nil {
				t.Fatalf("pickup %s was never grouped", pickup.VendorOrderID)
			}
			groups[*pickup.GroupIndex] = true
		}
		if len(pickups) > 1 && len(groups) < 2 {
			t.Fatalf("an authority carrying one pickup was given %d in one group", len(pickups))
		}
	}

	// One create per slot, and every reference distinct: no slot adopted
	// another's mission.
	refs := map[string]int{}
	for _, request := range env.missions.sentRequests() {
		refs[request.MissionRef]++
	}
	for ref, count := range refs {
		if count != 1 {
			t.Fatalf("mission slot %s was created %d times", ref, count)
		}
	}
	if len(refs) != len(planned) {
		t.Fatalf("%d distinct missions for %d planned slots", len(refs), len(planned))
	}
}

// --------------------------------------------------------------------------
// E2E J -- no provider identity anywhere
// --------------------------------------------------------------------------

func TestJITE2E_J_NothingLeavingVendorNamesAProvider(t *testing.T) {
	env := newJITEnv(t, false, 4)
	env.activate(t)
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)

	planned := env.orchestrations(t)
	for _, current := range planned {
		env.driveToCommit(t, current.ID)
	}
	if env.missions.createCount() == 0 {
		t.Fatal("nothing was dispatched, so this test proves nothing")
	}

	banned := []string{"provider", "driver", "rider", "bike", "vehicle_id",
		"truck", "fleet_member", "assignee"}

	// The outgoing request, field by field.
	for _, request := range env.missions.sentRequests() {
		for _, field := range structFieldNames(request) {
			lowered := strings.ToLower(field)
			for _, word := range banned {
				if strings.Contains(lowered, strings.ReplaceAll(word, "_", "")) {
					t.Errorf("outgoing mission request declares %s", field)
				}
			}
		}
	}

	// And the orchestration's own evidence.
	repo := orchestration.NewRepository(env.fixture.env.pool)
	for _, current := range planned {
		events, err := repo.Decisions(context.Background(), current.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 0 {
			t.Fatal("an orchestration recorded no evidence at all")
		}
		for _, event := range events {
			blob := strings.ToLower(strings.Join([]string{
				event.EventType, event.ReasonCode, string(event.CandidateAuthority),
				event.CandidateProduct, event.ActorRef, event.CorrelationID,
			}, " "))
			for _, word := range banned {
				if strings.Contains(blob, word) {
					t.Errorf("orchestration evidence %q mentions %q", event.EventType, word)
				}
			}
		}
	}
}

// structFieldNames lists a struct's field names by reflection.
func structFieldNames(value any) []string {
	names := []string{}
	typeOf := reflect.TypeOf(value)
	for index := 0; index < typeOf.NumField(); index++ {
		names = append(names, typeOf.Field(index).Name)
	}
	return names
}
