package main

// Post-pick physical re-evaluation through the REAL production caller: the
// vendor PACK transition. Real PostgreSQL order/quote/plan/fulfilment state, the
// real Section 17C planner (branch authority, catalogue logistics profiles,
// requirement engine) and the real fulfilment-group projection. Only the
// logistics authorities are explicit test doubles.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/shared/go/commercequote"
)

// physicalMissions is the mission authority: idempotent create per reference,
// cancel with read-back semantics, and a record of every call.
type physicalMissions struct {
	mu        sync.Mutex
	creates   map[string]int
	requests  []orders.MissionRequest
	cancelled []string
	offline   map[orders.LogisticsService]bool
}

func newPhysicalMissions() *physicalMissions {
	return &physicalMissions{creates: map[string]int{}, offline: map[orders.LogisticsService]bool{}}
}

func (m *physicalMissions) Available(service orders.LogisticsService) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.offline[service]
}

func (m *physicalMissions) CreateMission(_ context.Context, request orders.MissionRequest) (orders.MissionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creates[request.MissionRef]++
	m.requests = append(m.requests, request)
	return orders.MissionResult{ExternalRef: "ext_" + request.MissionRef, Status: orders.MissionAccepted,
		AcceptedFeeMinor: request.AgreedFeeMinor}, nil
}

func (m *physicalMissions) CancelMission(_ context.Context, operationRef, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelled = append(m.cancelled, operationRef)
	return nil
}

// distinctCreated counts distinct mission identities created under a prefix.
// Re-issuing a create for the same reference is idempotent at the authority
// and is not a second mission.
func (m *physicalMissions) distinctCreated(prefix string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for ref := range m.creates {
		if strings.HasPrefix(ref, prefix) {
			total++
		}
	}
	return total
}

func (m *physicalMissions) createCount(prefix string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for ref, count := range m.creates {
		if strings.HasPrefix(ref, prefix) {
			total += count
		}
	}
	return total
}

// quoteAuthority answers for one service; offline/unserviceable are switchable.
type quoteAuthority struct {
	service     string
	fee         int64
	unavailable bool
	refuse      string
}

func (a *quoteAuthority) Available() bool { return true }

func (a *quoteAuthority) RequestQuote(_ context.Context, request commercequote.Request) (commercequote.Quote, error) {
	if a.unavailable {
		return commercequote.Quote{}, errors.New("authority unreachable")
	}
	quote := commercequote.Quote{
		QuoteID: "q_" + a.service + "_" + request.QuoteReference, QuoteReference: request.QuoteReference,
		Revision: 1, Service: a.service, Serviceable: a.refuse == "", ReasonCode: commercequote.ReasonServiceable,
		VehicleClass: request.VehicleClass, FeeMinor: a.fee, Currency: "NGN",
		AvailableProviders: 1, ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if a.refuse != "" {
		quote.ReasonCode = a.refuse
	}
	return quote, nil
}

type physicalEnv struct {
	fixture  section17EFixture
	wallet   *fakeAuthority
	order    orders.ParentOrder
	missions *physicalMissions
	taxi     *quoteAuthority
	hauling  *quoteAuthority
	vendorA  orders.VendorOrder
	actorA   orders.Actor
}

// newPhysicalEnv builds a paid, activated two-vendor Shop order whose vendor A
// mission was frozen at checkout as DISPATCH/MOTORCYCLE. variantWeight is the
// real catalogue packed weight of vendor A's line.
func newPhysicalEnv(t *testing.T, variantWeight int64, options s17cVariantOptions) *physicalEnv {
	t.Helper()
	ctx := context.Background()
	authority := newFakeAuthority()
	fixture := withPayment(buildSection17EFixture(t, true, false), authority)
	env := fixture.env
	options.id = "variant_" + env.suffix
	if variantWeight > 0 {
		options.netWeightGrams = ptrInt64(variantWeight)
	}
	options.stackable = true
	env.seedVariant(ctx, options)

	missions := newPhysicalMissions()
	fixture.service.SetFulfilmentAuthorities(&fakeInventory{}, missions)
	groupService := fulfilmentgroup.NewService(fulfilmentgroup.Options{
		Store:      fulfilmentgroup.NewStore(env.pool, events.NewDomainPublisher(events.NewOutboxRepository(env.pool))),
		Production: production.NewService(production.Options{DB: env.pool}),
		Enabled:    true,
	})
	projection := newFulfilmentOperationsProjection(groupService, env.pool)
	if projection == nil {
		t.Fatal("projection not built")
	}
	fixture.service.SetFulfilmentProjection(projection)

	taxi := &quoteAuthority{service: "TAXI", fee: 900}
	hauling := &quoteAuthority{service: "HAULING", fee: 4000}
	planner, err := fulfilment.NewPlanner(fulfilment.Options{
		Logistics: newSection17CLogisticsAuthority(env.pool),
		Branches:  newSection17CBranchAuthority(env.pool),
		Dispatch:  &quoteAuthority{service: "DISPATCH", fee: 500},
		Taxi:      taxi,
		Hauling:   hauling,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.service.SetPhysicalLoadEvaluator(newPhysicalLoadEvaluator(env.pool, planner))

	order, _, err := fixture.service.StartPayment(ctx, fixture.customerID, fixture.quoteID, fixture.finger)
	if err != nil {
		t.Fatalf("StartPayment: %v", err)
	}
	if _, err = fixture.service.ActivateFulfilment(ctx, order.ID); err != nil {
		t.Fatalf("ActivateFulfilment: %v", err)
	}
	activated, err := fixture.service.GetForCustomer(ctx, fixture.customerID, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &physicalEnv{
		fixture: fixture, wallet: authority, order: activated, missions: missions, taxi: taxi, hauling: hauling,
		vendorA: fixture.vendorOrderFor(t, activated, env.businessA),
		actorA:  fixture.actor(fixture.staffA, env.businessA),
	}
}

// pack drives vendor A through ACCEPT and the real PACK transition.
func (p *physicalEnv) pack(t *testing.T) error {
	t.Helper()
	ctx := context.Background()
	current, _, err := p.fixture.service.Advance(ctx, p.actorA, p.vendorA.ID, orders.TransitionAccept, "", p.vendorA.Version)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	_, _, err = p.fixture.service.Advance(ctx, p.actorA, current.ID, orders.TransitionPack, "", current.Version)
	return err
}

func (p *physicalEnv) missionA(t *testing.T) orders.OrderMission {
	t.Helper()
	missions, err := orders.NewStore(p.fixture.env.pool).ListMissions(context.Background(), p.order.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, mission := range missions {
		for _, id := range mission.VendorOrderIDs {
			if id == p.vendorA.ID {
				return mission
			}
		}
	}
	t.Fatal("no mission for vendor A")
	return orders.OrderMission{}
}

func (p *physicalEnv) setPicked(t *testing.T, picked, actualMeasure, expectedMeasure int64) {
	t.Helper()
	ctx := context.Background()
	pool := p.fixture.env.pool
	if expectedMeasure > 0 {
		if _, err := pool.Exec(ctx, `ALTER TABLE commerce_vendor_order_lines DISABLE TRIGGER USER`); err != nil {
			t.Fatal(err)
		}
		_, err := pool.Exec(ctx, `UPDATE commerce_vendor_order_lines SET expected_measure_quantity=$2 WHERE vendor_order_id=$1`, p.vendorA.ID, expectedMeasure)
		if _, e := pool.Exec(ctx, `ALTER TABLE commerce_vendor_order_lines ENABLE TRIGGER USER`); e != nil {
			t.Fatal(e)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO commerce_vendor_order_line_outcomes(id,parent_order_id,vendor_order_id,order_line_id,ordered_quantity,picked_quantity,unavailable_quantity,actual_measure_quantity,outcome,picking_state,quality_result)
		SELECT 'outcome_'||l.id,l.parent_order_id,l.vendor_order_id,l.id,l.quantity,$2,0,$3,'FULLY_AVAILABLE','COMPLETE','MET'
		  FROM commerce_vendor_order_lines l WHERE l.vendor_order_id=$1`,
		p.vendorA.ID, picked, actualMeasure); err != nil {
		t.Fatal(err)
	}
}

// MOTORCYCLE -> CAR: the measured load outgrows the frozen motorcycle.
func TestPhysicalReplan_MotorcycleToCarThroughPack(t *testing.T) {
	p := newPhysicalEnv(t, 15_000, s17cVariantOptions{})
	before := p.missionA(t)
	if before.Service != orders.LogisticsDispatch || before.VehicleClass != "MOTORCYCLE" || before.Status != orders.MissionAccepted {
		t.Fatalf("activated mission = %+v", before)
	}
	// Ordered 1 x 1 kg-calibrated unit; measured 2 kg -> travels as 2 units = 30 kg.
	p.setPicked(t, 1, 2_000, 1_000)
	if err := p.pack(t); err != nil {
		t.Fatalf("pack: %v", err)
	}
	after := p.missionA(t)
	if after.Service != orders.LogisticsTaxi || after.VehicleClass != "CAR" || after.MissionRef == before.MissionRef ||
		after.Status != orders.MissionAccepted || after.AgreedFeeMinor != 900 {
		t.Fatalf("escalated mission = %+v", after)
	}
	if len(p.missions.cancelled) != 1 || p.missions.cancelled[0] != before.MissionRef {
		t.Fatalf("cancelled = %v, want the old mission once", p.missions.cancelled)
	}
	if p.missions.creates[after.MissionRef] != 1 {
		t.Fatalf("replacement created %d times", p.missions.creates[after.MissionRef])
	}
	var outcome string
	var customerFee, delta int64
	if err := p.fixture.env.pool.QueryRow(context.Background(), `SELECT outcome,customer_fee_minor,platform_funded_delta_minor
		FROM commerce_mission_physical_reevaluations WHERE mission_id=$1`, after.ID).Scan(&outcome, &customerFee, &delta); err != nil {
		t.Fatal(err)
	}
	if outcome != orders.PhysicalEscalated || customerFee != 500 || delta != 400 {
		t.Fatalf("evidence outcome=%s customer=%d delta=%d", outcome, customerFee, delta)
	}
	// The customer's approved order total never moves.
	reread, err := p.fixture.service.GetForCustomer(context.Background(), p.fixture.customerID, p.order.ID)
	if err != nil || reread.CustomerApprovedTotalMinor != p.order.CustomerApprovedTotalMinor {
		t.Fatalf("customer total changed: %d -> %d (%v)", p.order.CustomerApprovedTotalMinor, reread.CustomerApprovedTotalMinor, err)
	}
	// The plan slot now executes on Taxi in a new VEHICLE_OVERFLOW version.
	var reason, authority string
	if err = p.fixture.env.pool.QueryRow(context.Background(), `
		SELECT v.reason_code,pm.authority FROM commerce_fulfilment_groups g
		  JOIN commerce_fulfilment_plan_versions v ON v.id=g.current_plan_id
		  JOIN commerce_fulfilment_plan_missions pm ON pm.plan_id=v.id AND pm.slot_ref=$2
		 WHERE g.parent_order_id=$1`, p.order.ID, after.PlanMissionRef).Scan(&reason, &authority); err != nil {
		t.Fatal(err)
	}
	if reason != string(fulfilmentgroup.ReasonVehicleOverflow) || authority != "TAXI" {
		t.Fatalf("plan version reason=%s authority=%s", reason, authority)
	}

	// Replaying the same evidence changes nothing and creates nothing.
	if _, err = p.fixture.service.ReevaluatePhysicalLoad(context.Background(), p.order.ID, p.vendorA.ID); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(p.missions.cancelled) != 1 || p.missions.creates[after.MissionRef] != 1 {
		t.Fatalf("replay duplicated work: cancelled=%v creates=%d", p.missions.cancelled, p.missions.creates[after.MissionRef])
	}

	// CAR -> VAN: a later, larger measurement escalates again; never back.
	if _, err = p.fixture.env.pool.Exec(context.Background(), `UPDATE commerce_vendor_order_line_outcomes SET actual_measure_quantity=20000 WHERE vendor_order_id=$1`, p.vendorA.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = p.fixture.service.ReevaluatePhysicalLoad(context.Background(), p.order.ID, p.vendorA.ID); err != nil {
		t.Fatalf("second escalation: %v", err)
	}
	van := p.missionA(t)
	if van.Service != orders.LogisticsHauling || van.VehicleClass != "VAN" || strings.Count(van.MissionRef, "~px") != 1 {
		t.Fatalf("car->van mission = %+v", van)
	}
}

// MOTORCYCLE -> VAN/TRUCK in one step when the load is far too big for a car.
func TestPhysicalReplan_MotorcycleToVan(t *testing.T) {
	p := newPhysicalEnv(t, 300_000, s17cVariantOptions{})
	p.setPicked(t, 1, 0, 0)
	if err := p.pack(t); err != nil {
		t.Fatalf("pack: %v", err)
	}
	if m := p.missionA(t); m.Service != orders.LogisticsHauling || m.VehicleClass != "VAN" {
		t.Fatalf("mission = %+v", m)
	}
}

// Handling and dimensions escalate too, not just weight.
func TestPhysicalReplan_HandlingAndDimensionsEscalate(t *testing.T) {
	frozen := newPhysicalEnv(t, 2_000, s17cVariantOptions{temperature: "FROZEN"})
	frozen.setPicked(t, 1, 0, 0)
	if err := frozen.pack(t); err != nil {
		t.Fatalf("frozen pack: %v", err)
	}
	if m := frozen.missionA(t); m.VehicleClass != "VAN" || m.Service != orders.LogisticsHauling {
		t.Fatalf("frozen goods mission = %+v, want a cold-chain van", m)
	}

	long := newPhysicalEnv(t, 2_000, s17cVariantOptions{lengthMm: ptrInt(1_000)})
	long.setPicked(t, 1, 0, 0)
	if err := long.pack(t); err != nil {
		t.Fatalf("long pack: %v", err)
	}
	if m := long.missionA(t); m.VehicleClass != "CAR" {
		t.Fatalf("1 m item mission = %+v, want a car", m)
	}
}

// A smaller packed load keeps the frozen class: no downgrade, no authority call.
func TestPhysicalReplan_SmallerLoadKeepsClass(t *testing.T) {
	p := newPhysicalEnv(t, 500, s17cVariantOptions{})
	before := p.missionA(t)
	p.setPicked(t, 1, 0, 0)
	if err := p.pack(t); err != nil {
		t.Fatalf("pack: %v", err)
	}
	after := p.missionA(t)
	if after.MissionRef != before.MissionRef || after.Service != before.Service || len(p.missions.cancelled) != 0 {
		t.Fatalf("unchanged load moved the mission: %+v cancelled=%v", after, p.missions.cancelled)
	}
}

// No provider / unreachable authority / missing metadata: packing is refused,
// the frozen mission is untouched and the goods are never handed over.
func TestPhysicalReplan_RefusalsKeepGoodsBack(t *testing.T) {
	for name, setup := range map[string]func(*physicalEnv){
		"no provider":           func(p *physicalEnv) { p.taxi.refuse = commercequote.ReasonNoProviderAvailable },
		"authority unavailable": func(p *physicalEnv) { p.taxi.unavailable = true },
	} {
		t.Run(name, func(t *testing.T) {
			p := newPhysicalEnv(t, 15_000, s17cVariantOptions{})
			setup(p)
			before := p.missionA(t)
			p.setPicked(t, 1, 2_000, 1_000)
			err := p.pack(t)
			if !errors.Is(err, orders.ErrPhysicalReplanRequired) {
				t.Fatalf("pack err=%v, want replan required", err)
			}
			after := p.missionA(t)
			if after.MissionRef != before.MissionRef || after.Service != orders.LogisticsDispatch || len(p.missions.cancelled) != 0 {
				t.Fatalf("refusal changed the mission: %+v cancelled=%v", after, p.missions.cancelled)
			}
			current, err := p.fixture.service.GetForCustomer(context.Background(), p.fixture.customerID, p.order.ID)
			if err != nil {
				t.Fatal(err)
			}
			if vo := p.fixture.vendorOrderFor(t, current, p.fixture.env.businessA); vo.Status == orders.VendorStatusPacking {
				t.Fatal("goods were packed for an unsafe vehicle")
			}
		})
	}
	t.Run("missing packed weight", func(t *testing.T) {
		p := newPhysicalEnv(t, 0, s17cVariantOptions{})
		p.setPicked(t, 1, 0, 0)
		if err := p.pack(t); !errors.Is(err, orders.ErrPhysicalReplanRequired) {
			t.Fatalf("pack err=%v, want refusal for missing packed weight", err)
		}
	})
}

// Package count escalates: thirteen light units exceed the motorcycle's
// twelve-package envelope.
func TestPhysicalReplan_PackageCountEscalates(t *testing.T) {
	p := newPhysicalEnv(t, 100, s17cVariantOptions{})
	p.setPicked(t, 1, 13_000, 1_000)
	if err := p.pack(t); err != nil {
		t.Fatalf("pack: %v", err)
	}
	if m := p.missionA(t); m.VehicleClass != "CAR" || m.Service != orders.LogisticsTaxi {
		t.Fatalf("13-package mission = %+v, want a car", m)
	}
}

// Concurrent re-evaluations of the same packed load: one cancel, one
// escalation row, one replacement mission.
func TestPhysicalReplan_ConcurrentReevaluationIsSingleWinner(t *testing.T) {
	p := newPhysicalEnv(t, 15_000, s17cVariantOptions{})
	before := p.missionA(t)
	p.setPicked(t, 1, 2_000, 1_000)
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := p.fixture.service.ReevaluatePhysicalLoad(context.Background(), p.order.ID, p.vendorA.ID)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	conflicts := 0
	for err := range errs {
		switch {
		case err == nil:
		case errors.Is(err, orders.ErrVersionConflict):
			conflicts++
		default:
			t.Fatalf("concurrent re-evaluation: %v", err)
		}
	}
	after := p.missionA(t)
	var escalations int
	if err := p.fixture.env.pool.QueryRow(context.Background(), `SELECT count(*) FROM commerce_mission_physical_reevaluations
		WHERE mission_id=$1 AND outcome='ESCALATED'`, after.ID).Scan(&escalations); err != nil {
		t.Fatal(err)
	}
	if escalations != 1 || p.missions.distinctCreated(before.MissionRef+"~px") != 1 || after.Service != orders.LogisticsTaxi {
		t.Fatalf("escalations=%d distinct replacements=%d mission=%+v conflicts=%d",
			escalations, p.missions.distinctCreated(before.MissionRef+"~px"), after, conflicts)
	}
	if len(p.missions.cancelled) != 1 {
		t.Fatalf("concurrent replans cancelled %d times, want 1", len(p.missions.cancelled))
	}
}
