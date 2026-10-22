package adminintervention

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/adminresources"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/deliveryrecovery"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const purpose = "case-1"

type env struct {
	t                     *testing.T
	pool                  *pgxpool.Pool
	svc                   *admincommand.Service
	spec                  admincommand.Spec
	actor                 adminpolicy.Actor
	scope, foreign        adminpolicy.Scope
	issuer                *contextfixture.Fixture
	store                 *admincontext.Store
	org, customer, p1, p2 string
}

func allEngines() Engines {
	return Engines{IncidentRecovery: true, ReturnAssignment: true, CustomerRecovery: true, ReturnToSender: false, MaxAttempts: 10}
}

func setup(t *testing.T) *env {
	t.Helper()
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	spec, _ := admincommand.ForOwner(adminresources.Owner)
	e := &env{t: t, pool: db, spec: spec, org: uuid.NewString(), customer: uuid.NewString(), p1: uuid.NewString(), p2: uuid.NewString()}
	e.scope = adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: e.org, Tenant: e.org}
	other := uuid.NewString()
	e.foreign = adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: other, Tenant: other}
	p := policyfixture.New(t, e.scope, spec.Owner, spec.Capability, spec.Contract)
	p.Document.Rules[0].Scopes = append(p.Document.Rules[0].Scopes, e.foreign)
	read := p.Document.Rules[0]
	read.Capability, read.Permission = spec.ReadCapability, spec.ReadCapability
	p.Document.Rules = append(p.Document.Rules, read)
	p.Sign(t)
	e.issuer = contextfixture.New(t, spec.Owner)
	e.store = &admincontext.Store{DB: db, Authority: e.issuer.Authority, Reader: adminresources.Source()}
	e.actor = adminpolicy.Actor{Product: "karryam", Staff: "staff-a", Roles: []string{"staff"}, Permissions: []string{spec.Capability, spec.ReadCapability},
		Markets: []string{"M1"}, Zones: []string{"Z1"}, Organizations: []string{e.org, other}, Tenants: []string{e.org, other}}
	e.svc = &admincommand.Service{Spec: spec, DB: db, Policy: p.Engine, Contexts: e.store, Executor: &Executor{Engines: allEngines()}, WriteEnabled: true, ReadEnabled: true}
	for i, id := range []string{e.p1, e.p2} {
		if _, err := db.Exec(ctx, `INSERT INTO providers(id,phone,full_name) VALUES($1,$2,'Rider')`, id, "+23480990000"+string(rune('1'+i))); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		e.t.Fatal(sql, err)
	}
}
func (e *env) scalar(sql string, out any, args ...any) {
	e.t.Helper()
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(out); err != nil {
		e.t.Fatal(sql, err)
	}
}

// trip creates a canonical Trip + booking + delivery group/mission.
func (e *env) trip(status, missionState string) (string, string) {
	e.t.Helper()
	var booking, trip string
	e.scalar(`INSERT INTO dispatch_bookings(customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,'A',6.5,3.3,'B',6.6,3.4,100,'R','+2348000000000') RETURNING id::text`, &booking, e.customer)
	e.scalar(`INSERT INTO trips(booking_id,provider_id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,organization_id,status) VALUES($1,$2,$3,'A',6.5,3.3,'B',6.6,3.4,100,'R','+2348000000000',$4,$5) RETURNING id::text`, &trip, booking, e.p1, e.customer, e.org, status)
	var group string
	e.scalar(`INSERT INTO dispatch_delivery_groups(customer_id,source_type,source_ref,booking_id,trip_id,state) VALUES($1,'CUSTOMER',$2,$3,$4,'ACTIVE') RETURNING id::text`, &group, e.customer, "grp:"+trip, booking, trip)
	e.exec(`INSERT INTO dispatch_delivery_missions(delivery_group_id,source_type,source_ref,booking_id,trip_id,state) VALUES($1,'CUSTOMER',$2,$3,$4,$5)`, group, "msn:"+trip, booking, trip, missionState)
	return trip, booking
}

func (e *env) authorize(kind, id string) {
	e.t.Helper()
	ctx := context.Background()
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	binding, err := adminresources.Source().Binding(ctx, tx, kind, id)
	tx.Rollback(ctx)
	if err != nil {
		e.t.Fatal(kind, err)
	}
	a := e.issuer.Approval("ctx-"+kind+"-"+id, kind, id, e.spec.Capability, binding, e.scope)
	a.Capabilities = []string{e.spec.Capability, e.spec.ReadCapability}
	if err := e.store.Import(ctx, e.issuer.Sign(e.t, a), time.Now()); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) cmd(kind, id, action string, expected int64, params map[string]string) admincommand.Command {
	if params == nil {
		params = map[string]string{}
	}
	return admincommand.Command{ContractVersion: e.spec.Contract, OperationID: uuid.NewString(), SubjectKind: kind, SubjectID: id, Action: action, ExpectedVersion: expected,
		IdempotencyKey: uuid.NewString(), ReasonCode: "OPS_CASE", Params: params, RequestedAt: time.Now().UTC().Add(-time.Second)}
}
func (e *env) approve(c *admincommand.Command, approver string) {
	c.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: approver, IntentDigest: c.IntentDigest(e.actor.Staff, purpose, e.scope), ApprovedAt: time.Now().UTC().Add(-time.Second)}
}
func (e *env) run(c admincommand.Command) (admincommand.Receipt, error) {
	return e.svc.Execute(context.Background(), e.actor, purpose, e.scope, c)
}
func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	e.scalar(sql, &n, args...)
	return n
}
func (e *env) current(kind, id string) (int64, string) {
	e.t.Helper()
	ctx := context.Background()
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	v, s, err := e.svc.Executor.Current(ctx, tx, kind, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return v, s
}

func reassign(e *env, trip string, expected int64) admincommand.Command {
	c := e.cmd("missions", trip, "REQUEST_REASSIGNMENT", expected, map[string]string{"reason": "PROVIDER_UNRESPONSIVE"})
	e.approve(&c, "staff-b")
	return c
}

func TestDispatchInterventionMigrationEmbedsSharedLedger(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000073_admin_command_ledger.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.Contains(body, "-- shared-template-sha256: "+admincommand.SchemaSHA256()+"\n") || !strings.Contains(body, admincommand.Schema) {
		t.Fatal("Dispatch command ledger migration drifted from shared/go/admincommand/schema.sql")
	}
}

// Reassignment before custody opens the existing incident replacement flow,
// and the real recovery worker claims the incident it created. Retry and
// escalation then drive that same engine through its own fields.
func TestPostgresReassignmentRetryEscalationThroughRealRecoveryEngine(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	trip, _ := e.trip("en_route_pickup", "EN_ROUTE_PICKUP")
	e.authorize("missions", trip)

	unapproved := e.cmd("missions", trip, "REQUEST_REASSIGNMENT", 0, map[string]string{"reason": "PROVIDER_UNRESPONSIVE"})
	if _, err := e.run(unapproved); !errors.Is(err, admincommand.ErrInvalid) {
		t.Fatal("reassignment executed without HQ approval", err)
	}
	self := reassign(e, trip, 0)
	e.approve(&self, e.actor.Staff)
	if _, err := e.run(self); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatal("requester approved own reassignment", err)
	}
	stale := reassign(e, trip, 4)
	if _, err := e.run(stale); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("stale version accepted", err)
	}
	if e.count(`SELECT count(*) FROM dispatch_delivery_incidents WHERE trip_id=$1`, trip) != 0 {
		t.Fatal("refused command leaked a replacement")
	}
	c := reassign(e, trip, 0)
	receipt, err := e.run(c)
	if err != nil || receipt.PriorState != "TRIP_EN_ROUTE_PICKUP" || receipt.ResultingState != "REASSIGNMENT_PENDING" || receipt.Outcome != "REPLACEMENT_REQUESTED" || receipt.RecordedVersion != 1 {
		t.Fatal("reassignment", err, receipt)
	}
	if replay, err := e.run(c); err != nil || replay != receipt {
		t.Fatal("idempotent replay did not return original receipt", err)
	}
	var incident, missionState, incidentReason string
	e.scalar(`SELECT i.id::text FROM dispatch_delivery_incidents i WHERE i.trip_id=$1 AND i.state='REASSIGNING' AND i.recovery_result='PENDING'`, &incident, trip)
	e.scalar(`SELECT state FROM dispatch_delivery_missions WHERE trip_id=$1`, &missionState, trip)
	e.scalar(`SELECT reason_code FROM dispatch_delivery_incidents WHERE id=$1`, &incidentReason, incident)
	if missionState != "REASSIGNING" || incidentReason != "OPERATIONS_DIRECTED_REPLACEMENT" {
		t.Fatal("replacement intent not recorded through the incident flow", missionState, incidentReason)
	}
	var effectRef, decision string
	e.scalar(`SELECT effect_ref FROM dispatch_admin_intervention_records WHERE subject_id=$1`, &effectRef, trip)
	e.scalar(`SELECT decision_id::text FROM admin_owner_command_decisions WHERE idempotency_key=$1`, &decision, c.IdempotencyKey)
	if effectRef != incident || decision != receipt.DecisionID || e.count(`SELECT count(*) FROM dispatch_admin_intervention_records WHERE decision_id=$1`, decision) != 1 {
		t.Fatal("intervention record not bound to ledger decision")
	}
	if v, s := e.current("missions", trip); v != 1 || s != "REASSIGNMENT_PENDING" {
		t.Fatal("current", v, s)
	}

	// The existing recovery worker picks the request up.
	repo := deliveryrecovery.NewRepository(e.pool)
	claimed, err := repo.ClaimIncidentRecovery(ctx, time.Now().Add(time.Second), "worker-1", time.Minute, 10)
	if err != nil || len(claimed) != 1 || claimed[0].ID != incident || claimed[0].Attempts != 1 {
		t.Fatal("recovery engine did not claim the reassignment", err, claimed)
	}
	e.authorize("incidents", incident)
	retry := func(v int64) admincommand.Command {
		return e.cmd("incidents", incident, "RETRY_INCIDENT_RECOVERY", v, nil)
	}
	if _, err := e.run(retry(0)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("retry accepted while a worker attempt is in flight", err)
	}
	future := time.Now().Add(time.Hour)
	if err := repo.CompleteIncidentRecovery(ctx, incident, "worker-1", "UNKNOWN", "ASSIGNMENT_OUTCOME_UNKNOWN", future); err != nil {
		t.Fatal(err)
	}
	if none, _ := repo.ClaimIncidentRecovery(ctx, time.Now().Add(time.Second), "worker-2", time.Minute, 10); len(none) != 0 {
		t.Fatal("backoff not honoured before intervention")
	}
	r, err := e.run(retry(0))
	if err != nil || r.PriorState != "RECOVERY_UNKNOWN" || r.ResultingState != "RECOVERY_UNKNOWN" || r.Outcome != "RECOVERY_RETRY_SCHEDULED" {
		t.Fatal("retry", err, r)
	}
	var result string
	e.scalar(`SELECT recovery_result FROM dispatch_delivery_incidents WHERE id=$1`, &result, incident)
	if result != "UNKNOWN" {
		t.Fatal("retry fabricated a recovery result", result)
	}
	again, err := repo.ClaimIncidentRecovery(ctx, time.Now().Add(time.Second), "worker-2", time.Minute, 10)
	if err != nil || len(again) != 1 || again[0].Attempts != 2 {
		t.Fatal("engine did not pick up the re-armed retry", err, again)
	}
	if err := repo.CompleteIncidentRecovery(ctx, incident, "worker-2", "PENDING", "", future); err != nil {
		t.Fatal(err)
	}
	// An open replacement offer is withdrawn by escalation, as on exhaustion.
	var mission string
	e.scalar(`SELECT id::text FROM dispatch_delivery_missions WHERE trip_id=$1`, &mission, trip)
	var candidate string
	e.scalar(`INSERT INTO dispatch_assignment_candidates(mission_id,provider_id,rank,distance_meters,candidate_snapshot) VALUES($1,$2,1,100,'{}') RETURNING id::text`, &candidate, mission, e.p2)
	e.exec(`INSERT INTO dispatch_assignment_offers(mission_id,candidate_id,provider_id,offered_at,expires_at,incident_id) VALUES($1,$2,$3,now(),now()+interval '1 minute',$4)`, mission, candidate, e.p2, incident)
	if _, err := e.run(e.cmd("incidents", incident, "ESCALATE_INCIDENT_RECOVERY", 0, map[string]string{"escalation": "SAFETY_REVIEW"})); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("stale escalation accepted", err)
	}
	esc, err := e.run(e.cmd("incidents", incident, "ESCALATE_INCIDENT_RECOVERY", 1, map[string]string{"escalation": "SAFETY_REVIEW"}))
	if err != nil || esc.ResultingState != "RECOVERY_MANUAL_REVIEW" || esc.RecordedVersion != 2 {
		t.Fatal("escalation", err, esc)
	}
	var code string
	e.scalar(`SELECT recovery_result||':'||recovery_error_code FROM dispatch_delivery_incidents WHERE id=$1`, &code, incident)
	if code != "MANUAL_REVIEW:ADMIN_ESCALATED_SAFETY_REVIEW" || e.count(`SELECT count(*) FROM dispatch_assignment_offers WHERE incident_id=$1 AND status='OFFERED'`, incident) != 0 {
		t.Fatal("escalation did not reach the engine's operations state", code)
	}
	if none, _ := repo.ClaimIncidentRecovery(ctx, time.Now().Add(2*time.Hour), "worker-3", time.Minute, 10); len(none) != 0 {
		t.Fatal("engine still works an escalated incident")
	}
	if _, err := e.run(retry(2)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("retry from MANUAL_REVIEW accepted", err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE dispatch_admin_intervention_records SET outcome='X'`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatal("intervention records mutable", err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE dispatch_admin_intervention_heads SET version=version+5`); err == nil {
		t.Fatal("intervention head skipped versions")
	}
	history, err := e.svc.History(ctx, e.actor, purpose, e.scope, "incidents", incident)
	if err != nil || history.CurrentVersion != 2 || history.CurrentState != "RECOVERY_MANUAL_REVIEW" || len(history.Decisions) != 2 {
		t.Fatal("history", err, history)
	}
}

// Custody already held, a pickup handover in progress or an uncertain relay
// outcome each refuse reassignment with no side effect.
func TestPostgresReassignmentRefusedForCustodyAndUncertainOutcome(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	refused := func(name, trip string) {
		t.Helper()
		e.authorize("missions", trip)
		if _, err := e.run(reassign(e, trip, 0)); !errors.Is(err, admincommand.ErrRejected) {
			t.Fatalf("%s: reassignment not refused: %v", name, err)
		}
		if e.count(`SELECT count(*) FROM dispatch_delivery_incidents WHERE trip_id=$1`, trip)+e.count(`SELECT count(*) FROM dispatch_admin_intervention_heads WHERE subject_id=$1`, trip)+
			e.count(`SELECT count(*) FROM admin_owner_command_decisions WHERE subject_id=$1`, trip) != 0 {
			t.Fatalf("%s: refusal had side effects", name)
		}
		var state string
		e.scalar(`SELECT state FROM dispatch_delivery_missions WHERE trip_id=$1`, &state, trip)
		if state == "REASSIGNING" {
			t.Fatalf("%s: mission moved", name)
		}
	}
	picked, _ := e.trip("in_progress", "PICKED_UP")
	refused("custody after pickup", picked)
	transit, _ := e.trip("arrived_pickup", "IN_TRANSIT")
	refused("mission past pickup", transit)

	consumed, booking := e.trip("arrived_pickup", "AT_PICKUP")
	var mission string
	e.scalar(`SELECT id::text FROM dispatch_delivery_missions WHERE trip_id=$1`, &mission, consumed)
	e.exec(`INSERT INTO dispatch_handover_challenges(challenge_ref,context_key,version,stage,mission_id,mission_ref,stop_ref,trip_id,booking_id,manifest_ref,manifest_version,assigned_provider_id,intended_recipient_ref,recipient_type,otp_hash,qr_token_hash,expires_at,max_attempts,status,consumed_at,created_by_source,source_version)
		VALUES($1,$1,1,'PICKUP',$2::uuid,$2::text,'pickup',$3,$4,'m',1,$5,'sender','SENDER','h','q',now()+interval '1 hour',5,'CONSUMED',now(),'test',1)`, "ch-"+consumed, mission, consumed, booking, e.p1)
	refused("consumed pickup handover", consumed)

	fenced, _ := e.trip("assigned", "ASSIGNED")
	e.exec(`UPDATE trips SET fleet_assignment_id='fa-fenced' WHERE id=$1`, fenced)
	var relay string
	e.scalar(`INSERT INTO dispatch_fleet_relay_operations(operation_type,provider_id,assignment_id,idempotency_key,request_fingerprint,payload,payload_hash,status,readback_required) VALUES('PROVIDER_ASSIGNMENT_ACCEPT',$1,'fa-fenced','relay-1','fp','{}','h','UNKNOWN_OUTCOME',true) RETURNING id::text`, &relay, e.p1)
	refused("uncertain relay outcome", fenced)
	// Authoritative readback resolves the fence; the same request now proceeds.
	e.exec(`UPDATE dispatch_fleet_relay_operations SET status='CONFIRMED',readback_required=false,completed_at=now() WHERE id=$1`, relay)
	if r, err := e.run(reassign(e, fenced, 0)); err != nil || r.ResultingState != "REASSIGNMENT_PENDING" {
		t.Fatal("reassignment after readback", err)
	}
	if _, err := e.run(reassign(e, fenced, 1)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("second replacement opened while one is active", err)
	}
	_ = ctx
}

func TestPostgresConcurrentReassignmentCommitsExactlyOnce(t *testing.T) {
	e := setup(t)
	trip, _ := e.trip("assigned", "ASSIGNED")
	e.authorize("missions", trip)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.run(reassign(e, trip, 0))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	ok := 0
	for err := range errs {
		if err == nil {
			ok++
		} else if !errors.Is(err, admincommand.ErrConflict) && !errors.Is(err, admincommand.ErrRejected) {
			t.Fatal("unexpected", err)
		}
	}
	if ok != 1 || e.count(`SELECT count(*) FROM dispatch_delivery_incidents WHERE trip_id=$1`, trip) != 1 || e.count(`SELECT count(*) FROM dispatch_admin_intervention_records WHERE subject_id=$1`, trip) != 1 {
		t.Fatal("concurrent reassignment not exactly once", ok)
	}
}

func TestPostgresCrossTenantSubjectDenied(t *testing.T) {
	e := setup(t)
	trip, _ := e.trip("assigned", "ASSIGNED")
	e.authorize("missions", trip)
	c := e.cmd("missions", trip, "REQUEST_REASSIGNMENT", 0, map[string]string{"reason": "OPERATIONS_DIRECTED"})
	c.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: "staff-b", IntentDigest: c.IntentDigest(e.actor.Staff, purpose, e.foreign), ApprovedAt: time.Now().UTC()}
	if _, err := e.svc.Execute(context.Background(), e.actor, purpose, e.foreign, c); err == nil {
		t.Fatal("foreign tenant reassigned another tenant's mission")
	}
	if e.count(`SELECT count(*) FROM dispatch_delivery_incidents WHERE trip_id=$1`, trip) != 0 {
		t.Fatal("cross-tenant refusal had a side effect")
	}
	if _, err := e.svc.History(context.Background(), e.actor, purpose, e.foreign, "missions", trip); err == nil {
		t.Fatal("cross-tenant history disclosed")
	}
}

// returnFixture builds a fully linked return workflow (the shape native
// createReturnWorkflow produces) in the given assignment state.
func (e *env) returnFixture(assignment, workflow string, attempts int) string {
	e.t.Helper()
	trip, booking := e.trip("in_progress", "RETURNING")
	attempt, action, returnBooking, group, mission, wf := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	e.exec(`INSERT INTO dispatch_delivery_attempts(id,trip_id,provider_id,attempt_number,state,arrived_at,grace_expires_at,failure_reason,allowed_actions,policy_version,source_version,idempotency_key,request_fingerprint) VALUES($1::uuid,$2,$3,1,'RECOVERY_SELECTED',now()-interval '1 hour',now()-interval '50 minutes','RECEIVER_REFUSED','["RETURN_TO_SENDER"]','p',2,$1::uuid::text,'fp')`, attempt, trip, e.p1)
	e.exec(`INSERT INTO dispatch_delivery_recovery_actions(id,delivery_attempt_id,trip_id,booking_id,customer_id,action,state,policy_version,expected_trip_version,idempotency_key,request_fingerprint) VALUES($1::uuid,$2,$3,$4,$5,'RETURN_TO_SENDER','READY','p',1,$1::uuid::text,'fp')`, action, attempt, trip, booking, e.customer)
	e.exec(`INSERT INTO dispatch_bookings(id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,status) VALUES($1,$2,'B',6.6,3.4,'A',6.5,3.3,0,'Original sender','','searching')`, returnBooking, e.customer)
	e.exec(`INSERT INTO dispatch_delivery_groups(id,customer_id,source_type,source_ref,booking_id,trip_id,state) VALUES($1,$2,'CUSTOMER',$3,$4,$5,'RETURNING')`, group, e.customer, "return:"+booking, booking, trip)
	e.exec(`INSERT INTO dispatch_delivery_missions(id,delivery_group_id,source_type,source_ref,booking_id,state) VALUES($1,$2,'CUSTOMER',$3,$4,'PLANNED')`, mission, group, "return:"+trip, returnBooking)
	e.exec(`INSERT INTO dispatch_mission_execution_links(mission_id,booking_id,source_type,source_ref,source_version,operation_ref,request_fingerprint) VALUES($1,$2,'CUSTOMER',$3,1,$4,'fp')`, mission, returnBooking, "return:"+trip, action+":return-execution")
	e.exec(`INSERT INTO dispatch_return_workflows(id,recovery_action_id,original_trip_id,delivery_group_id,return_mission_id,return_booking_id,route_reference,route_snapshot,policy_version,state,assignment_state,assignment_attempt_count,assignment_next_attempt_at) VALUES($1,$2,$3,$4,$5,$6,'route','{}','p',$7,$8,$9,now()+interval '1 hour')`,
		wf, action, trip, group, mission, returnBooking, workflow, assignment, attempts)
	e.authorize("returns", wf)
	return wf
}

func TestPostgresReturnAssignmentRetryRearmsRealWorker(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	repo := deliveryrecovery.NewRepository(e.pool)
	wf := e.returnFixture("DEAD_LETTER", "SUPPORT_REQUIRED", 10)
	if items, err := repo.ClaimReturnAssignments(ctx, time.Now().Add(2*time.Hour), "w0", time.Minute, 10, 100); err != nil || len(items) != 0 {
		t.Fatal("dead letter claimable before intervention", err, items)
	}
	r, err := e.run(e.cmd("returns", wf, "RETRY_RETURN_ASSIGNMENT", 0, nil))
	if err != nil || r.PriorState != "ASSIGNMENT_DEAD_LETTER" || r.ResultingState != "ASSIGNMENT_UNKNOWN" || r.Outcome != "ASSIGNMENT_RETRY_REARMED" {
		t.Fatal("rearm", err, r)
	}
	items, err := repo.ClaimReturnAssignments(ctx, time.Now().Add(time.Second), "w1", time.Minute, 10, 100)
	if err != nil || len(items) != 1 || items[0].WorkflowID != wf || items[0].AttemptCount != 0 {
		t.Fatal("return worker did not pick up the re-armed retry", err, items)
	}
	if _, err := e.run(e.cmd("returns", wf, "RETRY_RETURN_ASSIGNMENT", 1, nil)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("retry accepted during a live worker lease", err)
	}
	if err := repo.CompleteReturnAssignment(ctx, wf, true, "", time.Now(), time.Minute, 10, "w1"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.run(e.cmd("returns", wf, "RETRY_RETURN_ASSIGNMENT", 1, nil)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("retry of an acknowledged assignment accepted", err)
	}
	if v, s := e.current("returns", wf); v != 1 || s != "ASSIGNMENT_ACKNOWLEDGED" {
		t.Fatal("current", v, s)
	}
	waiting := e.returnFixture("UNKNOWN", "ROUTED", 3)
	if r, err := e.run(e.cmd("returns", waiting, "RETRY_RETURN_ASSIGNMENT", 0, nil)); err != nil || r.Outcome != "ASSIGNMENT_RETRY_SCHEDULED" {
		t.Fatal("bring-forward", err)
	}
	if items, _ := repo.ClaimReturnAssignments(ctx, time.Now().Add(time.Second), "w2", time.Minute, 10, 100); len(items) != 1 || items[0].WorkflowID != waiting {
		t.Fatal("return worker did not pick up the scheduled retry", items)
	}
	disabled := &Executor{Engines: Engines{}}
	e.svc.Executor = disabled
	idle := e.returnFixture("UNKNOWN", "ROUTED", 1)
	if _, err := e.run(e.cmd("returns", idle, "RETRY_RETURN_ASSIGNMENT", 0, nil)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("retry recorded although no engine would act", err)
	}
}

// Replanning never bypasses the customer gate: the action waits in
// AWAITING_CUSTOMER_APPROVAL, scheduled activation ignores it, and only the
// customer's existing selection path (with its own gates) completes it.
func TestPostgresDeliveryReplanAwaitsCustomerApproval(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	trip, booking := e.trip("in_progress", "AT_DELIVERY")
	attempt, action := uuid.NewString(), uuid.NewString()
	e.exec(`INSERT INTO dispatch_delivery_attempts(id,trip_id,provider_id,attempt_number,state,arrived_at,grace_expires_at,failure_reason,allowed_actions,policy_version,source_version,idempotency_key,request_fingerprint) VALUES($1::uuid,$2,$3,1,'RECOVERY_SELECTED',now()-interval '1 hour',now()-interval '50 minutes','RECEIVER_NO_ANSWER','["RETRY_SAME_DAY","RESCHEDULE"]','p',2,$1::uuid::text,'fp')`, attempt, trip, e.p1)
	e.exec(`INSERT INTO dispatch_delivery_recovery_actions(id,delivery_attempt_id,trip_id,booking_id,customer_id,action,state,scheduled_for,policy_version,expected_trip_version,idempotency_key,request_fingerprint) VALUES($1::uuid,$2,$3,$4,$5,'RETRY_SAME_DAY','SCHEDULED',now()+interval '2 hours','p',1,$1::uuid::text,'fp')`, action, attempt, trip, booking, e.customer)
	e.authorize("delivery-recoveries", action)
	replan := func(kind string, v int64) admincommand.Command {
		c := e.cmd("delivery-recoveries", action, "REQUEST_DELIVERY_REPLAN", v, map[string]string{"replan": kind})
		e.approve(&c, "staff-b")
		return c
	}
	if _, err := e.run(replan("RETURN_TO_SENDER", 0)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("return replan accepted with the return workflow disabled", err)
	}
	r, err := e.run(replan("RESCHEDULE_ATTEMPT", 0))
	if err != nil || r.PriorState != "SCHEDULED" || r.ResultingState != "AWAITING_CUSTOMER_APPROVAL" || r.Outcome != "REPLAN_AWAITING_CUSTOMER_APPROVAL" {
		t.Fatal("replan", err, r)
	}
	var gate, attemptState string
	var allowed []string
	e.scalar(`SELECT gate FROM dispatch_admin_intervention_records WHERE decision_id=$1`, &gate, r.DecisionID)
	e.scalar(`SELECT state FROM dispatch_delivery_attempts WHERE id=$1`, &attemptState, attempt)
	e.scalar(`SELECT ARRAY(SELECT jsonb_array_elements_text(allowed_actions)) FROM dispatch_delivery_attempts WHERE id=$1`, &allowed, attempt)
	if gate != "AWAITING_CUSTOMER_APPROVAL" || attemptState != "RECEIVER_UNAVAILABLE" || len(allowed) != 1 || allowed[0] != "RESCHEDULE" {
		t.Fatal("customer gate not opened", gate, attemptState, allowed)
	}
	repo := deliveryrecovery.NewRepository(e.pool)
	if n, err := repo.ActivateScheduled(ctx, time.Now().Add(3*time.Hour), 100); err != nil || n != 0 {
		t.Fatal("scheduled activation bypassed the customer gate", err, n)
	}
	if _, err := e.run(replan("RESCHEDULE_ATTEMPT", 1)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("second replan accepted while awaiting the customer", err)
	}
	// The customer's own selection path is the approval.
	customer := deliveryrecovery.New(repo, deliveryrecovery.Policy{Enabled: true})
	when := time.Now().Add(26 * time.Hour)
	if _, err := customer.SelectRecovery(ctx, deliveryrecovery.SelectRecoveryCommand{BookingID: booking, CustomerID: e.customer, Action: deliveryrecovery.ReturnToSender, IdempotencyKey: uuid.NewString()}); err == nil {
		t.Fatal("customer selected an action outside the requested replan")
	}
	selected, err := customer.SelectRecovery(ctx, deliveryrecovery.SelectRecoveryCommand{BookingID: booking, CustomerID: e.customer, Action: deliveryrecovery.Reschedule, ScheduledFor: &when, IdempotencyKey: uuid.NewString()})
	if err != nil || selected.State != "SCHEDULED" {
		t.Fatal("customer approval path", err, selected)
	}
	var parked string
	e.scalar(`SELECT state FROM dispatch_delivery_recovery_actions WHERE id=$1`, &parked, action)
	if parked != "COMPLETED" {
		t.Fatal("parked action not completed by customer approval", parked)
	}
	if v, s := e.current("delivery-recoveries", action); v != 1 || s != "COMPLETED" {
		t.Fatal("current", v, s)
	}
	// A plan carrying a financial authorization is never re-routed.
	trip2, booking2 := e.trip("in_progress", "AT_DELIVERY")
	attempt2, action2 := uuid.NewString(), uuid.NewString()
	e.exec(`INSERT INTO dispatch_delivery_attempts(id,trip_id,provider_id,attempt_number,state,arrived_at,grace_expires_at,failure_reason,allowed_actions,policy_version,source_version,idempotency_key,request_fingerprint) VALUES($1::uuid,$2,$3,1,'RECOVERY_SELECTED',now()-interval '1 hour',now()-interval '50 minutes','RECEIVER_NO_ANSWER','["RESCHEDULE"]','p',2,$1::uuid::text,'fp')`, attempt2, trip2, e.p1)
	e.exec(`INSERT INTO dispatch_delivery_recovery_actions(id,delivery_attempt_id,trip_id,booking_id,customer_id,action,state,scheduled_for,financial_state,financial_operation_ref,policy_version,expected_trip_version,idempotency_key,request_fingerprint) VALUES($1::uuid,$2,$3,$4,$5,'RESCHEDULE','SCHEDULED',now()+interval '2 hours','APPROVED','pay-1','p',1,$1::uuid::text,'fp')`, action2, attempt2, trip2, booking2, e.customer)
	e.authorize("delivery-recoveries", action2)
	c := e.cmd("delivery-recoveries", action2, "REQUEST_DELIVERY_REPLAN", 0, map[string]string{"replan": "RESCHEDULE_ATTEMPT"})
	e.approve(&c, "staff-b")
	if _, err := e.run(c); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("replan bypassed a financial authorization", err)
	}
}

func TestPostgresSignedHTTPBoundary(t *testing.T) {
	e := setup(t)
	trip, _ := e.trip("assigned", "ASSIGNED")
	e.authorize("missions", trip)
	gin.SetMode(gin.TestMode)
	write, read := bytes.Repeat([]byte("w"), 32), bytes.Repeat([]byte("r"), 32)
	h := &admincommand.Handler{Service: e.svc, WriteSecret: write, ReadSecret: read}
	router := gin.New()
	h.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	client, err := admincommand.NewClient(adminresources.Owner, server.URL, write, read)
	if err != nil {
		t.Fatal(err)
	}
	q := adminread.Query{Scope: adminread.Scope{Market: e.scope.Market, Zone: e.scope.Zone, Organization: e.scope.Organization, Tenant: e.scope.Tenant}, Staff: e.actor.Staff, Purpose: purpose,
		Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1, Role: "staff", PolicyID: e.svc.Policy.ID(), PolicyVersion: e.svc.Policy.Version(), PolicyChecksum: e.svc.Policy.Checksum()}
	wrong, _ := admincommand.NewClient(adminresources.Owner, server.URL, bytes.Repeat([]byte("x"), 32), read)
	if _, err := wrong.Execute(context.Background(), q, reassign(e, trip, 0)); err == nil {
		t.Fatal("wrongly keyed writer accepted")
	}
	c := reassign(e, trip, 0)
	receipt, err := client.Execute(context.Background(), q, c)
	if err != nil || receipt.ResultingState != "REASSIGNMENT_PENDING" {
		t.Fatal("signed reassignment", err)
	}
	if back, err := client.Readback(context.Background(), q, "missions", trip, c.IdempotencyKey); err != nil || back != receipt {
		t.Fatal("signed readback", err)
	}
	if _, err := client.Readback(context.Background(), q, "missions", trip, uuid.NewString()); !errors.Is(err, admincommand.ErrAbsent) {
		t.Fatal("absent readback not distinguished", err)
	}
	if hist, err := client.History(context.Background(), q, "missions", trip); err != nil || hist.CurrentState != "REASSIGNMENT_PENDING" || hist.CurrentVersion != 1 {
		t.Fatal("signed history", err)
	}
	if _, err := client.Execute(context.Background(), q, reassign(e, trip, 0)); !errors.Is(err, admincommand.ErrRefused) {
		t.Fatal("stale version not refused over the wire", err)
	}
	// A captured signed request cannot be replayed (single-use nonce).
	body, _ := json.Marshal(reassign(e, trip, 1))
	send := func(req *http.Request) (int, string) {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]string
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out["error"]
	}
	first, _ := http.NewRequest(http.MethodPost, server.URL+e.spec.Path()+"?"+q.Values().Encode(), bytes.NewReader(body))
	first.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(first, adminread.Caller, write, body, time.Now()); err != nil {
		t.Fatal(err)
	}
	second, _ := http.NewRequest(http.MethodPost, first.URL.String(), bytes.NewReader(body))
	second.Header = first.Header.Clone()
	if status, code := send(first); status != 422 || code != "transition_rejected" {
		t.Fatal("first signed request", status, code)
	}
	if status, code := send(second); status != 409 || code != "replay_rejected" {
		t.Fatal("replayed nonce accepted", status, code)
	}
}
